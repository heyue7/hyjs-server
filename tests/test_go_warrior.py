"""养成协议在独立临时 SQLite 和真实 Go 进程上的回归。"""
import concurrent.futures
import json
import unittest

from test_go_server import GoServerTests
from server.codec import BY_NAME


class GoWarriorTests(unittest.TestCase):
    setUp = GoServerTests.setUp
    tearDown = GoServerTests.tearDown
    start = GoServerTests.start
    stop = GoServerTests.stop
    send = GoServerTests.send
    login = GoServerTests.login

    def character(self, uid='warrior-test', *, schedule=5, coin=20000, stones=50):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '养成回归', 'heroId': 'hero_047'}, login)
        # 只准备本测试的临时存档，把奖励来源与养成事务分别验证。
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            player, game = json.loads(row['player']), json.loads(row['game'])
            player.update(level=3, exp=0, coin=coin, noviceGuideSchedule=schedule)
            game['quests'] = ['quest_1', 'quest_2', 'quest_3']
            game['items'] = [{'id': 'item_005', 'num': stones}]
            game['warriors'].extend([
                {'id': 'hero_025', 'guid': 'partner', 'level': 1, 'exp': 0, 'talent': 0, 'training': {}, 'tempTraining': {}, 'breached': {}, 'awake': {}},
                {'id': 'hero_exp_005', 'guid': 'material', 'level': 1, 'exp': 0, 'talent': 0, 'training': {}, 'tempTraining': {}, 'breached': {}, 'awake': {}},
                {'id': 'hero_exp_001', 'guid': 'spare-material', 'level': 1, 'exp': 0, 'talent': 0, 'training': {}, 'tempTraining': {}, 'breached': {}, 'awake': {}},
                {'id': 'hero_026', 'guid': 'other-active', 'level': 1, 'exp': 0, 'talent': 0, 'training': {}, 'tempTraining': {}, 'breached': {}, 'awake': {}},
            ])
            game['formation'].extend([
                {'formatPos': 2, 'battlePos': 2, 'things': [{'pos': 0, 'id': 'hero_025', 'guid': 'partner'}]},
                {'formatPos': 3, 'battlePos': 3, 'things': [{'pos': 0, 'id': 'hero_026', 'guid': 'other-active'}]},
            ])
            game['guide_recruit'] = {'get': {'wars': [game['warriors'][1]]}}
            connection.execute('UPDATE profiles SET player=?,game=? WHERE id=?', (json.dumps(player), json.dumps(game), login.playerId))
        return login, player['heroWarId']

    def snapshot(self, login):
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            return json.loads(row['player']), json.loads(row['game'])

    def denied(self, name, login, fields):
        before = self.snapshot(login)
        opcode, error = self.send(name, fields, login)
        self.assertEqual((opcode, error.code), (10001, '90003'))
        self.assertEqual(self.snapshot(login), before)

    def talent(self, guid):
        return {'guid': guid, 'talent': '1', 'cost': [{'id': 'coin', 'num': 10000}, {'id': 'item_005', 'num': 50}]}

    def strengthen(self, material='material', material_id='hero_exp_005'):
        # LevelUpPage.SendUpgradeMsg 未填写 num 和金币，这里保留 APK 的真实形状。
        return {'guid': 'partner', 'cost': [{'id': material_id, 'guid': material}]}

    def test_main_talent_uses_player_level_and_survives_retry_restart(self):
        login, guid = self.character()
        opcode, reply = self.send('CSWarriorTalent', self.talent(guid), login)
        self.assertEqual(opcode, BY_NAME['SCWarriorTalent'], str(reply))
        self.assertEqual((reply.guid.id, reply.guid.guid, reply.guid.talent), ('hero_047', guid, 1))
        self.assertEqual({cost.id: cost.num for cost in reply.cost}, {'coin': 10000, 'item_005': 50})
        player, game = self.snapshot(login)
        self.assertEqual((player['level'], player['coin'], player['noviceGuideSchedule']), (3, 10000, 6))
        self.assertEqual(self.last_frames[1][1].player.coin, 10000)
        self.assertEqual(game['items'][0]['num'], 0)
        self.denied('CSWarriorTalent', login, self.talent(guid))
        self.stop()
        self.start()
        login = self.login('warrior-test')
        self.denied('CSWarriorTalent', login, self.talent(guid))
        _, heroes = self.send('CSWarriorList', login=login)
        self.assertEqual(next(hero.talent for hero in heroes.warriorList if hero.guid == guid), 1)

    def test_talent_rejects_foreign_skipped_forged_and_insufficient_costs_atomically(self):
        login, guid = self.character()
        self.denied('CSWarriorTalent', login, self.talent('foreign-guid'))
        fields = self.talent(guid)
        fields['talent'] = '2'
        self.denied('CSWarriorTalent', login, fields)
        fields = self.talent(guid)
        fields['cost'][1]['num'] = 1
        self.denied('CSWarriorTalent', login, fields)
        fields = self.talent('partner')
        fields['cost'][1]['num'] = 30
        self.denied('CSWarriorTalent', login, fields)  # 伙伴实际等级 1，表要求 5。
        poor, poor_guid = self.character('talent-poor', coin=9999)
        self.denied('CSWarriorTalent', poor, self.talent(poor_guid))
        missing, missing_guid = self.character('talent-missing', stones=49)
        self.denied('CSWarriorTalent', missing, self.talent(missing_guid))

    def test_upgrade_computes_coin_caps_level_and_prevents_duplicate_settlement(self):
        login, _ = self.character(schedule=6)
        fields = self.strengthen()
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.send('CSWarriorStrengthen', fields, login), range(6)))
        successful = [reply for opcode, reply in replies if opcode == BY_NAME['SCWarriorStrengthen']]
        self.assertEqual(len(successful), 1)
        self.assertEqual(sum(opcode == 10001 and reply.code == '90003' for opcode, reply in replies), 5)
        reply = successful[0]
        self.assertEqual((reply.guid.id, reply.guid.level, reply.guid.exp), ('hero_025', 3, 245))
        self.assertEqual([(cost.id, cost.guid, cost.num) for cost in reply.cost], [('hero_exp_005', 'material', 1), ('coin', '', 1000)])
        player, game = self.snapshot(login)
        self.assertEqual((player['coin'], player['noviceGuideSchedule']), (19000, 7))
        self.assertNotIn('material', {warrior['guid'] for warrior in game['warriors']})
        self.stop()
        self.start()
        login = self.login('warrior-test')
        self.denied('CSWarriorStrengthen', login, fields)
        _, heroes = self.send('CSWarriorList', login=login)
        partner = next(hero for hero in heroes.warriorList if hero.guid == 'partner')
        self.assertEqual((partner.level, partner.exp), (3, 245))

    def test_upgrade_rejects_foreign_duplicate_active_main_and_insufficient_materials(self):
        login, guid = self.character(schedule=6)
        self.denied('CSWarriorStrengthen', login, self.strengthen('foreign'))
        self.denied('CSWarriorStrengthen', login, self.strengthen(material_id='hero_exp_006'))
        self.denied('CSWarriorStrengthen', login, {'guid': 'partner', 'cost': [{'id': 'hero_exp_005', 'guid': 'material'}] * 2})
        self.denied('CSWarriorStrengthen', login, self.strengthen('other-active', 'hero_026'))
        self.denied('CSWarriorStrengthen', login, self.strengthen(guid, 'hero_047'))
        self.denied('CSWarriorStrengthen', login, self.strengthen('partner', 'hero_025'))
        fields = self.strengthen()
        fields['cost'][0]['num'] = 2
        self.denied('CSWarriorStrengthen', login, fields)
        self.denied('CSWarriorStrengthen', login, {'guid': 'material', 'cost': [{'id': 'hero_exp_001', 'guid': 'spare-material'}]})
        self.denied('CSFormationOn', login, {'formatPos': 2, 'id': 'hero_exp_005', 'guid': 'material'})
        poor, _ = self.character('upgrade-poor', schedule=6, coin=999)
        self.denied('CSWarriorStrengthen', poor, self.strengthen())

    def test_material_experience_and_exact_threshold_match_apk_calculation(self):
        login, _ = self.character(schedule=6)
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            player, game = json.loads(row['player']), json.loads(row['game'])
            player['level'] = 5
            material = next(hero for hero in game['warriors'] if hero['guid'] == 'material')
            material.update(id='hero_exp_001')  # 表 expBase=400。
            partner = next(hero for hero in game['warriors'] if hero['guid'] == 'partner')
            partner.update(level=2, exp=5)
            connection.execute('UPDATE profiles SET player=?,game=? WHERE id=?', (json.dumps(player), json.dumps(game), login.playerId))
        opcode, result = self.send('CSWarriorStrengthen', self.strengthen(material_id='hero_exp_001'), login)
        self.assertEqual(opcode, BY_NAME['SCWarriorStrengthen'], str(result))
        self.assertEqual((result.guid.level, result.guid.exp), (3, 245))  # 5+400-160；恰等于 3→4 的门槛仍为 3。
        self.assertEqual(next(cost.num for cost in result.cost if cost.id == 'coin'), 400)

    def test_upgraded_material_returns_accumulated_experience_and_coin(self):
        login, _ = self.character(schedule=6)
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            game = json.loads(row['game'])
            material = next(hero for hero in game['warriors'] if hero['guid'] == 'material')
            material.update(id='hero_025', level=2, exp=5)
            connection.execute('UPDATE profiles SET game=? WHERE id=?', (json.dumps(game), login.playerId))
        opcode, result = self.send('CSWarriorStrengthen', self.strengthen(material_id='hero_025'), login)
        self.assertEqual(opcode, BY_NAME['SCWarriorStrengthen'], str(result))
        # expBase1000 + 已升到等级2的80经验 + 当前5经验；金币比例配置为1。
        self.assertEqual(next(cost.num for cost in result.cost if cost.id == 'coin'), 1085)
        self.assertEqual((result.guid.level, result.guid.exp), (3, 245))
        player, game = self.snapshot(login)
        self.assertEqual(player['coin'], 18915)
        self.assertNotIn('material', {hero['guid'] for hero in game['warriors']})
        self.denied('CSWarriorStrengthen', login, self.strengthen('spare-material', 'hero_exp_001'))


del GoServerTests

if __name__ == '__main__':
    unittest.main()
