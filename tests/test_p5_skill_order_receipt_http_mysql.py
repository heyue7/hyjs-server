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

OWNED = ['hyjs-server/service/p5Service.go','hyjs-server/service/relationsService.go','hyjs-server/service/p5SkillOrderReceipt_test.go','tests/test_p5_skill_order_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_SKILL_ORDER_RECEIPT_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class SkillOrderReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_skill_http_' + secrets.token_hex(6)
        cls.test_user = 'p5skillhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-skill-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-skill-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_SKILL_ORDER_RECEIPT_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/formation-receipt-manifest.json','analysis/p5/helper-receipt-manifest.json']:
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
            report = shared.ROOT / 'analysis/p5/skill-order-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_skill_http_') or not cls.test_user.startswith('p5skillhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_skill_order_empty_ack_current_transport_sale_and_cold(self):
        began=time.monotonic();self.start();stamp=1800730000000;cold_count=0;uid='actual-skill-order-receipt';login=self.login(uid)
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
        def reject_old(fields,old):
            saved=self.profile(login);op,msg=self.stamped('CSSetSkillOrder',fields,login,old);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved);return msg.msg
        def current(fields,old,reply):
            saved=self.profile(login);self.assertEqual(self.call('CSSetSkillOrder',fields,login,old,'SCSetSkillOrder'),reply);self.assertEqual(self.profile(login),saved)
        call('CSPlayerCreate',{'playerName':'原排序回执','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p['level']=100;p['noviceGuideSchedule']=8;p['warriorBagSize']=1000;self.assertEqual((p.get('coin',0),p.get('diamond',0)),(0,0));g['items']=[{'id':'item_067','num':1}];self.write_profile(login,p,g)
        hero=call('CSItemUse',{'costItem':{'id':'item_067','num':1},'getItemIndex':1},'SCItemUse').get.wars[0];self.assertEqual(hero.id,'hero_011');row=next(x for x in shared.original_table('Hero') if x['id']==hero.id)
        a={'heroGuid':hero.guid,'skillOrder':row['SkillId1']+','+row['SkillId2']};b={'heroGuid':hero.guid,'skillOrder':row['SkillId2']+','+row['SkillId1']}
        first,first_stamp=once('CSSetSkillOrder',a,'SCSetSkillOrder');self.assertEqual(first.SerializeToString(),b'');cold();current(a,first_stamp,first)
        second,second_stamp=once('CSSetSkillOrder',b,'SCSetSkillOrder');reject_old(a,first_stamp);cold();reject_old(a,first_stamp);current(b,second_stamp,second)
        formation={'id':hero.id,'guid':hero.guid,'formatPos':2};call('CSFormationOn',formation,'SCFormationOn');call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter');fight=call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'1'},'SCCopyFightStart');projected=next(x for x in fight.battlePlayer.formation if x.guid==hero.guid);self.assertEqual(projected.skillOrder,b['skillOrder']+','+row['UltSkillId'])
        call('CSCopyLeave',{},'SCCopyLeave');call('CSFormationOff',formation,'SCFormationOff');current(b,second_stamp,second)
        sale,sale_stamp=once('CSWarriorSell',{'guid':[hero.guid]},'SCWarriorSell');earned=sum(x.num for x in sale.get.simps if x.id=='coin');self.assertGreater(earned,0);reject_old(b,second_stamp);cold();reject_old(b,second_stamp)
        p,g=self.profile(login);self.assertNotIn(hero.guid,[x['guid'] for x in g.get('warriors',[])]);self.assertEqual((p.get('coin',0),p.get('diamond',0)),(earned,0))
        saved=self.profile(login);self.assertEqual(self.call('CSWarriorSell',{'guid':[hero.guid]},login,sale_stamp,'SCWarriorSell'),sale);self.assertEqual(self.profile(login),saved)
        report={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_EMPTY_SKILL_ORDER_ACK_CURRENT_EFFECT_GUARD_INNODB_PHONE_PENDING','actualHeroSource':hero.id,'actualRetirementCoin':earned,'actualOrderBefore':a['skillOrder'],'actualOrderAfter':b['skillOrder'],'actualFrozenBattleSkillOrder':projected.skillOrder,'actions':'actual067→actualA8same/currentcold→actualB8same/oldAhotcoldreject/currentBkeep→actualformation→originalCopyStartfrozenorderB+ultimate→originalCopyLeave/noFightEnd/winner→actualformationoff/currentBkeep→actualSell→oldBhotcoldreject/profileunchanged/simpleSalekeep','finitePrerequisites':'level100/guide8/bag1000/held067x1;coin0diamond0/noawardedheroGUIDinject;notnaturalboxsupply/noFightEnd/enginewinner/phone','originalResponseBody':'SCSetSkillOrderempty;originalMsgCallbackusescapturedheroGUID/UIleft/right/thirdUpdateSkillIndex','coldRestarts':cold_count,'seconds':round(time.monotonic()-began,3),'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source,'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p5/skill-order-receipt-http-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
