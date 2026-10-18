"""觉醒全部协议断言运行于专属MySQL库，每次测试删除库和授权。"""
import os
from pathlib import Path
import types
import unittest
import uuid

from test_go_awaken import GoAwakenTests
from test_go_warrior_growth import MySQLWarriorGrowthTests


_root_sql = MySQLWarriorGrowthTests.root_sql
_mysql_start = MySQLWarriorGrowthTests.start


class SQLRows:
    def __init__(self, rows):
        self.rows = rows
    def fetchall(self):
        return self.rows
    def fetchone(self):
        return self.rows[0] if self.rows else None


class SQLConnection:
    def __init__(self, harness):
        self.harness = harness
    def __enter__(self):
        return self
    def __exit__(self, *_):
        return False
    def execute(self, sql, args=()):
        return SQLRows(self.harness.sql(sql, args))


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_AWAKE_TEST') == '1', '需显式启用独立MySQL觉醒回归')
class MySQLAwakenTests(GoAwakenTests):
    root_sql = _root_sql

    def setUp(self):
        self.repo = Path(__file__).resolve().parents[1]
        self.database = 'hyjs_test_awake_' + str(os.getpid()) + '_' + uuid.uuid4().hex[:8]
        self.assertTrue(self.database.startswith('hyjs_test_awake_'))
        env = dict(os.environ)
        for line in (self.repo/'.local-env/mysql.env').read_text().splitlines():
            if line and not line.startswith('#') and '=' in line:
                key, value = line.split('=', 1)
                env[key] = value.strip().strip('"').strip("'")
        self.env = dict(env, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER='hyjs', HYJS_MYSQL_DATABASE=self.database)
        self.addCleanup(self.cleanup_database)
        self.root_sql('CREATE DATABASE `' + self.database + '` CHARACTER SET utf8mb4; GRANT ALL ON `' + self.database + "`.* TO 'hyjs'@'%';")
        self.addCleanup(self.stop)
        self.start()

    def cleanup_database(self):
        self.assertTrue(self.database.startswith('hyjs_test_awake_'))
        self.root_sql('REVOKE ALL PRIVILEGES ON `' + self.database + "`.* FROM 'hyjs'@'%'; DROP DATABASE IF EXISTS `" + self.database + '`;')
        result = self.root_sql("SELECT COUNT(*) AS remaining FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='" + self.database + "'; SELECT COUNT(*) AS remaining FROM mysql.db WHERE Db='" + self.database + "';")
        self.assertEqual(result.splitlines(), ['remaining', '0', 'remaining', '0'])

    def tearDown(self):
        self.stop()

    def start(self):
        _mysql_start(self)
        self.service.store = types.SimpleNamespace(connect=lambda: SQLConnection(self))

    def sql(self, text, args=()):
        # 沿用既有安全字节绑定，仅改本测试允许的库前缀。
        self.assertTrue(self.database.startswith('hyjs_test_awake_'))
        parts = text.split('?')
        self.assertEqual(len(parts), len(args)+1)
        rendered = parts[0]
        for value, tail in zip(args, parts[1:]):
            if isinstance(value, (int, float)):
                rendered += str(value)
            elif value is None:
                rendered += 'NULL'
            else:
                rendered += "CONVERT(X'" + str(value).encode().hex() + "' USING utf8mb4)"
            rendered += tail
        rendered = rendered.replace('SELECT * FROM warrior_growth_receipts', 'SELECT player_id,guid,kind,timestamp,HEX(request) AS request,HEX(response) AS response FROM warrior_growth_receipts')
        lines = self.root_sql(rendered+';', self.database).splitlines()
        if not lines:
            return []
        keys = lines[0].split('\t')
        return [dict(zip(keys, line.split('\t'))) for line in lines[1:]]


del GoAwakenTests, MySQLWarriorGrowthTests
