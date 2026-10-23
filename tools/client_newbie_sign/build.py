"""Compile and statically verify two conditional original-UI guards; never package/install."""
import argparse, hashlib, json, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.client_arena_state.build import exact_body, methods, decode, instruction_value, member_reference
SOURCE=Path(__file__).resolve().parent
ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll"
HELPER="HYJS.NewbieSign.dll"
TARGETS={"UI.ShowRight::<OnTenDaySignClick>m__154":("cc08d51c8256615e10baaea3708c812f6ccfb7023e46e419c565c62cba238ca9","CanOpen"),"UI.TenDaySignPage::UpdateLoginReward":("9c7abed61179bc0b07f5dd95308ec9487c8e29dac7e560154df018b31cdf45c5","CanRender")}
def sha(raw):return hashlib.sha256(raw).hexdigest()
def require(ok,message):
 if not ok:raise ValueError(message)
def il(pe,m):
 raw=exact_body(pe,m);body=read_method_body_from_bytes(raw)
 return raw[body.header_size:body.header_size+body.code_size]
def verify(before,after,helper):
 left,right=dnfile.dnPE(data=before),dnfile.dnPE(data=after)
 old,new=methods(left),methods(right)
 require(len(old)==len(new),"Original method count changed")
 unchanged=engine=0;changed=[]
 for (name,rid,a),(other,rid2,b) in zip(old,new):
  require(name==other and rid==rid2 and a.Signature.value==b.Signature.value,"Original method ABI/token changed")
  require(bool(a.Rva)==bool(b.Rva),"Original body presence changed")
  if not a.Rva:continue
  if name not in TARGETS:
   require(exact_body(left,a)==exact_body(right,b),"Unrelated raw body changed: "+name)
   unchanged+=1;engine+=name.startswith("Battle.Engine");continue
  x,y=decode(left,a),decode(right,b)
  require(len(y)==len(x)+5,"Guard prefix length differs")
  require([v.opcode.name for v in y[:5]]==["ldarg.0","ldarg.1","call","brtrue","ret"],"Guard prefix differs")
  reference=member_reference(right,instruction_value(y[2]));owner=reference.Class.row
  require(str(owner.TypeNamespace)+"."+str(owner.TypeName)=="HYJS.NewbieSign.NewbieSignGuard" and str(reference.Name)==TARGETS[name][1],"Guard consumption target differs")
  require(y[3].operand==y[5].offset,"Guard success must reach untouched original entry")
  offsets={v.offset:i for i,v in enumerate(x)}
  for i,(a1,b1) in enumerate(zip(x,y[5:])):
   require(a1.opcode.name==b1.opcode.name,"Original opcode changed: "+name)
   if isinstance(a1.operand,int) and a1.opcode.name.startswith(("br","beq","bne","bge","bgt","ble","blt","leave")):
    require(b1.operand==y[5+offsets[a1.operand]].offset,"Original branch changed")
   elif a1.opcode.name=="switch":require(tuple(b1.operand)==tuple(y[5+offsets[v]].offset for v in a1.operand),"Original switch changed")
   else:require(instruction_value(a1)==instruction_value(b1),"Original operand changed: "+name)
  changed.append(name)
 require(engine==547 and len(changed)==2,"Engine/target coverage differs")
 # Original TypeDef/Field rows and method signatures remain in original row order.
 for label in ("TypeDef","Field"):
  aa=getattr(left.net.mdtables,label).rows;bb=getattr(right.net.mdtables,label).rows
  require(len(aa)==len(bb),label+" count changed")
  for a,b in zip(aa,bb):
   require(str(a.TypeName if label=="TypeDef" else a.Name)==str(b.TypeName if label=="TypeDef" else b.Name),label+" token identity changed")
   if label=="Field":require(a.Signature.value==b.Signature.value,"Original field signature changed")
 return dict(allowedChangedMethods=changed,originalEngineMethodsUnchanged=engine,unrelatedRawMethodBodiesUnchanged=unchanged,originalMetadataABIAndTokensPreserved=True,guardSuccessRunsUntouchedOriginalBody=True,guardFalseReturnsBeforePageOpenOrTemplateRender=True)
def main():
 if "--qa-diagnostics" in sys.argv:
  from qa_diagnostics import main as qa_main
  return qa_main()
 ap=argparse.ArgumentParser(description=__doc__)
 ap.add_argument("--base-apk",type=Path,required=True);ap.add_argument("--expected-sha256",required=True)
 ap.add_argument("--output-directory",type=Path,required=True);ap.add_argument("--tool-root",type=Path,required=True)
 args=ap.parse_args();base=args.base_apk.resolve();out=args.output_directory.resolve()
 require(sha(base.read_bytes())==args.expected_sha256,"Input APK SHA mismatch")
 require(not out.exists() and out.is_relative_to(ROOT),"Fresh project output required")
 pins={name:target[0] for name,target in TARGETS.items()}
 out.mkdir(parents=True)
 runtime=args.tool_root.resolve()/".tools/native-batch";dotnet=runtime/"dotnet/dotnet"
 compiler=next((runtime/"dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"));dnlib=runtime/"injector/bin/Debug/net8.0/dnlib.dll"
 def run(name,cmd):
  r=subprocess.run(["rtk","proxy",*map(str,cmd)],capture_output=True,text=True,timeout=180)
  (out/(name+".log")).write_text(r.stdout+r.stderr);require(r.returncode==0,name+" failed; see log")
 with zipfile.ZipFile(base) as archive,tempfile.TemporaryDirectory(prefix="hyjs-newbie-sign-") as temp:
  w=Path(temp);before=archive.read(ENTRY);host=w/"Assembly-CSharp.dll";host.write_bytes(before)
  pe=dnfile.dnPE(data=before)
  for name,_,m in methods(pe):
   if name in TARGETS:require(sha(il(pe,m))==pins[name],"Target original CIL changed: "+name)
  refs=[host]
  for n in ["mscorlib.dll","UnityEngine.dll"]:
   file=w/n;file.write_bytes(archive.read("assets/bin/Data/Managed/"+n));refs.append(file)
  helper=out/HELPER
  run("compile-helper",[dotnet,compiler,"/nologo","/target:library","/nostdlib+","/langversion:3","/deterministic+","/codepage:65001","/out:"+str(helper),*["/reference:"+str(r) for r in refs],SOURCE/"NewbieSign.cs"])
  shutil.copy2(SOURCE/"Injector.cs",w/"Program.cs")
  project=w/"Patch.csproj";project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run("compile-injector",[dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
  run("inject",[dotnet,w/"bin/Debug/net8.0/Patch.dll",host,helper,out/"Assembly-CSharp.dll"])
 after=(out/"Assembly-CSharp.dll").read_bytes();proof=verify(before,after,helper.read_bytes())
 run("verify-guard",[sys.executable,SOURCE/"verify_guard.py","--tool-root",args.tool_root.resolve(),"--output",out/"guard-cases.log"])
 proof.update(actualHelperGuardCases=8,helperGuardRuntime="net8 minimal UI doubles; not Unity/device")
 report=dict(schema="hyjs-client-newbie-sign-guard-v1",status="STATIC_VERIFIED_NOT_PACKAGED",managedOnly=True,inputAPKSHA256=args.expected_sha256,baseAPKSHA256=args.expected_sha256,inheritedJavaPublicProtectedABIExact=True,inheritedJavaByteExact=True,inputManagedSHA256=sha(before),replacementEntries={ENTRY:sha(after),"assets/bin/Data/Managed/"+HELPER:sha(helper.read_bytes())},outputManagedSHA256=sha(after),helperSHA256=sha(helper.read_bytes()),targetOriginalCILSHA256=pins,payloadFiles={ENTRY:str((out/"Assembly-CSharp.dll").relative_to(ROOT)),"assets/bin/Data/Managed/"+HELPER:str(helper.relative_to(ROOT))},sourcePins={str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in SOURCE.iterdir() if p.is_file()},deviceExecuted=False,serverMutated=False,**proof)
 (out/"manifest.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
 print(json.dumps(proof,ensure_ascii=False))
if __name__=="__main__":main()
