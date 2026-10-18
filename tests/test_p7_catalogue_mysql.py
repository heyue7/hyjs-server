"""Earned D-to-C original frames, frozen higher-grade offers and cold receipts."""
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


@unittest.skipUnless(os.environ.get('HYJS_P7_CATALOGUE_MYSQL_TEST')=='1','Explicit isolated catalogue test')
class ExpeditionCatalogueMySQLTests(p7.P7MySQLTests):
    start=shared.TalentScopeMySQLTests.start

    @classmethod
    def setUpClass(cls):
        cls.managed=ManagedMySQL();cls.addClassCleanup(cls.managed.close)
        cls.connection=cls.managed.allocate('p7_catalogue')
        cls.database=cls.connection.database;cls.test_user=cls.connection.user;cls.test_password=cls.connection.password
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-p7-catalogue-');cls.addClassCleanup(cls.temp.cleanup)
        cls.binary=str(Path(cls.temp.name)/'server');cls.test_binary=str(Path(cls.temp.name)/'service.test')
        base=Path(os.environ['HYJS_P7_CATALOGUE_BASE']).resolve()
        if (p7.ROOT/'build/candidates').resolve() not in base.parents:raise AssertionError('Use a fixed local candidate')
        cls.frozen_source=Path(cls.temp.name)/'source';shutil.copytree(base,cls.frozen_source,ignore=shutil.ignore_patterns('logs'))
        (Path(cls.temp.name)/'analysis').symlink_to(p7.ROOT/'analysis',target_is_directory=True)
        cls.clock=Path(cls.temp.name)/'clock'
        path=cls.frozen_source/'main.go';main=path.read_text();marker='defer cancel()\n'
        if main.count(marker)!=1:raise AssertionError('Unexpected bounded clock injection point')
        main=main.replace(marker,marker+'''\n
    // A private test process preserves original durations using historical time.
    if len(os.Getenv("HYJS_MYSQL_DATABASE")) < 13 || os.Getenv("HYJS_MYSQL_DATABASE")[:13] != "hyjs_test_p7_" { return errors.New("catalogue clock isolation guard") }
    clockRaw, err := os.ReadFile(os.Getenv("HYJS_P7_CATALOGUE_CLOCK"))
    if err != nil { return err }
    clockAt, err := strconv.ParseInt(string(clockRaw),10,64)
    if err != nil || clockAt > time.Now().UnixMilli() { return errors.New("catalogue requires historical bounded clock") }
    ctx = context.WithValue(ctx,service.P7ExpeditionClockContextKey{},time.UnixMilli(clockAt))
''',1);path.write_text(main)
        subprocess.run(['rtk','proxy','gofmt','-w',str(path)],check=True,capture_output=True)
        cls.source_hashes={str(p.relative_to(cls.frozen_source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(cls.frozen_source.rglob('*.go'))}
        for args in (['go','build','-o',cls.binary,'.'],['go','test','-race','-c','-o',cls.test_binary,'./service']):
            result=subprocess.run(['rtk','proxy']+args,cwd=cls.frozen_source,env=dict(os.environ,GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB'),capture_output=True,timeout=180)
            if result.returncode:raise AssertionError((result.stdout+result.stderr).decode(errors='replace')[-4000:])
        cls.env=dict(os.environ,**cls.connection.env())
        cls.env.update(GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB',HYJS_P7_CATALOGUE_CLOCK=str(cls.clock))
        cls.env.update({'HYJS_P7_MYSQL_'+key:cls.env['HYJS_MYSQL_'+key] for key in ['HOST','USER','PASSWORD','DATABASE']})
        cls.lock=open('/tmp/hyjs-isolated-mysql.lock','a');fcntl.flock(cls.lock,fcntl.LOCK_EX);cls.addClassCleanup(cls.lock.close)
        def cleanup():
            cls.managed.release(cls.database)
            if getattr(cls,'report',None):
                cls.report['cleanupManagedAllocationReleased']=True
                cls.report['cleanupMethod']='restricted manage.py test-drop, owned allocation only'
                (p7.ROOT/'analysis/p7/catalogue-mysql-validation.json').write_text(json.dumps(cls.report,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    def domain(self,pattern):
        result=subprocess.run(['rtk','proxy',self.test_binary,'-test.run='+pattern,'-test.count=1','-test.timeout=2m'],cwd=self.frozen_source/'service',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]')[-4000:]);return result.stdout.strip()

    def test_original_earned_c_three_offers_frozen_final_and_cold(self):
        began=time.monotonic();domain=self.domain('^TestP7ExpeditionCatalogue')
        self.domain('^TestP7ExpeditionProtocolSeed$')
        at=int(time.time()*1000)-26*3600000;self.clock.write_text(str(at));self.start();uid='p7-native-expedition';login=self.login(uid)
        self.sql('UPDATE p7_expedition_catalogue_policies SET enabled=1;')
        inter=self.call('CSInterExpedition',{},login,1,'SCInterExpedition');self.assertEqual(inter.level,'D')
        self.sql(f'UPDATE p7_expedition_states SET exp=1199 WHERE player_id={login.playerId};')
        offers=self.call('CSGetRefreshedMissions',{},login,2,'SCGetRefreshedMissions').refreshedMissions;self.assertEqual(len(offers),2)
        first=offers[0];guid=self.call('CSGetExpeditionIdleHeros',{'missionSerial':first.missionSerial},login,3,'SCGetExpeditionIdleHeros').canSendGuids[0]
        self.call('CSAcceptMission',{'missionSerial':first.missionSerial,'heroGuids':[guid]},login,4,'SCAcceptMission')
        quote=json.loads(self.sql(f"SELECT quote FROM p7_expedition_missions WHERE serial='{first.missionSerial}';").strip())
        self.stop();self.clock.write_text(str(at+13*3600000));self.start();login=self.login(uid)
        req={'missionSerial':first.missionSerial}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:results=list(pool.map(lambda _:self.call('CSGetFinalReward',req,login,5,'SCGetFinalReward'),range(8)))
        self.assertTrue(all(x==results[0] for x in results));self.assertEqual((results[0].level,results[0].exp),('C',quote['exp']-1))
        c=self.call('CSRefreshMissionList',{},login,6,'SCRefreshMissionList');self.assertEqual(len(c.refreshedMissions),3)
        missions={r['missionId']:r for r in shared.original_table('ExpeditionMission')}
        self.assertTrue(all(missions[x.missionId]['level']=='C' for x in c.refreshedMissions));self.assertEqual(len(set(x.missionId for x in c.refreshedMissions)),3)
        chosen=c.refreshedMissions[0];self.call('CSAcceptMission',{'missionSerial':chosen.missionSerial,'heroGuids':[guid]},login,7,'SCAcceptMission')
        frozen=json.loads(self.sql(f"SELECT quote FROM p7_expedition_missions WHERE serial='{chosen.missionSerial}';").strip());self.assertEqual((frozen['grade'],frozen['exp'],frozen['maxHeroes']),('C',frozen['hours']*20,1))
        before=self.profile(login)
        self.sql("UPDATE p7_expedition_catalogue_policies SET enabled=0,source='closed future'; UPDATE p7_expedition_policies SET enabled=0,reward='diamond,999';")
        self.stop();self.clock.write_text(str(at+26*3600000-1000));self.start();login=self.login(uid)
        req={'missionSerial':chosen.missionSerial}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:results=list(pool.map(lambda _:self.call('CSGetFinalReward',req,login,8,'SCGetFinalReward'),range(8)))
        self.assertTrue(all(x==results[0] for x in results));self.assertEqual([(r.id,r.num) for r in getattr(results[0],'get').simps],[('item_001',1)])
        self.assertEqual(results[0].exp,quote['exp']-1+frozen['exp'])
        after=self.profile(login);self.assertEqual(after[0]['diamond'],before[0]['diamond'])
        self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSGetFinalReward',req,login,8,'SCGetFinalReward'),results[0]);self.assertEqual(self.profile(login),after)
        self.assertEqual(self.stamped('CSRefreshMissionList',{},login,9)[0],10001)
        self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={login.playerId} AND kind='expedition_complete';").strip(),'2')
        type(self).report={'status':'passed','elapsedSeconds':round(time.monotonic()-began,3),'goMySQLRace':domain,'domainRows':90,'originalSSGate':True,'actualTransition':'D1199+committed mission EXP -> C','cOffers':3,'8ClaimsEach':'one persistent final and reward','frozenSource':str(self.frozen_source),'sourceHashes':self.source_hashes,'cold':'original quote/result/inventory retained','clockFixture':'Historical two13h windows preserve4/8/12h duration; no real4h wait claimed','finitePrerequisites':'Existing protocol seed32level/2partners/100energy/1000diamond;1199 prior expeditionEXP; no formal data touched','authoritative':False}

    test_1_isolated_transactions_real_profile_compute_and_gate=None
    test_2_native_rush_catalog_and_atomic_gate=None
    test_3_original_frozen_playback_reconnect_and_no_star_reward=None
    test_4_native_expedition_duration_occupancy_claim_and_restart=None
    test_5_native_awake_original_catalogue_no_fake_exploration=None


if __name__=='__main__':unittest.main()
