"""Future-published monthly prestige -> original ranks/cutoff/P1/title/cold.

Explicit isolated InnoDB only. A finite server calendar and initial guild-entry
funding are separate prerequisites; every month score comes from real existing
P13 terminal/factory APIs. No combat winner, prestige, title or reward is seeded.
"""
import concurrent.futures
import fcntl
import hashlib
import http.client
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile
import time
import unittest
from datetime import datetime,timedelta
from zoneinfo import ZoneInfo

from server.codec import BY_NAME,CLASSES,encode_http_request,decode_responses
import test_p10_mysql as transport
import test_p10_scene_tcp_mysql as native
import test_p13_union_fight_field_mysql as field

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P13_MONTHLY_MYSQL_TEST')=='1','Explicit actual monthly source HTTP/TCP/InnoDB')
class P13MonthlyMySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    call=transport.P10MySQLTests.call
    profile=transport.P10MySQLTests.profile
    recv=transport.P10MySQLTests.recv
    tcp_send=transport.P10MySQLTests.tcp_send
    connect=transport.P10MySQLTests.connect
    receive=field.P13UnionFightFieldMySQLTests.receive
    set_clock=field.P13UnionFightFieldMySQLTests.set_clock
    start=native.P10SceneTCPTests.start
    def stop(self):
        # Close and detach the owned PID even when its exit status is unexpected;
        # schema cleanup must remain reachable after a worker failure.
        process=self.process
        try:
            native.P10SceneTCPTests.stop(self)
        finally:
            if process is not None and process.poll() is not None:
                if process.stdout is not None and not process.stdout.closed:
                    output=process.stdout.read()[-5000:];process.stdout.close()
                    if output:self.nativeFailures.append(output.replace(getattr(self,'test_password','[unset]'),'[redacted]'))
                self.process=None
    setUp=transport.P10MySQLTests.setUp
    def stamped(self,name,fields,login,stamp):
        raw=encode_http_request(BY_NAME[name],CLASSES[name](**fields),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
        connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=60)
        try:
            connection.request('POST','/sao/',body=raw,headers={'Content-Type':'application/octet-stream'});response=connection.getresponse();data=response.read();self.assertEqual(response.status,200,data);return decode_responses(data)[0]
        finally:connection.close()
    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p13_month_')or not cls.test_user.startswith('p13month_'):raise AssertionError('monthly isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError(result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout
    def sql(self,text):return self.root_sql('USE `'+self.database+'`; '+text)
    def wait_sql(self,query,expected,seconds=50):
        deadline=time.monotonic()+seconds
        while time.monotonic()<deadline:
            if self.sql(query).strip()==str(expected):return
            self.assertIsNone(self.process.poll(),'native process exited during actual worker wait');time.sleep(.4)
        self.fail('actual worker condition timed out: '+query)
    def test_original_monthly_all_rounds_cutoff_titles_and_cold(self):
        began=time.monotonic();report={};self.sockets=[];self.nativeFailures=[]
        manifest=json.loads((ROOT/'analysis/p13/monthly-manifest.json').read_text())
        # Validate actual recovered PB consumers before running any costly source chain.
        for name,fields in {'RewardItemInfo':['rewardId','rewardTitle'],'SCUFRankInfo':['rankIno'],'RankRowInfo':['values'],'SCTitleList':['titleList','curTitle'],'TitleInfo':['titleId','state','deadTime']}.items():
            self.assertIn(name,CLASSES)
            self.assertTrue(set(fields)<=set(CLASSES[name].DESCRIPTOR.fields_by_name),name)
        with tempfile.TemporaryDirectory(prefix='hyjs-p13-monthly-native-')as temp:
            self.source=Path(temp);base=Path(manifest['base']);shutil.copytree(base/'hyjs-server',self.source/'hyjs-server',ignore=shutil.ignore_patterns('logs'));(self.source/'analysis').symlink_to(base/'analysis',target_is_directory=True)
            for item in [*[json.loads((ROOT/p).read_text())for p in manifest['dependencies']],manifest]:
                entries=item.get('files')or{e['path']:e['sha256']for e in item['owned']}
                for name,digest in entries.items():
                    if not name.startswith('hyjs-server/'):continue
                    src=Path(item['snapshot'])/name
                    if not src.is_absolute():src=ROOT/src
                    self.assertEqual(hashlib.sha256(src.read_bytes()).hexdigest(),digest);target=self.source/name;target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(src,target)
            shared=json.loads((ROOT/manifest['sharedManifest']).read_text())
            for name,digest in shared['files'].items():
                if not name.startswith('hyjs-server/'):continue
                src=Path(shared.get('snapshot',str((ROOT/manifest['sharedManifest']).parent)))/name
                if not src.is_absolute():src=ROOT/src
                self.assertEqual(hashlib.sha256(src.read_bytes()).hexdigest(),digest);shutil.copy2(src,self.source/name)
            runtime={e['path']:hashlib.sha256((self.source/e['path']).read_bytes()).hexdigest()for e in manifest['owned']if e['path'].endswith('.go')}
            scoped=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP13Monthly(PublishedBoundaryAndNoRetro|UnverifiedWeekLedgerNeverRanks)$','-count=1','-timeout=2m'],cwd=self.source/'hyjs-server',capture_output=True,text=True,timeout=240);self.assertEqual(scoped.returncode,0,(scoped.stdout+scoped.stderr)[-5000:])
            main_path=self.source/'hyjs-server/main.go';main=main_path.read_text()
            for entry in ['"hyjs-server/constUtil"','"strings"']:
                if entry not in main:main=main.replace('"hyjs-server/config"','"hyjs-server/config"\n'+entry,1)
            marker='defer cancel()\n';self.assertEqual(main.count(marker),1)
            main=main.replace(marker,marker+'''
    // Test-build-only finite server calendar, guarded by this suite's owned schema.
    if !strings.HasPrefix(os.Getenv("HYJS_MYSQL_DATABASE"), "hyjs_test_p13_month_") { return errors.New("monthly native isolation guard") }
    ctx = context.WithValue(ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time {
        raw, err := os.ReadFile(os.Getenv("HYJS_P13_FIELD_NATIVE_CLOCK")); if err != nil { panic(err) }
        at, err := strconv.ParseInt(string(raw), 10, 64); if err != nil { panic(err) }
        return time.UnixMilli(at)
    })
''',1);main_path.write_text(main);subprocess.run(['rtk','proxy','gofmt','-w',str(main_path)],check=True)
            self.binary=self.source/'monthly-native';built=subprocess.run(['rtk','proxy','go','build','-o',str(self.binary),'.'],cwd=self.source/'hyjs-server',capture_output=True,text=True,timeout=240);self.assertEqual(built.returncode,0,(built.stdout+built.stderr)[-5000:])
            type(self).database='hyjs_test_p13_month_'+secrets.token_hex(6);type(self).test_user='p13month_'+secrets.token_hex(5);type(self).test_password=secrets.token_hex(24);self.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_DATABASE=self.database,HYJS_MYSQL_USER=self.test_user,HYJS_MYSQL_PASSWORD=self.test_password)
            self.clock=self.source/'clock';self.env['HYJS_P13_FIELD_NATIVE_CLOCK']=str(self.clock)
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    self.root_sql(f"CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;CREATE USER '{self.test_user}'@'%' IDENTIFIED BY '{self.test_password}';GRANT ALL ON `{self.database}`.* TO '{self.test_user}'@'%';")
                    self.run_actual_month(report)
                    report.update(status='PASS_REAL_MONTH_PREPUBLICATION_TERMINAL_FACTORY_CUTOFF_P1_TITLE_COLD',seconds=round(time.monotonic()-began,3),ownedGoSHA256=runtime,scopedGo=scoped.stdout.strip(),sharedManifest=manifest['sharedManifest'],sharedFiles=shared['files'],phoneUI=False,productionModified=False)
                except BaseException as exc:
                    report.update(status='FAIL_PARTIAL_NATIVE',seconds=round(time.monotonic()-began,3),failure=type(exc).__name__+': '+str(exc),ownedGoSHA256=runtime,sharedManifest=manifest['sharedManifest'],sharedFiles=shared['files'],phoneUI=False,productionModified=False)
                    raise
                finally:
                    for sock in self.sockets:
                        try:sock.close()
                        except OSError:pass
                    try:self.stop()
                    finally:
                        self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';");left=self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';");self.assertEqual(left.split(),['0','0','0']);report['cleanupCounts']=[0,0,0]
                        if self.nativeFailures:report['nativeFailureOutput']=self.nativeFailures
                        if report.get('status')=='FAIL_PARTIAL_NATIVE':(ROOT/'analysis/p13/monthly-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
        if report.get('status'):(ROOT/'analysis/p13/monthly-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

    def run_actual_month(self,report):
        zone=ZoneInfo('Asia/Shanghai');publication=datetime(2026,10,31,12,tzinfo=zone);declaration=datetime(2026,11,2,12,tzinfo=zone);day=declaration.replace(hour=0)+timedelta(days=1);cutoff=datetime(2026,12,1,tzinfo=zone);month='2026-11';first=int((day+timedelta(hours=20)).timestamp()*1000);published=int(publication.timestamp()*1000)-1
        self.set_clock(published+1);self.start();uids=['monthly-'+secrets.token_hex(5)for _ in range(16)];players=[self.login(uid)for uid in uids];stamp=1802000000000
        def http(name,fields,login,want):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,want)
        def count_month(table):return self.sql(f"SELECT COUNT(*) FROM {table} WHERE month='{month}';").strip()
        for i,login in enumerate(players):http('CSPlayerCreate',{'playerName':'月赛原实员'+str(i),'heroId':'hero_047'},login,'SCPlayerCreate')
        for i,login in enumerate(players):self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',{30+i},'$.diamond',1000) WHERE id={login.playerId};")
        self.sql("UPDATE p13_guild_policies SET management_enabled=1;UPDATE p13_union_fight_policies SET enrollment_enabled=1,match_enabled=0;UPDATE p13_union_fight_settlement_policies SET enabled=0;")
        policy_rewards='[{"rank":1,"rewards":"item_311,8"},{"rank":2,"rewards":"item_311,4"},{"rank":8,"rewards":"item_311,2"}]'
        self.sql(f"INSERT INTO p13_union_fight_monthly_policies(month,enabled,version,source,published_at,rewards,member_title,chairman_title) VALUES('{month}',1,'local-month14-v1','explicit local future month badge8/4/2, original private payout unknown',{published},'{policy_rewards}',1,1);")
        stamp+=1;self.assertEqual(self.stamped('CSUFRankInfo',{'rankType':2},players[0],stamp)[0],10001);self.assertEqual(count_month('p13_union_fight_monthly_offers'),'1')
        self.sql(f"UPDATE p13_union_fight_monthly_policies SET enabled=0,rewards='[{{\"rank\":1,\"rewards\":\"item_311,999\"}}]' WHERE month='{month}';")
        self.set_clock(int(declaration.timestamp()*1000));guild_ids=[]
        for i,login in enumerate(players[:14]):
            http('CSUnionCreate',{'name':'月实公会'+str(i)},login,'SCUnionCreate');gid=http('CSGetUnionInfo',{},login,'SCGetUnionInfo').unionInfo.unionId;guild_ids.append(gid);self.sql(f"UPDATE p13_guilds SET level=2 WHERE id='{gid}';")
        http('CSUnionApply',{'unionId':guild_ids[0]},players[14],'SCUnionApply');http('CSUnionAgreeApply',{'playerId':players[14].playerId},players[0],'SCUnionAgreeApply')
        for login in players[:14]:http('CSUFUnionApply',{},login,'SCUFUnionApply')
        for login in players[:14]:http('CSUFPlayerApply',{},login,'SCUFPlayerApply')
        self.stop();calendar=day.strftime('%Y-%m-%d')
        self.sql(f"INSERT INTO p13_union_fight_schedules(day,first_start_at,second_start_at,duration_millis,source) VALUES('{calendar}',{first},{first+600000},300000,'explicit isolated one genuine declaration/two rounds; no private-time claim');UPDATE p13_union_fight_policies SET match_enabled=1;UPDATE p13_union_fight_field_policies SET enabled=1,preparation_millis=300000,left_revive_point_id=1,right_revive_point_id=3,initial_guard_hp=1000,initial_gate_hp=10000,move_tolerance=3,source='explicit local clean field no combat';UPDATE p13_union_fight_settlement_policies SET enabled=1,version='local-month-terminal-v1',source='explicit local real zero-fight terminal50/25, factory5',participation_rewards='item_311,1',victory_rewards='item_311,2',win_prestige=50,lose_prestige=25,factory_mission_prestige=5,published_at={published};")
        self.sql("INSERT INTO p13_union_fight_factory_sources(day,factory_id,enabled,chariot_hp,published_at,source) VALUES"+','.join(f"('{calendar}',{i},1,{1000+i},{published},'explicit local chariotHP1000+original factoryID, original private HP unknown')"for i in range(1,5))+';')
        for camp,point in enumerate([(-4.7,-.12,9.98),(80.22,-.33,-13.36)]):
            for n in range(2):self.sql(f"INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES('{calendar}','monthly-mine-{camp}-{n}',1,'1',{point[0]},{point[1]},{point[2]},50,{published},'explicit local50 real3s mine, no injected resource');")
        self.set_clock(int((day+timedelta(minutes=31)).timestamp()*1000));self.start();players=[self.login(uid)for uid in uids];self.wait_sql('SELECT COUNT(*) FROM p13_union_fight_matches;',14)
        self.assertEqual([int(x.split()[1])for x in self.sql("SELECT `group`,COUNT(*) FROM p13_union_fight_groups GROUP BY `group` ORDER BY `group`;").splitlines()],[2,4,4,4]);self.wait_sql('SELECT COUNT(*) FROM p13_union_fight_settlement_offers;',14)
        self.set_clock(first-60000);sockets=[self.connect(login)for login in players[:14]];self.sockets.extend(sockets)
        for login,sock in zip(players[:14],sockets):sock.settimeout(30);http('CSTransferScene',{'target':'fight_union'},login,'SCTransferScene');self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCUnionFightTransfer')
        self.set_clock(first+20000)
        for login,sock in zip(players[:14],sockets):self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCUnionFightTransfer')
        self.actual_factory(http,players[0],sockets[0],first,guild_ids[0])
        report['checkpoint']='ACTUAL_14_GUILDS_14_OFFERS_400_ORE_FACTORY5'
        live=http('CSUFRankInfo',{'rankType':2},players[0],'SCUFRankInfo');self.assertEqual([r.values[0]for r in live.rankIno.list],[5]);self.assertEqual(count_month('p13_union_fight_monthly_results'),'0')
        self.set_clock(first+300000);http('CSUFCurrentStageInfo',{},players[0],'SCUFCurrentStageInfo');self.wait_sql('SELECT COUNT(*) FROM p13_union_fight_settlement_results;',7)
        report['checkpoint']='ACTUAL_FIRST7_TERMINALS_LIVE_MONTH5'
        self.set_clock(first+540000)
        for login,sock in zip(players[:14],sockets):http('CSTransferScene',{'target':'fight_union'},login,'SCTransferScene');self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCUnionFightTransfer')
        self.set_clock(first+620000)
        for login,sock in zip(players[:14],sockets):self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCUnionFightTransfer')
        self.set_clock(first+900000);http('CSUFCurrentStageInfo',{},players[0],'SCUFCurrentStageInfo');self.wait_sql('SELECT COUNT(*) FROM p13_union_fight_settlement_results;',14)
        report['checkpoint']='ACTUAL_ALL14_TERMINALS_BOTH_ROUNDS'
        # Finish original field lifecycle before withholding the committed match
        # for the monthly missing-round fault; active workers retain real FK scope.
        self.set_clock(first+905000)
        for login in players[:14]:http('CSTransferScene',{'target':'town_main'},login,'SCTransferScene')
        self.wait_sql('SELECT COUNT(*) FROM p13_union_fight_field_players WHERE left_at=0;',0)
        for sock in sockets:sock.close()
        self.sockets=[]
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_settlement_intents;').strip(),'0');self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_guild_mission_facts;').strip(),'1')
        self.assertEqual(count_month('p13_union_fight_monthly_titles'),'0')
        # Retain a byte-identical committed match row and temporarily withhold it to
        # verify missing original round stays pending; restoration adds no new win.
        missing=self.sql('SELECT id FROM p13_union_fight_matches WHERE round=2 ORDER BY id LIMIT 1;').strip();self.sql(f"CREATE TABLE monthly_round_backup LIKE p13_union_fight_matches;INSERT INTO monthly_round_backup SELECT * FROM p13_union_fight_matches WHERE id='{missing}';DELETE FROM p13_union_fight_matches WHERE id='{missing}';")
        self.set_clock(int(cutoff.timestamp()*1000));stamp+=1;self.assertEqual(self.stamped('CSUFRankInfo',{'rankType':2},players[0],stamp)[0],10001);self.assertEqual(count_month('p13_union_fight_monthly_cutoffs'),'1');self.assertEqual(count_month('p13_union_fight_monthly_results'),'0')
        report['checkpoint']='ACTUAL_MONTH_CUTOFF_MISSING_ROUND_PENDING'
        frozen=self.sql(f"SELECT snapshot_hash FROM p13_union_fight_monthly_cutoffs WHERE month='{month}';").strip()
        http('CSTransferChairman',{'playerId':players[14].playerId},players[0],'SCTransferChairman');http('CSQuitUnion',{},players[0],'SCQuitUnion');http('CSUnionApply',{'unionId':guild_ids[0]},players[15],'SCUnionApply');http('CSUnionAgreeApply',{'playerId':players[15].playerId},players[14],'SCUnionAgreeApply')
        self.sql('INSERT INTO p13_union_fight_matches SELECT * FROM monthly_round_backup;DROP TABLE monthly_round_backup;')
        stamp+=1;rank_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8)as pool:rankings=list(pool.map(lambda _:self.call('CSUFRankInfo',{'rankType':2},players[0],rank_stamp,'SCUFRankInfo'),range(8)))
        self.assertTrue(all(x==rankings[0]for x in rankings));ranks=rankings[0].rankIno.list;self.assertEqual(len(ranks),14);self.assertEqual(ranks[0].values[0],105);self.assertEqual(count_month('p13_union_fight_monthly_results'),'1');self.assertEqual(self.sql(f"SELECT snapshot_hash FROM p13_union_fight_monthly_cutoffs WHERE month='{month}';").strip(),frozen)
        self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p13_union_fight_monthly_titles WHERE month='{month}' AND player_id={players[15].playerId};").strip(),'0');self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p13_union_fight_monthly_titles WHERE month='{month}' AND player_id={players[0].playerId} AND title_id='title_029';").strip(),'1');self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p13_union_fight_monthly_titles WHERE month='{month}' AND player_id={players[14].playerId} AND title_id='title_029';").strip(),'0')
        report['checkpoint']='ACTUAL_FROZEN_MONTH_RANKS_AND_CUTOFF_IDENTITIES'
        def badges(login):return sum(x.get('num',0)for x in(self.profile(login)[1].get('items')or[])if x['id']=='item_311')
        before_badge=badges(players[0]);inbox=http('CSQueryUserReward',{},players[0],'SCQueryUserReward');reward_ids=[x.rewardId for x in inbox.rewardItemInfoList if x.rewardTitle=='公会战月奖励'];self.assertEqual(len(reward_ids),1)
        stamp+=1;claim_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8)as pool:claims=list(pool.map(lambda _:self.call('CSAddRewardsToPlayer',{'rewardIds':reward_ids},players[0],claim_stamp,'SCAddRewardsToPlayer'),range(8)))
        self.assertEqual(badges(players[0])-before_badge,8)
        titles=http('CSTitleList',{},players[0],'SCTitleList');title_map={x.titleId:x for x in titles.titleList};self.assertEqual(title_map['title_028'].state,1);self.assertEqual(title_map['title_029'].state,1);self.assertEqual(title_map['title_029'].deadTime,int(cutoff.timestamp()*1000)+672*3600000)
        follower=http('CSTitleList',{},players[14],'SCTitleList');fm={x.titleId:x for x in follower.titleList};self.assertEqual(fm['title_028'].state,1);self.assertEqual(fm['title_029'].state,0)
        base_power=self.profile(players[0])[0]['fightPower'];http('CSChangeTitle',{'titleId':'title_029'},players[0],'SCChangeTitle');power=self.profile(players[0])[0]['fightPower'];self.assertGreater(power,base_power)
        old=players[0];self.stop();self.start();players=[self.login(uid)for uid in uids];self.assertEqual(http('CSTitleList',{},players[0],'SCTitleList').curTitle,'title_029');self.assertEqual(self.profile(players[0])[0]['fightPower'],power);self.assertEqual(badges(players[0]),before_badge+8);self.assertEqual(http('CSUFRankInfo',{'rankType':2},players[0],'SCUFRankInfo').rankIno,rankings[0].rankIno)
        self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_reward_deliveries WHERE source_key='p13-month:{month}';").strip(),'15');self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p13_union_fight_monthly_titles WHERE month='{month}';").strip(),'22');self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE table_schema='"+self.database+"' AND table_name LIKE 'p13_union_fight_monthly_%';").strip(),'InnoDB')
        for path in sorted((self.source/'hyjs-server/db/sql').glob('143_p13_union_fight_monthly_*.sql')):self.sql(path.read_text())
        report.update(actualGuilds=14,actualMembersAtCutoff=15,lateJoinExcluded=players[15].playerId,actualRounds=14,actualTwoRoundPrestige=105,actualFactoryOre=400,actualFactoryPrestige=5,p1FrozenMonthBadge8=True,actualMonthlyTitleRows=22,actualFightPower=[base_power,power],coldRestarts=2,missingRoundPendingWithCutoffPreserved=True,finitePrerequisites='Synthetic forward-only server calendar Oct31->Nov2/3->Dec1; own fresh characters level30..45/1000 entry diamonds, guild level2 eligibility and local factoryHP1000+originalID finite; original guild management/signup/matching/field entry/400ore/factory/terminal/P1/title APIs real. No winner/score/factory resource/month reward/title KV/effects/GUID injection. One committed original round temporarily withheld and restored identically to test missing-round pending; no new winner fabricated.',originalPrivateMonthlyPayout='unknown; explicit prepublished local8/4/2 and competition ties; old live999/off cannot edit frozen offer')

    def actual_factory(self,http,login,sock,first,guild):
        self.tcp_send(sock,'CSUFInit',{},login);frame=self.receive(sock,'SCUnionFightTransfer');camp=frame.player.camp
        recycler,npc,factory_id=('2','1',3)if camp==0 else('5','4',4);recycle={'x':-4.7,'y':-.12,'z':9.98}if camp==0 else{'x':80.22,'y':-.33,'z':-13.36};position={'x':15,'y':-4.02,'z':26}if camp==0 else{'x':60,'y':-3.58,'z':-30};factory=next(x.guid for x in frame.factoryInfos if x.factoryId==factory_id)
        self.tcp_send(sock,'CSMove',{'pointNew':recycle},login);self.receive(sock,'SCUFElementMove');scope=self.sql(f"SELECT match_id FROM p13_union_fight_field_players WHERE player_id={login.playerId} AND left_at=0 ORDER BY joined_at DESC LIMIT 1;").strip();nodes=[int(x)for x in self.sql(f"SELECT guid FROM p13_union_fight_resource_nodes WHERE match_id='{scope}' AND ABS(x-({recycle['x']}))<.001 ORDER BY guid;").splitlines()];self.assertEqual(len(nodes),2)
        for i in range(8):
            self.tcp_send(sock,'CSUFPlayerCollect',{'resGuid':nodes[0]if i<5 else nodes[1]},login);self.assertEqual(self.receive(sock,'SCUFPlayerCollect').collectTime[0],3);self.set_clock(first+23000+i*3000);self.assertEqual(self.receive(sock,'SCUFPlayerCollectEndDetail').addResourceNum,50);self.assertEqual(http('CSUFPlayerUpResource',{'npcId':recycler},login,'SCUFPlayerUpResource').resourceNum,50)
        self.set_clock(first+64000);self.tcp_send(sock,'CSMove',{'pointNew':position},login);self.receive(sock,'SCUFElementMove');self.assertEqual(http('CSUFOpenFactoryCost',{'factoryId':factory},login,'SCUFOpenFactoryCost').cost.resourceNum,400);http('CSUFFieldCariotFactoryOpen',{'factoryGuid':factory,'npcId':npc},login,'SCUFCariotFactoryOpen');self.assertEqual(self.sql(f"SELECT prestige FROM p13_union_fight_guild_mission_facts WHERE match_id='{scope}' AND guild_id='{guild}';").strip(),'5')

if __name__=='__main__':unittest.main()
