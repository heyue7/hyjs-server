"""Original five gem-box economy binary frames using locked disposable InnoDB."""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile
import time
import unittest

import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as transport

OWNED = ['hyjs-server/service/copyBoxService.go', 'hyjs-server/service/copyBox_test.go', 'hyjs-server/service/p5Service.go', 'hyjs-server/constUtil/copyBox.go', 'hyjs-server/db/copyBoxRule.go', 'hyjs-server/db/copyBoxClaim.go', 'hyjs-server/db/sql/110_copy_box_rules.sql', 'hyjs-server/db/sql/110_copy_box_rules.seed.sql', 'hyjs-server/db/sql/110_copy_box_claims.sql', 'analysis/p5/copy-box-original-audit.json', 'analysis/p5/copy-box-original.il.txt', 'analysis/p5/copy-box-scene-original.il.txt', 'analysis/p5/copy-box-event-original.il.txt', 'analysis/p5/copy-box-integration.patch', 'tests/test_p5_copy_box_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_COPY_BOX_MYSQL_TEST') == '1', 'Explicit locked isolated P5 copy box')
class CopyBoxMySQLTests(unittest.TestCase):
    send = shared.TalentScopeMySQLTests.send
    login = shared.TalentScopeMySQLTests.login
    stamped = transport.GameCreationMySQLTests.stamped
    call = shared.TalentScopeMySQLTests.call
    profile = shared.TalentScopeMySQLTests.profile
    start = shared.TalentScopeMySQLTests.start
    stop = shared.TalentScopeMySQLTests.stop
    setUp = shared.TalentScopeMySQLTests.setUp
    sql = shared.TalentScopeMySQLTests.sql
    write_profile = shared.TalentScopeMySQLTests.write_profile

    @classmethod
    def setUpClass(cls):
        # The lock covers schema creation, every frame, restart and final cleanup.
        cls.database = 'hyjs_test_p5_copy_box_' + secrets.token_hex(6)
        cls.test_user = 'p5box_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-copy-box-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-gem-supply')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_COPY_BOX_BASE', shared.ROOT / 'build/candidates/joint-natural-guild-v59-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p4/resource-exchange-manifest.json','analysis/p5/gold-egg-manifest.json','analysis/p5/copy-reset-manifest.json']:
            dependency=json.loads((shared.ROOT/manifest).read_text());dependency_source=shared.ROOT/dependency['snapshot']
            for entry in dependency['owned']:
                rel=entry['path']
                if rel.startswith('hyjs-server/'):
                    source_file=dependency_source/rel
                    if hashlib.sha256(source_file.read_bytes()).hexdigest()!=entry['sha256']:raise AssertionError('Dependency SHA changed')
                    target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(source_file,target)
        for rel in OWNED:
            if rel.startswith('hyjs-server/'):
                target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(shared.ROOT/rel,target)
        # Exact root-owned hook: suppress only when the open aggregate covers P2.
        target=source/'service/turntableService.go';old=target.read_text()
        needle='\tif redExchange != nil {\n\t\tresult.ActivityList = append(result.ActivityList, redExchange)\n\t}'
        replacement='\tif redExchange != nil {\n\t\tcovered,e:=s.P4ResourceExchangeCoversRed(ctx,id,time.Now())\n\t\tif e!=nil{return nil,e}\n\t\tif !covered{result.ActivityList=append(result.ActivityList,redExchange)}\n\t}'
        if needle not in old:raise AssertionError('Unexpected root hook baseline')
        target.write_text(old.replace(needle,replacement))
        target=source/'service/copyService.go';old=target.read_text()
        needle='\t\tresult = &pb.SCCopyEnter{TaskId: proto.String(jobs[0].GetTaskId()), JobList: jobs, SceneName: proto.String(mission["SceneName"])}'
        replacement='\t\tboxes, err := s.CaptureCopyBoxes(tx, id, b.GetChapterId(), b.GetMissionId())\n\t\tif err != nil { return err }\n'+needle.replace('JobList: jobs,','JobList: jobs, BoxList: boxes,')
        if needle not in old:raise AssertionError('Unexpected copyEnter baseline')
        target.write_text(old.replace(needle,replacement))
        cls.frozen_source=source;cls.baseline_source=str(base.relative_to(shared.ROOT))
        (Path(cls.temp.name)/'analysis').symlink_to(shared.ROOT/'analysis',target_is_directory=True)
        cls.source_shas={str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob('*.go'))}
        result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=source,capture_output=True,timeout=180)
        if result.returncode: raise AssertionError((result.stdout+result.stderr).decode(errors='replace')[-4000:])

        cls.lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        fcntl.flock(cls.lock.fileno(), fcntl.LOCK_EX)
        cls.addClassCleanup(cls.lock.close)
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")
        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0','0','0']: raise AssertionError('P5 copy box schema/user/grants remain')
            report = shared.ROOT / 'analysis/p5/copy-box-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_copy_box_') or not cls.test_user.startswith('p5box_'):raise AssertionError('P5 copy box isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 copy box administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_scene_boxes_full_wins_once_real_gear_hero_cold(self):
        began=time.monotonic();self.start();stamp=1800150000000;uid='actual-original-copy-chests';login=self.login(uid)
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        tasks=shared.original_table('CopyTask');missions=shared.original_table('CopyMission')
        call('CSPlayerCreate',{'playerName':'原场景箱源','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,stamina=1000,equipBagSize=1000,warriorBagSize=1000,noviceGuideSchedule=8)
        g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions];g['copy']['passedChapterInfo']=[];g['copy_first_passed_tasks']=[x['TaskId'] for x in tasks];g['run']=None;g['quests']=[x['OpenId'] for x in missions if x['OpenId']];self.write_profile(login,p,g)
        self.sql('UPDATE p4_copy_benefit_rules SET enabled=0;')
        saved=self.profile(login);stamp+=1
        self.assertEqual(self.stamped('CSCopyBox',{'boxId':'box_3'},login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
        claims=[];gear=None;warrior=None;last_request=None;last_stamp=None;last_response=None
        for mission in ['3','6','8']:
            chapter=next(x['ChapterId'] for x in missions if x['MissionId']==mission);request={'boxId':'box_'+mission}
            entered=call('CSCopyEnter',{'chapterId':chapter,'missionId':mission},'SCCopyEnter')
            self.assertEqual([(x.boxId,x.isOpen,x.res) for x in entered.boxList],[(request['boxId'],0,'box')])
            saved=self.profile(login);stamp+=1
            self.assertEqual(self.stamped('CSCopyBox',request,login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            for tid in sorted([x['TaskId'] for x in tasks if x['MissionId']==mission],key=int):
                start=call('CSCopyFightStart',{'chapterId':chapter,'missionId':mission,'taskId':tid},'SCCopyFightStart')
                end=call('CSCopyFightEnd',{'fightId':start.fightId,'result':1,'battlePlayer':start.battlePlayer},'SCCopyFightEnd')
            self.assertEqual(end.isCopyComplete,1)
            if mission=='8':
                p,g=self.profile(login);p['equipBagSize']=len(g.get('equipment',[]))+1;self.write_profile(login,p,g);saved=self.profile(login);stamp+=1
                self.assertEqual(self.stamped('CSCopyBox',request,login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
                self.assertEqual(self.sql(f"SELECT claimed FROM copy_box_claims WHERE player_id={login.playerId} AND box_id='box_8';").strip(),'0')
                p['equipBagSize']=1000;self.write_profile(login,p,g)
            before=self.profile(login);stamp+=1;box_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:results=list(pool.map(lambda _:self.call('CSCopyBox',request,login,box_stamp,'SCCopyBox'),range(8)))
            self.assertTrue(all(x==results[0] for x in results));response=results[0];after=self.profile(login)
            self.assertEqual(after[0]['coin']-before[0]['coin'],80000)
            if mission=='3':
                self.assertEqual(after[0]['diamond']-before[0]['diamond'],50)
            if mission=='6':gear=next(x for x in response.currentResource if x.id=='equip_002');self.assertTrue(gear.guid)
            if mission=='8':
                self.assertEqual(sum(x.num for x in response.currentResource if x.id=='equip_003'),2)
                self.assertEqual(len({x.guid for x in response.currentResource if x.id=='equip_003'}),2)
                warrior=next(x for x in response.currentResource if x.id=='hero_032');self.assertTrue(warrior.guid)
            stamp+=1;self.assertEqual(self.stamped('CSCopyBox',request,login,stamp)[0],10001);self.assertEqual(self.profile(login),after)
            self.assertEqual(self.sql(f"SELECT claimed FROM copy_box_claims WHERE player_id={login.playerId} AND box_id='{request['boxId']}';").strip(),'1')
            self.stop();self.start();login=self.login(uid)
            self.assertEqual(self.call('CSCopyBox',request,login,box_stamp,'SCCopyBox'),response);self.assertEqual(self.profile(login),after)
            claims.append({'mission':mission,'chapter':chapter,'resources':[(x.id,x.guid,x.num) for x in response.currentResource]})
            last_request,last_stamp,last_response=request,box_stamp,response
        call('CSFormationOn',{'formatPos':1,'id':gear.id,'guid':gear.guid},'SCFormationOn')
        call('CSFormationOn',{'formatPos':2,'id':warrior.id,'guid':warrior.guid},'SCFormationOn')
        entered=call('CSCopyEnter',{'chapterId':'1','missionId':'3'},'SCCopyEnter')
        self.assertEqual(entered.boxList[0].isOpen,1)
        saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved)
        self.sql('UPDATE copy_box_rules SET enabled=0 WHERE id=1;')
        self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.sql('SELECT enabled FROM copy_box_rules WHERE id=1;').strip(),'0')
        self.assertEqual(self.call('CSCopyBox',last_request,login,last_stamp,'SCCopyBox'),last_response);self.assertEqual(self.profile(login),saved)
        report={'date':'2026-10-14','policy':'copy-box-original-once-after-run-v1','originalRows':94,'reachableRows':79,'originalRewardEntries':203,'domainInstances':211,'actualSource':claims,'use':'box6 originalequip002 worn on mainactor;box8 bluehero032 actual formationpos2; unique2 equip003; complete cold profile','gate':'historic stars/no entry rejects;actual entered incomplete rejects;actual completed original client-result victory fixtures qualify; capacity1 for twoequipment fullrollback;8same once;newstamp denied;claimed reentryisOpen1;operatoroff/cold oldreceipt','preparation':'finite level100/stamina1000/bags1000/previousstars+firstpassed+original prior quest flags for entry qualification;coin/diamond initially0;no boxclaim/equip/hero/frag/sourcecoins inserted. Original server HTTP fightstarts/ends use explicitclient-result victory fixture, not engine/phone','productionModified':False,'phoneUI':False,'sharedCopyEnter':'exact proposed capture/BoxList hook; final owned sourceSHA below','seconds':round(time.monotonic()-began,3),'sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source}
        (shared.ROOT/'analysis/p5/copy-box-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
