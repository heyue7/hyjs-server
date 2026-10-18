"""Original23013/23105 Guide frames -> actual type2 -> progress13/cold.

One explicit root-coordinated managed SQL window; fixed prebuilt only. Three
original created players supply the original guide's two actual list targets.
"""
import concurrent.futures
import fcntl
import gzip
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


@unittest.skipUnless(os.environ.get('HYJS_P11_ARENA_GUIDE_MYSQL_TEST') == '1',
                     'explicit coordinated original Guide managed SQL window')
class P11ArenaGuideMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql

    def test_original_guide_query_actual_two_peers_type2_progress13_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source = Path(os.environ['HYJS_P11_ARENA_GUIDE_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent, ROOT/'build/candidates')
        proof = json.loads((ROOT/'analysis/p11/arena-guide-prebuilt-proof.json').read_text())
        self.assertEqual(proof['status'], 'PASS_DOMAIN_ALLPKG_VET_BUILD')
        for rel, expected in proof['runtimeSHA256'].items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(), expected, rel)
        prebuilt = Path(os.environ['HYJS_P11_ARENA_GUIDE_PREBUILT_BINARY']).resolve()
        self.assertEqual(prebuilt.parent, self.source)
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(), proof['binarySHA256'])
        helper = hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper, '483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        began = time.monotonic()
        report = dict(status='PENDING', prebuiltProof=proof, managedHelperSHA256=helper,
                      formalModified=False, phoneUI=False)
        checkpoint = ROOT/'analysis/p11/arena-guide-native-checkpoint.json'
        with tempfile.TemporaryDirectory(prefix='hyjs-arena-guide-native-') as temporary:
            self.binary = Path(temporary)/'guide-native'
            shutil.copy2(prebuilt, self.binary)
            self.managed_pool = ManagedMySQL()
            allocation = None
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    allocation = self.managed_pool.allocate('p11_arena_guide')
                    self.database, self.test_user = allocation.database, allocation.user
                    self.test_password = allocation.password
                    self.env = dict(os.environ, **allocation.env(), GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    identities = ['original-guide-'+secrets.token_hex(5) for _ in range(3)]
                    actors = [self.login(v) for v in identities]
                    stamp = 170000

                    def http(name, fields, actor, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, actor, stamp, expected)

                    for index, actor in enumerate(actors):
                        made = http('CSPlayerCreate', {'playerName':'原竞技引导'+str(index), 'heroId':'hero_047'}, actor, 'SCPlayerCreate')
                        player, game = self.profile(actor)
                        self.assertEqual((player.get('coin', 0), player.get('diamond', 0)), (0, 0))
                        player.update(level=10, noviceGuideSchedule=12)
                        main = next(v for v in game['warriors'] if v['guid'] == made.player.heroWarId)
                        self.assertEqual(main.get('level'), 1)
                        pr = json.dumps(player, ensure_ascii=False, separators=(',', ':')).encode().hex()
                        self.sql(f'UPDATE profiles SET player=CONVERT(0x{pr} USING utf8mb4) WHERE id={actor.playerId};')
                    owner, peer, other = actors
                    self.sql('UPDATE p11_arena_policies SET enabled=1;UPDATE p11_arena_fight_policies SET enabled=1;')
                    # This is the original Guide request and expected response,
                    # not a modern frame with its message identifier relabeled.
                    listing = http('CSPvpFindMatchGuide', {}, owner, 'SCPvpFindMatchGuide')
                    self.assertEqual({v.info.playerId for v in listing.rankList}, {peer.playerId, other.playerId})
                    self.assertEqual(len(listing.rankList), 2)
                    self.assertTrue(all(v.info.heroWarId == 'hero_047' for v in listing.rankList))
                    before = [self.profile(v) for v in actors]
                    old_ranks = {int(a):int(b) for a, b in [v.split('\t') for v in self.sql('SELECT player_id,position FROM p11_arena_members ORDER BY player_id;').strip().splitlines()]}
                    args = {'playerId':peer.playerId, 'serverId':1}
                    for bad_stamp in [0, -1]:
                        self.assertEqual(self.stamped('CSPvpFightResultGuide', args, owner, bad_stamp)[0], 10001)
                    stamp += 1
                    paid_stamp = stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        results = list(workers.map(lambda _:self.call('CSPvpFightResultGuide', args, owner, paid_stamp, 'SCPvpFightResultGuide'), range(8)))
                    fight = results[0]
                    self.assertTrue(all(v == fight for v in results))
                    self.assertIn(fight.winPlayerId, [owner.playerId, peer.playerId])
                    self.assertEqual((fight.roomId, fight.maxRoundNum), ('map_arena_001', 40))
                    self.assertEqual([v.playerId for v in fight.playerList], [owner.playerId, peer.playerId])
                    self.assertEqual([v.inst.firstOrder for v in fight.playerList], [1, 0])
                    for actor, source in zip([owner, peer], fight.playerList):
                        self.assertEqual(len(source.formation), 1)
                        self.assertEqual(source.formation[0].guid, self.profile(actor)[0]['heroWarId'])
                        self.assertEqual(source.formation[0].skillOrder, '047_1,047_2,047_3')
                        self.assertEqual(source.formation[0].inst.curHp, source.formation[0].prop.hp)
                    now = self.profile(owner)
                    self.assertEqual(now[0].get('energy', 0), before[0][0].get('energy', 0)-2)
                    self.assertEqual(now[0].get('noviceGuideSchedule'), 13)
                    self.assertEqual(self.profile(peer), before[1])
                    self.assertEqual(self.profile(other), before[2])
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_fight_settlements;').strip(), '1')
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p6_battle_records;').strip(), '1')
                    new_ranks = {int(a):int(b) for a, b in [v.split('\t') for v in self.sql('SELECT player_id,position FROM p11_arena_members ORDER BY player_id;').strip().splitlines()]}
                    expected_ranks = dict(old_ranks)
                    if fight.winPlayerId == owner.playerId and old_ranks[owner.playerId] > old_ranks[peer.playerId]:
                        expected_ranks[owner.playerId], expected_ranks[peer.playerId] = old_ranks[peer.playerId], old_ranks[owner.playerId]
                    self.assertEqual(new_ranks, expected_ranks)
                    source_fact = self.sql('SELECT HEX(input) FROM p6_battle_records;').strip()
                    stored = bytes.fromhex(source_fact)
                    decoded = gzip.decompress(stored) if stored.startswith(b'\x1f\x8b') else stored
                    self.assertIn('original23105', decoded.decode())
                    stamp += 1
                    self.assertEqual(self.stamped('CSPvpFightResultGuide', args, owner, stamp)[0], 10001)
                    self.assertEqual(self.profile(owner), now)
                    # Completed Guide retains its already accepted original body;
                    # no fresh charge or completion is derived from an old rank.
                    self.sql('UPDATE p11_arena_policies SET enabled=0;UPDATE p11_arena_fight_policies SET enabled=0;')
                    self.stop(); self.start()
                    owner = self.login(identities[0])
                    self.assertEqual(self.call('CSPvpFightResultGuide', args, owner, paid_stamp, 'SCPvpFightResultGuide'), fight)
                    self.assertEqual(self.profile(owner), now)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_fight_settlements;').strip(), '1')
                    self.assertEqual(self.sql('SELECT HEX(input) FROM p6_battle_records;').strip(), source_fact)
                    report.update(status='PASS_ORIGINAL23013_23105_ACTUAL_TYPE2_GUIDE13_COST_RANK_COLD', actualWinner=fight.winPlayerId, originalRequestIDs=[23013,23105], originalResponseIDs=[23014,23106], actualEnergyCost=2, actualPeers=2, currentSchedule=13, oldRanks=old_ranks, newRanks=new_ranks, finitePrerequisites='OriginalCreate047×3; isolated Playerlevel10 and previous guide completion12 only. Main remains original level1/GUID/three skills; no NPC, panel, wallet, result, rank or resource injection.', callbackScope='Original Guide forced attfirst matches existing original type2 only at complete Inst1/0; no Guide original-machine observation claimed.')
                except BaseException as error:
                    report.update(status='FAIL', failure=self.managed_pool.redact(str(error)))
                    raise
                finally:
                    try:
                        self.stop()
                    finally:
                        if allocation is not None:
                            self.managed_pool.release(allocation.database)
                        self.managed_pool.close()
                    report.update(managedAllocationsRemoved=True, seconds=round(time.monotonic()-began, 3))
                    checkpoint.write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
        (ROOT/'analysis/p11/arena-guide-mysql-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
