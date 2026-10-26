"""Build a pinned trial demand overlay without touching battle methods."""
import argparse,hashlib,json,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body,require
BASE='97a9ebcc82e1127a8c877ea2385da1a715a1b17d1f9a0eeed352debc53c70061'
DLL='assets/bin/Data/Managed/Assembly-CSharp.dll'
TARGETS={'UI.RushPage::<AskRushInfo>m__21D','UI.RushEliteBossPage::DoOpen','UI.RushEliteBossPage::DoReopen','UI.RushEliteBossPage::OnCoverPageRemove','UI.RushEliteBossPage::DoClose','UI.RushEliteBossPage::OnBossIconClick'}
def sha(b):return hashlib.sha256(b).hexdigest()
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--tool-root',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);a=p.parse_args()
 require(sha(a.base_apk.read_bytes())==BASE,'Qualified formal70 candidate required');out=a.output_directory.resolve();require(out.is_relative_to(ROOT)and not out.exists(),'Fresh owned output required');out.mkdir(parents=True)
 runtime=a.tool_root/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll';src=Path(__file__).parent
 def run(name,cmd):
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=120);(out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed; inspect owned log')
 with zipfile.ZipFile(a.base_apk)as z,tempfile.TemporaryDirectory(prefix='hyjs-trial-demand-')as temp:
  work=Path(temp);before=z.read(DLL);host=work/'Assembly-CSharp.dll';host.write_bytes(before);refs=[host]
  for name in ['UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll','LitJson.dll']:
   f=work/name;f.write_bytes(z.read('assets/bin/Data/Managed/'+name));refs.append(f)
  helper=out/'HYJSTrialDemand.dll';run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),*['/reference:'+str(f)for f in refs],src/'TrialDemand.cs'])
  trial=out/'HYJS.Trial66.dll';trialSource=ROOT/'tools/client_feedback66/TrialRuntime.cs'
  run('compile-trial-recovery',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/codepage:65001','/out:'+str(trial),*['/reference:'+str(f)for f in refs],trialSource])
  recoveryBefore=z.read('assets/bin/Data/Managed/HYJS.Trial66.dll')
  shutil.copy2(src/'Injector.cs',work/'Program.cs');shutil.copy2(src/'RecoveryProof.cs',work/'RecoveryProof.cs');project=work/'Patch.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  oldTrial=work/'oldTrial.dll';oldTrial.write_bytes(recoveryBefore)
  run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false']);run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',host,helper,out/'Assembly-CSharp.dll',oldTrial,trial,out/'recovery-proof.json']);dexsha=sha(z.read('classes2.dex'))
 after=(out/'Assembly-CSharp.dll').read_bytes();left,right=[dnfile.dnPE(data=b)for b in [before,after]];changes=[];engine=0
 for table in ['TypeDef','Field','MethodDef']:require(len(getattr(left.net.mdtables,table).rows)==len(getattr(right.net.mdtables,table).rows),'ABI inventory differs')
 for (n,i,x),(m,j,y)in zip(methods(left),methods(right)):
  require((n,i,x.Signature.value,x.struct.Flags)==(m,j,y.Signature.value,y.struct.Flags),'ABI/token differs')
  if not x.Rva:continue
  a,b=exact_body(left,x),exact_body(right,y)
  if a!=b:require(n in TARGETS,'Unrelated method differs '+n);changes.append(n)
  if n.startswith('Battle.Engine'):require(a==b,'Original engine changed');engine+=1
 require(set(changes)==TARGETS and engine==547,'Target/engine mismatch');require(not getattr(right.net.mdtables,'FieldPtr',None),'Unsupported FieldPtr')
 recoveryLeft,recoveryRight=[dnfile.dnPE(data=b)for b in [recoveryBefore,trial.read_bytes()]];recoveryChanges=json.loads((out/'recovery-proof.json').read_text())
 for table in ['TypeDef','Field','MethodDef']:require(len(getattr(recoveryLeft.net.mdtables,table).rows)==len(getattr(recoveryRight.net.mdtables,table).rows),'Recovery ABI inventory differs')
 for (n,i,x),(m,j,y)in zip(methods(recoveryLeft),methods(recoveryRight)):
  require((n,i,x.Signature.value,x.struct.Flags)==(m,j,y.Signature.value,y.struct.Flags),'Recovery ABI/token differs')
 require(set(recoveryChanges)=={'HYJS.Feedback66.TrialRuntime::Failure','HYJS.Feedback66.TrialRuntime::PageClick','HYJS.Feedback66.TrialRuntime::Resume'},'Unexpected recovery changes '+str(recoveryChanges))
 files={DLL:str((out/'Assembly-CSharp.dll').relative_to(ROOT)),'assets/bin/Data/Managed/HYJSTrialDemand.dll':str(helper.relative_to(ROOT)),'assets/bin/Data/Managed/HYJS.Trial66.dll':str(trial.relative_to(ROOT))}
 report=dict(schema='hyjs-trial-demand71-v1',managedOnly=True,inheritedJavaPublicProtectedABIExact=True,inheritedJavaByteExact=True,inputDexSHA256=dexsha,inputManagedSHA256=sha(before),outputManagedSHA256=sha(after),allowedChangedMethods=changes,recoveryAllowedChangedMethods=recoveryChanges,originalEngineMethodsExact=engine,baseAPKSHA256=BASE,payloadFiles=files,replacementEntries={n:sha((ROOT/f).read_bytes())for n,f in files.items()},sourcePins={str(f.relative_to(ROOT)):sha(f.read_bytes())for f in [src/'build.py',src/'TrialDemand.cs',src/'Injector.cs',src/'RecoveryProof.cs',trialSource]})
 (out/'manifest.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({'status':'STATIC_VERIFIED_NOT_INSTALLED','outputManagedSHA256':sha(after),'allowedChangedMethods':changes,'originalEngineMethodsExact':engine}))
if __name__=='__main__':main()
