"""Single Voice v4 original guild2 target0/private3 positive/zero/self native boundary regression.

Guild existence/level/initial creation coins are explicit isolated domain fixture prerequisites;
voice never grants or changes resources. No unrelated guild test suites or device operations.
"""
import hashlib,json,os,subprocess,time,unittest
from pathlib import Path
from test_p10_voice_mysql import P10VoiceMySQLTests,ROOT
from server.codec import BY_NAME
class P10VoiceChannelBoundaryMySQLTests(P10VoiceMySQLTests):
    def sql(self,text):return self.root_sql('USE `'+self.war_database+'`; '+text)
    def test_v4_guild2_target_zero_and_private3_positive_zero_self(self):
        began=time.monotonic()
        result=subprocess.run(['rtk','proxy',str(self.prebuilt/'service-race.test'),'-test.run=^TestP10VoiceGuildWithdrawalCurrentSourceAndFrozenBytes$','-test.count=1','-test.timeout=120s'],cwd=self.source/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=140)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'))
        self.env=dict(self.env,HYJS_MYSQL_DATABASE=self.war_database);self.start()
        members=self.sql('SELECT p.uid,p.id FROM profiles p JOIN p13_guild_members m ON m.player_id=p.id WHERE m.guild_id<>"" ORDER BY m.role DESC,p.id;').strip().splitlines();self.assertGreaterEqual(len(members),2)
        accounts=[]
        for member in members[:2]:
            uid,ident=member.split('\t');uid='voice-v4-member-'+ident
            # 原登录账号上限64；仅调整专库旧Go fixture过长测试名地址，不改密码/会话/资源。
            self.sql('UPDATE profiles SET uid="'+uid+'" WHERE id='+ident+';')
            op,login=self.send('CSUserLogin',{'platformUid':uid,'authCode':'isolated-local-pass','clientTime':1700000000000});self.assertEqual(op,BY_NAME['SCUserLogin']);self.assertEqual(login.playerId,int(ident));accounts.append(login)
        a,b=accounts;self.sql('UPDATE p10_social_policies SET enabled=1;UPDATE p10_scene_policies SET enabled=0;')
        before=[self.profile(x) for x in accounts];sender=self.connect(a);receiver=self.connect(b);raw=self.tone(2100)
        config=self.voice('/voice/config?chatType=2',a,method='GET');self.assertTrue(config['scope'].startswith('guild/'))
        guild=self.upload(a,'actual-guild-v4',raw,kind=2,scope=config['scope'])
        body={'chatType':2,'playerId':0,'isAudio':2,'content':'','downLoadUrl':guild['fileName'],'audioTime':2,'fileSize':len(raw)}
        self.tcp_send(sender,'CSChatTcp',body,a);op,frame=self.recv(sender);self.assertEqual(op,BY_NAME['SCChatTcp']);self.assertEqual((frame.chatType,frame.isAudio,frame.downLoadUrl),(2,2,guild['fileName']));self.assertEqual(self.recv(receiver)[1],frame)
        self.assertEqual(self.download(b,guild['fileName']),raw)
        private=self.upload(b,'actual-private-v4',raw)
        body={'chatType':3,'playerId':a.playerId,'isAudio':2,'content':'','downLoadUrl':private['fileName'],'audioTime':2,'fileSize':len(raw)}
        for invalid_target in [0,b.playerId]:
            invalid=dict(body,playerId=invalid_target);op,_=self.stamped('CSChatTcp',invalid,b,301);self.assertEqual(op,10001)
        self.tcp_send(receiver,'CSChatTcp',body,b);op,frame=self.recv(receiver);self.assertEqual(op,BY_NAME['SCChatTcp']);self.assertEqual((frame.chatType,frame.isAudio,frame.downLoadUrl),(3,2,private['fileName']));self.assertEqual(self.recv(sender)[1],frame)
        self.assertEqual(self.download(a,private['fileName']),raw);self.assertEqual([self.profile(x) for x in accounts],before)
        report={'status':'PASS_ACTUAL_ORIGINAL_GUILD2_ZERO_AND_PRIVATE3_BOUNDARIES','guild2':'actual guild/TCP/current member upload→target0 accepted→exact native recipient frame/real PCM bytes','private3':'target0/self refused before publish, actual nonself accepted/real PCM bytes','prerequisites':'one existing TestP10VoiceGuildWithdrawal producer fixture only, finite isolated level30/coins20000000/diamond1000/item089x50 original guild fixture; only finite username shortening to satisfy original64-char account contract; no claim natural currency source; resources/profile untouched by native voice','seconds':round(time.monotonic()-began,3),'sharedManifestSHA256':hashlib.sha256((ROOT/'analysis/p10/voice-shared-v4-source-20261015/manifest.json').read_bytes()).hexdigest(),'binarySHA256':self.fixed_manifest['nativeSHA256'],'phoneUI':False,'productionModified':False,'cleanup':'whole-life2schemas/user/grants lock; only native main exclusive P13 Voice isolation-name guard expanded in disposable test copy'}
        (ROOT/'analysis/p10/voice-channel-v4-mysql-result.json').write_text(json.dumps(report,indent=2)+'\n')
if __name__=='__main__':unittest.main()
