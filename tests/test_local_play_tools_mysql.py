"""One qualified binary: personal clock -> real catalogs -> original rewards.

Opt-in only. Uses a new owned managed MySQL allocation, original demo register/
create, original payment backed by explicit test-wallet credit, and read-only
business assertions. Administrative writes enable policies 192/194 and select
the existing original non-red MinLevel1 wheel in this isolated test schema.
No resource/level/profile/date fixture, compiler, phone, or external charge.
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
import unittest
import uuid

from server.codec import BY_NAME, CLASSES, parse_http
from hyjs_managed_mysql import ManagedMySQL
import test_continued_gameplay_mysql as transport
import test_local_wallet_mysql as wallet_transport


ROOT = Path(__file__).resolve().parents[1]
SOURCE_ROOT = Path(os.environ.get('HYJS_LOCAL_PLAY_SOURCE_ROOT', ROOT)).expanduser().resolve()
VALIDATION = Path(os.environ.get('HYJS_LOCAL_PLAY_VALIDATION',
    ROOT / 'analysis/growth-first-20261016/local-play-qualification.json')).expanduser().resolve()
REPORT = Path(os.environ.get('HYJS_LOCAL_PLAY_REPORT',
    ROOT / 'analysis/growth-first-20261016/local-play-http-mysql.json')).expanduser().resolve()


def protocol_preflight():
    """Safe without app/DB: require recovered wire descriptors and actual routes."""
    vectors = [
        ('CSUserLogin', {'platformUid': 'demo-tools-preflight',
                         'authCode': 'local-tools-password', 'clientTime': 1700000000000}),
        ('CSPlayerCreate', {'playerName': '日期活动真链', 'heroId': 'hero_047'}),
        ('CSPlayerLogin', {}), ('CSActivityList', {}), ('CSOpenStateCheck', {}),
        ('CSNewOpenSevenDayByTitle', {'day': 1, 'tap': 1}),
        ('CSNewOpenSevenDayByTitle', {'day': 2, 'tap': 1}),
        ('CSOpenServerActivity', {'dayType': 7}),
        ('CSOpenServerActivity', {'dayType': 14}),
        ('CSGetOpenServerReward', {'itemId': 1}),
        ('CSBigTurnTableInfo', {}), ('CSBigTurnRecord', {}),
        ('CSPlayBigTurn', {'count': 1}),
        ('CSLoginRewardInfo', {}), ('CSRewardLimitSignList', {}),
        ('CSStaminaRefill', {}), ('CSFortuneCatInfo', {}),
        ('CSActivityInfo', {'activityId': 'totalLogin_p1_directory'}),
        ('CSActivityShopInfo', {'activityId': 'limitExchange_p4_catalog_lp-' + '0' * 28}),
        ('CSContinuedRechargeInfo', {'activityId': 'continuedRecharge_lp-' + '0' * 28 + '_local_p3'}),
        ('CSWarriorRecruitGet', {'costType': [1, 2]}),
        ('CSIsCardClaimed', {}), ('CSGetVipBoonInfo', {}), ('CSOpenServerFundInfo', {}),
        ('CSGenerateOrder', {'paymentId': 'djsy.wuyou.andp0006', 'money': 6.0,
                             'moneyType': 'RMB', 'channelId': 'AND0003', 'shopId': 'common'}),
    ]
    for name, fields in vectors:
        raw = transport.encode_original_http(name, fields, 2, b'0' * 16, 100)
        request = parse_http(raw)
        if request.opcode != BY_NAME[name] or request.decode() != CLASSES[name](**fields):
            raise AssertionError('Original request vector changed: ' + name)
    for name, fields in {
        'SCActivityList': ['activityList'], 'SCOpenStateCheck': ['states'],
        'ActivityTab': ['activityId', 'status', 'order', 'name', 'param'],
        'SCActivityInfo': ['activityId', 'itemList', 'startTime', 'endTime', 'rewardEndTime'],
        'ActivityItem': ['index', 'rewardList', 'status', 'left', 'total'],
        'SCActivityShopInfo': ['activityId', 'shopItemList', 'startTime', 'endTime'],
        'ActivityShopItem': ['index', 'getResource', 'costResource', 'status', 'left', 'total'],
        'SCContinuedRechargeInfo': ['activityId', 'itemList', 'bigReward', 'currentIndex', 'status'],
        'SCLoginRewardInfo': ['hasReward', 'items', 'isShow'],
        'LoginRewardItem': ['day', 'rewards', 'state'],
        'SCRewardLimitSignList': ['limitDaySignDatas'],
        'LimitDaySignData': ['signNum', 'allNum', 'list'],
        'LimitSignRewardItem': ['getState', 'rewardId', 'itemKeyId'],
        'SCStaminaRefill': ['dataList'],
        'ActivityStaminaData': ['startTime', 'endTime', 'status'],
        'SCFortuneCatInfo': ['info'],
        'ActivityFortuneCatData': ['todayLeft', 'totalLeft', 'nextGetTime', 'currentTime'],
        'SCWarriorRecruitGet': ['recruit'],
        'WarriorRecruit': ['costType', 'rewardList', 'changeTime', 'freeTime'],
        'SCIsCardClaimed': ['monthStatus', 'permanentStatus'],
        'SCGetVipBoonInfo': ['daily', 'week'],
        'SCOpenServerFundInfo': ['buyStatus', 'buyCount', 'openServerFundList', 'everyoneBenefitList'],
        'SCOpenServerActivity': ['currentDay', 'days', 'activityEndTime', 'getRewardEndTime'],
        'SCNewOpenSevenDayByTitle': ['itemList', 'redTap'],
        'SCGetOpenServerReward': ['get'],
        'SCPlayBigTurn': ['cost', 'rewards', 'count'],
        'SCBigTurnTableInfo': ['isFree', 'score', 'serverTime'],
    }.items():
        if any(field not in CLASSES[name].DESCRIPTOR.fields_by_name for field in fields):
            raise AssertionError('Original response contract changed: ' + name)
    for filename, paths in {
        'localPlayTimeHandler.go': ['time/state', 'time/change'],
        'localPlayActivitiesHandler.go': ['activities/state', 'activities/apply'],
    }.items():
        source = (SOURCE_ROOT / 'hyjs-server/handler' / filename).read_text()
        if any('"/local-play/' + path + '"' not in source for path in paths):
            raise AssertionError('Local play HTTP routes missing: ' + filename)
    catalogue = (SOURCE_ROOT / 'hyjs-server/service/localPlayActivitiesCatalogue.go').read_text()
    definitions = re.findall(r'\{"([^"]+)",\s*"[^"]+",\s*"([^"]*)",', catalogue)
    if not definitions or len({key for key, _ in definitions}) != len(definitions):
        raise AssertionError('Activity catalog must have distinct actual IDs')
    return {'requestVectors': len(vectors), 'catalogue': dict(definitions),
            'administrativeWrites': ['enable local_wallet_policies id=1',
                                     'enable local_play_policies id=1',
                                     'isolated p2_king_rules.WheelPool -> existing turntable_20261011'],
            'profileFixtures': False, 'sourceOfDiamonds': 'Original 6-yuan order and original login rewards'}


def binary_preflight():
    if SOURCE_ROOT.parent != (ROOT / 'build/candidates').resolve():
        raise AssertionError('HYJS_LOCAL_PLAY_SOURCE_ROOT must name a direct fixed candidate')
    if SOURCE_ROOT in REPORT.parents:
        raise AssertionError('Keep runner results outside immutable candidate')
    proof = json.loads(VALIDATION.read_text())
    if not proof.get('qualified') or (ROOT / proof['candidate']).resolve() != SOURCE_ROOT:
        raise AssertionError('Use completed qualification for this exact candidate')
    binary = Path(os.environ.get('HYJS_LOCAL_PLAY_BINARY', '')).expanduser()
    if not binary.is_file():
        raise AssertionError('HYJS_LOCAL_PLAY_BINARY must name the prebuilt qualified app')
    manifest = json.loads((SOURCE_ROOT / 'manifest.json').read_text())
    paths = {row['path'] for row in manifest['files']}
    required = {'hyjs-server/service/localPlayClockService.go',
                'hyjs-server/service/localPlayActivitiesService.go',
                'hyjs-server/service/localPlayActivityScopeService.go',
                'hyjs-server/service/localPlayTurntableReceipt.go',
                'hyjs-server/handler/localPlayTimeHandler.go',
                'hyjs-server/handler/localPlayActivitiesHandler.go',
                'hyjs-server/db/sql/194_local_play_policies.sql',
                'hyjs-server/db/sql/194_local_play_payment_times.sql'}
    if not required.issubset(paths):
        raise AssertionError('Qualified candidate omits actual play-tools delivery')
    for name in ('server/codec.py', 'server/protocol/schema.json'):
        if name not in paths or (ROOT / name).read_bytes() != (SOURCE_ROOT / name).read_bytes():
            raise AssertionError('Runner original codec differs: ' + name)
    transport.SOURCE_ROOT, transport.SOURCE = SOURCE_ROOT, SOURCE_ROOT / 'manifest.json'
    transport.VALIDATION, transport.REPORT = VALIDATION, REPORT
    prior = os.environ.get('HYJS_CONTINUED_GAMEPLAY_BINARY')
    os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = str(binary.resolve())
    try:
        return transport.binary_source_preflight()
    finally:
        if prior is None:
            os.environ.pop('HYJS_CONTINUED_GAMEPLAY_BINARY', None)
        else:
            os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = prior


@unittest.skipUnless(os.environ.get('HYJS_LOCAL_PLAY_MYSQL_TEST') == '1',
                     'Explicit managed play-tools HTTP/SQL window required')
class LocalPlayToolsMySQLTests(unittest.TestCase):
    # Methods only: no inherited testcase, compiler, setup, old root_sql or allocator.
    send = transport.ContinuedGameplayMySQLTests.send
    login = transport.ContinuedGameplayMySQLTests.login
    call = transport.ContinuedGameplayMySQLTests.call
    stamped = transport.ContinuedGameplayMySQLTests.stamped
    profile = transport.ContinuedGameplayMySQLTests.profile
    stop = transport.ContinuedGameplayMySQLTests.stop
    start = wallet_transport.LocalWalletMySQLTests.start
    json_request = wallet_transport.LocalWalletMySQLTests.json_request
    wallet_body = wallet_transport.LocalWalletMySQLTests.wallet_body
    wallet = wallet_transport.LocalWalletMySQLTests.wallet
    order = wallet_transport.LocalWalletMySQLTests.order

    def sql(self, query):
        clean = query.strip()
        if not (clean.upper().startswith('SELECT ') or clean in (
                'UPDATE local_wallet_policies SET enabled=1 WHERE id=1;',
                'UPDATE local_play_policies SET enabled=1 WHERE id=1;',
                "UPDATE p2_king_rules SET wheel_pool='turntable_20261011' WHERE id=1;")):
            raise AssertionError('Runner forbids business fixtures and arbitrary writes')
        return self.pool.sql(self.database, query)

    def scalar(self, query):
        return self.sql(query).strip()

    def tools(self, endpoint, login, success=True, session=None, **fields):
        body = {'playerId': login.playerId,
                'sessionHex': session if session is not None else bytes(login.sessionKey).hex(), **fields}
        status, reply = self.json_request('/local-play/' + endpoint, body)
        self.assertEqual(status, 200)
        self.assertIsInstance(reply, dict)
        if success:
            self.assertEqual(reply.get('code'), 0, reply.get('msg'))
            self.assertIsInstance(reply.get('data'), dict)
            self.assertTrue(reply['data']['testOnly'])
            return reply['data']
        self.assertIsInstance(reply.get('code'), int)
        self.assertNotEqual(reply['code'], 0)
        self.assertIsNone(reply.get('data'))
        return reply['code']

    def change(self, login, action, request_id=None):
        return self.tools('time/change', login, action=action,
                          requestId=request_id or str(uuid.uuid4()))

    def activities(self, login, action='state', activity='all', request_id=None):
        if action == 'state':
            return self.tools('activities/state', login)
        return self.tools('activities/apply', login, action=action, activityId=activity,
                          requestId=request_id or str(uuid.uuid4()))

    def assert_clock(self, view, offset, revision):
        self.assertEqual((view['offsetDays'], view['revision'], view['timezone']),
                         (offset, revision, 'Asia/Shanghai'))
        self.assertEqual(view['gameTimeMs'] - view['realTimeMs'], offset * 86400000)
        self.assertLess(abs(view['realTimeMs'] - int(time.time() * 1000)), 10000)

    def bindings(self, login):
        text = self.scalar('SELECT activity_id,cycle_id,revision FROM local_play_activity_bindings '
                           f'WHERE player_id={int(login.playerId)} ORDER BY activity_id;')
        return {key: (cycle, int(revision)) for key, cycle, revision in
                (row.split('\t') for row in text.splitlines())}

    def tab(self, login, day):
        return self.call('CSNewOpenSevenDayByTitle', {'day': day, 'tap': 1},
                         login, 5000 + day, 'SCNewOpenSevenDayByTitle')

    def assert_open_day(self, login, expected):
        # Original dayType selects the 7/14-day section; current day is a result.
        reply = self.call('CSOpenServerActivity', {'dayType': 7}, login,
                          5100 + expected, 'SCOpenServerActivity')
        self.assertEqual(reply.currentDay, expected)
        return reply

    def claim(self, login, stamp):
        return self.call('CSGetOpenServerReward', {'itemId': 1}, login, stamp,
                         'SCGetOpenServerReward')

    def original_login(self, login):
        return self.call('CSPlayerLogin', {}, login, 4000, 'SCPlayerLogin')

    def assert_selector_response(self, name, fields, reply):
        if 'activityId' in fields and hasattr(reply, 'activityId'):
            self.assertEqual(reply.activityId, fields['activityId'])
        if name in ('CSActivityInfo', 'CSContinuedRechargeInfo'):
            if fields['activityId'] != 'allCopyDouble':
                self.assertTrue(reply.itemList, 'Original info must contain actual reward rows')
            self.assertTrue(all(item.index > 0 for item in reply.itemList))
            if name == 'CSContinuedRechargeInfo':
                self.assertTrue(reply.bigReward)
        elif name == 'CSActivityShopInfo':
            self.assertTrue(reply.shopItemList, 'Original shop must contain actual products')
            self.assertTrue(all(item.index > 0 and item.getResource and item.costResource
                                for item in reply.shopItemList))
        elif name == 'CSLoginRewardInfo':
            self.assertTrue(reply.items)
            self.assertTrue(all(item.day > 0 and item.rewards for item in reply.items))
        elif name == 'CSRewardLimitSignList':
            self.assertTrue(reply.HasField('limitDaySignDatas'))
            self.assertEqual(reply.limitDaySignDatas.allNum, len(reply.limitDaySignDatas.list))
            self.assertGreater(reply.limitDaySignDatas.allNum, 0)
        elif name == 'CSStaminaRefill':
            self.assertTrue(reply.dataList)
            self.assertTrue(all(item.startTime < item.endTime for item in reply.dataList))
        elif name == 'CSFortuneCatInfo':
            self.assertTrue(reply.HasField('info'))
            self.assertGreaterEqual(reply.info.todayLeft, 0)
            self.assertGreaterEqual(reply.info.totalLeft, 0)
        elif name == 'CSWarriorRecruitGet':
            self.assertEqual({item.costType for item in reply.recruit}, {1, 2})
            self.assertTrue(all(item.rewardList and item.changeTime > 0 for item in reply.recruit))
        elif name == 'CSOpenServerFundInfo':
            self.assertTrue(reply.openServerFundList)
            self.assertTrue(reply.everyoneBenefitList)

    def original_selectors(self, login, views, bindings, original_ids):
        """Read each open domain through its recovered native entry, without claims.

        IDs come from the actual immutable cycle snapshot. Nested seasonal and
        Type23 aggregate directories retain their original outer-page routing.
        """
        evidence = []
        welfare = {'login': 'CSLoginRewardInfo', 'limit': 'CSRewardLimitSignList',
                   'meal': 'CSStaminaRefill', 'cat': 'CSFortuneCatInfo'}
        listed = set(original_ids)
        for activity, view in views.items():
            if view['status'] != 'open':
                continue
            cycle = bindings[activity][0]
            snapshot = json.loads(bytes.fromhex(self.scalar(
                f"SELECT HEX(snapshot) FROM local_play_activity_cycles WHERE id='{cycle}';")))
            queries, expected_listed = [], []
            for item in snapshot.get('periods', []):
                if activity in ('Festival', 'Update'):
                    continue
                key = item['id']
                if key.startswith('totalLogin_p1_'):
                    key = 'totalLogin_p1_directory'
                queries.append(('CSActivityInfo', {'activityId': key}))
                expected_listed.append(key)
            for item in snapshot.get('welfare', []):
                if item['kind'] == 'rebate':
                    key = 'consumeRet_p4_' + cycle
                    queries.append(('CSActivityInfo', {'activityId': key}))
                    expected_listed.append(key)
                elif item['kind'] in welfare:
                    queries.append((welfare[item['kind']], {}))
                    if item['kind'] == 'cat':
                        expected_listed.append('fortuneCat_p4_' + cycle)
                else:
                    self.fail('Unknown frozen welfare entry kind: ' + item['kind'])
            if snapshot.get('exchange'):
                key = snapshot['exchange']['id']
                # This original page aggregates the frozen complete bindings,
                # rather than accidentally exposing one child product only.
                self.assertGreater(len(json.loads(bytes.fromhex(self.scalar(
                    f"SELECT HEX(bindings) FROM p4_resource_exchange_directories WHERE id='{key}';")))), 1)
                queries.append(('CSActivityShopInfo', {'activityId': key}))
                expected_listed.append(key)
            elif activity not in ('Festival', 'Update'):
                for item in snapshot.get('commerce', []):
                    queries.append(('CSActivityShopInfo', {'activityId': item['id']}))
                    expected_listed.append(item['id'])
            for item in snapshot.get('payments', []):
                name = 'CSContinuedRechargeInfo' if item['kind'] == 'continued' else 'CSActivityInfo'
                queries.append((name, {'activityId': item['id']}))
                expected_listed.append(item['id'])
            for bundle in snapshot.get('bundles', []):
                reply = self.call('CSActivityList', {'activityId': bundle['id']}, login,
                                  6200, 'SCActivityList')
                self.assertTrue(reply.activityList, 'Real seasonal parent must expose children')
                expected_listed.append(bundle['id'])
                evidence.append({'activity': activity, 'request': 'CSActivityList',
                                 'activityId': bundle['id'],
                                 'childIds': [item.activityId for item in reply.activityList]})
                for item in reply.activityList:
                    name = 'CSActivityShopInfo' if item.activityId.startswith('limitExchange') else 'CSActivityInfo'
                    queries.append((name, {'activityId': item.activityId}))
            if snapshot.get('copy'):
                queries.append(('CSActivityInfo', {'activityId': 'allCopyDouble'}))
            if snapshot.get('rotations'):
                queries.append(('CSWarriorRecruitGet', {'costType': [1, 2]}))
            if snapshot.get('entryOnly'):
                queries.extend({'FirstRecharge': [('CSActivityInfo', {'activityId': 'firstRecharge_local_p3'})],
                                'MonthCard': [('CSIsCardClaimed', {})],
                                'VIPBenefits': [('CSGetVipBoonInfo', {})],
                                'OpenSeverMoney': [('CSOpenServerFundInfo', {})]}[activity])
            if activity == 'BigTurnTable':
                queries.append(('CSBigTurnTableInfo', {}))
                expected_listed.append('bigTurnTable_20261011')
            # OpenHappy uses the separate original HUD/title selector tested below.
            for key in set(expected_listed):
                self.assertIn(key, listed, 'Private original activity is missing from root directory: ' + activity)
            for name, fields in queries:
                reply = self.call(name, fields, login, 6201, 'SC' + name[2:])
                self.assert_selector_response(name, fields, reply)
                evidence.append({'activity': activity, 'request': name, **fields})
        return evidence

    def draw(self, login, stamp, paid):
        before = self.profile(login)[0]
        reply = self.call('CSPlayBigTurn', {'count': 1}, login, stamp, 'SCPlayBigTurn')
        self.assertEqual(reply.count, 1)
        self.assertEqual([(item.id, item.num) for item in reply.cost],
                         [('diamond', 50)] if paid else [])
        gains = {item.id: item.num for item in reply.rewards.simps}
        after = self.profile(login)[0]
        self.assertEqual(after.get('diamond', 0), before.get('diamond', 0)
                         - (50 if paid else 0) + gains.get('diamond', 0))
        self.assertEqual(after.get('coin', 0), before.get('coin', 0) + gains.get('coin', 0))
        self.assertTrue(reply.rewards.SerializeToString(), 'Actual original reward must be present')
        return reply

    def test_personal_clock_real_catalogs_receipts_payment_and_cold(self):
        preflight = protocol_preflight()
        self.binary, binary_sha, source_sha = binary_preflight()
        if REPORT.exists():
            self.fail('Preserve old result; set a fresh HYJS_LOCAL_PLAY_REPORT')
        wallet_transport.SOURCE_ROOT = SOURCE_ROOT
        self.process, self.app_temp = None, None
        self.pool = ManagedMySQL()
        report = {'status': 'RUNNING', 'binarySHA256': binary_sha,
                  'sourceManifestSHA256': source_sha, 'sourceRoot': str(SOURCE_ROOT),
                  'runnerSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                  'preflight': preflight, 'checkpoints': [], 'phoneUI': False,
                  'externalPayment': False, 'profileFixtures': False}
        began = time.monotonic()
        lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            lock.close()
            self.fail('SQL_BUSY: no allocation or app started')
        try:
            self.app_temp = tempfile.TemporaryDirectory(prefix='hyjs-local-play-')
            connection = self.pool.allocate('local_play')
            self.database = connection.database
            self.env = {key: value for key, value in os.environ.items()
                        if key in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
            self.env.update(connection.env())
            self.env['TZ'] = 'Asia/Shanghai'
            self.start(wallet=False)
            suffix = secrets.token_hex(6)
            protected_uid, uid = 'demo-tools-protected-' + suffix, 'demo-tools-' + suffix
            protected, login = self.login(protected_uid), None
            self.assertEqual(protected.playerId, 1)
            self.call('CSPlayerCreate', {'playerName': '保护真角', 'heroId': 'hero_047'},
                      protected, 1, 'SCPlayerCreate')
            login = self.login(uid)
            self.call('CSPlayerCreate', {'playerName': '活动日期真链', 'heroId': 'hero_047'},
                      login, 1, 'SCPlayerCreate')
            protected_before = self.profile(protected)
            initial, initial_game = self.profile(login)
            self.assertEqual(initial.get('diamond', 0), 0)
            self.assertFalse(initial_game.get('items'))
            for route in ('time/state', 'activities/state'):
                self.tools(route, login, success=False)
            self.stop()
            self.start(wallet=True)
            login, protected = self.login(uid), self.login(protected_uid)
            self.assertEqual(self.scalar('SELECT enabled FROM local_play_policies WHERE id=1;'), '0')
            self.tools('time/state', login, success=False)
            self.tools('activities/state', login, success=False)
            self.sql('UPDATE local_play_policies SET enabled=1 WHERE id=1;')
            self.sql('UPDATE local_wallet_policies SET enabled=1 WHERE id=1;')
            for route in ('time/state', 'activities/state'):
                self.tools(route, protected, success=False)
                self.tools(route, login, success=False, session='00' * 16)
                self.tools(route, login, success=False, session='bad')
            for route, fields in (
                    ('time/change', {'action': 'nextDay', 'requestId': str(uuid.uuid4())}),
                    ('activities/apply', {'action': 'open', 'activityId': 'all',
                                          'requestId': str(uuid.uuid4())})):
                self.tools(route, protected, success=False, **fields)
                self.tools(route, login, success=False, session='00' * 16, **fields)
            self.tools('time/change', login, success=False, action='unknown',
                       requestId=str(uuid.uuid4()))
            self.tools('activities/apply', login, success=False, action='open',
                       activityId='unknown', requestId=str(uuid.uuid4()))
            report['checkpoints'].append('fresh-demo/protected1/defaultoff/current-session')
            self.assert_clock(self.tools('time/state', login), 0, 0)
            request_id = str(uuid.uuid4())
            with concurrent.futures.ThreadPoolExecutor(max_workers=4) as workers:
                changed = list(workers.map(lambda _: self.change(login, 'nextDay', request_id), range(4)))
            self.assertTrue(all(view == changed[0] for view in changed))
            self.assert_clock(changed[0], 1, 1)
            self.tools('time/change', login, success=False, action='previousDay', requestId=request_id)
            funding_start = int(time.time() * 1000)
            self.wallet('add', login, requestKey=str(uuid.uuid4()), amountCents=10000)
            real_before = int(time.time() * 1000)
            paid = self.order(login, 'djsy.wuyou.andp0006', 6001)
            self.assertEqual(paid.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
            self.assertEqual(self.profile(login)[0]['diamond'], 66)
            projection = self.scalar('SELECT real_at,game_at,offset_days FROM local_play_payment_times '
                                     f"WHERE order_id='{paid.orderId}';").split('\t')
            self.assertEqual(int(projection[2]), 1)
            self.assertEqual(int(projection[1]) - int(projection[0]), 86400000)
            self.assertGreaterEqual(int(projection[0]), real_before)
            accounting = self.scalar("SELECT created_at,confirmed_at FROM p3_payment_orders "
                                     f"WHERE id='{paid.orderId}';").split('\t')
            self.assertTrue(all(real_before <= int(value) <= int(time.time() * 1000) for value in accounting))
            ledger_query = 'SELECT `key`,delta_cents,balance_after,`at` FROM local_wallet_entries '
            ledger_query += f'WHERE player_id={int(login.playerId)} ORDER BY `at`,`key`;'
            ledger = self.scalar(ledger_query)
            ledger_rows = [row.split('\t') for row in ledger.splitlines()]
            self.assertEqual(sorted(int(row[1]) for row in ledger_rows), [-600, 10000])
            self.assertTrue(all(funding_start <= int(row[3]) <= int(time.time() * 1000)
                                for row in ledger_rows))
            self.assert_clock(self.change(login, 'previousDay'), 0, 2)
            self.assert_clock(self.change(login, 'previousDay'), -1, 3)
            self.assert_clock(self.change(login, 'reset'), 0, 4)
            report['checkpoints'].append('concurrent-idempotent-clock/payment-wall-time/previous-next-reset')
            opened_id = str(uuid.uuid4())
            opened = self.activities(login, 'open', request_id=opened_id)
            self.assertEqual(self.activities(login, 'open', request_id=opened_id), opened)
            self.tools('activities/apply', login, success=False, action='refresh',
                       activityId='all', requestId=opened_id)
            views = {item['id']: item for item in opened['activities']}
            outcomes = {item['id']: item for item in opened['results']}
            self.assertEqual(set(views), set(preflight['catalogue']))
            self.assertEqual(set(outcomes), set(views))
            for key, kind in preflight['catalogue'].items():
                self.assertIn(views[key]['status'], ('open', 'closed', 'unsupported'))
                self.assertEqual(outcomes[key]['status'], views[key]['status'])
                if not kind:
                    self.assertEqual(views[key]['status'], 'unsupported')
                    self.assertFalse(views[key]['canOpen'] or views[key]['canRefresh'])
            self.assertEqual(views['OpenHappy']['status'], 'open')
            self.assertEqual(views['BigTurnTable']['status'], 'open')
            report['actualCatalog'] = opened
            first_bindings = self.bindings(login)
            for key, view in views.items():
                if view['status'] != 'open':
                    self.assertNotIn(key, first_bindings,
                                     'Fresh closed/unsupported item acquired a private cycle')
                    continue
                self.assertIn(key, first_bindings)
                cycle = first_bindings[key][0]
                row = self.scalar('SELECT player_id,activity_id,start_at,end_at,reward_end_at,'
                                  'LENGTH(snapshot),source FROM local_play_activity_cycles '
                                  f"WHERE id='{cycle}';").split('\t')
                self.assertEqual((int(row[0]), row[1]), (login.playerId, key))
                self.assertEqual((int(row[2]), int(row[3])), (view['startsAt'], view['endsAt']))
                self.assertGreaterEqual(int(row[4]), int(row[3]))
                self.assertGreater(int(row[5]), 0, 'Open catalog needs a real frozen source snapshot')
                self.assertEqual(row[6], 'local-demo-activity-cycles-v1')
            # The deployed default selects the original red P2 wheel. A fresh
            # naturally registered Lv1 player must keep that real level gate.
            selected_source = self.scalar('SELECT wheel_pool FROM p2_king_rules WHERE id=1;')
            self.assertEqual(selected_source, 'red_turntable_20261013')
            red_cycle = first_bindings['BigTurnTable'][0]
            original_red = self.scalar("SELECT min_level,single_cost,ten_cost FROM gameplay_pools "
                                       f"WHERE id='{selected_source}';")
            min_level = int(original_red.split('\t')[0])
            self.assertGreater(min_level, self.profile(login)[0].get('level', 1))
            self.assertEqual(int(self.scalar("SELECT min_level FROM gameplay_pools "
                                              f"WHERE id='{red_cycle}';")), min_level)
            saved = self.profile(login)
            rejected_op, rejected = self.stamped('CSPlayBigTurn', {'count': 1}, login, 7900)
            self.assertEqual(rejected_op, 10001, 'Original red wheel must reject a fresh Lv1 character')
            self.assertEqual(self.profile(login), saved)
            # Authorized local operating selection only: keep both original
            # pools' actor gates/prices, player level/resources and random rules.
            self.assertEqual(self.scalar("SELECT min_level,single_cost,ten_cost FROM gameplay_pools "
                                         "WHERE id='turntable_20261011';"), '1\tdiamond,50\tdiamond,488')
            self.sql("UPDATE p2_king_rules SET wheel_pool='turntable_20261011' WHERE id=1;")
            self.activities(login, 'refresh', 'BigTurnTable')
            first_bindings = self.bindings(login)
            self.assertNotEqual(first_bindings['BigTurnTable'][0], red_cycle)
            self.assertEqual(self.scalar("SELECT min_level,single_cost,ten_cost FROM gameplay_pools "
                                         f"WHERE id='{selected_source}';"), original_red)
            views = {item['id']: item for item in self.activities(login)['activities']}
            report['wheelOperatingPrerequisite'] = {
                'isolatedSchemaOnly': True, 'defaultOriginalPool': selected_source,
                'defaultMinLevel': min_level, 'rejectedAtActualPlayerLevel': saved[0].get('level', 1),
                'originalRejectionCode': rejected.code, 'defaultPrivateCycleRetained': red_cycle,
                'selectedOriginalPool': 'turntable_20261011', 'selectedMinLevel': 1,
                'originalPrices': {'single': 50, 'ten': 488}, 'originalRedGateUnchanged': True,
                'profileLevelOrResourcesModified': False, 'originalPoolRulesModified': False}
            report['checkpoints'].append('original-red-wheel-level-rejection/isolated-existing-nonred-operator-selection')
            original_catalog = self.call('CSActivityList', {}, login, 6100, 'SCActivityList')
            self.assertTrue(original_catalog.activityList)
            report['originalActivityIds'] = [item.activityId for item in original_catalog.activityList]
            self.assertIn('bigTurnTable_20261011', report['originalActivityIds'])
            report['originalSelectors'] = self.original_selectors(
                login, views, first_bindings, report['originalActivityIds'])
            hud = self.call('CSOpenStateCheck', {}, login, 6101, 'SCOpenStateCheck')
            self.assertEqual({item.type: item.isOpen for item in hud.states}.get('ActivityNewOpen7Day'), 1)
            self.assert_open_day(login, 1)
            self.assertEqual(next(item for item in self.tab(login, 1).itemList if item.itemId == 1).state, 0)
            self.original_login(login)
            self.assertEqual(next(item for item in self.tab(login, 1).itemList if item.itemId == 1).state, 1)
            original_claim = self.claim(login, 7001)
            self.assertIn(('diamond', 100), [(item.id, item.num) for item in getattr(original_claim, 'get').simps])
            self.change(login, 'nextDay')
            self.original_login(login)
            self.assert_open_day(login, 2)
            self.assertTrue(self.tab(login, 2).itemList, 'Original day2 must be queryable')
            self.change(login, 'previousDay')
            self.original_login(login)
            self.assert_open_day(login, 1)
            self.assertEqual(next(item for item in self.tab(login, 1).itemList if item.itemId == 1).state, 2)
            self.assertEqual(self.stamped('CSGetOpenServerReward', {'itemId': 1}, login, 7002)[0], 10001)
            stable = self.profile(login)
            self.assertEqual(self.claim(login, 7001), original_claim)
            self.assertEqual(self.profile(login), stable)
            self.activities(login, 'refresh', 'OpenHappy')
            refreshed_bindings = self.bindings(login)
            self.assertNotEqual(refreshed_bindings['OpenHappy'][0], first_bindings['OpenHappy'][0])
            self.assert_open_day(login, 1)
            stable = self.profile(login)
            self.assertEqual(self.claim(login, 7001), original_claim)
            self.assertEqual(self.profile(login), stable)
            self.assertEqual(next(item for item in self.tab(login, 1).itemList if item.itemId == 1).state, 0)
            self.original_login(login)
            self.claim(login, 7003)
            self.assertEqual(self.scalar('SELECT COUNT(DISTINCT cycle_id) FROM local_play_open_server_receipts '
                                        f'WHERE player_id={int(login.playerId)};'), '2')
            report['checkpoints'].append('truthful-open-all/original-list-HUD/day1-day2-back/isolated-refresh-old-claim')
            free = self.draw(login, 8001, paid=False)
            charged = self.draw(login, 8002, paid=True)
            wheel_before = self.bindings(login)['BigTurnTable'][0]
            self.activities(login, 'refresh', 'BigTurnTable')
            wheel_after = self.bindings(login)['BigTurnTable'][0]
            self.assertNotEqual(wheel_before, wheel_after)
            stable = self.profile(login)
            self.assertEqual(self.call('CSPlayBigTurn', {'count': 1}, login, 8002, 'SCPlayBigTurn'), charged)
            self.assertEqual(self.profile(login), stable)
            self.assertEqual(self.stamped('CSPlayBigTurn', {'count': 10}, login, 8002)[0], 10001)
            self.draw(login, 8003, paid=False)
            self.draw(login, 8004, paid=True)
            self.assertEqual(self.scalar('SELECT COUNT(*) FROM turntable_states '
                                        f"WHERE player_id={int(login.playerId)} AND pool_id IN ('{wheel_before}','{wheel_after}');"), '2')
            self.assertEqual(self.scalar(ledger_query), ledger)
            before_cold = self.activities(login)
            clock = self.tools('time/state', login)
            bindings = self.bindings(login)
            saved = self.profile(login)
            old_session = bytes(login.sessionKey).hex()
            self.stop()
            self.start(wallet=True)
            login, protected = self.login(uid), self.login(protected_uid)
            self.tools('time/state', login, success=False, session=old_session)
            self.assertEqual(self.profile(login), saved)
            self.assertEqual(self.bindings(login), bindings)
            self.assertEqual(self.activities(login), before_cold)
            cold_clock = self.tools('time/state', login)
            self.assertEqual((cold_clock['offsetDays'], cold_clock['revision']),
                             (clock['offsetDays'], clock['revision']))
            self.assertEqual(self.change(login, 'nextDay', request_id), changed[0])
            self.assertEqual(self.tools('time/state', login)['offsetDays'], clock['offsetDays'])
            self.assertEqual(self.claim(login, 7001), original_claim)
            self.assertEqual(self.call('CSPlayBigTurn', {'count': 1}, login, 8002, 'SCPlayBigTurn'), charged)
            self.assertEqual(self.profile(login), saved)
            self.assertEqual(self.profile(protected), protected_before)
            self.assertEqual(self.scalar(ledger_query), ledger)
            self.assertEqual(self.scalar("SELECT created_at,confirmed_at FROM p3_payment_orders "
                                         f"WHERE id='{paid.orderId}';").split('\t'), accounting)
            report['checkpoints'].append('real-free-paid-wheel/cross-cycle-stamp/cold-state-and-receipts/protected1')
            report.update(status='PASS_PERSONAL_CLOCK_REAL_ACTIVITY_HTTP_MYSQL_COLD_PHONE_PENDING',
                          protected1Unchanged=True, coldRestarts=2,
                          finalCalendar={'offsetDays': clock['offsetDays'], 'revision': clock['revision']},
                          finalBindings=bindings, walletMoneyLedgerWallTimePreserved=True)
        except Exception as error:
            report.update(status='FAIL_PARTIAL_PREFIX_NOT_FULL_ACCEPTANCE',
                          failure=self.pool.redact(str(error))[:2000])
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
                    if self.app_temp is not None:
                        self.app_temp.cleanup()
                        report['ownedLogDirectoryRemoved'] = True
                    fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
                    lock.close()
                    report['seconds'] = round(time.monotonic() - began, 3)
                    REPORT.parent.mkdir(parents=True, exist_ok=True)
                    with REPORT.open('x') as output:
                        output.write(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
