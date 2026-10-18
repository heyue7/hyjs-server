"""Original pet-recruit protocol and declared local pool on disposable InnoDB."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import time
import unittest

import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as transport


@unittest.skipUnless(os.environ.get('HYJS_P9_RECRUIT_MYSQL_TEST')=='1','Explicit isolated ordinary pet recruit')
class PetRecruitMySQLTests(unittest.TestCase):
    setUpClass=classmethod(shared.TalentScopeMySQLTests.setUpClass.__func__)
    root_sql=classmethod(shared.TalentScopeMySQLTests.root_sql.__func__)
    send=shared.TalentScopeMySQLTests.send
    login=shared.TalentScopeMySQLTests.login
    stamped=transport.GameCreationMySQLTests.stamped
    call=shared.TalentScopeMySQLTests.call
    profile=shared.TalentScopeMySQLTests.profile
    start=shared.TalentScopeMySQLTests.start
    stop=shared.TalentScopeMySQLTests.stop
    setUp=shared.TalentScopeMySQLTests.setUp
    sql=shared.TalentScopeMySQLTests.sql
    write_profile=shared.TalentScopeMySQLTests.write_profile

    def test_original_prices_free_five_frozen_pool_pity_and_cold_inventory(self):
        began=time.monotonic();self.start();login=self.login('ordinary-pet-pool')
        stamp=1800020000000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        call('CSPlayerCreate',{'playerName':'原精灵普通招募','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=34,noviceGuideSchedule=8)
        missions=shared.original_table('CopyMission')
        g['copy']['passedCopyInfo']=[{'chapterId':'27','missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions if x['ChapterId']=='27']
        self.write_profile(login,p,g)
        # Historical chapter stars are finite eligibility prerequisites; all spent
        # diamonds below come from actual original starbox awards, never insertion.
        for box in range(3):call('CSCopyGetStarAward',{'chapterId':'27','starIndex':box},'SCCopyGetStarAward')
        self.assertEqual(self.profile(login)[0]['diamond'],1230)
        before=self.profile(login);self.assertEqual(self.stamped('CSPetRecruit',{'free':1,'numType':1},login,stamp+1)[0],10001);self.assertEqual(self.profile(login),before)
        first=call('CSPetFirstRecruit',{},'SCPetFirstRecruit');self.assertEqual(first.get.pets[0].id,'pet_002')
        info=call('CSPetRecruitGet',{},'SCPetRecruitGet').recruit
        self.assertEqual((info.num,info.mustTimes,info.reward),(1,9,''));self.assertGreater(info.nextFree,int(time.time()*1000))
        pool=json.loads(self.sql("SELECT CONVERT(pool USING utf8mb4) FROM p9_pet_recruit_rules WHERE id='ordinary';").strip());self.assertEqual(len(pool),78)
        raw,checksum=self.sql(f"SELECT HEX(input),input_hash FROM p9_pet_recruit_progress WHERE player_id={login.playerId};").strip().split('\t')
        self.assertEqual(hashlib.sha256(bytes.fromhex(raw)).hexdigest(),checksum)
        request={'free':1,'numType':1};stamp+=1;free_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as executor:
            replies=list(executor.map(lambda _:self.call('CSPetRecruit',request,login,free_stamp,'SCPetRecruit'),range(8)))
        self.assertTrue(all(x==replies[0] for x in replies));free=replies[0]
        self.assertEqual((len(free.get.pets),len(free.getList),len(free.cost)),(1,1,0));self.assertEqual(free.get.simps[0].num,2000)
        self.assertEqual((free.getList[0].id,free.getList[0].guid),(free.get.pets[0].id,free.get.pets[0].guid))
        before=self.profile(login);self.assertEqual(self.stamped('CSPetRecruit',request,login,stamp+1)[0],10001);self.assertEqual(self.profile(login),before)
        self.sql("UPDATE p9_pet_recruit_rules SET pool='[]' WHERE id='ordinary';")
        request={'free':0,'numType':2};stamp+=1;paid_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as executor:
            replies=list(executor.map(lambda _:self.call('CSPetRecruit',request,login,paid_stamp,'SCPetRecruit'),range(8)))
        self.assertTrue(all(x==replies[0] for x in replies));paid=replies[0]
        self.assertEqual((len(paid.get.pets),len(paid.getList),paid.cost[0].num,paid.get.simps[0].num),(5,5,700,10000))
        self.assertEqual(self.profile(login)[0]['diamond'],530)
        listing=call('CSPetList',{},'SCPetList');self.assertEqual(len(listing.petList),7)
        saved=self.profile(login);self.stop();self.start();login=self.login('ordinary-pet-pool')
        self.assertEqual(self.call('CSPetRecruit',request,login,paid_stamp,'SCPetRecruit'),paid);self.assertEqual(self.profile(login),saved)
        # Finite near-pity prerequisite checks the exact forced quality branch.
        self.sql(f"UPDATE p9_pet_recruit_progress SET since_guarantee=9 WHERE player_id={login.playerId}; UPDATE p9_pet_states SET must_times=0 WHERE player_id={login.playerId};")
        forced=call('CSPetRecruit',{'free':0,'numType':1},'SCPetRecruit');self.assertEqual(forced.cost[0].num,150)
        original={x['id']:x for x in shared.original_table('Pet')}
        self.assertIn(original[forced.get.pets[0].id]['quality'],['20','23'])
        self.assertEqual(call('CSPetRecruitGet',{},'SCPetRecruitGet').recruit.mustTimes,9)
        guarded=call('CSFormationOn',{'formatPos':1,'id':free.get.pets[0].id,'guid':free.get.pets[0].guid},'SCFormationOn')
        self.assertEqual(guarded.formationItem.things[-1].pos,7)
        self.sql(f"UPDATE p9_pet_states SET next_free={int((time.time()-50*3600)*1000)} WHERE player_id={login.playerId};")
        refreshed=call('CSPetRecruitGet',{},'SCPetRecruitGet').recruit;self.assertEqual(refreshed.num,1);self.assertGreater(refreshed.nextFree,int(time.time()*1000))
        stamp+=1;stamps=[stamp+i for i in range(8)];stamp+=8
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as executor:
            races=list(executor.map(lambda tick:self.stamped('CSPetRecruit',{'free':1,'numType':1},login,tick),stamps))
        self.assertEqual(sum(op==21516 for op,_ in races),1)
        self.sql("UPDATE p9_pet_recruit_rules SET enabled=0 WHERE id='ordinary';")
        self.assertEqual(call('CSPetRecruitGet',{},'SCPetRecruitGet').recruit.num,0)
        before=self.profile(login);self.assertEqual(self.stamped('CSPetRecruit',{'free':0,'numType':1},login,stamp+1)[0],10001);self.assertEqual(self.profile(login),before)
        self.stop();self.start();login=self.login('ordinary-pet-pool')
        self.assertEqual(self.call('CSPetRecruit',{'free':0,'numType':2},login,paid_stamp,'SCPetRecruit'),paid)
        listing=call('CSPetList',{},'SCPetList');self.assertEqual(len(listing.petList),9)
        self.assertIn(free.get.pets[0],list(listing.petList))
        self.assertEqual(self.profile(login)[0]['diamond'],380)
        self.assertEqual(self.profile(login)[0]['coin'],1221000)
        self.assertEqual(int(self.sql(f"SELECT SUM(amount) FROM p1_event_facts WHERE player_id={login.playerId} AND kind='pet_recruit';").strip()),8)
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('p9_pet_recruit_rules','p9_pet_recruit_progress','p9_pet_instances','p9_pet_receipts');").strip(),'InnoDB')
        report={'date':'2026-10-14','policy':'pet-recruit-78-v1','poolOriginalIDs':78,'firstIndependent':'pet002 does not spend ordinary free/pity','sourceDiamonds':1230,'actualDiamondsSpent':850,'ordinaryDraws':8,'paidFive700Single150':'passed','coin2000Each':'passed','eightConcurrentExactFrame':'one gain, unique actual GUIDs','distinctFreeFrames':'one winner','actualGuardPos7AndColdInventory':'passed','poolInputHash':checksum,'livePoolEditDoesNotChangeFrozenPlayer':'passed','offlineFreeCap':'50h isolated timer prerequisite, one free replenished','disabledColdHistoricalReceipt':'passed','nativeOnly':'original binary HTTP, not phone UI','preparation':'level34 / chapter27 historical stars / near-pity and elapsed-clock prerequisites; spent diamonds/pets never inserted','originalBoundary':'pool70/29/1 and gold guarantee/reset quality are declared local policy, original private server probability/guarantee quality unknown; original150/700/24h1free/2000coin preserved','seconds':round(time.monotonic()-began,3),'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'sourceSHA256':{k:v for k,v in self.source_shas.items() if 'p9PetRecruit' in k or 'p9PetFirstProjection' in k or 'p9Recruit' in k or k=='service/p9PetService.go'},'cleanup':'dedicated schema/user/grants and temporary build removed by class cleanup','productionModified':False}
        (shared.ROOT/'analysis/p9/recruit-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()
