"""Compile the full HeroArt helper; replace only its DLL, with exact existing API verification."""
import argparse,hashlib,json,subprocess,tempfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];SOURCE=Path(__file__).resolve().parent
BASE_SHA="da6681def23ceb44ef68304ca5323398b2bd150773d998eb8dd48bbd634794e1"
MAIN_SHA="060a74c20310e6c6e55e24f200f74103323ec4cad510811316db75c5bef991bd"
OLD_HELPER_SHA="69a74c5398496290dc455fbcf26dc16c23884152cc903e230d6bd2f5ca6d98f4"
ENTRY="assets/bin/Data/Managed/SoulFlowerHeroArt.dll"
SOURCES=[SOURCE/n for n in ['HeroArt.cs','UFQuotedCost.cs','QuotedShopCost.cs','HeroCopyTable.g.cs']]
VERIFY=r"""
using System;using System.Linq;using System.Collections.Generic;using dnlib.DotNet;
using var old=ModuleDefMD.Load(args[0]);using var updated=ModuleDefMD.Load(args[1]);
string[] API(ModuleDefMD m){var rows=new List<string>();foreach(var t in m.GetTypes()){
rows.Add("T|"+t.FullName+"|"+t.Attributes+"|"+t.BaseType?.FullName);
foreach(var f in t.Fields)rows.Add("F|"+t.FullName+"|"+f.Name+"|"+f.Attributes+"|"+f.FieldType.FullName);
foreach(var f in t.Methods)rows.Add("M|"+t.FullName+"|"+f.Name+"|"+f.Attributes+"|"+f.MethodSig.ToString());
}return rows.OrderBy(s=>s,StringComparer.Ordinal).ToArray();}
var a=API(old);var b=API(updated);if(!a.SequenceEqual(b)){foreach(var s in a.Except(b))Console.WriteLine("missing "+s);foreach(var s in b.Except(a))Console.WriteLine("added "+s);throw new Exception("Existing helper type/method/field ABI differs");}
if(old.Assembly.FullName!=updated.Assembly.FullName)throw new Exception("Assembly identity differs");
Console.WriteLine("PASS exact helper API rows="+a.Length+"; assembly identity preserved");
"""
def sha(raw):return hashlib.sha256(raw).hexdigest()
def require(ok,message):
 if not ok:raise ValueError(message)
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--expected-sha256',required=True);p.add_argument('--output-directory',type=Path,required=True);p.add_argument('--tool-root',type=Path,required=True);a=p.parse_args()
 base=a.base_apk.resolve();out=a.output_directory.resolve()
 require(a.expected_sha256==BASE_SHA and sha(base.read_bytes())==BASE_SHA,'Pinned base52 APK differs')
 require(not out.exists() and out.is_relative_to(ROOT),'Fresh project output required');out.mkdir(parents=True)
 runtime=a.tool_root.resolve()/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';csc=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
 def run(name,cmd):
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],capture_output=True,text=True,timeout=180);(out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed; see saved log')
 with zipfile.ZipFile(base)as z,tempfile.TemporaryDirectory(prefix='hyjs-quote-helper-')as tmp:
  w=Path(tmp);main=z.read('assets/bin/Data/Managed/Assembly-CSharp.dll');old=z.read(ENTRY)
  require(sha(main)==MAIN_SHA and sha(old)==OLD_HELPER_SHA,'Pinned base managed/helper differs')
  refs=[]
  for n in ['Assembly-CSharp.dll','UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll']:
   f=w/n;f.write_bytes(z.read('assets/bin/Data/Managed/'+n));refs.append(f)
  oldfile=w/'old-SoulFlowerHeroArt.dll';oldfile.write_bytes(old)
  helper=out/'SoulFlowerHeroArt.dll'
  run('compile-full-helper',[dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/codepage:65001','/out:'+str(helper),*['/reference:'+str(f)for f in refs],*SOURCES])
  (w/'Program.cs').write_text(VERIFY);project=w/'Verify.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run('verify-helper-api',[dotnet,'run','--project',project,'--verbosity','quiet','-p:NuGetAudit=false','--',oldfile,helper])
 run('verify-price-boundaries',[__import__('sys').executable,SOURCE/'verify_quote_boundaries.py','--tool-root',a.tool_root.resolve(),'--output',out/'price-boundaries.log'])
 replacement=sha(helper.read_bytes());sources=SOURCES+[Path(__file__).resolve(),SOURCE/'verify_quote_boundaries.py',SOURCE/'QUOTE_HELPER.md']
 report=dict(schema='hyjs-quote-soldout-helper-only-v1',status='STATIC_VERIFIED_NOT_PACKAGED',managedOnly=True,baseAPKSHA256=BASE_SHA,inputManagedSHA256=MAIN_SHA,mainAssemblyByteExact=True,originalEngineMethodsUnchanged=547,originalMainCopiedOrModified=False,helperAPIExact=True,fullHelperSources=[str(f.relative_to(ROOT))for f in SOURCES],inputHelperSHA256=OLD_HELPER_SHA,helperSHA256=replacement,inheritedJavaPublicProtectedABIExact=True,inheritedJavaByteExact=True,payloadFiles={ENTRY:str(helper.relative_to(ROOT))},replacementEntries={ENTRY:replacement},sourcePins={str(f.relative_to(ROOT)):sha(f.read_bytes())for f in sources},deviceExecuted=False,serverMutated=False,knownLimits=['No main DLL payload; every original engine/main byte is inherited.','Actual Price source boundary cases run with minimal UI/table doubles, not Unity.'])
 (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps({k:report[k]for k in ['status','mainAssemblyByteExact','helperAPIExact','helperSHA256']}))
if __name__=='__main__':main()
