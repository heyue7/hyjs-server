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

OWNED = ['hyjs-server/service/p5Service.go','hyjs-server/service/p5FormationNextService.go','hyjs-server/service/p5HelperReceipt_test.go','tests/test_p5_helper_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_HELPER_RECEIPT_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class HelperReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_helper_http_' + secrets.token_hex(6)
        cls.test_user = 'p5helperhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-helper-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-helper-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_HELPER_RECEIPT_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
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
            report = shared.ROOT / 'analysis/p5/helper-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_helper_http_') or not cls.test_user.startswith('p5helperhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_empty_ack_helpers_replace_retire_then_old_packets_and_cold(self):
        began=time.monotonic();self.start();stamp=1800720000000;cold_count=0;uid='actual-helper-receipt';login=self.login(uid)
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
        def current(name,fields,old,reply,response):
            saved=self.profile(login);self.assertEqual(self.call(name,fields,login,old,response),reply);self.assertEqual(self.profile(login),saved)
        call('CSPlayerCreate',{'playerName':'原助阵回执','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p['level']=100;p['noviceGuideSchedule']=8;p['warriorBagSize']=1000;self.assertEqual((p.get('coin',0),p.get('diamond',0)),(0,0));g['items']=[{'id':'item_067','num':2}];self.write_profile(login,p,g)
        heroes=[call('CSItemUse',{'costItem':{'id':'item_067','num':1},'getItemIndex':choice},'SCItemUse').get.wars[0] for choice in [1,2]];self.assertEqual([x.id for x in heroes],['hero_011','hero_051'])
        a,b=[{'guid':x.guid,'pos':1} for x in heroes];off={'pos':1}
        first,first_stamp=once('CSHelperOn',a,'SCHelperOn');self.assertEqual(first.SerializeToString(),b'');cold();current('CSHelperOn',a,first_stamp,first,'SCHelperOn')
        second,second_stamp=once('CSHelperOn',b,'SCHelperOn');reject_old('CSHelperOn',a,first_stamp);cold();reject_old('CSHelperOn',a,first_stamp)
        removed,off_stamp=once('CSHelperOff',off,'SCHelperOff');self.assertEqual(removed.SerializeToString(),b'');reject_old('CSHelperOn',b,second_stamp)
        sale,sale_stamp=once('CSWarriorSell',{'guid':[heroes[1].guid]},'SCWarriorSell');earned=sum(x.num for x in sale.get.simps if x.id=='coin');self.assertGreater(earned,0)
        cold();reject_old('CSHelperOn',b,second_stamp);current('CSHelperOff',off,off_stamp,removed,'SCHelperOff')
        final,final_stamp=once('CSHelperOn',a,'SCHelperOn');reject_old('CSHelperOff',off,off_stamp);cold();reject_old('CSHelperOff',off,off_stamp);current('CSHelperOn',a,final_stamp,final,'SCHelperOn')
        formation=call('CSFormationList',{},'SCFormationList');self.assertEqual([(x.pos,x.guid,x.id) for x in formation.helperList],[(1,heroes[0].guid,heroes[0].id)])
        p,g=self.profile(login);self.assertNotIn(heroes[1].guid,[x['guid'] for x in g.get('warriors',[])]);self.assertEqual((p.get('coin',0),p.get('diamond',0)),(earned,0))
        saved=self.profile(login);self.assertEqual(self.call('CSWarriorSell',{'guid':[heroes[1].guid]},login,sale_stamp,'SCWarriorSell'),sale);self.assertEqual(self.profile(login),saved)
        report={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_EMPTY_HELPER_ACK_CURRENT_EFFECT_GUARD_INNODB_PHONE_PENDING','actualHeroesSource':[x.id for x in heroes],'actualRetirementCoin':earned,'actions':'actual067choice1/2→helperA8samecurrentcold→helperB8same→oldAhotcoldreject→off8same→oldBhotreject→actualSellB→oldBcoldreject/currentemptyoffkeep→helperA→oldOffhotcoldreject/currentOncoldkeep→originalFormationListexactonlyA/simpleSellcurrentkeep','finitePrerequisites':'level100/guide8/bag1000/held067x2;coin0diamond0/noawardedheroGUIDinject;notnaturalgiftproduction/phone','originalResponseBody':'BothSCHelperOn/SCHelperOff empty;capturedGUID/poscallbackeffectschecked, notfakewholeWarriorbody','coldRestarts':cold_count,'seconds':round(time.monotonic()-began,3),'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source,'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p5/helper-receipt-http-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
