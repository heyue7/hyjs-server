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

OWNED = ['hyjs-server/service/p15SeasonalService.go', 'hyjs-server/service/p15Seasonal_test.go', 'hyjs-server/service/p4ActivityService.go', 'hyjs-server/service/p4CommerceService.go', 'hyjs-server/service/p4Service.go', 'hyjs-server/service/p4Welfare_test.go', 'hyjs-server/constUtil/p15Seasonal.go', 'hyjs-server/db/p15SeasonalBundle.go', 'hyjs-server/db/sql/102_p15_seasonal_bundles.sql', 'hyjs-server/db/sql/102_p15_seasonal_bundles.seed.sql', 'tests/test_p15_seasonal_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P15_SEASONAL_MYSQL_TEST') == '1', 'Explicit locked isolated P9 market')
class SeasonalMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p15_seasonal_' + secrets.token_hex(6)
        cls.test_user = 'p15season_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p15-seasonal-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-seasonal')
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")
        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0','0','0']: raise AssertionError('P9 market schema/user/grants remain')
            report = shared.ROOT / 'analysis/p15/seasonal-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P15_SEASONAL_BASE', shared.ROOT / 'build/candidates/joint-auto-recruit-v54-20261014/hyjs-server')).resolve()
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
        if not cls.database.startswith('hyjs_test_p15_seasonal_') or not cls.test_user.startswith('p15season_'):raise AssertionError('P9 market isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P9 market administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_nested_pages_real_login_sweep_material_exchange_and_cold(self):
        began=time.monotonic();self.start();uid='seasonal-native';login=self.login(uid);stamp=1800040000000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def fail(name,fields):
            nonlocal stamp
            stamp+=1;before=self.profile(login);self.assertEqual(self.stamped(name,fields,login,stamp)[0],10001);self.assertEqual(self.profile(login),before)
        call('CSPlayerCreate',{'playerName':'原节日材料业务','heroId':'hero_043'},'SCPlayerCreate')
        login=self.login(uid);call('CSPlayerLogin',{},'SCPlayerLogin')
        root=call('CSActivityList',{},'SCActivityList')
        parents={x.activityId.split('_')[0]:x.activityId for x in root.activityList if x.activityId.startswith(('festival_p4_','update_p4_','celebrationActivity_p4_'))}
        self.assertEqual(set(parents),{'festival','update','celebrationActivity'})
        self.assertFalse(any(x.activityId.startswith(('totalLoginFestival','limitExchangeFestival')) for x in root.activityList))
        festival=call('CSActivityList',{'activityId':parents['festival']},'SCActivityList')
        update=call('CSActivityList',{'activityId':parents['update']},'SCActivityList')
        self.assertEqual((len(festival.activityList),len(update.activityList)),(2,1));self.assertTrue(update.activityList[0].activityId.startswith('totalLoginUpdate_'))
        login_id=festival.activityList[0].activityId;exchange_id=festival.activityList[1].activityId
        login_info=call('CSActivityInfo',{'activityId':login_id},'SCActivityInfo')
        self.assertEqual(login_info.itemList[0].status,1)
        fields={'activityId':login_id,'index':1,'getResources':[{'id':x.id,'num':x.num} for x in login_info.itemList[0].rewardList]}
        stamp+=1;claim_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSActivityReward',fields,login,claim_stamp,'SCActivityReward'),range(8)))
        self.assertEqual(sum(len(x.get.simps)>0 for x in replies),1)
        p,g=self.profile(login);self.assertEqual({x['id']:x['num'] for x in g['items']}['item_124'],15)
        # Only finite historical stars and eligibility levels are prepared. Currency,
        # flower, pet and specialization-stone inventory comes from native operations.
        p.update(level=90,noviceGuideSchedule=8)
        missions=shared.original_table('CopyMission')
        g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions if x['ChapterId']=='27']+[{'chapterId':'1','missionId':'2','star':5,'challengeTimes':0}]
        self.write_profile(login,p,g)
        for box in range(3):call('CSCopyGetStarAward',{'chapterId':'27','starIndex':box},'SCCopyGetStarAward')
        coin_source=self.profile(login)[0]['coin'];self.assertGreaterEqual(coin_source,200000)
        pet=call('CSPetFirstRecruit',{},'SCPetFirstRecruit').get.pets[0]
        grown=call('CSPetLevelup',{'guid':pet.guid,'cost':[{'id':'item_124','num':15}]},'SCPetLevelup')
        self.assertGreater(grown.pet.level,1);self.assertEqual({x['id']:x['num'] for x in self.profile(login)[1]['items']}.get('item_124',0),0)
        celebration=call('CSActivityInfo',{'activityId':parents['celebrationActivity']},'SCActivityInfo')
        self.assertEqual(celebration.itemList[0].status,1)
        reward_fields={'activityId':parents['celebrationActivity'],'index':1,'getResources':[{'id':x.id,'num':x.num} for x in celebration.itemList[0].rewardList]}
        gained=call('CSActivityReward',reward_fields,'SCActivityReward');self.assertEqual([(x.id,x.num) for x in gained.get.simps],[('item_005',10),('item_047',5)])
        info=call('CSActivityShopInfo',{'activityId':exchange_id},'SCActivityShopInfo');slot=info.shopItemList[0]
        buy={'activityId':exchange_id,'index':slot.index,'num':1,'costResources':[{'id':x.id,'num':x.num} for x in slot.costResource],'getResources':[{'id':x.id,'num':x.num} for x in slot.getResource]}
        before_coin=self.profile(login)[0]['coin'];stamp+=1;buy_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSBuyActivityShop',buy,login,buy_stamp,'SCGetActivityShopReward'),range(8)))
        self.assertTrue(all(x==replies[0] for x in replies));purchased=replies[0]
        self.assertEqual(self.profile(login)[0]['coin'],before_coin-200000);self.assertEqual({x['id']:x['num'] for x in self.profile(login)[1]['items']}['item_470'],10)
        # Query and repeated same-day login must leave the three-day goal at one.
        update_info=call('CSActivityInfo',{'activityId':update.activityList[0].activityId},'SCActivityInfo')
        self.assertEqual((update_info.itemList[0].left,update_info.itemList[0].status),(1,0));login=self.login(uid);call('CSPlayerLogin',{},'SCPlayerLogin')
        self.assertEqual(call('CSActivityInfo',{'activityId':update.activityList[0].activityId},'SCActivityInfo').itemList[0].left,1)
        self.sql("UPDATE p15_seasonal_bundles SET children='[]',title='edited' WHERE kind='festival';")
        self.sql(f"UPDATE p1_period_instances SET rewards='coin,999999' WHERE id='{login_id}';")
        self.assertEqual(call('CSActivityList',{'activityId':parents['festival']},'SCActivityList'),festival)
        self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSBuyActivityShop',buy,login,buy_stamp,'SCGetActivityShopReward'),purchased)
        self.assertEqual(len(self.call('CSActivityReward',fields,login,claim_stamp,'SCActivityReward').get.simps),0)
        pet_list=call('CSPetList',{},'SCPetList');self.assertEqual(pet_list.petList[0],grown.pet)
        self.sql("UPDATE p15_seasonal_bundles SET enabled=0 WHERE kind='festival';")
        fail('CSActivityList',{'activityId':parents['festival']});fail('CSActivityInfo',{'activityId':login_id});fail('CSActivityReward',fields)
        fail('CSBuyActivityShop',dict(buy,index=1))
        new_root=call('CSActivityList',{},'SCActivityList');self.assertNotIn(parents['festival'],[x.activityId for x in new_root.activityList])
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='p15_seasonal_bundles';").strip(),'InnoDB')
        report={'date':'2026-10-14','types':[21,25,26],'policy':'seasonal-material-v1','rootAndNestedNativePrefixes':'passed','sourceChain':'type21 original ActivityInfo/Reward→true login breakthrough/talent materials; Festival true login15 flowers→original first002→level2; chapter27 actual starbox coins→exchange item470','originalDays':'Update target3 keeps real current login1; three-day eligibility uses domain event fixture only','concurrency8':'one P1 award/seven empty, one commerce transaction/eight same receipt','frozenHotEdit':'passed','cold':'claimed empty/query status, commerce exact receipt, actual grown pet','disabledParentDirectChild':'passed','coinsNeverInserted':True,'preparation':'finite level90/guide8/star prerequisites, no pets/currency/materials inserted','originalUnknown':'private-server dates/goals/rewards unknown; named local monthly+7day instance','phoneUI':False,'productionModified':False,'seconds':round(time.monotonic()-began,3),'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'P1Integration':'root child filtering and same-transaction parent guard applied in disposable fixed source','ownedSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'cleanup':'locked class cleanup dedicated schema/user/grants and temporary build'}
        report_path=shared.ROOT/'analysis/p15/seasonal-mysql-validation.json';report_path.parent.mkdir(parents=True,exist_ok=True);report_path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
