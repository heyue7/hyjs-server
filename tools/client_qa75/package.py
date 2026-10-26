"""Package one isolated QA75 .multi APK from the frozen base and audited overlay."""
from __future__ import annotations

import argparse
import copy
import fcntl
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_qa75 import build
from tools.client_qa75.manifest import patch_package
from tools import build_multiclient as multi
from tools.client_branding.package_release import copy_original_record


def package(base, proof, proof_sha, overlay, keystore):
    binding = build.read_binding(base, proof, proof_sha)
    base, overlay, keystore = Path(base).resolve(), Path(overlay).resolve(), Path(keystore)
    build.require(overlay.is_relative_to(ROOT / "analysis/growth75") and overlay.is_dir(), "Reviewed overlay under analysis/growth75 required")
    qualification_path = overlay / "manifest.json"
    qualification = json.loads(qualification_path.read_text())
    qualification_sha = build.file_sha(qualification_path)
    build.require(qualification.get("schema") == "hyjs-qa75-loopback-overlay-v1"
                  and qualification.get("status") == "STATIC_VERIFIED_NOT_PACKAGED", "Unqualified overlay")
    build.require(all(qualification.get(k) == v for k, v in binding.items()), "Overlay belongs to a different frozen input")
    build.require(qualification.get("sourcePins") == build.source_pins(), "Audited overlay tool sources changed")
    origin = build.qa_origin(qualification.get("origin"))
    before, wallet_before, old = build.read_base(base)
    build.require(old == qualification.get("inheritedOrigin")
                  and build.sha(before) == qualification.get("inputManagedSHA256")
                  and build.sha(wallet_before) == qualification.get("inputWalletConfigSHA256"), "Overlay input payload mismatch")
    after, wallet_after = (overlay / "Assembly-CSharp.dll").read_bytes(), (overlay / "local-wallet.json").read_bytes()
    build.require(build.sha(after) == qualification.get("outputManagedSHA256")
                  and build.sha(wallet_after) == qualification.get("outputWalletConfigSHA256")
                  and json.loads(wallet_after) == {"origin": origin}, "Overlay output payload mismatch")
    checks = build.verify(before, after, old, origin)
    build.require(keystore.is_file() and not keystore.is_symlink(), "Explicit existing signing identity required")
    build.require(shutil.disk_usage(overlay).free > base.stat().st_size * 3 + 200_000_000, "Insufficient owned temporary packaging space")
    output, record = overlay / "hyjs-qa75-local-multi.apk", overlay / "apk-proof.json"
    build.require(not output.exists() and not record.exists(), "Preserve existing QA APK and proof; use a fresh overlay")
    pins = build.source_pins()
    env = dict(os.environ, JAVA_HOME=str(multi.JAVA), PATH=str(multi.JAVA / "bin") + os.pathsep + os.environ.get("PATH", ""))

    def run(name, command):
        build.run(command, ROOT, overlay / (name + ".log"), env)
        return (overlay / (name + ".log")).read_text()

    with (overlay / ".package.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        build.require(not output.exists() and not record.exists(), "QA output changed while waiting for its own lock")
        build.require(build.read_binding(base, proof, proof_sha) == binding, "Frozen base changed")
        certificate = multi.cert_digest(base)
        before_inventory = multi.payload_inventory(base)
        with zipfile.ZipFile(base) as incoming:
            original_manifest = incoming.read(build.MANIFEST)
        # Reuse the existing binary-AXML parser and pool layout. Change only the
        # .local package; every current label/string/XML node stays inherited.
        patched_manifest, manifest_changes, label = patch_package(original_manifest)
        replacements = {build.DLL: after, build.WALLET: wallet_after, build.MANIFEST: patched_manifest}
        with tempfile.TemporaryDirectory(prefix=".qa75-apk-", dir=overlay) as temporary:
            work = Path(temporary)
            unsigned, aligned, signed = [work / name for name in ("unsigned.apk", "aligned.apk", "signed.apk")]
            with zipfile.ZipFile(base) as incoming, base.open("rb") as stream, zipfile.ZipFile(unsigned, "w") as outgoing:
                outgoing.comment = incoming.comment
                for info in incoming.infolist():
                    if multi.signature_entry(info.filename):
                        continue
                    if info.filename in replacements:
                        outgoing.writestr(copy.copy(info), replacements[info.filename])
                    else:
                        copy_original_record(stream, outgoing, info)
            run("zipalign", [multi.SDK / "zipalign", "-f", "-p", "4", unsigned, aligned])
            unsigned.unlink()
            run("sign", [multi.SDK / "apksigner", "sign", "--ks", keystore, "--ks-key-alias", "androiddebugkey",
                         "--ks-pass", "pass:android", "--key-pass", "pass:android", "--v4-signing-enabled", "false",
                         "--out", signed, aligned])
            aligned.unlink()
            run("verify-align", [multi.SDK / "zipalign", "-c", "-p", "4", signed])
            build.require(multi.cert_digest(signed) == certificate, "QA signer differs from the frozen input signer")
            after_inventory = multi.payload_inventory(signed)
            inherited = {n: v for n, v in before_inventory.items() if n not in replacements and not multi.signature_entry(n)}
            actual = {n: v for n, v in after_inventory.items() if n not in replacements and not multi.signature_entry(n)}
            build.require(inherited == actual, "Unrelated resource/DEX/compressed ZIP payload changed")
            build.require({n for n in after_inventory if not multi.signature_entry(n)} == set(inherited) | set(replacements), "QA ZIP entry inventory changed")
            with zipfile.ZipFile(signed) as archive:
                for name, raw in replacements.items():
                    build.require(archive.read(name) == raw, "Signed replacement changed: " + name)
            badging = run("package-label", [multi.SDK / "aapt2", "dump", "badging", signed])
            build.require("package: name='" + multi.TARGET_PACKAGE + "'" in badging
                          and "application-label:'" + label + "'" in badging, "SDK package/label verification failed")
            build.require(build.read_binding(base, proof, proof_sha) == binding and build.source_pins() == pins
                          and build.file_sha(qualification_path) == qualification_sha, "Frozen input, overlay or tools changed during packaging")
            result = {"schema": "hyjs-qa75-loopback-apk-v1", "status": "PASS_QA75_SIGN_ALIGN_STATIC_NOT_INSTALLED",
                      **binding, "binarySHA256": build.file_sha(signed), "outputAPK": str(output),
                      "origin": origin, "package": multi.TARGET_PACKAGE, "label": label,
                      "signingCertificateSHA256": certificate, "signatureVerified": True, "zipalignVerified": True,
                      "replacementEntries": {n: build.sha(raw) for n, raw in replacements.items()},
                      "manifestChanges": manifest_changes, "sourcePins": pins, "overlayManifestSHA256": qualification_sha,
                      "unchangedCompressedPayloadEntries": len(inherited),
                      "unchangedCompressedPayloadInventorySHA256": build.sha(json.dumps(inherited, sort_keys=True, separators=(",", ":")).encode()),
                      "dexUnchanged": {n: v for n, v in inherited.items() if n.endswith(".dex")},
                      "allOtherInheritedEntriesAndCompressedPayloadsExact": True,
                      "formalAPKUnchanged": True, "installed": False, "phoneExecuted": False,
                      "networkServicesContacted": False, "promoted": False, "neverPromote": True,
                      "tcpContract": "Original SCUserLogin returns127.0.0.1:18862; coordinator owns adb reverse",
                      **checks}
            # Publish proof and APK only after every guard succeeds. No root build
            # pointer, account data, device or network endpoint is accessed.
            signed.replace(output)
        result["ownedIntermediatesCleaned"] = not work.exists()
        with record.open("x") as handle:
            handle.write(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--base-manifest", type=Path, required=True)
    parser.add_argument("--base-manifest-sha256", required=True)
    parser.add_argument("--overlay", type=Path, required=True)
    parser.add_argument("--keystore", type=Path, required=True)
    args = parser.parse_args()
    result = package(args.base_apk, args.base_manifest, args.base_manifest_sha256, args.overlay, args.keystore)
    print(json.dumps({k: result[k] for k in ("status", "binarySHA256", "outputAPK", "package", "originalEngineMethodsUnchanged")}))


if __name__ == "__main__":
    main()
