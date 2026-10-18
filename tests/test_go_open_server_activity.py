"""原249任务14天开服活动：原二进制协议、专属SQL、重启与库存并发。"""
import concurrent.futures
import datetime
import json
import os
import unittest
from server.codec import BY_NAME, message
from test_go_equipment import GoEquipmentTests
from test_go_formation import GoFormationTests
from test_go_activity import MySQLActivityTests

class GoOpenServerActivityTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    stamped = GoFormationTests.stamped
    login = GoEquipmentTests.login
    mutate = GoEquipmentTests.mutate

    def sql(self, query, args=()):
        with self.service.store.connect() as c:
            return c.execute(query,args).fetchall()

    def profile(self, login):
        r=self.sql('SELECT player,game FROM profiles WHERE id=?',(login.playerId,))[0]
        return json.loads(r['player']),json.loads(r['game'])

    def period(self, day):
        zone=datetime.timezone(datetime.timedelta(hours=8))
        today=datetime.datetime.now(zone).replace(hour=0,minute=0,second=0,microsecond=0)
        start=int((today-datetime.timedelta(days=day-1)).timestamp()*1000)
        self.sql('UPDATE open_server_activity_rules SET start_at=?,end_at=?,reward_end_at=?',(start,start+14*86400000,start+15*86400000))

    def fixture(self, uid, day=1):
        login=self.login(uid)
        self.send('CSPlayerCreate',{'playerName':uid,'heroId':'hero_043'},login)
        self.mutate(login,lambda p,g:p.update(diamond=10000,coin=0,vip=0))
        self.period(day)
        return login

    def tab(self, login, day=1, tap=1):
        op,r=self.send('CSNewOpenSevenDayByTitle',{'day':day,'tap':tap},login)
        self.assertEqual(op,BY_NAME['SCNewOpenSevenDayByTitle'],str(r))
        return r

    def claim(self, login, item, stamp, choice=None):
        fields={'itemId':item}
        if choice is not None:fields['heroId']=choice
        return self.stamped('CSGetOpenServerReward',fields,login,stamp)

    def test_login_real_events_red_taps_claim_replay_restart_and_unimplemented(self):
        login=self.fixture('open-login')
        self.assertEqual(len(self.sql('SELECT id FROM open_server_activity_tasks')),249)
        counters=[dict(r) for r in self.sql('SELECT * FROM open_server_activity_counters WHERE player_id=?',(login.playerId,))]
        for _ in range(3):
            r=self.tab(login)
            self.assertEqual(r.itemList[0].state,0)
        self.assertEqual([dict(r) for r in self.sql('SELECT * FROM open_server_activity_counters WHERE player_id=?',(login.playerId,))],counters)
        self.send('CSPlayerLogin',login=login)
        r=self.tab(login)
        self.assertEqual((r.itemList[0].state,r.itemList[0].currentNum),(1,1))
        self.assertIn(1,r.redTap)
        self.assertEqual(r.itemList[1].state,0) # 真实支付尚未接入。
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies=list(pool.map(lambda n:(n,self.claim(login,1,100+n)),range(6)))
        wins=[(n,r) for n,(op,r) in replies if op==BY_NAME['SCGetOpenServerReward']]
        self.assertEqual(len(wins),1)
        winner,original=wins[0]
        self.assertEqual(self.profile(login)[0]['diamond'],10100)
        op,replay=self.claim(login,1,100+winner)
        self.assertEqual((op,replay),(BY_NAME['SCGetOpenServerReward'],original))
        self.assertEqual(self.claim(login,2,100+winner)[0],10001)
        self.stop();self.start();login=self.login('open-login')
        self.assertEqual(self.claim(login,1,100+winner)[1],original)
        self.assertEqual(self.profile(login)[0]['diamond'],10100)
        self.assertEqual(self.tab(login).itemList[0].state,2)
        self.sql("UPDATE open_server_activity_tasks SET reward='coin,999' WHERE id=1")
        self.assertEqual(self.tab(login).itemList[0].state,2)
        self.assertEqual(self.claim(login,1,100+winner)[1],original)
        self.assertEqual(self.claim(login,1,199)[0],10001)
        self.assertEqual(self.send('CSOpenServerActivity',{'dayType':1},login)[0],10001)
        self.assertEqual(self.send('CSNewOpenSevenDayByTitle',{'day':2,'tap':1},login)[0],10001)

    def test_shared_stock_choice_hero_experience_capacity_and_overflow(self):
        a=self.fixture('stock-a');b=self.fixture('stock-b')
        self.sql("UPDATE open_server_activity_tasks SET used_stock=target-1 WHERE id=20")
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            replies=list(pool.map(lambda login:(login,self.claim(login,20,200)),[a,b]))
        winners=[(login,r) for login,(op,r) in replies if op==BY_NAME['SCGetOpenServerReward']]
        self.assertEqual(len(winners),1)
        winner,result=winners[0]
        self.assertIn(('item_055',1),[(v.id,v.num) for v in result.get.simps])
        self.assertIn(('diamond',-400),[(v.id,v.num) for v in result.get.simps])
        self.assertEqual(self.profile(winner)[0]['diamond'],9600)
        self.assertEqual(int(self.sql('SELECT used_stock FROM open_server_activity_tasks WHERE id=20')[0]['used_stock']),5000)
        self.assertEqual(self.claim(winner,20,201)[0],10001)
        self.period(8)
        self.send('CSPlayerLogin',login=a)
        # day8登录碎片三选一，传入的heroId承载原资源ID。
        self.assertEqual(self.claim(a,137,300,'hero_043')[0],10001)
        op,r=self.claim(a,137,301,'frag_hero_001')
        self.assertEqual(op,BY_NAME['SCGetOpenServerReward'],str(r))
        self.assertEqual([(i.id,i.num) for i in r.get.simps],[('frag_hero_001',15)])
        # 原限购10次，原选饰品不允许任意客户端奖励。
        self.mutate(a,lambda p,g:p.update(diamond=30000))
        self.assertEqual(self.claim(a,147,302,'trea_005')[0],10001)
        for stamp in range(303,313):
            self.assertEqual(self.claim(a,147,stamp,'trea_011')[0],BY_NAME['SCGetOpenServerReward'])
        self.assertEqual(self.claim(a,147,313,'trea_011')[0],10001)
        self.period(10)
        before=len(self.profile(b)[1].get('warriors') or [])
        op,r=self.claim(b,184,400)
        self.assertEqual(op,BY_NAME['SCGetOpenServerReward'],str(r))
        self.assertEqual(len(r.get.wars),5)
        self.assertTrue(all(w.id=='hero_exp_004' and w.guid for w in r.get.wars))
        self.assertEqual(len(self.profile(b)[1]['warriors']),before+5)
        self.mutate(b,lambda p,g:p.update(equipBagSize=len(g.get('equipment') or [])+len(g.get('treasures') or [])))
        saved=self.profile(b)
        self.assertEqual(self.claim(b,151,401)[0],10001)
        self.assertEqual(self.profile(b),saved)
        self.mutate(b,lambda p,g:p.update(light=2**31-1))
        saved=self.profile(b)
        self.assertEqual(self.claim(b,185,402)[0],10001)
        self.assertEqual(self.profile(b),saved)
        self.assertEqual(int(self.sql('SELECT used_stock FROM open_server_activity_tasks WHERE id=185')[0]['used_stock']),0)



    def test_real_compose_and_paid_shop_events_increment_once(self):
        login=self.fixture('open-events',day=2)
        def seed(player,game):
            player.update(level=20)
            game['items']=[{'id':f'frag_trea_006_{i}','num':10} for i in range(1,5)]
        self.mutate(login,seed)
        for stamp in range(1000,1010):
            op,r=self.stamped('CSCombine',{'targetId':'trea_006'},login,stamp)
            self.assertEqual(op,BY_NAME['SCCombine'],str(r))
            if stamp==1000:
                first_count=int(self.sql("SELECT value FROM open_server_activity_counters WHERE player_id=? AND counter_key='compose_treasure'",(login.playerId,))[0]['value'])
                self.assertEqual(first_count,1)
                self.assertEqual(self.stamped('CSCombine',{'targetId':'trea_006'},login,stamp),(op,r))
                first_count=int(self.sql("SELECT value FROM open_server_activity_counters WHERE player_id=? AND counter_key='compose_treasure'",(login.playerId,))[0]['value'])
                self.assertEqual(first_count,1)
        op,replay=self.stamped('CSCombine',{'targetId':'trea_006'},login,1009)
        self.assertEqual(op,BY_NAME['SCCombine'],str(replay))
        counters={r['counter_key']:int(r['value']) for r in self.sql('SELECT counter_key,value FROM open_server_activity_counters WHERE player_id=?',(login.playerId,))}
        self.assertEqual(counters['compose_treasure'],10)
        self.assertEqual(counters['compose_treasure_purple'],10)
        tasks={i.itemId:i for i in self.tab(login,2,3).itemList}
        self.assertEqual((tasks[31].state,tasks[31].currentNum,tasks[34].state),(1,10,1))
        for _ in range(3):self.tab(login,2,3)
        current={r['counter_key']:int(r['value']) for r in self.sql('SELECT counter_key,value FROM open_server_activity_counters WHERE player_id=?',(login.playerId,))}
        self.assertEqual(current,counters)
        # 材料已经消耗完毕，非法重组不能生成实例或增加计数。
        self.assertEqual(self.stamped('CSCombine',{'targetId':'trea_006'},login,1010)[0],10001)
        failed={r['counter_key']:int(r['value']) for r in self.sql('SELECT counter_key,value FROM open_server_activity_counters WHERE player_id=?',(login.playerId,))}
        self.assertEqual(failed,counters)
        self.mutate(login,lambda p,g:g['items'].extend([{'id':f'frag_trea_011_{i}','num':1} for i in range(1,6)]))
        orange=self.stamped('CSCombine',{'targetId':'trea_011'},login,1011)
        self.assertEqual(orange[0],BY_NAME['SCCombine'],str(orange[1]))
        self.assertEqual(self.stamped('CSCombine',{'targetId':'trea_011'},login,1011),orange)
        colors={r['counter_key']:int(r['value']) for r in self.sql('SELECT counter_key,value FROM open_server_activity_counters WHERE player_id=?',(login.playerId,))}
        self.assertEqual((colors['compose_treasure'],colors['compose_treasure_purple'],colors['compose_treasure_orange']),(11,10,1))
        # 商店本地目录通过协议下发资源，固定价格保持原APK值。
        self.period(4)
        self.sql("UPDATE shop_products SET resource_id='item_004',amount=3 WHERE id='itemShopId_13'")
        fields={'shopType':1,'shopItemId':'itemShopId_13','buyCount':4}
        first=self.stamped('CSShopBuy',fields,login,2000)
        self.assertEqual(first[0],BY_NAME['SCShopBuy'],str(first[1]))
        self.assertEqual(self.stamped('CSShopBuy',fields,login,2000),first)
        count=int(self.sql("SELECT value FROM open_server_activity_counters WHERE player_id=? AND counter_key='buy_attack_token'",(login.playerId,))[0]['value'])
        self.assertEqual(count,12)
        task=next(i for i in self.tab(login,4,2).itemList if i.itemId==57)
        self.assertEqual((task.currentNum,task.state),(12,1))

    def test_copy_completed_mission_records_one_daily_event_not_each_fight(self):
        login=self.fixture('open-copy-events',day=10)
        self.mutate(login,lambda p,g:p.update(level=20,energy=500))
        before=self.sql("SELECT value FROM open_server_activity_counters WHERE player_id=? AND counter_key LIKE 'copy_win:%'",(login.playerId,))
        self.assertEqual(len(before),0)
        op,entry=self.send('CSCopyEnter',{'chapterId':'1','missionId':'1'},login)
        self.assertEqual(op,BY_NAME['SCCopyEnter'],str(entry))
        for task in ('1','4'):
            op,fight=self.send('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':task},login)
            self.assertEqual(op,BY_NAME['SCCopyFightStart'],str(fight))
            # 此处提供原协议允许的战斗结果输入，仅验证事务事件；不作为权威战斗重演证据。
            invalid=message('BattlePlayer');invalid.CopyFrom(fight.battlePlayer)
            invalid.formation[0].inst.curHp=float('nan')
            bad={'fightId':fight.fightId,'result':1,'battlePlayer':invalid}
            self.assertEqual(self.send('CSCopyFightEnd',bad,login)[0],10001)
            count=self.sql("SELECT value FROM open_server_activity_counters WHERE player_id=? AND counter_key LIKE 'copy_win:%'",(login.playerId,))
            self.assertEqual(len(count),0)
            fields={'fightId':fight.fightId,'result':1,'battlePlayer':fight.battlePlayer}
            op,end=self.send('CSCopyFightEnd',fields,login)
            self.assertEqual(op,BY_NAME['SCCopyFightEnd'],str(end))
            self.assertEqual(end.isCopyComplete,int(task=='4'))
            op,replay=self.send('CSCopyFightEnd',fields,login)
            self.assertEqual((op,replay),(BY_NAME['SCCopyFightEnd'],end))
            count=self.sql("SELECT counter_key,value FROM open_server_activity_counters WHERE player_id=? AND counter_key LIKE 'copy_win:%'",(login.playerId,))
            if task=='1':self.assertEqual(len(count),0)
            else:
                self.assertEqual(len(count),1)
                self.assertEqual(int(count[0]['value']),1)
                expected=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).strftime('%Y-%m-%d')
                self.assertEqual(count[0]['counter_key'],'copy_win:'+expected)
        task=next(i for i in self.tab(login,10,2).itemList if i.itemId==169)
        self.assertEqual((task.currentNum,task.state),(1,0))

    def test_original_open_state_entries_follow_server_window_only(self):
        login=self.fixture('open-state')
        for day,expected in [(0,(0,0)),(1,(1,0)),(7,(1,0)),(8,(0,1)),(15,(0,1)),(16,(0,0))]:
            self.period(day)
            op,r=self.send('CSOpenStateCheck',login=login)
            self.assertEqual(op,BY_NAME['SCOpenStateCheck'],str(r))
            self.assertEqual({v.type:v.isOpen for v in r.states},{'ActivityNewOpen7Day':expected[0],'ActivityNewOpen14Day':expected[1]})
        self.period(1)
        self.sql('UPDATE open_server_activity_rules SET enabled=0')
        op,r=self.send('CSOpenStateCheck',login=login)
        self.assertEqual(op,BY_NAME['SCOpenStateCheck'],str(r))
        self.assertTrue(all(v.isOpen==0 for v in r.states))

    def test_sql_reward_and_price_drift_rejected_without_charge(self):
        login=self.fixture('open-sql-drift')
        before=self.profile(login)
        self.sql('UPDATE open_server_activity_tasks SET new_price=new_price+1 WHERE id=20')
        self.assertEqual(self.claim(login,20,500)[0],10001)
        self.assertEqual(self.profile(login),before)
        self.assertEqual(next(i for i in self.tab(login,1,4).itemList if i.itemId==20).state,0)
        self.sql("UPDATE open_server_activity_tasks SET new_price=400,reward='diamond,1000' WHERE id=20")
        self.assertEqual(self.claim(login,20,501)[0],10001)
        self.assertEqual(self.profile(login),before)
        self.assertEqual(len(self.sql('SELECT timestamp FROM open_server_activity_receipts WHERE player_id=?',(login.playerId,))),0)

@unittest.skipUnless(os.environ.get('HYJS_MYSQL_ACTIVITY_TEST')=='1','需显式启用隔离MySQL开服活动回归')
class MySQLOpenServerActivityTests(GoOpenServerActivityTests):
    setUp=MySQLActivityTests.setUp
    tearDown=MySQLActivityTests.tearDown
    start=MySQLActivityTests.start
    root_sql=MySQLActivityTests.root_sql
    sql=MySQLActivityTests.sql
    mutate=MySQLActivityTests.mutate

    def test_mysql_receipt_columns_are_binary_and_tables_innodb(self):
        rows=self.sql("SELECT COLUMN_NAME,DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME='open_server_activity_receipts' AND COLUMN_NAME IN ('request','response')",(self.database,))
        self.assertEqual({r['COLUMN_NAME']:r['DATA_TYPE'] for r in rows},{'request':'longblob','response':'longblob'})
        rows=self.sql("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME LIKE 'open_server_activity_%'",(self.database,))
        self.assertEqual(len(rows),5)
        self.assertTrue(all(r['ENGINE']=='InnoDB' for r in rows))

del GoEquipmentTests,GoFormationTests,MySQLActivityTests
