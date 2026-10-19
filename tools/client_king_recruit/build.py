"""Overlay king world selection and three original portraits; no APK packaging/install."""
import argparse, hashlib, json, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body,require
SOURCE=Path(__file__).resolve().parent
TARGETS=['UI.TabKingsRecruit::'+name for name in ['UpdateAll','OnMsgCallback','RecruitCallback','OnRecruitOnce','OnRecruitTen','OnRewardBtn','OnPreviewBtn','OnBattlePreviewBtn']]+['UI.RecruitResultCardItem::SetHeroBuyId']
def sha(raw):return hashlib.sha256(raw).hexdigest()
def verify(before,after,pins):
 a,b=[dnfile.dnPE(data=x) for x in (before,after)]
 left,right=methods(a),methods(b)
 require(len(left)==len(right),'Original method count changed')
 for table in ['TypeDef','Field','MethodDef']:
  require(len(getattr(a.net.mdtables,table).rows)==len(getattr(b.net.mdtables,table).rows),'Original inventory changed '+table)
 require(not getattr(b.net.mdtables,'FieldPtr',None),'Old Unity FieldPtr forbidden')
 changed=[];engine=0
 for (name,rid,old),(name2,rid2,new) in zip(left,right):
  require((name,rid,old.Signature.value,old.struct.Flags,old.struct.ImplFlags)==(name2,rid2,new.Signature.value,new.struct.Flags,new.struct.ImplFlags),'Original ABI/token changed '+name)
  if not old.Rva:require(not new.Rva,'New original method body');continue
  x,y=exact_body(a,old),exact_body(b,new)
  if name in TARGETS:
   require(sha(x)==pins[name] and x!=y,'Exact king target pin/change differs '+name)
   changed.append({'method':name,'beforeSHA256':sha(x),'afterSHA256':sha(y)})
  else:require(x==y,'Unrelated original method changed '+name)
  if name.startswith('Battle.Engine'):engine+=1;require(x==y,'Engine changed')
 require(engine==547 and len(changed)==len(TARGETS),'King target/engine coverage differs')
 require(len(b.net.mdtables.AssemblyRef.rows)>=len(a.net.mdtables.AssemblyRef.rows),'Original assembly references removed')
 for old,new in zip(a.net.mdtables.AssemblyRef,b.net.mdtables.AssemblyRef):
  require((str(old.Name),old.struct.Flags,old.PublicKey.value)==(str(new.Name),new.struct.Flags,new.PublicKey.value),'Existing assembly identity changed')
 return {'changes':changed,'originalEngineMethodsUnchanged':engine,'originalMetadataABIAndTokensPreserved':True}
def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--base-apk',type=Path,required=True);parser.add_argument('--input-managed',type=Path)
 parser.add_argument('--base-sha256',required=True);parser.add_argument('--output-directory',type=Path,required=True)
 parser.add_argument('--helper-only',action='store_true',help='Compile helper against an already composed APK; do not rewrite original main')
 args=parser.parse_args();require(sha(args.base_apk.read_bytes())==args.base_sha256,'APK identity differs')
 out=args.output_directory.resolve();require(not out.exists(),'Use fresh output directory');out.mkdir(parents=True)
 pins=json.loads((SOURCE/'pins.json').read_text())
 runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
 def run(name,cmd):
  result=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=120)
  (out/(name+'.log')).write_text(result.stdout+result.stderr);require(result.returncode==0,name+' failed')
 with zipfile.ZipFile(args.base_apk) as archive,tempfile.TemporaryDirectory(prefix='hyjs-king-world-') as temporary:
  work=Path(temporary);before=args.input_managed.read_bytes() if args.input_managed else archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
  original=work/'Assembly-CSharp.dll';original.write_bytes(before)
  pe=dnfile.dnPE(data=before)
  for name,_,method in methods(pe):
   if name in TARGETS and not args.helper_only:require(sha(exact_body(pe,method))==pins[name],'Pinned input differs '+name)
  references=[original]
  for name in ['UnityEngine.dll','mscorlib.dll','Assembly-CSharp-firstpass.dll','System.dll','System.Core.dll','LitJson.dll','SoulFlowerHeroArt.dll']:
   target=work/name;target.write_bytes(archive.read('assets/bin/Data/Managed/'+name));references.append(target)
  helper=out/'SoulFlowerKingRecruit.dll'
  run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(helper),*['/reference:'+str(x) for x in references],SOURCE/'KingRecruitWorldUI.cs'])
  if args.helper_only:
   result={'schema':'hyjs-king-world-helper-only-v1','status':'COMPILED_NOT_PACKAGED','inputAPK_SHA256':args.base_sha256,'inputManagedSHA256':sha(before),'originalMainCopiedOrModified':False,'helperSHA256':sha(helper.read_bytes()),'replacementEntries':['assets/bin/Data/Managed/SoulFlowerKingRecruit.dll'],'phoneVerified':False,'sourcePins':{str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in [SOURCE/'KingRecruitWorldUI.cs',Path(__file__).resolve()]}}
   (out/'manifest.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps(result,ensure_ascii=False));return
  shutil.copy2(SOURCE/'Injector.cs',work/'Program.cs')
  project=work/'Patch.csproj';project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
  run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',original,out/'Assembly-CSharp.dll'])
 result={'schema':'hyjs-king-world-and-original-art-v1','status':'STATIC_VERIFIED_NOT_PACKAGED','inputAPK_SHA256':args.base_sha256,'inputManagedSHA256':sha(before),'outputManagedSHA256':sha((out/'Assembly-CSharp.dll').read_bytes()),'helperSHA256':sha(helper.read_bytes()),'replacementEntries':['assets/bin/Data/Managed/Assembly-CSharp.dll','assets/bin/Data/Managed/SoulFlowerKingRecruit.dll'],'phoneVerified':False,'sourcePins':{str(p.relative_to(ROOT)):sha(p.read_bytes()) for p in [SOURCE/'KingRecruitWorldUI.cs',SOURCE/'Injector.cs',SOURCE/'pins.json',Path(__file__).resolve()]},**verify(before,(out/'Assembly-CSharp.dll').read_bytes(),pins)}
 (out/'manifest.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps(result,ensure_ascii=False))
if __name__=='__main__':main()
