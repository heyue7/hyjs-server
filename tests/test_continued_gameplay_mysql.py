"""One prebuilt binary, original HTTP/TCP, owned managed MySQL only.

Explicit finite account balances/levels and HP are test qualifications. This does
not claim naturally earned initial resources, engine damage, or phone UI proof.
No source copy, compiler, old root_sql, or database administration is used here.
"""
import concurrent.futures
import fcntl
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import socket
import subprocess
import tempfile
import time
import types
import unittest

from server.codec import BY_NAME, CLASSES, encode_http_request, decode_responses, parse_http
from hyjs_managed_mysql import ManagedMySQL
import test_p10_mysql as transport


ROOT = Path(__file__).resolve().parents[1]
SOURCE_ROOT = Path(os.environ.get('HYJS_CONTINUED_GAMEPLAY_SOURCE_ROOT', ROOT)).expanduser().resolve()
VALIDATION = Path(os.environ.get('HYJS_CONTINUED_GAMEPLAY_VALIDATION',
                                ROOT / 'analysis/continued-batch-20261016-validation.json')).expanduser().resolve()
SOURCE = Path(os.environ.get('HYJS_CONTINUED_GAMEPLAY_SOURCE_MANIFEST',
                            SOURCE_ROOT / 'manifest.json' if SOURCE_ROOT != ROOT else
                            ROOT / 'analysis/continued-batch-20261016-final-source.json')).expanduser().resolve()
DEFAULT_REPORT = ROOT / 'analysis/continued-batch-20261016-protocol-validation.json'
REPORT = Path(os.environ.get('HYJS_CONTINUED_GAMEPLAY_REPORT', DEFAULT_REPORT)).expanduser().resolve()


def encode_original_http(name, fields, player_id, session, stamp):
    # Direct descriptor construction permits original fields also named "name".
    body = CLASSES[name](**fields)
    return encode_http_request(BY_NAME[name], body, player_id=player_id,
                               zone_id=1, session=session, timestamp=stamp)


def protocol_preflight():
    """Read recovered descriptors before allocating any database or app process."""
    fields = {
        'CSPlayerCreate': ['playerName', 'heroId'], 'CSUnionCreate': ['name'],
        'CSUnionDonate': ['donateType'], 'CSUnionOpenDonateBox': ['boxId'],
        'CSGetUnionInfo': [], 'CSUnionStore': [], 'CSTransferScene': ['target'],
        'CSSocketLogin': ['sessionKey'], 'CSLoginScene': [],
        'CSCurrentPosition': [], 'CSGetHp': ['playerId'],
        'CSTransferLine': ['line'],
        'SCUnionStore': ['boxes', 'currentScore'],
        'SCUnionOpenDonateBox': ['get'], 'SCGetHp': ['hp'],
    }
    for name, required in fields.items():
        if name not in BY_NAME or name not in CLASSES:
            raise AssertionError('Recovered original message missing: ' + name)
        known = CLASSES[name].DESCRIPTOR.fields_by_name
        if any(field not in known for field in required):
            raise AssertionError('Recovered original fields changed: ' + name)
    if BY_NAME['CSUnionOpenDonateBox'] != 57125 or BY_NAME['SCUnionOpenDonateBox'] != 57126:
        raise AssertionError('Original guild donate box opcode changed')
    vectors = [
        ('CSUserLogin', {'platformUid': 'continued-preflight',
                         'authCode': 'isolated-preflight', 'clientTime': 1700000000000}),
        ('CSPlayerCreate', {'playerName': '合批真角0', 'heroId': 'hero_047'}),
        ('CSUnionCreate', {'name': '捐箱真链'}),
        ('CSGetUnionInfo', {}), ('CSUnionStore', {}),
        ('CSUnionDonate', {'donateType': 'coin'}),
        ('CSUnionDonate', {'donateType': 'badge'}),
        ('CSUnionOpenDonateBox', {'boxId': '0' * 64}),
        ('CSTransferScene', {'target': 'field_001'}),
        ('CSSocketLogin', {'sessionKey': b'0' * 16}),
        ('CSLoginScene', {}), ('CSCurrentPosition', {}),
        ('CSGetHp', {'playerId': 2}),
        ('CSTransferLine', {'line': 2}), ('CSTransferLine', {'line': 1}),
    ]
    for name, values in vectors:
        raw = encode_original_http(name, values, 1, b'0' * 16, 1700000000000)
        decoded = parse_http(raw)
        if decoded.opcode != BY_NAME[name] or decoded.decode() != CLASSES[name](**values):
            raise AssertionError('Original request vector failed roundtrip: ' + name)
    return {'messagesChecked': len(fields), 'requestVectorsConstructed': len(vectors),
            'guildClaimOpcode': 57125, 'nameFieldRequestEncoded': True}


def binary_source_preflight():
    """Pin the caller-supplied existing executable to Root's completed batch."""
    if 'HYJS_CONTINUED_GAMEPLAY_SOURCE_ROOT' in os.environ:
        if not SOURCE_ROOT.is_dir() or SOURCE_ROOT.parent != (ROOT / 'build/candidates').resolve():
            raise AssertionError('Selected source root must be a direct project build/candidates directory')
    if SOURCE_ROOT != ROOT and REPORT == DEFAULT_REPORT:
        raise AssertionError('Candidate validation requires a separate HYJS_CONTINUED_GAMEPLAY_REPORT')
    binary = Path(os.environ.get('HYJS_CONTINUED_GAMEPLAY_BINARY', '')).expanduser()
    if not binary.is_file():
        raise AssertionError('HYJS_CONTINUED_GAMEPLAY_BINARY must name the prebuilt service')
    binary = binary.resolve()
    validation = json.loads(VALIDATION.read_text())
    expected = validation.get('binarySHA256') or validation.get('hyjs-serverSHA256')
    digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    if not isinstance(expected, str) or len(expected) != 64 or digest != expected:
        raise AssertionError('Prebuilt binary SHA differs from completed batch validation')
    source = json.loads(SOURCE.read_text())
    entries = source.get('files')
    if not isinstance(entries, list) or not entries:
        raise AssertionError('Current source manifest has no files')
    listed = set()
    for entry in entries:
        path = (SOURCE_ROOT / entry['path']).resolve()
        if SOURCE_ROOT not in path.parents or path in listed:
            raise AssertionError('Source manifest has duplicate or out-of-project paths')
        listed.add(path)
        if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != entry['sha256']:
            raise AssertionError('Current source differs from fixed source: ' + entry['path'])
    if set((SOURCE_ROOT / 'hyjs-server').rglob('*.go')) - listed:
        raise AssertionError('Current Go source has files absent from fixed source')
    return str(binary), digest, hashlib.sha256(SOURCE.read_bytes()).hexdigest()


@unittest.skipUnless(os.environ.get('HYJS_CONTINUED_GAMEPLAY_MYSQL_TEST') == '1',
                     'Explicitly enable the managed continued-gameplay batch')
class ContinuedGameplayMySQLTests(unittest.TestCase):
    # Reuse only transport operations. This class deliberately has no root_sql,
    # old allocator, old setUpClass, source overlay, or build methods.
    send = transport.P10MySQLTests.send
    login = transport.P10MySQLTests.login
    call = transport.P10MySQLTests.call
    profile = transport.P10MySQLTests.profile
    recv = transport.P10MySQLTests.recv
    tcp_send = transport.P10MySQLTests.tcp_send
    stop = transport.P10MySQLTests.stop

    def stamped(self, name, fields, login, stamp):
        raw = encode_original_http(name, fields, login.playerId,
                                   bytes(login.sessionKey), stamp)
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port,
                                                timeout=20)
        try:
            connection.request('POST', '/sao/', body=raw,
                               headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            data = response.read()
            self.assertEqual(response.status, 200)
            return decode_responses(data)[0]
        finally:
            connection.close()

    def start(self):
        # Only this run's owned application log lives in its disposable directory.
        args = [self.binary, '--http-port', '0', '--tcp-port', '0', '--tables',
                str(SOURCE_ROOT / 'analysis/tables'), '--log-file',
                str(Path(self.app_temp.name) / 'app.log')]
        self.process = subprocess.Popen(args, cwd=SOURCE_ROOT / 'hyjs-server', env=self.env,
                                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                        text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), 'Owned continued-gameplay app startup timed out')
            line = self.process.stdout.readline()
        ports = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(ports, 'Owned app did not provide loopback listener ports')
        self.service = types.SimpleNamespace(http_port=int(ports[1]), tcp_port=int(ports[2]))

    def sql(self, text):
        return self.pool.sql(self.database, text)

    def write_profile(self, login, player, game):
        player_hex = json.dumps(player, separators=(',', ':')).encode().hex()
        game_hex = json.dumps(game, separators=(',', ':')).encode().hex()
        self.sql(f'UPDATE profiles SET player=CONVERT(0x{player_hex} USING utf8mb4),'
                 f'game=CONVERT(0x{game_hex} USING utf8mb4) WHERE id={int(login.playerId)};')

    def connect(self, login):
        sock = socket.create_connection(('127.0.0.1', self.service.tcp_port), timeout=3)
        self.sockets.append(sock)
        self.tcp_send(sock, 'CSSocketLogin', {'sessionKey': bytes(login.sessionKey)}, login)
        opcode, response = self.recv(sock)
        self.assertEqual(opcode, BY_NAME['SCSocketLogin'])
        self.assertEqual(response.result, 100)
        return sock

    def tcp_call(self, sock, login, name, fields, expected):
        self.tcp_send(sock, name, fields, login)
        # Scene entry can queue ordinary peer/monster/side pushes before the ack.
        for _ in range(40):
            opcode, response = self.recv(sock)
            self.frame_opcodes.append(opcode)
            self.assertNotEqual(opcode, 10001, 'Original TCP request rejected: ' + name)
            if opcode == BY_NAME[expected]:
                return response
        self.fail('Original TCP acknowledgement missing: ' + expected)

    def close_sockets(self):
        for sock in self.sockets:
            try:
                sock.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            sock.close()
        self.sockets.clear()

    def test_original_guild_boxes_and_visible_hp_managed_batch(self):
        # All read-only contract/hash checks precede service allocation.
        preflight = protocol_preflight()
        self.binary, binary_sha, source_sha = binary_source_preflight()
        report = {'status': 'RUNNING', 'binarySHA256': binary_sha,
                  'sourceManifestSHA256': source_sha, 'preflight': preflight,
                  'sourceRoot': str(SOURCE_ROOT), 'sourceManifest': str(SOURCE),
                  'binaryValidation': str(VALIDATION),
                  'phoneUI': False, 'productionModified': False,
                  'finitePrerequisites': 'two real created accounts; level30/diamond1000/'
                  'coin20000000/item089x50; explicit HP ratios and outside-safe point '
                  'are isolated state fixtures, not natural damage or earned source',
                  'runnerSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
        self.process = None
        self.sockets = []
        self.frame_opcodes = []
        self.pool = ManagedMySQL()
        self.app_temp = None
        began = time.monotonic()
        lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            lock.close()
            self.fail('SQL_BUSY: no managed allocation, temporary directory or application started')
        try:
            self.app_temp = tempfile.TemporaryDirectory(prefix='hyjs-continued-gameplay-')
            connection = self.pool.allocate('continued_gameplay')
            self.database = connection.database
            # Whitelist process essentials; no inherited database account or test-time policy flags.
            self.env = {key: value for key, value in os.environ.items()
                        if key in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
            self.env.update(connection.env())
            self.env['TZ'] = 'Asia/Shanghai'
            self.start()
            suffix = secrets.token_hex(5)
            uids = ['continued-gameplay-' + suffix + '-' + str(i) for i in range(2)]
            accounts = [self.login(uid) for uid in uids]
            a, b = accounts
            for index, account in enumerate(accounts):
                self.call('CSPlayerCreate', {'playerName': '合批真角' + str(index),
                                            'heroId': 'hero_047'}, account, 1, 'SCPlayerCreate')
                player, game = self.profile(account)
                player.update(level=30, diamond=1000, coin=20000000)
                game['items'] = [item for item in (game.get('items') or []) if item.get('id') != 'item_089']
                game['items'].append({'id': 'item_089', 'num': 50})
                self.write_profile(account, player, game)
            self.sql('UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=1;')
            self.call('CSUnionCreate', {'name': '捐箱真链'}, a, 10, 'SCUnionCreate')
            self.assertEqual(self.profile(a)[0]['diamond'], 700)
            self.sql('UPDATE p13_guild_donate_box_policies SET enabled=1,'
                     'published_at=CAST(UNIX_TIMESTAMP(NOW(3))*1000 AS UNSIGNED)-1000 WHERE id=1;')
            for index in range(10):
                self.call('CSUnionDonate', {'donateType': 'coin'}, a, 100 + index, 'SCUnionDonate')
            for index in range(30):
                self.call('CSUnionDonate', {'donateType': 'badge'}, a, 200 + index, 'SCUnionDonate')
            store = self.call('CSUnionStore', {}, a, 400, 'SCUnionStore')
            self.assertEqual(store.currentScore, 250)
            self.assertEqual([box.score for box in store.boxes], [250, 400, 500, 600])
            self.assertEqual([box.state for box in store.boxes], [1, 0, 0, 0])
            self.assertEqual([box.reward for box in store.boxes],
                             ['coin,1000', 'coin,2000', 'coin,3000', 'coin,5000'])
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_guild_donate_box_sources;').strip(), '40')
            request = {'boxId': store.boxes[0].boxId}
            before = self.profile(a)[0]['coin']
            with concurrent.futures.ThreadPoolExecutor(max_workers=6) as workers:
                claims = list(workers.map(lambda _: self.call('CSUnionOpenDonateBox', request,
                                                            a, 500, 'SCUnionOpenDonateBox'), range(6)))
            self.assertTrue(all(reply == claims[0] for reply in claims))
            self.assertEqual([(item.id, item.num) for item in claims[0].get.simps], [('coin', 1000)])
            self.assertEqual(self.profile(a)[0]['coin'], before + 1000)
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_guild_donate_box_claims;').strip(), '1')
            self.assertEqual(self.stamped('CSUnionOpenDonateBox', request, a, 501)[0], 10001)
            self.sql('UPDATE p13_guild_donate_box_policies SET enabled=0 WHERE id=1;')
            self.call('CSUnionDonate', {'donateType': 'coin'}, a, 502, 'SCUnionDonate')
            self.assertEqual(self.profile(a)[0]['coin'], before + 1000 - 100000)
            frozen = self.call('CSUnionStore', {}, a, 503, 'SCUnionStore')
            self.assertEqual(frozen.currentScore, 250)
            self.assertEqual(frozen.boxes[0].state, 2)
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_guild_donate_box_sources;').strip(), '40')
            report['guild'] = {'actualCoinDonations': 11, 'actualBadgeDonations': 30,
                               'creditedDonationSources': 40, 'score': 250,
                               'claimRequestsSameStamp': 6, 'actualClaims': 1,
                               'actualAwardCoin': 1000, 'actualSubsequentCoinSpend': 100000,
                               'sourceClosedStillShowsClaim': True}

            # Both peers perform actual original scene entry and TCP room-loaded handshake.
            self.sql('UPDATE p10_scene_policies SET enabled=1,line_count=2,max_players=100 WHERE id=1;')
            sockets = []
            for index, account in enumerate(accounts):
                self.call('CSTransferScene', {'target': 'field_001'}, account,
                          600 + index, 'SCTransferScene')
                sock = self.connect(account)
                self.tcp_call(sock, account, 'CSLoginScene', {}, 'SCLoginScene')
                self.tcp_call(sock, account, 'CSCurrentPosition', {}, 'SCCurrentPosition')
                sockets.append(sock)
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_scene_presence WHERE '
                                     'scene="field_001" AND line=1 AND session_hash<>"";').strip(), '2')
            for hp in [0.375, 0.0]:
                player, game = self.profile(b)
                game['field']['side']['curHp'] = hp
                game['field']['side']['state'] = 0
                game['field']['side'].pop('battlePosHps', None)
                game['position']['current'] = {'x': 1000, 'y': 0, 'z': 1000}
                self.write_profile(b, player, game)
                saved = self.profile(b)
                for repeat in range(2):
                    reply = self.call('CSGetHp', {'playerId': b.playerId}, a,
                                      700 + repeat, 'SCGetHp')
                    self.assertAlmostEqual(reply.hp, hp)
                    self.assertEqual(self.profile(b), saved)
            self.call('CSTransferLine', {'line': 2}, b, 710, 'SCTransferLine')
            self.assertEqual(self.stamped('CSGetHp', {'playerId': b.playerId}, a, 711)[0], 10001)
            self.call('CSTransferLine', {'line': 1}, b, 712, 'SCTransferLine')
            self.assertEqual(self.call('CSGetHp', {'playerId': b.playerId}, a, 713, 'SCGetHp').hp, 0)
            sockets[1].shutdown(socket.SHUT_RDWR)
            sockets[1].close()
            deadline = time.monotonic() + 4
            while self.sql('SELECT session_hash FROM p10_scene_presence WHERE player_id='
                           + str(b.playerId) + ';').strip():
                self.assertLess(time.monotonic(), deadline, 'Owned socket disconnect did not revoke presence')
                time.sleep(0.05)
            self.assertEqual(self.stamped('CSGetHp', {'playerId': b.playerId}, a, 714)[0], 10001)
            report['hp'] = {'actualTCPPeers': 2, 'field': 'field_001',
                            'currentStoredRatios': [0.375, 0], 'readDoesNotMutateTarget': True,
                            'foreignLineRejected': True, 'disconnectedPeerRejected': True}

            # Reuse the same executable and database; sessions genuinely rotate on login.
            self.close_sockets()
            self.stop()
            self.start()
            accounts = [self.login(uid) for uid in uids]
            a, b = accounts
            for account in accounts:
                sock = self.connect(account)
                self.tcp_call(sock, account, 'CSLoginScene', {}, 'SCLoginScene')
                self.tcp_call(sock, account, 'CSCurrentPosition', {}, 'SCCurrentPosition')
            self.assertEqual(self.call('CSGetHp', {'playerId': b.playerId}, a, 720, 'SCGetHp').hp, 0)
            self.assertEqual(self.call('CSUnionOpenDonateBox', request, a, 500,
                                       'SCUnionOpenDonateBox'), claims[0])
            self.assertEqual(self.profile(a)[0]['coin'], before + 1000 - 100000)
            self.assertEqual(self.call('CSUnionStore', {}, a, 721, 'SCUnionStore').boxes[0].state, 2)
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_guild_donate_box_claims;').strip(), '1')
            report['coldRestarts'] = 1
            report['hp']['coldReconnectedRatio'] = 0
            report['guild']['coldSameStampResponseExact'] = True
            engines = self.sql('SELECT TABLE_NAME,ENGINE FROM information_schema.tables '
                               f'WHERE table_schema="{self.database}" AND TABLE_NAME IN '
                               '("profiles","p13_guild_donate_box_days","p13_guild_donate_box_sources",'
                               '"p13_guild_donate_box_claims","p10_scene_presence");')
            self.assertEqual(len(engines.strip().splitlines()), 5)
            self.assertTrue(all(line.endswith('InnoDB') for line in engines.strip().splitlines()))
            report['engines'] = engines.strip().splitlines()
            report['status'] = 'PASS_ORIGINAL_HTTP_TCP_INNODB_GUILD_BOX_AND_VISIBLE_HP_PHONE_PENDING'
        except Exception as error:
            report['status'] = 'FAIL_PARTIAL_PREFIX_NOT_FULL_ACCEPTANCE'
            report['failure'] = self.pool.redact(str(error))[:2000]
            raise
        finally:
            self.close_sockets()
            try:
                self.stop()
            except Exception as error:
                report['status'] = 'FAIL_OWNED_APP_CLEANUP'
                report['cleanupFailure'] = self.pool.redact(str(error))[:1000]
                raise
            finally:
                try:
                    self.pool.close()
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    report['ownedAppStopped'] = self.process is None or self.process.poll() is not None
                except Exception as error:
                    report['status'] = 'FAIL_OWNED_MANAGED_CLEANUP'
                    report['cleanupFailure'] = self.pool.redact(str(error))[:1000]
                    raise
                finally:
                    try:
                        if self.app_temp is not None:
                            self.app_temp.cleanup()
                            report['ownedLogDirectoryRemoved'] = True
                    finally:
                        fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
                        lock.close()
                        report['seconds'] = round(time.monotonic() - began, 3)
                        report['frameOpcodes'] = self.frame_opcodes
                        REPORT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
