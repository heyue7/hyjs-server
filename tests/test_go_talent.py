"""天赋主干、分支和闲置伙伴材料的临时 SQLite 真实协议回归。"""
import concurrent.futures
import json
import unittest

from test_go_server import GoServerTests
from test_go_warrior import GoWarriorTests
from server.codec import BY_NAME


class GoTalentTests(unittest.TestCase):
    setUp = GoServerTests.setUp
    tearDown = GoServerTests.tearDown
    start = GoServerTests.start
    stop = GoServerTests.stop
    send = GoServerTests.send
    login = GoServerTests.login
    snapshot = GoWarriorTests.snapshot
    denied = GoWarriorTests.denied

    def character(self, uid='talent', *, hero='hero_025', tier=4, level=35, donors=3):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '分支回归', 'heroId': 'hero_047'}, login)
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            player, game = json.loads(row['player']), json.loads(row['game'])
            player.update(level=100, coin=1000000, stamina=100000, noviceGuideSchedule=8)
            game['items'] = [{'id': 'item_005', 'num': 20000}, {'id': 'item_047', 'num': 20000}]
            game['warriors'].append({'id': hero, 'guid': 'target', 'level': level, 'talent': tier,
                                     'exp': 0, 'training': {}, 'tempTraining': {}, 'breached': {'level': 1}, 'awake': {}})
            material_id = 'hero_062' if hero in ('hero_069', 'hero_070', 'hero_073') else hero
            # 空突破对象保留旧档形状，查询时归一成客户端 IsZeroHero 所需的1级。
            for index in range(donors):
                game['warriors'].append({'id': material_id, 'guid': 'donor-' + str(index), 'level': 1,
                                         'talent': 0, 'exp': 0, 'training': {}, 'tempTraining': {},
                                         'breached': {}, 'awake': {}})
            game['formation'].append({'formatPos': 2, 'battlePos': 2,
                                      'things': [{'pos': 0, 'id': hero, 'guid': 'target'}]})
            connection.execute('UPDATE profiles SET player=?,game=? WHERE id=?',
                               (json.dumps(player), json.dumps(game), login.playerId))
        return login, player['heroWarId']

    def update(self, login, *, player=None, target=None, donor=None):
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            p, game = json.loads(row['player']), json.loads(row['game'])
            p.update(player or {})
            for hero in game['warriors']:
                if hero['guid'] == 'target':
                    hero.update(target or {})
                if hero['guid'] == 'donor-0':
                    hero.update(donor or {})
            connection.execute('UPDATE profiles SET player=?,game=? WHERE id=?',
                               (json.dumps(p), json.dumps(game), login.playerId))

    def branch(self, *, guid='target', talent='4-1', hero='hero_025', donors=('donor-0',)):
        return {'guid': guid, 'talent': talent,
                'cost': [{'id': hero, 'guid': donor, 'num': 1} for donor in donors]
                        + [{'id': 'item_047', 'num': 500 if talent == '4-1' else 1000}]}

    def battle(self, login, guid):
        _, reply = self.send('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, login)
        return next(hero for hero in reply.battlePlayer.formation if hero.guid == guid)

    def test_branch_consumes_only_configured_number_and_persists_battle_talent(self):
        login, _ = self.character()
        self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        before_actor = self.battle(login, 'target')
        request = self.branch(donors=('donor-0', 'donor-1', 'donor-2'))
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
            replies = list(pool.map(lambda _: self.send('CSWarriorTalent', request, login), range(5)))
        success = [reply for opcode, reply in replies if opcode == BY_NAME['SCWarriorTalent']]
        self.assertEqual(len(success), 1)
        self.assertEqual(sum(opcode == 10001 and reply.code == '90003' for opcode, reply in replies), 4)
        reply = success[0]
        self.assertEqual((reply.guid.talent, list(reply.guid.talentBranch)), (4, ['4-1']))
        self.assertEqual({(cost.id, cost.guid): cost.num for cost in reply.cost},
                         {('item_047', ''): 500, ('hero_025', 'donor-0'): 1})
        player, game = self.snapshot(login)
        self.assertEqual((player['coin'], player['noviceGuideSchedule']), (1000000, 8))
        self.assertEqual(next(item['num'] for item in game['items'] if item['id'] == 'item_047'), 19500)
        self.assertNotIn('donor-0', {hero['guid'] for hero in game['warriors']})
        self.assertTrue({'donor-1', 'donor-2'} <= {hero['guid'] for hero in game['warriors']})
        after_actor = self.battle(login, 'target')
        self.assertEqual(list(after_actor.talentList), list(before_actor.talentList) + ['tal_310'])
        # tal_310 在受伤/闪避事件触发，不能提前写成永久面板加成。
        self.assertEqual(after_actor.prop, before_actor.prop)
        self.stop()
        self.start()
        login = self.login('talent')
        self.denied('CSWarriorTalent', login, request)
        _, heroes = self.send('CSWarriorList', login=login)
        self.assertEqual(list(next(hero for hero in heroes.warriorList if hero.guid == 'target').talentBranch), ['4-1'])
        self.assertIn('tal_310', self.battle(login, 'target').talentList)

    def test_branch_checks_both_preconditions_and_main_role_item_only_cost(self):
        login, main = self.character(tier=3)
        self.denied('CSWarriorTalent', login, self.branch())
        self.update(login, target={'talent': 7})
        self.denied('CSWarriorTalent', login, self.branch(talent='7-1', donors=('donor-0', 'donor-1')))
        opcode, _ = self.send('CSWarriorTalent', self.branch(), login)
        self.assertEqual(opcode, BY_NAME['SCWarriorTalent'])
        opcode, reply = self.send('CSWarriorTalent', self.branch(talent='7-1', donors=('donor-1', 'donor-2')), login)
        self.assertEqual(opcode, BY_NAME['SCWarriorTalent'], str(reply))
        self.assertEqual((reply.guid.talent, list(reply.guid.talentBranch)), (7, ['4-1', '7-1']))
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            game = json.loads(row['game'])
            next(hero for hero in game['warriors'] if hero['guid'] == main).update(talent=4, level=1)
            connection.execute('UPDATE profiles SET game=? WHERE id=?', (json.dumps(game), login.playerId))
        opcode, reply = self.send('CSWarriorTalent', self.branch(guid=main, donors=()), login)
        self.assertEqual(opcode, BY_NAME['SCWarriorTalent'], str(reply))
        self.assertEqual((reply.guid.id, reply.guid.level, reply.guid.talent), ('hero_047', 1, 4))
        self.assertEqual(list(reply.guid.talentBranch), ['4-1'])
        self.assertEqual([(cost.id, cost.num) for cost in reply.cost], [('item_047', 500)])

    def test_trunk_four_uses_partner_level_and_same_hero_material_then_changes_properties(self):
        login, _ = self.character(tier=3, level=34)
        request = {'guid': 'target', 'talent': '4', 'cost': [{'id': 'coin', 'num': 40000},
                   {'id': 'item_005', 'num': 500}, {'id': 'hero_025', 'guid': 'donor-0', 'num': 1}]}
        self.denied('CSWarriorTalent', login, request)  # Player100不能替代伙伴的35级门槛。
        self.update(login, target={'level': 35})
        self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        before = self.battle(login, 'target')
        opcode, reply = self.send('CSWarriorTalent', request, login)
        self.assertEqual(opcode, BY_NAME['SCWarriorTalent'], str(reply))
        self.assertEqual(reply.guid.talent, 4)
        self.assertEqual(self.snapshot(login)[0]['coin'], 960000)
        self.assertNotIn('donor-0', {hero['guid'] for hero in self.snapshot(login)[1]['warriors']})
        after = self.battle(login, 'target')
        self.assertGreater(after.prop.att, before.prop.att)
        self.assertGreater(after.prop.hp, before.prop.hp)
        self.assertIn('tal_63', after.talentList)
        self.denied('CSWarriorTalent', login, request)

    def test_variant_target_validates_actual_material_id_from_guid(self):
        login, _ = self.character(hero='hero_069')
        # 原TalentDetail按目标heroId填写id，GUID所指材料则来自配置hero_062。
        opcode, reply = self.send('CSWarriorTalent', self.branch(hero='hero_069'), login)
        self.assertEqual(opcode, BY_NAME['SCWarriorTalent'], str(reply))
        self.assertEqual([(cost.id, cost.guid) for cost in reply.cost if cost.guid], [('hero_062', 'donor-0')])

    def test_forged_insufficient_foreign_duplicate_active_and_trained_materials_rollback(self):
        login, main = self.character()
        self.denied('CSWarriorTalent', login, self.branch(guid='foreign'))
        self.denied('CSWarriorTalent', login, self.branch(talent='9-1'))
        self.denied('CSWarriorTalent', login, self.branch(donors=()))
        self.denied('CSWarriorTalent', login, self.branch(donors=('foreign',)))
        self.denied('CSWarriorTalent', login, self.branch(donors=('donor-0', 'donor-0')))
        self.denied('CSWarriorTalent', login, self.branch(donors=('target',)))
        self.denied('CSWarriorTalent', login, self.branch(donors=(main,)))
        request = self.branch()
        request['cost'][-1]['num'] = 1
        self.denied('CSWarriorTalent', login, request)
        for growth in ({'level': 2}, {'talent': 1}, {'training': {'att': 1}},
                       {'tempTraining': {'hp': 1}}, {'breached': {'level': 2}}, {'awake': {'stage': 1}}):
            self.update(login, donor=growth)
            self.denied('CSWarriorTalent', login, self.branch())
            self.update(login, donor={'level': 1, 'talent': 0, 'training': {}, 'tempTraining': {},
                                      'breached': {'level': 1}, 'awake': {}})
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            game = json.loads(row['game'])
            game['formation'].append({'formatPos': 3, 'battlePos': 9,
                                      'things': [{'pos': 0, 'id': 'hero_025', 'guid': 'donor-0'}]})
            connection.execute('UPDATE profiles SET game=? WHERE id=?', (json.dumps(game), login.playerId))
        self.denied('CSWarriorTalent', login, self.branch())

    def test_legacy_empty_breach_normalizes_without_overwriting_existing_progress(self):
        login, _ = self.character()
        self.update(login, donor={'breached': {'level': 0, 'exp': 8, 'lastAddExpTime': '123', 'costRecord': 'saved'}})
        opcode, heroes = self.send('CSWarriorList', login=login)
        self.assertEqual(opcode, BY_NAME['SCWarriorList'])
        by_guid = {hero.guid: hero for hero in heroes.warriorList}
        self.assertEqual(by_guid['donor-1'].breached.level, 1)
        self.assertEqual((by_guid['donor-0'].breached.level, by_guid['donor-0'].breached.exp,
                          by_guid['donor-0'].breached.lastAddExpTime, by_guid['donor-0'].breached.costRecord),
                         (0, 8, '123', 'saved'))
        self.denied('CSWarriorTalent', login, self.branch())


del GoServerTests, GoWarriorTests

if __name__ == '__main__':
    unittest.main()
