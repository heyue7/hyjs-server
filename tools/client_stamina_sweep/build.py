"""Bounded stamina-purchase layout and copy-sweep stop overlay.

Builds an overlay main DLL plus one new helper DLL. It never packages,
installs, launches the game or touches a device.
"""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require

SOURCE = Path(__file__).resolve().parent
APK = ROOT / 'build/hyjs-local.apk'
APK_SHA = '9fec499af7473f5fd05da31670e27b3e9eb270ba9b0c9d52bd6f3238e1425285'
INPUT_SHA = '45bcf1ef1529bcdc85866923a53629f7e08a7a24540d34beb7230822a9a22b13'
PINS = {
    'UI.BuyItemPage::SetItem': 'd1a7fe6f44b85c78eae236104d5feca72300e6783c70fd5d5ac52cc32b74a707',
    'UI.UICopySweep::DoOpen': '30a5f0daac37079a688020deec9d81295d470421d890c0b1efb358a2d435d6df',
    'UI.UICopySweep::DoClose': '75483ab113118784ed688c73258f2396c4435b0964b356bec944ea5d0a4f8fc6',
    'UI.UICopySweep::OnSweepCopyAuto': 'b5ac1f0f11d9d0db5d0afa21b1c1c05d6fa16ee721d78470444fa72bbb06d841',
    'UI.UICopySweepEvent::OnStopClick': '0fd2a1a6c31ee139e1d92d52f2814ce1d884d3e535093ca6393bb1c04f365dc8',
    '<_StartSweep>c__Iterator141::MoveNext': '521f31ab82a6c92a9bb95b1093cd6b17da4429a27aaaf5ed9d824e15925c5a7a',
    '<OnSweepCopyAuto>c__AnonStorey19B::<>m__1E7': '5003116eaf09e6d47f4b36805abde58623eeeb7171819b3e92d0f3b721000136',
}
# Insertion points are expressed as instructions in the pinned input body.
# (index resolver, added instruction count, required helper calls)
INSERTS = {
    'UI.BuyItemPage::SetItem': [('quoted', 6, ['StaminaShop::IsStamina', 'StaminaShop::Render'])],
    'UI.UICopySweep::DoOpen': [('needStop', 2, ['StaminaSweep::Opened'])],
    'UI.UICopySweep::DoClose': [('first', 2, ['StaminaSweep::Closed'])],
    'UI.UICopySweep::OnSweepCopyAuto': [('first', 4, ['StaminaSweep::BeginSend']), ('offset:0020', 3, ['StaminaSweep::Capture'])],
    'UI.UICopySweepEvent::OnStopClick': [('needStop', 3, ['StaminaSweep::Stop'])],
    '<_StartSweep>c__Iterator141::MoveNext': [('offset:00ad', 4, ['StaminaSweep::CanContinue'])],
    '<OnSweepCopyAuto>c__AnonStorey19B::<>m__1E7': [('first', 8, ['StaminaSweep::Accept']), ('offset:00b6', 4, ['StaminaSweep::CanStartNext'])],
}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def decode(pe, method):
    return list(body(pe, method).instructions)


def body(pe, method):
    return read_method_body_from_bytes(pe.get_data(method.Rva, 1048576))


def instruction_offset_index(decoded):
    result = {}
    for index, instruction in enumerate(decoded):
        result[instruction.offset] = index
    end = decoded[-1].offset + decoded[-1].size
    result[end] = len(decoded)
    return result


def member_name(pe, token):
    table, index = token >> 24, token & 0xffffff
    try:
        if table == 0x2b:
            spec = pe.net.mdtables.MethodSpec.rows[index - 1]
            coded = spec.Method
            row = coded.row
            parent = getattr(row, 'Class', None)
            owner = ''
            if parent is not None:
                owner = str(getattr(parent.row, 'TypeName', ''))
            return owner + '::' + str(row.Name)
        if table == 0x0a:
            row = pe.net.mdtables.MemberRef.rows[index - 1]
            parent = row.Class.row
            return str(getattr(parent, 'TypeName', '')) + '::' + str(row.Name)
        if table == 0x06:
            row = pe.net.mdtables.MethodDef.rows[index - 1]
            return str(row.Name)
        if table == 0x04:
            return str(pe.net.mdtables.Field.rows[index - 1].Name)
        if table in (0x01, 0x02):
            row = pe.net.mdtables.TypeRef.rows[index - 1] if table == 0x01 else pe.net.mdtables.TypeDef.rows[index - 1]
            return str(row.TypeNamespace) + '.' + str(row.TypeName)
        if table == 0x70:
            return repr(pe.net.user_strings.get(index).value)
    except Exception:
        pass
    return str(token)


BRANCH = ('br', 'beq', 'bne', 'bge', 'bgt', 'ble', 'blt', 'leave', 'brfalse', 'brtrue', 'brnull', 'brinst', 'brzero', 'switch')
MEMBER = ('call', 'callvirt', 'newobj', 'ldftn', 'ldvirtftn', 'box', 'castclass', 'isinst',
          'ldtoken', 'ldfld', 'ldflda', 'stfld', 'ldsfld', 'ldsflda', 'stsfld')


def key(pe, instruction):
    op = instruction.opcode.name
    if op in BRANCH:
        return op
    operand = instruction.operand
    token = getattr(operand, 'value', None)
    if op in MEMBER and isinstance(token, int):
        return op + ' ' + member_name(pe, token)
    if op == 'ldstr' and isinstance(token, int):
        return op + ' ' + member_name(pe, token)
    if isinstance(token, int) and op in ('ldc.i4', 'ldc.i4.s', 'ldc.i4.0', 'ldc.i4.1', 'ldc.i4.2', 'ldc.i4.3',
                                        'ldc.i4.4', 'ldc.i4.5', 'ldc.i4.6', 'ldc.i4.7', 'ldc.i4.8', 'ldc.i4.m1',
                                        'ldc.i4.8'):
        return op + ' ' + str(token)
    return op + (' ' + str(operand) if operand is not None else '')


def insertion_index(decoded, spec):
    if spec == 'first':
        return 0
    if spec.startswith('offset:'):
        offset = int(spec.split(':', 1)[1], 16)
        return instruction_offset_index(decoded)[offset]
    if spec == 'needStop':
        for index, instruction in enumerate(decoded):
            token = getattr(instruction.operand, 'value', None)
            if instruction.opcode.name == 'stfld' and isinstance(token, int):
                row = pe_field(decoded, token)
                if row == 'needStop':
                    return index + 1
        raise ValueError('needStop store not found')
    if spec == 'quoted':
        for index, instruction in enumerate(decoded):
            if instruction.opcode.name == 'call' and 'QuotedShopCost::BuyPage' in key(PE[0], instruction):
                return index - 1
        raise ValueError('QuotedShopCost::BuyPage call not found')
    raise ValueError('Unknown insertion specification: ' + spec)


PE = [None]


def pe_field(decoded, token):
    table, index = token >> 24, token & 0xffffff
    if table != 0x04:
        return ''
    return str(PE[0].net.mdtables.Field.rows[index - 1].Name)


def verify_target(pe, name, old, new):
    old_instructions = decode(pe[0], old)
    new_instructions = decode(pe[1], new)
    old_keys = [key(pe[0], instruction) for instruction in old_instructions]
    new_keys = [key(pe[1], instruction) for instruction in new_instructions]
    points = sorted((insertion_index(old_instructions, spec), count, required)
                    for spec, count, required in INSERTS[name])
    mapping = {}
    blocks = {}
    old_position = 0
    new_position = 0
    inserted_new_indexes = []
    for old_index, count, required in points:
        prefix = old_index - old_position
        require(new_keys[new_position:new_position + prefix] == old_keys[old_position:old_index],
                'Original instruction stream changed before insertion: ' + name)
        for offset in range(prefix): mapping[old_position + offset] = new_position + offset
        blocks[old_index] = new_position + prefix
        inserted = new_keys[new_position + prefix:new_position + prefix + count]
        require(len(inserted) == count, 'Insertion count differs: ' + name)
        for wanted in required:
            require(any(wanted in value for value in inserted), 'Missing helper call ' + wanted + ': ' + name)
        inserted_new_indexes.extend(range(new_position + prefix, new_position + prefix + count))
        new_position += prefix + count
        old_position = old_index
    require(new_keys[new_position:] == old_keys[old_position:], 'Original instruction stream changed after insertion: ' + name)

    for offset in range(len(old_keys) - old_position): mapping[old_position + offset] = new_position + offset
    expected = {
        ('UI.BuyItemPage::SetItem', 'quoted'): ['ldarg.0', 'call StaminaShop::IsStamina', 'brfalse', 'ldarg.0', 'call StaminaShop::Render', 'br'],
        ('UI.UICopySweep::DoOpen', 'needStop'): ['ldarg.0', 'call StaminaSweep::Opened'],
        ('UI.UICopySweep::DoClose', 'first'): ['ldarg.0', 'call StaminaSweep::Closed'],
        ('UI.UICopySweep::OnSweepCopyAuto', 'first'): ['ldarg.0', 'call StaminaSweep::BeginSend', 'brtrue', 'ret'],
        ('UI.UICopySweep::OnSweepCopyAuto', 'offset:0020'): ['ldloc.1', 'ldarg.0', 'call StaminaSweep::Capture'],
        ('<_StartSweep>c__Iterator141::MoveNext', 'offset:00ad'): ['ldarg.0', 'ldfld <>f__this', 'call StaminaSweep::CanContinue', 'brfalse'],
        ('<OnSweepCopyAuto>c__AnonStorey19B::<>m__1E7', 'first'): ['ldarg.0', 'ldarg.0', 'ldfld <>f__this', 'ldarg.1', 'ldarg.2', 'call StaminaSweep::Accept', 'brtrue', 'ret'],
        ('<OnSweepCopyAuto>c__AnonStorey19B::<>m__1E7', 'offset:00b6'): ['ldarg.0', 'ldfld <>f__this', 'call StaminaSweep::CanStartNext', 'brfalse'],
    }
    for spec, count, required in INSERTS[name]:
        old_index = insertion_index(old_instructions, spec)
        start = blocks[old_index]
        actual = new_keys[start:start + count]
        wanted = expected.get((name, spec))
        if wanted is None:
            require(name == 'UI.UICopySweepEvent::OnStopClick' and actual[0] == 'ldarg.0' and actual[2] == 'call StaminaSweep::Stop' and actual[1] == old_keys[1], 'Stop insertion differs')
        else:
            require(actual == wanted, 'Exact insertion differs: ' + name + ' ' + spec + repr(actual))
    old_offsets = instruction_offset_index(old_instructions)
    new_offsets = instruction_offset_index(new_instructions)
    for old_index, new_index in mapping.items():
        old_ins, new_ins = old_instructions[old_index], new_instructions[new_index]
        if old_ins.opcode.name not in BRANCH:
            old_token = getattr(old_ins.operand, 'value', None)
            if isinstance(old_token, int):
                require(getattr(new_ins.operand, 'value', None) == old_token, 'Original operand token changed: ' + name)
            continue
        old_targets = old_ins.operand if old_ins.opcode.name == 'switch' else [old_ins.operand]
        new_targets = new_ins.operand if new_ins.opcode.name == 'switch' else [new_ins.operand]
        require(len(old_targets) == len(new_targets), 'Branch target count differs')
        for old_target, new_target in zip(old_targets, new_targets):
            target_index = old_offsets[old_target]
            expected_target = mapping[target_index]
            if (name.startswith('<_StartSweep>') and old_instructions[target_index].offset == 0xad) or (name.startswith('<OnSweepCopyAuto>') and old_instructions[target_index].offset == 0xb6):
                expected_target = blocks[target_index]
            require(new_offsets[new_target] == expected_target, 'Original branch target changed: ' + name)

    # Every newly introduced control-flow edge is pinned as well.
    for spec, count, required in INSERTS[name]:
        old_index = insertion_index(old_instructions, spec)
        start = blocks[old_index]
        for offset in range(count):
            ins = new_instructions[start + offset]
            if ins.opcode.name not in BRANCH: continue
            target = None
            if name == 'UI.BuyItemPage::SetItem':
                target = mapping[old_index] if offset == 2 else mapping[old_index + 2]
            elif name == 'UI.UICopySweep::OnSweepCopyAuto': target = mapping[0]
            elif name.startswith('<_StartSweep>'):
                target = mapping[old_offsets[0xd1]]
            elif name.startswith('<OnSweepCopyAuto>'):
                target = mapping[0] if spec == 'first' else mapping[old_offsets[0xe6]]
            require(target is not None and new_offsets[ins.operand] == target, 'Inserted branch target differs: ' + name)

    old_handlers = body(pe[0], old).exception_handlers
    new_handlers = body(pe[1], new).exception_handlers
    require(len(old_handlers) == len(new_handlers), 'Exception handler count changed: ' + name)
    for old_handler, new_handler in zip(old_handlers, new_handlers):
        require(old_handler.exception_type == new_handler.exception_type, 'Exception handler type changed: ' + name)
        require(str(old_handler.catch_type) == str(new_handler.catch_type), 'Catch type changed: ' + name)
        old_base, new_base = old_instructions[0].offset, new_instructions[0].offset
        for attr in ('try_start', 'try_end', 'handler_start', 'handler_end'):
            old_boundary = old_offsets[getattr(old_handler, attr) + old_base]
            new_boundary = new_offsets[getattr(new_handler, attr) + new_base]
            expected_boundary = mapping.get(old_boundary, len(new_instructions))
            require(new_boundary == expected_boundary, 'Exception boundary changed: ' + name + ' ' + attr)
    return dict(method=name, beforeSHA256=sha(exact_body(pe[0], old)), afterSHA256=sha(exact_body(pe[1], new)),
                insertedInstructions=len(new_keys) - len(old_keys))


def verify(before, after):
    require(sha(before) == INPUT_SHA, 'Input managed SHA differs')
    left, right = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    PE.clear()
    PE.extend([left, right])
    old_methods, new_methods = methods(left), methods(right)
    require(len(old_methods) == len(new_methods), 'Method inventory changed')
    for table in ('TypeDef', 'Field', 'MethodDef'):
        require(len(getattr(left.net.mdtables, table).rows) == len(getattr(right.net.mdtables, table).rows),
                'Original metadata inventory changed: ' + table)
    for old_field, new_field in zip(left.net.mdtables.Field.rows, right.net.mdtables.Field.rows):
        require(str(old_field.Name) == str(new_field.Name) and old_field.Signature.value == new_field.Signature.value and old_field.struct.Flags == new_field.struct.Flags, 'Original field ABI changed')
    for old_type, new_type in zip(left.net.mdtables.TypeDef.rows, right.net.mdtables.TypeDef.rows):
        require(str(old_type.TypeName) == str(new_type.TypeName) and str(old_type.TypeNamespace) == str(new_type.TypeNamespace) and old_type.struct.Flags == new_type.struct.Flags and old_type.struct.Extends_CodedIndex == new_type.struct.Extends_CodedIndex and old_type.struct.FieldList_Index == new_type.struct.FieldList_Index and old_type.struct.MethodList_Index == new_type.struct.MethodList_Index, 'Original type ABI changed')
    require(not getattr(right.net.mdtables, 'FieldPtr', None), 'Unexpected old-Unity FieldPtr')
    require(left.net.struct.Flags == right.net.struct.Flags, 'CLI flags changed')
    require(left.net.struct.StrongNameSignatureSize == right.net.struct.StrongNameSignatureSize == 0,
            'Unexpected strong-name signature')

    def identity(row):
        return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion,
                row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)

    require([identity(row) for row in left.net.mdtables.Assembly.rows] ==
            [identity(row) for row in right.net.mdtables.Assembly.rows], 'Assembly identity changed')
    old_refs = left.net.mdtables.AssemblyRef.rows
    new_refs = right.net.mdtables.AssemblyRef.rows
    require([identity(row) for row in new_refs[:len(old_refs)]] == [identity(row) for row in old_refs],
            'Original assembly references changed')
    require(len(new_refs) == len(old_refs) + 1 and str(new_refs[-1].Name) == 'SoulFlowerStaminaSweep',
            'Unexpected assembly reference addition')
    for row in new_refs:
        require(not (row.struct.Flags & 1) or bool(row.PublicKey.value), 'AssemblyRef full key missing: ' + str(row.Name))
    require(new_refs[-1].struct.Flags == 0 and new_refs[-1].PublicKey.value == b'',
            'Helper must have an empty PublicKeyToken without the PublicKey flag')

    changes = []
    engine = unchanged = 0
    for (name, rid, old), (new_name, new_rid, new) in zip(old_methods, new_methods):
        require((name, rid) == (new_name, new_rid) and old.Signature.value == new.Signature.value
                and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags,
                'Original ABI/token changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'New original body: ' + name)
            continue
        old_body, new_body = exact_body(left, old), exact_body(right, new)
        if name not in PINS:
            require(old_body == new_body, 'Unrelated original body changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
            continue
        require(sha(old_body) == PINS[name], 'Original target pin differs: ' + name)
        require(old_body != new_body, 'Target body did not change: ' + name)
        changes.append(verify_target((left, right), name, old, new))
    require(engine == 547, 'Original engine body count differs')
    require(len(changes) == len(PINS), 'Target count differs')
    return dict(changes=changes, originalEngineMethodsUnchanged=engine,
                unrelatedRawMethodBodiesUnchanged=unchanged,
                originalMetadataABIAndTokensPreserved=True,
                originalAssemblyIdentityAndReferencesPreserved=True,
                addedAssemblyReference='SoulFlowerStaminaSweep (flags 0, empty PublicKeyToken)',
                cliFlagsPreserved=True, strongNameSignatureBytes=0)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, default=APK)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    require(sha(args.base_apk.read_bytes()) == APK_SHA, 'Base APK identity differs')
    out = args.output_directory.resolve()
    require(not out.exists(), 'Use a fresh output directory')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'

    def run(name, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=120)
        (out / (name + '.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + ' failed; inspect saved log')

    with zipfile.ZipFile(args.base_apk) as archive, tempfile.TemporaryDirectory(prefix='hyjs-stamina-sweep-') as temporary:
        work = Path(temporary)
        before = archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
        require(sha(before) == INPUT_SHA, 'Current managed input required')
        host = work / 'Assembly-CSharp.dll'
        host.write_bytes(before)
        references = [host]
        for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll']:
            target = work / name
            target.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
            references.append(target)
        helper = out / 'SoulFlowerStaminaSweep.dll'
        run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
             '/deterministic+', '/out:' + str(helper), *['/reference:' + str(path) for path in references], SOURCE / 'Stamina.cs'])
        shutil.copy2(SOURCE / 'Injector.cs', work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        run('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', host, out / 'Assembly-CSharp.dll'])
    after = (out / 'Assembly-CSharp.dll').read_bytes()
    checks = verify(before, after)
    report = dict(
        schema='hyjs-stamina-purchase-and-sweep-stop-v1',
        status='STATIC_VERIFIED_NOT_PACKAGED',
        inputAPK=str(args.base_apk.relative_to(ROOT)),
        inputAPK_SHA256=APK_SHA,
        inputManagedSHA256=sha(before),
        outputManagedSHA256=sha(after),
        helperSHA256=sha(helper.read_bytes()),
        replacementEntries=['assets/bin/Data/Managed/Assembly-CSharp.dll',
                            'assets/bin/Data/Managed/SoulFlowerStaminaSweep.dll'],
        deviceExecuted=False,
        visualVerified=False,
        resourceFilesModified=False,
        targetPins=PINS,
        sourcePins={str(path.relative_to(ROOT)): sha(path.read_bytes())
                    for path in [SOURCE / 'Stamina.cs', SOURCE / 'Injector.cs', Path(__file__).resolve()]},
        knownLimits=[
            'BuyItemPage reuse through Page.Reopen does not run SetItem for another item; the overlay only renders when SetItem runs.',
            'Pending requests are bounded at 128; reaching capacity stops new sends and retains every submitted request until its callback.',
            'Visual placement and original-font readability require the Root device screenshot check.',
        ],
        **checks)
    (out / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({k: report[k] for k in ['status', 'outputManagedSHA256', 'helperSHA256',
                                              'originalEngineMethodsUnchanged', 'unrelatedRawMethodBodiesUnchanged']},
                     ensure_ascii=False))


if __name__ == '__main__':
    main()
