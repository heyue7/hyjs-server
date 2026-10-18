"""R4 prebuilt managed: real003/resources -> train74 -> level75/List -> train/reset/cold.

Only Player75 and actually acquired003 level74/near-threshold EXP are finite.
Wallet/orders, card choice, gold, training medicine and feed GUIDs are real
original business calls. No SDK/compiler, source mutation, phone or live DB.
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

import test_growth_resource_loop_mysql as shared
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_p5_talent_scope_mysql import original_table
from hyjs_managed_mysql import ManagedMySQL
from server.codec import CLASSES, parse_http

ROOT = Path(__file__).resolve().parents[1]
DELIVERY = ROOT / 'analysis/growth-first-20261016/m2-specialty-continuity-r1-20261017'
DEFAULT_SOURCE = ROOT / 'build/candidates/growth-gm-mainline-r4-20261017'
DEFAULT_BINARY = ROOT / 'build/hyjs-server-growth-gm-mainline-r4-20261017-pending'
DEFAULT_QUALIFICATION = ROOT / 'analysis/p3/growth-gm-mainline-r4-incremental-20261017.json'


def protocol_preflight():
    vectors = {
        'CSWarriorList': {}, 'CSWarriorGet': {'guid': ['actual003']},
        'CSWarriorTraining': {'guid': 'actual003', 'costType': 3, 'numType': 1},
        'CSWarriorTrainingReplace': {'guid': 'actual003', 'result': 1},
        'CSWarriorStrengthen': {'guid': 'actual003', 'cost': [{'id': 'hero_exp_001', 'guid': 'actual-feed'}]},
        'CSWarriorReset': {'guid': 'actual003'},
        'CSShopProductList': {'shopType': 1},
        'CSShopBuy': {'shopType': 1, 'shopItemId': 'itemShopId_9', 'buyCount': 2},
        'CSItemUse': {'costItem': {'id': 'item_023', 'num': 1}, 'getItemIndex': 5},
    }
    for name, fields in vectors.items():
        raw = shared.wire.encode_original_http(name, fields, 2, b'0' * 16, 338001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original M2 continuity frame differs: ' + name)
    expected_fields = {'SCWarriorList': {'warriorList'}, 'SCWarriorGet': {'warrior'},
        'SCWarriorTraining': {'guid', 'cost'}, 'SCWarriorTrainingReplace': {'result', 'cost'},
        'SCWarriorReset': {'warriors', 'cost', 'get'}, 'SCShopProductList': {'shopItemList'}}
    for name, fields in expected_fields.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Recovered response differs: ' + name)
    configure = {r['StaticName']: r['NameValue'] for r in original_table('GameConfigure')}
    expected = {'skillSpecialityLevelLimit': '75', 'WarriorTraining3': 'item_007,3;diamond,5',
                'WarriorRebuildCost': 'diamond,50', 'HeroStrengthCostExpToCoin': '1'}
    if any(configure.get(k) != v for k, v in expected.items()):
        raise AssertionError('Original gate/cost changed')
    choices = {(r['itemId'], r['odds']): r['result'] for r in original_table('ItemUse')}
    if choices.get(('item_023', '5')) != 'item_053,1' or choices.get(('item_053', '2')) != 'hero_003,1':
        raise AssertionError('Original paid003 choice changed')
    if choices.get(('item_041', '100')) != 'item_050,2;item_009,50;item_012,50;item_001,10;coin,1000000':
        raise AssertionError('Original VIP7 full reward changed')
    threshold = int(next(r['exp'] for r in original_table('HeroExp') if r['quality'] == '23' and r['level'] == '74'))
    food_exp = int(next(r['expBase'] for r in original_table('Hero') if r['id'] == 'hero_exp_001'))
    if threshold != 218717 or food_exp != 400:
        raise AssertionError('Original cross75 experience boundary changed')
    return {'requestVectors': len(vectors), 'responseContracts': len(expected_fields), 'finitePlayerLevel': 75,
        'finiteActualTargetLevel': 74, 'finiteActualTargetExp': threshold - 1, 'actualOriginalFeedExperience': food_exp,
        'actualCross75Exp': food_exp - 1, 'walletCents': 203000, 'trainingBundles': 2,
        'training007Bought': 10, 'training007Spent': 6, 'originalResetDiamondFee': 50,
        'highestSpecialtyReused': 'analysis/p5/specialty-highest-lifecycle-validation.json',
        'natural16SourceReused': 'analysis/p5/specialty-natural-mysql-validation.json',
        'scope': 'Cross-gate preparation and growth/refund; specialty level2/highest100 are not rerun here'}


def preflight():
    contract = protocol_preflight()
    chosen = {'SOURCE': Path(os.environ.get('HYJS_M2_SPECIALTY_SOURCE', DEFAULT_SOURCE)).resolve(),
              'BINARY': Path(os.environ.get('HYJS_M2_SPECIALTY_BINARY', DEFAULT_BINARY)).resolve(),
              'VALIDATION': Path(os.environ.get('HYJS_M2_SPECIALTY_VALIDATION', DEFAULT_QUALIFICATION)).resolve()}
    for suffix, value in chosen.items():
        key = 'HYJS_GROWTH_RESOURCE_' + suffix
        if key in os.environ and Path(os.environ[key]).resolve() != value:
            raise AssertionError('Conflicting fixed source path: ' + key)
        os.environ[key] = str(value)
    source, binary, manifest_sha = shared.preflight()
    pins = json.loads((DELIVERY / 'source-pins.json').read_text())
    expected = {r['path']: r['sha256'] for r in pins['owned']}
    expected.update({r['path']: r['afterSHA256'] for r in pins['sharedProposal']})
    for relative, digest in expected.items():
        if shared.sha(source / relative) != digest:
            raise AssertionError('Fixed R4 continuity delta differs: ' + relative)
    for relative, digest in pins['requiredAssets'].items():
        if shared.sha(source / relative) != digest:
            raise AssertionError('Original continuity table differs: ' + relative)
    main = (source / 'hyjs-server/main.go').read_text()
    required = ['InitializeP5(ctx)', 'InitializeWarriorGrowth(ctx)', 'InitializeWarriorAwake(ctx)',
        'InitSkillSpecialty(ctx)', 'InitializeShop(ctx)', 'InitializeP4(ctx)', 'InitializeLocalWallet(ctx)',
        'InitializeLocalPlay(ctx)', 'InitializeLocalPlayActivities(ctx)']
    if any('s.' + item not in main for item in required):
        raise AssertionError('Actual main missing full growth/source initialization')
    game = (source / 'hyjs-server/service/gameService.go').read_text()
    if game.count('s.skillSpecialtyPrepare(ctx, row.Id)') < 2 or 's.p5WarriorStrengthen(ctx, id, b, stamp)' not in (source / 'hyjs-server/service/p5FormationNextService.go').read_text():
        raise AssertionError('Original List/Get/strengthen dispatch missing')
    contract.update(productionSHA256=expected, actualMainInitializers=required,
        qualificationSHA256=shared.sha(chosen['VALIDATION']))
    return source, binary, manifest_sha, contract


@unittest.skipUnless(os.environ.get('HYJS_M2_SPECIALTY_MYSQL_TEST') == '1', 'Root managed prebuilt M2 SQL window')
class M2SpecialtyContinuityMySQLTests(unittest.TestCase):
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

    def test_actual_paid_sources_cross75_list_train_reset_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ.get('HYJS_M2_SPECIALTY_OUTPUT', DELIVERY / 'http-mysql-r1.json')).resolve()
        self.assertTrue(output.is_relative_to(DELIVERY))
        self.assertFalse(output.exists(), 'Preserve prior execution evidence')
        self.pool = ManagedMySQL()
        self.process = None
        report = dict(status='PENDING', fullChain=False, runnerSHA256=shared.sha(Path(__file__)),
            binarySHA256=shared.sha(binary), sourceManifestSHA256=manifest_sha, contract=contract,
            finitePreparation='Only Player75 and actually acquired003 level74/EXP218716; no currency, material, GUID, talent, training, skill or receipt injection',
            productionModified=False, externalPayment=False, phoneUI=False, nativeCombatRevalidated=False, highestStageRevalidated=False)
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-m2-specialty-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('m2_specialty')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    for table, columns in {
                        'profiles': {'id', 'uid', 'player', 'game'},
                        'local_wallet_policies': {'id', 'enabled'},
                        'local_wallet_entries': {'player_id', 'kind', 'delta_cents'},
                        'p3_payment_orders': {'player_id', 'status', 'cents', 'credited'},
                        'p3_vip_shop_products': {'id', 'enabled', 'min_v_ip', 'cost', 'reward'},
                        'warrior_growth_receipts': {'player_id', 'guid', 'kind', 'timestamp', 'request', 'response'},
                        'warrior_growth_rules': {'id', 'enabled', 'distribution'},
                        'skill_specialty_rules': {'id', 'enabled', 'initial_types'},
                        'shop_products': {'id', 'resource_id', 'amount', 'cost', 'daily_limit'},
                        'shop_receipts': {'player_id', 'timestamp'},
                        'p5_mutation_receipts': {'player_id', 'operation', 'timestamp', 'request', 'response'},
                    }.items():
                        columns_now = set(self.sql('SELECT COLUMN_NAME FROM information_schema.COLUMNS '
                            f"WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='{table}';").split())
                        self.assertTrue(columns <= columns_now, 'Actual schema drift: ' + table)
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-m2-specialty-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '连续培养保护', 'heroId': 'hero_001'}, protected, 338000, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-m2-specialty-' + suffix
                    owner = self.login(uid)
                    stamp = 338001
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
                    def count(item):
                        return sum(v['num'] for v in (self.profile(owner)[1].get('items') or []) if v['id'] == item)
                    def target_state():
                        return next(w for w in (self.profile(owner)[1].get('warriors') or []) if w['guid'] == target)
                    def ledger():
                        values = [v['value'] for v in (self.profile(owner)[1].get('kv') or []) if v['key'] == 'p5.invest:' + target]
                        return json.loads(values[0]) if values and values[0] else None
                    http('CSPlayerCreate', {'playerName': '真实连续培养', 'heroId': 'hero_001'}, 'SCPlayerCreate')
                    self.assertEqual((self.profile(owner)[0].get('coin', 0), self.profile(owner)[0].get('diamond', 0)), (0, 0))
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=203000)
                    listing = http('CSGetPaymentInfo', {'channelId': 'AND0003', 'shopId': 'common'}, 'SCGetPaymentInfo')
                    products = {v.paymentId: v for v in listing.paymentInfoList}
                    orders = []
                    for sku in ['djsy.wuyou.andp2000', 'local-p3-red-card']:
                        v = products[sku]
                        generated, _ = once('CSGenerateOrder', {'paymentId': sku, 'money': v.money,
                            'moneyType': v.moneyType, 'channelId': 'AND0003', 'shopId': v.shopId}, 'SCGenerateOrder')
                        self.assertEqual(generated.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                        orders.append(generated.orderId)
                    self.assertGreaterEqual(self.profile(owner)[0]['vip'], 7)
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    self.assertEqual(self.sql(f"SELECT COUNT(*),SUM(delta_cents) FROM local_wallet_entries WHERE player_id={owner.playerId} AND kind='debit';").strip(), '2\t-203000')
                    selected, _ = once('CSItemUse', {'costItem': {'id': 'item_023', 'num': 1}, 'getItemIndex': 5}, 'SCItemUse')
                    self.assertEqual([(v.id, v.num) for v in getattr(selected, 'get').simps], [('item_053', 1)])
                    card, _ = once('CSItemUse', {'costItem': {'id': 'item_053', 'num': 1}, 'getItemIndex': 2}, 'SCItemUse')
                    self.assertEqual(len(getattr(card, 'get').wars), 1)
                    actual = getattr(card, 'get').wars[0]
                    self.assertEqual((actual.id, actual.level), ('hero_003', 1))
                    target = actual.guid
                    self.assertEqual(self.sql("SELECT min_v_ip,cost,reward,enabled FROM p3_vip_shop_products WHERE id='p3vip7_growth_local';").strip(), '7\t800\titem_041,1\t0')
                    self.pool.sql(self.database, "UPDATE p3_vip_shop_products SET enabled=1 WHERE id='p3vip7_growth_local' AND min_v_ip=7 AND cost=800 AND reward='item_041,1' AND enabled=0;")
                    http('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1}, 'SCShopBuy')
                    box, _ = once('CSItemUse', {'costItem': {'id': 'item_041', 'num': 1}}, 'SCItemUse')
                    self.assertEqual(sum(v.num for v in getattr(box, 'get').simps if v.id == 'coin'), 1000000)
                    shop = http('CSShopProductList', {'shopType': 1}, 'SCShopProductList')
                    medicine = next(v for v in shop.shopItemList if v.id == 'itemShopId_9')
                    self.assertEqual([(v.id, v.num) for v in medicine.item], [('item_007', 5)])
                    self.assertEqual([(v.id, v.num) for v in medicine.price], [('diamond', 30)])
                    once('CSShopBuy', {'shopType': 1, 'shopItemId': 'itemShopId_9', 'buyCount': 2}, 'SCShopBuy')
                    self.assertEqual(count('item_007'), 10)
                    feed, _ = once('CSShopBuy', {'shopType': 1, 'shopItemId': 'itemShopId_5', 'buyCount': 1}, 'SCShopBuy')
                    self.assertEqual(len(getattr(feed, 'get').wars), 1)
                    food = getattr(feed, 'get').wars[0]
                    self.assertEqual((food.id, food.level), ('hero_exp_001', 1))
                    before_p, before_g = self.profile(owner)
                    index = next(i for i, w in enumerate(before_g['warriors']) if w['guid'] == target)
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',75), "
                        f"game=JSON_SET(game,'$.warriors[{index}].level',74,'$.warriors[{index}].exp',218716) "
                        f"WHERE id={owner.playerId} AND uid LIKE 'demo-m2-specialty-%';")
                    prepared_p, prepared_g = self.profile(owner)
                    self.assertEqual({k: v for k, v in prepared_p.items() if k != 'level'}, {k: v for k, v in before_p.items() if k != 'level'})
                    expected_g = json.loads(json.dumps(before_g))
                    expected_g['warriors'][index].update(level=74, exp=218716)
                    self.assertEqual(prepared_g, expected_g)
                    baseline = target_state()
                    baseline_p = prepared_p
                    self.assertFalse(baseline.get('skillRefine'))
                    self.assertIsNone(ledger())
                    records = []
                    def train():
                        preview, ps = once('CSWarriorTraining', {'guid': target, 'costType': 3, 'numType': 1}, 'SCWarriorTraining')
                        self.assertEqual(preview.guid.tempTraining.state, 1)
                        saved, ss = once('CSWarriorTrainingReplace', {'guid': target, 'result': 1}, 'SCWarriorTrainingReplace')
                        self.assertEqual(saved.result.tempTraining.state, 0)
                        self.assertEqual([(v.id, v.num) for v in preview.cost], [('item_007', 3), ('diamond', 5)])
                        records.append({'previewStamp': ps, 'confirmStamp': ss,
                            'actualTraining': {k: getattr(saved.result.training, k) for k in ('att', 'hp', 'pdef', 'mdef')}})
                        return saved, ss
                    train()
                    trained_ledger = ledger()
                    self.assertIsNotNone(trained_ledger)
                    before_level_coin = self.profile(owner)[0]['coin']
                    upgraded, level_stamp = once('CSWarriorStrengthen', {'guid': target,
                        'cost': [{'id': food.id, 'guid': food.guid}]}, 'SCWarriorStrengthen')
                    self.assertEqual((upgraded.guid.level, upgraded.guid.exp), (75, 399))
                    self.assertEqual(self.profile(owner)[0]['coin'], before_level_coin - 400)
                    before_list = ledger()
                    full = http('CSWarriorList', {}, 'SCWarriorList')
                    projected = next(w for w in full.warriorList if w.guid == target)
                    self.assertEqual([(v.type, v.level, v.strengthLevel) for v in projected.skillRefine], [(1, 1, 1), (2, 1, 1), (3, 1, 1)])
                    after_list = ledger()
                    self.assertNotEqual(after_list['expected'], before_list['expected'])
                    for field in ('baseline', 'costs', 'source', 'type'):
                        self.assertEqual(after_list[field], before_list[field], 'Free List defaults altered refundable investment')
                    self.assertEqual(count('item_007'), 7)
                    retained = self.profile(owner)
                    http('CSWarriorList', {}, 'SCWarriorList')
                    self.assertEqual(self.profile(owner), retained)
                    self.stop(); self.start(); owner = self.login(uid)
                    self.assertEqual(self.profile(owner), retained)
                    http('CSWarriorList', {}, 'SCWarriorList')
                    self.assertEqual(ledger(), after_list)
                    train()
                    self.assertEqual(count('item_007'), 4)
                    final_before_reset = self.profile(owner)
                    reset, reset_stamp = once('CSWarriorReset', {'guid': target}, 'SCWarriorReset')
                    self.assertEqual([(v.id, v.num) for v in reset.cost], [('diamond', 50)])
                    self.assertEqual(count('item_007'), 10)
                    restored = self.profile(owner)
                    self.assertEqual(target_state(), baseline)
                    self.assertEqual((restored[0]['coin'], restored[0]['diamond']), (baseline_p['coin'], baseline_p['diamond'] - 50))
                    self.assertIsNone(ledger())
                    returned = [w for w in restored[1]['warriors'] if w['id'] == 'hero_exp_001']
                    self.assertEqual(len(returned), 1)
                    self.assertNotEqual(returned[0]['guid'], food.guid)
                    self.assertEqual(getattr(reset, 'get').wars[0].guid, returned[0]['guid'])
                    self.stop(); self.start(); owner = self.login(uid)
                    self.assertEqual(self.profile(owner), restored)
                    self.assertEqual(self.call('CSWarriorReset', {'guid': target}, owner, reset_stamp, 'SCWarriorReset'), reset)
                    http('CSWarriorList', {}, 'SCWarriorList')
                    self.assertEqual(target_state(), baseline)
                    self.assertIsNone(ledger())
                    self.assertEqual(self.profile(owner), restored)
                    for bad_guid in ['not-owned', restored[0]['heroWarId']]:
                        http('CSWarriorReset', {'guid': bad_guid}, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), restored)
                    self.call('CSWarriorTraining', {'guid': target, 'costType': 3, 'numType': 1}, owner, records[0]['previewStamp'], 'ErrorMessage')
                    self.assertEqual(self.profile(owner), restored)
                    self.assertEqual(self.profile(protected), protected_before)
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    self.assertEqual(self.sql(f"SELECT COUNT(*),SUM(delta_cents) FROM local_wallet_entries WHERE player_id={owner.playerId} AND kind='debit';").strip(), '2\t-203000')
                    report.update(status='PASS', fullChain=True, actualOrderIDs=orders, actualWalletCents=203000,
                        actualPaidTarget003GUID=target, actualOriginalFeedGUID=food.guid,
                        actualFeedConsumed=1, actualFeedRefundGUID=returned[0]['guid'], actualOriginalFeedExperience=400,
                        actualCross75Level=75, actualCross75Exp=399, actualDefaultSpecialties=3,
                        trustedBaselineAndCostsUnchangedByList=True, actualTrainingRecords=records,
                        actual007Bought=10, actual007Spent=6, actual007Refunded=6, actual007Final=10,
                        actualResetDiamondFee=50, finalCoin=restored[0]['coin'], finalDiamond=restored[0]['diamond'],
                        actualVIP7CoinSource=1000000, coldRestarts=2, eightSameStampOnce=True,
                        originalListAndContinuedGrowth=True, resetAndColdOnce=True, protected1Exact=True,
                        naturalLevelUnlockClaimed=False, nativeBattleClaimed=False,
                        highestRange='Original49 highest100/strength10 retained in existing evidence; no new full-scope or native claim')
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
