"""One actual packaged second-map fish/food source over original HTTP/InnoDB.

Finite level25/Cooking2/coin prerequisites are explicit. Target ingredients and
servings originate solely in real frozen gathering. Original daily fatigue is
retained; first fish waits real5s, later isolated ready/rebirth only accelerates.
"""
import concurrent.futures
import hashlib
import http.client
import json
import os
from pathlib import Path
import time
import unittest

from server.codec import BY_NAME, decode_responses, encode_http_request, message
import test_copy_book_recovery_native_mysql as transport
import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_P8_PACKAGED_COOKING_MYSQL') == '1', 'Explicit fixed source/original frames/isolated second-map natural cooking')
class PackagedCookingMySQLTests(transport.CopyBookRecoveryNativeMySQLTests):
    __unittest_skip__ = False
    __unittest_skip_why__ = ''
    test_original_paid_recovery_short_source_chain_to_specialty = None

    @classmethod
    def setUpClass(cls):
        # Run this verifier after the inherited schema/user cleanup and temp
        # cleanup. The report contains no connection credentials.
        def verify_clean():
            if not hasattr(cls, 'database'):
                return
            counts = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';").split()
            if counts != ['0', '0', '0']:
                raise AssertionError('P8 source isolated schema/user/grants remain')
            path = shared.ROOT / 'analysis/p8/packaged-second-map-original-http-mysql-20261014.json'
            if path.exists():
                report = json.loads(path.read_text())
                report['isolatedCleanup'] = {'schemas': 0, 'users': 0, 'grants': 0}
                path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        cls.addClassCleanup(verify_clean)
        super().setUpClass()

    def test_actual_second_map_fish_food_concurrent_cold(self):
        self.start(); uid = 'p8-packaged-natural-source'; login = self.login(uid)
        self.assertEqual(self.send('CSPlayerCreate', {'playerName': '第二图料理来源', 'heroId': 'hero_043'}, login)[0], BY_NAME['SCPlayerCreate'])
        player, game = self.profile(login)
        player.update(level=25, coin=1000000, noviceGuideSchedule=7)
        self.assertFalse(game.get('cooking'))
        self.write_profile(login, player, game)
        self.sql(f'INSERT INTO cooking_craft_states(player_id,level,exp) VALUES({login.playerId},2,0);')
        stamp = 1800002700000
        def call(name, fields, expected):
            nonlocal stamp
            stamp += 1
            return self.call(name, fields, login, stamp, expected)

        def move(point):
            nonlocal stamp
            stamp += 1
            body = encode_http_request(BY_NAME['CSMove'], message('CSMove', pointNew=point, currentTime=stamp),
                                       player_id=login.playerId, zone_id=1, session=bytes(login.sessionKey), timestamp=stamp)
            connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=60)
            try:
                # Original movement is a one-way frame; a successful handler
                # can send no HTTP reply. Read the stored position as its effect.
                connection.request('POST', '/sao/', body=body, headers={'Content-Type': 'application/octet-stream'})
                reply = connection.getresponse()
                self.assertEqual(reply.status, 200)
                frames = decode_responses(reply.read())
                self.assertFalse(any(op == BY_NAME['ErrorMessage'] for op, _ in frames))
            finally:
                connection.close()
        call('CSTransferScene', {'target': 'field_002'}, 'SCTransferScene')
        call('CSLoginScene', {}, 'SCLoginScene')
        # The original scene loader acknowledges its current position before
        # movement. Login alone leaves the old-scene movement guard locked.
        call('CSCurrentPosition', {}, 'SCCurrentPosition')
        gained = {}; attempts = 0; source_rows = []
        while any(gained.get(key, 0) < 10 for key in ('FishItem_008', 'FishItem_014', 'FishItem_020')):
            self.assertLess(attempts, 100, 'Original daily200 fatigue exhausted; do not prepare fish or reset quota')
            raw = self.sql(f'SELECT CONCAT(pos_id,"|",remaining,"|",HEX(snapshot)) FROM p8_resource_nodes WHERE player_id={login.playerId} AND scene="field_002" ORDER BY pos_id;')
            nodes = [(v.split('|')[0], int(v.split('|')[1]), json.loads(bytes.fromhex(v.split('|')[2]))) for v in raw.splitlines()]
            selected = next((n for n in nodes if n[1] and n[2]['kind'] == 'fish' and n[2]['level'] == 2), None)
            if selected is None:
                # The original refresh comparison requires a positive past date.
                self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND scene="field_002" AND remaining=0;')
                call('CSLoginScene', {}, 'SCLoginScene'); continue
            pos, remaining, cfg = selected
            move(cfg['point'])
            self.assertEqual(self.profile(login)[1]['position']['current'], cfg['point'])
            started = call('CSFieldResourceStartGather', {'posId': pos}, 'SCFieldResourceStartGather'); self.assertEqual(started.result, 1)
            if attempts == 0:
                before = self.profile(login); stamp += 1
                self.assertEqual(self.stamped('CSFieldGetResource', {}, login, stamp)[0], 10001)
                self.assertEqual(self.profile(login), before); time.sleep(cfg['gatherMs'] / 1000 + .1)
            else:
                self.sql(f'UPDATE p8_gather_attempts SET ready_at=1 WHERE player_id={login.playerId} AND active=1;')
            claimed = call('CSFieldGetResource', {}, 'SCFieldGetResource'); self.assertEqual(claimed.code, 1)
            attempts += 1
            source_rows.append({'posId': pos, 'resourceId': cfg['resourceId'], 'gain': [{'id': v.id, 'num': v.num} for v in claimed.res.simps]})
            player, game = self.profile(login); gained = {v['id']: v['num'] for v in game['cooking']}
            self.assertEqual(game['field']['side']['tiredness'], 200 - 2 * attempts)
            if remaining == 1:
                self.sql(f'UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id={login.playerId} AND scene="field_002" AND pos_id="{pos}";')
        recipe = call('CSFishCooking', {'fish': {'id': 'FishItem_033', 'num': 2}}, 'SCFishCooking')
        self.assertEqual(recipe.res.simps[0].num, 2)
        player, game = self.profile(login); guid = player['heroWarId']
        fields = {'heroGuid': guid, 'costItem': {'id': 'FishItem_033', 'num': 1}}
        stamp += 1; paid = stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.call('CSFishUsing', fields, login, paid, 'SCFishUsing'), range(8)))
        self.assertTrue(all(v == replies[0] for v in replies)); self.assertEqual(replies[0].warrior.cooking.prop.hp, 10)
        player, game = self.profile(login); self.assertEqual(next(v['num'] for v in game['cooking'] if v['id'] == 'FishItem_033'), 1)
        self.stop(); self.start(); login = self.login(uid)
        self.assertEqual(self.call('CSFishUsing', fields, login, paid, 'SCFishUsing'), replies[0])
        before = self.profile(login)
        call('CSTransferScene', {'target': 'field_001'}, 'SCTransferScene')
        after = self.profile(login); self.assertEqual(after[1]['field']['side']['tiredness'], before[1]['field']['side']['tiredness'])
        call('CSTransferScene', {'target': 'field_002'}, 'SCTransferScene')
        self.assertEqual(self.profile(login)[1]['field']['side']['tiredness'], 200 - 2 * attempts)
        second = call('CSFishUsing', fields, 'SCFishUsing'); self.assertEqual(second.warrior.cooking.prop.hp, 20)
        before = self.profile(login)
        self.assertEqual(self.stamped('CSFishUsing', fields, login, paid)[0], 10001)
        self.assertEqual(self.profile(login), before)
        report = {'status': 'passed-actual-original-http-innoDB-natural-source-and-consumption', 'productionModified': False, 'phoneVerified': False,
                  'sourceSha256': self.source_shas, 'baseline': self.baseline_source, 'actualGathers': attempts, 'actualFatigueCost': 2 * attempts,
                  'targetIngredientsPrepared': False, 'sourceRows': source_rows, 'recipe': 'FishItem_033', 'actualCraftCount': 2,
                  'eightConcurrentAndCold': 'one serving and identical real warrior snapshot', 'actualHP': [10, 20],
                  'crossMapFatiguePreserved': True, 'retiredOldSnapshotRejectedNoMutation': True,
                  'boundary': 'level25/Cooking2/coin finite prerequisites; first real5s fish and early rejection; later isolated ready/rebirth acceleration; SQL permanent hp10 named compatibility unchanged'}
        out = shared.ROOT / 'analysis/p8/packaged-second-map-original-http-mysql-20261014.json'
        out.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__': unittest.main()
