"""Original Rush public Start/End frames on one centrally managed temporary schema.

Only run during the root-coordinated SQL window. Local stage/finite level inputs
are published before capture; winners come from the independent P6 calculation.
No formal profile, native phone, global authority switch or guessed winner is used.
"""
import hashlib
import http.client
import json
import os
from pathlib import Path
import secrets
import selectors
import subprocess
import tempfile
import time
import types
import unittest

from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME,decode_responses,encode_http_request,message
import test_p10_mysql as transport

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P7_SCOPED_MYSQL_TEST')=='1','Root-coordinated scoped Rush SQL window required')
class P7ScopedRushManagedMySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    stop=transport.P10MySQLTests.stop
    setUp=transport.P10MySQLTests.setUp
    report={}

    @classmethod
    def setUpClass(cls):
        cls.source=Path(os.environ['HYJS_P7_SCOPED_BUILD_ROOT']).resolve()
        if cls.source.parent.parent!=ROOT/'build/candidates' or cls.source.name!='hyjs-server':raise AssertionError('Fixed candidate required')
        if hashlib.sha256((cls.source/'service/p7RushService.go').read_bytes()).hexdigest()!='2ad916e959b763449c44f407bf325a139b444f848fc14f16d49a8d77e84f6fce':raise AssertionError('Exact scoped P7 proposal required')
        # Build before allocating the serial SQL window. An optional already
        # built local binary allows root to perform this step outside the lease.
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-p7-scope-');cls.addClassCleanup(cls.temp.cleanup)
        cls.binary=os.environ.get('HYJS_P7_SCOPED_BINARY') or str(Path(cls.temp.name)/'server')
        build_env=dict(os.environ,GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB')
        if not os.environ.get('HYJS_P7_SCOPED_BINARY'):
            result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=cls.source,env=build_env,capture_output=True,text=True,timeout=180)
            if result.returncode:raise AssertionError(result.stdout+result.stderr)
        cls.managed=ManagedMySQL();cls.addClassCleanup(cls.managed.close)
        cls.connection=cls.managed.allocate('p7_scope')
        cls.env=dict(os.environ,**cls.connection.env());cls.env.update(GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB')
        cls.report={'scope':'Original typed9/10 Start/End public protocol on actual new isolated accounts and current P5 roster; finite local level22/stage prepublication; original automatic-equivalent full End record from independent server calculation, no phone claim','binarySHA256':hashlib.sha256(Path(cls.binary).read_bytes()).hexdigest(),'formalDatabaseReadOrWrite':False,'globalAuthorityChanged':False}

    def sql(self,text):return self.managed.sql(self.connection.database,text)

    def start(self):
        self.process=subprocess.Popen([self.binary,'--http-port','0','--tcp-port','0','--tables',str(ROOT/'analysis/tables')],cwd=self.source,env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        deadline=time.monotonic()+90
        while time.monotonic()<deadline:
            with selectors.DefaultSelector() as ready:
                ready.register(self.process.stdout,selectors.EVENT_READ)
                if not ready.select(1):
                    if self.process.poll() is not None:break
                    continue
                line=self.process.stdout.readline()
            if 'HYJS Go ready' in line:
                import re
                match=re.search(r'HTTP=(?:127\.0\.0\.1|\[::\]):(\d+) TCP=(?:127\.0\.0\.1|\[::\]):(\d+)',line)
                if match:self.service=types.SimpleNamespace(http_port=int(match[1]),tcp_port=int(match[2]));return
        raise AssertionError('Scoped Rush isolated server failed readiness')

    def frames(self,name,fields,login,stamp):
        body=encode_http_request(BY_NAME[name],message(name,**fields),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
        connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=30)
        try:
            connection.request('POST','/sao/',body=body,headers={'Content-Type':'application/octet-stream'})
            response=connection.getresponse();raw=response.read();self.assertEqual(response.status,200)
            return decode_responses(raw)
        finally:connection.close()

    def call(self,name,fields,login,stamp,expected):
        frames=self.frames(name,fields,login,stamp)
        self.assertEqual(frames[0][0],BY_NAME[expected],str(frames[0][1]))
        return frames[0][1]

    def profile(self,login):
        values=self.sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id='+str(login.playerId)+';').strip().split('|')
        self.assertEqual(len(values),2)
        return [json.loads(bytes.fromhex(x)) for x in values]

    def publish(self,kind):
        # Reuse the original already phone-compared NPC template. This is a
        # prepublished finite local stage, never a claim about private floors.
        raw=(ROOT/'analysis/p6/actual-local-p7-rush-inputs-v104/000.SCRushFightStart.pb').read_bytes()
        typed=message('SCRushFightStart');typed.ParseFromString(raw)
        enemy=typed.battleMonsterList[0].SerializeToString(deterministic=True).hex()
        stage='scoped-native-'+str(kind)
        floor,star=(1,1) if kind==1 else (0,0)
        self.sql("INSERT INTO p7_rush_stage_rules(id,kind,task_id,condition_task_id,floor,star,min_level,enabled,scene,max_round,win_key,win_value,enemies,first_reward,reward,recommended_power,source) VALUES('"+stage+"',"+str(kind)+",'"+stage+"','',"+str(floor)+","+str(star)+",22,1,'map_grass_001',40,0,0,0x"+enemy+",'','point,10',0,'prepublished finite local original047 stage; unknown private floor'); UPDATE p7_rush_policies SET enabled=1; UPDATE p7_rush_scoped_policies SET enabled=0;")
        return {'rushType':kind,'taskId':stage,'star':star},stage

    def test_original_public_rush_real_compute_once_cold_and_atomic_rejection(self):
        self.start()
        cases=[]
        for kind in [1,2]:
            uid='p7-scoped-native-'+secrets.token_hex(5);login=self.login(uid)
            self.call('CSPlayerCreate',{'playerName':'真实试炼协议'+str(kind),'heroId':'hero_047'},login,1,'SCPlayerCreate')
            self.sql("UPDATE profiles SET player=JSON_SET(player,'$.level',22) WHERE id="+str(login.playerId)+';')
            self.call('CSFormationList',{},login,2,'SCFormationList')
            args,stage=self.publish(kind)
            before=self.profile(login)
            self.assertEqual(self.frames('CSRushFightStart',args,login,10)[0][0],10001)
            self.assertEqual(self.profile(login),before)
            self.sql('UPDATE p7_rush_scoped_policies SET enabled=1;')
            start=self.call('CSRushFightStart',args,login,11,'SCRushFightStart')
            self.assertEqual(start.battlePlayer.playerId,login.playerId)
            self.assertEqual(start.battlePlayer.formation[0].guid,before[0]['heroWarId'])
            self.assertEqual(start.battlePlayer.formation[0].skillOrder,'047_1,047_2,047_3')
            self.assertEqual(start,self.call('CSRushFightStart',args,login,11,'SCRushFightStart'))
            self.assertEqual(self.frames('CSRushFightStart',args,login,12)[0][0],10001)
            row=self.sql("SELECT HEX(terminal) FROM p7_rush_scoped_calculations WHERE fight_id='"+start.fightId+"';").strip()
            terminal=json.loads(bytes.fromhex(row));self.assertTrue(terminal['outcome']['authoritative'])
            expected=int(terminal['outcome']['winner']==0)
            record=terminal['originalOperates']
            end={'fightId':start.fightId,'result':expected,'optRecord':record}
            held=self.profile(login)
            self.assertEqual(self.frames('CSRushFightEnd',dict(end,result=1-expected),login,20)[0][0],10001)
            self.assertEqual(self.profile(login),held)
            altered=json.loads(json.dumps(record));altered['skillOpts']=altered['skillOpts'][:-1]
            self.assertEqual(self.frames('CSRushFightEnd',dict(end,optRecord=altered),login,21)[0][0],10001)
            self.assertEqual(self.profile(login),held)
            self.assertEqual(self.sql("SELECT status FROM p7_rush_fights WHERE id='"+start.fightId+"';").strip(),'computed')
            self.sql("UPDATE p7_rush_scoped_policies SET enabled=0; UPDATE p7_rush_stage_rules SET reward='point,999999' WHERE id='"+stage+"';")
            frames=self.frames('CSRushFightEnd',end,login,22)
            self.assertEqual([x[0]for x in frames],[BY_NAME['SCRushFightEnd'],BY_NAME['SCPlayer']])
            receipt=frames[0][1];self.assertEqual(receipt.result,expected)
            settled=self.profile(login)
            self.assertEqual(settled[0].get('point',0),before[0].get('point',0)+(10 if expected else 0))
            self.assertEqual(settled[0].get('diamond',0),before[0].get('diamond',0))
            self.assertEqual(self.frames('CSRushFightEnd',end,login,23)[0][1],receipt)
            self.assertEqual(self.profile(login),settled)
            state=self.sql('SELECT CONCAT(floor,"|",total_stars,"|",elite_used,"|",active_fight) FROM p7_rush_states WHERE player_id='+str(login.playerId)+';').strip().split('|')
            self.assertEqual(state[3],'')
            if kind==1 and expected:self.assertEqual(state[:2],['2','1'])
            if kind==2:self.assertEqual(state[2],'1')
            self.stop();self.start();login=self.login(uid)
            self.assertEqual(self.frames('CSRushFightEnd',end,login,24)[0][1],receipt)
            self.assertEqual(self.profile(login),settled)
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p6_battle_records WHERE player_id='+str(login.playerId)+'; SELECT COUNT(*) FROM p7_rush_scoped_calculations WHERE player_id='+str(login.playerId)+';').strip().split(),['1','1'])
            self.assertEqual(self.frames('CSRushEliteBossBuyTime',{},login,25)[0][0],10001)
            cases.append({'kind':kind,'actualWinner':terminal['outcome']['winner'],'finalRound':terminal['outcome']['finalRound'],'completeOriginalActionCount':len(record['skillOpts']),'independentProof':terminal['outcome']['capabilityHash'],'nativeStartEndColdOnce':True,'globalGateStillFalse':True})
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.connection.database+"' AND TABLE_NAME IN ('profiles','p7_rush_scoped_calculations','p6_battle_records');").strip(),'InnoDB')
        self.report['cases']=cases
        self.stop()

    @classmethod
    def tearDownClass(cls):
        cls.managed.close();cls.report['managedSchemaAndAccountReleased']=True
        path=os.environ.get('HYJS_P7_SCOPED_REPORT')
        if path:Path(path).write_text(json.dumps(cls.report,indent=2)+'\n')


del transport
