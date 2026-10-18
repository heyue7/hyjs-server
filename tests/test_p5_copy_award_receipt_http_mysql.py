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

OWNED = ['hyjs-server/service/p5CopyAwardReceiptService.go','hyjs-server/service/p5CopyAwardReceipt_test.go','hyjs-server/service/p5GrowthReceiptService.go','hyjs-server/service/copySweepService.go','tests/test_p5_copy_award_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_COPY_AWARD_RECEIPT_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class CopyAwardReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_copyaward_http_' + secrets.token_hex(6)
        cls.test_user = 'p5copyhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-copyaward-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-copyaward-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_COPY_AWARD_RECEIPT_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/empty-gem-sale-manifest.json','analysis/p8/gem-thirteen-manifest.json','analysis/p5/copy-award-shared-source-20261015/manifest.json']:
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
            report = shared.ROOT / 'analysis/p5/copy-award-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_copyaward_http_') or not cls.test_user.startswith('p5copyhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_copy_awards_actual_sell_food_and_cold(self):
        began=time.monotonic();self.start();stamp=1800430000000;cold_count=0;records=[]
        for scenario in ['firstdrop','sweep','pet']:
            uid='actual-copy-award-'+scenario;login=self.login(uid)
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
            def reject_old(name,fields,old):
                saved=self.profile(login);op,msg=self.stamped(name,fields,login,old);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved);return msg.msg
            call('CSPlayerCreate',{'playerName':'原副本回执'+scenario,'heroId':'hero_043'},'SCPlayerCreate')
            p,g=self.profile(login);p['warriorBagSize']=1000
            if scenario!='firstdrop':
                p['level']=34 if scenario=='pet' else 14
                g['copy']['passedCopyInfo']=[{'chapterId':'1','missionId':'1','star':5,'challengeTimes':0}]
            if scenario=='pet':g['items']=[{'id':'item_129','num':1}]
            self.write_profile(login,p,g)
            if scenario=='pet':
                pet=call('CSPetFirstRecruit',{},'SCPetFirstRecruit').get.pets[0]
                call('CSFormationOn',{'id':pet.id,'guid':pet.guid,'formatPos':1},'SCFormationOn')
            if scenario=='firstdrop':
                call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
                start=call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'1'},'SCCopyFightStart')
                fields={'fightId':start.fightId,'result':1,'battlePlayer':start.battlePlayer};name='CSCopyFightEnd';response='SCCopyFightEnd'
            else:fields={'chapterId':'1','missionId':'1'};name='CSCopySweep';response='SCCopySweep'
            award,old=once(name,fields,response);self.assertGreaterEqual(len(award.gain.wars),1)
            if scenario=='firstdrop':self.assertEqual((len(award.gain.wars),award.gain.wars[0].id),(1,'hero_exp_006'))
            if scenario=='pet':self.assertEqual((len(award.petAdds),award.petAdds[0].guid,award.petAdds[0].curExp),(1,pet.guid,2))
            cold();saved=self.profile(login);self.assertEqual(self.call(name,fields,login,old,response),award);self.assertEqual(self.profile(login),saved)
            if scenario=='pet':
                once('CSPetEat',{'guid':pet.guid,'cost':[{'id':'item_129','num':1}]},'SCPetEat')
                mutation='actualoriginalfood129 consumed/petintimacy2→22'
            else:
                w=award.gain.wars[0];once('CSWarriorSell',{'guid':[w.guid]},'SCWarriorSell');mutation='actualoriginalsource materialGUID sold/'+w.id
                self.assertFalse(any(x['guid']==w.guid for x in self.profile(login)[1]['warriors']))
            hot=reject_old(name,fields,old);cold();cold_error=reject_old(name,fields,old)
            records.append({'scenario':scenario,'awardedWarriors':len(award.gain.wars),'actualMutation':mutation,'hotRejected':hot,'coldRejected':cold_error,'eightInitialRetries':'exactsame original incrementalbody/single commit','currentColdReplay':'samewhilecurrent','coldRestarts':2})
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_COPY_AWARD_LIFECYCLE_PHONE_PENDING','actions':records,'qualification':'priorfiveStar/level14 or34/bag1000/food129one finite; first reportedwin uses isolated originalsettlement contract fixture, not originalengine/newauthority/naturalwinner claim; sweepstaminacost/materialaward/originalsell/food actual transactions','coldRestarts':cold_count,'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False,'baseline':self.baseline_source}
        (shared.ROOT/'analysis/p5/copy-award-receipt-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
