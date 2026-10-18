"""Actual PCM bytes via authenticated HTTP and original isAudio2 HTTP/TCP; isolated SQL only."""
import base64,concurrent.futures,fcntl,hashlib,http.client,json,os,re,secrets,selectors,shutil,subprocess,time,types,unittest,struct
from pathlib import Path
import test_p10_mysql as p10
from server.codec import BY_NAME
ROOT=Path(__file__).resolve().parents[1]
@unittest.skipUnless(os.environ.get('HYJS_P10_VOICE_MYSQL_TEST')=='1','explicit Voice isolated MySQL')
class P10VoiceMySQLTests(unittest.TestCase):
    send=p10.P10MySQLTests.send;login=p10.P10MySQLTests.login;stamped=p10.P10MySQLTests.stamped;root_sql=classmethod(p10.P10MySQLTests.root_sql.__func__);sql=p10.P10MySQLTests.sql
    setUp=p10.P10MySQLTests.setUp;stop=p10.P10MySQLTests.stop;call=p10.P10MySQLTests.call;profile=p10.P10MySQLTests.profile;recv=p10.P10MySQLTests.recv;tcp_send=p10.P10MySQLTests.tcp_send;connect=p10.P10MySQLTests.connect
    @classmethod
    def setUpClass(cls):
        cls.source=Path(os.environ['HYJS_P10_VOICE_FIXED_SOURCE']);cls.prebuilt=Path(os.environ['HYJS_P10_VOICE_PREBUILT']);assert cls.source.name.startswith('hyjs-p13-field-voice-') and cls.prebuilt.name.startswith('hyjs-p10-voice-prebuilt-')
        manifest=json.loads((cls.prebuilt/'manifest.json').read_text())
        for name,sha in manifest['files'].items():assert hashlib.sha256((cls.source/name).read_bytes()).hexdigest()==sha,name
        cls.binary=str(cls.prebuilt/'hyjs-native');assert hashlib.sha256(Path(cls.binary).read_bytes()).hexdigest()==manifest['nativeSHA256']
        cls.lock=open('/tmp/hyjs-isolated-mysql.lock','a+');fcntl.flock(cls.lock,fcntl.LOCK_EX);cls.addClassCleanup(lambda:(fcntl.flock(cls.lock,fcntl.LOCK_UN),cls.lock.close()))
        cls.database='hyjs_test_p10_voice_'+secrets.token_hex(5);cls.war_database='hyjs_test_p13_voice_'+secrets.token_hex(5);cls.test_user='p10_voice_'+secrets.token_hex(5);cls.test_password=secrets.token_hex(24)
        cls.addClassCleanup(lambda:cls.root_sql('DROP DATABASE IF EXISTS `'+cls.database+'`; DROP DATABASE IF EXISTS `'+cls.war_database+'`; DROP USER IF EXISTS \''+cls.test_user+'\'@\'%\';'))
        cls.root_sql('CREATE DATABASE `'+cls.database+'` CHARACTER SET utf8mb4;CREATE DATABASE `'+cls.war_database+'` CHARACTER SET utf8mb4;CREATE USER \''+cls.test_user+'\'@\'%\' IDENTIFIED BY \''+cls.test_password+'\';GRANT ALL ON `'+cls.database+'`.* TO \''+cls.test_user+'\'@\'%\';GRANT ALL ON `'+cls.war_database+'`.* TO \''+cls.test_user+'\'@\'%\';')
        cls.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER=cls.test_user,HYJS_MYSQL_PASSWORD=cls.test_password,HYJS_MYSQL_DATABASE=cls.database)
        for prefix,name in [('HYJS_P10',cls.database),('HYJS_P13',cls.war_database)]:
            for key,value in [('MYSQL_HOST','127.0.0.1:33068'),('MYSQL_USER',cls.test_user),('MYSQL_PASSWORD',cls.test_password),('MYSQL_DATABASE',name)]:cls.env[prefix+'_'+key]=value
        cls.fixed_manifest=manifest
    def start(self):
        self.process=subprocess.Popen(['rtk','proxy',self.binary,'--http-port','0','--tcp-port','0','--tables',str(self.source/'analysis/tables')],cwd=self.source/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ);self.assertTrue(ready.select(90),'Voice cold startup timeout');line=self.process.stdout.readline()
        match=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line);self.assertIsNotNone(match,line);self.service=types.SimpleNamespace(http_port=int(match[1]),tcp_port=int(match[2]))
    @staticmethod
    def tone(milliseconds):
        samples=milliseconds*16;raw=bytearray(struct.pack('<4sI4s4sIHHIIHH4sI',b'RIFF',36+samples*2,b'WAVE',b'fmt ',16,1,1,16000,32000,2,16,b'data',samples*2))
        for i in range(samples):raw+=struct.pack('<h',4000 if i%40<20 else -4000)
        return bytes(raw)
    def voice(self,path,login,body=None,extra=None,method='POST',expect_success=True):
        headers={'X-Hyjs-Player-ID':str(login.playerId),'X-Hyjs-Session-Key':base64.b64encode(bytes(login.sessionKey)).decode()};headers.update(extra or {})
        connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=10)
        try:
            connection.request(method,path,body=body,headers=headers);response=connection.getresponse();raw=response.read();ctype=response.getheader('Content-Type','');cache=response.getheader('Cache-Control','')
            if path=='/voice/download' and ctype.startswith('audio/wav'):
                self.assertTrue(expect_success);self.assertEqual(cache,'no-store');return raw
            data=json.loads(raw)
            if expect_success:self.assertEqual(data['code'],0,str(data));return data['data']
            self.assertNotEqual(data['code'],0,'unexpected authorized voice');return data
        finally:connection.close()
    def upload(self,login,nonce,raw,kind=3,scope='person',expect_success=True):
        return self.voice('/voice/upload',login,raw,{'Content-Type':'audio/wav','X-Hyjs-Chat-Type':str(kind),'X-Hyjs-Request-ID':nonce,'X-Hyjs-Voice-Scope':scope},expect_success=expect_success)
    def download(self,login,name,ok=True):return self.voice('/voice/download',login,b'',{'X-Hyjs-File-Name':name},expect_success=ok)
    def delete(self,login,name,ok=True):return self.voice('/voice/delete',login,b'',{'X-Hyjs-File-Name':name},expect_success=ok)
    def test_1_actual_innodb_race_pcm_and_all_channel_permissions(self):
        result=subprocess.run(['rtk','proxy',str(self.prebuilt/'service-race.test'),'-test.run=^TestP10Voice','-test.count=1','-test.timeout=180s'],cwd=self.source/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=200)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'));print(result.stdout.strip())
        tables=['p10_voice_policies','p10_voice_assets','p10_voice_messages']
        for name in [self.database,self.war_database]:
            rows=self.root_sql('SELECT TABLE_NAME,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=\''+name+'\' AND TABLE_NAME IN ('+','.join('\''+x+'\'' for x in tables)+');').splitlines()
            self.assertEqual(len(rows),3);self.assertTrue(all(x.endswith('\tInnoDB') for x in rows))
    def test_2_actual_http_tcp_cold_delete_blacklist_and_session(self):
        began=time.monotonic();self.start();uids=['voice-native-'+secrets.token_hex(4) for _ in range(3)];a,b,c=[self.login(uid) for uid in uids]
        for login,name in [(a,'真实音频甲'),(b,'真实音频乙'),(c,'真实音频丙')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        ids=','.join(str(x.playerId) for x in [a,b,c]);self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",20) WHERE id IN ('+ids+');UPDATE p10_social_policies SET enabled=1;UPDATE p10_scene_policies SET enabled=0;UPDATE p10_channel_policies SET world_enabled=0,field_enabled=0;UPDATE p10_voice_policies SET enabled=0;')
        original_profiles=[self.profile(x) for x in [a,b,c]]
        config=self.voice('/voice/config?chatType=3',a,method='GET');self.assertFalse(config['enabled']);self.assertEqual(config['adapter'],0)
        kv=self.call('CSKVInfo',{},a,11,'SCKVInfo');flags={x.key:x.value for x in kv.entry};self.assertEqual((flags['audiosystem'],flags['audioautoget']),('off','off'))
        raw=self.tone(2100);self.upload(a,'closed',raw,expect_success=False)
        self.sql('UPDATE p10_voice_policies SET enabled=1,retention_millis=60000,min_duration_millis=100,source="isolated-real-synthetic-pcm-transfer-v1";')
        kv=self.call('CSKVInfo',{},a,12,'SCKVInfo');flags={x.key:x.value for x in kv.entry};self.assertEqual((flags['audiosystem'],flags['audioautoget']),('on','off'))
        config=self.voice('/voice/config?chatType=3',a,method='GET');self.assertTrue(config['canRecord']);self.assertEqual((config['maxSeconds'],config['recordingGapSeconds'],config['scope']),(30,1,'person'))
        op,error=self.stamped('CSAudioChatCheck',{'chatType':3},a,2);self.assertEqual(op,10001)
        for invalid in [b'',b'fake audio',self.tone(30001)]:self.upload(a,'bad-'+secrets.token_hex(4),invalid,expect_success=False)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:uploads=list(pool.map(lambda _:self.upload(a,'real-nonce',raw),range(8)))
        self.assertTrue(all(u==uploads[0] for u in uploads));upload=uploads[0];name=upload['fileName'];self.assertRegex(name,r'^hyjsvoice_[0-9a-f]{32}\.wav$');self.assertEqual((upload['fileSize'],upload['audioTime'],upload['durationMillis'],upload['sha256']),(len(raw),2,2100,hashlib.sha256(raw).hexdigest()))
        self.upload(a,'REAL-NONCE',raw,expect_success=False)
        self.assertEqual(self.download(a,name),raw);self.download(b,name,False);self.download(c,name,False)
        check=self.call('CSAudioChatCheck',{'chatType':3},a,3,'SCAudioChatFileName');self.assertEqual(check.fileName,name)
        sender=self.connect(a);receiver=self.connect(b)
        fields={'chatType':3,'playerId':b.playerId,'isAudio':2,'downLoadUrl':name,'audioTime':2,'fileSize':len(raw),'content':''}
        self.tcp_send(sender,'CSChatTcp',fields,a);op,ack=self.recv(sender);self.assertEqual(op,BY_NAME['SCChatTcp']);self.assertEqual((ack.isAudio,ack.downLoadUrl,ack.audioTime,ack.play.playerId),(2,name,2,a.playerId));self.assertEqual(self.recv(receiver)[1],ack)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:results=list(pool.map(lambda _:self.call('CSChatTcp',fields,a,4,'SCChatTcp'),range(8)))
        self.assertTrue(all(x==ack for x in results));receiver.settimeout(.2)
        with self.assertRaises(TimeoutError):receiver.recv(1)
        receiver.settimeout(3);self.assertEqual(self.download(b,name),raw);self.download(c,name,False);self.delete(b,name,False)
        history=self.call('CSChatCache',{'chatType':3},b,5,'SCChatCache');self.assertEqual([x.audioDownloadUrl for x in history.chatList if x.isAudio==2],[name])
        for key,value in [('fileSize',len(raw)-1),('audioTime',3),('playerId',c.playerId),('isAudio',1),('downLoadUrl','http://untrusted.invalid/a.wav')]:
            changed=dict(fields);changed[key]=value;op,_=self.stamped('CSChatTcp',changed,a,6);self.assertEqual(op,10001,key)
        self.call('CSSendToBlackList',{'friendId':a.playerId},b,7,'SCSendToBlackList');self.download(b,name,False)
        history=self.call('CSChatCache',{'chatType':3},b,8,'SCChatCache');self.assertFalse(any(x.isAudio==2 for x in history.chatList))
        self.call('CSDeleteFromBlackList',{'friendId':a.playerId},b,9,'SCDeleteFromBlackList');self.assertEqual(self.download(b,name),raw)
        new_b=self.login(uids[1]);self.download(b,name,False);self.assertEqual(self.download(new_b,name),raw)
        sender.close();receiver.close();self.stop();self.start();self.assertEqual(self.download(new_b,name),raw)
        history=self.call('CSChatCache',{'chatType':3},new_b,10,'SCChatCache');self.assertEqual([x.audioDownloadUrl for x in history.chatList if x.isAudio==2],[name])
        self.assertTrue(self.delete(a,name)['deleted']);self.assertTrue(self.delete(a,name)['deleted']);self.download(new_b,name,False)
        self.assertEqual(self.sql('SELECT state,LENGTH(bytes) FROM p10_voice_assets WHERE file_name="'+name+'";').strip(),'3\t0')
        self.delete(a,'hyjsvoice_'+'0'*32+'.wav',False)
        self.assertEqual([self.profile(x) for x in [a,new_b,c]],original_profiles,'voice changed inventory/currency/player state')
        report={'status':'PASS_REAL_HTTP_TCP_INNODB_VOICE_BYTES_ACL','date':'2026-10-15','source':'actual synthetic PCM16 mono16k 2100ms square-wave test fixture; human microphone/playback remains pending root device','originalProtocol':'isAudio2/downLoadUrl actual stored opaque ID/audioTime floor/FileSize actual, CSAudioChatCheck no object refusal','eightUploadAndSend':'one actual blob/message/recipient push, exact repeats','cold':'same authenticated stored PCM SHA/bytes and original private history','permissions':'unpublished/foreign/blacklist/current session rotation/corrupt fields/foreign delete/missing object refusal/case-changed nonce refusal','domainRace':'all5 actual InnoDB/race including actual guild withdrawal and original matched current war roster/end; shared default-off kept','originalLoginAudioUI':'CSKVInfo audiosystem actual policy off→on, audioautoget localoff; no account blob edits', 'profilePreserved':True,'productionModified':False,'phoneMicrophoneVerified':False,'seconds':round(time.monotonic()-began,3),'sharedManifestSHA256':hashlib.sha256((ROOT/'analysis/p10/voice-shared-v3-source-20261015/manifest.json').read_bytes()).hexdigest(),'binarySHA256':self.fixed_manifest['nativeSHA256'],'cleanup':'whole-life SQL lock, isolated2schemas/user/grants removed by class cleanup'}
        (ROOT/'analysis/p10/voice-mysql-result.json').write_text(json.dumps(report,indent=2)+'\n')
if __name__=='__main__':unittest.main()
