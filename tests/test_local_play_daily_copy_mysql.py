"""Prebuilt calendar: original 16 daily-copy types/week, paid Start recovery.

Only Player75 is finite. No currency/stamina/material/winner is inserted. No
FightEnd is sent: the isolated database owns two real pending Starts until
cleanup. Cross-day replay does not claim original native combat certification.
"""
import concurrent.futures
import datetime
import fcntl
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import types
import unittest

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport, sha
from test_p5_talent_scope_mysql import original_table
from server.codec import CLASSES,parse_http
import test_continued_gameplay_mysql as wire

ROOT=Path(__file__).resolve().parents[1]
ZONE=datetime.timezone(datetime.timedelta(hours=8))


def contracts():
    for name,fields in {'CSDailyCopyState':{},'CSDailyCopyFightStart':{'ChapterId':'1'},
                        'CSCopyEnter':{'chapterId':'1','missionId':'1'}}.items():
        if parse_http(wire.encode_original_http(name,fields,2,b'0'*16,100)).decode()!=CLASSES[name](**fields):
            raise AssertionError('Original request differs: '+name)
    for name,fields in {'SCDailyCopyState':{'items'},'DailyCopyStateType':{'type','isOpen','lastTime','dailyCopyState'},
        'DailyCopyState':{'ChapterId','isOpen','LevelLimit','Recommend','Drop'},
        'SCDailyCopyFightStart':{'fightId','roomId','randSeed','battlePlayer','monster','maxRoundNum'}}.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original response differs: '+name)
    rules={r['Type']:r for r in original_table('DailyCopy')}
    chapters=original_table('DailyCopyChapter')
    if len(rules)!=16 or len(chapters)!=48 or any(sum(c['Type']==kind for c in chapters)!=3 for kind in rules):
        raise AssertionError('Original 16 types/48 difficulties required')
    if {k:int(v['Count']) for k,v in rules.items() if v['Count']!='1'}!={'GreatTrea':3,'PetEquip':3,'Theatric':3}:
        raise AssertionError('Original one/three daily limits changed')
    first=next(r for r in chapters if r['ChapterId']=='1')
    if first['Type']!='HeroExp' or rules['HeroExp']['OpenDay']!='1,3,5,7' or rules['HeroExp']['Stamina']!='10':
        raise AssertionError('Original HeroExp Monday/Wednesday Start route changed')
    return rules,chapters


def preflight():
    rules,chapters=contracts()
    source=Path(os.environ['HYJS_DAILY_CALENDAR_SOURCE']).resolve()
    binary=Path(os.environ['HYJS_DAILY_CALENDAR_BINARY']).resolve()
    record=Path(os.environ['HYJS_DAILY_CALENDAR_VALIDATION']).resolve()
    if source.parent!=ROOT/'build/candidates': raise AssertionError('Direct frozen root candidate required')
    paths=set()
    for row in json.loads((source/'manifest.json').read_bytes())['files']:
        path=(source/row['path']).resolve()
        if not path.is_relative_to(source) or path in paths or sha(path)!=row['sha256']:
            raise AssertionError('Frozen pin differs: '+row['path'])
        paths.add(path)
    if set((source/'hyjs-server').rglob('*.go'))-paths: raise AssertionError('Unpinned Go source')
    proof=json.loads(record.read_bytes())
    if proof.get('qualified') is not True or (ROOT/proof['candidate']).resolve()!=source:
        raise AssertionError('Root completed exact qualification required')
    if sha(binary)!=(proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use qualified same binary; no compiler')
    for name in ['server/codec.py','server/protocol/schema.json',*['analysis/tables/Table'+n+'.tsv'
            for n in ['DailyCopy','DailyCopyChapter','DailyCopyEnemy']]]:
        if sha(source/name)!=sha(ROOT/name): raise AssertionError('Original contract asset differs: '+name)
    daily=(source/'hyjs-server/service/dailyCopyService.go').read_text()
    owner=(source/'hyjs-server/service/combatOwnershipService.go').read_text()
    for needle in ['now = LocalPlayAt(tx.Statement.Context, now)',
                   'playNow := LocalPlayAt(tx.Statement.Context, now)',
                   'dailyCopyDay(playNow)']:
        if needle not in daily: raise AssertionError('Personal daily-copy integration missing: '+needle)
    if 'dailyCopyDay(LocalPlayAt(tx.Statement.Context, now))' not in owner:
        raise AssertionError('Cross-mode ownership must use same personal day')
    return source,binary,rules,chapters


@unittest.skipUnless(os.environ.get('HYJS_DAILY_CALENDAR_MYSQL_TEST')=='1','Root prebuilt managed window only')
class LocalPlayDailyCopyMySQLTests(unittest.TestCase):
    send,login,call=transport.send,transport.login,transport.call
    stamped,profile,start,stop,sql=transport.stamped,transport.profile,transport.start,transport.stop,transport.sql
    json_request,tools,change=transport.json_request,transport.tools,transport.change

    def test_original_week_start_quota_ownership_day_revisit_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        self.source,binary,rules,chapters=preflight()
        self.binary=str(binary)
        output=Path(os.environ['HYJS_DAILY_CALENDAR_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT/'analysis/growth-first-20261016'))
        self.assertFalse(output.exists(),'Retain previous attempts')
        report=dict(status='PENDING',fullChain=False,runnerSHA256=sha(__file__),binarySHA256=sha(binary),
            sourceManifestSHA256=sha(self.source/'manifest.json'),finitePreparation='Only Player75',
            noResourceOrWinInsertion=True,fightEndSent=False,phoneUI=False,
            formalDBConnections=0,formalDBWrites=0,week=[],starts=[])
        began=time.monotonic()
        self.pool,self.process=ManagedMySQL(),None
        with tempfile.TemporaryDirectory(prefix='hyjs-daily-calendar-') as temporary:
            self.app_temp=types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock','a+') as lock:
                fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
                try:
                    allocation=self.pool.allocate('daily_calendar')
                    self.database=allocation.database
                    self.env={k:v for k,v in os.environ.items() if k in {'PATH','HOME','TMPDIR','LANG','LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai',GOMEMLIMIT='768MiB',GOMAXPROCS='2')
                    self.start()
                    self.pool.sql(self.database,'UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    suffix=secrets.token_hex(5)
                    protected=self.login('demo-daily-protected-'+suffix)
                    self.assertEqual(protected.playerId,1)
                    self.call('CSPlayerCreate',{'playerName':'日常保护','heroId':'hero_001'},protected,1,'SCPlayerCreate')
                    protected_before=self.profile(protected)
                    uid='demo-daily-calendar-'+suffix
                    owner=self.login(uid)
                    stamp=int(time.time()*1000)
                    def http(name,fields,expected):
                        nonlocal stamp
                        stamp+=1
                        return self.call(name,fields,owner,stamp,expected)
                    def once_start():
                        nonlocal stamp
                        stamp+=1
                        saved=stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            rows=list(workers.map(lambda _:self.call('CSDailyCopyFightStart',{'ChapterId':'1'},owner,saved,'SCDailyCopyFightStart'),range(8)))
                        self.assertTrue(all(r==rows[0] for r in rows))
                        return rows[0],saved
                    def clock(): return self.tools(owner,'time/state')
                    def local_date(state): return datetime.datetime.fromtimestamp(state['gameTimeMs']/1000,ZONE)
                    http('CSPlayerCreate',{'playerName':'日常个人星期','heroId':'hero_001'},'SCPlayerCreate')
                    initial=self.profile(owner)
                    self.pool.sql(self.database,f"UPDATE profiles SET player=JSON_SET(player,'$.level',75) WHERE id={owner.playerId};")
                    self.assertEqual(self.profile(owner),[dict(initial[0],level=75),initial[1]])
                    # No fight exists during the complete weekday projection matrix.
                    for day in range(7):
                        time_state=clock() if day==0 else self.change(owner,'nextDay')
                        now=local_date(time_state)
                        weekday=now.isoweekday()
                        response=http('CSDailyCopyState',{},'SCDailyCopyState')
                        self.assertEqual({r.type for r in response.items},set(rules))
                        for item in response.items:
                            rule=rules[item.type]
                            opened=weekday in set(map(int,rule['OpenDay'].split(',')))
                            self.assertEqual((item.isOpen,item.lastTime),(int(opened),int(rule['Count'])))
                            expected={r['ChapterId']:r for r in chapters if r['Type']==item.type}
                            self.assertEqual({r.ChapterId for r in item.dailyCopyState},set(expected))
                            for difficulty in item.dailyCopyState:
                                row=expected[difficulty.ChapterId]
                                self.assertEqual((difficulty.LevelLimit,difficulty.Recommend,difficulty.Drop),
                                    (int(row['LevelLimit']),int(row['Recommend']),row['Drop']))
                                self.assertEqual(difficulty.isOpen,int(opened and int(row['LevelLimit'])<=75))
                        report['week'].append(dict(day=now.strftime('%Y-%m-%d'),weekday=weekday,checkedTypes=16,checkedDifficulties=48))
                    self.change(owner,'reset')
                    time_state=clock()
                    # Choose an actual upcoming Monday: +2 is always Wednesday,
                    # both original HeroExp open days, without a test clock.
                    for _ in range((1-local_date(time_state).isoweekday())%7):
                        time_state=self.change(owner,'nextDay')
                    day1=local_date(time_state).strftime('%Y-%m-%d')
                    before=self.profile(owner)
                    first,first_stamp=once_start()
                    self.assertEqual(self.profile(owner)[0]['stamina'],before[0]['stamina']-10)
                    before_reject=self.profile(owner)
                    http('CSCopyEnter',{'chapterId':'1','missionId':'1'},'ErrorMessage')
                    self.assertEqual(self.profile(owner),before_reject)
                    self.assertIsNone(self.profile(owner)[1].get('run'))
                    state=http('CSDailyCopyState',{},'SCDailyCopyState')
                    active=next(r for r in state.items if r.type=='HeroExp')
                    self.assertEqual((active.isOpen,active.lastTime),(1,1))
                    self.assertEqual([r.ChapterId for r in active.dailyCopyState if r.isOpen],['1'])
                    self.change(owner,'nextDay')
                    time_state=self.change(owner,'nextDay')
                    day2=local_date(time_state).strftime('%Y-%m-%d')
                    self.assertEqual(local_date(time_state).isoweekday(),3)
                    state=http('CSDailyCopyState',{},'SCDailyCopyState')
                    self.assertEqual(next(r for r in state.items if r.type=='HeroExp').lastTime,1)
                    before=self.profile(owner)
                    second,second_stamp=once_start()
                    self.assertNotEqual(second.fightId,first.fightId)
                    self.assertEqual(self.profile(owner)[0]['stamina'],before[0]['stamina']-10)
                    self.change(owner,'previousDay');self.change(owner,'previousDay')
                    self.assertEqual(local_date(clock()).strftime('%Y-%m-%d'),day1)
                    before=self.profile(owner)
                    resumed,resume_stamp=once_start()
                    self.assertEqual(resumed,first)
                    self.assertEqual(self.profile(owner),before)
                    state_rows=self.sql(f"SELECT day,used,`limit` FROM daily_copy_states WHERE player_id={owner.playerId} AND type='HeroExp' ORDER BY day;").strip().splitlines()
                    self.assertEqual(state_rows,[day1+'\t1\t1',day2+'\t1\t1'])
                    fight_rows=self.sql(f"SELECT fight_id,day,chapter_id,stamina,settled,HEX(start_snapshot) FROM daily_copy_fights WHERE player_id={owner.playerId} ORDER BY day;").strip().splitlines()
                    self.assertEqual(len(fight_rows),2)
                    for raw,start,day in zip(fight_rows,[first,second],[day1,day2]):
                        fields=raw.split('\t')
                        self.assertEqual(fields[:5],[start.fightId,day,'1','10','0'])
                        self.assertEqual(CLASSES['SCDailyCopyFightStart'].FromString(bytes.fromhex(fields[5])),start)
                        report['starts'].append(dict(fightId=start.fightId,day=day,chapter='1',actualStamina=10,used=1,settled=False))
                    # Frozen random drops exist only in the owned fight rows;
                    # no result/reward is posted or projected into inventory.
                    final=self.profile(owner)
                    for key in ['items','gems','cooking','equipment','treasures','warriors']:
                        self.assertEqual(final[1].get(key),initial[1].get(key))
                    self.assertEqual((final[0].get('coin',0),final[0].get('diamond',0)),
                                     (initial[0].get('coin',0),initial[0].get('diamond',0)))
                    self.stop();self.start();owner=self.login(uid)
                    self.assertEqual(self.profile(owner),final)
                    for saved in [first_stamp,resume_stamp]:
                        self.assertEqual(self.call('CSDailyCopyFightStart',{'ChapterId':'1'},owner,saved,'SCDailyCopyFightStart'),first)
                        self.assertEqual(self.profile(owner),final)
                    self.assertEqual(self.profile(protected),protected_before)
                    report.update(status='PASS',fullChain=True,weekdayTypesChecked=112,weekdayDifficultiesChecked=336,
                        originalDailyLimits={k:int(v['Count']) for k,v in rules.items()},
                        crossModeActiveRejected=True,newPersonalDayAllowsNewFight=True,oldPendingRestored=True,
                        coldFrozenStartExact=True,eightSameStampChargeOnce=True,protected1Exact=True,
                        cleanupScope='Two unsettled fights belong only to disposable allocation; no End/win fabricated')
                except BaseException as error:
                    report.update(status='FAIL',failureType=type(error).__name__,failure=str(error))
                    raise
                finally:
                    self.stop();self.pool.close()
                    report.update(seconds=round(time.monotonic()-began,3),ownedManagedAllocationsRemaining=len(self.pool.connections))
                    output.parent.mkdir(parents=True,exist_ok=True)
                    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__': unittest.main()
