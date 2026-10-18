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

OWNED = ['hyjs-server/service/formationService.go','hyjs-server/service/p5FormationNextService.go','hyjs-server/service/p5FormationExchangeReceipt_test.go','tests/test_p5_formation_exchange_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_FORMATION_EXCHANGE_RECEIPT_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class FormationExchangeReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_exchange_http_' + secrets.token_hex(6)
        cls.test_user = 'p5exchangehttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-exchange-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-exchange-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_FORMATION_EXCHANGE_RECEIPT_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/treasure-award-receipt-manifest.json','analysis/p5/formation-receipt-manifest.json','analysis/p5/helper-receipt-manifest.json','analysis/p5/skill-order-receipt-manifest.json','analysis/p5/equipment-formation-receipt-manifest.json','analysis/p5/treasure-formation-receipt-manifest.json']:
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
            report = shared.ROOT / 'analysis/p5/formation-exchange-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_exchange_http_') or not cls.test_user.startswith('p5exchangehttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_exchange_current_full_list_equipment_guardian_growth_and_cold(self):
        began=time.monotonic();self.start();stamp=1800760000000;cold_count=0;uid='actual-formation-exchange-receipt';login=self.login(uid)
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def once(name,fields,response):
            nonlocal stamp
            stamp+=1;old=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool: replies=list(pool.map(lambda _:self.call(name,fields,login,old,response),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));return replies[0],old
        def cold():
            nonlocal login,cold_count
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);cold_count+=1
        exchange={'fromPos':1,'toPos':2}
        def current(old,reply):
            saved=self.profile(login);self.assertEqual(self.call('CSFormationExChange',exchange,login,old,'SCFormationExChange'),reply);self.assertEqual(self.profile(login),saved)
        def reject(old):
            saved=self.profile(login);op,msg=self.stamped('CSFormationExChange',exchange,login,old);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved);return msg.msg
        call('CSPlayerCreate',{'playerName':'原整阵换位','heroId':'hero_043'},'SCPlayerCreate');p,g=self.profile(login);p.update(level=100,noviceGuideSchedule=8,equipBagSize=1000,warriorBagSize=1000);self.assertEqual((p.get('coin',0),p.get('diamond',0)),(0,0));g['items']=[{'id':'item_067','num':1},{'id':'item_119','num':1}];g['copy']['passedCopyInfo']=[{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0} for r in shared.original_table('CopyMission') if r['ChapterId']=='27'];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        source=call('CSCopyGetStarAward',{'chapterId':'27','starIndex':0},'SCCopyGetStarAward');earned=sum(x.num for x in source.gain.simps if x.id=='coin');self.assertEqual(earned,260000);source_diamond=sum(x.num for x in source.gain.simps if x.id=='diamond')
        hero=call('CSItemUse',{'costItem':{'id':'item_067','num':1},'getItemIndex':1},'SCItemUse').get.wars[0];self.assertEqual(hero.id,'hero_011');gear=call('CSItemUse',{'costItem':{'id':'item_119','num':1},'getItemIndex':1},'SCItemUse').get.equips[0];self.assertEqual(gear.equipId,'equip_037');pet=once('CSPetFirstRecruit',{},'SCPetFirstRecruit')[0].get.pets[0];self.assertEqual(pet.id,'pet_002')
        call('CSFormationOn',{'id':hero.id,'guid':hero.guid,'formatPos':2},'SCFormationOn');call('CSFormationOn',{'id':gear.equipId,'guid':gear.guid,'formatPos':1},'SCFormationOn');call('CSFormationOn',{'id':pet.id,'guid':pet.guid,'formatPos':1},'SCFormationOn')
        first,first_stamp=once('CSFormationExChange',exchange,'SCFormationExChange');self.assertTrue(any(slot.formatPos==2 and any(x.guid==gear.guid for x in slot.things) and any(x.guid==pet.guid for x in slot.things) for slot in first.formationList));cold();current(first_stamp,first)
        call('CSFormationOff',{'id':gear.equipId,'guid':gear.guid,'formatPos':2},'SCFormationOff');reject(first_stamp);cold();reject(first_stamp)
        call('CSFormationOff',{'id':pet.id,'guid':pet.guid,'formatPos':2},'SCFormationOff');sale,sale_stamp=once('CSEquipmentSell',{'cost':[{'id':gear.equipId,'guid':gear.guid,'num':1}]},'SCEquipmentSell');self.assertEqual(sum(x.num for x in sale.get.simps if x.id=='coin'),1000);reject(first_stamp);cold();reject(first_stamp)
        clean,clean_stamp=once('CSFormationExChange',exchange,'SCFormationExChange');cold();current(clean_stamp,clean)
        before_wallet=self.profile(login)[0];buy=call('CSShopBuy',{'shopType':1,'shopItemId':'itemShopId_5','buyCount':1},'SCShopBuy');after_wallet=self.profile(login)[0];purchase=before_wallet['coin']-after_wallet['coin'];purchase_diamond=before_wallet.get('diamond',0)-after_wallet.get('diamond',0);self.assertEqual(purchase,0);self.assertEqual(purchase_diamond,30);material=next(x for x in self.profile(login)[1]['warriors'] if x['id']=='hero_exp_001');before=self.profile(login)[0]['coin'];growth=call('CSWarriorStrengthen',{'guid':hero.guid,'cost':[{'id':material['id'],'guid':material['guid'],'num':1}]},'SCWarriorStrengthen');growth_cost=before-self.profile(login)[0]['coin'];self.assertEqual(growth_cost,400);current(clean_stamp,clean);cold();current(clean_stamp,clean)
        p,g=self.profile(login);self.assertEqual((p['coin'],p.get('diamond',0)),(earned+1000-purchase-growth_cost,source_diamond-purchase_diamond));self.assertNotIn(gear.guid,[x['guid'] for x in g.get('equipment',[])]);formation=call('CSFormationList',{},'SCFormationList');self.assertFalse(any(x.pos in [1,7] for slot in formation.formationList for x in slot.things));self.assertEqual(len(call('CSPetList',{},'SCPetList').petList),1)
        report={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_FORMATION_EXCHANGE_CURRENT_WHOLE_LIST_AND_OWNERSHIP_INNODB_PHONE_PENDING','actions':'actual067hero011/119037/Firstpet002→originalwhole1↔2swap8once/currentcold→actualequipmentoff/oldswaphotcoldreject→actualguardianoff/equipmentSell1000/oldswapcoldreject→newcleanswap8once/currentcold→actualordinaryexpbuy/real400coingrowth/currentdescriptorreceiptstillvalidcold;fullprofileunchangedonreject','coldRestarts':cold_count,'actualSourceCoin':earned,'actualEquipmentSaleCoin':1000,'actualExperiencePurchaseCoin':purchase,'actualExperiencePurchaseDiamond':purchase_diamond,'actualSourceDiamond':source_diamond,'finalDiamond':p.get('diamond',0),'actualGrowthCoin':growth_cost,'finalCoin':p['coin'],'actualHeroExpAfterGrowth':next(x.get('exp',0) for x in g['warriors'] if x['guid']==hero.guid),'finitePrerequisites':'level100/guide8/bags1000/priorchap27star5/held067119oneeach;coin0diamond0/noawardedGUID/Pet/hero/equipment/XPmaterial/propsinjected;notnaturalhistoricalstars/gifts/phone/winner','sameTransactionOwnership':'currentlockedprofilefullFormationList descriptor equality;eachWar/Equip/TreasheldGUID-ID andPetGUID-ID fromsameTX. Descriptorallowsactualgrowth, doesnottransportoldwholeWarriorbody','seconds':round(time.monotonic()-began,3),'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source,'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p5/formation-exchange-receipt-http-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
