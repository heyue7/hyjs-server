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

OWNED = ['hyjs-server/service/p5GemSupplyService.go', 'hyjs-server/service/p5GemSupply_test.go', 'hyjs-server/constUtil/p5GemSupply.go', 'hyjs-server/service/p4CommerceService.go', 'hyjs-server/service/p4ActivityService.go', 'hyjs-server/db/sql/83_p4_commerce_rules.seed.sql', 'analysis/p5/gem-box-original-source-audit.json', 'tests/test_p5_gem_supply_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_GEM_SUPPLY_MYSQL_TEST') == '1', 'Explicit locked isolated P5 gem supply')
class GemSupplyMySQLTests(unittest.TestCase):
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
        cls.lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        fcntl.flock(cls.lock.fileno(), fcntl.LOCK_EX)
        cls.addClassCleanup(cls.lock.close)
        cls.database = 'hyjs_test_p5_gem_supply_' + secrets.token_hex(6)
        cls.test_user = 'p5gemsupply_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-gem-supply-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-gem-supply')
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")
        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0','0','0']: raise AssertionError('P5 gem supply schema/user/grants remain')
            report = shared.ROOT / 'analysis/p5/gem-supply-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_GEM_SUPPLY_BASE', shared.ROOT / 'build/candidates/joint-chat-seasonal-v57a-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for rel in OWNED:
            if rel.startswith('hyjs-server/'):
                target=source/rel.removeprefix('hyjs-server/');target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(shared.ROOT/rel,target)
        cls.frozen_source=source;cls.baseline_source=str(base.relative_to(shared.ROOT))
        (Path(cls.temp.name)/'analysis').symlink_to(shared.ROOT/'analysis',target_is_directory=True)
        cls.source_shas={str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(source.rglob('*.go'))}
        result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=source,capture_output=True,timeout=180)
        if result.returncode: raise AssertionError((result.stdout+result.stderr).decode(errors='replace')[-4000:])

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_gem_supply_') or not cls.test_user.startswith('p5gemsupply_'):raise AssertionError('P5 gem supply isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 gem supply administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_five_boxes_source_coin_original_stones_actual_mount_and_cold(self):
        began=time.monotonic();self.start();stamp=1800080000000;results=[]
        ids=['item_137','item_342','item_343','item_344','item_345'];prices=[2000000,7000000,8000000,9000000,10000000]
        month=time.strftime('%Y%m',time.gmtime(time.time()+8*3600));key='limitExchange_p4_gem_'+month
        missions=shared.original_table('CopyMission');original_use=shared.original_table('ItemUse');mine=shared.original_table('MineSkill')
        for index,box_id in enumerate(ids):
            uid='natural-gem-supply-'+str(index);login=self.login(uid)
            def call(name,fields,response):
                nonlocal stamp
                stamp+=1;return self.call(name,fields,login,stamp,response)
            call('CSPlayerCreate',{'playerName':'宝石自然经济'+str(index),'heroId':'hero_043'},'SCPlayerCreate')
            p,g=self.profile(login);p.update(level=100,coin=0,noviceGuideSchedule=8)
            g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions]
            g['copy']['passedChapterInfo']=[]
            self.write_profile(login,p,g)
            source_coin=0;source_boxes=0
            for chapter in range(27,0,-1):
                for box in range(3):
                    result=call('CSCopyGetStarAward',{'chapterId':str(chapter),'starIndex':box},'SCCopyGetStarAward')
                    source_coin+=next(x.num for x in result.gain.simps if x.id=='coin');source_boxes+=1
                if source_coin>=prices[index]:break
            self.assertGreaterEqual(source_coin,prices[index]);self.assertEqual(self.profile(login)[0]['coin'],source_coin)
            listing=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo');item=listing.shopItemList[index]
            self.assertEqual((item.name,item.getResource[0].id,item.getResource[0].num,item.costResource[0].num),(next(x['name'] for x in shared.original_table('Item') if x['itemId']==box_id),box_id,1,prices[index]))
            req={'activityId':key,'index':item.index,'num':1,'costResources':item.costResource,'getResources':item.getResource}
            before=self.profile(login);stamp+=1
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool: bought=list(pool.map(lambda _:self.call('CSBuyActivityShop',req,login,stamp,'SCGetActivityShopReward'),range(8)))
            self.assertTrue(all(x==bought[0] for x in bought));self.assertEqual(self.profile(login)[0]['coin'],source_coin-prices[index])
            self.assertEqual(self.stamped('CSBuyActivityShop',req,login,stamp+1)[0],10001)
            options=[x for x in original_use if x['itemId']==box_id]
            choice=0 if index==0 else 1;open_req={'costItem':{'id':box_id,'num':1},'getItemIndex':choice};stamp+=1;open_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool: opened=list(pool.map(lambda _:self.call('CSItemUse',open_req,login,open_stamp,'SCItemUse'),range(8)))
            self.assertTrue(all(x==opened[0] for x in opened));gem=opened[0].get.simps[0]
            self.assertIn(gem.id,[x['result'].split(',')[0] for x in options]);self.assertEqual(gem.num,1)
            if index>0:self.assertEqual(gem.id,next(x['result'].split(',')[0] for x in options if int(x['odds'])==choice))
            p,g=self.profile(login);self.assertEqual(sum(x['num'] for x in g['gems'] if x['id']==gem.id),1)
            self.assertEqual(next((x['num'] for x in g['items'] if x['id']==box_id),0),0)
            saved=self.profile(login);self.stop();self.start();login=self.login(uid)
            self.assertEqual(self.call('CSItemUse',open_req,login,open_stamp,'SCItemUse'),opened[0]);self.assertEqual(self.profile(login),saved)
            details={'box':box_id,'sourceCoin':source_coin,'sourceOriginalStarBoxes':source_boxes,'paidCoin':prices[index],'actualGem':gem.id,'8buyAndBoxFrames':'exact once/cold'}
            if index==4:
                # Finite original119 equipment box is an explicitly separate prerequisite.
                # No gem/coin/gem-source boxes or equipment GUIDs are injected.
                p,g=self.profile(login);g['items']=[x for x in g['items'] if x['id']!='item_119']+[{'id':'item_119','num':1}]
                g['run']={'chapter':'27','mission':'161','passed':[]};self.write_profile(login,p,g)
                gear=call('CSItemUse',{'costItem':{'id':'item_119','num':1},'getItemIndex':1},'SCItemUse').get.equips[0]
                call('CSFormationOn',{'formatPos':1,'id':gear.equipId,'guid':gear.guid},'SCFormationOn')
                before_map=call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo')
                before_att=before_map.battlePlayer.formation[0].prop.att
                mounted=call('CSGemMount',{'guid':gear.guid,'gemId':gem.id,'pos':0},'SCGemMount')
                self.assertEqual(mounted.equip.gems[0],gem.id)
                after_map=call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo')
                delta=after_map.battlePlayer.formation[0].prop.att-before_att
                self.assertEqual(delta,4000)
                restored=call('CSGemUnmount',{'guid':gear.guid,'pos':0},'SCGemUnmount')
                self.assertEqual([(x.id,x.num) for x in restored.res.simps],[(gem.id,1)])
                details.update(actualOriginalAttackDelta=delta,unmountedExactGem=True)
                saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved)
            results.append(details)
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='p4_commerce_rules';").strip(),'InnoDB')
        report={'date':'2026-10-14','policy':'gem-mainline-coin-exchange-v1','results':results,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False,'sourceUnknown':'five original boxes have no production row across164 tables; monthly prices/gates/limits are explicitly configurable local policy; exact original ItemUse probability/selection preserved','preparation':'five independent level100/guide8 accounts and finite original historical stars; no coins/gems/gem boxes inserted; for grade10 actual downstream property only original119 equipment box/run27 eligibility prepared, actual GUID through original CSItemUse','cold':'five exact current box receipts and complete saved inventory; final mounted/unmounted gem and real battle property survive restart','sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'cleanup':'whole-life SQLfilelock/dedicated schema-user-grants/temp source-binary'}
        (shared.ROOT/'analysis/p5/gem-supply-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
