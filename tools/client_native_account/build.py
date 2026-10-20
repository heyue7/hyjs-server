"""Compose original-account UI, feedback IL header and world-map protocol fixes."""
import argparse
import hashlib
import json
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path
import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
ALLOWED = {'UserInput::Show', 'UserInput::OnRegistButtonClick', 'UserInput::OnLoginButton', 'UI.KeFuPage::DoOpen', 'UI.WorldMapPage::transfer'}
def sha(value): return hashlib.sha256(value).hexdigest()

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--base-sha256',required=True)
    p.add_argument('--input-managed',type=Path);p.add_argument('--output-directory',type=Path,required=True)
    a=p.parse_args();out=a.output_directory.resolve();require(not out.exists(),'Fresh directory required')
    require(sha(a.base_apk.read_bytes())==a.base_sha256,'APK identity changed');out.mkdir(parents=True)
    runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    source=Path(__file__).resolve().parent;dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    def run(name,cmd):
        r=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=120)
        (out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed; see saved log')
    with zipfile.ZipFile(a.base_apk) as z, tempfile.TemporaryDirectory(prefix='hyjs-native-account-') as t:
        work=Path(t);before=a.input_managed.read_bytes() if a.input_managed else z.read(ENTRY)
        host=work/'Assembly-CSharp.dll';host.write_bytes(before);refs=[host]
        for name in ['UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll']:
            f=work/name;f.write_bytes(z.read('assets/bin/Data/Managed/'+name));refs.append(f)
        helper=out/'SoulFlowerNativeAccount.dll'
        run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/resource:'+str(source/'resources/account-button.png')+',HYJS.AccountButton.png','/out:'+str(helper),*['/reference:'+str(x) for x in refs],source/'NativeAccount.cs'])
        shutil.copy2(source/'Injector.cs',work/'Program.cs');project=work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
        run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',host,helper,out/'Assembly-CSharp.dll'])
    after=(out/'Assembly-CSharp.dll').read_bytes();left,right=[dnfile.dnPE(data=b)for b in [before,after]]
    changes=[];engine=0
    for (n,i,x),(m,j,y) in zip(methods(left),methods(right)):
        require((n,i,x.Signature.value,x.struct.Flags)==(m,j,y.Signature.value,y.struct.Flags),'ABI changed: '+n)
        if not x.Rva:continue
        old,new=exact_body(left,x),exact_body(right,y)
        if old!=new:require(n in ALLOWED,'Unrelated body changed: '+n);changes.append(n)
        else:engine+=n.startswith('Battle.Engine')
    require(set(changes)==ALLOWED and engine==547,'Expected scoped method changes')
    require(not getattr(right.net.mdtables,'FieldPtr',None),'Unsupported FieldPtr')
    require(len(left.net.mdtables.ModuleRef.rows)==len(right.net.mdtables.ModuleRef.rows),'Unexpected ModuleRef')
    report={'status':'STATIC_VERIFIED_NOT_PACKAGED','baseAPKSHA256':a.base_sha256,'inputManagedSHA256':sha(before),'outputManagedSHA256':sha(after),'allowedChangedMethods':changes,'originalEngineMethodsExact':engine,'sourcePins':{str(f.relative_to(ROOT)):sha(f.read_bytes()) for f in [source/'NativeAccount.cs',source/'Injector.cs',source/'build.py',source/'resources/account-button.png',source/'resources/source.json']}}
    (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report))
if __name__=='__main__':main()
