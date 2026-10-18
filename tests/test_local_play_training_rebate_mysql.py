"""Prebuilt managed original training -> personal-day actual rebate -> cold.

Only Player10 is finite eligibility; no VIP, currency, materials, events or
training values are inserted. Every resource comes from a funded original30
order and ordinary shop. Root owns Go qualification/DB/phone windows.
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
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from server.codec import CLASSES, parse_http
import test_continued_gameplay_mysql as wire

ROOT = Path(__file__).resolve().parents[1]


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def protocol_preflight():
    vectors = {
        'CSShopBuy': {'shopType': 1, 'shopItemId': 'itemShopId_9', 'buyCount': 4},
        'CSWarriorTraining': {'guid': 'actual-owned-main', 'costType': 3, 'numType': 1},
        'CSWarriorTrainingReplace': {'guid': 'actual-owned-main', 'result': 1},
        'CSActivityInfo': {'activityId': 'consumeRet_p4_lp-actual'},
        'CSConsumeReturnReward': {'activityId': 'consumeRet_p4_lp-actual'},
    }
    for name, fields in vectors.items():
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 80101)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original training/rebate protocol differs: ' + name)
    for name, fields in {
        'SCWarriorTraining': {'guid', 'cost'},
        'SCWarriorTrainingReplace': {'result', 'cost'},
        'SCActivityInfo': {'activityId', 'itemList'},
        'SCActivityReward': {'activityId', 'itemList', 'get'},
    }.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original response consumer differs: ' + name)
    return {'requestVectors': len(vectors), 'finitePlayerLevel': 10,
            'actualShopCost': 120, 'actualTrainingCosts': [5, 25],
            'actualMaterialsBought': 20, 'actualMaterialsConsumed': [3, 15],
            'personalDayRebates': [12, 2], 'pveOrPhoneClaimed': False}


def preflight():
    source = Path(os.environ['HYJS_TRAINING_CLOCK_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_TRAINING_CLOCK_BINARY']).resolve()
    validation = Path(os.environ['HYJS_TRAINING_CLOCK_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use a fixed direct candidate')
    manifest = source / 'manifest.json'
    files = json.loads(manifest.read_bytes())['files']
    paths = set()
    for entry in files:
        path = (source / entry['path']).resolve()
        if source not in path.parents or path in paths or sha(path) != entry['sha256']:
            raise AssertionError('Frozen candidate pin differs: ' + entry['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Unpinned Go source in candidate')
    proof = json.loads(validation.read_bytes())
    if proof.get('qualified') is not True or (ROOT / proof['candidate']).resolve() != source:
        raise AssertionError('Completed exact root qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use exact qualified app; never compile here')
    production = (source / 'hyjs-server/service/warriorGrowthService.go').read_text()
    if '"warrior_training", 1, LocalPlayNow(ctx))' not in production:
        raise AssertionError('Actual training personal-day fix not integrated')
    for name in ['server/codec.py', 'server/protocol/schema.json', 'analysis/tables/TableGameConfigure.tsv']:
        if sha(source / name) != sha(ROOT / name):
            raise AssertionError('Original protocol/table differs: ' + name)
    return source, binary, sha(manifest), protocol_preflight()


@unittest.skipUnless(os.environ.get('HYJS_TRAINING_CLOCK_MYSQL_TEST') == '1',
                     'Root coordinated prebuilt managed window only')
class LocalPlayTrainingRebateMySQLTests(unittest.TestCase):
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
    order = transport.order
    tools = transport.tools
    change = transport.change

    def test_original_training_rewind_forward_rebate_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_TRAINING_CLOCK_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/activity-next-single-r1'))
        self.assertFalse(output.exists(), 'Keep prior failures immutable')
        report = dict(status='PENDING', fullChain=False, binarySHA256=sha(binary),
                      sourceManifestSHA256=manifest_sha, runnerSHA256=sha(__file__), contracts=contract,
                      noBusinessResourceInjection=True, finitePreparation='Only Player10; no natural level-up claim',
                      phoneUI=False, originalEngineNewScope=False, productionModified=False)
        began = time.monotonic()
        self.pool = ManagedMySQL()
        self.process = None
        with tempfile.TemporaryDirectory(prefix='hyjs-training-clock-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('training_clock')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-training-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '时钟保护', 'heroId': 'hero_001'}, protected, 80100, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-training-clock-' + suffix
                    owner = self.login(uid)
                    stamp = 80200

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        saved = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                            out = list(pool.map(lambda _: self.call(name, fields, owner, saved, expected), range(8)))
                        self.assertTrue(all(result == out[0] for result in out))
                        return out[0], saved

                    def count(item):
                        return sum(row['num'] for row in (self.profile(owner)[1].get('items') or []) if row['id'] == item)

                    def info(key):
                        return http('CSActivityInfo', {'activityId': key}, 'SCActivityInfo').itemList[0]

                    def train(num_type, material, diamond):
                        before = self.profile(owner)
                        old_material = count('item_007')
                        fields = dict(guid=guid, costType=3, numType=num_type)
                        _, saved = once('CSWarriorTraining', fields, 'SCWarriorTraining')
                        after = self.profile(owner)
                        self.assertEqual(before[0]['diamond'] - after[0]['diamond'], diamond)
                        self.assertEqual(old_material - count('item_007'), material)
                        self.assertEqual(next(w for w in before[1]['warriors'] if w['guid'] == guid).get('training', {}),
                                         next(w for w in after[1]['warriors'] if w['guid'] == guid).get('training', {}))
                        # Inspect actual accepted fact, never insert a fact or expected damage.
                        source_key = f'p5:training:{guid}:{saved}'
                        fact = self.sql("SELECT day,amount,occurred_at FROM p1_event_facts WHERE player_id=" + str(owner.playerId) +
                                        " AND kind='warrior_training' AND source_key='" + source_key + "';").strip().split('\t')
                        self.assertEqual(len(fact), 3)
                        state = self.tools(owner, 'time/state')
                        expected_day = time.strftime('%Y-%m-%d', time.gmtime(state['gameTimeMs'] / 1000 + 8 * 3600))
                        self.assertEqual((fact[0], fact[1]), (expected_day, '1'))
                        _, confirm_stamp = once('CSWarriorTrainingReplace', {'guid': guid, 'result': 1}, 'SCWarriorTrainingReplace')
                        grown = next(w for w in self.profile(owner)[1]['warriors'] if w['guid'] == guid)
                        self.assertFalse(grown.get('tempTraining'))
                        return saved, confirm_stamp, fact

                    def claim(key, amount):
                        item = info(key)
                        self.assertEqual((item.status, item.left), (1, amount))
                        before = self.profile(owner)[0]['diamond']
                        _, saved = once('CSConsumeReturnReward', {'activityId': key}, 'SCActivityReward')
                        self.assertEqual(self.profile(owner)[0]['diamond'] - before, amount)
                        self.assertEqual(info(key).status, 2)
                        return saved

                    http('CSPlayerCreate', {'playerName': '训练返利日期', 'heroId': 'hero_043'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual((initial[0].get('coin', 0), initial[0].get('diamond', 0), count('item_007')), (0, 0, 0))
                    # Bounded eligibility only; preserve every wallet/resource/warrior fact.
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',10) WHERE id={owner.playerId} AND uid LIKE 'demo-training-clock-%';")
                    eligible = self.profile(owner)
                    self.assertEqual(eligible[0], dict(initial[0], level=10))
                    self.assertEqual(eligible[1], initial[1])
                    guid = eligible[0]['heroWarId']
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=3000)
                    generated = self.order(owner, 'djsy.wuyou.andp0030', 80300)
                    self.assertEqual(generated.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    paid = self.profile(owner)
                    paid_diamond = paid[0]['diamond']
                    self.assertGreaterEqual(paid_diamond, 150)
                    order_before = self.sql("SELECT created_at,confirmed_at,cents FROM p3_payment_orders WHERE id='" + generated.orderId + "';").strip()
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    self.change(owner, 'previousDay')
                    self.tools(owner, 'activities/apply', action='open', activityId='CostReturn', requestId=str(uuid.uuid4()))
                    cycle = self.sql(f"SELECT cycle_id FROM local_play_activity_bindings WHERE player_id={owner.playerId} AND activity_id='CostReturn';").strip()
                    key = 'consumeRet_p4_' + cycle
                    self.assertEqual(info(key).left, 0)
                    once('CSShopBuy', {'shopType': 1, 'shopItemId': 'itemShopId_9', 'buyCount': 4}, 'SCShopBuy')
                    self.assertEqual(count('item_007'), 20)
                    first = train(1, 3, 5)
                    claim(key, 12)
                    self.change(owner, 'nextDay')
                    self.assertEqual(self.tools(owner, 'time/state')['offsetDays'], 0)
                    self.assertEqual(info(key).left, 0)
                    second = train(2, 15, 25)
                    second_claim = claim(key, 2)
                    final = self.profile(owner)
                    self.assertEqual((final[0]['diamond'], count('item_007')), (paid_diamond - 136, 2))
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={owner.playerId} AND kind='warrior_training';").strip(), '2')
                    self.assertNotEqual(first[2][0], second[2][0])
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(info(key).status, 2)
                    self.call('CSConsumeReturnReward', {'activityId': key}, owner, second_claim, 'SCActivityReward')
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.sql("SELECT created_at,confirmed_at,cents FROM p3_payment_orders WHERE id='" + generated.orderId + "';").strip(), order_before)
                    self.assertEqual(self.profile(protected), protected_before)
                    report.update(status='PASS', fullChain=True, orderId=generated.orderId, paidDiamond=paid_diamond,
                                  shopDiamond=120, trainingDiamond=30, materialBought=20, materialSpent=18,
                                  actualRebates=[12, 2], finalDiamond=paid_diamond - 136, remaining007=2,
                                  eightSameStampOnce=True, personalDayFacts=[first[2], second[2]],
                                  coldExact=True, paymentRealTimeUnchanged=True, protected1Exact=True)
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
