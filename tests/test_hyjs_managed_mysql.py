"""Pure mocked manager-contract tests. Never connect to a local service."""
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import hyjs_managed_mysql as managed


class ManagedMySQLContractTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='hyjs-managed-contract-')
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.addCleanup(patch.stopall)
        patch.object(managed, 'SERVICES', self.base).start()
        self.commands = []
        self.fail_drop = set()

        def run(command, **kwargs):
            self.commands.append((command, kwargs))
            if command[2] == 'docker':
                return SimpleNamespace(returncode=0, stdout='mock-result\n', stderr='')
            action, database = command[4], command[6]
            if action == 'test-create':
                path = self.base / 'test-connections' / (database + '.env')
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text('MYSQL_HOST=127.0.0.1\nMYSQL_PORT=3306\nMYSQL_DATABASE=' + database + '\nMYSQL_USER=hyjs_t_mock\nMYSQL_PASSWORD=mock-secret-only\n')
            return SimpleNamespace(returncode=1 if action == 'test-drop' and database in self.fail_drop else 0, stdout='mock-result\n', stderr='mock-secret-only')

        self.pool = managed.ManagedMySQL(runner=run)

    def test_two_schema_same_registered_account_binary_collation_and_reverse_release(self):
        a, b = self.pool.allocate('p14_a'), self.pool.allocate('p14_b')
        self.assertNotEqual(a.database, b.database)
        self.assertEqual((a.user, a.password), (b.user, b.password))
        self.assertNotIn(a.password, repr(a))
        self.assertEqual(a.env()['HYJS_MYSQL_HOST'], '127.0.0.1:3306')
        self.assertEqual(b.env()['HYJS_P13_MYSQL_DATABASE'], b.database)
        self.assertEqual(self.commands[0][0][-2:], ['--collation', 'utf8mb4_bin'])
        self.assertEqual(self.commands[1][0][-2:], ['--user', a.user])
        self.assertEqual(self.pool.sql(a.database, "SELECT 'DROP USER is plain fixture text';"), 'mock-result\n')
        sql_command, sql_arguments = self.commands[-1]
        self.assertEqual(sql_command[-2:], [a.user, a.database])
        self.assertEqual(sql_command[5], 'codex-local-mysql')
        self.assertIn('--default-character-set=utf8mb4', sql_command[8])
        self.assertNotIn(a.password, ' '.join(sql_command))
        self.assertEqual(sql_arguments['input'], a.password + "\nSELECT 'DROP USER is plain fixture text';")
        self.pool.close()
        self.assertEqual([command[0][6] for command in self.commands if command[0][2] != 'docker' and command[0][4] == 'test-drop'], [b.database, a.database])
        self.assertTrue(all(command[0][:2] == ['rtk', 'proxy'] for command in self.commands))
        self.assertTrue(all('root' not in command[0] for command in self.commands))

    def test_refuses_admin_foreign_and_unowned_database_before_any_sql(self):
        connection = self.pool.allocate('scope')
        count = len(self.commands)
        rejected = ['CREATE USER evil;', 'DROP DATABASE hyjs;', 'CREATE /* ordinary comment */ USER evil;', 'GRANT ALL ON hyjs.* TO someone;', 'SET @@GLOBAL.max_connections=1;', 'SET PERSIST x=1;', 'FLUSH TABLES;', 'USE hyjs;', '/*!50000 DROP USER x */', 'UPDATE hyjs.profiles SET id=1;', 'SELECT * FROM other_project.table1;', 'CREATE TABLE IF NOT EXISTS other_project.x(id INT);']
        for text in rejected:
            with self.subTest(text=text), self.assertRaises(ValueError):
                self.pool.sql(connection.database, text)
        with self.assertRaises(ValueError):
            self.pool.sql('hyjs', 'SELECT 1')
        with self.assertRaises(ValueError):
            self.pool.release('hyjs_test_unowned')
        self.assertEqual(len(self.commands), count)

    def test_compat_use_routes_only_allocated_schema_and_redacts_all_passwords(self):
        a, b = self.pool.allocate('source'), self.pool.allocate('destination')
        fixture = SimpleNamespace(database=a.database, managed_mysql=self.pool)
        self.assertEqual(managed.case_sql(fixture, 'USE `' + b.database + '`; SELECT 1;'), 'mock-result\n')
        self.assertEqual(self.commands[-1][0][-1], b.database)
        with self.assertRaises(ValueError):
            managed.case_sql(fixture, 'USE hyjs; SELECT 1;')
        self.assertEqual(self.pool.redact('value=' + a.password), 'value=[redacted]')

    def test_cleanup_attempts_every_owned_schema_and_does_not_leak_diagnostics(self):
        a, b = self.pool.allocate('first'), self.pool.allocate('second')
        self.fail_drop.add(b.database)
        with self.assertRaises(RuntimeError) as captured:
            self.pool.close()
        self.assertNotIn('mock-secret-only', str(captured.exception))
        self.assertEqual([command[0][6] for command in self.commands if command[0][2] != 'docker' and command[0][4] == 'test-drop'], [b.database, a.database])
        self.assertEqual(list(self.pool.connections), [b.database])


if __name__ == '__main__':
    unittest.main()
