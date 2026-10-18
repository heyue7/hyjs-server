"""Original hero-on whole formation lifecycle through actual frames and InnoDB."""
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

OWNED = ['hyjs-server/service/warriorService.go','hyjs-server/service/p5FormationNextService.go','hyjs-server/service/p5GrowthReceiptService.go','hyjs-server/constUtil/p5Next.go','hyjs-server/service/p5WarriorStrengthenReceipt_test.go','tests/test_p5_warrior_strengthen_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_WARRIOR_STRENGTHEN_RECEIPT_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class WarriorStrengthenReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_warrior_str_http_' + secrets.token_hex(6)
        cls.test_user = 'p5warstrhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-warstr-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-warstr-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_WARRIOR_STRENGTHEN_RECEIPT_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/treasure-award-receipt-manifest.json','analysis/p5/formation-receipt-manifest.json','analysis/p5/helper-receipt-manifest.json','analysis/p5/skill-order-receipt-manifest.json','analysis/p5/equipment-formation-receipt-manifest.json','analysis/p5/treasure-formation-receipt-manifest.json','analysis/p5/formation-exchange-receipt-manifest.json','analysis/p5/tactical-receipt-manifest.json','analysis/p5/treasure-strengthen-receipt-manifest.json']:
            dependency=json.loads((shared.ROOT/manifest).read_text());dependency_source=shared.ROOT/dependency.get('snapshot','.')
            for entry in dependency.get('owned',dependency.get('files',[])):
                rel=entry['path']
                if rel.startswith('hyjs-server/'):
                    source_file=shared.ROOT/entry['snapshot'] if 'snapshot' in entry else dependency_source/rel
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
            report = shared.ROOT / 'analysis/p5/warrior-strengthen-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_warrior_str_http_') or not cls.test_user.startswith('p5warstrhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_warrior_growth_committed_material_order_reset_sold_cold(self):
        began=time.monotonic();self.start();stamp=1800790000000;cold_count=0;uid='actual-warrior-strengthen-receipt';login=self.login(uid)
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def once(name,fields,response):
            nonlocal stamp
            stamp+=1;old=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call(name,fields,login,old,response),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));return replies[0],old
        def cold():
            nonlocal login,cold_count
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);cold_count+=1
        def keep(fields,old,reply):
            saved=self.profile(login);self.assertEqual(self.call('CSWarriorStrengthen',fields,login,old,'SCWarriorStrengthen'),reply);self.assertEqual(self.profile(login),saved)
        def reject(fields,old):
            saved=self.profile(login);op,msg=self.stamped('CSWarriorStrengthen',fields,login,old);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved);return msg.msg
        call('CSPlayerCreate',{'playerName':'原伙伴升级重试','heroId':'hero_043'},'SCPlayerCreate');p,g=self.profile(login);p.update(level=100,noviceGuideSchedule=8,warriorBagSize=1000);self.assertEqual((p.get('coin',0),p.get('diamond',0)),(0,0));g['items']=[{'id':'item_067','num':1}];g['copy']['passedCopyInfo']=[{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0} for r in shared.original_table('CopyMission') if r['ChapterId']=='27'];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        source=call('CSCopyGetStarAward',{'chapterId':'27','starIndex':0},'SCCopyGetStarAward');source_coin=sum(x.num for x in source.gain.simps if x.id=='coin');source_diamond=sum(x.num for x in source.gain.simps if x.id=='diamond');self.assertEqual((source_coin,source_diamond),(260000,350))
        target=call('CSItemUse',{'costItem':{'id':'item_067','num':1},'getItemIndex':1},'SCItemUse').get.wars[0];self.assertEqual(target.id,'hero_011');before=self.profile(login)[0];[call('CSShopBuy',{'shopType':1,'shopItemId':'itemShopId_5','buyCount':1},'SCShopBuy') for _ in range(2)];purchase=before['diamond']-self.profile(login)[0]['diamond'];self.assertEqual(purchase,60);foods=[x for x in self.profile(login)[1]['warriors'] if x['id']=='hero_exp_001'];self.assertEqual(len(foods),2)
        first={'guid':target.guid,'cost':[{'id':foods[0]['id'],'guid':foods[0]['guid']}]};grown,first_stamp=once('CSWarriorStrengthen',first,'SCWarriorStrengthen');self.assertEqual((grown.guid.level,grown.guid.exp),(2,260));self.assertEqual(sum(x.num for x in grown.cost if x.id=='coin'),400);keep(first,first_stamp,grown);cold();keep(first,first_stamp,grown)
        call('CSFormationOn',{'id':target.id,'guid':target.guid,'formatPos':2},'SCFormationOn');cold();keep(first,first_stamp,grown)
        row=next(x for x in shared.original_table('Hero') if x['id']==target.id);order=row['SkillId2']+','+row['SkillId1'];call('CSSetSkillOrder',{'heroGuid':target.guid,'skillOrder':order},'SCSetSkillOrder');reject(first,first_stamp);cold();reject(first,first_stamp)
        second={'guid':target.guid,'cost':[{'id':foods[1]['id'],'guid':foods[1]['guid']}]};grown2,second_stamp=once('CSWarriorStrengthen',second,'SCWarriorStrengthen');self.assertEqual(sum(x.num for x in grown2.cost if x.id=='coin'),400);self.assertEqual(grown2.guid.skillOrder,order);reject(first,first_stamp);cold();keep(second,second_stamp,grown2);self.assertEqual(self.profile(login)[0]['coin'],259200)
        call('CSFormationOff',{'id':target.id,'guid':target.guid,'formatPos':2},'SCFormationOff');reset,reset_stamp=once('CSWarriorReset',{'guid':target.guid},'SCWarriorReset');self.assertEqual(sum(x.num for x in reset.cost if x.id=='diamond'),50);self.assertEqual(sum(x.num for x in reset.get.simps if x.id=='coin'),800);refunded=[x for x in reset.get.wars if x.id=='hero_exp_001'];self.assertEqual(len(refunded),2);self.assertTrue(all(x.guid not in [v['guid'] for v in foods] for x in refunded));self.assertEqual(reset.warriors[0].skillOrder,order);reject(second,second_stamp);cold();reject(second,second_stamp)
        sale,sale_stamp=once('CSWarriorSell',{'guid':[target.guid]},'SCWarriorSell');self.assertEqual(sum(x.num for x in sale.get.simps if x.id=='coin'),1000);reject(first,first_stamp);reject(second,second_stamp);cold();reject(second,second_stamp);saved=self.profile(login);self.assertEqual(self.call('CSWarriorSell',{'guid':[target.guid]},login,sale_stamp,'SCWarriorSell'),sale);self.assertEqual(self.profile(login),saved);op,msg=self.stamped('CSWarriorReset',{'guid':target.guid},login,reset_stamp);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved)
        p,g=self.profile(login);self.assertEqual((p['coin'],p['diamond']),(261000,240));self.assertNotIn(target.guid,[x['guid'] for x in g['warriors']]);self.assertEqual(len([x for x in g['warriors'] if x['id']=='hero_exp_001']),2)
        report={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_WARRIOR_STRENGTHEN_COMMITTED_RECEIPT_CURRENT_BODY_ORDER_REFUND_SALE_INNODB_COLD_PHONE_PENDING','actions':'actual27star0source260000coin350diamond/actual067hero011→actualtwoordinaryshop1item5purchase60diamondtotaltworealexpGUIDs→originalnum0each400coin400expstrengthen8once/currenthotcold→actualFormationOn2/currentbodycoldkeep→actualSkillOrder2,1oldwholegrowthrejecthotcold→actualsecondgrowth400coin/currentorderandbodycoldkeep→actualOff2/reset50diamondrefund800coin+twofresh001→oldgrowthrejecthotcold→actualSale1000/simpleSaleoldcoldkeep→oldgrowth/oldresetwholebodyreject;fullprofileunchangedonretry/reject','coldRestarts':cold_count,'actualSourceCoin':source_coin,'actualSourceDiamond':source_diamond,'actualMaterialPurchaseDiamond':purchase,'actualGrowthCoin':800,'actualGrowthExp':800,'actualFirstLevel':grown.guid.level,'actualFirstExp':grown.guid.exp,'actualSecondLevel':grown2.guid.level,'actualSecondExp':grown2.guid.exp,'actualResetDiamond':50,'actualRefundCoin':800,'actualRefundFreshMaterialCount':2,'actualSaleCoin':1000,'finalCoin':p['coin'],'finalDiamond':p['diamond'],'finitePrerequisites':'level100/guide8/bag1000/held067x1/priorchap27star5;noawardedGUID/material/coin/diamond/XP/Warrior/formationinjected;notnaturalhistoricalstars/gift/winner/phone','seconds':round(time.monotonic()-began,3),'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source,'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p5/warrior-strengthen-receipt-http-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
