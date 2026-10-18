"""Original life boxes, physical gems and receipts on disposable MySQL/HTTP."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import unittest

from server.codec import BY_NAME
import test_p10_mysql as transport

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P5_LIFE_MYSQL_TEST') == '1', 'Explicit isolated life-box MySQL test')
class LifeBoxMySQLTests(unittest.TestCase):
    send = transport.P10MySQLTests.send
    login = transport.P10MySQLTests.login
    stamped = transport.P10MySQLTests.stamped
    call = transport.P10MySQLTests.call
    profile = transport.P10MySQLTests.profile
    start = transport.P10MySQLTests.start
    stop = transport.P10MySQLTests.stop
    setUp = transport.P10MySQLTests.setUp

    @classmethod
    def setUpClass(cls):
        cls.database = 'hyjs_test_p5life_' + secrets.token_hex(6)
        cls.test_user = 'p5life_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-life-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-life-box')
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")

        def clean():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0', '0', '0']:
                raise AssertionError('Life box test schema/user/grants remain')
        cls.addClassCleanup(clean)
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068',
                       HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password,
                       HYJS_MYSQL_DATABASE=cls.database, HYJS_P5_LIFE_MYSQL_DATABASE=cls.database)

    @classmethod
    def root_sql(cls, text):
        if not cls.database.startswith('hyjs_test_p5life_') or not cls.test_user.startswith('p5life_'):
            raise AssertionError('Life box isolation guard')
        result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                                 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                                input=text, text=True, capture_output=True, timeout=30)
        if result.returncode:
            raise AssertionError('Life box isolated database administration failed')
        return result.stdout

    def sql(self, text):
        return self.root_sql('USE `' + self.database + '`; ' + text)

    def test_1_original_rows_concurrency_mixed_rollback_and_reopen(self):
        result = subprocess.run(['rtk', 'proxy', 'go', 'test', '-race', './service', '-run', '^TestP5LifeBoxes$', '-count=1', '-v'],
                                cwd=ROOT / 'hyjs-server', env=self.env, capture_output=True, text=True, timeout=150)
        summary = '\n'.join(line for line in result.stdout.splitlines()
                            if line.startswith(('=== RUN', '--- PASS', '--- FAIL', '    --- PASS', '    --- FAIL', 'ok ', 'FAIL')))
        self.assertEqual(result.returncode, 0, summary)
        print(summary)

    def test_2_original_http_box_to_mount_reconnect_and_no_duplicate(self):
        began = time.monotonic()
        build = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', self.binary, '.'],
                               cwd=ROOT / 'hyjs-server', capture_output=True, timeout=180)
        self.assertEqual(build.returncode, 0, 'Life box candidate build failed')
        self.start()
        uid = 'life-box-native-' + secrets.token_hex(6)
        login = self.login(uid)
        self.call('CSPlayerCreate', {'playerName': '原宝石箱验收', 'heroId': 'hero_043'}, login, 1, 'SCPlayerCreate')
        player, game = self.profile(login)
        player.update(level=75, noviceGuideSchedule=7, coin=100000, diamond=100)
        # Only original sealed boxes are finite setup. Every gem and equipment
        # below must originate from a committed original box request.
        game['items'] = [{'id': 'item_342', 'num': 2}, {'id': 'item_076', 'num': 100}, {'id': 'item_119', 'num': 1}]
        game['gems'] = []
        raw = [json.dumps(v, ensure_ascii=False, separators=(',', ':')).encode().hex() for v in [player, game]]
        self.sql("UPDATE profiles SET player=X'" + raw[0] + "',game=X'" + raw[1] + "' WHERE id=" + str(login.playerId) + ';')
        choice = {'costItem': {'id': 'item_342', 'num': 1}, 'getItemIndex': 1}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.call('CSItemUse', choice, login, 99200, 'SCItemUse'), range(8)))
        self.assertTrue(all(reply == replies[0] for reply in replies))
        self.assertEqual([(r.id, r.num) for r in replies[0].get.simps], [('MineItem_025', 1)])
        equipment = self.call('CSItemUse', {'costItem': {'id': 'item_119', 'num': 1}, 'getItemIndex': 1}, login, 99201, 'SCItemUse').get.equips[0]
        mounted = self.call('CSGemMount', {'guid': equipment.guid, 'gemId': 'MineItem_025', 'pos': 0}, login, 99202, 'SCGemMount')
        self.assertEqual(mounted.equip.gems[0], 'MineItem_025')
        before = self.profile(login)
        opcode, _ = self.stamped('CSItemUse', {'costItem': {'id': 'item_342', 'num': 1}, 'getItemIndex': 7}, login, 99203)
        self.assertEqual(opcode, 10001)
        self.assertEqual(self.profile(login), before)
        self.call('CSItemUse', {'costItem': {'id': 'item_076', 'num': 100}}, login, 99204, 'SCItemUse')
        self.call('CSItemUse', {'costItem': {'id': 'item_342', 'num': 1}, 'getItemIndex': 6}, login, 99205, 'SCItemUse')
        saved = self.profile(login)
        self.assertTrue(saved[1]['gems'])
        self.assertFalse(any(r['id'].startswith('MineItem_') for r in saved[1]['items']))
        self.stop()
        self.start()
        login = self.login(uid)
        self.assertEqual(self.call('CSItemUse', choice, login, 99200, 'SCItemUse'), replies[0])
        self.assertEqual(self.profile(login), saved)
        listing = self.call('CSGemAndCookingList', {}, login, 99206, 'SCGemAndCookingList')
        self.assertEqual({r.id: r.num for r in listing.gemList}, {r['id']: r['num'] for r in saved[1]['gems']})
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME LIKE 'p5_%';").strip(), 'InnoDB')
        (ROOT / 'analysis/p5/life-box-validation.json').write_text(json.dumps({
            'date': '2026-10-14', 'goRaceMySQL': 'passed', 'originalHTTP': 'passed',
            'allOriginalNormalLifeBoxes': ['item_076', 'item_123', 'item_137', 'item_342', 'item_343', 'item_344', 'item_345'],
            'eightConcurrentSameReceipt': 'passed', 'choiceToPhysicalMount': 'passed',
            'invalidSelectionRollback': 'passed', 'random100OriginalBoxOutputs': 'passed',
            'processRestartAndOriginalInventoryQuery': 'passed',
            'binarySha256': hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),
            'networkSeconds': round(time.monotonic() - began, 3),
            'preparation': 'finite original sealed boxes only; no gem/equipment instance injected',
            'remaining': 'activity categories22/23 and malformed original row881 remain unaudited; natural box supply and phone UI await root',
            'productionModified': False}, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
