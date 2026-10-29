import ast
import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import shlex
import signal
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import trial82_activation as module


class Trial82ActivationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.revision = 'a' * 40
        self.live_revision = 'c' * 40
        self.binary_sha = 'b' * 64
        backup = self.root / 'verified.aesgcm'
        backup.write_bytes(b'authenticated backup fixture')
        self.backup = {'file': backup.name, 'sha256': hashlib.sha256(backup.read_bytes()).hexdigest(),
                       'format': 'HYJSBK02-AES256GCM-CHUNKED', 'revisionBefore': self.live_revision}
        self.calls = []
        self.mysql_values = iter(['1', '1'])

    def compose(self, *args):
        self.calls.append(args)

    def mysql(self, query):
        self.assertEqual(query, 'SELECT maintenance FROM client_update_policy WHERE id=1;')
        return next(self.mysql_values)

    def guard(self):
        guard = module.Trial82CutoverGuard(self.compose, self.root, self.revision, self.binary_sha, self.live_revision)
        guard.prepare()
        return guard

    def test_authenticated_backup_and_maintenance_success(self):
        guard = self.guard()
        self.assertEqual(json.loads(guard.manifest.read_text())['state'], 'awaiting-backup')
        result = module.activate_trial82(self.mysql, self.compose, guard, self.backup)
        self.assertEqual(self.calls, [('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--server-id', '1', '--tables', '/app/tables', '--apply-trial82', '--log-file', '/app/logs/trial82-apply.log')])
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['state'], 'applied-maintenance')
        self.assertEqual(record['backupBefore'], self.backup)
        self.assertEqual(record['sourceSHA256'], self.binary_sha)
        self.assertEqual(record['expectedLiveRevision'], self.live_revision)
        self.assertEqual(guard.manifest.stat().st_mode & 0o777, 0o600)
        self.assertEqual(guard.manifest.parent.stat().st_mode & 0o777, 0o700)
        self.assertEqual(result['manifestSHA256'], module.digest(guard.manifest))
        guard.complete = True
        guard.stop_if_unfinished()
        self.assertEqual(len(self.calls), 1)

    def test_invalid_backup_or_maintenance_never_runs_candidate(self):
        for variant in ('missing', 'format', 'hash', 'path', 'symlink', 'revision', 'maintenance'):
            with self.subTest(variant=variant):
                backup = dict(self.backup)
                self.calls = []
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
                elif variant == 'revision':
                    backup['revisionBefore'] = 'd' * 40
                self.mysql_values = iter(['0' if variant == 'maintenance' else '1'])
                guard = self.guard()
                with self.assertRaises(AssertionError):
                    module.activate_trial82(self.mysql, self.compose, guard, backup)
                self.assertEqual(self.calls, [])
                guard.stop_if_unfinished()
                self.assertEqual(self.calls, [('stop', '-t', '30', 'app')])
                record = json.loads(guard.manifest.read_text())
                self.assertEqual(record['state'], 'failed-maintenance')
                self.assertEqual(record['failure'], 'AssertionError')
                self.assertEqual(record['backupBefore'], backup)
                self.assertTrue(record['stopCommandSucceeded'])
                self.assertTrue((self.root / self.backup['file']).is_file())

    def test_backup_cli_postcheck_and_later_failure_keep_evidence_and_stop(self):
        guard = self.guard()
        guard.stop_if_unfinished()
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['state'], 'failed-maintenance')
        self.assertIsNone(record['backupBefore'])
        self.assertEqual(record['sourceSHA256'], self.binary_sha)

        for mode in ('cli', 'postcheck', 'interrupted'):
            with self.subTest(mode=mode):
                self.calls = []
                guard = self.guard()
                self.mysql_values = iter(['1', '0' if mode == 'postcheck' else '1'])
                def apply(*args):
                    self.calls.append(args)
                    if mode == 'cli':
                        raise RuntimeError('private detail')
                    if mode == 'interrupted':
                        raise KeyboardInterrupt()
                error = KeyboardInterrupt if mode == 'interrupted' else AssertionError if mode == 'postcheck' else RuntimeError
                with self.assertRaises(error):
                    module.activate_trial82(self.mysql, apply, guard, self.backup)
                guard.stop_if_unfinished()
                raw = guard.manifest.read_text()
                self.assertEqual(json.loads(raw)['state'], 'failed-maintenance')
                self.assertNotIn('private detail', raw)
                self.assertEqual(self.calls[-1], ('stop', '-t', '30', 'app'))

        self.calls = []
        guard = self.guard()
        self.mysql_values = iter(['1', '1'])
        module.activate_trial82(self.mysql, self.compose, guard, self.backup)
        guard.stop_if_unfinished()  # Simulates a later startup, health or recording failure.
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['state'], 'failed-maintenance')
        self.assertEqual(record['backupBefore'], self.backup)
        self.assertEqual(self.calls[-1], ('stop', '-t', '30', 'app'))

    def test_unprepared_guard_still_stops_and_reports_missing_manifest(self):
        for revision, binary, live in (('a' * 41, self.binary_sha, self.live_revision),
                                       (self.revision, 'not-a-digest', self.live_revision),
                                       (self.revision, self.binary_sha, 'c' * 63)):
            with self.subTest(revision=revision, binary=binary, live=live):
                self.calls = []
                guard = module.Trial82CutoverGuard(self.compose, self.root, revision, binary, live)
                with self.assertRaises(AssertionError):
                    guard.prepare()
                output = io.StringIO()
                with contextlib.redirect_stderr(output):
                    guard.stop_if_unfinished()
                self.assertEqual(self.calls, [('stop', '-t', '30', 'app')])
                self.assertIn('manifest was not initialized', output.getvalue())

    def test_guard_redacts_stop_failure(self):
        def fail(*args):
            raise RuntimeError('sensitive subprocess output')
        guard = module.Trial82CutoverGuard(fail, self.root, self.revision, self.binary_sha, self.live_revision)
        guard.prepare()
        output = io.StringIO()
        with contextlib.redirect_stderr(output):
            guard.stop_if_unfinished()
        self.assertIn('could not be stopped', output.getvalue())
        self.assertNotIn('sensitive subprocess output', output.getvalue())
        self.assertFalse(json.loads(guard.manifest.read_text())['stopCommandSucceeded'])

    def test_interruption_runs_guard_and_records_failure(self):
        for signum in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT):
            with self.subTest(signal=signum):
                directory = self.root / ('signal-' + str(signum))
                directory.mkdir()
                marker = directory / 'stopped'
                source = (
                    'import atexit, pathlib, signal, sys\n'
                    'sys.path.insert(0, ' + repr(str(HERE)) + ')\n'
                    'from trial82_activation import Trial82CutoverGuard\n'
                    'def stop(*args): pathlib.Path(' + repr(str(marker)) + ').write_text(" ".join(args))\n'
                    'guard = Trial82CutoverGuard(stop, pathlib.Path(' + repr(str(directory)) + '), "a"*40, "b"*64, "c"*40)\n'
                    'atexit.register(guard.stop_if_unfinished)\n'
                    'guard.install_signal_handlers()\n'
                    'guard.prepare()\n'
                    'print("ready", flush=True)\n'
                    'signal.pause()\n'
                )
                process = subprocess.Popen([sys.executable, '-c', source], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                try:
                    self.assertEqual(process.stdout.readline().strip(), 'ready')
                    os.kill(process.pid, signum)
                    process.communicate(timeout=5)
                    self.assertNotEqual(process.returncode, 0)
                    self.assertEqual(marker.read_text(), 'stop -t 30 app')
                    path = next(directory.glob('trial82-*/manifest.json'))
                    self.assertEqual(json.loads(path.read_text())['state'], 'failed-maintenance')
                finally:
                    if process.poll() is None:
                        process.kill()
                        process.communicate(timeout=5)

    def test_existing_ingress_only(self):
        available = self.root / 'sites-available' / 'hyjs-test.conf'
        enabled = self.root / 'sites-enabled' / 'hyjs-test.conf'
        available.parent.mkdir()
        enabled.parent.mkdir()
        with self.assertRaises(AssertionError):
            module.require_existing_ingress(available, enabled)
        available.write_text('owned site')
        with self.assertRaises(AssertionError):
            module.require_existing_ingress(available, enabled)
        other = self.root / 'other.conf'
        other.write_text('other site')
        enabled.symlink_to(other)
        with self.assertRaises(AssertionError):
            module.require_existing_ingress(available, enabled)
        enabled.unlink()
        enabled.symlink_to(available)
        module.require_existing_ingress(available, enabled)
        self.assertEqual(available.read_text(), 'owned site')
        available.unlink()
        available.symlink_to(other)
        with self.assertRaises(AssertionError):
            module.require_existing_ingress(available, enabled)

    def test_invalid_flags_rejected_before_external_work(self):
        live = self.live_revision
        calendar = '00000000-0000-0000-0000-000000000001'
        common = [
            ['--apply-trial82'],
            ['--apply-trial82', '--expected-live-revision', 'a' * 41],
            ['--apply-trial82', '--expected-live-revision', 'A' * 40],
            *[['--apply-trial82', flag, '--expected-live-revision', live] for flag in (
                '--apply-growth74', '--apply-growth75', '--apply-activity77', '--apply-feedback78',
                '--apply-feedback79', '--apply-feedback80', '--apply-feedback81', '--migrate-role-wallets')],
            ['--apply-trial82', '--warm-rush-history-player', '1', '--expected-live-revision', live],
            ['--apply-trial82', '--restore-activity-calendar', calendar, '--expected-live-revision', live],
        ]
        for script, extra in (('deploy.py', ['--provision']), ('deploy.py', ['--initial-dump', '/tmp/not-a-dump']),
                              ('activate.py', ['--initial-db']), ('activate.py', ['--initial-db', '--provision-ingress'])):
            base = [sys.executable, str(HERE / script)]
            if script == 'activate.py':
                base.append('/not/a/real/stage')
            for options in common + [['--apply-trial82', *extra, '--expected-live-revision', live]]:
                result = subprocess.run(base + options, capture_output=True, text=True)
                self.assertNotEqual(result.returncode, 0, (script, options))
                self.assertIn('AssertionError', result.stderr, (script, options, result.stderr))
                self.assertNotIn('FileNotFoundError', result.stderr, (script, options))

    def test_existing_ingress_option_allowed_and_live_revision_checked(self):
        result = subprocess.run([sys.executable, str(HERE / 'deploy.py'), '--check-config', '--env-file',
                                 str(HERE / '.env.example'), '--apply-trial82', '--expected-live-revision',
                                 self.live_revision, '--update-ingress'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('no external action performed', result.stdout)
        stage = self.root / 'owned' / '.incoming' / 'candidate'
        stage.mkdir(parents=True)
        (stage.parent.parent / '.hyjs-owned').write_text('')
        (stage.parent.parent / 'deployed.json').write_text(json.dumps({'revision': 'd' * 40}))
        (stage / 'artifact.json').write_text('{}')
        result = subprocess.run([sys.executable, str(HERE / 'activate.py'), str(stage), '--apply-trial82',
                                 '--expected-live-revision', self.live_revision, '--provision-ingress'],
                                capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Live revision changed', result.stderr)
        self.assertFalse((stage.parent.parent / 'backups').exists())

    def test_existing_warm_all_gate_runs_without_a_manual_player(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        start = next(i for i, n in enumerate(tree.body) if isinstance(n, ast.For)
                     and isinstance(n.target, ast.Name) and n.target.id == 'player_id'
                     and '--warm-rush-history-player' in ast.get_source_segment(source, n))
        end = next(i for i in range(start + 1, len(tree.body)) if isinstance(tree.body[i], ast.If)
                   and ast.get_source_segment(source, tree.body[i].test) == 'args.apply_growth74')
        block = compile(ast.Module(body=tree.body[start:end], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        calls = []
        namespace = {'args': SimpleNamespace(warm_rush_history_player=[], apply_trial82=True),
                     'compose': lambda *args: calls.append(args), 'manifest': {}}
        exec(block, namespace)
        self.assertEqual(calls, [('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--tables', '/app/tables',
                                  '--warm-rush-history-all', '--log-file', '/app/logs/rush-history-warm.log')])
        self.assertEqual(namespace['manifest']['prewarmedRushPlayers'], [])
        self.assertTrue(namespace['manifest']['staleRushHistoriesVerified'])

    def test_real_forwarding_and_guard_order(self):
        source = (HERE / 'deploy.py').read_text()
        tree = ast.parse(source)
        start = next(i for i, n in enumerate(tree.body) if isinstance(n, ast.Assign) and any(isinstance(t, ast.Name) and t.id == 'command' for t in n.targets))
        end = next(i for i in range(start, len(tree.body)) if isinstance(tree.body[i], ast.Expr) and isinstance(tree.body[i].value, ast.Call) and isinstance(tree.body[i].value.func, ast.Name) and tree.body[i].value.func.id == 'print')
        block = compile(ast.Module(body=tree.body[start:end], type_ignores=[]), str(HERE / 'deploy.py'), 'exec')
        for enabled in (False, True):
            args = SimpleNamespace(warm_rush_history_player=[], expected_live_revision=self.live_revision,
                                   apply_growth74=False, apply_growth75=False, apply_activity77=False,
                                   apply_feedback78=False, apply_feedback79=False, apply_feedback80=False,
                                   apply_feedback81=False, apply_trial82=enabled, restore_activity_calendar=None,
                                   migrate_role_wallets=False, provision=False, update_ingress=True, initial_dump=None)
            namespace = {'args': args, 'remote_stage': '/tmp/candidate', 'shlex': shlex}
            exec(block, namespace)
            argv = shlex.split(namespace['command'])
            self.assertEqual('--apply-trial82' in argv, enabled)
            self.assertIn('--provision-ingress', argv)
            self.assertEqual(argv[argv.index('--expected-live-revision') + 1], self.live_revision)
        self.assertIn("'trial82_activation.py'", source)
        activate = (HERE / 'activate.py').read_text()
        ordered = ["atexit.register(trial82_guard.stop_if_unfinished)", "trial82_guard.install_signal_handlers()",
                   "trial82_guard.prepare()", "compose('stop', '-t', '30', 'app')",
                   "verified_backup = backup_existing_database()", "trial82_activation.activate_trial82(",
                   "compose('up', '-d', '--no-deps', '--force-recreate', 'app')", "'Maintenance changed after health check'",
                   "(ROOT / 'deployed.json').write_text", "trial82_guard.complete = True"]
        positions = [activate.index(marker, activate.index(ordered[0])) for marker in ordered]
        self.assertEqual(positions, sorted(positions))
        self.assertEqual(activate.count('trial82_activation.require_existing_ingress('), 2)


if __name__ == '__main__':
    unittest.main()
