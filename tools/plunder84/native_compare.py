"""Freeze batch-84 NPC inputs and compare them against the pinned original host.

The host executes the original type-3 callback locally; this is not a phone
capture or a new combat authorization. It never updates game or account data.
"""
import argparse
import base64
import csv
import gzip
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile
import time

from google.protobuf.json_format import ParseDict

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from server.codec import message  # noqa: E402

CASE_NAMES = [f"npc84-{index:02d}" for index in range(1, 11)] + ["npc84-single", "npc84-sweep"]
SOURCE = "feedback84-plunder-npc-original-heroes-v1"
DELIVERY_SHA = "07e85496d17a40a3a42cddbb6651cac78c954b3762ab8216ee2d5e311f49c9a6"
RUNTIME_MANIFEST_SHA = "0d70fe1b4c70b98d7c209f24a0f9a412287b83605cff104a8a824160472a3bb5"
TABLE_MANIFEST_SHA = "014e4749af20ad919461fb6e4124020cd4fc180da21fb3fd490524ea849e4b1d"
ORIGINAL_ASSEMBLY_SHA = "ee14d5df4077573abf94be309d6ebdbe7f4ba84c31b09743ed9a9008b65d200f"
HOST_ENV = {"COMPlus_TieredCompilation": "0", "COMPlus_JitNoInline": "1", "COMPlus_JITMinOpts": "1"}
CODE_PATHS = [
    "server/codec.py",
    "server/protocol/schema.json",
    "hyjs-server/go.mod",
    "hyjs-server/go.sum",
    "hyjs-server/constUtil/p11PlunderNPC84.go",
    "hyjs-server/constUtil/p11PlunderSweep.go",
    "hyjs-server/service/p11PlunderNPC84Service.go",
    "hyjs-server/service/p11PlunderNPC84_test.go",
    "hyjs-server/service/p11PlunderFightService.go",
    "hyjs-server/service/p11PlunderSweepService.go",
]


def sha(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for part in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(part)
    return digest.hexdigest()


def require(condition, message_text):
    if not condition:
        raise ValueError(message_text)


def inside_analysis(path):
    path = path.resolve()
    require(path.is_relative_to((ROOT / "analysis").resolve()), "output and input must be inside this workspace's analysis directory")
    return path


def write_json(path, value):
    temporary = path.with_suffix(path.suffix + ".pending")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    temporary.replace(path)


def go_source_digest():
    # Include the shared combat engine and the generic exporter used by the
    # test, along with the batch-specific code. Keep paths in the digest so
    # moving or removing a source file also invalidates the frozen run.
    files = sorted((ROOT / "hyjs-server").rglob("*.go"))
    digest = hashlib.sha256()
    for path in files:
        relative = path.relative_to(ROOT).as_posix()
        digest.update(relative.encode("utf-8") + b"\0" + bytes.fromhex(sha(path)))
    return {"sha256": digest.hexdigest(), "fileCount": len(files)}


def table_rows(path):
    with path.open(newline="") as stream:
        rows = list(csv.reader(stream, delimiter="\t"))
    require(len(rows) > 5, "original table header missing")
    return [dict(zip(rows[1], row)) for row in rows[5:] if row and row[0]]


def actual_power(battle):
    # The Go test binary for this local batch runs on arm64, where its float
    # multiply-and-accumulate expressions may be fused. Preserve those exact
    # operations before RoundToEven; a half-way value must not be rounded from
    # a separately rounded intermediate.
    require(platform.machine().lower() in ("arm64", "aarch64") and hasattr(math, "fma"), "power verifier requires the batch's arm64 fused-float runtime")
    heroes = {row["id"]: row for row in table_rows(ROOT / "analysis/tables/TableHero.tsv")}
    config = {row["StaticName"]: row for row in table_rows(ROOT / "analysis/tables/TableGameConfigure.tsv")}
    factors = {key: float(config["fightpower_" + key + "_factor"]["NameValue"]) for key in ("att", "hp", "pdef", "mdef")}
    keys = ("hit", "dodge", "cri", "antiCri", "criFactor", "damageAdd", "damageReduce")
    total = 0.0
    for actor in battle.formation:
        hero = heroes[actor.id]
        base = 0.0
        for key in keys:
            base += float(hero.get(key) or 0)
        prop = actor.prop
        weighted = 0.0
        for key in ("att", "hp", "pdef", "mdef"):
            weighted = math.fma(getattr(prop, key), factors[key], weighted)
        multiplier = 100.0
        for key in keys:
            multiplier += getattr(prop, key)
        total = math.fma(weighted * 0.1 * (multiplier - base), 0.01, total)
    return round(total)


def prepare(exports, output):
    exports, output = inside_analysis(exports), inside_analysis(output)
    require(exports.is_dir() and not output.exists(), "existing export and new source directory required")
    output.mkdir(parents=True)
    rows = []
    for name in CASE_NAMES:
        files = {suffix: exports / (name + suffix) for suffix in (".pb", ".frozen-input.json", ".go-final.json", ".qualification.json", ".template.json")}
        if name in ("npc84-single", "npc84-sweep"):
            files[".actual-response.pb"] = exports / (name + ".actual-response.pb")
        require(all(path.is_file() for path in files.values()), "missing fixed Go export for " + name)
        wire = message("SCFightPlayback")
        wire.ParseFromString(files[".pb"].read_bytes())
        frozen = json.loads(files[".frozen-input.json"].read_bytes())
        result = json.loads(files[".go-final.json"].read_bytes())
        qualification = json.loads(files[".qualification.json"].read_bytes())
        template = json.loads(files[".template.json"].read_bytes())
        attacker = ParseDict(frozen["attacker"], message("BattlePlayer"))
        defender = ParseDict(frozen["defender"], message("BattlePlayer"))
        require(wire.battleType == frozen["battleType"] == 3 and wire.maxRound == frozen["maxRound"] == 20 and wire.randSeed == frozen["seed"], name + ": type3/seed mismatch")
        require(not wire.HasField("optRecord") and not wire.winConditions and wire.HasField("attacker") and wire.HasField("defender1") and not wire.HasField("defender2"), name + ": invalid original plunder input")
        require(attacker.SerializeToString(deterministic=True) == wire.attacker.SerializeToString(deterministic=True) and defender.SerializeToString(deterministic=True) == wire.defender1.SerializeToString(deterministic=True), name + ": wire changed frozen teams")
        require(wire.attackerFactor == wire.defenderFactor == 1 and wire.attacker.inst.firstOrder == 1 and wire.defender1.inst.firstOrder == 0, name + ": original driver mismatch")
        require(template["source"] == SOURCE and template["npcID"] == defender.playerId and template["heroes"] == [actor.id for actor in defender.formation], name + ": local source mismatch")
        require(template["targetFragment"].startswith("frag_trea_") and frozen.get("appliedProperties", {}).get("plunderTarget", [template["targetFragment"]]) == [template["targetFragment"]], name + ": original target fragment mismatch")
        require(template["actualPower"] == actual_power(defender) and abs(template["powerDifference"]) <= template["targetPower"] * .05 and template["powerDifference"] == template["actualPower"] - template["targetPower"], name + ": shown power differs from actual frozen properties")
        require(result["configurationHash"] == frozen["configurationHash"] == qualification["configurationHash"] and result["engineVersion"] == frozen["engineVersion"] == qualification["engineVersion"], name + ": source/qualification mismatch")
        require(result["outcome"]["authoritative"] and result["outcome"]["capabilityHash"] and qualification["actorCount"] == len(attacker.formation) + len(defender.formation), name + ": accepted type3 result missing")
        if ".actual-response.pb" in files:
            callback_bytes = files[".actual-response.pb"].read_bytes()
            callback = message("SCFightTreasureStartResult")
            callback.ParseFromString(callback_bytes)
            require(template["fightID"] == wire.fightid == callback.fightId and template["receiptResponseSHA256"] == sha(files[".actual-response.pb"]) and template["recordID"] and template["matchHash"], name + ": accepted fight identity mismatch")
            require(callback.randSeed == wire.randSeed and callback.maxRoundNum == wire.maxRound and len(callback.playerList) == 2 and callback.playerList[0].SerializeToString(deterministic=True) == attacker.SerializeToString(deterministic=True) and callback.playerList[1].SerializeToString(deterministic=True) == defender.SerializeToString(deterministic=True), name + ": accepted callback differs from frozen actors")
            require(callback.SerializeToString(deterministic=True) == callback_bytes, name + ": accepted callback is not canonical")
        else:
            callback = message("SCFightTreasureStartResult", fightId=wire.fightid, roomId="map_arena_001", randSeed=wire.randSeed, maxRoundNum=20)
            callback.playerList.add().CopyFrom(attacker)
            callback.playerList.add().CopyFrom(defender)
            callback_bytes = callback.SerializeToString(deterministic=True)
        typed = output / (name + ".plunder-start.bin")
        typed.write_bytes(callback_bytes)
        target = output / (name + ".plunder-target.txt")
        target.write_text(template["targetFragment"])
        pins = {}
        for suffix, source in files.items():
            destination = output / source.name
            shutil.copy2(source, destination)
            require(sha(source) == sha(destination), name + ": export changed while freezing")
            pins[destination.name] = sha(destination)
        pins[typed.name], pins[target.name] = sha(typed), sha(target)
        rows.append({"case": name, "pins": pins, "template": template, "qualificationHash": qualification["mechanismHash"], "seed": wire.randSeed})
    manifest = {"schema": "hyjs-plunder84-frozen-native-input-v1", "source": SOURCE, "cases": rows,
                "caseCount": len(rows), "numericTolerance": 0, "phoneCapture": False, "businessAuthority": False,
                "sourceCodeSHA256": {path: sha(ROOT / path) for path in CODE_PATHS}, "goSourceTree": go_source_digest(),
                "preparerSHA256": sha(Path(__file__))}
    write_json(output / "manifest.json", manifest)
    print(json.dumps({"status": "FROZEN", "cases": len(rows), "manifestSHA256": sha(output / "manifest.json")}, ensure_ascii=False))


def load_source(source):
    source = inside_analysis(source)
    manifest_path = source / "manifest.json"
    manifest = json.loads(manifest_path.read_bytes())
    require(manifest["schema"] == "hyjs-plunder84-frozen-native-input-v1" and manifest["source"] == SOURCE and [row["case"] for row in manifest["cases"]] == CASE_NAMES, "fixed case set mismatch")
    require(manifest["preparerSHA256"] == sha(Path(__file__)), "preparer source changed after freezing")
    require(manifest["goSourceTree"] == go_source_digest(), "Go source tree changed after freezing")
    for path, expected in manifest["sourceCodeSHA256"].items():
        require(path in CODE_PATHS and sha(ROOT / path) == expected, "Go source changed after freezing: " + path)
    for row in manifest["cases"]:
        for name, expected in row["pins"].items():
            require(Path(name).name == name and sha(source / name) == expected, "frozen case changed: " + name)
    return source, manifest


def verify_host(host_root):
    host_root = host_root.resolve()
    baseline = host_root / "analysis/growth-first-20261016/p6-local-headless-p7-runtime-r1"
    table_proof = host_root / "analysis/growth-first-20261016/p6-local-headless-p7-r1/attempt-2-managed-logger/manifest.json"
    require(sha(baseline / "delivery.json") == DELIVERY_SHA and sha(baseline / "manifest.json") == RUNTIME_MANIFEST_SHA and sha(table_proof) == TABLE_MANIFEST_SHA, "fixed original host evidence changed")
    manifest = json.loads((baseline / "manifest.json").read_bytes())
    pins = manifest["runtimeInputPins"]
    require(pins["Assembly-CSharp.dll"] == ORIGINAL_ASSEMBLY_SHA, "original assembly mismatch")
    runtime = baseline / "runtime"
    require({path.name for path in runtime.glob("*.dll")} == set(pins), "original runtime DLL set changed")
    for filename, expected in pins.items():
        require(sha(runtime / filename) == expected, "original host DLL changed: " + filename)
    config = json.loads((runtime / "LocalOriginalEngineHost.runtimeconfig.json").read_bytes())
    require(config["runtimeOptions"]["configProperties"]["System.Runtime.TieredCompilation"] is False, "host runtime configuration changed")
    expected_tables = json.loads(table_proof.read_bytes())["originalRawTablePins"]
    tables = list((runtime / "Assets/OutputBundle/tables").glob("*.txt"))
    require(len(tables) == 162, "original table denominator changed")
    for table in tables:
        # Historical APK evidence prefers the extracted lowercase asset when
        # both it and the TSV copy exist; the worktree may contain only TSVs.
        reference = host_root / "analysis/tables" / table.stem.lower()
        if not reference.exists():
            reference = host_root / "analysis/tables" / (table.stem + ".tsv")
        local = ROOT / "analysis/tables" / table.stem.lower()
        if not local.exists():
            local = ROOT / "analysis/tables" / (table.stem + ".tsv")
        key = str(reference.relative_to(host_root))
        require(key in expected_tables and sha(table) == expected_tables[key]["sha256"] and sha(reference) == sha(table) and sha(local) == sha(table), "original table changed: " + table.name)
    for relative, expected in manifest.get("runtimeNativePins", {}).items():
        require(sha(host_root / relative) == expected, "host JIT/runtime changed: " + relative)
    return runtime, pins


def compare_trace(path, go, row):
    states, records, inputs, results = [], [], [], []
    event_count = 0
    with path.open() as stream:
        for line in stream:
            event = json.loads(line)
            event_count += 1
            if event.get("kind") == "state":
                states.append(event["state"])
            elif event.get("kind") == "original_final_operates":
                records.append(event["data"])
            elif event.get("kind") == "input":
                inputs.append(event)
            elif event.get("kind") == "result":
                results.append(event)
    require(len(inputs) == len(records) == len(results) == 1 and states, "incomplete original type3 trace")
    require(inputs[0]["scope"] == "original-plunder-automatic-ai" and inputs[0]["battleType"] == 3 and inputs[0]["seed"] == row["seed"], "original host used a different input or driver")
    require(inputs[0]["typedCallbackSHA256"] == row["pins"][row["case"] + ".plunder-start.bin"] and inputs[0]["typedTargetSHA256"] == row["pins"][row["case"] + ".plunder-target.txt"], "typed plunder callback changed")
    state = states[-1]
    original_actors = [(camp, actor) for camp, key in enumerate(("attacker", "defender")) for actor in state[key]["actors"]]
    go_actors = go["finalActors"]
    require(len(original_actors) == len(go_actors), "terminal actor count differs")
    for (camp, original), computed in zip(original_actors, go_actors, strict=True):
        expected = (camp, original["uid"], original["heroId"], original["state"], original["attribute"]["hp"], original["attribute"]["maxHp"])
        actual = (computed["camp"], computed["uid"], computed["heroId"], computed["state"], computed["currentHp"], computed["maxHp"])
        require(expected == actual, "terminal identity/state/HP differs: " + repr((expected, actual)))
    native_record = base64.b64decode(records[0]["protobufBase64"], validate=True)
    go_record = message("BattleOperate", **go["originalOperates"]).SerializeToString(deterministic=True)
    require(native_record == go_record, "original final protobuf differs")
    require(results[0]["attackerWon"] == (go["outcome"]["winner"] == 0) and state["round"] == go["outcome"]["finalRound"], "winner or final round differs")
    return {"eventCount": event_count, "winner": results[0]["attackerWon"], "finalRound": state["round"],
            "actorCount": len(go_actors), "finalRecordSHA256": hashlib.sha256(native_record).hexdigest()}


def run(source, output, host_root):
    source, manifest = load_source(source)
    output = inside_analysis(output)
    require(not output.exists(), "new output directory required")
    runtime, pins = verify_host(host_root)
    output.mkdir(parents=True)
    report = {"schema": "hyjs-plunder84-original-host-comparison-v1", "status": "RUNNING", "sourceManifestSHA256": sha(source / "manifest.json"),
              "originalAssemblySHA256": ORIGINAL_ASSEMBLY_SHA, "runtimeDLLPins": pins, "runtimeEnvironment": HOST_ENV,
              "executionOrigin": "local-coreclr-original-engine-not-phone-capture", "numericTolerance": 0,
              "originalMethodBodiesChanged": False, "phoneCompared": False, "businessAuthority": False, "cases": []}
    write_json(output / "manifest.json", report)
    with tempfile.TemporaryDirectory(prefix="hyjs-plunder84-host-", dir="/private/tmp") as temporary:
        work = Path(temporary) / "runtime"
        work.mkdir()
        for filename in pins:
            shutil.copy2(runtime / filename, work / filename)
        shutil.copy2(runtime / "LocalOriginalEngineHost.runtimeconfig.json", work / "LocalOriginalEngineHost.runtimeconfig.json")
        shutil.copytree(runtime / "Assets", work / "Assets")
        dotnet = host_root.resolve() / ".tools/native-batch/dotnet/dotnet"
        report["dotnetSHA256"] = sha(dotnet)
        for row in manifest["cases"]:
            name = row["case"]
            case_dir = work / "cases" / name / "highest"
            case_dir.mkdir(parents=True)
            for suffix in (".pb", ".plunder-start.bin", ".plunder-target.txt"):
                shutil.copy2(source / (name + suffix), case_dir / (name + suffix))
            started = time.monotonic()
            observed = {"case": name, "template": row["template"], "status": "FAIL"}
            try:
                result = subprocess.run([str(dotnet), str(work / "LocalOriginalEngineHost.dll"), str(work), str(case_dir)], cwd=work,
                                        capture_output=True, text=True, timeout=120, env={**os.environ, **HOST_ENV})
                (output / (name + ".log")).write_text(result.stdout + result.stderr)
                require(result.returncode == 0, "original host exit " + str(result.returncode))
                raw_trace = case_dir / (name + ".native.jsonl")
                go = json.loads((source / (name + ".go-final.json")).read_bytes())
                observed.update(compare_trace(raw_trace, go, row))
                observed["status"] = "PASS"
            except subprocess.TimeoutExpired as error:
                def as_text(value):
                    return value.decode(errors="replace") if isinstance(value, bytes) else value or ""
                (output / (name + ".log")).write_text(as_text(error.stdout) + as_text(error.stderr) + "\nHOST_TIMEOUT_SECONDS=120\n")
                observed["error"] = "original host timeout"
            except Exception as error:
                observed["error"] = type(error).__name__ + ": " + str(error)
            raw_trace = case_dir / (name + ".native.jsonl")
            if raw_trace.exists():
                observed["traceSHA256"] = sha(raw_trace)
                with raw_trace.open("rb") as src, gzip.GzipFile(filename=str(output / (name + ".native.jsonl.gz")), mode="wb", mtime=0) as dst:
                    shutil.copyfileobj(src, dst)
                observed["compressedTraceSHA256"] = sha(output / (name + ".native.jsonl.gz"))
            observed["seconds"] = round(time.monotonic() - started, 3)
            observed["logSHA256"] = sha(output / (name + ".log")) if (output / (name + ".log")).exists() else None
            report["cases"].append(observed)
            write_json(output / "manifest.json", report)
    report["status"] = "PASS" if len(report["cases"]) == len(CASE_NAMES) and all(case["status"] == "PASS" for case in report["cases"]) else "FAIL"
    write_json(output / "manifest.json", report)
    print(json.dumps({"status": report["status"], "passed": sum(case["status"] == "PASS" for case in report["cases"]), "total": len(CASE_NAMES)}, ensure_ascii=False))
    return 0 if report["status"] == "PASS" else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    freeze = sub.add_parser("prepare")
    freeze.add_argument("--exports", type=Path, required=True)
    freeze.add_argument("--output", type=Path, required=True)
    execute = sub.add_parser("run")
    execute.add_argument("--source", type=Path, required=True)
    execute.add_argument("--output", type=Path, required=True)
    execute.add_argument("--host-root", type=Path, required=True)
    args = parser.parse_args()
    if args.command == "prepare":
        prepare(args.exports, args.output)
        return 0
    return run(args.source, args.output, args.host_root)


if __name__ == "__main__":
    raise SystemExit(main())
