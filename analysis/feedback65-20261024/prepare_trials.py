"""Earn six owned QA floors through real Start/End; never import progress or a winner."""
import json,time
import protocol as q

account=json.loads((q.PRIVATE/'account.json').read_text());assert account['playerId']==1
session=q.sql('SELECT HEX(session) FROM profiles WHERE id=1;').strip()
login=q.CLASSES['SCUserLogin'](playerId=1,sessionKey=bytes.fromhex(session))
rows=[];stamp=int(time.time()*1000)
def call(name,fields,response):
 global stamp
 stamp+=1;out,ms=q.expected(name,fields,login,response,stamp);rows.append({'request':name,'ms':ms});return out
info=call('CSGetRushInfo',{},'SCGetRushInfo');assert info.curFloor==1
for floor in range(1,7):
 tasks=call('CSRushCurrentInfo',{},'SCRushCurrentInfo').taskInfoList
 task=next(x for x in tasks if x.star==1)
 start=call('CSRushFightStart',{'rushType':1,'taskId':task.taskId,'star':1},'SCRushFightStart')
 assert start.fightId.startswith('p7-')
 raw=q.sql("SELECT HEX(preview) FROM p7_rush_manual_calculations WHERE fight_id='"+start.fightId+"';").strip()
 preview=json.loads(bytes.fromhex(raw));assert preview['outcome']['winner']==0,'Finite actual roster lost; retain outcome'
 result=call('CSRushFightEnd',{'fightId':start.fightId,'result':1,'optRecord':preview['originalOperates']},'SCRushFightEnd')
 assert result.result==1 and result.star==1
 if floor%3==0:
  call('CSRushGetSectionReward',{},'SCRushGetSectionReward')
  options=call('CSRushGetSectionAddition',{},'SCRushGetSectionAddition')
  call('CSRushAddition',{'select':options.starResult3,'star':3},'SCRushAddition')
 info=call('CSGetRushInfo',{},'SCGetRushInfo');assert info.curFloor==floor+1
(q.OUT/'trial-protocol-preparation.json').write_text(json.dumps({'database':q.database,'playerId':1,'source':'Real frozen automatic preview record submitted to real manual End validator; no saved progress imported','earnedFloors':6,'floor':info.curFloor,'requests':rows},indent=2)+'\n')
print(json.dumps({'earnedFloors':6,'currentFloor':info.curFloor,'requests':rows}))
