"""All sixteen usable original treasures: native acquisition, strength100 and refunds."""
import concurrent.futures
import hashlib
import json
import math
import os
from pathlib import Path
import subprocess
import unittest

import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_P5_TREASURE_MYSQL_TEST') == '1', 'Explicit isolated highest treasure test')
class TreasureLifecycleMySQLTests(unittest.TestCase):
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

    def test_all16_original_treasures_strength100_refund_restart(self):
        domain = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP5OriginalTreasureHighestFullScope$', '-count=1'],
                                cwd=self.frozen_source, env=self.env, text=True, capture_output=True, timeout=180)
        self.assertEqual(domain.returncode, 0, 'Dedicated InnoDB full highest treasure domain regression failed')
        self.start()
        treasures = shared.original_table('Treasure')
        usable = [row for row in treasures if int(row['maxStrengthen']) > 0]
        self.assertEqual(len(usable), 16)
        self.assertEqual([row['id'] for row in treasures if int(row['maxStrengthen']) == 0], ['trea_005', 'trea_010'])
        thresholds = shared.original_table('TreasureStrengthLevelExp')
        fragments = shared.original_table('Frag')
        config = {row['StaticName']: row['NameValue'] for row in shared.original_table('GameConfigure')}
        prepared = []
        results = []
        restart = []
        for index, row in enumerate(usable):
            uid = 'treasure-native-' + row['id']
            login = self.login(uid)
            self.call('CSPlayerCreate', {'playerName': '原饰品最高成长', 'heroId': 'hero_043'}, login, 1800000700000 + index, 'SCPlayerCreate')
            total_exp = sum(int(r['exp']) for r in thresholds if r['quality'] == row['quality'] and int(r['level']) < 100)
            materials = math.ceil(total_exp / 200000) * 4
            parts = [frag for frag in fragments if frag['target'] == row['id']]
            player, game = self.profile(login)
            player.update(level=100, coin=1000000000, diamond=1000, equipBagSize=1000, noviceGuideSchedule=8)
            game['items'] = [{'id': frag['id'], 'num': int(frag['num'])} for frag in parts] + [{'id': 'item_118', 'num': materials}]
            self.write_profile(login, player, game)
            target = self.call('CSCombine', {'targetId': row['id']}, login, 1800000701000 + index, 'SCCombine').get.treas[0]
            prepared.append((index, uid, row, login, target))

        def exercise(entry):
            index, uid, row, login, target = entry
            stamp = 1800000710000 + index * 10000
            def call(name, fields, response):
                nonlocal stamp
                stamp += 1
                return self.call(name, fields, login, stamp, response)
            total_exp = sum(int(r['exp']) for r in thresholds if r['quality'] == row['quality'] and int(r['level']) < 100)
            batches = math.ceil(total_exp / 200000)
            materials = batches * 4
            parts = [frag for frag in fragments if frag['target'] == row['id']]
            self.assertEqual(len(parts), int(row['needChipNum']))
            self.assertEqual((target.strengthLevel, target.refineLevel, target.currentExp), (1, 0, 0))
            food = []
            for offset in range(0, materials, 100):
                gain = call('CSItemUse', {'costItem': {'id': 'item_118', 'num': min(100, materials - offset)}, 'getItemIndex': 1}, 'SCItemUse')
                self.assertTrue(all(t.treasureId == 'trea_015' and t.strengthLevel == 1 and t.refineLevel == 0 for t in gain.get.treas))
                food.extend(t.guid for t in gain.get.treas)
            self.assertEqual(len(food), materials)
            fees = 0
            for offset in range(0, materials, 4):
                fields = {'targetId': target.guid, 'guids': food[offset:offset + 4]}
                response = call('CSTreasureStrengthen', fields, 'SCTreasureStrengthen')
                fees += sum(r.num for r in response.cost if r.id == 'coin')
                self.assertEqual(sum(r.num for r in response.cost if r.guid), 4)
                self.assertEqual(self.stamped('CSTreasureStrengthen', fields, login, stamp)[0], 10001)
            self.assertEqual((response.treasure.strengthLevel, response.treasure.currentExp), (100, materials * 50000 - total_exp))
            self.assertGreater(response.treasure.currentExp, 0)
            self.assertEqual(fees, int(materials * 50000 * float(config['TreasureStrengthCostExpToCoin'])))
            saved = self.profile(login)
            self.assertEqual((saved[0]['coin'], len(saved[1]['treasures'])), (1000000000 - fees, 1))
            self.assertTrue(all(item['num'] == 0 for item in (saved[1].get('items') or [])))
            self.assertEqual(self.stamped('CSTreasureStrengthen', {'targetId': target.guid, 'guids': food[:4]}, login, stamp + 1)[0], 10001)
            self.assertEqual(self.profile(login), saved)
            call('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, 'SCCopyEnter')
            base = call('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, 'SCCopyRunMapInfo').battlePlayer.formation[0]
            call('CSFormationOn', {'formatPos': 1, 'id': row['id'], 'guid': target.guid}, 'SCFormationOn')
            mounted = call('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, 'SCCopyRunMapInfo').battlePlayer.formation[0]
            self.assertTrue(any(getattr(mounted.prop, key) > getattr(base.prop, key) for key in ('att', 'hp', 'cri', 'perhit', 'anticri', 'perevade')))
            call('CSFormationOff', {'formatPos': 1, 'id': row['id'], 'guid': target.guid}, 'SCFormationOff')
            player, game = self.profile(login)
            player['equipBagSize'] = 110
            self.write_profile(login, player, game)
            before = self.profile(login)
            stamp += 1
            reset = {'targetId': target.guid}
            self.assertEqual(self.stamped('CStreasureRebuild', reset, login, stamp)[0], 10001)
            self.assertEqual(self.profile(login), before)
            player['equipBagSize'] = 1000
            self.write_profile(login, player, game)
            stamp += 1
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                replies = list(pool.map(lambda _: self.call('CStreasureRebuild', reset, login, stamp, 'SCtreasureRebuild'), range(8)))
            self.assertTrue(all(reply == replies[0] for reply in replies))
            final = self.profile(login)
            returned = final[1]['treasures']
            restored = next(t for t in returned if t['guid'] == target.guid)
            self.assertEqual((restored['strengthLevel'], restored['currentExp']), (1, 0))
            self.assertEqual((final[0]['coin'], len(returned)), (1000000000, materials + 1))
            self.assertEqual(final[0]['diamond'], 1000 - int(config['EquipmentRebuildCost'].split(',')[1]))
            self.assertEqual(sum(t['treasureId'] == 'trea_015' and t['guid'] != target.guid for t in returned), materials)
            restart.append((uid, reset, stamp, final))
            results.append({'treasureId': row['id'], 'originalFragmentAcquisition': 'passed', 'sealedOriginalRedBoxMaterials': materials,
                            'highestStrength': 100, 'retainedExp': materials * 50000 - total_exp, 'exactCoinCost': fees,
                            'realBattleEquipmentEffect': 'passed', 'lateRefundChunkCapacityRollback': 'passed', 'eightConcurrentReset': 'one refund, identical receipts'})
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            list(pool.map(exercise, prepared))
        self.stop()
        self.start()
        for uid, fields, stamp, saved in sorted(restart):
            login = self.login(uid)
            self.call('CStreasureRebuild', fields, login, stamp, 'SCtreasureRebuild')
            self.assertEqual(self.profile(login), saved)
        report = {'date': '2026-10-14', 'usableOriginalTreasures': sorted(results, key=lambda row: row['treasureId']),
                  'materialOnlyOriginalTreasures': ['trea_005', 'trea_010'], 'goInnoDB': 'passed', 'originalHTTP': 'passed',
                  'processRestart': 'passed', 'productionModified': False, 'binarySha256': hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),
                  'sourceSha256': self.source_shas, 'originalIL': 'analysis/p5/treasure-highest-original.il.txt',
                  'baselineSource': self.baseline_source,
                  'preparation': 'finite isolated original fragments, sealed item_118 balances, level and capacity prerequisite; genuine CSCombine and original box consumption',
                  'remaining': ['natural supply for every original fragment and item_118', 'refine30 full scope', 'phone UI and original dynamic golden']}
        (shared.ROOT / 'analysis/p5/treasure-highest-lifecycle-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
