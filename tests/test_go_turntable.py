"""幸运转盘：原 APK 协议、共享 SQL 库存、真实玩家和重启防重，仅使用独立临时库。"""
import concurrent.futures
import http.client
import json
import time
import unittest

from server.codec import BY_NAME, decode_responses, encode_http_request, message
from test_go_equipment import GoEquipmentTests
from test_go_formation import GoFormationTests


class GoTurntableTests(unittest.TestCase):
    setUp = GoEquipmentTests.setUp
    tearDown = GoEquipmentTests.tearDown
    start = GoEquipmentTests.start
    stop = GoEquipmentTests.stop
    send = GoEquipmentTests.send
    login = GoEquipmentTests.login
    mutate = GoEquipmentTests.mutate
    stamped = GoFormationTests.stamped

    def sql(self, text, args=()):
        with self.service.store.connect() as connection:
            return connection.execute(text, args).fetchall()

    def fixture(self, uid='turntable', name='转盘回归', diamond=1000):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': name, 'heroId': 'hero_043'}, login)
        self.mutate(login, lambda p, g: p.update(diamond=diamond))
        return login

    def deterministic(self, resource='item_011', amount=1, stocks=0):
        now = int(time.time()*1000)
        self.sql('UPDATE gameplay_pools SET start_at=?,end_at=?,reward_end_at=?,jackpot_per_million=0 WHERE id=?',
                 (now-60000, now+3600000, now+7200000, 'turntable_20261011'))
        self.sql('UPDATE gameplay_pool_rewards SET resource_id=?,amount=?,weight=1,stock=? WHERE pool_id=?',
                 (resource, amount, stocks, 'turntable_20261011'))
        # Fixed event dates may already have expired when this suite runs in a later month.
        self.sql('UPDATE turntable_globals SET settled=0,final_ranks=NULL')

    def profile(self, login):
        row = self.sql('SELECT player,game FROM profiles WHERE id=?', (login.playerId,))[0]
        return json.loads(row['player']), json.loads(row['game'])

    def snapshot(self):
        tables = ('profiles', 'turntable_globals', 'turntable_states', 'turntable_records')
        return {table: [dict(row) for row in self.sql(f'SELECT * FROM {table} ORDER BY 1,2')] for table in tables}

    def query(self, name, login):
        opcode, reply = self.send(name, login=login)
        self.assertEqual(opcode, BY_NAME['SC'+name[2:]], str(reply))
        return reply

    def frames(self, name, login):
        raw = encode_http_request(BY_NAME[name], message(name), player_id=login.playerId,
                                  zone_id=1, session=bytes(login.sessionKey), timestamp=100)
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=3)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            return decode_responses(response.read())
        finally:
            connection.close()

    def draw(self, login, count=1, stamp=100):
        opcode, reply = self.stamped('CSPlayBigTurn', {'count': count}, login, stamp)
        self.assertEqual(opcode, BY_NAME['SCPlayBigTurn'], str(reply))
        return reply

    def denied(self, login, count=1, stamp=100, code='90003'):
        before = self.snapshot()
        opcode, error = self.stamped('CSPlayBigTurn', {'count': count}, login, stamp)
        self.assertEqual((opcode, error.code), (10001, code), str(error))
        self.assertEqual(self.snapshot(), before)

    def test_original_prices_nine_slots_empty_real_ranking_and_activity_entry(self):
        login = self.fixture(diamond=0)
        self.deterministic()
        info = self.query('CSBigTurnTableInfo', login)
        self.assertEqual([r.id for r in info.rewardItems], list(range(9)))
        self.assertEqual((info.diamondPool, info.score, info.boughtCoins, info.isFree), (10000, 0, 0, 1))
        self.assertGreater(info.endTime, info.serverTime)
        self.assertGreater(info.rewardEndTime, info.endTime)
        self.assertFalse(info.otherPlayerRewards)
        self.assertFalse(self.query('CSBigTurnRank', login).ranks)
        self.assertFalse(self.query('CSBigTurnRecord', login).otherPlayerRewards)
        desc = self.query('CSBigTurnDesc', login)
        self.assertEqual((desc.topPrizeId, desc.advancePrizeId), ('item_008,100', 'item_008,20'))
        self.assertIn('本地服', desc.descriStr)
        entry = self.query('CSActivityList', login)
        self.assertEqual([tab.activityId for tab in entry.activityList], ['bigTurnTable_20261011', 'openServerFund_local_20261011', 'totalLogin_local_20261011'])
        self.sql('UPDATE gameplay_pools SET enabled=0 WHERE id=?', ('turntable_20261011',))
        self.assertNotIn('bigTurnTable_20261011', [v.activityId for v in self.query('CSActivityList', login).activityList])

    def test_free_paid_ten_cooldown_fractional_pool_and_restart_receipt(self):
        login = self.fixture()
        self.deterministic()
        self.assertFalse(self.draw(login).cost)
        paid = self.draw(login, stamp=101)
        self.assertEqual([(v.id, v.num) for v in paid.cost], [('diamond', 50)])
        ten = self.draw(login, count=10, stamp=102)
        self.assertEqual([(v.id, v.num) for v in ten.cost], [('diamond', 488)])
        self.assertEqual(list(ten.idNumStr), ['item_011,1']*10)
        player, game = self.profile(login)
        self.assertEqual((player['coin'], player['diamond']), (120000, 462))
        self.assertEqual(game['items'], [{'id': 'item_011', 'num': 12}])
        self.assertFalse(game['quests'])
        self.assertEqual(player['noviceGuideSchedule'], 0)
        info = self.query('CSBigTurnTableInfo', login)
        self.assertEqual((info.score, info.boughtCoins, info.isFree, info.diamondPool), (12, 120000, 0, 10010))
        self.assertAlmostEqual(info.nextFreeTime-info.serverTime, 86400000, delta=3000)
        self.assertEqual(self.sql('SELECT contribution_remainder FROM turntable_globals')[0][0], 7600)
        self.stop(); self.start(); login = self.login('turntable')
        before = self.snapshot()
        self.assertEqual(self.draw(login, count=10, stamp=102), ten)
        self.assertEqual(self.snapshot(), before)
        self.denied(login, stamp=102)
        self.denied(login, count=10, stamp=101)
        self.assertEqual(len(self.query('CSBigTurnRecord', login).otherPlayerRewards), 12)

    def test_sql_price_changes_rejected_but_score_and_free_rules_are_read_each_time(self):
        login = self.fixture()
        self.deterministic()
        self.sql('UPDATE gameplay_pools SET free_per_day=1,free_seconds=3600 WHERE id=?',
                 ('turntable_20261011',))
        self.sql('UPDATE turntable_rules SET score_per_draw=7')
        self.draw(login)
        self.sql('UPDATE turntable_states SET next_free=0')
        self.assertEqual(self.query('CSBigTurnTableInfo', login).isFree, 0)
        self.sql('UPDATE gameplay_pools SET single_cost=? WHERE id=?', ('diamond,3', 'turntable_20261011'))
        self.denied(login, stamp=101)
        self.sql('UPDATE gameplay_pools SET single_cost=?,ten_cost=? WHERE id=?', ('diamond,50', 'diamond,489', 'turntable_20261011'))
        self.denied(login, count=10, stamp=101)
        self.sql('UPDATE gameplay_pools SET ten_cost=?,coin_per_draw=10001 WHERE id=?', ('diamond,488', 'turntable_20261011'))
        self.denied(login, stamp=101)
        self.sql('UPDATE gameplay_pools SET coin_per_draw=10000 WHERE id=?', ('turntable_20261011',))
        paid = self.draw(login, stamp=101)
        self.assertEqual(paid.cost[0].num, 50)
        self.sql("UPDATE turntable_states SET free_day='2000-01-01',next_free=0")
        self.assertEqual(self.query('CSBigTurnTableInfo', login).isFree, 1)
        self.assertFalse(self.draw(login, stamp=102).cost)
        self.assertEqual(self.query('CSBigTurnTableInfo', login).score, 21)
        self.stop(); self.start(); login = self.login('turntable')
        self.assertEqual(self.sql('SELECT single_cost FROM gameplay_pools WHERE id=?', ('turntable_20261011',))[0][0], 'diamond,50')
        self.assertEqual(self.query('CSBigTurnTableInfo', login).score, 21)

    def test_concurrent_same_timestamp_commits_one_draw_and_one_record(self):
        login = self.fixture()
        self.deterministic()
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            results = list(pool.map(lambda _: self.draw(login), range(6)))
        self.assertTrue(all(result == results[0] for result in results))
        self.assertEqual(self.query('CSBigTurnTableInfo', login).score, 1)
        self.assertEqual(len(self.query('CSBigTurnRecord', login).otherPlayerRewards), 1)
        self.assertEqual(self.profile(login)[0]['coin'], 10000)
        self.assertEqual(self.profile(login)[0]['diamond'], 1000)

    def test_shared_stock_race_and_ten_draw_shortage_roll_back_whole_transaction(self):
        logins = [self.fixture(f'stock-{n}', name=f'真实玩家{n}') for n in range(9)]
        self.deterministic(stocks=1)
        self.denied(logins[0], count=10)
        with concurrent.futures.ThreadPoolExecutor(max_workers=9) as pool:
            results = list(pool.map(lambda login: self.draw(login), logins))
        self.assertEqual({result.rewardItemId for result in results}, set(range(9)))
        info = self.query('CSBigTurnTableInfo', logins[0])
        self.assertEqual([slot.surplus for slot in info.rewardItems], [0]*9)
        self.denied(logins[0], stamp=101)
        records = self.query('CSBigTurnRecord', logins[0]).otherPlayerRewards
        self.assertEqual(len(records), 9)
        self.assertEqual({row.split(';')[1] for row in records}, {f'真实玩家{n}' for n in range(9)})
        self.stop(); self.start(); login = self.login('stock-0')
        self.assertEqual([slot.surplus for slot in self.query('CSBigTurnTableInfo', login).rewardItems], [0]*9)

    def test_jackpot_pays_entire_shared_pool_and_records_real_player(self):
        login = self.fixture(name='大奖;真人,记录')
        self.deterministic()
        self.sql('UPDATE gameplay_pools SET jackpot_per_million=1000000 WHERE id=?', ('turntable_20261011',))
        jackpot = self.draw(login)
        self.assertEqual((jackpot.rewardItemId, jackpot.isDiamondPool), (9, 1))
        self.assertEqual([(s.id, s.num) for s in jackpot.rewards.simps], [('coin', 10000), ('diamond', 10000)])
        self.assertEqual(list(jackpot.idNumStr), ['diamondBigReward,10000'])
        self.assertEqual(self.query('CSBigTurnTableInfo', login).diamondPool, 0)
        self.assertEqual(self.profile(login)[0]['diamond'], 11000)
        paid = self.draw(login, stamp=101)
        self.assertEqual([(s.id, s.num) for s in paid.rewards.simps], [('coin', 10000), ('diamond', 1)])
        record = self.query('CSBigTurnRecord', login).otherPlayerRewards[-1]
        self.assertEqual(record.split(';')[:3], ['0', '大奖 真人 记录', 'diamond,1'])
        self.assertEqual(len(record.split(';')), 4)

    def test_equipment_rewards_have_unique_guids_bag_limit_and_restart_storage(self):
        login = self.fixture()
        self.deterministic(resource='equip_001')
        self.mutate(login, lambda p, g: p.update(equipBagSize=10))
        result = self.draw(login, count=10)
        self.assertEqual(len(result.rewards.equips), 10)
        guids = {equip.guid for equip in result.rewards.equips}
        self.assertEqual(len(guids), 10)
        self.assertTrue(all(len(guid) == 32 for guid in guids))
        self.assertTrue(all(equip.equipId == 'equip_001' and equip.level == 1 for equip in result.rewards.equips))
        self.denied(login, stamp=101)
        self.stop(); self.start(); login = self.login('turntable')
        self.assertEqual({equip.guid for equip in self.query('CSEquipmentTreasureList', login).equipmentList}, guids)
        self.assertEqual(self.draw(login, count=10), result)

    def test_real_ranking_freezes_and_automatically_pays_once_after_end(self):
        alice = self.fixture('alice', name='真人甲')
        bob = self.fixture('bob', name='真人乙')
        idle = self.fixture('idle', name='未参与真人')
        self.deterministic()
        self.draw(alice); self.draw(alice, stamp=101)
        last = self.draw(bob, count=10)
        ranks = self.query('CSBigTurnRank', idle).ranks
        self.assertEqual([(r.pid, r.rank, r.score) for r in ranks], [(bob.playerId, 1, 10), (alice.playerId, 2, 2)])
        now = int(time.time()*1000)
        self.sql('UPDATE gameplay_pools SET start_at=?,end_at=?,reward_end_at=? WHERE id=?',
                 (now-60000, now-1, now+3600000, 'turntable_20261011'))
        frozen = self.query('CSBigTurnRank', idle)
        self.assertEqual(frozen.ranks, ranks)
        self.assertEqual(self.sql('SELECT settled FROM turntable_globals')[0][0], 1)
        self.assertEqual({i['id']: i['num'] for i in self.profile(bob)[1]['items']}, {'item_011': 10, 'item_008': 100})
        self.assertEqual({i['id']: i['num'] for i in self.profile(alice)[1]['items']}, {'item_011': 2, 'item_008': 20})
        self.assertFalse(self.profile(idle)[1]['items'])
        frames = self.frames('CSBigTurnRank', bob)
        self.assertEqual([opcode for opcode, reply in frames], [BY_NAME['SCBigTurnRank'], BY_NAME['SCItemList'], BY_NAME['SCPlayer']])
        self.assertEqual({item.id: item.num for item in frames[1][1].itemList}, {'item_011': 10, 'item_008': 100})
        self.denied(bob, stamp=101)
        self.assertEqual(self.draw(bob, count=10), last)
        before = self.snapshot()
        self.query('CSBigTurnRank', alice)
        self.assertEqual(self.snapshot(), before)
        self.stop(); self.start(); idle = self.login('idle')
        self.assertEqual(self.query('CSBigTurnRank', idle), frozen)
        self.sql('UPDATE gameplay_pools SET reward_end_at=end_at WHERE id=?', ('turntable_20261011',))
        self.assertNotIn('bigTurnTable_20261011', [v.activityId for v in self.query('CSActivityList', idle).activityList])
        self.assertEqual(self.query('CSBigTurnTableInfo', idle).isFree, 0)

    def test_invalid_config_insufficient_currency_closed_activity_and_bad_packets_are_atomic(self):
        login = self.fixture(diamond=0)
        self.deterministic()
        self.draw(login)
        self.denied(login, stamp=101)
        self.denied(login, count=2, stamp=101)
        self.denied(login, stamp=0)
        self.sql('UPDATE gameplay_pool_rewards SET resource_id=? WHERE pool_id=? AND id=8', ('unknown_thing', 'turntable_20261011'))
        self.denied(login, stamp=101)
        self.deterministic()
        self.sql('UPDATE gameplay_pool_rewards SET weight=0 WHERE pool_id=? AND id=8', ('turntable_20261011',))
        self.denied(login, stamp=101)
        self.deterministic()
        now = int(time.time()*1000)
        self.sql('UPDATE gameplay_pools SET start_at=?,end_at=?,reward_end_at=? WHERE id=?',
                 (now+60000, now+3600000, now+7200000, 'turntable_20261011'))
        self.assertEqual(self.query('CSBigTurnTableInfo', login).isFree, 0)
        self.denied(login, stamp=101)

    def test_restart_after_end_settles_offline_winner_without_turntable_query(self):
        login = self.fixture('offline', name='离线真人')
        self.deterministic()
        result = self.draw(login)
        now = int(time.time()*1000)
        self.sql('UPDATE gameplay_pools SET start_at=?,end_at=?,reward_end_at=? WHERE id=?',
                 (now-60000, now-1, now+3600000, 'turntable_20261011'))
        self.stop(); self.start(); login = self.login('offline')
        _, profile = self.send('CSPlayerLogin', login=login)
        self.assertEqual(profile.player.coin, 10000)
        self.assertEqual({i['id']: i['num'] for i in self.profile(login)[1]['items']}, {'item_011': 1, 'item_008': 100})
        self.assertEqual(self.sql('SELECT settled FROM turntable_globals')[0][0], 1)
        self.assertEqual(self.draw(login), result)
        self.stop(); self.start(); login = self.login('offline')
        self.assertEqual({i['id']: i['num'] for i in self.profile(login)[1]['items']}, {'item_011': 1, 'item_008': 100})


del GoEquipmentTests, GoFormationTests
