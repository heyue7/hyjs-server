"""Basic bag consumables against a real Go server and isolated SQLite profiles."""
import concurrent.futures
import os
from pathlib import Path
import re
import selectors
import subprocess
import types
import unittest

from server.codec import BY_NAME
import test_go_chapter as chapter
from server.store import Store


class GoInventoryTests(unittest.TestCase):
    setUp = chapter.GoChapterTests.setUp
    tearDown = chapter.GoChapterTests.tearDown
    stop = chapter.GoChapterTests.stop
    send = chapter.GoChapterTests.send
    login = chapter.GoChapterTests.login
    create = chapter.GoChapterTests.create
    profile = chapter.GoChapterTests.profile
    mutate = chapter.GoChapterTests.mutate

    def start(self):
        binary = os.environ.get('HYJS_GO_TEST_BINARY')
        if not binary:
            return chapter.GoChapterTests.start(self)
        root = Path(__file__).resolve().parents[1]
        self.process = subprocess.Popen(
            [binary, '--db', str(self.path), '--http-port', '0', '--tcp-port', '0'],
            cwd=root/'hyjs-server', stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT, text=True)
        self.addCleanup(self.stop)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            if not ready.select(20):
                self.fail('Go server did not start within 20 seconds')
            line = self.process.stdout.readline()
        match = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(match, line)
        self.service = types.SimpleNamespace(http_port=int(match[1]), tcp_port=int(match[2]),
                                             store=Store(self.path))

    def supply(self, login, items, **resources):
        def update(player, game):
            player.update(resources)
            game['items'] = [{'id': key, 'num': count} for key, count in items.items()]
        self.mutate(login, update)

    def items(self, login):
        opcode, response = self.send('CSItemList', login=login)
        self.assertEqual(opcode, BY_NAME['SCItemList'], response)
        return {item.id: item.num for item in response.itemList}

    def use(self, login, item, num=1, **fields):
        return self.send('CSItemUse', {'costItem': {'id': item, 'num': num}, **fields}, login)

    def test_configured_rewards_batch_consumption_and_restart(self):
        login = self.create('inventory-basic')
        self.supply(login, {'item_001': 4, 'item_002': 3, 'item_062': 2},
                    energy=20, stamina=100, coin=500)
        login = self.login('inventory-basic')
        for item, count, resource, gain in [('item_001', 3, 'energy', 30),
                                           ('item_002', 3, 'stamina', 75),
                                           ('item_062', 2, 'coin', 20000)]:
            opcode, response = self.use(login, item, count)
            self.assertEqual(opcode, BY_NAME['SCItemUse'], response)
            self.assertEqual(len(self.last_frames), 1)
            self.assertEqual((response.cost.id, response.cost.num), (item, count))
            self.assertEqual([(v.id, v.num) for v in response.get.simps], [(resource, gain)])
            self.assertFalse(response.get.wars or response.get.equips)
        player = self.profile(login)
        self.assertEqual((player.energy, player.stamina, player.coin), (50, 175, 20500))
        self.assertEqual(self.items(login), {'item_001': 1})
        self.stop(); self.start(); login = self.login('inventory-basic')
        self.assertEqual(self.profile(login), player)
        self.assertEqual(self.items(login), {'item_001': 1})
        # Each successful use consumes another owned item; there is no operation ID in CSItemUse.
        self.assertEqual(self.use(login, 'item_001')[0], BY_NAME['SCItemUse'])
        self.assertEqual(self.profile(login).energy, 60)
        self.assertEqual(self.items(login), {})
        self.assertEqual(self.use(login, 'item_001')[1].code, '90003')
        self.assertEqual(self.profile(login).energy, 60)

    def test_invalid_and_unowned_requests_leave_inventory_and_resources_unchanged(self):
        login = self.create('inventory-invalid')
        self.supply(login, {'item_001': 2, 'item_007': 3})
        login = self.login('inventory-invalid')
        before, items = self.profile(login), self.items(login)
        requests = [({},), ({'costItem': {'id': 'item_001', 'num': 0}},),
                    ({'costItem': {'id': 'item_001', 'num': -1}},),
                    ({'costItem': {'id': 'item_001', 'num': 1, 'guid': 'invented'}},),
                    ({'costItem': {'id': 'item_001', 'num': 1}, 'getItemIndex': 1},),
                    ({'costItem': {'id': 'item_001', 'num': 3}},),
                    ({'costItem': {'id': 'item_062', 'num': 1}},),
                    ({'costItem': {'id': 'item_007', 'num': 1}},),
                    ({'costItem': {'id': 'unknown', 'num': 1}},)]
        for (fields,) in requests:
            opcode, error = self.send('CSItemUse', fields, login)
            self.assertEqual((opcode, error.code), (10001, '90003'))
            self.assertEqual(self.profile(login), before)
            self.assertEqual(self.items(login), items)

    def test_parallel_single_item_use_cannot_reward_more_than_inventory(self):
        login = self.create('inventory-parallel')
        self.supply(login, {'item_062': 1}, coin=100)
        login = self.login('inventory-parallel')
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            results = list(pool.map(lambda _: self.use(login, 'item_062'), range(8)))
        self.assertEqual(sum(op == BY_NAME['SCItemUse'] for op, response in results), 1)
        self.assertEqual(sum(op == 10001 and response.code == '90003'
                             for op, response in results), 7)
        self.assertEqual(self.items(login), {})
        self.assertEqual(self.profile(login).coin, 10100)
        self.stop(); self.start(); login = self.login('inventory-parallel')
        self.assertEqual(self.use(login, 'item_062')[1].code, '90003')
        self.assertEqual(self.profile(login).coin, 10100)

    def test_balance_and_wire_quantity_overflow_roll_back_all_effects(self):
        login = self.create('inventory-overflow')
        for item, num, resources in [('item_001', 1, {'energy': 2147483640}),
                                      ('item_002', 1, {'stamina': 2147483640}),
                                      ('item_062', 1, {'coin': 9223372036854775800}),
                                      ('item_062', 214749, {'coin': 100})]:
            self.supply(login, {item: num}, **resources)
            login = self.login('inventory-overflow')
            before = self.profile(login)
            opcode, error = self.use(login, item, num)
            self.assertEqual((opcode, error.code), (10001, '90003'))
            self.assertEqual(self.profile(login), before)
            self.assertEqual(self.items(login), {item: num})


if __name__ == '__main__':
    unittest.main()
