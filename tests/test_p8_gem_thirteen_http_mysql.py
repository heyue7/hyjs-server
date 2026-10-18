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

OWNED = ['hyjs-server/service/gemService.go','hyjs-server/service/p8GemThirteen_test.go','tests/test_p8_gem_thirteen_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P8_GEM_THIRTEEN_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class GemThirteenHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p8_gem13_http_' + secrets.token_hex(6)
        cls.test_user = 'p8gem13http_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p8-gem13-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-gem13-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P8_GEM_THIRTEEN_HTTP_BASE', shared.ROOT / 'build/candidates/joint-preview-caches-v78-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/award-collection-manifest.json','analysis/p5/refund-material-manifest.json','analysis/p5/world-seed-receipt-manifest.json','analysis/p8/gem-receipt-manifest.json','analysis/p5/empty-gem-sale-manifest.json']:
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
            report = shared.ROOT / 'analysis/p8/gem-thirteen-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p8_gem13_http_') or not cls.test_user.startswith('p8gem13http_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_six_combine_rainbow_thirteen_and_cold(self):
        began=time.monotonic();self.start();uid='actual-original-gem-thirteen';login=self.login(uid);stamp=1800230000000;cold_count=0
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        def once(name,fields,response):
            nonlocal stamp
            stamp+=1;saved_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call(name,fields,login,saved_stamp,response),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));return replies[0],saved_stamp
        def reject(name,fields,old=None):
            nonlocal stamp
            if old is None:stamp+=1;old=stamp
            saved=self.profile(login);opcode,_=self.stamped(name,fields,login,old);self.assertEqual(opcode,10001);self.assertEqual(self.profile(login),saved)
        def cold():
            nonlocal login,cold_count
            saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);cold_count+=1
        call('CSPlayerCreate',{'playerName':'原十三级七彩合成','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,noviceGuideSchedule=8,equipBagSize=1000,warriorBagSize=1000)
        g['gems']=[{'id':'MineItem_079','num':36}];g['items']=[{'id':'item_119','num':1}];self.write_profile(login,p,g)
        self.sql(f'INSERT INTO gem_craft_states(player_id,level,exp) VALUES({login.playerId},10,0);')
        high={'gemId':'MineItem_106'};middle={'gemId':'MineItem_080'}
        reject('CSGemCombineBySix',high)
        for i in range(6):
            result=call('CSGemCombineBySix',middle,'SCGemCombineBySix');self.assertEqual([(x.id,x.num) for x in result.cost],[('MineItem_079',6)]);self.assertEqual([(x.id,x.num) for x in result.res.simps],[('MineItem_080',1)])
        self.sql(f'UPDATE gem_craft_states SET level=9 WHERE player_id={login.playerId};');reject('CSGemCombineBySix',high);self.sql(f'UPDATE gem_craft_states SET level=10 WHERE player_id={login.playerId};')
        final,final_stamp=once('CSGemCombineBySix',high,'SCGemCombineBySix');self.assertEqual([(x.id,x.num) for x in final.cost],[('MineItem_080',6)]);self.assertEqual([(x.id,x.num) for x in final.res.simps],[('MineItem_106',1)])
        counts={x['id']:x['num'] for x in self.profile(login)[1]['gems']};self.assertEqual(counts.get('MineItem_079',0),0);self.assertEqual(counts.get('MineItem_080',0),0);self.assertEqual(counts['MineItem_106'],1)
        cold();self.assertEqual(self.call('CSGemCombineBySix',high,login,final_stamp,'SCGemCombineBySix'),final)
        reject('CSGemMake',{'gemId':'MineItem_106','num':1});reject('CSGemCombineBySix',high)
        gear=call('CSItemUse',{'costItem':{'id':'item_119','num':1},'getItemIndex':1},'SCItemUse').get.equips[0]
        call('CSFormationOn',{'formatPos':1,'id':gear.equipId,'guid':gear.guid},'SCFormationOn');call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
        before=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer.formation[0].prop
        mount={'guid':gear.guid,'gemId':'MineItem_106','pos':0};mounted,mount_stamp=once('CSGemMount',mount,'SCGemMount');self.assertEqual(mounted.equip.gems[0],'MineItem_106')
        after=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer.formation[0].prop
        expected={'att':3600,'hp':13680,'pdef':1080,'mdef':1080,'cri':5.4,'antiCri':5.4};deltas={k:getattr(after,k)-getattr(before,k) for k in expected}
        for k,v in expected.items():self.assertAlmostEqual(deltas[k],v,places=7)
        cold();self.assertEqual(self.call('CSGemMount',mount,login,mount_stamp,'SCGemMount'),mounted)
        unmounted,_=once('CSGemUnmount',{'guid':gear.guid,'pos':0},'SCGemUnmount');self.assertEqual([(x.id,x.num) for x in unmounted.res.simps],[('MineItem_106',1)])
        restored=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer.formation[0].prop;self.assertEqual(restored,before)
        reject('CSGemMount',mount,mount_stamp);cold();reject('CSGemMount',mount,mount_stamp)
        saved=self.profile(login);self.assertEqual(self.call('CSGemCombineBySix',high,login,final_stamp,'SCGemCombineBySix'),final);self.assertEqual(self.profile(login),saved)
        p,g=self.profile(login);self.assertEqual(p['coin'],0);self.assertEqual(p['diamond'],0);self.assertEqual(next(x['num'] for x in g['gems'] if x['id']=='MineItem_106'),1)
        call('CSCopyLeave',{},'SCCopyLeave')
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_GEM_THIRTEEN_PHONE_PENDING','actualOperations':'sixoriginal6×079→six080;8same6×080→one106;insufficientsource/skill9/no13manufacturefail;actualbox119→037GUID/formation/mountoriginal13/sixBattleprops→unmountexactrestore;retiredmountbodyreject/simplehighcombinereceiptcoldpreserved','battlePropertyDeltas':deltas,'grade7Input':36,'grade10Made':6,'grade13Final':1,'coinSpent':0,'diamondSpent':0,'coldRestarts':cold_count,'finitePrerequisites':'36heldoriginal079/level100/guide8/craft10historicalqualification/held119;nofinal106orEquipGUIDinjected;originalskill9qualificationtemporarilytestedthenrestore10;no natural36book/XP/highmap/winner/phoneclaim','sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False,'baseline':self.baseline_source}
        (shared.ROOT/'analysis/p8/gem-thirteen-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
