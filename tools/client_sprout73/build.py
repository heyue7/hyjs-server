import argparse,hashlib,json,shutil,subprocess,sys,tempfile,zipfile
from pathlib import Path
import dnfile
from PIL import Image
ROOT=Path(__file__).resolve().parents[2];HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT))
from tools.client_dungeon_access.build import verify,methods,cil
ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll"
def sha(b):return hashlib.sha256(b).hexdigest()
def main():
 ap=argparse.ArgumentParser();ap.add_argument("--base-apk",type=Path,required=True);ap.add_argument("--output-directory",type=Path,required=True);a=ap.parse_args();out=a.output_directory.resolve();assert out.is_relative_to(ROOT) and not out.exists();out.mkdir(parents=True)
 base=a.base_apk.read_bytes();assert sha(base)=="1532a4303f49ed1316163e2ba4dcf0b13504275b0fe975396ec952f1de13b8b1"
 tool=Path("/Users/lirui/LR/hyjs/.tools/native-batch");dotnet=tool/"dotnet/dotnet";csc=next((tool/"dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
 def run(n,cmd):
  r=subprocess.run(["rtk","proxy",*map(str,cmd)],capture_output=True,text=True,timeout=180);(out/(n+".log")).write_text(r.stdout+r.stderr);assert r.returncode==0,str(out/(n+".log"))
 with zipfile.ZipFile(a.base_apk)as z,tempfile.TemporaryDirectory(prefix="hyjs-sprout73-")as temp:
  work=Path(temp);refs=work/"refs";refs.mkdir();before=z.read(ENTRY)
  for n in z.namelist():
   if n.startswith("assets/bin/Data/Managed/")and n.endswith(".dll"):(refs/Path(n).name).write_bytes(z.read(n))
  image=Image.open(ROOT/"tools/ggo_items/assets/ggo_sprout.png").convert("RGBA");image=image.crop(image.getchannel("A").getbbox());image.save(work/"ggo_sprout.png")
  pe=dnfile.dnPE(data=before);targets=[]
  for name,rid,m in methods(pe):
   if name in ["Room.RCUIPlayer::DoStart","Room.RCUIPlayer::DoStop","Dict.Dao.TableCopyExpDao::ParseRowData"]:targets.append(dict(type=name.split("::")[0],method=name.split("::")[1],token=hex(0x6000000|rid),inputCILSHA256=sha(cil(pe,m)),helper="Attach"if name.endswith("DoStart")else ("Detach"if name.endswith("DoStop")else "Experience"),mode="tail",arguments=1))
  assert len(targets)==3;(work/"targets.json").write_text(json.dumps(targets));(out/"targets.json").write_text(json.dumps(targets,indent=2))
  helper=out/"HYJS.Sprout73.dll";run("helper",[dotnet,csc,"/nologo","/target:library","/nostdlib+","/langversion:3","/deterministic+","/out:"+str(helper),"/resource:"+str(work/"ggo_sprout.png")+",ggo_sprout.png",*["/r:"+str(p)for p in sorted(refs.glob("*.dll"))],HERE/"Runtime.cs"])
  shutil.copy2(HERE/"Injector.cs",work/"Program.cs");project=work/"Inject.csproj";project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(tool/"injector/bin/Debug/net8.0/dnlib.dll")+'</HintPath></Reference></ItemGroup></Project>')
  run("injector",[dotnet,"build",project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"]);result=out/"Assembly-CSharp.dll";run("inject",[dotnet,work/"bin/Debug/net8.0/Inject.dll",refs/"Assembly-CSharp.dll",helper,result,work/"targets.json"])
  proof=verify(before,result.read_bytes(),{t["type"]+"::"+t["method"]:t for t in targets},helper);payload={ENTRY:str(result.relative_to(ROOT)),"assets/bin/Data/Managed/HYJS.Sprout73.dll":str(helper.relative_to(ROOT))};report=dict(schema="hyjs-sprout73-managed-v1",baseAPKSHA256=sha(base),inputManagedSHA256=sha(before),outputManagedSHA256=sha(result.read_bytes()),payloadFiles=payload,replacementEntries={n:sha((ROOT/p).read_bytes())for n,p in payload.items()},originalRefsCSharp3Compiled=True,managedOnly=True,apkBuilt=False,deviceTested=False,visualVerified=False,experienceRows=100,experienceAbsoluteMap=True,experienceCoinUnchanged=True,sourcePins={str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in HERE.iterdir() if p.is_file()},**proof);(out/"manifest.json").write_text(json.dumps(report,indent=2)+"\n");print(json.dumps(proof))
if __name__=="__main__":main()
