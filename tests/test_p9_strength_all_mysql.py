"""Prepared paid pet -> original signed one-key/cold/reset in isolated InnoDB."""
import concurrent.futures
import hashlib
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

from server.codec import BY_NAME, CLASSES
import test_p9_highest_mysql as highest
SOURCE, ROOT = highest.SOURCE, highest.ROOT


@unittest.skipUnless(os.environ.get('HYJS_P9_STRENGTH_ALL_MYSQL_TEST') == '1', 'explicit isolated original one-key')
class P9StrengthAllMySQLTests(highest.P9HighestMySQLTests):
    test_all78_highest_original_queries_cold_guard_off_and_exact_reset = None

    def test_original_one_key_partial_concurrent_cold_maximum_and_reset(self):
        suffix = secrets.token_hex(6)
        database, user, password = 'hyjs_test_p9_' + suffix, 'p9_' + suffix, secrets.token_hex(24)
        began = time.monotonic()
        self.process = None

        def root_sql(query):
            self.assertTrue(database.startswith('hyjs_test_p9_'))
            result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                input=query, text=True, capture_output=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stderr.replace(password, '[redacted]'))
            return result.stdout

        def sql(query): return root_sql('USE `' + database + '`; ' + query)
        def profile(player_id):
            pieces = sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id=' + str(player_id) + ';').strip().split('|')
            return [json.loads(bytes.fromhex(part)) for part in pieces]
        def call(name, fields, login, stamp, expected):
            op, response = self.stamped(name, fields, login, stamp)
            self.assertEqual(op, BY_NAME[expected], str(response))
            return response
        def stop():
            if self.process is not None:
                if self.process.poll() is None:
                    self.process.terminate()
                    try: self.process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        self.process.kill(); self.process.wait(timeout=10)
                if self.process.stdout: self.process.stdout.close()
                self.process = None

        with tempfile.TemporaryDirectory(prefix='hyjs-p9-one-key-') as temporary:
            temp = Path(temporary)
            binary, output = temp / 'server', temp / 'paid.json'
            env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=user,
                HYJS_MYSQL_PASSWORD=password, HYJS_MYSQL_DATABASE=database,
                HYJS_P9_INTIMACY_MYSQL_DATABASE=database, HYJS_P9_STRENGTH_ALL_KEEP='1',
                HYJS_P9_STRENGTH_ALL_OUTPUT=str(output))
            def start():
                self.process = subprocess.Popen([str(binary), '--http-port', '0', '--tcp-port', '0',
                    '--tables', str(SOURCE / 'analysis/tables'), '--log-file', str(temp / 'server.log')],
                    cwd=SOURCE / 'hyjs-server', env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
                with selectors.DefaultSelector() as ready:
                    ready.register(self.process.stdout, selectors.EVENT_READ)
                    self.assertTrue(ready.select(90), 'one-key server startup timeout')
                    line = self.process.stdout.readline()
                ports = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
                self.assertIsNotNone(ports, line.replace(password, '[redacted]'))
                self.service = types.SimpleNamespace(http_port=int(ports[1]), tcp_port=int(ports[2]))
            try:
                root_sql(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
                prepared = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP9StrengthAllPartialBalanceConcurrentColdAndExpiredReceipt$', '-count=1'],
                    cwd=SOURCE / 'hyjs-server', env=env, capture_output=True, text=True, timeout=150)
                self.assertEqual(prepared.returncode, 0, (prepared.stdout + prepared.stderr).replace(password, '[redacted]'))
                fixture = json.loads(output.read_text())
                built = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', str(binary), '.'], cwd=SOURCE / 'hyjs-server', capture_output=True, text=True, timeout=180)
                self.assertEqual(built.returncode, 0, built.stdout + built.stderr)
                start()
                login = self.login('p9-strength-all')
                self.assertEqual(login.playerId, fixture['playerId'])
                original_pet = CLASSES['Pet'].FromString(bytes.fromhex(fixture['petWire']))
                fields = {'petGuid': original_pet.guid}
                listed = call('CSPetList', {}, login, 6500000, 'SCPetList')
                self.assertEqual(list(listed.petList), [original_pet])
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                    replies = list(pool.map(lambda _: call('CSPetEquipStrengthAll', fields, login, 6500001, 'SCPetEquipStrengthAll'), range(8)))
                first = replies[0]
                self.assertTrue(all(reply == first for reply in replies))
                self.assertEqual([(r.id, r.num) for r in first.cost], [('coin', 2520)])
                self.assertEqual([r.strengthLevel for r in first.petEquip], [1, 1])
                self.assertEqual([r.equipId for r in first.petEquip], [r.equipId for r in original_pet.petEquipList[:2]])
                self.assertEqual(profile(login.playerId)[0]['coin'], 1259)
                stop(); start()
                login = self.login('p9-strength-all')
                self.assertEqual(call('CSPetEquipStrengthAll', fields, login, 6500001, 'SCPetEquipStrengthAll'), first)
                before = profile(login.playerId)
                self.assertEqual(self.stamped('CSPetEquipStrengthAll', fields, login, 6500002)[0], 10001)
                self.assertEqual(profile(login.playerId), before)
                # Fund only this isolated account with a declared finite test budget.
                player = before[0]
                player['coin'] = 100000000
                raw = json.dumps(player, ensure_ascii=False, separators=(',', ':')).encode().hex()
                sql('UPDATE profiles SET player=UNHEX("' + raw + '") WHERE id=' + str(login.playerId) + ';')
                full = call('CSPetEquipStrengthAll', fields, login, 6500003, 'SCPetEquipStrengthAll')
                self.assertEqual(len(full.petEquip), 4)
                self.assertTrue(all(r.strengthLevel == original_pet.level for r in full.petEquip))
                self.assertEqual(self.stamped('CSPetEquipStrengthAll', fields, login, 6500001)[0], 10001)
                grown = call('CSPetList', {}, login, 6500004, 'SCPetList').petList[0]
                ledger = json.loads(bytes.fromhex(sql('SELECT HEX(costs) FROM p9_pet_investments WHERE guid="' + original_pet.guid + '";').strip()))
                paid = profile(login.playerId)
                checkpoint = {'playerId':login.playerId, 'player':paid[0], 'game':paid[1], 'petWire':grown.SerializeToString().hex(), 'investment':ledger,
                    'scope':'actual pet level mutation and signed original one-key transactions; finite isolated preparation; no auth/session retained'}
                checkpoint['ownedRows'] = {}
                for table in ('p9_pet_instances', 'p9_pet_investments', 'p9_pet_states', 'p9_pet_receipts'):
                    columns = [line.split('\t') for line in sql('SHOW COLUMNS FROM `' + table + '`;').splitlines()]
                    self.assertTrue(columns)
                    self.assertTrue(all(re.fullmatch('[a-z_]+', field[0]) for field in columns))
                    parts = []
                    metadata = []
                    for field in columns:
                        name, kind = field[:2]
                        binary_field = 'blob' in kind or 'binary' in kind
                        parts.extend(['"' + name + '"', 'HEX(`' + name + '`)' if binary_field else '`' + name + '`'])
                        metadata.append({'name':name, 'binaryHex':binary_field})
                    rows = sql('SELECT JSON_OBJECT(' + ','.join(parts) + ') FROM `' + table + '` WHERE player_id=' + str(login.playerId) + ';').splitlines()
                    checkpoint['ownedRows'][table] = {'columns':metadata, 'rows':[json.loads(row) for row in rows]}
                (ROOT / 'analysis/p9/one-key-actual-paid-checkpoint-20261014.json').write_text(json.dumps(checkpoint, indent=2))
                reset = call('CSPetReset', {'guid':original_pet.guid}, login, 6500005, 'SCPetReset')
                self.assertEqual(len(reset.get.pets),1)
                self.assertTrue(all(r.strengthLevel == 0 for r in reset.get.pets[0].petEquipList))
                after = profile(login.playerId)
                self.assertEqual(after[0]['coin'],paid[0]['coin']+ledger['coin'])
                self.assertEqual(after[0]['diamond'],paid[0]['diamond']-50)
                self.assertEqual(next(r['num'] for r in after[1]['items'] if r['id']=='item_124'),1000)
                report = {'sourceManifestSHA256':hashlib.sha256((SOURCE/'manifest.json').read_bytes()).hexdigest(),
                    'eightConcurrentOriginalHTTP':'one aggregate charge', 'coldDuplicate':'passed', 'partialSlots':2,
                    'originalMaximumSlots':4, 'reset':'exact actual investment, fresh identity, four base equipment',
                    'allocationPolicy':'named local original-slot round-robin; private original order unknown',
                    'productionModified':False, 'nativeCompared':False, 'authoritative':False}
            finally:
                stop()
                root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
            remaining = root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
            self.assertEqual(remaining.split(), ['0','0','0'])
            report.update(cleanup='0 databases/users/grants', seconds=round(time.monotonic()-began,3))
            (ROOT/'analysis/p9/one-key-mysql-validation-20261014.json').write_text(json.dumps(report,indent=2))
