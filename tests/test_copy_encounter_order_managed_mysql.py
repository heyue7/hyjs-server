"""Exact ordinary-copy collision delta on one managed temporary MySQL schema.

Requires the coordinated root SQL window. Formal Demo4 is SELECT-only; its paid
run is cloned under a fresh login/session. No native result or end packet is sent.
"""
import hashlib
import http.client
import json
import os
from pathlib import Path
import secrets
import selectors
import subprocess
import sys
import tempfile
import time
import types
import unittest

from server.codec import BY_NAME,decode_responses,encode_http_request,message
from hyjs_managed_mysql import ManagedMySQL,SERVICES
import test_p10_mysql as transport

ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tools'))
import shared_mysql


@unittest.skipUnless(os.environ.get('HYJS_COPY_ENCOUNTER_MYSQL_TEST')=='1','Explicit coordinated copy encounter SQL window')
class CopyEncounterManagedMySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    call=transport.P10MySQLTests.call
    stop=transport.P10MySQLTests.stop
    setUp=transport.P10MySQLTests.setUp
    report={}

    @classmethod
    def setUpClass(cls):
        cls.source=Path(os.environ['HYJS_COPY_ENCOUNTER_BUILD_ROOT']).resolve()
        if cls.source.parent.parent!=ROOT/'build/candidates' or cls.source.name!='hyjs-server':raise AssertionError('Fixed candidate required')
        if hashlib.sha256((cls.source/'service/copyService.go').read_bytes()).hexdigest()!='ccb5b5e32b1501ddfd46fa9255434d47e66fafeda92bc182a5f5e8d7daec2097':raise AssertionError('Exact encounter fix/Tech source required')
        # The project connection executes SELECT inside a server-enforced read-only
        # repeatable-read transaction. No credential/session/password column is read.
        output=shared_mysql.sql('SET TRANSACTION ISOLATION LEVEL REPEATABLE READ; START TRANSACTION READ ONLY; SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id=4; ROLLBACK;').strip().split('|')
        if len(output)!=2:raise AssertionError('Actual Demo4 read-only source missing')
        cls.player_raw,cls.game_raw=[bytes.fromhex(value).decode() for value in output]
        player,game=json.loads(cls.player_raw),json.loads(cls.game_raw)
        run=game.get('run')
        if player.get('playerId')!=4 or run is None or run.get('mission')!='8' or run.get('passed')!=['29'] or run.get('fight',{}).get('task')!='29' or not run.get('fight',{}).get('settled'):raise AssertionError('Actual paid mission8 source changed; do not rewrite formal run')
        cls.report={'scope':'Actual formal Demo4 mission8 passed29 source cloned to one managed schema under new original login/session; original BattlePlayer equipment/guards/GUIDs retained; no winner/result/endpacket sent','formalPlayerSHA256':hashlib.sha256(cls.player_raw.encode()).hexdigest(),'formalGameSHA256':hashlib.sha256(cls.game_raw.encode()).hexdigest(),'sourceCopySHA256':'ccb5b5e32b1501ddfd46fa9255434d47e66fafeda92bc182a5f5e8d7daec2097'}
        cls.managed=ManagedMySQL();cls.addClassCleanup(cls.managed.close)
        cls.connection=cls.managed.allocate('copy_encounter')
        cls.env=dict(os.environ,**cls.connection.env());cls.env.update(GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB')
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-copy-encounter-');cls.addClassCleanup(cls.temp.cleanup)
        cls.binary=str(Path(cls.temp.name)/'server')
        result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=cls.source,env=cls.env,capture_output=True,text=True,timeout=180)
        if result.returncode:raise AssertionError(cls.managed.redact(result.stdout+result.stderr))
        cls.report['binarySHA256']=hashlib.sha256(Path(cls.binary).read_bytes()).hexdigest()

    def sql(self,text):return self.managed.sql(self.connection.database,text)

    def start(self):
        self.process=subprocess.Popen([self.binary,'--http-port','0','--tcp-port','0','--tables',str(ROOT/'analysis/tables')],cwd=self.source,env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        deadline=time.monotonic()+90
        line=''
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
        raise AssertionError('Isolated copy server did not become ready')

    def stamped(self,name,fields,login,stamp):
        body=encode_http_request(BY_NAME[name],message(name,**fields),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
        connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=30)
        try:
            connection.request('POST','/sao/',body=body,headers={'Content-Type':'application/octet-stream'})
            response=connection.getresponse();raw=response.read();self.assertEqual(response.status,200)
            return decode_responses(raw)[0]
        finally:connection.close()

    def snapshot(self,id):
        result=self.sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id='+str(id)+';').strip().split('|')
        self.assertEqual(len(result),2)
        return [json.loads(bytes.fromhex(value)) for value in result]

    def test_actual_paid_run_clone_start_retry_conflict_and_cold_without_any_fight_result(self):
        self.start();uid='copy-actual-readonly-clone-'+secrets.token_hex(5);login=self.login(uid)
        self.call('CSPlayerCreate',{'playerName':'隔离实际地图碰撞','heroId':'hero_047'},login,1,'SCPlayerCreate')
        player=json.loads(self.player_raw);player['playerId']=login.playerId
        # Only the fresh isolated account identity changes. Its own UID/password/
        # legal session remain created by the real account service, never copied.
        encoded_player=json.dumps(player,ensure_ascii=False,separators=(',',':')).encode().hex();encoded_game=self.game_raw.encode().hex()
        self.sql('UPDATE profiles SET player=CONVERT(0x'+encoded_player+' USING utf8mb4),game=CONVERT(0x'+encoded_game+' USING utf8mb4) WHERE id='+str(login.playerId)+';')
        before=self.snapshot(login.playerId)
        args={'chapterId':before[1]['run']['chapter'],'missionId':'8','taskId':'31'}
        start=self.call('CSCopyFightStart',args,login,101,'SCCopyFightStart')
        self.assertEqual(start.battlePlayer.playerId,login.playerId)
        self.assertEqual([a.guid for a in start.battlePlayer.formation],[a['guid'] for a in before[1]['run']['fight']['start']['battlePlayer']['formation'] if a['guid'] in {w['guid'] for w in before[1]['warriors']}])
        stable=self.snapshot(login.playerId)
        self.assertEqual(stable[0],before[0])
        self.assertEqual(stable[1]['run']['passed'],['29'])
        self.assertEqual(stable[1]['run']['fight']['task'],'31')
        self.assertFalse(stable[1]['run']['fight'].get('settled',False))
        self.assertEqual(start,self.call('CSCopyFightStart',args,login,102,'SCCopyFightStart'))
        counts=self.sql('SELECT COUNT(*) FROM p6_battle_records WHERE player_id='+str(login.playerId)+'; SELECT COUNT(*) FROM copy_drop_snapshots WHERE player_id='+str(login.playerId)+';')
        for index,extra in enumerate([{'taskId':'30'},{'taskId':'32'},{'taskId':'29'},{'taskId':'999999'},{'taskId':'33'},{'missionId':'9'},{'chapterId':'999'}]):
            request=dict(args,**extra)
            op,_=self.stamped('CSCopyFightStart',request,login,200+index);self.assertEqual(op,10001)
            self.assertEqual(self.snapshot(login.playerId),stable)
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p6_battle_records WHERE player_id='+str(login.playerId)+'; SELECT COUNT(*) FROM copy_drop_snapshots WHERE player_id='+str(login.playerId)+';'),counts)
        self.stop();self.start();again=self.login(uid)
        self.assertEqual(self.call('CSCopyFightStart',args,again,301,'SCCopyFightStart'),start)
        self.assertEqual(self.snapshot(login.playerId),stable)
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.connection.database+"' AND TABLE_NAME IN ('profiles','p6_battle_records','copy_drop_snapshots');").strip(),'InnoDB')
        self.report.update({'actualPublicFrames':'Original54015 start31 afterrealpassed29, exactretry, activeother30/32/pass29/invalid/othermission/otherchapter allreject atomically, coldnewsession exactsavedstart','winnerPacketsSent':0,'endPacketsSent':0,'resourcesAndPaidRunPriorPassedUnchanged':True,'temporaryP6DropCounts':counts.split(),'formalRunWrites':0})

    @classmethod
    def tearDownClass(cls):
        # The wrapper records release success after all class cleanup callbacks.
        pass
