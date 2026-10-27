"""Build the scoped 77 managed overlay from the pinned released 76 APK."""
import argparse, hashlib, json, subprocess, tempfile, zipfile
from pathlib import Path
import sys
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
import dnfile

BASE = '5740a0f495f58de23049a16d5ba10aec79ebec5911d8ecf92bb197ccfa85654f'
MANAGED = 'assets/bin/Data/Managed/'
SOURCES = {
 'SoulFlowerActivity.dll': ['tools/client_activity/ActivityGuard.cs','tools/client_activity/InstanceSelector.cs'],
 'HYJS.Commerce66.dll': ['tools/client_shop_layout/RuntimeCommerce.cs'],
 'SoulFlowerGGOShop.dll': ['tools/ggo_items/client/GGOShop.cs','tools/ggo_items/client/GGOIconManifest.cs','tools/ggo_items/client/PetIconManifest75.cs','tools/ggo_items/client/TrialShopRefresh75.cs','tools/ggo_items/client/PetPortraitAssets75.cs'],
 'HYJS.BulkPurchase.dll': ['tools/client_bulk_purchase/Runtime.cs'],
}
VERIFY = r'''using System;using System.Linq;using System.Collections.Generic;using dnlib.DotNet;
using var a=ModuleDefMD.Load(args[0]);using var b=ModuleDefMD.Load(args[1]);
bool Exported(TypeDef t){return t.IsPublic || t.IsNestedPublic && t.DeclaringType!=null && Exported(t.DeclaringType);}
string[] API(ModuleDefMD m){var rows=new List<string>();foreach(var t in m.GetTypes().Where(Exported)){rows.Add("T|"+t.FullName+"|"+t.Attributes+"|"+t.BaseType?.FullName);foreach(var f in t.Fields.Where(x=>x.IsPublic||x.IsFamily||x.IsFamilyOrAssembly))rows.Add("F|"+t.FullName+"|"+f.Name+"|"+f.Attributes+"|"+f.FieldType.FullName);foreach(var x in t.Methods.Where(x=>x.IsPublic||x.IsFamily||x.IsFamilyOrAssembly))rows.Add("M|"+t.FullName+"|"+x.Name+"|"+x.Attributes+"|"+x.MethodSig);}return rows.OrderBy(s=>s,StringComparer.Ordinal).ToArray();}
if(a.Assembly.FullName!=b.Assembly.FullName)throw new Exception("identity changed");foreach(var x in API(a).Except(API(b)))throw new Exception("inherited API missing "+x);Console.WriteLine("PASS inherited public/protected API "+API(a).Length);'''
def sha(data): return hashlib.sha256(data).hexdigest()
def digest(p):
 with p.open('rb') as f: return hashlib.file_digest(f,'sha256').hexdigest()
def main():
 p=argparse.ArgumentParser();p.add_argument('--base-apk',type=Path,required=True);p.add_argument('--output-directory',type=Path,required=True);p.add_argument('--tool-root',type=Path,required=True);a=p.parse_args()
 base=a.base_apk.resolve();out=a.output_directory.resolve();assert digest(base)==BASE,'base SHA mismatch';assert out.is_relative_to(ROOT) and not out.exists(),'fresh project output required';out.mkdir(parents=True)
 runtime=a.tool_root.resolve()/'.tools/native-batch';dot=runtime/'dotnet/dotnet';csc=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
 def run(name,cmd):
  x=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=180);(out/(name+'.log')).write_text(x.stdout+x.stderr);assert x.returncode==0,name+' failed, see log'
 with tempfile.TemporaryDirectory(prefix='hyjs-growth77-') as temp,zipfile.ZipFile(base) as z:
  w=Path(temp);refs=w/'refs';refs.mkdir()
  for n in z.namelist():
   if n.startswith(MANAGED) and n.endswith('.dll'):(refs/Path(n).name).write_bytes(z.read(n))
  main_before=(refs/'Assembly-CSharp.dll').read_bytes();dex_sha=sha(z.read('classes2.dex'))
  for assembly, files in SOURCES.items():
   cmd=[dot,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/codepage:65001','/out:'+str(out/assembly)]
   if assembly=='SoulFlowerGGOShop.dll':cmd.append('/resource:'+str(ROOT/'tools/ggo_items/assets/ggo_credit_hud-source.png')+',credit_hud.png')
   cmd += ['/r:'+str(f) for f in sorted(refs.glob('*.dll')) if f.name != assembly]
   cmd += [ROOT/f for f in files];run('compile-'+assembly,cmd)
  project=w/'verify';project.mkdir();(project/'Program.cs').write_text(VERIFY);(project/'Verify.csproj').write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run('build-verify',[dot,'build',project/'Verify.csproj','-o',project/'out','--verbosity','quiet','-p:NuGetAudit=false'])
  for assembly in SOURCES:run('abi-'+assembly,[dot,project/'out/Verify.dll',refs/assembly,out/assembly])
  project2=w/'inject';project2.mkdir();(project2/'Program.cs').write_text((ROOT/'tools/client_activity/Growth77Injector.cs').read_text());(project2/'Inject.csproj').write_text((project/'Verify.csproj').read_text())
  run('build-injector',[dot,'build',project2/'Inject.csproj','-o',project2/'out','--verbosity','quiet','-p:NuGetAudit=false'])
  run('inject-main',[dot,project2/'out/Inject.dll',refs/'Assembly-CSharp.dll',out/'SoulFlowerActivity.dll',out/'Assembly-CSharp.dll'])
  before,after=dnfile.dnPE(data=main_before),dnfile.dnPE(str(out/'Assembly-CSharp.dll'))
  old,new=methods(before),methods(after);assert len(old)==len(new)
  changed=[];engine=0
  for (name,idx,x),(name2,idx2,y) in zip(old,new):
   assert (name,idx,x.Signature.value,x.struct.Flags)==(name2,idx2,y.Signature.value,y.struct.Flags)
   if x.Rva:
    same=exact_body(before,x)==exact_body(after,y)
    if not same:changed.append(name)
    elif name.startswith('Battle.Engine'):engine+=1
  expected=['UI.UIActivityBattleRace::MsgCallback','UI.AcRechargeSingleItem::SetActivityItem'];assert set(changed)==set(expected),(changed,expected);assert engine==547,engine
  assert all(not (row.struct.Flags & 1 and not row.PublicKey.value) for row in after.net.mdtables.AssemblyRef.rows), 'invalid assembly public key'
  assert not getattr(after.net.mdtables,'FieldPtr',None) and not getattr(after.net.mdtables,'MethodPtr',None)
  assert len(before.net.mdtables.Field.rows)==len(after.net.mdtables.Field.rows) and len(before.net.mdtables.TypeDef.rows)==len(after.net.mdtables.TypeDef.rows)
  paths={MANAGED+name:str((out/name).relative_to(ROOT)) for name in ['Assembly-CSharp.dll',*SOURCES]};hashes={entry:digest(ROOT/path) for entry,path in paths.items()}
  pins=[Path(__file__).resolve(),ROOT/'tools/client_activity/Growth77Injector.cs',ROOT/'tools/ggo_items/assets/ggo_credit_hud-source.png']+[ROOT/f for files in SOURCES.values() for f in files]
  manifest=dict(schema='hyjs-activity-growth77-overlay-v1',status='SCOPE_VERIFIED_NOT_PACKAGED',baseAPKSHA256=BASE,inputManagedSHA256=sha(main_before),inputDexSHA256=dex_sha,managedOnly=True,inheritedJavaPublicProtectedABIExact=True,inheritedJavaByteExact=True,allowedChangedMethods=expected,originalEngineMethodsExact=engine,helperPublicProtectedAPICompatible=True,payloadFiles=paths,replacementEntries=hashes,sourcePins={str(f.relative_to(ROOT)):digest(f) for f in pins},deviceTested=False)
  (out/'manifest.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
 assert digest(base)==BASE
 print(json.dumps({'status':manifest['status'],'changedMethods':changed,'originalEngineMethodsExact':engine,'replacementEntries':hashes},ensure_ascii=False))
if __name__=='__main__':main()
