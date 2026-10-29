"""Maintenance-only trial82 activation after an authenticated backup."""
from pathlib import Path
import re
import signal
import sys
import time

from activity77_activation import digest, private_write


_REVISION = re.compile(r"(?:[0-9a-f]{40}|[0-9a-f]{64})")


def require_existing_ingress(available, enabled):
    """Permit updating only the already installed HYJS ingress."""
    assert available.is_file() and not available.is_symlink(), 'Trial82 requires an existing project ingress'
    assert enabled.is_symlink() and enabled.is_file() and enabled.resolve() == available.resolve(), \
        'Trial82 requires the existing project ingress link'


class Trial82CutoverGuard:
    """Keep the application stopped and retain failure evidence until recorded."""

    def __init__(self, compose, backup_dir, revision, source_sha256, expected_live_revision):
        self.compose = compose
        self.backup_dir = backup_dir
        self.revision = revision
        self.source_sha256 = source_sha256
        self.expected_live_revision = expected_live_revision
        self.complete = False
        self.manifest = None
        self.record = None

    def prepare(self):
        assert _REVISION.fullmatch(self.revision), 'Invalid candidate revision'
        assert _REVISION.fullmatch(self.expected_live_revision), 'Invalid expected live revision'
        assert re.fullmatch(r'[0-9a-f]{64}', self.source_sha256), 'Invalid candidate binary digest'
        assert self.backup_dir.is_dir() and not self.backup_dir.is_symlink(), 'Private backup directory required'
        directory = self.backup_dir / ('trial82-' + self.revision[:12] + '-' + str(time.time_ns()))
        directory.mkdir(mode=0o700)
        self.manifest = directory / 'manifest.json'
        self.record = {'schema': 'trial82-activation-v1', 'revision': self.revision,
                       'sourceSHA256': self.source_sha256, 'expectedLiveRevision': self.expected_live_revision,
                       'backupBefore': None, 'state': 'awaiting-backup', 'maintenanceRequired': True}
        self.checkpoint()

    def checkpoint(self):
        private_write(self.manifest, self.record)

    def stop_if_unfinished(self):
        if self.complete:
            return
        stopped = False
        try:
            self.compose('stop', '-t', '30', 'app')
            stopped = True
        except Exception:
            print('Trial82 failed and the app could not be stopped; inspect the server immediately.', file=sys.stderr)
        if self.record is not None:
            self.record['state'] = 'failed-maintenance'
            self.record.setdefault('failure', 'cutover-incomplete')
            self.record['stopCommandSucceeded'] = stopped
            self.record['recovery'] = 'Keep app stopped; retain the authenticated backup and inspect the audited CLI result before retry.'
            try:
                self.checkpoint()
            except Exception:
                print('Trial82 failure manifest could not be saved; inspect the server immediately.', file=sys.stderr)
        else:
            print('Trial82 failure manifest was not initialized; inspect the server immediately.', file=sys.stderr)

    def install_signal_handlers(self):
        def interrupted(signum, _frame):
            raise SystemExit('Trial82 interrupted by signal ' + str(signum))

        for signum in (signal.SIGHUP, signal.SIGTERM):
            signal.signal(signum, interrupted)


def activate_trial82(mysql, compose, guard, backup_record):
    assert guard.record is not None and guard.manifest is not None, 'Trial82 guard is not prepared'
    try:
        guard.record['backupBefore'] = backup_record if isinstance(backup_record, dict) else None
        assert backup_record and backup_record.get('format') == 'HYJSBK02-AES256GCM-CHUNKED', 'Authenticated backup required'
        assert backup_record.get('revisionBefore') == guard.expected_live_revision, 'Backup live revision differs'
        name = backup_record.get('file', '')
        assert name and Path(name).name == name
        backup = guard.backup_dir / name
        assert backup.is_file() and not backup.is_symlink() and digest(backup) == backup_record.get('sha256'), 'Verified backup changed'
        guard.record['state'] = 'backup-verified'
        guard.checkpoint()
        assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Trial82 requires maintenance'
        compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--server-id', '1', '--tables', '/app/tables',
                '--apply-trial82', '--log-file', '/app/logs/trial82-apply.log')
        guard.record['state'] = 'candidate-cli-complete'
        guard.checkpoint()
        assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Maintenance changed during trial82 apply'
        guard.record['state'] = 'applied-maintenance'
        guard.checkpoint()
        return {'state': guard.record['state'], 'privateManifest': str(guard.manifest), 'manifestSHA256': digest(guard.manifest)}
    except BaseException as error:
        guard.record['failure'] = type(error).__name__
        raise
