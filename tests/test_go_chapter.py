"""Chapter-one progression checks against a real Go process and Python wire codec."""
import concurrent.futures
import json
import unittest

from server.codec import BY_NAME, message
from test_go_server import GoServerTests


class GoChapterTests(unittest.TestCase):
    # Reuse transport and isolated database helpers without inheriting unrelated tests.
    setUp = GoServerTests.setUp
    tearDown = GoServerTests.tearDown
    start = GoServerTests.start
    stop = GoServerTests.stop
    send = GoServerTests.send
    login = GoServerTests.login

    def create(self, uid):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '章节测试', 'heroId': 'hero_043'}, login)
        return login

    def profile(self, login):
        return self.send('CSPlayerLogin', login=login)[1].player

    def enter(self, login, mission):
        opcode, result = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': mission}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyEnter'], result)
        self.assertEqual(self.last_frames[1][0], BY_NAME['SCPlayer'])
        return result

    def finish(self, login, mission, task, parallel=False):
        opcode, fight = self.send('CSCopyFightStart',
                                  {'chapterId': '1', 'missionId': mission, 'taskId': task}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyFightStart'], fight)
        fields = {'fightId': fight.fightId, 'result': 1, 'battlePlayer': fight.battlePlayer}
        if parallel:
            with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
                replies = list(pool.map(lambda _: self.send('CSCopyFightEnd', fields, login), range(6)))
            for opcode, result in replies:
                self.assertEqual(opcode, BY_NAME['SCCopyFightEnd'], result)
                self.assertEqual(result, replies[0][1])
            return fight, replies[0][1]
        opcode, result = self.send('CSCopyFightEnd', fields, login)
        self.assertEqual(opcode, BY_NAME['SCCopyFightEnd'], result)
        return fight, result

    def claim(self, login, quest):
        opcode, result = self.send('CSQuestReward', {'questId': quest}, login)
        self.assertEqual(opcode, BY_NAME['SCQuestReward'], result)

    def mutate(self, login, update):
        self.stop()
        with self.service.store.connect() as conn:
            row = conn.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            player, game = json.loads(row['player']), json.loads(row['game'])
            update(player, game)
            conn.execute('UPDATE profiles SET player=?,game=? WHERE id=?',
                         (json.dumps(player), json.dumps(game), login.playerId))
        self.start()

    def test_locked_requests_and_insufficient_stamina_roll_back(self):
        login = self.create('chapter-lock')
        before = self.profile(login)
        for chapter, mission in [('1', '2'), ('1', '5'), ('2', '6'), ('1', '999')]:
            opcode, denied = self.send('CSCopyEnter', {'chapterId': chapter, 'missionId': mission}, login)
            self.assertEqual((opcode, denied.code), (10001, '90003'))
        self.assertEqual(self.profile(login), before)
        self.mutate(login, lambda p, g: p.update(stamina=9))
        login = self.login('chapter-lock')
        opcode, denied = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        self.assertEqual((opcode, denied.code), (10001, '90003'))
        self.assertEqual(self.profile(login).stamina, 9)
        with self.service.store.connect() as conn:
            game = json.loads(conn.execute('SELECT game FROM profiles WHERE id=?', (login.playerId,)).fetchone()[0])
        self.assertIsNone(game.get('run'))

    def test_enter_retry_loss_and_restart_keep_one_stamina_charge(self):
        login = self.create('chapter-resume')
        # Loading five CopySimpleInfo entries must not suppress the first-clear helper lineup.
        self.send('CSCopyPassedInfo', login=login)
        entry = self.enter(login, '1')
        self.assertEqual(self.profile(login).stamina, 90)
        self.assertEqual(self.enter(login, '1'), entry)
        _, first = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
        invalid = {'fightId': first.fightId, 'result': 2, 'battlePlayer': first.battlePlayer}
        self.assertEqual(self.send('CSCopyFightEnd', invalid, login)[1].code, '90003')
        defeat = message('BattlePlayer'); defeat.CopyFrom(first.battlePlayer)
        for hero in defeat.formation:
            hero.inst.curHp = 0
        _, loss = self.send('CSCopyFightEnd', {'fightId': first.fightId, 'result': 0, 'battlePlayer': defeat}, login)
        self.assertEqual(loss.isCopyComplete, 0)
        self.finish(login, '1', '1')
        self.stop(); self.start(); login = self.login('chapter-resume')
        entry = self.enter(login, '1')
        self.assertEqual([job.taskId for job in entry.jobList], ['4'])
        self.assertEqual(self.profile(login).stamina, 90)
        fight, result = self.finish(login, '1', '4', parallel=True)
        self.assertEqual({h.id for h in fight.battlePlayer.formation}, {'hero_043', 'hero_001', 'hero_007'})
        self.assertEqual((result.isCopyComplete, result.star), (1, 5))
        self.assertEqual((self.profile(login).coin, self.profile(login).stamina), (760, 100))
        self.enter(login, '1')
        self.finish(login, '1', '1')
        _, boss = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '4'}, login)
        self.assertEqual([h.id for h in boss.battlePlayer.formation], ['hero_043'])
        self.assertEqual(self.profile(login).stamina, 90)

    def test_all_first_chapter_records_unlock_and_survive_restart(self):
        login = self.create('chapter-complete')
        tasks = {'1': ['1', '4'], '2': ['5', '7', '8'], '3': ['9', '10', '12'],
                 '4': ['13', '15', '16'], '5': ['17', '19', '20']}
        for number, (mission, task_ids) in enumerate(tasks.items(), 1):
            entry = self.enter(login, mission)
            self.assertEqual([job.taskId for job in entry.jobList], task_ids)
            # Each encounter is ordered; a future task cannot be selected directly.
            _, denied = self.send('CSCopyFightStart',
                                  {'chapterId': '1', 'missionId': mission, 'taskId': task_ids[-1]}, login)
            self.assertEqual(denied.code, '90003')
            for task in task_ids:
                fight, result = self.finish(login, mission, task, parallel=(mission == '2' and task == '8'))
                self.assertTrue(fight.battleMonsterList)
                self.assertFalse(any(h.id == 'hero_exp_006' for h in fight.battlePlayer.formation))
                self.assertEqual(result.isCopyComplete, int(task == task_ids[-1]))
                if task == task_ids[-1]:
                    self.assertEqual(len(result.gain.wars), 1 if mission == '1' else 2)
                    self.assertTrue(all(w.id == 'hero_exp_006' for w in result.gain.wars))
            _, progress = self.send('CSCopyPassedInfo', login=login)
            passed = {info.missionId: info for info in progress.passedCopyInfo}
            self.assertEqual({k for k, v in passed.items() if v.star > 0}, set(list(tasks)[:number]))
            self.assertEqual(progress.passedChapterInfo[0].star, number * 5)
            self.assertTrue(all(passed[k].challengeTimes == 1 for k in list(tasks)[:number]))
            if number <= 3:
                # OpenId requires the following dialogue quest, even after mission completion.
                _, denied = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': str(number + 1)}, login)
                self.assertEqual(denied.code, '90003')
                self.claim(login, 'quest_' + str(number * 2 - 1))
                self.claim(login, 'quest_' + str(number * 2))
            elif number == 4:
                self.claim(login, 'quest_7')
            else:
                self.claim(login, 'quest_8')
        before = self.profile(login)
        self.stop(); self.start(); login = self.login('chapter-complete')
        self.assertEqual(self.profile(login), before)
        _, progress = self.send('CSCopyPassedInfo', login=login)
        self.assertEqual(progress.currentMissionId, '5')
        _, warriors = self.send('CSWarriorList', login=login)
        self.assertEqual(len([w for w in warriors.warriorList if w.id == 'hero_exp_006']), 9)
        self.assertEqual([(i.missionId, i.star, i.challengeTimes) for i in progress.passedCopyInfo],
                         [(str(i), 5, 1) for i in range(1, 6)])
        # A completed capped mission refuses a new run and atomically keeps stamina.
        def cap(player, game):
            next(i for i in game['copy']['passedCopyInfo'] if i['missionId'] == '5')['challengeTimes'] = 3
        self.mutate(login, cap); login = self.login('chapter-complete')
        stamina = self.profile(login).stamina
        _, denied = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '5'}, login)
        self.assertEqual(denied.code, '90003')
        self.assertEqual(self.profile(login).stamina, stamina)
        # The date changes reset counts while retaining completed missions and stars.
        self.mutate(login, lambda p, g: g.update(copy_challenge_day='2000-01-01'))
        login = self.login('chapter-complete')
        _, progress = self.send('CSCopyPassedInfo', login=login)
        self.assertEqual(progress.passedChapterInfo[0].star, 25)
        self.assertTrue(all(i.challengeTimes == 0 for i in progress.passedCopyInfo))
        self.enter(login, '5')
        self.assertEqual(self.profile(login).stamina, stamina - 10)

    def test_guaranteed_material_drop_is_saved_and_final_gain_replays_once(self):
        login = self.create('chapter-material')
        self.enter(login, '1')
        _, first = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
        defeat = message('BattlePlayer'); defeat.CopyFrom(first.battlePlayer)
        for hero in defeat.formation:
            hero.inst.curHp = 0
        _, loss = self.send('CSCopyFightEnd', {'fightId': first.fightId, 'result': 0, 'battlePlayer': defeat}, login)
        self.assertEqual(list(loss.gain.wars), [])
        self.assertEqual(list(loss.currentResource), [])
        _, won = self.finish(login, '1', '1', parallel=True)
        self.assertEqual(len(won.gain.wars), 1)
        material = won.gain.wars[0]
        self.assertEqual((material.id, material.level, material.exp, material.talent), ('hero_exp_006', 1, 0, 0))
        self.assertEqual([(r.id, r.guid, r.num) for r in won.currentResource], [('hero_exp_006', material.guid, 1)])
        _, warriors = self.send('CSWarriorList', login=login)
        self.assertEqual([w for w in warriors.warriorList if w.id == 'hero_exp_006'], [material])
        _, items = self.send('CSItemList', login=login)
        self.assertFalse(any(i.id == 'hero_exp_006' for i in items.itemList))
        self.stop(); self.start(); login = self.login('chapter-material')
        self.enter(login, '1')
        fight, complete = self.finish(login, '1', '4', parallel=True)
        # Final gain includes the persisted earlier encounter; unsupported Boss probabilities stay absent.
        self.assertEqual(list(complete.gain.wars), [material])
        self.assertEqual(list(complete.currentResource), [])
        self.assertEqual({i.id: i.num for i in complete.gain.simps}, {'coin': 760, 'exp': 10})
        self.stop(); self.start(); login = self.login('chapter-material')
        fields = {'fightId': fight.fightId, 'result': 1, 'battlePlayer': fight.battlePlayer}
        _, replay = self.send('CSCopyFightEnd', fields, login)
        self.assertEqual(replay, complete)
        _, warriors = self.send('CSWarriorList', login=login)
        self.assertEqual([w for w in warriors.warriorList if w.id == 'hero_exp_006'], [material])

    def test_leaving_unfinished_mission_does_not_repeat_first_drop(self):
        login = self.create('chapter-first-drop')
        self.enter(login, '1')
        _, won = self.finish(login, '1', '1')
        self.assertEqual(len(won.gain.wars), 1)
        self.send('CSCopyLeave', {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
        self.stop(); self.start(); login = self.login('chapter-first-drop')
        self.enter(login, '1')
        _, repeated = self.finish(login, '1', '1')
        self.assertEqual(list(repeated.gain.wars), [])
        self.assertEqual(list(repeated.currentResource), [])
        _, warriors = self.send('CSWarriorList', login=login)
        self.assertEqual(len([w for w in warriors.warriorList if w.id == 'hero_exp_006']), 1)

    def test_material_bag_limit_rolls_back_fight_and_can_retry_same_id(self):
        login = self.create('chapter-material-cap')
        self.mutate(login, lambda p, g: p.update(warriorBagSize=1))
        login = self.login('chapter-material-cap')
        self.enter(login, '1')
        _, fight = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
        fields = {'fightId': fight.fightId, 'result': 1, 'battlePlayer': fight.battlePlayer}
        _, denied = self.send('CSCopyFightEnd', fields, login)
        self.assertEqual(denied.code, '90003')
        with self.service.store.connect() as conn:
            game = json.loads(conn.execute('SELECT game FROM profiles WHERE id=?', (login.playerId,)).fetchone()[0])
        self.assertEqual(game['run']['passed'], [])
        self.assertFalse(game['run']['fight']['settled'])
        self.assertNotIn('1', game.get('copy_first_passed_tasks', []))
        self.assertEqual(len(game['warriors']), 1)
        self.mutate(login, lambda p, g: p.update(warriorBagSize=2))
        login = self.login('chapter-material-cap')
        _, won = self.send('CSCopyFightEnd', fields, login)
        self.assertEqual(len(won.gain.wars), 1)
        self.assertEqual(self.profile(login).stamina, 90)

    def test_already_completed_legacy_mission_does_not_retroactively_drop(self):
        login = self.create('chapter-old-material')
        self.send('CSCopyPassedInfo', login=login)
        def legacy(player, game):
            player.update(level=2, exp=0)
            next(i for i in game['copy']['passedCopyInfo'] if i['missionId'] == '1')['star'] = 5
        self.mutate(login, legacy); login = self.login('chapter-old-material')
        self.enter(login, '1')
        _, won = self.finish(login, '1', '1')
        self.assertEqual(list(won.gain.wars), [])
        self.assertEqual(list(won.currentResource), [])
        _, warriors = self.send('CSWarriorList', login=login)
        self.assertFalse(any(w.id == 'hero_exp_006' for w in warriors.warriorList))


del GoServerTests

if __name__ == '__main__':
    unittest.main()
