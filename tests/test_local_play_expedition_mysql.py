"""Prebuilt original expedition -> personal calendar -> real reward/consumer.

Only Player32 is finite eligibility. Original free strong recruit obtains the
idle GUID; initial original energy and a funded six-yuan order pay all costs.
No task/result/resource/event/clock is inserted. Default-off event policy is
retained, so ordinary task is real; random risk branches reuse domain evidence.
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

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from server.codec import CLASSES, parse_http
import test_continued_gameplay_mysql as wire

ROOT = Path(__file__).resolve().parents[1]


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def protocol_preflight():
    vectors = {
        'CSInterExpedition': {}, 'CSGetRefreshedMissions': {}, 'CSGetAcceptedMissions': {},
        'CSRefreshMissionList': {},
        'CSGetExpeditionIdleHeros': {'missionSerial': 'p7e-actual'},
        'CSAcceptMission': {'missionSerial': 'p7e-actual', 'heroGuids': ['actual-owned-idle']},
        'CSExpeditionMissionDetail': {'missionSerial': 'p7e-actual'},
        'CSGetFinalReward': {'missionSerial': 'p7e-actual'},
        'CSWarriorRecruitGet': {'costType': [2]},
        'CSWarriorRecruit': {'costType': 2, 'numType': 1, 'free': 1},
        'CSItemUse': {'costItem': {'id': 'item_001', 'num': 1}},
        'CSFormationOn': {'formatPos': 2, 'id': 'hero_actual', 'guid': 'actual-owned-idle'},
    }
    for name, fields in vectors.items():
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 110001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original request differs: ' + name)
    for name, fields in {
        'SCInterExpedition': {'level', 'exp', 'freeRefreshTime'},
        'SCGetRefreshedMissions': {'refreshedMissions'},
        'SCGetAcceptedMissions': {'acceptedMissions'},
        'SCRefreshMissionList': {'refreshedMissions', 'freeRefreshTime', 'cost'},
        'SCGetExpeditionIdleHeros': {'canSendGuids', 'sendGuids'},
        'SCExpeditionMissionDetail': {'events', 'failExp'},
        'SCGetFinalReward': {'get', 'level', 'exp'},
        'ExpeditionMissionInfo': {'missionSerial', 'state', 'startTime', 'endTime'},
        'SCItemUse': {'get', 'cost'},
    }.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original response consumer differs: ' + name)
    return {'originalRequestVectors': len(vectors), 'finiteLevel': 32,
            'source': 'original free strong recruit; original initial energy100; actual six-yuan order',
            'eventScope': 'unchanged default-off hourly event policy; actual base reward only',
            'newCombatScope': False, 'phoneUI': False}


def preflight():
    source = Path(os.environ['HYJS_EXPEDITION_CALENDAR_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_EXPEDITION_CALENDAR_BINARY']).resolve()
    validation = Path(os.environ['HYJS_EXPEDITION_CALENDAR_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use direct fixed root candidate')
    manifest = source / 'manifest.json'
    paths = set()
    for entry in json.loads(manifest.read_bytes())['files']:
        path = (source / entry['path']).resolve()
        if not path.is_relative_to(source) or path in paths or sha(path) != entry['sha256']:
            raise AssertionError('Frozen pin differs: ' + entry['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Unpinned candidate Go source')
    proof = json.loads(validation.read_bytes())
    if proof.get('qualified') is not True or (ROOT / proof['candidate']).resolve() != source:
        raise AssertionError('Completed exact root qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use exact prebuilt app; do not compile here')
    required = {
        'hyjs-server/service/p7ExpeditionService.go': ['freezeP7ExpeditionCalendar(tx, mission, now)', 'saveP7ExpeditionCalendar(tx, state, now)', 's.p7ExpeditionMissionNow(tx, mission, now)'],
        'hyjs-server/service/p7ExpeditionEventService.go': ['s.p7ExpeditionMissionNow(tx, mission, now)'],
        'hyjs-server/service/localPlayExpeditionCalendarService.go': ['expeditionClockHash', 'c.AcceptedOffset'],
    }
    for relative, needles in required.items():
        if not all(needle in (source / relative).read_text() for needle in needles):
            raise AssertionError('Expedition integration missing: ' + relative)
    for relative in ['server/codec.py', 'server/protocol/schema.json'] + [
            'analysis/tables/Table' + name + '.tsv' for name in
            ['ExpeditionMission', 'ExpeditionLevel', 'ExpeditionStory', 'Hero', 'GameConfigure', 'Payment', 'Item']]:
        if sha(source / relative) != sha(ROOT / relative):
            raise AssertionError('Original codec/table differs: ' + relative)
    return source, binary, sha(manifest), protocol_preflight()


@unittest.skipUnless(os.environ.get('HYJS_EXPEDITION_CALENDAR_MYSQL_TEST') == '1',
                     'Root prebuilt managed SQL window required')
class LocalPlayExpeditionCalendarMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    stamped, profile, start, stop, sql = transport.stamped, transport.profile, transport.start, transport.stop, transport.sql
    json_request, wallet_body, wallet, order = transport.json_request, transport.wallet_body, transport.wallet, transport.order
    tools, change = transport.tools, transport.change

    def test_original_expedition_calendar_reward_consumer_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contracts = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_EXPEDITION_CALENDAR_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/expedition-calendar-r1'))
        self.assertFalse(output.exists(), 'Preserve prior attempts')
        report = dict(status='PENDING', fullChain=False, binarySHA256=sha(binary), sourceManifestSHA256=manifest_sha,
                      runnerSHA256=sha(__file__), contracts=contracts, noBusinessResourceInjection=True,
                      finitePreparation='Player32 only, no natural level-up claim', phoneUI=False,
                      productionModified=False, externalPayment=False, phase='allocation')
        began = time.monotonic()
        self.pool, self.process = ManagedMySQL(), None
        with tempfile.TemporaryDirectory(prefix='hyjs-expedition-calendar-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('expedition_calendar')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    self.assertEqual(self.sql('SELECT enabled FROM p7_expedition_policies WHERE id=1;').strip(), '0')
                    self.assertEqual(self.sql('SELECT enabled FROM p7_expedition_event_policies WHERE id=1;').strip(), '0')
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-expedition-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '远征保护', 'heroId': 'hero_001'}, protected, 110000, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-expedition-calendar-' + suffix
                    owner = self.login(uid)
                    stamp = 111000

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        saved = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            rows = list(workers.map(lambda _: self.call(name, fields, owner, saved, expected), range(8)))
                        self.assertTrue(all(row == rows[0] for row in rows))
                        return rows[0], saved

                    def item_count(item):
                        return sum(row['num'] for row in (self.profile(owner)[1].get('items') or []) if row['id'] == item)

                    def reject(name, fields):
                        before = self.profile(owner)
                        http(name, fields, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), before)

                    def current_day():
                        state = self.tools(owner, 'time/state')
                        return time.strftime('%Y-%m-%d', time.gmtime(state['gameTimeMs'] / 1000 + 8 * 3600))

                    def mission_row(serial):
                        raw = self.sql("SELECT CONCAT(HEX(quote),'|',quote_hash,'|',start_at,'|',end_at,'|',state) FROM p7_expedition_missions WHERE player_id=" + str(owner.playerId) + " AND serial='" + serial + "';").strip().split('|')
                        self.assertEqual(len(raw), 5)
                        self.assertEqual(hashlib.sha256(bytes.fromhex(raw[0])).hexdigest(), raw[1])
                        return raw, json.loads(bytes.fromhex(raw[0]))

                    report['phase'] = 'real-idle-partner-and-original-paid-diamond'
                    http('CSPlayerCreate', {'playerName': '远征个人日期', 'heroId': 'hero_047'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual((initial[0]['level'], initial[0].get('coin', 0), initial[0].get('diamond', 0), initial[0]['energy']), (1, 0, 0, 100))
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',32) WHERE id={owner.playerId} AND uid LIKE 'demo-expedition-calendar-%';")
                    finite = self.profile(owner)
                    self.assertEqual(finite, [dict(initial[0], level=32), initial[1]])
                    http('CSWarriorRecruitGet', {'costType': [2]}, 'SCWarriorRecruitGet')
                    recruited, _ = once('CSWarriorRecruit', {'costType': 2, 'numType': 1, 'free': 1}, 'SCWarriorRecruit')
                    self.assertEqual(len(getattr(recruited, 'get').wars), 1)
                    hero = getattr(recruited, 'get').wars[0]
                    self.assertNotEqual(hero.guid, finite[0]['heroWarId'])
                    self.assertFalse(hero.id.startswith('hero_exp_'))
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=600)
                    paid = self.order(owner, 'djsy.wuyou.andp0006', 112000)
                    self.assertEqual(paid.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    paid_diamond = self.profile(owner)[0]['diamond']
                    self.assertGreaterEqual(paid_diamond, 40)
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    payment_before = self.sql("SELECT created_at,confirmed_at,cents FROM p3_payment_orders WHERE id='" + paid.orderId + "';").strip()
                    reject('CSInterExpedition', {})
                    # Only existing expedition publication is enabled; original quote/source/event defaults stay unchanged.
                    self.pool.sql(self.database, 'UPDATE p7_expedition_policies SET enabled=1 WHERE id=1 AND enabled=0;')
                    engine = self.sql("SELECT DISTINCT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('p7_expedition_missions','p7_expedition_states','p7_expedition_heroes','p7_rush_receipts','local_play_activity_operations');").strip().splitlines()
                    self.assertEqual(engine, ['InnoDB'])
                    report['phase'] = 'two-actual-personal-days-free-paid-refresh'
                    refresh_days = []
                    refresh_receipts = []
                    for day in range(2):
                        if day:
                            self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], 1)
                        day_key = current_day()
                        refresh_days.append(day_key)
                        inter = http('CSInterExpedition', {}, 'SCInterExpedition')
                        self.assertEqual(inter.freeRefreshTime, 2)
                        for number in range(3):
                            if number == 2:
                                refreshed, saved = once('CSRefreshMissionList', {}, 'SCRefreshMissionList')
                                refresh_receipts.append((refreshed, saved))
                            else:
                                refreshed = http('CSRefreshMissionList', {}, 'SCRefreshMissionList')
                            self.assertEqual(refreshed.freeRefreshTime, max(0, 1 - number))
                            self.assertEqual(len(refreshed.refreshedMissions), 2)
                            self.assertEqual(refreshed.cost.num, 20 if number == 2 else 0)
                            if number == 2:
                                self.assertEqual(refreshed.cost.id, 'diamond')
                    self.assertNotEqual(refresh_days[0], refresh_days[1])
                    self.assertEqual(self.profile(owner)[0]['diamond'], paid_diamond - 40)
                    selected = refreshed.refreshedMissions[0]
                    serial = selected.missionSerial
                    offered_row, quote = mission_row(serial)
                    self.assertIn(quote['hours'], [4, 8, 12])
                    self.assertEqual(quote['energy'], 2 * quote['hours'])
                    self.assertEqual((quote['reward'], quote.get('events')), ('item_001,1', None))
                    idle = http('CSGetExpeditionIdleHeros', {'missionSerial': serial}, 'SCGetExpeditionIdleHeros')
                    self.assertIn(hero.guid, idle.canSendGuids)
                    report['phase'] = 'original-accept-hero-hold-and-immutable-clock'
                    energy_before = self.profile(owner)[0]['energy']
                    _, accept_stamp = once('CSAcceptMission', {'missionSerial': serial, 'heroGuids': [hero.guid]}, 'SCAcceptMission')
                    self.assertEqual(self.profile(owner)[0]['energy'], energy_before - quote['energy'])
                    frozen_row, _ = mission_row(serial)
                    self.assertEqual(frozen_row[:2], offered_row[:2])
                    self.assertEqual(int(frozen_row[3]) - int(frozen_row[2]), quote['hours'] * 3600000)
                    clock_before = self.sql("SELECT HEX(response) FROM local_play_activity_operations WHERE player_id=" + str(owner.playerId) + " AND request_id='exp-clock:" + serial + "';").strip()
                    clock = json.loads(bytes.fromhex(clock_before))
                    self.assertEqual((clock['playerId'], clock['serial'], clock['quoteHash'], clock['acceptedOffset']), (owner.playerId, serial, frozen_row[1], 1))
                    self.assertEqual((clock['startAt'], clock['endAt']), (int(frozen_row[2]), int(frozen_row[3])))
                    self.assertEqual(clock['businessAt'] - clock['startAt'], 86400000)
                    reject('CSGetFinalReward', {'missionSerial': serial})
                    reject('CSFormationOn', {'formatPos': 2, 'id': hero.id, 'guid': hero.guid})
                    detail = http('CSExpeditionMissionDetail', {'missionSerial': serial}, 'SCExpeditionMissionDetail')
                    self.assertEqual(len(detail.events), 0)
                    self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], 2)
                    complete_day = current_day()
                    ready = http('CSGetAcceptedMissions', {}, 'SCGetAcceptedMissions')
                    task = next(row for row in ready.acceptedMissions if row.missionSerial == serial)
                    self.assertEqual(task.state, 2)
                    self.assertEqual((task.startTime, task.endTime), (int(frozen_row[2]) + 86400000, int(frozen_row[3]) + 86400000))
                    report['phase'] = 'actual-final-reward-original-item-consumer'
                    before_reward = item_count('item_001')
                    final_reward, final_stamp = once('CSGetFinalReward', {'missionSerial': serial}, 'SCGetFinalReward')
                    self.assertEqual(final_reward.exp, quote['exp'])
                    self.assertEqual([(row.id, row.num) for row in getattr(final_reward, 'get').simps], [('item_001', 1)])
                    self.assertEqual(item_count('item_001'), before_reward + 1)
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p7_expedition_heroes WHERE player_id={owner.playerId};').strip(), '0')
                    energy_before_use = self.profile(owner)[0]['energy']
                    use_fields = {'costItem': {'id': 'item_001', 'num': 1}}
                    used, use_stamp = once('CSItemUse', use_fields, 'SCItemUse')
                    self.assertEqual((used.cost.id, used.cost.num), ('item_001', 1))
                    self.assertEqual(self.profile(owner)[0]['energy'], energy_before_use + 10)
                    self.assertEqual(item_count('item_001'), before_reward)
                    report['phase'] = 'rewind-day-budget-claimed-replay-and-cold'
                    self.assertEqual(self.change(owner, 'reset')['offsetDays'], 0)
                    self.assertEqual(http('CSInterExpedition', {}, 'SCInterExpedition').freeRefreshTime, 0)
                    self.assertEqual(http('CSGetFinalReward', {'missionSerial': serial}, 'SCGetFinalReward'), final_reward)
                    self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], 1)
                    self.assertEqual(http('CSInterExpedition', {}, 'SCInterExpedition').freeRefreshTime, 0)
                    before_cold = self.profile(owner)
                    self.assertEqual(before_cold[0]['diamond'], paid_diamond - 40)
                    self.assertIsNone(before_cold[1].get('run'))
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), before_cold)
                    self.assertEqual(self.tools(owner, 'time/state')['offsetDays'], 1)
                    self.assertEqual(self.call('CSGetFinalReward', {'missionSerial': serial}, owner, final_stamp, 'SCGetFinalReward'), final_reward)
                    self.assertEqual(self.call('CSItemUse', use_fields, owner, use_stamp, 'SCItemUse'), used)
                    for response, saved in refresh_receipts:
                        self.assertEqual(self.call('CSRefreshMissionList', {}, owner, saved, 'SCRefreshMissionList'), response)
                    self.assertEqual(self.profile(owner), before_cold)
                    after_row, _ = mission_row(serial)
                    self.assertEqual(after_row[:4], frozen_row[:4])
                    self.assertEqual(after_row[4], '3')
                    self.assertEqual(self.sql("SELECT HEX(response) FROM local_play_activity_operations WHERE player_id=" + str(owner.playerId) + " AND request_id='exp-clock:" + serial + "';").strip(), clock_before)
                    facts = self.sql(f"SELECT kind,day,amount FROM p1_event_facts WHERE player_id={owner.playerId} AND (source_key='exp-accept:{serial}' OR source_key='exp-finish:{serial}' OR source_key LIKE 'exp-refresh:%') ORDER BY id;").strip().splitlines()
                    parsed = [row.split('\t') for row in facts]
                    self.assertEqual(sum(int(row[2]) for row in parsed if row[0] == 'diamond_spent'), 40)
                    self.assertEqual([(row[1], row[2]) for row in parsed if row[0] == 'expedition_complete'], [(complete_day, '1')])
                    self.assertEqual(self.sql("SELECT created_at,confirmed_at,cents FROM p3_payment_orders WHERE id='" + paid.orderId + "';").strip(), payment_before)
                    self.assertEqual(self.profile(protected), protected_before)
                    self.assertEqual(self.sql('SELECT enabled FROM p7_expedition_event_policies WHERE id=1;').strip(), '0')
                    report.update(status='PASS', fullChain=True, orderId=paid.orderId,
                                  actualIdleHeroID=hero.id, actualIdleGUID=hero.guid, missionSerial=serial,
                                  frozenQuote=quote, frozenClock=clock, refreshDays=refresh_days,
                                  refreshDiamondSpent=40, actualAcceptEnergy=quote['energy'],
                                  actualReward='item_001,1', actualRewardEnergyConsumer=10,
                                  completionDay=complete_day, finalFacts=parsed,
                                  eightSameStampOnce=True, coldExact=True, protected1Exact=True,
                                  paymentRealClockUnchanged=True, originalQuoteRealTimesUnchanged=True,
                                  defaultEventPolicyUnchanged=True)
                except BaseException as error:
                    report.update(status='FAIL', failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    self.stop()
                    self.pool.close()
                    report['seconds'] = round(time.monotonic() - began, 3)
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    output.parent.mkdir(parents=True, exist_ok=True)
                    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
