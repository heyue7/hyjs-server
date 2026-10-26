"""Add the reviewed three-portrait cold-cache fix to a frozen formal-origin75 APK."""
import argparse
import hashlib
import json
from pathlib import Path
import sys
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_sprout73 import package as packager


def sha(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-proof', type=Path, required=True)
    parser.add_argument('--base-proof-sha256', required=True)
    parser.add_argument('--overlay', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--record', type=Path, required=True)
    args = parser.parse_args()
    base = args.base_apk.resolve()
    proof_file = args.base_proof.resolve()
    overlay_file = args.overlay.resolve() / 'manifest.json'
    assert sha(proof_file) == args.base_proof_sha256
    parent = json.loads(proof_file.read_text())
    proof = json.loads(overlay_file.read_text())
    identity = sha(base)
    assert parent['binarySHA256'] == identity == proof['baseAPKSHA256']
    assert parent['buildCode'] == 2026102701 and parent['versionName'] == '1.0.75'
    assert proof['schema'] == 'hyjs-growth75-pet-cold-overlay-v1'
    assert proof['status'] == 'STATIC_VERIFIED_NOT_PACKAGED'
    assert proof['originalEngineMethodsExact'] == 547
    assert proof['originalMetadataABIAndTokensPreserved']
    assert proof['allOtherOriginalBodiesByteExact']
    assert proof['inheritedJavaByteExact'] and proof['allAssetsByteExact']
    assert proof['userCacheFilesUntouched'] and len(proof['allowedChangedMethods']) == 3
    assert set(proof['payloadFiles']) == {
        'assets/bin/Data/Managed/Assembly-CSharp.dll',
        'assets/bin/Data/Managed/SoulFlowerGGOShop.dll'}
    pins = proof['sourcePins']
    assert all(sha(ROOT / name) == digest for name, digest in pins.items())
    with zipfile.ZipFile(base) as apk:
        assert hashlib.sha256(apk.read('assets/bin/Data/Managed/Assembly-CSharp.dll')).hexdigest() == proof['inputManagedSHA256']
        assert json.loads(apk.read('assets/local-wallet.json'))['origin'] == 'http://134.175.127.249:28861'
        dex = {name: hashlib.sha256(apk.read(name)).hexdigest() for name in apk.namelist() if name.endswith('.dex')}
    overlay_sha = sha(overlay_file)
    previous = sys.argv
    try:
        sys.argv = [str(Path(packager.__file__)), '--base-apk', str(base), '--overlay', str(args.overlay.resolve()),
                    '--output', str(args.output.resolve()), '--record', str(args.record.resolve()),
                    '--build-code', '2026102702', '--version-name', '1.0.75']
        packager.main()
    finally:
        sys.argv = previous
    assert sha(overlay_file) == overlay_sha and sha(proof_file) == args.base_proof_sha256
    assert all(sha(ROOT / name) == digest for name, digest in pins.items())
    with zipfile.ZipFile(args.output) as apk:
        assert all(hashlib.sha256(apk.read(name)).hexdigest() == digest for name, digest in dex.items())
        assert json.loads(apk.read('assets/local-wallet.json'))['origin'] == 'http://134.175.127.249:28861'
    report = json.loads(args.record.read_text())
    report.update(status='SIGNED_ALIGNED_GROWTH75_COLD_CACHE_CANDIDATE_NOT_INSTALLED',
                  formalProductionOrigin=True, phoneVerified=False,
                  baseProof=str(proof_file), baseProofSHA256=args.base_proof_sha256,
                  overlayManifestSHA256=overlay_sha, inheritedJavaByteExact=True,
                  originalDexEntriesByteExact=dex,
                  inheritedHTTPUTCBodyExact=proof['inheritedHTTPUTCBodyExact'],
                  sourcePins={**report['sourcePins'], **pins,
                              str(Path(__file__).relative_to(ROOT)): sha(Path(__file__))})
    args.record.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({key: report[key] for key in ['status', 'binarySHA256', 'outputAPK']}))


if __name__ == '__main__':
    main()
