"""普通商店原协议：隔离SQL与真实Go进程，核对APK显示价格和交易原子性。"""
import concurrent.futures
import csv
import json
import os
import re
import selectors
import subprocess
import types
import uuid
import unittest
from pathlib import Path

from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests
from test_go_formation import GoFormationTests


class GoShopTests(unittest.TestCase):
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

    def fixture(self, diamond=10000, level=35, vip=0):
        login = self.login('shop')
        self.send('CSPlayerCreate', {'playerName': '商店回归', 'heroId': 'hero_047'}, login)
        self.mutate(login, lambda p,g: p.update(level=level, diamond=diamond, vip=vip))
        return login

    def profile(self, login):
        row = self.sql('SELECT player,game FROM profiles WHERE id=?', (login.playerId,))[0]
        return json.loads(row['player']), json.loads(row['game'])

    def snapshot(self, login):
        return (self.profile(login), [dict(r) for r in self.sql('SELECT * FROM shop_states ORDER BY player_id,product_id,day')],
                [dict(r) for r in self.sql('SELECT * FROM shop_receipts ORDER BY player_id')])

    def products(self, login):
        opcode, result = self.send('CSShopProductList', {'shopType':1}, login)
        self.assertEqual(opcode, BY_NAME['SCShopProductList'], str(result))
        return result.shopItemList

    def fields(self, product='itemShopId_8', count=1, shop_type=1):
        return {'shopType':shop_type, 'shopItemId':product, 'buyCount':count}

    def buy(self, login, product='itemShopId_8', count=1, stamp=100):
        opcode, result = self.stamped('CSShopBuy', self.fields(product,count), login, stamp)
        self.assertEqual(opcode, BY_NAME['SCShopBuy'], str(result))
        return result

    def deny(self, login, fields, stamp=100):
        before = self.snapshot(login)
        opcode, result = self.stamped('CSShopBuy', fields, login, stamp)
        self.assertEqual((opcode,result.code), (10001,'90003'), str(result))
        self.assertEqual(self.snapshot(login), before)

    def holdings(self, login):
        player, game = self.profile(login)
        return player['diamond'], {i['id']:i['num'] for i in (game['items'] or [])}

    def price_rows(self):
        path = Path(__file__).resolve().parents[1]/'analysis/tables/TableItemShopPrice.tsv'
        with path.open() as source:
            rows = list(csv.reader(source,delimiter='\t'))
        return [dict(zip(rows[1],r)) for r in rows[5:]]

    def test_catalog_price_matches_apk_every_allowed_purchase_and_read_is_pure(self):
        login = self.fixture()
        before = self.snapshot(login)
        products = self.products(login)
        self.assertEqual([p.id for p in products], ['itemShopId_'+str(i) for i in (8,9,10,11,13,14,16)])
        prices = self.price_rows()
        for product in products:
            self.assertEqual((product.dayLeftTimes,product.dayTotalTimes,product.canBuy),(20,20,20))
            self.assertEqual(len(product.item),1)
            self.assertEqual(len(product.price),1)
            self.assertEqual(list(product.price),list(product.exchangeList))
            self.assertEqual(product.condition.type,'level')
            for number in range(1,product.dayTotalTimes+1):
                row = next(r for r in prices if r['shopItemId']==product.id and int(r['count'].split('-')[0]) <= number <= int(r['count'].split('-')[-1]))
                self.assertEqual(row['price'],product.price[0].id+','+str(product.price[0].num))
        self.assertEqual(self.snapshot(login),before)

    def test_all_seed_materials_boxes_real_balances_cost_and_batch_quantities(self):
        login = self.fixture()
        expected = {p.id:(p.item[0].id,p.item[0].num,p.price[0].num) for p in self.products(login)}
        before_diamond, before_items = self.holdings(login)
        total = 0
        for index,(product,(resource,amount,price)) in enumerate(expected.items()):
            result = self.buy(login,product,count=3,stamp=100+index)
            self.assertEqual([(c.id,c.num) for c in result.cost],[('diamond',price*3)])
            self.assertEqual([(r.id,r.num) for r in result.get.simps],[(resource,amount*3)])
            shown = next(p for p in result.shopItemList if p.id==product)
            self.assertEqual((shown.dayLeftTimes,shown.canBuy),(17,17))
            total += price*3
            diamond, items = self.holdings(login)
            self.assertEqual(diamond,before_diamond-total)
            self.assertEqual(items[resource],before_items.get(resource,0)+amount*3)
        self.assertEqual(len(self.sql('SELECT * FROM shop_states')),7)

    def test_insufficient_balance_item_overflow_and_bad_parameters_rollback(self):
        login = self.fixture(diamond=19)
        self.deny(login,self.fields())
        self.mutate(login,lambda p,g:p.update(diamond=1000))
        def overflow(p,g):
            g['items'] = [{'id':'item_006','num':2147483647}]
        self.mutate(login,overflow)
        self.deny(login,self.fields())
        for fields in (self.fields(count=0),self.fields(count=-1),self.fields(count=10001),self.fields(shop_type=2),self.fields(product='foreign')):
            self.deny(login,fields)
        self.deny(login,self.fields(),stamp=0)

    def test_daily_limit_day_boundary_level_vip_and_disabled_gates(self):
        login = self.fixture(level=1)
        self.sql("UPDATE shop_products SET daily_limit=2,min_level=2 WHERE id='itemShopId_8'")
        product = next(p for p in self.products(login) if p.id=='itemShopId_8')
        self.assertEqual((product.canBuy,product.dayLeftTimes),(0,2))
        self.deny(login,self.fields())
        self.mutate(login,lambda p,g:p.update(level=2))
        self.sql("UPDATE shop_products SET min_vip=1 WHERE id='itemShopId_8'")
        self.deny(login,self.fields())
        self.mutate(login,lambda p,g:p.update(vip=1))
        self.buy(login,count=2)
        self.deny(login,self.fields(),stamp=101)
        product = next(p for p in self.products(login) if p.id=='itemShopId_8')
        self.assertEqual((product.canBuy,product.dayLeftTimes),(0,0))
        # Move this isolated fixture's previous count to an earlier date to check UTC+8 day indexing.
        self.sql("UPDATE shop_states SET day='2000-01-01' WHERE product_id='itemShopId_8'")
        self.assertEqual(next(p.dayLeftTimes for p in self.products(login) if p.id=='itemShopId_8'),2)
        self.buy(login,stamp=102)
        self.sql("UPDATE shop_products SET enabled=0 WHERE id='itemShopId_8'")
        self.assertNotIn('itemShopId_8',[p.id for p in self.products(login)])
        self.deny(login,self.fields(),stamp=103)

    def test_sql_hot_catalog_change_and_immutable_price_invalid_catalog_rollback(self):
        login = self.fixture()
        self.sql("UPDATE shop_products SET resource_id='item_007',amount=7,daily_limit=3 WHERE id='itemShopId_8'")
        result = self.buy(login,count=2)
        self.assertEqual([(r.id,r.num) for r in result.get.simps],[('item_007',14)])
        self.assertEqual([(c.id,c.num) for c in result.cost],[('diamond',40)])
        self.assertEqual(next(p.dayLeftTimes for p in result.shopItemList if p.id=='itemShopId_8'),1)
        self.stop();self.start();login=self.login('shop')
        product = next(p for p in self.products(login) if p.id=='itemShopId_8')
        self.assertEqual((product.item[0].id,product.item[0].num,product.dayTotalTimes),('item_007',7,3))
        for value in ('diamond,21','diamond,0','coin,20'):
            self.sql('UPDATE shop_products SET cost=? WHERE id=?',(value,'itemShopId_8'))
            self.deny(login,self.fields(),stamp=101)
        self.sql("UPDATE shop_products SET cost='diamond,20' WHERE id='itemShopId_8'")
        # A malformed other enabled product must roll back a purchase after tentative reward/deduction.
        self.sql("UPDATE shop_products SET amount=0 WHERE id='itemShopId_9'")
        self.deny(login,self.fields(),stamp=101)
        self.sql("UPDATE shop_products SET amount=5 WHERE id='itemShopId_9'")
        for column,value,restored in (('amount',10001,5),('resource_id','foreign','item_007'),('min_level',0,1),('min_vip',-1,0),('daily_limit',0,20)):
            self.sql('UPDATE shop_products SET '+column+'=? WHERE id=?',(value,'itemShopId_9'))
            self.deny(login,self.fields(),stamp=101)
            self.sql('UPDATE shop_products SET '+column+'=? WHERE id=?',(restored,'itemShopId_9'))
        self.sql("UPDATE shop_products SET daily_limit=101 WHERE id='itemShopId_16'")
        self.deny(login,self.fields(),stamp=101)

    def test_receipt_replay_after_disabled_hot_config_restart_conflict_and_old_stamp(self):
        login = self.fixture()
        first = self.buy(login,count=2)
        state = self.snapshot(login)
        self.assertEqual(self.buy(login,count=2),first)
        self.assertEqual(self.snapshot(login),state)
        self.deny(login,self.fields(count=1))
        self.deny(login,self.fields(product='itemShopId_9'))
        self.sql("UPDATE shop_products SET enabled=0,amount=99,cost='diamond,999' WHERE id='itemShopId_8'")
        self.stop();self.start();login=self.login('shop')
        self.assertEqual(self.buy(login,count=2),first)
        self.assertEqual(self.snapshot(login),state)
        self.deny(login,self.fields(count=2),stamp=99)
        second = self.buy(login,product='itemShopId_9',stamp=200)
        self.assertEqual([(c.id,c.num) for c in second.cost],[('diamond',30)])
        self.deny(login,self.fields(count=2),stamp=100)

    def test_parallel_same_receipt_charges_once_and_competing_requests_respect_limit(self):
        login = self.fixture()
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.buy(login,count=3),range(8)))
        self.assertTrue(all(r==replies[0] for r in replies))
        self.assertEqual(self.holdings(login)[0],9940)
        self.assertEqual(self.holdings(login)[1]['item_006'],3)
        self.sql("UPDATE shop_products SET daily_limit=4 WHERE id='itemShopId_9'")
        before_diamond,before_items = self.holdings(login)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda n:self.stamped('CSShopBuy',self.fields('itemShopId_9'),login,200+n),range(8)))
        successes = [r for op,r in replies if op==BY_NAME['SCShopBuy']]
        self.assertGreaterEqual(len(successes),1)
        self.assertLessEqual(len(successes),4)
        for op,r in replies:
            self.assertTrue(op==BY_NAME['SCShopBuy'] or (op==10001 and r.code=='90003'))
        diamond,items = self.holdings(login)
        self.assertEqual(diamond,before_diamond-len(successes)*30)
        self.assertEqual(items['item_007'],before_items.get('item_007',0)+len(successes)*5)
        self.assertEqual(int(self.sql("SELECT bought FROM shop_states WHERE product_id='itemShopId_9'")[0]['bought']),len(successes))


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_SHOP_TEST') == '1', '需显式启用独立 MySQL 商店回归')
class MySQLShopTests(GoShopTests):
    """重复运行全部商店断言；只创建和删除专属 hyjs_test_shop_* 数据库。"""
    def setUp(self):
        self.repo = Path(__file__).resolve().parents[1]
        self.database = 'hyjs_test_shop_' + str(os.getpid()) + '_' + uuid.uuid4().hex[:8]
        self.assertTrue(self.database.startswith('hyjs_test_shop_'))
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
            self.assertTrue(ready.select(20), 'MySQL商店服务启动超时')
            line = self.process.stdout.readline()
        matched = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(matched, line)
        self.service = types.SimpleNamespace(http_port=int(matched[1]), tcp_port=int(matched[2]))

    def sql(self, text, args=()):
        self.assertTrue(self.database.startswith('hyjs_test_shop_'))
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
        rendered = rendered.replace('SELECT * FROM shop_receipts', 'SELECT player_id,timestamp,HEX(request) AS request,HEX(response) AS response FROM shop_receipts')
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
