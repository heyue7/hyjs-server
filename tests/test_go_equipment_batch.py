"""原客户端五次强化报价、角色上限、真实日常计数及数据库事务回归。"""
import concurrent.futures
import os
import unittest

from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests
from test_mysql_equipment import MySQLEquipmentHarness


class GoEquipmentBatchTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    character = GoEquipmentTests.character
    mutate = GoEquipmentTests.mutate
    seed = GoEquipmentTests.seed
    equipment = GoEquipmentTests.equipment
    denied = GoEquipmentTests.denied
    equip = GoEquipmentTests.equip
    battle = GoEquipmentTests.battle

    def batch(self, login, coin, times=5):
        return self.send('CSEquipStrengthen', {'guid': 'equip-1', 'times': times,
                         'cost': [{'id': 'coin', 'num': coin}]}, login)

    def player(self, login):
        op, response = self.send('CSPlayerLogin', login=login)
        self.assertEqual(op, BY_NAME['SCPlayerLogin'], str(response))
        return response.player

    def daily(self, login):
        op, response = self.send('CSTaskList', login=login)
        self.assertEqual(op, BY_NAME['SCTaskList'], str(response))
        return next(t for t in response.taskList if t.taskId == 'local_equip_strengthen')

    def test_five_levels_charge_original_sum_once_and_reach_battle(self):
        login = self.character()
        self.seed(login)
        self.equip(login, 1)
        before_attack = self.battle(login).att
        # APK资质12的1至5级费用为8+18+26+47+74，共173金币。
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.batch(login, 173), range(6)))
        successes = [r for op, r in replies if op == BY_NAME['SCEquipStrengthen']]
        self.assertEqual(len(successes), 1, str(replies))
        r = successes[0]
        self.assertEqual((r.equip.level, r.costTimes, r.criTimes, r.cost[0].num), (6, 5, 0, 173))
        self.assertEqual(self.player(login).coin, 99827)
        self.assertEqual((self.daily(login).count, self.daily(login).state), (3, 0))
        self.assertAlmostEqual(self.battle(login).att - before_attack, 7.5)
        self.stop(); self.start(); login = self.login('equipment-test')
        self.assertEqual(self.equipment(login)['equip-1'].level, 6)
        self.assertEqual(self.player(login).coin, 99827)
        self.assertEqual(self.daily(login).count, 3)
        self.assertEqual(self.batch(login, 173)[0], 10001)

    def test_original_over_limit_quote_charges_only_two_actual_levels(self):
        login = self.character()
        self.seed(login, level=3, equip_level=5)
        # 原按钮因Mathf.Max报5至10级费用727；角色3级最多升至ceil(6.6)=7级。
        op, r = self.batch(login, 727)
        self.assertEqual(op, BY_NAME['SCEquipStrengthen'], str(r))
        self.assertEqual((r.equip.level, r.costTimes, r.cost[0].num), (7, 2, 180))
        self.assertEqual(self.player(login).coin, 99820)
        self.assertEqual((self.daily(login).count, self.daily(login).state), (2, 1))
        self.assertEqual(self.batch(login, 727)[0], 10001)
        self.stop(); self.start(); login = self.login('equipment-test')
        self.assertEqual((self.equipment(login)['equip-1'].level, self.player(login).coin), (7, 99820))
        self.assertEqual(self.daily(login).count, 2)

    def test_bad_quote_count_and_insufficient_balance_roll_back_entire_batch(self):
        login = self.character()
        self.seed(login, coin=172)
        self.daily(login)
        for cost, times in [(173, 5), (172, 5), (0, 0), (279, 6)]:
            self.assertEqual(self.batch(login, cost, times)[0], 10001)
            self.assertEqual((self.equipment(login)['equip-1'].level, self.player(login).coin), (1, 172))
            self.assertEqual(self.daily(login).count, 0)
        # 可提升的两级余额不足时也不能部分升级或部分推进日常。
        self.seed(login, level=3, equip_level=5, coin=179)
        self.assertEqual(self.batch(login, 727)[0], 10001)
        self.assertEqual((self.equipment(login)['equip-1'].level, self.player(login).coin), (5, 179))
        self.assertEqual(self.daily(login).count, 0)


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_EQUIPMENT_TEST') == '1', '需显式启用独立MySQL装备回归')
class MySQLEquipmentBatchTests(MySQLEquipmentHarness, GoEquipmentBatchTests):
    pass


del GoEquipmentTests, MySQLEquipmentHarness
