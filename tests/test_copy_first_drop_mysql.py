"""Original first-drop rewards on disposable InnoDB; wins are protocol fixtures."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import time
import unittest

import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as transport


@unittest.skipUnless(os.environ.get('HYJS_COPY_FIRST_MYSQL_TEST')=='1','Explicit isolated original first-drop regression')
class CopyFirstDropMySQLTests(unittest.TestCase):
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

    def test_original_frozen_first_source_and_real_inventory_use(self):
        began=time.monotonic();self.start();login=self.login('original-first-source')
        stamp=1800010000000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        call('CSPlayerCreate',{'playerName':'原首次来源','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=1000000,stamina=1000,warriorBagSize=1000,equipBagSize=1000,noviceGuideSchedule=8)
        self.write_profile(login,p,g)
        call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
        tasks=shared.original_table('CopyTask');results=[]
        for task in [x for x in tasks if x['MissionId']=='1']:
            start=call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':task['TaskId']},'SCCopyFightStart')
            end={'fightId':start.fightId,'result':1,'battlePlayer':start.battlePlayer}
            if task['TaskId']!='4':
                call('CSCopyFightEnd',end,'SCCopyFightEnd');continue
            raw,checksum=self.sql(f"SELECT HEX(input),input_hash FROM copy_drop_snapshots WHERE player_id={login.playerId} AND fight_id='{start.fightId}';").strip().split('\t')
            frozen=bytes.fromhex(raw);self.assertEqual(hashlib.sha256(frozen).hexdigest(),checksum)
            first=json.loads(frozen)['first'];self.assertEqual(len(first['entries'][0]['options']),4)
            self.sql("UPDATE copy_drop_rules SET enabled=0,mode='invalid' WHERE id='first';")
            p,g=self.profile(login);before=len(g['warriors']);p['warriorBagSize']=before;self.write_profile(login,p,g)
            rejected=self.profile(login);self.assertEqual(self.stamped('CSCopyFightEnd',end,login,stamp+1)[0],10001);self.assertEqual(self.profile(login),rejected)
            p['warriorBagSize']=1000;self.write_profile(login,p,g);stamp+=1
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                replies=list(pool.map(lambda _:self.call('CSCopyFightEnd',end,login,stamp,'SCCopyFightEnd'),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies))
            p,g=self.profile(login);self.assertIn(len(g['warriors'])-before,range(4,9))
            saved=[p,g];self.stop();self.start();login=self.login('original-first-source')
            self.assertEqual(self.call('CSCopyFightEnd',end,login,stamp,'SCCopyFightEnd'),replies[0]);self.assertEqual(self.profile(login),saved)
            results.append({'taskId':'4','randomInstanceDelta':len(g['warriors'])-before,'configurationHash':checksum,'eightConcurrent':'one exact result','coldExactReceipt':True,'capacityRollback':True})
        self.sql("UPDATE copy_drop_rules SET enabled=1,mode='relative-weights-v1' WHERE id='first';")
        missions={x['MissionId']:x for x in shared.original_table('CopyMission')}
        # Only prior encounter prerequisites are isolated. Source equipment/gem
        # rewards below are never inserted or directly granted.
        for tid in ['20','92']:
            task=next(x for x in tasks if x['TaskId']==tid);mission=missions[task['MissionId']]
            p,g=self.profile(login);g['run']={'chapter':mission['ChapterId'],'mission':task['MissionId'],'passed':[x['TaskId'] for x in tasks if x['MissionId']==task['MissionId'] and x['TaskId']!=tid]}
            self.write_profile(login,p,g)
            start=call('CSCopyFightStart',{'chapterId':mission['ChapterId'],'missionId':task['MissionId'],'taskId':tid},'SCCopyFightStart')
            end={'fightId':start.fightId,'result':1,'battlePlayer':start.battlePlayer}
            gained=call('CSCopyFightEnd',end,'SCCopyFightEnd')
            equipid='equip_001' if tid=='20' else 'equip_004'
            equip=next(x for x in gained.gain.equips if x.equipId==equipid)
            listing=call('CSEquipmentTreasureList',{},'SCEquipmentTreasureList')
            self.assertIn(equip,list(listing.equipmentList))
            call('CSFormationOn',{'formatPos':1,'id':equipid,'guid':equip.guid},'SCFormationOn')
            if tid=='92':
                gems=call('CSGemAndCookingList',{},'SCGemAndCookingList');self.assertEqual(next(x.num for x in gems.gemList if x.id=='MineItem_029'),1)
            saved=self.profile(login);self.stop();self.start();login=self.login('original-first-source')
            self.assertEqual(self.call('CSCopyFightEnd',end,login,stamp+1,'SCCopyFightEnd'),gained);self.assertEqual(self.profile(login),saved)
            p,g=self.profile(login);g['run']={'chapter':mission['ChapterId'],'mission':task['MissionId'],'passed':[x['TaskId'] for x in tasks if x['MissionId']==task['MissionId'] and x['TaskId']!=tid]}
            self.write_profile(login,p,g)
            retry_start=call('CSCopyFightStart',{'chapterId':mission['ChapterId'],'missionId':task['MissionId'],'taskId':tid},'SCCopyFightStart')
            repeated=call('CSCopyFightEnd',{'fightId':retry_start.fightId,'result':1,'battlePlayer':retry_start.battlePlayer},'SCCopyFightEnd')
            self.assertFalse(repeated.gain.equips)
            if tid=='92':
                gems=call('CSGemAndCookingList',{},'SCGemAndCookingList');self.assertEqual(next(x.num for x in gems.gemList if x.id=='MineItem_029'),1)
            results.append({'taskId':tid,'firstEquipmentId':equipid,'actualGUID':equip.guid,'equippedAndCold':True,'MineItem029GemInventory':tid=='92','newFightAfterFirstPassCannotRepeatFirst':True})
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('profiles','copy_drop_rules','copy_drop_snapshots');").strip(),'InnoDB')
        report={'date':'2026-10-14','originalFirstEntries':310,'sourceTypes':{'hero':203,'frag':75,'item':27,'equip':4,'MineItem':1},'originalFramesAndInnoDB':'passed','results':results,'seconds':round(time.monotonic()-began,3),'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'ownedSourceSHA256':{k:v for k,v in self.source_shas.items() if k in ['service/copyFirstDropService.go','service/copyDropService.go','service/copyService.go','constUtil/copyDrop.go']},'boundary':'relative-weights-v1 local compatibility: single100/1000 amounts are fixed, multiple amounts sample relative weights; original server probability not recovered; battle winners are explicit isolated client-result fixtures','preparation':'finite player level/coin/stamina and prior encounter prerequisites for Tasks20/92; no source experience/equipment/gem inserted','productionModified':False,'cleanup':'dedicated schema/user/grants and temporary source/binary cleaned by class cleanup'}
        (shared.ROOT/'analysis/p5/copy-first-drop-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()
