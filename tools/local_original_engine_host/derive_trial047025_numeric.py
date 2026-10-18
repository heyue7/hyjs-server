"""Derive strict Go comparison fixtures from explicit LOCAL original-Engine logs.

This is neither a device collector nor a native capability certificate. The
capture-like envelope exists only for the unchanged four numerical extractors.
"""
import argparse
import gzip
import hashlib
import json
from pathlib import Path
import subprocess
import sys
from trial047025_numeric import ROOT, SOURCE, sha


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run-manifest', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    if output.exists() or not output.is_relative_to(ROOT / 'analysis/p6'):
        raise ValueError('New owned analysis/p6 output only')
    run_path = args.run_manifest.resolve()
    run = json.loads(run_path.read_bytes())
    source = json.loads(SOURCE.read_bytes())
    if (run['caseSet'] != 'trial047025-numeric' or run['status'] != 'EXECUTED_PENDING_STRICT_COMPARE'
            or run['nativeCompared'] or run['authoritative']
            or run['caseSets'][0]['sourceManifestSHA256'] != sha(SOURCE)):
        raise ValueError('Exact completed LOCAL run required')
    output.mkdir()
    captures = []
    summary = []
    outputs = {}
    for row, observed in zip(source['cases'], run['caseSets'][0]['cases'], strict=True):
        raw = (run_path.parent / 'runtime' / observed['localWire']).with_suffix('.native.jsonl')
        if sha(raw) != run['outputPins'][str(raw.relative_to(run_path.parent))]:
            raise ValueError('Local trace SHA changed')
        with raw.open('rb') as src, gzip.GzipFile(filename=str(output / (raw.name + '.gz')), mode='wb', mtime=0) as dst:
            for chunk in iter(lambda: src.read(1024 * 1024), b''):
                dst.write(chunk)
        result = terminal = None
        count = stacks = 0
        for line in raw.open():
            event = json.loads(line)
            count += 1
            if event['kind'] == 'result':
                result = event
            if event['kind'] == 'state':
                terminal = event['state']
            if event['kind'] == 'ai_notify' and event.get('data', {}).get('stack'):
                stacks += 1
        if not result or not terminal:
            raise ValueError('Incomplete local Engine terminal')
        summary.append({'case': row['case'], 'attackerWon': result['attackerWon'],
                        'round': terminal['round'], 'events': count, 'platformStackEventsRetained': stacks,
                        'terminalHP': [[a['attribute']['hp'] for a in terminal[c]['actors']] for c in ['attacker', 'defender']]})
        captures.append({**row, 'batchWire': str(SOURCE.parent / row['wire']),
                         'nativeResult': raw.name, 'nativeSHA256': sha(raw), 'bytes': raw.stat().st_size})
        outputs[raw.name] = {'sha256': sha(raw), 'bytes': raw.stat().st_size}
    envelope = {'schema': 'local-original-engine-observations-not-phone-v1',
                'sourceManifest': str(SOURCE), 'sourceManifestSHA256': sha(SOURCE),
                'executionOrigin': 'local-coreclr-original-engine-not-phone-capture',
                'hostManifest': str(run_path.relative_to(ROOT)), 'hostManifestSHA256': sha(run_path),
                'nativeCompared': False, 'authoritative': False, 'cases': captures, 'outputs': outputs}
    capture_path = output / 'manifest.json'
    capture_path.write_text(json.dumps(envelope, ensure_ascii=False, indent=2) + '\n')
    fixtures = {}
    extraction = [('core', 'extract_native_pvp_golden.py', ['--details', '--attributes', '--buff-metadata']),
                  ('hits', 'extract_native_hit_exports.py', []),
                  ('manager', 'extract_native_manager_flags.py', []),
                  ('records', 'extract_native_original_operates.py', ['--terminal-hp'])]
    for kind, script, flags in extraction:
        target = output / (kind + '.json')
        operands = ([str(target.relative_to(ROOT)), '--batch', str(output.relative_to(ROOT))]
                    if kind == 'manager' else [str(output.relative_to(ROOT)), str(target.relative_to(ROOT))])
        subprocess.run([sys.executable, str(ROOT / 'analysis/p6' / script), *operands,
                        '--native-scope', 'original-pve-manual-then-auto-ai', *flags], cwd=ROOT, check=True)
        fixtures[kind] = {'path': str(target.relative_to(ROOT / 'analysis/p6')), 'sha256': sha(target)}
    bundle = {'schema': 'p7-numeric047025-local-matrix-v1', 'sourceManifest': str(SOURCE.relative_to(ROOT)),
              'sourceManifestSHA256': sha(SOURCE), 'captureManifest': str(capture_path.relative_to(ROOT)),
              'captureManifestSHA256': sha(capture_path), 'caseCount': 4,
              'nativeScope': 'original-pve-manual-then-auto-ai', 'fixtures': fixtures,
              'executionOrigin': envelope['executionOrigin'], 'businessAuthority': False,
              'nativeCompared': False, 'fullMatrixCompared': False, 'runnerSHA256': sha(Path(__file__))}
    bundle_path = output / 'bundle.json'
    bundle_path.write_text(json.dumps(bundle, ensure_ascii=False, indent=2) + '\n')
    report = {'status': 'LOCAL_FIXTURES_READY_GO_13_LAYERS_NOT_RUN_PHONE_ANCHOR_PENDING',
              'bundleSHA256': sha(bundle_path), 'bundle': str(bundle_path.relative_to(ROOT)),
              'sourceManifestSHA256': sha(SOURCE), 'hostSeconds': run['executionSeconds'],
              'cases': summary, 'numericTolerance': 0, 'platformStackRemoved': False,
              'fullPhoneSchemaStatus': 'NOT_COMPARED_NO_PHONE_ANCHOR',
              'businessAuthority': False, 'nativeCompared': False,
              'environment': {'HYJS_P7_NUMERIC_LOCAL_BUNDLE': str(bundle_path),
                              'HYJS_P7_NUMERIC_LOCAL_BUNDLE_SHA256': sha(bundle_path)},
              'testSelector': '^TestP7NumericLocal(RushNativeAllLayers|BattlePlayerProjection)$'}
    (output / 'comparison-preparation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False))


if __name__ == '__main__':
    main()
