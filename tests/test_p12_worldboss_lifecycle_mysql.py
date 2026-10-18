"""One fixed executable, original P12 HTTP/TCP and one managed InnoDB allocation.

This file never compiles, installs an APK, controls a phone or creates database
infrastructure. Full mode requires a real original 12:00/21:40 UTC+8 window.
Contracts mode can run outside that window and explicitly does not prove a new
battle chain. All account-growth imports and boundary state preparations are
reported; neither natural progression nor original-client numerical parity is
claimed. Run with PYTHONPATH=.:tests and HYJS_P12_WORLDBOSS_MYSQL_TEST=1.
"""
import base64
import concurrent.futures
import copy
from datetime import datetime, timedelta, timezone
import fcntl
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import subprocess
import tempfile
import time
import types
import unittest

from server.codec import BY_NAME, CLASSES, parse_http
from hyjs_managed_mysql import ManagedMySQL
import test_continued_gameplay_mysql as transport
from test_p7_rush_party_mysql import historical_roster, ROSTER, ROSTER_SHA


ROOT = Path(__file__).resolve().parents[1]
SOURCE_ROOT = Path(os.environ.get('HYJS_P12_WORLDBOSS_SOURCE_ROOT',
                                 ROOT / 'build/candidates/worldboss-lifecycle-party-20261016')).resolve()
VALIDATION = Path(os.environ.get('HYJS_P12_WORLDBOSS_VALIDATION',
                                ROOT / 'analysis/p12/worldboss-lifecycle-party-validation-20261016.json')).resolve()
REPORT = Path(os.environ.get('HYJS_P12_WORLDBOSS_REPORT',
                            ROOT / 'analysis/p12/worldboss-lifecycle-party-protocol-20261016.json')).resolve()
PHASE = os.environ.get('HYJS_P12_WORLDBOSS_PHASE', 'full')


def owned_roster(source, player_id, pet_guid):
    """Rebind only the owned Pet identity; retain both original actor growth paths."""
    if type(player_id) is not int or player_id <= 1 or not re.fullmatch(r'[a-f0-9]{32}', pet_guid):
        raise AssertionError('Use a new isolated nonprotected account and unique Pet identity')
    result = copy.deepcopy(source)
    old_guid = result['pet']['guid']
    pet = CLASSES['Pet']()
    pet.ParseFromString(bytes.fromhex(result['pet']['wireHex']))
    pet.guid = pet_guid
    result['pet'].update(guid=pet_guid, wireHex=pet.SerializeToString(deterministic=True).hex())
    slots = [thing for formation in result['gamePatch']['formation'] for thing in formation['things']
             if thing.get('id') == 'pet_002']
    if len(slots) != 1 or slots[0]['guid'] != old_guid or slots[0]['pos'] != 7:
        raise AssertionError('Retain the single historical Pet002 guard on hero001')
    slots[0]['guid'] = pet_guid
    # Original P12 level90 entry is an explicit finite player qualification.
    # Hero001 remains75, Hero003 remains4; no combat growth number is rewritten.
    result['playerPatch']['level'] = max(90, result['playerPatch']['level'])
    result['ownerPlayerId'] = player_id
    return result


def active_original_window(now=None):
    """Return exact original start/end, without modifying a process or DB clock."""
    now = now or datetime.now(timezone(timedelta(hours=8)))
    for hour, minute in ((12, 0), (21, 40)):
        start = now.replace(hour=hour, minute=minute, second=0, microsecond=0)
        end = start + timedelta(hours=1)
        if start <= now < end:
            return int(start.timestamp() * 1000), int(end.timestamp() * 1000)
    return None


def preflight():
    if PHASE not in {'full', 'contracts'}:
        raise AssertionError('HYJS_P12_WORLDBOSS_PHASE must be full or contracts')
    if SOURCE_ROOT.parent != (ROOT / 'build/candidates').resolve() or not SOURCE_ROOT.is_dir():
        raise AssertionError('Select one direct fixed build/candidates source directory')
    binary = Path(os.environ.get('HYJS_P12_WORLDBOSS_BINARY', '')).expanduser()
    if not binary.is_file():
        raise AssertionError('HYJS_P12_WORLDBOSS_BINARY must name the already qualified executable')
    proof = json.loads(VALIDATION.read_text())
    if not proof.get('qualified'):
        raise AssertionError('Root qualification must pass before original protocol validation')
    transport.SOURCE_ROOT = SOURCE_ROOT
    transport.SOURCE = SOURCE_ROOT / 'manifest.json'
    transport.VALIDATION = VALIDATION
    transport.REPORT = REPORT
    previous = os.environ.get('HYJS_CONTINUED_GAMEPLAY_BINARY')
    os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = str(binary.resolve())
    try:
        built, binary_sha, source_sha = transport.binary_source_preflight()
    finally:
        if previous is None:
            os.environ.pop('HYJS_CONTINUED_GAMEPLAY_BINARY', None)
        else:
            os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = previous
    # The live Python codec must match the exact recovered protocol in the fixed source.
    schema = Path('server/protocol/schema.json')
    if (ROOT / schema).read_bytes() != (SOURCE_ROOT / schema).read_bytes():
        raise AssertionError('Runner recovered codec schema differs from fixed candidate')
    vectors = {
        'CSWorldBossInfo': {}, 'CSWorldBossRank': {}, 'CSWorldBossReward': {},
        'CSWorldBossPropPurchase': {'propId': 'wordboss_1'},
        'CSWorldBossFight': {'bossId': 'server-generated-uuid'},
        'CSPlayerLogin': {}, 'CSQueryUserReward': {},
        'CSAddRewardsToPlayer': {'rewardIds': [1]},
        'CSItemUse': {'costItem': {'id': 'item_150', 'num': 1}},
        'CSFormationOn': {'id': 'pet_002', 'guid': 'a' * 32, 'formatPos': 1},
        'CSGetPaymentInfo': {}, 'CSResurrection': {},
        'CSGenerateOrder': {'paymentId': 'djsy.wuyou.andp0050', 'money': 50,
                            'moneyType': 'RMB', 'channelId': 'AND0003', 'shopId': 'common'},
        'CSCheckOrder': {'orderId': 'source-preflight', 'paymentId': 'djsy.wuyou.andp0050'},
    }
    for name, fields in vectors.items():
        frame = transport.encode_original_http(name, fields, 1, b'0' * 16, 0)
        request = parse_http(frame)
        if request.opcode != BY_NAME[name] or request.decode() != CLASSES[name](**fields):
            raise AssertionError('Original request roundtrip changed: ' + name)
    for name, fields in {
        'SCWorldBossFight': ['fightId', 'battlePlayer', 'battleMonsterList', 'damange',
                             'damageforwardPercent', 'damagelaterPercent', 'playerSide'],
        'SCWorldBossRank': ['bossHpPercent', 'damageInfos', 'lastDamageInfo'],
        'SCWorldBossPropPurchase': ['propPurchased', 'cost'],
    }.items():
        if any(field not in CLASSES[name].DESCRIPTOR.fields_by_name for field in fields):
            raise AssertionError('Original response contract changed: ' + name)
    if (BY_NAME['CSWorldBossFight'], BY_NAME['SCWorldBossFight']) != (24607, 24608):
        raise AssertionError('Original world-boss opcode changed')
    for file, literals in {
        'hyjs-server/handler/p12WorldBossFinishHandler.go': ['/world-boss/finish', 'DisallowUnknownFields'],
        'hyjs-server/handler/p12WorldBossRecoveryHandler.go': ['/world-boss/resume', '/world-boss/abandon'],
        'hyjs-server/model/p12WorldBossRecovery.go': ['json:"pending"', 'json:"scene,omitempty"'],
        'hyjs-server/model/p12WorldBossBattle.go': ['json:"currentHp"', 'json:"totalHp"'],
    }.items():
        source = (SOURCE_ROOT / file).read_text()
        if any(literal not in source for literal in literals):
            raise AssertionError('Bridge JSON route/field contract changed: ' + file)
    roster = historical_roster()
    pet = CLASSES['Pet']()
    pet.ParseFromString(bytes.fromhex(roster['pet']['wireHex']))
    if pet.level != 2 or roster['gamePatch']['equipment'][0]['level'] != 7:
        raise AssertionError('Require the same actual037Lv7/Pet002Lv2 P7 source')
    # A qualified current candidate must carry its independent type18 scope;
    # the old solo047 or other-mode certificates cannot satisfy this batch.
    scope_path = SOURCE_ROOT / 'hyjs-server/service/p12WorldBossNativeScopeService.go'
    if not scope_path.is_file() or 'validateP12WorldBossPartyScope' not in scope_path.read_text():
        raise AssertionError('Fixed candidate lacks the independent P12 party scope gate')
    return built, roster, {'binarySHA256': binary_sha, 'sourceManifestSHA256': source_sha,
                          'originalRequestVectors': len(vectors),
                          'actualP5FixtureSHA256': ROSTER_SHA, 'rosterPath': str(ROSTER),
                          'rosterValidatorSHA256': hashlib.sha256((ROOT / 'tests/test_p7_rush_party_mysql.py').read_bytes()).hexdigest(),
                          'sourceRoot': str(SOURCE_ROOT), 'validation': str(VALIDATION)}


@unittest.skipUnless(os.environ.get('HYJS_P12_WORLDBOSS_MYSQL_TEST') == '1',
                     'Explicitly enable the isolated fixed-binary P12 lifecycle batch')
class WorldBossLifecycleMySQLTests(unittest.TestCase):
    send = transport.ContinuedGameplayMySQLTests.send
    login = transport.ContinuedGameplayMySQLTests.login
    stamped = transport.ContinuedGameplayMySQLTests.stamped
    call = transport.ContinuedGameplayMySQLTests.call
    profile = transport.ContinuedGameplayMySQLTests.profile
    write_profile = transport.ContinuedGameplayMySQLTests.write_profile
    sql = transport.ContinuedGameplayMySQLTests.sql
    stop = transport.ContinuedGameplayMySQLTests.stop
    connect = transport.ContinuedGameplayMySQLTests.connect
    tcp_call = transport.ContinuedGameplayMySQLTests.tcp_call
    tcp_send = transport.ContinuedGameplayMySQLTests.tcp_send
    recv = transport.ContinuedGameplayMySQLTests.recv
    close_sockets = transport.ContinuedGameplayMySQLTests.close_sockets

    def start(self):
        # Only this owned isolated app opts in to the loopback test provider.
        args = [self.binary, '--http-port', '0', '--tcp-port', '0', '--tables',
                str(SOURCE_ROOT / 'analysis/tables'), '--log-file',
                str(Path(self.app_temp.name) / 'app.log'), '--enable-test-payments']
        self.process = subprocess.Popen(args, cwd=SOURCE_ROOT / 'hyjs-server', env=self.env,
                                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), 'Owned P12 app startup timed out')
            line = self.process.stdout.readline()
        ports = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(ports, 'Owned app must publish its loopback ports')
        self.service = types.SimpleNamespace(http_port=int(ports[1]), tcp_port=int(ports[2]))

    def next_stamp(self):
        self.lifecycle_stamp += 1
        return self.lifecycle_stamp

    def fund_test_orders(self, account, minimum):
        """Original SKU/order/query plus trusted local test-provider confirmation."""
        product_id = 'djsy.wuyou.andp0050'
        for _ in range(3):
            before = self.profile(account)[0]
            if before.get('diamond', 0) >= minimum:
                return
            products = self.call('CSGetPaymentInfo', {}, account, self.next_stamp(),
                                 'SCGetPaymentInfo').paymentInfoList
            product = next(row for row in products if row.paymentId == product_id)
            self.assertEqual((product.money, product.moneyType, product.shopId), (50, 'RMB', 'common'))
            order = self.call('CSGenerateOrder', {'paymentId': product_id, 'money': product.money,
                              'moneyType': product.moneyType, 'channelId': 'AND0003',
                              'shopId': product.shopId}, account, self.next_stamp(), 'SCGenerateOrder')
            query = {'orderId': order.orderId, 'paymentId': product_id}
            waiting = self.call('CSCheckOrder', query, account, self.next_stamp(), 'SCCheckOrder')
            self.assertEqual(waiting.transactionData.payStatus, 'PAY_STATUS_WAIT')
            self.assertEqual(self.profile(account)[0].get('diamond', 0), before.get('diamond', 0))
            body = {'orderId': order.orderId, 'eventId': 'p12-local-test-' + secrets.token_hex(12),
                    'amountCents': round(product.money * 100), 'outcome': 'confirmed'}
            connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=15)
            try:
                connection.request('POST', '/local-test/payments/confirm', json.dumps(body),
                                   {'Content-Type': 'application/json'})
                response = connection.getresponse()
                confirmation = json.loads(response.read())
                self.assertEqual(response.status, 200)
                self.assertEqual(confirmation.get('code'), 0)
                self.assertTrue(confirmation['data']['testOnly'])
            finally:
                connection.close()
            paid = self.call('CSCheckOrder', query, account, self.next_stamp(), 'SCCheckOrder')
            self.assertEqual(paid.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
            credited = json.loads(self.sql('SELECT JSON_OBJECT("credited",credited,"status",status,'
                                          '"player",player_id,"cents",cents) FROM p3_payment_orders '
                                          'WHERE id="' + order.orderId + '";'))
            self.assertEqual((credited['status'], credited['player'], credited['cents']),
                             ('confirmed', account.playerId, body['amountCents']))
            self.assertGreater(credited['credited'], 0)
            after = self.profile(account)[0]
            self.assertEqual(after.get('diamond', 0) - before.get('diamond', 0), credited['credited'])
            self.assertEqual(after.get('coin', 0), before.get('coin', 0))
            self.report['testProviderOrders'].append({'player': account.playerId, 'order': order.orderId,
                                                     'product': product_id, 'actualCredited': credited['credited'],
                                                     'amountCents': body['amountCents'], 'realMoneyCharged': False})
        if self.profile(account)[0].get('diamond', 0) >= minimum:
            return
        self.fail('Actual original SKU credit did not cover the declared spend budget')

    def ensure_scope_entry(self, account):
        """After a real defeat, pay for original full revival; never reset Side."""
        player, game = self.profile(account)
        side = game['field']['side']
        if side.get('curHp', 0) == 0 and side.get('state') == 5:
            self.fund_test_orders(account, 50)
            before = self.profile(account)[0].get('diamond', 0)
            revived = self.call('CSResurrection', {}, account, self.next_stamp(), 'SCResurrenttion')
            self.assertEqual([(cost.id, cost.num) for cost in revived.cost], [('diamond', 50)])
            self.assertEqual(self.profile(account)[0].get('diamond', 0), before - 50)
            self.report['actualPaidRevivals'].append({'player': account.playerId, 'diamond': 50})
            player, game = self.profile(account)
            side = game['field']['side']
        self.assertEqual((side.get('state', 0), side.get('curHp', 0), side.get('anger', 0)), (0, 1, 0),
                         'Original revival/recovery must establish full HP/AP0; no Side reset is allowed')
        self.assertTrue(all(row.get('hpPercent') == 1 for row in side.get('battlePosHps', [])))
        self.assertEqual((game['position']['scene'], game['field']['scene']), (self.boss_scene, self.boss_scene))
        self.assertFalse(game.get('position_loading'))

    def purchase_boost(self, account, prop):
        self.fund_test_orders(account, 150)
        before = self.profile(account)[0]['diamond']
        purchase = self.call('CSWorldBossPropPurchase', {'propId': prop}, account,
                             self.next_stamp(), 'SCWorldBossPropPurchase')
        self.assertEqual([(cost.id, cost.num) for cost in purchase.cost], [('diamond', 150)])
        self.assertEqual(self.profile(account)[0]['diamond'], before - 150)
        self.report['actualBoostPurchases'].append({'player': account.playerId, 'prop': prop,
                                                   'diamond': 150, 'purchased': list(purchase.propPurchased)})
        return purchase

    def bridge(self, path, account, body, success=True, session=None):
        session = bytes(account.sessionKey) if session is None else session
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=90)
        try:
            connection.request('POST', '/world-boss/' + path, json.dumps(body), headers={
                'Content-Type': 'application/json', 'X-Hyjs-Player-ID': str(account.playerId),
                'X-Hyjs-Session-Key': base64.b64encode(session).decode()})
            response = connection.getresponse()
            value = json.loads(response.read())
            self.assertEqual(response.status, 200)
            self.assertEqual(value.get('code') == 0, success, 'Local finish/resume contract rejected')
            return value.get('data')
        finally:
            connection.close()

    def receipt(self, fight):
        fields = ('fight_id', 'player_id', 'instance_id', 'predicted_damage', 'applied_damage',
                  'committed', 'abandoned_at', 'start_hash', 'finish_hash')
        pairs = ','.join('"' + key + '",' + key for key in fields)
        return json.loads(self.sql('SELECT JSON_OBJECT(' + pairs + ') FROM '
                                  'p12_world_boss_battle_receipts WHERE fight_id="' + fight + '";'))

    def instance(self, boss):
        return json.loads(self.sql('SELECT JSON_OBJECT("hp",current_hp,"total",total_hp,'
                                  '"killer",killer_id,"killedAt",killed_at) FROM '
                                  'p12_world_boss_battle_instances WHERE id="' + boss + '";'))

    def decoded_start(self, data):
        raw = base64.b64decode(data['startBase64'], validate=True)
        result = CLASSES['SCWorldBossFight']()
        result.ParseFromString(raw)
        return result

    def begin(self, account, boss):
        self.ensure_scope_entry(account)
        count = self.actual_starts.get(account.playerId, 0)
        self.assertLess(count, 5, 'Do not exceed the five actual free starts in this finite batch')
        start = self.call('CSWorldBossFight', {'bossId': boss}, account, 0, 'SCWorldBossFight')
        self.actual_starts[account.playerId] = count + 1
        self.assertIn(start.code, (1, 2))
        self.assertEqual(start.maxround, 40)
        self.assertEqual(len(start.battleMonsterList), 1)
        self.assertEqual(len(start.battleMonsterList[0].formation), 4)
        receipt = self.receipt(start.fightId)
        self.assertFalse(receipt['committed'])
        self.assertGreaterEqual(receipt['predicted_damage'], 0, 'Zero damage is a legitimate original terminal')
        self.assertEqual([hero.id for hero in start.battlePlayer.formation], ['hero_001', 'hero_003'])
        self.assertEqual([hero.pose for hero in start.battlePlayer.formation], [3, 7])
        self.assertEqual(start.battlePlayer.inst.anger, 0)
        for hero in start.battlePlayer.formation:
            self.assertEqual(hero.inst.curHp, hero.prop.hp)
        self.report['actualStarts'].append({'player': account.playerId, 'fightId': start.fightId,
                                           'seed': start.randSeed, 'code': start.code,
                                           'predictedDamage': receipt['predicted_damage'],
                                           'freeAttempt': count + 1})
        return start

    def prepare_growth(self, account, roster, scene, point):
        player, game = self.profile(account)
        self.assertEqual((player.get('coin', 0), player.get('diamond', 0)), (0, 0))
        self.assertFalse(game.get('run'))
        owned = owned_roster(roster, int(account.playerId), secrets.token_hex(16))
        expected_player, expected_game = copy.deepcopy((player, game))
        expected_player.update(owned['playerPatch'], noviceGuideSchedule=8)
        expected_game.update(copy.deepcopy(owned['gamePatch']))
        expected_game.update(position={'scene': scene, 'current': point}, position_loading=False,
                             field={'scene': scene, 'side': {'playerId': account.playerId, 'state': 0,
                                                            'curHp': 1, 'maxHp': 1, 'anger': 0}})
        # Exactly one initial finite roster/entry preparation. Later starts use
        # the original death/revival lifecycle, never another profile overwrite.
        self.write_profile(account, expected_player, expected_game)
        self.assertEqual(self.profile(account), [expected_player, expected_game])
        pet = owned['pet']
        self.sql('INSERT INTO p9_pet_instances(guid,player_id,pet_id,data,created_at,source) VALUES('
                 f'"{pet["guid"]}",{int(account.playerId)},"pet_002",0x{pet["wireHex"]},'
                 f'{int(time.time()*1000)},"finite historical roster SHA{ROSTER_SHA}; no acquisition claim");')
        self.call('CSFormationOn', {'id': 'pet_002', 'guid': pet['guid'], 'formatPos': 1},
                  account, self.next_stamp(), 'SCFormationOn')
        guarded_player, guarded_game = self.profile(account)
        for key in ('coin', 'diamond', 'point'):
            self.assertEqual(guarded_player.get(key, 0), player.get(key, 0))
        self.assertEqual(guarded_game.get('items'), game.get('items'))
        self.assertEqual(guarded_game.get('warriors'), expected_game['warriors'])
        self.assertEqual(guarded_game.get('equipment'), expected_game['equipment'])
        projections = [row for row in guarded_game['kv'] if row['key'].startswith('p9.pet:')]
        self.assertEqual(len(projections), 1)
        self.assertEqual(projections[0]['key'], 'p9.pet:' + pet['guid'])
        self.assertEqual(base64.b64decode(projections[0]['value'], validate=True), bytes.fromhex(pet['wireHex']))
        self.assertEqual(self.sql('SELECT player_id FROM p9_pet_instances WHERE guid="' + pet['guid'] + '";').strip(),
                         str(account.playerId))
        self.report['finitePartyAccounts'].append({'player': account.playerId, 'petGUID': pet['guid'],
                                                  'petOwnedBy': account.playerId,
                                                  'originalGuardPublishedSnapshot': True,
                                                  'walletOrItemsImported': False})

    def contracts(self, accounts):
        a, b = accounts[:2]
        for account in accounts:
            self.call('CSPlayerCreate', {'playerName': '首领链' + str(account.playerId), 'heroId': 'hero_001'},
                      account, 1, 'SCPlayerCreate')
        self.assertEqual(self.sql('SELECT CONCAT(enabled,"|",economy_enabled,"|",rewards_enabled) '
                                  'FROM p12_world_boss_battle_policies WHERE id=1;').strip(), '0|0|0')
        before = self.profile(a)
        self.assertEqual(self.stamped('CSWorldBossFight', {'bossId': 'unavailable'}, a, 0)[0], 10001)
        self.assertFalse(self.bridge('resume', a, {})['pending'])
        self.bridge('finish', a, {'fightId': 'unknown-server-fight'}, success=False)
        self.bridge('abandon', a, {'fightId': 'unknown-server-fight'}, success=False)
        for path, body in [('resume', {'winner': 0}),
                           ('finish', {'fightId': 'unknown', 'damage': 100}),
                           ('finish', {'fightId': 'unknown', 'winner': 1}),
                           ('abandon', {'fightId': 'unknown', 'result': 1})]:
            self.bridge(path, a, body, success=False)
        self.bridge('resume', a, {}, success=False, session=bytes(b.sessionKey))
        self.bridge('resume', a, {}, success=False, session=b'0' * 16)
        self.assertEqual(self.profile(a), before)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p12_world_boss_battle_receipts;').strip(), '0')
        return {'defaultClosed': True, 'authenticatedEmptyResume': True,
                'unknownResultsAndForeignSessionsRejected': True, 'noReceiptsOrResourceChanges': True}

    def full_chain(self, accounts, roster, report):
        a, b, c, d, e, f = accounts
        self.sql('UPDATE p12_world_boss_policies SET enabled=1,level=1,position_id=1 WHERE id=1; '
                 'UPDATE p12_world_boss_battle_policies SET enabled=1,economy_enabled=1 WHERE id=1;')
        info = self.call('CSWorldBossInfo', {}, a, 2, 'SCWorldBossInfo').worldBossShowInfo
        self.assertEqual(info.bossState, 1)
        boss, scene = info.bossuuid, info.scene
        self.boss_scene = scene
        cycle = json.loads(self.sql('SELECT JSON_OBJECT("start",starts_at,"end",ends_at,"state",state,'
                                   '"endedAt",ended_at,'
                                   '"hash",definition_hash,"definition",definition_json) '
                                   'FROM p12_world_boss_cycles WHERE id="' + boss + '";'))
        definition = json.loads(cycle['definition'])
        position = next(row for row in definition['Tables']['WorldBossPos'] if row['id'] == '1')
        self.assertEqual(position['scene'], scene)
        xyz = [float(value) for value in position['pos'].split(',')]
        point = dict(zip(('x', 'y', 'z'), xyz))
        for account in accounts:
            self.prepare_growth(account, roster, scene, point)
        self.assertEqual(len({row['petGUID'] for row in report['finitePartyAccounts']}), len(accounts))
        # Actual original test SKU credits must cover boosts and possible revival.
        # No exact bonus or first-charge multiplier is assumed by this runner.
        self.fund_test_orders(a, 700)
        paid_before = self.profile(a)[0]['diamond']
        for prop in ('wordboss_1', 'wordboss_3', 'wordboss_4'):
            self.purchase_boost(a, prop)
        duplicate = self.call('CSWorldBossPropPurchase', {'propId': 'wordboss_1'}, a, 11,
                              'SCWorldBossPropPurchase')
        self.assertEqual(len(duplicate.cost), 0)
        self.assertEqual(self.profile(a)[0]['diamond'], paid_before - 450)
        self.assertEqual(list(duplicate.propPurchased), ['wordboss_1', 'wordboss_3', 'wordboss_4'])
        hp0 = self.instance(boss)
        first = self.begin(a, boss)
        first_receipt = self.receipt(first.fightId)
        closure = json.loads(self.sql('SELECT JSON_OBJECT("inputHashValid",SHA2(input_json,256)=input_hash,'
                                     '"terminalHashValid",SHA2(terminal_json,256)=terminal_hash,'
                                     '"battleType",JSON_EXTRACT(input_json,"$.battleType"),'
                                     '"maxRound",JSON_EXTRACT(input_json,"$.maxRound"),'
                                     '"members",JSON_LENGTH(input_json,"$.monster.formation"),'
                                     '"paidBoosts",JSON_LENGTH(input_json,"$.appliedProperties.worldBossPurchasedBoosts")) '
                                     'FROM p12_world_boss_battle_receipts WHERE fight_id="' + first.fightId + '";'))
        self.assertEqual(closure, {'inputHashValid': 1, 'terminalHashValid': 1,
                                   'battleType': 18, 'maxRound': 40, 'members': 4, 'paidBoosts': 3})
        self.assertEqual(self.instance(boss), hp0)
        self.assertEqual([hero.prop.hit for hero in first.battlePlayer.formation], [175, 175])
        rank = self.call('CSWorldBossRank', {}, a, 12, 'SCWorldBossRank')
        self.assertEqual(len(rank.damageInfos), 0)
        self.assertEqual(rank.bossHpPercent, 1)
        sock = self.connect(a)
        replay = self.tcp_call(sock, a, 'CSWorldBossFight', {'bossId': boss}, 'SCWorldBossFight')
        self.assertEqual(replay, first, 'Original timestamp-free TCP retry changed the frozen battle')
        self.close_sockets()
        self.bridge('finish', b, {'fightId': first.fightId}, success=False)
        self.bridge('finish', a, {'fightId': first.fightId, 'damage': 2147483647}, success=False)
        self.bridge('finish', a, {'fightId': first.fightId, 'winner': 0}, success=False)
        self.assertFalse(self.receipt(first.fightId)['committed'])
        # Reuse one executable and this exact InnoDB schema across a real process restart.
        self.stop()
        self.start()
        resumed = self.bridge('resume', a, {})
        self.assertTrue(resumed['pending'])
        self.assertEqual(resumed['scene'], scene)
        self.assertEqual(self.decoded_start(resumed), first)
        old_a = a
        a = self.login(self.uids[0])
        self.bridge('finish', old_a, {'fightId': first.fightId}, success=False)
        self.assertEqual(self.decoded_start(self.bridge('resume', a, {})), first)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
            finishes = list(workers.map(lambda _: self.bridge('finish', a, {'fightId': first.fightId}), range(8)))
        self.assertEqual(sum(not row['alreadyCommitted'] for row in finishes), 1)
        self.assertEqual(len({row['startBase64'] for row in finishes}), 1)
        applied = finishes[0]['appliedDamage']
        self.assertEqual(applied, min(first_receipt['predicted_damage'], hp0['hp']))
        self.assertEqual(self.instance(boss)['hp'], hp0['hp'] - applied)
        self.assertEqual(self.receipt(first.fightId)['start_hash'], first_receipt['start_hash'])
        result = self.decoded_start(finishes[0])
        self.assertAlmostEqual(result.damageforwardPercent, 1)
        self.assertAlmostEqual(result.damagelaterPercent, (hp0['hp'] - applied) / hp0['total'])
        self.assertNotEqual(result.playerSide.state, 6)
        # A separate actual pending is deliberately abandoned, then zero-stamp new entry works.
        abandoned = self.begin(b, boss)
        before_abandon = self.instance(boss)
        self.assertTrue(self.bridge('abandon', b, {'fightId': abandoned.fightId})['abandoned'])
        self.assertTrue(self.bridge('abandon', b, {'fightId': abandoned.fightId})['abandoned'])
        self.bridge('finish', b, {'fightId': abandoned.fightId}, success=False)
        self.assertEqual(self.instance(boss), before_abandon)
        self.assertFalse(self.bridge('resume', b, {})['pending'])
        second = self.begin(b, boss)
        second_finish = self.bridge('finish', b, {'fightId': second.fightId})
        self.assertGreaterEqual(second_finish['appliedDamage'], 0)
        self.assertEqual(second_finish['appliedDamage'], self.receipt(second.fightId)['applied_damage'])
        # The primary chain above used untouched shared HP and real precomputed damage.
        report['actualPrimaryChain'] = {'initialSharedHP': hp0['hp'], 'firstApplied': applied,
                                        'secondApplied': second_finish['appliedDamage'],
                                        'afterTwoHP': self.instance(boss)['hp'],
                                        'paidDiamond': 450, 'paidBoosts': list(duplicate.propPurchased),
                                        'sameReceiptConcurrentRequests': 8, 'actualSubmissions': 1,
                                        'originalTimestampFreeTCPReplay': True, 'coldResume': True,
                                        'abandonNoDamage': True, 'rotatedOldSessionRejected': True}
        report['declaredMaximumDiamondBudgets'] = [
            {'player': a.playerId, 'boosts': 450, 'revivals': 250, 'requiresActualOrders': True},
            {'player': b.playerId, 'boosts': 0, 'revivals': 0, 'requiresActualOrders': False},
            *[{'player': account.playerId, 'boosts': 600, 'revivals': 200,
               'requiresActualOrders': True} for account in (c, d)],
        ]
        # A positive terminal is an observed prerequisite for the overkill race.
        # Retry through original revival only, at most five actual free starts
        # for each participant. Zero-damage commits remain valid evidence.
        positive = []
        attempts = []
        for account in (c, d):
            candidate = None
            # Predeclared real purchase path, never an input-panel/seed edit:
            # masks0/1/5/13/15 use only independently admitted original boosts.
            for prop in (None, 'wordboss_1', 'wordboss_3', 'wordboss_4', 'wordboss_2'):
                if prop is not None:
                    self.purchase_boost(account, prop)
                started = self.begin(account, boss)
                predicted = self.receipt(started.fightId)['predicted_damage']
                attempts.append({'player': account.playerId, 'fightId': started.fightId,
                                 'predictedDamage': predicted})
                if predicted > 0:
                    candidate = started
                    break
                finished = self.bridge('finish', account, {'fightId': started.fightId})
                self.assertEqual(finished['appliedDamage'], 0)
            positive.append(candidate)
        report['actualPositiveDamageSearch'] = {'attempts': attempts, 'maxFreeStartsPerPlayer': 5,
                                               'predeclaredPurchasedMasks': [0, 1, 5, 13, 15],
                                               'outcomeWasNotForced': True}
        if any(started is None for started in positive):
            for account, started in zip((c, d), positive):
                if started is not None:
                    self.assertTrue(self.bridge('abandon', account, {'fightId': started.fightId})['abandoned'])
            report['uncovered'] = ['two-positive-pending last-hit race', 'kill-triggered rank and reward chain',
                                   'expired pending callbacks', 'claimed rank pack subsequent use']
            report['reason'] = 'Five genuine free challenges did not provide two positive pending terminals'
            report['fullChain'] = False
            return
        # Only now prepare shared HP1, explicitly. Input, terminal and winner
        # remain those of the two actual positive precomputations above.
        pending = positive + [self.begin(account, boss) for account in (e, f)]
        boundary_before = self.instance(boss)
        self.sql('UPDATE p12_world_boss_battle_instances SET current_hp=1 WHERE id="' + boss + '";')
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as workers:
            last = list(workers.map(lambda pair: self.bridge('finish', pair[0], {'fightId': pair[1].fightId}),
                                    [(c, pending[0]), (d, pending[1])]))
        killed = self.instance(boss)
        self.assertEqual(sum(row['appliedDamage'] for row in last), 1)
        self.assertEqual(killed['hp'], 0)
        self.assertIn(killed['killer'], (c.playerId, d.playerId))
        self.assertGreater(killed['killedAt'], 0)
        self.assertEqual(sum(row['appliedDamage'] == 0 for row in last), 1)
        for row in last:
            self.assertEqual(self.decoded_start(row).damagelaterPercent, 0)
        self.sql('UPDATE p12_world_boss_battle_policies SET rewards_enabled=1 WHERE id=1;')
        # Temporarily advance only the isolated cycle end for expiry callbacks. Info/Start
        # are intentionally not invoked until its original exact boundary is restored.
        expired_at = int(time.time() * 1000) - 1000
        self.assertGreater(expired_at, cycle['start'])
        try:
            self.sql('UPDATE p12_world_boss_cycles SET ends_at=' + str(expired_at) + ' WHERE id="' + boss + '";')
            self.assertFalse(self.bridge('resume', e, {})['pending'])
            self.call('CSPlayerLogin', {}, f, 20, 'SCPlayerLogin')
            for item in pending[2:]:
                receipt = self.receipt(item.fightId)
                self.assertGreater(receipt['abandoned_at'], 0)
                self.assertFalse(receipt['committed'])
                self.assertEqual(receipt['applied_damage'], 0)
            self.assertNotEqual(self.profile(f)[1]['field']['side']['state'], 6)
        finally:
            self.sql('UPDATE p12_world_boss_cycles SET ends_at=' + str(cycle['end'])
                     + ',state="' + cycle['state'] + '",ended_at=' + str(cycle['endedAt'])
                     + ' WHERE id="' + boss + '";')
        self.assertEqual(self.sql('SELECT definition_hash FROM p12_world_boss_cycles WHERE id="' + boss + '";').strip(), cycle['hash'])
        self.bridge('finish', e, {'fightId': pending[2].fightId}, success=False)
        self.assertEqual(self.instance(boss), killed)
        report['explicitBoundaryPreparation'] = {
            'sharedHPBeforePreparation': boundary_before['hp'], 'preparedRemainingHP': 1,
            'doesNotClaimNaturalLastHit': True, 'twoDistinctActualPrecomputationsAppliedSum': 1,
            'singleKiller': True, 'losingConcurrentFinishAppliedZero': True,
            'temporaryCycleEnd': expired_at, 'originalCycleEndRestored': cycle['end'],
            'definitionJSONAndHashUntouched': True,
            'expiredResumeAndOrdinaryLoginAbandonedWithoutDamage': True}
        # Real original P1 query triggers server-side award synchronization for each owner.
        awards = []
        expiry_probe = None
        for account in (a, b, c, d):
            self.call('CSQueryUserReward', {}, account, 30, 'SCQueryUserReward')
            records = json.loads(self.sql('SELECT COALESCE(JSON_ARRAYAGG(JSON_OBJECT("kind",kind,'
                                         '"delivery",delivery_id,"expires",expires_at)),JSON_ARRAY()) '
                                         'FROM p12_world_boss_awards WHERE instance_id="' + boss + '" '
                                         'AND player_id=' + str(account.playerId) + ';'))
            self.assertTrue(records)
            for row in records:
                self.assertEqual(row['expires'], killed['killedAt'] + 7 * 86400000)
            if account.playerId == a.playerId:
                expiry_probe = next(row for row in records if row['kind'] == 0)
            ids = [row['delivery'] for row in records if row['delivery'] and row is not expiry_probe]
            before_coin = self.profile(account)[0].get('coin', 0)
            if ids:
                self.call('CSAddRewardsToPlayer', {'rewardIds': ids}, account, 31, 'SCAddRewardsToPlayer')
                after = self.profile(account)
                self.assertGreater(after[0]['coin'], before_coin)
                self.call('CSAddRewardsToPlayer', {'rewardIds': ids}, account, 32, 'SCAddRewardsToPlayer')
                self.assertEqual(self.profile(account), after)
            awards.append({'player': account.playerId, 'kinds': [row['kind'] for row in records],
                           'claimCount': len(ids)})
        self.assertIsNotNone(expiry_probe)
        # Test seven-day deadline via an explicit aged P1 deadline, not a seven-day wait.
        self.sql('UPDATE p1_reward_deliveries SET expires_at=' + str(int(time.time() * 1000) - 1000)
                 + ' WHERE id=' + str(expiry_probe['delivery']) + ' AND claimed_at=0;')
        prior = self.profile(a)
        self.assertEqual(self.stamped('CSAddRewardsToPlayer', {'rewardIds': [expiry_probe['delivery']]}, a, 33)[0], 10001)
        self.assertEqual(self.profile(a), prior)
        # The top original rank reward contains item_150; spend its genuinely claimed pack.
        owners = (a, b, c, d)
        owner = next(account for account in owners if any(item['id'] == 'item_150' and item.get('num', 0) > 0
                                                         for item in self.profile(account)[1].get('items', [])))
        before_items = self.profile(owner)[1]['items']
        pack_count = sum(item.get('num', 0) for item in before_items if item['id'] == 'item_150')
        used = self.call('CSItemUse', {'costItem': {'id': 'item_150', 'num': 1}}, owner, 40, 'SCItemUse')
        gain = [(item.id, item.num) for item in getattr(used, 'get').simps]
        self.assertEqual(len(gain), 1)
        self.assertIn(gain[0][0], {'item_140', 'item_142', 'item_144', 'item_146', 'item_148'})
        self.assertEqual(gain[0][1], 1)
        self.assertEqual(sum(item.get('num', 0) for item in self.profile(owner)[1]['items']
                             if item['id'] == 'item_150'), pack_count - 1)
        self.call('CSQueryUserReward', {}, a, 41, 'SCQueryUserReward')
        count_before = self.sql('SELECT COUNT(*) FROM p12_world_boss_awards;').strip()
        self.call('CSQueryUserReward', {}, a, 42, 'SCQueryUserReward')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p12_world_boss_awards;').strip(), count_before)
        report['awards'] = {'perPlayerKinds': awards, 'ttlMillis': 7 * 86400000,
                            'duplicateClaimNoSecondGrant': True, 'explicitAgedUnclaimedP1DeadlineRejected': True,
                            'claimedOriginalPackUsed': {'id': 'item_150', 'actualGain': gain},
                            'rewardRefreshNoDuplicateAward': True}
        rank = self.call('CSWorldBossRank', {}, a, 43, 'SCWorldBossRank')
        self.assertEqual(rank.bossHpPercent, 0)
        self.assertTrue(rank.HasField('lastDamageInfo'))
        self.assertEqual(rank.lastDamageInfo.damage, 1)
        self.assertEqual(sum(row.damage for row in rank.damageInfos),
                         applied + second_finish['appliedDamage'] + 1)
        self.stop()
        self.start()
        final = self.instance(boss)
        self.assertEqual(final, killed)
        self.assertTrue(self.bridge('finish', a, {'fightId': first.fightId})['alreadyCommitted'])
        self.assertEqual(self.instance(boss), killed)
        report['coldFinalReceiptsAndKillUnchanged'] = True
        report['fullChain'] = True

    def test_fixed_original_worldboss_lifecycle_batch(self):
        if REPORT.exists():
            self.fail('Preserve the existing report; choose a new HYJS_P12_WORLDBOSS_REPORT')
        self.binary, roster, proof = preflight()
        report = dict(proof, status='RUNNING', phase=PHASE, fullChain=False, phoneUI=False,
                      productionModified=False, nativeNumericalParity=False,
                      runnerSHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                      finitePrerequisites='six original-created hero001 isolated accounts; historical actual '
                      'hero001/003 with equip037Lv7/Pet002Lv2 only, unique per-owner PetGUID and original guard '
                      'snapshot publication; player level90/guide8 and initial original boss spawn/fullHP/AP0 '
                      'are finite entry qualifications. Original actor growth is unchanged. No wallet/items/KV/'
                      'progress/results/receipts are imported. Original P3 SKU orders and explicit loopback '
                      'test-provider confirmations credit actual diamonds, with no real money charged; '
                      'boost450 and subsequent original paid revivals50 use only those actual funds. '
                      'Separate overkill race requires two observed positive terminals then prepares sharedHP1; expiry callbacks '
                      'temporarily age isolated cycle end then restore it, keeping DefinitionJSON/hash. P1 '
                      'expiry uses one explicitly aged unclaimed deadline; seven real days are not claimed.')
        self.report = report
        self.actual_starts = {}
        self.lifecycle_stamp = 712000
        report.update(testProviderOrders=[], actualPaidRevivals=[], actualBoostPurchases=[],
                      actualStarts=[], finitePartyAccounts=[])
        window = active_original_window()
        if PHASE == 'full' and (not window or window[1] - int(time.time() * 1000) < 10 * 60000):
            report.update(status='WAITING_ORIGINAL_WINDOW', nextAction='Run full in real UTC+8 '
                          '12:00–13:00 or 21:40–22:40 with at least ten minutes remaining; contracts mode '
                          'can validate only negative/auth contracts outside the window',
                          managedAllocationsCreated=0, ownedAppStarted=False)
            REPORT.parent.mkdir(parents=True, exist_ok=True)
            REPORT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
            self.skipTest('Original world-boss window closed or less than ten minutes remain; no DB/app started')
        self.process = None
        self.sockets = []
        self.frame_opcodes = []
        self.app_temp = None
        self.pool = ManagedMySQL()
        began = time.monotonic()
        lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            lock.close()
            self.fail('SQL_BUSY: no owned schema, application or temporary directory was allocated')
        try:
            connection = self.pool.allocate('p12_worldboss_lifecycle')
            self.database = connection.database
            self.env = {key: value for key, value in os.environ.items()
                        if key in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
            self.env.update(connection.env())
            self.env.update({'HYJS_P12_MYSQL_' + key: value for key, value in {
                'HOST': connection.host, 'USER': connection.user, 'PASSWORD': connection.password,
                'DATABASE': connection.database}.items()})
            self.env['TZ'] = 'Asia/Shanghai'
            self.app_temp = tempfile.TemporaryDirectory(prefix='hyjs-p12-worldboss-lifecycle-')
            self.start()
            self.assertEqual(self.sql('SELECT authoritative_enabled FROM p6_combat_rules WHERE id=1;').strip(), '0')
            report['globalP6AuthoritativeEnabled'] = False
            self.assertEqual(self.sql('SELECT DISTINCT ENGINE FROM information_schema.TABLES '
                                      'WHERE TABLE_SCHEMA="' + self.database + '" '
                                      'AND TABLE_NAME LIKE "p12_world_boss_%";').strip(), 'InnoDB')
            # The P3 test provider always protects profile1, even in an empty
            # isolated schema. Reserve that row without any payment or growth.
            sentinel = self.login('demo-p12-reserved-' + secrets.token_hex(8))
            report['isolatedReservedPlayer'] = sentinel.playerId
            self.uids = ['demo-p12-lifecycle-' + secrets.token_hex(8) for _ in range(6 if PHASE == 'full' else 2)]
            accounts = [self.login(uid) for uid in self.uids]
            report['contracts'] = self.contracts(accounts)
            if PHASE == 'full':
                report['realOriginalWindow'] = {'startsAt': window[0], 'endsAt': window[1]}
                self.full_chain(accounts, roster, report)
            report['status'] = ('PASS' if report['fullChain'] else 'INCOMPLETE_ACTUAL_POSITIVE_DAMAGE_REQUIRED') \
                if PHASE == 'full' else 'PASS_CONTRACTS_ONLY'
            if PHASE == 'full' and not report['fullChain']:
                self.skipTest(report['reason'])
        except unittest.SkipTest:
            # Actual zero damage leaves the dependent business branch unproved;
            # preserve that result without labelling the whole chain PASS.
            raise
        except Exception as error:
            report['status'] = 'FAIL'
            report['failureType'] = type(error).__name__
            report['failure'] = self.pool.redact(str(error))[:1500]
            raise
        finally:
            try:
                self.close_sockets()
                self.stop()
            finally:
                try:
                    self.pool.close()
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    report['ownedAppStopped'] = self.process is None or self.process.poll() is not None
                except Exception as error:
                    report['status'] = 'FAIL_OWNED_MANAGED_CLEANUP'
                    report['cleanupFailure'] = self.pool.redact(str(error))[:1000]
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    raise
                finally:
                    if self.app_temp is not None:
                        self.app_temp.cleanup()
                        report['ownedLogDirectoryRemoved'] = True
                    fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
                    lock.close()
                    report['seconds'] = round(time.monotonic() - began, 3)
                    report['frameOpcodes'] = self.frame_opcodes
                    REPORT.parent.mkdir(parents=True, exist_ok=True)
                    REPORT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
