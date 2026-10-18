"""Prebuilt local wallet -> original payment -> persistent original product.

No compiler, development profile writes, resource fixtures, external payment or
phone action. One owned managed InnoDB allocation and isolated app log only.
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
import subprocess
import tempfile
import time
import types
import unittest

from server.codec import BY_NAME, CLASSES, parse_http
from hyjs_managed_mysql import ManagedMySQL
import test_continued_gameplay_mysql as transport


ROOT = Path(__file__).resolve().parents[1]
SOURCE_ROOT = Path(os.environ.get('HYJS_LOCAL_WALLET_SOURCE_ROOT', ROOT)).expanduser().resolve()
VALIDATION = Path(os.environ.get('HYJS_LOCAL_WALLET_VALIDATION',
    ROOT / 'analysis/growth-first-20261016/wallet-qualification.json')).expanduser().resolve()
REPORT = Path(os.environ.get('HYJS_LOCAL_WALLET_REPORT',
    ROOT / 'analysis/growth-first-20261016/wallet-http-mysql.json')).expanduser().resolve()


def original_rows(name):
    lines = (SOURCE_ROOT / 'analysis/tables' / ('Table' + name + '.tsv')).read_text().splitlines()
    return [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line]


def protocol_preflight():
    products = original_rows('Payment')
    six = next(row for row in products if row['paymentId'] == 'djsy.wuyou.andp0006')
    permanent = [row for row in products if row['paymentType'] == 'permanent'
                 and row['shopId'] == 'common' and 'gf_default' in row['channelIdList'].split(';')]
    vectors = [
        ('CSUserLogin', {'platformUid': 'demo-wallet-offline',
                         'authCode': 'local-test-password', 'clientTime': 1700000000000}),
        ('CSPlayerCreate', {'playerName': '余额真链', 'heroId': 'hero_047'}),
        ('CSGetPaymentInfo', {'channelId': 'AND0003', 'shopId': 'common'}),
        ('CSGenerateOrder', {'paymentId': six['paymentId'], 'money': 6.0,
                             'moneyType': 'RMB', 'channelId': 'AND0003', 'shopId': 'common'}),
        ('CSCheckOrder', {'orderId': 'p3-' + '0' * 32, 'paymentId': six['paymentId']}),
        ('CSIsCardClaimed', {}),
        ('CSClaimCard', {'cardType': 'permanent'}),
    ]
    for name, fields in vectors:
        raw = transport.encode_original_http(name, fields, 2, b'0' * 16, 100)
        request = parse_http(raw)
        if request.opcode != BY_NAME[name] or request.decode() != CLASSES[name](**fields):
            raise AssertionError('Original wallet payment request failed roundtrip: ' + name)
    for name, fields in {'SCGenerateOrder': ['orderId', 'transactionData'],
                         'SCCheckOrder': ['transactionData'],
                         'SCIsCardClaimed': ['monthStatus', 'permanentStatus'],
                         'SCClaimCard': ['get'],
                         'SCGetPaymentInfo': ['paymentInfoList'],
                         'TransactionData': ['payStatus', 'realGetMoney', 'freeDiamond', 'payDiamond']}.items():
        if any(field not in CLASSES[name].DESCRIPTOR.fields_by_name for field in fields):
            raise AssertionError('Original payment response changed: ' + name)
    if (six['money'] != '6.0' or six['moneyType'] != 'RMB'
            or six['charge'] != '60' or six['free'] != '6' or len(permanent) != 1):
        raise AssertionError('Original 6-yuan/permanent product scope changed')
    if (float(permanent[0]['money']) != 98.0 or permanent[0]['charge'] != '980'
            or permanent[0]['free'] != '100'):
        raise AssertionError('Original insufficient permanent-card price changed')
    handler = (SOURCE_ROOT / 'hyjs-server/handler/localWalletHandler.go').read_text()
    requests = (SOURCE_ROOT / 'hyjs-server/model/localWalletRequest.go').read_text()
    for action in ('state', 'add', 'pay'):
        if '"/local-wallet/' + action + '"' not in handler:
            raise AssertionError('Local wallet route missing: ' + action)
    for field in ('playerId', 'sessionHex', 'requestId', 'amountCents', 'orderId'):
        if 'json:"' + field + '"' not in requests:
            raise AssertionError('Local wallet request field missing: ' + field)
    wallet_vectors = [
        {'playerId': 2, 'sessionHex': '00' * 16},
        {'playerId': 2, 'sessionHex': '00' * 16, 'requestId': 'first100', 'amountCents': 10000},
        {'playerId': 2, 'sessionHex': '00' * 16, 'orderId': 'p3-' + '0' * 32},
    ]
    if any(json.loads(json.dumps(vector)) != vector for vector in wallet_vectors):
        raise AssertionError('Wallet JSON request vector did not roundtrip')
    return {'originalRequestVectorsConstructed': len(vectors),
            'walletJSONVectorsConstructed': len(wallet_vectors),
            'sixYuanProduct': six['paymentId'], 'permanentProduct': permanent[0]['paymentId'],
            'externalCharge': False, 'fixtures': 'Original registered and created demo accounts only'}


def binary_preflight():
    if SOURCE_ROOT.parent != (ROOT / 'build/candidates').resolve() or not SOURCE_ROOT.is_dir():
        raise AssertionError('Set HYJS_LOCAL_WALLET_SOURCE_ROOT to a direct fixed candidate')
    if SOURCE_ROOT in REPORT.parents:
        raise AssertionError('Keep wallet report outside the immutable candidate')
    binary = Path(os.environ.get('HYJS_LOCAL_WALLET_BINARY', '')).expanduser()
    if not binary.is_file():
        raise AssertionError('Set HYJS_LOCAL_WALLET_BINARY to the qualified prebuilt app')
    proof = json.loads(VALIDATION.read_text())
    if not proof.get('qualified') or (ROOT / proof['candidate']).resolve() != SOURCE_ROOT:
        raise AssertionError('Wallet source differs from completed qualification')
    manifest = json.loads((SOURCE_ROOT / 'manifest.json').read_text())
    paths = {entry['path'] for entry in manifest['files']}
    required = {'hyjs-server/service/localWalletService.go',
                'hyjs-server/service/paymentDemoProviderService.go',
                'hyjs-server/handler/localWalletHandler.go',
                'hyjs-server/model/localWalletRequest.go',
                'hyjs-server/model/localWalletModel.go',
                'hyjs-server/db/sql/192_local_wallet_balances.sql',
                'hyjs-server/db/sql/192_local_wallet_entries.sql',
                'hyjs-server/db/sql/192_local_wallet_policies.sql'}
    if not required.issubset(paths):
        raise AssertionError('Qualified source omits wallet transaction delivery')
    for name in ['server/codec.py', 'server/protocol/schema.json']:
        if name not in paths or (ROOT / name).read_bytes() != (SOURCE_ROOT / name).read_bytes():
            raise AssertionError('Original codec differs from frozen source: ' + name)
    transport.SOURCE_ROOT = SOURCE_ROOT
    transport.SOURCE = SOURCE_ROOT / 'manifest.json'
    transport.VALIDATION = VALIDATION
    transport.REPORT = REPORT
    prior = os.environ.get('HYJS_CONTINUED_GAMEPLAY_BINARY')
    os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = str(binary.resolve())
    try:
        return transport.binary_source_preflight()
    finally:
        if prior is None:
            os.environ.pop('HYJS_CONTINUED_GAMEPLAY_BINARY', None)
        else:
            os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = prior


@unittest.skipUnless(os.environ.get('HYJS_LOCAL_WALLET_MYSQL_TEST') == '1',
                     'Explicitly enable isolated wallet original HTTP/InnoDB')
class LocalWalletMySQLTests(unittest.TestCase):
    send = transport.ContinuedGameplayMySQLTests.send
    login = transport.ContinuedGameplayMySQLTests.login
    call = transport.ContinuedGameplayMySQLTests.call
    profile = transport.ContinuedGameplayMySQLTests.profile
    stamped = transport.ContinuedGameplayMySQLTests.stamped
    sql = transport.ContinuedGameplayMySQLTests.sql
    stop = transport.ContinuedGameplayMySQLTests.stop

    def startup_rejections(self):
        # A minimal owned config supplies no database credentials. Even if the
        # early feature guard regresses, no development configuration is read.
        folder = Path(self.app_temp.name) / 'startup-negative'
        folder.mkdir()
        config = 'game:\n  enableTestPayments: false\n  enableLocalWallet: false\n'
        for name in ('dev', 'prd'):
            (folder / ('config.' + name + '.yaml')).write_text(config)
        env = {key: value for key, value in os.environ.items()
               if key in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
        guard = '蓝绿包仅允许dev/test并显式启用测试支付'
        cases = [('missing-test-flag', ['--enable-local-wallet']),
                 ('production', ['--env', 'prd', '--enable-local-wallet', '--enable-test-payments'])]
        result = []
        for name, flags in cases:
            process = subprocess.run([self.binary, *flags], cwd=folder, env=env,
                                     capture_output=True, text=True, timeout=10)
            self.assertNotEqual(process.returncode, 0, 'Illegal wallet startup succeeded: ' + name)
            self.assertIn(guard, process.stdout + process.stderr,
                          'Illegal startup did not reject at the pre-database wallet guard: ' + name)
            self.assertEqual(sorted(file.name for file in folder.iterdir()),
                             ['config.dev.yaml', 'config.prd.yaml'])
            result.append({'case': name, 'exitCode': process.returncode,
                           'earlyFeatureGuard': True, 'databaseAllocation': False,
                           'databaseCredentialsSupplied': False, 'newFiles': False})
        return result

    def start(self, wallet=False):
        args = [self.binary, '--http-port', '0', '--tcp-port', '0', '--tables',
                str(SOURCE_ROOT / 'analysis/tables'), '--log-file',
                str(Path(self.app_temp.name) / 'app.log')]
        if wallet:
            args += ['--enable-test-payments', '--enable-local-wallet']
        self.process = subprocess.Popen(args, cwd=SOURCE_ROOT / 'hyjs-server', env=self.env,
                                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), 'Owned wallet app startup timed out')
            line = self.process.stdout.readline()
        ports = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(ports, 'Owned wallet app did not provide loopback ports')
        self.service = types.SimpleNamespace(http_port=int(ports[1]), tcp_port=int(ports[2]))

    def json_request(self, path, body):
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=20)
        try:
            connection.request('POST', path, json.dumps(body), {'Content-Type': 'application/json'})
            response = connection.getresponse()
            raw = response.read()
            try:
                data = json.loads(raw)
            except ValueError:
                data = None
            return response.status, data
        finally:
            connection.close()

    def order(self, login, product, stamp):
        row = next(row for row in original_rows('Payment') if row['paymentId'] == product)
        request = {'paymentId': product, 'money': float(row['money']),
                   'moneyType': row['moneyType'], 'channelId': 'AND0003', 'shopId': row['shopId']}
        return self.call('CSGenerateOrder', request, login, stamp, 'SCGenerateOrder')

    def wallet_body(self, login, fields):
        fields = dict(fields)
        # Python call-site names avoid collisions; wire keys follow the actual model.
        body = {'playerId': login.playerId,
                'sessionHex': fields.pop('sessionKey', bytes(login.sessionKey).hex())}
        if 'requestKey' in fields:
            fields['requestId'] = fields.pop('requestKey')
        body.update(fields)
        return body

    def wallet(self, action, login, **fields):
        route = 'state' if action == 'snapshot' else action
        status, reply = self.json_request('/local-wallet/' + route,
                                         self.wallet_body(login, fields))
        self.assertEqual(status, 200, 'Wallet route failed HTTP status')
        self.assertIsInstance(reply, dict, 'Wallet response must be a JSON object')
        self.assertEqual(reply.get('code'), 0, reply.get('msg'))
        self.assertIsInstance(reply.get('data'), dict)
        return reply['data']

    def wallet_rejected(self, action, login, expected_code=None, **fields):
        route = 'state' if action == 'snapshot' else action
        status, reply = self.json_request('/local-wallet/' + route,
                                         self.wallet_body(login, fields))
        self.assertEqual(status, 200, 'Wallet wrapper error must retain JSON response')
        self.assertIsInstance(reply, dict)
        self.assertIsInstance(reply.get('code'), int)
        self.assertNotEqual(reply['code'], 0)
        if expected_code is not None:
            self.assertEqual(reply['code'], expected_code, reply.get('msg'))
        self.assertIsNone(reply.get('data'))
        return reply

    def original_check(self, login, order_id, product, stamp, expected):
        reply = self.call('CSCheckOrder', {'orderId': order_id, 'paymentId': product},
                          login, stamp, 'SCCheckOrder')
        self.assertEqual(reply.transactionData.payStatus, expected)
        self.assertEqual(reply.transactionData.orderId, order_id)
        return reply

    def assert_view(self, view, cents, revision, entries):
        self.assertEqual((view['balanceCents'], view['revision'], len(view['entries'])),
                         (cents, revision, entries))
        self.assertEqual((view['name'], view['currency'], view['testOnly'], view['version']),
                         ('蓝绿包', 'CNY', True, 'local-demo-wallet-v1'))
        self.assertEqual(sum(row['amountCents'] for row in view['entries']), cents)

    def test_real_wallet_original_recharge_pending_idempotency_and_cold(self):
        preflight = protocol_preflight()
        self.binary, binary_sha, source_sha = binary_preflight()
        if REPORT.exists():
            self.fail('Preserve prior wallet report; choose a fresh HYJS_LOCAL_WALLET_REPORT')
        self.process = None
        self.app_temp = None
        self.pool = ManagedMySQL()
        began = time.monotonic()
        report = {'status': 'RUNNING', 'binarySHA256': binary_sha,
                  'sourceRoot': str(SOURCE_ROOT), 'sourceManifestSHA256': source_sha,
                  'qualification': str(VALIDATION), 'preflight': preflight,
                  'runnerSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                  'phoneUI': False, 'externalPayment': False, 'developmentModified': False,
                  'profileFixtures': False, 'sourceOfFunds': 'Explicit local wallet add API, test only'}
        lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            lock.close()
            self.fail('SQL_BUSY: no allocation, app or temporary directory started')
        try:
            self.app_temp = tempfile.TemporaryDirectory(prefix='hyjs-local-wallet-')
            report['invalidStartupCombinations'] = self.startup_rejections()
            connection = self.pool.allocate('local_wallet')
            self.database = connection.database
            self.env = {key: value for key, value in os.environ.items()
                        if key in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
            self.env.update(connection.env())
            self.env['TZ'] = 'Asia/Shanghai'
            self.start(wallet=False)
            suffix = secrets.token_hex(6)
            protected_uid, uid = 'demo-wallet-protected-' + suffix, 'demo-wallet-' + suffix
            protected = self.login(protected_uid)
            self.assertEqual(protected.playerId, 1, 'Fresh isolated first account is protected1')
            self.call('CSPlayerCreate', {'playerName': '保护真角', 'heroId': 'hero_047'},
                      protected, 1, 'SCPlayerCreate')
            login = self.login(uid)
            self.assertGreater(login.playerId, 1)
            self.call('CSPlayerCreate', {'playerName': '余额真链', 'heroId': 'hero_047'},
                      login, 1, 'SCPlayerCreate')
            protected_before = self.profile(protected)
            player, game = self.profile(login)
            self.assertEqual((player.get('coin', 0), player.get('diamond', 0)), (0, 0))
            self.assertFalse(game.get('items'))
            self.assertFalse(game.get('equipment'))
            self.wallet_rejected('snapshot', login, expected_code=7102)
            self.stop()
            self.start(wallet=True)
            login, protected = self.login(uid), self.login(protected_uid)
            self.assertEqual(self.sql('SELECT enabled FROM local_wallet_policies WHERE id=1;').strip(), '0')
            view = self.wallet('snapshot', login)
            self.assert_view(view, 0, 0, 0)
            self.assertFalse(view['enabled'])
            self.wallet_rejected('add', login, expected_code=7103,
                                 requestKey='closed', amountCents=10000)
            self.sql('UPDATE local_wallet_policies SET enabled=1 WHERE id=1;')
            tables = self.sql("SELECT TABLE_NAME,ENGINE FROM information_schema.TABLES "
                "WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('local_wallet_policies',"
                "'local_wallet_balances','local_wallet_entries','p3_payment_orders',"
                "'p3_payment_confirmations') ORDER BY TABLE_NAME;").strip().splitlines()
            self.assertEqual(len(tables), 5)
            self.assertTrue(all(line.endswith('\tInnoDB') for line in tables))
            self.wallet_rejected('snapshot', login, expected_code=7102, sessionKey='00' * 16)
            self.wallet_rejected('snapshot', login, expected_code=7001, sessionKey='bad')
            self.wallet_rejected('add', protected, expected_code=7001,
                                 requestKey='protected', amountCents=10000)
            self.wallet_rejected('snapshot', protected, expected_code=7001)
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                credits = list(workers.map(lambda _: self.wallet('add', login,
                    requestKey='first100', amountCents=10000), range(8)))
            for credit in credits:
                self.assert_view(credit, 10000, 1, 1)
            self.wallet_rejected('add', login, expected_code=7103,
                                 requestKey='first100', amountCents=10001)
            for amount in (0, -1):
                self.wallet_rejected('add', login, expected_code=7001,
                                     requestKey='invalid', amountCents=amount)
            six = self.order(login, preflight['sixYuanProduct'], 100)
            self.assertEqual(six.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
            self.original_check(login, six.orderId, preflight['sixYuanProduct'], 101,
                                'PAY_STATUS_SUCCESS')
            paid, _ = self.profile(login)
            self.assertEqual((paid['diamond'], paid['chargeDiamond'], paid['chargeCount']), (66, 60, 1))
            self.assert_view(self.wallet('snapshot', login), 9400, 2, 2)
            # Original timestamp/order identity, wallet pay and poll are all repeated.
            duplicate = self.order(login, preflight['sixYuanProduct'], 100)
            self.assertEqual(duplicate.orderId, six.orderId)
            self.assert_view(self.wallet('pay', login, orderId=six.orderId), 9400, 2, 2)
            self.assertEqual(self.profile(login)[0]['diamond'], 66)
            pending = self.order(login, preflight['permanentProduct'], 200)
            self.assertEqual(pending.transactionData.payStatus, 'PAY_STATUS_WAIT')
            self.original_check(login, pending.orderId, preflight['permanentProduct'], 201,
                                'PAY_STATUS_WAIT')
            view = self.wallet('snapshot', login)
            self.assert_view(view, 9400, 2, 2)
            self.assertEqual([row['orderId'] for row in view['pendingOrders']], [pending.orderId])
            self.assertEqual(view['pendingOrders'][0]['amountCents'], 9800)
            self.wallet_rejected('pay', login, expected_code=7103, orderId=pending.orderId)
            self.assertEqual(self.profile(login)[0]['diamond'], 66)
            self.assert_view(self.wallet('add', login, requestKey='second100', amountCents=10000),
                             19400, 3, 3)
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                purchases = list(workers.map(lambda _: self.wallet('pay', login,
                    orderId=pending.orderId), range(8)))
            for purchase in purchases:
                self.assert_view(purchase, 9600, 4, 4)
                self.assertEqual(purchase['pendingOrders'], [])
            self.original_check(login, pending.orderId, preflight['permanentProduct'], 202,
                                'PAY_STATUS_SUCCESS')
            paid, paid_game = self.profile(login)
            self.assertEqual((paid['diamond'], paid['chargeDiamond'], paid['chargeCount'],
                              paid['permanentCardValid']), (1046, 1040, 2, 1))
            self.assertEqual(self.sql('SELECT COUNT(*) FROM local_wallet_entries;').strip(), '4')
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p3_payment_confirmations;').strip(), '2')
            membership = self.call('CSIsCardClaimed', {}, login, 210, 'SCIsCardClaimed')
            self.assertEqual(membership.permanentStatus, 'canClaim')
            claim = self.call('CSClaimCard', {'cardType': 'permanent'}, login, 211, 'SCClaimCard')
            self.assertEqual([(item.id, item.num) for item in getattr(claim, 'get').simps],
                             [('diamond', 100)])
            membership = self.call('CSIsCardClaimed', {}, login, 212, 'SCIsCardClaimed')
            self.assertEqual(membership.permanentStatus, 'alreadyClaim')
            self.assertEqual(self.stamped('CSClaimCard', {'cardType': 'permanent'}, login, 213)[0], 10001)
            paid, paid_game = self.profile(login)
            self.assertEqual((paid['diamond'], paid['chargeDiamond'], paid['chargeCount']), (1146, 1040, 2))
            self.assert_view(self.wallet('snapshot', login), 9600, 4, 4)
            self.assertEqual(self.profile(protected), protected_before)
            before_cold = self.wallet('snapshot', login)
            old_session = bytes(login.sessionKey).hex()
            self.stop()
            self.start(wallet=True)
            login = self.login(uid)
            self.assertEqual(self.profile(login), [paid, paid_game])
            cold = self.wallet('snapshot', login)
            self.assertEqual(cold, before_cold)
            self.wallet_rejected('snapshot', login, expected_code=7102, sessionKey=old_session)
            self.assert_view(self.wallet('add', login, requestKey='first100', amountCents=10000),
                             9600, 4, 4)
            self.assert_view(self.wallet('pay', login, orderId=pending.orderId), 9600, 4, 4)
            self.original_check(login, six.orderId, preflight['sixYuanProduct'], 300,
                                'PAY_STATUS_SUCCESS')
            membership = self.call('CSIsCardClaimed', {}, login, 301, 'SCIsCardClaimed')
            self.assertEqual(membership.permanentStatus, 'alreadyClaim')
            self.assertEqual(self.stamped('CSClaimCard', {'cardType': 'permanent'}, login, 302)[0], 10001)
            self.assertEqual(self.profile(login), [paid, paid_game])
            report.update(status='PASS_LOCAL_WALLET_ORIGINAL_PAYMENT_INNODB_AND_COLD_PHONE_PENDING',
                protected1Unchanged=True, coldRestarts=2, originalOrders=[six.orderId, pending.orderId],
                initialBalanceCents=0, totalCreditsCents=20000, totalPaymentCents=10400,
                finalBalanceCents=9600, ledgerRows=4, originalPaymentDiamond=1046,
                actualPermanentDailyClaimDiamond=100, finalDiamond=1146,
                chargeCount=2, permanentCardValid=1, concurrentCredits=8, concurrentPayRetries=8,
                insufficientKeptPending=True, snapshot=cold)
        except Exception as error:
            report['status'] = 'FAIL_PARTIAL_PREFIX_NOT_FULL_ACCEPTANCE'
            report['failure'] = self.pool.redact(str(error))[:2000]
            raise
        finally:
            try:
                self.stop()
            finally:
                try:
                    self.pool.close()
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    report['ownedAppStopped'] = self.process is None or self.process.poll() is not None
                finally:
                    try:
                        if self.app_temp is not None:
                            self.app_temp.cleanup()
                            report['ownedLogDirectoryRemoved'] = True
                    finally:
                        fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
                        lock.close()
                        report['seconds'] = round(time.monotonic() - began, 3)
                        REPORT.parent.mkdir(parents=True, exist_ok=True)
                        with REPORT.open('x') as output:
                            output.write(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
