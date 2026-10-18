"""Prebuilt original022 -> original21093 -> real coin -> cold/replay.

Natural level1, actual local wallet/order and unchanged full random box pool.
No resource/level/GUID/receipt injection; root owns managed SQL/build windows.
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
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_local_play_training_rebate_mysql import sha
from test_p5_talent_scope_mysql import original_table
from server.codec import CLASSES, parse_http
import test_continued_gameplay_mysql as wire

ROOT = Path(__file__).resolve().parents[1]


def preflight():
    source = Path(os.environ['HYJS_NATIVE_TREASURE_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_NATIVE_TREASURE_BINARY']).resolve()
    record = Path(os.environ['HYJS_NATIVE_TREASURE_VALIDATION']).resolve()
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
    return source, binary, sha(manifest)


@unittest.skipUnless(os.environ.get('HYJS_NATIVE_TREASURE_MYSQL_TEST') == '1', 'Root prebuilt managed window')
class NativeTreasureSaleMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    stamped, profile, start, stop, sql = transport.stamped, transport.profile, transport.start, transport.stop, transport.sql
    json_request, wallet_body, wallet, order = transport.json_request, transport.wallet_body, transport.wallet, transport.order

    def test_original022_native_sale_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_NATIVE_TREASURE_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/resources'))
        self.assertFalse(output.exists(), 'Keep prior results immutable')
        report = dict(status='PENDING', fullChain=False, binarySHA256=sha(binary), sourceManifestSHA256=manifest_sha,
                      runnerSHA256=sha(__file__), noBusinessResourceInjection=True, naturalLevel=1,
                      phoneUI=False, productionModified=False, externalPayment=False,
                      scope='Original21093 pristine low-tier treasure; grown sale remains pending')
        began = time.monotonic()
        self.pool, self.process = ManagedMySQL(), None
        with tempfile.TemporaryDirectory(prefix='hyjs-native-treasure-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('native_treasure')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-native-treasure-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '出售保护', 'heroId': 'hero_001'}, protected, 94000, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-native-treasure-' + suffix
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
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=3000)
                    paid = self.order(owner, 'djsy.wuyou.andp0030', 94200)
                    self.assertEqual(paid.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    paid_diamond = self.profile(owner)[0]['diamond']
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    once('CSShopBuy', {'shopType': 1, 'shopItemId': 'itemShopId_14', 'buyCount': 20}, 'SCShopBuy')
                    use_fields = {'costItem': {'id': 'item_022', 'num': 20}}
                    award, use_stamp = once('CSItemUse', use_fields, 'SCItemUse')
                    tables = {row['id']: row for row in original_table('Treasure')}
                    low = [entry for entry in award.get.treas if int(tables[entry.treasureId]['quality']) <= 18]
                    self.assertTrue(low, 'Bounded20 genuine draws produced no low-tier sample; preserve random result')
                    target = low[0]
                    cost = {'id': target.treasureId, 'guid': target.guid, 'num': 1}
                    request = {'cost': [cost]}
                    reject({'cost': [cost, cost]})
                    reject({'cost': [dict(cost, num=2)]})
                    reject({'cost': [dict(cost, id='trea_999')]})
                    high = [entry for entry in award.get.treas if int(tables[entry.treasureId]['quality']) > 18]
                    if high:
                        reject({'cost': [{'id': high[0].treasureId, 'guid': high[0].guid, 'num': 1}]})
                    before = self.profile(owner)
                    response, sale_stamp = once('CSEquipmentSell', request, 'SCEquipmentSell')
                    final = self.profile(owner)
                    price = int(tables[target.treasureId]['sellPrice'])
                    self.assertEqual(final[0].get('coin', 0) - before[0].get('coin', 0), price)
                    self.assertEqual(final[0]['diamond'], paid_diamond - 100)
                    self.assertEqual(len(response.cost), 1)
                    self.assertEqual((response.cost[0].id, response.cost[0].guid, response.cost[0].num), (target.treasureId, target.guid, 1))
                    expected_game = dict(before[1], treasures=[entry for entry in before[1]['treasures'] if entry['guid'] != target.guid])
                    self.assertEqual(final[1], expected_game)
                    self.assertEqual(final[0]['level'], 1)
                    reject(request)
                    self.call('CSItemUse', use_fields, owner, use_stamp, 'ErrorMessage')
                    self.assertEqual(self.profile(owner), final)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), final)
                    self.call('CSEquipmentSell', request, owner, sale_stamp, 'SCEquipmentSell')
                    self.assertEqual(self.profile(owner), final)
                    self.call('CSItemUse', use_fields, owner, use_stamp, 'ErrorMessage')
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.profile(protected), protected_before)
                    report.update(status='PASS', fullChain=True, orderId=paid.orderId, originalBoxes=20,
                                  actualTreasureID=target.treasureId, actualGUID=target.guid, originalSaleCoin=price,
                                  paidDiamond=paid_diamond, finalDiamond=final[0]['diamond'], finalCoin=final[0].get('coin', 0),
                                  highTierObservedProtected=len(high), remainingTreasureCount=len(final[1]['treasures']),
                                  eightSameStampOnce=True, invalidAtomic=True, oldAwardResurrectionRejected=True,
                                  coldExact=True, protected1Exact=True)
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
