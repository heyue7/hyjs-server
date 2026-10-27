"""Run each activity77 transaction representative on its own managed schema."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tests'))
from hyjs_managed_mysql import ManagedMySQL


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    binary = args.binary.resolve()
    output = args.output.resolve()
    if not binary.is_relative_to(ROOT / 'build') or not output.is_relative_to(ROOT / 'analysis/activity-growth77'):
        parser.error('Use this task owned build and evidence paths')
    output.mkdir(parents=True, exist_ok=False)
    listing = subprocess.run(['rtk', 'proxy', str(binary), '-test.list', '^Test(Activity77|P8Town77|GGOAccountWeekly73MySQLConcurrent)'], capture_output=True, text=True, check=True)
    # The phone fixture is an opt-in long-lived UI environment with its own
    # resource owner; ordinary transaction validation must not invoke it.
    cases = [line for line in listing.stdout.splitlines()
             if line.startswith('Test') and line != 'TestActivity77PhoneFixture']
    if len(cases) < 7:
        raise RuntimeError('Missing compiled activity representatives')
    pool = ManagedMySQL()
    results = []
    started = time.monotonic()
    try:
        for index, name in enumerate(cases):
            connection = pool.allocate('p1_activity77_' + str(index))
            env = dict(os.environ, GOMAXPROCS='2', GOMEMLIMIT='768MiB',
                       HYJS_P1_MYSQL_HOST=connection.host, HYJS_P1_MYSQL_USER=connection.user,
                       HYJS_P1_MYSQL_PASSWORD=connection.password, HYJS_P1_MYSQL_DATABASE=connection.database,
                       HYJS_ACTIVITY77_FORM_SNAPSHOT=str(ROOT / 'analysis/activity-growth77/live-period-forms.json'))
            began = time.monotonic()
            run = subprocess.run(['rtk', 'proxy', str(binary), '-test.v', '-test.run', '^' + name + '$', '-test.timeout', '3m'],
                                 cwd=ROOT / 'hyjs-server/service', env=env, text=True, capture_output=True, timeout=200)
            log = pool.redact(run.stdout + run.stderr)
            (output / (str(index) + '.log')).write_text(log)
            passed = run.returncode == 0 and '--- PASS: ' + name in log and '--- SKIP: ' + name not in log
            record = {'test': name, 'pass': passed, 'exitCode': run.returncode, 'elapsedSeconds': round(time.monotonic() - began, 3)}
            results.append(record)
            print(json.dumps(record), flush=True)
            pool.release(connection.database)
            if not passed:
                break
    finally:
        pool.close()
        report = {'binarySHA256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'expectedCases': cases, 'results': results,
                  'pass': len(results) == len(cases) and all(row['pass'] for row in results),
                  'ownedAllocationsReleased': not bool(pool.connections), 'elapsedSeconds': round(time.monotonic() - started, 3)}
        (output / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
    if not report['pass']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
