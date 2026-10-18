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

OWNED = ['tests/test_p5_battle_award_projection_http_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_BATTLE_AWARD_PROJECTION_HTTP_MYSQL_TEST') == '1', 'Explicit locked isolated P5 fourcrystals')
class BattleAwardProjectionHTTPMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_baward_http_' + secrets.token_hex(6)
        cls.test_user = 'p5bawhttp_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-baward-http-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-baward-http')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_BATTLE_AWARD_PROJECTION_HTTP_BASE', shared.ROOT / 'build/candidates/joint-gems-guard-v84-20261015/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p5/empty-gem-sale-manifest.json','analysis/p8/gem-thirteen-manifest.json','analysis/p5/copy-award-receipt-manifest.json','analysis/p5/copy-award-shared-source-20261015/manifest.json','analysis/p8/revive-receipt-manifest.json','analysis/p5/treasure-award-receipt-manifest.json','analysis/p5/battle-award-projection-manifest.json',os.environ.get('HYJS_P5_BATTLE_AWARD_SHARED_MANIFEST','analysis/p5/battle-award-shared-source-20261015/manifest.json')]:
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
            report = shared.ROOT / 'analysis/p5/battle-award-projection-http-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_baward_http_') or not cls.test_user.startswith('p5bawhttp_'):raise AssertionError('P5 fourcrystals isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 fourcrystals administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_side_daily_actual_sources_sold_old_end_and_cold(self):
        began=time.monotonic();self.start();stamp=1800540000000;cold_count=0;rows=[]
        for scenario in ['side','daily']:
            uid='actual-original-battle-award-'+scenario;login=self.login(uid)
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
            call('CSPlayerCreate',{'playerName':'原战斗奖励'+scenario,'heroId':'hero_043'},'SCPlayerCreate')
            p,g=self.profile(login);p.update(level=100 if scenario=='daily' else 18,coin=0,diamond=0,warriorBagSize=1000,equipBagSize=1000);self.write_profile(login,p,g)
            source_info={};stamina_before=p['stamina']
            if scenario=='side':
                call('CSTransferScene',{'target':'field_001'},'SCTransferScene');call('CSLoginScene',{},'SCLoginScene');call('CSCurrentPosition',{},'SCCurrentPosition')
                p,g=self.profile(login);monster=g['field']['monsters'][0];start=call('CSSidePveFightStart',{'monsterId':monster['uuid']},'SCSidePveFightStart');party=start.battlePlayer;hp=0
                for hero in party.formation:hero.inst.curHp=hero.prop.hp;hp+=hero.prop.hp
                fields={'fightId':start.fightId,'battlePlayer':party,'result':1,'optRecord':{'finalRoundNum':1,'attackerTotalLeftHp':hp,'defenderTotalLeftHp':0}};end_name='CSSidePveFightEnd';end_reply='SCSidePveFightEnd';source_info={'scene':'field_001','actualMonster':monster['monId'],'actualFrozenFight':start.fightId}
            else:
                state=call('CSDailyCopyState',{},'SCDailyCopyState');item=next(x for x in state.items if x.isOpen==1 and x.type in ['HeroExp','TreasureExp']);chapter=item.dailyCopyState[0].ChapterId
                start,_=once('CSDailyCopyFightStart',{'ChapterId':chapter},'SCDailyCopyFightStart');fields={'ChapterId':chapter,'fightId':start.fightId,'isWin':1,'optRecord':{'finalRoundNum':1,'attackerTotalLeftHp':1,'defenderTotalLeftHp':0}};end_name='CSDailyCopyFightEnd';end_reply='SCDailyCopyFightEnd';source_info={'originalOpenType':item.type,'chapter':chapter,'naturalWeekdaySelection':True,'actualFrozenFight':start.fightId}
            award,old=once(end_name,fields,end_reply);gain=award.getn if scenario=='side' else award.get;self.assertGreater(len(gain.wars)+len(gain.treas),0)
            current=self.profile(login);cold();self.assertEqual(self.call(end_name,fields,login,old,end_reply),award);self.assertEqual(self.profile(login),current)
            if gain.wars:
                material=gain.wars[0];sell_name='CSWarriorSell';sell_fields={'guid':[material.guid]};sell_reply='SCWarriorSell';kind='hero';resource=material.id
            else:
                material=gain.treas[0];sell_name='CSTreasureSell';sell_fields={'guids':[material.guid]};sell_reply='SCTreasureSell';kind='treasure';resource=material.treasureId
            sold,_=once(sell_name,sell_fields,sell_reply);hot=reject(end_name,fields,old);cold();cold_error=reject(end_name,fields,old)
            p,g=self.profile(login);inventory=g.get('warriors',[]) if kind=='hero' else g.get('treasures',[]);self.assertFalse(any(x['guid']==material.guid for x in inventory));actual_stamina=stamina_before-p['stamina'];self.assertEqual(actual_stamina,10 if scenario=='daily' else 0)
            rows.append({'scenario':scenario,**source_info,'sourceWarriors':len(gain.wars),'sourceTreasures':len(gain.treas),'actualStaminaCost':actual_stamina,'actualSoldResource':resource,'actualOriginalSaleCoin':sum(x.num for x in sold.get.simps if x.id=='coin'),'oldRetiredEndHotColdRejected':[hot,cold_error],'currentHeldColdReply':'same','eightStartIfDailyAndEndAndSale':'exactonce/currentbody','wholeProfileRejectedUnchanged':True})
        data={'date':'2026-10-15','status':'PASS_ORIGINAL_HTTP_INNODB_SIDE_DAILY_RETIRED_AWARD_END_LIFECYCLE_PHONE_PENDING','actualSources':rows,'coldRestarts':cold_count,'qualification':'onlyoriginallevels18/100/bagcapacity finite; allawardGUIDs are originalfield/day-selectedDaily committedreward producers; originalclientreportedwin settlementcontract fixtures not originalengine/naturalwinner authority claim. No coin/diamond/reward/resource/GUID/dropweight/day/quota inserts; daily original10stamina/one originaldaystart consumed','frozenGoSHA256':self.source_shas,'sourceSHA256':{rel:hashlib.sha256((shared.ROOT/rel).read_bytes()).hexdigest() for rel in OWNED},'sharedManifest':os.environ.get('HYJS_P5_BATTLE_AWARD_SHARED_MANIFEST','analysis/p5/battle-award-shared-source-20261015/manifest.json'),'baseline':self.baseline_source,'seconds':round(time.monotonic()-began,3),'productionModified':False,'phoneUI':False}
        (shared.ROOT/'analysis/p5/battle-award-projection-http-mysql-validation.json').write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n');type(self).passed=True

if __name__=='__main__':unittest.main()
