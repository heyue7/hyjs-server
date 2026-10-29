"""Validate original desert interaction UI components in the pinned v80 APK."""
import argparse, json, struct, zipfile
from pathlib import Path
import UnityPy
from verify_trade_prefabs import BASE_SHA,asset,components,file_sha,hierarchy,require,sha

RESOURCES={
 'dialog':('ui/messagebox/messageboxwithconfirmandcancelbutton',500,9,'bef7ee3cde759b94d80f71ebfbee70bb',44),
 'countdown':('ui/messagebox/messageboxcountdown',499,9,'84383e5a2c8358d47aad491db79c4698',53),
 'toast':('ui/messagebox/popuptweentextbox',508,1,'49a18d749d3483a47868b972403b3534',15),
 'field':('pages/uifieldrunmap',331,250,'52822e775bb01134e99cd41a3dd47bf8',1126),
}
def target(f,g,pid,typ):
 require(pid in f.objects,'Missing referenced object')
 o=f.objects[pid]
 if typ=='GameObject':
  require(o.type.name==typ,'Expected GameObject');return o.read()
 require(o.type.name=='MonoBehaviour','Expected MonoBehaviour')
 owner=struct.unpack_from('<q',o.get_raw_data(),4)[0]; go=f.objects[owner].read()
 require((typ,pid) in components(f,g,go),'Expected '+typ)
 return go

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--base-apk',type=Path,required=True);ap.add_argument('--output',type=Path,required=True);a=ap.parse_args()
 require(file_sha(a.base_apk)==BASE_SHA,'Wrong base APK')
 with zipfile.ZipFile(a.base_apk) as z:
  gmraw=z.read('assets/bin/Data/globalgamemanagers');gm=UnityPy.load(gmraw);gmfile=next(iter(gm.files.values()))
  mapping=dict(next(o for o in gm.objects if o.type.name=='ResourceManager').read_typetree()['m_Container'])
  ga=asset(z.read('assets/bin/Data/globalgamemanagers.assets'));loaded={};assets={}
  for key,(resource,fid,root,name,controller) in RESOURCES.items():
   require(mapping[resource]=={'m_FileID':fid,'m_PathID':root},'Resource mapping changed '+key)
   require(gmfile.externals[fid-1].path.split('/')[-1]==name,'Resource external changed '+key)
   raw=z.read('assets/bin/Data/'+name);f=asset(raw);paths={'':f.objects[root].read()} if key=='field' else hierarchy(f,root);loaded[key]=(f,paths,controller)
   expected={'dialog':'MessageBoxWidth2BtnController','countdown':'MessageBoxCountdown','toast':'PopupTweenTextBox','field':'UIFieldRunMap'}[key]
   require((expected,controller) in components(f,ga,paths['']),'Missing native controller '+key)
   assets[key]={'resource':resource,'resourceFile':'assets/bin/Data/'+name,'sha256':sha(raw),'rootPathId':root,'controller':expected,'controllerPathId':controller}
  dialogs={}
  for key in ('dialog','countdown'):
   f,paths,controller=loaded[key];raw=f.objects[controller].get_raw_data();values={}
   for i,(field,typ) in enumerate([('mask','GameObject'),('label','UILabel'),('ok','GameObject'),('cancel','GameObject'),('title','UILabel')]):
    fid,pid=struct.unpack_from('<iq',raw,32+i*12);require(fid==0,'Unexpected external field');go=target(f,ga,pid,typ);values[field]={'pathId':pid,'type':typ,'name':go.m_Name}
   for label,method in [('ConfirmButton','OnConfirmClick'),('CancelButton','OnCancelClick')]:
    path='Window/Main/'+label;require(path in paths,'Missing button '+path);go=paths[path];cs=components(f,ga,go);names=[x[0] for x in cs]
    require('BoxCollider' in names and 'UIButton' in names,'Missing interactive button component')
    pid=next(pid for typ,pid in cs if typ=='UIButton');button=f.objects[pid].get_raw_data()
    require(method.encode('ascii') in button and struct.pack('<iq',0,controller) in button,'Missing native event binding '+path)
    art=paths[path+'/Label']; require('UISprite' in [x[0] for x in components(f,ga,art)],'Missing native button sprite')
    values[label]={'components':names,'listener':method,'targetControllerPathId':controller,'captionType':'UISprite'}
   dialogs[key]=values
  f,paths,controller=loaded['toast'];fid,pid=struct.unpack_from('<iq',f.objects[controller].get_raw_data(),32);require(fid==0,'Toast label external')
  go=target(f,ga,pid,'GameObject');require('UILabel' in [x[0] for x in components(f,ga,go)],'Toast UILabel missing')
  toast={'labelGameObjectPathId':pid,'labelName':go.m_Name,'component':'UILabel'}
  f,paths,controller=loaded['field'];states=[]
  for o in f.objects.values():
   if o.type.name!='MonoBehaviour': continue
   fid,pid=struct.unpack_from('<iq',o.get_raw_data(),16)
   if fid==1 and pid in ga.objects and ga.objects[pid].read().m_ClassName=='UIFieldReadState': states.append(o)
  require(len(states)==1,'Expected exactly one native read state');state=states[0]; raw=state.get_raw_data()
  pattern=struct.pack('<iq',0,state.path_id);runraw=f.objects[controller].get_raw_data();require(runraw.count(pattern)==1,'Native readState field missing')
  field={'readStatePathId':state.path_id,'runMapFieldOffset':runraw.find(pattern),'references':{}}
  for name,offset,typ in [('slider',44,'UISlider'),('cost',64,'UILabel'),('fish',80,'GameObject'),('mine',92,'GameObject')]:
   fid,pid=struct.unpack_from('<iq',raw,offset);require(fid==0,'ReadState external field');go=target(f,ga,pid,typ);field['references'][name]={'pathId':pid,'type':typ,'name':go.m_Name}
  raw=z.read('assets/android/tabledata/tablefieldresource');table=UnityPy.load(raw);text=next(o.read().m_Script for o in table.objects if o.type.name=='TextAsset')
  rows={line.split('\t')[0]:line.split('\t') for line in text.splitlines()};groups=[]
  for kind in ('fish','mine'):
   for level in range(3,7):
    key='group_'+kind+'_'+str(level).zfill(3);row=rows[key];require(int(row[1])==level and row[2] and row[6].startswith('tiredness,'),'Native gather resource mismatch')
    groups.append({'id':key,'level':level,'model':row[2],'cost':row[6]})
  proof={'schema':'hyjs-desert-v4-native-interaction-ui-v1','baseAPKSHA256':BASE_SHA,'globalGameManagersSHA256':sha(gmraw),'assets':assets,'dialogs':dialogs,'toast':toast,'fieldReadState':field,'gatherGroups':groups,'tableBundleSHA256':sha(raw),'nativeLogicEvidence':{'dialogInit':'v80 MessageBoxWidth2BtnController.Init uses m_kLabel and native button callback; original event target and component types checked above.','fieldReadStatePage':'UIFieldRunMap.Init assigns readState.page at runtime before UIFieldReadState.Hide uses it.','countdown':'OneButton forwards a shared Action for click and timeout, and sets its fourth argument as title. TwoButton uses separate bool click callback; timeout destroys the dialog without invoking it while cancel is active.'},'deviceInteractionsVerified':False}
  require(not a.output.exists(),'Refusing to overwrite proof');a.output.write_text(json.dumps(proof,ensure_ascii=False,indent=2)+'\n')
  print(json.dumps({'output':str(a.output),'sha256':file_sha(a.output),'dialogButtonBindings':4,'gatherGroups':len(groups),'deviceInteractionsVerified':False}))
if __name__=='__main__':main()
