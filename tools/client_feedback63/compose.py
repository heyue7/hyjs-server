"""Compose batch63 scoped managed payloads against the released 62 APK."""
import argparse
import hashlib
import json
from pathlib import Path
import sys
import zipfile

import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_one_key59.build import verify
from tools.client_feedback58.compose import qa_main
from tools.client_arena_state.build import methods, exact_body

MAIN = "assets/bin/Data/Managed/Assembly-CSharp.dll"
BASE_SHA = "fe427f13e8683b434bf7d2756b2dbd4a6f42746d0d6c66c86264182252704580"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def owned(path):
    path = path.resolve()
    assert path.is_relative_to(ROOT), "Payload must belong to this checkout"
    return path


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--base-apk", type=Path, required=True)
    p.add_argument("--field-overlay", type=Path, required=True)
    p.add_argument("--dungeon-overlay", type=Path, required=True)
    p.add_argument("--commerce-manifest", type=Path, action="append", required=True)
    p.add_argument("--output-directory", type=Path, required=True)
    p.add_argument("--qa-origin")
    a = p.parse_args()
    assert hashlib.file_digest(a.base_apk.open("rb"), "sha256").hexdigest() == BASE_SHA
    out = owned(a.output_directory)
    assert not out.exists(), "Fresh output required"
    out.mkdir(parents=True)
    with zipfile.ZipFile(a.base_apk) as z:
        original = z.read(MAIN)
        dex_sha = sha(z.read("classes2.dex"))
    current = original
    pins, payload, chain, targets = {}, {}, [], []
    for directory, target_file in [(a.field_overlay, "tools/client_field63/targets.json"),
                                   (a.dungeon_overlay, "tools/client_dungeon63/targets.json")]:
        directory = owned(directory)
        manifest = directory / "manifest.json"
        proof = json.loads(manifest.read_text())
        assert proof["baseAPKSHA256"] == BASE_SHA and proof["inputManagedSHA256"] == sha(current)
        for entry, file in proof["payloadFiles"].items():
            file = owned(ROOT / file)
            assert sha(file.read_bytes()) == proof["replacementEntries"][entry]
            payload[entry] = file
        current = payload[MAIN].read_bytes()
        pins.update(proof["sourcePins"])
        targets.extend(json.loads((ROOT / target_file).read_text()))
        chain.append({"manifest": str(manifest.relative_to(ROOT)), "sha256": sha(manifest.read_bytes())})
    # Later manifests may replace one helper; source pins follow the selected helper.
    helpers = {}
    for manifest in a.commerce_manifest:
        manifest = owned(manifest)
        proof = json.loads(manifest.read_text())
        assert proof["inputAPKSHA256"] == BASE_SHA and proof["inputManagedSHA256"] == sha(original)
        helpers.update(proof["helpers"])
        chain.append({"manifest": str(manifest.relative_to(ROOT)), "sha256": sha(manifest.read_bytes())})
    assert set(helpers) == {"assets/bin/Data/Managed/" + n + ".dll" for n in
                            ["SoulFlowerActivity", "SoulFlowerReWelfare", "SoulFlowerGGOShop"]}
    for entry, item in helpers.items():
        file = owned(Path(item["path"]))
        assert sha(file.read_bytes()) == item["sha256"]
        payload[entry] = file
        pins.update(item["sourcePins"])
    combined_proof = verify(original, current, targets)
    qa_proof = None
    if a.qa_origin:
        assert a.qa_origin == "http://127.0.0.1:28979"
        before = current
        current, qa_proof = qa_main(current, a.base_apk, a.qa_origin, out)
        left, right = dnfile.dnPE(data=before), dnfile.dnPE(data=current)
        extra = [{"token": hex(0x6000000 + rid)} for (name, rid, old), (_, _, new) in
                 zip(methods(left), methods(right)) if old.Rva and exact_body(left, old) != exact_body(right, new)]
        assert len(extra) == 4
        combined_proof = verify(original, current, targets + extra)
        for file in ["tools/client_remote/build.py", "tools/client_remote/Injector.cs", "tools/client_feedback58/compose.py"]:
            pins[file] = sha((ROOT / file).read_bytes())
    pe = dnfile.dnPE(data=current)
    host = {(str(t.TypeNamespace), str(t.TypeName)) for t in pe.net.mdtables.TypeDef.rows}
    for t in pe.net.mdtables.TypeRef.rows:
        scope = t.ResolutionScope
        assert not (scope.table.name == "ModuleRef" and
                    ((str(t.TypeNamespace), str(t.TypeName)) in host or
                     str(scope.row.Name).startswith(("HYJS.", "SoulFlower")))), "Foreign helper module scope"
    selected = {int(t, 16) for t in combined_proof["changedMethodTokens"]}
    names = sorted({name for name, rid, _ in methods(pe) if (0x6000000 + rid) in selected})
    pins[str(Path(__file__).relative_to(ROOT))] = sha(Path(__file__).read_bytes())
    for file, digest in pins.items():
        assert sha(owned(ROOT / file).read_bytes()) == digest, "Source changed: " + file
    (out / "Assembly-CSharp.dll").write_bytes(current)
    payload[MAIN] = out / "Assembly-CSharp.dll"
    files = {entry: str(file.relative_to(ROOT)) for entry, file in payload.items()}
    report = dict(schema="hyjs-feedback63-managed-v1", baseAPKSHA256=BASE_SHA,
                  inputManagedSHA256=sha(original), outputManagedSHA256=sha(current),
                  inputDexSHA256=dex_sha, managedOnly=True, inheritedJavaByteExact=True,
                  inheritedJavaPublicProtectedABIExact=True, payloadFiles=files,
                  replacementEntries={entry: sha(file.read_bytes()) for entry, file in payload.items()},
                  sourcePins=pins, allowedChangedMethods=names, **combined_proof,
                  managedChain=chain, qaOnly=bool(a.qa_origin), neverPromote=bool(a.qa_origin),
                  qaOrigin=a.qa_origin, qaOriginProof=qa_proof, deviceTested=False)
    (out / "manifest.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({key: report[key] for key in ["outputManagedSHA256", "changedMethodTokens", "originalEngineMethods", "nonTargetRawCILExact", "qaOnly"]}))


if __name__ == "__main__":
    main()
