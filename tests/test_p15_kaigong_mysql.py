"""Original PetShop/Decompose binary frames using locked disposable InnoDB."""
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

OWNED = ['hyjs-server/service/p15SeasonalService.go', 'hyjs-server/service/p15Seasonal_test.go', 'hyjs-server/service/p4ActivityService.go', 'hyjs-server/service/p4CommerceService.go', 'hyjs-server/service/p4Service.go', 'hyjs-server/service/p4Welfare_test.go', 'hyjs-server/constUtil/p15Seasonal.go', 'hyjs-server/db/p15SeasonalBundle.go', 'hyjs-server/db/sql/102_p15_seasonal_bundles.sql', 'hyjs-server/db/sql/102_p15_seasonal_bundles.seed.sql', 'hyjs-server/constUtil/p15KaiGong.go', 'hyjs-server/service/p15KaiGong_test.go', 'hyjs-server/db/sql/80_p1_period_instances.seed.sql', 'tests/test_p15_kaigong_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P15_KAIGONG_MYSQL_TEST') == '1', 'Explicit locked isolated P9 market')
class KaiGongMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p15_kaigong_' + secrets.token_hex(6)
        cls.test_user = 'p15kaigong_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p15-kaigong-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-kaigong')
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")
        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0','0','0']: raise AssertionError('P9 market schema/user/grants remain')
            report = shared.ROOT / 'analysis/p15/kaigong-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P15_KAIGONG_BASE', shared.ROOT / 'build/candidates/joint-auto-recruit-v54-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('Market baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for rel in OWNED:
            if rel.startswith('hyjs-server/'):
                target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(shared.ROOT/rel,target)
        cls.frozen_source=source;cls.baseline_source=str(base.relative_to(shared.ROOT))
        (Path(cls.temp.name)/'analysis').symlink_to(shared.ROOT/'analysis',target_is_directory=True)
        cls.source_shas={str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob('*.go'))}
        period=source/'service/periodService.go'
        text=period.read_text()
        text=text.replace('!s.IsP1Activity(cfg.ID) {','!s.IsP1Activity(cfg.ID) || isP15SeasonalChild(cfg.ID) {')
        needle='err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {'
        text=text.replace(needle,needle+'\n\t\tif err := s.p15SeasonalGuard(tx,key,time.Now()); err != nil { return err }',1)
        period.write_text(text)
        cls.source_shas['service/periodService.go']=hashlib.sha256(period.read_bytes()).hexdigest()
        result=subprocess.run(['rtk','proxy','go','build' ,'-o',cls.binary,'.'],cwd=source,capture_output=True,timeout=180)
        if result.returncode: raise AssertionError((result.stdout+result.stderr).decode(errors='replace')[-4000:])

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p15_kaigong_') or not cls.test_user.startswith('p15kaigong_'):raise AssertionError('P9 market isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P9 market administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original32_actual_ten_sweeps_source_replay_and_cold(self):
        began=time.monotonic();self.start();uid='native-kaigong';login=self.login(uid);stamp=1800050000000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        call('CSPlayerCreate',{'playerName':'原攻略福利来源','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=75,noviceGuideSchedule=8)
        missions=shared.original_table('CopyMission')
        g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions if x['ChapterId']=='27']+[{'chapterId':'1','missionId':'2','star':5,'challengeTimes':0}]
        self.write_profile(login,p,g)
        for box in range(3):call('CSCopyGetStarAward',{'chapterId':'27','starIndex':box},'SCCopyGetStarAward')
        stamina=self.profile(login)[0]['stamina']
        if stamina<100:
            required=(100-stamina+24)//25
            call('CSShopBuy',{'shopType':1,'shopItemId':'itemShopId_2','buyCount':required},'SCShopBuy')
        key=next(x.activityId for x in call('CSActivityList',{},'SCActivityList').activityList if x.activityId.startswith('kaiGongActivity_p4_'))
        initial=call('CSActivityInfo',{'activityId':key},'SCActivityInfo');self.assertEqual((initial.itemList[0].left,initial.itemList[0].total,initial.itemList[0].status),(0,10,0))
        sweep={'chapterId':'1','missionId':'2'};stamp+=1;first_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSCopySweep',sweep,login,first_stamp,'SCCopySweep'),range(8)))
        self.assertTrue(all(x==replies[0] for x in replies));self.assertEqual(call('CSActivityInfo',{'activityId':key},'SCActivityInfo').itemList[0].left,1)
        for _ in range(8):call('CSCopySweep',sweep,'SCCopySweep')
        ninth=call('CSActivityInfo',{'activityId':key},'SCActivityInfo');self.assertEqual((ninth.itemList[0].left,ninth.itemList[0].status),(9,0))
        reward={'activityId':key,'index':1,'getResources':[{'id':x.id,'num':x.num} for x in ninth.itemList[0].rewardList]}
        before=self.profile(login);self.assertEqual(self.stamped('CSActivityReward',reward,login,stamp+1)[0],10001);self.assertEqual(self.profile(login),before);stamp+=1
        self.assertEqual(call('CSActivityInfo',{'activityId':key},'SCActivityInfo').itemList[0].left,9)
        call('CSCopySweep',sweep,'SCCopySweep');earned=call('CSActivityInfo',{'activityId':key},'SCActivityInfo');self.assertEqual(earned.itemList[0].status,1)
        stamp+=1;claim_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSActivityReward',reward,login,claim_stamp,'SCActivityReward'),range(8)))
        gains=[x for x in replies if len(x.get.simps)>0];self.assertEqual(len(gains),1);self.assertEqual([(x.id,x.num) for x in gains[0].get.simps],[('item_139',5),('item_150',1)])
        # The reward is an original random certificate package, not an enchantment stone.
        box={'costItem':{'id':'item_150','num':1}};stamp+=1;box_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:opened=list(pool.map(lambda _:self.call('CSItemUse',box,login,box_stamp,'SCItemUse'),range(8)))
        self.assertTrue(all(x==opened[0] for x in opened));certificate=opened[0].get.simps[0]
        self.assertIn(certificate.id,['item_140','item_142','item_144','item_146','item_148']);self.assertEqual(certificate.num,1)
        saved=self.profile(login);stacks={x['id']:x['num'] for x in saved[1]['items']};self.assertEqual(stacks['item_139'],5);self.assertEqual(stacks.get('item_150',0),0);self.assertEqual(stacks[certificate.id],1)
        self.sql(f"UPDATE p1_period_instances SET rewards='coin,999999' WHERE id='{key}';")
        self.stop();self.start();login=self.login(uid)
        self.assertEqual(len(self.call('CSActivityReward',reward,login,claim_stamp,'SCActivityReward').get.simps),0)
        self.assertEqual(self.call('CSItemUse',box,login,box_stamp,'SCItemUse'),opened[0]);self.assertEqual(self.profile(login),saved);self.assertEqual(call('CSActivityInfo',{'activityId':key},'SCActivityInfo').itemList[0].status,2)
        self.sql(f"UPDATE p1_period_instances SET enabled=0 WHERE id='{key}';")
        before=self.profile(login);self.assertEqual(self.stamped('CSActivityInfo',{'activityId':key},login,stamp+1)[0],10001);self.assertEqual(self.profile(login),before)
        report={'date':'2026-10-14','type':32,'policy':'kaigong-mainline-supply-v1','originalUIProtocol':'Panel32 uses RechargeSurprise; original Show CSActivityInfo/Reward with activityId, independent CSKaiGong caller not found and remains unproven','actualSource':'10 original sweep successes, first8 same stamp one copy fact; chapter27 starbox diamonds fund any missing stamina through original shop','ninthAndQueryGate':'ninth fails claim, query does not add wins','concurrentClaim8':'one gain/seven empty','gain':'item139x5/item150x1→original ItemUse633..637 five equal-weight basic certificate→one actual certificate', 'certificateID':certificate.id, 'eightBoxReplayCold':'same actual draw, one consume, exact cold receipt','frozenCold':'mutated live rewards retain original frozen/claimed status and complete profile','disabledNewQuery':'passed','preparation':'level75/guide8 and prior chapter1/27 stars; no materials/currency/stamina injected','sourceUnknown':'original threshold/reward/window unknown, explicitly local month10mainline and7day earned grace','phoneUI':False,'productionModified':False,'seconds':round(time.monotonic()-began,3),'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'ownedSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'cleanup':'locked isolated schema/user/grants and temp source/binary removed'}
        (shared.ROOT/'analysis/p15/kaigong-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
