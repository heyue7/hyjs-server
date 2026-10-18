"""Original level18 fish/mining tutorial -> actual gather -> cooking/refining/cold."""
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

@unittest.skipUnless(os.environ.get('HYJS_P8_GUIDE_MYSQL_TEST')=='1','Explicit original guide/gather/craft fresh InnoDB flow')
class P8GuideMySQLTests(unittest.TestCase):
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
        if not cls.database.startswith('hyjs_test_p8_guide_')or not cls.test_user.startswith('p8guide_'):raise AssertionError('Own guide schema required')
        p=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if p.returncode:raise AssertionError(p.stderr.replace(cls.test_password,'[redacted]'))
        return p.stdout

    def test_original_guide_fish_mine_natural_craft_and_cold(self):
        began=time.monotonic();report={};self.process=None;sock=None
        manifest=json.loads((ROOT/'analysis/p8/guide-manifest.json').read_text())
        with tempfile.TemporaryDirectory(prefix='hyjs-p8-guide-')as temporary:
            self.source=Path(temporary).resolve();base=ROOT/manifest['base'];shutil.copytree(base/'hyjs-server',self.source/'hyjs-server',ignore=shutil.ignore_patterns('logs'));(self.source/'analysis').symlink_to(base/'analysis',target_is_directory=True)
            for name in ['analysis/p10/stable-scene-tcp-source-20261015/manifest.json',*manifest['dependencies'],'analysis/p8/guide-manifest.json']:
                cfg=json.loads((ROOT/name).read_text());entries=cfg.get('owned',cfg.get('files'))
                if isinstance(entries,dict):entries=[{'path':p,'sha256':h}for p,h in entries.items()]
                for entry in entries:
                    if not entry['path'].startswith('hyjs-server/'):continue
                    original=ROOT/cfg['snapshot']/entry['path'];self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256']);shutil.copy2(original,self.source/entry['path'])
            shared_dir=ROOT/'analysis/p8/guide-shared-source-20261015';handler='hyjs-server/handler/gameHandler.go';shared_sha=json.loads((shared_dir/'manifest.json').read_text())['files'][handler];original=shared_dir/handler;self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),shared_sha);shutil.copy2(original,self.source/handler)
            server=self.source/'hyjs-server';self.binary=self.source/'guide-native'
            scoped=[]
            for args in [['go','test','./service','-run','^TestP8OriginalGuideResources$','-count=1','-timeout=2m'],['go','build','-o',str(self.binary),'.']]:
                result=subprocess.run(['rtk','proxy',*args],cwd=server,capture_output=True,text=True,timeout=240);self.assertEqual(result.returncode,0,(result.stdout+result.stderr)[-5000:]);scoped.append((result.stdout+result.stderr).strip())
            type(self).database='hyjs_test_p8_guide_'+secrets.token_hex(6);type(self).test_user='p8guide_'+secrets.token_hex(5);type(self).test_password=secrets.token_hex(24);self.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_DATABASE=self.database,HYJS_MYSQL_USER=self.test_user,HYJS_MYSQL_PASSWORD=self.test_password)
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
                    http('CSPlayerCreate',{'playerName':'原生活引导闭合','heroId':'hero_043'},'SCPlayerCreate');p,g=self.profile(login);self.assertEqual(p.get('coin',0),0);p.update(level=18,noviceGuideSchedule=9);g['copy']['passedCopyInfo']=[{'chapterId':v['ChapterId'],'missionId':v['MissionId'],'star':5,'challengeTimes':0}for v in shared.original_table('CopyMission')if v['ChapterId']=='1'];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
                    reward=http('CSCopyGetStarAward',{'chapterId':'1','starIndex':1},'SCCopyGetStarAward');self.assertEqual(sum(v.num for v in reward.gain.simps if v.id=='coin'),6000)
                    op,error=self.stamped('CSGuideFish',{},login,stamp+1);stamp+=1;self.assertEqual(op,10001)
                    http('CSTransferScene',{'target':'field_001'},'SCTransferScene');enter();move({'x':7,'y':-.5,'z':17})
                    for name in ['CSGuideFish','CSGuideMine']:
                        op,error=self.stamped(name,{},login,0);self.assertEqual(op,10001);self.assertIn('timestamp',error.msg.lower())
                    op,error=self.stamped('CSGuideMine',{},login,stamp+1);stamp+=1;self.assertEqual(op,10001)
                    stamp+=1;fish_stamp=stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8)as pool:fish=list(pool.map(lambda _:self.call('CSGuideFish',{},login,fish_stamp,'SCGuideFish'),range(8)))
                    self.assertTrue(all(v==fish[0]for v in fish));self.assertEqual({v.id:v.num for v in fish[0].get.simps},{'FishItem_007':1,'FishItem_013':1,'FishItem_019':1});self.assertEqual(self.profile(login)[0]['noviceGuideSchedule'],18)
                    op,error=self.stamped('CSGuideFish',{},login,stamp+1);stamp+=1;self.assertEqual(op,10001);cold();move({'x':20.84,'y':-.45,'z':7.61});mine=http('CSGuideMine',{},'SCGuideMine');mine_stamp=stamp;self.assertEqual([(v.id,v.num)for v in mine.get.simps],[('MineItem_007',2)]);self.assertEqual(self.profile(login)[0]['noviceGuideSchedule'],19);cold();self.assertEqual(self.call('CSGuideMine',{},login,mine_stamp,'SCGuideMine'),mine);self.assertEqual(count('MineItem_007'),2)
                    rows=[];real_wait=0
                    for kind,key,goal in [('fish','FishItem_007',5),('mine','MineItem_007',5)]:
                        while count(key)<goal:
                            self.assertLess(len(rows),60,'Bounded actual source only')
                            text=self.sql(f'SELECT CONCAT(pos_id,"|",remaining,"|",HEX(snapshot)) FROM p8_resource_nodes WHERE player_id={login.playerId} AND scene="field_001" ORDER BY pos_id;');nodes=[(v.split('|')[0],int(v.split('|')[1]),json.loads(bytes.fromhex(v.split('|')[2])))for v in text.splitlines()];chosen=next((v for v in nodes if v[1]>0 and v[2]['kind']==kind and v[2]['level']==1),None)
                            if chosen is None:
                                self.assertTrue(any(v[1]==0 and v[2]['kind']==kind for v in nodes),'Original map has no level1 or exhausted usable node');self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND remaining=0;');tcp('CSCurrentPosition',{},'SCCurrentPosition');continue
                            pos,left,cfg=chosen;move(cfg['point']);started=tcp('CSFieldResourceStartGather',{'posId':pos},'SCFieldResourceStartGather');self.assertEqual(started.result,1)
                            if not rows:
                                waiting=time.monotonic();time.sleep(cfg['gatherMs']/1000+.05);real_wait=round(time.monotonic()-waiting,3)
                            else:self.sql(f'UPDATE p8_gather_attempts SET ready_at=1 WHERE player_id={login.playerId} AND active=1;')
                            result=tcp('CSFieldGetResource',{},'SCFieldGetResource');self.assertEqual(result.code,1);rows.append({'kind':kind,'gain':[{'id':v.id,'num':v.num}for v in result.res.simps]})
                            if left==1:self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND pos_id="{pos}";')
                    raw_fish=count('FishItem_007');raw_ore=count('MineItem_007');crafted=http('CSFishCooking',{'fish':{'id':'FishItem_026','num':1}},'SCFishCooking');self.assertEqual(sum(v.num for v in crafted.res.simps if v.id=='FishItem_026'),1);refined=http('CSMineRefine',{'mineId':'MineItem_007','num':5},'SCMineRefine');self.assertEqual(refined.add,50);self.assertEqual(count('MineItem_007'),raw_ore-5)
                    p,g=self.profile(login);g['field']['side'].update(curHp=.4,state=0);g['field']['side'].pop('battlePosHps',None);g['position']['current']={'x':1000,'y':0,'z':1000};self.write_profile(login,p,g);tcp('CSHpRecover',{'costItem':{'id':'FishItem_026','num':1}},'SCHpRecover');self.assertAlmostEqual(self.profile(login)[1]['field']['side']['curHp'],.45);self.assertEqual(count('FishItem_007'),raw_fish-5);self.assertEqual(count('FishItem_026'),0);cold();p,g=self.profile(login);self.assertEqual(p['coin'],1000);self.assertEqual(p['noviceGuideSchedule'],19);self.assertEqual(g['field']['side']['tiredness'],200-2*len(rows));self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p8_gather_receipts WHERE player_id={login.playerId} AND operation IN ("guide-fish","guide-mine");').strip(),'2');sock.close();sock=None;self.stop()
                    report={'status':'PASS_ORIGINAL_GUIDE18_FISH19_MINE_ACTUAL_GATHER_COOK_REFINE_FOOD_COLD','seconds':round(time.monotonic()-began,3),'ownedGoSHA256':{e['path']:hashlib.sha256((self.source/e['path']).read_bytes()).hexdigest()for e in manifest['owned']if e['path'].endswith('.go')},'sharedHandlerSHA256':shared_sha,'scopedGo':scoped,'actualGuideFish':{'FishItem_007':1,'FishItem_013':1,'FishItem_019':1},'actualGuideMine':{'MineItem_007':2},'eightFishRequests':'same committed response, one grant','originalSchedule':[9,18,19],'actualGatherRows':rows,'firstGatherWaitSeconds':real_wait,'actualFatigue':2*len(rows),'coinSource':6000,'cookingCoinCost':5000,'finalCoin':1000,'cooking':'original026x1/25XP, actual food HP .4->.45','refining':{'ore':5,'exp':50,'gain':[{'id':v.id,'num':v.num}for v in refined.res.simps],'crystalPolicy':'Existing named local SQL default crystalPerBatch1, unchanged; not original private distribution'},'coldRestarts':colds,'finitePrerequisites':'Only original18 level/current schedule9/historicalchapter1stars; no currency/guide material inserted. Actual6000coin reward, original fixed guide material and actual weighted source. Firstgather5sec real; subsequent existing attempts readyAt/nodeRefresh isolated clock only. Injury .4/outside-safe finite food-use qualification, not actual P6 damage.','phoneUI':False,'productionModified':False}
                finally:
                    if sock is not None:sock.close()
                    self.stop();self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';");left=self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';");self.assertEqual(left.split(),['0','0','0']);report['cleanupCounts']=[0,0,0]
        if report.get('status'):(ROOT/'analysis/p8/guide-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
