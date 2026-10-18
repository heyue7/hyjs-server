"""Compose the approved operations-wallet, unavailable UI and branding overlays.

This entry point only runs when explicitly invoked by the release owner. It
produces one new candidate and an exclusive evidence record. It never installs,
promotes, replaces current/rollback or reruns original patch flags.
"""
import argparse
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import struct
import sys
import tempfile
import time
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
BASE_SHA = "6074ace33c72819ae0e163e3b72c78cd6cb356cecbda30a5a0a82b2297a24518"
BASE_PROOF = ROOT / "analysis/admin-growth-20261017/apk-build-r4.json"
PHONE_PROOF = ROOT / "analysis/green-hero-compat-20261017/phone-r2/final.json"
ICON = "res/drawable/supersdk_icon.png"
RESOURCE_CONFIG = "assets/local-resource-overrides.json"


def require(value, message):
    if not value:
        raise ValueError(message)


def raw_record_range(stream, info):
    """Bound the existing local record without copying old signing blocks."""
    stream.seek(info.header_offset)
    header = stream.read(30)
    require(len(header) == 30 and header[:4] == b"PK\x03\x04", "Invalid inherited ZIP local header")
    flags, method = struct.unpack_from("<HH", header, 6)
    names, extra = struct.unpack_from("<HH", header, 26)
    require(flags == info.flag_bits and method == info.compress_type, "Inherited ZIP local/central flags differ")
    start = info.header_offset + 30 + names + extra
    stop = start + info.compress_size
    if flags & 8:
        require(info.file_size <= 0xffffffff and info.compress_size <= 0xffffffff, "ZIP64 data descriptor is outside fixed APK scope")
        stream.seek(stop)
        descriptor = stream.read(16)
        signed = descriptor[:4] == b"PK\x07\x08"
        values = struct.unpack_from("<III", descriptor, 4 if signed else 0)
        require(values == (info.CRC, info.compress_size, info.file_size), "Inherited ZIP descriptor differs")
        stop += 16 if signed else 12
    return start, stop


def compressed_hash(stream, info):
    start, _ = raw_record_range(stream, info)
    stream.seek(start)
    digest = hashlib.sha256()
    left = info.compress_size
    while left:
        block = stream.read(min(left, 1024 * 1024))
        require(bool(block), "Truncated inherited compressed payload")
        digest.update(block)
        left -= len(block)
    return digest.hexdigest()


def copy_original_record(source, output, info):
    """Keep inherited compressed bytes exact; zipalign may adjust only padding."""
    _, stop = raw_record_range(source, info)
    cloned = copy.copy(info)
    cloned.header_offset = output.fp.tell()
    source.seek(info.header_offset)
    left = stop - info.header_offset
    while left:
        block = source.read(min(left, 1024 * 1024))
        require(bool(block), "Truncated inherited ZIP record")
        output.fp.write(block)
        left -= len(block)
    output.filelist.append(cloned)
    output.NameToInfo[cloned.filename] = cloned
    output.start_dir = output.fp.tell()
    output._didModify = True


def installed_base(archive, proof, phone, common):
    require(proof.get("status") == "PASS_BUILD_VERIFY_CANDIDATE_NOT_INSTALLED" and
            proof.get("binarySHA256") == BASE_SHA and proof.get("signatureVerified") is True and
            proof.get("zipalignVerified") is True, "Fixed 6074 candidate build qualification differs")
    require(phone.get("status") == "PASS_INSTALLED_CACHE_LOGIN_STOPPED" and phone.get("apkSHA256") == BASE_SHA and
            phone.get("installedSHA256MatchesCandidate") is True and
            phone.get("nativeLogin", {}).get("expectedPlayerId") == 4 and phone["nativeLogin"].get("login") == "verified" and
            phone.get("cacheChecks", {}).get("all12Matched") is True,
            "Fixed base lacks matching actual installation/login/dual-cache proof")
    engine = proof.get("bridgeProof", {})
    require(engine.get("originalEngineMethods") == 547 and
            engine.get("originalEngineInstructionBytesAndExceptionRegionsUnchanged") is True,
            "Base lacks inherited original Engine547 qualification")
    entries = proof.get("qualifiedWalletEntries", {})
    require({common.DLL, common.BRIDGE, "classes2.dex", "classes.dex", "classes3.dex", "AndroidManifest.xml",
             "assets/local-wallet.json", "assets/local-wallet/icon.png", RESOURCE_CONFIG}.issubset(entries),
            "Fixed qualified wallet entry inventory is incomplete")
    for name, expected in entries.items():
        require(name in archive.namelist() and common._hash_entry(archive, name) == expected,
                "Fixed base qualified entry differs: " + name)
    require(entries[common.DLL] == engine.get("outputSHA256") and entries[common.BRIDGE] == engine.get("bridgeSHA256"),
            "Base DLL/bridge identities disagree")
    return dict(entries)


def build_release(args):
    from tools import build_wallet_client as common
    from tools.client_branding.branding import NAME, patch_manifest
    from tools.green_hero_compat.build_unavailable_guard import TARGETS, verify as verify_guard

    base, ui, guard, branding = [path.resolve() for path in [args.base_apk, args.ui, args.unavailable_overlay, args.branding_overlay]]
    base_proof, phone_proof, record = [path.resolve() for path in [args.base_proof, args.installed_proof, args.record]]
    candidate = ROOT / "build/hyjs-ops-wallet-candidate.apk"
    require(base == ROOT / "build/hyjs-growth-candidate.apk" and base_proof == BASE_PROOF and phone_proof == PHONE_PROOF,
            "Use the fixed installed 6074 candidate, r4 proof and phone-r2 installation proof")
    require(ui.is_relative_to(ROOT / "build") and guard.is_relative_to(ROOT / "analysis") and
            branding.is_relative_to(ROOT / "analysis") and branding.name != "branding-r1" and
            record.is_relative_to(ROOT / "analysis") and not record.exists(),
            "Use owned fixed overlay directories, latest branding and a new analysis record")
    require(not candidate.exists() and not candidate.with_suffix(".apk.idsig").exists(), "Preserve existing operations-wallet candidate")
    sdk = Path(os.environ.get("ANDROID_SDK_ROOT", Path.home() / "Library/Android/sdk"))
    android = sdk / "build-tools/33.0.0"
    java_home = Path.home() / "Library/Java/JavaVirtualMachines/openjdk-18.0.1.1/Contents/Home"
    key = ROOT / ".local-signing/debug.keystore"
    require(key.is_file(), "Existing project signing identity is required")
    env = dict(os.environ, JAVA_HOME=str(java_home), PATH=str(java_home / "bin") + os.pathsep + os.environ.get("PATH", ""))
    began = time.monotonic()
    temp_path = None
    report = {"status": "PENDING", "mode": "ops-wallet-unavailable-content-and-branding",
              "baseAPK": str(base.relative_to(ROOT)), "baseSHA256": BASE_SHA,
              "phoneUI": False, "installed": False, "promoted": False, "originalAPKRepatchedFromFlags": False,
              "packagerSource": str(Path(__file__).resolve().relative_to(ROOT)),
              "packagerSourceSHA256": common.digest(Path(__file__))}
    source_pins = {Path(__file__).resolve(): report["packagerSourceSHA256"],
                   Path(common.__file__).resolve(): common.digest(Path(common.__file__))}
    with (ROOT / "build/.client-build.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        try:
            protected = {path: common.digest(path) if path.exists() else None for path in
                         [ROOT / "build/hyjs-local.apk", ROOT / "build/hyjs-rollback.apk"]}
            require(common.digest(base) == BASE_SHA, "Fixed installed base APK changed")
            require(not candidate.exists() and not record.exists(), "Candidate or record appeared while waiting for the build lock")
            require(shutil.disk_usage(base.parent).free >= base.stat().st_size * 3 + 200 * 1024 * 1024, "Insufficient owned build space")
            for path in [base_proof, phone_proof, ui / "manifest.json", guard / "manifest.json", branding / "manifest.json"]:
                source_pins[path] = common.digest(path)
            proof = json.loads(base_proof.read_text())
            phone = json.loads(phone_proof.read_text())
            guard_proof = json.loads((guard / "manifest.json").read_text())
            brand_proof = json.loads((branding / "manifest.json").read_text())
            require(guard_proof.get("schema") == "hyjs-unavailable-content-view-guard-v1" and
                    guard_proof.get("status") == "STATIC_GUARD_VERIFIED_NOT_PACKAGED" and
                    set(guard_proof.get("allowedChangedMethods", [])) == TARGETS and
                    guard_proof.get("originalEngineMethodsUnchanged") == 547 and
                    guard_proof.get("allInheritedNonTargetRawMethodsExact") is True and
                    guard_proof.get("formationAndSixGreenInheritedUnchanged") is True,
                    "Unavailable UI overlay qualification differs")
            for name, expected in guard_proof["sourcePins"].items():
                path = (ROOT / name).resolve()
                require(path.is_relative_to(ROOT / "tools/green_hero_compat") and common.digest(path) == expected,
                        "Unavailable UI source pin differs")
                source_pins[path] = expected
            dll = guard / "Assembly-CSharp.dll"
            require(common.digest(dll) == guard_proof["outputManagedSHA256"], "Fixed guarded DLL changed")
            source_pins[dll] = common.digest(dll)
            require(brand_proof.get("schema") == "hyjs-client-branding-overlay-v1" and
                    brand_proof.get("status") == "STATIC_OVERLAY_NOT_PACKAGED" and brand_proof.get("baseAPKSHA256") == BASE_SHA and
                    brand_proof.get("applicationName") == NAME and
                    brand_proof.get("allOtherDecodedElementAttributesExact") is True and
                    brand_proof.get("resourcesArscUnchanged") is True and
                    set(brand_proof.get("replacements", {})) == {"AndroidManifest.xml", ICON},
                    "Latest application branding proof differs")
            branding_source = ROOT / "tools/client_branding/branding.py"
            require(common.digest(branding_source) == brand_proof.get("sourceSHA256"), "Branding source changed")
            source_pins[branding_source] = common.digest(branding_source)
            ui_proof = json.loads((ui / "manifest.json").read_text())
            common.verify_wallet_ui(ui / "classes2.dex", ui / "AndroidManifest.wallet.xml")
            require(common.digest(ui / "classes2.dex") == ui_proof.get("dexSHA256") and
                    common.digest(ui / "AndroidManifest.wallet.xml") == ui_proof.get("manifestProposalSHA256") and
                    ui_proof.get("iconIncluded") is True, "Wallet dex/proposal proof differs")
            seen_sources = set()
            for row in ui_proof["sources"]:
                path = (ROOT / row["path"]).resolve()
                require((path.is_relative_to(ROOT / "tools/local_wallet") or
                         path.is_relative_to(ROOT / "tools/green_hero_compat/java") or
                         path == ROOT / "tools/green_hero_compat/local-resource-overrides.json") and
                        common.digest(path) == row["sha256"], "Wallet source pin differs")
                seen_sources.add(path)
                source_pins[path] = row["sha256"]
            require(ROOT / "tools/green_hero_compat/java/com/hyjs/wallet/LocalAssetOverlay.java" in seen_sources,
                    "Wallet UI lost the inherited six-resource refresh module")
            activity = (ROOT / "tools/local_wallet/src/com/hyjs/wallet/WalletActivity.java").read_text()
            require("LocalAssetOverlay.synchronize(this)" in activity and
                    activity.index("LocalAssetOverlay.synchronize(this)") < activity.index("super.onCreate(state)"),
                    "Resource refresh no longer precedes Unity startup")
            certificate = common._certificate(common.run([android / "apksigner", "verify", "--print-certs", base], env))
            require(certificate == proof["signerCertificateSHA256"], "Installed base signing identity differs")
            with tempfile.TemporaryDirectory(prefix="hyjs-ops-wallet-", dir=ROOT / "build") as temporary:
                temp_path = Path(temporary)
                unsigned, aligned, signed = [temp_path / name for name in ["unsigned.apk", "aligned.apk", "signed.apk"]]
                inherited = {}
                compressed = {}
                metadata = {}
                with zipfile.ZipFile(base) as incoming, base.open("rb") as raw_source:
                    names = incoming.namelist()
                    require(len(names) == len(set(names)), "Base has duplicate ZIP entries")
                    qualified = installed_base(incoming, proof, phone, common)
                    require(common._hash_entry(incoming, common.DLL) == guard_proof["inputManagedSHA256"], "Guard input differs from installed base DLL")
                    checks = verify_guard(incoming.read(common.DLL), dll.read_bytes(), guard_proof["configuration"])
                    require(all(guard_proof.get(key) == value for key, value in checks.items()), "Fresh original Engine/UI guard comparison differs")
                    replacements = {common.DLL: dll.read_bytes(), "classes2.dex": (ui / "classes2.dex").read_bytes()}
                    for entry, row in brand_proof["replacements"].items():
                        path = (branding / row["file"]).resolve()
                        require(path.is_relative_to(branding) and common.digest(path) == row["sha256"], "Fixed branding payload changed")
                        replacements[entry] = path.read_bytes()
                        source_pins[path] = row["sha256"]
                    require(set(replacements) == {common.DLL, "classes2.dex", "AndroidManifest.xml", ICON} and set(replacements).issubset(names),
                            "Exactly four existing APK entries may change")
                    expected_manifest, recomputed_brand = patch_manifest(incoming.read("AndroidManifest.xml"))
                    require(replacements["AndroidManifest.xml"] == expected_manifest and
                            all(brand_proof.get(key) == value for key, value in recomputed_brand.items()), "Recomputed manifest branding differs")
                    require(common.digest(ROOT / "tools/client_branding/icon.png") == hashlib.sha256(replacements[ICON]).hexdigest(),
                            "Branding does not use the current reviewed icon")
                    source_pins[ROOT / "tools/client_branding/icon.png"] = common.digest(ROOT / "tools/client_branding/icon.png")
                    for entry in ["assets/local-wallet.json", "assets/local-wallet/icon.png", RESOURCE_CONFIG]:
                        require(common.digest(ui / entry) == common._hash_entry(incoming, entry), "Wallet configuration/icon/resource whitelist must remain exact")
                        source_pins[ui / entry] = common.digest(ui / entry)
                    require(common.digest(ui / "assets/local-wallet/icon.png") == ui_proof.get("iconSHA256"), "Wallet icon proof differs")
                    source_pins[ui / "classes2.dex"] = common.digest(ui / "classes2.dex")
                    with zipfile.ZipFile(unsigned, "w") as outgoing:
                        outgoing.comment = incoming.comment
                        for info in incoming.infolist():
                            if common._signature(info.filename):
                                continue
                            if info.filename in replacements:
                                outgoing.writestr(copy.copy(info), replacements[info.filename])
                                continue
                            inherited[info.filename] = common._hash_entry(incoming, info.filename)
                            compressed[info.filename] = compressed_hash(raw_source, info)
                            metadata[info.filename] = (info.compress_type, info.compress_size, info.file_size, info.CRC)
                            copy_original_record(raw_source, outgoing, info)
                common.run([android / "zipalign", "-f", "-p", "4", unsigned, aligned], env)
                common.run([android / "apksigner", "sign", "--ks", key, "--ks-pass", "pass:android", "--key-pass", "pass:android", "--out", signed, aligned], env)
                require(common._certificate(common.run([android / "apksigner", "verify", "--verbose", "--print-certs", signed], env)) == certificate,
                        "Signed candidate identity differs")
                common.run([android / "zipalign", "-c", "-p", "4", signed], env)
                hashes = {name: hashlib.sha256(raw).hexdigest() for name, raw in replacements.items()}
                with zipfile.ZipFile(signed) as verified, signed.open("rb") as raw_signed:
                    names = verified.namelist()
                    require(len(names) == len(set(names)) and {n for n in names if not common._signature(n)} == set(inherited) | set(replacements),
                            "Signed candidate ZIP entry set differs")
                    for name, expected in dict(inherited, **hashes).items():
                        require(common._hash_entry(verified, name) == expected, "Candidate inherited/patched payload differs: " + name)
                    for name, expected in compressed.items():
                        info = verified.getinfo(name)
                        require((info.compress_type, info.compress_size, info.file_size, info.CRC) == metadata[name] and
                                compressed_hash(raw_signed, info) == expected, "Inherited compression bytes differ: " + name)
                for path, expected in source_pins.items():
                    require(common.digest(path) == expected, "Fixed build input changed during packaging: " + str(path.relative_to(ROOT)))
                require(common.digest(base) == BASE_SHA, "Installed base changed during packaging")
                for path, expected in protected.items():
                    require((common.digest(path) if path.exists() else None) == expected, "Protected current/rollback changed")
                qualified.update(hashes)
                engine = dict(proof["bridgeProof"])
                engine.update(mode="ops-wallet-unavailable-content-view-guard", inputSHA256=guard_proof["inputManagedSHA256"],
                              outputSHA256=guard_proof["outputManagedSHA256"], freshEngineComparison=True,
                              managedAssemblyByteIdenticalToBase=False, onlyChangedEntries=sorted(TARGETS))
                # Retain prior qualified modules without changing their historical records.
                for key_name in ["modernIME", "originalClassesDexSHA256", "greenHeroProof", "formationGuardProof",
                                 "awakeEndRecoveryOverlayProof", "activityTitleOverlayProof", "formationOverlayProof",
                                 "resourceOverlayProof", "awakeGateRemovalProof"]:
                    if key_name in proof:
                        report[key_name] = proof[key_name]
                report.update(status="PASS_BUILD_VERIFY_CANDIDATE_NOT_INSTALLED", binarySHA256=common.digest(signed),
                              outputAPK=str(candidate.relative_to(ROOT)), signerCertificateSHA256=certificate,
                              signatureVerified=True, zipalignVerified=True, qualifiedWalletEntries=qualified,
                              replacementEntries=hashes, bridgeProof=engine, manifestProof=brand_proof,
                              unavailableContentOverlayProof=guard_proof, brandingOverlayProof=brand_proof,
                              baseWalletProof=str(base_proof.relative_to(ROOT)), baseWalletProofSHA256=source_pins[base_proof],
                              baseInstalledProof=str(phone_proof.relative_to(ROOT)), baseInstalledProofSHA256=source_pins[phone_proof],
                              ui=str(ui.relative_to(ROOT)), uiProofSHA256=source_pins[ui / "manifest.json"],
                              unavailableOverlay=str(guard.relative_to(ROOT)), unavailableOverlaySHA256=source_pins[guard / "manifest.json"],
                              brandingOverlay=str(branding.relative_to(ROOT)), brandingOverlaySHA256=source_pins[branding / "manifest.json"],
                              allOtherInheritedEntriesExact=True, inheritedEntryCount=len(inherited),
                              inheritedEntrySHA256=hashlib.sha256(json.dumps(inherited, sort_keys=True, separators=(",", ":")).encode()).hexdigest(),
                              inheritedCompressedPayloadsExact=True,
                              inheritedCompressedPayloadsSHA256=hashlib.sha256(json.dumps(compressed, sort_keys=True, separators=(",", ":")).encode()).hexdigest(),
                              classesDexSHA256=inherited.get("classes.dex"), baseClassesDexByteIdentical=True,
                              modernIMEInheritedFromBase=True, walletConfigAndIconInheritedExact=True,
                              resourceRefreshBeforeUnity=True, resourceRefreshWhitelistEntries=6,
                              managedAssemblyByteIdenticalToBase=False, currentAPKUnchanged=True, rollbackAPKUnchanged=True,
                              basePromotionRequired=False, baseQualification="fixed-build-and-actual-installed-proof")
                signed.replace(candidate)
                idsig = signed.with_suffix(".apk.idsig")
                if idsig.exists():
                    idsig.replace(candidate.with_suffix(".apk.idsig"))
        except BaseException as error:
            report.update(status="FAILED_NOT_QUALIFIED", errorType=type(error).__name__, error=str(error)[-6000:])
            raise
        finally:
            report.update(ownedTemporaryFilesCleaned=temp_path is None or not temp_path.exists(), seconds=round(time.monotonic() - began, 3))
            record.parent.mkdir(parents=True, exist_ok=True)
            with record.open("x") as evidence:
                evidence.write(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: report[key] for key in ["status", "binarySHA256", "inheritedEntryCount", "outputAPK"]}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--base-proof", type=Path, required=True)
    parser.add_argument("--installed-proof", type=Path, required=True)
    parser.add_argument("--ui", type=Path, required=True)
    parser.add_argument("--unavailable-overlay", type=Path, required=True)
    parser.add_argument("--branding-overlay", type=Path, required=True)
    parser.add_argument("--record", type=Path, required=True)
    build_release(parser.parse_args())


if __name__ == "__main__":
    main()
