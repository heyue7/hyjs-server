"""Equip refine/star costs, timestamp replay, and real battle properties in temporary saves."""
import concurrent.futures
import json
import unittest

from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests
from test_go_formation import GoFormationTests


class GoEquipmentAdvanceTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    character = GoEquipmentTests.character
    mutate = GoEquipmentTests.mutate
    seed = GoEquipmentTests.seed
    equipment = GoEquipmentTests.equipment
    battle = GoEquipmentTests.battle
    stamped = GoFormationTests.stamped

    def fixture(self, uid='equipment-advance', *, high=False, level=70, coin=1000000, items=None):
        login = self.character(uid)
        self.seed(login, level=level, coin=coin)
        def apply(player, game):
            game['items'] = [{'id': key, 'num': num} for key, num in (items or {}).items()]
            if high:
                for n, equip in enumerate(game['equipment']):
                    equip['equipId'] = f'equip_{25 + n:03}'
        self.mutate(login, apply)
        return login

    def saved(self, login):
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
        return json.loads(row['player']), json.loads(row['game'])

    def denied(self, login, name, fields, stamp=100):
        before = self.saved(login)
        opcode, error = self.stamped(name, fields, login, stamp)
        self.assertEqual((opcode, error.code), (10001, '90003'), str(error))
        self.assertEqual(self.saved(login), before)

    def refine(self, login, costs, stamp=100, guid='equip-1'):
        opcode, result = self.stamped('CSEquipRefine', {'guid': guid, 'cost': costs}, login, stamp)
        self.assertEqual(opcode, BY_NAME['SCEquipRefine'], str(result))
        return result

    def star(self, login, stamp=100, guid='equip-1'):
        opcode, result = self.stamped('CSEquipStar', {'guid': guid}, login, stamp)
        self.assertEqual(opcode, BY_NAME['SCEquipStar'], str(result))
        return result

    def wear(self, login, number):
        equip = self.equipment(login)[f'equip-{number}']
        opcode, result = self.send('CSFormationOn', {'id': equip.equipId, 'guid': equip.guid, 'formatPos': 1}, login)
        self.assertEqual(opcode, BY_NAME['SCFormationOn'], str(result))

    def test_refine_mixed_stones_residual_experience_and_restart(self):
        login = self.fixture(level=1, items={'item_011': 2, 'item_010': 1})
        costs = [{'id': 'item_011', 'num': 1}, {'id': 'item_010', 'num': 1}]
        result = self.refine(login, costs)
        self.assertEqual((result.equip.refineLv, result.equip.refineExp), (1, 5))
        self.assertEqual([(c.id, c.num, c.guid) for c in result.cost],
                         [('item_011', 1, ''), ('item_010', 1, '')])
        player, game = self.saved(login)
        self.assertEqual(player['coin'], 1000000)
        self.assertEqual(game['items'], [{'id': 'item_011', 'num': 1}])
        self.assertFalse(game['quests'])
        self.assertEqual(player['noviceGuideSchedule'], 0)
        self.stop(); self.start(); login = self.login('equipment-advance')
        self.assertEqual(self.refine(login, costs), result)
        self.assertEqual((self.equipment(login)['equip-1'].refineLv,
                          self.equipment(login)['equip-1'].refineExp), (1, 5))
        self.assertEqual(self.saved(login), (player, game))
        self.refine(login, [{'id': 'item_011', 'num': 1}], stamp=101)
        self.assertEqual(self.equipment(login)['equip-1'].refineExp, 10)
        self.denied(login, 'CSEquipRefine', {'guid': 'equip-1', 'cost': costs}, stamp=100)

    def test_refine_ownership_material_validation_and_cap_are_atomic(self):
        login = self.fixture(items={'item_008': 1, 'item_011': 1})
        for fields, stamp in [({'guid': 'foreign', 'cost': [{'id': 'item_008', 'num': 1}]}, 100),
                              ({'guid': 'equip-1', 'cost': [{'id': 'item_008', 'guid': 'foreign', 'num': 1}]}, 100),
                              ({'guid': 'equip-1', 'cost': [{'id': 'item_008', 'num': 2}]}, 100),
                              ({'guid': 'equip-1', 'cost': [{'id': 'item_005', 'num': 1}]}, 100),
                              ({'guid': 'equip-1', 'cost': [{'id': 'coin', 'num': 1}]}, 100),
                              ({'guid': 'equip-1', 'cost': [{'id': 'item_008', 'num': 1}, {'id': 'item_008', 'num': 1}]}, 100),
                              ({'guid': 'equip-1', 'cost': [{'id': 'item_011', 'num': 0}]}, 100),
                              ({'guid': 'equip-1', 'cost': [{'id': 'item_011', 'num': 1}]}, 0)]:
            self.denied(login, 'CSEquipRefine', fields, stamp)
        self.mutate(login, lambda p, g: g['equipment'][0].update(refineLv=79, refineExp=21690))
        result = self.refine(login, [{'id': 'item_011', 'num': 1}])
        self.assertEqual((result.equip.refineLv, result.equip.refineExp), (80, 4))
        self.denied(login, 'CSEquipRefine', {'guid': 'equip-1', 'cost': [{'id': 'item_008', 'num': 1}]}, 101)

    def test_refine_concurrent_identical_packets_replay_and_content_conflicts(self):
        login = self.fixture(items={'item_008': 1, 'item_011': 1})
        fields = {'guid': 'equip-1', 'cost': [{'id': 'item_008', 'num': 1}]}
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            results = list(pool.map(lambda _: self.stamped('CSEquipRefine', fields, login, 100), range(6)))
        self.assertTrue(all(opcode == BY_NAME['SCEquipRefine'] for opcode, _ in results), str(results))
        self.assertTrue(all(result == results[0][1] for _, result in results))
        self.assertEqual((results[0][1].equip.refineLv, results[0][1].equip.refineExp), (2, 20))
        self.assertEqual(self.saved(login)[1]['items'], [{'id': 'item_011', 'num': 1}])
        self.denied(login, 'CSEquipRefine', {'guid': 'equip-1', 'cost': [{'id': 'item_011', 'num': 1}]}, 100)
        self.denied(login, 'CSEquipRefine', fields, 99)

    def test_star_fixed_cost_concurrency_and_restart_replay(self):
        login = self.fixture(high=True, coin=100000, items={'item_460': 3, 'item_459': 70, 'item_461': 20})
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.star(login), range(6)))
        self.assertTrue(all(reply == replies[0] for reply in replies))
        result = replies[0]
        self.assertEqual(result.equip.star, 1)
        self.assertEqual([(c.id, c.num) for c in result.cost],
                         [('coin', 100000), ('item_460', 3), ('item_459', 70), ('item_461', 20)])
        player, game = self.saved(login)
        self.assertEqual(player['coin'], 0)
        self.assertFalse(game['items'])
        self.stop(); self.start(); login = self.login('equipment-advance')
        self.assertEqual(self.star(login), result)
        self.assertEqual(self.saved(login), (player, game))
        self.denied(login, 'CSEquipStar', {'guid': 'equip-1'}, 99)
        self.denied(login, 'CSEquipStar', {'guid': 'equip-1'}, 101)
        def next_materials(player, game):
            player['coin'] = 200000
            game['items'] = [{'id': k, 'num': n} for k, n in {'item_460': 5, 'item_459': 90, 'item_462': 22}.items()]
        self.mutate(login, next_materials)
        self.assertEqual(self.star(login, 101).equip.star, 2)
        self.denied(login, 'CSEquipStar', {'guid': 'equip-1'}, 100)

    def test_star_gate_cap_missing_material_and_foreign_guid_roll_back(self):
        login = self.fixture(high=True, level=69, items={'item_460': 3, 'item_459': 70, 'item_461': 19})
        self.denied(login, 'CSEquipStar', {'guid': 'equip-1'})
        self.mutate(login, lambda p, g: p.update(level=70))
        self.denied(login, 'CSEquipStar', {'guid': 'equip-1'})
        self.denied(login, 'CSEquipStar', {'guid': 'foreign'})
        self.denied(login, 'CSEquipStar', {'guid': 'equip-1'}, 0)
        self.mutate(login, lambda p, g: g['equipment'][0].update(star=5))
        self.denied(login, 'CSEquipStar', {'guid': 'equip-1'})
        self.mutate(login, lambda p, g: g['equipment'][0].update(star=0, equipId='equip_001'))
        self.denied(login, 'CSEquipStar', {'guid': 'equip-1'})

    def test_refine_attributes_and_master_reach_real_battle(self):
        login = self.fixture(items={'item_008': 4})
        baseline = self.battle(login)
        for n in range(1, 5):
            self.refine(login, [{'id': 'item_008', 'num': 1}], guid=f'equip-{n}')
            self.wear(login, n)
        prop = self.battle(login)
        expected = {'att': 81, 'hp': 1920, 'mdef': 28, 'pdef': 28,
                    'cri': 2, 'antiCri': 2, 'hit': 1.6, 'dodge': 1.6,
                    'damageAdd': 3, 'damageReduce': 3}
        for key, increment in expected.items():
            self.assertAlmostEqual(getattr(prop, key) - getattr(baseline, key), increment, msg=key)
        self.assertEqual(self.send('CSEquipStrengthen',
                {'guid': 'equip-1', 'times': 1, 'cost': [{'id': 'coin', 'num': 8}]}, login)[0], BY_NAME['SCEquipStrengthen'])

    def test_star_single_piece_and_complete_set_percentage_order(self):
        login = self.fixture(high=True, items={'item_460': 12, 'item_459': 280, 'item_461': 80})
        for n in range(1, 5):
            self.wear(login, n)
        before = self.battle(login)
        for n in range(1, 4):
            self.star(login, guid=f'equip-{n}')
        partial = self.battle(login)
        self.assertAlmostEqual(partial.att, before.att + 500)
        self.assertAlmostEqual(partial.hp, before.hp + 2000)
        self.assertAlmostEqual(partial.mdef, before.mdef + 150)
        self.assertAlmostEqual(partial.pdef, before.pdef)
        self.star(login, guid='equip-4')
        prop = self.battle(login)
        for key, increment in {'att': 500, 'hp': 2000, 'mdef': 150, 'pdef': 150}.items():
            self.assertAlmostEqual(getattr(prop, key), (getattr(before, key) + increment) * 1.01, msg=key)

    def test_legend_refine_bonuses_accumulate_at_unlocked_thresholds(self):
        login = self.fixture()
        self.mutate(login, lambda p, g: g['equipment'][0].update(equipId='equip_037', refineLv=21, refineExp=0))
        baseline = self.battle(login)
        self.wear(login, 1)
        prop = self.battle(login)
        # Quality23 weapon has 149 base, 26.0 refine growth, and two unlocked +1000 bonuses.
        self.assertAlmostEqual(prop.att - baseline.att, 149 + 26 * 21 + 2000)
        self.assertAlmostEqual(prop.cri - baseline.cri, 21)


del GoEquipmentTests, GoFormationTests
