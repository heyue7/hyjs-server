"""人物训练/突破原协议在独立临时 SQL 和真实 Go 进程上的回归。"""
import concurrent.futures
import csv
import json
import os
import re
import selectors
import subprocess
import types
import uuid
from pathlib import Path
import unittest

from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests
from test_go_formation import GoFormationTests


class GoWarriorGrowthTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    mutate = GoEquipmentTests.mutate
    stamped = GoFormationTests.stamped

    def sql(self, text, args=()):
        with self.service.store.connect() as connection:
            return connection.execute(text, args).fetchall()

    def fixture(self, level=35, stones=1000, coin=100000, diamond=100):
        login = self.login('growth')
        self.send('CSPlayerCreate', {'playerName': '成长回归', 'heroId': 'hero_047'}, login)
        def prepare(p, g):
            p.update(level=level, coin=coin, diamond=diamond)
            g['items'] = [{'id': 'item_007', 'num': stones}, {'id': 'item_006', 'num': stones}]
        self.mutate(login, prepare)
        player, game = self.profile(login)
        return login, player['heroWarId']

    def profile(self, login):
        row = self.sql('SELECT player,game FROM profiles WHERE id=?', (login.playerId,))[0]
        return json.loads(row['player']), json.loads(row['game'])

    def warrior(self, login, guid):
        return next(w for w in self.profile(login)[1]['warriors'] if w['guid'] == guid)

    def resources(self, login):
        p, g = self.profile(login)
        return p['coin'], p['diamond'], {i['id']: i['num'] for i in g['items']}

    def snapshot(self, login):
        return self.profile(login), [dict(r) for r in self.sql('SELECT * FROM warrior_growth_receipts ORDER BY player_id,guid,kind')]

    def deny(self, name, fields, login, stamp):
        before = self.snapshot(login)
        opcode, result = self.stamped(name, fields, login, stamp)
        self.assertEqual((opcode, result.code), (10001, '90003'), str(result))
        self.assertEqual(self.snapshot(login), before)

    def train(self, login, guid, tier=1, times=1, stamp=100):
        fields = {'guid': guid, 'costType': tier, 'numType': times}
        opcode, result = self.stamped('CSWarriorTraining', fields, login, stamp)
        self.assertEqual(opcode, BY_NAME['SCWarriorTraining'], str(result))
        return result

    def replace(self, login, guid, result=1, stamp=200):
        opcode, response = self.stamped('CSWarriorTrainingReplace', {'guid': guid, 'result': result}, login, stamp)
        self.assertEqual(opcode, BY_NAME['SCWarriorTrainingReplace'], str(response))
        return response

    def test_training_confirm_cancel_sql_weights_restart_and_retry(self):
        login, guid = self.fixture()
        self.sql("UPDATE warrior_growth_rules SET distribution='[[100,1]]' WHERE id LIKE 'training_%'")
        first = self.train(login, guid)
        self.assertEqual(first.guid.training.hp, 0)
        self.assertGreater(first.guid.tempTraining.hp, 0)
        self.assertEqual({c.id: c.num for c in first.cost}, {'item_007': 5})
        resources = self.resources(login)
        self.assertEqual(self.train(login, guid), first)
        self.assertEqual(self.resources(login), resources)
        self.deny('CSWarriorTraining', {'guid': guid, 'costType': 1, 'numType': 1}, login, 101)
        self.stop(); self.start(); login = self.login('growth')
        self.assertEqual(self.train(login, guid), first)
        confirmed = self.replace(login, guid)
        self.assertEqual(confirmed.result.training.hp, first.guid.tempTraining.hp)
        self.assertEqual(confirmed.result.tempTraining.hp, 0)
        self.assertEqual(self.replace(login, guid), confirmed)
        before = confirmed.result.training
        second = self.train(login, guid, tier=2, times=2, stamp=300)
        self.assertEqual({c.id: c.num for c in second.cost}, {'item_007': 20, 'coin': 15000})
        canceled = self.replace(login, guid, result=0, stamp=400)
        self.assertEqual(canceled.result.training, before)
        self.assertEqual(canceled.result.tempTraining.hp, 0)
        self.deny('CSWarriorTrainingReplace', {'guid': guid, 'result': 1}, login, 401)
        third = self.train(login, guid, tier=3, times=3, stamp=500)
        self.assertEqual({c.id: c.num for c in third.cost}, {'item_007': 30, 'diamond': 50})

    def test_training_rejects_invalid_owned_level_cost_and_sql_without_mutation(self):
        login, guid = self.fixture(level=9)
        self.deny('CSWarriorTraining', {'guid': guid, 'costType': 1, 'numType': 1}, login, 100)
        self.mutate(login, lambda p,g: p.update(level=35))
        for fields in ({'guid': 'foreign', 'costType': 1, 'numType': 1},
                       {'guid': guid, 'costType': 0, 'numType': 1},
                       {'guid': guid, 'costType': 1, 'numType': 4}):
            self.deny('CSWarriorTraining', fields, login, 100)
        self.sql("UPDATE warrior_growth_rules SET distribution='[[10,0]]' WHERE id='training_1'")
        self.deny('CSWarriorTraining', {'guid': guid, 'costType': 1, 'numType': 1}, login, 100)
        self.sql("UPDATE warrior_growth_rules SET distribution='[[10,1]]' WHERE id='training_1'")
        self.mutate(login, lambda p,g: g.update(items=[{'id': 'item_007', 'num': 4}]))
        self.deny('CSWarriorTraining', {'guid': guid, 'costType': 1, 'numType': 1}, login, 100)

    def test_parallel_training_charges_once_and_sql_negative_delta_is_cancelable(self):
        login, guid = self.fixture()
        self.sql("UPDATE warrior_growth_rules SET distribution='[[100,1]]' WHERE id='training_1'")
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            results = list(pool.map(lambda _: self.train(login, guid), range(4)))
        self.assertTrue(all(r == results[0] for r in results))
        self.assertEqual(self.resources(login)[2]['item_007'], 995)
        confirmed = self.replace(login, guid)
        self.sql("UPDATE warrior_growth_rules SET distribution='[[-50,1]]' WHERE id='training_1'")
        decreased = self.train(login, guid, stamp=300)
        self.assertLess(decreased.guid.tempTraining.hp, 0)
        result = self.replace(login, guid, stamp=400)
        self.assertEqual(result.result.training.hp, confirmed.result.training.hp + decreased.guid.tempTraining.hp)

    def test_breach_real_cost_experience_ten_attempts_retry_and_restart(self):
        login, guid = self.fixture()
        fields = {'guid': guid, 'type': 1}
        opcode, single = self.stamped('CSWarriorBreached', fields, login, 100)
        self.assertEqual(opcode, BY_NAME['SCWarriorBreached'], str(single))
        self.assertEqual((single.guid.breached.level, single.guid.breached.exp), (1, 2))
        self.assertEqual({c.id: c.num for c in single.cost}, {'item_006': 2})
        before = self.resources(login)
        self.assertEqual(self.stamped('CSWarriorBreached', fields, login, 100)[1], single)
        self.assertEqual(self.resources(login), before)
        self.stop(); self.start(); login = self.login('growth')
        self.assertEqual(self.stamped('CSWarriorBreached', fields, login, 100)[1], single)
        opcode, ten = self.stamped('CSWarriorBreached', {'guid': guid, 'type': 2}, login, 200)
        self.assertEqual(opcode, BY_NAME['SCWarriorBreached'], str(ten))
        self.assertEqual((ten.guid.breached.level, ten.guid.breached.exp), (1, 22))
        self.assertEqual({c.id: c.num for c in ten.cost}, {'item_006': 20})
        self.deny('CSWarriorBreached', {'guid': guid, 'type': 2}, login, 100)
        self.deny('CSWarriorBreached', {'guid': guid, 'type': 3}, login, 300)

    def test_breach_boundary_max_level_and_insufficient_rollback(self):
        login, guid = self.fixture()
        def state(p,g):
            w = next(w for w in g['warriors'] if w['guid'] == guid)
            w['breached'] = {'level': 1, 'exp': 118}
        self.mutate(login, state)
        # SQL critical multiplier=1 makes the APK's first-stage fixed +2 deterministic.
        self.sql("UPDATE warrior_growth_rules SET critical_multiplier=1 WHERE id='breach'")
        opcode, result = self.stamped('CSWarriorBreached', {'guid': guid, 'type': 1}, login, 100)
        self.assertEqual(opcode, BY_NAME['SCWarriorBreached'], str(result))
        self.assertEqual((result.guid.breached.level, result.guid.breached.exp), (2, 0))
        self.mutate(login, lambda p,g: g.update(items=[{'id': 'item_006', 'num': 49}]))
        self.deny('CSWarriorBreached', {'guid': guid, 'type': 2}, login, 200)
        def maximum(p,g):
            next(w for w in g['warriors'] if w['guid'] == guid)['breached'] = {'level': 25, 'exp': 0}
        self.mutate(login, maximum)
        self.deny('CSWarriorBreached', {'guid': guid, 'type': 1}, login, 300)

    def test_confirmed_training_and_breach_percent_enter_real_battle(self):
        login, guid = self.fixture()
        self.sql("UPDATE warrior_growth_rules SET distribution='[[100,1]]' WHERE id='training_1'")
        def begin():
            self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
            opcode, fight = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
            self.assertEqual(opcode, BY_NAME['SCCopyFightStart'], str(fight))
            hero = next(h for h in fight.battlePlayer.formation if h.guid == guid)
            self.send('CSCopyLeave', {'chapterId': '1', 'missionId': '1'}, login)
            return hero.prop
        base = begin()
        preview = self.train(login, guid)
        pending = begin()
        self.assertEqual(pending, base)
        confirmed = self.replace(login, guid).result.training
        grown = begin()
        for key in ('hp', 'att', 'pdef', 'mdef'):
            self.assertAlmostEqual(getattr(grown,key), getattr(base,key) + getattr(confirmed,key))
        def breach(p,g):
            next(w for w in g['warriors'] if w['guid'] == guid)['breached'] = {'level': 2, 'exp': 0}
        self.mutate(login, breach)
        breached = begin()
        for key in ('hp', 'att', 'pdef', 'mdef'):
            self.assertAlmostEqual(getattr(breached,key), getattr(grown,key)*1.06)

    def test_zero_preview_does_not_lock_client_and_sql_disable_rolls_back(self):
        login, guid = self.fixture()
        self.sql("UPDATE warrior_growth_rules SET distribution='[[-100,1]]' WHERE id='training_1'")
        result = self.train(login, guid)
        self.assertEqual(result.guid.tempTraining.state, 0)
        self.assertEqual(result.guid.tempTraining.hp, 0)
        self.train(login, guid, stamp=101)
        self.assertEqual(self.resources(login)[2]['item_007'], 990)
        self.sql("UPDATE warrior_growth_rules SET enabled=0 WHERE id='training_1'")
        self.deny('CSWarriorTraining', {'guid': guid, 'costType': 1, 'numType': 1}, login, 102)

    def test_training_at_apk_cap_does_not_charge_and_step_clamps(self):
        login, guid = self.fixture()
        path = Path(__file__).resolve().parents[1]/'analysis/tables/TableHeroTrainingMaxMin.tsv'
        with path.open() as source:
            rows = list(csv.reader(source, delimiter='\t'))
        row = next(dict(zip(rows[1], r)) for r in rows[5:] if dict(zip(rows[1],r))['warId'] == 'hero_047' and dict(zip(rows[1],r))['warLv'] == '35')
        caps = {key: int(row[key+'Max']) for key in ('hp','att','pdef','mdef')}
        def near_cap(p,g):
            next(w for w in g['warriors'] if w['guid'] == guid)['training'] = {key: v-1 for key,v in caps.items()}
        self.mutate(login, near_cap)
        self.sql("UPDATE warrior_growth_rules SET distribution='[[10000,1]]' WHERE id='training_1'")
        result = self.train(login, guid, times=3)
        for key in caps:
            self.assertEqual(getattr(result.guid.tempTraining,key), 1)
        confirmed = self.replace(login,guid).result.training
        for key, value in caps.items():
            self.assertEqual(getattr(confirmed,key), value)
        self.deny('CSWarriorTraining', {'guid': guid,'costType': 1,'numType': 1}, login, 300)

    def test_sql_breach_reset_policy_and_level_gate_are_explicit(self):
        login, guid = self.fixture(level=34)
        self.deny('CSWarriorBreached', {'guid': guid, 'type': 1}, login, 100)
        def old(p,g):
            p['level'] = 35
            next(w for w in g['warriors'] if w['guid'] == guid)['breached'] = {'level': 1, 'exp': 20, 'lastAddExpTime': '1'}
        self.mutate(login, old)
        self.sql("UPDATE warrior_growth_rules SET reset_seconds=1 WHERE id='breach'")
        opcode, result = self.stamped('CSWarriorBreached', {'guid': guid, 'type': 1}, login, 100)
        self.assertEqual(opcode, BY_NAME['SCWarriorBreached'], str(result))
        self.assertEqual(result.guid.breached.exp, 2)
        self.sql("UPDATE warrior_growth_rules SET critical_multiplier=0 WHERE id='breach'")
        self.deny('CSWarriorBreached', {'guid': guid, 'type': 1}, login, 101)


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_GROWTH_TEST') == '1', '需显式启用独立 MySQL 成长回归')
class MySQLWarriorGrowthTests(GoWarriorGrowthTests):
    """重复运行全部成长断言；只创建和删除专属 hyjs_test_growth_* 数据库。"""
    def setUp(self):
        self.repo = Path(__file__).resolve().parents[1]
        self.database = 'hyjs_test_growth_' + str(os.getpid()) + '_' + uuid.uuid4().hex[:8]
        self.assertTrue(self.database.startswith('hyjs_test_growth_'))
        env = dict(os.environ)
        for line in (self.repo/'.local-env/mysql.env').read_text().splitlines():
            if line and not line.startswith('#') and '=' in line:
                key, value = line.split('=', 1)
                env[key] = value.strip().strip('"').strip("'")
        self.env = dict(env, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER='hyjs', HYJS_MYSQL_DATABASE=self.database)
        self.root_sql('CREATE DATABASE `' + self.database + '` CHARACTER SET utf8mb4; GRANT ALL ON `' + self.database + "`.* TO 'hyjs'@'%';")
        self.addCleanup(lambda: self.root_sql('REVOKE ALL PRIVILEGES ON `' + self.database + "`.* FROM 'hyjs'@'%'; DROP DATABASE IF EXISTS `" + self.database + '`;'))
        self.addCleanup(self.stop)
        self.start()

    def tearDown(self):
        self.stop()

    def root_sql(self, sql, database=None):
        # 容器内部读取已有root密码，命令和输出均不包含凭据值。
        command = ['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                   'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --default-character-set=utf8mb4' + (' ' + database if database else '')]
        result = subprocess.run(command, input=sql, text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result.stdout

    def start(self):
        binary = os.environ.get('HYJS_GO_TEST_BINARY', str(self.repo/'build/hyjs-server'))
        self.process = subprocess.Popen([binary, '--http-port', '0', '--tcp-port', '0'],
                                        cwd=self.repo/'hyjs-server', env=self.env,
                                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(20), 'MySQL成长服务启动超时')
            line = self.process.stdout.readline()
        matched = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(matched, line)
        self.service = types.SimpleNamespace(http_port=int(matched[1]), tcp_port=int(matched[2]))

    def sql(self, text, args=()):
        self.assertTrue(self.database.startswith('hyjs_test_growth_'))
        parts = text.split('?')
        self.assertEqual(len(parts), len(args)+1)
        rendered = parts[0]
        for value, tail in zip(args, parts[1:]):
            if isinstance(value, (int,float)):
                rendered += str(value)
            elif value is None:
                rendered += 'NULL'
            else:
                rendered += "CONVERT(X'" + str(value).encode().hex() + "' USING utf8mb4)"
            rendered += tail
        # 二进制回执转为hex便于稳定比较，不让原protobuf字节破坏mysql行列分隔。
        rendered = rendered.replace('SELECT * FROM warrior_growth_receipts', 'SELECT player_id,guid,kind,timestamp,HEX(request) AS request,HEX(response) AS response FROM warrior_growth_receipts')
        output = self.root_sql(rendered + ';', self.database)
        lines = output.splitlines()
        if not lines:
            return []
        keys = lines[0].split('\t')
        return [dict(zip(keys,line.split('\t'))) for line in lines[1:]]

    def mutate(self, login, action):
        player, game = self.profile(login)
        action(player, game)
        self.sql('UPDATE profiles SET player=?,game=? WHERE id=?', (json.dumps(player), json.dumps(game), login.playerId))


del GoEquipmentTests, GoFormationTests
