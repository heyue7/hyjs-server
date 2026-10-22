"""Compose instance protection onto the qualified pet follow50 managed DLL; no packaging or device effects."""
import argparse, hashlib, json, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path
import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require

BASE = '03896a5e323edb77ec394fc3a359e6a3488d45d09bef9ae008d30d7bfa460563'
MANAGED = '789befb6331bff7675dda31ca0eb76a91c091898c0e4894a2c5718942a562b59'
# Exact changed tokens come from the reviewed injector; all other raw bodies are compared.

def sha(raw): return hashlib.sha256(raw).hexdigest()

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk', type=Path, required=True)
    p.add_argument('--tool-root', type=Path, required=True)
    p.add_argument('--output-directory', type=Path, required=True)
    a = p.parse_args(); out = a.output_directory.resolve()
    require(not out.exists(), 'Fresh output required')
    require(sha(a.base_apk.read_bytes()) == BASE, 'Pet-follow50 APK required')
    out.mkdir(parents=True)
    runtime = a.tool_root/'.tools/native-batch'; dotnet = runtime/'dotnet/dotnet'
    compiler = next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    source = Path(__file__).resolve().parent
    def run(name, cmd):
        r = subprocess.run(['rtk', 'proxy', *map(str, cmd)], cwd=ROOT, capture_output=True, text=True, timeout=120)
        (out/(name+'.log')).write_text(r.stdout+r.stderr)
        require(r.returncode == 0, name+' failed; inspect log')
    with zipfile.ZipFile(a.base_apk) as z, tempfile.TemporaryDirectory(prefix='hyjs-guide49-') as temp:
        work = Path(temp); before = z.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
        require(sha(before) == MANAGED, 'Pet-follow50 managed identity differs')
        host = work/'Assembly-CSharp.dll'; host.write_bytes(before); refs = [host]
        for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll', 'LitJson.dll']:
            target = work/name; target.write_bytes(z.read('assets/bin/Data/Managed/'+name)); refs.append(target)
        run('compile-guide-ui', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:'+str(out/'InstanceProtection.dll'), *['/reference:'+str(x) for x in refs], source/'Controls.cs'])
        shutil.copy2(source/'Injector.cs', work/'Program.cs')
        project = work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        run('inject', [dotnet, work/'bin/Debug/net8.0/Patch.dll', host, out/'InstanceProtection.dll', out/'Assembly-CSharp.dll', out/'targets.json'])
    targets = {int(x['token']) for x in json.loads((out/'targets.json').read_text())}
    after = (out/'Assembly-CSharp.dll').read_bytes()
    left, right = [dnfile.dnPE(data=b) for b in [before, after]]
    changes = []; engine = 0
    for table in ['TypeDef', 'Field', 'MethodDef']:
        require(len(getattr(left.net.mdtables, table).rows) == len(getattr(right.net.mdtables, table).rows), 'Metadata inventory changed')
    for (n, rid, x), (m, rid2, y) in zip(methods(left), methods(right)):
        require((n, rid, x.Signature.value, x.struct.Flags) == (m, rid2, y.Signature.value, y.struct.Flags), 'ABI/token changed')
        if not x.Rva: continue
        old, new = exact_body(left, x), exact_body(right, y)
        if old != new:
            require((0x06000000|rid) in targets, 'Unrelated method changed '+n)
            changes.append({'method':n, 'token':hex(0x06000000|rid), 'beforeSHA256':sha(old), 'afterSHA256':sha(new)})
        if n.startswith('Battle.Engine'):
            require(old == new, 'Original engine changed'); engine += 1
    require({int(x['token'],16) for x in changes} == targets and engine == 547, 'Target/engine coverage differs')
    require(not getattr(right.net.mdtables, 'FieldPtr', None), 'Unsupported FieldPtr')
    report = {'schema':'hyjs-instance-lock51-v1','status':'STATIC_VERIFIED_NOT_PACKAGED','baseAPKSHA256':BASE,'inputManagedSHA256':MANAGED,'outputManagedSHA256':sha(after),'helperSHA256':sha((out/'InstanceProtection.dll').read_bytes()),'changes':changes,'originalEngineMethodsExact':engine,'sourcePins':{str(f.relative_to(ROOT)):sha(f.read_bytes()) for f in [source/'build.py',source/'Injector.cs',source/'Controls.cs']}}
    (out/'manifest.json').write_text(json.dumps(report,indent=2)+'\n'); print(json.dumps(report))

if __name__ == '__main__': main()
