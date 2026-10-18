"""装备基础闭环：真实 Go 进程、独立临时 SQLite、原 APK 二进制消息。"""
import concurrent.futures
import json
import os
from pathlib import Path
import re
import selectors
import subprocess
import types
import unittest

from test_go_server import GoServerTests
from server.codec import BY_NAME
from server.store import Store


class GoEquipmentTests(unittest.TestCase):
    setUp = GoServerTests.setUp
    tearDown = GoServerTests.tearDown
    stop = GoServerTests.stop
    send = GoServerTests.send
    login = GoServerTests.login

    def start(self):
        # Allow an isolated build without replacing the shared/live server binary.
        root = Path(__file__).resolve().parents[1]
        binary = os.environ.get('HYJS_GO_TEST_BINARY', str(root / 'build/hyjs-server'))
        self.process = subprocess.Popen(
            [binary, '--db', str(self.path), '--http-port', '0', '--tcp-port', '0'],
            cwd=root / 'hyjs-server', stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
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

    def character(self, uid='equipment-test'):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '装备回归', 'heroId': 'hero_043'}, login)
        return login

    def mutate(self, login, callback):
        # Only this test's temporary database is modified; no real save is touched.
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            player, game = json.loads(row['player']), json.loads(row['game'])
            callback(player, game)
            connection.execute('UPDATE profiles SET player=?,game=? WHERE id=?',
                               (json.dumps(player), json.dumps(game), login.playerId))

    def seed(self, login, level=10, coin=100000, equip_level=1):
        def apply(player, game):
            player.update(level=level, coin=coin)
            game['equipment'] = [
                {'guid': f'equip-{n}', 'equipId': f'equip_{n:03}', 'level': equip_level,
                 'refineLv': 0, 'refineExp': 0, 'star': 0, 'starExp': 0,
                 'lastHeroId': '', 'coinPrice': 1000, 'gems': [], 'forgeLevel': 0}
                for n in range(1, 6)
            ]
        self.mutate(login, apply)

    def equipment(self, login):
        opcode, reply = self.send('CSEquipmentTreasureList', login=login)
        self.assertEqual(opcode, BY_NAME['SCEquipmentTreasureList'], str(reply))
        return {equip.guid: equip for equip in reply.equipmentList}

    def formation(self, login):
        _, reply = self.send('CSFormationList', login=login)
        return {slot.formatPos: slot for slot in reply.formationList}

    def equip(self, login, number, position=1):
        opcode, reply = self.send('CSFormationOn',
                                 {'id': f'equip_{number:03}', 'guid': f'equip-{number}', 'formatPos': position}, login)
        self.assertEqual(opcode, BY_NAME['SCFormationOn'], str(reply))
        return reply

    def denied(self, login, name, fields, code='90003'):
        opcode, error = self.send(name, fields, login)
        self.assertEqual((opcode, error.code), (10001, code), str(error))

    def strengthen(self, login, cost=8, guid='equip-1'):
        return self.send('CSEquipStrengthen', {'guid': guid, 'times': 1,
                                             'cost': [{'id': 'coin', 'num': cost}]}, login)

    def battle(self, login):
        # Discard only the test fixture's active run to sample fresh server attributes.
        self.mutate(login, lambda player, game: game.pop('run', None))
        opcode, entry = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyEnter'], str(entry))
        opcode, fight = self.send('CSCopyFightStart',
                                 {'chapterId': '1', 'missionId': '1', 'taskId': entry.jobList[0].taskId}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyFightStart'], str(fight))
        return fight.battlePlayer.formation[0].prop

    def test_equipment_query_preserves_complete_records_after_restart(self):
        login = self.character()
        self.assertEqual(self.equipment(login), {})
        self.seed(login)
        self.mutate(login, lambda player, game: game['equipment'][0].update(
            refineLv=2, refineExp=13, star=1, starExp=7, lastHeroId='hero_025',
            coinPrice=2048, gems=['gem_001'], forgeLevel=1))
        before = self.equipment(login)['equip-1']
        self.assertEqual((before.refineLv, before.refineExp, before.star, before.starExp,
                          before.coinPrice, list(before.gems), before.forgeLevel),
                         (2, 13, 1, 7, 2048, ['gem_001'], 1))
        self.stop()
        self.start()
        login = self.login('equipment-test')
        self.assertEqual(self.equipment(login)['equip-1'], before)
        self.denied(login, 'CSFormationOn', {'id': 'equip_001', 'guid': 'equip-1', 'formatPos': 1})

    def test_equipment_replace_unmount_and_ownership_are_persistent(self):
        login = self.character()
        self.seed(login)
        self.denied(login, 'CSFormationOn', {'id': 'equip_001', 'guid': 'foreign', 'formatPos': 1})
        self.denied(login, 'CSFormationOn', {'id': 'equip_005', 'guid': 'equip-1', 'formatPos': 1})
        self.denied(login, 'CSFormationOn', {'id': 'equip_001', 'guid': 'equip-1', 'formatPos': 2})
        self.equip(login, 1)
        self.equip(login, 2)
        replacement = self.equip(login, 5)
        self.assertEqual({thing.pos: thing.guid for thing in replacement.formationItem.things if thing.pos > 0},
                         {1: 'equip-5', 2: 'equip-2'})
        # A stale unmount cannot remove the replacement weapon.
        self.denied(login, 'CSFormationOff', {'id': 'equip_001', 'guid': 'equip-1', 'formatPos': 1})
        self.stop()
        self.start()
        login = self.login('equipment-test')
        self.assertEqual(self.equipment(login)['equip-5'].lastHeroId, 'hero_043')
        self.assertEqual({thing.pos: thing.guid for thing in self.formation(login)[1].things if thing.pos > 0},
                         {1: 'equip-5', 2: 'equip-2'})
        opcode, reply = self.send('CSFormationOff', {'id': 'equip_005', 'guid': 'equip-5', 'formatPos': 1}, login)
        self.assertEqual(opcode, BY_NAME['SCFormationOff'], str(reply))
        self.assertEqual([thing.guid for thing in self.formation(login)[1].things if thing.pos > 0], ['equip-2'])
        self.assertEqual(len(self.equipment(login)), 5)

    def test_single_strengthening_replay_charges_once_and_survives_restart(self):
        login = self.character()
        self.seed(login)
        self.equip(login, 1)
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.strengthen(login), range(6)))
        successes = [reply for opcode, reply in replies if opcode == BY_NAME['SCEquipStrengthen']]
        self.assertEqual(len(successes), 1)
        self.assertEqual(sum(opcode == 10001 and reply.code == '90003' for opcode, reply in replies), 5)
        reply = successes[0]
        self.assertEqual((reply.equip.level, reply.costTimes, reply.criTimes), (2, 1, 0))
        self.assertEqual([(cost.id, cost.num) for cost in reply.cost], [('coin', 8)])
        self.stop()
        self.start()
        login = self.login('equipment-test')
        self.assertEqual(self.equipment(login)['equip-1'].level, 2)
        _, player = self.send('CSPlayerLogin', login=login)
        self.assertEqual(player.player.coin, 99992)
        opcode, reply = self.strengthen(login, cost=18)
        self.assertEqual(opcode, BY_NAME['SCEquipStrengthen'], str(reply))
        self.assertEqual(reply.equip.level, 3)

    def test_strengthen_checks_cost_balance_level_and_unsupported_forms(self):
        login = self.character()
        self.seed(login, coin=7)
        self.denied(login, 'CSEquipStrengthen', {'guid': 'equip-1', 'times': 1, 'cost': [{'id': 'coin', 'num': 8}]})
        self.assertEqual(self.equipment(login)['equip-1'].level, 1)
        self.seed(login)
        for fields in (
            {'guid': 'equip-1', 'times': 1, 'cost': [{'id': 'coin', 'num': 1}]},
            {'guid': 'equip-1', 'times': 1, 'cost': [{'id': 'diamond', 'num': 8}]},
            {'guid': 'equip-1', 'times': 1, 'cost': [{'id': 'coin', 'num': 8, 'guid': 'coin-guid'}]},
            {'guid': 'equip-1', 'times': 5, 'cost': [{'id': 'coin', 'num': 172}]},
            {'guid': 'equip-1', 'times': 6, 'cost': [{'id': 'coin', 'num': 279}]},
        ):
            self.denied(login, 'CSEquipStrengthen', fields)
        self.seed(login, level=1, equip_level=2)
        # Original OnStrengthOnce tests currentLv < playerLv*2.2 before adding one.
        opcode, reply = self.strengthen(login, cost=18)
        self.assertEqual(opcode, BY_NAME['SCEquipStrengthen'], str(reply))
        self.assertEqual(reply.equip.level, 3)
        self.denied(login, 'CSEquipStrengthen', {'guid': 'equip-1', 'times': 1, 'cost': [{'id': 'coin', 'num': 26}]})
        _, player = self.send('CSPlayerLogin', login=login)
        self.assertEqual((player.player.coin, self.equipment(login)['equip-1'].level), (99982, 3))
        self.denied(login, 'CSEquipStrengthenOneKey', {'guid': 'equip-1'}, code='90001')

    def test_equipment_set_master_and_level_properties_reach_real_battle(self):
        login = self.character()
        self.seed(login, equip_level=10)
        baseline = self.battle(login)
        for number in range(1, 5):
            self.equip(login, number)
        equipped = self.battle(login)
        # Original equip_001..004, outfit_1 (2/3/4 pieces), type1 master at minLv10.
        self.assertAlmostEqual(equipped.att - baseline.att, 33 + 1.5 * 9 + 33 + 10)
        self.assertAlmostEqual(equipped.hp - baseline.hp, 800 + 40 * 9 + 800)
        self.assertAlmostEqual(equipped.mdef - baseline.mdef, 20 + 9)
        self.assertAlmostEqual(equipped.pdef - baseline.pdef, 20 + 9)
        self.assertAlmostEqual(equipped.damageAdd - baseline.damageAdd, 2)
        self.assertAlmostEqual(equipped.damageReduce - baseline.damageReduce, 2)


del GoServerTests
