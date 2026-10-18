"""Actual four qualifiers -> both genuine first-round series -> next-round sources.

Only finite level70 and prepublished local calendar/policy are setup. A natural
same-realm first-round draw remains pending without a matching independent KF
certificate. IDs, pairs, battle inputs and natural winners are never changed.
"""
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import secrets
import time
import unittest

import test_p14_pair_guess_award_mysql as prepared
from server.codec import CLASSES
import p14_next_round_formation_checks as next_formation
import p14_next_round_game_checks as next_game

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get("HYJS_P14_NEXT_ROUND_CHAIN_MYSQL_TEST")=="1","explicit managed two-parent next-round original chain")
class P14NextRoundActualMySQLTests(prepared.P14PairGuessAwardNativeMySQLTests):
    @classmethod
    def setUpClass(cls):
        inherited=ROOT/"analysis/p14/pair-guess-award-native-validation.json"
        saved=inherited.read_bytes() if inherited.exists() else None
        def record():
            report=dict(getattr(cls,"native_report",None) or {"status":"FAILED_BEFORE_CHAIN"})
            report["managedAllocationRemoved"]=not bool(getattr(getattr(cls,"managed_mysql",None),"connections",{}))
            if hasattr(cls,"realms"):report["managedConnectionFilesRemoved"]=all(not((Path.home() / '.codex/local-services/test-connections')/(db+".env")).exists() for db in cls.realms.values())
            (ROOT/"analysis/p14/next-round-actual-native-validation.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
            if saved is None:inherited.unlink(missing_ok=True)
            else:inherited.write_bytes(saved)
        cls.addClassCleanup(record)
        super().setUpClass()

    def run(self,result=None):
        result=super().run(result)
        if any(test is self for test,_ in result.failures+result.errors):
            getattr(type(self),"native_report",{})["status"]="FAILED_ACTUAL_NEXT_ROUND_CHAIN"
        return result

    def start(self,zone,fixed=None):
        if not hasattr(self,"next_initialized"):self.next_initialized=set()
        if zone not in self.next_initialized:
            for n in (171,173,175):
                for script in sorted((self.source/"hyjs-server/db/sql").glob(str(n)+"_*.sql"),key=lambda p:(p.name.endswith(".seed.sql"),p.name)):self.sql(zone,script.read_text())
            self.next_initialized.add(zone)
        return super().start(zone,fixed)

    def configure_additional_admission(self,now):
        # Extensions publish an independently named capability before signup.
        return

    def same_realm_admission_ready(self):
        return False

    def expects_next_result(self):
        return False

    def after_next_round_input(self,players,identities,offer,exported):
        return

    def checkpoint(self,phase,**kwargs):
        type(self).native_report.update(phase=phase,**kwargs)
        (ROOT/getattr(self,"checkpoint_relative_path","analysis/p14/next-round-actual-checkpoint.json")).write_text(json.dumps(type(self).native_report,ensure_ascii=False,indent=2)+"\n")

    def stop(self,zone):
        process=self.processes.pop(zone,None)
        if process is None:return
        began=time.monotonic()
        if process.poll() is None:process.terminate()
        try:output=process.communicate(timeout=10)[0]
        except __import__("subprocess").TimeoutExpired:
            process.kill();output=process.communicate()[0]
        safe=(output or "").replace(self.test_password,"[redacted]")[-16000:]
        if getattr(self,"signing_key",None):safe=safe.replace(self.signing_key.hex(),"[redacted]")
        with (ROOT/getattr(self,"runtime_relative_path","analysis/p14/next-round-actual-runtime.log")).open("a") as log:
            log.write("realm "+str(zone)+" exit "+str(process.returncode)+"\n"+safe+"\n")
        self.shutdownTimes.append(round(time.monotonic()-began,3))
        self.assertEqual(process.returncode,0,"owned native shutdown: "+safe[-2000:])

    def export_actual_initial_sources(self,players,draw):
        # Pending branches also retain every observed PB before managed cleanup.
        # Prior runs' assets are never reused as the current draw's instance.
        evidence=ROOT/("analysis/p14/next-parent-actual-purpose3-native-"+str(time.time_ns()))
        self.actual_source_evidence=str(evidence.relative_to(ROOT))
        evidence.mkdir(exist_ok=False)
        members=[]
        for zone in (1,2):
            for owner in players[zone]:
                where=" WHERE server_id="+str(zone)+" AND player_id="+str(owner.playerId)
                fields="'serverId',server_id,'playerId',player_id,'seasonIndex',season_index,'roundStage',round_stage,'revision',revision,'capturedAt',captured_at,'configurationHash',configuration_hash,'engineVersion',engine_version,'clientVersion',client_version,'qualificationHash',qualification_hash,'offerHash',offer_hash,'sourceHash',source_hash,'snapshotHash',snapshot_hash"
                metadata=json.loads(self.sql(zone,"SELECT JSON_OBJECT("+fields+") FROM p14_final_formations"+where+";").strip())
                stem="realm"+str(zone)+"-player"+str(owner.playerId)
                for column,suffix in (("battle","BattlePlayer.pb"),("snapshot","FormationSimpleInfo.pb")):
                    raw=bytes.fromhex(self.sql(zone,"SELECT HEX("+column+") FROM p14_final_formations"+where+";").strip())
                    name=stem+"-"+suffix;(evidence/name).write_bytes(raw)
                    metadata[column+"File"]=name;metadata[column+"SHA256"]=hashlib.sha256(raw).hexdigest()
                members.append(metadata)
        (evidence/"manifest.json").write_text(json.dumps({"scope":"this actual four-qualifier purpose3 and natural draw only; no played/winner claim","members":members,"actualDraw":draw,"nativeSHA256":self.meta["nativeSHA256"],"sourceProofSHA256":hashlib.sha256((self.prebuilt/"manifest.json").read_bytes()).hexdigest()},ensure_ascii=False,indent=2)+"\n")

    def test_original_match_scoped_result_point_debit_retry_and_cold(self):
        began=time.monotonic();self.shutdownTimes=[]
        type(self).native_report={"status":"RUNNING_TRUE_PARENT_SERIES_NEXT_ROUND_INPUT","productionModified":False}
        for zone in (1,2):
            for script in sorted((self.source/"hyjs-server/db/sql").glob("148_*.sql"),key=lambda p:(p.name.endswith(".seed.sql"),p.name)):self.sql(zone,script.read_text())
            self.start(zone)
        players={zone:[] for zone in (1,2)}
        for zone in (1,2):
            for index in range(2):
                login=self.login(zone,"same-realm-source-"+str(zone)+"-"+str(index)+"-"+secrets.token_hex(4))
                self.call(zone,"CSPlayerCreate",{"playerName":"同服真源"+str(zone)+str(index),"heroId":"hero_047"},login,2,"SCPlayerCreate")
                players[zone].append(login)
            self.sql(zone,'UPDATE profiles SET player=JSON_SET(player,"$.level",70);UPDATE p10_scene_policies SET enabled=0;')
            for owner in players[zone]:self.call(zone,"CSPlayer",{},owner,3,"SCPlayer")
        now=int(time.time()*1000);six_start=now+26000;six_end=now+3*86400000
        slots={1:(now-2000,now-1000),2:(now-1000,now+10000),3:(now+10000,now+11000),4:(now+11000,now+25000),5:(now+25000,six_start),6:(six_start,six_end),7:(six_end,six_end+600000),8:(six_end+600000,six_end+600000+6*3600000)}
        slots[9]=(slots[8][1],slots[8][1]+600000)
        slots[10]=(slots[9][1],slots[9][1]+6*3600000)
        for stage in range(11,20):slots[stage]=(slots[10][1]+(stage-11)*1000,slots[10][1]+(stage-10)*1000)
        values=','.join('(61,'+str(a)+','+str(stage)+','+str(b)+',"actual same-realm source stage","local initial publication before all signups")' for stage,(a,b) in slots.items())
        key=secrets.token_bytes(32)
        self.signing_key=key
        specs=[("p14_promotion_rank_policies","p14PromotionRank.go","P14PromotionRankSource"),("p14_final_qualification_policies","p14FinalQualification.go","P14FinalQualificationSource"),("p14_final_formation_policies","p14FinalFormation.go","P14FinalFormationSource"),("p14_final_draw_policies","p14FinalDraw.go","P14FinalDrawSource")]
        specs += [("p14_per_game_policies","p14PerGame.go","P14PerGameSource"),("p14_final_continuation_policies","p14FinalContinuation.go","P14FinalContinuationSource"),("p14_final_game_policies","p14FinalGame.go","P14FinalGameSource"),("p14_final_series_policies","p14FinalSeries.go","P14FinalSeriesSource"),("p14_final_settlement_policies","p14FinalSettlement.go","P14FinalSettlementSource"),("p14_final_result_policies","p14FinalResult.go","P14FinalResultSource"),("p14_final_bracket_policies","p14FinalBracket.go","P14FinalBracketSource"),("p14_next_round_game_policies","p14NextRoundGame.go","P14NextRoundGameSource")]
        recovery="local-prepublished-zero-roundauto-downtime-recovery-v1; actual-qualifiedAt-observation; stage5-7; original-capacity1-10; no-round-or-match"
        rules='{"matchCost":1,"maxRound":40,"winScore":15,"lossScore":0,"minimumScore":1}'
        fight_source=self.source_constant("p14PromotionFight.go","P14PromotionFightSource")
        for zone in (1,2):
            self.sql(zone,'UPDATE p14_season_policies SET enabled=1,season_index=61,register_start='+str(now-1000)+',register_end='+str(now+10000)+';INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES'+values+';INSERT INTO p14_trusted_peers(server_id,enabled,endpoint,signing_key) VALUES('+str(3-zone)+',1,"http://127.0.0.1:'+str(self.ports[3-zone][0])+'",UNHEX("'+key.hex()+'"));UPDATE p14_audition_match_policies SET enabled=1,auto_recovery_enabled=1,auto_recovery_published_at='+str(now)+',auto_recovery_source="'+recovery+'";UPDATE p14_qualification_federation_policies SET enabled=1;UPDATE p14_promotion_policies SET enabled=1;')
            for table,file,name in specs:self.sql(zone,"UPDATE "+table+" SET enabled=1,version='actual-four-qualifier-source-v1',source='"+self.source_constant(file,name)+"',published_at="+str(now-1000)+" WHERE id=1;")
            self.sql(zone,"UPDATE p14_promotion_fight_policies SET enabled=1,version='actual-four-qualifier-source-score-v1',source='"+fight_source+"',rules_json='"+rules+"',rules_hash='"+hashlib.sha256(rules.encode()).hexdigest()+"',published_at="+str(now)+";")
        self.configure_additional_admission(now)
        for zone in (1,2):
            for owner in players[zone]:self.call(zone,"CSKFEnterGame",{},owner,100,"SCKFEnterGame")
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_foreign_qualifications WHERE season_index=61;","1",timeout=40)
        while int(time.time()*1000)<six_start+100:time.sleep(.1)
        for zone in (1,2):
            for owner in players[zone]:self.call(zone,"CSKFEnterFrightGame",{},owner,110,"SCKFEnterFrightGame")
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_promotion_formations;","4")
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
            self.assertEqual(self.sql(zone,"SELECT JSON_LENGTH(CONVERT(content USING utf8mb4),'$.ranking') FROM p14_final_qualifications;").strip(),"4")
        for zone in (1,2):
            for owner in players[zone]:self.call(zone,"CSKFEnterFrightGame",{},owner,160,"SCKFEnterFrightGame")
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_formations;","4")
        for zone in (1,2):self.call(zone,"CSKFMatchFrightList",{"groupId":"A"},players[zone][0],180,"SCKFMatchFrightList")
        actual_draw=json.loads(bytes.fromhex(self.sql(1,"SELECT HEX(content) FROM p14_final_draws;").strip()))
        self.assertEqual(len(actual_draw["entrants"]),4)
        self.assertEqual(len(actual_draw["pairs"]),2)
        self.export_actual_initial_sources(players,actual_draw)
        self.checkpoint("REAL_FOUR_QUALIFIERS_PURPOSE3_DRAW",actualDraw=actual_draw)
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
            self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_series WHERE left_wins+right_wins=1;","2",timeout=45)
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
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_series WHERE state=3 AND (left_wins=3 OR right_wins=3);","2")
        histories={zone:self.sql(zone,"SELECT id,manifest_hash,left_wins,right_wins,winner_server_id,winner_player_id FROM p14_final_series ORDER BY pair_index;").strip() for zone in (1,2)}
        self.assertEqual(histories[1],histories[2])
        self.checkpoint("BOTH_TRUE_THREE_WIN_SERIES_COMPLETE",parentHistories=histories,played=played)
        self.clock.write_text(str(slots[9][0]+1))
        for zone in (1,2):self.wait_sql(zone,"SELECT COUNT(*) FROM p14_final_bracket_offers;","1")
        offer_hash=self.sql(1,"SELECT content_hash FROM p14_final_bracket_offers;").strip()
        offer=json.loads(bytes.fromhex(self.sql(1,"SELECT HEX(content) FROM p14_final_bracket_offers;").strip()))
        self.assertEqual((offer["roundStage"],len(offer["parents"]),len(offer["entrants"])),(10,2,2))
        qualifiers=[]
        for i,entry in enumerate(offer["entrants"]):
            realm,pid=entry["serverId"],entry["playerId"]
            owner=identities[(realm,pid)]
            state=self.call(realm,"CSKFOpenFrightServerInfo",{},owner,900+i*20,"SCKFOpenFrightServerInfo")
            self.assertEqual((state.typeStage,state.isEnterCurStage),(9,0))
            qualifiers.append(next_formation.capture_actual(self,realm,owner,901+i*20,offer_hash))
        for i,(entry,source) in enumerate(zip(offer["entrants"],qualifiers)):
            realm,pid=entry["serverId"],entry["playerId"]
            next_formation.assert_peer_and_cold(self,realm,identities[(realm,pid)],players[3-realm][0],source,950+i*10)
        second_views=[self.call(zone,"CSKFMatchFrightList",{"groupId":"A"},players[zone][0],980,"SCKFMatchFrightList") for zone in (1,2)]
        self.assertEqual(second_views[0],second_views[1])
        self.assertEqual(len(second_views[0].kfMatchAllRandList.kFMatchSecondList),2)
        self.checkpoint("TRUE_NEXT_QUALIFICATION_AND_FRESH173_SYNCED",nextOffer=offer,qualifiers=qualifiers)
        exported=next_game.freeze_and_export_actual(self,offer_hash,ROOT/"analysis/p14/next-round-actual-game1-native")
        self.after_next_round_input(players,identities,offer,exported)
        for zone in (1,2):
            self.assertEqual(self.sql(zone,"SELECT COUNT(*) FROM p14_next_round_games;").strip(),"1")
            if not self.expects_next_result():
                self.assertEqual(self.sql(zone,"SELECT COUNT(*) FROM p6_battle_records WHERE source LIKE 'p14-next%';").strip(),"0")
            self.stop(zone)
        type(self).native_report={"status":"PASS_TRUE_TWO_PARENT_SERIES_NEXT_QUALIFICATION_FRESH173_SEAL175_GAME1_COLD","seconds":round(time.monotonic()-began,3),"actualDraw":actual_draw,"parentHistories":histories,"played":played,"nextOffer":offer,"qualifiers":qualifiers,"nextGame":exported,"nativeSHA256":self.meta["nativeSHA256"],"sourceProofSHA256":hashlib.sha256((self.prebuilt/"manifest.json").read_bytes()).hexdigest(),"finiteSetup":"level70 and named prepublished calendar/policies only; no qualification/score/winner/source rows inserted","clock":"real eligibility then isolated authenticated internal clock; no actual waiting-days claim","nextGameScope":"unplayed true175input only; no next winner or same-realm authority","productionModified":False,"phoneVerified":False,"shutdownSeconds":self.shutdownTimes,"nextSeriesChecks":getattr(self,"next_series_checks",None)}

if __name__=="__main__":unittest.main()
