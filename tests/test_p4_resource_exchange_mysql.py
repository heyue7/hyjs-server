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

OWNED = ['hyjs-server/service/p4ResourceExchangeService.go', 'hyjs-server/service/p4ResourceExchange_test.go', 'hyjs-server/service/p4CommerceService.go', 'hyjs-server/service/p4ActivityService.go', 'hyjs-server/service/p4Service.go', 'hyjs-server/constUtil/p4ResourceExchange.go', 'hyjs-server/db/p4ResourceExchangeDirectory.go', 'hyjs-server/db/sql/106_p4_resource_exchange_directories.sql', 'tests/test_p4_resource_exchange_mysql.py', 'analysis/p4/type23-directory-original.il.txt', 'hyjs-server/service/redWheelService.go', 'hyjs-server/service/p5MaterialSupply_test.go', 'analysis/p4/resource-exchange-integration.patch']

@unittest.skipUnless(os.environ.get('HYJS_P4_RESOURCE_EXCHANGE_MYSQL_TEST') == '1', 'Explicit locked isolated P5 gem supply')
class ResourceExchangeMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p4_resource_exchange_' + secrets.token_hex(6)
        cls.test_user = 'p4exchange_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p4-exchange-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-gem-supply')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P4_RESOURCE_EXCHANGE_BASE', shared.ROOT / 'build/candidates/joint-natural-guild-v59-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
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
            report = shared.ROOT / 'analysis/p4/resource-exchange-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p4_resource_exchange_') or not cls.test_user.startswith('p4exchange_'):raise AssertionError('P5 gem supply isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 gem supply administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_single_type23_all10_existing_quotas_receipts_and_cold(self):
        began=time.monotonic();self.start();stamp=1800110000000;results=[]
        month=time.strftime('%Y%m',time.gmtime(time.time()+8*3600));key='limitExchange_p4_resources_'+month
        child_base='limitExchange_p4_'+month;child_gem='limitExchange_p4_gem_'+month;child_book='limitExchange_p4_specialty_'+month
        specs=[('equip_037',None,0),('item_137',child_gem,1),('item_342',child_gem,2),('item_343',child_gem,3),('item_344',child_gem,4),('item_345',child_gem,5),('item_441',child_book,1),('item_445',child_book,2),('item_449',child_book,3),('item_453',child_book,4)]
        missions=shared.original_table('CopyMission');all_requests=[]
        red_count=int(self.sql('SELECT COUNT(*) FROM p2_red_exchanges WHERE enabled=1;').strip());expected=10+red_count
        for index,(resource,legacy_key,legacy_index) in enumerate(specs):
            uid='natural-resource-directory-'+str(index);login=self.login(uid)
            def call(name,fields,response):
                nonlocal stamp
                stamp+=1;return self.call(name,fields,login,stamp,response)
            call('CSPlayerCreate',{'playerName':'原资源兑换'+str(index),'heroId':'hero_043'},'SCPlayerCreate')
            p,g=self.profile(login);p.update(level=100,coin=0,noviceGuideSchedule=8)
            g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions];g['copy']['passedChapterInfo']=[]
            if index==0:g['items']=[{'id':'item_021','num':10}] # finite base-exchange essence prerequisite, declared separately
            self.write_profile(login,p,g)
            tabs=call('CSActivityList',{},'SCActivityList')
            # Exact original client dictionary: one prefix maps to one Type23 key.
            roots=[x for x in tabs.activityList if x.activityId.split('_')[0]=='limitExchange']
            self.assertEqual(len(roots),1);self.assertEqual(roots[0].activityId,key)
            listing=call('CSActivityShopInfo',{'activityId':roots[0].activityId},'SCActivityShopInfo');self.assertEqual(len(listing.shopItemList),expected)
            self.assertEqual(len(set(x.index for x in listing.shopItemList)),expected)
            item=next(x for x in listing.shopItemList if x.getResource[0].id==resource)
            source_coin=0;boxes=0
            if item.costResource[0].id=='coin':
                for chapter in range(27,0,-1):
                    for box in range(3):
                        gain=call('CSCopyGetStarAward',{'chapterId':str(chapter),'starIndex':box},'SCCopyGetStarAward').gain
                        source_coin+=sum(x.num for x in gain.simps if x.id=='coin');boxes+=1
                    if source_coin>=item.costResource[0].num*(item.total if resource in ['item_441','item_445','item_449','item_453'] else 1):break
                self.assertGreaterEqual(source_coin,item.costResource[0].num)
            req={'activityId':key,'index':item.index,'num':1,'costResources':item.costResource,'getResources':item.getResource};stamp+=1;first_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSBuyActivityShop',req,login,first_stamp,'SCGetActivityShopReward'),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));first=replies[0];self.assertEqual(len(first.shopItemList),expected)
            self.assertEqual(next(x.left for x in first.shopItemList if x.index==item.index),item.total-1)
            if index==0:
                self.assertEqual(len(first.get.equips),1);self.assertEqual(first.get.equips[0].equipId,resource);legacy_key=child_base;legacy_index=1
                self.assertEqual(next((x['num'] for x in self.profile(login)[1]['items'] if x['id']=='item_021'),0),0)
            else:
                self.assertEqual([(x.id,x.num) for x in first.get.simps],[(resource,item.getResource[0].num)])
                self.assertEqual(self.profile(login)[0]['coin'],source_coin-item.costResource[0].num)
            legacy=dict(req,activityId=legacy_key,index=legacy_index);stamp+=1
            if item.total==1:self.assertEqual(self.stamped('CSBuyActivityShop',legacy,login,stamp)[0],10001)
            else:
                legacy['num']=item.total-1
                self.call('CSBuyActivityShop',legacy,login,stamp,'SCGetActivityShopReward')
                stamp+=1;self.assertEqual(self.stamped('CSBuyActivityShop',req,login,stamp)[0],10001)
            # One child state owns both entry paths; no outer quota exists.
            total=json.loads(self.sql(f"SELECT payload FROM p4_commerce_states WHERE player_id={login.playerId} AND rule_id='{legacy_key}';").strip())['total']
            self.assertEqual(total['p4Essence_1' if index==0 else resource],item.total)
            saved=self.profile(login);self.stop();self.start();login=self.login(uid)
            self.assertEqual(self.call('CSBuyActivityShop',req,login,first_stamp,'SCGetActivityShopReward'),first);self.assertEqual(self.profile(login),saved)
            results.append({'resource':resource,'outerIndex':item.index,'child':legacy_key,'price':[(x.id,x.num) for x in item.costResource],'sourceOriginalStarBoxes':boxes,'sourceCoin':source_coin,'8frames':'exact once and original child quota','cold':True})
            all_requests.append((uid,req,first_stamp,first))
        # P2 uses its original resource currencies and state, with no new outer quota.
        uid='aggregate-red-original-state';login=self.login(uid)
        call('CSPlayerCreate',{'playerName':'原红兑换聚合','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,noviceGuideSchedule=8);g['items']=[{'id':'item_020','num':100},{'id':'item_021','num':200}];self.write_profile(login,p,g)
        direct_red=call('CSActivityShopInfo',{'activityId':'limitExchange_p2_red'},'SCActivityShopInfo')
        red_item=direct_red.shopItemList[2]
        aggregate=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo')
        mapped=next(x for x in aggregate.shopItemList if x.index>10 and x.getResource[0].id==red_item.getResource[0].id)
        red_req={'activityId':key,'index':mapped.index,'costResources':mapped.costResource,'getResources':mapped.getResource};stamp+=1;red_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:red_replies=list(pool.map(lambda _:self.call('CSGetActivityShopReward',red_req,login,red_stamp,'SCGetActivityShopReward'),range(8)))
        self.assertTrue(all(x==red_replies[0] for x in red_replies))
        stamp+=1;old_red_req=dict(red_req,activityId=direct_red.activityId,index=red_item.index)
        self.assertEqual(self.stamped('CSGetActivityShopReward',old_red_req,login,stamp)[0],10001)
        saved=self.profile(login);self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSGetActivityShopReward',red_req,login,red_stamp,'SCGetActivityShopReward'),red_replies[0]);self.assertEqual(self.profile(login),saved)
        # Existing direct child receipts and source configurations remain valid.
        uid='legacy-book-receipt';login=self.login(uid)
        call('CSPlayerCreate',{'playerName':'旧书回执保留','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=0,noviceGuideSchedule=8);g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions];self.write_profile(login,p,g)
        for box in range(3):call('CSCopyGetStarAward',{'chapterId':'27','starIndex':box},'SCCopyGetStarAward')
        old=call('CSActivityShopInfo',{'activityId':child_book},'SCActivityShopInfo').shopItemList[0]
        old_req={'activityId':child_book,'index':old.index,'num':1,'costResources':old.costResource,'getResources':old.getResource};stamp+=1;old_stamp=stamp
        old_reply=self.call('CSBuyActivityShop',old_req,login,old_stamp,'SCGetActivityShopReward')
        self.assertEqual(next(x.left for x in call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo').shopItemList if x.getResource[0].id=='item_441'),old.total-1)
        # Operator closure suppresses only affected children; old receipts replay first.
        self.sql(f"UPDATE p4_commerce_rules SET enabled=0 WHERE id='{child_gem}';")
        self.assertEqual(len(call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo').shopItemList),5+red_count)
        self.sql(f"UPDATE p4_resource_exchange_directories SET enabled=0 WHERE id='{key}';")
        self.assertFalse(any(x.activityId==key for x in call('CSActivityList',{},'SCActivityList').activityList))
        stamp+=1;self.assertEqual(self.stamped('CSActivityShopInfo',{'activityId':key},login,stamp)[0],10001)
        self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSBuyActivityShop',old_req,login,old_stamp,'SCGetActivityShopReward'),old_reply)
        uid,req,first_stamp,first=all_requests[1];login=self.login(uid)
        self.assertEqual(self.call('CSBuyActivityShop',req,login,first_stamp,'SCGetActivityShopReward'),first)
        self.assertEqual(self.sql(f"SELECT enabled FROM p4_resource_exchange_directories WHERE id='{key}';").strip(),'0')
        self.assertIn('InnoDB',self.sql("SELECT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='p4_resource_exchange_directories';"))
        report={'date':'2026-10-14','policy':'resource-exchange-directory-v1','results':results,'oneOriginalType23Reachability':'original prefix-to-type23/activityIds dictionary replay has exactly one visible ID and all10 unique indices; native phone remains root verification','oldReceipts':'legacy book and outer gem exact cold replay after child/directory closure','productionModified':False,'phoneUI':False,'preparation':'finite level100 original historical stars; every coin acquired by actual original star awards; base10 item021 and separate red100 item020/200 item021 prerequisites prepared, no books/gem packages/coins/GUID inserted','seconds':round(time.monotonic()-began,3),'sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest()}
        (shared.ROOT/'analysis/p4/resource-exchange-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
