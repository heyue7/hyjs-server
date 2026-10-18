"""Runner provisioning contracts with no SQL, Docker or service process."""
from pathlib import Path
from types import SimpleNamespace
import unittest
import tempfile
from unittest.mock import patch

import test_p11_friend_scope as friend
import test_p11_arena_scope as arena
import test_p11_mysql as legacy
import test_p7_mysql as rush
import test_p7_catalogue_mysql as catalogue
import test_p7_preview_mysql as preview
import test_p7_stories_mysql as stories


class FakeConnection:
    def __init__(self,label):
        self.database='hyjs_test_'+label+'_mock';self.user='hyjs_t_mock';self.password='mock-private-value';self.host='127.0.0.1:3306'
    def env(self):
        return {'HYJS_MYSQL_HOST':self.host,'HYJS_MYSQL_USER':self.user,'HYJS_MYSQL_PASSWORD':self.password,'HYJS_MYSQL_DATABASE':self.database}


class FakePool:
    instances=[]
    def __init__(self):self.calls=[];self.closed=False;self.instances.append(self)
    def allocate(self,label):self.calls.append(('allocate',label));return FakeConnection(label)
    def sql(self,database,text):self.calls.append(('sql',database,text));return 'mock-result'
    def close(self):self.closed=True


class ManagedRunnerContractTests(unittest.TestCase):
    def test_setup_uses_shared_allocation_scoped_env_and_cleanup_without_admin_sql(self):
        candidates=Path(__file__).resolve().parents[1]/'build/candidates'
        candidates.mkdir(parents=True,exist_ok=True)
        temporary=tempfile.TemporaryDirectory(prefix='p11-p7-managed-mock-',dir=candidates)
        self.addCleanup(temporary.cleanup)
        fixed=Path(temporary.name)/'hyjs-server'
        (fixed/'handler').mkdir(parents=True);(fixed/'service').mkdir()
        (fixed/'handler/gameHandler.go').write_text('CSFriendFight CSPvpFightResult')
        (fixed/'service/p11ArenaService.go').write_text('p11FriendScopedFightAt p11ArenaScopedFightAt')
        cases=[(friend,friend.P11FriendScopeTests,'HYJS_P11_FRIEND_BUILD_ROOT'),(arena,arena.P11ArenaScopeTests,'HYJS_P11_ARENA_BUILD_ROOT'),(legacy,legacy.P11MySQLTests,None),(rush,rush.P7MySQLTests,None)]
        for module,parent,variable in cases:
            with self.subTest(parent=parent.__name__):
                child=type('IsolatedManagedSetup',(parent,),{})
                commands=[]
                def no_service(args,**kwargs):
                    if not any(value in args for value in ['go','git','gofmt']):raise AssertionError('Unexpected external operation')
                    commands.append((args,kwargs));return SimpleNamespace(returncode=0,stdout='',stderr='')
                values={variable:str(fixed)} if variable else {}
                with patch.object(module,'ManagedMySQL',FakePool),patch.object(module.subprocess,'run',no_service),patch.dict(module.os.environ,values):
                    child.setUpClass()
                    try:
                        pool=FakePool.instances[-1]
                        self.assertEqual(child.env['HYJS_MYSQL_HOST'],'127.0.0.1:3306')
                        self.assertEqual(child.env['GOMEMLIMIT'],'1GiB')
                        self.assertTrue(child.database.startswith('hyjs_test_'))
                        self.assertEqual(child.database_sql('SELECT 1'),'mock-result')
                        self.assertEqual(pool.calls[-1],('sql',child.database,'SELECT 1'))
                        self.assertTrue(commands)
                        for _,options in commands:
                            if 'env' in options:self.assertEqual(options['env']['GOMEMLIMIT'],'1GiB')
                    finally:child.doClassCleanups()
                    self.assertTrue(pool.closed)

    def test_all_seven_runners_keep_schema_guard_and_do_not_send_use_or_admin_ddl(self):
        cases=[friend.P11FriendScopeTests,arena.P11ArenaScopeTests,legacy.P11MySQLTests,rush.P7MySQLTests,catalogue.ExpeditionCatalogueMySQLTests,preview.ExpeditionPreviewMySQLTests,stories.ExpeditionStoriesMySQLTests]
        for parent in cases:
            with self.subTest(parent=parent.__name__):
                child=type('IsolatedSQLGuard',(parent,),{})
                child.database='hyjs_test_p11_mock' if parent in cases[:3] else 'hyjs_test_p7_mock'
                child.managed=FakePool()
                self.assertEqual(child.database_sql('SELECT 2'),'mock-result')
                self.assertEqual(child.managed.calls[-1],('sql',child.database,'SELECT 2'))
                child.database='hyjs'
                with self.assertRaises(AssertionError):child.database_sql('SELECT 2')
                self.assertEqual(len(child.managed.calls),1)


if __name__=='__main__':unittest.main()
