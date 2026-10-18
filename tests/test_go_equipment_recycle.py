"""Original binary equipment recovery, isolated temporary saves, deterministic endpoint odds."""
import concurrent.futures
import unittest

from server.codec import BY_NAME
from test_go_equipment_advance import GoEquipmentAdvanceTests


class GoEquipmentRecycleTests(unittest.TestCase):
    setUp = GoEquipmentAdvanceTests.setUp
    tearDown = GoEquipmentAdvanceTests.tearDown
    start = GoEquipmentAdvanceTests.start
    stop = GoEquipmentAdvanceTests.stop
    send = GoEquipmentAdvanceTests.send
    login = GoEquipmentAdvanceTests.login
    character = GoEquipmentAdvanceTests.character
    mutate = GoEquipmentAdvanceTests.mutate
    seed = GoEquipmentAdvanceTests.seed
    equipment = GoEquipmentAdvanceTests.equipment
    stamped = GoEquipmentAdvanceTests.stamped
    saved = GoEquipmentAdvanceTests.saved
    denied = GoEquipmentAdvanceTests.denied

    def fixture(self, uid='equipment-recycle'):
        login = self.character(uid)
        self.seed(login, level=70, coin=100000)
        return login

    def decompose(self, login, guids, stamp=100):
        opcode, result = self.stamped('CSEquipDecompose', {'guid': guids}, login, stamp)
        self.assertEqual(opcode, BY_NAME['SCEquipDecompose'], str(result))
        return result

    def forge_fixture(self):
        login = self.fixture('equipment-forge')
        self.mutate(login, lambda p, g: (
            g['equipment'][0].update(equipId='equip_037'),
            g.update(items=[{'id': 'frag_equip_037', 'num': 10}])))
        return login

    def forge(self, login, stamp=100, costs=None):
        costs = costs or [{'id': 'frag_equip_037', 'num': 3}, {'id': 'coin', 'num': 1000}]
        opcode, result = self.stamped('CSEquipForge', {'guid': 'equip-1', 'cost': costs}, login, stamp)
        self.assertEqual(opcode, BY_NAME['SCEquipForge'], str(result))
        return result

    def test_base_rewards_batch_restart_and_concurrent_retry(self):
        login = self.fixture()
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.decompose(login, ['equip-1', 'equip-5']), range(6)))
        self.assertTrue(all(reply == replies[0] for reply in replies))
        result = replies[0]
        self.assertEqual([(i.id, i.num) for i in result.get.simps], [('point', 780), ('coin', 2000)])
        self.assertEqual([(i.id, i.guid, i.num) for i in result.cost],
                         [('equip_001', 'equip-1', 1), ('equip_005', 'equip-5', 1)])
        player, game = self.saved(login)
        self.assertEqual((player['coin'], player['point']), (102000, 780))
        self.assertEqual(len(game['equipment']), 3)
        self.stop(); self.start(); login = self.login('equipment-recycle')
        self.assertEqual(self.decompose(login, ['equip-1', 'equip-5']), result)
        self.assertEqual(self.saved(login), (player, game))
        self.denied(login, 'CSEquipDecompose', {'guid': ['equip-2']}, 100)
        self.denied(login, 'CSEquipDecompose', {'guid': ['equip-1', 'equip-5']}, 99)

    def test_invalid_batch_foreign_worn_and_overflow_are_atomic(self):
        login = self.fixture()
        for guids in [[], ['equip-1'] * 2, ['equip-1', 'foreign'], [f'equip-{i}' for i in range(1, 6)]]:
            self.denied(login, 'CSEquipDecompose', {'guid': guids})
        opcode, result = self.send('CSFormationOn', {'guid': 'equip-1', 'id': 'equip_001', 'formatPos': 1}, login)
        self.assertEqual(opcode, BY_NAME['SCFormationOn'], str(result))
        self.denied(login, 'CSEquipDecompose', {'guid': ['equip-2', 'equip-1']})
        self.mutate(login, lambda p, g: p.update(point=2147483640))
        self.denied(login, 'CSEquipDecompose', {'guid': ['equip-2']})

    def test_sql_progression_policy_and_disable_are_enforced(self):
        login = self.fixture()
        self.mutate(login, lambda p, g: g['equipment'][0].update(level=2))
        with self.service.store.connect() as connection:
            connection.execute("UPDATE equipment_recycle_rules SET progression_refund='reject_progressed'")
        self.denied(login, 'CSEquipDecompose', {'guid': ['equip-1']})
        with self.service.store.connect() as connection:
            connection.execute("UPDATE equipment_recycle_rules SET progression_refund='base_only'")
        result = self.decompose(login, ['equip-1'])
        self.assertEqual([(i.id, i.num) for i in result.get.simps], [('point', 80), ('coin', 1000)])
        with self.service.store.connect() as connection:
            connection.execute('UPDATE equipment_recycle_rules SET enabled=0')
        self.denied(login, 'CSEquipDecompose', {'guid': ['equip-2']}, 101)

    def test_forge_original_fixed_cost_and_same_packet_concurrency(self):
        login = self.forge_fixture()
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.forge(login), range(6)))
        self.assertTrue(all(reply == replies[0] for reply in replies))
        result = replies[0]
        self.assertEqual((result.result, result.equip.forgeLevel), (1, 1))
        player, game = self.saved(login)
        self.assertEqual(player['coin'], 99000)
        self.assertEqual(game['items'], [{'id': 'frag_equip_037', 'num': 7}])
        self.stop(); self.start(); login = self.login('equipment-forge')
        self.assertEqual(self.forge(login), result)
        self.assertEqual(self.saved(login), (player, game))
        self.denied(login, 'CSEquipForge', {'guid': 'equip-1', 'cost': [{'id': 'coin', 'num': 1}]}, 101)

    def test_forge_ownership_gate_and_fragments_are_atomic(self):
        login = self.forge_fixture()
        costs = [{'id': 'frag_equip_037', 'num': 3}, {'id': 'coin', 'num': 1000}]
        for fields in [
            {'guid': 'foreign', 'cost': costs},
            {'guid': 'equip-1', 'cost': costs + [costs[0]]},
            {'guid': 'equip-1', 'cost': [{'id': 'frag_equip_037', 'guid': 'foreign', 'num': 3}, costs[1]]},
        ]:
            self.denied(login, 'CSEquipForge', fields)
        self.mutate(login, lambda p, g: p.update(level=59))
        self.denied(login, 'CSEquipForge', {'guid': 'equip-1', 'cost': costs})
        self.mutate(login, lambda p, g: (p.update(level=60), g.update(items=[])))
        self.denied(login, 'CSEquipForge', {'guid': 'equip-1', 'cost': costs})


del GoEquipmentAdvanceTests

if __name__ == '__main__':
    unittest.main()
