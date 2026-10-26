"""Compose the independently verified UI and wall-clock overlays into one APK75."""
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
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--ui-overlay', type=Path, required=True)
    parser.add_argument('--clock-overlay', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--record', type=Path, required=True)
    parser.add_argument('--combined-overlay', type=Path, required=True)
    args = parser.parse_args()
    base = args.base_apk.resolve()
    output = args.output.resolve()
    record = args.record.resolve()
    combined = args.combined_overlay.resolve()
    assert combined.is_relative_to(ROOT / 'analysis/growth75') and not combined.exists()
    proofs = [json.loads((folder / 'manifest.json').read_text()) for folder in
              [args.ui_overlay, args.clock_overlay]]
    identity = sha(base)
    payloads, replacements, pins = {}, {}, {}
    for proof in proofs:
        assert proof['baseAPKSHA256'] == identity
        assert proof['originalEngineMethodsExact'] == 547
        assert proof['originalMetadataABIAndTokensPreserved']
        assert not set(payloads).intersection(proof['payloadFiles'])
        for name, value in proof['payloadFiles'].items():
            path = ROOT / value
            assert path.is_file() and sha(path) == proof['replacementEntries'][name]
            payloads[name] = value
            replacements[name] = proof['replacementEntries'][name]
        for name, digest in proof.get('sourcePins', {}).items():
            assert sha(ROOT / name) == digest, 'Source drift: ' + name
            pins[name] = digest
    assert len(payloads) == 8 and 'classes2.dex' in payloads
    assert proofs[1]['scope']['changedBytes'] == 1
    assert proofs[0]['inheritedJavaPublicProtectedABIExact']
    combined.mkdir(parents=True)
    proof = dict(schema='hyjs-growth75-composed-overlay-v1', baseAPKSHA256=identity,
                 payloadFiles=payloads, replacementEntries=replacements, sourcePins=pins,
                 originalEngineMethodsExact=547, originalMetadataABIAndTokensPreserved=True,
                 componentProofs=proofs)
    (combined / 'manifest.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
    previous = sys.argv
    try:
        sys.argv = [str(Path(packager.__file__)), '--base-apk', str(base), '--overlay', str(combined),
                    '--output', str(output), '--record', str(record), '--build-code', '2026102701',
                    '--version-name', '1.0.75']
        packager.main()
    finally:
        sys.argv = previous
    report = json.loads(record.read_text())
    with zipfile.ZipFile(output) as apk:
        for name, digest in proofs[0]['originalDexEntriesMustRemainExact'].items():
            assert hashlib.sha256(apk.read(name)).hexdigest() == digest
    report.update(status='SIGNED_ALIGNED_GROWTH75_CANDIDATE_NOT_INSTALLED',
                  allJavaByteExact=False, inheritedJavaPublicProtectedABIExact=True,
                  originalDexEntriesByteExact=proofs[0]['originalDexEntriesMustRemainExact'],
                  sourcePins={**pins, str(Path(__file__).relative_to(ROOT)): sha(Path(__file__))},
                  phoneVerified=False)
    record.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({key: report[key] for key in ['status', 'binarySHA256', 'outputAPK']}))


if __name__ == '__main__':
    main()
