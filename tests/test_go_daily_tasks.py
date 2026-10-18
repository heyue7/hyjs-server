"""日常任务：真实 Go 进程、原 APK 协议、隔离存档与并发领取。"""
import concurrent.futures
import json
import os
import unittest

from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests
from test_go_formation import GoFormationTests
from test_go_activity import MySQLActivityTests


class GoDailyTaskTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    mutate = GoEquipmentTests.mutate
    seed = GoEquipmentTests.seed
    stamped = GoFormationTests.stamped

    def sql(self, query, args=()):
        with self.service.store.connect() as conn:
            return conn.execute(query, args).fetchall()

    def profile(self, login):
        row = self.sql('SELECT player,game FROM profiles WHERE id=?', (login.playerId,))[0]
        return json.loads(row['player']), json.loads(row['game'])

    def info(self, login):
        op, reply = self.send('CSTaskList', login=login)
        self.assertEqual(op, BY_NAME['SCTaskList'], str(reply))
        return reply

    def task(self, login, task_id):
        return next(t for t in self.info(login).taskList if t.taskId == task_id)

    def player(self, login):
        return self.profile(login)[0]

    def create(self, uid):
        login = self.login(uid)
        self.assertEqual(self.send('CSPlayerCreate', {'playerName': '日常验收', 'heroId': 'hero_043'}, login)[0],
                         BY_NAME['SCPlayerCreate'])
        return login

    def test_real_events_claim_box_parallel_restart_and_frozen_rewards(self):
        login = self.create('isolated-daily-wire')
        initial = self.info(login)
        self.assertEqual((len(initial.taskList), len(initial.boxList), initial.score), (7, 4, 0))
        self.assertTrue(all(t.count == 0 and t.state == 1 for t in initial.taskList))
        self.assertEqual(self.send('CSGetTaskReward', {'taskId': 'local_recruit'}, login)[0], 10001)
        self.assertEqual(self.send('CSGetTaskBoxReward', {'boxId': 'box_001'}, login)[0], 10001)
        self.seed(login, level=10, coin=100000)
        self.mutate(login, lambda p, g: g.update(items=[{'id': 'item_016', 'num': 10}]))
        fields = {'costType': 1, 'numType': 2, 'free': 0}
        # 招募十连及同包并发重放只能增加三次封顶进度。
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.stamped('CSWarriorRecruit', fields, login, 1700000000001), range(6)))
        self.assertTrue(all(op == BY_NAME['SCWarriorRecruit'] for op, _ in replies), str(replies))
        self.assertEqual((self.task(login, 'local_recruit').count, self.task(login, 'local_recruit').state), (3, 0))
        self.assertEqual(self.task(login, 'local_equip_strengthen').count, 0)
        # 强化实际三级；非法费用拒绝时保持日常进度。
        self.assertEqual(self.send('CSEquipStrengthen', {'guid': 'equip-1', 'times': 1, 'cost': [{'id': 'coin', 'num': 1}]}, login)[0], 10001)
        for cost in (8, 18, 26):
            op, reply = self.send('CSEquipStrengthen', {'guid': 'equip-1', 'times': 1,
                                  'cost': [{'id': 'coin', 'num': cost}]}, login)
            self.assertEqual(op, BY_NAME['SCEquipStrengthen'], str(reply))
        self.assertEqual((self.task(login, 'local_equip_strengthen').count,
                          self.task(login, 'local_equip_strengthen').state), (3, 0))
        self.sql("UPDATE daily_task_rules SET reward='coin,999999',score=999 WHERE id='local_recruit'")
        before_coin = self.player(login)['coin']
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            rewards = list(pool.map(lambda _: self.send('CSGetTaskReward', {'taskId': 'local_recruit'}, login), range(8)))
        self.assertEqual(sum(op == BY_NAME['SCGetTaskReward'] for op, _ in rewards), 1)
        self.assertEqual(self.player(login)['coin'], before_coin + 1000)
        self.assertEqual(self.info(login).score, 20)
        self.assertEqual(self.send('CSGetTaskReward', {'taskId': 'local_equip_strengthen'}, login)[0], BY_NAME['SCGetTaskReward'])
        self.assertEqual(self.info(login).score, 40)
        before = self.player(login)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            boxes = list(pool.map(lambda _: self.send('CSGetTaskBoxReward', {'boxId': 'box_001'}, login), range(8)))
        self.assertEqual(sum(op == BY_NAME['SCGetTaskBoxReward'] for op, _ in boxes), 1)
        self.assertEqual((self.player(login)['coin'], self.player(login)['diamond']),
                         (before['coin'] + 20000, before['diamond'] + 20))
        self.assertEqual(self.send('CSGetTaskBoxReward', {'boxId': 'box_002'}, login)[0], 10001)
        self.assertEqual(self.send('CSGetTaskBoxReward', {'boxId': 'box_005'}, login)[0], 10001)
        self.stop(); self.start(); login = self.login('isolated-daily-wire')
        restored = self.info(login)
        self.assertEqual((restored.score, restored.boxList[0].state), (40, 2))
        self.assertEqual(self.task(login, 'local_recruit').score, 20)
        self.assertEqual(self.send('CSGetTaskReward', {'taskId': 'local_recruit'}, login)[0], 10001)
        self.assertEqual(self.send('CSGetTaskBoxReward', {'boxId': 'box_001'}, login)[0], 10001)

    def test_reward_overflow_keeps_unclaimed_receipt_and_profile(self):
        login = self.create('isolated-daily-overflow')
        # 将单个本地规则目标设为一次，真实招募完成目标后构造隔离库资源边界。
        self.sql("UPDATE daily_task_rules SET total=1 WHERE id='local_recruit'")
        op, reply = self.stamped('CSWarriorRecruit', {'costType': 1, 'numType': 1, 'free': 1}, login, 1700000000001)
        self.assertEqual(op, BY_NAME['SCWarriorRecruit'], str(reply))
        self.assertEqual(self.task(login, 'local_recruit').state, 0)
        self.mutate(login, lambda p, g: p.update(coin=2**63 - 1))
        before = self.player(login)
        self.assertEqual(self.send('CSGetTaskReward', {'taskId': 'local_recruit'}, login)[0], 10001)
        self.assertEqual(self.player(login), before)
        self.assertEqual((self.task(login, 'local_recruit').state, self.info(login).score), (0, 0))

    def test_seven_real_gameplay_events_reach_all_four_boxes(self):
        login = self.create('isolated-daily-all-events')
        self.seed(login, level=20, coin=1000000)
        # 只给独立角色补足测试材料；任务计数全部由原协议成功业务产生。
        def materials(player, game):
            game['items'] = [{'id': key, 'num': amount} for key, amount in
                             [('item_016', 10), ('item_007', 10), ('item_011', 2)] +
                             [(f'frag_trea_001_{i}', 1) for i in range(1, 4)]]
        self.mutate(login, materials)
        self.assertEqual(self.info(login).score, 0)
        recruit = {'costType': 1, 'numType': 2, 'free': 0}
        self.assertEqual(self.stamped('CSWarriorRecruit', recruit, login, 100)[0], BY_NAME['SCWarriorRecruit'])
        for cost in (8, 18, 26):
            self.assertEqual(self.send('CSEquipStrengthen', {'guid': 'equip-1', 'times': 1,
                             'cost': [{'id': 'coin', 'num': cost}]}, login)[0], BY_NAME['SCEquipStrengthen'])
        refine = {'guid': 'equip-1', 'cost': [{'id': 'item_011', 'num': 1}]}
        first = self.stamped('CSEquipRefine', refine, login, 100)
        self.assertEqual(first[0], BY_NAME['SCEquipRefine'], str(first[1]))
        self.assertEqual(self.stamped('CSEquipRefine', refine, login, 100)[1], first[1])
        op, composed = self.stamped('CSCombine', {'targetId': 'trea_001'}, login, 100)
        self.assertEqual(op, BY_NAME['SCCombine'], str(composed))
        target = composed.get.treas[0].guid
        self.mutate(login, lambda p, g: g['treasures'].append({'guid': 'daily-food', 'treasureId': 'trea_005',
                    'strengthLevel': 1, 'refineLevel': 0, 'currentExp': 0}))
        op, strengthened = self.send('CSTreasureStrengthen', {'targetId': target, 'guids': ['daily-food']}, login)
        self.assertEqual(op, BY_NAME['SCTreasureStrengthen'], str(strengthened))
        training = {'guid': self.player(login)['heroWarId'], 'costType': 1, 'numType': 1}
        op, trained = self.stamped('CSWarriorTraining', training, login, 100)
        self.assertEqual(op, BY_NAME['SCWarriorTraining'], str(trained))
        self.assertEqual(self.stamped('CSWarriorTraining', training, login, 100)[1], trained)
        for _ in range(3):
            op, entry = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
            self.assertEqual(op, BY_NAME['SCCopyEnter'], str(entry))
            for task in entry.jobList:
                op, fight = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': task.taskId}, login)
                self.assertEqual(op, BY_NAME['SCCopyFightStart'], str(fight))
                finish = {'fightId': fight.fightId, 'result': 1, 'battlePlayer': fight.battlePlayer}
                op, result = self.send('CSCopyFightEnd', finish, login)
                self.assertEqual(op, BY_NAME['SCCopyFightEnd'], str(result))
                self.assertEqual(self.send('CSCopyFightEnd', finish, login)[1], result)
        complete = self.info(login)
        self.assertTrue(all(t.count == t.total and t.state == 0 for t in complete.taskList), str(complete))
        self.assertEqual(complete.score, 0)
        for task in complete.taskList:
            self.assertEqual(self.send('CSGetTaskReward', {'taskId': task.taskId}, login)[0], BY_NAME['SCGetTaskReward'])
        self.assertEqual(self.info(login).score, 140)
        for box in self.info(login).boxList:
            self.assertEqual(self.send('CSGetTaskBoxReward', {'boxId': box.boxId}, login)[0], BY_NAME['SCGetTaskBoxReward'])
        self.assertTrue(all(box.state == 2 for box in self.info(login).boxList))

    def test_real_sweep_counts_once_and_box_overflow_rolls_back_every_resource(self):
        login = self.create('isolated-daily-sweep')
        self.seed(login, level=20, coin=100000)
        # 隔离角色已有五星通关；每次扫荡仍走原协议校验、扣体力及提交回执。
        def passed(player, game):
            player.update(stamina=100)
            game['copy']['passedCopyInfo'] = [{'chapterId': '1', 'missionId': '1',
                                               'star': 5, 'challengeTimes': 0}]
        self.mutate(login, passed)
        sweep = {'chapterId': '1', 'missionId': '1'}
        self.assertEqual(self.info(login).score, 0)
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.stamped('CSCopySweep', sweep, login, 100), range(6)))
        self.assertTrue(all(op == BY_NAME['SCCopySweep'] for op, _ in replies), str(replies))
        self.assertTrue(all(reply == replies[0][1] for _, reply in replies))
        self.assertEqual(self.task(login, 'local_copy').count, 1)
        self.assertEqual(self.stamped('CSCopySweep', sweep, login, 99)[0], 10001)
        for stamp in (101, 102):
            self.assertEqual(self.stamped('CSCopySweep', sweep, login, stamp)[0], BY_NAME['SCCopySweep'])
        self.assertEqual((self.task(login, 'local_copy').count,
                          self.task(login, 'local_copy').state), (3, 0))
        self.assertEqual(self.send('CSGetTaskReward', {'taskId': 'local_copy'}, login)[0],
                         BY_NAME['SCGetTaskReward'])
        self.assertEqual(self.stamped('CSWarriorRecruit', {'costType': 1, 'numType': 1, 'free': 1}, login, 100)[0],
                         BY_NAME['SCWarriorRecruit'])
        # 单次真实招募不能领取需要三次的任务；随后独立角色补足合法招募材料。
        self.assertEqual(self.send('CSGetTaskReward', {'taskId': 'local_recruit'}, login)[0], 10001)
        self.mutate(login, lambda p, g: g.update(items=[{'id': 'item_016', 'num': 10}]))
        self.assertEqual(self.stamped('CSWarriorRecruit', {'costType': 1, 'numType': 2, 'free': 0}, login, 101)[0],
                         BY_NAME['SCWarriorRecruit'])
        self.assertEqual(self.send('CSGetTaskReward', {'taskId': 'local_recruit'}, login)[0],
                         BY_NAME['SCGetTaskReward'])
        self.assertEqual(self.info(login).score, 40)
        # 基础宝箱先增加金币，再处理钻石；钻石溢出必须回滚前项资源和领取记录。
        self.mutate(login, lambda p, g: p.update(diamond=2**63 - 1))
        before = self.profile(login)
        self.assertEqual(self.send('CSGetTaskBoxReward', {'boxId': 'box_001'}, login)[0], 10001)
        self.assertEqual(self.profile(login), before)
        self.assertEqual(self.info(login).boxList[0].state, 0)
        self.mutate(login, lambda p, g: p.update(diamond=0))
        self.assertEqual(self.send('CSGetTaskBoxReward', {'boxId': 'box_001'}, login)[0],
                         BY_NAME['SCGetTaskBoxReward'])
        self.assertEqual(self.info(login).boxList[0].state, 2)

    def test_invalid_frozen_snapshot_rolls_back_gameplay_and_recruit_receipt(self):
        login = self.create('isolated-daily-invalid-snapshot')
        self.info(login)
        snapshot = self.sql('SELECT day,snapshot FROM daily_task_states WHERE player_id=?',
                            (login.playerId,))[0]
        frozen = snapshot['snapshot']
        broken = json.loads(frozen)
        broken['tasks'][0]['reward'] = 'unknown,1'
        self.sql('UPDATE daily_task_states SET snapshot=? WHERE player_id=? AND day=?',
                 (json.dumps(broken), login.playerId, snapshot['day']))
        before = self.profile(login)
        request = {'costType': 1, 'numType': 1, 'free': 1}
        self.assertEqual(self.stamped('CSWarriorRecruit', request, login, 100)[0], 10001)
        self.assertEqual(self.profile(login), before)
        self.sql('UPDATE daily_task_states SET snapshot=? WHERE player_id=? AND day=?',
                 (frozen, login.playerId, snapshot['day']))
        self.assertEqual(self.stamped('CSWarriorRecruit', request, login, 100)[0],
                         BY_NAME['SCWarriorRecruit'])
        self.assertEqual(self.task(login, 'local_recruit').count, 1)


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_DAILY_TEST') == '1', '需显式启用独立 MySQL 日常回归')
class MySQLDailyTaskTests(GoDailyTaskTests):
    """复用专属测试建库保护，验证 InnoDB 行锁与真实二进制任务闭环。"""
    setUp = MySQLActivityTests.setUp
    tearDown = MySQLActivityTests.tearDown
    start = MySQLActivityTests.start
    root_sql = MySQLActivityTests.root_sql
    sql = MySQLActivityTests.sql
    mutate = MySQLActivityTests.mutate

    def test_mysql_daily_schema(self):
        rows = self.sql("SELECT TABLE_NAME,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME IN ('daily_task_rules','daily_task_box_rules','daily_task_states')", (self.database,))
        self.assertEqual({r['TABLE_NAME']: r['ENGINE'] for r in rows},
                         {name: 'InnoDB' for name in ('daily_task_rules', 'daily_task_box_rules', 'daily_task_states')})
        keys = self.sql("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA=? AND TABLE_NAME='daily_task_states' AND CONSTRAINT_NAME='PRIMARY' ORDER BY ORDINAL_POSITION", (self.database,))
        self.assertEqual([r['COLUMN_NAME'] for r in keys], ['player_id', 'day'])


del GoEquipmentTests
del GoFormationTests
del MySQLActivityTests
