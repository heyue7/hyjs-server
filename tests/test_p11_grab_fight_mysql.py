"""Actual occupied-slot SCGrab19, atomic original costs and cold receipt recovery.

Explicit isolated prerequisites only: two real originalCreate047 players at
original eligibility30/40 and main20/40. No result/slot/session/wallet injection.
This runner always reuses a source-pinned prebuilt; it never compiles during SQL.
"""
import concurrent.futures
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


@unittest.skipUnless(os.environ.get('HYJS_P11_GRAB_FIGHT_MYSQL_TEST') == '1',
                     'explicit root-coordinated independent19 SQL window')
class P11GrabFightMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql

    def test_actual_occupied_source_costs_once_transfer_or_retention_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source = Path(os.environ['HYJS_P11_GRAB_FIGHT_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent, ROOT/'build/candidates')
        proof_path = ROOT/'analysis/p11/grab-fight-prebuilt-proof.json'
        proof = json.loads(proof_path.read_text())
        self.assertEqual(proof['status'], 'PASS_DOMAIN_ALLPKG_VET_BUILD')
        self.assertTrue(proof['mode19IndependentProof'])
        for rel, expected in proof['runtimeSHA256'].items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(), expected, rel)
        prebuilt = Path(os.environ['HYJS_P11_GRAB_FIGHT_PREBUILT_BINARY']).resolve()
        self.assertEqual(prebuilt.parent, self.source)
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(), proof['binarySHA256'])
        helper = hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper, '483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        began = time.monotonic()
        report = dict(status='PENDING', sourceProofSHA256=hashlib.sha256(proof_path.read_bytes()).hexdigest(),
                      managedHelperSHA256=helper, formalModified=False, phoneUI=False)
        checkpoint = ROOT/'analysis/p11/grab-fight-native-checkpoint.json'
        with tempfile.TemporaryDirectory(prefix='hyjs-grab-fight-native-') as temporary:
            self.binary = Path(temporary)/'grab-fight-native'
            shutil.copy2(prebuilt, self.binary)
            self.managed_pool = ManagedMySQL()
            allocation = None
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    allocation = self.managed_pool.allocate('p11_grab_fight')
                    self.database, self.test_user = allocation.database, allocation.user
                    self.test_password = allocation.password
                    self.env = dict(os.environ, **allocation.env(), GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.assertEqual(self.sql('SELECT @@character_set_client;').strip(), 'utf8mb4')
                    for filename in ['168_p11_grab_fight_policies.sql', '168_p11_grab_fight_offers.sql',
                                     '168_p11_grab_fight_settlements.sql', '168_p11_grab_fight_policies.seed.sql']:
                        self.sql((self.source/'hyjs-server/db/sql'/filename).read_text())
                    self.start()
                    self.assertEqual(self.sql('SELECT enabled FROM p11_grab_fight_policies;').strip(), '0')
                    identities = ['grab19-real-'+secrets.token_hex(5) for _ in range(2)]
                    actors = [self.login(v) for v in identities]
                    stamp = 170000

                    def http(name, fields, actor, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, actor, stamp, expected)

                    for actor, hero_level in zip(actors, [40, 20]):
                        made = http('CSPlayerCreate', {'playerName':'原争夺'+str(hero_level), 'heroId':'hero_047'}, actor, 'SCPlayerCreate')
                        player, game = self.profile(actor)
                        self.assertEqual((player.get('coin', 0), player.get('diamond', 0)), (0, 0))
                        player.update(level=max(30, hero_level), noviceGuideSchedule=8)
                        main = next(v for v in game['warriors'] if v['guid'] == made.player.heroWarId)
                        main.update(level=hero_level, exp=0)
                        pr = json.dumps(player, ensure_ascii=False, separators=(',', ':')).encode().hex()
                        gr = json.dumps(game, ensure_ascii=False, separators=(',', ':')).encode().hex()
                        self.sql(f'UPDATE profiles SET player=CONVERT(0x{pr} USING utf8mb4),game=CONVERT(0x{gr} USING utf8mb4) WHERE id={actor.playerId};')
                    strong, weak = actors
                    self.sql('UPDATE p11_grab_policies SET enabled=1;')
                    original_owner = http('CSGrab', {'index':1}, strong, 'SCGrab')
                    self.assertFalse(original_owner.HasField('attPlayer'))
                    self.assertEqual(original_owner.grabData.playerId, strong.playerId)
                    initial = [self.profile(a) for a in actors]
                    slot_before = self.sql('SELECT * FROM p11_grab_slots WHERE position=1;')
                    stamp += 1
                    rejected = self.stamped('CSGrab', {'index':1}, weak, stamp)
                    self.assertEqual(rejected[0], 10001)
                    self.assertEqual([self.profile(a) for a in actors], initial)
                    self.assertEqual(self.sql('SELECT * FROM p11_grab_slots WHERE position=1;'), slot_before)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_grab_fight_settlements;').strip(), '0')
                    now = int(time.time()*1000)
                    at = now+600
                    self.sql(f"UPDATE p11_grab_fight_policies SET enabled=1,version='local-native-grab19-v1',published_at={now},starts_at={at},ends_at={at+600000} WHERE id=1;")
                    time.sleep(max(0, at/1000-time.time())+0.02)
                    accepted = []
                    for index, owner, challenger in [(1, strong, weak), (2, weak, strong)]:
                        if index == 2:
                            last = accepted[-1][2]
                            actual_owner = next(a for a in actors if a.playerId == last.grabData.playerId)
                            http('CSGiveUpGrab', {'index':1}, actual_owner, 'SCGiveUpGrab')
                            claimed = http('CSGrab', {'index':2}, owner, 'SCGrab')
                            self.assertFalse(claimed.HasField('attPlayer'))
                        owner_before = self.profile(owner)
                        challenger_before = self.profile(challenger)
                        previous = self.sql(f'SELECT session_id,started_at,expires_at FROM p11_grab_slots WHERE position={index};').strip().split('\t')
                        stamp += 1
                        fight_stamp = stamp
                        args = {'index':index}
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            replies = list(workers.map(lambda _:self.call('CSGrab', args, challenger, fight_stamp, 'SCGrab'), range(8)))
                        fight = replies[0]
                        self.assertTrue(all(v == fight for v in replies))
                        self.assertTrue(fight.HasField('attPlayer') and fight.HasField('defPlayer'))
                        self.assertEqual(fight.attPlayer.playerId, challenger.playerId)
                        self.assertEqual(fight.defPlayer.playerId, owner.playerId)
                        self.assertEqual(fight.maxRoundNum, 40)
                        self.assertGreater(fight.randSeed, 0)
                        self.assertTrue(fight.fightId)
                        self.assertIn(fight.winPlayerId, [owner.playerId, challenger.playerId])
                        won = fight.winPlayerId == challenger.playerId
                        self.assertEqual(fight.grabData.playerId, challenger.playerId if won else owner.playerId)
                        self.assertEqual(self.profile(challenger)[0].get('energy', 0), challenger_before[0].get('energy', 0)-10)
                        self.assertEqual(fight.currEnergy, challenger_before[0].get('energy', 0)-10)
                        delta = self.sql(f'SELECT before_energy,after_energy,before_used,after_used,before_remaining,after_remaining FROM p11_grab_fight_settlements WHERE player_id={challenger.playerId} AND stamp={fight_stamp};').strip().split('\t')
                        self.assertEqual(len(delta), 6)
                        be, ae, bu, au, br, ar = map(int, delta)
                        self.assertEqual((be-ae, au-bu, br-ar), (10, 1, 1))
                        self.assertEqual(fight.grabTimes, ar)
                        self.assertEqual(self.profile(owner), owner_before)
                        ended = self.sql(f"SELECT ended_at,end_reason FROM p11_grab_sessions WHERE id='{previous[0]}';").strip().split('\t')
                        self.assertEqual(int(ended[0]) > 0, won)
                        if not won:
                            self.assertEqual(self.sql(f'SELECT session_id,started_at,expires_at FROM p11_grab_slots WHERE position={index};').strip().split('\t'), previous)
                        accepted.append((challenger, fight_stamp, fight))
                        report.setdefault('actualResults', []).append(dict(index=index, winner=fight.winPlayerId, challengerWon=won, energyCost=10, eightOnce=True))
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_grab_fight_settlements;').strip(), '2')
                    self.assertEqual(self.sql("SELECT COUNT(*) FROM p6_battle_records WHERE source='p11-grab-occupied-original19' AND authoritative=1;").strip(), '2')
                    journal = self.sql('SELECT player_id,stamp,defender_id,position,day,request_hash,previous_session_id,previous_started_at,previous_expires_at,new_session_id,offer_version,offer_hash,fight_id,p6_record_id,scope_proof,before_energy,after_energy,before_remaining,after_remaining,before_used,after_used,won,HEX(start_pb),start_hash,HEX(terminal),terminal_hash,HEX(response),response_hash,created_at FROM p11_grab_fight_settlements ORDER BY player_id,stamp;')
                    after = [self.profile(a) for a in actors]
                    self.sql('UPDATE p11_grab_policies SET enabled=0; UPDATE p11_grab_fight_policies SET enabled=0;')
                    self.stop(); self.start()
                    actors = [self.login(v) for v in identities]
                    for old_actor, old_stamp, old_reply in accepted:
                        actor = next(a for a in actors if a.playerId == old_actor.playerId)
                        self.assertEqual(self.call('CSGrab', {'index':old_reply.grabData.position}, actor, old_stamp, 'SCGrab'), old_reply)
                    self.assertEqual([self.profile(a) for a in actors], after)
                    self.assertEqual(self.sql('SELECT player_id,stamp,defender_id,position,day,request_hash,previous_session_id,previous_started_at,previous_expires_at,new_session_id,offer_version,offer_hash,fight_id,p6_record_id,scope_proof,before_energy,after_energy,before_remaining,after_remaining,before_used,after_used,won,HEX(start_pb),start_hash,HEX(terminal),terminal_hash,HEX(response),response_hash,created_at FROM p11_grab_fight_settlements ORDER BY player_id,stamp;'), journal)
                    report.update(status='PASS_ACTUAL_OCCUPIED19_COST_ONCE_OWNERSHIP_AND_COLD',
                                  finitePrerequisites='Only realCreate047 plus isolated player30/40 main20/40 guide8; actual empty claim/real current owner before fight. No seed/winner/slot/session/fee/HP edit.',
                                  cold=1, acceptedBattles=2, rejectedDefaultOffNoCharge=True)
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
        (ROOT/'analysis/p11/grab-fight-mysql-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
