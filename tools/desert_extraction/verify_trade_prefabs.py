"""Inspect the exact v80 NGUI trade prefabs without executing the game."""
import argparse, hashlib, json, re, struct, zipfile
from pathlib import Path
import UnityPy

BASE_SHA='25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
ASSETS={
 'message':('ui/messagebox/messageboxwithconfirmandcancelbutton',500,9,'assets/bin/Data/bef7ee3cde759b94d80f71ebfbee70bb'),
 'amount':('pages/activityitembatchbuypage',170,23,'assets/bin/Data/e3be5c84acc4fa449bcf273184ef2f36'),
}

def require(condition,message):
 if not condition: raise ValueError(message)
def file_sha(path):
 h=hashlib.sha256()
 with path.open('rb') as f:
  for part in iter(lambda:f.read(1024*1024),b''): h.update(part)
 return h.hexdigest()
def sha(data): return hashlib.sha256(data).hexdigest()
def asset(raw): return next(iter(UnityPy.load(raw).files.values()))
def components(f,global_assets,go):
 result=[]
 for _,ptr in go.m_Component:
  o=f.objects[ptr.m_PathID]; name=o.type.name
  if name=='MonoBehaviour':
   raw=o.get_raw_data(); fid,pid=struct.unpack_from('<iq',raw,16)
   require(fid==1 and pid in global_assets.objects,'Unknown prefab script')
   name=global_assets.objects[pid].read().m_ClassName
  result.append((name,o.path_id))
 return result
def hierarchy(f,root):
 result={}
 def visit(go,path):
  require(path not in result,'Ambiguous prefab path '+path); result[path]=go
  transforms=[p for kind,p in go.m_Component if kind in (4,224)]
  require(len(transforms)==1,'Unexpected transform count '+path)
  for childptr in transforms[0].read().m_Children:
   child=childptr.read().m_GameObject.read()
   visit(child,path+'/'+child.m_Name if path else child.m_Name)
 visit(f.objects[root].read(),''); return result

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--base-apk',type=Path,required=True);ap.add_argument('--output',type=Path,required=True);a=ap.parse_args()
 require(file_sha(a.base_apk)==BASE_SHA,'APK is not the fixed v80 base')
 with zipfile.ZipFile(a.base_apk) as z:
  gmraw=z.read('assets/bin/Data/globalgamemanagers'); gm=UnityPy.load(gmraw); files=next(iter(gm.files.values()))
  mappings=dict(next(o for o in gm.objects if o.type.name=='ResourceManager').read_typetree()['m_Container'])
  global_assets=asset(z.read('assets/bin/Data/globalgamemanagers.assets')); scenes={}; result={}
  for key,(resource,fid,root,name) in ASSETS.items():
   require(mappings[resource]=={'m_FileID':fid,'m_PathID':root},'ResourceManager mapping changed: '+key)
   require(files.externals[fid-1].path.replace('archive:/','').split('/')[-1]==name.split('/')[-1],'ResourceManager external changed: '+key)
   raw=z.read(name); f=asset(raw); paths=hierarchy(f,root); scenes[key]=(f,paths)
   result[key]={'resourceKey':resource,'resourceFile':name,'sha256':sha(raw),'rootPathId':root,'rootName':paths[''].m_Name}
  checks=[]
  for scene,button,caption in [('message','Window/Main/ConfirmButton','Label'),('message','Window/Main/CancelButton','Label'),('amount','bg/AmontControl/okButton','leftAction')]:
   f,paths=scenes[scene]; artpath=button+'/'+caption
   require(button in paths and artpath in paths,'Button or caption missing: '+button)
   buttonclasses=[x[0] for x in components(f,global_assets,paths[button])]; artclasses=[x[0] for x in components(f,global_assets,paths[artpath])]
   labels=[p for p,go in paths.items() if (p==button or p.startswith(button+'/')) and 'UILabel' in [x[0] for x in components(f,global_assets,go)]]
   require('BoxCollider' in buttonclasses and 'UIButton' in buttonclasses and 'UISprite' in artclasses and not labels,'Original button structure changed: '+button)
   spriteid=next(x[1] for x in components(f,global_assets,paths[artpath]) if x[0]=='UISprite')
   strings=[x.decode('ascii') for x in re.findall(rb'[\x20-\x7e]{3,}',f.objects[spriteid].get_raw_data())]
   checks.append({'scene':scene,'button':button,'buttonComponents':buttonclasses,'captionPath':artpath,'captionComponents':artclasses,'captionAsciiStrings':strings,'descendantUILabels':labels})
  f,paths=scenes['amount']; root=paths['']; scripts=components(f,global_assets,root)
  script=next(pid for name,pid in scripts if name=='ActivityItemBatchBuyPage'); raw=f.objects[script].get_raw_data()
  expected=[('iconRoot','GameObject'),('labName','UILabel'),('labHaveNum','UILabel'),('labLeftBuyCount','UILabel'),('labDesc','UILabel'),('labTotalPrice','UILabel'),('labCurCount','UILabel'),('priceIcon','UISprite')]
  require(len(raw)>=len(expected)*12,'Quantity field bytes missing'); fields=[]
  for i,(field,typ) in enumerate(expected):
   fid,pid=struct.unpack_from('<iq',raw,len(raw)-len(expected)*12+i*12); require(fid==0 and pid in f.objects,'Missing native field '+field)
   target=f.objects[pid]
   if typ=='GameObject': require(target.type.name==typ,'Wrong native field type '+field);go=target.read()
   else:
    require(target.type.name=='MonoBehaviour','Wrong native field type '+field); owner=struct.unpack_from('<q',target.get_raw_data(),4)[0];go=f.objects[owner].read()
    require((typ,pid) in components(f,global_assets,go),'Wrong native field script '+field)
   fields.append({'field':field,'pathId':pid,'componentType':typ,'gameObject':go.m_Name})
  for scene in ('message','amount'):
   f,paths=scenes[scene];require('UIPanel' in [x[0] for x in components(f,global_assets,paths[''])],'Missing native UIPanel')
  proof={'schema':'hyjs-desert-v4-trade-native-components-v1','baseAPKSHA256':BASE_SHA,'globalGameManagersSHA256':sha(gmraw),'assets':result,'buttons':checks,'quantityFields':fields,'metadataBasis':'Original managed declaration field order and Unity 5.3 serialized PPtr layout; component references checked against prefab objects and original MonoScripts.','unityRuntimeVerified':False,'deviceVerified':False}
  require(not a.output.exists(),'Refusing to overwrite evidence');a.output.write_text(json.dumps(proof,ensure_ascii=False,indent=2)+'\n')
  print(json.dumps({'output':str(a.output),'sha256':file_sha(a.output),'buttons':len(checks),'quantityFields':len(fields),'runtimeVerified':False}))
if __name__=='__main__': main()
