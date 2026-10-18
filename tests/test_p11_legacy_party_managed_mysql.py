"""Original Friend11 actual legacy source clone on one managed temporary schema.

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

@unittest.skipUnless(os.environ.get('HYJS_P11_LEGACY_MYSQL_TEST')=='1','Root-coordinated SQL window required')
class P11LegacyPartyManagedMySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    stop=transport.P10MySQLTests.stop
    setUp=transport.P10MySQLTests.setUp
    report={}

    @classmethod
    def setUpClass(cls):
        cls.source=Path(os.environ['HYJS_P11_LEGACY_BUILD_ROOT']).resolve()
        if cls.source.parent.parent!=ROOT/'build/candidates' or cls.source.name!='hyjs-server':raise AssertionError('Fixed candidate required')
        if hashlib.sha256((cls.source/'service/p11FriendFightService.go').read_bytes()).hexdigest()!='56e1f0027d586818a2d146fc6f9afa92508facefccdd3ef533715e76df6c3f91':raise AssertionError('Exact legacy Friend source required')
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-p11-legacy-');cls.addClassCleanup(cls.temp.cleanup)
        cls.binary=os.environ.get('HYJS_P11_LEGACY_BINARY') or str(Path(cls.temp.name)/'server')
        build_env=dict(os.environ,GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB')
        if not os.environ.get('HYJS_P11_LEGACY_BINARY'):
            result=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=cls.source,env=build_env,capture_output=True,text=True,timeout=180)
            if result.returncode:raise AssertionError(result.stdout+result.stderr)
        raw=(ROOT/'analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.actual-profile-source.json').read_bytes()
        if hashlib.sha256(raw).hexdigest()!='89c060a63da435f39a4d15137a0435efb77b68ab786747440f1b081805c0a88b':raise AssertionError('Readonly actual source SHA changed')
        cls.profiles=json.loads(raw)
        proof_files=['service/p11FriendFightService.go','service/p6LegacyPartySettlementService.go','constUtil/p6LegacyPartySettlement.go','service/combatOriginalProtoService.go']
        cls.source_proof={name:hashlib.sha256((cls.source/name).read_bytes()).hexdigest() for name in proof_files if (cls.source/name).exists()}
        cls.managed=ManagedMySQL();cls.addClassCleanup(cls.managed.close)
        cls.connection=cls.managed.allocate('p11_legacy')
        cls.env=dict(os.environ,**cls.connection.env());cls.env.update(GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB')
        cls.report={'scope':'Original public Friend11; immutable actual Demo4/3 P5 full-profile compatibility source cloned under fresh legal isolated accounts; all original001/003/047(tal1)/025/gear/mainGUID/fullthree skills retained; no formal SELECT or writes','binarySHA256':hashlib.sha256(Path(cls.binary).read_bytes()).hexdigest(),'actualSourceSHA256':hashlib.sha256(raw).hexdigest(),'formalDatabaseReadOrWrite':False,'fixedSourceProof':cls.source_proof}

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

    def test_original_public_actual_legacy_party_real_winner_once_cold_and_acl(self):
        self.start()
        uids=['p11-legacy-native-'+secrets.token_hex(5) for _ in range(2)]
        owner,peer=[self.login(uid)for uid in uids]
        for login in [owner,peer]:self.call('CSPlayerCreate',{'playerName':'隔离实际阵容'+str(login.playerId),'heroId':'hero_047'},login,1,'SCPlayerCreate')
        for sample,login in zip(self.profiles,[owner,peer]):
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
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_friend_fight_settlements; SELECT COUNT(*) FROM p6_battle_records;').strip().split(),['0','0'])
        self.assertIsNotNone(response,'No supported real-random attempt within the finite bound')
        self.assertIn(response.winPlayerId,[owner.playerId,peer.playerId])
        self.assertEqual([[h.id for h in p.formation]for p in response.playerList],[['hero_001','hero_003'],['hero_047','hero_025']])
        for player,sample in zip(response.playerList,self.profiles):
            expected={w['guid']:w['id']for w in sample['game']['warriors']}
            for hero in player.formation:
                self.assertEqual(expected[hero.guid],hero.id)
                prefix=hero.id.removeprefix('hero_')
                self.assertEqual(hero.skillOrder,','.join(prefix+'_'+str(i)for i in [1,2,3]))
        self.assertEqual([self.profile(login)for login in [owner,peer]],before)
        frozen=self.sql('SELECT CONCAT(scope_proof,"|",response_hash,"|",terminal_hash) FROM p11_friend_fight_settlements;').strip()
        self.assertTrue(frozen.startswith('c2ade70c613604da2a84cac844b269745288bee1922fa608774b46208fb6d78b|'))
        self.assertEqual(self.call('CSFriendFight',args,owner,accepted_stamp,'SCFriendFight'),response)
        self.assertEqual(self.sql('SELECT CONCAT(status,"|",authoritative) FROM p6_battle_records;').strip(),'computed|1')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_friend_fight_settlements; SELECT COUNT(*) FROM p6_battle_records; SELECT COUNT(*) FROM p11_arena_members WHERE best_won_rank>0;').strip().split(),['1','1','0'])
        self.sql('UPDATE p11_arena_policies SET enabled=0,duel_enabled=0;')
        self.stop();self.start();owner=self.login(uids[0]);peer=self.login(uids[1])
        self.assertEqual(self.call('CSFriendFight',args,owner,accepted_stamp,'SCFriendFight'),response)
        self.assertEqual(self.sql('SELECT CONCAT(scope_proof,"|",response_hash,"|",terminal_hash) FROM p11_friend_fight_settlements;').strip(),frozen)
        self.assertEqual(self.frames('CSFriendFight',args,owner,100)[0][0],10001)
        self.call('CSSendToBlackList',{'friendId':owner.playerId},peer,101,'SCSendToBlackList')
        self.assertEqual(self.frames('CSFriendFight',args,owner,accepted_stamp)[0][0],10001)
        self.assertEqual([self.profile(login)for login in [owner,peer]],before)
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.connection.database+"' AND TABLE_NAME IN ('profiles','p6_battle_records','p11_friend_fight_settlements');").strip(),'InnoDB')
        self.report.update({'actualWinnerPlayerId':response.winPlayerId,'realRandomAttempts':attempt+1,'originalPublicStartComputeReceiptColdOnce':True,'actualLegacyMainAndFullThreeSkillsPreserved':True,'profileResourcesAndRanksUnchanged':True,'currentBlacklistRevokesHistoricalReceipt':True,'globalAuthorityChanged':False})
        self.stop()

    @classmethod
    def tearDownClass(cls):
        cls.managed.close();cls.report['managedSchemaAndAccountReleased']=True
        path=os.environ.get('HYJS_P11_LEGACY_REPORT')
        if path:Path(path).write_text(json.dumps(cls.report,indent=2)+'\n')

del transport
