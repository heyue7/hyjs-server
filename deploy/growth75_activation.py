"""Audited maintenance-only growth75 data activation, called by activate.py.

The MySQL callback executes a whole SQL document in one connection, without
--force. A failed CAS closes that connection before COMMIT, rolling its current
transaction back; prior explicit CLI steps stay audited under maintenance.
"""
from pathlib import Path
import hashlib
import json
import os
import subprocess
import time


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def private_write(path, raw):
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600), 'w', encoding='utf-8') as stream:
        stream.write(raw)
    os.chmod(path, 0o600)



def bind_schedule_revision(sql, revision):
    assert isinstance(revision, int) and 0 <= revision < 9223372036854775807
    assert sql.count('COMMIT;') == 1 and 'START TRANSACTION;' in sql
    guard = (f"UPDATE admin_activity_schedule_revisions SET revision=revision+1 WHERE server_id=1 AND revision={revision};\n"
             "SET @growth75_calendar_changed=ROW_COUNT();\n"
             "INSERT INTO growth75_activity_assert(ok) VALUES(@growth75_calendar_changed=1);\n"
             "INSERT INTO admin_audits(time,actor,action,target,detail) VALUES(UNIX_TIMESTAMP(NOW(3))*1000,'authorized-ops','growth75.schedule.revision','1',"
             f"'calendar CAS revision {revision} to {revision+1}; reject stale admin schedule pages');\n")
    return sql.replace('COMMIT;', guard + 'COMMIT;', 1)


def activate_growth75(mysql, compose, patch_script, backup_dir, revision, backup_record):
    assert backup_record and backup_record.get('file') and backup_record.get('sha256'), 'Growth75 requires a verified private encrypted database backup'
    assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Growth75 requires maintenance'
    assert patch_script.is_file() and backup_dir.is_dir() and not backup_dir.is_symlink()
    directory = backup_dir / ('growth75-' + revision[:12] + '-' + str(time.time_ns()))
    directory.mkdir(mode=0o700)
    record = {'schema': 'growth75-activation-v1', 'revision': revision, 'backupBefore': backup_record,
              'state': 'started', 'maintenanceRequired': True, 'files': {}}

    def checkpoint():
        private_write(directory / 'manifest.json', json.dumps(record, ensure_ascii=False, indent=2) + '\n')

    def script(*args):
        result = subprocess.run(['python3', str(patch_script), *map(str, args)], capture_output=True, text=True, encoding='utf-8')
        if result.returncode:
            raise RuntimeError('Activity generator failed: ' + result.stderr[-5000:])
        return result.stdout

    checkpoint()
    try:
        private_write(directory / 'before-policy.tsv', mysql('SELECT id,maintenance,latest_build,min_login_build,min_run_build,version_name FROM client_update_policy WHERE id=1;'))
        private_write(directory / 'publication-proof.tsv', mysql('SELECT id,time,action,target,detail FROM admin_audits WHERE id=321;'))
        # The candidate runs before export: its catalogue/CAS edits must be in the snapshot.
        compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--server-id', '1', '--tables', '/app/tables',
                '--enable-test-payments', '--enable-local-wallet', '--apply-growth75', '--log-file', '/app/logs/growth75-apply.log')
        record['state'] = 'candidate-cli-complete'
        checkpoint()
        export = script('--export-sql')
        private_write(directory / 'export.sql', export)
        snapshot = directory / 'snapshot.tsv'
        private_write(snapshot, mysql('SET NAMES utf8mb4;\n' + export))
        row_counts = {}
        for line in snapshot.read_text(encoding='utf-8').splitlines():
            if line:
                table, _ = line.split('\t', 1)
                row_counts[table] = row_counts.get(table, 0) + 1
        assert all(count < 20000 for count in row_counts.values()), 'Activity snapshot reached an export bound; inspect before applying'
        record['snapshotRows'] = row_counts
        sql = directory / 'activity-cas.sql'
        changes_manifest = directory / 'activity-cas.json'
        script('--snapshot', snapshot, '--sql', sql, '--manifest', changes_manifest)
        for path in (sql, changes_manifest):
            os.chmod(path, 0o600)
        changes = json.loads(changes_manifest.read_text(encoding='utf-8'))
        assert changes['schema'] == 'growth75-activity-cas-v1' and changes['sourceSHA256'] == digest(snapshot)
        record['changeCount'] = len(changes['changes'])
        revision_before = int(mysql('SELECT revision FROM admin_activity_schedule_revisions WHERE server_id=1;').strip())
        executable_sql = directory / 'activity-cas-executed.sql'
        private_write(executable_sql, bind_schedule_revision(sql.read_text(encoding='utf-8'), revision_before))
        record['scheduleRevision'] = {'before': revision_before, 'after': revision_before + 1}
        targets = sorted({row['table'] for row in changes['changes']} | {'admin_audits', 'admin_activity_schedule_revisions'})
        assert all(name.replace('_', '').isalnum() for name in targets)
        engines = mysql("SELECT table_name,engine FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN (" + ','.join("'" + name + "'" for name in targets) + ');')
        engine_map = dict(line.split('\t') for line in engines.strip().splitlines() if line)
        assert set(engine_map) == set(targets) and all(engine.upper() == 'INNODB' for engine in engine_map.values()), 'CAS targets require InnoDB transactional tables'
        private_write(directory / 'transaction-engines.tsv', engines)
        record['files'] = {p.name: digest(p) for p in (directory / 'before-policy.tsv', directory / 'publication-proof.tsv', directory / 'transaction-engines.tsv', directory / 'export.sql', snapshot, sql, executable_sql, changes_manifest)}
        record['state'] = 'cas-generated'
        checkpoint()
        # Never split this document across separate mysql invocations.
        applied = mysql('SET NAMES utf8mb4;\n' + executable_sql.read_text(encoding='utf-8'))
        tail = applied.strip().splitlines()[-1].split('\t')
        assert tail == [str(record['changeCount']), str(record['changeCount'])], 'CAS row-count verification failed'
        private_write(directory / 'applied.tsv', applied)
        assert mysql('SELECT maintenance,latest_build,min_login_build,min_run_build,version_name FROM client_update_policy WHERE id=1;').strip() == '1\t2026102702\t2026102702\t2026102702\t1.0.75', 'Candidate compatibility policy or maintenance changed'
        record['state'] = 'applied-maintenance'
        record['files']['applied.tsv'] = digest(directory / 'applied.tsv')
        checkpoint()
        return {'state': record['state'], 'changeCount': record['changeCount'],
                'privateManifest': str(directory / 'manifest.json'), 'manifestSHA256': digest(directory / 'manifest.json')}
    except Exception as error:
        record['state'] = 'failed-maintenance'
        record['failure'] = type(error).__name__
        record['failureDetail'] = str(error)[-5000:]
        record['recovery'] = 'Keep app stopped. A failed uncommitted CAS connection rolls back. Prior candidate CLI domain edits remain audited; inspect this manifest and the encrypted pre-deploy backup before a bounded retry.'
        checkpoint()
        raise
