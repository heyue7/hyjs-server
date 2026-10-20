"""Run the composed original DAO under the existing .NET host, without Unity UI or battle claims."""
import argparse,csv,hashlib,json,os,shutil,subprocess,tempfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--main',type=Path,required=True);p.add_argument('--helper',type=Path,required=True);p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args();assert not a.output.exists()
 data=json.loads((Path(__file__).parent/'hero_copy.json').read_text());expected={str(r['id']):r['new']for r in data['tableSkillCastProposal']['rows']}
 table=list(csv.reader((ROOT/'analysis/tables/TableSkillCast.tsv').open(),delimiter='\t'));rows=[r for r in table[5:]if r and r[0]in expected];control=next(r for r in table[5:]if r and r[0]=='1359');rows.append(control);expected[control[0]]=float(control[6])
 with tempfile.TemporaryDirectory(prefix='hyjs-native-cast-proof-')as temporary:
  work=Path(temporary);runtime=work/'data';runtime.mkdir()
  with zipfile.ZipFile(a.base_apk)as z:
   for name in z.namelist():
    if name.startswith('assets/bin/Data/Managed/')and name.endswith('.dll'):(runtime/Path(name).name).write_bytes(z.read(name))
  shutil.copy2(a.main,runtime/'Assembly-CSharp.dll');shutil.copy2(a.helper,runtime/'SoulFlowerHeroArt.dll')
  (work/'input.json').write_text(json.dumps({'rows':rows,'expected':expected}))
  (work/'Program.cs').write_text(r'''using System.Reflection;
using System.Runtime.Loader;
using System.Text.Json;
var directory=Path.GetFullPath(args[0]);
AssemblyLoadContext.Default.Resolving+=(context,name)=>{var f=Path.Combine(directory,name.Name+".dll");return File.Exists(f)?context.LoadFromAssemblyPath(f):null;};
var main=AssemblyLoadContext.Default.LoadFromAssemblyPath(Path.Combine(directory,"Assembly-CSharp.dll"));
var type=main.GetType("Dict.Dao.TableSkillCastDao",true)!;
var dao=Activator.CreateInstance(type)!;
var parse=type.GetMethod("ParseRowData",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic)!;
using var document=JsonDocument.Parse(File.ReadAllText(args[1]));
var expected=document.RootElement.GetProperty("expected");var totals=new Dictionary<string,double>();int count=0;
foreach(var input in document.RootElement.GetProperty("rows").EnumerateArray()){
 var columns=input.EnumerateArray().Select(x=>x.GetString()!).ToArray();
 var row=parse.Invoke(dao,new object[]{columns})!;var rt=row.GetType();
 var id=(string)rt.GetField("SkillCastId")!.GetValue(row)!;var skill=(string)rt.GetField("SkillId")!.GetValue(row)!;
 var factor=(double)rt.GetField("ExportFactor")!.GetValue(row)!;
 if(id!=columns[0]||skill!=columns[1]||factor!=expected.GetProperty(id).GetDouble())throw new Exception("Original DAO factor mismatch "+id);
 if(id!="1359"){if(!totals.ContainsKey(skill))totals[skill]=0;totals[skill]+=factor;}count++;
}
foreach(var pair in totals)if(pair.Value!=(pair.Key.EndsWith("_3")?620:100))throw new Exception("Native skill total mismatch "+pair.Key);
Console.WriteLine(JsonSerializer.Serialize(new{nativeDaoRowsChecked=count,unrelatedControlPreserved=true,nativeSkillTotals=totals,engineMethodsExecuted=false}));
''')
  (work/'Probe.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup></Project>')
  dotnet=ROOT/'.tools/native-batch/dotnet/dotnet';build=subprocess.run(['rtk','proxy',str(dotnet),'build',str(work/'Probe.csproj'),'--nologo','--verbosity','quiet','-p:NuGetAudit=false'],capture_output=True,text=True,timeout=120);assert build.returncode==0,build.stdout+build.stderr
  run=subprocess.run(['rtk','proxy',str(dotnet),str(work/'bin/Debug/net8.0/Probe.dll'),str(runtime),str(work/'input.json')],capture_output=True,text=True,timeout=40)
  if run.returncode:raise RuntimeError(run.stdout+run.stderr)
  proof=json.loads(run.stdout);proof.update(mainSHA256=hashlib.sha256(a.main.read_bytes()).hexdigest(),helperSHA256=hashlib.sha256(a.helper.read_bytes()).hexdigest(),status='PASS_ORIGINAL_DAO_RUNTIME')
  a.output.parent.mkdir(parents=True,exist_ok=True);a.output.write_text(json.dumps(proof,indent=2)+'\n');print(json.dumps(proof))
if __name__=='__main__':main()
