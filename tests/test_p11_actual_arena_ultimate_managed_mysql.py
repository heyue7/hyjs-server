"""Root-serialized original23101 complete-party acceptance on one managed schema.

Every challenge queries a real current P5 roster and uses server randomness.
Only an immutable prior actual profile is cloned to finite isolated accounts.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import unittest
from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME
import test_p10_mysql as transport
from test_p11_actual_party_ultimate_managed_mysql import P11ActualPartyUltimateManagedMySQLTests as FriendHarness
ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P11_ACTUAL_ARENA_MYSQL_TEST')=='1','Root-coordinated managed SQL window required')
class P11ActualArenaUltimateManagedMySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    stop=transport.P10MySQLTests.stop
    setUp=transport.P10MySQLTests.setUp
    start=FriendHarness.start
    frames=FriendHarness.frames
    call=FriendHarness.call
    profile=FriendHarness.profile

    @classmethod
    def setUpClass(cls):
        cls.source=Path(os.environ['HYJS_P11_ACTUAL_ARENA_BUILD_ROOT']).resolve()
        if cls.source.parent.parent!=ROOT/'build/candidates' or cls.source.name!='hyjs-server':raise AssertionError('Fixed candidate required')
        pins={'service/p11ArenaFightService.go':'7f2b39b9d0787166ce1db3c53af3e65e8c142b6e45ea88ab0a4d2af8598d3a55','service/p6ActualArenaSettlementService.go':None,'constUtil/p6ActualArenaSettlement.go':None}
        for name,expected in pins.items():
            actual=hashlib.sha256((cls.source/name).read_bytes()).hexdigest()
            if expected and actual!=expected:raise AssertionError('Exact original Arena full-party proposal required')
            pins[name]=actual
        cls.binary=os.environ['HYJS_P11_ACTUAL_ARENA_BINARY']
        if hashlib.sha256(Path(cls.binary).read_bytes()).hexdigest()!='b3bbaf976146f4aa1e20db739a87625c1f7852ba6c2e9c4f7ab2caf447f9aeef':raise AssertionError('Prebuilt fixed native binary required; no concurrent Go build')
        raw=(ROOT/'analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.actual-profile-source.json').read_bytes()
        if hashlib.sha256(raw).hexdigest()!='89c060a63da435f39a4d15137a0435efb77b68ab786747440f1b081805c0a88b':raise AssertionError('Actual profile source changed')
        cls.profiles=json.loads(raw)
        match=re.search(r'P6ActualArenaNativeProof\s*=\s*"([a-f0-9]{64})"',(cls.source/'constUtil/p6ActualArenaSettlement.go').read_text())
        if not match:raise AssertionError('Independent typed2 proof required before allocation')
        cls.scope_proof=match[1]
        cls.managed=ManagedMySQL();cls.addClassCleanup(cls.managed.close)
        cls.connection=cls.managed.allocate('p11_fullarena')
        cls.env=dict(os.environ,**cls.connection.env(),GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='768MiB',GOMAXPROCS='2')
        cls.report={'scope':'Original modern23101 complete owned actual P5 parties; finite isolated level20 eligibility, no panel edits, real server seed and query-frozen roster; no formal SQL read or write','fixedSourceProof':pins,'binarySHA256':hashlib.sha256(Path(cls.binary).read_bytes()).hexdigest(),'cases':[]}

    def sql(self,text):return self.managed.sql(self.connection.database,text)

    def run_actual_party(self,party):
        self.start()
        uids=['p11-fullarena-'+secrets.token_hex(5)for _ in range(2)]
        peer,owner=[self.login(uid)for uid in uids]
        for login in [peer,owner]:self.call('CSPlayerCreate',{'playerName':'隔离竞技完整阵容'+str(login.playerId),'heroId':'hero_047'},login,1,'SCPlayerCreate')
        sample=self.profiles[party]
        for login in [owner,peer]:
            player=dict(sample['player']);player.update(playerId=login.playerId,level=max(20,player['level']))
            phex=json.dumps(player,ensure_ascii=False,separators=(',',':')).encode().hex()
            ghex=json.dumps(sample['game'],ensure_ascii=False,separators=(',',':')).encode().hex()
            self.sql('UPDATE profiles SET player=CONVERT(0x'+phex+' USING utf8mb4),game=CONVERT(0x'+ghex+' USING utf8mb4) WHERE id='+str(login.playerId)+';')
        # The old readonly profile contains historical recovery clocks. Settle
        # them through the original login before the payment baseline; do not
        # attribute normal stamina/energy recovery to the Arena transaction.
        peer,owner=[self.login(uid)for uid in uids]
        for login in [owner,peer]:self.call('CSPlayer',{},login,2,'SCPlayer')
        self.sql('UPDATE p11_arena_policies SET enabled=1;')
        query=self.call('CSPvpFindMatch',{},owner,10,'SCPvpFindMatch')
        ranks={x.info.playerId:x.pvpRank for x in query.rankList};ranks[owner.playerId]=query.myrank.pvpRank
        self.assertIn(peer.playerId,ranks)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_members WHERE best_won_rank>0 AND player_id IN ('+str(owner.playerId)+','+str(peer.playerId)+');').strip(),'0')
        before=[self.profile(x)for x in [owner,peer]]
        args={'playerId':peer.playerId,'serverId':1}
        self.sql('UPDATE p11_arena_fight_policies SET enabled=0;')
        self.assertEqual(self.frames('CSPvpFightResult',args,owner,20)[0][0],10001)
        self.sql('UPDATE p11_arena_fight_policies SET enabled=1;')
        self.assertEqual(self.frames('CSPvpFightResult',args,owner,0)[0][0],10001)
        response=self.call('CSPvpFightResult',args,owner,21,'SCPvpFightResult')
        self.assertIn(response.winPlayerId,[owner.playerId,peer.playerId])
        self.assertEqual([p.playerId for p in response.playerList],[owner.playerId,peer.playerId])
        for player in response.playerList:
            self.assertEqual([h.id for h in player.formation],(['hero_001','hero_003']if party==0 else ['hero_047','hero_025']))
            self.assertEqual([h.guid for h in player.formation],[x.guid for x in response.playerList[0].formation])
            for actor in player.formation:
                prefix=actor.id.removeprefix('hero_');self.assertEqual(actor.skillOrder,','.join(prefix+'_'+str(i)for i in [1,2,3]))
        self.assertEqual([(x.id,x.num)for x in response.boxList],[('coin',0)]*3)
        self.assertEqual([(x.id,x.num)for x in response.result.simps],[('energy',-2)])
        expected=json.loads(json.dumps(before));expected[0][0]['energy']-=2
        self.assertEqual([self.profile(x)for x in [owner,peer]],expected)
        row=self.sql('SELECT CONCAT(scope_proof,"|",attacker_old_rank,"|",defender_old_rank,"|",attacker_new_rank,"|",defender_new_rank,"|",winner_id) FROM p11_arena_fight_settlements WHERE player_id='+str(owner.playerId)+';').strip().split('|')
        self.assertEqual(row[0],self.scope_proof)
        own,other=int(row[1]),int(row[2])
        if response.winPlayerId==owner.playerId and own>other:own,other=other,own
        self.assertEqual([int(row[3]),int(row[4])],[own,other])
        self.assertEqual(response.myrank.pvpRank,own)
        terminal=json.loads(bytes.fromhex(self.sql('SELECT HEX(terminal) FROM p11_arena_fight_settlements WHERE player_id='+str(owner.playerId)+';').strip()))
        self.assertTrue(terminal['outcome']['authoritative']);self.assertEqual(terminal['outcome']['capabilityHash'],self.scope_proof)
        begun=sorted({e.get('skill')for e in terminal['outcome']['events']if e.get('kind')=='skill_begin'}&{'001_3','003_3','025_3','047_3'})
        self.assertTrue(({'001_3','003_3'}if party==0 else {'025_3','047_3'})<=set(begun))
        frozen=self.sql('SELECT CONCAT(response_hash,"|",terminal_hash,"|",scope_proof) FROM p11_arena_fight_settlements WHERE player_id='+str(owner.playerId)+';')
        self.assertEqual(self.sql('SELECT CONCAT(status,"|",authoritative) FROM p6_battle_records WHERE player_id='+str(owner.playerId)+';').strip(),'computed|1')
        self.assertEqual(self.call('CSPvpFightResult',args,owner,21,'SCPvpFightResult'),response)
        self.sql('UPDATE p11_arena_fight_policies SET enabled=0; UPDATE p11_arena_policies SET enabled=0;')
        self.stop();self.start();owner=self.login(uids[1]);peer=self.login(uids[0])
        self.assertEqual(self.call('CSPvpFightResult',args,owner,21,'SCPvpFightResult'),response)
        self.assertEqual([self.profile(x)for x in [owner,peer]],expected)
        self.assertEqual(self.frames('CSPvpFightResult',args,owner,22)[0][0],10001)
        self.assertEqual(self.sql('SELECT CONCAT(response_hash,"|",terminal_hash,"|",scope_proof) FROM p11_arena_fight_settlements WHERE player_id='+str(owner.playerId)+';'),frozen)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_fight_settlements WHERE player_id='+str(owner.playerId)+'; SELECT COUNT(*) FROM p6_battle_records WHERE player_id='+str(owner.playerId)+';').strip().split(),['1','1'])
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.connection.database+"' AND TABLE_NAME IN ('profiles','p6_battle_records','p11_arena_fight_settlements');").strip(),'InnoDB')
        self.report['cases'].append({'partyIndex':party,'winnerID':response.winPlayerId,'actualExecutedUltimates':begun,'finalRound':terminal['outcome']['finalRound'],'fullOriginalThreeSkillsRetained':True,'realQueryFrozenP5NoPanelEdits':True,'costEnergy2ExactlyOnce':True,'computedRankTransition':True,'coldImmutableReceipt':True,'defaultOffAndZeroStampRejected':True})
        self.stop()

    def test_0_original_001003_complete_arena_party(self):self.run_actual_party(0)
    def test_1_original_047025_complete_arena_party(self):self.run_actual_party(1)

    @classmethod
    def tearDownClass(cls):
        cls.managed.close();cls.report['managedResourcesReleased']=True
        target=os.environ.get('HYJS_P11_ACTUAL_ARENA_REPORT')
        if target:Path(target).write_text(json.dumps(cls.report,indent=2,ensure_ascii=False)+'\n')
