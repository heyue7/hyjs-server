"""P6 actual original protocol / InnoDB tests, strictly dedicated hyjs_test_p6_* schemas."""
import concurrent.futures
import http.client
import hashlib
import gzip
import json
import os
import re
import selectors
import time
import types
from pathlib import Path
import subprocess
import tempfile
import unittest
import uuid
from google.protobuf.json_format import MessageToDict
from server.codec import BY_NAME, decode_responses, encode_http_request, message
import test_go_equipment
import test_go_formation
import test_go_warrior_growth
import test_mysql_equipment

@unittest.skipUnless(os.environ.get('HYJS_MYSQL_P6_TEST') == '1', 'isolated P6 MySQL regression disabled')
class P6MySQLTests(unittest.TestCase):
    root_sql = test_go_warrior_growth.MySQLWarriorGrowthTests.root_sql
    stop = test_go_equipment.GoEquipmentTests.stop
    login = test_go_equipment.GoEquipmentTests.login
    stamped = test_go_formation.GoFormationTests.stamped
    mutate = test_go_warrior_growth.MySQLWarriorGrowthTests.mutate

    def send(self, name, fields=None, login=None):
        # Concurrent development suites share CPU; keep a bounded protocol deadline.
        args = {'zone_id': 1, 'timestamp': 1700000000000}
        if login:
            args.update(player_id=login.playerId, session=bytes(login.sessionKey))
        raw = encode_http_request(BY_NAME[name], message(name, **(fields or {})), **args)
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=20)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            data = response.read()
            self.assertEqual(response.status, 200, data)
            self.last_frames = decode_responses(data)
            return self.last_frames[0]
        finally:
            connection.close()

    @classmethod
    def setUpClass(cls):
        cls.workspace = Path(__file__).resolve().parents[1]
        cls.repo = Path(os.environ.get('HYJS_P6_SOURCE', str(cls.workspace))).resolve()
        if cls.repo != cls.workspace and (not cls.repo.is_relative_to(cls.workspace/'build/candidates') or not (cls.repo/'manifest.json').is_file()):
            raise ValueError('P6 requires a fixed workspace candidate')
        # The InnoDB service test initializes its own schema and never needs an HTTP binary.
        cls.direct = os.environ.get('HYJS_MYSQL_P6_DIRECT') == '1'
        if cls.direct:
            return
        cls.temporary = tempfile.TemporaryDirectory(prefix='hyjs-p6-binary-')
        cls.binary = str(Path(cls.temporary.name)/'server')
        result = subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=cls.repo/'hyjs-server',capture_output=True,text=True,timeout=120)
        if result.returncode:
            cls.temporary.cleanup()
            raise AssertionError(result.stderr)
        cls.old_binary = os.environ.get('HYJS_GO_TEST_BINARY')
        os.environ['HYJS_GO_TEST_BINARY'] = cls.binary

    @classmethod
    def tearDownClass(cls):
        if cls.direct:
            return
        if cls.old_binary is None: os.environ.pop('HYJS_GO_TEST_BINARY',None)
        else: os.environ['HYJS_GO_TEST_BINARY']=cls.old_binary
        cls.temporary.cleanup()

    def setUp(self):
        if self.direct:
            self.assertIn(self._testMethodName, ('test_innodb_six_concurrent_whole_computations_and_resource_rollback', 'test_innodb_actual_growth_full_matrix', 'test_innodb_unused_victory_transport', 'test_innodb_actual_training_full_matrix', 'test_innodb_actual_breach_full_matrix', 'test_innodb_actual_combined_full_matrix', 'test_innodb_kf_global_projection_transport'))
        self.database='hyjs_test_p6_'+str(os.getpid())+'_'+uuid.uuid4().hex[:8]
        env=dict(os.environ)
        for line in (self.workspace/'.local-env/mysql.env').read_text().splitlines():
            if line and not line.startswith('#') and '=' in line:
                key,value=line.split('=',1);env[key]=value.strip().strip('"').strip("'")
        self.env=dict(env,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER='hyjs',HYJS_MYSQL_DATABASE=self.database)
        self.addCleanup(self.cleanup_database)
        self.root_sql('CREATE DATABASE `'+self.database+'` CHARACTER SET utf8mb4; GRANT ALL ON `'+self.database+"`.* TO 'hyjs'@'%';")
        self.addCleanup(self.stop)
        if not self.direct:
            self.start()

    def start(self):
        self.process=subprocess.Popen([self.binary,'--http-port','0','--tcp-port','0'],cwd=self.repo/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        deadline=time.monotonic()+60
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ)
            while time.monotonic()<deadline:
                if ready.select(min(5,max(0,deadline-time.monotonic()))):
                    line=self.process.stdout.readline()
                    matched=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line)
                    self.assertIsNotNone(matched,line[-1000:])
                    self.service=types.SimpleNamespace(http_port=int(matched[1]),tcp_port=int(matched[2]))
                    return
                if self.process.poll() is not None:
                    self.fail('P6 temporary server exited before readiness')
        self.fail('P6 temporary server readiness exceeded 60 seconds')

    def cleanup_database(self):
        self.assertTrue(self.database.startswith('hyjs_test_p6_'))
        self.root_sql('REVOKE ALL PRIVILEGES ON `'+self.database+"`.* FROM 'hyjs'@'%'; DROP DATABASE IF EXISTS `"+self.database+'`;')
        output=self.root_sql("SELECT COUNT(*) AS remaining FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='"+self.database+"'; SELECT COUNT(*) AS remaining FROM mysql.db WHERE Db='"+self.database+"';")
        self.assertEqual(output.splitlines(),['remaining','0','remaining','0'])

    def sql(self,text,args=()):
        self.assertTrue(self.database.startswith('hyjs_test_p6_'))
        parts=text.split('?');self.assertEqual(len(parts),len(args)+1);rendered=parts[0]
        for value,tail in zip(args,parts[1:]):
            rendered += str(value) if isinstance(value,(int,float)) else 'NULL' if value is None else "CONVERT(X'"+str(value).encode().hex()+"' USING utf8mb4)"
            rendered += tail
        lines=self.root_sql(rendered+';',self.database).splitlines()
        if not lines:return []
        return [dict(zip(lines[0].split('\t'),line.split('\t'))) for line in lines[1:]]

    def profile(self,login):
        row=self.sql('SELECT player,game FROM profiles WHERE id=?',(login.playerId,))[0]
        return json.loads(row['player']),json.loads(row['game'])


    def fixture(self,uid='p6-native'):
        login=self.login(uid)
        self.send('CSPlayerCreate',{'playerName':'录像隔离回归','heroId':'hero_047'},login)
        self.mutate(login,lambda p,g:p.update(level=10,stamina=200,noviceGuideSchedule=7))
        return login

    def call(self,name,fields,login,expected):
        opcode,response=self.send(name,fields,login)
        self.assertEqual(opcode,BY_NAME[expected],str(response))
        return response

    def begin(self,login):
        entry=self.call('CSCopyEnter',{'chapterId':'1','missionId':'1'},login,'SCCopyEnter')
        return self.call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':entry.jobList[0].taskId},login,'SCCopyFightStart')

    def records(self,login):
        rows = self.sql('SELECT id,status,authoritative,input_hash,configuration_hash,HEX(input) AS frozen_hex FROM p6_battle_records WHERE player_id=?',(login.playerId,))
        for row in rows:
            raw = bytes.fromhex(row.pop('frozen_hex'))
            if raw.startswith(b'\x1f\x8b'):
                raw = gzip.decompress(raw)
            self.assertEqual(hashlib.sha256(raw).hexdigest(), row['input_hash'])
            row['frozen'] = raw.decode()
        return rows

    def finish_observed(self,login,start):
        # This tests the existing P0 acceptance/recording path; the submitted winner
        # is explicitly observed, never a claim of server-computed battle parity.
        player=start.battlePlayer
        hp=sum(h.inst.curHp for h in player.formation)
        operate={'finalRoundNum':1,'attackerTotalLeftHp':hp,'defenderTotalLeftHp':0,
                 'skillOpts':[{'roundNum':1,'roundTime':0,'camp':0,'actorUid':'0_'+player.formation[0].guid,'skillIndex':0}]}
        return self.call('CSCopyFightEnd',{'fightId':start.fightId,'result':1,'battlePlayer':player,'optRecord':operate},login,'SCCopyFightEnd')

    def test_native_protocol_capture_observed_query_restart_concurrent_retry(self):
        login=self.fixture();start=self.begin(login)
        rows=self.records(login);self.assertEqual(len(rows),1)
        row=rows[0];self.assertEqual((row['status'],row['authoritative']),('captured','0'))
        frozen=json.loads(row['frozen']);self.assertEqual(int(frozen['seed']),start.randSeed)
        self.assertEqual(frozen['attacker']['formation'][0]['guid'],start.battlePlayer.formation[0].guid)
        self.assertEqual(frozen['monster']['formation'][0]['id'],start.battleMonsterList[0].formation[0].id)
        self.assertEqual(self.send('CSFightPlayback',{'fightId':start.fightId,'serverId':1},login)[0],10001)
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            ends=list(pool.map(lambda _:self.finish_observed(login,start),range(4)))
        self.assertTrue(all(end==ends[0] for end in ends))
        row=self.records(login)[0];self.assertEqual((row['status'],row['authoritative']),('observed','0'))
        before=self.profile(login)[0]['coin']
        playback=self.call('CSFightPlayback',{'fightId':start.fightId,'serverId':1},login,'SCFightPlayback')
        self.assertEqual(playback.attacker,start.battlePlayer)
        self.assertEqual(playback.defender2,start.battleMonsterList[0])
        self.assertEqual((playback.randSeed,playback.fightid,playback.clientResult,playback.serverResult),(start.randSeed,row['id'],0,0))
        self.assertTrue(playback.optRecord.skillOpts)
        self.assertEqual(self.profile(login)[0]['coin'],before)
        self.stop();self.start();login=self.login('p6-native')
        self.assertEqual(self.call('CSFightPlayback',{'fightId':row['id'],'serverId':1},login,'SCFightPlayback'),playback)
        outsider=self.fixture('p6-other')
        self.assertEqual(self.send('CSFightPlayback',{'fightId':row['id'],'serverId':1},outsider)[0],10001)
        self.sql('UPDATE p6_battle_records SET input=? WHERE id=?',('{}',row['id']))
        self.assertEqual(self.send('CSFightPlayback',{'fightId':row['id'],'serverId':1},login)[0],10001)

    def test_native_cancel_and_foreign_fight_preserve_wallet(self):
        login=self.fixture();start=self.begin(login);row=self.records(login)[0]
        before=self.profile(login)[0]['coin']
        self.call('CSCopyLeave',{},login,'SCCopyLeave')
        self.assertEqual(self.records(login)[0]['status'],'cancelled')
        self.assertEqual(self.send('CSFightPlayback',{'fightId':row['id'],'serverId':1},login)[0],10001)
        self.assertEqual(self.send('CSFightPlayback',{'fightId':'x'*129,'serverId':1},login)[0],10001)
        self.assertEqual(self.profile(login)[0]['coin'],before)
        self.assertEqual(self.sql('SELECT COUNT(*) AS n FROM p6_battle_records WHERE source_fight=?',(start.fightId,))[0]['n'],'1')

    def test_completed_v2_v3_snapshot_playback_after_process_restart(self):
        login = self.fixture('p6-legacy-version')
        start = self.begin(login)
        self.finish_observed(login, start)
        row = self.records(login)[0]
        expected = self.call('CSFightPlayback', {'fightId': row['id'], 'serverId': 1}, login, 'SCFightPlayback')
        # Historical v2/v3 share the schema. Freeze versioned fixtures; never re-compute them.
        frozen = json.loads(row['frozen'])
        for version in ('apk-10056262-p6-v2', 'apk-10056262-p6-v3'):
            frozen['engineVersion'] = version
            raw = json.dumps(frozen, separators=(',', ':'), ensure_ascii=False).encode()
            self.sql('UPDATE p6_battle_records SET engine_version=?,input=?,input_hash=? WHERE id=?',
                     (version, raw.decode(), hashlib.sha256(raw).hexdigest(), row['id']))
            self.stop()
            self.start()
            login = self.login('p6-legacy-version')
            restored = self.call('CSFightPlayback', {'fightId': start.fightId, 'serverId': 1}, login, 'SCFightPlayback')
            self.assertEqual(restored, expected)

    def test_innodb_six_concurrent_whole_computations_and_resource_rollback(self):
        self.stop()
        env=dict(self.env)
        env['HYJS_MYSQL_P6_DSN']=env['HYJS_MYSQL_USER']+':'+env['HYJS_MYSQL_PASSWORD']+'@tcp('+env['HYJS_MYSQL_HOST']+')/'+self.database+'?parseTime=true&charset=utf8mb4'
        # Large native configuration matrices can share CPU with parallel phase acceptance.
        compute_timeout=int(os.environ.get('HYJS_P6_MYSQL_COMPUTE_TIMEOUT_SECONDS','240' if self.direct else '120'))
        self.assertGreaterEqual(compute_timeout,120)
        self.assertLessEqual(compute_timeout,600)
        result=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP6MySQLConcurrentComputeAndAtomicResources$','-count=1'],cwd=self.repo/'hyjs-server',env=env,capture_output=True,text=True,timeout=compute_timeout)
        self.assertEqual(result.returncode,0,result.stdout+result.stderr)
        record=self.sql("SELECT status,authoritative FROM p6_battle_records WHERE source='isolated' AND source_fight='concurrent'")
        self.assertEqual(record,[{'status':'computed','authoritative':'0'}])
        self.assertEqual(self.sql("SELECT COUNT(*) AS n FROM p6_battle_records WHERE source_fight='rollback'")[0]['n'],'0')

    @unittest.skipUnless(os.environ.get('HYJS_P6_ACTUAL_GROWTH_TEST') == '1', 'actual P5 growth matrix disabled')
    def test_innodb_actual_growth_full_matrix(self):
        self.stop()
        env = dict(self.env)
        env['HYJS_MYSQL_P6_DSN'] = env['HYJS_MYSQL_USER'] + ':' + env['HYJS_MYSQL_PASSWORD'] + '@tcp(' + env['HYJS_MYSQL_HOST'] + ')/' + self.database + '?parseTime=true&charset=utf8mb4'
        result = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP6MySQLActualGrowthFullMatrix$', '-count=1', '-timeout=600s'], cwd=self.repo/'hyjs-server', env=env, capture_output=True, text=True, timeout=600)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        records = self.sql("SELECT status,authoritative,COUNT(*) AS n FROM p6_battle_records WHERE source='p6_actual_growth' GROUP BY status,authoritative")
        self.assertEqual(records, [{'status': 'computed', 'authoritative': '0', 'n': '182'}])

    def test_innodb_unused_victory_transport(self):
        self.stop()
        env = dict(self.env)
        env['HYJS_MYSQL_P6_DSN'] = env['HYJS_MYSQL_USER'] + ':' + env['HYJS_MYSQL_PASSWORD'] + '@tcp(' + env['HYJS_MYSQL_HOST'] + ')/' + self.database + '?parseTime=true&charset=utf8mb4'
        result = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP6MySQLUnusedWinConditionColdOriginalTransport$', '-count=1', '-timeout=240s'], cwd=self.repo/'hyjs-server', env=env, capture_output=True, text=True, timeout=240)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        records = self.sql("SELECT status,authoritative,COUNT(*) AS n FROM p6_battle_records WHERE source='p6_unused_victory' GROUP BY status,authoritative")
        self.assertEqual(records, [{'status': 'computed', 'authoritative': '0', 'n': '3'}])

    @unittest.skipUnless(os.environ.get('HYJS_P6_ACTUAL_TRAINING_TEST') == '1', 'actual P5 training stage disabled')
    def test_innodb_actual_training_full_matrix(self):
        self.stop()
        env = dict(self.env)
        env['HYJS_P6_ACTUAL_GROWTH_TEST'] = '1'
        env['HYJS_MYSQL_P6_DSN'] = env['HYJS_MYSQL_USER'] + ':' + env['HYJS_MYSQL_PASSWORD'] + '@tcp(' + env['HYJS_MYSQL_HOST'] + ')/' + self.database + '?parseTime=true&charset=utf8mb4'
        result = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP6MySQLActualGrowthFullMatrix$', '-count=1', '-timeout=600s'], cwd=self.repo/'hyjs-server', env=env, capture_output=True, text=True, timeout=600)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        records = self.sql("SELECT status,authoritative,COUNT(*) AS n FROM p6_battle_records WHERE source='p6_actual_training' GROUP BY status,authoritative")
        self.assertEqual(records, [{'status': 'computed', 'authoritative': '0', 'n': '152'}])

    def test_innodb_kf_global_projection_transport(self):
        self.stop()
        env = dict(self.env)
        env['HYJS_MYSQL_P6_DSN'] = env['HYJS_MYSQL_USER'] + ':' + env['HYJS_MYSQL_PASSWORD'] + '@tcp(' + env['HYJS_MYSQL_HOST'] + ')/' + self.database + '?parseTime=true&charset=utf8mb4'
        result = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP6MySQLKFGlobalProjectionColdTransportAndMetadataRetry$', '-count=1', '-timeout=240s'], cwd=self.repo/'hyjs-server', env=env, capture_output=True, text=True, timeout=240)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        records = self.sql("SELECT status,authoritative,COUNT(*) AS n FROM p6_battle_records WHERE source='p6_kf_projection' GROUP BY status,authoritative")
        self.assertEqual(records, [{'status': 'computed', 'authoritative': '0', 'n': '3'}])

    @unittest.skipUnless(os.environ.get('HYJS_P6_ACTUAL_COMBINED_TEST') == '1', 'actual P5 consecutive highest combined stage disabled')
    def test_innodb_actual_combined_full_matrix(self):
        self.stop()
        env = dict(self.env)
        env['HYJS_P6_ACTUAL_GROWTH_TEST'] = '1'
        env['HYJS_P6_ACTUAL_TRAINING_TEST'] = '0'
        env['HYJS_P6_ACTUAL_BREACH_TEST'] = '0'
        env['HYJS_MYSQL_P6_DSN'] = env['HYJS_MYSQL_USER'] + ':' + env['HYJS_MYSQL_PASSWORD'] + '@tcp(' + env['HYJS_MYSQL_HOST'] + ')/' + self.database + '?parseTime=true&charset=utf8mb4'
        result = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP6MySQLActualGrowthFullMatrix$', '-count=1', '-timeout=600s'], cwd=self.repo/'hyjs-server', env=env, capture_output=True, text=True, timeout=600)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        records = self.sql("SELECT status,authoritative,COUNT(*) AS n FROM p6_battle_records WHERE source='p6_actual_combined' GROUP BY status,authoritative")
        self.assertEqual(records, [{'status': 'computed', 'authoritative': '0', 'n': '152' if os.environ.get('HYJS_P6_ACTUAL_COMBINED_SCOPE') == 'all' else '38'}])

    @unittest.skipUnless(os.environ.get('HYJS_P6_ACTUAL_BREACH_TEST') == '1', 'actual P5 breach25 stage disabled')
    def test_innodb_actual_breach_full_matrix(self):
        self.stop()
        env = dict(self.env)
        env['HYJS_P6_ACTUAL_GROWTH_TEST'] = '1'
        env['HYJS_P6_ACTUAL_TRAINING_TEST'] = '0'
        env['HYJS_MYSQL_P6_DSN'] = env['HYJS_MYSQL_USER'] + ':' + env['HYJS_MYSQL_PASSWORD'] + '@tcp(' + env['HYJS_MYSQL_HOST'] + ')/' + self.database + '?parseTime=true&charset=utf8mb4'
        result = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP6MySQLActualGrowthFullMatrix$', '-count=1', '-timeout=600s'], cwd=self.repo/'hyjs-server', env=env, capture_output=True, text=True, timeout=600)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        records = self.sql("SELECT status,authoritative,COUNT(*) AS n FROM p6_battle_records WHERE source='p6_actual_breach' GROUP BY status,authoritative")
        self.assertEqual(records, [{'status': 'computed', 'authoritative': '0', 'n': '152'}])
