"""Store the verified501 ZIP entries without compression; preserve all payloads.

Old Android Mono lacks DeflateStream's CreateZStream native implementation.
The existing verifier already accepts stored ZIP entries. This packaging-only
repair avoids that native dependency without changing its strict SHA checks.
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
import time
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.build_wallet_client import (DLL, _certificate, _hash_entry, _signature,
    digest, run, verify_wallet_upgrade_base)
from tools.local_asset_ready.build import SCOPE, SCOPE_SHA, verify_scope_apk


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--base-proof', type=Path, required=True)
    parser.add_argument('--record', type=Path, required=True)
    args = parser.parse_args()
    current = ROOT / 'build/hyjs-local.apk'
    rollback = ROOT / 'build/hyjs-rollback.apk'
    record, proof_path = args.record.resolve(), args.base_proof.resolve()
    if (not record.is_relative_to(ROOT / 'analysis') or record.exists()
            or not proof_path.is_relative_to(ROOT / 'analysis')):
        raise ValueError('Use the qualified local proof and a new analysis record')
    proof = json.loads(proof_path.read_bytes())
    if digest(SCOPE) != SCOPE_SHA:
        raise ValueError('Frozen resource closure changed')
    scope = json.loads(SCOPE.read_bytes())
    selected = {row['entry'] for row in scope['resources']}
    sdk = Path(os.environ.get('ANDROID_SDK_ROOT', Path.home() / 'Library/Android/sdk'))
    android = sdk / 'build-tools/33.0.0'
    java = Path.home() / 'Library/Java/JavaVirtualMachines/openjdk-18.0.1.1/Contents/Home'
    key = ROOT / '.local-signing/debug.keystore'
    for required in (android / 'zipalign', android / 'apksigner', java / 'bin/java', key):
        if not required.is_file():
            raise ValueError('Existing signing tool missing: ' + str(required))
    env = dict(os.environ, JAVA_HOME=str(java), PATH=str(java / 'bin') + os.pathsep + os.environ.get('PATH', ''))
    report = dict(status='PENDING', mode='verified501-stored-zip-packaging-only',
        baseSHA256=args.base_sha256, baseWalletProof=str(proof_path.relative_to(ROOT)),
        baseWalletProofSHA256=digest(proof_path), scopeSHA256=SCOPE_SHA,
        sourceSHA256=digest(Path(__file__)), installed=False, phoneUI=False,
        globalFullFlagChanged=False, originalResourceLoaderChanged=False,
        playerStateOrBattleResultChanged=False, managedAssemblyByteIdenticalToBase=True)
    began, temporary = time.monotonic(), None
    with (ROOT / 'build/.client-build.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        try:
            if digest(current) != args.base_sha256:
                raise ValueError('Current APK changed before packaging')
            if shutil.disk_usage(current.parent).free < current.stat().st_size * 3 + 250 * 1024 * 1024:
                raise ValueError('Insufficient free disk for signed intermediates')
            assembly, inventory = verify_scope_apk(current, scope, existing_bridge=True)
            certificate = _certificate(run([android / 'apksigner', 'verify', '--print-certs', current], env))
            if certificate != proof['signerCertificateSHA256']:
                raise ValueError('Base signing identity changed')
            inherited, changed = {}, []
            with tempfile.TemporaryDirectory(prefix='hyjs-stored501-', dir=ROOT / 'build') as directory:
                temporary = Path(directory)
                unsigned, aligned, signed = [temporary / n for n in ('unsigned.apk', 'aligned.apk', 'signed.apk')]
                with zipfile.ZipFile(current) as incoming:
                    qualified = verify_wallet_upgrade_base(incoming, proof, args.base_sha256)
                    with zipfile.ZipFile(unsigned, 'w') as outgoing:
                        outgoing.comment = incoming.comment
                        for info in incoming.infolist():
                            if _signature(info.filename):
                                continue
                            target = copy.copy(info)
                            if info.filename in selected:
                                target.compress_type = zipfile.ZIP_STORED
                                if info.compress_type != target.compress_type:
                                    changed.append(info.filename)
                            hasher = hashlib.sha256()
                            with incoming.open(info) as source, outgoing.open(target, 'w') as destination:
                                while block := source.read(1024 * 1024):
                                    hasher.update(block)
                                    destination.write(block)
                            inherited[info.filename] = hasher.hexdigest()
                if not changed or len(selected) != 2680:
                    raise ValueError('Expected the original scoped compressed entries')
                run([android / 'zipalign', '-f', '-p', '4', unsigned, aligned], env)
                run([android / 'apksigner', 'sign', '--ks', key, '--ks-pass', 'pass:android',
                     '--key-pass', 'pass:android', '--out', signed, aligned], env)
                if _certificate(run([android / 'apksigner', 'verify', '--verbose', '--print-certs', signed], env)) != certificate:
                    raise ValueError('Signed APK identity differs')
                run([android / 'zipalign', '-c', '-p', '4', signed], env)
                with zipfile.ZipFile(signed) as checked:
                    names = checked.namelist()
                    if len(names) != len(set(names)) or {n for n in names if not _signature(n)} != set(inherited):
                        raise ValueError('Signed entry set changed')
                    for name, expected in inherited.items():
                        if _hash_entry(checked, name) != expected:
                            raise ValueError('APK payload changed: ' + name)
                        expected_method = zipfile.ZIP_STORED if name in selected else None
                        if expected_method is not None and checked.getinfo(name).compress_type != expected_method:
                            raise ValueError('Scoped resource still needs native ZIP inflation')
                    verify_wallet_upgrade_base(checked, proof, args.base_sha256)
                final_assembly, final_inventory = verify_scope_apk(signed, scope, existing_bridge=True)
                if final_assembly != assembly or final_inventory != inventory:
                    raise ValueError('Original resource or managed evidence changed')
                if digest(current) != args.base_sha256 or digest(proof_path) != report['baseWalletProofSHA256']:
                    raise ValueError('Base APK or proof changed before promotion')
                # Keep the ordinary proof shape usable by subsequent compositions.
                # Every managed and wallet entry is inherited byte for byte.
                for name in ('bridgeProof', 'manifestProof', 'modernIME', 'resourceOverlayProof',
                             'activityTitleOverlayProof', 'formationOverlayProof', 'originalClassesDexSHA256'):
                    if name in proof:
                        report[name] = proof[name]
                report.update(qualifiedWalletEntries=qualified, replacementEntries={},
                    baseClassesDexByteIdentical=True, allOtherInheritedEntriesExact=True,
                    inheritedEntryCount=len(inherited),
                    inheritedEntrySHA256=hashlib.sha256(json.dumps(inherited, sort_keys=True, separators=(',', ':')).encode()).hexdigest(),
                    storedClosureEntries=len(selected), compressionOnlyChangedEntries=changed,
                    allNonSignatureEntryPayloadsByteIdentical=True,
                    originalManagedSHA256=hashlib.sha256(assembly).hexdigest(),
                    classesDexSHA256=qualified.get('classes.dex'), modernIMEInheritedFromBase=True,
                    outputAPK=str(current.relative_to(ROOT)), binarySHA256=digest(signed),
                    signerCertificateSHA256=certificate, signatureVerified=True, zipalignVerified=True,
                    packageSizeBytes=signed.stat().st_size,
                    packageSizeDeltaBytes=signed.stat().st_size-current.stat().st_size)
                current.replace(rollback)
                try:
                    signed.replace(current)
                except BaseException:
                    if not current.exists():
                        rollback.replace(current)
                    raise
                idsig = signed.with_suffix('.apk.idsig')
                if idsig.exists():
                    idsig.replace(current.with_suffix('.apk.idsig'))
                else:
                    current.with_suffix('.apk.idsig').unlink(missing_ok=True)
                rollback.with_suffix('.apk.idsig').unlink(missing_ok=True)
                report.update(status='PASS_BUILD_VERIFY_PROMOTED_NOT_INSTALLED',
                              rollbackAPK=str(rollback.relative_to(ROOT)), rollbackSHA256=digest(rollback))
        except BaseException as error:
            report.update(status='FAILED_NOT_QUALIFIED', errorType=type(error).__name__, error=str(error)[-4000:])
            raise
        finally:
            report.update(ownedTemporaryFilesCleaned=temporary is None or not temporary.exists(),
                          seconds=round(time.monotonic()-began, 3))
            record.parent.mkdir(parents=True, exist_ok=True)
            record.write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps({k: report[k] for k in ('status', 'binarySHA256', 'storedClosureEntries', 'packageSizeDeltaBytes', 'seconds')}))


if __name__ == '__main__':
    main()
