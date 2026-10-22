"""Combine scoped GGO resources and native-shop hooks into a signed APK.

All inherited ZIP compressed records and all original methods outside three
declared UI hooks remain byte exact. Android and the existing update helper read
the increased version from PackageManager, so no inherited Java is rebuilt.
"""
import argparse
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.package_release import compressed_hash, copy_original_record, require
from tools.client_update.package import patch_manifest, verify_managed


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for key in ['base-apk', 'assets', 'client', 'output', 'record']:
        parser.add_argument('--' + key, required=True, type=Path)
    parser.add_argument('--build-code', type=int, required=True)
    parser.add_argument('--version-name', required=True)
    args = parser.parse_args()
    base, output, record = [p.resolve() for p in [args.base_apk, args.output, args.record]]
    require(output.is_relative_to(ROOT / 'build') and record.is_relative_to(ROOT / 'analysis'), 'Use owned outputs')
    require(not output.exists() and not record.exists(), 'Fresh outputs required')
    asset = json.loads((args.assets / 'manifest.json').read_text())
    client = json.loads((args.client / 'manifest.json').read_text())
    base_sha = common.digest(base)
    require(base_sha == asset['baseAPKSHA256'] == client['baseAPKSHA256'], 'Overlay base differs')
    require(asset['schema'] == 'hyjs-ggo-image-assets-v1' and client['schema'] == 'hyjs-ggo-shop-overlay-v1', 'Overlay schema differs')
    pins = {ROOT / name: digest for name, digest in client['sourcePins'].items()}
    pins[Path(__file__)] = common.digest(Path(__file__))
    for path, digest in pins.items():
        require(common.digest(path) == digest, 'Build source changed: ' + str(path))
    payload = {}
    for proof, directory in [(asset, args.assets.resolve()), (client, args.client.resolve())]:
        expected = ({row['entry']: row['sha256'] for row in proof['files']}
                    if proof is asset else proof['replacementEntries'])
        require(set(expected) == set(proof['payloadFiles']), 'Incomplete overlay inventory')
        for name, path in proof['payloadFiles'].items():
            file = Path(path).resolve()
            require(file.is_relative_to(directory), 'Payload escaped qualified directory')
            require(name not in payload, 'Overlapping entry: ' + name)
            raw = file.read_bytes()
            require(hashlib.sha256(raw).hexdigest() == expected[name], 'Overlay digest changed: ' + name)
            payload[name] = raw
    with zipfile.ZipFile(base) as incoming:
        require(common._hash_entry(incoming, common.DLL) == client['inputManagedSHA256'], 'Managed base changed')
        verify_managed(incoming.read(common.DLL), payload[common.DLL], [row['method'] for row in client['allowedChangedMethods']])
        payload['AndroidManifest.xml'] = patch_manifest(incoming.read('AndroidManifest.xml'), args.build_code, args.version_name)
    sdk = Path.home() / 'Library/Android/sdk/build-tools/33.0.0'
    jdk = Path('/opt/homebrew/opt/openjdk@21')
    environment = dict(os.environ, JAVA_HOME=str(jdk), PATH=str(jdk / 'bin') + os.pathsep + os.environ.get('PATH', ''))
    require(shutil.disk_usage(ROOT).free > 2 * base.stat().st_size + 150_000_000, 'Insufficient packaging space')
    certificate = common._certificate(common.run([sdk / 'apksigner', 'verify', '--print-certs', base], environment))
    output.parent.mkdir(parents=True, exist_ok=True)
    inherited, compressed = {}, {}
    with (ROOT / 'build/.client-build.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        with tempfile.TemporaryDirectory(prefix='ggo-candidate-', dir=ROOT / 'build') as folder:
            temporary = Path(folder)
            unsigned, aligned, signed = [temporary / name for name in ['unsigned.apk', 'aligned.apk', 'signed.apk']]
            with zipfile.ZipFile(base) as incoming, base.open('rb') as stream, zipfile.ZipFile(unsigned, 'w') as target:
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
                for name in sorted(set(payload) - set(incoming.namelist())):
                    target.writestr(name, payload[name], compress_type=zipfile.ZIP_DEFLATED)
            common.run([sdk / 'zipalign', '-f', '-p', '4', unsigned, aligned], environment)
            unsigned.unlink()
            common.run([sdk / 'apksigner', 'sign', '--ks', ROOT / '.local-signing/debug.keystore', '--ks-pass', 'pass:android', '--key-pass', 'pass:android', '--out', signed, aligned], environment)
            aligned.unlink()
            require(common._certificate(common.run([sdk / 'apksigner', 'verify', '--verbose', '--print-certs', signed], environment)) == certificate, 'Signing identity changed')
            common.run([sdk / 'zipalign', '-c', '-p', '4', signed], environment)
            hashes = {name: hashlib.sha256(raw).hexdigest() for name, raw in payload.items()}
            with zipfile.ZipFile(signed) as verified, signed.open('rb') as stream:
                names = verified.namelist()
                require(len(names) == len(set(names)) and {name for name in names if not common._signature(name)} == set(inherited) | set(payload), 'APK inventory changed')
                for name, digest in {**inherited, **hashes}.items():
                    require(common._hash_entry(verified, name) == digest, 'APK digest differs: ' + name)
                for name, digest in compressed.items():
                    require(compressed_hash(stream, verified.getinfo(name)) == digest, 'Inherited compressed bytes changed: ' + name)
            for path, digest in pins.items():
                require(common.digest(path) == digest, 'Build input changed while packaging')
            require(common.digest(base) == base_sha, 'Base changed while packaging')
            result = dict(status='SIGNED_GGO_CANDIDATE_NOT_INSTALLED', baseAPKSHA256=base_sha,
                          binarySHA256=common.digest(signed), outputAPK=str(output.relative_to(ROOT)),
                          versionName=args.version_name, buildCode=args.build_code,
                          replacementEntries=hashes, originalEngineMethodsExact=547,
                          inheritedEntryCount=len(inherited), allOtherEntriesAndCompressedPayloadsExact=True,
                          allInheritedJavaExact=True, signerCertificateSHA256=certificate,
                          signatureVerified=True, zipalignVerified=True, installed=False, promoted=False)
            signed.replace(output)
    record.parent.mkdir(parents=True, exist_ok=True)
    record.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({key: result[key] for key in ['status', 'binarySHA256', 'versionName', 'buildCode', 'outputAPK']}))


if __name__ == '__main__':
    main()
