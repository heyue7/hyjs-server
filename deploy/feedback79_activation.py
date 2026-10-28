"""Maintenance-only feedback79 activation after the authenticated backup."""
from pathlib import Path
import re
import signal
import sys
import time

from activity77_activation import digest, private_write


class Feedback79CutoverGuard:
    """Keep the application stopped if any cutover step fails after quiescing."""

    def __init__(self, compose):
        self.compose = compose
        self.complete = False

    def stop_if_unfinished(self):
        if not self.complete:
            try:
                self.compose('stop', '-t', '30', 'app')
            except Exception:
                print('Feedback79 failed and the app could not be stopped; inspect the server immediately.', file=sys.stderr)

    def install_signal_handlers(self):
        def interrupted(signum, _frame):
            raise SystemExit('Feedback79 interrupted by signal ' + str(signum))

        for signum in (signal.SIGHUP, signal.SIGTERM):
            signal.signal(signum, interrupted)


def activate_feedback79(mysql, compose, backup_dir, revision, backup_record, source_sha256):
    assert re.fullmatch(r'[0-9a-f]{40,64}', revision), 'Invalid candidate revision'
    assert re.fullmatch(r'[0-9a-f]{64}', source_sha256), 'Invalid candidate binary digest'
    assert backup_dir.is_dir() and not backup_dir.is_symlink()
    assert backup_record and backup_record.get('format') == 'HYJSBK02-AES256GCM-CHUNKED', 'Authenticated backup required'
    name = backup_record.get('file', '')
    assert name and Path(name).name == name
    backup = backup_dir / name
    assert backup.is_file() and not backup.is_symlink() and digest(backup) == backup_record.get('sha256'), 'Verified backup changed'
    assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Feedback79 requires maintenance'
    directory = backup_dir / ('feedback79-' + revision[:12] + '-' + str(time.time_ns()))
    directory.mkdir(mode=0o700)
    manifest = directory / 'manifest.json'
    record = {'schema': 'feedback79-activation-v1', 'revision': revision, 'sourceSHA256': source_sha256,
              'backupBefore': backup_record, 'state': 'started', 'maintenanceRequired': True}

    def checkpoint():
        private_write(manifest, record)

    checkpoint()
    try:
        compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--server-id', '1', '--tables', '/app/tables',
                '--apply-feedback79', '--log-file', '/app/logs/feedback79-apply.log')
        record['state'] = 'candidate-cli-complete'
        checkpoint()
        assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Maintenance changed during feedback79 apply'
        record['state'] = 'applied-maintenance'
        checkpoint()
        return {'state': record['state'], 'privateManifest': str(manifest), 'manifestSHA256': digest(manifest)}
    except Exception as error:
        record['state'] = 'failed-maintenance'
        record['failure'] = type(error).__name__
        record['recovery'] = 'Keep app stopped; retain the authenticated backup and inspect the audited CLI result before retry.'
        checkpoint()
        raise
