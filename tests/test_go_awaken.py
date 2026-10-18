"""伙伴觉醒与融合：独立临时SQLite、真实Go进程、恢复的Python协议编解码。"""
import concurrent.futures
import json
import unittest

from server.codec import BY_NAME
from test_go_server import GoServerTests
from test_go_warrior import GoWarriorTests
from test_go_formation import GoFormationTests


class GoAwakenTests(unittest.TestCase):
    setUp = GoServerTests.setUp
    tearDown = GoServerTests.tearDown
    start = GoServerTests.start
    stop = GoServerTests.stop
    send = GoServerTests.send
    login = GoServerTests.login
    stamped = GoFormationTests.stamped
    snapshot = GoWarriorTests.snapshot

    def sql(self, text, args=()):
        with self.service.store.connect() as connection:
            return connection.execute(text, args).fetchall()

    def mutate(self, login, fn):
        p, g = self.snapshot(login)
        fn(p, g)
        self.sql('UPDATE profiles SET player=?,game=? WHERE id=?',
                 (json.dumps(p), json.dumps(g), login.playerId))

    def fixture(self, uid='awake', stage=0, level=70, donors=9):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '觉醒回归', 'heroId': 'hero_047'}, login)
        def prepare(p, g):
            p.update(level=100, coin=1000000, stamina=100000)
            g['items'] = [{'id': key, 'num': count} for key, count in
                          [('item_320', 4000), ('item_313', 400), ('item_312', 1000)]]
            target = {'id': 'hero_001', 'guid': 'target', 'level': level, 'exp': 0, 'talent': 0,
                      'training': {}, 'tempTraining': {}, 'breached': {'level': 1},
                      'awake': {'stage': stage, 'fuseLevel': 0, 'fuseExp': 0}}
            if stage == 2:
                target['awakenSkill'] = '0001_1'
            g['warriors'].append(target)
            for i in range(donors):
                donor = dict(target, guid='donor-%02d' % i, level=1, awake={})
                donor.pop('awakenSkill', None)
                g['warriors'].append(donor)
            g['formation'].append({'formatPos': 2, 'battlePos': 2,
                                   'things': [{'pos': 0, 'id': 'hero_001', 'guid': 'target'}]})
        self.mutate(login, prepare)
        return login

    def deny(self, name, fields, login, stamp):
        before = self.snapshot(login)
        receipts = [dict(r) for r in self.sql('SELECT * FROM warrior_growth_receipts')]
        opcode, reply = self.stamped(name, fields, login, stamp)
        self.assertEqual((opcode, reply.code), (10001, '90003'), str(reply))
        self.assertEqual(self.snapshot(login), before)
        self.assertEqual([dict(r) for r in self.sql('SELECT * FROM warrior_growth_receipts')], receipts)

    def battle(self, login):
        _, reply = self.send('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, login)
        return next(w for w in reply.battlePlayer.formation if w.guid == 'target')

    def test_stages_exact_cost_retry_restart_and_battle(self):
        login = self.fixture()
        self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        before = self.battle(login)
        fields = {'guid': 'target'}
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            results = list(pool.map(lambda _: self.stamped('CSWarriorAwake', fields, login, 100), range(4)))
        self.assertTrue(all(op == BY_NAME['SCWarriorAwake'] for op, _ in results), str(results))
        first = results[0][1]
        self.assertTrue(all(reply == first for _, reply in results))
        self.assertEqual(first.newWarrior.awake.stage, 1)
        self.assertEqual({(c.id, c.guid): c.num for c in first.cost},
                         {('item_320', ''): 800, ('item_313', ''): 50, ('item_312', ''): 15,
                          ('hero_001', 'donor-00'): 1, ('hero_001', 'donor-01'): 1})
        actor1 = self.battle(login)
        self.assertEqual(actor1.awakeStage, 1)
        self.assertIn('tal_1087', actor1.talentList)
        self.assertGreater(actor1.prop.hp, before.prop.hp)
        self.stop()
        self.start()
        login = self.login('awake')
        self.assertEqual(self.stamped('CSWarriorAwake', fields, login, 100)[1], first)
        opcode, second = self.stamped('CSWarriorAwake', fields, login, 200)
        self.assertEqual(opcode, BY_NAME['SCWarriorAwake'], str(second))
        self.assertEqual(second.newWarrior.awake.stage, 2)
        self.assertEqual(second.newWarrior.awakenSkill, '0001_1')
        self.assertEqual(sum(c.num for c in second.cost if c.id == 'hero_001'), 5)
        actor2 = self.battle(login)
        self.assertEqual((actor2.awakeStage, actor2.awakenSkill), (2, '0001_1'))
        self.assertGreater(actor2.prop.hp, actor1.prop.hp)
        self.deny('CSWarriorAwake', fields, login, 300)
        self.deny('CSWarriorAwake', fields, login, 150)
        self.assertEqual(self.snapshot(login)[0]['coin'], 1000000)

    def test_ownership_gate_donor_safety_and_rollback(self):
        login = self.fixture(level=69, donors=2)
        fields = {'guid': 'target'}
        self.deny('CSWarriorAwake', fields, login, 100)
        self.deny('CSWarriorAwake', {'guid': 'foreign-guid'}, login, 100)
        def prepare(p, g):
            next(w for w in g['warriors'] if w['guid'] == 'target')['level'] = 70
            next(w for w in g['warriors'] if w['guid'] == 'donor-00')['training'] = {'hp': 1}
        self.mutate(login, prepare)
        self.deny('CSWarriorAwake', fields, login, 100)
        def active(p, g):
            next(w for w in g['warriors'] if w['guid'] == 'donor-00')['training'] = {}
            g['formation'].append({'formatPos': 3, 'battlePos': 3,
                                   'things': [{'pos': 0, 'id': 'hero_001', 'guid': 'donor-00'}]})
        self.mutate(login, active)
        self.deny('CSWarriorAwake', fields, login, 100)
        def poor(p, g):
            g['formation'] = g['formation'][:2]
            p['coin'] = 0
        self.mutate(login, poor)
        self.sql("UPDATE warrior_awake_rules SET coin_cost=1 WHERE id='awake_1'")
        self.deny('CSWarriorAwake', fields, login, 100)
        self.sql("UPDATE warrior_awake_rules SET enabled=0 WHERE id='awake_1'")
        self.deny('CSWarriorAwake', fields, login, 100)

    def test_fusion_server_exp_cost_limits_replay_and_battle(self):
        login = self.fixture(stage=2)
        self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        before = self.battle(login)
        fields = {'guid': 'target', 'cost': [{'id': 'item_312', 'num': 1}]}
        opcode, reply = self.stamped('CSAwakeFuse', fields, login, 100)
        self.assertEqual(opcode, BY_NAME['SCAwakeFuse'], str(reply))
        # hero_001 quality23: 50 + 100*4经验到5级，余量50。
        self.assertEqual((reply.newWarrior.awake.fuseLevel, reply.newWarrior.awake.fuseExp), (5, 50))
        self.assertEqual([(c.id, c.num) for c in reply.cost], [('item_312', 1)])
        self.assertEqual(self.stamped('CSAwakeFuse', fields, login, 100)[1], reply)
        after = self.battle(login)
        self.assertAlmostEqual(after.prop.hp-before.prop.hp, 389*5)
        self.assertAlmostEqual(after.prop.att-before.prop.att, 10*5)
        self.deny('CSAwakeFuse', {'guid': 'target', 'cost': [{'id': 'item_312', 'num': 2}]}, login, 100)
        self.deny('CSAwakeFuse', fields, login, 99)
        for bad in [[], [{'id': 'item_313', 'num': 1}], [{'id': 'item_312', 'num': 0}],
                    [{'id': 'item_312', 'num': 1, 'guid': 'fake'}],
                    [{'id': 'item_312', 'num': 1}, {'id': 'item_312', 'num': 1}],
                    [{'id': 'item_312', 'num': 2000}]]:
            self.deny('CSAwakeFuse', {'guid': 'target', 'cost': bad}, login, 200)
        def maxout(p, g):
            next(w for w in g['warriors'] if w['guid'] == 'target')['awake'] = {'stage': 2, 'fuseLevel': 99, 'fuseExp': 1699}
        self.mutate(login, maxout)
        opcode, reply = self.stamped('CSAwakeFuse', fields, login, 200)
        self.assertEqual(opcode, BY_NAME['SCAwakeFuse'], str(reply))
        self.assertEqual((reply.newWarrior.awake.fuseLevel, reply.newWarrior.awake.fuseExp), (100, 0))
        maximum = self.battle(login)
        self.assertAlmostEqual(maximum.prop.hp-before.prop.hp, 30*389+40*418+30*487)
        self.assertAlmostEqual(maximum.prop.att-before.prop.att, 30*10+40*15+30*20)
        self.deny('CSAwakeFuse', fields, login, 300)
        self.stop()
        self.start()
        login = self.login('awake')
        self.assertEqual(self.stamped('CSAwakeFuse', fields, login, 200)[1], reply)
        self.deny('CSAwakeFuse', fields, login, 300)

    def test_quality20_uses_own_cost_and_fusion_exp_table(self):
        login = self.fixture()
        def change_hero(p, g):
            for w in g['warriors']:
                if w['id'] == 'hero_001':
                    w['id'] = 'hero_009'
            g['items'] = [{'id': k, 'num': n} for k, n in
                          [('item_322', 1000), ('item_317', 200), ('item_316', 200), ('item_312', 10)]]
            g['formation'][1]['things'][0]['id'] = 'hero_009'
        self.mutate(login, change_hero)
        for stamp, count in [(100, 2), (200, 5)]:
            opcode, reply = self.stamped('CSWarriorAwake', {'guid': 'target'}, login, stamp)
            self.assertEqual(opcode, BY_NAME['SCWarriorAwake'], str(reply))
            self.assertEqual(sum(c.num for c in reply.cost if c.id == 'hero_009'), count)
        self.assertEqual({c.id: c.num for c in reply.cost if not c.guid},
                         {'item_322': 400, 'item_317': 50, 'item_316': 100})
        self.assertEqual(reply.newWarrior.awakenSkill, '0009_1')
        opcode, reply = self.stamped('CSAwakeFuse', {'guid': 'target', 'cost': [{'id': 'item_312', 'num': 1}]}, login, 100)
        self.assertEqual(opcode, BY_NAME['SCAwakeFuse'], str(reply))
        self.assertEqual((reply.newWarrior.awake.fuseLevel, reply.newWarrior.awake.fuseExp), (11, 25))


del GoServerTests, GoWarriorTests, GoFormationTests

if __name__ == '__main__':
    unittest.main()
