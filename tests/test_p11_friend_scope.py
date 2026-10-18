"""Original public Friend11 lifecycle against an explicitly fixed isolated source."""
import concurrent.futures
import http.client
import time
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import unittest

from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME, decode_responses, encode_http_request, message
import test_p10_mysql as transport

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get("HYJS_P11_FRIEND_ACCEPTANCE") == "1", "explicit isolated public friend acceptance")
class P11FriendScopeTests(unittest.TestCase):
    send = transport.P10MySQLTests.send
    login = transport.P10MySQLTests.login
    stamped = transport.P10MySQLTests.stamped
    call = transport.P10MySQLTests.call
    profile = transport.P10MySQLTests.profile
    start = transport.P10MySQLTests.start
    stop = transport.P10MySQLTests.stop
    setUp = transport.P10MySQLTests.setUp

    @classmethod
    def setUpClass(cls):
        source = Path(os.environ["HYJS_P11_FRIEND_BUILD_ROOT"]).resolve()
        if source.parent.parent != ROOT / "build/candidates" or source.name != "hyjs-server":
            raise AssertionError("fixed owned candidate required")
        handler = (source / "handler/gameHandler.go").read_text()
        if "CSFriendFight" not in handler or "p11FriendScopedFightAt" not in (source / "service/p11ArenaService.go").read_text():
            raise AssertionError("public shared/domain entry missing")
        cls.managed = ManagedMySQL()
        cls.addClassCleanup(cls.managed.close)
        cls.connection = cls.managed.allocate("p11_friend")
        cls.database = cls.connection.database
        cls.test_user = cls.connection.user
        cls.test_password = cls.connection.password
        cls.temp = tempfile.TemporaryDirectory(prefix="hyjs-p11-friend-isolated-")
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / "hyjs-p11-friend")
        cls.env = dict(os.environ, **cls.connection.env())
        cls.env.update(GOTOOLCHAIN="go1.27.2", GOMEMLIMIT="1GiB")
        result = subprocess.run(["rtk", "proxy", "go", "build", "-o", cls.binary, "."], cwd=source, env=cls.env, capture_output=True, text=True, timeout=180)
        if result.returncode:
            raise AssertionError((result.stdout + result.stderr).replace(cls.test_password, "[redacted]"))

    @classmethod
    def database_sql(cls, statement):
        if not cls.database.startswith("hyjs_test_p11_"):
            raise AssertionError("P11 managed isolation guard")
        return cls.managed.sql(cls.database, statement)

    def sql(self, statement):
        return self.database_sql(statement)

    def test_original_public_unknown_main_rollback(self):
        self.start()
        owner,peer=[self.login("p11-unknown-main-"+secrets.token_hex(5)) for _ in range(2)]
        for login,name,hero in [(owner,"原未知主角甲","hero_044"),(peer,"原已审主角乙","hero_047")]:
            self.call("CSPlayerCreate",{"playerName":name,"heroId":hero},login,1,"SCPlayerCreate")
        self.sql("UPDATE p10_social_policies SET enabled=1; UPDATE p11_arena_policies SET enabled=0,duel_enabled=0;")
        self.call("CSApplyFriend",{"friendId":peer.playerId},owner,10,"SCApplyFriend")
        self.call("CSAgreeApply",{"friendId":owner.playerId},peer,10,"SCAgreeApply")
        before=[self.profile(login) for login in [owner,peer]]
        args={"playerId":peer.playerId,"serverId":1}
        self.assertEqual(self.stamped("CSFriendFight",args,owner,11)[0],10001)
        self.sql("UPDATE p11_arena_policies SET enabled=1,duel_enabled=1;")
        for stamp in [0,12,13]:
            self.assertEqual(self.stamped("CSFriendFight",args,owner,stamp)[0],10001)
        self.assertEqual([self.profile(login) for login in [owner,peer]],before)
        for table in ["p6_battle_records","p11_friend_fight_settlements"]:
            self.assertEqual(self.sql("SELECT COUNT(*) FROM "+table+" WHERE player_id="+str(owner.playerId)+";").strip(),"0")
        print("Original HTTP21087: allowed unknown044 main retained; defaultoff/zeroStamp/unknown family all rejected without partial capture/receipt/profile changes.")

    def test_original_friend_full_start_compute_receipt_cold_and_permissions(self):
        self.start();self.team_latencies=[];self.stamped=self.full_engine_stamped
        names = ["p11-playable-" + secrets.token_hex(5) for _ in range(3)]
        owner, peer, unsupported = [self.login(name) for name in names]
        for login, name, hero in [(owner, "真实切磋甲", "hero_047"), (peer, "真实切磋乙", "hero_047"), (unsupported, "未知范围丙", "hero_044")]:
            self.call("CSPlayerCreate", {"playerName": name, "heroId": hero}, login, 1, "SCPlayerCreate")
        # Finite local level preparation retains each original main and all
        # actual default inventory/formation data; no extra skill or item source.
        ids=",".join(str(item.playerId) for item in [owner,peer,unsupported])
        self.sql("UPDATE profiles SET player=JSON_SET(player,'$.level',20) WHERE id IN ("+ids+"); UPDATE p10_social_policies SET enabled=1;")
        for target, stamp in [(peer, 20), (unsupported, 21)]:
            self.call("CSApplyFriend", {"friendId": target.playerId, "word": "实际好友切磋"}, owner, stamp, "SCApplyFriend")
            self.call("CSAgreeApply", {"friendId": owner.playerId}, target, stamp, "SCAgreeApply")
        players = [owner, peer, unsupported]
        before = [self.profile(login) for login in players]
        args = {"playerId": peer.playerId, "serverId": 1}
        policy = self.sql("SELECT CONCAT(enabled,'|',duel_enabled) FROM p11_arena_policies WHERE id=1;").strip()
        self.assertEqual(policy, "0|0")
        self.assertEqual(self.stamped("CSFriendFight", args, owner, 100)[0], 10001)
        self.sql("UPDATE p11_arena_policies SET enabled=1,duel_enabled=1;")
        self.assertEqual(self.stamped("CSFriendFight", args, owner, 0)[0], 10001)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.call("CSFriendFight", args, owner, 101, "SCFriendFight"), range(8)))
        response = replies[0]
        self.assertTrue(all(item == response for item in replies))
        self.assertIn(response.winPlayerId, [owner.playerId, peer.playerId])
        self.assertEqual(response.maxRoundNum, 40)
        self.assertEqual(response.roomId, "map_forest_001")
        self.assertEqual([item.playerId for item in response.playerList], [owner.playerId, peer.playerId])
        for item, profile in zip(response.playerList, before[:2]):
            self.assertEqual(len(item.formation), 1)
            self.assertEqual(item.formation[0].id, "hero_047")
            self.assertEqual(item.formation[0].guid, profile[0]["heroWarId"])
            self.assertEqual(item.formation[0].skillOrder, "047_1,047_2,047_3")
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p11_friend_fight_settlements;").strip(), "1")
        self.assertEqual(self.sql("SELECT CONCAT(status,'|',authoritative) FROM p6_battle_records;").strip(), "computed|1")
        frozen = self.sql("SELECT CONCAT(fight_id,'|',p6_record_id,'|',scope_proof,'|',response_hash,'|',terminal_hash) FROM p11_friend_fight_settlements;")
        self.assertEqual([self.profile(login) for login in players], before)
        self.assertEqual(self.stamped("CSFightPlayback", {"fightId":response.fightId,"serverId":1},owner,105)[0],10001)
        # A distinct valid original main is unsupported, with no partial capture.
        self.assertEqual(self.stamped("CSFriendFight", {"playerId": unsupported.playerId, "serverId": 1}, owner, 102)[0], 10001)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p6_battle_records;").strip(), "1")
        # Current rosters and switch changes cannot recompute an old accepted fight.
        self.sql("UPDATE p11_arena_policies SET enabled=0,duel_enabled=0;")
        self.stop()
        self.start()
        owner = self.login(names[0])
        self.assertEqual(self.call("CSFriendFight", args, owner, 101, "SCFriendFight"), response)
        self.assertEqual(self.sql("SELECT CONCAT(fight_id,'|',p6_record_id,'|',scope_proof,'|',response_hash,'|',terminal_hash) FROM p11_friend_fight_settlements;"), frozen)
        self.assertEqual(self.stamped("CSFriendFight", args, owner, 103)[0], 10001)
        self.assertEqual(self.stamped("CSFriendFight", {"playerId": unsupported.playerId, "serverId": 1}, owner, 101)[0], 10001)
        self.call("CSSendToBlackList", {"friendId": owner.playerId}, peer, 104, "SCSendToBlackList")
        self.assertEqual(self.stamped("CSFriendFight", args, owner, 101)[0], 10001)
        self.assertEqual([self.profile(login) for login in [owner, peer, unsupported]], before)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p11_friend_fight_settlements;").strip(), "1")
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p11_arena_members WHERE best_won_rank>0;").strip(), "0")
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME IN ('p11_friend_fight_settlements','p6_battle_records');").strip(), "InnoDB")
        print("Original public Friend11: allowed starter ownership retained; actual runtime seed/winner;8 retries once;unknown real main rollback;defaultoff/cold/friend ACL;all profiles/ranks unchanged.")

    def full_engine_stamped(self,name,fields,login,stamp):
        raw=encode_http_request(BY_NAME[name],message(name,**fields),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
        connection=http.client.HTTPConnection("127.0.0.1",self.service.http_port,timeout=30)
        started=time.monotonic()
        try:
            connection.request("POST","/sao/",body=raw,headers={"Content-Type":"application/octet-stream"})
            response=connection.getresponse();data=response.read();self.assertEqual(response.status,200,data)
            return decode_responses(data)[0]
        finally:
            connection.close()
            if name=="CSFriendFight":self.team_latencies.append(round(time.monotonic()-started,3))

    def test_original_owned_partner_claim_formation_duel_and_cold(self):
        self.start();self.team_latencies=[];self.stamped=self.full_engine_stamped
        names=["p11-owned-team-"+secrets.token_hex(5) for _ in range(2)]
        owner,peer=[self.login(name)for name in names]
        for login,name in [(owner,"实际主角伙伴甲"),(peer,"实际主角伙伴乙")]:
            self.call("CSPlayerCreate",{"playerName":name,"heroId":"hero_047"},login,1,"SCPlayerCreate")
        ids=",".join(str(x.playerId)for x in [owner,peer])
        self.sql("UPDATE profiles SET player=JSON_SET(player,'$.level',100) WHERE id IN ("+ids+"); UPDATE p10_social_policies SET enabled=1;")
        self.call("CSApplyFriend",{"friendId":peer.playerId},owner,10,"SCApplyFriend")
        self.call("CSAgreeApply",{"friendId":owner.playerId},peer,10,"SCAgreeApply")
        # Explicit finite isolated operational delivery rows prepare owned
        # resources. Public original claim and formation handlers create and
        # equip each actual GUID; no profile/actor/HP/winner edits are used.
        def claim_form(login,hero,slot,stamp):
            key="p11-owned-http-"+secrets.token_hex(5)
            self.sql("INSERT INTO p1_reward_deliveries(player_id,source_key,title,content,rewards,created_at,expires_at,claimed_at) VALUES("+str(login.playerId)+",'"+key+"','finite original formation','explicit isolated operational fixture, not recruit','"+hero+",1',"+str(int(time.time()*1000))+",0,0);")
            reward_id=int(self.sql("SELECT id FROM p1_reward_deliveries WHERE source_key='"+key+"';").strip())
            receipt=self.call("CSAddRewardsToPlayer",{"rewardIds":[reward_id]},login,stamp,"SCAddRewardsToPlayer")
            self.assertEqual(len(receipt.result.wars),1);card=receipt.result.wars[0]
            self.assertEqual(card.id,hero);self.assertTrue(card.guid)
            self.call("CSFormationOn",{"id":hero,"guid":card.guid,"formatPos":slot},login,stamp+1,"SCFormationOn")
            self.assertGreater(int(self.sql("SELECT claimed_at FROM p1_reward_deliveries WHERE id="+str(reward_id)+";").strip()),0)
            return card.guid
        own_guid=claim_form(owner,"hero_069",2,20);peer_guid=claim_form(peer,"hero_070",2,30)
        before=[self.profile(x)for x in [owner,peer]]
        args={"playerId":peer.playerId,"serverId":1}
        self.assertEqual(self.stamped("CSFriendFight",args,owner,40)[0],10001)
        self.sql("UPDATE p11_arena_policies SET enabled=1,duel_enabled=1;")
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies=list(pool.map(lambda _:self.call("CSFriendFight",args,owner,41,"SCFriendFight"),range(8)))
        response=replies[0];self.assertTrue(all(x==response for x in replies))
        self.assertIn(response.winPlayerId,[owner.playerId,peer.playerId])
        self.assertEqual([x.playerId for x in response.playerList],[owner.playerId,peer.playerId])
        for player,profile,partner in zip(response.playerList,before,[own_guid,peer_guid]):
            self.assertEqual(len(player.formation),2)
            self.assertEqual([(x.guid,x.pose)for x in player.formation],[(profile[0]["heroWarId"],3),(partner,7)])
            self.assertEqual(player.formation[0].skillOrder,"047_1,047_2,047_3")
        self.assertEqual([self.profile(x)for x in [owner,peer]],before)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p11_friend_fight_settlements WHERE player_id="+str(owner.playerId)+";").strip(),"1")
        self.assertEqual(self.sql("SELECT CONCAT(status,'|',authoritative) FROM p6_battle_records WHERE player_id="+str(owner.playerId)+";").strip(),"computed|1")
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p11_arena_members WHERE best_won_rank>0;").strip(),"0")
        claim_form(owner,"hero_071",3,50);after_extra=self.profile(owner)
        self.assertEqual(self.stamped("CSFriendFight",args,owner,42)[0],10001)
        self.assertEqual(self.profile(owner),after_extra)
        self.sql("UPDATE p11_arena_policies SET enabled=0,duel_enabled=0;")
        self.stop();self.start();owner=self.login(names[0])
        self.assertEqual(self.call("CSFriendFight",args,owner,41,"SCFriendFight"),response)
        self.assertEqual(self.profile(owner),after_extra)
        self.call("CSSendToBlackList",{"friendId":owner.playerId},peer,60,"SCSendToBlackList")
        self.assertEqual(self.stamped("CSFriendFight",args,owner,41)[0],10001)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p6_battle_records WHERE player_id="+str(owner.playerId)+";").strip(),"1")
        print("Original HTTP21087 owned twoactor: actual claim/formation/mainGUID retained; true winner/eight retries once; no profile/resources/ranks; real thirdactor rollback; disabled/current-roster drift cold frozen receipt; current blacklist revokes. Latencies:",self.team_latencies)
