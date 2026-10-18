"""P7原目录、权威门禁、冻结计算与原录像帧；仅写入专属schema。"""
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import shutil
import subprocess
import tempfile
import unittest
import types
from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME
from test_server import LiveServerTests
from test_go_formation import GoFormationTests

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P7_MYSQL_TEST')=='1','显式启用P7专属MySQL回归')
class P7MySQLTests(unittest.TestCase):
    send=LiveServerTests.send
    login=LiveServerTests.login
    stamped=GoFormationTests.stamped

    @classmethod
    def setUpClass(cls):
        cls.managed=ManagedMySQL();cls.addClassCleanup(cls.managed.close)
        cls.connection=cls.managed.allocate('p7_legacy')
        cls.database=cls.connection.database;cls.test_user=cls.connection.user;cls.test_password=cls.connection.password
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-p7-isolated-');cls.addClassCleanup(cls.temp.cleanup)
        cls.binary=str(Path(cls.temp.name)/'hyjs-p7')
        cls.env=dict(os.environ,**cls.connection.env())
        cls.env.update(GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB')
        cls.env.update({'HYJS_P7_MYSQL_'+key:cls.env['HYJS_MYSQL_'+key] for key in ['HOST','USER','PASSWORD','DATABASE']})
        # 根代理尚未接共享入口时，仅在临时源码副本应用精确patch。
        build_root=ROOT/'hyjs-server';cls.used_pending_patch='InitializeP7(ctx)' not in (build_root/'main.go').read_text()
        if cls.used_pending_patch:
            build_root=Path(cls.temp.name)/'source';shutil.copytree(ROOT/'hyjs-server',build_root,ignore=shutil.ignore_patterns('logs'))
            applied=subprocess.run(['rtk','proxy','git','apply',str(ROOT/'analysis/p7/rush-integration.patch')],cwd=build_root,capture_output=True,text=True,timeout=30)
            if applied.returncode:raise AssertionError(applied.stdout+applied.stderr)
        cls.used_expedition_patch='InitializeP7Expedition(ctx)' not in (build_root/'main.go').read_text()
        if cls.used_expedition_patch:
            if build_root == ROOT/'hyjs-server':
                build_root=Path(cls.temp.name)/'source';shutil.copytree(ROOT/'hyjs-server',build_root,ignore=shutil.ignore_patterns('logs'))
            applied=subprocess.run(['rtk','proxy','git','apply',str(ROOT/'analysis/p7/expedition-integration.patch')],cwd=build_root,capture_output=True,text=True,timeout=30)
            if applied.returncode:raise AssertionError(applied.stdout+applied.stderr)
        cls.used_awake_patch='InitializeP7Awake(ctx)' not in (build_root/'main.go').read_text()
        if cls.used_awake_patch:
            if build_root == ROOT/'hyjs-server':
                build_root=Path(cls.temp.name)/'source';shutil.copytree(ROOT/'hyjs-server',build_root,ignore=shutil.ignore_patterns('logs'))
            applied=subprocess.run(['rtk','proxy','git','apply',str(ROOT/'analysis/p7/awake-integration.patch')],cwd=build_root,capture_output=True,text=True,timeout=30)
            if applied.returncode:raise AssertionError(applied.stdout+applied.stderr)
        result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=build_root,env=cls.env,capture_output=True,text=True,timeout=120)
        if result.returncode:raise AssertionError(result.stdout+result.stderr)

    @classmethod
    def database_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p7_'):raise AssertionError('P7 managed isolation guard')
        return cls.managed.sql(cls.database,text)

    def setUp(self):self.process=None;self.addCleanup(self.stop)
    def sql(self,text):return self.database_sql(text)
    def start(self):
        self.process=subprocess.Popen([self.binary,'--http-port','0','--tcp-port','0','--tables',str(ROOT/'analysis/tables')],cwd=ROOT/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ);self.assertTrue(ready.select(90),'P7冷启动超时');line=self.process.stdout.readline()
        match=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line);self.assertIsNotNone(match,line)
        self.service=types.SimpleNamespace(http_port=int(match[1]),tcp_port=int(match[2]))
    def stop(self):
        if self.process is not None and self.process.poll() is None:
            self.process.terminate()
            try:self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:self.process.kill();self.process.wait();self.fail('P7服务未响应ctx退出')
        if self.process is not None and self.process.stdout:self.process.stdout.close()
    def call(self,name,fields,login,stamp,expected):
        op,r=self.stamped(name,fields,login,stamp);self.assertEqual(op,BY_NAME[expected],str(r));return r
    def profile(self,login):
        row=self.sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id='+str(login.playerId)+';').strip().split('|');return [json.loads(bytes.fromhex(x)) for x in row]

    def test_1_isolated_transactions_real_profile_compute_and_gate(self):
        result=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP7','-count=1','-timeout=150s'],cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=180)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'));print(result.stdout.strip())

    def test_2_native_rush_catalog_and_atomic_gate(self):
        self.start();login=self.login('p7-network-'+secrets.token_hex(4))
        self.call('CSPlayerCreate',{'playerName':'原试炼目录门禁','heroId':'hero_047'},login,1,'SCPlayerCreate')
        catalog=self.call('CSGetRushEliteBossInfo',{},login,1,'SCGetRushEliteBossInfo')
        self.assertEqual((len(catalog.bossInfoList),catalog.leftTime,catalog.totalTime,catalog.diamondLeftTime),(34,3,3,3));self.assertEqual(catalog.bossInfoList[0].eliteTaskId,'01')
        self.assertEqual(catalog.bossInfoList[0].firstRewardList[0].num,2000);self.assertTrue(all(b.isOpen==0 and b.isSuccess==0 for b in catalog.bossInfoList))
        self.sql('UPDATE p7_rush_policies SET enabled=1; UPDATE p6_combat_rules SET authoritative_enabled=1;')
        before=self.profile(login)
        for name,fields in [('CSRushFightStart',{'rushType':2,'taskId':'01'}),('CSRushEliteBossBuyTime',{}),('CSRushResetChallengeCount',{}),('CSRush3Hit',{}),('CSRushToTop',{})]:
            op,r=self.stamped(name,fields,login,100);self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(login),before)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p7_rush_fights WHERE player_id='+str(login.playerId)+';').strip(),'0')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p7_rush_receipts WHERE player_id='+str(login.playerId)+';').strip(),'0')

    def test_3_original_frozen_playback_reconnect_and_no_star_reward(self):
        self.start();login=self.login('p7-native-diagnostic')
        row=self.sql('SELECT id,replay_id FROM p7_rush_fights WHERE player_id='+str(login.playerId)+' AND diagnostic=1 ORDER BY id LIMIT 1;').strip().split('\t');self.assertEqual(len(row),2)
        first=self.call('CSFightPlayback',{'fightId':row[0],'serverId':1},login,1,'SCFightPlayback')
        self.assertEqual(first.fightid,row[1]);self.assertEqual(first.attacker.playerId,login.playerId);self.assertTrue(first.optRecord.skillOpts)
        before=self.profile(login);op,r=self.stamped('CSRushFightEnd',{'fightId':row[0],'result':1,'optRecord':first.optRecord},login,101)
        self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(login),before)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p7_rush_progress WHERE player_id='+str(login.playerId)+';').strip(),'0')
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p1_event_facts WHERE player_id="+str(login.playerId)+" AND kind='rush_star_total';").strip(),'0')
        self.stop();self.start();login=self.login('p7-native-diagnostic');second=self.call('CSFightPlayback',{'fightId':row[0],'serverId':1},login,1,'SCFightPlayback');self.assertEqual(first,second)
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p7_%';").strip(),'InnoDB')
        print('P7 pending shared patch used:',self.used_pending_patch)

    def test_4_native_expedition_duration_occupancy_claim_and_restart(self):
        self.start();login=self.login('p7-native-expedition')
        self.sql('UPDATE p7_expedition_policies SET enabled=1;')
        inter=self.call('CSInterExpedition',{},login,1,'SCInterExpedition');self.assertEqual((inter.level,inter.exp,inter.freeRefreshTime),('D',0,2))
        stock=self.call('CSGetRefreshedMissions',{},login,1,'SCGetRefreshedMissions');self.assertEqual(len(stock.refreshedMissions),2)
        serial=stock.refreshedMissions[0].missionSerial
        idle=self.call('CSGetExpeditionIdleHeros',{'missionSerial':serial},login,1,'SCGetExpeditionIdleHeros');self.assertEqual(len(idle.canSendGuids),2)
        guid=idle.canSendGuids[0];before=self.profile(login)
        req={'missionSerial':serial,'heroGuids':[guid]}
        first=self.call('CSAcceptMission',req,login,401,'SCAcceptMission');self.assertEqual(first,self.call('CSAcceptMission',req,login,401,'SCAcceptMission'))
        accepted=self.call('CSGetAcceptedMissions',{},login,1,'SCGetAcceptedMissions').acceptedMissions[0]
        hours=(accepted.endTime-accepted.startTime)//3600000;self.assertIn(hours,(4,8,12));self.assertEqual(accepted.state,1)
        self.assertEqual(self.profile(login)[0]['energy'],before[0]['energy']-hours*2)
        held=self.profile(login)
        op,r=self.stamped('CSWarriorSell',{'guid':[guid]},login,403);self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(login),held)
        op,r=self.stamped('CSGetFinalReward',{'missionSerial':serial},login,402);self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(login),held)
        # 有限时钟夹具只移动原起止时刻，保留真实4/8/12小时差；不声称实等时长验收。
        self.sql("UPDATE p7_expedition_missions SET start_at=UNIX_TIMESTAMP(NOW(3))*1000-"+str(hours*3600000+1000)+",end_at=UNIX_TIMESTAMP(NOW(3))*1000-1000 WHERE serial='"+serial+"';")
        self.stop();self.start();login=self.login('p7-native-expedition')
        ready=self.call('CSGetAcceptedMissions',{},login,1,'SCGetAcceptedMissions');self.assertEqual(ready.acceptedMissions[0].state,2)
        claim=self.call('CSGetFinalReward',{'missionSerial':serial},login,402,'SCGetFinalReward');self.assertEqual(claim.exp,hours*10);self.assertEqual(claim.get.simps[0].id,'item_001');self.assertEqual(claim.get.simps[0].num,1)
        self.sql('UPDATE p7_expedition_policies SET enabled=0;')
        self.assertEqual(claim,self.call('CSGetFinalReward',{'missionSerial':serial},login,402,'SCGetFinalReward'));self.assertEqual(claim,self.call('CSGetFinalReward',{'missionSerial':serial},login,404,'SCGetFinalReward'))
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p7_expedition_heroes WHERE player_id='+str(login.playerId)+';').strip(),'0')
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p1_event_facts WHERE player_id="+str(login.playerId)+" AND kind='expedition_complete';").strip(),'1')
        self.stop();self.start();login=self.login('p7-native-expedition');self.assertEqual(claim,self.call('CSGetFinalReward',{'missionSerial':serial},login,402,'SCGetFinalReward'))
        print('P7 expedition pending patch used:',self.used_expedition_patch)

    def test_5_native_awake_original_catalogue_no_fake_exploration(self):
        import datetime
        self.start();uid='p7-awake-network-'+secrets.token_hex(4);login=self.login(uid)
        self.call('CSPlayerCreate',{'playerName':'觉醒目录门禁','heroId':'hero_047'},login,1,'SCPlayerCreate')
        catalog=self.call('CSAwakeCopyData',{},login,1,'SCAwakeCopyData');self.assertEqual((catalog.lastNum,len(catalog.copys)),(2,5))
        weekday=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoweekday()
        self.assertEqual([c.chapterId for c in catalog.copys],['1','2','3','4','5'])
        for chapter in catalog.copys:
            self.assertEqual(chapter.isOpen,int(str(weekday) in chapter.openDay.split(',')));self.assertEqual(len(chapter.smallCopys),0)
        before=self.profile(login);self.sql('UPDATE p7_awake_policies SET enabled=1;')
        for name,fields in [('CSEnterAwakeCopy',{'copyId':'101'}),('CSAwakeCopyFightBegin',{'monGuid':'fake'}),('CSAwakeCopyFightEnd',{'fightId':'fake','isWinClient':1}),('CSAwakeCopyOpenBox',{'boxGuid':'fake'}),('CSAwakeCopySettle',{}),('CSAwakeRevice',{}),('CSAwakeUseItem',{'itemId':'item_312'})]:
            op,r=self.stamped(name,fields,login,501);self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(login),before)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p7_awake_runs WHERE player_id='+str(login.playerId)+';').strip(),'0')
        self.stop();self.start();login=self.login(uid)
        restored=self.call('CSAwakeCopyData',{},login,1,'SCAwakeCopyData');self.assertEqual(catalog,restored)
        print('P7 awake pending patch used:',self.used_awake_patch)

del LiveServerTests
del GoFormationTests
