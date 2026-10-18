"""Check that unattended device failures stop before installation or credential input."""
import subprocess
import contextlib
import io
import json
import hashlib
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

from tools.device_demo import Device, PACKAGE, MULTI_PACKAGE


class DeviceFailureTests(unittest.TestCase):
    def test_owned_packages_do_not_accept_each_others_foreground_or_credentials(self):
        for selected, other in ((PACKAGE, MULTI_PACKAGE), (MULTI_PACKAGE, PACKAGE)):
            device = Device('stub-device', selected)
            self.assertTrue(device.game_is_foreground('topResumedActivity= ' + selected + '/.Game'))
            self.assertFalse(device.game_is_foreground('topResumedActivity= ' + other + '/.Game'))
            self.assertFalse(device.game_is_foreground('topResumedActivity= ' + selected + '.foreign/.Game'))
            with patch.object(device, 'require_game_surface_accessible'), \
                    patch.object(device, 'text', return_value='topResumedActivity= ' + other + '/.Game'), \
                    patch.object(device, 'run') as command:
                with self.assertRaisesRegex(RuntimeError, 'foreground demo'):
                    device.login_saved('must-not-be-read.json')
                command.assert_not_called()

    def test_secondary_install_refuses_main_apk_before_device_changes(self):
        with tempfile.TemporaryDirectory() as folder:
            apk = Path(folder)/'candidate.apk'
            apk.write_bytes(b'controlled-test')
            device = Device('stub-device', MULTI_PACKAGE)
            with patch('tools.device_demo.ROOT', Path(folder)), \
                    patch('tools.device_demo.subprocess.run', return_value=SimpleNamespace(stdout="package: name='"+PACKAGE+"' versionCode='3001'")), \
                    patch.object(device, 'prepare') as prepare, patch.object(device, 'run') as command:
                with self.assertRaisesRegex(RuntimeError, 'exact owned multi-client'):
                    device.install(apk)
                prepare.assert_not_called()
                command.assert_not_called()

    def test_stalled_vendor_commit_retries_only_the_new_owned_session(self):
        with tempfile.TemporaryDirectory() as folder:
            apk = Path(folder) / 'candidate.apk'
            apk.write_bytes(b'owned-session-candidate')
            expected = hashlib.sha256(apk.read_bytes()).hexdigest()
            device = Device('stub-device')
            proc = SimpleNamespace(returncode=1, pid=123, poll=lambda: None,
                                   terminate=lambda: None,
                                   communicate=lambda **kwargs: (b'', b''))
            replies = ['Success: created install session [73]', 'Success: streamed', 'Success',
                       'package:/isolated/base.apk', expected + ' /isolated/base.apk',
                       'package:/isolated/base.apk', expected + ' /isolated/base.apk']
            with patch('tools.device_demo.ROOT', Path(folder)), patch.object(device, 'prepare'), \
                    patch.object(device, 'run'), patch.object(device, 'approve_own_install'), \
                    patch.object(device, 'text', side_effect=replies) as command, \
                    patch.object(device, 'restart') as restart, \
                    patch.object(device, 'wait_rendered', return_value={}), \
                    patch('tools.device_demo.time.monotonic', side_effect=[0, 40]), \
                    patch('tools.device_demo.subprocess.Popen', return_value=proc) as installer:
                self.assertEqual(device.install(apk)['apk_sha256'], expected)
                restart.assert_called_once()
                installer.assert_called_once()
                commits = [call.args for call in command.call_args_list if 'install-commit' in call.args]
                self.assertEqual(commits, [('shell', 'cmd', 'package', 'install-commit', '73')])
                self.assertEqual(installer.call_args.args[0][-2:], ['install-commit', '73'])

    def test_saved_pending_session_prevents_a_second_install(self):
        with tempfile.TemporaryDirectory() as folder:
            apk = Path(folder) / 'candidate.apk'
            apk.write_bytes(b'candidate')
            state = Path(folder) / '.local-env/owned-install-stub-device.json'
            state.parent.mkdir()
            state.write_text('{"sessionId":"71"}')
            device = Device('stub-device')
            with patch('tools.device_demo.ROOT', Path(folder)), patch.object(device, 'prepare'), \
                    patch.object(device, 'run') as command, patch('tools.device_demo.subprocess.Popen') as installer:
                with self.assertRaisesRegex(RuntimeError, 'previous owned install session'):
                    device.install(apk)
                command.assert_not_called()
                installer.assert_not_called()

    def test_vendor_install_failure_only_continues_for_exact_installed_candidate(self):
        with tempfile.TemporaryDirectory() as folder:
            apk = Path(folder) / 'candidate.apk'
            apk.write_bytes(b'isolated-test-candidate')
            expected = hashlib.sha256(apk.read_bytes()).hexdigest()
            for installed in (expected, '0' * 64):
                with self.subTest(exact_candidate=installed == expected):
                    device = Device('stub-device')
                    proc = SimpleNamespace(returncode=1, pid=123,
                                           poll=lambda: 1,
                                           communicate=lambda **kwargs: (b'Failure [INSTALL_FAILED_ABORTED]', b''))
                    with patch.object(device, 'prepare'), \
                            patch('tools.device_demo.ROOT', Path(folder)), \
                            patch.object(device, 'run'), \
                            patch.object(device, 'text', side_effect=['Success: created install session [7]', 'Success: streamed', 'package:/isolated/base.apk', installed + ' /isolated/base.apk']), \
                            patch.object(device, 'restart') as restart, \
                            patch.object(device, 'wait_rendered', return_value={'game_foreground': True}), \
                            patch('tools.device_demo.subprocess.Popen', return_value=proc) as installer:
                        if installed == expected:
                            result = device.install(apk)
                            self.assertEqual(result['apk_sha256'], expected)
                            restart.assert_called_once()
                        else:
                            with self.assertRaisesRegex(RuntimeError, 'differs from candidate'):
                                device.install(apk)
                            restart.assert_not_called()
                        self.assertEqual(installer.call_count, 1)
                        self.assertEqual(installer.call_args.args[0][-2:], ['install-commit', '7'])

    def test_prepare_preserves_the_owners_longer_timeout_and_never_sleeps(self):
        for timeout, changes in [('5000', True), ('2147483647', False), ('-1', False)]:
            with self.subTest(timeout=timeout), tempfile.TemporaryDirectory() as folder:
                device = Device('test-device')
                with patch.object(device, 'display_state_file', return_value=Path(folder) / 'display.json'), \
                        patch.object(device, 'text', return_value=timeout), \
                        patch.object(device, 'keyguard', return_value={'showing': False}), \
                        patch.object(device, 'run') as command:
                    device.prepare()
                calls = [item.args for item in command.call_args_list]
                self.assertEqual(('shell', 'settings', 'put', 'system', 'screen_off_timeout', '1800000') in calls, changes)
                self.assertIn(('shell', 'input', 'keyevent', 'KEYCODE_WAKEUP'), calls)
                self.assertFalse(any('KEYCODE_SLEEP' in call for call in calls))

    def test_adb_timeout_does_not_expose_credential_arguments(self):
        device = Device('test-device')
        credential = 'private-value-that-must-not-appear'
        timeout = subprocess.TimeoutExpired(['adb', 'input', 'text', credential], 20)
        with patch('tools.device_demo.subprocess.run', side_effect=timeout):
            with self.assertRaises(RuntimeError) as result:
                device.run('shell', 'input', 'text', credential)
        self.assertNotIn(credential, str(result.exception))
        self.assertTrue(result.exception.__suppress_context__)

    def test_secure_keyguard_prevents_saved_login_and_render_access(self):
        device = Device('test-device')
        with patch.object(device, 'keyguard', return_value={'showing': True, 'secure': True, 'occluded': False}), \
                patch.object(device, 'run') as command:
            with self.assertRaisesRegex(RuntimeError, 'owner to unlock'):
                device.login_saved('does-not-need-to-exist.json')
            with self.assertRaisesRegex(RuntimeError, 'owner to unlock'):
                device.wait_rendered(1)
            command.assert_not_called()

    def test_lock_occlusion_only_allows_the_own_foreground_game(self):
        device = Device('test-device')
        with patch.object(device, 'keyguard', return_value={'showing': True, 'secure': True, 'occluded': True}):
            with patch.object(device, 'text', return_value='topResumedActivity= com.android.systemui/.LockScreen'):
                with self.assertRaisesRegex(RuntimeError, 'owner to unlock'):
                    device.require_game_surface_accessible()


class SavedLoginTests(unittest.TestCase):
    @contextlib.contextmanager
    def harness(self, rows, *, saved_changes=None, command_error=None, selector_seen=True, login_seen=True):
        """Replace every device/DB operation; use temporary non-production secrets."""
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder).resolve()
            account = root / '.local-env' / 'saved.json'
            account.parent.mkdir()
            saved = {'uid': 'secret-expected-uid', 'password': 'secret-password',
                     'initialized': True, 'player_id': 3, 'role': 'starter'}
            saved.update(saved_changes or {})
            account.write_text(json.dumps(saved))
            device = Device('stub-device')
            events = []

            def command(*args, **kwargs):
                # Store operation names, never credential values, in test evidence.
                events.append(('command', args[2]))
                if command_error and args[2] == 'text':
                    raise RuntimeError(saved['uid'] + saved['password'])
                return b''

            stdout, stderr = io.StringIO(), io.StringIO()
            with patch('tools.device_demo.ROOT', root), \
                    patch.object(device, 'require_game_surface_accessible'), \
                    patch.object(device, 'text', return_value='topResumedActivity= ' + PACKAGE), \
                    patch.object(device, 'frame', return_value=SimpleNamespace(width=2800, height=1260)), \
                    patch.object(device, 'run', side_effect=command) as adb, \
                    patch.object(device, 'ocr', side_effect=lambda contains='':
                                 ([{'text': '游戏登录', 'y': .2}, {'text': '账号：请输入邮箱', 'y': .31},
                                   {'text': '密码：请输入密码', 'y': .41}] if login_seen else [])
                                 if not contains else ([{'text': '当前区:本地测试'}] if selector_seen else [])), \
                    patch.object(device, 'tap_text', side_effect=lambda value: events.append(('enter', value))), \
                    patch('tools.device_demo.time.sleep', side_effect=lambda value: events.append(('wait', value))), \
                    patch('tools.demo_accounts.sql', side_effect=rows) as database, \
                    contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                yield device, account, events, adb, database, stdout, stderr, saved

    def test_town_or_reconnect_modal_stops_before_reading_target_or_entering_credentials(self):
        with self.harness([], login_seen=False) as h:
            device, account, events, adb, database, stdout, stderr, saved = h
            with self.assertRaises(RuntimeError):
                device.login_saved(account)
            adb.assert_not_called()
            database.assert_not_called()
            # The existing render-boundary retry observes the same empty dialog
            # three more times; it must never type, tap, or read saved credentials.
            self.assertEqual(events, [('wait', 1)] * 3)

    def test_correct_saved_target_rotates_session_and_waits_for_editor_commit(self):
        with self.harness(['3\t' + 'a' * 64, '3\t' + 'b' * 64]) as h:
            device, account, events, adb, database, stdout, stderr, saved = h
            result = device.login_saved(account)
            self.assertEqual(result, {'login': 'verified', 'account_role': 'starter'})
            self.assertEqual(events, [('command', 'tap'), ('wait', 1.5),
                                     ('command', 'text'), ('wait', 0.5),
                                     ('command', 'keyevent'), ('wait', 0.5)] * 2 +
                                    [('command', 'tap'), ('wait', 1.2), ('enter', '进入游戏'), ('wait', 1.2)])
            self.assertEqual(database.call_count, 2)
            for item in database.call_args_list:
                query = item.args[0]
                self.assertTrue(query.startswith('SELECT id,SHA2(session,256) FROM profiles WHERE uid='))
                self.assertIn('AND player IS NOT NULL', query)
                self.assertNotIn(saved['password'], query)
                self.assertNotIn(saved['uid'], query)
            self.assertEqual(stdout.getvalue() + stderr.getvalue(), '')
            self.assertNotIn(saved['uid'], json.dumps(result))
            self.assertNotIn(saved['password'], json.dumps(result))

    def test_wrong_typed_uid_or_password_never_retypes_or_creates_a_character(self):
        for cause in ('dropped UID character', 'wrong password'):
            with self.subTest(cause=cause), self.harness(['3\t' + 'a' * 64] * 7) as h:
                device, account, events, adb, database, stdout, stderr, saved = h
                with self.assertRaises(RuntimeError) as stopped:
                    device.login_saved(account)
                self.assertEqual(sum(event == ('command', 'text') for event in events), 2)
                self.assertEqual(sum(event == ('command', 'tap') for event in events), 3)
                self.assertEqual(database.call_count, 7)
                self.assertTrue(stopped.exception.__suppress_context__)
                for secret in (saved['uid'], saved['password'], 'a' * 64):
                    self.assertNotIn(secret, str(stopped.exception))
                self.assertEqual(stdout.getvalue() + stderr.getvalue(), '')

    def test_login_dialog_does_not_authenticate_until_verified_server_selector(self):
        with self.harness(['3\t' + 'a' * 64], selector_seen=False) as h:
            device, account, events, adb, database, stdout, stderr, saved = h
            with self.assertRaises(RuntimeError):
                device.login_saved(account)
            self.assertFalse(any(event[0] == 'enter' for event in events))
            self.assertEqual(database.call_count, 1)
            self.assertEqual(stdout.getvalue() + stderr.getvalue(), '')

    def test_unknown_or_mismatched_saved_target_stops_before_input(self):
        for row in ('', '5\t' + 'a' * 64, '3\tNULL'):
            with self.subTest(row_kind=len(row)), self.harness([row]) as h:
                device, account, events, adb, database, stdout, stderr, saved = h
                with self.assertRaises(RuntimeError):
                    device.login_saved(account)
                adb.assert_not_called()
                self.assertEqual(database.call_count, 1)
                self.assertEqual(events, [])
                self.assertEqual(stdout.getvalue() + stderr.getvalue(), '')

    def test_changed_target_id_stops_even_when_session_rotates(self):
        with self.harness(['3\t' + 'a' * 64, '5\t' + 'b' * 64]) as h:
            device, account, events, adb, database, stdout, stderr, saved = h
            with self.assertRaises(RuntimeError):
                device.login_saved(account)
            self.assertEqual(database.call_count, 2)
            self.assertEqual(sum(event == ('command', 'text') for event in events), 2)
            self.assertEqual(stdout.getvalue() + stderr.getvalue(), '')

    def test_sensitive_command_and_database_errors_are_suppressed(self):
        cases = [(['3\t' + 'a' * 64], True),
                 (RuntimeError('secret-expected-uid secret-password session-data'), False)]
        for rows, command_error in cases:
            with self.subTest(command_error=command_error), \
                    self.harness(rows, command_error=command_error) as h:
                device, account, events, adb, database, stdout, stderr, saved = h
                with self.assertRaises(RuntimeError) as stopped:
                    device.login_saved(account)
                self.assertTrue(stopped.exception.__suppress_context__)
                for secret in (saved['uid'], saved['password'], 'session-data'):
                    self.assertNotIn(secret, str(stopped.exception))
                self.assertEqual(stdout.getvalue() + stderr.getvalue(), '')

    def test_initialized_account_without_player_reference_stops_before_input(self):
        with self.harness([], saved_changes={'player_id': None}) as h:
            device, account, events, adb, database, stdout, stderr, saved = h
            with self.assertRaises(RuntimeError):
                device.login_saved(account)
            adb.assert_not_called()
            database.assert_not_called()

    def test_existing_legacy_account_without_fixture_metadata_is_verified(self):
        with self.harness(['1\t' + 'a' * 64, '1\t' + 'b' * 64],
                          saved_changes={'initialized': False, 'player_id': None,
                                         'role': 'secret-password'}) as h:
            device, account, events, adb, database, stdout, stderr, saved = h
            self.assertEqual(device.login_saved(account),
                             {'login': 'verified', 'account_role': 'original'})
            self.assertEqual(database.call_count, 2)
            self.assertEqual(stdout.getvalue() + stderr.getvalue(), '')
            for marker in ('topResumedActivity=', 'mResumedActivity:'):
                with patch.object(device, 'text', return_value=marker + ' ' + PACKAGE + '/com.unity3d.player.UnityPlayerActivity'):
                    device.require_game_surface_accessible()


if __name__ == '__main__':
    unittest.main()
