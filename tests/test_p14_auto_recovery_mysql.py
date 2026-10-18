"""Actual service outage covering the whole audition, then qualified stage7."""
import json
import os
import secrets
import subprocess
import time
from pathlib import Path
import hashlib
from server.codec import CLASSES
from google.protobuf.json_format import MessageToDict

import test_p14_cross_mysql as cross
from test_p14_promotion_formation_mysql import P14PromotionFormationMySQLTests as base


class P14AutoRecoveryMySQLTests(base):
    def test_1_actual_mysql_race_qualified_upload(self):
        self.shutdownTimes = []
        result = subprocess.run(['rtk', 'proxy', str(self.prebuilt / 'service-race.test'), '-test.v', '-test.run=^TestP14AutoRecovery', '-test.count=1', '-test.timeout=180s'], cwd=self.source / 'hyjs-server/service', env=self.env, capture_output=True, text=True, timeout=210)
        self.assertEqual(result.returncode, 0, (result.stdout + result.stderr).replace(self.test_password, '[redacted]'))
        self.assertEqual(result.stdout.count('--- PASS: TestP14AutoRecovery'), 3)
        self.assertNotIn('no tests to run', result.stdout + result.stderr)
        print(result.stdout.strip())

    def test_2_two_native_realms_stage7_upload_type2_outbox_cold(self):
        began = time.monotonic()
        self.shutdownTimes = []
        self.start(1)
        self.start(2)
        uids = {zone: ['auto-outage-' + str(zone) + '-' + secrets.token_hex(4) for _ in range(2)] for zone in [1, 2]}
        players = {zone: [self.login(zone, uid) for uid in uids[zone]] for zone in [1, 2]}
        self.assertEqual([x.playerId for x in players[1]], [x.playerId for x in players[2]])
        for zone in [1, 2]:
            for i, player in enumerate(players[zone]):
                self.call(zone, 'CSPlayerCreate', {'playerName': '停服晋级' + str(zone) + '员' + str(i), 'heroId': 'hero_047'}, player, 2, 'SCPlayerCreate')
            self.sql(zone, 'UPDATE profiles SET player=JSON_SET(player,"$.level",70);UPDATE p10_scene_policies SET enabled=0;')
            for player in players[zone]:
                self.call(zone, 'CSPlayer', {}, player, 3, 'SCPlayer')
        now = int(time.time() * 1000)
        slots = {1: (now - 2000, now - 1000), 2: (now - 1000, now + 14000), 3: (now + 14000, now + 15000), 4: (now + 15000, now + 20000), 5: (now + 20000, now + 21000), 6: (now + 21000, now + 22000), 7: (now + 22000, now + 300000)}
        for stage in range(8, 20):
            slots[stage] = (now + 300000 + (stage - 8) * 1000, now + 301000 + (stage - 8) * 1000)
        values = ','.join('(46,' + str(a) + ',' + str(stage) + ',' + str(b) + ',"原阶段' + str(stage) + '","isolated actual fullauditionoutage calendar")' for stage, (a, b) in slots.items())
        key = secrets.token_bytes(32)
        recovery = 'local-prepublished-zero-roundauto-downtime-recovery-v1; actual-qualifiedAt-observation; stage5-7; original-capacity1-10; no-round-or-match'
        for zone in [1, 2]:
            self.sql(zone, 'UPDATE p14_season_policies SET enabled=1,season_index=46,register_start=' + str(now - 1000) + ',register_end=' + str(now + 14000) + ';INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES' + values + ';INSERT INTO p14_trusted_peers(server_id,enabled,endpoint,signing_key) VALUES(' + str(3 - zone) + ',1,"http://127.0.0.1:' + str(self.ports[3 - zone][0]) + '",UNHEX("' + key.hex() + '"));UPDATE p14_audition_match_policies SET enabled=1,auto_recovery_enabled=1,auto_recovery_published_at=' + str(now) + ',auto_recovery_source="' + recovery + '";UPDATE p14_qualification_federation_policies SET enabled=1;UPDATE p14_promotion_policies SET enabled=1;')
            for player in players[zone]:
                self.call(zone, 'CSKFEnterGame', {}, player, 400 + player.playerId, 'SCKFEnterGame')
        before = {zone: self.profiles(zone, players[zone]) for zone in [1, 2]}
        fixed = dict(self.ports)
        for zone in [1, 2]:
            self.assertEqual(self.sql(zone, 'SELECT COUNT(*) FROM p14_audition_qualifications;').strip(), '0')
            self.assertEqual(self.sql(zone, 'SELECT COUNT(*) FROM p14_audition_matches;').strip(), '0')
            self.stop(zone)
        # Both real services remain down across every instant of stage4.
        while int(time.time() * 1000) < slots[7][0] + 100:
            time.sleep(.1)
        for zone in [1, 2]:
            self.start(zone, fixed[zone])
        players = {zone: [self.login(zone, uid) for uid in uids[zone]] for zone in [1, 2]}
        for zone in [1, 2]:
            self.wait_sql(zone, 'SELECT COUNT(*) FROM p14_audition_qualifications WHERE recovery_source="' + recovery + '";', '1')
            self.wait_sql(zone, 'SELECT COUNT(*) FROM p14_foreign_qualifications;', '1')
            observed = int(self.sql(zone, 'SELECT qualified_at FROM p14_audition_qualifications;').strip())
            self.assertGreaterEqual(observed, slots[7][0])
            self.assertLessEqual(observed, int(time.time() * 1000))
            self.assertEqual(self.sql(zone, 'SELECT round_count FROM p14_audition_qualifications;').strip(), '0')
            self.assertEqual(self.sql(zone, 'SELECT COUNT(*) FROM p14_audition_matches;').strip(), '0')
            result = self.call(zone, 'CSKuaFuMatchResult', {}, players[zone][0], 500, 'SCKuaFuMatchResult')
            self.assertEqual((result.result, result.matchResult.winTimes, result.matchResult.failTimes), (1, 0, 0))
            sock = self.tcp(zone, players[zone][0])
            self.tcp_send(zone, sock, 'CSKuaFuMatchResult', {}, players[zone][0])
            op, frame = cross.guild.P13MySQLTests.recv(self, sock)
            self.assertEqual((op, frame.result, frame.matchResult.winTimes, frame.matchResult.failTimes), (cross.BY_NAME['SCKuaFuMatchResult'], 1, 0, 0))
            sock.close()
            info = self.call(zone, 'CSKFOpenFrightServerInfo', {}, players[zone][0], 501, 'SCKFOpenFrightServerInfo')
            self.assertEqual((info.typeStage, info.isEnterCurStage), (7, 1))
            self.call(zone, 'CSKFEnterFrightGame', {}, players[zone][0], 502, 'SCKFEnterFrightGame')
        # Export immutable actual qualified source bytes for the combat agent.
        # The typed wrapper is an explicitly local read-only draft, never a match/result.
        if os.environ.get('HYJS_P14_TYPE17_ASSET_DIR'):
            destination = Path(os.environ['HYJS_P14_TYPE17_ASSET_DIR'])
            root = (Path.home() / 'LR/hyjs/analysis/p14')
            assert destination.parent == root and destination.name.startswith('native-type17-qualified-source-')
            destination.mkdir(exist_ok=False)
            sources = []
            battles = []
            for zone, index in [(1, 0), (2, 1)]:
                player = players[zone][index]
                columns = self.sql(zone, 'SELECT CONCAT(HEX(battle),"|",HEX(snapshot),"|",snapshot_hash,"|",source_hash,"|",configuration_hash,"|",engine_version,"|",client_version) FROM p14_audition_rosters WHERE season_index=46 AND server_id=' + str(zone) + ' AND player_id=' + str(player.playerId) + ';').strip().split('|')
                assert len(columns) == 7
                raw = bytes.fromhex(columns[0])
                battle = CLASSES['BattlePlayer']()
                battle.ParseFromString(raw)
                assert battle.playerId == player.playerId and battle.serverId == zone
                name = 'realm' + str(zone) + '-player' + str(player.playerId) + '-original-battle.pb'
                (destination / name).write_bytes(raw)
                public = bytes.fromhex(columns[1])
                public_name = 'realm' + str(zone) + '-formation.pb'
                (destination / public_name).write_bytes(public)
                sources.append({'serverId': zone, 'playerId': player.playerId, 'battleFile': name, 'battleSHA256': hashlib.sha256(raw).hexdigest(), 'formationFile': public_name, 'formationSHA256': hashlib.sha256(public).hexdigest(), 'snapshotHash': columns[2], 'rosterSourceHash': columns[3], 'configurationHash': columns[4], 'engineVersion': columns[5], 'clientVersion': columns[6], 'qualification': self.sql(zone, 'SELECT source_hash,qualified_at,recovery_source FROM p14_audition_qualifications;').strip(), 'originalBattle': MessageToDict(battle, preserving_proto_field_name=True)})
                battles.append(battle)
            draft = CLASSES['SCKuafuMatchPlayer'](fightId='local-readonly-qualified-type17-draft', playerList=battles, randomSeed=secrets.randbelow(2147483646) + 1, maxRoundNum=40)
            assert not draft.HasField('roomId') and not draft.HasField('winPlayerId')
            draft_raw = draft.SerializeToString()
            (destination / 'typed-draft.pb').write_bytes(draft_raw)
            (destination / 'typed-draft.json').write_text(json.dumps(MessageToDict(draft, preserving_proto_field_name=True), ensure_ascii=False, indent=2) + '\n')
            proof = {'scope': 'actual original authenticated signup/cutoff/qualified source bytes; constructed local typed17 read-only draft; not an executed match or private-server input; no compute/authority/result', 'sources': sources, 'firstOrders': [battle.inst.firstOrder for battle in battles], 'sourceBytesUnchanged': True, 'roomId': 'omitted; original SetData fallback map_forest_001', 'winPlayerId': 'omitted', 'draftSHA256': hashlib.sha256(draft_raw).hexdigest(), 'nativeSHA256': self.meta['nativeSHA256'], 'authority': False}
            (destination / 'manifest.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
        hashes = {zone: self.sql(zone, 'SELECT source_hash,qualified_at,recovery_source,recovery_published_at FROM p14_audition_qualifications;') for zone in [1, 2]}
        for zone in [1, 2]:
            self.wait_sql(zone, 'SELECT COUNT(*) FROM p14_promotion_formations;', '2')
            self.assertEqual(self.profiles(zone, players[zone]), before[zone])
            self.sql(zone, 'UPDATE p14_audition_match_policies SET auto_recovery_enabled=0,auto_recovery_source="changedafterfulfillment";')
            self.stop(zone)
        for zone in [1, 2]:
            self.start(zone, fixed[zone])
            players[zone] = [self.login(zone, uid) for uid in uids[zone]]
            self.assertEqual(self.sql(zone, 'SELECT source_hash,qualified_at,recovery_source,recovery_published_at FROM p14_audition_qualifications;'), hashes[zone])
            info = self.call(zone, 'CSKFOpenFrightServerInfo', {}, players[zone][0], 503, 'SCKFOpenFrightServerInfo')
            self.assertEqual((info.typeStage, info.isEnterCurStage), (7, 1))
            self.assertEqual(self.profiles(zone, players[zone]), before[zone])
            self.assertEqual(self.sql(zone, 'SELECT COUNT(*) FROM p14_qualification_deliveries;').strip(), '1')
            self.stop(zone)
        print(json.dumps({'status': 'PASS_ACTUAL_TWO_REALM_WHOLE_AUDITION_OUTAGE_RECOVERY', 'seconds': round(time.monotonic() - began, 3), 'shutdownSeconds': self.shutdownTimes, 'scope': 'prepublished defaultoff local recovery; real two native services stopped across entirestage4; real signup members→stage7actualqualifiedAt→exact signedfederation→original0win0lossresult/admission/upload→cold remains; match0; profile unchanged', 'authority': False}, ensure_ascii=False))
