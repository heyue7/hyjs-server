"""Two-method Awake difficulty-row overlay; no APK packaging or device action."""
import argparse
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from local_asset_ready.remove_gate import exact_body, require, sha

ROOT = Path(__file__).resolve().parents[2]
BASE_APK = 'a137ae5b6a5b9b46352205ff9b0578128a872b066a5de88a518efc65ae4d9b6b'
BASE_DLL = 'f4eb8b19aabaa861edb4e669fe43739d2df6f27b0b23500b86a3dde75c4ce8e9'
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
TARGETS = {
    'AwakeDiffItem::SetDiffItem': '565547e0692e52fc99f28fdd99b09d767fe2d6e4f2bdab06dbd0598f7cb5b516',
    'ChallengeActivityCopyPage::SetSecondPage': 'd95fe78da820eaeb1c67e162988bedbc37bb320cb62c5f5003701265d3defca8',
}


def methods(pe):
    return [('.'.join(filter(None, [str(t.TypeNamespace), str(t.TypeName)])) + '::' + str(m.row.Name),
             m.row_index, m.row) for t in pe.net.mdtables.TypeDef for m in t.MethodList]


def operand(value):
    return value.value if hasattr(value, 'value') else value.index if hasattr(value, 'index') else value


def symbol(pe, token):
    value = token.value
    table = {0x04: 'Field', 0x06: 'MethodDef', 0x0a: 'MemberRef'}[value >> 24]
    return str(getattr(pe.net.mdtables, table).rows[(value & 0xffffff) - 1].Name)


def verify(before_raw, after_raw):
    require(sha(before_raw) == BASE_DLL, 'Exact installed P9 managed base required')
    before, after = dnfile.dnPE(data=before_raw), dnfile.dnPE(data=after_raw)
    old_methods, new_methods = methods(before), methods(after)
    require(len(old_methods) == len(new_methods), 'No added/removed methods')
    unchanged = engine = 0
    targets, bodies = [], {}
    for (name, rid, old), (newname, newrid, new) in zip(old_methods, new_methods):
        require((name, rid) == (newname, newrid) and old.Signature.value == new.Signature.value
                and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags,
                'Method ABI/token/flags changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'Bodyless method changed: ' + name)
            continue
        a, b = exact_body(before, old), exact_body(after, new)
        if name not in TARGETS:
            require(a == b, 'Non-target raw header/IL/EH changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
            continue
        require(sha(a) == TARGETS[name], 'Original target guard differs: ' + name)
        ob, nb = read_method_body_from_bytes(a), read_method_body_from_bytes(b)
        require(not ob.exception_handlers and not nb.exception_handlers, 'Unexpected target EH')
        suffix = nb.instructions[-len(ob.instructions):]
        old_index = {i.offset: n for n, i in enumerate(ob.instructions)}
        new_index = {i.offset: n for n, i in enumerate(suffix)}
        for oi, ni in zip(ob.instructions, suffix):
            require(oi.opcode.name == ni.opcode.name, 'Original suffix opcode changed: ' + name)
            if oi.opcode.flow_control.name in {'Branch', 'Cond_Branch'}:
                require(old_index[oi.operand] == new_index[ni.operand], 'Original suffix branch changed')
            else:
                require(operand(oi.operand) == operand(ni.operand), 'Original suffix operand changed')
        targets.append(dict(method=name, token=hex(0x06000000 | rid), inputRawSHA256=sha(a),
                            outputRawSHA256=sha(b), originalInstructions=len(ob.instructions),
                            prefixInstructions=len(nb.instructions)-len(ob.instructions),
                            originalInstructionAndBranchSuffixPreserved=True, exceptionRegions=0))
        bodies[name] = b
    require(engine == 547 and len(targets) == 2, 'Original Engine/target inventory differs')
    return dict(originalEngineMethods=engine, unchangedRawMethodBodies=unchanged,
                allOtherOriginalRawMethodHeadersILAndEHByteIdentical=True,
                methodTokensABISignaturesAndFlagsPreserved=True, onlyChangedMethods=list(TARGETS),
                onlyChangedEntries=list(TARGETS),
                targets=targets), after, bodies


def execute(pe, raw, arguments, bodies, events):
    """Execute emitted IL control flow with explicit dictionary/Unity call stubs.

    Original list, toggle and UILabel calls are modeled. This does not execute Unity.
    """
    code = read_method_body_from_bytes(raw).instructions
    offsets = {i.offset: n for n, i in enumerate(code)}
    stack, local, pc = [], [None] * 5, 0
    for _ in range(3000):
        i = code[pc]
        pc += 1
        op, arg = i.opcode.name, i.operand
        if op == 'nop':
            pass
        elif op.startswith('ldarg'):
            stack.append(arguments[int(op[-1]) if op[-1].isdigit() else arg.index])
        elif op == 'ldnull':
            stack.append(None)
        elif op.startswith('ldc.i4'):
            tail = op[7:]
            stack.append(int(arg) if op in {'ldc.i4', 'ldc.i4.s'} else -1 if tail == 'm1' else int(tail))
        elif op.startswith('stloc'):
            local[int(op[-1]) if op[-1].isdigit() else arg.index] = stack.pop()
        elif op.startswith('ldloca'):
            stack.append(('local-ref', arg.index))
        elif op.startswith('ldloc'):
            stack.append(local[int(op[-1]) if op[-1].isdigit() else arg.index])
        elif op == 'ldfld':
            stack.append(stack.pop()[symbol(pe, arg)])
        elif op == 'stfld':
            value, obj = stack.pop(), stack.pop()
            obj[symbol(pe, arg)] = value
        elif op == 'ldsfld':
            stack.append('' if symbol(pe, arg) == 'Empty' else 'existing-original-comparer')
        elif op == 'ldlen':
            stack.append(len(stack.pop()))
        elif op == 'conv.i4':
            stack.append(int(stack.pop()))
        elif op == 'ldelem.ref':
            index, array = stack.pop(), stack.pop()
            stack.append(array[index])
        elif op == 'add':
            right, left = stack.pop(), stack.pop()
            stack.append(left + right)
        elif op == 'ceq':
            right, left = stack.pop(), stack.pop()
            stack.append(left == right)
        elif op in {'call', 'callvirt'}:
            name = symbol(pe, arg)
            if name == 'get_gameObject':
                obj = stack.pop()
                stack.append(obj.setdefault('gameObject', {'active': True}))
            elif name == 'SetActive':
                active, obj = stack.pop(), stack.pop()
                obj['active'] = bool(active)
            elif name == 'get_Count':
                stack.append(len(stack.pop()))
            elif name == 'get_Item':
                index, obj = stack.pop(), stack.pop()
                stack.append(obj[index])
            elif name == 'get_smallCopys':
                stack.append(stack.pop()['smallCopys'])
            elif name == 'Sort':
                stack.pop()
                stack.pop().sort(key=lambda x: int(x['copyId']))
            elif name == 'get_dropReward':
                stack.append(stack.pop()['dropReward'])
            elif name == 'get_isOpen':
                stack.append(stack.pop()['isOpen'])
            elif name == 'get_fightPower':
                stack.append(stack.pop()['fightPower'])
            elif name == 'ToString':
                value = stack.pop()
                stack.append(str(local[value[1]]) if isinstance(value, tuple) else str(value))
            elif name == 'set_text':
                value, obj = stack.pop(), stack.pop()
                obj['text'] = value
            elif name == 'SetDiffItem':
                args = [stack.pop() for _ in range(6)][::-1]
                execute(pe, bodies['AwakeDiffItem::SetDiffItem'], args, bodies, events)
            elif name == 'ContainsKey':
                key, obj = stack.pop(), stack.pop()
                stack.append(key in obj)
            elif name == 'UpdateItem':
                stack.pop()
                stack.pop()
            elif name == 'SetBg':
                events.append(('background', stack.pop()['currAwakeCopy']))
            elif name == 'SetOpenTime':
                chapter = stack.pop()
                stack.pop()
                events.append(('openTime', chapter))
            else:
                raise AssertionError('Unmodeled emitted call: ' + name)
        elif op in {'br', 'br.s'}:
            pc = offsets[arg]
        elif op in {'brtrue', 'brtrue.s', 'brfalse', 'brfalse.s'}:
            if bool(stack.pop()) == op.startswith('brtrue'):
                pc = offsets[arg]
        elif op in {'blt', 'bgt', 'ble', 'bge', 'bne.un'}:
            right, left = stack.pop(), stack.pop()
            if {'blt': left < right, 'bgt': left > right, 'ble': left <= right, 'bge': left >= right,
                'bne.un': left != right}[op]:
                pc = offsets[arg]
        elif op == 'ret':
            require(not stack, 'Emitted void return stack is unbalanced')
            return
        else:
            raise AssertionError('Unmodeled emitted opcode: ' + op)
    raise AssertionError('Emitted control flow exceeded bound')


def vectors(pe, bodies):
    def row():
        return dict(taskInfo={'copyId': 'stale501'}, page={'stale': True}, gameObject={'active': True},
                    canBattle={'active': True}, cannotBattle={'active': False}, fight={})
    slots = [row() for _ in range(3)]
    page = dict(AwakeDiffTab=slots, chapterDic={}, selectDict={})
    evidence = []
    for chapter, count, opened in [(5, 3, 1), (1, 0, 1), (2, 1, 1), (3, 2, 0), (5, 3, 1), (1, 0, 1)]:
        tasks = [dict(copyId=str(chapter*100+n+1), dropReward='item_144', fightPower=100+n)
                 for n in reversed(range(count))]
        page['chapterDic'][chapter] = dict(smallCopys=tasks, isOpen=opened)
        calls = []
        execute(pe, bodies['ChallengeActivityCopyPage::SetSecondPage'], [page, chapter], bodies, calls)
        for index, slot in enumerate(slots):
            require(slot['gameObject']['active'] == (index < count), 'Stale row remains or valid row hidden')
            if index < count:
                require(slot['taskInfo']['copyId'] == str(chapter*100+index+1) and slot['page'] is page,
                        'Current sorted task was not rebound')
                require(slot['canBattle']['active'] == bool(opened)
                        and slot['cannotBattle']['active'] != bool(opened), 'Original open gate changed')
                require(slot['fight']['text'] == str(100+index), 'Original fight label changed')
            else:
                require(slot['taskInfo'] is None and slot['page'] is None, 'Hidden row retained private callback identity')
        require(page['currAwakeCopy'] == chapter and calls == [('background', chapter), ('openTime', chapter)],
                'Original chapter/background/open-time suffix changed')
        evidence.append(dict(chapter=chapter, taskCount=count, isOpen=bool(opened),
                             visibleTaskIDs=[s['taskInfo']['copyId'] for s in slots if s['gameObject']['active']],
                             hiddenRowsHaveNullTaskAndPage=True))
    # Even an empty dictionary must clear retained rows before original early exit.
    page['chapterDic'] = {}
    execute(pe, bodies['ChallengeActivityCopyPage::SetSecondPage'], [page, 4], bodies, [])
    require(all(not r['gameObject']['active'] and r['taskInfo'] is None and r['page'] is None for r in slots),
            'Empty chapter dictionary did not clear rows')
    return dict(status='PASS', method='Emitted IL interpreter with explicit original list/Unity-call models',
                transitions=evidence, emptyDictionaryClears=True, originalSuffixPreserved=True,
                unityOrPhoneExecuted=False, resourceGateOrSceneMappingChanged=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-proof', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    require(not args.output_directory.exists(), 'Fresh output directory required')
    proof_raw = args.base_proof.read_bytes()
    proof = json.loads(proof_raw)
    require(sha(args.base_apk.read_bytes()) == BASE_APK and proof.get('binarySHA256') == BASE_APK,
            'Pinned installed P9 APK/proof required')
    require(proof.get('signatureVerified') and proof.get('zipalignVerified'), 'Qualified base required')
    with zipfile.ZipFile(args.base_apk) as apk:
        require(len(apk.namelist()) == len(set(apk.namelist())), 'Duplicate APK entries')
        before = apk.read(ENTRY)
        require(sha(before) == BASE_DLL, 'Installed P9 assembly differs')
        for entry, pin in proof['qualifiedWalletEntries'].items():
            require(sha(apk.read(entry)) == pin, 'Inherited qualified entry differs: ' + entry)
    args.output_directory.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet, dnlib = runtime/'dotnet/dotnet', runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    with tempfile.TemporaryDirectory(prefix='hyjs-awake-diff-rows-') as temporary:
        folder = Path(temporary)
        (folder/'Assembly-CSharp.dll').write_bytes(before)
        shutil.copy2(Path(__file__).with_name('Injector.cs'), folder/'Program.cs')
        project = folder/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib">'
                           '<HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        def run(argv):
            subprocess.run(['rtk', 'proxy', *map(str, argv)], cwd=ROOT, check=True, timeout=90)
        run([dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        output = args.output_directory/'Assembly-CSharp.dll'
        run([dotnet, folder/'bin/Debug/net8.0/Patch.dll', folder/'Assembly-CSharp.dll', output])
        checks, pe, bodies = verify(before, output.read_bytes())
        correctness = vectors(pe, bodies)
    require(sha(args.base_apk.read_bytes()) == BASE_APK, 'Input APK changed')
    source_files = [Path(__file__).resolve(), Path(__file__).with_name('Injector.cs').resolve(),
                    ROOT/'tools/local_asset_ready/remove_gate.py']
    manifest = dict(schema='awake-diff-rows-ui-overlay-v1', status='PASS_OFFLINE_NOT_PACKAGED', **checks,
                    baseAPKSHA256=BASE_APK, baseProof=str(args.base_proof), baseProofSHA256=sha(proof_raw),
                    inputManagedSHA256=BASE_DLL, outputManagedSHA256=sha(output.read_bytes()),
                    replacementEntries={ENTRY: sha(output.read_bytes())}, correctness=correctness,
                    inheritedP9PetEquipFivePreserved=True, originalEngineAndEHUnchanged=True,
                    noNewRuntimeAssembly=True, apkBuilt=False, phoneExecuted=False,
                    privateFieldsResetWithinDeclaringType=True, asyncCallbackAndABIChanged=False,
                    sourcePins={str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in source_files})
    (args.output_directory/'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps({'status':manifest['status'], 'outputManagedSHA256':manifest['outputManagedSHA256'],
                      'unchangedRawMethodBodies':checks['unchangedRawMethodBodies'],
                      'originalEngineMethods':checks['originalEngineMethods'],
                      'vectors':len(correctness['transitions'])+1}))


if __name__ == '__main__':
    main()
