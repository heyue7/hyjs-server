"""真实 Go 二进制资源恢复：离线、余数、消费、并发和重启，全部使用隔离库。"""
import concurrent.futures
import json
import os
import time
import unittest

from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests
from test_go_activity import MySQLActivityTests


class GoResourceTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    mutate = GoEquipmentTests.mutate

    def sql(self, query, args=()):
        with self.service.store.connect() as conn:
            return conn.execute(query, args).fetchall()

    def profile(self, login):
        row = self.sql('SELECT player,game FROM profiles WHERE id=?', (login.playerId,))[0]
        return json.loads(row['player']), json.loads(row['game'])

    def create(self, uid):
        login = self.login(uid)
        op, reply = self.send('CSPlayerCreate', {'playerName': '资源回归', 'heroId': 'hero_043'}, login)
        self.assertEqual(op, BY_NAME['SCPlayerCreate'], str(reply))
        return login

    def player(self, login):
        op, reply = self.send('CSPlayer', login=login)
        self.assertEqual(op, BY_NAME['SCPlayer'], str(reply))
        return reply.player

    def test_offline_parallel_remainder_restart_and_nonresource_query(self):
        login = self.create('resource-offline')
        now = int(time.time() * 1000)
        self.mutate(login, lambda p, g: p.update(energy=10, stamina=50,
                    energyLastRefreshTime=now - 2 * 1800000 - 30000,
                    staminaLastRefreshTime=now - 3 * 360000 - 30000))
        with concurrent.futures.ThreadPoolExecutor(max_workers=12) as pool:
            players = list(pool.map(lambda _: self.player(login), range(12)))
        self.assertTrue(all((p.energy, p.stamina) == (12, 53) for p in players))
        saved, _ = self.profile(login)
        self.assertEqual((saved['energyLastRefreshTime'], saved['staminaLastRefreshTime']),
                         (now - 30000, now - 30000))
        self.stop()
        self.start()
        login = self.login('resource-offline')
        self.assertEqual((self.player(login).energy, self.player(login).stamina), (12, 53))
        # 任意普通查询也提交到期资源；不能只在打开资源面板时结算。
        self.mutate(login, lambda p, g: p.update(stamina=40, staminaLastRefreshTime=now - 360000 - 30000))
        self.assertEqual(self.send('CSItemList', login=login)[0], BY_NAME['SCItemList'])
        self.assertEqual(self.profile(login)[0]['stamina'], 41)

    def test_recovery_precedes_real_consumption_and_full_balance_restarts_timer(self):
        login = self.create('resource-consume')
        now = int(time.time() * 1000)
        self.mutate(login, lambda p, g: p.update(stamina=9, staminaLastRefreshTime=now - 360000 - 30000))
        op, reply = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        self.assertEqual(op, BY_NAME['SCCopyEnter'], str(reply))
        after = self.player(login)
        self.assertEqual(after.stamina, 0)
        self.assertEqual(after.staminaLastRefreshTime, now - 30000)
        self.send('CSCopyLeave', login=login)
        old = now - 86400000
        self.mutate(login, lambda p, g: p.update(stamina=100, staminaLastRefreshTime=old))
        before = int(time.time() * 1000)
        self.assertEqual(self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)[0], BY_NAME['SCCopyEnter'])
        after = self.player(login)
        self.assertEqual(after.stamina, 90)
        self.assertGreaterEqual(after.staminaLastRefreshTime, before)
        self.assertLessEqual(after.staminaLastRefreshTime, int(time.time() * 1000))
        # 满值等待时间不能在消费后一次性补回。
        self.assertEqual(self.player(login).stamina, 90)

    def test_legacy_overcap_and_resource_item_keep_real_balance(self):
        login = self.create('resource-legacy')
        self.mutate(login, lambda p, g: (p.update(energy=45, stamina=120,
                    energyLastRefreshTime=0, staminaLastRefreshTime=0),
                    g.update(items=[{'id': 'item_001', 'num': 1}, {'id': 'item_002', 'num': 1}])))
        before = int(time.time() * 1000)
        p = self.player(login)
        self.assertEqual((p.energy, p.stamina), (45, 120))
        self.assertGreaterEqual(p.energyLastRefreshTime, before)
        for key in ('item_001', 'item_002'):
            op, reply = self.send('CSItemUse', {'costItem': {'id': key, 'num': 1}}, login)
            self.assertEqual(op, BY_NAME['SCItemUse'], str(reply))
        self.assertEqual((self.player(login).energy, self.player(login).stamina), (55, 145))
        self.stop()
        self.start()
        login = self.login('resource-legacy')
        self.assertEqual((self.player(login).energy, self.player(login).stamina), (55, 145))


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_RESOURCE_TEST') == '1', '需显式启用隔离 MySQL 资源回归')
class MySQLResourceTests(GoResourceTests):
    setUp = MySQLActivityTests.setUp
    tearDown = MySQLActivityTests.tearDown
    start = MySQLActivityTests.start
    root_sql = MySQLActivityTests.root_sql
    sql = MySQLActivityTests.sql
    mutate = MySQLActivityTests.mutate


del GoEquipmentTests
del MySQLActivityTests
