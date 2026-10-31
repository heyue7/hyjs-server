import ast
import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import calendar_restore_activation as module

REQUEST = '8e53bca0-6ccb-4c16-bb18-416a2925c4af'
LIVE = 'a' * 40
REVISION = 'b' * 40
DIGEST = 'c' * 64


class Formal110ArgumentsTests(unittest.TestCase):
    def test_explicit_standalone_request_and_revision_are_required(self):
        valid = {'calendar_formal_110': True, 'restore_activity_calendar': REQUEST,
                 'expected_live_revision': LIVE}
        for remote in (False, True):
            module.validate_formal110_arguments(SimpleNamespace(**valid), remote=remote)
            for key, value in [('restore_activity_calendar', None), ('restore_activity_calendar', 'bad'),
                               ('expected_live_revision', 'a' * 39), ('expected_live_revision', 'A' * 40),
                               ('expected_live_revision', 'a' * 41), ('migrate_role_wallets', True),
                               ('maintenance_redeploy', True), ('apply_growth75', True),
                               ('warm_rush_history_player', [1]),
                               ('initial_db' if remote else 'initial_dump', True),
                               ('provision_ingress' if remote else 'update_ingress', True)]:
                with self.subTest(remote=remote, key=key, value=value):
                    with self.assertRaises(ValueError):
                        module.validate_formal110_arguments(SimpleNamespace(**dict(valid, **{key: value})), remote=remote)
        # Existing non-formal releases retain their current validation path.
        module.validate_formal110_arguments(SimpleNamespace(calendar_formal_110=False))

    def test_both_command_line_entrypoints_reject_before_network_or_database_access(self):
        for script, prefix in [('deploy.py', []), ('activate.py', ['/nonexistent-stage'])]:
            for extra in ([], ['--restore-activity-calendar', REQUEST, '--expected-live-revision', 'a' * 39],
                          ['--restore-activity-calendar', REQUEST, '--expected-live-revision', LIVE,
                           '--initial-db' if script == 'activate.py' else '--provision']):
                with self.subTest(script=script, extra=extra):
                    result = subprocess.run([sys.executable, '-B', str(HERE / script), *prefix,
                                             '--calendar-formal-110', *extra], capture_output=True, text=True)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertNotIn('SSH', result.stderr)
                    self.assertNotIn('MySQL', result.stderr)

    def test_actual_deployment_command_forwards_the_explicit_flag(self):
        source = (HERE / 'deploy.py').read_text()
        tree = ast.parse(source)
        node = next(n for n in tree.body if isinstance(n, ast.If) and
                    "command += ' --calendar-formal-110'" in ast.get_source_segment(source, n))
        code = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'deploy.py'), 'exec')
        for enabled in (False, True):
            namespace = {'args': SimpleNamespace(calendar_formal_110=enabled), 'command': 'activate'}
            exec(code, namespace)
            self.assertEqual(namespace['command'], 'activate' + (' --calendar-formal-110' if enabled else ''))

    def test_final_activation_and_deployment_verify_formal_identity_and_digest(self):
        source = (HERE / 'deploy.py').read_text()
        node = next(n for n in ast.parse(source).body if isinstance(n, ast.If) and
                    'Formal 110 deployed calendar identity or digest differs' in ast.get_source_segment(source, n))
        code = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'deploy.py'), 'exec')
        valid = {'formal110': True, 'requestId': REQUEST, 'openingAt': module.FORMAL110_OPENING_MILLIS, 'digest': DIGEST}
        namespace = {'args': SimpleNamespace(calendar_formal_110=True, restore_activity_calendar=REQUEST),
                     'deployed': {'calendarRestore': valid}, 'calendar_restore_activation': module,
                     're': __import__('re')}
        exec(code, namespace)
        for key, value in [('formal110', False), ('requestId', 'different'), ('openingAt', 1), ('digest', 'bad')]:
            namespace['deployed'] = {'calendarRestore': dict(valid, **{key: value})}
            with self.assertRaises(ValueError):
                exec(code, namespace)
        namespace['args'].calendar_formal_110 = False
        exec(code, namespace)
        source = (HERE / 'activate.py').read_text()
        node = next(n for n in ast.parse(source).body if isinstance(n, ast.If) and
                    'deployed_status.update(formal110=True' in ast.get_source_segment(source, n))
        code = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        namespace = {'args': SimpleNamespace(calendar_formal_110=True), 'deployed_status': {},
                     'manifest': {'calendarRestore': valid}}
        exec(code, namespace)
        self.assertEqual(namespace['deployed_status'], {'formal110': True, 'calendarDigest': DIGEST})

    def test_vip10_predicate_is_readonly_and_bound_to_reviewed_go_constants(self):
        module.validate_formal110_source_constants(HERE.parent / 'hyjs-server/constUtil/payment.go')
        self.assertTrue(module.VIP10_CHECK_SQL.startswith('SELECT COUNT(*)'))
        for fragment in ('enabled=1', 'min_v_ip=10', 'cost=2888', 'total_limit=0',
                         "BINARY cost_resource=BINARY 'diamond'", "BINARY reward=BINARY 'item_044,1'",
                         'BINARY source=BINARY', module.VIP10_SOURCE):
            self.assertIn(fragment, module.VIP10_CHECK_SQL)
        with tempfile.TemporaryDirectory() as tmp:
            source = Path(tmp) / 'payment.go'
            source.write_text('const P3VIP10GrowthProduct = "different"\n')
            with self.assertRaises(ValueError):
                module.validate_formal110_source_constants(source)


class Formal110ActivationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.backup_dir = self.root / 'backups'
        self.backup_dir.mkdir(mode=0o700)
        backup = self.backup_dir / 'pre-deploy.aesgcm'
        backup.write_bytes(b'fixture authenticated snapshot')
        self.backup = {'file': backup.name, 'sha256': hashlib.sha256(backup.read_bytes()).hexdigest(),
                       'format': 'HYJSBK02-AES256GCM-CHUNKED', 'revisionBefore': LIVE}
        self.calls = []
        self.queries = []
        self.maintenance = '1'
        self.vip = '1'
        self.offset = '0'
        self.preview = {'formal110': True, 'serverId': 1, 'digest': DIGEST,
                        'windows': [{'occurrenceId': 'example'}], 'openingAt': module.FORMAL110_OPENING_MILLIS,
                        'blockers': []}
        self.result = {'formal110': True, 'requestId': REQUEST, 'digest': DIGEST,
                       'openingAt': module.FORMAL110_OPENING_MILLIS, 'windows': 1,
                       'clock': {'serverId': 1, 'offsetDays': 0}}
        self.failure = None

    def mysql(self, sql):
        self.queries.append(sql)
        if sql == module.MAINTENANCE_SQL:
            return self.maintenance
        if sql == module.VIP10_CHECK_SQL:
            return self.vip
        if sql == 'SELECT offset_days FROM server_game_clocks WHERE server_id=1;':
            return self.offset
        self.fail('Unexpected or mutating SQL')

    def compose(self, *args):
        self.calls.append(args)
        if args[0] == 'stop':
            return SimpleNamespace(stdout=b'')
        if '--preview-activity-calendar' in args:
            value = self.preview
        elif '--restore-activity-calendar' in args:
            if self.failure == 'lost-response':
                raise RuntimeError('simulated lost response after possible commit')
            value = self.result
        else:
            self.fail('Unexpected compose command: ' + repr(args))
        return SimpleNamespace(stdout=('diagnostic\n' + json.dumps(value) + '\n').encode())

    def docker_run(self, *args):
        self.assertEqual(args, ('docker', 'inspect', '--format={{.State.Running}}', 'hyjs-game'))
        return SimpleNamespace(stdout=b'false\n')

    def guard(self):
        value = module.Formal110CutoverGuard(self.compose, self.mysql, self.docker_run, self.backup_dir,
                                              REVISION, LIVE, REQUEST)
        value.prepare()
        return value

    def invoke(self, guard, backup=None):
        return module.activate_calendar_restore(self.mysql, self.compose, self.backup_dir, REVISION,
                                                 self.backup if backup is None else backup, REQUEST,
                                                 formal110=True, expected_live_revision=LIVE, guard=guard)

    def test_formal_cutover_checks_backup_and_product_without_writing_it(self):
        guard = self.guard()
        output = self.invoke(guard)
        self.assertTrue(output['formal110'])
        self.assertEqual(output['digest'], DIGEST)
        commands = [args for args in self.calls if args[0] == 'run']
        self.assertEqual(len(commands), 2)
        self.assertTrue(all('--calendar-formal-110' in args for args in commands))
        self.assertIn('--preview-activity-calendar', commands[0])
        self.assertIn('--restore-activity-calendar', commands[1])
        self.assertFalse(any('--restore-vip10-book' in args for args in self.calls))
        self.assertTrue(all(q.startswith('SELECT ') for q in self.queries))
        self.assertGreaterEqual(self.queries.count(module.VIP10_CHECK_SQL), 3)
        self.assertTrue(guard.record['applyMayHaveCommitted'])
        self.assertTrue(guard.record['applyConfirmed'])
        evidence = self.backup_dir / ('calendar-' + REQUEST) / 'manifest.json'
        self.assertEqual(json.loads(evidence.read_text())['status'], 'applied')
        self.assertEqual(evidence.stat().st_mode & 0o777, 0o600)
        self.assertEqual(guard.manifest.stat().st_mode & 0o777, 0o600)
        guard.complete = True
        guard.stop_if_unfinished()
        self.assertFalse(any(args[0] == 'stop' for args in self.calls))

    def test_incorrect_backup_product_or_maintenance_stops_before_apply(self):
        for variant in ('backup-format', 'backup-hash', 'backup-revision', 'backup-path', 'vip', 'maintenance'):
            with self.subTest(variant=variant):
                self.calls, self.queries, self.vip, self.maintenance = [], [], '1', '1'
                backup = dict(self.backup)
                if variant == 'backup-format': backup['format'] = 'other'
                if variant == 'backup-hash': backup['sha256'] = '0' * 64
                if variant == 'backup-revision': backup['revisionBefore'] = 'd' * 40
                if variant == 'backup-path': backup['file'] = '../outside'
                if variant == 'vip': self.vip = '0'
                if variant == 'maintenance': self.maintenance = '0'
                guard = self.guard()
                with self.assertRaises(ValueError):
                    self.invoke(guard, backup)
                guard.stop_if_unfinished()
                record = json.loads(guard.manifest.read_text())
                self.assertFalse(record['applyMayHaveCommitted'])
                self.assertTrue(record['stopConfirmed'])
                self.assertEqual(record['maintenanceObserved'], self.maintenance)
                self.assertEqual(self.calls, [('stop', '-t', '30', 'app')])

    def test_unprivate_retry_evidence_is_rejected_before_preview(self):
        calendar_dir = self.backup_dir / ('calendar-' + REQUEST)
        calendar_dir.mkdir(mode=0o755)
        calendar_dir.chmod(0o755)
        guard = self.guard()
        with self.assertRaises(ValueError):
            self.invoke(guard)
        guard.stop_if_unfinished()
        self.assertFalse(any('--preview-activity-calendar' in args for args in self.calls))
        self.assertTrue(guard.record['stopConfirmed'])

    def test_checkpoint_failure_prevents_calendar_apply(self):
        guard = self.guard()
        actual_write = module.private_write
        def failing_write(path, value):
            if value.get('status') == 'applying':
                raise OSError('simulated evidence sync failure')
            return actual_write(path, value)
        with mock.patch.object(module, 'private_write', side_effect=failing_write):
            with self.assertRaises(OSError):
                self.invoke(guard)
        guard.stop_if_unfinished()
        self.assertFalse(any('--restore-activity-calendar' in args for args in self.calls))
        self.assertTrue(guard.record['stopConfirmed'])

    def test_unqualified_preview_never_applies(self):
        for key, value in [('formal110', False), ('serverId', 2), ('digest', 'not-a-digest'),
                           ('openingAt', 1), ('windows', []), ('blockers', ['fixture blocker'])]:
            with self.subTest(key=key):
                self.calls = []
                self.preview = dict(self.preview, **{key: value})
                # Use an isolated evidence directory for each request fixture.
                calendar_dir = self.backup_dir / ('calendar-' + REQUEST)
                if calendar_dir.exists():
                    import shutil
                    shutil.rmtree(calendar_dir)
                guard = self.guard()
                with self.assertRaises(ValueError):
                    self.invoke(guard)
                guard.stop_if_unfinished()
                self.assertFalse(guard.record['applyMayHaveCommitted'])
                self.assertFalse(any('--restore-activity-calendar' in args for args in self.calls))
                self.preview = {'formal110': True, 'serverId': 1, 'digest': DIGEST,
                                'windows': [{'occurrenceId': 'example'}],
                                'openingAt': module.FORMAL110_OPENING_MILLIS, 'blockers': []}

    def test_lost_apply_response_keeps_uncertain_commit_and_retry_reuses_preview(self):
        self.failure = 'lost-response'
        first = self.guard()
        with self.assertRaises(RuntimeError):
            self.invoke(first)
        first.stop_if_unfinished()
        self.assertTrue(first.record['applyMayHaveCommitted'])
        self.assertFalse(first.record['applyConfirmed'])
        self.assertEqual(first.record['failureStage'], 'applying-calendar')
        record = self.backup_dir / ('calendar-' + REQUEST) / 'manifest.json'
        self.assertEqual(json.loads(record.read_text())['status'], 'applying')
        self.failure = None
        second = self.guard()
        result = self.invoke(second)
        self.assertTrue(result['formal110'])
        self.assertEqual(sum('--preview-activity-calendar' in args for args in self.calls), 1)
        self.assertEqual(sum('--restore-activity-calendar' in args for args in self.calls), 2)
        second.complete = True

    def test_result_mismatch_and_postcommit_failure_keep_app_stopped(self):
        for variant in ('formal-flag', 'digest', 'offset'):
            with self.subTest(variant=variant):
                self.calls, self.queries, self.offset = [], [], '0'
                calendar_dir = self.backup_dir / ('calendar-' + REQUEST)
                if calendar_dir.exists():
                    import shutil
                    shutil.rmtree(calendar_dir)
                self.result = {'formal110': True, 'requestId': REQUEST, 'digest': DIGEST,
                               'openingAt': module.FORMAL110_OPENING_MILLIS, 'windows': 1,
                               'clock': {'serverId': 1, 'offsetDays': 0}}
                if variant == 'formal-flag': self.result['formal110'] = False
                if variant == 'digest': self.result['digest'] = 'd' * 64
                if variant == 'offset': self.offset = '1'
                guard = self.guard()
                with self.assertRaises(ValueError):
                    self.invoke(guard)
                guard.stop_if_unfinished()
                saved = json.loads(guard.manifest.read_text())
                self.assertTrue(saved['applyMayHaveCommitted'])
                self.assertEqual(saved['applyConfirmed'], variant == 'offset')
                self.assertTrue(saved['stopConfirmed'])
                self.assertEqual(saved['maintenanceObserved'], '1')
                self.assertTrue((calendar_dir / 'result.json').is_file())

    def test_tampered_retry_preview_is_rejected(self):
        self.failure = 'lost-response'
        first = self.guard()
        with self.assertRaises(RuntimeError):
            self.invoke(first)
        first.stop_if_unfinished()
        preview = self.backup_dir / ('calendar-' + REQUEST) / 'preview.json'
        preview.write_text(preview.read_text() + ' ')
        self.calls = []
        second = self.guard()
        with self.assertRaises(ValueError):
            self.invoke(second)
        second.stop_if_unfinished()
        self.assertFalse(any('--restore-activity-calendar' in args for args in self.calls))

    def test_retry_keeps_possible_prior_commit_even_if_new_preflight_fails(self):
        self.failure = 'lost-response'
        first = self.guard()
        with self.assertRaises(RuntimeError):
            self.invoke(first)
        first.stop_if_unfinished()
        self.vip = '0'
        second = self.guard()
        self.assertTrue(second.record['applyMayHaveCommitted'])
        self.assertEqual(second.record['priorAttempt'], 'applying')
        with self.assertRaises(ValueError):
            self.invoke(second)
        second.stop_if_unfinished()
        saved = json.loads(second.manifest.read_text())
        self.assertTrue(saved['applyMayHaveCommitted'])
        self.assertFalse(saved['applyConfirmed'])
        self.assertTrue(saved['stopConfirmed'])

    def test_guard_reports_unconfirmed_stop_without_changing_maintenance(self):
        guard = self.guard()
        guard.run = lambda *_args: SimpleNamespace(stdout=b'true\n')
        guard.record['applyMayHaveCommitted'] = True
        guard.checkpoint('checking-health')
        guard.stop_if_unfinished()
        self.assertFalse(guard.record['stopConfirmed'])
        self.assertEqual(guard.record['maintenanceObserved'], '1')
        self.assertEqual(guard.record['failureStage'], 'checking-health')
        self.assertTrue(guard.record['applyMayHaveCommitted'])

    def test_running_application_blocks_calendar_before_preview(self):
        guard = self.guard()
        guard.run = lambda *_args: SimpleNamespace(stdout=b'true\n')
        with self.assertRaises(ValueError):
            self.invoke(guard)
        self.assertFalse(any('--preview-activity-calendar' in args for args in self.calls))
        guard.stop_if_unfinished()
        self.assertFalse(guard.record['stopConfirmed'])
        self.assertFalse(guard.record['applyMayHaveCommitted'])


class Formal110IntegrationBlocksTests(unittest.TestCase):
    def test_formal_cutover_rejects_runtime_and_compose_drift_without_writing_them(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        node = next(n for n in tree.body if isinstance(n, ast.If) and
                    "formal110_guard.checkpoint('checking-existing-runtime')" in ast.get_source_segment(source, n))
        code = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        for change in ('none', 'runtime', 'duplicate', 'compose'):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp) / 'root'
                stage = Path(tmp) / 'stage'
                root.mkdir(); stage.mkdir()
                private = root / '.env'
                original = 'HYJS_PUBLIC_HOST=expected\nHYJS_HTTP_PORT=28861\n'
                private.write_text(original if change != 'duplicate' else original + 'HYJS_HTTP_PORT=28861\n')
                (root / 'compose.yaml').write_text('reviewed')
                (stage / 'compose.yaml').write_text('different' if change == 'compose' else 'reviewed')
                runtime = {'HYJS_PUBLIC_HOST': 'expected', 'HYJS_HTTP_PORT': '28861'}
                if change == 'runtime': runtime['HYJS_PUBLIC_HOST'] = 'changed'
                namespace = {'args': SimpleNamespace(calendar_formal_110=True),
                             'formal110_guard': SimpleNamespace(checkpoint=lambda _state: None),
                             'private_env': private, 'runtime': runtime, 'ROOT': root, 'stage': stage}
                before = private.read_bytes()
                if change == 'none':
                    exec(code, namespace)
                else:
                    with self.assertRaises(AssertionError):
                        exec(code, namespace)
                self.assertEqual(private.read_bytes(), before)

    def test_actual_stop_branch_preserves_default_and_never_toggles_formal_maintenance(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        node = next(n for n in tree.body if isinstance(n, ast.If) and
                    "formal110_guard.checkpoint('stopping-application')" in ast.get_source_segment(source, n))
        code = compile(ast.Module(body=[node], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        for formal in (False, True):
            calls = []
            guard = SimpleNamespace(checkpoint=lambda name: calls.append(('checkpoint', name)),
                                    require_stopped=lambda: calls.append(('verified-stopped',)))
            namespace = {'args': SimpleNamespace(restore_activity_calendar=REQUEST, calendar_formal_110=formal),
                         'compose': lambda *args: calls.append(args), 'mysql': lambda _q: '1',
                         'formal110_guard': guard, 'calendar_restore_activation': module}
            exec(code, namespace)
            if formal:
                self.assertEqual(calls, [('checkpoint', 'stopping-application'), ('stop', '-t', '30', 'app'),
                                         ('verified-stopped',)])
            else:
                self.assertIn('--calendar-maintenance', calls[0])
                self.assertEqual(calls[-1], ('stop', '-t', '30', 'app'))

    def test_actual_start_health_and_maintenance_blocks_trigger_stop_guard_on_failure(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        start = next(i for i, n in enumerate(tree.body) if isinstance(n, ast.If) and
                     "formal110_guard.checkpoint('starting-application')" in ast.get_source_segment(source, n))
        end = next(i for i in range(start, len(tree.body)) if isinstance(tree.body[i], ast.If) and
                   "formal110_guard.checkpoint('checking-maintenance-and-vip10-after-health')" in ast.get_source_segment(source, tree.body[i]))
        code = compile(ast.Module(body=tree.body[start:end + 1], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        names = ('migrate_role_wallets', 'apply_activity77', 'apply_feedback78', 'apply_feedback79',
                 'apply_feedback80', 'apply_feedback81', 'apply_trial82', 'apply_feedback84',
                 'apply_feedback85', 'maintenance_redeploy')
        for failure in ('none', 'startup', 'health', 'maintenance', 'vip'):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as tmp:
                calls = []
                def compose(*args):
                    calls.append(args)
                    if failure == 'startup' and args[0] == 'up':
                        raise RuntimeError('startup failure')
                    return SimpleNamespace(stdout=b'')
                def mysql(sql):
                    if sql == module.MAINTENANCE_SQL: return '0' if failure == 'maintenance' else '1'
                    if sql == module.VIP10_CHECK_SQL: return '0' if failure == 'vip' else '1'
                    raise AssertionError('Unexpected SQL')
                def urlopen(*_args, **_kwargs):
                    if failure == 'health': raise OSError('health failure')
                    return io.BytesIO(b'{"status":"ok"}')
                guard = module.Formal110CutoverGuard(compose, mysql, lambda *_a: SimpleNamespace(stdout=b'false'),
                                                     Path(tmp), REVISION, LIVE, REQUEST)
                guard.prepare()
                guard.record['applyMayHaveCommitted'] = True
                guard.record['applyConfirmed'] = True
                values = {name: False for name in names}
                values['calendar_formal_110'] = True
                namespace = {'args': SimpleNamespace(**values), 'formal110_guard': guard, 'compose': compose,
                             'calendar_restore_activation': module, 'mysql': mysql,
                             'urllib': SimpleNamespace(request=SimpleNamespace(urlopen=urlopen)),
                             'runtime': {'HYJS_UPSTREAM_PORT': '12345'}, 'json': json,
                             'time': SimpleNamespace(sleep=lambda _: None)}
                if failure == 'none':
                    exec(code, namespace)
                    self.assertEqual(guard.record['state'], 'checking-maintenance-and-vip10-after-health')
                    guard.complete = True
                else:
                    with self.assertRaises((RuntimeError, SystemExit, ValueError)):
                        exec(code, namespace)
                    guard.stop_if_unfinished()
                    self.assertEqual(calls[-1], ('stop', '-t', '30', 'app'))
                    self.assertTrue(guard.record['stopConfirmed'])
                    self.assertTrue(guard.record['applyMayHaveCommitted'])
                    self.assertTrue(guard.record['applyConfirmed'])


if __name__ == '__main__':
    unittest.main()
