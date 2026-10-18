from hyjs_managed_mysql import provision_case
"""Real two-realm stage7 profile upload and query, independent of battle results."""
import concurrent.futures
import csv
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import tempfile
import unittest

import test_p14_cross_mysql as cross
from test_p14_qualification_native_mysql import P14QualificationNativeMySQLTests as qualified
from server.codec import BY_NAME


@unittest.skipUnless(os.environ.get('HYJS_P14_FORMATION_NATIVE_MYSQL_TEST') == '1', 'explicit isolated stage7 native')
class P14PromotionFormationMySQLTests(unittest.TestCase):
    root_sql = classmethod(cross.P14CrossMySQLTests.root_sql.__func__)
    sql = cross.P14CrossMySQLTests.sql
    setUp = cross.P14CrossMySQLTests.setUp
    stop_all = cross.P14CrossMySQLTests.stop_all
    send = cross.P14CrossMySQLTests.send
    call = cross.P14CrossMySQLTests.call
    login = cross.P14CrossMySQLTests.login
    wait_sql = cross.P14CrossMySQLTests.wait_sql
    tcp = cross.P14CrossMySQLTests.tcp
    tcp_send = cross.P14CrossMySQLTests.tcp_send
    internal = cross.P14CrossMySQLTests.internal
    start = qualified.start
    stop = qualified.stop
    profiles = qualified.profiles

    @classmethod
    def setUpClass(cls):
        cls.source = Path(os.environ['HYJS_P14_FORMATION_SOURCE'])
        assert cls.source.name.startswith('hyjs-p13-natural-missions-')
        cls.prebuilt = cls.source / 'prebuilt'
        cls.meta = json.loads((cls.prebuilt / 'manifest.json').read_text())
        for path, sha in cls.meta['files'].items():
            assert hashlib.sha256((cls.source / path).read_bytes()).hexdigest() == sha, path
        cls.binary = cls.prebuilt / 'hyjs-native'
        assert hashlib.sha256(cls.binary.read_bytes()).hexdigest() == cls.meta['nativeSHA256']
        assert hashlib.sha256((cls.prebuilt / 'service-race.test').read_bytes()).hexdigest() == cls.meta['raceSHA256']
        cls.lock = open('/tmp/hyjs-isolated-mysql.lock', 'a+')
        fcntl.flock(cls.lock, fcntl.LOCK_EX)
        cls.addClassCleanup(lambda: (fcntl.flock(cls.lock, fcntl.LOCK_UN), cls.lock.close()))
        pool, primary = provision_case(cls, 'p13_p14_form_domain')
        cls.realm_connections = {zone: pool.allocate('p13_p14_form' + str(zone)) for zone in [1, 2]}
        cls.realms = {zone: connection.database for zone, connection in cls.realm_connections.items()}
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p14-formation-clock-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.clock = Path(cls.temp.name) / 'p13-clock'
        cls.clock.write_text(str(int(time.time() * 1000)))
        cls.env['HYJS_P13_FIELD_NATIVE_CLOCK'] = str(cls.clock)

    def test_1_actual_mysql_race_qualified_upload(self):
        began = time.monotonic()
        self.shutdownTimes = []
        result = subprocess.run(['rtk', 'proxy', str(self.prebuilt / 'service-race.test'), '-test.v', '-test.run=^TestP14PromotionFormation', '-test.count=1', '-test.timeout=180s'], cwd=self.source / 'hyjs-server/service', env=self.env, capture_output=True, text=True, timeout=210)
        self.assertEqual(result.returncode, 0, (result.stdout + result.stderr).replace(self.test_password, '[redacted]'))
        self.assertEqual(result.stdout.count('--- PASS: TestP14PromotionFormation'), 2)
        self.assertNotIn('no tests to run', result.stdout + result.stderr)
        print(result.stdout.strip())

    def test_2_two_native_realms_stage7_upload_type2_outbox_cold(self):
        began = time.monotonic()
        self.shutdownTimes = []
        self.start(1)
        self.start(2)
        uids = {zone: ['prom-upload-' + str(zone) + '-' + secrets.token_hex(4) for _ in range(2)] for zone in [1, 2]}
        players = {zone: [self.login(zone, uid) for uid in uids[zone]] for zone in [1, 2]}
        self.assertEqual([x.playerId for x in players[1]], [x.playerId for x in players[2]])
        for zone in [1, 2]:
            for i, player in enumerate(players[zone]):
                self.call(zone, 'CSPlayerCreate', {'playerName': '晋级服' + str(zone) + '员' + str(i), 'heroId': 'hero_047'}, player, 2, 'SCPlayerCreate')
            self.sql(zone, 'UPDATE profiles SET player=JSON_SET(player,"$.level",70);UPDATE p10_scene_policies SET enabled=0;')
            for player in players[zone]:
                self.call(zone, 'CSPlayer', {}, player, 3, 'SCPlayer')
        now = int(time.time() * 1000)
        slots = {1: (now - 2000, now - 1000), 2: (now - 1000, now + 14000), 3: (now + 14000, now + 15000), 4: (now + 15000, now + 35000), 5: (now + 35000, now + 36000), 6: (now + 36000, now + 37000), 7: (now + 37000, now + 300000)}
        for stage in range(8, 20):
            slots[stage] = (now + 300000 + (stage - 8) * 1000, now + 301000 + (stage - 8) * 1000)
        values = ','.join('(45,' + str(a) + ',' + str(stage) + ',' + str(b) + ',"原阶段' + str(stage) + '","isolated actual stage7 upload calendar")' for stage, (a, b) in slots.items())
        key = secrets.token_bytes(32)
        for zone in [1, 2]:
            self.sql(zone, 'UPDATE p14_season_policies SET enabled=1,season_index=45,register_start=' + str(now - 1000) + ',register_end=' + str(now + 14000) + ';INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES' + values + ';INSERT INTO p14_trusted_peers(server_id,enabled,endpoint,signing_key) VALUES(' + str(3 - zone) + ',1,"http://127.0.0.1:' + str(self.ports[3 - zone][0]) + '",UNHEX("' + key.hex() + '"));UPDATE p14_audition_match_policies SET enabled=1;UPDATE p14_qualification_federation_policies SET enabled=1;')
            for player in players[zone]:
                self.call(zone, 'CSKFEnterGame', {}, player, 400 + player.playerId, 'SCKFEnterGame')
        for zone in [1, 2]:
            self.wait_sql(zone, 'SELECT COUNT(*) FROM p14_foreign_qualifications WHERE season_index=45;', '1', timeout=45)
        while int(time.time() * 1000) < slots[7][0]:
            time.sleep(.1)
        originals = {zone: self.sql(zone, 'SELECT server_id,player_id,revision,snapshot_hash FROM p14_members ORDER BY server_id,player_id;') + self.sql(zone, 'SELECT source_hash FROM p14_audition_rosters ORDER BY player_id;') + self.sql(zone, 'SELECT source_hash FROM p14_audition_qualifications;') for zone in [1, 2]}
        for zone in [1, 2]:
            op, _ = self.send(zone, 'CSKFEnterFrightGame', {}, players[zone][0], 500)
            self.assertEqual(op, 10001, 'stage7 independent defaultoff')
            self.sql(zone, 'UPDATE p14_promotion_policies SET enabled=1;')
            fallback = self.call(zone, 'CSKFPlayerInfo', {'playerId': players[3 - zone][0].playerId, 'serverId': 3 - zone, 'type': 2, 'peakMatchIndex': 45}, players[zone][0], 501, 'SCKFPlayerInfo')
            self.assertEqual(fallback.formationInfo.name, '晋级服' + str(3 - zone) + '员0')
        # The authenticated original formation command changes a real active
        # battle position. No snapshot or qualification rows are seeded.
        with (self.source / 'analysis/tables/TableFormation.tsv').open(encoding='utf-8-sig') as table:
            rows = list(csv.reader(table, delimiter='\t'))
            definition = next(dict(zip(rows[1], row)) for row in rows[5:] if row and row[0] == '1')
        positions = [int(x) for x in definition['posInfo'].split(',')]
        for zone in [1, 2]:
            formation = self.call(zone, 'CSFormationList', {}, players[zone][0], 502, 'SCFormationList')
            active = [x for x in formation.formationList if any(t.id.startswith('hero_') for t in x.things)]
            self.assertTrue(active)
            mapping = ';'.join(str(x.formatPos) + ',' + str(positions[(positions.index(x.battlePos) + 1) % len(positions)]) for x in active)
            self.call(zone, 'CSFormationChange', {'fomationMapStr': mapping}, players[zone][0], 503, 'SCFormationChange')
        before = {zone: self.profiles(zone, players[zone]) for zone in [1, 2]}
        fixed = dict(self.ports)
        self.stop(2)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            uploads = list(pool.map(lambda _: self.call(1, 'CSKFEnterFrightGame', {}, players[1][0], 504, 'SCKFEnterFrightGame'), range(8)))
        self.assertTrue(all(x == uploads[0] for x in uploads))
        self.assertGreater(uploads[0].lastFightPower, 0)
        self.wait_sql(1, 'SELECT COUNT(*) FROM p14_deliveries WHERE confirmed=0 AND JSON_EXTRACT(CONVERT(payload USING utf8mb4),"$.protocolVersion")=2 AND attempt_count>0;', '1', timeout=20)
        payload = bytes.fromhex(self.sql(1, 'SELECT HEX(payload) FROM p14_deliveries WHERE JSON_EXTRACT(CONVERT(payload USING utf8mb4),"$.protocolVersion")=2;').strip())
        obj = json.loads(payload)
        self.assertEqual(obj['protocolVersion'], 2)
        self.stop(1)
        self.start(1, fixed[1])
        self.start(2, fixed[2])
        players = {zone: [self.login(zone, uid) for uid in uids[zone]] for zone in [1, 2]}
        self.wait_sql(1, 'SELECT COUNT(*) FROM p14_deliveries WHERE confirmed=0;', '0')
        self.wait_sql(2, 'SELECT COUNT(*) FROM p14_promotion_formations WHERE server_id=1;', '1')
        for zone in [1, 2]:
            self.call(zone, 'CSKFEnterFrightGame', {}, players[zone][0], 505, 'SCKFEnterFrightGame')
        for zone in [1, 2]:
            self.wait_sql(zone, 'SELECT COUNT(DISTINCT server_id) FROM p14_promotion_formations;', '2')
            old = self.sql(zone, 'SELECT server_id,player_id,revision,snapshot_hash FROM p14_members ORDER BY server_id,player_id;') + self.sql(zone, 'SELECT source_hash FROM p14_audition_rosters ORDER BY player_id;') + self.sql(zone, 'SELECT source_hash FROM p14_audition_qualifications;')
            self.assertEqual(old, originals[zone], 'promotion rewrote real signup/audition identity')
            self.assertEqual(self.profiles(zone, players[zone]), before[zone], 'upload mutated resources/profile')
            self.assertEqual(self.sql(zone, 'SELECT COUNT(*) FROM p14_promotion_states;').strip(), '0')
            self.assertEqual(self.sql(zone, 'SELECT COUNT(*) FROM p14_audition_matches;').strip(), '0')
            sock = self.tcp(zone, players[zone][0])
            self.tcp_send(zone, sock, 'CSKFPlayerInfo', {'playerId': players[3 - zone][0].playerId, 'serverId': 3 - zone, 'peakMatchIndex': 45, 'type': 2}, players[zone][0])
            op, reply = cross.guild.P13MySQLTests.recv(self, sock)
            self.assertEqual((op, reply.formationInfo.name), (BY_NAME['SCKFPlayerInfo'], '晋级服' + str(3 - zone) + '员0'))
            self.assertEqual(self.sql(zone, 'SELECT COUNT(*) FROM p14_promotion_formations f JOIN p14_members m USING(season_index,server_id,player_id) WHERE f.snapshot_hash<>m.snapshot_hash;').strip(), '2')
        status, reply = self.internal(2, payload, key)
        self.assertEqual((status, reply['code'], reply['data']['confirmed']), (200, 0, True))
        swapped = dict(obj, protocolVersion=1)
        _, reply = self.internal(2, json.dumps(swapped, separators=(',', ':')).encode(), key)
        self.assertNotEqual(reply['code'], 0, 'purpose-swapped replay accepted as signup')
        for zone in [1, 2]:
            self.assertEqual(self.sql(zone, 'SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA="' + self.realms[zone] + '" AND TABLE_NAME="p14_promotion_formations";').strip(), 'InnoDB')
            self.stop(zone)
        print(json.dumps({'status': 'PASS_ACTUAL_TWO_REALM_STAGE7_FORMATION', 'seconds': round(time.monotonic() - began, 3), 'cleanShutdownSeconds': self.shutdownTimes, 'source': 'real original signup/cutoff/zero-roundauto/qualification federation; original formationChange->81030->signed purpose2/offlineoutbox/cold->original24907type2; original signup roster/qualification unchanged', 'authority': False, 'fixture': 'level70 only; no snapshot/qualification/match/score/resource injection'}, ensure_ascii=False))
