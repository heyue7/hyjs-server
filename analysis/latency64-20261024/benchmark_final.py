import sys,json,time,statistics,hashlib,os,shutil,urllib.request
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(Path(__file__).parent));import protocol as q
OUT=Path(__file__).parent;PRIVATE=q.PRIVATE
requests=[('CSPlayer',{}),('CSWarriorList',{}),('CSEquipmentTreasureList',{}),('CSFormationList',{}),('CSQuestList',{}),('CSTaskList',{}),('CSMailList',{'mailType':'allMail'}),('CSShopProductList',{'shopType':1}),('CSRewardMonthSignList',{}),('CSActivityList',{}),('CSIsCardClaimed',{})]
reports=[]
for label,file in [('baseline','hyjs-latency64-before'),('final','hyjs-latency64-final')]:
 old=json.loads((PRIVATE/'processes.json').read_text())['appPid'];source=ROOT/'build'/file;target=ROOT/'build/hyjs-latency64-current';tmp=target.with_suffix('.next64');shutil.copy2(source,tmp);os.replace(tmp,target);(PRIVATE/'restart').touch()
 for _ in range(200):
  time.sleep(.2)
  try:
   state=json.loads((PRIVATE/'processes.json').read_text())
   if state['appPid']==old:continue
   with urllib.request.urlopen('http://127.0.0.1:28979/health',timeout=1) as x:health=json.load(x)
   if health['status']=='ok':break
  except Exception:pass
 else:raise RuntimeError('QA restart did not become ready')
 runtime=json.loads((PRIVATE/'runtime.json').read_text());runtime.update(pid=state['appPid'],binarySHA256=hashlib.file_digest(source.open('rb'),'sha256').hexdigest());(PRIVATE/'runtime.json').write_text(json.dumps(runtime))
 session=q.sql('SELECT HEX(session) FROM profiles WHERE id=1;').strip();login=q.CLASSES['SCUserLogin'](playerId=1,sessionKey=bytes.fromhex(session));rows=[]
 for name,fields in requests:
  warm,ms=q.call(name,fields,login);assert warm.DESCRIPTOR.name!='ErrorMessage',name
  samples=[]
  for _ in range(9):
   response,ms=q.call(name,fields,login);assert response.DESCRIPTOR.name==warm.DESCRIPTOR.name;samples.append(ms);time.sleep(.04)
  row=dict(request=name,fields=fields,response=warm.DESCRIPTOR.name,medianMs=round(statistics.median(samples),3),samplesMs=samples);rows.append(row);print(label,name,row['medianMs'],flush=True)
 report=dict(label=label,runtime=runtime,playerId=1,level=100,originalBinaryHTTP=True,quietNoPhoneOrBrowser=True,warmupPerRequest=1,repeat=9,items=rows);reports.append(report);(OUT/('quiet-'+label+'64.json')).write_text(json.dumps(report,indent=2)+'\n')
comparison=[dict(request=a['request'],beforeMs=a['medianMs'],afterMs=b['medianMs'],changeMs=round(b['medianMs']-a['medianMs'],3)) for a,b in zip(reports[0]['items'],reports[1]['items'])];(OUT/'quiet-comparison64.json').write_text(json.dumps(comparison,indent=2)+'\n');print(json.dumps(comparison),flush=True)
