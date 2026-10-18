"""First-chapter star chests use a real Go process and isolated temporary saves."""
import concurrent.futures
import json
import unittest

from server.codec import BY_NAME
from test_go_equipment import GoEquipmentTests


class GoChapterRewardTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    character = GoEquipmentTests.character
    mutate = GoEquipmentTests.mutate
    equipment = GoEquipmentTests.equipment
    denied = GoEquipmentTests.denied

    def saved(self, login):
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
        return json.loads(row['player']), json.loads(row['game'])

    def stars(self, login, total):
        def apply(player, game):
            # Fixture setup touches only this test's database, never a real account.
            game['copy']['passedCopyInfo'] = [
                {'chapterId': '1', 'missionId': str(n), 'star': max(0, min(5, total - (n - 1) * 5)),
                 'challengeTimes': 0, 'isOpen': 1} for n in range(1, 6)]
        self.mutate(login, apply)

    def claim(self, login, index=0):
        opcode, result = self.send('CSCopyGetStarAward', {'chapterId': '1', 'starIndex': index}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyGetStarAward'], str(result))
        return result.gain

    def test_thresholds_and_request_scope_reject_without_changes(self):
        login = self.character('chapter-reward-threshold')
        for total, index in [(7, 0), (14, 1), (24, 2)]:
            self.stars(login, total)
            before = self.saved(login)
            self.denied(login, 'CSCopyGetStarAward', {'chapterId': '1', 'starIndex': index})
            self.assertEqual(self.saved(login), before)
        for chapter, index in [('2', 0), ('', 0), ('1', -1), ('1', 3)]:
            before = self.saved(login)
            self.denied(login, 'CSCopyGetStarAward', {'chapterId': chapter, 'starIndex': index})
            self.assertEqual(self.saved(login), before)

    def test_all_original_rewards_and_claim_flags_survive_restart(self):
        login = self.character('chapter-reward-all')
        self.stars(login, 25)
        # Boxes have independent star thresholds; no prior-box condition appears in the client/table.
        expected = {2: {'diamond': 90, 'item_005': 150, 'coin': 10000, 'item_001': 2},
                    0: {'diamond': 30, 'item_005': 50},
                    1: {'diamond': 60, 'item_005': 100, 'coin': 6000, 'item_016': 2}}
        guid = None
        for index in [2, 0, 1]:
            gain = self.claim(login, index)
            self.assertEqual({i.id: i.num for i in gain.simps}, expected[index])
            self.assertEqual(len(gain.equips), int(index == 0))
            if index == 0:
                equip = gain.equips[0]
                guid = equip.guid
                self.assertTrue(guid)
                self.assertEqual((equip.equipId, equip.level, equip.coinPrice), ('equip_001', 1, 1000))
                self.assertEqual((equip.refineLv, equip.star, equip.forgeLevel, equip.lastHeroId), (0, 0, 0, ''))
        player, game = self.saved(login)
        self.assertEqual((player['coin'], player['diamond']), (16000, 180))
        self.assertEqual({i['id']: i['num'] for i in game['items']},
                         {'item_005': 300, 'item_001': 2, 'item_016': 2})
        self.assertFalse(game['quests'])
        self.assertEqual(player['noviceGuideSchedule'], 0)
        before = player, game
        self.stop(); self.start(); login = self.login('chapter-reward-all')
        self.assertEqual(self.saved(login), before)
        _, progress = self.send('CSCopyPassedInfo', login=login)
        chapter = next(c for c in progress.passedChapterInfo if c.chapterId == '1')
        self.assertEqual((chapter.star, chapter.reward0Claimed, chapter.reward1Claimed, chapter.reward2Claimed),
                         (25, True, True, True))
        self.assertEqual(set(self.equipment(login)), {guid})
        before = self.saved(login)
        for index in range(3):
            self.denied(login, 'CSCopyGetStarAward', {'chapterId': '1', 'starIndex': index})
        self.assertEqual(self.saved(login), before)

    def test_concurrent_claim_grants_one_owned_instance(self):
        login = self.character('chapter-reward-concurrent')
        self.stars(login, 8)
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.send('CSCopyGetStarAward',
                                {'chapterId': '1', 'starIndex': 0}, login), range(6)))
        success = [reply for opcode, reply in replies if opcode == BY_NAME['SCCopyGetStarAward']]
        failures = [reply for opcode, reply in replies if opcode == 10001]
        self.assertEqual((len(success), len(failures)), (1, 5))
        self.assertTrue(all(error.code == '90003' for error in failures))
        player, game = self.saved(login)
        self.assertEqual(player['diamond'], 30)
        self.assertEqual(game['items'], [{'id': 'item_005', 'num': 50}])
        self.assertEqual(len(game['equipment']), 1)
        self.assertEqual(game['equipment'][0]['guid'], success[0].gain.equips[0].guid)

    def test_overflow_and_corrupt_stars_roll_back_whole_reward(self):
        login = self.character('chapter-reward-rollback')
        self.stars(login, 8)
        self.mutate(login, lambda p, g: g.update(items=[{'id': 'item_005', 'num': 2147483647}]))
        before = self.saved(login)
        self.denied(login, 'CSCopyGetStarAward', {'chapterId': '1', 'starIndex': 0})
        self.assertEqual(self.saved(login), before)
        self.stars(login, 8)
        self.mutate(login, lambda p, g: g['copy']['passedCopyInfo'][0].update(star=6))
        before = self.saved(login)
        self.denied(login, 'CSCopyGetStarAward', {'chapterId': '1', 'starIndex': 0})
        self.assertEqual(self.saved(login), before)
        self.stars(login, 15)
        self.mutate(login, lambda p, g: p.update(coin=9223372036854775807))
        before = self.saved(login)
        self.denied(login, 'CSCopyGetStarAward', {'chapterId': '1', 'starIndex': 1})
        self.assertEqual(self.saved(login), before)
        self.mutate(login, lambda p, g: (p.update(diamond=9223372036854775807), g.update(items=[])))
        before = self.saved(login)
        self.denied(login, 'CSCopyGetStarAward', {'chapterId': '1', 'starIndex': 0})
        self.assertEqual(self.saved(login), before)
        self.mutate(login, lambda p, g: (p.update(diamond=0),
                    g['copy']['passedCopyInfo'].append(dict(g['copy']['passedCopyInfo'][0]))))
        before = self.saved(login)
        self.denied(login, 'CSCopyGetStarAward', {'chapterId': '1', 'starIndex': 0})
        self.assertEqual(self.saved(login), before)

    def test_real_two_mission_clear_reward_equip_and_strengthen(self):
        login = self.character('chapter-reward-play')
        for mission, tasks in [('1', ['1', '4']), ('2', ['5', '7', '8'])]:
            opcode, result = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': mission}, login)
            self.assertEqual(opcode, BY_NAME['SCCopyEnter'], str(result))
            for task in tasks:
                opcode, fight = self.send('CSCopyFightStart',
                        {'chapterId': '1', 'missionId': mission, 'taskId': task}, login)
                self.assertEqual(opcode, BY_NAME['SCCopyFightStart'], str(fight))
                opcode, result = self.send('CSCopyFightEnd',
                        {'fightId': fight.fightId, 'result': 1, 'battlePlayer': fight.battlePlayer}, login)
                self.assertEqual(opcode, BY_NAME['SCCopyFightEnd'], str(result))
            if mission == '1':
                self.denied(login, 'CSCopyGetStarAward', {'chapterId': '1', 'starIndex': 0})
                for quest in ['quest_1', 'quest_2']:
                    self.assertEqual(self.send('CSQuestReward', {'questId': quest}, login)[0], BY_NAME['SCQuestReward'])
        before_player, before_game = self.saved(login)
        equip = self.claim(login).equips[0]
        player, game = self.saved(login)
        self.assertEqual(game['quests'], before_game['quests'])
        self.assertEqual(player['noviceGuideSchedule'], before_player['noviceGuideSchedule'])
        self.assertEqual(player['diamond'], before_player['diamond'] + 30)
        opcode, result = self.send('CSFormationOn', {'id': equip.equipId, 'guid': equip.guid, 'formatPos': 1}, login)
        self.assertEqual(opcode, BY_NAME['SCFormationOn'], str(result))
        opcode, result = self.send('CSEquipStrengthen',
                {'guid': equip.guid, 'times': 1, 'cost': [{'id': 'coin', 'num': 8}]}, login)
        self.assertEqual(opcode, BY_NAME['SCEquipStrengthen'], str(result))
        self.stop(); self.start(); login = self.login('chapter-reward-play')
        self.assertEqual(self.equipment(login)[equip.guid].level, 2)
        _, formation = self.send('CSFormationList', login=login)
        slot = next(s for s in formation.formationList if s.formatPos == 1)
        self.assertIn(equip.guid, [thing.guid for thing in slot.things])


del GoEquipmentTests
