"""Recover the one byte-identical original map bundle from the pinned RE archive."""
import argparse, copy, hashlib, json, zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
ENTRY = "assets/android/map/map_nightforestfield_001"
DONOR_SHA = "2d32f6babaae52113ffe144fa79fa139c5244ebb4b80bc1d07376c2e7a23b257"
BUNDLE_SHA = "60eafad353de37c0e2a13c2b90b35079d2a48a3b185404d327a63ce2a36a776c"

def verify(base, donor):
    import UnityPy
    assert hashlib.file_digest(donor.open("rb"), "sha256").hexdigest() == DONOR_SHA, "Donor differs"
    with zipfile.ZipFile(base) as original, zipfile.ZipFile(donor) as source:
        assert ENTRY not in original.namelist(), "Supplement is already present"
        payload = source.read(ENTRY)
        assert len(payload) == 13943479 and hashlib.sha256(payload).hexdigest() == BUNDLE_SHA
        tree = next(o.read_typetree() for o in UnityPy.load(original.read("assets/android/android")).objects if o.type.name == "AssetBundleManifest")
        names, infos = dict(tree["AssetBundleNames"]), dict(tree["AssetBundleInfos"])
        root = next(key for key, value in names.items() if value == ENTRY[len("assets/android/"):])
        todo, seen = [root], set()
        while todo:
            key = todo.pop()
            if key in seen: continue
            seen.add(key); todo.extend(infos[key]["AssetBundleDependencies"])
        dependencies = sorted("assets/android/" + names[key] for key in seen)
        # This fixed donor bundle contains its scene, meshes and materials in two serialized files.
        # The separately recorded all-object audit found only Unity builtin external references.
        # Old graph declarations refer to 20 unavailable bundles that this exact bundle never references.
        old_tree = copy.deepcopy(tree)
        infos[root]["AssetBundleDependencies"] = []
        environment = UnityPy.load(original.read("assets/android/android"))
        obj = next(o for o in environment.objects if o.type.name == "AssetBundleManifest")
        obj.save_typetree(tree)
        manifest = environment.file.save()
        checked = next(o.read_typetree() for o in UnityPy.load(manifest).objects if o.type.name == "AssetBundleManifest")
        expected = copy.deepcopy(old_tree)
        dict(expected["AssetBundleInfos"])[root]["AssetBundleDependencies"] = []
        assert checked == expected, "Unrelated manifest content changed"
        return {ENTRY:payload,"assets/android/android":manifest}, dependencies

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--base-apk", type=Path, required=True)
    p.add_argument("--donor-apk", type=Path, required=True)
    p.add_argument("--output-directory", type=Path, required=True)
    args = p.parse_args(); out = args.output_directory.resolve()
    assert out.is_relative_to(ROOT) and not out.exists()
    payloads, dependencies = verify(args.base_apk, args.donor_apk)
    files, hashes = {}, {}
    for name, data in payloads.items():
        target = out / name; target.parent.mkdir(parents=True,exist_ok=True); target.write_bytes(data)
        files[name] = str(target.relative_to(ROOT)); hashes[name] = hashlib.sha256(data).hexdigest()
    report = dict(baseAPKSHA256=hashlib.file_digest(args.base_apk.open("rb"), "sha256").hexdigest(), donorAPK=str(args.donor_apk.resolve()), donorAPKSHA256=DONOR_SHA, payloadFiles=files, replacementEntries=hashes, previousDeclaredDependencies=dependencies, onlyRootDependencyDeclarationChanged=True, originalMapByteIdentical=True, actualSerializedClosure="two local serialized files and Unity builtins; fixed SHA all-object audit")
    (out / "manifest.json").write_text(json.dumps(report, indent=2)+"\n")
    print(json.dumps(dict(bundleBytes=len(payloads[ENTRY]), previousDeclaredClosureBundles=len(dependencies), sha256=BUNDLE_SHA)))

if __name__ == "__main__": main()
