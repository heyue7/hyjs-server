"""Create a semantic, managed-only sweep pacing overlay; never package an APK."""
import argparse
import hashlib
import json
import struct
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods, operand
from tools.local_asset_ready.remove_gate import exact_body, require

ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
REFERENCE_APK = '25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
REFERENCE_MAIN = '8b732cce869cad9a9a06c75d3ba4891b4084a3ac646848fc99a933148e300437'
PVP_MOVE = '<SweepNext>c__IteratorF4::MoveNext'
COPY_MOVE = '<_StartSweep>c__Iterator141::MoveNext'
PVP_ROW = 'UI.PVPSweepPage::CreateItem'
COPY_ROW = 'UI.UICopySweep::Refresh'
TARGETS = {PVP_MOVE, COPY_MOVE, PVP_ROW, COPY_ROW}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def file_sha(path):
    with path.open('rb') as f:
        return hashlib.file_digest(f, 'sha256').hexdigest()


def token_name(pe, value):
    token = getattr(value, 'value', 0)
    table = {0x04: 'Field', 0x06: 'MethodDef', 0x0a: 'MemberRef'}.get(token >> 24)
    return str(getattr(pe.net.mdtables, table).rows[(token & 0xffffff)-1].Name) if table else ''


def exactly_one(indexes, message):
    require(len(indexes) == 1, message)
    return indexes[0]


def needle(pe, name, code):
    if name in (PVP_MOVE, COPY_MOVE):
        wait = exactly_one([i for i, x in enumerate(code) if x.opcode.name == 'newobj' and token_name(pe, x.operand) == '.ctor'
                            and (getattr(x.operand, 'value', 0) == 0x0a000940)], 'wait constructor differs: ' + name)
        if name == PVP_MOVE:
            require(code[wait-1].opcode.name == 'ldc.r4' and code[wait-1].operand == .5, 'arena wait differs')
            return wait-1
        expected = [('ldarg.0', ''), ('ldfld', '<>f__this'), ('ldfld', 'titleAlpha'), ('ldfld', 'duration'),
                    ('ldarg.0', ''), ('ldfld', '<>f__this'), ('ldfld', 'bodyScale'), ('ldfld', 'duration'), ('add', '')]
        require([(x.opcode.name, token_name(pe, x.operand)) for x in code[wait-9:wait]] == expected, 'copy wait expression differs')
        return wait
    call_name = 'Begin' if name == PVP_ROW else 'MoveUpPosition'
    call = exactly_one([i for i, x in enumerate(code) if x.opcode.name == 'call' and token_name(pe, x.operand) == call_name], 'scroll call differs')
    index = call - (2 if name == PVP_ROW else 1)
    require(code[index].opcode.name == 'ldc.r4' and struct.pack('<f', code[index].operand) == struct.pack('<f', .2), 'scroll duration differs')
    return index


def normalized_operand(i, offsets):
    if i.opcode.name == 'switch':
        return [offsets[x] for x in i.operand]
    if i.opcode.flow_control.name in ('Branch', 'Cond_Branch'):
        return offsets[i.operand]
    x = operand(i.operand)
    return struct.pack('<f', x) if i.opcode.name == 'ldc.r4' else x


def verify_target(oldpe, newpe, name, old, new):
    a, b = [read_method_body_from_bytes(exact_body(pe, row)) for pe, row in ((oldpe, old), (newpe, new))]
    require(not a.exception_handlers and not b.exception_handlers, 'target EH changed: ' + name)
    ac, bc = a.instructions, b.instructions
    at = needle(oldpe, name, ac)
    if name == COPY_MOVE:
        require(len(bc) == len(ac)+2 and [x.opcode.name for x in bc[at:at+2]] == ['pop', 'ldc.r4'] and
                struct.pack('<f', bc[at+1].operand) == struct.pack('<f', .05), 'copy insertion differs')
        mapped = list(range(at)) + list(range(at+2, len(bc)))
    else:
        require(len(ac) == len(bc), 'target length changed: ' + name)
        mapped = list(range(len(bc)))
    old_offsets = {x.offset: i for i, x in enumerate(ac)}
    new_offsets = {bc[new_i].offset: old_i for old_i, new_i in enumerate(mapped)}
    for ix, mapped_ix in enumerate(mapped):
        x, y = ac[ix], bc[mapped_ix]
        require(x.opcode.name == y.opcode.name, 'original opcode changed: ' + name + ' ' + str(ix))
        if ix == at and name != COPY_MOVE:
            desired = .05 if name == PVP_MOVE else 0.0
            require(y.opcode.name == 'ldc.r4' and struct.pack('<f', y.operand) == struct.pack('<f', desired), 'patched value differs: ' + name)
        else:
            require(normalized_operand(x, old_offsets) == normalized_operand(y, new_offsets), 'original operand or branch changed: ' + name + ' ' + str(ix))
    return {'method': name, 'token': hex(0x06000000 | old.row_id) if hasattr(old, 'row_id') else None,
            'inputBodySHA256': sha(exact_body(oldpe, old)), 'outputBodySHA256': sha(exact_body(newpe, new)),
            'originalInstructionsPreservedExceptDeclaredConstants': True, 'allOriginalBranchTargetsAndTokensPreserved': True,
            'insertedInstructions': 2 if name == COPY_MOVE else 0}


def verify(before, after):
    left, right = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    old_methods, new_methods = methods(left), methods(right)
    require(len(old_methods) == len(new_methods), 'method inventory changed')
    for table in ('TypeDef', 'Field', 'MethodDef', 'Param', 'TypeRef', 'MemberRef', 'AssemblyRef'):
        require(len(getattr(left.net.mdtables, table).rows) == len(getattr(right.net.mdtables, table).rows), 'metadata inventory changed: ' + table)
    require(not getattr(right.net.mdtables, 'FieldPtr', None) and not getattr(right.net.mdtables, 'MethodPtr', None), 'Unity pointer table added')
    for a, b in zip(left.net.mdtables.Field.rows, right.net.mdtables.Field.rows):
        require((str(a.Name), a.Signature.value, a.struct.Flags) == (str(b.Name), b.Signature.value, b.struct.Flags), 'field ABI changed')
    for a, b in zip(left.net.mdtables.TypeDef.rows, right.net.mdtables.TypeDef.rows):
        require((str(a.TypeName), str(a.TypeNamespace), a.struct.Flags, a.struct.Extends_CodedIndex, a.struct.FieldList_Index, a.struct.MethodList_Index) ==
                (str(b.TypeName), str(b.TypeNamespace), b.struct.Flags, b.struct.Extends_CodedIndex, b.struct.FieldList_Index, b.struct.MethodList_Index), 'type ABI changed')
    for table, attrs in (('AssemblyRef', ('Name', 'Flags', 'PublicKey')), ('MemberRef', ('Name', 'Signature')), ('TypeRef', ('TypeName', 'TypeNamespace'))):
        for a, b in zip(getattr(left.net.mdtables, table).rows, getattr(right.net.mdtables, table).rows):
            for attr in attrs:
                x, y = getattr(a, attr), getattr(b, attr)
                require((x.value if hasattr(x, 'value') else str(x)) == (y.value if hasattr(y, 'value') else str(y)), 'reference changed: ' + table + '.' + attr)
    changes, unchanged, engine = [], 0, 0
    for (name, rid, a), (name2, rid2, b) in zip(old_methods, new_methods):
        require((name, rid, a.Signature.value, a.struct.Flags, a.struct.ImplFlags) == (name2, rid2, b.Signature.value, b.struct.Flags, b.struct.ImplFlags), 'method ABI/token changed: ' + name)
        if not a.Rva:
            require(not b.Rva, 'body added: ' + name)
            continue
        first, second = exact_body(left, a), exact_body(right, b)
        if name in TARGETS:
            record = verify_target(left, right, name, a, b)
            record['token'] = hex(0x06000000 | rid)
            changes.append(record)
        else:
            require(first == second, 'unrelated raw method body changed: ' + name)
            unchanged += 1
        if name.startswith('Battle.Engine'):
            require(first == second, 'battle engine changed: ' + name)
            engine += 1
    require({x['method'] for x in changes} == TARGETS and engine == 547, 'target/engine inventory mismatch')
    return {'allowedChangedMethods': changes, 'allOtherOriginalBodiesByteExact': True,
            'unchangedBodyCount': unchanged, 'originalEngineMethodsExact': engine, 'methodTokensABIPreserved': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', required=True, type=Path, help='Packaging base; may be the shipped 80 APK')
    parser.add_argument('--base-sha256', required=True, help='Exact SHA-256 of the passed base APK')
    parser.add_argument('--input-managed', type=Path, help='Already-composed Assembly-CSharp; omit to read base APK')
    parser.add_argument('--input-managed-sha256', required=True, help='Exact SHA-256 of the composed input')
    parser.add_argument('--output-directory', required=True, type=Path)
    parser.add_argument('--tool-root', default='/Users/lirui/LR/hyjs', type=Path)
    args = parser.parse_args()
    apk, out = args.base_apk.resolve(), args.output_directory.resolve()
    require(file_sha(apk) == args.base_sha256.lower(), 'base APK SHA mismatch')
    require(out.is_relative_to(ROOT/'tools/client_sweep84') or out.is_relative_to(ROOT/'analysis/feedback84/sweep-client'), 'output must be task-owned')
    require(not out.exists(), 'fresh output directory required')
    with zipfile.ZipFile(apk) as z:
        require(z.namelist().count(ENTRY) == 1, 'ambiguous base assembly entry')
        before = args.input_managed.read_bytes() if args.input_managed else z.read(ENTRY)
    require(sha(before) == args.input_managed_sha256.lower(), 'input managed SHA mismatch')
    dot = args.tool_root / '.tools/native-batch/dotnet/dotnet'
    dnlib = args.tool_root / '.tools/native-batch/injector/bin/Debug/net8.0/dnlib.dll'
    out.mkdir(parents=True)

    def run(label, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=120)
        (out/(label+'.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, label + ' failed; inspect ' + str(out/(label+'.log')))

    with tempfile.TemporaryDirectory(prefix='hyjs-sweep84-') as temporary:
        work = Path(temporary)
        (work/'Input.dll').write_bytes(before)
        (work/'Program.cs').write_text((Path(__file__).with_name('Injector.cs')).read_text())
        project = work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run('compile', [dot, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        output = out/'Assembly-CSharp.dll'
        run('inject', [dot, work/'bin/Debug/net8.0/Patch.dll', work/'Input.dll', output])
        checks = verify(before, output.read_bytes())
    require(file_sha(apk) == args.base_sha256.lower(), 'input APK changed during build')
    if args.input_managed:
        require(file_sha(args.input_managed) == args.input_managed_sha256.lower(), 'input managed changed during build')
    sources = [Path(__file__).resolve(), Path(__file__).with_name('Injector.cs').resolve(), Path(__file__).with_name('dump_il.py').resolve()]
    manifest = {'schema':'hyjs-sweep84-overlay-v1', 'status':'STATIC_VERIFIED_NOT_PACKAGED',
                'reference80APKSHA256': REFERENCE_APK, 'reference80ManagedSHA256': REFERENCE_MAIN,
                'baseAPKSHA256':args.base_sha256.lower(), 'inputManagedSHA256':sha(before),
                'outputManagedSHA256':file_sha(output), 'replacementEntries':{ENTRY:file_sha(output)},
                'payloadFiles':{ENTRY:str(output)}, 'noNewRuntimeAssembly':True,
                'deviceTested':False, 'networkTimingMeasured':False,
                'sourcePins':{str(p.relative_to(ROOT)):file_sha(p) for p in sources}, **checks}
    (out/'manifest.json').write_text(json.dumps(manifest, indent=2, ensure_ascii=False)+'\n')
    print(json.dumps({'status':manifest['status'], 'outputManagedSHA256':manifest['outputManagedSHA256'],
                      'unchangedMethods':checks['unchangedBodyCount'], 'engineMethods':checks['originalEngineMethodsExact']}))


if __name__ == '__main__':
    main()
