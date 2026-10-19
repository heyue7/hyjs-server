"""Patch two pinned native UI methods and compile the independent feedback helper."""
import argparse,hashlib,json,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.client_achievements.build import normalize
from tools.local_asset_ready.remove_gate import exact_body,require
SOURCE=Path(__file__).resolve().parent
PINS=json.loads((SOURCE/'target-pins.json').read_text())
def sha(data):return hashlib.sha256(data).hexdigest()
def verify(before,after):
 a,b=[dnfile.dnPE(data=x) for x in [before,after]]
 for tab in ['TypeDef','Field','MethodDef']:
  require(len(getattr(a.net.mdtables,tab).rows)==len(getattr(b.net.mdtables,tab).rows),'Inventory differs '+tab)
 require(a.net.mdtables.Module.rows[0].Mvid.value==b.net.mdtables.Module.rows[0].Mvid.value,'MVID differs')
 require(not getattr(b.net.mdtables,'FieldPtr',None),'FieldPtr unsupported')
 for x,y in zip(a.net.mdtables.Field.rows,b.net.mdtables.Field.rows):require(str(x.Name)==str(y.Name) and x.Signature.value==y.Signature.value and x.struct.Flags==y.struct.Flags,'Field ABI differs')
 def ref(r):return(str(r.Name),str(r.Culture),r.struct.MajorVersion,r.struct.MinorVersion,r.struct.BuildNumber,r.struct.RevisionNumber,r.struct.Flags,r.PublicKey.value)
 require([ref(r) for r in a.net.mdtables.Assembly.rows]==[ref(r) for r in b.net.mdtables.Assembly.rows],'Assembly identity differs')
 old,new=a.net.mdtables.AssemblyRef.rows,b.net.mdtables.AssemblyRef.rows
 require([ref(r) for r in old]==[ref(r) for r in new[:len(old)]] and len(new)==len(old)+1 and str(new[-1].Name)=='SoulFlowerWelfareFeedback','Prior references differ')
 changes=[];same=engines=0
 for (name,rid,x),(name2,rid2,y) in zip(methods(a),methods(b)):
  require((name,rid)==(name2,rid2) and x.Signature.value==y.Signature.value and x.struct.Flags==y.struct.Flags and x.struct.ImplFlags==y.struct.ImplFlags,'Method ABI differs '+name)
  if not x.Rva:require(not y.Rva,'Unexpected body');continue
  oldbody,newbody=exact_body(a,x),exact_body(b,y)
  if name not in PINS:
   require(oldbody==newbody,'Unrelated raw method changed '+name);same+=1;engines+=name.startswith('Battle.Engine');continue
  require(sha(oldbody)==PINS[name],'Target source differs '+name)
  ob,oc=decoded(a,x);nb,nc=decoded(b,y)
  require(not ob.exception_handlers and not nb.exception_handlers,'Unexpected EH')
  if name.endswith('GetTextString'):
   require(len(nc)==len(oc)+6 and [op for op,v in nc[:6]]==['ldarg.0','ldfld','ldc.i4','bne.un','ldstr','ret'] and nc[2][1]==32 and nc[3][1]==6,'Wrong native type32 guard')
   require(nc[4][1]=='主线通关{0}次','Wrong earned goal title')
   indexes={i:i-6 for i in range(6,len(nc))};require(normalize(nc,indexes)==oc,'Original other type titles changed')
  else:
   require(oc[-1]==('ret',None) and len(nc)==len(oc)+2 and [op for op,v in nc[-3:]]==['ldarg.0','call','ret'],'Wrong claim suffix')
   row=b.net.mdtables.MemberRef.rows[(nc[-2][1]&0xffffff)-1]
   require(str(row.Name)=='AchievementClaimed' and str(row.Class.row.TypeNamespace)=='HYJS.WelfareFeedback','Wrong callback ABI')
   require(nc[:-3]+[('ret',None)]==oc,'Original claimed state changed')
  changes.append(dict(method=name,beforeSHA256=sha(oldbody),afterSHA256=sha(newbody)))
 require(engines==547 and len(changes)==2,'Wrong target/engine count')
 return dict(changes=changes,unrelatedRawMethodsUnchanged=same,originalEngineMethodsUnchanged=engines,originalMetadataABIAndTokensPreserved=True)
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--input-managed',type=Path,required=True);p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);a=p.parse_args()
 before=a.input_managed.read_bytes();out=a.output_directory.resolve();require(not out.exists(),'Fresh output required');out.mkdir(parents=True)
 runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
 def run(name,cmd):
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=90);(out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed')
 with tempfile.TemporaryDirectory(prefix='hyjs-welfare-feedback-') as tmp:
  w=Path(tmp);host=w/'Assembly-CSharp.dll';host.write_bytes(before);refs=[host]
  with zipfile.ZipFile(a.base_apk) as z:
   for n in ['UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll']:
    x=w/n;x.write_bytes(z.read('assets/bin/Data/Managed/'+n));refs.append(x)
  bridge=out/'SoulFlowerWelfareFeedback.dll'
  run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(bridge),*['/reference:'+str(x) for x in refs],SOURCE/'Feedback.cs'])
  shutil.copy2(SOURCE/'Injector.cs',w/'Program.cs');project=w/'Patch.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false']);run('inject',[dotnet,w/'bin/Debug/net8.0/Patch.dll',host,out/'Assembly-CSharp.dll'])
 after=(out/'Assembly-CSharp.dll').read_bytes();report=dict(status='STATIC_VERIFIED_NOT_PACKAGED',deviceExecuted=False,inputManagedSHA256=sha(before),outputManagedSHA256=sha(after),helperSHA256=sha(bridge.read_bytes()),oldUnityCSharp3Compiled=True,sourcePins={str(x.relative_to(ROOT)):sha(x.read_bytes()) for x in SOURCE.iterdir() if x.is_file()},**verify(before,after));(out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report,ensure_ascii=False))
if __name__=='__main__':main()
