"""Static-only WorldMapPage packaged-field lock overlay; inherits stamina r4."""
import argparse
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_stamina_sweep.build import sha, decode, key, body, instruction_offset_index, BRANCH, methods, exact_body, require, dnfile

SOURCE = Path(__file__).resolve().parent
BASE = ROOT / 'analysis/playability31-20261020/client/overlay-stamina-sweep-r4-request-token'
INPUT_SHA = '301dc5eba69cb4761da2d40bda1a441dae880e35922d417cc0d5fde231bd52ce'
APK_SHA = '9fec499af7473f5fd05da31670e27b3e9eb270ba9b0c9d52bd6f3238e1425285'
PINS = {
 'isSenlinOpen':'60016f53a5bad4924c6ef4b212008efa854b2ca02b6a2d1156c74898e62a2780',
 'isHuoshanOpen':'66c32ea75297482c60d0a7b20b5b72113e538008e36b8e64cde5ad094ea27ea5',
 'isShamoOpen':'e8bea504ab0c95ee02c8be823e0d519d3b137d9e8041d76e6934695a4d536348',
 'isWaiyuOpen':'643521375c795aa983ea8e02d1751308d8a6824fb7d52fe7ffca74c14f2d6c49',
 'onMapClick':'0fdd7dd1c3a837a61af5e1df378edfa8e57a7e5cef82b2071cd40062886c97f3',
 'onTransfer':'7626fc3010f1034da33494584861c7b6d2327bda28b9feff6db6d1cb14dbd302',
}
PINS = {'UI.WorldMapPage::'+n:v for n,v in PINS.items()}

def verify(before, after):
 require(sha(before)==INPUT_SHA, 'Input SHA mismatch')
 left,right=dnfile.dnPE(data=before),dnfile.dnPE(data=after)
 for table in ('TypeDef','Field','MethodDef','Assembly','AssemblyRef','TypeRef','MemberRef','MethodSpec'):
  a,b=getattr(left.net.mdtables,table).rows,getattr(right.net.mdtables,table).rows
  require(len(a)==len(b), 'Metadata inventory changed: '+table)
  for x,y in zip(a,b):
   # Heap offsets may move; resolved identities/signatures and coded indices must stay.
   for attr in ('Name','TypeName','TypeNamespace','Signature','PublicKey','Culture'):
    if hasattr(x,attr):
     u,v=getattr(x,attr),getattr(y,attr)
     require(getattr(u,'value',str(u))==getattr(v,'value',str(v)), 'Metadata identity changed: '+table+' '+attr)
   for attr in ('Flags','ImplFlags','Extends_CodedIndex','FieldList_Index','MethodList_Index','Class_CodedIndex','ResolutionScope_CodedIndex','MajorVersion','MinorVersion','BuildNumber','RevisionNumber'):
    if hasattr(x.struct,attr): require(getattr(x.struct,attr)==getattr(y.struct,attr), 'Metadata ABI changed: '+attr)
 require(not getattr(right.net.mdtables,'FieldPtr',None), 'Unexpected FieldPtr')
 require(left.net.struct.Flags==right.net.struct.Flags, 'CLI flags changed')
 require(left.net.struct.StrongNameSignatureSize==right.net.struct.StrongNameSignatureSize==0,'Strong name changed')
 changes=[];engine=unchanged=0
 for (name,rid,a),(newname,newrid,b) in zip(methods(left),methods(right)):
  require((name,rid)==(newname,newrid),'Method token changed')
  if not a.Rva:
   require(not b.Rva,'New body');continue
  raw1,raw2=exact_body(left,a),exact_body(right,b)
  if name not in PINS:
   require(raw1==raw2,'Unrelated raw body changed: '+name)
   unchanged+=1;engine+=name.startswith('Battle.Engine');continue
  require(sha(raw1)==PINS[name],'Target pin mismatch: '+name)
  old,new=decode(left,a),decode(right,b)
  ok,nk=[key(left,i) for i in old],[key(right,i) for i in new]
  short=name.split('::')[1]
  if short.startswith('is'):
   prefix=short[2:-4].lower()
   expected=ok[:9]+['ldarg.0','ldc.i4.0','stfld '+prefix+'Flag','ldarg.0','ldfld '+prefix+'Text',"ldstr '暂未开放'",'callvirt set_text','ldarg.0','ldfld '+prefix+'Text','callvirt Component::get_gameObject','ldc.i4.1','callvirt GameObject::SetActive','ldarg.0','ldfld '+prefix+'Lock','callvirt GameObject::get_gameObject','ldc.i4.1','callvirt GameObject::SetActive','ret']
   require(nk==expected,'Exact NGUI lock body differs: '+name)
   mapping={i:i for i in range(9)}
   # Every reference comes from an unchanged original NGUI call or original field.
   for i in new:
    token=getattr(i.operand,'value',None)
    if token is not None and i.opcode.name!='ldstr':
     require(any(getattr(o.operand,'value',None)==token for o in old),'New unexpected operand: '+name)
  else:
   anchor,count=(3,9) if short=='onMapClick' else (0,11)
   expected=['ldloc.0','ldc.i4.2','blt','ldloc.0','ldc.i4.5','bgt','ldarg.0','call mapCloseTip','ret'] if short=='onMapClick' else ['ldarg.1',"ldstr 'field_001'",'call String::op_Equality','brtrue','ldarg.1',"ldstr 'field_002'",'call String::op_Equality','brtrue','ldarg.0','call mapCloseTip','ret']
   require(nk[:anchor]==ok[:anchor] and nk[anchor:anchor+count]==expected and nk[anchor+count:]==ok[anchor:],'Guard instruction stream differs: '+name)
   mapping={i:i+(count if i>=anchor else 0) for i in range(len(old))}
   oo,no=instruction_offset_index(old),instruction_offset_index(new)
   for i in range(anchor,anchor+count):
    if new[i].opcode.name in BRANCH: require(no[new[i].operand]==mapping[anchor],'Guard can escape supported-map boundary')
   for i,j in mapping.items():
    if old[i].opcode.name in BRANCH:
     ot=old[i].operand if old[i].opcode.name=='switch' else [old[i].operand]
     nt=new[j].operand if new[j].opcode.name=='switch' else [new[j].operand]
     require([mapping[oo[t]] for t in ot]==[no[t] for t in nt],'Original branch targets changed: '+name)
  for i,j in mapping.items():
   token=getattr(old[i].operand,'value',None)
   if token is not None: require(getattr(new[j].operand,'value',None)==token,'Original operand token changed')
  require(len(body(left,a).exception_handlers)==len(body(right,b).exception_handlers)==0,'Unexpected target EH')
  require(getattr(body(left,a).local_var_sig_tok,'value',None)==getattr(body(right,b).local_var_sig_tok,'value',None),'Target local signature changed')
  changes.append(dict(method=name,beforeSHA256=sha(raw1),afterSHA256=sha(raw2)))
 require(engine==547 and len(changes)==6,'Coverage count differs')
 return dict(changes=changes,originalEngineMethodsUnchanged=engine,unrelatedRawMethodBodiesUnchanged=unchanged,originalMetadataABIAndTokensPreserved=True,originalBranchAndGuardBoundariesVerified=True)

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--output-directory',type=Path,required=True)
 p.add_argument('--base-apk',type=Path,default=ROOT/'apks/build/hyjs-local.apk')
 args=p.parse_args();out=args.output_directory.resolve()
 require(not out.exists(),'Fresh output required');out.mkdir(parents=True)
 before=(BASE/'Assembly-CSharp.dll').read_bytes()
 require(sha(before)==INPUT_SHA and sha(args.base_apk.read_bytes())==APK_SHA,'Pinned inputs required')
 runtime=ROOT/'.tools/native-batch';dotnet=runtime/'dotnet/dotnet';dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
 def run(name,cmd):
  r=subprocess.run(['rtk','proxy',*map(str,cmd)],cwd=ROOT,capture_output=True,text=True,timeout=120)
  (out/(name+'.log')).write_text(r.stdout+r.stderr);require(r.returncode==0,name+' failed')
 with tempfile.TemporaryDirectory(prefix='hyjs-field-entry-') as temp:
  work=Path(temp);shutil.copy2(SOURCE/'Injector.cs',work/'Program.cs')
  project=work/'Patch.csproj'
  project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
  run('compile-injector',[dotnet,'build',project,'--nologo','--verbosity','quiet','-p:NuGetAudit=false'])
  run('inject',[dotnet,work/'bin/Debug/net8.0/Patch.dll',BASE/'Assembly-CSharp.dll',out/'Assembly-CSharp.dll'])
 after=(out/'Assembly-CSharp.dll').read_bytes();checks=verify(before,after)
 inherited=json.loads((BASE/'manifest.json').read_text())
 helper=BASE/'SoulFlowerStaminaSweep.dll';require(sha(helper.read_bytes())==inherited['helperSHA256'],'Inherited helper changed')
 shutil.copy2(helper,out/helper.name)
 report=dict(schema='hyjs-world-map-packaged-field-lock-v1',status='STATIC_VERIFIED_NOT_PACKAGED',inputAPK_SHA256=APK_SHA,inputManagedSHA256=INPUT_SHA,outputManagedSHA256=sha(after),replacementEntries={'assets/bin/Data/Managed/Assembly-CSharp.dll':sha(after),'assets/bin/Data/Managed/SoulFlowerStaminaSweep.dll':sha(helper.read_bytes())},inheritedStaminaManifest=str((BASE/'manifest.json').relative_to(ROOT)),addedHelper=False,deviceExecuted=False,visualVerified=False,resourceFilesModified=False,targetPins=PINS,sourcePins={str(x.relative_to(ROOT)):sha(x.read_bytes()) for x in (SOURCE/'Injector.cs',Path(__file__).resolve())},knownLimits=['Original NGUI lock visibility and text clipping require Root device check.'],**checks)
 (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
 print(json.dumps(report,ensure_ascii=False))
if __name__=='__main__':main()
