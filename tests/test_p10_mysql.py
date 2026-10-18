"""P10两真实账号原HTTP/TCP好友/私聊，专属InnoDB库结束清理。"""
import concurrent.futures
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import shutil
import socket
import struct
import subprocess
import tempfile
import types
import unittest
from server.codec import BY_NAME, decode_responses, message
from test_server import LiveServerTests
from test_go_formation import GoFormationTests
ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P10_MYSQL_TEST')=='1','显式启用P10专属MySQL')
class P10MySQLTests(unittest.TestCase):
    send=LiveServerTests.send
    login=LiveServerTests.login
    stamped=GoFormationTests.stamped
    @classmethod
    def setUpClass(cls):
        cls.ran_go_fixture=False
        cls.database='hyjs_test_p10_'+secrets.token_hex(5);cls.test_user='p10_'+secrets.token_hex(5);cls.test_password=secrets.token_hex(24)
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-p10-isolated-');cls.addClassCleanup(cls.temp.cleanup);cls.binary=str(Path(cls.temp.name)/'hyjs-p10')
        cls.root_sql("CREATE DATABASE `"+cls.database+"` CHARACTER SET utf8mb4; CREATE USER '"+cls.test_user+"'@'%' IDENTIFIED BY '"+cls.test_password+"'; GRANT ALL ON `"+cls.database+"`.* TO '"+cls.test_user+"'@'%';")
        cls.addClassCleanup(lambda:cls.root_sql("DROP DATABASE IF EXISTS `"+cls.database+"`; DROP USER IF EXISTS '"+cls.test_user+"'@'%';"))
        cls.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER=cls.test_user,HYJS_MYSQL_PASSWORD=cls.test_password,HYJS_MYSQL_DATABASE=cls.database,
                     HYJS_P10_MYSQL_HOST='127.0.0.1:33068',HYJS_P10_MYSQL_USER=cls.test_user,HYJS_P10_MYSQL_PASSWORD=cls.test_password,HYJS_P10_MYSQL_DATABASE=cls.database)
        build_root=ROOT/'hyjs-server';cls.used_pending_patch='InitializeP10Social(ctx)' not in (build_root/'main.go').read_text()
        if cls.used_pending_patch:
            build_root=Path(cls.temp.name)/'source';shutil.copytree(ROOT/'hyjs-server',build_root,ignore=shutil.ignore_patterns('logs'))
            result=subprocess.run(['rtk','proxy','git','apply',str(ROOT/'analysis/p10/social-integration.patch')],cwd=build_root,capture_output=True,text=True,timeout=30)
            if result.returncode:raise AssertionError(result.stdout+result.stderr)
        cls.used_energy_patch='InitializeP10FriendEnergy(ctx)' not in (build_root/'main.go').read_text()
        if cls.used_energy_patch:
            if build_root == ROOT/'hyjs-server':
                build_root=Path(cls.temp.name)/'source';shutil.copytree(ROOT/'hyjs-server',build_root,ignore=shutil.ignore_patterns('logs'))
            result=subprocess.run(['rtk','proxy','git','apply',str(ROOT/'analysis/p10/energy-integration.patch')],cwd=build_root,capture_output=True,text=True,timeout=30)
            if result.returncode:raise AssertionError(result.stdout+result.stderr)
        cls.used_profile_patch='DispatchP10Profile(ctx' not in (build_root/'service/gameService.go').read_text()
        if cls.used_profile_patch:
            if build_root == ROOT/'hyjs-server':
                build_root=Path(cls.temp.name)/'source';shutil.copytree(ROOT/'hyjs-server',build_root,ignore=shutil.ignore_patterns('logs'))
            result=subprocess.run(['rtk','proxy','git','apply',str(ROOT/'analysis/p10/profile-integration.patch')],cwd=build_root,capture_output=True,text=True,timeout=30)
            if result.returncode:raise AssertionError(result.stdout+result.stderr)
        cls.used_scene_patch='InitializeP10Scene(ctx)' not in (build_root/'main.go').read_text()
        cls.include_scene=os.environ.get('HYJS_P10_SKIP_PENDING_SCENE')!='1'
        if cls.used_scene_patch and cls.include_scene:
            if build_root == ROOT/'hyjs-server':
                build_root=Path(cls.temp.name)/'source';shutil.copytree(ROOT/'hyjs-server',build_root,ignore=shutil.ignore_patterns('logs'))
            result=subprocess.run(['rtk','proxy','git','apply',str(ROOT/'analysis/p10/scene-integration.patch')],cwd=build_root,capture_output=True,text=True,timeout=30)
            if result.returncode:raise AssertionError(result.stdout+result.stderr)
        cls.used_channel_patch='InitializeP10Channel(ctx)' not in (build_root/'main.go').read_text()
        cls.include_channel=not cls.used_channel_patch or os.environ.get('HYJS_P10_INCLUDE_PENDING_CHANNEL')=='1'
        if cls.used_channel_patch and cls.include_channel:
            if build_root == ROOT/'hyjs-server':
                build_root=Path(cls.temp.name)/'source';shutil.copytree(ROOT/'hyjs-server',build_root,ignore=shutil.ignore_patterns('logs'))
            result=subprocess.run(['rtk','proxy','git','apply',str(ROOT/'analysis/p10/channel-integration.patch')],cwd=build_root,capture_output=True,text=True,timeout=30)
            if result.returncode:raise AssertionError(result.stdout+result.stderr)
        cls.used_lucky_patch='InitializeP10Lucky(ctx)' not in (build_root/'main.go').read_text()
        cls.include_lucky=not cls.used_lucky_patch or os.environ.get('HYJS_P10_INCLUDE_PENDING_LUCKY')=='1'
        if cls.used_lucky_patch and cls.include_lucky:
            if build_root == ROOT/'hyjs-server':
                build_root=Path(cls.temp.name)/'source';shutil.copytree(ROOT/'hyjs-server',build_root,ignore=shutil.ignore_patterns('logs'))
            result=subprocess.run(['rtk','proxy','git','apply',str(ROOT/'analysis/p10/lucky-integration.patch')],cwd=build_root,capture_output=True,text=True,timeout=30)
            if result.returncode:raise AssertionError(result.stdout+result.stderr)
        result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=build_root,capture_output=True,text=True,timeout=120)
        if result.returncode:raise AssertionError(result.stdout+result.stderr)
    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p10_') or not cls.test_user.startswith('p10_'):raise AssertionError('P10 isolation guard')
        r=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if r.returncode:raise AssertionError(r.stderr.replace(cls.test_password,'[redacted]'))
        return r.stdout
    def sql(self,text):return self.root_sql('USE `'+self.database+'`; '+text)
    def setUp(self):self.process=None;self.addCleanup(self.stop)
    def start(self):
        self.process=subprocess.Popen([self.binary,'--http-port','0','--tcp-port','0','--tables',str(ROOT/'analysis/tables')],cwd=ROOT/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ);self.assertTrue(ready.select(90),'P10冷启动超时');line=self.process.stdout.readline()
        m=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line);self.assertIsNotNone(m,line);self.service=types.SimpleNamespace(http_port=int(m[1]),tcp_port=int(m[2]))
    def stop(self):
        if self.process is not None and self.process.poll() is None:
            self.process.terminate()
            try:self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:self.process.kill();self.process.wait();self.fail('P10服务ctx退出超时')
        if self.process is not None and self.process.stdout:self.process.stdout.close()
    def call(self,name,fields,login,stamp,expected):
        op,r=self.stamped(name,fields,login,stamp);self.assertEqual(op,BY_NAME[expected],str(r));return r
    def profile(self,login):
        raw=self.sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id='+str(login.playerId)+';').strip().split('|');return [json.loads(bytes.fromhex(x)) for x in raw]
    def recv(self,sock):
        def exact(n):
            out=b''
            while len(out)<n:
                chunk=sock.recv(n-len(out))
                if not chunk:raise AssertionError('P10 TCP连接提前结束')
                out+=chunk
            return out
        header=exact(8);size,_=struct.unpack('>ii',header);return decode_responses(header+exact(size))[0]
    def tcp_send(self,sock,name,fields,login):
        payload=message(name,**fields).SerializeToString();sock.sendall(struct.pack('>iiiiqi',24+len(payload),1,BY_NAME[name],1,login.playerId,len(payload))+payload)
    def connect(self,login):
        sock=socket.create_connection(('127.0.0.1',self.service.tcp_port),timeout=3);self.addCleanup(sock.close)
        self.tcp_send(sock,'CSSocketLogin',{'sessionKey':bytes(login.sessionKey)},login);op,r=self.recv(sock);self.assertEqual(op,BY_NAME['SCSocketLogin']);self.assertEqual(r.result,100);return sock
    def test_1_innodb_real_relationship_concurrency_private_permissions(self):
        self.__class__.ran_go_fixture=True
        r=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP10','-count=1','-timeout=150s'],cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=180)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));self.__class__.ran_go_fixture=True;print(r.stdout.strip())
    def test_2_two_accounts_original_http_tcp_offline_and_session_rotation(self):
        self.start();uids=['p10-native-'+secrets.token_hex(4) for _ in range(3)];a,b,c=[self.login(uid) for uid in uids]
        for login,name in [(a,'社交甲原帧'),(b,'社交乙原帧'),(c,'社交丙原帧')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql('UPDATE p10_scene_policies SET enabled=0; UPDATE p10_social_policies SET enabled=1; UPDATE profiles SET player=JSON_SET(player,"$.level",20) WHERE id IN ('+','.join(str(x.playerId) for x in [a,b,c])+');')
        before=[self.profile(x) for x in [a,b,c]]
        found=self.call('CSSearchPlayer',{'idOrName':'社交乙原帧'},a,1,'SCSearchPlayer');self.assertEqual([p.playerId for p in found.player],[b.playerId]);self.assertGreater(found.player[0].fightPower,0)
        request={'friendId':b.playerId,'word':'原申请文本'};self.call('CSApplyFriend',request,a,11,'SCApplyFriend');self.call('CSApplyFriend',request,a,12,'SCApplyFriend')
        apply=self.call('CSFriendApplyList',{},b,1,'SCFriendApplyList');self.assertEqual([(x.friendId,x.word) for x in apply.applyList],[(a.playerId,'原申请文本')])
        op,r=self.stamped('CSAgreeApply',{'friendId':a.playerId},c,1);self.assertEqual(op,10001,str(r))
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:list(pool.map(lambda _:self.call('CSAgreeApply',{'friendId':a.playerId},b,13,'SCAgreeApply'),range(8)))
        for login,target in [(a,b),(b,a)]:self.assertEqual([f.simplePlayerInfo.playerId for f in self.call('CSFriendList',{},login,1,'SCFriendList').friendList],[target.playerId])
        sender=self.connect(a);receiver=self.connect(b);chat={'chatType':3,'playerId':b.playerId,'content':'真实原TCP私聊'}
        self.tcp_send(sender,'CSChatTcp',chat,a);op,ack=self.recv(sender);self.assertEqual(op,BY_NAME['SCChatTcp']);self.assertEqual(ack.play.playerId,a.playerId)
        op,incoming=self.recv(receiver);self.assertEqual(op,BY_NAME['SCChatTcp']);self.assertEqual(ack,incoming)
        self.tcp_send(sender,'CSChatTcp',chat,a);self.assertEqual(self.recv(sender)[1],ack);receiver.settimeout(.3)
        with self.assertRaises(socket.timeout):self.recv(receiver)
        receiver.settimeout(3)
        self.assertEqual(len(self.call('CSChatCache',{'chatType':3},c,1,'SCChatCache').chatList),0)
        # 重登真实轮换会话，旧TCP接收端失去下一条消息权限。
        fresh_b=self.login(uids[1]);self.sql('UPDATE p10_social_chat_states SET last_send_at=0 WHERE player_id='+str(a.playerId)+';')
        chat['content']='轮换后离线消息';self.tcp_send(sender,'CSChatTcp',chat,a);self.recv(sender);receiver.settimeout(.3)
        with self.assertRaises(socket.timeout):self.recv(receiver)
        receiver.close();new_receiver=self.connect(fresh_b)
        self.sql('UPDATE p10_social_chat_states SET last_send_at=0 WHERE player_id='+str(a.playerId)+';');chat['content']='新会话即时消息';self.tcp_send(sender,'CSChatTcp',chat,a);self.assertEqual(self.recv(sender)[1],self.recv(new_receiver)[1])
        self.assertEqual(len(self.call('CSChatCache',{'chatType':3},fresh_b,1,'SCChatCache').chatList),3)
        self.call('CSSendToBlackList',{'friendId':a.playerId},fresh_b,14,'SCSendToBlackList')
        for login in [a,fresh_b]:self.assertEqual(len(self.call('CSFriendList',{},login,1,'SCFriendList').friendList),0)
        self.assertEqual([x.playerId for x in self.call('CSFriendBlackList',{},fresh_b,1,'SCFriendBlackList').blackList],[a.playerId])
        op,r=self.stamped('CSApplyFriend',{'friendId':b.playerId},a,15);self.assertEqual(op,10001,str(r));self.assertEqual(len(self.call('CSChatCache',{'chatType':3},fresh_b,1,'SCChatCache').chatList),0)
        sender.close();new_receiver.close();self.stop();self.start();a=self.login(uids[0]);b=self.login(uids[1]);self.call('CSDeleteFromBlackList',{'friendId':a.playerId},b,16,'SCDeleteFromBlackList')
        self.assertEqual(len(self.call('CSChatCache',{'chatType':3},b,1,'SCChatCache').chatList),3);self.assertEqual(len(self.call('CSFriendList',{},b,1,'SCFriendList').friendList),0)
        self.assertEqual([self.profile(x) for x in [a,b,c]],before)
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p10_%';").strip(),'InnoDB')
        print('P10 pending shared patch used:',self.used_pending_patch)

    def test_3_native_friend_energy_real_give_claim_flags_and_restart(self):
        self.start();uids=['p10-energy-native-'+secrets.token_hex(4) for _ in range(3)];a,b,c=[self.login(uid) for uid in uids]
        for login,name in [(a,'耐力赠送甲'),(b,'耐力领取乙'),(c,'耐力赠送丙')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql('UPDATE p10_social_policies SET enabled=1; UPDATE p10_friend_energy_policies SET enabled=1,amount=1; UPDATE profiles SET player=JSON_SET(player,"$.energy",28,"$.energyLastRefreshTime",CAST(UNIX_TIMESTAMP(NOW(3))*1000 AS UNSIGNED)) WHERE id='+str(b.playerId)+';')
        for sender,stamp in [(a,21),(c,22)]:self.call('CSApplyFriend',{'friendId':b.playerId},sender,stamp,'SCApplyFriend');self.call('CSAgreeApply',{'friendId':sender.playerId},b,stamp,'SCAgreeApply')
        before_sender=self.profile(a);self.call('CSGiveEnergy',{'friendId':b.playerId},a,31,'SCGiveEnergy');self.call('CSGiveEnergy',{'friendId':b.playerId},a,32,'SCGiveEnergy')
        pending=self.call('CSFriendList',{},b,1,'SCFriendList');self.assertEqual(next(f for f in pending.friendList if f.simplePlayerInfo.playerId==a.playerId).canGet,1)
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:claims=list(pool.map(lambda _:self.call('CSGetEnergy',{'friendId':a.playerId},b,33,'SCGetEnergy'),range(6)))
        self.assertTrue(all(r.currentEnergy==29 for r in claims));self.assertEqual(self.profile(a),before_sender)
        after=self.call('CSFriendList',{},b,1,'SCFriendList');self.assertEqual(after.getEnergyNum,1);self.assertEqual(next(f for f in after.friendList if f.simplePlayerInfo.playerId==a.playerId).canGet,0)
        self.call('CSBatchSendFriendEnergy',{},c,34,'SCBatchSendFriendEnergy');batch=self.call('CSBatchGetFriendEnergy',{},b,35,'SCBatchGetFriendEnergy');self.assertEqual(list(batch.getIds),[c.playerId]);self.assertEqual(batch.add.simps[0].num,1)
        self.assertEqual(batch,self.call('CSBatchGetFriendEnergy',{},b,35,'SCBatchGetFriendEnergy'));self.assertEqual(self.profile(b)[0]['energy'],30)
        op,r=self.stamped('CSGetEnergy',{'friendId':a.playerId},c,36);self.assertEqual(op,10001,str(r))
        self.sql('UPDATE p10_friend_energy_policies SET enabled=0,amount=7;')
        self.stop();self.start();b=self.login(uids[1]);self.assertEqual(self.call('CSGetEnergy',{'friendId':a.playerId},b,33,'SCGetEnergy').currentEnergy,29)
        self.assertEqual(batch,self.call('CSBatchGetFriendEnergy',{},b,35,'SCBatchGetFriendEnergy'));self.assertEqual(self.profile(b)[0]['energy'],30)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_friend_energy_gifts WHERE receiver_id='+str(b.playerId)+' AND claimed=1;').strip(),'2')
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p1_event_facts WHERE player_id="+str(b.playerId)+" AND kind='friend_energy_received';").strip(),'2')
        print('P10 energy pending patch used:',self.used_energy_patch)

    def test_4_original_full_actual_formation_profile_permissions_and_restart(self):
        self.start();owner=self.login('p10-native-profile');uid='p10-profile-reader-'+secrets.token_hex(4);reader=self.login(uid)
        self.call('CSPlayerCreate',{'playerName':'真实资料查看方','heroId':'hero_047'},reader,1,'SCPlayerCreate');self.sql('UPDATE p10_social_policies SET enabled=1;')
        before=self.profile(owner);req={'playerId':owner.playerId,'serverId':1}
        first=self.call('CSFindPlayerInfo',req,reader,1,'SCFindPlayerInfo');info=first.formationInfo
        self.assertEqual(info.name,'原真实阵容资料');self.assertGreater(info.fightPower,0);self.assertEqual(len(info.formationItemInfoList),1)
        slot=info.formationItemInfoList[0];self.assertEqual((slot.warInfo.heroId,slot.warInfo.level),('hero_047',20));self.assertGreater(slot.warInfo.baseProp.att,0)
        self.assertEqual([e.equipId for e in slot.equipInfoList],['equip_013']);self.assertEqual([t.treasureId for t in slot.treaInfoList],['trea_001']);self.assertEqual(self.profile(owner),before)
        self.stop();self.start();owner=self.login('p10-native-profile');reader=self.login(uid);self.assertEqual(first,self.call('CSFindPlayerInfo',req,reader,1,'SCFindPlayerInfo'))
        self.call('CSSendToBlackList',{'friendId':reader.playerId},owner,2,'SCSendToBlackList');op,r=self.stamped('CSFindPlayerInfo',req,reader,1);self.assertEqual(op,10001,str(r))
        op,r=self.stamped('CSFindPlayerInfo',{'playerId':owner.playerId,'serverId':2},owner,1);self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(owner),before)
        print('P10 profile pending patch used:',self.used_profile_patch)

    def test_5_three_real_tcp_scene_move_line_visibility_session_and_restart(self):
        if not self.include_scene:self.skipTest('只验当前真实登录候选，不临时接未注册Scene')
        self.start();uids=['p10-scene-native-'+secrets.token_hex(4) for _ in range(3)];a,b,c=[self.login(uid) for uid in uids]
        for login,name in [(a,'同屏甲'),(b,'同屏乙'),(c,'跨线丙')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql('UPDATE p10_scene_policies SET enabled=1,line_count=2,max_players=100;')
        sa=self.connect(a);sb=self.connect(b)
        self.assertEqual([p.info.playerId for p in self.recv(sa)[1].player],[b.playerId]);self.assertEqual([p.info.playerId for p in self.recv(sb)[1].player],[a.playerId])
        sc=self.connect(c);self.assertEqual([p.info.playerId for p in self.recv(sa)[1].player],[c.playerId]);self.assertEqual([p.info.playerId for p in self.recv(sb)[1].player],[c.playerId]);self.assertEqual([p.info.playerId for p in self.recv(sc)[1].player],[a.playerId,b.playerId])
        lines=self.call('CSGetLineInfo',{},a,1,'SCGetLineInfo');self.assertEqual(lines.line,1);self.assertEqual([x.line for x in lines.lineInfos],[1,2])
        self.call('CSTransferLine',{'line':2},c,51,'SCTransferLine');self.assertEqual(list(self.recv(sa)[1].playerId),[c.playerId]);self.assertEqual(list(self.recv(sb)[1].playerId),[c.playerId]);self.assertEqual(list(self.recv(sc)[1].playerId),[a.playerId,b.playerId])
        self.tcp_send(sa,'CSMove',{'pointNew':{'x':7,'y':0,'z':9},'currentTime':1700000000000},a);op,moved=self.recv(sb);self.assertEqual(op,BY_NAME['SCMovePush']);self.assertEqual((moved.position[0].playerId,moved.position[0].current.x),(a.playerId,7));sc.settimeout(.3)
        with self.assertRaises(socket.timeout):self.recv(sc)
        sc.settimeout(3)
        visible=self.call('CSMovePlayerInfo',{'playerIds':[a.playerId,b.playerId,c.playerId,c.playerId]},a,1,'SCMovePlayerInfo');self.assertEqual([p.info.playerId for p in visible.playerInfos],[a.playerId,b.playerId])
        self.call('CSTransferLine',{'line':2},a,52,'SCTransferLine');self.assertEqual(list(self.recv(sb)[1].playerId),[a.playerId]);self.assertEqual(list(self.recv(sa)[1].playerId),[b.playerId]);self.assertEqual([p.info.playerId for p in self.recv(sc)[1].player],[a.playerId]);self.assertEqual([p.info.playerId for p in self.recv(sa)[1].player],[c.playerId])
        fresh_a=self.login(uids[0]);new_sa=self.connect(fresh_a);self.assertEqual(list(self.recv(sc)[1].playerId),[a.playerId]);self.assertEqual([p.info.playerId for p in self.recv(sc)[1].player],[a.playerId]);self.assertEqual([p.info.playerId for p in self.recv(new_sa)[1].player],[c.playerId])
        self.tcp_send(sa,'CSMove',{'pointNew':{'x':99,'y':0,'z':9},'currentTime':1700000000001},a);op,r=self.recv(sa);self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(fresh_a)[1]['position']['current']['x'],7)
        sa.close();new_sa.close();sb.close();sc.close();self.stop();self.start();a=self.login(uids[0]);b=self.login(uids[1]);sa=self.connect(a)
        restored=self.call('CSGetLineInfo',{},a,1,'SCGetLineInfo');self.assertEqual(restored.line,2)
        nobody=self.call('CSMovePlayerInfo',{'playerIds':[b.playerId]},a,1,'SCMovePlayerInfo');self.assertEqual(len(nobody.playerInfos),0)
        print('P10 scene pending patch used:',self.used_scene_patch)

    def test_6_initial_original_login_queries_work_with_default_closed_policy(self):
        self.start();uid='p10-closed-bootstrap-'+secrets.token_hex(4);login=self.login(uid)
        self.call('CSPlayerCreate',{'playerName':'默认关闭登录原帧','heroId':'hero_047'},login,1,'SCPlayerCreate')
        # 联合组前序Go显式启用过有限夹具时，仅在专库恢复关闭；单独热修复验证不做该SQL。
        if self.ran_go_fixture:
            self.sql('UPDATE p10_social_policies SET enabled=0; UPDATE p10_scene_policies SET enabled=0;')
            if self.include_channel:self.sql('UPDATE p10_channel_policies SET world_enabled=0,field_enabled=0;')
            if self.include_lucky:self.sql('UPDATE p10_lucky_policies SET fireworks_enabled=0,drop_enabled=0;')
        self.assertEqual(self.sql('SELECT enabled FROM p10_social_policies WHERE id=1;').strip(),'0')
        if self.include_lucky:
            self.assertEqual(self.sql('SELECT CONCAT(fireworks_enabled,drop_enabled) FROM p10_lucky_policies WHERE id=1;').strip(),'00')
            self.assertEqual(len(self.call('CSCurrentLuckyBagsInfo',{},login,1,'SCCurrentLuckyBagsInfo').luckyBags),0)
        before=self.profile(login)
        self.assertEqual(self.sql('SELECT enabled FROM p10_scene_policies WHERE id=1;').strip(),'0')
        if self.include_channel:
            self.assertEqual(self.sql('SELECT CONCAT(world_enabled,field_enabled) FROM p10_channel_policies WHERE id=1;').strip(),'00')
            self.assertEqual(len(self.call('CSSceneChatCache',{},login,1,'SCSceneChatCache').chatList),0)
        for name,fields in [('CSRefreshMovePlayerInfo',{}),('CSMovePlayerInfo',{}),('CSMovePlayerInfo',{'playerIds':[login.playerId]})]:
            reply=self.call(name,fields,login,1,'SCMovePlayerInfo');self.assertEqual([x.info.playerId for x in reply.playerInfos],[login.playerId]);self.assertEqual(reply.playerInfos[0].info.warId,'hero_047')
        sock=self.connect(login)
        self.tcp_send(sock,'CSRefreshMovePlayerInfo',{},login);op,reply=self.recv(sock);self.assertEqual(op,BY_NAME['SCMovePlayerInfo']);self.assertEqual([x.info.playerId for x in reply.playerInfos],[login.playerId])
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_scene_presence WHERE player_id='+str(login.playerId)+';').strip(),'0')
        sock.close()
        for name,expected in [('CSFriendList','SCFriendList'),('CSFriendBlackList','SCFriendBlackList'),('CSFriendApplyList','SCFriendApplyList'),('CSRecommendFriendList','SCRecommendFriendList')]:self.call(name,{},login,1,expected)
        for channel in [0,1,2,3,4,5,6]:self.assertEqual(len(self.call('CSChatCache',{'chatType':channel},login,1,'SCChatCache').chatList),0)
        detail=self.call('CSFindPlayerInfo',{'playerId':login.playerId,'serverId':1},login,1,'SCFindPlayerInfo');self.assertEqual(detail.formationInfo.name,'默认关闭登录原帧')
        target=self.login('p10-closed-target-'+secrets.token_hex(4));self.call('CSPlayerCreate',{'playerName':'关闭关系目标','heroId':'hero_047'},target,1,'SCPlayerCreate')
        mutations=[('CSApplyFriend',{'friendId':target.playerId}),('CSChatTcp',{'chatType':3,'playerId':target.playerId,'content':'关闭时拒绝的新消息'})]
        if self.include_channel:mutations.extend([('CSChatTcp',{'chatType':1,'content':'关闭的世界文字'}),('CSChatTcp',{'chatType':5,'content':'关闭的野外文字'})])
        if self.include_lucky:mutations.extend([('CSItemUse',{'costItem':{'id':'item_350','num':1}}),('CSThrowLuckyBags',{'num':1,'posArray':[{'x':0,'y':0,'z':0}]})])
        for name,fields in mutations:
            op,r=self.stamped(name,fields,login,21);self.assertEqual(op,10001,str(r));self.assertIn('尚未开放',r.msg)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_social_applies WHERE sender_id='+str(login.playerId)+';').strip(),'0');self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_social_chats WHERE sender_id='+str(login.playerId)+';').strip(),'0')
        self.assertEqual(self.profile(login),before);self.assertEqual(self.sql('SELECT enabled FROM p10_social_policies WHERE id=1;').strip(),'0')
        self.stop();self.start();login=self.login(uid);self.call('CSFriendList',{},login,1,'SCFriendList');self.call('CSFriendBlackList',{},login,1,'SCFriendBlackList');self.call('CSChatCache',{},login,1,'SCChatCache')
        print('P10 bootstrap current source pending social/energy/profile:',self.used_pending_patch,self.used_energy_patch,self.used_profile_patch,'Scene extension included:',self.include_scene,'Fresh default without fixture reset:',not self.ran_go_fixture)

    def test_7_original_world_field_text_visibility_black_retry_history_and_restart(self):
        if not self.include_channel:self.skipTest('未集成Channel，临时接点需显式HYJS_P10_INCLUDE_PENDING_CHANNEL=1')
        self.start();uids=['p10-channel-native-'+secrets.token_hex(4) for _ in range(3)];a,b,c=[self.login(uid) for uid in uids]
        for login,name in [(a,'频道甲'),(b,'频道乙'),(c,'频道丙')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        # 联合Go夹具留下的是专库真实公共历史，本原帧用例先清该专属测试域，避免与50条夹具混读。
        self.sql('DELETE FROM p10_channel_chats; UPDATE p10_social_policies SET enabled=1; UPDATE p10_scene_policies SET enabled=1; UPDATE p10_channel_policies SET world_enabled=1,field_enabled=1; UPDATE profiles SET player=JSON_SET(player,"$.level",20,"$.noviceGuideSchedule",7) WHERE id IN ('+','.join(str(x.playerId) for x in [a,b,c])+');')
        for login in [a,b,c]:self.call('CSTransferScene',{'target':'field_001'},login,11,'SCTransferScene')
        sa=self.connect(a);sb=self.connect(b);self.recv(sa);self.recv(sb)
        sc=self.connect(c);self.recv(sa);self.recv(sb);self.recv(sc)
        self.call('CSTransferLine',{'line':2},c,12,'SCTransferLine');self.recv(sa);self.recv(sb);self.recv(sc)
        fields={'chatType':5,'content':'原野外同线文字'};self.tcp_send(sa,'CSChatTcp',fields,a);op,original=self.recv(sa);self.assertEqual(op,BY_NAME['SCChatTcp']);self.assertEqual(original.play.playerId,a.playerId);self.assertEqual(self.recv(sb)[1],original)
        sc.settimeout(.2)
        with self.assertRaises(socket.timeout):self.recv(sc)
        sc.settimeout(3)
        self.tcp_send(sa,'CSChatTcp',fields,a);self.assertEqual(self.recv(sa)[1],original);sb.settimeout(.2)
        with self.assertRaises(socket.timeout):self.recv(sb)
        sb.settimeout(3)
        self.assertEqual(len(self.call('CSSceneChatCache',{},b,1,'SCSceneChatCache').chatList),1);self.assertEqual(len(self.call('CSSceneChatCache',{},c,1,'SCSceneChatCache').chatList),0)
        self.call('CSSendToBlackList',{'friendId':a.playerId},b,13,'SCSendToBlackList');self.assertEqual(len(self.call('CSChatCache',{'chatType':5},b,1,'SCChatCache').chatList),0)
        world={'chatType':1,'content':'原世界跨线文字'};self.tcp_send(sc,'CSChatTcp',world,c);op,message=self.recv(sc);self.assertEqual(op,BY_NAME['SCChatTcp']);self.assertEqual(self.recv(sa)[1],message);self.assertEqual(self.recv(sb)[1],message)
        self.tcp_send(sa,'CSChatTcp',{'chatType':1,'content':'绕过频道节流'},a);self.assertEqual(self.recv(sa)[0],10001)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_channel_chats;').strip(),'2')
        self.assertEqual([x.content for x in self.call('CSChatCache',{},a,1,'SCChatCache').chatList],['原野外同线文字','原世界跨线文字'])
        self.assertEqual([x.content for x in self.call('CSChatCache',{},b,1,'SCChatCache').chatList],['原世界跨线文字'])
        sa.close();sb.close();sc.close();self.stop();self.start();a=self.login(uids[0]);b=self.login(uids[1]);c=self.login(uids[2])
        # 关闭新消息仍提供真实世界历史；野外历史重新要求当前场景TCP资格。
        self.sql('UPDATE p10_channel_policies SET world_enabled=0,field_enabled=0;')
        history=self.call('CSChatCache',{'chatType':1},a,1,'SCChatCache');self.assertEqual([x.content for x in history.chatList],['原世界跨线文字'])
        self.assertEqual(len(self.call('CSSceneChatCache',{},a,1,'SCSceneChatCache').chatList),0)
        sa=self.connect(a);self.assertEqual([x.content for x in self.call('CSSceneChatCache',{},a,1,'SCSceneChatCache').chatList],['原野外同线文字'])
        self.tcp_send(sa,'CSChatTcp',world,a);op,r=self.recv(sa);self.assertEqual(op,10001,str(r));self.assertIn('尚未开放',r.msg)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_channel_chats;').strip(),'2')
        print('P10 channel pending patch used:',self.used_channel_patch)

    def test_8_original_fireworks_real20_bags_shared_throw_claim_race_and_restart(self):
        if not self.include_lucky:self.skipTest('未集成Lucky，临时接点需显式HYJS_P10_INCLUDE_PENDING_LUCKY=1')
        self.start();uids=['p10-lucky-native-'+secrets.token_hex(4) for _ in range(3)];a,b,c=[self.login(uid) for uid in uids]
        for login,name in [(a,'撒福袋甲'),(b,'抢福袋乙'),(c,'抢福袋丙')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql('DELETE FROM p10_lucky_objects; DELETE FROM p10_lucky_dailies; UPDATE p10_social_policies SET enabled=1; UPDATE p10_scene_policies SET enabled=1; UPDATE p10_channel_policies SET world_enabled=1; UPDATE p10_lucky_policies SET fireworks_enabled=1,drop_enabled=1; UPDATE profiles SET player=JSON_SET(player,"$.level",70,"$.noviceGuideSchedule",7) WHERE id='+str(a.playerId)+';')
        # 有限隔离准备通过真实P1邮件领奖形成350来源，原节日活动获取350仍未恢复。
        self.sql('INSERT INTO p1_reward_deliveries(player_id,source_key,title,content,rewards,created_at,expires_at,claimed_at) VALUES('+str(a.playerId)+',"p10-lucky-native-fixture","有限烟花源","隔离来源","item_350,1",1,0,0);')
        inbox=self.call('CSQueryUserReward',{},a,1,'SCQueryUserReward');self.assertEqual(len(inbox.rewardItemInfoList),1)
        gain=self.call('CSAddRewardsToPlayer',{'rewardIds':[inbox.rewardItemInfoList[0].rewardId]},a,2,'SCAddRewardsToPlayer');self.assertEqual([(x.id,x.num) for x in gain.result.simps],[('item_350',1)])
        sa=self.connect(a);sb=self.connect(b);self.recv(sa);self.recv(sb);sc=self.connect(c);self.recv(sa);self.recv(sb);self.recv(sc)
        before=self.profile(a)
        fire={'costItem':{'id':'item_350','num':1}};converted=self.call('CSItemUse',fire,a,501,'SCItemUse');self.assertEqual([(x.id,x.num) for x in converted.get.simps],[('item_349',20)])
        for sock in [sa,sb,sc]:op,visual=self.recv(sock);self.assertEqual(op,BY_NAME['SCSpecialEfficiency']);self.assertEqual((visual.index,visual.playerId),(1,a.playerId))
        self.assertEqual(self.call('CSItemUse',fire,a,501,'SCItemUse'),converted)
        after=self.profile(a);stacks={x['id']:x['num'] for x in after[1]['items']};self.assertEqual(stacks.get('item_349'),20);self.assertEqual(stacks.get('item_350',0),0)
        fields={'num':2,'posArray':[{'x':0,'y':0,'z':0},{'x':1,'y':0,'z':0}]};self.tcp_send(sa,'CSThrowLuckyBags',fields,a)
        op,cost=self.recv(sa);self.assertEqual(op,BY_NAME['SCThrowLuckyBags']);self.assertEqual([(x.id,x.num) for x in cost.costResource],[('item_349',2)])
        appeared=self.recv(sa)[1];self.assertEqual(len(appeared.luckyBags),2);self.assertEqual(self.recv(sb)[1],appeared);self.assertEqual(self.recv(sc)[1],appeared)
        self.assertEqual(len(self.call('CSCurrentLuckyBagsInfo',{},b,1,'SCCurrentLuckyBagsInfo').luckyBags),2)
        self.tcp_send(sa,'CSThrowLuckyBags',fields,a);self.assertEqual(self.recv(sa)[1],cost);sb.settimeout(.2)
        with self.assertRaises(socket.timeout):self.recv(sb)
        sb.settimeout(3)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_lucky_objects;').strip(),'2');self.assertEqual({x['id']:x['num'] for x in self.profile(a)[1]['items']}['item_349'],18)
        target=appeared.luckyBags[0].guid
        def snatch(login,sock):
            responses=[];removed=[]
            for _ in range(4):
                self.tcp_send(sock,'CSSnatchOneLuckyBag',{'guid':target},login)
                while True:
                    op,reply=self.recv(sock)
                    if op==BY_NAME['SCRemoveOneLuckyBag']:removed.append(reply.guid);continue
                    self.assertIn(op,[BY_NAME['SCSnatchOneLuckyBag'],10001]);responses.append((login.playerId,op,reply));break
            return responses,removed
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:parts=list(pool.map(lambda args:snatch(*args),[(b,sb),(c,sc)]))
        results=[row for responses,_ in parts for row in responses]
        winners=[id for id,op,_ in results if op==BY_NAME['SCSnatchOneLuckyBag']];self.assertTrue(winners);self.assertEqual(len(set(winners)),1)
        winner=b if winners[0]==b.playerId else c;original=next(reply for _,op,reply in results if op==BY_NAME['SCSnatchOneLuckyBag']);self.assertEqual(original.throwPlayerName,'撒福袋甲')
        op,removed=self.recv(sa);self.assertEqual(op,BY_NAME['SCRemoveOneLuckyBag']);self.assertEqual(removed.guid,target)
        for _,removed in parts:self.assertEqual(removed,[target])
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_lucky_objects WHERE claimed_at>0;').strip(),'1');self.assertEqual(self.sql('SELECT snatched FROM p10_lucky_dailies WHERE player_id='+str(winner.playerId)+';').strip(),'1')
        second=appeared.luckyBags[1].guid;frozen=self.sql('SELECT CONCAT(reward,"|",reward_hash,"|",HEX(frame)) FROM p10_lucky_objects WHERE guid="'+second+'";').strip()
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p1_event_facts WHERE player_id='+str(a.playerId)+' AND kind="fireworks_used";').strip(),'1')
        sa.close();sb.close();sc.close();self.stop();self.start();a,b,c=[self.login(uid) for uid in uids];winner=b if winners[0]==b.playerId else c;sa=self.connect(a);sb=self.connect(b);self.recv(sa);self.recv(sb)
        self.sql('UPDATE p10_lucky_policies SET fireworks_enabled=0,drop_enabled=0,reward_json="[]",reward_hash="closed_new_pool";')
        self.assertEqual(self.call('CSSnatchOneLuckyBag',{'guid':target},winner,610,'SCSnatchOneLuckyBag'),original)
        self.assertEqual(self.sql('SELECT CONCAT(reward,"|",reward_hash,"|",HEX(frame)) FROM p10_lucky_objects WHERE guid="'+second+'";').strip(),frozen)
        current=self.call('CSCurrentLuckyBagsInfo',{},b,1,'SCCurrentLuckyBagsInfo');self.assertEqual([x.guid for x in current.luckyBags],[second])
        claimed=self.call('CSSnatchOneLuckyBag',{'guid':second},b,620,'SCSnatchOneLuckyBag');self.assertEqual(claimed.throwPlayerName,'撒福袋甲')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_lucky_objects WHERE claimed_at>0;').strip(),'2')
        print('P10 lucky pending patch used:',self.used_lucky_patch)

del LiveServerTests
del GoFormationTests
