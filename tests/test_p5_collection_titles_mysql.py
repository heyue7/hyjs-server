"""Original guide hero025 completes finite purple/ALO history, titles wear and cold."""
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

@unittest.skipUnless(os.environ.get('HYJS_P5_COLLECTION_TITLES_MYSQL_TEST')=='1','Explicit original collection titles HTTP/InnoDB flow')
class P5CollectionTitlesMySQLTests(unittest.TestCase):
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
        if not cls.database.startswith('hyjs_test_p5_cohort_')or not cls.test_user.startswith('p5cohort_'):raise AssertionError('Own title schema required')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError(result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_actual_guide_last_card_original_titles_and_cold(self):
        began=time.monotonic();report={};self.process=None
        manifest=json.loads((ROOT/'analysis/p5/collection-titles-manifest.json').read_text())
        with tempfile.TemporaryDirectory(prefix='hyjs-p5-cohort-')as temporary:
            self.source=Path(temporary).resolve();base=ROOT/manifest['base'];shutil.copytree(base/'hyjs-server',self.source/'hyjs-server',ignore=shutil.ignore_patterns('logs'));(self.source/'analysis').symlink_to(base/'analysis',target_is_directory=True)
            for item in [*[json.loads((ROOT/p).read_text())for p in manifest['dependencies']],manifest]:
                for entry in item['owned']:
                    if not entry['path'].startswith('hyjs-server/'):continue
                    original=ROOT/item['snapshot']/entry['path'];self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256']);shutil.copy2(original,self.source/entry['path'])
            self.binary=self.source/'collection-titles-native';scoped=[]
            for args in [['go','test','./service','-run','^TestP5Original(Collection|Progress)Titles$','-count=1','-timeout=2m'],['go','build','-o',str(self.binary),'.']]:
                result=subprocess.run(['rtk','proxy',*args],cwd=self.source/'hyjs-server',text=True,capture_output=True,timeout=300);self.assertEqual(result.returncode,0,(result.stdout+result.stderr)[-5000:]);scoped.append((result.stdout+result.stderr).strip())
            type(self).database='hyjs_test_p5_cohort_'+secrets.token_hex(6);type(self).test_user='p5cohort_'+secrets.token_hex(5);type(self).test_password=secrets.token_hex(24);self.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_DATABASE=self.database,HYJS_MYSQL_USER=self.test_user,HYJS_MYSQL_PASSWORD=self.test_password)
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    self.root_sql(f"CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;CREATE USER '{self.test_user}'@'%' IDENTIFIED BY '{self.test_password}';GRANT ALL ON `{self.database}`.* TO '{self.test_user}'@'%';");self.start();uid='collection-title-'+secrets.token_hex(5);login=self.login(uid);stamp=1801110000000;colds=0
                    def http(name,fields,want):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,login,stamp,want)
                    def titles():return http('CSTitleList',{},'SCTitleList')
                    def entries(reply):return {v.titleId:v for v in reply.titleList}
                    def cold():
                        nonlocal login,colds
                        before=self.profile(login);self.stop();self.start();login=self.login(uid);after=self.profile(login);self.assertEqual(after[0]['fightPower'],before[0]['fightPower']);self.assertEqual(after[1].get('kv',[]),before[1].get('kv',[]));colds+=1
                    http('CSPlayerCreate',{'playerName':'原图鉴称号真实招募','heroId':'hero_043'},'SCPlayerCreate')
                    rows=shared.original_table('Hero');visible=[h for h in rows if h['id'].startswith('hero')and h['heroClass']=='0'and h['iconName']and h['IsShowInCard']!='1'and int(h['quality'])>12]
                    history=sorted({h['id']for h in visible if(h['quality']=='18'or h['space']=='2')and h['id']!='hero_025'})
                    self.assertEqual(len([h for h in visible if h['quality']=='18']),13);self.assertEqual(len([h for h in visible if h['space']=='2']),19)
                    self.sql('INSERT INTO p5_collections(player_id,hero_id,acquired_at) VALUES '+','.join(f"({login.playerId},'{hero}',12345)"for hero in history)+';')
                    p,g=self.profile(login);p['noviceGuideSchedule']=1;g['quests']=list(set((g.get('quests')or[])+['quest_1']));self.write_profile(login,p,g)
                    before=entries(titles());self.assertEqual((before['title_003'].current,before['title_003'].total,before['title_003'].state),(18,19,0));self.assertEqual((before['title_006'].current,before['title_006'].total,before['title_006'].state),(12,13,0));self.assertEqual(before['title_005'].total,10);self.assertEqual(before['title_007'].total,26);self.assertEqual(before['title_008'].total,16)
                    source=http('CSWarriorRecruitGuide',{},'SCWarriorRecruitGuide');self.assertEqual(len(source.get.wars),1);actual=source.get.wars[0];self.assertEqual(actual.id,'hero_025');self.assertTrue(actual.guid)
                    unlocked=entries(titles());self.assertEqual((unlocked['title_003'].current,unlocked['title_003'].total,unlocked['title_003'].state),(19,19,1));self.assertEqual((unlocked['title_006'].current,unlocked['title_006'].total,unlocked['title_006'].state),(13,13,1));base_power=self.profile(login)[0]['fightPower']
                    purple=http('CSChangeTitle',{'titleId':'title_006'},'SCChangeTitle');self.assertEqual(purple.curTitle,'title_006');purple_power=self.profile(login)[0]['fightPower'];self.assertGreater(purple_power,base_power)
                    alo=http('CSChangeTitle',{'titleId':'title_003'},'SCChangeTitle');self.assertEqual(alo.curTitle,'title_003');alo_power=self.profile(login)[0]['fightPower'];self.assertGreater(alo_power,purple_power);cold();self.assertEqual(titles().curTitle,'title_003');self.assertEqual(self.profile(login)[0]['fightPower'],alo_power)
                    sale=http('CSWarriorSell',{'guid':[actual.guid]},'SCWarriorSell');self.assertEqual(sum(v.num for v in sale.get.simps if v.id=='coin'),1000);self.assertFalse(any(v['guid']==actual.guid for v in self.profile(login)[1]['warriors']));cold();after=entries(titles());self.assertEqual(after['title_003'].state,1);self.assertEqual(after['title_006'].state,1);self.assertEqual(after['title_003'].current,19);self.assertEqual(after['title_006'].current,13)
                    unchanged=self.sql(f"SELECT COUNT(*),MIN(acquired_at),MAX(acquired_at) FROM p5_collections WHERE player_id={login.playerId} AND hero_id IN ("+','.join("'"+h+"'"for h in history)+');').split();self.assertEqual(unchanged,[str(len(history)),'12345','12345']);self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p5_collections WHERE player_id={login.playerId} AND hero_id='hero_025';").strip(),'1')
                    http('CSChangeTitle',{'titleId':''},'SCChangeTitle');self.assertEqual(self.profile(login)[0]['fightPower'],base_power);self.stop()
                    report={'status':'PASS_ORIGINAL_GUIDE_REAL_LAST_CARD_PURPLE_ALO_TITLES_WEAR_SALE_COLD','seconds':round(time.monotonic()-began,3),'ownedGoSHA256':{e['path']:hashlib.sha256((self.source/e['path']).read_bytes()).hexdigest()for e in manifest['owned']if e['path'].endswith('.go')},'scopedGo':scoped,'originalVisibleWorldCohorts':[21,19,22,3],'originalVisibleColorCohorts':[10,13,26,16],'actualOriginalSource':'CSWarriorRecruitGuide -> hero_025 quality18/world2, real new GUID; zero injected last card or title','actualUnlock':['title_006 purple13/13 att100','title_003 ALO19/19 att300'],'actualFightPower':[base_power,purple_power,alo_power,base_power],'actualSaleCoin':1000,'coldRestarts':colds,'historicalAcquiredAtUnchanged':12345,'finitePrerequisites':f'Explicit {len(history)} previously acquired history IDs with AcquiredAt12345; quest_1 completed and guide1 finite qualification. Other history not claimed natural. No source card/title/currency/effects/selected-title injected. No P6 victory.','phoneUI':False,'productionModified':False}
                finally:
                    self.stop();self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';");left=self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';");self.assertEqual(left.split(),['0','0','0']);report['cleanupCounts']=[0,0,0]
        if report.get('status'):(ROOT/'analysis/p5/collection-titles-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
