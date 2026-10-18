"""Run complete Go verification with bounded timing evidence, without raw log files."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]


def run_phase(command, directory, environment, phase):
    results, failure_tail, test_output = [], [], {}
    process = subprocess.Popen(['rtk', 'proxy', *command], cwd=directory,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                               text=True, env=environment)
    for line in process.stdout:
        try:
            row = json.loads(line)
        except json.JSONDecodeError:
            failure_tail.append(line.strip()[:1500])
            failure_tail = failure_tail[-40:]
            continue
        action, test = row.get('Action'), row.get('Test', '')
        if action == 'build-output':
            # Go结构化编译诊断不属于测试output；保留真正错误，避免失败记录只剩build failed。
            failure_tail.append(row.get('Output', '').strip()[:1500])
            failure_tail = failure_tail[-40:]
        if action == 'output' and test:
            lines = test_output.setdefault(test, [])
            lines.append(row.get('Output', '').strip()[:1500])
            del lines[:-15]
        if action == 'fail' and test:
            failure_tail.extend(test_output.get(test, []))
            failure_tail = failure_tail[-40:]
        if action in ('pass', 'skip', 'fail') and test:
            test_output.pop(test, None)
        if action in ('pass', 'fail', 'skip') and '/' not in test:
            result = {key: row[key] for key in ('Action', 'Package', 'Test', 'Elapsed') if key in row}
            result['phase'] = phase
            results.append(result)
            if not test or action == 'fail' or row.get('Elapsed', 0) >= 10:
                print(json.dumps(result), flush=True)
        if action == 'output' and any(term in row.get('Output', '') for term in ('panic:', 'FAIL', 'Error:')):
            failure_tail.append(row['Output'].strip()[:1500])
            failure_tail = failure_tail[-40:]
    return process.wait(), results, failure_tail


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--candidate', type=Path, required=True)
    parser.add_argument('--record', type=Path, required=True)
    parser.add_argument('--resume-timeout-record', type=Path,
                        help='Continue only a timed-out immutable candidate, retaining prior completed tests')
    parser.add_argument('--go-timeout', default='30m', choices=('20m', '30m'),
                        help='Bound a growing complete suite without restarting after predictable20m cutoff')
    args = parser.parse_args()
    candidate = args.candidate.resolve()
    record = args.record.resolve()
    if candidate.parent != ROOT / 'build/candidates' or not record.is_relative_to(ROOT / 'analysis/p3'):
        raise RuntimeError('Use an owned fixed candidate and verification record')
    manifest = json.loads((candidate / 'manifest.json').read_text())
    if manifest.get('stable'):
        raise RuntimeError('Do not retest an already verified candidate')
    started = time.monotonic()
    results = []
    failure_tail = []
    environment = dict(os.environ)
    environment.setdefault('GOMEMLIMIT', '2GiB')
    # Stop scheduling further tests after the first failure so the owning
    # developer gets feedback before another full suite of slow goldens runs.
    # A successful run still executes the complete suite.
    command = ['go', 'test', '-json', '-failfast', '-timeout', args.go_timeout, './...']
    # Legacy protocol/replay semantics are cheap and should fail before the
    # native golden corpus. The complete suite still runs after this check.
    preflight = ['go', 'test', '-json', '-failfast', '-run',
                 '^TestP6Original(PVERecord|Protocol|UF14)', '-timeout', '3m', './service']
    phases = []
    resume = None
    invocations = [('legacy-replay-preflight', preflight), ('complete', command)]
    if args.resume_timeout_record:
        previous_path = args.resume_timeout_record.resolve()
        previous_bytes = previous_path.read_bytes()
        previous = json.loads(previous_bytes)
        if (not previous_path.is_relative_to(ROOT / 'analysis/p3') or previous_path == record
                or previous.get('candidate') != str(candidate.relative_to(ROOT))
                or previous.get('status') != 'failed'
                or not any('test timed out after' in line for line in previous.get('failureTail', []))
                or any(row.get('Action') == 'fail' and row.get('Test') for row in previous['topLevelResults'])):
            raise RuntimeError('Only an unchanged timeout with no test assertion failure can resume')
        for row in manifest['files']:
            with (candidate / row['path']).open('rb') as source:
                if hashlib.file_digest(source, 'sha256').hexdigest() != row['sha256']:
                    raise RuntimeError('Timed-out candidate changed: ' + row['path'])
        # A completed test is reusable only for this exact fixed source. The
        # interrupted test has no pass row and therefore always runs again.
        results = [row for row in previous['topLevelResults']
                   if row.get('phase') == 'complete' and row.get('Action') in ('pass', 'skip')]
        completed = sorted({row['Test'] for row in results
                            if row.get('Package') == 'hyjs-server/service' and row.get('Test')})
        if not completed:
            raise RuntimeError('Timeout record has no completed service tests to reuse')
        invocation = command[:-1] + ['-skip', '^(' + '|'.join(map(re.escape, completed)) + ')$', './...']
        invocations = [('timeout-remainder', invocation)]
        resume = {'record': str(previous_path.relative_to(ROOT)),
                  'recordSHA256': hashlib.sha256(previous_bytes).hexdigest(),
                  'reusedServiceTests': len(completed), 'allCandidateSHA256Verified': True,
                  'previousElapsedSeconds': previous['elapsedSeconds']}
    for phase, invocation in invocations:
        phase_started = time.monotonic()
        code, rows, failure_tail = run_phase(invocation, candidate / 'hyjs-server', environment, phase)
        results.extend(rows)
        phases.append({'phase': phase, 'command': ' '.join(invocation), 'exitCode': code,
                       'elapsedSeconds': round(time.monotonic() - phase_started, 3)})
        if code:
            break
    report = {'candidate': str(candidate.relative_to(ROOT)), 'command': ' '.join(command),
              'goSoftMemoryLimit': environment['GOMEMLIMIT'], 'phases': phases,
              'elapsedSeconds': round(time.monotonic() - started, 3), 'exitCode': code,
              'status': 'passed' if code == 0 else 'failed', 'topLevelResults': results,
              'slowestTests': sorted((r for r in results if r.get('Test')),
                                    key=lambda r: r.get('Elapsed', 0), reverse=True)[:20]}
    if resume:
        report['resumedTimeout'] = resume
        report['scope'] = 'Complete package coverage in two immutable-source runs; prior passes retained, interrupted and unrun tests executed'
    if code:
        report['failureTail'] = failure_tail
    record.parent.mkdir(parents=True, exist_ok=True)
    record.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'status': report['status'], 'elapsedSeconds': report['elapsedSeconds'],
                      'record': str(record.relative_to(ROOT))}), flush=True)
    raise SystemExit(code)


if __name__ == '__main__':
    main()
