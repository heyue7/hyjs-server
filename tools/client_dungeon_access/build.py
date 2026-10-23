"""Build a pinned visual-only dungeon adapter; never package, install or contact servers."""
import argparse, hashlib, json, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT=Path(__file__).resolve().parents[2]
SOURCE=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT))
from tools.client_arena_state.build import exact_body,methods,decode,instruction_value,member_reference
ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll"
HELPER="HYJS.DungeonContent.dll"
def sha(raw):return hashlib.sha256(raw).hexdigest()
def require(ok,message):
 if not ok:raise ValueError(message)
def cil(pe,m):
  body=exact_body(pe,m);parsed=read_method_body_from_bytes(body)
  return body[parsed.header_size:parsed.header_size+parsed.code_size]
def table(name):
 import csv
 rows=list(csv.reader((ROOT/"analysis/tables"/("Table"+name+".tsv")).open(),delimiter="\t"))
 return [dict(zip(rows[1],r))for r in rows[5:]if r and len(r)==len(rows[1])]
def resources(z,policy,extra):
 import UnityPy
 names=set(z.namelist());require(len(names)==len(z.infolist()),"Duplicate APK entry");names.update(extra)
 tree=next(x.read_typetree()for x in UnityPy.load(extra.get("assets/android/android",z.read("assets/android/android"))).objects if x.type.name=="AssetBundleManifest")
 graph=dict(tree["AssetBundleNames"]);infos=dict(tree["AssetBundleInfos"]);ids={v:k for k,v in graph.items()}
 available={n[15:]for n in names if n.startswith("assets/android/")}
 present=set(policy["whitelist"])
 require(len(present)==37 and all("map/"+x in available for x in present),"Scene whitelist is incomplete")
 pending=[ids["map/"+x]for x in present];seen=set()
 while pending:
  i=pending.pop()
  if i in seen:continue
  seen.add(i);pending.extend(infos[i]["AssetBundleDependencies"])
 bundles=sorted(graph[i]for i in seen)
 require(set(bundles)<=available,"Selected scene dependencies unavailable")
 hashed=[]
 for n in bundles:
  entry="assets/android/"+n
  if entry in extra: h=sha(extra[entry])
  else:
   with z.open(entry)as f: h=hashlib.file_digest(f,"sha256").hexdigest()
  hashed.append((n,h))
 mappings=policy["sceneMappings"];records=[]
 missions=table("CopyMission");tasks=table("CopyTask")
 require(len(missions)==161 and len(table("CopyChapter"))==27 and len(tasks)==638,"Original dungeon directory drift")
 for family,rows,key,kind in [("mission",missions,"MissionId","run"),("task",tasks,"TaskId","battle")]:
  for row in rows:
   original=row["SceneName"]
   chosen=original if original in present else mappings.get(original,{}).get(kind)
   require(chosen in present,"Unmapped dungeon scene: "+family+"/"+row[key]+"/"+original)
   records.append(dict(family=family,id=row[key],original=original,resolved=chosen,replaced=chosen!=original))
 replaced=sum(x["replaced"]for x in records if x["family"]=="mission")
 require(replaced==policy["replacedMainMissionCount"],"Replacement mission count differs")
 return dict(sceneCount=len(present),dependencyBundles=len(bundles),dependencyInventorySHA256=sha(json.dumps(hashed,separators=(",",":")).encode()),mainChapters=27,mainMissions=len(missions),mainTasks=len(tasks),replacedMissions=replaced,records=records,originalMissingLayoutsRecovered=False,runtimeVerified=False)
def verify(before,after,targets,helper_path):
 a,b=dnfile.dnPE(data=before),dnfile.dnPE(data=after);left,right=methods(a),methods(b)
 require(len(left)==len(right),"Method inventory changed")
 wanted=set(targets);changed=set();engine=untouched=0
 for (name,i,m),(n,j,k)in zip(left,right):
  require((name,i,m.Signature.value,m.struct.Flags,m.struct.ImplFlags)==(n,j,k.Signature.value,k.struct.Flags,k.struct.ImplFlags),"Method ABI changed: "+name)
  require(bool(m.Rva)==bool(k.Rva),"Body presence changed")
  if not m.Rva:continue
  if exact_body(a,m)==exact_body(b,k):
   untouched+=1;engine+=name.startswith("Battle.Engine");continue
  require(name in wanted and i==(int(targets[name]["token"],16)&0xffffff),"Unapproved raw method or overload changed: "+name);changed.add(name)
 require(changed==wanted and engine==547,"Target or original engine coverage mismatch")
 for name in ["TypeDef","Field"]:
  x=getattr(a.net.mdtables,name).rows;y=getattr(b.net.mdtables,name).rows
  require(len(x)==len(y),name+" count changed")
  for p,q in zip(x,y):
   require(str(p.TypeName if name=="TypeDef" else p.Name)==str(q.TypeName if name=="TypeDef" else q.Name),name+" token identity changed")
   if name=="Field":require(p.Signature.value==q.Signature.value,"Field signature changed")
 # No helper may feed a battle-engine call or rewrite a download-state getter.
 helper=dnfile.dnPE(data=(helper_path).read_bytes())
 for _,_,m in methods(helper):
  if not m.Rva:continue
  for ins in decode(helper,m):
   if ins.opcode.name not in ("call","callvirt","newobj"):continue
   token=instruction_value(ins)
   if not isinstance(token,int)or token>>24!=10:continue
   ref=member_reference(helper,token)
   require(not str(ref.Name).startswith("set_IsAssetFull") and not str(ref.Name).startswith("set_IsDownloadOver"),"Download state mutation in helper")
 return dict(allowedChangedMethods=sorted(changed),originalEngineMethodsExact=engine,unrelatedRawMethodBodiesExact=untouched,originalMetadataABIAndTokensPreserved=True)
def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument("--base-apk",type=Path,required=True);ap.add_argument("--input-managed",type=Path);ap.add_argument("--resource-overlay",type=Path,required=True);ap.add_argument("--expected-sha256",required=True);ap.add_argument("--output-directory",type=Path,required=True);ap.add_argument("--tool-root",type=Path,required=True);args=ap.parse_args()
 base=args.base_apk.resolve();out=args.output_directory.resolve();policy=json.loads((SOURCE/"catalogue.json").read_text())
 require(args.expected_sha256==policy["fixedInputAPKSHA256"],"Unreviewed base identity")
 with base.open("rb")as f:require(hashlib.file_digest(f,"sha256").hexdigest()==args.expected_sha256,"Input APK differs")
 require(out.is_relative_to(ROOT)and not out.exists(),"Fresh project output required");out.mkdir(parents=True)
 specs=json.loads((SOURCE/"targets.json").read_text());rows=specs.get("targets",specs)if isinstance(specs,dict)else specs
 targets={r.get("fullName",r.get("type", "")+"::"+r["method"]):r for r in rows}
 runtime=args.tool_root.resolve()/".tools/native-batch";dotnet=runtime/"dotnet/dotnet";compiler=next((runtime/"dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"));dnlib=runtime/"injector/bin/Debug/net8.0/dnlib.dll"
 def run(label,cmd):
  r=subprocess.run(["rtk","proxy",*map(str,cmd)],capture_output=True,text=True,timeout=180);(out/(label+".log")).write_text(r.stdout+r.stderr);require(r.returncode==0,label+" failed; inspect task log")
 resource=json.loads((args.resource_overlay/"manifest.json").read_text());require(resource["baseAPKSHA256"]==args.expected_sha256,"Resource overlay base differs")
 extra={n:(ROOT/file).read_bytes()for n,file in resource["payloadFiles"].items()}
 require(all(sha(data)==resource["replacementEntries"][n]for n,data in extra.items()),"Resource payload differs")
 with zipfile.ZipFile(base)as z,tempfile.TemporaryDirectory(prefix="hyjs-dungeon-content-")as temp:
  work=Path(temp);before=args.input_managed.read_bytes() if args.input_managed else z.read(ENTRY);host=work/"Assembly-CSharp.dll";host.write_bytes(before);pe=dnfile.dnPE(data=before)
  for name,rid,m in methods(pe):
   if name in targets and rid==(int(targets[name]["token"],16)&0xffffff):
    expected=targets[name].get("cilSHA256",targets[name].get("inputCILSHA256"))
    require(sha(cil(pe,m))==expected,"Input target CIL differs: "+name)
  require(set(targets)<={n for n,_,_ in methods(pe)},"Target not found")
  resource_proof=resources(z,policy,extra);(out/"resource-coverage.json").write_text(json.dumps(resource_proof,ensure_ascii=False,indent=2)+"\n")
  refs=[host]
  for name in ["mscorlib.dll","System.dll","System.Core.dll","UnityEngine.dll"]:
   f=work/name;f.write_bytes(z.read("assets/bin/Data/Managed/"+name));refs.append(f)
  helper=out/HELPER
  run("compile-helper",[dotnet,compiler,"/nologo","/target:library","/nostdlib+","/langversion:3","/deterministic+","/codepage:65001","/out:"+str(helper),*["/reference:"+str(f)for f in refs],SOURCE/"Runtime.cs"])
  shutil.copy2(SOURCE/"Injector.cs",work/"Program.cs");project=work/"Patch.csproj"
  project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run("compile-injector",[dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
  run("inject",[dotnet,work/"bin/Debug/net8.0/Patch.dll",host,helper,out/"Assembly-CSharp.dll"])
  checks=verify(before,(out/"Assembly-CSharp.dll").read_bytes(),targets,helper)
  report=dict(schema="hyjs-dungeon-content-client-v1",managedOnly=True,status="COMPILED_STATIC_VERIFIED_NOT_PACKAGED",baseAPKSHA256=args.expected_sha256,inputManagedSHA256=sha(before),inputDexSHA256=sha(z.read("classes2.dex")),inheritedJavaPublicProtectedABIExact=True,inheritedJavaByteExact=True,payloadFiles={ENTRY:str((out/"Assembly-CSharp.dll").relative_to(ROOT)),"assets/bin/Data/Managed/"+HELPER:str(helper.relative_to(ROOT))},replacementEntries={ENTRY:sha((out/"Assembly-CSharp.dll").read_bytes()),"assets/bin/Data/Managed/"+HELPER:sha(helper.read_bytes())},sourcePins={str(p.relative_to(ROOT)):sha(p.read_bytes())for p in SOURCE.iterdir()if p.is_file()},runtimeVerified=False,resourceProof="resource-coverage.json",**checks)
  (out/"manifest.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n");print(json.dumps({**checks,"resourceBundles":resource_proof["dependencyBundles"],"replacedMissions":resource_proof["replacedMissions"]}))
if __name__=="__main__":main()
