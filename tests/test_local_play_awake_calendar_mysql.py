"""Prebuilt R8 original Awake calendar and accepted type16 End lifecycle.

Reuses the SHA-bound historical 001/003/037/Pet002 prerequisite and existing
TestP7AwakeExportOriginalHTTPStart calculator. Ordinary wallets/items/progress,
winners, scores and receipts are never imported. No compiler or phone access.
"""
import concurrent.futures
import copy
import datetime
import fcntl
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import tempfile
import time
import types
import unittest

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport, sha
from test_p7_rush_party_mysql import historical_roster, fixture_sql, ROSTER_SHA, ROSTER
from server.codec import CLASSES, parse_http
import test_continued_gameplay_mysql as wire

ROOT=Path(__file__).resolve().parents[1]
ZONE=datetime.timezone(datetime.timedelta(hours=8))


def contracts():
    vectors={
        'CSAwakeCopyData':{},'CSEnterAwakeCopy':{'copyId':'501'},
        'CSAwakeCopyOpenBox':{'boxGuid':'actual-guid'},
        'CSAwakeCopyFightBegin':{'monGuid':'actual-guid'},
        'CSAwakeCopyFightEnd':{'fightId':'actual-fight','isWinClient':0,
            'optRecord':{'finalRoundNum':1,'attackerTotalLeftHp':0,'defenderTotalLeftHp':1}},
        'CSAwakeCopySettle':{},
        'CSFormationOn':{'id':'pet_002','guid':'actual-owned-pet','formatPos':1},
    }
    for name,fields in vectors.items():
        if parse_http(wire.encode_original_http(name,fields,2,b'0'*16,100)).decode()!=CLASSES[name](**fields):
            raise AssertionError('Original request changed: '+name)
    for name,fields in {
        'SCAwakeCopyData':{'copys','lastNum','isAutoSettled'},
        'SCEnterAwakeCopy':{'boxes','monsters','refreshData','isOver'},
        'SCAwakeCopyFightBegin':{'fightId','battlePlayer','battleMonser','randomSeed','maxRoundNum','roomId'},
        'SCAwakeCopyFightEnd':{'isWinServer','refreshData','fightMonster'},
        'SCAwakeCopySettle':{'boxRewards','ScoreRewards','totalRewards','score'},
    }.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original response changed: '+name)
    roster=historical_roster()
    return roster,dict(requests=len(vectors),sourceRosterSHA256=ROSTER_SHA,
        finitePreparation='Historical equipped 001/003/037/Pet002 only; ordinary wallets/items/P7 progress remain fresh',
        calculator='Existing exact TestP7AwakeExportOriginalHTTPStart; no supplied winner/End or recompilation')


def preflight():
    roster,contract=contracts()
    source=Path(os.environ['HYJS_AWAKE_CALENDAR_SOURCE']).resolve()
    binary=Path(os.environ['HYJS_AWAKE_CALENDAR_BINARY']).resolve()
    record=Path(os.environ['HYJS_AWAKE_CALENDAR_VALIDATION']).resolve()
    calculator=Path(os.environ['HYJS_AWAKE_CALENDAR_SERVICE_TEST']).resolve()
    if source.parent!=ROOT/'build/candidates': raise AssertionError('Direct immutable root candidate required')
    pinned=set()
    for row in json.loads((source/'manifest.json').read_bytes())['files']:
        path=(source/row['path']).resolve()
        if not path.is_relative_to(source) or path in pinned or sha(path)!=row['sha256']:
            raise AssertionError('Source pin mismatch: '+row['path'])
        pinned.add(path)
    if set((source/'hyjs-server').rglob('*.go'))-pinned: raise AssertionError('Unpinned Go')
    proof=json.loads(record.read_bytes())
    if proof.get('qualified') is not True or (ROOT/proof['candidate']).resolve()!=source:
        raise AssertionError('Completed exact root qualification required')
    if sha(binary)!=(proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use same qualified app without compiling')
    if not calculator.is_relative_to(ROOT/'build') or sha(calculator)!=os.environ['HYJS_AWAKE_CALENDAR_SERVICE_TEST_SHA256']:
        raise AssertionError('Root explicitly pinned existing service.test required')
    required={
        'hyjs-server/service/localPlayAwakeCalendarService.go':'p7AwakeScoreClock',
        'hyjs-server/service/p7AwakeService.go':'saveP7AwakeCalendar(tx, daily, now)',
        'hyjs-server/service/p7AwakeBusiness_test.go':'func TestP7AwakeExportOriginalHTTPStart(',
    }
    for path,needle in required.items():
        if needle not in (source/path).read_text(): raise AssertionError('Calendar/real-End integration absent: '+path)
    # The existing calculator reads these original assets in its established cwd.
    # Verify all of them, rather than silently reading a newer working table.
    for path in (source/'analysis/tables').glob('*.tsv'):
        if sha(path)!=sha(ROOT/path.relative_to(source)): raise AssertionError('Calculator original tables differ: '+path.name)
    for path in ['server/codec.py','server/protocol/schema.json',str(ROSTER.relative_to(ROOT)),
                 'hyjs-server/service/p7AwakeBusiness_test.go','hyjs-server/service/p7Awake16ScopeService.go']:
        if sha(source/path)!=sha(ROOT/path): raise AssertionError('Existing calculator/roster contract drift: '+path)
    const=(source/'hyjs-server/constUtil/p7Awake16Scope.go').read_text()
    match=re.search(r'P7Awake16NativeProof\s*=\s*"([0-9a-f]{64})"',const)
    if match is None: raise AssertionError('Independent original type16 proof must remain pinned and nonempty')
    contract.update(scopeProof=match[1],calculatorSHA256=sha(calculator),
        runnerDependencies={p:sha(ROOT/p) for p in ['tests/test_p7_rush_party_mysql.py','tests/test_growth_recruit_mysql.py',
            'tests/test_continued_gameplay_mysql.py','tests/hyjs_managed_mysql.py']})
    return source,binary,calculator,roster,contract


@unittest.skipUnless(os.environ.get('HYJS_AWAKE_CALENDAR_MYSQL_TEST')=='1','Root coordinated prebuilt managed window only')
class LocalPlayAwakeCalendarMySQLTests(unittest.TestCase):
    send,login,call=transport.send,transport.login,transport.call
    stamped,profile,start,stop,sql=transport.stamped,transport.profile,transport.start,transport.stop,transport.sql
    json_request,tools,change=transport.json_request,transport.tools,transport.change

    def test_original_two_day_budgets_frozen_hundred_award_rewind_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        self.source,binary,calculator,roster,contract=preflight()
        self.binary=str(binary)
        output=Path(os.environ['HYJS_AWAKE_CALENDAR_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT/'analysis/growth-first-20261016'))
        self.assertFalse(output.exists(),'Preserve existing reports')
        # The established no-DB End generator deliberately accepts only analysis/p7.
        evidence=ROOT/'analysis/p7'/(output.stem+'-wire')
        self.assertFalse(evidence.exists(),'Preserve original Starts/calculations')
        evidence.mkdir(parents=True)
        report=dict(status='PENDING',fullChain=False,binarySHA256=sha(binary),runnerSHA256=sha(__file__),
            sourceManifestSHA256=sha(self.source/'manifest.json'),contract=contract,
            phoneUI=False,naturalRosterClaimed=False,winnerScoreReceiptImported=False,
            scope='Local501, two personal days/two actual runs each; existing independent type16 family',
            fullAwakeContent=False,ordinaryInventoryOrWalletImported=False,
            formalDBConnections=0,formalDBWrites=0,actualRuns=[],calendarChanges=[])
        began=time.monotonic()
        self.pool,self.process=ManagedMySQL(),None
        with tempfile.TemporaryDirectory(prefix='hyjs-awake-calendar-') as temporary:
            self.app_temp=types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock','a+') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
                try:
                    allocation=self.pool.allocate('awake_calendar')
                    self.database=allocation.database
                    self.env={k:v for k,v in os.environ.items() if k in {'PATH','HOME','TMPDIR','LANG','LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai',GOMEMLIMIT='768MiB',GOMAXPROCS='2')
                    self.start()
                    self.pool.sql(self.database,'UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    for table in ['p7_awake_runs','p7_awake_states','p7_awake_explorations','p7_awake_fights',
                                  'local_play_activity_operations','p1_event_facts','p6_battle_records']:
                        self.assertEqual(self.sql("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='"+table+"';").strip(),'InnoDB')
                    suffix=secrets.token_hex(5)
                    protected=self.login('demo-awake-calendar-protected-'+suffix)
                    self.assertEqual(protected.playerId,1)
                    self.call('CSPlayerCreate',{'playerName':'觉醒日历保护','heroId':'hero_001'},protected,1,'SCPlayerCreate')
                    protected_before=self.profile(protected)
                    uid='demo-awake-calendar-'+suffix
                    owner=self.login(uid)
                    stamp=int(time.time()*1000)
                    def http(name,fields,expected):
                        nonlocal stamp
                        stamp+=1
                        return self.call(name,fields,owner,stamp,expected)
                    def same_stamp(name,fields,expected,workers=4):
                        nonlocal stamp
                        stamp+=1
                        saved=stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
                            rows=list(pool.map(lambda _:self.call(name,fields,owner,saved,expected),range(workers)))
                        self.assertTrue(all(r==rows[0] for r in rows))
                        return rows[0],saved
                    def reject(name,fields):
                        before=self.profile(owner)
                        http(name,fields,'ErrorMessage')
                        self.assertEqual(self.profile(owner),before)
                    def day(time_state): return datetime.datetime.fromtimestamp(time_state['gameTimeMs']/1000,ZONE).strftime('%Y-%m-%d')
                    def change(action):
                        value=self.change(owner,action)
                        report['calendarChanges'].append(dict(action=action,offsetDays=value['offsetDays'],day=day(value)))
                        return value
                    def catalogue(left):
                        out=http('CSAwakeCopyData',{},'SCAwakeCopyData')
                        self.assertEqual(out.lastNum,left)
                        self.assertEqual(out.isAutoSettled,0)
                        return out
                    def cold():
                        nonlocal owner
                        before=self.profile(owner)
                        self.stop();self.start();owner=self.login(uid)
                        self.assertEqual(self.profile(owner),before)
                    http('CSPlayerCreate',{'playerName':'觉醒个人日真实链','heroId':'hero_001'},'SCPlayerCreate')
                    fresh=self.profile(owner)
                    self.pool.sql(self.database,fixture_sql(owner.playerId,roster))
                    expected=copy.deepcopy(fresh)
                    expected[0].update(roster['playerPatch']);expected[1].update(roster['gamePatch'])
                    self.assertEqual(self.profile(owner),expected)
                    http('CSFormationOn',{'id':roster['pet']['id'],'guid':roster['pet']['guid'],'formatPos':1},'SCFormationOn')
                    baseline=self.profile(owner)
                    for key in ['items','gems','cooking']:
                        self.assertEqual(baseline[1].get(key),fresh[1].get(key))
                    for key in ['coin','diamond','point']:
                        self.assertEqual(baseline[0].get(key,0),fresh[0].get(key,0))
                    state=self.tools(owner,'time/state')
                    now=datetime.datetime.fromtimestamp(state['gameTimeMs']/1000,ZONE)
                    for _ in range((4-now.weekday())%7): state=change('nextDay')
                    friday=day(state)
                    saturday=day(change('nextDay'))
                    self.assertEqual(next(c for c in catalogue(2).copys if c.chapterId=='5').isOpen,1)
                    reject('CSEnterAwakeCopy',{'copyId':'501'})
                    explore_hex=self.sql('SELECT HEX(source) FROM p7_awake_policies WHERE id=2 AND enabled=0 AND min_level=70;').strip()
                    battle_hex=self.sql('SELECT HEX(source) FROM p7_awake_policies WHERE id=3 AND enabled=0 AND min_level=70;').strip()
                    self.assertTrue(bytes.fromhex(explore_hex).decode().startswith('local-awake-forest-explore-v1:'))
                    self.assertTrue(bytes.fromhex(battle_hex).decode().startswith('local-awake16-forest-v1:'))
                    activation=("START TRANSACTION; SELECT id FROM p7_awake_policies WHERE id IN (2,3) ORDER BY id FOR UPDATE; "
                        "SELECT copy_id FROM p7_awake_stage_rules WHERE copy_id='501' FOR UPDATE; "
                        f"SET @ready=(SELECT COUNT(*)=1 FROM p7_awake_policies WHERE id=2 AND enabled=0 AND min_level=70 AND source=CONVERT(X'{explore_hex}' USING utf8mb4)) AND "
                        f"(SELECT COUNT(*)=1 FROM p7_awake_policies WHERE id=3 AND enabled=0 AND min_level=70 AND source=CONVERT(X'{battle_hex}' USING utf8mb4)) AND "
                        f"(SELECT COUNT(*)=1 FROM p7_awake_stage_rules WHERE copy_id='501' AND chapter_id='5' AND enabled=0 AND content_restored=1 AND source=CONVERT(X'{explore_hex}' USING utf8mb4)); "
                        "UPDATE p7_awake_policies SET enabled=1 WHERE id IN (2,3) AND @ready; "
                        "UPDATE p7_awake_stage_rules SET enabled=1 WHERE copy_id='501' AND @ready; SELECT @ready; COMMIT;")
                    self.assertEqual(self.pool.sql(self.database,activation).strip().splitlines()[-1],'1')
                    receipts=[]
                    accepted_days=[saturday,saturday,friday,friday]
                    for index,accepted_day in enumerate(accepted_days):
                        if index==1: self.assertEqual(day(change('nextDay')),saturday)
                        if index==2: self.assertEqual(day(change('previousDay')),friday)
                        catalogue(2-index%2)
                        before_entry=self.profile(owner)
                        enter,_=same_stamp('CSEnterAwakeCopy',{'copyId':'501'},'SCEnterAwakeCopy')
                        self.assertEqual((len(enter.boxes),len(enter.monsters),enter.refreshData.score),(4,1,0))
                        self.assertEqual(self.profile(owner),before_entry,'Original awake stamina0 must not invent a resource charge')
                        row=self.sql(f"SELECT r.id,r.start_at,r.end_at,r.freeze_hash,HEX(r.freeze) FROM p7_awake_runs r JOIN p7_awake_explorations e ON e.run_id=r.id WHERE r.player_id={owner.playerId} AND e.status='active';").strip().split('\t')
                        self.assertEqual(len(row),5)
                        frozen=json.loads(bytes.fromhex(row[4]))
                        self.assertEqual(frozen['businessDay'],accepted_day)
                        self.assertEqual(frozen['businessAt'],int(row[1])+self.tools(owner,'time/state')['offsetDays']*86400000)
                        self.assertEqual(int(row[2])-int(row[1]),frozen['durationMinutes']*60000)
                        if index==0:
                            self.assertEqual(day(change('previousDay')),friday)
                            self.assertEqual(http('CSEnterAwakeCopy',{'copyId':'501'},'SCEnterAwakeCopy'),enter)
                            self.assertEqual(enter.refreshData.endTime,int(row[2]))
                            catalogue(2)
                        for box_index,box in enumerate(enter.boxes):
                            if index==0 and box_index==0:
                                stamp+=1;saved=stamp
                                with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                                    opened=list(pool.map(lambda _:self.call('CSAwakeCopyOpenBox',{'boxGuid':box.guid},owner,saved,'SCAwakeCopyOpenBox'),range(4)))
                                self.assertEqual(sum(bool(r.rewards) for r in opened),1)
                            else: http('CSAwakeCopyOpenBox',{'boxGuid':box.guid},'SCAwakeCopyOpenBox')
                        before_end=self.profile(owner)
                        start=http('CSAwakeCopyFightBegin',{'monGuid':enter.monsters[0].guid},'SCAwakeCopyFightBegin')
                        self.assertEqual([a.id for a in start.battlePlayer.formation],['hero_001','hero_003'])
                        start_file=evidence/f'{index+1:02}.start.pb'
                        start_file.write_bytes(start.SerializeToString(deterministic=True))
                        terminal_file=evidence/f'{index+1:02}.computed.json'
                        calc_env={k:v for k,v in self.env.items() if k in {'PATH','HOME','TMPDIR','LANG','LC_ALL','GOMEMLIMIT','GOMAXPROCS'}}
                        calc_env.update(HYJS_P7_AWAKE_HTTP_START=str(start_file),HYJS_P7_AWAKE_HTTP_START_SHA256=sha(start_file),HYJS_P7_AWAKE_HTTP_OUTPUT=str(terminal_file))
                        calculation=subprocess.run([str(calculator),'-test.run=^TestP7AwakeExportOriginalHTTPStart$','-test.v'],
                            cwd=ROOT/'hyjs-server/service',env=calc_env,capture_output=True,text=True,timeout=120)
                        (evidence/f'{index+1:02}.calculator.log').write_text(calculation.stdout+calculation.stderr)
                        self.assertEqual(calculation.returncode,0,(calculation.stdout+calculation.stderr)[-3000:])
                        self.assertIn('--- PASS: TestP7AwakeExportOriginalHTTPStart ',calculation.stdout)
                        terminal=json.loads(terminal_file.read_bytes())
                        self.assertFalse(terminal['outcome']['authoritative'])
                        actual=int(terminal['outcome']['winner']==0)
                        self.assertEqual(actual,1,'Preserve a true defeat, never replace the result')
                        end={'fightId':start.fightId,'isWinClient':actual,'optRecord':terminal['originalOperates']}
                        self.assertEqual(self.sql(f"SELECT CONCAT(scope_proof,'|',status) FROM p7_awake_fights WHERE player_id={owner.playerId} AND id='{start.fightId}';").strip(),contract['scopeProof']+'|pending')
                        if index==0:
                            bad=copy.deepcopy(end);bad['isWinClient']=0
                            reject('CSAwakeCopyFightEnd',bad)
                            cold()
                            self.assertEqual(http('CSAwakeCopyFightBegin',{'monGuid':enter.monsters[0].guid},'SCAwakeCopyFightBegin'),start)
                        accepted,_=same_stamp('CSAwakeCopyFightEnd',end,'SCAwakeCopyFightEnd')
                        self.assertEqual((accepted.isWinServer,accepted.refreshData.score,accepted.fightMonster.hpPercent),(1,100,0))
                        self.assertEqual(self.profile(owner),before_end,'Accepted End must retain rewards until original Settle')
                        receipt,settle_stamp=same_stamp('CSAwakeCopySettle',{},'SCAwakeCopySettle')
                        self.assertEqual(receipt.score,100)
                        self.assertEqual(len(receipt.ScoreRewards),2 if index%2==0 else 0)
                        if index%2==0:
                            self.assertEqual({r.id:r.num for r in receipt.ScoreRewards},{'coin':5000,'hero_exp_002':1})
                        receipts.append((settle_stamp,receipt))
                        fact=self.sql(f"SELECT source_key,day FROM p1_event_facts WHERE player_id={owner.playerId} AND source_key='awake-score-day:{accepted_day}' AND kind='awake-local-daily-score';").strip().split('\t')
                        self.assertEqual(fact,['awake-score-day:'+accepted_day,friday])
                        saved_freeze=self.sql(f"SELECT CONCAT(freeze_hash,'|',HEX(freeze)) FROM p7_awake_runs WHERE id='{row[0]}';").strip()
                        self.assertEqual(saved_freeze,row[3]+'|'+row[4])
                        report['actualRuns'].append(dict(runID=row[0],fightID=start.fightId,acceptedDay=accepted_day,
                            completionDay=day(self.tools(owner,'time/state')),score=100,dailyReward=index%2==0,
                            startSHA256=sha(start_file),calculationSHA256=sha(terminal_file),actualWinner=terminal['outcome']['winner']))
                        if index==0:
                            cold()
                            saved=self.profile(owner)
                            self.assertEqual(self.call('CSAwakeCopySettle',{},owner,settle_stamp,'SCAwakeCopySettle'),receipt)
                            self.assertEqual(self.profile(owner),saved)
                    catalogue(0)
                    reject('CSEnterAwakeCopy',{'copyId':'501'})
                    self.assertEqual(day(change('nextDay')),saturday)
                    catalogue(0);reject('CSEnterAwakeCopy',{'copyId':'501'})
                    self.assertEqual(day(change('previousDay')),friday)
                    catalogue(0)
                    for accepted_day in [friday,saturday]:
                        raw=self.sql(f"SELECT HEX(response) FROM local_play_activity_operations WHERE player_id={owner.playerId} AND request_id='awake-day:{accepted_day}' AND action='awake-day' AND activity_id='awake';").strip()
                        self.assertEqual(json.loads(bytes.fromhex(raw)),{'day':accepted_day,'used':2})
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p7_awake_explorations WHERE player_id={owner.playerId} AND status='settled';").strip(),'4')
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p6_battle_records WHERE player_id={owner.playerId} AND source='p7-awake16' AND authoritative=1;").strip(),'4')
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={owner.playerId} AND kind='awake-local-daily-score';").strip(),'2')
                    cold();saved=self.profile(owner)
                    for saved_stamp,receipt in receipts:
                        self.assertEqual(self.call('CSAwakeCopySettle',{},owner,saved_stamp,'SCAwakeCopySettle'),receipt)
                        self.assertEqual(self.profile(owner),saved)
                    catalogue(0)
                    self.assertEqual(self.profile(protected),protected_before)
                    report.update(status='PASS',fullChain=True,actualTrueHundredRuns=4,
                        originalDailyBudget=2,distinctAcceptedAwardDays=2,realDeadlineUnchanged=True,
                        noQuotaResetOnRevisit=True,coldOriginalReceiptsExact=True,
                        concurrentEndAndSettleOnce=True,forgedEndRejected=True,protected1Exact=True,
                        actualInventorySource='Only four real exploration settlements; no reward/winner injection')
                except BaseException as error:
                    report.update(status='FAIL',failureType=type(error).__name__,failure=self.pool.redact(str(error)))
                    raise
                finally:
                    self.stop();self.pool.close()
                    report.update(seconds=round(time.monotonic()-began,3),ownedManagedAllocationsRemaining=len(self.pool.connections))
                    report['ownedAppStopped']=self.process is None or self.process.poll() is not None
                    output.parent.mkdir(parents=True,exist_ok=True)
                    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__': unittest.main()
