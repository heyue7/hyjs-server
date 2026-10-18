"""Recheck named missing content in both supplied APKs without executing them."""
import csv
import hashlib
import io
import json
from pathlib import Path
import zipfile
import UnityPy

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "analysis/missing-config-branding-20261017"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def read_table(apk, name):
    entry = "assets/android/tabledata/table" + name.lower()
    raw = apk.read(entry)
    env = UnityPy.load(raw)
    assets = [o.read() for o in env.objects if o.type.name == "TextAsset"]
    if len(assets) != 1:
        raise ValueError("Expected one table TextAsset: " + entry)
    script = assets[0].m_Script
    script = script if isinstance(script, str) else bytes(script).decode("utf-8-sig")
    lines = list(csv.reader(io.StringIO(script), delimiter="\t"))
    # Original binary tables carry two human/English headers and three type/meta rows.
    rows = [dict(zip(lines[1], row)) for row in lines[5:] if row and row[0]]
    return {"entry": entry, "bundleSHA256": sha(raw), "textSHA256": sha(script.encode("utf-8")),
            "fields": lines[1], "rowCount": len(rows)}, rows


def main():
    inputs = [("original", ROOT / "apks/original/10056262.apk"),
              ("gm", ROOT / "apks/original/黑衣剑士-离线联机-GM版.apk.1")]
    old_audit = json.loads((ROOT / "analysis/gm-apk-resource-audit-20261017/resource-comparison.json").read_text())
    report = {"schema": "two-apk-bounded-missing-content-v1", "packages": {}, "matrix": []}
    selected = ["RushEliteBoss", "ExpeditionMission", "ExpeditionLevel", "ExpeditionStory",
                "GameConfigure", "Frag", "Hero", "Field", "UnionFightBaseCamp",
                "UnionFightFactory", "UnionFightGate", "UnionFightTower", "UnionFightNpc"]
    for label, path in inputs:
        with path.open("rb") as f:
            identity = hashlib.file_digest(f, "sha256").hexdigest()
        with zipfile.ZipFile(path) as apk:
            names = apk.namelist()
            package = {"path": str(path), "sha256": identity, "tables": {},
                       "relevantTableEntries": [n for n in names if n.startswith("assets/android/tabledata/") and
                                                any(word in n for word in ["rush", "elite", "expedition", "unionfight"])],
                       "fieldMaps": [], "heroFragmentGaps": []}
            data = {}
            for name in selected:
                package["tables"][name], data[name] = read_table(apk, name)
            for field in data["Field"]:
                if field["FieldId"] not in ["field_003", "field_004", "field_005", "field_006"]:
                    continue
                map_entry = "assets/android/map/" + field["FieldMap"]
                package["fieldMaps"].append({"id": field["FieldId"], "originalRow": field,
                                             "sceneEntry": map_entry, "sceneEntryPresent": map_entry in names,
                                             "missingSceneDeclaredInDependencyGraph": ("map/" + field["FieldMap"]) in old_audit["fullGraph"][label]["missingFromFullGraph"]})
            for hero_id in ["hero_069", "hero_070", "hero_073"]:
                hero = next(r for r in data["Hero"] if r["id"] == hero_id)
                fragments = [r for r in data["Frag"] if r["target"] == hero_id]
                package["heroFragmentGaps"].append({"heroID": hero_id, "heroRow": hero,
                                                    "originalIsShowInCard": hero["IsShowInCard"], "fragmentRows": fragments})
            package["eliteRows"] = data["RushEliteBoss"]
            package["specialExpeditionRows"] = [r for r in data["ExpeditionStory"] if r["storyId"] in
                                                 ["stroy_095", "stroy_096", "stroy_097", "stroy_106", "stroy_107", "stroy_108"]]
            package["relatedConfigureRows"] = [r for r in data["GameConfigure"] if any(k in r["StaticName"].lower() for k in
                                                         ["expedition", "rush", "unionfight", "strengthbuilding"])]
            report["packages"][label] = package
    left, right = report["packages"]["original"], report["packages"]["gm"]
    report["allSelectedTableBundlesIdenticalBetweenAPKs"] = all(left["tables"][k]["bundleSHA256"] == right["tables"][k]["bundleSHA256"] for k in selected)
    report["matrix"] = [
        {"feature": "trial-back-half", "category": "original server configuration missing",
         "original": "No trial enemy/floor/win-condition table among packaged tabledata; original UI reads SCGetRushInfo and SCRushFightStart from network",
         "gm": "Same table inventory; focused OfflineStage/Progress/Growth plus prior full MemberRef inventory provide no separate original trial producer",
         "guard": "Retain actual enabled rows; unavailable next floor displays 暂未开放 before preparation/scene/network"},
        {"feature": "elite-34-bosses", "category": "original server formation configuration missing",
         "original": "34 RushEliteBoss rows contain icon, rewards and prerequisite only; no enemy HP/formation/skill/scene columns",
         "gm": "Exact same RushEliteBoss bundle; no recovered distinct producer from focused GM domain and prior full MemberRefs",
         "guard": "Only the explicitly unconfigured boss IDs are blocked; existing compatible first boss remains governed by current enabled state"},
        {"feature": "expedition-SS-special", "category": "formula/publication configuration missing",
         "original": "SS missions/levels, cost 5/hour, extra-luck 1/10000, skill-luck 8 and six special story texts are present. SS trigger, special reward distribution/quantity and rare trigger remain absent",
         "gm": "All same selected tables; focused GM types add no original publication/reward rule",
         "guard": "Preserve D-S consumers; no new SS/special offer produced, existing explicit grade rejection is retained"},
        {"feature": "union-fight-building-upgrade", "category": "upgrade formula missing",
         "original": "Static building/NPC tables and protocol UFStrengthBuildingInfo exist; maxLevel, upgradeCost, upgradeDesc arrive in server response, no per-level upgrade table found",
         "gm": "Same five static building tables; no recovered CSUFStrengthBuildingInfo/CSUFStrengthBuilding producer",
         "guard": "Block only war-building strengthen entry/start; ordinary guild tech/statue/tree remains available"},
        {"feature": "hero069-070-073-frag", "category": "fragment conversion parameters absent",
         "original": "Three original hero rows present with IsShowInCard=1; no Frag.target rows. Original visibility field retained without assigning new visibility semantics",
         "gm": "Exact same Hero/Frag bundles; no conversion parameters supplied",
         "guard": "Do not disable combat or all character growth. Existing reset/conversion consumer still rejects missing fragment rule; no arbitrary fragment count invented"},
        {"feature": "field003-006", "category": "actual scene resources missing",
         "original": "Field rows reference four map bundles absent from ZIP and declared missing in retained full dependency audit",
         "gm": "Same four absent scenes; OfflineDesert only substitutes map_desertrun_002 for trial field003 and is not original scene recovery",
         "guard": "Explicit four target guard before client transition and server transaction; field001/002 retained"},
    ]
    report["reusedEvidence"] = ["analysis/gm-apk-resource-audit-20261017/resource-comparison.json",
                                 "analysis/gm-apk-resource-audit-20261017/server/member-index.json",
                                 "analysis/gm-apk-resource-audit-20261017/planning/dungeons.md"]
    report["newREA"] = {"originalParent": "ev_ca5f8e732d3053b82b40007462e745da7088dd9670b6f3ba0c5e5494af081191",
                        "gmParent": "ev_170b7814e22a522a2785560aa2adf44a9a7d011e92bc25bdcf468ce9134f969f"}
    report["boundaries"] = ["Bounded static inspection of two supplied APKs only; no external hot-update package or original server recovered",
                             "No APK execution, target networking, database mutation, phone or battle test",
                             "Presence of story text/icon does not prove complete original server configuration"]
    OUT.mkdir(exist_ok=True)
    (OUT / "missing-matrix.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"sameSelectedTables": report["allSelectedTableBundlesIdenticalBetweenAPKs"],
                      "features": len(report["matrix"]), "fieldMapsPresent": {k: [r["sceneEntryPresent"] for r in v["fieldMaps"]] for k, v in report["packages"].items()}}))


if __name__ == "__main__":
    main()
