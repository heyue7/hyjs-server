"""P13原野外组队真实TCP邀请/多玩家管理和InnoDB并发，结束清理专库。"""
import os
from pathlib import Path
import secrets
import subprocess
import unittest
from server.codec import BY_NAME
import test_p13_mysql as guild
ROOT=Path(__file__).resolve().parents[1]
@unittest.skipUnless(os.environ.get('HYJS_P13_TEAM_MYSQL_TEST')=='1','显式启用P13Team专属MySQL')
class P13TeamMySQLTests(unittest.TestCase):
    send=guild.P13MySQLTests.send
    login=guild.P13MySQLTests.login
    stamped=guild.P13MySQLTests.stamped
    call=guild.P13MySQLTests.call
    profile=guild.P13MySQLTests.profile
    recv=guild.P13MySQLTests.recv
    tcp_send=guild.P13MySQLTests.tcp_send
    connect=guild.P13MySQLTests.connect
    start=guild.P13MySQLTests.start
    stop=guild.P13MySQLTests.stop
    setUp=guild.P13MySQLTests.setUp
    @classmethod
    def setUpClass(cls):guild.P13MySQLTests.setUpClass.__func__(cls)
    root_sql=classmethod(guild.P13MySQLTests.root_sql.__func__)
    sql=guild.P13MySQLTests.sql
    def receive_team(self,sock,expected):
        for _ in range(10):
            op,reply=self.recv(sock)
            if op==BY_NAME['SCPlayer']:continue
            self.assertEqual(op,BY_NAME[expected],str(reply));return reply
        self.fail('原队伍响应超出限定帧数')
    def team_send(self,sock,login,request,fields,expected):
        self.tcp_send(sock,request,fields,login);return self.receive_team(sock,expected)
    def test_1_go_original_team_concurrency_atomicity_invite_generations_and_restart(self):
        args=['rtk','proxy','go','test','./service','-run','^TestP13Team','-count=1','-timeout=120s']
        if os.environ.get('HYJS_P13_TEAM_GO_RACE')=='1':args.insert(4,'-race')
        r=subprocess.run(args,cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p13_team%';").strip(),'InnoDB')
    def test_2_original_tcp_invite_accept_leader_change_kick_history_reconnect_and_disband(self):
        if os.environ.get('HYJS_P13_TEAM_NATIVE_TEST')!='1':self.skipTest('共享入口后显式启用原TCP完整链')
        self.assertIn('InitializeP13Team(ctx)',(ROOT/'hyjs-server/main.go').read_text());self.assertIn('DispatchP13Team(ctx',(ROOT/'hyjs-server/service/gameService.go').read_text())
        r=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(r.returncode,0,r.stdout+r.stderr)
        self.start();uids=['p13-team-native-'+secrets.token_hex(4) for _ in range(4)];a,b,c,d=[self.login(uid) for uid in uids]
        for i,login in enumerate([a,b,c,d]):self.call('CSPlayerCreate',{'playerName':'原组队'+str(i),'heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql('UPDATE p13_team_policies SET enabled=0;')
        sa,sb,sc,sd=[self.connect(x) for x in [a,b,c,d]]
        self.tcp_send(sa,'CSCreateTeam',{},a);op,_=self.recv(sa);self.assertEqual(op,10001)
        before=[self.profile(x) for x in [a,b,c,d]]
        # 原两个数组必须一一配对，任何拒绝不留下队伍或个人teamId。
        for req,fields in [('CSTeamCreateAndInvite',{'invitePlayerIdList':[b.playerId],'inviteServerIdList':[]}),('CSTeamCreateAndInvite',{'invitePlayerIdList':[b.playerId],'inviteServerIdList':[2]}),('CSTeamCreateAndInvite',{'invitePlayerIdList':[a.playerId],'inviteServerIdList':[1]}),('CSChangeTeamLeader',{'teamId':'missing','toPlayerId':a.playerId})]:
            op,_=self.stamped(req,fields,a,99);self.assertEqual(op,10001)
        self.assertEqual([self.profile(x) for x in [a,b,c,d]],before)
        self.sql('UPDATE p13_team_policies SET enabled=1;')
        created=self.team_send(sa,a,'CSTeamCreateAndInvite',{'invitePlayerIdList':[b.playerId,c.playerId],'inviteServerIdList':[1,1]},'SCTeamCreateAndInvite');team=created.team.teamId
        self.assertEqual(created.invitePlayerId,a.playerId);self.assertEqual(created.team.maxSize,5);self.assertEqual(created.team.memberList[0].playerId,a.playerId)
        for sock in [sb,sc]:invitation=self.receive_team(sock,'SCTeamCreateAndInvite');self.assertEqual(invitation.invitePlayerId,a.playerId);self.assertEqual(invitation.team.teamId,team)
        self.team_send(sb,b,'CSJoinTeam',{'teamId':team},'SCJoinTeam');joined=self.receive_team(sa,'SCJoinTeam');self.assertEqual(joined.toPlayerId,b.playerId)
        self.team_send(sc,c,'CSJoinTeam',{'teamId':team},'SCJoinTeam')
        for sock in [sa,sb]:joined=self.receive_team(sock,'SCJoinTeam');self.assertEqual(joined.toPlayerId,c.playerId)
        query=self.call('CSQueryTeam',{'teamId':team},a,101,'SCQueryTeam');self.assertEqual([x.playerId for x in query.team.memberList],[a.playerId,b.playerId,c.playerId])
        op,_=self.stamped('CSJoinTeam',{'teamId':team},d,102);self.assertEqual(op,10001)
        op,_=self.stamped('CSQueryTeam',{'teamId':team},d,103);self.assertEqual(op,10001)
        op,_=self.stamped('CSTeamKick',{'treamId':team,'toPlayerId':c.playerId},b,104);self.assertEqual(op,10001)
        self.call('CSTeamInvite',{'teamId':team,'invitePlayerIdList':[d.playerId],'inviteServerIdList':[1]},a,105,'SCTeamInvite');self.receive_team(sd,'SCTeamCreateAndInvite')
        self.call('CSChangeTeamLeader',{'teamId':team,'toPlayerId':b.playerId},a,106,'SCChangeTeamLeader')
        for sock in [sb,sc]:changed=self.receive_team(sock,'SCChangeTeamLeader');self.assertEqual(changed.team.memberList[0].playerId,b.playerId)
        op,_=self.stamped('CSJoinTeam',{'teamId':team},d,107);self.assertEqual(op,10001)
        kicked=self.team_send(sb,b,'CSTeamKick',{'treamId':team,'toPlayerId':c.playerId},'SCTeamKick');self.assertEqual(kicked.member.playerId,c.playerId)
        for sock in [sa,sc]:kicked=self.receive_team(sock,'SCTeamKick');self.assertEqual(kicked.member.playerId,c.playerId)
        self.assertEqual(self.profile(c)[0].get('teamId',''),'')
        empty=self.call('CSQueryTeamTick',{},c,108,'SCQueryTeamTick');self.assertEqual(empty.team.teamId,'');self.assertEqual(empty.seconds,3)
        # 断线与服务重启保存实际名册，不采用未知自动队长代理或超时解散。
        self.stop();self.start();a=self.login(uids[0]);b=self.login(uids[1]);sb=self.connect(b)
        recovered=self.call('CSQueryTeamTick',{},b,109,'SCQueryTeamTick');self.assertEqual(recovered.team.memberList[0].playerId,b.playerId);self.assertEqual({x.playerId for x in recovered.team.memberList},{a.playerId,b.playerId})
        self.team_send(sb,b,'CSDisbandTeam',{'teamId':team},'SCDisbandTeam')
        self.assertEqual(self.profile(a)[0].get('teamId',''),'');self.assertEqual(self.profile(b)[0].get('teamId',''),'')
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_team_members WHERE team_id='"+team+"';").strip(),'0')
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_team_invites WHERE team_id='"+team+"';").strip(),'0')
