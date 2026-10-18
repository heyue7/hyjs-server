"""Prebuilt original recruit -> actual inventory/growth -> calendar -> OS supply.

The first account stays at its natural created level and obtains its feed from
an original shop purchase funded by a real local-wallet-backed 6-yuan order.
Only a separate OS account receives finite level85/historical-star eligibility;
its coins come from original star claims. No coin, fragment, warrior or receipt
is inserted. No compiler, phone, production account or external payment.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import types
import unittest
import uuid

from hyjs_managed_mysql import ManagedMySQL
from server.codec import CLASSES, parse_http
import test_continued_gameplay_mysql as wire
import test_local_wallet_mysql as wallet_wire
from test_p5_talent_scope_mysql import original_table

ROOT = Path(__file__).resolve().parents[1]
OS_PREFIX = 'limitExchange_p4_red_os_'


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def protocol_preflight():
    vectors = {
        'CSWarriorRecruitGet': {'costType': [1, 2, 3]},
        'CSWarriorRecruit': {'costType': 2, 'numType': 1, 'free': 1},
        'CSFormationOn': {'formatPos': 2, 'id': 'hero_032', 'guid': 'actual-guid'},
        'CSWarriorStrengthen': {'guid': 'actual-guid', 'cost': [{'id': 'hero_exp_001', 'guid': 'actual-feed'}]},
        'CSShopBuy': {'shopType': 1, 'shopItemId': 'itemShopId_5', 'buyCount': 1},
        'CSGenerateOrder': {'paymentId': 'djsy.wuyou.andp0006', 'money': 6.0,
                            'moneyType': 'RMB', 'channelId': 'AND0003', 'shopId': 'common'},
        'CSCheckOrder': {'orderId': 'actual-order', 'paymentId': 'djsy.wuyou.andp0006'},
        'CSActivityShopInfo': {'activityId': 'limitExchange_p4_catalog_lp-' + '0' * 26},
        'CSBuyActivityShop': {'activityId': 'actual-directory', 'index': 1, 'num': 3,
                              'costResources': [{'id': 'coin', 'num': 500000}],
                              'getResources': [{'id': 'frag_hero_077', 'num': 5}]},
        'CSCopyGetStarAward': {'chapterId': '1', 'starIndex': 0},
    }
    for name, fields in vectors.items():
        encoded = wire.encode_original_http(name, fields, 2, b'0' * 16, 100)
        if parse_http(encoded).decode() != CLASSES[name](**fields):
            raise AssertionError('Recovered original wire changed: ' + name)
    required = {'SCWarriorRecruit': {'get', 'cost'}, 'SCWarriorStrengthen': {'guid', 'cost'},
                'WarriorRecruit': {'costType', 'num', 'free', 'freeTime', 'leftTimes'},
                'ActivityShopItem': {'index', 'getResource', 'costResource', 'left'},
                'SCGetActivityShopReward': {'get', 'costResources'}}
    for name, fields in required.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Recovered original response changed: ' + name)
    return {'requestVectors': len(vectors), 'responseContracts': len(required)}


def preflight():
    contracts = protocol_preflight()
    source = Path(os.environ['HYJS_GROWTH_RECRUIT_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_GROWTH_RECRUIT_BINARY']).resolve()
    validation = Path(os.environ['HYJS_GROWTH_RECRUIT_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use the direct fixed growth candidate')
    manifest = source / 'manifest.json'
    entries = json.loads(manifest.read_text())['files']
    paths = set()
    for entry in entries:
        path = (source / entry['path']).resolve()
        if source not in path.parents or path in paths or sha(path) != entry['sha256']:
            raise AssertionError('Fixed source/asset pin mismatch: ' + entry['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Candidate contains unpinned Go source')
    proof = json.loads(validation.read_text())
    if proof.get('qualified') is not True or Path(proof.get('candidate', '')).resolve() != source:
        raise AssertionError('Completed exact root qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use the qualified app without compiling')
    for name in ('Hero', 'Frag', 'HeroExp', 'GameConfigure', 'CopyMission', 'Payment'):
        relative = 'analysis/tables/Table' + name + '.tsv'
        if sha(source / relative) != sha(ROOT / relative):
            raise AssertionError('Runtime original table differs: ' + name)
    for relative in ('server/codec.py', 'server/protocol/schema.json'):
        if (source / relative).read_bytes() != (ROOT / relative).read_bytes():
            raise AssertionError('Original frame codec differs from candidate')
    required = {'hyjs-server/service/growthRecruitService.go': 'growthRecruitCalendarCount',
                'hyjs-server/service/recruitService.go': 'growthRecruitState(tx, id',
                'hyjs-server/service/redP2Service.go': 'growthRecruitKingDay(tx, state',
                'hyjs-server/service/localPlayActivityScopeService.go': 's.p15AppendRedOSBindings(tx, &directory',
                'hyjs-server/db/sql/191_growth_recruit_policies.sql': 'published_at'}
    for relative, needle in required.items():
        if needle not in (source / relative).read_text():
            raise AssertionError('Required growth integration missing: ' + relative)
    return source, binary, sha(manifest), contracts


@unittest.skipUnless(os.environ.get('HYJS_GROWTH_RECRUIT_MYSQL_TEST') == '1',
                     'Root coordinated isolated InnoDB window required')
class GrowthRecruitMySQLTests(unittest.TestCase):
    # Only transport methods: no inherited testcase/setup/compiler/root SQL.
    send = wire.ContinuedGameplayMySQLTests.send
    login = wire.ContinuedGameplayMySQLTests.login
    call = wire.ContinuedGameplayMySQLTests.call
    stamped = wire.ContinuedGameplayMySQLTests.stamped
    profile = wire.ContinuedGameplayMySQLTests.profile
    stop = wire.ContinuedGameplayMySQLTests.stop
    json_request = wallet_wire.LocalWalletMySQLTests.json_request
    wallet_body = wallet_wire.LocalWalletMySQLTests.wallet_body
    wallet = wallet_wire.LocalWalletMySQLTests.wallet
    order = wallet_wire.LocalWalletMySQLTests.order

    def start(self):
        wallet_wire.SOURCE_ROOT = self.source
        wallet_wire.LocalWalletMySQLTests.start(self, wallet=True)

    def sql(self, query):
        if not query.lstrip().upper().startswith('SELECT '):
            raise AssertionError('General SQL entry is read-only')
        return self.pool.sql(self.database, query)

    def tools(self, owner, endpoint, **fields):
        status, response = self.json_request('/local-play/' + endpoint,
            {'playerId': owner.playerId, 'sessionHex': bytes(owner.sessionKey).hex(), **fields})
        self.assertEqual(status, 200)
        self.assertEqual(response.get('code'), 0, response.get('msg'))
        return response['data']

    def change(self, owner, action):
        return self.tools(owner, 'time/change', action=action, requestId=str(uuid.uuid4()))

    def activity(self, owner, action):
        return self.tools(owner, 'activities/apply', action=action,
                          activityId='TimeLimitExchange', requestId=str(uuid.uuid4()))

    def directory(self, owner):
        value = self.sql('SELECT HEX(c.snapshot) FROM local_play_activity_cycles c '
            'JOIN local_play_activity_bindings b ON b.cycle_id=c.id '
            f"WHERE b.player_id={int(owner.playerId)} AND b.activity_id='TimeLimitExchange';").strip()
        key = json.loads(bytes.fromhex(value))['exchange']['id']
        row = self.sql("SELECT input_hash,HEX(bindings) FROM p4_resource_exchange_directories WHERE id='" + key + "';").strip()
        digest, raw = row.split('\t')
        self.assertEqual(hashlib.sha256(bytes.fromhex(raw)).hexdigest(), digest)
        return key, digest, raw, json.loads(bytes.fromhex(raw))

    def publish_os(self, child):
        # Fixed policy publication is an explicit isolated operator action.
        # Never replace the rules/frozen quota/receipt with a fixture.
        now = int(time.time() * 1000)
        self.assertTrue(self.sql("SELECT source FROM p4_commerce_rules WHERE id='" + child + "';").strip().startswith('red-os-mainline-coin-fragments-v1'))
        self.assertEqual(self.sql('SELECT enabled,published_at FROM growth_recruit_policies WHERE id=1;').strip(), '0\t0')
        self.pool.sql(self.database,
            "START TRANSACTION; UPDATE p4_commerce_rules SET enabled=1 WHERE id='" + child + "' AND enabled=0 AND source LIKE 'red-os-mainline-coin-fragments-v1%'; "
            f"UPDATE growth_recruit_policies SET enabled=1,published_at={now} WHERE id=1 AND enabled=0 AND published_at=0 AND version='single-player-recruit-v1'; COMMIT;")

    def finite_os_eligibility(self, owner):
        self.assertGreater(owner.playerId, 1)
        before = self.profile(owner)
        self.assertEqual((before[0].get('coin', 0), before[0].get('diamond', 0)), (0, 0))
        missions = [dict(chapterId=row['ChapterId'], missionId=row['MissionId'], star=5, challengeTimes=0)
                    for row in original_table('CopyMission')]
        raw = json.dumps(missions, separators=(',', ':')).encode().hex()
        self.pool.sql(self.database, "UPDATE profiles SET player=JSON_SET(player,'$.level',85), "
            "game=JSON_SET(game,'$.copy.passedCopyInfo',CAST(CONVERT(0x" + raw + " USING utf8mb4) AS JSON)) "
            f"WHERE id={int(owner.playerId)} AND uid LIKE 'demo-growth-os-%';")
        after = self.profile(owner)
        self.assertEqual(after[0].get('coin', 0), 0)
        self.assertEqual(after[1]['warriors'], before[1]['warriors'])
        self.assertEqual(after[1].get('items'), before[1].get('items'))

    def test_original_free_growth_rewind_private_os_quota_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contracts = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_GROWTH_RECRUIT_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/recruit'))
        self.assertFalse(output.exists(), 'Preserve prior acceptance attempts')
        self.pool = ManagedMySQL()
        self.process = None
        report = {'status': 'PENDING', 'binarySHA256': sha(binary), 'sourceManifestSHA256': manifest_sha,
                  'runnerSHA256': sha(__file__), 'contracts': contracts, 'phoneUI': False,
                  'productionModified': False, 'externalPayment': False, 'fullChain': False,
                  'scope': 'original free ordinary/strong acquisition and actual growth; bounded new-policy OS15-fragment purchase/quota; historical150 combine reused separately',
                  'naturalLevel85Claim': False, 'newOS150Revalidated': False,
                  'finiteOSPrerequisites': 'separate account level85 + original historical stars; no actual wins claimed; no coin/fragment/warrior inserted',
                  'historical150Combine': 'analysis/p15/red-three-supply-mysql-validation.json reused as historical finite proof; not rerun here'}
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-growth-recruit-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('growth_recruit')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-growth-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '保护招募账号', 'heroId': 'hero_043'}, protected, 1, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    owner_uid = 'demo-growth-recruit-' + suffix
                    owner = self.login(owner_uid)
                    self.assertGreater(owner.playerId, 1)
                    stamp = 410000

                    def http(login, name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, login, stamp, expected)

                    def once(login, name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        selected_stamp = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            replies = list(workers.map(lambda _: self.call(name, fields, login, selected_stamp, expected), range(8)))
                        self.assertTrue(all(x == replies[0] for x in replies))
                        return replies[0], selected_stamp

                    http(owner, 'CSPlayerCreate', {'playerName': '正常招募养成', 'heroId': 'hero_043'}, 'SCPlayerCreate')
                    fresh = self.profile(owner)
                    self.assertEqual((fresh[0].get('coin', 0), fresh[0].get('diamond', 0), fresh[0]['level']), (0, 0, 1))
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    acquired = []
                    for kind in (1, 2):
                        reply, saved_stamp = once(owner, 'CSWarriorRecruit', {'costType': kind, 'numType': 1, 'free': 1}, 'SCWarriorRecruit')
                        self.assertEqual(len(reply.cost), 0)
                        self.assertEqual(len(getattr(reply, 'get').wars), 1)
                        acquired.append((getattr(reply, 'get').wars[0], saved_stamp, kind))
                    initial_inventory = self.profile(owner)[1]['warriors']
                    self.assertEqual(len(initial_inventory), len(fresh[1]['warriors']) + 2)
                    strong, strong_stamp, _ = acquired[1]
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=600)
                    stamp += 1
                    order = self.order(owner, 'djsy.wuyou.andp0006', stamp)
                    self.assertEqual(order.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    checked = http(owner, 'CSCheckOrder', {'orderId': order.orderId, 'paymentId': 'djsy.wuyou.andp0006'}, 'SCCheckOrder')
                    self.assertEqual(checked.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    donor = http(owner, 'CSShopBuy', {'shopType': 1, 'shopItemId': 'itemShopId_5', 'buyCount': 1}, 'SCShopBuy')
                    feed = getattr(donor, 'get').wars[0]
                    self.assertEqual(feed.id, 'hero_exp_001')
                    http(owner, 'CSFormationOn', {'formatPos': 2, 'id': strong.id, 'guid': strong.guid}, 'SCFormationOn')
                    grown = http(owner, 'CSWarriorStrengthen', {'guid': strong.guid, 'cost': [{'id': feed.id, 'guid': feed.guid}]}, 'SCWarriorStrengthen')
                    self.assertGreater((grown.guid.level, grown.guid.exp), (strong.level, strong.exp))
                    before_old = self.profile(owner)
                    self.assertEqual(self.stamped('CSWarriorRecruit', {'costType': 2, 'numType': 1, 'free': 1}, owner, strong_stamp)[0], 10001)
                    self.assertEqual(self.profile(owner), before_old)
                    self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], 1)
                    http(owner, 'CSWarriorRecruit', {'costType': 1, 'numType': 1, 'free': 1}, 'SCWarriorRecruit')
                    self.assertEqual(self.change(owner, 'previousDay')['offsetDays'], 0)
                    self.assertEqual(http(owner, 'CSWarriorRecruitGet', {'costType': [1]}, 'SCWarriorRecruitGet').recruit[0].num, 2)
                    self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], 1)
                    self.assertEqual(http(owner, 'CSWarriorRecruitGet', {'costType': [1]}, 'SCWarriorRecruitGet').recruit[0].num, 2)
                    self.assertEqual(self.change(owner, 'reset')['offsetDays'], 0)

                    os_uid = 'demo-growth-os-' + suffix
                    os_owner = self.login(os_uid)
                    http(os_owner, 'CSPlayerCreate', {'playerName': '有限三红资格', 'heroId': 'hero_043'}, 'SCPlayerCreate')
                    self.finite_os_eligibility(os_owner)
                    earned = 0
                    star_claims = []
                    for chapter in reversed(original_table('CopyChapter')):
                        for index in range(3):
                            reward = http(os_owner, 'CSCopyGetStarAward', {'chapterId': chapter['ChapterId'], 'starIndex': index}, 'SCCopyGetStarAward')
                            earned += sum(x.num for x in reward.gain.simps if x.id == 'coin')
                            star_claims.append((chapter['ChapterId'], index))
                            if earned >= 1500000:
                                break
                        if earned >= 1500000:
                            break
                    self.assertGreaterEqual(earned, 1500000)
                    self.assertEqual(self.profile(os_owner)[0]['coin'], earned)
                    self.activity(os_owner, 'open')
                    old_key, old_hash, old_raw, old_bindings = self.directory(os_owner)
                    self.assertFalse(any(x['childId'].startswith(OS_PREFIX) for x in old_bindings))
                    month = time.strftime('%Y%m', time.gmtime(time.time() + 8 * 3600))
                    child = OS_PREFIX + month
                    self.publish_os(child)
                    http(os_owner, 'CSActivityShopInfo', {'activityId': old_key}, 'SCActivityShopInfo')
                    self.assertEqual(self.directory(os_owner)[:3], (old_key, old_hash, old_raw))
                    self.activity(os_owner, 'refresh')
                    key, _, _, bindings = self.directory(os_owner)
                    addresses = [x for x in bindings if x['childId'] == child]
                    self.assertEqual({x['productId'] for x in addresses}, {'frag_hero_077', 'frag_hero_078', 'frag_hero_079'})
                    listing = http(os_owner, 'CSActivityShopInfo', {'activityId': key}, 'SCActivityShopInfo')
                    product = next(x for x in listing.shopItemList if x.getResource[0].id == 'frag_hero_077')
                    self.assertEqual((product.getResource[0].num, product.costResource[0].id, product.costResource[0].num), (5, 'coin', 500000))
                    request = {'activityId': key, 'index': product.index, 'num': 3,
                               'costResources': product.costResource, 'getResources': product.getResource}
                    paid, paid_stamp = once(os_owner, 'CSBuyActivityShop', request, 'SCGetActivityShopReward')
                    self.assertEqual(sum(x.num for x in getattr(paid, 'get').simps if x.id == 'frag_hero_077'), 15)
                    self.assertEqual(self.profile(os_owner)[0]['coin'], earned - 1500000)
                    quota = self.sql(f"SELECT HEX(payload) FROM p4_commerce_states WHERE player_id={os_owner.playerId} AND rule_id='{child}';").strip()
                    progress = json.loads(bytes.fromhex(quota))
                    self.assertEqual((progress['total']['frag_hero_077'], progress['bought']['frag_hero_077']), (3, 3))
                    other_before = self.profile(owner)
                    http(owner, 'CSActivityShopInfo', {'activityId': key}, 'ErrorMessage')
                    self.assertEqual(self.profile(owner), other_before)
                    self.activity(os_owner, 'refresh')
                    new_key, _, _, new_bindings = self.directory(os_owner)
                    self.assertNotEqual(new_key, key)
                    self.assertEqual([x['childId'] for x in new_bindings if x['productId'].startswith('frag_hero_07')], [child] * 3)
                    listing = http(os_owner, 'CSActivityShopInfo', {'activityId': new_key}, 'SCActivityShopInfo')
                    product = next(x for x in listing.shopItemList if x.getResource[0].id == 'frag_hero_077')
                    http(os_owner, 'CSBuyActivityShop', {**request, 'activityId': new_key, 'index': product.index, 'num': 1,
                        'costResources': product.costResource, 'getResources': product.getResource}, 'ErrorMessage')
                    self.assertEqual(self.sql(f"SELECT HEX(payload) FROM p4_commerce_states WHERE player_id={os_owner.playerId} AND rule_id='{child}';").strip(), quota)
                    self.assertEqual(self.sql("SELECT input_hash,HEX(bindings) FROM p4_resource_exchange_directories WHERE id='" + old_key + "';").strip(), old_hash + '\t' + old_raw)
                    before_cold = self.profile(owner), self.profile(os_owner)
                    self.stop()
                    self.start()
                    owner, os_owner = self.login(owner_uid), self.login(os_uid)
                    self.assertEqual((self.profile(owner), self.profile(os_owner)), before_cold)
                    self.assertEqual(self.tools(owner, 'time/state')['offsetDays'], 0)
                    self.assertEqual(self.call('CSBuyActivityShop', request, os_owner, paid_stamp, 'SCGetActivityShopReward'), paid)
                    self.assertEqual(self.profile(protected), protected_before)
                    self.assertEqual(self.sql(f'SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=\'{self.database}\' AND TABLE_NAME IN (\'profiles\',\'growth_recruit_policies\',\'local_play_activity_operations\');').split(), ['InnoDB'])
                    report.update(status='PASS_ORIGINAL_RECRUIT_GROWTH_CALENDAR_OS_PRIVATE_QUOTA_COLD_PHONE_PENDING',
                        fullChain=True, createdNaturalLevel=1, actualFreeAwards=[x[0].id for x in acquired],
                        actualStrongGUID=strong.guid, actualGrowth={'level': grown.guid.level, 'exp': grown.guid.exp},
                        materialSource='original600centorder→actualdiamond→originalhero_exp001shop',
                        OS={'actualStarClaims': star_claims, 'actualSourceCoin': earned, 'fragmentsPaid': 15,
                            'paidCoin': 1500000, 'monthChild': child, 'addresses': 3,
                            'oldPrivateBytesUnchanged': True, 'newPrivateRefreshPreservedQuota': True},
                        originalPaymentOrder=order.orderId, protected1Exact=True, foreignPrivateRejected=True,
                        calendarRevisitFreeUsageRestored=True, staleGrownRecruitReplayRejected=True, coldRestarts=1)
                except BaseException as error:
                    report.update(status='FAIL_PARTIAL_PREFIX_NOT_FULL_ACCEPTANCE', failureType=type(error).__name__, failure=str(error))
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
