"""Accepted original82057 -> original daily score -> tier2 -> item002/cold.

Opt-in only, after one root-coordinated Go/SQL lease. The fixed prebuilt must
contain an independently certified mode20 and the managed-schema-only cutoff
clock. The clock affects daily cutoff only; actor, P6 and entry clocks stay real.
"""
import concurrent.futures
import datetime
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


@unittest.skipUnless(os.environ.get('HYJS_P11_LIMIT_DAILY_MYSQL_TEST') == '1',
                     'explicit root-coordinated accepted mode20 daily window')
class P11LimitDailyMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql

    def test_actual_challenger_cutoff_original_score_tier2_use_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source = Path(os.environ['HYJS_P11_LIMIT_DAILY_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent, ROOT/'build/candidates')
        proof = json.loads((ROOT/'analysis/p11/limit-daily-prebuilt-proof.json').read_text())
        self.assertEqual(proof['status'], 'PASS_DOMAIN_ALLPKG_VET_BUILD')
        self.assertTrue(proof['mode20IndependentProof'])
        self.assertTrue(proof['managedDailyClockOnly'])
        for rel, expected in proof['runtimeSHA256'].items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(), expected, rel)
        prebuilt = Path(os.environ['HYJS_P11_LIMIT_DAILY_PREBUILT_BINARY']).resolve()
        self.assertEqual(prebuilt.parent, self.source)
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(), proof['binarySHA256'])
        helper = hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper, '483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        began = time.monotonic()
        report = dict(status='PENDING', sourceProof=proof, managedHelperSHA256=helper,
                      formalModified=False, phoneUI=False)
        checkpoint = ROOT/'analysis/p11/limit-daily-native-checkpoint.json'
        with tempfile.TemporaryDirectory(prefix='hyjs-limit-daily-native-') as temporary:
            self.binary = Path(temporary)/'daily-native'
            shutil.copy2(prebuilt, self.binary)
            clock = Path(temporary)/'cutoff-clock'
            clock.write_text('0')
            self.managed_pool = ManagedMySQL()
            allocation = None
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    allocation = self.managed_pool.allocate('p11_limit_daily')
                    self.database, self.test_user = allocation.database, allocation.user
                    self.test_password = allocation.password
                    self.env = dict(os.environ, **allocation.env(), GOMEMLIMIT='768MiB',
                                    GOMAXPROCS='2', HYJS_TEST_LIMIT_DAILY_CLOCK=str(clock))
                    self.assertEqual(self.sql('SELECT @@character_set_client;').strip(), 'utf8mb4')
                    for domain, suffixes in [
                        ('163_p11_limit_fight_', ['policies.sql', 'offers.sql', 'matches.sql', 'settlements.sql', 'policies.seed.sql']),
                        ('164_p11_limit_daily_', ['policies.sql', 'offers.sql', 'challenges.sql', 'snapshots.sql', 'awards.sql', 'policies.seed.sql']),
                    ]:
                        for suffix in suffixes:
                            self.sql((self.source/'hyjs-server/db/sql'/(domain+suffix)).read_text())
                    self.start()
                    self.assertEqual(self.sql('SELECT enabled FROM p11_limit_daily_policies;').strip(), '0')
                    identities = ['daily-actual-'+secrets.token_hex(5) for _ in range(2)]
                    actors = [self.login(v) for v in identities]
                    stamp = 150000

                    def http(name, fields, actor, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, actor, stamp, expected)

                    def count(actor, resource):
                        return sum(v['num'] for v in (self.profile(actor)[1].get('items') or []) if v['id'] == resource)

                    for actor, level in zip(actors, [40, 20]):
                        made = http('CSPlayerCreate', {'playerName':'原日榜'+str(level), 'heroId':'hero_047'}, actor, 'SCPlayerCreate')
                        player, game = self.profile(actor)
                        self.assertEqual((player.get('coin', 0), player.get('diamond', 0)), (0, 0))
                        player.update(level=level, noviceGuideSchedule=8)
                        main = next(v for v in game['warriors'] if v['guid'] == made.player.heroWarId)
                        main.update(level=level, exp=0)
                        pr = json.dumps(player, ensure_ascii=False, separators=(',', ':')).encode().hex()
                        gr = json.dumps(game, ensure_ascii=False, separators=(',', ':')).encode().hex()
                        self.sql(f'UPDATE profiles SET player=CONVERT(0x{pr} USING utf8mb4),game=CONVERT(0x{gr} USING utf8mb4) WHERE id={actor.playerId};')
                    owner, peer = actors
                    now = int(time.time()*1000)
                    at = (now//1000+3)*1000
                    end = at+86400000
                    local = datetime.datetime.fromtimestamp(at/1000, datetime.timezone(datetime.timedelta(hours=8)))
                    cutoff = local.hour*3600+local.minute*60+local.second
                    period = 'daily-native-'+secrets.token_hex(4)
                    self.sql(f"UPDATE p11_limit_policies SET enabled=1,reward_enabled=1,period_id='{period}',starts_at={now-60000},ends_at={end+86400000} WHERE id=1;")
                    for actor in actors:
                        entry = http('CSLimitPvpFindMatch', {}, actor, 'SCLimitPvpFindMatch')
                        self.assertEqual((entry.myrank.pvpIntegral, entry.fightTimes, entry.allTimes), (10, 0, 10))
                    self.sql(f"UPDATE p11_limit_fight_policies SET enabled=1,version='daily-actual-mode20-v1',published_at={now},start_at={at},end_at={end} WHERE id=1;")
                    self.sql(f"UPDATE p11_limit_daily_policies SET enabled=1,version='local-native-original-seven-ranks',published_at={now},start_at={at},end_at={end},cutoff_second={cutoff},min_challenges=1 WHERE id=1;")
                    time.sleep(max(0, at/1000-time.time())+0.02)
                    found = http('CSLimitPvpFindMatch', {}, owner, 'SCLimitPvpFindMatch')
                    self.assertEqual([v.info.playerId for v in found.rankList], [peer.playerId])
                    offer = self.sql('SELECT id,source_hash,rules_json FROM p11_limit_daily_offers;')
                    self.assertTrue(offer.strip())
                    before = [self.profile(v) for v in actors]
                    args = {'playerId':peer.playerId, 'serverId':1}
                    stamp += 1
                    fight_stamp = stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        fights = list(workers.map(lambda _:self.call('CSLimitPvpFightResult', args, owner, fight_stamp, 'SCLimitPvpFightResult'), range(8)))
                    fight = fights[0]
                    self.assertTrue(all(v == fight for v in fights))
                    self.assertIn(fight.winPlayerId, [v.playerId for v in actors])
                    self.assertEqual(self.profile(owner)[0].get('energy', 0), before[0][0].get('energy', 0)-2)
                    self.assertEqual(self.profile(peer), before[1])
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_limit_fight_settlements;').strip(), '1')
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_limit_daily_challenges;').strip(), '1')
                    self.assertEqual(self.sql('SELECT used FROM p11_limit_daily WHERE player_id='+str(owner.playerId)+';').strip(), '1')
                    delta = 5 if fight.winPlayerId == owner.playerId else 0
                    self.assertEqual(self.sql('SELECT score FROM p11_limit_members WHERE player_id='+str(owner.playerId)+';').strip(), str(10+delta))
                    # Disable the current catalog and change the live threshold;
                    # the actual first offer keeps its original seven/min1 terms.
                    self.sql('UPDATE p11_limit_daily_policies SET enabled=0,min_challenges=99;')
                    self.assertEqual(self.sql('SELECT id,source_hash,rules_json FROM p11_limit_daily_offers;'), offer)
                    self.stop()
                    clock.write_text(str(end+1))
                    self.start()
                    deadline = time.monotonic()+20
                    while time.monotonic() < deadline:
                        if self.sql('SELECT COUNT(*) FROM p11_limit_daily_snapshots;').strip() == '1':
                            break
                        time.sleep(0.05)
                    self.assertEqual(self.sql("SELECT CONCAT(status,':',HEX(reason)) FROM p11_limit_daily_snapshots;").strip(), 'complete:')
                    award = self.sql('SELECT player_id,`rank`,bonus,before_score,after_score FROM p11_limit_daily_awards;').strip().split('\t')
                    self.assertEqual(len(award), 5)
                    player_id, rank, bonus, old_score, new_score = map(int, award)
                    self.assertEqual(player_id, owner.playerId)
                    self.assertEqual(bonus, 50 if rank == 1 else 40)
                    self.assertEqual((old_score, new_score), (10+delta, 10+delta+bonus))
                    self.assertEqual(self.sql("SELECT COUNT(*) FROM p1_event_facts WHERE kind='limit_arena_daily_score';").strip(), '1')
                    ledger = self.sql('SELECT * FROM p11_limit_daily_awards;')
                    owner, peer = [self.login(v) for v in identities]
                    listing = http('CSLimitPvpRewardInfo', {}, owner, 'SCLimitPvpRewardInfo')
                    tier = next(v for v in listing.items if v.rewardKey == 2)
                    self.assertEqual((tier.integral, tier.state), (30, 1))
                    stamp += 1
                    reward_stamp = stamp
                    reward_args = {'itemId':'2'}
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        rewards = list(workers.map(lambda _:self.call('CSGetLimitPvpRewardInfo', reward_args, owner, reward_stamp, 'SCGetLimitPvpRewardInfo'), range(8)))
                    self.assertTrue(all(v == rewards[0] for v in rewards))
                    self.assertEqual(sorted((v.id, v.num) for v in rewards[0].get.simps), [('frag_hero_082', 5), ('item_002', 2)])
                    self.assertEqual((count(owner, 'frag_hero_082'), count(owner, 'item_002')), (5, 2))
                    stamina = self.profile(owner)[0].get('stamina', 0)
                    used = http('CSItemUse', {'costItem':{'id':'item_002','num':1}}, owner, 'SCItemUse')
                    self.assertEqual([(v.id, v.num) for v in used.get.simps], [('stamina', 25)])
                    self.assertEqual(self.profile(owner)[0].get('stamina', 0), stamina+25)
                    self.assertEqual(count(owner, 'item_002'), 1)
                    after = self.profile(owner)
                    self.stop(); self.start()
                    owner = self.login(identities[0])
                    self.assertEqual(self.call('CSGetLimitPvpRewardInfo', reward_args, owner, reward_stamp, 'SCGetLimitPvpRewardInfo'), rewards[0])
                    self.assertEqual(self.call('CSLimitPvpFightResult', args, owner, fight_stamp, 'SCLimitPvpFightResult'), fight)
                    self.assertEqual(self.profile(owner), after)
                    self.assertEqual(self.sql('SELECT * FROM p11_limit_daily_awards;'), ledger)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_limit_daily_snapshots;').strip(), '1')
                    report.update(status='PASS_ACTUAL_MODE20_DAILY_CUTOFF_ORIGINAL30_REWARD_ITEM002_COLD', actualWinner=fight.winPlayerId, actualChallengeScore=delta, actualEnergyCost=2, dailyAward=dict(rank=rank, bonus=bonus, before=old_score, after=new_score), tier2='frag_hero_082x5;item_002x2', actualItem002Stamina=25, dailyAdmissionRows=1, cold=2, finitePrerequisites='Only originalCreate047 and isolated originalPlayer/main40vs20 with guide8; real period entry10. No wallet/score/rank/result/item injection. Only daily cutoff clock moves; actor/P6/entry time stays real.')
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
        (ROOT/'analysis/p11/limit-daily-mysql-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
