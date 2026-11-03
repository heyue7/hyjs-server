"""Build a pinned, statically verified 1.1.7 managed overlay without packaging."""
import hashlib
import json
import shutil
import struct
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools.awake_diff_rows.build import methods, operand
from tools.local_asset_ready.remove_gate import exact_body, require
from tools.client_relations85.verify_existing import verify_metadata

APK = Path('/Users/lirui/LR/hyjs/deliveries/client-update-1.1.7/黑衣剑士灵魂之花v1.1.7.apk')
APK_SHA = '191fe4826ad162d508b2d3fc8e0f8e6b6c5372815ad9f82cb21e7160bea17aa3'
MAIN_SHA = '0e0fb0e88cc295877b5c4c74e3bfe3280e12bf0b7aa10e6521a69ad998fdbde1'
DEX_SHA = '131bacb3847eda65274c4111fbbd7cf02d5e437b5363f414210828f6d03fa143'
ENTRY = 'assets/bin/Data/Managed/'
MAIN = ENTRY + 'Assembly-CSharp.dll'
HELPER = ENTRY + 'HYJS.Ranks.dll'
OUT = ROOT / 'build/leaderboard-refresh-20261103/client'
RUNTIME = Path('/Users/lirui/LR/hyjs/.tools/native-batch')
HTTP_TOKEN = 0x06003df4

# Names, RIDs, counts and body hashes are bound to the fixed APK.
CALLS = {
    'UI.RankPage::Refresh': (0x51ec, 10, '74ecb507c8d065d245c59ba4953a86ea9dac875c291789f4bb4c10ea3daf2084'),
    'UI.PowerPage::getPowerRank': (0x4e86, 1, '059c3e6ad721a911a2d3d3017ea8f2690c943750251f09ac1708642b83ef40e1'),
    'UI.LevelPage::getPowerRank': (0x4e6f, 1, '5feda9132ec4c0777ccc6ba3bde45dd6fdc2d9f74708d18bcdfc5feac27df103'),
    'UI.UnionRankPage::RefreshPage': (0x5652, 1, 'd1770155afdd0a7c6603e3811057b12a4b677080c7b9aa654a0afb9444c6e51e'),
    'UI.UnionFightWeekRankPage::OnTabBtn': (0x554c, 4, '64bd2f62dac0be6242f1a7d1b14cd7d71c7923c8d18972207ab3f38dd3d2169c'),
    'UI.ServerBattlePromotion::UpdatePlayerList': (0x58fe, 1, 'a7b4b85f6b071351e1cb4be57409d4e9f553841c2ee0d7a4b407dbd195cba5b0'),
    'UI.ServerBattleRankHead::UpDateServerBattleList': (0x521c, 1, '041147daef89f16dbcf817b02a41fa44e0a62b2c33eca67d555fb67a8fbb4f0c'),
}
WORSHIP = {
    'UI.LevelPage::CSGoodCallback': (0x4e7b, 'd89472b3adebf539175324f4b42821c9cb2d1901c8ffb22e6850ad5211ac60f1'),
    'UI.PowerPage::CSGoodCallback': (0x4e92, '8b3230ac8a9be2376b2406b2c539c8c1cbce272429d250a66220ac383351161c'),
}
TARGETS = set(CALLS) | set(WORSHIP)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def file_sha(path):
    with path.open('rb') as file:
        return hashlib.file_digest(file, 'sha256').hexdigest()


def assembly_identity(row):
    return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion,
            row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)


def compressed(number):
    if number < 0x80:
        return bytes([number])
    if number < 0x4000:
        return bytes([(number >> 8) | 0x80, number & 255])
    require(number < 0x20000000, 'Encoded type index too large')
    return bytes([(number >> 24) | 0xc0, (number >> 16) & 255, (number >> 8) & 255, number & 255])


def hook(pe, instruction, name, signature):
    token = operand(instruction.operand)
    require(instruction.opcode.name == 'call' and isinstance(token, int) and token >> 24 == 0x0a,
            'Expected helper member reference')
    row = pe.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1]
    owner = row.Class.row
    require(str(row.Name) == name and str(owner.TypeNamespace) == 'HYJS.Ranks' and str(owner.TypeName) == 'Refresh' and
            owner.ResolutionScope.table.name == 'AssemblyRef' and str(owner.ResolutionScope.row.Name) == 'HYJS.Ranks' and
            row.Signature.value == signature, 'Helper owner or signature differs: ' + name)
    return token


def eh_shape(body, shift):
    result = []
    for row in body.exception_handlers:
        def adjust(value):
            return value - shift if value >= 0 else value
        result.append((row.exception_type, adjust(row.try_start), adjust(row.try_end),
                       adjust(row.handler_start), adjust(row.handler_end), adjust(row.filter_start),
                       getattr(row.catch_type, 'value', row.catch_type)))
    return result


def helper_scope(main, helper):
    host, bridge = (dnfile.dnPE(data=data) for data in (main, helper))
    native = {(str(row.TypeNamespace), str(row.TypeName)) for row in host.net.mdtables.TypeDef.rows}
    matched = []
    for row in bridge.net.mdtables.TypeRef.rows:
        name = (str(row.TypeNamespace), str(row.TypeName))
        if name in native:
            require(row.ResolutionScope.table.name == 'AssemblyRef' and
                    str(row.ResolutionScope.row.Name) == 'Assembly-CSharp', 'Helper native type scope differs: ' + '.'.join(name))
            matched.append('.'.join(name))
    require({'NetWork.Layer.HttpHandler', 'NetWork.Packets.Packet', 'UI.UIPlayerData', 'UI.Page',
             'ProtoBuf.Message.CSUnionSearch', 'ProtoBuf.Message.SCGood'} <= set(matched), 'Required helper type binding missing')
    return {'nativeTypeReferencesVerified': len(matched), 'nativeTypesBoundToOriginalMain': True}


def verify(before, after, helper):
    a, b = (dnfile.dnPE(data=data) for data in (before, after))
    for table in ('FieldPtr', 'MethodPtr'):
        require(not getattr(b.net.mdtables, table, None), 'Unity pointer table added: ' + table)
    for table in ('TypeDef', 'Field', 'MethodDef', 'Param'):
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows), 'Definition inventory differs: ' + table)
    require(a.net.mdtables.Module.rows[0].Mvid.value == b.net.mdtables.Module.rows[0].Mvid.value, 'MVID changed')
    require([assembly_identity(x) for x in a.net.mdtables.Assembly.rows] == [assembly_identity(x) for x in b.net.mdtables.Assembly.rows], 'Assembly identity changed')
    old_refs, new_refs = a.net.mdtables.AssemblyRef.rows, b.net.mdtables.AssemblyRef.rows
    require(len(new_refs) == len(old_refs) + 1 and [assembly_identity(x) for x in old_refs] ==
            [assembly_identity(x) for x in new_refs[:-1]] and str(new_refs[-1].Name) == 'HYJS.Ranks' and
            new_refs[-1].struct.Flags == 0 and new_refs[-1].PublicKey.value == b'', 'Assembly references changed')
    for table, attrs in (('Field', ('Name', 'Signature')), ('TypeRef', ('TypeName', 'TypeNamespace')), ('MemberRef', ('Name', 'Signature'))):
        left, right = (getattr(pe.net.mdtables, table).rows for pe in (a, b))
        require(len(right) >= len(left), 'Metadata rows removed: ' + table)
        for x, y in zip(left, right):
            for attr in attrs:
                first, second = getattr(x, attr), getattr(y, attr)
                require((first.value if hasattr(first, 'value') else str(first)) ==
                        (second.value if hasattr(second, 'value') else str(second)), 'Metadata value changed: ' + table + '.' + attr)
            if table == 'TypeRef':
                require((x.ResolutionScope.table.name, x.ResolutionScope.row_index) == (y.ResolutionScope.table.name, y.ResolutionScope.row_index), 'TypeRef scope changed')
            if table == 'MemberRef':
                require((x.Class.table.name, x.Class.row_index) == (y.Class.table.name, y.Class.row_index), 'MemberRef scope changed')
    mono = next(i for i, r in enumerate(b.net.mdtables.TypeRef.rows, 1) if str(r.TypeNamespace) == 'UnityEngine' and str(r.TypeName) == 'MonoBehaviour')
    packet = next(i for i, r in enumerate(b.net.mdtables.TypeDef.rows, 1) if str(r.TypeNamespace) == 'NetWork.Packets' and str(r.TypeName) == 'Packet')
    worship_sig = b'\x00\x03\x01\x12' + compressed((mono << 2) | 1) + b'\x12' + compressed(packet << 2) + b'\x02'
    send_sig = a.net.mdtables.MethodDef.rows[(HTTP_TOKEN & 0xffffff) - 1].Signature.value
    original, changed = methods(a), methods(b)
    require(len(original) == len(changed), 'Method inventory differs')
    records = []
    unchanged = engine = substituted = 0
    for (name, rid, old), (name2, rid2, new) in zip(original, changed):
        require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) ==
                (name2, rid2, new.Signature.value, new.struct.Flags, new.struct.ImplFlags), 'Method identity/ABI differs: ' + name)
        if not old.Rva:
            require(not new.Rva, 'Bodyless method changed: ' + name)
            continue
        first, second = exact_body(a, old), exact_body(b, new)
        if name not in TARGETS:
            require(first == second, 'Unrelated original body changed: ' + name)
            unchanged += 1
            if name.startswith('Battle.Engine'):
                engine += 1
            continue
        old_body, new_body = (read_method_body_from_bytes(x) for x in (first, second))
        require(old_body.max_stack == new_body.max_stack and
                getattr(old_body.local_var_sig_tok, 'value', None) == getattr(new_body.local_var_sig_tok, 'value', None), 'Target stack/locals changed: ' + name)
        if name in CALLS:
            expected_rid, expected_count, pin = CALLS[name]
            require(rid == expected_rid and sha(first) == pin, 'Target input pin differs: ' + name)
            require(not old_body.exception_handlers and not new_body.exception_handlers and len(old_body.instructions) == len(new_body.instructions), 'Target shape differs: ' + name)
            expected = bytearray(first)
            indexes = [i for i, x in enumerate(old_body.instructions) if x.opcode.name == 'call' and operand(x.operand) == HTTP_TOKEN]
            require(len(indexes) == expected_count, 'Target call count differs: ' + name)
            for index in indexes:
                old_instruction, new_instruction = old_body.instructions[index], new_body.instructions[index]
                require(old_instruction.offset == new_instruction.offset, 'Call offset shifted: ' + name)
                token = hook(b, new_instruction, 'Send', send_sig)
                struct.pack_into('<I', expected, old_instruction.offset + 1, token)
            require(bytes(expected) == second, 'Original body changed beyond specified call operands: ' + name)
            substituted += len(indexes)
            detail = {'substitutedSendCalls': len(indexes), 'originalBodyExactExceptCallTokens': True}
        else:
            expected_rid, pin = WORSHIP[name]
            require(rid == expected_rid and sha(first) == pin, 'Worship input pin differs: ' + name)
            require(len(old_body.exception_handlers) == len(new_body.exception_handlers) == 1 and
                    [x.opcode.name for x in new_body.instructions[:4]] == ['ldarg.0', 'ldarg.1', 'ldarg.2', 'call'], 'Worship entry shape differs: ' + name)
            hook(b, new_body.instructions[3], 'WorshipCompleted', worship_sig)
            old_code = first[old_body.header_size:old_body.header_size + old_body.code_size]
            new_code = second[new_body.header_size:new_body.header_size + new_body.code_size]
            require(len(new_code) == len(old_code) + 8 and new_code[:4] == b'\x02\x03\x04\x28' and new_code[8:] == old_code,
                    'Original worship IL suffix differs: ' + name)
            require(eh_shape(old_body, 0) == eh_shape(new_body, 8), 'Worship exception boundaries changed: ' + name)
            require(old_body.flags.InitLocals == new_body.flags.InitLocals and old_body.flags.MoreSects == new_body.flags.MoreSects,
                    'Worship body flags changed: ' + name)
            detail = {'entryPrefixInstructions': 4, 'originalILAndBranchesExact': True, 'originalExceptionRegionsPreserved': True}
        records.append({'method': name, 'token': hex(0x06000000 | rid), 'inputBodySHA256': sha(first),
                        'outputBodySHA256': sha(second), **detail})
    require({x['method'] for x in records} == TARGETS and substituted == 19 and engine == 547,
            'Expected nine methods, nineteen calls and 547 engine methods')
    return {'changedMethods': records, 'unchangedMethodBodies': unchanged, 'originalEngineMethodsExact': engine,
            'unrelatedRawMethodBodiesByteExact': True, 'methodNamesRIDsSignaturesAndFlagsExact': True,
            'substitutedSendCalls': substituted, 'worshipEntryHooks': 2, 'unityPointerTablesAbsent': True,
            **verify_metadata(before, after), **helper_scope(before, helper)}


def run(argv):
    result = subprocess.run(['rtk', 'proxy', *map(str, argv)], cwd=ROOT, capture_output=True, text=True, timeout=150)
    require(result.returncode == 0, 'Command failed: ' + str(argv[0]) + '\n' + result.stdout + result.stderr)
    return result.stdout.strip()


def main():
    require(not OUT.exists(), 'Fresh overlay output required')
    require(file_sha(APK) == APK_SHA, 'Pinned base APK differs')
    source_files = [HERE / x for x in ('Runtime.cs', 'RuntimeModels.cs', 'ModelTests.cs', 'run-model-tests.sh', 'Injector.cs', 'build.py')]
    source_files += [ROOT / x for x in ('tools/awake_diff_rows/build.py', 'tools/local_asset_ready/remove_gate.py',
                                      'tools/client_relations85/verify_existing.py')]
    pins = {str(path.relative_to(ROOT)): file_sha(path) for path in source_files}
    dotnet = RUNTIME / 'dotnet/dotnet'
    compiler = next((RUNTIME / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = RUNTIME / 'injector/bin/Debug/net8.0/dnlib.dll'
    ref = sorted((RUNTIME / 'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))[-1]
    modern = ['/reference:' + str(x) for x in sorted(ref.glob('*.dll'))]
    runtime_version = ref.parents[1].name
    with tempfile.TemporaryDirectory(prefix='hyjs-rank-overlay-') as temporary, zipfile.ZipFile(APK) as archive:
        work = Path(temporary)
        require(len(archive.namelist()) == len(set(archive.namelist())), 'Duplicate APK entries')
        before = archive.read(MAIN)
        require(sha(before) == MAIN_SHA and sha(archive.read('classes2.dex')) == DEX_SHA, 'Pinned managed or DEX input differs')
        references = []
        for name in ('Assembly-CSharp.dll', 'Assembly-CSharp-firstpass.dll', 'mscorlib.dll', 'System.dll', 'System.Core.dll', 'UnityEngine.dll'):
            path = work / name
            path.write_bytes(archive.read(ENTRY + name))
            references.append('/reference:' + str(path))
        helper = work / 'HYJS.Ranks.dll'
        run([dotnet, compiler, '/nologo', '/noconfig', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+',
             '/out:' + str(helper), *references, HERE / 'Runtime.cs', HERE / 'RuntimeModels.cs'])
        for name in ('Injector', 'ModelTests'):
            (work / (name + '.runtimeconfig.json')).write_text(json.dumps({'runtimeOptions': {'tfm': 'net8.0', 'framework': {'name': 'Microsoft.NETCore.App', 'version': runtime_version}}}))
        shutil.copy2(dnlib, work / 'dnlib.dll')
        injector = work / 'Injector.dll'
        run([dotnet, compiler, '/nologo', '/noconfig', '/target:exe', '/nostdlib+', '/out:' + str(injector),
             *modern, '/reference:' + str(dnlib), HERE / 'Injector.cs'])
        patched = work / 'patched.dll'
        injection = run([dotnet, injector, work / 'Assembly-CSharp.dll', helper, patched])
        tests = work / 'ModelTests.dll'
        run([dotnet, compiler, '/nologo', '/noconfig', '/target:exe', '/nostdlib+', '/langversion:3', '/out:' + str(tests),
             *modern, HERE / 'Runtime.cs', HERE / 'RuntimeModels.cs', HERE / 'ModelTests.cs'])
        model = run([dotnet, tests])
        require(model == 'Rank refresh model passed 60 assertions.', 'Model assertions differ')
        after, helper_bytes = patched.read_bytes(), helper.read_bytes()
        checks = verify(before, after, helper_bytes)
        require(file_sha(APK) == APK_SHA and pins == {str(path.relative_to(ROOT)): file_sha(path) for path in source_files}, 'Build inputs changed')
        OUT.mkdir(parents=True)
        try:
            (OUT / 'Assembly-CSharp.dll').write_bytes(after)
            (OUT / 'HYJS.Ranks.dll').write_bytes(helper_bytes)
            payload = {MAIN: str((OUT / 'Assembly-CSharp.dll').relative_to(ROOT)), HELPER: str((OUT / 'HYJS.Ranks.dll').relative_to(ROOT))}
            report = {'schema': 'hyjs-rank-refresh-managed-v1', 'status': 'STATIC_AND_MODEL_VERIFIED_NOT_PACKAGED',
                      'baseAPKSHA256': APK_SHA, 'inputManagedSHA256': MAIN_SHA, 'inputDexSHA256': DEX_SHA,
                      'managedOnly': True, 'inheritedJavaByteExact': True, 'inheritedJavaPublicProtectedABIExact': True,
                      'allowedChangedMethods': sorted(TARGETS), 'payloadFiles': payload,
                      'replacementEntries': {name: file_sha(ROOT / path) for name, path in payload.items()},
                      'sourcePins': pins, 'oldUnityProfileCSharp3Compiled': True, 'modelEvidence': model,
                      'injectorEvidence': injection, 'apkBuilt': False, 'deviceTested': False, **checks}
            (OUT / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        except Exception:
            shutil.rmtree(OUT)
            raise
    print(json.dumps({'status': report['status'], 'unchangedMethods': checks['unchangedMethodBodies'],
                      'engineMethods': checks['originalEngineMethodsExact'], 'calls': checks['substitutedSendCalls'],
                      'worshipEntries': checks['worshipEntryHooks'], 'payload': report['replacementEntries']}))


if __name__ == '__main__':
    main()
