"""Prepare the one-cell breach text change and merge it into an APK overlay."""

import argparse
import hashlib
import json
import zipfile
from pathlib import Path

import UnityPy

ROOT = Path(__file__).resolve().parents[2]
ENTRY = "assets/android/tabledata/tabletext"
TARGET = "break_through_desc"
BEFORE = "[000000]突破值每天24:00清零，当前有{0}直接升级[-]"
AFTER = "[000000]当前有{0}直接升级[-]"
SCHEMA = "hyjs-breach84-tabletext-overlay-v1"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def digest(path):
    with Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def asset(raw):
    env = UnityPy.load(raw)
    objects = list(env.objects)
    texts = [obj for obj in objects if obj.type.name == "TextAsset"]
    if len(texts) != 1:
        raise ValueError("expected exactly one table TextAsset")
    return env, objects, texts[0]


def script(obj):
    data = obj.read_typetree()["m_Script"]
    return data.decode("utf-8") if isinstance(data, bytes) else data


def change_cell(original):
    lines = original.splitlines(keepends=True)
    if len(lines) < 6:
        raise ValueError("table headers missing")
    columns = lines[1].rstrip("\r\n").split("\t")
    identity, value = columns.index("id"), columns.index("text")
    matches = []
    for i in range(5, len(lines)):
        body = lines[i].rstrip("\r\n")
        cells = body.split("\t")
        if len(cells) == len(columns) and cells[identity] == TARGET:
            matches.append((i, cells, lines[i][len(body):]))
    if len(matches) != 1:
        raise ValueError("expected exactly one breach description")
    index, cells, ending = matches[0]
    if cells[value] != BEFORE:
        raise ValueError("breach description does not match the audited input")
    cells[value] = AFTER
    lines[index] = "\t".join(cells) + ending
    return "".join(lines)


def patch(raw):
    env, objects, text = asset(raw)
    original = script(text)
    updated = change_cell(original)
    before = {obj.path_id: (obj.type.name, sha(obj.get_raw_data())) for obj in objects}
    old_tree = text.read_typetree()
    tree = text.read_typetree()
    tree["m_Script"] = updated
    text.save_typetree(tree)
    output = env.file.save(packer="lz4")
    _, after_objects, after_text = asset(output)
    after = {obj.path_id: (obj.type.name, sha(obj.get_raw_data())) for obj in after_objects}
    if set(before) != set(after) or any(before[i] != after[i] for i in before if i != text.path_id):
        raise ValueError("other asset objects changed")
    new_tree = after_text.read_typetree()
    old_tree.pop("m_Script")
    new_tree.pop("m_Script")
    if old_tree != new_tree or script(after_text) != updated:
        raise ValueError("text metadata or table script changed unexpectedly")
    return output, {
        "inputSHA256": sha(raw),
        "outputSHA256": sha(output),
        "originalScriptSHA256": sha(original.encode("utf-8")),
        "updatedScriptSHA256": sha(updated.encode("utf-8")),
        "targetRow": TARGET,
        "targetColumn": "text",
        "otherRowsAndCellsExact": True,
        "otherAssetObjectsExact": True,
    }


def prepare(base, expected_sha, out):
    if len(expected_sha) != 64 or expected_sha.lower() != digest(base):
        raise ValueError("base APK SHA-256 mismatch")
    if not out.is_relative_to(ROOT / "build") or out.exists():
        raise ValueError("output must be a fresh directory inside this checkout's build")
    with zipfile.ZipFile(base) as apk:
        raw, proof = patch(apk.read(ENTRY))
    out.mkdir(parents=True)
    resource = out / "tabletext"
    resource.write_bytes(raw)
    manifest = {
        "schema": SCHEMA,
        "status": "STATIC_VERIFIED_NOT_PACKAGED",
        "baseAPKSHA256": expected_sha.lower(),
        "before": BEFORE,
        "after": AFTER,
        "proofs": {ENTRY: proof},
        "payloadFiles": {ENTRY: str(resource.relative_to(ROOT))},
        "replacementEntries": {ENTRY: sha(raw)},
        "sourcePins": {str(Path(__file__).resolve().relative_to(ROOT)): digest(__file__)},
        "deviceTested": False,
    }
    (out / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": manifest["status"], "proof": proof}, ensure_ascii=False))


def apply_overlay(base, out, payload, pins):
    """Apply the cell to any earlier tabletext payload instead of replacing it."""
    report = json.loads((out / "manifest.json").read_text())
    if report["schema"] != SCHEMA or digest(base) != report["baseAPKSHA256"]:
        raise ValueError("overlay manifest or base APK differs")
    for source, expected in report["sourcePins"].items():
        file = ROOT / source
        if digest(file) != expected:
            raise ValueError("overlay builder changed after preparation")
        pins[file] = expected
    with zipfile.ZipFile(base) as apk:
        reference, proof = patch(apk.read(ENTRY))
        if proof != report["proofs"][ENTRY] or reference != (out / "tabletext").read_bytes():
            raise ValueError("prepared reference overlay changed")
        merged_input = payload.get(ENTRY, apk.read(ENTRY))
        merged, merged_proof = patch(merged_input)
    payload[ENTRY] = merged
    pins[out / "tabletext"] = digest(out / "tabletext")
    pins[out / "manifest.json"] = digest(out / "manifest.json")
    return {"mergedInputSHA256": sha(merged_input), "proof": merged_proof}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", required=True, type=Path)
    parser.add_argument("--base-sha256", required=True)
    parser.add_argument("--output-directory", required=True, type=Path)
    args = parser.parse_args()
    prepare(args.base_apk.resolve(), args.base_sha256, args.output_directory.resolve())
