"""Package a qualified DEX-only wallet update without changing inherited APK payloads."""
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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--overlay', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--record', type=Path, required=True)
    args = parser.parse_args()
    base, overlay, candidate, record = [p.resolve() for p in (args.base_apk, args.overlay, args.output, args.record)]
    require(candidate.is_relative_to(ROOT / 'apks/build') and not candidate.exists() and not record.exists(), 'Use fresh project output and record paths')
    qualification = json.loads((overlay / 'manifest.json').read_text())
    require(common.digest(base) == args.base_sha256 == qualification['baseAPKSHA256'], 'Qualified base differs')
    require(qualification['managedAssemblyByteExact'] and qualification['inheritedJavaPublicProtectedABIExact'], 'DEX-only ABI qualification required')
    require(qualification['originalEngineMethodsExact'] == 547, 'Engine qualification differs')
    for name, expected in qualification['sourcePins'].items():
        source = (ROOT / name).resolve()
        require(source.is_relative_to(ROOT / 'tools') and common.digest(source) == expected, 'Qualified source changed')
    dex = (overlay / 'classes2.dex').read_bytes()
    require(hashlib.sha256(dex).hexdigest() == qualification['outputDexSHA256'], 'Qualified DEX changed')
    sdk = Path.home() / 'Library/Android/sdk/build-tools/33.0.0'
    java = Path('/opt/homebrew/opt/openjdk@21')
    env = dict(os.environ, JAVA_HOME=str(java), PATH=str(java / 'bin') + os.pathsep + os.environ.get('PATH', ''))
    key = ROOT / '.local-signing/debug.keystore'
    require(key.is_file(), 'Preserve existing signing identity')
    record.parent.mkdir(parents=True, exist_ok=True)
    with (ROOT / 'build/.client-build.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        protected = {p: common.digest(p) for p in [base, ROOT / 'apks/build/hyjs-local.apk', ROOT / 'apks/build/hyjs-rollback.apk'] if p.is_file()}
        require(common.digest(base) == args.base_sha256 and not candidate.exists(), 'Inputs changed while queued')
        certificate = common._certificate(common.run([sdk / 'apksigner', 'verify', '--print-certs', base], env))
        inherited, compressed = {}, {}
        with tempfile.TemporaryDirectory(prefix='hyjs-wallet-dex-', dir=ROOT / 'build') as temporary:
            folder = Path(temporary)
            unsigned, aligned, signed = [folder / n for n in ['unsigned.apk', 'aligned.apk', 'signed.apk']]
            with zipfile.ZipFile(base) as incoming, base.open('rb') as stream, zipfile.ZipFile(unsigned, 'w') as outgoing:
                require(len(incoming.namelist()) == len(set(incoming.namelist())), 'Duplicate source entries')
                require(common._hash_entry(incoming, common.DLL) == qualification['inputManagedSHA256'], 'Managed input differs')
                require(common._hash_entry(incoming, 'classes2.dex') == qualification['inputDexSHA256'], 'DEX input differs')
                outgoing.comment = incoming.comment
                for info in incoming.infolist():
                    if common._signature(info.filename):
                        continue
                    if info.filename == 'classes2.dex':
                        outgoing.writestr(copy.copy(info), dex)
                        continue
                    inherited[info.filename] = common._hash_entry(incoming, info.filename)
                    compressed[info.filename] = compressed_hash(stream, info)
                    copy_original_record(stream, outgoing, info)
            common.run([sdk / 'zipalign', '-f', '-p', '4', unsigned, aligned], env)
            common.run([sdk / 'apksigner', 'sign', '--ks', key, '--ks-pass', 'pass:android', '--key-pass', 'pass:android', '--out', signed, aligned], env)
            require(common._certificate(common.run([sdk / 'apksigner', 'verify', '--verbose', '--print-certs', signed], env)) == certificate, 'Signing identity changed')
            common.run([sdk / 'zipalign', '-c', '-p', '4', signed], env)
            with zipfile.ZipFile(signed) as actual, signed.open('rb') as stream:
                require(len(actual.namelist()) == len(set(actual.namelist())), 'Duplicate output entries')
                require({n for n in actual.namelist() if not common._signature(n)} == set(inherited) | {'classes2.dex'}, 'APK inventory changed')
                require(common._hash_entry(actual, 'classes2.dex') == qualification['outputDexSHA256'], 'Packaged DEX differs')
                for name, expected in inherited.items():
                    require(common._hash_entry(actual, name) == expected and compressed_hash(stream, actual.getinfo(name)) == compressed[name], 'Inherited payload changed: ' + name)
            for path, expected in protected.items():
                require(common.digest(path) == expected, 'Protected deliverable changed')
            result = {'status': 'SIGNED_CANDIDATE_NOT_INSTALLED', 'baseAPKSHA256': args.base_sha256, 'binarySHA256': common.digest(signed), 'outputAPK': str(candidate.relative_to(ROOT)), 'overlayManifest': str((overlay / 'manifest.json').relative_to(ROOT)), 'replacementEntries': {'classes2.dex': qualification['outputDexSHA256']}, 'signatureVerified': True, 'zipalignVerified': True, 'signerCertificateSHA256': certificate, 'inheritedEntryCount': len(inherited), 'inheritedCompressedPayloadsExact': True, 'managedAssemblyByteExact': True, 'originalEngineMethodsExact': 547, 'packagerSHA256': common.digest(Path(__file__))}
            signed.replace(candidate)
            idsig = signed.with_suffix('.apk.idsig')
            if idsig.exists():
                idsig.replace(candidate.with_suffix('.apk.idsig'))
        result['ownedTemporaryFilesCleaned'] = not folder.exists()
        with record.open('x') as stream:
            stream.write(json.dumps(result, indent=2) + '\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
