"""Real red179/180 equal quote: exact original frame does not pay twice."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import unittest
import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as stressed_transport


@unittest.skipUnless(os.environ.get('HYJS_P5_EQUAL_QUOTE_MYSQL_TEST')=='1','Explicit original equal quote lifecycle test')
class EqualQuoteMySQLTests(unittest.TestCase):
    setUpClass=classmethod(shared.TalentScopeMySQLTests.setUpClass.__func__)
    root_sql=classmethod(shared.TalentScopeMySQLTests.root_sql.__func__)
    send=shared.TalentScopeMySQLTests.send
    login=shared.TalentScopeMySQLTests.login
    stamped=stressed_transport.GameCreationMySQLTests.stamped
    call=shared.TalentScopeMySQLTests.call
    profile=shared.TalentScopeMySQLTests.profile
    start=shared.TalentScopeMySQLTests.start
    stop=shared.TalentScopeMySQLTests.stop
    setUp=shared.TalentScopeMySQLTests.setUp
    sql=shared.TalentScopeMySQLTests.sql
    write_profile=shared.TalentScopeMySQLTests.write_profile

    def test_eight_same_stamp_one_stage_same_price_new_stamp_and_cold(self):
        self.start();login=self.login('p5-equipment-equal-original')
        self.call('CSPlayerCreate',{'playerName':'原同价强化回执','heroId':'hero_043'},login,1800001700000,'SCPlayerCreate')
        player,game=self.profile(login);player.update(level=81,coin=1000000000,diamond=10000,equipBagSize=1000,noviceGuideSchedule=8)
        game['items']=[{'id':'item_119','num':1}];self.write_profile(login,player,game)
        gear=self.call('CSItemUse',{'costItem':{'id':'item_119','num':1},'getItemIndex':1},login,1800001700001,'SCItemUse').get.equips[0]
        self.assertEqual(gear.equipId,'equip_037')
        real179=self.call('CSEquipStrengthenOneKey',{'guid':gear.guid},login,1800001700002,'SCEquipStrengthenOneKey')
        self.assertEqual((real179.equip.level,real179.costTimes),(179,178))
        player,game=self.profile(login);player['level']=100;self.write_profile(login,player,game)
        before=self.profile(login)
        fields={'guid':gear.guid,'times':1,'cost':[{'id':'coin','num':502446}]};stamp=1800001700003
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies=list(pool.map(lambda _:self.call('CSEquipStrengthen',fields,login,stamp,'SCEquipStrengthen'),range(8)))
        self.assertTrue(all(r==replies[0] for r in replies));self.assertEqual(replies[0].equip.level,180)
        after=self.profile(login);self.assertEqual(before[0]['coin']-after[0]['coin'],502446)
        next_stage=self.call('CSEquipStrengthen',fields,login,stamp+1,'SCEquipStrengthen');self.assertEqual(next_stage.equip.level,181)
        saved=self.profile(login);self.assertEqual(before[0]['coin']-saved[0]['coin'],502446*2)
        self.assertEqual(self.stamped('CSEquipStrengthen',fields,login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
        self.stop();self.start();login=self.login('p5-equipment-equal-original')
        self.assertEqual(self.call('CSEquipStrengthen',fields,login,stamp+1,'SCEquipStrengthen'),next_stage);self.assertEqual(self.profile(login),saved)
        reset=self.call('CSEquipReBuild',{'guid':gear.guid},login,stamp+2,'SCEquipReBuild')
        restored=self.profile(login);self.assertEqual(next(e['level'] for e in restored[1]['equipment'] if e['guid']==gear.guid),1);self.assertEqual(restored[0]['coin'],1000000000)
        self.assertEqual(self.stamped('CSEquipStrengthen',fields,login,stamp+1)[0],10001);self.assertEqual(self.profile(login),restored)
        self.stop();self.start();login=self.login('p5-equipment-equal-original')
        self.assertEqual(self.call('CSEquipReBuild',{'guid':gear.guid},login,stamp+2,'SCEquipReBuild'),reset);self.assertEqual(self.profile(login),restored)
        (shared.ROOT/'analysis/p5/equipment-equal-quote-validation.json').write_text(json.dumps({'date':'2026-10-14','originalHTTP':'passed','isolatedMySQL':'passed','originalRedEqualFees':{'179':502446,'180':502446},'realZeroTo179':'original sealedred119 and178 actual one-key stages at original player81 cap','eightConcurrentSameOriginalFrame':'one mutation179→180,8 identical original responses,one502446 cost','legitimateNewStampSameQuote':'180→181 for another502446','oldSnapshotAfter181AndReset':'rejected,complete stored profile unchanged','coldCurrentGrowthAndResetReceipt':'identical,inventory unchanged','binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'sourceSha256':self.source_shas,'productionModified':False,'preparation':'finite player81/100 prerequisites and coins,original119box; no highest fixture'},ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()
