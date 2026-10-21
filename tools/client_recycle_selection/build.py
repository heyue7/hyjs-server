"""Build the bounded three-method native selection adapter and verify every inherited body."""
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
from tools.client_display.build import exact_body

SOURCE = Path(__file__).resolve().parent
MAIN = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER = 'assets/bin/Data/Managed/SoulFlowerRecycle.dll'
TARGETS = {'UI.' + name + '::OnPetBreakupButtionOneKeyClick' for name in (
    'RecycleBreakupSelectHeroPanel', 'RecycleBreakupSelectEquipPanel', 'RecycleBreakupSelectPetPanel')}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(ok, message):
    if not ok:
        raise ValueError(message)


def verify(before, after):
    left, right = [dnfile.dnPE(data=raw) for raw in (before, after)]
    old, new = methods(left), methods(right)
    require(len(old) == len(new), 'Method count changed')
    changed, engine = [], 0
    for (name, rid, a), (other, oid, b) in zip(old, new):
        require((name, rid, a.Signature.value, a.struct.Flags, a.struct.ImplFlags) ==
                (other, oid, b.Signature.value, b.struct.Flags, b.struct.ImplFlags), 'Original ABI differs: ' + name)
        if not a.Rva:
            require(not b.Rva, 'Unexpected body')
            continue
        x, y = exact_body(left, a), exact_body(right, b)
        if name not in TARGETS:
            require(x == y, 'Unrelated original method changed: ' + name)
            engine += name.startswith('Battle.Engine')
            continue
        body = read_method_body_from_bytes(y)
        require(not body.exception_handlers and [i.opcode.name for i in body.instructions] == ['ldarg.0','call','ret'], 'Selection dispatch differs')
        token = body.instructions[1].operand.value
        call = right.net.mdtables.MemberRef.rows[(token & 0xffffff)-1]
        require(str(call.Name) == 'Select' and str(call.Class.row.TypeName) == 'SelectionControl', 'Wrong helper target')
        changed.append({'method': name, 'beforeSHA256': sha(x), 'afterSHA256': sha(y)})
    require(engine == 547 and len(changed) == 3, 'Protected inventory differs')
    return {'changes': changed, 'originalEngineMethodsExact': engine, 'allUnrelatedMethodBodiesExact': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    out = args.output_directory.resolve()
    require(not out.exists(), 'Use a fresh output directory')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime/'injector/bin/Debug/net8.0/dnlib.dll'

    def run(name, cmd):
        result = subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=180)
        (out/(name+'.log')).write_text(result.stdout+result.stderr)
        require(result.returncode == 0, name+' failed; see '+str(out/(name+'.log')))

    with zipfile.ZipFile(args.base_apk) as archive, tempfile.TemporaryDirectory(prefix='hyjs-selection40-') as tmp:
        work = Path(tmp)
        before = archive.read(MAIN)
        original = work/'Assembly-CSharp.dll'
        original.write_bytes(before)
        refs = [original]
        for name in ('UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll','LitJson.dll'):
            ref = work/name
            ref.write_bytes(archive.read('assets/bin/Data/Managed/'+name))
            refs.append(ref)
        helper = out/'SoulFlowerRecycle.dll'
        run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),*['/reference:'+str(ref) for ref in refs],SOURCE/'Selection.cs'])
        shutil.copy2(SOURCE/'Injector.cs',work/'Program.cs')
        project = work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',original,out/'Assembly-CSharp.dll'])
        after = (out/'Assembly-CSharp.dll').read_bytes()
        payloads = {MAIN:out/'Assembly-CSharp.dll',HELPER:helper}
        report = {'schema':'hyjs-original-recycle-selection-v1','status':'STATIC_VERIFIED_NOT_PACKAGED',
                  'baseAPKSHA256':sha(args.base_apk.read_bytes()),'inputManagedSHA256':sha(before),
                  'outputManagedSHA256':sha(after),'inputDexSHA256':sha(archive.read('classes2.dex')),
                  'managedOnly':True,'inheritedJavaPublicProtectedABIExact':True,'inheritedJavaByteExact':True,
                  'phoneVerified':False,'originalResourceUIExact':True,
                  'payloadFiles':{name:str(path.relative_to(ROOT)) for name,path in payloads.items()},
                  'replacementEntries':{name:sha(path.read_bytes()) for name,path in payloads.items()},
                  'sourcePins':{str(path.relative_to(ROOT)):sha(path.read_bytes()) for path in (SOURCE/'Selection.cs',SOURCE/'Injector.cs',Path(__file__).resolve())},
                  **verify(before,after)}
        (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
        print(json.dumps({key:report[key] for key in ('status','outputManagedSHA256','originalEngineMethodsExact')}))


if __name__ == '__main__':
    main()
