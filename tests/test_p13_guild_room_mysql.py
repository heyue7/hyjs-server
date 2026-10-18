"""P13公会领地原二进制HTTP/TCP、真实InnoDB会员隔离与撤权，清理专库用户。"""
import os
from pathlib import Path
import secrets
import socket
import subprocess
import unittest
from server.codec import BY_NAME, CLASSES, encode_http_request, decode_responses
import test_p13_mysql as guild
ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P13_ROOM_MYSQL_TEST')=='1','显式启用P13领地专属MySQL')
class P13GuildRoomMySQLTests(unittest.TestCase):
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
    def call_all(self,name,fields,login,stamp):
        import http.client
        raw=encode_http_request(BY_NAME[name],CLASSES[name](**fields),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
        connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=5)
        try:
            connection.request('POST','/sao/',body=raw,headers={'Content-Type':'application/octet-stream'})
            response=connection.getresponse();data=response.read();self.assertEqual(response.status,200,data);return decode_responses(data)
        finally:connection.close()
    def drain(self,sock):
        frames=[];sock.settimeout(.15)
        try:
            for _ in range(30):
                try:frames.append(self.recv(sock))
                except socket.timeout:break
            else:self.fail('领地帧队列超出限定')
        finally:sock.settimeout(3)
        return frames
    def receive(self,sock,name):
        for _ in range(20):
            op,frame=self.recv(sock)
            if op==BY_NAME[name]:return frame
            self.assertIn(op,[BY_NAME[n] for n in ['SCPlayer','SCMovePlayerInfo','SCPlayerAppear','SCPlayerDisappear','SCUnionBuilding','SCFieldPlayerState','SCRefreshScene']],str(frame))
        self.fail('领地原帧响应超出限定')
    def test_1_go_room_current_membership_private_scope_and_restart(self):
        args=['rtk','proxy','go','test','./service','-run','^TestP13Guild(Room|Welfare|Building)','-count=1','-timeout=120s']
        if os.environ.get('HYJS_P13_ROOM_GO_RACE')=='1':args.insert(4,'-race')
        r=subprocess.run(args,cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
    def test_2_native_original_room_transfer_current_login_building_move_exit_reconnect(self):
        if os.environ.get('HYJS_P13_ROOM_NATIVE_TEST')!='1':self.skipTest('共享入口后显式启用原HTTP/TCP领地')
        r=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(r.returncode,0,r.stdout+r.stderr)
        self.start();uids=['p13-room-'+secrets.token_hex(4) for _ in range(4)];a,b,c,d=[self.login(uid) for uid in uids];logins=[a,b,c,d]
        for i,login in enumerate(logins):self.call('CSPlayerCreate',{'playerName':'领地原帧'+str(i),'heroId':'hero_047'},login,1,'SCPlayerCreate')
        ids=','.join(str(x.playerId) for x in logins)
        self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000) WHERE id IN ('+ids+'); UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=0; UPDATE p10_scene_policies SET enabled=0; UPDATE p10_channel_policies SET world_enabled=0,field_enabled=0;')
        self.call('CSUnionCreate',{'name':'领地原甲'},a,10,'SCUnionCreate');ga=self.call('CSGetUnionInfo',{},a,11,'SCGetUnionInfo').unionInfo.unionId
        self.call('CSUnionApply',{'unionId':ga},b,12,'SCUnionApply');self.call('CSUnionAgreeApply',{'playerId':b.playerId},a,13,'SCUnionAgreeApply')
        self.call('CSUnionCreate',{'name':'领地原乙'},c,10,'SCUnionCreate');gc=self.call('CSGetUnionInfo',{},c,11,'SCGetUnionInfo').unionInfo.unionId
        op,_=self.stamped('CSTransferScene',{'target':'town_union'},d,20);self.assertEqual(op,10001)
        before=[self.profile(x)[0] for x in logins]
        sa,sb,sc,sd=[self.connect(x) for x in logins]
        for login in [a,b,c]:self.call('CSTransferScene',{'target':'town_union','point':{'x':0,'y':0,'z':0}},login,21,'SCTransferScene')
        pushes=[self.drain(s) for s in [sa,sb,sc,sd]]
        self.assertTrue(any(op==BY_NAME['SCPlayerAppear'] and any(p.info.playerId==b.playerId for p in frame.player) for op,frame in pushes[0]))
        self.assertTrue(any(op==BY_NAME['SCPlayerAppear'] and any(p.info.playerId==a.playerId for p in frame.player) for op,frame in pushes[1]))
        self.assertEqual(pushes[2],[]);self.assertEqual(pushes[3],[])
        self.assertEqual(self.sql('SELECT CONCAT(enabled) FROM p10_scene_policies;').strip(),'0')
        for login,gid in [(a,ga),(b,ga),(c,gc)]:self.assertEqual(self.sql('SELECT scene FROM p10_scene_presence WHERE player_id='+str(login.playerId)+';').strip(),'p13-union:'+gid)
        for name,expected in [('CSCurrentPosition','SCCurrentPosition'),('CSLoginScene','SCLoginScene')]:
            frames=self.call_all(name,{},a,22);self.assertEqual(frames[0][0],BY_NAME[expected]);self.assertEqual(frames[0][1].self.position.scene,'town_union');self.assertEqual(frames[0][1].limit,35);point=frames[0][1].self.position.current;self.assertAlmostEqual(point.x,22.85771,places=5);self.assertAlmostEqual(point.y,1.192649,places=5);self.assertAlmostEqual(point.z,22.85441,places=5)
            building=next(frame for op,frame in frames if op==BY_NAME['SCUnionBuilding']);self.assertEqual((building.heroStatueOpen,building.guardTreeOpen),(0,0));state=next(frame for op,frame in frames if op==BY_NAME['SCFieldPlayerState']);self.assertEqual([(p.playerId,p.kills) for p in state.player],[(a.playerId,0)])
        self.tcp_send(sa,'CSRefreshUnionBuilding',{},a);building=self.receive(sa,'SCUnionBuilding');self.assertEqual((building.heroStatueOpen,building.guardTreeOpen),(0,0))
        peers=self.call('CSMovePlayerInfo',{'playerIds':[a.playerId,b.playerId,c.playerId,d.playerId]},a,23,'SCMovePlayerInfo');self.assertEqual({p.info.playerId for p in peers.playerInfos},{a.playerId,b.playerId});self.assertTrue(all(p.position.scene=='town_union' for p in peers.playerInfos))
        peers=self.call('CSRefreshMovePlayerInfo',{},c,23,'SCMovePlayerInfo');self.assertEqual([p.info.playerId for p in peers.playerInfos],[c.playerId])
        for s in [sa,sb,sc,sd]:self.drain(s)
        self.tcp_send(sa,'CSMove',{'pointNew':{'x':7,'y':0,'z':9},'currentTime':1700000000000},a);moved=self.receive(sb,'SCMovePush');self.assertEqual((moved.position[0].playerId,moved.position[0].current.x),(a.playerId,7));self.assertEqual(self.drain(sc),[]);self.assertEqual(self.drain(sd),[])
        self.assertEqual([self.profile(x)[0] for x in logins],before)
        # 真实登录轮换会话后，新TCP恢复本人公会实例，旧TCP请求无法改变位置。
        fresh_b=self.login(uids[1]);new_sb=self.connect(fresh_b);self.receive(sa,'SCPlayerDisappear');self.receive(sa,'SCPlayerAppear');self.receive(new_sb,'SCPlayerAppear')
        self.tcp_send(sb,'CSRefreshUnionBuilding',{},b);op,_=self.recv(sb);self.assertEqual(op,10001)
        self.tcp_send(new_sb,'CSLoginScene',{},fresh_b);self.assertEqual(self.receive(new_sb,'SCLoginScene').self.position.scene,'town_union');self.drain(new_sb);self.drain(sa)
        self.call('CSQuitUnion',{},fresh_b,24,'SCQuitUnion');self.receive(sa,'SCPlayerDisappear');self.receive(new_sb,'SCRefreshScene');self.assertEqual(self.profile(fresh_b)[1]['position']['scene'],'town_main')
        self.tcp_send(new_sb,'CSLoginScene',{},fresh_b);self.assertEqual(self.receive(new_sb,'SCLoginScene').self.position.scene,'town_main');self.drain(new_sb)
        self.tcp_send(new_sb,'CSRefreshUnionBuilding',{},fresh_b);op,_=self.recv(new_sb);self.assertEqual(op,10001)
        self.tcp_send(sa,'CSMove',{'pointNew':{'x':8,'y':0,'z':9},'currentTime':1700000000001},a);self.assertEqual(self.drain(new_sb),[]);self.assertEqual(self.drain(sc),[])
        self.call('CSDissolveUnion',{},a,25,'SCDissolveUnion');self.receive(sa,'SCRefreshScene');self.assertEqual(self.profile(a)[1]['position']['scene'],'town_main');self.assertEqual(self.profile(c)[1]['position']['scene'],'town_union')
        self.stop();self.start();c=self.login(uids[2]);sc=self.connect(c);self.tcp_send(sc,'CSLoginScene',{},c);self.assertEqual(self.receive(sc,'SCLoginScene').self.position.scene,'town_union');self.assertEqual(self.sql('SELECT scene FROM p10_scene_presence WHERE player_id='+str(c.playerId)+';').strip(),'p13-union:'+gc)

    def test_3_native_original_readonly_uncreated_buildings_and_unprayed_church(self):
        if os.environ.get('HYJS_P13_ROOM_NATIVE_TEST')!='1':self.skipTest('显式启用原公会福利HTTP/TCP')
        r=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(r.returncode,0,r.stdout+r.stderr)
        self.start();uid='p13-welfare-'+secrets.token_hex(4);a=self.login(uid);b=self.login('p13-welfare-out-'+secrets.token_hex(4))
        for login,name in [(a,'福利原会长'),(b,'福利原外会')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000,"$.coin",1000000) WHERE id='+str(a.playerId)+'; UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=1;')
        self.call('CSUnionCreate',{'name':'福利原目录'},a,30,'SCUnionCreate');self.call('CSUnionDonate',{'donateType':'coin'},a,31,'SCUnionDonate');before=self.profile(a)
        for name,reply_name,field in [('CSUnionHeroStatueInfo','SCUnionHeroStatueInfo','unionPropAddAtt'),('CSUnionGuardTreeInfo','SCUnionGuardTreeInfo','unionPropAddDef')]:
            reply=self.call(name,{},a,32,reply_name);self.assertEqual((reply.coinNum,reply.badgeNum),(100000,0));self.assertEqual(len(getattr(reply,field)),0);op,_=self.stamped(name,{},b,32);self.assertEqual(op,10001)
        self.sql('UPDATE p10_scene_policies SET enabled=0;');sa=self.connect(a)
        self.tcp_send(sa,'CSUnionLightChurchInfo',{},a);reply=self.receive(sa,'SCUnionLightChurchInfo');self.assertEqual((reply.prayTimes,reply.contribution,reply.invalidTime),(0,20,0));self.assertTrue(reply.HasField('addProp'));self.assertEqual(reply.addProp.SerializeToString(),b'')
        op,_=self.stamped('CSUnionLightChurchInfo',{},b,33);self.assertEqual(op,10001)
        for name,fields in [('CSUnionHeroStatueUpgrade',{'type':'att'}),('CSUnionGuardTreeUpgrade',{'type':'hp'}),('CSUnionPray',{})]:
            op,error=self.stamped(name,fields,a,34);self.assertEqual(op,10001);self.assertIn('原规则核实',str(error))
        self.assertEqual(self.profile(a),before);info=self.call('CSGetUnionInfo',{},a,35,'SCGetUnionInfo');self.assertEqual((info.coinNum,info.badgeNum,info.heroStatueOpen,info.guardTreeOpen),(100000,0,0,0))
        self.sql('UPDATE p13_guild_policies SET management_enabled=0,economy_enabled=0;');self.stop();self.start();a=self.login(uid)
        reply=self.call('CSUnionLightChurchInfo',{},a,36,'SCUnionLightChurchInfo');self.assertEqual((reply.prayTimes,reply.contribution,reply.invalidTime),(0,20,0));self.assertEqual(self.profile(a),before)

    def test_4_original_building_permission_cost_shared_push_retry_restart(self):
        if os.environ.get('HYJS_P13_ROOM_NATIVE_TEST')!='1':self.skipTest('显式启用原建设HTTP/TCP')
        import concurrent.futures
        r=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(r.returncode,0,r.stdout+r.stderr)
        self.start();uids=['p13-build-'+secrets.token_hex(4) for _ in range(4)];a,b,c,d=[self.login(uid) for uid in uids]
        for i,login in enumerate([a,b,c,d]):self.call('CSPlayerCreate',{'playerName':'建设原帧'+str(i),'heroId':'hero_047'},login,1,'SCPlayerCreate')
        ids=','.join(str(x.playerId) for x in [a,b,c,d]);self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000,"$.coin",2000000) WHERE id IN ('+ids+'); UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=1; UPDATE p10_scene_policies SET enabled=0;')
        self.call('CSUnionCreate',{'name':'原建设甲'},a,40,'SCUnionCreate');ga=self.call('CSGetUnionInfo',{},a,41,'SCGetUnionInfo').unionInfo.unionId
        for login in [b,c]:self.call('CSUnionApply',{'unionId':ga},login,42,'SCUnionApply');self.call('CSUnionAgreeApply',{'playerId':login.playerId},a,43+login.playerId,'SCUnionAgreeApply')
        self.call('CSPromotionViceChairman',{'playerId':b.playerId},a,49,'SCPromotionViceChairman')
        self.call('CSUnionCreate',{'name':'原建设外会'},d,40,'SCUnionCreate')
        # 实际捐献建立共享资产来源；大额建设边界使用明确隔离测试余额。
        self.call('CSUnionDonate',{'donateType':'coin'},c,50,'SCUnionDonate');before=[self.profile(x) for x in [a,b,c,d]]
        op,_=self.stamped('CSUnionOpenBuilder',{'type':'HeroStatue'},a,51);self.assertEqual(op,10001)
        self.sql("UPDATE p13_guilds SET coin=20000000,badge=40 WHERE id='"+ga+"';")
        for login in [c,d]:op,_=self.stamped('CSUnionOpenBuilder',{'type':'HeroStatue'},login,51);self.assertEqual(op,10001)
        sa,sb,sc,sd=[self.connect(x) for x in [a,b,c,d]]
        for login in [a,b,c,d]:self.call('CSTransferScene',{'target':'town_union'},login,52,'SCTransferScene')
        for sock in [sa,sb,sc,sd]:self.drain(sock)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:opened=list(pool.map(lambda _:self.call('CSUnionOpenBuilder',{'type':'HeroStatue'},a,53,'SCUnionOpenBuilder'),range(8)))
        self.assertTrue(all(x==opened[0] for x in opened))
        for sock in [sa,sb,sc]:reply=self.receive(sock,'SCUnionBuilding');self.assertEqual((reply.heroStatueOpen,reply.guardTreeOpen),(1,0))
        self.assertEqual(self.drain(sd),[])
        info=self.call('CSGetUnionInfo',{},c,54,'SCGetUnionInfo');self.assertEqual((info.coinNum,info.badgeNum,info.heroStatueOpen,info.guardTreeOpen),(10000000,20,1,0))
        op,_=self.stamped('CSUnionOpenBuilder',{'type':'GuardTree'},a,53);self.assertEqual(op,10001)
        op,_=self.stamped('CSUnionOpenBuilder',{'type':'HeroStatue'},b,55);self.assertEqual(op,10001)
        self.call('CSUnionOpenBuilder',{'type':'GuardTree'},b,56,'SCUnionOpenBuilder')
        for sock in [sa,sb,sc]:reply=self.receive(sock,'SCUnionBuilding');self.assertEqual((reply.heroStatueOpen,reply.guardTreeOpen),(1,1))
        self.assertEqual(self.drain(sd),[])
        # 建设不消费个人钱包，也不会从目录制造未核实科技等级。
        self.assertEqual([self.profile(x)[0] for x in [a,b,c,d]],[x[0] for x in before])
        self.assertEqual(len(self.call('CSUnionHeroStatueInfo',{},a,57,'SCUnionHeroStatueInfo').unionPropAddAtt),0)
        self.assertEqual(self.sql("SELECT CONCAT(coin,',',badge,',',hero_statue_open,',',guard_tree_open) FROM p13_guilds WHERE id='"+ga+"';").strip(),'0,0,1,1')
        self.stop();self.start();a=self.login(uids[0]);sa=self.connect(a)
        frames=self.call_all('CSLoginScene',{},a,58);reply=next(frame for op,frame in frames if op==BY_NAME['SCUnionBuilding']);self.assertEqual((reply.heroStatueOpen,reply.guardTreeOpen),(1,1))
        self.tcp_send(sa,'CSRefreshUnionBuilding',{},a);reply=self.receive(sa,'SCUnionBuilding');self.assertEqual((reply.heroStatueOpen,reply.guardTreeOpen),(1,1))
        self.call('CSUnionOpenBuilder',{'type':'HeroStatue'},a,53,'SCUnionOpenBuilder');self.assertEqual(self.sql("SELECT CONCAT(coin,',',badge) FROM p13_guilds WHERE id='"+ga+"';").strip(),'0,0')

if __name__=='__main__':unittest.main()
