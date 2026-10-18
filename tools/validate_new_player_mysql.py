"""Validate the new-player/content batch on separate managed HYJS MySQL schemas."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from hyjs_managed_mysql import ManagedMySQL


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate", type=Path, required=True)
    parser.add_argument("--attempt", type=int, default=1)
    parser.add_argument("--shop-fixture", choices=("p1", "p4"), default="p1")
    args = parser.parse_args()
    candidate = args.candidate.resolve()
    if candidate.parent != ROOT / "build/candidates" or not (candidate / "manifest.json").is_file():
        raise ValueError("Use one pinned project candidate")
    if args.attempt < 1:
        raise ValueError("Attempt must be positive")
    target = ROOT / "analysis/new-player-content-20261018"
    target.mkdir(exist_ok=True)
    record = target / ("mysql.json" if args.attempt == 1 else f"mysql-r{args.attempt}.json")
    if record.exists():
        raise ValueError("Prior evidence is immutable; choose another attempt")
    cases = [
        ("TestAdminContentFrozenClaims", "p1"),
        ("TestGameFeedbackOwnerHistory", "p1"),
        ("TestAdminOriginalAchievementConditions", "p1"),
        ("TestNewPlayerWalletRegularAccount", "p1"),
        ("TestPlayerOpeningDaysSeparateAndPersistent", "p1"),
        ("TestStrategyHubLockedWantedEmpty", "p1"),
        ("TestAdminShopsMutations", args.shop_fixture),
    ]
    pool = ManagedMySQL()
    results = []
    began = time.monotonic()
    failure = None
    try:
        for index, (selector, fixture) in enumerate(cases):
            conn = pool.allocate(fixture + "_newcontent" + str(index))
            # Discard inherited fixture selectors so every case stays within this exact allocation.
            env = {k: v for k, v in os.environ.items() if not k.startswith(("HYJS_P1_MYSQL_", "HYJS_P4_MYSQL_")) and k != "HYJS_MYSQL_TEST_DSN"}
            env.update(GOTOOLCHAIN="go1.27.2", GOMAXPROCS="2", GOMEMLIMIT="768MiB")
            prefix = "HYJS_" + fixture.upper() + "_MYSQL_"
            env.update({prefix + key: value for key, value in {"HOST": conn.host, "USER": conn.user, "PASSWORD": conn.password, "DATABASE": conn.database}.items()})
            started = time.monotonic()
            run = subprocess.run(["rtk", "proxy", "go", "test", "-json", "-count=1", "-p=1", "-run", "^" + selector + "$", "-timeout", "4m", "./service"], cwd=candidate / "hyjs-server", env=env, capture_output=True, text=True, timeout=300)
            actions = []
            output = []
            for line in run.stdout.splitlines():
                try:
                    row = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if row.get("Test") == selector and row.get("Action") in ("pass", "fail"):
                    actions.append({key: row[key] for key in ("Action", "Test", "Elapsed") if key in row})
                if row.get("Output"):
                    output.append(pool.redact(row["Output"]))
            passed = run.returncode == 0 and any(row["Action"] == "pass" for row in actions)
            item = {"selector": selector, "database": conn.database, "fixture": fixture, "exitCode": run.returncode, "matchedAndPassed": passed, "elapsedSeconds": round(time.monotonic() - started, 3), "results": actions}
            if not passed:
                item["failureTail"] = output[-12:]
                item["stderrTail"] = pool.redact(run.stderr)[-3000:]
            results.append(item)
            print(json.dumps(item), flush=True)
            pool.release(conn.database)
            if not passed:
                break
    except Exception as error:
        failure = pool.redact(str(error))
    finally:
        try:
            pool.close()
        except Exception as error:
            failure = pool.redact(str(error))
        report = {"candidate": str(candidate.relative_to(ROOT)), "status": "PASS" if not failure and len(results) == len(cases) and all(row["matchedAndPassed"] for row in results) else "FAILED", "results": results, "ownedDatabasesAndUsersReleased": not bool(pool.connections), "elapsedSeconds": round(time.monotonic() - began, 3)}
        if failure:
            report["runnerFailure"] = failure
        record.write_text(json.dumps(report, indent=2))
        print(json.dumps({key: report[key] for key in ("status", "ownedDatabasesAndUsersReleased", "elapsedSeconds")}), flush=True)
    if report["status"] != "PASS":
        raise SystemExit(1)


if __name__ == "__main__":
    main()
