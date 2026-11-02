"""Compare real receipt recovery before and after with the same isolated checks."""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
RUNTIME = Path('/Users/lirui/LR/hyjs/.tools/native-batch/dotnet')
OUTPUT = ROOT / 'analysis/round-two-performance-final/client-receipt-model.json'
SOURCES = [HERE / 'BattleRecovery.cs', HERE / 'FightEndJournal.cs',
           HERE / 'BattleReceiptRecoveryChecks.cs', Path(__file__).resolve()]
BASELINE = 'ef0eaac:tools/desert_extraction/BattleRecovery.cs'
DISABLED = ('boss_disabled_engine_receipt', 'monster_disabled_engine_receipt')


def digest(data):
    return hashlib.sha256(data).hexdigest()


def run(command, timeout, binary=False):
    return subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          text=not binary, timeout=timeout)


def run_revision(name, source, work, dotnet, compiler, refs, framework):
    target = work / (name + '.dll')
    command = [dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
               '/out:' + str(target)]
    command += ['/reference:' + str(path) for path in sorted(refs.glob('*.dll'))]
    command += [source, SOURCES[1], SOURCES[2]]
    compiled = run(command, 90)
    if compiled.returncode:
        return {'status': 'COMPILE_ERROR', 'error': (compiled.stdout + compiled.stderr).strip()}
    (work / (name + '.runtimeconfig.json')).write_text(json.dumps({
        'runtimeOptions': {'tfm': 'net8.0',
                           'framework': {'name': 'Microsoft.NETCore.App', 'version': framework}}
    }), encoding='utf-8')
    executed = run([dotnet, target], 30)
    cases = {}
    total = None
    failures = None
    for line in executed.stdout.splitlines():
        parts = line.split('\t', 3)
        if len(parts) == 4 and parts[0] == 'CASE':
            cases[parts[1]] = {'status': parts[2], 'detail': parts[3]}
        elif len(parts) == 3 and parts[0] == 'TOTAL':
            total, failures = int(parts[1]), int(parts[2])
    if total is None or len(cases) != 25:
        return {'status': 'RUN_ERROR', 'exitCode': executed.returncode,
                'output': (executed.stdout + executed.stderr).strip(), 'cases': cases}
    return {'status': 'PASS' if executed.returncode == 0 and failures == 0 else 'FAIL',
            'exitCode': executed.returncode, 'assertions': total, 'failures': failures,
            'cases': cases, 'output': executed.stdout.strip()}


def main():
    record = {'status': 'FAIL', 'baseline': BASELINE,
              'scope': 'Real BattleRecovery and FightEndJournal; stubbed Unity, PlayerPrefs, '
              'transport and protobuf codec', 'deviceExecuted': False, 'databaseAccess': False,
              'battleEngineExecuted': False, 'temporaryFilesCleaned': False}
    work = None
    try:
        before_hashes = {str(path.relative_to(ROOT)): digest(path.read_bytes()) for path in SOURCES}
        old = run(['git', 'show', BASELINE], 30, binary=True)
        if old.returncode:
            raise RuntimeError('Could not load fixed baseline BattleRecovery source')
        dotnet = RUNTIME / 'dotnet'
        compiler = next((RUNTIME / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
        refs = next((RUNTIME / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
        framework = refs.parents[1].name
        with tempfile.TemporaryDirectory(prefix='hyjs-battle-receipt-check-') as directory:
            work = Path(directory)
            old_source = work / 'BattleRecovery-before.cs'
            old_source.write_bytes(old.stdout)
            record['before'] = run_revision('ReceiptBefore', old_source, work, dotnet, compiler, refs, framework)
            record['after'] = run_revision('ReceiptAfter', SOURCES[0], work, dotnet, compiler, refs, framework)
        record['temporaryFilesCleaned'] = not work.exists()
        if before_hashes != {str(path.relative_to(ROOT)): digest(path.read_bytes()) for path in SOURCES}:
            raise RuntimeError('A source changed while the checks were running')
        record.update(sourceSHA256=before_hashes, baselineSourceSHA256=digest(old.stdout),
                      dotnet=str(dotnet), framework=framework)
        before_cases = record['before'].get('cases', {})
        baseline_reproduced = all(before_cases.get(name, {}).get('status') == 'FAIL' and
                                  'disabled-engine original result handler' in before_cases[name].get('detail', '')
                                  for name in DISABLED)
        record['baselineDisabledEngineFailureReproduced'] = baseline_reproduced
        if baseline_reproduced and record['after']['status'] == 'PASS':
            record['status'] = 'PASS'
    except Exception as error:
        if work is not None:
            record['temporaryFilesCleaned'] = not work.exists()
        record['error'] = str(error)
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'status': record['status'], 'before': record.get('before', {}).get('status'),
                      'after': record.get('after', {}).get('status'),
                      'afterFailures': [name for name, case in record.get('after', {}).get('cases', {}).items()
                                        if case['status'] == 'FAIL'],
                      'evidence': str(OUTPUT), 'error': record.get('error')}, ensure_ascii=False))
    return 0 if record['status'] == 'PASS' else 1


if __name__ == '__main__':
    raise SystemExit(main())
