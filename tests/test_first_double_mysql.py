"""Fixed prebuilt managed HTTP: original SKU lifetime double + real wallet.

Only isolated operator policy switches are prepared. All account levels,
diamonds, VIP, order snapshots and receipts come from original business calls.
No compiler, phone, production schema or external payment is used.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import tempfile
import time
import types
import unittest
import uuid

from hyjs_managed_mysql import ManagedMySQL
from server.codec import CLASSES, parse_http
import test_continued_gameplay_mysql as wire
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_p5_talent_scope_mysql import original_table

ROOT = Path(__file__).resolve().parents[1]
SQL_COLUMNS = {
    'profiles': {'id', 'player', 'game', 'session'},
    'first_double_policies': {'id', 'enabled', 'version', 'source'},
    'local_wallet_policies': {'id', 'enabled'},
    'local_play_policies': {'id', 'enabled'},
    'local_wallet_entries': {'player_id', 'key', 'kind', 'delta_cents', 'balance_after', 'revision', 'at'},
    'local_wallet_balances': {'player_id', 'balance_cents', 'revision'},
    'p3_payment_orders': {'id', 'player_id', 'product_id', 'snapshot', 'status', 'cents', 'credited', 'created_at', 'confirmed_at', 'expires_at'},
    'p3_payment_products': {'id', 'first_double', 'cents', 'charge', 'free', 'reward', 'source', 'enabled', 'kind', 'currency', 'channels', 'shop'},
    'local_play_activity_bindings': {'player_id', 'activity_id', 'cycle_id', 'revision'},
}


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def protocol_preflight():
    vectors = {
        'CSGetPaymentInfo': {'channelId': 'AND0003', 'shopId': 'common'},
        'CSGenerateOrder': {'paymentId': 'djsy.wuyou.andp0006', 'money': 6.0,
                            'moneyType': 'RMB', 'channelId': 'AND0003', 'shopId': 'common'},
        'CSCheckOrder': {'paymentId': 'djsy.wuyou.andp0006', 'orderId': 'original-order'},
        'CSCopyBoonInfo': {'activityId': 'RechangeDouble'},
    }
    for name, fields in vectors.items():
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 195001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original request changed: ' + name)
    for name, fields in {
        'PaymentInfo': {'paymentId', 'chargeType', 'giftRate', 'charge', 'free'},
        'SCCopyBoonInfo': {'activityId', 'information'},
        'SCGenerateOrder': {'orderId', 'transactionData'},
    }.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original response changed: ' + name)
    payments = {row['paymentId']: row for row in original_table('Payment')}
    used = {}
    for sku, cents, charge, free in [('djsy.wuyou.andp0006', 600, 60, 6),
                                      ('djsy.wuyou.andp0030', 3000, 300, 30),
                                      ('djsy.wuyou.andp0050', 5000, 500, 55)]:
        row = payments[sku]
        if (round(float(row['money']) * 100), int(row['charge']), int(row['free']), row['paymentType']) != (cents, charge, free, 'once'):
            raise AssertionError('Original SKU economics changed')
        used[sku] = {'cents': cents, 'charge': charge, 'free': free}
    return {'originalRequestVectors': len(vectors), 'originalResponseContracts': 3, 'SKUs': used}


def preflight():
    contracts = protocol_preflight()
    source = Path(os.environ['HYJS_FIRST_DOUBLE_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_FIRST_DOUBLE_BINARY']).resolve()
    proof = json.loads(Path(os.environ['HYJS_FIRST_DOUBLE_VALIDATION']).read_text())
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use direct immutable candidate')
    manifest = source / 'manifest.json'
    paths = set()
    for item in json.loads(manifest.read_text())['files']:
        path = (source / item['path']).resolve()
        if source not in path.parents or path in paths or sha(path) != item['sha256']:
            raise AssertionError('Candidate pin changed: ' + item['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Unpinned Go source')
    if proof.get('qualified') is not True or Path(proof.get('candidate', '')).resolve() != source:
        raise AssertionError('Completed exact candidate qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Reuse qualified binary without compiling')
    for relative in ['analysis/tables/TablePayment.tsv', 'analysis/tables/TableVipPerks.tsv',
                     'server/codec.py', 'server/protocol/schema.json']:
        if sha(source / relative) != sha(ROOT / relative):
            raise AssertionError('Runtime asset differs: ' + relative)
    hooks = {
        'hyjs-server/main.go': ['s.InitializeFirstDouble(ctx)'],
        'hyjs-server/service/paymentService.go': ['s.firstDoubleProduct(tx, id, &product)', 's.firstDoublePaymentInfo(ctx, id, p)',
                                                 'credited += product.Charge', 'p.GetChargeDiamond() + product.Charge'],
        'hyjs-server/service/firstDoubleService.go': ['product.FirstDouble = count == 0', 'return "first", 1, nil',
                                                    'constUtil.P3PaymentConfirmed', 's.firstDoubleOriginalSKU(product)'],
        'hyjs-server/service/localWalletService.go': ['Kind: "debit", DeltaCents: -order.Cents', 'Kind: "add", DeltaCents: cents'],
        'hyjs-server/service/localPlayActivitiesService.go': ['action != "open" && action != "refresh"'],
    }
    contracts['serviceSHA256'] = {}
    for relative, needles in hooks.items():
        path = source / relative
        if any(needle not in path.read_text() for needle in needles):
            raise AssertionError('Required fixed production contract missing: ' + relative)
        contracts['serviceSHA256'][relative] = sha(path)
    constants = (source / 'hyjs-server/constUtil/firstDouble.go').read_text()
    values = {}
    for name in ('FirstDoubleVersion', 'FirstDoubleSource'):
        match = re.search(r'^const ' + name + r' = (".*")$', constants, re.M)
        if match is None:
            raise AssertionError('Missing named policy constant')
        values[name] = json.loads(match[1])
    return source, binary, sha(manifest), contracts, values


@unittest.skipUnless(os.environ.get('HYJS_FIRST_DOUBLE_MYSQL_TEST') == '1', 'Root coordinated managed/prebuilt SQL window')
class FirstDoubleMySQLTests(unittest.TestCase):
    send = transport.send
    login = transport.login
    call = transport.call
    stamped = transport.stamped
    profile = transport.profile
    start = transport.start
    stop = transport.stop
    sql = transport.sql
    json_request = transport.json_request
    wallet_body = transport.wallet_body
    wallet = transport.wallet
    order = transport.order
    tools = transport.tools
    change = transport.change

    def schema_preflight(self):
        found = {}
        for table, required in SQL_COLUMNS.items():
            columns = self.sql("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() "
                               "AND TABLE_NAME='" + table + "' ORDER BY ORDINAL_POSITION;").split()
            self.assertTrue(required <= set(columns), 'Actual schema differs: ' + table)
            found[table] = columns
        return found

    def policy(self, enabled):
        row = self.sql('SELECT enabled,version,HEX(source) FROM first_double_policies WHERE id=1;').strip().split('\t')
        self.assertEqual(row[1:], [self.policy_values['FirstDoubleVersion'], self.policy_values['FirstDoubleSource'].encode().hex().upper()])
        self.pool.sql(self.database, 'UPDATE first_double_policies SET enabled=' + ('1' if enabled else '0') + ' WHERE id=1;')
        self.assertEqual(self.sql('SELECT enabled,version,HEX(source) FROM first_double_policies WHERE id=1;').strip().split('\t'),
                         ['1' if enabled else '0', *row[1:]])

    def stored_order(self, order_id):
        row = self.sql("SELECT id,product_id,status,cents,credited,created_at,confirmed_at,expires_at,HEX(snapshot) "
                       "FROM p3_payment_orders WHERE id='" + order_id + "';").strip().split('\t')
        return dict(orderId=row[0], productId=row[1], status=row[2], cents=int(row[3]), credited=int(row[4]),
                    createdAt=int(row[5]), confirmedAt=int(row[6]), expiresAt=int(row[7]),
                    snapshot=json.loads(bytes.fromhex(row[8])))

    def catalogue(self, account):
        result = self.call('CSGetPaymentInfo', {'channelId': 'AND0003', 'shopId': 'common'}, account, 0, 'SCGetPaymentInfo')
        return {item.paymentId: item for item in result.paymentInfoList}

    def binding(self, account):
        return self.sql('SELECT cycle_id,revision FROM local_play_activity_bindings '
                        f"WHERE player_id={account.playerId} AND activity_id='RechangeDouble';").strip()

    def test_original_first_ever_double_wallet_pending_parallel_calendar_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contracts, self.policy_values = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_FIRST_DOUBLE_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/first-double'))
        self.assertFalse(output.exists(), 'Preserve previous run evidence')
        self.pool = ManagedMySQL()
        self.process = None
        report = dict(status='PENDING', fullChain=False, binarySHA256=sha(binary), sourceManifestSHA256=manifest_sha,
                      runnerSHA256=sha(__file__), contracts=contracts, externalPayment=False, phoneUI=False,
                      productionModified=False, finiteProfilePreparation=False)
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-first-double-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('first_double')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    report['schemaPreflight'] = self.schema_preflight()
                    self.assertEqual(self.sql('SELECT enabled FROM first_double_policies WHERE id=1;').strip(), '0')
                    products_query = ('SELECT id,first_double,cents,charge,free,reward,source,enabled,kind,currency,channels,shop '
                                      'FROM p3_payment_products ORDER BY id;')
                    products_before = self.sql(products_query)
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-first-double-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '保护首充', 'heroId': 'hero_043'}, protected, 195001, 'SCPlayerCreate')
                    protected_profile = self.profile(protected)
                    history = self.login('demo-first-double-history-' + suffix)
                    self.call('CSPlayerCreate', {'playerName': '原历史付款', 'heroId': 'hero_043'}, history, 195002, 'SCPlayerCreate')
                    uid = 'demo-first-double-owner-' + suffix
                    owner = self.login(uid)
                    self.call('CSPlayerCreate', {'playerName': '原首充双倍', 'heroId': 'hero_043'}, owner, 195003, 'SCPlayerCreate')
                    self.assertEqual(self.profile(owner)[0].get('level'), 1)
                    self.assertEqual(self.catalogue(owner)['djsy.wuyou.andp0030'].chargeType, 'general')
                    old = self.order(owner, 'djsy.wuyou.andp0006', 195010)
                    self.assertEqual(old.transactionData.payStatus, 'PAY_STATUS_WAIT')
                    old_snapshot = self.stored_order(old.orderId)
                    self.assertFalse(old_snapshot['snapshot']['first_double'])
                    self.wallet('add', history, requestKey=str(uuid.uuid4()), amountCents=3000)
                    historic = self.order(history, 'djsy.wuyou.andp0030', 195011)
                    self.assertEqual(self.stored_order(historic.orderId)['credited'], 330)
                    self.policy(True)
                    self.assertEqual(self.stored_order(old.orderId), old_snapshot)
                    self.assertEqual(self.catalogue(history)['djsy.wuyou.andp0030'].chargeType, 'general')
                    info = self.catalogue(owner)['djsy.wuyou.andp0030']
                    self.assertEqual((info.chargeType, info.giftRate, info.charge, info.free), ('first', 1, 300, 30))
                    pending = []
                    for stamp in (195020, 195021):
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            replies = list(workers.map(lambda _: self.order(owner, 'djsy.wuyou.andp0030', stamp), range(8)))
                        self.assertEqual(len({reply.orderId for reply in replies}), 1)
                        order = self.stored_order(replies[0].orderId)
                        self.assertEqual(order['status'], 'pending')
                        self.assertTrue(order['snapshot']['first_double'])
                        pending.append(order)
                    self.assertNotEqual(pending[0]['orderId'], pending[1]['orderId'])
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM local_wallet_entries WHERE player_id={owner.playerId};").strip(), '0')
                    protected_order = self.order(protected, 'djsy.wuyou.andp0006', 195022)
                    self.assertEqual(protected_order.transactionData.payStatus, 'PAY_STATUS_WAIT')
                    status, rejected = self.json_request('/local-wallet/add', self.wallet_body(protected,
                        {'requestId': str(uuid.uuid4()), 'amountCents': 600}))
                    self.assertEqual(status, 200)
                    self.assertNotEqual(rejected.get('code'), 0)
                    self.assertEqual(self.profile(protected), protected_profile)
                    # Policy stop affects new orders. Accepted pending snapshots
                    # keep their exact amount and lifetime-bonus arbitration.
                    self.policy(False)
                    credit_key = str(uuid.uuid4())
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        credits = list(workers.map(lambda _: self.wallet('add', owner, requestKey=credit_key, amountCents=14600), range(8)))
                    self.assertTrue(all(item['balanceCents'] == 14600 and item['revision'] == 1 for item in credits))
                    self.wallet('pay', owner, orderId=old.orderId)
                    self.assertEqual(self.stored_order(old.orderId)['credited'], 66)
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        list(workers.map(lambda i: self.wallet('pay', owner, orderId=pending[i % 2]['orderId']), range(8)))
                    raced = [self.stored_order(item['orderId']) for item in pending]
                    self.assertEqual(sorted(item['credited'] for item in raced), [330, 630])
                    self.assertTrue(all(item['status'] == 'confirmed' for item in raced))
                    self.policy(True)
                    independent = self.catalogue(owner)['djsy.wuyou.andp0050']
                    self.assertEqual((independent.chargeType, independent.giftRate), ('first', 1))
                    third = self.order(owner, 'djsy.wuyou.andp0050', 195030)
                    self.assertEqual(self.stored_order(third.orderId)['credited'], 1055)
                    # Open then refresh the actual private Type33 period. Its
                    # explanatory endpoint never creates new per-SKU eligibility.
                    def activity(action):
                        result = self.tools(owner, 'activities/apply', action=action,
                                            activityId='RechangeDouble', requestId=str(uuid.uuid4()))
                        outcomes = {item['id']: item for item in result['results']}
                        self.assertEqual(outcomes['RechangeDouble']['status'], 'open')
                        return result
                    activity('open')
                    initial_binding = self.binding(owner)
                    self.assertTrue(initial_binding)
                    self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], 1)
                    activity('refresh')
                    self.assertNotEqual(self.binding(owner), initial_binding)
                    for action in ('previousDay', 'reset'):
                        self.assertEqual(self.change(owner, action)['offsetDays'], 0)
                    final_catalogue = self.catalogue(owner)
                    for sku in ('djsy.wuyou.andp0006', 'djsy.wuyou.andp0030', 'djsy.wuyou.andp0050'):
                        self.assertEqual(final_catalogue[sku].chargeType, 'general')
                    boon = self.call('CSCopyBoonInfo', {'activityId': 'RechangeDouble'}, owner, 195031, 'SCCopyBoonInfo')
                    self.assertEqual(boon.activityId, 'RechangeDouble')
                    self.assertTrue(boon.information)
                    repeated = self.order(owner, 'djsy.wuyou.andp0030', 195032)
                    self.assertEqual(self.stored_order(repeated.orderId)['credited'], 330)
                    player = self.profile(owner)[0]
                    expected_vip = max(int(row['vip']) for row in original_table('VipPerks') if int(row['chargeDiamond']) <= 1460)
                    self.assertEqual((player['diamond'], player['chargeDiamond'], player['chargeCount'], player['vip']), (2411, 1460, 5, expected_vip))
                    self.assertEqual(self.sql(f"SELECT COUNT(*),SUM(cents),SUM(credited) FROM p3_payment_orders WHERE player_id={owner.playerId} AND status='confirmed';").strip(), '5\t14600\t2411')
                    self.assertEqual(self.sql(f"SELECT COUNT(*),SUM(delta_cents) FROM local_wallet_entries WHERE player_id={owner.playerId} AND kind='debit';").strip(), '5\t-14600')
                    wallet_before = self.wallet('snapshot', owner)
                    self.assertEqual((wallet_before['balanceCents'], wallet_before['revision'], len(wallet_before['entries'])), (0, 6, 6))
                    self.assertEqual(self.sql(products_query), products_before)
                    all_orders = [self.stored_order(order_id) for order_id in [old.orderId, *[item['orderId'] for item in pending], third.orderId, repeated.orderId]]
                    before_cold = self.profile(owner)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), before_cold)
                    self.assertEqual(self.wallet('snapshot', owner), wallet_before)
                    self.assertEqual(self.tools(owner, 'time/state')['offsetDays'], 0)
                    self.assertEqual([self.stored_order(item['orderId']) for item in all_orders], all_orders)
                    self.wallet('pay', owner, orderId=raced[0]['orderId'])
                    self.assertEqual(self.wallet('snapshot', owner), wallet_before)
                    self.assertEqual(self.catalogue(owner)['djsy.wuyou.andp0030'].chargeType, 'general')
                    self.assertEqual(self.profile(protected), protected_profile)
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM local_wallet_balances WHERE player_id={protected.playerId};').strip(), '0')
                    self.assertEqual(self.sql('SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() '
                        "AND TABLE_NAME IN ('profiles','first_double_policies','p3_payment_orders','local_wallet_entries');").split(), ['InnoDB'])
                    report.update(status='PASS', fullChain=True, actualWallet={'creditCents': 14600, 'debitCents': 14600,
                        'balanceCents': 0, 'revision': 6, 'ledgerRows': 6}, actualOrders=all_orders,
                        historicalBeforePolicy=self.stored_order(historic.orderId), racedCredited=sorted(item['credited'] for item in raced),
                        actualDiamond=2411, actualChargeDiamond=1460, actualVIP=expected_vip,
                        defaultOff=True, legacyPendingNotUpgraded=True, acceptedPendingHonoredAfterStop=True,
                        perSKUIndependent=True, originalFirstGiftRate=1, paidCatalogueGeneral=True,
                        realPrivateRefreshAndDateNoReset=True, coldRecovery=True, protected1Exact=True,
                        paymentProductsUnchanged=True, zeroProfileResourceInjection=True)
                except BaseException as error:
                    report.update(status='FAIL', failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    self.stop()
                    report['ownedAppStopped'] = self.process is None or self.process.poll() is not None
                    self.pool.close()
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    report['seconds'] = round(time.monotonic() - began, 3)
                    output.parent.mkdir(parents=True, exist_ok=True)
                    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
