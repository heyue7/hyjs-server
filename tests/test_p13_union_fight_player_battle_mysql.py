"""SQL172: real managed accounts/guild declarations/socket0 and accepted timer.

Only --prepare compiles an isolated clock-bearing test application. Runtime source
is pinned before allocation; finite level/calendar/policy are explicit and no HP,
BattlePB, winner, fight record or reward is inserted into the managed schema.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import subprocess
import sys
import tempfile
import time
import types
import unittest
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from hyjs_managed_mysql import ManagedMySQL
import test_p13_mysql as guild
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME, message
from tools.native_batch.export_uf14_war_aftermath import source_snapshot, export_evidence

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PROOF = ROOT / 'analysis/p13/player-battle172-guard-prebuilt-source-proof.json'


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def prepare(destination, proof_path):
    """Run only inside the coordinated Go lease; no MySQL operation occurs."""
    proof = json.loads(proof_path.read_text())
    source = Path(proof['buildRoot'])
    for rel, expected in proof['runtimeSHA256'].items():
        if digest(source / rel) != expected:
            raise AssertionError('172 source drift: ' + rel)
    if destination.exists():
        raise AssertionError('Choose a new owned preparation directory')
    destination.mkdir(parents=True)
    main_path = destination / 'main-clock.go'
    original_main = source / 'hyjs-server/main.go'
    main = original_main.read_text()
    marker = 'defer cancel()\n'
    if main.count(marker) != 1:
        raise AssertionError('172 clock injection boundary drift')
    main = main.replace('"hyjs-server/config"', '"hyjs-server/config"\n"hyjs-server/constUtil"\n"strings"\n"sync"', 1)
    main = main.replace(marker, marker + '''
    // Isolated SQL172 QA clock, guarded by the manager-owned schema prefix.
    if !strings.HasPrefix(os.Getenv("HYJS_MYSQL_DATABASE"), "hyjs_test_p13_") { return errors.New("ordinary172 isolation guard") }
    var qaClockMu sync.Mutex
    var qaClockRaw string
    var qaClockAnchor time.Time
    var qaClockBase int64
    ctx = context.WithValue(ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time {
        raw, err := os.ReadFile(os.Getenv("HYJS_P13_FIELD_NATIVE_CLOCK")); if err != nil { panic(err) }
        qaClockMu.Lock(); defer qaClockMu.Unlock()
        changed := string(raw) != qaClockRaw
        if changed { qaClockRaw = string(raw); qaClockAnchor = time.Now() }
        parts := strings.Split(string(raw), "|")
        at, err := strconv.ParseInt(parts[0], 10, 64); if err != nil { panic(err) }
        if len(parts) == 3 && parts[1] == "realtime" {
            wallAnchor, err := strconv.ParseInt(parts[2], 10, 64); if err != nil { panic(err) }
            if changed { qaClockBase = at + max(int64(0), time.Now().UnixMilli() - wallAnchor) }
            at = qaClockBase + time.Since(qaClockAnchor).Milliseconds()
        }
        return time.UnixMilli(at)
    })
''', 1)
    main_path.write_text(main)
    overlay = destination / 'go-overlay.json'
    overlay.write_text(json.dumps({'Replace': {str(original_main): str(main_path)}}) + '\n')
    env = dict(os.environ, GOTOOLCHAIN='go1.27.2', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
    for command in [['rtk', 'proxy', 'gofmt', '-w', str(main_path)], ['rtk', 'proxy', 'go', 'build', '-p', '1', '-overlay', str(overlay), '-o', str(destination / 'hyjs-native'), '.']]:
        result = subprocess.run(command, cwd=source / 'hyjs-server', env=env, capture_output=True, text=True, timeout=180)
        if result.returncode:
            raise RuntimeError('172 prepare failed: ' + result.stdout + result.stderr)
    prepared = {'baseProof': str(proof_path), 'baseProofSHA256': digest(proof_path), 'source': str(source), 'nativeSHA256': digest(destination / 'hyjs-native'), 'clockMainSHA256': digest(main_path), 'runtimeSHA256': proof['runtimeSHA256']}
    (destination / 'manifest.json').write_text(json.dumps(prepared, indent=2) + '\n')
    print(json.dumps({'prepared': str(destination), 'manifestSHA256': digest(destination / 'manifest.json'), 'nativeSHA256': prepared['nativeSHA256']}))


@unittest.skipUnless(os.environ.get('HYJS_P13_ORDINARY172_MYSQL_TEST') == '1', 'Explicit managed SQL172 lease required')
class P13Ordinary172MySQLTests(unittest.TestCase):
    login = guild.P13MySQLTests.login
    stamped = guild.P13MySQLTests.stamped
    call = guild.P13MySQLTests.call
    profile = guild.P13MySQLTests.profile
    send = guild.P13MySQLTests.send
    recv = guild.P13MySQLTests.recv
    tcp_send = guild.P13MySQLTests.tcp_send
    connect = guild.P13MySQLTests.connect
    stop = guild.P13MySQLTests.stop
    receive = field.P13UnionFightFieldMySQLTests.receive
    drain = field.P13UnionFightFieldMySQLTests.drain
    call_all = field.P13UnionFightFieldMySQLTests.call_all

    @classmethod
    def setUpClass(cls):
        cls.prepared = Path(os.environ['HYJS_P13_ORDINARY172_PREPARED']).resolve()
        cls.proof = json.loads((cls.prepared / 'manifest.json').read_text())
        cls.source = Path(cls.proof['source'])
        for rel, expected in cls.proof['runtimeSHA256'].items():
            if digest(cls.source / rel) != expected:
                raise AssertionError('172 native source drift: ' + rel)
        if digest(Path(cls.proof['baseProof'])) != cls.proof['baseProofSHA256']:
            raise AssertionError('172 base proof drift')
        if digest(cls.prepared / 'main-clock.go') != cls.proof['clockMainSHA256']:
            raise AssertionError('172 clock-only source drift')
        if digest(cls.prepared / 'hyjs-native') != cls.proof['nativeSHA256']:
            raise AssertionError('172 native SHA drift')
        cls.pool = ManagedMySQL()
        cls.addClassCleanup(cls.pool.close)
        conn = cls.pool.allocate('p13_ordinary172')
        cls.database, cls.test_password = conn.database, conn.password
        cls.env = dict(os.environ, **conn.env(), GOMEMLIMIT='768MiB', GOMAXPROCS='2')
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p13-ordinary172-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(cls.prepared / 'hyjs-native')

    def sql(self, text):
        return self.pool.sql(self.database, text)

    def setUp(self):
        self.process = None
        self.addCleanup(self.stop)
        self.clock = Path(self.temp.name) / 'clock'
        self.env['HYJS_P13_FIELD_NATIVE_CLOCK'] = str(self.clock)

    def set_clock(self, at, realtime=False):
        temporary = self.clock.with_suffix('.next')
        temporary.write_text(str(at) + ("|realtime|" + str(time.time_ns() // 1000000) if realtime else ""))
        os.replace(temporary, self.clock)

    def start(self):
        self.process = subprocess.Popen([self.binary, '--http-port', '0', '--tcp-port', '0', '--tables', str(self.source / 'analysis/tables')], cwd=self.source / 'hyjs-server', env=self.env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), '172 native startup timeout')
            line = self.process.stdout.readline()
        match = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(match, self.pool.redact(line))
        self.service = types.SimpleNamespace(http_port=int(match[1]), tcp_port=int(match[2]))

    def wait_sql(self, sql, expected, timeout=15):
        end = time.monotonic() + timeout
        while time.monotonic() < end:
            if self.sql(sql).strip() == expected:
                return
            time.sleep(.05)
        self.assertEqual(self.sql(sql).strip(), expected)

    def test_actual_socket0_pending_finalhp_death_reward_and_cold(self):
        began = time.monotonic()
        monday = int(datetime(2026, 10, 19, 12, tzinfo=ZoneInfo('Asia/Shanghai')).timestamp() * 1000)
        deadline = monday + 12 * 3600000
        first = deadline + 20 * 3600000
        self.set_clock(monday)
        self.start()
        self.assertEqual(self.sql('SELECT enabled FROM p13_union_fight_player_battle_policies WHERE id=1;').strip(), '0')
        export_war_source = os.environ.get('HYJS_P13_ORDINARY172_SOURCE_EXPORT') == '1'
        partial_policy = os.environ.get('HYJS_P13_ORDINARY172_PARTIAL_POLICY') == '1'
        self.assertFalse(partial_policy and not export_war_source, 'Partial acceptance needs real same-war fresh enemy')
        policy_id = 2 if partial_policy else 1
        uids = ['ordinary172-' + secrets.token_hex(6) for _ in range(20 if export_war_source else 10)]
        players = [self.login(uid) for uid in uids]
        for i, login in enumerate(players):
            self.call('CSPlayerCreate', {'playerName': '实普通战' + str(i), 'heroId': 'hero_047'}, login, 1, 'SCPlayerCreate')
        ids = ','.join(str(login.playerId) for login in players)
        self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000,"$.coin",20000000) WHERE id IN (' + ids + '); UPDATE profiles SET player=JSON_SET(player,"$.level",50) WHERE id=' + str(players[0].playerId) + '; UPDATE p13_guild_policies SET management_enabled=1; UPDATE p13_union_fight_policies SET enrollment_enabled=1,match_enabled=1;')
        self.sql("UPDATE p13_union_fight_field_policies SET enabled=1,preparation_millis=300000,left_revive_point_id=1,right_revive_point_id=3,initial_guard_hp=1000,initial_gate_hp=10000,move_tolerance=3,source='finite172 local field; actual P5 playerHP'; UPDATE p13_union_fight_player_battle_policies SET enabled=1,version='actual172-native-v1',source='prepublished local ordinary14/fullHP/factor1/noattackfee/ordinal1cap40',published_at=" + str(monday) + ',starts_at=' + str(monday) + ',ends_at=' + str(monday + 72 * 3600000) + ",revive_rule='local-death-ordinal-one-based-cap40-v1' WHERE id=" + str(policy_id) + ";")
        guild_ids = []
        for i, login in enumerate(players[:10]):
            self.call('CSUnionCreate', {'name': '原战实会' + str(i)}, login, 100 + i, 'SCUnionCreate')
            gid = self.call('CSGetUnionInfo', {}, login, 120 + i, 'SCGetUnionInfo').unionInfo.unionId
            guild_ids.append(gid)
            self.sql("UPDATE p13_guilds SET level=2 WHERE id='" + gid + "';")
            if not export_war_source:
                self.call('CSUFUnionApply', {}, login, 140 + i, 'SCUFUnionApply')
                self.call('CSUFPlayerApply', {}, login, 160 + i, 'SCUFPlayerApply')
        if export_war_source:
            # Every actual guild receives another real member before the same
            # declaration deadline. Natural draw and original IDs stay intact.
            for i, member in enumerate(players[10:]):
                self.call('CSUnionApply', {'unionId': guild_ids[i]}, member, 170 + i, 'SCUnionApply')
                self.call('CSUnionAgreeApply', {'playerId': member.playerId}, players[i], 180 + i, 'SCUnionAgreeApply')
                # Declaration freezes the actual membership, so both real
                # members must already belong to this guild before it declares.
                self.call('CSUFUnionApply', {}, players[i], 140 + i, 'SCUFUnionApply')
                self.call('CSUFPlayerApply', {}, players[i], 160 + i, 'SCUFPlayerApply')
                self.call('CSUFPlayerApply', {}, member, 190 + i, 'SCUFPlayerApply')
        self.sql("INSERT INTO p13_union_fight_schedules(day,first_start_at,second_start_at,duration_millis,source) VALUES('2026-10-20'," + str(first) + ',' + str(first + 600000) + ",300000,'finite172 isolated published original war calendar');")
        self.set_clock(deadline)
        self.stop()
        self.start()
        self.wait_sql('SELECT COUNT(*) FROM p13_union_fight_matches WHERE round=1;', '4')
        match_id, left, right = self.sql("SELECT id,left_guild_id,right_guild_id FROM p13_union_fight_matches WHERE round=1 AND (left_guild_id='" + guild_ids[0] + "' OR right_guild_id='" + guild_ids[0] + "');").strip().split('\t')
        opponent_gid = right if left == guild_ids[0] else left
        a, b = players[0], players[guild_ids.index(opponent_gid)]
        selected_indices = [0, guild_ids.index(opponent_gid)]
        if export_war_source:
            selected_indices += [10, 10 + guild_ids.index(opponent_gid)]
        selected_uids = [uids[index] for index in selected_indices]
        active = [players[index] for index in selected_indices]
        self.set_clock(first - 60000)
        sockets = [self.connect(login) for login in active]
        for login in active:
            self.call('CSTransferScene', {'target': 'fight_union'}, login, 200, 'SCTransferScene')
            frames = self.call_all('CSCurrentPosition', {}, login, 201)
            self.assertTrue(any(op == BY_NAME['SCUnionFightTransfer'] for op, _ in frames))
        for sock in sockets:
            self.drain(sock)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_player_battle_offers WHERE match_id='" + match_id + "';").strip(), '1')
        self.set_clock(first + 60000, realtime=True)
        target = self.sql("SELECT x,y,z FROM p13_union_fight_field_players WHERE match_id='" + match_id + "' AND player_id=" + str(b.playerId) + ';').strip().split('\t')
        self.tcp_send(sockets[0], 'CSMove', {'pointNew': dict(zip(['x', 'y', 'z'], map(float, target)))}, a)
        self.wait_sql("SELECT CAST(ABS(x-" + target[0] + ")<0.0001 AS UNSIGNED) FROM p13_union_fight_field_players WHERE match_id='" + match_id + "' AND player_id=" + str(a.playerId) + ';', '1')
        for sock in sockets:
            self.drain(sock)
        wallet_before = self.sql('SELECT id,HEX(player) FROM profiles WHERE id IN (' + str(a.playerId) + ',' + str(b.playerId) + ') ORDER BY id;')
        battle_started = time.monotonic()
        self.tcp_send(sockets[0], 'CSUFPlayerAskFight', {'targetGuid': b.playerId}, a)
        begin = self.receive(sockets[0], 'SCUFPlayerFightBegin')
        self.assertEqual({begin.guid, begin.targetGuid}, {a.playerId, b.playerId})
        self.wait_sql("SELECT COUNT(*) FROM p13_union_fight_player_battle_attempts WHERE client_stamp=0 AND status='pending';", '1')
        self.tcp_send(sockets[0], 'CSUFPlayerAskFight', {'targetGuid': b.playerId}, a)
        self.receive(sockets[0], 'SCUFPlayerFightBegin')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_player_battle_attempts;').strip(), '1')
        self.assertEqual(self.sql('SELECT status FROM p13_union_fight_player_battle_attempts;').strip(), 'pending')
        self.sql('UPDATE p13_union_fight_player_battle_policies SET enabled=0;')
        # The battle interval now advances only with real Go monotonic elapsed;
        # no 4999/5000 jump is injected into the original socket acceptance.
        self.wait_sql("SELECT COUNT(*) FROM p13_union_fight_player_battle_attempts WHERE status='settled';", '1')
        actual_battle_seconds = round(time.monotonic() - battle_started, 3)
        after = self.sql('SELECT HEX(`after`) FROM p13_union_fight_player_battle_attempts;').strip()
        row = self.sql('SELECT dead_player_id,reward_player_id,revive_seconds FROM p13_union_fight_player_battle_attempts;').strip().split('\t')
        dead, winner, duration = map(int, row)
        self.assertEqual({dead, winner}, {a.playerId, b.playerId})
        self.assertEqual(duration, 5)
        detail = self.receive(sockets[0], 'SCUFPlayerFightEndDetail')
        self.assertEqual(detail.player.guid, a.playerId)
        death = self.receive(sockets[0], 'SCUFPlayerDead')
        self.assertEqual(list(death.guid), [dead])
        self.assertEqual(len(death.reviveTime), 1)
        self.assertGreaterEqual(death.reviveTime[0], 0)
        self.assertLessEqual(death.reviveTime[0], 5)
        self.assertEqual(list(death.reviveItemId), [''])
        frames = json.loads(bytes.fromhex(after))
        terminal = json.loads(bytes.fromhex(self.sql('SELECT HEX(terminal) FROM p13_union_fight_player_battle_attempts;').strip()))
        self.assertTrue(terminal['outcome']['authoritative'])
        self.assertEqual(terminal['outcome']['capabilityHash'], 'dd937dec43673be9d883f405491589fd4fcb1f8891ba09861b96bee34519c980')
        for camp, frame in enumerate(frames):
            actors = [actor for actor in terminal['finalActors'] if actor['camp'] == camp]
            fight = frame['fightInfo']
            self.assertEqual(len(actors), len(fight['index']))
            by_pose = {actor['pose']: actor for actor in actors}
            for index, pose in enumerate(fight['index']):
                self.assertEqual(fight['hpPos'][index], by_pose[pose]['currentHp'])
                self.assertEqual(fight['maxHpPos'][index], by_pose[pose]['maxHp'])
            battle = terminal['outcome']['attacker' if camp == 0 else 'defender']
            self.assertEqual(fight.get('anger', 0), battle['inst'].get('anger', 0))
        won_frame = next(frame for frame in frames if frame['guid'] == winner)
        self.assertEqual((won_frame['killNum'], won_frame['ownScore'], won_frame['ownToken']), (1, 5, 5))
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p6_battle_records WHERE source='p13-uf-player14-factor1' AND authoritative=1 AND status='computed';").strip(), '1')
        self.assertEqual(wallet_before, self.sql('SELECT id,HEX(player) FROM profiles WHERE id IN (' + str(a.playerId) + ',' + str(b.playerId) + ') ORDER BY id;'))
        fact = self.sql('SELECT id,status,HEX(source_hash),HEX(`after`),p6_record_id FROM p13_union_fight_player_battle_attempts;')
        self.stop()
        self.start()
        fresh = [self.login(uid) for uid in selected_uids]
        # Original war peers are online authenticated sinks. Restore every real
        # connection before checking the whole group; login alone is offline.
        cold_sockets = [self.connect(login) for login in fresh]
        for login in fresh:
            self.call_all('CSCurrentPosition', {}, login, 400)
        for sock in cold_sockets:
            self.drain(sock)
        cold_socket = cold_sockets[0]
        self.tcp_send(cold_socket, 'CSUFRefreshElementArray', {'guid': []}, fresh[0])
        cold_players = self.receive(cold_socket, 'SCUFPlayer')
        self.assertEqual({player.guid for player in cold_players.player}, {login.playerId for login in active})
        for player in cold_players.player:
            if player.guid not in {a.playerId, b.playerId}:
                self.assertTrue(all(hp > 0 and hp == maximum for hp, maximum in zip(player.fightInfo.hpPos, player.fightInfo.maxHpPos)))
                continue
            expected = next(frame for frame in frames if frame['guid'] == player.guid)['fightInfo']
            self.assertEqual(list(player.fightInfo.index), expected['index'])
            self.assertEqual(list(player.fightInfo.hpPos), expected['hpPos'])
            self.assertEqual(list(player.fightInfo.maxHpPos), expected['maxHpPos'])
            self.assertEqual(player.fightInfo.anger, expected.get('anger', 0))
        self.assertEqual(fact, self.sql('SELECT id,status,HEX(source_hash),HEX(`after`),p6_record_id FROM p13_union_fight_player_battle_attempts;'))
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME LIKE 'p13_union_fight_player_battle_%';").strip(), 'InnoDB')
        exported = None
        if export_war_source:
            # Capture actual partial survivor and a real fresh opposite member;
            # no dead participant is restored or borrowed from an earlier time.
            attempt_id = self.sql('SELECT id FROM p13_union_fight_player_battle_attempts;').strip()
            source = source_snapshot(self.sql, attempt_id, [login.playerId for login in active])
            export_root = ROOT / 'analysis/p13' / ('actual-player-battle172-source-' + attempt_id[:16])
            exported = export_evidence(source, export_root, Path(self.proof['baseProof']), [begin, detail, death, cold_players])
        second_fact = None
        if partial_policy:
            # A genuine new enemy remains alive in the same current war. The
            # second request consumes the first real terminal's exact HP/AP.
            by_id = {row['playerId']: row for row in source['participants']}
            survived = by_id[winner]
            candidates = [row for row in source['participants'] if row['camp'] != survived['camp'] and row['guildId'] != survived['guildId'] and row['attemptCount'] == 0]
            self.assertTrue(candidates, 'Actual never-fought same-war enemy missing')
            enemy = min(candidates, key=lambda row: row['playerId'])
            source_frame = message('InstUFPlayer'); source_frame.ParseFromString(bytes.fromhex(survived['frame']))
            self.assertTrue(any(0 < hp < maximum for hp, maximum in zip(source_frame.fightInfo.hpPos, source_frame.fightInfo.maxHpPos)), 'Real survivor was not injured; do not create HP')
            owner_index = next(i for i, login in enumerate(fresh) if login.playerId == winner)
            actor_login, actor_socket = fresh[owner_index], cold_sockets[owner_index]
            point = self.sql("SELECT x,y,z FROM p13_union_fight_field_players WHERE match_id='" + match_id + "' AND player_id=" + str(enemy['playerId']) + ';').strip().split('\t')
            self.tcp_send(actor_socket, 'CSMove', {'pointNew': dict(zip(['x', 'y', 'z'], map(float, point)))}, actor_login)
            self.wait_sql("SELECT CAST(ABS(x-" + point[0] + ")<0.0001 AS UNSIGNED) FROM p13_union_fight_field_players WHERE match_id='" + match_id + "' AND player_id=" + str(winner) + ';', '1')
            for sock in cold_sockets: self.drain(sock)
            self.sql('UPDATE p13_union_fight_player_battle_policies SET enabled=1 WHERE id=2;')
            wallet_second = self.sql('SELECT id,HEX(player) FROM profiles ORDER BY id;')
            second_started = time.monotonic()
            self.tcp_send(actor_socket, 'CSUFPlayerAskFight', {'targetGuid': enemy['playerId']}, actor_login)
            self.receive(actor_socket, 'SCUFPlayerFightBegin')
            self.wait_sql("SELECT COUNT(*) FROM p13_union_fight_player_battle_attempts WHERE status='pending';", '1')
            second_id, start_hex, proof = self.sql("SELECT id,HEX(start_pb),scope_proof FROM p13_union_fight_player_battle_attempts WHERE status='pending';").strip().split('\t')
            expected_proof = digest(ROOT / 'analysis/p6/uf14-actual-wounded-native-proof-v196.json')
            self.assertEqual(proof, expected_proof)
            second_start = message('SCUFFightRecordReplay'); second_start.ParseFromString(bytes.fromhex(start_hex))
            self.assertEqual(second_start.playerList[0].playerId, winner)
            self.assertEqual(second_start.playerList[1].playerId, enemy['playerId'])
            self.assertEqual(second_start.playerList[0].inst.anger, source_frame.fightInfo.anger)
            pose_hp = dict(zip(source_frame.fightInfo.index, source_frame.fightInfo.hpPos))
            for hero in second_start.playerList[0].formation:
                self.assertEqual(hero.inst.curHp, pose_hp[hero.pose])
            self.tcp_send(actor_socket, 'CSUFPlayerAskFight', {'targetGuid': enemy['playerId']}, actor_login)
            self.receive(actor_socket, 'SCUFPlayerFightBegin')
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_player_battle_attempts;').strip(), '2')
            self.sql('UPDATE p13_union_fight_player_battle_policies SET enabled=0;')
            self.wait_sql("SELECT COUNT(*) FROM p13_union_fight_player_battle_attempts WHERE status='settled';", '2')
            elapsed = round(time.monotonic() - second_started, 3)
            second_after, second_terminal = self.sql("SELECT HEX(`after`),HEX(terminal) FROM p13_union_fight_player_battle_attempts WHERE id='" + second_id + "';").strip().split('\t')
            result = json.loads(bytes.fromhex(second_terminal))
            self.assertEqual(result['outcome']['capabilityHash'], expected_proof)
            for camp, frame in enumerate(json.loads(bytes.fromhex(second_after))):
                actors = {actor['pose']: actor for actor in result['finalActors'] if actor['camp'] == camp}
                for index, pose in enumerate(frame['fightInfo']['index']):
                    self.assertEqual(frame['fightInfo']['hpPos'][index], actors[pose]['currentHp'])
                    self.assertEqual(frame['fightInfo']['maxHpPos'][index], actors[pose]['maxHp'])
            self.assertEqual(wallet_second, self.sql('SELECT id,HEX(player) FROM profiles ORDER BY id;'))
            self.assertEqual(fact, self.sql("SELECT id,status,HEX(source_hash),HEX(`after`),p6_record_id FROM p13_union_fight_player_battle_attempts WHERE id='" + attempt_id + "';"))
            second_fact = self.sql('SELECT id,status,HEX(source_hash),HEX(`after`),p6_record_id FROM p13_union_fight_player_battle_attempts ORDER BY id;')
            self.stop(); self.start()
            again = [self.login(uid) for uid in selected_uids]
            again_sockets = [self.connect(login) for login in again]
            for login in again: self.call_all('CSCurrentPosition', {}, login, 500)
            for sock in again_sockets: self.drain(sock)
            self.tcp_send(again_sockets[0], 'CSUFRefreshElementArray', {'guid': []}, again[0])
            current_players = self.receive(again_sockets[0], 'SCUFPlayer')
            self.assertEqual({value.guid for value in current_players.player}, {login.playerId for login in again})
            self.assertEqual(second_fact, self.sql('SELECT id,status,HEX(source_hash),HEX(`after`),p6_record_id FROM p13_union_fight_player_battle_attempts ORDER BY id;'))
            # Save exact second Start/terminal as a separate later evidence set;
            # the first accepted row/source manifest remains unchanged.
            second_dir = ROOT / 'analysis/p13' / ('actual-player-partial172-' + second_id[:16])
            second_dir.mkdir(exist_ok=False)
            assets = {'actual-start.SCUFFightRecordReplay.pb': bytes.fromhex(start_hex), 'actual-terminal.json': bytes.fromhex(second_terminal), 'actual-aftermath.json': bytes.fromhex(second_after), 'actual-cold.SCUFPlayer.pb': current_players.SerializeToString(deterministic=True)}
            for name, body in assets.items(): (second_dir / name).write_bytes(body)
            (second_dir / 'manifest.json').write_text(json.dumps(dict(status='ACTUAL_SECOND_SAME_WAR_CURRENT_HP_AP_ACCEPTED',firstSourceManifestSHA256=digest(export_root/'manifest.json'),capabilityProof=expected_proof,sourceProofSHA256=self.proof['baseProofSHA256'],seed=second_start.randomSeed,firstAttempt=attempt_id,secondAttempt=second_id,elapsedSeconds=elapsed,files=[dict(path=name,sha256=hashlib.sha256(body).hexdigest()) for name,body in assets.items()],globalAuthority=False),indent=2)+'\n')
        report = {'status': 'PASS', 'seconds': round(time.monotonic() - began, 3), 'actualSocketBattleElapsedSeconds': actual_battle_seconds, 'sourceProofSHA256': self.proof['baseProofSHA256'], 'actual': str(len(players)) + 'realCreate047/guild registration/natural drawn pair/Current+movement/Socket0/5s pending/actualHP+AP/death5score5warToken/cold once', 'finite': 'Player50vs30/coins/diamonds/guildLv2/local calendar and published local revive-index rule; no HP/PB/seed/winner/kill injected', 'MySQLAllocation': 'managed project account; class cleanup releases exact schema', 'globalAuthority': False, 'partialSecondAccepted': bool(second_fact), 'actualPartialSecondManifest': str(second_dir / 'manifest.json') if second_fact else None, 'actualWarSourceExport': str(export_root) if exported else None, 'actualWarSourceManifestSHA256': digest(export_root / 'manifest.json') if exported else None}
        (ROOT / ('analysis/p13/player-battle172-partial-mysql-validation.json' if partial_policy else 'analysis/p13/player-battle172-mysql-validation.json')).write_text(json.dumps(report, indent=2) + '\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument('--prepare', type=Path)
    parser.add_argument('--proof', type=Path, default=DEFAULT_PROOF)
    args, remaining = parser.parse_known_args()
    if args.prepare:
        prepare(args.prepare.resolve(), args.proof.resolve())
    else:
        unittest.main(argv=[__file__] + remaining)
