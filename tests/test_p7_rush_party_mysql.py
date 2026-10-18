"""Opt-in prebuilt original HTTP/InnoDB P7 complete001003 party acceptance.

The explicit finite prerequisite imports only the captured equipped party and
its required owned037/Pet002 instances. Fresh wallet/items and all P7 progress
remain untouched. Actual account4 and natural card acquisition are separate.
No Go compilation, phone access, supplied result/receipt/inventory seeding.
"""
import concurrent.futures
import base64
import copy
import fcntl
import hashlib
import json
import math
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import unittest

from hyjs_managed_mysql import ManagedMySQL
from test_p7_maze_trader_mysql import P7MazeTraderMySQLTests as Transport
from server.codec import message

ROOT = Path(__file__).resolve().parents[1]
ROSTER = ROOT / 'analysis/p7/party-rush-isolated-roster-r2-20261016.json'
ROSTER_SHA = 'e60e9774a43efe58f67b9eb84c1a30cf40a3a30f2e5f68c87d063d3aeb29a5c9'
PROOF = 'c3c8a25cc91889a21e5bedb5100b2b342b030a1537f014a95a3f77d750711798'


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def historical_roster():
    if digest(ROSTER) != ROSTER_SHA:
        raise RuntimeError('Finite historical roster changed')
    source = json.loads(ROSTER.read_bytes())
    for key in ('source', 'petSource'):
        binding = source[key]
        if digest(ROOT / binding['path']) != binding['sha256']:
            raise RuntimeError('Historical source binding changed')
    if (set(source['playerPatch']) != {'level', 'heroWarId'} or
            set(source['gamePatch']) != {'warriors', 'formation', 'equipment'} or
            [hero['id'] for hero in source['gamePatch']['warriors']] != ['hero_001', 'hero_003'] or
            [gear['equipId'] for gear in source['gamePatch']['equipment']] != ['equip_037']):
        raise RuntimeError('Finite roster must not import wallets/items/progress')
    pet = message('Pet')
    pet_raw = bytes.fromhex(source['pet']['wireHex'])
    pet.ParseFromString(pet_raw)
    if (pet.id, pet.guid) != (source['pet']['id'], source['pet']['guid']) or pet.id != 'pet_002':
        raise RuntimeError('Historical necessary owned Pet source differs')
    original = json.loads((ROOT / source['source']['path']).read_bytes())[0]['game']
    projected = next(item for item in original['kv'] if item['key'] == source['petSource']['projectionKey'])
    if (base64.b64decode(projected['value'], validate=True) != pet_raw or
            hashlib.sha256(pet_raw).hexdigest() != source['petSource']['protobufSHA256']):
        raise RuntimeError('Owned Pet must match the same historical equipped roster projection')
    return source


def fixture_sql(player_id, source):
    if type(player_id) is not int or player_id <= 0:
        raise RuntimeError('New isolated account ID required')
    encoded = lambda value: json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode().hex()
    player = source['playerPatch']
    patch = ','.join("'$." + key + "',CAST(CONVERT(0x" + encoded(value) + ' USING utf8mb4) AS JSON)'
                     for key, value in source['gamePatch'].items())
    pet = source['pet']
    # Only historical equipped prerequisites are imported. No result, balance,
    # ordinary items, P7 state or any previous receipt appears in this SQL.
    return (f"UPDATE profiles SET player=JSON_SET(player,'$.level',{int(player['level'])},"
            f"'$.heroWarId',CONVERT(0x{player['heroWarId'].encode().hex()} USING utf8mb4)),"
            f'game=JSON_SET(game,{patch}) WHERE id={player_id}; '
            'INSERT INTO p9_pet_instances(guid,player_id,pet_id,data,created_at,source) VALUES('
            f'CONVERT(0x{pet["guid"].encode().hex()} USING utf8mb4),{player_id},'
            f'CONVERT(0x{pet["id"].encode().hex()} USING utf8mb4),0x{pet["wireHex"]},'
            f'{int(time.time()*1000)},"finite historical equipped roster SHA{ROSTER_SHA}; no natural acquisition claim");')


def perk_parameters():
    """Explicit existing choices only; defaults retain the verified first path."""
    star = int(os.environ.get('HYJS_P7_PARTY_STAR', '1'))
    last_floor = int(os.environ.get('HYJS_P7_PARTY_LAST_FLOOR', '4'))
    if star not in (1, 2, 3) or last_floor not in (4, 6):
        raise RuntimeError('Use original star1/2/3 and existing published floor4/6 only')
    return star, last_floor


@unittest.skipUnless(os.environ.get('HYJS_P7_PARTY_MYSQL_TEST') == '1', 'Root coordinated managed SQL lease')
class P7RushPartyMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql

    def test_actual_party_stars_addition_next_floor_reward_shop_and_cold_once(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        roster = historical_roster()
        star_choice, last_floor = perk_parameters()
        self.source = Path(os.environ['HYJS_P7_PARTY_SOURCE']).resolve()
        self.binary = Path(os.environ['HYJS_P7_PARTY_BINARY']).resolve()
        calculator = Path(os.environ['HYJS_P7_PARTY_SERVICE_TEST']).resolve()
        self.assertEqual(self.source.parent, ROOT / 'build/candidates')
        self.assertEqual(digest(self.binary), os.environ['HYJS_P7_PARTY_BINARY_SHA256'])
        self.assertEqual(digest(calculator), os.environ['HYJS_P7_PARTY_SERVICE_TEST_SHA256'])
        self.assertIn(PROOF, (self.source / 'hyjs-server/constUtil/p7RushParty.go').read_text())
        certificate = ROOT / 'analysis/p6/party-rush-manual-capability-certificate-20261016.json'
        self.assertEqual(digest(certificate), PROOF)
        output = Path(os.environ['HYJS_P7_PARTY_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/p7'))
        self.assertFalse(output.exists(), 'Each acceptance keeps its own immutable record')
        evidence = output.with_name(output.stem + '-wire')
        self.assertFalse(evidence.exists())
        evidence.mkdir(parents=True)
        report = {'status': 'PENDING', 'binarySHA256': digest(self.binary),
                  'serviceTestSHA256': digest(calculator), 'source': str(self.source),
                  'scopeProof': PROOF, 'rosterFixtureSHA256': ROSTER_SHA,
                  'finitePrerequisite': roster['scope'], 'ordinaryWalletInventoryImported': False,
                  'resultOrReceiptSeeded': False, 'formalAccount4Modified': False,
                  'phoneUI': False, 'arbitraryRepeatedManualControlsSupported': False,
                  'starChoice': star_choice, 'lastFloor': last_floor,
                  'perkCost': star_choice * 3,
                  'actualBattles': []}
        began = time.monotonic()
        self.managed_pool = ManagedMySQL()
        with tempfile.TemporaryDirectory(prefix='hyjs-p7-party-') as temporary:
            self.native_log = Path(temporary) / 'native.log'
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    allocation = self.managed_pool.allocate('p7_party_http')
                    self.database, self.test_user = allocation.database, allocation.user
                    self.test_password = allocation.password
                    self.env = dict(os.environ, **allocation.env(), GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    tables = ('profiles', 'p6_battle_records', 'p7_rush_fights',
                              'p7_rush_manual_calculations', 'p7_rush_receipts',
                              'p7_rush_addition_offers', 'p9_pet_instances')
                    engines = self.sql('SELECT CONCAT(TABLE_NAME,"|",ENGINE) FROM information_schema.TABLES '
                                       'WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN (' +
                                       ','.join('"' + name + '"' for name in tables) + ');').splitlines()
                    self.assertEqual({row.split('|')[0] for row in engines}, set(tables))
                    self.assertTrue(all(row.endswith('|InnoDB') for row in engines))
                    activation = ROOT / 'analysis/p7/rush-continuation-activate-20261016.sql'
                    report['activationSHA256'] = digest(activation)
                    self.sql(activation.read_text())
                    identity = 'original-party-' + secrets.token_hex(6)
                    owner = self.login(identity)
                    stamp = 307000

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)

                    http('CSPlayerCreate', {'playerName': '原双人试炼', 'heroId': 'hero_001'}, 'SCPlayerCreate')
                    fresh = self.profile(owner)
                    self.assertEqual(tuple(fresh[0].get(key, 0) for key in ('coin', 'diamond', 'point')), (0, 0, 0))
                    self.sql(fixture_sql(owner.playerId, roster))
                    prepared = self.profile(owner)
                    expected_profile = copy.deepcopy(fresh)
                    expected_profile[0].update(roster['playerPatch'])
                    expected_profile[1].update(roster['gamePatch'])
                    self.assertEqual(prepared, expected_profile, 'Finite prerequisite must change only declared fields')
                    # The original guard action publishes its owned Pet snapshot;
                    # merely copying an owning row is not a usable P5 projection.
                    http('CSFormationOn', {'id': roster['pet']['id'], 'guid': roster['pet']['guid'], 'formatPos': 1}, 'SCFormationOn')
                    guarded = self.profile(owner)
                    self.assertEqual(tuple(guarded[0].get(key, 0) for key in ('coin', 'diamond', 'point')), (0, 0, 0))
                    self.assertEqual(guarded[1].get('items'), fresh[1].get('items'))
                    projected = next(item for item in guarded[1]['kv'] if item['key'] == roster['petSource']['projectionKey'])
                    self.assertEqual(base64.b64decode(projected['value'], validate=True), bytes.fromhex(roster['pet']['wireHex']))
                    report['petSnapshotPublishedByOriginalGuard'] = True
                    http('CSFormationList', {}, 'SCFormationList')

                    def ledger():
                        return self.sql(f'SELECT CONCAT(COUNT(*),"|",SUM(status=1)) FROM p7_rush_manual_calculations WHERE player_id={owner.playerId};').strip()

                    def actual_start(floor):
                        nonlocal stamp
                        tasks = http('CSRushCurrentInfo', {}, 'SCRushCurrentInfo').taskInfoList
                        self.assertEqual(sorted(t.star for t in tasks), [1, 2, 3])
                        task = next(t for t in tasks if t.star == star_choice)
                        fields = {'rushType': 1, 'taskId': task.taskId, 'star': star_choice}
                        stamp += 1
                        start_stamp = stamp
                        start = self.call('CSRushFightStart', fields, owner, start_stamp, 'SCRushFightStart')
                        self.assertEqual([hero.id for hero in start.battlePlayer.formation], ['hero_001', 'hero_003'])
                        self.assertTrue(all(not hero.talentList for hero in start.battlePlayer.formation))
                        start_file = evidence / f'{floor:03d}.SCRushFightStart.pb'
                        start_file.write_bytes(start.SerializeToString(deterministic=True))
                        terminal_file = evidence / f'{floor:03d}.calculated.json'
                        env = dict(self.env, HYJS_P7_PARTY_HTTP_START=str(start_file),
                                   HYJS_P7_PARTY_HTTP_START_SHA256=digest(start_file),
                                   HYJS_P7_PARTY_HTTP_AUTO_AT='30.1', HYJS_P7_PARTY_HTTP_OUTPUT=str(terminal_file))
                        run = subprocess.run([str(calculator), '-test.run=^TestP7PartyRushExportOriginalHTTPStart$', '-test.v'],
                                             cwd=ROOT / 'hyjs-server/service', env=env,
                                             capture_output=True, text=True, timeout=120)
                        self.assertEqual(run.returncode, 0, (run.stdout + run.stderr)[-3000:])
                        self.assertIn('--- PASS: TestP7PartyRushExportOriginalHTTPStart ', run.stdout)
                        terminal = json.loads(terminal_file.read_bytes())
                        self.assertFalse(terminal['outcome']['authoritative'])
                        actual = int(terminal['outcome']['winner'] == 0)
                        end = {'fightId': start.fightId, 'result': actual, 'optRecord': terminal['originalOperates']}
                        frozen = self.sql('SELECT CONCAT(driver_policy,"|",scope_proof,"|",status,"|",LENGTH(COALESCE(terminal,""))) '
                                          f'FROM p7_rush_manual_calculations WHERE player_id={owner.playerId} AND fight_id="{start.fightId}";').strip().split('|')
                        self.assertEqual(frozen[1:], [PROOF, '0', '0'])
                        self.assertIn('party001003', frozen[0])
                        report['actualBattles'].append({'floor': floor, 'fightId': start.fightId,
                                                       'sourceSHA256': digest(start_file), 'calculationSHA256': digest(terminal_file),
                                                       'actualWinner': terminal['outcome']['winner'],
                                                       'actualFinalRound': terminal['outcome']['finalRound']})
                        return start, end, fields, start_stamp

                    baseline_props = []
                    for floor in (1, 2, 3):
                        start, end, _, _ = actual_start(floor)
                        self.assertEqual(end['result'], 1, 'Preserve a real loss; never replace its winner')
                        before_end = self.profile(owner)
                        for variant in ('false-result', 'hp', 'transform', 'duplicate-operation') if floor == 1 else ():
                            bad = copy.deepcopy(end)
                            if variant == 'false-result':
                                bad['result'] = 0
                            elif variant == 'hp':
                                bad['optRecord']['attackerTotalLeftHp'] += 1
                            elif variant == 'transform':
                                bad['optRecord']['attackerTransOpt'] = {'roundNum': 1}
                            else:
                                bad['optRecord']['skillOpts'].append(copy.deepcopy(bad['optRecord']['skillOpts'][0]))
                            http('CSRushFightEnd', bad, 'ErrorMessage')
                            self.assertEqual(self.profile(owner), before_end)
                            self.assertEqual(ledger(), '1|0')
                        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                            replies = list(pool.map(lambda _: self.call('CSRushFightEnd', end, owner, stamp + 1, 'SCRushFightEnd'), range(4)))
                        self.assertTrue(all(reply == replies[0] for reply in replies))
                        self.assertEqual(ledger(), f'{floor}|{floor}')
                        self.assertEqual(self.profile(owner)[0].get('point', 0), floor * 10)
                        baseline_props = [copy.deepcopy(hero.prop) for hero in start.battlePlayer.formation]
                    section = http('CSRushGetSectionReward', {}, 'SCRushGetSectionReward')
                    self.assertEqual([(item.id, item.num) for item in section.gain.simps], [('item_001', star_choice)])
                    energy = self.profile(owner)[0]['energy']
                    http('CSItemUse', {'costItem': {'id': 'item_001', 'num': 1}}, 'SCItemUse')
                    self.assertEqual(self.profile(owner)[0]['energy'], energy + 10)
                    option = http('CSRushGetSectionAddition', {}, 'SCRushGetSectionAddition')
                    selected = (option.starResult3, option.starResult6, option.starResult9)[star_choice - 1]
                    self.assertEqual(option.costStar, star_choice * 3)
                    self.assertEqual(selected, ('peratt,0.03', 'perhp,0.06', 'perpdef,0.09')[star_choice - 1])
                    stamp += 1
                    addition_stamp = stamp
                    fields = {'select': selected, 'star': star_choice * 3}
                    addition = self.call('CSRushAddition', fields, owner, addition_stamp, 'SCRushAddition')
                    for floor in range(4, last_floor + 1):
                        successor, end, start_fields, start_stamp = actual_start(floor)
                        key = ('att', 'hp', 'pdef')[star_choice - 1]
                        for original, hero in zip(baseline_props, successor.battlePlayer.formation):
                            self.assertEqual(getattr(hero.prop, key), math.floor(getattr(original, key) * (1 + 0.03 * star_choice)))
                            self.assertEqual(hero.inst.curHp, hero.prop.hp)
                        if floor == last_floor:
                            # Admission closes only after the last paid input is
                            # frozen; its exact pending source must still finish.
                            self.sql('UPDATE p7_rush_manual_policies SET enabled=0 WHERE id=1; UPDATE p7_rush_continuation_policies SET enabled=0;')
                            self.stop()
                            self.start()
                            owner = self.login(identity)
                            restored = self.call('CSRushFightStart', start_fields, owner, start_stamp, 'SCRushFightStart')
                            self.assertEqual(restored, successor)
                        before = self.profile(owner)
                        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                            replies = list(pool.map(lambda _: self.call('CSRushFightEnd', end, owner, stamp + 1, 'SCRushFightEnd'), range(4)))
                        self.assertTrue(all(reply == replies[0] for reply in replies))
                        accepted = replies[0]
                        self.assertEqual(accepted.star, star_choice)
                        self.assertEqual(end['result'], 1, 'Finite actual source must win to continue reward use')
                        self.assertEqual(self.profile(owner)[0].get('point', 0), before[0].get('point', 0) + 10)
                        self.assertEqual(ledger(), f'{floor}|{floor}')
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p6_battle_records WHERE player_id={owner.playerId} AND authoritative=1;').strip(), str(last_floor))
                    activity = self.sql('SELECT id FROM p1_period_instances WHERE enabled=1 AND kind="rush_star_total" LIMIT 1;').strip()
                    self.assertTrue(activity)
                    view = http('CSActivityInfo', {'activityId': activity}, 'SCActivityInfo')
                    self.assertEqual(view.itemList[0].status, 1)
                    http('CSActivityReward', {'activityId': activity, 'index': 1, 'getResources': list(view.itemList[0].rewardList)}, 'SCActivityReward')
                    buy = http('CSShopBuy', {'shopType': 5, 'shopItemId': 'p4Point_1', 'buyCount': 1}, 'SCShopBuy')
                    self.assertEqual([(cost.id, cost.num) for cost in buy.cost], [('point', 100)])
                    final = self.profile(owner)
                    self.assertEqual(tuple(final[0].get(key, 0) for key in ('coin', 'diamond', 'point')), (0, 0, last_floor * 10))
                    self.assertEqual(sum(x['num'] for x in final[1]['items'] if x['id'] == 'item_021'), 1)
                    self.stop()
                    self.start()
                    owner = self.login(identity)
                    self.assertEqual(http('CSRushFightEnd', end, 'SCRushFightEnd'), accepted)
                    self.assertEqual(self.call('CSRushAddition', fields, owner, addition_stamp, 'SCRushAddition'), addition)
                    self.assertEqual(self.profile(owner), final)
                    if last_floor == 6:
                        http('CSRushResetChallengeCount', {}, 'SCRushResetChallengeCount')
                        reset_state = self.sql('SELECT CONCAT(run_id,"|",floor,"|",total_stars,"|",record_stars) '
                                               f'FROM p7_rush_states WHERE player_id={owner.playerId};').strip().split('|')
                        self.assertEqual(reset_state[:3], ['2', '1', '0'])
                        self.assertGreaterEqual(int(reset_state[3]), 6 * star_choice)
                        empty = http('CSRushAdditionView', {}, 'SCRushAdditionView')
                        self.assertEqual((empty.prop.peratt, empty.prop.perhp, empty.prop.perpdef), (0, 0, 0))
                        http('CSRushGetSectionAddition', {}, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), final)
                        report['sixFloorResetIsolatesPriorPerkAndStars'] = True
                    report.update(status='PASS_ORIGINAL_HTTP_INNODB_PARTY_STARS_ADDITION_NEXT_FLOOR_POINT_SHOP_COLD_ONCE',
                                  actualPartyStarts=last_floor, concurrentEndOnce=True, sameLedgerColdOnce=True,
                                  forgedResultsAndTransformsRejected=True, additionConsumedOnActualSuccessor=True,
                                  earnedActivityPointShopMaterial=True)
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
                    report.update(wallSeconds=round(time.monotonic()-began, 3), cleanupErrors=cleanup_errors,
                                  remainingOwnedAllocations=len(self.managed_pool.connections))
                    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
                    if cleanup_errors:
                        self.fail('Owned resource cleanup requires inspection: ' + ','.join(cleanup_errors))


if __name__ == '__main__':
    unittest.main()
