"""All78 highest pets in isolated InnoDB and original signed HTTP, then paid reset."""
import concurrent.futures
import hashlib
import http.client
import json
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

from server.codec import BY_NAME, CLASSES, encode_http_request, decode_responses
import test_go_formation as formation

ROOT = Path(__file__).resolve().parents[1]
SOURCE = Path(os.environ.get('HYJS_P9_HIGHEST_SOURCE', str(ROOT / 'build/candidates/p9-highest-levels-20261014'))).resolve()


@unittest.skipUnless(os.environ.get('HYJS_P9_HIGHEST_MYSQL_TEST') == '1', 'explicit isolated all78 pets')
class P9HighestMySQLTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        # Optional suites remain discoverable after obsolete candidates are
        # cleaned. An enabled suite still requires an explicit fixed source.
        if not SOURCE.is_relative_to(ROOT / 'build/candidates') or not (SOURCE / 'manifest.json').is_file():
            raise ValueError('highest growth requires HYJS_P9_HIGHEST_SOURCE to name a retained fixed workspace candidate')

    send = formation.GoFormationTests.send
    _unauthed_send = formation.GoFormationTests._unauthed_send
    login = formation.GoFormationTests.login
    def stamped(self, name, fields, login, stamp):
        raw = encode_http_request(BY_NAME[name], CLASSES[name](**fields), player_id=login.playerId,
            zone_id=1, session=bytes(login.sessionKey), timestamp=stamp)
        connection = http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=15)
        try:
            connection.request('POST','/sao/',body=raw,headers={'Content-Type':'application/octet-stream'})
            response = connection.getresponse()
            data = response.read()
            self.assertEqual(response.status,200,data)
            return decode_responses(data)[0]
        finally:
            connection.close()

    def test_all78_highest_original_queries_cold_guard_off_and_exact_reset(self):
        self.run_highest_original_queries(False)

    def run_highest_original_queries(self, equipment, intimacy=False):
        count = 19 if equipment else 78
        suffix = secrets.token_hex(6)
        database, user, password = 'hyjs_test_p9_' + suffix, 'p9_' + suffix, secrets.token_hex(24)
        def root_sql(query):
            self.assertTrue(database.startswith('hyjs_test_p9_'))
            result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                input=query, text=True, capture_output=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stderr.replace(password, '[redacted]'))
            return result.stdout
        def sql(query): return root_sql('USE `' + database + '`; ' + query)
        def profile(player_id):
            row = sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id=' + str(player_id) + ';').strip().split('|')
            return [json.loads(bytes.fromhex(part)) for part in row]
        def call(name, fields, login, stamp, expected):
            op, response = self.stamped(name, fields, login, stamp)
            self.assertEqual(op, BY_NAME[expected], str(response))
            return response
        self.process = None
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-p9-highest-') as temporary:
            temp = Path(temporary)
            output = temp / 'highest.json'
            binary = temp / 'native-server'
            def start():
                self.process = subprocess.Popen([str(binary), '--http-port', '0', '--tcp-port', '0',
                    '--tables', str(SOURCE / 'analysis/tables'), '--log-file', str(temp / 'native.log')],
                    cwd=SOURCE / 'hyjs-server', env=dict(env, HYJS_MYSQL_DATABASE=database),
                    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
                with selectors.DefaultSelector() as ready:
                    ready.register(self.process.stdout, selectors.EVENT_READ)
                    self.assertTrue(ready.select(90), 'isolated all78 startup timeout')
                    line = self.process.stdout.readline()
                ports = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
                self.assertIsNotNone(ports, line.replace(password, '[redacted]'))
                self.service = types.SimpleNamespace(http_port=int(ports[1]), tcp_port=int(ports[2]))
            def stop():
                if self.process is not None and self.process.poll() is None:
                    self.process.terminate()
                    try: self.process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        self.process.kill(); self.process.wait(timeout=10)
                        self.fail('isolated all78 server shutdown timeout')
                if self.process is not None and self.process.stdout:
                    self.process.stdout.close()
                self.process = None
            try:
                root_sql(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
                env = dict(os.environ, HYJS_P9_MYSQL_DATABASE=database, HYJS_MYSQL_HOST='127.0.0.1:33068',
                    HYJS_MYSQL_USER=user, HYJS_MYSQL_PASSWORD=password, HYJS_P9_HIGHEST_KEEP='1',
                    HYJS_P9_HIGHEST_OUTPUT=str(output), HYJS_P9_INTIMACY_MYSQL_DATABASE=database if intimacy else "", HYJS_P9_INTIMACY_KEEP="1", HYJS_P9_INTIMACY_OUTPUT=str(output))
                result = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run',
                    '^TestP9IntimacyHighestAll78ActualFoodRandomPropsColdAndRefund$' if intimacy else ('^TestP9HighestEquipmentLifecycleAll19$' if equipment else '^TestP9HighestLifecycleAll78$'), '-count=1', '-timeout=420s', '-v'],
                    cwd=SOURCE / 'hyjs-server', env=env, capture_output=True, text=True, timeout=450)
                self.assertEqual(result.returncode, 0, (result.stdout + result.stderr).replace(password, '[redacted]'))
                fixture = json.loads(output.read_text())
                self.assertFalse(fixture['resetByGo'])
                self.assertEqual(len(fixture['records']), count)
                retained = ROOT / ('analysis/p9/all78-intimacy-highest-mysql-20261014.json' if intimacy else ('analysis/p9/all19-equipment-highest-mysql-20261014.json' if equipment else 'analysis/p9/all78-highest-mysql-20261014.json'))
                retained.write_bytes(output.read_bytes())
                built = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', str(binary), '.'],
                    cwd=SOURCE / 'hyjs-server', capture_output=True, text=True, timeout=180)
                self.assertEqual(built.returncode, 0, built.stdout + built.stderr)
                start()
                records = fixture['records']
                for index, saved in enumerate(records):
                    login = self.login(('p9-intimacy-highest-' if intimacy else 'p9-highest-') + saved['petId'])
                    self.assertEqual(login.playerId, saved['playerId'])
                    expected_pet = CLASSES['Pet'].FromString(bytes.fromhex(saved['petWire']))
                    expected_battle = CLASSES['BattlePlayer'].FromString(bytes.fromhex(saved['battleWire']))
                    listed = call('CSPetList', {}, login, 2000000 + index * 10, 'SCPetList')
                    self.assertEqual(list(listed.petList), [expected_pet])
                    if intimacy:
                        shown = call('CSPetHudongShow', {'guid': expected_pet.guid}, login, 2100000 + index, 'SCPetHudongShow')
                        self.assertEqual(shown.pet, expected_pet)
                        self.assertEqual(self.stamped('CSPetEat', {'guid': expected_pet.guid,
                            'cost': [CLASSES['BaseResource'](id='item_128', num=1)]}, login, 2200000 + index)[0], 10001)
                    call('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login, 2000001 + index * 10, 'SCCopyEnter')
                    info = call('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, login, 2000002 + index * 10, 'SCCopyRunMapInfo')
                    self.assertEqual(info.battlePlayer, expected_battle)
                    call('CSCopyLeave', {'chapterId': '1', 'missionId': '1'}, login, 2000003 + index * 10, 'SCCopyLeave')
                stop(); start()
                # Complete every paid reset through the signed original endpoint.
                for index, saved in enumerate(records):
                    login = self.login(('p9-intimacy-highest-' if intimacy else 'p9-highest-') + saved['petId'])
                    expected_pet = CLASSES['Pet'].FromString(bytes.fromhex(saved['petWire']))
                    listed = call('CSPetList', {}, login, 3000000 + index * 10, 'SCPetList')
                    self.assertEqual(list(listed.petList), [expected_pet])
                    before = profile(login.playerId)
                    self.assertEqual(self.stamped('CSPetReset', {'guid': expected_pet.guid}, login, 3000001 + index * 10)[0], 10001)
                    after_denied = profile(login.playerId)
                    # Profile transactions may apply due natural energy recovery.
                    # Verify the rejected reset preserves every other field and all game state.
                    recovery_fields = {'energy', 'stamina', 'energyLastRefreshTime', 'staminaLastRefreshTime'}
                    self.assertEqual({k:v for k,v in after_denied[0].items() if k not in recovery_fields},
                                     {k:v for k,v in before[0].items() if k not in recovery_fields})
                    self.assertEqual(after_denied[1], before[1])
                    for field in recovery_fields:
                        self.assertGreaterEqual(after_denied[0].get(field,0), before[0].get(field,0))
                    call('CSFormationOff', {'id': expected_pet.id, 'guid': expected_pet.guid, 'formatPos': 1}, login, 3000002 + index * 10, 'SCFormationOff')
                    stamp = 3000003 + index * 10
                    fields = {'guid': expected_pet.guid}
                    if index == 0:
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                            replies = list(pool.map(lambda _: call('CSPetReset', fields, login, stamp, 'SCPetReset'), range(8)))
                        self.assertTrue(all(reply == replies[0] for reply in replies))
                        reset = replies[0]
                    else: reset = call('CSPetReset', fields, login, stamp, 'SCPetReset')
                    self.assertEqual(len(reset.get.pets), 1 + saved['donors'])
                    self.assertTrue(all(p.level == 1 and p.breached.level == 0 and p.guid != expected_pet.guid for p in reset.get.pets))
                    if intimacy:
                        self.assertTrue(all(p.eat.level == 0 and p.eat.exp == 0 and not p.randPropList for p in reset.get.pets))
                    player, game = profile(login.playerId)
                    self.assertEqual(player['coin'], saved['initialCoin'])
                    self.assertEqual(player['diamond'], saved['initialDiamond'] - 50)
                    self.assertEqual({item['id']: item['num'] for item in game['items']}, saved['initialItems'])
                    self.assertEqual(call('CSPetReset', fields, login, stamp, 'SCPetReset'), reset)
                stop()
                engines = sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + database + "' AND TABLE_NAME LIKE 'p9_%';")
                self.assertEqual(engines.strip(), 'InnoDB')
                report = {'pets':count, 'actualUpgrades':0 if intimacy else count*5, 'actualBreaches':0 if intimacy else count*4, 'intimacyHighest':intimacy, 'spentFood':sum(row.get('spentFood',0) for row in records), 'originalHighestQueries':count,
                    'coldOriginalHighestQueries':count, 'originalPaidResets':count, 'eightConcurrentReset':'passed',
                    'equipmentHighest':equipment, 'strengthCalls':fixture.get('strengthCalls',0),
                    'advanceCalls':fixture.get('advanceCalls',0), 'awakeCalls':fixture.get('awakeCalls',0),
                    'sourceManifestSHA256':hashlib.sha256((SOURCE/'manifest.json').read_bytes()).hexdigest(),
                    'fixtureSHA256':hashlib.sha256(retained.read_bytes()).hexdigest(),
                    'scope':'prepared finite resources; real growth, guardian, signed original HTTP, cold and reset; natural and device gates remain', 'randomAllocation':'published named local equal pool policy; original private distribution unknown' if intimacy else None,
                    'terminalOverflow':'named local minimal-material rounding; original private server rule unknown',
                    'nativeCompared':False, 'authoritative':False, 'productionModified':False}
            finally:
                stop()
                root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
            remaining = root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
            self.assertEqual(remaining.split(), ['0','0','0'])
            report.update(cleanup='0 databases/users/grants', seconds=round(time.monotonic()-began,3))
            (ROOT/('analysis/p9/all78-intimacy-highest-mysql-validation-20261014.json' if intimacy else ('analysis/p9/all19-equipment-highest-mysql-validation-20261014.json' if equipment else 'analysis/p9/all78-highest-mysql-validation-20261014.json'))).write_text(json.dumps(report,indent=2))


@unittest.skipUnless(os.environ.get('HYJS_P9_EQUIPMENT_HIGHEST_MYSQL_TEST') == '1', 'explicit isolated all19 highest equipment')
class P9HighestEquipmentMySQLTests(P9HighestMySQLTests):
    test_all78_highest_original_queries_cold_guard_off_and_exact_reset = None

    def test_all19_equipment_highest_original_queries_cold_and_exact_reset(self):
        self.run_highest_original_queries(True)


@unittest.skipUnless(os.environ.get('HYJS_P9_INTIMACY_HIGHEST_MYSQL_TEST') == '1', 'explicit isolated all78 intimacy')
class P9HighestIntimacyMySQLTests(P9HighestMySQLTests):
    test_all78_highest_original_queries_cold_guard_off_and_exact_reset = None

    def test_all78_intimacy_highest_original_queries_cold_and_exact_reset(self):
        self.run_highest_original_queries(False, intimacy=True)


if __name__ == '__main__':
    unittest.main()
