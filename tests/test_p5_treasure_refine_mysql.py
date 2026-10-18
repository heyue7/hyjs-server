"""All original refine0-to30 paths with genuine native fragment-created donors."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import subprocess
import unittest
import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_P5_TREASURE_REFINE_MYSQL_TEST') == '1', 'Explicit isolated treasure refinement test')
class TreasureRefineMySQLTests(unittest.TestCase):
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

    def test_all16_refine30_exact_costs_native_donors_reset_restart(self):
        domain = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP5OriginalTreasureRefineFullScope$', '-count=1'],
                                cwd=self.frozen_source, env=self.env, text=True, capture_output=True, timeout=180)
        self.assertEqual(domain.returncode, 0, 'Dedicated InnoDB full refinement domain regression failed')
        self.start()
        usable = [r for r in shared.original_table('Treasure') if int(r['maxRefine']) > 0]
        self.assertEqual(len(usable), 16)
        rows = shared.original_table('TreasureRefineLevel')
        fragments = shared.original_table('Frag')
        prepared = []
        results = []
        restart = []
        for index, config in enumerate(usable):
            uid = 'treasure-refine-native-' + config['id']
            login = self.login(uid)
            self.call('CSPlayerCreate', {'playerName': '原饰品满精炼', 'heroId': 'hero_043'}, login, 1800000800000 + index, 'SCPlayerCreate')
            costs = {}
            for row in rows:
                if row['treasureId'] != config['id']: continue
                for part in row['objStr'].split(';'):
                    key, number = part.split(',')
                    costs[key] = costs.get(key, 0) + int(number)
            donors = costs[config['id']]
            parts = [r for r in fragments if r['target'] == config['id']]
            player, game = self.profile(login)
            player.update(level=100, coin=1000000000, diamond=1000, equipBagSize=1000, noviceGuideSchedule=8)
            game['items'] = [{'id':r['id'], 'num':int(r['num']) * (donors + 1)} for r in parts] + [{'id':'item_012','num':costs['item_012']}]
            self.write_profile(login, player, game)
            target = self.call('CSCombine', {'targetId':config['id']}, login, 1800000801000 + index, 'SCCombine').get.treas[0]
            prepared.append((index, uid, config, login, target))

        def exercise(entry):
            index, uid, config, login, target = entry
            stamp = 1800000810000 + index * 10000
            def call(name, fields, response):
                nonlocal stamp
                stamp += 1
                return self.call(name, fields, login, stamp, response)
            costs = {}
            levels = [r for r in rows if r['treasureId'] == config['id']]
            self.assertEqual(len(levels), 30)
            for row in levels:
                for part in row['objStr'].split(';'):
                    key, number = part.split(',')
                    costs[key] = costs.get(key, 0) + int(number)
            donors = costs[config['id']]
            parts = [r for r in fragments if r['target'] == config['id']]
            identities = []
            for _ in range(donors):
                material = call('CSCombine', {'targetId':config['id']}, 'SCCombine').get.treas[0]
                identities.append(material.guid)
                self.assertEqual((material.strengthLevel, material.refineLevel, material.currentExp), (1,0,0))
            consumed = []
            for row in levels:
                fields = {'targetId':target.guid}
                result = call('CSTreasureRefine', fields, 'SCTreasureRefine')
                self.assertEqual(result.treasure.refineLevel, int(row['level']))
                consumed.extend(c.guid for c in result.cost if c.guid)
                self.assertEqual(self.call('CSTreasureRefine', fields, login, stamp, 'SCTreasureRefine'), result)
            self.assertEqual(consumed, identities)
            highest = self.profile(login)
            self.assertEqual((highest[0]['coin'], len(highest[1]['treasures'])), (1000000000-costs['coin'],1))
            self.assertTrue(all(r['num']==0 for r in (highest[1].get('items') or [])))
            stamp += 1
            self.assertEqual(self.stamped('CSTreasureRefine', {'targetId':target.guid}, login, stamp)[0],10001)
            self.assertEqual(self.profile(login), highest)
            call('CSCopyEnter', {'chapterId':'1','missionId':'1'}, 'SCCopyEnter')
            base = call('CSCopyRunMapInfo', {'chapterId':'1','missionId':'1'}, 'SCCopyRunMapInfo').battlePlayer.formation[0]
            call('CSFormationOn', {'formatPos':1,'id':config['id'],'guid':target.guid}, 'SCFormationOn')
            mounted = call('CSCopyRunMapInfo', {'chapterId':'1','missionId':'1'}, 'SCCopyRunMapInfo').battlePlayer.formation[0]
            self.assertTrue(any(getattr(mounted.prop,k)>getattr(base.prop,k) for k in ('att','hp','cri','perhit','anticri','perevade')))
            call('CSFormationOff', {'formatPos':1,'id':config['id'],'guid':target.guid}, 'SCFormationOff')
            player, game = self.profile(login)
            player['equipBagSize']=110
            self.write_profile(login,player,game)
            before = self.profile(login)
            stamp += 1
            reset = {'targetId':target.guid}
            self.assertEqual(self.stamped('CStreasureRebuild',reset,login,stamp)[0],10001)
            self.assertEqual(self.profile(login),before)
            player['equipBagSize']=1000
            self.write_profile(login,player,game)
            response = call('CStreasureRebuild',reset,'SCtreasureRebuild')
            self.assertEqual(self.call('CStreasureRebuild',reset,login,stamp,'SCtreasureRebuild'),response)
            final = self.profile(login)
            restored = next(r for r in final[1]['treasures'] if r['guid']==target.guid)
            self.assertEqual((restored['refineLevel'],restored['strengthLevel'],restored['currentExp']),(0,1,0))
            self.assertEqual((final[0]['coin'],len(final[1]['treasures'])),(1000000000,donors+1))
            self.assertEqual(next(r['num'] for r in final[1]['items'] if r['id']=='item_012'),costs['item_012'])
            restart.append((uid,reset,stamp,final))
            results.append({'treasureId':config['id'],'highestRefine':30,'nativeFragmentCreatedDonors':donors,
                            'exactOriginalCosts':costs,'battleProperties':'passed','lateCapacityRollback':'passed','fullActualInvestmentRefund':'passed'})
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            list(pool.map(exercise,prepared))
        self.stop()
        self.start()
        for uid,fields,stamp,saved in sorted(restart):
            self.call('CStreasureRebuild',fields,self.login(uid),stamp,'SCtreasureRebuild')
            self.assertEqual(self.profile(self.login(uid)),saved)
        (shared.ROOT/'analysis/p5/treasure-refine-lifecycle-validation.json').write_text(json.dumps({
            'date':'2026-10-14','all16Paths':sorted(results,key=lambda r:r['treasureId']), 'goInnoDB':'passed','originalHTTP':'passed',
            'restart':'passed','sourceSha256':self.source_shas,'binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),
            'baselineSource':self.baseline_source,
            'productionModified':False,'preparation':'finite isolated original fragments and item_012; target and all 6144 donors acquired by native CSCombine',
            'remaining':['natural fragment and item_012 source coverage','phone UI','original dynamic golden']},ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':
    unittest.main()
