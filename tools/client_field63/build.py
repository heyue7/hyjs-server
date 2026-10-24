"""Build a pinned batch64 field correction over the published batch63 overlay without packaging or device access."""
import argparse,hashlib,json,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2]
HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT))
from tools.client_dungeon_access.build import verify,methods,cil
ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll"
APK_SHA="4a85ed009e86c649b5954e8d1f02165e9d1951ec89256196f3dccd9076e6183d"
MAIN_SHA="6ee235813aa4c40b3d0e447857fec5248f4439c47828cb5e76553293a464bf57"
HELPER_SHA="d3526fab1351697042d9603aa3863fe3c202897bc0d23610165522790832be10"
def sha(b):return hashlib.sha256(b).hexdigest()
def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument("--base-apk",type=Path,required=True);ap.add_argument("--input-managed",type=Path);ap.add_argument("--output-directory",type=Path,required=True);args=ap.parse_args()
 assert sha(args.base_apk.read_bytes())==APK_SHA,"Unreviewed APK"
 out=args.output_directory.resolve();assert out.is_relative_to(ROOT) and not out.exists();out.mkdir(parents=True)
 targets=json.loads((HERE/"targets.json").read_text());targetmap={t["type"]+"::"+t["method"]:t for t in targets}
 tools=ROOT/".tools/native-batch";dotnet=tools/"dotnet/dotnet";csc=next((tools/"dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
 pins={str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in HERE.iterdir() if p.is_file()}
 def run(label,cmd):
  result=subprocess.run(["rtk","proxy",*map(str,cmd)],capture_output=True,text=True,timeout=180);(out/(label+".log")).write_text(result.stdout+result.stderr)
  assert result.returncode==0,label+" failed: "+str(out/(label+".log"))
 with zipfile.ZipFile(args.base_apk) as apk,tempfile.TemporaryDirectory(prefix="hyjs-field63-") as tmp:
  work=Path(tmp);refs=work/"refs";refs.mkdir()
  assert sha(apk.read(ENTRY))==MAIN_SHA,"Published63 Main drift"
  before=args.input_managed.read_bytes() if args.input_managed else apk.read(ENTRY)
  assert sha(before)==MAIN_SHA,"Input Main must be the published63 overlay; compose unrelated changes after this correction"
  assert sha(apk.read("assets/bin/Data/Managed/HYJS.Field63.dll"))==HELPER_SHA,"Published63 field helper drift"
  pe=dnfile.dnPE(data=before)
  seen=set()
  for name,rid,method in methods(pe):
   if name in targetmap:
    target=targetmap[name];assert rid==int(target["token"],16)&0xffffff and sha(cil(pe,method))==target["inputCILSHA256"],"Target input drift: "+name;seen.add(name)
  assert seen==set(targetmap)
  # Existing three hooks are already present in the release. Verify their ABI
  # and target, then preserve their raw bodies while replacing the helper.
  from dncil.cil.body.reader import read_method_body_from_bytes
  for t in targets:
   if not t.get("retained"):continue
   body=read_method_body_from_bytes(pe.get_data(pe.net.mdtables.MethodDef.rows[(int(t["token"],16)&0xffffff)-1].Rva))
   ins=body.instructions
   if t["mode"]=="replace":assert [i.opcode.name for i in ins]==["ldarg.0"]+(["ldarg.1"] if t["arguments"]==2 else [])+["call","ret"] and not body.exception_handlers
   else:assert [i.opcode.name for i in ins[-3:]]==["ldarg.0","call","ret"]
   ref=pe.net.mdtables.MemberRef.rows[(ins[-2].operand.value&0xffffff)-1]
   assert str(ref.Name)==t["helper"] and str(ref.Class.row.TypeNamespace)=="HYJS.Field63"
   assert ref.Signature.value==bytes.fromhex("0002011c1c" if t["arguments"]==2 else "0001011c")
  for n in apk.namelist():
   if n.startswith("assets/bin/Data/Managed/") and n.endswith(".dll") and not n.endswith("/HYJS.Field63.dll"):(refs/Path(n).name).write_bytes(apk.read(n))
  (refs/"Assembly-CSharp.dll").write_bytes(before)
  helper=out/"HYJS.Field63.dll"
  run("helper",[dotnet,csc,"/nologo","/target:library","/nostdlib+","/langversion:3","/deterministic+","/out:"+str(helper),*["/r:"+str(p)for p in sorted(refs.glob("*.dll"))],HERE/"Runtime.cs"])
  shutil.copy2(HERE/"Injector.cs",work/"Program.cs");project=work/"Inject.csproj"
  project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(tools/"injector/bin/Debug/net8.0/dnlib.dll")+'</HintPath></Reference></ItemGroup></Project>')
  run("injector",[dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
  result=out/"Assembly-CSharp.dll";run("inject",[dotnet,work/"bin/Debug/net8.0/Inject.dll",refs/"Assembly-CSharp.dll",helper,result,HERE/"targets.json"])
  proof=verify(before,result.read_bytes(),{k:v for k,v in targetmap.items() if not v.get("retained")},helper)
  after=dnfile.dnPE(data=result.read_bytes())
  for ref in after.net.mdtables.TypeRef.rows:
   if str(ref.TypeNamespace).startswith("Room"):
    scope=ref.ResolutionScope.row
    assert not (type(scope).__name__=="ModuleRefRow" and str(scope.Name)=="HYJS.Field63.dll"),"Game type incorrectly scoped to helper module"
  for ref in after.net.mdtables.MemberRef.rows:
   if str(getattr(ref.Class.row,"TypeNamespace",""))=="HYJS.Field63" and str(getattr(ref.Class.row,"TypeName",""))=="Runtime":
    expected=bytes.fromhex("0002011c1c" if str(ref.Name)=="Monsters" else "0001011c")
    assert ref.Signature.value==expected,"Helper hook ABI must contain object parameters only"
  from dncil.cil.body.reader import read_method_body_from_bytes
  for t in targets:
   m=after.net.mdtables.MethodDef.rows[(int(t["token"],16)&0xffffff)-1];body=read_method_body_from_bytes(after.get_data(m.Rva));ins=body.instructions
   if t["mode"]=="replace":assert [i.opcode.name for i in ins]==["ldarg.0"]+(["ldarg.1"] if t["arguments"]==2 else [])+["call","ret"] and not body.exception_handlers
   else:assert [i.opcode.name for i in ins[-3:]]==["ldarg.0","call","ret"]
   hook=ins[-2];ref=after.net.mdtables.MemberRef.rows[(hook.operand.value&0xffffff)-1];assert str(ref.Name)==t["helper"] and str(ref.Class.row.TypeNamespace)=="HYJS.Field63"
  payload={ENTRY:str(result.relative_to(ROOT)),"assets/bin/Data/Managed/HYJS.Field63.dll":str(helper.relative_to(ROOT))}
  manifest=dict(schema="hyjs-field64-managed-overlay-v1",baseAPKSHA256=APK_SHA,inputManagedSHA256=sha(before),published63ManagedSHA256=MAIN_SHA,retained63HookCount=3,newBossHookCount=1,priorFieldHelperSHA256=HELPER_SHA,outputManagedSHA256=sha(result.read_bytes()),payloadFiles=payload,replacementEntries={n:sha((ROOT/p).read_bytes())for n,p in payload.items()},sourcePins=pins,managedOnly=True,inheritedJavaByteExact=True,inheritedJavaPublicProtectedABIExact=True,inputDexSHA256=sha(apk.read("classes2.dex")),originalRefsCSharp3Compiled=True,apkBuilt=False,deviceTested=False,networkTimingMeasured=False,**proof)
  (out/"manifest.json").write_text(json.dumps(manifest,indent=2)+"\n")
 assert pins=={str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in HERE.iterdir() if p.is_file()}
 print(json.dumps(proof))
if __name__=="__main__":main()
