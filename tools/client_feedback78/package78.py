"""Compose the pinned 77 candidate with the two reviewed feedback78 overlays."""
import argparse
import hashlib
import io
import json
import subprocess
import sys
import zipfile
from pathlib import Path

import dnfile
from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.package_release import require
from tools.client_feedback78 import build_os_selector_text as os_text
from tools.client_sprout73 import build_refresh as sprout
from tools.client_compat63.dex_patch import extract_pins
from tools.client_branding.branding import chunks, elements, strings_from_pool

BASE = ROOT / "build/activity-growth77/hyjs-1.0.77-candidate-r8.apk"
SPROUT = ROOT / "build/feedback78/sprout"
OS = ROOT / "build/feedback78/os-selector"
MANAGED_PROOF = ROOT / "build/feedback78/package-input-r1"
OUTPUT = ROOT / "build/feedback78/hyjs-1.0.78-candidate-r1.apk"
RECORD = ROOT / "build/feedback78/package-r1.json"
SIGNING = Path("/Users/lirui/LR/hyjs/.local-signing")
BASE_SHA = "c970012d7c785c58c3b923ea4a0ea8e650204ed0e50e16e914f47e54ac4aece8"
HELPER_ENTRY = "assets/bin/Data/Managed/HYJS.Sprout73.dll"
HELPER_SHA = "07fcf041b77c41836ef0f10e17a0a876dc427784deb34cdad50725270b803208"
DESCRIPTION = "可选择一名OS角色：OS桐人、OS亚丝娜、尤娜。"
EXPECTED_RESOURCE_HASHES = {
    "assets/android/tabledata/tableitem": "752c940c5556ff00a9c165d12e9c80b542cd385576d7bf052df1709a9ae352c7",
    "assets/android/tabledata/tabletext": "f8a1d6693d4a6e2a70bee1bc998f655616e57161265879d10fea48a7b9a7a5a4",
}
EXPECTED_WHITELIST = {
    "tabledata/tablehero", "tabledata/tablefrag",
    "model/lingnai01_body", "model/sicary01_body", "model/nio01_body",
    "smallassetmanifest",
}


def pin_sources(manifest, expected_names, pins):
    source_pins = manifest.get("sourcePins", {})
    require(set(source_pins) == set(expected_names), "Unexpected source pin inventory")
    for name, digest in source_pins.items():
        path = (ROOT / name).resolve()
        require(path.is_relative_to(ROOT) and common.digest(path) == digest, "Source pin differs: " + name)
        require(path not in pins or pins[path] == digest, "Conflicting source pin: " + name)
        pins[path] = digest


def whitelist(archive):
    config = json.loads(archive.read("assets/local-resource-overrides.json"))
    pins = extract_pins(archive.read("classes2.dex"))
    require(config.get("files") == pins and len(pins) == 6, "DEX and configuration whitelist differ")
    require({pin["path"] for pin in pins} == EXPECTED_WHITELIST, "Unexpected resource whitelist")
    for pin in pins:
        data = archive.read("assets/android/" + pin["path"])
        require(len(data) == pin["bytes"] and os_text.sha(data) == pin["sha256"],
                "Whitelisted resource differs: " + pin["path"])
    require(not ({n.removeprefix("assets/android/") for n in EXPECTED_RESOURCE_HASHES}
                 & EXPECTED_WHITELIST), "Description tables are whitelisted")
    return pins


def apply_os_selector_overlay(base, directory, payload, pins):
    """Replay exactly the qualified item_9760 table edits; no generic resources."""
    directory = directory.resolve()
    require(directory == OS.resolve(), "Unexpected OS overlay directory")
    manifest_path = directory / "manifest.json"
    report = json.loads(manifest_path.read_text())
    require(report.get("schema") == "hyjs-os-selector-description78-v1"
            and report.get("status") == "STATIC_VERIFIED_NOT_PACKAGED"
            and report.get("baseAPKSHA256") == BASE_SHA == common.digest(base),
            "Unqualified OS selector overlay")
    require(report.get("descriptionBefore") == os_text.OLD_DESCRIPTION
            and report.get("descriptionAfter") == DESCRIPTION, "Description differs")
    require(report.get("replacementEntries") == EXPECTED_RESOURCE_HASHES
            and set(report.get("payloadFiles", {})) == set(EXPECTED_RESOURCE_HASHES)
            and set(report.get("proofs", {})) == set(EXPECTED_RESOURCE_HASHES),
            "OS overlay entry scope differs")
    pin_sources(report, {
        "tools/client_feedback78/build_os_selector_text.py",
        "hyjs-server/service/data/growth74_resources.json",
    }, pins)
    pins[manifest_path] = common.digest(manifest_path)
    pins[Path(__file__).resolve()] = common.digest(Path(__file__))
    specs = [
        ("tableitem", "itemId", "item_9760", "intro"),
        ("tabletext", "id", "desc_item_9760", "text"),
    ]
    with zipfile.ZipFile(base) as archive:
        whitelist_pins = whitelist(archive)
        require(os_text.sha(archive.read(os_text.MANAGED)) == report["inputManagedSHA256"],
                "OS overlay managed base differs")
        uses = [r for r in os_text.rows(archive.read("assets/android/tabledata/tableitemuse"))
                if r["itemId"] == "item_9760"]
        rewards = ["hero_077,1", "hero_078,1", "hero_079,1"]
        require([r["result"] for r in uses] == rewards, "Original selector options differ")
        heroes = {r["id"]: r for r in os_text.rows(archive.read("assets/android/tabledata/tablehero"))}
        texts = {r["id"]: r["text"] for r in os_text.rows(archive.read("assets/android/tabledata/tabletext"))}
        names = [texts[heroes[reward.split(",")[0]]["name"]] for reward in rewards]
        require(names == ["OS桐人", "OS亚丝娜", "尤娜"]
                and report.get("originalOptionNames") == dict(zip(rewards, names))
                and DESCRIPTION == "可选择一名OS角色：" + "、".join(names) + "。",
                "Actual selector names differ")
        source = json.loads(os_text.SOURCE.read_text())
        items = [r for r in source["items"] if r["itemId"] == "item_9760"]
        source_uses = [r for r in source["uses"] if r["itemId"] == "item_9760"]
        fields = ("id", "itemId", "result", "odds", "category")
        require(len(items) == 1 and items[0]["desc"] == items[0]["intro"] == DESCRIPTION
                and [{k: r[k] for k in fields} for r in source_uses]
                == [{k: r[k] for k in fields} for r in uses]
                and report.get("serverDescriptionsAndOptionsMatch") is True,
                "Server item descriptions or selector options differ")
        for name, key, identity, column in specs:
            entry = "assets/android/tabledata/" + name
            expected, proof = os_text.patch(archive.read(entry), key, identity, column, DESCRIPTION)
            file = (ROOT / report["payloadFiles"][entry]).resolve()
            require(file == (directory / name).resolve() and file.read_bytes() == expected
                    and os_text.sha(expected) == EXPECTED_RESOURCE_HASHES[entry]
                    and report["proofs"][entry] == proof, "Description replay differs: " + entry)
            require(entry not in payload, "Overlapping description overlay")
            payload[entry] = expected
            pins[file] = common.digest(file)
        original = {entry: os_text.sha(archive.read(entry)) for entry in (
            "assets/android/tabledata/tableitemuse", "assets/android/tabledata/tablehero")}
        require(original == report["unchangedOriginalEntries"], "Unchanged selector assets differ")
    return {"schema": report["schema"], "manifestSHA256": common.digest(manifest_path),
            "sourcePinsVerified": True, "descriptionBefore": report["descriptionBefore"],
            "descriptionAfter": DESCRIPTION, "originalOptionNames": report["originalOptionNames"],
            "replayedCellProofs": report["proofs"], "whitelistPins": whitelist_pins,
            "descriptionTablesInWhitelist": False}


def prepare_managed_proof():
    require(common.digest(BASE) == BASE_SHA, "Base APK differs")
    manifest_path = SPROUT / "manifest.json"
    report = json.loads(manifest_path.read_text())
    require(report.get("schema") == "hyjs-sprout78-helper-refresh-v1"
            and report.get("status") == "STATIC_VERIFIED_NOT_PACKAGED"
            and report.get("baseAPKSHA256") == BASE_SHA, "Unqualified sprout overlay")
    require(report.get("payloadFiles") == {HELPER_ENTRY: "build/feedback78/sprout/HYJS.Sprout73.dll"}
            and report.get("replacementEntries") == {HELPER_ENTRY: HELPER_SHA}, "Helper payload scope differs")
    pins = {}
    pin_sources(report, {"tools/client_sprout73/build_refresh.py",
                         "tools/client_sprout73/Runtime.cs",
                         "tools/ggo_items/assets/ggo_sprout.png"}, pins)
    helper_path = SPROUT / "HYJS.Sprout73.dll"
    require(common.digest(helper_path) == HELPER_SHA, "Helper payload differs")
    helper = helper_path.read_bytes()
    with zipfile.ZipFile(BASE) as archive:
        require(len(archive.namelist()) == len(set(archive.namelist())), "Duplicate base ZIP entry")
        main = archive.read(common.DLL)
        previous = archive.read(HELPER_ENTRY)
        require(os_text.sha(main) == report["inputManagedSHA256"]
                and os_text.sha(previous) == report["previousHelperSHA256"], "Sprout input differs")
        main_pe = dnfile.dnPE(data=main)
        hooks = sprout.check_hooks(main_pe)
        engine = sum(bool(m.Rva) for n, _, m in sprout.methods(main_pe) if n.startswith("Battle.Engine"))
        require(hooks == report["existingHooksVerified"] and engine == report["originalEngineMethodsExact"] == 547,
                "Existing sprout hooks or engine inventory differ")
        old_pe, new_pe = (dnfile.dnPE(data=data) for data in (previous, helper))
        require(sprout.assembly_identity(old_pe) == sprout.assembly_identity(new_pe)
                and sprout.public_api(old_pe) == sprout.public_api(new_pe)
                and report.get("helperPublicAPIExact") is True, "Helper public ABI differs")
        resources = [(str(item.name), item.data) for item in new_pe.net.resources]
        require(len(resources) == 1 and resources[0][0] == "ggo_sprout.png", "Embedded icon resource differs")
        expected_icon = Image.open(sprout.IMAGE).convert("RGBA")
        expected_icon = expected_icon.crop(expected_icon.getchannel("A").getbbox())
        actual_icon = Image.open(io.BytesIO(resources[0][1])).convert("RGBA")
        require(actual_icon.size == expected_icon.size and actual_icon.tobytes() == expected_icon.tobytes(),
                "Embedded sprout icon pixels differ")
        dex_sha = common._hash_entry(archive, "classes2.dex")
        whitelist(archive)
    pins[manifest_path] = common.digest(manifest_path)
    pins[helper_path] = common.digest(helper_path)
    pins[Path(__file__).resolve()] = common.digest(Path(__file__))
    proof = {"schema": "hyjs-feedback78-managed-composite-v1", "baseAPKSHA256": BASE_SHA,
             "inputManagedSHA256": report["inputManagedSHA256"], "inputDexSHA256": dex_sha,
             "inheritedJavaPublicProtectedABIExact": True, "inheritedJavaByteExact": True,
             "managedOnly": True, "managedAssemblyByteExact": True,
             "payloadFiles": report["payloadFiles"], "replacementEntries": report["replacementEntries"],
             "existingHooksVerified": hooks, "helperPublicAPIExact": True,
             "embeddedSproutIconPixelsExact": True, "originalEngineMethodsExact": engine,
             "sourcePins": {str(path.relative_to(ROOT)): digest for path, digest in sorted(pins.items())}}
    return proof


def verify_final(proof):
    result = json.loads(RECORD.read_text())
    require(result["status"] == "PASS_SIGN_ALIGN_SCOPED_OVERLAY_NOT_INSTALLED"
            and common.digest(OUTPUT) == result["binarySHA256"]
            and result["baseAPKSHA256"] == BASE_SHA
            and result["javaByteExact"] and result["managedAssemblyByteExact"]
            and result["allOtherEntriesAndCompressedPayloadsExact"]
            and result["signatureVerified"] and result["zipalignVerified"],
            "Native packaging proof differs")
    require(set(result["replacementEntries"]) == set(EXPECTED_RESOURCE_HASHES)
            | {HELPER_ENTRY, "AndroidManifest.xml"}, "Final replacement inventory differs")
    require(result["replacementEntries"][HELPER_ENTRY] == HELPER_SHA
            and all(result["replacementEntries"][entry] == digest
                    for entry, digest in EXPECTED_RESOURCE_HASHES.items()), "Final payload hashes differ")
    with zipfile.ZipFile(OUTPUT) as archive:
        require(common._hash_entry(archive, common.DLL) == proof["inputManagedSHA256"]
                and common._hash_entry(archive, "classes2.dex") == proof["inputDexSHA256"],
                "Final managed assembly or DEX differs")
        final_pins = whitelist(archive)
        parts = chunks(archive.read("AndroidManifest.xml"))
        pools = [part for part in parts if part[0] == 1]
        require(len(pools) == 1, "Final manifest string pool differs")
        strings, _ = strings_from_pool(pools[0][2])
        manifest_rows = [attributes for tag, attributes in elements(parts, strings) if tag == "manifest"]
        require(len(manifest_rows) == 1, "Final Android manifest differs")
        version = {name: value for _, name, _, value in manifest_rows[0]
                   if name in ("versionCode", "versionName")}
        require(version == {"versionCode": 2026102801, "versionName": "1.0.78"},
                "Final Android version differs")
    result["finalAPKVerification"] = {
        "versionCode": version["versionCode"], "versionName": version["versionName"],
        "resourceWhitelistEntries": len(final_pins), "resourceWhitelistVerified": True,
        "managedAssemblyAndDEXMatchBase": True, "engineMethodCount": proof["originalEngineMethodsExact"],
        "existingSproutHooks": proof["existingHooksVerified"], "helperPublicAPIExact": True,
        "embeddedSproutIconPixelsExact": True,
        "managedInputProof": str((MANAGED_PROOF / "manifest.json").relative_to(ROOT)),
    }
    RECORD.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": "FINAL_APK_VERIFIED", "binarySHA256": result["binarySHA256"],
                      "outputAPK": str(OUTPUT.relative_to(ROOT)), "record": str(RECORD.relative_to(ROOT))}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prepare-only", action="store_true", help="Validate inputs and write managed proof only")
    args = parser.parse_args()
    require(not OUTPUT.exists() and not RECORD.exists(), "Fresh 78 output required")
    proof = prepare_managed_proof()
    check_payload, check_pins = {}, {}
    apply_os_selector_overlay(BASE, OS, check_payload, check_pins)
    MANAGED_PROOF.mkdir(parents=True, exist_ok=True)
    manifest = MANAGED_PROOF / "manifest.json"
    encoded = json.dumps(proof, ensure_ascii=False, indent=2) + "\n"
    if manifest.exists():
        require(manifest.read_text() == encoded, "Existing managed proof differs")
    else:
        manifest.write_text(encoded)
    if args.prepare_only:
        print(json.dumps({"status": "INPUT_REPLAY_PASS", "proof": str(manifest.relative_to(ROOT))}))
        return
    link = ROOT / ".local-signing"
    require(not link.exists() and not link.is_symlink(), "Signing link already exists")
    require(SIGNING.is_dir(), "Existing signing directory missing")
    link.symlink_to(SIGNING, target_is_directory=True)
    try:
        command = ["rtk", "proxy", sys.executable, str(ROOT / "tools/client_release/package_native.py"),
                   "--base-apk", str(BASE), "--overlay", str(MANAGED_PROOF),
                   "--os-selector-description-overlay", str(OS),
                   "--output", str(OUTPUT), "--record", str(RECORD),
                   "--build-code", "2026102801", "--version-name", "1.0.78"]
        subprocess.run(command, cwd=ROOT, check=True)
        verify_final(proof)
    finally:
        link.unlink()


if __name__ == "__main__":
    main()
