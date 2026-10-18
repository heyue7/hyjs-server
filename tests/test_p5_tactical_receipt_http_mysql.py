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

OWNED = ['hyjs-server/service/formationService.go','hyjs-server/service/p5FormationNextService.go','hyjs-server/service/p5Service.go','hyjs-server/constUtil/p5Next.go','hyjs-server/service/p5TacticalReceipt_test.go','tests/test_p5_tactical_receipt_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_TACTICAL_RECEIPT_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class TacticalReceiptHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_tactical_http_' + secrets.token_hex(6)
        cls.test_user = 'p5tacticalhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-tactical-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-tactical-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_TACTICAL_RECEIPT_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/treasure-award-receipt-manifest.json','analysis/p5/formation-receipt-manifest.json','analysis/p5/helper-receipt-manifest.json','analysis/p5/skill-order-receipt-manifest.json','analysis/p5/equipment-formation-receipt-manifest.json','analysis/p5/treasure-formation-receipt-manifest.json','analysis/p5/formation-exchange-receipt-manifest.json']:
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
            report = shared.ROOT / 'analysis/p5/tactical-receipt-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_tactical_http_') or not cls.test_user.startswith('p5tacticalhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_tactical_selection_mapping_upgrade_and_frozen_p6_cold(self):
        began=time.monotonic();self.start();stamp=1800770000000;cold_count=0;uid='actual-tactical-receipt';login=self.login(uid)
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
        def keep(name,fields,response,old,reply):
            saved=self.profile(login);self.assertEqual(self.call(name,fields,login,old,response),reply);self.assertEqual(self.profile(login),saved)
        def reject(name,fields,old):
            saved=self.profile(login);op,msg=self.stamped(name,fields,login,old);self.assertEqual(op,10001);self.assertEqual(self.profile(login),saved);return msg.msg
        call('CSPlayerCreate',{'playerName':'原战术回执','heroId':'hero_043'},'SCPlayerCreate');p,g=self.profile(login);main=p['heroWarId'];p.update(level=100,noviceGuideSchedule=8,warriorBagSize=1000);self.assertEqual((p.get('coin',0),p.get('diamond',0)),(0,0));g['items']=[{'id':'item_067','num':1},{'id':'item_372','num':5}];g['copy']['passedCopyInfo']=[{'chapterId':r['ChapterId'],'missionId':r['MissionId'],'star':5,'challengeTimes':0} for r in shared.original_table('CopyMission') if r['MissionId'] in ['1','2']];g['copy']['passedChapterInfo']=[];self.write_profile(login,p,g)
        sources=[call('CSQuestReward',{'questId':q},'SCQuestReward') for q in ['quest_1','quest_2','quest_3']];source_coin=sum(x.num for r in sources for x in r.get.simps if x.id=='coin');source_diamond=sum(x.num for r in sources for x in r.get.simps if x.id=='diamond');source_stone=sum(x.num for r in sources for x in r.get.simps if x.id=='item_373');self.assertEqual((source_coin,source_diamond,source_stone),(150000,40,11))
        hero=call('CSItemUse',{'costItem':{'id':'item_067','num':1},'getItemIndex':1},'SCItemUse').get.wars[0];self.assertEqual(hero.id,'hero_011');call('CSFormationOn',{'id':hero.id,'guid':hero.guid,'formatPos':2},'SCFormationOn');unlocked=call('CSTaticalFormationUnLock',{'tacticalId':'2'},'SCTaticalFormationUnLock');self.assertEqual([(x.id,x.num) for x in unlocked.cost],[('item_373',1)])
        use2={'id':'2'};use1={'id':'1'};first,first_stamp=once('CSUseTaticalFormation',use2,'SCUseTaticalFormation');cold();keep('CSUseTaticalFormation',use2,'SCUseTaticalFormation',first_stamp,first)
        call('CSUseTaticalFormation',use1,'SCUseTaticalFormation');reject('CSUseTaticalFormation',use2,first_stamp);cold();reject('CSUseTaticalFormation',use2,first_stamp)
        selected,selected_stamp=once('CSUseTaticalFormation',use2,'SCUseTaticalFormation');change={'id':'2','formation':'1,13;2,8'};mapped,mapped_stamp=once('CSTaticalFormationChange',change,'SCTaticalFormationChange');reject('CSUseTaticalFormation',use2,selected_stamp);cold();keep('CSTaticalFormationChange',change,'SCTaticalFormationChange',mapped_stamp,mapped)
        changed={'id':'2','formation':'1,7;2,9'};current_map,current_map_stamp=once('CSTaticalFormationChange',changed,'SCTaticalFormationChange');reject('CSTaticalFormationChange',change,mapped_stamp);cold();reject('CSTaticalFormationChange',change,mapped_stamp)
        current,current_stamp=once('CSUseTaticalFormation',use2,'SCUseTaticalFormation');up=call('CSTaticalFormationLevelUp',{'tacticalId':'2'},'SCTaticalFormationLevelUp');self.assertEqual(up.currLevel,2);self.assertEqual([(x.id,x.num) for x in up.cost],[('item_373',5),('item_372',5)]);reject('CSUseTaticalFormation',use2,current_stamp);reject('CSTaticalFormationChange',changed,current_map_stamp);cold();reject('CSUseTaticalFormation',use2,current_stamp)
        final,final_stamp=once('CSUseTaticalFormation',use2,'SCUseTaticalFormation');cold();keep('CSUseTaticalFormation',use2,'SCUseTaticalFormation',final_stamp,final);formation=call('CSFormationList',{},'SCFormationList');self.assertEqual(formation.tacticalSkillId,'7');self.assertEqual({x.formatPos:x.battlePos for x in formation.formationList},{1:7,2:9})
        call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter');battle=call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'1'},'SCCopyFightStart');poses={x.guid:x.pose for x in battle.battlePlayer.formation};self.assertEqual(poses,{main:7,hero.guid:9});call('CSUseTaticalFormation',use1,'SCUseTaticalFormation');reject('CSUseTaticalFormation',use2,final_stamp);reject('CSTaticalFormationChange',changed,current_map_stamp);cold();again=call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'1'},'SCCopyFightStart');self.assertEqual(again,battle);call('CSCopyLeave',{},'SCCopyLeave')
        p,g=self.profile(login);items={x['id']:x.get('num',0) for x in g['items']};self.assertEqual((p['coin'],p['diamond'],items.get('item_373',0),items.get('item_372',0)),(150000,40,5,0));self.assertEqual(g['tactical_skill_id'],'1');self.assertEqual(call('CSGetTaticalFormation',{'id':'2'},'SCGetTaticalFormation').formation.level,2)
        report={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_TACTICAL_SELECTION_MAPPING_SKILL_RECEIPT_AND_FROZEN_P6_COLD_INNODB_PHONE_PENDING','actions':'actualquest1/2/3→11original373+150000coin+40diamond;actual067hero011→originalunlock2cost373x1→use2eightonce/currentcold→use1/oldUse2rejecthotcold→use2/newmap13,8eightonce/oldUse2reject→newmap7,9/oldMaprejecthotcold→actualupgrade2cost373x5/372x5→oldUseandMapreject→newUse2cold/skill7/actualP6frozenposes7,9→actualUse1/oldUseandMapreject/cold→originalfightstartretainsfullfrozenBattle→Leavewithoutwinner','coldRestarts':cold_count,'actualOriginalQuestStone':source_stone,'actualOriginalQuestCoin':source_coin,'actualOriginalQuestDiamond':source_diamond,'actualUnlockCost':'item_373,1','actualUpgradeCost':'item_373,5;item_372,5','finalCoin':p['coin'],'finalDiamond':p['diamond'],'finalStone':items.get('item_373',0),'finalCrystal':items.get('item_372',0),'finitePrerequisites':'level100/guide8/bag1000/held067one/held372five/priorchapter1mission1and2star5;noGUID/War/Pet/formation/373/coin/diamond/skill/positions/fightwininjected;372naturalExpeditionStorysourceconditionalnotclaimed','seconds':round(time.monotonic()-began,3),'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'frozenGoSHA256':self.source_shas,'baseline':self.baseline_source,'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p5/tactical-receipt-http-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
