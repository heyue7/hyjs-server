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

OWNED = ['hyjs-server/service/p5GoldEggService.go', 'hyjs-server/service/p5GoldEgg_test.go', 'hyjs-server/constUtil/p5GoldEgg.go', 'hyjs-server/db/p5GoldEggRule.go', 'hyjs-server/db/sql/107_p5_gold_egg_rules.sql', 'hyjs-server/db/sql/107_p5_gold_egg_rules.seed.sql', 'hyjs-server/service/p5Service.go', 'hyjs-server/service/p4CommerceService.go', 'hyjs-server/service/p4ResourceExchangeService.go', 'hyjs-server/db/sql/83_p4_commerce_rules.seed.sql', 'analysis/p5/gold-egg-original-source-audit.json', 'analysis/p5/gold-egg-bag-original.il.txt', 'tests/test_p5_gold_egg_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_GOLD_EGG_MYSQL_TEST') == '1', 'Explicit locked isolated P5 gem supply')
class GoldEggMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_gold_egg_' + secrets.token_hex(6)
        cls.test_user = 'p5goldegg_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-gold-egg-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-gem-supply')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_GOLD_EGG_BASE', shared.ROOT / 'build/candidates/joint-natural-guild-v59-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        dependency=json.loads((shared.ROOT/'analysis/p4/resource-exchange-manifest.json').read_text())
        dependency_source=shared.ROOT/dependency['snapshot']
        for entry in dependency['owned']:
            rel=entry['path']
            if rel.startswith('hyjs-server/'):
                source_file=dependency_source/rel
                if hashlib.sha256(source_file.read_bytes()).hexdigest()!=entry['sha256']:raise AssertionError('Aggregate dependency SHA changed')
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
            if left.split() != ['0','0','0']: raise AssertionError('P5 gem supply schema/user/grants remain')
            report = shared.ROOT / 'analysis/p5/gold-egg-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_gold_egg_') or not cls.test_user.startswith('p5goldegg_'):raise AssertionError('P5 gem supply isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 gem supply administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_gold_egg_source_coin_original_bag_use_frozen_and_cold(self):
        began=time.monotonic();self.start();stamp=1800120000000;uid='natural-gold-egg-source';login=self.login(uid)
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        call('CSPlayerCreate',{'playerName':'原失落金蛋来源','heroId':'hero_043'},'SCPlayerCreate')
        missions=shared.original_table('CopyMission');p,g=self.profile(login);p.update(level=100,coin=0,noviceGuideSchedule=8)
        g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        natural_coin=0;box_count=0
        for chapter in range(27,0,-1):
            for box in range(3):
                gain=call('CSCopyGetStarAward',{'chapterId':str(chapter),'starIndex':box},'SCCopyGetStarAward').gain
                natural_coin+=sum(x.num for x in gain.simps if x.id=='coin');box_count+=1
            if natural_coin>=12000000:break
        self.assertGreaterEqual(natural_coin,12000000);self.assertEqual(self.profile(login)[0]['coin'],natural_coin)
        month=time.strftime('%Y%m',time.gmtime(time.time()+8*3600));key='limitExchange_p4_resources_'+month;child='limitExchange_p4_gold_'+month
        tabs=call('CSActivityList',{},'SCActivityList');roots=[x for x in tabs.activityList if x.activityId.split('_')[0]=='limitExchange'];self.assertEqual(len(roots),1);self.assertEqual(roots[0].activityId,key)
        listing=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo');self.assertEqual(len(listing.shopItemList),17);self.assertEqual(len(set(x.index for x in listing.shopItemList)),17)
        egg=next(x for x in listing.shopItemList if x.getResource[0].id=='item_360');self.assertEqual((egg.name,egg.total,egg.costResource[0].num),('失落金蛋',1,4000000))
        buy={'activityId':key,'index':egg.index,'num':1,'costResources':egg.costResource,'getResources':egg.getResource};use={'costItem':{'id':'item_360','num':1},'getItemIndex':0}
        results=[];first_buy=None;first_use=None;first_buy_stamp=None;first_use_stamp=None
        options=[x for x in shared.original_table('ItemUse') if x['itemId']=='item_360'];allowed=[('MineItem_028,1' if x['id']=='881' and x['result']=='MineItem_028，1' else x['result']) for x in options]
        for day in range(3):
            if day:
                progress=json.loads(self.sql(f"SELECT payload FROM p4_commerce_states WHERE player_id={login.playerId} AND rule_id='{child}';").strip());progress['day']='20000101'
                raw=json.dumps(progress,separators=(',',':')).encode().hex();self.sql(f"UPDATE p4_commerce_states SET payload=X'{raw}' WHERE player_id={login.playerId} AND rule_id='{child}';")
            before=self.profile(login);stamp+=1;buy_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:bought=list(pool.map(lambda _:self.call('CSBuyActivityShop',buy,login,buy_stamp,'SCGetActivityShopReward'),range(8)))
            self.assertTrue(all(x==bought[0] for x in bought));self.assertEqual(self.profile(login)[0]['coin'],before[0]['coin']-4000000)
            stamp+=1;self.assertEqual(self.stamped('CSBuyActivityShop',buy,login,stamp)[0],10001)
            saved=self.profile(login);stamp+=1
            self.assertEqual(self.stamped('CSItemUse',{'costItem':{'id':'item_360','num':2},'getItemIndex':0},login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            stamp+=1;self.assertEqual(self.stamped('CSItemUse',dict(use,getItemIndex=1),login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            stamp+=1;use_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:opened=list(pool.map(lambda _:self.call('CSItemUse',use,login,use_stamp,'SCItemUse'),range(8)))
            self.assertTrue(all(x==opened[0] for x in opened));result=opened[0]
            self.assertEqual((result.cost.id,result.cost.num),('item_360',1))
            awards=[(x.id,x.num) for x in result.get.simps]+[(x.equipId,1) for x in result.get.equips]
            self.assertEqual(len(awards),1);self.assertIn(awards[0][0]+','+str(awards[0][1]),allowed)
            self.assertEqual(next((x['num'] for x in self.profile(login)[1]['items'] if x['id']=='item_360'),0),0)
            if day==0:first_buy=bought[0];first_use=result;first_buy_stamp=buy_stamp;first_use_stamp=use_stamp
            saved=self.profile(login);self.stop();self.start();login=self.login(uid)
            self.assertEqual(self.call('CSItemUse',use,login,use_stamp,'SCItemUse'),result);self.assertEqual(self.call('CSBuyActivityShop',buy,login,buy_stamp,'SCGetActivityShopReward'),bought[0]);self.assertEqual(self.profile(login),saved)
            results.append({'actualOriginalAward':awards[0],'sourceCostCoin':4000000,'8buyAndUse':'one exact current award/receipt/cold','dayEligibilityOnly':day>0})
        progress=json.loads(self.sql(f"SELECT payload FROM p4_commerce_states WHERE player_id={login.playerId} AND rule_id='{child}';").strip());progress['day']='20000101';raw=json.dumps(progress,separators=(',',':')).encode().hex();self.sql(f"UPDATE p4_commerce_states SET payload=X'{raw}' WHERE player_id={login.playerId} AND rule_id='{child}';")
        stamp+=1;self.assertEqual(self.stamped('CSBuyActivityShop',buy,login,stamp)[0],10001)
        rule=self.sql("SELECT LENGTH(frozen),input_hash FROM p5_gold_egg_rules WHERE id='lost-gold-egg-v1';").strip().split('\t');self.assertGreater(int(rule[0]),1000);self.assertEqual(len(rule[1]),64)
        self.sql("UPDATE p5_gold_egg_rules SET rules='[]' WHERE id='lost-gold-egg-v1';")
        self.assertEqual(self.call('CSItemUse',use,login,first_use_stamp,'SCItemUse'),first_use)
        self.sql("UPDATE p5_gold_egg_rules SET enabled=0 WHERE id='lost-gold-egg-v1';")
        self.assertFalse(any(x.getResource[0].id=='item_360' for x in call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo').shopItemList))
        self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSItemUse',use,login,first_use_stamp,'SCItemUse'),first_use);self.assertEqual(self.call('CSBuyActivityShop',buy,login,first_buy_stamp,'SCGetActivityShopReward'),first_buy)
        self.assertEqual(self.sql("SELECT enabled FROM p5_gold_egg_rules WHERE id='lost-gold-egg-v1';").strip(),'0')
        report={'date':'2026-10-14','policy':'gold-egg-relative-weights-v1/gold-egg-mainline-coin-exchange-v1','results':results,'sourceOriginalStarBoxes':box_count,'sourceNaturalCoin':natural_coin,'sourceActualPaidCoin':12000000,'preparation':'finite level100/guide8/original historical stars; only commerce previous-day eligibility cursors shifted; no coin/egg/award/GUID inserted','originalUse':'category23 BagMemberDetail original direct one CSItemUse, no choice; actual awards within all54 exact original rows; only original881 punctuation normalized','privateUnknown':'original production/rates/pity and param1..13 meanings unknown; named relative weights does not claim original probability restoration','oneType23':'17 unique current original-product indices, root exact CoversRed patch; phone pending root','productionModified':False,'phoneUI':False,'seconds':round(time.monotonic()-began,3),'sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest()}
        (shared.ROOT/'analysis/p5/gold-egg-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
