"""Actual accepted Arena2 win -> local prize -> original box use, managed MySQL.

This suite stays opt-in. Request the one coordinated SQL window before execution;
all provisioning/cleanup goes through the local-services restricted allocator.
"""
import concurrent.futures
import datetime
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import unittest

import test_p10_mysql as transport
import test_p10_scene_tcp_mysql as native
import test_p11_arena_scope as arena
from hyjs_managed_mysql import ManagedMySQL

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P11_ARENA_PRIZE_MYSQL_TEST') == '1',
                     'explicit coordinated managed MySQL prize acceptance')
class P11ArenaPrizeMySQLTests(unittest.TestCase):
    send = transport.P10MySQLTests.send
    login = transport.P10MySQLTests.login
    stamped = arena.P11ArenaScopeTests.stamped
    call = transport.P10MySQLTests.call
    profile = transport.P10MySQLTests.profile
    start = native.P10SceneTCPTests.start
    stop = native.P10SceneTCPTests.stop
    setUp = transport.P10MySQLTests.setUp

    def sql(self, statement):
        return self.managed_pool.sql(self.database, statement)

    def test_original_arena_local_win_box_only_first_granted_and_cold(self):
        # A separate ready switch prevents accidental execution during shared
        # migration even when the domain opt-in was retained in a shell.
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1',
                         'wait for root shared-connection acceptance and one SQL window')
        self.source = Path(os.environ['HYJS_P11_ARENA_PRIZE_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent, ROOT/'build/candidates')
        self.assertIn('ApplyP11ArenaFightPrize',
                      (self.source/'hyjs-server/service/p11ArenaFightService.go').read_text())
        began = time.monotonic()
        runtime_paths = json.loads((ROOT/'analysis/p11/arena-prize-code-validation.json').read_text())['ownedFiles']
        runtime_hashes = {v['path']: hashlib.sha256((self.source/v['path']).read_bytes()).hexdigest() for v in runtime_paths}
        runtime_hashes['hyjs-server/service/p11ArenaFightService.go'] = hashlib.sha256((self.source/'hyjs-server/service/p11ArenaFightService.go').read_bytes()).hexdigest()
        helper_sha = hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper_sha, 'ec02759e488a72c40b08c2da629d7d2be2adcca26b5ce0f056f7b04135ad56bd', 'fixed temporary-account SQL helper required')
        report = {'status': 'PENDING', 'productionModified': False, 'phoneUI': False, 'managedHelperSHA256': helper_sha, 'runtimeSHA256': runtime_hashes}
        with tempfile.TemporaryDirectory(prefix='hyjs-arena-prize-native-') as temporary:
            self.binary = Path(temporary)/'arena-prize'
            # Lock only the resource lifecycle, after the single native compile.
            build = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', str(self.binary), '.'],
                                   cwd=self.source/'hyjs-server', capture_output=True,
                                   text=True, timeout=240)
            self.assertEqual(build.returncode, 0, (build.stdout+build.stderr)[-4000:])
            self.managed_pool = ManagedMySQL()
            allocation = None
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    allocation = self.managed_pool.allocate('p11_prize')
                    self.database = allocation.database
                    self.test_user = allocation.user
                    self.test_password = allocation.password
                    self.assertTrue(self.test_user.startswith('hyjs_t_'))
                    self.env = dict(os.environ, **allocation.env())
                    for name in ['147_p11_arena_prize_policies.sql',
                                 '147_p11_arena_prize_offers.sql',
                                 '147_p11_arena_prize_awards.sql',
                                 '147_p11_arena_prize_policies.seed.sql']:
                        self.sql((self.source/'hyjs-server/db/sql'/name).read_text())
                    self.start()
                    self.assertEqual(self.sql('SELECT enabled FROM p11_arena_prize_policies;').strip(), '0')
                    names = ['p11-real-prize-'+secrets.token_hex(5) for _ in range(2)]
                    defender, owner = [self.login(name) for name in names]
                    for login, label in [(defender, '原竞技低阶'), (owner, '原竞技奖箱')]:
                        self.call('CSPlayerCreate', {'playerName': label, 'heroId': 'hero_047'}, login, 1, 'SCPlayerCreate')
                    # Only finite isolated level qualification. Winner, rank,
                    # wallet, boxes, rewards and all main GUIDs come from APIs.
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',20) WHERE id={defender.playerId};UPDATE profiles SET player=JSON_SET(player,'$.level',100) WHERE id={owner.playerId};")
                    self.sql('UPDATE p11_arena_policies SET enabled=1;UPDATE p11_arena_fight_policies SET enabled=1;')
                    stamp = 200
                    def http(name, fields, login, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, login, stamp, expected)
                    def count(login, item):
                        return sum(x['num'] for x in (self.profile(login)[1].get('items') or []) if x['id'] == item)
                    def challenge(login, peer):
                        http('CSPvpFindMatch', {}, login, 'SCPvpFindMatch')
                        return http('CSPvpFightResult', {'playerId': peer.playerId, 'serverId': 1}, login, 'SCPvpFightResult')
                    before = self.profile(owner)
                    off = challenge(owner, defender)
                    self.assertEqual(off.winPlayerId, owner.playerId)
                    self.assertEqual([(v.id, v.num) for v in off.boxList], [('coin', 0)]*3)
                    self.assertEqual([(v.id, v.num) for v in off.result.simps], [('energy', -2)])
                    self.assertEqual(count(owner, 'item_027'), 0)
                    # Prepublish this exact local finite source. Distinct box
                    # quantities make accidental preview grants observable.
                    now_ms = int(time.time()*1000)
                    local = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)))
                    next_month = (local.replace(day=28)+datetime.timedelta(days=4)).replace(day=1,hour=0,minute=0,second=0,microsecond=0)
                    pool = json.dumps([{'id':'item_027','num':n,'weight':1} for n in [1,2,3]], separators=(',', ':'))
                    self.sql(f"UPDATE p11_arena_prize_policies SET enabled=1,version='local-native-three-distinct-quantities',published_at={now_ms-1},start_at={now_ms},end_at={int(next_month.timestamp()*1000)},min_level=10,daily_wins=1,window_wins=2,pool_json=CONVERT(0x{pool.encode().hex()} USING utf8mb4) WHERE id=1;")
                    http('CSPvpFindMatch', {}, owner, 'SCPvpFindMatch')
                    stamp += 1; win_stamp = stamp
                    args = {'playerId':defender.playerId,'serverId':1}
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        won = list(workers.map(lambda _:self.call('CSPvpFightResult',args,owner,win_stamp,'SCPvpFightResult'), range(8)))
                    result = won[0]
                    self.assertTrue(all(v == result for v in won))
                    self.assertEqual(result.winPlayerId, owner.playerId)
                    self.assertEqual(sorted(v.num for v in result.boxList), [1,2,3])
                    self.assertTrue(all(v.id == 'item_027' for v in result.boxList))
                    quantity = result.boxList[0].num
                    self.assertEqual([(v.id,v.num) for v in result.result.simps], [('energy',-2),('item_027',quantity)])
                    self.assertEqual(count(owner,'item_027'), quantity)
                    self.assertEqual(self.sql("SELECT COUNT(*) FROM p11_arena_prize_awards WHERE status='awarded';").strip(), '1')
                    self.assertEqual(http('CSPlayer',{},owner,'SCPlayer').player.heroWarId, before[0]['heroWarId'])
                    frozen = self.sql('SELECT source_hash,snapshot FROM p11_arena_prize_offers;')
                    self.sql("UPDATE p11_arena_prize_policies SET daily_wins=100,window_wins=100,pool_json='[{\"id\":\"coin\",\"num\":99,\"weight\":1}]';")
                    capped = challenge(owner,defender)
                    self.assertEqual(capped.winPlayerId,owner.playerId)
                    self.assertEqual([(v.id,v.num) for v in capped.boxList],[('coin',0)]*3)
                    self.assertEqual(count(owner,'item_027'),quantity)
                    self.assertEqual(self.sql('SELECT source_hash,snapshot FROM p11_arena_prize_offers;'),frozen)
                    lost = challenge(defender,owner)
                    self.assertEqual(lost.winPlayerId,owner.playerId)
                    self.assertEqual(len(lost.boxList),0)
                    self.assertEqual([(v.id,v.num) for v in lost.result.simps],[('energy',-2)])
                    use = http('CSItemUse',{'costItem':{'id':'item_027','num':1}},owner,'SCItemUse')
                    self.assertEqual(count(owner,'item_027'),quantity-1)
                    self.assertTrue(use.get.equips or use.get.simps)
                    for gear in use.get.equips:
                        self.assertTrue(gear.guid)
                        self.assertTrue(any(v['guid']==gear.guid for v in (self.profile(owner)[1].get('equipment') or [])))
                    # Original probabilities can return fragments. Keep the
                    # original draw and verify its actual inventory delta.
                    for item in use.get.simps:
                        self.assertGreaterEqual(count(owner,item.id),item.num)
                    after = self.profile(owner)
                    ledger = self.sql('SELECT fight_id,status,source_hash FROM p11_arena_prize_awards ORDER BY fight_id;')
                    self.sql('UPDATE p11_arena_prize_policies SET enabled=0;UPDATE p11_arena_fight_policies SET enabled=0;')
                    self.stop();self.start();owner=self.login(names[1])
                    self.assertEqual(self.call('CSPvpFightResult',args,owner,win_stamp,'SCPvpFightResult'),result)
                    self.assertEqual(self.profile(owner),after)
                    self.assertEqual(self.sql('SELECT fight_id,status,source_hash FROM p11_arena_prize_awards ORDER BY fight_id;'),ledger)
                    self.assertEqual(http('CSPlayer',{},owner,'SCPlayer').player.heroWarId,before[0]['heroWarId'])
                    report.update(status='PASS_REAL_ARENA_WIN_PRIZE_OPEN_COLD', finitePrerequisites='Original CreatePlayer047; isolated level100/20 only. No rank/winner/currency/box/GUID injection.', actualAward={'id':'item_027','num':quantity}, actualOriginalBoxResult=str(use.get), actualChallengerEnergySpent=6, actualDefenderEnergySpent=2, previewSum=6, originalPreviewGranted=False, coldRestarts=1, frozenOfferSHA256=hashlib.sha256(frozen.encode()).hexdigest(), sqlAccountScope='Application and every fixture SQL use allocated hyjs_t_ temporary account, one managed random hyjs_test_ database only; password stdin; no project/root account')
                finally:
                    try:
                        self.stop()
                    finally:
                        try:
                            if allocation is not None:
                                self.managed_pool.release(allocation.database)
                                report['ownedAllocationRemoved']=True
                        finally:
                            self.managed_pool.close()
        for rel, expected_sha in runtime_hashes.items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(), expected_sha, 'native source changed during run')
        report['seconds']=round(time.monotonic()-began,3)
        (ROOT/'analysis/p11/arena-prize-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
