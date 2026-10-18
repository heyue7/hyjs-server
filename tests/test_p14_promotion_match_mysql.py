"""Real two-realm qualification and purpose2 feed the scoped typed17 consumer.

Requires Root's single SQL window and an immutable prebuilt source. Empty P6
proof is tested as a full rollback; certified builds exercise actual results.
No outcome, qualification, grant, point or score rows are prepared by fixtures.
"""
import concurrent.futures
import csv
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import unittest

from hyjs_managed_mysql import provision_case
from server.codec import BY_NAME, CLASSES
import test_p14_cross_mysql as cross
from test_p14_qualification_native_mysql import P14QualificationNativeMySQLTests as qualified
from test_p14_prayer_typed17_source_mysql import P14PrayerTyped17SourceMySQLTests as sources

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get("HYJS_P14_PROMOTION_MATCH_MYSQL_TEST") == "1", "explicit scoped promotion isolated native")
class P14PromotionMatchMySQLTests(unittest.TestCase):
    root_sql = classmethod(cross.P14CrossMySQLTests.root_sql.__func__)
    sql = cross.P14CrossMySQLTests.sql
    setUp = cross.P14CrossMySQLTests.setUp
    stop_all = cross.P14CrossMySQLTests.stop_all
    send = sources.send
    call = cross.P14CrossMySQLTests.call
    login = cross.P14CrossMySQLTests.login
    wait_sql = cross.P14CrossMySQLTests.wait_sql
    start = qualified.start
    stop = qualified.stop
    profiles = qualified.profiles

    @classmethod
    def setUpClass(cls):
        cls.native_report = None
        cls.source = Path(os.environ["HYJS_P14_PROMOTION_MATCH_SOURCE"]).resolve()
        cls.prebuilt = Path(os.environ["HYJS_P14_PROMOTION_MATCH_PREBUILT"]).resolve()
        assert cls.source.name.startswith("hyjs-p14-promotion-match-")
        assert cls.prebuilt.name.startswith("hyjs-p14-match-prebuilt-")
        cls.meta = json.loads((cls.prebuilt / "manifest.json").read_text())
        for path, digest in cls.meta["files"].items():
            assert hashlib.sha256((cls.source / path).read_bytes()).hexdigest() == digest, path
        cls.binary = cls.prebuilt / "hyjs-native"
        assert hashlib.sha256(cls.binary.read_bytes()).hexdigest() == cls.meta["nativeSHA256"]
        cls.lock = open("/tmp/hyjs-isolated-mysql.lock", "a+")
        fcntl.flock(cls.lock, fcntl.LOCK_EX)
        cls.addClassCleanup(lambda: (fcntl.flock(cls.lock, fcntl.LOCK_UN), cls.lock.close()))
        def record_cleanup():
            report=getattr(cls,"native_report",None)
            if report is None:
                return
            owned=bool(cls.managed_mysql.connections)
            report["managedAllocationRemoved"]=not owned
            report["managedConnectionFilesRemoved"]=all(not ((Path.home() / '.codex/local-services/test-connections')/(name+".env")).exists() for name in cls.realms.values())
            report["cleanup"]= "managed test-drop released both schemas and shared temporary account" if not owned and report["managedConnectionFilesRemoved"] else "FAILED_OWNED_ALLOCATION_CLEANUP"
            (ROOT/"analysis/p14/promotion-match-native-validation.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
            if owned or not report["managedConnectionFilesRemoved"]:
                raise AssertionError("promotion managed allocations not released")
        cls.addClassCleanup(record_cleanup)
        pool, primary = provision_case(cls, "p13_promotion_match1")
        cls.realm_connections = {1:primary, 2:pool.allocate("p13_promotion_match2")}
        cls.realms = {zone:connection.database for zone,connection in cls.realm_connections.items()}
        cls.temp = tempfile.TemporaryDirectory(prefix="hyjs-p14-match-clock-")
        cls.addClassCleanup(cls.temp.cleanup)
        cls.clock = Path(cls.temp.name) / "clock"
        cls.clock.write_text(str(int(time.time()*1000)))
        cls.env["HYJS_P14_PROMOTION_CLOCK"] = str(cls.clock)

    def counts(self, zone):
        return self.sql(zone, "SELECT (SELECT COUNT(*) FROM p14_promotion_fights),(SELECT COUNT(*) FROM p14_promotion_fight_offers),(SELECT COUNT(*) FROM p6_battle_records WHERE source='p14-promotion-scoped'),(SELECT COUNT(*) FROM p14_promotion_states),(SELECT COUNT(*) FROM p14_promotion_grants);")

    def test_original_match_scoped_result_point_debit_retry_and_cold(self):
        began = time.monotonic()
        self.shutdownTimes = []
        for zone in [1,2]:
            # Validate the canonical SQL148 schema/closed seed before application AutoMigrate.
            for script in sorted((self.source / "hyjs-server/db/sql").glob("148_*.sql"), key=lambda path:(path.name.endswith(".seed.sql"),path.name)):
                self.sql(zone,script.read_text())
            self.start(zone)
        uids = {z:["promotion-match-"+str(z)+"-"+secrets.token_hex(4) for _ in range(2)] for z in [1,2]}
        players = {z:[self.login(z,uid) for uid in uids[z]] for z in [1,2]}
        for zone in [1,2]:
            for i,player in enumerate(players[zone]):
                self.call(zone,"CSPlayerCreate",{"playerName":"晋级真赛"+str(zone)+str(i),"heroId":"hero_047"},player,2,"SCPlayerCreate")
            # Level is the finite isolated prerequisite. All eligibility and formation rows are produced by real commands.
            self.sql(zone,'UPDATE profiles SET player=JSON_SET(player,"$.level",70);UPDATE p10_scene_policies SET enabled=0;')
            for player in players[zone]:
                self.call(zone,"CSPlayer",{},player,3,"SCPlayer")
        now = int(time.time()*1000)
        self.clock.write_text(str(now))
        slots = {1:(now-2000,now-1000),2:(now-1000,now+10000),3:(now+10000,now+11000),4:(now+11000,now+25000),5:(now+25000,now+26000),6:(now+26000,now+27000),7:(now+27000,now+3*86400000)}
        for stage in range(8,20):
            at=slots[7][1]+(stage-8)*1000
            slots[stage]=(at,at+1000)
        values=','.join('(61,'+str(a)+','+str(stage)+','+str(b)+',"原阶段'+str(stage)+'","local isolated real qualification with internal noon clock")' for stage,(a,b) in slots.items())
        key=secrets.token_bytes(32)
        recovery="local-prepublished-zero-roundauto-downtime-recovery-v1; actual-qualifiedAt-observation; stage5-7; original-capacity1-10; no-round-or-match"
        for zone in [1,2]:
            self.sql(zone,'UPDATE p14_season_policies SET enabled=1,season_index=61,register_start='+str(now-1000)+',register_end='+str(now+10000)+';INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES'+values+';INSERT INTO p14_trusted_peers(server_id,enabled,endpoint,signing_key) VALUES('+str(3-zone)+',1,"http://127.0.0.1:'+str(self.ports[3-zone][0])+'",UNHEX("'+key.hex()+'"));UPDATE p14_audition_match_policies SET enabled=1,auto_recovery_enabled=1,auto_recovery_published_at='+str(now)+',auto_recovery_source="'+recovery+'";UPDATE p14_qualification_federation_policies SET enabled=1;UPDATE p14_promotion_policies SET enabled=1;')
            for player in players[zone]:
                self.call(zone,"CSKFEnterGame",{},player,400+player.playerId,"SCKFEnterGame")
        for zone in [1,2]:
            self.wait_sql(zone,"SELECT COUNT(*) FROM p14_foreign_qualifications WHERE season_index=61;","1",timeout=40)
        while int(time.time()*1000)<slots[7][0]+100:
            time.sleep(.1)
        self.clock.write_text(str(int(time.time()*1000)))
        chosen = {1:players[1][0],2:players[2][1]}
        self.assertNotEqual(chosen[1].playerId,chosen[2].playerId,"single original winnerId must remain unambiguous")
        for zone in [1,2]:
            self.call(zone,"CSKFEnterFrightGame",{},chosen[zone],600,"SCKFEnterFrightGame")
        for zone in [1,2]:
            self.wait_sql(zone,"SELECT COUNT(*) FROM p14_promotion_formations;","2")
        original = {z:self.sql(z,"SELECT source_hash FROM p14_audition_rosters ORDER BY player_id;") for z in [1,2]}
        before_profiles={z:self.profiles(z,players[z]) for z in [1,2]}
        # Produce an unsupported pose through the genuine formation command,
        # upload it through purpose2, and retain its actual signed source.
        with (self.source / "analysis/tables/TableFormation.tsv").open(encoding="utf-8-sig") as table:
            rows=list(csv.reader(table,delimiter="\t"))
        definition=next(dict(zip(rows[1],row)) for row in rows[5:] if row and row[0]=="1")
        positions=[int(value) for value in definition["posInfo"].split(",")]
        formation=self.call(1,"CSFormationList",{},chosen[1],610,"SCFormationList")
        active=[item for item in formation.formationList if any(thing.id.startswith("hero_") for thing in item.things)]
        self.assertTrue(active)
        restore_map=";".join(str(item.formatPos)+","+str(item.battlePos) for item in active)
        altered_map=";".join(str(item.formatPos)+","+str(positions[(positions.index(item.battlePos)+1)%len(positions)]) for item in active)
        self.call(1,"CSFormationChange",{"fomationMapStr":altered_map},chosen[1],611,"SCFormationChange")
        self.call(1,"CSKFEnterFrightGame",{},chosen[1],612,"SCKFEnterFrightGame")
        self.wait_sql(2,"SELECT revision FROM p14_promotion_formations WHERE season_index=61 AND server_id=1 AND player_id="+str(chosen[1].playerId)+";","2")
        unknown=CLASSES["BattlePlayer"]()
        unknown.ParseFromString(bytes.fromhex(self.sql(1,"SELECT HEX(battle) FROM p14_promotion_formations WHERE season_index=61 AND server_id=1 AND player_id="+str(chosen[1].playerId)+";").strip()))
        self.assertNotEqual(unknown.formation[0].pose,3)
        state=self.call(1,"CSKFOpenFrightServerInfo",{},chosen[1],700,"SCKFOpenFrightServerInfo")
        self.assertEqual((state.typeStage,state.isEnterCurStage),(7,1))
        # A future public HTTP stamp is only a request identity; it cannot advance the service clock.
        unchanged=self.call(1,"CSKFOpenFrightServerInfo",{},chosen[1],state.currentTime+86400000,"SCKFOpenFrightServerInfo")
        self.assertEqual(unchanged.currentTime,state.currentTime)
        eligible=max(slots[7][0],int(self.sql(1,"SELECT qualified_at FROM p14_audition_qualifications WHERE season_index=61;").strip()))
        local=dt.datetime.fromtimestamp(eligible/1000,dt.timezone(dt.timedelta(hours=8)))
        noon=local.replace(hour=12,minute=0,second=0,microsecond=0)
        if int(noon.timestamp()*1000)<eligible:
            noon+=dt.timedelta(days=1)
        at=int(noon.timestamp()*1000)+1
        self.clock.write_text(str(at))
        counts_before=self.counts(1)
        self.assertEqual(self.send(1,"CSKuafuMatchPlayer",{},chosen[1],702)[0],10001,"new match policy defaultoff")
        self.assertEqual(self.counts(1),counts_before)
        raw='{"matchCost":1,"maxRound":40,"winScore":15,"lossScore":0,"minimumScore":1}'
        source="local-prepublished-promotion-fight-v1; originalpoint1/max40/initialscore1; scopedtyped17; foreign-distinct-playerid; localwin15-loss0-no-powerbonus-min1"
        self.sql(1,"UPDATE p14_promotion_fight_policies SET enabled=1,version='local-promotion-score-v1',source='"+source+"',rules_json='"+raw+"',rules_hash='"+hashlib.sha256(raw.encode()).hexdigest()+"',published_at="+str(at-1)+";")
        for invalid in [0,-1]:
            self.assertEqual(self.send(1,"CSKuafuMatchPlayer",{},chosen[1],invalid)[0],10001)
        # Corrupt only the isolated published policy hash, then use the actual
        # unsupported formation. Neither may leave a new grant, capture or debit.
        self.sql(1,"UPDATE p14_promotion_fight_policies SET rules_hash='"+"0"*64+"';")
        self.assertEqual(self.send(1,"CSKuafuMatchPlayer",{},chosen[1],750)[0],10001)
        self.assertEqual(self.counts(1),counts_before)
        self.sql(1,"UPDATE p14_promotion_fight_policies SET rules_hash='"+hashlib.sha256(raw.encode()).hexdigest()+"';")
        self.assertEqual(self.send(1,"CSKuafuMatchPlayer",{},chosen[1],751)[0],10001)
        self.assertEqual(self.counts(1),counts_before)
        # Restore by the same original command while the real purpose2 peer
        # still uses wall time, before advancing only this test's business clock.
        self.clock.write_text(str(int(time.time()*1000)))
        self.call(1,"CSFormationChange",{"fomationMapStr":restore_map},chosen[1],613,"SCFormationChange")
        self.call(1,"CSKFEnterFrightGame",{},chosen[1],614,"SCKFEnterFrightGame")
        self.wait_sql(2,"SELECT revision FROM p14_promotion_formations WHERE season_index=61 AND server_id=1 AND player_id="+str(chosen[1].playerId)+";","3")
        self.clock.write_text(str(at))
        certified=os.environ.get("HYJS_P14_EXPECT_CROSS_CERTIFIED")=="1"
        if not certified:
            # Empty compiled proof must roll back even the newly captured input and first offer.
            for stamp in [800,800,801]:
                self.assertEqual(self.send(1,"CSKuafuMatchPlayer",{},chosen[1],stamp)[0],10001)
                self.assertEqual(self.counts(1),counts_before)
            self.assertEqual(self.call(1,"CSKFOpenFrightServerInfo",{},chosen[1],802,"SCKFOpenFrightServerInfo").mateTime,20)
            result={"status":"PASS_PENDING_PROOF_FULL_TRANSACTION_ROLLBACK","authority":False}
        else:
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                responses=list(pool.map(lambda _:self.call(1,"CSKuafuMatchPlayer",{},chosen[1],800,"SCKuafuMatchPlayer"),range(8)))
            first=responses[0]
            self.assertTrue(all(reply==first for reply in responses))
            self.assertEqual((first.state,first.lastMatchNum,len(first.playerList)),(0,19,2))
            self.assertIn(first.winPlayerId,[chosen[1].playerId,chosen[2].playerId])
            delta=15 if first.winPlayerId==chosen[1].playerId else 0
            self.assertEqual(first.changeScore,delta)
            self.assertEqual(first.playerList[0].inst.firstOrder,0)
            self.assertEqual(first.playerList[1].inst.firstOrder,0)
            self.assertFalse(first.HasField("roomId"))
            self.assertEqual(self.sql(1,"SELECT points_before,points_after,score_before,score_after,score_delta FROM p14_promotion_fights;").strip(),"20\t19\t1\t"+str(1+delta)+"\t"+str(delta))
            self.assertEqual(self.call(1,"CSKFOpenFrightServerInfo",{},chosen[1],803,"SCKFOpenFrightServerInfo").mateTime,19)
            self.clock.write_text(str(at+86400000))
            self.assertEqual(self.call(1,"CSKFOpenFrightServerInfo",{},chosen[1],805,"SCKFOpenFrightServerInfo").mateTime,20)
            self.assertEqual(self.sql(1,"SELECT COUNT(*) FROM p14_promotion_grants;").strip(),"2")
            self.assertEqual(self.sql(1,"SELECT score_after FROM p14_promotion_fights;").strip(),str(1+delta))
            self.sql(1,"UPDATE p14_promotion_fight_policies SET enabled=0;")
            self.assertEqual(self.call(1,"CSKuafuMatchPlayer",{},chosen[1],800,"SCKuafuMatchPlayer"),first)
            self.assertEqual(self.send(1,"CSKuafuMatchPlayer",{},chosen[1],804)[0],10001)
            fixed=self.ports[1]
            self.stop(1)
            self.start(1,fixed)
            self.assertEqual(self.call(1,"CSKuafuMatchPlayer",{},chosen[1],800,"SCKuafuMatchPlayer"),first)
            self.assertEqual(self.sql(1,"SELECT COUNT(*) FROM p14_promotion_fights;").strip(),"1")
            result={"status":"PASS_ACTUAL_SCOPED_TYPED17_POINT_RESULT_RETRY_COLD","authority":"only independently certified typed17 family","actualWinnerId":first.winPlayerId,"actualDelta":delta,"responseSHA256":hashlib.sha256(first.SerializeToString()).hexdigest()}
        for zone in [1,2]:
            self.assertEqual(self.sql(zone,"SELECT source_hash FROM p14_audition_rosters ORDER BY player_id;"),original[zone])
            self.assertEqual(self.profiles(zone,players[zone]),before_profiles[zone])
            self.stop(zone)
        result.update({"seconds":round(time.monotonic()-began,3),"clock":"real qualification then isolated internal business clock advanced to next original noon; no waiting-until-tomorrow claim; no q/grant/point/score fixture writes","nativeSHA256":self.meta["nativeSHA256"],"sourceProofSHA256":hashlib.sha256((self.prebuilt/"manifest.json").read_bytes()).hexdigest(),"shutdownSeconds":self.shutdownTimes,"productionModified":False,"phoneVerified":False,"cleanup":"class registered managed release runs after this report"})
        result["sqlAccountScope"]="both applications and every canonical DDL/fixture statement use the same manager allocated hyjs_t_ account; no development hyjs credential"
        result["managedHelperSHA256"]=hashlib.sha256((ROOT/"tests/hyjs_managed_mysql.py").read_bytes()).hexdigest()
        type(self).native_report=result
        (ROOT/"analysis/p14/promotion-match-native-validation.json").write_text(json.dumps(result,ensure_ascii=False,indent=2)+"\n")


if __name__=="__main__":
    unittest.main()
