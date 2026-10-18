"""UIHero-derived growth in battle packets, using an isolated real Go server."""
import csv
import unittest
from pathlib import Path

from server.codec import BY_NAME
from test_go_chapter import GoChapterTests


class GoBattleStatsTests(unittest.TestCase):
    setUp = GoChapterTests.setUp
    tearDown = GoChapterTests.tearDown
    start = GoChapterTests.start
    stop = GoChapterTests.stop
    send = GoChapterTests.send
    login = GoChapterTests.login
    create = GoChapterTests.create
    profile = GoChapterTests.profile
    enter = GoChapterTests.enter
    mutate = GoChapterTests.mutate

    def set_party(self, login, main_level, main_talent, partner_level, partner_talent,
                  training=None):
        def update(player, game):
            player['level'] = main_level
            main = next(w for w in game['warriors'] if w['guid'] == player['heroWarId'])
            # The main hero's persisted Warrior.level must not override Player.level.
            main.update(level=1, talent=main_talent)
            if training:
                main['training'] = training
                main['tempTraining'] = dict.fromkeys(training, 999)
            partner = {'id': 'hero_025', 'guid': 'battle-partner', 'level': partner_level,
                       'exp': 0, 'talent': partner_talent, 'training': {},
                       'tempTraining': {}, 'breached': {}, 'awake': {}}
            game['warriors'].append(partner)
            game['formation'].append({'formatPos': 2, 'battlePos': 2,
                                      'things': [{'pos': 0, 'id': partner['id'], 'guid': partner['guid']}]})
        self.mutate(login, update)

    def begin(self, login):
        opcode, fight = self.send('CSCopyFightStart',
                                  {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyFightStart'], fight)
        return fight

    def assert_actor(self, actor, level, talents, **stats):
        self.assertEqual(actor.level, level)
        self.assertEqual(list(actor.talentList), talents)
        for key, expected in stats.items():
            self.assertAlmostEqual(getattr(actor.prop, key), expected, msg=key)
        self.assertEqual(actor.inst.curHp, actor.prop.hp)

    def test_main_and_partner_levels_apply_current_trunk_and_passive_talents(self):
        login = self.create('battle-growth')
        self.set_party(login, 5, 1, 5, 1)
        login = self.login('battle-growth')
        self.enter(login, '1')
        fight = self.begin(login)
        actors = {h.id: h for h in fight.battlePlayer.formation}
        # Hero bases + current tier growth * 4 + cumulative ConstAdd + Forever self Buff.
        self.assert_actor(actors['hero_043'], 5, ['tal_1'],
                          hp=1518, att=129, pdef=46, mdef=46)
        self.assert_actor(actors['hero_025'], 5, ['tal_8'],
                          hp=2520, att=84, pdef=47, mdef=56)
        # CopyEnemy remains authoritative for monsters; hero growth is party-only.
        table_path = Path(__file__).resolve().parents[1] / 'analysis/tables/TableCopyEnemy.tsv'
        with table_path.open() as source:
            rows = list(csv.reader(source, delimiter='\t'))
        enemies = [dict(zip(rows[1], row)) for row in rows[5:]]
        for group in fight.battleMonsterList:
            for actor in group.formation:
                row = next(e for e in enemies if 'monster-' + e['CopyEnemyId'] == actor.guid)
                self.assertEqual(actor.prop.hp, float(row['Hp']))
                self.assertEqual(actor.prop.att, float(row['Attack']))

    def test_cumulative_talents_current_growth_training_and_saved_fight_replay(self):
        login = self.create('battle-cumulative')
        self.set_party(login, 15, 2, 15, 2,
                       {'hp': 11, 'att': 3, 'pdef': 4, 'mdef': 5})
        login = self.login('battle-cumulative')
        self.enter(login, '1')
        fight = self.begin(login)
        actors = {h.id: h for h in fight.battlePlayer.formation}
        self.assert_actor(actors['hero_043'], 15, ['tal_1', 'tal_26'],
                          hp=2010, att=172, pdef=76, mdef=77, dodge=15)
        self.assert_actor(actors['hero_025'], 15, ['tal_8', 'tal_17'],
                          hp=3640, att=125, pdef=73, mdef=83, antiCri=10)
        # Persisted FightStart retains its original properties through profile edits/restart.
        self.mutate(login, lambda p, g: p.update(level=16))
        login = self.login('battle-cumulative')
        self.assertEqual(self.begin(login), fight)
        self.send('CSCopyLeave', {'chapterId': '1', 'missionId': '1'}, login)
        self.enter(login, '1')
        updated = self.begin(login)
        main = next(h for h in updated.battlePlayer.formation if h.id == 'hero_043')
        self.assert_actor(main, 16, ['tal_1', 'tal_26'],
                          hp=2045, att=175, pdef=78, mdef=79, dodge=15)


del GoChapterTests


if __name__ == '__main__':
    unittest.main()
