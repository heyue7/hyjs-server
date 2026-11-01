"""Build and statically verify the fixed 1.1.2 chest overlay; no APK or device operations."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require
from tools.client_relations85.verify_existing import verify_metadata
from tools.client_fragment_boxes85.build import assembly_identity

APK_SHA = '408e9d03291d6f3505fb91590134b25a8b6aa68f3ebde9c2f4ff985299baccb3'
MAIN_SHA = '3d8f50e543ad082ea03bff294f17421a2eed4316d5cda664b03cc0f46cc23e8a'
ENTRY = 'assets/bin/Data/Managed/'
MAIN = ENTRY + 'Assembly-CSharp.dll'
HELPER = ENTRY + 'HYJS.CopyBoxSync.dll'
PINS = {
    'Room.RCUIBox::CopyOpenBox': (0x4571, '3baf463efd62b4538f2763abcd51177dfef061b8f5d1d73936bbdbce85aa2239'),
    'Room.RoomCopy::Exit': (0x44a3, 'ee390fb8cae828e16683e463a9dc5670686ef8d0ce911c9ac0e6e03118f87ff4'),
    'NetAPI::SendHttp': (0x3df4, '936753cd38339a2cc8f59855df331c2d5e963303f86113144921060f2d8b6c08'),
}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def digest(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def operand(value):
    return value.value if hasattr(value, 'value') else value.index if hasattr(value, 'index') else value


def normalized(body, rows):
    positions = {row.offset: number for number, row in enumerate(rows)}
    positions[body.header_size + body.code_size] = len(rows)
    output = []
    for row in rows:
        value = row.operand
        if row.opcode.flow_control.name in ('Branch', 'Cond_Branch'):
            value = [positions[x] for x in value] if isinstance(value, list) else positions[value]
        else:
            value = operand(value)
        output.append((row.opcode.name, value))
    return output


def hook(pe, instruction, expected):
    require(instruction.opcode.name == 'call' and operand(instruction.operand) >> 24 == 10, 'hook call differs')
    row = pe.net.mdtables.MemberRef.rows[(operand(instruction.operand) & 0xffffff) - 1]
    owner = row.Class.row
    require(str(row.Name) == expected and str(owner.TypeNamespace) == 'HYJS.CopyBoxSync' and
            str(owner.TypeName) == 'Runtime' and owner.ResolutionScope.table.name == 'AssemblyRef' and
            str(owner.ResolutionScope.row.Name) == 'HYJS.CopyBoxSync', 'hook target differs')
    handler = next(index for index, item in enumerate(pe.net.mdtables.TypeDef.rows, 1)
                   if str(item.TypeNamespace) == 'NetWork.Layer' and str(item.TypeName) == 'HttpHandler')
    index = handler << 2
    if index < 0x80:
        encoded = bytes([index])
    elif index < 0x4000:
        encoded = bytes([(index >> 8) | 0x80, index & 0xff])
    else:
        encoded = bytes([(index >> 24) | 0xc0, (index >> 16) & 0xff, (index >> 8) & 0xff, index & 0xff])
    handler_type = b'\x12' + encoded
    signatures = {'WrapOpen': b'\x00\x01' + handler_type + handler_type,
                  'WrapLeave': b'\x00\x02' + handler_type + handler_type + b'\x1c',
                  'ObserveSend': b'\x00\x02\x01\x08\x1c'}
    require(row.Signature.value == signatures[expected], 'hook signature or native callback scope differs')


def verify_helper_scope(main, helper):
    host, bridge = (dnfile.dnPE(data=value) for value in (main, helper))
    native = {(str(row.TypeNamespace), str(row.TypeName)) for row in host.net.mdtables.TypeDef.rows}
    matched = []
    for row in bridge.net.mdtables.TypeRef.rows:
        name = (str(row.TypeNamespace), str(row.TypeName))
        if name not in native:
            continue
        require(row.ResolutionScope.table.name == 'AssemblyRef' and
                str(row.ResolutionScope.row.Name) == 'Assembly-CSharp', 'helper native type scope differs: ' + '.'.join(name))
        matched.append('.'.join(name))
    require('NetWork.Layer.HttpHandler' in matched and 'UI.UIPlayerData' in matched, 'helper native binding missing')
    return {'helperMainAssemblyTypeRefsVerified': len(matched), 'nativeCallbackTypeDefBound': True}


def verify(before, after):
    old, new = [dnfile.dnPE(data=raw) for raw in (before, after)]
    require(not getattr(new.net.mdtables, 'FieldPtr', None) and not getattr(new.net.mdtables, 'MethodPtr', None), 'Unity pointer tables forbidden')
    for table in ('TypeDef', 'Field', 'MethodDef', 'Param'):
        require(len(getattr(old.net.mdtables, table).rows) == len(getattr(new.net.mdtables, table).rows), 'definition inventory differs: ' + table)
    require(old.net.mdtables.Module.rows[0].Mvid.value == new.net.mdtables.Module.rows[0].Mvid.value, 'MVID differs')
    require([assembly_identity(x) for x in old.net.mdtables.Assembly.rows] == [assembly_identity(x) for x in new.net.mdtables.Assembly.rows], 'assembly identity differs')
    a, b = old.net.mdtables.AssemblyRef.rows, new.net.mdtables.AssemblyRef.rows
    require(len(b) == len(a) + 1 and [assembly_identity(x) for x in a] == [assembly_identity(x) for x in b[:-1]] and str(b[-1].Name) == 'HYJS.CopyBoxSync', 'assembly references differ')
    for table, attrs in (('Field', ('Name', 'Signature')), ('TypeRef', ('TypeName', 'TypeNamespace')), ('MemberRef', ('Name', 'Signature'))):
        a, b = (getattr(pe.net.mdtables, table).rows for pe in (old, new))
        require(len(b) >= len(a), 'metadata rows removed: ' + table)
        for x, y in zip(a, b):
            for attr in attrs:
                left, right = getattr(x, attr), getattr(y, attr)
                require((left.value if hasattr(left, 'value') else str(left)) == (right.value if hasattr(right, 'value') else str(right)), 'metadata value differs: ' + table)
            if table == 'TypeRef':
                require((x.ResolutionScope.table.name, x.ResolutionScope.row_index) == (y.ResolutionScope.table.name, y.ResolutionScope.row_index), 'type reference scope differs')
            elif table == 'MemberRef':
                require((x.Class.table.name, x.Class.row_index) == (y.Class.table.name, y.Class.row_index), 'member reference scope differs')
    changes, unchanged, engines = [], 0, 0
    left, right = methods(old), methods(new)
    require(len(left) == len(right), 'method inventory differs')
    for (name, rid, a), (other, other_rid, b) in zip(left, right):
        require((name, rid, a.Signature.value, a.struct.Flags, a.struct.ImplFlags) ==
                (other, other_rid, b.Signature.value, b.struct.Flags, b.struct.ImplFlags), 'method ABI differs: ' + name)
        if not a.Rva:
            require(not b.Rva, 'bodyless method changed')
            continue
        x, y = exact_body(old, a), exact_body(new, b)
        if name not in PINS:
            require(x == y, 'unrelated method changed: ' + name)
            unchanged += 1
            engines += name.startswith('Battle.Engine')
            continue
        expected_rid, expected_hash = PINS[name]
        require(rid == expected_rid and sha(x) == expected_hash, 'input method pin differs')
        old_body, new_body = (read_method_body_from_bytes(v) for v in (x, y))
        require(not old_body.exception_handlers and not new_body.exception_handlers and
                getattr(old_body.local_var_sig_tok, 'value', None) == getattr(new_body.local_var_sig_tok, 'value', None), 'target locals/exception regions differ')
        rows = list(new_body.instructions)
        if name == 'NetAPI::SendHttp':
            require([i.opcode.name for i in rows[:3]] == ['ldarg.0', 'ldarg.1', 'call'], 'observation hook differs')
            hook(new, rows[2], 'ObserveSend')
            rows = rows[3:]
        else:
            call = next(i for i, row in enumerate(rows) if row.opcode.name == 'call' and operand(row.operand) == 0x06003df4)
            if name == 'Room.RCUIBox::CopyOpenBox':
                hook(new, rows[call - 1], 'WrapOpen')
                rows = rows[:call - 1] + rows[call:]
            else:
                require(rows[call - 2].opcode.name == 'ldarg.0', 'exit owner argument differs')
                hook(new, rows[call - 1], 'WrapLeave')
                rows = rows[:call - 2] + rows[call:]
        require(normalized(old_body, old_body.instructions) == normalized(new_body, rows), 'original instruction flow differs: ' + name)
        changes.append({'method': name, 'token': hex(0x06000000 | rid), 'beforeSHA256': sha(x), 'afterSHA256': sha(y)})
    require(len(changes) == 3 and engines == 547, 'target or engine count differs')
    return {'changedMethods': changes, 'unchangedMethodBodies': unchanged, 'originalEngineMethodsExact': engines,
            'originalMethodInstructionsAndBranchesExact': True, 'originalMethodABIExact': True, **verify_metadata(before, after)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    base, output = args.base_apk.resolve(), args.output_directory.resolve()
    require(output.is_relative_to(HERE / '.build') and not output.exists(), 'fresh owned ignored output required')
    require(digest(base) == APK_SHA, 'fixed 1.1.2 APK differs')
    sources = [HERE / name for name in ('Runtime.cs', 'Injector.cs', 'ModelTests.cs', 'build.py', '.gitignore', 'README.md')]
    pins = {str(path.relative_to(ROOT)): digest(path) for path in sources}
    output.mkdir(parents=True)
    sdk = Path('/Users/lirui/LR/hyjs/.tools/native-batch')
    dotnet = sdk / 'dotnet/dotnet'
    compiler = next((sdk / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = sdk / 'injector/bin/Debug/net8.0/dnlib.dll'
    ref = sorted((sdk / 'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))[-1]
    modern = ['/reference:' + str(path) for path in sorted(ref.glob('*.dll'))]
    runtime_version = ref.parents[1].name
    def run(label, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, text=True, capture_output=True, timeout=120)
        (output / (label + '.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, label + ' failed; inspect task-owned log')
        if result.stdout.strip(): print(result.stdout.strip(), flush=True)
    with tempfile.TemporaryDirectory(prefix='hyjs-copybox-sync-') as temporary, zipfile.ZipFile(base) as archive:
        work = Path(temporary)
        require(len(archive.namelist()) == len(set(archive.namelist())), 'duplicate APK entries')
        original = archive.read(MAIN)
        require(sha(original) == MAIN_SHA, 'fixed managed input differs')
        for name in ('Assembly-CSharp.dll', 'Assembly-CSharp-firstpass.dll', 'mscorlib.dll', 'System.dll', 'System.Core.dll', 'UnityEngine.dll'):
            (work / name).write_bytes(archive.read(ENTRY + name))
        refs = ['/reference:' + str(work / name) for name in ('Assembly-CSharp.dll', 'Assembly-CSharp-firstpass.dll', 'mscorlib.dll', 'System.dll', 'System.Core.dll', 'UnityEngine.dll')]
        helper = output / 'HYJS.CopyBoxSync.dll'
        run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(helper), *refs, HERE / 'Runtime.cs'])
        for name in ('Injector', 'ModelTests'):
            (work / (name + '.runtimeconfig.json')).write_text(json.dumps({'runtimeOptions': {'tfm': 'net8.0', 'framework': {'name': 'Microsoft.NETCore.App', 'version': runtime_version}}}))
        shutil.copy2(dnlib, work / 'dnlib.dll')
        injector = work / 'Injector.dll'
        run('compile-injector', [dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/out:' + str(injector), *modern, '/reference:' + str(dnlib), HERE / 'Injector.cs'])
        result = output / 'Assembly-CSharp.dll'
        run('inject', [dotnet, injector, work / 'Assembly-CSharp.dll', helper, result])
        tests = work / 'ModelTests.dll'
        run('compile-model', [dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3', '/out:' + str(tests), *modern, HERE / 'Runtime.cs', HERE / 'ModelTests.cs'])
        run('model-tests', [dotnet, tests])
        checks = {**verify(original, result.read_bytes()), **verify_helper_scope(original, helper.read_bytes())}
        dex_sha = sha(archive.read('classes2.dex'))
    require(digest(base) == APK_SHA and pins == {str(path.relative_to(ROOT)): digest(path) for path in sources}, 'input changed during build')
    payload = {MAIN: str(result.relative_to(ROOT)), HELPER: str(helper.relative_to(ROOT))}
    report = {'schema': 'hyjs-copybox-sync-112-managed-v1', 'status': 'PARTIAL_INSTANCE_ONLY_MODEL_AND_STATIC_VERIFIED_NOT_DEVICE_TESTED',
              'baseAPKSHA256': APK_SHA, 'inputManagedSHA256': MAIN_SHA, 'inputDexSHA256': dex_sha,
              'managedOnly': True, 'inheritedJavaByteExact': True, 'inheritedJavaPublicProtectedABIExact': True,
              'stackableInventorySync': 'DISABLED_UNVERSIONED', 'uniqueInstanceSync': 'EXACT_CAPTURED_GUIDS_ONLY',
              'maximumFailuresPerDrain': 3,
              'allowedChangedMethods': list(PINS), 'payloadFiles': payload,
              'replacementEntries': {name: digest(ROOT / path) for name, path in payload.items()}, 'sourcePins': pins, **checks}
    (output / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'status': report['status'], 'unchangedMethods': checks['unchangedMethodBodies'],
                      'originalEngineMethods': checks['originalEngineMethodsExact'], 'payload': report['replacementEntries']}))


if __name__ == '__main__':
    main()
