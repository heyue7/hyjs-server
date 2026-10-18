"""第1章扫荡：原二进制协议、独立存档、并发回执和日切。

五星记录在隔离夹具中准备；本测试不宣称恢复了原版权威战斗或随机掉落。
"""
import concurrent.futures
import http.client
import os
import time
import unittest

from server.codec import BY_NAME, decode_responses, encode_http_request, message
from test_go_activity import GoActivityTests, MySQLActivityTests


class GoSweepTests(unittest.TestCase):
    setUp = GoActivityTests.setUp
    tearDown = GoActivityTests.tearDown
    start = GoActivityTests.start
    stop = GoActivityTests.stop
    send = GoActivityTests.send
    login = GoActivityTests.login
    mutate = GoActivityTests.mutate
    sql = GoActivityTests.sql
    profile = GoActivityTests.profile

    def fixture(self, uid='sweep', level=14, vip=0, mission='1'):
        login = self.login(uid)
        self.send('CSPlayerCreate', {'playerName': '扫荡回归', 'heroId': 'hero_043'}, login)
        self.send('CSCopyPassedInfo', login=login)

        def prepare(player, game):
            player.update(level=level, vip=vip, exp=0, coin=0, energy=100, stamina=100)
            info = next(i for i in game['copy']['passedCopyInfo'] if i['missionId'] == mission)
            info.update(star=5, challengeTimes=0)
        self.mutate(login, prepare)
        return login

    def sweep(self, login, stamp=100, mission='1', chapter='1'):
        # 每个并发请求独立读取全部帧，避免共享 last_frames 影响断言。
        raw = encode_http_request(
            BY_NAME['CSCopySweep'], message('CSCopySweep', chapterId=chapter, missionId=mission),
            zone_id=1, player_id=login.playerId, session=bytes(login.sessionKey), timestamp=stamp)
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=10)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            data = response.read()
            self.assertEqual(response.status, 200, data)
            return decode_responses(data)
        finally:
            connection.close()

    def accepted(self, frames, mission='1', times=1):
        self.assertEqual([op for op, _ in frames],
                         [BY_NAME['SCCopySweep'], BY_NAME['SCPlayer'], BY_NAME['SCCopyPassedInfo']])
        result, player, progress = [payload for _, payload in frames]
        self.assertEqual([(r.id, r.num) for r in result.cost], [('stamina', 10)])
        self.assertEqual((result.wantedId, result.wantedLevel), ('', 0))
        self.assertFalse(result.petAdds)
        self.assertFalse(result.gain.wars)
        self.assertFalse(result.gain.equips)
        info = next(i for i in progress.passedCopyInfo if i.missionId == mission)
        self.assertEqual((info.star, info.challengeTimes, info.isOpen), (5, times, 1))
        self.assertEqual(progress.passedChapterInfo[0].star, 5)
        return result, player.player

    def denied(self, login, **args):
        before = self.profile(login)
        frames = self.sweep(login, **args)
        self.assertEqual(frames[0][0], 10001, str(frames))
        self.assertEqual(frames[0][1].code, '90003', str(frames))
        self.assertEqual(self.profile(login), before, '失败扫荡修改了隔离存档')

    def test_exact_packet_concurrency_restart_replay_and_conflict(self):
        login = self.fixture()
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.sweep(login), range(8)))
        for frames in replies:
            result, player = self.accepted(frames)
            self.assertEqual(result, replies[0][0][1])
            self.assertEqual((player.coin, player.exp, player.stamina), (2190, 140, 90))
        player, game = self.profile(login)
        self.assertEqual((player['coin'], player['exp'], player['stamina']), (2190, 140, 90))
        self.assertEqual(game['copy_sweep_receipt']['timestamp'], 100)
        self.denied(login, stamp=99)
        self.denied(login, stamp=100, mission='2')
        self.stop()
        self.start()
        login = self.login('sweep')
        self.assertEqual(self.sweep(login)[0][1], replies[0][0][1])
        self.assertEqual(self.profile(login), (player, game))
        result, player = self.accepted(self.sweep(login, stamp=101), times=2)
        self.assertEqual({r.id: r.num for r in result.gain.simps}, {'coin': 2190, 'exp': 140})
        self.assertEqual((player.coin, player.level, player.exp, player.stamina, player.energy),
                         (4380, 15, 0, 100, 110))

    def test_level_vip_stars_stamina_and_active_run_gates(self):
        login = self.fixture(level=13, vip=1)
        self.denied(login)
        self.mutate(login, lambda p, g: p.update(vip=2))
        result, player = self.accepted(self.sweep(login))
        self.assertEqual({r.id: r.num for r in result.gain.simps}, {'coin': 2080, 'exp': 130})
        self.assertEqual((player.level, player.stamina), (13, 90))
        self.mutate(login, lambda p, g: g['copy']['passedCopyInfo'][0].update(star=4))
        self.denied(login, stamp=101)
        self.mutate(login, lambda p, g: g['copy']['passedCopyInfo'][0].update(star=5))
        self.mutate(login, lambda p, g: p.update(stamina=9))
        self.denied(login, stamp=101)
        self.mutate(login, lambda p, g: p.update(stamina=100))
        self.mutate(login, lambda p, g: g.update(run={'chapter': '1', 'mission': '1', 'complete': False}))
        self.denied(login, stamp=101)
        self.mutate(login, lambda p, g: g.pop('run'))
        self.denied(login, stamp=0)
        self.denied(login, stamp=101, mission='999')
        self.denied(login, stamp=101, mission='6', chapter='2')
        self.mutate(login, lambda p, g: g['copy']['passedCopyInfo'][0].update(challengeTimes=99))
        self.denied(login, stamp=101)

    def test_capped_mission_day_reset_and_cross_day_receipt(self):
        login = self.fixture(mission='5')
        for stamp in range(100, 103):
            self.accepted(self.sweep(login, stamp=stamp, mission='5'), mission='5', times=stamp-99)
        self.denied(login, stamp=103, mission='5')
        previous = self.profile(login)[0]
        self.mutate(login, lambda p, g: g.update(copy_challenge_day='2000-01-01'))
        # 新日重发最后一包同步零次数，继续复用昨日增量且不再次发奖。
        self.accepted(self.sweep(login, stamp=102, mission='5'), mission='5', times=0)
        self.assertEqual(self.profile(login)[0], previous)
        self.accepted(self.sweep(login, stamp=103, mission='5'), mission='5', times=1)
        self.assertEqual(self.profile(login)[0]['coin'], previous['coin'] + 2300)
        self.stop()
        self.start()
        login = self.login('sweep')
        self.accepted(self.sweep(login, stamp=103, mission='5'), mission='5', times=1)

    def test_corrupt_records_and_upgrade_overflow_roll_back_and_retry(self):
        login = self.fixture()
        self.mutate(login, lambda p, g: p.update(exp=279, energy=2147483647))
        self.denied(login)
        self.mutate(login, lambda p, g: p.update(energy=100, stamina=2147483647))
        self.denied(login)
        self.mutate(login, lambda p, g: p.update(stamina=100, coin=9223372036854775807))
        self.denied(login)
        self.mutate(login, lambda p, g: p.update(coin=0, exp=9223372036854775807))
        self.denied(login)
        self.mutate(login, lambda p, g: p.update(exp=279))
        self.mutate(login, lambda p, g: g['copy']['passedCopyInfo'].append(None))
        self.denied(login)
        self.mutate(login, lambda p, g: g['copy']['passedCopyInfo'].pop())
        self.mutate(login, lambda p, g: g['copy']['passedCopyInfo'].append(dict(g['copy']['passedCopyInfo'][0])))
        self.denied(login)
        self.mutate(login, lambda p, g: g['copy']['passedCopyInfo'].pop())
        result, player = self.accepted(self.sweep(login))
        self.assertEqual((player.level, player.exp, player.stamina, player.energy), (15, 139, 110, 110))

    def test_reward_cooking_consumes_three_minutes_once_with_sweep(self):
        login = self.fixture(level=18)
        self.mutate(login, lambda p, g: g.update(cooking=[{'id': 'FishItem_037', 'num': 1}]))
        guid = self.profile(login)[0]['heroWarId']
        op, reply = self.send('CSFishUsing', {'costItem': {'id': 'FishItem_037', 'num': 1}, 'heroGuid': guid}, login)
        self.assertEqual(op, BY_NAME['SCFishUsing'], str(reply))
        effect_before = int(self.sql('SELECT end_at FROM cooking_effects WHERE player_id=?', (login.playerId,))[0]['end_at'])
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.sweep(login), range(8)))
        for frames in replies:
            result, player = self.accepted(frames)
            self.assertEqual({r.id: r.num for r in result.gain.simps}, {'coin': 2827, 'exp': 193})
            self.assertEqual((player.coin, player.exp), (2827, 193))
        effect_after = int(self.sql('SELECT end_at FROM cooking_effects WHERE player_id=?', (login.playerId,))[0]['end_at'])
        self.assertEqual(effect_before-effect_after, 3*60000)
        self.mutate(login, lambda p, g: p.update(coin=9223372036854775807))
        self.denied(login, stamp=101)
        self.assertEqual(int(self.sql('SELECT end_at FROM cooking_effects WHERE player_id=?', (login.playerId,))[0]['end_at']), effect_after)
        self.mutate(login, lambda p, g: p.update(coin=2827))
        self.stop(); self.start(); login = self.login('sweep')
        self.assertEqual(self.sweep(login)[0][1], replies[0][0][1])
        self.assertEqual(int(self.sql('SELECT end_at FROM cooking_effects WHERE player_id=?', (login.playerId,))[0]['end_at']), effect_after)
        self.sql('UPDATE cooking_effects SET end_at=? WHERE player_id=?', (int(time.time()*1000)-1, login.playerId))
        result, player = self.accepted(self.sweep(login, stamp=101), times=2)
        self.assertEqual({r.id: r.num for r in result.gain.simps}, {'coin': 2630, 'exp': 180})


@unittest.skipUnless(os.environ.get('HYJS_MYSQL_ACTIVITY_TEST') == '1', '需显式启用独立 MySQL 扫荡回归')
class MySQLSweepTests(GoSweepTests):
    """全部扫荡断言在独立 hyjs_test_activity_* InnoDB 数据库重复执行。"""
    setUp = MySQLActivityTests.setUp
    tearDown = MySQLActivityTests.tearDown
    start = MySQLActivityTests.start
    root_sql = MySQLActivityTests.root_sql
    sql = MySQLActivityTests.sql
    mutate = MySQLActivityTests.mutate


del GoActivityTests, MySQLActivityTests


if __name__ == '__main__':
    unittest.main()
