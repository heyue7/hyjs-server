"""P13真实角色公会事务/原二进制协议，独立InnoDB库和用户结束清理。"""
import concurrent.futures
import http.client
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import unittest
from server.codec import BY_NAME, CLASSES, encode_http_request, decode_responses
import test_p10_mysql as transport
from hyjs_managed_mysql import case_sql, provision_case
ROOT=Path(__file__).resolve().parents[1]
@unittest.skipUnless(os.environ.get('HYJS_P13_MYSQL_TEST')=='1','显式启用P13专属MySQL')
class P13MySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    def stamped(self,name,fields,login,stamp):
        raw=encode_http_request(BY_NAME[name],CLASSES[name](**fields),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
        connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=5)
        try:
            connection.request('POST','/sao/',body=raw,headers={'Content-Type':'application/octet-stream'})
            response=connection.getresponse();data=response.read();self.assertEqual(response.status,200,data)
            return decode_responses(data)[0]
        finally:connection.close()
    call=transport.P10MySQLTests.call
    profile=transport.P10MySQLTests.profile
    recv=transport.P10MySQLTests.recv
    tcp_send=transport.P10MySQLTests.tcp_send
    connect=transport.P10MySQLTests.connect
    start=transport.P10MySQLTests.start
    stop=transport.P10MySQLTests.stop
    setUp=transport.P10MySQLTests.setUp
    @classmethod
    def setUpClass(cls):
        provision_case(cls, 'p13')
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-p13-isolated-');cls.addClassCleanup(cls.temp.cleanup);cls.binary=str(Path(cls.temp.name)/'hyjs-p13')
    @classmethod
    def root_sql(cls,text):
        return case_sql(cls,text)
    def sql(self,text):return self.root_sql('USE `'+self.database+'`; '+text)
    def test_1_actual_go_mysql_concurrency_permissions_recovery_and_economy(self):
        args=['rtk','proxy','go','test','./service','-run','^TestP13Guild','-count=1','-timeout=120s']
        if os.environ.get('HYJS_P13_GO_RACE')=='1':args.insert(4,'-race')
        r=subprocess.run(args,cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p13_%';").strip(),'InnoDB')
    def test_2_original_binary_http_guild_lifecycle_restart_and_no_double_charge(self):
        if os.environ.get('HYJS_P13_NATIVE_TEST')!='1':self.skipTest('等待共享入口集成后显式启用原二进制HTTP验收')
        self.assertIn('InitializeP13Guild(ctx)',(ROOT/'hyjs-server/main.go').read_text());self.assertIn('DispatchP13Guild(ctx',(ROOT/'hyjs-server/service/gameService.go').read_text())
        r=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(r.returncode,0,r.stdout+r.stderr)
        self.start();uids=['p13-native-'+secrets.token_hex(4) for _ in range(3)];a,b,c=[self.login(uid) for uid in uids]
        for login,name in [(a,'原公会甲'),(b,'原公会乙'),(c,'原公会丙')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        ids=','.join(str(x.playerId) for x in [a,b,c]);self.sql('UPDATE p13_guild_policies SET management_enabled=0,economy_enabled=0; UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000,"$.coin",2000000) WHERE id IN ('+ids+');')
        op,_=self.stamped('CSUnionCreate',{'name':'原二进制公会'},a,100);self.assertEqual(op,10001)
        before=self.profile(a)
        for req,fields in [('CSUnionCreate',{'name':'剑'*7}),('CSUnionCreate',{'name':'😀'*4}),('CSUnionSearch',{'page':0}),('CSUnionApply',{'unionId':' fake '}),('CSUnionAgreeApply',{'playerId':a.playerId}),('CSUnionDonate',{'donateType':'diamond'}),('CSUnionNoticeChange',{'content':'公'*71}),('CSUnionDeclarationChange',{'content':'宣'*41})]:
            op,_=self.stamped(req,fields,a,99);self.assertEqual(op,10001)
        self.assertEqual(self.profile(a),before)
        self.sql('UPDATE p13_guild_policies SET management_enabled=1;')
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:created=list(pool.map(lambda _:self.call('CSUnionCreate',{'name':'原二进制公会'},a,100,'SCUnionCreate'),range(8)))
        self.assertTrue(all(x==created[0] for x in created));self.assertEqual(created[0].cost[0].id,'diamond');self.assertEqual(created[0].cost[0].num,300);self.assertEqual(self.profile(a)[0]['diamond'],700)
        info=self.call('CSGetUnionInfo',{},a,101,'SCGetUnionInfo');guild=info.unionInfo.unionId;self.assertEqual(info.myRole,2)
        self.call('CSUnionApply',{'unionId':guild},b,102,'SCUnionApply');search=self.call('CSUnionSearch',{'name':'原二进制公会','page':1},b,103,'SCUnionSearch');self.assertEqual(search.unions[0].isApplied,1)
        self.call('CSUnionAgreeApply',{'playerId':b.playerId},a,104,'SCUnionAgreeApply')
        op,_=self.stamped('CSPromotionViceChairman',{'playerId':a.playerId},b,105);self.assertEqual(op,10001)
        self.call('CSPromotionViceChairman',{'playerId':b.playerId},a,105,'SCPromotionViceChairman');self.call('CSUnionNoticeChange',{'content':'原职位授权公告'},b,106,'SCUnionNoticeChange')
        self.call('CSUnionApply',{'unionId':guild},c,107,'SCUnionApply');self.call('CSUnionAgreeApply',{'playerId':c.playerId},b,108,'SCUnionAgreeApply')
        self.sql('UPDATE p13_guild_policies SET economy_enabled=1;')
        donation=self.call('CSUnionDonate',{'donateType':'coin'},c,109,'SCUnionDonate');self.assertEqual(donation.contributionAdd,20);self.assertEqual(donation.score,5);self.assertEqual(donation.donateScoreAdd,1)
        self.call('CSUnionDonate',{'donateType':'coin'},c,109,'SCUnionDonate');self.assertEqual(self.profile(c)[0]['coin'],1900000)
        self.call('CSTransferChairman',{'playerId':c.playerId},a,110,'SCTransferChairman');self.call('CSQuitUnion',{},a,111,'SCQuitUnion')
        self.assertEqual(self.call('CSGetUnionInfo',{},a,112,'SCGetUnionInfo').hasUnion,0)
        op,_=self.stamped('CSUnionApply',{'unionId':guild},a,113);self.assertEqual(op,10001)
        self.stop();self.start();a=self.login(uids[0]);c=self.login(uids[2]);info=self.call('CSGetUnionInfo',{},c,114,'SCGetUnionInfo');self.assertEqual(info.myRole,2);self.assertEqual(info.notice,'原职位授权公告');self.assertEqual(info.coinNum,100000)
        self.assertEqual(self.call('CSUnionDonate',{'donateType':'coin'},c,109,'SCUnionDonate'),donation);self.assertEqual(self.profile(c)[0]['coin'],1900000)
        self.call('CSDissolveUnion',{},c,115,'SCDissolveUnion');self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_guild_members WHERE guild_id='"+guild+"';").strip(),'0')

    def test_3_original_tcp_guild_chat_foreign_history_and_immediate_revocation(self):
        if os.environ.get('HYJS_P13_NATIVE_TEST')!='1':self.skipTest('共享入口集成后显式启用原TCP验收')
        r=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(r.returncode,0,r.stdout+r.stderr)
        self.start();logins=[self.login('p13-chat-'+secrets.token_hex(4)) for _ in range(4)];a,b,c,d=logins
        for i,login in enumerate(logins):self.call('CSPlayerCreate',{'playerName':'真实群聊'+str(i),'heroId':'hero_047'},login,1,'SCPlayerCreate')
        ids=','.join(str(x.playerId) for x in logins);self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000) WHERE id IN ('+ids+'); UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=0,chat_enabled=0; UPDATE p10_channel_policies SET world_enabled=0,field_enabled=0;')
        self.call('CSUnionCreate',{'name':'原频道甲'},a,201,'SCUnionCreate');guild=self.call('CSGetUnionInfo',{},a,202,'SCGetUnionInfo').unionInfo.unionId
        for login in [b,c]:self.call('CSUnionApply',{'unionId':guild},login,203,'SCUnionApply');self.call('CSUnionAgreeApply',{'playerId':login.playerId},a,204+login.playerId,'SCUnionAgreeApply')
        self.call('CSUnionCreate',{'name':'原频道外会'},d,201,'SCUnionCreate');sa,sb,sc,sd=[self.connect(x) for x in logins]
        self.tcp_send(sa,'CSChatTcp',{'chatType':2,'content':'关闭公会频道'},a);op,_=self.recv(sa);self.assertEqual(op,10001)
        self.sql('UPDATE p13_guild_policies SET chat_enabled=1;')
        self.tcp_send(sa,'CSChatTcp',{'chatType':2,'content':'原二进制真实同会'},a)
        op,frame=self.recv(sa);self.assertEqual(op,BY_NAME['SCChatTcp']);self.assertEqual(frame.chatType,2);self.assertEqual(frame.play.playerId,a.playerId)
        for sock in [sb,sc]:op,push=self.recv(sock);self.assertEqual(op,BY_NAME['SCChatTcp']);self.assertEqual(push,frame)
        sd.settimeout(.2)
        with self.assertRaises(socket.timeout):sd.recv(1)
        self.assertEqual(len(self.call('CSChatCache',{'chatType':2},d,206,'SCChatCache').chatList),0)
        self.assertEqual(len(self.call('CSChatCache',{'chatType':2},b,206,'SCChatCache').chatList),1)
        # 正式world/field开关保持关闭，独立公会chat门禁已真实发送。
        self.assertEqual(self.sql('SELECT CONCAT(world_enabled,field_enabled) FROM p10_channel_policies;').strip(),'00')
        self.call('CSQuitUnion',{},c,207,'SCQuitUnion');self.assertEqual(len(self.call('CSChatCache',{'chatType':2},c,208,'SCChatCache').chatList),0)
        self.tcp_send(sc,'CSChatTcp',{'chatType':2,'content':'退会伪装成员'},c);op,_=self.recv(sc);self.assertEqual(op,10001)
        self.tcp_send(sb,'CSChatTcp',{'chatType':2,'content':'退会后成员撤权'},b);op,_=self.recv(sb);self.assertEqual(op,BY_NAME['SCChatTcp']);op,_=self.recv(sa);self.assertEqual(op,BY_NAME['SCChatTcp'])
        sc.settimeout(.2)
        with self.assertRaises(socket.timeout):sc.recv(1)
        self.call('CSDissolveUnion',{},a,209,'SCDissolveUnion');self.assertEqual(len(self.call('CSChatCache',{'chatType':2},b,210,'SCChatCache').chatList),0)
        self.tcp_send(sa,'CSChatTcp',{'chatType':2,'content':'解散后伪装公会'},a);op,_=self.recv(sa);self.assertEqual(op,10001)
