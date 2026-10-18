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

OWNED = ['hyjs-server/service/p5Service.go','hyjs-server/service/p5GrowthReceiptService.go','hyjs-server/service/p5WorldSeedReceipt_test.go','tests/test_p5_world_seed_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_WORLD_SEED_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class WorldSeedReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_seed_http_' + secrets.token_hex(6)
        cls.test_user = 'p5seedhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-seed-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-seed-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_WORLD_SEED_HTTP_BASE', shared.ROOT / 'build/candidates/joint-preview-caches-v78-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/award-collection-manifest.json','analysis/p5/refund-material-manifest.json']:
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
            report = shared.ROOT / 'analysis/p5/world-seed-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_seed_http_') or not cls.test_user.startswith('p5seedhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_current_world_node_identity_growth_and_cold(self):
        began=time.monotonic();self.start();uid='actual-worldseed-receipt-lifecycle';login=self.login(uid);stamp=1800200000000;cold_count=0
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def once(fields):
            nonlocal stamp
            stamp+=1;saved_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool: replies=list(pool.map(lambda _:self.call('CSLightenWorldSeed',fields,login,saved_stamp,'SCLightenWorldSeed'),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));return replies[0],saved_stamp
        def reject_old(node,old_stamp):
            saved=self.profile(login);opcode,_=self.stamped('CSLightenWorldSeed',{'id':node},login,old_stamp);self.assertEqual(opcode,10001);self.assertEqual(self.profile(login),saved)
        def cold():
            nonlocal login,cold_count
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);cold_count+=1
        call('CSPlayerCreate',{'playerName':'世界之种回执保护','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,warriorBagSize=1000,noviceGuideSchedule=8)
        g['items']=[{'id':'item_017','num':25}]
        g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in shared.original_table('CopyMission')];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        # Seed inputs remain finite. Training wallet and medication have real
        # original star award/ordinaryshop sources, with no fake fight winner.
        for box in range(3):call('CSCopyGetStarAward',{'chapterId':'1','starIndex':box},'SCCopyGetStarAward')
        call('CSShopBuy',{'shopType':1,'shopItemId':'itemShopId_9','buyCount':1},'SCShopBuy')
        shop_items=self.profile(login)[1].get('items') or []
        self.assertGreaterEqual(next(x['num'] for x in shop_items if x['id']=='item_007'),4)
        old_simple=None
        for node in range(2,10):
            call('CSLightenWorldSeed',{'id':node},'SCLightenWorldSeed')
            if node==9:old_simple=stamp
        purple,purple_stamp=once({'id':10});self.assertEqual(purple.newWarrior.id,'hero_044');guid=purple.newWarrior.guid
        cold();self.assertEqual(self.call('CSLightenWorldSeed',{'id':10},login,purple_stamp,'SCLightenWorldSeed'),purple)
        before=self.profile(login);training=call('CSWarriorTraining',{'guid':guid,'costType':2,'numType':1},'SCWarriorTraining')
        self.assertEqual(before[0]['coin']-self.profile(login)[0]['coin'],3000)
        self.assertEqual([(x.id,x.num) for x in training.cost],[('item_007',4),('coin',3000)])
        reject_old(10,purple_stamp);reject_old(9,old_simple);cold();reject_old(10,purple_stamp)
        for node in range(11,25):call('CSLightenWorldSeed',{'id':node},'SCLightenWorldSeed')
        orange,orange_stamp=once({'id':25});self.assertEqual(orange.newWarrior.id,'hero_045');self.assertEqual(orange.newWarrior.guid,guid)
        reject_old(10,purple_stamp);cold();self.assertEqual(self.call('CSLightenWorldSeed',{'id':25},login,orange_stamp,'SCLightenWorldSeed'),orange);reject_old(9,old_simple)
        p,g=self.profile(login);self.assertEqual(p['currentWorldSeedId'],25);self.assertEqual(next(x['num'] for x in g['items'] if x['id']=='item_017'),1)
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_WORLDSEED_LIFECYCLE_PHONE_PENDING','actions':'24actualnodes2..25; actualpurple044/orange045sameGUID;8concurrent eachquality/currentcold; actualstarcoin/ordinaryshoptrainingmedication+3000cointraining; grown/advancedpreviousnodeoldframesrejectedfullprofileunchanged','coldRestarts':cold_count,'finitePrerequisites':'25item017/level100/guide8/bags1000/priororiginalmissionstar5;coin0diamond0/no fakefightwinner;seednatural84 remainsblocked','sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False,'baseline':self.baseline_source}
        (shared.ROOT/'analysis/p5/world-seed-receipt-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
