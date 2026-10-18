"""Combine the corrected original promotion6 transport with real final32 inputs.

Only prepublished policy/calendar and finite level70 prerequisites are fixtures.
All qualifications, accepted scores, peer versions and final rosters are produced
by the actual Go servers. Root owns the one shared MySQL test window.
"""
import json
import os
from pathlib import Path
import re
import unittest

import test_p14_promotion_stage6_mysql as original6

ROOT = Path(__file__).resolve().parents[1]
FINAL_SOURCE = "local-complete-signed-final32-v1; originaltop32; all-frozen-peer-final-scores; frozen-rank-rule; take-min32-no-fill-no-bye-wins"


@unittest.skipUnless(os.environ.get("HYJS_P14_FINAL32_MYSQL_TEST") == "1", "explicit combined original6 and final32 managed native")
class P14FinalQualificationNativeMySQLTests(original6.P14PromotionStageSixMySQLTests):
    @classmethod
    def setUpClass(cls):
        def record_final32():
            if os.environ.get("HYJS_P14_FINAL32_MYSQL_TEST") != "1":
                return
            report = cls.native_report or {"status": "FAILED_BEFORE_COMPLETE_COMBINED_RESULT", "productionModified": False}
            if hasattr(cls, "managed_mysql"):
                report["managedAllocationRemoved"] = not bool(cls.managed_mysql.connections)
            if hasattr(cls, "realms"):
                report["managedConnectionFilesRemoved"] = all(not ((Path.home() / '.codex/local-services/test-connections') / (database + ".env")).exists() for database in cls.realms.values())
            (ROOT / "analysis/p14/final32-native-validation.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        cls.addClassCleanup(record_final32)
        super().setUpClass()

    def setUp(self):
        super().setUp()
        self.final_initialized = set()
        self.final_checks = {}

    def start(self, zone, fixed=None):
        if zone not in self.final_initialized:
            scripts = (self.source / "hyjs-server/db/sql").glob("152_*.sql")
            for script in sorted(scripts, key=lambda item: (item.name.endswith(".seed.sql"), item.name)):
                self.sql(zone, script.read_text())
            self.assertEqual(self.sql(zone, "SELECT enabled FROM p14_final_qualification_policies;").strip(), "0")
            self.final_initialized.add(zone)
        return super().start(zone, fixed)

    def sql(self, zone, text):
        result = super().sql(zone, text)
        if "UPDATE p14_season_policies SET enabled=1,season_index=61," in text:
            published = int(re.search(r"register_start=(\d+)", text)[1])
            super().sql(zone, "UPDATE p14_final_qualification_policies SET enabled=1,version='local-complete-final32-test-v1',source='" + FINAL_SOURCE + "',published_at=" + str(published) + ";")
        return result

    def check_final_seal(self):
        end = int(self.sql(1, "SELECT stage_end FROM p14_promotion_rank_offers WHERE season_index=61 AND server_id=1;").strip())
        fixed2 = self.ports[2]
        self.stop(2)
        self.clock.write_text(str(end+1))
        self.wait_sql(1, "SELECT COUNT(*) FROM p14_promotion_rank_versions WHERE server_id=1 AND JSON_EXTRACT(CONVERT(content USING utf8mb4),'$.closed')=true;", "1")
        self.assertEqual(self.send(1, "CSKFOpenFrightServerInfo", {}, self.logins[1][0], 870)[0], 10001, "missing final peer returns pending, never boolean1 or fake loss stage")
        self.assertEqual(self.sql(1, "SELECT COUNT(*) FROM p14_final_qualifications;").strip(), "0")
        self.start(2, fixed2)
        super().check_final_seal()
        hashes = []
        for zone in [1, 2]:
            for player in self.logins[zone]:
                accepted = self.call(zone, "CSKFOpenFrightServerInfo", {}, player, 871, "SCKFOpenFrightServerInfo")
                self.assertEqual((accepted.typeStage, accepted.isEnterCurStage, accepted.mateTime), (7, 0, 0))
            self.assertEqual(self.sql(zone, "SELECT COUNT(*) FROM p14_final_qualifications;").strip(), "1")
            hashes.append(self.sql(zone, "SELECT content_hash FROM p14_final_qualifications;").strip())
        self.assertEqual(hashes[0], hashes[1], "same real final source set must create the same common roster hash")
        body = json.loads(bytes.fromhex(self.sql(1, "SELECT HEX(content) FROM p14_final_qualifications;").strip()))
        self.assertEqual(len(body["sources"]), 2)
        self.assertEqual(len(body["ranking"]), 4, "do not create 28 fictitious finalists")
        self.assertEqual([(entry["ServerID"], entry["PlayerID"]) for entry in body["ranking"]], [(row.values[1], row.pid) for row in self.rank_expected.rankIno.list])
        # Earned frozen qualification survives policy closure and cold startup.
        self.sql(1, "UPDATE p14_final_qualification_policies SET enabled=0;")
        self.assertEqual(self.call(1, "CSKFOpenFrightServerInfo", {}, self.logins[1][0], 872, "SCKFOpenFrightServerInfo").isEnterCurStage, 0)
        fixed1 = self.ports[1]
        self.stop(1)
        self.start(1, fixed1)
        self.assertEqual(self.call(1, "CSKFOpenFrightServerInfo", {}, self.logins[1][0], 873, "SCKFOpenFrightServerInfo").isEnterCurStage, 0)
        self.assertEqual(self.sql(1, "SELECT content_hash FROM p14_final_qualifications;").strip(), hashes[0])
        self.clock.write_text(str(end+1001))
        in16 = self.call(1, "CSKFOpenFrightServerInfo", {}, self.logins[1][0], 874, "SCKFOpenFrightServerInfo")
        self.assertEqual((in16.typeStage, in16.isEnterCurStage), (8, 0))
        self.clock.write_text(str(end+2001))
        self.assertEqual(self.send(1, "CSKFOpenFrightServerInfo", {}, self.logins[1][0], 875)[0], 10001, "a top32 qualifier does not invent a top16 win or a failure code")
        evidence = ROOT / "analysis/p14/final32-actual-native"
        evidence.mkdir(exist_ok=True)
        (evidence / "complete-source-qualification.json").write_text(json.dumps(body, ensure_ascii=False, indent=2) + "\n")
        self.final_checks = {"defaultOff": True, "publishedBeforeOriginal6": True, "missingFinalPeer": "true destination offline at cutoff; own final version alone keeps original7 pending error and no snapshot",
                             "completeFinalPeer": "both real signed closed versions → same contentSHA → all four actual qualifiers, no filler/bye wins", "commonContentSHA256": hashes[0],
                             "coldAndClosure": "same earned snapshot read after hot policy closure and actual source restart", "originalStages": "6 match/rank and originalwire0, 7 real final32 wire0, 8 remains qualified0, 9 explicitpending without top16 result", "finalsBattlesImplemented": False}

    def test_original_match_scoped_result_point_debit_retry_and_cold(self):
        super().test_original_match_scoped_result_point_debit_retry_and_cold()
        type(self).native_report.update({"status": "PASS_ORIGINAL_PROMOTION6_AND_COMPLETE_REAL_TWO_REALM_FINAL32", "workers": 11, "final32": self.final_checks})


if __name__ == "__main__":
    unittest.main()
