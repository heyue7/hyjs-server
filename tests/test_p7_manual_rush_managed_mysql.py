"""Serial managed original HTTP Trial9 manual-to-Auto settlement acceptance.
The finite client-control record is independently calculated from this new paid
source; this suite does not claim physical phone UI or inject a result/winner.
"""
import gzip
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import unittest

from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME
import test_p7_scoped_rush_managed_mysql as prior

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P7_MANUAL_MYSQL_TEST')=='1','Root-coordinated manual Trial SQL window required')
class P7ManualRushManagedMySQLTests(unittest.TestCase):
    send=prior.P7ScopedRushManagedMySQLTests.send
    login=prior.P7ScopedRushManagedMySQLTests.login
    stop=prior.P7ScopedRushManagedMySQLTests.stop
    setUp=prior.P7ScopedRushManagedMySQLTests.setUp
    start=prior.P7ScopedRushManagedMySQLTests.start
    sql=prior.P7ScopedRushManagedMySQLTests.sql
    frames=prior.P7ScopedRushManagedMySQLTests.frames
    call=prior.P7ScopedRushManagedMySQLTests.call
    profile=prior.P7ScopedRushManagedMySQLTests.profile
    publish=prior.P7ScopedRushManagedMySQLTests.publish

    @classmethod
    def setUpClass(cls):
        cls.source=Path(os.environ['HYJS_P7_MANUAL_BUILD_ROOT']).resolve()
        if cls.source.parent.parent!=ROOT/'build/candidates' or cls.source.name!='hyjs-server':raise AssertionError('Fixed source required')
        expected={'service/p7ManualRushService.go': 'f4948c8fc5ed73af50a6bec44d3999730a8fcc84fc2bac021656323659db1733', 'service/p7ScopedRushService.go': '1a8d04afc6bcde779e2bb4997e493793a04fbfdacfff17f912bc53aad34cfd83', 'service/p7RushService.go': '0814882636772f02366afac8d16ccb6fd6979f41369711fbbe9f16342ce5c51a', 'service/p6RushManualSettlementService.go': '28b6723faa36c7ba28c2daaa26c1fdcc87ad68c31361f172a0f907bb043f0bad', 'constUtil/p6RushManualSettlement.go': '8b5dbaf621ecc68cc8078515091fe6d0c90f35f73422bcbd76048b3e997c449f'}
        for relative,sha in expected.items():
            if hashlib.sha256((cls.source/relative).read_bytes()).hexdigest()!=sha:raise AssertionError('Exact manual source changed '+relative)
        cls.binary=os.environ['HYJS_P7_MANUAL_BINARY']
        cls.client=os.environ['HYJS_P7_MANUAL_CLIENT_BINARY']
        if not Path(cls.binary).is_file() or not Path(cls.client).is_file():raise AssertionError('Precompile both binaries outside SQL window')
        cls.managed=ManagedMySQL();cls.addClassCleanup(cls.managed.close)
        cls.connection=cls.managed.allocate('p7_manual')
        cls.env=dict(os.environ,**cls.connection.env(),GOMEMLIMIT='768MiB',GOMAXPROCS='2')
        cls.report={'scope':'Original HTTP paid Trial9; finite new047/level22 source with independently generated live manual-to-Auto client-control record; no physical phone claim','binarySHA256':hashlib.sha256(Path(cls.binary).read_bytes()).hexdigest(),'sourcePins':expected,'formalReadOrWrite':False,'globalAuthorityChanged':False}

    def test_paid_trial_full_manual_end_atomic_once_cold(self):
        self.start()
        uid='p7-manual-'+secrets.token_hex(5);login=self.login(uid)
        self.call('CSPlayerCreate',{'playerName':'独立原试炼手动链','heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql("UPDATE profiles SET player=JSON_SET(player,'$.level',22) WHERE id="+str(login.playerId)+';')
        self.call('CSFormationList',{},login,2,'SCFormationList')
        args,stage=self.publish(1)
        self.sql('UPDATE p7_rush_scoped_policies SET enabled=1; UPDATE p7_rush_manual_policies SET enabled=1;')
        start=self.call('CSRushFightStart',args,login,10,'SCRushFightStart')
        self.assertEqual(start,self.call('CSRushFightStart',args,login,10,'SCRushFightStart'))
        row=self.sql("SELECT CONCAT(m.status,'|',b.status,'|',b.authoritative,'|',HEX(b.input),'|',b.input_hash) FROM p7_rush_manual_calculations m JOIN p6_battle_records b ON b.id=m.p6_record_id WHERE m.fight_id='"+start.fightId+"';").strip().split('|')
        self.assertEqual(row[:3],['0','captured','0'])
        input_bytes=bytes.fromhex(row[3]);raw=gzip.decompress(input_bytes) if input_bytes[:2]==b'\x1f\x8b' else input_bytes
        self.assertEqual(hashlib.sha256(raw).hexdigest(),row[4])
        held=self.profile(login)
        with tempfile.TemporaryDirectory(prefix='hyjs-p7-manual-client-') as private:
            source=Path(private)/'source.json';destination=Path(private)/'record.json'
            source.write_bytes(raw);source.chmod(0o600)
            result=subprocess.run([self.client,'-test.run=^TestP7ManualNativeClientRecord$','-test.count=1'],cwd=self.source/'service',env=dict(os.environ,GOMEMLIMIT='768MiB',GOMAXPROCS='2',HYJS_P7_MANUAL_CLIENT_INPUT=str(source),HYJS_P7_MANUAL_CLIENT_OUTPUT=str(destination)),capture_output=True,text=True,timeout=45)
            if result.returncode:raise AssertionError(self.managed.redact(result.stdout+result.stderr))
            record=json.loads(destination.read_text())
        self.assertEqual(record['skillOpts'][0]['roundTime'],30.100000000000158)
        expected=int(record.get('defenderTotalLeftHp',0)==0 and record.get('attackerTotalLeftHp',0)>0)
        end={'fightId':start.fightId,'result':expected,'optRecord':record}
        changed=json.loads(json.dumps(record));changed['skillOpts'][0]['roundTime']+=0.1
        for stamp,fields in [(20,dict(end,result=1-expected)),(21,dict(end,optRecord=changed))]:
            self.assertEqual(self.frames('CSRushFightEnd',fields,login,stamp)[0][0],10001)
            self.assertEqual(self.profile(login),held)
            self.assertEqual(self.sql("SELECT status FROM p7_rush_manual_calculations WHERE fight_id='"+start.fightId+"';").strip(),'0')
        self.sql("UPDATE p7_rush_manual_policies SET enabled=0; UPDATE p7_rush_scoped_policies SET enabled=0; UPDATE p7_rush_stage_rules SET reward='point,999999' WHERE id='"+stage+"';")
        frames=self.frames('CSRushFightEnd',end,login,22)
        self.assertEqual([x[0]for x in frames],[BY_NAME['SCRushFightEnd'],BY_NAME['SCPlayer']])
        receipt=frames[0][1];self.assertEqual(receipt.result,expected)
        accepted=self.profile(login)
        self.assertEqual(accepted[0].get('point',0),held[0].get('point',0)+(10 if expected else 0))
        final=self.sql("SELECT CONCAT(m.status,'|',b.status,'|',b.authoritative,'|',HEX(b.input),'|',CHAR_LENGTH(m.derived_calculation_source_sha256),'|',CHAR_LENGTH(m.end_request_hash)) FROM p7_rush_manual_calculations m JOIN p6_battle_records b ON b.id=m.p6_record_id WHERE m.fight_id='"+start.fightId+"';").strip().split('|')
        self.assertEqual(final[:3],['1','computed','1']);self.assertEqual(final[3],row[3]);self.assertEqual(final[4:],['64','64'])
        self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.frames('CSRushFightEnd',end,login,23)[0][1],receipt)
        self.assertEqual(self.profile(login),accepted)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p7_rush_manual_calculations; SELECT COUNT(*) FROM p7_rush_scoped_calculations;').strip().split(),['1','0'])
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.connection.database+"' AND TABLE_NAME IN ('profiles','p7_rush_manual_calculations','p6_battle_records');").strip(),'InnoDB')
        self.report.update(actualResult=expected,actualFinalRound=record['finalRoundNum'],operations=len(record['skillOpts']),manualFirstClock=record['skillOpts'][0]['roundTime'],paidFrozenSourceUnchanged=True,strictConflictRollbacks=True,coldOnce=True)
        self.stop()

    @classmethod
    def tearDownClass(cls):
        cls.managed.close();cls.report['managedSchemaAndAccountReleased']=True
        if os.environ.get('HYJS_P7_MANUAL_REPORT'):Path(os.environ['HYJS_P7_MANUAL_REPORT']).write_text(json.dumps(cls.report,indent=2)+'\n')
