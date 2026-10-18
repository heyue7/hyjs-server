"""英雄阵容和战术布阵的真实协议回归，仅使用独立临时 SQLite。"""
import concurrent.futures
import http.client
import json
import unittest

from test_go_server import GoServerTests
from server.codec import BY_NAME, decode_responses, encode_http_request, message


class GoFormationTests(unittest.TestCase):
    setUp = GoServerTests.setUp
    tearDown = GoServerTests.tearDown
    start = GoServerTests.start
    stop = GoServerTests.stop
    _unauthed_send = GoServerTests.send
    login = GoServerTests.login

    def send(self, name, fields=None, login=None):
        # 日常操作使用不同原帧时间戳；明确回执重试仍由stamped传入同一值。
        if login is None:
            return self._unauthed_send(name, fields, login)
        self.request_stamp = getattr(self, 'request_stamp', 1700000000000) + 1
        return self.stamped(name, fields or {}, login, self.request_stamp)

    def character(self, uid='formation', *, level=23, stones=1000, scrolls=1000):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '阵容回归', 'heroId': 'hero_047'}, login)
        # 准备本测试账户的角色和材料，保留旧档没有战术字段的迁移形状。
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            player, game = json.loads(row['player']), json.loads(row['game'])
            player.update(level=level, noviceGuideSchedule=7, stamina=100000)
            game['items'] = [{'id': 'item_373', 'num': stones}, {'id': 'item_372', 'num': scrolls}]
            for number, hero in enumerate(('hero_025', 'hero_026', 'hero_027', 'hero_028', 'hero_029', 'hero_exp_001')):
                game['warriors'].append({'id': hero, 'guid': 'owned-' + str(number), 'level': 1, 'talent': 0,
                                         'exp': 0, 'training': {}, 'tempTraining': {}, 'breached': {}, 'awake': {}})
            game['warriors'].append({'id': 'hero_025', 'guid': 'duplicate-hero', 'level': 1, 'talent': 0,
                                     'exp': 0, 'training': {}, 'tempTraining': {}, 'breached': {}, 'awake': {}})
            connection.execute('UPDATE profiles SET player=?,game=? WHERE id=?', (json.dumps(player), json.dumps(game), login.playerId))
        return login, player['heroWarId']

    def snapshot(self, login):
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player,game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            return json.loads(row['player']), json.loads(row['game'])

    def stamped(self, name, fields, login, stamp):
        # 原请求时间戳保留在签名包头中，重复发送同一个包验证事务去重。
        raw = encode_http_request(BY_NAME[name], message(name, **fields), player_id=login.playerId,
                                  zone_id=1, session=bytes(login.sessionKey), timestamp=stamp)
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=3)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            data = response.read()
            self.assertEqual(response.status, 200, data)
            frames = decode_responses(data)
            return frames[0]
        finally:
            connection.close()

    def denied(self, name, login, fields=None, stamp=None):
        before = self.snapshot(login)
        opcode, error = self.send(name, fields, login) if stamp is None else self.stamped(name, fields, login, stamp)
        self.assertEqual((opcode, error.code), (10001, '90003'), str(error))
        self.assertEqual(self.snapshot(login), before)

    def on(self, login, position=2, number=0):
        hero = ('hero_025', 'hero_026', 'hero_027', 'hero_028', 'hero_029', 'hero_exp_001')[number]
        opcode, reply = self.send('CSFormationOn', {'formatPos': position, 'id': hero, 'guid': 'owned-' + str(number)}, login)
        self.assertEqual(opcode, BY_NAME['SCFormationOn'], str(reply))
        self.assertGreater(reply.fightpower, 0)
        return reply

    def unlock(self, login, tactical='2'):
        self.unlock_stamp = getattr(self, 'unlock_stamp', 1700000000000) + 1
        opcode, reply = self.stamped('CSTaticalFormationUnLock', {'tacticalId': tactical}, login, self.unlock_stamp)
        self.assertEqual(opcode, BY_NAME['SCTaticalFormationUnLock'], str(reply))
        return reply

    def formation(self, login):
        opcode, result = self.send('CSFormationList', login=login)
        self.assertEqual(opcode, BY_NAME['SCFormationList'], str(result))
        return result

    def test_legacy_default_and_free_hero_management_survive_restart(self):
        login, main = self.character(level=1)
        result = self.formation(login)
        self.assertEqual((result.tacticalSkillId, result.formationList[0].battlePos), ('1', 3))
        self.assertNotIn('guide_recruit', self.snapshot(login)[1])
        self.assertEqual(self.on(login).formationItem.battlePos, 7)
        before = self.snapshot(login)[0]
        for _ in range(3):
            self.on(login)
        self.assertEqual(len(self.formation(login).formationList), 2)
        self.assertEqual(self.snapshot(login)[0], before)
        self.on(login, number=1)  # 替换伙伴时不要求重复走新手招募。
        _, result = self.send('CSFormationOff', {'formatPos': 2, 'id': 'hero_026', 'guid': 'owned-1'}, login)
        self.assertGreater(result.fightpower, 0)
        _, result = self.send('CSFormationOff', {'formatPos': 2, 'id': 'hero_026', 'guid': 'owned-1'}, login)
        self.assertGreater(result.fightpower, 0)
        self.assertFalse(next(slot for slot in self.formation(login).formationList if slot.formatPos == 2).things)
        self.stop()
        self.start()
        login = self.login('formation')
        current = self.formation(login)
        self.assertEqual((current.tacticalSkillId, current.formationList[0].things[0].guid), ('1', main))
        self.assertTrue(self.snapshot(login)[1]['tactical_formations'])

    def test_ownership_material_main_and_duplicate_guards_are_atomic(self):
        login, main = self.character()
        self.formation(login)
        self.denied('CSFormationOn', login, {'formatPos': 2, 'id': 'hero_025', 'guid': 'foreign'})
        self.denied('CSFormationOn', login, {'formatPos': 2, 'id': 'hero_025', 'guid': 'owned-1'})
        self.denied('CSFormationOn', login, {'formatPos': 2, 'id': 'hero_exp_001', 'guid': 'owned-5'})
        self.denied('CSFormationOff', login, {'formatPos': 1, 'id': 'hero_047', 'guid': main})
        self.on(login)
        self.denied('CSFormationOn', login, {'formatPos': 3, 'id': 'hero_025', 'guid': 'owned-0'})
        self.denied('CSFormationOn', login, {'formatPos': 3, 'id': 'hero_025', 'guid': 'duplicate-hero'})
        self.on(login, number=1)
        self.denied('CSFormationOff', login, {'formatPos': 2, 'id': 'hero_025', 'guid': 'owned-0'})

    def test_native_main_replacement_level_gate_receipt_identity_and_restart(self):
        login, main = self.character('main-replacement', level=11)
        self.formation(login)
        fields = {'formatPos': 1, 'id': 'hero_025', 'guid': 'owned-0'}
        self.denied('CSFormationOn', login, fields, stamp=1800000000100)
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT player FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            player = json.loads(row['player'])
            player['level'] = 12
            connection.execute('UPDATE profiles SET player=? WHERE id=?', (json.dumps(player), login.playerId))
        before_main = next(w for w in self.snapshot(login)[1]['warriors'] if w['guid'] == main)
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            replies = list(pool.map(lambda _: self.stamped('CSFormationOn', fields, login, 1800000000101), range(6)))
        self.assertTrue(all(op == BY_NAME['SCFormationOn'] for op, _ in replies))
        self.assertTrue(all(result == replies[0][1] for _, result in replies))
        saved = self.snapshot(login)
        self.assertEqual(saved[0]['heroWarId'], main)
        self.assertEqual(next(w for w in saved[1]['warriors'] if w['guid'] == main), before_main)
        self.assertEqual(self.formation(login).formationList[0].things[0].guid, 'owned-0')
        self.denied('CSFormationOn', login, {'formatPos': 1, 'id': 'hero_026', 'guid': 'owned-1'}, stamp=1800000000101)
        self.denied('CSWarriorSell', login, {'guid': [main]}, stamp=1800000000102)
        self.stop()
        self.start()
        login = self.login('main-replacement')
        self.assertEqual(self.stamped('CSFormationOn', fields, login, 1800000000101)[1], replies[0][1])
        self.assertEqual(self.snapshot(login), saved)
        opcode, restored = self.stamped('CSFormationOn', {'formatPos': 1, 'id': 'hero_047', 'guid': main}, login, 1800000000103)
        self.assertEqual(opcode, BY_NAME['SCFormationOn'])
        self.assertEqual(restored.formationItem.things[0].guid, main)

    def test_position_level_limits_and_four_person_default_capacity(self):
        for level, position in ((1, 3), (5, 4), (15, 5)):
            login, _ = self.character('level-' + str(level), level=level)
            self.formation(login)
            self.denied('CSFormationOn', login, {'formatPos': position, 'id': 'hero_025', 'guid': 'owned-0'})
        level5, _ = self.character('level5', level=5)
        self.on(level5, 3)
        level15, _ = self.character('level15', level=15)
        self.on(level15, 4)
        login, _ = self.character('capacity')
        self.formation(login)
        self.denied('CSFormationOn', login, {'formatPos': 5, 'id': 'hero_025', 'guid': 'owned-0'})
        self.unlock(login)
        self.send('CSUseTaticalFormation', {'id': '2'}, login)
        self.on(login, 5)
        self.denied('CSUseTaticalFormation', login, {'id': '1'})
        self.send('CSFormationOff', {'formatPos': 5, 'id': 'hero_025', 'guid': 'owned-0'}, login)
        opcode, _ = self.send('CSUseTaticalFormation', {'id': '1'}, login)
        self.assertEqual(opcode, BY_NAME['SCUseTaticalFormation'])

    def test_exchange_preserves_loadout_and_duplicate_packet_does_not_exchange_twice(self):
        login, main = self.character()
        self.on(login)
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            game = json.loads(row['game'])
            game['formation'][0]['things'].append({'pos': 1, 'id': 'equip_001', 'guid': 'main-sword'})
            game['formation'][1]['things'].append({'pos': 2, 'id': 'equip_002', 'guid': 'partner-armor'})
            game['equipment'] = [{'equipId': 'equip_001', 'guid': 'main-sword', 'level': 1, 'lastHeroId': 'hero_047'},
                                 {'equipId': 'equip_002', 'guid': 'partner-armor', 'level': 1, 'lastHeroId': 'hero_025'}]
            connection.execute('UPDATE profiles SET game=? WHERE id=?', (json.dumps(game), login.playerId))
        fields = {'fromPos': 1, 'toPos': 2}
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            replies = list(pool.map(lambda _: self.stamped('CSFormationExChange', fields, login, 1800000000000), range(4)))
        for opcode, result in replies:
            self.assertEqual(opcode, BY_NAME['SCFormationExChange'], str(result))
            self.assertEqual([slot.things[0].guid for slot in result.formationList], ['owned-0', main])
        slots = self.formation(login).formationList
        self.assertEqual([slot.battlePos for slot in slots], [3, 7])
        self.assertEqual([slot.things[1].guid for slot in slots], ['partner-armor', 'main-sword'])
        self.denied('CSFormationExChange', login, {'fromPos': 2, 'toPos': 1}, stamp=1800000000000)
        self.denied('CSFormationExChange', login, fields, stamp=1799999999999)
        self.stop()
        self.start()
        login = self.login('formation')
        self.stamped('CSFormationExChange', fields, login, 1800000000000)
        self.assertEqual(self.formation(login).formationList[1].things[0].guid, main)
        self.stamped('CSFormationExChange', fields, login, 1800000000001)
        self.assertEqual(self.formation(login).formationList[0].things[0].guid, main)

    def test_tactical_gate_query_unlock_and_retry_use_actual_configuration(self):
        early, _ = self.character('early', level=19)
        self.formation(early)
        for name, fields in (('CSTaticalFormationList', {}), ('CSGetTaticalFormation', {'id': '2'}),
                             ('CSTaticalFormationUnLock', {'tacticalId': '2'}), ('CSUseTaticalFormation', {'id': '1'})):
            self.denied(name, early, fields)
        login, _ = self.character(stones=11)
        _, locked = self.send('CSGetTaticalFormation', {'id': '2'}, login)
        self.assertEqual((locked.formation.id, locked.formation.level), ('2', 0))
        self.assertEqual([position.battlePos for position in locked.formation.posInfoList], [3, 7, 8, 9, 13])
        self.denied('CSUseTaticalFormation', login, {'id': '2'})
        self.denied('CSTaticalFormationUnLock', login, {'tacticalId': 'unknown'})
        self.denied('CSGetTaticalFormation', login, {'id': 'unknown'})
        first = self.unlock(login)
        self.assertEqual([(cost.id, cost.num) for cost in first.cost], [('item_373', 1)])
        stamp = self.unlock_stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            repeated = list(pool.map(lambda _: self.stamped('CSTaticalFormationUnLock', {'tacticalId': '2'}, login, stamp), range(4)))
        for opcode, reply in repeated:
            self.assertEqual((opcode, reply), (BY_NAME['SCTaticalFormationUnLock'], first))
        self.assertEqual(self.snapshot(login)[1]['items'][0]['num'], 10)
        self.assertFalse(self.unlock(login).cost)
        self.assertEqual([(cost.id, cost.num) for cost in self.unlock(login, '3').cost], [('item_373', 10)])
        self.denied('CSTaticalFormationUnLock', login, {'tacticalId': '4'})
        _, result = self.send('CSTaticalFormationList', login=login)
        self.assertEqual(list(zip(result.unlockTacticalList, result.tacticalLevelList)), [('1', 1), ('2', 1), ('3', 1)])

    def test_upgrade_exact_cost_concurrent_retry_restart_and_second_level(self):
        login, _ = self.character(stones=50, scrolls=50)
        self.unlock(login)
        self.send('CSUseTaticalFormation', {'id': '2'}, login)
        fields = {'tacticalId': '2'}
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
            replies = list(pool.map(lambda _: self.stamped('CSTaticalFormationLevelUp', fields, login, 1800000000000), range(5)))
        for opcode, reply in replies:
            self.assertEqual(opcode, BY_NAME['SCTaticalFormationLevelUp'], str(reply))
            self.assertEqual(reply.currLevel, 2)
            self.assertEqual([(cost.id, cost.num) for cost in reply.cost], [('item_373', 5), ('item_372', 5)])
        game = self.snapshot(login)[1]
        self.assertEqual([item['num'] for item in game['items']], [44, 45])
        self.assertEqual(self.formation(login).tacticalSkillId, '7')
        self.stop()
        self.start()
        login = self.login('formation')
        self.stamped('CSTaticalFormationLevelUp', fields, login, 1800000000000)
        self.assertEqual([item['num'] for item in self.snapshot(login)[1]['items']], [44, 45])
        self.denied('CSTaticalFormationLevelUp', login, fields, stamp=1799999999999)
        opcode, reply = self.stamped('CSTaticalFormationLevelUp', fields, login, 1800000000001)
        self.assertEqual((opcode, reply.currLevel), (BY_NAME['SCTaticalFormationLevelUp'], 3))
        self.assertEqual([(cost.id, cost.num) for cost in reply.cost], [('item_373', 7), ('item_372', 10)])
        self.assertEqual(self.formation(login).tacticalSkillId, '8')

    def test_upgrade_insufficient_second_material_rolls_back_first_material(self):
        login, _ = self.character(stones=6, scrolls=4)
        self.unlock(login)
        self.denied('CSTaticalFormationLevelUp', login, {'tacticalId': '2'}, stamp=1800000000000)
        self.denied('CSTaticalFormationLevelUp', login, {'tacticalId': '3'}, stamp=1800000000000)
        self.denied('CSTaticalFormationLevelUp', login, {'tacticalId': '1'}, stamp=1800000000000)
        self.assertEqual([item['num'] for item in self.snapshot(login)[1]['items']], [5, 4])

    def test_tactical_mapping_rejects_duplicate_illegal_missing_and_locked_positions(self):
        login, _ = self.character()
        self.on(login)
        self.unlock(login)
        self.send('CSUseTaticalFormation', {'id': '2'}, login)
        for mapping in ('1,3;2,3', '1,3;1,7', '1,3', '1,3;2,2', '1,3;2,7;3,8', '1,3;2,7;', '1,x;2,7'):
            self.denied('CSTaticalFormationChange', login, {'id': '2', 'formation': mapping})
        self.denied('CSTaticalFormationChange', login, {'id': '3', 'formation': '1,3;2,7'})
        opcode, result = self.send('CSTaticalFormationChange', {'id': '2', 'formation': '1,13;2,8'}, login)
        self.assertEqual(opcode, BY_NAME['SCTaticalFormationChange'], str(result))
        self.assertEqual([slot.battlePos for slot in self.formation(login).formationList], [13, 8])
        self.send('CSUseTaticalFormation', {'id': '1'}, login)
        self.assertEqual([slot.battlePos for slot in self.formation(login).formationList], [3, 7])
        self.send('CSUseTaticalFormation', {'id': '2'}, login)
        self.assertEqual([slot.battlePos for slot in self.formation(login).formationList], [13, 8])
        opcode, result = self.send('CSFormationChange', {'fomationMapStr': '1,7;2,3'}, login)
        self.assertEqual(opcode, BY_NAME['SCFormationChange'], str(result))
        self.assertEqual([slot.battlePos for slot in result.formationList], [7, 3])
        self.stop()
        self.start()
        login = self.login('formation')
        _, result = self.send('CSGetTaticalFormation', {'id': '2'}, login)
        self.assertEqual([position.battlePos for position in result.formation.posInfoList[:2]], [7, 3])

    def test_selected_tactical_bonus_and_pose_reach_actual_battle_properties(self):
        login, _ = self.character()
        self.on(login)
        # 准备主干6的伙伴，实际天赋含12%攻击，验证阵型1%攻击以加法合并。
        with self.service.store.connect() as connection:
            row = connection.execute('SELECT game FROM profiles WHERE id=?', (login.playerId,)).fetchone()
            game = json.loads(row['game'])
            next(hero for hero in game['warriors'] if hero['guid'] == 'owned-0')['talent'] = 6
            connection.execute('UPDATE profiles SET game=? WHERE id=?', (json.dumps(game), login.playerId))
        self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        _, before = self.send('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, login)
        baseline = {hero.guid: hero for hero in before.battlePlayer.formation}
        self.unlock(login)
        self.send('CSUseTaticalFormation', {'id': '2'}, login)
        self.stamped('CSTaticalFormationLevelUp', {'tacticalId': '2'}, login, 1800000000000)
        self.send('CSTaticalFormationChange', {'id': '2', 'formation': '1,13;2,8'}, login)
        opcode, after = self.send('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyRunMapInfo'], str(after))
        for hero in after.battlePlayer.formation:
            self.assertAlmostEqual(hero.prop.pdef, baseline[hero.guid].prop.pdef * 1.01)
            self.assertAlmostEqual(hero.prop.mdef, baseline[hero.guid].prop.mdef * 1.01)
            self.assertAlmostEqual(hero.inst.curHp, hero.prop.hp)
        partner = next(hero for hero in after.battlePlayer.formation if hero.guid == 'owned-0')
        self.assertAlmostEqual(partner.prop.att, baseline['owned-0'].prop.att * 1.13 / 1.12)
        self.assertEqual(partner.pose, 8)

    def test_all_tactical_levels_use_original_bonus_text_and_stop_at_table_maximum(self):
        login, _ = self.character()
        self.unlock(login)
        self.send('CSUseTaticalFormation', {'id': '2'}, login)
        self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        for level in range(2, 11):
            opcode, reply = self.stamped('CSTaticalFormationLevelUp', {'tacticalId': '2'}, login, 1800000000000 + level)
            self.assertEqual((opcode, reply.currLevel), (BY_NAME['SCTaticalFormationLevelUp'], level))
            # 六级bonus有尾空格，真实战斗应与其他等级一样可解析。
            opcode, result = self.send('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, login)
            self.assertEqual(opcode, BY_NAME['SCCopyRunMapInfo'], str(result))
        self.assertEqual([item['num'] for item in self.snapshot(login)[1]['items']], [884, 775])
        self.assertEqual(self.formation(login).tacticalSkillId, '15')
        self.denied('CSTaticalFormationLevelUp', login, {'tacticalId': '2'}, stamp=1800000000011)

    def test_active_battle_snapshot_survives_formation_changes_and_demo_helpers_have_unique_poses(self):
        login, main = self.character()
        self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        opcode, saved = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyFightStart'], str(saved))
        saved_bytes = saved.SerializeToString()
        self.on(login)
        self.stamped('CSFormationExChange', {'fromPos': 1, 'toPos': 2}, login, 1800000000000)
        self.unlock(login)
        self.send('CSUseTaticalFormation', {'id': '2'}, login)
        self.stamped('CSTaticalFormationLevelUp', {'tacticalId': '2'}, login, 1800000000001)
        _, resumed = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
        self.assertEqual(resumed.SerializeToString(), saved_bytes)
        self.stop()
        self.start()
        login = self.login('formation')
        _, resumed = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
        self.assertEqual(resumed.SerializeToString(), saved_bytes)
        opcode, completed = self.send('CSCopyFightEnd', {'fightId': saved.fightId, 'result': 1,
                                                         'battlePlayer': saved.battlePlayer}, login)
        self.assertEqual((opcode, completed.isCopyComplete), (BY_NAME['SCCopyFightEnd'], 0), str(completed))
        opcode, demo = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '4'}, login)
        self.assertEqual(opcode, BY_NAME['SCCopyFightStart'], str(demo))
        actors = list(demo.battlePlayer.formation)
        poses = [hero.pose for hero in actors]
        self.assertEqual(len(poses), len(set(poses)), [(hero.guid, hero.pose) for hero in actors])
        self.assertTrue({main, 'owned-0'} <= {hero.guid for hero in actors})
        self.assertEqual({hero.id: hero.pose for hero in actors if hero.guid.startswith('demo-')},
                         {'hero_001': 3, 'hero_007': 7})


del GoServerTests

if __name__ == '__main__':
    unittest.main()
