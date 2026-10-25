"""Pinned guard10 local CoreCLR diagnostic. Never a phone capture or authority grant."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[2]
SHARED = Path('/Users/lirui/LR/hyjs')
BASE = SHARED / 'analysis/growth-first-20261016/p6-local-headless-p7-runtime-r1'
DELIVERY_SHA = '07e85496d17a40a3a42cddbb6651cac78c954b3762ab8216ee2d5e311f49c9a6'
SOURCE = ROOT / 'analysis/p6/guild-guard-inputs-20261025-native/manifest.json'
SOURCE_SHA = '462e6144ab013f7cc228cc3ebcdb01bd1b76ecbc6b2bfadc8c0640579fe73bb0'
JIT = {'COMPlus_TieredCompilation': '0', 'COMPlus_JitNoInline': '1', 'COMPlus_JITMinOpts': '1'}
sys.path.insert(0, str(ROOT))
from google.protobuf import json_format
from server.codec import message


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def need(value, explanation):
    if not value:
        raise ValueError(explanation)


def read_pb(kind, path):
    value = message(kind)
    value.ParseFromString(Path(path).read_bytes())
    return value


def initial_actors(wire):
    return [{'camp': camp, 'uid': str(camp) + '_' + hero.guid, 'guid': hero.guid,
             'heroId': hero.id, 'pose': hero.pose, 'bloodNumber': hero.bloodNumber,
             'currentHp': hero.inst.curHp, 'maxHp': hero.prop.hp,
             'initialActorSHA256': hashlib.sha256(hero.SerializeToString(deterministic=True)).hexdigest()}
            for camp, side in enumerate((wire.attacker, wire.defender2)) for hero in side.formation]


def prepare(output):
    need(not output.exists(), 'Use a fresh diagnostic output directory')
    need(sha(SOURCE) == SOURCE_SHA and sha(BASE / 'delivery.json') == DELIVERY_SHA, 'Pinned source/delivery changed')
    source = json.loads(SOURCE.read_bytes())
    need(len(source['cases']) == 10 and not source['blockedProducerCases'], 'Exactly ten existing cases required')
    need(not source['authoritative'] and not source['warApplied'] and not source['nativeCompared'], 'Diagnostic source boundary changed')
    exported = SOURCE.parent / 'source-export.json'
    need(sha(exported) == source['sourceExportSHA256'], 'Producer evidence changed')
    exported_rows = {row['guardID']: row for row in json.loads(exported.read_bytes())['cases']}
    validated = []
    for index, row in enumerate(source['cases']):
        need(row['wire'] == '%03d.pb' % index and row['nativeScope'] == 'original-pve-automatic-ai', 'Numbered automatic source changed')
        for name, digest in exported_rows[row['case']]['files'].items():
            need(Path(name).name == name and sha(SOURCE.parent / name) == digest, 'Producer sidecar changed: ' + name)
        need(sha(SOURCE.parent / row['wire']) == row['wireSHA256'], 'Wire changed')
        need(sha(SOURCE.parent / row['originalCallbackFile']) == row['originalCallbackSHA256'], 'Typed callback changed')
        need(sha(SOURCE.parent / row['frozenInput']) == row['p6InputHash'], 'Frozen input changed')
        wire = read_pb('SCFightPlayback', SOURCE.parent / row['wire'])
        typed = read_pb('SCUFPveFightRecordReplay', SOURCE.parent / row['originalCallbackFile'])
        frozen = json.loads((SOURCE.parent / row['frozenInput']).read_bytes())
        need(wire.battleType == 14 and wire.maxRound == 40 and wire.randSeed == row['seed'] and wire.fightid == row['fightId'], 'Wire identity changed')
        need(wire.attackerFactor == wire.defenderFactor == 1 and not wire.HasField('defender1') and not wire.HasField('optRecord') and not wire.winConditions, 'Driver/result hint changed')
        need(typed.player == wire.attacker and typed.monster == wire.defender2 and typed.fightId == wire.fightid and typed.randomSeed == wire.randSeed and typed.maxRound == 40 and typed.attackerDamageFactor == typed.defenderDamageFactor == 1 and not typed.HasField('isPlayerWin'), 'Typed source differs')
        need(json_format.ParseDict(frozen['attacker'], message('BattlePlayer')) == wire.attacker and json_format.ParseDict(frozen['monster'], message('BattleMonster')) == wire.defender2, 'Frozen actors differ')
        validated.append(dict(row, initialActors=initial_actors(wire)))
    reference = json.loads((BASE / 'manifest.json').read_bytes())
    for name, digest in reference['runtimeInputPins'].items():
        need(sha(BASE / 'runtime' / name) == digest, 'Stable DLL changed: ' + name)
    proof = json.loads((SHARED / 'analysis/growth-first-20261016/p6-local-headless-p7-r1/attempt-2-managed-logger/manifest.json').read_bytes())['originalRawTablePins']
    proof = {Path(key).stem.lower(): value['sha256'] for key, value in proof.items()}
    need(len(proof) == 162, 'Ambiguous original raw-table source names')
    tables = list((BASE / 'runtime/Assets').rglob('*.txt'))
    need(len(tables) == 162, 'Original table denominator changed')
    for path in tables:
        key = path.stem.lower()
        need(sha(path) == proof[key], 'Original table changed: ' + path.name)
    output.mkdir(parents=True)
    runtime = output / 'runtime'
    runtime.mkdir()
    for name in reference['runtimeInputPins']:
        shutil.copy2(BASE / 'runtime' / name, runtime / name)
    shutil.copytree(BASE / 'runtime/Assets', runtime / 'Assets')
    shutil.copy2(BASE / 'runtime/LocalOriginalEngineHost.runtimeconfig.json', runtime / 'GuildGuardProbeHost.runtimeconfig.json')
    cases = runtime / 'cases/pve-auto'
    cases.mkdir(parents=True)
    for row in validated:
        shutil.copy2(SOURCE.parent / row['wire'], cases / row['wire'])
    snapshots = output / 'execution-source'
    snapshots.mkdir()
    for path in [Path(__file__), Path(__file__).with_name('GuildGuardProgram.cs')]:
        shutil.copy2(path, snapshots / path.name)
    return dict(schema='guild-guard10-local-host-diagnostic-v1', status='PREPARED', executionOrigin='local-coreclr-original-engine-not-phone-capture',
                sourceManifest=str(SOURCE), sourceManifestSHA256=SOURCE_SHA, baseline=str(BASE), baselineDeliverySHA256=DELIVERY_SHA,
                runtimeDLLPins=reference['runtimeInputPins'], raw162TablePins={str(path.relative_to(BASE / 'runtime')): sha(path) for path in tables},
                sourcePins={str(path.relative_to(ROOT)): sha(path) for path in [Path(__file__), Path(__file__).with_name('GuildGuardProgram.cs')]},
                runtimeEnvironment=JIT, automaticDriver='Original NativeBatchRunner.RunCase(path,true,false)', cases=validated,
                nativeCompared=False, authoritative=False, warApplied=False, original547AndCallerChanged=False)


def difference(left, right, path='$'):
    if type(left) is not type(right) and not isinstance(left, (int, float)):
        return dict(path=path, go=left, host=right)
    if isinstance(left, dict) and isinstance(right, dict):
        if set(left) != set(right):
            return dict(path=path, goKeys=list(left), hostKeys=list(right))
        for key in left:
            value = difference(left[key], right[key], path + '.' + key)
            if value:
                return value
    elif isinstance(left, list) and isinstance(right, list):
        if len(left) != len(right):
            return dict(path=path, goCount=len(left), hostCount=len(right))
        for index, (a, b) in enumerate(zip(left, right)):
            value = difference(a, b, path + '[' + str(index) + ']')
            if value:
                return value
    elif left != right:
        return dict(path=path, go=left, host=right)
    return None


def compare_case(row, trace):
    terminal = json.loads((SOURCE.parent / (Path(row['frozenInput']).stem.replace('.frozen-input', '') + '.go-prospective-terminal.json')).read_bytes())
    outcome = terminal['outcome']
    first_state = last_state = record = result = None
    presses, platform, events = [], [], 0
    with trace.open() as stream:
        for line in stream:
            event = json.loads(line)
            events += 1
            if event['kind'] == 'input':
                need(event['scope'] == 'original-pve-automatic-ai' and event['seed'] == row['seed'] and event['battleType'] == 14 and event['attackerFactor'] == event['defenderFactor'] == 1, 'Actual host driver/input mismatch')
            elif event['kind'] == 'state':
                if first_state is None:
                    first_state = event['state']
                last_state = event['state']
            elif event['kind'] == 'original_final_operates':
                need(record is None, 'Duplicate original final record')
                record = base64.b64decode(event['data']['protobufBase64'], validate=True)
            elif event['kind'] == 'skill_press':
                data = event['data']
                presses.append(dict(roundNum=event['round'], roundTime=event['time'], camp=data['camp'], actorUid=re.sub(r'^[01]_', '', data['actorUid']), skillIndex=data['skillIndex']))
            elif event['kind'] == 'ai_notify' and 'stack' in event['data']:
                platform.append(dict(event=events - 1, stack=event['data']['stack']))
            elif event['kind'] == 'result':
                result = event['attackerWon']
    need(first_state is not None and last_state is not None and record is not None and result is not None, 'Incomplete original host trace')
    host_initial, host_final = [], []
    for camp, side in enumerate(('attacker', 'defender')):
        source = [actor for actor in row['initialActors'] if actor['camp'] == camp]
        need(len(source) == len(first_state[side]['actors']) == len(last_state[side]['actors']), 'Actor denominator differs')
        for initial, final, origin in zip(first_state[side]['actors'], last_state[side]['actors'], source):
            host_initial.append(dict(camp=camp, uid=initial['uid'], heroId=initial['heroId'], currentHp=initial['attribute']['hp'], maxHp=initial['attribute']['maxHp']))
            host_final.append(dict(camp=camp, uid=final['uid'], guid=origin['guid'], heroId=final['heroId'], pose=origin['pose'], bloodNumber=origin['bloodNumber'], currentHp=final['attribute']['hp'], maxHp=final['attribute']['maxHp'], state=final['state'], initialActorSHA256=origin['initialActorSHA256']))
    expected_initial = [{key: value for key, value in actor.items() if key in ['camp', 'uid', 'heroId', 'currentHp', 'maxHp']} for actor in row['initialActors']]
    initial_sha = difference([actor['initialActorSHA256'] for actor in terminal['finalActors']], [actor['initialActorSHA256'] for actor in row['initialActors']])
    ap_expected = [outcome[side]['inst'].get('anger', 0) for side in ('attacker', 'monster')]
    ap_actual = [last_state[side]['anger'] for side in ('attacker', 'defender')]
    expected_record = json_format.ParseDict(terminal['originalOperates'], message('BattleOperate')).SerializeToString(deterministic=True)
    actual_record = message('BattleOperate')
    actual_record.ParseFromString(record)
    comparisons = dict(initialActorInputSHA=initial_sha, initialHP=difference(expected_initial, host_initial),
                       finalActors=difference(terminal['finalActors'], host_final), finalAP=difference(ap_expected, ap_actual),
                       initialAP=difference([row[side]['anger'] for side in ('attacker', 'monster')], [first_state[side]['anger'] for side in ('attacker', 'defender')]),
                       skillPressOperations=difference(outcome['operates'].get('skillOpts', []), presses),
                       winner=difference(outcome['winner'] == 0, result), finalRound=difference(outcome['finalRound'], last_state['round']),
                       originalFinalPB=None if expected_record == record else dict(goSHA256=hashlib.sha256(expected_record).hexdigest(), hostSHA256=hashlib.sha256(record).hexdigest()))
    return dict(case=row['case'], status='PASS' if all(value is None for value in comparisons.values()) else 'FAIL', comparisons=comparisons,
                eventCount=events, hostTraceSHA256=sha(trace), originalFinalPBSHA256=hashlib.sha256(record).hexdigest(),
                platformIdentityObservations=platform, fullSchemaStatus='NOT_COMPARABLE_NO_PHONE_TRACE', numericTolerance=0,
                actorPoseAndInputHashProvenance='Exact source wire/typed input, then host UID/hero/HP match; pose/input SHA is not invented from runtime actor fields')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--prepare-only', action='store_true')
    args = parser.parse_args()
    output = args.output_directory.resolve()
    report = prepare(output)
    manifest = output / 'manifest.json'
    manifest.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    if args.prepare_only:
        print(json.dumps(dict(status=report['status'], manifest=str(manifest))))
        return
    dotnet = ROOT / '.tools/native-batch/dotnet/dotnet'
    compiler = next((dotnet.parent / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = sorted((dotnet.parent / 'packs/Microsoft.NETCore.App.Ref/8.0.32/ref/net8.0').glob('*.dll'))
    runtime = output / 'runtime'
    try:
        need(refs, 'Use existing SDK reference pack only')
        command = ['rtk', 'proxy', str(dotnet), str(compiler), '/nologo', '/target:exe', '/out:' + str(runtime / 'GuildGuardProbeHost.dll'), *['/reference:' + str(path) for path in refs], str(Path(__file__).with_name('GuildGuardProgram.cs'))]
        result = subprocess.run(command, capture_output=True, text=True, timeout=60)
        (output / 'compile-wrapper.log').write_text(result.stdout + result.stderr)
        need(result.returncode == 0, 'Small wrapper compilation failed')
        report['compiledWrapperSHA256'] = sha(runtime / 'GuildGuardProbeHost.dll')
        report['results'] = []
        for row in report['cases']:
            trace = runtime / 'cases/pve-auto' / Path(row['wire']).with_suffix('.native.jsonl')
            started = time.monotonic()
            command = ['rtk', 'proxy', str(dotnet), str(runtime / 'GuildGuardProbeHost.dll'), str(runtime), str(runtime / 'cases/pve-auto'), row['wire']]
            try:
                result = subprocess.run(command, capture_output=True, text=True, timeout=120, env={**os.environ, **JIT})
                (output / ('execute-' + row['wire'] + '.log')).write_text(result.stdout + result.stderr)
                need(result.returncode == 0, 'Host failed; see per-case log')
                compared = compare_case(row, trace)
            except Exception as error:
                compared = dict(case=row['case'], status='FAIL', error=str(error), partialTracePreserved=trace.exists())
            compared['seconds'] = time.monotonic() - started
            report['results'].append(compared)
            manifest.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        report['status'] = 'HOST_DIAGNOSTIC_COMPARE_PASS' if all(row['status'] == 'PASS' for row in report['results']) else 'HOST_DIAGNOSTIC_COMPARE_FAIL'
    except Exception as error:
        report.update(status='FAIL', error=str(error))
    report['fullSchemaStatus'] = 'NOT_COMPARABLE_NO_PHONE_TRACE'
    report['numericTolerance'] = 0
    report['outputPins'] = {str(path.relative_to(output)): sha(path) for path in runtime.rglob('*.native.jsonl')}
    need(all(sha(runtime / name) == digest for name, digest in report['runtimeDLLPins'].items()), 'Inherited runtime changed')
    need(sha(SOURCE) == SOURCE_SHA, 'Input source changed while running')
    manifest.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(dict(status=report['status'], results=[{key: row.get(key) for key in ['case', 'status', 'seconds', 'error']} for row in report.get('results', [])], manifest=str(manifest))))


if __name__ == '__main__':
    main()
