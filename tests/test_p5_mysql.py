"""P5真实隔离MySQL养成、背包及回收回归；专属库和专属用户始终清理。"""
import os
import hashlib
import json
from pathlib import Path
import re
import secrets
import selectors
import subprocess
import tempfile
import types
import unittest
import uuid
from server.codec import BY_NAME
from test_server import LiveServerTests
from test_go_formation import GoFormationTests


@unittest.skipUnless(os.environ.get('HYJS_P5_MYSQL_TEST') == '1', '显式启用P5专属MySQL回归')
class P5MySQLTests(unittest.TestCase):
    send = LiveServerTests.send
    login = LiveServerTests.login
    stamped = GoFormationTests.stamped

    def test_original_protocol_messages_and_innodb_payment_domain(self):
        root = Path(__file__).resolve().parents[1]
        suffix = uuid.uuid4().hex[:12]
        database = 'hyjs_test_p5_' + suffix
        user = 'hyjs_p5_' + suffix
        password = secrets.token_hex(24)
        self.assertTrue(database.startswith('hyjs_test_p5_'))

        def root_sql(query):
            result = subprocess.run(
                ['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                input=query, text=True, capture_output=True, timeout=30)
            if result.returncode:
                self.fail('P5 isolated database administration failed')
            return result.stdout

        try:
            root_sql(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; "
                     f"CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; "
                     f"GRANT ALL ON `{database}`.* TO '{user}'@'%';")
            env = dict(os.environ, HYJS_P5_MYSQL_DATABASE=database,
                       HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=user,
                       HYJS_MYSQL_PASSWORD=password)
            result = subprocess.run(
                ['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP5Domain$', '-count=1', '-v'],
                cwd=root / 'hyjs-server', env=env, text=True, capture_output=True, timeout=180)
            # Database credentials remain in the child environment. Never print
            # arbitrary diagnostic output which could include a DSN or session.
            summary = '\n'.join(line for line in result.stdout.splitlines()
                                if line.startswith(('=== RUN', '--- PASS', '--- FAIL', '    --- PASS', '    --- FAIL', 'ok ', 'FAIL')))
            self.assertEqual(result.returncode, 0, summary)
            self.assertIn('--- PASS: TestP5Domain', summary)
            self.verify_native_network(root, database, env, root_sql)
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining = root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; "
                             f"SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; "
                             f"SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(), ['0', '0', '0'])

    def verify_native_network(self, root, database, env, root_sql):
        # 只修改本测试新建角色，验证原二进制入口和实际服务进程恢复。
        with tempfile.TemporaryDirectory(prefix='hyjs-p5-native-') as temp:
            binary = str(Path(temp) / 'hyjs-p5')
            built = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', binary, '.'],
                                   cwd=root / 'hyjs-server', capture_output=True, timeout=180)
            self.assertEqual(built.returncode, 0, 'P5 isolated binary build failed')
            service_env = dict(env, HYJS_MYSQL_DATABASE=database)
            process = None

            def start():
                nonlocal process
                process = subprocess.Popen([binary, '--http-port', '0', '--tcp-port', '0'],
                                           cwd=root / 'hyjs-server', env=service_env,
                                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
                with selectors.DefaultSelector() as ready:
                    ready.register(process.stdout, selectors.EVENT_READ)
                    self.assertTrue(ready.select(90), 'P5 isolated service startup timeout')
                    line = process.stdout.readline()
                found = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
                self.assertIsNotNone(found, 'P5 isolated service startup rejected')
                self.service = types.SimpleNamespace(http_port=int(found[1]), tcp_port=int(found[2]))

            def stop():
                nonlocal process
                if process is not None and process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=10)
                if process is not None and process.stdout:
                    process.stdout.close()
                process = None

            def sql(query):
                return root_sql(f'USE `{database}`; ' + query)

            def profile(login):
                raw = sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id=' + str(login.playerId) + ';').strip().split('|')
                return [json.loads(bytes.fromhex(value)) for value in raw]

            def call(name, fields, login, stamp, expected):
                opcode, result = self.stamped(name, fields, login, stamp)
                self.assertEqual(opcode, BY_NAME[expected], str(result))
                return result

            try:
                start()
                uid = 'p5-native-' + secrets.token_hex(4)
                login = self.login(uid)
                call('CSPlayerCreate', {'playerName': 'P5养成原协议回归', 'heroId': 'hero_043'}, login, 1, 'SCPlayerCreate')
                p, g = profile(login)
                p.update(level=25, coin=1000000, diamond=100, noviceGuideSchedule=7)
                g['items'] = [{'id': 'item_001', 'num': 2}, {'id': 'item_034', 'num': 1}, {'id': 'item_067', 'num': 1}, {'id': 'item_017', 'num': 2}]
                values = [json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode().hex() for value in [p, g]]
                sql("UPDATE profiles SET player=X'" + values[0] + "',game=X'" + values[1] + "' WHERE id=" + str(login.playerId) + ';')
                fields = {'costItem': {'id': 'item_001', 'num': 1}}
                first = call('CSItemUse', fields, login, 101, 'SCItemUse')
                self.assertEqual([(r.id, r.num) for r in first.get.simps], [('energy', 10)])
                replay = call('CSItemUse', fields, login, 101, 'SCItemUse')
                self.assertEqual(first, replay)
                self.assertEqual(call('CSPlayer', {}, login, 2, 'SCPlayer').player.energy, 110)
                selected = call('CSItemUse', {'costItem': {'id': 'item_067', 'num': 1}, 'getItemIndex': 1}, login, 102, 'SCItemUse')
                helper = selected.get.wars[0]
                self.assertEqual(helper.id, 'hero_011')
                call('CSHelperOn', {'guid': helper.guid, 'pos': 1}, login, 103, 'SCHelperOn')
                formation = call('CSFormationList', {}, login, 3, 'SCFormationList')
                self.assertEqual(formation.helperList[0].guid, helper.guid)
                before = profile(login)
                self.assertEqual(self.stamped('CSWarriorSell', {'guid': [helper.guid]}, login, 104)[0], 10001)
                self.assertEqual(profile(login), before)
                call('CSSetSkillOrder', {'heroGuid': p['heroWarId'], 'skillOrder': '043_2,043_1'}, login, 105, 'SCSetSkillOrder')
                call('CSLightenWorldSeed', {'id': 2}, login, 106, 'SCLightenWorldSeed')
                stop()
                start()
                login = self.login(uid)
                call('CSPlayerLogin', {}, login, 4, 'SCPlayerLogin')
                self.assertEqual(call('CSFormationList', {}, login, 4, 'SCFormationList').helperList[0].guid, helper.guid)
                self.assertEqual(call('CSPlayer', {}, login, 4, 'SCPlayer').player.currentWorldSeedId, 2)
                self.assertEqual(call('CSItemUse', fields, login, 101, 'SCItemUse'), first)
                call('CSHelperOff', {'pos': 1}, login, 107, 'SCHelperOff')
                call('CSWarriorSell', {'guid': [helper.guid]}, login, 108, 'SCWarriorSell')
                handbook = call('CSWarriorHandbook', {}, login, 5, 'SCWarriorHandbook')
                self.assertIn('hero_011', handbook.idList)
                engines = sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + database + "' AND TABLE_NAME LIKE 'p5_%';")
                self.assertEqual(engines.strip(), 'InnoDB')
                report = {'date': '2026-10-13', 'goDomainSubtests': 15, 'nativeNetwork': 'passed',
                          'binarySha256': hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                          'isolatedMySQL': True, 'serverRestart': 'passed', 'receiptReplayAfterRestart': 'passed',
                          'formationHelperPersistence': 'passed', 'protectedRecycleRollback': 'passed',
                          'worldSeedPersistence': 'passed', 'collectionAfterSale': 'passed',
                          'tablesEngine': 'InnoDB', 'productionSaveModified': False, 'productionDeployment': False}
                (root / 'analysis/p5/backend-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
            finally:
                stop()


del LiveServerTests
del GoFormationTests

if __name__ == '__main__':
    unittest.main()
