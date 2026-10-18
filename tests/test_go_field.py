"""Single-player field encounters against a real Go process and an isolated DB."""
import concurrent.futures
import csv
import json
import os
from pathlib import Path
import re
import selectors
import shutil
import subprocess
import types
import unittest

import test_go_chapter as chapter
from server.codec import BY_NAME, message
from server.store import Store

ROOT = Path(__file__).resolve().parents[1]


class GoFieldTests(unittest.TestCase):
    setUp = chapter.GoChapterTests.setUp
    tearDown = chapter.GoChapterTests.tearDown
    stop = chapter.GoChapterTests.stop
    send = chapter.GoChapterTests.send
    login = chapter.GoChapterTests.login
    create = chapter.GoChapterTests.create
    profile = chapter.GoChapterTests.profile
    mutate = chapter.GoChapterTests.mutate

    def start(self):
        binary = os.environ.get('HYJS_GO_TEST_BINARY', '/tmp/hyjs-field-core-test')
        args = [binary, '--db', str(self.path), '--http-port', '0', '--tcp-port', '0']
        if getattr(self, 'tables', None):
            args += ['--tables', str(self.tables)]
        self.process = subprocess.Popen(args, cwd=ROOT/'hyjs-server', stdout=subprocess.PIPE,
                                        stderr=subprocess.STDOUT, text=True)
        self.addCleanup(self.stop)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            if not ready.select(20):
                self.fail('Go server did not start within 20 seconds')
            line = self.process.stdout.readline()
        match = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(match, line)
        self.service = types.SimpleNamespace(http_port=int(match[1]), tcp_port=int(match[2]),
                                             store=Store(self.path))

    def prepare(self, uid):
        login = self.create(uid)
        self.mutate(login, lambda p, g: p.update(level=18))
        login = self.login(uid)
        opcode, result = self.send('CSTransferScene', {'target': 'field_001'}, login)
        self.assertEqual(opcode, BY_NAME['SCTransferScene'], result)
        self.assertEqual(result.result, 1)
        return login, self.monsters(login)

    def state(self, login):
        with self.service.store.connect() as conn:
            return json.loads(conn.execute('SELECT game FROM profiles WHERE id=?',
                                          (login.playerId,)).fetchone()[0])

    def monsters(self, login):
        opcode, result = self.send('CSCurrentPosition', login=login)
        self.assertEqual(opcode, BY_NAME['SCCurrentPosition'], result)
        self.assertEqual(result.self.position.scene, 'field_001')
        frames = dict(self.last_frames)
        self.assertIn(BY_NAME['SCSideMonster'], frames)
        return list(frames[BY_NAME['SCSideMonster']].monsters)

    def begin(self, login, monster):
        opcode, fight = self.send('CSSidePveFightStart', {'monsterId': monster.uuid}, login)
        self.assertEqual(opcode, BY_NAME['SCSidePveFightStart'], fight)
        # Original BattleData selects Field.BattleMap only when fightScene is empty.
        self.assertEqual((fight.code, fight.maxround, fight.fightScene), (1, 40, ''))
        return fight

    def end_fields(self, fight, win=True, fraction=1):
        party = message('BattlePlayer')
        party.CopyFrom(fight.battlePlayer)
        for hero in party.formation:
            hero.inst.curHp *= fraction
        return {'fightId': fight.fightId, 'result': int(win), 'battlePlayer': party,
                'optRecord': {'finalRoundNum': 1,
                              'attackerTotalLeftHp': sum(h.inst.curHp for h in party.formation),
                              'defenderTotalLeftHp': 0 if win else 1}}

    def finish(self, login, fields):
        opcode, result = self.send('CSSidePveFightEnd', fields, login)
        self.assertEqual(opcode, BY_NAME['SCSidePveFightEnd'], result)
        return result

    def test_entry_level_gate_real_spawns_and_monster_ownership(self):
        login = self.create('field-gate')
        before = self.state(login)
        for fields in ({'target': 'field_001'}, {'target': 'field_002'}, {'target': ''},
                       {'target': 'field_001', 'point': {'x': float('nan')}}):
            self.assertEqual(self.send('CSTransferScene', fields, login)[1].code, '90003')
            self.assertEqual(self.state(login), before)
        self.mutate(login, lambda p, g: p.update(level=18))
        login = self.login('field-gate')
        self.send('CSTransferScene', {'target': 'field_001'}, login)
        monsters = self.monsters(login)
        self.assertEqual(len(monsters), 23)
        self.assertEqual(len({m.uuid for m in monsters}), 23)
        self.assertEqual(len({m.monsterPosId for m in monsters}), 23)
        self.assertTrue(all(m.state == 1 and m.curHp == 1 for m in monsters))
        known = monsters[0]
        _, checked = self.send('CSSideMonsterCheck', {'uuid': known.uuid}, login)
        self.assertEqual((checked.code, checked.monster.uuid), (1, known.uuid))
        _, missing = self.send('CSSideMonsterCheck', {'uuid': 'foreign'}, login)
        self.assertEqual((missing.code, missing.monster.uuid), (2003, 'foreign'))
        fight = self.begin(login, known)
        with (ROOT/'analysis/tables/TableFieldMonster.tsv').open() as file:
            rows = list(csv.reader(file, delimiter='\t'))
        expected = [dict(zip(rows[1], row)) for row in rows[5:]
                    if len(row) > 1 and row[1] == known.monId]
        actual = [h for group in fight.battleMonsterList for h in group.formation]
        self.assertEqual(len(actual), len(expected))
        self.assertEqual(sorted((h.id, h.prop.hp, h.prop.att) for h in actual),
                         sorted((r['HeroId'], float(r['Hp']), float(r['Attack'])) for r in expected))
        self.assertEqual(self.begin(login, known), fight)
        self.assertEqual(self.send('CSSidePveFightStart', {'monsterId': monsters[1].uuid}, login)[1].code,
                         '90003')
        self.assertEqual(self.send('CSTransferScene', {'target': 'town'}, login)[1].code, '90003')

    def test_settlement_parallel_retry_restart_and_slot_hp_persistence(self):
        login, monsters = self.prepare('field-win')
        fight = self.begin(login, monsters[0])
        self.stop(); self.start(); login = self.login('field-win')
        self.assertEqual(self.begin(login, monsters[0]), fight)
        before = self.profile(login)
        fields = self.end_fields(fight, fraction=0.75)
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.send('CSSidePveFightEnd', fields, login), range(6)))
        for opcode, result in replies:
            self.assertEqual(opcode, BY_NAME['SCSidePveFightEnd'], result)
            self.assertEqual(result, replies[0][1])
        result = replies[0][1]
        self.assertEqual((result.fightResult, result.monster.state, result.monster.curHp), (1, 3, 0))
        coins = sum(s.num for s in result.getn.simps if s.id == 'coin')
        self.assertIn(coins, (100, 200))
        self.assertIn(len(result.getn.wars), (1, 2, 3))
        self.assertTrue(all(w.id == 'hero_exp_001' for w in result.getn.wars))
        self.assertEqual(self.profile(login).coin, before.coin+coins)
        self.assertEqual(self.profile(login).stamina, before.stamina)
        saved = self.state(login)
        self.assertEqual(saved['field']['reward_kills'], 1)
        self.assertAlmostEqual(saved['field']['side']['curHp'], 0.75)
        self.stop(); self.start(); login = self.login('field-win')
        self.assertEqual(self.finish(login, fields), result)
        self.assertEqual(self.state(login)['field']['reward_kills'], 1)
        _, checked = self.send('CSSideMonsterCheck', {'uuid': monsters[0].uuid}, login)
        self.assertEqual(checked.code, 2003)
        next_fight = self.begin(login, monsters[1])
        self.assertAlmostEqual(next_fight.battlePlayer.formation[0].inst.curHp /
                               next_fight.battlePlayer.formation[0].prop.hp, 0.75)

    def test_forged_results_loss_and_death_do_not_award(self):
        login, monsters = self.prepare('field-loss')
        fight = self.begin(login, monsters[0])
        before = self.state(login)
        for fields in ({'fightId': fight.fightId, 'result': 1},
                       self.end_fields(fight, fraction=2),
                       self.end_fields(fight, fraction=float('nan')),
                       {**self.end_fields(fight), 'optRecord': {'finalRoundNum': 41,
                          'attackerTotalLeftHp': 1, 'defenderTotalLeftHp': 0}},
                       {**self.end_fields(fight), 'fightId': 'foreign'}):
            self.assertEqual(self.send('CSSidePveFightEnd', fields, login)[1].code, '90003')
            self.assertEqual(self.state(login), before)
        loss = self.finish(login, self.end_fields(fight, win=False, fraction=0))
        self.assertEqual((loss.fightResult, loss.state, loss.monster.state), (0, 5, 1))
        self.assertFalse(loss.getn.simps or loss.getn.wars)
        self.assertEqual(self.state(login)['field']['reward_kills'], 0)
        self.assertEqual(self.send('CSSidePveFightStart', {'monsterId': monsters[0].uuid}, login)[1].code,
                         '90003')
        opcode, transfer = self.send('CSTransferScene', {'target': 'town_main'}, login)
        self.assertEqual((opcode, transfer.result), (BY_NAME['SCTransferScene'], 1))
        _, position = self.send('CSCurrentPosition', login=login)
        self.assertEqual(position.self.position.scene, 'town_main')

    def test_daily_reward_limit_and_reset_preserve_combat_results(self):
        login, monsters = self.prepare('field-cap')
        self.mutate(login, lambda p, g: g['field'].update(reward_kills=50))
        login = self.login('field-cap')
        before = self.profile(login)
        capped = self.finish(login, self.end_fields(self.begin(login, monsters[0])))
        self.assertEqual((capped.fightResult, capped.monster.state), (1, 3))
        self.assertFalse(capped.getn.simps or capped.getn.wars)
        self.assertEqual(self.profile(login).coin, before.coin)
        self.mutate(login, lambda p, g: g['field'].update(reward_day='2000-01-01'))
        login = self.login('field-cap')
        earned = self.finish(login, self.end_fields(self.begin(login, monsters[1])))
        self.assertTrue(earned.getn.wars)
        self.assertEqual(self.state(login)['field']['reward_kills'], 1)

    def test_respawn_and_expired_fight_release_without_awards(self):
        login, monsters = self.prepare('field-respawn')
        fight = self.begin(login, monsters[0])
        result = self.finish(login, self.end_fields(fight))
        self.mutate(login, lambda p, g: g['field']['respawn_at'].update({monsters[0].uuid: 1}))
        login = self.login('field-respawn')
        self.send('CSPingTcp', {'clientTime': 1}, login)
        pushed = dict(self.last_frames)
        self.assertIn(BY_NAME['SCSideMonster'], pushed)
        refreshed = list(pushed[BY_NAME['SCSideMonster']].monsters)
        replacement = next(m for m in refreshed if m.monsterPosId == monsters[0].monsterPosId)
        self.assertNotEqual(replacement.uuid, monsters[0].uuid)
        self.assertEqual(replacement.state, 1)
        self.assertEqual(self.finish(login, self.end_fields(fight)), result)
        expired = self.begin(login, replacement)
        self.mutate(login, lambda p, g: g['field']['fight'].update(started_at=1))
        login = self.login('field-respawn')
        self.monsters(login)
        self.assertIsNone(self.state(login)['field'].get('fight'))
        self.assertEqual(self.send('CSSidePveFightEnd', self.end_fields(expired), login)[1].code, '90003')
        self.assertEqual(self.state(login)['field']['reward_kills'], 1)
        fresh = self.begin(login, replacement)
        self.assertNotEqual(fresh.fightId, expired.fightId)

    def test_overflow_and_full_bag_roll_back_settlement(self):
        login, monsters = self.prepare('field-overflow')
        fight = self.begin(login, monsters[0])
        self.mutate(login, lambda p, g: p.update(coin=9223372036854775800))
        login = self.login('field-overflow')
        before = self.state(login)
        self.assertEqual(self.send('CSSidePveFightEnd', self.end_fields(fight), login)[1].code, '90003')
        self.assertEqual(self.state(login), before)
        self.mutate(login, lambda p, g: p.update(coin=0, warriorBagSize=len(g['warriors'])))
        login = self.login('field-overflow')
        before = self.state(login)
        self.assertEqual(self.send('CSSidePveFightEnd', self.end_fields(fight), login)[1].code, '90003')
        self.assertEqual(self.state(login), before)
        self.assertEqual(self.profile(login).coin, 0)

    def test_configured_fish_and_mine_drops_persist_in_correct_bags(self):
        # A temporary copy gives rare quantity branches a single weighted outcome.
        self.stop()
        self.tables = Path(self.temp.name)/'tables'
        shutil.copytree(ROOT/'analysis/tables', self.tables)
        path = self.tables/'TableFieldMonster.tsv'
        with path.open() as file:
            rows = list(csv.reader(file, delimiter='\t'))
        reward = rows[1].index('rewardGroupId')
        for row in rows[5:]:
            if row[0]: row[reward] = 'MineItem_007:1,1000|FishItem_007:1,1000|coin:100,1000|hero_exp_001:1,1000'
        with path.open('w') as file:
            csv.writer(file, delimiter='\t', lineterminator='\n').writerows(rows)
        self.start()
        login, monsters = self.prepare('field-materials')
        result = self.finish(login, self.end_fields(self.begin(login, monsters[0])))
        self.assertEqual({s.id:s.num for s in result.getn.simps},
                         {'MineItem_007': 1, 'FishItem_007': 1, 'coin': 100})
        _, bag = self.send('CSGemAndCookingList', login=login)
        self.assertEqual([(s.id,s.num) for s in bag.gemList], [('MineItem_007',1)])
        self.assertEqual([(s.id,s.num) for s in bag.cookingList], [('FishItem_007',1)])
        self.stop(); self.start(); login = self.login('field-materials')
        self.assertEqual(self.send('CSGemAndCookingList', login=login)[1], bag)


if __name__ == '__main__':
    unittest.main()
