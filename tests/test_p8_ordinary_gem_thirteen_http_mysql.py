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

OWNED = ['hyjs-server/service/p8OrdinaryGemThirteen_test.go','tests/test_p8_ordinary_gem_thirteen_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P8_ORDINARY_GEM_THIRTEEN_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class OrdinaryGemThirteenHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p8_ordgem13_http_' + secrets.token_hex(6)
        cls.test_user = 'p8og13http_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p8-ordgem13-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-ordgem13-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P8_ORDINARY_GEM_THIRTEEN_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/empty-gem-sale-manifest.json','analysis/p8/gem-thirteen-manifest.json']:
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
            report = shared.ROOT / 'analysis/p8/ordinary-gem-thirteen-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p8_ordgem13_http_') or not cls.test_user.startswith('p8og13http_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_all_six_ordinary_thirteen_and_cold(self):
        began=time.monotonic();self.start();stamp=1800240000000;results=[]
        rows=shared.original_table('MineSkill');equips=shared.original_table('Equip');box_choices=[x for x in shared.original_table('ItemUse') if x['itemId']=='item_119'];propnames=['att','hp','pdef','mdef','cri','antiCri'];expected=[7000,26600,2100,2100,7.8,7.8]
        for colour in range(1,7):
            uid='original-ordinary-gem13-'+str(colour);login=self.login(uid);cold_count=0
            def call(name,fields,response):
                nonlocal stamp
                stamp+=1;return self.call(name,fields,login,stamp,response)
            def once(name,fields,response):
                nonlocal stamp
                stamp+=1;saved_stamp=stamp
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call(name,fields,login,saved_stamp,response),range(8)))
                self.assertTrue(all(x==replies[0] for x in replies));return replies[0],saved_stamp
            def cold():
                nonlocal login,cold_count
                saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved);cold_count+=1
            call('CSPlayerCreate',{'playerName':'六色原十三阶'+str(colour),'heroId':'hero_043'},'SCPlayerCreate')
            p,g=self.profile(login);p.update(level=100,coin=0,diamond=0,noviceGuideSchedule=8,equipBagSize=1000,warriorBagSize=1000);g['items']=[{'id':'item_345','num':27},{'id':'item_119','num':1}];self.write_profile(login,p,g)
            self.sql(f'INSERT INTO gem_craft_states(player_id,level,exp) VALUES({login.playerId},10,0);')
            open_fields={'costItem':{'id':'item_345','num':27},'getItemIndex':colour};opened,_=once('CSItemUse',open_fields,'SCItemUse');self.assertEqual(sum(x.num for x in opened.get.simps),27)
            def gem(level):return next(x['MineItemID'] for x in rows if x['Type']=='magicGem' and int(x['Color'])==colour and int(x['Level'])==level)
            source=gem(10);target=gem(13);self.assertTrue(all(x.id==source for x in opened.get.simps))
            operations=0;last=None;last_stamp=None
            for level,count in [(11,9),(12,3),(13,1)]:
                for i in range(count):
                    fields={'gemId':gem(level)}
                    if level==13:last,last_stamp=once('CSGemCombineByThree',fields,'SCGemCombineByThree')
                    else:last=call('CSGemCombineByThree',fields,'SCGemCombineByThree');last_stamp=stamp
                    operations+=1;self.assertEqual([(x.id,x.num) for x in last.cost],[(gem(level-1),3)]);self.assertEqual([(x.id,x.num) for x in last.res.simps],[(gem(level),1)])
            self.assertEqual(operations,13)
            counts={x['id']:x['num'] for x in self.profile(login)[1]['gems']};self.assertEqual(counts.get(source,0),0);self.assertEqual(counts.get(gem(11),0),0);self.assertEqual(counts.get(gem(12),0),0);self.assertEqual(counts[target],1)
            choice=None;hole=None
            for candidate in box_choices:
                key=candidate['result'].split(',')[0];row=next(x for x in equips if x['id']==key)
                matches=[pos for pos in range(4) if int(row['hole'+str(pos+1)])==colour]
                if matches:choice=int(candidate['odds']);hole=matches[0];break
            self.assertIsNotNone(choice)
            gear=call('CSItemUse',{'costItem':{'id':'item_119','num':1},'getItemIndex':choice},'SCItemUse').get.equips[0]
            call('CSFormationOn',{'formatPos':1,'id':gear.equipId,'guid':gear.guid},'SCFormationOn');call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
            before=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer.formation[0].prop
            mount={'guid':gear.guid,'gemId':target,'pos':hole};mounted,mount_stamp=once('CSGemMount',mount,'SCGemMount')
            after=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer.formation[0].prop;deltas={name:getattr(after,name)-getattr(before,name) for name in propnames}
            for index,name in enumerate(propnames):self.assertAlmostEqual(deltas[name],expected[index] if index+1==colour else 0,places=7)
            cold();self.assertEqual(self.call('CSGemMount',mount,login,mount_stamp,'SCGemMount'),mounted)
            unmounted,_=once('CSGemUnmount',{'guid':gear.guid,'pos':hole},'SCGemUnmount');self.assertEqual([(x.id,x.num) for x in unmounted.res.simps],[(target,1)])
            restored=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer.formation[0].prop;self.assertEqual(restored,before);cold()
            saved=self.profile(login);self.assertEqual(self.stamped('CSGemMount',mount,login,mount_stamp)[0],10001);self.assertEqual(self.profile(login),saved)
            saved=self.profile(login);self.assertEqual(self.call('CSGemCombineByThree',{'gemId':target},login,last_stamp,'SCGemCombineByThree'),last);self.assertEqual(self.profile(login),saved)
            p,g=self.profile(login);self.assertEqual(p['coin'],0);self.assertEqual(p['diamond'],0);self.assertEqual(next(x['num'] for x in g['gems'] if x['id']==target),1)
            call('CSCopyLeave',{},'SCCopyLeave')
            results.append({'colour':colour,'target':target,'sourceOriginalBoxChoices':27,'sourceGrade10':source,'actualThreeCombineCount':operations,'equipment':gear.equipId,'originalMatchedHole':hole,'battlePropertyDeltas':deltas,'coldRestarts':cold_count})
            print({'colourComplete':colour,'target':target,'actualCombines':operations,'cold':cold_count},flush=True)
        report={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_ALL_SIX_ORDINARY_GEM_THIRTEEN_PHONE_PENDING','results':results,'finitePrerequisites':'sixindependent27held345/119box/craft10historical/lv100/guide8 qualifications;actualoriginalboxes create162grade10gems,no highestgem orGUIDinjected;no natural27boxes/sourcecoin/craftXP/multimonthclaim','sourceBoundary':'existingnamedGemSupply34510mcoin/day1/month3 implies27boxesatleast9month/270m percolourifoperatorsunchanged;finiteproofnotnaturalsupply. No addedsource/newcommodity/price/quota','sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False,'baseline':self.baseline_source,'P6Boundary':'originalBattleHerotransportprops measured,no Fightwinner/end/PVP authority claim'}
        (shared.ROOT/'analysis/p8/ordinary-gem-thirteen-http-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
