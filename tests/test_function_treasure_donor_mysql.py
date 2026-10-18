"""Prebuilt original treasure strengthening with a legitimately grown donor.

Only level/balances/two original040 parcels are finite isolated prerequisites.
GUIDs, growth, investments, refunds and receipts come from original requests.
No new-account progression, compiler, production mutation or phone operation.
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

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_function_pet_equip_five_mysql import FunctionPetEquipFiveMySQLTests as frames
from test_local_play_training_rebate_mysql import sha

ROOT = Path(__file__).resolve().parents[1]
DELIVERY = ROOT / 'analysis/growth-first-20261016/function-treasure-donor-r1-20261017'


def preflight():
    source = Path(os.environ['HYJS_TREASURE_DONOR_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_TREASURE_DONOR_BINARY']).resolve()
    validation = Path(os.environ['HYJS_TREASURE_DONOR_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Require a fixed direct candidate')
    manifest = source / 'manifest.json'
    pins = json.loads(manifest.read_bytes())['files']
    known = set()
    for row in pins:
        path = (source / row['path']).resolve()
        if not path.is_relative_to(source) or path in known or sha(path) != row['sha256']:
            raise AssertionError('Candidate pin differs: ' + row['path'])
        known.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - known:
        raise AssertionError('Unpinned Go source')
    proof = json.loads(validation.read_bytes())
    if not proof.get('qualified') or (ROOT / proof['candidate']).resolve() != source:
        raise AssertionError('Require exact qualified candidate')
    if sha(binary) != proof['binarySHA256']:
        raise AssertionError('Require exact prebuilt binary')
    for rel in ['server/codec.py', 'server/protocol/schema.json',
                *[f'analysis/tables/Table{x}.tsv' for x in
                  ['Treasure', 'TreasureStrengthLevelExp', 'GameConfigure', 'ItemUse']]]:
        if sha(source / rel) != sha(ROOT / rel):
            raise AssertionError('Original contract differs: ' + rel)
    return source, binary, sha(manifest)


@unittest.skipUnless(os.environ.get('HYJS_TREASURE_DONOR_MYSQL_TEST') == '1',
                     'Root prebuilt managed database window')
class FunctionTreasureDonorMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    profile, stop, sql = transport.profile, transport.stop, transport.sql
    start, raw_profile = frames.start, frames.raw_profile
    # Original item use also emits SCPlayer; reuse the multi-frame transport.
    stamped = transport.stamped

    def test_grown_donor_transfers_investment_once_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_TREASURE_DONOR_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(DELIVERY))
        self.assertFalse(output.exists(), 'Keep previous attempts immutable')
        report = dict(status='PENDING', fullChain=False, runnerSHA256=sha(__file__),
                      binarySHA256=sha(binary), sourceManifestSHA256=manifest_sha,
                      finitePrerequisites='level16/100000coin/100diamond/item040x2',
                      naturalAcquisitionProven=False, productionModified=False,
                      injectedGUIDOrInvestment=False, phoneUI=False, phase='allocate')
        self.pool, self.process = ManagedMySQL(), None
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-treasure-donor-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('treasure_donor')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items()
                                if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-donor-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '饰品保护', 'heroId': 'hero_043'},
                              protected, 934000, 'SCPlayerCreate')
                    protected_before = self.raw_profile(protected)
                    uid = 'demo-treasure-donor-' + suffix
                    owner = self.login(uid)
                    self.assertGreater(owner.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '饰品材料', 'heroId': 'hero_047'},
                              owner, 934001, 'SCPlayerCreate')
                    # A finite legal prerequisite, with no donor state or ledger fabricated.
                    self.pool.sql(self.database,
                        f"UPDATE profiles SET player=JSON_SET(player,'$.level',16,'$.coin',100000,'$.diamond',100),"
                        "game=JSON_SET(game,'$.items',JSON_ARRAY(JSON_OBJECT('id','item_040','num',2))) "
                        f"WHERE id={owner.playerId};")
                    stamp = 934002

                    def http(name, fields, response):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, response)

                    def once(name, fields, response):
                        nonlocal stamp
                        stamp += 1
                        value = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                            replies = list(pool.map(lambda _: self.call(name, fields, owner, value, response), range(8)))
                        self.assertTrue(all(r == replies[0] for r in replies))
                        return replies[0], value

                    def reject(name, fields, old=None):
                        before = self.raw_profile(owner)
                        if old is None:
                            http(name, fields, 'ErrorMessage')
                        else:
                            self.call(name, fields, owner, old, 'ErrorMessage')
                        self.assertEqual(self.raw_profile(owner), before)

                    report['phase'] = 'original-item-use-and-donor-growth'
                    gifts = [http('CSItemUse', {'costItem': {'id': 'item_040', 'num': 1}}, 'SCItemUse')
                             for _ in range(2)]
                    for gift in gifts:
                        self.assertEqual(sorted(t.treasureId for t in gift.get.treas), ['trea_011', 'trea_013'])
                        self.assertEqual(sorted((v.id, v.num) for v in gift.get.simps),
                                         [('item_002', 10), ('item_005', 1000), ('item_048', 4)])
                        self.assertEqual((gift.cost.id, gift.cost.num), ('item_040', 1))
                    donor = next(t for t in gifts[0].get.treas if t.treasureId == 'trea_011')
                    food = next(t for t in gifts[0].get.treas if t.treasureId == 'trea_013')
                    target = next(t for t in gifts[1].get.treas if t.treasureId == 'trea_011')
                    first = {'targetId': donor.guid, 'guids': [food.guid]}
                    grown, first_stamp = once('CSTreasureStrengthen', first, 'SCTreasureStrengthen')
                    self.assertGreater(grown.treasure.strengthLevel, 1)
                    self.assertEqual(sum(v.num for v in grown.cost if v.id == 'coin'), 20000)
                    self.assertEqual(self.profile(owner)[0]['coin'], 80000)
                    report['phase'] = 'consume-legally-grown-donor'
                    second = {'targetId': target.guid, 'guids': [donor.guid]}
                    result, second_stamp = once('CSTreasureStrengthen', second, 'SCTreasureStrengthen')
                    self.assertGreater(result.treasure.strengthLevel, grown.treasure.strengthLevel)
                    self.assertEqual(sum(v.num for v in result.cost if v.id == 'coin'), 40000)
                    self.assertEqual(self.profile(owner)[0]['coin'], 40000)
                    self.assertNotIn(donor.guid, [v['guid'] for v in self.profile(owner)[1]['treasures']])
                    reject('CSTreasureStrengthen', first, first_stamp)
                    before_cold = self.profile(owner)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), before_cold)
                    self.assertEqual(self.call('CSTreasureStrengthen', second, owner, second_stamp,
                                               'SCTreasureStrengthen'), result)
                    self.assertEqual(self.profile(owner), before_cold)
                    reject('CSTreasureStrengthen', first, first_stamp)
                    report['phase'] = 'reset-transferred-investment'
                    reset, reset_stamp = once('CStreasureRebuild', {'targetId': target.guid}, 'SCtreasureRebuild')
                    self.assertEqual(sum(v.num for v in reset.cost if v.id == 'diamond'), 50)
                    self.assertEqual(sum(v.num for v in reset.get.simps if v.id == 'coin'), 60000)
                    # The original callback includes the reset target alongside new refunds.
                    reset_target = [v for v in reset.get.treas if v.guid == target.guid]
                    self.assertEqual(len(reset_target), 1)
                    self.assertEqual((reset_target[0].treasureId, reset_target[0].strengthLevel,
                                      reset_target[0].refineLevel, reset_target[0].currentExp),
                                     ('trea_011', 1, 0, 0))
                    refund_treasures = [v for v in reset.get.treas if v.guid != target.guid]
                    refunded = sorted(v.treasureId for v in refund_treasures)
                    self.assertEqual(refunded, ['trea_011', 'trea_013'])
                    self.assertTrue(all(v.guid not in [donor.guid, food.guid, target.guid] for v in refund_treasures))
                    self.assertEqual((self.profile(owner)[0]['coin'], self.profile(owner)[0]['diamond']), (100000, 50))
                    reject('CSTreasureStrengthen', second, second_stamp)
                    saved = self.profile(owner)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), saved)
                    self.assertEqual(self.call('CStreasureRebuild', {'targetId': target.guid}, owner,
                                               reset_stamp, 'SCtreasureRebuild'), reset)
                    self.assertEqual(self.profile(owner), saved)
                    self.assertEqual(self.raw_profile(protected), protected_before)
                    report.update(status='PASS', fullChain=True, actualDonorCoin=20000,
                                  actualTargetCoin=40000, refundCoin=60000, refundIDs=refunded,
                                  eightSameStampOnce=True, coldExact=True,
                                  staleDonorAndTargetReceiptsRejected=True, protected1Exact=True)
                except BaseException as error:
                    report.update(status='FAIL', fullChain=False, failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    try:
                        self.stop()
                    finally:
                        try:
                            self.pool.close()
                        finally:
                            report['seconds'] = round(time.monotonic() - began, 3)
                            report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                            if self.pool.connections:
                                report.update(status='FAIL', fullChain=False, cleanupFailure=True)
                            output.parent.mkdir(parents=True, exist_ok=True)
                            output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
