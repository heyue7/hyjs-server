"""Compile and check the real awake-entry wrapper using minimal native stubs."""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
RUNTIME = Path('/Users/lirui/LR/hyjs/.tools/native-batch/dotnet')
OUTPUT = ROOT / 'analysis/round-two-performance-final/client-awake-model.json'
SOURCES = [HERE / 'AwakeStartRecovery.cs', HERE / 'AwakeStartRecoveryChecks.cs', Path(__file__).resolve()]
EXPECTED = [
    'http_wrapper_and_missing_owner',
    'duplicate_start_blocked',
    'timeout_and_late_callback',
    'callback_deadline_without_iterator',
    'failure_paths',
    'successful_native_and_iterator_once',
    'successful_reply_external_scope_change',
    'iterator_resource_failure',
    'playback_without_http',
    'stale_scopes_cannot_clear_new_request',
]


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def invoke(arguments, timeout):
    result = subprocess.run(['rtk', 'proxy', *map(str, arguments)], capture_output=True,
                            text=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError('Command failed with exit %d\n%s%s' %
                           (result.returncode, result.stdout, result.stderr))
    return result.stdout


def main():
    record = {'status': 'FAIL', 'scope': 'Real AwakeStartRecovery with stubbed native UI, '
              'HTTP transport, and original coroutine', 'deviceExecuted': False,
              'databaseAccess': False, 'battleEngineExecuted': False, 'cases': {},
              'temporaryFilesCleaned': False}
    work = None
    try:
        before = {str(path.relative_to(ROOT)): digest(path) for path in SOURCES}
        dotnet = RUNTIME / 'dotnet'
        compiler = next((RUNTIME / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
        refs = next((RUNTIME / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
        framework = refs.parents[1].name
        with tempfile.TemporaryDirectory(prefix='hyjs-awake-start-check-') as directory:
            work = Path(directory)
            target = work / 'AwakeStartRecoveryChecks.dll'
            command = [dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
                       '/out:' + str(target)]
            command += ['/reference:' + str(path) for path in sorted(refs.glob('*.dll'))]
            command += SOURCES[:2]
            compile_output = invoke(command, 90)
            (work / 'AwakeStartRecoveryChecks.runtimeconfig.json').write_text(json.dumps({
                'runtimeOptions': {'tfm': 'net8.0',
                                   'framework': {'name': 'Microsoft.NETCore.App', 'version': framework}}
            }), encoding='utf-8')
            output = invoke([dotnet, target], 30)
        record['temporaryFilesCleaned'] = not work.exists()
        after = {str(path.relative_to(ROOT)): digest(path) for path in SOURCES}
        if before != after:
            raise RuntimeError('A source changed while the checks were running')
        case_lines = [line.split('\t') for line in output.splitlines() if line.startswith('CASE\t')]
        if any(len(line) != 3 for line in case_lines) or [line[1] for line in case_lines] != EXPECTED:
            raise RuntimeError('Unexpected case output:\n' + output)
        if not output.splitlines()[-1].startswith('PASS\t'):
            raise RuntimeError('Missing final assertion count:\n' + output)
        total = int(output.splitlines()[-1].split('\t')[1])
        record.update(status='PASS', cases={line[1]: int(line[2]) for line in case_lines},
                      assertions=total, sourceSHA256=before, dotnet=str(dotnet), framework=framework,
                      compileOutput=compile_output.strip(), output=output.strip())
    except Exception as error:
        if work is not None:
            record['temporaryFilesCleaned'] = not work.exists()
        record['error'] = str(error)
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'status': record['status'], 'assertions': record.get('assertions'),
                      'evidence': str(OUTPUT), 'error': record.get('error')}, ensure_ascii=False))
    return 0 if record['status'] == 'PASS' else 1


if __name__ == '__main__':
    raise SystemExit(main())
