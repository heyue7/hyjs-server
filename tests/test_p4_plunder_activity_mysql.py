"""Original actual plunder attempt -> Type10 P1 reward -> original box use/cold.

Finite historical chapter qualification is explicit. Currency, source box,
fragment, accepted independent type3 result and activity reward are real APIs.
"""
import concurrent.futures
import csv
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import tempfile
import time
import unittest

from hyjs_managed_mysql import ManagedMySQL
from server.codec import message
from test_p11_arena_prize_mysql import P11ArenaPrizeMySQLTests as Transport

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P4_PLUNDER_ACTIVITY_MYSQL_TEST') == '1',
                     'explicit root-coordinated managed SQL window')
class P4PlunderActivityMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql

    def test_original_real_plunder_activity_reward_box_use_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source = Path(os.environ['HYJS_P4_PLUNDER_ACTIVITY_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent, ROOT/'build/candidates')
        owned = json.loads((ROOT/'analysis/p4/plunder-activity-code-manifest.json').read_text())
        shared = json.loads((ROOT/owned['sharedManifest']).read_text())
        proof = json.loads((ROOT/'analysis/p4/plunder-activity-go-validation.json').read_text())
        hashes = {}
        for row in owned['files']+shared['files']:
            value = hashlib.sha256((self.source/row['path']).read_bytes()).hexdigest()
            self.assertEqual(value, row['sha256'], row['path'])
            hashes[row['path']] = value
        for rel, value in proof['ownedGoSHA256'].items():
            self.assertEqual(hashes[rel], value)
        for rel, expected in proof['indirectRuntimeSHA256'].items():
            actual = hashlib.sha256((self.source/rel).read_bytes()).hexdigest()
            self.assertEqual(actual, expected, 'prebuilt indirect runtime '+rel)
            hashes[rel] = actual
        helper = hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper, '483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        prebuilt = Path(os.environ['HYJS_P4_PLUNDER_ACTIVITY_PREBUILT_BINARY']).resolve()
        self.assertEqual(prebuilt, self.source/'type10-native')
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(), proof['binarySHA256'])
        pending = 'P6PlunderNativeProof = ""' in (self.source/'hyjs-server/constUtil/p6PlunderSettlement.go').read_text()
        self.assertFalse(pending, 'independent certified type3 is required for actual Type10 acceptance')
        began = time.monotonic()
        report = dict(status='PENDING', runtimeSHA256=hashes, managedHelperSHA256=helper,
                      independentType3ProofPending=pending, productionModified=False, phoneUI=False)
        checkpoint = ROOT/'analysis/p4/plunder-activity-native-checkpoint.json'

        def save():
            checkpoint.write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')

        with tempfile.TemporaryDirectory(prefix='hyjs-type10-native-') as temporary:
            self.binary = Path(temporary)/'type10-native'
            shutil.copy2(prebuilt, self.binary)
            self.managed_pool = ManagedMySQL()
            connection = None
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    connection = self.managed_pool.allocate('p4_plunder_activity')
                    self.database, self.test_user = connection.database, connection.user
                    self.test_password = connection.password
                    self.env = dict(os.environ, **connection.env(), GOMEMLIMIT='768MiB')
                    self.assertEqual(self.sql('SELECT @@character_set_client;').strip(), 'utf8mb4')
                    for name in ['policies.sql', 'offers.sql', 'matches.sql', 'settlements.sql', 'policies.seed.sql']:
                        self.sql((self.source/'hyjs-server/db/sql'/('156_p11_plunder_fight_'+name)).read_text())
                    self.start()
                    self.assertEqual(self.sql('SELECT enabled FROM p11_plunder_fight_policies;').strip(), '0')
                    identities = ['plunder-original-'+secrets.token_hex(5) for _ in range(2)]
                    owner, defender = [self.login(v) for v in identities]
                    stamp = 98000

                    def http(name, fields, actor, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, actor, stamp, expected)

                    def count(actor, resource):
                        return sum(v['num'] for v in (self.profile(actor)[1].get('items') or []) if v['id'] == resource)

                    main_guids = {}
                    for index, actor in enumerate([owner, defender]):
                        created = http('CSPlayerCreate', {'playerName': '夺宝真源'+str(index), 'heroId': 'hero_047'}, actor, 'SCPlayerCreate')
                        main_guids[actor.playerId] = created.player.heroWarId
                        player, game = self.profile(actor)
                        self.assertEqual((player.get('coin', 0), player.get('diamond', 0)), (0, 0))
                        player.update(level=100 if index == 0 else 20, noviceGuideSchedule=8)
                        main = next(v for v in game['warriors'] if v['guid'] == created.player.heroWarId)
                        main['level'] = 100 if index == 0 else 20
                        main['exp'] = 0
                        if index == 1:
                            with (ROOT/'analysis/tables/TableCopyMission.tsv').open() as table_file:
                                table = list(csv.reader(table_file, delimiter='\t'))
                            missions = [dict(zip(table[1], v)) for v in table[5:] if v and v[0]]
                            game.setdefault('copy', {})['passedCopyInfo'] = [
                                dict(chapterId=v['ChapterId'], missionId=v['MissionId'], star=5,
                                     challengeTimes=0, isOpen=1) for v in missions if v['ChapterId'] == '1']
                            game['copy']['passedChapterInfo'] = []
                        p = json.dumps(player, ensure_ascii=False, separators=(',', ':')).encode().hex()
                        g = json.dumps(game, ensure_ascii=False, separators=(',', ':')).encode().hex()
                        self.sql(f'UPDATE profiles SET player=CONVERT(0x{p} USING utf8mb4),game=CONVERT(0x{g} USING utf8mb4) WHERE id={actor.playerId};')
                    star = http('CSCopyGetStarAward', {'chapterId': '1', 'starIndex': 2}, defender, 'SCCopyGetStarAward')
                    self.assertEqual(sum(v.num for v in star.gain.simps if v.id == 'diamond'), 90)
                    listing = http('CSShopProductList', {'shopType': 1}, defender, 'SCShopProductList')
                    sku = next(v for v in listing.shopItemList if v.id == 'itemShopId_14')
                    self.assertEqual([(v.id, v.num) for v in sku.item], [('item_022', 1)])
                    self.assertEqual([(v.id, v.num) for v in sku.price], [('diamond', 5)])
                    fragment = None
                    opened_boxes = 0
                    for _ in range(18):
                        http('CSShopBuy', {'shopType': 1, 'shopItemId': sku.id, 'buyCount': 1}, defender, 'SCShopBuy')
                        opened = http('CSItemUse', {'costItem': {'id': 'item_022', 'num': 1}}, defender, 'SCItemUse')
                        opened_boxes += 1
                        candidates = [v.id for v in opened.get.simps if v.id.startswith('frag_trea_')]
                        if candidates:
                            fragment = candidates[0]
                            break
                    self.assertIsNotNone(fragment, 'real original random box did not provide a fragment within actual90diamond budget')
                    report.update(actualStarDiamond=90, actualBoxDiamond=5*opened_boxes,
                                  actualOriginalBoxesOpened=opened_boxes, actualTargetFragment=fragment)
                    save()
                    self.assertEqual(count(owner, fragment), 0)
                    self.assertGreaterEqual(count(defender, fragment), 1)
                    request = dict(fightPlayerId=defender.playerId, targetFlagId=fragment)
                    for bad_stamp in [0, -1]:
                        opcode, rejected = self.stamped('CSFightTreasureStartResult', request, owner, bad_stamp)
                        self.assertEqual(opcode, 10001)
                        self.assertIn('Invalid original plunder target or timestamp', str(rejected))
                    self.assertEqual(self.stamped('CSTreasureBattleList', {'fragTreasId': fragment}, owner, 0)[0], 10001)
                    now_ms = int(time.time()*1000)
                    start_at = now_ms+1200
                    self.sql(f"UPDATE p11_plunder_fight_policies SET enabled=1,version='native-real-source-type3',published_at={now_ms},start_at={start_at},end_at={start_at+3600000},chance_tier=6,drop_ppm=1000000,level_difference=100 WHERE id=1;")
                    activity_id = self.sql("SELECT id FROM p1_period_instances WHERE kind='p4_plunder_attempt';").strip()
                    self.assertTrue(activity_id.startswith('robTreasureTimes_p4_'))
                    self.assertEqual(self.sql(f"SELECT enabled FROM p1_period_instances WHERE id='{activity_id}';").strip(), '0')
                    self.sql(f"UPDATE p1_period_instances SET enabled=1,goal=1 WHERE id='{activity_id}';")
                    time.sleep(max(0, start_at/1000-time.time())+0.02)
                    targets = self.call('CSTreasureBattleList', {'fragTreasId': fragment}, owner, 0, 'SCTreasureBattleList')
                    self.assertEqual([(v.simplePlayerInfo.playerId, v.isRobot, v.getChance) for v in targets.infoList], [(defender.playerId, 0, 6)])
                    tabs = http('CSActivityList', {}, owner, 'SCActivityList')
                    self.assertEqual([v.activityId for v in tabs.activityList if v.activityId.startswith('robTreasureTimes')], [activity_id])
                    frozen = self.sql('SELECT id,source_hash,offer_hash,HEX(attacker),HEX(defender) FROM p11_plunder_fight_matches ORDER BY id;')
                    self.assertTrue(frozen.strip())
                    before_owner, before_defender = self.profile(owner), self.profile(defender)
                    before_holder = count(defender, fragment)
                    stamp += 1
                    fight_stamp = stamp
                    if pending:
                        opcode, rejected = self.stamped('CSFightTreasureStartResult', request, owner, fight_stamp)
                        self.assertEqual(opcode, 10001)
                        self.assertIn('服务端战斗尚未支持该技能效果或运行状态', str(rejected))
                        self.assertEqual(self.profile(owner), before_owner)
                        self.assertEqual(self.profile(defender), before_defender)
                        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_plunder_fight_settlements;').strip(), '0')
                        self.assertEqual(self.sql('SELECT COUNT(*) FROM p6_battle_records;').strip(), '0')
                        report['status'] = 'PASS_REAL_STAR_BOX_HOLDER_FROZEN_TYPE3_PENDING_ATOMIC_REJECT'
                    else:
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                            results = list(pool.map(lambda _: self.call('CSFightTreasureStartResult', request, owner,
                                                                        fight_stamp, 'SCFightTreasureStartResult'), range(8)))
                        self.assertTrue(all(v == results[0] for v in results))
                        result = results[0]
                        self.assertEqual((result.isWin, result.isGetFrag, result.maxRoundNum), (1, 1, 20))
                        self.assertEqual([(v.id, v.num) for v in result.cost], [('energy', 2)])
                        self.assertEqual({v.id: v.num for v in result.get.simps}, {'coin': 5500, 'exp': 200, fragment: 1})
                        self.assertEqual((result.reward, result.rewardFake1, result.rewardFake2), (fragment+',1', 'coin,0', 'coin,0'))
                        self.assertEqual(count(owner, fragment), 1)
                        self.assertEqual(count(defender, fragment), before_holder-1)
                        self.assertEqual(self.profile(owner)[0]['energy'], before_owner[0]['energy']-2)
                        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_plunder_fight_settlements;').strip(), '1')
                        report.update(status='PASS_REAL_STAR_BOX_HOLDER_TYPED3_PAYMENT_TRANSFER_COLD', sameStampWorkers=8,
                                      actualEnergyCost=2, actualCoinGain=5500, actualExpGain=200,
                                      actualFragmentTransfer=1, localTestDropPPM=1000000, localTestLevelDifference=100)
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={owner.playerId} AND kind='p4_plunder_attempt';").strip(), '1')
                    info = http('CSActivityInfo', {'activityId': activity_id}, owner, 'SCActivityInfo')
                    self.assertEqual(len(info.itemList), 1)
                    item = info.itemList[0]
                    self.assertEqual((item.value, item.left, item.status), ('1', 1, 1))
                    self.assertEqual([(v.id, v.num) for v in item.rewardList], [('item_022', 1)])
                    self.sql(f"UPDATE p1_period_instances SET goal=99,rewards='diamond,999' WHERE id='{activity_id}';")
                    claim = {'activityId': activity_id, 'index': 1,
                             'getResources': [{'id': v.id, 'num': v.num} for v in item.rewardList]}
                    box_before = count(owner, 'item_022')
                    stamp += 1
                    reward_stamp = stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        claimed = list(workers.map(lambda _: self.call('CSActivityReward', claim, owner,
                                                                      reward_stamp, 'SCActivityReward'), range(8)))
                    self.assertEqual(sum(v.num for response in claimed for v in response.get.simps if v.id == 'item_022'), 1)
                    self.assertEqual(count(owner, 'item_022'), box_before+1)
                    actual_use = http('CSItemUse', {'costItem': {'id': 'item_022', 'num': 1}}, owner, 'SCItemUse')
                    self.assertEqual(count(owner, 'item_022'), box_before)
                    self.assertTrue(actual_use.get.simps or actual_use.get.wars or actual_use.get.equips or actual_use.get.treas)
                    for resource in actual_use.get.simps:
                        self.assertGreaterEqual(count(owner, resource.id), resource.num)
                    report.update(status='PASS_ORIGINAL_TYPE10_ACCEPTED_PLUNDER_P1_BOX_USE_COLD',
                                  actualActivityID=activity_id, actualActivityAttempts=1,
                                  activityReward={'id': 'item_022', 'num': 1}, activityRewardWorkers=8,
                                  actualOriginalActivityBoxResult=str(actual_use.get),
                                  frozenGoalRewardPreservedAfterLiveEdit=True)
                    stable_owner, stable_defender = self.profile(owner), self.profile(defender)
                    self.stop()
                    self.start()
                    owner, defender = [self.login(v) for v in identities]
                    self.assertEqual(self.profile(owner), stable_owner)
                    self.assertEqual(self.profile(defender), stable_defender)
                    self.assertEqual(self.sql('SELECT id,source_hash,offer_hash,HEX(attacker),HEX(defender) FROM p11_plunder_fight_matches ORDER BY id;'), frozen)
                    self.assertEqual(self.profile(owner)[0]['heroWarId'], main_guids[owner.playerId])
                    if not pending:
                        self.assertEqual(self.call('CSFightTreasureStartResult', request, owner, fight_stamp, 'SCFightTreasureStartResult'), results[0])
                        self.assertEqual(self.profile(owner), stable_owner)
                        self.assertEqual(self.profile(defender), stable_defender)
                    current_info = http('CSActivityInfo', {'activityId': activity_id}, owner, 'SCActivityInfo')
                    self.assertEqual((current_info.itemList[0].value, current_info.itemList[0].status), ('1', 2))
                    old_claim = http('CSActivityReward', claim, owner, 'SCActivityReward')
                    self.assertEqual(list(old_claim.get.simps), [])
                    self.assertEqual(self.profile(owner), stable_owner)
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={owner.playerId} AND kind='p4_plunder_attempt';").strip(), '1')
                    report.update(coldRestarts=1, sourceDefaultOff=True, actualCurrentHolderVerified=True,
                                  actualUnprotectedDistinctActors=True, noResourceOrWinnerInjection=True,
                                  finitePrerequisites='OriginalCreate047; finite isolated player/main100vs20 and guide8; explicit test-local matching levelDifference100; defender original chapter1 historical25stars. Actual90diamond/coin10000 star reward, original5diamond022 purchases and unmodified random boxes; no item/fragment/wallet/GUID/seed/protection/result injection.')
                except BaseException as failure:
                    report.update(status='FAIL', failure=self.managed_pool.redact(str(failure)))
                    raise
                finally:
                    try:
                        self.stop()
                    finally:
                        try:
                            if connection is not None:
                                self.managed_pool.release(connection.database)
                        finally:
                            self.managed_pool.close()
                    report['managedAllocationsRemoved'] = True
                    report['seconds'] = round(time.monotonic()-began, 3)
                    save()
        (ROOT/'analysis/p4/plunder-activity-mysql-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
