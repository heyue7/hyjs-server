"""Replay one pinned existing family with the already compiled stable runtime.

No compile, SDK acquisition, source patch, Go, SQL or device operation. Each
family supplies only immutable existing PB inputs and diagnostic sidecars.
"""
from pathlib import Path
import argparse
import json
import os
import shutil
import subprocess
import time
from case_sets import ROOT, SETS, prepare, sha
from run import STABLE_HOST_ENV

BASE = ROOT / 'analysis/growth-first-20261016/p6-local-headless-p7-runtime-r1'
BASE_DELIVERY_SHA = '07e85496d17a40a3a42cddbb6651cac78c954b3762ab8216ee2d5e311f49c9a6'
TABLE_PROOF = ROOT / 'analysis/growth-first-20261016/p6-local-headless-p7-r1/attempt-2-managed-logger/manifest.json'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--case-set', choices=sorted(SETS), required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    output = args.output_directory.resolve()
    if output.exists():
        raise FileExistsError('Preserve fixed source/output evidence')
    if sha(BASE / 'delivery.json') != BASE_DELIVERY_SHA:
        raise ValueError('Fixed stable runtime proof changed')
    output.mkdir(parents=True)
    work = output / 'runtime'
    work.mkdir()
    baseline = BASE / 'runtime'
    reference = json.loads((BASE / 'manifest.json').read_bytes())
    for path in baseline.glob('*.dll'):
        if sha(path) != reference['runtimeInputPins'][path.name]:
            raise ValueError('Previously compiled runtime DLL changed: ' + str(path))
        shutil.copy2(path, work / path.name)
    shutil.copy2(baseline / 'LocalOriginalEngineHost.runtimeconfig.json', work / 'LocalOriginalEngineHost.runtimeconfig.json')
    shutil.copytree(baseline / 'Assets', work / 'Assets')
    table_reference = json.loads(TABLE_PROOF.read_bytes())['originalRawTablePins']
    table_pins = {}
    for file in (work / 'Assets').rglob('*.txt'):
        source = ROOT / 'analysis/tables' / file.name.removesuffix('.txt').lower()
        if not source.exists():
            source = ROOT / 'analysis/tables' / file.name.replace('.txt', '.tsv')
        expected = table_reference[str(source.relative_to(ROOT))]['sha256']
        if sha(source) != expected or sha(file) != expected:
            raise ValueError('Original162 table differs')
        table_pins[str(file.relative_to(work))] = expected
    if len(table_pins) != 162:
        raise ValueError('Original table denominator differs')
    report = {'schema': 'local-original-engine-existing-family-replay-v1', 'status': 'STARTED',
              'executionOrigin': 'local-coreclr-original-engine-not-phone-capture',
              'caseSet': args.case_set, 'baselineDeliverySHA256': BASE_DELIVERY_SHA,
              'sourcePins': {str(p.relative_to(ROOT)): sha(p) for p in Path(__file__).parent.glob('*.*') if p.is_file()},
              'runtimeEnvironment': dict(STABLE_HOST_ENV), 'runtimeDLLPins': dict(reference['runtimeInputPins']),
              'originalRaw162TablePins': table_pins,
              'original547DAOAndCallerUnchanged': True, 'nativeCompared': False, 'authoritative': False,
              'GoSQLPhoneOrCompilationExecuted': False, 'full13FamilyGoMatrixExecuted': False}
    snapshots = output / 'execution-source'
    snapshots.mkdir()
    for path in Path(__file__).parent.glob('*.*'):
        if path.is_file():
            shutil.copy2(path, snapshots / path.name)
    report['caseSets'] = prepare(args.case_set, work)
    report['executionSeconds'] = {}
    try:
        for group in report['caseSets']:
            command = [str(ROOT / '.tools/native-batch/dotnet/dotnet'), str(work / 'LocalOriginalEngineHost.dll'), str(work), str(work / group['caseDirectory'])]
            started = time.monotonic()
            result = subprocess.run(command, capture_output=True, text=True, timeout=120,
                                    env={**os.environ, **STABLE_HOST_ENV})
            (output / ('execute-' + group['set'] + '.log')).write_text(result.stdout + result.stderr)
            report['executionSeconds'][group['set']] = time.monotonic() - started
            if result.returncode:
                raise RuntimeError('Original host exit ' + str(result.returncode))
        report['status'] = 'EXECUTED_PENDING_STRICT_COMPARE'
    except subprocess.TimeoutExpired as error:
        decode = lambda value: value.decode(errors='replace') if isinstance(value, bytes) else value or ''
        (output / 'timeout.log').write_text(decode(error.stdout) + decode(error.stderr))
        report['status'] = 'FAIL'
        report['error'] = '120-second bounded host timeout'
    except Exception as error:
        report['status'] = 'FAIL'
        report['error'] = str(error)
    report['outputPins'] = {str(p.relative_to(output)): sha(p) for p in (work / 'cases').rglob('*.native.jsonl')}
    (output / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'status': report['status'], 'seconds': report['executionSeconds']}, ensure_ascii=False))
    return 0 if report['status'].startswith('EXECUTED') else 1


if __name__ == '__main__':
    raise SystemExit(main())
