"""全部饰品闭环断言重复运行于独立 MySQL 库，完成后删除库及临时授权。"""
import json
import os
from pathlib import Path
import types
import unittest
import uuid

from test_go_treasure import GoTreasureTests
from test_go_treasure_source import GoTreasureSourceTests
from test_go_warrior_growth import MySQLWarriorGrowthTests


_mysql_start=MySQLWarriorGrowthTests.start


class SQLRows:
    def __init__(self, rows):
        self.rows=rows
    def fetchall(self):
        return self.rows
    def fetchone(self):
        return self.rows[0] if self.rows else None


class SQLConnection:
    def __init__(self, harness):
        self.harness=harness
    def __enter__(self):
        return self
    def __exit__(self, *_):
        return False
    def execute(self, sql, args=()):
        return SQLRows(self.harness.sql(sql,args))


class MySQLTreasureHarness:
    root_sql=MySQLWarriorGrowthTests.root_sql
    stop=GoTreasureTests.stop

    def setUp(self):
        self.repo=Path(__file__).resolve().parents[1]
        self.database='hyjs_test_treasure_'+str(os.getpid())+'_'+uuid.uuid4().hex[:8]
        self.assertTrue(self.database.startswith('hyjs_test_treasure_'))
        env=dict(os.environ)
        for line in (self.repo/'.local-env/mysql.env').read_text().splitlines():
            if line and not line.startswith('#') and '=' in line:
                key,value=line.split('=',1)
                env[key]=value.strip().strip('"').strip("'")
        self.env=dict(env,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER='hyjs',HYJS_MYSQL_DATABASE=self.database)
        # 先登记清理，再创建专属库，部分初始化失败也会执行撤权与删除。
        self.addCleanup(self.cleanup_database)
        self.root_sql('CREATE DATABASE `'+self.database+'` CHARACTER SET utf8mb4; GRANT ALL ON `'+self.database+"`.* TO 'hyjs'@'%';")
        self.addCleanup(self.stop)
        self.start()

    def cleanup_database(self):
        self.assertTrue(self.database.startswith('hyjs_test_treasure_'))
        self.root_sql('REVOKE ALL PRIVILEGES ON `'+self.database+"`.* FROM 'hyjs'@'%'; DROP DATABASE IF EXISTS `"+self.database+'`;')
        # 清理证据直接查询数据库与授权表，只输出专属库的计数。
        output=self.root_sql("SELECT COUNT(*) AS remaining FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='"+self.database+"'; SELECT COUNT(*) AS remaining FROM mysql.db WHERE Db='"+self.database+"';")
        self.assertEqual(output.splitlines(),['remaining','0','remaining','0'])

    def tearDown(self):
        self.stop()

    def start(self):
        _mysql_start(self)
        self.service.store=types.SimpleNamespace(connect=lambda:SQLConnection(self))

    def sql(self,text,args=()):
        self.assertTrue(self.database.startswith('hyjs_test_treasure_'))
        parts=text.split('?');self.assertEqual(len(parts),len(args)+1)
        rendered=parts[0]
        for value,tail in zip(args,parts[1:]):
            if isinstance(value,(int,float)):
                rendered+=str(value)
            elif value is None:
                rendered+='NULL'
            else:
                rendered+="CONVERT(X'"+str(value).encode().hex()+"' USING utf8mb4)"
            rendered+=tail
        lines=self.root_sql(rendered+';',self.database).splitlines()
        if not lines:return []
        keys=lines[0].split('\t');rows=[]
        for line in lines[1:]:
            row=dict(zip(keys,line.split('\t')))
            for key in ('enabled','max_batch','amount','weight'):
                if key in row:row[key]=int(row[key])
            rows.append(row)
        return rows

    def mutate(self,login,action):
        row=self.sql('SELECT player,game FROM profiles WHERE id=?',(login.playerId,))[0]
        player,game=json.loads(row['player']),json.loads(row['game'])
        action(player,game)
        self.sql('UPDATE profiles SET player=?,game=? WHERE id=?',(json.dumps(player),json.dumps(game),login.playerId))


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_TREASURE_TEST')=='1','需显式启用独立MySQL饰品回归')
class MySQLTreasureTests(MySQLTreasureHarness,GoTreasureTests):
    pass


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_TREASURE_TEST')=='1','需显式启用独立MySQL饰品来源回归')
class MySQLTreasureSourceTests(MySQLTreasureHarness,GoTreasureSourceTests):
    pass


del GoTreasureTests,GoTreasureSourceTests,MySQLWarriorGrowthTests
