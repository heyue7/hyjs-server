"""Collect a prepared original-engine batch with input, scope and result checks."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
sys.path.insert(0, str(ROOT))
from device_demo import ADB, Device, PACKAGE
from server.codec import message


def validate_typed_preflight(case, wire, selector, name):
    """Real collector admission, shared with pure tests before any result wait."""
    scheduled = (case.get('nativeScope') == 'original-pve-manual-then-auto-ai'
                 and case.get('manualControlSource') == 'prepublished-diagnostic-schedule-v1')
    stem = Path(name).stem
    if scheduled:
        delay = case.get('manualAutoStartRoundTime')
        if (selector != 'highest' or wire.battleType not in (9, 10) or wire.HasField('optRecord')
                or not isinstance(delay, (int, float)) or not math.isfinite(delay) or delay < 0 or delay > 110
                or case.get('batchTypedCallback') != stem + '.rush-start.bin'
                or case.get('batchTypedDriver') != stem + '.rush-auto-start.txt'
                or case.get('batchTypedDriverPolicy') != stem + '.rush-auto-policy.txt'
                or case.get('typedDriverPolicySHA256') != hashlib.sha256(case['manualControlSource'].encode('ascii')).hexdigest()
                or case.get('typedDriverSHA256') != hashlib.sha256(str(delay).encode('ascii')).hexdigest()):
            raise RuntimeError('Explicit manual schedule requires exact typed/driver/policy SHA and delay binding')
    elif case.get('batchTypedDriverPolicy'):
        raise RuntimeError('Observed driver cannot acquire an explicit schedule policy')
    if case.get('batchTypedCallback'):
        typed = case['batchTypedCallback']
        if not ((typed == stem + '.rush-start.bin' and wire.battleType in (9, 10)
                 and (selector == 'pve-auto' or selector == 'highest' and (wire.HasField('optRecord') or scheduled)))
                or (typed == stem + '.wanted-start.bin' and wire.battleType == 8 and selector == 'highest' and case.get('nativeScope') == 'original-wanted-automatic-ai')
                or (typed == stem + '.awake-start.bin' and wire.battleType == 16 and selector == 'highest' and case.get('nativeScope') == 'original-awake16-automatic-ai')
                or (typed == stem + '.field-pvp.bin' and wire.battleType == 5 and selector == 'highest' and case.get('nativeScope') == 'original-field-pvp5-field002-automatic-ai' and case.get('fieldContext') == 'field_002')
                or (typed == stem + '.arena-result.bin' and wire.battleType == 2 and selector == 'highest')
                or (typed == stem + '.worldboss-start.bin' and wire.battleType == 18 and selector == 'highest' and case.get('nativeScope') == 'original-worldboss-automatic-ai')
                or (typed == stem + '.grab-start.bin' and wire.battleType == 19 and selector == 'highest')
                or (typed == stem + '.limit-result.bin' and wire.battleType == 20 and selector == 'highest')
                or (typed == stem + '.kf-replay.bin' and wire.battleType == 17 and selector == 'highest'
                    and case.get('nativeScope') in ('local-kf-replay-joint-realm-player-viewer-v1','original-kf-numeric-pid-wrapper-only-v1','local-kf-replay-same-realm-distinct-player-viewer-v1'))
                or (typed == stem + '.plunder-start.bin' and wire.battleType == 3 and selector == 'highest')):
            raise RuntimeError('Invalid original typed callback binding')
    if case.get('batchTypedDriver') and case.get('nativeScope') not in ('local-kf-replay-joint-realm-player-viewer-v1','original-kf-numeric-pid-wrapper-only-v1','local-kf-replay-same-realm-distinct-player-viewer-v1'):
        if (wire.battleType not in (9, 10) or case['batchTypedDriver'] != stem + '.rush-auto-start.txt'
                or selector != 'highest' or (not wire.HasField('optRecord') and not scheduled)):
            raise RuntimeError('Invalid original manual control source binding')
    if case.get('nativeScope') in ('local-kf-replay-joint-realm-player-viewer-v1','original-kf-numeric-pid-wrapper-only-v1','local-kf-replay-same-realm-distinct-player-viewer-v1'):
        joint=(case.get('viewerServerID'),case.get('viewerPlayerID'))
        if (wire.battleType!=17 or selector!='highest' or any(type(x)!=int for x in joint) or joint[1]<=0 or (joint[0]<=0 if case.get('nativeScope')!='original-kf-numeric-pid-wrapper-only-v1' else joint[0]!=0)
                or case.get('batchTypedCallback')!=stem+'.kf-replay.bin' or case.get('batchTypedDriver')!=stem+'.kf-viewer.txt'
                or case.get('typedDriverSHA256')!=hashlib.sha256(('%s|%d|%d'%(case['nativeScope'],*joint)).encode('ascii')).hexdigest()):
            raise RuntimeError('KF explicit local viewer identity/SHA binding differs')
    return scheduled


def validate_typed_execution(case, first):
    if case.get('batchTypedDriverPolicy') and first.get('typedDriverPolicySHA256') != case['typedDriverPolicySHA256']:
        raise ValueError('Original manual driver policy execution SHA changed')
    if case.get('manualControlSource') and first.get('manualControlSource') != case['manualControlSource']:
        raise ValueError('Original manual control provenance differs')
    # A typed source is accepted only when the running original caller reports
    # the exact artifact hash; input PB identity alone does not prove its use.
    if case.get('batchTypedCallback') and first.get('typedCallbackSHA256') != case['typedCallbackSHA256']:
        raise ValueError('Original typed callback execution SHA changed')
    if case.get('batchTypedDriver') and first.get('typedDriverSHA256') != case['typedDriverSHA256']:
        raise ValueError('Original manual control execution SHA changed')
    if case.get('batchTypedTarget') and first.get('typedTargetSHA256') != case['typedTargetSHA256']:
        raise ValueError('Original typed target execution SHA changed')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--prepared', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--serial')
    parser.add_argument('--timeout', type=int, default=600)
    args = parser.parse_args()
    prepared = args.prepared.resolve()
    output = args.output.resolve()
    if not prepared.is_relative_to(ROOT / 'build/native-batch-input'):
        raise RuntimeError('Use the verified private-runner staging manifest')
    if not output.is_relative_to(ROOT / 'analysis/p6'):
        raise RuntimeError('Use the project combat output directory')
    data = json.loads(prepared.read_text())
    source_manifest = Path(data['sourceManifest']).resolve()
    if not source_manifest.is_relative_to(ROOT / 'analysis'):
        raise RuntimeError('Invalid original source manifest')
    source_manifest_sha = hashlib.sha256(source_manifest.read_bytes()).hexdigest()
    selector = data['selector']
    if selector not in ('highest', 'golden', 'pve-auto', 'pve-compat'):
        raise RuntimeError('Invalid original-runner selector')
    cases = data['cases']
    if not cases:
        raise RuntimeError('No prepared cases')
    output.mkdir(parents=True, exist_ok=True)
    device = Device(args.serial)
    remote = 'files/native-batch/' + selector
    staged = json.loads(device.text('shell', 'run-as', PACKAGE, 'cat', remote + '/manifest.json'))
    if staged != data:
        raise RuntimeError('Device staging does not match the selected manifest')
    # Verify all private wire inputs before trusting any terminal result files.
    input_identities = {}
    for case in cases:
        name = case['batchWire']
        if len(name) != 6 or not name[:3].isdigit() or not name.endswith('.pb'):
            raise RuntimeError('Invalid original input name')
        actual = device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + name).split()[0]
        if actual != case['wireSHA256']:
            raise RuntimeError('Original device input changed')
        raw = (prepared.parent / name).read_bytes()
        if hashlib.sha256(raw).hexdigest() != actual:
            raise RuntimeError('Local staging no longer matches the original wire')
        wire = message('SCFightPlayback')
        wire.ParseFromString(raw)
        if not wire.IsInitialized():
            raise RuntimeError('Original wire is incomplete')
        input_identities[name] = (wire.fightid, wire.randSeed, wire.battleType)
        validate_typed_preflight(case, wire, selector, name)
        if case.get('batchTypedCallback'):
            typed_name = case['batchTypedCallback']
            typed_sha = hashlib.sha256((prepared.parent / typed_name).read_bytes()).hexdigest()
            if typed_sha != case['typedCallbackSHA256'] or device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + typed_name).split()[0] != typed_sha:
                raise RuntimeError('Original typed callback changed after staging')
        if case.get('batchTypedDriver'):
            driver_name = case['batchTypedDriver']
            driver_sha = hashlib.sha256((prepared.parent / driver_name).read_bytes()).hexdigest()
            if driver_sha != case['typedDriverSHA256'] or device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + driver_name).split()[0] != driver_sha:
                raise RuntimeError('Original manual control changed after staging')
        if case.get('batchTypedDriverPolicy'):
            policy_name = case['batchTypedDriverPolicy']
            policy_data = (prepared.parent / policy_name).read_bytes()
            if (policy_data.decode('ascii') != case['manualControlSource']
                    or hashlib.sha256(policy_data).hexdigest() != case['typedDriverPolicySHA256']
                    or device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + policy_name).split()[0] != case['typedDriverPolicySHA256']):
                raise RuntimeError('Original manual driver policy changed after staging')
        if case.get('batchTypedTarget'):
            target_name = case['batchTypedTarget']
            if wire.battleType != 3 or target_name != Path(name).stem + '.plunder-target.txt':
                raise RuntimeError('Invalid original target source binding')
            target_sha = hashlib.sha256((prepared.parent / target_name).read_bytes()).hexdigest()
            if target_sha != case['typedTargetSHA256'] or device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + target_name).split()[0] != target_sha:
                raise RuntimeError('Original target changed after staging')
    deadline = time.monotonic() + args.timeout
    last = None
    while time.monotonic() < deadline:
        status = device.text('shell', 'run-as', PACKAGE, 'cat', remote + '/status.txt')
        if status != last:
            print(status[:500], flush=True)
            last = status
        if status.startswith('failed'):
            raise RuntimeError('Original batch failed; preserve its source and partial output')
        if status == 'complete ' + str(len(cases)):
            break
        if not status.startswith('running '):
            raise RuntimeError('Unexpected original batch status')
        time.sleep(2)
    else:
        raise RuntimeError('Original batch observation timed out; preserve the running batch')
    results = []
    for case in cases:
        stem = Path(case['batchWire']).stem
        target = output / (stem + '.native.jsonl')
        temporary = target.with_suffix('.pending')
        # Stream large native logs to one temporary file instead of accumulating
        # a second in-memory copy or copying the full historical evidence tree.
        with temporary.open('wb') as sink:
            copied = subprocess.run(
                [str(ADB), '-s', device.serial, 'exec-out', 'run-as', PACKAGE,
                 'cat', remote + '/' + target.name], stdout=sink,
                stderr=subprocess.PIPE, timeout=90)
        if copied.returncode:
            temporary.unlink(missing_ok=True)
            raise RuntimeError('Original output transfer failed')
        first = None
        projection = None
        last = None
        lines = 0
        try:
            with temporary.open('rb') as source:
                for line in source:
                    if not line.strip():
                        continue
                    last = json.loads(line)
                    if last.get("kind")=="wrapper_projection": projection=last.get("data")
                    if first is None:
                        first = last
                    lines += 1
            if not first or first.get('kind') != 'input' or not last or last.get('kind') != ('projection_result' if case.get('nativeScope')=='original-kf-numeric-pid-wrapper-only-v1' else 'result'):
                raise ValueError('Incomplete original result')
            expected_scope = (case['nativeScope'] if case.get('nativeScope') in ('local-kf-replay-joint-realm-player-viewer-v1','original-kf-numeric-pid-wrapper-only-v1','local-kf-replay-same-realm-distinct-player-viewer-v1')
                              else 'original-pve-manual-then-auto-ai' if case.get('batchTypedDriver')
                              else 'local-pve-no-target-finish-v1' if selector == 'pve-compat'
                              else 'original-pve-automatic-ai' if selector == 'pve-auto'
                              else 'original-plunder-automatic-ai' if case.get('battleType') == 3
                              else case['nativeScope'] if case.get('nativeScope') in ('original-awake16-automatic-ai', 'original-field-pvp5-field002-automatic-ai', 'original-wanted-automatic-ai')
                              else 'original-worldboss-automatic-ai' if case.get('battleType') == 18 and case.get('batchTypedCallback')
                              else 'original-pvp-automatic-ai' if case.get('battleType') in (2, 20, 5, 12, 18, 11, 19, 17, 14, 15)
                              else 'original-pve-playback-ai')
            if first.get('scope') != expected_scope:
                raise ValueError('Original execution mode changed')
            if first.get('seed') != case['seed'] or first.get('battleType') != case['battleType']:
                raise ValueError('Original execution input changed')
            if (first.get('fightId'), first.get('seed'), first.get('battleType')) != input_identities[case['batchWire']]:
                raise ValueError('Original execution wire identity changed')
            validate_typed_execution(case, first)
            if case.get('nativeScope')=='original-kf-numeric-pid-wrapper-only-v1':
                if (not case.get('wrapperProjectionOnly') or last.get('engineExecuted') is not False
                        or 'attackerWon' in last or projection!=case.get('expectedWrapperProjection')):
                    raise ValueError('Original KF projection-only evidence differs or pretends to be an Engine terminal')
            if case.get('fightId') and first.get('fightId') != case['fightId']:
                raise ValueError('Original fight identity changed')
        except (ValueError, KeyError):
            temporary.unlink(missing_ok=True)
            raise RuntimeError('Original output does not match the prepared battle') from None
        temporary.replace(target)
        with target.open('rb') as source:
            digest = hashlib.file_digest(source, 'sha256').hexdigest()
        remote_digest = device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + target.name).split()[0]
        if remote_digest != digest:
            raise RuntimeError('Collected original output differs from its completed device file')
        results.append(dict(case, nativeResult=target.name, nativeSHA256=digest,
                            nativeScope=first['scope'], bytes=target.stat().st_size,
                            lines=lines, status='native-collected-difference-pending'))
        print(json.dumps({'case': stem, 'scope': first['scope'], 'bytes': target.stat().st_size,
                          'attackerWon': last.get('attackerWon')}), flush=True)
    record = dict(data, cases=results, sourceManifestSHA256=source_manifest_sha,
                  status='collected-difference-pending')
    (output / 'manifest.json').write_text(json.dumps(record, indent=2) + '\n')
    # Remove only this completed batch's duplicate files after its permanent
    # original source and output have been verified. Keep terminal metadata.
    if (device.text('shell', 'run-as', PACKAGE, 'cat', remote + '/status.txt') != 'complete ' + str(len(cases))
            or json.loads(device.text('shell', 'run-as', PACKAGE, 'cat', remote + '/manifest.json')) != data):
        raise RuntimeError('Batch changed before duplicate cleanup; collected evidence is retained')
    for case in cases:
        name = case['batchWire']
        device.run('shell', 'run-as', PACKAGE, 'rm', remote + '/' + name,
                   remote + '/' + Path(name).stem + '.native.jsonl')
        (prepared.parent / name).unlink()
        if case.get('batchTypedCallback'):
            typed_name = case['batchTypedCallback']
            device.run('shell', 'run-as', PACKAGE, 'rm', remote + '/' + typed_name)
            (prepared.parent / typed_name).unlink()
        if case.get('batchTypedDriver'):
            driver_name = case['batchTypedDriver']
            device.run('shell', 'run-as', PACKAGE, 'rm', remote + '/' + driver_name)
            (prepared.parent / driver_name).unlink()
        if case.get('batchTypedDriverPolicy'):
            policy_name = case['batchTypedDriverPolicy']
            device.run('shell', 'run-as', PACKAGE, 'rm', remote + '/' + policy_name)
            (prepared.parent / policy_name).unlink()
        if case.get('batchTypedTarget'):
            target_name = case['batchTypedTarget']
            device.run('shell', 'run-as', PACKAGE, 'rm', remote + '/' + target_name)
            (prepared.parent / target_name).unlink()
    prepared.unlink()
    print(json.dumps({'collected': len(results), 'manifest': str(output / 'manifest.json')}), flush=True)


if __name__ == '__main__':
    main()
