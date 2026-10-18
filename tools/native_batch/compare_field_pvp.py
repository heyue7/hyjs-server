"""Prepare or run strict four-case type5 field002 player diagnostic comparison; no device or DB access.

Default --preflight only validates frozen inputs. After collect.py, --capture and
--output derive the existing numerical/export/manager/final fixtures. An explicit
--service-test may run an already-built service.test; this tool never builds Go.
"""
import argparse
import base64
import hashlib
import json
import math
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
sys.path.insert(0, str(ROOT / 'analysis/p6'))
from server.codec import message
from tools.native_batch.collect import validate_typed_execution, validate_typed_preflight
from native_evidence import open_native

SOURCE = ROOT / 'analysis/p6/field-pvp5-actual-actors-pending-20261016/manifest.json'
SOURCE_SHA = 'd705ccbf9ea8c1b226a229becbc5bfbe67117e1a4b7ff5da2a21d95c75cc8ece'
SCOPE = 'original-field-pvp5-field002-automatic-ai'
REQUIRED_LAYERS = ('core', 'actor44', 'lifetimes', 'Buffmetadata', 'manager',
                   'HP-AP-combo-control', 'cast-values-order', 'cast-complete-collection',
                   'cast-loads-dead', 'Buff-HP-AP', 'Buff-internal-order',
                   'standalone-full-collection', 'final-protocol-bytes-terminal-HP')


def sha(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def preflight():
    if sha(SOURCE) != SOURCE_SHA:
        raise RuntimeError('Pinned independent typed5 source changed')
    source = json.loads(SOURCE.read_bytes())
    if len(source['cases']) != 4 or source.get('nativeCompared') or source.get('authoritative'):
        raise RuntimeError('Independent type5 denominator or authority changed')
    for path, expected in source['sourcePins'].items():
        if sha(ROOT / path) != expected:
            raise RuntimeError('Pinned actual actor provenance changed: ' + path)
    for index, case in enumerate(source['cases']):
        for key, hashkey in [('wire', 'wireSHA256'), ('typedCallback', 'typedCallbackSHA256')]:
            if Path(case[key]).name != case[key] or sha(SOURCE.parent / case[key]) != case[hashkey]:
                raise RuntimeError('Exact typed5 artifact SHA changed')
        wire = message('SCFightPlayback'); wire.ParseFromString((SOURCE.parent / case['wire']).read_bytes())
        typed = message('SidePvpFightData'); typed.ParseFromString((SOURCE.parent / case['typedCallback']).read_bytes())
        if (case['seed'] != (7, 314159, 7, 314159)[index] or case['battleType'] != 5
                or case['nativeScope'] != SCOPE or case['fieldContext'] != 'field_002'
                or wire.battleType != 5 or wire.fightid != case['fightId'] or typed.id != wire.fightid
                or typed.randSeed != wire.randSeed or wire.randSeed != case['seed']
                or typed.maxRoundCount != 40 or wire.maxRound != 40
                or wire.attackerFactor != 1 or wire.defenderFactor != 1 or wire.HasField('defender2')
                or wire.HasField('optRecord') or wire.winConditions
                or wire.attacker != typed.attPlayer or wire.defender1 != typed.defPlayer
                or len(wire.attacker.formation) != 1 or len(wire.defender1.formation) != 1
                or typed.HasField('winPlayerId') or typed.attGet or typed.defLose or typed.HasField('attFirstPlayerId')
                or any(case.get(key) for key in ('result', 'expectedWinner', 'originalEnd', 'originalRecordSHA256'))):
            raise RuntimeError('Independent typed5 original projection differs or contains result hints')
        if case['actorSHA256'] != [hashlib.sha256(a.SerializeToString(deterministic=True)).hexdigest() for a in (wire.attacker, wire.defender1)]:
            raise RuntimeError('Actual actor complete bytes changed')
    return source

def validate_capture(capture_path, source):
    capture = json.loads(capture_path.read_bytes())
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
                (case['fightId'], case['seed'], 5) or not last or last.get('kind') != 'result' or
                type(last.get('attackerWon')) is not bool or record_count != 1 or
                digest.hexdigest() != row.get('nativeSHA256')):
            raise RuntimeError('Incomplete, changed or wrong original native output')
        validate_typed_execution(row, first)
    return capture


def preserve_records(records_path, output, source):
    records = json.loads(records_path.read_bytes())
    originals = {row['wireSHA256']: row for row in records['cases']}
    if len(originals) != 4 or set(originals) != {case['wireSHA256'] for case in source['cases']}:
        raise RuntimeError('Original field PK terminal denominator differs')
    artifacts = []
    for index, case in enumerate(source['cases']):
        row = originals[case['wireSHA256']]; record = row['originalFinalRecord']
        raw = base64.b64decode(record['protobufBase64'], validate=True)
        if hashlib.sha256(raw).hexdigest() != record['protobufSHA256']:
            raise RuntimeError('Original record bytes SHA differs')
        decoded = message('BattleOperate'); decoded.ParseFromString(raw)
        if not decoded.IsInitialized() or decoded.finalRoundNum < 1 or type(row.get('attackerWon')) is not bool:
            raise RuntimeError('Native original record/result incomplete')
        native_path = output / f'{index:03}.original.BattleOperate.pb'; native_path.write_bytes(raw)
        terminal = row.get('originalFinalActorHP', [])
        typed = message('SidePvpFightData'); typed.ParseFromString((SOURCE.parent / case['typedCallback']).read_bytes())
        expected = [(0, a) for a in typed.attPlayer.formation] + [(1, a) for a in typed.defPlayer.formation]
        if len(terminal) != 2 or len(expected) != 2:
            raise RuntimeError('Both actual original terminal actors required')
        for final, (camp, actor) in zip(terminal, expected):
            if final['camp'] != camp or final['heroId'] != actor.id or not final.get('uid') or type(final.get('state')) is not int or not all(math.isfinite(final[k]) for k in ('currentHp', 'maxHp')):
                raise RuntimeError('Original terminal actor identity/state/HP incomplete')
        artifacts.append({'case': case['case'], 'wireSHA256': case['wireSHA256'],
                          'originalRecord': native_path.name, 'originalRecordSHA256': sha(native_path),
                          'typedBusinessResultCaptured': False, 'originalFinalActorHP': terminal})
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
                          'sourceKind': '', 'actualBusinessStart': False,
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
    bundle = {'schema': 'p12-field-pvp5-native-matrix-v1', 'sourceManifest': str(SOURCE.relative_to(ROOT)),
              'sourceManifestSHA256': SOURCE_SHA, 'captureManifest': str(capture.relative_to(ROOT)),
              'captureManifestSHA256': sha(capture), 'caseCount': 4, 'nativeScope': SCOPE,
              'fixtures': fixtures, 'records': preserve_records(output / 'records.json', output, source),
              'sourceKind': '', 'actualBusinessStart': False,
              'businessAuthority': False, 'fullMatrixCompared': False,
              'runnerSHA256': sha(Path(__file__).resolve())}
    bundle_path = output / 'bundle.json'
    bundle_path.write_text(json.dumps(bundle, indent=2) + '\n')
    env = {'HYJS_P12_FIELD_PVP_NATIVE_BUNDLE': str(bundle_path),
           'HYJS_P12_FIELD_PVP_NATIVE_BUNDLE_SHA256': sha(bundle_path)}
    report = {'status': 'FIXTURES_READY_COMPARISON_PENDING', 'bundle': str(bundle_path.relative_to(ROOT)),
              'bundleSHA256': sha(bundle_path), 'environment': env, 'cases': 4,
              'requiredTest': '^TestP12FieldPVPNativeAllLayers$', 'requiredLayers': REQUIRED_LAYERS,
              'sourceKind': '', 'actualBusinessStart': False,
              'businessAuthority': False, 'phoneBusinessAccepted': False}
    if args.service_test:
        binary = args.service_test.resolve()
        if not binary.is_relative_to(ROOT / 'build') or not binary.is_file():
            raise RuntimeError('Explicit existing owned build test binary required')
        report['testBinarySHA256'] = sha(binary)
        log = output / 'comparison.log'
        with log.open('x') as sink:
            run = subprocess.run([str(binary), '-test.run=^TestP12FieldPVPNativeAllLayers$', '-test.v'],
                                 cwd=ROOT / 'hyjs-server/service', env={**os.environ, **env},
                                 stdout=sink, stderr=subprocess.STDOUT)
        report['testExitCode'] = run.returncode
        report['comparisonLogSHA256'] = sha(log)
        log_text = log.read_text()
        layers_pass = all('--- PASS: TestP12FieldPVPNativeAllLayers/' + layer + ' ' in log_text
                          for layer in REQUIRED_LAYERS)
        report['allThirteenLayersPassed'] = layers_pass
        if run.returncode == 0 and '--- PASS: TestP12FieldPVPNativeAllLayers ' in log_text and '--- SKIP:' not in log_text and layers_pass:
            report['status'] = 'FOUR_CASE_DIAGNOSTIC_FULL_MATRIX_PASS_CAPABILITY_PENDING'
        else:
            report['status'] = 'FULL_MATRIX_FAILED_OR_NOT_EXECUTED'
    (output / 'comparison-report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    if report['status'] == 'FULL_MATRIX_FAILED_OR_NOT_EXECUTED':
        raise SystemExit(1)


if __name__ == '__main__':
    main()
