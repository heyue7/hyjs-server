"""P4 原二进制协议与真实 MySQL；专属库、专属用户和临时端口，结束自动清理。"""
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
from test_go_formation import GoFormationTests

ROOT = Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P4_MYSQL_TEST') == '1', '需要显式启用 P4 专属 MySQL 回归')
class P4MySQLTests(unittest.TestCase):
    send = LiveServerTests.send
    login = LiveServerTests.login
    stamped=GoFormationTests.stamped

    @classmethod
    def setUpClass(cls):
        cls.database = 'hyjs_test_p4_' + secrets.token_hex(5)
        cls.test_user = 'p4_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p4-test-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-p4')
        built = subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=120)
        if built.returncode:
            raise AssertionError(built.stdout + built.stderr)
        cls.root_sql('CREATE DATABASE `' + cls.database + '` CHARACTER SET utf8mb4; CREATE USER ' + "'" + cls.test_user + "'@'%' IDENTIFIED BY '" + cls.test_password + "'; GRANT ALL ON `" + cls.database + "`.* TO '" + cls.test_user + "'@'%';")
        cls.addClassCleanup(lambda: cls.root_sql('DROP DATABASE IF EXISTS `' + cls.database + "`; DROP USER IF EXISTS '" + cls.test_user + "'@'%';"))
        cls.env = dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER=cls.test_user,HYJS_MYSQL_PASSWORD=cls.test_password,HYJS_MYSQL_DATABASE=cls.database,
                       HYJS_P4_MYSQL_HOST='127.0.0.1:33068',HYJS_P4_MYSQL_USER=cls.test_user,HYJS_P4_MYSQL_PASSWORD=cls.test_password,HYJS_P4_MYSQL_DATABASE=cls.database)

    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p4_') or not cls.test_user.startswith('p4_'):
            raise AssertionError('P4 isolation guard')
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
            # 完整P1-P6初始化与其他域隔离测试可能并发，给冷启动保留90秒预算。
            self.assertTrue(ready.select(90),'P4 服务启动超时')
            line=self.process.stdout.readline()
        match=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line)
        self.assertIsNotNone(match,line)
        self.service=types.SimpleNamespace(http_port=int(match[1]),tcp_port=int(match[2]))

    def stop(self):
        if self.process is not None and self.process.poll() is None:
            self.process.terminate()
            try:self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.process.kill();self.process.wait();self.fail('P4 服务未按ctx关闭')
        if self.process is not None and self.process.stdout:self.process.stdout.close()

    def sql(self,text):
        self.assertTrue(self.database.startswith('hyjs_test_p4_'))
        return self.root_sql('USE `' + self.database + '`; ' + text)

    def test_direct_transactions_with_isolated_mysql(self):
        completed=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP4','-count=1','-timeout=150s'],cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=180)
        self.assertEqual(completed.returncode,0,(completed.stdout+completed.stderr).replace(self.test_password,'[redacted]'))
        print(completed.stdout.strip())


    def profile(self,login):
        row=self.sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id='+str(login.playerId)+';').strip().split('|')
        return [json.loads(bytes.fromhex(raw)) for raw in row]

    def fund(self,login):
        p,g=self.profile(login)
        p.update(level=75,diamond=50000,coin=1000000,light=1000,point=1000,popularity=1000,score=1000,noviceGuideSchedule=7)
        g['items']=[{'id':'item_013','num':2},{'id':'item_021','num':10}]
        raw=[json.dumps(v,ensure_ascii=False,separators=(',',':')).encode().hex() for v in [p,g]]
        self.sql("UPDATE profiles SET player=X'"+raw[0]+"',game=X'"+raw[1]+"' WHERE id="+str(login.playerId)+';')

    def call(self,name,fields,login,stamp,expected):
        op,r=self.stamped(name,fields,login,stamp)
        self.assertEqual(op,BY_NAME[expected],str(r));return r

    def deny(self,name,fields,login,stamp):
        before=self.profile(login);op,r=self.stamped(name,fields,login,stamp)
        self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(login),before)

    def test_native_p4_welfare_shops_activity_routes_and_restart(self):
        for table in ['p4_welfare_rules','p4_welfare_states','p4_receipts','p4_commerce_rules','p4_commerce_states','p4_commerce_stocks','p4_recruit_rotations','p4_player_stats','p4_activity_registry','p4_copy_benefit_rules','p4_copy_benefit_grants']:
            # 单独执行协议用例时数据库尚无表；完整套件先清理配置破坏用例的P4数据。
            exists = self.sql("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='"+table+"';").strip()
            if exists == '1':
                self.sql('DELETE FROM '+table+';')
        # 事务用例另建的短竞赛不属于正式月入口，清理后再核验本地投放目录。
        exists = self.sql("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='p1_period_instances';").strip()
        if exists == '1':
            self.sql("DELETE FROM p1_period_instances WHERE id LIKE 'kaiFuJingSai_p4_test_%' OR id LIKE 'kaiFuJingSai_p4_entry_%';")
        self.start();uid='p4-network-'+secrets.token_hex(4);login=self.login(uid)
        self.call('CSPlayerCreate',{'playerName':'P4原协议回归','heroId':'hero_043'},login,1,'SCPlayerCreate')
        query=self.call('CSLoginRewardInfo',{},login,1,'SCLoginRewardInfo')
        self.assertEqual((len(query.items),query.hasReward),(10,0))
        self.deny('CSGetLoginReward',{},login,10)
        self.call('CSPlayerLogin',{},login,2,'SCPlayerLogin')
        query=self.call('CSLoginRewardInfo',{},login,2,'SCLoginRewardInfo');self.assertEqual(query.items[0].state,1)
        first=self.call('CSGetLoginReward',{},login,10,'SCGetLoginReward')
        self.assertEqual(first.get.simps[0].num,1000)
        before=self.profile(login);self.assertEqual(self.call('CSGetLoginReward',{},login,10,'SCGetLoginReward'),first);self.assertEqual(self.profile(login),before)
        limit=self.call('CSRewardLimitSignList',{},login,3,'SCRewardLimitSignList');self.assertEqual(limit.limitDaySignDatas.allNum,7)
        self.call('CSReceiveLimitMonthSign',{},login,1,'SCReceiveLimitMonthSign')
        fresh=self.call('CSReceiveLimitMonthSign',{},login,2,'SCReceiveLimitMonthSign');self.assertFalse(fresh.get.simps)
        meal=self.call('CSStaminaRefill',{},login,3,'SCStaminaRefill');self.assertEqual(len(meal.dataList),2)
        cat=self.call('CSFortuneCatInfo',{},login,3,'SCFortuneCatInfo');self.assertEqual((cat.info.todayLeft,cat.info.totalLeft),(3,15))
        self.call('CSFortuneCatReward',{'type':0},login,1,'SCFortuneCatReward');self.deny('CSFortuneCatReward',{'type':0},login,2)
        self.fund(login)
        products=self.call('CSShopProductList',{'shopType':1},login,4,'SCShopProductList');self.assertEqual(len(products.shopItemList),14)
        trade=self.call('CSShopBuy',{'shopType':1,'shopItemId':'itemShopId_1','buyCount':6},login,1,'SCShopBuy');self.assertEqual(trade.get.simps[0].id,'energy')
        for typ,sku in [(4,'p4Popularity_1'),(5,'p4Point_1'),(7,'p4Score_1')]:
            self.assertEqual(len(self.call('CSShopProductList',{'shopType':typ},login,4,'SCShopProductList').shopItemList),2)
            self.call('CSShopBuy',{'shopType':typ,'shopItemId':sku,'buyCount':1},login,1,'SCShopBuy')
        memory=self.call('CSRefreshShopQuery',{'shopType':3},login,4,'SCRefreshShopQuery');self.assertEqual(len(memory.shopItemList),4)
        self.call('CSRefreshShopRefresh',{'shopType':3},login,1,'SCRefreshShopRefresh')
        refreshed=self.call('CSRefreshShopRefresh',{'shopType':3},login,2,'SCRefreshShopRefresh');self.assertEqual(refreshed.cost[0].id,'item_013')
        self.call('CSRefreshShopBuy',{'shopType':3,'shopItemId':refreshed.shopItemList[0].id},login,3,'SCRefreshShopBuy')
        tabs=self.call('CSActivityList',{},login,4,'SCActivityList');p4tabs=[t for t in tabs.activityList if '_p4_' in t.activityId]
        self.assertEqual(len(p4tabs),11)
        original=['halfDiscountSale','discountSale','limitExchange','limitSale','mysticDiscount']
        generic=['warriorRescruitTimes','warriorGetSpecial','mainCopyWinTimes','kaiFuJingSai']
        for tab in p4tabs:
            prefix=tab.activityId.split('_')[0]
            if prefix in generic:
                info=self.call('CSActivityInfo',{'activityId':tab.activityId},login,4,'SCActivityInfo');self.assertTrue(info.itemList)
            elif prefix in original:
                info=self.call('CSActivityShopInfo',{'activityId':tab.activityId},login,4,'SCActivityShopInfo');self.assertTrue(info.shopItemList)
                if prefix in ['halfDiscountSale','discountSale']:
                    item=info.shopItemList[0]
                    fields={'activityId':tab.activityId,'index':item.index,'num':1,'costResources':[{'id':r.id,'num':r.num} for r in item.costResource],'getResources':[{'id':r.id,'num':r.num} for r in item.getResource]}
                    self.call('CSBuyActivityShop',fields,login,1,'SCGetActivityShopReward')
        info=self.call('CSMysticDiscountInfo',{},login,4,'SCMysticDiscountInfo');draw=self.call('CSMysticDiscount',{'activityId':info.activityId},login,1,'SCMysticDiscount')
        again=self.call('CSMysticDiscount',{'activityId':info.activityId},login,2,'SCMysticDiscount');self.assertEqual(draw.discount,again.discount)
        rebate=next(t for t in p4tabs if t.activityId.startswith('consumeRet_'))
        info=self.call('CSActivityInfo',{'activityId':rebate.activityId},login,4,'SCActivityInfo');self.assertEqual(info.itemList[0].status,1)
        self.call('CSConsumeReturnReward',{'activityId':rebate.activityId},login,1,'SCActivityReward')
        before=self.profile(login);self.call('CSConsumeReturnReward',{'activityId':rebate.activityId},login,2,'SCActivityReward');self.assertEqual(before,self.profile(login))
        recruit=self.call('CSWarriorRecruitGet',{'costType':[1,2]},login,4,'SCWarriorRecruitGet')
        for item in recruit.recruit:
            preview=self.call('CSWarriorRecruitGetPreview',{'costType':item.costType},login,4,'SCWarriorRecruitGetPreview');self.assertEqual(set(item.rewardList),set(preview.rewards));self.assertGreater(item.changeTime,0)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p4_activity_registry;').strip(),'36')
        before=self.profile(login);self.stop();self.start();login=self.login(uid)
        self.assertEqual(before,self.profile(login));self.assertEqual(self.call('CSGetLoginReward',{},login,10,'SCGetLoginReward'),first);self.assertEqual(before,self.profile(login))
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p4_%';").strip(),'InnoDB')

    def test_native_copy_benefits_sweep_gate_receipt_and_restart(self):
        self.start()
        # 仅启用专属测试库的一小时窗口，正式门禁保持关闭。
        now = int(time.time()*1000)
        self.sql('UPDATE p4_copy_benefit_rules SET enabled=0;')
        self.sql("INSERT INTO p4_copy_benefit_rules(id,enabled,start_at,end_at,weekday_mask,start_minute,end_minute,currency_factor,drop_factor,source) VALUES ('p4:copy:network',1,"+str(now-60000)+','+str(now+3600000)+",127,0,1440,2,2,'P4 isolated native settlement');")
        uid = 'p4-copy-'+secrets.token_hex(4)
        login = self.login(uid)
        self.call('CSPlayerCreate', {'playerName':'副本福利原协议','heroId':'hero_043'}, login, 1, 'SCPlayerCreate')
        self.call('CSCopyPassedInfo', {}, login, 1, 'SCCopyPassedInfo')
        p,g = self.profile(login)
        p.update(level=14,exp=0,coin=0,stamina=100,energy=100,noviceGuideSchedule=7)
        for row in g['copy']['passedCopyInfo']:
            if row['chapterId']=='1' and row['missionId']=='1':
                row.update(star=5,challengeTimes=0)
        raw=[json.dumps(v,ensure_ascii=False,separators=(',',':')).encode().hex() for v in [p,g]]
        self.sql("UPDATE profiles SET player=X'"+raw[0]+"',game=X'"+raw[1]+"' WHERE id="+str(login.playerId)+';')
        info = self.call('CSActivityInfo', {'activityId':'allCopyDouble'}, login, 1, 'SCActivityInfo')
        self.assertEqual(info.param,'2')
        tabs = self.call('CSActivityList', {}, login, 1, 'SCActivityList')
        self.assertEqual(len([t for t in tabs.activityList if t.activityId=='allCopyDouble']),1)
        request={'chapterId':'1','missionId':'1'}
        first=self.call('CSCopySweep',request,login,100,'SCCopySweep')
        self.assertEqual({r.id:r.num for r in first.gain.simps},{'coin':4380,'exp':280})
        p,g=self.profile(login)
        self.assertEqual((p['coin'],p['level'],p['exp'],p['stamina']),(4380,15,0,110))
        self.assertEqual(self.call('CSCopySweep',request,login,100,'SCCopySweep'),first)
        self.stop();self.start();login=self.login(uid)
        self.assertEqual(self.call('CSCopySweep',request,login,100,'SCCopySweep'),first)
        self.assertEqual(self.profile(login)[0]['coin'],4380)
        # 新角色保留未首通主线，走原入场/战斗/结算协议检查额外Hero真实GUID。
        fresh=self.login('p4-copy-drop-'+secrets.token_hex(4))
        self.call('CSPlayerCreate',{'playerName':'福利首掉实例','heroId':'hero_043'},fresh,1,'SCPlayerCreate')
        original=self.profile(fresh)[0]
        original['level']=14
        blob=json.dumps(original,ensure_ascii=False,separators=(',',':')).encode().hex()
        self.sql("UPDATE profiles SET player=X'"+blob+"' WHERE id="+str(fresh.playerId)+';')
        self.call('CSCopyEnter',{'chapterId':'1','missionId':'1'},fresh,1,'SCCopyEnter')
        final=None
        for task in ['1','4']:
            fight=self.call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':task},fresh,1,'SCCopyFightStart')
            finish={'fightId':fight.fightId,'result':1,'battlePlayer':fight.battlePlayer}
            result=self.call('CSCopyFightEnd',finish,fresh,1,'SCCopyFightEnd')
            self.assertEqual(self.call('CSCopyFightEnd',finish,fresh,1,'SCCopyFightEnd'),result)
            if task=='1':
                self.assertEqual([w.id for w in result.gain.wars],['hero_exp_006','hero_exp_006'])
                self.assertEqual(len(set(w.guid for w in result.gain.wars)),2)
                self.assertEqual(len(result.currentResource),2)
            final=result
        self.assertEqual(final.isCopyComplete,1)
        self.assertEqual({r.id:r.num for r in final.gain.simps},{'coin':4380,'exp':280})
        self.assertEqual(len([w for w in self.profile(fresh)[1]['warriors'] if w['id']=='hero_exp_006']),2)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p4_copy_benefit_grants WHERE player_id='+str(fresh.playerId)+';').strip(),'1')
        self.sql("UPDATE p4_copy_benefit_rules SET enabled=0 WHERE id='p4:copy:network';")
        second=self.call('CSCopySweep',request,login,101,'SCCopySweep')
        self.assertEqual({r.id:r.num for r in second.gain.simps},{'coin':2300,'exp':150})
        self.assertEqual(self.profile(login)[0]['coin'],6680)
        tabs=self.call('CSActivityList',{},login,1,'SCActivityList')
        self.assertFalse([t for t in tabs.activityList if t.activityId=='allCopyDouble'])
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p4_copy_benefit_%';").strip(),'InnoDB')

del LiveServerTests
del GoFormationTests

if __name__=='__main__':unittest.main()
