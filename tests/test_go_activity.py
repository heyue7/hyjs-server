"""基金和全民福利：真实二进制请求、独立SQLite、重启与并发防重。"""
import concurrent.futures
import json
import time
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

class GoActivityTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    mutate = GoEquipmentTests.mutate

    def sql(self, text, args=()):
        with self.service.store.connect() as c:
            return c.execute(text,args).fetchall()

    def fixture(self, uid="fund", vip=2, level=10, diamond=1000):
        login=self.login(uid)
        self.send("CSPlayerCreate",{"playerName":uid,"heroId":"hero_043"},login)
        self.mutate(login,lambda p,g:p.update(vip=vip,level=level,diamond=diamond))
        now=int(time.time()*1000)
        self.sql("UPDATE activity_rules SET start_at=?,end_at=?,reward_end_at=?",(now-60000,now+3600000,now+7200000))
        return login

    def info(self, login):
        op,r=self.send("CSOpenServerFundInfo",login=login)
        self.assertEqual(op,BY_NAME["SCOpenServerFundInfo"],str(r))
        return r

    def profile(self,login):
        row=self.sql("SELECT player,game FROM profiles WHERE id=?",(login.playerId,))[0]
        return json.loads(row["player"]),json.loads(row["game"])

    def balance(self,login):
        return json.loads(self.sql("SELECT player FROM profiles WHERE id=?",(login.playerId,))[0]["player"])["diamond"]

    def test_buy_claim_concurrency_restart_and_real_count(self):
        login=self.fixture()
        info=self.info(login)
        self.assertEqual((info.buyStatus,info.buyCount),(1,0))
        self.assertEqual([x.index for x in info.openServerFundList],[10,25,35,45,55,60])
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
            replies=list(pool.map(lambda _:self.send("CSBuyOpenServerFund",login=login),range(5)))
        self.assertEqual(sum(op==BY_NAME["SCBuyOpenServerFund"] for op,r in replies),1)
        self.assertEqual(self.balance(login),0)
        self.assertEqual(self.info(login).buyCount,1)
        fields={"rewardType":"openServerFund","index":10}
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
            replies=list(pool.map(lambda _:self.send("CSOpenServerFundReward",fields,login),range(5)))
        self.assertEqual(sum(op==BY_NAME["SCOpenServerFundReward"] for op,r in replies),1)
        self.assertEqual(self.balance(login),500)
        self.stop();self.start();login=self.login("fund")
        self.assertEqual(self.info(login).openServerFundList[0].status,2)
        self.assertEqual(self.balance(login),500)
        other=self.fixture("other",vip=0)
        self.assertEqual(self.info(other).buyCount,1)
        self.sql("UPDATE activity_rewards SET threshold=1 WHERE reward_type='everyoneBenefit' AND threshold=10")
        op,r=self.send("CSOpenServerFundReward",{"rewardType":"everyoneBenefit","index":1},other)
        self.assertEqual(op,BY_NAME["SCOpenServerFundReward"],str(r))
        self.assertEqual(r.get.wars[0].id,"hero_exp_004")
        self.assertTrue(r.get.wars[0].guid)

    def test_gates_period_invalid_config_and_sql_hot_changes(self):
        login=self.fixture(vip=1)
        op,r=self.send("CSBuyOpenServerFund",login=login)
        self.assertEqual(op,10001)
        self.mutate(login,lambda p,g:p.update(vip=2,diamond=999))
        self.assertEqual(self.send("CSBuyOpenServerFund",login=login)[0],10001)
        self.mutate(login,lambda p,g:p.update(diamond=1000))
        self.assertEqual(self.send("CSBuyOpenServerFund",login=login)[0],BY_NAME["SCBuyOpenServerFund"])
        for fields in ({"rewardType":"wrong","index":10},{"rewardType":"openServerFund","index":25},{"rewardType":"everyoneBenefit","index":10}):
            self.assertEqual(self.send("CSOpenServerFundReward",fields,login)[0],10001)
        self.sql("UPDATE activity_rewards SET reward='diamond,750' WHERE reward_type='openServerFund' AND threshold=10")
        op,r=self.send("CSOpenServerFundReward",{"rewardType":"openServerFund","index":10},login)
        self.assertEqual(op,BY_NAME["SCOpenServerFundReward"],str(r))
        self.assertEqual(self.balance(login),750)
        self.sql("UPDATE activity_rules SET reward_end_at=?",(int(time.time()*1000)-1,))
        self.assertEqual(self.send("CSOpenServerFundInfo",login=login)[0],10001)

    def login_info(self, login):
        op,r=self.send("CSActivityInfo",{"activityId":"totalLogin_local_20261011"},login)
        self.assertEqual(op,BY_NAME["SCActivityInfo"],str(r))
        return r

    def test_login_day_only_on_successful_player_login_and_atomic_claim(self):
        login=self.fixture("login-days")
        now=int(time.time()*1000)
        self.sql("UPDATE activity_rules SET start_at=?,end_at=?,reward_end_at=? WHERE id='totalLogin_local_20261011'",(now-60000,now+3600000,now+7200000))
        self.assertEqual(self.login_info(login).itemList[0].left,0)
        self.assertEqual(self.login_info(login).itemList[0].left,0)
        self.send("CSPlayerLogin",login=login)
        self.send("CSPlayerLogin",login=login)
        info=self.login_info(login)
        self.assertEqual((info.itemList[0].left,info.itemList[0].status,info.itemList[0].pickOneOrGetAll),(1,1,1))
        self.assertEqual(info.itemList[1].status,0)
        fields={"activityId":"totalLogin_local_20261011","index":1,"getResources":[{"id":"coin","num":10000}]}
        tampered=dict(fields,getResources=[{"id":"diamond","num":10000}])
        self.assertEqual(self.send("CSActivityReward",tampered,login)[0],10001)
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
            replies=list(pool.map(lambda _:self.send("CSActivityReward",fields,login),range(5)))
        self.assertEqual(sum(op==BY_NAME["SCActivityReward"] for op,r in replies),1)
        row=self.sql("SELECT player FROM profiles WHERE id=?",(login.playerId,))[0]
        self.assertEqual(json.loads(row["player"])["coin"],10000)
        self.stop();self.start();login=self.login("login-days")
        self.send("CSPlayerLogin",login=login)
        self.assertEqual(self.login_info(login).itemList[0].status,2)
        self.assertEqual(self.sql("SELECT days FROM activity_login_states WHERE player_id=?",(login.playerId,))[0]["days"],1)
        self.assertEqual(self.send("CSActivityInfo",{"activityId":"dailySign"},login)[0],10001)
        # Isolated fixture represents a recorded prior day; successful login advances one day.
        self.sql("UPDATE activity_login_states SET last_day='2000-01-01' WHERE player_id=?",(login.playerId,))
        self.send("CSPlayerLogin",login=login)
        self.assertEqual(self.login_info(login).itemList[1].status,1)
        self.assertEqual(self.sql("SELECT days FROM activity_login_states WHERE player_id=?",(login.playerId,))[0]["days"],2)
        self.sql("UPDATE activity_rules SET end_at=? WHERE id='totalLogin_local_20261011'",(int(time.time()*1000)-1,))
        self.sql("UPDATE activity_login_states SET last_day='2000-01-01' WHERE player_id=?",(login.playerId,))
        self.send("CSPlayerLogin",login=login)
        self.assertEqual(self.sql("SELECT days FROM activity_login_states WHERE player_id=?",(login.playerId,))[0]["days"],2)
        op,r=self.send("CSActivityReward",{"activityId":"totalLogin_local_20261011","index":2,"getResources":[{"id":"item_001","num":1}]},login)
        self.assertEqual(op,BY_NAME["SCActivityReward"],str(r))
        self.sql("UPDATE activity_rules SET enabled=0 WHERE id='totalLogin_local_20261011'")
        self.assertEqual(self.send("CSActivityInfo",{"activityId":"totalLogin_local_20261011"},login)[0],10001)


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_ACTIVITY_TEST') == '1', '需显式启用独立 MySQL 活动回归')
class MySQLActivityTests(GoActivityTests):
    """重复运行全部活动断言；只创建和删除专属 hyjs_test_activity_* 数据库。"""
    def setUp(self):
        self.repo = Path(__file__).resolve().parents[1]
        self.database = 'hyjs_test_activity_' + str(os.getpid()) + '_' + uuid.uuid4().hex[:8]
        self.assertTrue(self.database.startswith('hyjs_test_activity_'))
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
            self.assertTrue(ready.select(20), 'MySQL活动服务启动超时')
            line = self.process.stdout.readline()
        matched = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(matched, line)
        self.service = types.SimpleNamespace(http_port=int(matched[1]), tcp_port=int(matched[2]))

    def sql(self, text, args=()):
        self.assertTrue(self.database.startswith('hyjs_test_activity_'))
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
        output = self.root_sql(rendered + ';', self.database)
        lines = output.splitlines()
        if not lines:
            return []
        keys = lines[0].split('\t')
        rows = [dict(zip(keys,line.split('\t'))) for line in lines[1:]]
        for row in rows:
            if "days" in row: row["days"] = int(row["days"])
        return rows

    def mutate(self, login, action):
        player, game = self.profile(login)
        action(player, game)
        self.sql('UPDATE profiles SET player=?,game=? WHERE id=?', (json.dumps(player), json.dumps(game), login.playerId))


del GoEquipmentTests
