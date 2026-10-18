"""Original day1 400-diamond box -> ordinary coloured gear -> real ore gem.

Uses the same root-qualified resource binary. Only level18/historical chapter
stars are bounded eligibility; no material, currency, box or equipment GUID
is injected. No compiler/phone or live mutations. Preserve real RNG failure.
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

import test_growth_resource_loop_mysql as base
from hyjs_managed_mysql import ManagedMySQL
from server.codec import message, parse_http


def protocol_preflight():
    vectors = {
        'CSGetOpenServerReward': {'itemId': 20},
        'CSItemUse': {'costItem': {'id': 'item_055', 'num': 1}, 'getItemIndex': 0},
        'CSFormationOn': {'id': 'equip_005', 'guid': 'actual-guid', 'formatPos': 1},
        'CSGemMount': {'guid': 'actual-guid', 'pos': 0, 'gemId': 'MineItem_019'},
        'CSGemUnmount': {'guid': 'actual-guid', 'pos': 0},
        'CSCopyEnter': {'chapterId': '1', 'missionId': '1'},
        'CSCopyRunMapInfo': {'chapterId': '1', 'missionId': '1'},
        'CSCopyLeave': {'chapterId': '1', 'missionId': '1', 'taskId': ''},
    }
    for name, fields in vectors.items():
        raw = base.wire.encode_original_http(name, fields, 2, b'0' * 16, 200)
        if parse_http(raw).decode() != message(name).__class__(**fields):
            raise AssertionError('Original consumer wire changed: ' + name)
    for name, fields in {
        'SCGetOpenServerReward': {'get'}, 'SCItemUse': {'cost', 'get'},
        'SCFormationOn': {'formationItem'}, 'SCGemMount': {'equip', 'cost'},
        'SCGemUnmount': {'equip', 'res'}, 'SCCopyRunMapInfo': {'battlePlayer'},
        'BattlePlayer': {'formation'}, 'BattleHero': {'prop'},
    }.items():
        if not fields <= set(message(name).DESCRIPTOR.fields_by_name):
            raise AssertionError('Original consumer output changed: ' + name)
    original = base.original_table
    row = next(r for r in original('ActivityNewOpen7Day') if r['id'] == '20')
    if (row['taskType'], row['openDay'], row['newPrice'], row['reward']) != ('7', '1', '400', 'item_055,1;'):
        raise AssertionError('Original 400 diamond source changed')
    box = [r for r in original('ItemUse') if r['itemId'] == 'item_055']
    if len(box) != 1 or box[0]['category'] != '2' or box[0]['result'] != 'equip_005,1;equip_006,1;equip_007,1;equip_008,1':
        raise AssertionError('Original deterministic four-gear box changed')
    return {'originalRequests': len(vectors), 'originalBox': box[0]['id'], 'originalOpenTask': 20, 'originalPrice': 400}


@unittest.skipUnless(os.environ.get('HYJS_GROWTH_EQUIPMENT_MYSQL_TEST') == '1', 'Root coordinated same-binary SQL window')
class GrowthEquipmentGemMySQLTests(unittest.TestCase):
    send = base.GrowthResourceLoopMySQLTests.send
    login = base.GrowthResourceLoopMySQLTests.login
    stamped = base.GrowthResourceLoopMySQLTests.stamped
    call = base.GrowthResourceLoopMySQLTests.call
    profile = base.GrowthResourceLoopMySQLTests.profile
    start = base.GrowthResourceLoopMySQLTests.start
    stop = base.GrowthResourceLoopMySQLTests.stop
    setUp = base.GrowthResourceLoopMySQLTests.setUp
    sql = base.GrowthResourceLoopMySQLTests.sql
    json_request = base.GrowthResourceLoopMySQLTests.json_request
    directory = base.GrowthResourceLoopMySQLTests.directory

    def test_original_blue_box_real_gem_mount_full_panel_unmount_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        contract = protocol_preflight()
        self.source, binary, manifest_sha = base.preflight()
        self.assertEqual(base.sha(Path(base.__file__)), 'd12406fd260d7a27ab1fd4845698b146d915154d77866565eddbd1c0453a60a5')
        for table in ['ActivityNewOpen7Day', 'ItemUse', 'Equip']:
            self.assertEqual(base.sha(self.source / 'analysis/tables' / ('Table' + table + '.tsv')),
                             base.sha(base.ROOT / 'analysis/tables' / ('Table' + table + '.tsv')))
        self.binary = str(binary)
        report_path = Path(os.environ['HYJS_GROWTH_EQUIPMENT_OUTPUT']).resolve()
        self.assertTrue(report_path.is_relative_to(base.ROOT / 'analysis/growth-first-20261016/resources'))
        self.assertFalse(report_path.exists())
        report = dict(status='PENDING', runnerSHA256=base.sha(Path(__file__)),
                      resourceRunnerSHA256=base.sha(Path(base.__file__)),
                      binarySHA256=base.sha(binary), sourceManifestSHA256=manifest_sha,
                      contract=contract, finitePrerequisites='isolated level18/historical original chapter1..8stars; no natural wins asserted',
                      walletMaterialOrAwardGUIDSeeded=False, phone=False, productionModified=False)
        began = time.monotonic()
        self.managed_pool = ManagedMySQL()
        with tempfile.TemporaryDirectory(prefix='hyjs-growth-ordinary-gear-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.managed_pool.allocate('growth_gear')
                    self.database = allocation.database
                    self.env = {**os.environ, **allocation.env(), 'GOMAXPROCS': '2', 'GOMEMLIMIT': '768MiB'}
                    self.start()
                    uid = 'demo-growth-gear-' + secrets.token_hex(5)
                    protected = self.login(uid + '-protected')
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '装备保护账号', 'heroId': 'hero_047'}, protected, 1, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    owner = self.login(uid)
                    stamp = 1400000

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                            replies = list(pool.map(lambda _: self.call(name, fields, owner, stamp, expected), range(8)))
                        self.assertTrue(all(r == replies[0] for r in replies))
                        return replies[0], stamp

                    def apply(activity):
                        status, reply = self.json_request('/local-play/activities/apply', {
                            'playerId': owner.playerId, 'sessionHex': bytes(owner.sessionKey).hex(),
                            'activityId': activity, 'action': 'open', 'requestId': str(uuid.uuid4())})
                        self.assertEqual(status, 200)
                        self.assertEqual(reply.get('code'), 0, reply.get('msg'))

                    def count(key):
                        game = self.profile(owner)[1]
                        return sum(i['num'] for slot in ('items', 'gems', 'cooking') for i in (game.get(slot) or []) if i['id'] == key)

                    http('CSPlayerCreate', {'playerName': '铁血魔石真链', 'heroId': 'hero_047'}, 'SCPlayerCreate')
                    fresh = self.profile(owner)[0]
                    self.assertEqual((fresh.get('coin', 0), fresh.get('diamond', 0)), (0, 0))
                    missions = [dict(chapterId=r['ChapterId'], missionId=r['MissionId'], star=5, challengeTimes=0)
                                for r in base.original_table('CopyMission') if int(r['ChapterId']) <= 8]
                    raw = json.dumps(missions, separators=(',', ':')).encode().hex()
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',18),game=JSON_SET(game,'$.copy.passedCopyInfo',CAST(CONVERT(0x{raw} USING utf8mb4) AS JSON)) WHERE id={int(owner.playerId)};")
                    for chapter in range(1, 9):
                        for index in range(3):
                            http('CSCopyGetStarAward', {'chapterId': str(chapter), 'starIndex': index}, 'SCCopyGetStarAward')
                    before = self.profile(owner)[0]
                    self.assertGreaterEqual(before['coin'], 1010000)
                    self.assertGreaterEqual(before['diamond'], 400)
                    self.sql('UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    apply('OpenHappy')
                    purchase, _ = once('CSGetOpenServerReward', {'itemId': 20}, 'SCGetOpenServerReward')
                    self.assertEqual(self.profile(owner)[0]['diamond'], before['diamond'] - 400)
                    self.assertEqual(count('item_055'), 1)
                    opened, _ = once('CSItemUse', {'costItem': {'id': 'item_055', 'num': 1}, 'getItemIndex': 0}, 'SCItemUse')
                    self.assertEqual(count('item_055'), 0)
                    gears = list(opened.get.equips)
                    self.assertEqual({g.equipId for g in gears}, {'equip_005', 'equip_006', 'equip_007', 'equip_008'})
                    self.assertEqual(len({g.guid for g in gears}), 4)
                    gear_tables = {r['id']: r for r in base.original_table('Equip')}
                    coloured = {}
                    for gear in gears:
                        coloured[int(gear_tables[gear.equipId]['hole1'])] = gear
                        http('CSFormationOn', {'id': gear.equipId, 'guid': gear.guid, 'formatPos': 1}, 'SCFormationOn')
                    month = time.strftime('%Y%m', time.gmtime(time.time() + 8*3600))
                    self.sql(f"UPDATE p4_commerce_rules SET enabled=1 WHERE id='{base.PREFIX + month}' AND source LIKE 'growth-solo-materials-v1%';")
                    apply('TimeLimitExchange')
                    outer, _, _, bindings = self.directory(owner)
                    binding = next(b for b in bindings if b['childId'].startswith(base.PREFIX) and b['productId'] == 'MineItem_007')
                    listing = http('CSActivityShopInfo', {'activityId': outer}, 'SCActivityShopInfo')
                    quote = next(i for i in listing.shopItemList if i.index == binding['index'])
                    once('CSBuyActivityShop', {'activityId': outer, 'index': quote.index, 'num': 20,
                         'costResources': quote.costResource, 'getResources': quote.getResource}, 'SCGetActivityShopReward')
                    once('CSMineRefine', {'mineId': 'MineItem_007', 'num': 100}, 'SCMineRefine')
                    candidates = [r for r in base.original_table('MineSkill') if r['Type'] == 'magicGem' and r['Level'] == '1' and int(r['Color']) in coloured]
                    recipe = next((r for r in candidates if all(k == 'coin' or count(k) >= int(n)
                                  for k, n in (part.split(',') for part in r['Material'].split(';')))), None)
                    self.assertIsNotNone(recipe, 'Actual finite refinery has no affordable matching recipe; preserve genuine RNG result')
                    gem = recipe['MineItemID']
                    once('CSGemMake', {'gemId': gem, 'num': 1}, 'SCGemMake')
                    gear = coloured[int(recipe['Color'])]
                    http('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, 'SCCopyEnter')
                    before_panel = http('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, 'SCCopyRunMapInfo').battlePlayer
                    once('CSGemMount', {'guid': gear.guid, 'pos': 0, 'gemId': gem}, 'SCGemMount')
                    after_panel = http('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, 'SCCopyRunMapInfo').battlePlayer
                    prop, value = recipe['Value'].split(',')
                    self.assertEqual(getattr(after_panel.formation[0].prop, prop) - getattr(before_panel.formation[0].prop, prop), float(value))
                    unmount_fields = {'guid': gear.guid, 'pos': 0}
                    unmounted, unmount_stamp = once('CSGemUnmount', unmount_fields, 'SCGemUnmount')
                    restored = http('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, 'SCCopyRunMapInfo').battlePlayer
                    self.assertEqual(restored, before_panel)
                    self.assertEqual(count(gem), 1)
                    http('CSCopyLeave', {'chapterId': '1', 'missionId': '1', 'taskId': ''}, 'SCCopyLeave')
                    saved = self.profile(owner)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), saved)
                    self.assertEqual(self.call('CSGemUnmount', unmount_fields, owner, unmount_stamp, 'SCGemUnmount'), unmounted)
                    self.assertEqual(self.profile(owner), saved)
                    self.assertEqual(self.profile(protected), protected_before)
                    report.update(status='PASS', actualSource='original open task20,400 diamonds', actualBox='item_055',
                                  equipmentIDs=sorted(g.equipId for g in gears), actualOreSpent=100, actualGem=gem,
                                  actualBattlePanelDelta={prop: float(value)}, sourceAndConsumers8DuplicateOnce=True,
                                  originalUnmountRestoresPanel=True, noClaimedBattleWinner=True, coldRestarts=1, protected1Exact=True)
                except BaseException as error:
                    report.update(status='FAIL', failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    self.stop()
                    self.managed_pool.close()
                    report.update(seconds=round(time.monotonic()-began, 3), ownedAppStopped=self.process is None,
                                  ownedManagedAllocationsRemaining=len(self.managed_pool.connections))
                    report_path.parent.mkdir(parents=True, exist_ok=True)
                    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
