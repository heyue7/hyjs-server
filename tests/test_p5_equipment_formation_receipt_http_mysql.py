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

OWNED = ['hyjs-server/service/p5Service.go','hyjs-server/service/p5FormationNextService.go','hyjs-server/service/equipmentService.go','hyjs-server/constUtil/p5Next.go','hyjs-server/service/p5EquipmentFormationReceipt_test.go','tests/test_p5_equipment_formation_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_EQUIPMENT_FORMATION_RECEIPT_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class EquipmentFormationReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_wear_http_' + secrets.token_hex(6)
        cls.test_user = 'p5wearhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-wear-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-wear-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_EQUIPMENT_FORMATION_RECEIPT_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/formation-receipt-manifest.json','analysis/p5/helper-receipt-manifest.json','analysis/p5/skill-order-receipt-manifest.json']:
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
            report = shared.ROOT / 'analysis/p5/equipment-formation-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_wear_http_') or not cls.test_user.startswith('p5wearhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_equipment_old_frames_cannot_rewear_or_clear_swapped_slot(self):
        began=time.monotonic();self.start();stamp=1800750000000;cold_count=0;uid='actual-equipment-wear-receipt';login=self.login(uid)
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
        def current(name,fields,old,reply,response):
            saved=self.profile(login);self.assertEqual(self.call(name,fields,login,old,response),reply);self.assertEqual(self.profile(login),saved)
        def reject(name,fields,old):
            saved=self.profile(login);op,msg=self.stamped(name,fields,login,old);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved);return msg.msg
        call('CSPlayerCreate',{'playerName':'原装备穿脱','heroId':'hero_043'},'SCPlayerCreate');p,g=self.profile(login);p['level']=100;p['noviceGuideSchedule']=8;p['equipBagSize']=1000;self.assertEqual((p.get('coin',0),p.get('diamond',0)),(0,0));g['items']=[{'id':'item_119','num':2}];self.write_profile(login,p,g)
        a,b=[call('CSItemUse',{'costItem':{'id':'item_119','num':1},'getItemIndex':1},'SCItemUse').get.equips[0] for _ in range(2)];self.assertEqual((a.equipId,b.equipId),('equip_037','equip_037'));self.assertNotEqual(a.guid,b.guid);place=lambda x:{'id':x.equipId,'guid':x.guid,'formatPos':1}
        first,first_stamp=once('CSFormationOn',place(a),'SCFormationOn');cold();current('CSFormationOn',place(a),first_stamp,first,'SCFormationOn')
        second,second_stamp=once('CSFormationOn',place(b),'SCFormationOn');reject('CSFormationOn',place(a),first_stamp);cold();reject('CSFormationOn',place(a),first_stamp);current('CSFormationOn',place(b),second_stamp,second,'SCFormationOn')
        removed,removed_stamp=once('CSFormationOff',place(b),'SCFormationOff');cold();current('CSFormationOff',place(b),removed_stamp,removed,'SCFormationOff')
        last,last_stamp=once('CSFormationOn',place(a),'SCFormationOn');reject('CSFormationOff',place(b),removed_stamp)
        last_off,last_off_stamp=once('CSFormationOff',place(a),'SCFormationOff');cold();current('CSFormationOff',place(a),last_off_stamp,last_off,'SCFormationOff')
        sold,sold_stamp=once('CSEquipmentSell',{'cost':[{'id':a.equipId,'guid':a.guid,'num':1}]},'SCEquipmentSell');self.assertEqual(sum(x.num for x in sold.get.simps if x.id=='coin'),1000);reject('CSFormationOff',place(a),last_off_stamp);reject('CSFormationOn',place(a),last_stamp);cold();reject('CSFormationOff',place(a),last_off_stamp)
        p,g=self.profile(login);self.assertEqual((p.get('coin',0),p.get('diamond',0)),(1000,0));self.assertNotIn(a.guid,[x['guid'] for x in g.get('equipment',[])]);formation=call('CSFormationList',{},'SCFormationList');self.assertFalse(any(x.pos==1 for slot in formation.formationList for x in slot.things))
        saved=self.profile(login);self.assertEqual(self.call('CSEquipmentSell',{'cost':[{'id':a.equipId,'guid':a.guid,'num':1}]},login,sold_stamp,'SCEquipmentSell'),sold);self.assertEqual(self.profile(login),saved)
        report={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_STAMPED_EQUIPMENT_WEAR_OFF_CURRENT_EFFECT_GUARD_INNODB_PHONE_PENDING','actualEquipmentSources':['original119choice1→equip037A','original119choice1→equip037B'],'actions':'actual119two→OnA8same/currentcold→OnB8same/oldAhotcoldreject/currentBkeep→OffB8same/currentcold→OnAnew/oldOffBreject→OffAnew/currentcold→actualSellA1000→oldOn/Offhotcoldreject/profileunchanged/finalslotempty/Bheld/simpleSaleprotected','coldRestarts':cold_count,'finitePrerequisites':'level100/guide8/bag1000/held119x2;coin0diamond0/noequipmentGUID/propinjection/notnatural119supply/phone/enginewinner','originalSemantics':'On originalEquipSimplePage/Remodel callbacks usecapturedGUID/pos/type UpdateEquip;Off ArrayPage callback lookupcapturedoldGUID type thenclearsrequestedtype slot. ResponsesretainoriginalSCFormationOn/Off andcurrentSCPlayer P5existingstyle','sameFrameTransaction':'originalframe stamp durable P5receipt; sameapplyusedwithdirectdomainwrapper preservesalloriginalcost/level/type/ownership/slotrules; noreexecutionoldcommittedframe','seconds':round(time.monotonic()-began,3),'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source,'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p5/equipment-formation-receipt-http-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
