"""Maintenance-only trial82 and feedback84 activation after an authenticated backup."""
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

    def __init__(self, compose, backup_dir, revision, source_sha256, expected_live_revision, *, mode='configuration'):
        self.compose = compose
        self.backup_dir = backup_dir
        self.revision = revision
        self.source_sha256 = source_sha256
        self.expected_live_revision = expected_live_revision
        self.mode = mode
        self.complete = False
        self.manifest = None
        self.record = None

    def prepare(self):
        assert _REVISION.fullmatch(self.revision), 'Invalid candidate revision'
        assert _REVISION.fullmatch(self.expected_live_revision), 'Invalid expected live revision'
        assert re.fullmatch(r'[0-9a-f]{64}', self.source_sha256), 'Invalid candidate binary digest'
        assert self.mode in ('configuration', 'maintenance-redeploy'), 'Invalid maintenance mode'
        assert self.backup_dir.is_dir() and not self.backup_dir.is_symlink(), 'Private backup directory required'
        directory = self.backup_dir / ('trial82-' + self.revision[:12] + '-' + str(time.time_ns()))
        directory.mkdir(mode=0o700)
        self.manifest = directory / 'manifest.json'
        self.record = {'schema': 'trial82-activation-v1', 'revision': self.revision,
                       'sourceSHA256': self.source_sha256, 'expectedLiveRevision': self.expected_live_revision,
                       'backupBefore': None, 'state': 'awaiting-backup', 'maintenanceRequired': True, 'mode': self.mode}
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
            self.record.setdefault('failureStage', self.record.get('state', 'guard-uninitialized'))
            self.record['state'] = 'failed-maintenance'
            self.record.setdefault('failure', 'cutover-incomplete')
            self.record['stopCommandSucceeded'] = stopped
            self.record['recovery'] = 'Keep app stopped; retain the authenticated backup and any new writes; inspect the manifest before retry.'
            try:
                self.checkpoint()
            except Exception:
                print('Trial82 failure manifest could not be saved; inspect the server immediately.', file=sys.stderr)
        else:
            print('Trial82 failure manifest was not initialized; inspect the server immediately.', file=sys.stderr)

    def install_signal_handlers(self):
        def interrupted(signum, _frame):
            raise SystemExit('Trial82 interrupted by signal ' + str(signum))

        for signum in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT):
            signal.signal(signum, interrupted)


def activate_trial82(mysql, compose, guard, backup_record, *, apply_trial82=True, apply_feedback84=False, apply_feedback85=False, maintenance_redeploy=False):
    assert guard.record is not None and guard.manifest is not None, 'Trial82 guard is not prepared'
    try:
        assert all(type(value) is bool for value in (apply_trial82, apply_feedback84, apply_feedback85, maintenance_redeploy)), 'Invalid activation selection'
        selected = apply_trial82 or apply_feedback84 or apply_feedback85
        assert (maintenance_redeploy and not selected or not maintenance_redeploy and selected), 'Choose configuration publication or maintenance redeploy'
        assert guard.mode == ('maintenance-redeploy' if maintenance_redeploy else 'configuration'), 'Guard mode differs from activation'
        commands = []
        if apply_trial82:
            commands.append(('--apply-trial82', '/app/logs/trial82-apply.log'))
        if apply_feedback84:
            commands.append(('--apply-feedback84', '/app/logs/feedback84-apply.log'))
        if apply_feedback85:
            commands.append(('--apply-feedback85', '/app/logs/feedback85-apply.log'))
        guard.record['requestedCommands'] = [flag for flag, _ in commands]
        guard.record['commands'] = []
        guard.record['state'] = 'checking-backup'
        guard.record['backupBefore'] = backup_record if isinstance(backup_record, dict) else None
        guard.checkpoint()
        assert backup_record and backup_record.get('format') == 'HYJSBK02-AES256GCM-CHUNKED', 'Authenticated backup required'
        assert backup_record.get('revisionBefore') == guard.expected_live_revision, 'Backup live revision differs'
        name = backup_record.get('file', '')
        assert name and Path(name).name == name
        backup = guard.backup_dir / name
        assert backup.is_file() and not backup.is_symlink() and digest(backup) == backup_record.get('sha256'), 'Verified backup changed'
        guard.record['state'] = 'backup-verified'
        guard.checkpoint()
        guard.record['state'] = 'checking-maintenance-before'
        guard.checkpoint()
        assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Activation requires maintenance'
        for flag, log_file in commands:
            label = flag.removeprefix('--apply-')
            operation = {'command': flag, 'status': 'started'}
            guard.record['commands'].append(operation)
            guard.record['state'] = 'candidate-cli-' + label + '-running'
            guard.checkpoint()
            try:
                compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--server-id', '1', '--tables', '/app/tables',
                        flag, '--log-file', log_file)
            except BaseException as error:
                operation['status'] = 'interrupted' if isinstance(error, (KeyboardInterrupt, SystemExit)) else 'failed'
                raise
            operation['status'] = 'completed'
            guard.record['state'] = 'checking-maintenance-after-' + label
            guard.checkpoint()
            assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Maintenance changed during ' + label + ' apply'
        guard.record['state'] = 'verified-maintenance' if maintenance_redeploy else 'applied-maintenance'
        guard.checkpoint()
        return {'state': guard.record['state'], 'appliedCommands': [operation['command'] for operation in guard.record['commands']],
                'privateManifest': str(guard.manifest), 'manifestSHA256': digest(guard.manifest)}
    except BaseException as error:
        guard.record['failure'] = type(error).__name__
        guard.record['failureStage'] = guard.record.get('state', 'guard-uninitialized')
        try:
            guard.checkpoint()
        except Exception:
            print('Activation failure manifest could not be saved; inspect the server immediately.', file=sys.stderr)
        raise
