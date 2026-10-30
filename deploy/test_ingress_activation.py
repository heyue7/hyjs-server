import ast
import hashlib
import json
import os
from pathlib import Path
import stat
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import ingress_activation as module
import trial82_activation


class IngressActivationTests(unittest.TestCase):
    def setUp(self):
        self.folder = tempfile.TemporaryDirectory(prefix='hyjs-ingress-test-')
        self.addCleanup(self.folder.cleanup)
        self.root = Path(self.folder.name)
        self.available = self.root / 'available'
        self.enabled = self.root / 'enabled'
        self.stage = self.root / 'stage'
        for path in (self.available, self.enabled, self.stage):
            path.mkdir()
        self.destination = self.available / 'hyjs-test.conf'
        self.destination.write_bytes(b'old configuration\n')
        self.destination.chmod(0o640)
        self.link = self.enabled / 'hyjs-test.conf'
        self.link.symlink_to('../available/hyjs-test.conf')
        self.original = self.destination.stat()
        self.desired = b'new configuration\n'
        self.commands = []
        try:
            os.setxattr(self.destination, 'user.hyjs-test', b'preserve')
            self.has_xattr = True
        except (AttributeError, OSError):
            self.has_xattr = False

    def run_command(self, *args):
        self.commands.append(args)

    def update(self, run=None):
        module.update_existing(self.destination, self.link, self.desired, self.stage, run or self.run_command)

    def check_properties(self, expected):
        self.assertEqual(self.destination.read_bytes(), expected)
        actual = self.destination.stat()
        self.assertEqual(stat.S_IMODE(actual.st_mode), stat.S_IMODE(self.original.st_mode))
        self.assertEqual((actual.st_uid, actual.st_gid), (self.original.st_uid, self.original.st_gid))
        if self.has_xattr:
            self.assertEqual(os.getxattr(self.destination, 'user.hyjs-test'), b'preserve')
        self.assertEqual(os.readlink(self.link), '../available/hyjs-test.conf')

    def recovery(self):
        folders = list(self.stage.glob('.ingress-recovery-*'))
        self.assertEqual(len(folders), 1)
        folder = folders[0]
        self.assertEqual(stat.S_IMODE(folder.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE((folder / 'original.conf').stat().st_mode), 0o600)
        self.assertEqual((folder / 'original.conf').read_bytes(), b'old configuration\n')
        metadata = json.loads((folder / 'metadata.json').read_text())
        self.assertEqual((metadata['mode'], metadata['uid'], metadata['gid']),
                         (0o640, self.original.st_uid, self.original.st_gid))
        return folder

    def test_preflight_uses_isolated_http_tree_and_cleans_it(self):
        candidate = self.stage / 'nginx.conf'
        candidate.write_bytes(self.desired)
        paths = []
        def inspect(*args):
            self.assertEqual(args[:3], ('nginx', '-t', '-c'))
            config = Path(args[3])
            paths.append(config)
            text = config.read_text()
            self.assertIn('events {}', text)
            self.assertIn('http { access_log off; include ', text)
            isolated = config.parent / 'site.conf'
            self.assertIn(str(isolated), text)
            self.assertEqual(isolated.read_bytes(), self.desired)
        data = module.preflight(candidate, hashlib.sha256(self.desired).hexdigest(), inspect)
        self.assertEqual(data, self.desired)
        self.assertFalse(paths[0].exists())

    def test_preflight_refuses_changed_artifact_before_invoking_nginx(self):
        candidate = self.stage / 'nginx.conf'
        candidate.write_bytes(self.desired)
        with self.assertRaisesRegex(module.IngressError, 'verified artifact'):
            module.preflight(candidate, '0' * 64, self.run_command)
        self.assertEqual(self.commands, [])

    def test_success_replaces_atomically_and_preserves_attributes(self):
        self.update()
        self.check_properties(self.desired)
        self.assertEqual(self.commands, [('nginx', '-t'), ('systemctl', 'reload', 'nginx')])
        self.assertEqual(list(self.stage.iterdir()), [])
        self.assertEqual(list(self.available.glob('.hyjs-test.conf.hyjs-*')), [])

    def test_unchanged_configuration_does_not_reload(self):
        self.desired = self.destination.read_bytes()
        identity = self.destination.stat().st_ino
        self.update()
        self.assertEqual(self.destination.stat().st_ino, identity)
        self.assertEqual(self.commands, [('nginx', '-t')])

    def test_nginx_test_failure_restores_and_checks_original(self):
        def run(*args):
            self.commands.append(args)
            if len(self.commands) == 1:
                raise RuntimeError('candidate syntax rejected')
        with self.assertRaisesRegex(module.IngressError, 'original restored and checked'):
            self.update(run)
        self.check_properties(b'old configuration\n')
        self.assertEqual(self.commands, [('nginx', '-t'), ('nginx', '-t')])
        self.recovery()

    def test_failed_reload_restores_checks_and_reloads_original(self):
        def run(*args):
            self.commands.append(args)
            if self.commands == [('nginx', '-t'), ('systemctl', 'reload', 'nginx')]:
                raise RuntimeError('reload failed')
        with self.assertRaisesRegex(module.IngressError, 'original restored and checked'):
            self.update(run)
        self.check_properties(b'old configuration\n')
        self.assertEqual(self.commands, [('nginx', '-t'), ('systemctl', 'reload', 'nginx'),
                                         ('nginx', '-t'), ('systemctl', 'reload', 'nginx')])
        self.recovery()

    def test_external_rewrite_during_failure_is_not_overwritten(self):
        def run(*args):
            self.commands.append(args)
            self.destination.write_bytes(b'external edit\n')
            raise RuntimeError('test failed after external edit')
        with self.assertRaisesRegex(module.IngressError, 'recovery was unsafe or failed'):
            self.update(run)
        self.assertEqual(self.destination.read_bytes(), b'external edit\n')
        self.assertEqual(self.commands, [('nginx', '-t')])
        self.recovery()

    def test_external_replacement_with_same_bytes_is_not_overwritten(self):
        def run(*args):
            self.commands.append(args)
            other = self.available / 'external.tmp'
            other.write_bytes(self.desired)
            os.replace(other, self.destination)
            raise RuntimeError('concurrent same-content replacement')
        with self.assertRaisesRegex(module.IngressError, 'recovery was unsafe or failed'):
            self.update(run)
        self.assertEqual(self.destination.read_bytes(), self.desired)
        self.recovery()

    def test_external_link_change_is_not_overwritten(self):
        alternate = self.available / 'external.conf'
        alternate.write_bytes(b'external\n')
        def run(*args):
            self.commands.append(args)
            self.link.unlink()
            self.link.symlink_to('../available/external.conf')
            raise RuntimeError('link changed')
        with self.assertRaisesRegex(module.IngressError, 'recovery was unsafe or failed'):
            self.update(run)
        self.assertEqual(os.readlink(self.link), '../available/external.conf')
        self.assertEqual(alternate.read_bytes(), b'external\n')
        self.recovery()

    def test_restore_write_failure_keeps_candidate_and_private_original(self):
        replace = module._atomic_replace
        calls = []
        def flaky(*args):
            calls.append(args)
            if len(calls) == 2:
                raise OSError('restore write failed')
            return replace(*args)
        def run(*args):
            self.commands.append(args)
            raise RuntimeError('candidate test failed')
        with mock.patch.object(module, '_atomic_replace', side_effect=flaky):
            with self.assertRaisesRegex(module.IngressError, 'recovery was unsafe or failed'):
                self.update(run)
        self.check_properties(self.desired)
        self.recovery()

    def test_old_configuration_test_failure_keeps_restored_file_and_evidence(self):
        def run(*args):
            self.commands.append(args)
            raise RuntimeError('nginx test failed')
        with self.assertRaisesRegex(module.IngressError, 'recovery was unsafe or failed'):
            self.update(run)
        self.check_properties(b'old configuration\n')
        self.assertEqual(self.commands, [('nginx', '-t'), ('nginx', '-t')])
        self.recovery()

    def test_restored_configuration_reload_failure_keeps_original_and_evidence(self):
        def run(*args):
            self.commands.append(args)
            if args == ('systemctl', 'reload', 'nginx'):
                raise RuntimeError('reload failed')
        with self.assertRaisesRegex(module.IngressError, 'recovery was unsafe or failed'):
            self.update(run)
        self.check_properties(b'old configuration\n')
        self.assertEqual(self.commands, [('nginx', '-t'), ('systemctl', 'reload', 'nginx'),
                                         ('nginx', '-t'), ('systemctl', 'reload', 'nginx')])
        self.recovery()

    def test_invalid_link_refuses_before_update(self):
        self.link.unlink()
        self.link.write_bytes(b'not a symlink')
        with self.assertRaises(module.IngressError):
            self.update()
        self.check_properties_without_link(b'old configuration\n')
        self.assertEqual(self.commands, [])
        self.assertEqual(list(self.stage.iterdir()), [])

    def check_properties_without_link(self, expected):
        self.assertEqual(self.destination.read_bytes(), expected)
        actual = self.destination.stat()
        self.assertEqual(stat.S_IMODE(actual.st_mode), stat.S_IMODE(self.original.st_mode))
        self.assertEqual((actual.st_uid, actual.st_gid), (self.original.st_uid, self.original.st_gid))

    def test_cutover_ingress_failure_remains_covered_by_stop_guard(self):
        source = (HERE / 'activate.py').read_text()
        tree = ast.parse(source)
        block = next(node for node in tree.body if isinstance(node, ast.If)
                     and ast.get_source_segment(source, node.test) == 'args.provision_ingress'
                     and 'ingress_activation.update_existing' in ast.get_source_segment(source, node))
        compiled = compile(ast.Module(body=[block], type_ignores=[]), str(HERE / 'activate.py'), 'exec')
        (self.stage / 'nginx.conf').write_bytes(self.desired)
        backups = self.root / 'backups'
        backups.mkdir()
        stops = []
        guard = trial82_activation.Trial82CutoverGuard(lambda *args: stops.append(args), backups,
                                                       'a' * 40, 'b' * 64, 'c' * 40)
        guard.prepare()
        def mapped_path(name):
            if name == '/etc/nginx/sites-available/hyjs-test.conf':
                return self.destination
            if name == '/etc/nginx/sites-enabled/hyjs-test.conf':
                return self.link
            return Path(name)
        count = []
        def fail_candidate(*args):
            count.append(args)
            if len(count) == 1:
                raise RuntimeError('candidate syntax rejected')
        namespace = {'args': SimpleNamespace(provision_ingress=True, apply_trial82=True,
                                              apply_feedback84=True, apply_feedback85=True),
                     'Path': mapped_path, 'trial82_activation': trial82_activation,
                     'ingress_activation': module, 'stage': self.stage, 'run': fail_candidate,
                     'ingress_desired': self.desired}
        with self.assertRaises(module.IngressError):
            exec(compiled, namespace)
        guard.stop_if_unfinished()
        self.assertEqual(stops, [('stop', '-t', '30', 'app')])
        self.assertEqual(json.loads(guard.manifest.read_text())['state'], 'failed-maintenance')
        self.check_properties(b'old configuration\n')

    def test_bundle_carries_verified_helper_and_first_provision_stays_distinct(self):
        deploy = (HERE / 'deploy.py').read_text()
        activate = (HERE / 'activate.py').read_text()
        self.assertIn("'ingress_activation.py'", deploy)
        self.assertIn("assert 'ingress_activation.py' in manifest['files']", activate)
        self.assertIn("ingress_activation.preflight(stage / 'nginx.conf', manifest['files']['nginx.conf'], run)", activate)
        self.assertIn('destination.write_bytes(desired)', activate)

    def test_deployment_retry_preserves_ingress_recovery_evidence(self):
        source = (HERE / 'deploy.py').read_text()
        tree = ast.parse(source)
        assignment = next(node for node in tree.body if isinstance(node, ast.Assign)
                          and any(isinstance(target, ast.Name) and target.id == 'prepare' for target in node.targets))
        compiled = compile(ast.Module(body=[assignment], type_ignores=[]), str(HERE / 'deploy.py'), 'exec')
        remote = self.root / 'remote'
        incoming = remote / '.incoming'
        incoming.mkdir(parents=True)
        (remote / '.hyjs-owned').touch()
        revision = 'a' * 40
        existing = incoming / revision[:12]
        existing.mkdir()
        (existing / 'artifact.json').write_text(json.dumps({'revision': revision}))
        evidence = existing / '.ingress-recovery-owned'
        evidence.mkdir()
        (evidence / 'original.conf').write_bytes(b'old')
        namespace = {'remote_stage': str(existing), 'REMOTE': str(remote), 'revision': revision}
        exec(compiled, namespace)
        with self.assertRaisesRegex(AssertionError, 'recovery evidence'):
            exec(namespace['prepare'], {})
        self.assertEqual((evidence / 'original.conf').read_bytes(), b'old')

        # Without retained ingress evidence the existing reproducible stage can
        # still be recreated by the original deployment flow.
        (evidence / 'original.conf').unlink()
        evidence.rmdir()
        exec(namespace['prepare'], {})
        self.assertTrue(existing.is_dir())
        self.assertEqual(list(existing.iterdir()), [])


if __name__ == '__main__':
    unittest.main()
