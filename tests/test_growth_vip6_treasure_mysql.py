"""Prebuilt G6: actual wallet/order ->VIP6 parcel ->two treasure lifecycles.

Finite Player15 for original wear, then16 for original recycle-reset. No resource,
currency, VIP, parcel or instance-GUID injection; no compiler/device/live DB.
"""
import concurrent.futures
import fcntl
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
import test_growth_resource_loop_mysql as shared
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_p5_talent_scope_mysql import original_table
from server.codec import CLASSES, parse_http

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'build/candidates/growth-vip6-r3-20261017'
BINARY = ROOT / 'build/hyjs-server-growth-vip6-r3-20261017-pending'
QUALIFICATION = ROOT / 'analysis/p3/growth-vip6-r3-incremental-20261017.json'
DELIVERY = ROOT / 'analysis/growth-first-20261016/resources/g6-next-gap-r1/manifest.json'
PRODUCT = 'p3vip6_growth_local'
TREASURES = ['trea_011', 'trea_013']


def protocol_preflight():
    vectors = {'CSShopProductList': {'shopType': 2},
        'CSShopBuy': {'shopType': 2, 'shopItemId': PRODUCT, 'buyCount': 1},
        'CSItemUse': {'costItem': {'id': 'item_040', 'num': 1}},
        'CSFormationOn': {'id': 'trea_011', 'guid': 'actual-guid', 'formatPos': 1},
        'CSFormationOff': {'id': 'trea_011', 'guid': 'actual-guid', 'formatPos': 1},
        'CSTreasureRefine': {'targetId': 'actual-guid'},
        'CStreasureRebuild': {'targetId': 'actual-guid'},
        'CSTreasureSell': {'guids': ['actual-guid']}}
    for name, fields in vectors.items():
        raw = shared.wire.encode_original_http(name, fields, 2, b'0' * 16, 400001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original G6 frame differs: ' + name)
    box = [r for r in original_table('ItemUse') if r['itemId'] == 'item_040']
    if len(box) != 1 or box[0]['result'] != 'trea_011,1;trea_013,1;item_005,1000;item_048,4;item_002,10':
        raise AssertionError('Original040 complete fixed parcel differs')
    gold_box = next(r for r in original_table('ItemUse') if r['itemId'] == 'item_041')
    gold_rewards = dict(part.split(',') for part in gold_box['result'].split(';'))
    if gold_rewards.get('coin') != '1000000' or gold_rewards.get('item_012') != '50':
        raise AssertionError('Original VIP7 complete source gold/stone reward differs')
    rows = original_table('TreasureRefineLevel')
    for key in TREASURES:
        first = [r for r in rows if r['treasureId'] == key and r['level'] == '1']
        if len(first) != 1 or first[0]['objStr'] != 'item_012,30;coin,30000':
            raise AssertionError('Original first refinement cost differs: ' + key)
    cfg = {r['StaticName']: r['NameValue'] for r in original_table('GameConfigure')}
    if (cfg['RobTreasureLevel'], cfg['RecycleRoomLevel'], cfg['EquipmentRebuildCost']) != ('15', '16', 'diamond,50'):
        raise AssertionError('Original wear/reset levels or reset fee differs')
    prices = [r for r in original_table('ItemShopPrice') if r['shopItemId'] == 'itemShopId_11']
    if not prices or any(r['price'] != 'diamond,15' for r in prices):
        raise AssertionError('Original stone shop price differs')
    return dict(originalRequests=len(vectors), originalParcel=box[0]['result'], originalVIP7Stone=50,
        originalRefinePerTreasure={'item_012': 30, 'coin': 30000}, originalResetPerTreasure={'diamond': 50},
        finiteLevels={'wear': 15, 'reset': 16}, VIPShopRetry='daily claim; second request rejected, no stamp idempotency',
        originalStoneShop='itemShopId_11/15diamond per5stones;12portions=60stones/180diamond')


def preflight():
    contract = protocol_preflight()
    for key, value in {'HYJS_GROWTH_RESOURCE_SOURCE': SOURCE, 'HYJS_GROWTH_RESOURCE_BINARY': BINARY,
                       'HYJS_GROWTH_RESOURCE_VALIDATION': QUALIFICATION}.items():
        os.environ.setdefault(key, str(value))
    source, binary, manifest_sha = shared.preflight()
    if source != SOURCE:
        raise AssertionError('Use the fixed qualified G6 source')
    for name, expected in json.loads(DELIVERY.read_bytes())['originalTables'].items():
        if shared.sha(source / 'analysis/tables' / ('Table' + name + '.tsv')) != expected:
            raise AssertionError('Original G6 asset differs: ' + name)
    hooks = {'hyjs-server/service/paymentService.go': 's.seedP3VIP6GrowthProduct(tx)',
        'hyjs-server/service/p3VIP6GrowthService.go': 'MinVIP: 6, Cost: 700, Reward: "item_040,1"',
        'hyjs-server/service/shopService.go': '{"itemShopId_11", "item_012", "diamond,15", 5}',
        'hyjs-server/service/vipService.go': '"vip-shop:" + product + ":" + p3Day(now)',
        'hyjs-server/service/p5TreasureResetService.go': 's.p5Refund(p, g, treasure.GetGuid(), treasure)',
        'hyjs-server/service/p5Service.go': 'case "warrior-reset", "warrior-decompose", "equipment-reset", "equipment-decompose", "treasure-reset":',
        'hyjs-server/service/localWalletService.go': 'Kind: "debit", DeltaCents: -order.Cents'}
    contract['productionSHA256'] = {}
    for relative, needle in hooks.items():
        if needle not in (source / relative).read_text():
            raise AssertionError('Actual G6 integration differs: ' + relative)
        contract['productionSHA256'][relative] = shared.sha(source / relative)
    main = (source / 'hyjs-server/main.go').read_text()
    for name in ['InitializeP1', 'InitializePayments', 'InitializeShop', 'InitializeP5',
                 'InitializeLocalWallet', 'InitializeLocalPlay', 'InitializeOpenServerActivity', 'InitializeDailyTasks']:
        if 's.' + name + '(ctx)' not in main:
            raise AssertionError('Actual main lifecycle dependency missing: ' + name)
    contract['qualificationSHA256'] = shared.sha(QUALIFICATION)
    return source, binary, manifest_sha, contract


@unittest.skipUnless(os.environ.get('HYJS_GROWTH_VIP6_MYSQL_TEST') == '1', 'Root managed prebuilt G6 window')
class GrowthVIP6TreasureMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    stamped, profile, start, stop, sql = transport.stamped, transport.profile, transport.start, transport.stop, transport.sql
    json_request, wallet_body, wallet = transport.json_request, transport.wallet_body, transport.wallet
    tools, change = transport.tools, transport.change

    def test_paid_original_parcel_two_refinements_wear_reset_sale_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ.get('HYJS_GROWTH_VIP6_OUTPUT',
            ROOT / 'analysis/growth-first-20261016/resources/g6-http-mysql-r1.json')).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/resources'))
        self.assertFalse(output.exists(), 'Preserve earlier validation evidence')
        report = dict(status='PENDING', fullChain=False, binarySHA256=shared.sha(binary),
            sourceManifestSHA256=manifest_sha, runnerSHA256=shared.sha(__file__), contract=contract,
            finitePreparation='Only isolated Player15 wear, then Player16 original reset; no resources/VIP/instances inserted',
            productionModified=False, externalPayment=False, phoneUI=False, P6AuthorityChanged=False)
        began = time.monotonic()
        self.pool, self.process = ManagedMySQL(), None
        with tempfile.TemporaryDirectory(prefix='hyjs-g6-vip6-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('growth_vip6')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    for table, expected in {'p3_vip_shop_products': {'id', 'min_v_ip', 'cost', 'reward', 'enabled'},
                        'p3_membership_claims': {'player_id', 'kind', 'period'},
                        'p3_payment_activity_states': {'player_id', 'activity_id', 'snapshot'},
                        'p3_payment_orders': {'id', 'player_id', 'status', 'cents', 'created_at', 'confirmed_at'},
                        'local_play_payment_times': {'order_id', 'game_at', 'real_at', 'offset_days'},
                        'local_wallet_entries': {'player_id', 'kind', 'delta_cents'},
                        'p5_mutation_receipts': {'player_id', 'operation', 'timestamp'},
                        'p5_collections': {'player_id'}, 'shop_states': {'player_id', 'product_id', 'day', 'bought'},
                        'shop_receipts': {'player_id', 'timestamp'}}.items():
                        columns = self.sql('SELECT COLUMN_NAME FROM information_schema.COLUMNS '
                            f"WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='{table}';").split()
                        self.assertTrue(expected <= set(columns), table + ' actual schema')
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-g6-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '宝物来源保护', 'heroId': 'hero_001'}, protected, 400000, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    identity = 'demo-g6-vip6-' + suffix
                    owner = self.login(identity)
                    stamp = 400001

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once(name, fields, expected):
                        nonlocal stamp
                        if name == 'CSShopBuy' and fields.get('shopType') == 2:
                            raise AssertionError('VIP daily purchase has no stamped replay')
                        stamp += 1
                        saved = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            replies = list(workers.map(lambda _: self.call(name, fields, owner, saved, expected), range(8)))
                        self.assertTrue(all(r == replies[0] for r in replies))
                        return replies[0], saved

                    def reject(name, fields):
                        before = self.profile(owner)
                        http(name, fields, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), before)

                    def count(key):
                        return sum(x['num'] for x in (self.profile(owner)[1].get('items') or []) if x['id'] == key)

                    def treasures():
                        return self.profile(owner)[1].get('treasures') or []

                    def level(value):
                        before = self.profile(owner)
                        self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',{value}) WHERE id={owner.playerId};")
                        self.assertEqual(self.profile(owner), [dict(before[0], level=value), before[1]])

                    def vip_quote():
                        items = http('CSShopProductList', {'shopType': 2}, 'SCShopProductList').shopItemList
                        return next(x for x in items if x.id == PRODUCT)

                    http('CSPlayerCreate', {'playerName': '原VIP6宝物链', 'heroId': 'hero_001'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual((initial[0].get('coin', 0), initial[0].get('diamond', 0)), (0, 0))
                    self.assertFalse(treasures())
                    level(15)
                    vip_buy = {'shopType': 2, 'shopItemId': PRODUCT, 'buyCount': 1}
                    product_row = self.sql(f"SELECT min_v_ip,cost,reward,enabled FROM p3_vip_shop_products WHERE id='{PRODUCT}';").strip()
                    self.assertEqual(product_row, '6\t700\titem_040,1\t0')
                    reject('CSShopBuy', vip_buy)
                    for key, minimum, price, reward in [(PRODUCT, 6, 700, 'item_040,1'), ('p3vip7_growth_local', 7, 800, 'item_041,1')]:
                        self.assertEqual(self.sql(f"SELECT min_v_ip,cost,reward,enabled FROM p3_vip_shop_products WHERE id='{key}';").strip(), f'{minimum}\t{price}\t{reward}\t0')
                        self.pool.sql(self.database, f"UPDATE p3_vip_shop_products SET enabled=1 WHERE id='{key}' AND min_v_ip={minimum} AND cost={price} AND reward='{reward}' AND enabled=0;")
                    self.assertEqual(vip_quote().canBuy, 0)
                    reject('CSShopBuy', vip_buy)  # unpaid qualification, no injected VIP
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=200000)
                    catalogue = http('CSGetPaymentInfo', {'channelId': 'AND0003', 'shopId': 'common'}, 'SCGetPaymentInfo')
                    sku = next(x for x in catalogue.paymentInfoList if x.paymentId == 'djsy.wuyou.andp2000')
                    self.assertEqual(sku.money, 2000.0)
                    order, order_stamp = once('CSGenerateOrder', {'paymentId': sku.paymentId, 'money': sku.money,
                        'moneyType': sku.moneyType, 'channelId': 'AND0003', 'shopId': sku.shopId}, 'SCGenerateOrder')
                    self.assertEqual(order.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    actual_vip = self.profile(owner)[0]['vip']
                    self.assertEqual((actual_vip, self.profile(owner)[0]['diamond']), (7, 23000))
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    self.assertEqual(self.sql(f"SELECT COUNT(*),SUM(delta_cents) FROM local_wallet_entries WHERE player_id={owner.playerId} AND kind='debit';").strip(), '1\t-200000')
                    order_before = self.sql(f"SELECT status,cents,credited,created_at,expires_at,confirmed_at FROM p3_payment_orders WHERE id='{order.orderId}';").strip()
                    self.assertTrue(order_before.startswith('confirmed\t200000\t23000\t'))
                    quote = vip_quote()
                    self.assertEqual((quote.canBuy, quote.dayLeftTimes, quote.dayTotalTimes), (1, 1, 1))
                    self.assertEqual([(x.id, x.num) for x in quote.price], [('diamond', 700)])
                    bought = http('CSShopBuy', vip_buy, 'SCShopBuy')  # original daily claim, intentionally single
                    self.assertEqual([(x.id, x.num) for x in bought.cost], [('diamond', 700)])
                    reject('CSShopBuy', vip_buy)
                    self.assertEqual(vip_quote().dayLeftTimes, 0)
                    self.assertEqual(count('item_040'), 1)
                    opened, open_stamp = once('CSItemUse', {'costItem': {'id': 'item_040', 'num': 1}}, 'SCItemUse')
                    gained = getattr(opened, 'get')
                    acquired = sorted(gained.treas, key=lambda x: x.treasureId)
                    self.assertEqual([x.treasureId for x in acquired], TREASURES)
                    self.assertEqual(len({x.guid for x in acquired}), 2)
                    self.assertTrue(all(x.guid and x.strengthLevel == 1 and x.refineLevel == 0 for x in acquired))
                    self.assertEqual({x.id: x.num for x in gained.simps}, {'item_005': 1000, 'item_048': 4, 'item_002': 10})
                    self.assertEqual([count(x) for x in ['item_005', 'item_048', 'item_002', 'item_040']], [1000, 4, 10, 0])
                    http('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1}, 'SCShopBuy')
                    gold, _ = once('CSItemUse', {'costItem': {'id': 'item_041', 'num': 1}}, 'SCItemUse')
                    self.assertEqual(sum(x.num for x in getattr(gold, 'get').simps if x.id == 'coin'), 1000000)
                    source_stones = sum(x.num for x in getattr(gold, 'get').simps if x.id == 'item_012')
                    self.assertEqual(source_stones, contract['originalVIP7Stone'])
                    self.assertEqual(count('item_012'), source_stones)
                    ordinary = http('CSShopProductList', {'shopType': 1}, 'SCShopProductList')
                    stones = next(x for x in ordinary.shopItemList if x.id == 'itemShopId_11')
                    self.assertEqual([(x.id, x.num) for x in stones.item], [('item_012', 5)])
                    self.assertEqual([(x.id, x.num) for x in stones.price], [('diamond', 15)])
                    once('CSShopBuy', {'shopType': 1, 'shopItemId': stones.id, 'buyCount': 12}, 'SCShopBuy')
                    self.assertEqual(count('item_012'), source_stones + 60)
                    refined = []
                    for gear in acquired:
                        result, _ = once('CSTreasureRefine', {'targetId': gear.guid}, 'SCTreasureRefine')
                        self.assertEqual((result.treasure.guid, result.treasure.refineLevel), (gear.guid, 1))
                        self.assertEqual({x.id: x.num for x in result.cost}, {'item_012': 30, 'coin': 30000})
                        refined.append(result.treasure.treasureId)
                    self.assertEqual((count('item_012'), self.profile(owner)[0]['coin']), (source_stones, 940000))
                    for gear in acquired:
                        once('CSFormationOn', {'id': gear.treasureId, 'guid': gear.guid, 'formatPos': 1}, 'SCFormationOn')
                    formation = http('CSFormationList', {}, 'SCFormationList')
                    mounted = {t.guid: t.pos for slot in formation.formationList for t in slot.things if t.pos in (5, 6)}
                    self.assertEqual(mounted, {acquired[0].guid: 5, acquired[1].guid: 6})
                    for gear in acquired:
                        once('CSFormationOff', {'id': gear.treasureId, 'guid': gear.guid, 'formatPos': 1}, 'SCFormationOff')
                    reject('CStreasureRebuild', {'targetId': acquired[0].guid})  # original reset opens at16
                    level(16)
                    resets = []
                    for gear in acquired:
                        fields = {'targetId': gear.guid}
                        reset, saved = once('CStreasureRebuild', fields, 'SCtreasureRebuild')
                        self.assertEqual([(x.id, x.num) for x in reset.cost], [('diamond', 50)])
                        self.assertEqual({x.id: x.num for x in getattr(reset, 'get').simps}, {'item_012': 30, 'coin': 30000})
                        restored = next(x for x in treasures() if x['guid'] == gear.guid)
                        self.assertEqual((restored['strengthLevel'], restored.get('refineLevel', 0), restored.get('currentExp', 0)), (1, 0, 0))
                        resets.append((fields, reset, saved))
                    self.assertEqual((count('item_012'), self.profile(owner)[0]['coin'], self.profile(owner)[0]['diamond']), (source_stones + 60, 1000000, 21220))
                    day0 = self.tools(owner, 'time/state')
                    self.assertEqual(day0['offsetDays'], 0)
                    day_key = time.strftime('%Y-%m-%d', time.gmtime(day0['gameTimeMs'] / 1000 + 28800))
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p3_membership_claims WHERE player_id={owner.playerId} AND kind='vip-shop:{PRODUCT}' AND period='{day_key}';").strip(), '1')
                    frozen_before = self.sql(f"SELECT HEX(snapshot) FROM p3_payment_activity_states WHERE player_id={owner.playerId} AND activity_id='vip-shop:{PRODUCT}:{day_key}';").strip()
                    self.assertTrue(frozen_before)
                    self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], 1)
                    self.assertEqual(vip_quote().dayLeftTimes, 1)
                    self.assertEqual(self.change(owner, 'reset')['offsetDays'], 0)
                    reject('CSShopBuy', vip_buy)
                    self.assertEqual(vip_quote().dayLeftTimes, 0)
                    self.assertEqual(self.sql(f"SELECT HEX(snapshot) FROM p3_payment_activity_states WHERE player_id={owner.playerId} AND activity_id='vip-shop:{PRODUCT}:{day_key}';").strip(), frozen_before)
                    self.assertEqual(self.sql(f"SELECT status,cents,credited,created_at,expires_at,confirmed_at FROM p3_payment_orders WHERE id='{order.orderId}';").strip(), order_before)
                    after_reset = self.profile(owner)
                    self.stop()
                    self.start()
                    owner = self.login(identity)
                    self.assertEqual(self.profile(owner), after_reset)
                    for fields, reply, saved in resets:
                        self.assertEqual(self.call('CStreasureRebuild', fields, owner, saved, 'SCtreasureRebuild'), reply)
                    self.assertEqual(self.profile(owner), after_reset)
                    reject('CSShopBuy', vip_buy)
                    sale_fields = {'guids': [x.guid for x in acquired]}
                    sold, sale_stamp = once('CSTreasureSell', sale_fields, 'SCTreasureSell')
                    self.assertEqual(sum(x.num for x in getattr(sold, 'get').simps if x.id == 'coin'), 2000)
                    self.assertFalse(treasures())
                    final = self.profile(owner)
                    self.assertEqual((final[0]['coin'], final[0]['diamond'], count('item_012')), (1002000, 21220, source_stones + 60))
                    self.stop()
                    self.start()
                    owner = self.login(identity)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.call('CSTreasureSell', sale_fields, owner, sale_stamp, 'SCTreasureSell'), sold)
                    reject('CStreasureRebuild', {'targetId': acquired[0].guid})
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.profile(protected), protected_before)
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    report.update(status='PASS', fullChain=True, actualParcelID='item_040', actualPaidVIP=actual_vip,
                        actualTreasureIDs=TREASURES, actualTreasureGUIDs=[x.guid for x in acquired],
                        actualRefinedTreasureIDs=sorted(refined), actualResetTreasureIDs=TREASURES,
                        actualRecycledTreasureIDs=TREASURES, actualOrderId=order.orderId,
                        actualWalletCents=200000, actualVIP6Cost=700, actualVIP7Gold=1000000,
                        original040CompleteRewardPreserved=True, actualStonePurchased=60,
                        actualStoneConsumed=60, actualVIP7Stone=source_stones, finalStone=source_stones + 60, actualCoinRefineConsumed=60000, actualResetDiamond=100,
                        actualRefundStone=60, actualRefundCoin=60000, actualSaleCoin=2000,
                        finalCoin=1002000, finalDiamond=21220, protected1Exact=True, coldRestored=True,
                        dailyDuplicateAndRewindRejected=True, oldDailyQuoteImmutable=True,
                        paymentRealTimestampsUnchanged=True, originalReset15Denied16Paid=True,
                        eightSameStampOpenRefineWearResetSale=True, vipDailyPurchaseUsesSingleOriginalRequest=True,
                        globalP6Authority=False)
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
