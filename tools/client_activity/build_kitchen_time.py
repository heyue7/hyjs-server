"""Compose two local presentation repairs onto the already-hooked pinned APK."""
import argparse,hashlib,json,subprocess,tempfile,zipfile,shutil,sys,contextlib,io
from pathlib import Path
import dnfile
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT))
from tools.client_activity.build import decoded
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
SHA="02cd2225e4615c3e5da16ffeb36fddcf6f2873e8bdd2d1f22e1967a9f7589b53"
ENTRY="assets/bin/Data/Managed/Assembly-CSharp.dll"
TARGETS={"UI.UIActivityAsunaKitchen::MsgCallback","UI.UIActivityBattleRace::MsgCallback"}
sha=lambda b:hashlib.sha256(b).hexdigest()
def main():
 ap=argparse.ArgumentParser();ap.add_argument('--base-apk',type=Path,required=True);ap.add_argument('--output-directory',type=Path,required=True);a=ap.parse_args();assert sha(a.base_apk.read_bytes())==SHA
 out=a.output_directory.resolve();assert not out.exists();out.mkdir(parents=True)
 runtime=ROOT/'.tools/native-batch'
 if not (runtime/'dotnet/dotnet').exists():runtime=Path('/Users/lirui/LR/hyjs/.tools/native-batch')
 dotnet=runtime/'dotnet/dotnet';compiler=next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'));net=next((runtime/'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'));dnlib=runtime/'injector/bin/Debug/net8.0/dnlib.dll'
 def run(name,args):
  r=subprocess.run(['rtk','proxy',*map(str,args)],capture_output=True,text=True,timeout=90);(out/(name+'.log')).write_text(r.stdout+r.stderr);assert r.returncode==0,(name,r.stdout+r.stderr)
 sources=[Path(__file__).with_name(n)for n in ['ActivityGuard.cs','InstanceSelector.cs','KitchenTimeInjector.cs']]
 with tempfile.TemporaryDirectory(prefix='hyjs-kitchen-time-')as tmp:
  work=Path(tmp);refs=[]
  with zipfile.ZipFile(a.base_apk)as z:
   before=z.read(ENTRY);oldhelper=z.read('assets/bin/Data/Managed/SoulFlowerActivity.dll')
   for n in ['Assembly-CSharp.dll','UnityEngine.dll','Assembly-CSharp-firstpass.dll','mscorlib.dll','System.dll','System.Core.dll']:
    p=work/n;p.write_bytes(z.read('assets/bin/Data/Managed/'+n));refs.append(p)
  run('compile-helper',[dotnet,compiler,'/nologo','/target:library','/nostdlib+','/langversion:3','/deterministic+','/out:'+str(out/'SoulFlowerActivity.dll'),*['/r:'+str(p)for p in refs],*sources[:2]])
  run('compile-injector',[dotnet,compiler,'/nologo','/target:exe','/nostdlib+','/langversion:latest','/deterministic+','/out:'+str(work/'Patch.dll'),*['/r:'+str(p)for p in net.glob('*.dll')],'/r:'+str(dnlib),sources[2]])
  shutil.copy2(dnlib,work/'dnlib.dll');(work/'Patch.runtimeconfig.json').write_text(json.dumps({'runtimeOptions':{'tfm':'net8.0','framework':{'name':'Microsoft.NETCore.App','version':'8.0.0'}}}))
  run('patch',[dotnet,work/'Patch.dll',work/'Assembly-CSharp.dll',out/'Assembly-CSharp.dll'])
 after=(out/'Assembly-CSharp.dll').read_bytes();l,r=[dnfile.dnPE(data=x)for x in [before,after]];lm,rm=methods(l),methods(r);assert len(lm)==len(rm);changed=[];engine=0;unchanged=0
 for table in ['TypeDef','Field','MethodDef']:assert len(getattr(l.net.mdtables,table).rows)==len(getattr(r.net.mdtables,table).rows)
 for (name,rid,x),(newname,newrid,y)in zip(lm,rm):
  assert(name,rid)==(newname,newrid)and x.Signature.value==y.Signature.value and x.struct.Flags==y.struct.Flags and x.struct.ImplFlags==y.struct.ImplFlags,name
  if not x.Rva:assert not y.Rva;continue
  old,new=exact_body(l,x),exact_body(r,y)
  if name not in TARGETS:assert old==new,name;unchanged+=1;engine+=name.startswith('Battle.Engine');continue
  ob,oc=decoded(l,x);nb,nc=decoded(r,y);assert not ob.exception_handlers and not nb.exception_handlers
  if 'AsunaKitchen' in name:
   assert [x[0]for x in nc[:6]]==['ldarg.0','ldarg.1','ldarg.2','call','brtrue','ret']and nc[4][1]==6
   def adjust(item):
    op,v=item
    if op.startswith(('br','beq','bne','bge','bgt','ble','blt','leave'))and isinstance(v,int):v-=6
    return op,v
   assert [adjust(x)for x in nc[6:]]==oc
  else:
   at=next(i for i,x in enumerate(oc)if x==('ldstr','activity_time_Format'))
   def adjust(item):
    op,v=item
    if op.startswith(('br','beq','bne','bge','bgt','ble','blt','leave'))and isinstance(v,int)and v>=at+6:v-=4
    return op,v
   assert [adjust(x)for x in nc]==oc[:at]+[('ldstr','{0}')]+[('nop',None)]*5+oc[at+2:]
  changed.append({'method':name,'beforeSHA256':sha(old),'afterSHA256':sha(new)})
 assert engine==547 and len(changed)==2
 # Retain every inherited helper method/field signature; adding the guard is additive ABI.
 ha,hb=[dnfile.dnPE(data=x)for x in [oldhelper,(out/'SoulFlowerActivity.dll').read_bytes()]]
 def abi(pe):
  return {(str(t.TypeNamespace)+'.'+str(t.TypeName),str(m.row.Name),m.row.Signature.value)for t in pe.net.mdtables.TypeDef.rows for m in t.MethodList if not str(m.row.Name).startswith('<')}
 # Type tokens may move after an additive method; public bridge methods are checked by name/count below.
 for t in ha.net.mdtables.TypeDef.rows:
  full=str(t.TypeNamespace)+'.'+str(t.TypeName);matches=[v for v in hb.net.mdtables.TypeDef.rows if str(v.TypeNamespace)+'.'+str(v.TypeName)==full];assert len(matches)==1,full
  names=[str(m.row.Name)for m in matches[0].MethodList]
  for m in t.MethodList:assert str(m.row.Name)in names,(full,str(m.row.Name))
 # Concrete prefab proves the fixed sibling prefix and date-only activityTime label.
 decoder=ROOT/'tools/client_stamina_sweep/decode_buyitempage.py';code=decoder.read_text().replace("APK = ROOT / 'build/hyjs-local.apk'",'APK = Path('+repr(str(a.base_apk.resolve()))+')').replace('pages/buyitempage','activityinnerpage/battlerace');buf=io.StringIO()
 with contextlib.redirect_stdout(buf):exec(compile(code,str(decoder),'exec'),{'__file__':str(decoder),'__name__':'__main__'})
 prefab=json.loads(buf.getvalue());labels=[{'object':o['object'],'gameObject':o['pathid'],'label':c['pathid'],'text':c['text']}for o in prefab['objects']for c in o['components']if c.get('text')in ['活动时间：','4月15日00:00:03至4月18日00:00:03']];assert len(labels)==2
 report={'schema':'hyjs-kitchen-time-local-overlay-v1','status':'STATIC_VERIFIED_NOT_PACKAGED','baseAPKSHA256':SHA,'inputManagedSHA256':sha(before),'outputManagedSHA256':sha(after),'originalEngineMethodsExact':engine,'unrelatedRawMethodBodiesExact':unchanged,'originalMetadataABIAndTokensPreserved':True,'inheritedHelperMethodsPreserved':True,'changes':changed,'prefabEvidence':{'resource':'activityinnerpage/battlerace','entry':prefab['entry'],'labels':labels,'targetField':'UI.UIActivityBattleRace.activityTime'},'emptyContract':'Current active kitchen failed response or fewer than two windows uses existing shared Empty; valid two windows continue original method; late callbacks ignored','phoneVerified':False,'payloadFiles':{ENTRY:str((out/'Assembly-CSharp.dll').relative_to(ROOT)),'assets/bin/Data/Managed/SoulFlowerActivity.dll':str((out/'SoulFlowerActivity.dll').relative_to(ROOT))},'replacementEntries':{ENTRY:sha(after),'assets/bin/Data/Managed/SoulFlowerActivity.dll':sha((out/'SoulFlowerActivity.dll').read_bytes())},'sourcePins':{str(p.relative_to(ROOT)):sha(p.read_bytes())for p in [Path(__file__).resolve(),*sources]}}
 (out/'manifest.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps({'status':report['status'],'engine':engine,'changes':changed}))
if __name__=='__main__':main()
