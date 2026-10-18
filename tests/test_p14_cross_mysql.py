"""P14两个真实隔离Go服、各自InnoDB和同本地ID：原帧/HMAC/离线outbox恢复。"""
import concurrent.futures
import hashlib
import hmac
import http.client
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import socket
import struct
import subprocess
import time
import unittest
from server.codec import BY_NAME,CLASSES,encode_http_request,decode_responses,message
import test_p13_mysql as guild
ROOT=Path(__file__).resolve().parents[1]
SOURCE=Path(os.environ.get('HYJS_P14_SOURCE_ROOT',str(ROOT))).resolve()
if SOURCE!=ROOT and not SOURCE.name.startswith('hyjs-p14-season-'):
    raise AssertionError('P14 source isolation guard')

@unittest.skipUnless(os.environ.get('HYJS_P14_MYSQL_TEST')=='1','显式启用P14独立真实两服MySQL')
class P14CrossMySQLTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        guild.P13MySQLTests.setUpClass.__func__(cls)
        cls.realm_connections = {zone: cls.managed_mysql.allocate('p13_p14_realm' + str(zone)) for zone in [1, 2]}
        cls.realms = {zone: connection.database for zone, connection in cls.realm_connections.items()}
    root_sql=classmethod(guild.P13MySQLTests.root_sql.__func__)
    def sql(self,zone,text):return self.root_sql('USE `'+self.realms[zone]+'`; '+text)
    def setUp(self):self.processes={};self.ports={};self.addCleanup(self.stop_all)
    def stop_all(self):
        for zone in list(self.processes):self.stop(zone)
    def start(self,zone,fixed=None):
        env=dict(self.env,HYJS_MYSQL_DATABASE=self.realms[zone])
        http_port,tcp_port=fixed or (0,0)
        process=subprocess.Popen([self.binary,'--server-id',str(zone),'--http-port',str(http_port),'--tcp-port',str(tcp_port),'--tables',str(SOURCE/'analysis/tables'),'--log-file',str(Path(self.temp.name)/('p14-zone-'+str(zone)+'.log'))],cwd=SOURCE/'hyjs-server',env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        self.processes[zone]=process
        with selectors.DefaultSelector() as ready:
            ready.register(process.stdout,selectors.EVENT_READ);self.assertTrue(ready.select(90),'P14原服冷启动超时');line=process.stdout.readline()
        match=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line);self.assertIsNotNone(match,line.replace(self.test_password,'[redacted]'));self.ports[zone]=(int(match[1]),int(match[2]))
    def stop(self,zone):
        process=self.processes.pop(zone,None)
        if process is None:return
        if process.poll() is None:
            process.terminate()
            try:process.wait(timeout=10)
            except subprocess.TimeoutExpired:process.kill();process.wait();self.fail('P14 ctx停止超时')
        if process.stdout:process.stdout.close()
    def send(self,zone,name,fields=None,login=None,stamp=1700000000000,packet_zone=None):
        raw=encode_http_request(BY_NAME[name],message(name,**(fields or {})),zone_id=zone if packet_zone is None else packet_zone,player_id=login.playerId if login else 0,session=bytes(login.sessionKey) if login else bytes(16),timestamp=stamp)
        connection=http.client.HTTPConnection('127.0.0.1',self.ports[zone][0],timeout=5)
        try:
            connection.request('POST','/sao/',body=raw,headers={'Content-Type':'application/octet-stream'});response=connection.getresponse();data=response.read();self.assertEqual(response.status,200,data);return decode_responses(data)[0]
        finally:connection.close()
    def call(self,zone,name,fields,login,stamp,expected):
        op,response=self.send(zone,name,fields,login,stamp);self.assertEqual(op,BY_NAME[expected],str(response));return response
    def login(self,zone,uid):return self.call(zone,'CSUserLogin',{'platformUid':uid,'authCode':'isolated-local-pass'},None,1,'SCUserLogin')
    def wait_sql(self,zone,query,want,timeout=25):
        deadline=time.monotonic()+timeout
        while time.monotonic()<deadline:
            actual=self.sql(zone,query).strip()
            if actual==want:return
            time.sleep(.25)
        self.fail('P14持久状态超时：'+actual+' != '+want)
    def internal(self,zone,payload,key=None,signature=None):
        if signature is None:signature=hmac.new(key,payload,hashlib.sha256).hexdigest()
        connection=http.client.HTTPConnection('127.0.0.1',self.ports[zone][0],timeout=5)
        try:
            connection.request('POST','/internal/p14/snapshot',body=payload,headers={'Content-Type':'application/json','X-HYJS-P14-Signature':signature});response=connection.getresponse();return response.status,json.loads(response.read())
        finally:connection.close()
    def tcp_send(self,zone,sock,name,fields,login):
        payload=message(name,**fields).SerializeToString();sock.sendall(struct.pack('>iiiiqi',24+len(payload),1,BY_NAME[name],zone,login.playerId,len(payload))+payload)
    def tcp(self,zone,login):
        sock=socket.create_connection(('127.0.0.1',self.ports[zone][1]),timeout=3);self.addCleanup(sock.close)
        self.tcp_send(zone,sock,'CSSocketLogin',{'sessionKey':bytes(login.sessionKey)},login);op,reply=guild.P13MySQLTests.recv(self,sock);self.assertEqual((op,reply.result),(BY_NAME['SCSocketLogin'],100));return sock
    def test_1_mysql_race_real_snapshot_identity_permissions_replay_and_outbox(self):
        r=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP14Cross','-count=1','-timeout=120s'],cwd=SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
    def test_3_mysql_race_all_published_original_calendar_stages(self):
        r=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP14CrossCalendar','-count=1','-timeout=120s'],cwd=SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
    def test_4_two_real_servers_original_calendar_frames_history_and_restart(self):
        r=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=SOURCE/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(r.returncode,0,r.stdout+r.stderr)
        self.start(1);self.start(2);uids={zone:'p14-calendar-'+str(zone)+'-'+secrets.token_hex(4) for zone in [1,2]};logins={zone:self.login(zone,uids[zone]) for zone in [1,2]}
        for zone in [1,2]:self.call(zone,'CSPlayerCreate',{'playerName':'原日历服'+str(zone),'heroId':'hero_047'},logins[zone],2,'SCPlayerCreate')
        now=int(time.time()*1000);base=now-5400000
        values=','.join('(9,'+str(base+(stage-1)*3600000)+','+str(stage)+','+str(base+stage*3600000)+",'明确原阶段"+str(stage)+"','隔离发布时间fixture：原运营日期未知')" for stage in range(1,20))
        for zone in [1,2]:self.sql(zone,'DELETE FROM p14_time_slots; UPDATE p14_season_policies SET enabled=1,season_index=9,register_start='+str(base+3600000)+',register_end='+str(base+7200000)+'; INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES'+values+';')
        responses={zone:self.call(zone,'CSKFTimeInfo',{'peakMatchIndex':9},logins[zone],3,'SCKFTimeInfo') for zone in [1,2]};self.assertEqual(responses[1],responses[2]);self.assertEqual([x.stage for x in responses[1].timeList],list(range(1,20)));self.assertEqual(responses[1].timeList[0].start,base)
        op,_=self.send(1,'CSKFTimeInfo',{'peakMatchIndex':-1},logins[1],4);self.assertEqual(op,10001)
        sock=self.tcp(2,logins[2]);self.tcp_send(2,sock,'CSKFTimeInfo',{'peakMatchIndex':0},logins[2]);op,reply=guild.P13MySQLTests.recv(self,sock);self.assertEqual(op,BY_NAME['SCKFTimeInfo']);self.assertEqual(reply,responses[2])
        # 历史日历独立保留，当前没有已发布赛季10时不借用旧赛程。
        for zone in [1,2]:self.sql(zone,'UPDATE p14_season_policies SET season_index=10,register_start=0,register_end=0;')
        for zone in [1,2]:
            self.assertEqual(self.call(zone,'CSKFTimeInfo',{'peakMatchIndex':9},logins[zone],5,'SCKFTimeInfo'),responses[zone]);op,_=self.send(zone,'CSKFTimeInfo',{},logins[zone],6);self.assertEqual(op,10001)
        for zone in [1,2]:self.stop(zone)
        for zone in [1,2]:self.start(zone);logins[zone]=self.login(zone,uids[zone]);self.assertEqual(self.call(zone,'CSKFTimeInfo',{'peakMatchIndex':9},logins[zone],7,'SCKFTimeInfo'),responses[zone])
        # 重叠发布的两个阶段拒绝投影，不能任意选取其中一个时刻。
        self.sql(1,'UPDATE p14_time_slots SET end_at='+str(base+7200000)+' WHERE season_index=9 AND stage=1;');op,_=self.send(1,'CSKFTimeInfo',{'peakMatchIndex':9},logins[1],8);self.assertEqual(op,10001)
    def test_2_two_isolated_real_servers_original_frames_same_id_signed_route_offline_restart(self):
        r=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=SOURCE/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(r.returncode,0,r.stdout+r.stderr)
        self.start(1);self.start(2);uids={1:'p14-realm1-'+secrets.token_hex(4),2:'p14-realm2-'+secrets.token_hex(4)};logins={zone:self.login(zone,uids[zone]) for zone in [1,2]}
        self.assertEqual(logins[1].playerId,logins[2].playerId);pid=logins[1].playerId
        for zone in [1,2]:self.call(zone,'CSPlayerCreate',{'playerName':'真实原服'+str(zone),'heroId':'hero_047'},logins[zone],2,'SCPlayerCreate')
        # 真实两服独立认证后，客户端任意zone或另一服会话均不能冒认原服。
        for zone in [1,2]:
            op,_=self.send(zone,'CSKFLogin',{},logins[zone],3,packet_zone=3-zone);self.assertEqual(op,10001)
            op,_=self.send(zone,'CSKFLogin',{},logins[3-zone],3);self.assertEqual(op,10001)
        key=secrets.token_bytes(32);now=int(time.time()*1000)
        for zone in [1,2]:
            self.sql(zone,'UPDATE profiles SET player=JSON_SET(player,"$.level",70) WHERE id='+str(pid)+'; UPDATE p14_season_policies SET enabled=1,season_index=9,register_start='+str(now-60000)+',register_end='+str(now+1800000)+'; INSERT INTO p14_trusted_peers(server_id,enabled,endpoint,signing_key) VALUES('+str(3-zone)+',1,"http://127.0.0.1:'+str(self.ports[3-zone][0])+'",UNHEX("'+key.hex()+'"));')
        for zone in [1,2]:
            for name in ['CSKFEnterGame','CSKFEnterFrightGame']:op,_=self.send(zone,name,{},logins[zone],0);self.assertEqual(op,10001)
            self.call(zone,'CSKFEnterGame',{},logins[zone],10,'SCKFEnterGame')
        for zone in [1,2]:
            self.wait_sql(zone,'SELECT COUNT(*) FROM p14_members WHERE season_index=9 AND player_id='+str(pid)+';','2')
            self.wait_sql(zone,'SELECT COUNT(*) FROM p14_deliveries WHERE confirmed=1;','1')
            local=self.call(zone,'CSKFLogin',{},logins[zone],11,'SCKFLogin');self.assertEqual((local.isEnter,local.typeStage,local.kfOrder),(1,2,9))
            info=self.call(zone,'CSKFPlayerInfo',{'playerId':pid,'serverId':3-zone,'peakMatchIndex':9,'replayFlag':0},logins[zone],12,'SCKFPlayerInfo');self.assertEqual(info.formationInfo.name,'真实原服'+str(3-zone));self.assertGreater(info.formationInfo.fightPower,0)
            self.assertEqual(self.sql(zone,'SELECT COUNT(DISTINCT server_id) FROM p14_members WHERE player_id='+str(pid)+';').strip(),'2')
        sock=self.tcp(2,logins[2]);self.tcp_send(2,sock,'CSKFLogin',{},logins[2]);op,reply=guild.P13MySQLTests.recv(self,sock);self.assertEqual((op,reply.isEnter,reply.kfOrder),(BY_NAME['SCKFLogin'],1,9))
        self.tcp_send(2,sock,'CSKFPlayerInfo',{'playerId':pid,'serverId':1,'peakMatchIndex':9,'replayFlag':0},logins[2]);op,reply=guild.P13MySQLTests.recv(self,sock);self.assertEqual((op,reply.formationInfo.name),(BY_NAME['SCKFPlayerInfo'],'真实原服1'))
        original=self.sql(1,'SELECT HEX(payload) FROM p14_deliveries ORDER BY created_at LIMIT 1;').strip();payload=bytes.fromhex(original)
        # 内部入口真实校验签名/目的服/原配置；公共/sao注册没有内部信任能力。
        status,reply=self.internal(2,payload,signature='0'*64);self.assertNotEqual(reply['code'],0)
        changed=json.loads(payload);changed['destinationServerId']=1;raw=json.dumps(changed,separators=(',',':')).encode();status,reply=self.internal(2,raw,key);self.assertNotEqual(reply['code'],0)
        changed=json.loads(payload);changed['revision']=100;changed['configurationHash']='0'*64;changed['deliveryId']=hashlib.sha256(('p14/9/1/'+str(pid)+'/100/2').encode()).hexdigest();raw=json.dumps(changed,separators=(',',':')).encode();status,reply=self.internal(2,raw,key);self.assertNotEqual(reply['code'],0)
        # 原服2真实进程离线，原服1两次上传仅增加持久outbox，无奖扣费或虚构败北。
        old_ports=self.ports[2];self.stop(2)
        for stamp in [20,21]:self.call(1,'CSKFEnterFrightGame',{},logins[1],stamp,'SCKFEnterFrightGame')
        self.wait_sql(1,'SELECT COUNT(*) FROM p14_deliveries WHERE confirmed=0;','2');self.wait_sql(1,'SELECT COUNT(*) FROM p14_deliveries WHERE confirmed=0 AND attempt_count>0;','2')
        self.stop(1);self.start(1);logins[1]=self.login(1,uids[1]);self.assertEqual(self.sql(1,'SELECT COUNT(*) FROM p14_deliveries WHERE confirmed=0;').strip(),'2')
        # 重新绑定真实目的地址，原服2同库同端口启动后恢复到来源修订3。
        self.start(2,old_ports);logins[2]=self.login(2,uids[2]);self.sql(2,'UPDATE p14_trusted_peers SET endpoint="http://127.0.0.1:'+str(self.ports[1][0])+'" WHERE server_id=1;')
        self.wait_sql(1,'SELECT COUNT(*) FROM p14_deliveries WHERE confirmed=0;','0');self.wait_sql(2,'SELECT revision FROM p14_members WHERE season_index=9 AND server_id=1 AND player_id='+str(pid)+';','3')
        status,reply=self.internal(2,payload,key);self.assertEqual((status,reply['code'],reply['data']['confirmed']),(200,0,True));self.assertEqual(self.sql(2,'SELECT revision FROM p14_members WHERE season_index=9 AND server_id=1 AND player_id='+str(pid)+';').strip(),'3')
        self.call(1,'CSKFEnterFrightGame',{},logins[1],21,'SCKFEnterFrightGame');self.assertEqual(self.sql(1,'SELECT COUNT(*) FROM p14_deliveries;').strip(),'3')
        for zone in [1,2]:
            self.assertEqual(self.sql(zone,"SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.realms[zone]+"' AND TABLE_NAME LIKE 'p14_%';").strip(),'InnoDB')
            self.assertEqual(self.sql(zone,'SELECT COUNT(*) FROM p14_receipts WHERE operation="CSKFGuess";').strip(),'0')
        op,_=self.send(1,'CSKFGuess',{},logins[1],30);self.assertEqual(op,10001)
        # 原赛季截止后新上传拒绝，之前精确原回执仍能恢复。
        self.sql(1,'UPDATE p14_season_policies SET register_end='+str(int(time.time()*1000)-1)+';');op,_=self.send(1,'CSKFEnterFrightGame',{},logins[1],31);self.assertEqual(op,10001)
        self.call(1,'CSKFEnterFrightGame',{},logins[1],21,'SCKFEnterFrightGame')

if __name__=='__main__':unittest.main()
