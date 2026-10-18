"""Original field-PK HTTP/TCP on one qualified executable and managed schema.

No compiler, APK/device operation, source copy, ad-hoc DB account or live-policy
write. Only newly allocated accounts receive the explicitly reported finite
level25/coin100000/current-injury/red-penalty prerequisites. Battle actors are
always produced by actual CreatePlayer/P5, winners never provided by the runner.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import select
import subprocess
import tempfile
import time
import types
import unittest

from server.codec import BY_NAME, CLASSES, parse_http
from hyjs_managed_mysql import ManagedMySQL
import test_continued_gameplay_mysql as transport

ROOT = Path(__file__).resolve().parents[1]
SOURCE_ROOT = Path(os.environ.get('HYJS_P12_FIELD_PVP_SOURCE_ROOT', ROOT)).resolve()
VALIDATION = Path(os.environ.get('HYJS_P12_FIELD_PVP_VALIDATION', ROOT / 'analysis/p12/field-pvp-playable-20261016/validation.json')).resolve()
REPORT = Path(os.environ.get('HYJS_P12_FIELD_PVP_REPORT', ROOT / 'analysis/p12/field-pvp-playable-20261016/http-mysql-report.json')).resolve()


def verify_pk_delivery(frames, response, terminal, viewer_id):
    """Verify original frame order and actual server terminal, without new math."""
    wanted = [BY_NAME['SCSidePvpFightResult'], BY_NAME['SCFieldPlayerState'], BY_NAME['SCPlayer']]
    relevant = [(op, value) for op, value in frames if op in wanted]
    if [op for op, _ in relevant] != wanted:
        raise AssertionError('Expected one original PK result -> side -> current player on each participant socket')
    result, side_frame, current = [value for _, value in relevant]
    if result.SerializeToString(deterministic=True) != response.SerializeToString(deterministic=True):
        raise AssertionError('Participants received different original PK result')
    if current.player.playerId != viewer_id:
        raise AssertionError('Current SCPlayer belongs to another participant')
    sides = {side.playerId: side for side in side_frame.player}
    if len(side_frame.player) != 2 or set(sides) != {response.attPlayerId, response.defPlayerId}:
        raise AssertionError('Original side frame must contain exactly both actual participants')
    actors = terminal['finalActors']
    if len(actors) != 2 or {actor['camp'] for actor in actors} != {0, 1}:
        raise AssertionError('Actual original type5 terminal actor denominator changed')
    for camp, player_id, key in [(0, response.attPlayerId, 'attacker'), (1, response.defPlayerId, 'defender')]:
        actor = next(actor for actor in actors if actor['camp'] == camp)
        side = sides[player_id]
        if actor['maxHp'] <= 0 or side.curHp != actor['currentHp'] / actor['maxHp']:
            raise AssertionError('Side HP does not project the actual terminal actor')
        if side.anger != terminal['outcome'][key]['inst'].get('anger', 0):
            raise AssertionError('Side AP was reset or taken from the initial actor')
        if side.state != (5 if actor['currentHp'] == 0 else 0):
            raise AssertionError('Original dead/idle state does not follow actual terminal HP')
        if len(side.battlePosHps) != 1 or side.battlePosHps[0].battlePos != actor['pose'] or side.battlePosHps[0].hpPercent != side.curHp:
            raise AssertionError('Side per-position HP source changed')
    return {'orderedOpcodes': wanted, 'viewer': viewer_id,
            'fight': response.fightId, 'terminalActors': len(actors),
            'currentCoin': current.player.coin}


def verify_replay_terminal(replay, terminal):
    """Bind each exact source hero and retained injury to its final original actor."""
    damage = {}
    for camp, source in [(0, replay.attPlayer), (1, replay.defPlayer)]:
        if len(source.formation) != 1 or source.formation[0].id != 'hero_047' or source.formation[0].pose != 3:
            raise AssertionError('Business source escaped the independently certified single047 family')
        actor = next(actor for actor in terminal['finalActors'] if actor['camp'] == camp)
        initial = source.formation[0]
        digest = hashlib.sha256(initial.SerializeToString(deterministic=True)).hexdigest()
        if digest != actor['initialActorSHA256'] or actor['guid'] != initial.guid or actor['heroId'] != initial.id:
            raise AssertionError('Actual terminal actor is not bound to the frozen business source')
        if actor['maxHp'] != initial.prop.hp or actor['currentHp'] > initial.inst.curHp:
            raise AssertionError('Current bounded physical-only family unexpectedly gained HP')
        damage[str(camp)] = initial.inst.curHp - actor['currentHp']
    if replay.attPlayer.inst.firstOrder != 1 or replay.defPlayer.inst.firstOrder != 0 or replay.maxRoundCount != 40:
        raise AssertionError('Original type5 initiative/max40 wrapper changed')
    return damage

def preflight():
    if SOURCE_ROOT.parent != (ROOT / 'build/candidates').resolve() or not SOURCE_ROOT.is_dir():
        raise AssertionError('Require one direct immutable candidate before allocation')
    binary = Path(os.environ.get('HYJS_P12_FIELD_PVP_BINARY', '')).resolve()
    if not binary.is_file():
        raise AssertionError('Select a prebuilt qualified executable; this runner never compiles')
    proof = json.loads(VALIDATION.read_text())
    if not proof.get('qualified'):
        raise AssertionError('Complete root qualification before field-PK protocol validation')
    transport.SOURCE_ROOT, transport.SOURCE = SOURCE_ROOT, SOURCE_ROOT / 'manifest.json'
    transport.VALIDATION, transport.REPORT = VALIDATION, REPORT
    previous = os.environ.get('HYJS_CONTINUED_GAMEPLAY_BINARY')
    os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = str(binary)
    try:
        built, binary_sha, source_sha = transport.binary_source_preflight()
    finally:
        if previous is None:
            os.environ.pop('HYJS_CONTINUED_GAMEPLAY_BINARY', None)
        else:
            os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = previous
    schema = Path('server/protocol/schema.json')
    if (ROOT / schema).read_bytes() != (SOURCE_ROOT / schema).read_bytes():
        raise AssertionError('Original codec differs from fixed source')
    for name, fields in {
        'CSSidePvpFightResult': {'playerId': 42, 'fightType': 0},
        'CSPlayerRedeem': {'type': 0}, 'CSSideGetPvpMVList': {},
        'CSSideGetPvpMV': {'fightId': 'bounded-preflight'},
        'CSTransferScene': {'target': 'field_002'}, 'CSCurrentPosition': {},
        'CSMove': {'pointNew': {'x': 68, 'y': 15, 'z': 84}},
    }.items():
        frame = transport.encode_original_http(name, fields, 2, b'0' * 16, 0)
        parsed = parse_http(frame)
        if parsed.opcode != BY_NAME[name] or parsed.decode() != CLASSES[name](**fields):
            raise AssertionError('Original frame contract changed: ' + name)
    if BY_NAME['CSSidePvpFightResult'] != 30045:
        raise AssertionError('Original PK TCP opcode changed')
    if 'winPlayerId' in CLASSES['CSSidePvpFightResult'].DESCRIPTOR.fields_by_name:
        raise AssertionError('Request must never provide a winner')
    for path, literal in {
        'hyjs-server/service/p12FieldPVPService.go': 'DispatchP12FieldPVP',
        'hyjs-server/service/gameService.go': 'DispatchP12FieldPVP',
        'hyjs-server/main.go': 's.P12FieldPVPReady = true',
        'hyjs-server/service/p12FieldPVPIntegrationService.go': 'syncP12FieldPVPPenaltyForRead',
        'hyjs-server/service/combatOwnershipService.go': 'P12FieldPVPRequireIdle',
        'hyjs-server/service/fieldService.go': 'ShouldAllowP12PrisonPosition',
        'hyjs-server/service/p8LifeService.go': 'pauseP12FieldPVPSurvival',
        'hyjs-server/service/p12FieldPVPScopeService.go': 'ComputeP12FieldPVPProtocolFinalHP',
    }.items():
        if literal not in (SOURCE_ROOT / path).read_text():
            raise AssertionError('Candidate lacks required integration: ' + path)
    native_source = (SOURCE_ROOT / 'hyjs-server/constUtil/p12FieldPVPScope.go').read_text()
    proof_sha = 'f3e056101195af7fd8ccc63287b05dfb828aea6aaa64cd819eca2bcf02f42211'
    if not re.search(r'P12FieldPVPNativeProof\s*=\s*"' + proof_sha + r'"', native_source):
        raise AssertionError('Require this independent type5 certificate, without substituting another mode')
    return built, {'scopeProof': proof_sha, 'binarySHA256': binary_sha, 'sourceManifestSHA256': source_sha,
                   'sourceRoot': str(SOURCE_ROOT), 'runnerSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}


@unittest.skipUnless(os.environ.get('HYJS_P12_FIELD_PVP_MYSQL_TEST') == '1', 'Explicit isolated field-PK batch')
class FieldPVPMySQLTests(unittest.TestCase):
    send = transport.ContinuedGameplayMySQLTests.send
    login = transport.ContinuedGameplayMySQLTests.login
    stamped = transport.ContinuedGameplayMySQLTests.stamped
    call = transport.ContinuedGameplayMySQLTests.call
    profile = transport.ContinuedGameplayMySQLTests.profile
    write_profile = transport.ContinuedGameplayMySQLTests.write_profile
    sql = transport.ContinuedGameplayMySQLTests.sql
    stop = transport.ContinuedGameplayMySQLTests.stop
    connect = transport.ContinuedGameplayMySQLTests.connect
    tcp_send = transport.ContinuedGameplayMySQLTests.tcp_send
    tcp_call = transport.ContinuedGameplayMySQLTests.tcp_call
    recv = transport.ContinuedGameplayMySQLTests.recv
    close_sockets = transport.ContinuedGameplayMySQLTests.close_sockets

    def start(self):
        self.process = subprocess.Popen([self.binary, '--http-port', '0', '--tcp-port', '0',
                                         '--tables', str(SOURCE_ROOT / 'analysis/tables'),
                                         '--log-file', str(Path(self.app_temp.name) / 'app.log')],
                                        cwd=SOURCE_ROOT / 'hyjs-server', env=self.env,
                                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), 'Owned app startup timeout')
            line = self.process.stdout.readline()
        ports = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(ports, line)
        self.service = types.SimpleNamespace(http_port=int(ports[1]), tcp_port=int(ports[2]))

    def receipt(self, fight_id):
        self.assertRegex(fight_id, r'^[a-f0-9-]{36}$')
        return json.loads(self.sql('SELECT JSON_OBJECT("fight",fight_id,"owner",player_id,'
                                   '"peer",opponent_id,"stamp",stamp,"winner",winner_id,'
                                   '"loot",loot_coin,"proof",scope_proof,"terminal",HEX(terminal),'
                                   '"p6",p6_record_id,"busy",busy_until,"created",created_at) '
                                   'FROM p12_field_pvp_receipts WHERE fight_id="' + fight_id + '";'))

    def wait_busy(self, receipt):
        remaining = receipt['busy'] / 1000 - time.time()
        if remaining > 0:
            time.sleep(min(7, remaining + .15))

    def move(self, sock, account, x):
        self.tcp_send(sock, 'CSMove', {'pointNew': {'x': x, 'y': 15, 'z': 84},
                                     'currentTime': int(time.time() * 1000)}, account)
        self.tcp_call(sock, account, 'CSGetHp', {'playerId': account.playerId}, 'SCGetHp')
        self.assertEqual(self.profile(account)[1]['position']['current']['x'], x)

    def drain(self, sock):
        frames = []
        for _ in range(128):
            if not select.select([sock], [], [], .03)[0]:
                return frames
            frames.append(self.recv(sock))
        self.fail('Unexpected unbounded original push backlog')

    def pk_delivery(self, sock):
        frames = []
        deadline = time.monotonic() + 15
        for _ in range(80):
            remaining = deadline - time.monotonic()
            self.assertGreater(remaining, 0, 'Original participant delivery deadline elapsed')
            self.assertTrue(select.select([sock], [], [], remaining)[0], 'Original participant delivery missing')
            op, value = self.recv(sock)
            self.frame_opcodes.append(op)
            self.assertNotEqual(op, 10001, 'Original participant TCP result rejected')
            frames.append((op, value))
            if op == BY_NAME['SCPlayer'] and any(code == BY_NAME['SCSidePvpFightResult'] for code, _ in frames):
                return frames
        self.fail('Original result/side/current-player delivery missing')

    def reject(self, name, fields, account, stamp=0, reason=None):
        opcode, message = self.stamped(name, fields, account, stamp)
        self.assertEqual(opcode, 10001, 'Expected original ErrorMessage for ' + name)
        if reason is not None:
            self.assertIn(reason, message.msg, 'Rejection came from an unrelated validation path')
        return {'request': name, 'stampPositive': stamp > 0, 'errorCode': message.code,
                'guardReason': message.msg if reason else None}

    def online_facts(self, account):
        return json.loads(self.sql('SELECT JSON_OBJECT("player",player_id,"scene",scene,"line",line,'
                                   '"authenticated",session_hash<>"") FROM p10_scene_presence '
                                   'WHERE player_id=' + str(account.playerId) + ';'))

    def test_full_pk_loot_busy_cold_mvs_judicial_send_and_redeem(self):
        self.binary, report = preflight()
        report.update(status='RUNNING', phoneUI=False, productionModified=False,
                      finitePrerequisites='new CreatePlayer047 accounts; playerlevel25/guide7/coin100000; '
                      'one saved positive injury for ordinary and judicial target; prior redkill6 for judicial boundary; '
                      'not natural progression, earned red kills or original-server rule proof',
                      globalAuthorityExpanded=False)
        self.process, self.app_temp = None, None
        self.sockets, self.frame_opcodes = [], []
        self.pool = ManagedMySQL()
        began = time.monotonic()
        lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            lock.close()
            self.fail('SQL_BUSY before allocating schema or process')
        try:
            connection = self.pool.allocate('p12_field_pvp')
            self.database = connection.database
            report['managedDatabase'] = self.database
            report['actualApplicationEnvironment'] = ['HYJS_MYSQL_HOST', 'HYJS_MYSQL_USER', 'HYJS_MYSQL_PASSWORD', 'HYJS_MYSQL_DATABASE']
            report['goFixtureExecuted'] = False
            self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
            self.env.update({key: value for key, value in connection.env().items() if key.startswith('HYJS_MYSQL_')})
            self.env['TZ'] = 'Asia/Shanghai'
            self.app_temp = tempfile.TemporaryDirectory(prefix='hyjs-p12-field-pvp-')
            self.start()
            self.assertEqual(self.sql('SELECT enabled FROM p12_field_pvp_policies WHERE id=1;').strip(), '0')
            self.assertEqual(self.sql('SELECT authoritative_enabled FROM p6_combat_rules WHERE id=1;').strip(), '0')
            self.assertEqual(self.sql('SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA="' + self.database + '" AND TABLE_NAME LIKE "p12_field_pvp_%";').strip(), 'InnoDB')
            sentinel = self.login('pk-protected-' + secrets.token_hex(6))
            self.assertEqual(sentinel.playerId, 1)
            protected = self.sql('SELECT SHA2(COALESCE(player,""),256),SHA2(COALESCE(game,""),256) FROM profiles WHERE id=1;')
            uids = ['pk-real-' + secrets.token_hex(7) for _ in range(3)]
            a, b, c = [self.login(uid) for uid in uids]
            for i, account in enumerate([a, b, c]):
                self.call('CSPlayerCreate', {'playerName': '野外真链' + str(i), 'heroId': 'hero_047'}, account, 1, 'SCPlayerCreate')
                player, game = self.profile(account)
                player.update(level=25, coin=100000, noviceGuideSchedule=7)
                self.write_profile(account, player, game)
            closed, _ = self.stamped('CSSidePvpFightResult', {'playerId': b.playerId}, a, 0)
            self.assertEqual(closed, 10001)
            self.sql('UPDATE p10_scene_policies SET enabled=1;UPDATE p12_field_pvp_policies SET enabled=1;')
            for account in [a, b, c]:
                self.call('CSTransferScene', {'target': 'field_001'}, account, 10, 'SCTransferScene')
                self.call('CSCurrentPosition', {}, account, 0, 'SCCurrentPosition')
            sa, sb, sc = [self.connect(account) for account in [a, b, c]]
            # First map is allSafe1 even outside its small regeneration rectangle.
            for sock, account, x in [(sa, a, 15), (sb, b, 16), (sc, c, 17)]:
                self.move(sock, account, x)
            first_safe = self.call('CSSidePvpFightResult', {'playerId': b.playerId}, a, 0, 'SCSidePvpFightResult')
            self.assertEqual(first_safe.code, 2007)
            for account in [a, b, c]:
                self.call('CSTransferScene', {'target': 'field_002'}, account, 20, 'SCTransferScene')
                self.call('CSCurrentPosition', {}, account, 0, 'SCCurrentPosition')
            safe = self.call('CSSidePvpFightResult', {'playerId': b.playerId}, a, 0, 'SCSidePvpFightResult')
            self.assertEqual(safe.code, 2007)
            for sock, account, x in [(sa, a, 68), (sb, b, 69), (sc, c, 70)]:
                self.move(sock, account, x)
            self.call('CSTransferLine', {'line': 2}, b, 31, 'SCTransferLine')
            report['crossLineRejection'] = self.reject('CSSidePvpFightResult', {'playerId': b.playerId}, a)
            self.call('CSTransferLine', {'line': 1}, b, 32, 'SCTransferLine')
            self.move(sb, b, 90)
            distance = self.call('CSSidePvpFightResult', {'playerId': b.playerId}, a, 0, 'SCSidePvpFightResult')
            self.assertEqual(distance.code, 2001)
            self.move(sb, b, 69)
            bad = CLASSES['SCUserLogin']()
            bad.CopyFrom(a)
            bad.sessionKey = bytes([a.sessionKey[0] ^ 1]) + bytes(a.sessionKey)[1:]
            report['staleSessionRejection'] = self.reject('CSSidePvpFightResult', {'playerId': b.playerId}, bad)
            self.sockets.remove(sb)
            sb.close()
            deadline = time.monotonic() + 5
            while self.online_facts(b)['authenticated'] and time.monotonic() < deadline:
                time.sleep(.03)
            self.assertFalse(self.online_facts(b)['authenticated'], 'Target socket disconnect did not revoke current scene session')
            report['offlineRejection'] = self.reject('CSSidePvpFightResult', {'playerId': b.playerId}, a)
            sb = self.connect(b)
            facts = [self.online_facts(account) for account in [a, b, c]]
            self.assertTrue(all(row['authenticated'] and row['scene'] == 'field_002' and row['line'] == 1 for row in facts))
            report['actualTCPQualification'] = facts
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p12_field_pvp_receipts;').strip(), '0')
            for account in [b, c]:
                player, game = self.profile(account)
                game['field']['side'].update(curHp=.01, battlePos3=.01,
                                            battlePosHps=[{'battlePos': 3, 'hpPercent': .01}])
                self.write_profile(account, player, game)
            # Original socket request has no timestamp and no result. The HTTP
            # retry during its lease must return exactly the already accepted fight.
            for sock in [sa, sb, sc]:
                self.drain(sock)
            self.tcp_send(sa, 'CSSidePvpFightResult', {'playerId': b.playerId}, a)
            owner_frames = self.pk_delivery(sa)
            peer_frames = self.pk_delivery(sb)
            first = next(value for op, value in owner_frames if op == BY_NAME['SCSidePvpFightResult'])
            self.assertEqual(first.code, 1)
            row = self.receipt(first.fightId)
            self.assertIsNone(row['stamp'])
            self.assertEqual(row['busy'] - row['created'], 6000)
            self.assertEqual(row['proof'], report['scopeProof'])
            terminal = json.loads(bytes.fromhex(row['terminal']))
            report['ordinarySocketDeliveries'] = [verify_pk_delivery(owner_frames, first, terminal, a.playerId),
                                                  verify_pk_delivery(peer_frames, first, terminal, b.playerId)]
            report['ordinary'] = {k: v for k, v in row.items() if k != 'terminal'}
            before = [self.profile(account) for account in [a, b]]
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                retries = list(pool.map(lambda _: self.call('CSSidePvpFightResult', {'playerId': b.playerId}, a, 0, 'SCSidePvpFightResult'), range(8)))
            self.assertTrue(all(reply.SerializeToString() == first.SerializeToString() for reply in retries))
            self.assertEqual([self.profile(account) for account in [a, b]], before)
            reverse = self.call('CSSidePvpFightResult', {'playerId': a.playerId}, b, 0, 'SCSidePvpFightResult')
            self.assertEqual(reverse.code, 2002)
            self.assertEqual(row['winner'], a.playerId, 'Finite injury fixture must really win; no fallback winner')
            self.assertEqual(row['loot'], 2000)
            self.assertEqual((before[0][0]['coin'], before[1][0]['coin']), (102000, 98000))
            for camp, account in enumerate([a, b]):
                side = self.profile(account)[1]['field']['side']
                actor = next(actor for actor in terminal['finalActors'] if actor['camp'] == camp)
                self.assertEqual(side['curHp'], actor['currentHp'] / actor['maxHp'])
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p12_field_pvp_receipts;').strip(), '1')
            replay = self.call('CSSideGetPvpMV', {'fightId': first.fightId}, b, 0, 'SCSideGetPvpMV')
            self.assertEqual(replay.data.winPlayerId, row['winner'])
            report['actualTerminalDamageByCamp'] = verify_replay_terminal(replay.data, terminal)
            self.assertGreater(report['actualTerminalDamageByCamp']['1'], 0, 'No substituted damage or fallback winner')
            self.assertEqual(replay.data.defPlayer.formation[0].inst.curHp,
                             replay.data.defPlayer.formation[0].prop.hp * .01)
            self.assertEqual(len(self.call('CSSideGetPvpMVList', {}, a, 0, 'SCSideGetPvpMVList').mvs), 1)
            self.assertEqual(self.stamped('CSSideGetPvpMV', {'fightId': first.fightId}, c, 0)[0], 10001)
            self.wait_busy(row)
            self.close_sockets(); self.stop(); self.start()
            a, b, c = [self.login(uid) for uid in uids]
            sa, sb, sc = [self.connect(account) for account in [a, b, c]]
            replay_cold = self.call('CSSideGetPvpMV', {'fightId': first.fightId}, b, 0, 'SCSideGetPvpMV')
            self.assertEqual(replay_cold.SerializeToString(), replay.SerializeToString())
            # A bounded prior-red-history fixture enables judicial eligibility.
            # It changes no battle number, winner, coin, source proof or receipt.
            player, game = self.profile(c)
            game['field']['side']['kills'] = 6
            self.write_profile(c, player, game)
            self.sql('INSERT INTO p12_field_pvp_states(player_id,kills,day,daily_kills,loot_coin,busy_until,last_fight_id,jail_until,redeem_count,source) '
                     'SELECT ' + str(c.playerId) + ',6,DATE_FORMAT(CONVERT_TZ(UTC_TIMESTAMP(),"+00:00","+08:00"),"%Y-%m-%d"),0,0,0,"",0,0,source FROM p12_field_pvp_policies WHERE id=1;')
            for sock in [sa, sb, sc]:
                self.drain(sock)
            self.tcp_send(sa, 'CSSidePvpFightResult', {'playerId': c.playerId, 'fightType': 1}, a)
            sender_frames = self.pk_delivery(sa)
            judicial_peer_frames = self.pk_delivery(sc)
            send = next(value for op, value in sender_frames if op == BY_NAME['SCSidePvpFightResult'])
            self.assertEqual((send.code, send.winPlayerId), (1, a.playerId))
            judicial = self.receipt(send.fightId)
            self.assertEqual(judicial['loot'], 0)
            judicial_terminal = json.loads(bytes.fromhex(judicial['terminal']))
            report['judicialSocketDeliveries'] = [verify_pk_delivery(sender_frames, send, judicial_terminal, a.playerId),
                                                  verify_pk_delivery(judicial_peer_frames, send, judicial_terminal, c.playerId)]
            self.wait_busy(judicial)
            query = self.call('CSPlayerRedeem', {'type': 0}, c, 0, 'SCPlayerRedeem')
            self.assertGreater(query.redeemTime, int(time.time() * 1000))
            self.assertEqual([(cost.id, cost.num) for cost in query.cost], [('coin', 10000)])
            self.assertEqual(self.profile(c)[1]['position']['scene'], 'town_main')
            report['prisonControlRejections'] = [
                self.reject('CSTransferScene', {'target': 'field_002'}, c, reason='刑期'),
                self.reject('CSResurrection', {}, c, stamp=3001, reason='刑期')]
            # Original judicial callback transfers to this exact original Town point.
            prison_point = self.profile(c)[1]['position']['current']
            self.call('CSTransferScene', {'target': 'town_main', 'point': prison_point}, c, 0, 'SCTransferScene')
            prison_position = self.call('CSCurrentPosition', {}, c, 0, 'SCCurrentPosition')
            self.assertEqual(prison_position.self.position.scene, 'town_main')
            self.assertEqual(prison_position.self.info.playerSide.kills, 6)
            paid = self.call('CSPlayerRedeem', {'type': 1}, c, 0, 'SCPlayerRedeem')
            self.assertEqual(paid.redeemTime, 0)
            after_pay = self.profile(c)
            self.assertEqual(after_pay[0]['coin'], 90000)
            self.assertEqual(after_pay[1]['field']['side']['kills'], 0)
            self.assertEqual(after_pay[1]['position']['scene'], 'town_main')
            freed_position = self.call('CSCurrentPosition', {}, c, 0, 'SCCurrentPosition')
            self.assertEqual(freed_position.self.info.playerSide.kills, 0)
            self.call('CSPlayerRedeem', {'type': 1}, c, 0, 'SCPlayerRedeem')
            self.assertEqual(self.profile(c)[0]['coin'], 90000)
            self.assertEqual(self.sql('SELECT redeem_count FROM p12_field_pvp_states WHERE player_id=' + str(c.playerId) + ';').strip(), '1')
            self.close_sockets(); self.stop(); self.start()
            a, b, c = [self.login(uid) for uid in uids]
            self.assertEqual(self.profile(c)[0]['coin'], 90000)
            self.assertEqual(self.call('CSPlayerRedeem', {'type': 0}, c, 0, 'SCPlayerRedeem').redeemTime, 0)
            self.assertEqual(self.sql('SELECT SHA2(COALESCE(player,""),256),SHA2(COALESCE(game,""),256) FROM profiles WHERE id=1;'), protected)
            self.assertEqual(self.sql('SELECT authoritative_enabled FROM p6_combat_rules WHERE id=1;').strip(), '0')
            report.update(status='PASS_ORIGINAL_HTTP_TCP_INNODB_BATCH', judicial={k: v for k, v in judicial.items() if k != 'terminal'},
                          naturalSentenceExpiry='domain boundary test only; not waited in protocol batch',
                          phoneUI=False, exactOnceTCP='same-request six-second lease only; no infinite delayed retry identity')
        except Exception as error:
            report.update(status='FAIL', failure=self.pool.redact(str(error))[:1500], failureType=type(error).__name__)
            raise
        finally:
            try:
                self.close_sockets(); self.stop()
            finally:
                try:
                    self.pool.close()
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    report['ownedAppStopped'] = self.process is None or self.process.poll() is not None
                    report['ownedSocketsRemaining'] = len(self.sockets)
                finally:
                    if self.app_temp is not None:
                        self.app_temp.cleanup()
                    fcntl.flock(lock.fileno(), fcntl.LOCK_UN); lock.close()
                    report['seconds'] = round(time.monotonic() - began, 3)
                    report['frameOpcodes'] = self.frame_opcodes
                    REPORT.parent.mkdir(parents=True, exist_ok=True)
                    REPORT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
