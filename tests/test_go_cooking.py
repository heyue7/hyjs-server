"""料理：原帧制作/角色永久属性/全局时效和真实副本结算，全部存档在隔离数据库。"""
import concurrent.futures
import json
import math
import os
import time
import unittest
from pathlib import Path

from server.codec import BY_NAME
from test_go_activity import GoActivityTests, MySQLActivityTests
from test_go_equipment import GoEquipmentTests
from test_go_formation import GoFormationTests


class GoCookingTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    stamped = GoFormationTests.stamped
    login = GoEquipmentTests.login
    mutate = GoEquipmentTests.mutate
    sql = GoActivityTests.sql
    profile = GoActivityTests.profile
    battle = GoEquipmentTests.battle

    def fixture(self, uid='cooking', level=60):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '料理回归', 'heroId': 'hero_043'}, login)

        def seed(player, game):
            player.update(level=level, coin=10000000, stamina=1000)
            game['cooking'] = [{'id': 'FishItem_%03d' % i, 'num': 2000} for i in range(7, 26)]
        self.mutate(login, seed)
        return login

    def success(self, name, fields, login, stamp):
        opcode, reply = self.stamped(name, fields, login, stamp)
        self.assertEqual(opcode, BY_NAME['SC' + name[2:]], str(reply))
        return reply

    def craft(self, login, item='FishItem_029', num=1, stamp=100):
        return self.success('CSFishCooking', {'fish': {'id': item, 'num': num}}, login, stamp)

    def use(self, login, item, stamp=200):
        hero = self.profile(login)[0]['heroWarId']
        return self.success('CSFishUsing', {'costItem': {'id': item, 'num': 1}, 'heroGuid': hero}, login, stamp)

    def inventory(self, login):
        opcode, reply = self.send('CSGemAndCookingList', login=login)
        self.assertEqual(opcode, BY_NAME['SCGemAndCookingList'], str(reply))
        return {item.id: item.num for item in reply.cookingList}

    def effects(self, login):
        opcode, reply = self.send('CSCookingEffectList', login=login)
        self.assertEqual(opcode, BY_NAME['SCCookingEffectList'], str(reply))
        return list(reply.effectList)

    def denied(self, name, fields, login, stamp):
        before = self.profile(login)
        opcode, reply = self.stamped(name, fields, login, stamp)
        self.assertEqual((opcode, reply.code), (10001, '90003'), str(reply))
        self.assertEqual(self.profile(login), before)

    def test_craft_permanent_battle_replay_sql_rule_and_restart(self):
        login = self.fixture()
        baseline = self.battle(login)
        crafted = self.craft(login, num=24)
        self.assertEqual((crafted.level, crafted.exp, crafted.add), (2, 0, 600))
        self.assertEqual({item.id: item.num for item in crafted.cost},
                         {'coin': 480000, 'FishItem_007': 120, 'FishItem_013': 120, 'FishItem_019': 120})
        self.assertEqual(self.profile(login)[0]['coin'], 9520000)
        consumed = self.use(login, 'FishItem_029')
        self.assertEqual(consumed.warrior.cooking.prop.att, 1)
        self.assertAlmostEqual(self.battle(login).att - baseline.att, 1)
        self.assertEqual(self.use(login, 'FishItem_029'), consumed)
        self.assertEqual(self.inventory(login)['FishItem_029'], 23)
        self.sql('UPDATE cooking_rules SET gains=? WHERE id=1', (json.dumps({'att': 3}),))
        self.stop()
        self.start()
        login = self.login('cooking')
        self.assertEqual(self.use(login, 'FishItem_029'), consumed)
        self.assertEqual(self.use(login, 'FishItem_029', stamp=201).warrior.cooking.prop.att, 4)
        self.assertAlmostEqual(self.battle(login).att - baseline.att, 4)
        opcode, levels = self.send('CSCookingLevel', login=login)
        self.assertEqual(opcode, BY_NAME['SCCookingLevel'], str(levels))
        self.assertEqual((levels.level, levels.exp, levels.gemLevel, levels.gemExp), (2, 0, 1, 0))

    def test_concurrent_craft_and_invalid_use_roll_back(self):
        login = self.fixture()
        fields = {'fish': {'id': 'FishItem_029', 'num': 1}}
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
            replies = list(pool.map(lambda _: self.stamped('CSFishCooking', fields, login, 100), range(5)))
        self.assertTrue(all(opcode == BY_NAME['SCFishCooking'] for opcode, _ in replies), str(replies))
        self.assertTrue(all(reply == replies[0][1] for _, reply in replies))
        self.assertEqual(self.inventory(login)['FishItem_029'], 1)
        self.assertEqual(self.profile(login)[0]['coin'], 9980000)
        self.denied('CSFishCooking', {'fish': {'id': 'FishItem_029', 'num': 2}}, login, 100)
        for fish in ({'id': 'FishItem_029', 'num': -1}, {'id': 'FishItem_029', 'num': 1001},
                     {'id': 'FishItem_030', 'num': 1}, {'id': 'FishItem_050', 'num': 1},
                     {'id': 'FishItem_007', 'num': 1}, {'id': 'FishItem_029', 'num': 1, 'guid': 'foreign'}):
            self.denied('CSFishCooking', {'fish': fish}, login, 101)
        hero = self.profile(login)[0]['heroWarId']
        for cost, guid in (({'id': 'FishItem_029', 'num': 1}, 'foreign'),
                           ({'id': 'FishItem_029', 'num': 2}, hero),
                           ({'id': 'FishItem_029', 'num': 1, 'guid': 'foreign'}, hero),
                           ({'id': 'FishItem_007', 'num': 1}, hero)):
            self.denied('CSFishUsing', {'costItem': cost, 'heroGuid': guid}, login, 200)
        self.craft(login, item='FishItem_026', stamp=102)
        self.denied('CSFishUsing', {'costItem': {'id': 'FishItem_026', 'num': 1}, 'heroGuid': hero}, login, 201)
        self.sql('UPDATE cooking_rules SET gains=? WHERE id=1', ('{"att":-1}',))
        self.denied('CSFishUsing', {'costItem': {'id': 'FishItem_029', 'num': 1}, 'heroGuid': hero}, login, 202)
        self.assertEqual(len(self.sql('SELECT * FROM cooking_mutation_receipts WHERE player_id=? AND opcode=24715', (login.playerId,))), 0)
        self.mutate(login, lambda p, g: p.update(level=17))
        self.denied('CSFishCooking', fields, login, 103)

    def timed_fixture(self):
        login = self.fixture()
        self.sql('INSERT INTO cooking_craft_states(player_id,level,exp) VALUES(?,3,0)', (login.playerId,))
        self.craft(login, item='FishItem_037', num=3)
        before = int(time.time() * 1000)
        self.use(login, 'FishItem_037')
        effect = self.effects(login)[0]
        self.assertEqual(effect.effectId, 'extraCoinExp')
        self.assertAlmostEqual(effect.effectValue, 0.075)
        self.assertGreaterEqual(effect.effectTime, before + 30 * 60000)
        self.assertLessEqual(effect.effectTime, int(time.time() * 1000) + 30 * 60000)
        return login

    def reward_at_level(self, level):
        path = Path(__file__).resolve().parents[1] / 'analysis/tables/TableCopyExp.tsv'
        lines = path.read_text().splitlines()
        for line in lines[5:]:
            row = dict(zip(lines[1].split('\t'), line.split('\t')))
            if row.get('level') == str(level):
                return int(row['coin']), int(row['exp'])
        self.fail('Missing original reward row')

    def test_real_copy_victory_failure_retry_and_expiry(self):
        login = self.timed_fixture()
        # 原协议完整副本战斗依次结算任务 1/4，胜利仅最终任务加金币/经验。
        opcode, entry = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyEnter'], str(entry))
        opcode, fight = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyFightStart'], str(fight))
        end = {'fightId': fight.fightId, 'result': 0, 'battlePlayer': fight.battlePlayer}
        before = self.profile(login)[0]['coin']
        opcode, failed = self.send('CSCopyFightEnd', end, login)
        self.assertEqual(opcode, BY_NAME['SCCopyFightEnd'], str(failed))
        self.assertEqual(self.profile(login)[0]['coin'], before)
        self.assertEqual(self.send('CSCopyFightEnd', end, login)[1], failed)
        self.mutate(login, lambda p, g: g.pop('run', None))
        self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        for task in ('1', '4'):
            opcode, fight = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': task}, login)
            self.assertEqual(opcode, BY_NAME['SCCopyFightStart'], str(fight))
            end = {'fightId': fight.fightId, 'result': 1, 'battlePlayer': fight.battlePlayer}
            opcode, won = self.send('CSCopyFightEnd', end, login)
            self.assertEqual(opcode, BY_NAME['SCCopyFightEnd'], str(won))
        rewards = {item.id: item.num for item in won.gain.simps if item.id in ('coin', 'exp')}
        coin, exp = self.reward_at_level(60)
        self.assertEqual(rewards, {'coin': math.floor(coin * 1.075), 'exp': math.floor(exp * 1.075)})
        before = self.profile(login)
        self.assertEqual(self.send('CSCopyFightEnd', end, login)[1], won)
        self.assertEqual(self.profile(login), before)
        self.sql('UPDATE cooking_effects SET end_at=? WHERE player_id=?', (int(time.time() * 1000) - 1, login.playerId))
        self.assertEqual(self.effects(login), [])

    def test_timed_replacement_sweep_duration_and_replay_after_restart(self):
        login = self.timed_fixture()
        self.craft(login, item='FishItem_036', stamp=101)
        self.use(login, 'FishItem_036', stamp=201)
        effect = self.effects(login)[0]
        self.assertEqual(effect.effectId, 'extraExp')
        self.assertAlmostEqual(effect.effectValue, 0.05)
        self.send('CSCopyPassedInfo', login=login)

        def sweep_ready(player, game):
            game.pop('run', None)
            next(info for info in game['copy']['passedCopyInfo'] if info['missionId'] == '1').update(star=5, challengeTimes=0)
        self.mutate(login, sweep_ready)
        fields = {'chapterId': '1', 'missionId': '1'}
        swept = self.success('CSCopySweep', fields, login, 300)
        coin, exp = self.reward_at_level(60)
        self.assertEqual({item.id: item.num for item in swept.gain.simps}, {'coin': coin, 'exp': math.floor(exp * 1.05)})
        self.assertEqual(self.effects(login)[0].effectTime, effect.effectTime - 3 * 60000)
        self.stop()
        self.start()
        login = self.login('cooking')
        before = self.profile(login)
        self.assertEqual(self.success('CSCopySweep', fields, login, 300), swept)
        self.assertEqual(self.profile(login), before)
        self.assertEqual(self.effects(login)[0].effectTime, effect.effectTime - 3 * 60000)


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_ACTIVITY_TEST') == '1', '需显式启用独立 MySQL 料理回归')
class MySQLCookingTests(GoCookingTests):
    setUp = MySQLActivityTests.setUp
    tearDown = MySQLActivityTests.tearDown
    start = MySQLActivityTests.start
    root_sql = MySQLActivityTests.root_sql
    sql = MySQLActivityTests.sql
    mutate = MySQLActivityTests.mutate


del GoEquipmentTests, GoFormationTests, GoActivityTests, MySQLActivityTests
