"""P5下一组原主角阵容位替换，专属MySQL用户/库，定点回归并清理。"""
import json
import hashlib
import os
from pathlib import Path
import re
import secrets
import selectors
import subprocess
import tempfile
import time
import types
import unittest
from server.codec import BY_NAME
from test_go_formation import GoFormationTests

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P5_NEXT_MYSQL_TEST') == '1', '显式启用P5下一组MySQL回归')
class P5NextMySQLTests(unittest.TestCase):
    send = GoFormationTests.send
    _unauthed_send = GoFormationTests._unauthed_send
    login = GoFormationTests.login
    stamped = GoFormationTests.stamped

    def test_native_main_replacement_transaction_guids_and_restart(self):
        suffix = secrets.token_hex(6)
        database = 'hyjs_test_p5next_' + suffix
        user = 'p5next_' + suffix
        password = secrets.token_hex(24)

        def root_sql(query):
            self.assertTrue(database.startswith('hyjs_test_p5next_'))
            result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                                     'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                                    input=query, text=True, capture_output=True, timeout=30)
            self.assertEqual(result.returncode, 0, 'P5 next isolated database administration failed')
            return result.stdout

        started = time.monotonic()
        try:
            root_sql(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
            env = dict(os.environ, HYJS_P5_NEXT_MYSQL_DATABASE=database, HYJS_MYSQL_HOST='127.0.0.1:33068',
                       HYJS_MYSQL_USER=user, HYJS_MYSQL_PASSWORD=password)
            result = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP5FormationNext$', '-count=1', '-v'],
                                    cwd=ROOT / 'hyjs-server', env=env, text=True, capture_output=True, timeout=120)
            summary = '\n'.join(line for line in result.stdout.splitlines()
                                if line.startswith(('=== RUN', '--- PASS', '--- FAIL', 'ok ', 'FAIL')))
            self.assertEqual(result.returncode, 0, summary)
            self.assertIn('--- PASS: TestP5FormationNext', summary)
            engines = root_sql(f"SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='{database}' AND TABLE_NAME IN ('profiles','p5_mutation_receipts');")
            self.assertEqual(engines.strip(), 'InnoDB')
            network = self.native_network(database, env, root_sql)
            report = {'date': '2026-10-13', 'candidateGroup': 'P5-next-main-formation', 'mysqlTest': 'passed',
                      'originalFrame': 'CSFormationOn opcode 21047; Go protobuf request dispatch',
                      'originalGate': 'SwapMainHeroLevel=12', 'originalMainGuidPreserved': 'passed',
                      'sameIdDistinctGuidPreserved': 'passed', 'offFormationMainCannotAssistOrRecycle': 'passed',
                      'sixConcurrentSameFrameReceipt': 'passed', 'databaseConnectionReopen': 'passed',
                      'nativeFrameReplayAfterReopen': 'passed', 'loadoutAndInvestmentRefund': 'passed',
                      'tablesEngine': 'InnoDB', 'networkRoute': 'passed',
                      'productionSaveModified': False, 'productionDeployment': False}
            report.update(network)
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining = root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(), ['0', '0', '0'])
        report.update(dedicatedDatabaseUserGrantCleanup='passed', testSeconds=round(time.monotonic() - started, 3))
        (ROOT / 'analysis/p5/next-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')

    def native_network(self, database, env, root_sql):
        with tempfile.TemporaryDirectory(prefix='hyjs-p5next-native-') as temp:
            binary = str(Path(temp) / 'hyjs-p5-next')
            built = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', binary, '.'], cwd=ROOT / 'hyjs-server', capture_output=True, timeout=120)
            self.assertEqual(built.returncode, 0, 'P5 next isolated build failed')
            process = None
            service_env = dict(env, HYJS_MYSQL_DATABASE=database)

            def start():
                nonlocal process
                process = subprocess.Popen([binary, '--http-port', '0', '--tcp-port', '0'], cwd=ROOT / 'hyjs-server', env=service_env,
                                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
                with selectors.DefaultSelector() as ready:
                    ready.register(process.stdout, selectors.EVENT_READ)
                    self.assertTrue(ready.select(90), 'P5 next isolated startup timeout')
                    line = process.stdout.readline()
                found = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
                self.assertIsNotNone(found, 'P5 next isolated startup rejected')
                self.service = types.SimpleNamespace(http_port=int(found[1]), tcp_port=int(found[2]))

            def stop():
                nonlocal process
                if process is not None and process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill(); process.wait(timeout=10)
                if process is not None and process.stdout:
                    process.stdout.close()
                process = None

            def sql(query):
                return root_sql(f'USE `{database}`; ' + query)

            def profile(login):
                raw = sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id=' + str(login.playerId) + ';').strip().split('|')
                return [json.loads(bytes.fromhex(value)) for value in raw]

            def call(fields, login, stamp):
                opcode, result = self.stamped('CSFormationOn', fields, login, stamp)
                self.assertEqual(opcode, BY_NAME['SCFormationOn'], str(result))
                return result

            try:
                start()
                uid = 'p5next-native-' + secrets.token_hex(4)
                login = self.login(uid)
                self.assertEqual(self.send('CSPlayerCreate', {'playerName': '原主角更换网络', 'heroId': 'hero_001'}, login)[0], BY_NAME['SCPlayerCreate'])
                p, g = profile(login)
                main_guid = p['heroWarId']
                main_snapshot = next(w for w in g['warriors'] if w['guid'] == main_guid)
                p.update(level=11, noviceGuideSchedule=7)
                g['warriors'].append({'id': 'hero_020', 'guid': 'p5next-owned', 'level': 1})

                def persist():
                    values = [json.dumps(v, ensure_ascii=False, separators=(',', ':')).encode().hex() for v in [p, g]]
                    sql("UPDATE profiles SET player=X'" + values[0] + "',game=X'" + values[1] + "' WHERE id=" + str(login.playerId) + ';')

                persist()
                fields = {'formatPos': 1, 'id': 'hero_020', 'guid': 'p5next-owned'}
                before = profile(login)
                self.assertEqual(self.stamped('CSFormationOn', fields, login, 70001)[0], 10001)
                self.assertEqual(profile(login), before)
                p['level'] = 12
                persist()
                first = call(fields, login, 70002)
                self.assertEqual(first.formationItem.things[0].guid, 'p5next-owned')
                saved = profile(login)
                self.assertEqual(saved[0]['heroWarId'], main_guid)
                self.assertEqual(next(w for w in saved[1]['warriors'] if w['guid'] == main_guid), main_snapshot)
                collision = dict(fields, guid=main_guid, id='hero_001')
                self.assertEqual(self.stamped('CSFormationOn', collision, login, 70002)[0], 10001)
                self.assertEqual(profile(login), saved)
                self.assertEqual(self.stamped('CSWarriorSell', {'guid': [main_guid]}, login, 70003)[0], 10001)
                self.assertEqual(profile(login), saved)
                stop(); start(); login = self.login(uid)
                self.assertEqual(call(fields, login, 70002), first)
                self.assertEqual(profile(login), saved)
                restored = call(collision, login, 70004)
                self.assertEqual(restored.formationItem.things[0].guid, main_guid)
                return {'networkBinarySha256': hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                        'serverProcessRestart': 'passed', 'nativeReplayAfterProcessRestart': 'passed',
                        'network11Denied12Allowed': 'passed', 'networkProtectedOriginalMain': 'passed'}
            finally:
                stop()


del GoFormationTests


if __name__ == '__main__':
    unittest.main()
