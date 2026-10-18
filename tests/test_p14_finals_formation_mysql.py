"""Actual final32 -> original81030/purpose3/type3/5-minute lock/cold.

The shared manager allocates two temporary schemas under Root's one SQL window.
Qualification, score, final32 and formation rows are never fixture inserts.
Only the named disabled-by-default policy/calendar and finite level70 are setup.
"""
import concurrent.futures
import csv
import hashlib
import hmac
import http.client
import json
import os
from pathlib import Path
import re
import unittest

import test_p14_promotion_stage6_mysql as original6

ROOT = Path(__file__).resolve().parents[1]
FINAL_SOURCE = "local-complete-signed-final32-v1; originaltop32; all-frozen-peer-final-scores; frozen-rank-rule; take-min32-no-fill-no-bye-wins"
FORMATION_SOURCE = "local-original-final16-formation-v1; original81030-type3; true-final32-common-source; five-minutes-before-original-stage8; no-series-result"


@unittest.skipUnless(os.environ.get("HYJS_P14_FINALS_FORMATION_MYSQL_TEST") == "1", "explicit managed first-final formation native")
class P14FinalsFormationNativeMySQLTests(original6.P14PromotionStageSixMySQLTests):
    @classmethod
    def setUpClass(cls):
        # Preserve prior evidence byte-for-byte while reusing the actual stable
        # qualification/match superclass; keep this delivery's report independent.
        inherited = [ROOT / "analysis/p14/promotion-stage-six-native-validation.json",
                     ROOT / "analysis/p14/promotion-stage-six-native-match-subchain-validation.json"]
        saved = {path: path.read_bytes() if path.exists() else None for path in inherited}

        def record_and_restore():
            report = getattr(cls, "native_report", None) or {"status": "FAILED_BEFORE_COMPLETE_RESULT", "productionModified": False}
            if hasattr(cls, "managed_mysql"):
                report["managedAllocationRemoved"] = not bool(cls.managed_mysql.connections)
            if hasattr(cls, "realms"):
                report["managedConnectionFilesRemoved"] = all(not ((Path.home() / '.codex/local-services/test-connections') / (database + ".env")).exists() for database in cls.realms.values())
            (ROOT / "analysis/p14/finals-formation-native-validation.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
            for path, old in saved.items():
                if old is None:
                    path.unlink(missing_ok=True)
                else:
                    path.write_bytes(old)

        cls.addClassCleanup(record_and_restore)
        super().setUpClass()

    def setUp(self):
        super().setUp()
        self.final_initialized = set()
        self.formation_checks = {}

    def start(self, zone, fixed=None):
        if zone not in self.final_initialized:
            for number in (152, 154):
                scripts = (self.source / "hyjs-server/db/sql").glob(str(number) + "_*.sql")
                for script in sorted(scripts, key=lambda item: (item.name.endswith(".seed.sql"), item.name)):
                    self.sql(zone, script.read_text())
            self.assertEqual(self.sql(zone, "SELECT enabled FROM p14_final_formation_policies;").strip(), "0")
            self.final_initialized.add(zone)
        return super().start(zone, fixed)

    def sql(self, zone, text):
        if "INSERT INTO p14_time_slots" in text:
            # The full first-final calendar is published before signup. Its real
            # ten-minute preparation allows the original five-minute lock.
            start = int(re.search(r"\(61,(\d+),7,", text)[1])

            def slot(match):
                stage = int(match[2])
                if stage < 7:
                    return match[0]
                if stage == 7:
                    a, b = start, start + 600000
                elif stage == 8:
                    a, b = start + 600000, start + 1200000
                else:
                    a = start + 1200000 + (stage - 9) * 1000
                    b = a + 1000
                return "(61," + str(a) + "," + str(stage) + "," + str(b) + ","

            text = re.sub(r"\(61,(\d+),(\d+),(\d+),", slot, text)
        result = super().sql(zone, text)
        if "UPDATE p14_season_policies SET enabled=1,season_index=61," in text:
            published = int(re.search(r"register_start=(\d+)", text)[1])
            super().sql(zone, "UPDATE p14_final_qualification_policies SET enabled=1,version='local-complete-final32-formation-test-v1',source='" + FINAL_SOURCE + "',published_at=" + str(published) + ";UPDATE p14_final_formation_policies SET version='local-final16-formation-test-v1',source='" + FORMATION_SOURCE + "',published_at=" + str(published) + ";")
        return result

    def check_signed_rank(self):
        # Reuse the existing real accepted score producer without repeating its
        # already validated malformed-rank envelope matrix in this delivery.
        self.start(2, self.rank_fixed2)
        self.wait_sql(1, "SELECT COUNT(*) FROM p14_promotion_rank_deliveries WHERE revision=2 AND confirmed=1;", "1")
        replies = [self.rank(zone, 850) for zone in (1, 2)]
        self.assertEqual(replies[0], replies[1])
        self.assertEqual({(row.values[1], row.pid) for row in replies[0].rankIno.list}, {(1, 1), (1, 2), (2, 1), (2, 2)})
        self.rank_expected = replies[0]
        self.rank_verified = True

    def snapshot_import(self, zone, request):
        raw = json.dumps(request, separators=(",", ":")).encode()
        connection = http.client.HTTPConnection("127.0.0.1", self.ports[zone][0], timeout=20)
        try:
            signature = hmac.new(self.signing_key, raw, hashlib.sha256).hexdigest()
            connection.request("POST", "/internal/p14/snapshot", body=raw, headers={"Content-Type": "application/json", "X-HYJS-P14-Signature": signature})
            response = connection.getresponse()
            return json.loads(response.read())
        finally:
            connection.close()

    def check_final_seal(self):
        end = int(self.sql(1, "SELECT stage_end FROM p14_promotion_rank_offers WHERE season_index=61 AND server_id=1;").strip())
        self.clock.write_text(str(end + 1))
        common = []
        for zone in (1, 2):
            self.wait_sql(zone, "SELECT COUNT(*) FROM p14_promotion_rank_versions v WHERE revision=(SELECT MAX(revision) FROM p14_promotion_rank_versions x WHERE x.server_id=v.server_id) AND JSON_EXTRACT(CONVERT(content USING utf8mb4),'$.closed')=true;", "2")
            for player in self.logins[zone]:
                reply = self.call(zone, "CSKFOpenFrightServerInfo", {}, player, 870, "SCKFOpenFrightServerInfo")
                self.assertEqual((reply.typeStage, reply.isEnterCurStage), (7, 0))
            common.append(self.sql(zone, "SELECT content_hash FROM p14_final_qualifications;").strip())
        self.assertEqual(common[0], common[1])
        self.check_first_final_formations(end, common[0])

    def check_first_final_formations(self, end, common):
        a = self.logins[1][0]
        archived = {z: self.sql(z, "SELECT snapshot_hash,revision FROM p14_members ORDER BY server_id,player_id;SELECT source_hash FROM p14_audition_rosters ORDER BY player_id;SELECT source_hash FROM p14_promotion_formations ORDER BY server_id,player_id;") for z in (1, 2)}
        target = {"type": 3, "playerId": a.playerId, "serverId": 1}
        self.assertEqual(self.send(1, "CSKFEnterFrightGame", {}, a, 900)[0], 10001)
        self.assertEqual(self.send(1, "CSKFPlayerInfo", target, a, 900)[0], 10001, "closed purpose3 must not expose signup as current finals")
        for zone in (1, 2):
            self.assertEqual(self.sql(zone, "SELECT COUNT(*) FROM p14_final_formations;").strip(), "0")
            self.sql(zone, "UPDATE p14_final_formation_policies SET enabled=1;")

        b = self.logins[2][1]
        self.call(2, "CSKFEnterFrightGame", {}, b, 901, "SCKFEnterFrightGame")
        self.wait_sql(1, "SELECT COUNT(*) FROM p14_final_formations WHERE server_id=2 AND player_id=" + str(b.playerId) + ";", "1")
        fallback = self.call(1, "CSKFPlayerInfo", target, a, 901, "SCKFPlayerInfo")
        promotion = self.call(1, "CSKFPlayerInfo", dict(target, type=2), a, 901, "SCKFPlayerInfo")
        self.assertEqual(fallback, promotion, "before upload uses genuine promotion snapshot")
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.call(1, "CSKFEnterFrightGame", {}, a, 902, "SCKFEnterFrightGame"), range(8)))
        self.assertTrue(all(reply == replies[0] for reply in replies))
        self.assertEqual(self.sql(1, "SELECT revision FROM p14_final_formations WHERE server_id=1 AND player_id=1;").strip(), "1")
        self.wait_sql(2, "SELECT COUNT(*) FROM p14_final_formations WHERE server_id=1 AND player_id=1;", "1")
        first_battle_sha = self.sql(1, "SELECT SHA2(battle,256) FROM p14_final_formations WHERE server_id=1 AND player_id=1;").strip()
        # A genuine formation command changes a real position. Only this next
        # original81030 capture changes finals, and restoration leaves the profile
        # unchanged for the inherited resource-conservation assertions.
        formation = self.call(1, "CSFormationList", {}, a, 903, "SCFormationList")
        active = [item for item in formation.formationList if any(thing.id.startswith("hero_") for thing in item.things)]
        with (self.source / "analysis/tables/TableFormation.tsv").open(encoding="utf-8-sig") as table:
            rows = list(csv.reader(table, delimiter="\t"))
        definition = next(dict(zip(rows[1], row)) for row in rows[5:] if row and row[0] == "1")
        positions = [int(value) for value in definition["posInfo"].split(",")]
        restore = ";".join(str(item.formatPos) + "," + str(item.battlePos) for item in active)
        altered = ";".join(str(item.formatPos) + "," + str(positions[(positions.index(item.battlePos) + 1) % len(positions)]) for item in active)
        self.call(1, "CSFormationChange", {"fomationMapStr": altered}, a, 904, "SCFormationChange")
        self.assertEqual(self.call(1, "CSKFPlayerInfo", target, a, 905, "SCKFPlayerInfo"), fallback, "mutable profile cannot change a frozen final upload")
        self.call(1, "CSKFEnterFrightGame", {}, a, 906, "SCKFEnterFrightGame")
        changed = self.call(1, "CSKFPlayerInfo", target, a, 907, "SCKFPlayerInfo")
        # Public FormationSimpleInfo describes format slots; BattlePlayer alone
        # carries the actual tactical battle pose. Compare that real capture.
        self.assertEqual(changed, fallback)
        changed_battle_sha = self.sql(1, "SELECT SHA2(battle,256) FROM p14_final_formations WHERE server_id=1 AND player_id=1;").strip()
        self.assertNotEqual(changed_battle_sha, first_battle_sha)
        self.call(1, "CSFormationChange", {"fomationMapStr": restore}, a, 908, "SCFormationChange")
        # Last legal upload is one millisecond before the original 5min boundary.
        lock = int(self.sql(1, "SELECT lock_at FROM p14_final_formation_offers;").strip())
        self.assertEqual(lock, end + 300000)
        self.clock.write_text(str(lock - 1))
        fixed2 = self.ports[2]
        self.stop(2)
        self.call(1, "CSKFEnterFrightGame", {}, a, 909, "SCKFEnterFrightGame")
        self.clock.write_text(str(lock))
        self.assertEqual(self.send(1, "CSKFEnterFrightGame", {}, a, 910)[0], 10001)
        self.assertEqual(self.sql(1, "SELECT revision FROM p14_final_formations WHERE server_id=1 AND player_id=1;").strip(), "3")
        self.assertEqual(self.sql(1, "SELECT SHA2(battle,256) FROM p14_final_formations WHERE server_id=1 AND player_id=1;").strip(), first_battle_sha)
        # Exact retry is a historical receipt; a new submission stays forbidden.
        self.assertEqual(self.call(1, "CSKFEnterFrightGame", {}, a, 909, "SCKFEnterFrightGame"), replies[0])
        self.start(2, fixed2)
        self.wait_sql(2, "SELECT revision FROM p14_final_formations WHERE server_id=1 AND player_id=1;", "3")
        self.assertEqual(self.call(2, "CSKFPlayerInfo", target, self.logins[2][0], 911, "SCKFPlayerInfo"), fallback)
        payload = json.loads(bytes.fromhex(self.sql(1, "SELECT HEX(payload) FROM p14_deliveries WHERE JSON_EXTRACT(CONVERT(payload USING utf8mb4),'$.protocolVersion')=3 AND JSON_EXTRACT(CONVERT(payload USING utf8mb4),'$.revision')=3;").strip()))
        self.assertEqual((payload["protocolVersion"], payload["roundStage"], payload["finalQualificationHash"]), (3, 8, common))
        self.assertTrue(self.snapshot_import(2, payload)["data"]["confirmed"])
        for field, value in (("protocolVersion", 2), ("roundStage", 10), ("finalQualificationHash", "0" * 64), ("finalOfferHash", "0" * 64), ("capturedAt", lock)):
            changed = dict(payload, revision=4, **{field: value})
            identity = "p14/final/{seasonIndex}/{roundStage}/{finalQualificationHash}/{finalOfferHash}/{sourceServerId}/{playerId}/{revision}/{destinationServerId}".format(**changed)
            changed["deliveryId"] = hashlib.sha256(identity.encode()).hexdigest()
            self.assertNotEqual(self.snapshot_import(2, changed)["code"], 0, "new signed delivery must reach scope/capture validation, not an old receipt conflict")
        self.assertEqual(self.sql(2, "SELECT revision FROM p14_final_formations WHERE server_id=1 AND player_id=1;").strip(), "3")
        self.sql(1, "UPDATE p14_final_formation_policies SET enabled=0;")
        fixed1 = self.ports[1]
        self.stop(1)
        self.start(1, fixed1)
        self.assertEqual(self.call(1, "CSKFPlayerInfo", target, a, 912, "SCKFPlayerInfo"), fallback)
        self.assertEqual(self.call(1, "CSKFEnterFrightGame", {}, a, 909, "SCKFEnterFrightGame"), replies[0])
        self.clock.write_text(str(end + 600001))
        self.assertEqual(self.send(1, "CSKFEnterFrightGame", {}, a, 913)[0], 10001)
        self.assertEqual(self.call(1, "CSKFPlayerInfo", target, a, 914, "SCKFPlayerInfo"), fallback)
        for zone in (1, 2):
            self.assertEqual(self.sql(zone, "SELECT snapshot_hash,revision FROM p14_members ORDER BY server_id,player_id;SELECT source_hash FROM p14_audition_rosters ORDER BY player_id;SELECT source_hash FROM p14_promotion_formations ORDER BY server_id,player_id;"), archived[zone])
        evidence = ROOT / "analysis/p14/finals-formation-actual-native"
        evidence.mkdir(exist_ok=True)
        source_rows = []
        for origin, player in ((1, a), (2, b)):
            where = " WHERE server_id=" + str(origin) + " AND player_id=" + str(player.playerId)
            battle_bytes = bytes.fromhex(self.sql(origin, "SELECT HEX(battle) FROM p14_final_formations" + where + ";").strip())
            frame_bytes = bytes.fromhex(self.sql(origin, "SELECT HEX(snapshot) FROM p14_final_formations" + where + ";").strip())
            name = "realm" + str(origin) + "-player" + str(player.playerId)
            (evidence / (name + "-BattlePlayer.pb")).write_bytes(battle_bytes)
            (evidence / (name + "-FormationSimpleInfo.pb")).write_bytes(frame_bytes)
            metadata = self.sql(origin, "SELECT JSON_OBJECT('serverId',server_id,'playerId',player_id,'seasonIndex',season_index,'roundStage',round_stage,'revision',revision,'capturedAt',captured_at,'clientVersion',client_version,'engineVersion',engine_version,'configurationHash',configuration_hash,'qualificationHash',qualification_hash,'offerHash',offer_hash,'source',source,'sourceHash',source_hash,'snapshotHash',snapshot_hash) FROM p14_final_formations" + where + ";").strip()
            source_rows.append(dict(json.loads(metadata), battleSHA256=hashlib.sha256(battle_bytes).hexdigest(), formationSHA256=hashlib.sha256(frame_bytes).hexdigest()))
        (evidence / "manifest.json").write_text(json.dumps({"scope": "actual individual purpose3 captures; no bracket/game seed or winner yet", "rows": source_rows, "applicationSourceProofSHA256": hashlib.sha256((self.prebuilt / "manifest.json").read_bytes()).hexdigest()}, ensure_ascii=False, indent=2) + "\n")
        self.formation_checks = {"defaultOff": True, "commonQualificationSHA256": common, "purpose": 3, "roundStage": 8, "sameStamp": "8 requests produce one revision/outbox", "realFormation": "original tactical pose changes actual frozen BattlePlayer SHA only after upload; public format-slot projection remains correct; restoration recaptures original full BattlePlayer SHA", "originalLock": "lock-1 upload accepted; lock and active stage8 rejected; old receipt preserved", "peer": "destination stopped at lock-1; genuine signed revision3 imported after lock; exact retry confirmed", "cold": "current type3 and old receipt preserved after source shutdown and policy close", "historicalSignupAuditionPromotion": "all source and snapshot hashes unchanged", "seriesWinnerImplemented": False, "phoneVerified": False}

    def test_original_match_scoped_result_point_debit_retry_and_cold(self):
        super().test_original_match_scoped_result_point_debit_retry_and_cold()
        type(self).native_report.update({"status": "PASS_REAL_FINAL32_ORIGINAL_FINALS_FORMATION_PURPOSE3_FIVE_MINUTE_LOCK_COLD", "workers": 11, "finalFormation": self.formation_checks})


if __name__ == "__main__":
    unittest.main()
