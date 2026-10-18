"""Repair host type bindings in the explicitly pinned composed client."""
import argparse, hashlib, json, shutil, subprocess, sys, tempfile
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body,require
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument("--input-managed",type=Path,required=True);p.add_argument("--input-sha256",required=True);p.add_argument("--output-directory",type=Path,required=True);a=p.parse_args()
 require(sha(a.input_managed)==a.input_sha256,"Pinned input differs");out=a.output_directory;require(not out.exists(),"Fresh output required");out.mkdir(parents=True)
 dot=ROOT/".tools/native-batch/dotnet/dotnet";source=Path(__file__).with_name("BindingRepair.cs");dnlib=ROOT/".tools/native-batch/injector/bin/Debug/net8.0/dnlib.dll"
 with tempfile.TemporaryDirectory(prefix="hyjs-binding-repair-") as t:
  w=Path(t);shutil.copy2(source,w/"Program.cs");project=w/"Patch.csproj";project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  for name,cmd in [("build",[dot,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"]),("repair",[dot,w/"bin/Debug/net8.0/Patch.dll",a.input_managed.resolve(),(out/"Assembly-CSharp.dll").resolve()])]:
   r=subprocess.run(["rtk","proxy",*map(str,cmd)],capture_output=True,text=True,timeout=90);(out/(name+".log")).write_text(r.stdout+r.stderr);require(r.returncode==0,name+" failed")
 before,after=[dnfile.dnPE(str(x))for x in (a.input_managed,out/"Assembly-CSharp.dll")];changed=[]
 for i,(x,y)in enumerate(zip(before.net.mdtables.TypeRef.rows,after.net.mdtables.TypeRef.rows),1):
  require((str(x.TypeName),str(x.TypeNamespace))==(str(y.TypeName),str(y.TypeNamespace)),"Type identity changed")
  if (x.ResolutionScope.table.name,x.ResolutionScope.row_index)!=(y.ResolutionScope.table.name,y.ResolutionScope.row_index):
   require(x.ResolutionScope.table.name=="ModuleRef" and str(x.ResolutionScope.row.Name) in {"SoulFlowerDisplay.dll","SoulFlowerActivity.dll"} and y.ResolutionScope.table.name=="Module","Unexpected binding change");changed.append(str(x.TypeNamespace)+"."+str(x.TypeName))
 require(len(changed)==6,"Expected 6 bindings")
 for (n,i,x),(m,j,y)in zip(methods(before),methods(after)):
  require((n,i,x.Signature.value)==(m,j,y.Signature.value),"Method ABI changed")
  if x.Rva:require(exact_body(before,x)==exact_body(after,y),"Method body changed: "+n)
 report={"status":"PASS_STATIC_HOST_BINDINGS_REPAIRED","inputManagedSHA256":a.input_sha256,"outputManagedSHA256":sha(out/"Assembly-CSharp.dll"),"repairedTypeBindings":changed,"allMethodBodiesExact":True,"sourcePins":{str(f.relative_to(ROOT)):sha(f)for f in (Path(__file__).resolve(),source)}};(out/"manifest.json").write_text(json.dumps(report,indent=2)+"\n");print(json.dumps(report))
if __name__=="__main__":main()
