"""One original Type4 root, actual login facts and shared claims on disposable InnoDB."""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import unittest

from server.codec import BY_NAME
import test_p1_mysql as p1
import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_P1_DIRECTORY_MYSQL_TEST') == '1', 'Explicit isolated login directory test')
class LoginDirectoryMySQLTests(p1.P1MySQLTests):
    start = shared.TalentScopeMySQLTests.start

    @classmethod
    def setUpClass(cls):
        cls.database = 'hyjs_test_p1_' + secrets.token_hex(5)
        cls.test_user = 'p1_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p1-directory-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'server')
        cls.test_binary = str(Path(cls.temp.name) / 'service.test')
        cls.frozen_source = Path(os.environ['HYJS_P1_DIRECTORY_BASE']).resolve()
        if (p1.ROOT / 'build/candidates').resolve() not in cls.frozen_source.parents:
            raise AssertionError('Use a fixed candidate')
        for args in (['go','build','-o',cls.binary,'.'], ['go','test','-race','-c','-o',cls.test_binary,'./service']):
            result = subprocess.run(['rtk','proxy'] + args, cwd=cls.frozen_source, capture_output=True, timeout=180)
            if result.returncode: raise AssertionError((result.stdout+result.stderr).decode(errors='replace')[-4000:])
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user, HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database,
                       HYJS_P1_MYSQL_HOST='127.0.0.1:33068', HYJS_P1_MYSQL_USER=cls.test_user, HYJS_P1_MYSQL_PASSWORD=cls.test_password, HYJS_P1_MYSQL_DATABASE=cls.database)
        # Compile before acquiring the full schema-to-cleanup lifecycle lock.
        cls.lock = open('/tmp/hyjs-isolated-mysql.lock','a')
        fcntl.flock(cls.lock, fcntl.LOCK_EX)
        cls.addClassCleanup(cls.lock.close)
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")
        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0','0','0']: raise AssertionError('Login directory isolated resources remain')
            if getattr(cls,'report',None):
                cls.report['cleanupCounts']=[0,0,0]
                (p1.ROOT/'analysis/p1/login-directory-captions-mysql-validation.json').write_text(json.dumps(cls.report,indent=2)+'\n')
        cls.addClassCleanup(cleanup)

    def test_directory_original_frames_real_day_once_and_cold(self):
        began=time.monotonic()
        domain=subprocess.run(['rtk','proxy',self.test_binary,'-test.run=^TestP1(LoginDirectory|ActivityGoalValue)','-test.count=1','-test.timeout=2m'],cwd=self.frozen_source/'service',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(domain.returncode,0,(domain.stdout+domain.stderr).replace(self.test_password,'[redacted]')[-4000:])
        # Domain tests deliberately hot-edit global period rules. Start the
        # independent protocol scenario with a fresh disposable schema.
        self.root_sql(f"DROP DATABASE `{self.database}`; CREATE DATABASE `{self.database}` CHARACTER SET utf8mb4;")
        self.start();uid='p1-directory-'+secrets.token_hex(5);login=self.login(uid)
        self.assertEqual(self.send('CSPlayerCreate',{'playerName':'登录目录原协议','heroId':'hero_043'},login)[0],BY_NAME['SCPlayerCreate'])
        self.assertEqual(self.send('CSPlayerLogin',login=login)[0],BY_NAME['SCPlayerLogin'])
        key='totalLogin_p1_directory'
        tabs=self.send('CSActivityList',login=login)[1].activityList
        roots=[x.activityId for x in tabs if x.activityId.split('_')[0]=='totalLogin']
        self.assertEqual(roots,[key])
        op,info=self.send('CSActivityInfo',{'activityId':key},login)
        self.assertEqual(op,BY_NAME['SCActivityInfo']);self.assertEqual(len(info.itemList),7)
        self.assertEqual(info.information,'各条奖励独立统计，领取期限见条目。')
        self.assertTrue(all('·'+str(x.total)+'天·领至' in x.message for x in info.itemList))
        one=next(x for x in info.itemList if [(r.id,r.num) for r in x.rewardList]==[('coin',1000)])
        three=next(x for x in info.itemList if x.total==3 and [(r.id,r.num) for r in x.rewardList]==[('item_001',1)])
        self.assertEqual((one.value,one.left,one.total,one.status),('1',1,1,1))
        self.assertEqual((three.value,three.left,three.total,three.status),('3',1,3,0))
        req={'activityId':key,'index':one.index,'getResources':[{'id':x.id,'num':x.num} for x in one.rewardList]}
        facts=self.sql(f'SELECT COUNT(*) FROM p1_event_facts WHERE player_id={login.playerId};').strip()
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies=list(pool.map(lambda _:self.send('CSActivityReward',req,login),range(8)))
        self.assertTrue(all(op==BY_NAME['SCActivityReward'] for op,r in replies))
        self.assertEqual(sum(bool(getattr(r,'get').simps) for op,r in replies),1)
        self.assertTrue(all(len(r.itemList)==7 for op,r in replies))
        self.assertEqual(self.send('CSPlayer',login=login)[1].player.coin,1000)
        child=self.sql(f'SELECT instance_id FROM p1_login_directory_members WHERE index_id={one.index};').strip()
        legacy=dict(req,activityId=child,index=1)
        self.assertFalse(getattr(self.send('CSActivityReward',legacy,login)[1],'get').simps)
        reward=int(self.sql(f"SELECT reward_id FROM p1_period_progress WHERE player_id={login.playerId} AND instance_id='{child}';").strip())
        self.assertFalse(self.send('CSAddRewardsToPlayer',{'rewardIds':[reward]},login)[1].result.simps)
        self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p1_event_facts WHERE player_id={login.playerId};').strip(),facts)
        legacy_item=next(x for x in info.itemList if [(r.id,r.num) for r in x.rewardList]==[('coin',10000)])
        legacy_req={'activityId':key,'index':legacy_item.index,'getResources':[{'id':'coin','num':10000}]}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            legacy_replies=list(pool.map(lambda _:self.send('CSActivityReward',legacy_req,login),range(8)))
        self.assertEqual(sum(op==BY_NAME['SCActivityReward'] for op,r in legacy_replies),1)
        self.assertTrue(all(len(r.itemList)==7 for op,r in legacy_replies if op==BY_NAME['SCActivityReward']))
        self.assertEqual(self.send('CSActivityReward',dict(legacy_req,activityId='totalLogin_local_20261011',index=1),login)[0],10001)
        self.assertEqual(self.send('CSPlayer',login=login)[1].player.coin,11000)
        self.stop();self.start();login=self.login(uid);self.send('CSPlayerLogin',login=login)
        cold=self.send('CSActivityInfo',{'activityId':key},login)[1]
        self.assertEqual([x.index for x in cold.itemList],[x.index for x in info.itemList])
        self.assertEqual([x.message for x in cold.itemList],[x.message for x in info.itemList])
        self.assertEqual(next(x.status for x in cold.itemList if x.index==one.index),2)
        self.assertEqual(next(x.status for x in cold.itemList if x.index==legacy_item.index),2)
        self.assertFalse(getattr(self.send('CSActivityReward',req,login)[1],'get').simps)
        self.assertEqual(self.send('CSPlayer',login=login)[1].player.coin,11000)
        self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_reward_deliveries WHERE player_id={login.playerId} AND source_key='activity:{child}';").strip(),'1')
        type(self).report={'status':'passed','elapsedSeconds':round(time.monotonic()-began,3),'fixedSource':str(self.frozen_source.relative_to(p1.ROOT)),
            'binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'originalType4Roots':roots,'goMySQLRace':domain.stdout.strip(),
            'goals':['1','3'],'realLoginDays':1,'eightClaims':'one grant','crossEntry':'direct and reward-center empty','cold':'indices and delivery unchanged',
            'formalResourcesInjected':False,'authoritative':False}

    # Inherited regressions are exercised by the separate P1 suite/full Go run.
    test_direct_transactions_with_isolated_mysql = None
    test_native_rewards_mail_login_facts_restart_and_cross_entry_claim = None


if __name__ == '__main__': unittest.main()
