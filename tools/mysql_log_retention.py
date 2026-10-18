"""Reclaim this project's unused MySQL binlogs after a consistent owned backup."""
import fcntl
import json
import os
from pathlib import Path
import re
import subprocess
import time

from release_go import ROOT, sql, state


def main():
    raise RuntimeError('Disabled for shared local MySQL: binlog maintenance belongs to the shared-service operator; no SQL was issued')
    database = 'hyjs'
    # Coordinate with the same lock as every isolated MySQL verifier. The
    # production account bytes and current active binary log remain preserved.
    with open('/tmp/hyjs-isolated-mysql.lock', 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if sql('SHOW REPLICA STATUS;', database):
            raise RuntimeError('Replication has consumers; retain their logs')
        before = state(database)
        previous = sql('SHOW BINARY LOGS;', database).splitlines()
        if not previous:
            raise RuntimeError('No enabled binary log to retain')
        sql('FLUSH BINARY LOGS;', database)
        current = sql('SHOW BINARY LOGS;', database).splitlines()[-1].split('\t')[0]
        if not re.fullmatch(r'binlog\.\d{6}', current):
            raise RuntimeError('Unknown local binary log filename')
        stamp = time.strftime('%Y%m%d-%H%M%S')
        backup = ROOT / 'server/data/backups' / ('pre-binlog-retention-' + stamp + '.sql')
        fd = os.open(backup, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'wb') as output:
            result = subprocess.run(
                ['rtk', 'proxy', 'docker', 'exec', 'hyjs-mysql', 'sh', '-c',
                 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysqldump -uroot --single-transaction --routines --triggers "$1"',
                 'retention', database], stdout=output, stderr=subprocess.PIPE, timeout=90)
        if result.returncode or not backup.stat().st_size:
            raise RuntimeError('Owned consistent backup failed; retain all logs')
        # Keep development change history bounded. InnoDB crash recovery and
        # the full SQL restore source remain independent of binary log cleanup.
        sql('SET PERSIST binlog_expire_logs_seconds=7200; PURGE BINARY LOGS TO "' + current + '";', database)
        remaining = sql('SHOW BINARY LOGS;', database).splitlines()
        if not remaining or remaining[0].split('\t')[0] != current:
            raise RuntimeError('Retained current binary log was not verified')
        after = state(database)
        if before['original'] != after['original'] or before['demo'] != after['demo']:
            raise RuntimeError('Protected account state changed during log retention')
        prior_bytes = sum(int(row.split('\t')[1]) for row in previous)
        remaining_bytes = sum(int(row.split('\t')[1]) for row in remaining)
        record = {'database': database, 'backup': str(backup), 'backupBytes': backup.stat().st_size,
                  'retainedFrom': current, 'removedLogs': len(previous),
                  'reclaimedGuestBytes': prior_bytes - remaining_bytes, 'retentionSeconds': 7200,
                  'replicationConsumers': 0, 'originalSaveVerified': True, 'before': before, 'after': after}
        report = ROOT / 'analysis/p15' / ('mysql-binlog-retention-' + stamp + '.json')
        report.write_text(json.dumps(record, indent=2) + '\n')
        print(json.dumps({'removedLogs': len(previous), 'reclaimedGuestBytes': record['reclaimedGuestBytes'],
                          'backup': str(backup), 'retainedFrom': current, 'originalSaveVerified': True}))


if __name__ == '__main__':
    main()
