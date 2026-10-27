import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('activity77_activation', HERE / 'activity77_activation.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class Activity77ActivationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        backup = self.root / 'verified.aesgcm'
        backup.write_bytes(b'authenticated fixture')
        self.backup = {'file': backup.name, 'sha256': hashlib.sha256(backup.read_bytes()).hexdigest(),
                       'format': 'HYJSBK02-AES256GCM-CHUNKED'}
        self.revision = 'a' * 40
        self.sha = 'b' * 64
        self.calls = []
        self.mysql_values = iter(['1', '1'])

    def mysql(self, query):
        self.assertEqual(query, 'SELECT maintenance FROM client_update_policy WHERE id=1;')
        return next(self.mysql_values)

    def compose(self, *args):
        self.calls.append(args)

    def execute(self):
        return module.activate_activity77(self.mysql, self.compose, self.root, self.revision, self.backup, self.sha)

    def test_success_and_private_checkpoint(self):
        result = self.execute()
        self.assertEqual(self.calls, [('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--server-id', '1', '--tables', '/app/tables', '--enable-test-payments', '--enable-local-wallet', '--apply-activity77', '--log-file', '/app/logs/activity77-apply.log')])
        manifest = Path(result['privateManifest'])
        state = json.loads(manifest.read_text())
        self.assertEqual(state['state'], 'applied-maintenance')
        self.assertEqual(state['sourceSHA256'], self.sha)
        self.assertEqual(manifest.stat().st_mode & 0o777, 0o600)
        self.assertEqual(result['manifestSHA256'], module.digest(manifest))

    def test_cli_failure_retains_private_checkpoint(self):
        def fail(*args):
            raise RuntimeError('private detail')
        self.compose = fail
        with self.assertRaises(RuntimeError):
            self.execute()
        manifest = next(self.root.glob('activity77-*/manifest.json'))
        raw = manifest.read_text()
        self.assertEqual(json.loads(raw)['state'], 'failed-maintenance')
        self.assertNotIn('private detail', raw)
        self.assertTrue((self.root / self.backup['file']).exists())

    def test_maintenance_after_cli_and_backup_guard(self):
        self.mysql_values = iter(['1', '0'])
        with self.assertRaises(AssertionError):
            self.execute()
        self.assertEqual(json.loads(next(self.root.glob('activity77-*/manifest.json')).read_text())['state'], 'failed-maintenance')
        self.backup['sha256'] = '0' * 64
        with self.assertRaises(AssertionError):
            self.execute()

    def test_arguments_rejected_before_external_work(self):
        for script in ('deploy.py', 'activate.py'):
            base = ['python3', str(HERE / script)]
            if script == 'activate.py':
                base.append('/not/a/real/stage')
            for options in (['--apply-activity77'],
                            ['--apply-activity77', '--apply-growth75', '--expected-live-revision', 'x'],
                            ['--apply-activity77', '--migrate-role-wallets', '--expected-live-revision', 'x'],
                            ['--apply-activity77', '--restore-activity-calendar', '00000000-0000-0000-0000-000000000001', '--expected-live-revision', 'x']):
                result = subprocess.run(base + options, capture_output=True, text=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('AssertionError', result.stderr)
                self.assertNotIn('FileNotFoundError', result.stderr)


if __name__ == '__main__':
    unittest.main()
