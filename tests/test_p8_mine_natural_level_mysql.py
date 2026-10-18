"""Fresh original mining1 -> actual100ore/refine1000XP -> natural2 -> real tier2 ore use/cold."""
import concurrent.futures
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

import test_p10_mysql as transport
import test_p10_scene_tcp_mysql as native
import test_p5_talent_scope_mysql as shared
from server.codec import BY_NAME

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P8_MINE_NATURAL_LEVEL_MYSQL_TEST')=='1','Explicit original guide/gather/craft fresh InnoDB flow')
class P8MineNaturalLevelMySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    stamped=transport.P10MySQLTests.stamped
    call=transport.P10MySQLTests.call
    profile=transport.P10MySQLTests.profile
    sql=shared.TalentScopeMySQLTests.sql
    write_profile=shared.TalentScopeMySQLTests.write_profile
    connect=transport.P10MySQLTests.connect
    recv=transport.P10MySQLTests.recv
    tcp_send=transport.P10MySQLTests.tcp_send
    start=native.P10SceneTCPTests.start
    stop=native.P10SceneTCPTests.stop
    setUp=transport.P10MySQLTests.setUp

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p8_natural_')or not cls.test_user.startswith('p8natural_'):raise AssertionError('Own guide schema required')
        p=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if p.returncode:raise AssertionError(p.stderr.replace(cls.test_password,'[redacted]'))
        return p.stdout

    def test_original_mining_fresh_one_to_natural_two_unlock_and_use(self):
        began=time.monotonic();report={};self.process=None;sock=None
        manifest=json.loads((ROOT/'analysis/p8/guide-manifest.json').read_text())
        with tempfile.TemporaryDirectory(prefix='hyjs-p8-natural-')as temporary:
            self.source=Path(temporary).resolve();base=ROOT/manifest['base'];shutil.copytree(base/'hyjs-server',self.source/'hyjs-server',ignore=shutil.ignore_patterns('logs'));(self.source/'analysis').symlink_to(base/'analysis',target_is_directory=True)
            for name in ['analysis/p10/stable-scene-tcp-source-20261015/manifest.json',*manifest['dependencies'],'analysis/p8/guide-manifest.json']:
                cfg=json.loads((ROOT/name).read_text());entries=cfg.get('owned',cfg.get('files'))
                if isinstance(entries,dict):entries=[{'path':p,'sha256':h}for p,h in entries.items()]
                for entry in entries:
                    if not entry['path'].startswith('hyjs-server/'):continue
                    original=ROOT/cfg['snapshot']/entry['path'];self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256']);shutil.copy2(original,self.source/entry['path'])
            shared_dir=ROOT/'analysis/p8/guide-shared-source-20261015';handler='hyjs-server/handler/gameHandler.go';shared_sha=json.loads((shared_dir/'manifest.json').read_text())['files'][handler];original=shared_dir/handler;self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),shared_sha);shutil.copy2(original,self.source/handler)
            server=self.source/'hyjs-server';self.binary=self.source/'mine-natural-native'
            scoped=[]
            for args in [['go','test','./service','-run','^TestP8OriginalGuideResources$','-count=1','-timeout=2m'],['go','build','-o',str(self.binary),'.']]:
                result=subprocess.run(['rtk','proxy',*args],cwd=server,capture_output=True,text=True,timeout=240);self.assertEqual(result.returncode,0,(result.stdout+result.stderr)[-5000:]);scoped.append((result.stdout+result.stderr).strip())
            type(self).database='hyjs_test_p8_natural_'+secrets.token_hex(6);type(self).test_user='p8natural_'+secrets.token_hex(5);type(self).test_password=secrets.token_hex(24);self.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_DATABASE=self.database,HYJS_MYSQL_USER=self.test_user,HYJS_MYSQL_PASSWORD=self.test_password)
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    self.root_sql(f"CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;CREATE USER '{self.test_user}'@'%' IDENTIFIED BY '{self.test_password}';GRANT ALL ON `{self.database}`.* TO '{self.test_user}'@'%';")
                    self.start();uid='p8-guide-'+secrets.token_hex(5);login=self.login(uid);stamp=1800900000000;colds=0
                    def http(name,fields,want):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,login,stamp,want)
                    def count(key):
                        _,g=self.profile(login);return sum(v.get('num',0)for v in(g.get('cooking',[])if key.startswith('Fish')else g.get('gems',[]))if v['id']==key)
                    def tcp(name,fields,want):
                        sock.settimeout(.02)
                        try:
                            while True:self.recv(sock)
                        except socket.timeout:pass
                        sock.settimeout(8);self.tcp_send(sock,name,fields,login)
                        for _ in range(30):
                            op,v=self.recv(sock)
                            if op==BY_NAME[want]:return v
                            if op==10001:raise AssertionError(name+': '+v.msg)
                        raise AssertionError('No '+want)
                    def enter():
                        nonlocal sock
                        sock=self.connect(login);tcp('CSLoginScene',{},'SCLoginScene');tcp('CSCurrentPosition',{},'SCCurrentPosition')
                    def move(point):
                        self.tcp_send(sock,'CSMove',{'pointNew':point,'currentTime':int(time.time()*1000)},login);tcp('CSGetHp',{},'SCGetHp');saved=self.profile(login)[1]['position']['current'];self.assertAlmostEqual(saved['x'],point['x'],places=3);self.assertAlmostEqual(saved['z'],point['z'],places=3)
                    def cold():
                        nonlocal login,sock,colds
                        before=self.profile(login);sock.close();self.stop();self.start();login=self.login(uid);after=self.profile(login)
                        self.assertEqual(after[0]['noviceGuideSchedule'],before[0]['noviceGuideSchedule']);self.assertEqual(after[1].get('gems',[]),before[1].get('gems',[]));self.assertEqual(after[1].get('cooking',[]),before[1].get('cooking',[]));colds+=1;enter()
                    http('CSPlayerCreate',{'playerName':'原冶炼自然二级','heroId':'hero_043'},'SCPlayerCreate');p,g=self.profile(login);self.assertEqual(p.get('coin',0),0);p.update(level=18,noviceGuideSchedule=9);self.write_profile(login,p,g)
                    fresh=http('CSCookingLevel',{},'SCCookingLevel');self.assertEqual((fresh.gemLevel,fresh.gemExp),(1,0));http('CSTransferScene',{'target':'field_001'},'SCTransferScene');enter();move({'x':7,'y':-.5,'z':17});http('CSGuideFish',{},'SCGuideFish');move({'x':20.84,'y':-.45,'z':7.61});http('CSGuideMine',{},'SCGuideMine');self.assertEqual(count('MineItem_007'),2)
                    def nodes():
                        text=self.sql(f'SELECT CONCAT(pos_id,"|",remaining,"|",HEX(snapshot)) FROM p8_resource_nodes WHERE player_id={login.playerId} AND scene="field_001" ORDER BY pos_id;')
                        return [(v.split('|')[0],int(v.split('|')[1]),json.loads(bytes.fromhex(v.split('|')[2])))for v in text.splitlines()]
                    locked=next((v for v in nodes()if v[2]['kind']=='mine'and v[2]['level']==2),None);self.assertIsNotNone(locked,'Original first-map must have actual tier2 node for unlock evidence');move(locked[2]['point']);op,denied=self.stamped('CSFieldResourceStartGather',{'posId':locked[0]},login,stamp+1);stamp+=1;self.assertEqual(op,10001);self.assertEqual(self.profile(login)[1]['field']['side']['tiredness'],200)
                    rows=[];batches=[];real_wait=0
                    def gather(level,key,goal):
                        nonlocal real_wait
                        while count(key)<goal:
                            self.assertLess(len(rows),100,'Original200 daily fatigue only; stop at sufficient materials')
                            options=nodes();chosen=next((v for v in options if v[1]>0 and v[2]['kind']=='mine'and v[2]['level']==level),None)
                            if chosen is None:
                                self.assertTrue(any(v[1]==0 and v[2]['kind']=='mine' for v in options),'No actual usable original node');self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND remaining=0;');tcp('CSCurrentPosition',{},'SCCurrentPosition');continue
                            pos,left,cfg=chosen;move(cfg['point']);started=tcp('CSFieldResourceStartGather',{'posId':pos},'SCFieldResourceStartGather');self.assertEqual(started.result,1)
                            if not rows:
                                at=time.monotonic();time.sleep(cfg['gatherMs']/1000+.05);real_wait=round(time.monotonic()-at,3)
                            else:self.sql(f'UPDATE p8_gather_attempts SET ready_at=1 WHERE player_id={login.playerId} AND active=1;')
                            result=tcp('CSFieldGetResource',{},'SCFieldGetResource');self.assertEqual(result.code,1);rows.append({'tier':level,'gain':[{'id':v.id,'num':v.num}for v in result.res.simps]})
                            if left==1:self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND pos_id="{pos}";')
                    # Each real batch consumes five actually collected ore; reach the original1000XP then stop.
                    for index in range(20):
                        gather(1,'MineItem_007',5);before=count('MineItem_007');refined=http('CSMineRefine',{'mineId':'MineItem_007','num':5},'SCMineRefine');self.assertEqual(refined.add,50);self.assertEqual(count('MineItem_007'),before-5);batches.append({'tier':1,'add':refined.add,'level':refined.level,'exp':refined.exp,'gain':[{'id':v.id,'num':v.num}for v in refined.res.simps]})
                        if index<19:self.assertEqual((refined.level,refined.exp),(1,50*(index+1)))
                    self.assertEqual((refined.level,refined.exp),(2,0));natural=http('CSCookingLevel',{},'SCCookingLevel');self.assertEqual((natural.gemLevel,natural.gemExp),(2,0));first_count=len(rows);cold();cold_level=http('CSCookingLevel',{},'SCCookingLevel');self.assertEqual((cold_level.gemLevel,cold_level.gemExp),(2,0))
                    gather(2,'MineItem_008',5);before=count('MineItem_008');second=http('CSMineRefine',{'mineId':'MineItem_008','num':5},'SCMineRefine');self.assertEqual((second.level,second.exp,second.add),(2,100,100));self.assertEqual(count('MineItem_008'),before-5);batches.append({'tier':2,'add':second.add,'level':second.level,'exp':second.exp,'gain':[{'id':v.id,'num':v.num}for v in second.res.simps]});cold();levels=http('CSCookingLevel',{},'SCCookingLevel');self.assertEqual((levels.gemLevel,levels.gemExp),(2,100));p,g=self.profile(login);self.assertEqual(p.get('coin',0),0);self.assertEqual(p.get('diamond',0),0);self.assertEqual(g['field']['side']['tiredness'],200-2*len(rows));self.assertEqual(count('MineItem_007'),2+sum(v['num']for row in rows for v in row['gain']if v['id']=='MineItem_007')-100);self.assertEqual(count('MineItem_008'),sum(v['num']for row in rows for v in row['gain']if v['id']=='MineItem_008')-5);sock.close();sock=None;self.stop()
                    report={'status':'PASS_FRESH_ORIGINAL_MINING1_1000_REAL_XP_TO2_AND_PREVIOUSLY_LOCKED_TIER2_REAL_USE_COLD','seconds':round(time.monotonic()-began,3),'runtimeSource':manifest['snapshot'],'guideManifestSHA256':hashlib.sha256((ROOT/'analysis/p8/guide-manifest.json').read_bytes()).hexdigest(),'sharedHandlerSHA256':shared_sha,'runtimeGoSHA256':{name:hashlib.sha256((server/name).read_bytes()).hexdigest()for name in ['service/p8GuideService.go','service/p8GatherService.go','service/p8LifeService.go','service/gemService.go']},'initialGemLevel':1,'initialGemExp':0,'actualGuideOre':2,'actualLevel1GatherCount':first_count,'actualTotalGatherCount':len(rows),'actualFatigueUsed':2*len(rows),'actualGatherRows':rows,'actualRefinementBatches':batches,'realFirstGatherSeconds':real_wait,'actualLevel2UnlockedAndUsed':True,'finalGemLevel':2,'finalGemExp':100,'finalCoin':0,'finalDiamond':0,'coldRestarts':colds,'finitePrerequisites':'Only original characterlevel18/schedule9; no currency/ore/gem/craftlevel/XP/rank/skillbook prepared. Original fixed guide grants and actual weighted mining. First5s real, later readyAt and exhausted node refresh clocks isolated; no fatigue reset, no multi-day claim. Existing local crystalPerBatch1 unchanged and each actual random gain logged; not original private crystal distribution.','productionModified':False,'phoneUI':False}
                finally:
                    if sock is not None:sock.close()
                    self.stop();self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';");left=self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';");self.assertEqual(left.split(),['0','0','0']);report['cleanupCounts']=[0,0,0]
        if report.get('status'):(ROOT/'analysis/p8/mine-natural-level-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
