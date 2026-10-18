"""饰品箱合法获得链：原二进制协议、真实Go、隔离临时库存与运营SQL。"""
import csv
from pathlib import Path
import unittest
from test_go_treasure import GoTreasureTests
from server.codec import BY_NAME

class GoTreasureSourceTests(unittest.TestCase):
    setUp=GoTreasureTests.setUp
    tearDown=GoTreasureTests.tearDown
    stop=GoTreasureTests.stop
    start=GoTreasureTests.start
    send=GoTreasureTests.send
    login=GoTreasureTests.login
    character=GoTreasureTests.character
    mutate=GoTreasureTests.mutate
    denied=GoTreasureTests.denied
    treasures=GoTreasureTests.treasures
    formation=GoTreasureTests.formation
    battle=GoTreasureTests.battle

    def seed(self,login,key,num):
        self.mutate(login,lambda p,g:g.update(items=[{'id':key,'num':num}],treasures=[]))

    def use(self,login,key,num=1,index=0):
        return self.send('CSItemUse',{'costItem':{'id':key,'num':num},'getItemIndex':index},login)

    def test_whole_treasure_batch_unique_guid_restart_and_replay(self):
        login=self.character();self.seed(login,'item_025',8)
        self.fixed_stamp=1800000000020
        op,r=self.use(login,'item_025',3);self.assertEqual(op,BY_NAME['SCItemUse'],str(r))
        self.assertEqual((r.cost.id,r.cost.num),('item_025',3));self.assertEqual(len(r.get.treas),3)
        self.assertEqual(len({t.guid for t in r.get.treas}),3)
        self.assertTrue(all(t.treasureId in ['trea_011','trea_012','trea_013','trea_014'] and t.strengthLevel==1 and t.refineLevel==0 for t in r.get.treas))
        _,replay=self.use(login,'item_025',3);self.assertEqual(replay,r)
        self.denied(login,'CSItemUse',{'costItem':{'id':'item_025','num':1}})
        self.fixed_stamp-=1;self.denied(login,'CSItemUse',{'costItem':{'id':'item_025','num':3}})
        del self.fixed_stamp
        self.assertEqual(len(self.treasures(login)),3)
        _,items=self.send('CSItemList',login=login);self.assertEqual(next(i.num for i in items.itemList if i.id=='item_025'),5)
        saved=self.treasures(login);self.stop();self.start();login=self.login('equipment-test');self.assertEqual(self.treasures(login),saved)

    def test_random_box_results_follow_apk_weighted_domain_and_inventory(self):
        login=self.character();self.seed(login,'item_022',100)
        op,r=self.use(login,'item_022',100);self.assertEqual(op,BY_NAME['SCItemUse'],str(r))
        table=Path(__file__).resolve().parents[1]/'analysis/tables/TableItemUse.tsv'
        rows=list(csv.DictReader(table.read_text().splitlines()[1:],delimiter='\t'))[3:]
        allowed={x['result'].split(',')[0] for x in rows if x['itemId']=='item_022'}
        self.assertTrue(all(x.treasureId in allowed for x in r.get.treas));self.assertTrue(all(x.id in allowed for x in r.get.simps))
        self.assertEqual(len(r.get.treas)+sum(x.num for x in r.get.simps),100)
        self.assertEqual(len(self.treasures(login)),len(r.get.treas))
        _,fragments=self.send('CSFragmentList',login=login)
        self.assertEqual({x.id:x.num for x in fragments.fragTreasureList},{x.id:x.num for x in r.get.simps})
        self.denied(login,'CSItemUse',{'costItem':{'id':'item_022','num':1}})

    def test_sql_disable_invalid_mode_and_inventory_insufficiency_do_not_consume(self):
        login=self.character();self.seed(login,'item_115',2)
        with self.service.store.connect() as conn:
            conn.execute("UPDATE treasure_box_rules SET enabled=0 WHERE item_id='item_115'")
        self.denied(login,'CSItemUse',{'costItem':{'id':'item_115','num':1}})
        self.stop();self.start();login=self.login('equipment-test')
        self.denied(login,'CSItemUse',{'costItem':{'id':'item_115','num':1}})
        with self.service.store.connect() as conn:
            conn.execute("UPDATE treasure_box_rules SET enabled=1,mode='unknown' WHERE item_id='item_115'")
        self.denied(login,'CSItemUse',{'costItem':{'id':'item_115','num':1}})
        with self.service.store.connect() as conn:
            conn.execute("UPDATE treasure_box_rules SET mode='normalized_weight' WHERE item_id='item_115'")
        self.denied(login,'CSItemUse',{'costItem':{'id':'item_115','num':3}})
        op,r=self.use(login,'item_115',2);self.assertEqual(op,BY_NAME['SCItemUse'],str(r));self.assertEqual(len(r.get.treas),2)
        self.assertEqual(len(self.treasures(login)),2)

    def test_sql_reward_hot_update_old_receipt_stable_and_restart(self):
        login=self.character();self.seed(login,'item_022',5)
        with self.service.store.connect() as conn:
            conn.execute("UPDATE treasure_box_rewards SET weight=0 WHERE item_id='item_022'")
            conn.execute("UPDATE treasure_box_rewards SET weight=1 WHERE item_id='item_022' AND resource_id='frag_trea_001_1'")
        self.fixed_stamp=1800000000030
        op,first=self.use(login,'item_022');self.assertEqual(op,BY_NAME['SCItemUse'],str(first))
        self.assertEqual([(r.id,r.num) for r in first.get.simps],[('frag_trea_001_1',1)])
        with self.service.store.connect() as conn:
            conn.execute("UPDATE treasure_box_rewards SET weight=0 WHERE item_id='item_022'")
            conn.execute("UPDATE treasure_box_rewards SET weight=1 WHERE item_id='item_022' AND resource_id='trea_001'")
        _,replay=self.use(login,'item_022');self.assertEqual(replay,first)
        del self.fixed_stamp
        op,second=self.use(login,'item_022');self.assertEqual(op,BY_NAME['SCItemUse'],str(second));self.assertEqual(second.get.treas[0].treasureId,'trea_001')
        self.stop();self.start();login=self.login('equipment-test')
        op,third=self.use(login,'item_022');self.assertEqual(op,BY_NAME['SCItemUse'],str(third));self.assertEqual(third.get.treas[0].treasureId,'trea_001')
        self.assertNotEqual(second.get.treas[0].guid,third.get.treas[0].guid)

    def test_shared_capacity_batch_and_sql_reward_quantity_rollback(self):
        login=self.character();self.seed(login,'item_025',5)
        def capacity(player,game):
            player['equipBagSize']=3
            game['equipment']=[{'guid':'capacity-equip','equipId':'equip_001','level':1}]
        self.mutate(login,capacity)
        def profile():
            with self.service.store.connect() as conn:
                return dict(conn.execute('SELECT player,game FROM profiles WHERE id=?',(login.playerId,)).fetchone())
        before=profile()
        self.denied(login,'CSItemUse',{'costItem':{'id':'item_025','num':3}})
        self.assertEqual(profile(),before)
        # SQL single reward exceeding remaining slots must roll back the whole request.
        with self.service.store.connect() as conn:
            conn.execute("UPDATE treasure_box_rewards SET amount=3 WHERE item_id='item_025'")
        self.denied(login,'CSItemUse',{'costItem':{'id':'item_025','num':1}})
        self.assertEqual(profile(),before)
        with self.service.store.connect() as conn:
            conn.execute("UPDATE treasure_box_rewards SET amount=101 WHERE item_id='item_025'")
        self.denied(login,'CSItemUse',{'costItem':{'id':'item_025','num':1}})
        self.assertEqual(profile(),before)
        with self.service.store.connect() as conn:
            conn.execute("UPDATE treasure_box_rewards SET amount=1 WHERE item_id='item_025'")
        op,r=self.use(login,'item_025',2);self.assertEqual(op,BY_NAME['SCItemUse'],str(r));self.assertEqual(len(r.get.treas),2)
        self.assertEqual(len(self.treasures(login)),2)
        self.denied(login,'CSItemUse',{'costItem':{'id':'item_025','num':1}})
        # Stackable chips do not consume equipment/treasure instance slots.
        self.mutate(login,lambda p,g:g['items'].append({'id':'item_022','num':1}))
        with self.service.store.connect() as conn:
            conn.execute("UPDATE treasure_box_rewards SET weight=0 WHERE item_id='item_022'")
            conn.execute("UPDATE treasure_box_rewards SET weight=1 WHERE item_id='item_022' AND resource_id='frag_trea_001_1'")
        op,r=self.use(login,'item_022');self.assertEqual(op,BY_NAME['SCItemUse'],str(r));self.assertEqual(r.get.simps[0].id,'frag_trea_001_1')
        self.assertEqual(len(self.treasures(login)),2)

    def test_fragment_box_relogin_initialization_projects_each_chip_once(self):
        login=self.character();self.seed(login,'item_022',2)
        with self.service.store.connect() as conn:
            conn.execute("UPDATE treasure_box_rewards SET weight=0 WHERE item_id='item_022'")
            conn.execute("UPDATE treasure_box_rewards SET weight=1 WHERE item_id='item_022' AND resource_id='frag_trea_001_1'")
        op,gain=self.use(login,'item_022',2)
        self.assertEqual(op,BY_NAME['SCItemUse'],str(gain));self.assertEqual([(x.id,x.num) for x in gain.get.simps],[('frag_trea_001_1',2)])
        self.stop();self.start();login=self.login('equipment-test')
        names=['CSPlayerLogin','CSWarriorList','CSFormationList','CSItemList','CSQuestList','CSKVInfo',
               'CSCopyPassedInfo','CSCopyCurrentInfo','CSGetUnionAddBaseProp','CSShopProductList',
               'CSEquipmentTreasureList','CSGemAndCookingList','CSFragmentList','CSTitleList',
               'CSPetList','CSChangeNameTimes','CSFriendList','CSGetUnionInfo','CSCookingLevel',
               'CSRedPoint','CSFriendBlackList','CSNavigationItem','CSKFLogin','CSChatCache',
               'CSCurrentLuckyBagsInfo','CSLoginRewardInfo','CSNoticeUrl','CSOpenStateCheck',
               'CSUFInfoPreView','CSKFOpenFrightServerInfo','CSGetRebels','CSCookingEffectList',
               'CSLoginScene','CSCurrentPosition','CSBigClientReward']
        replies={}
        for name in names:
            opcode,reply=self.send(name,login=login)
            self.assertEqual(opcode,BY_NAME['SC'+name[2:]],str(reply));replies[name]=reply
        # InitData adds SCItemList then SCFragmentList; the fragment has one authoritative projection.
        items=replies['CSItemList'].itemList;frags=replies['CSFragmentList'].fragTreasureList
        self.assertFalse(any(x.id.startswith('frag_') for x in items))
        self.assertEqual([(x.id,x.num) for x in frags],[('frag_trea_001_1',2)])
        self.assertEqual(sum(x.num for x in list(items)+list(frags) if x.id=='frag_trea_001_1'),2)
        with self.service.store.connect() as conn:
            row=conn.execute('SELECT game FROM profiles WHERE id=?',(login.playerId,)).fetchone()
        import json
        stored=json.loads(row['game'])
        self.assertEqual(next(x['num'] for x in stored['items'] if x['id']=='frag_trea_001_1'),2)

    def test_local_sql_mode_and_apk_weights_are_explicit(self):
        login=self.character()
        with self.service.store.connect() as conn:
            rules=conn.execute('SELECT * FROM treasure_box_rules ORDER BY item_id').fetchall()
        self.assertEqual(len(rules),6)
        self.assertTrue(all(r['mode']=='normalized_weight' and r['max_batch']==100 and 'original denominator unknown' in r['source'] for r in rules))

del GoTreasureTests
