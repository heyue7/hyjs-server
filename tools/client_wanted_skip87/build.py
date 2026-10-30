"""Build the two-method Wanted skip overlay; no APK, network or device operation."""
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
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.client_achievements.build import normalize
from tools.local_asset_ready.remove_gate import exact_body, require
from tools.client_fragment_boxes85.build import assembly_identity
from tools.client_relations85.verify_existing import verify_metadata

PINS = json.loads((HERE / 'target-pins.json').read_text())
APK_SHA = '25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
SCHEMA = 'hyjs-wanted-skip87-overlay-v1'
ENTRY = 'assets/bin/Data/Managed/'

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()

def bridge(pe, token, name):
    require(token >> 24 == 10, 'helper reference kind differs')
    ref = pe.net.mdtables.MemberRef.rows[(token & 0xffffff)-1]
    owner = ref.Class.row
    require(str(ref.Name) == name and str(owner.TypeNamespace) == 'HYJS.WantedSkip87' and
            str(owner.TypeName) == 'WantedSkip' and owner.ResolutionScope.table.name == 'AssemblyRef' and
            str(owner.ResolutionScope.row.Name) == 'HYJS.WantedSkip87', 'helper reference differs')

def verify(before, after):
    old, new = [dnfile.dnPE(data=raw) for raw in (before, after)]
    for table in ('TypeDef', 'Field', 'MethodDef', 'Param'):
        require(len(getattr(old.net.mdtables, table).rows) == len(getattr(new.net.mdtables, table).rows), 'definition inventory differs')
    require(old.net.mdtables.Module.rows[0].Mvid.value == new.net.mdtables.Module.rows[0].Mvid.value, 'MVID differs')
    require([assembly_identity(x) for x in old.net.mdtables.Assembly.rows] == [assembly_identity(x) for x in new.net.mdtables.Assembly.rows], 'assembly differs')
    a, b = old.net.mdtables.AssemblyRef.rows, new.net.mdtables.AssemblyRef.rows
    require(len(b) == len(a)+1 and [assembly_identity(x) for x in a] == [assembly_identity(x) for x in b[:-1]] and str(b[-1].Name) == 'HYJS.WantedSkip87', 'assembly references differ')
    for table, attrs in (('Field', ('Name', 'Signature')), ('TypeRef', ('TypeName', 'TypeNamespace')), ('MemberRef', ('Name', 'Signature'))):
        a, b = (getattr(pe.net.mdtables, table).rows for pe in (old, new))
        require(len(b) >= len(a), 'original rows removed')
        for x, y in zip(a, b):
            for attr in attrs:
                left, right = getattr(x, attr), getattr(y, attr)
                require((left.value if hasattr(left, 'value') else str(left)) == (right.value if hasattr(right, 'value') else str(right)), 'original member ABI differs')
            if table == 'TypeRef':
                require((x.ResolutionScope.table.name, x.ResolutionScope.row_index) == (y.ResolutionScope.table.name, y.ResolutionScope.row_index), 'type reference scope differs')
            elif table == 'MemberRef':
                require((x.Class.table.name, x.Class.row_index) == (y.Class.table.name, y.Class.row_index), 'member reference scope differs')
    changed, unchanged, engine = [], 0, 0
    left, right = methods(old), methods(new)
    require(len(left) == len(right), 'method count differs')
    for (name, rid, a), (name2, rid2, b) in zip(left, right):
        require((name, rid, a.Signature.value, a.struct.Flags, a.struct.ImplFlags) == (name2, rid2, b.Signature.value, b.struct.Flags, b.struct.ImplFlags), 'method ABI differs')
        if not a.Rva:
            require(not b.Rva, 'unexpected body')
            continue
        x, y = exact_body(old, a), exact_body(new, b)
        if name not in PINS:
            require(x == y, 'unrelated method changed: '+name)
            unchanged += 1
            if name.startswith('Battle.Engine'): engine += 1
            continue
        require(sha(x) == PINS[name], 'input target differs')
        ob, oc = decoded(old, a)
        nb, nc = decoded(new, b)
        require(not ob.exception_handlers and not nb.exception_handlers, 'exception regions differ')
        if name == 'Battle.BattleManager::Skip':
            require(len(oc) == 7 and len(nc) == 9 and [i[0] for i in nc[3:5]] == ['ldarg.0', 'call'], 'skip hook differs')
            bridge(new, nc[4][1], 'BeforeSkip')
            mapping = {i if i < 3 else i+2: i for i in range(len(oc))}
        else:
            require(len(nc) == len(oc)+4 and [i[0] for i in nc[:4]] == ['ldarg.1', 'call', 'brfalse', 'ret'] and nc[2][1] == 4, 'receiver hook differs')
            bridge(new, nc[1][1], 'IgnoreEndedControl')
            mapping = {i+4: i for i in range(len(oc))}
        require(normalize(nc, mapping) == oc, 'original method instructions or branches differ')
        changed.append({'method': name, 'token': hex(0x06000000|rid), 'before': sha(x), 'after': sha(y)})
    require(len(changed) == 2 and engine == 547, 'target or engine inventory differs')
    return {'changedMethods': changed, 'unchangedMethodBodies': unchanged, 'engineMethodsExact': engine, **verify_metadata(before, after)}

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input-managed', type=Path, required=True)
    parser.add_argument('--input-managed-sha256', required=True)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--tool-root', type=Path, default=Path('/Users/lirui/LR/hyjs'))
    args = parser.parse_args()
    before = args.input_managed.read_bytes()
    require(sha(before) == args.input_managed_sha256 and digest(args.base_apk) == APK_SHA, 'fixed input differs')
    pe = dnfile.dnPE(data=before)
    require({n:sha(exact_body(pe,m)) for n,_,m in methods(pe) if n in PINS} == PINS, 'target pin differs or patch already present')
    out = args.output_directory.resolve()
    require(not out.exists(), 'fresh output directory required')
    out.mkdir(parents=True)
    sdk = args.tool_root.resolve()/'.tools/native-batch'
    dotnet = sdk/'dotnet/dotnet'
    csc = next((sdk/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = sdk/'injector/bin/Debug/net8.0/dnlib.dll'
    sources = [HERE/n for n in ('Runtime.cs','Injector.cs','ModelTests.cs','build.py','target-pins.json')]
    pins = {str(p.relative_to(ROOT)):digest(p) for p in sources}
    def run(label, command):
        r = subprocess.run(['rtk','proxy',*map(str,command)], capture_output=True, text=True, timeout=120)
        (out/(label+'.log')).write_text(r.stdout+r.stderr)
        require(r.returncode == 0, label+' failed; inspect log')
    with tempfile.TemporaryDirectory(prefix='hyjs-wanted-skip87-') as tmp, zipfile.ZipFile(args.base_apk) as archive:
        work = Path(tmp)
        host = work/'Assembly-CSharp.dll'
        host.write_bytes(before)
        refs = [host]
        for name in ('mscorlib.dll','System.dll','System.Core.dll','UnityEngine.dll'):
            file = work/name
            file.write_bytes(archive.read(ENTRY+name))
            refs.append(file)
        helper = out/'HYJS.WantedSkip87.dll'
        run('compile-helper',[dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),*['/reference:'+str(p) for p in refs],HERE/'Runtime.cs'])
        shutil.copy2(HERE/'Injector.cs',work/'Program.cs')
        project = work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        (work/'NuGet.Config').write_text('<configuration><packageSources><clear /></packageSources></configuration>')
        run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false','--configfile',work/'NuGet.Config'])
        output = out/'Assembly-CSharp.dll'
        run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',host,helper,output])
        reference = sorted((sdk/'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))[-1]
        model = work/'Model.dll'
        (work/'Model.runtimeconfig.json').write_text(json.dumps({'runtimeOptions':{'tfm':'net8.0','framework':{'name':'Microsoft.NETCore.App','version':'8.0.0'}}}))
        run('compile-model',[dotnet,csc,'/nologo','/target:exe','/nostdlib+','/out:'+str(model),*['/reference:'+str(p) for p in reference.glob('*.dll')],HERE/'Runtime.cs',HERE/'ModelTests.cs'])
        run('run-model',[dotnet,model])
    checks = verify(before,output.read_bytes())
    require(digest(args.input_managed) == sha(before) and pins == {str(p.relative_to(ROOT)):digest(p) for p in sources}, 'source changed during build')
    report = {'schema':SCHEMA,'status':'MODEL_VERIFIED_NOT_DEVICE_TESTED','inputManagedSHA256':sha(before),'outputManagedSHA256':digest(output),'helperSHA256':digest(helper),'sourcePins':pins,'payloadFiles':{ENTRY+'Assembly-CSharp.dll':str(output),ENTRY+'HYJS.WantedSkip87.dll':str(helper)},**checks}
    (out/'manifest.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({k:report[k] for k in ('status','outputManagedSHA256','helperSHA256','engineMethodsExact')}))

if __name__ == '__main__':
    main()
