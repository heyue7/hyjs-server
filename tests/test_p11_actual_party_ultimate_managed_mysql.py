"""Original Friend11 complete actual ultimate-party sources on one managed schema.

Use only the root-coordinated SQL window. Both real readonly profiles are cloned
to fresh isolated account IDs without actor, skill, gear or GUID changes.
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

@unittest.skipUnless(os.environ.get('HYJS_P11_ACTUAL_PARTY_MYSQL_TEST')=='1','Root-coordinated SQL window required')
class P11ActualPartyUltimateManagedMySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    stop=transport.P10MySQLTests.stop
    setUp=transport.P10MySQLTests.setUp
    report={}

    @classmethod
    def setUpClass(cls):
        cls.source=Path(os.environ['HYJS_P11_ACTUAL_PARTY_BUILD_ROOT']).resolve()
        if cls.source.parent.parent!=ROOT/'build/candidates' or cls.source.name!='hyjs-server':raise AssertionError('Fixed candidate required')
        if hashlib.sha256((cls.source/'service/p11FriendFightService.go').read_bytes()).hexdigest()!='8766d0f0383fe30d0bda1a33f578473ad0ab5a3e7461b22d504be48347348ed1':raise AssertionError('Exact complete-party Friend proposal required')
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-p11-actual-party-');cls.addClassCleanup(cls.temp.cleanup)
        cls.binary=os.environ.get('HYJS_P11_ACTUAL_PARTY_BINARY') or str(Path(cls.temp.name)/'server')
        build_env=dict(os.environ,GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='768MiB',GOMAXPROCS='2')
        if not os.environ.get('HYJS_P11_ACTUAL_PARTY_BINARY'):
            result=subprocess.run(['rtk','proxy','go','build','-p','1','-o',cls.binary,'.'],cwd=cls.source,env=build_env,capture_output=True,text=True,timeout=180)
            if result.returncode:raise AssertionError(result.stdout+result.stderr)
        raw=(ROOT/'analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.actual-profile-source.json').read_bytes()
        if hashlib.sha256(raw).hexdigest()!='89c060a63da435f39a4d15137a0435efb77b68ab786747440f1b081805c0a88b':raise AssertionError('Readonly actual source SHA changed')
        cls.profiles=json.loads(raw)
        import re
        match=re.search(r'P6ActualPartyNativeProof\s*=\s*"([a-f0-9]{64})"',(cls.source/'constUtil/p6ActualPartySettlement.go').read_text())
        if not match:raise AssertionError('Independent original ultimate proof must be certified before any SQL allocation')
        cls.scope_proof=match[1]
        proof_files=['service/p11FriendFightService.go','service/p6ActualPartySettlementService.go','constUtil/p6ActualPartySettlement.go','service/combatOriginalProtoService.go']
        cls.source_proof={name:hashlib.sha256((cls.source/name).read_bytes()).hexdigest() for name in proof_files if (cls.source/name).exists()}
        cls.managed=ManagedMySQL();cls.addClassCleanup(cls.managed.close)
        cls.connection=cls.managed.allocate('p11_actual_party')
        cls.env=dict(os.environ,**cls.connection.env());cls.env.update(GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='768MiB',GOMAXPROCS='2')
        cls.report={'scope':'Original public Friend11; complete immutable actual P5 party mirrored under fresh isolated legal accounts. All original actor/gear/GUID/fullthree skills retained; actual ultimate executions/timeout, no formal SELECT or writes','cases':[],'binarySHA256':hashlib.sha256(Path(cls.binary).read_bytes()).hexdigest(),'actualSourceSHA256':hashlib.sha256(raw).hexdigest(),'formalDatabaseReadOrWrite':False,'fixedSourceProof':cls.source_proof}

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
        raise AssertionError('Legacy Friend isolated server failed readiness')

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

    def run_complete_party(self,party_index):
        self.start()
        uids=['p11-complete-party-native-'+secrets.token_hex(5) for _ in range(2)]
        owner,peer=[self.login(uid)for uid in uids]
        for login in [owner,peer]:self.call('CSPlayerCreate',{'playerName':'隔离实际阵容'+str(login.playerId),'heroId':'hero_047'},login,1,'SCPlayerCreate')
        for sample,login in zip([self.profiles[party_index]]*2,[owner,peer]):
            player=dict(sample['player']);player['playerId']=login.playerId
            player_hex=json.dumps(player,ensure_ascii=False,separators=(',',':')).encode().hex()
            game_hex=json.dumps(sample['game'],ensure_ascii=False,separators=(',',':')).encode().hex()
            self.sql('UPDATE profiles SET player=CONVERT(0x'+player_hex+' USING utf8mb4),game=CONVERT(0x'+game_hex+' USING utf8mb4) WHERE id='+str(login.playerId)+';')
        self.sql('UPDATE p10_social_policies SET enabled=1;')
        self.call('CSApplyFriend',{'friendId':peer.playerId,'word':'原实际阵容切磋'},owner,10,'SCApplyFriend')
        self.call('CSAgreeApply',{'friendId':owner.playerId},peer,10,'SCAgreeApply')
        before=[self.profile(login)for login in [owner,peer]]
        args={'playerId':peer.playerId,'serverId':1}
        self.assertEqual(self.frames('CSFriendFight',args,owner,20)[0][0],10001)
        self.sql('UPDATE p11_arena_policies SET enabled=1,duel_enabled=1;')
        self.assertEqual(self.frames('CSFriendFight',args,owner,0)[0][0],10001)
        # Every attempt uses server randomness. An unobserved executed branch
        # must roll back rather than manufacturing a winner or stripping skills.
        response=None
        for attempt in range(16):
            accepted_stamp=21+attempt
            frames=self.frames('CSFriendFight',args,owner,accepted_stamp)
            if frames[0][0]==BY_NAME['SCFriendFight']:
                response=frames[0][1];break
            self.assertEqual(frames[0][0],10001)
            self.assertIn('服务端战斗尚未支持该技能效果或运行状态',str(frames[0][1]))
            self.assertEqual([self.profile(login)for login in [owner,peer]],before)
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_friend_fight_settlements WHERE player_id='+str(owner.playerId)+'; SELECT COUNT(*) FROM p6_battle_records WHERE player_id='+str(owner.playerId)+';').strip().split(),['0','0'])
        self.assertIsNotNone(response,'No supported real-random attempt within the finite bound')
        self.assertIn(response.winPlayerId,[owner.playerId,peer.playerId])
        self.assertEqual([[h.id for h in p.formation]for p in response.playerList],([['hero_001','hero_003']]*2 if party_index==0 else [['hero_047','hero_025']]*2))
        for player,sample in zip(response.playerList,[self.profiles[party_index]]*2):
            expected={w['guid']:w['id']for w in sample['game']['warriors']}
            for hero in player.formation:
                self.assertEqual(expected[hero.guid],hero.id)
                prefix=hero.id.removeprefix('hero_')
                self.assertEqual(hero.skillOrder,','.join(prefix+'_'+str(i)for i in [1,2,3]))
        self.assertEqual([self.profile(login)for login in [owner,peer]],before)
        frozen=self.sql('SELECT CONCAT(scope_proof,"|",response_hash,"|",terminal_hash) FROM p11_friend_fight_settlements WHERE player_id='+str(owner.playerId)+';').strip()
        self.assertTrue(frozen.startswith(self.scope_proof+'|'))
        terminal=json.loads(bytes.fromhex(self.sql('SELECT HEX(terminal) FROM p11_friend_fight_settlements WHERE player_id='+str(owner.playerId)+';').strip()))
        begun={e.get('skill')for e in terminal['outcome']['events']if e.get('kind')=='skill_begin'}
        self.assertTrue(({'001_3','003_3'}if party_index==0 else {'047_3','025_3'})<=begun,'Ultimate must really begin, queued press alone is insufficient')
        self.assertEqual(self.call('CSFriendFight',args,owner,accepted_stamp,'SCFriendFight'),response)
        self.assertEqual(self.sql('SELECT CONCAT(status,"|",authoritative) FROM p6_battle_records WHERE player_id='+str(owner.playerId)+';').strip(),'computed|1')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_friend_fight_settlements WHERE player_id='+str(owner.playerId)+'; SELECT COUNT(*) FROM p6_battle_records WHERE player_id='+str(owner.playerId)+'; SELECT COUNT(*) FROM p11_arena_members WHERE best_won_rank>0;').strip().split(),['1','1','0'])
        self.sql('UPDATE p11_arena_policies SET enabled=0,duel_enabled=0;')
        self.stop();self.start();owner=self.login(uids[0]);peer=self.login(uids[1])
        self.assertEqual(self.call('CSFriendFight',args,owner,accepted_stamp,'SCFriendFight'),response)
        self.assertEqual(self.sql('SELECT CONCAT(scope_proof,"|",response_hash,"|",terminal_hash) FROM p11_friend_fight_settlements WHERE player_id='+str(owner.playerId)+';').strip(),frozen)
        self.assertEqual(self.frames('CSFriendFight',args,owner,100)[0][0],10001)
        self.call('CSSendToBlackList',{'friendId':owner.playerId},peer,101,'SCSendToBlackList')
        self.assertEqual(self.frames('CSFriendFight',args,owner,accepted_stamp)[0][0],10001)
        self.assertEqual([self.profile(login)for login in [owner,peer]],before)
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.connection.database+"' AND TABLE_NAME IN ('profiles','p6_battle_records','p11_friend_fight_settlements');").strip(),'InnoDB')
        self.report['cases'].append({'partyIndex':party_index,'actualExecutedUltimates':sorted(begun&{'001_3','003_3','047_3','025_3'}),'actualWinnerPlayerId':response.winPlayerId,'realRandomAttempts':attempt+1,'originalPublicStartComputeReceiptColdOnce':True,'actualCompletePartyAndFullThreeSkillsPreserved':True,'profileResourcesAndRanksUnchanged':True,'currentBlacklistRevokesHistoricalReceipt':True,'globalAuthorityChanged':False})
        self.stop()

    def test_0_original_public_001003_actual_ultimate_complete_party(self):
        self.run_complete_party(0)

    def test_1_original_public_047025_actual_ultimate_complete_party(self):
        self.run_complete_party(1)

    @classmethod
    def tearDownClass(cls):
        cls.managed.close();cls.report['managedSchemaAndAccountReleased']=True
        path=os.environ.get('HYJS_P11_ACTUAL_PARTY_REPORT')
        if path:Path(path).write_text(json.dumps(cls.report,indent=2)+'\n')

del transport
