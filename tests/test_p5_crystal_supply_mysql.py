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

OWNED = ['hyjs-server/service/p5CrystalSupplyService.go', 'hyjs-server/service/p5CrystalSupply_test.go', 'hyjs-server/constUtil/p5CrystalSupply.go', 'hyjs-server/service/p4CommerceService.go', 'hyjs-server/service/p4ResourceExchangeService.go', 'hyjs-server/db/sql/83_p4_commerce_rules.seed.sql', 'analysis/p5/crystal-supply-original-audit.json', 'tests/test_p5_crystal_supply_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_CRYSTAL_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class CrystalSupplyMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_crystal_' + secrets.token_hex(6)
        cls.test_user = 'p5crys_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-crystal-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-gem-supply')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_CRYSTAL_BASE', shared.ROOT / 'build/candidates/joint-catalogue-car-v65-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p8/ingredient-supply-manifest.json','analysis/p15/red-three-supply-manifest.json']:
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
            if left.split() != ['0','0','0']: raise AssertionError('P5 fourcrystals schema/user/grants remain')
            report = shared.ROOT / 'analysis/p5/crystal-supply-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_crystal_') or not cls.test_user.startswith('p5crys_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_four_crystal_coin_source_books_strength_hp_and_cold(self):
        began=time.monotonic();self.start();stamp=1800180000000;uid='actual-four-specialty-crystals';login=self.login(uid)
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        from datetime import datetime,timezone,timedelta
        month=datetime.now(timezone(timedelta(hours=8))).strftime('%Y%m');key='limitExchange_p4_resources_'+month;child='limitExchange_p4_crystals_'+month
        call('CSPlayerCreate',{'playerName':'原四晶自然强化','heroId':'hero_043'},'SCPlayerCreate');missions=shared.original_table('CopyMission')
        p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,stamina=1000,equipBagSize=1000,warriorBagSize=1000,noviceGuideSchedule=8);g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions];g['copy']['passedChapterInfo']=[];g['run']=None;self.write_profile(login,p,g)
        natural=0;stars=0
        for chapter in reversed(shared.original_table('CopyChapter')):
            for box in range(3):
                reward=call('CSCopyGetStarAward',{'chapterId':chapter['ChapterId'],'starIndex':box},'SCCopyGetStarAward');natural+=sum(x.num for x in reward.gain.simps if x.id=='coin');stars+=1
                if natural>=15550000:break
            if natural>=15550000:break
        self.assertGreaterEqual(natural,15550000);self.assertEqual(self.profile(login)[0]['coin'],natural)
        off=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo');oldindices={item.index:item.name for item in off.shopItemList}
        crystal_ids=['item_442','item_446','item_450','item_454'];book_ids=['item_441','item_445','item_449','item_453']
        self.assertFalse(any(item.getResource[0].id in crystal_ids for item in off.shopItemList));self.assertEqual(self.sql(f"SELECT enabled,OCTET_LENGTH(COALESCE(frozen,'')) FROM p4_commerce_rules WHERE id='{child}';").strip(),'0\t0')
        self.sql(f"UPDATE p4_commerce_rules SET enabled=1 WHERE id='{child}';")
        listing=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo');products={item.getResource[0].id:item for item in listing.shopItemList if item.getResource[0].id in crystal_ids+book_ids}
        self.assertEqual(len(products),8)
        for item in listing.shopItemList:
            if item.index in oldindices:self.assertEqual(item.name,oldindices[item.index])
        last_buy=None;last_req=None;last_buy_stamp=None;source_buys=[];paid=0
        for resource,quantity in list(zip(crystal_ids,[4,4,4,4]))+list(zip(book_ids,[9,15,8,10])):
            product=products[resource];req={'activityId':key,'index':product.index,'num':quantity,'costResources':product.costResource,'getResources':product.getResource};stamp+=1;buy_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSBuyActivityShop',req,login,buy_stamp,'SCGetActivityShopReward'),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));amount=(3 if resource in crystal_ids else 10)*quantity;self.assertEqual(next(x['num'] for x in self.profile(login)[1]['items'] if x['id']==resource),amount)
            paid+=product.costResource[0].num*quantity;source_buys.append({'resource':resource,'bundles':quantity,'actualQuantity':amount,'paidCoin':product.costResource[0].num*quantity});last_buy,last_req,last_buy_stamp=replies[0],req,buy_stamp
        self.assertEqual(paid,15550000);self.assertEqual(self.profile(login)[0]['coin'],natural-paid)
        phases=[(10,1,{},90,1500),(20,2,{'item_444':40},150,3000),(40,4,{'item_447':70,'item_448':50},80,5000),(70,7,{'item_451':85,'item_452':60},100,6000)];effects=[]
        for i,(level,strength,other,book_num,hp_delta) in enumerate(phases):
            p,g=self.profile(login);guid='crystal-finite-partner-'+str(i);target={'id':'hero_001','guid':guid,'level':100,'exp':0,'talent':0,'training':{},'tempTraining':{},'breached':{'level':1},'awake':{},'skillRefine':[{'roleGuid':guid,'type':1,'level':level,'strengthLevel':strength}]};g['warriors']=[x for x in g['warriors'] if x['guid']!=guid]+[target]
            for resource,num in other.items():g['items']=[x for x in g['items'] if x['id']!=resource]+[{'id':resource,'num':num}]
            g['run']={'chapter':'27','mission':'161','passed':[]};self.write_profile(login,p,g)
            call('CSFormationOn',{'formatPos':2,'id':target['id'],'guid':guid},'SCFormationOn');before=next(x.prop.hp for x in call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo').battlePlayer.formation if x.guid==guid)
            fields={'roleGuid':guid,'type':1};stamp+=1;strength_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSStrengthRoleSkillRefineLevel',fields,login,strength_stamp,'SCStrengthRoleSkillRefineLevel'),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));self.assertEqual(next(x.num for x in replies[0].cost if x.id==crystal_ids[i]),10);self.assertEqual(next(x.num for x in replies[0].cost if x.id==book_ids[i]),book_num)
            after=next(x.prop.hp for x in call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo').battlePlayer.formation if x.guid==guid);self.assertEqual(after-before,hp_delta)
            self.assertEqual(next(x['num'] for x in self.profile(login)[1]['items'] if x['id']==crystal_ids[i]),2)
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);self.assertEqual(self.call('CSStrengthRoleSkillRefineLevel',fields,login,strength_stamp,'SCStrengthRoleSkillRefineLevel'),replies[0])
            effects.append({'crystal':crystal_ids[i],'actuallyConsumed':10,'actualBooksConsumed':book_num,'actualHPDelta':hp_delta,'eightOnceAndCold':True})
        self.sql(f"UPDATE p4_commerce_rules SET enabled=0 WHERE id='{child}';");self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.sql(f"SELECT enabled FROM p4_commerce_rules WHERE id='{child}';").strip(),'0');self.assertEqual(self.call('CSBuyActivityShop',last_req,login,last_buy_stamp,'SCGetActivityShopReward'),last_buy)
        final=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo');self.assertFalse(any(item.getResource[0].id in crystal_ids for item in final.shopItemList))
        data={'date':'2026-10-14','status':'PASS_SOURCE_USE_PHONE_PENDING','policy':'specialty-crystal-mainline-coin-exchange-v1','defaultOff':True,'actualOriginalStarBoxes':stars,'actualSourceCoin':natural,'actualPaidCoin':paid,'sourceBuys':source_buys,'effects':effects,'gate':'defaultoff nofreeze/lateappend stableindices;every8buy/use exactonce/cold4/fullprofile/offoldreceipt/operatoroffpreserved','preparation':'finite level100/stamina1000/bags1000/originalhistoricalstars;4 independent finitehero001guid/prior specialtylevel+strength/other5material IDs qualifications;coin0/diamond0/no4crystals/no4books/sourcecoin insertion. No daycursors or winner/boss/PVPfacts. Natural source for other prereqmaterials and fullhighest all49 remain separately unresolved.','originalBoundary':'FieldBoss21..32 original3/0 opaque10/90 source remains absent highmap/authority;local500k costreference uses originalbosscoin250k×2 only, not claim originalopsprice/probability. Existing books/gems/redOS/foodssource limits unchanged','productionModified':False,'phoneUI':False,'seconds':round(time.monotonic()-began,3),'sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source}
        (shared.ROOT/'analysis/p5/crystal-supply-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
