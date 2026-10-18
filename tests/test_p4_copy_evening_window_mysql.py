"""Declared local evening original mainline benefit binary frames using locked disposable InnoDB."""
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

OWNED = ['hyjs-server/constUtil/p4CopyBenefits.go', 'hyjs-server/service/p4CopyWindowService.go', 'hyjs-server/service/p4CopyWindow_test.go', 'hyjs-server/constUtil/p4CopyWindow.go', 'hyjs-server/service/p4CopyBenefitsService.go', 'hyjs-server/db/sql/83_p4_copy_benefit_rules.seed.sql', 'tests/test_p4_copy_evening_window_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P4_EVENING_WINDOW_MYSQL_TEST') == '1', 'Explicit locked isolated P4 evening window')
class CopyEveningWindowMySQLTests(unittest.TestCase):
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
        cls.lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        fcntl.flock(cls.lock.fileno(), fcntl.LOCK_EX)
        cls.addClassCleanup(cls.lock.close)
        cls.database = 'hyjs_test_p4_evening_window_' + secrets.token_hex(6)
        cls.test_user = 'p4evening_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p4-evening-window-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-evening-window')
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")
        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0','0','0']: raise AssertionError('P4 evening window schema/user/grants remain')
            report = shared.ROOT / 'analysis/p4/copy-evening-window-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P4_EVENING_WINDOW_BASE', shared.ROOT / 'build/candidates/joint-chat-seasonal-v57a-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('Evening baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for rel in OWNED:
            if rel.startswith('hyjs-server/'):
                target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(shared.ROOT/rel,target)
        # Root-owned source differs from this fixed base only by the agreed append hook.
        hook_old='gain.Simps = append(gain.Simps, bonus.GetSimps()...)\n\t\t\t\tgain.Wars = append(gain.Wars, bonus.GetWars()...)'
        target=source/'service/copyService.go';expected=target.read_text().replace(hook_old,'p1AppendGain(gain, bonus)')
        shared_source=shared.ROOT/'hyjs-server/service/copyService.go'
        if hashlib.sha256(shared_source.read_bytes()).hexdigest()!='82b79627f4838030ced965ba64445dfb2d713c4abbc05aa35f81b37ba05d16fe': raise AssertionError('Root shared hook changed; freeze again')
        if shared_source.read_text()!=expected:raise AssertionError('Unexpected root copyService changes beyond exact append hook')
        shutil.copy2(shared_source,target)
        cls.frozen_source=source;cls.baseline_source=str(base.relative_to(shared.ROOT))
        (Path(cls.temp.name)/'analysis').symlink_to(shared.ROOT/'analysis',target_is_directory=True)
        cls.source_shas={str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob('*.go'))}
        result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=source,capture_output=True,timeout=180)
        if result.returncode: raise AssertionError((result.stdout+result.stderr).decode(errors='replace')[-4000:])

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p4_evening_window_') or not cls.test_user.startswith('p4evening_'):raise AssertionError('P4 evening window isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P4 evening window administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_declared_window_source_sweep_cost_bonus_current_receipt_and_operatoroff(self):
        began=time.monotonic();self.start();uid='declared-evening-window';login=self.login(uid);stamp=1800100000000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        call('CSPlayerCreate',{'playerName':'晚间原双倍福利','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=14,coin=0,exp=0,stamina=100,noviceGuideSchedule=8)
        g['copy']['passedCopyInfo']=[{'chapterId':'1','missionId':'1','star':5,'challengeTimes':0}];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        month=time.strftime('%Y%m',time.gmtime(time.time()+8*3600));key='p4:copy:evening:'+month
        row=self.sql(f"SELECT enabled,start_minute,end_minute,currency_factor,drop_factor,source FROM p4_copy_benefit_rules WHERE id='{key}';").strip().split('\t')
        self.assertEqual(row[:5],['1','1140','1260','2','2']);self.assertIn('local-mainline-evening-double-v1',row[5])
        # The service creates the instance itself. Outside its actual daily window,
        # this isolated suite shifts only minute eligibility; no XP/coin/facts inserted.
        minute=(time.gmtime(time.time()+8*3600).tm_hour*60+time.gmtime(time.time()+8*3600).tm_min);shifted=not (1140<=minute<1260)
        if shifted:self.sql(f"UPDATE p4_copy_benefit_rules SET start_minute={minute},end_minute={min(1440,minute+60)} WHERE id='{key}';")
        info=call('CSActivityInfo',{'activityId':'allCopyDouble'},'SCActivityInfo');self.assertEqual(info.param,'2');self.assertLess(len(info.information),80)
        sweep={'chapterId':'1','missionId':'1'};stamp+=1;first_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSCopySweep',sweep,login,first_stamp,'SCCopySweep'),range(8)))
        self.assertTrue(all(x==replies[0] for x in replies));first=replies[0]
        coin=next(x.num for x in first.gain.simps if x.id=='coin');exp=next(x.num for x in first.gain.simps if x.id=='exp')
        self.assertEqual((coin,exp),(4380,280));self.assertEqual(self.profile(login)[0]['coin'],4380)
        self.assertEqual((self.profile(login)[0]['level'],self.profile(login)[0]['stamina']),(15,110))
        saved=self.profile(login);self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSCopySweep',sweep,login,first_stamp,'SCCopySweep'),first);self.assertEqual(self.profile(login),saved)
        self.sql(f"UPDATE p4_copy_benefit_rules SET currency_factor=0,drop_factor=0 WHERE id='{key}';")
        self.assertEqual(call('CSActivityInfo',{'activityId':'allCopyDouble'},'SCActivityInfo').param,'2')
        self.sql(f"UPDATE p4_copy_benefit_rules SET enabled=0 WHERE id='{key}';")
        self.assertEqual(call('CSActivityInfo',{'activityId':'allCopyDouble'},'SCActivityInfo').endTime,0)
        plain=call('CSCopySweep',sweep,'SCCopySweep');plain_coin=next(x.num for x in plain.gain.simps if x.id=='coin');self.assertEqual(plain_coin,2300)
        saved=self.profile(login);self.stop();self.start();login=self.login(uid)
        self.assertEqual(call('CSActivityInfo',{'activityId':'allCopyDouble'},'SCActivityInfo').endTime,0)
        self.assertEqual(self.profile(login),saved)
        self.assertEqual(self.sql(f"SELECT enabled FROM p4_copy_benefit_rules WHERE id='{key}';").strip(),'0')
        report={'date':'2026-10-14','policy':'local-mainline-evening-double-v1','defaultCreatedByService':row[:5],'originalMainlineSource':'actual original single sweep, current server timestamp and10 stamina; finite level14 and firstmission star prerequisite, coin starts0','actualCoinAndXP':[coin,exp],'actualLevelAndStaminaAfter':[15,110],'8sameFrame':'once/identical response/cold receipt and complete profile','operatorOff':'new sweep2300,Info closed,service restart preservesoff and complete save','hotEdit':'actual frozen2 factors retain earlier input','minuteEligibilityShiftOnly':shifted,'originalPrivateWindowUnknown':True,'productionModified':False,'phoneUI':False,'seconds':round(time.monotonic()-began,3),'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'cleanup':'whole-life filelock/schema-user-grants/temp source-binary'}
        (shared.ROOT/'analysis/p4/copy-evening-window-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
