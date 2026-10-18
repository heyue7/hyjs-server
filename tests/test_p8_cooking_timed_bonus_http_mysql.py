"""Fresh original cooking rank through daily bounded real fishing/crafting/use."""
import concurrent.futures
import math
import http.client
from server.codec import BY_NAME, message, encode_http_request, decode_responses
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

OWNED = ['tests/test_p8_cooking_timed_bonus_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P8_COOKING_TIMED_BONUS_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class CookingTimedBonusHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p8_cooknatess_http_' + secrets.token_hex(6)
        cls.test_user = 'p8minehttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p8-cooknatess-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-cooknatess-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P8_COOKING_TIMED_BONUS_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/empty-gem-sale-manifest.json','analysis/p8/gem-thirteen-manifest.json','analysis/p5/copy-award-receipt-manifest.json','analysis/p5/copy-award-shared-source-20261015/manifest.json','analysis/p8/revive-receipt-manifest.json','analysis/p5/treasure-award-receipt-manifest.json','analysis/p5/battle-award-projection-manifest.json','analysis/p5/battle-award-shared-source-20261015/manifest.json']:
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
            report = shared.ROOT / 'analysis/p8/cooking-timed-bonus-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p8_cooknatess_http_') or not cls.test_user.startswith('p8minehttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_all_three_timed_recipe_paid_sweep_bonus_expiry_and_cold(self):
        began=time.monotonic();self.start();stamp=1800620000000;rows=[];cold_count=0
        from datetime import datetime,timezone,timedelta
        month=datetime.now(timezone(timedelta(hours=8))).strftime('%Y%m');root_key='limitExchange_p4_resources_'+month;child='limitExchange_p4_ingredients_'+month;recipes={r['FishItemID']:r for r in shared.original_table('FishSkill')};missions=shared.original_table('CopyMission');minutes=int(next(r['NameValue'] for r in shared.original_table('GameConfigure') if r['StaticName']=='LifeSkillFightTenTimeCost'))
        for item_id in ['FishItem_043','FishItem_044','FishItem_045']:
            uid='actual-cooking-bonus-'+item_id;login=self.login(uid);row=recipes[item_id]
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
            def effect():
                parts=self.sql(f'SELECT kind,value,end_at FROM cooking_effects WHERE player_id={login.playerId};').strip().split('\t');return parts[0],float(parts[1]),int(parts[2])
            def currencies(reply):return tuple(sum(x.num for x in reply.gain.simps if x.id==rid) for rid in ['coin','exp'])
            call('CSPlayerCreate',{'playerName':'原时效料理'+item_id[-3:],'heroId':'hero_043'},'SCPlayerCreate')
            p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,stamina=100,equipBagSize=1000,warriorBagSize=1000,noviceGuideSchedule=8)
            g['copy']['passedCopyInfo']=[{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0,'isOpen':1} for r in missions];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g);self.sql(f'INSERT INTO cooking_craft_states(player_id,level,exp) VALUES({login.playerId},5,0);')
            if not rows:
                listing=call('CSActivityShopInfo',{'activityId':root_key},'SCActivityShopInfo');self.assertFalse(any(any(x.id.startswith('FishItem_') for x in sku.getResource) for sku in listing.shopItemList));self.sql(f"UPDATE p4_commerce_rules SET enabled=1 WHERE id='{child}';")
            listing=call('CSActivityShopInfo',{'activityId':root_key},'SCActivityShopInfo');products={sku.getResource[0].id:sku for sku in listing.shopItemList if sku.getResource and sku.getResource[0].id.startswith('FishItem_')}
            costs=[(p.split(',')[0],int(p.split(',')[1])) for p in row['Material'].split(';')];raw=[(rid,n) for rid,n in costs if rid!='coin'];craft_coin=next(n for rid,n in costs if rid=='coin');required=craft_coin+sum(products[rid].costResource[0].num for rid,_ in raw);earned=0;stars=0
            for chapter in reversed(shared.original_table('CopyChapter')):
                for box in range(3):
                    gain=call('CSCopyGetStarAward',{'chapterId':chapter['ChapterId'],'starIndex':box},'SCCopyGetStarAward').gain;earned+=sum(x.num for x in gain.simps if x.id=='coin');stars+=1
                    if earned>=required:break
                if earned>=required:break
            bought=0
            for rid,num in raw:
                sku=products[rid];self.assertEqual(sku.getResource[0].num,num);fields={'activityId':root_key,'index':sku.index,'num':1,'costResources':sku.costResource,'getResources':sku.getResource};once('CSBuyActivityShop',fields,'SCGetActivityShopReward');bought+=sku.costResource[0].num
            sweep={'chapterId':'1','missionId':'1'};base=call('CSCopySweep',sweep,'SCCopySweep');base_coin,base_exp=currencies(base)
            craft={'fish':{'id':item_id,'num':1}};made,craft_stamp=once('CSFishCooking',craft,'SCFishCooking');use={'costItem':{'id':item_id,'num':1},'heroGuid':self.profile(login)[0]['heroWarId']};used,use_stamp=once('CSFishUsing',use,'SCFishUsing');kind,ratio,before_end=effect();self.assertEqual(kind,row['ValueType']);self.assertAlmostEqual(ratio,float(row['Value'].split(';')[1])/100);self.assertGreater(before_end,int(time.time()*1000)+29*60000)
            cold();self.assertEqual(effect(),(kind,ratio,before_end));self.assertEqual(self.call('CSFishUsing',use,login,use_stamp,'SCFishUsing'),used)
            boosted,boost_stamp=once('CSCopySweep',sweep,'SCCopySweep');actual_coin,actual_exp=currencies(boosted);expected_coin=math.floor(base_coin*(1+ratio)) if kind in ['extraCoin','extraCoinExp'] else base_coin;expected_exp=math.floor(base_exp*(1+ratio)) if kind in ['extraExp','extraCoinExp'] else base_exp;self.assertEqual((actual_coin,actual_exp),(expected_coin,expected_exp));self.assertEqual(effect()[2],before_end-minutes*60000)
            exact_after=effect();self.assertEqual(self.call('CSFishUsing',use,login,use_stamp,'SCFishUsing'),used);self.assertEqual(effect(),exact_after);self.assertEqual(self.call('CSFishCooking',craft,login,craft_stamp,'SCFishCooking'),made);self.assertEqual(effect(),exact_after)
            cold();self.assertEqual(effect(),exact_after);saved=self.profile(login);self.assertEqual(self.call('CSCopySweep',sweep,login,boost_stamp,'SCCopySweep'),boosted);self.assertEqual(self.profile(login),saved);self.assertEqual(effect(),exact_after)
            self.sql(f'UPDATE cooking_effects SET end_at=1 WHERE player_id={login.playerId};');self.assertFalse(call('CSCookingEffectList',{},'SCCookingEffectList').effectList);saved=self.profile(login);self.assertEqual(self.call('CSCopySweep',sweep,login,boost_stamp,'SCCopySweep'),boosted);self.assertEqual(self.profile(login),saved);self.assertEqual(effect()[2],1)
            plain=call('CSCopySweep',sweep,'SCCopySweep');self.assertEqual(currencies(plain),(base_coin,base_exp));self.assertEqual(effect()[2],1);p,g=self.profile(login);self.assertEqual(p['stamina'],70);self.assertEqual(p['coin'],earned-bought-craft_coin+base_coin+actual_coin+base_coin);self.assertFalse(any(x['id']==item_id and x['num']>0 for x in (g.get('cooking') or [])))
            rows.append({'item':item_id,'kind':kind,'ratio':ratio,'actualSourceStarBoxes':stars,'actualStarCoin':earned,'actualRawSourceCoin':bought,'actualCraftCoin':craft_coin,'actualUnboostedCoinExp':[base_coin,base_exp],'actualBoostedCoinExp':[actual_coin,actual_exp],'originalSweepTimeCostMinutes':minutes,'actualEffectEndBefore':before_end,'actualEffectEndAfter':exact_after[2],'eightSweepsSingleTimeCost':True,'expiredNextOriginalSweepNoBonus':True,'expiredOriginalCurrentReceiptNoRecost':True,'paidUseOldReceiptNoTimerRefill':True,'actualStamina':30,'finalCoin':p['coin'],'coldRestarts':2})
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_THREE_TIMED_MEALS_REAL_PAID_SWEEP_BONUS_EXPIRY_PHONE_PENDING','actualCases':rows,'coldRestarts':cold_count,'qualification':'level100/guide8/priororiginalstars/capacity/Cooking5 finite; no cookingXP/ranknaturalclaim. Actualstarcoin→nameddefaultoffhighingredient supply enabledonly dedicatedschema→originalmake/use→three actualpaid sweeps/30stamina andsourceawards. Onlyeffect.end_at=1 is dedicatedexpiryclock qualification; no sourcecoin/raw/dish/effect/value/winnerfacts inserted','frozenGoSHA256':self.source_shas,'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'baseline':self.baseline_source,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p8/cooking-timed-bonus-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
