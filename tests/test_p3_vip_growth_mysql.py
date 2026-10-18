"""P3真实隔离MySQL订单、会员及领奖回归；专属库和专属用户始终清理。"""
import os
from pathlib import Path
import secrets
import subprocess
import unittest
import uuid


@unittest.skipUnless(os.environ.get('HYJS_P3_MYSQL_TEST') == '1' and os.environ.get('HYJS_P3_VIP_GROWTH_MYSQL_TEST') == '1', '显式启用P3专属MySQL回归')
class P3VIPGrowthMySQLTests(unittest.TestCase):
    def test_defaultoff_original_vip_growth_orders_books_and_cold(self):
        root = Path(__file__).resolve().parents[1]
        candidate = Path(os.environ['HYJS_P3_VIP_GROWTH_CANDIDATE']).resolve()
        self.assertEqual(candidate.parent, root / 'build/candidates')
        self.assertTrue((candidate / 'manifest.json').is_file())
        suffix = uuid.uuid4().hex[:12]
        database = 'hyjs_test_p3_' + suffix
        user = 'hyjs_p3_' + suffix
        password = secrets.token_hex(24)
        self.assertTrue(database.startswith('hyjs_test_p3_'))

        def root_sql(query):
            result = subprocess.run(
                ['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                input=query, text=True, capture_output=True, timeout=30)
            if result.returncode:
                self.fail('P3 isolated database administration failed')
            return result.stdout

        try:
            root_sql(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; "
                     f"CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; "
                     f"GRANT ALL ON `{database}`.* TO '{user}'@'%';")
            env = dict(os.environ, GOMEMLIMIT='2GiB', HYJS_P3_MYSQL_DATABASE=database,
                       HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=user,
                       HYJS_MYSQL_PASSWORD=password)
            result = subprocess.run(
                ['rtk', 'proxy', 'go', 'test', '-race', './service', '-run', '^TestP3VIP9GrowthActualTestOrdersAndOriginalBook$', '-count=1', '-v'],
                cwd=candidate / 'hyjs-server', env=env, text=True, capture_output=True, timeout=120)
            # Database credentials remain in the child environment. Never print
            # arbitrary diagnostic output which could include a DSN or session.
            summary = '\n'.join(line for line in result.stdout.splitlines()
                                if line.startswith(('=== RUN', '--- PASS', '--- FAIL', '    --- PASS', '    --- FAIL', 'ok ', 'FAIL', '    p3VIP9Growth_test.go:')))
            self.assertEqual(result.returncode, 0, summary)
            self.assertIn('--- PASS: TestP3VIP9GrowthActualTestOrdersAndOriginalBook', summary)
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining = root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; "
                             f"SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; "
                             f"SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(), ['0', '0', '0'])


if __name__ == '__main__':
    unittest.main()
