import ast
import contextlib
import hashlib
import io
import json
import os
import select
from pathlib import Path
import shlex
import signal
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import trial82_activation as module


def cutover_block():
    # Exercise the actual top-level cutover block with isolated I/O replacements.
    source = (HERE / 'activate.py').read_text()
    tree = ast.parse(source)
    matches = [node for node in tree.body if isinstance(node, ast.If)
               and 'trial82_guard = trial82_activation.Trial82CutoverGuard(' in ast.get_source_segment(source, node)
               and "compose('stop', '-t', '30', 'app')" in ast.get_source_segment(source, node)]
    assert len(matches) == 1
    return compile(ast.Module(body=matches, type_ignores=[]), str(HERE / 'activate.py'), 'exec')


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

    def guard(self, *, mode='configuration'):
        guard = module.Trial82CutoverGuard(self.compose, self.root, self.revision, self.binary_sha, self.live_revision, mode=mode)
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

    def test_combined_activation_records_each_cli_after_one_backup(self):
        guard = self.guard()
        self.mysql_values = iter(['1', '1', '1'])
        result = module.activate_trial82(self.mysql, self.compose, guard, self.backup,
                                         apply_trial82=True, apply_feedback84=True)
        self.assertEqual([call[call.index('--tables') + 2] for call in self.calls],
                         ['--apply-trial82', '--apply-feedback84'])
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['requestedCommands'], ['--apply-trial82', '--apply-feedback84'])
        self.assertEqual(record['commands'], [{'command': '--apply-trial82', 'status': 'completed'},
                                              {'command': '--apply-feedback84', 'status': 'completed'}])
        self.assertEqual(record['backupBefore'], self.backup)
        self.assertEqual(result['appliedCommands'], record['requestedCommands'])
        self.assertEqual(result['manifestSHA256'], module.digest(guard.manifest))
        guard.complete = True
        guard.stop_if_unfinished()
        self.assertEqual(len(self.calls), 2)

    def test_feedback84_alone_uses_the_same_guard(self):
        guard = self.guard()
        result = module.activate_trial82(self.mysql, self.compose, guard, self.backup,
                                         apply_trial82=False, apply_feedback84=True)
        self.assertEqual(len(self.calls), 1)
        self.assertIn('--apply-feedback84', self.calls[0])
        self.assertNotIn('--apply-trial82', self.calls[0])
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['commands'], [{'command': '--apply-feedback84', 'status': 'completed'}])
        self.assertEqual(result['appliedCommands'], ['--apply-feedback84'])
        guard.complete = True
        guard.stop_if_unfinished()
        self.assertEqual(len(self.calls), 1)

    def test_combined_feedback85_uses_authenticated_backup_and_maintenance(self):
        guard = self.guard()
        self.mysql_values = iter(['1', '1', '1', '1'])
        result = module.activate_trial82(self.mysql, self.compose, guard, self.backup,
                                         apply_trial82=True, apply_feedback84=True, apply_feedback85=True)
        commands = ['--apply-trial82', '--apply-feedback84', '--apply-feedback85']
        self.assertEqual([call[call.index('--tables') + 2] for call in self.calls], commands)
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['requestedCommands'], commands)
        self.assertEqual(result['appliedCommands'], commands)
        self.assertEqual(record['backupBefore'], self.backup)
        self.assertEqual(record['state'], 'applied-maintenance')
        guard.complete = True

    def test_feedback85_failure_records_and_stops_under_maintenance(self):
        guard = self.guard()
        self.mysql_values = iter(['1'])
        def fail(*args):
            self.calls.append(args)
            raise RuntimeError('private candidate error')
        with self.assertRaises(RuntimeError):
            module.activate_trial82(self.mysql, fail, guard, self.backup,
                                    apply_trial82=False, apply_feedback85=True)
        guard.stop_if_unfinished()
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['requestedCommands'], ['--apply-feedback85'])
        self.assertEqual(record['commands'], [{'command': '--apply-feedback85', 'status': 'failed'}])
        self.assertEqual(record['state'], 'failed-maintenance')
        self.assertEqual(self.calls[-1], ('stop', '-t', '30', 'app'))
        self.assertNotIn('private candidate error', json.dumps(record))

    def test_combined_feedback84_failure_stops_and_records_completed_trial82(self):
        guard = self.guard()
        self.mysql_values = iter(['1', '1'])
        def fail_second(*args):
            self.calls.append(args)
            if '--apply-feedback84' in args:
                raise RuntimeError('private candidate error')
        with self.assertRaises(RuntimeError):
            module.activate_trial82(self.mysql, fail_second, guard, self.backup,
                                    apply_trial82=True, apply_feedback84=True)
        guard.stop_if_unfinished()
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['state'], 'failed-maintenance')
        self.assertEqual(record['failureStage'], 'candidate-cli-feedback84-running')
        self.assertEqual(record['commands'], [{'command': '--apply-trial82', 'status': 'completed'},
                                              {'command': '--apply-feedback84', 'status': 'failed'}])
        self.assertEqual(record['backupBefore'], self.backup)
        self.assertEqual(self.calls[-1], ('stop', '-t', '30', 'app'))
        self.assertNotIn('private candidate error', guard.manifest.read_text())

        # A fresh guarded retry invokes both audited commands again; each CLI
        # owns its original idempotency and drift checks.
        self.calls = []
        self.mysql_values = iter(['1', '1', '1'])
        retry = self.guard()
        result = module.activate_trial82(self.mysql, self.compose, retry, self.backup,
                                         apply_trial82=True, apply_feedback84=True)
        self.assertEqual(result['appliedCommands'], ['--apply-trial82', '--apply-feedback84'])
        retry.complete = True

    def test_combined_maintenance_drift_stops_before_next_step(self):
        for values, expected, stage in ((['1', '0'], ['--apply-trial82'], 'checking-maintenance-after-trial82'),
                                        (['1', '1', '0'], ['--apply-trial82', '--apply-feedback84'], 'checking-maintenance-after-feedback84')):
            with self.subTest(stage=stage):
                self.calls = []
                self.mysql_values = iter(values)
                guard = self.guard()
                with self.assertRaises(AssertionError):
                    module.activate_trial82(self.mysql, self.compose, guard, self.backup,
                                            apply_trial82=True, apply_feedback84=True)
                guard.stop_if_unfinished()
                record = json.loads(guard.manifest.read_text())
                self.assertEqual(record['failureStage'], stage)
                self.assertEqual([row['command'] for row in record['commands']], expected)
                self.assertTrue(all(row['status'] == 'completed' for row in record['commands']))
                self.assertEqual(self.calls[-1], ('stop', '-t', '30', 'app'))

    def test_feedback84_bad_backup_stops_without_candidate(self):
        guard = self.guard()
        backup = dict(self.backup, sha256='0' * 64)
        with self.assertRaises(AssertionError):
            module.activate_trial82(self.mysql, self.compose, guard, backup,
                                    apply_trial82=False, apply_feedback84=True)
        guard.stop_if_unfinished()
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['failureStage'], 'checking-backup')
        self.assertEqual(record['requestedCommands'], ['--apply-feedback84'])
        self.assertEqual(record['commands'], [])
        self.assertEqual(self.calls, [('stop', '-t', '30', 'app')])

    def test_backup_creation_failure_records_stage_and_stops(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        node = next(n for n in tree.body if isinstance(n, ast.If)
                    and ast.get_source_segment(source, n.test) == "args.with_backup and (ROOT / 'migration.json').exists()"
                    and 'verified_backup = backup_existing_database()' in ast.get_source_segment(source, n))
        block = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        (self.root / 'migration.json').write_text('{}')
        guard = self.guard()
        guard.record['requestedCommands'] = ['--apply-feedback84']
        def fail_backup():
            raise OSError('private backup failure')
        namespace = {'ROOT': self.root, 'args': SimpleNamespace(with_backup=True, apply_trial82=False, apply_feedback84=True, apply_feedback85=False, maintenance_redeploy=False),
                     'trial82_guard': guard, 'backup_existing_database': fail_backup}
        with self.assertRaises(OSError):
            exec(block, namespace)
        guard.stop_if_unfinished()
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['state'], 'failed-maintenance')
        self.assertEqual(record['failureStage'], 'backing-up')
        self.assertIsNone(record['backupBefore'])
        self.assertEqual(record['requestedCommands'], ['--apply-feedback84'])
        self.assertEqual(self.calls, [('stop', '-t', '30', 'app')])
        self.assertNotIn('private backup failure', guard.manifest.read_text())

    def test_empty_activation_selection_is_rejected(self):
        guard = self.guard()
        with self.assertRaises(AssertionError):
            module.activate_trial82(self.mysql, self.compose, guard, self.backup,
                                    apply_trial82=False, apply_feedback84=False)
        guard.stop_if_unfinished()
        self.assertEqual(self.calls, [('stop', '-t', '30', 'app')])

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

    def cutover_namespace(self, root, *, trial=True, feedback=False, feedback85=False, redeploy=False, mysql=None, compose=None, binary=None):
        callbacks = []
        flags = dict(apply_growth74=False, apply_growth75=False, apply_activity77=False,
                     apply_feedback78=False, apply_feedback79=False, apply_feedback80=False,
                     apply_feedback81=False, apply_trial82=trial, apply_feedback84=feedback, apply_feedback85=feedback85, maintenance_redeploy=redeploy,
                     expected_live_revision=self.live_revision, initial_db=False)
        namespace = {'ROOT': root, 'args': SimpleNamespace(**flags),
                     'manifest': {'revision': self.revision, 'files': {'app/hyjs-server': binary or self.binary_sha}},
                     'mysql': mysql or (lambda _query: '1'), 'compose': compose or self.compose,
                     'atexit': SimpleNamespace(register=callbacks.append)}
        return namespace, callbacks

    def test_mysql_readiness_retries_wrapped_failure_and_fails_closed(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        loops = [node for node in tree.body if isinstance(node, ast.For)
                 and "mysql('SELECT 1;')" in ast.get_source_segment(source, node)
                 and 'HYJS MySQL did not become ready' in ast.get_source_segment(source, node)]
        self.assertEqual(len(loops), 1)
        block = compile(ast.Module(body=loops, type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        for failures in (2, 80):
            with self.subTest(failures=failures):
                calls, sleeps = [], []
                def mysql(query):
                    self.assertEqual(query, 'SELECT 1;')
                    calls.append(query)
                    if len(calls) <= failures:
                        raise RuntimeError('wrapped temporary failure')
                    return '1'
                namespace = {'mysql': mysql, 'time': SimpleNamespace(sleep=sleeps.append), 'subprocess': subprocess}
                if failures == 80:
                    with self.assertRaisesRegex(SystemExit, 'HYJS MySQL did not become ready'):
                        exec(block, namespace)
                    self.assertEqual((len(calls), sleeps), (80, [2] * 80))
                else:
                    exec(block, namespace)
                    self.assertEqual((len(calls), sleeps), (3, [2, 2]))

    def test_mysql_readiness_handles_legacy_error_and_propagates_other_errors(self):
        source = (HERE / 'activate.py').read_text()
        node = next(n for n in ast.parse(source).body if isinstance(n, ast.For)
                    and "mysql('SELECT 1;')" in ast.get_source_segment(source, n)
                    and 'HYJS MySQL did not become ready' in ast.get_source_segment(source, n))
        block = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        calls, sleeps = [], []
        def legacy(_query):
            calls.append(1)
            if len(calls) == 1:
                raise subprocess.CalledProcessError(1, 'synthetic')
            return '1'
        exec(block, {'mysql': legacy, 'time': SimpleNamespace(sleep=sleeps.append), 'subprocess': subprocess})
        self.assertEqual((len(calls), sleeps), (2, [2]))
        def unexpected(_query):
            raise ValueError('unrelated defect')
        with self.assertRaisesRegex(ValueError, 'unrelated defect'):
            exec(block, {'mysql': unexpected, 'time': SimpleNamespace(sleep=sleeps.append), 'subprocess': subprocess})
        self.assertEqual(sleeps, [2])

    def test_real_cutover_preflight_failures_do_not_stop_app(self):
        for selection in ((True, False), (False, True), (True, True)):
            for failure in ('missing-migration', 'symlink-migration', 'maintenance-off', 'maintenance-error',
                            'prepare-bad-binary', 'prepare-write-error', 'requested-write-error'):
                with self.subTest(selection=selection, failure=failure):
                    root = self.root / (str(selection) + failure)
                    root.mkdir(); (root / 'backups').mkdir()
                    if failure == 'symlink-migration':
                        target = root / 'other.json'; target.write_text('{}')
                        (root / 'migration.json').symlink_to(target)
                    elif failure != 'missing-migration':
                        (root / 'migration.json').write_text('{}')
                    calls = []
                    def mysql(query):
                        self.assertEqual(query, 'SELECT maintenance FROM client_update_policy WHERE id=1;')
                        if failure == 'maintenance-error':
                            raise OSError('preflight read failed')
                        return '0' if failure == 'maintenance-off' else '1'
                    namespace, callbacks = self.cutover_namespace(
                        root, trial=selection[0], feedback=selection[1], mysql=mysql,
                        compose=lambda *args: calls.append(args),
                        binary='bad' if failure == 'prepare-bad-binary' else self.binary_sha)
                    with contextlib.ExitStack() as patches:
                        patches.enter_context(mock.patch.object(module.Trial82CutoverGuard, 'install_signal_handlers'))
                        if failure == 'prepare-write-error':
                            patches.enter_context(mock.patch.object(module, 'private_write', side_effect=OSError('disk failure')))
                        elif failure == 'requested-write-error':
                            original_write = module.private_write
                            writes = []
                            def fail_second_write(*args):
                                writes.append(1)
                                if len(writes) == 2:
                                    raise OSError('requested command checkpoint failed')
                                return original_write(*args)
                            patches.enter_context(mock.patch.object(module, 'private_write', side_effect=fail_second_write))
                        with self.assertRaises((AssertionError, OSError)):
                            exec(cutover_block(), namespace)
                    for callback in reversed(callbacks):
                        callback()
                    self.assertEqual(callbacks, [], 'preflight must not arm the stop guard')
                    self.assertEqual(calls, [], 'preflight must not stop or invoke candidate commands')

    def test_real_cutover_arms_before_stop_and_preserves_selection(self):
        for trial, feedback in ((True, False), (False, True), (True, True)):
            with self.subTest(trial=trial, feedback=feedback):
                root = self.root / ('success-%s-%s' % (trial, feedback))
                root.mkdir(); (root / 'backups').mkdir(); (root / 'migration.json').write_text('{}')
                calls = []
                namespace, callbacks = self.cutover_namespace(root, trial=trial, feedback=feedback,
                                                               compose=lambda *args: calls.append(args))
                with mock.patch.object(module.Trial82CutoverGuard, 'install_signal_handlers') as install:
                    exec(cutover_block(), namespace)
                self.assertEqual(len(callbacks), 1)
                install.assert_called_once_with()
                guard = namespace['trial82_guard']
                expected = [flag for flag, selected in (('--apply-trial82', trial), ('--apply-feedback84', feedback)) if selected]
                self.assertEqual(json.loads(guard.manifest.read_text())['requestedCommands'], expected)
                self.assertEqual(calls, [('stop', '-t', '30', 'app')])
                guard.complete = True
                callbacks[0]()
                self.assertEqual(calls, [('stop', '-t', '30', 'app')])

    def test_feedback85_cutover_arms_only_after_preconditions(self):
        root = self.root / 'feedback85-only'
        root.mkdir(); (root / 'backups').mkdir(); (root / 'migration.json').write_text('{}')
        calls = []
        namespace, callbacks = self.cutover_namespace(root, trial=False, feedback85=True,
                                                       compose=lambda *args: calls.append(args))
        with mock.patch.object(module.Trial82CutoverGuard, 'install_signal_handlers'):
            exec(cutover_block(), namespace)
        guard = namespace['trial82_guard']
        self.assertEqual(json.loads(guard.manifest.read_text())['requestedCommands'], ['--apply-feedback85'])
        self.assertEqual(calls, [('stop', '-t', '30', 'app')])
        guard.complete = True
        callbacks[0]()
        self.assertEqual(len(calls), 1)

    def test_real_cutover_failure_after_arming_stops_and_records(self):
        root = self.root / 'post-arm'; root.mkdir(); (root / 'backups').mkdir()
        (root / 'migration.json').write_text('{}')
        calls = []
        def fail_first_stop(*args):
            calls.append(args)
            if len(calls) == 1:
                raise OSError('first stop failed')
        namespace, callbacks = self.cutover_namespace(root, trial=True, feedback=True, compose=fail_first_stop)
        with mock.patch.object(module.Trial82CutoverGuard, 'install_signal_handlers'):
            with self.assertRaises(OSError):
                exec(cutover_block(), namespace)
        self.assertEqual(len(callbacks), 1)
        callbacks[0]()
        self.assertEqual(calls, [('stop', '-t', '30', 'app'), ('stop', '-t', '30', 'app')])
        record = json.loads(namespace['trial82_guard'].manifest.read_text())
        self.assertEqual(record['state'], 'failed-maintenance')
        self.assertEqual(record['failureStage'], 'awaiting-backup')
        self.assertEqual(record['requestedCommands'], ['--apply-trial82', '--apply-feedback84'])
        self.assertTrue(record['stopCommandSucceeded'])
        self.assertNotIn('first stop failed', json.dumps(record))

    def test_signal_handler_install_failure_before_registration_does_not_stop(self):
        root = self.root / 'signal-install-failure'; root.mkdir(); (root / 'backups').mkdir()
        (root / 'migration.json').write_text('{}')
        calls = []
        namespace, callbacks = self.cutover_namespace(root, compose=lambda *args: calls.append(args))
        with mock.patch.object(module.Trial82CutoverGuard, 'install_signal_handlers', side_effect=OSError('install failed')):
            with self.assertRaises(OSError):
                exec(cutover_block(), namespace)
        self.assertEqual(callbacks, [])
        self.assertEqual(calls, [])

    def test_failure_immediately_after_registration_stops(self):
        root = self.root / 'registration-failure'; root.mkdir(); (root / 'backups').mkdir()
        (root / 'migration.json').write_text('{}')
        calls = []
        namespace, callbacks = self.cutover_namespace(root, compose=lambda *args: calls.append(args))
        def register_then_fail(callback):
            callbacks.append(callback)
            raise OSError('failure after registration')
        namespace['atexit'] = SimpleNamespace(register=register_then_fail)
        with mock.patch.object(module.Trial82CutoverGuard, 'install_signal_handlers'):
            with self.assertRaises(OSError):
                exec(cutover_block(), namespace)
        self.assertEqual(len(callbacks), 1)
        self.assertEqual(calls, [])
        callbacks[0]()
        self.assertEqual(calls, [('stop', '-t', '30', 'app')])
        self.assertEqual(json.loads(namespace['trial82_guard'].manifest.read_text())['state'], 'failed-maintenance')

    def test_signal_during_prepare_or_after_arming_uses_real_cutover(self):
        source = r"""
import atexit, pathlib, signal, sys
from types import SimpleNamespace
sys.path.insert(0, sys.argv[1])
import test_trial82_activation as tests
import trial82_activation as activation
root = pathlib.Path(sys.argv[2]); marker = root / 'stopped'; mode = sys.argv[3]
args = SimpleNamespace(apply_growth74=False, apply_growth75=False, apply_activity77=False,
                       apply_feedback78=False, apply_feedback79=False, apply_feedback80=False,
                       apply_feedback81=False, apply_trial82=True, apply_feedback84=True, apply_feedback85=False, maintenance_redeploy=False,
                       expected_live_revision='c'*40, initial_db=False)
manifest = {'revision':'a'*40, 'files':{'app/hyjs-server':'b'*64}}
count = [0]
def compose(*args):
    count[0] += 1
    if mode == 'after' and count[0] == 1:
        print('ready', flush=True); signal.pause()
    else:
        marker.write_text(' '.join(args))
def hold_prepare(*args):
    print('ready', flush=True); signal.pause()
if mode == 'before': activation.private_write = hold_prepare
if mode == 'before-register':
    install = activation.Trial82CutoverGuard.install_signal_handlers
    def hold_install(guard):
        install(guard)
        print('ready', flush=True); signal.pause()
    activation.Trial82CutoverGuard.install_signal_handlers = hold_install
namespace = {'ROOT':root, 'args':args, 'manifest':manifest, 'mysql':lambda query:'1',
             'compose':compose, 'atexit':atexit}
exec(tests.cutover_block(), namespace)
"""
        for mode in ('before', 'before-register', 'after'):
            for signum in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT):
                with self.subTest(mode=mode, signal=signum):
                    root = self.root / ('boundary-%s-%d' % (mode, signum))
                    root.mkdir(); (root / 'backups').mkdir(); (root / 'migration.json').write_text('{}')
                    process = subprocess.Popen([sys.executable, '-c', source, str(HERE), str(root), mode],
                                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                    try:
                        ready, _, _ = select.select([process.stdout], [], [], 5)
                        self.assertTrue(ready, 'child did not reach the intended signal boundary')
                        self.assertEqual(process.stdout.readline().strip(), 'ready')
                        os.kill(process.pid, signum)
                        process.communicate(timeout=5)
                        self.assertNotEqual(process.returncode, 0)
                        marker = root / 'stopped'
                        if mode != 'after':
                            self.assertFalse(marker.exists(), 'preflight signal must not stop the app')
                        else:
                            self.assertEqual(marker.read_text(), 'stop -t 30 app')
                            path = next((root / 'backups').glob('trial82-*/manifest.json'))
                            record = json.loads(path.read_text())
                            self.assertEqual(record['state'], 'failed-maintenance')
                            self.assertEqual(record['requestedCommands'], ['--apply-trial82', '--apply-feedback84'])
                    finally:
                        if process.poll() is None:
                            process.kill(); process.communicate(timeout=5)

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
                    'guard.prepare()\n'
                    'guard.install_signal_handlers()\n'
                    'atexit.register(guard.stop_if_unfinished)\n'
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
        for selection in (['--apply-trial82'], ['--apply-feedback84'], ['--apply-feedback85'], ['--apply-trial82', '--apply-feedback84', '--apply-feedback85']):
            common = [
                selection,
                [*selection, '--expected-live-revision', 'a' * 41],
                [*selection, '--expected-live-revision', 'A' * 40],
                *[[*selection, flag, '--expected-live-revision', live] for flag in (
                    '--apply-growth74', '--apply-growth75', '--apply-activity77', '--apply-feedback78',
                    '--apply-feedback79', '--apply-feedback80', '--apply-feedback81', '--migrate-role-wallets')],
                [*selection, '--warm-rush-history-player', '1', '--expected-live-revision', live],
                [*selection, '--restore-activity-calendar', calendar, '--expected-live-revision', live],
            ]
            for script, extra in (('deploy.py', ['--provision']), ('deploy.py', ['--initial-dump', '/tmp/not-a-dump']),
                                  ('activate.py', ['--initial-db']), ('activate.py', ['--initial-db', '--provision-ingress'])):
                base = [sys.executable, '-B', str(HERE / script), '--with-backup']
                if script == 'activate.py':
                    base.append('/not/a/real/stage')
                for options in common + [[*selection, *extra, '--expected-live-revision', live]]:
                    result = subprocess.run(base + options, capture_output=True, text=True)
                    self.assertNotEqual(result.returncode, 0, (script, options))
                    self.assertIn('AssertionError', result.stderr, (script, options, result.stderr))
                    self.assertNotIn('FileNotFoundError', result.stderr, (script, options))

    def test_existing_ingress_option_allowed_and_live_revision_checked(self):
        for selection in (['--apply-trial82'], ['--apply-feedback84'], ['--apply-feedback85'], ['--apply-trial82', '--apply-feedback84', '--apply-feedback85']):
            result = subprocess.run([sys.executable, str(HERE / 'deploy.py'), '--check-config', '--env-file',
                                     str(HERE / '.env.example'), *selection, '--expected-live-revision',
                                     self.live_revision, '--update-ingress', '--with-backup'], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn('no external action performed', result.stdout)
        stage = self.root / 'owned' / '.incoming' / 'candidate'
        stage.mkdir(parents=True)
        (stage.parent.parent / '.hyjs-owned').write_text('')
        (stage.parent.parent / 'deployed.json').write_text(json.dumps({'revision': 'd' * 40}))
        (stage / 'artifact.json').write_text('{}')
        result = subprocess.run([sys.executable, '-B', str(HERE / 'activate.py'), str(stage), '--with-backup', '--apply-trial82',
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
        for redeploy in (False, True):
            with self.subTest(redeploy=redeploy):
                calls = []
                guard = SimpleNamespace(record={}, checkpoint=lambda: None)
                namespace = {'args': SimpleNamespace(warm_rush_history_player=[], apply_trial82=not redeploy, apply_feedback84=False, apply_feedback85=False, maintenance_redeploy=redeploy),
                             'compose': lambda *args: calls.append(args), 'manifest': {}, 'trial82_guard': guard,
                             'mysql': lambda _query: '1'}
                exec(block, namespace)
                self.assertEqual(calls, [('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--tables', '/app/tables',
                                          '--warm-rush-history-all', '--log-file', '/app/logs/rush-history-warm.log')])
                self.assertEqual(namespace['manifest']['prewarmedRushPlayers'], [])
                self.assertTrue(namespace['manifest']['staleRushHistoriesVerified'])
                self.assertEqual(guard.record['state'], 'checking-maintenance-after-warm' if redeploy else 'rush-history-verified')

    def test_real_forwarding_and_guard_order(self):
        source = (HERE / 'deploy.py').read_text()
        tree = ast.parse(source)
        start = next(i for i, n in enumerate(tree.body) if isinstance(n, ast.Assign) and any(isinstance(t, ast.Name) and t.id == 'command' for t in n.targets))
        end = next(i for i in range(start, len(tree.body)) if isinstance(tree.body[i], ast.Expr) and isinstance(tree.body[i].value, ast.Call) and isinstance(tree.body[i].value.func, ast.Name) and tree.body[i].value.func.id == 'print')
        block = compile(ast.Module(body=tree.body[start:end], type_ignores=[]), str(HERE / 'deploy.py'), 'exec')
        for trial_enabled, feedback_enabled, feedback85_enabled in ((False, False, False), (True, False, False), (False, True, False), (True, True, False), (False, False, True), (True, True, True)):
            args = SimpleNamespace(with_backup=True, warm_rush_history_player=[], expected_live_revision=self.live_revision,
                                   apply_growth74=False, apply_growth75=False, apply_activity77=False,
                                   apply_feedback78=False, apply_feedback79=False, apply_feedback80=False,
                                   apply_feedback81=False, apply_trial82=trial_enabled, apply_feedback84=feedback_enabled, apply_feedback85=feedback85_enabled, restore_activity_calendar=None,
                                   migrate_role_wallets=False, provision=False, update_ingress=True, initial_dump=None, maintenance_redeploy=False)
            namespace = {'args': args, 'remote_stage': '/tmp/candidate', 'shlex': shlex}
            exec(block, namespace)
            argv = shlex.split(namespace['command'])
            self.assertIn('--with-backup', argv)
            self.assertEqual('--apply-trial82' in argv, trial_enabled)
            self.assertEqual('--apply-feedback84' in argv, feedback_enabled)
            self.assertEqual('--apply-feedback85' in argv, feedback85_enabled)
            self.assertIn('--provision-ingress', argv)
            self.assertEqual(argv[argv.index('--expected-live-revision') + 1], self.live_revision)
        self.assertIn("'trial82_activation.py'", source)
        activate = (HERE / 'activate.py').read_text()
        ordered = ["assert (ROOT / 'migration.json').is_file() and not (ROOT / 'migration.json').is_symlink()",
                   "assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;')",
                   "trial82_guard.prepare()", "trial82_guard.record['requestedCommands']", "trial82_guard.checkpoint()",
                   "trial82_guard.install_signal_handlers()", "atexit.register(trial82_guard.stop_if_unfinished)",
                   "compose('stop', '-t', '30', 'app')", "verified_backup = backup_existing_database()",
                   "trial82_activation.activate_trial82(", "compose('up', '-d', '--no-deps', '--force-recreate', 'app')",
                   "'Maintenance changed after health check'", "(ROOT / 'deployed.json').write_text", "trial82_guard.complete = True"]
        cutover = activate.index('if args.apply_growth74 or args.apply_growth75 or args.apply_activity77 or args.apply_feedback78')
        positions = [activate.index(marker, cutover) for marker in ordered]
        self.assertEqual(positions, sorted(positions))
        self.assertEqual(activate.count('trial82_activation.require_existing_ingress('), 2)

    def test_deployment_record_contains_only_actual_selected_results(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        node = next(n for n in tree.body if isinstance(n, ast.If)
                    and 'trial82_activation.activate_trial82(' in ast.get_source_segment(source, n)
                    and 'manifest[\'trial82Applied\']' in ast.get_source_segment(source, n))
        block = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        for trial_enabled, feedback_enabled, feedback85_enabled in ((True, False, False), (False, True, False), (True, True, False), (False, False, True), (True, True, True)):
            with self.subTest(trial=trial_enabled, feedback=feedback_enabled, feedback85=feedback85_enabled):
                calls = []
                def apply(mysql, compose, guard, backup, *, apply_trial82, apply_feedback84, apply_feedback85):
                    calls.append((apply_trial82, apply_feedback84, apply_feedback85, backup))
                    return {'state': 'applied-maintenance', 'appliedCommands':
                            [flag for flag, selected in (('--apply-trial82', apply_trial82),
                                                         ('--apply-feedback84', apply_feedback84), ('--apply-feedback85', apply_feedback85)) if selected]}
                manifest = {}
                namespace = {'args': SimpleNamespace(apply_trial82=trial_enabled, apply_feedback84=feedback_enabled, apply_feedback85=feedback85_enabled),
                             'trial82_activation': SimpleNamespace(activate_trial82=apply),
                             'mysql': object(), 'compose': object(), 'trial82_guard': object(),
                             'verified_backup': self.backup, 'manifest': manifest}
                exec(block, namespace)
                self.assertEqual(calls, [(trial_enabled, feedback_enabled, feedback85_enabled, self.backup)])
                self.assertEqual('trial82Applied' in manifest, trial_enabled)
                self.assertEqual('feedback84Applied' in manifest, feedback_enabled)
                self.assertEqual('feedback85Applied' in manifest, feedback85_enabled)
                for result in manifest.values():
                    self.assertEqual(result['appliedCommands'],
                                     [flag for flag, selected in (('--apply-trial82', trial_enabled),
                                                                  ('--apply-feedback84', feedback_enabled), ('--apply-feedback85', feedback85_enabled)) if selected])

    def test_deployment_record_pins_latest_private_manifest(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        node = next(n for n in tree.body if isinstance(n, ast.If)
                    and "applied['manifestSHA256'] = trial82_activation.digest" in ast.get_source_segment(source, n))
        block = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        guard = self.guard()
        applied = {'manifestSHA256': 'stale'}
        namespace = {'args': SimpleNamespace(apply_trial82=True, apply_feedback84=True, apply_feedback85=False, maintenance_redeploy=False),
                     'trial82_guard': guard, 'trial82_activation': module, 'applied': applied}
        exec(block, namespace)
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['state'], 'recording-deployment')
        self.assertEqual(applied['manifestSHA256'], module.digest(guard.manifest))

    def test_maintenance_redeploy_flags_rejected_before_external_work(self):
        selection = ['--maintenance-redeploy']
        pinned = ['--expected-live-revision', self.live_revision]
        common = [selection, [*selection, '--expected-live-revision', 'a' * 41],
                  [*selection, '--expected-live-revision', 'A' * 40]]
        for flag in ('--apply-growth74', '--apply-growth75', '--apply-activity77', '--apply-feedback78',
                     '--apply-feedback79', '--apply-feedback80', '--apply-feedback81', '--apply-trial82',
                     '--apply-feedback84', '--apply-feedback85', '--migrate-role-wallets'):
            common.append([*selection, *pinned, flag])
        common += [[*selection, *pinned, '--warm-rush-history-player', '1'],
                   [*selection, *pinned, '--restore-activity-calendar', '00000000-0000-0000-0000-000000000001']]
        for script, extras in (('deploy.py', [['--provision'], ['--update-ingress'], ['--initial-dump', '/tmp/unused']]),
                               ('activate.py', [['--initial-db'], ['--provision-ingress']])):
            base = [sys.executable, '-B', str(HERE / script), '--with-backup'] + (['/not/a/real/stage'] if script == 'activate.py' else [])
            for options in common + [[*selection, *pinned, *extra] for extra in extras]:
                with self.subTest(script=script, options=options):
                    result = subprocess.run(base + options, capture_output=True, text=True)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn('AssertionError', result.stderr)
                    self.assertNotIn('FileNotFoundError', result.stderr)
        result = subprocess.run([sys.executable, str(HERE / 'deploy.py'), '--check-config', '--env-file',
                                 str(HERE / '.env.example'), '--with-backup', *selection, *pinned], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('no external action performed', result.stdout)

    def test_maintenance_redeploy_real_forwarding(self):
        source = (HERE / 'deploy.py').read_text()
        tree = ast.parse(source)
        start = next(i for i, n in enumerate(tree.body) if isinstance(n, ast.Assign)
                     and any(isinstance(t, ast.Name) and t.id == 'command' for t in n.targets))
        end = next(i for i in range(start, len(tree.body)) if isinstance(tree.body[i], ast.Expr)
                   and isinstance(tree.body[i].value, ast.Call) and isinstance(tree.body[i].value.func, ast.Name)
                   and tree.body[i].value.func.id == 'print')
        block = compile(ast.Module(body=tree.body[start:end], type_ignores=[]), str(HERE / 'deploy.py'), 'exec')
        args = SimpleNamespace(with_backup=True, warm_rush_history_player=[], expected_live_revision=self.live_revision,
                               apply_growth74=False, apply_growth75=False, apply_activity77=False,
                               apply_feedback78=False, apply_feedback79=False, apply_feedback80=False,
                               apply_feedback81=False, apply_trial82=False, apply_feedback84=False,
                               apply_feedback85=False, maintenance_redeploy=True, restore_activity_calendar=None,
                               migrate_role_wallets=False, provision=False, update_ingress=False, initial_dump=None)
        namespace = {'args': args, 'remote_stage': '/tmp/candidate', 'shlex': shlex}
        exec(block, namespace)
        argv = shlex.split(namespace['command'])
        self.assertEqual(argv.count('--with-backup'), 1)
        self.assertEqual(argv.count('--maintenance-redeploy'), 1)
        self.assertEqual(argv[argv.index('--expected-live-revision') + 1], self.live_revision)
        self.assertFalse(any(value.startswith('--apply-') for value in argv))
        self.assertNotIn('--provision-ingress', argv)

    def test_maintenance_redeploy_checks_actual_container_revision_before_build(self):
        source = (HERE / 'activate.py').read_text()
        node = next(n for n in ast.parse(source).body if isinstance(n, ast.If)
                    and 'Live container revision differs from deployment record' in ast.get_source_segment(source, n))
        self.assertLess(source.index(ast.get_source_segment(source, node)), source.index("run('docker', 'build'"))
        block = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        for revision in (self.live_revision, 'd' * 40, ''):
            with self.subTest(revision=revision):
                calls = []
                def inspect(*args):
                    calls.append(args)
                    return SimpleNamespace(stdout=revision.encode())
                namespace = {'args': SimpleNamespace(maintenance_redeploy=True, expected_live_revision=self.live_revision),
                             'run': inspect}
                if revision == self.live_revision:
                    exec(block, namespace)
                else:
                    with self.assertRaises(AssertionError):
                        exec(block, namespace)
                self.assertEqual(len(calls), 1)
                self.assertEqual(calls[0][0:2], ('docker', 'inspect'))

    def test_maintenance_redeploy_preflight_and_guard(self):
        for failure in ('missing-migration', 'maintenance-off', 'maintenance-after-stop', 'none', 'stop'):
            with self.subTest(failure=failure):
                root = self.root / ('redeploy-' + failure)
                root.mkdir(); (root / 'backups').mkdir()
                if failure != 'missing-migration':
                    (root / 'migration.json').write_text('{}')
                calls = []
                def compose(*args):
                    calls.append(args)
                    if failure == 'stop' and len(calls) == 1:
                        raise OSError('stop failed')
                maintenance_values = iter(['0'] if failure == 'maintenance-off' else
                                          ['1', '0'] if failure == 'maintenance-after-stop' else ['1', '1'])
                namespace, callbacks = self.cutover_namespace(root, trial=False, redeploy=True,
                    mysql=lambda _query: next(maintenance_values), compose=compose)
                with mock.patch.object(module.Trial82CutoverGuard, 'install_signal_handlers'):
                    if failure == 'none':
                        exec(cutover_block(), namespace)
                    else:
                        with self.assertRaises((AssertionError, OSError)):
                            exec(cutover_block(), namespace)
                if failure in ('missing-migration', 'maintenance-off'):
                    self.assertEqual((calls, callbacks), ([], []))
                    continue
                self.assertEqual(len(callbacks), 1)
                guard = namespace['trial82_guard']
                record = json.loads(guard.manifest.read_text())
                self.assertEqual(record['mode'], 'maintenance-redeploy')
                self.assertEqual(record['requestedCommands'], [])
                if failure == 'none':
                    guard.complete = True
                callbacks[0]()
                self.assertEqual(len(calls), 2 if failure in ('stop', 'maintenance-after-stop') else 1)
                if failure in ('stop', 'maintenance-after-stop'):
                    self.assertEqual(json.loads(guard.manifest.read_text())['state'], 'failed-maintenance')

    def test_maintenance_redeploy_backup_failure_preserves_guard(self):
        source = (HERE / 'activate.py').read_text()
        node = next(n for n in ast.parse(source).body if isinstance(n, ast.If)
                    and ast.get_source_segment(source, n.test) == "args.with_backup and (ROOT / 'migration.json').exists()"
                    and 'verified_backup = backup_existing_database()' in ast.get_source_segment(source, n))
        block = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        (self.root / 'migration.json').write_text('{}')
        guard = self.guard(mode='maintenance-redeploy')
        def fail_backup():
            raise OSError('private backup failure')
        args = SimpleNamespace(with_backup=True, apply_trial82=False, apply_feedback84=False, apply_feedback85=False, maintenance_redeploy=True)
        with self.assertRaises(OSError):
            exec(block, {'ROOT': self.root, 'args': args, 'trial82_guard': guard, 'backup_existing_database': fail_backup})
        guard.stop_if_unfinished()
        record = json.loads(guard.manifest.read_text())
        self.assertEqual((record['mode'], record['state'], record['failureStage']),
                         ('maintenance-redeploy', 'failed-maintenance', 'backing-up'))
        self.assertIsNone(record['backupBefore'])
        self.assertEqual(self.calls, [('stop', '-t', '30', 'app')])
        self.assertNotIn('private backup failure', json.dumps(record))

    def test_maintenance_redeploy_validates_backup_and_never_runs_publication(self):
        for variant in ('ok', 'missing', 'hash', 'revision', 'maintenance', 'mode'):
            with self.subTest(variant=variant):
                self.calls = []
                guard = self.guard(mode='configuration' if variant == 'mode' else 'maintenance-redeploy')
                backup = None if variant == 'missing' else dict(self.backup)
                if variant == 'hash':
                    backup['sha256'] = '0' * 64
                if variant == 'revision':
                    backup['revisionBefore'] = 'd' * 40
                self.mysql_values = iter(['0' if variant == 'maintenance' else '1'])
                if variant == 'ok':
                    result = module.activate_trial82(self.mysql, self.compose, guard, backup,
                        apply_trial82=False, apply_feedback84=False, apply_feedback85=False, maintenance_redeploy=True)
                    record = json.loads(guard.manifest.read_text())
                    self.assertEqual(record['mode'], 'maintenance-redeploy')
                    self.assertEqual(record['state'], 'verified-maintenance')
                    self.assertEqual(record['commands'], [])
                    self.assertEqual(result['appliedCommands'], [])
                    self.assertEqual(result['manifestSHA256'], module.digest(guard.manifest))
                    self.assertEqual(self.calls, [])
                    guard.complete = True
                else:
                    with self.assertRaises(AssertionError):
                        module.activate_trial82(self.mysql, self.compose, guard, backup,
                            apply_trial82=False, apply_feedback84=False, apply_feedback85=False, maintenance_redeploy=True)
                    guard.stop_if_unfinished()
                    self.assertEqual(self.calls, [('stop', '-t', '30', 'app')])
                    self.assertEqual(json.loads(guard.manifest.read_text())['state'], 'failed-maintenance')

    def test_maintenance_redeploy_validates_before_warm_and_records_result(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        node = next(n for n in tree.body if isinstance(n, ast.If)
                    and ast.get_source_segment(source, n.test) == 'args.maintenance_redeploy'
                    and 'trial82_activation.activate_trial82(' in ast.get_source_segment(source, n))
        self.assertLess(source.index(ast.get_source_segment(source, node)), source.index("'--warm-rush-history-all'"))
        block = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        guard = self.guard(mode='maintenance-redeploy')
        self.mysql_values = iter(['1'])
        manifest = {}
        exec(block, {'args': SimpleNamespace(maintenance_redeploy=True), 'trial82_activation': module,
                     'mysql': self.mysql, 'compose': self.compose, 'trial82_guard': guard,
                     'verified_backup': self.backup, 'manifest': manifest})
        self.assertEqual(manifest['maintenanceRedeployed']['appliedCommands'], [])
        self.assertEqual(manifest['maintenanceRedeployed']['state'], 'verified-maintenance')
        self.assertEqual(self.calls, [])

        record_node = next(n for n in tree.body if isinstance(n, ast.If)
                           and "applied['manifestSHA256'] = trial82_activation.digest" in ast.get_source_segment(source, n))
        record_block = compile(ast.Module(body=[record_node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        args = SimpleNamespace(apply_trial82=False, apply_feedback84=False, apply_feedback85=False, maintenance_redeploy=True)
        exec(record_block, {'args': args, 'trial82_guard': guard, 'trial82_activation': module,
                            'mysql': lambda _query: '1', 'applied': manifest['maintenanceRedeployed']})
        self.assertEqual(manifest['maintenanceRedeployed']['manifestSHA256'], module.digest(guard.manifest))
        self.assertEqual(json.loads(guard.manifest.read_text())['state'], 'recording-deployment')
        with self.assertRaises(AssertionError):
            exec(record_block, {'args': args, 'trial82_guard': guard, 'trial82_activation': module,
                                'mysql': lambda _query: '0', 'applied': manifest['maintenanceRedeployed']})
        guard.stop_if_unfinished()
        self.assertEqual(self.calls[-1], ('stop', '-t', '30', 'app'))

    def test_maintenance_redeploy_warm_postcheck_rejects_maintenance_drift(self):
        source = (HERE / 'activate.py').read_text()
        node = next(n for n in ast.parse(source).body if isinstance(n, ast.If)
                    and 'Maintenance changed after history verification' in ast.get_source_segment(source, n))
        block = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        guard = self.guard(mode='maintenance-redeploy')
        with self.assertRaises(AssertionError):
            exec(block, {'args': SimpleNamespace(maintenance_redeploy=True), 'trial82_guard': guard, 'mysql': lambda _query: '0'})
        guard.stop_if_unfinished()
        record = json.loads(guard.manifest.read_text())
        self.assertEqual(record['failureStage'], 'checking-maintenance-after-warm')
        self.assertEqual(self.calls[-1], ('stop', '-t', '30', 'app'))

    def test_maintenance_redeploy_health_and_postcheck_guard(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        start = next(i for i, n in enumerate(tree.body) if isinstance(n, ast.If)
                     and "trial82_guard.record['state'] = 'starting-application'" in ast.get_source_segment(source, n))
        end = next(i for i in range(start, len(tree.body)) if isinstance(tree.body[i], ast.If)
                   and 'Maintenance changed after health check' in ast.get_source_segment(source, tree.body[i]))
        block = compile(ast.Module(body=tree.body[start:end + 1], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        for failure in ('none', 'startup', 'health', 'maintenance'):
            with self.subTest(failure=failure):
                calls = []
                def compose(*args):
                    calls.append(args)
                    if failure == 'startup' and args[0] == 'up':
                        raise RuntimeError('startup failed')
                def urlopen(*_args, **_kwargs):
                    if failure == 'health':
                        raise OSError('health failed')
                    return io.BytesIO(b'{"status":"ok"}')
                guard = module.Trial82CutoverGuard(compose, self.root, self.revision, self.binary_sha,
                                                   self.live_revision, mode='maintenance-redeploy')
                guard.prepare()
                args = SimpleNamespace(apply_trial82=False, apply_feedback84=False, apply_feedback85=False,
                    maintenance_redeploy=True, migrate_role_wallets=False, apply_activity77=False,
                    apply_feedback78=False, apply_feedback79=False, apply_feedback80=False, apply_feedback81=False)
                namespace = {'args': args, 'trial82_guard': guard, 'compose': compose,
                             'urllib': SimpleNamespace(request=SimpleNamespace(urlopen=urlopen)),
                             'runtime': {'HYJS_UPSTREAM_PORT': '12345'}, 'json': json,
                             'time': SimpleNamespace(sleep=lambda _: None),
                             'mysql': lambda _query: '0' if failure == 'maintenance' else '1'}
                if failure == 'none':
                    exec(block, namespace)
                    self.assertEqual(guard.record['state'], 'checking-maintenance-after-health')
                    guard.complete = True
                else:
                    with self.assertRaises((RuntimeError, SystemExit, AssertionError)):
                        exec(block, namespace)
                    guard.stop_if_unfinished()
                    self.assertEqual(calls[-1], ('stop', '-t', '30', 'app'))
                    self.assertEqual(json.loads(guard.manifest.read_text())['state'], 'failed-maintenance')

    def test_maintenance_redeploy_writes_success_record_in_private_fixture(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        start = next(i for i, n in enumerate(tree.body) if isinstance(n, ast.If)
                     and "trial82_guard.record['state'] = 'starting-application'" in ast.get_source_segment(source, n))
        health_end = next(i for i in range(start, len(tree.body)) if isinstance(tree.body[i], ast.If)
                          and 'Maintenance changed after health check' in ast.get_source_segment(source, tree.body[i]))
        install = next(i for i, n in enumerate(tree.body) if isinstance(n, ast.If)
                       and "trial82_guard.record['state'] = 'installing-artifacts'" in ast.get_source_segment(source, n))
        record = next(i for i in range(install, len(tree.body)) if isinstance(tree.body[i], ast.Expr)
                      and "(ROOT / 'deployed.json').write_text" in ast.get_source_segment(source, tree.body[i]))
        complete = next(n for n in tree.body if isinstance(n, ast.If)
                        and 'trial82_guard.complete = True' in ast.get_source_segment(source, n))
        block = compile(ast.Module(body=tree.body[start:health_end + 1] + tree.body[install:record + 1] + [complete],
                                   type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        root = self.root / 'release'
        stage = root / '.incoming' / 'candidate'
        (stage / 'app').mkdir(parents=True)
        (stage / 'app' / 'binary').write_bytes(b'new candidate')
        (root / 'app').mkdir()
        (root / 'app' / 'binary').write_bytes(b'old candidate')
        calls = []
        guard = module.Trial82CutoverGuard(lambda *args: calls.append(args), self.root, self.revision,
                                           self.binary_sha, self.live_revision, mode='maintenance-redeploy')
        guard.prepare()
        self.mysql_values = iter(['1'])
        applied = module.activate_trial82(self.mysql, guard.compose, guard, self.backup,
            apply_trial82=False, apply_feedback84=False, apply_feedback85=False, maintenance_redeploy=True)
        manifest = {'revision': self.revision, 'files': {'app/hyjs-server': self.binary_sha},
                    'maintenanceRedeployed': applied}
        args = SimpleNamespace(apply_trial82=False, apply_feedback84=False, apply_feedback85=False,
            maintenance_redeploy=True, migrate_role_wallets=False, apply_activity77=False,
            apply_feedback78=False, apply_feedback79=False, apply_feedback80=False, apply_feedback81=False)
        namespace = {'ROOT': root, 'stage': stage, 'args': args, 'manifest': manifest, 'applied': applied,
                     'trial82_guard': guard, 'trial82_activation': module, 'compose': guard.compose,
                     'urllib': SimpleNamespace(request=SimpleNamespace(urlopen=lambda *_args, **_kwargs: io.BytesIO(b'{"status":"ok"}'))),
                     'runtime': {'HYJS_UPSTREAM_PORT': '12345'}, 'json': json, 'os': os,
                     'time': SimpleNamespace(sleep=lambda _: None, gmtime=lambda: None, strftime=lambda *_args: 'fixture'),
                     'mysql': lambda _query: '1', 'shutil': __import__('shutil'),
                     'run': lambda *_args: SimpleNamespace(stdout=self.revision.encode())}
        exec(block, namespace)
        saved = json.loads((root / 'deployed.json').read_text())
        self.assertEqual(saved['revision'], self.revision)
        self.assertEqual(saved['maintenanceRedeployed']['appliedCommands'], [])
        self.assertEqual(saved['maintenanceRedeployed']['manifestSHA256'], module.digest(guard.manifest))
        self.assertEqual(json.loads(guard.manifest.read_text())['mode'], 'maintenance-redeploy')
        self.assertEqual((root / 'app' / 'binary').read_bytes(), b'new candidate')
        self.assertTrue(guard.complete)
        self.assertEqual(calls, [('up', '-d', '--no-deps', '--force-recreate', 'app')])
        guard.stop_if_unfinished()
        self.assertEqual(len(calls), 1)


if __name__ == '__main__':
    unittest.main()
