"""Safety guards run without Docker; real cold restore is opt-in and uses owned engines only."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock
from tools.mysql_restore_drill import RestoreDrill

ROOT = Path(__file__).resolve().parents[1]


class RestoreDrillSafetyTests(unittest.TestCase):
    def test_foreign_engine_rejected_before_any_command(self):
        drill = RestoreDrill()
        self.addCleanup(drill.temp.cleanup)
        commands = []
        drill.run_command = lambda *args, **kwargs: commands.append(args)
        for name in ['hyjs-mysql', 'hyjs', 'hyjs-test-restore-a-000000000000']:
            with self.assertRaisesRegex(RuntimeError, 'owned engine guard'):
                drill.guard(name)
        self.assertEqual(commands, [])

    def test_fake_owned_name_requires_matching_container_label(self):
        drill = RestoreDrill()
        self.addCleanup(drill.temp.cleanup)
        name = 'hyjs-test-restore-a-' + drill.token
        drill.owned.append(name)
        drill.run_command = lambda *args, **kwargs: b'unrelated-owner\n'
        with self.assertRaisesRegex(RuntimeError, 'ownership label'):
            drill.guard(name)

    def test_ambient_mysql_password_never_reaches_owned_root_command(self):
        drill = RestoreDrill()
        self.addCleanup(drill.temp.cleanup)
        captured = []
        drill.guard = lambda name: None
        drill.run_command = lambda *args, **kwargs: captured.append(kwargs['env']) or b'1\n'
        with mock.patch.dict(os.environ, MYSQL_PWD='test-sentinel-only'):
            drill.sql('unit-owned-engine', 'SELECT 1;', root=True)
        self.assertNotIn('MYSQL_PWD', captured[0])


@unittest.skipUnless(os.environ.get('HYJS_RESTORE_DRILL_TEST') == '1', 'explicit isolated cold database restore')
class RestoreDrillColdMySQLTests(unittest.TestCase):
    def test_real_backup_fresh_engine_sigkill_rollback_and_original_protocol(self):
        with tempfile.TemporaryDirectory(prefix='hyjs-safe-drill-summary-') as folder:
            report_path = Path(folder) / 'summary.json'
            result = subprocess.run(['rtk', 'proxy', 'python3', 'tools/mysql_restore_drill.py', '--run-isolated', '--report', str(report_path)],
                                    cwd=ROOT, capture_output=True, text=True, timeout=600)
            self.assertEqual(result.returncode, 0, result.stderr)
            report = json.loads(report_path.read_text())
            self.assertTrue(report['checks'])
            self.assertTrue(all(report['checks'].values()))
            self.assertTrue(report['checks']['database_engine_sigkill_recovery'])
            self.assertTrue(report['checks']['uncommitted_transaction_rolled_back'])
            self.assertTrue(report['checks']['engines_users_databases_dumps_removed'])
            self.assertGreater(report['table_counts']['p13_guild_receipts'], 0)
            self.assertGreater(report['table_counts']['p11_grab_sessions'], 1)
            self.assertGreater(report['table_counts']['p6_battle_records'], 0)
            self.assertIn('whole P1-E.1 and P15-E.4', report['unfinished'])
            self.assertFalse({'password', 'session', 'account_password', 'credentials'} & set(report))
            print('Cold engine backup/restore checks:', len(report['checks']), 'elapsed:', report['elapsed_seconds'])


if __name__ == '__main__':
    unittest.main()
