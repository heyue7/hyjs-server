"""Original 15 enabled awakenings: fragment acquisition to fusion100 and reset."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import unittest

from server.codec import BY_NAME
import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_P5_AWAKE_LIFECYCLE_MYSQL_TEST') == '1', 'Explicit isolated awakening lifecycle test')
class AwakeLifecycleMySQLTests(unittest.TestCase):
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

    def test_all15_fragment_talent_awake_fusion100_refund_and_restart(self):
        domain = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP5OriginalAwakeFullLifecycle$', '-count=1'],
                                cwd=shared.ROOT / 'hyjs-server', env=self.env, text=True, capture_output=True, timeout=180)
        self.assertEqual(domain.returncode, 0, 'Dedicated InnoDB complete awakening domain regression failed')
        self.start()
        heroes = [row for row in shared.original_table('Hero') if row['heroClass'] == '0' and row['IsOpenAwake'] == '1']
        fragments = {row['target']: row for row in shared.original_table('Frag')}
        awake_costs = shared.original_table('HeroAwakenCost')
        fuse_levels = shared.original_table('HeroAwakenFuseLevel')
        stamp = 1800000200000
        results = []
        restart = []
        for hero in heroes:
            hero_id = hero['id']
            uid = 'p5-native-awake-' + hero_id + '-' + secrets.token_hex(3)
            login = self.login(uid)
            self.call('CSPlayerCreate', {'playerName': '原觉醒闭环', 'heroId': 'hero_043'}, login, stamp, 'SCPlayerCreate')
            player, game = self.profile(login)
            player.update(level=100, noviceGuideSchedule=8, coin=100000000, diamond=1000, warriorBagSize=1000)
            resources = {'item_005': 100000, 'item_312': 100000}
            for row in awake_costs:
                if row['heroId'] == hero_id:
                    for part in row['itemCost'].split(';'):
                        resources[part.split(',')[0]] = 100000
            fragment = fragments[hero_id]
            resources[fragment['id']] = int(fragment['num']) * 8
            game['items'] = [{'id': key, 'num': value} for key, value in resources.items()]
            self.write_profile(login, player, game)
            target = None
            for _ in range(8):
                stamp += 1
                gain = self.call('CSCombine', {'targetId': hero_id}, login, stamp, 'SCCombine')
                self.assertEqual(gain.get.wars[0].id, hero_id)
                target = target or gain.get.wars[0]
            player, game = self.profile(login)
            owned = next(row for row in game['warriors'] if row['guid'] == target.guid)
            owned['level'] = 100
            self.write_profile(login, player, game)
            initial = self.profile(login)
            stamp += 1
            fields = {'guid': target.guid, 'talent': '1', 'cost': [{'id': 'coin', 'num': 10000}, {'id': 'item_005', 'num': 30}]}
            self.call('CSWarriorTalent', fields, login, stamp, 'SCWarriorTalent')
            fields = {'guid': target.guid}
            for stage in (1, 2):
                stamp += 1
                if stage == 1:
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                        replies = list(pool.map(lambda _: self.call('CSWarriorAwake', fields, login, stamp, 'SCWarriorAwake'), range(8)))
                    self.assertTrue(all(reply == replies[0] for reply in replies))
                    response = replies[0]
                else:
                    response = self.call('CSWarriorAwake', fields, login, stamp, 'SCWarriorAwake')
                self.assertEqual(response.newWarrior.awake.stage, stage)
                self.assertEqual(self.call('CSWarriorAwake', fields, login, stamp, 'SCWarriorAwake'), response)
            exp = sum(int(row['needExp']) for row in fuse_levels if row['quality'] == hero['quality'])
            number = (exp + 499) // 500
            stamp += 1
            fuse = {'guid': target.guid, 'cost': [{'id': 'item_312', 'num': number}]}
            fused = self.call('CSAwakeFuse', fuse, login, stamp, 'SCAwakeFuse')
            self.assertEqual((fused.newWarrior.awake.stage, fused.newWarrior.awake.fuseLevel, fused.newWarrior.awake.fuseExp), (2, 100, 0))
            self.assertEqual(self.call('CSAwakeFuse', fuse, login, stamp, 'SCAwakeFuse'), fused)
            stamp += 1
            ordered = hero['AwakeSkillId'] + ',' + hero['SkillId2'] + ',' + hero['SkillId1']
            self.call('CSSetSkillOrder', {'heroGuid': target.guid, 'skillOrder': ordered}, login, stamp, 'SCSetSkillOrder')
            before = self.profile(login)
            stamp += 1
            self.assertEqual(self.stamped('CSAwakeFuse', fuse, login, stamp)[0], 10001)
            self.assertEqual(self.stamped('CSWarriorAwake', fields, login, stamp)[0], 10001)
            self.assertEqual(self.profile(login), before)
            stamp += 1
            reset = self.call('CSWarriorReset', fields, login, stamp, 'SCWarriorReset')
            saved = self.profile(login)
            self.assertEqual(saved[0]['coin'], initial[0]['coin'])
            self.assertEqual(len(saved[1]['warriors']), len(initial[1]['warriors']))
            expected = dict(next(w for w in initial[1]['warriors'] if w['guid'] == target.guid))
            expected['skillOrder'] = hero['SkillId2'] + ',' + hero['SkillId1']
            self.assertEqual(next(w for w in saved[1]['warriors'] if w['guid'] == target.guid), expected)
            expected_items = {w['id']: w['num'] for w in initial[1]['items']}
            actual_items = {w['id']: w['num'] for w in saved[1]['items']}
            self.assertEqual(actual_items, expected_items)
            reset_stamp = stamp
            stamp += 1
            self.call('CSFormationOn', {'formatPos': 2, 'id': hero_id, 'guid': target.guid}, login, stamp, 'SCFormationOn')
            self.call('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login, stamp + 1, 'SCCopyEnter')
            battle = self.call('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, login, stamp + 2, 'SCCopyRunMapInfo')
            actor = next(w for w in battle.battlePlayer.formation if w.guid == target.guid)
            self.assertEqual((actor.awakeStage, actor.awakenSkill, actor.skillOrder),
                             (0, '', expected['skillOrder'] + ',' + hero['UltSkillId']))
            saved = self.profile(login)
            results.append({'heroId': hero_id, 'acquisition': 'eight original CSCombine requests from exact finite fragment fixtures',
                            'talent': 1, 'awakeStages': [1, 2], 'maxFuse': 100, 'fusionCost': number,
                            'concurrentAwake': 'eight identical receipts, one consumption', 'fullActualRefund': 'passed',
                            'highestAwakenedSkillReorderToResetToFormation': 'passed'})
            restart.append((uid, target.guid, fields, reset_stamp, reset, saved))
            stamp += 3
        self.stop()
        self.start()
        for uid, guid, fields, old_stamp, reset, saved in restart:
            login = self.login(uid)
            self.assertEqual(self.call('CSWarriorReset', fields, login, old_stamp, 'SCWarriorReset'), reset)
            self.assertEqual(self.profile(login), saved)
            warrior = next(w for w in self.call('CSWarriorList', {}, login, stamp + 1, 'SCWarriorList').warriorList if w.guid == guid)
            self.assertEqual((warrior.talent, warrior.awake.stage, warrior.awake.fuseLevel), (0, 0, 0))
        self.assertEqual(len(results), 15)
        (shared.ROOT / 'analysis/p5/awake-full-lifecycle-validation.json').write_text(json.dumps({
            'date': '2026-10-14', 'allOriginalEnabledHeroes': results, 'originalHTTP': 'passed',
            'goInnoDBFullLifecycle': 'passed', 'processRestartReceipts': '15/15 retained without duplicate refund',
            'binarySha256': hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(), 'sourceSha256': self.source_shas,
            'productionModified': False, 'remaining': ['natural fragment/material source per ID', 'phone UI', 'dynamic native awakening combat golden'],
            'preparation': 'finite original fragment/material balances and level prerequisite in dedicated test saves only'}, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
