"""Build a pinned, resource-only candidate for original 3D player-name fonts."""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import zipfile

import UnityPy

ROOT = Path(__file__).resolve().parents[2]
APK_SHA = "d60dd8b6a9de9b11d8a673d240f19f3cc16d33956cdede386794cc26e2f71f8d"
ENTRY = "assets/bin/Data/c09f4969afcb1ed43858888756b9f3fe"
ENTRY_SHA = "18a791169a577c0e48bbcd6d24c3ccd40c1967743821b6639986479afe0c09eb"
TEXT = (29, 30, 31, 32, 33, 34)
RENDERERS = (20, 21, 22, 24, 25, 26)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def objects(raw):
    env = UnityPy.load(raw)
    return env, {obj.path_id: obj for obj in env.objects}


def build_overlay(base_apk):
    base_apk = Path(base_apk)
    require(sha(base_apk.read_bytes()) == APK_SHA, "Base APK SHA drift")
    with zipfile.ZipFile(base_apk) as archive:
        raw = archive.read(ENTRY)
        require(sha(raw) == ENTRY_SHA, "Prefab entry SHA drift")
        parts = sorted((n for n in archive.namelist() if n.startswith("assets/bin/Data/sharedassets0.assets.split")),
                       key=lambda n: int(n.rsplit("split", 1)[1]))
        shared = b"".join(archive.read(n) for n in parts)
        _, shared_objects = objects(shared)
        font = shared_objects[191].read_typetree()
        material = shared_objects[80].read_typetree()
        texture = shared_objects[147].read_typetree()
        require(shared_objects[191].type.name == "Font" and font["m_Name"] == "Words", "Words font differs")
        require(font["m_DefaultMaterial"] == {"m_FileID": 0, "m_PathID": 80}, "Font material differs")
        require(font["m_Texture"] == {"m_FileID": 0, "m_PathID": 147}, "Font texture differs")
        require(shared_objects[80].type.name == "Material", "Material target differs")
        require(material["m_SavedProperties"]["m_TexEnvs"][0][1]["m_Texture"] == font["m_Texture"], "Material atlas differs")
        require(shared_objects[147].type.name == "Texture2D", "Atlas target differs")
        managed = {n: sha(archive.read(n)) for n in archive.namelist() if n.endswith(".dll") or n.endswith(".dex")}
    env, before = objects(raw)
    source_pins = {str(pid): dict(type=obj.type.name, rawSHA256=sha(obj.get_raw_data())) for pid, obj in before.items()}
    original_trees = {pid: before[pid].read_typetree() for pid in TEXT + RENDERERS}
    original_raw = {pid: obj.get_raw_data() for pid, obj in before.items()}
    assets = before[29].assets_file
    old_externals = [(x.path, x.guid.hex(), x.type, x.temp_empty) for x in assets.externals]
    require("sharedassets0.assets" not in [x.path for x in assets.externals], "Unexpected existing shared reference")
    external = copy.copy(assets.externals[0])
    external.path = "sharedassets0.assets"
    external.guid = bytes(16)
    assets.externals.append(external)
    file_id = len(assets.externals)
    changes = []
    for pid in TEXT + RENDERERS:
        tree = copy.deepcopy(original_trees[pid])
        require(before[pid].save_typetree(original_trees[pid]) == original_raw[pid], "Original typetree roundtrip differs")
        if pid in TEXT:
            require(tree["m_Font"] == {"m_FileID": 0, "m_PathID": 0}, "Original TextMesh font differs")
            tree["m_Font"] = {"m_FileID": file_id, "m_PathID": 191}
            field = "m_Font"
        else:
            require(tree["m_Materials"] == [{"m_FileID": 3, "m_PathID": 10100}], "Original renderer material differs")
            tree["m_Materials"] = [{"m_FileID": file_id, "m_PathID": 80}]
            field = "m_Materials"
        patched = before[pid].save_typetree(tree)
        require(len(patched) == len(original_raw[pid]), "Object size changed")
        changes.append(dict(pathId=pid, field=field, before=original_trees[pid][field], after=tree[field],
                            changedObjectByteOffsets=[i for i, pair in enumerate(zip(original_raw[pid], patched)) if pair[0] != pair[1]]))
    output = assets.save()
    _, after = objects(output)
    require(set(before) == set(after), "Object inventory changed")
    for pid, obj in after.items():
        require(obj.type.name == before[pid].type.name, "Object type changed")
        if pid not in TEXT + RENDERERS:
            require(obj.get_raw_data() == original_raw[pid], "Non-target object changed: " + str(pid))
        else:
            tree = obj.read_typetree()
            field = "m_Font" if pid in TEXT else "m_Materials"
            expected = {"m_FileID": file_id, "m_PathID": 191} if pid in TEXT else [{"m_FileID": file_id, "m_PathID": 80}]
            require(tree[field] == expected, "Reloaded target reference differs")
            require(all(tree[k] == v for k, v in original_trees[pid].items() if k != field), "Non-target field changed")
    actual_ext = after[29].assets_file.externals
    require([(x.path, x.guid.hex(), x.type, x.temp_empty) for x in actual_ext[:-1]] == old_externals, "Existing external mapping changed")
    require(actual_ext[-1].path == "sharedassets0.assets", "New external mapping differs")
    proof = dict(schema="hyjs-head-name-fonts-v1", status="STATIC_VERIFIED_CANDIDATE_NOT_DEVICE_VERIFIED",
                 baseAPKSHA256=APK_SHA, targetEntry=ENTRY, inputEntrySHA256=ENTRY_SHA, outputEntrySHA256=sha(output),
                 sourcePins=source_pins, sourceSHA256={"tools/client_head_names/build.py": sha(Path(__file__).read_bytes())},
                 unityPyVersion=UnityPy.__version__, changes=changes, appendedExternal=dict(fileId=file_id, path=external.path),
                 sharedAssetsSHA256=sha(shared), fontDependencies={str(pid): dict(type=shared_objects[pid].type.name,
                 rawSHA256=sha(shared_objects[pid].get_raw_data())) for pid in (191, 80, 147)},
                 dynamicAtlas=dict(width=texture["m_Width"], height=texture["m_Height"], runtimePopulationRequired=True),
                 allNonTargetObjectsRawEqual=True, nonTargetObjectCount=len(before)-12,
                 allOtherTargetFieldsEqual=True, existingExternalMappingsPreserved=True,
                 originalManagedAndJavaSHA256=managed, replacementEntries=[ENTRY], modifiedManagedAssemblies=[],
                 modifiedJava=[], unchangedOtherAPKEntriesByConstruction=True, deviceExecuted=False, visualVerified=False)
    return {ENTRY: output}, proof


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, default=ROOT / "build/hyjs-local.apk")
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    require(not args.output_directory.exists(), "Fresh output directory required")
    entries, proof = build_overlay(args.base_apk)
    args.output_directory.mkdir(parents=True)
    for name, raw in entries.items():
        destination = args.output_directory / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(raw)
    (args.output_directory / "manifest.json").write_text(json.dumps(proof, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(dict(status=proof["status"], outputDirectory=str(args.output_directory), replacementEntries=list(entries))))


if __name__ == "__main__":
    main()
