"""Reuse actual Final154 preparation for real original81046 zero-filler groups.

Run only with Root's managed SQL window and the separately fixed155 binary.
This does not claim original hidden draw parameters, bye wins, series results
or phone UI acceptance; Root owns the reviewed safe-empty-slot DLL deployment.
"""
import json
import os
from pathlib import Path
import re
import unittest

import test_p14_finals_formation_mysql as formation

ROOT = Path(__file__).resolve().parents[1]
DRAW_SOURCE = "local-complete-final32-common-hash-draw-v1; original81046-groups-A-D; real-eight-slot-slices-adjacent-pairs; no-filler-no-bye-result"


@unittest.skipUnless(os.environ.get("HYJS_P14_FINALS_DRAW_MYSQL_TEST") == "1", "explicit managed final draw native")
class P14FinalsDrawNativeMySQLTests(formation.P14FinalsFormationNativeMySQLTests):
    @classmethod
    def setUpClass(cls):
        if os.environ.get("HYJS_P14_EXPECT_CROSS_CERTIFIED") != "1":
            raise AssertionError("Finals native requires the actual certified17 preparation mode")
        previous = ROOT / "analysis/p14/finals-formation-native-validation.json"
        saved = previous.read_bytes() if previous.exists() else None

        def record_and_restore():
            report = getattr(cls, "native_report", None) or {"status": "FAILED_BEFORE_REAL_DRAW_COMPLETE", "productionModified": False}
            (ROOT / "analysis/p14/finals-draw-native-validation.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
            if saved is None:
                previous.unlink(missing_ok=True)
            else:
                previous.write_bytes(saved)

        cls.addClassCleanup(record_and_restore)
        super().setUpClass()

    def setUp(self):
        super().setUp()
        self.draw_initialized = set()
        self.draw_checks = {}

    def start(self, zone, fixed=None):
        if zone not in self.draw_initialized:
            scripts = (self.source / "hyjs-server/db/sql").glob("155_*.sql")
            for script in sorted(scripts, key=lambda item: (item.name.endswith(".seed.sql"), item.name)):
                self.sql(zone, script.read_text())
            self.assertEqual(self.sql(zone, "SELECT enabled FROM p14_final_draw_policies;").strip(), "0")
            self.draw_initialized.add(zone)
        return super().start(zone, fixed)

    def sql(self, zone, text):
        result = super().sql(zone, text)
        if "UPDATE p14_season_policies SET enabled=1,season_index=61," in text:
            published = int(re.search(r"register_start=(\d+)", text)[1])
            super().sql(zone, "UPDATE p14_final_draw_policies SET version='local-common-final32-draw-test-v1',source='" + DRAW_SOURCE + "',published_at=" + str(published) + ";")
        return result

    def check_first_final_formations(self, end, common):
        # Final154 lock/offline/retry matrix has its own stable actual proof.
        # This new window checks only real draw and ordered-pair captures.
        a = self.logins[1][0]
        self.assertEqual(self.send(1, "CSKFMatchFrightList", {"groupId": "A"}, a, 930)[0], 10001)
        for zone in (1, 2):
            self.assertEqual(self.sql(zone, "SELECT COUNT(*) FROM p14_final_draws;").strip(), "0")
            self.sql(zone, "UPDATE p14_final_draw_policies SET enabled=1;")
        views = []
        common_hashes = []
        bodies = []
        for zone in (1, 2):
            groups = []
            identities = set()
            for group in ("A", "B", "C", "D"):
                reply = self.call(zone, "CSKFMatchFrightList", {"groupId": group}, self.logins[zone][0], 931, "SCKFMatchFrightList")
                self.assertEqual(reply.stageType, 7)
                current = reply.kfMatchAllRandList
                self.assertEqual((len(current.kFMatchSecondList), len(current.kFMatchThirdList)), (0, 0), "no future winners fabricated")
                for entrant in current.kFMatchFirstList:
                    self.assertGreater(entrant.playerId, 0)
                    self.assertIn(entrant.serverId, (1, 2))
                    self.assertGreater(entrant.fightNum, 0)
                    self.assertEqual(entrant.image, "hero_047")
                    self.assertEqual((entrant.winTime, entrant.promotionInfo, entrant.guessType), (0, 0, 0))
                    identities.add((entrant.serverId, entrant.playerId))
                groups.append(current.SerializeToString())
            self.assertEqual(identities, {(1, 1), (1, 2), (2, 1), (2, 2)})
            views.append(groups)
            common_hashes.append(self.sql(zone, "SELECT content_hash FROM p14_final_draws;").strip())
            bodies.append(json.loads(bytes.fromhex(self.sql(zone, "SELECT HEX(content) FROM p14_final_draws;").strip())))
        self.assertEqual(views[0], views[1], "two genuine peers show the same complete true group identities")
        self.assertEqual(common_hashes[0], common_hashes[1])
        self.assertEqual(bodies[0], bodies[1])
        body = bodies[0]
        self.assertEqual(body["qualificationHash"], common)
        self.assertEqual(len(body["entrants"]), 4)
        self.assertEqual(len(body["pairs"]), 2)
        self.assertEqual([sum(entry["group"] == group for entry in body["entrants"]) for group in ("A", "B", "C", "D")], [4, 0, 0, 0])

        evidence = ROOT / "analysis/p14/finals-draw-actual-native"
        evidence.mkdir(exist_ok=True)
        actual_pair = body["pairs"][0]
        ordered_sources = []
        for zone in (1, 2):
            self.sql(zone, "UPDATE p14_final_formation_policies SET enabled=1;")
        for position, side in enumerate(("left", "right")):
            origin = actual_pair[side + "ServerId"]
            player_id = actual_pair[side + "PlayerId"]
            owner = next(player for player in self.logins[origin] if player.playerId == player_id)
            self.call(origin, "CSKFEnterFrightGame", {}, owner, 940 + position, "SCKFEnterFrightGame")
            where = " WHERE server_id=" + str(origin) + " AND player_id=" + str(player_id)
            self.wait_sql(3 - origin, "SELECT COUNT(*) FROM p14_final_formations" + where + ";", "1")
            battle = bytes.fromhex(self.sql(origin, "SELECT HEX(battle) FROM p14_final_formations" + where + ";").strip())
            frame = bytes.fromhex(self.sql(origin, "SELECT HEX(snapshot) FROM p14_final_formations" + where + ";").strip())
            metadata = json.loads(self.sql(origin, "SELECT JSON_OBJECT('serverId',server_id,'playerId',player_id,'seasonIndex',season_index,'roundStage',round_stage,'revision',revision,'capturedAt',captured_at,'clientVersion',client_version,'engineVersion',engine_version,'configurationHash',configuration_hash,'qualificationHash',qualification_hash,'offerHash',offer_hash,'sourceHash',source_hash,'snapshotHash',snapshot_hash) FROM p14_final_formations" + where + ";").strip())
            import hashlib
            name = "actual-pair-" + side
            (evidence / (name + "-BattlePlayer.pb")).write_bytes(battle)
            (evidence / (name + "-FormationSimpleInfo.pb")).write_bytes(frame)
            ordered_sources.append(dict(metadata, battleSHA256=hashlib.sha256(battle).hexdigest(), formationSHA256=hashlib.sha256(frame).hexdigest()))
        (evidence / "actual-ordered-pair-source.json").write_text(json.dumps({"scope": "true155 bracket ordered pair and real purpose3 captures; no game1 seed/warId published and no winner", "drawSHA256": common_hashes[0], "pair": actual_pair, "orderedSources": ordered_sources}, ensure_ascii=False, indent=2) + "\n")
        self.formation_checks = {"stable154Dependency": "analysis/p14/stable-finals-formation-source-20261015/manifest.json", "in155Native": "only actual first ordered pair new purpose3 captures; full154 lock matrix is not repeated"}
        # Earned draw survives disabled current policy and a real process restart.
        self.sql(1, "UPDATE p14_final_draw_policies SET enabled=0;")
        fixed = self.ports[1]
        self.stop(1)
        self.start(1, fixed)
        after = self.call(1, "CSKFMatchFrightList", {"groupId": "A"}, a, 932, "SCKFMatchFrightList")
        self.assertEqual(after.kfMatchAllRandList.SerializeToString(), views[0][0])
        self.assertEqual(self.sql(1, "SELECT content_hash FROM p14_final_draws;").strip(), common_hashes[0])
        self.assertEqual(self.send(1, "CSKFMatchFrightList", {}, a, 933)[0], 10001, "original FinalFour empty group is a separate pending result, not group A")
        self.assertEqual(self.send(1, "CSKFMatchFrightList", {"groupId": "A", "replayFlag": 1}, a, 934)[0], 10001)
        (evidence / "actual-common-draw.json").write_text(json.dumps(body, ensure_ascii=False, indent=2) + "\n")
        for index, raw in enumerate(views[0]):
            (evidence / ("group-" + chr(65 + index) + "-KFMatchAllRandList.pb")).write_bytes(raw)
        self.draw_checks = {"defaultOff": True, "localRulePublishedBeforePromotion": True, "qualificationSHA256": common, "drawSHA256": common_hashes[0], "actualEntrants": 4, "groupLengths": [4, 0, 0, 0], "actualOrderedPairs": body["pairs"], "jointIdentity": "pid1/2 both realms retained", "coldAndHotClose": True, "noFillerOrByeOrSeriesWinner": True, "original81046": True, "phoneVerified": False}

    def test_original_match_scoped_result_point_debit_retry_and_cold(self):
        super().test_original_match_scoped_result_point_debit_retry_and_cold()
        type(self).native_report.update({"status": "PASS_REAL_FINAL32_ORIGINAL_FINAL_DRAW_ZERO_FILLER_ORDERED_PAIR_COLD", "finalDraw": self.draw_checks})


if __name__ == "__main__":
    unittest.main()
