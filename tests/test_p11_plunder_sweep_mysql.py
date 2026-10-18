"""Original56009 actual5 child outcomes -> source energy -> real obtained stop.

Original stars, boxes and energy item are API outputs. Historical chapter1
qualification/player levels are explicit finite preparation. No winner injection.
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


@unittest.skipUnless(os.environ.get('HYJS_P11_PLUNDER_SWEEP_MYSQL_TEST') == '1',
                     'explicit root-coordinated managed SQL window')
class P11PlunderSweepMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql

    def test_original_five_actual_children_energy_item_fragment_stop_type10_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source = Path(os.environ['HYJS_P11_PLUNDER_SWEEP_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent, ROOT/'build/candidates')
        owned = json.loads((ROOT/'analysis/p11/plunder-sweep-code-manifest.json').read_text())
        shared = json.loads((ROOT/owned['sharedManifest']).read_text())
        proof = json.loads((ROOT/'analysis/p11/plunder-sweep-go-validation.json').read_text())
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
        prebuilt = Path(os.environ['HYJS_P11_PLUNDER_SWEEP_PREBUILT_BINARY']).resolve()
        self.assertEqual(prebuilt, self.source/'sweep-native')
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(), proof['binarySHA256'])
        pending = 'P6PlunderNativeProof = ""' in (self.source/'hyjs-server/constUtil/p6PlunderSettlement.go').read_text()
        self.assertFalse(pending, 'independent type3 certificate is required for original56009')
        began = time.monotonic()
        report = dict(status='PENDING', runtimeSHA256=hashes, managedHelperSHA256=helper,
                      independentType3ProofPending=pending, productionModified=False, phoneUI=False)
        checkpoint = ROOT/'analysis/p11/plunder-sweep-native-checkpoint.json'

        def save():
            checkpoint.write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')

        with tempfile.TemporaryDirectory(prefix='hyjs-plunder-sweep-native-') as temporary:
            self.binary = Path(temporary)/'sweep-native'
            shutil.copy2(prebuilt, self.binary)
            self.managed_pool = ManagedMySQL()
            connection = None
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    connection = self.managed_pool.allocate('p11_plunder_sweep')
                    self.database, self.test_user = connection.database, connection.user
                    self.test_password = connection.password
                    self.env = dict(os.environ, **connection.env(), GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.assertEqual(self.sql('SELECT @@character_set_client;').strip(), 'utf8mb4')
                    for name in ['policies.sql', 'offers.sql', 'matches.sql', 'settlements.sql', 'policies.seed.sql']:
                        self.sql((self.source/'hyjs-server/db/sql'/('156_p11_plunder_fight_'+name)).read_text())
                    self.sql((self.source/'hyjs-server/db/sql/156_p11_plunder_sweep_receipts.sql').read_text())
                    self.start()
                    self.assertEqual(self.sql('SELECT CONCAT(enabled,":",sweep_enabled) FROM p11_plunder_fight_policies;').strip(), '0:0')
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
                        if index in (0, 1):
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
                    owner_star = http('CSCopyGetStarAward', {'chapterId': '1', 'starIndex': 0}, owner, 'SCCopyGetStarAward')
                    self.assertEqual(sum(v.num for v in owner_star.gain.simps if v.id == 'diamond'), 30)
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
                    request = {'fragId': fragment, 'sweepNumber': 5, 'autoAddEnergy': 0}
                    for bad_stamp in [0, -1]:
                        opcode, rejected = self.stamped('CSSweepTreasureFight', request, owner, bad_stamp)
                        self.assertEqual(opcode, 10001)
                        self.assertIn('Invalid original plunder sweep', str(rejected))
                    self.assertEqual(self.stamped('CSSweepTreasureFight', request, owner, stamp+1)[0], 10001)
                    now_ms = int(time.time()*1000)
                    start_at = now_ms+1200
                    self.sql(f"UPDATE p11_plunder_fight_policies SET enabled=1,sweep_enabled=1,version='native-five-real-outcomes',published_at={now_ms},start_at={start_at},end_at={start_at+3600000},chance_tier=6,drop_ppm=0,level_difference=100 WHERE id=1;")
                    activity_id = self.sql("SELECT id FROM p1_period_instances WHERE kind='p4_plunder_attempt';").strip()
                    self.assertTrue(activity_id.startswith('robTreasureTimes_p4_'))
                    self.sql(f"UPDATE p1_period_instances SET enabled=1 WHERE id='{activity_id}';")
                    time.sleep(max(0, start_at/1000-time.time())+0.02)
                    targets = self.call('CSTreasureBattleList', {'fragTreasId': fragment}, owner, 0, 'SCTreasureBattleList')
                    self.assertEqual([(v.simplePlayerInfo.playerId,v.isRobot) for v in targets.infoList], [(defender.playerId,0)])
                    before_owner = self.profile(owner)
                    before_holder = count(defender, fragment)
                    # The original5 path sends autoAddEnergy0. Automatic
                    # medicine is closed before any original item purchase.
                    self.assertEqual(self.stamped('CSSweepTreasureFight', dict(request, autoAddEnergy=1), owner, stamp+2)[0], 10001)
                    stamp += 3
                    five_stamp = stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        five_results = list(workers.map(lambda _: self.call('CSSweepTreasureFight', request, owner,
                                                                           five_stamp, 'SCSweepTreasureFight'), range(8)))
                    five = five_results[0]
                    self.assertTrue(all(v == five for v in five_results))
                    self.assertEqual(len(five.rewardList), 5)
                    self.assertEqual([(v.id,v.num) for v in five.cost], [('energy',10)])
                    self.assertTrue(all(v.isGetFrag == 0 for v in five.rewardList))
                    actual_coin = sum(v.coin for v in five.rewardList)
                    actual_exp = sum(v.exp for v in five.rewardList)
                    self.assertEqual(self.profile(owner)[0]['coin'], before_owner[0].get('coin',0)+actual_coin)
                    self.assertEqual(self.profile(owner)[0]['energy'], before_owner[0]['energy']-10)
                    self.assertEqual(count(defender,fragment), before_holder)
                    self.assertEqual(count(owner,fragment), 0)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p6_battle_records WHERE authoritative=1;').strip(), '5')
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_plunder_fight_settlements WHERE stamp<0;').strip(), '5')
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_plunder_sweep_receipts;').strip(), '1')
                    info = http('CSActivityInfo', {'activityId':activity_id}, owner, 'SCActivityInfo')
                    self.assertEqual((info.itemList[0].value,info.itemList[0].left,info.itemList[0].status), ('5',5,1))
                    # Spend actual original star diamonds on the original energy
                    # item; no automatic supply/paid receipt shortcut is enabled.
                    shop = http('CSShopProductList', {'shopType':1}, owner, 'SCShopProductList')
                    energy_sku = next(v for v in shop.shopItemList if v.id == 'itemShopId_3')
                    self.assertEqual([(v.id,v.num) for v in energy_sku.item], [('item_001',1)])
                    self.assertEqual([(v.id,v.num) for v in energy_sku.price], [('diamond',10)])
                    http('CSShopBuy', {'shopType':1,'shopItemId':energy_sku.id,'buyCount':1}, owner, 'SCShopBuy')
                    http('CSItemUse', {'costItem':{'id':'item_001','num':1}}, owner, 'SCItemUse')
                    self.assertEqual(self.profile(owner)[0]['energy'], before_owner[0]['energy'])
                    self.assertEqual(count(owner,'item_001'), 0)
                    # A new prepublished version gives an observable obtained
                    # target stop. PPM0 and1m are explicit finite local test rules.
                    now_ms = int(time.time()*1000)
                    start_at = now_ms+1200
                    self.sql(f"UPDATE p11_plunder_fight_policies SET version='native-actual-fragment-stop',published_at={now_ms},start_at={start_at},end_at={start_at+3600000},drop_ppm=1000000 WHERE id=1;")
                    time.sleep(max(0,start_at/1000-time.time())+0.02)
                    stopped = http('CSSweepTreasureFight', request, owner, 'SCSweepTreasureFight')
                    self.assertEqual(len(stopped.rewardList), 1)
                    self.assertEqual(stopped.rewardList[0].isGetFrag, 1)
                    self.assertEqual([(v.id,v.num) for v in stopped.cost], [('energy',2)])
                    self.assertEqual(count(owner,fragment), 1)
                    self.assertEqual(count(defender,fragment), before_holder-1)
                    self.assertEqual(self.profile(owner)[0]['energy'], before_owner[0]['energy']-2)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_plunder_fight_settlements;').strip(), '6')
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_plunder_sweep_receipts;').strip(), '2')
                    claimed = http('CSActivityReward', {'activityId':activity_id,'index':1,'getResources':[{'id':'item_022','num':1}]}, owner, 'SCActivityReward')
                    self.assertEqual([(v.id,v.num) for v in claimed.get.simps], [('item_022',1)])
                    used_box = http('CSItemUse', {'costItem':{'id':'item_022','num':1}}, owner, 'SCItemUse')
                    self.assertEqual(count(owner,'item_022'), 0)
                    self.sql('UPDATE p11_plunder_fight_policies SET enabled=0;')
                    frozen = self.sql('SELECT stamp,request_hash,source_hash,response_hash FROM p11_plunder_sweep_receipts ORDER BY stamp;')
                    report.update(status='PASS_ORIGINAL56009_FIVE_ACTUAL_CHILDREN_ENERGY_SOURCE_TARGET_STOP_TYPE10_COLD',
                                  sameStampWorkers=8, actualFirstChildren=5, actualFirstEnergyCost=10,
                                  actualFirstCoin=actual_coin, actualFirstExp=actual_exp,
                                  actualOwnerStarDiamond=30, actualEnergyItemDiamond=10, actualEnergyItemRestored=10, actualBeforeEnergy=before_owner[0]['energy'],
                                  actualSecondChildren=1, actualSecondEnergyCost=2, actualFragmentTransfer=1,
                                  localTestPPM=[0,1000000], actualType10Attempts=6, activityReward022=1,
                                  actualActivityBoxResult=str(used_box.get), newSweepFlagDefaultOff=True)
                    stable_owner, stable_defender = self.profile(owner), self.profile(defender)
                    self.stop()
                    self.start()
                    owner, defender = [self.login(v) for v in identities]
                    self.assertEqual(self.profile(owner), stable_owner)
                    self.assertEqual(self.profile(defender), stable_defender)
                    self.assertEqual(self.sql('SELECT stamp,request_hash,source_hash,response_hash FROM p11_plunder_sweep_receipts ORDER BY stamp;'), frozen)
                    self.assertEqual(self.profile(owner)[0]['heroWarId'], main_guids[owner.playerId])
                    self.assertEqual(self.call('CSSweepTreasureFight', request, owner, five_stamp, 'SCSweepTreasureFight'), five)
                    self.assertEqual(self.profile(owner), stable_owner)
                    self.assertEqual(self.profile(defender), stable_defender)
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={owner.playerId} AND kind='p4_plunder_attempt';").strip(), '6')
                    report.update(coldRestarts=1, sourceDefaultOff=True, actualCurrentHolderVerified=True,
                                  actualUnprotectedDistinctActors=True, noResourceOrWinnerInjection=True,
                                  finitePrerequisites='OriginalCreate047; finite isolated player/main100vs20 and guide8; explicit test-local matching levelDifference100; both original chapter1 historical25stars. Actual defender90diamond/coin10000 star reward and original5diamond022; actual owner30diamond firststar and10diamond original001. All captures/winners/coins/EXP/fragment transfer/energy recovery/Type10 reward from original APIs; no item/wallet/GUID/seed/protection/result injection.')
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
        (ROOT/'analysis/p11/plunder-sweep-mysql-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
