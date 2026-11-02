"""Replay real cold-fight discovery and its connection fence with fake transport."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
SOURCES = [HERE / 'EncounterStartRecovery.cs', HERE / 'ColdBattleRecovery.cs',
           HERE / 'ColdBattleRecoveryChecks.cs', Path(__file__).resolve()]


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def invoke(command, timeout):
    result = subprocess.run(['rtk', 'proxy', *map(str, command)], capture_output=True,
                            text=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError('Command failed with exit %d\n%s%s' %
                           (result.returncode, result.stdout, result.stderr))
    return result.stdout


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runtime', type=Path, required=True, help='native-batch root')
    parser.add_argument('--record', type=Path, help='write scoped check evidence')
    args = parser.parse_args()
    record = {'status': 'FAIL', 'scope': 'Real cold and encounter recovery with stubbed Unity, '
              'transport, and discovery replies', 'deviceExecuted': False, 'databaseAccess': False,
              'battleEngineExecuted': False, 'temporaryFilesCleaned': False}
    work = None
    try:
        before = {str(path.relative_to(ROOT)): digest(path) for path in SOURCES}
        runtime = args.runtime.resolve() / 'dotnet'
        dotnet = runtime / 'dotnet'
        compiler = next((runtime / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
        refs = next((runtime / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
        framework = refs.parents[1].name
        with tempfile.TemporaryDirectory(prefix='hyjs-cold-recovery-check-') as directory:
            work = Path(directory)
            target = work / 'ColdBattleRecoveryChecks.dll'
            command = [dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
                       '/out:' + str(target)]
            command += ['/reference:' + str(path) for path in sorted(refs.glob('*.dll'))]
            command += SOURCES[:3]
            compile_output = invoke(command, 90)
            (work / 'ColdBattleRecoveryChecks.runtimeconfig.json').write_text(json.dumps({
                'runtimeOptions': {'tfm': 'net8.0',
                                   'framework': {'name': 'Microsoft.NETCore.App', 'version': framework}}
            }), encoding='utf-8')
            output = invoke([dotnet, target], 30)
        record['temporaryFilesCleaned'] = not work.exists()
        if before != {str(path.relative_to(ROOT)): digest(path) for path in SOURCES}:
            raise RuntimeError('A source changed while the checks were running')
        prefix = 'ColdBattleRecovery replay passed: '
        if not output.strip().startswith(prefix) or not output.strip().endswith(' assertions'):
            raise RuntimeError('Unexpected check output:\n' + output)
        assertions = int(output.strip()[len(prefix):-len(' assertions')])
        record.update(status='PASS', assertions=assertions, sourceSHA256=before,
                      dotnet=str(dotnet), framework=framework, compileOutput=compile_output.strip(),
                      output=output.strip())
    except Exception as error:
        if work is not None:
            record['temporaryFilesCleaned'] = not work.exists()
        record['error'] = str(error)
    if args.record:
        args.record.parent.mkdir(parents=True, exist_ok=True)
        args.record.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(record.get('output') or record.get('error'))
    return 0 if record['status'] == 'PASS' else 1


if __name__ == '__main__':
    raise SystemExit(main())
