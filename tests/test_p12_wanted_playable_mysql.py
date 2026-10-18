"""Opt-in prebuilt Wanted HTTP/InnoDB chain; root owns resource scheduling.

Fresh isolated accounts receive only the explicit historical equipped001003
roster prerequisite. Wallets, items, passed copies, discoveries, fights and
receipts are never imported. First-copy winners/HP/operations must arrive from
the original automatic native caller for the exact newly exported real Start.
This runner never compiles Go or controls a device. No probability or clock edit.
"""
import base64
import concurrent.futures
import copy
import fcntl
import hashlib
import json
import math
import os
from pathlib import Path
import re
import secrets
import selectors
import subprocess
import sys
import tempfile
import time
import types
import unittest

from google.protobuf.json_format import MessageToDict
from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME, encode_http_request, message, parse_http
from test_p7_rush_party_mysql import historical_roster, ROSTER_SHA
from test_p7_maze_trader_mysql import P7MazeTraderMySQLTests as Transport
from test_p12_worldboss_lifecycle_mysql import WorldBossLifecycleMySQLTests as PaymentTransport

ROOT = Path(__file__).resolve().parents[1]


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def contract_preflight():
    requests = {
        'CSGetRebels': {}, 'CSShareRebel': {'rebelId': 'server-instance'},
        'CSRebelFightStart': {'rebelId': 'server-instance', 'type': 1},
        'CSRebelFightEnd': {'fightId': 'server-fight', 'result': 0, 'optRecord': {}},
        'CSGetRebelRewardInfo': {}, 'CSGetRebelReward': {'rebelRewardId': 1},
        'CSCopyEnter': {'chapterId': '1', 'missionId': '1'},
        'CSCopyFightStart': {'chapterId': '1', 'missionId': '1', 'taskId': '1'},
        'CSCopyFightEnd': {'fightId': 'actual', 'result': 0, 'optRecord': {}},
        'CSCopySweep': {'chapterId': '1', 'missionId': '1'},
        'CSPlayerLogin': {},
        'CSShopProductList': {'shopType': 7},
        'CSShopBuy': {'shopType': 7, 'shopItemId': 'p4Score_1', 'buyCount': 1},
        'CSFormationOn': {'formatPos': 1, 'id': 'pet_002', 'guid': 'owned-pet'},
        'CSGetPaymentInfo': {},
        'CSGenerateOrder': {'paymentId': 'djsy.wuyou.andp0050', 'money': 50,
                            'moneyType': 'RMB', 'channelId': 'AND0003', 'shopId': 'common'},
        'CSCheckOrder': {'orderId': 'server-order', 'paymentId': 'djsy.wuyou.andp0050'},
    }
    for name, fields in requests.items():
        value = message(name, **fields)
        frame = encode_http_request(BY_NAME[name], value, player_id=77, zone_id=1,
                                    session=b'0'*16, timestamp=1792140000000)
        parsed = parse_http(frame)
        if value != parsed.decode() or parsed.opcode != BY_NAME[name]:
            raise AssertionError('Recovered original request contract changed: ' + name)
    expected = {
        'SCRebelFightStart': {'fightId', 'randSeed', 'battlePlayer', 'battleMonsterList', 'fightRoomId', 'damageFactor', 'maxround'},
        'SCRebelFightEnd': {'get', 'cost', 'damage', 'result', 'rankBefore', 'rankNow', 'recordAdd', 'scoreAdd'},
        'SCGetRebels': {'rebels', 'normalWantedAttTimes', 'rageWantedAttTimes', 'exploit', 'exploitNum', 'maxHurt', 'maxHurtNum'},
    }
    for name, fields in expected.items():
        if not fields <= set(message(name).DESCRIPTOR.fields_by_name):
            raise AssertionError('Recovered response contract changed: ' + name)
    historical_roster()
    return {'status': 'STATIC_CONTRACT_PREFLIGHT_PASS', 'originalRequests': len(requests),
            'rosterSHA256': ROSTER_SHA, 'databaseAccess': False, 'deviceAccess': False,
            'sourceWinnerOrProgressSupplied': False}


def source_preflight():
    """Read one qualified immutable artifact; no service or SQL allocation."""
    source = Path(os.environ['HYJS_P12_WANTED_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_P12_WANTED_BINARY']).resolve()
    validation = Path(os.environ['HYJS_P12_WANTED_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates' or not binary.is_relative_to(ROOT / 'build'):
        raise AssertionError('Use a fixed local candidate and a root-built binary')
    proof = json.loads(validation.read_bytes())
    if (not proof.get('qualified') or (ROOT / proof['candidate']).resolve() != source or
            proof.get('binarySHA256') != digest(binary) or
            digest(binary) != os.environ['HYJS_P12_WANTED_BINARY_SHA256']):
        raise AssertionError('Qualification/candidate/executable SHA binding differs')
    manifest = json.loads((source / 'manifest.json').read_bytes())
    pins = {row['path']: row['sha256'] for row in manifest['files']}
    for name, expected in pins.items():
        if digest(source / name) != expected:
            raise AssertionError('Fixed candidate source pin changed: ' + name)
    if (ROOT / 'server/protocol/schema.json').read_bytes() != (source / 'server/protocol/schema.json').read_bytes():
        raise AssertionError('Live runner codec differs from fixed candidate')
    for name, token in [('main.go', 'InitializeP12Wanted'),
                        ('service/gameService.go', 'DispatchP12Wanted'),
                        ('service/copyService.go', 'DiscoverP12Wanted'),
                        ('service/copySweepService.go', 'DiscoverP12Wanted'),
                        ('service/combatOwnershipService.go', 'P12WantedRequireIdle')]:
        if token not in (source / 'hyjs-server' / name).read_text():
            raise AssertionError('Required integrated entry absent: ' + name)
    scope = (source / 'hyjs-server/constUtil/p12WantedScope.go').read_text()
    certificates = {}
    for key in ('P12WantedNativeProof', 'P12WantedPartialHPProof'):
        found = re.search(key + r'\s*=\s*"([0-9a-f]{64})"', scope)
        if found is None:
            raise AssertionError('Independent full/partial proof must be nonempty: ' + key)
        paths = [name for name, sha in pins.items() if sha == found[1]
                 and name.startswith('analysis/p12/wanted-playable-20261016/') and name.endswith('.json')]
        if len(paths) != 1:
            raise AssertionError('Exact Wanted certificate must be pinned in the artifact: ' + key)
        certificate = json.loads((source / paths[0]).read_bytes())
        if (certificate['nativeScope'] != 'original-wanted-automatic-ai' or
                certificate['caseCount'] != 4 or not certificate['allThirteenLayersPassed'] or
                certificate['scopeImplementationSHA256'] != digest(source / certificate['scopeImplementation'])):
            raise AssertionError('Certificate does not bind the exact independent Wanted implementation')
        certificates[key] = found[1]
    return source, binary, {'status': 'FIXED_SOURCE_PREFLIGHT_PASS', 'candidate': str(source),
        'binarySHA256': digest(binary), 'manifestSHA256': digest(source / 'manifest.json'),
        'validationSHA256': digest(validation), 'pinnedFiles': len(pins), 'certificates': certificates,
        'runnerSHA256': digest(Path(__file__)), 'databaseAccess': False, 'deviceAccess': False}


def owned_roster(source, main_guid):
    """Retain source stats/ownership layout while allocating independent identities."""
    result = copy.deepcopy(source)
    mapping = {result['playerPatch']['heroWarId']: main_guid}
    for row in [*result['gamePatch']['warriors'], *result['gamePatch']['equipment']]:
        mapping.setdefault(row['guid'], secrets.token_hex(16))
    mapping[result['pet']['guid']] = secrets.token_hex(16)

    def rebound(value):
        if isinstance(value, dict):
            return {key: rebound(item) for key, item in value.items()}
        if isinstance(value, list):
            return [rebound(item) for item in value]
        return mapping.get(value, value) if isinstance(value, str) else value

    result['playerPatch'] = rebound(result['playerPatch'])
    result['gamePatch'] = rebound(result['gamePatch'])
    pet = message('Pet'); pet.ParseFromString(bytes.fromhex(result['pet']['wireHex']))
    pet.guid = mapping[pet.guid]
    result['pet'].update(guid=pet.guid, wireHex=pet.SerializeToString(deterministic=True).hex())
    if len(set(mapping.values())) != len(mapping):
        raise AssertionError('Account-owned actor/equipment/Pet identities must be distinct')
    return result


@unittest.skipUnless(os.environ.get('HYJS_P12_WANTED_MYSQL_TEST') == '1',
                     'Explicit root managed SQL lease and fixed integrated candidate required')
class WantedPlayableMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql
    fund_test_orders = PaymentTransport.fund_test_orders

    def next_stamp(self):
        self.stamp += 1
        return self.stamp

    def http(self, name, fields, account, expected):
        return self.call(name, fields, account, self.next_stamp(), expected)

    def start(self):
        self.process = subprocess.Popen([str(self.binary), '--http-port', '0', '--tcp-port', '0',
            '--tables', str(self.source / 'analysis/tables'), '--log-file', str(self.native_log),
            '--enable-test-payments'], cwd=self.source / 'hyjs-server', env=self.env,
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), 'Owned Wanted app startup timed out')
            line = self.process.stdout.readline()
        match = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(match, line)
        self.service = types.SimpleNamespace(http_port=int(match[1]), tcp_port=int(match[2]))

    def prepare_party(self, account, roster):
        before = self.profile(account)
        self.assertEqual(tuple(before[0].get(key, 0) for key in ('coin', 'diamond', 'score')), (0, 0, 0))
        self.assertFalse(before[1].get('run'))
        owned = owned_roster(roster, before[0]['heroWarId'])
        player, game = copy.deepcopy(before)
        player.update(owned['playerPatch'])
        game.update(owned['gamePatch'])
        encode = lambda value: json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode().hex()
        # Only declared historical growth/formation prerequisites are written.
        # The compared expected profile retains the fresh wallet/items/progress.
        self.sql('UPDATE profiles SET player=CONVERT(0x' + encode(player) + ' USING utf8mb4),'
                 'game=CONVERT(0x' + encode(game) + ' USING utf8mb4) WHERE id=' + str(account.playerId) + ';')
        self.assertEqual(self.profile(account), [player, game])
        pet = owned['pet']
        self.sql('INSERT INTO p9_pet_instances(guid,player_id,pet_id,data,created_at,source) VALUES('
                 f'"{pet["guid"]}",{account.playerId},"pet_002",0x{pet["wireHex"]},'
                 f'{int(time.time()*1000)},"finite historical actual equipped source {ROSTER_SHA}; no natural acquisition claim");')
        self.http('CSFormationOn', {'id': 'pet_002', 'guid': pet['guid'], 'formatPos': 1}, account, 'SCFormationOn')
        guarded = self.profile(account)
        self.assertEqual(guarded[0].get('score', 0), 0)
        self.assertEqual(guarded[1].get('items'), before[1].get('items'))
        projection = next(row for row in guarded[1]['kv'] if row['key'] == 'p9.pet:' + pet['guid'])
        self.assertEqual(base64.b64decode(projection['value'], validate=True), bytes.fromhex(pet['wireHex']))
        self.report['finiteAccounts'].append({'player': account.playerId, 'petGUID': pet['guid'],
            'heroGUIDs': [row['guid'] for row in owned['gamePatch']['warriors']],
            'level': player['level'], 'ordinaryWalletOrInventoryImported': False,
            'progressOrReceiptImported': False, 'originalGuardPublished': True})

    def original_copy_end(self, start, index):
        """Export a genuine paid Start and wait for a separately root-owned original capture."""
        self.assertEqual(len(start.battleMonsterList), 1, 'Do not flatten an original multi-wave copy')
        # Type1 preparation and collection are intentionally confined to p6;
        # p12 source staging only admits the independent typed5/8 families.
        folder = self.copy_native / 'native-inputs' / ('%03d' % index)
        folder.mkdir(parents=True)
        wire = message('SCFightPlayback', battleType=1, fightid=start.fightId,
                       randSeed=start.randSeed, attacker=start.battlePlayer,
                       defender2=start.battleMonsterList[0], maxRound=start.maxRound,
                       attackerFactor=1, defenderFactor=1)
        (folder / '000.pb').write_bytes(wire.SerializeToString(deterministic=True))
        source = {'actualBusinessStart': True, 'authoritative': False,
            'nativeCompared': False, 'sourceKind': 'actual-isolated-paid-copy-wanted-discovery',
            'cases': [{'wire': '000.pb', 'wireSHA256': digest(folder / '000.pb'),
                       'fightId': start.fightId, 'seed': start.randSeed, 'battleType': 1,
                       'nativeScope': 'original-pve-automatic-ai', 'status': 'awaiting-original-automatic-ai'}]}
        manifest = folder / 'manifest.json'
        manifest.write_text(json.dumps(source, indent=2) + '\n')
        result_folder = self.copy_native / 'native-results' / ('%03d' % index)
        capture_path = result_folder / 'manifest.json'
        self.report['awaitingOriginalCopyCapture'] = {'source': str(manifest),
            'sourceSHA256': digest(manifest), 'collectOutput': str(result_folder), 'selector': 'pve-auto'}
        self.write_report()
        deadline = time.monotonic() + float(os.environ.get('HYJS_P12_WANTED_NATIVE_WAIT_SECONDS', '600'))
        capture = None
        while capture is None:
            if capture_path.exists():
                try:
                    capture = json.loads(capture_path.read_bytes())
                except (json.JSONDecodeError, OSError):
                    pass
            if capture is not None:
                break
            if time.monotonic() >= deadline:
                self.incomplete('Exact newly exported first-copy original automatic result was not supplied; no winner invented')
            time.sleep(.25)
        self.assertEqual(capture['selector'], 'pve-auto')
        self.assertEqual(Path(capture['sourceManifest']).resolve(), manifest.resolve())
        self.assertEqual(capture['sourceManifestSHA256'], digest(manifest))
        self.assertEqual(len(capture['cases']), 1)
        case = capture['cases'][0]
        self.assertEqual(case['wireSHA256'], source['cases'][0]['wireSHA256'])
        self.assertEqual(case['nativeScope'], 'original-pve-automatic-ai')
        path = result_folder / case['nativeResult']
        self.assertEqual(path.parent, result_folder)
        self.assertEqual(digest(path), case['nativeSHA256'])
        first = last = state = None
        final = []
        with path.open() as stream:
            for line in stream:
                event = json.loads(line)
                first = first or event
                last = event
                if event['kind'] == 'state':
                    state = event['state']
                if event['kind'] == 'original_final_operates':
                    final.append(event)
        self.assertEqual((first['kind'], first['fightId'], first['seed'], first['battleType']),
                         ('input', start.fightId, start.randSeed, 1))
        self.assertEqual(first['scope'], 'original-pve-automatic-ai')
        self.assertEqual(last['kind'], 'result')
        self.assertIs(type(last['attackerWon']), bool)
        self.assertEqual(len(final), 1)
        record_info = final[0]['data']
        raw = base64.b64decode(record_info['protobufBase64'], validate=True)
        record = message('BattleOperate'); record.ParseFromString(raw)
        self.assertTrue(record.IsInitialized())
        self.assertIsNotNone(state)
        self.assertEqual((state['round'], final[0]['round']), (record.finalRoundNum, record.finalRoundNum))
        actors = [{'camp': camp, 'uid': actor['uid'], 'heroId': actor['heroId'],
                   'currentHp': actor['attribute']['hp'], 'maxHp': actor['attribute']['maxHp']}
                  for camp, side in enumerate(('attacker', 'defender')) for actor in state[side]['actors']]
        party = message('BattlePlayer'); party.CopyFrom(start.battlePlayer)
        by_uid = {actor['uid']: actor for actor in actors}
        self.assertEqual(len(by_uid), len(party.formation) + len(start.battleMonsterList[0].formation))
        for camp, formation in enumerate((party.formation, start.battleMonsterList[0].formation)):
            for actor in formation:
                actual = by_uid[str(camp) + '_' + actor.guid]
                self.assertEqual((actual['camp'], actual['heroId'], actual['maxHp']), (camp, actor.id, actor.prop.hp))
                self.assertTrue(math.isfinite(actual['currentHp']) and 0 <= actual['currentHp'] <= actor.prop.hp)
                if camp == 0:
                    actor.inst.curHp = actual['currentHp']
        self.assertEqual(sum(actor.inst.curHp for actor in party.formation), record.attackerTotalLeftHp)
        self.assertEqual(sum(actor['currentHp'] for actor in actors if actor['camp'] == 1), record.defenderTotalLeftHp)
        self.report['originalCopyResults'].append({'fightId': start.fightId,
            'attackerWon': last['attackerWon'], 'captureManifestSHA256': digest(capture_path),
            'nativeSHA256': case['nativeSHA256'], 'originalOperateSHA256': hashlib.sha256(raw).hexdigest()})
        self.report.pop('awaitingOriginalCopyCapture', None)
        return {'fightId': start.fightId, 'result': int(last['attackerWon']),
                'battlePlayer': party, 'optRecord': record}

    def write_report(self):
        self.output.write_text(json.dumps(self.report, ensure_ascii=False, indent=2) + '\n')

    def incomplete(self, reason):
        self.report.update(status='INCOMPLETE_ACTUAL_SOURCE_OR_BUDGET', fullChain=False, reason=reason)
        self.write_report()
        self.skipTest(reason)

    def discover(self, owner):
        entry = self.http('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, owner, 'SCCopyEnter')
        for index, job in enumerate(entry.jobList):
            start = self.http('CSCopyFightStart', {'chapterId': '1', 'missionId': '1', 'taskId': job.taskId}, owner, 'SCCopyFightStart')
            finish = self.original_copy_end(start, index)
            ended = self.http('CSCopyFightEnd', finish, owner, 'SCCopyFightEnd')
            if not finish['result']:
                self.incomplete('The genuine original first-copy result was defeat; preserve actual paid result')
        self.assertEqual(ended.isCopyComplete, 1)
        state = self.profile(owner)
        passed = next(row for row in state[1]['copy']['passedCopyInfo'] if row['chapterId'] == '1' and row['missionId'] == '1')
        self.assertEqual(passed['star'], 5)
        for index in range(40):
            rebels = self.http('CSGetRebels', {}, owner, 'SCGetRebels').rebels
            if rebels:
                self.assertEqual(len(rebels), 1)
                self.report['naturalDiscovery'] = {'instance': rebels[0].id, 'sweepsActuallyConsumed': index,
                    'probabilityEdited': False, 'source': 'real complete then legal one-at-a-time sweeps'}
                return rebels[0]
            if index == 39:
                break
            player = self.profile(owner)[0]
            if player.get('stamina', 0) < 10:
                self.fund_test_orders(owner, 500)
                before = self.profile(owner)[0]
                bought = self.http('CSShopBuy', {'shopType': 1, 'shopItemId': 'itemShopId_2', 'buyCount': 1}, owner, 'SCShopBuy')
                self.assertEqual([(row.id, row.num) for row in bought.get.simps], [('stamina', 25)])
                self.assertGreater(self.profile(owner)[0]['stamina'], before.get('stamina', 0))
                self.report['actualStaminaPurchases'].append(MessageToDict(bought, preserving_proto_field_name=True))
            before = self.profile(owner)
            self.http('CSCopySweep', {'chapterId': '1', 'missionId': '1'}, owner, 'SCCopySweep')
            after = self.profile(owner)
            self.assertEqual(before[0]['stamina'] - after[0]['stamina'], 10)
        self.incomplete('No original-odds discovery in 39 legally paid sweeps; no instance/probability injected')

    def fight(self, account, instance, attack_type):
        fields = {'rebelId': instance, 'type': attack_type}
        stamp = self.next_stamp()
        start = self.call('CSRebelFightStart', fields, account, stamp, 'SCRebelFightStart')
        self.assertEqual(self.call('CSRebelFightStart', fields, account, stamp, 'SCRebelFightStart'), start)
        raw = self.sql('SELECT HEX(terminal_json) FROM p12_wanted_fights WHERE id="' + start.fightId + '";').strip()
        terminal = json.loads(bytes.fromhex(raw))
        self.assertTrue(terminal['outcome']['authoritative'])
        result = int(terminal['outcome']['winner'] == 0)
        finish = {'fightId': start.fightId, 'result': result, 'optRecord': terminal['originalOperates']}
        before = self.profile(account)
        self.http('CSRebelFightEnd', dict(finish, result=1-result), account, 'ErrorMessage')
        self.assertEqual(self.profile(account), before)
        end_stamp = self.next_stamp()
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            replies = list(pool.map(lambda _: self.call('CSRebelFightEnd', finish, account, end_stamp, 'SCRebelFightEnd'), range(4)))
        self.assertTrue(all(reply == replies[0] for reply in replies))
        ended = replies[0]
        self.assertEqual(ended.result, result)
        self.assertGreaterEqual(ended.damage, 0)
        self.report['actualWantedFights'].append({'player': account.playerId, 'fightId': start.fightId,
            'seed': start.randSeed, 'attackerWon': bool(result), 'damage': ended.damage,
            'recordAdd': ended.recordAdd, 'scoreAdd': ended.scoreAdd, 'concurrentOnce': 4})
        return ended, (finish, end_stamp)

    def test_natural_discovery_real_share_damage_reward_score_shop_and_cold(self):
        contract_preflight()
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source = Path(os.environ['HYJS_P12_WANTED_SOURCE']).resolve()
        self.binary = Path(os.environ['HYJS_P12_WANTED_BINARY']).resolve()
        self.assertEqual(self.source.parent, ROOT / 'build/candidates')
        self.assertEqual(digest(self.binary), os.environ['HYJS_P12_WANTED_BINARY_SHA256'])
        qualification = json.loads(Path(os.environ['HYJS_P12_WANTED_VALIDATION']).read_bytes())
        self.assertTrue(qualification.get('qualified'))
        self.assertEqual((ROOT / 'server/protocol/schema.json').read_bytes(), (self.source / 'server/protocol/schema.json').read_bytes())
        for name, token in [('gameService.go', 'DispatchP12Wanted'), ('copyService.go', 'DiscoverP12Wanted'), ('copySweepService.go', 'DiscoverP12Wanted')]:
            self.assertIn(token, (self.source / 'hyjs-server/service' / name).read_text(), 'Integrated entry required before allocating SQL')
        scope = (self.source / 'hyjs-server/constUtil/p12WantedScope.go').read_text()
        self.assertRegex(scope, r'P12WantedNativeProof\s*=\s*"[0-9a-f]{64}"')
        self.assertRegex(scope, r'P12WantedPartialHPProof\s*=\s*"[0-9a-f]{64}"', 'FullHP proof cannot authorize follow-up battles')
        self.output = Path(os.environ['HYJS_P12_WANTED_REPORT']).resolve()
        self.assertTrue(self.output.is_relative_to(ROOT / 'analysis/p12'))
        self.assertFalse(self.output.exists())
        self.output.parent.mkdir(parents=True, exist_ok=True)
        self.evidence = self.output.with_name(self.output.stem + '-wire')
        self.assertFalse(self.evidence.exists())
        self.evidence.mkdir()
        self.stamp = int(time.time()*1000)
        self.report = {'status': 'PENDING', 'fullChain': False, 'binarySHA256': digest(self.binary),
            'source': str(self.source), 'finiteRosterSHA256': ROSTER_SHA, 'finiteAccounts': [],
            'testProviderOrders': [], 'actualStaminaPurchases': [], 'originalCopyResults': [],
            'actualWantedFights': [], 'formalAccountsModified': False, 'phoneBusinessUI': False,
            'dailyRankMailboxRequiresSeparateRealNextDayAcceptance': True}
        self.managed_pool = ManagedMySQL()
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-wanted-http-') as temporary:
            self.native_log = Path(temporary) / 'app.log'
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    allocation = self.managed_pool.allocate('p12_wanted_http')
                    self.database, self.test_user, self.test_password = allocation.database, allocation.user, allocation.password
                    self.env = dict(os.environ, **allocation.env(), GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    tables = ['profiles', 'p12_wanted_policies', 'p12_wanted_instances', 'p12_wanted_discoveries', 'p12_wanted_fights', 'p12_wanted_reward_claims', 'p9_pet_instances']
                    engines = self.sql('SELECT CONCAT(TABLE_NAME,"|",ENGINE) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN (' + ','.join('"'+name+'"' for name in tables) + ');').splitlines()
                    self.assertEqual({row.split('|')[0] for row in engines}, set(tables))
                    self.assertTrue(all(row.endswith('|InnoDB') for row in engines))
                    self.assertEqual(self.sql('SELECT enabled FROM p12_wanted_policies WHERE id=1;').strip(), '0')
                    self.sql('UPDATE p12_wanted_policies SET enabled=1 WHERE id=1;')
                    identities = ['wanted-natural-' + secrets.token_hex(8) for _ in range(2)]
                    owner, peer = [self.login(identity) for identity in identities]
                    roster = historical_roster()
                    for index, account in enumerate((owner, peer)):
                        self.http('CSPlayerCreate', {'playerName': '悬赏真链' + str(index), 'heroId': 'hero_001'}, account, 'SCPlayerCreate')
                        self.prepare_party(account, roster)
                    found = self.discover(owner)
                    self.assertEqual(list(self.http('CSGetRebels', {}, peer, 'SCGetRebels').rebels), [])
                    self.http('CSShareRebel', {'rebelId': found.id}, owner, 'ErrorMessage')
                    positive = False
                    for _ in range(5):
                        ended, receipt = self.fight(owner, found.id, 1)
                        if ended.damage > 0:
                            positive = True
                            break
                    if not positive:
                        self.incomplete('Five actual original encounters yielded no shareable positive damage; zero samples preserved')
                    live = float(self.sql('SELECT current_hp FROM p12_wanted_instances WHERE id="' + found.id + '";').strip())
                    if live == 0:
                        self.incomplete('Owner actually killed private first instance before sharing; no HP reset')
                    self.http('CSShareRebel', {'rebelId': found.id}, owner, 'SCShareRebel')
                    self.assertIn(found.id, [row.id for row in self.http('CSGetRebels', {}, peer, 'SCGetRebels').rebels])
                    ended, peer_receipt = self.fight(peer, found.id, 2)
                    for account in (owner, peer):
                        board = self.http('CSGetRebels', {}, account, 'SCGetRebels')
                        self.assertGreaterEqual(board.exploit, 0)
                        info = self.http('CSGetRebelRewardInfo', {}, account, 'SCGetRebelRewardInfo')
                        for reward in info.rebelRewardInfos:
                            if reward.state == 1:
                                self.http('CSGetRebelReward', {'rebelRewardId': reward.rewardId}, account, 'SCGetRebelReward')
                    if self.profile(owner)[0].get('score', 0) < 50:
                        self.incomplete('Actual score did not reach original score-shop price; no score injection')
                    listing = self.http('CSShopProductList', {'shopType': 7}, owner, 'SCShopProductList')
                    self.assertIn('p4Score_1', [row.id for row in listing.shopItemList])
                    args = {'shopType': 7, 'shopItemId': 'p4Score_1', 'buyCount': 1}
                    stamp = self.next_stamp()
                    before = self.profile(owner)
                    bought = self.call('CSShopBuy', args, owner, stamp, 'SCShopBuy')
                    self.assertEqual([(row.id, row.num) for row in bought.cost], [('score', 50)])
                    after = self.profile(owner)
                    self.assertEqual(before[0]['score'] - after[0]['score'], 50)
                    self.stop(); self.start()
                    owner, peer = [self.login(identity) for identity in identities]
                    self.assertEqual(self.call('CSShopBuy', args, owner, stamp, 'SCShopBuy'), bought)
                    self.assertEqual(self.profile(owner), after)
                    finish, end_stamp = peer_receipt
                    self.call('CSRebelFightEnd', finish, peer, end_stamp, 'SCRebelFightEnd')
                    self.http('CSGetRebels', {}, owner, 'SCGetRebels')
                    self.report.update(status='PASS_NATURAL_DISCOVERY_SHARE_COMBAT_DAILY_REWARD_SHOP_COLD_SUBCHAIN',
                        fullChain=False, acceptedSubchain=True, actualScoreSpend=50, coldRestart=True,
                        pending=['real next-day original rank mailbox reward', 'original business UI', 'all Wanted content'])
                except unittest.SkipTest:
                    raise
                except BaseException as error:
                    self.report.update(status='FAIL', error=self.managed_pool.redact(repr(error)))
                    raise
                finally:
                    self.stop(); self.managed_pool.close()
                    self.report.update(seconds=round(time.monotonic()-began, 3), resources={'process': 0, 'managedSchema': 0, 'managedAccount': 0})
                    self.write_report()


if __name__ == '__main__':
    if '--preflight-contracts' in sys.argv:
        print(json.dumps(contract_preflight(), indent=2))
    else:
        unittest.main()
