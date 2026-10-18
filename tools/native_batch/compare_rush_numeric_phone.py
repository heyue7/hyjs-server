"""Prepare or run strict four-case Android natural047tal1/025lv3 comparison; no device or DB access.

Default --preflight only validates frozen inputs. After collect.py, --capture and
--output derive the existing numerical/export/manager/final fixtures. An explicit
--service-test may run an already-built service.test; this tool never builds Go.
"""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
sys.path.insert(0, str(ROOT / 'analysis/p6'))
from server.codec import message
from tools.native_batch.prepare import load_rush_typed_source
from tools.native_batch.collect import validate_typed_execution, validate_typed_preflight
from native_evidence import open_native

SOURCE = ROOT / 'analysis/p6/p7-numeric047025-local-inputs-r1-20261017/manifest.json'
SOURCE_SHA = '7e74d64e647ee3bc0de9f48afd10cea8bae14ea14461652342d06b88ab8eceac'
SCOPE = 'original-pve-manual-then-auto-ai'


def sha(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def preflight():
    if sha(SOURCE) != SOURCE_SHA:
        raise RuntimeError('Pinned four-case P7 input manifest changed')
    source = json.loads(SOURCE.read_bytes())
    if len(source['cases']) != 4 or source.get('nativeCompared') or source.get('authoritative'):
        raise RuntimeError('Diagnostic denominator or authority changed')
    for case in source['cases']:
        load_rush_typed_source(case, SOURCE, 'highest')
        if sha(SOURCE.parent / case['wire']) != case['wireSHA256']:
            raise RuntimeError('Original wire SHA changed')
        if any(case.get(key) for key in ('originalEnd', 'originalRecordSHA256', 'expectedWinner', 'result')):
            raise RuntimeError('Native input must not carry result hints')
    return source


def validate_capture(capture_path, source):
    capture = json.loads(capture_path.read_bytes())
    if sha(capture_path) != 'd48077b4f44aaea01bafe09ca004d0aa8575e15b867e9d4d35c7ffa4aaaa08ff':
        raise RuntimeError('Pinned Android original capture manifest changed')
    if (Path(capture['sourceManifest']).resolve() != SOURCE or
            capture.get('sourceManifestSHA256') != SOURCE_SHA or capture.get('selector') != 'highest' or
            len(capture.get('cases', [])) != 4):
        raise RuntimeError('Capture is not the exact four-case highest batch')
    captured = {case['wireSHA256']: case for case in capture['cases']}
    if len(captured) != 4 or set(captured) != {case['wireSHA256'] for case in source['cases']}:
        raise RuntimeError('Captured case denominator or identity changed')
    for case in source['cases']:
        row = captured[case['wireSHA256']]
        for key, expected in case.items():
            if key not in ('status', 'nativeScope') and row.get(key) != expected:
                raise RuntimeError('Collected source metadata differs: ' + key)
        if row.get('nativeScope') != SCOPE:
            raise RuntimeError('Original execution scope changed')
        wire = message('SCFightPlayback')
        wire.ParseFromString((SOURCE.parent / case['wire']).read_bytes())
        validate_typed_preflight(row, wire, 'highest', row['batchWire'])
        name = row.get('nativeResult', '')
        if Path(name).name != name or not name.endswith('.native.jsonl'):
            raise RuntimeError('Invalid captured native filename')
        digest = hashlib.sha256()
        first = last = None
        record_count = 0
        with open_native(capture_path.parent / name) as stream:
            for line in stream:
                digest.update(line)
                event = json.loads(line)
                first = first or event
                last = event
                if event.get('kind') == 'original_final_operates':
                    record_count += 1
        if (not first or first.get('kind') != 'input' or first.get('scope') != SCOPE or
                (first.get('fightId'), first.get('seed'), first.get('battleType')) !=
                (case['fightId'], case['seed'], 9) or not last or last.get('kind') != 'result' or
                type(last.get('attackerWon')) is not bool or record_count != 1 or
                digest.hexdigest() != row.get('nativeSHA256')):
            raise RuntimeError('Incomplete, changed or wrong original native output')
        validate_typed_execution(row, first)
    return capture


def preserve_records(records_path, output, source):
    records = json.loads(records_path.read_bytes())
    originals = {row['wireSHA256']: row for row in records['cases']}
    if len(originals) != 4 or set(originals) != {case['wireSHA256'] for case in source['cases']}:
        raise RuntimeError('Original final-record denominator differs')
    artifacts = []
    for index, case in enumerate(source['cases']):
        row = originals[case['wireSHA256']]
        record = row['originalFinalRecord']
        raw = base64.b64decode(record['protobufBase64'], validate=True)
        if hashlib.sha256(raw).hexdigest() != record['protobufSHA256']:
            raise RuntimeError('Original protobuf-net record SHA differs')
        decoded = message('BattleOperate')
        decoded.ParseFromString(raw)
        if not decoded.IsInitialized() or decoded.finalRoundNum < 1:
            raise RuntimeError('Original record is incomplete')
        native_path = output / f'{index:03d}.original.BattleOperate.pb'
        native_path.write_bytes(raw)
        # Headless diagnostics never send a business End. Preserve its original
        # record bytes separately and identify this convenient typed wrapper as derived.
        end = message('CSRushFightEnd', fightId=case['fightId'],
                      result=1 if row['attackerWon'] else 0, optRecord=decoded)
        derived_path = output / f'{index:03d}.derived.CSRushFightEnd.pb'
        derived_path.write_bytes(end.SerializeToString(deterministic=True))
        artifacts.append({'case': case['case'], 'wireSHA256': case['wireSHA256'],
                          'originalRecord': native_path.name, 'originalRecordSHA256': sha(native_path),
                          'derivedTypedEnd': derived_path.name, 'derivedTypedEndSHA256': sha(derived_path),
                          'typedEndProvenance': 'DERIVED_ENVELOPE_FROM_ORIGINAL_RECORD_NOT_NETWORK_CAPTURE'})
    return artifacts


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--preflight', action='store_true')
    parser.add_argument('--capture', type=Path)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--service-test', type=Path, help='Explicit prebuilt test binary; never Go compilation')
    args = parser.parse_args()
    source = preflight()
    if args.preflight or not args.capture:
        if args.output or args.service_test or args.capture:
            raise RuntimeError('--preflight is read-only and cannot also derive/run comparisons')
        print(json.dumps({'status': 'INPUT_PREFLIGHT_PASS', 'cases': 4, 'sourceSHA256': SOURCE_SHA,
                          'inputHasResults': False, 'deviceAccess': False, 'databaseAccess': False}))
        return
    capture = args.capture.resolve() / 'manifest.json'
    if not capture.is_relative_to(ROOT / 'analysis/p6'):
        raise RuntimeError('Use the owned original capture under analysis/p6')
    validate_capture(capture, source)
    output = args.output.resolve() if args.output else None
    if not output or not output.is_relative_to(ROOT / 'analysis/p6') or output.exists():
        raise RuntimeError('Use a new immutable output directory under analysis/p6')
    output.mkdir(mode=0o700)
    fixtures = {}
    extraction = [
        ('core', 'extract_native_pvp_golden.py', ['--details', '--attributes', '--buff-metadata']),
        ('hits', 'extract_native_hit_exports.py', []),
        ('manager', 'extract_native_manager_flags.py', []),
        ('records', 'extract_native_original_operates.py', ['--terminal-hp']),
    ]
    for key, script, flags in extraction:
        target = output / (key + '.json')
        operands = ([str(target.relative_to(ROOT)), '--batch', str(capture.parent.relative_to(ROOT))]
                    if key == 'manager' else [str(capture.parent.relative_to(ROOT)), str(target.relative_to(ROOT))])
        subprocess.run([sys.executable, str(ROOT / 'analysis/p6' / script), *operands,
                        '--native-scope', SCOPE, *flags], cwd=ROOT, check=True)
        fixtures[key] = {'path': str(target.relative_to(ROOT / 'analysis/p6')), 'sha256': sha(target)}
    bundle = {'schema': 'p7-numeric047025-android-native-matrix-v1', 'sourceManifest': str(SOURCE.relative_to(ROOT)),
              'sourceManifestSHA256': SOURCE_SHA, 'captureManifest': str(capture.relative_to(ROOT)),
              'captureManifestSHA256': sha(capture), 'caseCount': 4, 'nativeScope': SCOPE,
              'fixtures': fixtures, 'records': preserve_records(output / 'records.json', output, source),
              'businessAuthority': False, 'fullMatrixCompared': False,
              'executionOrigin': 'android-original-engine-native-batch',
              'fullSchemaComparison': {'path': 'analysis/growth-first-20261016/p6-trial047025-numeric-android-handoff-r1-20261017/phone-vs-host-comparison-r1.json', 'sha256': 'b0b07ee1fd468878dfc4c26f904a6b9075a7a3ab077dfe3bf3ed0713adcc7903', 'status': 'FAIL', 'difference': 'ai_notify.data.stack'},
              'runnerSHA256': sha(Path(__file__).resolve())}
    bundle_path = output / 'bundle.json'
    bundle_path.write_text(json.dumps(bundle, indent=2) + '\n')
    env = {'HYJS_P7_NUMERIC_PHONE_BUNDLE': str(bundle_path),
           'HYJS_P7_NUMERIC_PHONE_BUNDLE_SHA256': sha(bundle_path)}
    report = {'status': 'FIXTURES_READY_COMPARISON_PENDING', 'bundle': str(bundle_path.relative_to(ROOT)),
              'bundleSHA256': sha(bundle_path), 'environment': env, 'cases': 4,
              'requiredTest': '^TestP7NumericPhoneRushNativeAllLayers$', 'businessAuthority': False}
    if args.service_test:
        binary = args.service_test.resolve()
        if not binary.is_relative_to(ROOT / 'build') or not binary.is_file():
            raise RuntimeError('Explicit existing owned build test binary required')
        report['testBinarySHA256'] = sha(binary)
        log = output / 'comparison.log'
        with log.open('x') as sink:
            run = subprocess.run([str(binary), '-test.run=^TestP7NumericPhoneRushNativeAllLayers$', '-test.v'],
                                 cwd=ROOT / 'hyjs-server/service', env={**os.environ, **env},
                                 stdout=sink, stderr=subprocess.STDOUT)
        report['testExitCode'] = run.returncode
        report['comparisonLogSHA256'] = sha(log)
        log_text = log.read_text()
        if run.returncode == 0 and '--- PASS: TestP7NumericPhoneRushNativeAllLayers ' in log_text and '--- SKIP:' not in log_text:
            report['status'] = 'FOUR_CASE_FULL_MATRIX_PASS_CAPABILITY_PENDING'
        else:
            report['status'] = 'FULL_MATRIX_FAILED_OR_NOT_EXECUTED'
    (output / 'comparison-report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    if report['status'] == 'FULL_MATRIX_FAILED_OR_NOT_EXECUTED':
        raise SystemExit(1)


if __name__ == '__main__':
    main()
