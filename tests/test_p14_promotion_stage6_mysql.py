# Separate native verification for the original UI stage6 correction.
"""One real two-realm qualification/match chain feeds signed original type2 ranks.

Root schedules the single managed MySQL window. No score or qualification facts
are fixtures; only the named closed-by-default policy and calendar are published.
"""
import hashlib
import hmac
import http.client
import json
import os
from pathlib import Path
import re
import threading
import time
import unittest

import p14_promotion_stage6_match_subchain as subchain

ROOT = Path(__file__).resolve().parents[1]
RANK_SOURCE = "local-signed-accepted-promotion-score-v1; originaltype2; own-active-spends-only; qualified-roster-power; prepublished-score-desc-power-desc-server-asc-player-asc"


class AtomicClock(type(Path())):
    def write_text(self, data, **kwargs):
        staged = self.with_name(self.name + ".next")
        written = Path.write_text(staged, data, **kwargs)
        os.replace(staged, self)
        return written


@unittest.skipUnless(os.environ.get("HYJS_P14_PROMOTION_RANK_MYSQL_TEST") == "1", "explicit promotion rank managed native")
class P14PromotionStageSixMySQLTests(subchain.P14PromotionStageSixMatchSubchainTests):
    @classmethod
    def setUpClass(cls):
        def record_final():
            if cls.native_report is not None:
                (ROOT / "analysis/p14/promotion-stage-six-native-validation.json").write_text(json.dumps(cls.native_report, ensure_ascii=False, indent=2) + "\n")
        cls.addClassCleanup(record_final)
        super().setUpClass()
        cls.clock = AtomicClock(cls.clock)

    def setUp(self):
        super().setUp()
        self.rank_initialized = set()
        self.logins = {}
        self.rank_mutex = threading.Lock()
        self.rank_offline_started = False
        self.rank_verified = False
        self.rank_checks = {}

    def start(self, zone, fixed=None):
        if zone not in self.rank_initialized:
            scripts = (self.source / "hyjs-server/db/sql").glob("151_*.sql")
            for script in sorted(scripts, key=lambda item: (item.name.endswith(".seed.sql"), item.name)):
                self.sql(zone, script.read_text())
            self.rank_initialized.add(zone)
        return super().start(zone, fixed)

    def sql(self, zone, text):
        result = super().sql(zone, text)
        if "UPDATE p14_season_policies SET enabled=1,season_index=61," in text:
            published = int(re.search(r"register_start=(\d+)", text)[1])
            self.signing_key = bytes.fromhex(re.search(r'UNHEX\("([0-9a-f]{64})"\)', text)[1])
            super().sql(zone, "UPDATE p14_promotion_rank_policies SET enabled=1,version='local-promotion-rank-v1',source='" + RANK_SOURCE + "',published_at=" + str(published) + ";")
        return result

    def login(self, zone, uid):
        player = super().login(zone, uid)
        self.logins.setdefault(zone, []).append(player)
        return player

    def send(self, zone, name, fields=None, login=None, stamp=1700000000000):
        if name == "CSKuafuMatchPlayer" and stamp == 800 and not self.rank_verified:
            with self.rank_mutex:
                if not self.rank_offline_started:
                    for origin in [1, 2]:
                        self.wait_sql(3-origin, "SELECT COUNT(*) FROM p14_promotion_rank_versions WHERE server_id=" + str(origin) + " AND revision=1;", "1")
                    self.rank_fixed2 = self.ports[2]
                    self.stop(2)
                    self.rank_offline_started = True
        return super().send(zone, name, fields, login, stamp)

    def internal(self, zone, raw, signature=None):
        signature = signature if signature is not None else hmac.new(self.signing_key, raw, hashlib.sha256).hexdigest()
        connection = http.client.HTTPConnection("127.0.0.1", self.ports[zone][0], timeout=20)
        try:
            connection.request("POST", "/internal/p14/promotion-rank", body=raw, headers={"Content-Type": "application/json", "X-HYJS-P14-Signature": signature})
            response = connection.getresponse()
            return response.status, json.loads(response.read())
        finally:
            connection.close()

    def call(self, zone, name, fields, login, stamp, expected):
        reply = super().call(zone, name, fields, login, stamp, expected)
        if name == "CSKFOpenFrightServerInfo" and stamp == 803 and not self.rank_verified:
            self.check_signed_rank()
        if name == "CSKFOpenFrightServerInfo" and stamp == 805 and self.rank_verified:
            self.check_final_seal()
        return reply

    def rank(self, zone, stamp):
        return super().call(zone, "CSKFRankInfo", {"type": 2}, self.logins[zone][0], stamp, "SCKFRankInfo")

    def check_signed_rank(self):
        self.wait_sql(1, "SELECT COUNT(*) FROM p14_promotion_rank_deliveries WHERE revision=2 AND confirmed=0 AND attempt_count>0;", "1")
        payload = bytes.fromhex(self.sql(1, "SELECT HEX(payload) FROM p14_promotion_rank_deliveries WHERE revision=2;").strip())
        self.start(2, self.rank_fixed2)
        self.wait_sql(1, "SELECT COUNT(*) FROM p14_promotion_rank_deliveries WHERE revision=2 AND confirmed=1;", "1")
        self.wait_sql(2, "SELECT COUNT(*) FROM p14_promotion_rank_versions WHERE server_id=1 AND revision=2;", "1")
        self.assertEqual(bytes.fromhex(self.sql(1, "SELECT HEX(payload) FROM p14_promotion_rank_deliveries WHERE revision=2;").strip()), payload)
        replies = [self.rank(zone, 850) for zone in [1, 2]]
        self.assertEqual(replies[0], replies[1])
        rows = replies[0].rankIno.list
        self.assertEqual(len(rows), 4)
        identities = [(row.values[1], row.pid) for row in rows]
        self.assertEqual(set(identities), {(1, 1), (1, 2), (2, 1), (2, 2)})
        self.assertEqual([row.rank for row in rows], [1, 2, 3, 4])
        self.assertTrue(all(len(row.values) == 3 for row in rows))
        score_after = int(self.sql(1, "SELECT score_after FROM p14_promotion_fights;").strip())
        for row in rows:
            self.assertEqual(row.values[2], score_after if (row.values[1], row.pid) == (1, 1) else 1)
            self.assertGreater(row.values[0], 0)
        # The opaque signed body survives offline delivery and exact retry unchanged.
        self.assertEqual(self.internal(2, payload)[1]["data"]["confirmed"], True)
        envelope = json.loads(payload)
        compact = lambda value: json.dumps(value, separators=(",", ":"), ensure_ascii=False).encode()
        cases = [compact(dict(envelope, unknownField=1)), payload+b"{}", b" "*1048577,
                 compact(dict(envelope, snapshot=None)), compact(dict(envelope, protocolVersion=2)),
                 compact(dict(envelope, sourceServerId=0)), compact(dict(envelope, destinationServerId=1))]
        for raw in cases:
            self.assertEqual(self.internal(2, raw)[1]["code"], 7001)
        self.assertEqual(self.internal(2, payload, "not-hex")[1]["code"], 7001)
        self.assertNotEqual(self.internal(2, payload, "0"*64)[1]["code"], 0)
        # Even a trusted signature cannot turn an impossible +1 increment into an accepted score.
        bad = json.loads(payload)
        bad["snapshot"]["revision"] = 3
        bad["snapshot"]["previousHash"] = self.sql(2, "SELECT content_hash FROM p14_promotion_rank_versions WHERE server_id=1 AND revision=2;").strip()
        bad["snapshot"]["scores"][0]["score"] += 1
        snapshot = compact(bad["snapshot"])
        bad["deliveryId"] = hashlib.sha256(("p14/rank/1/2/"+hashlib.sha256(snapshot).hexdigest()).encode()).hexdigest()
        self.assertNotEqual(self.internal(2, compact(bad))[1]["code"], 0)
        self.assertEqual(self.sql(2, "SELECT COUNT(*) FROM p14_promotion_rank_versions WHERE server_id=1;").strip(), "2")
        self.sql(2, "UPDATE p14_promotion_rank_policies SET enabled=0;")
        self.assertEqual(self.internal(2, payload)[1]["data"]["confirmed"], True)
        self.assertEqual(super().send(2, "CSKFRankInfo", {"type": 2}, self.logins[2][0], 851)[0], 10001)
        self.sql(2, "UPDATE p14_promotion_rank_policies SET enabled=1;")
        evidence = ROOT / "analysis/p14/promotion-stage-six-actual-native"
        evidence.mkdir(exist_ok=True)
        (evidence / "accepted-source-version2.json").write_bytes(payload)
        (evidence / "original-SCKFRankInfo.pb").write_bytes(replies[0].SerializeToString())
        self.rank_expected = replies[0]
        self.rank_verified = True
        self.rank_checks.update({"jointIdentities": identities, "actualActiveScore": score_after,
                                "signedBodySHA256": hashlib.sha256(payload).hexdigest(), "originalRankSHA256": hashlib.sha256(replies[0].SerializeToString()).hexdigest(),
                                "offlineRetry": "destination stopped before real accepted match; exact persisted version2 retried and imported after restart",
                                "strictImport": "unknown/trailing/null/oversize/protocol/IDs/invalidhex/wrongHMAC/impossible signed score all rejected; exact retry acknowledged with gate closed"})

    def check_final_seal(self):
        end = int(self.sql(1, "SELECT stage_end FROM p14_promotion_rank_offers WHERE season_index=61 AND server_id=1;").strip())
        self.clock.write_text(str(end+1))
        for zone in [1, 2]:
            self.wait_sql(zone, "SELECT COUNT(*) FROM p14_promotion_rank_versions v WHERE revision=(SELECT MAX(revision) FROM p14_promotion_rank_versions x WHERE x.server_id=v.server_id) AND JSON_EXTRACT(CONVERT(content USING utf8mb4),'$.closed')=true;", "2")
            self.assertEqual(self.rank(zone, 860), self.rank_expected)
        self.rank_checks["closed"] = "both real source ledgers seal after isolated stage deadline; both peers retain identical original rank; no final32 qualification invented"

    def test_original_match_scoped_result_point_debit_retry_and_cold(self):
        super().test_original_match_scoped_result_point_debit_retry_and_cold()
        self.assertTrue(self.rank_verified)
        type(self).native_report.update({"status": "PASS_REAL_QUALIFICATION_ACCEPTED_MATCH_SIGNED_TWO_REALM_ORIGINAL_RANK_OFFLINE_COLD_SEAL", "rank": self.rank_checks, "workers": 10})


if __name__ == "__main__":
    unittest.main()
