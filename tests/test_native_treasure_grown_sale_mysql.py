"""Prebuilt original022 -> actual growth/refine -> native21093 liquidation.

Natural level1; actual local2000-yuan wallet order -> VIP7 original041 coin/stone
parcel plus unchanged full random022 pool. Only existing VIP7 operator publication
is enabled in the isolated allocation. No business resource/level/GUID injection.
Root owns compilation, database window and execution. No external payment/phone.
"""
import concurrent.futures
import copy
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
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_local_play_training_rebate_mysql import sha
from test_p5_talent_scope_mysql import original_table
from server.codec import CLASSES, parse_http
import test_continued_gameplay_mysql as wire

ROOT = Path(__file__).resolve().parents[1]


def preflight():
    source = Path(os.environ['HYJS_GROWN_TREASURE_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_GROWN_TREASURE_BINARY']).resolve()
    record = Path(os.environ['HYJS_GROWN_TREASURE_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use a fixed direct candidate')
    manifest = source / 'manifest.json'
    pins = json.loads(manifest.read_bytes())['files']
    pinned = set()
    for row in pins:
        path = (source / row['path']).resolve()
        if not path.is_relative_to(source) or path in pinned or sha(path) != row['sha256']:
            raise AssertionError('Candidate source pin differs: ' + row['path'])
        pinned.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - pinned:
        raise AssertionError('Unpinned Go source')
    proof = json.loads(record.read_bytes())
    if not proof.get('qualified') or (ROOT / proof['candidate']).resolve() != source or sha(binary) != proof['binarySHA256']:
        raise AssertionError('Use the exact qualified app without a compiler')
    for name in ['server/codec.py', 'server/protocol/schema.json', 'analysis/tables/TableTreasure.tsv']:
        if sha(source / name) != sha(ROOT / name):
            raise AssertionError('Original protocol/table differs: ' + name)
    fields = {'cost': [{'id': 'trea_001', 'guid': 'actual-owned', 'num': 1}]}
    raw = wire.encode_original_http('CSEquipmentSell', fields, 2, b'0' * 16, 94001)
    if parse_http(raw).decode() != CLASSES['CSEquipmentSell'](**fields):
        raise AssertionError('Original21093 protocol differs')
    if 'p5NativeTreasureSalePrice(p, g, cost)' not in (source / 'hyjs-server/service/p5RecycleService.go').read_text():
        raise AssertionError('Native mixed sale adapter is missing')
    for name in ['ItemUse', 'TreasureStrengthLevelExp', 'TreasureRefineLevel', 'GameConfigure', 'Payment']:
        relative = 'analysis/tables/Table' + name + '.tsv'
        if sha(source / relative) != sha(ROOT / relative):
            raise AssertionError('Original source table differs: ' + name)
    vectors = {'CSTreasureStrengthen': {'targetId': 'actual', 'guids': ['actual-donor']},
               'CSTreasureRefine': {'targetId': 'actual'}}
    for name, fields in vectors.items():
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 94001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original growth protocol differs: ' + name)
    for name, fields in {'SCTreasureStrengthen': {'treasure', 'cost'},
                         'SCTreasureRefine': {'treasure', 'cost'},
                         'SCEquipmentSell': {'get', 'cost'}}.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original growth callback differs: ' + name)
    native = (source / 'hyjs-server/service/p5NativeTreasureSaleService.go').read_text()
    recycle = (source / 'hyjs-server/service/p5RecycleService.go').read_text()
    if 'p5.native-sale:' not in native or 's.p5SettleNativeTreasureSale(p, g, cost)' not in recycle:
        raise AssertionError('Complete tracked cash liquidation hook absent')
    gold = next(x for x in original_table('ItemUse') if x['itemId'] == 'item_041')
    contents = dict(x.split(',') for x in gold['result'].split(';'))
    if contents.get('coin') != '1000000' or contents.get('item_012') != '50':
        raise AssertionError('Original041 complete funding parcel differs')
    return source, binary, sha(manifest)


@unittest.skipUnless(os.environ.get('HYJS_GROWN_TREASURE_MYSQL_TEST') == '1', 'Root prebuilt managed window')
class GrownNativeTreasureSaleMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    stamped, profile, start, stop, sql = transport.stamped, transport.profile, transport.start, transport.stop, transport.sql
    json_request, wallet_body, wallet, order = transport.json_request, transport.wallet_body, transport.wallet, transport.order

    def test_original022_grown_cash_liquidation_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_GROWN_TREASURE_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/resources'))
        self.assertFalse(output.exists(), 'Keep prior results immutable')
        report = dict(status='PENDING', fullChain=False, binarySHA256=sha(binary), sourceManifestSHA256=manifest_sha,
                      runnerSHA256=sha(__file__), noBusinessResourceInjection=True, naturalLevel=1,
                      phoneUI=False, productionModified=False, externalPayment=False,
                      scope='Original21093 tracked low-tier growth/refine cash liquidation; original orange/red protection retained',
                      finitePayment='Actual local2000-yuan order/VIP7/one original041 parcel; no external money',
                      localRule='original-bag-low-treasure-sale-v2-cash-liquidation; cash-only; paid investment audited and settled')
        began = time.monotonic()
        self.pool, self.process = ManagedMySQL(), None
        with tempfile.TemporaryDirectory(prefix='hyjs-grown-treasure-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('grown_treasure')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-grown-treasure-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '出售保护', 'heroId': 'hero_001'}, protected, 94000, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-grown-treasure-' + suffix
                    owner = self.login(uid)
                    stamp = 94100

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        saved = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            rows = list(workers.map(lambda _: self.call(name, fields, owner, saved, expected), range(8)))
                        self.assertTrue(all(row == rows[0] for row in rows))
                        return rows[0], saved

                    def reject(fields):
                        before = self.profile(owner)
                        http('CSEquipmentSell', fields, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), before)

                    http('CSPlayerCreate', {'playerName': '原背包饰品', 'heroId': 'hero_001'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual((initial[0]['level'], initial[0].get('coin', 0), initial[0].get('diamond', 0)), (1, 0, 0))
                    # Enable only the existing fixed VIP7 product, preserving its
                    # original cost/reward/qualification. This is operator publication,
                    # with no profile, resource, level, receipt or random-pool writes.
                    row = self.sql("SELECT min_v_ip,cost,reward,enabled FROM p3_vip_shop_products WHERE id='p3vip7_growth_local';").strip()
                    self.assertEqual(row, '7\t800\titem_041,1\t0')
                    self.pool.sql(self.database, "UPDATE p3_vip_shop_products SET enabled=1 WHERE id='p3vip7_growth_local' AND min_v_ip=7 AND cost=800 AND reward='item_041,1' AND enabled=0;")
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=200000)
                    paid = self.order(owner, 'djsy.wuyou.andp2000', 94200)
                    self.assertEqual(paid.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    paid_profile = self.profile(owner)[0]
                    paid_diamond = paid_profile['diamond']
                    self.assertEqual((paid_profile['level'], paid_profile['vip']), (1, 7))
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    http('CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1}, 'SCShopBuy')
                    gold, _ = once('CSItemUse', {'costItem': {'id': 'item_041', 'num': 1}}, 'SCItemUse')
                    self.assertEqual({x.id: x.num for x in gold.get.simps}.get('coin'), 1000000)
                    self.assertEqual({x.id: x.num for x in gold.get.simps}.get('item_012'), 50)
                    once('CSShopBuy', {'shopType': 1, 'shopItemId': 'itemShopId_14', 'buyCount': 20}, 'SCShopBuy')
                    use_fields = {'costItem': {'id': 'item_022', 'num': 20}}
                    award, use_stamp = once('CSItemUse', use_fields, 'SCItemUse')
                    tables = {row['id']: row for row in original_table('Treasure')}
                    low = [entry for entry in award.get.treas if int(tables[entry.treasureId]['quality']) <= 18
                           and int(tables[entry.treasureId]['maxStrengthen']) > 1]
                    self.assertTrue(low, 'Bounded20 genuine draws have no usable low target; preserve random result')
                    target = low[0]
                    donors = [entry for entry in award.get.treas if entry.guid != target.guid]
                    self.assertTrue(donors, 'No actual original-box strengthening donor')
                    donor = donors[0]
                    report.update(actualRandom022IDs=[x.treasureId for x in award.get.treas],
                                  actualTargetID=target.treasureId, actualTargetGUID=target.guid,
                                  actualDonorID=donor.treasureId, actualDonorGUID=donor.guid)
                    cost = {'id': target.treasureId, 'guid': target.guid, 'num': 1}
                    request = {'cost': [cost]}
                    reject({'cost': [cost, cost]})
                    reject({'cost': [dict(cost, num=2)]})
                    reject({'cost': [dict(cost, id='trea_999')]})
                    high = [entry for entry in award.get.treas if int(tables[entry.treasureId]['quality']) > 18]
                    if high:
                        reject({'cost': [{'id': high[0].treasureId, 'guid': high[0].guid, 'num': 1}]})
                    strengthen_fields = {'targetId': target.guid, 'guids': [donor.guid]}
                    pre_growth = self.profile(owner)
                    strength, strength_stamp = once('CSTreasureStrengthen', strengthen_fields, 'SCTreasureStrengthen')
                    strength_coin = sum(x.num for x in strength.cost if x.id == 'coin')
                    self.assertGreater(strength_coin, 0)
                    self.assertEqual(self.profile(owner)[0]['coin'], pre_growth[0]['coin'] - strength_coin)
                    refine_fields = {'targetId': target.guid}
                    refined, refine_stamp = once('CSTreasureRefine', refine_fields, 'SCTreasureRefine')
                    self.assertEqual({x.id: x.num for x in refined.cost}, {'item_012': 30, 'coin': 30000})
                    before = self.profile(owner)
                    grown = next(x for x in before[1]['treasures'] if x['guid'] == target.guid)
                    self.assertEqual(grown['refineLevel'], 1)
                    self.assertGreater(grown['strengthLevel'], 1)
                    kv = lambda game: {x['key']: x.get('value', '') for x in game.get('kv', [])}
                    ledger_raw = kv(before[1])['p5.invest:' + target.guid]
                    ledger = json.loads(ledger_raw)
                    paid_costs = {x['id']: x['num'] for x in ledger['costs']}
                    self.assertEqual(paid_costs['coin'], strength_coin + 30000)
                    self.assertEqual(paid_costs['item_012'], 30)
                    self.assertEqual(paid_costs[donor.treasureId], 1)
                    quality = tables[target.treasureId]['quality']
                    exp = sum(int(x['exp']) for x in original_table('TreasureStrengthLevelExp')
                              if x['quality'] == quality and 1 <= int(x['level']) < grown['strengthLevel'])
                    price = int(tables[target.treasureId]['sellPrice']) + exp + grown.get('currentExp', 0)
                    self.assertGreater(price, int(tables[target.treasureId]['sellPrice']))
                    # Entire mixed batch must reject without clearing the valid ledger.
                    reject({'cost': [cost, {'id': 'trea_001', 'guid': 'not-owned', 'num': 1}]})
                    response, sale_stamp = once('CSEquipmentSell', request, 'SCEquipmentSell')
                    final = self.profile(owner)
                    self.assertEqual(final[0]['coin'] - before[0]['coin'], price)
                    self.assertEqual(final[0]['diamond'], paid_diamond - 800 - 100)
                    self.assertEqual(len(response.cost), 1)
                    self.assertEqual((response.cost[0].id, response.cost[0].guid, response.cost[0].num), (target.treasureId, target.guid, 1))
                    self.assertEqual([(x.id, x.num) for x in response.get.simps], [('coin', price)])
                    self.assertFalse(response.get.treas or response.get.equips or response.get.wars)
                    final_kv = kv(final[1])
                    self.assertEqual(final_kv.get('p5.invest:' + target.guid, ''), '')
                    audit = json.loads(final_kv['p5.native-sale:' + target.guid])
                    self.assertEqual(audit['policy'], 'original-bag-low-treasure-sale-v2-cash-liquidation')
                    self.assertEqual(audit['cash'], price)
                    self.assertEqual(audit['investment'], ledger)
                    self.assertEqual(audit['before'], grown)
                    expected_game = copy.deepcopy(before[1])
                    expected_game['treasures'] = [x for x in expected_game['treasures'] if x['guid'] != target.guid]
                    for x in expected_game.get('kv', []):
                        if x['key'] == 'p5.invest:' + target.guid:
                            x['value'] = ''
                    expected_game.setdefault('kv', []).append({'key': 'p5.native-sale:' + target.guid,
                                                              'value': final_kv['p5.native-sale:' + target.guid]})
                    self.assertEqual(final[1], expected_game, 'Unexpected refund or inventory/state changes')
                    self.assertEqual(final[0]['level'], 1)
                    reject(request)
                    for name, fields, old_stamp in [('CSItemUse', use_fields, use_stamp),
                            ('CSTreasureStrengthen', strengthen_fields, strength_stamp),
                            ('CSTreasureRefine', refine_fields, refine_stamp)]:
                        self.call(name, fields, owner, old_stamp, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), final)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), final)
                    self.call('CSEquipmentSell', request, owner, sale_stamp, 'SCEquipmentSell')
                    self.assertEqual(self.profile(owner), final)
                    for name, fields, old_stamp in [('CSItemUse', use_fields, use_stamp),
                            ('CSTreasureStrengthen', strengthen_fields, strength_stamp),
                            ('CSTreasureRefine', refine_fields, refine_stamp)]:
                        self.call(name, fields, owner, old_stamp, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), final)
                    reject(request)
                    self.assertEqual(self.profile(protected), protected_before)
                    report.update(status='PASS', fullChain=True, orderId=paid.orderId, originalBoxes=20,
                                  originalSaleCoin=price, strengthCoin=strength_coin, refineCoin=30000,
                                  refineStone=30, auditedPaidCosts=paid_costs, extraRefund=False,
                                  paidDiamond=paid_diamond, finalDiamond=final[0]['diamond'], finalCoin=final[0]['coin'],
                                  highTierObservedProtected=len(high), remainingTreasureCount=len(final[1]['treasures']),
                                  eightSameStampOnce=True, invalidAtomic=True, oldAwardResurrectionRejected=True,
                                  oldGrowthResurrectionRejected=True, coldExact=True, protected1Exact=True)
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


if __name__ == '__main__':
    unittest.main()
