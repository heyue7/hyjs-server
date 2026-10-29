"""Build the pinned 82 candidate and its loopback-only multi QA derivative."""
import argparse
import copy
import fcntl
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
HOST = Path("/Users/lirui/LR/hyjs")
sys.path.insert(0, str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.package_release import compressed_hash, copy_original_record
from tools.client_qa75 import build as qa
from tools.client_qa75.manifest import patch_package
from tools.client_release.package_native import verify_final_resource_pins, verify_java_class_inventory
from tools.client_trial_refresh82 import build as overlay_build
from tools.client_update.package import patch_manifest

BASE_SHA = "25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a"
VERSION = "1.0.82"
BUILD_CODE = 2026102901
QA_ORIGIN = "http://127.0.0.1:18871"
QA_TCP = "127.0.0.1:18872"
LOCAL_PACKAGE = "com.yunchang.djsy.ewan.local"
MULTI_PACKAGE = "com.yunchang.djsy.ewan.multi"
MANAGED = "assets/bin/Data/Managed/Assembly-CSharp.dll"
WALLET = "assets/local-wallet.json"
MANIFEST = "AndroidManifest.xml"
SDK = Path.home() / "Library/Android/sdk/build-tools/33.0.0"
JDK = Path("/opt/homebrew/opt/openjdk@21")
KEYSTORE = HOST / ".local-signing/debug.keystore"


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def run(args, env, timeout=300):
    result = subprocess.run(["rtk", "proxy", *map(str, args)], cwd=ROOT, env=env,
                            capture_output=True, text=True, timeout=timeout)
    require(result.returncode == 0, "command failed: " + str(args[0]) + "\n" + (result.stdout + result.stderr)[-5000:])
    return result.stdout


def package_one(source, destination, replacements, package_name, certificate, env, work, label):
    unsigned, aligned, signed = [work / (label + "-" + suffix + ".apk") for suffix in ("unsigned", "aligned", "signed")]
    inherited, compressed = {}, {}
    with zipfile.ZipFile(source) as incoming, source.open("rb") as stream, zipfile.ZipFile(unsigned, "w") as outgoing:
        names = incoming.namelist()
        require(len(names) == len(set(names)) and set(replacements) <= set(names), "input entries missing or duplicated")
        outgoing.comment = incoming.comment
        for info in incoming.infolist():
            if common._signature(info.filename):
                continue
            if info.filename in replacements:
                outgoing.writestr(copy.copy(info), replacements[info.filename])
            else:
                inherited[info.filename] = common._hash_entry(incoming, info.filename)
                compressed[info.filename] = compressed_hash(stream, info)
                copy_original_record(stream, outgoing, info)
    run([SDK / "zipalign", "-f", "-p", "4", unsigned, aligned], env)
    unsigned.unlink()
    run([SDK / "apksigner", "sign", "--ks", KEYSTORE, "--ks-key-alias", "androiddebugkey", "--ks-pass", "env:HYJS_TRIAL82_SIGN_PASS",
         "--key-pass", "env:HYJS_TRIAL82_SIGN_PASS", "--v4-signing-enabled", "false", "--out", signed, aligned], env)
    aligned.unlink()
    actual_certificate = common._certificate(run([SDK / "apksigner", "verify", "--verbose", "--print-certs", signed], env))
    require(actual_certificate == certificate, "signer changed")
    run([SDK / "zipalign", "-c", "-p", "4", signed], env)
    badging = run([SDK / "aapt", "dump", "badging", signed], env)
    require("package: name='" + package_name + "'" in badging and "versionCode='" + str(BUILD_CODE) + "'" in badging
            and "versionName='" + VERSION + "'" in badging, "manifest package/version mismatch")
    changed = {name: sha(raw) for name, raw in replacements.items()}
    with zipfile.ZipFile(signed) as verified, signed.open("rb") as stream:
        names = verified.namelist()
        require(len(names) == len(set(names)) and {n for n in names if not common._signature(n)} == set(inherited) | set(changed), "final ZIP inventory changed")
        for name, expected in {**inherited, **changed}.items():
            require(common._hash_entry(verified, name) == expected, "final entry differs: " + name)
        for name, expected in compressed.items():
            require(compressed_hash(stream, verified.getinfo(name)) == expected, "inherited compressed bytes differ: " + name)
        verify_final_resource_pins(verified, {})
    result = {"binarySHA256": common.digest(signed), "package": package_name, "versionName": VERSION,
              "versionCode": BUILD_CODE, "replacementEntries": changed, "inheritedEntries": len(inherited),
              "allOtherEntriesAndCompressedPayloadsExact": True, "allInheritedDexByteExact": True,
              "signingCertificateSHA256": certificate, "signatureVerified": True, "zipalignVerified": True,
              "deviceTested": False, "installed": False, "promoted": False}
    signed.replace(destination)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--overlay", type=Path, required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    base, overlay, out = args.base_apk.resolve(), args.overlay.resolve(), args.output_directory.resolve()
    require(out == HOST / "build/trial82" and not out.exists(), "fresh task-owned output directory required")
    require(base.is_file() and not base.is_symlink() and common.digest(base) == BASE_SHA, "80 base APK changed")
    require(KEYSTORE.is_file() and not KEYSTORE.is_symlink(), "existing signing identity unavailable")
    proof_path = overlay / "manifest.json"
    proof = json.loads(proof_path.read_text())
    require(proof.get("baseAPKSHA256") == BASE_SHA and proof.get("managedOnly") is True
            and proof.get("originalEngineMethodsExact") == 547 and proof.get("allOtherOriginalBodiesByteExact") is True
            and proof.get("inheritedHelperPublicMethodsCompatible") is True, "overlay proof is incomplete")
    for name, expected in proof["sourcePins"].items():
        path = (ROOT / name).resolve()
        require(path.is_relative_to(ROOT) and common.digest(path) == expected, "overlay source changed: " + name)
    payload = {name: (ROOT / path).read_bytes() for name, path in proof["payloadFiles"].items()}
    require(set(payload) == {MANAGED, "assets/bin/Data/Managed/SoulFlowerGGOShop.dll"}, "unexpected overlay entries")
    require(all(sha(value) == proof["replacementEntries"][name] for name, value in payload.items()), "overlay payload changed")
    require(shutil.disk_usage(HOST).free > base.stat().st_size * 4 + 300_000_000, "insufficient packaging space")
    out.mkdir(parents=True)
    env = dict(os.environ, JAVA_HOME=str(JDK), PATH=str(JDK / "bin") + os.pathsep + os.environ.get("PATH", ""),
               HYJS_TRIAL82_SIGN_PASS="android", DOTNET_CLI_TELEMETRY_OPTOUT="1")
    cert = common._certificate(run([SDK / "apksigner", "verify", "--print-certs", base], env))
    release = out / "hyjs-1.0.82-candidate-r1.apk"
    qa_output = out / "hyjs-1.0.82-qa-multi-18871-r1.apk"
    lock_path = HOST / "build/.client-build.lock"
    with lock_path.open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        with tempfile.TemporaryDirectory(prefix=".trial82-package-", dir=out) as temporary:
            work = Path(temporary)
            with zipfile.ZipFile(base) as archive:
                before = archive.read(MANAGED)
                require(sha(before) == proof["inputManagedSHA256"], "base managed hash differs")
                check = overlay_build.check_main(before, payload[MANAGED])
                require(check["originalEngineMethodsExact"] == 547, "managed proof changed")
                verify_final_resource_pins(archive, payload)
                java = verify_java_class_inventory(archive, payload)
                payload[MANIFEST] = patch_manifest(archive.read(MANIFEST), BUILD_CODE, VERSION)
            release_proof = package_one(base, release, payload, LOCAL_PACKAGE, cert, env, work, "release")
            require(common.digest(base) == BASE_SHA, "base changed during packaging")

            qa.TARGET = QA_ORIGIN
            main_before, wallet_before, old_origin = qa.read_base(release)
            qa_source = work / "qa-input.dll"; qa_source.write_bytes(main_before)
            inject = work / "qa-inject"; inject.mkdir()
            (inject / "Program.cs").write_text((ROOT / "tools/client_recycle_selection/QAInjector.cs").read_text())
            runtime = HOST / ".tools/native-batch"
            dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"
            project = inject / "Inject.csproj"
            project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
            dot = runtime / "dotnet/dotnet"
            run([dot, "build", project, "--verbosity", "quiet", "-p:NuGetAudit=false", "-p:UseSharedCompilation=false"], env, 180)
            qa_main = work / "qa-main.dll"
            run([dot, inject / "bin/Debug/net8.0/Inject.dll", qa_source, old_origin, QA_ORIGIN, qa_main], env, 180)
            qa_checks = qa.verify(main_before, qa_main.read_bytes(), old_origin, QA_ORIGIN)
            with zipfile.ZipFile(release) as archive:
                qa_manifest, manifest_changes, label = patch_package(archive.read(MANIFEST))
            qa_replacements = {MANAGED: qa_main.read_bytes(), WALLET: (json.dumps({"origin": QA_ORIGIN}, separators=(",", ":")) + "\n").encode(), MANIFEST: qa_manifest}
            qa_proof = package_one(release, qa_output, qa_replacements, MULTI_PACKAGE, cert, env, work, "qa")
            qa_proof.update({"qaOnly": True, "neverPromote": True, "httpOrigin": QA_ORIGIN,
                             "tcpContract": "isolated SCUserLogin supplies " + QA_TCP + "; device reverse owned by verifier",
                             "manifestChanges": manifest_changes, "inheritedApplicationLabel": label,
                             "originPatchProof": qa_checks})
            require(common.digest(release) == release_proof["binarySHA256"] and common.digest(base) == BASE_SHA, "frozen APK changed")
            for name, expected in proof["sourcePins"].items():
                require(common.digest(ROOT / name) == expected, "source changed while packaging: " + name)
            record = {"schema": "hyjs-trial-refresh82-packages-v1", "status": "SIGNED_ALIGNED_NOT_INSTALLED",
                      "baseAPKSHA256": BASE_SHA, "overlayManifestSHA256": common.digest(proof_path),
                      "originalEngineMethodsExact": 547, "managedScope": check, "javaClassInventory": java,
                      "release": {"path": str(release), **release_proof}, "qa": {"path": str(qa_output), **qa_proof},
                      "packageScriptSHA256": common.digest(Path(__file__)), "sourcePins": proof["sourcePins"]}
        record["ownedIntermediatesCleaned"] = not work.exists()
        (out / "manifest.json").write_text(json.dumps(record, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": record["status"], "releaseSHA256": release_proof["binarySHA256"],
                      "qaSHA256": qa_proof["binarySHA256"], "outputDirectory": str(out)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
