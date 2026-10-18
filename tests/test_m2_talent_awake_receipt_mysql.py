"""M2 prebuilt managed: legal003/materials -> unread talent response -> awake/cold.

Only Player70 and the actually acquired target003's level70 are finite input.
No cards, materials, coins, diamonds, talents or awakening stages are inserted.
Twenty original paid VIP7 boxes produce all gold through existing daily claims.
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

import test_growth_resource_loop_mysql as shared
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_p5_talent_scope_mysql import original_table
from hyjs_managed_mysql import ManagedMySQL
from server.codec import CLASSES, parse_http

ROOT = Path(__file__).resolve().parents[1]
SOURCES = {
    'item_337': ('limitExchange_p4_awakeelement_', 'awake-element-choice-single-player-v1', 500000, 100, 5, 150),
    'item_330': ('limitExchange_p4_awakematerial_', 'awake-material-choice-mainline-coin-exchange-v1', 1000000, 1, 1, 30),
}


def protocol_preflight():
    requests = {
        'CSGetPaymentInfo': {'channelId': 'AND0003', 'shopId': 'common'},
        'CSGenerateOrder': {'paymentId': 'local-p3-red-card', 'money': 30.0, 'moneyType': 'RMB', 'channelId': 'AND0003', 'shopId': 'common'},
        'CSItemUse': {'costItem': {'id': 'item_337', 'num': 100}, 'getItemIndex': 3},
        'CSShopBuy': {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1},
        'CSBuyActivityShop': {'activityId': 'actual-private-directory', 'index': 1, 'num': 5,
                              'costResources': [{'id': 'coin', 'num': 500000}], 'getResources': [{'id': 'item_337', 'num': 100}]},
        'CSWarriorAwake': {'guid': 'actual-target-guid'},
        'CSWarriorTalent': {'guid': 'actual-target-guid', 'talent': '1',
            'cost': [{'id': 'coin', 'num': 10000}, {'id': 'item_005', 'num': 30}]},
    }
    for name, fields in requests.items():
        raw = shared.wire.encode_original_http(name, fields, 2, b'0' * 16, 337001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original M2 request differs: ' + name)
    choices = {(r['itemId'], r['odds']): r['result'] for r in original_table('ItemUse')}
    expected = {('item_023', '5'): 'item_053,1', ('item_053', '2'): 'hero_003,1',
                ('item_330', '1'): 'item_312,2', ('item_330', '2'): 'item_315,8',
                ('item_337', '1'): 'item_320,1', ('item_337', '3'): 'item_322,1'}
    if any(choices.get(k) != v for k, v in expected.items()):
        raise AssertionError('Original card/material choice mapping differs')
    costs = [r for r in original_table('HeroAwakenCost') if r['heroId'] == 'hero_003' and r['stage'] == '1']
    if len(costs) != 1 or costs[0]['itemCost'] != 'item_322,800;item_315,50;item_312,15' or costs[0]['selfCostNum'] != '2':
        raise AssertionError('Original003 awakening recipe differs')
    for name, fields in {'SCWarriorTalent': {'guid', 'cost'}, 'SCWarriorAwake': {'newWarrior', 'cost'}}.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original M2 response shape differs: ' + name)
    talents = {(r['warId'], r['talentLevel']): r for r in original_table('TalentAdvenced')}
    if (talents['hero_003', '1']['cost'] != 'coin,10000;item_005,30' or
        talents['hero_003', '2']['cost'] != 'coin,20000;item_005,150' or
        talents['hero_003', '2']['parent'] != '1'):
        raise AssertionError('Actual003 original talent cost/prerequisite differs')
    vip6 = choices.get(('item_040', '100'))
    if vip6 != 'trea_011,1;trea_013,1;item_005,1000;item_048,4;item_002,10':
        raise AssertionError('Original VIP6 box medicine source differs')
    return {'originalRequests': len(requests), 'originalChoices': {a + '/' + b: v for (a, b), v in expected.items()},
            'actualConsumer': costs[0], 'finiteLevels': {'Player': 70, 'actual003Target': 70},
            'walletCents': 209000, 'paidVIP7Boxes': 20, 'actualCoinFromBoxes': 20000000,
            'source337Coin': 4500000, 'source330Coin': 15000000, 'talentMedicineSource': 'original040 ->005x1000, buy700 diamonds once at paidVIP6', 'actualTalentCoinCost': 30000, 'finalMedicine005': 820, 'finalDiamond': 7200, 'finalCoin': 470000}


def preflight():
    contract = protocol_preflight()
    # Reuse the existing candidate verifier without selecting an old G3 app.
    for suffix in ('SOURCE', 'BINARY', 'VALIDATION'):
        chosen = Path(os.environ['HYJS_M2_TALENT_' + suffix]).resolve()
        inherited = 'HYJS_GROWTH_RESOURCE_' + suffix
        if inherited in os.environ and Path(os.environ[inherited]).resolve() != chosen:
            raise AssertionError('Conflicting shared growth path: ' + inherited)
        os.environ[inherited] = str(chosen)
    source, binary, manifest_sha = shared.preflight()
    for table in ('Payment', 'Item', 'ItemUse', 'Hero', 'TalentAdvenced', 'HeroAwakenCost', 'HeroAwakenAdd', 'GameConfigure'):
        relative = 'analysis/tables/Table' + table + '.tsv'
        if shared.sha(source / relative) != shared.sha(ROOT / relative):
            raise AssertionError('Original G3 asset differs: ' + table)
    hooks = {
        'hyjs-server/service/p4CommerceService.go': ['seedP5AwakeElementSupply(tx, now)'],
        'hyjs-server/service/p4ResourceExchangeService.go': ['p5AppendAwakeElementBinding'],
        'hyjs-server/service/p5AwakeElementSupplyService.go': ['"item_337,100", "coin,500000"', 'product.DailyLimit, product.TotalLimit = 5, 150'],
        'hyjs-server/service/p5AwakeMaterialSupplyService.go': ['"item_330,1", "coin,1000000"', 'previous.Enabled'],
        'hyjs-server/service/warriorAwakeService.go': ['donors[:count]', 'cost["itemCost"]', 'Stage: proto.Int32(stage)'],
        'hyjs-server/service/localWalletService.go': ['Kind: "debit", DeltaCents: -order.Cents'],
    }
    contract['productionSHA256'] = {}
    for relative, needles in hooks.items():
        path = source / relative
        if any(needle not in path.read_text() for needle in needles):
            raise AssertionError('Missing actual G3 production integration: ' + relative)
        contract['productionSHA256'][relative] = shared.sha(path)
    expected = json.loads((ROOT / 'analysis/growth-first-20261016/m2-talent-receipt-r1-20261017/source-pins.json').read_text())
    for relative in ('hyjs-server/service/warriorService.go', 'hyjs-server/service/warriorGrowthService.go'):
        if shared.sha(source / relative) != expected['files'][relative]:
            raise AssertionError('M2 production code differs from the reviewed receipt delta: ' + relative)
    game = (source / 'hyjs-server/service/gameService.go').read_text()
    if 'return s.p5WarriorTalent(ctx, row.Id, b, r.Timestamp)' not in game:
        raise AssertionError('Actual original talent HTTP dispatch has no committed receipt')
    main = (source / 'hyjs-server/main.go').read_text()
    initializers = ['InitializeWarriorGrowth(ctx)', 'InitializeWarriorAwake(ctx)',
                    'InitializeShop(ctx)', 'InitializeP5(ctx)', 'InitializeLocalWallet(ctx)',
                    'InitializeLocalPlay(ctx)', 'InitializeLocalPlayActivities(ctx)', 'InitializeP4(ctx)']
    if any('s.' + name not in main for name in initializers) or 's.P5Ready = true' not in main:
        raise AssertionError('Actual main omits a complete growth/source dependency')
    contract['actualMainInitializers'] = initializers
    contract['qualificationSHA256'] = shared.sha(os.environ['HYJS_M2_TALENT_VALIDATION'])
    contract['M2ProductionSHA256'] = {relative: expected['files'][relative] for relative in
        ('hyjs-server/service/warriorService.go', 'hyjs-server/service/warriorGrowthService.go')}
    return source, binary, manifest_sha, contract


@unittest.skipUnless(os.environ.get('HYJS_M2_TALENT_MYSQL_TEST') == '1', 'Root managed prebuilt M2 SQL window')
class M2TalentAwakeReceiptMySQLTests(unittest.TestCase):
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

    def test_actual_sources_talent_lost_response_awake_next_node_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_M2_TALENT_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/m2-talent-receipt-r1-20261017'))
        self.assertFalse(output.exists(), 'Preserve old validation evidence')
        self.pool = ManagedMySQL()
        self.process = None
        report = dict(status='PENDING', fullChain=False, runnerSHA256=shared.sha(Path(__file__)),
                      sourceManifestSHA256=manifest_sha, binarySHA256=shared.sha(binary), contract=contract,
                      finitePreparation='Only Player70 and actually paid/acquired target003 level70; no historical star or resource/talent/phase injection',
                      productionModified=False, externalPayment=False, phoneUI=False, highestStageRevalidated=False, nativeCombatRevalidated=False)
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-growth-awake-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('m2_talent')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    for table, columns in {
                        'p4_commerce_rules': {'id', 'enabled', 'rules', 'source', 'frozen'},
                        'p4_resource_exchange_directories': {'id', 'input_hash', 'bindings'},
                        'p3_vip_shop_products': {'id', 'min_v_ip', 'cost', 'reward', 'source', 'enabled'},
                        'local_wallet_entries': {'player_id', 'kind', 'delta_cents'},
                        'p3_payment_orders': {'player_id', 'status', 'cents', 'credited'},
                        'first_double_policies': {'id', 'enabled'},
                        'warrior_growth_receipts': {'player_id', 'guid', 'kind', 'timestamp', 'request', 'response'},
                        'warrior_awake_rules': {'id', 'enabled', 'coin_cost'},
                    }.items():
                        actual = self.sql('SELECT COLUMN_NAME FROM information_schema.COLUMNS '
                                          f"WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='{table}';").split()
                        self.assertTrue(columns <= set(actual), 'G3 actual SQL columns changed: ' + table)
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    self.assertEqual(self.sql('SELECT enabled FROM first_double_policies WHERE id=1;').strip(), '0')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-m2-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '觉醒供应保护', 'heroId': 'hero_001'}, protected, 337000, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-m2-talent-' + suffix
                    owner = self.login(uid)
                    stamp = 337001

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        saved = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                            replies = list(pool.map(lambda _: self.call(name, fields, owner, saved, expected), range(8)))
                        self.assertTrue(all(reply == replies[0] for reply in replies))
                        return replies[0], saved

                    def reject(name, fields):
                        before = self.profile(owner)
                        http(name, fields, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), before)

                    def count(item):
                        return sum(row['num'] for row in (self.profile(owner)[1].get('items') or []) if row['id'] == item)

                    def choose(box, number, index, target, amount):
                        before = count(target)
                        result, saved = once('CSItemUse', {'costItem': {'id': box, 'num': number}, 'getItemIndex': index}, 'SCItemUse')
                        gained = getattr(result, 'get')
                        self.assertTrue(all(item.id == target for item in gained.simps))
                        self.assertEqual(sum(item.num for item in gained.simps), amount)
                        self.assertEqual(count(target), before + amount)
                        return result, saved

                    http('CSPlayerCreate', {'playerName': '原材料实养成', 'heroId': 'hero_001'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    main_guid = initial[0]['heroWarId']
                    self.assertEqual(tuple(initial[0].get(key, 0) for key in ('coin', 'diamond')), (0, 0))
                    credit_key = str(uuid.uuid4())
                    self.wallet('add', owner, requestKey=credit_key, amountCents=209000)
                    catalogue = http('CSGetPaymentInfo', {'channelId': 'AND0003', 'shopId': 'common'}, 'SCGetPaymentInfo')
                    prices = {row.paymentId: row for row in catalogue.paymentInfoList}
                    orders, acquired = [], []
                    for sku in ['djsy.wuyou.andp2000', 'local-p3-red-card', 'local-p3-red-card', 'local-p3-red-card']:
                        product = prices[sku]
                        self.assertEqual(product.money, 2000.0 if sku.endswith('2000') else 30.0)
                        request = {'paymentId': sku, 'money': product.money, 'moneyType': product.moneyType,
                                   'channelId': 'AND0003', 'shopId': product.shopId}
                        generated, _ = once('CSGenerateOrder', request, 'SCGenerateOrder')
                        self.assertEqual(generated.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                        orders.append(generated.orderId)
                        if sku == 'local-p3-red-card':
                            choose('item_023', 1, 5, 'item_053', 1)
                            selected, _ = once('CSItemUse', {'costItem': {'id': 'item_053', 'num': 1}, 'getItemIndex': 2}, 'SCItemUse')
                            warriors = getattr(selected, 'get').wars
                            self.assertEqual(len(warriors), 1)
                            self.assertEqual((warriors[0].id, warriors[0].level), ('hero_003', 1))
                            acquired.append(warriors[0].guid)
                    self.assertEqual(len(set(acquired)), 3)
                    target = acquired[0]
                    paid = self.profile(owner)
                    self.assertEqual((paid[0]['vip'], paid[0]['chargeDiamond'], paid[0]['diamond']), (7, 20900, 23900))
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    self.assertEqual(self.sql(f"SELECT COUNT(*),SUM(delta_cents) FROM local_wallet_entries WHERE player_id={owner.playerId} AND kind='debit';").strip(), '4\t-209000')
                    # Open the old private directory before either new source is
                    # published. Queries must preserve its exact bytes forever.
                    self.activity(owner, 'open')
                    old_key, old_hash, old_raw, old_bindings = self.directory(owner)
                    self.assertFalse(any(row.get('productId') in SOURCES for row in old_bindings))
                    source_records = {}
                    for item, (prefix, policy, coin, amount, daily, total) in SOURCES.items():
                        rows = self.sql("SELECT id,enabled,HEX(rules),source FROM p4_commerce_rules WHERE id LIKE '" + prefix +
                                        "%' AND LENGTH(id)=" + str(len(prefix) + 6) + ' ORDER BY start_at DESC LIMIT 1;').strip().split('\t')
                        self.assertEqual(len(rows), 4)
                        self.assertEqual(rows[1], '0')
                        self.assertTrue(rows[3].startswith(policy + '：'))
                        product = json.loads(bytes.fromhex(rows[2]))['products'][0]
                        self.assertEqual((product['id'], product['rewards'], product['cost'], product['minLevel'],
                                          product['dailyLimit'], product['totalLimit']),
                                         (item, item + ',' + str(amount), 'coin,' + str(coin), 70, daily, total))
                        source_records[item] = {'id': rows[0], 'source': rows[3], 'product': product}
                        self.pool.sql(self.database, "UPDATE p4_commerce_rules SET enabled=1 WHERE id='" + rows[0] + "' AND enabled=0;")
                    before_new = http('CSActivityShopInfo', {'activityId': old_key}, 'SCActivityShopInfo')
                    self.assertFalse(any(v.id in SOURCES for row in before_new.shopItemList for v in row.getResource))
                    self.assertEqual(self.sql("SELECT input_hash,HEX(bindings) FROM p4_resource_exchange_directories WHERE id='" + old_key + "';").strip(), old_hash + '\t' + old_raw)
                    self.activity(owner, 'refresh')
                    directory_key, _, _, bindings = self.directory(owner)
                    self.assertNotEqual(directory_key, old_key)
                    addresses = {row['productId']: row for row in bindings if row.get('productId') in SOURCES}
                    self.assertEqual(set(addresses), set(SOURCES))

                    def quote(item):
                        listing = http('CSActivityShopInfo', {'activityId': directory_key}, 'SCActivityShopInfo')
                        selected = [row for row in listing.shopItemList if row.index == addresses[item]['index']]
                        self.assertEqual(len(selected), 1)
                        return selected[0]

                    def buy(item, number):
                        row = quote(item)
                        _, _, coin, amount, _, _ = SOURCES[item]
                        self.assertEqual([(r.id, r.num) for r in row.costResource], [('coin', coin)])
                        self.assertEqual([(r.id, r.num) for r in row.getResource], [(item, amount)])
                        request = {'activityId': directory_key, 'index': row.index, 'num': number,
                                   'costResources': [{'id': r.id, 'num': r.num} for r in row.costResource],
                                   'getResources': [{'id': r.id, 'num': r.num} for r in row.getResource]}
                        before_coin = self.profile(owner)[0].get('coin', 0)
                        before_items = count(item)
                        response, saved = once('CSBuyActivityShop', request, 'SCGetActivityShopReward')
                        self.assertEqual(self.profile(owner)[0]['coin'], before_coin - coin * number)
                        self.assertEqual(count(item), before_items + amount * number)
                        return request, response, saved

                    # Level1 is denied before any explicit level qualification.
                    low = quote('item_337')
                    reject('CSBuyActivityShop', {'activityId': directory_key, 'index': low.index, 'num': 1,
                        'costResources': [{'id': r.id, 'num': r.num} for r in low.costResource],
                        'getResources': [{'id': r.id, 'num': r.num} for r in low.getResource]})
                    # Only levels change; GUIDs, inventories and all other fields
                    # retain their actual original acquisition state.
                    player, game = self.profile(owner)
                    index = next(i for i, row in enumerate(game['warriors']) if row['guid'] == target)
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',70), "
                        f"game=JSON_SET(game,'$.warriors[{index}].level',70) WHERE id={owner.playerId} AND uid LIKE 'demo-m2-talent-%';")
                    qualified = self.profile(owner)
                    self.assertEqual({k: v for k, v in qualified[0].items() if k != 'level'}, {k: v for k, v in player.items() if k != 'level'})
                    expected_game = json.loads(json.dumps(game))
                    expected_game['warriors'][index]['level'] = 70
                    self.assertEqual(qualified[1], expected_game)
                    # Enable only the already-validated original VIP7 source in
                    # this owned schema; no frozen quote or prior claim is reset.
                    vip6 = self.sql("SELECT min_v_ip,cost,reward,enabled FROM p3_vip_shop_products WHERE id='p3vip6_growth_local';").strip()
                    self.assertEqual(vip6, '6\t700\titem_040,1\t0')
                    self.pool.sql(self.database, "UPDATE p3_vip_shop_products SET enabled=1 WHERE id='p3vip6_growth_local' AND min_v_ip=6 AND cost=700 AND reward='item_040,1' AND enabled=0;")
                    http('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip6_growth_local', 'buyCount': 1}, 'SCShopBuy')
                    medicine_box, medicine_box_stamp = once('CSItemUse', {'costItem': {'id': 'item_040', 'num': 1}}, 'SCItemUse')
                    self.assertEqual(count('item_005'), 1000)
                    self.assertEqual(sorted(v.treasureId for v in getattr(medicine_box, 'get').treas), ['trea_011', 'trea_013'])
                    vip = self.sql("SELECT min_v_ip,cost,reward,enabled FROM p3_vip_shop_products WHERE id='p3vip7_growth_local';").strip()
                    self.assertEqual(vip, '7\t800\titem_041,1\t0')
                    self.pool.sql(self.database, "UPDATE p3_vip_shop_products SET enabled=1 WHERE id='p3vip7_growth_local' AND min_v_ip=7 AND cost=800 AND reward='item_041,1' AND enabled=0;")
                    for day in range(20):
                        if day:
                            self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], day)
                        http('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1}, 'SCShopBuy')
                        opened, _ = once('CSItemUse', {'costItem': {'id': 'item_041', 'num': 1}}, 'SCItemUse')
                        self.assertEqual(sum(item.num for item in getattr(opened, 'get').simps if item.id == 'coin'), 1000000)
                    self.assertEqual((self.profile(owner)[0]['coin'], self.profile(owner)[0]['diamond']), (20000000, 7200))
                    self.assertEqual(self.change(owner, 'reset')['offsetDays'], 0)
                    reject('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1})
                    purchases, selections = [], []
                    for day in range(15):
                        if day:
                            self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], day)
                        if day < 2:
                            purchases.append(buy('item_337', 5 if day == 0 else 4))
                        purchases.append(buy('item_330', 1))
                        option, target_item, amount = (1, 'item_312', 2) if day < 8 else (2, 'item_315', 8)
                        result, saved = choose('item_330', 1, option, target_item, amount)
                        selections.append(({'costItem': {'id': 'item_330', 'num': 1}, 'getItemIndex': option}, result, saved))
                    self.assertEqual(self.change(owner, 'reset')['offsetDays'], 0)
                    self.assertEqual((count('item_337'), count('item_330'), count('item_312'), count('item_315')), (900, 0, 16, 56))
                    for i in range(9):
                        option, target_item = (3, 'item_322') if i < 8 else (1, 'item_320')
                        result, saved = choose('item_337', 100, option, target_item, 100)
                        selections.append(({'costItem': {'id': 'item_337', 'num': 100}, 'getItemIndex': option}, result, saved))
                    self.assertEqual((count('item_322'), count('item_320')), (800, 100))
                    talent1 = {'guid': target, 'talent': '1', 'cost': [
                        {'id': 'coin', 'num': 10000}, {'id': 'item_005', 'num': 30}]}
                    before_talent = self.profile(owner)
                    stamp += 1
                    talent1_stamp = stamp
                    raw = shared.wire.encode_original_http('CSWarriorTalent', talent1,
                        owner.playerId, bytes(owner.sessionKey), talent1_stamp)
                    abandoned = http_client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=20)
                    try:
                        abandoned.request('POST', '/sao/', body=raw,
                            headers={'Content-Type': 'application/octet-stream'})
                        # Never call getresponse/read: the application commits
                        # while this caller deliberately leaves its response unread.
                        deadline = time.monotonic() + 10
                        receipt_hex = ''
                        while time.monotonic() < deadline:
                            receipt_hex = self.sql(f"SELECT HEX(response) FROM warrior_growth_receipts WHERE player_id={owner.playerId} AND guid='{target}' AND kind='talent' AND timestamp={talent1_stamp};").strip()
                            if receipt_hex:
                                break
                            time.sleep(.05)
                        self.assertTrue(receipt_hex, 'First unread original talent request did not commit')
                    finally:
                        abandoned.close()
                    committed = CLASSES['SCWarriorTalent']()
                    committed.ParseFromString(bytes.fromhex(receipt_hex))
                    self.assertEqual((committed.guid.guid, committed.guid.id, committed.guid.talent), (target, 'hero_003', 1))
                    talent_state = self.profile(owner)
                    self.assertEqual(talent_state[0]['coin'], before_talent[0]['coin'] - 10000)
                    self.assertEqual(count('item_005'), 970)
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                        replays = list(pool.map(lambda _: self.call('CSWarriorTalent', talent1, owner,
                            talent1_stamp, 'SCWarriorTalent'), range(8)))
                    self.assertTrue(all(v == committed for v in replays))
                    self.assertEqual(self.profile(owner), talent_state)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), talent_state)
                    self.assertEqual(self.call('CSWarriorTalent', talent1, owner, talent1_stamp, 'SCWarriorTalent'), committed)
                    conflict = dict(talent1, talent='2')
                    self.call('CSWarriorTalent', conflict, owner, talent1_stamp, 'ErrorMessage')
                    self.assertEqual(self.profile(owner), talent_state)
                    awake, awake_stamp = once('CSWarriorAwake', {'guid': target}, 'SCWarriorAwake')
                    self.assertEqual((awake.newWarrior.guid, awake.newWarrior.id, awake.newWarrior.awake.stage), (target, 'hero_003', 1))
                    self.assertEqual((count('item_322'), count('item_320'), count('item_312'), count('item_315')), (0, 100, 1, 6))
                    final = self.profile(owner)
                    self.assertEqual((final[0]['coin'], final[0]['diamond'], final[0]['heroWarId']), (490000, 7200, main_guid))
                    self.assertEqual((quote('item_337').total, quote('item_337').left), (5, 0))
                    self.assertEqual((quote('item_330').total, quote('item_330').left), (1, 0))
                    old_day_request = dict(purchases[0][0], num=1)
                    reject('CSBuyActivityShop', old_day_request)
                    actual_guids = {row['guid'] for row in final[1]['warriors']}
                    self.assertIn(target, actual_guids)
                    self.assertFalse(set(acquired[1:]) & actual_guids)
                    self.assertEqual(self.sql("SELECT input_hash,HEX(bindings) FROM p4_resource_exchange_directories WHERE id='" + old_key + "';").strip(), old_hash + '\t' + old_raw)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.tools(owner, 'time/state')['offsetDays'], 0)
                    request, response, saved = purchases[0]
                    self.assertEqual(self.call('CSBuyActivityShop', request, owner, saved, 'SCGetActivityShopReward'), response)
                    request, response, saved = selections[-1]
                    self.assertEqual(self.call('CSItemUse', request, owner, saved, 'SCItemUse'), response)
                    self.assertEqual(self.call('CSWarriorAwake', {'guid': target}, owner, awake_stamp, 'SCWarriorAwake'), awake)
                    self.assertEqual(self.profile(owner), final)
                    self.call('CSWarriorTalent', talent1, owner, talent1_stamp, 'ErrorMessage')
                    self.assertEqual(self.profile(owner), final)
                    talent2 = {'guid': target, 'talent': '2', 'cost': [
                        {'id': 'coin', 'num': 20000}, {'id': 'item_005', 'num': 150}]}
                    second, talent2_stamp = once('CSWarriorTalent', talent2, 'SCWarriorTalent')
                    self.assertEqual((second.guid.talent, second.guid.awake.stage), (2, 1))
                    final = self.profile(owner)
                    self.assertEqual((final[0]['coin'], final[0]['diamond'], count('item_005')), (470000, 7200, 820))
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.call('CSWarriorTalent', talent2, owner, talent2_stamp, 'SCWarriorTalent'), second)
                    self.call('CSWarriorTalent', talent1, owner, talent1_stamp, 'ErrorMessage')
                    # Talent2 changes the complete Warrior carried by old Awake1.
                    self.call('CSWarriorAwake', {'guid': target}, owner, awake_stamp, 'ErrorMessage')
                    self.assertEqual(self.profile(owner), final)
                    actual = next(w for w in final[1]['warriors'] if w['guid'] == target)
                    self.assertEqual((actual['talent'], actual['awake']['stage']), (2, 1))
                    self.assertEqual(self.profile(protected), protected_before)
                    report.update(status='PASS', fullChain=True, original337Consumed=900, original330Consumed=15,
                        actualElement322Consumed=800, source337={'coinSpent': 4500000, 'originalBoxesBought': 900,
                        'actualSelected322': 800, 'actualSelected320': 100, 'actualConsumed322': 800, 'remaining320': 100},
                        source330={'coinSpent': 15000000, 'originalBoxesBought': 15, 'actualSelected312': 16,
                                   'actualSelected315': 56, 'actualConsumed312': 15, 'actualConsumed315': 50,
                                   'remaining312': 1, 'remaining315': 6},
                        actualWalletCents=209000, actualOrderIDs=orders, actualPaidCards=acquired,
                        actualVIP7Boxes=20, actualCoinFromOriginalBoxes=20000000, finalCoin=470000, finalDiamond=7200, actualMedicine005Bought=1000, actualMedicine005Consumed=180, actualMedicine005Remaining=820,
                        actualAwakeTarget=target, actualAwakeStage=1, actualIdleSameCardsConsumed=2,
                        currentSources=source_records, privateDirectory=directory_key, oldDirectoryImmutable=True,
                        datesRestoredTo0=True, sourceDailyAndPeriodLimitsUnchanged=True, eightSameStampOnce=True,
                        coldOriginalPurchaseChoiceAwakeReceipts=True, protected1Exact=True,
                        talentFirstUnreadResponseCommitted=True,
                        unreadResponseBoundary='HTTP request sent; receipt observed committed in DB; response body never read before caller close',
                        talentEightRetriesOnce=True, talentColdReplay=True, talentConflictRejected=True,
                        laterAwakeningRetiresTalent1=True, nextTalentAfterAwake=2,
                        laterTalentRetiresAwakeFrame=True, actualVIP6MedicineBox=1,
                        talent1Stamp=talent1_stamp, talent2Stamp=talent2_stamp,
                        coldRestarts=3, naturalLevelUnlockClaimed=False,
                        highestStageHistorical='analysis/p5/awake-material-supply-mysql-validation.json finite stage2/fusion proof reused; not rerun or natural full-stage claim')
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
