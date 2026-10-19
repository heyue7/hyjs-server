"""Package a verified native account/wallet DEX over an explicit signed APK.

The managed engine, IME, assets and all other ZIP records stay byte-identical.
This creates a candidate only; installation and current/rollback promotion are separate.
"""
import argparse
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.package_release import compressed_hash, copy_original_record, require
from tools.login_ui.build import dex_abi


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--overlay', type=Path, required=True)
    parser.add_argument('--output-apk', type=Path, required=True)
    parser.add_argument('--record', type=Path, required=True)
    args = parser.parse_args()
    base, overlay, output, record = [path.resolve() for path in [args.base_apk, args.overlay, args.output_apk, args.record]]
    require(output.is_relative_to(ROOT / 'apks/build') and output != base, 'Use an owned candidate output')
    require(not output.exists() and not record.exists(), 'Preserve existing candidate/evidence')
    require(common.digest(base) == args.base_sha256, 'Base APK changed')
    qualification = json.loads((overlay / 'manifest.json').read_text())
    require(qualification['baseAPKSHA256'] == args.base_sha256 and qualification['managedAssemblyByteExact'], 'Expected DEX-only qualification')
    require(qualification['inheritedJavaPublicProtectedABIExact'], 'Inherited Java ABI must remain present')
    sources = {ROOT / name: value for name, value in qualification['sourcePins'].items()}
    for path, value in sources.items():
        require(path.is_relative_to(ROOT / 'tools') and common.digest(path) == value, 'Source changed: ' + str(path))
    dex = (overlay / 'classes2.dex').read_bytes()
    dex_hash = hashlib.sha256(dex).hexdigest()
    require(dex_hash == qualification['outputDexSHA256'], 'Overlay DEX changed')
    sdk = Path.home() / 'Library/Android/sdk/build-tools/33.0.0'
    java = Path.home() / 'Library/Java/JavaVirtualMachines/openjdk-18.0.1.1/Contents/Home'
    env = dict(os.environ, JAVA_HOME=str(java), PATH=str(java / 'bin') + os.pathsep + os.environ.get('PATH', ''))

    def run(*command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], capture_output=True, text=True, env=env, timeout=180)
        require(result.returncode == 0, 'Packaging command failed: ' + str(command[0]))
        return result.stdout + result.stderr

    with (ROOT / 'build/.client-build.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        require(common.digest(base) == args.base_sha256 and not output.exists(), 'Base/output changed while waiting')
        protected = {p: common.digest(p) if p.exists() else None for p in [ROOT / 'apks/build/hyjs-local.apk', ROOT / 'apks/build/hyjs-rollback.apk']}
        certificate = common._certificate(run(sdk / 'apksigner', 'verify', '--print-certs', base))
        inherited, compressed, metadata = {}, {}, {}
        with tempfile.TemporaryDirectory(prefix='hyjs-account-wallet-', dir=ROOT / 'build') as temporary:
            temp = Path(temporary)
            unsigned, aligned, signed = [temp / name for name in ['unsigned.apk', 'aligned.apk', 'signed.apk']]
            with zipfile.ZipFile(base) as incoming, base.open('rb') as stream:
                require(len(incoming.namelist()) == len(set(incoming.namelist())), 'Duplicate base ZIP entries')
                require(common._hash_entry(incoming, 'classes2.dex') == qualification['inputDexSHA256'], 'DEX input mismatch')
                old_api, new_api = dex_abi(incoming.read('classes2.dex')), dex_abi(dex)
                require(all(cls in new_api and all(member in new_api[cls] for member in api) for cls, api in old_api.items()), 'Inherited Java API removed')
                with zipfile.ZipFile(unsigned, 'w') as outgoing:
                    outgoing.comment = incoming.comment
                    for info in incoming.infolist():
                        if common._signature(info.filename):
                            continue
                        if info.filename == 'classes2.dex':
                            outgoing.writestr(copy.copy(info), dex)
                            continue
                        inherited[info.filename] = common._hash_entry(incoming, info.filename)
                        compressed[info.filename] = compressed_hash(stream, info)
                        metadata[info.filename] = (info.compress_type, info.compress_size, info.file_size, info.CRC)
                        copy_original_record(stream, outgoing, info)
            run(sdk / 'zipalign', '-f', '-p', '4', unsigned, aligned)
            run(sdk / 'apksigner', 'sign', '--ks', ROOT / '.local-signing/debug.keystore', '--ks-pass', 'pass:android', '--key-pass', 'pass:android', '--out', signed, aligned)
            require(common._certificate(run(sdk / 'apksigner', 'verify', '--verbose', '--print-certs', signed)) == certificate, 'Signing identity changed')
            run(sdk / 'zipalign', '-c', '-p', '4', signed)
            with zipfile.ZipFile(signed) as actual, signed.open('rb') as stream:
                require({n for n in actual.namelist() if not common._signature(n)} == set(inherited) | {'classes2.dex'}, 'ZIP inventory changed')
                for name, expected in dict(inherited, **{'classes2.dex': dex_hash}).items():
                    require(common._hash_entry(actual, name) == expected, 'Entry changed: ' + name)
                for name, expected in compressed.items():
                    info = actual.getinfo(name)
                    require((info.compress_type, info.compress_size, info.file_size, info.CRC) == metadata[name] and compressed_hash(stream, info) == expected, 'Inherited compressed record changed')
            for path, value in sources.items():
                require(common.digest(path) == value, 'Source changed during packaging')
            for path, value in protected.items():
                require((common.digest(path) if path.exists() else None) == value, 'Current/rollback changed')
            result = dict(status='PASS_DEX_ONLY_SIGN_ALIGN_CANDIDATE', baseAPKSHA256=args.base_sha256,
                          binarySHA256=common.digest(signed), replacementEntries={'classes2.dex': dex_hash},
                          inheritedEntryCount=len(inherited), allOtherEntriesAndCompressedPayloadsExact=True,
                          managedAssemblyByteExact=True, originalEngineMethodsExact=547,
                          signatureVerified=True, zipalignVerified=True, signerCertificateSHA256=certificate,
                          outputAPK=str(output.relative_to(ROOT)), installed=False, promoted=False,
                          overlayQualification=qualification, packagerSourceSHA256=common.digest(Path(__file__)))
            signed.replace(output)
            sidecar = signed.with_suffix('.apk.idsig')
            if sidecar.exists():
                sidecar.replace(output.with_suffix('.apk.idsig'))
        result['ownedTemporaryFilesCleaned'] = not temp.exists()
        record.parent.mkdir(parents=True, exist_ok=True)
        record.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({key: result[key] for key in ['status', 'binarySHA256', 'outputAPK']}))


if __name__ == '__main__':
    main()
