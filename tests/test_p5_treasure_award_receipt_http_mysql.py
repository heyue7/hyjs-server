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

OWNED = ['hyjs-server/service/treasureService.go','hyjs-server/service/p5GrowthReceiptService.go','hyjs-server/service/p5TreasureAwardReceipt_test.go','tests/test_p5_treasure_award_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_TREASURE_AWARD_RECEIPT_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class TreasureAwardReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_treaward_http_' + secrets.token_hex(6)
        cls.test_user = 'p5trehttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-treaward-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-treaward-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_TREASURE_AWARD_RECEIPT_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/empty-gem-sale-manifest.json','analysis/p8/gem-thirteen-manifest.json','analysis/p5/copy-award-receipt-manifest.json','analysis/p5/copy-award-shared-source-20261015/manifest.json','analysis/p8/revive-receipt-manifest.json']:
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
            report = shared.ROOT / 'analysis/p5/treasure-award-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_treaward_http_') or not cls.test_user.startswith('p5trehttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_treasure_combine_box_consuming_growth_sale_and_cold(self):
        began=time.monotonic();self.start();uid='actual-original-treasure-awards';login=self.login(uid);stamp=1800510000000;cold_count=0
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
        def reject(name,fields,old):
            saved=self.profile(login);op,msg=self.stamped(name,fields,login,old);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved);return msg.msg
        call('CSPlayerCreate',{'playerName':'原饰品增量回执','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,equipBagSize=1000)
        g['items']=[{'id':x,'num':1} for x in ['frag_trea_001_1','frag_trea_001_2','frag_trea_001_3','item_115']]
        g['copy']['passedCopyInfo']=[{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0} for r in shared.original_table('CopyMission') if r['ChapterId']=='27'];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        source=call('CSCopyGetStarAward',{'chapterId':'27','starIndex':0},'SCCopyGetStarAward');earned=sum(x.num for x in source.gain.simps if x.id=='coin');self.assertEqual(earned,260000)
        combine={'targetId':'trea_001'};combined,combined_stamp=once('CSCombine',combine,'SCCombine');self.assertEqual(len(combined.get.treas),1);target=combined.get.treas[0]
        box={'costItem':{'id':'item_115','num':1}};opened,box_stamp=once('CSItemUse',box,'SCItemUse');self.assertEqual(len(opened.get.treas),1);food=opened.get.treas[0];self.assertIn(food.treasureId,['trea_015','trea_016','trea_017','trea_018'])
        cold();self.assertEqual(self.call('CSCombine',combine,login,combined_stamp,'SCCombine'),combined);self.assertEqual(self.call('CSItemUse',box,login,box_stamp,'SCItemUse'),opened)
        before_coin=self.profile(login)[0]['coin'];grown=call('CSTreasureStrengthen',{'targetId':target.guid,'guids':[food.guid]},'SCTreasureStrengthen');spent=before_coin-self.profile(login)[0]['coin'];self.assertEqual(spent,50000);self.assertGreater(grown.treasure.strengthLevel,1);self.assertFalse(any(x['guid']==food.guid for x in self.profile(login)[1]['treasures']))
        grown_errors=[reject('CSCombine',combine,combined_stamp),reject('CSItemUse',box,box_stamp)];cold();grown_cold=[reject('CSCombine',combine,combined_stamp),reject('CSItemUse',box,box_stamp)]
        sold,sold_stamp=once('CSTreasureSell',{'guids':[target.guid]},'SCTreasureSell');self.assertEqual(sum(x.num for x in sold.get.simps if x.id=='coin'),51000);self.assertEqual(len(sold.get.treas),1);refunded=sold.get.treas[0];self.assertEqual(refunded.treasureId,food.treasureId);self.assertNotIn(refunded.guid,[food.guid,target.guid])
        retired=[reject('CSCombine',combine,combined_stamp),reject('CSItemUse',box,box_stamp)];cold();retired_cold=[reject('CSCombine',combine,combined_stamp),reject('CSItemUse',box,box_stamp)]
        self.assertEqual(self.call('CSTreasureSell',{'guids':[target.guid]},login,sold_stamp,'SCTreasureSell'),sold);p,g=self.profile(login);self.assertEqual(p['coin'],261000);self.assertEqual(len(g['treasures']),1);self.assertEqual(g['treasures'][0]['guid'],refunded.guid)
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_TREASURE_AWARD_FULLENTITY_LIFECYCLE_PHONE_PENDING','actualOriginalStarCoin':earned,'actualCombineTarget':target.treasureId,'actualCombineCosts':[{'id':x.id,'num':x.num} for x in combined.cost],'actualBox115RandomResult':food.treasureId,'actualStrengthLevel':grown.treasure.strengthLevel,'actualStrengthCostCoin':spent,'originalInputFoodConsumed':True,'grownOldBodiesRejected':grown_errors,'grownColdOldBodiesRejected':grown_cold,'actualSaleOriginalBaseAndPaidRefundCoin':51000,'actualFoodRefundNewGUID':refunded.guid,'retiredOldBodiesRejected':retired,'retiredColdOldBodiesRejected':retired_cold,'currentSaleColdReply':'same','finalCoin':p['coin'],'eightOriginalCombineBoxAndSale':'exactonce/currentbody','coldRestarts':cold_count,'qualification':'level100/priorchapter27stars/bag1000/three originalfrag001 parts and115one held finite. No rawtreasure/GUID/growth/wallet insertion; existing1154equal sourceweights retained; coinactualstaraward thenactual50000strength/fullrefund; originalprivate box probability denominator unknown retains namednormalizedrule','frozenGoSHA256':self.source_shas,'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'baseline':self.baseline_source,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p5/treasure-award-receipt-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
