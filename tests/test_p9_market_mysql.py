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

OWNED = [
 'hyjs-server/constUtil/p9Market.go', 'hyjs-server/db/p9PetMarketRule.go',
 'hyjs-server/db/p9PetMarketState.go', 'hyjs-server/db/p9PetRetired.go',
 'hyjs-server/db/sql/88_p9_pet_retired.sql', 'hyjs-server/db/sql/101_p9_pet_market_rules.sql',
 'hyjs-server/db/sql/101_p9_pet_market_states.sql', 'hyjs-server/db/sql/101_p9_pet_market_rules.seed.sql',
 'hyjs-server/service/p9PetMarketService.go', 'hyjs-server/service/p9PetDecomposeService.go',
 'hyjs-server/service/p9PetRecycleService.go', 'hyjs-server/service/p9PetMarket_test.go',
 'hyjs-server/service/p9PetRecycle_test.go', 'tests/test_p9_market_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P9_MARKET_MYSQL_TEST') == '1', 'Explicit locked isolated P9 market')
class PetMarketMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p9_market_' + secrets.token_hex(6)
        cls.test_user = 'p9market_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p9-market-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-market')
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")
        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0','0','0']: raise AssertionError('P9 market schema/user/grants remain')
            report = shared.ROOT / 'analysis/p9/market-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P9_MARKET_BASE', shared.ROOT / 'build/candidates/joint-auto-recruit-v54-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('Market baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for rel in OWNED:
            if rel.startswith('hyjs-server/'):
                target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(shared.ROOT/rel,target)
        cls.frozen_source=source;cls.baseline_source=str(base.relative_to(shared.ROOT))
        (Path(cls.temp.name)/'analysis').symlink_to(shared.ROOT/'analysis',target_is_directory=True)
        cls.source_shas={str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob('*.go'))}
        result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=source,capture_output=True,timeout=180)
        if result.returncode: raise AssertionError((result.stdout+result.stderr).decode(errors='replace')[-4000:])

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p9_market_') or not cls.test_user.startswith('p9market_'):raise AssertionError('P9 market isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P9 market administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_actual_source_decompose_market_growth_refund_stock_restart(self):
        began=time.monotonic();self.start();uid='actual-pet-market';login=self.login(uid)
        stamp=1800030000000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def fail(name,fields):
            nonlocal stamp
            stamp+=1;before=self.profile(login);self.assertEqual(self.stamped(name,fields,login,stamp)[0],10001);self.assertEqual(self.profile(login),before)
        def race(name,fields,response):
            nonlocal stamp
            stamp+=1
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                replies=list(pool.map(lambda _:self.call(name,fields,login,stamp,response),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));return replies[0],stamp
        call('CSPlayerCreate',{'playerName':'原精灵币完整链','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=75,noviceGuideSchedule=8)
        missions=shared.original_table('CopyMission')
        g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions if x['ChapterId'] in ('26','27')]
        self.write_profile(login,p,g)
        for chapter in ('26','27'):
            for box in range(3):call('CSCopyGetStarAward',{'chapterId':chapter,'starIndex':box},'SCCopyGetStarAward')
        source_diamonds=self.profile(login)[0]['diamond'];self.assertGreaterEqual(source_diamonds,1500)
        cfg=json.loads(self.sql("SELECT CONVERT(input USING utf8mb4) FROM p9_pet_market_rules WHERE id='pet-market';").strip())
        self.assertEqual(len(cfg['offers']),112);self.assertEqual(sum(x['resource'].startswith('frag_pet_') for x in cfg['offers']),78)
        cfg['offers']=[x for x in cfg['offers'] if x['resource'] in ('item_124','frag_pet_001')];cfg['slots']=2
        raw=json.dumps(cfg,ensure_ascii=False,separators=(',',':')).encode().hex()
        self.sql(f"UPDATE p9_pet_market_rules SET input=X'{raw}' WHERE id='pet-market';")
        first=call('CSPetFirstRecruit',{},'SCPetFirstRecruit').get.pets[0]
        free=call('CSPetRecruit',{'free':1,'numType':1},'SCPetRecruit').get.pets[0]
        call('CSFormationOn',{'formatPos':1,'id':free.id,'guid':free.guid},'SCFormationOn')
        fail('CSPetDecompose',{'guids':[first.guid,free.guid]})
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p9_pet_retired;').strip(),'0')
        # Restore only the formation prerequisite; both pet instances came from original recruit.
        p,g=self.profile(login)
        for formation in g['formation']:formation['things']=[x for x in formation['things'] if x.get('guid')!=free.guid]
        self.write_profile(login,p,g)
        fail('CSPetDecompose',{'guids':[first.guid,first.guid]})
        fail('CSPetDecompose',{'guids':[first.guid,'foreign-guid']})
        decomposed,decompose_stamp=race('CSPetDecompose',{'guids':[first.guid]},'SCPetDecompose')
        self.assertEqual([(x.id,x.num) for x in decomposed.get.simps],[('coinpet',100)])
        self.assertEqual((decomposed.cost[0].guid,len(decomposed.get.pets)),(first.guid,0))
        self.assertEqual(self.profile(login)[0]['coinpet'],100)
        self.assertEqual(self.sql("SELECT CONCAT(coinpet,',',coin) FROM p9_pet_retired;").strip(),'100,0')
        listing=call('CSPetShopProductList',{},'SCPetShopProductList');self.assertEqual(listing.refreshTimes,10)
        item=next(x for x in listing.shopItemList if x.item[0].id=='item_124');buy={'shopItemId':item.id,'buyCount':1}
        fail('CSPetShopBuy',{'shopItemId':item.id,'buyCount':0});fail('CSPetShopBuy',{'shopItemId':'fake','buyCount':1})
        purchased,buy_stamp=race('CSPetShopBuy',buy,'SCPetShopBuy');self.assertEqual((purchased.cost[0].id,purchased.cost[0].num),('coinpet',20))
        self.assertEqual(self.profile(login)[0]['coinpet'],80);fail('CSPetShopBuy',buy)
        coin_before=self.profile(login)[0]['coin']
        upgraded=call('CSPetLevelup',{'guid':free.guid,'cost':[{'id':'item_124','num':1}]},'SCPetLevelup')
        self.assertGreater(upgraded.pet.exp,0)
        refunded,_=race('CSPetDecompose',{'guids':[free.guid]},'SCPetDecompose')
        quality={x['id']:x['quality'] for x in shared.original_table('Pet')}[free.id]
        self.assertEqual(self.profile(login)[0]['coinpet'],80+cfg['decompose'][quality]);self.assertEqual(self.profile(login)[0]['coin'],coin_before)
        self.assertEqual({x['id']:x['num'] for x in self.profile(login)[1]['items']}['item_124'],10)
        self.assertEqual(len(refunded.get.pets),0)
        self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSPetDecompose',{'guids':[first.guid]},login,decompose_stamp,'SCPetDecompose'),decomposed)
        self.assertEqual(self.call('CSPetShopBuy',buy,login,buy_stamp,'SCPetShopBuy'),purchased)
        fresh,refresh_stamp=race('CSRefreshPetShop',{},'SCPetShopProductList');self.assertEqual(fresh.refreshTimes,9)
        self.assertEqual(self.profile(login)[0]['diamond'],source_diamonds-150)
        fail('CSPetShopBuy',buy);self.assertEqual(self.stamped('CSPetShopBuy',buy,login,buy_stamp)[0],10001)
        self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSRefreshPetShop',{},login,refresh_stamp,'SCPetShopProductList'),fresh)
        for _ in range(9):fresh=call('CSRefreshPetShop',{},'SCPetShopProductList')
        self.assertEqual(fresh.refreshTimes,0);self.assertEqual(self.profile(login)[0]['diamond'],source_diamonds-1500)
        fail('CSRefreshPetShop',{})
        self.sql(f"UPDATE p9_pet_market_states SET week_key='20261005' WHERE player_id={login.playerId};")
        rollover=call('CSPetShopProductList',{},'SCPetShopProductList');self.assertEqual(rollover.refreshTimes,10)
        self.assertEqual(self.stamped('CSRefreshPetShop',{},login,refresh_stamp)[0],10001)
        frozen_hash=self.sql(f"SELECT input_hash FROM p9_pet_market_states WHERE player_id={login.playerId};").strip()
        self.sql("UPDATE p9_pet_market_rules SET input='{}' WHERE id='pet-market';")
        self.assertEqual(call('CSPetShopProductList',{},'SCPetShopProductList'),rollover)
        self.sql("UPDATE p9_pet_market_rules SET enabled=0 WHERE id='pet-market';");fail('CSPetShopProductList',{})
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('p9_pet_market_rules','p9_pet_market_states','p9_pet_retired');").strip(),'InnoDB')
        report={'date':'2026-10-14','policy':'pet-market-v1','originalFragments':78,'fullOffers':112,'originalRefresh':'diamond150 x weekly10','sourceDiamonds':source_diamonds,'actualSpentDiamond':1500,'petCoinSource':'original first002 and ordinary free original pet only','chain':'real recruit→decompose coinpet→buy item124→actual level consume→exact resources refund','concurrent8':'decompose, buy, refresh one mutation each','mixedGuardedForeignDuplicateRollback':'passed','retiredCoinpetSeparateCoin':'passed','stockEpochOldSKUAndProjectionRefusal':'passed','coldReceipts':'decompose/buy/current refresh exact','weeklyReset':'only isolated prior-week cursor prerequisite','frozenLiveEdit':'passed','disabledNewQuery':'passed','inputHash':frozen_hash,'originalHTTPInnoDB':True,'phoneUI':False,'productionModified':False,'preparation':'level75 and chapter26/27 prior stars, finite operator original2-product window; no pet/coinpet/material/diamonds inserted','sourceUnknown':'original private server catalog/prices/coinpet value unknown, explicit configurable local20/100/500 and prices','seconds':round(time.monotonic()-began,3),'baseline':self.baseline_source,'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'ownedSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'cleanup':'class cleanup removes dedicated schema/user/grants and temporary frozen build'}
        (shared.ROOT/'analysis/p9/market-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
