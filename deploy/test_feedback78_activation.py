import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
spec = importlib.util.spec_from_file_location('feedback78_activation', HERE / 'feedback78_activation.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class Feedback78ActivationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        backup = self.root / 'verified.aesgcm'
        backup.write_bytes(b'authenticated backup fixture')
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
        return module.activate_feedback78(self.mysql, self.compose, self.root, self.revision, self.backup, self.sha)

    def test_authenticated_backup_and_maintenance_success(self):
        result = self.execute()
        self.assertEqual(self.calls, [('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--server-id', '1', '--tables', '/app/tables', '--apply-feedback78', '--log-file', '/app/logs/feedback78-apply.log')])
        manifest = Path(result['privateManifest'])
        state = json.loads(manifest.read_text())
        self.assertEqual(state['state'], 'applied-maintenance')
        self.assertEqual(state['backupBefore'], self.backup)
        self.assertEqual(state['sourceSHA256'], self.sha)
        self.assertEqual(manifest.stat().st_mode & 0o777, 0o600)
        self.assertEqual(result['manifestSHA256'], module.digest(manifest))
        self.assertTrue((self.root / self.backup['file']).is_file())

    def test_invalid_backup_or_maintenance_never_runs_cli(self):
        for variant in ('missing', 'format', 'hash', 'path', 'symlink', 'maintenance', 'revision', 'binary'):
            with self.subTest(variant=variant):
                backup = dict(self.backup)
                revision, binary = self.revision, self.sha
                values = iter(['1'])
                if variant == 'missing':
                    backup = None
                elif variant == 'format':
                    backup['format'] = 'unauthenticated'
                elif variant == 'hash':
                    backup['sha256'] = '0' * 64
                elif variant == 'path':
                    backup['file'] = '../verified.aesgcm'
                elif variant == 'symlink':
                    link = self.root / 'linked.aesgcm'
                    link.symlink_to(self.root / self.backup['file'])
                    backup['file'] = link.name
                elif variant == 'maintenance':
                    values = iter(['0'])
                elif variant == 'revision':
                    revision = 'not-a-revision'
                else:
                    binary = 'not-a-digest'
                calls = []
                with self.assertRaises(AssertionError):
                    module.activate_feedback78(lambda _: next(values), lambda *args: calls.append(args), self.root, revision, backup, binary)
                self.assertEqual(calls, [])
                self.assertTrue((self.root / self.backup['file']).is_file())

    def test_cli_or_postcheck_failure_preserves_private_evidence(self):
        def fail(*args):
            raise RuntimeError('private detail')

        self.compose = fail
        with self.assertRaises(RuntimeError):
            self.execute()
        manifests = list(self.root.glob('feedback78-*/manifest.json'))
        self.assertEqual(len(manifests), 1)
        raw = manifests[0].read_text()
        self.assertEqual(json.loads(raw)['state'], 'failed-maintenance')
        self.assertNotIn('private detail', raw)
        self.assertTrue((self.root / self.backup['file']).is_file())
        self.compose = lambda *args: self.calls.append(args)
        self.mysql_values = iter(['1', '0'])
        with self.assertRaises(AssertionError):
            self.execute()
        states = [json.loads(path.read_text())['state'] for path in self.root.glob('feedback78-*/manifest.json')]
        self.assertEqual(states, ['failed-maintenance', 'failed-maintenance'])

    def test_failure_guard_stops_and_success_disarms_it(self):
        guard = module.Feedback78CutoverGuard(self.compose)
        guard.stop_if_unfinished()
        self.assertEqual(self.calls, [('stop', '-t', '30', 'app')])
        guard.complete = True
        guard.stop_if_unfinished()
        self.assertEqual(len(self.calls), 1)

        def fail(*args):
            raise RuntimeError('sensitive subprocess output')

        output = io.StringIO()
        with contextlib.redirect_stderr(output):
            module.Feedback78CutoverGuard(fail).stop_if_unfinished()
        self.assertIn('could not be stopped', output.getvalue())
        self.assertNotIn('sensitive subprocess output', output.getvalue())

    def test_termination_signals_trigger_failure_guard(self):
        for signum in (signal.SIGHUP, signal.SIGTERM):
            with self.subTest(signal=signum):
                marker = self.root / ('stopped-' + str(signum))
                source = (
                    'import atexit, pathlib, signal, sys\n'
                    'sys.path.insert(0, ' + repr(str(HERE)) + ')\n'
                    'from feedback78_activation import Feedback78CutoverGuard\n'
                    'def stop(*args): pathlib.Path(' + repr(str(marker)) + ').write_text(" ".join(args))\n'
                    'guard = Feedback78CutoverGuard(stop)\n'
                    'atexit.register(guard.stop_if_unfinished)\n'
                    'guard.install_signal_handlers()\n'
                    'print("ready", flush=True)\n'
                    'signal.pause()\n'
                )
                process = subprocess.Popen([sys.executable, '-c', source], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                try:
                    self.assertEqual(process.stdout.readline().strip(), 'ready')
                    os.kill(process.pid, signum)
                    _, stderr = process.communicate(timeout=5)
                    self.assertNotEqual(process.returncode, 0)
                    self.assertIn('Feedback78 interrupted by signal', stderr)
                    self.assertEqual(marker.read_text(), 'stop -t 30 app')
                finally:
                    if process.poll() is None:
                        process.kill()
                        process.communicate(timeout=5)

    def test_invalid_flags_rejected_before_external_work(self):
        revision = 'a' * 40
        common = [
            ['--apply-feedback78'],
            ['--apply-feedback78', '--expected-live-revision', 'bad'],
            ['--apply-feedback78', '--apply-growth74', '--expected-live-revision', revision],
            ['--apply-feedback78', '--apply-growth75', '--expected-live-revision', revision],
            ['--apply-feedback78', '--apply-activity77', '--expected-live-revision', revision],
            ['--apply-feedback78', '--migrate-role-wallets', '--expected-live-revision', revision],
            ['--apply-feedback78', '--warm-rush-history-player', '1', '--expected-live-revision', revision],
            ['--apply-feedback78', '--restore-activity-calendar', '00000000-0000-0000-0000-000000000001', '--expected-live-revision', revision],
        ]
        for script, extra in (('deploy.py', ['--provision']), ('deploy.py', ['--update-ingress']),
                              ('activate.py', ['--initial-db']), ('activate.py', ['--provision-ingress'])):
            base = [sys.executable, str(HERE / script)]
            if script == 'activate.py':
                base.append('/not/a/real/stage')
            for options in common + [['--apply-feedback78', *extra, '--expected-live-revision', revision]]:
                result = subprocess.run(base + options, capture_output=True, text=True)
                self.assertNotEqual(result.returncode, 0, (script, options))
                self.assertIn('AssertionError', result.stderr, (script, options, result.stderr))
                self.assertNotIn('FileNotFoundError', result.stderr, (script, options))

    def test_changed_live_revision_rejected_before_external_work(self):
        root = self.root / 'owned'
        stage = root / '.incoming' / 'candidate'
        stage.mkdir(parents=True)
        (root / '.hyjs-owned').write_text('')
        (root / 'deployed.json').write_text(json.dumps({'revision': 'a' * 40}))
        (stage / 'artifact.json').write_text('{}')
        result = subprocess.run([sys.executable, str(HERE / 'activate.py'), str(stage), '--apply-feedback78', '--expected-live-revision', 'b' * 40], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Live revision changed', result.stderr)
        self.assertTrue((stage / 'artifact.json').is_file())
        self.assertFalse((root / 'backups').exists())

    def test_cutover_wiring_order_and_artifact_packaging(self):
        deploy = (HERE / 'deploy.py').read_text()
        activate = (HERE / 'activate.py').read_text()
        self.assertIn("'feedback78_activation.py'", deploy)
        self.assertIn("command += ' --apply-feedback78'", deploy)
        ordered = ["atexit.register(feedback78_guard.stop_if_unfinished)",
                   "feedback78_guard.install_signal_handlers()",
                   "verified_backup = backup_existing_database()",
                   "feedback78_activation.activate_feedback78(",
                   "compose('up', '-d', '--no-deps', '--force-recreate', 'app')",
                   "'Maintenance changed after health check'",
                   "feedback78_guard.complete = True"]
        positions = [activate.index(marker) for marker in ordered]
        self.assertEqual(positions, sorted(positions))
        self.assertIn("if args.migrate_role_wallets or args.apply_activity77 or args.apply_feedback78 or args.apply_feedback79:", activate)


if __name__ == '__main__':
    unittest.main()
