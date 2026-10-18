"""Prebuilt original warrior upgrading with a legitimately grown donor.

Only level/balances/three original067 choice boxes are finite isolated prerequisites.
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
from server.codec import CLASSES, BY_NAME, SCHEMA, parse_http
import test_continued_gameplay_mysql as wire
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_function_pet_equip_five_mysql import FunctionPetEquipFiveMySQLTests as frames
from test_local_play_training_rebate_mysql import sha

ROOT = Path(__file__).resolve().parents[1]
DELIVERY = ROOT / 'analysis/growth-first-20261016/function-warrior-donor-r1-20261017'


def protocol_preflight():
    vectors = {
        'CSPlayerCreate': {'playerName': '伙伴材料', 'heroId': 'hero_047'},
        'CSItemUse': {'costItem': {'id': 'item_067', 'num': 1}, 'getItemIndex': 1},
        'CSWarriorStrengthen': {'guid': 'target-guid',
            'cost': [{'id': 'hero_011', 'guid': 'donor-guid'}]},
        'CSWarriorReset': {'guid': 'target-guid'},
    }
    for name, fields in vectors.items():
        decoded = parse_http(wire.encode_original_http(name, fields, 2, b'0'*16, 934001))
        if decoded.opcode != BY_NAME[name] or decoded.decode() != CLASSES[name](**fields):
            raise AssertionError('Original wire changed: ' + name)
    # Pin each actual container. Item use cost is singular; the reset target
    # is separate from refunded units even when a result projection includes it.
    contracts = {
        'CSItemUse': {'costItem': ('BaseResource', False), 'getItemIndex': ('int32', False)},
        'SCItemUse': {'get': ('ResultObject', False), 'cost': ('BaseResource', False)},
        'SCWarriorStrengthen': {'guid': ('Warrior', False), 'cost': ('BaseResource', True)},
        'SCWarriorReset': {'cost': ('BaseResource', True), 'get': ('ResultObject', False),
                           'warriors': ('Warrior', True)},
        'ResultObject': {'wars': ('Warrior', True), 'simps': ('SimpleItem', True)},
    }
    for name, wanted in contracts.items():
        found = {f['name']: (f['type'], f.get('repeated', False))
                 for f in SCHEMA['messages'][name]['fields']}
        if any(found.get(key) != value for key, value in wanted.items()):
            raise AssertionError('Original container changed: ' + name)
    expected = {'CSItemUse': 21089, 'SCItemUse': 21090,
                'CSWarriorStrengthen': 21027, 'SCWarriorStrengthen': 21028,
                'CSWarriorReset': 21019, 'SCWarriorReset': 21020}
    if any(BY_NAME[name] != number for name, number in expected.items()):
        raise AssertionError('Original warrior opcode changed')
    # Construct the original result shapes without launching an application.
    CLASSES['SCItemUse'](get={'wars': [{'id': 'hero_011', 'guid': 'a', 'level': 1, 'exp': 0}]},
                         cost={'id': 'item_067', 'num': 1}).SerializeToString()
    CLASSES['SCWarriorStrengthen'](guid={'id': 'hero_011', 'guid': 'a', 'level': 4, 'exp': 151},
        cost=[{'id': 'hero_011', 'guid': 'b', 'num': 1}, {'id': 'coin', 'num': 1000}]).SerializeToString()
    CLASSES['SCWarriorReset'](cost=[{'id': 'diamond', 'num': 50}],
        get={'simps': [{'id': 'coin', 'num': 3000}],
             'wars': [{'id': 'hero_011', 'guid': 'new-a', 'level': 1, 'exp': 0},
                      {'id': 'hero_011', 'guid': 'new-b', 'level': 1, 'exp': 0}]},
        warriors=[{'id': 'hero_011', 'guid': 'target-guid', 'level': 1, 'exp': 0}]).SerializeToString()
    return dict(requestVectors=len(vectors), responseContainers=len(contracts), opcodes=expected)


def preflight():
    protocol_preflight()
    source = Path(os.environ['HYJS_WARRIOR_DONOR_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_WARRIOR_DONOR_BINARY']).resolve()
    validation = Path(os.environ['HYJS_WARRIOR_DONOR_VALIDATION']).resolve()
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
                  ['Hero', 'HeroExp', 'GameConfigure', 'ItemUse']]]:
        if sha(source / rel) != sha(ROOT / rel):
            raise AssertionError('Original contract differs: ' + rel)
    integrated = (source / 'hyjs-server/service/p5InvestmentService.go').read_text()
    if 's.p5WarriorMaterialCosts(beforeP, beforeG, g, w, guid, request)' not in integrated:
        raise AssertionError('Require integrated warrior donor save')
    for relative in ['hyjs-server/service/p5WarriorMaterialService.go',
                     'hyjs-server/constUtil/p5WarriorMaterial.go']:
        if not (source / relative).is_file():
            raise AssertionError('Required warrior domain missing: ' + relative)
    return source, binary, sha(manifest)


@unittest.skipUnless(os.environ.get('HYJS_WARRIOR_DONOR_MYSQL_TEST') == '1',
                     'Root prebuilt managed database window')
class FunctionWarriorDonorMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    profile, stop, sql = transport.profile, transport.stop, transport.sql
    start, raw_profile = frames.start, frames.raw_profile
    # Original item use also emits SCPlayer; reuse the multi-frame transport.
    stamped = transport.stamped

    def test_grown_donor_transfers_investment_once_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_WARRIOR_DONOR_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(DELIVERY))
        self.assertFalse(output.exists(), 'Keep previous attempts immutable')
        report = dict(status='PENDING', fullChain=False, runnerSHA256=sha(__file__),
                      binarySHA256=sha(binary), sourceManifestSHA256=manifest_sha,
                      finitePrerequisites='level16/10000coin/100diamond/item067x3',
                      naturalAcquisitionProven=False, productionModified=False,
                      injectedGUIDOrInvestment=False, phoneUI=False, phase='allocate')
        self.pool, self.process = ManagedMySQL(), None
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-warrior-donor-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('warrior_donor')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items()
                                if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-donor-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '伙伴保护', 'heroId': 'hero_043'},
                              protected, 934000, 'SCPlayerCreate')
                    protected_before = self.raw_profile(protected)
                    uid = 'demo-warrior-donor-' + suffix
                    owner = self.login(uid)
                    self.assertGreater(owner.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '伙伴材料', 'heroId': 'hero_047'},
                              owner, 934001, 'SCPlayerCreate')
                    # A finite legal prerequisite, with no donor state or ledger fabricated.
                    self.pool.sql(self.database,
                        f"UPDATE profiles SET player=JSON_SET(player,'$.level',16,'$.coin',10000,'$.diamond',100),"
                        "game=JSON_SET(game,'$.items',JSON_ARRAY(JSON_OBJECT('id','item_067','num',3))) "
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

                    report['phase'] = 'original-067-choice-and-donor-growth'
                    gifts = [http('CSItemUse', {'costItem': {'id': 'item_067', 'num': 1},
                                               'getItemIndex': 1}, 'SCItemUse')
                             for _ in range(3)]
                    for gift in gifts:
                        self.assertEqual([(w.id, w.level, w.exp) for w in gift.get.wars],
                                         [('hero_011', 1, 0)])
                        self.assertEqual((gift.cost.id, gift.cost.num), ('item_067', 1))
                        self.assertFalse(gift.get.simps or gift.get.equips or gift.get.treas or gift.get.pets)
                    donor, food, target = [gift.get.wars[0] for gift in gifts]
                    original_guids = {donor.guid, food.guid, target.guid}
                    self.assertEqual(len(original_guids), 3)
                    self.assertFalse(any(v['id'] == 'item_067' and v.get('num', 0)
                                         for v in self.profile(owner)[1]['items']))
                    first = {'guid': donor.guid, 'cost': [{'id': food.id, 'guid': food.guid}]}
                    grown = http('CSWarriorStrengthen', first, 'SCWarriorStrengthen')
                    first_stamp = stamp
                    self.assertEqual((grown.guid.guid, grown.guid.level, grown.guid.exp),
                                     (donor.guid, 4, 151))
                    self.assertEqual([(v.id, v.guid, v.num) for v in grown.cost],
                                     [('hero_011', food.guid, 1), ('coin', '', 1000)])
                    self.assertEqual(self.profile(owner)[0]['coin'], 9000)
                    report['phase'] = 'consume-legally-grown-warrior'
                    second = {'guid': target.guid, 'cost': [{'id': donor.id, 'guid': donor.guid}]}
                    result, second_stamp = once('CSWarriorStrengthen', second, 'SCWarriorStrengthen')
                    self.assertEqual((result.guid.guid, result.guid.level, result.guid.exp),
                                     (target.guid, 5, 271))
                    self.assertEqual([(v.id, v.guid, v.num) for v in result.cost],
                                     [('hero_011', donor.guid, 1), ('coin', '', 2000)])
                    p, game = self.profile(owner)
                    self.assertEqual((p['coin'], p['diamond']), (7000, 100))
                    self.assertFalse({donor.guid, food.guid} & {v['guid'] for v in game['warriors']})
                    kv = {v['key']: v.get('value', '') for v in game.get('kv', [])}
                    investment = json.loads(kv['p5.invest:' + target.guid])
                    self.assertEqual({v['id']: v['num'] for v in investment['costs']},
                                     {'coin': 3000, 'hero_011': 2})
                    self.assertFalse(kv.get('p5.invest:' + donor.guid))
                    audit = json.loads(kv['p5.warrior-material:' + donor.guid])
                    self.assertEqual(audit['targetGuid'], target.guid)
                    self.assertEqual(audit['before']['guid'], donor.guid)
                    reject('CSWarriorStrengthen', first, first_stamp)
                    before_cold = self.profile(owner)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), before_cold)
                    self.assertEqual(self.call('CSWarriorStrengthen', second, owner, second_stamp,
                                               'SCWarriorStrengthen'), result)
                    self.assertEqual(self.profile(owner), before_cold)
                    reject('CSWarriorStrengthen', first, first_stamp)
                    report['phase'] = 'reset-transferred-warrior-investment'
                    reset, reset_stamp = once('CSWarriorReset', {'guid': target.guid}, 'SCWarriorReset')
                    self.assertEqual([(v.id, v.num) for v in reset.cost], [('diamond', 50)])
                    self.assertEqual([(v.id, v.num) for v in reset.get.simps], [('coin', 3000)])
                    reset_target = [v for v in reset.get.wars if v.guid == target.guid]
                    self.assertLessEqual(len(reset_target), 1)
                    for value in reset_target:
                        self.assertEqual((value.id, value.level, value.exp), ('hero_011', 1, 0))
                    # The target is an original reset projection, not a returned material.
                    refund_units = [v for v in reset.get.wars if v.guid != target.guid]
                    self.assertEqual([(v.id, v.level, v.exp) for v in refund_units],
                                     [('hero_011', 1, 0), ('hero_011', 1, 0)])
                    refund_guids = {v.guid for v in refund_units}
                    self.assertEqual(len(refund_guids), 2)
                    self.assertFalse(refund_guids & original_guids)
                    self.assertEqual([(v.guid, v.level, v.exp) for v in reset.warriors],
                                     [(target.guid, 1, 0)])
                    p, game = self.profile(owner)
                    self.assertEqual((p['coin'], p['diamond']), (10000, 50))
                    actual = [v for v in game['warriors'] if v['id'] == 'hero_011']
                    self.assertEqual({v['guid'] for v in actual}, {target.guid} | refund_guids)
                    self.assertTrue(all(v['level'] == 1 and v.get('exp', 0) == 0 for v in actual))
                    kv = {v['key']: v.get('value', '') for v in game.get('kv', [])}
                    self.assertFalse(kv.get('p5.invest:' + target.guid))
                    reject('CSWarriorStrengthen', second, second_stamp)
                    saved = self.profile(owner)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), saved)
                    self.assertEqual(self.call('CSWarriorReset', {'guid': target.guid}, owner,
                                               reset_stamp, 'SCWarriorReset'), reset)
                    self.assertEqual(self.profile(owner), saved)
                    self.assertEqual(self.raw_profile(protected), protected_before)
                    report.update(status='PASS', fullChain=True, actualDonorCoin=1000,
                                  actualTargetCoin=2000, refundCoin=3000,
                                  refundIDs=[v.id for v in refund_units],
                                  sourceChoiceItem='item_067', sourceChoiceIndex=1,
                                  actualTargetLevel=5, actualTargetEXP=271,
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
