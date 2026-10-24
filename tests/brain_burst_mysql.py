#!/usr/bin/env python3
"""Exclusive, managed three-schema Brain Burst financial verification.

No root access, production/dev schema, service startup, or operational awards.
The Go tests use finite isolated fixtures; all three schemas share one pool user.
"""
import argparse
import fcntl
import json
import os
from pathlib import Path
import subprocess
import sys
import time

from hyjs_managed_mysql import ManagedMySQL

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--case', choices=['contract', 'native', 'all'], default='all')
    parser.add_argument('--output', type=Path, default=ROOT / 'analysis/brain-burst-release-20261025')
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    # The shared lease must be acquired before allocating or touching any MySQL schema.
    lease = open('/tmp/hyjs-isolated-mysql.lock', 'a+')
    try:
        fcntl.flock(lease, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        lease.close()
        print('Shared MySQL lease is held by another session; no allocations or tests started.')
        return 75
    report = {'startedAt': time.strftime('%Y-%m-%dT%H:%M:%S%z'), 'cases': []}
    status = 0
    try:
        cases = ['contract', 'native'] if args.case == 'all' else [args.case]
        for case in cases:
            pool = ManagedMySQL()
            item = {'case': case, 'schemas': [], 'released': False}
            report['cases'].append(item)
            try:
                first = pool.allocate('p1_bb_' + case)
                shared = pool.allocate('bb_accounts')
                second = pool.allocate('bb_realm2')
                item['schemas'] = [first.database, shared.database, second.database]
                env = dict(os.environ, **first.env())
                env.update({
                    'HYJS_BB_MYSQL_LEASE': 'managed-three-schema',
                    'HYJS_TEST_ACCOUNTS_SCHEMA': shared.database,
                    'HYJS_TEST_SECOND_REALM_SCHEMA': second.database,
                    'HYJS_P1_MYSQL_HOST': first.host,
                    'HYJS_P1_MYSQL_USER': first.user,
                    'HYJS_P1_MYSQL_PASSWORD': first.password,
                    'HYJS_P1_MYSQL_DATABASE': first.database,
                })
                selector = '^TestBrainBurstMySQLThreeSchemaRollbackAndConcurrency$' if case == 'contract' else '^TestGlobalAccountsThreeSchemaRealms$'
                command = ['rtk', 'proxy', 'go', 'test', './service', '-run', selector, '-count=1', '-v', '-timeout', '150s']
                started = time.monotonic()
                result = subprocess.run(command, cwd=ROOT / 'hyjs-server', env=env, capture_output=True, text=True, timeout=180)
                item.update({'selector': selector, 'exitCode': result.returncode, 'seconds': round(time.monotonic() - started, 3)})
                text = pool.redact(result.stdout + result.stderr)
                log = args.output / ('mysql-' + case + '.log')
                log.write_text(text)
                item['log'] = str(log.resolve())
                print(case + ': exit=' + str(result.returncode) + ' log=' + str(log))
                if result.returncode:
                    print(text[-2200:])
                    status = result.returncode
            except Exception as error:
                item['error'] = pool.redact(str(error))
                status = 1
                print(case + ': ' + item['error'])
            finally:
                pool.close()
                item['released'] = not pool.connections
            if status:
                break
    finally:
        report['finishedAt'] = time.strftime('%Y-%m-%dT%H:%M:%S%z')
        (args.output / 'mysql-verification.json').write_text(json.dumps(report, ensure_ascii=False, indent=2))
        fcntl.flock(lease, fcntl.LOCK_UN)
        lease.close()
    return status


if __name__ == '__main__':
    sys.exit(main())
