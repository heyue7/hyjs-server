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

OWNED = ['hyjs-server/service/p15RedOSSupplyService.go', 'hyjs-server/service/p15RedOSSupply_test.go', 'hyjs-server/constUtil/p15RedOSSupplies.go', 'hyjs-server/service/p4CommerceService.go', 'hyjs-server/service/p4ResourceExchangeService.go', 'hyjs-server/db/sql/83_p4_commerce_rules.seed.sql', 'analysis/p15/red-three-source-audit.json', 'analysis/p15/red-three-fragment-consumer-original.il.txt', 'tests/test_p15_red_os_supply_mysql.py','hyjs-server/service/redP2Service.go','hyjs-server/service/redCombineReceiptLifecycle_test.go']

@unittest.skipUnless(os.environ.get('HYJS_P15_RED_OS_MYSQL_TEST') == '1', 'Explicit locked isolated P15 three reds')
class RedOSSupplyMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p15_red_os_' + secrets.token_hex(6)
        cls.test_user = 'p15os_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p15-red-os-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-gem-supply')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P15_RED_OS_BASE', shared.ROOT / 'build/candidates/joint-catalogue-car-v65-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p8/ingredient-supply-manifest.json']:
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
        if needle in old:target.write_text(old.replace(needle,replacement))
        elif 'P4ResourceExchangeCoversRed(ctx' not in old:raise AssertionError('Unexpected root hook baseline')
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
            if left.split() != ['0','0','0']: raise AssertionError('P15 three reds schema/user/grants remain')
            report = shared.ROOT / 'analysis/p15/red-three-supply-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p15_red_os_') or not cls.test_user.startswith('p15os_'):raise AssertionError('P15 three reds isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P15 three reds administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_three_red_source_150_combine_formation_transport_recycle_cold(self):
        began=time.monotonic();self.start();stamp=1800170000000;missions=shared.original_table('CopyMission');heroes={x['id']:x for x in shared.original_table('Hero')};results=[]
        from datetime import datetime,timezone,timedelta
        month=datetime.now(timezone(timedelta(hours=8))).strftime('%Y%m');key='limitExchange_p4_resources_'+month;child='limitExchange_p4_red_os_'+month
        king_before=self.sql("SELECT CONCAT(id,'|',resource_id,'|',amount,'|',weight) FROM gameplay_pool_rewards WHERE pool_id='recruit_3' ORDER BY id;")
        for hid in ['hero_077','hero_078','hero_079']:
            uid='actual-red-os-'+hid;login=self.login(uid)
            def call(name,fields,response):
                nonlocal stamp
                stamp+=1;return self.call(name,fields,login,stamp,response)
            call('CSPlayerCreate',{'playerName':'原三红来源'+hid[-3:],'heroId':'hero_043'},'SCPlayerCreate')
            p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,stamina=1000,equipBagSize=1000,warriorBagSize=1000,noviceGuideSchedule=8)
            g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions];g['copy']['passedChapterInfo']=[];g['run']=None;self.write_profile(login,p,g)
            if not results:
                before=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo');oldindices={item.index:item.name for item in before.shopItemList}
                self.assertFalse(any(any(x.id in ['frag_hero_077','frag_hero_078','frag_hero_079'] for x in item.getResource) for item in before.shopItemList))
                self.assertEqual(self.sql(f"SELECT enabled,OCTET_LENGTH(COALESCE(frozen,'')) FROM p4_commerce_rules WHERE id='{child}';").strip(),'0\t0')
                self.sql(f"UPDATE p4_commerce_rules SET enabled=1 WHERE id='{child}';")
            listing=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo');product=next(item for item in listing.shopItemList if item.getResource[0].id=='frag_'+hid)
            for item in listing.shopItemList:
                if item.index in oldindices:self.assertEqual(item.name,oldindices[item.index])
            self.assertEqual((product.getResource[0].num,product.costResource[0].num),(5,500000))
            natural=0;stars=0
            for chapter in reversed(shared.original_table('CopyChapter')):
                for box in range(3):
                    reward=call('CSCopyGetStarAward',{'chapterId':chapter['ChapterId'],'starIndex':box},'SCCopyGetStarAward');natural+=sum(x.num for x in reward.gain.simps if x.id=='coin');stars+=1
                    if natural>=15100000:break
                if natural>=15100000:break
            self.assertGreaterEqual(natural,15100000);self.assertEqual(self.profile(login)[0]['coin'],natural)
            req={'activityId':key,'index':product.index,'num':3,'costResources':product.costResource,'getResources':product.getResource};shifts=0;last_buy=None;last_stamp=None
            for day in range(10):
                if day:
                    payload=json.loads(self.sql(f"SELECT CONVERT(payload USING utf8mb4) FROM p4_commerce_states WHERE player_id={login.playerId} AND rule_id='{child}';").strip());payload['day']='20000101';raw=json.dumps(payload,ensure_ascii=False,separators=(',',':')).encode().hex();self.sql(f"UPDATE p4_commerce_states SET payload=X'{raw}' WHERE player_id={login.playerId} AND rule_id='{child}';");shifts+=1
                stamp+=1;last_stamp=stamp
                if day==9:
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSBuyActivityShop',req,login,last_stamp,'SCGetActivityShopReward'),range(8)))
                    self.assertTrue(all(x==replies[0] for x in replies));last_buy=replies[0]
                else:last_buy=self.call('CSBuyActivityShop',req,login,last_stamp,'SCGetActivityShopReward')
                self.assertEqual(sum(x.num for x in last_buy.get.simps if x.id=='frag_'+hid),15)
                self.assertEqual(next(x['num'] for x in self.profile(login)[1]['items'] if x['id']=='frag_'+hid),15*(day+1))
                saved=self.profile(login);stamp+=1;self.assertEqual(self.stamped('CSBuyActivityShop',req,login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            self.assertEqual(self.profile(login)[0]['coin'],natural-15000000)
            combine={'targetId':hid};stamp+=1;combine_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:combined=list(pool.map(lambda _:self.call('CSCombine',combine,login,combine_stamp,'SCCombine'),range(8)))
            self.assertTrue(all(x==combined[0] for x in combined));war=combined[0].get.wars[0];self.assertEqual(war.id,hid);self.assertEqual([(x.id,x.num) for x in combined[0].cost],[('frag_'+hid,150)]);self.assertTrue(war.guid)
            self.assertEqual(next((x['num'] for x in self.profile(login)[1]['items'] if x['id']=='frag_'+hid),0),0)
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);self.assertEqual(self.call('CSCombine',combine,login,combine_stamp,'SCCombine'),combined[0]);self.assertEqual(self.call('CSBuyActivityShop',req,login,last_stamp,'SCGetActivityShopReward'),last_buy)
            donor=call('CSShopBuy',{'shopType':1,'shopItemId':'itemShopId_5','buyCount':1},'SCShopBuy').get.wars[0]
            grown=call('CSWarriorStrengthen',{'guid':war.guid,'cost':[{'id':donor.id,'guid':donor.guid}]},'SCWarriorStrengthen')
            actual_growth_coin=sum(x.num for x in grown.cost if x.id=='coin')
            saved=self.profile(login);stamp+=1;self.assertEqual(self.stamped('CSCombine',combine,login,combine_stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            call('CSFormationOn',{'formatPos':2,'id':hid,'guid':war.guid},'SCFormationOn')
            call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter');fight=call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'1'},'SCCopyFightStart');actor=next(x for x in fight.battlePlayer.formation if x.guid==war.guid)
            self.assertEqual(actor.id,hid);self.assertEqual(actor.skillOrder,','.join(heroes[hid][k] for k in ['SkillId1','SkillId2','UltSkillId']));self.assertGreater(actor.prop.hp,0);self.assertGreater(actor.prop.att,0)
            saved=self.profile(login);stamp+=1;self.assertEqual(self.stamped('CSWarriorDecompose',{'guid':[war.guid]},login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            call('CSCopyLeave',{},'SCCopyLeave');call('CSFormationOff',{'formatPos':2,'id':hid,'guid':war.guid},'SCFormationOff');saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved)
            stamp+=1;decompose_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:decomposed=list(pool.map(lambda _:self.call('CSWarriorDecompose',{'guid':[war.guid]},login,decompose_stamp,'SCWarriorDecompose'),range(8)))
            self.assertTrue(all(x==decomposed[0] for x in decomposed));self.assertFalse(any(x['guid']==war.guid for x in self.profile(login)[1]['warriors']));self.assertEqual(sum(x.num for x in decomposed[0].get.simps if x.id=='coin'),1000+actual_growth_coin)
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);self.assertEqual(self.call('CSWarriorDecompose',{'guid':[war.guid]},login,decompose_stamp,'SCWarriorDecompose'),decomposed[0])
            saved=self.profile(login);self.assertEqual(self.stamped('CSCombine',combine,login,combine_stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            results.append({'hero':hid,'rawFragmentsPaid':150,'actualSourceCoin':natural,'originalStarBoxes':stars,'actualPaidCoin':15000000,'eligibilityDayCursorShiftsOnly':shifts,'GUID':war.guid,'originalModel':heroes[hid]['resName'],'originalIcon':heroes[hid]['iconName'],'actualBattleTransport':{'skillOrder':actor.skillOrder,'hp':actor.prop.hp,'att':actor.prop.att},'actualNaturalGrowth':{'source':'original ordinaryshop hero_exp001 from naturalstardiamond30','coinPaidThenRefunded':actual_growth_coin,'oldCombineGrownRejected':True},'realUse':'originalformationpos2/battleStart only/no winnerfixture/protecteddecompose rejection→actualleave/off→8decompose once original1000coin+realgrowthrefund/cold','retiredCombineReceipt':'new strict heldGUID+wholeWar equality;grown and retired oldstamp rejected without profile/cost/event mutation, historical receipt audit preserved'})
        self.assertEqual(self.sql("SELECT CONCAT(id,'|',resource_id,'|',amount,'|',weight) FROM gameplay_pool_rewards WHERE pool_id='recruit_3' ORDER BY id;"),king_before)
        self.sql(f"UPDATE p4_commerce_rules SET enabled=0 WHERE id='{child}';");self.stop();self.start();login=self.login(uid);self.assertEqual(self.sql(f"SELECT enabled FROM p4_commerce_rules WHERE id='{child}';").strip(),'0');self.assertEqual(self.call('CSBuyActivityShop',req,login,last_stamp,'SCGetActivityShopReward'),last_buy)
        data={'date':'2026-10-14','status':'PASS_SOURCE_CONSUMPTION_PHONE_PENDING','policy':'red-os-mainline-coin-fragments-v1','defaultOff':True,'originalThree':results,'king13PoolUnchanged':True,'originalFragPrice16':'item033 soul resale, never sourcecoin price','preparation':'three independent finite level100/stamina1000/bags1000/prior originalstar qualification;sourcecoin0/diamond0/noOS hero/fragment/sourcecoin/GUID inserted;actualstarboxes→coinpaid→actual150→originalcombine→formation/battle transport;dayeligible9cursor shifts/account not real10days;no battlewinner or PVPfacts','modelAvailability':'original APK actual model/icon/battle/field/low entries SHA in source audit; server actor transport only, parent actualphone render/skills pending','productionModified':False,'phoneUI':False,'seconds':round(time.monotonic()-began,3),'sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source}
        (shared.ROOT/'analysis/p15/red-three-supply-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
