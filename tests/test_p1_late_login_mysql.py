"""Actual original login -> late activity -> concurrent/cross-entry claims -> cold login."""
import concurrent.futures
import secrets
import time
import unittest

from server.codec import BY_NAME
from test_p1_mysql import P1MySQLTests


class P1LateLoginMySQLTests(P1MySQLTests):
    def test_late_period_real_login_and_single_claim(self):
        self.start()
        uid = 'late-login-' + secrets.token_hex(4)
        login = self.login(uid)
        self.assertEqual(self.send('CSPlayerCreate', {'playerName': '迟到活动回归', 'heroId': 'hero_043'}, login)[0], BY_NAME['SCPlayerCreate'])
        self.assertEqual(self.send('CSPlayerLogin', login=login)[0], BY_NAME['SCPlayerLogin'])
        now = int(time.time() * 1000)
        key = 'celebrationActivity_p4_late_' + secrets.token_hex(4)
        self.sql("INSERT INTO p1_period_instances (id,enabled,title,kind,mode,goal,rewards,start_at,end_at,reward_end_at,rank_places,rank_rewards,source) VALUES ('" + key + "',1,'实际登录补给','login_day','cumulative',1,'item_124,15'," + str(now-3600000) + ',' + str(now+3600000) + ',' + str(now+7200000) + ",0,'','isolated-real-login-v1');")
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.send('CSPlayerLogin', login=login), range(8)))
        self.assertTrue(all(op == BY_NAME['SCPlayerLogin'] for op, _ in replies))
        op, info = self.send('CSActivityInfo', {'activityId': key}, login)
        self.assertEqual(op, BY_NAME['SCActivityInfo'])
        self.assertEqual((info.itemList[0].left, info.itemList[0].total, info.itemList[0].status), (1, 1, 1))
        args = {'activityId': key, 'index': 1, 'getResources': [{'id': 'item_124', 'num': 15}]}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            claimed = list(pool.map(lambda _: self.send('CSActivityReward', args, login), range(8)))
        self.assertTrue(all(op == BY_NAME['SCActivityReward'] for op, _ in claimed))
        self.assertEqual(sum(bool(result.get.simps) for _, result in claimed), 1)
        reward_id = int(self.sql("SELECT reward_id FROM p1_period_progress WHERE instance_id='" + key + "' AND player_id=" + str(login.playerId) + ';').strip())
        self.assertFalse(self.send('CSAddRewardsToPlayer', {'rewardIds': [reward_id]}, login)[1].result.simps)
        self.stop()
        self.start()
        login = self.login(uid)
        self.send('CSPlayerLogin', login=login)
        op, info = self.send('CSActivityInfo', {'activityId': key}, login)
        self.assertEqual((op, info.itemList[0].left, info.itemList[0].status), (BY_NAME['SCActivityInfo'], 1, 2))
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p1_event_facts WHERE player_id=" + str(login.playerId) + " AND kind='login_day';").strip(), '1')
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p1_reward_deliveries WHERE player_id=" + str(login.playerId) + " AND source_key='activity:" + key + "';").strip(), '1')


# Run only this new scenario; the unchanged P1 regressions remain in their own suite.
del P1MySQLTests

if __name__ == '__main__':
    unittest.main()
