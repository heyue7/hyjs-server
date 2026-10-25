"""Compile one original-style trial reset button hook over the pinned formal APK."""
import argparse,hashlib,json,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body

def sha(raw):return hashlib.sha256(raw).hexdigest()
def main():
 p=argparse.ArgumentParser();p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--base-sha256',required=True);p.add_argument('--output-directory',type=Path,required=True);a=p.parse_args();out=a.output_directory.resolve();assert not out.exists() and out.is_relative_to(ROOT);assert sha(a.base_apk.read_bytes())==a.base_sha256;out.mkdir(parents=True)
 runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll';source=Path(__file__).parent
 def run(name,args):
  r=subprocess.run(['rtk','proxy',*map(str,args)],capture_output=True,text=True,timeout=120,cwd=ROOT);(out/(name+'.log')).write_text(r.stdout+r.stderr);assert r.returncode==0,name+' failed; inspect saved log'
 entry='assets/bin/Data/Managed/Assembly-CSharp.dll'
 with zipfile.ZipFile(a.base_apk)as z,tempfile.TemporaryDirectory(prefix='hyjs-reset69-')as temp:
  work=Path(temp);before=z.read(entry);host=work/'Assembly-CSharp.dll';host.write_bytes(before);refs=[host]
  for name in ['UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll']:
   f=work/name;f.write_bytes(z.read('assets/bin/Data/Managed/'+name));refs.append(f)
  helper=out/'SoulFlowerTrialReset69.dll';run('helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),*['/reference:'+str(f)for f in refs],source/'TrialReset.cs'])
  shutil.copy2(source/'Injector.cs',work/'Program.cs');project=work/'Patch.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run('injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false']);run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',host,helper,out/'Assembly-CSharp.dll',sha(before)])
  after=(out/'Assembly-CSharp.dll').read_bytes();left,right=[dnfile.dnPE(data=b)for b in [before,after]];changed=[];engine=0
  assert len(methods(left))==len(methods(right))
  for (n,i,x),(m,j,y)in zip(methods(left),methods(right)):
   assert (n,i,x.Signature.value,x.struct.Flags)==(m,j,y.Signature.value,y.struct.Flags)
   if not x.Rva:continue
   if exact_body(left,x)!=exact_body(right,y):changed.append(n)
   else:engine+=n.startswith('Battle.Engine')
  assert changed==['UI.RushPage::Refresh'] and engine==547
  payload={entry:str((out/'Assembly-CSharp.dll').relative_to(ROOT)),'assets/bin/Data/Managed/SoulFlowerTrialReset69.dll':str(helper.relative_to(ROOT))}
  manifest={'schema':'trial-reset69-native-button-overlay-v1','baseAPKSHA256':a.base_sha256,'inputManagedSHA256':sha(before),'outputManagedSHA256':sha(after),'inputDexSHA256':sha(z.read('classes2.dex')),'managedOnly':True,'inheritedJavaByteExact':True,'inheritedJavaPublicProtectedABIExact':True,'payloadFiles':payload,'replacementEntries':{n:sha((ROOT/f).read_bytes())for n,f in payload.items()},'allowedChangedMethods':changed,'originalEngineMethodsExact':engine,'sourcePins':{str(f.relative_to(ROOT)):sha(f.read_bytes())for f in [source/'build.py',source/'Injector.cs',source/'TrialReset.cs']}}
  (out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n');print(json.dumps({'changed':changed,'originalEngineMethodsExact':engine}))
if __name__=='__main__':main()
