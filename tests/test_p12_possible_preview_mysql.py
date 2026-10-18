"""Frozen P12 possible-ID preview: original signed-Num frame/current instance/cold/no resources; real InnoDB."""
import concurrent.futures,fcntl,hashlib,json,os,re,secrets,selectors,subprocess,time,types,unittest
from pathlib import Path
import test_p12_mysql as p12
from server.codec import BY_NAME
ROOT=Path(__file__).resolve().parents[1]
@unittest.skipUnless(os.environ.get('HYJS_P12_POSSIBLE_PREVIEW_MYSQL_TEST')=='1','explicit isolated worldboss preview')
class P12PossiblePreviewMySQLTests(unittest.TestCase):
    send=p12.P12MySQLTests.send;login=p12.P12MySQLTests.login;stamped=p12.P12MySQLTests.stamped;call=p12.P12MySQLTests.call;profile=p12.P12MySQLTests.profile;recv=p12.P12MySQLTests.recv;tcp_send=p12.P12MySQLTests.tcp_send;connect=p12.P12MySQLTests.connect;stop=p12.P12MySQLTests.stop;setUp=p12.P12MySQLTests.setUp
    root_sql=classmethod(p12.P12MySQLTests.root_sql.__func__);sql=p12.P12MySQLTests.sql
    @classmethod
    def setUpClass(cls):
        cls.source=Path(os.environ['HYJS_P12_PREVIEW_FIXED_SOURCE']);cls.prebuilt=Path(os.environ['HYJS_P12_PREVIEW_PREBUILT']);assert cls.source.name.startswith('hyjs-p12-possible-preview-') and cls.prebuilt.name.startswith('hyjs-p12-preview-prebuilt-')
        cls.meta=json.loads((cls.prebuilt/'manifest.json').read_text())
        for path,sha in cls.meta['files'].items():assert hashlib.sha256((cls.source/path).read_bytes()).hexdigest()==sha,path
        cls.binary=str(cls.prebuilt/'hyjs-native');assert hashlib.sha256(Path(cls.binary).read_bytes()).hexdigest()==cls.meta['nativeSHA256']
        cls.lock=open('/tmp/hyjs-isolated-mysql.lock','a+');fcntl.flock(cls.lock,fcntl.LOCK_EX);cls.addClassCleanup(lambda:(fcntl.flock(cls.lock,fcntl.LOCK_UN),cls.lock.close()))
        cls.database='hyjs_test_p12_possible_'+secrets.token_hex(5);cls.test_user='p12_possible_'+secrets.token_hex(5);cls.test_password=secrets.token_hex(24)
        cls.addClassCleanup(lambda:cls.root_sql('DROP DATABASE IF EXISTS `'+cls.database+'`;DROP USER IF EXISTS \''+cls.test_user+'\'@\'%\';'))
        cls.root_sql('CREATE DATABASE `'+cls.database+'` CHARACTER SET utf8mb4;CREATE USER \''+cls.test_user+'\'@\'%\' IDENTIFIED BY \''+cls.test_password+'\';GRANT ALL ON `'+cls.database+'`.* TO \''+cls.test_user+'\'@\'%\';')
        cls.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER=cls.test_user,HYJS_MYSQL_PASSWORD=cls.test_password,HYJS_MYSQL_DATABASE=cls.database,HYJS_P12_MYSQL_HOST='127.0.0.1:33068',HYJS_P12_MYSQL_USER=cls.test_user,HYJS_P12_MYSQL_PASSWORD=cls.test_password,HYJS_P12_MYSQL_DATABASE=cls.database)
    def start(self):
        self.process=subprocess.Popen(['rtk','proxy',self.binary,'--http-port','0','--tcp-port','0','--tables',str(self.source/'analysis/tables')],cwd=self.source/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ);self.assertTrue(ready.select(90));line=self.process.stdout.readline()
        match=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line);self.assertIsNotNone(match,line);self.service=types.SimpleNamespace(http_port=int(match[1]),tcp_port=int(match[2]))
    def test_1_innodb_race_all_fifty_and_legacy_source_freeze(self):
        result=subprocess.run(['rtk','proxy',str(self.prebuilt/'service-race.test'),'-test.run=^TestP12WorldBoss','-test.count=1','-test.timeout=180s'],cwd=self.source/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=200)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'));print(result.stdout.strip())
        engines=self.root_sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p12_world_boss_%';").strip();self.assertEqual(engines,'InnoDB')
    def test_2_original_http_tcp_level50_id_only_cold_and_no_resource(self):
        began=time.monotonic();self.start();uids=['possible-boss-'+secrets.token_hex(4) for _ in range(2)];players=[self.login(x) for x in uids]
        for login,name in zip(players,['原可能ID甲','原可能ID乙']):self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        before=[self.profile(x) for x in players];self.sql('UPDATE p10_scene_policies SET enabled=0;UPDATE p12_world_boss_policies SET enabled=0,level=50,position_id=1;')
        prior=self.sql('SELECT COUNT(*) FROM p12_world_boss_cycles;')
        op,_=self.stamped('CSWorldBossReward',{},players[0],101);self.assertEqual(op,10001);self.assertEqual(self.sql('SELECT COUNT(*) FROM p12_world_boss_cycles;'),prior)
        cfg=json.loads(self.sql('SELECT definition_json FROM p12_world_boss_policies WHERE id=1;').strip());self.assertEqual(cfg['WeightedRewardPreviewSource'],'local-original-worldboss-possible-id-preview-v1')
        self.sql('UPDATE p12_world_boss_policies SET enabled=1;')
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:previews=list(pool.map(lambda i:self.call('CSWorldBossReward',{},players[i%2],102,'SCWorldBossReward'),range(8)))
        preview=previews[0];self.assertTrue(all(x==preview for x in previews));self.assertEqual([x.type for x in preview.rewardInfos],list(range(6)))
        rows={int(r['type']):r['reward'] for r in cfg['Tables']['WorldBossReward'] if r['level']=='50'}
        for tier in preview.rewardInfos:
            actual=[(x.id,x.num) for x in tier.rewards]
            if tier.type<4:expected=[(x.split(',')[0],int(x.split(',')[1])) for x in rows[tier.type].split(';')]
            else:expected=[(x.split(':')[0],-1) for x in rows[tier.type].split('|')]
            self.assertEqual(actual,expected)
        info=self.call('CSWorldBossInfo',{},players[0],103,'SCWorldBossInfo').worldBossShowInfo;self.assertEqual(info.bossLevel,50)
        snapshot=self.sql('SELECT CONCAT(id,"|",window_key,"|",definition_hash,"|",level) FROM p12_world_boss_cycles WHERE id="'+info.bossuuid+'";')
        sock=self.connect(players[0]);self.tcp_send(sock,'CSWorldBossReward',{},players[0]);op,tcp=self.recv(sock);self.assertEqual(op,BY_NAME['SCWorldBossReward']);self.assertEqual(tcp,preview);sock.close()
        # 只改未来运营定义，已形成的当前实例沿真实frozen source和原50级表，未改奖励表/角色/随机结果。
        cfg['WeightedRewardPreviewSource']='';edited=json.dumps(cfg,separators=(',',':'));self.sql("UPDATE p12_world_boss_policies SET level=30,definition_json=CONVERT(0x"+edited.encode().hex()+" USING utf8mb4),definition_hash='"+hashlib.sha256(edited.encode()).hexdigest()+"' WHERE id=1;")
        self.stop();self.start();self.assertEqual(self.sql('SELECT CONCAT(id,"|",window_key,"|",definition_hash,"|",level) FROM p12_world_boss_cycles WHERE id="'+info.bossuuid+'";'),snapshot)
        self.assertEqual(self.call('CSWorldBossReward',{},players[1],104,'SCWorldBossReward'),preview)
        for name,fields in [('CSWorldBossFight',{'bossId':info.bossuuid}),('CSWorldBossPropPurchase',{'propId':'wordboss_1'})]:op,_=self.stamped(name,fields,players[0],105);self.assertEqual(op,10001)
        rank=self.call('CSWorldBossRank',{},players[0],106,'SCWorldBossRank');self.assertFalse(rank.damageInfos);self.assertFalse(rank.HasField('lastDamageInfo'));self.assertFalse(rank.HasField('myDamageInfo'))
        self.sql('UPDATE p12_world_boss_policies SET enabled=0;');op,_=self.stamped('CSWorldBossReward',{},players[0],107);self.assertEqual(op,10001)
        self.assertEqual([self.profile(x) for x in players],before)
        report={'status':'PASS_ORIGINAL_HTTP_TCP_INNODB_POSSIBLE_ID_PREVIEW','date':'2026-10-15','scope':'original all50×6 tiers in domain/race; native actual operator selected level50 current calendar instance, eight concurrent2actors one immutable frame/UUID, signednum-1 on weighted type4/5; exact fixed quantity type0..3; no random draw/probability/guaranteed quantity','legacy':'domain missing/unknown source closed high30+; sameinstance source/level/table frozen on current policy hot change/cold; global gate remains defaultoff and disables old reads','resource':'complete actor profiles/inventory/currencies unchanged; actual fight/prop purchase refuse; no participant/damage/last-hit/winner/award','originalPrivateProjection':'unknown; named local ID-only view uses original CommonIcon -1 quantity hidden, not original private lottery or settlement','seconds':round(time.monotonic()-began,3),'binarySHA256':self.meta['nativeSHA256'],'phoneUI':False,'productionModified':False,'cleanup':'whole-life filelock/schema/user/grants cleanup; outside-lock fixedsource/prebuild then native only additional isolated schema name guard'}
        (ROOT/'analysis/p12/possible-id-preview-mysql-result.json').write_text(json.dumps(report,indent=2)+'\n')
if __name__=='__main__':unittest.main()
