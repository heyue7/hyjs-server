"""Original28B TCP gather→cook→food/revival and WorldBoss error continuity."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
import unittest

import test_p10_mysql as tcpbase
import test_p10_scene_tcp_mysql as native
import test_p5_talent_scope_mysql as shared
from server.codec import BY_NAME

ROOT=Path(__file__).resolve().parents[1]
BASE=ROOT/'build/candidates/joint-rush-formation-v97a-20261015'
OWNED=['hyjs-server/handler/gameHandler.go','hyjs-server/service/p8GatherService.go','hyjs-server/service/p8LifeService.go','hyjs-server/handler/p8OriginalSocketTimestamp_test.go']

@unittest.skipUnless(os.environ.get('HYJS_P8_ORIGINAL_SOCKET_MYSQL_TEST')=='1','Explicit original28B socket full-flow fresh InnoDB test')
class P8OriginalSocketMySQLTests(unittest.TestCase):
    send=tcpbase.P10MySQLTests.send
    login=tcpbase.P10MySQLTests.login
    stamped=tcpbase.P10MySQLTests.stamped
    call=tcpbase.P10MySQLTests.call
    profile=tcpbase.P10MySQLTests.profile
    sql=shared.TalentScopeMySQLTests.sql
    write_profile=shared.TalentScopeMySQLTests.write_profile
    connect=tcpbase.P10MySQLTests.connect
    recv=tcpbase.P10MySQLTests.recv
    tcp_send=tcpbase.P10MySQLTests.tcp_send
    start=native.P10SceneTCPTests.start
    stop=native.P10SceneTCPTests.stop
    setUp=tcpbase.P10MySQLTests.setUp

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p8_tcp_') or not cls.test_user.startswith('p8tcp_'):raise AssertionError('P8 TCP dedicated database guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError(result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def command(self,args,cwd,fail=False):
        result=subprocess.run(['rtk','proxy',*map(str,args)],cwd=cwd,text=True,capture_output=True,timeout=240)
        output=result.stdout+result.stderr
        if fail:
            self.assertNotEqual(result.returncode,0,output);self.assertIn('original zero timestamp socket rejected',output)
        else:self.assertEqual(result.returncode,0,output[-5000:])
        return output

    def test_original_six_socket_entries_real_source_consumption_and_next_info(self):
        began=time.monotonic();report={};self.process=None
        with tempfile.TemporaryDirectory(prefix='hyjs-p8-original-socket-') as directory:
            self.source=Path(directory).resolve();shutil.copytree(BASE/'hyjs-server',self.source/'hyjs-server',ignore=shutil.ignore_patterns('logs'));(self.source/'analysis').symlink_to(BASE/'analysis',target_is_directory=True)
            dep=json.loads((ROOT/'analysis/p10/stable-scene-tcp-source-20261015/manifest.json').read_text());rel='hyjs-server/service/p10SceneService.go';entry=next(x for x in dep['files'] if x['path']==rel);original=ROOT/dep['snapshot']/rel;self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256']);shutil.copy2(original,self.source/rel)
            manifest=json.loads((ROOT/'analysis/p8/original-socket-manifest.json').read_text())
            for entry in manifest['owned']:
                if not entry['path'].startswith('hyjs-server/'):continue
                original=ROOT/manifest['snapshot']/entry['path'];self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256']);shutil.copy2(original,self.source/entry['path'])
            server=self.source/'hyjs-server';source_shas={str(p.relative_to(server)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(server.rglob('*.go'))};old=self.source/'old-handler.json';old.write_text(json.dumps({'Replace':{str(server/'handler/gameHandler.go'):str(BASE/'hyjs-server/handler/gameHandler.go')}}));old_proof=self.command(['go','test','-overlay',old,'./handler','-run','^TestOriginalP8SocketZeroTimestampValidation$','-count=1'],server,True);scoped=self.command(['go','test','./handler','./service','-run','^(TestOriginalP8SocketZeroTimestampValidation|TestP8Life)$','-count=1','-timeout=3m'],server);self.binary=self.source/'hyjs-p8-native';self.command(['go','build','-o',self.binary,'.'],server)
            type(self).database='hyjs_test_p8_tcp_'+secrets.token_hex(6);type(self).test_user='p8tcp_'+secrets.token_hex(5);type(self).test_password=secrets.token_hex(24);self.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_DATABASE=self.database,HYJS_MYSQL_USER=self.test_user,HYJS_MYSQL_PASSWORD=self.test_password)
            with open('/tmp/hyjs-isolated-mysql.lock','a+') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    self.root_sql(f"CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;CREATE USER '{self.test_user}'@'%' IDENTIFIED BY '{self.test_password}';GRANT ALL ON `{self.database}`.* TO '{self.test_user}'@'%';")
                    self.start();uid='original-p8-tcp-'+secrets.token_hex(5);login=self.login(uid);stamp=1800800000000;colds=0;frame_log=[];sock=None
                    def http(name,fields,want):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,login,stamp,want)
                    def drain():
                        sock.settimeout(.025)
                        try:
                            while True:self.recv(sock)
                        except socket.timeout:pass
                        finally:sock.settimeout(8)
                    def tcp(name,fields,want):
                        drain();self.tcp_send(sock,name,fields,login)
                        for _ in range(20):
                            op,value=self.recv(sock)
                            if op==BY_NAME[want]:frame_log.append({'request':name,'response':want,'headerBytes':28,'requestTimestamp':0});return value
                            if op==10001:raise AssertionError('Unexpected business refusal '+name+': '+value.msg)
                        raise AssertionError('Expected TCP reply missing '+want)
                    def info():
                        if self.profile(login)[1].get('field',{}).get('side',{}).get('curHp',1)<=0:
                            return tcp('CSGetHp',{},'SCGetHp')
                        pos=self.sql(f'SELECT pos_id FROM p8_resource_nodes WHERE player_id={login.playerId} AND scene="field_001" ORDER BY pos_id LIMIT 1;').strip();self.assertTrue(pos)
                        return tcp('CSSideResourceCheck',{'posId':pos},'SCSideResourceCheck')
                    def refused(name,fields):
                        before=self.profile(login);result=tcp(name,fields,'ErrorMessage');self.assertEqual(self.profile(login)[0].get('diamond',0),before[0].get('diamond',0));info();return result.msg
                    def enter_socket():
                        nonlocal sock
                        sock=self.connect(login);tcp('CSLoginScene',{},'SCLoginScene');tcp('CSCurrentPosition',{},'SCCurrentPosition');info()
                    def cold():
                        nonlocal login,colds,sock
                        p,g=self.profile(login);wallet=(p.get('coin',0),p.get('diamond',0));items=g.get('cooking',[]);sock.close();self.stop();self.start();login=self.login(uid);after=self.profile(login);self.assertEqual((after[0].get('coin',0),after[0].get('diamond',0)),wallet);self.assertEqual(after[1].get('cooking',[]),items);colds+=1;enter_socket()
                    def move(point):
                        self.tcp_send(sock,'CSMove',{'pointNew':point,'currentTime':int(time.time()*1000)},login);info();saved=self.profile(login)[1]['position']['current'];self.assertAlmostEqual(saved['x'],point['x'],places=3);self.assertAlmostEqual(saved['z'],point['z'],places=3)
                    def death():
                        p,g=self.profile(login);g['field']['side'].update(curHp=0,state=5);g['field']['side'].pop('battlePosHps',None);g['field'].pop('fight',None);g['position']['current']={'x':1000,'y':0,'z':1000};self.write_profile(login,p,g)
                    http('CSPlayerCreate',{'playerName':'原野外TCP全流程','heroId':'hero_043'},'SCPlayerCreate');p,g=self.profile(login);p.update(level=100,noviceGuideSchedule=8,warriorBagSize=1000,equipBagSize=1000);self.assertEqual((p.get('coin',0),p.get('diamond',0)),(0,0));g['copy']['passedCopyInfo']=[{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0} for r in shared.original_table('CopyMission') if r['ChapterId']=='27'];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g);source=http('CSCopyGetStarAward',{'chapterId':'27','starIndex':0},'SCCopyGetStarAward');self.assertEqual({x.id:x.num for x in source.gain.simps if x.id in ['coin','diamond']},{'coin':260000,'diamond':350});http('CSTransferScene',{'target':'field_001'},'SCTransferScene');enter_socket()
                    # HTTP still requires the actual positive original stamp for these six entry points.
                    zero_http=[]
                    for name,fields in [('CSFieldResourceStartGather',{'posId':'original'}),('CSFieldGetResource',{}),('CSHpRecover',{'costItem':{'id':'item_129','num':1}}),('CSResurrection',{}),('CSTransfer2Safe',{}),('CSWorldBossFight',{'bossId':'original'})]:
                        before=self.profile(login);op,result=self.stamped(name,fields,login,0);self.assertEqual(op,10001);self.assertIn('时间戳',result.msg);self.assertEqual(self.profile(login),before);zero_http.append(name)
                    gather_rows=[];real_wait=0
                    while sum(x.get('num',0) for x in self.profile(login)[1].get('cooking',[]) if x['id']=='FishItem_007')<10:
                        self.assertLess(len(gather_rows),80,'Stop at original fatigue allowance; no RNG or resource injection')
                        nodes=self.sql(f'SELECT CONCAT(pos_id,"|",remaining,"|",HEX(snapshot)) FROM p8_resource_nodes WHERE player_id={login.playerId} AND scene="field_001" ORDER BY pos_id;');nodes=[(v.split('|')[0],int(v.split('|')[1]),json.loads(bytes.fromhex(v.split('|')[2]))) for v in nodes.splitlines()];chosen=next((v for v in nodes if v[1]>0 and v[2]['kind']=='fish' and v[2]['level']==1),None)
                        if chosen is None:self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND remaining=0;');info();continue
                        pos,left,cfg=chosen;move(cfg['point']);start=tcp('CSFieldResourceStartGather',{'posId':pos},'SCFieldResourceStartGather');self.assertEqual(start.result,1)
                        if not gather_rows:
                            token=self.sql(f'SELECT token FROM p8_gather_attempts WHERE player_id={login.playerId};').strip()
                            for _ in range(3):self.assertEqual(tcp('CSFieldResourceStartGather',{'posId':pos},'SCFieldResourceStartGather'),start)
                            self.assertEqual(self.sql(f'SELECT token FROM p8_gather_attempts WHERE player_id={login.playerId};').strip(),token);refused('CSFieldGetResource',{});cold();before_wait=time.monotonic();time.sleep(cfg['gatherMs']/1000+.05);real_wait=round(time.monotonic()-before_wait,3)
                        else:self.sql(f'UPDATE p8_gather_attempts SET ready_at=1 WHERE player_id={login.playerId} AND active=1;')
                        result=tcp('CSFieldGetResource',{},'SCFieldGetResource');self.assertEqual(result.code,1);gather_rows.append({'resource':cfg['resourceId'],'gain':[{'id':v.id,'num':v.num} for v in result.res.simps]});self.assertEqual(self.profile(login)[1]['field']['side']['tiredness'],200-2*len(gather_rows));info()
                        if len(gather_rows)==1:
                            current=self.profile(login)[1].get('cooking',[])
                            for _ in range(3):self.assertNotEqual(tcp('CSFieldGetResource',{},'SCFieldGetResource').code,1)
                            self.assertEqual(self.profile(login)[1].get('cooking',[]),current)
                        if left==1:self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND pos_id="{pos}";')
                    crafted=http('CSFishCooking',{'fish':{'id':'FishItem_026','num':2}},'SCFishCooking');self.assertEqual(sum(x.num for x in crafted.res.simps if x.id=='FishItem_026'),2);p,g=self.profile(login);g['field']['side'].update(curHp=.4,state=0);g['field']['side'].pop('battlePosHps',None);g['position']['current']={'x':1000,'y':0,'z':1000};self.write_profile(login,p,g)
                    food={'costItem':{'id':'FishItem_026','num':1}};tcp('CSHpRecover',food,'SCHpRecover');self.assertAlmostEqual(self.profile(login)[1]['field']['side']['curHp'],.45);refused('CSHpRecover',food);self.sql(f'UPDATE p8_life_states SET food_ready_at=1 WHERE player_id={login.playerId};');tcp('CSHpRecover',food,'SCHpRecover');self.assertAlmostEqual(self.profile(login)[1]['field']['side']['curHp'],.5);info();self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p1_event_facts WHERE player_id={login.playerId} AND kind="field_hp_recover" AND source_key LIKE "p8:food:ready:%";').strip(),'2')
                    paid_errors=[]
                    for _ in range(2):
                        death();paid=tcp('CSResurrection',{},'SCResurrenttion');self.assertEqual(paid.cost[0].num,50);self.assertEqual(self.profile(login)[1]['field']['side']['curHp'],1);paid_errors.append(refused('CSResurrection',{}))
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p1_event_facts WHERE player_id={login.playerId} AND source_key LIKE "p8:resurrect:death:%" AND amount=50;').strip(),'2');self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p8_gather_receipts WHERE player_id={login.playerId};').strip(),'0')
                    death();paid_http=http('CSResurrection',{},'SCResurrenttion');paid_stamp=stamp
                    for _ in range(3):self.assertEqual(self.call('CSResurrection',{},login,paid_stamp,'SCResurrenttion'),paid_http)
                    cold();self.assertEqual(self.call('CSResurrection',{},login,paid_stamp,'SCResurrenttion'),paid_http);death();tcp('CSGetHp',{},'SCGetHp');early_free=refused('CSTransfer2Safe',{});free_start=time.monotonic();time.sleep(10.05);free=tcp('CSTransfer2Safe',{},'SCTransfer2Safe');self.assertEqual(free.result,1);free_wait=round(time.monotonic()-free_start,3);self.assertAlmostEqual(self.profile(login)[1]['field']['side']['curHp'],.1);refused('CSTransfer2Safe',{});cold()
                    # Open only original query policy in this dedicated schema; battle authority stays unavailable.
                    self.sql('UPDATE p12_world_boss_policies SET enabled=1;');boss=tcp('CSWorldBossInfo',{},'SCWorldBossInfo');boss_id=boss.worldBossShowInfo.bossuuid;boss_error=tcp('CSWorldBossFight',{'bossId':boss_id},'ErrorMessage');after_boss=tcp('CSWorldBossInfo',{},'SCWorldBossInfo');self.assertEqual(after_boss,boss);info()
                    p,g=self.profile(login);self.assertEqual((p['coin'],p['diamond']),(250000,200));self.assertEqual(sum(x.get('num',0) for x in g.get('cooking',[]) if x['id']=='FishItem_026'),0);self.assertEqual(g['field']['side']['tiredness'],200-2*len(gather_rows));self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p8_gather_receipts WHERE player_id={login.playerId};').strip(),'1');engines=self.sql('SELECT TABLE_NAME,ENGINE FROM information_schema.tables WHERE table_schema="'+self.database+'" AND TABLE_NAME IN ("profiles","p8_gather_attempts","p8_resource_nodes","p8_life_states","p8_gather_receipts");');self.assertTrue(engines.strip() and all(v.endswith('InnoDB') for v in engines.splitlines()))
                    sock.close();self.stop();report={'status':'PASS_ORIGINAL28B_TCP_FIVE_P8_ENTRIES_REAL_GATHER_COOK_FOOD_REVIVE_AND_WORLDBOSS_ERROR_NEXT_INFO_INNODB','seconds':round(time.monotonic()-began,3),'baseline':str(BASE.relative_to(ROOT)),'ownedGoSHA256':{p:source_shas[p.removeprefix('hyjs-server/')] for p in OWNED},'frozenGoSHA256':source_shas,'oldHandlerCounterexample':old_proof.strip(),'scopedGo':scoped.strip(),'zeroHTTPAllSixRejected':zero_http,'actualGatherCount':len(gather_rows),'actualGatherRows':gather_rows,'actualFirstGatherWaitSeconds':real_wait,'actualDailyFatigueUsed':2*len(gather_rows),'actualCooking':'FishItem026x2/10000coin/50XP from actual TCP Fish007x10','actualFoodHP':[.4,.45,.5],'actualFoodFacts':2,'actualPaidTCPRevives':2,'actualPaidHTTPRevives':1,'actualPaidDiamondCost':150,'actualFreeReviveWaitSeconds':free_wait,'coldRestarts':colds,'finalCoin':p['coin'],'finalDiamond':p['diamond'],'worldBossFightError':boss_error.msg,'nextWorldBossInfoSameConnection':True,'frameLog':frame_log,'engines':engines.strip().splitlines(),'finitePrerequisites':'Playerlevel100/guide8/bags/historicalchapter27stars; wounded/dead state fixtures are finite survival qualification, not original engine damage/winner. Subsequent gatherReadyAt/nodeRefresh and one foodReadyAt are isolated clocks only; original fatigue/RNG/amounts/costs/rewards/definitions unchanged. Actual firstgather5s and free10s wait. P12 querypolicy enabled only isolatedschema, combat authority unchanged.','originalEvidence':'analysis/p10/transport-audit-20261015/socket-positive-stamp-conflicts.il.txt','productionModified':False,'phoneUI':False}
                finally:
                    self.stop();self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';");remaining=self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';");self.assertEqual(remaining.split(),['0','0','0']);report['cleanupCounts']=[0,0,0]
        if report.get('status'):(ROOT/'analysis/p8/original-socket-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
