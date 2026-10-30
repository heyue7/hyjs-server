"""Audit v85 night-forest upgrade compatibility without compiling or running Unity."""

import csv
import hashlib
import json
import subprocess
import sys
import zipfile
from pathlib import Path

import dnfile
import UnityPy

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_arena_state.build import decode, exact_body, instruction_value, member_reference, methods

WORK = ROOT / "analysis/fragment-box85/awake-copy-entry/client"
COMMIT = "b0e684edde8f847c7371aaed093cbd2d47f5d9d8"
NIGHT = "map/map_nightforestfield_001"
KNOWN_ANDROID = "28e264a89fb6b1a5fc27fa5a6fbdfb55cf510e4ba6e6431d1a023631d620f28d"
APKS = {
    "80": (Path("/Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.80.apk"),
           "25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a"),
    "83": (Path("/Users/lirui/.codex/worktrees/desert-extraction-v4/hyjs/build/desert-extraction-v4/desert-trial-qa-v83-r3.apk"),
           "1d4b6798ffe54f2545a7d14b9109ae9bd84e7a6d66f7a17b253e213b414adaa8"),
}
MAIN83 = Path("/Users/lirui/.codex/worktrees/desert-extraction-v4/hyjs/analysis/desert-extraction-v4-client/combined83-r3/static/Assembly-CSharp.dll")
MAIN83_SHA = "54c0f8567c711d661b2743e8a48044c3c406f3ae8b80e03534d2794f52c46425"


def sha(data):
    return hashlib.sha256(data).hexdigest()


def check(ok, message):
    if not ok:
        raise ValueError(message)


def mask_method(source, signature):
    start = source.index(signature)
    opening = source.index("{", start + len(signature))
    depth, state, at = 0, "code", opening
    while at < len(source):
        c = source[at]
        following = source[at + 1] if at + 1 < len(source) else ""
        if state == "code":
            if c == '"':
                state = "string"
            elif c == "'":
                state = "char"
            elif c == "/" and following == "/":
                state = "line"
                at += 1
            elif c == "/" and following == "*":
                state = "block"
                at += 1
            elif c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
                if depth == 0:
                    return source[:opening] + "{masked}" + source[at + 1:]
        elif state in ("string", "char"):
            if c == "\\":
                at += 1
            elif (state == "string" and c == '"') or (state == "char" and c == "'"):
                state = "code"
        elif state == "line" and c == "\n":
            state = "code"
        elif state == "block" and c == "*" and following == "/":
            state = "code"
            at += 1
        at += 1
    raise ValueError("Unclosed source method: " + signature)


def named(pe, name):
    found = [m for n, _, m in methods(pe) if n == name]
    check(len(found) == 1, "Expected one method: " + name)
    return found[0]


def call_names(pe, method):
    result = []
    for instruction in decode(pe, method):
        token = instruction_value(instruction)
        if instruction.opcode.name not in ("call", "callvirt") or not isinstance(token, int):
            continue
        if token >> 24 == 10:
            ref = member_reference(pe, token)
            owner = ref.Class.row
            result.append(str(getattr(owner, "TypeNamespace", "")) + "." + str(getattr(owner, "TypeName", "")) + "::" + str(ref.Name))
        elif token >> 24 == 6:
            result.append(str(pe.net.mdtables.MethodDef.rows[(token & 0xffffff) - 1].Name))
    return result


def main():
    out = WORK / "nightforest-upgrade-static-r2.json"
    check(not out.exists(), "Fresh report required")
    old = subprocess.run(["rtk", "proxy", "git", "show", COMMIT + ":tools/client_dungeon_access/Runtime.cs"],
                         cwd=ROOT, capture_output=True, check=True, timeout=60).stdout.decode("utf-8")
    runtime = (ROOT / "tools/client_dungeon_access/Runtime.cs").read_text()
    contract = (ROOT / "tools/client_awake85_layout/Awake85Contract.cs").read_text()
    marker = "\n        // v85 layout is bound to a server-generated run"
    check(runtime.count(marker) == 1, "Expected v85 source boundary")
    without_added = runtime.split(marker, 1)[0].rstrip("\n") + "\n    }\n}\n"
    for method in ("public static void AwakeAdapt(RoomChallenge room)",
                   "public static void AwakePrepare(RoomChallenge room)",
                   "public static void AwakeAlignPlayer(RoomChallenge room)"):
        old, without_added = mask_method(old, method), mask_method(without_added, method)
    check(old == without_added, "Source outside the three awake methods and v85 block changed")
    adapt = runtime.split("public static void AwakeAdapt(RoomChallenge room)", 1)[1].split("private static bool AwakeCurrent", 1)[0]
    order = [adapt.index(name) for name in ("Awake85Transition(", "Awake85NightForest.Applies(",
                                             "Awake85EnsureNightForest(", "Scene(chapter.sceneId)")]
    check(order == sorted(order) and len(set(order)) == 4, "Awake registration order differs")
    check('chapter == "1" && state != null && state.Active(room, chapter, entry, sceneType)' in contract,
          "Registration scope differs")
    check(contract.index("night manifest conflict") < contract.index("full.Add(Key, added)"), "Conflict check follows mutation")
    check("Object.ReferenceEquals(full, small)" in contract, "Shared-dictionary guard absent")
    for required in ("assetFileTool", "smallAssetFileTool", "AssetFileTool.LoadFromString", "Awake85NightForest.Register<AssetCell>"):
        check(required in runtime, "Runtime registration missing: " + required)

    apks = {}
    for label, (apk, expected_sha) in APKS.items():
        with apk.open("rb") as handle:
            check(hashlib.file_digest(handle, "sha256").hexdigest() == expected_sha, "APK differs: " + label)
        with zipfile.ZipFile(apk) as archive:
            manifest_bytes = archive.read("assets/android/android")
            tree = next(o.read_typetree() for o in UnityPy.load(manifest_bytes).objects if o.type.name == "AssetBundleManifest")
            names, info = dict(tree["AssetBundleNames"]), dict(tree["AssetBundleInfos"])
            ids = {name: key for key, name in names.items()}
            check(NIGHT in ids, "APK android graph lacks night forest: " + label)
            node = ids[NIGHT]
            deps = [names[i] for i in info[node]["AssetBundleDependencies"]]
            check(not deps, "APK night forest has unexpected dependencies: " + label)
            main_raw = MAIN83.read_bytes() if label == "83" else archive.read("assets/bin/Data/Managed/Assembly-CSharp.dll")
            if label == "83":
                check(sha(main_raw) == MAIN83_SHA, "83 selected Main differs")
            main_pe = dnfile.dnPE(data=main_raw)
            getfull = named(main_pe, "AssetLoad.AssetBundleManager::GetFullPath")
            calls = call_names(main_pe, getfull)
            check(calls.count("HYJS.DungeonManifest.ManifestPath::Resolve") == 3, "Android cache hook differs")
            helper_raw = archive.read("assets/bin/Data/Managed/HYJS.DungeonManifest.dll")
            helper = dnfile.dnPE(data=helper_raw)
            resolve = named(helper, "HYJS.DungeonManifest.ManifestPath::Resolve")
            strings = []
            for instruction in decode(helper, resolve):
                token = instruction_value(instruction)
                if instruction.opcode.name == "ldstr" and isinstance(token, int) and token >> 24 == 0x70:
                    strings.append(str(helper.net.user_strings.get(token & 0xffffff)))
            check(KNOWN_ANDROID in strings and "android" in strings and "!assets/android/" in strings,
                  "Known android cache resolver literals differ")
            apks[label] = {
                "apkPath": str(apk), "apkSHA256": expected_sha, "selectedMainSHA256": sha(main_raw),
                "androidManifestSHA256": sha(manifest_bytes), "nightRootPresent": True,
                "nightDirectDependencies": deps,
                "manifestPathHelperSHA256": sha(helper_raw), "manifestPathResolveBodySHA256": sha(exact_body(helper, resolve)),
                "getFullPathResolveCalls": 3, "knownCacheSHA256Embedded": True,
            }

    table_path = ROOT / "analysis/tables/TableAwakeCopyMonsterOutside.tsv"
    rows = list(csv.reader(table_path.open(), delimiter="\t"))
    table = {r[0]: dict(zip(rows[1], r)) for r in rows[5:] if r}
    monsters = []
    for chapter, mon in (("1", "mon_024"), ("2", "mon_010"), ("3", "mon_030"), ("4", "mon_016"), ("5", "mon_006")):
        row = table[mon]
        view, wander = float(row["ViewR"]), float(row["wanderR"])
        check(view + wander <= 9, "Monster radius exceeds layout threshold")
        monsters.append({"chapter": chapter, "monster": mon, "viewR": view, "wanderR": wander, "sum": view + wander})

    files = [ROOT / "tools/client_dungeon_access/Runtime.cs", ROOT / "tools/client_awake85_layout/Awake85Contract.cs",
             ROOT / "tools/client_awake85_layout/ContractTests.cs", ROOT / "tools/client_awake85_layout/VerifyHelper.cs",
             ROOT / "tools/client_awake85_layout/build.py", Path(__file__)]
    result = {
        "scope": "static source, pinned APK resources and managed CIL; no C# build, Unity execution, device, DB or network",
        "status": "SOURCE_ONLY_PENDING_REBUILD_AND_TEST",
        "sourceSHA256": {str(path.relative_to(ROOT)): sha(path.read_bytes()) for path in files},
        "baselineCommit": COMMIT,
        "unchangedSourceOutsideThreeAwakeMethodsAndAddedV85Block": True,
        "awakeCallOrder": ["Awake85Transition", "Applies", "Awake85EnsureNightForest", "Scene"],
        "registrationGuards": "sceneType 10, chapter 1, same bound v85 room and entry; validate both dictionaries before additions",
        "monsterTableSHA256": sha(table_path.read_bytes()), "monsters": monsters, "layoutRadiusThreshold": 9,
        "apkGraphs": apks,
        "knownCachedAndroid": {
            "sha256": KNOWN_ANDROID,
            "directCacheBytesInspected": False,
            "observation": "Both checked helpers contain the pinned cache literal and GetFullPath calls Resolve; the helper's bounded known-cache path redirects to APK android under its documented path and stability checks.",
            "unknownCache": "Other cache bytes and graph contents remain unverified; original resolver leaves unknown cache selected.",
        },
        "previousArtifacts": {
            "build80": "build80-r3", "build83": "build83-r2", "sceneReports": ["scenes80-r4.json", "scenes83-r4.json"],
            "status": "historical only; generated before this source change and not a current build or test",
        },
        "limitations": ["No C# compilation or tests were run for this revision", "No real Unity NavMesh, cache, upgrade or device execution", "Unknown persistent android graphs may differ"],
    }
    out.write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n")
    print("Static source and pinned APK graph audit PASS: " + str(out))


if __name__ == "__main__":
    main()
