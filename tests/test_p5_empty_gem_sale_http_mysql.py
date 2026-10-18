"""Original worldseed currentnode/fullmain receipt originalHTTP lifecycle."""
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

OWNED = ['hyjs-server/service/p5RecycleService.go','hyjs-server/service/p5EmptyGemSale_test.go','tests/test_p5_empty_gem_sale_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_EMPTY_GEM_SALE_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class EmptyGemSaleHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_emptysale_http_' + secrets.token_hex(6)
        cls.test_user = 'p5empshttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-emptysale-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-emptysale-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_EMPTY_GEM_SALE_HTTP_BASE', shared.ROOT / 'build/candidates/joint-preview-caches-v78-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/award-collection-manifest.json','analysis/p5/refund-material-manifest.json','analysis/p5/world-seed-receipt-manifest.json','analysis/p8/gem-receipt-manifest.json']:
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
            report = shared.ROOT / 'analysis/p5/empty-gem-sale-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_emptysale_http_') or not cls.test_user.startswith('p5empshttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_empty_slots_sale_growth_refund_and_cold(self):
        began=time.monotonic();self.start();uid='actual-gem-receipt-lifecycle';login=self.login(uid);stamp=1800210000000;cold_count=0
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def once(name,fields,response):
            nonlocal stamp
            stamp+=1;saved_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool: replies=list(pool.map(lambda _:self.call(name,fields,login,saved_stamp,response),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));return replies[0],saved_stamp
        def reject_old(name,fields,old_stamp):
            saved=self.profile(login);opcode,_=self.stamped(name,fields,login,old_stamp);self.assertEqual(opcode,10001);self.assertEqual(self.profile(login),saved)
        def cold():
            nonlocal login,cold_count
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);cold_count+=1
        call('CSPlayerCreate',{'playerName':'宝石回执保护','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,warriorBagSize=1000,equipBagSize=1000,noviceGuideSchedule=8)
        g['items']=[{'id':'item_119','num':1}]
        g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in shared.original_table('CopyMission')];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        source_coin=0;source_boxes=0
        for chapter in range(27,0,-1):
            for box in range(3):
                award=call('CSCopyGetStarAward',{'chapterId':str(chapter),'starIndex':box},'SCCopyGetStarAward')
                source_coin+=next(x.num for x in award.gain.simps if x.id=='coin');source_boxes+=1
            if source_coin>=15100000:break
        self.assertGreaterEqual(source_coin,15100000)
        key='limitExchange_p4_gem_'+time.strftime('%Y%m',time.gmtime(time.time()+8*3600))
        gems=[];box_receipts=[]
        for box_id in ['item_342','item_343']:
            listing=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo');item=next(x for x in listing.shopItemList if x.getResource[0].id==box_id)
            request={'activityId':key,'index':item.index,'num':1,'costResources':item.costResource,'getResources':item.getResource}
            once('CSBuyActivityShop',request,'SCGetActivityShopReward')
            fields={'costItem':{'id':box_id,'num':1},'getItemIndex':1};award,old=once('CSItemUse',fields,'SCItemUse')
            gems.append(award.get.simps[0].id);box_receipts.append((fields,old,award))
        self.assertEqual(gems,['MineItem_025','MineItem_026'])
        gear=call('CSItemUse',{'costItem':{'id':'item_119','num':1},'getItemIndex':1},'SCItemUse').get.equips[0];self.assertEqual(gear.equipId,'equip_037')
        mount={'guid':gear.guid,'gemId':gems[0],'pos':0};mounted,mount_stamp=once('CSGemMount',mount,'SCGemMount')
        cold();self.assertEqual(self.call('CSGemMount',mount,login,mount_stamp,'SCGemMount'),mounted)
        quality=next(x['quality'] for x in shared.original_table('Equip') if x['id']==gear.equipId);cost=int(next(x['coin'] for x in shared.original_table('EquipExp') if x['quality']==quality and x['level']=='1'))
        before_coin=self.profile(login)[0]['coin'];once('CSEquipStrengthen',{'guid':gear.guid,'times':1,'cost':[{'id':'coin','num':cost}]},'SCEquipStrengthen');self.assertEqual(before_coin-self.profile(login)[0]['coin'],cost)
        reject_old('CSGemMount',mount,mount_stamp)
        change={'guid':gear.guid,'gemId':gems[1]};changed,change_stamp=once('CSGemMountOrChange',change,'SCGemMountOrChange')
        self.assertEqual(changed.equip.gems[0],gems[1]);self.assertEqual([(x.id,x.num) for x in changed.res.simps],[(gems[0],1)])
        cold();self.assertEqual(self.call('CSGemMountOrChange',change,login,change_stamp,'SCGemMountOrChange'),changed)
        unmount={'guid':gear.guid,'pos':0};unmounted,unmount_stamp=once('CSGemUnmount',unmount,'SCGemUnmount')
        self.assertEqual([(x.id,x.num) for x in unmounted.res.simps],[(gems[1],1)]);reject_old('CSGemMountOrChange',change,change_stamp)
        cold();self.assertEqual(self.call('CSGemUnmount',unmount,login,unmount_stamp,'SCGemUnmount'),unmounted)
        once('CSGemMount',mount,'SCGemMount');reject_old('CSGemUnmount',unmount,unmount_stamp)
        _,change_stamp=once('CSGemMountOrChange',change,'SCGemMountOrChange')
        sell={'cost':[{'id':gear.equipId,'guid':gear.guid,'num':1}]}
        reject_old('CSEquipmentSell',sell,stamp+1)
        # Successful unmount preserves four empty original hole positions.
        _,unmount_stamp=once('CSGemUnmount',unmount,'SCGemUnmount')
        current=next(x for x in self.profile(login)[1]['equipment'] if x['guid']==gear.guid)
        self.assertEqual(current['gems'],['','','',''])
        recycled,sell_stamp=once('CSEquipmentSell',sell,'SCEquipmentSell')
        self.assertFalse(any(x.id in gems for x in recycled.get.simps))
        reject_old('CSGemMountOrChange',change,change_stamp);cold();reject_old('CSGemMountOrChange',change,change_stamp);reject_old('CSGemUnmount',unmount,unmount_stamp)
        base_coin=int(next(x['sellPrice'] for x in shared.original_table('Equip') if x['id']==gear.equipId))
        self.assertEqual(sum(x.num for x in recycled.get.simps if x.id=='coin'),cost+base_coin)
        saved=self.profile(login);self.assertEqual(self.call('CSEquipmentSell',sell,login,sell_stamp,'SCEquipmentSell'),recycled);self.assertEqual(self.profile(login),saved)
        p,g=self.profile(login);self.assertFalse(g.get('equipment'));self.assertEqual(p['coin'],source_coin-15000000+base_coin)
        counts={x['id']:x['num'] for x in g['gems']};self.assertEqual(counts[gems[0]],1);self.assertEqual(counts[gems[1]],1)
        # Simple awarded gem descriptors can be replayed after real movement.
        for fields,old,award in box_receipts:
            saved=self.profile(login);self.assertEqual(self.call('CSItemUse',fields,login,old,'SCItemUse'),award);self.assertEqual(self.profile(login),saved)
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_EMPTY_GEM_SALE_PHONE_PENDING','actions':'actualstarcoin→existing7m342/8m343→originalred025026; actualheld119→037GUID;8mount/currentcold→paidstrengthen oldmountreject;8swap/currentcold→8unmount oldswapreject/currentcold→remount oldunmountreject→swap→mountedSale refused→actualfouremptyunmount→actualsale exactrefund/basecoin→coldretiredreject;simpleoldboxdescriptorspreserved','sourceCoin':source_coin,'sourceStarBoxes':source_boxes,'paidGemBoxesCoin':15000000,'actualStrengthenCost':cost,'originalSellBaseCoin':base_coin,'actualSaleTotalCoin':cost+base_coin,'finalCoin':p['coin'],'gemConservation':{x:counts[x] for x in gems},'coldRestarts':cold_count,'finitePrerequisites':'originalheld119/level100/guide8/bags1000/priororiginalmissionstar5;coin0diamond0/no gem/sourcebox/GUIDinjected;existingGemSupplyprice/sourceoriginalunknownpolicyretained','sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False,'baseline':self.baseline_source}
        (shared.ROOT/'analysis/p5/empty-gem-sale-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
