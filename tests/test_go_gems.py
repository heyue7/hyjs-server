"""宝石闭环：原 APK 二进制帧、隔离 SQLite/MySQL、真实进程重启和战斗属性。"""
import concurrent.futures
import json
import os
import unittest

from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests
from test_go_formation import GoFormationTests
from test_go_activity import GoActivityTests, MySQLActivityTests


class GoGemTests(unittest.TestCase):
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

    def fixture(self, uid='gems', level=60):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '宝石回归', 'heroId': 'hero_043'}, login)

        def seed(player, game):
            player.update(level=level, coin=10000000)
            game['equipment'] = [{'guid': 'gem-equip', 'equipId': 'equip_037', 'level': 1, 'gems': []}]
            game['gems'] = [{'id': key, 'num': 2000} for key in (
                'MineItem_007', 'MineItem_013', 'MineItem_014', 'MineItem_015',
                'MineItem_016', 'MineItem_017', 'MineItem_018', 'MineItem_081', 'MineItem_084')]

        self.mutate(login, seed)
        opcode, reply = self.send('CSFormationOn', {'id': 'equip_037', 'guid': 'gem-equip', 'formatPos': 1}, login)
        self.assertEqual(opcode, BY_NAME['SCFormationOn'], str(reply))
        return login

    def success(self, name, fields, login, stamp):
        opcode, reply = self.stamped(name, fields, login, stamp)
        self.assertEqual(opcode, BY_NAME['SC' + name[2:]], str(reply))
        return reply

    def denied(self, name, fields, login, stamp):
        # 同时验证失败请求没有改变金币、材料、装备及经验，也没有留下成功回执。
        before = self.profile(login)
        states = [dict(row) for row in self.sql('SELECT * FROM gem_craft_states WHERE player_id=?', (login.playerId,))]
        receipts = [dict(row) for row in self.sql('SELECT opcode,timestamp FROM gem_mutation_receipts WHERE player_id=? ORDER BY opcode', (login.playerId,))]
        opcode, reply = self.stamped(name, fields, login, stamp)
        self.assertEqual((opcode, reply.code), (10001, '90003'), str(reply))
        self.assertEqual(self.profile(login), before)
        self.assertEqual([dict(row) for row in self.sql('SELECT * FROM gem_craft_states WHERE player_id=?', (login.playerId,))], states)
        self.assertEqual([dict(row) for row in self.sql('SELECT opcode,timestamp FROM gem_mutation_receipts WHERE player_id=? ORDER BY opcode', (login.playerId,))], receipts)

    def gems(self, login):
        opcode, reply = self.send('CSGemAndCookingList', login=login)
        self.assertEqual(opcode, BY_NAME['SCGemAndCookingList'], str(reply))
        return {item.id: item.num for item in reply.gemList}

    def test_make_mount_replace_unmount_battle_and_restart(self):
        login = self.fixture()
        baseline = self.battle(login)
        craft = self.success('CSGemMake', {'gemId': 'MineItem_019', 'num': 20}, login, 100)
        self.assertEqual((craft.level, craft.exp, craft.add), (2, 0, 1000))
        self.assertEqual({cost.id: cost.num for cost in craft.cost},
                         {'coin': 200000, 'MineItem_081': 200, 'MineItem_013': 40, 'MineItem_084': 200})
        self.assertEqual(self.profile(login)[0]['coin'], 9800000)
        self.assertEqual(self.gems(login)['MineItem_019'], 20)
        mounted = self.success('CSGemMount', {'guid': 'gem-equip', 'gemId': 'MineItem_019', 'pos': 0}, login, 110)
        self.assertEqual(list(mounted.equip.gems), ['MineItem_019', '', '', ''])
        self.assertAlmostEqual(self.battle(login).att - baseline.att, 200)
        self.success('CSGemMake', {'gemId': 'MineItem_020', 'num': 1}, login, 101)
        fields = {'guid': 'gem-equip', 'gemId': 'MineItem_020'}
        changed = self.success('CSGemMountOrChange', fields, login, 120)
        self.assertEqual(changed.equip.gems[0], 'MineItem_020')
        self.assertEqual((changed.res.simps[0].id, changed.res.simps[0].num), ('MineItem_019', 1))
        self.assertAlmostEqual(self.battle(login).att - baseline.att, 400)
        self.stop()
        self.start()
        login = self.login('gems')
        self.assertEqual(self.success('CSGemMountOrChange', fields, login, 120), changed)
        self.assertEqual(self.gems(login)['MineItem_019'], 20)
        opcode, levels = self.send('CSCookingLevel', login=login)
        self.assertEqual(opcode, BY_NAME['SCCookingLevel'], str(levels))
        self.assertEqual((levels.gemLevel, levels.gemExp), (2, 100))
        self.success('CSGemUnmount', {'guid': 'gem-equip', 'pos': 0}, login, 130)
        self.assertAlmostEqual(self.battle(login).att, baseline.att)
        self.assertEqual(self.gems(login)['MineItem_020'], 1)

    def test_atomic_concurrency_replay_and_validation(self):
        login = self.fixture()
        fields = {'gemId': 'MineItem_019', 'num': 1}
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.stamped('CSGemMake', fields, login, 100), range(6)))
        self.assertTrue(all(opcode == BY_NAME['SCGemMake'] for opcode, _ in replies), str(replies))
        self.assertTrue(all(reply == replies[0][1] for _, reply in replies))
        self.assertEqual(self.profile(login)[0]['coin'], 9990000)
        self.assertEqual(self.gems(login)['MineItem_019'], 1)
        self.denied('CSGemMake', {'gemId': 'MineItem_019', 'num': 2}, login, 100)
        self.denied('CSGemMake', fields, login, 99)
        for invalid in ({'gemId': 'MineItem_019', 'num': -1}, {'gemId': 'MineItem_019', 'num': 1001},
                        {'gemId': 'MineItem_020', 'num': 1}, {'gemId': 'foreign', 'num': 1},
                        {'gemId': 'MineItem_079', 'num': 1}):
            self.denied('CSGemMake', invalid, login, 101)
        for invalid in ({'guid': 'foreign', 'gemId': 'MineItem_019', 'pos': 0},
                        {'guid': 'gem-equip', 'gemId': 'MineItem_019', 'pos': 1},
                        {'guid': 'gem-equip', 'gemId': 'MineItem_013', 'pos': 0},
                        {'guid': 'gem-equip', 'gemId': 'MineItem_019', 'pos': -1}):
            self.denied('CSGemMount', invalid, login, 102)
        # 仅剩一份制作材料，两个新请求最多有一个事务可以成功。
        self.mutate(login, lambda p, g: next(item for item in g['gems'] if item['id'] == 'MineItem_081').update(num=10))
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            replies = list(pool.map(lambda stamp: self.stamped('CSGemMake', fields, login, stamp), (103, 104)))
        self.assertEqual(sum(opcode == BY_NAME['SCGemMake'] for opcode, _ in replies), 1)
        self.assertEqual(self.gems(login)['MineItem_019'], 2)
        self.assertEqual(self.profile(login)[0]['coin'], 9980000)

    def test_refine_random_receipt_sql_rule_and_restart(self):
        login = self.fixture()
        fields = {'mineId': 'MineItem_007', 'num': 100}
        refined = self.success('CSMineRefine', fields, login, 100)
        self.assertEqual((refined.level, refined.exp, refined.add), (2, 0, 1000))
        rewards = {item.id: item.num for item in refined.res.simps}
        self.assertEqual(sum(rewards.get(key, 0) for key in ('MineItem_081', 'MineItem_082')), 20)
        self.assertIn(rewards['MineItem_084'], range(20, 41))
        self.assertEqual(sum(rewards.get('MineItem_%03d' % index, 0) for index in range(13, 19)), 20)
        self.assertEqual(self.gems(login)['MineItem_007'], 1900)
        self.sql('UPDATE gem_rules SET crystal_per_batch=0 WHERE id=1')
        self.stop()
        self.start()
        login = self.login('gems')
        self.assertEqual(self.success('CSMineRefine', fields, login, 100), refined)
        self.assertEqual(self.gems(login)['MineItem_007'], 1900)
        self.assertEqual(int(self.sql('SELECT crystal_per_batch FROM gem_rules WHERE id=1')[0]['crystal_per_batch']), 0)
        second = self.success('CSMineRefine', {'mineId': 'MineItem_007', 'num': 5}, login, 101)
        self.assertFalse(any(item.id in {'MineItem_%03d' % i for i in range(13, 19)} for item in second.res.simps))
        self.denied('CSMineRefine', {'mineId': 'MineItem_007', 'num': 6}, login, 102)
        self.denied('CSMineRefine', {'mineId': 'MineItem_019', 'num': 5}, login, 102)
        self.sql('UPDATE gem_rules SET enabled=0 WHERE id=1')
        self.denied('CSMineRefine', {'mineId': 'MineItem_007', 'num': 5}, login, 103)

    def test_combine_transform_and_colorful_six_attributes(self):
        login = self.fixture()
        self.sql('INSERT INTO gem_craft_states(player_id,level,exp) VALUES(?,10,0)', (login.playerId,))
        self.mutate(login, lambda p, g: g['gems'].extend(
            {'id': key, 'num': count} for key, count in {'MineItem_019': 4, 'MineItem_029': 1, 'MineItem_079': 6}.items()))
        combined = self.success('CSGemCombineByThree', {'gemId': 'MineItem_020'}, login, 100)
        self.assertEqual((combined.cost[0].id, combined.cost[0].num), ('MineItem_019', 3))
        self.success('CSGemCombineByTwo', {'gemId': ['MineItem_019', 'MineItem_029', 'MineItem_039']}, login, 110)
        self.denied('CSGemCombineByTwo', {'gemId': ['MineItem_020', 'MineItem_020', 'MineItem_040']}, login, 111)
        self.denied('CSGemCombineByThree', {'gemId': 'MineItem_080'}, login, 101)
        self.success('CSGemCombineBySix', {'gemId': 'MineItem_080'}, login, 120)
        self.denied('CSGemCombineBySix', {'gemId': 'MineItem_020'}, login, 121)
        gems = self.gems(login)
        self.assertEqual({key: gems.get(key, 0) for key in ('MineItem_019', 'MineItem_020', 'MineItem_039', 'MineItem_079', 'MineItem_080')},
                         {'MineItem_019': 0, 'MineItem_020': 1, 'MineItem_039': 1, 'MineItem_079': 0, 'MineItem_080': 1})
        baseline = self.battle(login)
        self.success('CSGemMountOrChange', {'guid': 'gem-equip', 'gemId': 'MineItem_080'}, login, 130)
        mounted = self.battle(login)
        for field, gain in {'att': 1200, 'hp': 4560, 'pdef': 360, 'mdef': 360, 'cri': 1.8, 'antiCri': 1.8}.items():
            self.assertAlmostEqual(getattr(mounted, field) - getattr(baseline, field), gain)


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_ACTIVITY_TEST') == '1', '需显式启用独立 MySQL 宝石回归')
class MySQLGemTests(GoGemTests):
    setUp = MySQLActivityTests.setUp
    tearDown = MySQLActivityTests.tearDown
    start = MySQLActivityTests.start
    root_sql = MySQLActivityTests.root_sql
    sql = MySQLActivityTests.sql
    mutate = MySQLActivityTests.mutate


# 避免 unittest 把作为测试夹具导入的类重复执行。
del GoEquipmentTests, GoFormationTests, GoActivityTests, MySQLActivityTests
