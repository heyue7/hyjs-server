"""Prebuilt original 82027 five-strengthening HTTP transaction contract.

Finite owned pet047 instances, current levels and coin; no acquisition/source
or phone proof. Root executes against a qualified fixed candidate and managed
allocation. This module only imports transport; no compiler or legacy SQL setup.
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

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from server.codec import CLASSES, BY_NAME, parse_http
import test_continued_gameplay_mysql as wire
import test_local_wallet_mysql as app_transport

ROOT = Path(__file__).resolve().parents[1]


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


TABLES = ['Pet', 'PetTalent', 'PetTalentSkill', 'PetEquip', 'PetEquipStrength', 'PetEquipAdvance']
DELIVERY = ROOT / 'analysis/growth-first-20261016/function-pet-equip-five-r1-20261017'
POLICY = 'local-pet-equipment-five-bounded-v1'


def rows(source, name):
    lines = (source / f'analysis/tables/Table{name}.tsv').read_text().splitlines()
    return [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line]


def finite_pet(source, guid, level, strength):
    row = next(r for r in rows(source, 'Pet') if r['id'] == 'pet_047')
    if row['quality'] != '23':
        raise AssertionError('Original red pet changed')
    talents = [r for r in rows(source, 'PetTalentSkill') if r['petId'] == row['id']]
    if any(r['odd'] != '100' for r in talents):
        raise AssertionError('Fixture must use deterministic original talent rows')
    equips = [r for r in rows(source, 'PetEquip') if r['petId'] == row['id']]
    if len(equips) != 4 or {r['position'] for r in equips} != {'1','2','3','4'}:
        raise AssertionError('Original red four slots differ')
    return CLASSES['Pet'](id=row['id'], guid=guid, level=level, exp=0,
        breached={'level': 0}, eat={'level': 0, 'exp': 0, 'hdTimeList': [0,0,0]},
        initProp=row['defaultTalent'], talentList=[{'talent': r['talent'], 'talentLevel': int(r['talentLevel'])} for r in talents],
        petEquipList=[{'petGuid': guid, 'equipId': r['id'], 'strengthLevel': strength if i == 0 else 0,
                      'advanceLevel': 0, 'awakeLevel': 0} for i,r in enumerate(equips)])


def fees(source, equip, first, last):
    table = {(r['equipId'], int(r['level'])): r for r in rows(source, 'PetEquipStrength')}
    result = []
    for n in range(first, last+1):
        resource, amount = table[equip,n]['cost'].split(',')
        if resource != 'coin' or int(amount) <= 0:
            raise AssertionError('Expected original positive coin row')
        result.append((resource, int(amount)))
    return result


def protocol_preflight(source=ROOT):
    pet = finite_pet(source, '00000000-0000-4000-8000-000000000001', 20, 8)
    fields = {'petGuid': pet.guid, 'equipId': pet.petEquipList[0].equipId, 'num': 5}
    for name, values in [('CSPetEquipStrengthOne', fields), ('CSGetPetEquipInfo', {'petGuid': [pet.guid]}), ('CSPetList', {})]:
        packet = parse_http(wire.encode_original_http(name, values, 2, b'0'*16, 926001))
        if packet.opcode != BY_NAME[name] or packet.decode() != CLASSES[name](**values):
            raise AssertionError('Original request roundtrip: ' + name)
    if (BY_NAME['CSPetEquipStrengthOne'], BY_NAME['SCPetEquipStrengthOne']) != (82027,82028):
        raise AssertionError('Original equipment opcode differs')
    if set(CLASSES['SCPetEquipStrengthOne'].DESCRIPTOR.fields_by_name) != {'petEquip', 'cost'}:
        raise AssertionError('Original consumer fields differ')
    return dict(requestOpcode=82027, responseOpcode=82028, localBoundaryPolicy=POLICY,
                petIDs=['pet_047'], finitePetCount=2, petLevels=[20,9], currentStrength=[8,6],
                advanceCapFee=fees(source, pet.petEquipList[0].equipId,9,10),
                petCapFee=fees(source, pet.petEquipList[0].equipId,7,9),
                naturalAcquisitionProven=False, phoneUI=False)


def source_contract(source):
    service = (source / 'hyjs-server/service/p9PetEquipService.go').read_text()
    for token in ['s.p9EquipFiveSteps(pet, equip)', 'step <= steps', 's.growthCost(',
                  's.p9NextMutation(', 'p9SavePet(', 'p9Invest(', 's.p9RecordCosts(',
                  '"pet-equip-strength"', '"p9:equip-strength:"', 'steps < b.GetNum()', 'P9PetEquipFiveSource']:
        if token not in service:
            raise AssertionError('Integrated original transaction missing: ' + token)
    helper = (source / 'hyjs-server/service/p9PetEquipBatchService.go').read_text()
    if 'p9EquipFiveSteps' not in helper or '+5 > int64(maximum)' not in helper:
        raise AssertionError('Native total maximum gate missing')
    if POLICY not in (source / 'hyjs-server/constUtil/p9PetEquipBatch.go').read_text():
        raise AssertionError('Local source policy missing')
    return protocol_preflight(source)


def preflight():
    source = Path(os.environ.get('HYJS_PET_EQUIP_FIVE_SOURCE', ROOT / 'build/candidates/function-pet-equip-five-r1-20261017')).resolve()
    binary = Path(os.environ.get('HYJS_PET_EQUIP_FIVE_BINARY', ROOT / 'build/hyjs-server-function-pet-equip-five-r1-20261017-pending')).resolve()
    validation = Path(os.environ.get('HYJS_PET_EQUIP_FIVE_VALIDATION', ROOT / 'analysis/p3/function-pet-equip-five-r1-incremental-20261017.json')).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use a direct fixed candidate')
    manifest = source / 'manifest.json'
    paths = set()
    for entry in json.loads(manifest.read_bytes())['files']:
        path = (source / entry['path']).resolve()
        if not path.is_relative_to(source) or path in paths or sha(path) != entry['sha256']:
            raise AssertionError('Fixed candidate pin differs: ' + entry['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Unpinned Go source')
    proof = json.loads(validation.read_bytes())
    if proof.get('qualified') is not True or (ROOT / proof['candidate']).resolve() != source:
        raise AssertionError('Exact completed qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use the exact prebuilt qualified app')
    for rel in ['server/codec.py', 'server/protocol/schema.json', *[f'analysis/tables/Table{name}.tsv' for name in TABLES]]:
        if sha(source / rel) != sha(ROOT / rel):
            raise AssertionError('Original protocol/table differs: ' + rel)
    return source, binary, sha(manifest), source_contract(source)


@unittest.skipUnless(os.environ.get('HYJS_PET_EQUIP_FIVE_MYSQL_TEST') == '1', 'Root prebuilt managed window')
class FunctionPetEquipFiveMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    profile, stop, sql = transport.profile, transport.stop, transport.sql

    def start(self):
        app_transport.SOURCE_ROOT = self.source
        app_transport.LocalWalletMySQLTests.start(self, wallet=False)

    def stamped(self, name, fields, owner, stamp):
        raw = wire.encode_original_http(name, fields, owner.playerId, bytes(owner.sessionKey), stamp)
        connection = http_client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=40)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            self.assertEqual(response.status, 200)
            frames = wire.decode_responses(response.read())
            self.assertEqual(len(frames), 1, 'One original response frame')
            return frames[0]
        finally:
            connection.close()

    def raw_profile(self, owner):
        return self.sql(f'SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id={owner.playerId};').strip()

    def snapshot(self, owner):
        p = owner.playerId
        queries = [f'SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id={p}',
            f'SELECT guid,HEX(data),source FROM p9_pet_instances WHERE player_id={p} ORDER BY guid',
            f'SELECT guid,HEX(costs),HEX(donors),source FROM p9_pet_investments WHERE player_id={p} ORDER BY guid',
            f'SELECT operation,timestamp,HEX(request),HEX(response) FROM p9_pet_receipts WHERE player_id={p} ORDER BY operation,timestamp',
            f'SELECT source_key,kind,amount,occurred_at,day FROM p1_event_facts WHERE player_id={p} ORDER BY source_key,kind']
        return [self.sql(q+';').strip() for q in queries]

    def coin(self, owner):
        return int(self.profile(owner)[0].get('coin', 0))

    def set_coin(self, owner, value):
        self.assertGreater(owner.playerId, 1)
        self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.coin',{value}) WHERE id={owner.playerId};")

    def verify_charge(self, owner, reply, pet, index, first, last, before, stamp):
        equip = pet.petEquipList[index]
        expected = fees(self.source, equip.equipId, first, last)
        self.assertEqual([(v.id,v.num) for v in reply.cost], expected)
        target = CLASSES['PetEquip']()
        target.CopyFrom(equip)
        target.strengthLevel = last
        self.assertEqual(reply.petEquip, target)
        total = sum(n for _,n in expected)
        self.assertEqual(before-self.coin(owner), total)
        fact = self.sql(f"SELECT amount FROM p1_event_facts WHERE player_id={owner.playerId} AND source_key='p9:equip-strength:{stamp}' AND kind='coin_spent';").strip()
        self.assertEqual(fact, str(total))
        data = self.sql(f"SELECT HEX(data) FROM p9_pet_instances WHERE player_id={owner.playerId} AND guid='{pet.guid}';").strip()
        stored = CLASSES['Pet'].FromString(bytes.fromhex(data))
        self.assertEqual(stored.petEquipList[index], target)
        receipt = self.sql(f"SELECT HEX(response) FROM p9_pet_receipts WHERE player_id={owner.playerId} AND operation='pet-equip-strength' AND timestamp={stamp};").strip()
        self.assertEqual(CLASSES['SCPetEquipStrengthOne'].FromString(bytes.fromhex(receipt)), reply)
        return total

    def test_original_five_boundaries_retry_cold_and_rollback(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_PET_EQUIP_FIVE_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(DELIVERY))
        self.assertFalse(output.exists(), 'Keep previous attempt evidence')
        report = dict(status='PENDING', fullChain=False, runnerSHA256=sha(__file__), binarySHA256=sha(binary),
                      sourceManifestSHA256=manifest_sha, contract=contract, productionModified=False,
                      finiteFixture=True, naturalAcquisitionProven=False, phoneUI=False, phase='allocation')
        self.pool, self.process = ManagedMySQL(), None
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-pet-five-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('pet_equip_five')
                    self.database = allocation.database
                    self.env = {k:v for k,v in os.environ.items() if k in {'PATH','HOME','TMPDIR','LANG','LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-petfive-protected-'+suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName':'五连保护','heroId':'hero_043'}, protected, 926000, 'SCPlayerCreate')
                    protected_before = self.snapshot(protected)
                    uid = 'demo-petfive-'+suffix
                    owner = self.login(uid)
                    self.assertGreater(owner.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName':'五连验证','heroId':'hero_047'}, owner, 926001, 'SCPlayerCreate')
                    # Explicit finite fixtures: no fabricated natural acquisition/investment.
                    report['phase'] = 'finite-owned-original-pet-fixture'
                    a = finite_pet(self.source, str(uuid.uuid4()), 20, 8)
                    b = finite_pet(self.source, str(uuid.uuid4()), 9, 6)
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',34,'$.coin',10000000) WHERE id={owner.playerId};")
                    for pet in [a,b]:
                        self.pool.sql(self.database, f"INSERT INTO p9_pet_instances(guid,player_id,pet_id,data,created_at,source) VALUES ('{pet.guid}',{owner.playerId},'pet_047',UNHEX('{pet.SerializeToString().hex()}'),{int(time.time()*1000)},'finite-pet-five-http-v1');")
                    listing = self.call('CSPetList', {}, owner, 926002, 'SCPetList')
                    self.assertEqual({p.guid:p for p in listing.petList}, {a.guid:a,b.guid:b})
                    info = self.call('CSGetPetEquipInfo', {'petGuid':[a.guid,b.guid]}, owner, 926003, 'SCGetPetEquipInfo')
                    self.assertEqual(list(info.petEquip), list(a.petEquipList)+list(b.petEquipList))
                    request = lambda pet,index,num=5: dict(petGuid=pet.guid,equipId=pet.petEquipList[index].equipId,num=num)
                    report['phase'] = 'advance-cap-eight-same-stamp'
                    before = self.coin(owner)
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        results = list(workers.map(lambda _: self.call('CSPetEquipStrengthOne', request(a,0), owner, 926100, 'SCPetEquipStrengthOne'), range(8)))
                    self.assertTrue(all(v == results[0] for v in results))
                    advance_fee = self.verify_charge(owner, results[0], a, 0, 9, 10, before, 926100)
                    report['phase'] = 'pet-level-cap'
                    before = self.coin(owner)
                    pet_reply = self.call('CSPetEquipStrengthOne', request(b,0), owner, 926101, 'SCPetEquipStrengthOne')
                    pet_fee = self.verify_charge(owner, pet_reply, b, 0, 7, 9, before, 926101)
                    for pet,expected in [(a,advance_fee),(b,pet_fee)]:
                        raw = self.sql(f"SELECT HEX(costs),source FROM p9_pet_investments WHERE player_id={owner.playerId} AND guid='{pet.guid}';").strip().split('\t')
                        self.assertEqual(json.loads(bytes.fromhex(raw[0])), {'coin':expected})
                        self.assertIn(POLICY,raw[1])
                    report['phase'] = 'normal-five-and-single'
                    before = self.coin(owner)
                    normal = self.call('CSPetEquipStrengthOne', request(a,1), owner, 926102, 'SCPetEquipStrengthOne')
                    self.verify_charge(owner, normal, a, 1, 1, 5, before, 926102)
                    before = self.coin(owner)
                    single = self.call('CSPetEquipStrengthOne', request(a,1,1), owner, 926103, 'SCPetEquipStrengthOne')
                    self.verify_charge(owner, single, a, 1, 6, 6, before, 926103)
                    frozen = self.snapshot(owner)
                    self.call('CSPetEquipStrengthOne', request(a,1), owner, 926102, 'ErrorMessage')
                    for pet in [a,b]:
                        self.call('CSPetEquipStrengthOne', request(pet,0), owner, 926104, 'ErrorMessage')
                    self.call('CSPetEquipStrengthOne', request(a,0), protected, 926105, 'ErrorMessage')
                    self.assertEqual(self.snapshot(owner), frozen)
                    self.assertEqual(self.snapshot(protected), protected_before)
                    report['phase'] = 'cold-current-receipts-exact'
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.snapshot(owner), frozen)
                    self.assertEqual(self.call('CSPetEquipStrengthOne',request(a,0),owner,926100,'SCPetEquipStrengthOne'), results[0])
                    self.assertEqual(self.call('CSPetEquipStrengthOne',request(b,0),owner,926101,'SCPetEquipStrengthOne'), pet_reply)
                    self.assertEqual(self.snapshot(owner), frozen)
                    report['phase'] = 'insufficient-final-coin-atomic-rollback'
                    self.set_coin(owner, sum(n for _,n in fees(self.source,a.petEquipList[2].equipId,1,5))-1)
                    failed_before = self.snapshot(owner)
                    self.call('CSPetEquipStrengthOne',request(a,2),owner,926200,'ErrorMessage')
                    self.assertEqual(self.snapshot(owner),failed_before)
                    self.assertEqual(self.snapshot(protected), protected_before)
                    report.update(status='PASS', fullChain=True, scope='finite equipment five-step HTTP transaction only',
                        advanceBoundary=[8,10], petBoundary=[6,9], advanceBoundaryCoin=advance_fee, petBoundaryCoin=pet_fee,
                        normalFiveAndSingle=True, exactOriginalFeeRows=True, eightSameStampOnce=True,
                        coldReceiptsExact=True, staleReceiptRejected=True, currentCapRejected=True,
                        insufficientAtomicRollback=True, protected1Exact=True)
                except BaseException as error:
                    report.update(status='FAIL', failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    try:
                        self.stop()
                    except BaseException as error:
                        report.update(status='FAIL', fullChain=False, cleanupFailure='owned-app-stop', cleanupError=str(error))
                        raise
                    finally:
                        try:
                            self.pool.close()
                        except BaseException as error:
                            report.update(status='FAIL', fullChain=False, cleanupFailure='managed-allocation-release', cleanupError=str(error))
                            raise
                        finally:
                            report['seconds'] = round(time.monotonic() - began, 3)
                            report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                            if self.pool.connections:
                                report.update(status='FAIL', fullChain=False, cleanupFailure=True)
                            output.parent.mkdir(parents=True, exist_ok=True)
                            output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
