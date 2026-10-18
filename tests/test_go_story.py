"""第一章主线在真实 Go 进程、独立临时 SQLite 上的协议回归。"""
import concurrent.futures
import csv
import json
import unittest

from test_go_server import GoServerTests
from server.codec import BY_NAME


class GoStoryTests(unittest.TestCase):
    # Reuse the live-process fixture without collecting its inherited tests.
    setUp = GoServerTests.setUp
    tearDown = GoServerTests.tearDown
    start = GoServerTests.start
    stop = GoServerTests.stop
    send = GoServerTests.send
    login = GoServerTests.login

    def character(self, uid='story-test'):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '剧情回归', 'heroId': 'hero_043'}, login)
        return login

    def clear_mission(self, login, mission):
        opcode, entry = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': mission}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyEnter'], str(entry))
        for job in entry.jobList:
            opcode, fight = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': mission, 'taskId': job.taskId}, login)
            self.assertEqual(opcode, BY_NAME['SCCopyFightStart'], str(fight))
            opcode, result = self.send('CSCopyFightEnd', {'fightId': fight.fightId, 'result': 1, 'battlePlayer': fight.battlePlayer}, login)
            self.assertEqual(opcode, BY_NAME['SCCopyFightEnd'], str(result))
        self.assertEqual(result.isCopyComplete, 1)
        self.send('CSCopyLeave', login=login)

    def claim(self, login, quest):
        opcode, reward = self.send('CSQuestReward', {'questId': quest}, login)
        self.assertEqual(opcode, BY_NAME['SCQuestReward'], str(reward))
        return {item.id: item.num for item in reward.get.simps}

    def denied(self, name, login, **fields):
        opcode, error = self.send(name, fields, login)
        self.assertEqual((opcode, error.code), (10001, '90003'))

    def test_dialogue_claim_unlocks_next_mission_once_and_survives_restart(self):
        login = self.character()
        self.denied('CSQuestReward', login, questId='quest_2')
        self.denied('CSQuestReward', login, questId='unknown')
        self.clear_mission(login, '1')
        self.denied('CSQuestReward', login, questId='quest_2')
        self.claim(login, 'quest_1')
        self.denied('CSCopyEnter', login, chapterId='1', missionId='2')
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.send('CSQuestReward', {'questId': 'quest_2'}, login), range(6)))
        successful = [result for opcode, result in replies if opcode == BY_NAME['SCQuestReward']]
        self.assertEqual(len(successful), 1)
        self.assertEqual({item.id: item.num for item in successful[0].get.simps}, {'coin': 50000, 'diamond': 20})
        self.assertEqual(sum(opcode == 10001 and result.code == '90003' for opcode, result in replies), 5)
        self.stop()
        self.start()
        login = self.login('story-test')
        _, quests = self.send('CSQuestList', login=login)
        self.assertEqual(list(quests.questList), ['quest_1', 'quest_2'])
        _, player = self.send('CSPlayerLogin', login=login)
        self.assertEqual((player.player.coin, player.player.diamond), (100760, 20))
        self.assertEqual(player.player.noviceGuideSchedule, 4)
        _, progress = self.send('CSCopyPassedInfo', login=login)
        info = {copy.missionId: copy for copy in progress.passedCopyInfo}
        self.assertEqual((info['1'].star, info['2'].isOpen, progress.currentMissionId), (5, 1, '2'))
        self.denied('CSQuestReward', login, questId='quest_2')
        self.denied('CSQuestReward', login, questId='quest_3')
        self.clear_mission(login, '2')
        self.assertEqual(self.claim(login, 'quest_3'), {'diamond': 20, 'coin': 50000, 'item_373': 6})

    def test_complete_first_chapter_sequence_uses_apk_rewards(self):
        login = self.character('story-chapter')
        with open('analysis/tables/TableStoryQuest.tsv', encoding='utf-8', newline='') as source:
            rows = list(csv.reader(source, delimiter='\t'))
        quests = [dict(zip(rows[1], row)) for row in rows[5:13]]
        schedule_after_claim = {'quest_1': 1, 'quest_2': 4, 'quest_3': 5, 'quest_4': 8, 'quest_5': 9, 'quest_6': 9, 'quest_7': 9, 'quest_8': 9}
        with open('analysis/tables/TableGuide.tsv', encoding='utf-8', newline='') as source:
            guide_rows = list(csv.reader(source, delimiter='\t'))
        guides = {int(row[1]): dict(zip(guide_rows[1], row)) for row in guide_rows[5:] if row[1] != '-1'}
        next_guide = {'quest_2': 'guide_quest_3', 'quest_3': 'guide_talent', 'quest_4': 'guide_quest_5', 'quest_5': 'guide_copy_reward'}
        for mission in ('1', '2', '3', '4', '5'):
            self.clear_mission(login, mission)
            for quest in (row for row in quests if row['missionId'] == mission):
                expected = {key: int(value) for key, value in (part.split(',') for part in quest['rewards'].split(';'))}
                self.assertEqual(self.claim(login, quest['questId']), expected)
                expected_schedule = schedule_after_claim[quest['questId']]
                self.assertEqual(self.last_frames[1][1].player.noviceGuideSchedule, expected_schedule)
                if quest['questId'] in next_guide:
                    # Real process restart must restore the completed guide's login continuation.
                    self.stop()
                    self.start()
                    login = self.login('story-chapter')
                    _, resumed = self.send('CSPlayerLogin', login=login)
                    self.assertEqual(resumed.player.noviceGuideSchedule, expected_schedule)
                    self.assertEqual(guides[resumed.player.noviceGuideSchedule]['loginGuide'], next_guide[quest['questId']])
        _, completed = self.send('CSQuestList', login=login)
        self.assertEqual(list(completed.questList), [row['questId'] for row in quests])
        _, passed = self.send('CSCopyPassedInfo', login=login)
        self.assertEqual([copy.missionId for copy in passed.passedCopyInfo if copy.star > 0], ['1', '2', '3', '4', '5'])
        _, profile = self.send('CSPlayerLogin', login=login)
        self.assertEqual(profile.player.diamond, 140)
        self.assertEqual((profile.player.level, profile.player.exp, profile.player.coin), (6, 0, 404900))
        _, items = self.send('CSItemList', login=login)
        inventory = {item.id: item.num for item in items.itemList}
        self.assertEqual((inventory['item_005'], inventory['item_373']), (50, 11))
        self.assertNotIn('exp', inventory)
        self.denied('CSQuestReward', login, questId='quest_9')

    def test_partial_reward_failure_rolls_back_items_coin_and_quest(self):
        login = self.character('story-rollback')
        self.clear_mission(login, '1')
        # An overflowing final item must roll back the earlier coin and item rewards.
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            state = json.loads(row['game'])
            state['items'] = [{'id': 'item_373', 'num': 2147483647}]
            connection.execute('UPDATE profiles SET game=? WHERE id=?', (json.dumps(state), login.playerId))
        self.denied('CSQuestReward', login, questId='quest_1')
        _, profile = self.send('CSPlayerLogin', login=login)
        self.assertEqual(profile.player.coin, 760)
        _, items = self.send('CSItemList', login=login)
        self.assertEqual({item.id: item.num for item in items.itemList}, {'item_373': 2147483647})
        _, quests = self.send('CSQuestList', login=login)
        self.assertEqual(list(quests.questList), [])


del GoServerTests

if __name__ == '__main__':
    unittest.main()
