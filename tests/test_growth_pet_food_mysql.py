"""G5 prebuilt managed wallet/VIP gold ->129 source -> actual first-pet intimacy.

Only Player34 is a finite qualification. All pets, food, money and diamonds use
actual production actions. This runner never compiles or touches a live schema.
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
from server.codec import BY_NAME, CLASSES, parse_http

ROOT = Path(__file__).resolve().parents[1]
SOURCE = Path(os.environ.get('HYJS_GROWTH_PET_FOOD_SOURCE', ROOT / 'build/candidates/growth-pet-food-r3-20261017')).resolve()
BINARY = Path(os.environ.get('HYJS_GROWTH_PET_FOOD_BINARY', ROOT / 'build/hyjs-server-growth-pet-food-r3-20261017-pending')).resolve()
VALIDATION = Path(os.environ.get('HYJS_GROWTH_PET_FOOD_VALIDATION', ROOT / 'analysis/p3/growth-pet-food-r3-incremental-20261017.json')).resolve()
DELIVERY = ROOT / 'analysis/growth-first-20261016/resources/g5-pet-food-r1/manifest.json'
PREFIX = 'limitExchange_p4_petfood_'


def protocol_preflight():
    vectors = {'CSPetFirstRecruit': {}, 'CSPetList': {},
        'CSPetEat': {'guid': 'actual-first-pet-guid', 'cost': [{'id': 'item_129', 'num': 32}]},
        'CSBuyActivityShop': {'activityId': 'actual-private-key', 'index': 1, 'num': 5,
            'costResources': [{'id': 'coin', 'num': 60000}], 'getResources': [{'id': 'item_129', 'num': 6}]},
        'CSShopBuy': {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1},
        'CSItemUse': {'costItem': {'id': 'item_041', 'num': 1}}}
    for name, fields in vectors.items():
        raw = shared.wire.encode_original_http(name, fields, 2, b'0' * 16, 129001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original G5 request changed: ' + name)
    if (BY_NAME['CSPetEat'], BY_NAME['SCPetEat']) != (21505, 21506):
        raise AssertionError('Original pet-eat opcode changed')
    pet = next(r for r in original_table('Pet') if r['id'] == 'pet_002')
    food = next(r for r in original_table('Item') if r['itemId'] == 'item_129')
    config = {r['StaticName']: r['NameValue'] for r in original_table('GameConfigure')}
    intimacy = next(r for r in original_table('PetIntimacy') if r['quality'] == '20' and r['intimacyLevel'] == '0')
    next_level = next(r for r in original_table('PetIntimacy') if r['quality'] == '20' and r['intimacyLevel'] == '1')
    if ((pet['quality'], pet['nature'], pet['defaultTalent']) != ('20', '1', 'PetTalent_11')
            or (food['category'], food['nature'], food['exp']) != ('9', '1', '10')
            or config['PetFirstRecruitGet'] != 'pet_002,1' or config['PetUnlockLevel'] != '34'
            or config['PetSameNatureIntimacyProportion'] != '2'
            or intimacy['intimacy'] != '626' or next_level['additionalTalent'] != '1'):
        raise AssertionError('Original first-pet/food/threshold contract differs')
    box = next(r for r in original_table('ItemUse') if r['itemId'] == 'item_041')
    if 'coin,1000000' not in box['result'].split(';'):
        raise AssertionError('Original VIP7 box gold source changed')
    return dict(requestVectors=len(vectors), firstPet='pet_002', food='item_129',
        firstThreshold=626, actualFoodConsumed=32, actualIntimacyGain=640,
        finalIntimacyLevel=1, finalRemainder=14, actualRandomTalentCount=1,
        randomTalent='actual policy draw; no predetermined winner/talent',
        localSource='coin60000 ->129x6; min34/day5/period150; defaultoff')


def preflight():
    contract = protocol_preflight()
    for key, value in {'HYJS_GROWTH_RESOURCE_SOURCE': SOURCE, 'HYJS_GROWTH_RESOURCE_BINARY': BINARY,
                       'HYJS_GROWTH_RESOURCE_VALIDATION': VALIDATION}.items():
        os.environ.setdefault(key, str(value))
    source, binary, manifest_sha = shared.preflight()
    if source != SOURCE:
        raise AssertionError('Use the exact qualified G5 candidate')
    delivery = json.loads(DELIVERY.read_bytes())
    for name, expected in delivery['originalTablePins'].items():
        if shared.sha(source / 'analysis/tables' / ('Table' + name + '.tsv')) != expected:
            raise AssertionError('Original G5 table differs: ' + name)
    hooks = {'hyjs-server/service/p4CommerceService.go': 'seedP9PetFoodSupply(tx, now)',
             'hyjs-server/service/p4ResourceExchangeService.go': 'p9AppendPetFoodBinding',
             'hyjs-server/service/p9PetFoodSupplyService.go': 'product.DailyLimit, product.TotalLimit = 5, 150',
             'hyjs-server/service/p9PetIntimacyService.go': 's.p9AllocateIntimacy(tx, id, pet, unlocked)'}
    contract['productionSHA256'] = {}
    for relative, needle in hooks.items():
        if needle not in (source / relative).read_text():
            raise AssertionError('G5 production integration missing: ' + relative)
        contract['productionSHA256'][relative] = shared.sha(source / relative)
    main = (source / 'hyjs-server/main.go').read_text()
    initializers = ['InitializeP9Pets(ctx)', 'InitializeP9PetNext(ctx)', 'InitializeP9PetRecycle(ctx)',
                    'InitializeP9CopyIntimacy(ctx)', 'InitializeP9PetIntimacy(ctx)']
    if any('s.' + name not in main for name in initializers):
        raise AssertionError('Actual main must initialize the entire pet-eat lifecycle')
    contract['originalMainInitializers'] = initializers
    contract['qualificationSHA256'] = shared.sha(VALIDATION)
    return source, binary, manifest_sha, contract


@unittest.skipUnless(os.environ.get('HYJS_GROWTH_PET_FOOD_MYSQL_TEST') == '1', 'Root managed prebuilt G5 window')
class GrowthPetFoodMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    stamped, profile, start, stop, sql = transport.stamped, transport.profile, transport.start, transport.stop, transport.sql
    json_request, wallet_body, wallet = transport.json_request, transport.wallet_body, transport.wallet
    tools, change, activity, directory = transport.tools, transport.change, transport.activity, transport.directory

    def test_actual_first_fire_pet_paid_food_intimacy_talent_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ.get('HYJS_GROWTH_PET_FOOD_OUTPUT',
            ROOT / 'analysis/growth-first-20261016/resources/g5-http-mysql-r1.json')).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/resources'))
        self.assertFalse(output.exists(), 'Preserve earlier acceptance records')
        report = dict(status='PENDING', fullChain=False, binarySHA256=shared.sha(binary),
            sourceManifestSHA256=manifest_sha, runnerSHA256=shared.sha(__file__), contract=contract,
            finitePreparation='Only isolated Player34; no pet/item/coin/diamond/intimacy or talent injection',
            productionModified=False, externalPayment=False, phoneUI=False, P6AuthorityChanged=False)
        began = time.monotonic()
        self.pool, self.process = ManagedMySQL(), None
        with tempfile.TemporaryDirectory(prefix='hyjs-g5-pet-food-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('growth_pet_food')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    for table, expected in {'local_wallet_entries': {'player_id', 'kind', 'delta_cents'},
                            'p3_vip_shop_products': {'min_v_ip', 'cost', 'reward', 'enabled'},
                            'p4_commerce_rules': {'id', 'enabled', 'source', 'rules', 'frozen'},
                            'p9_pet_talent_allocations': {'guid', 'player_id', 'draws', 'policy_hash'},
                            'p9_pet_instances': {'guid', 'player_id'},
                            'p9_pet_states': {'player_id', 'first_granted'},
                            'p9_pet_receipts': {'player_id', 'operation', 'timestamp'},
                            'p9_pet_intimacy_rules': {'quality', 'enabled', 'pool'},
                            'p9_pet_copy_grants': {'player_id', 'source_key', 'facts'},
                            'p9_pet_deferred_consumptions': {'player_id', 'source_key', 'guid', 'applied'},
                            'p9_pet_retired': {'guid', 'player_id'},
                            'p9_pet_investments': {'guid', 'player_id'}}.items():
                        columns = self.sql('SELECT COLUMN_NAME FROM information_schema.COLUMNS '
                            f"WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='{table}';").split()
                        self.assertTrue(expected <= set(columns), table + ' actual schema')
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-g5-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '精灵供食保护', 'heroId': 'hero_001'}, protected, 129000, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    identity = 'demo-g5-firepet-' + suffix
                    owner = self.login(identity)
                    stamp = 129001

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        saved = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            replies = list(workers.map(lambda _: self.call(name, fields, owner, saved, expected), range(8)))
                        self.assertTrue(all(r == replies[0] for r in replies), '8same-stamp responses differ')
                        return replies[0], saved

                    def reject(name, fields):
                        before = self.profile(owner)
                        http(name, fields, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), before)

                    def food_count():
                        return sum(v['num'] for v in (self.profile(owner)[1].get('items') or []) if v['id'] == 'item_129')

                    def current_pet():
                        pets = http('CSPetList', {}, 'SCPetList').petList
                        self.assertEqual(len(pets), 1)
                        return pets[0]

                    http('CSPlayerCreate', {'playerName': '火食真实亲密', 'heroId': 'hero_001'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual((initial[0].get('coin', 0), initial[0].get('diamond', 0)), (0, 0))
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',34) WHERE id={owner.playerId};")
                    qualified = self.profile(owner)
                    self.assertEqual(qualified, [dict(initial[0], level=34), initial[1]])
                    first, first_stamp = once('CSPetFirstRecruit', {}, 'SCPetFirstRecruit')
                    self.assertEqual(len(getattr(first, 'get').pets), 1)
                    pet = getattr(first, 'get').pets[0]
                    self.assertEqual((pet.id, pet.level, pet.eat.level, pet.eat.exp, pet.initProp), ('pet_002', 1, 0, 0, 'PetTalent_11'))
                    self.assertFalse(pet.randPropList)
                    self.assertEqual(current_pet(), pet)
                    self.assertEqual(food_count(), 0)
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=200000)
                    catalogue = http('CSGetPaymentInfo', {'channelId': 'AND0003', 'shopId': 'common'}, 'SCGetPaymentInfo')
                    product = next(p for p in catalogue.paymentInfoList if p.paymentId == 'djsy.wuyou.andp2000')
                    self.assertEqual(product.money, 2000.0)
                    order, _ = once('CSGenerateOrder', {'paymentId': product.paymentId, 'money': product.money,
                        'moneyType': product.moneyType, 'channelId': 'AND0003', 'shopId': product.shopId}, 'SCGenerateOrder')
                    self.assertEqual(order.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    self.assertEqual((self.profile(owner)[0]['vip'], self.profile(owner)[0]['diamond']), (7, 23000))
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    self.assertEqual(self.sql(f"SELECT COUNT(*),SUM(delta_cents) FROM local_wallet_entries WHERE player_id={owner.playerId} AND kind='debit';").strip(), '1\t-200000')
                    vip = self.sql("SELECT min_v_ip,cost,reward,enabled FROM p3_vip_shop_products WHERE id='p3vip7_growth_local';").strip()
                    self.assertEqual(vip, '7\t800\titem_041,1\t0')
                    reject('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1})
                    self.pool.sql(self.database, "UPDATE p3_vip_shop_products SET enabled=1 WHERE id='p3vip7_growth_local' AND min_v_ip=7 AND cost=800 AND reward='item_041,1' AND enabled=0;")
                    http('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1}, 'SCShopBuy')
                    reject('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1})
                    opened, _ = once('CSItemUse', {'costItem': {'id': 'item_041', 'num': 1}}, 'SCItemUse')
                    self.assertEqual(sum(x.num for x in getattr(opened, 'get').simps if x.id == 'coin'), 1000000)
                    self.assertEqual((self.profile(owner)[0]['coin'], self.profile(owner)[0]['diamond']), (1000000, 22200))
                    # Freeze an old private catalogue before this source is published.
                    self.activity(owner, 'open')
                    old_key, old_hash, old_raw, old_bindings = self.directory(owner)
                    self.assertFalse(any(v['productId'] == 'item_129' for v in old_bindings))
                    row = self.sql("SELECT id,enabled,HEX(rules),source FROM p4_commerce_rules WHERE id LIKE '" + PREFIX +
                        "%' AND LENGTH(id)=" + str(len(PREFIX) + 6) + ' ORDER BY start_at DESC LIMIT 1;').strip().split('\t')
                    child, enabled, raw_rules, rule_source = row
                    self.assertEqual(enabled, '0')
                    self.assertTrue(rule_source.startswith('first-fire-pet-food-coin-supply-v1：'))
                    rules = json.loads(bytes.fromhex(raw_rules))
                    self.assertEqual(len(rules['products']), 1)
                    rule = rules['products'][0]
                    self.assertEqual({k: rule[k] for k in ['id', 'rewards', 'cost', 'minLevel', 'dailyLimit', 'totalLimit']},
                        dict(id='item_129', rewards='item_129,6', cost='coin,60000', minLevel=34, dailyLimit=5, totalLimit=150))
                    reject('CSActivityShopInfo', {'activityId': child})
                    self.pool.sql(self.database, f"UPDATE p4_commerce_rules SET enabled=1 WHERE id='{child}' AND enabled=0 AND source LIKE 'first-fire-pet-food-coin-supply-v1%';")
                    old_listing = http('CSActivityShopInfo', {'activityId': old_key}, 'SCActivityShopInfo')
                    self.assertFalse(any(r.id == 'item_129' for x in old_listing.shopItemList for r in x.getResource))
                    self.assertEqual(self.directory(owner)[:3], (old_key, old_hash, old_raw))
                    self.activity(owner, 'refresh')
                    directory_key, _, _, bindings = self.directory(owner)
                    self.assertNotEqual(directory_key, old_key)
                    addresses = [v for v in bindings if v['productId'] == 'item_129']
                    self.assertEqual(len(addresses), 1)
                    self.assertTrue(addresses[0]['childId'].startswith(PREFIX))

                    def quote():
                        listing = http('CSActivityShopInfo', {'activityId': directory_key}, 'SCActivityShopInfo')
                        value = next(x for x in listing.shopItemList if x.index == addresses[0]['index'])
                        self.assertEqual([(x.id, x.num) for x in value.costResource], [('coin', 60000)])
                        self.assertEqual([(x.id, x.num) for x in value.getResource], [('item_129', 6)])
                        return value

                    def request(number):
                        value = quote()
                        return {'activityId': directory_key, 'index': value.index, 'num': number,
                            'costResources': [{'id': x.id, 'num': x.num} for x in value.costResource],
                            'getResources': [{'id': x.id, 'num': x.num} for x in value.getResource]}

                    buy5 = request(5)
                    day0 = self.tools(owner, 'time/state')
                    self.assertEqual(day0['offsetDays'], 0)
                    self.assertEqual(self.sql(f"SELECT offset_days FROM local_play_clock_states WHERE player_id={owner.playerId};").strip(), '0')
                    bought5, buy5_stamp = once('CSBuyActivityShop', buy5, 'SCGetActivityShopReward')
                    self.assertEqual((food_count(), self.profile(owner)[0]['coin']), (30, 700000))
                    self.assertEqual((quote().total, quote().left), (5, 0))
                    reject('CSBuyActivityShop', request(1))
                    day1 = self.change(owner, 'nextDay')
                    self.assertEqual(day1['offsetDays'], 1)
                    self.assertEqual(self.sql(f"SELECT offset_days FROM local_play_clock_states WHERE player_id={owner.playerId};").strip(), '1')
                    date0 = time.strftime('%Y-%m-%d', time.gmtime(day0['gameTimeMs'] / 1000 + 28800))
                    date1 = time.strftime('%Y-%m-%d', time.gmtime(day1['gameTimeMs'] / 1000 + 28800))
                    self.assertNotEqual(date0, date1)
                    buy1 = request(1)
                    bought1, buy1_stamp = once('CSBuyActivityShop', buy1, 'SCGetActivityShopReward')
                    self.assertEqual((food_count(), self.profile(owner)[0]['coin']), (36, 640000))
                    self.assertEqual(quote().left, 4)
                    self.assertEqual(self.change(owner, 'reset')['offsetDays'], 0)
                    reject('CSBuyActivityShop', request(1))
                    rewind_feed = os.environ.get('HYJS_GROWTH_PET_FOOD_REWIND_FEED') == '1'
                    if rewind_feed:
                        self.assertEqual(self.change(owner, 'previousDay')['offsetDays'], -1)
                    feed_clock = self.tools(owner, 'time/state')
                    feed_request = {'guid': pet.guid, 'cost': [{'id': 'item_129', 'num': 32}]}
                    fed, feed_stamp = once('CSPetEat', feed_request, 'SCPetEat')
                    self.assertEqual([(x.id, x.num) for x in fed.cost], [('item_129', 32)])
                    self.assertEqual((fed.pet.guid, fed.pet.id, fed.pet.eat.level, fed.pet.eat.exp), (pet.guid, 'pet_002', 1, 14))
                    self.assertEqual(len(fed.pet.randPropList), 1)
                    self.assertIn(fed.pet.randPropList[0], ['PetTalent_' + str(i) for i in range(12, 21)])
                    self.assertEqual(current_pet(), fed.pet)
                    self.assertEqual((food_count(), self.profile(owner)[0]['coin']), (4, 640000))
                    allocation_row = self.sql(f"SELECT player_id,HEX(draws) FROM p9_pet_talent_allocations WHERE guid='{pet.guid}';").strip().split('\t')
                    self.assertEqual(int(allocation_row[0]), owner.playerId)
                    self.assertEqual(json.loads(bytes.fromhex(allocation_row[1])), list(fed.pet.randPropList))
                    if rewind_feed:
                        fact = self.sql("SELECT day,amount FROM p1_event_facts WHERE player_id=" + str(owner.playerId) +
                            " AND source_key='p9:eat:" + str(feed_stamp) + "' AND kind='spend:item_129';").strip().split('\t')
                        expected_day = time.strftime('%Y-%m-%d', time.gmtime(feed_clock['gameTimeMs'] / 1000 + 28800))
                        self.assertEqual(fact, [expected_day, '32'])
                        self.assertEqual(self.change(owner, 'reset')['offsetDays'], 0)
                    frozen = self.sql(f"SELECT input_hash,HEX(bindings) FROM p4_resource_exchange_directories WHERE id='{old_key}';").strip()
                    self.assertEqual(frozen, old_hash + '\t' + old_raw)
                    final = self.profile(owner)
                    self.stop()
                    self.start()
                    owner = self.login(identity)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(current_pet(), fed.pet)
                    self.assertEqual(self.call('CSPetEat', feed_request, owner, feed_stamp, 'SCPetEat'), fed)
                    self.assertEqual(self.call('CSBuyActivityShop', buy5, owner, buy5_stamp, 'SCGetActivityShopReward'), bought5)
                    self.assertEqual(self.call('CSBuyActivityShop', buy1, owner, buy1_stamp, 'SCGetActivityShopReward'), bought1)
                    self.assertEqual(self.tools(owner, 'time/state')['offsetDays'], 0)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.profile(protected), protected_before)
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    report.update(status='PASS', fullChain=True, actualPetGUID=pet.guid, actualPetID=pet.id,
                        actualOrderId=order.orderId, actualWalletCents=200000, actualVIP7Boxes=1,
                        actualCoinEarned=1000000, source129={'coinSpent': 360000, 'originalFoodsBought': 36,
                        'originalFoodsConsumed': 32, 'originalFoodsRemaining': 4, 'privateDirectory': directory_key},
                        actualFoodPurchased=36, actualFoodConsumed=32, actualIntimacyExp=14,
                        actualFeedPersonalOffset=feed_clock['offsetDays'], actualRewindFeedVerified=rewind_feed,
                        actualRandomTalentCount=len(fed.pet.randPropList),
                        original129Consumed=32, actualIntimacyLevel=1, actualIntimacyRemainder=14,
                        actualRandomTalent=list(fed.pet.randPropList), finalCoin=640000, finalDiamond=22200,
                        twoPersonalDates=True, actualPersonalDates=[date0, date1], resetOffset0=True, dayLimitAndRewindVerified=True,
                        period150ContractPreserved=True, period150ExhaustionRevalidated=False,
                        oldPrivateImmutable=True, newPrivateExplicitRefresh=True, protected1Exact=True,
                        eightSameStampPurchaseAndFeed=True, coldOriginalReceipts=True,
                        sourcePolicy='first-fire-pet-food-coin-supply-v1', globalP6Authority=False)
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
