"""Build a pinned batch63 visual field overlay without packaging or device access."""
import argparse,hashlib,json,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2]
HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT))
from tools.client_dungeon_access.build import verify,methods,cil
ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll"
APK_SHA="fe427f13e8683b434bf7d2756b2dbd4a6f42746d0d6c66c86264182252704580"
MAIN_SHA="29bbef55a2458304b7b86b8a0fc1388b8bf3c6c1185ea47dfb58ce0c6e8abb2e"
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
  assert sha(apk.read(ENTRY))==MAIN_SHA,"Official62 Main drift"
  before=args.input_managed.read_bytes() if args.input_managed else apk.read(ENTRY)
  pe=dnfile.dnPE(data=before)
  seen=set()
  for name,rid,method in methods(pe):
   if name in targetmap:
    target=targetmap[name];assert rid==int(target["token"],16)&0xffffff and sha(cil(pe,method))==target["inputCILSHA256"],"Target input drift: "+name;seen.add(name)
  assert seen==set(targetmap)
  for n in apk.namelist():
   if n.startswith("assets/bin/Data/Managed/") and n.endswith(".dll"):(refs/Path(n).name).write_bytes(apk.read(n))
  (refs/"Assembly-CSharp.dll").write_bytes(before)
  helper=out/"HYJS.Field63.dll"
  run("helper",[dotnet,csc,"/nologo","/target:library","/nostdlib+","/langversion:3","/deterministic+","/out:"+str(helper),*["/r:"+str(p)for p in sorted(refs.glob("*.dll"))],HERE/"Runtime.cs"])
  shutil.copy2(HERE/"Injector.cs",work/"Program.cs");project=work/"Inject.csproj"
  project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(tools/"injector/bin/Debug/net8.0/dnlib.dll")+'</HintPath></Reference></ItemGroup></Project>')
  run("injector",[dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
  result=out/"Assembly-CSharp.dll";run("inject",[dotnet,work/"bin/Debug/net8.0/Inject.dll",refs/"Assembly-CSharp.dll",helper,result,HERE/"targets.json"])
  proof=verify(before,result.read_bytes(),targetmap,helper)
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
  manifest=dict(schema="hyjs-field63-managed-overlay-v1",baseAPKSHA256=APK_SHA,inputManagedSHA256=sha(before),official62ManagedSHA256=MAIN_SHA,outputManagedSHA256=sha(result.read_bytes()),payloadFiles=payload,replacementEntries={n:sha((ROOT/p).read_bytes())for n,p in payload.items()},sourcePins=pins,managedOnly=True,inheritedJavaByteExact=True,inheritedJavaPublicProtectedABIExact=True,inputDexSHA256=sha(apk.read("classes2.dex")),originalRefsCSharp3Compiled=True,apkBuilt=False,deviceTested=False,networkTimingMeasured=False,**proof)
  (out/"manifest.json").write_text(json.dumps(manifest,indent=2)+"\n")
 assert pins=={str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in HERE.iterdir() if p.is_file()}
 print(json.dumps(proof))
if __name__=="__main__":main()
