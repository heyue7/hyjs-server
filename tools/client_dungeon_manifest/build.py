"""Build one scoped legacy manifest-cache path hook; no APK packaging or device access."""
import argparse,hashlib,json,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
from tools.client_arena_state.build import exact_body,methods,decode,instruction_value,member_reference
SOURCE=Path(__file__).resolve().parent
ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll";HELPER="HYJS.DungeonManifest.dll"
BASE_SHA="b7ad3c7c8ba53cc7990f3f3590642aa405e9a554135916a468668f0c7fdbdbbd"
ANDROID_SHA="28e264a89fb6b1a5fc27fa5a6fbdfb55cf510e4ba6e6431d1a023631d620f28d"
TARGET="AssetLoad.AssetBundleManager::GetFullPath"
def sha(b):return hashlib.sha256(b).hexdigest()
def require(x,msg):
 if not x:raise ValueError(msg)
def il(pe,m):
 b=read_method_body_from_bytes(exact_body(pe,m));return b.raw_bytes[b.header_size:b.header_size+b.code_size]
def verify(before,after):
 left,right=dnfile.dnPE(data=before),dnfile.dnPE(data=after);old,new=methods(left),methods(right);require(len(old)==len(new),"Method count changed")
 unchanged=engine=0;changed=[]
 for (name,rid,a),(name2,rid2,b)in zip(old,new):
  require(name==name2 and rid==rid2 and a.Signature.value==b.Signature.value,"Original ABI/token changed")
  require(bool(a.Rva)==bool(b.Rva),"Original body presence changed")
  if not a.Rva:continue
  if name!=TARGET:
   require(exact_body(left,a)==exact_body(right,b),"Non-target raw body changed: "+name);unchanged+=1;engine+=name.startswith("Battle.Engine");continue
  x,y=decode(left,a),decode(right,b);require(len(y)==len(x)+6,"Expected two extra instructions per original return")
  mapped=[];cursor=0
  for original in x:
   mapped.append(cursor)
   if original.opcode.name=="ret":
    require([v.opcode.name for v in y[cursor:cursor+3]]==["ldarg.1","call","ret"],"Return hook differs")
    ref=member_reference(right,instruction_value(y[cursor+1]));owner=ref.Class.row
    require(str(owner.TypeNamespace)+"."+str(owner.TypeName)=="HYJS.DungeonManifest.ManifestPath" and str(ref.Name)=="Resolve","Wrong helper reference");cursor+=3
   else:cursor+=1
  require(cursor==len(y),"Unexpected trailing code")
  positions={v.offset:i for i,v in enumerate(x)}
  for i,a1 in enumerate(x):
   b1=y[mapped[i]]
   if a1.opcode.name=="ret":continue
   require(a1.opcode.name==b1.opcode.name,"Original opcode changed")
   if isinstance(a1.operand,int) and a1.opcode.name.startswith(("br","beq","bne","bge","bgt","ble","blt","leave")):
    require(b1.operand==y[mapped[positions[a1.operand]]].offset,"Original branch target changed")
   elif a1.opcode.name=="switch":require(tuple(b1.operand)==tuple(y[mapped[positions[v]]].offset for v in a1.operand),"Switch changed")
   else:require(instruction_value(a1)==instruction_value(b1),"Original operand changed")
  changed.append(name)
 require(engine==547 and changed==[TARGET],"Engine or target coverage differs")
 for label in ['TypeDef','Field']:
  aa=getattr(left.net.mdtables,label).rows;bb=getattr(right.net.mdtables,label).rows;require(len(aa)==len(bb),label+" count changed")
  for a,b in zip(aa,bb):
   require(str(a.TypeName if label=='TypeDef'else a.Name)==str(b.TypeName if label=='TypeDef'else b.Name),"Original metadata name changed")
   if label=='Field':require(a.Signature.value==b.Signature.value,"Field signature changed")
 bad=[]
 for t in right.net.mdtables.TypeRef.rows:
  scope=t.ResolutionScope.row
  if t.ResolutionScope.table.name=='ModuleRef' and str(scope.Name)==HELPER:bad.append(str(t.TypeNamespace)+'.'+str(t.TypeName))
 require(not bad,"Final helper ModuleRef TypeRefs remain")
 return dict(allowedChangedMethods=changed,originalEngineMethodsUnchanged=engine,unrelatedRawMethodBodiesUnchanged=unchanged,originalMetadataABIAndTokensPreserved=True,originalBodyPreservedWithThreeReturnHooks=True,finalHelperModuleRefTypes=bad)
def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--base-apk',type=Path,required=True);ap.add_argument('--expected-sha256',default=BASE_SHA);ap.add_argument('--input-managed',type=Path);ap.add_argument('--output-directory',type=Path,required=True);ap.add_argument('--tool-root',type=Path,required=True);a=ap.parse_args()
 base=a.base_apk.resolve();out=a.output_directory.resolve();require(a.expected_sha256==BASE_SHA and sha(base.read_bytes())==BASE_SHA,"Fixed base55 APK mismatch");require(not out.exists() and out.is_relative_to(ROOT),"Fresh project output required");out.mkdir(parents=True)
 runtime=a.tool_root.resolve()/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
 def run(label,cmd):
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],capture_output=True,text=True,timeout=180);(out/(label+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,label+' failed; see log')
 with zipfile.ZipFile(base)as z,tempfile.TemporaryDirectory(prefix='hyjs-dungeon-manifest-')as temp:
  require(sha(z.read('assets/android/android'))==ANDROID_SHA,"Base android manifest pin mismatch");original=z.read(ENTRY);before=a.input_managed.read_bytes()if a.input_managed else original
  left,basepe=dnfile.dnPE(data=before),dnfile.dnPE(data=original)
  target=next(m for n,_,m in methods(left)if n==TARGET);baseline=next(m for n,_,m in methods(basepe)if n==TARGET);require(il(left,target)==il(basepe,baseline),"Target already modified");targetpin=sha(il(left,target))
  w=Path(temp);host=w/'Assembly-CSharp.dll';host.write_bytes(before);refs=[host]
  for n in ['mscorlib.dll','UnityEngine.dll']:
   f=w/n;f.write_bytes(z.read('assets/bin/Data/Managed/'+n));refs.append(f)
  helper=out/HELPER;run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/codepage:65001','/out:'+str(helper),*['/reference:'+str(f)for f in refs],SOURCE/'Runtime.cs'])
  shutil.copy2(SOURCE/'Injector.cs',w/'Program.cs');project=w/'Patch.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false']);run('inject',[dotnet,w/'bin/Debug/net8.0/Patch.dll',host,helper,out/'Assembly-CSharp.dll'])
 after=(out/'Assembly-CSharp.dll').read_bytes();proof=verify(before,after)
 report=dict(schema='hyjs-client-dungeon-manifest-cache-v1',status='STATIC_VERIFIED_NOT_PACKAGED',managedOnly=True,baseAPKSHA256=BASE_SHA,inputAPKSHA256=BASE_SHA,knownCachedAndroidSHA256=ANDROID_SHA,maximumReadBytes=2097152,inputManagedSHA256=sha(before),outputManagedSHA256=sha(after),helperSHA256=sha(helper.read_bytes()),targetOriginalCILSHA256={TARGET:targetpin},replacementEntries={ENTRY:sha(after),'assets/bin/Data/Managed/'+HELPER:sha(helper.read_bytes())},payloadFiles={ENTRY:str((out/'Assembly-CSharp.dll').relative_to(ROOT)),'assets/bin/Data/Managed/'+HELPER:str(helper.relative_to(ROOT))},sourcePins={str(f.relative_to(ROOT)):sha(f.read_bytes())for f in SOURCE.iterdir()if f.is_file()},deviceExecuted=False,serverMutated=False,inheritedJavaByteExact=True,inheritedJavaPublicProtectedABIExact=True,**proof)
 (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(proof))
if __name__=='__main__':main()
