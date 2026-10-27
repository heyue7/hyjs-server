"""Maintenance-only activity77 activation; called after authenticated backup."""
from pathlib import Path
import hashlib
import json
import os
import re
import time


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def private_write(path, value):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w', encoding='utf-8') as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.write('\n')
    os.chmod(path, 0o600)


def activate_activity77(mysql, compose, backup_dir, revision, backup_record, source_sha256):
    assert re.fullmatch(r'[0-9a-f]{40,64}', revision), 'Invalid candidate revision'
    assert re.fullmatch(r'[0-9a-f]{64}', source_sha256), 'Invalid candidate binary digest'
    assert backup_dir.is_dir() and not backup_dir.is_symlink()
    assert backup_record and backup_record.get('format') == 'HYJSBK02-AES256GCM-CHUNKED', 'Authenticated backup required'
    name = backup_record.get('file', '')
    assert name and Path(name).name == name
    backup = backup_dir / name
    assert backup.is_file() and not backup.is_symlink() and digest(backup) == backup_record.get('sha256'), 'Verified backup changed'
    assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Activity77 requires maintenance'
    directory = backup_dir / ('activity77-' + revision[:12] + '-' + str(time.time_ns()))
    directory.mkdir(mode=0o700)
    record = {'schema': 'activity77-activation-v1', 'revision': revision, 'sourceSHA256': source_sha256,
              'backupBefore': backup_record, 'state': 'started', 'maintenanceRequired': True}
    manifest = directory / 'manifest.json'
    def checkpoint():
        private_write(manifest, record)
    checkpoint()
    try:
        compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--server-id', '1', '--tables', '/app/tables',
                '--enable-test-payments', '--enable-local-wallet', '--apply-activity77', '--log-file', '/app/logs/activity77-apply.log')
        record['state'] = 'candidate-cli-complete'
        checkpoint()
        assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Maintenance changed during activity77 apply'
        record['state'] = 'applied-maintenance'
        checkpoint()
        return {'state': record['state'], 'privateManifest': str(manifest), 'manifestSHA256': digest(manifest)}
    except Exception as error:
        record['state'] = 'failed-maintenance'
        record['failure'] = type(error).__name__
        record['recovery'] = 'Keep app stopped; retain the authenticated backup and inspect audited CLI steps before retry.'
        checkpoint()
        raise
