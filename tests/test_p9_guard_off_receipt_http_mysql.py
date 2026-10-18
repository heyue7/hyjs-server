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

OWNED = ['hyjs-server/service/p9PetService.go','hyjs-server/service/p9GuardOffReceipt_test.go','tests/test_p9_guard_off_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P9_GUARD_OFF_RECEIPT_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class GuardOffReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p9_off_http_' + secrets.token_hex(6)
        cls.test_user = 'p9offhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p9-off-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-p9off-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P9_GUARD_OFF_RECEIPT_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in []:
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
            report = shared.ROOT / 'analysis/p9/guard-off-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p9_off_http_') or not cls.test_user.startswith('p9offhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_guard_off_equal_power_different_current_pet_and_cold(self):
        began=time.monotonic();self.start();stamp=1800740000000;cold_count=0;uid='actual-pet-off-receipt';login=self.login(uid)
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
        def current(fields,old,reply):
            saved=self.profile(login);self.assertEqual(self.call('CSFormationOff',fields,login,old,'SCFormationOff'),reply);self.assertEqual(self.profile(login),saved)
        def reject(fields,old):
            saved=self.profile(login);op,msg=self.stamped('CSFormationOff',fields,login,old);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved);return msg.msg
        call('CSPlayerCreate',{'playerName':'原精灵卸下','heroId':'hero_043'},'SCPlayerCreate');p,g=self.profile(login);p['level']=34;p['noviceGuideSchedule']=8;p['warriorBagSize']=1000;self.assertEqual((p.get('coin',0),p.get('diamond',0)),(0,0));g['items']=[{'id':'item_067','num':1},{'id':'frag_pet_002','num':50}];self.write_profile(login,p,g)
        hero=call('CSItemUse',{'costItem':{'id':'item_067','num':1},'getItemIndex':1},'SCItemUse').get.wars[0];self.assertEqual(hero.id,'hero_011');call('CSFormationOn',{'id':hero.id,'guid':hero.guid,'formatPos':2},'SCFormationOn')
        a=once('CSPetFirstRecruit',{},'SCPetFirstRecruit')[0].get.pets[0];combined=once('CSCombine',{'targetId':'pet_002'},'SCCombine')[0];b=combined.get.pets[0];self.assertEqual((a.id,b.id),('pet_002','pet_002'));self.assertNotEqual(a.guid,b.guid);self.assertEqual([(x.id,x.num) for x in combined.cost],[('frag_pet_002',50)])
        place=lambda pet,pos:{'id':pet.id,'guid':pet.guid,'formatPos':pos}
        once('CSFormationOn',place(a,1),'SCFormationOn');once('CSFormationOn',place(b,2),'SCFormationOn');old,old_stamp=once('CSFormationOff',place(a,1),'SCFormationOff');cold();current(place(a,1),old_stamp,old)
        once('CSFormationOff',place(b,2),'SCFormationOff');once('CSFormationOn',place(b,1),'SCFormationOn');p,g=self.profile(login);self.assertEqual(p['fightPower'],old.fightpower)
        rejected=reject(place(a,1),old_stamp);cold();rejected_cold=reject(place(a,1),old_stamp)
        formations=call('CSFormationList',{},'SCFormationList');guards=[(slot.formatPos,x.guid,x.id) for slot in formations.formationList for x in slot.things if x.pos==7];self.assertEqual(guards,[(1,b.guid,b.id)])
        new,new_stamp=once('CSFormationOff',place(b,1),'SCFormationOff');cold();current(place(b,1),new_stamp,new);p,g=self.profile(login);self.assertEqual((p.get('coin',0),p.get('diamond',0)),(0,0));self.assertEqual(sum(x.get('num',0) for x in g.get('items',[]) if x['id']=='frag_pet_002'),0)
        report={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_GUARD_OFF_EQUAL_POWER_CURRENT_SLOT_GUARD_INNODB_PHONE_PENDING','actualPetSources':['originalCSPetFirstRecruit pet_002','originalCSCombine exact50frag_pet_002→newpet_002GUID'],'originalSourceA':a.guid,'originalSourceB':b.guid,'oldOffFightPower':old.fightpower,'actualEqualPowerAfterMove':old.fightpower,'actions':'actual067hero011camp2/actualfirstA+50fragCombineB→OnA1/OnB2→OffA1old/currentcold→OffB2/OnB1 exactsameaggregatepower/Aheldnotguarded→oldOffAhotcoldreject/fullprofileunchanged→originalFormationListB1only→actualOffB1new/currentcoldkeep;8actualframesonce','coldRestarts':cold_count,'rejected':rejected,'rejectedCold':rejected_cold,'finitePrerequisites':'level34/guide8/bag1000/held067x1and50originalfrag002;coin0diamond0;noPet/hero/GUID/rawprop/fightpowerinjected;notnaturalfragmentproduction/enginewinner/phone','seconds':round(time.monotonic()-began,3),'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source,'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p9/guard-off-receipt-http-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
