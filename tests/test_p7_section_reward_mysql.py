"""Original54519/54523 three real accepted Trial floors; single managed SQL lease.

Finite Player22 only, actual Create047/main1/GUID unchanged. Explicit defaultoff
local first-three catalogue uses the complete original-observed NPC047 source;
private floor/difficulty/reward/aggregation unknown. Every new random battle is
independently calculated, and only its actual complete winning End counts.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import tempfile
import time
import unittest
from hyjs_managed_mysql import ManagedMySQL
from test_p7_maze_trader_mysql import P7MazeTraderMySQLTests as Transport
from server.codec import message,BY_NAME
ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P7_SECTION_REWARD_MYSQL_TEST')=='1','Root single managed SQL lease')
class P7SectionRewardMySQLTests(unittest.TestCase):
 send=Transport.send
 login=Transport.login
 stamped=Transport.stamped
 call=Transport.call
 profile=Transport.profile
 start=Transport.start
 stop=Transport.stop
 setUp=Transport.setUp
 sql=Transport.sql
 def test_original_three_real_floors_preview_auto_flag_claim_use_cold(self):
  self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
  proof=json.loads((ROOT/'analysis/p7/section-reward-prebuilt-proof.json').read_text())
  self.assertEqual(proof['status'],'PASS_DOMAIN_ALLPKG_VET_BUILD')
  self.source=Path(proof['source']).resolve();self.assertEqual(self.source.parent,ROOT/'build/candidates')
  for kind in ['runtimeSHA256','tableSHA256']:
   for rel,sha in proof[kind].items():self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(),sha,rel)
  binary=Path(proof['binary']);self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),proof['binarySHA256'])
  helper=hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
  self.assertEqual(helper,'483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
  # Fail before resource allocation if an original label/type is misread.
  for name in ['CSRushRewardPreview','CSRushGetSectionReward']:self.assertEqual(list(message(name).DESCRIPTOR.fields),[])
  fields=message('SCRushRewardPreview').DESCRIPTOR.fields_by_name
  self.assertEqual(set(fields),{'sectionStar','sectionFrom','sectionTo','threeStarReward','sixStarReward','nineStarReward'})
  for field in ['threeStarReward','sixStarReward','nineStarReward']:self.assertTrue(fields[field].is_repeated)
  self.assertEqual(set(message('SCRushGetSectionReward').DESCRIPTOR.fields_by_name),{'gain'})
  self.assertIn('optRecord',message('CSRushFightEnd').DESCRIPTOR.fields_by_name)
  began=time.monotonic();report=dict(status='PENDING',runtimeBinarySHA256=proof['binarySHA256'],managedHelperSHA256=helper,finitePreparation={'PlayerLevel':22,'main':'actual Create047/main1/GUID/all3skills unchanged','wallet':'Create values unchanged'},originalPrivateFloorKnown=False,localCatalogue='original-observed completeNPC047, floors1..3, star choices1/2/3, point10/floor; highest-tier only',formalModified=False,phoneUI=False)
  checkpoint=ROOT/'analysis/p7/section-reward-native-checkpoint.json'
  with tempfile.TemporaryDirectory(prefix='hyjs-section-native-')as temporary:
   self.binary=Path(temporary)/'section-native';shutil.copy2(binary,self.binary)
   self.native_log=Path(temporary)/'native.log';self.managed_pool=ManagedMySQL()
   with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
    fcntl.flock(lock,fcntl.LOCK_EX)
    try:
     allocation=self.managed_pool.allocate('p7_section');self.database,self.test_user=allocation.database,allocation.user;self.test_password=allocation.password
     self.env=dict(os.environ,**allocation.env(),GOMEMLIMIT='768MiB',GOMAXPROCS='2')
     for filename in ['178_p7_section_reward_policies.sql','178_p7_section_reward_policies.seed.sql','178_p7_section_reward_offers.sql']:self.sql((self.source/'hyjs-server/db/sql'/filename).read_text())
     self.start()
     self.assertEqual(self.sql('SELECT enabled FROM p7_section_reward_policies;').strip(),'0')
     self.assertEqual(self.sql("SELECT COUNT(*) FROM p7_rush_stage_rules WHERE id LIKE 'local-section:%' AND enabled=0;").strip(),'9')
     self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME IN ('p7_section_reward_policies','p7_section_reward_offers','p7_rush_stage_rules','p7_rush_receipts','p6_battle_records');").strip(),'InnoDB')
     identity='original-section-'+secrets.token_hex(5);owner=self.login(identity);stamp=278000
     def http(name,fields,expected):
      nonlocal stamp
      stamp+=1;return self.call(name,fields,owner,stamp,expected)
     def count(item):return sum(v['num']for v in(self.profile(owner)[1].get('items')or[])if v['id']==item)
     http('CSPlayerCreate',{'playerName':'原试炼星奖','heroId':'hero_047'},'SCPlayerCreate')
     initial=self.profile(owner);self.assertEqual((initial[0].get('coin',0),initial[0].get('diamond',0),initial[0].get('energy',0)),(0,0,100))
     self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',22) WHERE id={owner.playerId};")
     http('CSFormationList',{},'SCFormationList')
     http('CSRushRewardPreview',{},'ErrorMessage')
     self.sql("UPDATE p7_rush_stage_rules SET enabled=1 WHERE id LIKE 'local-section:%';UPDATE p7_rush_policies SET enabled=1;UPDATE p7_rush_scoped_policies SET enabled=1;UPDATE p7_rush_manual_policies SET enabled=0;")
     at=int(time.time()*1000);self.sql(f'UPDATE p7_section_reward_policies SET enabled=1,published_at={at-1000},start_at={at-1000},end_at={at+3600000};')
     view=self.call('CSRushRewardPreview',{},owner,0,'SCRushRewardPreview');self.assertEqual((view.sectionFrom,view.sectionTo,view.sectionStar),(1,3,0))
     records=[]
     for floor in [1,2,3]:
      info=http('CSGetRushInfo',{},'SCGetRushInfo');self.assertEqual(info.curFloor,floor);self.assertEqual(info.isRewardNeedGet,0)
      tasks=http('CSRushCurrentInfo',{},'SCRushCurrentInfo').taskInfoList
      self.assertEqual(sorted(t.star for t in tasks),[1,2,3]);self.assertTrue(all(t.point==10 and t.coin==0 for t in tasks))
      task=next(t for t in tasks if t.star==3)
      start=http('CSRushFightStart',{'rushType':1,'taskId':task.taskId,'star':task.star},'SCRushFightStart')
      self.assertEqual(start.battlePlayer.formation[0].guid,initial[0]['heroWarId']);self.assertEqual(start.battlePlayer.formation[0].skillOrder,'047_1,047_2,047_3')
      self.assertIsNotNone(start.battleMonsterList[0].inst)
      terminal=json.loads(bytes.fromhex(self.sql("SELECT HEX(terminal) FROM p7_rush_scoped_calculations WHERE fight_id='"+start.fightId+"';").strip()))
      outcome=terminal['outcome'];actual=int(outcome['winner']==0)
      records.append(dict(floor=floor,fightId=start.fightId,seed=start.randSeed,actualWinner=outcome['winner'],finalRound=outcome['finalRound'],proof=outcome['capabilityHash'],actions=len(terminal['originalOperates']['skillOpts'])))
      self.assertEqual(actual,1,'Actual source lost; preserve actual result, no winner rewrite')
      end={'fightId':start.fightId,'result':actual,'optRecord':terminal['originalOperates']}
      if floor==1:
       held=self.profile(owner);http('CSRushFightEnd',dict(end,result=0),'ErrorMessage');self.assertEqual(self.profile(owner),held)
      result=http('CSRushFightEnd',end,'SCRushFightEnd');self.assertEqual((result.result,result.star),(1,3))
      self.assertEqual(http('CSRushRewardPreview',{},'SCRushRewardPreview').sectionStar,floor*3)
      if floor<3:http('CSRushGetSectionReward',{},'ErrorMessage')
     info=http('CSGetRushInfo',{},'SCGetRushInfo');self.assertEqual((info.curFloor,info.curTotalStar,info.isRewardNeedGet),(4,9,1))
     http('CSRushCurrentInfo',{},'ErrorMessage')
     http('CSRushFightStart',{'rushType':1,'taskId':'local-section-4','star':3},'ErrorMessage')
     for invalid in [0,-1]:self.assertEqual(self.stamped('CSRushGetSectionReward',{},owner,invalid)[0],BY_NAME['ErrorMessage'])
     frozen=self.sql('SELECT CONCAT(policy_hash,"|",first_start_hash,"|",SHA2(stage_rules,256)) FROM p7_section_reward_offers;')
     self.sql("UPDATE p7_section_reward_policies SET enabled=0,nine_reward='coin,99999';")
     stamp+=1;claim_stamp=stamp
     with concurrent.futures.ThreadPoolExecutor(max_workers=8)as workers:results=list(workers.map(lambda _:self.call('CSRushGetSectionReward',{},owner,claim_stamp,'SCRushGetSectionReward'),range(8)))
     awarded=results[0];self.assertTrue(all(v==awarded for v in results));self.assertEqual([(v.id,v.num)for v in awarded.gain.simps],[('item_001',3)])
     self.assertEqual(count('item_001'),3);self.assertEqual(self.profile(owner)[0]['point'],30)
     http('CSRushGetSectionReward',{},'ErrorMessage')
     energy=self.profile(owner)[0]['energy'];http('CSItemUse',{'costItem':{'id':'item_001','num':3}},'SCItemUse')
     self.assertEqual((count('item_001'),self.profile(owner)[0]['energy']),(0,energy+30))
     held=self.profile(owner);self.stop();self.start();owner=self.login(identity)
     self.assertEqual(self.call('CSRushGetSectionReward',{},owner,claim_stamp,'SCRushGetSectionReward'),awarded);self.assertEqual(self.profile(owner),held)
     info=http('CSGetRushInfo',{},'SCGetRushInfo');self.assertEqual((info.curFloor,info.curTotalStar,info.isRewardNeedGet),(4,9,0))
     self.assertEqual(self.sql('SELECT CONCAT(policy_hash,"|",first_start_hash,"|",SHA2(stage_rules,256)) FROM p7_section_reward_offers;'),frozen)
     self.assertEqual(self.sql('SELECT COUNT(*) FROM p7_rush_receipts WHERE domain="section-reward";').strip(),'1')
     report.update(status='PASS',acceptedFloors=records,actualStars=9,actualPoint30=True,highestTierOnly={'id':'item_001','num':3},actualUseEnergy30=True,originalEmptyClaimOnce8=True,coldOnce=True,closedPolicyFrozen=True,earnedOnlyFourthInfo=True,unpublishedFourthStartDenied=True)
    except BaseException as error:
     report.update(status='FAIL',error=self.managed_pool.redact(repr(error)));raise
    finally:
     self.stop();self.managed_pool.close();report.update(wallSeconds=round(time.monotonic()-began,3),resources={'process':0,'managedSchema':0,'managedAccount':0,'privateEnv':0});checkpoint.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
