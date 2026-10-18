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

OWNED = ['hyjs-server/service/copySpecialDropService.go', 'hyjs-server/service/copySpecialDrop_test.go', 'hyjs-server/service/copyDropService.go', 'hyjs-server/service/copySweepService.go', 'hyjs-server/constUtil/copySpecialDrop.go', 'hyjs-server/db/copySpecialRule.go', 'hyjs-server/db/copySpecialState.go', 'hyjs-server/db/sql/108_copy_special_rules.sql', 'hyjs-server/db/sql/108_copy_special_rules.seed.sql', 'hyjs-server/db/sql/108_copy_special_states.sql', 'analysis/p5/copy-special-original-audit.json', 'tests/test_p5_copy_special_mysql.py']

@unittest.skipUnless(os.environ.get('HYJS_P5_COPY_SPECIAL_MYSQL_TEST') == '1', 'Explicit locked isolated P5 gem supply')
class CopySpecialMySQLTests(unittest.TestCase):
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
        cls.database = 'hyjs_test_p5_copy_special_' + secrets.token_hex(6)
        cls.test_user = 'p5special_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-special-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-gem-supply')
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database)
        base = Path(os.environ.get('HYJS_P5_COPY_SPECIAL_BASE', shared.ROOT / 'build/candidates/joint-natural-guild-v59-20261014/hyjs-server')).resolve()
        if (shared.ROOT / 'build/candidates').resolve() not in base.parents: raise AssertionError('GemSupply baseline must be fixed local candidate')
        source=Path(cls.temp.name)/'source';shutil.copytree(base,source,ignore=shutil.ignore_patterns('logs'))
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
            if left.split() != ['0','0','0']: raise AssertionError('P5 gem supply schema/user/grants remain')
            report = shared.ROOT / 'analysis/p5/copy-special-mysql-validation.json'
            if report.exists() and getattr(cls, 'passed', False):
                data=json.loads(report.read_text());data['cleanupCounts']=[0,0,0];report.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p5_copy_special_') or not cls.test_user.startswith('p5special_'):raise AssertionError('P5 gem supply isolation guard')
        result=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:raise AssertionError('P5 gem supply administration failed: '+result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def test_original_frozen_special_wins_sweep_fragment_combine_and_cold(self):
        began=time.monotonic();self.start();stamp=1800130000000;uid='actual-special-equipment';login=self.login(uid)
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        tasks=shared.original_table('CopyTask');missions=shared.original_table('CopyMission')
        def prepare():
            p,g=self.profile(login);p.update(level=100,coin=0,stamina=1000,equipBagSize=1000,warriorBagSize=1000,noviceGuideSchedule=8)
            g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions];g['copy']['passedChapterInfo']=[];g['copy_first_passed_tasks']=[x['TaskId'] for x in tasks];g['run']=None
            self.write_profile(login,p,g)
        call('CSPlayerCreate',{'playerName':'原特殊四胜装备','heroId':'hero_043'},'SCPlayerCreate');prepare()
        self.sql('UPDATE p4_copy_benefit_rules SET enabled=0;') # isolate the new source from existing independent double windows
        self.assertEqual(self.sql("SELECT enabled,cycle FROM copy_special_rules WHERE id='special-local-v1';").strip(),'1\t4')
        before_gear=len(self.profile(login)[1].get('equipment',[]));final_end=None;final_result=None;final_stamp=None
        fight_cursor_shifts=0
        for cycle in range(4):
            if cycle==3:
                p,g=self.profile(login);g['copy_challenge_day']='20000101';self.write_profile(login,p,g);fight_cursor_shifts+=1
            call('CSCopyEnter',{'chapterId':'1','missionId':'5'},'SCCopyEnter')
            for tid in sorted([x['TaskId'] for x in tasks if x['MissionId']=='5'],key=int):
                start=call('CSCopyFightStart',{'chapterId':'1','missionId':'5','taskId':tid},'SCCopyFightStart');end={'fightId':start.fightId,'result':1,'battlePlayer':start.battlePlayer}
                if tid=='20' and cycle==3:
                    frozen=json.loads(self.sql(f"SELECT CONVERT(input USING utf8mb4) FROM copy_drop_snapshots WHERE player_id={login.playerId} AND fight_id='{start.fightId}';").strip());self.assertEqual(frozen['special']['cycle'],4)
                    self.sql("UPDATE copy_special_rules SET cycle=9 WHERE id='special-local-v1';")
                    p,g=self.profile(login);p['equipBagSize']=len(g.get('equipment',[]));self.write_profile(login,p,g);saved=self.profile(login);stamp+=1
                    self.assertEqual(self.stamped('CSCopyFightEnd',end,login,stamp)[0],10001);self.assertEqual(self.profile(login),saved)
                    self.assertEqual(self.sql(f"SELECT successes,credit,spent,granted FROM copy_special_states WHERE player_id={login.playerId} AND task_id='20';").strip(),'3\t3\t0\t0')
                    p['equipBagSize']=1000;self.write_profile(login,p,g);stamp+=1;final_stamp=stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:results=list(pool.map(lambda _:self.call('CSCopyFightEnd',end,login,final_stamp,'SCCopyFightEnd'),range(8)))
                    if not all(x==results[0] for x in results):
                        from google.protobuf.json_format import MessageToDict
                        a=MessageToDict(results[0]);b=MessageToDict(next(x for x in results if x!=results[0]))
                        changes=[]
                        def diff(a,b,path=''):
                            if type(a)!=type(b):changes.append((path,a,b));return
                            if isinstance(a,dict):
                                for k in sorted(set(a)|set(b)):diff(a.get(k),b.get(k),path+'.'+k)
                            elif isinstance(a,list):
                                for i in range(max(len(a),len(b))):diff(a[i] if i<len(a) else None,b[i] if i<len(b) else None,path+'['+str(i)+']')
                            elif a!=b:changes.append((path,a,b))
                        diff(a,b);self.fail('special first/replay presence drift '+repr(changes[:10]))
                    final_result=results[0];final_end=end
                else:result=call('CSCopyFightEnd',end,'SCCopyFightEnd')
            current=len(self.profile(login)[1].get('equipment',[]));self.assertEqual(current-before_gear,1 if cycle==3 else 0)
        self.assertEqual(self.sql(f"SELECT successes,credit,spent,granted FROM copy_special_states WHERE player_id={login.playerId} AND task_id='20';").strip(),'4\t0\t4\t1')
        equip=next(x for x in final_result.gain.equips if x.equipId=='equip_001');self.assertTrue(any(x.id==equip.equipId and x.guid==equip.guid for x in final_result.currentResource))
        saved=self.profile(login);self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSCopyFightEnd',final_end,login,final_stamp,'SCCopyFightEnd'),final_result);self.assertEqual(self.profile(login),saved)
        call('CSFormationOn',{'formatPos':1,'id':equip.equipId,'guid':equip.guid},'SCFormationOn')
        # An old snapshot lacking Special does not receive retrospective credit.
        call('CSCopyEnter',{'chapterId':'1','missionId':'5'},'SCCopyEnter')
        for tid in sorted([x['TaskId'] for x in tasks if x['MissionId']=='5'],key=int):
            start=call('CSCopyFightStart',{'chapterId':'1','missionId':'5','taskId':tid},'SCCopyFightStart')
            if tid=='20':
                frozen=json.loads(self.sql(f"SELECT CONVERT(input USING utf8mb4) FROM copy_drop_snapshots WHERE player_id={login.playerId} AND fight_id='{start.fightId}';").strip());frozen.pop('special',None)
                raw=json.dumps(frozen,ensure_ascii=False,separators=(',',':')).encode();digest=hashlib.sha256(raw).hexdigest()
                self.sql(f"UPDATE copy_drop_snapshots SET input=X'{raw.hex()}',input_hash='{digest}' WHERE player_id={login.playerId} AND fight_id='{start.fightId}';")
            call('CSCopyFightEnd',{'fightId':start.fightId,'result':1,'battlePlayer':start.battlePlayer},'SCCopyFightEnd')
        self.assertEqual(self.sql(f"SELECT successes,credit,spent,granted FROM copy_special_states WHERE player_id={login.playerId} AND task_id='20';").strip(),'4\t0\t4\t1')
        # Original sweep pays real stamina and conserves repeated fragment source.
        uid='actual-special-fragment';login=self.login(uid);call('CSPlayerCreate',{'playerName':'原特殊碎片合成','heroId':'hero_043'},'SCPlayerCreate');prepare();self.sql("UPDATE copy_special_rules SET cycle=4,enabled=1 WHERE id='special-local-v1';")
        cost_stamina=0;cursor_shifts=0;last=None;last_stamp=None
        for n in range(40):
            if n and n%int(next(x['ChallengeCount'] for x in missions if x['MissionId']=='7'))==0:
                p,g=self.profile(login);g['copy_challenge_day']='20000101';self.write_profile(login,p,g);cursor_shifts+=1
            sweep={'chapterId':'2','missionId':'7'};stamp+=1;last_stamp=stamp
            if n==39:
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:results=list(pool.map(lambda _:self.call('CSCopySweep',sweep,login,last_stamp,'SCCopySweep'),range(8)))
                self.assertTrue(all(x==results[0] for x in results));last=results[0]
            else:last=self.call('CSCopySweep',sweep,login,last_stamp,'SCCopySweep')
            cost_stamina+=sum(x.num for x in last.cost if x.id=='stamina')
            self.assertEqual(sum(x.num for x in last.gain.simps if x.id=='frag_hero_032'),1 if (n+1)%4==0 else 0)
        self.assertEqual(cost_stamina,400);self.assertEqual(self.profile(login)[0]['stamina'],600)
        self.assertEqual(next(x['num'] for x in self.profile(login)[1]['items'] if x['id']=='frag_hero_032'),10)
        self.assertEqual(self.sql(f"SELECT successes,credit,spent,granted FROM copy_special_states WHERE player_id={login.playerId} AND task_id='28';").strip(),'40\t0\t40\t10')
        saved=self.profile(login);self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSCopySweep',sweep,login,last_stamp,'SCCopySweep'),last);self.assertEqual(self.profile(login),saved)
        combine={'targetId':'hero_032'};stamp+=1;combine_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:combined=list(pool.map(lambda _:self.call('CSCombine',combine,login,combine_stamp,'SCCombine'),range(8)))
        self.assertTrue(all(x==combined[0] for x in combined));warrior=combined[0].get.wars[0];self.assertEqual(warrior.id,'hero_032');self.assertEqual([(x.id,x.num) for x in combined[0].cost],[('frag_hero_032',10)])
        self.assertEqual(next((x['num'] for x in self.profile(login)[1]['items'] if x['id']=='frag_hero_032'),0),0)
        call('CSFormationOn',{'formatPos':2,'id':warrior.id,'guid':warrior.guid},'SCFormationOn');saved=self.profile(login);self.stop();self.start();login=self.login(uid);self.assertEqual(self.profile(login),saved)
        self.sql("UPDATE copy_special_rules SET enabled=0 WHERE id='special-local-v1';")
        p,g=self.profile(login);g['copy_challenge_day']='20000101';self.write_profile(login,p,g)
        stamp+=1;plain=self.call('CSCopySweep',sweep,login,stamp,'SCCopySweep');self.assertFalse(any(x.id=='frag_hero_032' for x in plain.gain.simps))
        self.assertEqual(self.sql(f"SELECT successes FROM copy_special_states WHERE player_id={login.playerId} AND task_id='28';").strip(),'40')
        self.stop();self.start();login=self.login(uid);self.assertEqual(self.sql("SELECT enabled,cycle FROM copy_special_rules WHERE id='special-local-v1';").strip(),'0\t4')
        self.assertEqual(self.call('CSCombine',combine,login,combine_stamp,'SCCombine'),combined[0])
        report={'date':'2026-10-14','policy':'copy-special-local-four-wins-v1','originalCoverage':'79 tasks102 opaque tuples;4equip/98frag; domain all4local ticks each passed','actualFightSource':'four whole original mission5 runs (12 actual original HTTP starts/ends with explicit isolated client-result victory contract, not original-engine/phone winner); Task20 fourth original equip001 actual GUID/currentResource/equip/cold','frozenOld4New9':'start4 cycle retained during live9; capacity failure counter/profile rollback3/3/0/0; eight identical actual final end, counter4/0/4/1; old JSON snapshot without Special adds no credit','actualSweepSource':'40 original mission7 sweeps/400paid stamina, rawfrag03210, exact8finalonce/cold -> actual original10fragment combine hero032 uniqueGUID -> formationpos2/cold','eligibilityOnlyPreviousDayCursors':{'fight':fight_cursor_shifts,'sweep':cursor_shifts},'operatorOff':'new source count/grant stops, current old combine receipt cold retained, reinitialize retains off/cycle4','preparation':'finite level100/stamina1000/bag1000/prior originalstars+firstpassed; copy history qualifies input but never seeds special counters/resources; no equipment/fragment/warrior GUID/sourcecoin/XP inserted; isolated doublebenefits off; wins are explicitly client-result fixtures, sweep original server transaction','originalPrivateUnknown':'three values are preserved opaque audit data; local Cycle4 is independent, no claim of original probabilities/pity','productionModified':False,'phoneUI':False,'seconds':round(time.monotonic()-began,3),'sourceSHA256':{x:hashlib.sha256((shared.ROOT/x).read_bytes()).hexdigest() for x in OWNED},'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest()}
        (shared.ROOT/'analysis/p5/copy-special-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');self.__class__.passed=True

if __name__=='__main__':unittest.main()
