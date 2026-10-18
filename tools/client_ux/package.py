"""Sign the original-style client UI candidate while preserving every other inherited APK entry."""
import argparse
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.package_release import compressed_hash, copy_original_record, require
from tools.client_ux.build import verify

BASE = "b0b8b4a04c46dd8d002db9ecf34303cf122aba9a073fc85244315b2be9cdb0b7"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--overlay", type=Path, required=True)
    parser.add_argument("--record", type=Path, required=True)
    args = parser.parse_args()
    base = ROOT / "build/hyjs-local.apk"
    baseproof = ROOT / "analysis/game-registration-20261018/apk-build-r1.json"
    phoneproof = ROOT / "analysis/game-registration-20261018/phone-r1/final.json"
    candidate = ROOT / "build/hyjs-client-ux-candidate.apk"
    overlay, record = args.overlay.resolve(), args.record.resolve()
    require(not candidate.exists() and not record.exists(), "Preserve existing candidate and records")
    proof = json.loads(baseproof.read_text())
    phone = json.loads(phoneproof.read_text())
    qualification = json.loads((overlay / "manifest.json").read_text())
    require(common.digest(base) == BASE == proof["binarySHA256"] == phone["apkSHA256"], "Installed qualified base differs")
    require(proof["signatureVerified"] and proof["zipalignVerified"] and phone["status"] == "PASS_EXPLICIT_REGISTRATION_LOGIN_CREATE_PAGE_STOPPED", "Installed base qualification incomplete")
    require(qualification["baseAPKSHA256"] == BASE and qualification["originalEngineMethodsUnchanged"] == 547, "Registration qualification differs")
    pins = {Path(__file__).resolve(): common.digest(Path(__file__)), baseproof: common.digest(baseproof), phoneproof: common.digest(phoneproof), overlay / "manifest.json": common.digest(overlay / "manifest.json")}
    for name, expected in qualification["sourcePins"].items():
        path = ROOT / name
        require(path.resolve().is_relative_to(ROOT / "tools/client_ux") and common.digest(path) == expected, "Registration source pin differs")
        pins[path] = expected
    replacements = {common.DLL: (overlay / "Assembly-CSharp.dll").read_bytes(), "classes2.dex": (overlay / "classes2.dex").read_bytes()}
    hashes = {name: hashlib.sha256(raw).hexdigest() for name, raw in replacements.items()}
    require(hashes[common.DLL] == qualification["outputManagedSHA256"] and hashes["classes2.dex"] == qualification["outputDexSHA256"], "Overlay payload changed")
    sdk = Path.home() / "Library/Android/sdk/build-tools/33.0.0"
    java = Path.home() / "Library/Java/JavaVirtualMachines/openjdk-18.0.1.1/Contents/Home"
    env = dict(os.environ, JAVA_HOME=str(java), PATH=str(java / "bin") + os.pathsep + os.environ.get("PATH", ""))
    key = ROOT / ".local-signing/debug.keystore"
    require(key.is_file(), "Existing signing identity absent")
    with (ROOT / "build/.client-build.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        require(common.digest(base) == BASE and not candidate.exists(), "Base or output changed while waiting for lock")
        protected = {p: common.digest(p) if p.exists() else None for p in [base, ROOT / "build/hyjs-rollback.apk"]}
        certificate = common._certificate(common.run([sdk / "apksigner", "verify", "--print-certs", base], env))
        require(certificate == proof["signerCertificateSHA256"], "Base signer differs")
        inherited, compressed, metadata = {}, {}, {}
        with tempfile.TemporaryDirectory(prefix="hyjs-registration-apk-", dir=ROOT / "build") as temporary:
            temp = Path(temporary)
            unsigned, aligned, signed = [temp / name for name in ["unsigned.apk", "aligned.apk", "signed.apk"]]
            with zipfile.ZipFile(base) as incoming, base.open("rb") as stream:
                require(len(incoming.namelist()) == len(set(incoming.namelist())), "Duplicate base entries")
                require(common._hash_entry(incoming, common.DLL) == qualification["inputManagedSHA256"] and common._hash_entry(incoming, "classes2.dex") == qualification["inputDexSHA256"], "Overlay inputs differ")
                with zipfile.ZipFile(ROOT / "apks/original/10056262.apk") as original_apk: original = original_apk.read(common.DLL)
                checks = verify(incoming.read(common.DLL), replacements[common.DLL], original)
                for name, expected in proof["qualifiedWalletEntries"].items():
                    require(common._hash_entry(incoming, name) == expected, "Base qualified entry differs: " + name)
                with zipfile.ZipFile(unsigned, "w") as outgoing:
                    outgoing.comment = incoming.comment
                    for info in incoming.infolist():
                        if common._signature(info.filename): continue
                        if info.filename in replacements:
                            outgoing.writestr(copy.copy(info), replacements[info.filename]); continue
                        inherited[info.filename] = common._hash_entry(incoming, info.filename)
                        compressed[info.filename] = compressed_hash(stream, info)
                        metadata[info.filename] = (info.compress_type, info.compress_size, info.file_size, info.CRC)
                        copy_original_record(stream, outgoing, info)
            common.run([sdk / "zipalign", "-f", "-p", "4", unsigned, aligned], env)
            common.run([sdk / "apksigner", "sign", "--ks", key, "--ks-pass", "pass:android", "--key-pass", "pass:android", "--out", signed, aligned], env)
            require(common._certificate(common.run([sdk / "apksigner", "verify", "--verbose", "--print-certs", signed], env)) == certificate, "Candidate signer differs")
            common.run([sdk / "zipalign", "-c", "-p", "4", signed], env)
            with zipfile.ZipFile(signed) as actual, signed.open("rb") as stream:
                names = actual.namelist()
                require(len(names) == len(set(names)) and {n for n in names if not common._signature(n)} == set(inherited) | set(replacements), "Candidate entry inventory differs")
                for name, expected in dict(inherited, **hashes).items():
                    require(common._hash_entry(actual, name) == expected, "Candidate payload differs: " + name)
                for name, expected in compressed.items():
                    info = actual.getinfo(name)
                    require((info.compress_type, info.compress_size, info.file_size, info.CRC) == metadata[name] and compressed_hash(stream, info) == expected, "Inherited compressed payload differs: " + name)
            for path, expected in pins.items(): require(common.digest(path) == expected, "Build source changed: " + str(path))
            for path, expected in protected.items(): require((common.digest(path) if path.exists() else None) == expected, "Current/rollback changed")
            result = dict(proof)
            qualified = dict(proof["qualifiedWalletEntries"]); qualified.update(hashes)
            bridge = dict(proof["bridgeProof"])
            bridge.update(outputSHA256=hashes[common.DLL], inputSHA256=qualification["inputManagedSHA256"], freshEngineComparison=True, onlyChangedEntries=[row["method"] for row in checks["changes"]])
            result.update(status="PASS_BUILD_VERIFY_CANDIDATE_NOT_INSTALLED", binarySHA256=common.digest(signed), outputAPK=str(candidate.relative_to(ROOT)),
                          mode="original-guides-notice-feedback", baseAPK=str(base.relative_to(ROOT)), actualBaseAPK=str(base.relative_to(ROOT)),
                          baseSHA256=BASE, baseWalletProof=str(baseproof.relative_to(ROOT)), baseInstalledProof=str(phoneproof.relative_to(ROOT)),
                          signatureVerified=True, zipalignVerified=True, signerCertificateSHA256=certificate,
                          qualifiedWalletEntries=qualified, bridgeProof=bridge, replacementEntries=hashes,
                          clientUXOverlayProof=qualification, clientUXOverlay=str(overlay.relative_to(ROOT)),
                          allOtherInheritedEntriesExact=True, inheritedEntryCount=len(inherited), inheritedCompressedPayloadsExact=True,
                          currentAPKUnchanged=True, rollbackAPKUnchanged=True, installed=False, promoted=False,
                          packagerSourceSHA256=common.digest(Path(__file__)), **checks)
            signed.replace(candidate)
            idsig = signed.with_suffix(".apk.idsig")
            if idsig.exists(): idsig.replace(candidate.with_suffix(".apk.idsig"))
        result["ownedTemporaryFilesCleaned"] = not temp.exists()
        with record.open("x") as handle: handle.write(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({k: result[k] for k in ["status", "binarySHA256", "outputAPK", "inheritedEntryCount"]}))


if __name__ == "__main__": main()
