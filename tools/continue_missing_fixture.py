"""Continue a fixed Go candidate after adding only its missing native fixtures."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import time

from test_candidate import ROOT, run_phase


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--candidate', type=Path, required=True)
    parser.add_argument('--previous', type=Path, required=True)
    parser.add_argument('--repair', type=Path, required=True)
    parser.add_argument('--record', type=Path, required=True)
    args = parser.parse_args()
    candidate = args.candidate.resolve()
    previous_path, repair_path, record = (p.resolve() for p in
                                         (args.previous, args.repair, args.record))
    if candidate.parent != ROOT / 'build/candidates' or any(
            not p.is_relative_to(ROOT / 'analysis/p3') for p in
            (previous_path, repair_path, record)) or record == previous_path:
        raise RuntimeError('Use the owned candidate and separate P3 evidence records')
    previous = json.loads(previous_path.read_text())
    repair = json.loads(repair_path.read_text())
    baseline_path = (ROOT / repair['beforeManifest']).resolve()
    if (not baseline_path.is_relative_to(ROOT / 'analysis/p3') or
            digest(baseline_path) != repair['beforeManifestSHA256']):
        raise RuntimeError('The original fixed candidate manifest is required')
    baseline = json.loads(baseline_path.read_text())
    current = json.loads((candidate / 'manifest.json').read_text())
    if (current.get('stable') or previous.get('status') != 'failed' or
            previous.get('candidate') != str(candidate.relative_to(ROOT))):
        raise RuntimeError('Only the failed fixed candidate can continue')
    tail = previous.get('failureTail', [])
    missing = [line for line in tail if 'no such file or directory' in line]
    if (not missing or any('panic:' in line for line in tail) or
            any(re.search(r'\.go:\d+:', line) and
                'no such file or directory' not in line for line in tail)):
        raise RuntimeError('Continuation is restricted to absent native fixture failures')
    failed = {r['Test'] for r in previous['topLevelResults']
              if r.get('Action') == 'fail' and r.get('Test')}
    if failed != set(repair['failedTests']) or any(
            not name.startswith('TestP6Native') for name in failed):
        raise RuntimeError('Every failed test must be explicitly identified')
    old = {r['path']: r['sha256'] for r in baseline['files']}
    new = {r['path']: r['sha256'] for r in current['files']}
    added = repair['addedFiles']
    if (not added or set(new) != set(old) | set(added) or set(old) & set(added)
            or any(new.get(p) != sha for p, sha in old.items())):
        raise RuntimeError('Existing pinned source must remain entirely unchanged')
    for name, sha in added.items():
        if (not name.startswith('analysis/p6/') or '..' in Path(name).parts or
                Path(name).suffix not in ('.json', '.pb', '.bin', '.gz', '.txt') or
                new[name] != sha):
            raise RuntimeError('Only explicitly pinned native fixture additions are allowed')
    for line in missing:
        match = re.search(r'open ../../(analysis/p6/[^:]+): no such file', line)
        if not match or match.group(1) not in added:
            raise RuntimeError('Each missing fixture must be present in the pinned additions')
    for name, sha in new.items():
        if digest(candidate / name) != sha:
            raise RuntimeError('Candidate SHA changed: ' + name)
    completed = [r for r in previous['topLevelResults'] if
                 r.get('phase') == 'complete' and r.get('Action') in ('pass', 'skip')]
    names = sorted({r['Test'] for r in completed if
                    r.get('Package') == 'hyjs-server/service' and r.get('Test')})
    if not names or failed & set(names):
        raise RuntimeError('Retain only completed independent tests; rerun every failure')
    command = ['go', 'test', '-json', '-failfast', '-timeout', '20m',
               '-skip', '^(' + '|'.join(map(re.escape, names)) + ')$', './...']
    environment = dict(os.environ)
    environment.setdefault('GOMEMLIMIT', '2GiB')
    start = time.monotonic()
    code, rows, failure = run_phase(command, candidate / 'hyjs-server',
                                    environment, 'fixture-remainder')
    result = {'candidate': str(candidate.relative_to(ROOT)),
              'command': ' '.join(command), 'status': 'passed' if code == 0 else 'failed',
              'exitCode': code, 'elapsedSeconds': round(time.monotonic() - start, 3),
              'goSoftMemoryLimit': environment['GOMEMLIMIT'],
              'topLevelResults': completed + rows,
              'continuation': {'previousRecord': str(previous_path.relative_to(ROOT)),
                               'previousRecordSHA256': digest(previous_path),
                               'repairRecord': str(repair_path.relative_to(ROOT)),
                               'repairRecordSHA256': digest(repair_path),
                               'unchangedOriginalPinnedFiles': len(old),
                               'addedNativeFixtures': len(added),
                               'reusedCompletedServiceTests': len(names),
                               'allPinnedSHA256Verified': True,
                               'scope': 'Complete package coverage across original run and missing-fixture remainder; no existing source or fixture changed'}}
    if code:
        result['failureTail'] = failure
    record.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps({k: result[k] for k in ('status', 'elapsedSeconds', 'exitCode')}), flush=True)


if __name__ == '__main__':
    main()
