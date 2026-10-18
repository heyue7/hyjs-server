"""P12原世界首领元数据/真实UUID/日历恢复，独占InnoDB测试库结束清理。"""
import concurrent.futures
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import unittest
from server.codec import BY_NAME
import test_p10_mysql as transport
ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P12_MYSQL_TEST')=='1','显式启用P12专属MySQL')
class P12MySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    stamped=transport.P10MySQLTests.stamped
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
        cls.database='hyjs_test_p12_'+secrets.token_hex(5);cls.test_user='p12_'+secrets.token_hex(5);cls.test_password=secrets.token_hex(24)
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-p12-isolated-');cls.addClassCleanup(cls.temp.cleanup);cls.binary=str(Path(cls.temp.name)/'hyjs-p12')
        cls.root_sql("CREATE DATABASE `"+cls.database+"` CHARACTER SET utf8mb4; CREATE USER '"+cls.test_user+"'@'%' IDENTIFIED BY '"+cls.test_password+"'; GRANT ALL ON `"+cls.database+"`.* TO '"+cls.test_user+"'@'%';")
        cls.addClassCleanup(lambda:cls.root_sql("DROP DATABASE IF EXISTS `"+cls.database+"`; DROP USER IF EXISTS '"+cls.test_user+"'@'%';"))
        cls.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER=cls.test_user,HYJS_MYSQL_PASSWORD=cls.test_password,HYJS_MYSQL_DATABASE=cls.database,HYJS_P12_MYSQL_HOST='127.0.0.1:33068',HYJS_P12_MYSQL_USER=cls.test_user,HYJS_P12_MYSQL_PASSWORD=cls.test_password,HYJS_P12_MYSQL_DATABASE=cls.database)
        if 'InitializeP12WorldBoss(ctx)' not in (ROOT/'hyjs-server/main.go').read_text():raise AssertionError('P12共享接点尚未集成，需要测试实际源码')
        r=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=180)
        if r.returncode:raise AssertionError(r.stdout+r.stderr)
    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p12_') or not cls.test_user.startswith('p12_'):raise AssertionError('P12 isolation guard')
        r=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if r.returncode:raise AssertionError(r.stderr.replace(cls.test_password,'[redacted]'))
        return r.stdout
    def sql(self,text):return self.root_sql('USE `'+self.database+'`; '+text)
    def test_1_actual_go_mysql_windows_concurrency_frozen_snapshots_and_recovery(self):
        args=['rtk','proxy','go','test','./service','-run','^TestP12WorldBoss','-count=1','-timeout=120s']
        if os.environ.get('HYJS_P12_GO_RACE')=='1':args.insert(4,'-race')
        r=subprocess.run(args,cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
    def test_2_original_http_tcp_real_metadata_immutable_cycle_restart_and_no_fake_damage(self):
        self.start();uids=['p12-world-native-'+secrets.token_hex(4) for _ in range(2)];players=[self.login(uid) for uid in uids]
        for login,name in zip(players,['原首领查询甲','原首领查询乙']):self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql('UPDATE p12_world_boss_policies SET enabled=0; UPDATE p10_scene_policies SET enabled=0;')
        before=[self.profile(x) for x in players];old_count=self.sql('SELECT COUNT(*) FROM p12_world_boss_cycles;')
        op,r=self.stamped('CSWorldBossInfo',{},players[0],101);self.assertEqual(op,10001,str(r));self.assertEqual(self.sql('SELECT COUNT(*) FROM p12_world_boss_cycles;'),old_count)
        self.sql('UPDATE p12_world_boss_policies SET enabled=1,level=1,position_id=1;')
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:infos=list(pool.map(lambda index:self.call('CSWorldBossInfo',{},players[index%2],102,'SCWorldBossInfo'),range(8)))
        info=infos[0].worldBossShowInfo;self.assertTrue(all(r.worldBossShowInfo==info for r in infos));self.assertTrue(info.bossuuid);self.assertEqual((info.bossLevel,info.bossPicPath,info.scene),(1,'monster_041','field_002'));self.assertEqual(len(info.props),4);self.assertTrue(all(x.cost=='diamond,150' for x in info.props));self.assertEqual(list(info.propPurchased),[])
        snapshot=self.sql('SELECT CONCAT(id,"|",window_key,"|",starts_at,"|",ends_at,"|",definition_hash,"|",level,"|",position_id) FROM p12_world_boss_cycles WHERE id="'+info.bossuuid+'";')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p12_world_boss_cycles WHERE window_key=(SELECT window_key FROM (SELECT window_key FROM p12_world_boss_cycles WHERE id="'+info.bossuuid+'") w);').strip(),'1')
        rank=self.call('CSWorldBossRank',{},players[0],103,'SCWorldBossRank');self.assertEqual(rank.bossHpPercent,1);self.assertEqual(len(rank.damageInfos),0);self.assertFalse(rank.HasField('myDamageInfo'));self.assertFalse(rank.HasField('lastDamageInfo'))
        for name,fields in [('CSWorldBossPropPurchase',{'propId':'wordboss_1'}),('CSWorldBossPropPurchase',{'propId':'invalid'}),('CSWorldBossFight',{'bossId':info.bossuuid}),('CSWorldBossFight',{'bossId':''})]:op,r=self.stamped(name,fields,players[0],104);self.assertEqual(op,10001,str(r))
        preview=self.call('CSWorldBossReward',{},players[0],106,'SCWorldBossReward')
        self.assertEqual([x.type for x in preview.rewardInfos],list(range(6)))
        self.assertEqual([(x.id,x.num) for x in preview.rewardInfos[0].rewards],[('coin',10000)])
        definition=json.loads(self.sql('SELECT definition_json FROM p12_world_boss_cycles WHERE id=\"'+info.bossuuid+'\";').strip())
        rows={int(row['type']):row['reward'] for row in definition['Tables']['WorldBossReward'] if row['level']=='1'}
        for row in preview.rewardInfos:self.assertEqual([(x.id,x.num) for x in row.rewards],[(part.split(',')[0],int(part.split(',')[1])) for part in rows[row.type].split(';')])
        replay=self.call('CSWorldBossReward',{},players[1],107,'SCWorldBossReward');self.assertEqual(preview,replay)
        sock=self.connect(players[0]);self.tcp_send(sock,'CSWorldBossInfo',{},players[0]);op,tcp=self.recv(sock);self.assertEqual(op,BY_NAME['SCWorldBossInfo']);self.assertEqual(tcp.worldBossShowInfo.bossuuid,info.bossuuid);sock.close();self.assertEqual([self.profile(x) for x in players],before)
        self.sql('UPDATE p12_world_boss_policies SET level=2,position_id=2;');self.stop();self.start();players=[self.login(uid) for uid in uids]
        self.assertEqual(self.sql('SELECT CONCAT(id,"|",window_key,"|",starts_at,"|",ends_at,"|",definition_hash,"|",level,"|",position_id) FROM p12_world_boss_cycles WHERE id="'+info.bossuuid+'";'),snapshot)
        resumed=self.call('CSWorldBossInfo',{},players[0],105,'SCWorldBossInfo').worldBossShowInfo
        if resumed.bossuuid==info.bossuuid:
            self.assertEqual(resumed.bossLevel,1)
            self.assertEqual(self.call('CSWorldBossReward',{},players[0],108,'SCWorldBossReward'),preview)
        self.assertEqual([self.profile(x) for x in players],before)
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p12_world_boss_%';").strip(),'InnoDB')
        print('P12 actual source original HTTP/TCP metadata and unique immutable SQL cycle; complete six-tier reward preview without charge/damage/last-hit/winner/award.')
