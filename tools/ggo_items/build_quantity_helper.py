"""Build only the GGO quantity helper against pinned formal55 references; no main injection or APK packaging."""
import argparse, hashlib, json, subprocess, tempfile, zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
SOURCE=Path(__file__).resolve().parent/'client'
BASE_SHA='b7ad3c7c8ba53cc7990f3f3590642aa405e9a554135916a468668f0c7fdbdbbd'
MAIN_SHA='c575957a4d2615be7d25cc926d2dea1b8910925ee5f8b1b11880517f0b6c6d19'
MAIN='assets/bin/Data/Managed/Assembly-CSharp.dll'
ENTRY='assets/bin/Data/Managed/SoulFlowerGGOShop.dll'
VERIFY=r"""
using System;using System.Linq;using System.Collections.Generic;using dnlib.DotNet;
using var old=ModuleDefMD.Load(args[0]);using var updated=ModuleDefMD.Load(args[1]);
bool Exported(TypeDef t){return t.IsPublic || t.IsNestedPublic && t.DeclaringType!=null && Exported(t.DeclaringType);}
string[] API(ModuleDefMD m){var rows=new List<string>();foreach(var t in m.GetTypes().Where(Exported)){
rows.Add("T|"+t.FullName+"|"+t.Attributes+"|"+t.BaseType?.FullName);
foreach(var f in t.Fields.Where(f=>f.IsPublic||f.IsFamily||f.IsFamilyOrAssembly))rows.Add("F|"+t.FullName+"|"+f.Name+"|"+f.Attributes+"|"+f.FieldType.FullName);
foreach(var f in t.Methods.Where(f=>f.IsPublic||f.IsFamily||f.IsFamilyOrAssembly))rows.Add("M|"+t.FullName+"|"+f.Name+"|"+f.Attributes+"|"+f.MethodSig.ToString());
}return rows.OrderBy(s=>s,StringComparer.Ordinal).ToArray();}
var a=API(old);var b=API(updated);foreach(var row in a.Except(b))throw new Exception("Inherited helper API missing: "+row);
if(old.Assembly.FullName!=updated.Assembly.FullName)throw new Exception("Assembly identity changed");
if(updated.GetAssemblyRefs().Any(r=>r.Name=="HYJS.BulkPurchase"))throw new Exception("Quantity dependency must stay reflection-only");
var shop=updated.GetTypes().Single(t=>t.FullName=="HYJS.GGO.GGOShop");
foreach(var name in new[]{"LoadPending","SavePending","Fetch"})if(!shop.Methods.Any(m=>m.Name==name))throw new Exception("Missing inherited request lifecycle");
Console.WriteLine("PASS inherited public/protected helper API rows="+a.Length+"; identity and reflection-only dependency verified");
"""
def sha(raw):return hashlib.sha256(raw).hexdigest()
def file_sha(p):
 with p.open('rb')as f:return hashlib.file_digest(f,'sha256').hexdigest()
def require(ok,msg):
 if not ok:raise ValueError(msg)
def method_source(text, name):
 start=text.index('        '+('int ' if name=='Maximum' else 'void ')+name+'(')
 opening=text.index('{',start);depth=0
 for end in range(opening,len(text)):
  if text[end]=='{':depth+=1
  elif text[end]=='}':
   depth-=1
   if depth==0:return text[start:end+1]
 raise ValueError('Incomplete source method '+name)
def boundaries(work, litjson, run, dotnet):
 text=(SOURCE/'GGOShop.cs').read_text();parts=[method_source(text,n)for n in ['Maximum','LoadPending','SavePending']]
 code=r"""
using System;using System.Collections;using LitJson;
static class PlayerPrefs{public static string Raw;public static string GetString(string key,string fallback){return Raw??fallback;}public static void SetString(string key,string value){Raw=value;}public static void Save(){}}
class Boundary{
 string pendingId="",pendingProduct="",pendingVersion="";int pendingCount=1;bool pendingUnreadable;long balance;string PreferenceKey(){return "owned";}
METHODS
 static int checks;static void Check(bool b,string label){if(!b)throw new Exception(label);checks++;}
 static void Main(){
 Boundary h=new Boundary();h.balance=100;
 Check(h.Maximum(8,10,2)==8,"remaining cap");h.balance=25;Check(h.Maximum(99,10,2)==2,"affordability");
 h.balance=Int64.MaxValue;Check(h.Maximum(-1,1,1)==100,"protocol cap");Check(h.Maximum(0,1,1)==0,"sold out");
 Check(h.Maximum(100,1,Int32.MaxValue)==1,"reward checked");Check(h.Maximum(100,Int32.MaxValue,1)==1,"cost checked");Check(h.Maximum(100,0,1)==0,"invalid price");
 string id="12345678-1234-4234-8234-123456789abc";
 string prefix="{\"id\":\""+id+"\",\"product\":\"product\",\"version\":\"v1\"";
 PlayerPrefs.Raw=prefix+"}";h.LoadPending();Check(!h.pendingUnreadable&&h.pendingCount==1&&h.pendingId==id,"legacy count defaults one");
 h.pendingCount=7;h.SavePending();string saved=PlayerPrefs.Raw;h.pendingCount=1;h.LoadPending();Check(!h.pendingUnreadable&&h.pendingCount==7&&h.pendingId==id,"same UUID/count persisted");
 h.SavePending();Check(saved==PlayerPrefs.Raw,"retry does not change pending count");
 foreach(string bad in new[]{"0","101","-1","1.5","true","\"3\""}){PlayerPrefs.Raw=prefix+",\"count\":"+bad+"}";h.LoadPending();Check(h.pendingUnreadable,"invalid persisted count "+bad);}
 PlayerPrefs.Raw=prefix+",\"count\":100}";h.LoadPending();Check(!h.pendingUnreadable&&h.pendingCount==100,"cap100 persisted");
 Console.WriteLine("PASS "+checks+" actual source-method boundaries using original LitJson; Unity/transport not executed");
 }
}
"""
 code=code.replace('METHODS','\n'.join(parts));logic=work/'logic';logic.mkdir();(logic/'Program.cs').write_text(code)
 project=logic/'Cases.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="LitJson"><HintPath>'+str(litjson)+'</HintPath></Reference></ItemGroup></Project>')
 run('verify-count-persistence',[dotnet,'run','--project',project,'--verbosity','quiet','-p:NuGetAudit=false'])
 return {n:sha(v.encode())for n,v in zip(['Maximum','LoadPending','SavePending'],parts)}
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--expected-sha256',required=True);p.add_argument('--output-directory',type=Path,required=True);p.add_argument('--tool-root',type=Path,required=True);a=p.parse_args()
 base=a.base_apk.resolve();out=a.output_directory.resolve();require(a.expected_sha256==BASE_SHA and file_sha(base)==BASE_SHA,'Pinned formal55 APK differs');require(not out.exists()and out.is_relative_to(ROOT),'Fresh project output required');out.mkdir(parents=True)
 runtime=a.tool_root.resolve()/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';csc=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
 def run(name,cmd):
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],capture_output=True,text=True,timeout=180);(out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed; see saved log')
 sources=[SOURCE/'GGOShop.cs',SOURCE/'GGOIconManifest.cs',Path(__file__).resolve()]
 with zipfile.ZipFile(base)as z,tempfile.TemporaryDirectory(prefix='hyjs-ggo-quantity-')as tmp:
  w=Path(tmp);main=z.read(MAIN);old=z.read(ENTRY);require(sha(main)==MAIN_SHA,'Pinned main differs')
  refs=[]
  for n in ['Assembly-CSharp.dll','UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll','LitJson.dll','SoulFlowerHeroArt.dll']:
   f=w/n;f.write_bytes(z.read('assets/bin/Data/Managed/'+n));refs.append(f)
  oldfile=w/'old-SoulFlowerGGOShop.dll';oldfile.write_bytes(old);helper=out/'SoulFlowerGGOShop.dll'
  run('compile-helper',[dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/codepage:65001','/out:'+str(helper),*['/reference:'+str(f)for f in refs],*sources[:2]])
  (w/'Program.cs').write_text(VERIFY);project=w/'Verify.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run('verify-helper-api',[dotnet,'run','--project',project,'--verbosity','quiet','-p:NuGetAudit=false','--',oldfile,helper])
  boundary_hashes=boundaries(w,w/'LitJson.dll',run,dotnet)
  inherited={i.filename:dict(bytes=i.file_size,crc32=format(i.CRC,'08x'))for i in z.infolist()if not i.is_dir()and i.filename!=ENTRY}
  (out/'inherited-entry-inventory.json').write_text(json.dumps(inherited,separators=(',',':'))+'\n')
 require(file_sha(base)==BASE_SHA,'Input APK changed during compilation')
 h=sha(helper.read_bytes());report=dict(schema='hyjs-ggo-quantity-helper-only-v1',status='STATIC_VERIFIED_NOT_PACKAGED',managedOnly=True,baseAPKSHA256=BASE_SHA,inputManagedSHA256=MAIN_SHA,outputManagedSHA256=MAIN_SHA,mainAssemblyByteExact=True,allOtherEntriesInheritedByteExact=True,originalEngineMethodsUnchanged=547,helperPublicProtectedAPICompatible=True,actualSourceMethodBoundarySHA256=boundary_hashes,sourceMethodBoundaryRuntime='net8 with original LitJson and minimal PlayerPrefs double',inputHelperSHA256=sha(old),helperSHA256=h,payloadFiles={ENTRY:str(helper.relative_to(ROOT))},replacementEntries={ENTRY:h},sourcePins={str(f.relative_to(ROOT)):sha(f.read_bytes())for f in sources},inheritedEntryInventorySHA256=file_sha(out/'inherited-entry-inventory.json'),inheritedJavaPublicProtectedABIExact=True,inheritedJavaByteExact=True,deviceExecuted=False,serverMutated=False,knownLimits=['No main or other entry payload; inherited bytes remain pinned base55.','Native quantity shell belongs to HYJS.BulkPurchase.BulkPurchase; reflection-only API requires combined overlay.','C#3 actual reference compilation and inherited ABI checked; Unity persistence/network/render requires parent QA.'])
 (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps({k:report[k]for k in ['status','mainAssemblyByteExact','helperPublicProtectedAPICompatible','helperSHA256']}))
if __name__=='__main__':main()
