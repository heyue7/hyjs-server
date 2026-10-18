"""Prebuilt personal Trial quota and first-section pending recovery.

Only Player22 is finite. Currency comes from an actual local wallet order;
every floor outcome comes from its accepted frozen calculation. No score,
win, progress, inventory or receipt is inserted.
"""
import concurrent.futures
import fcntl
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import types
import unittest
import uuid

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport, sha
from server.codec import CLASSES, parse_http
import test_continued_gameplay_mysql as wire

ROOT = Path(__file__).resolve().parents[1]


def preflight():
    for name, fields in {'CSGetRushEliteBossInfo': {}, 'CSRushCurrentInfo': {},
                         'CSRushResetChallengeCount': {},
                         'CSRushFightStart': {'rushType': 1, 'taskId': 'local-section-1', 'star': 1}}.items():
        assert parse_http(wire.encode_original_http(name, fields, 2, b'0'*16, 100)).decode() == CLASSES[name](**fields)
    source = Path(os.environ['HYJS_RUSH_CALENDAR_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_RUSH_CALENDAR_BINARY']).resolve()
    proof = json.loads(Path(os.environ['HYJS_RUSH_CALENDAR_VALIDATION']).read_bytes())
    assert source.parent == ROOT/'build/candidates' and proof['qualified'] is True
    assert (ROOT/proof['candidate']).resolve() == source and sha(binary) == proof['binarySHA256']
    pins = set()
    for row in json.loads((source/'manifest.json').read_bytes())['files']:
        path = (source/row['path']).resolve()
        assert path.is_relative_to(source) and path not in pins and sha(path) == row['sha256']
        pins.add(path)
    assert not set((source/'hyjs-server').rglob('*.go'))-pins
    for path in ['server/codec.py', 'server/protocol/schema.json']:
        assert sha(source/path) == sha(ROOT/path)
    assert 'saveP7RushCalendar(tx, state, now)' in (source/'hyjs-server/service/p7RushService.go').read_text()
    assert 'stage.Source != constUtil.P7SectionStageSource' in (source/'hyjs-server/service/p7RushContinuationService.go').read_text()
    return source, binary


@unittest.skipUnless(os.environ.get('HYJS_RUSH_CALENDAR_MYSQL_TEST') == '1', 'Root managed prebuilt window only')
class LocalPlayRushCalendarMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    stamped, profile, start, stop, sql = transport.stamped, transport.profile, transport.start, transport.stop, transport.sql
    json_request, tools, change = transport.json_request, transport.tools, transport.change
    wallet_body, wallet = transport.wallet_body, transport.wallet

    def test_real_wins_paid_reset_day_revisit_and_first_section_cold_pending(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_RUSH_CALENDAR_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT/'analysis/growth-first-20261016'))
        self.assertFalse(output.exists())
        report = dict(status='PENDING', runnerSHA256=sha(__file__), binarySHA256=sha(binary),
                      finitePreparation='Only Player22; actual original Create047 and real wallet30 order',
                      seededWinnerScoreCurrencyInventory=False, phoneUI=False, actualBattles=[])
        began = time.monotonic()
        self.pool, self.process = ManagedMySQL(), None
        with tempfile.TemporaryDirectory(prefix='hyjs-rush-calendar-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('rush_calendar')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH','HOME','TMPDIR','LANG','LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    now = int(time.time()*1000)
                    self.pool.sql(self.database, 'UPDATE local_play_policies SET enabled=1 WHERE id=1;'
                             'UPDATE local_wallet_policies SET enabled=1 WHERE id=1;'
                             "UPDATE p7_rush_stage_rules SET enabled=1 WHERE id LIKE 'local-section:%';"
                             'UPDATE p7_rush_policies SET enabled=1 WHERE id=1;'
                             'UPDATE p7_rush_scoped_policies SET enabled=1 WHERE id=1;'
                             'UPDATE p7_rush_manual_policies SET enabled=0 WHERE id=1;'
                             f'UPDATE p7_section_reward_policies SET enabled=1,published_at={now-1000},start_at={now-1000},end_at={now+3600000} WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-rush-calendar-protected-'+suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName':'试炼保护','heroId':'hero_047'}, protected, 1, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-rush-calendar-'+suffix
                    owner = self.login(uid)
                    stamp = int(time.time()*1000)

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once_reset():
                        nonlocal stamp
                        stamp += 1
                        saved = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                            rows = list(pool.map(lambda _: self.call('CSRushResetChallengeCount', {}, owner, saved, 'SCRushResetChallengeCount'), range(8)))
                        self.assertTrue(all(r == rows[0] for r in rows))
                        return rows[0], saved

                    def state():
                        http('CSGetRushEliteBossInfo', {}, 'SCGetRushEliteBossInfo')
                        return self.sql(f'SELECT day,run_id,floor,free_reset_used,paid_reset_used,elite_used,elite_bought,active_fight FROM p7_rush_states WHERE player_id={owner.playerId};').strip().split('\t')

                    http('CSPlayerCreate', {'playerName':'原试炼日历','heroId':'hero_047'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual(initial[0].get('diamond', 0), 0)
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=3000)
                    product = next(p for p in http('CSGetPaymentInfo', {'channelId':'AND0003','shopId':'common'}, 'SCGetPaymentInfo').paymentInfoList if p.paymentId == 'djsy.wuyou.andp0030')
                    order = http('CSGenerateOrder', {'paymentId':product.paymentId,'money':product.money,'moneyType':product.moneyType,'channelId':'AND0003','shopId':product.shopId}, 'SCGenerateOrder')
                    self.assertEqual(order.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',22) WHERE id={owner.playerId};")
                    http('CSFormationList', {}, 'SCFormationList')
                    paid_diamond = self.profile(owner)[0]['diamond']
                    first_receipt = None
                    for run in range(3):
                        for floor in range(1, 4):
                            request = dict(rushType=1, taskId=f'local-section-{floor}', star=1)
                            start = http('CSRushFightStart', request, 'SCRushFightStart')
                            terminal = json.loads(bytes.fromhex(self.sql("SELECT HEX(terminal) FROM p7_rush_scoped_calculations WHERE fight_id='"+start.fightId+"';").strip()))
                            outcome = terminal['outcome']
                            actual = int(outcome['winner'] == 0)
                            report['actualBattles'].append(dict(fightId=start.fightId, run=run+1, floor=floor, actualWinner=outcome['winner'], finalRound=outcome['finalRound']))
                            self.assertEqual(actual, 1, 'Preserve actual loss; do not rewrite winner')
                            end = http('CSRushFightEnd', dict(fightId=start.fightId,result=actual,optRecord=terminal['originalOperates']), 'SCRushFightEnd')
                            self.assertEqual((end.result,end.star), (1,1))
                        if run == 2:
                            self.change(owner, 'nextDay')
                        reset, saved = once_reset()
                        if run == 0: first_receipt = (reset, saved)
                        self.assertEqual([(r.id,r.num) for r in reset.cost], [('diamond',50)] if run == 1 else [])
                    tomorrow = state()
                    self.assertEqual(tomorrow[1:5], ['4','1','1','0'])
                    self.change(owner, 'previousDay')
                    yesterday = state()
                    self.assertEqual(yesterday[1:5], ['4','1','1','1'])
                    request = dict(rushType=1,taskId='local-section-1',star=1)
                    pending = http('CSRushFightStart', request, 'SCRushFightStart')
                    self.change(owner, 'nextDay')
                    self.assertEqual(http('CSRushFightStart', request, 'SCRushFightStart'), pending)
                    saved = self.profile(owner)
                    self.stop(); self.start(); owner = self.login(uid)
                    self.assertEqual(self.profile(owner), saved)
                    info = http('CSRushCurrentInfo', {}, 'SCRushCurrentInfo')
                    self.assertEqual([(t.taskId,t.star) for t in info.taskInfoList], [('local-section-1',1)])
                    self.assertEqual(http('CSRushFightStart', request, 'SCRushFightStart'), pending)
                    held = self.profile(owner)
                    http('CSRushFightStart', dict(request,star=2), 'ErrorMessage')
                    self.assertEqual(self.call('CSRushResetChallengeCount', {}, owner, first_receipt[1], 'SCRushResetChallengeCount'), first_receipt[0])
                    self.assertEqual(self.profile(owner), held)
                    self.assertEqual(held[0]['diamond'], paid_diamond-50)
                    self.change(owner, 'previousDay')
                    returned = state()
                    self.assertEqual(returned[1:5], ['4','1','1','1'])
                    self.assertEqual(returned[-1], pending.fightId)
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p7_rush_fights WHERE player_id={owner.playerId};').strip(),'10')
                    archives = self.sql(f"SELECT HEX(response) FROM local_play_activity_operations WHERE player_id={owner.playerId} AND action='rush-day' ORDER BY request_id;").split()
                    quotas = [json.loads(bytes.fromhex(raw)) for raw in archives]
                    self.assertEqual(len(quotas),2)
                    self.assertEqual(sorted((q['freeResetUsed'],q['paidResetUsed']) for q in quotas),[(1,0),(1,1)])
                    self.assertEqual(self.profile(protected),protected_before)
                    report.update(status='PASS', fullChain=True, actualAcceptedWins=9, pendingFight=pending.fightId,
                                  actualResetDiamondCost=50, originalPaidOrderCents=3000, twoDaysRestored=True,
                                  eightSameStampOnce=True, coldPendingExact=True, oldReceiptNoNewReset=True, protected1Exact=True)
                except BaseException as error:
                    report.update(status='FAIL', failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    self.stop(); self.pool.close()
                    report.update(seconds=round(time.monotonic()-began,3), ownedManagedAllocationsRemaining=len(self.pool.connections))
                    output.parent.mkdir(parents=True,exist_ok=True)
                    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__ == '__main__': unittest.main()
