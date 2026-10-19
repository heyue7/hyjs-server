"""Apply only the pinned type32 guide title color hook to any composed feedback main."""
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
PINS=json.loads((SOURCE/'style-target-pins.json').read_text())
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
 require([ref(r) for r in old]==[ref(r) for r in new] and sum(str(r.Name)=='SoulFlowerWelfareFeedback' for r in new)==1,'Prior references differ')
 changes=[];same=engines=0
 for (name,rid,x),(name2,rid2,y) in zip(methods(a),methods(b)):
  require((name,rid)==(name2,rid2) and x.Signature.value==y.Signature.value and x.struct.Flags==y.struct.Flags and x.struct.ImplFlags==y.struct.ImplFlags,'Method ABI differs '+name)
  if not x.Rva:require(not y.Rva,'Unexpected body');continue
  oldbody,newbody=exact_body(a,x),exact_body(b,y)
  if name not in PINS:
   require(oldbody==newbody,'Unrelated raw method changed '+name);same+=1;engines+=name.startswith('Battle.Engine');continue
  require(sha(oldbody)==PINS[name],'Target source differs '+name)
  ob,oc=decoded(a,x);nb,nc=decoded(b,y)
  require(len(ob.exception_handlers)==len(nb.exception_handlers),'EH count differs')
  require(len(nc)==len(oc)+2 and nc[34][0]=='ldarg.0' and nc[35][0]=='call','Wrong title color hook')
  row=b.net.mdtables.MemberRef.rows[(nc[35][1]&0xffffff)-1]
  require(str(row.Name)=='GuideTitle' and str(row.Class.row.TypeNamespace)=='HYJS.WelfareFeedback' and str(row.Class.row.TypeName)=='FeedbackBridge','Wrong color bridge ABI')
  require(str(a.net.mdtables.Field.rows[(oc[27][1]&0xffffff)-1].Name)=='labTitle','Wrong original title field')
  indexes={i:(i if i<34 else i-2) for i in range(len(nc)) if i not in (34,35)}
  require(normalize(nc,indexes)==oc,'Original activity binding changed')
  oi={instruction.offset-ob.header_size:i for i,instruction in enumerate(ob.instructions)}
  ni={instruction.offset-nb.header_size:indexes[i] for i,instruction in enumerate(nb.instructions) if i in indexes}
  oi[ob.instructions[-1].offset+ob.instructions[-1].size-ob.header_size]=len(oc)
  ni[nb.instructions[-1].offset+nb.instructions[-1].size-nb.header_size]=len(oc)
  for oe,ne in zip(ob.exception_handlers,nb.exception_handlers):
   require(oe.exception_type==ne.exception_type and oe.catch_type==ne.catch_type,'EH type differs')
   for key in ['try_start','try_end','handler_start','handler_end']:
    require(oi[getattr(oe,key)]==ni[getattr(ne,key)],'Original EH range differs')
  changes.append(dict(method=name,beforeSHA256=sha(oldbody),afterSHA256=sha(newbody)))
 require(engines==547 and len(changes)==1,'Wrong target/engine count')
 return dict(changes=changes,unrelatedRawMethodsUnchanged=same,originalEngineMethodsUnchanged=engines,originalMetadataABIAndTokensPreserved=True)
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--input-managed',type=Path,required=True);p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);a=p.parse_args()
 before=a.input_managed.read_bytes();out=a.output_directory.resolve();require(not out.exists(),'Fresh output required');out.mkdir(parents=True)
 runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
 def run(name,cmd):
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=90);(out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed')
 with tempfile.TemporaryDirectory(prefix='hyjs-welfare-title-style-') as tmp:
  w=Path(tmp);host=w/'Assembly-CSharp.dll';host.write_bytes(before);refs=[host]
  with zipfile.ZipFile(a.base_apk) as z:
   for n in ['UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll']:
    x=w/n;x.write_bytes(z.read('assets/bin/Data/Managed/'+n));refs.append(x)
  bridge=out/'SoulFlowerWelfareFeedback.dll'
  run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(bridge),*['/reference:'+str(x) for x in refs],SOURCE/'Feedback.cs'])
  shutil.copy2(SOURCE/'StyleInjector.cs',w/'Program.cs');project=w/'Patch.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false']);run('inject',[dotnet,w/'bin/Debug/net8.0/Patch.dll',host,out/'Assembly-CSharp.dll'])
  consumer=w/'Consumer.cs';consumer.write_text('public static class Consumer { public static void Keep(UI.AcRechargeSingleItem item) { HYJS.WelfareFeedback.FeedbackBridge.GuideTitle(item); } }')
  run('compile-metadata-consumer',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/out:'+str(w/'Consumer.dll'),'/reference:'+str(out/'Assembly-CSharp.dll'),'/reference:'+str(bridge),*['/reference:'+str(x) for x in refs[1:]],consumer])
 after=(out/'Assembly-CSharp.dll').read_bytes();report=dict(status='STATIC_VERIFIED_NOT_PACKAGED',deviceExecuted=False,inputManagedSHA256=sha(before),outputManagedSHA256=sha(after),helperSHA256=sha(bridge.read_bytes()),oldUnityCSharp3Compiled=True,guideActivityType=32,titleColorRGBA=[0.22,0.22,0.22,1],fontAndSizePreserved=True,sourcePins={str(x.relative_to(ROOT)):sha(x.read_bytes()) for x in SOURCE.iterdir() if x.is_file()},**verify(before,after));(out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report,ensure_ascii=False))
if __name__=='__main__':main()
