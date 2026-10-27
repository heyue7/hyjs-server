"""Patch only the OS selector's two description cells in the pinned 77 APK."""
import argparse
import hashlib
import json
import sys
import zipfile
from pathlib import Path

import dnfile
import UnityPy

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_arena_state.build import methods

BASE_SHA256 = "c970012d7c785c58c3b923ea4a0ea8e650204ed0e50e16e914f47e54ac4aece8"
OLD_DESCRIPTION = "可选择一名OS角色：OS桐人、OS亚丝娜、OS诗乃。"
MANAGED = "assets/bin/Data/Managed/Assembly-CSharp.dll"
SOURCE = ROOT / "hyjs-server/service/data/growth74_resources.json"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def asset(raw):
    env = UnityPy.load(raw)
    objects = list(env.objects)
    texts = [obj for obj in objects if obj.type.name == "TextAsset"]
    if len(texts) != 1:
        raise ValueError("Expected exactly one table TextAsset")
    return env, objects, texts[0]


def table_script(obj):
    script = obj.read_typetree()["m_Script"]
    return script.decode("utf-8") if isinstance(script, bytes) else script


def rows(raw):
    _, _, text = asset(raw)
    lines = table_script(text).splitlines()
    columns = lines[1].split("\t")
    return [dict(zip(columns, values)) for values in (line.split("\t") for line in lines[5:])
            if len(values) == len(columns)]


def change_cell(script, key, identity, column, description):
    lines = script.splitlines(keepends=True)
    columns = lines[1].rstrip("\r\n").split("\t")
    key_index, value_index = columns.index(key), columns.index(column)
    found = []
    for i in range(5, len(lines)):
        body = lines[i].rstrip("\r\n")
        values = body.split("\t")
        if len(values) == len(columns) and values[key_index] == identity:
            found.append((i, values, lines[i][len(body):]))
    if len(found) != 1:
        raise ValueError("Expected exactly one target row: " + identity)
    index, values, ending = found[0]
    if values[value_index] != OLD_DESCRIPTION:
        raise ValueError("Input description differs: " + identity)
    values[value_index] = description
    lines[index] = "\t".join(values) + ending
    return "".join(lines)


def patch(raw, key, identity, column, description):
    env, objects, text = asset(raw)
    original = table_script(text)
    updated = change_cell(original, key, identity, column, description)
    before = {obj.path_id: (obj.type.name, sha(obj.get_raw_data())) for obj in objects}
    tree = text.read_typetree()
    tree["m_Script"] = updated
    text.save_typetree(tree)
    output = env.file.save(packer="lz4")
    _, new_objects, new_text = asset(output)
    after = {obj.path_id: (obj.type.name, sha(obj.get_raw_data())) for obj in new_objects}
    if set(before) != set(after) or any(before[i] != after[i] for i in before if i != text.path_id):
        raise ValueError("Non-text asset inventory or bytes changed")
    old_tree = text.read_typetree()
    new_tree = new_text.read_typetree()
    old_tree.pop("m_Script")
    new_tree.pop("m_Script")
    if old_tree != new_tree or table_script(new_text) != updated:
        raise ValueError("Text asset metadata or exact table script changed")
    return output, {"inputSHA256": sha(raw), "outputSHA256": sha(output),
                    "targetRow": identity, "targetColumn": column,
                    "originalScriptSHA256": sha(original.encode()),
                    "updatedScriptSHA256": sha(updated.encode()),
                    "otherRowsAndCellsExact": True, "otherAssetObjectsExact": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    parser.add_argument("--description", required=True)
    args = parser.parse_args()
    if not args.description or any(c in args.description for c in "\t\r\n") or args.description == OLD_DESCRIPTION:
        raise ValueError("A changed single-line description is required")
    base, out = args.base_apk.resolve(), args.output_directory.resolve()
    with base.open("rb") as f:
        if hashlib.file_digest(f, "sha256").hexdigest() != BASE_SHA256:
            raise ValueError("Base APK identity differs")
    if not out.is_relative_to(ROOT / "build") or out.exists():
        raise ValueError("Fresh build output required")
    out.mkdir(parents=True)
    specs = [("tableitem", "itemId", "item_9760", "intro"),
             ("tabletext", "id", "desc_item_9760", "text")]
    payload, proofs = {}, {}
    with zipfile.ZipFile(base) as apk:
        item_use = [r for r in rows(apk.read("assets/android/tabledata/tableitemuse")) if r["itemId"] == "item_9760"]
        expected_rewards = ["hero_077,1", "hero_078,1", "hero_079,1"]
        if [r["result"] for r in item_use] != expected_rewards:
            raise ValueError("Original selector options differ")
        heroes = {r["id"]: r for r in rows(apk.read("assets/android/tabledata/tablehero"))}
        text_rows = {r["id"]: r["text"] for r in rows(apk.read("assets/android/tabledata/tabletext"))}
        names = [text_rows[heroes[reward.split(",")[0]]["name"]] for reward in expected_rewards]
        if args.description != "可选择一名OS角色：" + "、".join(names) + "。":
            raise ValueError("Description does not match actual original option names")
        source = json.loads(SOURCE.read_text())
        source_items = [r for r in source["items"] if r["itemId"] == "item_9760"]
        source_uses = [r for r in source["uses"] if r["itemId"] == "item_9760"]
        fields = ("id", "itemId", "result", "odds", "category")
        if len(source_items) != 1 or source_items[0]["desc"] != args.description or source_items[0]["intro"] != args.description:
            raise ValueError("Server descriptions differ")
        if [{k: r[k] for k in fields} for r in source_uses] != [{k: r[k] for k in fields} for r in item_use]:
            raise ValueError("Server selector options differ")
        for name, key, identity, column in specs:
            entry = "assets/android/tabledata/" + name
            raw, proof = patch(apk.read(entry), key, identity, column, args.description)
            file = out / name
            file.write_bytes(raw)
            payload[entry] = str(file.relative_to(ROOT))
            proofs[entry] = proof
        managed = apk.read(MANAGED)
        pe = dnfile.dnPE(data=managed)
        engine = sum(bool(row.Rva) for name, _, row in methods(pe) if name.startswith("Battle.Engine"))
        if engine != 547:
            raise ValueError("Original engine inventory differs")
        preserved = {}
        for name in ("tableitemuse", "tablehero"):
            entry = "assets/android/tabledata/" + name
            preserved[entry] = sha(apk.read(entry))
    report = {"schema": "hyjs-os-selector-description78-v1", "status": "STATIC_VERIFIED_NOT_PACKAGED",
              "baseAPKSHA256": BASE_SHA256, "descriptionBefore": OLD_DESCRIPTION,
              "descriptionAfter": args.description, "originalOptionNames": dict(zip(expected_rewards, names)),
              "serverDescriptionsAndOptionsMatch": True, "proofs": proofs,
              "unchangedOriginalEntries": preserved, "inputManagedSHA256": sha(managed),
              "originalEngineMethodsExact": engine, "originalMetadataABIAndTokensPreserved": True,
              "payloadFiles": payload, "replacementEntries": {k: sha((ROOT / v).read_bytes()) for k, v in payload.items()},
              "sourcePins": {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in (Path(__file__).resolve(), SOURCE)},
              "deviceTested": False}
    (out / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": report["status"], "replacementEntries": report["replacementEntries"]}, ensure_ascii=False))


if __name__ == "__main__":
    main()
