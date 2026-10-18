"""Managed, prebuilt G4: real VIP gold -> four Glory bodies -> forge/recycle.

Only Player60 is a finite eligibility input. Currency comes from one real local
wallet-backed original 2000-yuan order and ten daily paid original VIP7 boxes.
No inventory, GUID, forge outcome, diamond, coin or battle result is inserted.
"""
import concurrent.futures
import datetime
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

import test_growth_resource_loop_mysql as shared
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_p5_talent_scope_mysql import original_table
from hyjs_managed_mysql import ManagedMySQL
from server.codec import CLASSES, parse_http

ROOT = Path(__file__).resolve().parents[1]
PREFIX = 'limitExchange_p4_gloryfrag_'
POLICY = 'glory-forge-fragment-single-player-v1'
EQUIPS = tuple('equip_0' + str(i) for i in range(41, 45))
FRAGS = tuple('frag_' + item for item in EQUIPS)


def protocol_preflight():
    vectors = {
        'CSGetPaymentInfo': {'channelId': 'AND0003', 'shopId': 'common'},
        'CSGenerateOrder': {'paymentId': 'djsy.wuyou.andp2000', 'money': 2000.0,
                            'moneyType': 'RMB', 'channelId': 'AND0003', 'shopId': 'common'},
        'CSShopBuy': {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1},
        'CSItemUse': {'costItem': {'id': 'item_041', 'num': 1}},
        'CSActivityShopInfo': {'activityId': 'actual-private-directory'},
        'CSBuyActivityShop': {'activityId': 'actual-private-directory', 'index': 1, 'num': 5,
                              'costResources': [{'id': 'coin', 'num': 100000}],
                              'getResources': [{'id': FRAGS[0], 'num': 3}]},
        'CSCombine': {'targetId': EQUIPS[0]},
        'CSEquipForge': {'guid': 'actual-guid', 'cost': [{'id': FRAGS[0], 'num': 3}, {'id': 'coin', 'num': 1000}]},
        'CSFormationOn': {'id': EQUIPS[0], 'guid': 'actual-guid', 'formatPos': 1},
        'CSFormationOff': {'id': EQUIPS[0], 'guid': 'actual-guid', 'formatPos': 1},
        'CSEquipReBuild': {'guid': 'actual-guid'},
        'CSEquipDecompose': {'guid': ['actual-guid']},
    }
    for name, fields in vectors.items():
        raw = shared.wire.encode_original_http(name, fields, 2, b'0' * 16, 441001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original G4 request changed: ' + name)
    responses = {'SCGetPaymentInfo': {'paymentInfoList'}, 'SCGenerateOrder': {'orderId', 'transactionData'},
                 'SCItemUse': {'get', 'cost'}, 'SCActivityShopInfo': {'shopItemList'},
                 'ActivityShopItem': {'index', 'costResource', 'getResource', 'left', 'total'},
                 'SCGetActivityShopReward': {'get', 'costResources'}, 'SCCombine': {'get'},
                 'SCEquipForge': {'equip', 'result', 'cost'}, 'SCFormationOn': {'formationItem', 'fightpower'},
                 'SCFormationOff': {'fightpower'}, 'SCEquipReBuild': {'get'}, 'SCEquipDecompose': {'get', 'cost'},
                 'Equipment': {'guid', 'equipId', 'forgeLevel'}, 'ResultObject': {'equips', 'simps'}}
    for name, fields in responses.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original G4 response changed: ' + name)
    fragments = {r['id']: r for r in original_table('Frag')}
    equips = {r['id']: r for r in original_table('Equip')}
    for item, frag in zip(EQUIPS, FRAGS):
        if (fragments[frag]['target'], fragments[frag]['num'], fragments[frag]['price'], equips[item]['quality']) != (item, '60', '500', '23'):
            raise AssertionError('Original sixty-fragment Glory recipe changed')
    config = {r['StaticName']: r['NameValue'] for r in original_table('GameConfigure')}
    if config['EquipForgeLevel'] != '60' or config['EquipmentRebuildCost'] != 'diamond,50':
        raise AssertionError('Original forge/reset gate changed')
    forge = [r for r in original_table('EquipForgeOdds') if r['quality'] == '23' and r['forgeLevel'] == '1']
    if len(forge) != 1 or (forge[0]['odds'], forge[0]['cost'], forge[0]['fragNum']) != ('100', 'coin,1000', '3'):
        raise AssertionError('Original first forge odds/cost changed')
    if not any(r.get('quality') == '23' and r.get('retRes') == 'point,30000;coin,1000'
               for r in original_table('EquipDecompose')):
        raise AssertionError('Original red decomposition changed')
    return dict(requestVectors=len(vectors), responseContracts=len(responses), equips=list(EQUIPS),
                qualification='Only Player60; no resource/GUID/result insertion', walletCents=200000,
                paidVIP7Boxes=10, actualGold=10000000, fragmentBundlesPerProduct=21,
                fragmentsPerProduct=63, fragmentCost=8400000, expectedFinalCoin=1604000,
                expectedFinalDiamond=14800, expectedFinalPoint=120000,
                nativeEvidence='Historical original P5 highest matrix reused; no P6 battle or new native claim')


def preflight():
    contract = protocol_preflight()
    source = Path(os.environ['HYJS_GROWTH_GLORY_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_GROWTH_GLORY_BINARY']).resolve()
    validation = Path(os.environ['HYJS_GROWTH_GLORY_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use the direct frozen root candidate')
    manifest = source / 'manifest.json'
    paths = set()
    for entry in json.loads(manifest.read_text())['files']:
        path = (source / entry['path']).resolve()
        if source not in path.parents or path in paths or shared.sha(path) != entry['sha256']:
            raise AssertionError('Frozen candidate pin mismatch: ' + entry['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Unpinned candidate Go source')
    proof = json.loads(validation.read_text())
    if proof.get('qualified') is not True or Path(proof.get('candidate', '')).resolve() != source:
        raise AssertionError('Exact completed root qualification is required')
    if shared.sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Qualified binary differs; never compile in runner')
    for name in ('Frag', 'Equip', 'EquipForgeOdds', 'EquipForgeProp', 'EquipDecompose', 'GameConfigure', 'Payment', 'ItemUse'):
        relative = 'analysis/tables/Table' + name + '.tsv'
        if shared.sha(source / relative) != shared.sha(ROOT / relative):
            raise AssertionError('Original runtime asset differs: ' + name)
    for relative in ('server/codec.py', 'server/protocol/schema.json'):
        if shared.sha(source / relative) != shared.sha(ROOT / relative):
            raise AssertionError('Original codec differs: ' + relative)
    hooks = {'hyjs-server/service/p4CommerceService.go': 'seedP5GloryFragmentSupply(tx, now)',
             'hyjs-server/service/p4ResourceExchangeService.go': 'p5AppendGloryFragmentBinding',
             'hyjs-server/service/p5GloryFragmentSupplyService.go': 'product.DailyLimit, product.TotalLimit = 5, 150',
             'hyjs-server/constUtil/p5GloryFragmentSupply.go': POLICY}
    contract['sourceSHA256'] = {}
    for relative, needle in hooks.items():
        if needle not in (source / relative).read_text():
            raise AssertionError('Missing G4 production integration: ' + relative)
        contract['sourceSHA256'][relative] = shared.sha(source / relative)
    contract['runnerDependencySHA256'] = {relative: shared.sha(ROOT / relative) for relative in (
        'tests/hyjs_managed_mysql.py', 'tests/test_growth_recruit_mysql.py',
        'tests/test_growth_resource_loop_mysql.py', 'tests/test_continued_gameplay_mysql.py',
        'tests/test_local_wallet_mysql.py', 'tests/test_p5_talent_scope_mysql.py')}
    return source, binary, shared.sha(manifest), contract


@unittest.skipUnless(os.environ.get('HYJS_GROWTH_GLORY_MYSQL_TEST') == '1', 'Root managed prebuilt G4 SQL window')
class GrowthGloryEquipmentMySQLTests(unittest.TestCase):
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
    tools = transport.tools
    change = transport.change
    activity = transport.activity
    directory = transport.directory

    def test_actual_gold_four_glory_growth_refund_recycle_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_GROWTH_GLORY_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/resources'))
        self.assertFalse(output.exists(), 'Preserve prior evidence; choose a new attempt record')
        self.pool, self.process = ManagedMySQL(), None
        report = dict(status='PENDING', fullChain=False, runnerSHA256=shared.sha(Path(__file__)),
                      sourceManifestSHA256=manifest_sha, binarySHA256=shared.sha(binary), contract=contract,
                      finitePreparation='Only Player60', productionModified=False, externalPayment=False,
                      phoneUI=False, highestStageRevalidated=False, nativeBattleStarted=False)
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-growth-glory-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('growth_glory')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    for table, columns in {
                        'p4_commerce_rules': {'id', 'enabled', 'rules', 'source', 'frozen'},
                        'p4_resource_exchange_directories': {'id', 'input_hash', 'bindings'},
                        'p3_vip_shop_products': {'id', 'min_v_ip', 'cost', 'reward', 'enabled'},
                        'local_wallet_entries': {'player_id', 'kind', 'delta_cents'},
                        'p3_payment_orders': {'player_id', 'status', 'cents', 'credited'},
                        'first_double_policies': {'id', 'enabled'},
                        'p4_receipts': {'player_id', 'domain', 'timestamp', 'request', 'response'},
                        'p5_mutation_receipts': {'player_id', 'operation', 'timestamp', 'request', 'response'},
                        'equipment_mutation_receipts': {'player_id', 'opcode', 'timestamp', 'request', 'response'},
                    }.items():
                        actual = self.sql('SELECT COLUMN_NAME FROM information_schema.COLUMNS '
                                          f"WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='{table}';").split()
                        self.assertTrue(columns <= set(actual), 'Actual G4 SQL columns changed: ' + table)
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    self.assertEqual(self.sql('SELECT enabled FROM first_double_policies WHERE id=1;').strip(), '0')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-g4-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '荣耀保护账号', 'heroId': 'hero_001'}, protected, 441000, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-g4-glory-' + suffix
                    owner, stamp = self.login(uid), 441001

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
                        self.assertTrue(all(reply == replies[0] for reply in replies))
                        return replies[0], saved

                    def reject(name, fields, saved=None):
                        before = self.profile(owner)
                        if saved is None:
                            http(name, fields, 'ErrorMessage')
                        else:
                            self.call(name, fields, owner, saved, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), before)

                    def count(item):
                        return sum(row['num'] for row in (self.profile(owner)[1].get('items') or []) if row['id'] == item)

                    http('CSPlayerCreate', {'playerName': '荣耀真实培养', 'heroId': 'hero_001'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual(tuple(initial[0].get(k, 0) for k in ('coin', 'diamond', 'point')), (0, 0, 0))
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=200000)
                    catalogue = http('CSGetPaymentInfo', {'channelId': 'AND0003', 'shopId': 'common'}, 'SCGetPaymentInfo')
                    product = next(row for row in catalogue.paymentInfoList if row.paymentId == 'djsy.wuyou.andp2000')
                    self.assertEqual(product.money, 2000.0)
                    order, _ = once('CSGenerateOrder', {'paymentId': product.paymentId, 'money': product.money,
                        'moneyType': product.moneyType, 'channelId': 'AND0003', 'shopId': product.shopId}, 'SCGenerateOrder')
                    self.assertEqual(order.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    self.assertEqual((self.profile(owner)[0]['vip'], self.profile(owner)[0]['diamond']), (7, 23000))
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    self.assertEqual(self.sql(f"SELECT COUNT(*),SUM(delta_cents) FROM local_wallet_entries WHERE player_id={owner.playerId} AND kind='debit';").strip(), '1\t-200000')
                    self.assertEqual(self.sql("SELECT min_v_ip,cost,reward,enabled FROM p3_vip_shop_products WHERE id='p3vip7_growth_local';").strip(), '7\t800\titem_041,1\t0')
                    self.pool.sql(self.database, "UPDATE p3_vip_shop_products SET enabled=1 WHERE id='p3vip7_growth_local' AND min_v_ip=7 AND cost=800 AND reward='item_041,1' AND enabled=0;")
                    for day in range(10):
                        if day:
                            self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], day)
                        http('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1}, 'SCShopBuy')
                        opened, _ = once('CSItemUse', {'costItem': {'id': 'item_041', 'num': 1}}, 'SCItemUse')
                        self.assertEqual(sum(x.num for x in getattr(opened, 'get').simps if x.id == 'coin'), 1000000)
                    self.assertEqual((self.profile(owner)[0]['coin'], self.profile(owner)[0]['diamond']), (10000000, 15000))
                    current_time = self.change(owner, 'reset')
                    self.assertEqual(current_time['offsetDays'], 0)
                    current_month = datetime.datetime.fromtimestamp(current_time['gameTimeMs'] / 1000,
                        datetime.timezone(datetime.timedelta(hours=8))).strftime('%Y%m')
                    reject('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1})
                    self.activity(owner, 'open')
                    old_key, old_hash, old_raw, old_bindings = self.directory(owner)
                    self.assertFalse(any(b.get('productId') in FRAGS for b in old_bindings))
                    source_row = self.sql("SELECT id,enabled,HEX(rules),source FROM p4_commerce_rules WHERE id='" +
                        PREFIX + current_month + "';").strip().split('\t')
                    self.assertEqual(len(source_row), 4)
                    self.assertEqual(source_row[1], '0')
                    self.assertTrue(source_row[3].startswith(POLICY + '：'))
                    products = json.loads(bytes.fromhex(source_row[2]))['products']
                    self.assertEqual({p['id'] for p in products}, set(FRAGS))
                    for p in products:
                        self.assertEqual((p['rewards'], p['cost'], p['minLevel'], p['dailyLimit'], p['totalLimit']),
                                         (p['id'] + ',3', 'coin,100000', 60, 5, 150))
                    self.pool.sql(self.database, "UPDATE p4_commerce_rules SET enabled=1 WHERE id='" + source_row[0] + "' AND enabled=0;")
                    old_listing = http('CSActivityShopInfo', {'activityId': old_key}, 'SCActivityShopInfo')
                    self.assertFalse(any(x.id in FRAGS for q in old_listing.shopItemList for x in q.getResource))
                    self.activity(owner, 'refresh')
                    key, key_hash, key_raw, bindings = self.directory(owner)
                    addresses = {b['productId']: b['index'] for b in bindings if b.get('productId') in FRAGS}
                    self.assertEqual(set(addresses), set(FRAGS))
                    self.assertNotEqual(key, old_key)

                    def quote(frag):
                        listing = http('CSActivityShopInfo', {'activityId': key}, 'SCActivityShopInfo')
                        found = [q for q in listing.shopItemList if q.index == addresses[frag]]
                        self.assertEqual(len(found), 1)
                        q = found[0]
                        self.assertEqual([(x.id, x.num) for x in q.costResource], [('coin', 100000)])
                        self.assertEqual([(x.id, x.num) for x in q.getResource], [(frag, 3)])
                        return q

                    def buy_fields(frag, number):
                        q = quote(frag)
                        return {'activityId': key, 'index': q.index, 'num': number,
                                'costResources': [{'id': x.id, 'num': x.num} for x in q.costResource],
                                'getResources': [{'id': x.id, 'num': x.num} for x in q.getResource]}

                    reject('CSBuyActivityShop', buy_fields(FRAGS[0], 1))
                    player, game = self.profile(owner)
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',60) WHERE id={owner.playerId} AND uid LIKE 'demo-g4-glory-%';")
                    qualified = self.profile(owner)
                    self.assertEqual(qualified[0], dict(player, level=60))
                    self.assertEqual(qualified[1], game)
                    tampered = buy_fields(FRAGS[0], 1)
                    tampered['costResources'] = [{'id': 'coin', 'num': 99999}]
                    reject('CSBuyActivityShop', tampered)
                    purchases = []
                    for day in range(5):
                        if day:
                            self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], day)
                        for frag in FRAGS:
                            n = 5 if day < 4 else 1
                            request = buy_fields(frag, n)
                            before_coin, before_frag = self.profile(owner)[0]['coin'], count(frag)
                            response, saved = once('CSBuyActivityShop', request, 'SCGetActivityShopReward')
                            purchases.append((request, response, saved))
                            self.assertEqual(self.profile(owner)[0]['coin'], before_coin - n * 100000)
                            self.assertEqual(count(frag), before_frag + n * 3)
                            if day == 0:
                                reject('CSBuyActivityShop', buy_fields(frag, 1))
                    self.assertEqual([count(frag) for frag in FRAGS], [63] * 4)
                    self.assertEqual(self.profile(owner)[0]['coin'], 1600000)
                    self.assertEqual(self.change(owner, 'reset')['offsetDays'], 0)
                    for frag in FRAGS:
                        self.assertEqual(quote(frag).left, 0)
                        reject('CSBuyActivityShop', buy_fields(frag, 1))
                    bodies, receipts = [], []
                    for equip, frag in zip(EQUIPS, FRAGS):
                        combined, combine_stamp = once('CSCombine', {'targetId': equip}, 'SCCombine')
                        gear = getattr(combined, 'get').equips[0]
                        self.assertEqual(gear.equipId, equip)
                        self.assertTrue(gear.guid)
                        self.assertEqual(count(frag), 3)
                        forge_request = {'guid': gear.guid, 'cost': [{'id': frag, 'num': 3}, {'id': 'coin', 'num': 1000}]}
                        forged, forge_stamp = once('CSEquipForge', forge_request, 'SCEquipForge')
                        self.assertEqual((forged.result, forged.equip.forgeLevel), (1, 1))
                        self.assertEqual(count(frag), 0)
                        reject('CSCombine', {'targetId': equip}, combine_stamp)
                        saved_profile = self.profile(owner)
                        self.stop()
                        self.start()
                        owner = self.login(uid)
                        self.assertEqual(self.profile(owner), saved_profile)
                        self.assertEqual(self.call('CSEquipForge', forge_request, owner, forge_stamp, 'SCEquipForge'), forged)
                        worn = http('CSFormationOn', {'id': equip, 'guid': gear.guid, 'formatPos': 1}, 'SCFormationOn')
                        self.assertTrue(any(x.guid == gear.guid for x in worn.formationItem.things))
                        http('CSFormationOff', {'id': equip, 'guid': gear.guid, 'formatPos': 1}, 'SCFormationOff')
                        before_reset = self.profile(owner)[0]
                        reset, reset_stamp = once('CSEquipReBuild', {'guid': gear.guid}, 'SCEquipReBuild')
                        self.assertEqual(count(frag), 3)
                        after_reset = self.profile(owner)[0]
                        self.assertEqual(after_reset['coin'], before_reset['coin'] + 1000)
                        self.assertEqual(after_reset['diamond'], before_reset['diamond'] - 50)
                        reset_profile = self.profile(owner)
                        self.stop()
                        self.start()
                        owner = self.login(uid)
                        self.assertEqual(self.profile(owner), reset_profile)
                        self.assertEqual(self.call('CSEquipReBuild', {'guid': gear.guid}, owner, reset_stamp, 'SCEquipReBuild'), reset)
                        removed, removed_stamp = once('CSEquipDecompose', {'guid': [gear.guid]}, 'SCEquipDecompose')
                        gain = {x.id: x.num for x in getattr(removed, 'get').simps}
                        self.assertEqual(gain, {'point': 30000, 'coin': 1000})
                        reject('CSCombine', {'targetId': equip}, combine_stamp)
                        reject('CSEquipForge', forge_request, forge_stamp)
                        reject('CSEquipReBuild', {'guid': gear.guid}, reset_stamp)
                        reject('CSEquipDecompose', {'guid': [gear.guid]})
                        bodies.append({'id': equip, 'guid': gear.guid, 'forgeLevel': 1, 'recycled': True})
                        receipts.append(('CSEquipDecompose', {'guid': [gear.guid]}, removed_stamp, removed))
                    final = self.profile(owner)
                    self.assertEqual(tuple(final[0].get(k, 0) for k in ('coin', 'diamond', 'point')), (1604000, 14800, 120000))
                    self.assertEqual([count(frag) for frag in FRAGS], [3] * 4)
                    self.assertFalse({b['guid'] for b in bodies} & {e['guid'] for e in final[1].get('equipment', [])})
                    for frozen_key, digest, raw in ((old_key, old_hash, old_raw), (key, key_hash, key_raw)):
                        self.assertEqual(self.sql("SELECT input_hash,HEX(bindings) FROM p4_resource_exchange_directories WHERE id='" + frozen_key + "';").strip(), digest + '\t' + raw)
                    self.activity(owner, 'refresh')
                    fresh_key, _, _, fresh_bindings = self.directory(owner)
                    self.assertNotEqual(fresh_key, key)
                    self.assertEqual({b['productId'] for b in fresh_bindings if b.get('productId') in FRAGS}, set(FRAGS))
                    fresh_listing = http('CSActivityShopInfo', {'activityId': fresh_key}, 'SCActivityShopInfo')
                    indices = {b['index'] for b in fresh_bindings if b.get('productId') in FRAGS}
                    self.assertEqual([q.left for q in fresh_listing.shopItemList if q.index in indices], [5] * 4)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.sql("SELECT input_hash,HEX(bindings) FROM p4_resource_exchange_directories WHERE id='" + key + "';").strip(), key_hash + '\t' + key_raw)
                    actual_operations = self.sql(f"SELECT operation,COUNT(*) FROM p5_mutation_receipts WHERE player_id={owner.playerId} "
                        "AND operation IN ('equipment-reset','equipment-decompose') GROUP BY operation ORDER BY operation;").strip()
                    self.assertEqual(actual_operations, 'equipment-decompose\t4\nequipment-reset\t4')
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.tools(owner, 'time/state')['offsetDays'], 0)
                    # Decomposition carries only simple refunds; removed full
                    # body/reset projections are rejected above, never revived.
                    for name, request, saved, response in receipts:
                        self.assertEqual(self.call(name, request, owner, saved, name.replace('CS', 'SC', 1)), response)
                    request, response, saved = purchases[0]
                    self.assertEqual(self.call('CSBuyActivityShop', request, owner, saved, 'SCGetActivityShopReward'), response)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.profile(protected), protected_before)
                    report.update(status='PASS', fullChain=True, actualOrderID=order.orderId, actualWalletCents=200000,
                        paidVIP7Boxes=10, actualGold=10000000, fragmentCoinSpent=8400000,
                        actualBodies=bodies, finalCoin=1604000, finalDiamond=14800, finalPoint=120000,
                        actualCombinedEquipIDs=list(EQUIPS), actualForgedEquipIDs=list(EQUIPS),
                        actualRecycledEquipIDs=list(EQUIPS),
                        sourceID=source_row[0], sourceProducts=products, firstPrivateDirectory=old_key,
                        purchasedDirectory=key, refreshedDirectory=fresh_key, oldDirectoryImmutable=True,
                        rewindDailyCapPreserved=True, eightSameStampOnce=True, originalGuaranteedFirstForge=True,
                        staleGrownAndDeletedGUIDRejected=True, coldOriginalReceipts=True, protected1Exact=True,
                        originalQuoteTamperRejected=True, resetAndDecomposeCommittedOnceCounts=actual_operations,
                        datesRestoredTo0=True, refreshCreatesExplicitIndependentPeriodQuota=True,
                        staticBattlePanelHTTP=False, staticPanelEvidence='Existing domain test battlePlayer before/after; no new Run/P6 battle')
                except BaseException as error:
                    report.update(status='FAIL', failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    cleanup_error = None
                    try:
                        self.stop()
                    except BaseException as error:
                        cleanup_error = error
                        report.update(status='FAIL', fullChain=False, cleanupStopFailure=str(error))
                    report['ownedAppStopped'] = self.process is None or self.process.poll() is not None
                    try:
                        self.pool.close()
                    except BaseException as error:
                        cleanup_error = error
                        report.update(status='FAIL', fullChain=False, cleanupManagedFailure=str(error))
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    report['seconds'] = round(time.monotonic() - began, 3)
                    output.parent.mkdir(parents=True, exist_ok=True)
                    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
                    if cleanup_error is not None:
                        raise cleanup_error


if __name__ == '__main__':
    unittest.main()
