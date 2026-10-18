"""技能专精：原协议、原表费用/属性/门槛与明确本地初始化规则。"""
import concurrent.futures
import http.client
import os
import unittest

from server.codec import BY_NAME, decode_responses, encode_http_request, message
from test_go_activity import GoActivityTests, MySQLActivityTests


class GoSkillSpecialtyTests(unittest.TestCase):
    setUp = GoActivityTests.setUp
    tearDown = GoActivityTests.tearDown
    start = GoActivityTests.start
    stop = GoActivityTests.stop
    send = GoActivityTests.send
    login = GoActivityTests.login
    mutate = GoActivityTests.mutate
    sql = GoActivityTests.sql
    profile = GoActivityTests.profile

    def fixture(self, uid='specialty', level=75, hero='hero_001'):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '专精回归', 'heroId': hero}, login)
        def prepare(p, g):
            p.update(level=level, stamina=100, energy=100)
            g['items'] = [{'id': f'item_{i}', 'num': 1000} for i in range(439, 455)]
        self.mutate(login, prepare)
        return login

    def request(self, login, kind=1, stamp=100, strength=False, guid=None):
        name = 'CSStrengthRoleSkillRefineLevel' if strength else 'CSUpRoleSkillRefineLevel'
        if guid is None:
            guid = self.profile(login)[0]['heroWarId']
        raw = encode_http_request(BY_NAME[name], message(name, roleGuid=guid, type=kind),
                                  zone_id=1, player_id=login.playerId, session=bytes(login.sessionKey), timestamp=stamp)
        c = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=10)
        try:
            c.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            reply = c.getresponse()
            data = reply.read()
            self.assertEqual(reply.status, 200)
            return decode_responses(data)[0]
        finally:
            c.close()

    def upgraded(self, login, strength=False, **args):
        op, reply = self.request(login, strength=strength, **args)
        self.assertEqual(op, BY_NAME['SCStrengthRoleSkillRefineLevel' if strength else 'SCUpRoleSkillRefineLevel'], str(reply))
        return reply

    def denied(self, login, **args):
        before = self.profile(login)
        op, reply = self.request(login, **args)
        self.assertEqual((op, reply.code), (10001, '90003'), str(reply))
        self.assertEqual(self.profile(login), before)

    def warriors(self, login):
        op, reply = self.send('CSWarriorList', login=login)
        self.assertEqual(op, BY_NAME['SCWarriorList'], str(reply))
        return {w.guid: w for w in reply.warriorList}

    def battle(self, login):
        self.mutate(login, lambda p, g: g.pop('run', None))
        op, entry = self.send('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login)
        self.assertEqual(op, BY_NAME['SCCopyEnter'], str(entry))
        op, fight = self.send('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': '1'}, login)
        self.assertEqual(op, BY_NAME['SCCopyFightStart'], str(fight))
        guid = self.profile(login)[0]['heroWarId']
        return next(w.prop for w in fight.battlePlayer.formation if w.guid == guid)

    def test_queries_concurrent_packet_restart_and_battle_property(self):
        login = self.fixture()
        guid = self.profile(login)[0]['heroWarId']
        initial = self.warriors(login)[guid]
        self.assertEqual([(r.type, r.level, r.strengthLevel) for r in initial.skillRefine], [(1, 1, 1), (2, 1, 1), (3, 1, 1)])
        baseline = self.battle(login)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.request(login), range(8)))
        self.assertTrue(all(op == BY_NAME['SCUpRoleSkillRefineLevel'] for op, _ in replies), str(replies))
        self.assertTrue(all(reply == replies[0][1] for _, reply in replies))
        self.assertEqual([(r.id, r.num) for r in replies[0][1].cost], [('item_439', 20), ('item_440', 15)])
        inventory = {r['id']: r['num'] for r in self.profile(login)[1]['items']}
        self.assertEqual((inventory['item_439'], inventory['item_440']), (980, 985))
        after = self.battle(login)
        self.assertAlmostEqual(after.hp - baseline.hp, 500)
        self.denied(login, stamp=99)
        self.denied(login, stamp=100, kind=2)
        self.stop(); self.start(); login = self.login('specialty')
        self.assertEqual(self.upgraded(login), replies[0][1])
        self.assertEqual(self.warriors(login)[guid].skillRefine[0].level, 2)
        op, reply = self.send('CSWarriorGet', {'guid': [guid]}, login)
        self.assertEqual(op, BY_NAME['SCWarriorGet'], str(reply))
        self.assertEqual(reply.warrior[0].skillRefine[0].level, 2)

    def test_original_quality_owner_and_main_vs_companion_level(self):
        login = self.fixture(level=74)
        self.denied(login)
        self.mutate(login, lambda p, g: p.update(level=75))
        self.upgraded(login)  # 主角Warrior.level仍为1，取真实Player.level。
        self.denied(login, stamp=101, guid='foreign')
        self.denied(login, stamp=101, kind=4)
        self.denied(login, stamp=101, kind=6)
        other = self.fixture(uid='unsupported-quality', hero='hero_043')
        self.denied(other)
        def companion(p, g):
            g['warriors'].append({'id': 'hero_001', 'guid': 'companion', 'level': 74, 'exp': 0, 'talent': 0, 'breached': {'level': 1}})
        self.mutate(login, companion)
        self.denied(login, stamp=101, guid='companion')
        self.mutate(login, lambda p, g: g['warriors'][-1].update(level=75))
        self.upgraded(login, stamp=101, guid='companion')

    def test_upgrade_strength_transition_limits_and_material_rollback(self):
        login = self.fixture()
        self.warriors(login)
        self.mutate(login, lambda p, g: g['warriors'][0]['skillRefine'][0].update(level=10))
        self.denied(login)
        result = self.upgraded(login, strength=True)
        self.assertEqual([(r.id, r.num) for r in result.cost], [('item_441', 90), ('item_442', 10)])
        self.assertEqual((result.skillRefine[0].level, result.skillRefine[0].strengthLevel), (10, 2))
        self.assertEqual(self.upgraded(login, strength=True), result)
        result = self.upgraded(login, stamp=101)
        self.assertEqual((result.skillRefine[0].level, result.skillRefine[0].strengthLevel), (11, 2))
        self.denied(login, stamp=101, strength=True)
        self.mutate(login, lambda p, g: g['items'][0].update(num=19))
        self.denied(login, stamp=102)
        self.mutate(login, lambda p, g: g['items'][0].update(num=1000))
        self.mutate(login, lambda p, g: g['items'].append(dict(g['items'][0])))
        self.denied(login, stamp=102)
        self.mutate(login, lambda p, g: g['items'].pop())
        self.mutate(login, lambda p, g: g['warriors'][0]['skillRefine'][0].update(level=100, strengthLevel=10))
        self.denied(login, stamp=102)
        self.denied(login, stamp=102, strength=True)

    def test_cross_operation_old_receipts_and_same_millisecond_are_rejected(self):
        login = self.fixture()
        self.warriors(login)
        self.mutate(login, lambda p, g: g['warriors'][0]['skillRefine'][0].update(level=9))
        level = self.upgraded(login, stamp=100)
        self.assertEqual((level.skillRefine[0].level, level.skillRefine[0].strengthLevel), (10, 1))
        self.denied(login, stamp=100, strength=True)
        strength = self.upgraded(login, stamp=101, strength=True)
        self.assertEqual((strength.skillRefine[0].level, strength.skillRefine[0].strengthLevel), (10, 2))
        # 后续强化已改变完整专精快照，旧升级包不能再次覆盖原客户端列表。
        self.denied(login, stamp=100)
        self.denied(login, stamp=101)
        before = self.profile(login)
        self.assertEqual(self.upgraded(login, stamp=101, strength=True), strength)
        self.assertEqual(self.profile(login), before)
        inventory = {r['id']: r['num'] for r in before[1]['items']}
        self.assertEqual((inventory['item_439'], inventory['item_440'], inventory['item_441'], inventory['item_442']),
                         (980, 985, 910, 990))
        self.stop(); self.start(); login = self.login('specialty')
        self.denied(login, stamp=100)
        self.assertEqual(self.upgraded(login, stamp=101, strength=True), strength)
        self.assertEqual(self.profile(login), before)
        newest = self.upgraded(login, stamp=102)
        self.assertEqual((newest.skillRefine[0].level, newest.skillRefine[0].strengthLevel), (11, 2))
        self.denied(login, stamp=101, strength=True)
        before = self.profile(login)
        self.assertEqual(self.upgraded(login, stamp=102), newest)
        self.assertEqual(self.profile(login), before)

    def test_explicit_sql_unlock_policy_and_hot_disable(self):
        login = self.fixture()
        op, settings = self.send('CSKVInfo', login=login)
        self.assertEqual(op, BY_NAME['SCKVInfo'], str(settings))
        self.assertEqual({entry.key: entry.value for entry in settings.entry}['skillSpecialtySwitch'], 'on')
        self.warriors(login)
        def ready(p, g):
            for item in g['warriors'][0]['skillRefine']:
                item.update(level=40, strengthLevel=4)
        self.mutate(login, ready)
        self.upgraded(login, strength=True, kind=1)
        self.denied(login, kind=4)
        reply = self.upgraded(login, strength=True, kind=2, stamp=101)
        self.assertEqual([(r.type, r.level, r.strengthLevel) for r in reply.skillRefine if r.type == 4], [(4, 1, 1)])
        self.upgraded(login, kind=4, stamp=102)
        reply = self.upgraded(login, strength=True, kind=3, stamp=103)
        self.assertEqual([(r.type, r.level, r.strengthLevel) for r in reply.skillRefine if r.type == 5], [(5, 1, 1)])
        self.upgraded(login, kind=5, stamp=104)
        self.sql("UPDATE skill_specialty_rules SET enabled=0 WHERE id='default'")
        op, settings = self.send('CSKVInfo', login=login)
        self.assertEqual(op, BY_NAME['SCKVInfo'], str(settings))
        self.assertEqual({entry.key: entry.value for entry in settings.entry}['skillSpecialtySwitch'], 'off')
        self.denied(login, kind=5, stamp=105)
        self.assertTrue(self.warriors(login))
        self.stop(); self.start(); login = self.login('specialty')
        self.denied(login, kind=5, stamp=105)
        self.sql("UPDATE skill_specialty_rules SET enabled=1,unlock_policy='unknown' WHERE id='default'")
        self.denied(login, kind=5, stamp=105)


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_ACTIVITY_TEST') == '1', '需显式启用独立 MySQL 专精回归')
class MySQLSkillSpecialtyTests(GoSkillSpecialtyTests):
    setUp = MySQLActivityTests.setUp
    tearDown = MySQLActivityTests.tearDown
    start = MySQLActivityTests.start
    root_sql = MySQLActivityTests.root_sql
    sql = MySQLActivityTests.sql
    mutate = MySQLActivityTests.mutate


del GoActivityTests, MySQLActivityTests
