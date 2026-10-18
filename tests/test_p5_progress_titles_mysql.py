"""Original actual sweep level24->25 -> title009 wear/effects/clear/cold."""
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

@unittest.skipUnless(os.environ.get('HYJS_P5_PROGRESS_TITLES_MYSQL_TEST')=='1','Explicit original progression titles HTTP/InnoDB flow')
class P5ProgressTitlesMySQLTests(unittest.TestCase):
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
        if not cls.database.startswith('hyjs_test_p5_titles_')or not cls.test_user.startswith('p5titles_'):raise AssertionError('Own title schema required')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError(result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_actual_level25_title_wear_effect_and_cold(self):
        began=time.monotonic();report={};self.process=None;tail=os.environ.get('HYJS_P5_PROGRESS_TITLES_TAIL')=='1'
        manifest=json.loads((ROOT/'analysis/p5/progress-titles-manifest.json').read_text())
        with tempfile.TemporaryDirectory(prefix='hyjs-p5-titles-')as temporary:
            self.source=Path(temporary).resolve();base=ROOT/manifest['base'];shutil.copytree(base/'hyjs-server',self.source/'hyjs-server',ignore=shutil.ignore_patterns('logs'));(self.source/'analysis').symlink_to(base/'analysis',target_is_directory=True)
            for entry in manifest['owned']:
                if not entry['path'].startswith('hyjs-server/'):continue
                original=ROOT/manifest['snapshot']/entry['path'];self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256']);shutil.copy2(original,self.source/entry['path'])
            self.binary=self.source/'progress-titles-native';scoped=[]
            for args in [['go','test','./service','-run','^TestP5OriginalProgressTitles$','-count=1','-timeout=2m'],['go','build','-o',str(self.binary),'.']]:
                result=subprocess.run(['rtk','proxy',*args],cwd=self.source/'hyjs-server',text=True,capture_output=True,timeout=240);self.assertEqual(result.returncode,0,(result.stdout+result.stderr)[-5000:]);scoped.append((result.stdout+result.stderr).strip())
            type(self).database='hyjs_test_p5_titles_'+secrets.token_hex(6);type(self).test_user='p5titles_'+secrets.token_hex(5);type(self).test_password=secrets.token_hex(24);self.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_DATABASE=self.database,HYJS_MYSQL_USER=self.test_user,HYJS_MYSQL_PASSWORD=self.test_password)
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    self.root_sql(f"CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;CREATE USER '{self.test_user}'@'%' IDENTIFIED BY '{self.test_password}';GRANT ALL ON `{self.database}`.* TO '{self.test_user}'@'%';");self.start();uid='progress-title-'+secrets.token_hex(5);login=self.login(uid);stamp=1801100000000;colds=0
                    def http(name,fields,want):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,login,stamp,want)
                    def title_list():return http('CSTitleList',{},'SCTitleList')
                    def entries(reply):return {v.titleId:v for v in reply.titleList}
                    def cold():
                        nonlocal login,colds
                        before=self.profile(login);self.stop();self.start();login=self.login(uid);after=self.profile(login);self.assertEqual(after[0]['level'],before[0]['level']);self.assertEqual(after[0].get('exp',0),before[0].get('exp',0));self.assertEqual(after[0]['fightPower'],before[0]['fightPower']);self.assertEqual(after[1].get('kv',[]),before[1].get('kv',[]));colds+=1
                    http('CSPlayerCreate',{'playerName':'原升级称号真实用途','heroId':'hero_043'},'SCPlayerCreate');p,g=self.profile(login);threshold=int(next(v['exp']for v in shared.original_table('UserExp')if v['level']=='24'));p.update(level=25 if tail else 24,exp=0 if tail else threshold-1,noviceGuideSchedule=9);g['copy']['isOpen']=1;g['copy']['passedCopyInfo']=[{'chapterId':'1','missionId':'1','star':5,'challengeTimes':0}];self.write_profile(login,p,g)
                    source_exp=0
                    if not tail:
                        before=entries(title_list());self.assertEqual((before['title_009'].state,before['title_009'].current,before['title_009'].total),(0,24,25));self.assertEqual(before['title_015'].state,0);self.assertEqual(before['title_017'].state,0);op,error=self.stamped('CSChangeTitle',{'titleId':'title_009'},login,stamp+1);stamp+=1;self.assertEqual(op,10001)
                        swept=http('CSCopySweep',{'chapterId':'1','missionId':'1'},'SCCopySweep');source_exp=sum(v.num for v in swept.gain.simps if v.id=='exp');after=self.profile(login);self.assertEqual(after[0]['level'],25);self.assertEqual(after[0].get('exp',0),source_exp-1)
                    unlocked=entries(title_list());self.assertEqual((unlocked['title_009'].state,unlocked['title_009'].current,unlocked['title_009'].total),(1,25,25));self.assertEqual(unlocked['title_010'].state,0);base_power=self.profile(login)[0]['fightPower'];stamp+=1;wear_stamp=stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8)as pool:replies=list(pool.map(lambda _:self.call('CSChangeTitle',{'titleId':'title_009'},login,wear_stamp,'SCChangeTitle'),range(8)))
                    self.assertTrue(all(v==replies[0]for v in replies));self.assertEqual(replies[0].curTitle,'title_009');power=self.profile(login)[0]['fightPower'];self.assertEqual(power-base_power,20);self.assertEqual(title_list().curTitle,'title_009');cold();self.assertEqual(title_list().curTitle,'title_009');self.assertEqual(self.profile(login)[0]['fightPower'],power)
                    cleared=http('CSChangeTitle',{'titleId':''},'SCChangeTitle');self.assertEqual(cleared.curTitle,'');self.assertEqual(self.profile(login)[0]['fightPower'],base_power);cold();self.assertEqual(title_list().curTitle,'');self.assertEqual(self.profile(login)[0]['fightPower'],base_power);self.stop()
                    report={'status':'PASS_INDEPENDENT_TITLE009_COLD_CLEAR_TAIL'if tail else'PASS_ACTUAL_ORIGINAL_SWEEP_LEVEL25_UNLOCK_TITLE009_WEAR_DEFENSE_POWER_CLEAR_COLD','seconds':round(time.monotonic()-began,3),'ownedGoSHA256':{e['path']:hashlib.sha256((self.source/e['path']).read_bytes()).hexdigest()for e in manifest['owned']if e['path'].endswith('.go')},'scopedGo':scoped,'actualOriginalPlayerLevel':[25]if tail else[24,25],'actualExperienceGain':source_exp,'originalTitle':'title_009/level25/pdef10/mdef10','originalHTTP':'CSTitleList24801/CSChangeTitle24803','actualFightPower':[base_power,power,base_power],'domainExactBattleDefenseChange':[10,10],'eightWear':'one current title/effect, same original response','coldRestarts':colds,'craftMilestones':'Focused domain validates fresh1 -> actual saved jewelry5/cooking3 qualification, higher10/6 stayslocked; finite crafting level fixture clearly separate from natural source.','finitePrerequisites':('Separate suffix actor has finite characterlevel25/XP0/guide9; no title, earned-title KV, effect, or fightPower injected. Actual source is the separately retained prefix evidence; no P6 engine victory claim.' if tail else 'Only characterlevel24/XPthreshold-1/guide9/historical mission1five-star eligibility. Actual original ten-stamina sweep XP crosses25. No level25/title/granted-title/effect/fightPower injected; no P6 engine victory claim.'),'prefixEvidence':'analysis/p5/progress-titles-prefix-validation.json'if tail else'', 'tailQualification':'Separate fresh finitelevel25 actor verifies suffix only; prefix actual24->25 on previous own actor already passed before runner expectation error'if tail else'', 'phoneUI':False,'productionModified':False}
                finally:
                    self.stop();self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';");left=self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';");self.assertEqual(left.split(),['0','0','0']);report['cleanupCounts']=[0,0,0]
        if report.get('status'):(ROOT/'analysis/p5/progress-titles-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
