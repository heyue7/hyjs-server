"""Root-prebuilt original AincradStars private earned-star activity chain.

Finite Player22 only, actual Create047 level1/GUID/complete skills unchanged.
Original known scoped Trial computation supplies every real End. No winner,
old total, EventFact, reward, wallet or material is seeded. Full original event
ladder remains unknown: this named local first tier is new earned3 ->point100.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import types
import unittest
import uuid

import test_growth_resource_loop_mysql as shared
from hyjs_managed_mysql import ManagedMySQL
from server.codec import message, parse_http


def protocol_preflight():
    vectors = {
        'CSActivityList': {}, 'CSActivityInfo': {'activityId': 'aincradStars_p4_lp-' + '0' * 26},
        'CSActivityReward': {'activityId': 'aincradStars_p4_lp-' + '0' * 26,
                             'index': 1, 'getResources': [{'id': 'point', 'num': 100}]},
        'CSRushCurrentInfo': {}, 'CSRushFightStart': {'rushType': 1, 'taskId': 'local-section-1', 'star': 3},
        'CSRushResetChallengeCount': {},
        'CSShopBuy': {'shopType': 5, 'shopItemId': 'p4Point_1', 'buyCount': 1},
    }
    for name, fields in vectors.items():
        raw = shared.wire.encode_original_http(name, fields, 2, b'0' * 16, 500)
        if parse_http(raw).decode() != message(name).__class__(**fields):
            raise AssertionError('Original Aincrad consumer contract changed: ' + name)
    for name, fields in {
        'SCActivityList': {'activityList'}, 'SCActivityInfo': {'itemList', 'activityId'},
        'ActivityItem': {'left', 'total', 'status', 'rewardList', 'index'},
        'SCActivityReward': {'get', 'itemList'}, 'SCRushFightStart': {'fightId', 'randSeed', 'battlePlayer', 'battleMonsterList'},
        'SCRushFightEnd': {'star', 'result'}, 'SCShopBuy': {'cost', 'get'},
    }.items():
        if not fields <= set(message(name).DESCRIPTOR.fields_by_name):
            raise AssertionError('Original activity output changed: ' + name)
    return {'requestRoundTrips': len(vectors), 'reward': 'named local first3 earned stars100point, not full original ladder'}


@unittest.skipUnless(os.environ.get('HYJS_AINCRAD_MYSQL_TEST') == '1', 'Root coordinated managed SQL lease')
class AincradPersonalMySQLTests(unittest.TestCase):
    send = shared.GrowthResourceLoopMySQLTests.send
    login = shared.GrowthResourceLoopMySQLTests.login
    stamped = shared.GrowthResourceLoopMySQLTests.stamped
    call = shared.GrowthResourceLoopMySQLTests.call
    profile = shared.GrowthResourceLoopMySQLTests.profile
    start = shared.GrowthResourceLoopMySQLTests.start
    stop = shared.GrowthResourceLoopMySQLTests.stop
    setUp = shared.GrowthResourceLoopMySQLTests.setUp
    sql = shared.GrowthResourceLoopMySQLTests.sql
    json_request = shared.GrowthResourceLoopMySQLTests.json_request

    def test_actual_trial_new_stars_refresh_old_award_point_shop_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        contract = protocol_preflight()
        self.source, binary, manifest_sha = shared.preflight()
        self.assertEqual(shared.sha(Path(shared.__file__)), 'd12406fd260d7a27ab1fd4845698b146d915154d77866565eddbd1c0453a60a5')
        self.assertIn('recordP7AcceptedStarActivity(tx, &fight, cfg, state)', (self.source / 'hyjs-server/service/p7RushService.go').read_text())
        self.assertIn('P7RushEarnedStarEvent', (self.source / 'hyjs-server/service/localPlayActivityScopeService.go').read_text())
        claim_service = self.source / 'hyjs-server/service/periodService.go'
        for contract_line in ('gain := &pb.ResultObject{}', 'if delivery.ClaimedAt == 0 {',
                              'delivery.Result = blob', 'Get: gain}'):
            self.assertIn(contract_line, claim_service.read_text())
        self.binary = str(binary)
        output = Path(os.environ['HYJS_AINCRAD_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(shared.ROOT / 'analysis/growth-first-20261016/aincrad'))
        self.assertFalse(output.exists())
        evidence = output.with_name(output.stem + '-wire')
        self.assertFalse(evidence.exists())
        evidence.mkdir(parents=True)
        report = dict(status='PENDING', fullChain=False, runnerSHA256=shared.sha(Path(__file__)), sourceManifestSHA256=manifest_sha,
                      binarySHA256=shared.sha(binary), contract=contract,
                      claimServiceSHA256=shared.sha(claim_service),
                      finitePreparation='only Player22; actual Create047 level1/GUID/all3skills, initial wallet/inventory unchanged',
                      seededWinnerFactStarsCurrency=False, globalAuthorityChanged=False, phone=False, productionModified=False,
                      actualBattles=[])
        self.managed_pool = ManagedMySQL()
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-aincrad-http-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.managed_pool.allocate('aincrad_activity')
                    self.database = allocation.database
                    self.env = {**os.environ, **allocation.env(), 'GOMAXPROCS': '2', 'GOMEMLIMIT': '768MiB'}
                    self.start()
                    self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('profiles','p1_event_facts','p1_period_instances','p7_rush_fights','local_play_activity_cycles');").strip(), 'InnoDB')
                    for table, columns in {
                        'p1_reward_deliveries': {'id', 'player_id', 'source_key', 'rewards', 'claimed_at', 'result'},
                        'p1_period_progress': {'instance_id', 'player_id', 'reward_id'},
                    }.items():
                        actual_columns = self.sql('SELECT COLUMN_NAME FROM information_schema.COLUMNS '
                            f"WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='{table}';").split()
                        self.assertTrue(columns <= set(actual_columns), 'Actual claim receipt schema changed: ' + table)
                    identity = 'demo-aincrad-' + secrets.token_hex(6)
                    protected = self.login(identity + '-protected')
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '试炼保护账号', 'heroId': 'hero_047'}, protected, 1, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    owner = self.login(identity)
                    stamp = 1600000

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                            results = list(pool.map(lambda _: self.call(name, fields, owner, stamp, expected), range(8)))
                        self.assertTrue(all(r == results[0] for r in results))
                        return results[0], stamp

                    def apply(action, request_id=None):
                        body = {'playerId': owner.playerId, 'sessionHex': bytes(owner.sessionKey).hex(),
                                'activityId': 'AincradStars', 'action': action, 'requestId': request_id or str(uuid.uuid4())}
                        status, reply = self.json_request('/local-play/activities/apply', body)
                        self.assertEqual(status, 200)
                        self.assertEqual(reply.get('code'), 0, reply.get('msg'))
                        return reply['data']

                    def current():
                        row = self.sql('SELECT c.id,HEX(c.snapshot),c.event_after_id FROM local_play_activity_cycles c '
                            'JOIN local_play_activity_bindings b ON b.cycle_id=c.id '
                            f"WHERE b.player_id={int(owner.playerId)} AND b.activity_id='AincradStars';").strip().split('\t')
                        snapshot = json.loads(bytes.fromhex(row[1]))
                        self.assertEqual(len(snapshot['periods']), 1)
                        period = snapshot['periods'][0]
                        self.assertEqual((period['kind'], period['mode'], period['goal'], period['rewards']), ('rush_star_earned', 'cumulative', 3, 'point,100'))
                        return period['id'], '|'.join(row)

                    def info(key, left, status):
                        view = http('CSActivityInfo', {'activityId': key}, 'SCActivityInfo')
                        self.assertEqual(len(view.itemList), 1)
                        self.assertEqual((view.itemList[0].left, view.itemList[0].total, view.itemList[0].status), (left, 3, status))
                        return view

                    def claim(key, view):
                        nonlocal stamp
                        # P1 claims acknowledge status2 on every retry. Only the
                        # first grant carries a delta; later Get is intentionally
                        # empty. Verify the saved delivery and actual balance.
                        query = ('SELECT d.id,d.rewards,d.claimed_at,HEX(d.result),p.reward_id,d.source_key '
                                 'FROM p1_reward_deliveries d JOIN p1_period_progress p ON p.reward_id=d.id '
                                 f"WHERE p.player_id={int(owner.playerId)} AND d.player_id=p.player_id AND p.instance_id='{key}';")
                        before_receipt = self.sql(query).strip().split('\t')
                        self.assertEqual(len(before_receipt), 6)
                        self.assertEqual((before_receipt[1], before_receipt[4], before_receipt[5]),
                                         ('point,100', before_receipt[0], 'activity:' + key))
                        was_claimed = int(before_receipt[2]) > 0
                        before_profile = self.profile(owner)
                        stamp += 1
                        saved_stamp = stamp
                        request = {'activityId': key, 'index': 1,
                                   'getResources': view.itemList[0].rewardList}
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                            results = list(pool.map(lambda _: self.call('CSActivityReward', request, owner,
                                                                      saved_stamp, 'SCActivityReward'), range(8)))
                        expected_gain = message('ResultObject', simps=[{'id': 'point', 'num': 100}])
                        gains = []
                        for reply in results:
                            self.assertEqual(reply.activityId, key)
                            self.assertEqual(len(reply.itemList), 1)
                            item = reply.itemList[0]
                            self.assertEqual((item.index, item.left, item.total, item.status), (1, 3, 3, 2))
                            self.assertEqual(item.rewardList, view.itemList[0].rewardList)
                            gain = getattr(reply, 'get')
                            self.assertIn(gain, [message('ResultObject'), expected_gain])
                            gains.append(100 if gain == expected_gain else 0)
                        self.assertEqual(sum(gains), 0 if was_claimed else 100)
                        after_profile = self.profile(owner)
                        delta = 0 if was_claimed else 100
                        self.assertEqual(after_profile[0].get('point', 0), before_profile[0].get('point', 0) + delta)
                        self.assertEqual({k: v for k, v in after_profile[0].items() if k != 'point'},
                                         {k: v for k, v in before_profile[0].items() if k != 'point'})
                        self.assertEqual(after_profile[1], before_profile[1])
                        after_receipt = self.sql(query).strip().split('\t')
                        self.assertEqual(after_receipt[:2], before_receipt[:2])
                        self.assertEqual(after_receipt[4:], before_receipt[4:])
                        self.assertGreater(int(after_receipt[2]), 0)
                        stored_gain = message('ResultObject')
                        stored_gain.ParseFromString(bytes.fromhex(after_receipt[3]))
                        self.assertEqual(stored_gain, expected_gain)
                        if was_claimed:
                            self.assertEqual(after_receipt, before_receipt)
                        sequence = len(report.get('activityClaims', []))
                        for ordinal, reply in enumerate(results):
                            (evidence / f'claim-{sequence}-{ordinal}.SCActivityReward.pb').write_bytes(
                                reply.SerializeToString(deterministic=True))
                        audit = dict(activityId=key, requestStamp=saved_stamp, alreadyClaimed=was_claimed,
                                     concurrentGainPoints=gains, actualPointDelta=delta,
                                     beforeReceipt=before_receipt, afterReceipt=after_receipt)
                        (evidence / f'claim-{sequence}.receipt.json').write_text(json.dumps(audit, ensure_ascii=False, indent=2) + '\n')
                        report.setdefault('activityClaims', []).append(audit)
                        return results[0], saved_stamp

                    def play(floor, star):
                        task = next(t for t in http('CSRushCurrentInfo', {}, 'SCRushCurrentInfo').taskInfoList if t.star == star)
                        start = http('CSRushFightStart', {'rushType': 1, 'taskId': task.taskId, 'star': star}, 'SCRushFightStart')
                        raw = start.SerializeToString(deterministic=True)
                        path = evidence / (str(len(report['actualBattles'])) + '.SCRushFightStart.pb')
                        path.write_bytes(raw)
                        terminal_raw = bytes.fromhex(self.sql(f"SELECT HEX(terminal) FROM p7_rush_scoped_calculations WHERE fight_id='{start.fightId}';").strip())
                        terminal_path = path.with_suffix('.terminal.json')
                        terminal_path.write_bytes(terminal_raw)
                        terminal = json.loads(terminal_raw)
                        self.assertTrue(terminal['outcome']['authoritative'])
                        winner = terminal['outcome']['winner']
                        actual = int(winner == 0)
                        self.assertEqual(actual, 1, 'Actual source lost: preserve real outcome; never replace winner or actor values')
                        end = {'fightId': start.fightId, 'result': actual, 'optRecord': terminal['originalOperates']}
                        held = self.profile(owner)
                        before_facts = self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={int(owner.playerId)} AND kind IN ('rush_star_total','rush_star_earned');").strip()
                        http('CSRushFightEnd', dict(end, result=0), 'ErrorMessage')
                        self.assertEqual(self.profile(owner), held)
                        self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={int(owner.playerId)} AND kind IN ('rush_star_total','rush_star_earned');").strip(), before_facts)
                        accepted, _ = once('CSRushFightEnd', end, 'SCRushFightEnd')
                        self.assertEqual((accepted.result, accepted.star), (actual, star))
                        report['actualBattles'].append(dict(floor=floor, actualStar=star, fightId=start.fightId,
                             seed=start.randSeed, actualWinner=winner, scopeProof=terminal['outcome']['capabilityHash'],
                             startSHA256=hashlib.sha256(raw).hexdigest(), terminalSHA256=hashlib.sha256(terminal_raw).hexdigest()))
                        return end

                    http('CSPlayerCreate', {'playerName': '艾恩星数真链', 'heroId': 'hero_047'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual(tuple(initial[0].get(k, 0) for k in ('coin', 'diamond', 'point')), (0, 0, 0))
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',22) WHERE id={int(owner.playerId)};")
                    http('CSFormationList', {}, 'SCFormationList')
                    self.sql('UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    self.assertEqual(apply('open')['results'][0]['status'], 'closed')
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM local_play_activity_bindings WHERE player_id={int(owner.playerId)} AND activity_id='AincradStars';").strip(), '0')
                    self.sql("UPDATE p7_rush_stage_rules SET enabled=1 WHERE id LIKE 'local-section:%';UPDATE p7_rush_policies SET enabled=1;UPDATE p7_rush_scoped_policies SET enabled=1;UPDATE p7_rush_manual_policies SET enabled=0;")
                    now = int(time.time()*1000)
                    self.sql(f'UPDATE p7_section_reward_policies SET enabled=1,published_at={now-1000},start_at={now-1000},end_at={now+3600000};')
                    play(1, 3)
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={int(owner.playerId)} AND kind='rush_star_earned';").strip(), '1')
                    request_id = str(uuid.uuid4())
                    opened = apply('open', request_id)
                    self.assertEqual(opened['results'][0]['status'], 'open')
                    self.assertEqual(apply('open', request_id), opened)
                    activity, old_cycle = current()
                    info(activity, 0, 0)
                    listed = http('CSActivityList', {}, 'SCActivityList')
                    self.assertIn(activity, [tab.activityId for tab in listed.activityList])
                    self.assertEqual(len([tab for tab in listed.activityList if tab.activityId.startswith('aincradStars_')]), 1)
                    old_end = play(2, 1)
                    info(activity, 1, 0)
                    http('CSRushFightEnd', old_end, 'SCRushFightEnd')
                    info(activity, 1, 0)
                    play(3, 1)
                    info(activity, 2, 0)
                    once('CSRushResetChallengeCount', {}, 'SCRushResetChallengeCount')
                    info(activity, 2, 0)
                    play(1, 1)
                    ready = info(activity, 3, 1)
                    refresh_id = str(uuid.uuid4())
                    refresh = apply('refresh', refresh_id)
                    self.assertEqual(apply('refresh', refresh_id), refresh)
                    newest, _ = current()
                    self.assertNotEqual(newest, activity)
                    info(newest, 0, 0)
                    info(activity, 3, 1)
                    claim(activity, ready)
                    info(activity, 3, 2)
                    self.assertEqual(self.profile(owner)[0]['point'], 140)
                    held = self.profile(owner)
                    self.stop();self.start();owner = self.login(identity)
                    self.assertEqual(self.profile(owner), held)
                    info(newest, 0, 0)
                    info(activity, 3, 2)
                    play(2, 3)
                    next_ready = info(newest, 3, 1)
                    claim(newest, next_ready)
                    buy, buy_stamp = once('CSShopBuy', {'shopType': 5, 'shopItemId': 'p4Point_1', 'buyCount': 1}, 'SCShopBuy')
                    self.assertEqual([(v.id, v.num) for v in buy.cost], [('point', 100)])
                    held = self.profile(owner)
                    self.assertEqual((held[0].get('point', 0), held[0].get('coin', 0), held[0].get('diamond', 0)), (150, 0, 0))
                    self.assertEqual(sum(i['num'] for i in held[1]['items'] if i['id'] == 'item_021'), 1)
                    old_id = old_cycle.split('|')[0]
                    self.assertEqual(self.sql(f"SELECT id,HEX(snapshot),event_after_id FROM local_play_activity_cycles WHERE id='{old_id}';").strip().replace('\t', '|'), old_cycle)
                    info(activity, 3, 2)
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={int(owner.playerId)} AND kind='rush_star_earned';").strip(), '5')
                    self.stop();self.start();owner = self.login(identity)
                    self.assertEqual(self.call('CSShopBuy', {'shopType': 5, 'shopItemId': 'p4Point_1', 'buyCount': 1}, owner, buy_stamp, 'SCShopBuy'), buy)
                    claim(newest, next_ready)
                    self.assertEqual(self.profile(owner), held)
                    self.assertEqual(self.profile(protected), protected_before)
                    report.update(status='PASS', fullChain=True, actualNewEarnedStars=[3, 1, 1, 1, 3], initialOld3Excluded=True,
                         originalPeriodIDs=[activity, newest], originalGenericSelectors=True, newPeriodNeedsActual3=True,
                         retiredPendingOriginalAwardClaimedOnce=True, oldPeriodSnapshotExact=True,
                         acceptedEnd8Once=True, rejectedWinnerCreatesNoFact=True, actualPointShopEssence=1,
                         finalPoint=150, coldRestarts=2, protected1Exact=True)
                except BaseException as error:
                    report.update(status='FAIL', error=self.managed_pool.redact(repr(error)))
                    raise
                finally:
                    cleanup_errors = []
                    try:
                        self.stop()
                    except BaseException as error:
                        cleanup_errors.append(type(error).__name__)
                    try:
                        self.managed_pool.close()
                    except BaseException as error:
                        cleanup_errors.append(type(error).__name__)
                    report.update(wallSeconds=round(time.monotonic()-began, 3), remainingOwnedAllocations=len(self.managed_pool.connections), ownedAppStopped=self.process is None, cleanupErrors=cleanup_errors)
                    if cleanup_errors:
                        report['status'] = 'FAIL_CLEANUP'
                    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
                    self.assertFalse(cleanup_errors, 'Owned application or managed allocation cleanup failed')


if __name__ == '__main__':
    unittest.main()
