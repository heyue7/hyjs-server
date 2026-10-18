"""Published local P13 zero-combat round through original HTTP/TCP and InnoDB.

The caller prebuilds the immutable source outside the shared MySQL lock. This
suite owns its schema/user and checks actual signup, mining, result and P1 claim.
It never enables combat authority or connects to the demonstration save.
"""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import unittest
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo

import test_p13_mysql as guild
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

ROOT = Path(__file__).resolve().parents[1]
SOURCE = Path(os.environ.get('HYJS_P13_FIELD_SOURCE_ROOT', str(ROOT))).resolve()


@unittest.skipUnless(os.environ.get('HYJS_P13_SETTLEMENT_MYSQL_TEST') == '1', 'explicit isolated P13 settlement')
class P13UnionFightSettlementMySQLTests(unittest.TestCase):
    setUp = guild.P13MySQLTests.setUp
    login = guild.P13MySQLTests.login
    call = guild.P13MySQLTests.call
    stamped = guild.P13MySQLTests.stamped
    profile = guild.P13MySQLTests.profile
    send = guild.P13MySQLTests.send
    recv = guild.P13MySQLTests.recv
    tcp_send = guild.P13MySQLTests.tcp_send
    connect = guild.P13MySQLTests.connect
    sql = guild.P13MySQLTests.sql
    root_sql = classmethod(guild.P13MySQLTests.root_sql.__func__)
    set_clock = field.P13UnionFightFieldMySQLTests.set_clock
    start = field.P13UnionFightFieldMySQLTests.start
    receive = field.P13UnionFightFieldMySQLTests.receive

    @classmethod
    def setUpClass(cls):
        if not SOURCE.name.startswith(('hyjs-p13-zero-settlement-', 'hyjs-p13-transport-mvp-', 'hyjs-p13-natural-missions-')):
            raise AssertionError('fixed settlement source required')
        cls.prebuilt = SOURCE / 'prebuilt'
        cls.proof = json.loads((cls.prebuilt / 'manifest.json').read_text())
        for relative, digest in cls.proof['files'].items():
            if hashlib.sha256((SOURCE / relative).read_bytes()).hexdigest() != digest:
                raise AssertionError('fixed source drift ' + relative)
        for name, key in [('hyjs-native', 'native_sha256'), ('service-race.test', 'race_sha256')]:
            if hashlib.sha256((cls.prebuilt / name).read_bytes()).hexdigest() != cls.proof[key]:
                raise AssertionError('prebuilt drift ' + name)
        guild.P13MySQLTests.setUpClass.__func__(cls)
        cls.binary = str(cls.prebuilt / 'hyjs-native')
        cls.native_root = SOURCE

    def stop(self):
        if self.process is not None and self.process.poll() is None:
            self.process.terminate()
            try:
                self.assertEqual(self.process.wait(timeout=15), 0, 'ctx clean shutdown')
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
                self.fail('native shutdown timeout')
        if self.process is not None and self.process.stdout:
            self.process.stdout.close()

    def test_1_actual_mysql_race_zero_round_and_refused_combat(self):
        result = subprocess.run(['rtk', 'proxy', str(self.prebuilt / 'service-race.test'),
                                 '-test.run', '^TestP13ZeroFight', '-test.count=1', '-test.timeout=180s'],
                                cwd=SOURCE / 'hyjs-server/service', env=self.env,
                                capture_output=True, text=True, timeout=200)
        self.assertEqual(result.returncode, 0, (result.stdout + result.stderr).replace(self.test_password, '[redacted]'))
        print(result.stdout.strip())

    def test_2_original_tcp_http_signup_mine_result_p1_offline_cold(self):
        # Remove only the preceding isolated test's P13 facts; native users are fresh.
        tables = self.sql("SHOW TABLES LIKE 'p13_union_fight_%';").splitlines()
        if tables:
            self.sql(' '.join('DELETE FROM `' + table + '`;' for table in tables
                              if table not in ['p13_union_fight_policies', 'p13_union_fight_field_policies',
                                               'p13_union_fight_settlement_policies']))
        self.clock = Path(self.temp.name) / 'clock'
        monday = datetime(2026, 10, 5 if getattr(self, 'weekly_rewards', False) else 12, 12, tzinfo=ZoneInfo('Asia/Shanghai'))
        day = monday.replace(hour=0) + timedelta(days=1)
        calendar_day = day.strftime('%Y-%m-%d')
        first = int((day + timedelta(hours=20)).timestamp() * 1000)
        self.set_clock(int(monday.timestamp() * 1000))
        self.env['HYJS_P13_FIELD_NATIVE_CLOCK'] = str(self.clock)
        self.start()
        uids = ['p13-zero-native-' + secrets.token_hex(5) for _ in range(9)]
        players = [self.login(uid) for uid in uids]
        for index, login in enumerate(players):
            self.call('CSPlayerCreate', {'playerName': '原终局' + str(index), 'heroId': 'hero_047'}, login, 1, 'SCPlayerCreate')
        ids = ','.join(str(x.playerId) for x in players)
        self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000,"$.coin",20000000) WHERE id IN (' + ids + '); '
                 'UPDATE p13_guild_policies SET management_enabled=1; '
                 'UPDATE p13_union_fight_policies SET enrollment_enabled=1,match_enabled=0; '
                 'UPDATE p13_union_fight_settlement_policies SET enabled=0;')
        guild_ids = []
        for index, login in enumerate(players[:6]):
            self.call('CSUnionCreate', {'name': '终局实会' + str(index)}, login, 2, 'SCUnionCreate')
            gid = self.call('CSGetUnionInfo', {}, login, 3, 'SCGetUnionInfo').unionInfo.unionId
            guild_ids.append(gid)
            self.sql("UPDATE p13_guilds SET level=2 WHERE id='" + gid + "';")
        # Actual enrollment produces powers 3x/2x/1x/1x/1x/1x, without assigning a winner.
        for extra, leader in [(6, 0), (7, 0), (8, 1)]:
            self.call('CSUnionApply', {'unionId': guild_ids[leader]}, players[extra], 4, 'SCUnionApply')
            self.call('CSUnionAgreeApply', {'playerId': players[extra].playerId}, players[leader], 40 + extra, 'SCUnionAgreeApply')
        for login in players[:6]:
            self.call('CSUFUnionApply', {}, login, 5, 'SCUFUnionApply')
        for login in players:
            self.call('CSUFPlayerApply', {}, login, 6, 'SCUFPlayerApply')
        self.stop()
        published = first - 86400000
        self.sql("INSERT INTO p13_union_fight_schedules(day,first_start_at,second_start_at,duration_millis,source) VALUES('" + calendar_day + "'," + str(first) + ',' + str(first + 600000) + ",300000,'isolated published two-round calendar'); "
                 "UPDATE p13_union_fight_policies SET match_enabled=1; "
                 "UPDATE p13_union_fight_field_policies SET enabled=1,preparation_millis=300000,left_revive_point_id=1,right_revive_point_id=3,initial_guard_hp=1000,initial_gate_hp=10000,move_tolerance=3,source='explicit local scene HP; no combat authority'; "
                 "UPDATE p13_union_fight_settlement_policies SET enabled=1,version='local-item311-v1',source='isolated operator publication; prestige archived review, item311 UI supported; quantities locally chosen, not original private source',participation_rewards='item_311,1',victory_rewards='item_311,2',win_prestige=50,lose_prestige=25,published_at=" + str(published) + ";")
        if getattr(self, 'transport_mvp', False):
            self.sql("UPDATE p13_union_fight_settlement_policies SET transport_mvp_rewards='item_311,4';")
        if getattr(self, 'natural_missions', False):
            self.sql('UPDATE p13_union_fight_settlement_policies SET personal_missions=\'[{"kind":1,"target":2,"score":5},{"kind":2,"target":2,"score":7}]\', task_rewards=\'item_311,6\';')
        if getattr(self, 'weekly_rewards', False):
            self.sql("INSERT INTO p13_union_fight_weekly_policies(week_start,`group`,enabled,version,source,rewards,published_at) VALUES('" + monday.strftime('%Y-%m-%d') + "','B',1,'local-week-prize-v1','explicit local weekly quantities8/4; Sunday actual participants','[{\"rank\":1,\"rewards\":\"item_311,8\"},{\"rank\":2,\"rewards\":\"item_311,4\"}]'," + str(published) + ");")
        self.set_clock(int((day + timedelta(minutes=31)).timestamp() * 1000))
        self.start()
        players = [self.login(uid) for uid in uids]
        # Initial real matching freezes the full tables/rosters. Observe that
        # worker commit before issuing UI queries, without seeding match rows.
        for _ in range(60):
            matched = self.sql("SELECT COUNT(*) FROM p13_union_fight_matches WHERE `group`='B' AND round=1;").strip()
            offers = self.sql("SELECT COUNT(*) FROM p13_union_fight_settlement_offers;").strip()
            if matched == '1' and int(offers) >= 4:
                break
            time.sleep(.5)
        else:
            self.fail('actual matching/offer worker did not finish')
        self.call('CSUFCurrentStageInfo', {}, players[1], 7, 'SCUFCurrentStageInfo')
        # Read the actual group B pairing created from real frozen enrollments.
        pair = self.sql("SELECT CONCAT(id,'|',left_guild_id,'|',right_guild_id) FROM p13_union_fight_matches WHERE `group`='B' AND round=1;").strip().split('|')
        self.assertEqual(len(pair), 3, pair)
        match_id, left, right = pair
        selected = [guild_ids.index(gid) for gid in [left, right]]
        a, b = [players[i] for i in selected]
        for index, x, y, z in [(0, -4.7, -.12, 9.98), (1, 80.22, -.33, -13.36)]:
            self.sql("INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES('" + calendar_day + "','zero-native-" + str(index) + "',1,'1'," + str(x) + ',' + str(y) + ',' + str(z) + ',2,' + str(first - 60000) + ",'isolated original mine type1 at original recycle NPC');")
        self.set_clock(first - 60000)
        sockets = [self.connect(login) for login in [a, b]]
        if getattr(self, 'weekly_rewards', False):
            for sock in sockets: sock.settimeout(20)
        for login, sock in zip([a, b], sockets):
            self.call('CSTransferScene', {'target': 'fight_union'}, login, 8, 'SCTransferScene')
            self.tcp_send(sock, 'CSCurrentPosition', {}, login)
            self.receive(sock, 'SCUnionFightTransfer')
        stage = self.call('CSUFCurrentStageInfo', {}, a, 9, 'SCUFCurrentStageInfo')
        self.assertEqual(stage.stage, 3)
        self.set_clock(first + 20000)
        self.assertEqual(self.call('CSUFCurrentStageInfo', {}, a, 10, 'SCUFCurrentStageInfo').stage, 4)
        point = {'x': -4.7, 'y': -.12, 'z': 9.98}
        self.tcp_send(sockets[0], 'CSMove', {'pointNew': point}, a)
        self.set_clock(first + 40000)
        self.tcp_send(sockets[0], 'CSMove', {'pointNew': point}, a)
        guid = int(self.sql("SELECT guid FROM p13_union_fight_resource_nodes WHERE match_id='" + match_id + "' AND ABS(x+4.7)<0.001;").strip())
        if getattr(self, 'natural_missions', False):
            self.tcp_send(sockets[0], 'CSUFPlayerMissionList', {}, a)
            missions = self.receive(sockets[0], 'SCUFPlayerMissionList').missionInfos
            self.assertEqual([(m.progress, m.isComplete) for m in missions], [('0/2', 0), ('0/2', 0)])
            self.tcp_send(sockets[1], 'CSUFUnionMissionList', {}, b)
            self.assertEqual(len(self.receive(sockets[1], 'SCUFUnionMissionList').missionInfos), 0)
        self.tcp_send(sockets[0], 'CSUFPlayerCollect', {'resGuid': guid}, a)
        self.receive(sockets[0], 'SCUFPlayerCollect')
        self.set_clock(first + 43000)
        time.sleep(1.2)  # Wait for the real field worker to complete the original 3-second mine.
        if getattr(self, 'natural_missions', False):
            self.assertEqual(self.receive(sockets[0], 'SCUFMissionPush').playerFightScore, 5)
        if getattr(self, 'transport_mvp', False):
            self.tcp_send(sockets[0], 'CSUFPlayerUpResource', {'npcId': '2'}, a)
            deposit = self.receive(sockets[0], 'SCUFPlayerUpResource')
            if getattr(self, 'natural_missions', False):
                self.assertEqual(self.receive(sockets[0], 'SCUFMissionPush').playerFightScore, 12)
            self.assertEqual(self.sql("SELECT CONCAT(ore,'|',deposits) FROM p13_union_fight_transport_stats WHERE match_id='" + match_id + "' AND player_id=" + str(a.playerId) + ";").strip(), '2|1')
            self.tcp_send(sockets[0], 'CSUFPlayerUpResource', {'npcId': '2'}, a)
            for _ in range(80):
                op, _ = self.recv(sockets[0])
                if op == 10001: break
            else: self.fail('empty TCP0 ore retry did not reject')
            self.assertEqual(self.sql("SELECT CONCAT(ore,'|',deposits) FROM p13_union_fight_transport_stats WHERE match_id='" + match_id + "' AND player_id=" + str(a.playerId) + ";").strip(), '2|1')
        else:
            deposit = self.call('CSUFPlayerUpResource', {'npcId': '2'}, a, 11, 'SCUFPlayerUpResource')
        self.assertEqual(deposit.resourceNum, 2)
        if getattr(self, 'natural_missions', False):
            self.tcp_send(sockets[0], 'CSUFPlayerMissionList', {}, a)
            missions = self.receive(sockets[0], 'SCUFPlayerMissionList').missionInfos
            self.assertEqual([(m.progress, m.isComplete, m.rewardNum) for m in missions], [('2/2', 1, 5), ('2/2', 1, 7)])
            self.tcp_send(sockets[1], 'CSUFPlayerMissionList', {}, b)
            self.assertEqual([m.isComplete for m in self.receive(sockets[1], 'SCUFPlayerMissionList').missionInfos], [0, 0])
        self.set_clock(first + 299999)
        op, _ = self.stamped('CSUFFightResult', {}, a, 12)
        self.assertEqual(op, 10001)
        self.set_clock(first + 300000)
        stage = self.call('CSUFCurrentStageInfo', {}, a, 13, 'SCUFCurrentStageInfo')
        self.assertEqual(stage.stage, 5)
        self.assertIn(stage.firstFightInfo.fightResult, [1, 2])
        self.tcp_send(sockets[0], 'CSUFFightResult', {}, a)
        result = self.receive(sockets[0], 'SCUnionFightResult').result
        self.assertEqual(result.fightId, match_id)
        if getattr(self, 'transport_mvp', False):
            self.assertEqual(len(result.mvpInfos), 1)
            self.assertEqual((result.mvpInfos[0].type, result.mvpInfos[0].playerId, result.mvpInfos[0].value), (2, a.playerId, 2))
            self.assertEqual((result.selfAchieves[2].type, result.selfAchieves[2].isSuccess, result.selfAchieves[2].rewards[0].num), (2, 1, 4))
        if getattr(self, 'natural_missions', False):
            self.assertEqual((result.selfAchieves[3].type, result.selfAchieves[3].isSuccess, result.selfAchieves[3].rewards[0].num), (3, 1, 6))
        self.assertEqual(result, self.call('CSUFFightResult', {}, a, 14, 'SCUnionFightResult').result)
        stored = self.sql("SELECT CONCAT(winner_guild_id,'|',left_power,'|',right_power,'|',award_at) FROM p13_union_fight_settlement_results WHERE match_id='" + match_id + "';").strip().split('|')
        self.assertNotEqual(stored[1], stored[2])
        winner = left if int(stored[1]) < int(stored[2]) else right
        self.assertEqual(result.winUnionId, winner)
        for login in [a, b]:
            report = self.call('CSUFUnionResultInfo', {}, login, 15, 'SCUFUnionResultInfo')
            self.assertEqual(len(report.reports), 1)
        # Both original clients enter round two; even a refused TCP attack prevents
        # peaceful settlement and the empty result request cannot select round one.
        self.set_clock(first + 540000)
        time.sleep(1.2)
        for login, sock in zip([a, b], sockets):
            self.call('CSTransferScene', {'target': 'fight_union'}, login, 21, 'SCTransferScene')
            self.tcp_send(sock, 'CSCurrentPosition', {}, login)
            self.receive(sock, 'SCUnionFightTransfer')
        self.set_clock(first + 620000)
        if not getattr(self, 'weekly_rewards', False):
            self.tcp_send(sockets[0], 'CSUFPlayerAskFight', {'targetGuid': b.playerId}, a)
            for _ in range(80):
                op, frame = self.recv(sockets[0])
                if op == 10001:
                    break
            else:
                self.fail('refused original TCP fight did not return explicit error')
            self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_settlement_intents WHERE player_id=" + str(a.playerId) + ";").strip(), '1')
            self.set_clock(first + 900000)
            stage = self.call('CSUFCurrentStageInfo', {}, a, 22, 'SCUFCurrentStageInfo')
            self.assertEqual((stage.stage, stage.secondFightInfo.fightResult), (7, 0))
            op, _ = self.stamped('CSUFFightResult', {}, a, 23)
            self.assertEqual(op, 10001, 'second pending round must not replay first round')
        else:
            self.set_clock(first + 900000)
            stage = self.call('CSUFCurrentStageInfo', {}, a, 22, 'SCUFCurrentStageInfo')
            self.assertEqual(stage.stage, 7)
            self.assertIn(stage.secondFightInfo.fightResult, [1, 2])
            self.tcp_send(sockets[0], 'CSUFFightResult', {}, a)
            second_result = self.receive(sockets[0], 'SCUnionFightResult').result
            self.assertNotEqual(second_result.fightId, match_id)
            self.assertEqual(second_result.winUnionId, winner)
            rank = self.call('CSUFRankInfo', {'rankType': 1, 'subType': 2}, a, 24, 'SCUFRankInfo')
            self.assertEqual([(x.rank, x.values[0]) for x in rank.rankIno.list], [(1, 100), (2, 50)])
            self.assertTrue(all(len(x.values) == 2 and x.values[1] == 1 for x in rank.rankIno.list))
            empty = self.call('CSUFRankInfo', {'rankType': 1, 'subType': 1}, a, 25, 'SCUFRankInfo')
            self.assertEqual(len(empty.rankIno.list), 0)
        # Offline winner remains entitled, policy closure prevents new offers only.
        before = {x.playerId: self.profile(x) for x in [a, b]}
        for sock in sockets:
            sock.close()
        self.sql('UPDATE p13_union_fight_settlement_policies SET enabled=0;')
        self.set_clock(int(stored[3]) - 1)
        self.call('CSUFCurrentStageInfo', {}, a, 16, 'SCUFCurrentStageInfo')
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p1_reward_deliveries WHERE source_key='p13-war:" + match_id + "';").strip(), '0')
        self.stop()
        self.set_clock(int(stored[3]))
        self.start()
        a, b = [self.login(uids[i]) for i in selected]
        self.call('CSUFCurrentStageInfo', {}, a, 17, 'SCUFCurrentStageInfo')
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p1_reward_deliveries WHERE source_key='p13-war:" + match_id + "';").strip(), '2')
        for login, gid in [(a, left), (b, right)]:
            rewards = self.call('CSQueryUserReward', {}, login, 18, 'SCQueryUserReward').rewardItemInfoList
            self.assertEqual(len(rewards), 2 if getattr(self, 'weekly_rewards', False) else 1)
            fields = {'rewardIds': [x.rewardId for x in rewards]}
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                list(pool.map(lambda _: self.call('CSAddRewardsToPlayer', fields, login, 19, 'SCAddRewardsToPlayer'), range(8)))
            after = self.profile(login)
            count = lambda profile: sum(x['num'] for x in (profile[1].get('items') or []) if x['id'] == 'item_311')
            self.assertEqual(count(after) - count(before[login.playerId]), (3 if gid == winner else 1) * (2 if getattr(self, 'weekly_rewards', False) else 1) + (4 if getattr(self, 'transport_mvp', False) and login.playerId == a.playerId else 0) + (6 if getattr(self, 'natural_missions', False) and login.playerId == a.playerId else 0))
            self.assertEqual(after[0]['coin'], before[login.playerId][0]['coin'])
            self.assertEqual(after[0]['diamond'], before[login.playerId][0]['diamond'])
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME LIKE 'p13_union_fight_settlement_%';").strip(), 'InnoDB')
        if getattr(self, 'weekly_rewards', False):
            sunday = monday.replace(hour=0) + timedelta(days=6)
            self.set_clock(int(sunday.timestamp() * 1000) - 1)
            self.call('CSUFCurrentStageInfo', {}, a, 26, 'SCUFCurrentStageInfo')
            self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_weekly_results;").strip(), '0')
            self.stop()
            self.sql("UPDATE p13_union_fight_weekly_policies SET enabled=0,rewards='[{\"rank\":1,\"rewards\":\"item_311,999\"}]';")
            self.set_clock(int(sunday.timestamp() * 1000))
            self.start()
            a, b = [self.login(uids[i]) for i in selected]
            self.call('CSUFCurrentStageInfo', {}, a, 27, 'SCUFCurrentStageInfo')
            self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_weekly_results WHERE delivered_at>0;").strip(), '1')
            before_week = {x.playerId: self.profile(x) for x in [a, b]}
            for login, gid in [(a, left), (b, right)]:
                rewards = self.call('CSQueryUserReward', {}, login, 28, 'SCQueryUserReward').rewardItemInfoList
                self.assertEqual(len(rewards), 1)
                fields = {'rewardIds': [rewards[0].rewardId]}
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                    list(pool.map(lambda _: self.call('CSAddRewardsToPlayer', fields, login, 29, 'SCAddRewardsToPlayer'), range(8)))
                after = self.profile(login)
                self.assertEqual(count(after) - count(before_week[login.playerId]), 8 if gid == winner else 4)
                self.assertEqual(after[0]['coin'], before_week[login.playerId][0]['coin'])
                self.assertEqual(after[0]['diamond'], before_week[login.playerId][0]['diamond'])
            self.stop(); self.start()
            a = self.login(uids[selected[0]])
            self.assertEqual(len(self.call('CSQueryUserReward', {}, a, 30, 'SCQueryUserReward').rewardItemInfoList), 0)
            print('actual two timed rounds -> original weekly rank -> Sunday offline frozen P1/cold8: passed')
        print('actual signup/match/mining/TCP result/stages/P1 offline cold claim8: passed; no P6 authority')


if __name__ == '__main__':
    unittest.main()
