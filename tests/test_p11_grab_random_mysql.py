"""Actual original occupation ->TCP red79 ->82053 award ->item use/cold.

Only run during the single root-coordinated managed SQL window. The isolated
published policy uses a real one-second local interval; no session time is edited.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import tempfile
import time
import unittest

from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME
from test_p11_arena_prize_mysql import P11ArenaPrizeMySQLTests as Transport
from test_p10_mysql import P10MySQLTests as Socket

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P11_GRAB_RANDOM_MYSQL_TEST') == '1',
                     'explicit coordinated managed SQL window')
class P11GrabRandomMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql
    connect = Socket.connect
    recv = Socket.recv
    tcp_send = Socket.tcp_send

    def test_original_occupation_redpoint_tcp_claim_use_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source = Path(os.environ['HYJS_P11_GRAB_RANDOM_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent, ROOT/'build/candidates')
        owned = json.loads((ROOT/'analysis/p11/grab-random-code-manifest.json').read_text())
        shared = json.loads((ROOT/'analysis/p11/grab-random-root-shared-resume-20261015/manifest.json').read_text())
        hashes = {}
        for row in owned['files']+shared['files']:
            digest = hashlib.sha256((self.source/row['path']).read_bytes()).hexdigest()
            self.assertEqual(digest, row['sha256'], row['path'])
            hashes[row['path']] = digest
        helper = hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper, '483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        began = time.monotonic()
        report = {'status': 'PENDING', 'runtimeSHA256': hashes, 'managedHelperSHA256': helper}
        checkpoint = ROOT/'analysis/p11/grab-random-native-checkpoint.json'

        def save_checkpoint():
            checkpoint.write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')

        const_source = (self.source/'hyjs-server/constUtil/p11GrabRandom.go').read_text()
        expected_source = json.loads(re.search(r'P11GrabRandomSource\s*=\s*("[^\n]+")', const_source)[1])
        with tempfile.TemporaryDirectory(prefix='hyjs-grab-random-') as temporary:
            self.binary = Path(temporary)/'grab-random-native'
            prebuilt = os.environ.get('HYJS_P11_GRAB_RANDOM_PREBUILT_BINARY')
            if prebuilt:
                build_proof = json.loads((ROOT/'analysis/p11/grab-random-go-validation.json').read_text())
                original_binary = Path(prebuilt).resolve()
                self.assertEqual(original_binary, self.source/'grab-random-native')
                self.assertEqual(build_proof['status'], 'PASS')
                for rel, digest in build_proof['ownedGoSHA256'].items():
                    self.assertEqual(hashes[rel], digest, 'prebuilt Go source '+rel)
                self.assertEqual(hashlib.sha256(original_binary.read_bytes()).hexdigest(), build_proof['binarySHA256'])
                shutil.copy2(original_binary, self.binary)
            else:
                built = subprocess.run(['rtk', 'proxy', 'go', 'build', '-p', '1', '-o', str(self.binary), '.'],
                                       cwd=self.source/'hyjs-server', capture_output=True, text=True,
                                       env=dict(os.environ, GOMEMLIMIT='768MiB'), timeout=300)
                self.assertEqual(built.returncode, 0, (built.stdout+built.stderr)[-4000:])
            self.managed_pool = ManagedMySQL()
            connection = None
            sock = None
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    connection = self.managed_pool.allocate('p11_grab_random')
                    self.database = connection.database
                    self.test_user = connection.user
                    self.test_password = connection.password
                    self.assertTrue(self.test_user.startswith('hyjs_t_'))
                    self.env = dict(os.environ, **connection.env(), GOMEMLIMIT='768MiB')
                    charset = self.sql('SELECT @@character_set_client,@@character_set_connection,@@character_set_results;').strip().split('\t')
                    report['fixtureSessionCharsets'] = charset
                    save_checkpoint()
                    self.assertEqual(charset, ['utf8mb4']*3)
                    for filename in ['153_p11_grab_random_policies.sql', '153_p11_grab_random_offers.sql',
                                     '153_p11_grab_random_awards.sql', '153_p11_grab_random_policies.seed.sql']:
                        self.sql((self.source/'hyjs-server/db/sql'/filename).read_text())
                    seed = json.loads(self.sql("SELECT JSON_OBJECT('enabled',enabled,'version',version,'publishedAt',published_at,'startAt',start_at,'endAt',end_at,'intervalSeconds',interval_seconds,'dailyLimit',daily_limit,'sourceHEX',HEX(source)) FROM p11_grab_random_policies WHERE id=1;"))
                    report['canonicalSeed'] = seed
                    report['expectedSourceHEX'] = expected_source.encode().hex().upper()
                    save_checkpoint()
                    self.assertEqual(seed, dict(enabled=0, version='local-grab-random-v1', publishedAt=0,
                                                startAt=0, endAt=0, intervalSeconds=1800, dailyLimit=6,
                                                sourceHEX=expected_source.encode().hex().upper()))
                    self.start()
                    self.assertEqual(self.sql('SELECT enabled FROM p11_grab_random_policies;').strip(), '0')
                    uid = 'original-grab-random-'+secrets.token_hex(5)
                    login = self.login(uid)
                    stamp = 97000

                    def http(name, fields, want):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, login, stamp, want)

                    def count(item):
                        return sum(v['num'] for v in (self.profile(login)[1].get('items') or []) if v['id'] == item)

                    def point(reply):
                        return next((v.value for v in reply.redPointList if v.type == 'grabRandomReward'), 0)

                    def tcp_point():
                        deadline = time.monotonic()+5
                        while time.monotonic() < deadline:
                            op, reply = self.recv(sock)
                            if op == BY_NAME['SCRedPoint']:
                                self.assertTrue(any(v.type == 'grabRandomReward' for v in reply.redPointList))
                                return point(reply)
                        self.fail('original20118 redpoint missing')

                    created = http('CSPlayerCreate', {'playerName': '原占领随机奖励', 'heroId': 'hero_047'}, 'SCPlayerCreate')
                    main_guid = created.player.heroWarId
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',30,'$.noviceGuideSchedule',8) WHERE id={login.playerId};")
                    initial = self.profile(login)
                    self.assertEqual(initial[0].get('coin', 0), 0)
                    self.assertEqual(initial[0].get('diamond', 0), 0)
                    self.assertEqual(count('item_001'), 0)
                    self.assertEqual(point(self.call('CSRedPoint', {}, login, 0, 'SCRedPoint')), 0)
                    self.assertEqual(len(self.call('CSGetGrabData', {}, login, 0, 'SCGetGrabData').grabData), 18)
                    for invalid_stamp in [0, -1]:
                        opcode, rejected = self.stamped('CSGetRandReward', {}, login, invalid_stamp)
                        self.assertEqual(opcode, 10001)
                        self.assertIn('Invalid original grab random reward timestamp', str(rejected))
                    stamp += 1
                    self.assertEqual(self.stamped('CSGetRandReward', {}, login, stamp)[0], 10001)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_grab_random_offers;').strip(), '0')
                    now_ms = int(time.time()*1000)
                    start_at = now_ms+1200
                    end_at = start_at+600000
                    pool = '[{"id":"item_001","num":1,"weight":1}]'
                    self.sql("UPDATE p11_grab_policies SET enabled=1;")
                    self.sql(f"UPDATE p11_grab_random_policies SET enabled=1,version='native-real-one-second-local',published_at={now_ms},start_at={start_at},end_at={end_at},interval_seconds=1,daily_limit=1,pool_json=CONVERT(0x{pool.encode().hex()} USING utf8mb4) WHERE id=1;")
                    published = json.loads(self.sql("SELECT JSON_OBJECT('enabled',enabled,'version',version,'publishedAt',published_at,'startAt',start_at,'endAt',end_at,'intervalSeconds',interval_seconds,'dailyLimit',daily_limit,'pool',pool_json,'sourceHEX',HEX(source)) FROM p11_grab_random_policies WHERE id=1;"))
                    report['prepublishedPolicy'] = published
                    save_checkpoint()
                    self.assertEqual(published, dict(enabled=1, version='native-real-one-second-local',
                                                     publishedAt=now_ms, startAt=start_at, endAt=end_at,
                                                     intervalSeconds=1, dailyLimit=1, pool=pool,
                                                     sourceHEX=expected_source.encode().hex().upper()))
                    time.sleep(max(0, start_at/1000-time.time())+0.03)
                    occupied = http('CSGrab', {'index': 1}, 'SCGrab')
                    self.assertEqual(occupied.grabData.playerId, login.playerId)
                    self.assertFalse(occupied.HasField('attPlayer'))
                    self.assertEqual(occupied.winPlayerId, 0)
                    self.assertEqual(occupied.currEnergy, initial[0]['energy']-10)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_grab_sessions;').strip(), '1')
                    session = self.sql('SELECT id,player_id,position,started_at,expires_at,ended_at FROM p11_grab_sessions;')
                    self.assertEqual(point(http('CSRedPoint', {}, 'SCRedPoint')), 0)
                    sock = self.connect(login)
                    time.sleep(1.05)
                    before_claim = http('CSGetGrabData', {}, 'SCGetGrabData')
                    self.assertEqual(before_claim.grabData[0].playerId, login.playerId)
                    self.assertEqual(tcp_point(), 1)
                    # The original button sees the enum-name redpoint through its
                    # actual TCP handler and sends the unchanged HTTP82053 body.
                    stamp += 1
                    claim_stamp = stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        results = list(workers.map(lambda _: self.call('CSGetRandReward', {}, login,
                                                                       claim_stamp, 'SCGetRandReward'), range(8)))
                    self.assertTrue(all(v == results[0] for v in results))
                    self.assertEqual([(v.id, v.num) for v in results[0].add.simps], [('item_001', 1)])
                    self.assertEqual(count('item_001'), 1)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_grab_random_awards;').strip(), '1')
                    self.assertEqual(point(http('CSRedPoint', {}, 'SCRedPoint')), 0)
                    http('CSGetGrabData', {}, 'SCGetGrabData')
                    self.assertEqual(tcp_point(), 0)
                    energy = self.profile(login)[0]['energy']
                    used = http('CSItemUse', {'costItem': {'id': 'item_001', 'num': 1}}, 'SCItemUse')
                    self.assertEqual([(v.id, v.num) for v in used.get.simps], [('energy', 10)])
                    self.assertEqual(self.profile(login)[0]['energy'], energy+10)
                    self.assertEqual(count('item_001'), 0)
                    info = http('CSGetTimeRewardInfo', {}, 'SCGetTimeRewardInfo')
                    self.assertEqual(info.myGrabMinutes, 0)
                    self.assertEqual(list(info.rewardTime), [])
                    self.assertEqual(self.sql('SELECT id,player_id,position,started_at,expires_at,ended_at FROM p11_grab_sessions;'), session)
                    frozen = self.sql('SELECT source_hash,HEX(snapshot) FROM p11_grab_random_offers;')
                    self.sql("UPDATE p11_grab_random_policies SET enabled=0,daily_limit=1000,pool_json='[{\"id\":\"coin\",\"num\":999,\"weight\":1}]';")
                    stamp += 1
                    self.assertEqual(self.stamped('CSGetRandReward', {}, login, stamp)[0], 10001)
                    sock.close()
                    sock = None
                    self.stop()
                    self.start()
                    login = self.login(uid)
                    self.assertEqual(self.call('CSGetRandReward', {}, login, claim_stamp, 'SCGetRandReward'), results[0])
                    self.assertEqual(count('item_001'), 0)
                    current = http('CSPlayer', {}, 'SCPlayer').player
                    self.assertEqual(current.heroWarId, main_guid)
                    self.assertEqual(current.coin, 0)
                    self.assertEqual(current.diamond, 0)
                    self.assertEqual(current.energy, initial[0]['energy'])
                    self.assertEqual(self.sql('SELECT source_hash,HEX(snapshot) FROM p11_grab_random_offers;'), frozen)
                    self.assertEqual(self.sql("SELECT COUNT(*) FROM p1_event_facts WHERE kind='grab_occupation_random_reward';").strip(), '1')
                    http('CSGiveUpGrab', {'index': 1}, 'SCGiveUpGrab')
                    self.assertEqual(http('CSGetGrabData', {}, 'SCGetGrabData').grabData[0].playerId, 0)
                    report.update(status='PASS_ORIGINAL_OCCUPATION_TCP_RED79_82053_ITEM_USE_COLD',
                                  actualSession=session.strip(), actualOccupationEnergyCost=10,
                                  originalRandomAward={'id': 'item_001', 'num': 1}, actualItemEnergyGain=10,
                                  finalEnergy=current.energy, finalCoin=current.coin, finalDiamond=current.diamond,
                                  finalItem001=0, actualTCPRedpointValues=[1, 0], originalRedpointKey='grabRandomReward',
                                  sameClaimWorkers=8, coldRestarts=1, sourceDefaultOff=True,
                                  originalOneTwoThreeHourRewardsUnchanged=True, noOccupationClockEdits=True,
                                  localTestPublication='explicit prepublished one-second interval/daily1/item001x1 pool; real wall clock only, not original private policy',
                                  finitePrerequisites='Original CreatePlayer047 plus isolated level30/guide8 only; no currency/item/session/winner/rank/time/GUID injection.',
                                  productionModified=False, phoneUI=False)
                except BaseException as failure:
                    report.update(status='FAIL', failure=self.managed_pool.redact(str(failure)))
                    raise
                finally:
                    if sock is not None:
                        sock.close()
                    try:
                        self.stop()
                    finally:
                        try:
                            if connection is not None:
                                self.managed_pool.release(connection.database)
                        finally:
                            self.managed_pool.close()
                    report['managedAllocationsRemoved'] = True
                    save_checkpoint()
        for rel, digest in hashes.items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(), digest)
        report['seconds'] = round(time.monotonic()-began, 3)
        (ROOT/'analysis/p11/grab-random-mysql-validation.json').write_text(
            json.dumps(report, ensure_ascii=False, indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
