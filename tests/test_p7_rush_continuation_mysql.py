"""Root-coordinated prebuilt original Rush protocol and managed MySQL chain.

The only finite preparation is Player22 on a new original047 account. First
three accepted wins earn the real star currency; all subsequent End winners and
rewards come from the actual frozen computation. This runner uses the certified
automatic-record family. Original phone manual/auto UI remains separate.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import math
import os
from pathlib import Path
import secrets
import tempfile
import time
import unittest

from hyjs_managed_mysql import ManagedMySQL
from test_p7_maze_trader_mysql import P7MazeTraderMySQLTests as Transport
from server.codec import message

ROOT = Path(__file__).resolve().parents[1]


def preflight_protocol():
    for name in ['CSRushGetSectionAddition', 'CSRushAdditionView']:
        assert not list(message(name).DESCRIPTOR.fields), name
    assert set(message('CSRushAddition').DESCRIPTOR.fields_by_name) == {'select', 'star'}
    assert set(message('SCRushGetSectionAddition').DESCRIPTOR.fields_by_name) == {'starResult3', 'starResult6', 'starResult9', 'prop', 'costStar'}
    assert 'optRecord' in message('CSRushFightEnd').DESCRIPTOR.fields_by_name
    assert 'battleMonsterList' in message('SCRushFightStart').DESCRIPTOR.fields_by_name


@unittest.skipUnless(os.environ.get('HYJS_P7_CONTINUATION_MYSQL_TEST') == '1', 'Root single managed SQL lease')
class P7RushContinuationMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql

    def test_original_stars_addition_fourth_actual_end_reward_cold_and_new_run(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        preflight_protocol()
        self.source = Path(os.environ['HYJS_P7_CONTINUATION_SOURCE']).resolve()
        self.binary = Path(os.environ['HYJS_P7_CONTINUATION_BINARY']).resolve()
        self.assertTrue((self.source / 'hyjs-server/go.mod').is_file())
        digest = hashlib.sha256(self.binary.read_bytes()).hexdigest()
        self.assertEqual(digest, os.environ['HYJS_P7_CONTINUATION_BINARY_SHA256'])
        activation = ROOT / 'analysis/p7/rush-continuation-activate-20261016.sql'
        report = dict(status='PENDING', binarySHA256=digest, source=str(self.source),
                      activationSHA256=hashlib.sha256(activation.read_bytes()).hexdigest(),
                      finitePreparation={'PlayerLevel': 22, 'main': 'original Create047 level1, GUID, complete skills and initial wallet unchanged'},
                      localCatalogue='named continuation4..6; original NPC047 with explicit 1.5/2/2.5 panel scaling',
                      phoneUI=False, formalModified=False, globalAuthorityChanged=False)
        output = Path(os.environ.get('HYJS_P7_CONTINUATION_OUTPUT', str(ROOT / 'analysis/p7/rush-continuation-native-checkpoint.json')))
        began = time.monotonic()
        self.managed_pool = ManagedMySQL()
        with tempfile.TemporaryDirectory(prefix='hyjs-p7-continuation-') as temporary:
            self.native_log = Path(temporary) / 'native.log'
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    allocation = self.managed_pool.allocate('p7_continuation')
                    self.database, self.test_user = allocation.database, allocation.user
                    self.test_password = allocation.password
                    self.env = dict(os.environ, **allocation.env(), GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    self.assertEqual(self.sql("SELECT COUNT(*) FROM p7_rush_stage_rules WHERE id LIKE 'local-continuation:%' AND enabled=0;").strip(), '9')
                    self.sql(activation.read_text())
                    # The original phone manual driver is covered by its own
                    # certificate; this deterministic transport probe uses auto.
                    self.sql('UPDATE p7_rush_manual_policies SET enabled=0 WHERE id=1;')
                    identity = 'continuation-' + secrets.token_hex(5)
                    owner = self.login(identity)
                    stamp = 284000

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    http('CSPlayerCreate', {'playerName': '试炼真实加成', 'heroId': 'hero_047'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual((initial[0].get('coin', 0), initial[0].get('diamond', 0), initial[0].get('energy', 0)), (0, 0, 100))
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',22) WHERE id={owner.playerId};")
                    http('CSFormationList', {}, 'SCFormationList')
                    records = []

                    def actual_start(floor):
                        tasks = http('CSRushCurrentInfo', {}, 'SCRushCurrentInfo').taskInfoList
                        self.assertEqual(sorted(t.star for t in tasks), [1, 2, 3])
                        self.assertTrue(all(t.point == 10 and t.coin == 0 for t in tasks))
                        task = next(t for t in tasks if t.star == 1)
                        start = http('CSRushFightStart', {'rushType': 1, 'taskId': task.taskId, 'star': task.star}, 'SCRushFightStart')
                        self.assertEqual(start.battlePlayer.formation[0].guid, initial[0]['heroWarId'])
                        terminal = json.loads(bytes.fromhex(self.sql("SELECT HEX(terminal) FROM p7_rush_scoped_calculations WHERE fight_id='" + start.fightId + "';").strip()))
                        actual = int(terminal['outcome']['winner'] == 0)
                        end = {'fightId': start.fightId, 'result': actual, 'optRecord': terminal['originalOperates']}
                        records.append(dict(floor=floor, fightId=start.fightId, seed=start.randSeed, actualWinner=terminal['outcome']['winner'], finalRound=terminal['outcome']['finalRound'], capability=terminal['outcome']['capabilityHash']))
                        return start, end, actual

                    for floor in [1, 2, 3]:
                        start, end, actual = actual_start(floor)
                        self.assertEqual(actual, 1, 'Actual first-section source lost; retain outcome and do not rewrite winner')
                        ended = http('CSRushFightEnd', end, 'SCRushFightEnd')
                        self.assertEqual((ended.result, ended.star), (1, 1))
                    baseline_attack = start.battlePlayer.formation[0].prop.att
                    info = http('CSGetRushInfo', {}, 'SCGetRushInfo')
                    self.assertEqual((info.curFloor, info.curTotalStar, info.isAdditionNeedGet), (4, 3, 1))
                    earned = http('CSRushGetSectionReward', {}, 'SCRushGetSectionReward')
                    self.assertEqual([(r.id, r.num) for r in earned.gain.simps], [('item_001', 1)])
                    old_energy = self.profile(owner)[0]['energy']
                    http('CSItemUse', {'costItem': {'id': 'item_001', 'num': 1}}, 'SCItemUse')
                    self.assertEqual(self.profile(owner)[0]['energy'], old_energy + 10)
                    option = http('CSRushGetSectionAddition', {}, 'SCRushGetSectionAddition')
                    self.assertEqual((option.costStar, option.starResult3), (3, 'peratt,0.03'))
                    http('CSRushAddition', {'select': option.starResult6, 'star': 6}, 'ErrorMessage')
                    stamp += 1
                    buy_stamp = stamp
                    paid = self.call('CSRushAddition', {'select': option.starResult3, 'star': 3}, owner, buy_stamp, 'SCRushAddition')
                    self.assertEqual(paid.prop.peratt, .03)
                    self.assertEqual(http('CSRushGetSectionAddition', {}, 'SCRushGetSectionAddition').costStar, 0)
                    before_fourth = self.profile(owner)
                    fourth, end, actual = actual_start(4)
                    self.assertEqual(fourth.battlePlayer.formation[0].prop.att, math.floor(baseline_attack * 1.03))
                    self.assertGreater(fourth.battlePlayer.formation[0].prop.att, baseline_attack)
                    # A reconnect restores the exact pending local fight even
                    # after its catalogue closes; no new battle is calculated.
                    self.sql('UPDATE p7_rush_continuation_policies SET enabled=0;')
                    self.stop()
                    self.start()
                    owner = self.login(identity)
                    pending_tasks = http('CSRushCurrentInfo', {}, 'SCRushCurrentInfo').taskInfoList
                    self.assertEqual([(v.taskId, v.star) for v in pending_tasks], [('local-continuation-4', 1)])
                    resumed = http('CSRushFightStart', {'rushType': 1, 'taskId': 'local-continuation-4', 'star': 1}, 'SCRushFightStart')
                    self.assertEqual(resumed, fourth)
                    self.assertEqual(self.sql("SELECT COUNT(*) FROM p7_rush_fights WHERE id='" + fourth.fightId + "';").strip(), '1')
                    http('CSRushFightStart', {'rushType': 1, 'taskId': 'local-continuation-4', 'star': 2}, 'ErrorMessage')
                    held = self.profile(owner)
                    http('CSRushFightEnd', dict(end, result=1-actual), 'ErrorMessage')
                    self.assertEqual(self.profile(owner), held)
                    # Changing the live directory cannot alter a pending Start.
                    self.sql("UPDATE p7_rush_continuation_policies SET enabled=0;UPDATE p7_rush_stage_rules SET reward='point,99999' WHERE id='local-continuation:4:1';")
                    ended = http('CSRushFightEnd', end, 'SCRushFightEnd')
                    self.assertEqual((ended.result, ended.star), (actual, actual))
                    after_fourth = self.profile(owner)
                    self.assertEqual(after_fourth[0].get('point', 0), before_fourth[0].get('point', 0) + 10*actual)
                    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as workers:
                        duplicates = list(workers.map(lambda _: self.call('CSRushFightEnd', end, owner, stamp+1, 'SCRushFightEnd'), range(4)))
                    self.assertTrue(all(v == ended for v in duplicates))
                    self.assertEqual(self.profile(owner), after_fourth)
                    self.stop()
                    self.start()
                    owner = self.login(identity)
                    self.assertEqual(http('CSRushFightEnd', end, 'SCRushFightEnd'), ended)
                    self.assertEqual(self.call('CSRushAddition', {'select': option.starResult3, 'star': 3}, owner, buy_stamp, 'SCRushAddition'), paid)
                    self.assertEqual(self.profile(owner), after_fourth)
                    self.assertEqual(http('CSRushAdditionView', {}, 'SCRushAdditionView').prop.peratt, .03)
                    http('CSRushResetChallengeCount', {}, 'SCRushResetChallengeCount')
                    self.assertEqual(http('CSRushAdditionView', {}, 'SCRushAdditionView').prop.peratt, 0)
                    self.assertEqual(http('CSGetRushInfo', {}, 'SCGetRushInfo').curFloor, 1)
                    again, again_end, again_actual = actual_start(1)
                    http('CSRushFightEnd', again_end, 'SCRushFightEnd')
                    report.update(status='PASS', actualBattles=records, fourthActualResult=actual,
                                  realThreeStarPurchase=True, fourthSnapshotConsumesAddition=True,
                                  sectionRewardUsed=True, coldReceiptOnce=True, concurrentEndOnce=True,
                                  coldPendingExactStart=True,
                                  liveRuleChangePreservesPending=True, originalResetAndReplay=True,
                                  newRunActualResult=again_actual)
                except BaseException as error:
                    report.update(status='FAIL', error=self.managed_pool.redact(repr(error)))
                    raise
                finally:
                    self.stop()
                    self.managed_pool.close()
                    report.update(wallSeconds=round(time.monotonic()-began, 3), resources={'process': 0, 'managedSchema': 0, 'managedAccount': 0, 'privateEnv': 0})
                    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
