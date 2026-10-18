"""One Natural047025 Trial floor via prebuilt app + independent prebuilt exporter.

Finite isolated prerequisite: captured actual047tal1/025Lv3/equip001Lv2,
Player22 only. No natural level-up claim, wallet/progress/result/receipt import,
phone access, Go compilation or development database writes.
"""
import base64
import concurrent.futures
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import tempfile
import time
import unittest

from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME, CLASSES, parse_http
import test_continued_gameplay_mysql as wire
from test_p7_maze_trader_mysql import P7MazeTraderMySQLTests as NativeTransport

ROOT = Path(__file__).resolve().parents[1]
SOURCE_ROOT = Path(os.environ.get('HYJS_FUNCTION_NATURAL_RUSH_SOURCE_ROOT', ROOT)).resolve()
VALIDATION = Path(os.environ.get('HYJS_FUNCTION_NATURAL_RUSH_VALIDATION',
    ROOT / 'analysis/p3/function-player-rename-r1-incremental-20261017.json')).resolve()
REPORT = Path(os.environ.get('HYJS_FUNCTION_NATURAL_RUSH_REPORT',
    ROOT / 'analysis/p7/function-natural-rush-http-mysql-r1.json')).resolve()
INPUT_MANIFEST = ROOT / 'analysis/p6/p7-numeric047025-local-inputs-r1-20261017/manifest.json'
INPUT_SHA = '7e74d64e647ee3bc0de9f48afd10cea8bae14ea14461652342d06b88ab8eceac'
RECORDS = ROOT / 'analysis/p6/p7-numeric047025-android-fixtures-r1-20261017/records.json'
RECORDS_SHA = 'be6883a4d93de76ad167402ea02251d66bf1000bc714280deaa7404d05bf5bf6'
CERTIFICATE = ROOT / 'analysis/p6/natural047025-numeric-android-capability-certificate-20261017.json'
PROOF = '538e98ce9a3aa2d4f9a7aa9cb87e74b0051ef130e824c264f54af4a7c27bee40'
PROFILE = ROOT / 'analysis/growth-first-20261016/m1-natural-phone-20261017/102-first-real-strengthen-profile.json'
PROFILE_SHA = '2fefea1a2ac41e5f38ff946942e70012a07d63b614758f0d409bc3215facc06c'


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def protocol_preflight():
    for path, sha in ((INPUT_MANIFEST, INPUT_SHA), (RECORDS, RECORDS_SHA),
                      (CERTIFICATE, PROOF), (PROFILE, PROFILE_SHA)):
        if digest(path) != sha:
            raise AssertionError('Fixed original source changed: ' + str(path))
    manifest = json.loads(INPUT_MANIFEST.read_bytes())
    records = json.loads(RECORDS.read_bytes())
    if len(manifest['cases']) != 4 or len(records['cases']) != 4:
        raise AssertionError('Four same-source Android anchors required')
    if records['wireSourceManifestSHA256'] != INPUT_SHA:
        raise AssertionError('Original Android records bound to another input')
    for source, record in zip(manifest['cases'], records['cases']):
        for key, sha_key in [('wire', 'wireSHA256'), ('typedCallback', 'typedCallbackSHA256')]:
            if digest(INPUT_MANIFEST.parent / source[key]) != source[sha_key]:
                raise AssertionError('Original input changed')
        raw = base64.b64decode(record['originalFinalRecord']['protobufBase64'], validate=True)
        if (record['wireSHA256'] != source['wireSHA256'] or
                hashlib.sha256(raw).hexdigest() != record['originalFinalRecord']['protobufSHA256']):
            raise AssertionError('Android original complete record changed')
        CLASSES['BattleOperate'].FromString(raw)
    captured = json.loads(PROFILE.read_bytes())
    game = captured['game']
    warriors = [v for v in game['warriors'] if v['id'] in ('hero_047', 'hero_025')]
    if [(v['id'], v['talent']) for v in warriors] != [('hero_047', 1), ('hero_025', 0)]:
        raise AssertionError('Actual tutorial skill/talent prerequisite changed')
    if warriors[1]['level'] != 3 or len(game['equipment']) != 1 or game['equipment'][0]['level'] != 2:
        raise AssertionError('Actual companion/strengthened equipment prerequisite changed')
    anchor = CLASSES['SCRushFightStart'].FromString(
        (INPUT_MANIFEST.parent / manifest['cases'][0]['typedCallback']).read_bytes())
    vectors = [
        ('CSPlayerCreate', {'playerName': '原双人功能', 'heroId': 'hero_047'}),
        ('CSFormationList', {}), ('CSGetRushInfo', {}), ('CSRushCurrentInfo', {}),
        ('CSRushFightStart', {'rushType': 1, 'taskId': 'local-section-1', 'star': 1}),
        ('CSRushFightEnd', {'fightId': 'preflight-only', 'result': 1,
                            'optRecord': CLASSES['BattleOperate'].FromString(base64.b64decode(
                                records['cases'][0]['originalFinalRecord']['protobufBase64']))}),
    ]
    for name, fields in vectors:
        encoded = wire.encode_original_http(name, fields, 2, b'0' * 16, 1)
        request = parse_http(encoded)
        if request.opcode != BY_NAME[name] or request.decode() != CLASSES[name](**fields):
            raise AssertionError('Original request preflight failed: ' + name)
    return captured, anchor, {'originalRequestVectors': len(vectors),
                             'androidAnchorCases': 4, 'proof': PROOF,
                             'inputManifestSHA256': INPUT_SHA, 'recordsSHA256': RECORDS_SHA}


def binary_preflight():
    if SOURCE_ROOT.parent != ROOT / 'build/candidates' or not SOURCE_ROOT.is_dir():
        raise AssertionError('Use one direct fixed candidate SOURCE_ROOT')
    validation = json.loads(VALIDATION.read_bytes())
    if not validation.get('qualified') or (ROOT / validation['candidate']).resolve() != SOURCE_ROOT:
        raise AssertionError('Qualified candidate binding differs')
    binary = Path(os.environ['HYJS_FUNCTION_NATURAL_RUSH_BINARY']).resolve()
    if digest(binary) != validation.get('binarySHA256'):
        raise AssertionError('Prebuilt application differs from qualification')
    manifest_path = SOURCE_ROOT / 'manifest.json'
    entries = json.loads(manifest_path.read_bytes())['files']
    listed = set()
    for item in entries:
        path = (SOURCE_ROOT / item['path']).resolve()
        if SOURCE_ROOT not in path.parents or path in listed or digest(path) != item['sha256']:
            raise AssertionError('Fixed candidate source changed: ' + item['path'])
        listed.add(path)
    if set((SOURCE_ROOT / 'hyjs-server').rglob('*.go')) - listed:
        raise AssertionError('Candidate Go files absent from manifest')
    for name in ('server/codec.py', 'server/protocol/schema.json'):
        if digest(ROOT / name) != digest(SOURCE_ROOT / name):
            raise AssertionError('Original protocol codec differs from fixed candidate')
    if PROOF not in (SOURCE_ROOT / 'hyjs-server/constUtil/p7NaturalRushScope.go').read_text():
        raise AssertionError('Natural Android family is not activated in selected binary source')
    exporter = SOURCE_ROOT / 'hyjs-server/service/p7NaturalRushHTTPExport_test.go'
    if exporter not in listed or 'TestP7NaturalRushExportOriginalHTTPStart' not in exporter.read_text():
        raise AssertionError('Fixed candidate omits the independent original Start exporter')
    calculator = Path(os.environ['HYJS_FUNCTION_NATURAL_RUSH_SERVICE_TEST']).resolve()
    expected = os.environ['HYJS_FUNCTION_NATURAL_RUSH_SERVICE_TEST_SHA256']
    if not re.fullmatch('[0-9a-f]{64}', expected) or digest(calculator) != expected:
        raise AssertionError('Root-prebuilt same-source service.test hash differs')
    if REPORT.exists() or SOURCE_ROOT in REPORT.parents or REPORT.parent != ROOT / 'analysis/p7':
        raise AssertionError('Use a fresh report directly under analysis/p7')
    return binary, calculator, digest(manifest_path)


@unittest.skipUnless(os.environ.get('HYJS_FUNCTION_NATURAL_RUSH_MYSQL_TEST') == '1',
                     'Explicit root-coordinated prebuilt managed SQL window required')
class FunctionNaturalRushMySQLTests(unittest.TestCase):
    send = wire.ContinuedGameplayMySQLTests.send
    login = wire.ContinuedGameplayMySQLTests.login
    stamped = wire.ContinuedGameplayMySQLTests.stamped
    call = wire.ContinuedGameplayMySQLTests.call
    profile = wire.ContinuedGameplayMySQLTests.profile
    stop = wire.ContinuedGameplayMySQLTests.stop
    sql = wire.ContinuedGameplayMySQLTests.sql
    start = NativeTransport.start

    def test_one_original_trial_strict_end_reward_eight_retries_and_cold(self):
        captured, anchor, preflight = protocol_preflight()
        self.binary, calculator, manifest_sha = binary_preflight()
        report = {'status': 'PENDING', 'binarySHA256': digest(self.binary),
                  'serviceTestSHA256': digest(calculator), 'sourceManifestSHA256': manifest_sha,
                  'runnerSHA256': digest(Path(__file__)), 'preflight': preflight,
                  'finitePrerequisite': 'Actual captured047tal1/025Lv3/equip001Lv2 imported ONLY into new managed profile; Player22 finite qualification; no natural unlocking claim',
                  'walletItemsProgressResultsReceiptsImported': False, 'phoneUI': False,
                  'formalProfilesReadOrWritten': False, 'actualBattles': 0}
        self.process = None
        self.pool = ManagedMySQL()
        began = time.monotonic()
        lock = open('/tmp/hyjs-isolated-mysql.lock', 'a+')
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            lock.close()
            self.fail('SQL_BUSY: no allocation or app started')
        try:
            with tempfile.TemporaryDirectory(prefix='hyjs-function-natural-') as temporary:
                self.native_log = Path(temporary) / 'app.log'
                allocation = self.pool.allocate('function_natural_rush')
                self.database = allocation.database
                self.source = SOURCE_ROOT
                self.env = {key: value for key, value in os.environ.items()
                            if key in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                self.env.update(allocation.env())
                self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                self.start()
                tables = ('profiles', 'p7_rush_fights', 'p7_rush_manual_calculations',
                          'p7_rush_receipts', 'p6_battle_records', 'p1_event_facts')
                engines = self.sql('SELECT TABLE_NAME,ENGINE FROM information_schema.TABLES WHERE '
                    'TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN (' + ','.join('"' + n + '"' for n in tables) + ');').splitlines()
                self.assertEqual({x.split('\t')[0] for x in engines}, set(tables))
                self.assertTrue(all(x.endswith('\tInnoDB') for x in engines))
                uid = 'function-natural-' + secrets.token_hex(6)
                owner = self.login(uid)
                self.call('CSPlayerCreate', {'playerName': '原双人功能', 'heroId': 'hero_047'},
                          owner, 1, 'SCPlayerCreate')
                fresh = self.profile(owner)
                self.assertEqual(tuple(fresh[0].get(k, 0) for k in ('coin', 'diamond', 'point')), (0, 0, 0))
                patch = {k: captured['game'][k] for k in ('warriors', 'equipment', 'formation')}
                patch['warriors'] = [v for v in patch['warriors'] if v['id'] in ('hero_047', 'hero_025')]
                encoded = lambda v: json.dumps(v, ensure_ascii=False, separators=(',', ':')).encode().hex()
                paths = ','.join("'$." + k + "',CAST(CONVERT(0x" + encoded(v) + ' USING utf8mb4) AS JSON)'
                                 for k, v in patch.items())
                self.sql("UPDATE profiles SET player=JSON_SET(player,'$.level',22,'$.heroWarId',"
                         f"CONVERT(0x{captured['player']['heroWarId'].encode().hex()} USING utf8mb4)),"
                         f'game=JSON_SET(game,{paths}) WHERE id={owner.playerId};')
                expected = copy.deepcopy(fresh)
                expected[0].update(level=22, heroWarId=captured['player']['heroWarId'])
                expected[1].update(patch)
                self.assertEqual(self.profile(owner), expected)
                self.call('CSFormationList', {}, owner, 2, 'SCFormationList')
                self.sql('UPDATE p7_rush_policies SET enabled=1 WHERE id=1; '
                         'UPDATE p7_rush_scoped_policies SET enabled=1 WHERE id=1; '
                         'UPDATE p7_rush_manual_policies SET enabled=1 WHERE id=1; '
                         'UPDATE p7_rush_stage_rules SET enabled=1 WHERE id="local-section:1:1";')
                # Rule primary ID differs from the original taskId. Use the
                # actual original page projection, as the native client does.
                tasks = self.call('CSRushCurrentInfo', {}, owner, 3, 'SCRushCurrentInfo').taskInfoList
                self.assertEqual(len(tasks), 1)
                self.assertEqual(tasks[0].star, 1)
                args = {'rushType': 1, 'taskId': tasks[0].taskId, 'star': 1}
                start = self.call('CSRushFightStart', args, owner, 10, 'SCRushFightStart')
                self.assertEqual(start, self.call('CSRushFightStart', args, owner, 10, 'SCRushFightStart'))
                self.assertEqual([h.id for h in start.battlePlayer.formation], ['hero_047', 'hero_025'])
                for actual, original in zip(start.battlePlayer.formation, anchor.battlePlayer.formation):
                    self.assertEqual(actual, original, 'Production panel must equal same finite original source')
                source_row = self.sql('SELECT CONCAT(status,"|",scope_proof,"|",LENGTH(COALESCE(terminal,""))) '
                    f'FROM p7_rush_manual_calculations WHERE player_id={owner.playerId} AND fight_id="{start.fightId}";').strip()
                self.assertEqual(source_row, '0|' + PROOF + '|0')
                evidence = REPORT.with_suffix('')
                self.assertTrue(evidence.name.startswith('function-natural-rush-'))
                self.assertFalse(evidence.exists())
                evidence.mkdir()
                start_file = evidence / '001.SCRushFightStart.pb'
                terminal_file = evidence / '001.calculated.json'
                start_file.write_bytes(start.SerializeToString(deterministic=True))
                start_file.chmod(0o600)
                env = dict(self.env, HYJS_P7_NATURAL_HTTP_START=str(start_file),
                           HYJS_P7_NATURAL_HTTP_START_SHA256=digest(start_file),
                           HYJS_P7_NATURAL_HTTP_AUTO_AT='0',
                           HYJS_P7_NATURAL_HTTP_OUTPUT=str(terminal_file))
                calculation = subprocess.run([str(calculator),
                    '-test.run=^TestP7NaturalRushExportOriginalHTTPStart$', '-test.count=1', '-test.v'],
                    cwd=SOURCE_ROOT / 'hyjs-server/service', env=env,
                    capture_output=True, text=True, timeout=120)
                self.assertEqual(calculation.returncode, 0,
                                 self.pool.redact(calculation.stdout + calculation.stderr)[-4000:])
                self.assertIn('--- PASS: TestP7NaturalRushExportOriginalHTTPStart ', calculation.stdout)
                terminal = json.loads(terminal_file.read_bytes())
                self.assertFalse(terminal['outcome']['authoritative'])
                actual_result = int(terminal['outcome']['winner'] == 0)
                end = {'fightId': start.fightId, 'result': actual_result,
                       'optRecord': terminal['originalOperates']}
                held = self.profile(owner)
                self.call('CSRushFightEnd', dict(end, result=1 - actual_result), owner, 20, 'ErrorMessage')
                altered = copy.deepcopy(end)
                altered['optRecord']['attackerTotalLeftHp'] += 1
                self.call('CSRushFightEnd', altered, owner, 21, 'ErrorMessage')
                self.assertEqual(self.profile(owner), held)
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                    replies = list(pool.map(lambda _: self.call('CSRushFightEnd', end, owner, 22,
                                                               'SCRushFightEnd'), range(8)))
                receipt = replies[0]
                self.assertTrue(all(v == receipt for v in replies))
                self.assertEqual(receipt.result, actual_result)
                accepted = self.profile(owner)
                self.assertEqual(accepted[0].get('point', 0), held[0].get('point', 0) + 10 * actual_result)
                self.assertEqual(accepted[0].get('coin', 0), held[0].get('coin', 0))
                row = self.sql('SELECT CONCAT(status,"|",scope_proof,"|",CHAR_LENGTH(end_request_hash)) '
                    f'FROM p7_rush_manual_calculations WHERE player_id={owner.playerId};').strip()
                self.assertEqual(row, '1|' + PROOF + '|64')
                self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p6_battle_records WHERE player_id={owner.playerId} AND authoritative=1;').strip(), '1')
                self.stop()
                self.start()
                owner = self.login(uid)
                self.assertEqual(self.call('CSRushFightEnd', end, owner, 22, 'SCRushFightEnd'), receipt)
                self.assertEqual(self.call('CSRushFightEnd', end, owner, 23, 'SCRushFightEnd'), receipt)
                self.assertEqual(self.profile(owner), accepted)
                report.update(status='PASS_ORIGINAL_HTTP_INNODB_NATURAL_FLOOR1_STRICT_END_REWARD_COLD',
                    actualBattles=1, actualWinner=terminal['outcome']['winner'],
                    actualFinalRound=terminal['outcome']['finalRound'], pointReward=10 * actual_result,
                    actualStartSHA256=digest(start_file), calculatedTerminalSHA256=digest(terminal_file),
                    originalFullEndOperations=len(end['optRecord']['skillOpts']),
                    sameStampConcurrent8Once=True, forgedWinnerAndHPRejected=True,
                    coldOriginalReceiptOnce=True, noOldAndroidEndSubmitted=True)
        except BaseException as error:
            report.update(status='FAIL_PARTIAL_NOT_FULL_ACCEPTANCE', error=self.pool.redact(repr(error)))
            raise
        finally:
            errors = []
            try:
                self.stop()
            except BaseException as error:
                errors.append(type(error).__name__)
            try:
                self.pool.close()
            except BaseException as error:
                errors.append(type(error).__name__)
            report.update(seconds=round(time.monotonic() - began, 3), cleanupErrors=errors,
                          remainingOwnedAllocations=len(self.pool.connections))
            REPORT.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
            fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
            lock.close()
            if errors:
                self.fail('Owned cleanup requires inspection: ' + ','.join(errors))


if __name__ == '__main__':
    unittest.main()
