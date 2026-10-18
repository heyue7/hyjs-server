"""Original awarded warrior/equipment/treasure stale receipt HTTP lifecycle."""
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

OWNED = ['tests/test_p5_refund_material_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_REFUND_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class RefundMaterialHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_refund_http_' + secrets.token_hex(6)
        cls.test_user = 'p5rfhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-award-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-award-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_REFUND_HTTP_BASE', shared.ROOT / 'build/candidates/joint-catalogue-car-v65-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/gold-egg-manifest.json','analysis/p5/copy-special-manifest.json','analysis/p5/copy-reset-manifest.json','analysis/p5/copy-box-manifest.json','analysis/p8/ingredient-supply-manifest.json','analysis/p15/red-three-supply-manifest.json','analysis/p5/crystal-supply-manifest.json','analysis/p5/award-collection-manifest.json','analysis/p5/refund-material-manifest.json']:
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
            report = shared.ROOT / 'analysis/p5/refund-material-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_refund_http_') or not cls.test_user.startswith('p5rfhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_three_original_award_bodies_growth_sale_rejection_simple_and_cold(self):
        began=time.monotonic();self.start();uid='actual-refund-material-lifecycle';login=self.login(uid);stamp=1800191000000;actual_cold=0
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def once(name,fields,response):
            nonlocal stamp
            stamp+=1;saved_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call(name,fields,login,saved_stamp,response),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));return replies[0],saved_stamp
        def reject_old(fields,old_stamp):
            saved=self.profile(login);opcode,_=self.stamped('CSItemUse',fields,login,old_stamp);self.assertEqual(opcode,10001);self.assertEqual(self.profile(login),saved)
        def cold():
            nonlocal login,actual_cold
            actual_cold+=1
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved)
        call('CSPlayerCreate',{'playerName':'原箱奖励回执保护','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,warriorBagSize=1000,equipBagSize=1000,noviceGuideSchedule=8)
        g['items']=[x for x in (g.get('items') or []) if x['id'] not in ['item_067','item_051','item_040','item_041']]+[{'id':x,'num':1} for x in ['item_067','item_051','item_040','item_041']]
        g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in shared.original_table('CopyMission')];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        # Wallet sources are real original awards; held four boxes are a finite
        # declared prerequisite, with no awarded GUID or material inserted.
        for box in range(3):call('CSCopyGetStarAward',{'chapterId':'27','starIndex':box},'SCCopyGetStarAward')
        simple_fields={'costItem':{'id':'item_041','num':1}};simple,simple_stamp=once('CSItemUse',simple_fields,'SCItemUse');self.assertFalse(simple.get.wars or simple.get.equips or simple.get.treas)
        call('CSShopBuy',{'shopType':1,'shopItemId':'itemShopId_5','buyCount':1},'SCShopBuy')
        war_fields={'costItem':{'id':'item_067','num':1},'getItemIndex':1};war,war_stamp=once('CSItemUse',war_fields,'SCItemUse');self.assertEqual(len(war.get.wars),1);war_guid=war.get.wars[0].guid
        cold();self.assertEqual(self.call('CSItemUse',war_fields,login,war_stamp,'SCItemUse'),war)
        material=next(x for x in self.profile(login)[1]['warriors'] if x['id']=='hero_exp_001');before_coin=self.profile(login)[0]['coin']
        call('CSWarriorStrengthen',{'guid':war_guid,'cost':[{'id':material['id'],'guid':material['guid'],'num':1}]},'SCWarriorStrengthen');self.assertEqual(before_coin-self.profile(login)[0]['coin'],400)
        reject_old(war_fields,war_stamp);cold();reject_old(war_fields,war_stamp)
        sale_fields={'guid':[war_guid]};sale,sale_stamp=once('CSWarriorSell',sale_fields,'SCWarriorSell');reject_old(war_fields,war_stamp);self.assertFalse(any(x['guid']==war_guid for x in self.profile(login)[1]['warriors']))
        self.assertEqual(len(sale.get.wars),1);returned=sale.get.wars[0];self.assertEqual(returned.id,'hero_exp_001')
        self.assertEqual(self.call('CSWarriorSell',sale_fields,login,sale_stamp,'SCWarriorSell'),sale)
        once('CSWarriorSell',{'guid':[returned.guid]},'SCWarriorSell')
        saved=self.profile(login);opcode,_=self.stamped('CSWarriorSell',sale_fields,login,sale_stamp);self.assertEqual(opcode,10001);self.assertEqual(self.profile(login),saved)
        cold();saved=self.profile(login);opcode,_=self.stamped('CSWarriorSell',sale_fields,login,sale_stamp);self.assertEqual(opcode,10001);self.assertEqual(self.profile(login),saved)
        equip_fields={'costItem':{'id':'item_051','num':1}};equip,equip_stamp=once('CSItemUse',equip_fields,'SCItemUse');self.assertEqual(len(equip.get.equips),1);gear=equip.get.equips[0]
        cold();self.assertEqual(self.call('CSItemUse',equip_fields,login,equip_stamp,'SCItemUse'),equip)
        quality=next(x['quality'] for x in shared.original_table('Equip') if x['id']==gear.equipId);cost=int(next(x['coin'] for x in shared.original_table('EquipExp') if x['quality']==quality and x['level']=='1'))
        once('CSEquipStrengthen',{'guid':gear.guid,'times':1,'cost':[{'id':'coin','num':cost}]},'SCEquipStrengthen');reject_old(equip_fields,equip_stamp)
        cold();reject_old(equip_fields,equip_stamp)
        once('CSEquipmentSell',{'cost':[{'id':gear.equipId,'guid':gear.guid,'num':1}]},'SCEquipmentSell');reject_old(equip_fields,equip_stamp);self.assertFalse(any(x['guid']==gear.guid for x in (self.profile(login)[1].get('equipment') or [])))
        treasure_fields={'costItem':{'id':'item_040','num':1}};treasure,treasure_stamp=once('CSItemUse',treasure_fields,'SCItemUse');self.assertEqual(len(treasure.get.treas),2);target=treasure.get.treas[0]
        cold();self.assertEqual(self.call('CSItemUse',treasure_fields,login,treasure_stamp,'SCItemUse'),treasure)
        once('CSTreasureRefine',{'targetId':target.guid},'SCTreasureRefine');reject_old(treasure_fields,treasure_stamp);cold();reject_old(treasure_fields,treasure_stamp)
        once('CSTreasureSell',{'guids':[target.guid]},'SCTreasureSell');reject_old(treasure_fields,treasure_stamp);self.assertFalse(any(x['guid']==target.guid for x in self.profile(login)[1]['treasures']))
        p,g=self.profile(login);p['stamina']=10;self.write_profile(login,p,g);call('CSItemUse',{'costItem':{'id':'item_001','num':1}},'SCItemUse');cold()
        before=self.profile(login);self.assertEqual(self.call('CSItemUse',simple_fields,login,simple_stamp,'SCItemUse'),simple);self.assertEqual(self.profile(login),before)
        for fields,old_stamp in [(war_fields,war_stamp),(equip_fields,equip_stamp),(treasure_fields,treasure_stamp)]:reject_old(fields,old_stamp)
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_AWARD_AND_REFUND_LIFECYCLE_PHONE_PENDING','actualRefundMaterialRetired':returned.id,'refundHeldOldReplay':True,'refundConsumedOldRejectedAndCold':True,'actualWarriorAward':war.get.wars[0].id,'actualEquipmentAward':gear.equipId,'actualTreasuresAward':[x.treasureId for x in treasure.get.treas],'actions':'actual4heldboxuse/8once/unchangedcold;realstardiamond+simplegiftcoin→ordinaryheroexp001→400coinwarstrengthen;realgearstrengthen/refine/sellall3types;grown/retiredoldCSItemUse reject fullprofileunchanged;simpleonlyconsume→coldoldreceipt preserved','coldRestarts':actual_cold,'finitePrerequisites':'level100/guide8/bags1000/priororiginalmissionstar5 and4heldboxes067,051,040,041;coin0diamond0/no awardedWar/Equip/TreasGUID injected;actualwallet+growthmaterialsource as noted;no naturalboxproduction or phone claim','sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False,'baseline':self.baseline_source}
        (shared.ROOT/'analysis/p5/refund-material-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()

