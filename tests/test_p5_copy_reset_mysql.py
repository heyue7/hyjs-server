"""Original five gem-box economy binary frames using locked disposable InnoDB."""
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

OWNED = ['hyjs-server/service/copyResetService.go', 'hyjs-server/service/copyReset_test.go', 'hyjs-server/service/p5Service.go', 'hyjs-server/constUtil/copyReset.go', 'hyjs-server/db/copyResetRule.go', 'hyjs-server/db/copyResetDaily.go', 'hyjs-server/db/sql/109_copy_reset_rules.sql', 'hyjs-server/db/sql/109_copy_reset_rules.seed.sql', 'hyjs-server/db/sql/109_copy_reset_daily.sql', 'analysis/p5/copy-reset-original-audit.json', 'analysis/p5/copy-reset-original.il.txt', 'analysis/p5/copy-reset-data-original.il.txt', 'analysis/p5/copy-reset-event-original.il.txt', 'tests/test_p5_copy_reset_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_COPY_RESET_MYSQL_TEST') == '1', 'Explicit locked isolated P5 gem supply')
class CopyResetMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_copy_reset_' + secrets.token_hex(6)
        cls.test_user = 'p5reset_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-copy-reset-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-gem-supply')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_COPY_RESET_BASE', shared.ROOT / 'build/candidates/joint-natural-guild-v59-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
        for manifest in ['analysis/p4/resource-exchange-manifest.json','analysis/p5/gold-egg-manifest.json']:
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
        # Exact root-owned hook: suppress only when the open aggregate covers P2.
        target=source/'service/turntableService.go';old=target.read_text()
        needle='\tif redExchange != nil {\n\t\tresult.ActivityList = append(result.ActivityList, redExchange)\n\t}'
        replacement='\tif redExchange != nil {\n\t\tcovered,e:=s.P4ResourceExchangeCoversRed(ctx,id,time.Now())\n\t\tif e!=nil{return nil,e}\n\t\tif !covered{result.ActivityList=append(result.ActivityList,redExchange)}\n\t}'
        if needle not in old:raise AssertionError('Unexpected root hook baseline')
        target.write_text(old.replace(needle,replacement))
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
            if left.split() != ['0','0','0']: raise AssertionError('P5 gem supply schema/user/grants remain')
            report = shared.ROOT / 'analysis/p5/copy-reset-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_copy_reset_') or not cls.test_user.startswith('p5reset_'):raise AssertionError('P5 gem supply isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 gem supply administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_vip0_vip12_quote_source_diamond_reset_continue_and_cold(self):
        began=time.monotonic();self.start();stamp=1800140000000;results=[];missions=shared.original_table('CopyMission')
        for vip in [0,12]:
            uid='original-copy-reset-vip-'+str(vip);login=self.login(uid)
            def call(name,fields,response):
                nonlocal stamp
                stamp+=1;return self.call(name,fields,login,stamp,response)
            call('CSPlayerCreate',{'playerName':'原关卡重置'+str(vip),'heroId':'hero_043'},'SCPlayerCreate')
            p,g=self.profile(login);p.update(level=100,diamond=0,vip=vip,stamina=1000,equipBagSize=1000,warriorBagSize=1000,noviceGuideSchedule=8)
            g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions];g['copy']['passedChapterInfo']=[];g['run']=None;self.write_profile(login,p,g)
            natural_diamond=0
            for chapter in ['27','26']:
                for box in range(3):
                    gain=call('CSCopyGetStarAward',{'chapterId':chapter,'starIndex':box},'SCCopyGetStarAward').gain;natural_diamond+=sum(x.num for x in gain.simps if x.id=='diamond')
            self.assertEqual(self.profile(login)[0]['diamond'],natural_diamond)
            prices=[20] if vip==0 else [20,30,50,100];actual_paid=0;last_reset=None;last_stamp=None;last_request=None
            for i,price in enumerate(prices):
                target=['5','7','9','11'][i];chapter=next(x['ChapterId'] for x in missions if x['MissionId']==target);maximum=int(next(x['ChallengeCount'] for x in missions if x['MissionId']==target))
                # Original server sweep creates the exhausted condition; no challengeCount prepared.
                for _ in range(maximum):call('CSCopySweep',{'chapterId':chapter,'missionId':target},'SCCopySweep')
                query={'chapterId':chapter,'missionId':target};quote=call('CSCopyGetResetInfo',query,'SCCopyGetResetInfo')
                self.assertEqual((quote.remainResetCount,quote.cost[0].id,quote.cost[0].num),(len(prices)-i,'diamond',price))
                req=dict(query,cost=quote.cost[0]);before=self.profile(login);stamp+=1
                self.assertEqual(self.stamped('CSCopyResetChallengeCount',dict(query,cost={'id':'diamond','num':1}),login,stamp)[0],10001);self.assertEqual(self.profile(login),before)
                stamp+=1;paid_stamp=stamp
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:reset=list(pool.map(lambda _:self.call('CSCopyResetChallengeCount',req,login,paid_stamp,'SCCopyResetChallengeCount'),range(8)))
                self.assertTrue(all(x==reset[0] for x in reset));self.assertEqual((reset[0].currentSimpleInfo.chapterId,reset[0].currentSimpleInfo.missionId,reset[0].currentSimpleInfo.challengeTimes),(chapter,target,0))
                actual_paid+=price;self.assertEqual(self.profile(login)[0]['diamond'],natural_diamond-actual_paid)
                saved=self.profile(login);self.stop();self.start();login=self.login(uid)
                self.assertEqual(self.call('CSCopyResetChallengeCount',req,login,paid_stamp,'SCCopyResetChallengeCount'),reset[0]);self.assertEqual(self.profile(login),saved)
                # Actual next sweep uses the newly available original attempt, no bonus eligibility inserted.
                continued=call('CSCopySweep',query,'SCCopySweep');self.assertTrue(continued.gain.simps)
                progress=call('CSCopyPassedInfo',{},'SCCopyPassedInfo');self.assertEqual(next(x.challengeTimes for x in progress.passedCopyInfo if x.missionId==target),1)
                last_reset=reset[0];last_stamp=paid_stamp;last_request=req
            quote=call('CSCopyGetResetInfo',{'chapterId':'1','missionId':'5'},'SCCopyGetResetInfo');self.assertEqual(quote.remainResetCount,0);self.assertEqual(len(quote.cost),1)
            # Querying another exhausted target retains the player-shared quota.
            target='13';chapter=next(x['ChapterId'] for x in missions if x['MissionId']==target);maximum=int(next(x['ChallengeCount'] for x in missions if x['MissionId']==target))
            for _ in range(maximum):call('CSCopySweep',{'chapterId':chapter,'missionId':target},'SCCopySweep')
            query={'chapterId':chapter,'missionId':target};quote=call('CSCopyGetResetInfo',query,'SCCopyGetResetInfo');self.assertEqual(quote.remainResetCount,0);before=self.profile(login);stamp+=1
            self.assertEqual(self.stamped('CSCopyResetChallengeCount',dict(query,cost=quote.cost[0]),login,stamp)[0],10001);self.assertEqual(self.profile(login),before)
            facts=int(self.sql(f"SELECT COALESCE(SUM(amount),0) FROM p1_event_facts WHERE player_id={login.playerId} AND kind='diamond_spent';").strip());self.assertEqual(facts,actual_paid)
            self.sql("UPDATE copy_reset_rules SET enabled=0;");self.stop();self.start();login=self.login(uid)
            self.assertEqual(self.call('CSCopyResetChallengeCount',last_request,login,last_stamp,'SCCopyResetChallengeCount'),last_reset)
            self.assertEqual(self.sql('SELECT enabled FROM copy_reset_rules WHERE id=1;').strip(),'0');self.sql('UPDATE copy_reset_rules SET enabled=1;')
            results.append({'vip':vip,'quota':len(prices),'priceTiers':prices,'actualSourceOriginalStarDiamond':natural_diamond,'paidDiamond':actual_paid,'8each':'one reset/one price/P1 accurate/cold','realExhaustAndContinue':'original server sweep to cap -> exact targeted0 -> new original sweep1','playerSharedCap':'another exhausted mission remain0/price nonempty/new reset rejects','off':'current old receipt retained after actual continue and cold'})
        report={'date':'2026-10-14','policy':'copy-reset-original-tiers-v1','results':results,'preparation':'finite level100/guide8/stamina1000/bag1000 and original historicalstars; VIP12 eligibility explicitly prepared only for high-tier rule check, not original paidVIP source; no diamond/coin/reset/challenge count/cost event inserted, original starboxes fund actual original prices and real server sweeps create exhaustion','privateUnknown':'player-shared vsmission scope not proved; named conservative shared player-day default, sourcefreeze/operators preserved; originaltiers20/30/50/100 and originalVIP numPerk1 correct','productionModified':False,'phoneUI':False,'seconds':round(time.monotonic()-began,3),'sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest()}
        (shared.ROOT/'analysis/p5/copy-reset-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
