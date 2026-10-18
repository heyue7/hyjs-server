"""Actual original local teams on configured realm2, isolated from realm1."""
import hashlib
import json
import os
from pathlib import Path
import secrets
import time
import unittest

from server.codec import BY_NAME
import test_p10_profile_realm_mysql as profile

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P13_TEAM_REALM_TEST') == '1', 'explicit original two-realm local team test')
class P13TeamRealmMySQLTests(profile.P10ProfileRealmMySQLTests):
    __unittest_skip__ = False
    test_original_local_profile_two_realms_http_tcp_cold = None

    def receive_team(self, sock, expected):
        for _ in range(12):
            op, reply = profile.native.cross.guild.P13MySQLTests.recv(self, sock)
            if op == BY_NAME['SCPlayer']:
                continue
            self.assertEqual(op, BY_NAME[expected], str(reply))
            return reply
        self.fail('original team response exceeded bounded frame count')

    def team_send(self, zone, sock, login, request, fields, expected):
        self.tcp_send(zone, sock, request, fields, login)
        return self.receive_team(sock, expected)

    def assert_realm(self, team, zone):
        self.assertTrue(team.teamId)
        self.assertTrue(team.memberList)
        self.assertTrue(all(member.serverId == zone for member in team.memberList), str(team))

    def test_original_realm2_invite_accept_leader_kick_exit_cold_disband(self):
        began = time.monotonic()
        self.shutdownTimes = []
        for zone in [1, 2]:
            self.start(zone)
        uids = {zone: ['team-realm-' + str(zone) + '-' + secrets.token_hex(4) for _ in range(4)] for zone in [1, 2]}
        players = {zone: [self.login(zone, uid) for uid in uids[zone]] for zone in [1, 2]}
        self.assertEqual([p.playerId for p in players[1]], [p.playerId for p in players[2]])
        for zone in [1, 2]:
            for i, login in enumerate(players[zone]):
                self.call(zone, 'CSPlayerCreate', {'playerName': '双队服' + str(zone) + '员' + str(i), 'heroId': 'hero_047'}, login, 2, 'SCPlayerCreate')
            self.sql(zone, 'UPDATE p13_team_policies SET enabled=1;')
        sockets = {zone: [self.tcp(zone, login) for login in players[zone]] for zone in [1, 2]}
        home = self.team_send(1, sockets[1][0], players[1][0], 'CSCreateTeam', {}, 'SCCreateTeam').team
        self.assert_realm(home, 1)
        realm1_before = self.profiles(1, players[1])
        a, b, c, d = players[2]
        sa, sb, sc, sd = sockets[2]
        for wrong in [0, 1, 3]:
            self.assertEqual(self.send(2, 'CSCheckTeam', {'toPlayerId': b.playerId, 'toServerid': wrong}, a, 100)[0], 10001)
            self.assertEqual(self.send(2, 'CSTeamCreateAndInvite', {'invitePlayerIdList': [b.playerId], 'inviteServerIdList': [wrong]}, a, 101)[0], 10001)
        self.assertEqual(self.sql(2, 'SELECT COUNT(*) FROM p13_teams;').strip(), '0')
        available = self.team_send(2, sa, a, 'CSCheckTeam', {'toPlayerId': b.playerId, 'toServerid': 2}, 'SCCheckTeam')
        self.assertEqual(available.checkResult, 1)
        created = self.team_send(2, sa, a, 'CSTeamCreateAndInvite', {'invitePlayerIdList': [b.playerId, c.playerId], 'inviteServerIdList': [2, 2]}, 'SCTeamCreateAndInvite')
        self.assert_realm(created.team, 2)
        team = created.team.teamId
        for sock in [sb, sc]:
            invitation = self.receive_team(sock, 'SCTeamCreateAndInvite')
            self.assertEqual(invitation.team.teamId, team)
            self.assert_realm(invitation.team, 2)
        self.team_send(2, sb, b, 'CSJoinTeam', {'teamId': team}, 'SCJoinTeam')
        self.receive_team(sa, 'SCJoinTeam')
        self.team_send(2, sc, c, 'CSJoinTeam', {'teamId': team}, 'SCJoinTeam')
        for sock in [sa, sb]:
            self.receive_team(sock, 'SCJoinTeam')
        for wrong in [0, 1, 3]:
            self.assertEqual(self.send(2, 'CSTeamInvite', {'teamId': team, 'invitePlayerIdList': [d.playerId], 'inviteServerIdList': [wrong]}, a, 102)[0], 10001)
        self.team_send(2, sa, a, 'CSTeamInvite', {'teamId': team, 'invitePlayerIdList': [d.playerId], 'inviteServerIdList': [2]}, 'SCTeamInvite')
        self.assert_realm(self.receive_team(sd, 'SCTeamCreateAndInvite').team, 2)
        self.team_send(2, sd, d, 'CSJoinTeam', {'teamId': team}, 'SCJoinTeam')
        for sock in [sa, sb, sc]:
            self.receive_team(sock, 'SCJoinTeam')
        query = self.call(2, 'CSQueryTeam', {'teamId': team}, a, 103, 'SCQueryTeam')
        self.assert_realm(query.team, 2)
        self.assertEqual(len(query.team.memberList), 4)
        self.team_send(2, sa, a, 'CSChangeTeamLeader', {'teamId': team, 'toPlayerId': b.playerId}, 'SCChangeTeamLeader')
        for sock in [sb, sc, sd]:
            self.assert_realm(self.receive_team(sock, 'SCChangeTeamLeader').team, 2)
        self.team_send(2, sb, b, 'CSTeamKick', {'treamId': team, 'toPlayerId': c.playerId}, 'SCTeamKick')
        for sock in [sa, sc, sd]:
            self.receive_team(sock, 'SCTeamKick')
        self.team_send(2, sa, a, 'CSExitTeam', {'teamId': team}, 'SCExitTeam')
        for sock in [sb, sd]:
            self.receive_team(sock, 'SCExitTeam')
        fixed = self.ports[2]
        self.stop(2)
        self.start(2, fixed)
        players[2] = [self.login(2, uid) for uid in uids[2]]
        a, b, c, d = players[2]
        sb, sd = self.tcp(2, b), self.tcp(2, d)
        cold = self.call(2, 'CSQueryTeamTick', {}, b, 104, 'SCQueryTeamTick')
        self.assert_realm(cold.team, 2)
        self.assertEqual([member.playerId for member in cold.team.memberList], [b.playerId, d.playerId])
        self.team_send(2, sb, b, 'CSDisbandTeam', {'teamId': team}, 'SCDisbandTeam')
        self.receive_team(sd, 'SCDisbandTeam')
        for snapshot in self.profiles(2, players[2]):
            self.assertEqual(snapshot[0].get('teamId', ''), '')
        self.assertEqual(self.sql(2, 'SELECT COUNT(*) FROM p13_team_members WHERE team_id<>"";').strip(), '0')
        self.assertEqual(self.profiles(1, players[1]), realm1_before)
        self.assertEqual(self.call(1, 'CSQueryTeam', {}, players[1][0], 105, 'SCQueryTeam').team, home)
        self.team_send(1, sockets[1][0], players[1][0], 'CSDisbandTeam', {'teamId': home.teamId}, 'SCDisbandTeam')
        for zone in [1, 2]:
            self.stop(zone)
        proof = {'status': 'PASS_REALM2_ORIGINAL_LOCAL_TEAM_FULL_TCP_CHAIN', 'seconds': round(time.monotonic() - began, 3), 'sourceProofSHA256': hashlib.sha256((self.prebuilt / 'manifest.json').read_bytes()).hexdigest(), 'actualPlayers': 8, 'realm2AcceptedServer': 2, 'invalidServers': [0, 1, 3], 'realm1SameLocalIDsAndFullProfilesUnchanged': True, 'originalTCP': 'actualauthenticated8sockets; create/invite/listpush/join/query/transfer/kick/exit/cold/tick/disband', 'finitePrerequisites': 'none; originalcreatedlevel1 starter accounts; only isolated preexistingdefaultoffTeam policy enabled', 'shutdownSeconds': self.shutdownTimes, 'phone': False, 'productionModified': False}
        (ROOT / 'analysis/p13/team-local-realm-native-checkpoint.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
        print('actual original realm2 team fullchain PASS', proof['seconds'])


if __name__ == '__main__':
    unittest.main()
