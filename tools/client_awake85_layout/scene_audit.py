"""Audit each Awake scene route against a pinned APK, Main, helper and manifest overlay."""

import argparse
import csv
import hashlib
import json
import re
import sys
import zipfile
from pathlib import Path

import dnfile
import UnityPy

ROOT = Path(__file__).resolve().parents[2]
SOURCE = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
from tools.client_arena_state.build import decode, instruction_value, member_reference, methods
sys.path.insert(0, str(SOURCE))
from build import HELPER, MAIN, SMALL, small_overlay

DESERT_ENTRY = ("assets/android/hyjs/desert53/"
                "2d696b559ae5990b1be9978968e11fbb181ccfe1004a4cdd21cd8d1b435d05d1/map_desertfield_001")
DESERT_SHA = "2d696b559ae5990b1be9978968e11fbb181ccfe1004a4cdd21cd8d1b435d05d1"
CHAPTER_TABLE_SHA = "f8edd3d9d502afcaf71837f03a33461bdba2e2b0f3a7ac7247a2b7677f0c5e24"
REQUIRED_ALIASES = {
    "map_ice_001": "map_snowforest_001",
    "map_volcano_001": "map_cave_001",
    "map_volcanofield_001": "map_forestfield_001",
    "map_desertfield_001": "map_desertrun_002",
}


def require(value, message):
    if not value:
        raise ValueError(message)


def digest_file(path):
    with path.open("rb") as f:
        return hashlib.file_digest(f, "sha256").hexdigest()


def has_call(pe, owner, method, target_owner, target_method):
    chosen = [m for n, _, m in methods(pe) if n == owner + "::" + method]
    require(len(chosen) == 1, "Ambiguous Main method: " + owner + "::" + method)
    for ins in decode(pe, chosen[0]):
        token = instruction_value(ins)
        if ins.opcode.name not in ("call", "callvirt") or not isinstance(token, int) or token >> 24 != 10:
            continue
        ref = member_reference(pe, token)
        typ = ref.Class.row
        name = str(getattr(typ, "TypeNamespace", "")) + "." + str(getattr(typ, "TypeName", ""))
        if name == target_owner and str(ref.Name) == target_method:
            return True
    return False


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base-apk", type=Path, required=True)
    ap.add_argument("--base-sha256", required=True)
    ap.add_argument("--input-managed", type=Path)
    ap.add_argument("--input-managed-sha256")
    ap.add_argument("--overlay-directory", type=Path, required=True)
    ap.add_argument("--expected-desert-helper-sha256")
    ap.add_argument("--desert-resource-overlay", type=Path)
    ap.add_argument("--desert-helper", type=Path)
    ap.add_argument("--output", type=Path, required=True)
    args = ap.parse_args()
    out = args.output.resolve()
    allowed = (ROOT / "analysis/fragment-box85/awake-copy-entry/client").resolve()
    require(out.is_relative_to(allowed) and not out.exists(), "Fresh task-owned output required")
    require(digest_file(args.base_apk) == args.base_sha256, "APK digest differs")
    require(bool(args.input_managed) == bool(args.input_managed_sha256), "Managed path/SHA must be paired")
    require(bool(args.desert_resource_overlay) == bool(args.desert_helper),
            "Combined desert resources and helper must be supplied together")
    if args.input_managed:
        require(digest_file(args.input_managed) == args.input_managed_sha256, "Managed digest differs")
    overlay = json.loads((args.overlay_directory / "manifest.json").read_text())
    compatibility = json.loads((args.overlay_directory / "helper-compatibility.json").read_text())
    require(overlay["baseAPKSHA256"] == args.base_sha256 and not overlay["mainBytesModified"], "Overlay input differs")
    require(digest_file(args.overlay_directory / "HYJS.DungeonContent.dll") == overlay["outputHelperSHA256"],
            "Output helper digest differs")
    for relative, expected in overlay["sourcePins"].items():
        require(digest_file(ROOT / relative) == expected, "Overlay source differs: " + relative)
    require(not any(compatibility[key] for key in ("missing", "abiDifferences", "semanticDifferences", "requiredCallErrors")),
            "Helper compatibility has errors")
    preserved = compatibility["preservedMethodSemantics"]
    require(any("DungeonContent::Scene|" in x for x in preserved) and
            any("DungeonContent::.cctor|" in x for x in preserved), "Existing scene mapper not preserved")
    source = (ROOT / "tools/client_dungeon_access/Runtime.cs").read_text()
    alias_section = source.split("private static readonly Dictionary<string, string> Aliases", 1)[1].split("};", 1)[0]
    aliases = dict(re.findall(r'\{\s*"(map_[^"]+)"\s*,\s*"(map_[^"]+)"\s*\}', alias_section))
    for name, target in REQUIRED_ALIASES.items():
        require(aliases.get(name) == target, "Awake alias differs: " + name)
    whitelist_section = source.split("private static readonly HashSet<string> Whitelist", 1)[1].split("}, StringComparer.Ordinal);", 1)[0]
    whitelist = set(re.findall(r'"(map_[^"]+)"', whitelist_section))
    chapter_path = ROOT / "analysis/tables/TableAwakeCopyChapter.tsv"
    require(digest_file(chapter_path) == CHAPTER_TABLE_SHA, "Awake chapter table digest differs")
    chapters = list(csv.reader(chapter_path.open(), delimiter="\t"))
    rows = [dict(zip(chapters[1], row)) for row in chapters[5:] if row]
    require({r["chapterId"] for r in rows} == {"1", "2", "3", "4", "5"}, "Chapter table differs")
    resource_payload = None
    resource_manifest_sha = None
    if args.desert_resource_overlay:
        from tools.desert_extraction import package_resources
        resource_dir = args.desert_resource_overlay.resolve()
        require(resource_dir.is_relative_to(ROOT) and args.desert_helper.resolve().is_relative_to(ROOT),
                "Combined resource inputs must be inside the project")
        resource_payload, resource_pins = {}, {}
        resource_proof = package_resources.apply_overlay(args.base_apk, resource_dir, resource_payload, resource_pins)
        require(resource_proof.get('schema') == package_resources.AWAKE_SCHEMA
                and resource_proof.get('onlyDesertAndNightRowsAndMapEntry') is True,
                "Combined desert and night resource replay differs")
        resource_manifest_sha = digest_file(resource_dir/'resource-manifest.json')
    with zipfile.ZipFile(args.base_apk) as z:
        names = set(z.namelist())
        require(len(names) == len(z.infolist()), "Duplicate APK entries")
        main_bytes = args.input_managed.read_bytes() if args.input_managed else z.read(MAIN)
        require(hashlib.sha256(main_bytes).hexdigest() == overlay["inputManagedSHA256"], "Main/overlay mismatch")
        small = (args.overlay_directory / "smallassetmanifest").read_bytes()
        expected, _ = small_overlay(z.read(SMALL))
        require(small == expected and hashlib.sha256(small).hexdigest() == overlay["smallManifestOutputSHA256"],
                "Small manifest overlay differs")
        if resource_payload is not None:
            small = resource_payload[SMALL]
        small_rows = {}
        for line in small.decode("utf-8").splitlines()[1:]:
            fields = line.split("\t")
            require(len(fields) == 3 and fields[0] not in small_rows, "Invalid small manifest")
            small_rows[fields[0]] = (fields[1], int(fields[2]))
        tree = next(o.read_typetree() for o in UnityPy.load(z.read("assets/android/android")).objects
                    if o.type.name == "AssetBundleManifest")
        graph, infos = dict(tree["AssetBundleNames"]), dict(tree["AssetBundleInfos"])
        ids = {name: key for key, name in graph.items()}
        pe = dnfile.dnPE(data=main_bytes)
        owner = "HYJS.Desert53.DesertBundle"
        override = has_call(pe, "AssetLoad.AssetBundleManager", "IsAssetExist", owner, "IsTarget")
        if override:
            require(has_call(pe, "AssetLoad.AssetBundleManager", "GetFullPath", owner, "ResolvePath") and
                    has_call(pe, "AssetLoad.AssetBundleManager", "LoadAssetBundle", owner, "GetDependencies"),
                    "Incomplete desert loader hooks")
            if resource_payload is None:
                require(args.expected_desert_helper_sha256 is not None and
                        hashlib.sha256(z.read("assets/bin/Data/Managed/SoulFlowerDesert53.dll")).hexdigest() == args.expected_desert_helper_sha256,
                        "Desert helper identity differs")
                require(DESERT_ENTRY in names, "Desert replacement bundle missing")
                data = z.read(DESERT_ENTRY)
            else:
                require(DESERT_ENTRY not in names and args.expected_desert_helper_sha256 is not None
                        and digest_file(args.desert_helper) == args.expected_desert_helper_sha256,
                        "Combined desert helper identity differs")
                data = resource_payload[DESERT_ENTRY]
            require(hashlib.sha256(data).hexdigest() == DESERT_SHA, "Desert replacement bundle differs")
            require(small_rows.get("map/map_desertfield_001") == (hashlib.md5(data).hexdigest(), len(data)),
                    "Desert manifest row differs")
        else:
            require(DESERT_ENTRY not in names and resource_payload is None,
                    "Desert bundle present without pinned loader")

        def check_resource(name):
            key = "map/" + name
            require(key in small_rows, "Scene missing small manifest: " + name)
            if override and name == "map_desertfield_001":
                return {"target": name, "kind": "desert53-rebuild", "closure": "self-contained-loader", "sha256": DESERT_SHA}
            require("assets/android/" + key in names and key in ids, "Scene root missing: " + name)
            pending, seen = [ids[key]], set()
            while pending:
                i = pending.pop()
                if i in seen:
                    continue
                seen.add(i)
                pending.extend(infos[i]["AssetBundleDependencies"])
            missing = sorted(graph[i] for i in seen if "assets/android/" + graph[i] not in names or graph[i] not in small_rows)
            require(not missing, "Scene dependency/manifest missing: " + name + " " + repr(missing[:5]))
            return {"target": name, "kind": "packaged", "closure": len(seen),
                    "sha256": hashlib.sha256(z.read("assets/android/" + key)).hexdigest()}

        def resolve(name):
            key = "map/" + name
            if key in small_rows or (override and name == "map_desertfield_001"):
                chosen = name
            else:
                chosen = aliases.get(name, name)
            if chosen != name:
                require(chosen in whitelist, "Alias target missing whitelist: " + chosen)
            result = check_resource(chosen)
            result["original"] = name
            result["aliasApplied"] = chosen != name
            result["readyScene"] = chosen in whitelist
            return result

        results = []
        for row in sorted(rows, key=lambda r: int(r["chapterId"])):
            results.append({"chapter": row["chapterId"], "openDay": row["openDay"],
                            "exploration": resolve(row["sceneId"]), "battle": resolve(row["BattleMap"])})
        any_ready = any(x["exploration"]["readyScene"] and x["battle"]["readyScene"] for x in results)
        require(any_ready, "The existing global AwakeEntryReady gate has no ready chapter")
        report = {
            "scope": "declared resources and initialized small-manifest route; no Unity or persistent assetmanifest execution",
            "baseAPKSHA256": args.base_sha256, "mainSHA256": hashlib.sha256(main_bytes).hexdigest(),
            "helperSHA256": overlay["outputHelperSHA256"], "desertLoaderOverride": override,
            "combinedResourceManifestSHA256": resource_manifest_sha,
            "catalogSHA256": hashlib.sha256(small).hexdigest(),
            "desertHelperSHA256": args.expected_desert_helper_sha256 if override else None,
            "chapterTableSHA256": CHAPTER_TABLE_SHA, "auditSourceSHA256": digest_file(Path(__file__)),
            "overlayManifestSHA256": digest_file(args.overlay_directory / "manifest.json"),
            "helperCompatibilitySHA256": digest_file(args.overlay_directory / "helper-compatibility.json"),
            "sceneAliases": REQUIRED_ALIASES, "chapters": results, "awakeEntryReadyAnyChapter": any_ready,
            "limitations": ["runtime assetFileTool/persistent cache can change IsAssetExist", "dynamic actor art and gameplay not checked"],
        }
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps({"chapters": len(results), "desertOverride": override,
                          "routes": [(x["chapter"], x["exploration"]["target"], x["battle"]["target"]) for x in results]}))


if __name__ == "__main__":
    main()
