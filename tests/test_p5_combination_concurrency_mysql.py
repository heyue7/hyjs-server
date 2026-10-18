"""Fresh-account native combination concurrency with whole-domain retry regression."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import unittest
import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_P5_MUTATION_MYSQL_TEST') == '1', 'Explicit isolated mutation concurrency test')
class CombinationConcurrencyMySQLTests(unittest.TestCase):
    setUpClass = classmethod(shared.TalentScopeMySQLTests.setUpClass.__func__)
    root_sql = classmethod(shared.TalentScopeMySQLTests.root_sql.__func__)
    send = shared.TalentScopeMySQLTests.send
    login = shared.TalentScopeMySQLTests.login
    stamped = shared.TalentScopeMySQLTests.stamped
    call = shared.TalentScopeMySQLTests.call
    profile = shared.TalentScopeMySQLTests.profile
    start = shared.TalentScopeMySQLTests.start
    stop = shared.TalentScopeMySQLTests.stop
    setUp = shared.TalentScopeMySQLTests.setUp
    sql = shared.TalentScopeMySQLTests.sql
    write_profile = shared.TalentScopeMySQLTests.write_profile

    def test_six_fresh_equipment_strength_and_forge_primary_lock_plan(self):
        self.start()
        plan=self.sql('EXPLAIN SELECT * FROM p1_period_instances FORCE INDEX (PRIMARY) ORDER BY id ASC FOR UPDATE;')
        self.assertIn('PRIMARY',plan)
        self.assertNotIn('filesort',plan.lower())
        quotes=[int(r['coin']) for r in shared.original_table('EquipExp') if r['quality']=='23' and int(r['level'])<260]
        prepared=[]
        for index in range(6):
            login=self.login('p5-first-equipment-concurrent-'+str(index))
            self.call('CSPlayerCreate',{'playerName':'并发初装备成长','heroId':'hero_043'},login,1800000950000+index,'SCPlayerCreate')
            player,game=self.profile(login)
            player.update(level=100,coin=1000000000,equipBagSize=1000)
            guid='parallel-equipment-'+str(index)
            game['equipment']=[{'equipId':'equip_037','guid':guid,'level':1,'refineLv':0,'refineExp':0,'star':0,'forgeLevel':0,'gems':[]}]
            game['items']=[{'id':'frag_equip_037','num':3}]
            self.write_profile(login,player,game)
            prepared.append((login,guid))
        def grow(entry):
            login,guid=entry
            self.call('CSEquipStrengthen',{'guid':guid,'times':5,'cost':[{'id':'coin','num':sum(quotes[:5])}]},login,1800000951000,'SCEquipStrengthen')
            result=self.call('CSEquipStrengthenOneKey',{'guid':guid},login,1800000952000,'SCEquipStrengthenOneKey')
            self.assertEqual((result.equip.level,result.costTimes),(260,254))
            fields={'guid':guid,'cost':[{'id':'frag_equip_037','num':3},{'id':'coin','num':1000}]}
            forged=self.call('CSEquipForge',fields,login,1800000953000,'SCEquipForge')
            self.assertEqual((forged.result,forged.equip.forgeLevel),(1,1))
            self.assertEqual(self.call('CSEquipForge',fields,login,1800000953000,'SCEquipForge'),forged)
            player,game=self.profile(login)
            self.assertEqual(player['coin'],1000000000-sum(quotes)-1000)
            self.assertTrue(all(r['num']==0 for r in (game.get('items') or [])))
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:list(pool.map(grow,prepared))
        (shared.ROOT/'analysis/p5/equipment-primary-lock-concurrency-validation.json').write_text(json.dumps({
            'date':'2026-10-14','sixFreshAccountsOriginalQuotedStrengthAndOneKey260':'passed','sixActualForge1AndDuplicateReceipt':'passed',
            'explain':'PRIMARY forward index scan, no filesort','explainResult':plan.strip(),'productionModified':False,
            'binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'sourceSha256':self.source_shas},ensure_ascii=False,indent=2)+'\n')

    def test_six_fresh_combine_accounts_and_eight_identical_boxes(self):
        self.start()
        prepared = []
        parts = [r for r in shared.original_table('Frag') if r['target']=='trea_015']
        for index in range(6):
            uid='p5-first-combine-concurrent-'+str(index)
            login=self.login(uid)
            self.call('CSPlayerCreate',{'playerName':'并发初合成','heroId':'hero_043'},login,1800000900000+index,'SCPlayerCreate')
            player,game=self.profile(login)
            player.update(level=100,equipBagSize=1000)
            game['items']=[{'id':r['id'],'num':int(r['num'])} for r in parts]+[{'id':'item_118','num':4}]
            self.write_profile(login,player,game)
            prepared.append((uid,login))
        def combine(entry):
            uid,login=entry
            fields={'targetId':'trea_015'}
            response=self.call('CSCombine',fields,login,1800000901000,'SCCombine')
            self.assertEqual(len(response.get.treas),1)
            self.assertEqual(len(response.cost),len(parts))
            self.assertEqual(self.call('CSCombine',fields,login,1800000901000,'SCCombine'),response)
            player,game=self.profile(login)
            self.assertEqual(len(game['treasures']),1)
            self.assertTrue(all(r['num']==0 for r in (game['items'] or []) if r['id']!='item_118'))
            return uid,response.get.treas[0].guid
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            results=list(pool.map(combine,prepared))
        login=prepared[0][1]
        fields={'costItem':{'id':'item_118','num':4},'getItemIndex':1}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies=list(pool.map(lambda _:self.call('CSItemUse',fields,login,1800000902000,'SCItemUse'),range(8)))
        self.assertTrue(all(r==replies[0] for r in replies))
        self.assertEqual(len(replies[0].get.treas),4)
        saved=self.profile(login)
        self.assertEqual(len(saved[1]['treasures']),5)
        self.assertTrue(all(r['num']==0 for r in (saved[1].get('items') or [])))
        self.stop();self.start()
        self.assertEqual(self.call('CSItemUse',fields,self.login(prepared[0][0]),1800000902000,'SCItemUse'),replies[0])
        self.assertEqual(self.profile(self.login(prepared[0][0])),saved)
        (shared.ROOT/'analysis/p5/combination-concurrency-validation.json').write_text(json.dumps({
            'date':'2026-10-14','sixFreshAccountsCombine':'passed','eightSameBoxReceipts':'one material consumption and four original instances',
            'restartReplay':'passed','originalHTTP':'passed','productionModified':False,'paths':results,
            'binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'sourceSha256':self.source_shas,
            'baselineSource':self.baseline_source,'preparation':'finite dedicated original fragments and unopened item_118; no transaction error suppressed'},ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()
