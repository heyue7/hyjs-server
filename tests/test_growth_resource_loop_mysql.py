"""Single prebuilt growth batch: actual coins -> raw materials -> dish effect.

Only level18 and historical chapter stars are bounded qualifications in this
isolated account. No wallet, fish, ore, gem, partner, or dish is seeded. No
compiler, phone, remote DB, or live-account mutation occurs in this runner.
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
import unittest
import types
import uuid

from hyjs_managed_mysql import ManagedMySQL
import test_p7_maze_trader_mysql as transport
import test_continued_gameplay_mysql as wire
import test_local_wallet_mysql as wallet_transport
from test_p5_talent_scope_mysql import original_table
from server.codec import message

ROOT = Path(__file__).resolve().parents[1]
PREFIX = 'limitExchange_p5_growth_materials_'


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def protocol_preflight():
    # Validate the actual request and response contracts before an SQL lease.
    requests = {
        'CSFishCooking': {'fish': {'id': 'FishItem_027', 'num': 1}},
        'CSFishUsing': {'heroGuid': 'actual-guid', 'costItem': {'id': 'FishItem_027', 'num': 1}},
        'CSMineRefine': {'mineId': 'MineItem_007', 'num': 100},
        'CSGemMake': {'gemId': 'MineItem_019', 'num': 1},
        'CSBuyActivityShop': {'activityId': 'limitExchange_p4_catalog_lp-' + '0' * 28,
                              'index': 1, 'num': 1, 'costResources': [{'id': 'coin', 'num': 50000}],
                              'getResources': [{'id': 'MineItem_007', 'num': 5}]},
        'CSActivityList': {}, 'CSActivityShopInfo': {'activityId': 'preflight'},
        'CSWarriorRecruit': {'costType': 1, 'numType': 1, 'free': 1},
        'CSCopyGetStarAward': {'chapterId': '1', 'starIndex': 0},
    }
    from server.codec import parse_http
    for name, fields in requests.items():
        encoded = wire.encode_original_http(name, fields, 2, b'0' * 16, 100)
        if parse_http(encoded).decode() != message(name).__class__(**fields):
            raise AssertionError('Original request round trip changed: ' + name)
    responses = {
        'SCActivityList': {'activityList'}, 'ActivityTab': {'activityId'},
        'SCActivityShopInfo': {'shopItemList'},
        'ActivityShopItem': {'index', 'costResource', 'getResource'},
        'SCGetActivityShopReward': {'costResources', 'get'}, 'SCWarriorRecruit': {'get'},
        'SCFishCooking': {'cost', 'res'}, 'SCFishUsing': {'cost', 'warrior'},
        'SCMineRefine': {'cost', 'res'}, 'SCGemMake': {'cost', 'res'},
        'SCCopyGetStarAward': {'gain'},
    }
    for name, fields in responses.items():
        if not fields <= set(message(name).DESCRIPTOR.fields_by_name):
            raise AssertionError('Original response contract changed: ' + name)
    return {'originalRequestVectors': len(requests), 'responseContracts': len(responses)}


def preflight():
    """Finish source/binary/protocol checks before reserving any SQL resource."""
    protocol_preflight()
    source = Path(os.environ['HYJS_GROWTH_RESOURCE_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_GROWTH_RESOURCE_BINARY']).resolve()
    validation = Path(os.environ['HYJS_GROWTH_RESOURCE_VALIDATION']).resolve()
    manifest = Path(os.environ.get('HYJS_GROWTH_RESOURCE_MANIFEST', source / 'manifest.json')).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use the fixed root growth candidate')
    fixed = json.loads(manifest.read_text())
    files = fixed['files']
    paths = set()
    for entry in files:
        path = (source / entry['path']).resolve()
        if source not in path.parents or path in paths or sha(path) != entry['sha256']:
            raise AssertionError('Fixed growth candidate source mismatch')
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Growth candidate has unpinned Go code')
    built = json.loads(validation.read_text())
    if built.get('qualified') is not True:
        raise AssertionError('Root qualification record must pass before SQL work')
    if Path(built.get('candidate', '')).resolve() != source:
        raise AssertionError('Qualification source must be this exact frozen candidate')
    for table in ['FishSkill', 'MineSkill', 'CopyMission']:
        if sha(source / 'analysis/tables' / ('Table' + table + '.tsv')) != sha(ROOT / 'analysis/tables' / ('Table' + table + '.tsv')):
            raise AssertionError('Original consumer table differs from fixed runtime input')
    if sha(binary) != (built.get('binarySHA256') or built.get('hyjs-serverSHA256')):
        raise AssertionError('Reuse the root verified binary without recompiling')
    if 'seedP5GrowthResourceSupply(tx, now)' not in (source / 'hyjs-server/service/p4CommerceService.go').read_text():
        raise AssertionError('Missing production seed integration')
    if 'p5AppendGrowthResourceBindings(tx, row, &bindings, now)' not in (source / 'hyjs-server/service/p4ResourceExchangeService.go').read_text():
        raise AssertionError('Missing stable aggregate exchange integration')
    return source, binary, sha(manifest)


@unittest.skipUnless(os.environ.get('HYJS_GROWTH_RESOURCE_MYSQL_TEST') == '1', 'Root coordinated managed SQL window')
class GrowthResourceLoopMySQLTests(unittest.TestCase):
    send = transport.P7MazeTraderMySQLTests.send
    login = transport.P7MazeTraderMySQLTests.login
    stamped = wire.ContinuedGameplayMySQLTests.stamped
    call = transport.P7MazeTraderMySQLTests.call
    profile = transport.P7MazeTraderMySQLTests.profile
    json_request = wallet_transport.LocalWalletMySQLTests.json_request

    def start(self):
        wallet_transport.SOURCE_ROOT = self.source
        wallet_transport.LocalWalletMySQLTests.start(self, wallet=True)

    def activity(self, owner, action, request_id=None):
        status, reply = self.json_request('/local-play/activities/apply', {
            'playerId': owner.playerId, 'sessionHex': bytes(owner.sessionKey).hex(),
            'activityId': 'TimeLimitExchange', 'action': action,
            'requestId': request_id or str(uuid.uuid4())})
        self.assertEqual(status, 200)
        self.assertEqual(reply.get('code'), 0, reply.get('msg'))
        return reply['data']

    def directory(self, owner):
        value = self.sql('SELECT HEX(c.snapshot) FROM local_play_activity_cycles c '
                         'JOIN local_play_activity_bindings b ON b.cycle_id=c.id '
                         f"WHERE b.player_id={int(owner.playerId)} AND b.activity_id='TimeLimitExchange';").strip()
        key = json.loads(bytes.fromhex(value))['exchange']['id']
        quoted = key.replace("'", "''")
        frozen = self.sql(f"SELECT input_hash,HEX(bindings) FROM p4_resource_exchange_directories WHERE id='{quoted}';").strip()
        digest, raw = frozen.split('\t')
        bindings = json.loads(bytes.fromhex(raw))
        self.assertEqual(hashlib.sha256(bytes.fromhex(raw)).hexdigest(), digest)
        return key, digest, raw, bindings
    stop = transport.P7MazeTraderMySQLTests.stop
    setUp = transport.P7MazeTraderMySQLTests.setUp
    sql = transport.P7MazeTraderMySQLTests.sql

    def test_paid_raw_materials_original_cooking_effect_refining_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, self.binary, source_sha = preflight()
        output = Path(os.environ['HYJS_GROWTH_RESOURCE_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/resources'))
        self.assertFalse(output.exists(), 'Preserve earlier acceptance attempts')
        report = {'status': 'PENDING', 'binarySHA256': sha(self.binary), 'sourceManifestSHA256': source_sha,
                  'runnerSHA256': sha(Path(__file__)), 'phoneUI': False, 'productionModified': False,
                  'walletOrMaterialSeeded': False, 'finitePrerequisites': 'level18 + original historical chapter1..8 five-star eligibility; no actual wins claimed',
                  'policy': 'growth-solo-materials-v1', 'fullChain': False}
        began = time.monotonic()
        self.managed_pool = ManagedMySQL()
        with tempfile.TemporaryDirectory(prefix='hyjs-growth-resource-') as temporary:
            self.native_log = Path(temporary) / 'native.log'
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.managed_pool.allocate('growth_resource')
                    self.database, self.test_user = allocation.database, allocation.user
                    self.test_password = allocation.password
                    self.env = {**os.environ, **allocation.env(), 'GOMEMLIMIT': '768MiB', 'GOMAXPROCS': '2'}
                    self.binary = str(self.binary)
                    self.start()
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-growth-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '保护资源账号', 'heroId': 'hero_047'}, protected, 1, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    identity = 'demo-growth-resource-' + suffix
                    owner = self.login(identity)
                    self.assertGreater(owner.playerId, 1)
                    stamp = 1200000

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        saved_stamp = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                            replies = list(pool.map(lambda _: self.call(name, fields, owner, saved_stamp, expected), range(8)))
                        self.assertTrue(all(reply == replies[0] for reply in replies))
                        return replies[0], saved_stamp

                    def count(key):
                        game = self.profile(owner)[1]
                        return sum(v['num'] for inv in ('cooking', 'gems', 'items') for v in (game.get(inv) or []) if v['id'] == key)

                    http('CSPlayerCreate', {'playerName': '单人成长原料', 'heroId': 'hero_047'}, 'SCPlayerCreate')
                    fresh = self.profile(owner)
                    self.assertEqual((fresh[0].get('coin', 0), fresh[0].get('diamond', 0)), (0, 0))
                    missions = [dict(chapterId=r['ChapterId'], missionId=r['MissionId'], star=5, challengeTimes=0)
                                for r in original_table('CopyMission') if int(r['ChapterId']) <= 8]
                    stars_hex = json.dumps(missions, separators=(',', ':')).encode().hex()
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',18), game=JSON_SET(game,'$.copy.passedCopyInfo',CAST(CONVERT(0x{stars_hex} USING utf8mb4) AS JSON)) WHERE id={owner.playerId};")
                    earned = 0
                    for chapter in range(1, 9):
                        for index in range(3):
                            result = http('CSCopyGetStarAward', {'chapterId': str(chapter), 'starIndex': index}, 'SCCopyGetStarAward')
                            earned += sum(v.num for v in result.gain.simps if v.id == 'coin')
                    self.assertGreaterEqual(earned, 1120000, 'Actual original star income must fund the chosen materials')
                    self.assertEqual(self.profile(owner)[0]['coin'], earned)
                    month = time.strftime('%Y%m', time.gmtime(time.time() + 8*3600))
                    key = PREFIX + month
                    self.assertEqual(self.sql(f"SELECT enabled FROM p4_commerce_rules WHERE id='{key}';").strip(), '0')
                    http('CSActivityShopInfo', {'activityId': key}, 'ErrorMessage')
                    # Open before source publication; the personal snapshot cannot
                    # adopt late goods until a real explicit refresh creates a new cycle.
                    self.assertEqual(self.sql('SELECT enabled FROM local_play_policies WHERE id=1;').strip(), '0')
                    self.sql('UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    self.activity(owner, 'open')
                    old_key, old_hash, old_raw, old_bindings = self.directory(owner)
                    self.assertFalse(any(v['childId'].startswith(PREFIX) for v in old_bindings))
                    self.sql(f"UPDATE p4_commerce_rules SET enabled=1 WHERE id='{key}' AND enabled=0 AND source LIKE 'growth-solo-materials-v1%';")
                    http('CSActivityShopInfo', {'activityId': old_key}, 'SCActivityShopInfo')
                    self.assertEqual(self.directory(owner)[:3], (old_key, old_hash, old_raw))
                    refresh_id = str(uuid.uuid4())
                    opened = self.activity(owner, 'refresh', refresh_id)
                    self.assertEqual(self.activity(owner, 'refresh', refresh_id), opened)
                    outer, _, _, all_bindings = self.directory(owner)
                    self.assertNotEqual(outer, old_key)
                    source_bindings = [v for v in all_bindings if v['childId'].startswith(PREFIX)]
                    self.assertEqual(len(source_bindings), 25)
                    self.assertEqual(len({v['productId'] for v in source_bindings}), 25)
                    tabs = http('CSActivityList', {}, 'SCActivityList')
                    self.assertIn(outer, [v.activityId for v in tabs.activityList])
                    listing = http('CSActivityShopInfo', {'activityId': outer}, 'SCActivityShopInfo')
                    selected_indices = {v['index'] for v in source_bindings}
                    goods = {v.getResource[0].id: v for v in listing.shopItemList
                             if v.index in selected_indices and len(v.getResource) == 1}
                    self.assertEqual(set(goods), {v['productId'] for v in source_bindings})
                    recruited = http('CSWarriorRecruit', {'costType': 1, 'numType': 1, 'free': 1}, 'SCWarriorRecruit')
                    self.assertEqual(len(recruited.get.wars), 1)
                    partner = recruited.get.wars[0]
                    purchases = {}
                    for raw_id, amount in [('FishItem_007', 1), ('FishItem_013', 1), ('MineItem_007', 20)]:
                        item = goods[raw_id]
                        request = {'activityId': outer, 'index': item.index, 'num': amount, 'costResources': item.costResource, 'getResources': item.getResource}
                        before = self.profile(owner)
                        bad = dict(request, costResources=[{'id': 'coin', 'num': 1}])
                        http('CSBuyActivityShop', bad, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), before)
                        reply, receipt_stamp = once('CSBuyActivityShop', request, 'SCGetActivityShopReward')
                        self.assertEqual(count(raw_id), amount * 5)
                        purchases[raw_id] = {'request': request, 'stamp': receipt_stamp, 'response': reply}
                    http('CSBuyActivityShop', purchases['MineItem_007']['request'], 'ErrorMessage')
                    cooked, cook_stamp = once('CSFishCooking', {'fish': {'id': 'FishItem_027', 'num': 1}}, 'SCFishCooking')
                    self.assertEqual({v.id: v.num for v in cooked.cost}, {'FishItem_007': 5, 'FishItem_013': 5, 'coin': 10000})
                    before_partner = next(v for v in self.profile(owner)[1]['warriors'] if v['guid'] == partner.guid)
                    used, use_stamp = once('CSFishUsing', {'heroGuid': partner.guid, 'costItem': {'id': 'FishItem_027', 'num': 1}}, 'SCFishUsing')
                    before_def = before_partner.get('cooking', {}).get('prop', {}).get('pdef', 0)
                    self.assertEqual(used.warrior.cooking.prop.pdef - before_def, 1)
                    self.assertEqual([count(v) for v in ('FishItem_007', 'FishItem_013', 'FishItem_027')], [0, 0, 0])
                    refined, refine_stamp = once('CSMineRefine', {'mineId': 'MineItem_007', 'num': 100}, 'SCMineRefine')
                    self.assertEqual(count('MineItem_007'), 0)
                    # Select a real available level1 recipe; never force a random refining result.
                    recipe = next((r for r in original_table('MineSkill') if r['Type'] == 'magicGem' and r['Level'] == '1'
                                   and all(k == 'coin' or count(k) >= int(n) for k, n in (entry.split(',') for entry in r['Material'].split(';')))), None)
                    self.assertIsNotNone(recipe, 'Bounded actual random ore result has insufficient ingredients; preserve this failure')
                    made, make_stamp = once('CSGemMake', {'gemId': recipe['MineItemID'], 'num': 1}, 'SCGemMake')
                    self.assertEqual([(v.id, v.num) for v in made.res.simps], [(recipe['MineItemID'], 1)])
                    saved = self.profile(owner)
                    self.stop(); self.start(); owner = self.login(identity)
                    self.assertEqual(self.profile(owner), saved)
                    self.assertEqual(self.call('CSFishUsing', {'heroGuid': partner.guid, 'costItem': {'id': 'FishItem_027', 'num': 1}}, owner, use_stamp, 'SCFishUsing'), used)
                    self.assertEqual(self.call('CSGemMake', {'gemId': recipe['MineItemID'], 'num': 1}, owner, make_stamp, 'SCGemMake'), made)
                    self.assertEqual(self.profile(owner), saved)
                    self.assertEqual(self.directory(owner)[0], outer)
                    self.assertEqual(self.profile(protected), protected_before)
                    retired = self.sql(f"SELECT input_hash,HEX(bindings) FROM p4_resource_exchange_directories WHERE id='{old_key}';").strip()
                    self.assertEqual(retired, old_hash + '\t' + old_raw)
                    self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('profiles','p4_commerce_rules','p4_commerce_states','gem_craft_states','cooking_craft_states');").strip(), 'InnoDB')
                    report.update(status='PASS', fullChain=True, actualCoinIncome=earned,
                                  actualPartner=partner.id, actualDish='FishItem_027', permanentDefenseGain=1,
                                  actualOreSpent=100, actualGem=recipe['MineItemID'],
                                  personalDirectory=outer, personalRawIDs=sorted(goods), oldDirectoryFrozen=True, explicitRefreshIdempotent=True, protected1Exact=True,
                                  duplicate8='purchases/cooking/eating/refine/make once', coldRestarts=1,
                                  newMountClaim=False, mountLimit='actual star gear001 has no original gem hole; highest finite119box mount proof reused separately',
                                  randomRefiningResultPreserved=True, wholeInventoryColdPreserved=True)
                except BaseException as error:
                    report.update(status='FAIL', failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    self.stop()
                    report['ownedAppStopped'] = self.process is None or self.process.poll() is not None
                    self.managed_pool.close()
                    report['ownedManagedAllocationsRemaining'] = len(self.managed_pool.connections)
                    report['seconds'] = round(time.monotonic() - began, 3)
                    output.parent.mkdir(parents=True, exist_ok=True)
                    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
