"""Original weekend2x private calendar -> sweep -> Player upgrade -> card use.

Finite only Player14 and historical mission1 five stars. No EXP/resource/GUID
or battle-result injection. Root supplies qualified prebuilt app; no compiler.
Only isolated enabled flags choose the existing original-weekend2x source.
"""
import concurrent.futures
from collections import Counter
import datetime
import fcntl
import hashlib
import http.client as http_client
import json
import os
from pathlib import Path
import re
import secrets
import tempfile
import time
import types
import unittest
import uuid

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_p5_talent_scope_mysql import original_table
from server.codec import CLASSES, BY_NAME, parse_http
import test_continued_gameplay_mysql as wire

ROOT = Path(__file__).resolve().parents[1]


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def protocol_preflight():
    vectors = {
        'CSActivityInfo': {'activityId': 'allCopyDouble'},
        'CSCopyBoonInfo': {'activityId': 'allCopyDouble'},
        'CSCopySweep': {'chapterId': '1', 'missionId': '1'},
        'CSWarriorRecruitGet': {'costType': [2]},
        'CSWarriorRecruit': {'costType': 2, 'numType': 1, 'free': 1},
        'CSWarriorStrengthen': {'guid': 'actual-partner', 'cost': [{'id': 'hero_exp_006', 'guid': 'actual-donor'}]},
    }
    for name, fields in vectors.items():
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 121001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original request differs: ' + name)
    for name, fields in {
        'SCActivityInfo': {'param', 'startTime', 'endTime'},
        'SCCopyBoonInfo': {'activityId', 'information'},
        'SCCopySweep': {'gain', 'cost'}, 'SCPlayer': {'player'},
        'SCCopyPassedInfo': {'passedCopyInfo'}, 'SCWarriorStrengthen': {'guid', 'cost'},
    }.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original consumer differs: ' + name)
    return {'requestVectors': len(vectors), 'sourceWeekendMask': 65,
            'sourceCurrencyFactor': 2, 'sourceDailyMinutes': [0, 1440],
            'finitePreparation': 'only Player14 and mission1 historical five stars; no natural-win claim',
            'actualFirstSweep': {'exp': 280, 'coin': 4380, 'staminaCost': 10, 'upgradeStamina': 20},
            'fullOriginalResponseFrames': ['SCCopySweep', 'SCPlayer', 'SCCopyPassedInfo']}


def preflight():
    source = Path(os.environ['HYJS_COPY_EXPERIENCE_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_COPY_EXPERIENCE_BINARY']).resolve()
    validation = Path(os.environ['HYJS_COPY_EXPERIENCE_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use fixed direct candidate')
    manifest = source / 'manifest.json'
    paths = set()
    for entry in json.loads(manifest.read_bytes())['files']:
        path = (source / entry['path']).resolve()
        if not path.is_relative_to(source) or path in paths or sha(path) != entry['sha256']:
            raise AssertionError('Frozen pin differs: ' + entry['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Unpinned Go source')
    proof = json.loads(validation.read_bytes())
    if proof.get('qualified') is not True or (ROOT / proof['candidate']).resolve() != source:
        raise AssertionError('Exact root qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use exact prebuilt app without compiling')
    copy = (source / 'hyjs-server/service/copyService.go').read_text()
    sweep = (source / 'hyjs-server/service/copySweepService.go').read_text()
    if 'gain, LocalPlayAt(ctx, benefitAt))' not in copy or 's.ApplyP4CopyCurrencyBonus(tx, LocalPlayAt(ctx, benefitAt), coin, exp)' not in copy or 's.ApplyP4CopyCurrencyBonus(tx, LocalPlayAt(ctx, now), coin, exp)' not in sweep:
        raise AssertionError('Exact three calendar settlement hooks are missing')
    if 's.ApplyP4CopyDropBonus(tx, id, player, game, source, loot, LocalPlayAt(ctx, now))' not in sweep or 'P4CopySweepDropSourcePrefix' not in sweep:
        raise AssertionError('Actual loot-only sweep2x ledger integration is missing')
    for relative in ['server/codec.py', 'server/protocol/schema.json'] + [
            'analysis/tables/Table' + name + '.tsv' for name in
            ['CopyMission', 'CopyExp', 'CopyTask', 'UserExp', 'Hero', 'HeroExp', 'GameConfigure']]:
        if sha(source / relative) != sha(ROOT / relative):
            raise AssertionError('Original source/table differs: ' + relative)
    return source, binary, sha(manifest), protocol_preflight()


@unittest.skipUnless(os.environ.get('HYJS_COPY_EXPERIENCE_MYSQL_TEST') == '1', 'Root prebuilt managed window')
class LocalPlayCopyExperienceMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    stamped, profile, start, stop, sql = transport.stamped, transport.profile, transport.start, transport.stop, transport.sql
    json_request, wallet_body = transport.json_request, transport.wallet_body
    tools, change = transport.tools, transport.change

    def original_frames(self, name, fields, owner, stamp):
        raw = wire.encode_original_http(name, fields, owner.playerId, bytes(owner.sessionKey), stamp)
        connection = http_client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=40)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            self.assertEqual(response.status, 200)
            return wire.decode_responses(response.read())
        finally:
            connection.close()

    def test_original_weekend_personal_sweep_player_upgrade_material_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_COPY_EXPERIENCE_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/player-experience-r1'))
        self.assertFalse(output.exists(), 'Preserve prior reports')
        report = dict(status='PENDING', fullChain=False, binarySHA256=sha(binary), sourceManifestSHA256=manifest_sha,
                      runnerSHA256=sha(__file__), contract=contract, noBusinessResourceInjection=True,
                      naturalLevel14Claimed=False, nativeWinClaimed=False, phoneUI=False,
                      productionModified=False, externalPayment=False, phase='allocation')
        began = time.monotonic()
        self.pool, self.process = ManagedMySQL(), None
        with tempfile.TemporaryDirectory(prefix='hyjs-copy-experience-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('copy_experience')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-copy-exp-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '经验保护', 'heroId': 'hero_001'}, protected, 120000, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-copy-experience-' + suffix
                    owner = self.login(uid)
                    stamp = 121000

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    def once(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        saved = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            out = list(workers.map(lambda _: self.call(name, fields, owner, saved, expected), range(8)))
                        self.assertTrue(all(row == out[0] for row in out))
                        return out[0], saved

                    report['phase'] = 'finite-eligibility-real-free-partner'
                    http('CSPlayerCreate', {'playerName': '个人经验福利', 'heroId': 'hero_047'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual((initial[0]['level'], initial[0].get('exp', 0), initial[0].get('coin', 0), initial[0].get('diamond', 0), initial[0]['stamina']), (1, 0, 0, 0, 100))
                    passed = [{'chapterId': '1', 'missionId': '1', 'star': 5, 'challengeTimes': 0}]
                    encoded = json.dumps(passed, separators=(',', ':')).encode().hex()
                    self.pool.sql(self.database, "UPDATE profiles SET player=JSON_SET(player,'$.level',14), game=JSON_SET(game,'$.copy.passedCopyInfo',CAST(CONVERT(0x" + encoded + " USING utf8mb4) AS JSON)) WHERE id=" + str(owner.playerId) + " AND uid LIKE 'demo-copy-experience-%';")
                    finite = self.profile(owner)
                    self.assertEqual(finite[0], dict(initial[0], level=14))
                    expected = json.loads(json.dumps(initial[1]))
                    expected['copy']['passedCopyInfo'] = passed
                    self.assertEqual(finite[1], expected)
                    http('CSWarriorRecruitGet', {'costType': [2]}, 'SCWarriorRecruitGet')
                    recruited, _ = once('CSWarriorRecruit', {'costType': 2, 'numType': 1, 'free': 1}, 'SCWarriorRecruit')
                    self.assertEqual(len(getattr(recruited, 'get').wars), 1)
                    partner = getattr(recruited, 'get').wars[0]
                    self.assertFalse(partner.id.startswith('hero_exp_'))
                    # Publish only existing on/off choices. Preserve the actual weekend mask, minutes, factors and source.
                    # The original ID column is VARBINARY64; MySQL8 REGEXP rejects binary character sets.
                    # Decode the ID for this read and apply the exact legacy shape in Python.
                    source_ids = self.sql("SELECT CONVERT(id USING utf8mb4) FROM p4_copy_benefit_rules ORDER BY start_at DESC,id DESC;").splitlines()
                    eligible_sources = [value for value in source_ids if re.fullmatch(r'p4:copy:[0-9]{6}', value)]
                    self.assertTrue(eligible_sources, 'Existing default-off original-weekend source is missing')
                    source_id = eligible_sources[0]
                    source_columns = "JSON_OBJECT('weekdayMask',weekday_mask,'startMinute',start_minute,'endMinute',end_minute,'currencyFactor',currency_factor,'dropFactor',drop_factor,'source',source)"
                    source_before = json.loads(self.sql('SELECT ' + source_columns + " FROM p4_copy_benefit_rules WHERE id='" + source_id + "';").strip())
                    self.assertEqual(tuple(source_before[key] for key in ['weekdayMask', 'startMinute', 'endMinute', 'currencyFactor', 'dropFactor']), (65, 0, 1440, 2, 2))
                    self.pool.sql(self.database, "UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1; UPDATE p4_copy_benefit_rules SET enabled=0 WHERE id LIKE 'p4:copy:evening:%'; UPDATE p4_copy_benefit_rules SET enabled=1 WHERE id='" + source_id + "';")
                    report['phase'] = 'original-next-weekend-calendar-and-private-production'
                    visited = []
                    for _ in range(7):
                        state = self.change(owner, 'nextDay')
                        visited.append(state['offsetDays'])
                        go_weekday = (datetime.datetime.fromtimestamp(state['gameTimeMs'] / 1000, datetime.timezone(datetime.timedelta(hours=8))).weekday() + 1) % 7
                        if source_before['weekdayMask'] & (1 << go_weekday):
                            break
                    else:
                        self.fail('Original weekend did not occur within seven true personal days')
                    offset = state['offsetDays']
                    applied = self.tools(owner, 'activities/apply', action='open', activityId='CopyBenefits', requestId=str(uuid.uuid4()))
                    result = next(row for row in applied['results'] if row['id'] == 'CopyBenefits')
                    self.assertEqual(result['status'], 'open', result)
                    cycle_id = self.sql(f"SELECT cycle_id FROM local_play_activity_bindings WHERE player_id={owner.playerId} AND activity_id='CopyBenefits';").strip()
                    snapshot_hex = self.sql("SELECT HEX(snapshot) FROM local_play_activity_cycles WHERE id='" + cycle_id + "';").strip()
                    snapshot = json.loads(bytes.fromhex(snapshot_hex))
                    self.assertEqual(len(snapshot['copy']), 1)
                    rule = snapshot['copy'][0]
                    self.assertEqual(tuple(rule[k] for k in source_before if k != 'source'), tuple(source_before[k] for k in source_before if k != 'source'))
                    self.assertEqual(rule['source'], source_before['source'])
                    self.assertLess(state['realTimeMs'], rule['startAt'])
                    page = http('CSActivityInfo', {'activityId': 'allCopyDouble'}, 'SCActivityInfo')
                    self.assertEqual((page.param, page.startTime, page.endTime), ('2', rule['startAt'], rule['endAt']))
                    boon = http('CSCopyBoonInfo', {'activityId': 'allCopyDouble'}, 'SCCopyBoonInfo')
                    self.assertEqual(boon.activityId, 'copyDouble')
                    self.assertIn('2倍', boon.information)
                    report['phase'] = 'eight-original-sweeps-one-exp280-player15'
                    before = self.profile(owner)
                    sweep_fields = {'chapterId': '1', 'missionId': '1'}
                    stamp += 1
                    sweep_stamp = stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        packets = list(workers.map(lambda _: self.original_frames('CSCopySweep', sweep_fields, owner, sweep_stamp), range(8)))
                    self.assertTrue(all(packet == packets[0] for packet in packets))
                    frames = packets[0]
                    self.assertEqual([op for op, _ in frames], [BY_NAME[n] for n in ['SCCopySweep', 'SCPlayer', 'SCCopyPassedInfo']])
                    sweep, player_frame = frames[0][1], frames[1][1].player
                    self.assertEqual(sum(row.num for row in sweep.gain.simps if row.id == 'exp'), 280)
                    self.assertEqual(sum(row.num for row in sweep.gain.simps if row.id == 'coin'), 4380)
                    self.assertEqual([(row.id, row.num) for row in sweep.cost], [('stamina', 10)])
                    after = self.profile(owner)
                    self.assertEqual((after[0]['level'], after[0]['exp']), (15, 0))
                    self.assertEqual(after[0]['coin'] - before[0].get('coin', 0), 4380)
                    self.assertEqual(after[0]['stamina'], before[0]['stamina'] - 10 + 20)
                    self.assertEqual((player_frame.level, player_frame.exp, player_frame.stamina, player_frame.coin), (15, 0, after[0]['stamina'], after[0]['coin']))
                    report['phase'] = 'actual-loot-only-p4-grant-once-and-independent-guids'
                    drop_source = f'sweep:{sweep_stamp}:1/1'
                    grant_where = f"player_id={owner.playerId} AND source_key='{drop_source}'"
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p4_copy_benefit_grants WHERE ' + grant_where + ';').strip(), '1')
                    grant_query = ("SELECT CONCAT(CONVERT(source_key USING utf8mb4),'|',CONVERT(rule_id USING utf8mb4),'|',base_hash,'|',HEX(bonus)) FROM p4_copy_benefit_grants WHERE " + grant_where + ';')
                    grant_before = self.sql(grant_query).strip()
                    grant_source, grant_rule, base_hash, bonus_hex = grant_before.split('|')
                    self.assertEqual((grant_source, grant_rule), (drop_source, rule['id']))
                    self.assertRegex(base_hash, r'^[0-9a-f]{64}$')
                    bonus_raw = bytes.fromhex(bonus_hex)
                    bonus = CLASSES['ResultObject'].FromString(bonus_raw)
                    self.assertTrue(bonus.wars, 'Original nonzero card drop must produce a real independent2x bonus')
                    all_wars = list(sweep.gain.wars)
                    bonus_guids = {row.guid for row in bonus.wars}
                    all_guids = [row.guid for row in all_wars]
                    self.assertEqual(len(all_guids), len(set(all_guids)))
                    self.assertTrue(all(all_guids))
                    self.assertEqual(len(bonus_guids), len(bonus.wars))
                    self.assertEqual(len(all_wars), 2 * len(bonus.wars))
                    self.assertTrue(bonus_guids <= set(all_guids))
                    by_guid = {row.guid: row for row in all_wars}
                    for row in bonus.wars:
                        self.assertEqual(by_guid[row.guid], row)
                    base_wars = [row for row in all_wars if row.guid not in bonus_guids]
                    self.assertEqual(Counter(row.id for row in base_wars), Counter(row.id for row in bonus.wars))
                    self.assertFalse(any(row.id in {'coin', 'exp'} for row in bonus.simps), 'Currency must not enter loot2x ledger')
                    actual124 = sum(row.num for row in sweep.gain.simps if row.id == 'item_124')
                    bonus124 = sum(row.num for row in bonus.simps if row.id == 'item_124')
                    self.assertEqual(actual124 % 2, 0)
                    self.assertEqual(actual124, 2 * bonus124)
                    # Currency was asserted4380/280 above; it is not multiplied again by the loot-only stage.
                    report['phase'] = 'actual-sweep-exp-card-actual-owned-partner-consumer'
                    donors = [row for row in sweep.gain.wars if row.id == 'hero_exp_006']
                    self.assertTrue(donors, 'Actual normal nonzero-count hero_exp_006 pool must produce a real GUID')
                    donor = donors[0]
                    source_hero = next(row for row in original_table('Hero') if row['id'] == donor.id)
                    factor = int(next(row for row in original_table('GameConfigure') if row['StaticName'] == 'HeroStrengthCostExpToCoin')['NameValue'])
                    expected_coin = int(source_hero['expBase']) * factor
                    grow_fields = {'guid': partner.guid, 'cost': [{'id': donor.id, 'guid': donor.guid}]}
                    grown, grow_stamp = once('CSWarriorStrengthen', grow_fields, 'SCWarriorStrengthen')
                    self.assertGreater((grown.guid.level, grown.guid.exp), (partner.level, partner.exp))
                    final = self.profile(owner)
                    self.assertEqual(final[0]['coin'], after[0]['coin'] - expected_coin)
                    self.assertEqual((final[0]['level'], final[0]['exp']), (15, 0), 'Partner experience must not mutate Player EXP')
                    self.assertFalse(any(row['guid'] == donor.guid for row in final[1]['warriors']))
                    # Consumed awarded GUIDs make the original sweep receipt non-replayable; rejection must never resurrect it.
                    self.call('CSCopySweep', sweep_fields, owner, sweep_stamp, 'ErrorMessage')
                    self.assertEqual(self.profile(owner), final)
                    report['phase'] = 'rewind-and-cold-old-award-guard'
                    self.assertEqual(self.change(owner, 'reset')['offsetDays'], 0)
                    self.assertEqual(self.profile(owner), final)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.tools(owner, 'time/state')['offsetDays'], 0)
                    self.call('CSCopySweep', sweep_fields, owner, sweep_stamp, 'ErrorMessage')
                    self.assertEqual(self.call('CSWarriorStrengthen', grow_fields, owner, grow_stamp, 'SCWarriorStrengthen'), grown)
                    self.assertEqual(self.profile(owner), final)
                    self.assertEqual(self.sql(grant_query).strip(), grant_before)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p4_copy_benefit_grants WHERE ' + grant_where + ';').strip(), '1')
                    self.assertEqual(self.sql("SELECT HEX(snapshot) FROM local_play_activity_cycles WHERE id='" + cycle_id + "';").strip(), snapshot_hex)
                    self.assertEqual(json.loads(self.sql('SELECT ' + source_columns + " FROM p4_copy_benefit_rules WHERE id='" + source_id + "';").strip()), source_before)
                    self.assertEqual(self.profile(protected), protected_before)
                    report.update(status='PASS', fullChain=True, selectedSource=source_id, originalSource=source_before,
                                  visitedOffsets=visited, actualWeekendOffset=offset, privateCycleID=cycle_id,
                                  actualSweepEXP=280, actualSweepCoin=4380, actualUpgradeStamina=20,
                                  actualPlayerLevel=15, actualPlayerEXP=0, actualDonorGUID=donor.guid,
                                  actualPartnerID=partner.id, actualPartnerGUID=partner.guid,
                                  actualPartnerLevel=grown.guid.level, actualPartnerEXP=grown.guid.exp,
                                  actualPartnerUpgradeCoin=expected_coin,
                                  sweepDropSource=drop_source, sweepDropRule=grant_rule, sweepDropBaseHash=base_hash,
                                  sweepDropBonusSHA256=hashlib.sha256(bonus_raw).hexdigest(),
                                  actualWarDropCount=len(all_wars), actualBonusWarCount=len(bonus.wars),
                                  actualWarDropGUIDs=all_guids, actualBonusWarGUIDs=sorted(bonus_guids),
                                  actualItem124=actual124, actualBonusItem124=bonus124,
                                  actualLootDoubleLedgerOnce=True, actualCurrencyOnlyDouble=True,
                                  eightConcurrentSweepOnce=True,
                                  eightConcurrentConsumerOnce=True, fullThreeOriginalResponseFrames=True,
                                  oldConsumedAwardRejected=True, coldExact=True, protected1Exact=True,
                                  originalWeekdayAndMultiplierUnchanged=True,
                                  mainlineDropScope='existing same-source domain evidence only; no battle End forged by runner')
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
