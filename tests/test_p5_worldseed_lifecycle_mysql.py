"""Both original main families across all 85 seed nodes, continued growth and restart."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import unittest

import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_P5_WORLDSEED_MYSQL_TEST') == '1', 'Explicit isolated worldseed lifecycle test')
class WorldSeedLifecycleMySQLTests(unittest.TestCase):
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

    def test_all85_two_original_main_families_continued_growth_and_restart(self):
        domain = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP5OriginalWorldSeedFullLifecycle$', '-count=1'],
                                cwd=shared.ROOT / 'hyjs-server', env=self.env, text=True, capture_output=True, timeout=180)
        self.assertEqual(domain.returncode, 0, 'Dedicated InnoDB complete worldseed domain regression failed')
        self.start()
        nodes = shared.original_table('WorldSeed')
        talents = shared.original_table('TalentAdvenced')
        self.assertEqual(len(nodes), 85)
        stamp = 1800000400000
        results = []
        restart = []
        for main_id in ('hero_043', 'hero_047'):
            uid = 'worldseed-native-' + main_id
            login = self.login(uid)
            self.call('CSPlayerCreate', {'playerName': '原种子完整成长', 'heroId': main_id}, login, stamp, 'SCPlayerCreate')
            player, game = self.profile(login)
            player.update(level=100, coin=100000000, diamond=1000, noviceGuideSchedule=8)
            game['items'] = [{'id': 'item_005', 'num': 100000}, {'id': 'item_007', 'num': 1000}, {'id': 'item_017', 'num': 100}, {'id': 'item_119', 'num': 1}]
            self.write_profile(login, player, game)
            identity = player['heroWarId']
            stamp += 1
            box = self.call('CSItemUse', {'costItem': {'id': 'item_119', 'num': 1}, 'getItemIndex': 1}, login, stamp, 'SCItemUse')
            equip = box.get.equips[0]
            stamp += 1
            self.call('CSFormationOn', {'formatPos': 1, 'id': equip.equipId, 'guid': equip.guid}, login, stamp, 'SCFormationOn')

            def activate(hero_id, tier):
                nonlocal stamp
                node = next(row for row in talents if row['warId'] == hero_id and row['talentLevel'] == str(tier))
                costs = [{'id': part.split(',')[0], 'num': int(part.split(',')[1])} for part in node['cost'].split(';')]
                stamp += 1
                self.call('CSWarriorTalent', {'guid': identity, 'talent': str(tier), 'cost': costs}, login, stamp, 'SCWarriorTalent')

            activate(main_id, 1)
            current_id = main_id
            current_talent = 1
            transitions = []
            for node in nodes[1:]:
                stamp += 1
                fields = {'id': int(node['id'])}
                response = self.call('CSLightenWorldSeed', fields, login, stamp, 'SCLightenWorldSeed')
                self.assertEqual(self.call('CSLightenWorldSeed', fields, login, stamp, 'SCLightenWorldSeed'), response)
                if node['type'] == '3':
                    self.assertEqual(response.newWarrior.guid, identity)
                    current_id = response.newWarrior.id
                    current_talent += 1
                    activate(current_id, current_talent)
                    transitions.append({'node': int(node['id']), 'mainHero': current_id, 'continuedTalent': current_talent})
            player, game = self.profile(login)
            main = next(row for row in game['warriors'] if row['guid'] == identity)
            equipment = next(row for row in game['equipment'] if row['guid'] == equip.guid)
            self.assertEqual((player['heroWarId'], player['currentWorldSeedId'], main['id'], main['talent']), (identity, 85, current_id, 3))
            self.assertEqual(next(row['num'] for row in game['items'] if row['id'] == 'item_017'), 16)
            self.assertEqual(equipment['lastHeroId'], current_id)
            before = self.profile(login)
            stamp += 1
            self.assertEqual(self.stamped('CSLightenWorldSeed', {'id': 86}, login, stamp)[0], 10001)
            self.assertEqual(self.profile(login), before)
            stamp += 1
            training_fields = {'guid': identity, 'costType': 3, 'numType': 1}
            self.call('CSWarriorTraining', training_fields, login, stamp, 'SCWarriorTraining')
            stamp += 1
            self.call('CSWarriorTrainingReplace', {'guid': identity, 'result': 1}, login, stamp, 'SCWarriorTrainingReplace')
            stamp += 1
            self.assertEqual(self.stamped('CSWarriorReset', {'guid': identity}, login, stamp)[0], 10001)
            saved = self.profile(login)
            ledger = json.loads(next(row['value'] for row in saved[1]['kv'] if row['key'] == 'p5.invest:' + identity))
            self.assertNotIn('item_017', {row['id'] for row in ledger['costs']})
            restart.append((uid, identity, current_id, saved))
            results.append({'originalMain': main_id, 'highestWorldSeed': 85, 'seedCost': 84, 'qualityTransitions': transitions,
                            'originalBoxToEquipment': equip.equipId, 'continuedTraining': 'passed', 'identityProtection': 'passed',
                            'noRefundableGlobalSeedCosts': True})
        self.stop()
        self.start()
        for uid, identity, current_id, saved in restart:
            login = self.login(uid)
            self.assertEqual(self.profile(login), saved)
            warrior = next(row for row in self.call('CSWarriorList', {}, login, stamp + 1, 'SCWarriorList').warriorList if row.guid == identity)
            self.assertEqual((warrior.id, warrior.talent), (current_id, 3))
        (shared.ROOT / 'analysis/p5/worldseed-full-lifecycle-validation.json').write_text(json.dumps({
            'date': '2026-10-14', 'originalNodeCount': 85, 'originalMainPaths': results, 'goInnoDB': 'passed',
            'originalHTTP': 'passed', 'processRestart': 'passed', 'binarySha256': hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),
            'worldSeedServiceSha256': self.source_shas['service/worldSeedService.go'], 'productionModified': False,
            'preparation': 'finite isolated material balances and level prerequisite; actual sealed equipment box consumption',
            'remaining': ['natural item_017 supply through complete source chain', 'phone UI', 'red main form original reachable source audit']}, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
