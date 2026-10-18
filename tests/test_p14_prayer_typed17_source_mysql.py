from hyjs_managed_mysql import provision_case
"""Capture actual qualified purpose2 teams after paid original guild prayer.

The output is a local typed17 diagnostic draft. This tool never computes a
winner, changes a BattlePlayer, or writes a match/result/score/reward.
"""
import fcntl
import hashlib
import http.client
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import unittest

from google.protobuf.json_format import MessageToDict
from server.codec import BY_NAME, CLASSES, decode_responses, encode_http_request
import test_p14_cross_mysql as cross
import test_p14_qualification_native_mysql as qualified

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P14_PRAYER_SOURCE_TEST') == '1', 'explicit isolated actual source capture')
class P14PrayerTyped17SourceMySQLTests(unittest.TestCase):
    root_sql = classmethod(cross.P14CrossMySQLTests.root_sql.__func__)
    sql = cross.P14CrossMySQLTests.sql
    setUp = cross.P14CrossMySQLTests.setUp
    stop_all = cross.P14CrossMySQLTests.stop_all
    call = cross.P14CrossMySQLTests.call
    login = cross.P14CrossMySQLTests.login
    wait_sql = cross.P14CrossMySQLTests.wait_sql
    tcp = cross.P14CrossMySQLTests.tcp
    tcp_send = cross.P14CrossMySQLTests.tcp_send
    start = qualified.P14QualificationNativeMySQLTests.start
    stop = qualified.P14QualificationNativeMySQLTests.stop
    profiles = qualified.P14QualificationNativeMySQLTests.profiles

    def send(self, zone, name, fields=None, login=None, stamp=1700000000000):
        request = CLASSES[name](**(fields or {}))
        raw = encode_http_request(BY_NAME[name], request, zone_id=zone, player_id=login.playerId if login else 0, session=bytes(login.sessionKey) if login else bytes(16), timestamp=stamp)
        connection = http.client.HTTPConnection('127.0.0.1', self.ports[zone][0], timeout=30)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            data = response.read()
            self.assertEqual(response.status, 200, data)
            return decode_responses(data)[0]
        finally:
            connection.close()

    @classmethod
    def setUpClass(cls):
        cls.source = Path(os.environ['HYJS_P14_PRAYER_SOURCE_ROOT']).resolve()
        assert cls.source.name.startswith('hyjs-p13-prayer-')
        cls.prebuilt = cls.source / 'prebuilt'
        cls.meta = json.loads((cls.prebuilt / 'manifest.json').read_text())
        for path, digest in cls.meta['files'].items():
            assert hashlib.sha256((cls.source / path).read_bytes()).hexdigest() == digest, path
        cls.binary = cls.prebuilt / 'hyjs-native'
        assert hashlib.sha256(cls.binary.read_bytes()).hexdigest() == cls.meta['native_sha256']
        cls.lock = open('/tmp/hyjs-isolated-mysql.lock', 'a+')
        fcntl.flock(cls.lock, fcntl.LOCK_EX)
        cls.addClassCleanup(lambda: (fcntl.flock(cls.lock, fcntl.LOCK_UN), cls.lock.close()))
        pool, primary = provision_case(cls, 'p13_prayer171')
        cls.realm_connections = {1: primary, 2: pool.allocate('p13_prayer172')}
        cls.realms = {zone: connection.database for zone, connection in cls.realm_connections.items()}
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p14-prayer-source-')
        cls.addClassCleanup(cls.temp.cleanup)
        # This source uses genuine wall time, including membership/prayer expiry.
        cls.env.pop('HYJS_P13_FIELD_NATIVE_CLOCK', None)

    def capture(self, zone, login, phase):
        stamp = 10000 + len(self.captures)
        self.call(zone, 'CSKFEnterFrightGame', {}, login, stamp, 'SCKFEnterFrightGame')
        where = 'season_index=47 AND server_id=' + str(zone) + ' AND player_id=' + str(login.playerId)
        columns = self.sql(zone, 'SELECT CONCAT(HEX(battle),"|",HEX(snapshot),"|",snapshot_hash,"|",source_hash,"|",configuration_hash,"|",engine_version,"|",client_version,"|",revision,"|",captured_at,"|",qualification_hash) FROM p14_promotion_formations WHERE ' + where + ';').strip().split('|')
        self.assertEqual(len(columns), 10)
        raw, formation = bytes.fromhex(columns[0]), bytes.fromhex(columns[1])
        battle = CLASSES['BattlePlayer']()
        battle.ParseFromString(raw)
        self.assertEqual((battle.playerId, battle.serverId, len(battle.formation)), (login.playerId, zone, 1))
        actor = battle.formation[0]
        self.assertEqual(actor.id, 'hero_047')
        self.assertEqual(actor.prop.perhp, 0, 'P5 must fold prayer once into the actual panel')
        self.assertEqual(actor.inst.curHp, actor.prop.hp)
        self.assertEqual(actor.prop.hp, 4641 if 'strong' in phase else 3094)
        self.assertEqual(actor.pose, 3)
        self.assertEqual(battle.inst.firstOrder, 0, 'preserve actual source firstOrder')
        prefix = 'realm' + str(zone) + '-player' + str(login.playerId) + '-' + phase
        (self.destination / (prefix + '-battle.pb')).write_bytes(raw)
        (self.destination / (prefix + '-formation.pb')).write_bytes(formation)
        # Retain the exact signed-purpose body, without saving any signing key.
        deliveries = self.sql(zone, 'SELECT HEX(payload) FROM p14_deliveries WHERE JSON_EXTRACT(CONVERT(payload USING utf8mb4),"$.protocolVersion")=2 AND JSON_EXTRACT(CONVERT(payload USING utf8mb4),"$.playerId")=' + str(login.playerId) + ' AND JSON_EXTRACT(CONVERT(payload USING utf8mb4),"$.revision")=' + columns[7] + ';').strip().splitlines()
        self.assertEqual(len(deliveries), 1)
        payload = bytes.fromhex(deliveries[0])
        (self.destination / (prefix + '-purpose2.json')).write_bytes(payload)
        self.wait_sql(3 - zone, 'SELECT revision FROM p14_promotion_formations WHERE ' + where + ';', columns[7])
        self.wait_sql(zone, 'SELECT COUNT(*) FROM p14_deliveries WHERE confirmed=0;', '0')
        foreign = self.sql(3 - zone, 'SELECT HEX(battle),source_hash FROM p14_promotion_formations WHERE ' + where + ';').strip().split('\t')
        self.assertEqual(foreign, [raw.hex().upper(), columns[3]])
        entry = {'serverId': zone, 'playerId': login.playerId, 'phase': phase, 'battleFile': prefix + '-battle.pb', 'battleSHA256': hashlib.sha256(raw).hexdigest(), 'formationFile': prefix + '-formation.pb', 'formationSHA256': hashlib.sha256(formation).hexdigest(), 'purpose2File': prefix + '-purpose2.json', 'purpose2SHA256': hashlib.sha256(payload).hexdigest(), 'snapshotHash': columns[2], 'sourceHash': columns[3], 'configurationHash': columns[4], 'engineVersion': columns[5], 'clientVersion': columns[6], 'revision': int(columns[7]), 'capturedAt': int(columns[8]), 'qualificationHash': columns[9], 'originalBattle': MessageToDict(battle, preserving_proto_field_name=True), 'actualForeignBytesEqual': True}
        self.captures.append(entry)
        return battle, entry

    def draft(self, name, left, right):
        draft = CLASSES['SCKuafuMatchPlayer'](fightId='local-readonly-prayer-' + name, playerList=[left[0], right[0]], randomSeed=secrets.randbelow(2147483646) + 1, maxRoundNum=40)
        self.assertFalse(draft.HasField('roomId'))
        self.assertFalse(draft.HasField('winPlayerId'))
        raw = draft.SerializeToString()
        (self.destination / (name + '.pb')).write_bytes(raw)
        (self.destination / (name + '.json')).write_text(json.dumps(MessageToDict(draft, preserving_proto_field_name=True), ensure_ascii=False, indent=2) + '\n')
        self.drafts.append({'file': name + '.pb', 'sha256': hashlib.sha256(raw).hexdigest(), 'sourceHashes': [left[1]['sourceHash'], right[1]['sourceHash']], 'hp': [left[0].formation[0].prop.hp, right[0].formation[0].prop.hp], 'firstOrders': [p.inst.firstOrder for p in draft.playerList], 'winner': 'omitted; no compute performed'})

    def pray(self, zone, login):
        before = self.profiles(zone, [login])[0][0]
        self.assertEqual(before.get('contribution', 0), 0)
        for n in range(5):
            self.call(zone, 'CSUnionDonate', {'donateType': 'coin'}, login, 2000 + n, 'SCUnionDonate')
        ready = self.profiles(zone, [login])[0][0]
        self.assertEqual(ready['contribution'], 100)
        self.assertEqual(before['coin'] - ready['coin'], 500000)
        self.call(zone, 'CSUnionPray', {}, login, 2100, 'SCUnionPray')
        info = self.call(zone, 'CSUnionLightChurchInfo', {}, login, 2101, 'SCUnionLightChurchInfo')
        self.assertEqual((info.prayTimes, info.contribution, info.addProp.perhp), (1, 0, 50))
        # Original zero means this authenticated realm. The independent
        # explicit realm2 profile-target bug is tracked for its own delivery.
        panel = self.call(zone, 'CSFindPlayerInfo', {'playerId': login.playerId, 'serverId': 0}, login, 2102, 'SCFindPlayerInfo').formationInfo
        self.assertEqual(panel.formationItemInfoList[0].warInfo.baseProp.hp, 4641)
        row = self.sql(zone, 'SELECT CONCAT(source_hash,"|",pool_hash,"|",HEX(effect),"|",applied_at,"|",expires_at,"|",contribution_cost) FROM p13_guild_prayer_rolls WHERE player_id=' + str(login.playerId) + ';').strip().split('|')
        self.prayers.append({'serverId': zone, 'playerId': login.playerId, 'actualDonations': 5, 'coinPaid': 500000, 'contributionEarnedAndSpent': 100, 'rollSourceHash': row[0], 'poolHash': row[1], 'effectHex': row[2], 'appliedAt': int(row[3]), 'expiresAt': int(row[4]), 'contributionCost': int(row[5]), 'publicHP': 4641})

    def test_actual_two_realms_paid_prayer_purpose2_strong_weak_source(self):
        began = time.monotonic()
        self.shutdownTimes, self.captures, self.drafts, self.prayers = [], [], [], []
        self.destination = ROOT / 'analysis/p14' / ('native-type17-prayer-source-' + secrets.token_hex(4))
        self.destination.mkdir(exist_ok=False)
        self.start(1)
        self.start(2)
        uids = {z: ['typed17-prayer-' + str(z) + '-' + secrets.token_hex(4) for _ in range(2)] for z in [1, 2]}
        players = {z: [self.login(z, uid) for uid in uids[z]] for z in [1, 2]}
        for z in [1, 2]:
            for i, login in enumerate(players[z]):
                self.call(z, 'CSPlayerCreate', {'playerName': '实源祈祷' + str(z) + '员' + str(i), 'heroId': 'hero_047'}, login, 2, 'SCPlayerCreate')
            self.sql(z, 'UPDATE profiles SET player=JSON_SET(player,"$.level",70,"$.coin",2000000,"$.diamond",1000);UPDATE p10_scene_policies SET enabled=0;UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=1;')
            for login in players[z]:
                self.call(z, 'CSPlayer', {}, login, 3, 'SCPlayer')
        chosen = {1: players[1][0], 2: players[2][1]}
        companions = {1: players[1][1], 2: players[2][0]}
        for z in [1, 2]:
            self.call(z, 'CSUnionCreate', {'name': '实源公会' + str(z)}, chosen[z], 100, 'SCUnionCreate')
            gid = self.call(z, 'CSGetUnionInfo', {}, chosen[z], 101, 'SCGetUnionInfo').unionInfo.unionId
            self.call(z, 'CSUnionApply', {'unionId': gid}, companions[z], 102, 'SCUnionApply')
            self.call(z, 'CSUnionAgreeApply', {'playerId': companions[z].playerId}, chosen[z], 103, 'SCUnionAgreeApply')
            pool = '[{"type":"perhp","value":50,"weight":1}]'
            source = 'local-prepublished-original-prayer-v1; poolweights-and-percent-operator-defined; original100contribution-day3-duration24h; singleeffect-overwrite'
            self.sql(z, "UPDATE p13_guild_prayer_policies SET enabled=1,version='local-typed17-actual-prayer-hp50-v1',source='" + source + "',pool_json='" + pool + "',pool_hash='" + hashlib.sha256(pool.encode()).hexdigest() + "',published_at=" + str(int(time.time() * 1000) - 1000) + ';')
        now = int(time.time() * 1000)
        slots = {1: (now - 2000, now - 1000), 2: (now - 1000, now + 14000), 3: (now + 14000, now + 15000), 4: (now + 15000, now + 35000), 5: (now + 35000, now + 36000), 6: (now + 36000, now + 37000), 7: (now + 37000, now + 600000)}
        for stage in range(8, 20):
            slots[stage] = (now + 600000 + (stage - 8) * 1000, now + 601000 + (stage - 8) * 1000)
        values = ','.join('(47,' + str(a) + ',' + str(stage) + ',' + str(b) + ',"原阶段' + str(stage) + '","local isolated actual source calendar")' for stage, (a, b) in slots.items())
        key = secrets.token_bytes(32)
        recovery = 'local-prepublished-zero-roundauto-downtime-recovery-v1; actual-qualifiedAt-observation; stage5-7; original-capacity1-10; no-round-or-match'
        for z in [1, 2]:
            self.sql(z, 'UPDATE p14_season_policies SET enabled=1,season_index=47,register_start=' + str(now - 1000) + ',register_end=' + str(now + 14000) + ';INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES' + values + ';INSERT INTO p14_trusted_peers(server_id,enabled,endpoint,signing_key) VALUES(' + str(3 - z) + ',1,"http://127.0.0.1:' + str(self.ports[3 - z][0]) + '",UNHEX("' + key.hex() + '"));UPDATE p14_audition_match_policies SET enabled=1,auto_recovery_enabled=1,auto_recovery_published_at=' + str(now) + ',auto_recovery_source="' + recovery + '";UPDATE p14_qualification_federation_policies SET enabled=1;UPDATE p14_promotion_policies SET enabled=1;')
            for login in players[z]:
                self.call(z, 'CSKFEnterGame', {}, login, 400 + login.playerId, 'SCKFEnterGame')
        # Existing <=10 zero-round qualification supplies this source. Keep
        # both processes running; the independently validated outage/recovery
        # lifecycle is not repeated for a paid-prayer source capture.
        for z in [1, 2]:
            self.wait_sql(z, 'SELECT COUNT(*) FROM p14_audition_qualifications;', '1', timeout=40)
            self.wait_sql(z, 'SELECT COUNT(*) FROM p14_foreign_qualifications;', '1')
        while int(time.time() * 1000) < slots[7][0] + 100:
            time.sleep(.1)
        originals = {z: self.sql(z, 'SELECT source_hash FROM p14_audition_rosters ORDER BY player_id;') for z in [1, 2]}
        self.assertTrue(all(value.strip() for value in originals.values()))
        weak_a = self.capture(1, chosen[1], 'weak-before-prayer')
        weak_b = self.capture(2, chosen[2], 'weak-before-prayer')
        self.pray(1, chosen[1])
        strong_a = self.capture(1, chosen[1], 'strong-paid-prayer')
        self.draft('strong-attacker-weak-defender', strong_a, weak_b)
        self.pray(2, chosen[2])
        strong_b = self.capture(2, chosen[2], 'strong-paid-prayer')
        self.draft('both-paid-prayer', strong_a, strong_b)
        self.call(1, 'CSTransferChairman', {'playerId': companions[1].playerId}, chosen[1], 3000, 'SCTransferChairman')
        self.call(1, 'CSQuitUnion', {}, chosen[1], 3001, 'SCQuitUnion')
        weak_after = self.capture(1, chosen[1], 'weak-after-real-leave')
        self.assertEqual(weak_after[0].formation[0].prop, weak_a[0].formation[0].prop)
        self.draft('weak-attacker-strong-defender', weak_after, strong_b)
        for z in [1, 2]:
            self.assertEqual(self.sql(z, 'SELECT source_hash FROM p14_audition_rosters ORDER BY player_id;'), originals[z])
            for table in ['p14_audition_matches', 'p14_promotion_states', 'p14_promotion_grants']:
                self.assertEqual(self.sql(z, 'SELECT COUNT(*) FROM ' + table + ';').strip(), '0')
            self.stop(z)
        proof = {'status': 'PASS_ACTUAL_PAID_PRAYER_TO_QUALIFIED_PURPOSE2_SOURCE', 'seconds': round(time.monotonic() - began, 3), 'scope': 'actual two isolated realms original signup/zero-round qualification/genuine guild donations/prayer/original81030 purpose2/signed peer imports; local typed17 readonly drafts only; no calculation/winner/result/reward', 'finitePrerequisites': 'four original created047 accounts level70/coin2m/diamond1000; local prepublished prayer perhp50 pool; no contribution/roll/BattlePlayer/qualification injected', 'prayers': self.prayers, 'sources': self.captures, 'drafts': self.drafts, 'sourceProofSHA256': hashlib.sha256((self.prebuilt / 'manifest.json').read_bytes()).hexdigest(), 'shutdownSeconds': self.shutdownTimes, 'roomId': 'omitted; original map_forest_001 fallback', 'authority': False, 'phone': False, 'productionModified': False}
        (self.destination / 'manifest.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
        print('actual paid prayer typed17 source PASS', self.destination, proof['seconds'])


if __name__ == '__main__':
    unittest.main()
