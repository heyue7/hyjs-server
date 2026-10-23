"""Fixed61 latency62 managed-only overlay, preserving all unrelated raw bodies."""
import argparse, hashlib, importlib.util, json, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT=Path(__file__).resolve().parents[2]
HERE=Path(__file__).resolve().parent
APK_SHA="65a9b29bbf176a2788abf7896116b14794c7a818eb673b8ad71a0084ef8dcc3e"
MAIN_SHA="642734ef5323217f800a6ed018d98607a8a0eeec8c029bab6f7e777b567e9569"
MANAGED="assets/bin/Data/Managed/"
def sha(x):return hashlib.sha256(x).hexdigest()
def run(args):subprocess.run(["rtk","proxy",*map(str,args)],cwd=ROOT,check=True)
def prior():
 spec=importlib.util.spec_from_file_location("proof59",ROOT/"tools/client_one_key59/build.py");m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def verify(before,after,targets):
 proof=prior(); result=proof.verify(before,after,targets)
 a,b=dnfile.dnPE(data=before),dnfile.dnPE(data=after)
 checks=[]
 for t in targets:
  rid=int(t["token"],16)&0xffffff
  x,y=[read_method_body_from_bytes(p.get_data(p.net.mdtables.MethodDef.rows[rid-1].Rva)) for p in [a,b]]
  assert sha(proof.cil(a,a.net.mdtables.MethodDef.rows[rid-1]))==t["inputCILSHA256"]
  assert getattr(x.local_var_sig_tok,"value",None)==getattr(y.local_var_sig_tok,"value",None)
  assert not x.exception_handlers and not y.exception_handlers
  ins=y.instructions
  if t["mode"]=="replace":assert [i.opcode.name for i in ins]==["ldarg.0","call","ret"];hook=ins[1]
  else:
   assert ins[-2].opcode.name=="call" and ins[-1].opcode.name=="ret"
   hook=ins[-2]
   removed=2 if "local" in t else 1
   if "local" in t:assert ins[-3].opcode.name=="ldloc.0"
   def norm(rows):return [(i.opcode.name,getattr(i.operand,"value",str(i.operand) if i.operand is not None else None)) for i in rows]
   assert norm(x.instructions)==norm(ins[:-(removed+1)]+ins[-1:]),"Original request sequence changed"
  ref=b.net.mdtables.MemberRef.rows[(hook.operand.value&0xffffff)-1]
  assert str(ref.Name)==t["helper"] and str(ref.Class.row.TypeName)==t["helperType"].split(".")[-1] and str(ref.Class.row.TypeNamespace)=="HYJS.Latency62"
  checks.append(dict(token=t["token"],sourceCILPinned=True,originalLocalsAndEHExact=True,declaredHookExact=True))
 assert [r.Signature.value for r in a.net.mdtables.StandAloneSig.rows]==[r.Signature.value for r in b.net.mdtables.StandAloneSig.rows[:len(a.net.mdtables.StandAloneSig.rows)]]
 result.update(targetProofs=checks,originalRequestOperationsExactExceptParameterHook=True,originalSendSerializeQueueCallbackRetryTickShowCleanErrorHandlersRawExact=True,originalTimeoutCreationOperationsExactExceptFrontHook=True,originalStandAloneSignaturesExact=True)
 return result

def main():
 parser=argparse.ArgumentParser();parser.add_argument("--base-apk",type=Path,default=ROOT/"build/hyjs-recycle61-r3.apk");parser.add_argument("--output-directory",type=Path,required=True);args=parser.parse_args()
 assert sha(args.base_apk.read_bytes())==APK_SHA
 out=args.output_directory.resolve();assert not out.exists();out.mkdir(parents=True)
 targets=json.loads((HERE/"targets.json").read_text())
 sources=[ROOT/"tools/client_release/package_native.py",ROOT/"tools/login_ui/build.py",*sorted(HERE.glob("*.cs")),HERE/"targets.json",HERE/"README.md",HERE/"build.py",ROOT/"tools/client_one_key59/build.py"]
 pins={str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in sources}
 work=ROOT/".tools/native-batch";dotnet=work/"dotnet/dotnet";csc=next((work/"dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
 with tempfile.TemporaryDirectory(prefix="hyjs62-latency-build-") as tmp:
  temp=Path(tmp);refs=temp/"refs";refs.mkdir()
  with zipfile.ZipFile(args.base_apk) as apk:
   java={n:sha(apk.read(n)) for n in apk.namelist() if n.startswith("classes") and n.endswith(".dex")}
   sys.path.insert(0,str(ROOT))
   from tools.login_ui.build import dex_class_headers
   headers={}
   for n in java:
    current=dex_class_headers(apk.read(n));assert not set(headers)&set(current),"Duplicate Java class"
    headers.update(current)
   for name,(flags,parent) in headers.items():assert parent not in headers or not headers[parent][0]&16,"Final superclass"
   assert "classes2.dex" in java and headers
   for n in apk.namelist():
    if n.startswith(MANAGED) and n.endswith(".dll"):(refs/Path(n).name).write_bytes(apk.read(n))
  before=(refs/"Assembly-CSharp.dll").read_bytes();assert sha(before)==MAIN_SHA
  refpins={p.name:sha(p.read_bytes()) for p in refs.glob("*.dll")}
  helper=out/"HYJS.Latency62.dll"
  run([dotnet,csc,"/nologo","/target:library","/nostdlib+","/langversion:3","/deterministic+","/out:"+str(helper),*["/r:"+str(p) for p in sorted(refs.glob("*.dll"))],HERE/"Policy.cs",HERE/"NetworkParameters.cs",HERE/"NetworkFeedback.cs",HERE/"FrontLayer.cs"])
  for name in ["Inject","Probe"]:
   folder=temp/name;folder.mkdir();shutil.copy2(HERE/("Injector.cs" if name=="Inject" else "Probe.cs"),folder/"Program.cs")
   dep=""
   if name=="Inject":dep='<ItemGroup><Reference Include="dnlib"><HintPath>'+str(work/"injector/bin/Debug/net8.0/dnlib.dll")+'</HintPath></Reference></ItemGroup>'
   else:
    for source in ["Policy.cs","NetworkParameters.cs"]:shutil.copy2(HERE/source,folder/source)
   project=folder/(name+".csproj");project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable><NoWarn>SYSLIB0014</NoWarn></PropertyGroup>'+dep+'</Project>')
   run([dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
  result=out/"Assembly-CSharp.dll"
  run([dotnet,temp/"Inject/bin/Debug/net8.0/Inject.dll",refs/"Assembly-CSharp.dll",helper,result,HERE/"targets.json"])
  proof=verify(before,result.read_bytes(),targets)
  run([dotnet,temp/"Probe/bin/Debug/net8.0/Probe.dll",out/"policy-webrequest-probe.json"])
 assert pins=={str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in sources}
 names=[t["type"]+"::"+t["method"] for t in targets]
 payload={MANAGED+p.name:str(p.relative_to(ROOT)) for p in [result,helper]}
 manifest=dict(schema="hyjs-latency62-managed-overlay-v1",baseAPK="build/hyjs-recycle61-r3.apk",baseAPKSHA256=APK_SHA,inputManagedSHA256=MAIN_SHA,outputManagedSHA256=sha(result.read_bytes()),helperSHA256=sha(helper.read_bytes()),sourcePins=pins,referencePins=refpins,**proof,allowedChangedMethods=names,payloadFiles=payload,replacementEntries={n:sha((ROOT/p).read_bytes()) for n,p in payload.items()},inheritedJavaEntrySHA256=java,inheritedJavaByteExact=all(n not in payload for n in java),inheritedJavaPublicProtectedABIExact=all(n not in payload for n in java),inputDexSHA256=java["classes2.dex"],inheritedJavaClassHeadersChecked=len(headers),managedOnly=set(payload)=={MANAGED+"Assembly-CSharp.dll",MANAGED+"HYJS.Latency62.dll"},originalRefsCSharp3Compiled=True,policyWebRequestProbePassed=True,temporaryBuildDirectoryRemoved=True,apkBuilt=False,deviceTested=False,networkTimingMeasured=False)
 (out/"manifest.json").write_text(json.dumps(manifest,indent=2)+"\n")
 # Exercise the actual packager through every pre-write check. Abort exactly
 # at its first unsigned ZIP creation, so no large APK is produced.
 from tools.client_release import package_native
 original_zip=package_native.zipfile.ZipFile
 original_argv=sys.argv[:]
 class ReachedAPKWrite(Exception):pass
 def read_only_zip(file,mode="r",*args,**kwargs):
  if mode!="r":raise ReachedAPKWrite(str(file))
  return original_zip(file,mode,*args,**kwargs)
 package_native.zipfile.ZipFile=read_only_zip
 sys.argv=[str(ROOT/"tools/client_release/package_native.py"),"--base-apk",str(args.base_apk),"--overlay",str(out),"--output",str(out/"preflight-never-written.apk"),"--record",str(out/"preflight-never-written.json")]
 reached=False
 try:package_native.main()
 except ReachedAPKWrite:reached=True
 finally:
  package_native.zipfile.ZipFile=original_zip;sys.argv=original_argv
 assert reached and not (out/"preflight-never-written.apk").exists()
 (out/"package-contract-preflight.json").write_text(json.dumps(dict(actualPackageNativeEntryExercised=True,allChecksThroughUnsignedZIPWritePassed=True,javaDuplicateFinalClassAndManagedABIEngineChecksPassed=True,sourcePinsAndPayloadHashesPassed=True,baseSignatureCertificateReadable=True,apkWritten=False,postWriteSigningAndEntryVerificationRequiresPackaging=True),indent=2)+"\n")
 print(json.dumps({k:v for k,v in manifest.items() if k not in ["sourcePins","referencePins"]},indent=2))
if __name__=="__main__":main()
