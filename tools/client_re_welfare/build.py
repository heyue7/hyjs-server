"""Apply four pinned Re welfare UI methods to a composed main, without packaging an APK."""
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

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.local_asset_ready.remove_gate import exact_body, require
from tools.client_achievements.build import normalize

SOURCE = Path(__file__).resolve().parent
TARGETS = json.loads((SOURCE / 'target-pins.json').read_text())


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def helper_call(pe, token, name):
    require(token >> 24 == 10, 'Expected helper MemberRef')
    row = pe.net.mdtables.MemberRef.rows[(token & 0xffffff)-1]
    require(str(row.Name) == name and str(row.Class.row.TypeNamespace) == 'HYJS.ReWelfare' and str(row.Class.row.TypeName) == 'ReWelfareBridge', 'Unexpected helper ABI')


def verify(before, after):
    a, b = (dnfile.dnPE(data=raw) for raw in (before, after))
    for table in ['TypeDef', 'Field', 'MethodDef']:
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows), 'Original inventory changed: ' + table)
    require(a.net.mdtables.Module.rows[0].Mvid.value == b.net.mdtables.Module.rows[0].Mvid.value, 'MVID changed')
    for old, new in zip(a.net.mdtables.Field.rows, b.net.mdtables.Field.rows):
        require(str(old.Name) == str(new.Name) and old.Signature.value == new.Signature.value and old.struct.Flags == new.struct.Flags, 'Original field ABI changed')
    def identity(row):
        return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion, row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)
    require([identity(r) for r in a.net.mdtables.Assembly.rows] == [identity(r) for r in b.net.mdtables.Assembly.rows], 'Assembly identity changed')
    old_refs, new_refs = a.net.mdtables.AssemblyRef.rows, b.net.mdtables.AssemblyRef.rows
    require([identity(r) for r in old_refs] == [identity(r) for r in new_refs[:len(old_refs)]], 'Prior overlay references changed')
    require(len(new_refs) == len(old_refs)+1 and str(new_refs[-1].Name) == 'SoulFlowerReWelfare' and new_refs[-1].struct.Flags == 0 and new_refs[-1].PublicKey.value == b'', 'Wrong new helper identity')
    require(a.net.struct.Flags == b.net.struct.Flags and a.net.struct.StrongNameSignatureSize == b.net.struct.StrongNameSignatureSize == 0, 'CLI identity changed')
    changes = []
    unchanged = engine = 0
    for (name, rid, old), (new_name, new_rid, new) in zip(methods(a), methods(b)):
        require((name, rid) == (new_name, new_rid) and old.Signature.value == new.Signature.value and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags, 'Method ABI changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'Unexpected method body')
            continue
        original, output = exact_body(a, old), exact_body(b, new)
        if name not in TARGETS:
            require(original == output, 'Unrelated input method changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
            continue
        require(sha(original) == TARGETS[name], 'Pinned native target differs: ' + name)
        old_body, oc = decoded(a, old)
        new_body, nc = decoded(b, new)
        if name.endswith('::ButtonClickCallBack'):
            require(len(nc) == len(oc)+8 and [op for op, _ in nc[:8]] == ['ldarg.0', 'ldarg.0', 'ldfld', 'ldarg.1', 'ldarg.2', 'call', 'brtrue', 'ret'], 'Wrong cost/cache prefix')
            helper_call(b, nc[5][1], 'BeforeCatReply')
            require(nc[6][1] == 8, 'Wrong native callback entry')
            indexes = {i: i-8 for i in range(8, len(nc))}
            require(normalize(nc, indexes) == oc, 'Original callback logic changed')
        elif name.endswith('::OnRechargeBtn'):
            require(len(nc) == len(oc)+4 and [op for op, _ in nc[:4]] == ['ldarg.0', 'call', 'brfalse', 'ret'], 'Wrong navigation guard')
            helper_call(b, nc[1][1], 'NavigateContinued')
            require(nc[2][1] == 4, 'Wrong native button entry')
            indexes = {i: i-4 for i in range(4, len(nc))}
            require(normalize(nc, indexes) == oc, 'Original claim navigation changed')
        else:
            packet = name.endswith('::MsgCallback')
            added = 4 if packet else 3
            suffix = ['ldarg.0', 'ldarg.1', 'ldarg.2', 'call', 'ret'] if packet else ['ldarg.0', 'ldarg.1', 'call', 'ret']
            returns = sum(op == 'ret' for op, _ in oc)
            require(returns > 0 and len(nc) == len(oc)+added*returns, 'Unexpected native exits')
            indexes = {}
            normalized_code = list(nc)
            pos = 0
            for i, (op, value) in enumerate(oc):
                indexes[pos] = i
                if op == 'ret':
                    require([operation for operation, _ in nc[pos:pos+len(suffix)]] == suffix, 'Wrong per-return display suffix')
                    helper_call(b, nc[pos+len(suffix)-2][1], 'BindContinued' if packet else 'BindCat')
                    normalized_code[pos] = ('ret', None)
                    pos += len(suffix)
                else:
                    pos += 1
            require(pos == len(nc) and normalize(normalized_code, indexes) == oc, 'Original state/display body changed')
        require(len(old_body.exception_handlers) == len(new_body.exception_handlers), 'EH count changed')
        oi = {ins.offset-old_body.header_size: i for i, ins in enumerate(old_body.instructions)}
        ni = {ins.offset-new_body.header_size: indexes[i] for i, ins in enumerate(new_body.instructions) if i in indexes}
        oi[old_body.instructions[-1].offset+old_body.instructions[-1].size-old_body.header_size] = len(oc)
        ni[new_body.instructions[-1].offset+new_body.instructions[-1].size-new_body.header_size] = len(oc)
        for oe, ne in zip(old_body.exception_handlers, new_body.exception_handlers):
            require(oe.exception_type == ne.exception_type and oe.catch_type == ne.catch_type, 'EH type changed')
            for key in ['try_start', 'try_end', 'handler_start', 'handler_end']:
                require(oi[getattr(oe, key)] == ni[getattr(ne, key)], 'EH boundary changed')
        changes.append({'method': name, 'token': hex(0x6000000 | rid), 'beforeSHA256': sha(original), 'afterSHA256': sha(output)})
    require(engine == 547 and len(changes) == 4, 'Wrong target/engine count')
    return {'changes': changes, 'unrelatedInputRawMethodsUnchanged': unchanged, 'originalEngineMethodsUnchanged': engine, 'originalMetadataABIAndTokensPreserved': True, 'priorAssemblyReferencesPreserved': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, default=ROOT/'apks/build/hyjs-local.apk')
    parser.add_argument('--input-managed', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    before = args.input_managed.read_bytes()
    out = args.output_directory.resolve()
    require(not out.exists(), 'Fresh output directory required')
    out.mkdir(parents=True)
    runtime = ROOT/'.tools/native-batch'
    dotnet = runtime/'dotnet/dotnet'
    compiler = next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    def run(name, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=90)
        (out/(name+'.log')).write_text(result.stdout+result.stderr)
        require(result.returncode == 0, name+' failed; inspect saved log')
    with tempfile.TemporaryDirectory(prefix='hyjs-re-welfare-') as temporary:
        work = Path(temporary)
        host = work/'Assembly-CSharp.dll'
        host.write_bytes(before)
        refs = [host]
        with zipfile.ZipFile(args.base_apk) as archive:
            for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll']:
                path = work/name
                path.write_bytes(archive.read('assets/bin/Data/Managed/'+name))
                refs.append(path)
        bridge = out/'SoulFlowerReWelfare.dll'
        run('compile-bridge', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:'+str(bridge), *['/reference:'+str(r) for r in refs], SOURCE/'ReWelfare.cs'])
        shutil.copy2(SOURCE/'Injector.cs', work/'Program.cs')
        project = work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        run('inject', [dotnet, work/'bin/Debug/net8.0/Patch.dll', host, bridge, out/'Assembly-CSharp.dll'])
        consumer = work/'Consumer.cs'
        consumer.write_text('public static class Consumer { public static UI.UIActivityTreasureCast Keep(UI.UIActivityTreasureCast page) { return page; } }')
        run('compile-metadata-consumer', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/out:'+str(work/'Consumer.dll'), '/reference:'+str(out/'Assembly-CSharp.dll'), *['/reference:'+str(r) for r in refs[1:]], consumer])
    after = (out/'Assembly-CSharp.dll').read_bytes()
    checks = verify(before, after)
    report = {'schema': 'hyjs-client-re-welfare-v1', 'status': 'STATIC_VERIFIED_NOT_PACKAGED', 'inputManagedSHA256': sha(before), 'outputManagedSHA256': sha(after), 'bridgeSHA256': sha(bridge.read_bytes()), 'oldUnityProfileCSharp3Compiled': True, 'metadataConsumerCompiled': True, 'deviceExecuted': False, 'replacementEntries': ['assets/bin/Data/Managed/Assembly-CSharp.dll', 'assets/bin/Data/Managed/SoulFlowerReWelfare.dll'], 'sourcePins': {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [SOURCE/'ReWelfare.cs', SOURCE/'Injector.cs', SOURCE/'build.py', SOURCE/'target-pins.json']}, **checks}
    (out/'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps({k: report[k] for k in ['status', 'outputManagedSHA256', 'bridgeSHA256', 'unrelatedInputRawMethodsUnchanged', 'originalEngineMethodsUnchanged']}))


if __name__ == '__main__':
    main()
