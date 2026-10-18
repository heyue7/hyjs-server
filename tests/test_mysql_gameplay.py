"""MySQL 网络闭环。调用者提供专属 hyjs_test_* 库和临时 binary，全部源数据由本测试创建。"""
import concurrent.futures
import json
import os
from pathlib import Path
import re
import selectors
import subprocess
import tempfile
import types
import unittest

from server.codec import BY_NAME, message
from server.gameplay import Gameplay
from server.store import Store
import test_server
import test_go_formation


class MySQLGameplayTests(unittest.TestCase):
    send = test_server.LiveServerTests.send
    login = test_server.LiveServerTests.login
    stamped = test_go_formation.GoFormationTests.stamped

    def setUp(self):
        name = os.environ.get('HYJS_MYSQL_TEST_DATABASE', '')
        binary = os.environ.get('HYJS_GO_TEST_BINARY', '')
        if not name.startswith('hyjs_test_') or not binary:
            self.skipTest('专属 MySQL 测试库和临时 binary 未配置')
        self.repo = Path(__file__).resolve().parents[1]
        self.binary = binary
        self.env = dict(os.environ, HYJS_MYSQL_DATABASE=name)
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        source = Path(self.temp.name)/'python.sqlite3'
        store = Store(source)
        row = store.login('mysql-migrated', 'local-test-password')
        store.create_player(row['id'], 'MySQL迁移角色', 'hero_047')
        game = Gameplay(store)
        game.enter(row['id'], '1', '1')
        self.saved = game.begin(row['id'], '1', '1', '1')
        with store.connect() as conn:
            player = json.loads(conn.execute('SELECT player FROM profiles WHERE id=?', (row['id'],)).fetchone()[0])
            player['diamond'] = 10000
            conn.execute('UPDATE profiles SET player=? WHERE id=?', (json.dumps(player), row['id']))
        self.old_id = row['id']
        command = ['rtk', 'proxy', 'go', 'run', './cmd/migrate-mysql', '--source', str(source), '--apply']
        for expected in (1, 0):
            imported = subprocess.run(command, cwd=self.repo/'hyjs-server', env=self.env, text=True, capture_output=True)
            self.assertEqual(imported.returncode, 0, imported.stderr)
            self.assertEqual(json.loads(imported.stdout)['inserted'], expected)
        self.addCleanup(self.stop)
        self.start()

    def start(self):
        self.process = subprocess.Popen([self.binary, '--http-port', '0', '--tcp-port', '0'],
            cwd=self.repo/'hyjs-server', env=self.env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(20), 'MySQL server startup timeout')
            line = self.process.stdout.readline()
        matched = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(matched, line)
        self.service = types.SimpleNamespace(http_port=int(matched[1]), tcp_port=int(matched[2]))

    def stop(self):
        proc = getattr(self, 'process', None)
        if proc and proc.poll() is None:
            proc.terminate()
            proc.wait(timeout=8)
        if proc and proc.stdout:
            proc.stdout.close()

    def player(self, login):
        op, result = self.send('CSPlayerLogin', login=login)
        self.assertEqual(op, BY_NAME['SCPlayerLogin'], str(result))
        return result.player

    def test_fixed_guide_claim_updates_stage_once_and_survives_mysql_restart(self):
        # Use the test library's protocol battle receipts; this is independent
        # transaction verification, while actual phone victories are recorded in P0.
        login = self.login('mysql-migrated')
        op, denied = self.send('CSWarriorRecruitGuide', login=login)
        self.assertEqual((op, denied.code), (10001, '90003'))
        for task in ('1', '4'):
            op, fight = self.send('CSCopyFightStart',
                {'chapterId': '1', 'missionId': '1', 'taskId': task}, login)
            self.assertEqual(op, BY_NAME['SCCopyFightStart'])
            op, result = self.send('CSCopyFightEnd',
                {'fightId': fight.fightId, 'result': 1, 'battlePlayer': fight.battlePlayer}, login)
            self.assertEqual(op, BY_NAME['SCCopyFightEnd'])
        self.send('CSQuestReward', {'questId': 'quest_1'}, login)
        # Completing the next story before visiting the shop must keep the
        # pending guide claim visible, then restore the stored later stage.
        self.send('CSQuestReward', {'questId': 'quest_2'}, login)
        before = self.player(login)
        self.assertEqual(before.noviceGuideSchedule, 1)
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.send('CSWarriorRecruitGuide', login=login), range(6)))
        hero = replies[0][1].get.wars[0]
        self.assertEqual(hero.id, 'hero_025')
        self.assertTrue(all(op == BY_NAME['SCWarriorRecruitGuide'] and reply.get.wars[0] == hero
                            for op, reply in replies))
        self.send('CSWarriorRecruitGuide', login=login)
        self.assertEqual(self.last_frames[1][0], BY_NAME['SCPlayer'])
        self.assertEqual(self.last_frames[1][1].player.noviceGuideSchedule, 4)
        after = self.player(login)
        self.assertEqual((after.coin, after.diamond), (before.coin, before.diamond))
        _, heroes = self.send('CSWarriorList', login=login)
        self.assertEqual(sum(w.id == 'hero_025' for w in heroes.warriorList), 1)
        self.stop()
        self.start()
        login = self.login('mysql-migrated')
        self.assertEqual(self.player(login), after)
        self.assertEqual(self.send('CSWarriorRecruitGuide', login=login)[1].get.wars[0], hero)
        _, heroes = self.send('CSWarriorList', login=login)
        self.assertEqual(sum(w.id == 'hero_025' for w in heroes.warriorList), 1)

    def test_migrated_active_battle_recruit_turntable_parallel_receipts_and_restart(self):
        login = self.login('mysql-migrated')
        self.assertEqual(login.playerId, self.old_id)
        op, active = self.send('CSCopyFightStart', {'chapterId':'1','missionId':'1','taskId':'1'}, login)
        self.assertEqual(op, BY_NAME['SCCopyFightStart'], str(active))
        self.assertEqual(active, message('SCCopyFightStart', **self.saved))
        op, finished = self.send('CSCopyFightEnd', {'fightId':active.fightId,'result':1,'battlePlayer':active.battlePlayer}, login)
        self.assertEqual(op, BY_NAME['SCCopyFightEnd'], str(finished))
        before = self.player(login)
        def recruit():
            return self.stamped('CSWarriorRecruit', {'costType':2,'numType':1,'free':0}, login, 1700000000100)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: recruit(), range(8)))
        self.assertTrue(all(op == BY_NAME['SCWarriorRecruit'] for op, _ in replies), str(replies))
        self.assertTrue(all(reply == replies[0][1] for _, reply in replies))
        self.assertEqual([(x.id,x.num) for x in replies[0][1].cost], [('diamond',300)])
        self.assertEqual(self.player(login).diamond, before.diamond-300)
        op, free = self.stamped('CSPlayBigTurn', {'count':1}, login, 1700000000200)
        self.assertEqual(op, BY_NAME['SCPlayBigTurn'], str(free))
        self.assertFalse(free.cost)
        self.assertEqual(len(free.idNumStr), 1)
        op, paid = self.stamped('CSPlayBigTurn', {'count':1}, login, 1700000000201)
        self.assertEqual(op, BY_NAME['SCPlayBigTurn'], str(paid))
        self.assertEqual([(x.id,x.num) for x in paid.cost], [('diamond',50)])
        def ten():
            return self.stamped('CSPlayBigTurn', {'count':10}, login, 1700000000202)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            turns = list(pool.map(lambda _: ten(), range(8)))
        self.assertTrue(all(op == BY_NAME['SCPlayBigTurn'] for op, _ in turns), str(turns))
        self.assertTrue(all(reply == turns[0][1] for _, reply in turns))
        self.assertEqual([(x.id,x.num) for x in turns[0][1].cost], [('diamond',488)])
        self.assertEqual(len(turns[0][1].idNumStr), 10)
        # 奖池存在真实大奖概率，余额应以每次回包真实 diamond 奖励计算。
        gain = sum(x.num for r in (free, paid, turns[0][1]) for x in r.rewards.simps if x.id == 'diamond')
        after = self.player(login)
        self.assertEqual(after.diamond, before.diamond-300-50-488+gain)
        info_op, info = self.send('CSBigTurnTableInfo', login=login)
        self.assertEqual(info_op, BY_NAME['SCBigTurnTableInfo'], str(info))
        self.assertEqual((info.score, info.boughtCoins), (12, 120000))
        self.stop()
        self.start()
        login = self.login('mysql-migrated')
        self.assertEqual(self.player(login), after)
        self.assertEqual(recruit()[1], replies[0][1])
        self.assertEqual(ten()[1], turns[0][1])
        self.assertEqual(self.player(login), after)


if __name__ == '__main__':
    unittest.main()
