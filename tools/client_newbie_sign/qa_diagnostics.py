"""QA-only diagnostics over the fixed already-guarded55 APK. Never package/install."""
import argparse,hashlib,json,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
from tools.client_arena_state.build import exact_body,methods,decode,instruction_value,member_reference
SOURCE=Path(__file__).parent;ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll";HELPER="HYJS.NewbieSign.dll"
def sha(x):return hashlib.sha256(x).hexdigest()
def require(x,msg):
 if not x:raise ValueError(msg)
def callname(pe,ins):
 if ins.opcode.name!="call":return ""
 token=instruction_value(ins)
 if not isinstance(token,int)or token>>24!=0x0A:return ""
 r=member_reference(pe,token);return str(r.Name) if r else ""
def verify(before,after):
 a,b=dnfile.dnPE(data=before),dnfile.dnPE(data=after);old,new=methods(a),methods(b);require(len(old)==len(new),"method count changed")
 changed=[];unchanged=engine=0
 for (name,rid,x),(name2,rid2,y) in zip(old,new):
  require((name,rid,x.Signature.value)==(name2,rid2,y.Signature.value),"method ABI changed")
  if not x.Rva:continue
  if name not in ["Room.RoomTown::<ShowTenDaySign>m__8D","UI.TenDaySignPage::UpdateLoginReward"]:
   require(exact_body(a,x)==exact_body(b,y),"unrelated raw body changed "+name);unchanged+=1;engine+=name.startswith("Battle.Engine");continue
  left,right=decode(a,x),decode(b,y);offsets={};kept=[];i=0
  if name.startswith("Room."):
   require([v.opcode.name for v in right[:3]]==["ldarg.1","ldarg.2","call"]and callname(b,right[2])=="DiagnoseAuto","auto prefix differs");i=3
  while i<len(right):
   ins=right[i]
   if name.startswith("UI.")and i+2<len(right)and ins.opcode.name=="ldarg.0"and callname(b,right[i+1])=="RenderExit"and right[i+2].opcode.name=="ret":
    for j in range(3):offsets[right[i+j].offset]=len(kept)
    kept.append(right[i+2]);i+=3
   else:offsets[ins.offset]=len(kept);kept.append(ins);i+=1
  require(len(left)==len(kept),"native instruction count differs")
  lm={v.offset:i for i,v in enumerate(left)}
  for l,r in zip(left,kept):
   require(l.opcode.name==r.opcode.name,"native opcode changed "+name)
   if isinstance(l.operand,int)and l.opcode.name.startswith(("br","beq","bne","bge","bgt","ble","blt","leave")):require(lm[l.operand]==offsets[r.operand],"native branch changed")
   elif l.opcode.name=="switch":require([lm[v]for v in l.operand]==[offsets[v]for v in r.operand],"native switch changed")
   else:require(instruction_value(l)==instruction_value(r),"native operand changed "+name)
  changed.append(name)
 require(engine==547 and len(changed)==2,"coverage differs")
 return dict(originalEngineMethodsUnchanged=engine,unchangedInheritedRawMethodBodies=unchanged,changedMethods=changed,nativeInstructionsAndBranchesPreserved=True,existingTwoGuardsNotDuplicated=True)
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument("--qa-diagnostics",action="store_true",required=True);p.add_argument("--base-apk",type=Path,required=True);p.add_argument("--expected-sha256",required=True);p.add_argument("--output-directory",type=Path,required=True);p.add_argument("--tool-root",type=Path,required=True);args=p.parse_args()
 require(args.expected_sha256=="b7ad3c7c8ba53cc7990f3f3590642aa405e9a554135916a468668f0c7fdbdbbd","fixed55 input required")
 require(sha(args.base_apk.read_bytes())==args.expected_sha256,"input digest mismatch")
 out=args.output_directory.resolve();require(not out.exists()and out.is_relative_to(ROOT),"fresh project output required");out.mkdir(parents=True)
 runtime=args.tool_root.resolve()/".tools/native-batch";dotnet=runtime/"dotnet/dotnet";compiler=next((runtime/"dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"));dnlib=runtime/"injector/bin/Debug/net8.0/dnlib.dll"
 def run(name,cmd):
  r=subprocess.run(["rtk","proxy",*map(str,cmd)],capture_output=True,text=True,timeout=180);(out/(name+".log")).write_text(r.stdout+r.stderr);require(r.returncode==0,name+" failed; inspect log")
 with zipfile.ZipFile(args.base_apk)as z,tempfile.TemporaryDirectory(prefix="hyjs-newbie-qa-")as temp:
  w=Path(temp);before=z.read(ENTRY);host=w/"Assembly-CSharp.dll";host.write_bytes(before)
  refs=[host]
  for n in ["mscorlib.dll","UnityEngine.dll"]:f=w/n;f.write_bytes(z.read("assets/bin/Data/Managed/"+n));refs.append(f)
  helper=out/HELPER
  run("compile-helper",[dotnet,compiler,"/nologo","/target:library","/nostdlib+","/langversion:3","/define:HYJS_NEWBIE_QA","/deterministic+","/codepage:65001","/out:"+str(helper),*["/reference:"+str(r)for r in refs],SOURCE/"NewbieSign.cs"])
  shutil.copy2(SOURCE/"QADiagnosticInjector.cs",w/"Program.cs");project=w/"Patch.csproj";project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run("compile-injector",[dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
  run("inject",[dotnet,w/"bin/Debug/net8.0/Patch.dll",host,helper,out/"Assembly-CSharp.dll"])
 after=(out/"Assembly-CSharp.dll").read_bytes();proof=verify(before,after)
 report=dict(status="STATIC_VERIFIED_NOT_PACKAGED",managedOnly=True,sourcePins={str(p.relative_to(ROOT)):sha(p.read_bytes())for p in SOURCE.iterdir()if p.is_file()},schema="hyjs-newbie-qa-diagnostics-v1",qaOnly=True,neverPromote=True,deviceExecuted=False,serverMutated=False,baseAPKSHA256=args.expected_sha256,inputAPKSHA256=args.expected_sha256,inputManagedSHA256=sha(before),outputManagedSHA256=sha(after),helperSHA256=sha(helper.read_bytes()),logTag="HYJS.NewbieSignQA",replacementEntries={ENTRY:sha(after),"assets/bin/Data/Managed/"+HELPER:sha(helper.read_bytes())},payloadFiles={ENTRY:str((out/"Assembly-CSharp.dll").relative_to(ROOT)),"assets/bin/Data/Managed/"+HELPER:str(helper.relative_to(ROOT))},**proof)
 (out/"manifest.json").write_text(json.dumps(report,indent=2)+"\n");print(json.dumps(proof))
if __name__=="__main__":main()
