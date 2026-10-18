"""One future8qualifier full actual prefix -> real177 parents ->180 round12.

Source preparation only until Root grants the SQL lease and an independently
certified same-realm capability is pinned. No known-empty-proof run is started.
"""
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import time
import unittest
import test_p14_next_result_mysql as prepared
import p14_final_rounds_checks as round_checks
import p14_final_rounds_diagnostics as diagnostics

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get("HYJS_P14_FINAL_ROUNDS_MYSQL_TEST")=="1","explicit managed8qualifier remaining-round chain")
class P14FinalRoundsNativeMySQLTests(prepared.P14NextResultNativeMySQLTests):
    __unittest_skip__=os.environ.get("HYJS_P14_FINAL_ROUNDS_MYSQL_TEST")!="1"
    checkpoint_relative_path="analysis/p14/final-rounds-native-checkpoint.json"
    runtime_relative_path="analysis/p14/final-rounds-native-runtime.log"

    def wait_sql(self,zone,query,want,timeout=25):
        # Keep the precise owned read that failed, without sampling a new draw
        # or changing any producer/result while the worker is still pending.
        # Eight real entrants create several independently signed deliveries.
        # Keep a bounded async wait within the original five-minute lock window;
        # a short polling fixture deadline cannot turn healthy pending into loss.
        budget=max(timeout,180)
        began=time.monotonic()
        try:
            return super().wait_sql(zone,query,want,budget)
        except AssertionError:
            self.pending_read={"realm":zone,"read":query,"expected":want,"timeoutSeconds":budget}
            getattr(type(self),"native_report",{})["pendingRead"]=self.pending_read
            raise
        finally:
            elapsed=round(time.monotonic()-began,3)
            if elapsed>25:
                getattr(type(self),"native_report",{}).setdefault("slowActualReads",[]).append({"realm":zone,"read":query,"expected":want,"seconds":elapsed,"budget":budget})

    def tearDown(self):
        # unittest invokes this before registered stop/drop cleanups. Preserve
        # genuine pending inputs and delivery journals even on an assertion.
        try:
            path=diagnostics.export(self,ROOT,"before managed cleanup")
            getattr(type(self),"native_report",{})["preCleanupEvidence"]=path
            if getattr(self,"pending_read",None) and os.environ.get("HYJS_P14_SIGNED_TRANSPORT_PROBE")=="1":
                probe=diagnostics.probe_signed_transport(self,ROOT)
                getattr(type(self),"native_report",{})["signedTransportProbe"]=probe
        finally:
            super().tearDown()

    @classmethod
    def setUpClass(cls):
        source=Path(os.environ["HYJS_P14_PROMOTION_MATCH_SOURCE"])
        text=(source/"hyjs-server/constUtil/p14KFSameRealm.go").read_text()
        for key in ("P14KFSameRealmNativeProof","P14KFSameRealmWrapperManifest","P14KFSameRealmWrapperDLL"):
            value=re.search(r'\b'+key+r'\s*=\s*"([^\"]*)"',text)[1]
            if not re.fullmatch(r"[0-9a-f]{64}",value):raise unittest.SkipTest("independent actual same-realm proof/wrapper pending before allocation")
        def record():
            report=dict(getattr(cls,"native_report",None) or {"status":"FAILED_BEFORE_ROUND12_CHAIN"})
            report["managedAllocationRemoved"]=not bool(getattr(getattr(cls,"managed_mysql",None),"connections",{}))
            (ROOT/"analysis/p14/final-rounds-native-validation.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        cls.addClassCleanup(record)
        super().setUpClass()

    def start(self,zone,fixed=None):
        if not hasattr(self,"rounds_initialized"):self.rounds_initialized=set()
        if zone not in self.rounds_initialized:
            for script in sorted((self.source/"hyjs-server/db/sql").glob("180_*.sql"),key=lambda p:(p.name.endswith(".seed.sql"),p.name)):self.sql(zone,script.read_text())
            self.rounds_initialized.add(zone)
        return super().start(zone,fixed)

    def configure_additional_admission(self,now):
        super().configure_additional_admission(now)
        self.assertTrue(self.same_realm_admission_ready(),"independent capability already checked before allocation")
        for realm in (1,2):
            self.sql(realm,"UPDATE p14_final_round_policies SET enabled=1,version='actual-eight-qualifier-remaining-rounds-v1',source='"+self.source_constant("p14FinalRounds.go","P14FinalRoundsSource")+"',published_at="+str(now-1000)+" WHERE id=1;")

    def test_original_match_scoped_result_point_debit_retry_and_cold(self):
        began=time.monotonic();self.shutdownTimes=[]
        type(self).native_report={"status":"RUNNING_EIGHT_ACTUAL_QUALIFIERS_THROUGH_ROUND12","productionModified":False}
        for zone in (1,2):
            for script in sorted((self.source/"hyjs-server/db/sql").glob("148_*.sql"),key=lambda p:(p.name.endswith(".seed.sql"),p.name)):self.sql(zone,script.read_text())
            self.start(zone)
        players={zone:[] for zone in (1,2)}
        for zone in (1,2):
            for index in range(4):
                login=self.login(zone,"same-realm-source-"+str(zone)+"-"+str(index)+"-"+secrets.token_hex(4))
                self.call(zone,"CSPlayerCreate",{"playerName":"同服真源"+str(zone)+str(index),"heroId":"hero_047"},login,2,"SCPlayerCreate")
                players[zone].append(login)
            self.sql(zone,'UPDATE profiles SET player=JSON_SET(player,"$.level",70);UPDATE p10_scene_policies SET enabled=0;')
            for owner in players[zone]:self.call(zone,"CSPlayer",{},owner,3,"SCPlayer")
        now=int(time.time()*1000);six_start=now+91000;six_end=now+3*86400000
        # Original audition boundary workers use real wall time. Keep their
        # genuine registration/qualification clock and allow managed setup time.
        self.clock.write_text("0")
        slots={1:(now-2000,now-1000),2:(now-1000,now+60000),3:(now+60000,now+61000),4:(now+61000,now+90000),5:(now+90000,six_start),6:(six_start,six_end),7:(six_end,six_end+600000),8:(six_end+600000,six_end+600000+6*3600000)}
        slots[9]=(slots[8][1],slots[8][1]+600000)
        slots[10]=(slots[9][1],slots[9][1]+6*3600000)
        cursor=slots[10][1]
        for stage in range(11,20):
            duration=600000 if stage%2 else 6*3600000
            slots[stage]=(cursor,cursor+duration);cursor+=duration
        values=','.join('(61,'+str(a)+','+str(stage)+','+str(b)+',"actual same-realm source stage","local initial publication before all signups")' for stage,(a,b) in slots.items())
        key=secrets.token_bytes(32)
        self.signing_key=key
        specs=[("p14_promotion_rank_policies","p14PromotionRank.go","P14PromotionRankSource"),("p14_final_qualification_policies","p14FinalQualification.go","P14FinalQualificationSource"),("p14_final_formation_policies","p14FinalFormation.go","P14FinalFormationSource"),("p14_final_draw_policies","p14FinalDraw.go","P14FinalDrawSource")]
        specs += [("p14_per_game_policies","p14PerGame.go","P14PerGameSource"),("p14_final_continuation_policies","p14FinalContinuation.go","P14FinalContinuationSource"),("p14_final_game_policies","p14FinalGame.go","P14FinalGameSource"),("p14_final_series_policies","p14FinalSeries.go","P14FinalSeriesSource"),("p14_final_settlement_policies","p14FinalSettlement.go","P14FinalSettlementSource"),("p14_final_result_policies","p14FinalResult.go","P14FinalResultSource"),("p14_final_bracket_policies","p14FinalBracket.go","P14FinalBracketSource"),("p14_next_round_game_policies","p14NextRoundGame.go","P14NextRoundGameSource")]
        recovery="local-prepublished-zero-roundauto-downtime-recovery-v1; actual-qualifiedAt-observation; stage5-7; original-capacity1-10; no-round-or-match"
        rules='{"matchCost":1,"maxRound":40,"winScore":15,"lossScore":0,"minimumScore":1}'
        fight_source=self.source_constant("p14PromotionFight.go","P14PromotionFightSource")
        for zone in (1,2):
            self.sql(zone,'UPDATE p14_season_policies SET enabled=1,season_index=61,register_start='+str(now-1000)+',register_end='+str(now+60000)+';INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES'+values+';INSERT INTO p14_trusted_peers(server_id,enabled,endpoint,signing_key) VALUES('+str(3-zone)+',1,"http://127.0.0.1:'+str(self.ports[3-zone][0])+'",UNHEX("'+key.hex()+'"));UPDATE p14_audition_match_policies SET enabled=1,auto_recovery_enabled=1,auto_recovery_published_at='+str(now)+',auto_recovery_source="'+recovery+'";UPDATE p14_qualification_federation_policies SET enabled=1;UPDATE p14_promotion_policies SET enabled=1;')
            for table,file,name in specs:self.sql(zone,"UPDATE "+table+" SET enabled=1,version='actual-eight-qualifier-source-v1',source='"+self.source_constant(file,name)+"',published_at="+str(now-1000)+" WHERE id=1;")
            self.sql(zone,"UPDATE p14_promotion_fight_policies SET enabled=1,version='actual-eight-qualifier-source-score-v1',source='"+fight_source+"',rules_json='"+rules+"',rules_hash='"+hashlib.sha256(rules.encode()).hexdigest()+"',published_at="+str(now)+";")
        self.configure_additional_admission(now)
        for zone in (1,2):
            for owner in players[zone]:self.call(zone,"CSKFEnterGame",{},owner,100,"SCKFEnterGame")
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_foreign_qualifications WHERE season_index=61;","1",timeout=120)
        while int(time.time()*1000)<six_start+100:time.sleep(.1)
        for zone in (1,2):
            for owner in players[zone]:self.call(zone,"CSKFEnterFrightGame",{},owner,110,"SCKFEnterFrightGame")
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_promotion_formations;","8")
        eligible=max(six_start,int(self.sql(1,"SELECT qualified_at FROM p14_audition_qualifications;").strip()))
        local=dt.datetime.fromtimestamp(eligible/1000,dt.timezone(dt.timedelta(hours=8)));noon=local.replace(hour=12,minute=0,second=0,microsecond=0)
        if int(noon.timestamp()*1000)<eligible:noon+=dt.timedelta(days=1)
        self.clock.write_text(str(int(noon.timestamp()*1000)+1))
        for zone in (1,2):
            for owner in players[zone]:self.assertEqual(self.call(zone,"CSKFOpenFrightServerInfo",{},owner,120,"SCKFOpenFrightServerInfo").mateTime,20)
        promotion=self.call(1,"CSKuafuMatchPlayer",{},players[1][0],130,"SCKuafuMatchPlayer")
        self.assertEqual((promotion.state,promotion.lastMatchNum,len(promotion.playerList)),(0,19,2))
        self.clock.write_text(str(six_end+1))
        # The original stage panel is the real lazy qualification trigger.
        # Wait for actual signed final score versions, then request the same
        # authenticated panel a real client opens; never insert a certificate.
        for zone in (1,2):
            self.wait_sql(zone,"SELECT COUNT(DISTINCT server_id) FROM p14_promotion_rank_versions WHERE season_index=61 AND JSON_UNQUOTE(JSON_EXTRACT(CONVERT(content USING utf8mb4),'$.closed'))='true';","2",timeout=40)
            state=self.call(zone,"CSKFOpenFrightServerInfo",{},players[zone][0],150,"SCKFOpenFrightServerInfo")
            self.assertEqual((state.typeStage,state.isEnterCurStage),(7,0))
            self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_qualifications;","1",timeout=40)
            self.assertEqual(self.sql(zone,"SELECT JSON_LENGTH(CONVERT(content USING utf8mb4),'$.ranking') FROM p14_final_qualifications;").strip(),"8")
        for zone in (1,2):
            for owner in players[zone]:self.call(zone,"CSKFEnterFrightGame",{},owner,160,"SCKFEnterFrightGame")
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_formations;","8")
        for zone in (1,2):self.call(zone,"CSKFMatchFrightList",{"groupId":"A"},players[zone][0],180,"SCKFMatchFrightList")
        actual_draw=json.loads(bytes.fromhex(self.sql(1,"SELECT HEX(content) FROM p14_final_draws;").strip()))
        self.assertEqual(len(actual_draw["entrants"]),8)
        self.assertEqual(len(actual_draw["pairs"]),4)
        self.export_actual_initial_sources(players,actual_draw)
        self.checkpoint("REAL_EIGHT_QUALIFIERS_PURPOSE3_DRAW",actualDraw=actual_draw)
        if any(pair["leftServerId"]==pair["rightServerId"] for pair in actual_draw["pairs"]) and not self.same_realm_admission_ready():
            # Preserve the natural draw; the existing181 certificate explicitly
            # excludes same-realm. No seed, identity or opponent is changed.
            for zone in (1,2):
                self.assertEqual(self.sql(zone,"SELECT COUNT(*) FROM p14_final_settlements;SELECT COUNT(*) FROM p14_final_continuation_results;SELECT COUNT(*) FROM p14_next_round_games;").strip(),"0\n0\n0")
                self.stop(zone)
            type(self).native_report.update(status="PENDING_NATURAL_SAME_REALM_FIRST_PAIR_CERTIFICATE",seconds=round(time.monotonic()-began,3),scope="actual four qualification/purpose3/draw preserved; no KF computation or next-round eligibility claim",shutdownSeconds=self.shutdownTimes)
            return
        identities={(zone,owner.playerId):owner for zone in (1,2) for owner in players[zone]}
        first=int(self.sql(1,"SELECT fight_start FROM p14_final_formation_offers;").strip())
        lock=int(self.sql(1,"SELECT lock_at FROM p14_final_formation_offers;").strip())
        self.clock.write_text(str(lock+1))
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_seals;","2")
        self.clock.write_text(str(first+1))
        for zone in (1,2):
            self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_series WHERE left_wins+right_wins=1;","4",timeout=45)
        self.checkpoint("BOTH_REAL_FIRST_GAMES_CERTIFIED")
        played=[]
        for game in range(2,6):
            raw=self.sql(1,"SELECT JSON_OBJECT('id',id,'state',state,'manifestHEX',HEX(manifest)) FROM p14_final_series ORDER BY pair_index;").strip()
            summaries=[json.loads(line) for line in raw.splitlines()]
            active=[row for row in summaries if row["state"]!=3]
            if not active:break
            self.clock.write_text(str(first+(game-2)*3600000+1000))
            for row in active:
                manifest=json.loads(bytes.fromhex(row["manifestHEX"]));pair=manifest["pair"]
                for server_key,player_key in (("leftServerId","leftPlayerId"),("rightServerId","rightPlayerId")):
                    realm,pid=pair[server_key],pair[player_key]
                    self.call(realm,"CSKFEnterFrightGame",{},identities[(realm,pid)],300+game,"SCKFEnterFrightGame")
            for zone in (1,2):
                self.wait_sql(zone,"SELECT COUNT(*) FROM p14_per_game_formations WHERE game_index="+str(game)+";",str(2*len(active)))
            self.clock.write_text(str(first+(game-1)*3600000-300000+1))
            required_origins=sum(len({json.loads(bytes.fromhex(row["manifestHEX"]))["pair"]["leftServerId"],json.loads(bytes.fromhex(row["manifestHEX"]))["pair"]["rightServerId"]}) for row in active)
            for zone in (1,2):
                self.wait_sql(zone,"SELECT COUNT(*) FROM p14_per_game_seals WHERE game_index="+str(game)+";",str(required_origins))
            self.clock.write_text(str(first+(game-1)*3600000+1))
            for zone in (1,2):
                self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_series_slots WHERE game_index="+str(game)+" AND state=3;",str(len(active)),timeout=45)
            played.append({"game":game,"actualPairs":len(active)})
            self.checkpoint("BOTH_PARENT_SERIES_ACTUAL_SLOT_"+str(game),played=played)
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_series WHERE state=3 AND (left_wins=3 OR right_wins=3);","4")
        histories={zone:self.sql(zone,"SELECT id,manifest_hash,left_wins,right_wins,winner_server_id,winner_player_id FROM p14_final_series ORDER BY pair_index;").strip() for zone in (1,2)}
        self.assertEqual(histories[1],histories[2])
        self.checkpoint("BOTH_TRUE_THREE_WIN_SERIES_COMPLETE",parentHistories=histories,played=played)
        self.clock.write_text(str(slots[9][0]+1))
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_bracket_offers;","2")
        actual_offers=[json.loads(bytes.fromhex(line)) for line in self.sql(1,"SELECT HEX(content) FROM p14_final_bracket_offers ORDER BY pair_index;").strip().splitlines()]
        self.assertEqual([o["pairIndex"] for o in actual_offers],[0,1])
        for position,offer in enumerate(actual_offers):
            self.assertEqual((offer["roundStage"],len(offer["parents"]),len(offer["entrants"])),(10,2,2))
            for index,entry in enumerate(offer["entrants"]):
                realm,pid=entry["serverId"],entry["playerId"]
                self.call(realm,"CSKFEnterFrightGame",{},identities[(realm,pid)],5000+position*100+index,"SCKFEnterFrightGame")
        # All real parents share the original round clock; capture every active
        # pair before advancing any lock, rather than replaying a prefix twice.
        lock=actual_offers[0]["lockAt"];next_start=actual_offers[0]["fightStart"]
        self.clock.write_text(str(lock))
        for offer in actual_offers:
            # Read the authoritative saved hash instead of guessing Go JSON order.
            offer_hash=self.sql(1,"SELECT content_hash FROM p14_final_bracket_offers WHERE pair_index="+str(offer["pairIndex"])+";").strip()
            expected=len({e["serverId"] for e in offer["entrants"]})
            for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_next_round_game_seals WHERE offer_hash='"+offer_hash+"';",str(expected))
        self.clock.write_text(str(next_start+1))
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_next_series WHERE left_wins+right_wins=1;","2",timeout=45)
        next_played=[]
        for game in range(2,6):
            rows=[json.loads(line) for line in self.sql(1,"SELECT JSON_OBJECT('id',id,'state',state,'manifestHEX',HEX(manifest)) FROM p14_next_series ORDER BY pair_index;").strip().splitlines()]
            active=[row for row in rows if row["state"]!=3]
            if not active:break
            self.clock.write_text(str(next_start+(game-2)*3600000+1000))
            required=0
            for row in active:
                manifest=json.loads(bytes.fromhex(row["manifestHEX"]));pair=manifest["pair"]
                required+=len({pair["leftServerId"],pair["rightServerId"]})
                for realm,pid in ((pair["leftServerId"],pair["leftPlayerId"]),(pair["rightServerId"],pair["rightPlayerId"])):
                    self.call(realm,"CSKFEnterFrightGame",{},identities[(realm,pid)],6000+game*100+realm*10+pid,"SCKFEnterFrightGame")
            for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_next_slot_formations WHERE game_index="+str(game)+";",str(2*len(active)))
            self.clock.write_text(str(next_start+(game-1)*3600000-300000))
            for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_next_slot_seals WHERE game_index="+str(game)+";",str(required))
            self.clock.write_text(str(next_start+(game-1)*3600000+1))
            for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_next_series_slots WHERE game_index="+str(game)+" AND state=3;",str(len(active)),timeout=45)
            next_played.append({"gameIndex":game,"actualPairs":len(active)})
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_next_series WHERE state=3 AND (left_wins=3 OR right_wins=3);","2")
        self.checkpoint("TWO_ACTUAL_177_PARENTS_COMPLETE",actualDraw=actual_draw,round10Slots=next_played)
        self.clock.write_text(str(slots[11][0]+1))
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_round_offers WHERE round_stage=12;","1")
        series_id=self.sql(1,"SELECT id FROM p14_final_round_series WHERE round_stage=12;").strip()
        for zone in (1,2):self.assertEqual(self.sql(zone,"SELECT id FROM p14_final_round_series WHERE round_stage=12;").strip(),series_id)
        new_offer=json.loads(bytes.fromhex(self.sql(1,"SELECT HEX(content) FROM p14_final_round_offers WHERE round_stage=12;").strip()))
        for entry in new_offer["entrants"]:
            realm,pid=entry["serverId"],entry["playerId"]
            opened=self.call(realm,"CSKFOpenFrightServerInfo",{},identities[(realm,pid)],8000+realm*10+pid,"SCKFOpenFrightServerInfo")
            self.assertEqual((opened.typeStage,opened.isEnterCurStage),(11,0))
        evidence=ROOT/("analysis/p14/final-round12-actual-native-"+str(time.time_ns()))
        actual=round_checks.run_actual(self,players,identities,series_id,evidence)
        pair=new_offer["entrants"];final=actual["final"]
        for zone in (1,2):
            ui=self.call(zone,"CSKFMatchFrightList",{"groupId":"A"},players[zone][0],9000+zone,"SCKFMatchFrightList")
            self.assertEqual((len(ui.kfMatchAllRandList.kFMatchFirstList),len(ui.kfMatchAllRandList.kFMatchSecondList),len(ui.kfMatchAllRandList.kFMatchThirdList)),(8,4,2))
            self.assertEqual([(entry.serverId,entry.playerId,entry.winTime) for entry in ui.kfMatchAllRandList.kFMatchThirdList],[(pair[0]["serverId"],pair[0]["playerId"],final["leftWins"]),(pair[1]["serverId"],pair[1]["playerId"],final["rightWins"])])
        self.clock.write_text(str(slots[13][0]+1))
        for zone in (1,2):
            self.assertEqual(self.sql(zone,"SELECT COUNT(*) FROM p14_final_round_offers WHERE round_stage>=14;").strip(),"0")
            self.assertEqual(self.send(zone,"CSKFMatchFrightList",{},players[zone][0],9100+zone)[0],10001)
            self.stop(zone)
        type(self).native_report.update(status="PASS_EIGHT_REAL_QUALIFIERS_TO_TRUE_ROUND12_THREE_WIN_COLD",seconds=round(time.monotonic()-began,3),actualDraw=actual_draw,round10Slots=next_played,round12=actual,evidence=str(evidence.relative_to(ROOT)),finiteSetup="finite level70 and named calendar/policy before signup; no source/score/result rows inserted",clock="actual wallclock signup/small-roster qualification then isolated Noon/remaining-stage business clock; no actual waiting-days claim",futureRounds="missing remaining group parents and unknown original semifinal mapping stay pending",nativeSHA256=self.meta["nativeSHA256"],sourceProofSHA256=hashlib.sha256((self.prebuilt/"manifest.json").read_bytes()).hexdigest(),productionModified=False,phoneVerified=False)

if __name__=="__main__":unittest.main()
