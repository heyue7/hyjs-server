"""Package the fixed playability/UI overlays once; preserve unrelated APK entries."""
import argparse
import copy
import fcntl
import hashlib
import json
import os
import shutil
from pathlib import Path
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.package_release import compressed_hash, copy_original_record, require
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
from tools.client_playability.build import TARGETS as PLAYABILITY
from tools.client_display.build import TARGETS as DISPLAY
from tools.client_activity.build import TARGETS as ACTIVITY
import dnfile

BASE = "b9825732e3aa82714483645b0b2d87431bc56a58992d32c1c0ca637e01d941af"
AUDIT = ROOT / "analysis/playability-fixes-20261018"
ENTRY = "assets/bin/Data/Managed/"


def verify_managed(before, after):
    allowed = set(PLAYABILITY) | set(DISPLAY) | set(ACTIVITY) | {"UserInput::OnLoginButton"}
    a, b = [dnfile.dnPE(data=raw) for raw in (before, after)]
    old, new = methods(a), methods(b)
    require(len(old) == len(new), "Managed method inventory changed")
    require(not getattr(b.net.mdtables, "FieldPtr", None), "Old Unity cannot gain FieldPtr")
    for name in ("TypeDef", "Field"):
        require(len(getattr(a.net.mdtables, name).rows) == len(getattr(b.net.mdtables, name).rows), "Managed ABI table changed")
    changed, engine, unchanged = [], 0, 0
    for (name, rid, m), (other, oid, n) in zip(old, new):
        require((name, rid, m.Signature.value, m.struct.Flags, m.struct.ImplFlags) ==
                (other, oid, n.Signature.value, n.struct.Flags, n.struct.ImplFlags), "Managed ABI changed: " + name)
        if not m.Rva:
            require(not n.Rva, "Unexpected managed body")
            continue
        left, right = exact_body(a, m), exact_body(b, n)
        if left != right:
            require(name in allowed, "Unapproved method changed: " + name)
            changed.append(name)
        else:
            unchanged += 1
            engine += name.startswith("Battle.Engine")
    for ref in b.net.mdtables.TypeRef.rows:
        require(not (ref.ResolutionScope.table.name == "ModuleRef" and str(ref.ResolutionScope.row.Name) in {"SoulFlowerDisplay.dll", "SoulFlowerActivity.dll"}), "Host type points to helper netmodule")
    require(set(changed) == allowed and engine == 547, "Expected overlay union or original engine differs")
    return {"changedMethods": changed, "originalEngineMethodsExact": engine, "otherInheritedMethodsExact": unchanged}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--base-apk", type=Path, help="Explicit qualified b982 baseline; promotion normally moves it to the rollback path")
    p.add_argument("--output", type=Path, required=True)
    p.add_argument("--record", type=Path, required=True)
    p.add_argument("--preview-without-art", action="store_true", help="Temporary UI verification only; never a final deliverable")
    args = p.parse_args()
    base = args.base_apk.resolve() if args.base_apk else ROOT / "apks/build/hyjs-local.apk"
    if not args.base_apk and common.digest(base) != BASE:
        base = ROOT / "apks/build/hyjs-rollback.apk"
    output, record = args.output.resolve(), args.record.resolve()
    require(output.parent == ROOT / "apks/build" and not output.exists() and not record.exists(), "Use fresh owned output and record")
    require(common.digest(base) == BASE, "Qualified base changed")
    overlays = {
        "playability": AUDIT / "client/overlay-r3",
        "display": AUDIT / "display/overlay-scope-entry-release-r5",
        "activity": AUDIT / "activity/overlay-import-scope-r3",
        "login": AUDIT / "login-ui/overlay-final-r5",
        "bindings": AUDIT / "client/host-bindings-r1",
    }
    manifests = {key: json.loads((folder / "manifest.json").read_text()) for key, folder in overlays.items()}
    pins = {}
    for key, manifest in manifests.items():
        for name, digest in manifest.get("sourcePins", {}).items():
            path = ROOT / name
            require(path.resolve().is_relative_to(ROOT) and common.digest(path) == digest, "Source pin changed: " + name)
            pins[path] = digest
    files = {
        ENTRY + "Assembly-CSharp.dll": overlays["bindings"] / "Assembly-CSharp.dll",
        ENTRY + "LocalFormationUI.dll": overlays["playability"] / "LocalFormationUI.dll",
        ENTRY + "LocalPlayability.dll": overlays["playability"] / "LocalPlayability.dll",
        ENTRY + "SoulFlowerDisplay.dll": overlays["display"] / "SoulFlowerDisplay.dll",
        ENTRY + "SoulFlowerActivity.dll": overlays["activity"] / "SoulFlowerActivity.dll",
        "classes2.dex": overlays["login"] / "classes2.dex",
        "assets/login-ui/panel.png": ROOT / "tools/login_ui/assets/panel.png",
    }
    if args.preview_without_art:
        files.pop("assets/login-ui/panel.png")
    payload = {name: path.read_bytes() for name, path in files.items()}
    hashes = {name: hashlib.sha256(raw).hexdigest() for name, raw in payload.items()}
    require(hashes[ENTRY + "SoulFlowerActivity.dll"] == manifests["activity"]["bridgeSHA256"], "Activity bridge differs")
    require(hashes[common.DLL] == manifests["bindings"]["outputManagedSHA256"], "Final main DLL differs")
    require(hashes["classes2.dex"] == manifests["login"]["outputDexSHA256"], "Final native UI differs")
    sdk = Path.home() / "Library/Android/sdk/build-tools/33.0.0"
    jdk = Path("/opt/homebrew/opt/openjdk@21")
    env = dict(os.environ, JAVA_HOME=str(jdk), PATH=str(jdk / "bin") + os.pathsep + os.environ.get("PATH", ""))
    certificate = common._certificate(common.run([sdk / "apksigner", "verify", "--print-certs", base], env))
    inherited, compressed = {}, {}
    require(shutil.disk_usage(ROOT).free > base.stat().st_size * 2 + 150_000_000, "Need space for two temporary APK copies plus margin")
    output.parent.mkdir(exist_ok=True)
    with (ROOT / "build/.client-build.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        require(common.digest(base) == BASE, "Base changed while waiting")
        with tempfile.TemporaryDirectory(prefix="hyjs-final-client-", dir=ROOT / "build") as temporary:
            temp = Path(temporary)
            unsigned, aligned, signed = [temp / name for name in ("unsigned.apk", "aligned.apk", "signed.apk")]
            with zipfile.ZipFile(base) as incoming, base.open("rb") as stream:
                require(len(incoming.namelist()) == len(set(incoming.namelist())), "Duplicate base entries")
                checks = verify_managed(incoming.read(common.DLL), payload[common.DLL])
                for name, digest in manifests["login"]["originalDexEntriesMustRemainExact"].items():
                    require(common._hash_entry(incoming, name) == digest, "Inherited IME or original Java changed")
                with zipfile.ZipFile(unsigned, "w") as target:
                    target.comment = incoming.comment
                    for info in incoming.infolist():
                        if common._signature(info.filename):
                            continue
                        if info.filename in payload:
                            target.writestr(copy.copy(info), payload[info.filename])
                        else:
                            inherited[info.filename] = common._hash_entry(incoming, info.filename)
                            compressed[info.filename] = compressed_hash(stream, info)
                            copy_original_record(stream, target, info)
                    for name in set(payload) - set(incoming.namelist()):
                        target.writestr(name, payload[name], compress_type=zipfile.ZIP_DEFLATED)
            common.run([sdk / "zipalign", "-f", "-p", "4", unsigned, aligned], env)
            # The aligned copy is complete; release the reproducible first copy
            # before signing so APK preparation needs at most two full copies.
            unsigned.unlink()
            common.run([sdk / "apksigner", "sign", "--ks", ROOT / ".local-signing/debug.keystore", "--ks-pass", "pass:android", "--key-pass", "pass:android", "--out", signed, aligned], env)
            aligned.unlink()
            require(common._certificate(common.run([sdk / "apksigner", "verify", "--verbose", "--print-certs", signed], env)) == certificate, "Signer changed")
            common.run([sdk / "zipalign", "-c", "-p", "4", signed], env)
            with zipfile.ZipFile(signed) as actual, signed.open("rb") as stream:
                names = actual.namelist()
                require(len(names) == len(set(names)) and {n for n in names if not common._signature(n)} == set(inherited) | set(payload), "Final inventory differs")
                for name, digest in {**inherited, **hashes}.items():
                    require(common._hash_entry(actual, name) == digest, "Final entry differs: " + name)
                for name, digest in compressed.items():
                    require(compressed_hash(stream, actual.getinfo(name)) == digest, "Inherited compressed data changed")
            for path, digest in pins.items():
                require(common.digest(path) == digest, "Source changed during packaging")
            require(common.digest(base) == BASE, "Current deliverable changed")
            result = dict(status="PREVIEW_WITHOUT_ART_NOT_FINAL" if args.preview_without_art else "PASS_BUILD_SIGN_ALIGN_NOT_INSTALLED", baseAPKSHA256=BASE,
                          binarySHA256=common.digest(signed), outputAPK=str(output.relative_to(ROOT)),
                          replacementEntries=hashes, inheritedEntryCount=len(inherited),
                          allOtherEntriesAndCompressedPayloadsExact=True,
                          signerCertificateSHA256=certificate, signatureVerified=True, zipalignVerified=True,
                          originalClassesDexAndModernIMEExact=True, imageAssetIncluded=not args.preview_without_art, **checks)
            signed.replace(output)
            sidecar = signed.with_suffix(".apk.idsig")
            if sidecar.exists():
                sidecar.replace(output.with_suffix(".apk.idsig"))
    record.parent.mkdir(parents=True, exist_ok=True)
    record.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: result[key] for key in ("status", "binarySHA256", "outputAPK", "originalEngineMethodsExact")}))


if __name__ == "__main__":
    main()
