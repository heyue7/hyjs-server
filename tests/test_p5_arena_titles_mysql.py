"""Original accepted Arena2 winner -> five original titles -> attributes/cold."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import unittest

import test_p10_mysql as transport
import test_p10_scene_tcp_mysql as native
import test_p11_arena_scope as arena

ROOT = Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get("HYJS_P5_ARENA_TITLES_MYSQL_TEST") == "1", "explicit isolated verified arena titles flow")
class P5ArenaTitlesMySQLTests(unittest.TestCase):
    send = transport.P10MySQLTests.send
    login = transport.P10MySQLTests.login
    stamped = arena.P11ArenaScopeTests.stamped
    call = transport.P10MySQLTests.call
    profile = transport.P10MySQLTests.profile
    sql = transport.P10MySQLTests.sql
    start = native.P10SceneTCPTests.start
    stop = native.P10SceneTCPTests.stop
    setUp = transport.P10MySQLTests.setUp

    @classmethod
    def root_sql(cls, text):
        if not cls.database.startswith("hyjs_test_p5_arena_") or not cls.test_user.startswith("p5arena_"):
            raise AssertionError("own arena title schema required")
        result = subprocess.run(["rtk", "proxy", "docker", "exec", "-i", "hyjs-mysql", "sh", "-c", 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'], input=text, text=True, capture_output=True, timeout=30)
        if result.returncode:
            raise AssertionError(result.stderr.replace(cls.test_password, "[redacted]"))
        return result.stdout

    def test_actual_original_winner_titles_effects_and_cold(self):
        began = time.monotonic()
        self.source = Path(os.environ["HYJS_P5_ARENA_TITLES_BUILD_ROOT"]).resolve()
        self.assertEqual(self.source.parent, ROOT / "build/candidates")
        self.assertIn("P11ArenaVerifiedBestWonRank", (self.source / "hyjs-server/service/collectionService.go").read_text())
        report = {}
        with tempfile.TemporaryDirectory(prefix="hyjs-p5-arena-titles-") as temporary:
            self.binary = Path(temporary) / "arena-titles-native"
            # Compile before acquiring the sole test database lifecycle lock.
            built = subprocess.run(["rtk", "proxy", "go", "build", "-o", str(self.binary), "."], cwd=self.source / "hyjs-server", capture_output=True, text=True, timeout=240)
            self.assertEqual(built.returncode, 0, (built.stdout + built.stderr)[-5000:])
            type(self).database = "hyjs_test_p5_arena_" + secrets.token_hex(6)
            type(self).test_user = "p5arena_" + secrets.token_hex(5)
            type(self).test_password = secrets.token_hex(24)
            self.env = dict(os.environ, HYJS_MYSQL_HOST="127.0.0.1:33068", HYJS_MYSQL_DATABASE=self.database, HYJS_MYSQL_USER=self.test_user, HYJS_MYSQL_PASSWORD=self.test_password)
            with open("/tmp/hyjs-isolated-mysql.lock", "a+") as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    self.root_sql(f"CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;CREATE USER '{self.test_user}'@'%' IDENTIFIED BY '{self.test_password}';GRANT ALL ON `{self.database}`.* TO '{self.test_user}'@'%';")
                    self.start()
                    names = ["p5-arena-title-" + secrets.token_hex(5) for _ in range(2)]
                    defender, owner = [self.login(name) for name in names]
                    for login, label in [(defender, "原竞技对手"), (owner, "原竞技称号")]:
                        self.call("CSPlayerCreate", {"playerName": label, "heroId": "hero_047"}, login, 1, "SCPlayerCreate")
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',20) WHERE id={defender.playerId};UPDATE profiles SET player=JSON_SET(player,'$.level',100) WHERE id={owner.playerId};")
                    self.sql("UPDATE p11_arena_policies SET enabled=1;UPDATE p11_arena_fight_policies SET enabled=1;")
                    query = self.call("CSPvpFindMatch", {}, owner, 10, "SCPvpFindMatch")
                    self.assertEqual(next(v.pvpRank for v in query.rankList if v.info.playerId == defender.playerId), 1)
                    wanted = ["title_022", "title_023", "title_024", "title_025", "title_026"]
                    stamp = 100
                    def http(name, fields, login, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, login, stamp, expected)
                    def titles(login):
                        return {v.titleId: v for v in http("CSTitleList", {}, login, "SCTitleList").titleList}
                    def panel(login):
                        return http("CSFindPlayerInfo", {"playerId": login.playerId, "serverId": 1}, login, "SCFindPlayerInfo").formationInfo
                    for login in [defender, owner]:
                        initial = titles(login)
                        self.assertTrue(all(initial[key].state == 0 for key in wanted))
                    energy = self.profile(owner)[0]["energy"]
                    result = http("CSPvpFightResult", {"playerId": defender.playerId, "serverId": 1}, owner, "SCPvpFightResult")
                    self.assertEqual(result.winPlayerId, owner.playerId)
                    self.assertEqual(result.myrank.pvpRank, 1)
                    self.assertEqual(self.profile(owner)[0]["energy"], energy - 2)
                    qualified = titles(owner)
                    for key, goal in zip(wanted, [3000, 500, 100, 10, 1]):
                        self.assertEqual((qualified[key].state, qualified[key].current, qualified[key].total), (1, 1, goal))
                    denied = titles(defender)
                    self.assertTrue(all(denied[key].state == 0 for key in wanted))
                    base = panel(owner)
                    props = ["hit", "dodge", "antiCri", "cri", "damageReduce"]
                    powers = []
                    for key, prop, amount in zip(wanted, props, [2, 2, 2, 2, 3]):
                        reply = http("CSChangeTitle", {"titleId": key}, owner, "SCChangeTitle")
                        self.assertEqual(reply.curTitle, key)
                        current = panel(owner)
                        original = base.formationItemInfoList[0].warInfo.baseProp
                        actual = current.formationItemInfoList[0].warInfo.baseProp
                        expected = type(original)();expected.CopyFrom(original)
                        setattr(expected, prop, getattr(expected, prop) + amount)
                        self.assertEqual(actual, expected)
                        self.assertEqual(http("CSPlayer", {}, owner, "SCPlayer").player.fightPower, current.fightPower)
                        powers.append(current.fightPower)
                    before = self.profile(owner)
                    kv = {v["key"]: v["value"] for v in before[1].get("kv", [])}
                    for key in wanted:
                        self.assertIn("proof=", kv["p5.title:" + key])
                    ledger = self.sql("SELECT fight_id,response_hash,terminal_hash FROM p11_arena_fight_settlements;")
                    self.sql("UPDATE p11_arena_policies SET enabled=0;UPDATE p11_arena_fight_policies SET enabled=0;")
                    self.stop();self.start();owner=self.login(names[1])
                    after = titles(owner)
                    self.assertTrue(all(after[key].state == 1 for key in wanted))
                    self.assertEqual(panel(owner).fightPower, powers[-1])
                    self.assertEqual(self.profile(owner)[1].get("kv", []), before[1].get("kv", []))
                    self.assertEqual(self.sql("SELECT fight_id,response_hash,terminal_hash FROM p11_arena_fight_settlements;"), ledger)
                    http("CSChangeTitle", {"titleId": ""}, owner, "SCChangeTitle")
                    self.assertEqual(panel(owner), base)
                    report = {"status": "PASS_ORIGINAL_ARENA_WINNER_FIVE_TITLES_WEAR_PUBLIC_PROP_AND_COLD", "seconds": round(time.monotonic()-began,3), "finitePrerequisites": "Only original CreatePlayer047 and isolated level100/20. No winner/rank/BestWonRank/title/effect/currency/GUID injection. Original FindMatch and one actual type2 engine result consume Energy2 and produce rank1; only its verified proof grants five original titles.", "actualEnergyCost": 2, "verifiedBestRank": 1, "titles": wanted, "powerSequence": [base.fightPower] + powers + [base.fightPower], "coldRestarts": 1, "ownedGoSHA256": {p: hashlib.sha256((self.source/p).read_bytes()).hexdigest() for p in ["hyjs-server/service/collectionService.go", "hyjs-server/service/p5ArenaTitles_test.go"]}, "phoneUI": False, "productionModified": False}
                finally:
                    self.stop()
                    self.root_sql(f"DROP DATABASE IF EXISTS `{self.database}`;DROP USER IF EXISTS '{self.test_user}'@'%';")
                    remaining = self.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{self.database}';SELECT COUNT(*) FROM mysql.user WHERE user='{self.test_user}';SELECT COUNT(*) FROM mysql.db WHERE Db='{self.database}';")
                    self.assertEqual(remaining.split(), ["0", "0", "0"])
                    report["cleanupCounts"] = [0,0,0]
        if report.get("status"):
            (ROOT/"analysis/p5/arena-titles-mysql-validation.json").write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n")

if __name__ == "__main__":
    unittest.main()
