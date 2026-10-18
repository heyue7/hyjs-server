"""P2 原协议真实 Go/MySQL 回归。只写 hyjs_test_p2_*，退出撤权删库。"""
import concurrent.futures
import csv
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import uuid

from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests
from test_go_formation import GoFormationTests
from test_go_warrior_growth import MySQLWarriorGrowthTests
from test_mysql_equipment import MySQLEquipmentHarness


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_P2_TEST') == '1', 'isolated P2 MySQL regression disabled')
class P2MySQLTests(unittest.TestCase):
    root_sql = MySQLWarriorGrowthTests.root_sql
    start = MySQLWarriorGrowthTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    stamped = GoFormationTests.stamped
    sql = MySQLEquipmentHarness.sql
    mutate = MySQLWarriorGrowthTests.mutate

    @classmethod
    def setUpClass(cls):
        cls.repo = Path(__file__).resolve().parents[1]
        cls.temporary = tempfile.TemporaryDirectory(prefix='hyjs-p2-binary-')
        cls.binary = str(Path(cls.temporary.name)/'server')
        result = subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=cls.repo/'hyjs-server',capture_output=True,text=True,timeout=120)
        if result.returncode:
            cls.temporary.cleanup()
            raise AssertionError(result.stderr)
        cls.old_binary = os.environ.get('HYJS_GO_TEST_BINARY')
        os.environ['HYJS_GO_TEST_BINARY'] = cls.binary

    @classmethod
    def tearDownClass(cls):
        if cls.old_binary is None: os.environ.pop('HYJS_GO_TEST_BINARY',None)
        else: os.environ['HYJS_GO_TEST_BINARY']=cls.old_binary
        cls.temporary.cleanup()

    def setUp(self):
        self.database='hyjs_test_p2_'+str(os.getpid())+'_'+uuid.uuid4().hex[:8]
        env=dict(os.environ)
        for line in (self.repo/'.local-env/mysql.env').read_text().splitlines():
            if line and not line.startswith('#') and '=' in line:
                key,value=line.split('=',1);env[key]=value.strip().strip('"').strip("'")
        self.env=dict(env,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER='hyjs',HYJS_MYSQL_DATABASE=self.database)
        self.addCleanup(self.cleanup_database)
        self.root_sql('CREATE DATABASE `'+self.database+'` CHARACTER SET utf8mb4; GRANT ALL ON `'+self.database+"`.* TO 'hyjs'@'%';")
        self.addCleanup(self.stop)
        self.start()

    def cleanup_database(self):
        self.assertTrue(self.database.startswith('hyjs_test_p2_'))
        self.root_sql('REVOKE ALL PRIVILEGES ON `'+self.database+"`.* FROM 'hyjs'@'%'; DROP DATABASE IF EXISTS `"+self.database+'`;')
        output=self.root_sql("SELECT COUNT(*) AS remaining FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='"+self.database+"'; SELECT COUNT(*) AS remaining FROM mysql.db WHERE Db='"+self.database+"';")
        self.assertEqual(output.splitlines(),['remaining','0','remaining','0'])

    def sql(self,text,args=()):
        self.assertTrue(self.database.startswith('hyjs_test_p2_'))
        parts=text.split('?');self.assertEqual(len(parts),len(args)+1);rendered=parts[0]
        for value,tail in zip(args,parts[1:]):
            rendered += str(value) if isinstance(value,(int,float)) else 'NULL' if value is None else "CONVERT(X'"+str(value).encode().hex()+"' USING utf8mb4)"
            rendered += tail
        lines=self.root_sql(rendered+';',self.database).splitlines()
        if not lines:return []
        return [dict(zip(lines[0].split('\t'),line.split('\t'))) for line in lines[1:]]

    def profile(self,login):
        row=self.sql('SELECT player,game FROM profiles WHERE id=?',(login.playerId,))[0]
        return json.loads(row['player']),json.loads(row['game'])

    def fixture(self,uid='p2',level=20):
        login=self.login(uid)
        self.send('CSPlayerCreate',{'playerName':'红色回归','heroId':'hero_047'},login)
        def prepare(p,g):
            p.update(level=level,diamond=20000,coin=1000000,stamina=200,noviceGuideSchedule=7)
            g['items']=[{'id':key,'num':n} for key,n in [('item_023',2),('item_119',3),('item_020',800),('item_021',300),('item_122',1),('item_361',2)]]
        self.mutate(login,prepare)
        return login

    def call(self,name,fields,login,stamp,expected):
        opcode,response=self.stamped(name,fields,login,stamp)
        self.assertEqual(opcode,BY_NAME[expected],str(response))
        return response

    def box(self,login,key,index,stamp,num=1):
        return self.call('CSItemUse',{'costItem':{'id':key,'num':num},'getItemIndex':index},login,stamp,'SCItemUse')

    def deny(self,name,fields,login,stamp):
        before=self.profile(login)
        opcode,response=self.stamped(name,fields,login,stamp)
        self.assertEqual((opcode,response.code),(10001,'90003'),str(response))
        self.assertEqual(self.profile(login),before)

    def test_king_actual_protocol_preview_single_ten_free_luck_restart(self):
        login=self.fixture()
        info=self.call('CSWarriorRecruitGet',{'costType':[1,2,3]},login,1,'SCWarriorRecruitGet')
        king=next(r for r in info.recruit if r.costType==3)
        self.assertEqual((king.leftTimes,king.luck,king.on),(50,0,1))
        self.assertTrue(king.rewardList)
        preview=self.call('CSWarriorRecruitGetPreview',{'costType':3},login,2,'SCWarriorRecruitGetPreview')
        self.assertTrue(set(king.rewardList)<=set(preview.rewards))
        before=self.profile(login)[0]
        request={'costType':3,'numType':1,'free':0}
        single=self.call('CSWarriorRecruit',request,login,100,'SCWarriorRecruit')
        self.assertEqual([(c.id,c.num) for c in single.cost],[('diamond',300)])
        self.assertEqual(self.call('CSWarriorRecruit',request,login,100,'SCWarriorRecruit'),single)
        self.deny('CSWarriorRecruit',dict(request,numType=2),login,100)
        ten=self.call('CSWarriorRecruit',dict(request,numType=2),login,101,'SCWarriorRecruit')
        self.assertEqual(len(ten.get.wars),10)
        self.assertEqual(ten.cost[0].num,2800)
        self.call('CSWarriorRecruit',dict(request,free=1),login,102,'SCWarriorRecruit')
        self.deny('CSWarriorRecruit',dict(request,free=1),login,103)
        self.assertEqual(self.profile(login)[0]['diamond'],int(before['diamond'])-3100)
        state=self.sql('SELECT draws,luck FROM p2_king_states WHERE player_id=?',(login.playerId,))[0]
        self.assertEqual((int(state['draws']),int(state['luck'])),(12,120))
        self.sql('UPDATE p2_king_states SET luck=500 WHERE player_id=?',(login.playerId,))
        reward={'reward':king.rewardList[0]}
        lucky=self.call('CSWarriorRecruitLuckyReward',reward,login,104,'SCWarriorRecruitLuckyReward')
        self.assertEqual(lucky.get.wars[0].id,reward['reward'])
        self.stop();self.start();login=self.login('p2')
        self.assertEqual(self.call('CSWarriorRecruitLuckyReward',reward,login,104,'SCWarriorRecruitLuckyReward'),lucky)
        self.deny('CSWarriorRecruitLuckyReward',reward,login,105)
        self.sql('UPDATE p2_king_states SET draws=49 WHERE player_id=?',(login.playerId,))
        self.deny('CSWarriorRecruit',dict(request,numType=2),login,106)
        self.call('CSWarriorRecruit',request,login,107,'SCWarriorRecruit')
        self.deny('CSWarriorRecruit',request,login,108)

    def test_boxes_fragments_selection_instances_growth_and_battle(self):
        login=self.fixture(level=30)
        self.deny('CSItemUse',{'costItem':{'id':'item_023','num':1},'getItemIndex':0},login,1)
        self.assertEqual(self.box(login,'item_023',5,2).get.simps[0].id,'item_053')
        hero=self.box(login,'item_053',1,3).get.wars[0]
        gear=self.box(login,'item_119',1,4).get.equips[0]
        self.assertEqual((hero.id,gear.equipId),('hero_001','equip_037'))
        self.assertEqual(len(self.box(login,'item_361',0,5,num=2).get.wars),2)
        self.call('CSFormationOn',{'id':hero.id,'guid':hero.guid,'formatPos':2},login,6,'SCFormationOn')
        self.call('CSFormationOn',{'id':gear.equipId,'guid':gear.guid,'formatPos':2},login,7,'SCFormationOn')
        self.call('CSCopyEnter',{'chapterId':'1','missionId':'1'},login,8,'SCCopyEnter')
        battle=self.call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'1'},login,9,'SCCopyFightStart')
        actor=next(h for h in battle.battlePlayer.formation if h.guid==hero.guid)
        self.assertGreater(actor.prop.att,0)
        self.assertGreater(actor.prop.hp,0)
        self.box(login,'item_122',1,10)
        frags=self.call('CSWarriorFragGet',{},login,11,'SCWarriorFragGet')
        fragment=next(f for f in frags.warriorFrag if f.id=='frag_hero_001')
        target=next(r for r in self.table('Frag') if r['target']=='hero_001')
        def add(p,g):
            next(i for i in g['items'] if i['id']==fragment.id)['num']=int(target['num'])+2
        self.mutate(login,add)
        combined=self.call('CSCombine',{'targetId':'hero_001'},login,12,'SCCombine')
        self.assertEqual(combined.get.wars[0].id,'hero_001')
        self.assertNotEqual(combined.get.wars[0].guid,hero.guid)
        before_player, before_game = self.profile(login)
        before=int(before_player['coin'])
        before_soul=sum(i['num'] for i in before_game['items'] if i['id']=='item_033')
        sold=self.call('CSWarriorFragSell',{'sell':[{'id':fragment.id,'num':2}]},login,13,'SCWarriorFragSell')
        after_player, after_game = self.profile(login)
        self.assertEqual(int(after_player['coin']),before)
        self.assertEqual(next(i['num'] for i in after_game['items'] if i['id']=='item_033'),before_soul+2*int(target['price']))
        self.assertEqual(next(i.num for i in sold.get.simps if i.id=='item_033'),2*int(target['price']))
        self.assertEqual(self.call('CSWarriorFragSell',{'sell':[{'id':fragment.id,'num':2}]},login,13,'SCWarriorFragSell'),sold)

    def table(self,name):
        with (self.repo/'analysis/tables'/('Table'+name+'.tsv')).open() as source:rows=list(csv.reader(source,delimiter='\t'))
        return [dict(zip(rows[1],r)) for r in rows[5:]]

    def test_exchange_native_tab_status_price_quota_and_replay(self):
        login=self.fixture()
        tabs=self.call('CSActivityList',{},login,1,'SCActivityList')
        self.assertIn('limitExchange_p2_red',[t.activityId for t in tabs.activityList])
        info=self.call('CSActivityShopInfo',{'activityId':'limitExchange_p2_red'},login,2,'SCActivityShopInfo')
        self.assertGreater(info.endTime,info.startTime)
        item=next(i for i in info.shopItemList if i.index==3)
        self.assertEqual((item.status,item.left),(1,1))
        request={'activityId':info.activityId,'index':3,'costResources':[{'id':'item_021','num':99}]}
        self.deny('CSGetActivityShopReward',request,login,3)
        request['costResources'][0]['num']=100
        bought=self.call('CSGetActivityShopReward',request,login,4,'SCGetActivityShopReward')
        self.assertEqual(bought.get.equips[0].equipId,'equip_037')
        done=next(i for i in bought.shopItemList if i.index==3)
        self.assertEqual((done.status,done.left),(2,0))
        self.assertEqual(self.call('CSGetActivityShopReward',request,login,4,'SCGetActivityShopReward'),bought)
        self.deny('CSGetActivityShopReward',request,login,5)
        self.assertEqual(int(self.sql("SELECT sum(amount) AS n FROM p1_event_facts WHERE player_id=? AND kind='spend:item_021'",(login.playerId,))[0]['n']),100)

    def test_red_wheel_real_instance_shared_stock_and_replay(self):
        login=self.fixture()
        self.sql("UPDATE gameplay_pools SET jackpot_per_million=0 WHERE id='red_turntable_20261013'")
        self.sql("DELETE FROM gameplay_pool_rewards WHERE pool_id='red_turntable_20261013'")
        for index,key in enumerate(['equip_037','equip_041']):
            self.sql("INSERT INTO gameplay_pool_rewards(pool_id,id,resource_id,amount,weight,guaranteed,stock) VALUES('red_turntable_20261013',?,?,1,1,0,1)",(index,key))
        # 原UI九奖位要求，其他七位有限库存已耗尽，仍保留完整图标列表。
        for index in range(2,9):
            self.sql("INSERT INTO gameplay_pool_rewards(pool_id,id,resource_id,amount,weight,guaranteed,stock) VALUES('red_turntable_20261013',?,'item_005',1,1,0,1)",(index,))
        self.sql("UPDATE turntable_globals SET used_stock=? WHERE pool_id='red_turntable_20261013'",(json.dumps({str(i):1 for i in range(2,9)}),))
        request={'count':1}
        first=self.call('CSPlayBigTurn',request,login,100,'SCPlayBigTurn')
        second=self.call('CSPlayBigTurn',request,login,101,'SCPlayBigTurn')
        self.assertNotEqual(first.rewardItemId,second.rewardItemId)
        gear=self.profile(login)[1]['equipment']
        self.assertEqual({g['equipId'] for g in gear},{'equip_037','equip_041'})
        self.assertEqual(len({g['guid'] for g in gear}),2)
        before=self.profile(login)
        self.assertEqual(self.call('CSPlayBigTurn',request,login,101,'SCPlayBigTurn'),second)
        self.assertEqual(self.profile(login),before)
        self.deny('CSPlayBigTurn',request,login,102)
        self.assertEqual(int(self.sql("SELECT count(*) AS n FROM turntable_records WHERE pool_id='red_turntable_20261013'")[0]['n']),2)
        self.assertEqual(int(self.sql("SELECT sum(amount) AS n FROM p1_event_facts WHERE player_id=? AND kind='diamond_spent'",(login.playerId,))[0]['n']),50)

    def test_auditable_sql_seed_innodb_preserves_existing_rules(self):
        self.sql("UPDATE gameplay_pool_rewards SET weight=123 WHERE pool_id='recruit_3'")
        for path in sorted((self.repo/'hyjs-server/db/sql').glob('81_p2_*.sql')):
            if not path.name.endswith('.seed.sql'): self.root_sql(path.read_text(),self.database)
        self.root_sql((self.repo/'hyjs-server/db/sql/81_p2_king_rules.seed.sql').read_text(),self.database)
        weights=self.sql("SELECT distinct weight FROM gameplay_pool_rewards WHERE pool_id='recruit_3'")
        self.assertEqual(weights,[{'weight':'123'}])
        engines=self.sql("SELECT TABLE_NAME,ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME LIKE 'p2_%'",(self.database,))
        self.assertEqual(len(engines),6)
        self.assertEqual({r['ENGINE'] for r in engines},{'InnoDB'})

    def test_mysql_rowlocks_concurrent_box_global_exchange_and_durable_restart(self):
        self.stop()
        env=dict(self.env,HYJS_MYSQL_TEST_DATABASE=self.database)
        result=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestMySQLP2Transactions$','-count=1'],cwd=self.repo/'hyjs-server',env=env,text=True,capture_output=True,timeout=120)
        self.assertEqual(result.returncode,0,result.stdout+result.stderr)


del MySQLWarriorGrowthTests,GoEquipmentTests,GoFormationTests,MySQLEquipmentHarness
