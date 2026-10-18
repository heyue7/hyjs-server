"""Managed original star coins ->330 choice ->312 real awakening fusion.

Opt in only during the root-coordinated SQL window. Initial awakening qualification
uses finite raw fragments/materials and the actual original combine/talent/awake APIs.
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
from test_p11_arena_prize_mysql import P11ArenaPrizeMySQLTests as Transport

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P5_AWAKE_MATERIAL_MYSQL_TEST') == '1',
                     'explicit coordinated managed SQL window')
class P5AwakeMaterialSupplyMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql

    def test_original_star_coin_choice_book_actual_fusion_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source = Path(os.environ['HYJS_P5_AWAKE_MATERIAL_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent, ROOT/'build/candidates')
        manifest = json.loads((ROOT/'analysis/p5/awake-material-supply-code-manifest.json').read_text())
        hashes = {v['path']: hashlib.sha256((self.source/v['path']).read_bytes()).hexdigest()
                  for v in manifest['files']}
        for row in manifest['files']:
            self.assertEqual(hashes[row['path']], row['sha256'])
        helper = hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper, '483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        proof = json.loads((ROOT/'analysis/p5/awake-material-supply-go-validation.json').read_text())
        self.assertEqual(proof['status'], 'PASS')
        prebuilt = Path(os.environ['HYJS_P5_AWAKE_MATERIAL_PREBUILT_BINARY']).resolve()
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(), proof['binarySHA256'])
        for rel, expected in proof['sourceSHA256'].items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(), expected, rel)
        began = time.monotonic()
        report = {'status': 'PENDING', 'sourceSHA256': hashes, 'helperSHA256': helper,
                  'prebuiltSHA256': proof['binarySHA256']}
        with tempfile.TemporaryDirectory(prefix='hyjs-awake-material-') as temporary:
            self.binary = Path(temporary)/'awake-material-native'
            shutil.copy2(prebuilt, self.binary)
            self.managed_pool = ManagedMySQL()
            connection = None
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    connection = self.managed_pool.allocate('p5_awake_material')
                    self.database = connection.database
                    self.test_user = connection.user
                    self.test_password = connection.password
                    self.assertTrue(self.test_user.startswith('hyjs_t_'))
                    self.env = dict(os.environ, **connection.env(), GOMEMLIMIT='768MiB')
                    self.start()
                    month = time.strftime('%Y%m', time.localtime())
                    child = 'limitExchange_p4_awakematerial_'+month
                    directory = 'limitExchange_p4_resources_'+month
                    self.assertEqual(self.sql("SELECT CONCAT(enabled,':',COALESCE(OCTET_LENGTH(frozen),0)) FROM p4_commerce_rules WHERE id='"+child+"';").strip(), '0:0')
                    seed = (self.source/'hyjs-server/db/sql/83_p4_commerce_rules.seed.sql').read_text()
                    self.sql(seed)
                    uid = 'original-awake-material-'+secrets.token_hex(5)
                    login = self.login(uid)
                    stamp = 96000

                    def http(name, fields, want):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, login, stamp, want)

                    def count(item):
                        return sum(v['num'] for v in (self.profile(login)[1].get('items') or []) if v['id'] == item)

                    def save(player, game):
                        p = json.dumps(player, ensure_ascii=False, separators=(',', ':')).encode().hex()
                        g = json.dumps(game, ensure_ascii=False, separators=(',', ':')).encode().hex()
                        self.sql(f'UPDATE profiles SET player=CONVERT(0x{p} USING utf8mb4),game=CONVERT(0x{g} USING utf8mb4) WHERE id={login.playerId};')

                    http('CSPlayerCreate', {'playerName': '觉醒材料原选择', 'heroId': 'hero_043'}, 'SCPlayerCreate')
                    player, game = self.profile(login)
                    self.assertEqual(player.get('coin', 0), 0)
                    self.assertEqual(count('item_330'), 0)
                    main_guid = player['heroWarId']
                    # Historical stars and initial raw awakening prerequisites are
                    # finite isolated preparation. Currency, GUIDs, stages, selected
                    # books and fusion below are all actual original API results.
                    with (ROOT/'analysis/tables/TableCopyMission.tsv').open() as source:
                        rows = list(csv.reader(source, delimiter='\t'))
                    missions = [dict(zip(rows[1], row)) for row in rows[5:] if row and row[0]]
                    player.update(level=100, noviceGuideSchedule=8)
                    game.setdefault('copy', {})['passedCopyInfo'] = [
                        {'chapterId': row['ChapterId'], 'missionId': row['MissionId'], 'star': 5,
                         'challengeTimes': 0, 'isOpen': 1}
                        for row in missions if row['ChapterId'] == '27']
                    game['copy']['passedChapterInfo'] = []
                    game['items'] = [{'id': key, 'num': value} for key, value in
                                     {'frag_hero_001': 1200, 'item_320': 2000,
                                      'item_313': 130, 'item_312': 45}.items()]
                    save(player, game)
                    coins = 0
                    for index in range(3):
                        reward = http('CSCopyGetStarAward', {'chapterId': '27', 'starIndex': index}, 'SCCopyGetStarAward')
                        coins += sum(v.num for v in reward.gain.simps if v.id == 'coin')
                    self.assertEqual(coins, 1205000)
                    self.assertEqual(self.profile(login)[0]['coin'], coins)
                    target = None
                    for _ in range(8):
                        combined = http('CSCombine', {'targetId': 'hero_001'}, 'SCCombine')
                        target = target or combined.get.wars[0].guid
                    player, game = self.profile(login)
                    next(v for v in game['warriors'] if v['guid'] == target)['level'] = 100
                    save(player, game)
                    http('CSWarriorTalent', {'guid': target, 'talent': '1',
                         'cost': [{'id': 'coin', 'num': 10000}, {'id': 'item_005', 'num': 30}]}, 'SCWarriorTalent')
                    for stage in (1, 2):
                        awakened = http('CSWarriorAwake', {'guid': target}, 'SCWarriorAwake')
                        self.assertEqual(awakened.newWarrior.awake.stage, stage)
                    self.assertEqual(count('item_312'), 0)
                    self.assertEqual(count('item_313'), 0)
                    self.assertEqual(count('frag_hero_001'), 0)
                    base = http('CSActivityShopInfo', {'activityId': directory}, 'SCActivityShopInfo')
                    self.assertFalse(any(v.getResource[0].id == 'item_330' for v in base.shopItemList))
                    self.sql("UPDATE p4_commerce_rules SET enabled=1 WHERE id='"+child+"';")
                    self.sql(seed)
                    listing = http('CSActivityShopInfo', {'activityId': directory}, 'SCActivityShopInfo')
                    self.assertEqual(list(listing.shopItemList[:-1]), list(base.shopItemList))
                    item = listing.shopItemList[-1]
                    self.assertEqual((item.name, [(v.id, v.num) for v in item.getResource],
                                      [(v.id, v.num) for v in item.costResource], item.total, item.left),
                                     ('圣诞礼物', [('item_330', 1)], [('coin', 1000000)], 1, 1))
                    args = {'activityId': directory, 'index': item.index, 'num': 1,
                            'getResources': [{'id': v.id, 'num': v.num} for v in item.getResource],
                            'costResources': [{'id': v.id, 'num': v.num} for v in item.costResource]}
                    stamp += 1
                    buy_stamp = stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        bought = list(workers.map(lambda _: self.call('CSBuyActivityShop', args, login,
                                                                     buy_stamp, 'SCGetActivityShopReward'), range(8)))
                    self.assertTrue(all(v == bought[0] for v in bought))
                    self.assertEqual(count('item_330'), 1)
                    self.assertEqual(self.profile(login)[0]['coin'], 195000)
                    used = http('CSItemUse', {'costItem': {'id': 'item_330', 'num': 1}, 'getItemIndex': 1}, 'SCItemUse')
                    self.assertEqual([(v.id, v.num) for v in used.get.simps], [('item_312', 2)])
                    self.assertEqual(count('item_330'), 0)
                    self.assertEqual(count('item_312'), 2)
                    fused = http('CSAwakeFuse', {'guid': target, 'cost': [{'id': 'item_312', 'num': 1}]}, 'SCAwakeFuse')
                    self.assertEqual(fused.newWarrior.guid, target)
                    self.assertEqual(fused.newWarrior.awake.stage, 2)
                    self.assertGreater(fused.newWarrior.awake.fuseLevel, 0)
                    self.assertEqual(count('item_312'), 1)
                    self.assertEqual(self.profile(login)[0]['heroWarId'], main_guid)
                    frozen = self.sql("SELECT HEX(frozen) FROM p4_commerce_rules WHERE id='"+child+"';")
                    self.sql("UPDATE p4_commerce_rules SET enabled=0,rules=JSON_SET(CONVERT(rules USING utf8mb4),'$.products[0].cost','coin,1') WHERE id='"+child+"';")
                    stable = self.profile(login)
                    self.stop()
                    self.start()
                    login = self.login(uid)
                    self.assertEqual(self.profile(login), stable)
                    self.assertEqual(self.call('CSBuyActivityShop', args, login, buy_stamp, 'SCGetActivityShopReward'), bought[0])
                    self.assertEqual(self.profile(login), stable)
                    off = http('CSActivityShopInfo', {'activityId': directory}, 'SCActivityShopInfo')
                    self.assertFalse(any(v.getResource[0].id == 'item_330' for v in off.shopItemList))
                    self.assertEqual(self.sql("SELECT HEX(frozen) FROM p4_commerce_rules WHERE id='"+child+"';"), frozen)
                    report.update(status='PASS_ORIGINAL_STAR_COIN_330_CHOICE_312_FUSION_COLD',
                                  actualStarCoin=coins, actualTalentCoin=10000, actualSupplyCoin=1000000,
                                  finalCoin=195000, actualChoice312=2, actualFusion312=1, final312=1,
                                  actualAwakeStages=[1, 2], actualMainGUIDUnchanged=True,
                                  actualTargetGUID=target, actualFuseLevel=fused.newWarrior.awake.fuseLevel,
                                  originalChoices='1:312x2;2:315x8;3:313x8; native chooses1, all3 Go verified',
                                  coldRestarts=1, samePurchaseWorkers=8, sourceDefaultOff=True,
                                  finitePrerequisites='CreatePlayer043; level100/guide8/chapter27 historical stars; raw1200 hero001fragments/2000element320/130proof313/45book312 then eight actual combines, talent and two actual awakens. No currency/GUID/stage/fusion/box injection. Actual bought312 is separate after all45 prior books consumed.',
                                  productionModified=False, phoneUI=False)
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
        for rel, digest in hashes.items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(), digest)
        report['seconds'] = round(time.monotonic()-began, 3)
        (ROOT/'analysis/p5/awake-material-supply-mysql-validation.json').write_text(
            json.dumps(report, ensure_ascii=False, indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
