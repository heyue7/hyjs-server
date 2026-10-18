"""Prebuilt original rename query/free/300diamond/receipt/cold short contract.

Only a finite 300-diamond balance is prepared in this owned isolated schema.
No level/inventory/identity/result injection, payment, Go compiler or device use.
"""
import base64
import concurrent.futures
import fcntl
import hashlib
import http.client as http_client
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import types
import unittest

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from server.codec import CLASSES, BY_NAME, parse_http
import test_continued_gameplay_mysql as wire
import test_local_wallet_mysql as app_transport

ROOT = Path(__file__).resolve().parents[1]


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def protocol_preflight():
    vectors = [('CSChangeNameTimes', {}),
               ('CSChangePlayerName', {'playerName': '六个汉字正好'}),
               ('CSChangePlayerName', {'playerName': 'Nick12345678'})]
    for name, fields in vectors:
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 911001)
        packet = parse_http(raw)
        if packet.opcode != BY_NAME[name] or packet.decode() != CLASSES[name](**fields):
            raise AssertionError('Original request differs: ' + name)
    expected = {'CSChangePlayerName': 20017, 'SCChangePlayerName': 20018,
                'CSChangeNameTimes': 20115, 'SCChangeNameTimes': 20116}
    if any(BY_NAME[name] != opcode for name, opcode in expected.items()):
        raise AssertionError('Original rename opcode mapping differs')
    for name, fields in {'SCChangePlayerName': {'cost'}, 'SCChangeNameTimes': {'changeNameTimes'},
                         'BaseResource': {'id', 'num'}, 'Player': {'name', 'diamond'}}.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original rename consumer differs: ' + name)
    return dict(originalRequestVectors=len(vectors), responseDescriptors=4,
                firstFree=True, paidDiamond=300, utf16WidthLimit=12,
                finitePrerequisite='only300diamond in owned isolated profile; not a natural-source claim',
                clientCallbackDeletesCost=True, extraSCPlayerForbidden=True,
                duplicateDisplayNames='existing Create compatibility; ownership stays playerID')


def source_contract(source):
    game = (source / 'hyjs-server/service/gameService.go').read_text()
    for token in ['case *pb.CSChangePlayerName:', 's.changePlayerName(ctx, row.Id, b, r.Timestamp)',
                  'case *pb.CSChangeNameTimes:', 's.changeNameTimes(ctx, row.Id)']:
        if token not in game:
            raise AssertionError('Original rename dispatch missing: ' + token)
    handler = (source / 'hyjs-server/handler/gameHandler.go').read_text()
    if 'case *pb.CSChangePlayerName:' not in handler or 'constUtil.PlayerRenameInvalid' not in handler:
        raise AssertionError('Original rename input guard missing')
    service = (source / 'hyjs-server/service/playerRenameService.go').read_text()
    for token in ['Strength: "UPDATE"', 'readPlayerRenameState', 'state.RequestHash',
                  'state.ResponseHash', 'EditNameCost', 'LocalPlayNow(ctx)', 'saveProfile(tx, &row, p, g)']:
        if token not in service:
            raise AssertionError('Actual rename transaction missing: ' + token)
    if 'lockPlayerNameRegistry' in service or 'ensurePlayerNameAvailable' in service:
        raise AssertionError('This batch must not invent global display-name uniqueness')
    main = (source / 'hyjs-server/main.go').read_text()
    if 's.InitializeP1(ctx)' not in main:
        raise AssertionError('Normal P1 cost startup is missing')
    constants = (source / 'hyjs-server/constUtil/playerRename.go').read_text()
    if 'player.rename.v1' not in constants:
        raise AssertionError('Persistent per-owner KV key missing')
    profile = (source / 'hyjs-server/db/profile.go').read_text()
    if 'return "profiles"' not in profile or 'column:player' not in profile or 'column:game' not in profile:
        raise AssertionError('Actual profile SQL projection differs')
    fact = (source / 'hyjs-server/db/eventFact.go').read_text()
    if 'return "p1_event_facts"' not in fact or 'SourceKey' not in fact or 'Amount' not in fact:
        raise AssertionError('Actual P1 cost ledger fields differ')
    lines = (source / 'analysis/tables/TableGameConfigure.tsv').read_text().splitlines()
    rows = [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line]
    if next(r['NameValue'] for r in rows if r['StaticName'] == 'EditNameCost') != 'diamond,300':
        raise AssertionError('Original paid rename price differs')
    return protocol_preflight()


def preflight():
    source = Path(os.environ.get('HYJS_RENAME_SOURCE', ROOT / 'build/candidates/function-player-rename-r1-20261017')).resolve()
    binary = Path(os.environ.get('HYJS_RENAME_BINARY', ROOT / 'build/hyjs-server-function-player-rename-r1-20261017-pending')).resolve()
    validation = Path(os.environ.get('HYJS_RENAME_VALIDATION', ROOT / 'analysis/p3/function-player-rename-r1-incremental-20261017.json')).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use a direct fixed candidate')
    manifest = source / 'manifest.json'
    paths = set()
    for entry in json.loads(manifest.read_bytes())['files']:
        path = (source / entry['path']).resolve()
        if not path.is_relative_to(source) or path in paths or sha(path) != entry['sha256']:
            raise AssertionError('Fixed candidate pin differs: ' + entry['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Unpinned Go source')
    proof = json.loads(validation.read_bytes())
    if proof.get('qualified') is not True or (ROOT / proof['candidate']).resolve() != source:
        raise AssertionError('Exact completed qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Use the exact prebuilt qualified app')
    for rel in ['server/codec.py', 'server/protocol/schema.json', 'analysis/tables/TableGameConfigure.tsv']:
        if sha(source / rel) != sha(ROOT / rel):
            raise AssertionError('Original protocol/table differs: ' + rel)
    return source, binary, sha(manifest), source_contract(source)


@unittest.skipUnless(os.environ.get('HYJS_RENAME_MYSQL_TEST') == '1', 'Root prebuilt managed window')
class FunctionPlayerRenameMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    profile, stop, sql = transport.profile, transport.stop, transport.sql

    def start(self):
        app_transport.SOURCE_ROOT = self.source
        app_transport.LocalWalletMySQLTests.start(self, wallet=False)

    def stamped(self, name, fields, owner, stamp):
        raw = wire.encode_original_http(name, fields, owner.playerId, bytes(owner.sessionKey), stamp)
        connection = http_client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=40)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            self.assertEqual(response.status, 200)
            frames = wire.decode_responses(response.read())
            self.assertEqual(len(frames), 1, 'Only the original result is legal; extraSCPlayer double-debits the client')
            return frames[0]
        finally:
            connection.close()

    def raw_profile(self, owner):
        return self.sql(f'SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id={owner.playerId};').strip()

    def count(self, owner, stamp=911001):
        return self.call('CSChangeNameTimes', {}, owner, stamp, 'SCChangeNameTimes').changeNameTimes

    def receipt(self, owner, name, count, stamp, response):
        p, game = self.profile(owner)
        states = [entry['value'] for entry in game.get('kv', []) if entry['key'] == 'player.rename.v1']
        self.assertEqual(len(states), 1)
        state = json.loads(states[0])
        self.assertEqual((p['name'], state['name'], state['count'], state['stamp']), (name, name, count, stamp))
        req = CLASSES['CSChangePlayerName'](playerName=name).SerializeToString()
        out = response.SerializeToString()
        self.assertEqual(base64.b64decode(state.get('request') or ''), req)
        self.assertEqual(base64.b64decode(state.get('response') or ''), out)
        self.assertEqual(state['requestHash'], hashlib.sha256(req).hexdigest())
        self.assertEqual(state['responseHash'], hashlib.sha256(out).hexdigest())
        return state

    def test_original_free_paid_once_input_counter_owner_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_RENAME_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/function-player-rename-r1-20261017'))
        self.assertFalse(output.exists(), 'Keep previous attempt evidence')
        report = dict(status='PENDING', fullChain=False, runnerSHA256=sha(__file__), binarySHA256=sha(binary),
            sourceManifestSHA256=manifest_sha, contract=contract, phoneUI=False, productionModified=False,
            externalPayment=False, finiteDiamond=300, phase='allocation')
        self.pool, self.process = ManagedMySQL(), None
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-player-rename-') as directory:
            self.app_temp = types.SimpleNamespace(name=directory)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('player_rename')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-rename-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '改名保护', 'heroId': 'hero_001'}, protected, 911000, 'SCPlayerCreate')
                    protected_before = self.raw_profile(protected)
                    uid = 'demo-player-rename-' + suffix
                    owner = self.login(uid)
                    self.call('CSPlayerCreate', {'playerName': '旧昵称', 'heroId': 'hero_047'}, owner, 911001, 'SCPlayerCreate')
                    self.assertGreater(owner.playerId, 1)
                    self.assertEqual(self.count(owner), 0)
                    initial = self.profile(owner)
                    before = self.raw_profile(owner)
                    report['phase'] = 'original-invalid-width-control-whitespace-no-cost'
                    invalid = ['', ' ', ' 前空格', '尾空格 ', '1234567890123', '七个汉字超限了', '甲\n乙', '甲\u200b乙']
                    for i, name in enumerate(invalid):
                        self.call('CSChangePlayerName', {'playerName': name}, owner, 911010 + i, 'ErrorMessage')
                    self.call('CSChangePlayerName', {'playerName': '合法昵称'}, owner, 0, 'ErrorMessage')
                    self.assertEqual(self.raw_profile(owner), before)
                    self.assertEqual(self.count(owner), 0)
                    report['phase'] = 'finite300diamond-only-original-first-free'
                    self.assertEqual(initial[0].get('diamond', 0), 0)
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.diamond',300) WHERE id={owner.playerId} AND uid='" + uid + "';")
                    funded = self.profile(owner)
                    self.assertEqual(funded[0], dict(initial[0], diamond=300))
                    self.assertEqual(funded[1], initial[1])
                    free_name, free_stamp = '六个汉字正好', 911100
                    free_request = {'playerName': free_name}
                    free = self.call('CSChangePlayerName', free_request, owner, free_stamp, 'SCChangePlayerName')
                    self.assertEqual(len(free.cost), 0)
                    self.assertEqual(self.profile(owner)[0]['diamond'], 300)
                    self.assertEqual(self.count(owner), 1)
                    self.receipt(owner, free_name, 1, free_stamp, free)
                    free_bytes = self.raw_profile(owner)
                    self.assertEqual(self.call('CSChangePlayerName', free_request, owner, free_stamp, 'SCChangePlayerName'), free)
                    self.assertEqual(self.raw_profile(owner), free_bytes)
                    # Existing Create permits this public display name. Counts
                    # and actual mutations must stay isolated by real playerID.
                    peer = self.login('demo-rename-peer-' + suffix)
                    self.call('CSPlayerCreate', {'playerName': free_name, 'heroId': 'hero_047'}, peer, 911101, 'SCPlayerCreate')
                    peer_before = self.raw_profile(peer)
                    self.assertEqual(self.count(peer), 0)
                    self.assertEqual(self.profile(peer)[0]['name'], free_name)
                    report['phase'] = 'eight-original-paid-requests-one300diamond-debit'
                    paid_name, paid_stamp = 'Nick12345678', 911200
                    paid_request = {'playerName': paid_name}
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        results = list(workers.map(lambda _: self.call('CSChangePlayerName', paid_request, owner, paid_stamp, 'SCChangePlayerName'), range(8)))
                    self.assertTrue(all(out == results[0] for out in results))
                    paid = results[0]
                    self.assertEqual([(v.id, v.num) for v in paid.cost], [('diamond', 300)])
                    self.assertEqual(self.count(owner), 2)
                    final = self.profile(owner)
                    self.assertEqual((final[0]['name'], final[0].get('diamond', 0)), (paid_name, 0))
                    for key in ['coin', 'point', 'level', 'exp', 'energy', 'stamina']:
                        self.assertEqual(final[0].get(key, 0), funded[0].get(key, 0), key)
                    paid_state = self.receipt(owner, paid_name, 2, paid_stamp, paid)
                    final_bytes = self.raw_profile(owner)
                    event_where = f"player_id={owner.playerId} AND source_key='player-rename:{paid_stamp}' AND kind='diamond_spent'"
                    self.assertEqual(self.sql('SELECT COUNT(*),SUM(amount) FROM p1_event_facts WHERE ' + event_where + ';').strip(), '1\t300')
                    for request, stamp in [(free_request, free_stamp), ({'playerName': '篡改昵称'}, paid_stamp), ({'playerName': paid_name}, paid_stamp + 1), ({'playerName': '余额不足'}, paid_stamp + 2)]:
                        self.call('CSChangePlayerName', request, owner, stamp, 'ErrorMessage')
                    self.assertEqual(self.raw_profile(owner), final_bytes)
                    self.assertEqual(self.count(owner), 2)
                    self.assertEqual(self.raw_profile(peer), peer_before)
                    spoof = types.SimpleNamespace(playerId=protected.playerId, sessionKey=owner.sessionKey)
                    self.call('CSChangePlayerName', {'playerName': '越权昵称'}, spoof, paid_stamp + 3, 'ErrorMessage')
                    self.assertEqual(self.raw_profile(protected), protected_before)
                    report['phase'] = 'cold-name-counter-cost-only-receipt-and-owner-isolation'
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.raw_profile(owner), final_bytes)
                    self.assertEqual(self.count(owner), 2)
                    self.assertEqual(self.call('CSChangePlayerName', paid_request, owner, paid_stamp, 'SCChangePlayerName'), paid)
                    self.assertEqual(self.receipt(owner, paid_name, 2, paid_stamp, paid), paid_state)
                    self.assertEqual(self.raw_profile(owner), final_bytes)
                    self.assertEqual(self.sql('SELECT COUNT(*),SUM(amount) FROM p1_event_facts WHERE ' + event_where + ';').strip(), '1\t300')
                    self.assertEqual(self.raw_profile(peer), peer_before)
                    self.assertEqual(self.raw_profile(protected), protected_before)
                    engine = self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME IN ('profiles','p1_event_facts');").strip()
                    self.assertEqual(engine, 'InnoDB')
                    report.update(status='PASS', fullChain=True, original20017to20018=True, original20115to20116=True,
                        invalidNamesRejected=len(invalid) + 1, actualFirstFree=True, actualPaidDiamond=300,
                        actualCount=2, eightConcurrentPaidOnce=True, actualP1CostOnce=True,
                        onlyOriginalCostFrame=True, noExtraSCPlayer=True, profileQueryUnchanged=True,
                        duplicateDisplayNameOwnerIsolation=True, coldReceiptExact=True,
                        coldName=paid_name, protected1Exact=True)
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
