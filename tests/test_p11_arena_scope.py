"""Original public Arena2 rank-only lifecycle against an explicitly fixed isolated source."""
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


@unittest.skipUnless(os.environ.get("HYJS_P11_ARENA_ACCEPTANCE") == "1", "explicit isolated public arena acceptance")
class P11ArenaScopeTests(unittest.TestCase):
    send = transport.P10MySQLTests.send
    login = transport.P10MySQLTests.login
    def stamped(self, name, fields, login, stamp):
        # This test includes whole original-engine calculation and eight locked
        # retries while the root full suite may run. Record actual latency and
        # use a bounded30s transport wait, without changing server behavior.
        raw=encode_http_request(BY_NAME[name],message(name,**fields),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
        connection=http.client.HTTPConnection("127.0.0.1",self.service.http_port,timeout=30)
        started=time.monotonic()
        try:
            connection.request("POST","/sao/",body=raw,headers={"Content-Type":"application/octet-stream"})
            response=connection.getresponse();data=response.read()
            self.assertEqual(response.status,200,data)
            return decode_responses(data)[0]
        finally:
            connection.close()
            if name=="CSPvpFightResult":
                self.arena_latencies=getattr(self,"arena_latencies",[])
                self.arena_latencies.append(round(time.monotonic()-started,3))
    call = transport.P10MySQLTests.call
    profile = transport.P10MySQLTests.profile
    start = transport.P10MySQLTests.start
    stop = transport.P10MySQLTests.stop
    setUp = transport.P10MySQLTests.setUp

    @classmethod
    def setUpClass(cls):
        source = Path(os.environ["HYJS_P11_ARENA_BUILD_ROOT"]).resolve()
        if source.parent.parent != ROOT / "build/candidates" or source.name != "hyjs-server":
            raise AssertionError("fixed owned candidate required")
        handler = (source / "handler/gameHandler.go").read_text()
        if "CSPvpFightResult" not in handler or "p11ArenaScopedFightAt" not in (source / "service/p11ArenaService.go").read_text():
            raise AssertionError("public shared/domain entry missing")
        cls.managed = ManagedMySQL()
        cls.addClassCleanup(cls.managed.close)
        cls.connection = cls.managed.allocate("p11_arena")
        cls.database = cls.connection.database
        cls.test_user = cls.connection.user
        cls.test_password = cls.connection.password
        cls.temp = tempfile.TemporaryDirectory(prefix="hyjs-p11-arena-isolated-")
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / "hyjs-p11-arena")
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

    def test_original_arena_payment_rank_receipt_cold_and_initial_rank_guard(self):
        self.start()
        names = ["arena-public-" + secrets.token_hex(5) for _ in range(3)]
        defender,owner,unknown=[self.login(name) for name in names]
        for login,name,hero in [(defender,"竞技对手甲","hero_047"),(owner,"竞技挑战乙","hero_047"),(unknown,"未知阵容丙","hero_044")]:
            self.call("CSPlayerCreate",{"playerName":name,"heroId":hero},login,1,"SCPlayerCreate")
        ids=",".join(str(x.playerId) for x in [owner,defender,unknown])
        # Explicit finite isolated level preparation preserves all original
        # owned main GUIDs, inventories and actual actor projections.
        self.sql("UPDATE profiles SET player=JSON_SET(player,'$.level',20) WHERE id IN ("+ids+"); UPDATE profiles SET player=JSON_SET(player,'$.level',100) WHERE id="+str(owner.playerId)+";")
        query=self.call("CSPvpFindMatch",{},owner,20,"SCPvpFindMatch")
        ranks={x.info.playerId:x.pvpRank for x in query.rankList}
        ranks[owner.playerId]=query.myrank.pvpRank
        self.assertLess(ranks[defender.playerId],ranks[owner.playerId])
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p11_arena_members WHERE best_won_rank>0;").strip(),"0")
        before_owner=self.profile(owner)
        before_def=self.profile(defender)
        args={"playerId":defender.playerId,"serverId":1}
        self.sql("UPDATE p11_arena_policies SET enabled=1,duel_enabled=1;")
        # The independent rank-changing policy remains off even when Friend11
        # is enabled; no resource or rank result is created by the query.
        self.assertEqual(self.stamped("CSPvpFightResult",args,owner,30)[0],10001)
        self.sql("UPDATE p11_arena_fight_policies SET enabled=1;")
        self.assertEqual(self.stamped("CSPvpFightResult",args,owner,0)[0],10001)
        self.assertEqual(self.stamped("CSPvpFightResult",{"playerId":unknown.playerId,"serverId":1},owner,31)[0],10001)
        self.assertEqual(self.profile(owner),before_owner)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies=list(pool.map(lambda _:self.call("CSPvpFightResult",args,owner,32,"SCPvpFightResult"),range(8)))
        response=replies[0]
        self.assertTrue(all(x==response for x in replies))
        self.assertEqual(response.winPlayerId,owner.playerId)
        self.assertEqual(response.myrank.pvpRank,ranks[defender.playerId])
        self.assertEqual(response.roomId,"map_arena_001")
        self.assertEqual(response.maxRoundNum,40)
        self.assertEqual([(item.id, item.num) for item in response.boxList], [('coin', 0)] * 3)
        self.assertEqual([(x.id,x.num) for x in response.result.simps],[("energy",-2)])
        self.assertEqual([x.playerId for x in response.playerList],[owner.playerId,defender.playerId])
        for actor,profile in zip(response.playerList,[before_owner,before_def]):
            self.assertEqual(len(actor.formation),1)
            self.assertEqual(actor.formation[0].guid,profile[0]["heroWarId"])
            self.assertEqual(actor.formation[0].skillOrder,"047_1,047_2,047_3")
        after=self.profile(owner)
        self.assertEqual(after[0]["energy"],before_owner[0]["energy"]-2)
        expected=json.loads(json.dumps(before_owner));expected[0]["energy"]-=2
        self.assertEqual(after,expected)
        self.assertEqual(self.profile(defender),before_def)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p11_arena_fight_settlements;").strip(),"1")
        self.assertEqual(self.sql("SELECT CONCAT(status,'|',authoritative) FROM p6_battle_records;").strip(),"computed|1")
        self.assertEqual(self.sql("SELECT position FROM p11_arena_members WHERE player_id="+str(owner.playerId)+";").strip(),str(ranks[defender.playerId]))
        self.assertEqual(self.sql("SELECT position FROM p11_arena_members WHERE player_id="+str(defender.playerId)+";").strip(),str(ranks[owner.playerId]))
        self.assertEqual(self.sql("SELECT best_won_rank FROM p11_arena_members WHERE player_id="+str(owner.playerId)+";").strip(),str(ranks[defender.playerId]))
        self.assertEqual(self.sql("SELECT best_won_rank FROM p11_arena_members WHERE player_id="+str(defender.playerId)+";").strip(),"0")
        witness=self.sql("SELECT CONCAT(fight_id,'|',p6_record_id,'|',scope_proof,'|',terminal_hash,'|',response_hash) FROM p11_arena_fight_settlements;")
        self.sql("UPDATE p11_arena_policies SET enabled=0; UPDATE p11_arena_fight_policies SET enabled=0;")
        self.stop();self.start();owner=self.login(names[1])
        self.assertEqual(self.call("CSPvpFightResult",args,owner,32,"SCPvpFightResult"),response)
        self.assertEqual(self.profile(owner),after)
        self.assertEqual(self.sql("SELECT CONCAT(fight_id,'|',p6_record_id,'|',scope_proof,'|',terminal_hash,'|',response_hash) FROM p11_arena_fight_settlements;"),witness)
        self.assertEqual(self.stamped("CSPvpFightResult",args,owner,33)[0],10001)
        self.assertEqual(self.stamped("CSPvpFightResult",{"playerId":unknown.playerId,"serverId":1},owner,32)[0],10001)
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME IN ('p11_arena_fight_settlements','p6_battle_records');").strip(),"InnoDB")
        print("actual modern23101 latencies seconds:",self.arena_latencies)
        print("Original HTTP23101 actual owned047 typed2: independent defaultoff; unknown roster/zero stamp rollback; eight same-stamp retries one real Energy2/rank change; immutable original SC after cold; actual winner best-rank witness, initial ranks excluded; no box/direct unknown reward.")
