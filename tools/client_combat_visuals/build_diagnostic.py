import hashlib,json,shutil,subprocess,tempfile,zipfile,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
import dnfile
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
S=Path(__file__).parent;O=ROOT/'analysis/play-feedback-20261019/combat/formation-diag-r1';O.mkdir(exist_ok=False)
base=ROOT/'apks/build/hyjs-feedback-b26-r2.apk';sha=lambda x:hashlib.sha256(x).hexdigest()
assert sha(base.read_bytes())=='8990e488e5d8d16480140cab31908bb0625af6fe15ee70fecf58af7e37cea1b3'
runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
def run(n,args):
 r=subprocess.run(['rtk','proxy',*map(str,args)],cwd=ROOT,capture_output=True,text=True);(O/(n+'.log')).write_text(r.stdout+r.stderr);assert r.returncode==0,n
with zipfile.ZipFile(base) as z,tempfile.TemporaryDirectory(prefix='hyjs-formation-diag-') as tmp:
 w=Path(tmp);refs=[]
 for n in ['Assembly-CSharp.dll','UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll']:
  q=w/n;q.write_bytes(z.read('assets/bin/Data/Managed/'+n));refs.append(q)
 before=refs[0].read_bytes();helper=O/'SoulFormationDiagnostics.dll'
 run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),*['/reference:'+str(q) for q in refs],S/'FormationDiagnostics.cs'])
 shutil.copy2(S/'DiagnosticInjector.cs',w/'Program.cs')
 (w/'Patch.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
 run('compile-injector',[dotnet,'build',w/'Patch.csproj','--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
 run('inject',[dotnet,w/'bin/Debug/net8.0/Patch.dll',refs[0],helper,O/'Assembly-CSharp.dll'])
 after=(O/'Assembly-CSharp.dll').read_bytes();a,b=dnfile.dnPE(data=before),dnfile.dnPE(data=after);changes=[];eng=0
 for (n,r,x),(nn,rr,y) in zip(methods(a),methods(b)):
  assert n==nn and r==rr and x.Signature.value==y.Signature.value
  if x.Rva:
   u,v=exact_body(a,x),exact_body(b,y)
   if u!=v:changes.append(n)
   if n.startswith('Battle.Engine'):assert u==v;eng+=1
 assert changes==['UI.UIFormationPopupPage::InitPage','UI.UIFormationPopupPage::SetModelPos'] and eng==547,changes
 d={'schema':'hyjs-formation-diagnostic-v1','baseAPKSHA256':sha(base.read_bytes()),'inputManagedSHA256':sha(before),'payloadFiles':{'assets/bin/Data/Managed/Assembly-CSharp.dll':str((O/'Assembly-CSharp.dll').relative_to(ROOT)),'assets/bin/Data/Managed/SoulFormationDiagnostics.dll':str(helper.relative_to(ROOT))},'replacementEntries':{'assets/bin/Data/Managed/Assembly-CSharp.dll':sha(after),'assets/bin/Data/Managed/SoulFormationDiagnostics.dll':sha(helper.read_bytes())},'allowedChangedMethods':changes,'originalEngineMethodsExact':eng,'diagnosticOnly':True,'removeHelperForFinal':True}
 (O/'manifest.json').write_text(json.dumps(d,indent=2)+'\n');print(json.dumps(d))
