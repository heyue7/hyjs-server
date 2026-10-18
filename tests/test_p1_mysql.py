"""P1 原二进制协议与真实 MySQL；专属库、专属用户和临时端口，结束自动清理。"""
import concurrent.futures
import http.client
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import subprocess
import tempfile
import time
import types
import unittest
from server.codec import BY_NAME
from test_server import LiveServerTests

ROOT = Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P1_MYSQL_TEST') == '1', '需要显式启用 P1 专属 MySQL 回归')
class P1MySQLTests(unittest.TestCase):
    send = LiveServerTests.send
    login = LiveServerTests.login

    @classmethod
    def setUpClass(cls):
        cls.database = 'hyjs_test_p1_' + secrets.token_hex(5)
        cls.test_user = 'p1_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p1-test-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-p1')
        built = subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=120)
        if built.returncode:
            raise AssertionError(built.stdout + built.stderr)
        cls.root_sql('CREATE DATABASE `' + cls.database + '` CHARACTER SET utf8mb4; CREATE USER ' + "'" + cls.test_user + "'@'%' IDENTIFIED BY '" + cls.test_password + "'; GRANT ALL ON `" + cls.database + "`.* TO '" + cls.test_user + "'@'%';")
        cls.addClassCleanup(lambda: cls.root_sql('DROP DATABASE IF EXISTS `' + cls.database + "`; DROP USER IF EXISTS '" + cls.test_user + "'@'%';"))
        cls.env = dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER=cls.test_user,HYJS_MYSQL_PASSWORD=cls.test_password,HYJS_MYSQL_DATABASE=cls.database,
                       HYJS_P1_MYSQL_HOST='127.0.0.1:33068',HYJS_P1_MYSQL_USER=cls.test_user,HYJS_P1_MYSQL_PASSWORD=cls.test_password,HYJS_P1_MYSQL_DATABASE=cls.database)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p1_') or not cls.test_user.startswith('p1_'):
            raise AssertionError('P1 isolation guard')
        result = subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c',
              'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if result.returncode:
            raise AssertionError(result.stderr.replace(cls.test_password,'[redacted]'))
        return result.stdout

    def setUp(self):
        self.process = None
        self.addCleanup(self.stop)

    def start(self):
        self.process = subprocess.Popen([self.binary,'--http-port','0','--tcp-port','0'],cwd=ROOT/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ)
            self.assertTrue(ready.select(30),'P1 服务启动超时')
            line=self.process.stdout.readline()
        match=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line)
        self.assertIsNotNone(match,line)
        self.service=types.SimpleNamespace(http_port=int(match[1]),tcp_port=int(match[2]))

    def stop(self):
        if self.process is not None and self.process.poll() is None:
            self.process.terminate()
            try:self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.process.kill();self.process.wait();self.fail('P1 服务未按ctx关闭')
        if self.process is not None and self.process.stdout:self.process.stdout.close()

    def sql(self,text):
        self.assertTrue(self.database.startswith('hyjs_test_p1_'))
        return self.root_sql('USE `' + self.database + '`; ' + text)

    def test_direct_transactions_with_isolated_mysql(self):
        completed=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP1([^0-9]|$)','-count=1'],cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=180)
        self.assertEqual(completed.returncode,0,(completed.stdout+completed.stderr).replace(self.test_password,'[redacted]'))
        print(completed.stdout.strip())

    def test_native_rewards_mail_login_facts_restart_and_cross_entry_claim(self):
        self.start();uid='p1-network-'+secrets.token_hex(4);login=self.login(uid)
        self.assertEqual(self.send('CSPlayerCreate',{'playerName':'公共奖励网络回归','heroId':'hero_043'},login)[0],BY_NAME['SCPlayerCreate'])
        for _ in range(3):self.assertEqual(self.send('CSPlayerLogin',login=login)[0],BY_NAME['SCPlayerLogin'])
        op,check=self.send('CSCheckNewMail',login=login);self.assertEqual((op,check.hasNew),(BY_NAME['SCCheckNewMail'],1))
        op,mail=self.send('CSMailList',{'mailType':'sysMail'},login);self.assertEqual(op,BY_NAME['SCMailList']);self.assertEqual(mail.mailBoxList[0].id,'sysMail');self.assertEqual(len(mail.mailBoxList[0].mailList),1)
        letter=mail.mailBoxList[0].mailList[0]
        self.assertEqual(self.send('CSDeleteMail',{'type':letter.type,'createTime':letter.time},login)[0],10001)
        self.stop();self.start();login=self.login(uid)
        op,rewards=self.send('CSQueryUserReward',login=login);self.assertEqual(op,BY_NAME['SCQueryUserReward']);self.assertEqual(len(rewards.rewardItemInfoList),1)
        reward=rewards.rewardItemInfoList[0];self.assertEqual([(r.id,r.num) for r in reward.rewardList],[('coin',1000)])
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies=list(pool.map(lambda _:self.send('CSAddRewardsToPlayer',{'rewardIds':[reward.rewardId]},login),range(6)))
        self.assertEqual(sum(bool(r.result.simps) for op,r in replies),1)
        self.assertTrue(all(op==BY_NAME['SCAddRewardsToPlayer'] for op,r in replies))
        self.assertEqual(self.send('CSPlayer',login=login)[1].player.coin,1000)
        key=self.sql('SELECT instance_id FROM p1_period_progress WHERE player_id='+str(login.playerId)+' AND reward_id='+str(reward.rewardId)+';').strip()
        op,claimed=self.send('CSActivityReward',{'activityId':key,'index':1,'getResources':[{'id':'coin','num':1000}]},login)
        self.assertEqual(op,BY_NAME['SCActivityReward']);self.assertFalse(claimed.get.simps)
        self.assertEqual(self.send('CSDeleteMail',{'type':letter.type,'createTime':letter.time},login)[0],BY_NAME['SCDeleteMail'])
        self.stop();self.start();login=self.login(uid);self.send('CSPlayerLogin',login=login)
        self.assertFalse(self.send('CSQueryUserReward',login=login)[1].rewardItemInfoList)
        self.assertFalse(self.send('CSMailList',{'mailType':'allMail'},login)[1].mailBoxList)
        self.assertEqual(self.send('CSPlayer',login=login)[1].player.coin,1000)
        # 运维实际调用方可核对运行版本、来源与领取状态；输出不包含正文或凭据。
        connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=5)
        connection.request('POST','/local/operations',body=json.dumps({'playerId':login.playerId,'instanceId':key}),headers={'Content-Type':'application/json'})
        response=connection.getresponse();self.assertEqual(response.status,200);ops=json.loads(response.read());connection.close()
        self.assertEqual(ops['code'],0);data=ops['data']
        self.assertEqual((data['pendingRewards'],data['claimedRewards'],data['status'],data['database']),(0,1,'ready','mysql'))
        self.assertEqual(data['binarySha256'],hashlib.sha256(Path(self.binary).read_bytes()).hexdigest());self.assertGreater(data['startedAt'],0)
        self.assertEqual(data['recent'][0]['id'],reward.rewardId)
        self.assertFalse({'password','session','content','title'} & set(data['recent'][0]))
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p1_event_facts WHERE player_id='+str(login.playerId)+" AND kind='login_day';").strip(),'1')
        engines=self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p1_%';")
        self.assertEqual(engines.strip(),'InnoDB')
        # 本专属用户没有正式库授权，恢复/回归无法修改正式数据。
        denied=subprocess.run(['rtk','proxy','docker','exec','-i','-e','MYSQL_PWD','hyjs-mysql','mysql','-u'+self.test_user,'--batch','--skip-column-names'],input='SELECT COUNT(*) FROM hyjs.profiles;',env=dict(os.environ,MYSQL_PWD=self.test_password),text=True,capture_output=True,timeout=15)
        self.assertNotEqual(denied.returncode,0)
        self.verify_isolated_backup_restore(uid,login.playerId)

    def verify_isolated_backup_restore(self,uid,player_id):
        # 真正逻辑备份到0600临时文件，再恢复到另一个专属库，原测试库也不被覆盖。
        self.stop()
        restore_database=self.database+'_restore'
        self.assertTrue(restore_database.startswith('hyjs_test_p1_'))
        self.root_sql('CREATE DATABASE `'+restore_database+'` CHARACTER SET utf8mb4; GRANT ALL ON `'+restore_database+"`.* TO '"+self.test_user+"'@'%';")
        self.addCleanup(lambda:self.root_sql('DROP DATABASE IF EXISTS `'+restore_database+'`;'))
        self.addCleanup(self.stop)
        backup=Path(self.temp.name)/'p1-private-backup.sql'
        backup.touch(mode=0o600)
        secret_env=dict(os.environ,MYSQL_PWD=self.test_password)
        with backup.open('wb') as target:
            saved=subprocess.run(['rtk','proxy','docker','exec','-i','-e','MYSQL_PWD','hyjs-mysql','mysqldump','-u'+self.test_user,'--single-transaction','--no-tablespaces','--skip-comments','--skip-lock-tables',self.database],env=secret_env,stdout=target,stderr=subprocess.PIPE,timeout=30)
        self.assertEqual(saved.returncode,0,saved.stderr.decode().replace(self.test_password,'[redacted]'))
        self.assertGreater(backup.stat().st_size,0)
        with backup.open('rb') as source:
            restored=subprocess.run(['rtk','proxy','docker','exec','-i','-e','MYSQL_PWD','hyjs-mysql','mysql','-u'+self.test_user,restore_database],env=secret_env,stdin=source,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=30)
        self.assertEqual(restored.returncode,0,restored.stderr.decode().replace(self.test_password,'[redacted]'))
        original_env=self.env;self.env=dict(self.env,HYJS_MYSQL_DATABASE=restore_database)
        try:
            self.start();login=self.login(uid);self.assertEqual(login.playerId,player_id)
            self.send('CSPlayerLogin',login=login)
            self.assertEqual(self.send('CSPlayer',login=login)[1].player.coin,1000)
            self.assertFalse(self.send('CSQueryUserReward',login=login)[1].rewardItemInfoList)
            self.assertFalse(self.send('CSMailList',{'mailType':'allMail'},login)[1].mailBoxList)
            count=self.root_sql('USE `'+restore_database+'`; SELECT COUNT(*) FROM p1_event_facts WHERE player_id='+str(player_id)+" AND kind='login_day';").strip()
            self.assertEqual(count,'1')
        finally:
            self.stop();self.env=original_env

del LiveServerTests

if __name__=='__main__':unittest.main()
