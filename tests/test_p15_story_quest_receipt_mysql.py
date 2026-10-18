"""Original quest18 reward -> exact retry/cold -> earned equipment/coin use.

Uses one explicitly qualified prebuilt binary and one managed temporary schema.
Only level30 and prior original story/mission qualification are finite fixtures;
no wallet, reward inventory, GUID or receipt is inserted. No historical natural
battle, NPC interaction, development account or phone UI verification is claimed.
"""
import concurrent.futures
import fcntl
import hashlib
import http.client
import json
import os
from pathlib import Path
import secrets
import struct
import tempfile
import time
import unittest

from server.codec import BY_NAME, CLASSES, decode_responses, parse_http
from hyjs_managed_mysql import ManagedMySQL
import test_continued_gameplay_mysql as transport


ROOT = Path(__file__).resolve().parents[1]
SOURCE_ROOT = Path(os.environ.get('HYJS_P15_STORY_SOURCE_ROOT', ROOT)).expanduser().resolve()
VALIDATION = Path(os.environ.get('HYJS_P15_STORY_VALIDATION',
                                ROOT / 'analysis/p3/p15-story-n2-incremental-20261016.json')).expanduser().resolve()
REPORT = Path(os.environ.get('HYJS_P15_STORY_REPORT',
                            ROOT / 'analysis/p15/story-quest-receipt-protocol-20261016.json')).expanduser().resolve()


def original_rows(name):
    lines = (SOURCE_ROOT / 'analysis/tables' / ('Table' + name + '.tsv')).read_text().splitlines()
    return [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line]


def protocol_preflight():
    """Construct every original request without service/database allocation."""
    vectors = [
        ('CSUserLogin', {'platformUid': 'p15-story-offline', 'authCode': 'isolated',
                         'clientTime': 1700000000000}, 1),
        ('CSPlayerCreate', {'playerName': '主线真链', 'heroId': 'hero_047'}, 1),
        ('CSQuestReward', {'questId': 'quest_16'}, 100),
        ('CSQuestReward', {'questId': 'quest_18'}, 100),
        ('CSQuestReward', {'questId': 'quest_18'}, 101),
        ('CSQuestReward', {'questId': 'quest_19'}, 100),
        ('CSQuestReward', {'questId': 'quest_18'}, 0),
        ('CSQuestReward', {'questId': 'quest_18'}, -1),
        ('CSEquipStrengthen', {'guid': '0' * 32, 'times': 1,
                               'cost': [{'id': 'coin', 'num': 8}]}, 200),
    ]
    for name, fields, stamp in vectors:
        raw = transport.encode_original_http(name, fields, 1, b'0' * 16, stamp)
        request = parse_http(raw)
        if (request.opcode != BY_NAME[name] or request.timestamp != stamp
                or request.decode() != CLASSES[name](**fields)):
            raise AssertionError('Original story request failed roundtrip: ' + name)
    for name, fields in {
        'SCQuestReward': ['get'], 'SCPlayer': ['player'],
        'SCCopyPassedInfo': ['passedCopyInfo'], 'SCEquipStrengthen': ['equip', 'cost'],
    }.items():
        if name not in BY_NAME or any(field not in CLASSES[name].DESCRIPTOR.fields_by_name
                                      for field in fields):
            raise AssertionError('Original story response contract changed: ' + name)
    if BY_NAME['CSQuestReward'] != 24403:
        raise AssertionError('Original quest request opcode changed')
    quests = {row['questId']: row for row in original_rows('StoryQuest')}
    if quests['quest_18']['rewards'] != 'diamond,30;coin,50000;exp,130;equip_005,1':
        raise AssertionError('Original quest18 reward changed')
    if quests['quest_16']['npcId'] != 'area_22' or quests['quest_16']['type'] != '1':
        raise AssertionError('Original quest16 NPC area contract changed')
    if next(row['OpenId'] for row in original_rows('CopyMission')
            if row['MissionId'] == '12') != 'quest_16':
        raise AssertionError('Original chapter3 quest gate changed')
    quality = next(row['quality'] for row in original_rows('Equip') if row['id'] == 'equip_005')
    cost = int(next(row['coin'] for row in original_rows('EquipExp')
                    if row['quality'] == quality and row['level'] == '1'))
    if not 0 < cost < 50000:
        raise AssertionError('Original awarded equipment growth quote invalid')
    return {'originalRequestVectorsConstructed': len(vectors), 'questRequestOpcode': 24403,
            'originalQuest18Equipment': 'equip_005', 'originalEquipmentCoinQuote': cost,
            'originalQuest16Area': 'area_22', 'originalChapter3Gate': 'quest_16'}


def binary_preflight():
    if SOURCE_ROOT.parent != (ROOT / 'build/candidates').resolve() or not SOURCE_ROOT.is_dir():
        raise AssertionError('Set HYJS_P15_STORY_SOURCE_ROOT to the direct frozen N+2 candidate')
    if SOURCE_ROOT in REPORT.parents:
        raise AssertionError('Keep the protocol report outside the immutable source candidate')
    binary = Path(os.environ.get('HYJS_P15_STORY_BINARY', '')).expanduser()
    if not binary.is_file():
        raise AssertionError('Set HYJS_P15_STORY_BINARY to the existing qualified executable')
    proof = json.loads(VALIDATION.read_text())
    if not proof.get('qualified') or (ROOT / proof['candidate']).resolve() != SOURCE_ROOT:
        raise AssertionError('Story source does not match the completed qualification')
    required = ['hyjs-server/db/sql/187_story_quest_receipts.sql',
                'hyjs-server/db/storyQuestReceipt.go', 'hyjs-server/service/storyService.go',
                'hyjs-server/constUtil/storyQuestReceipt.go']
    manifest = json.loads((SOURCE_ROOT / 'manifest.json').read_text())
    paths = {entry['path'] for entry in manifest['files']}
    if not set(required).issubset(paths):
        raise AssertionError('Qualified source omits SQL187 story delivery')
    for name in ['server/codec.py', 'server/protocol/schema.json']:
        if name not in paths or hashlib.sha256((ROOT / name).read_bytes()).digest() != hashlib.sha256((SOURCE_ROOT / name).read_bytes()).digest():
            raise AssertionError('Runner original codec differs from frozen source: ' + name)
    # Select safe transport only. No inherited setup, root_sql, build or old allocator.
    transport.SOURCE_ROOT = SOURCE_ROOT
    transport.SOURCE = SOURCE_ROOT / 'manifest.json'
    transport.VALIDATION = VALIDATION
    transport.REPORT = REPORT
    previous = os.environ.get('HYJS_CONTINUED_GAMEPLAY_BINARY')
    os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = str(binary.resolve())
    try:
        return transport.binary_source_preflight()
    finally:
        if previous is None:
            os.environ.pop('HYJS_CONTINUED_GAMEPLAY_BINARY', None)
        else:
            os.environ['HYJS_CONTINUED_GAMEPLAY_BINARY'] = previous


@unittest.skipUnless(os.environ.get('HYJS_P15_STORY_MYSQL_TEST') == '1',
                     'Explicitly enable isolated original story HTTP/InnoDB')
class StoryQuestReceiptMySQLTests(unittest.TestCase):
    send = transport.ContinuedGameplayMySQLTests.send
    login = transport.ContinuedGameplayMySQLTests.login
    call = transport.ContinuedGameplayMySQLTests.call
    profile = transport.ContinuedGameplayMySQLTests.profile
    write_profile = transport.ContinuedGameplayMySQLTests.write_profile
    sql = transport.ContinuedGameplayMySQLTests.sql
    start = transport.ContinuedGameplayMySQLTests.start
    stop = transport.ContinuedGameplayMySQLTests.stop

    def request_frames(self, name, fields, login, stamp):
        raw = transport.encode_original_http(name, fields, login.playerId,
                                             bytes(login.sessionKey), stamp)
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=20)
        try:
            connection.request('POST', '/sao/', body=raw,
                               headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            data = response.read()
            self.assertEqual(response.status, 200)
            frames = decode_responses(data)
            self.assertTrue(frames, 'Original story response is empty')
            size = struct.unpack_from('>i', data)[0]
            return frames, data[:8 + size]
        finally:
            connection.close()

    def stamped(self, name, fields, login, stamp):
        return self.request_frames(name, fields, login, stamp)[0][0]

    def story_reward(self, login):
        frames, raw = self.request_frames('CSQuestReward', {'questId': 'quest_18'}, login, 100)
        self.assertEqual([opcode for opcode, _ in frames],
                         [BY_NAME['SCQuestReward'], BY_NAME['SCPlayer'], BY_NAME['SCCopyPassedInfo']])
        return frames[0][1], raw, frames[1][1].player

    def test_original_story_exact_concurrent_cold_and_earned_equipment_use(self):
        preflight = protocol_preflight()
        self.binary, binary_sha, source_sha = binary_preflight()
        if REPORT.exists():
            self.fail('Preserve prior story report; choose a new HYJS_P15_STORY_REPORT for retry')
        self.process = None
        self.app_temp = None
        self.pool = ManagedMySQL()
        began = time.monotonic()
        report = {'status': 'RUNNING', 'binarySHA256': binary_sha,
                  'sourceRoot': str(SOURCE_ROOT), 'sourceManifestSHA256': source_sha,
                  'binaryValidation': str(VALIDATION), 'preflight': preflight,
                  'runnerSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                  'phoneUI': False, 'productionModified': False,
                  'finitePrerequisites': 'one original created047 account; only level30, all original '
                  'StoryQuest rows ordered before quest18 and original chapter3/mission13 star3; '
                  'no prior natural battle/NPC claim; initial wallet/inventory/GUID/receipts unchanged'}
        lock = open('/tmp/hyjs-isolated-mysql.lock', 'a')
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            lock.close()
            self.fail('SQL_BUSY: no managed allocation, app or temporary directory started')
        try:
            connection = self.pool.allocate('p15_story_receipt')
            self.database = connection.database
            self.env = {key: value for key, value in os.environ.items()
                        if key in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
            self.env.update(connection.env())
            self.env['TZ'] = 'Asia/Shanghai'
            self.app_temp = tempfile.TemporaryDirectory(prefix='hyjs-p15-story-')
            self.start()
            uid = 'p15-story-' + secrets.token_hex(6)
            login = self.login(uid)
            self.call('CSPlayerCreate', {'playerName': '主线真链', 'heroId': 'hero_047'},
                      login, 1, 'SCPlayerCreate')
            player, game = self.profile(login)
            self.assertEqual((player.get('coin', 0), player.get('diamond', 0), player.get('exp', 0)),
                             (0, 0, 0))
            self.assertFalse(game.get('equipment'))
            self.assertFalse(game.get('items'))
            self.assertEqual(self.sql('SELECT COUNT(*) FROM story_quest_receipts;').strip(), '0')
            player['level'] = 30
            quests = original_rows('StoryQuest')
            order = int(next(row['order'] for row in quests if row['questId'] == 'quest_18'))
            game['quests'] = [row['questId'] for row in quests if int(row['order']) < order]
            game['copy']['passedCopyInfo'] = [{'chapterId': '3', 'missionId': '13', 'star': 3}]
            game['copy']['passedChapterInfo'] = []
            # The only profile write is this explicit prerequisite fixture before any award.
            self.write_profile(login, player, game)
            self.assertEqual(self.sql("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='story_quest_receipts';").strip(), 'InnoDB')
            for stamp in [0, -1]:
                self.assertEqual(self.stamped('CSQuestReward', {'questId': 'quest_18'},
                                              login, stamp)[0], 10001)
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                claims = list(workers.map(lambda _: self.story_reward(login), range(8)))
            award, first_raw, _ = claims[0]
            self.assertTrue(all(raw == first_raw and reply == award for reply, raw, _ in claims))
            self.assertEqual([(item.id, item.num) for item in getattr(award, 'get').simps],
                             [('diamond', 30), ('coin', 50000), ('exp', 130)])
            equipment = getattr(award, 'get').equips
            self.assertEqual(len(equipment), 1)
            gear = equipment[0]
            self.assertEqual((gear.equipId, gear.level), ('equip_005', 1))
            self.assertTrue(gear.guid)
            player, game = self.profile(login)
            self.assertEqual((player['coin'], player['diamond'], player['exp']), (50000, 30, 130))
            self.assertEqual(game['quests'].count('quest_18'), 1)
            self.assertEqual(len(game['equipment']), 1)
            self.assertEqual(game['equipment'][0]['guid'], gear.guid)
            receipt = self.sql('SELECT quest_id,HEX(request),HEX(response),expires_at-created_at '
                               'FROM story_quest_receipts WHERE player_id=' + str(login.playerId)
                               + ' AND stamp=100;').strip().split('\t')
            self.assertEqual(receipt[0], 'quest_18')
            self.assertEqual(bytes.fromhex(receipt[1]), CLASSES['CSQuestReward'](questId='quest_18').SerializeToString())
            self.assertEqual(bytes.fromhex(receipt[2]), first_raw[8:])
            self.assertEqual(int(receipt[3]), 24 * 60 * 60 * 1000)
            self.assertEqual(self.sql('SELECT COUNT(*) FROM story_quest_receipts;').strip(), '1')
            self.assertEqual(self.stamped('CSQuestReward', {'questId': 'quest_18'}, login, 101)[0], 10001)
            self.assertEqual(self.stamped('CSQuestReward', {'questId': 'quest_19'}, login, 100)[0], 10001)
            self.stop()
            self.start()
            login = self.login(uid)
            cold_award, cold_raw, current_player = self.story_reward(login)
            self.assertEqual(cold_raw, first_raw)
            self.assertEqual(cold_award, award)
            self.assertEqual((current_player.coin, current_player.diamond), (50000, 30))
            cost = preflight['originalEquipmentCoinQuote']
            growth = self.call('CSEquipStrengthen', {'guid': gear.guid, 'times': 1,
                                                    'cost': [{'id': 'coin', 'num': cost}]},
                               login, 200, 'SCEquipStrengthen')
            self.assertEqual((growth.equip.guid, growth.equip.level), (gear.guid, 2))
            self.assertEqual([(item.id, item.num) for item in growth.cost], [('coin', cost)])
            self.assertEqual(self.stamped('CSQuestReward', {'questId': 'quest_18'}, login, 100)[0], 10001)
            self.stop()
            self.start()
            login = self.login(uid)
            self.assertEqual(self.stamped('CSQuestReward', {'questId': 'quest_18'}, login, 100)[0], 10001)
            final, game = self.profile(login)
            self.assertEqual((final['coin'], final['diamond'], final['exp']), (50000 - cost, 30, 130))
            self.assertEqual(game['quests'].count('quest_18'), 1)
            self.assertEqual([(item['guid'], item['equipId'], item['level'])
                              for item in game['equipment']], [(gear.guid, 'equip_005', 2)])
            self.assertEqual(self.sql('SELECT COUNT(*) FROM story_quest_receipts;').strip(), '1')
            report.update(status='PASS_ORIGINAL_HTTP_INNODB_STORY_EXACT_RECOVERY_AND_EARNED_USE_PHONE_PENDING',
                          actualQuest='quest_18', concurrentSameStamp=8, coldRestarts=2,
                          originalReward={'coin': 50000, 'diamond': 30, 'exp': 130, 'equip_005': 1},
                          rewardFrameSHA256=hashlib.sha256(first_raw).hexdigest(),
                          actualEquipmentCoinUse=cost, finalCoin=50000 - cost,
                          receiptRows=1, differentStampRejected=True, changedQuestRejected=True,
                          zeroAndNegativeHTTPStampRejected=True, currentInstanceStaleFrameRejectedHotAndCold=True)
        except Exception as error:
            report['status'] = 'FAIL_PARTIAL_PREFIX_NOT_FULL_ACCEPTANCE'
            report['failure'] = self.pool.redact(str(error))[:2000]
            raise
        finally:
            try:
                self.stop()
            except Exception:
                report['status'] = 'FAIL_OWNED_APP_CLEANUP'
                raise
            finally:
                try:
                    self.pool.close()
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    report['ownedAppStopped'] = self.process is None or self.process.poll() is not None
                except Exception:
                    report['status'] = 'FAIL_OWNED_CLEANUP'
                    raise
                finally:
                    try:
                        if self.app_temp is not None:
                            self.app_temp.cleanup()
                            report['ownedLogDirectoryRemoved'] = True
                    finally:
                        fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
                        lock.close()
                        report['seconds'] = round(time.monotonic() - began, 3)
                        REPORT.parent.mkdir(parents=True, exist_ok=True)
                        with REPORT.open('x') as output:
                            output.write(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
