"""Original worldseed currentnode/fullmain receipt originalHTTP lifecycle."""
import concurrent.futures
import http.client
import re
import selectors
import types
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

OWNED = ['tests/test_p8_vip_book_source_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P8_VIP_BOOK_SOURCE_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class VIPBookSourceHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p8_vipbook_http_' + secrets.token_hex(6)
        cls.test_user = 'p8bookhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p8-vipbook-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-vipbook-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P8_VIP_BOOK_SOURCE_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/empty-gem-sale-manifest.json','analysis/p8/gem-thirteen-manifest.json','analysis/p3/vip-growth-source-manifest.json']:
            dependency=json.loads((shared.ROOT/manifest).read_text());dependency_source=shared.ROOT/dependency.get('snapshot','.')
            for entry in dependency.get('owned',dependency.get('files',[])):
                rel=entry['path']
                if rel.startswith('hyjs-server/'):
                    source_file=(shared.ROOT/entry['snapshot']) if 'snapshot' in entry else dependency_source/rel
                    if hashlib.sha256(source_file.read_bytes()).hexdigest()!=entry['sha256']:raise AssertionError('Dependency SHA changed')
                    target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(source_file,target)
        for rel in OWNED:
            if rel.startswith('hyjs-server/'):
                target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(shared.ROOT/rel,target)
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
            report = shared.ROOT / 'analysis/p8/vip-book-source-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p8_vipbook_http_') or not cls.test_user.startswith('p8bookhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def start(self):
        args=[self.binary,'--http-port','0','--tcp-port','0','--tables',str(shared.ROOT/'analysis/tables'),'--enable-test-payments']
        if 'flag.String("log-file"' in (self.frozen_source/'main.go').read_text():args+=['--log-file',str(Path(self.temp.name)/'app.log')]
        self.process=subprocess.Popen(args,cwd=shared.ROOT/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ);self.assertTrue(ready.select(90));line=self.process.stdout.readline()
        ports=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line);self.assertIsNotNone(ports,line);self.service=types.SimpleNamespace(http_port=int(ports[1]),tcp_port=int(ports[2]))

    def test_actual_original_test_orders_vip_boxes_books_skills_and_fresh_recipe_gates(self):
        began=time.monotonic();self.start();stamp=1800250000000;cold_count=0;uid='demo-p8-vip-book-source';protected=self.login('protected-p8-vip-book-source')
        self.call('CSPlayerCreate',{'playerName':'书来源保护角色','heroId':'hero_043'},protected,stamp,'SCPlayerCreate');protected_before=self.profile(protected)
        login=self.login(uid)
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def once(name,fields,response):
            nonlocal stamp
            stamp+=1;saved_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool: replies=list(pool.map(lambda _:self.call(name,fields,login,saved_stamp,response),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));return replies[0],saved_stamp
        def reject(name,fields,old=None):
            nonlocal stamp
            if old is None:stamp+=1;old=stamp
            saved=self.profile(login);op,msg=self.stamped(name,fields,login,old);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved);return msg.msg
        def cold():
            nonlocal login,cold_count
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);cold_count+=1
        def confirm(order,index):
            conn=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=60)
            try:
                body=json.dumps({'orderId':order,'eventId':'isolated-p8-vipbook-'+str(index),'amountCents':200000,'outcome':'confirmed'})
                conn.request('POST','/local-test/payments/confirm',body=body,headers={'Content-Type':'application/json'});reply=conn.getresponse();data=json.loads(reply.read());self.assertEqual(reply.status,200);self.assertTrue(data['data']['testOnly']);self.assertEqual(data['data']['status'],'confirmed')
            finally:conn.close()
        call('CSPlayerCreate',{'playerName':'原书真实会员来源','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);self.assertEqual((p.get('vip',0),p['coin'],p['diamond']),(0,0,0))
        catalogue=call('CSShopProductList',{'shopType':2},'SCShopProductList');self.assertFalse(any(x.id.endswith('_growth_local') for x in catalogue.shopItemList))
        self.sql("UPDATE p3_vip_shop_products SET enabled=1 WHERE id IN ('p3vip7_growth_local','p3vip8_growth_local','p3vip9_growth_local','p3vip10_growth_local');")
        reject('CSShopBuy',{'shopType':2,'shopItemId':'p3vip9_growth_local','buyCount':1})
        for index in range(1,11):
            order=call('CSGenerateOrder',{'paymentId':'djsy.wuyou.andp2000','money':2000,'moneyType':'RMB','channelId':'AND0003','shopId':'common'},'SCGenerateOrder');confirm(order.orderId,index)
            if index==5:self.assertEqual((self.profile(login)[0]['vip'],self.profile(login)[0]['chargeDiamond']),(9,100000))
        p,g=self.profile(login);self.assertEqual((p['vip'],p['chargeDiamond']),(10,200000));diamond_from_orders=p['diamond']
        # Only original outdoor opening qualification is finite; crafting state
        # stays fresh1 and never receives prepared levels, experience or skills.
        outdoor=int(next(x['NameValue'] for x in shared.original_table('GameConfigure') if x['StaticName']=='OutdoorLevel'));p['level']=outdoor;self.write_profile(login,p,g)
        records=[];skill_receipts=[];price_total=0
        for vip,price,box,book,skill in [(7,800,'item_041',None,None),(8,900,'item_042','item_063','LifeSkill_085'),(9,1000,'item_043','item_065','SkillFishItem_051'),(10,1100,'item_044','item_064','LifeSkill_086')]:
            product='p3vip'+str(vip)+'_growth_local';listing=call('CSShopProductList',{'shopType':2},'SCShopProductList');row=next(x for x in listing.shopItemList if x.id==product);self.assertEqual((row.price[0].id,row.price[0].num,row.condition.value),('diamond',price,vip))
            fields={'shopType':2,'shopItemId':product,'buyCount':1};stamp+=1;buy_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.stamped('CSShopBuy',fields,login,buy_stamp),range(8)))
            successful=[msg for op,msg in replies if op==transport.BY_NAME['SCShopBuy']] if hasattr(transport,'BY_NAME') else [msg for op,msg in replies if op!=10001]
            self.assertEqual(len(successful),1);self.assertEqual(sum(op==10001 for op,_ in replies),7);price_total+=price
            opened,_=once('CSItemUse',{'costItem':{'id':box,'num':1}},'SCItemUse')
            if book:
                self.assertEqual(sum(x.num for x in opened.get.simps if x.id==book),1);study={'costItem':{'id':book,'num':1}};learned,learn_stamp=once('CSItemUse',study,'SCItemUse');skill_receipts.append((study,learn_stamp,learned));self.assertIn(skill,call('CSCookingLevel',{},'SCCookingLevel').skills)
                reject('CSItemUse',study)
            else:self.assertFalse(any(x.id in ['item_063','item_064','item_065'] for x in opened.get.simps))
            records.append({'VIP':vip,'product':product,'price':price,'box':box,'book':book,'skill':skill,'eightPurchases':'one successful/seven quota-rejected','eightBoxAndStudy':'exactonce'})
        state=call('CSCookingLevel',{},'SCCookingLevel');self.assertEqual((state.level,state.gemLevel),(1,1));self.assertEqual(set(state.skills),{'LifeSkill_085','LifeSkill_086','SkillFishItem_051'})
        gates=[]
        for name,fields in [('CSGemMake',{'gemId':'MineItem_079','num':1}),('CSGemMake',{'gemId':'MineItem_080','num':1}),('CSFishCooking',{'fish':{'id':'FishItem_050','num':1}})]:gates.append({'name':name,'fields':fields,'error':reject(name,fields)})
        cold()
        for study,old,reply in skill_receipts:
            saved=self.profile(login);self.assertEqual(self.call('CSItemUse',study,login,old,'SCItemUse'),reply);self.assertEqual(self.profile(login),saved)
        state=call('CSCookingLevel',{},'SCCookingLevel');self.assertEqual((state.level,state.gemLevel),(1,1));self.assertEqual(set(state.skills),{'LifeSkill_085','LifeSkill_086','SkillFishItem_051'})
        p,g=self.profile(login);self.assertEqual(p['diamond'],diamond_from_orders-price_total);self.assertEqual(self.profile(protected),protected_before)
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_ACTUAL_VIP_BOOK_SOURCE_FRESH_RECIPE_GATES','actualTestOnlyOrders':10,'actualPaymentProduct':'djsy.wuyou.andp2000','chargeDiamond':p['chargeDiamond'],'VIP':p['vip'],'actualDiamondFromConfirmedOrders':diamond_from_orders,'paidVIPBoxesDiamond':price_total,'finalDiamond':p['diamond'],'boxes':records,'actualSkills':list(state.skills),'freshCookingLevel':state.level,'freshGemLevel':state.gemLevel,'correctFreshRecipeFailures':gates,'coldRestarts':cold_count,'qualification':'only originalOutdoorLevel finite; no VIP/diamond/paymentfacts/box/books/skills/craftlevel/XP/material insertions;actualtrustedloopbacktestconfirm flag/no real money. Defaultoff productsenabledonly dedicated test; originalboxcontents/gates retained','highestRecipeBoundary':'booksources/learning completed; naturalCraft7/10/Cook6 andraw086highmaterialsremainpendingactuallongsource, no fullrecipe sourceclaim','protectedFirstProfileUnchanged':True,'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False,'baseline':self.baseline_source}
        (shared.ROOT/'analysis/p8/vip-book-source-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
