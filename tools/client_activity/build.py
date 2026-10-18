"""Protect activity rendering boundaries, retaining original ID/prefab/server routing."""
import argparse,hashlib,json,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods,operand
from tools.local_asset_ready.remove_gate import exact_body,require
BASE="b9825732e3aa82714483645b0b2d87431bc56a58992d32c1c0ca637e01d941af"
BASE_DLL="979701a7ee26e016b27ca0fe1970a340270ea0ed833c694285fbb60913f65da7"
ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll"
TARGETS={'UI.UIActivity::MsgCallback': 'cda66876a753f21954add67aefdbd521a68cc737582709d78f9a97a4b372ec5e', 'UI.UIActivity::Init': 'b42e83c3bdd1cf3672ae43045eead75f33d47c81189cf1c9d30c381b84b05582', 'UI.UIActivityPanel::Refersh': '18293a839d814c182b1c3b095acfa410e7faa2e575f8f6f1f042f7179add8e63', 'UI.UIActivityPanel::Show': 'f7e2ccc47692b235d122b052b2ea4ca0d809f01dbe1b0852781d3edd343bcf48', 'UI.UIActivityPanel::OnPageClosed': 'd73843ff77745a0ee8a40e78c8d288d2d190e3a3d2af468cc6d3c3a3e26b5fba', 'UI.UIActivityPanel::OnPageCoverPageRemoved': 'dda6d62fc9c612d44bf32a8e4dc78ba7f0ae16c5102dc7c001d537e2532bfde4', 'UI.UIActivityPanel::OnPageCoverPageOpen': '5bc1f973c7c67238063f1ab0c6e13edc44482dd62b6738540aeec2c8a58c7248'}

def sha(raw):return hashlib.sha256(raw).hexdigest()
def decoded(pe,row):
 body=read_method_body_from_bytes(exact_body(pe,row)); indexes={i.offset:n for n,i in enumerate(body.instructions)}
 def val(i):
  if i.opcode.name=="ldstr":return pe.net.user_strings.get(i.operand.value&0xffffff).value
  if i.opcode.name=="switch":return tuple(indexes[o] for o in i.operand)
  if i.opcode.flow_control.name in ("Branch","Cond_Branch"):return indexes[i.operand]
  return operand(i.operand)
 return body,[(i.opcode.name,val(i))for i in body.instructions]

def verify(before,after,expected_input_sha=BASE_DLL):
 require(sha(before)==expected_input_sha,"Exact bound input required")
 a,b=[dnfile.dnPE(data=x)for x in(before,after)]; left,right=methods(a),methods(b)
 require(len(left)==len(right),"Inherited method inventory changed")
 for table in ("TypeDef","Field","MethodDef"):
  require(len(getattr(a.net.mdtables,table).rows)==len(getattr(b.net.mdtables,table).rows),"Original inventory changed: "+table)
 require(not getattr(b.net.mdtables,"FieldPtr",None),"Old Unity must not gain FieldPtr")
 changed=[];unchanged=engine=0
 for (n,r,old),(nn,rr,new)in zip(left,right):
  require((n,r)==(nn,rr)and old.Signature.value==new.Signature.value and old.struct.Flags==new.struct.Flags and old.struct.ImplFlags==new.struct.ImplFlags,"Inherited ABI/token changed: "+n)
  if not old.Rva:require(not new.Rva,"Unexpected body");continue
  x,y=exact_body(a,old),exact_body(b,new)
  if n not in TARGETS:
   require(x==y,"Unrelated raw method changed: "+n);unchanged+=1;engine+=n.startswith("Battle.Engine");continue
  require(sha(x)==TARGETS[n],"Original target guard differs: "+n)
  ob,oc=decoded(a,old);nb,nc=decoded(b,new)
  if n=="UI.UIActivityPanel::Show":
   require([o for o,v in nc]==["ldarg.0","ldarg.1","call","ret"],"Unexpected show boundary")
   require(not nb.exception_handlers,"Unexpected show EH")
  else:
   count={"UI.UIActivity::MsgCallback":6,"UI.UIActivity::Init":5}.get(n,4)
   require(len(nc)==len(oc)+count and nc[count-2][0]=="brtrue" and nc[count-2][1]==count and nc[count-1][0]=="ret","Unexpected activity guard")
   def unshift(item):
    op,value=item
    if op=="switch":return(op,tuple(v-count for v in value))
    if op.startswith(("br","beq","bne","bge","bgt","ble","blt","leave"))and isinstance(value,int):return(op,value-count)
    return item
   require([unshift(i)for i in nc[count:]]==oc,"Original control flow suffix changed: "+n)
   require(len(ob.exception_handlers)==len(nb.exception_handlers),"Original EH count changed")
   shift=nb.instructions[count].offset-ob.instructions[0].offset
   for oe,ne in zip(ob.exception_handlers,nb.exception_handlers):
    for key in ("try_start","try_end","handler_start","handler_end"):
     require(getattr(ne,key)==getattr(oe,key)+shift,"Original exception scope changed: "+n+key)
  changed.append({"method":n,"beforeSHA256":sha(x),"afterSHA256":sha(y)})
 require(engine==547 and len(changed)==7,"Engine/target count differs")
 return {"changes":changed,"originalEngineMethodsUnchanged":engine,"unrelatedRawMethodBodiesUnchanged":unchanged,"originalMetadataABIAndTokensPreserved":True,"originalActivityIDAndPrefabRoutingPreserved":True}

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument("--base-apk",type=Path,default=ROOT/"apks/build/hyjs-local.apk");p.add_argument("--input-managed",type=Path);p.add_argument("--input-managed-sha256");p.add_argument("--output-directory",type=Path,required=True);args=p.parse_args()
 require(sha(args.base_apk.read_bytes())==BASE,"Pinned b982 base APK required");out=args.output_directory.resolve();require(not out.exists(),"Use fresh output directory");out.mkdir(parents=True)
 with zipfile.ZipFile(args.base_apk)as z:before=z.read(ENTRY)
 expected=BASE_DLL
 if args.input_managed:
  require(args.input_managed_sha256 is not None and len(args.input_managed_sha256)==64,"Composition input needs explicit SHA");before=args.input_managed.read_bytes();expected=args.input_managed_sha256
 require(sha(before)==expected,"Composition input SHA differs")
 runtime=ROOT/".tools/native-batch";dotnet=runtime/"dotnet/dotnet";compiler=next((runtime/"dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"));dnlib=runtime/"injector/bin/Debug/net8.0/dnlib.dll";injector=Path(__file__).with_name("Injector.cs");source=Path(__file__).with_name("ActivityGuard.cs")
 def run(name,command):
  r=subprocess.run(["rtk","proxy",*map(str,command)],cwd=ROOT,capture_output=True,text=True,timeout=90);(out/(name+".log")).write_text(r.stdout+r.stderr);require(r.returncode==0,name+" failed; inspect saved log")
 with tempfile.TemporaryDirectory(prefix="hyjs-activity-overlay-")as temp:
  work=Path(temp);current=work/"Assembly-CSharp.dll";current.write_bytes(before);references=[current]
  with zipfile.ZipFile(args.base_apk)as z:
   for name in ("UnityEngine.dll","mscorlib.dll","Assembly-CSharp-firstpass.dll","System.dll","System.Core.dll"):
    f=work/name;f.write_bytes(z.read("assets/bin/Data/Managed/"+name));references.append(f)
  bridge=out/"SoulFlowerActivity.dll";run("compile-activity-bridge",[dotnet,compiler,"/nologo","/target:library","/nostdlib+","/langversion:3","/deterministic+","/out:"+str(bridge),*["/reference:"+str(f)for f in references],source])
  shutil.copy2(injector,work/"Program.cs");project=work/"Patch.csproj";project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>');run("compile-injector",[dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"]);run("patch",[dotnet,work/"bin/Debug/net8.0/Patch.dll",current,bridge,out/"Assembly-CSharp.dll"])
 after=(out/"Assembly-CSharp.dll").read_bytes();checks=verify(before,after,expected)
 report={"schema":"hyjs-client-activity-boundaries-v1","status":"STATIC_VERIFIED_NOT_PACKAGED","baseAPKSHA256":BASE,"inputManagedSHA256":sha(before),"outputManagedSHA256":sha(after),"bridgeSHA256":sha(bridge.read_bytes()),"replacementEntries":[ENTRY,"assets/bin/Data/Managed/SoulFlowerActivity.dll"],"deviceExecuted":False,"actualCrashRootCauseProven":False,"scope":"Directory packet/null IDs/duplicate prefixes, empty tabs, unknown templates, original template initialization exceptions, null template lifecycle. No generic child Update/Awake exception recovery claimed.","sourcePins":{str(f.relative_to(ROOT)):sha(f.read_bytes())for f in (Path(__file__).resolve(),injector,source)},**checks};(out/"manifest.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n");print(json.dumps({"status":report["status"],"outputManagedSHA256":sha(after),"bridgeSHA256":sha(bridge.read_bytes())}))
if __name__=="__main__":main()
