"""One bounded runtime-only probe of the two existing long Trial9 cases.

Reuses the already compiled original caller/game/DAO/Unity adapter and host.
No compilation, SDK downloads, Go, SQL, phone or battle-source changes.
"""
from pathlib import Path
import argparse
import hashlib
import json
import os
import shutil
import subprocess
import time
from case_sets import ROOT, SETS, pinned, prepare, sha

PRIOR = ROOT / 'analysis/growth-first-20261016/p6-local-headless-p7-r1/attempt-2-managed-logger'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    output = args.output_directory.resolve()
    if output.exists():
        raise FileExistsError('Preserve immutable diagnostic evidence')
    output.mkdir(parents=True)
    work = output / 'runtime'
    work.mkdir()
    baseline = PRIOR / 'runtime'
    report = {'schema': 'local-original-engine-runtime-probe-v1', 'status': 'STARTED',
              'executionOrigin': 'local-coreclr-original-engine-not-phone-capture',
              'sourceRuntimeManifest': str((PRIOR / 'manifest.json').relative_to(ROOT)),
              'sourceRuntimeManifestSHA256': sha(PRIOR / 'manifest.json'),
              'sourcePins': {str(p.relative_to(ROOT)): sha(p) for p in [Path(__file__).resolve(), Path(__file__).parent / 'case_sets.py']},
              'runtimeEnvironment': {'COMPlus_TieredCompilation': '0', 'COMPlus_JitNoInline': '1', 'COMPlus_JITMinOpts': '1'},
              'runtimeRationale': 'Disable tier transitions/inlining; minimum optimization retains StackTrace-based original-caller origin frames. No IL rewrite.',
              'nativeCompared': False, 'authoritative': False, 'full13FamilyGoMatrixExecuted': False,
              'caseSet': 'p7-rush-existing-long-only', 'originalEngineOrCallerModified': False}
    for path in baseline.glob('*.dll'):
        shutil.copy2(path, work / path.name)
    shutil.copytree(baseline / 'Assets', work / 'Assets')
    config = json.loads((baseline / 'LocalOriginalEngineHost.runtimeconfig.json').read_bytes())
    config['runtimeOptions']['configProperties'] = {'System.Runtime.TieredCompilation': False}
    (work / 'LocalOriginalEngineHost.runtimeconfig.json').write_text(json.dumps(config, indent=2) + '\n')
    groups = prepare('p7-rush', work)
    group = groups[0]
    selected = [row for row in group['cases'] if Path(row['localWire']).stem in {'002', '003'}]
    if len(selected) != 2:
        raise ValueError('Exactly the two existing long cases required')
    case_dir = work / group['caseDirectory']
    for p in case_dir.iterdir():
        if p.name[:3] in {'000', '001'}:
            p.unlink()
    group['cases'] = selected
    report['caseSets'] = [group]
    report['runtimeInputPins'] = {p.name: sha(p) for p in work.glob('*.dll')}
    assert all(sha(p) == sha(baseline / p.name) for p in work.glob('*.dll'))
    runtime = ROOT / '.tools/native-batch/dotnet/shared/Microsoft.NETCore.App/8.0.32'
    jit = (runtime / 'libclrjit.dylib').read_bytes()
    core = (runtime / 'libcoreclr.dylib').read_bytes()
    assert b'JitNoInline' in jit and b'TieredCompilation' in core and b'JITMinOpts' in core
    report['runtimeNativePins'] = {str((runtime / n).relative_to(ROOT)): sha(runtime / n) for n in ['libclrjit.dylib', 'libcoreclr.dylib']}
    command = [str(ROOT / '.tools/native-batch/dotnet/dotnet'), str(work / 'LocalOriginalEngineHost.dll'), str(work), str(case_dir)]
    started = time.monotonic()
    try:
        result = subprocess.run(command, capture_output=True, text=True, timeout=120,
                                env={**os.environ, **report['runtimeEnvironment']})
        (output / 'execute.log').write_text(result.stdout + result.stderr)
        report['exitCode'] = result.returncode
        report['status'] = 'EXECUTED_PENDING_STRICT_COMPARE' if result.returncode == 0 else 'FAIL'
    except subprocess.TimeoutExpired as error:
        decode = lambda value: value.decode(errors='replace') if isinstance(value, bytes) else value or ''
        (output / 'execute.log').write_text(decode(error.stdout) + decode(error.stderr) + '\nHOST_TIMEOUT_SECONDS=120\n')
        report['status'] = 'FAIL'
        report['error'] = '120-second bounded original-host timeout'
    report['seconds'] = time.monotonic() - started
    report['outputPins'] = {str(p.relative_to(output)): sha(p) for p in case_dir.glob('*.native.jsonl')}
    report['logSHA256'] = sha(output / 'execute.log')
    (output / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({k: report[k] for k in ['status', 'seconds', 'runtimeEnvironment']}, ensure_ascii=False))
    return 0 if report['status'].startswith('EXECUTED') else 1


if __name__ == '__main__':
    raise SystemExit(main())
