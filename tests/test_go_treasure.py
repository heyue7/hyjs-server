"""饰品原协议闭环；仅使用独立临时SQLite测试夹具。"""
import concurrent.futures
import http.client
import time
from test_go_equipment import GoEquipmentTests
from server.codec import BY_NAME, message, encode_http_request, decode_responses
import unittest

class GoTreasureTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    stop = GoEquipmentTests.stop
    start = GoEquipmentTests.start
    def send(self, name, fields=None, login=None):
        args={'zone_id':1,'timestamp':getattr(self,'fixed_stamp',time.time_ns()//1000)}
        if login: args.update(player_id=login.playerId,session=bytes(login.sessionKey))
        raw=encode_http_request(BY_NAME[name],message(name,**(fields or {})),**args)
        connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=3)
        try:
            connection.request('POST','/sao/',body=raw,headers={'Content-Type':'application/octet-stream'})
            reply=connection.getresponse(); data=reply.read();self.assertEqual(reply.status,200,data)
            self.last_frames=decode_responses(data);return self.last_frames[0]
        finally: connection.close()
    login = GoEquipmentTests.login
    character = GoEquipmentTests.character
    mutate = GoEquipmentTests.mutate
    denied = GoEquipmentTests.denied
    battle = GoEquipmentTests.battle
    formation = GoEquipmentTests.formation

    def seed(self, login):
        def apply(player, game):
            player.update(level=20, coin=1000000)
            game['treasures'] = []
            game['items'] = [{'id': f'frag_trea_001_{i}', 'num': 1} for i in range(1,4)] + [{'id':'item_012','num':200}]
        self.mutate(login, apply)

    def treasures(self, login):
        opcode, reply = self.send('CSEquipmentTreasureList', login=login)
        self.assertEqual(opcode, BY_NAME['SCEquipmentTreasureList'], str(reply))
        return {t.guid:t for t in reply.treasureList}

    def combine(self, login):
        opcode, reply = self.send('CSCombine', {'targetId':'trea_001'}, login)
        self.assertEqual(opcode, BY_NAME['SCCombine'], str(reply))
        return reply.get.treas[0]

    def test_combine_all_distinct_chips_concurrent_and_restart(self):
        login=self.character();self.seed(login)
        _,before=self.send('CSFragmentList',login=login)
        self.assertEqual(len(before.fragTreasureList),3)
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            replies=list(pool.map(lambda _:self.send('CSCombine',{'targetId':'trea_001'},login),range(4)))
        self.assertEqual(sum(op==BY_NAME['SCCombine'] for op,_ in replies),1)
        treas=self.treasures(login);self.assertEqual(len(treas),1)
        _,after=self.send('CSFragmentList',login=login);self.assertEqual(len(after.fragTreasureList),0)
        self.stop();self.start();login=self.login('equipment-test')
        self.assertEqual(self.treasures(login),treas)

    def test_combine_receipt_replay_and_stale_request(self):
        login=self.character();self.seed(login)
        self.mutate(login,lambda p,g:[x.update(num=3) for x in g['items'] if x['id'].startswith('frag_')])
        self.fixed_stamp=1800000000010
        first=self.combine(login);replay=self.combine(login);self.assertEqual(first,replay)
        self.fixed_stamp+=1;second=self.combine(login);self.assertNotEqual(first.guid,second.guid)
        self.fixed_stamp-=1;self.denied(login,'CSCombine',{'targetId':'trea_001'})
        del self.fixed_stamp
        self.assertEqual(len(self.treasures(login)),2)
        _,fragments=self.send('CSFragmentList',login=login);self.assertTrue(all(f.num==1 for f in fragments.fragTreasureList))

    def test_mount_strengthen_refine_unmount_reaches_battle(self):
        login=self.character();self.seed(login);t=self.combine(login)
        baseline=self.battle(login)
        op,reply=self.send('CSFormationOn',{'id':t.treasureId,'guid':t.guid,'formatPos':1},login)
        self.assertEqual(op,BY_NAME['SCFormationOn'],str(reply));self.assertEqual(reply.formationItem.things[-1].pos,5)
        mounted=self.battle(login);self.assertAlmostEqual(mounted.att-baseline.att,10)
        self.mutate(login,lambda p,g:g['treasures'].append({'guid':'food','treasureId':'trea_005','strengthLevel':1,'refineLevel':0,'currentExp':0}))
        self.denied(login,'CSTreasureStrengthen',{'targetId':t.guid,'guids':['food','food']})
        op,r=self.send('CSTreasureStrengthen',{'targetId':t.guid,'guids':['food']},login)
        self.assertEqual(op,BY_NAME['SCTreasureStrengthen'],str(r));self.assertGreater(r.treasure.strengthLevel,1)
        self.denied(login,'CSTreasureStrengthen',{'targetId':t.guid,'guids':['food']})
        strong=self.battle(login);self.assertAlmostEqual(strong.att-mounted.att,10*(r.treasure.strengthLevel-1))
        self.fixed_stamp=1800000000001
        op,r=self.send('CSTreasureRefine',{'targetId':t.guid},login)
        self.assertEqual(op,BY_NAME['SCTreasureRefine'],str(r));self.assertEqual(r.treasure.refineLevel,1)
        _,replay=self.send('CSTreasureRefine',{'targetId':t.guid},login);self.assertEqual(replay,r)
        del self.fixed_stamp
        refined=self.battle(login);self.assertAlmostEqual(refined.att,strong.att*1.01)
        # Level2 requires an extra untouched copy; failure rolls back coin and stone.
        self.denied(login,'CSTreasureRefine',{'targetId':t.guid})
        op,r=self.send('CSFormationOff',{'id':t.treasureId,'guid':t.guid,'formatPos':1},login)
        self.assertEqual(op,BY_NAME['SCFormationOff'],str(r));self.assertAlmostEqual(self.battle(login).att,baseline.att)
        self.stop();self.start();login=self.login('equipment-test');persist=self.treasures(login)[t.guid]
        self.assertEqual(persist.refineLevel,1);self.assertGreater(persist.strengthLevel,1)

    def test_refine_copy_consumption_and_skill_threshold(self):
        login=self.character();self.seed(login)
        def fixture(player,game):
            player['coin']=1000000
            game['items']=[{'id':'item_012','num':500}]
            game['treasures']=[{'guid':'target','treasureId':'trea_015','strengthLevel':1,'refineLevel':4,'currentExp':0}]+[{'guid':f'copy-{i}','treasureId':'trea_015','strengthLevel':1,'refineLevel':0,'currentExp':0} for i in range(2)]
        self.mutate(login,fixture)
        baseline=self.battle(login)
        op,r=self.send('CSFormationOn',{'id':'trea_015','guid':'target','formatPos':1},login)
        self.assertEqual(op,BY_NAME['SCFormationOn'],str(r))
        before=self.battle(login);self.assertAlmostEqual(before.att,(baseline.att+200)*1.16)
        op,r=self.send('CSTreasureRefine',{'targetId':'target'},login)
        self.assertEqual(op,BY_NAME['SCTreasureRefine'],str(r));self.assertEqual(r.treasure.refineLevel,5)
        self.assertEqual({c.guid for c in r.cost if c.guid},{'copy-0','copy-1'})
        self.assertEqual(set(self.treasures(login)),{'target'})
        after=self.battle(login);self.assertAlmostEqual(after.att,(baseline.att+200+730)*1.20)
        _,player=self.send('CSPlayerLogin',login=login);self.assertEqual(player.player.coin,700000)

    def test_shared_equipment_treasure_capacity_combine_atomic_rollback(self):
        login=self.character();self.seed(login)
        def fixture(player,game):
            player['equipBagSize']=1
            game['equipment']=[{'guid':'capacity-equip','equipId':'equip_001','level':1}]
        self.mutate(login,fixture)
        with self.service.store.connect() as conn:
            before=dict(conn.execute('SELECT player,game FROM profiles WHERE id=?',(login.playerId,)).fetchone())
        self.denied(login,'CSCombine',{'targetId':'trea_001'})
        with self.service.store.connect() as conn:
            after=dict(conn.execute('SELECT player,game FROM profiles WHERE id=?',(login.playerId,)).fetchone())
        self.assertEqual(after,before)
        self.mutate(login,lambda p,g:p.update(equipBagSize=2))
        self.combine(login);self.assertEqual(len(self.treasures(login)),1)
        self.mutate(login,lambda p,g:[x.update(num=1) for x in g['items'] if x['id'].startswith('frag_')])
        self.denied(login,'CSCombine',{'targetId':'trea_001'})
        _,fragments=self.send('CSFragmentList',login=login)
        self.assertEqual([x.num for x in fragments.fragTreasureList],[1,1,1])

    def test_chip_kind_and_guid_ownership_validation(self):
        login=self.character();self.seed(login)
        self.mutate(login,lambda p,g:g['items'].__setitem__(1,{'id':'frag_trea_001_1','num':10}))
        self.denied(login,'CSCombine',{'targetId':'trea_001'});self.assertEqual(self.treasures(login),{})
        self.denied(login,'CSFormationOn',{'id':'trea_001','guid':'foreign','formatPos':1})
        self.denied(login,'CSTreasureStrengthen',{'targetId':'foreign','guids':['foreign']})
        self.denied(login,'CSTreasureRefine',{'targetId':'foreign'})

del GoEquipmentTests
