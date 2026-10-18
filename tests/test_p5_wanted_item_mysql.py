"""Original chapter diamond -> fixed-price wanted order -> paid attempt counter/cold."""
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

@unittest.skipUnless(os.environ.get('HYJS_P5_WANTED_ITEM_MYSQL_TEST')=='1','Explicit original wanted order original HTTP/InnoDB flow')
class P5WantedItemMySQLTests(unittest.TestCase):
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
        if not cls.database.startswith('hyjs_test_p5_wanted_')or not cls.test_user.startswith('p5wanted_'):raise AssertionError('Own wanted schema required')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError(result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_canonical_wanted_order_seed_preserves_operator(self):
        began=time.monotonic();type(self).database='hyjs_test_p5_wanted_'+secrets.token_hex(6);type(self).test_user='p5wanted_'+secrets.token_hex(5);type(self).test_password=secrets.token_hex(24);seed=ROOT/'hyjs-server/db/sql/12_shop_products.seed.sql';report={}
        with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
            fcntl.flock(lock,fcntl.LOCK_EX)
            try:
                self.root_sql(f"CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;CREATE USER '{self.test_user}'@'%' IDENTIFIED BY '{self.test_password}';GRANT ALL ON `{self.database}`.* TO '{self.test_user}'@'%';")
                def own(text):return self.root_sql('USE `'+self.database+'`;SET NAMES utf8mb4;'+text)
                own((ROOT/'hyjs-server/db/sql/12_shop_products.sql').read_text());own(seed.read_text());row=own('SELECT resource_id,amount,cost,daily_limit,min_level,priority,enabled,source FROM shop_products WHERE id="itemShopId_12";').strip().split('\t');self.assertEqual(row[:7],['item_004','1','diamond,200','10','1','8','1']);self.assertIn('wanted-order-original-fixed-slot-v1',row[7]);self.assertEqual(own('SELECT COUNT(*) FROM shop_products;').strip(),'8');own('UPDATE shop_products SET enabled=0,source="operator-choice" WHERE id="itemShopId_12";');own(seed.read_text());self.assertEqual(own('SELECT enabled,source FROM shop_products WHERE id="itemShopId_12";').strip(),'0\toperator-choice');report={'status':'PASS_CANONICAL_FIXED_PRICE12_DEFAULT_SOURCE_OPERATOR_PRESERVED','seconds':round(time.monotonic()-began,3),'seedSHA256':hashlib.sha256(seed.read_bytes()).hexdigest(),'productionModified':False}
            finally:
                self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';");left=self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';");self.assertEqual(left.split(),['0','0','0']);report['cleanupCounts']=[0,0,0]
        (ROOT/'analysis/p5/wanted-item-sql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

    def test_original_wanted_order_purchase_use_current_counter_and_cold(self):
        began=time.monotonic();report={};self.process=None
        manifest=json.loads((ROOT/'analysis/p5/wanted-item-manifest.json').read_text())
        with tempfile.TemporaryDirectory(prefix='hyjs-p5-wanted-')as temporary:
            self.source=Path(temporary).resolve();base=ROOT/manifest['base'];shutil.copytree(base/'hyjs-server',self.source/'hyjs-server',ignore=shutil.ignore_patterns('logs'));(self.source/'analysis').symlink_to(base/'analysis',target_is_directory=True)
            for entry in manifest['owned']:
                if not entry['path'].startswith('hyjs-server/'):continue
                original=ROOT/manifest['snapshot']/entry['path'];self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),entry['sha256']);shutil.copy2(original,self.source/entry['path'])
            self.binary=self.source/'wanted-native';scoped=[]
            for args in [['go','test','./service','-run','^TestP5OriginalWantedOrder$','-count=1','-timeout=2m'],['go','build','-o',str(self.binary),'.']]:
                result=subprocess.run(['rtk','proxy',*args],cwd=self.source/'hyjs-server',text=True,capture_output=True,timeout=240);self.assertEqual(result.returncode,0,(result.stdout+result.stderr)[-5000:]);scoped.append((result.stdout+result.stderr).strip())
            type(self).database='hyjs_test_p5_wanted_'+secrets.token_hex(6);type(self).test_user='p5wanted_'+secrets.token_hex(5);type(self).test_password=secrets.token_hex(24);self.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_DATABASE=self.database,HYJS_MYSQL_USER=self.test_user,HYJS_MYSQL_PASSWORD=self.test_password)
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    self.root_sql(f"CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;CREATE USER '{self.test_user}'@'%' IDENTIFIED BY '{self.test_password}';GRANT ALL ON `{self.database}`.* TO '{self.test_user}'@'%';");self.start();uid='wanted-order-'+secrets.token_hex(5);login=self.login(uid);stamp=1801000000000;colds=0
                    def http(name,fields,want):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,login,stamp,want)
                    def count():
                        return sum(v.get('num',0)for v in self.profile(login)[1].get('items',[])if v['id']=='item_004')
                    def cold():
                        nonlocal login,colds
                        before=self.profile(login);self.stop();self.start();login=self.login(uid);after=self.profile(login);self.assertEqual(after[0].get('wantedAttTimes',0),before[0].get('wantedAttTimes',0));self.assertEqual(after[0].get('wantedAttTime',0),before[0].get('wantedAttTime',0));self.assertEqual(after[0].get('diamond',0),before[0].get('diamond',0));self.assertEqual(after[1].get('items',[]),before[1].get('items',[]));colds+=1
                    http('CSPlayerCreate',{'playerName':'原通缉令实际使用','heroId':'hero_043'},'SCPlayerCreate');p,g=self.profile(login);self.assertEqual(p.get('diamond',0),0);self.assertEqual(p.get('wantedAttTimes',0),0);p.update(level=35,noviceGuideSchedule=9);g['copy']['passedCopyInfo']=[{'chapterId':v['ChapterId'],'missionId':v['MissionId'],'star':5,'challengeTimes':0}for v in shared.original_table('CopyMission')if v['ChapterId']in['1','2']];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
                    source=0
                    for chapter in ['1','2']:
                        for star in [0,1,2]:
                            result=http('CSCopyGetStarAward',{'chapterId':chapter,'starIndex':star},'SCCopyGetStarAward');source+=sum(v.num for v in result.gain.simps if v.id=='diamond')
                    self.assertEqual(source,540);self.assertEqual(count(),0);catalog=http('CSShopProductList',{'shopType':1},'SCShopProductList');item=next(v for v in catalog.shopItemList if v.id=='itemShopId_12');self.assertEqual([(v.id,v.num)for v in item.item],[('item_004',1)]);self.assertEqual([(v.id,v.num)for v in item.exchangeList],[('diamond',200)]);self.assertEqual(item.dayLeftTimes,10)
                    purchase=http('CSShopBuy',{'shopType':1,'shopItemId':'itemShopId_12','buyCount':2},'SCShopBuy');self.assertEqual([(v.id,v.num)for v in purchase.cost],[('diamond',400)]);self.assertEqual(count(),2);wanted_time=self.profile(login)[0].get('wantedAttTime',0);stamp+=1;first_stamp=stamp;request={'costItem':{'id':'item_004','num':1}}
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8)as pool:replies=list(pool.map(lambda _:self.call('CSItemUse',request,login,first_stamp,'SCItemUse'),range(8)))
                    self.assertTrue(all(v==replies[0]for v in replies));self.assertEqual([(v.id,v.num)for v in replies[0].get.simps],[('wantedAttTimes',1)]);self.assertEqual(count(),1);current=http('CSPlayer',{},'SCPlayer');self.assertEqual(current.player.wantedAttTimes,1);self.assertEqual(current.player.wantedAttTime,wanted_time);cold();self.assertEqual(self.call('CSItemUse',request,login,first_stamp,'SCItemUse'),replies[0]);self.assertEqual(self.profile(login)[0]['wantedAttTimes'],1)
                    second=http('CSItemUse',request,'SCItemUse');second_stamp=stamp;self.assertEqual([(v.id,v.num)for v in second.get.simps],[('wantedAttTimes',1)]);self.assertEqual(count(),0);self.assertEqual(self.profile(login)[0]['wantedAttTimes'],2);before=self.profile(login);op,error=self.stamped('CSItemUse',request,login,stamp+1);stamp+=1;self.assertEqual(op,10001);self.assertEqual(self.profile(login),before);cold();current=http('CSPlayer',{},'SCPlayer');self.assertEqual(current.player.wantedAttTimes,2);self.assertEqual(self.call('CSItemUse',request,login,second_stamp,'SCItemUse'),second);self.assertEqual(count(),0);self.assertEqual(self.profile(login)[0]['diamond'],140);day=http('CSShopProductList',{'shopType':1},'SCShopProductList');self.assertEqual(next(v for v in day.shopItemList if v.id=='itemShopId_12').dayLeftTimes,8);self.stop()
                    report={'status':'PASS_ORIGINAL_STAR_DIAMOND_SHOP004_USE_WANTED_COUNTER_CURRENT_AND_COLD','seconds':round(time.monotonic()-began,3),'ownedGoSHA256':{e['path']:hashlib.sha256((self.source/e['path']).read_bytes()).hexdigest()for e in manifest['owned']if e['path'].endswith('.go')},'scopedGo':scoped,'actualStarDiamond':source,'actualPurchaseDiamond':400,'originalShopSlot':'itemShopId_12/200diamond/daily10','actualOrdersPurchased':2,'actualOrdersConsumed':2,'counterSequence':[0,1,2],'originalGainResource':'wantedAttTimes','unchangedRecoveryTime':wanted_time,'finalDiamond':140,'finalOrders':0,'dayLeft':8,'eightFirstUse':'one item consumed, one paid attempt, same original response','coldRestarts':colds,'finitePrerequisites':'Only characterlevel35/guide9/historicalchapter1and2stars/bag default100. Actual star diamonds, purchased004 and paid wanted counter never inserted; no P12 combat/winner/recovery-time change. Original fixed price retained; local catalog mapping slot12->004 explicitly named private mapping unknown.','phoneUI':False,'productionModified':False}
                finally:
                    self.stop();self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';");left=self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';");self.assertEqual(left.split(),['0','0','0']);report['cleanupCounts']=[0,0,0]
        if report.get('status'):(ROOT/'analysis/p5/wanted-item-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
