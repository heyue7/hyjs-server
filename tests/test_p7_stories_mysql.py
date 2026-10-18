"""Original matching story frames, lucky rewards, cold frozen receipts."""
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

from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME
import test_p7_mysql as p7
import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_P7_STORIES_MYSQL_TEST')=='1','Explicit isolated stories test')
class ExpeditionStoriesMySQLTests(p7.P7MySQLTests):
    start=shared.TalentScopeMySQLTests.start

    @classmethod
    def setUpClass(cls):
        cls.managed=ManagedMySQL();cls.addClassCleanup(cls.managed.close)
        cls.connection=cls.managed.allocate('p7_stories')
        cls.database=cls.connection.database;cls.test_user=cls.connection.user;cls.test_password=cls.connection.password
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-p7-stories-');cls.addClassCleanup(cls.temp.cleanup)
        cls.binary=str(Path(cls.temp.name)/'server');cls.test_binary=str(Path(cls.temp.name)/'service.test')
        base=Path(os.environ['HYJS_P7_STORIES_BASE']).resolve()
        if (p7.ROOT/'build/candidates').resolve() not in base.parents:raise AssertionError('Use a fixed local story candidate')
        cls.frozen_source=Path(cls.temp.name)/'source';shutil.copytree(base,cls.frozen_source,ignore=shutil.ignore_patterns('logs'))
        (Path(cls.temp.name)/'analysis').symlink_to(p7.ROOT/'analysis',target_is_directory=True)
        cls.clock=Path(cls.temp.name)/'clock'
        path=cls.frozen_source/'main.go';main=path.read_text();marker='defer cancel()\n'
        if main.count(marker)!=1:raise AssertionError('Unexpected bounded clock injection point')
        main=main.replace(marker,marker+'''\n
    // A private test process preserves original durations using historical time.
    if len(os.Getenv("HYJS_MYSQL_DATABASE")) < 13 || os.Getenv("HYJS_MYSQL_DATABASE")[:13] != "hyjs_test_p7_" { return errors.New("stories clock isolation guard") }
    clockRaw, err := os.ReadFile(os.Getenv("HYJS_P7_STORIES_CLOCK"))
    if err != nil { return err }
    clockAt, err := strconv.ParseInt(string(clockRaw),10,64)
    if err != nil || clockAt > time.Now().UnixMilli() { return errors.New("stories require historical bounded clock") }
    ctx = context.WithValue(ctx,service.P7ExpeditionClockContextKey{},time.UnixMilli(clockAt))
''',1);path.write_text(main)
        subprocess.run(['rtk','proxy','gofmt','-w',str(path)],check=True,capture_output=True)
        compiled_at=time.monotonic()
        cls.source_hashes={str(p.relative_to(cls.frozen_source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(cls.frozen_source.rglob('*.go'))}
        for args in (['go','build','-o',cls.binary,'.'],['go','test','-race','-c','-o',cls.test_binary,'./service']):
            result=subprocess.run(['rtk','proxy']+args,cwd=cls.frozen_source,env=dict(os.environ,GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB'),capture_output=True,timeout=180)
            if result.returncode:raise AssertionError((result.stdout+result.stderr).decode(errors='replace')[-4000:])
        cls.compile_seconds=round(time.monotonic()-compiled_at,3); print('Stories isolated server/race precompile complete',cls.compile_seconds,flush=True)
        cls.env=dict(os.environ,**cls.connection.env())
        cls.env.update(GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB',HYJS_P7_STORIES_CLOCK=str(cls.clock))
        cls.env.update({'HYJS_P7_MYSQL_'+key:cls.env['HYJS_MYSQL_'+key] for key in ['HOST','USER','PASSWORD','DATABASE']})
        lock_at=time.monotonic();cls.lock=open('/tmp/hyjs-isolated-mysql.lock','a');fcntl.flock(cls.lock,fcntl.LOCK_EX);cls.addClassCleanup(cls.lock.close);cls.lock_seconds=round(time.monotonic()-lock_at,3);print('Stories exclusive MySQL acquired',cls.lock_seconds,flush=True)
        def cleanup():
            cls.managed.release(cls.database)
            if getattr(cls,'report',None):
                cls.report['cleanupManagedAllocationReleased']=True
                cls.report['cleanupMethod']='restricted manage.py test-drop, owned allocation only'
                (p7.ROOT/'analysis/p7/stories-mysql-validation.json').write_text(json.dumps(cls.report,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    def domain(self,pattern):
        result=subprocess.run(['rtk','proxy',self.test_binary,'-test.run='+pattern,'-test.count=1','-test.timeout=2m'],cwd=self.frozen_source/'service',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]')[-4000:]);return result.stdout.strip()

    def test_original_matching_stories_lucky_frozen_cold_claim(self):
        began=time.monotonic();domain=self.domain('^TestP7ExpeditionStory')
        self.domain('^TestP7ExpeditionProtocolSeed$')
        uid='p7-native-expedition';at=int(time.time()*1000)-27*3600000
        self.clock.write_text(str(at));self.start();login=self.login(uid)
        runs=[]
        for case,lucky in enumerate((0,100)):
            current=at+case*14*3600000
            self.stop();self.clock.write_text(str(current));self.start();login=self.login(uid)
            self.sql(f"UPDATE p7_expedition_policies SET enabled=1,reward='item_005,3'; UPDATE p7_expedition_event_policies SET enabled=1,risk_percent=0; UPDATE p7_expedition_story_policies SET enabled=1,lucky_percent={lucky};")
            offers=self.call('CSGetRefreshedMissions' if case==0 else 'CSRefreshMissionList',{},login,100+case*20,'SCGetRefreshedMissions' if case==0 else 'SCRefreshMissionList').refreshedMissions
            chosen=offers[0];guid=self.call('CSGetExpeditionIdleHeros',{'missionSerial':chosen.missionSerial},login,101+case*20,'SCGetExpeditionIdleHeros').canSendGuids[0]
            self.call('CSAcceptMission',{'missionSerial':chosen.missionSerial,'heroGuids':[guid]},login,102+case*20,'SCAcceptMission')
            quote=json.loads(self.sql(f"SELECT quote FROM p7_expedition_missions WHERE serial='{chosen.missionSerial}';").strip())
            self.assertEqual(quote['events']['stories']['luckyPercent'],lucky)
            self.assertEqual(len(quote['events']['stories']['ordinary']),4);self.assertEqual(len(quote['events']['stories']['lucky']),1)
            before=self.profile(login);old=next((x['num'] for x in (before[1]['items'] or []) if x['id']=='item_005'),0)
            self.assertEqual(len(self.call('CSExpeditionMissionDetail',{'missionSerial':chosen.missionSerial},login,103+case*20,'SCExpeditionMissionDetail').events),0)
            self.sql("UPDATE p7_expedition_story_policies SET enabled=0,lucky_percent=50,source='closed future'; UPDATE p7_expedition_event_policies SET enabled=0,risk_percent=100; UPDATE p7_expedition_policies SET enabled=0,reward='diamond,999';")
            self.stop();self.clock.write_text(str(current+13*3600000));self.start();login=self.login(uid)
            detail=self.call('CSExpeditionMissionDetail',{'missionSerial':chosen.missionSerial},login,104+case*20,'SCExpeditionMissionDetail')
            self.assertEqual(len(detail.events),quote['hours']-1)
            rows={r['storyId']:r for r in shared.original_table('ExpeditionStory')}
            reward='item_005,'+('6' if lucky else '3')
            self.assertTrue(all(e.eventType==(1 if lucky else 0) and e.reward==reward and rows[e.storyId]['rewardId']=='item_005' for e in detail.events))
            req={'missionSerial':chosen.missionSerial}
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:results=list(pool.map(lambda _:self.call('CSGetFinalReward',req,login,105+case*20,'SCGetFinalReward'),range(8)))
            self.assertTrue(all(x==results[0] for x in results))
            expected=3+(6 if lucky else 3)*(quote['hours']-1)
            after=self.profile(login);got=next(x['num'] for x in after[1]['items'] if x['id']=='item_005')
            self.assertEqual(got-old,expected);self.assertEqual(after[0]['diamond'],before[0]['diamond'])
            self.stop();self.start();login=self.login(uid)
            self.assertEqual(self.call('CSGetFinalReward',req,login,105+case*20,'SCGetFinalReward'),results[0]);self.assertEqual(self.profile(login),after)
            self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p7_expedition_heroes WHERE player_id={login.playerId};").strip(),'0')
            runs.append({'luckyPercent':lucky,'hours':quote['hours'],'actualDeferredItem005':expected,'events':len(detail.events),'originalKind':1 if lucky else 0,'8ConcurrentClaims':'one persistent receipt/inventory delta','cold':'quote/items/receipt unchanged after policies disabled'})
        self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={login.playerId} AND kind='expedition_complete';").strip(),'2')
        # SQL publication defaults stay closed and initialization preserves operator choices.
        ddl=(self.frozen_source/'db/sql/114_p7_expedition_story_policies.sql').read_text()
        seed=(self.frozen_source/'db/sql/114_p7_expedition_story_policies.seed.sql').read_text()
        self.sql('DROP TABLE p7_expedition_story_policies;'+ddl+seed)
        self.assertEqual(self.sql('SELECT enabled,lucky_percent FROM p7_expedition_story_policies WHERE id=1;').split(),['0','20'])
        self.sql("UPDATE p7_expedition_story_policies SET lucky_percent=33,source='preserved operator source';")
        self.stop();self.start();self.assertEqual(self.sql('SELECT enabled,lucky_percent,source FROM p7_expedition_story_policies WHERE id=1;').strip(),'0\t33\tpreserved operator source')
        self.assertEqual(self.sql("SELECT engine FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='p7_expedition_story_policies';").strip(),'InnoDB')
        type(self).report={'status':'passed','elapsedSeconds':round(time.monotonic()-began,3),'goMySQLRace':domain,'compileSeconds':self.compile_seconds,'lockWaitSeconds':self.lock_seconds,'originalStoryRows':92,'matchingRewardKinds':19,'actualOriginalFrames':runs,'frozenSource':str(self.frozen_source),'sourceHashes':self.source_hashes,'SQL':'114 InnoDB/default off/operator preserved','clockFixture':'Historical two13h windows preserve original4/8/12h durations; no real4h wait','finitePrerequisites':'Existing isolated32level/2partners/100energy/1000diamond protocol seed; no expedition facts/resources injected; formal untouched','authoritative':False}

    test_1_isolated_transactions_real_profile_compute_and_gate=None
    test_2_native_rush_catalog_and_atomic_gate=None
    test_3_original_frozen_playback_reconnect_and_no_star_reward=None
    test_4_native_expedition_duration_occupancy_claim_and_restart=None
    test_5_native_awake_original_catalogue_no_fake_exploration=None


if __name__=='__main__':unittest.main()
