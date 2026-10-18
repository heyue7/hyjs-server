"""Short prebuilt original benefit page -> actual shared sale -> receipt/cold.

Only the exact displayed diamond price is prepared in the owned isolated player.
TimeLimitSail opens through the real current-owner tool API; its source, prices,
inventory, quotas and receipts are never inserted. No compiler or device use.
"""
import concurrent.futures
import fcntl
import hashlib
import http.client as http_client
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
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from server.codec import CLASSES, BY_NAME, parse_http
import test_continued_gameplay_mysql as wire
import test_local_wallet_mysql as app_transport

ROOT = Path(__file__).resolve().parents[1]
DOMAIN = 'benefit-gift:time-limit-sale:v1'


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def protocol_preflight():
    vectors = {'CSOpenStateCheck': {}, 'CSBenefitGiftList': {},
               'CSBenefitGiftInfo': {}, 'CSBenefitGiftReward': {},
               'CSActivityShopInfo': {'activityId': 'actual-sale'},
               'CSGetActivityShopReward': {'activityId': 'actual-sale', 'index': 1,
                   'costResources': [{'id': 'diamond', 'num': 20}],
                   'getResources': [{'id': 'item_013', 'num': 1}]}}
    for name, fields in vectors.items():
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 931001)
        packet = parse_http(raw)
        if packet.opcode != BY_NAME[name] or packet.decode() != CLASSES[name](**fields):
            raise AssertionError('Original request differs: ' + name)
    for name, opcode in {'CSBenefitGiftList': 57149, 'SCBenefitGiftList': 57150,
                         'CSBenefitGiftInfo': 57151, 'SCBenefitGiftInfo': 57152,
                         'CSBenefitGiftReward': 57153, 'SCBenefitGiftReward': 57154}.items():
        if BY_NAME[name] != opcode:
            raise AssertionError('Original opcode differs: ' + name)
    fields = {'SCOpenStateCheck': {'states'}, 'OpenState': {'type', 'isOpen'},
              'SCBenefitGiftList': {'itemList'}, 'SCBenefitGiftInfo': {
                  'targetId', 'giftTitle', 'buyType', 'openTime', 'closeTime',
                  'price', 'discountPrice', 'reward', 'rewardState'},
              'SCBenefitGiftReward': {'get', 'cost', 'rewardState'},
              'SCActivityShopInfo': {'activityId', 'shopItemList'},
              'ActivityShopItem': {'index', 'getResource', 'costResource', 'left'}}
    for name, required in fields.items():
        if not required <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Actual original consumer differs: ' + name)
    return dict(originalRequestVectors=len(vectors), responseDescriptors=len(fields),
        nativeTownFlag='benefitGiftActivity', originalInfoOpcode=57151,
        originalPurchaseOpcode=57153, extraSCPlayerForbidden=True,
        finitePrerequisite='only displayed diamond balance; no natural resource-source claim',
        operatorSource='existing TimeLimitSail current-owner private cycle; no new gift catalogue',
        moneyOffer=False)


def source_contract(source):
    game = (source / 'hyjs-server/service/gameService.go').read_text()
    if 'case *pb.CSBenefitGiftList, *pb.CSBenefitGiftInfo, *pb.CSBenefitGiftReward:' not in game or 's.benefitGiftAt(ctx, row.Id, b, r.Timestamp, LocalPlayNow(ctx))' not in game:
        raise AssertionError('Actual original benefit dispatcher missing')
    states = (source / 'hyjs-server/service/openServerActivityService.go').read_text()
    if 's.BenefitGiftOpenState(ctx, id, now)' not in states:
        raise AssertionError('Native town icon is not connected')
    service = (source / 'hyjs-server/service/benefitGiftService.go').read_text()
    for token in ['localPlayEntryCycle(tx, "TimeLimitSail")', 'localPlayActivityVisible(tx, key)',
                  's.loadP4Commerce(', 's.p4Deduct(', 's.GrantFixedReward(',
                  'saveP4Commerce(tx, row, state)', 'benefitGiftReceipt(',
                  'shown.RuleID != quote.RuleID', 'shown.Day != quote.Day',
                  's.benefitGiftPageAt(ctx, id, &pb.CSBenefitGiftInfo{}, 0, now, false)']:
        if token not in service:
            raise AssertionError('Shared real sale/quote transaction missing: ' + token)
    main = (source / 'hyjs-server/main.go').read_text()
    for init in ['s.InitializeP1(ctx)', 's.InitializeP4(ctx)', 's.InitializeLocalPlay(ctx)',
                 's.InitializeLocalPlayActivities(ctx)', 's.InitializeOpenServerActivity(ctx)']:
        if init not in main:
            raise AssertionError('Normal startup dependency missing: ' + init)
    for file, table in [('p4CommerceState.go', 'p4_commerce_states'),
                        ('p4CommerceStock.go', 'p4_commerce_stocks'),
                        ('p4Receipt.go', 'p4_receipts')]:
        if 'return "' + table + '"' not in (source / 'hyjs-server/db' / file).read_text():
            raise AssertionError('Actual ledger SQL table differs: ' + file)
    return protocol_preflight()


def preflight():
    source = Path(os.environ['HYJS_BENEFIT_GIFT_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_BENEFIT_GIFT_BINARY']).resolve()
    validation = Path(os.environ['HYJS_BENEFIT_GIFT_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use a direct fixed candidate')
    manifest = source / 'manifest.json'
    pinned = set()
    for entry in json.loads(manifest.read_bytes())['files']:
        path = (source / entry['path']).resolve()
        if not path.is_relative_to(source) or path in pinned or sha(path) != entry['sha256']:
            raise AssertionError('Candidate pin differs: ' + entry['path'])
        pinned.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - pinned:
        raise AssertionError('Unpinned Go source')
    proof = json.loads(validation.read_bytes())
    if proof.get('qualified') is not True or (ROOT / proof['candidate']).resolve() != source:
        raise AssertionError('Completed exact qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Exact prebuilt qualified binary required')
    for relative in ['server/codec.py', 'server/protocol/schema.json']:
        if sha(source / relative) != sha(ROOT / relative):
            raise AssertionError('Original codec differs: ' + relative)
    return source, binary, sha(manifest), source_contract(source)


@unittest.skipUnless(os.environ.get('HYJS_BENEFIT_GIFT_MYSQL_TEST') == '1', 'Root managed prebuilt window')
class FunctionBenefitGiftMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    profile, stop, sql = transport.profile, transport.stop, transport.sql
    json_request, tools = transport.json_request, transport.tools

    def start(self):
        app_transport.SOURCE_ROOT = self.source
        # The existing combined demo-tool flags enable the personal activity API;
        # this testcase never opens a wallet, generates an order or confirms pay.
        app_transport.LocalWalletMySQLTests.start(self, wallet=True)

    def stamped(self, name, fields, owner, stamp):
        raw = wire.encode_original_http(name, fields, owner.playerId, bytes(owner.sessionKey), stamp)
        connection = http_client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=40)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            self.assertEqual(response.status, 200)
            frames = wire.decode_responses(response.read())
            self.assertEqual(len(frames), 1, 'Native callback must not be followed by SCPlayer')
            return frames[0]
        finally:
            connection.close()

    def raw_profile(self, owner):
        return self.sql(f'SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id={owner.playerId};').strip()

    def open_state(self, owner, stamp):
        reply = self.call('CSOpenStateCheck', {}, owner, stamp, 'SCOpenStateCheck')
        flags = [state.isOpen for state in reply.states if state.type == 'benefitGiftActivity']
        self.assertEqual(len(flags), 1)
        return flags[0]

    @staticmethod
    def simple_count(profile, item_id):
        return sum(int(item.get('num', 0)) for item in (profile[1].get('items') or []) if item['id'] == item_id)

    def test_native_current_owner_offer_actual_purchase_shared_quota_receipt_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_BENEFIT_GIFT_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/function-benefit-gift-r1-20261017'))
        self.assertFalse(output.exists(), 'Preserve previous attempts')
        report = dict(status='PENDING', fullChain=False, runnerSHA256=sha(__file__),
            binarySHA256=sha(binary), sourceManifestSHA256=manifest_sha, contract=contract,
            phoneUI=False, productionModified=False, financialOperations=False, phase='allocation')
        self.pool, self.process = ManagedMySQL(), None
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-benefit-gift-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('benefit_gift')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    suffix = secrets.token_hex(5)
                    stamp = int(time.time() * 1000)
                    protected = self.login('demo-benefit-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '福利保护', 'heroId': 'hero_001'}, protected, stamp, 'SCPlayerCreate')
                    protected_before = self.raw_profile(protected)
                    uid = 'demo-benefit-gift-' + suffix
                    owner = self.login(uid)
                    self.call('CSPlayerCreate', {'playerName': '限时特惠', 'heroId': 'hero_047'}, owner, stamp + 1, 'SCPlayerCreate')
                    self.assertGreater(owner.playerId, 1)
                    # Only isolated operator enablement; the API creates its own
                    # current-owner cycle from the existing real source snapshot.
                    self.pool.sql(self.database, 'UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    report['phase'] = 'real-owner-TimeLimitSail-open-and-native-town-icon'
                    opened = self.tools(owner, 'activities/apply', action='open', activityId='TimeLimitSail', requestId=str(uuid.uuid4()))
                    self.assertTrue(any(item['id'] == 'TimeLimitSail' and item['status'] == 'open' for item in opened['activities']))
                    cycle = self.sql('SELECT c.id,HEX(c.snapshot) FROM local_play_activity_cycles c '
                        'JOIN local_play_activity_bindings b ON b.cycle_id=c.id '
                        f"WHERE b.player_id={owner.playerId} AND b.activity_id='TimeLimitSail';").strip().split('\t')
                    snapshot = json.loads(bytes.fromhex(cycle[1]))
                    sales = [rule for rule in snapshot['commerce'] if rule['kind'] == 'sale']
                    self.assertEqual(len(sales), 1)
                    rule_id = sales[0]['id']
                    self.assertEqual(self.open_state(owner, stamp + 2), 1)
                    # The HUD can expose the icon without silently confirming a
                    # page quote. Direct purchase must still be rejected.
                    self.call('CSBenefitGiftReward', {}, owner, stamp + 3, 'ErrorMessage')
                    info = self.call('CSBenefitGiftInfo', {}, owner, stamp + 4, 'SCBenefitGiftInfo')
                    self.assertEqual((info.buyType, info.rewardState), ('diamond', 0))
                    self.assertTrue(info.targetId and info.giftTitle)
                    self.assertGreater(info.discountPrice, 0)
                    self.assertGreaterEqual(info.price, info.discountPrice)
                    self.assertLess(info.openTime, info.closeTime)
                    self.assertEqual([(item.id, item.num) for item in info.reward], [('item_013', 1)])
                    generic = self.call('CSActivityShopInfo', {'activityId': rule_id}, owner, stamp + 5, 'SCActivityShopInfo')
                    products = [item for item in generic.shopItemList
                                if [(r.id, r.num) for r in item.getResource] == [(r.id, r.num) for r in info.reward]
                                and [(r.id, r.num) for r in item.costResource] == [('diamond', info.discountPrice)]]
                    self.assertEqual(len(products), 1)
                    self.assertEqual(products[0].left, 1)
                    initial = self.profile(owner)
                    self.call('CSBenefitGiftReward', {}, owner, stamp + 6, 'ErrorMessage')
                    self.assertEqual(self.profile(owner), initial)
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.diamond',{info.discountPrice}) WHERE id={owner.playerId} AND uid='" + uid + "';")
                    funded = self.profile(owner)
                    self.assertEqual(funded[0], dict(initial[0], diamond=info.discountPrice))
                    self.assertEqual(funded[1], initial[1])
                    report['phase'] = 'original57153-eight-retries-one-real-purchase'
                    purchase_stamp = stamp + 7
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        replies = list(workers.map(lambda _: self.call('CSBenefitGiftReward', {}, owner, purchase_stamp, 'SCBenefitGiftReward'), range(8)))
                    self.assertTrue(all(reply == replies[0] for reply in replies))
                    reward = replies[0]
                    self.assertEqual(reward.rewardState, 2)
                    self.assertEqual([(r.id, r.num) for r in reward.cost], [('diamond', info.discountPrice)])
                    self.assertEqual([(r.id, r.num) for r in reward.get.simps], [('item_013', 1)])
                    final = self.profile(owner)
                    self.assertEqual(final[0].get('diamond', 0), 0)
                    self.assertEqual(self.simple_count(final, 'item_013'), self.simple_count(funded, 'item_013') + 1)
                    for key in ['coin', 'exp', 'level', 'point', 'energy', 'stamina']:
                        self.assertEqual(final[0].get(key, 0), funded[0].get(key, 0), key)
                    self.assertEqual(self.open_state(owner, stamp + 8), 0)
                    self.assertEqual(self.call('CSBenefitGiftInfo', {}, owner, stamp + 9, 'SCBenefitGiftInfo').rewardState, 2)
                    after = self.raw_profile(owner)
                    generic_request = {'activityId': rule_id, 'index': products[0].index,
                        'costResources': [dict(id=r.id, num=r.num) for r in products[0].costResource],
                        'getResources': [dict(id=r.id, num=r.num) for r in products[0].getResource]}
                    report['phase'] = 'generic-page-shared-quota-and-persistent-original-receipt'
                    self.call('CSGetActivityShopReward', generic_request, owner, stamp + 10, 'ErrorMessage')
                    self.assertEqual(self.raw_profile(owner), after)
                    generic_after = self.call('CSActivityShopInfo', {'activityId': rule_id}, owner, stamp + 11, 'SCActivityShopInfo')
                    self.assertEqual(next(item.left for item in generic_after.shopItemList if item.index == products[0].index), 0)
                    self.assertEqual(self.sql(f"SELECT used FROM p4_commerce_stocks WHERE rule_id='{rule_id}' AND product_id='{info.targetId}';").strip(), '1')
                    receipt_where = f"player_id={owner.playerId} AND domain='{DOMAIN}'"
                    receipt_sql = 'SELECT CONCAT(timestamp,"|",HEX(request),"|",HEX(response)) FROM p4_receipts WHERE ' + receipt_where + ';'
                    receipt = self.sql(receipt_sql).strip()
                    self.assertTrue(receipt.startswith(str(purchase_stamp) + '|'))
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p4_receipts WHERE ' + receipt_where + ';').strip(), '1')
                    report['phase'] = 'cold-original57153-receipt-and-closed-icon'
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.call('CSBenefitGiftReward', {}, owner, purchase_stamp, 'SCBenefitGiftReward'), reward)
                    self.assertEqual(self.open_state(owner, stamp + 12), 0)
                    self.assertEqual(self.call('CSBenefitGiftInfo', {}, owner, stamp + 13, 'SCBenefitGiftInfo').rewardState, 2)
                    self.assertEqual(self.raw_profile(owner), after)
                    self.assertEqual(self.sql(receipt_sql).strip(), receipt)
                    self.assertEqual(self.raw_profile(protected), protected_before)
                    self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME IN ('profiles','p4_receipts','p4_commerce_states','p4_commerce_stocks');").strip(), 'InnoDB')
                    report.update(status='PASS', fullChain=True, actualRuleID=rule_id, personalCycle=cycle[0],
                        actualProductID=info.targetId, actualDiamondCost=info.discountPrice,
                        actualReward='item_013,1', eightOriginalRetriesOnce=True, genericSharedQuota=True,
                        sharedStockUsed=1, nativeReceiptColdExact=True, nativeIconOpenThenClosed=True,
                        protected1Exact=True, finiteDiamond=info.discountPrice)
                except BaseException as error:
                    report.update(status='FAIL', failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    self.stop()
                    self.pool.close()
                    report['seconds'] = round(time.monotonic() - began, 3)
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    output.parent.mkdir(parents=True, exist_ok=True)
                    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
