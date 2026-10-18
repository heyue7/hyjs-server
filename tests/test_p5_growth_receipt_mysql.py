"""Actual growth/reset rejects stale complete original snapshots, keeps reset replay."""
import hashlib
import json
import os
from pathlib import Path
import unittest
import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as stressed_transport


@unittest.skipUnless(os.environ.get('HYJS_P5_RECEIPT_MYSQL_TEST')=='1','Explicit isolated reset receipt test')
class GrowthReceiptMySQLTests(unittest.TestCase):
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

    def test_actual_three_families_reset_old_growth_and_cold_receipts(self):
        self.start();login=self.login('p5-growth-receipt-native')
        self.call('CSPlayerCreate',{'playerName':'原重置回执边界','heroId':'hero_043'},login,1800001200000,'SCPlayerCreate')
        player,game=self.profile(login);player.update(level=100,coin=1000000000,diamond=10000,warriorBagSize=1000,equipBagSize=1000,noviceGuideSchedule=8)
        supplies={'item_119':1,'item_008':1,'item_459':70,'item_460':3,'item_461':20,'item_312':1,'item_439':20,'item_440':15,'item_012':1000,'frag_equip_037':3}
        for row in shared.original_table('Frag'):
            if row['target']=='hero_001':supplies[row['id']]=int(row['num'])*8
            if row['target']=='trea_015':supplies[row['id']]=int(row['num'])
        awake_costs=[r for r in shared.original_table('HeroAwakenCost') if r['heroId']=='hero_001']
        for row in awake_costs:
            for part in row['itemCost'].split(';'):
                key,number=part.split(',');supplies[key]=supplies.get(key,0)+int(number)
        game['items']=[{'id':key,'num':value} for key,value in sorted(supplies.items())];self.write_profile(login,player,game)
        stamp=1800001201000;old=[];resets=[]
        def call(name,fields,response,remember=False):
            nonlocal stamp
            stamp+=1;result=self.call(name,fields,login,stamp,response)
            if remember:old.append((name,fields,stamp))
            return result
        def reset(name,fields,response):
            result=call(name,fields,response);resets.append((name,fields,stamp,response,result));return result
        def rejected():
            saved=self.profile(login)
            for name,fields,original_stamp in old:self.assertEqual(self.stamped(name,fields,login,original_stamp)[0],10001)
            self.assertEqual(self.profile(login),saved)
            old.clear()
        gear=call('CSItemUse',{'costItem':{'id':'item_119','num':1},'getItemIndex':1},'SCItemUse').get.equips[0]
        call('CSEquipStrengthenOneKey',{'guid':gear.guid},'SCEquipStrengthenOneKey',True)
        call('CSEquipRefine',{'guid':gear.guid,'cost':[{'id':'item_008','num':1}]},'SCEquipRefine',True)
        call('CSEquipStar',{'guid':gear.guid},'SCEquipStar',True)
        call('CSEquipForge',{'guid':gear.guid,'cost':[{'id':'frag_equip_037','num':3},{'id':'coin','num':1000}]},'SCEquipForge',True)
        reset('CSEquipReBuild',{'guid':gear.guid},'SCEquipReBuild');rejected()
        hero=None
        for _ in range(8):
            response=call('CSCombine',{'targetId':'hero_001'},'SCCombine')
            if hero is None:hero=response.get.wars[0]
        player,game=self.profile(login);next(w for w in game['warriors'] if w['guid']==hero.guid)['level']=100;self.write_profile(login,player,game)
        call('CSWarriorAwake',{'guid':hero.guid},'SCWarriorAwake')
        call('CSWarriorAwake',{'guid':hero.guid},'SCWarriorAwake',True)
        call('CSAwakeFuse',{'guid':hero.guid,'cost':[{'id':'item_312','num':1}]},'SCAwakeFuse',True)
        call('CSUpRoleSkillRefineLevel',{'roleGuid':hero.guid,'type':1},'SCUpRoleSkillRefineLevel',True)
        reset('CSWarriorReset',{'guid':hero.guid},'SCWarriorReset');rejected()
        treasure=call('CSCombine',{'targetId':'trea_015'},'SCCombine').get.treas[0]
        call('CSTreasureRefine',{'targetId':treasure.guid},'SCTreasureRefine',True)
        reset('CStreasureRebuild',{'targetId':treasure.guid},'SCtreasureRebuild');rejected()
        saved=self.profile(login)
        for name,fields,original_stamp,response,result in resets:self.assertEqual(self.call(name,fields,login,original_stamp,response),result)
        self.assertEqual(self.profile(login),saved)
        self.stop();self.start();login=self.login('p5-growth-receipt-native')
        for name,fields,original_stamp,response,result in resets:self.assertEqual(self.call(name,fields,login,original_stamp,response),result)
        self.assertEqual(self.profile(login),saved)
        # New real growth retires the complete reset snapshots as well. A donor
        # returned by reset is consumed legitimately; its old returned instance
        # must not be projected back into the client's inventory.
        call('CSEquipStrengthenOneKey',{'guid':gear.guid},'SCEquipStrengthenOneKey')
        _,current_game=self.profile(login)
        donor=next(w for w in current_game['warriors'] if w['id']=='hero_001' and w['guid']!=hero.guid)
        call('CSWarriorStrengthen',{'guid':hero.guid,'cost':[{'id':donor['id'],'guid':donor['guid']}]},'SCWarriorStrengthen')
        call('CSTreasureRefine',{'targetId':treasure.guid},'SCTreasureRefine')
        changed=self.profile(login)
        for name,fields,original_stamp,_,_ in resets:
            self.assertEqual(self.stamped(name,fields,login,original_stamp)[0],10001)
        self.assertEqual(self.profile(login),changed)
        (shared.ROOT/'analysis/p5/growth-reset-receipt-validation.json').write_text(json.dumps({
            'date':'2026-10-14','originalHTTP':'passed','isolatedMySQL':'passed','expiredSnapshots':['equip260','equipRefine','equipStar','equipForge','awake2','awakeFuse','specialty','treasureRefine'],
            'currentResetReceiptReplay':'same three families before/after cold process restart','inventoryMutationOnExpiredRequest':False,'resetAfterRealRegrowthOrReturnedDonorConsumed':'all three original reset receipts rejected, inventory unchanged',
            'binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'sourceSha256':self.source_shas,'productionModified':False,
            'preparation':'finite dedicated original boxes, fragments and upgrade materials; actual original acquisition and mutations, hero100 prerequisite only'},ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()
