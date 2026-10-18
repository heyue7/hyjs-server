"""Offline Android handoff and exact phone-versus-host comparison for typed9.

No action in this script accesses ADB, a service or a database. Device staging
and collection commands are printed for the assigned phone owner to execute.
"""
import argparse
import collections
import copy
import hashlib
from itertools import zip_longest
import json
from pathlib import Path
import shutil
import sys
import zipfile
from trial047025_natural import ROOT, SOURCE, sha
from compare import first_difference

SOURCE_SHA = 'e7bd26ff96f85de6ab38df71535557efe443aa04655f2850a10325d6a638709f'
APK_SHA = '692767bc2d23ac79f00be13632551b85f6938ea6bd790ec3deea46468f30ec43'
PRIVATE_CALLER_SHA = '1cd443e01b42b808be90e84d248e2e9432d6c7ab52093f817eff6106b7c5c207'
LOCAL = ROOT / 'analysis/growth-first-20261016/p6-local-trial047025-natural-r1-20261017'
CAPTURE = ROOT / 'analysis/p6/p7-natural047025-native-r1-20261017'


def inputs():
    sys.path.insert(0, str(ROOT))
    from server.codec import message
    from tools.native_batch.prepare import load_rush_typed_source
    from tools.native_batch.collect import validate_typed_preflight
    if sha(SOURCE) != SOURCE_SHA:
        raise ValueError('Fixed four-case source changed')
    source = json.loads(SOURCE.read_bytes())
    if source['nativeCompared'] or source['authoritative'] or len(source['cases']) != 4:
        raise ValueError('Input count or authority changed')
    rows = []
    for index, row in enumerate(source['cases']):
        load_rush_typed_source(row, SOURCE, 'highest')
        wire_path = SOURCE.parent / row['wire']
        wire = message('SCFightPlayback')
        wire.ParseFromString(wire_path.read_bytes())
        if sha(wire_path) != row['wireSHA256'] or not wire.IsInitialized():
            raise ValueError('Incomplete or changed original wire')
        stem = f'{index:03d}'
        prepared = {**row, 'batchWire': stem + '.pb',
                    'batchTypedCallback': stem + '.rush-start.bin',
                    'batchTypedDriver': stem + '.rush-auto-start.txt',
                    'typedDriverSHA256': hashlib.sha256(str(row['manualAutoStartRoundTime']).encode('ascii')).hexdigest(),
                    'batchTypedDriverPolicy': stem + '.rush-auto-policy.txt',
                    'typedDriverPolicySHA256': hashlib.sha256(row['manualControlSource'].encode('ascii')).hexdigest()}
        validate_typed_preflight(prepared, wire, 'highest', prepared['batchWire'])
        rows.append(prepared)
    return source, rows


def preflight(output):
    import dnfile
    from dncil.cil.body.reader import read_method_body_from_bytes
    source, rows = inputs()
    if output.exists():
        raise FileExistsError('Preserve Android preflight/handoff')
    apk = ROOT / 'build/hyjs-local.apk'
    if sha(apk) != APK_SHA:
        raise ValueError('Offline APK692 baseline changed; do not assume installed identity')
    with zipfile.ZipFile(apk) as archive:
        bridge = archive.read('assets/bin/Data/Managed/NativeBatch.dll')
    if hashlib.sha256(bridge).hexdigest() != 'e8e79ff499823e011277ff6170b4c5956dde3de9fc1cbc9ed21fe0bdd54231a1':
        raise ValueError('APK private caller ABI changed')
    private = ROOT / '.tools/native-batch/native-runner-current.dll'
    host = ROOT / 'analysis/growth-first-20261016/p6-local-headless-p7-runtime-r1/runtime/LocalOriginalCaller.dll'
    if sha(private) != PRIVATE_CALLER_SHA or sha(host) != '8fd6a7d61dc001fd78a8ec903e84204c30b34f04213dbd351a7f4dafadf0b02c':
        raise ValueError('Pinned existing caller changed')
    method_proofs = []
    for path in [host, private]:
        assembly = dnfile.dnPE(str(path))
        methods = {}
        for owner in assembly.net.mdtables.TypeDef.rows:
            if str(owner.TypeName) not in ['NativeBatchRunner', 'OriginalHeadlessPlaybackCaller']:
                continue
            for method in owner.MethodList:
                if str(method.row.Name) not in ['RunCase', 'RunRushManualAutoStart']:
                    continue
                body = read_method_body_from_bytes(assembly.get_data(method.row.Rva))
                il = assembly.get_data(method.row.Rva + body.header_size, body.code_size)
                eh = [(e.exception_type, e.try_start, e.try_end, e.handler_start, e.handler_end,
                       e.filter_start, getattr(e.catch_type, 'value', None)) for e in body.exception_handlers]
                methods[str(owner.TypeName) + '::' + str(method.row.Name)] = {
                    'instructionSHA256': hashlib.sha256(il).hexdigest(), 'exceptionRegions': eh}
        method_proofs.append(methods)
    if len(method_proofs[0]) != 2 or method_proofs[0] != method_proofs[1]:
        raise ValueError('Existing Android caller and executed host typed9 entry differ')
    output.mkdir(parents=True)
    staged = output / 'offline-android-layout'
    staged.mkdir()
    for row in rows:
        shutil.copy2(SOURCE.parent / row['wire'], staged / row['batchWire'])
        shutil.copy2(SOURCE.parent / row['typedCallback'], staged / row['batchTypedCallback'])
        (staged / row['batchTypedDriver']).write_text(str(row['manualAutoStartRoundTime']))
        (staged / row['batchTypedDriverPolicy']).write_text(row['manualControlSource'])
    prepared = {'sourceManifest': str(SOURCE), 'selector': 'highest', 'cases': rows,
                'scope': 'original-engine inputs; AI mode and native results require separate verification',
                'nativeCompared': False, 'authoritative': False}
    (staged / 'manifest.json').write_text(json.dumps(prepared, indent=2))
    tool_paths = [Path(__file__), ROOT / 'tools/native_batch/prepare.py', ROOT / 'tools/native_batch/collect.py',
                  ROOT / 'tools/native_batch/NativeBatchBridge.cs', ROOT / 'tools/native_batch/NativeBatchRunner.cs',
                  ROOT / 'analysis/p6/OriginalHeadlessPlaybackCaller.cs']
    report = {'schema': 'typed9-natural047025-android-handoff-v1', 'status': 'OFFLINE_PREFLIGHT_PASS_PHONE_NOT_TOUCHED',
              'sourceManifest': str(SOURCE.relative_to(ROOT)), 'sourceSHA256': SOURCE_SHA,
              'expectedAPK': APK_SHA, 'expectedPrivateCallerSHA256': PRIVATE_CALLER_SHA,
              'privateCallerPath': 'files/native-runner-current.dll', 'checkedLocalAPKOnlyNotCurrentPhone': True,
              'androidBridgeSHA256': hashlib.sha256(bridge).hexdigest(),
              'hostAndAndroidTypedEntryInstructionBytesAndEH': method_proofs[0],
              'sourcePins': {str(p.relative_to(ROOT)): sha(p) for p in tool_paths},
              'layoutPins': {str(p.relative_to(output)): sha(p) for p in staged.iterdir()},
              'caseCount': 4, 'selector': 'highest', 'kitServerId': '60006', 'kitFightId': 'highest',
              'businessAuthority': False, 'nativeCompared': False, 'deviceAccess': False,
              'commands': [
                  'rtk proxy .analysis-venv/bin/python tools/native_batch/prepare.py --manifest ' + str(SOURCE.relative_to(ROOT)) + ' --selector highest --serial 10AF7Q0KSJ003GK',
                  'rtk proxy .analysis-venv/bin/python tools/native_batch/collect.py --prepared build/native-batch-input/highest/manifest.json --output ' + str(CAPTURE.relative_to(ROOT)) + ' --serial 10AF7Q0KSJ003GK --timeout 180',
                  'rtk proxy .analysis-venv/bin/python tools/local_original_engine_host/phone_trial047025_natural.py --capture ' + str((CAPTURE / 'manifest.json').relative_to(ROOT)) + ' --output ' + str((output / 'phone-vs-host-comparison-r1.json').relative_to(ROOT))]}
    (output / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False))


def compare(capture_path, output):
    sys.path.insert(0, str(ROOT))
    sys.path.insert(0, str(ROOT / 'analysis/p6'))
    from native_evidence import open_native
    from tools.native_batch.collect import validate_typed_execution
    if output.exists():
        raise FileExistsError('Preserve every original comparison/failure')
    source, expected = inputs()
    capture = json.loads(capture_path.read_bytes())
    if (Path(capture['sourceManifest']).resolve() != SOURCE or capture['sourceManifestSHA256'] != SOURCE_SHA
            or capture['selector'] != 'highest' or len(capture['cases']) != 4
            or capture.get('authoritative') or capture.get('nativeCompared')):
        raise ValueError('Exact new four-case original capture required')
    local_run = json.loads((LOCAL / 'manifest.json').read_bytes())
    observed = {row['wireSHA256']: row for row in capture['cases']}
    if len(observed) != 4 or set(observed) != {row['wireSHA256'] for row in expected}:
        raise ValueError('Original capture denominator differs')
    cases = []
    for index, row in enumerate(expected):
        phone_row = observed[row['wireSHA256']]
        for key, value in row.items():
            if key not in ['status', 'nativeScope'] and phone_row.get(key) != value:
                raise ValueError('Original collected metadata differs: ' + key)
        if phone_row['nativeScope'] != 'original-pve-manual-then-auto-ai':
            raise ValueError('Wrong original execution mode')
        name = phone_row['nativeResult']
        if Path(name).name != name or name != f'{index:03d}.native.jsonl':
            raise ValueError('Only exact numbered original capture files')
        phone = capture_path.parent / name
        host = LOCAL / 'runtime' / local_run['caseSets'][0]['cases'][index]['localWire']
        host = host.with_suffix('.native.jsonl')
        if sha(host) != local_run['outputPins'][str(host.relative_to(LOCAL))]:
            raise ValueError('Fixed local original Engine output changed')
        full_delta = payload_delta = None
        platform = []
        counts = collections.Counter()
        digest = hashlib.sha256()
        first = last = None
        phone_count = host_count = 0
        with open_native(phone) as old, host.open('rb') as new:
            for event_index, (a_line, b_line) in enumerate(zip_longest(old, new)):
                if a_line is not None:
                    digest.update(a_line)
                a = json.loads(a_line) if a_line is not None else None
                b = json.loads(b_line) if b_line is not None else None
                phone_count += a is not None
                host_count += b is not None
                if a is not None:
                    first = first or a
                    last = a
                    counts[a['kind']] += 1
                path = '$[' + str(event_index) + ']'
                if full_delta is None:
                    full_delta = first_difference(a, b, path)
                if a is not None and b is not None and a.get('kind') == b.get('kind') == 'ai_notify':
                    left, right = a['data'].get('stack'), b['data'].get('stack')
                    if left != right:
                        platform.append({'event': event_index, 'round': a['round'], 'time': a['time'],
                                         'phoneStack': left, 'hostStack': right})
                    # A second explicitly scoped diagnostic comparison only;
                    # the full-schema delta above and both raw logs retain it.
                    a, b = copy.deepcopy(a), copy.deepcopy(b)
                    a['data'].pop('stack', None)
                    b['data'].pop('stack', None)
                if payload_delta is None:
                    payload_delta = first_difference(a, b, path)
        if (digest.hexdigest() != phone_row['nativeSHA256'] or first is None or first['kind'] != 'input'
                or first['fightId'] != row['fightId'] or first['seed'] != row['seed'] or first['battleType'] != 9
                or first['scope'] != 'original-pve-manual-then-auto-ai'
                or last is None or last['kind'] != 'result' or type(last.get('attackerWon')) is not bool
                or counts['original_final_operates'] != 1):
            raise ValueError('Changed, incomplete or wrong original typed9 trace')
        validate_typed_execution(phone_row, first)
        cases.append({'case': row['case'], 'phoneTraceSHA256': digest.hexdigest(), 'hostTraceSHA256': sha(host),
                      'phoneRows': phone_count, 'hostRows': host_count, 'eventCounts': dict(counts),
                      'fullSchemaExactMatch': full_delta is None, 'fullSchemaFirstDifference': full_delta,
                      'payloadExactExceptExplicitPlatformStack': payload_delta is None,
                      'payloadFirstDifference': payload_delta, 'platformIdentityDifferences': platform})
    report = {'schema': 'typed9-natural047025-phone-host-comparison-v1',
              'fullSchemaStatus': 'PASS' if all(c['fullSchemaExactMatch'] for c in cases) else 'FAIL',
              'battlePayloadStatusExceptExplicitPlatformStack': 'PASS' if all(c['payloadExactExceptExplicitPlatformStack'] for c in cases) else 'FAIL',
              'sourceManifestSHA256': SOURCE_SHA, 'captureManifestSHA256': sha(capture_path),
              'localExecutionManifestSHA256': sha(LOCAL / 'manifest.json'), 'runnerSHA256': sha(Path(__file__)),
              'numericTolerance': 0, 'listOrderPreserved': True, 'rawCapturesUnchanged': True,
              'explicitPlatformField': 'ai_notify.data.stack', 'cases': cases,
              'businessAuthority': False, 'productionFamilyOpened': False,
              'go13LocalReport': str((LOCAL / 'go-diagnostic-r1.json').relative_to(ROOT)),
              'go13LocalReportSHA256': sha(LOCAL / 'go-diagnostic-r1.json'),
              'phoneFixture13MatrixExecutedByThisScript': False}
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({k: report[k] for k in ['fullSchemaStatus', 'battlePayloadStatusExceptExplicitPlatformStack']}, ensure_ascii=False))
    return 0 if report['battlePayloadStatusExceptExplicitPlatformStack'] == 'PASS' else 1


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--preflight', action='store_true')
    parser.add_argument('--capture', type=Path)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.preflight == bool(args.capture):
        parser.error('Choose offline preflight or compare a completed independent capture')
    if args.preflight:
        preflight(args.output.resolve())
    else:
        raise SystemExit(compare(args.capture.resolve(), args.output.resolve()))
