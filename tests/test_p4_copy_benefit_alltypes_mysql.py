"""Original confirmed first-drop benefit binary frames using locked disposable InnoDB."""
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

OWNED = ['hyjs-server/service/p4CopyBenefitsService.go', 'hyjs-server/service/p4CopyBenefits_test.go', 'hyjs-server/constUtil/p4CopyBenefits.go', 'tests/test_p4_copy_benefit_alltypes_mysql.py', 'analysis/p4/copy-benefit-alltypes-integration.patch']

@unittest.skipUnless(os.environ.get('HYJS_P4_BENEFIT_ALLTYPES_MYSQL_TEST') == '1', 'Explicit locked isolated P4 confirmed benefit')
class CopyBenefitAllTypesMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p4_benefit_alltypes_' + secrets.token_hex(6)
        cls.test_user = 'p4benefit_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p4-benefit-alltypes-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-benefit-alltypes')
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")
        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0','0','0']: raise AssertionError('P4 confirmed benefit schema/user/grants remain')
            report = shared.ROOT / 'analysis/p4/copy-benefit-alltypes-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P4_BENEFIT_ALLTYPES_BASE', shared.ROOT / 'build/candidates/joint-chat-seasonal-v57a-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('Benefit baseline must be fixed local candidate')
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
        if not cls.database.startswith('hyjs_test_p4_benefit_alltypes_') or not cls.test_user.startswith('p4benefit_'):raise AssertionError('P4 confirmed benefit isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P4 confirmed benefit administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_fragment_equipment_gem_confirmed_bonus_atomic_and_cold(self):
        began=time.monotonic();self.start();uid='native-all-confirmed-benefit';login=self.login(uid);stamp=1800090000000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        call('CSPlayerCreate',{'playerName':'完整首掉福利','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=0,stamina=1000,warriorBagSize=1000,equipBagSize=1000,noviceGuideSchedule=8);self.write_profile(login,p,g)
        # This is an explicit isolated original-compatible2x window, never production.
        self.sql("UPDATE p4_copy_benefit_rules SET enabled=1,weekday_mask=127,start_minute=0,end_minute=1440,currency_factor=2,drop_factor=2,frozen=NULL;")
        info=call('CSActivityInfo',{'activityId':'allCopyDouble'},'SCActivityInfo');self.assertEqual(info.param,'2');self.assertLess(len(info.information),80)
        tasks=shared.original_table('CopyTask');missions={x['MissionId']:x for x in shared.original_table('CopyMission')};results=[]
        def run(tid):
            task=next(x for x in tasks if x['TaskId']==tid);mission=missions[task['MissionId']]
            p,g=self.profile(login);g['run']={'chapter':mission['ChapterId'],'mission':task['MissionId'],'passed':[x['TaskId'] for x in tasks if x['MissionId']==task['MissionId'] and x['TaskId']!=tid]};self.write_profile(login,p,g)
            start=call('CSCopyFightStart',{'chapterId':mission['ChapterId'],'missionId':task['MissionId'],'taskId':tid},'SCCopyFightStart')
            return {'fightId':start.fightId,'result':1,'battlePlayer':start.battlePlayer}
        for tid in ['20','28','92']:
            end=run(tid)
            if tid=='20':
                p,g=self.profile(login);p['equipBagSize']=1;self.write_profile(login,p,g);before=self.profile(login)
                self.assertEqual(self.stamped('CSCopyFightEnd',end,login,stamp+1)[0],10001);self.assertEqual(self.profile(login),before)
                p['equipBagSize']=1000;self.write_profile(login,p,g)
            stamp+=1;settle_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool: replies=list(pool.map(lambda _:self.call('CSCopyFightEnd',end,login,settle_stamp,'SCCopyFightEnd'),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));result=replies[0];p,g=self.profile(login)
            details={'task':tid,'8sameEnd':'one actual settlement/one independent bonus/cold'}
            if tid in ['20','92']:
                eid='equip_001' if tid=='20' else 'equip_004';gears=[x for x in result.gain.equips if x.equipId==eid]
                self.assertEqual(len(gears),2);self.assertEqual(len({x.guid for x in gears}),2)
                self.assertEqual(sum(x['equipId']==eid for x in g['equipment']),2)
                current=[x.guid for x in result.currentResource if x.id==eid];self.assertTrue(all(x.guid in current for x in gears))
                details.update(equipment=eid,twoActualUniqueGUIDs=True,currentResourceIncludesBonusGUID=True)
            if tid=='28':
                self.assertEqual(sum(x.num for x in result.gain.simps if x.id=='frag_hero_032'),2)
                self.assertEqual(next(x['num'] for x in g['items'] if x['id']=='frag_hero_032'),2);details['originalFragment32']=2
            if tid=='92':
                self.assertEqual(sum(x.num for x in result.gain.simps if x.id=='MineItem_029'),2)
                self.assertEqual(next(x['num'] for x in g['gems'] if x['id']=='MineItem_029'),2);details['actualMine29']=2
            saved=self.profile(login);self.stop();self.start();login=self.login(uid)
            self.assertEqual(self.call('CSCopyFightEnd',end,login,settle_stamp,'SCCopyFightEnd'),result);self.assertEqual(self.profile(login),saved)
            results.append(details)
        self.assertEqual(int(self.sql(f"SELECT COUNT(*) FROM p4_copy_benefit_grants WHERE player_id={login.playerId};").strip()),3)
        # Operator changes cannot alter a frozen actual window; closing stops new bonuses.
        self.sql("UPDATE p4_copy_benefit_rules SET currency_factor=0,drop_factor=0;")
        self.assertEqual(call('CSActivityInfo',{'activityId':'allCopyDouble'},'SCActivityInfo').param,'2')
        repeat=run('92');again=call('CSCopyFightEnd',repeat,'SCCopyFightEnd');self.assertFalse(again.gain.equips)
        self.assertEqual(next(x['num'] for x in self.profile(login)[1]['gems'] if x['id']=='MineItem_029'),2)
        self.sql("UPDATE p4_copy_benefit_rules SET enabled=0;")
        self.assertEqual(call('CSActivityInfo',{'activityId':'allCopyDouble'},'SCActivityInfo').endTime,0)
        saved=self.profile(login);self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSCopyFightEnd',repeat,login,stamp,'SCCopyFightEnd'),again);self.assertEqual(self.profile(login),saved)
        report={'date':'2026-10-14','actualOriginalHTTPAndInnoDB':'passed','results':results,'capacityFailure':'base plus bonus overflow rolls back complete first-drop/bonus/coin/win/frame and profile','windowFreeze':'live0factors ignored after actual freezing;shutdown fresh info closes;exact current cold old settlement preserved','repeatNewFight':'known original first equipment/gem not granted again','domain':'all7 types:item/frag/Mine/hero/expHero/equipment/treasure actual unique instances/8/cold;unknownPet/emptyGUID fail;native actual638-first source20/28/92','sharedHookSHA256':'82b79627f4838030ced965ba64445dfb2d713c4abbc05aa35f81b37ba05d16fe','sourceUnknown':'operator window unchanged; native explicit isolated2x window, original private timing and unknownSpecialOdds/Pet probabilities remain unresolved','preparation':'finite level100/stamina1000/prior encounter eligibility; wins are explicit protocol client-result fixtures, no actual gear/gem/fragment/experience source inserted; not real original engine orphone victory','productionModified':False,'phoneUI':False,'seconds':round(time.monotonic()-began,3),'sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'cleanup':'whole-life SQLfilelock/schema-user-grants/temp source-binary'}
        (shared.ROOT/'analysis/p4/copy-benefit-alltypes-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
