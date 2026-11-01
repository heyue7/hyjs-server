"""Prepare the reviewed 1.1.6 managed overlays for the scoped APK packager."""

import argparse
import hashlib
import json
import shutil
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
APK_SHA = "bd3e4caefe47545de7b2272681a363f79df5d75838b96bc752e882cdbb07137e"
MAIN_SHA = "3d8f50e543ad082ea03bff294f17421a2eed4316d5cda664b03cc0f46cc23e8a"
HELPER_SHA = "17672c2a1e09334a82c37168586720241faec0b26d3bf83e31bc71090740da0b"
MAIN_ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
HELPER_ENTRY = "assets/bin/Data/Managed/SoulFlowerActivity.dll"
METHOD = "<CopyToBattleScene>c__AnonStorey163::<>m__37"


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def require(condition, message):
    if not condition:
        raise SystemExit(message)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", required=True, type=Path)
    parser.add_argument("--copy-overlay", required=True, type=Path)
    parser.add_argument("--activity-overlay", required=True, type=Path)
    parser.add_argument("--output-directory", required=True, type=Path)
    args = parser.parse_args()
    base = args.base_apk.resolve()
    target = args.output_directory.resolve()
    require(target.is_relative_to(ROOT) and not target.exists(), "Use a fresh output inside this worktree")
    require(digest(base) == APK_SHA, "1.1.5 base APK differs")
    copy_dir, activity_dir = args.copy_overlay.resolve(), args.activity_overlay.resolve()
    copy_proof = json.loads((copy_dir / "manifest.json").read_text())
    activity_proof = json.loads((activity_dir / "manifest.json").read_text())
    require(copy_proof.get("status") == "PASS_OFFLINE_NOT_PACKAGED", "Copy patch is not qualified")
    require(activity_proof.get("status") == "STATIC_VERIFIED_NOT_PACKAGED", "Activity helper is not qualified")
    require(copy_proof.get("baseAPKSHA256") == activity_proof.get("baseAPKSHA256") == APK_SHA, "Overlay base mismatch")
    require(copy_proof.get("inputManagedSHA256") == activity_proof.get("mainAssemblySHA256") == MAIN_SHA, "Main assembly base mismatch")
    require(activity_proof.get("shippedHelperSHA256") == activity_proof.get("reconstructedHelperSHA256") == HELPER_SHA, "Shipped helper was not exactly reconstructed")
    checks = copy_proof.get("checks", {})
    require(checks.get("changedMethod") == METHOD and checks.get("unchangedBattleEngineMethods") == 547 and checks.get("otherMethodHeadersILAndExceptionRegionsByteIdentical"), "Copy patch method checks differ")
    main = copy_dir / "Assembly-CSharp.dll"
    helper = activity_dir / "SoulFlowerActivity.dll"
    main_sha = digest(main)
    helper_sha = digest(helper)
    require(main_sha == copy_proof.get("outputManagedSHA256"), "Copy output hash differs")
    require(helper_sha == activity_proof.get("candidateHelperSHA256"), "Helper output hash differs")
    pins = {}
    for manifest, field in ((copy_proof, "sourcePins"), (activity_proof, "sourceSHA256")):
        for name, expected in manifest.get(field, {}).items():
            path = (ROOT / name).resolve()
            require(path.is_relative_to(ROOT) and path.is_file() and digest(path) == expected, "Source changed: " + name)
            require(name not in pins or pins[name] == expected, "Conflicting source pin: " + name)
            pins[name] = expected
    require(pins, "Missing source pins")
    pins[str(Path(__file__).resolve().relative_to(ROOT))] = digest(Path(__file__).resolve())
    with zipfile.ZipFile(base) as archive:
        require(hashlib.sha256(archive.read(MAIN_ENTRY)).hexdigest() == MAIN_SHA, "Base main assembly differs")
        require(hashlib.sha256(archive.read(HELPER_ENTRY)).hexdigest() == HELPER_SHA, "Base helper differs")
        dex_sha = hashlib.sha256(archive.read("classes2.dex")).hexdigest()
    target.mkdir(parents=True)
    shutil.copyfile(main, target / main.name)
    shutil.copyfile(helper, target / helper.name)
    shutil.copyfile(copy_dir / "manifest.json", target / "copy-proof.json")
    shutil.copyfile(activity_dir / "manifest.json", target / "activity-proof.json")
    for name in ("copy-proof.json", "activity-proof.json"):
        path = target / name
        pins[str(path.relative_to(ROOT))] = digest(path)
    payload = {MAIN_ENTRY: str((target / main.name).relative_to(ROOT)), HELPER_ENTRY: str((target / helper.name).relative_to(ROOT))}
    proof = {
        "schema": "hyjs-copy-recharge-116-managed-v1",
        "baseAPKSHA256": APK_SHA,
        "inputManagedSHA256": MAIN_SHA,
        "inputDexSHA256": dex_sha,
        "managedOnly": True,
        "inheritedJavaPublicProtectedABIExact": True,
        "inheritedJavaByteExact": True,
        "allowedChangedMethods": [METHOD],
        "payloadFiles": payload,
        "replacementEntries": {MAIN_ENTRY: main_sha, HELPER_ENTRY: helper_sha},
        "sourcePins": pins,
        "copyProofSHA256": pins[str((target / "copy-proof.json").relative_to(ROOT))],
        "activityProofSHA256": pins[str((target / "activity-proof.json").relative_to(ROOT))],
        "deviceVerified": False,
    }
    (target / "manifest.json").write_text(json.dumps(proof, ensure_ascii=False, indent=2) + "\n")
    print("Prepared 1.1.6 copy and recharge overlays; device validation is pending")


if __name__ == "__main__":
    main()
