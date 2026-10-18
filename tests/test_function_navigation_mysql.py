"""Short prebuilt original navigation -> real027 craft -> hint refresh/cold.

Finite isolated eligibility/inventory only: Player OutdoorLevel, 10000 coin and
five each Fish007/Fish013. These are navigation fixtures, not natural sources.
No payment, battle End, reward creation, Go compiler or device operation.
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

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from server.codec import CLASSES, BY_NAME, parse_http
import test_continued_gameplay_mysql as wire
import test_local_wallet_mysql as app_transport

ROOT = Path(__file__).resolve().parents[1]


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def rows(source, name):
    lines = (source / 'analysis/tables' / ('Table' + name + '.tsv')).read_text().splitlines()
    return [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line]


def protocol_preflight():
    vectors = {'CSNavigationItem': {},
               'CSFishCooking': {'fish': {'id': 'FishItem_027', 'num': 1}}}
    for name, fields in vectors.items():
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 901001)
        packet = parse_http(raw)
        if packet.opcode != BY_NAME[name] or packet.decode() != CLASSES[name](**fields):
            raise AssertionError('Original request roundtrip differs: ' + name)
    if (BY_NAME['CSNavigationItem'], BY_NAME['SCNavigationItem'], BY_NAME['CSFishCooking']) != (20125, 20126, 24713):
        raise AssertionError('Original opcodes differ')
    required = {'SCNavigationItem': {'navItems'}, 'NavigationItem': {'type', 'value'},
                'SCFishCooking': {'level', 'exp', 'add', 'res', 'cost'},
                'ResultObject': {'simps'}, 'BaseResource': {'id', 'num'}}
    for name, fields in required.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original response differs: ' + name)
    return {'requestVectors': len(vectors), 'responseContracts': len(required),
            'requestOpcode': 20125, 'responseOpcode': 20126,
            'clientOwnedTypes': [11, 12, 19, 20, 21], 'deferredWantedTypes': [5, 6]}


def preflight():
    source = Path(os.environ.get('HYJS_NAVIGATION_SOURCE', ROOT / 'build/candidates/growth-function-navigation-r2-20261017')).resolve()
    binary = Path(os.environ.get('HYJS_NAVIGATION_BINARY', ROOT / 'build/hyjs-server-growth-function-navigation-r2-20261017-pending')).resolve()
    validation = Path(os.environ.get('HYJS_NAVIGATION_VALIDATION', ROOT / 'analysis/p3/growth-function-navigation-r2-incremental-20261017.json')).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use a direct fixed candidate')
    manifest = source / 'manifest.json'
    paths = set()
    for entry in json.loads(manifest.read_bytes())['files']:
        path = (source / entry['path']).resolve()
        if not path.is_relative_to(source) or path in paths or sha(path) != entry['sha256']:
            raise AssertionError('Frozen pin differs: ' + entry['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Unpinned Go source')
    proof = json.loads(validation.read_bytes())
    if proof.get('qualified') is not True or (ROOT / proof['candidate']).resolve() != source:
        raise AssertionError('Exact completed root qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use the exact prebuilt qualified binary')
    for rel in ['server/codec.py', 'server/protocol/schema.json'] + [
            'analysis/tables/Table' + name + '.tsv' for name in ['FishSkill', 'GameConfigure', 'NavigationItem']]:
        if sha(source / rel) != sha(ROOT / rel):
            raise AssertionError('Original frame/table differs: ' + rel)
    game = (source / 'hyjs-server/service/gameService.go').read_text()
    if 'case *pb.CSNavigationItem:' not in game or 's.navigationItems(ctx, row.Id)' not in game:
        raise AssertionError('Original dispatch is still an empty stub')
    main = (source / 'hyjs-server/main.go').read_text()
    for init in ['s.InitCooking(ctx)', 's.InitializeDailyCopy(ctx)', 's.InitializeP7(ctx)',
                 's.InitializeP7Expedition(ctx)', 's.InitializeP7Awake(ctx)', 's.InitializeP10Voice(ctx)',
                 's.InitializeP13Guild(ctx)', 's.P8LifeReady = true']:
        if init not in main:
            raise AssertionError('Normal startup dependency missing: ' + init)
    receipt = (source / 'hyjs-server/db/cookingMutationReceipt.go').read_text()
    for token in ['Timestamp int64', 'cooking_mutation_receipts']:
        if token not in receipt:
            raise AssertionError('Actual receipt model differs: ' + token)
    craft = (source / 'hyjs-server/service/cookingService.go').read_text()
    for token in ['func (s *GameService) fishCooking(', 's.cookingMutation(ctx, id, constUtil.CookingMakeOpcode', 's.cookingOpen(p)']:
        if token not in craft:
            raise AssertionError('Actual consumer differs: ' + token)
    dish = next(r for r in rows(source, 'FishSkill') if r['FishItemID'] == 'FishItem_027')
    if (dish['Type'], dish['IsDish'], dish['LevelLimit'], dish['Material'], dish.get('SkillNeeded', '')) != ('dish', '1', '1', 'FishItem_007,5;FishItem_013,5;coin,10000', ''):
        raise AssertionError('Actual original027 recipe differs')
    outdoor = int(next(r['NameValue'] for r in rows(source, 'GameConfigure') if r['StaticName'] == 'OutdoorLevel'))
    if outdoor < 1:
        raise AssertionError('Original life gate missing')
    contract = protocol_preflight()
    contract.update(finitePreparation={'playerLevel': outdoor, 'coin': 10000,
        'cooking': [{'id': 'FishItem_007', 'num': 5}, {'id': 'FishItem_013', 'num': 5}]},
        naturalSourceClaimed=False, originalRecipe=dish['Material'])
    return source, binary, sha(manifest), contract


@unittest.skipUnless(os.environ.get('HYJS_NAVIGATION_MYSQL_TEST') == '1', 'Root managed prebuilt window')
class FunctionNavigationMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    profile, stop, sql = transport.profile, transport.stop, transport.sql

    def start(self):
        # Use the candidate's normal startup and all real initializers; no feature
        # suppressions, custom schema subset or wallet/payment flags are supplied.
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
            self.assertEqual(len(frames), 1, 'Exact single original response required')
            return frames[0]
        finally:
            connection.close()

    def raw_profile(self, owner):
        return self.sql(f'SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id={owner.playerId};').strip()

    def hints(self, owner, stamp):
        out = self.call('CSNavigationItem', {}, owner, stamp, 'SCNavigationItem')
        kinds = [item.type for item in out.navItems]
        self.assertEqual(kinds, sorted(set(kinds)))
        self.assertTrue(all(item.value == 1 for item in out.navItems))
        self.assertFalse(set(kinds) & {11, 12, 19, 20, 21, 5, 6})
        return out, kinds

    def test_original_navigation_real_recipe_receipt_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_NAVIGATION_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/function-navigation-r1-20261017'))
        self.assertFalse(output.exists(), 'Keep prior attempt reports')
        report = dict(status='PENDING', fullChain=False, runnerSHA256=sha(__file__),
            binarySHA256=sha(binary), sourceManifestSHA256=manifest_sha, contract=contract,
            phoneUI=False, productionModified=False, externalPayment=False,
            naturalResourcesClaimed=False, finiteNavigationInventory=True, phase='allocation')
        self.pool, self.process = ManagedMySQL(), None
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-function-navigation-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('function_navigation')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-navigation-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '导航保护', 'heroId': 'hero_001'}, protected, 901000, 'SCPlayerCreate')
                    protected_before = self.raw_profile(protected)
                    uid = 'demo-navigation-' + suffix
                    owner = self.login(uid)
                    self.assertGreater(owner.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '导航料理', 'heroId': 'hero_047'}, owner, 901001, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    baseline, baseline_types = self.hints(owner, 901002)
                    self.assertNotIn(15, baseline_types)
                    finite = contract['finitePreparation']
                    cooking_hex = json.dumps(finite['cooking'], separators=(',', ':')).encode().hex()
                    self.pool.sql(self.database,
                        f"UPDATE profiles SET player=JSON_SET(player,'$.level',{finite['playerLevel']},'$.coin',10000), "
                        "game=JSON_SET(game,'$.cooking',CAST(CONVERT(0x" + cooking_hex + " USING utf8mb4) AS JSON)) "
                        f"WHERE id={owner.playerId} AND uid='" + uid + "';")
                    prepared = self.profile(owner)
                    self.assertEqual(prepared[0], dict(initial[0], level=finite['playerLevel'], coin=10000))
                    expected_game = json.loads(json.dumps(initial[1]))
                    expected_game['cooking'] = finite['cooking']
                    self.assertEqual(prepared[1], expected_game)
                    report['phase'] = 'query-actionable-original-recipe-without-side-effects'
                    before = self.raw_profile(owner)
                    first, first_types = self.hints(owner, 901003)
                    self.assertIn(15, first_types)
                    for stamp in [901003, 901004, 901005]:
                        self.assertEqual(self.hints(owner, stamp)[0], first)
                    self.assertEqual(self.raw_profile(owner), before)
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p6_battle_records WHERE player_id={owner.playerId};').strip(), '0')
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM cooking_mutation_receipts WHERE player_id={owner.playerId};').strip(), '0')
                    request = {'fish': {'id': 'FishItem_027', 'num': 1}}
                    craft_stamp = 901006
                    report['phase'] = 'real-original027-eight-retries-one-consumption'
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        results = list(workers.map(lambda _: self.call('CSFishCooking', request, owner, craft_stamp, 'SCFishCooking'), range(8)))
                    self.assertTrue(all(out == results[0] for out in results))
                    cooked = results[0]
                    self.assertEqual({r.id: r.num for r in cooked.cost}, {'FishItem_007': 5, 'FishItem_013': 5, 'coin': 10000})
                    self.assertEqual([(r.id, r.num) for r in cooked.res.simps], [('FishItem_027', 1)])
                    final = self.profile(owner)
                    self.assertEqual(final[0].get('coin', 0), 0)
                    for key in ['diamond', 'point', 'energy', 'stamina', 'exp', 'level']:
                        self.assertEqual(final[0].get(key, 0), prepared[0].get(key, 0), key)
                    food = {r['id']: r['num'] for r in final[1]['cooking']}
                    self.assertEqual(food.get('FishItem_007', 0), 0)
                    self.assertEqual(food.get('FishItem_013', 0), 0)
                    self.assertEqual(food.get('FishItem_027', 0), 1)
                    after_bytes = self.raw_profile(owner)
                    after, after_types = self.hints(owner, 901007)
                    self.assertNotIn(15, after_types)
                    self.assertEqual(self.raw_profile(owner), after_bytes)
                    receipt_where = f'player_id={owner.playerId} AND opcode=24713'
                    receipt = self.sql('SELECT CONCAT(timestamp,"|",HEX(request),"|",HEX(response)) FROM cooking_mutation_receipts WHERE ' + receipt_where + ';').strip()
                    self.assertTrue(receipt.startswith(str(craft_stamp) + '|'))
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM cooking_mutation_receipts WHERE ' + receipt_where + ';').strip(), '1')
                    self.assertEqual(self.call('CSFishCooking', request, owner, craft_stamp, 'SCFishCooking'), cooked)
                    self.assertEqual(self.raw_profile(owner), after_bytes)
                    report['phase'] = 'cold-original-navigation-and-persisted-once-receipt'
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.raw_profile(owner), after_bytes)
                    self.assertEqual(self.hints(owner, 901008)[0], after)
                    self.assertEqual(self.call('CSFishCooking', request, owner, craft_stamp, 'SCFishCooking'), cooked)
                    self.assertEqual(self.raw_profile(owner), after_bytes)
                    self.assertEqual(self.sql('SELECT CONCAT(timestamp,"|",HEX(request),"|",HEX(response)) FROM cooking_mutation_receipts WHERE ' + receipt_where + ';').strip(), receipt)
                    self.assertEqual(self.raw_profile(protected), protected_before)
                    engine = self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME IN ('profiles','cooking_mutation_receipts','cooking_craft_states');").strip()
                    self.assertEqual(engine, 'InnoDB')
                    report.update(status='PASS', fullChain=True, beforeTypes=first_types,
                        afterTypes=after_types, original20125to20126=True, queryProfileBytesUnchanged=True,
                        actualRecipe='FishItem_027', actualCoinCost=10000, actualDishCount=1,
                        eightConcurrentCraftOnce=True, receiptColdByteIdentical=True,
                        clientOwnedHintsNotDuplicated=True, deferredWantedNotFabricated=True,
                        coldNavigationExact=True, protected1Exact=True)
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
