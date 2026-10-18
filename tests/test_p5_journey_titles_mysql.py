"""Original chapter/trial title qualification from finite actual-domain saved progress."""
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

import test_p10_mysql as transport
import test_p10_scene_tcp_mysql as native
import test_p5_talent_scope_mysql as shared

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P5_JOURNEY_TITLES_MYSQL_TEST')=='1','Explicit original journey titles HTTP/InnoDB flow')
class P5JourneyTitlesMySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    stamped=transport.P10MySQLTests.stamped
    call=transport.P10MySQLTests.call
    profile=transport.P10MySQLTests.profile
    sql=shared.TalentScopeMySQLTests.sql
    write_profile=shared.TalentScopeMySQLTests.write_profile
    start=native.P10SceneTCPTests.start
    stop=native.P10SceneTCPTests.stop
    setUp=transport.P10MySQLTests.setUp

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_journey_')or not cls.test_user.startswith('p5journey_'):raise AssertionError('Own title schema required')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError(result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_saved_chapter_trial_titles_and_cold(self):
        began=time.monotonic();report={};self.process=None
        manifest=json.loads((ROOT/'analysis/p5/journey-titles-manifest.json').read_text())
        with tempfile.TemporaryDirectory(prefix='hyjs-p5-journey-')as temporary:
            self.source=Path(temporary).resolve();base=ROOT/manifest['base'];shutil.copytree(base/'hyjs-server',self.source/'hyjs-server',ignore=shutil.ignore_patterns('logs'));(self.source/'analysis').symlink_to(base/'analysis',target_is_directory=True)
            for item in [*[json.loads((ROOT/p).read_text())for p in manifest['dependencies']],manifest]:
                for entry in item['owned']:
                    if not entry['path'].startswith('hyjs-server/'):continue
                    original=ROOT/item['snapshot']/entry['path'];self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256']);shutil.copy2(original,self.source/entry['path'])
            self.binary=self.source/'journey-titles-native';scoped=[]
            for args in [['go','test','./service','-run','^TestP5OriginalJourneyTitles$','-count=1','-timeout=2m'],['go','build','-o',str(self.binary),'.']]:
                result=subprocess.run(['rtk','proxy',*args],cwd=self.source/'hyjs-server',text=True,capture_output=True,timeout=300);self.assertEqual(result.returncode,0,(result.stdout+result.stderr)[-5000:]);scoped.append((result.stdout+result.stderr).strip())
            type(self).database='hyjs_test_p5_journey_'+secrets.token_hex(6);type(self).test_user='p5journey_'+secrets.token_hex(5);type(self).test_password=secrets.token_hex(24);self.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_DATABASE=self.database,HYJS_MYSQL_USER=self.test_user,HYJS_MYSQL_PASSWORD=self.test_password)
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    self.root_sql(f"CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;CREATE USER '{self.test_user}'@'%' IDENTIFIED BY '{self.test_password}';GRANT ALL ON `{self.database}`.* TO '{self.test_user}'@'%';");self.start();uid='journey-title-'+secrets.token_hex(5);login=self.login(uid);stamp=1801110000000;colds=0
                    def http(name,fields,want):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,login,stamp,want)
                    def titles():return http('CSTitleList',{},'SCTitleList')
                    def entries(reply):return {v.titleId:v for v in reply.titleList}
                    def cold():
                        nonlocal login,colds
                        before=self.profile(login);self.stop();self.start();login=self.login(uid);after=self.profile(login);self.assertEqual(after[0]['fightPower'],before[0]['fightPower']);self.assertEqual(after[1].get('kv',[]),before[1].get('kv',[]));colds+=1
                    http('CSPlayerCreate',{'playerName':'原章节试炼里程碑','heroId':'hero_043'},'SCPlayerCreate')
                    missions=[h for h in shared.original_table('CopyMission')if h['ChapterId']=='10'];self.assertEqual(len(missions),6)
                    p,g=self.profile(login);g['copy']['currentChapterId']='20';g['copy']['passedChapterInfo']=[{'chapterId':'20','star':25}];g['copy']['passedCopyInfo']=[{'chapterId':'10','missionId':h['MissionId'],'star':1}for h in missions[:-1]];self.write_profile(login,p,g)
                    locked=entries(titles());self.assertEqual((locked['title_013'].current,locked['title_013'].total,locked['title_013'].state),(0,10,0));self.assertEqual(locked['title_014'].state,0)
                    p,g=self.profile(login);g['copy']['passedCopyInfo'].append({'chapterId':'10','missionId':missions[-1]['MissionId'],'star':1});self.write_profile(login,p,g)
                    complete=entries(titles());self.assertEqual((complete['title_013'].current,complete['title_013'].total,complete['title_013'].state),(10,10,1));self.assertEqual(complete['title_014'].state,0)
                    self.sql(f"INSERT INTO p7_rush_states(player_id,day,run_id,floor,total_stars,record_stars,high_floor,free_reset_used,paid_reset_used,elite_used,elite_bought,active_fight) VALUES({login.playerId},'2026-10-15',1,30,0,0,29,0,0,0,0,'');")
                    trial=entries(titles());self.assertEqual((trial['title_019'].current,trial['title_019'].total,trial['title_019'].state),(29,30,0))
                    self.sql(f"UPDATE p7_rush_states SET high_floor=30,floor=1 WHERE player_id={login.playerId};")
                    state=self.sql(f"SELECT * FROM p7_rush_states WHERE player_id={login.playerId};");trial=entries(titles());self.assertEqual((trial['title_019'].current,trial['title_019'].total,trial['title_019'].state),(30,30,1));self.assertEqual(trial['title_020'].state,0);self.assertEqual(trial['title_021'].state,0);base_power=self.profile(login)[0]['fightPower']
                    chapter=http('CSChangeTitle',{'titleId':'title_013'},'SCChangeTitle');self.assertEqual(chapter.curTitle,'title_013');chapter_power=self.profile(login)[0]['fightPower'];self.assertGreater(chapter_power,base_power);cold();self.assertEqual(titles().curTitle,'title_013')
                    highest=http('CSChangeTitle',{'titleId':'title_019'},'SCChangeTitle');self.assertEqual(highest.curTitle,'title_019');trial_power=self.profile(login)[0]['fightPower'];self.assertGreater(trial_power,base_power);cold();self.assertEqual(titles().curTitle,'title_019');self.assertEqual(self.profile(login)[0]['fightPower'],trial_power);self.assertEqual(self.sql(f"SELECT * FROM p7_rush_states WHERE player_id={login.playerId};"),state)
                    clear=http('CSChangeTitle',{'titleId':''},'SCChangeTitle');self.assertEqual(clear.curTitle,'');self.assertEqual(self.profile(login)[0]['fightPower'],base_power);self.stop()
                    report={'status':'PASS_ORIGINAL_HTTP_SAVED_CHAPTER_TRIAL_QUALIFICATION_AND_COLD','seconds':round(time.monotonic()-began,3),'ownedGoSHA256':{e['path']:hashlib.sha256((self.source/e['path']).read_bytes()).hexdigest()for e in manifest['owned']if e['path'].endswith('.go')},'scopedGo':scoped,'originalChapter':'chapter10 all six original missions star1; openchapter20 and summary25 alone never qualify','originalTrial':'HighFloor29/Floor30 locked; HighFloor30/Floor1 qualifies,60/100 locked; owner SQL unchanged by list/wear/cold','actualFightPower':[base_power,chapter_power,trial_power,base_power],'coldRestarts':colds,'finitePrerequisites':'All chapter/stage progress is explicitly finite historical fixture. No new victory, sweep, P6 winner, source resource, earned title KV, bonus or fightPower injected. Only actual CSTitleList/CSChangeTitle transactions and client protocol projections are verified. Existing owner producer remains unchanged.','phoneUI':False,'productionModified':False}
                finally:
                    self.stop();self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';");left=self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';");self.assertEqual(left.split(),['0','0','0']);report['cleanupCounts']=[0,0,0]
        if report.get('status'):(ROOT/'analysis/p5/journey-titles-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
