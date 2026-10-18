"""Prebuilt original warrior upgrading with confirmed training and cooked donor investments.

Only level/balances/original boxes, training stones and cooked027 stock are finite prerequisites.
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
import test_function_warrior_donor_mysql as original

sha = original.sha

ROOT = Path(__file__).resolve().parents[1]
DELIVERY = ROOT / 'analysis/growth-first-20261016/function-warrior-grown-material-r1-20261017'


def protocol_preflight(source=ROOT):
    old = original.protocol_preflight()
    vectors = {
        'CSWarriorTraining': {'guid': 'donor', 'costType': 3, 'numType': 1},
        'CSWarriorTrainingReplace': {'guid': 'donor', 'result': 1},
        'CSFishUsing': {'heroGuid': 'donor', 'costItem': {'id': 'FishItem_027', 'num': 1}},
        'CSWarriorStrengthen': {'guid': 'target',
            'cost': [{'id': 'hero_011', 'guid': 'a'}, {'id': 'hero_011', 'guid': 'b'}]},
        'CSItemUse': {'costItem': {'id': 'item_066', 'num': 1}},
    }
    for name, fields in vectors.items():
        decoded = parse_http(wire.encode_original_http(name, fields, 2, b'0'*16, 936001))
        if decoded.opcode != BY_NAME[name] or decoded.decode() != CLASSES[name](**fields):
            raise AssertionError('Original grown-material wire changed: ' + name)
    contracts = {
        'SCWarriorTraining': {'guid': ('Warrior', False), 'cost': ('BaseResource', True)},
        'SCWarriorTrainingReplace': {'result': ('Warrior', False), 'cost': ('BaseResource', True)},
        'SCFishUsing': {'warrior': ('Warrior', False), 'cost': ('BaseResource', True)},
    }
    for name, wanted in contracts.items():
        actual = {f['name']: (f['type'], f.get('repeated', False))
                  for f in SCHEMA['messages'][name]['fields']}
        if any(actual.get(key) != value for key, value in wanted.items()):
            raise AssertionError('Original response container changed: ' + name)
    expected = {'CSWarriorTraining': 21031, 'SCWarriorTraining': 21032,
                'CSWarriorTrainingReplace': 21033, 'SCWarriorTrainingReplace': 21034,
                'CSFishUsing': 24715, 'SCFishUsing': 24716}
    if any(BY_NAME[name] != number for name, number in expected.items()):
        raise AssertionError('Original training/cooking opcode changed')
    for name, body in {
        'SCWarriorTraining': {'guid': {'id': 'hero_011', 'guid': 'a', 'level': 12, 'exp': 2088,
            'tempTraining': {'state': 1, 'att': 1}}, 'cost': [{'id': 'item_007', 'num': 3}, {'id': 'diamond', 'num': 5}]},
        'SCWarriorTrainingReplace': {'result': {'id': 'hero_011', 'guid': 'a',
            'training': {'state': 0, 'att': 1}}, 'cost': []},
        'SCFishUsing': {'warrior': {'id': 'hero_011', 'guid': 'a', 'cooking': {'prop': {'pdef': 1.0}}},
            'cost': [{'id': 'FishItem_027', 'num': 1}]},
    }.items():
        CLASSES[name](**body).SerializeToString()

    def rows(name):
        lines = [line.split('\t') for line in (source / f'analysis/tables/Table{name}.tsv').read_text().splitlines()]
        return [dict(zip(lines[1], line)) for line in lines[5:] if line]

    configure = {r['StaticName']: r['NameValue'] for r in rows('GameConfigure')}
    for key, value in {'HeroTrainLevel': '10', 'OutdoorLevel': '18',
                       'WarriorTraining3': 'item_007,3;diamond,5', 'WarriorRebuildCost': 'diamond,50'}.items():
        if configure[key] != value:
            raise AssertionError('Original gate/cost differs: ' + key)
    dish = next(r for r in rows('FishSkill') if r['FishItemID'] == 'FishItem_027')
    if (dish['Type'], dish['ValueType'], dish['Value']) != ('dish', 'permanent4', 'pdef'):
        raise AssertionError('Original permanent dish differs')
    use = rows('ItemUse')
    if not any(r['itemId']=='item_067' and r['category']=='5' and r['odds']=='1' and r['result']=='hero_011,1' for r in use):
        raise AssertionError('Original067 choice differs')
    if not any(r['itemId']=='item_066' and r['category']=='2' and r['result']=='hero_exp_004,5' for r in use):
        raise AssertionError('Original066 fixed source differs')
    experience = {int(r['level']): int(r['exp']) for r in rows('HeroExp') if r['quality']=='20'}
    levels = []
    for amount in [20000,22000]:
        level, remain = 1, amount
        while remain > experience[level]:
            remain -= experience[level]
            level += 1
        levels.append((level, remain))
    if levels != [(12,2088),(12,4088)]:
        raise AssertionError('Original EXP vectors differ')
    return dict(requestVectors=old['requestVectors']+len(vectors),
                responseContainers=old['responseContainers']+len(contracts),
                opcodes={**old['opcodes'], **expected}, finitePlayerLevel=25,
                originalTrainingMinimumHeroLevel=10, originalCookingMinimumPlayerLevel=18,
                actualSourceBoxes=['item_067:choice1', 'item_066:fixed5EXPcards'],
                prerequisiteCookingStock='FishItem_027x3; no permanent warrior stats injected',
                realDurationOrBusinessClockModified=False)


def preflight():
    protocol_preflight()
    source = Path(os.environ['HYJS_WARRIOR_GROWN_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_WARRIOR_GROWN_BINARY']).resolve()
    validation = Path(os.environ['HYJS_WARRIOR_GROWN_VALIDATION']).resolve()
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
                  ['Hero', 'HeroExp', 'HeroTrainingMaxMin', 'GameConfigure', 'ItemUse', 'FishSkill']]]:
        if sha(source / rel) != sha(ROOT / rel):
            raise AssertionError('Original contract differs: ' + rel)
    integrated = (source / 'hyjs-server/service/p5InvestmentService.go').read_text()
    if 's.p5WarriorMaterialCosts(beforeP, beforeG, g, w, guid, request)' not in integrated:
        raise AssertionError('Require integrated warrior donor save')
    for relative in ['hyjs-server/service/p5WarriorMaterialService.go',
                     'hyjs-server/constUtil/p5WarriorMaterial.go']:
        if not (source / relative).is_file():
            raise AssertionError('Required warrior domain missing: ' + relative)
    helper = (source / 'hyjs-server/service/p5WarriorMaterialService.go').read_text()
    for token in ['trainingPending(material.GetTempTraining())', 'p5WarriorMaterialPristine',
                  's.p8CookingRefundCosts(costs)']:
        if token not in helper:
            raise AssertionError('Require complete grown-material helper: ' + token)
    protocol_preflight(source)
    return source, binary, sha(manifest)


@unittest.skipUnless(os.environ.get('HYJS_WARRIOR_GROWN_MYSQL_TEST') == '1',
                     'Root prebuilt managed database window')
class FunctionWarriorGrownDonorMySQLTests(unittest.TestCase):
    send, login, call = original.transport.send, original.transport.login, original.transport.call
    profile, stop, sql = original.transport.profile, original.transport.stop, original.transport.sql
    start, raw_profile = original.frames.start, original.frames.raw_profile
    # Original item use also emits SCPlayer; reuse the multi-frame transport.
    stamped = original.transport.stamped

    def test_confirmed_training_cooking_donors_merge_reset_once_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_WARRIOR_GROWN_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(DELIVERY))
        self.assertFalse(output.exists(), 'Keep previous attempts immutable')
        report = dict(status='PENDING', fullChain=False, runnerSHA256=sha(__file__),
                      binarySHA256=sha(binary), sourceManifestSHA256=manifest_sha,
                      finitePrerequisites='level25/100000coin/100diamond/item067x3/item066x1/item007x3/FishItem027x3',
                      naturalAcquisitionProven=False, productionModified=False,
                      injectedGUIDOrInvestment=False, phoneUI=False, phase='allocate')
        self.pool, self.process = ManagedMySQL(), None
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-warrior-grown-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('warrior_grown')
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
                    uid = 'demo-warrior-grown-' + suffix
                    owner = self.login(uid)
                    self.assertGreater(owner.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '伙伴材料', 'heroId': 'hero_047'},
                              owner, 934001, 'SCPlayerCreate')
                    # A finite legal prerequisite, with no donor state or ledger fabricated.
                    self.pool.sql(self.database,
                        f"UPDATE profiles SET player=JSON_SET(player,'$.level',25,'$.coin',100000,'$.diamond',100),"
                        "game=JSON_SET(game,'$.items',JSON_ARRAY(JSON_OBJECT('id','item_067','num',3),"
                        "JSON_OBJECT('id','item_066','num',1),JSON_OBJECT('id','item_007','num',3)),"
                        "'$.cooking',JSON_ARRAY(JSON_OBJECT('id','FishItem_027','num',3))) "
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

                    report['phase'] = 'original-boxes-and-first-level-growth'
                    gifts = [http('CSItemUse', {'costItem': {'id': 'item_067', 'num': 1},
                                               'getItemIndex': 1}, 'SCItemUse') for _ in range(3)]
                    for gift in gifts:
                        self.assertEqual([(w.id,w.level,w.exp) for w in gift.get.wars], [('hero_011',1,0)])
                        self.assertEqual((gift.cost.id,gift.cost.num), ('item_067',1))
                    donor, other, target = [gift.get.wars[0] for gift in gifts]
                    parcel = http('CSItemUse', {'costItem': {'id': 'item_066', 'num': 1}}, 'SCItemUse')
                    self.assertEqual((parcel.cost.id,parcel.cost.num), ('item_066',1))
                    self.assertEqual([(w.id,w.level,w.exp) for w in parcel.get.wars], [('hero_exp_004',1,0)]*5)
                    exp_card = parcel.get.wars[0]
                    spare_experience = {w.guid for w in parcel.get.wars[1:]}
                    old_guids = {donor.guid, other.guid, target.guid, exp_card.guid}
                    self.assertEqual(len(old_guids | spare_experience), 8)
                    first = {'guid': donor.guid, 'cost': [{'id': exp_card.id, 'guid': exp_card.guid}]}
                    fed = http('CSWarriorStrengthen', first, 'SCWarriorStrengthen')
                    first_stamp = stamp
                    self.assertEqual((fed.guid.guid,fed.guid.level,fed.guid.exp), (donor.guid,12,2088))
                    self.assertEqual({(c.id,c.guid):c.num for c in fed.cost},
                                     {('hero_exp_004',exp_card.guid):1,('coin',''):20000})
                    self.assertEqual(self.profile(owner)[0]['coin'], 80000)

                    report['phase'] = 'real-training-pending-confirmation'
                    training = {'guid': donor.guid, 'costType': 3, 'numType': 1}
                    preview = http('CSWarriorTraining', training, 'SCWarriorTraining')
                    self.assertEqual({c.id:c.num for c in preview.cost}, {'item_007':3,'diamond':5})
                    self.assertEqual(preview.guid.tempTraining.state, 1)
                    deltas = [getattr(preview.guid.tempTraining,k) for k in ['hp','att','pdef','mdef']]
                    self.assertTrue(all(v > 0 for v in deltas))
                    consume = {'guid': target.guid,
                               'cost': [{'id':donor.id,'guid':donor.guid}, {'id':other.id,'guid':other.guid}]}
                    reject('CSWarriorStrengthen', consume)
                    confirm = {'guid': donor.guid, 'result': 1}
                    trained = http('CSWarriorTrainingReplace', confirm, 'SCWarriorTrainingReplace')
                    confirm_stamp = stamp
                    self.assertFalse(trained.cost)
                    self.assertEqual(trained.result.guid, donor.guid)
                    self.assertEqual(trained.result.training.state, 0)
                    self.assertEqual([getattr(trained.result.training,k) for k in ['hp','att','pdef','mdef']], deltas)
                    self.assertEqual(trained.result.tempTraining.state, 0)
                    self.assertEqual((self.profile(owner)[0]['coin'],self.profile(owner)[0]['diamond']), (80000,95))

                    report['phase'] = 'three-real-permanent-dishes-and-multi-donor-consumption'
                    last_food = None
                    for unit in [target, donor, other]:
                        last_food = {'heroGuid':unit.guid, 'costItem':{'id':'FishItem_027','num':1}}
                        food_result = http('CSFishUsing', last_food, 'SCFishUsing')
                        self.assertEqual([(c.id,c.num) for c in food_result.cost], [('FishItem_027',1)])
                        self.assertEqual(food_result.warrior.guid, unit.guid)
                        self.assertEqual(food_result.warrior.cooking.prop.pdef, 1)
                    food_stamp = stamp
                    result, consume_stamp = once('CSWarriorStrengthen', consume, 'SCWarriorStrengthen')
                    self.assertEqual((result.guid.guid,result.guid.level,result.guid.exp), (target.guid,12,4088))
                    self.assertEqual({(c.id,c.guid):c.num for c in result.cost},
                                     {('hero_011',donor.guid):1,('hero_011',other.guid):1,('coin',''):22000})
                    # Only original level/EXP is inherited; donor passive stats are not copied.
                    self.assertEqual(result.guid.cooking.prop.pdef, 1)
                    self.assertTrue(all(getattr(result.guid.training,k)==0 for k in ['hp','att','pdef','mdef']))
                    p, game = self.profile(owner)
                    self.assertEqual((p['coin'],p['diamond']), (58000,95))
                    self.assertFalse(any(v['id']=='FishItem_027' and v.get('num',0) for v in game.get('cooking', [])))
                    self.assertFalse({donor.guid,other.guid,exp_card.guid} & {w['guid'] for w in game['warriors']})
                    kv = {v['key']:v.get('value','') for v in game.get('kv',[])}
                    investment = json.loads(kv['p5.invest:'+target.guid])
                    self.assertEqual({v['id']:v['num'] for v in investment['costs']},
                        {'coin':42000,'diamond':5,'hero_011':2,'hero_exp_004':1,'item_007':3,'FishItem_027':3})
                    for unit in [donor,other]:
                        self.assertFalse(kv.get('p5.invest:'+unit.guid))
                        audit = json.loads(kv['p5.warrior-material:'+unit.guid])
                        self.assertEqual(audit['targetGuid'], target.guid)
                        self.assertEqual(audit['before']['guid'], unit.guid)
                    reject('CSWarriorStrengthen', first, first_stamp)
                    reject('CSWarriorTrainingReplace', confirm, confirm_stamp)
                    reject('CSFishUsing', last_food, food_stamp)
                    saved = self.profile(owner)
                    self.stop(); self.start(); owner = self.login(uid)
                    self.assertEqual(self.profile(owner), saved)
                    self.assertEqual(self.call('CSWarriorStrengthen',consume,owner,consume_stamp,'SCWarriorStrengthen'),result)
                    self.assertEqual(self.profile(owner), saved)

                    report['phase'] = 'one-final-aggregate-rounddown-and-reset'
                    reset_fields = {'guid':target.guid}
                    reset, reset_stamp = once('CSWarriorReset',reset_fields,'SCWarriorReset')
                    self.assertEqual([(c.id,c.num) for c in reset.cost], [('diamond',50)])
                    self.assertEqual({r.id:r.num for r in reset.get.simps},
                                     {'coin':42000,'diamond':5,'item_007':3,'FishItem_027':1})
                    targets = [w for w in reset.warriors if w.guid==target.guid]
                    self.assertEqual(len(targets),1)
                    self.assertEqual((targets[0].id,targets[0].level,targets[0].exp),('hero_011',1,0))
                    self.assertFalse(targets[0].HasField('cooking'))
                    # Exclude the target body before counting refunded materials.
                    returned = [w for w in reset.get.wars if w.guid!=target.guid]
                    self.assertEqual(sorted(w.id for w in returned),['hero_011','hero_011','hero_exp_004'])
                    self.assertTrue(all(w.level==1 and w.exp==0 for w in returned))
                    returned_guids = {w.guid for w in returned}
                    self.assertEqual(len(returned_guids),3)
                    self.assertFalse(returned_guids & (old_guids | spare_experience))
                    p, game = self.profile(owner)
                    self.assertEqual((p['coin'],p['diamond']),(100000,50))
                    self.assertEqual(sum(v['num'] for v in game['cooking'] if v['id']=='FishItem_027'),1)
                    self.assertEqual(sum(v['num'] for v in game['items'] if v['id']=='item_007'),3)
                    warrior_guids = {w['guid'] for w in game['warriors']}
                    self.assertTrue(spare_experience <= warrior_guids)
                    self.assertFalse({donor.guid,other.guid,exp_card.guid} & warrior_guids)
                    self.assertEqual({w['guid'] for w in game['warriors'] if w['id']=='hero_011'},
                                     {target.guid} | {w.guid for w in returned if w.id=='hero_011'})
                    self.assertEqual({w['guid'] for w in game['warriors'] if w['id']=='hero_exp_004'},
                                     spare_experience | {w.guid for w in returned if w.id=='hero_exp_004'})
                    kv = {v['key']:v.get('value','') for v in game.get('kv',[])}
                    self.assertFalse(kv.get('p5.invest:'+target.guid))
                    reject('CSWarriorStrengthen',consume,consume_stamp)
                    saved = self.profile(owner)
                    self.stop(); self.start(); owner = self.login(uid)
                    self.assertEqual(self.profile(owner),saved)
                    self.assertEqual(self.call('CSWarriorReset',reset_fields,owner,reset_stamp,'SCWarriorReset'),reset)
                    self.assertEqual(self.profile(owner),saved)
                    self.assertEqual(self.raw_profile(protected),protected_before)
                    report.update(status='PASS',fullChain=True,actualInitialFeedCoin=20000,
                        actualConsumerEXP=22000,actualConsumerCoin=22000,actualTrainingDiamond=5,
                        realTrainingConfirmed=True,passiveDonorStatsNotInherited=True,
                        transferredCoin=42000,transferredDiamond=5,mergedDishUnits=3,finalDishRefund=1,
                        refundCoin=42000,refundDiamond=5,refundIDs=sorted(w.id for w in returned),
                        eightSameStampOnce=True,coldExact=True,staleDonorAndTargetReceiptsRejected=True,
                        protected1Exact=True)
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
