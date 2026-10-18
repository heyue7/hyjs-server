"""每月签到：真实Go二进制协议、隔离SQL热配置、并发和重启。"""
import concurrent.futures
import json
import os
import unittest
from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests
from test_go_activity import MySQLActivityTests

class GoMonthSignTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    mutate = GoEquipmentTests.mutate

    def sql(self, query, args=()):
        with self.service.store.connect() as c:
            return c.execute(query, args).fetchall()

    def info(self, login):
        op, reply = self.send('CSRewardMonthSignList', login=login)
        self.assertEqual(op, BY_NAME['SCRewardMonthSignList'], str(reply))
        return reply

    def coin(self, login):
        return json.loads(self.sql('SELECT player FROM profiles WHERE id=?', (login.playerId,))[0]['player'])['coin']

    def test_claim_vip_difference_concurrency_restart_sql_and_luxury(self):
        login = self.login('isolated-month-sign')
        self.send('CSPlayerCreate', {'playerName': '签到回归', 'heroId': 'hero_043'}, login)
        self.mutate(login, lambda p,g: p.update(coin=0, vip=0))
        initial = self.info(login)
        self.assertEqual((initial.currentDay, initial.list[0].getState, initial.luxuryState), (1,1,0))
        self.sql("UPDATE month_sign_rules SET reward='coin,123',double_vip=2 WHERE day=1")
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.send('CSReceiveMonthSign', login=login), range(8)))
        self.assertEqual(sum(op == BY_NAME['SCReceiveMonthSign'] for op,r in replies), 1)
        self.assertEqual(self.coin(login), 123)
        self.assertEqual(self.info(login).list[0].getState, 2)
        self.sql("UPDATE month_sign_rules SET reward='coin,99999' WHERE day=1")
        self.mutate(login, lambda p,g: p.update(vip=2))
        op, reply = self.send('CSReceiveMonthSign', login=login)
        self.assertEqual(op, BY_NAME['SCReceiveMonthSign'], str(reply))
        self.assertEqual(self.coin(login), 246)
        self.assertEqual(self.info(login).list[0].getState, 3)
        self.assertEqual(self.send('CSReceiveMonthSign', login=login)[0], 10001)
        self.assertEqual(self.send('CSReceiveLuxurySign', login=login)[0], 10001)
        self.stop(); self.start(); login = self.login('isolated-month-sign')
        self.assertEqual(self.coin(login), 246)
        self.assertEqual(self.info(login).list[0].getState, 3)
        self.assertEqual(self.send('CSReceiveMonthSign', login=login)[0], 10001)
        self.assertEqual(self.sql('SELECT reward FROM month_sign_rules WHERE day=1')[0]['reward'], 'coin,99999')
        self.sql("UPDATE month_sign_rules SET reward='unsupported,1' WHERE day=2")
        self.assertEqual(self.send('CSRewardMonthSignList', login=login)[0], 10001)
        self.assertEqual(self.coin(login), 246)


    def test_resource_overflow_rolls_back_profile_and_claim(self):
        # 每个账号仅存在于本测试创建的临时库，覆盖双倍第二段溢出的整笔回滚。
        cases = [
            ('coin', 'coin,123', {'coin': 2**63-101, 'vip': 0}, None),
            ('diamond', 'diamond,123', {'diamond': 2**63-101, 'vip': 0}, None),
            ('double', 'coin,1', {'coin': 2**63-2, 'vip': 2}, None),
            ('item', 'item_001,2', {'vip': 0}, [{'id': 'item_001', 'num': 2**31-2}]),
        ]
        for name, reward, values, items in cases:
            with self.subTest(resource=name):
                login = self.login('isolated-sign-overflow-' + name)
                self.send('CSPlayerCreate', {'playerName': name, 'heroId': 'hero_043'}, login)
                def fixture(player, game):
                    player.update(values)
                    if items is not None:
                        game['items'] = items
                self.mutate(login, fixture)
                self.sql('UPDATE month_sign_rules SET reward=?,double_vip=2 WHERE day=1', (reward,))
                before = self.sql('SELECT player,game FROM profiles WHERE id=?', (login.playerId,))[0]
                op, reply = self.send('CSReceiveMonthSign', login=login)
                self.assertEqual(op, 10001, str(reply))
                after = self.sql('SELECT player,game FROM profiles WHERE id=?', (login.playerId,))[0]
                self.assertEqual(dict(after), dict(before))
                self.assertEqual(len(self.sql('SELECT * FROM month_sign_states WHERE player_id=?', (login.playerId,))), 0)
                self.assertEqual(self.info(login).list[0].getState, 1)

del GoEquipmentTests

@unittest.skipUnless(os.environ.get('HYJS_MYSQL_ACTIVITY_TEST') == '1', '需显式启用独立MySQL签到回归')
class MySQLMonthSignTests(GoMonthSignTests):
    """沿用专属hyjs_test_activity_*建库清库保护，复用原协议签到断言。"""
    setUp = MySQLActivityTests.setUp
    tearDown = MySQLActivityTests.tearDown
    start = MySQLActivityTests.start
    root_sql = MySQLActivityTests.root_sql
    sql = MySQLActivityTests.sql
    mutate = MySQLActivityTests.mutate

    def test_mysql_schema_uses_innodb_and_explicit_columns(self):
        rows = self.sql("SELECT TABLE_NAME,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME IN ('month_sign_rules','month_sign_states')", (self.database,))
        self.assertEqual({r['TABLE_NAME']:r['ENGINE'] for r in rows}, {'month_sign_rules':'InnoDB','month_sign_states':'InnoDB'})
        columns = self.sql("SELECT COLUMN_NAME,DATA_TYPE,CHARACTER_MAXIMUM_LENGTH FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME='month_sign_states'", (self.database,))
        found = {r['COLUMN_NAME']:r for r in columns}
        self.assertEqual(found['month']['CHARACTER_MAXIMUM_LENGTH'], '7')
        self.assertEqual(found['last_day']['CHARACTER_MAXIMUM_LENGTH'], '10')
        self.assertEqual(found['last_double_vip']['DATA_TYPE'], 'int')
        self.assertEqual(found['player_id']['DATA_TYPE'], 'bigint')
        keys = self.sql("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA=? AND TABLE_NAME='month_sign_states' AND CONSTRAINT_NAME='PRIMARY' ORDER BY ORDINAL_POSITION", (self.database,))
        self.assertEqual([r['COLUMN_NAME'] for r in keys], ['player_id','month'])
        rules = self.sql("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME='month_sign_rules'", (self.database,))
        self.assertIn('double_vip', [r['COLUMN_NAME'] for r in rules])

    def profile(self, login):
        row = self.sql('SELECT player,game FROM profiles WHERE id=?', (login.playerId,))[0]
        return json.loads(row['player']), json.loads(row['game'])

del MySQLActivityTests
