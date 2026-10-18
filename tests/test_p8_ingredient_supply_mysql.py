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

OWNED = ['hyjs-server/service/p8IngredientSupplyService.go', 'hyjs-server/service/p8IngredientSupply_test.go', 'hyjs-server/constUtil/p8IngredientSupply.go', 'hyjs-server/service/p4CommerceService.go', 'hyjs-server/service/p4ResourceExchangeService.go', 'hyjs-server/db/sql/83_p4_commerce_rules.seed.sql', 'analysis/p8/ingredient-supply-original-audit.json', 'tests/test_p8_ingredient_supply_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P8_INGREDIENT_MYSQL_TEST') == '1', 'Explicit locked isolated P8 ingredients')
class IngredientSupplyMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p8_ingredient_' + secrets.token_hex(6)
        cls.test_user = 'p8food_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-ingredients-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-gem-supply')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P8_INGREDIENT_BASE', shared.ROOT / 'build/candidates/joint-catalogue-car-v65-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p4/resource-exchange-manifest.json','analysis/p5/gold-egg-manifest.json','analysis/p4/commerce-title-manifest.json']:
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
            if left.split() != ['0','0','0']: raise AssertionError('P8 ingredients schema/user/grants remain')
            report = shared.ROOT / 'analysis/p8/ingredient-supply-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p8_ingredient_') or not cls.test_user.startswith('p8food_'):raise AssertionError('P8 ingredients isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P8 ingredients administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_actual_coin_source_all13_recipe_make_use_and_cold(self):
        began=time.monotonic();self.start();stamp=1800160000000;missions=shared.original_table('CopyMission');recipes=[x for x in shared.original_table('FishSkill') if x['IsDish']=='1' and int(x['LevelLimit'])>=4];report=[];source_coin_total=0;paid_total=0;craft_total=0
        from datetime import datetime,timezone,timedelta
        month=datetime.now(timezone(timedelta(hours=8))).strftime('%Y%m');key='limitExchange_p4_resources_'+month;child='limitExchange_p4_ingredients_'+month
        for row in recipes:
            uid='actual-cooking-'+row['FishItemID'];login=self.login(uid)
            def call(name,fields,response):
                nonlocal stamp
                stamp+=1;return self.call(name,fields,login,stamp,response)
            call('CSPlayerCreate',{'playerName':'原高阶料理'+row['FishItemID'][-3:],'heroId':'hero_043'},'SCPlayerCreate')
            p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,stamina=1000,equipBagSize=1000,warriorBagSize=1000,noviceGuideSchedule=8)
            g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions];g['copy']['passedChapterInfo']=[];g['run']=None
            if row['FishItemID']=='FishItem_050':g['items']=(g.get('items') or [])+[{'id':'item_043','num':1}]
            self.write_profile(login,p,g);self.sql(f'INSERT INTO cooking_craft_states(player_id,level,exp) VALUES({login.playerId},6,0);')
            if not report:
                default=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo')
                self.assertFalse(any(any(x.id.startswith('FishItem_') for x in item.getResource) for item in default.shopItemList))
                self.assertEqual(self.sql(f"SELECT enabled,OCTET_LENGTH(COALESCE(frozen,'')) FROM p4_commerce_rules WHERE id='{child}';").strip(),'0\t0')
                self.sql(f"UPDATE p4_commerce_rules SET enabled=1 WHERE id='{child}';")
            listing=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo')
            products={item.getResource[0].id:item for item in listing.shopItemList if item.getResource and item.getResource[0].id.startswith('FishItem_')}
            self.assertEqual(len(products),10)
            costs=[(part.split(',')[0],int(part.split(',')[1])) for part in row['Material'].split(';')];raw=[(rid,num) for rid,num in costs if rid!='coin'];craft_coin=next(num for rid,num in costs if rid=='coin');required=sum(products[rid].costResource[0].num for rid,_ in raw)+craft_coin
            natural=0;stars=0
            for chapter in reversed(shared.original_table('CopyChapter')):
                for box in range(3):
                    reward=call('CSCopyGetStarAward',{'chapterId':chapter['ChapterId'],'starIndex':box},'SCCopyGetStarAward');natural+=sum(x.num for x in reward.gain.simps if x.id=='coin');stars+=1
                    if natural>=required:break
                if natural>=required:break
            self.assertGreaterEqual(natural,required);self.assertEqual(self.profile(login)[0]['coin'],natural)
            paid=0;last_buy=None;last_request=None;last_buy_stamp=None
            for rid,num in raw:
                product=products[rid];self.assertEqual(product.getResource[0].num,num)
                req={'activityId':key,'index':product.index,'num':1,'costResources':product.costResource,'getResources':product.getResource};stamp+=1;buy_stamp=stamp
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:results=list(pool.map(lambda _:self.call('CSBuyActivityShop',req,login,buy_stamp,'SCGetActivityShopReward'),range(8)))
                self.assertTrue(all(x==results[0] for x in results));paid+=product.costResource[0].num;last_buy,last_request,last_buy_stamp=results[0],req,buy_stamp
                saved=self.profile(login);stamp+=1;self.assertEqual(self.stamped('CSBuyActivityShop',req,login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            if row['FishItemID']=='FishItem_050':
                call('CSItemUse',{'costItem':{'id':'item_043','num':1}},'SCItemUse')
                call('CSItemUse',{'costItem':{'id':'item_065','num':1}},'SCItemUse')
                self.assertIn('SkillFishItem_051',call('CSCookingLevel',{},'SCCookingLevel').skills)
            craft={'fish':{'id':row['FishItemID'],'num':1}};stamp+=1;craft_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:made=list(pool.map(lambda _:self.call('CSFishCooking',craft,login,craft_stamp,'SCFishCooking'),range(8)))
            self.assertTrue(all(x==made[0] for x in made));self.assertEqual([(x.id,x.num) for x in made[0].res.simps],[(row['FishItemID'],1)])
            self.assertEqual(self.profile(login)[0]['coin'],natural-paid-craft_coin)
            if row['ValueType']=='onetimeRecover':
                call('CSTransferScene',{'target':'field_001'},'SCTransferScene');call('CSLoginScene',{},'SCLoginScene');call('CSCurrentPosition',{},'SCCurrentPosition')
                p,g=self.profile(login);g['field']['side'].update(curHp=0.4,state=0,battlePosHps=[]);g['field'].pop('fight',None);g['position']['current']={'x':1000,'y':0,'z':1000};self.write_profile(login,p,g)
                use_name,use_response='CSHpRecover','SCHpRecover';use={'costItem':{'id':row['FishItemID'],'num':1}}
            else:
                use_name,use_response='CSFishUsing','SCFishUsing';use={'costItem':{'id':row['FishItemID'],'num':1},'heroGuid':self.profile(login)[0]['heroWarId']}
            stamp+=1;use_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:used=list(pool.map(lambda _:self.call(use_name,use,login,use_stamp,use_response),range(8)))
            self.assertTrue(all(x==used[0] for x in used));saved=self.profile(login)
            self.assertFalse(any(x['id']==row['FishItemID'] and x['num'] for x in saved[1].get('cooking',[])))
            if row['ValueType']=='onetimeRecover':self.assertGreater(saved[1]['field']['side']['curHp'],0.4);effect={'hpBefore':0.4,'hpAfter':saved[1]['field']['side']['curHp']}
            elif row['ValueType'].startswith('extra'):
                effect_raw=self.sql(f"SELECT kind,value,end_at FROM cooking_effects WHERE player_id={login.playerId};").strip().split('\t');self.assertEqual(effect_raw[0],row['ValueType']);self.assertGreater(float(effect_raw[1]),0);self.assertGreater(int(effect_raw[2]),int(time.time()*1000));effect={'kind':effect_raw[0],'ratio':float(effect_raw[1]),'endAt':int(effect_raw[2])}
            else:
                prop=next(x for x in saved[1]['warriors'] if x['guid']==saved[0]['heroWarId'])['cooking']['prop'];effect={k:prop.get(k,0) for k in row['Value'].split(';')};self.assertTrue(all(v>0 for v in effect.values()))
            self.stop();self.start();login=self.login(uid)
            self.assertEqual(self.profile(login),saved);self.assertEqual(self.call(use_name,use,login,use_stamp,use_response),used[0]);self.assertEqual(self.call('CSFishCooking',craft,login,craft_stamp,'SCFishCooking'),made[0]);self.assertEqual(self.call('CSBuyActivityShop',last_request,login,last_buy_stamp,'SCGetActivityShopReward'),last_buy)
            source_coin_total+=natural;paid_total+=paid;craft_total+=craft_coin;report.append({'recipe':row['FishItemID'],'rawCost':raw,'sourceStarBoxes':stars,'actualSourceCoin':natural,'actualPaidSupplyCoin':paid,'actualCraftCoin':craft_coin,'actualEffect':effect})
        self.sql(f"UPDATE p4_commerce_rules SET enabled=0 WHERE id='{child}';");self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.sql(f"SELECT enabled FROM p4_commerce_rules WHERE id='{child}';").strip(),'0')
        self.assertEqual(self.call('CSBuyActivityShop',last_request,login,last_buy_stamp,'SCGetActivityShopReward'),last_buy)
        listing=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo');self.assertFalse(any(any(x.id.startswith('FishItem_') for x in item.getResource) for item in listing.shopItemList))
        data={'date':'2026-10-14','status':'PASS','policy':'cooking-ingredient-mainline-coin-exchange-v1','defaultOff':True,'all13OriginalRecipes':report,'actualSourceCoin':source_coin_total,'actualPaidSupplyCoin':paid_total,'actualCraftCoin':craft_total,'eightOnce':'each originalbuy/craft/use exactlyonce;daycap;cold all13profile/current use/craft/source receipts;offoldreceipt','preparation':'13 independent finite level100/cooking6/stamina1000/bag1000/prior originalstars accounts;zero sourcecoin/diamonds;no rawfish/dish/skills/sourcecoin inserted. recipe050 finite originalitem043gift prerequisite->actualboxitem065->actuallearn;no claimoriginaltopgift naturalproduction. Healing only finite injury0.4/outsideSafePosition1000, no boss/win facts','originalBoundary':'4FieldMap bundles absent; natural highmap/WorldBoss authorities remain absent; ingredients are explicitly defaultoff configurablelocalcoinsource. Permanent magnitude usesexisting namedSQLrule unchanged. Phone pending root.','productionModified':False,'phoneUI':False,'seconds':round(time.monotonic()-began,3),'sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source}
        (shared.ROOT/'analysis/p8/ingredient-supply-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
