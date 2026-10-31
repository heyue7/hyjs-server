import ast
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

import backup_pointer

HERE = Path(__file__).resolve().parent


class BackupPointerTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix='hyjs-backup-pointer-')
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.pointer = self.root / 'backup-latest.json'
        self.old = b'{"file":"previous.aesgcm","sha256":"old"}'
        self.pointer.write_bytes(self.old)
        self.pointer.chmod(0o640)
        self.record = {'file': 'new.aesgcm', 'sha256': 'new', 'revisionBefore': 'old-revision'}

    def assert_no_temporary_files(self):
        self.assertEqual([self.pointer.name], sorted(p.name for p in self.root.iterdir()))

    def test_success_replaces_old_pointer_with_private_synced_json(self):
        real_fsync = os.fsync
        real_replace = os.replace
        events = []

        def sync(fd):
            events.append('directory fsync' if stat.S_ISDIR(os.fstat(fd).st_mode) else 'file fsync')
            return real_fsync(fd)

        def replace(source, target):
            self.assertEqual(0o600, stat.S_IMODE(Path(source).stat().st_mode))
            self.assertEqual(self.old, self.pointer.read_bytes())
            events.append('replace')
            return real_replace(source, target)

        with mock.patch.object(backup_pointer.os, 'fsync', side_effect=sync):
            with mock.patch.object(backup_pointer.os, 'replace', side_effect=replace):
                backup_pointer.write_backup_pointer(self.pointer, self.record)
        self.assertEqual(self.record, json.loads(self.pointer.read_text(encoding='utf-8')))
        self.assertEqual(0o600, stat.S_IMODE(self.pointer.stat().st_mode))
        self.assertEqual(['file fsync', 'replace', 'directory fsync'], events)
        self.assert_no_temporary_files()

    def test_replace_failure_preserves_old_pointer_and_cleans_temporary(self):
        with mock.patch.object(backup_pointer.os, 'replace', side_effect=OSError('replace failed')):
            with self.assertRaisesRegex(OSError, 'replace failed'):
                backup_pointer.write_backup_pointer(self.pointer, self.record)
        self.assertEqual(self.old, self.pointer.read_bytes())
        self.assertEqual(0o640, stat.S_IMODE(self.pointer.stat().st_mode))
        self.assert_no_temporary_files()

    def test_file_fsync_failure_preserves_old_pointer_and_cleans_temporary(self):
        with mock.patch.object(backup_pointer.os, 'fsync', side_effect=OSError('file fsync failed')):
            with mock.patch.object(backup_pointer.os, 'replace') as replace:
                with self.assertRaisesRegex(OSError, 'file fsync failed'):
                    backup_pointer.write_backup_pointer(self.pointer, self.record)
                replace.assert_not_called()
        self.assertEqual(self.old, self.pointer.read_bytes())
        self.assertEqual(0o640, stat.S_IMODE(self.pointer.stat().st_mode))
        self.assert_no_temporary_files()

    def test_directory_fsync_failure_reports_unconfirmed_committed_pointer(self):
        real_fsync = os.fsync
        calls = 0

        def fail_directory(fd):
            nonlocal calls
            calls += 1
            if calls == 2:
                raise OSError('directory fsync failed')
            return real_fsync(fd)

        with mock.patch.object(backup_pointer.os, 'fsync', side_effect=fail_directory):
            with self.assertRaisesRegex(OSError, 'directory fsync failed'):
                backup_pointer.write_backup_pointer(self.pointer, self.record)
        self.assertEqual(self.record, json.loads(self.pointer.read_text(encoding='utf-8')))
        self.assertEqual(0o600, stat.S_IMODE(self.pointer.stat().st_mode))
        self.assert_no_temporary_files()


class BackupPolicyTest(unittest.TestCase):
    def activation_block(self, marker):
        source = (HERE / 'activate.py').read_text()
        node = next(n for n in ast.parse(source).body if isinstance(n, ast.If)
                    and ast.get_source_segment(source, n.test) == "args.with_backup and (ROOT / 'migration.json').exists()"
                    and marker in ast.get_source_segment(source, n))
        return compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')

    def test_backup_work_and_space_check_only_run_when_selected(self):
        space = self.activation_block('required_backup_space')
        backup = self.activation_block('verified_backup = backup_existing_database()')
        with tempfile.TemporaryDirectory(prefix='hyjs-backup-policy-') as directory:
            root = Path(directory)
            (root / 'migration.json').write_text('{}')
            # An invalid old pointer would fail immediately if the default path read it.
            (root / 'backup-latest.json').write_text('invalid json')
            args = SimpleNamespace(with_backup=False, calendar_formal_110=False, apply_trial82=False,
                                   apply_feedback84=False, apply_feedback85=False, maintenance_redeploy=False)
            mysql = mock.Mock()
            disk_usage = mock.Mock(return_value=SimpleNamespace(free=1024 * 1024 * 1024))
            create_backup = mock.Mock(return_value={'file': 'verified.aesgcm'})
            namespace = {'ROOT': root, 'args': args, 'mysql': mysql, 'json': json,
                         'shutil': SimpleNamespace(disk_usage=disk_usage), 'backup_existing_database': create_backup}
            exec(space, namespace)
            exec(backup, namespace)
            mysql.assert_not_called()
            disk_usage.assert_not_called()
            create_backup.assert_not_called()
            self.assertEqual('invalid json', (root / 'backup-latest.json').read_text())

            (root / 'backups').mkdir()
            (root / 'backups' / 'prior.aesgcm').write_bytes(b'old-backup')
            (root / 'backup-latest.json').write_text(json.dumps({'file': 'prior.aesgcm'}))
            args.with_backup = True
            exec(space, namespace)
            exec(backup, namespace)
            disk_usage.assert_called_once_with(root)
            create_backup.assert_called_once_with()
            self.assertEqual({'file': 'verified.aesgcm'}, namespace['verified_backup'])

    def test_backup_flag_is_forwarded_only_when_selected(self):
        source = (HERE / 'deploy.py').read_text()
        node = next(n for n in ast.parse(source).body if isinstance(n, ast.If)
                    and ast.get_source_segment(source, n.test) == 'args.with_backup'
                    and "command += ' --with-backup'" in ast.get_source_segment(source, n))
        block = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'deploy.py'), 'exec')
        for selected in (False, True):
            namespace = {'args': SimpleNamespace(with_backup=selected), 'command': 'activate'}
            exec(block, namespace)
            self.assertEqual('activate --with-backup' if selected else 'activate', namespace['command'])

    def test_special_modes_require_backup_before_external_work(self):
        modes = [
            ['--migrate-role-wallets'], ['--restore-activity-calendar', '00000000-0000-0000-0000-000000000001'],
            ['--calendar-formal-110'], ['--apply-growth74'], ['--apply-growth75'], ['--apply-activity77'],
            ['--apply-feedback78'], ['--apply-feedback79'], ['--apply-feedback80'], ['--apply-feedback81'],
            ['--apply-trial82'], ['--apply-feedback84'], ['--apply-feedback85'], ['--maintenance-redeploy'],
        ]
        for script in ('deploy.py', 'activate.py'):
            prefix = [sys.executable, '-B', str(HERE / script)]
            if script == 'activate.py':
                prefix.append('/not/a/real/stage')
            for mode in modes:
                with self.subTest(script=script, mode=mode):
                    result = subprocess.run([*prefix, *mode], capture_output=True, text=True)
                    self.assertEqual(2, result.returncode)
                    self.assertIn('requires explicit --with-backup', result.stderr)
                    self.assertNotIn('FileNotFoundError', result.stderr)

    def test_local_config_check_allows_normal_and_explicit_backup(self):
        for flags in ([], ['--with-backup']):
            with self.subTest(flags=flags):
                result = subprocess.run([sys.executable, '-B', str(HERE / 'deploy.py'), '--check-config',
                                         '--env-file', str(HERE / '.env.example'), *flags],
                                        capture_output=True, text=True)
                self.assertEqual(0, result.returncode, result.stderr)
                self.assertIn('no external action performed', result.stdout)


if __name__ == '__main__':
    unittest.main()
