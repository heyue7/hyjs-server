"""Project the 30 approved orange cast factors at the original DAO row boundary."""
import argparse,hashlib,json,subprocess,tempfile,shutil,sys
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body,require
SOURCE=Path(__file__).resolve().parent
TARGET='Dict.Dao.TableSkillCastDao::ParseRowData'
def sha(b):return hashlib.sha256(b).hexdigest()
def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--input-managed',type=Path,required=True);p.add_argument('--helper',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);a=p.parse_args()
 before=a.input_managed.read_bytes();out=a.output_directory.resolve();require(not out.exists(),'Fresh output required');out.mkdir(parents=True)
 helper=dnfile.dnPE(data=a.helper.read_bytes());require(any(str(m.row.Name)=='AdjustSkillCast' for t in helper.net.mdtables.TypeDef.rows for m in t.MethodList),'Missing compiled factor helper')
 runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
 with tempfile.TemporaryDirectory(prefix='hyjs-three-orange-')as tmp:
  work=Path(tmp);shutil.copy2(SOURCE/'SkillFactorInjector.cs',work/'Program.cs');project=work/'Patch.csproj'
  project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  for name,cmd in [('compile',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false']),('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',a.input_managed.resolve(),out/'Assembly-CSharp.dll'])]:
   r=subprocess.run(['rtk','proxy',*map(str,cmd)],capture_output=True,text=True,timeout=120);(out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed')
 after=(out/'Assembly-CSharp.dll').read_bytes();left,right=[dnfile.dnPE(data=b)for b in [before,after]];changed=[];engine=0
 require(len(methods(left))==len(methods(right)),'Original method inventory differs')
 for (n,i,x),(m,j,y)in zip(methods(left),methods(right)):
  require((n,i,x.Signature.value,x.struct.Flags)==(m,j,y.Signature.value,y.struct.Flags),'Original ABI differs')
  if not x.Rva:continue
  if exact_body(left,x)!=exact_body(right,y):require(n==TARGET,'Unrelated method differs '+n);changed.append(n)
  else:engine+=n.startswith('Battle.Engine')
 require(changed==[TARGET]and engine==547,'Wrong DAO/engine scope');require(len(left.net.mdtables.ModuleRef.rows)==len(right.net.mdtables.ModuleRef.rows),'Self module reference forbidden')
 report={'status':'STATIC_VERIFIED_NOT_PACKAGED','inputManagedSHA256':sha(before),'outputManagedSHA256':sha(after),'allowedChangedMethods':changed,'originalEngineMethodsExact':engine,'helperSHA256':sha(a.helper.read_bytes()),'sourcePins':{str(f.relative_to(ROOT)):sha(f.read_bytes())for f in [SOURCE/'SkillFactorInjector.cs',SOURCE/'build_skill_factors.py',SOURCE/'hero_copy.json']}}
 (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report,ensure_ascii=False))
if __name__=='__main__':main()
