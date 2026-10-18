"""Resume the genuine paid checkpoint and original AI outcome on disposable MySQL.

The first winner is immutable original device evidence. Further source fights
wait for new root-owned automatic PVE results. Whole credential-free checkpoints
are saved at each boundary; no books or reported victory are synthesized.
"""
import concurrent.futures
import hashlib
import http.client
import json
import math
import os
from pathlib import Path
import shutil
import subprocess
import time
import unittest

from google.protobuf.json_format import MessageToDict, ParseDict
from server.codec import BY_NAME, decode_responses, encode_http_request, message
import test_copy_full_scope_mysql as common
import test_p5_talent_scope_mysql as shared


def atomic_json(path, value):
    temporary = path.with_suffix(path.suffix + '.tmp')
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
    temporary.replace(path)


@unittest.skipUnless(os.environ.get('HYJS_COPY_BOOK_RECOVERY_NATIVE_TEST') == '1',
                     'Explicit genuine paid recovery/original AI/isolated MySQL')
class CopyBookRecoveryNativeMySQLTests(common.OriginalCopyFullMySQLTests):
    __unittest_skip__ = False
    __unittest_skip_why__ = ''
    test_all_original_chapters_quests_rewards_books_specialty_and_cold_receipts = None

    @classmethod
    def setUpClass(cls):
        common.OriginalCopyFullMySQLTests.setUpClass.__func__(cls)
        # The runtime is the fixed joint candidate. Only this independent gated
        # recovery test is added to its frozen source for metadata preparation.
        relative = 'service/copyBookRecovery_test.go'
        shutil.copy2(shared.ROOT / 'hyjs-server' / relative, cls.frozen_source / relative)
        cls.source_shas[relative] = hashlib.sha256((cls.frozen_source / relative).read_bytes()).hexdigest()

    @classmethod
    def root_sql(cls, text):
        if not cls.database.startswith('hyjs_test_p5_talent_') or not cls.test_user.startswith('p5talent_'):
            raise AssertionError('Original paid recovery isolation guard')
        result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                                 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                                input=text, text=True, capture_output=True, timeout=60)
        if result.returncode:
            raise AssertionError('Original paid recovery isolated SQL failed')
        return result.stdout

    def stamped(self, name, fields, login, stamp):
        raw = encode_http_request(BY_NAME[name], message(name, **fields), player_id=login.playerId,
                                  zone_id=1, session=bytes(login.sessionKey), timestamp=stamp)
        # The prior stressed 20s failure stays in its immutable evidence folder.
        # This named isolated test permits 60s; the original frame is unchanged.
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=60)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            data = response.read()
            self.assertEqual(response.status, 200)
            return decode_responses(data)[0]
        finally:
            connection.close()

    def send(self, name, fields=None, login=None):
        args = {'zone_id': 1, 'timestamp': 1800002300000}
        if login is not None:
            args.update(player_id=login.playerId, session=bytes(login.sessionKey))
        raw = encode_http_request(BY_NAME[name], message(name, **(fields or {})), **args)
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=60)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            self.assertEqual(response.status, 200)
            self.last_frames = decode_responses(response.read())
            return self.last_frames[0]
        finally:
            connection.close()

    def recovered_login(self):
        # This fresh account was created by the gated Go helper, with test-only
        # authentication. Its numeric owner remains the actual original owner1.
        opcode, reply = self.send('CSUserLogin', {'platformUid': 'original-native-book-recovery',
                                  'authCode': 'isolated-test-only', 'clientTime': 1800002300000})
        self.assertEqual(opcode, BY_NAME['SCUserLogin'])
        self.assertEqual(reply.playerId, 1)
        return reply

    def checkpoint(self, folder, name, login, start=None):
        # One consistent read includes the full paid profile and every source
        # replay/drop row. Auth/session/hash credentials are excluded entirely.
        drop_columns = ['player_id', 'fight_id', 'task_id', 'input', 'input_hash']
        replay_columns = ['id', 'player_id', 'source', 'source_fight', 'status', 'seed',
                          'configuration_hash', 'engine_version', 'client_version', 'input',
                          'operates', 'outcome', 'input_hash', 'replay_hash', 'outcome_hash',
                          'client_result', 'server_result', 'authoritative', 'created_at', 'expires_at', 'finished_at']
        def object_sql(columns, binary):
            return 'JSON_OBJECT(' + ','.join("'" + key + "'," + ('HEX(' + key + ')' if key in binary else key)
                                               for key in columns) + ')'
        raw = self.sql('START TRANSACTION WITH CONSISTENT SNAPSHOT; '
            'SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id=1; '
            'SELECT ' + object_sql(drop_columns, {'input'}) + ' FROM copy_drop_snapshots WHERE player_id=1 ORDER BY fight_id; '
            'SELECT ' + object_sql(replay_columns, {'input', 'operates', 'outcome'}) + ' FROM p6_battle_records WHERE player_id=1 AND source="copy" ORDER BY id; COMMIT;').strip().splitlines()
        self.assertTrue(raw)
        player, game = [json.loads(bytes.fromhex(x)) for x in raw[0].split('|')]
        rows = [json.loads(line) for line in raw[1:]]
        value = {'schema': 'actual-paid-copy-state-with-frozen-source-metadata-v1',
                 'player': player, 'game': game, 'sourceMetadataRows': rows,
                 'productionModified': False, 'credentialsIncluded': False}
        if start is not None:
            value['originalStart'] = MessageToDict(start)
            value['originalStartSHA256'] = hashlib.sha256(start.SerializeToString(deterministic=True)).hexdigest()
        target = folder / name
        atomic_json(target, value)
        return {'file': name, 'sha256': hashlib.sha256(target.read_bytes()).hexdigest()}

    def original_finish(self, start, events, battle_type=1):
        inputs = [event for event in events if event.get('kind') == 'input']
        results = [event for event in events if event.get('kind') == 'result']
        self.assertEqual(len(inputs), 1)
        self.assertEqual(len(results), 1)
        self.assertEqual((inputs[0]['fightId'], inputs[0]['seed'], inputs[0]['battleType']),
                         (start.fightId, start.randSeed, battle_type))
        self.assertIn('automatic', inputs[0]['scope'])
        self.assertIs(type(results[0].get('attackerWon')), bool)
        self.assertEqual(events[-1]['kind'], 'result')
        final = next(event['state'] for event in reversed(events) if event.get('kind') == 'state')
        party = message('BattlePlayer'); party.CopyFrom(start.battlePlayer)
        native = {actor['uid']: actor for actor in final['attacker']['actors']}
        self.assertEqual(len(native), len(final['attacker']['actors']))
        self.assertEqual(set(native), {'0_' + actor.guid for actor in party.formation})
        for actor in party.formation:
            actual = native['0_' + actor.guid]
            self.assertEqual(actual['heroId'], actor.id)
            hp = actual['attribute']['hp']
            self.assertTrue(math.isfinite(hp)); self.assertGreaterEqual(hp, 0); self.assertLessEqual(hp, actor.prop.hp)
            actor.inst.curHp = hp
        defender_hp = [actor['attribute']['hp'] for actor in final['defender']['actors']]
        self.assertTrue(all(math.isfinite(hp) and hp >= 0 for hp in defender_hp))
        operate = message('BattleOperate', finalRoundNum=final['round'],
                          attackerTotalLeftHp=sum(actor.inst.curHp for actor in party.formation),
                          defenderTotalLeftHp=sum(defender_hp))
        for event in events:
            if event.get('kind') == 'skill_press':
                operate.skillOpts.add(roundNum=event['round'], roundTime=event['time'], camp=event['data']['camp'],
                                      actorUid=event['data']['actorUid'], skillIndex=event['data']['skillIndex'])
        return {'fightId': start.fightId, 'result': int(results[0]['attackerWon']),
                'battlePlayer': party, 'optRecord': operate}

    def test_original_paid_recovery_short_source_chain_to_specialty(self):
        folder = Path(os.environ.get('HYJS_COPY_BOOK_RECOVERY_EVIDENCE_FOLDER',
                                     shared.ROOT / 'analysis/p6/copy-book-recovery-native-20261014')).resolve()
        self.assertIn((shared.ROOT / 'analysis/p6').resolve(), folder.parents)
        folder.mkdir(parents=True, exist_ok=True)
        manifest_path = folder / 'manifest.json'
        self.assertFalse(manifest_path.exists(), 'Keep each actual recovery chain immutable')
        old = shared.ROOT / 'analysis/p6/copy-book-native-chain-20261014'
        continuation = os.environ.get('HYJS_COPY_BOOK_CONTINUATION_FILE')
        manifest = {'date': '2026-10-14', 'status': 'strict-recovery-preparing', 'cases': [],
                    'sourceSha256': self.source_shas, 'baseline': self.baseline_source,
                    'binarySHA256': hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),
                    'originalPaidSnapshotSHA256': 'a6a060ababa6a89103d397487567de228c7bb9a8a02cb27889fdece868de6bc0',
                    'productionModified': False, 'booksDirectlyPrepared': False, 'nativeCompared': False,
                    'authoritative': False, 'maximumNewSourceFights': 5,
                    'preparation': 'whole genuine000 Player/Game retained; actual34 fragment combines and14 paid talents; same original ID/seed/fullPB; new isolated auth only; no repeated paidprep; mission41/quest49 and first3 encounters are prior explicit prerequisites',
                    'privatePolicyBoundary': 'relative-weights-v1 SQL ordinaryDropOdds compatibility; private server probability and SpecialOdds unknown'}
        if continuation:
            manifest['actualContinuationCheckpoint'] = {'file': continuation, 'sha256': os.environ['HYJS_COPY_BOOK_CONTINUATION_SHA256']}
        manifest['originalTableSHA256'] = {path.name: hashlib.sha256(path.read_bytes()).hexdigest()
                                          for path in sorted((shared.ROOT / 'analysis/tables').glob('*.tsv'))}
        atomic_json(manifest_path, manifest)
        began = time.monotonic()
        try:
            env = dict(self.env, HYJS_COPY_BOOK_RECOVERY_CAPTURE_MYSQL='1')
            prepared = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestCopyOriginalPaidStateRecovery$',
                                        '-count=1', '-v', '-timeout=90s'], cwd=self.frozen_source,
                                       env=env, text=True, capture_output=True, timeout=120)
            (folder / 'strict-recovery-go.log').write_text(prepared.stdout + prepared.stderr)
            self.assertEqual(prepared.returncode, 0, 'Strict isolated whole paid state/original wire recovery failed')
            self.start(); login = self.recovered_login(); stamp = 1800002300000
            def call(name, fields, response):
                nonlocal stamp
                stamp += 1
                return self.call(name, fields, login, stamp, response)
            player, game = self.profile(login)
            restored_start = None
            if not continuation:
                self.assertFalse(any(row['id'] in ('item_439', 'item_440') and row['num'] for row in game['items']))
                restored_start = ParseDict(game['run']['fight']['start'], message('SCCopyFightStart'))
                self.assertEqual(call('CSCopyFightStart', {'chapterId': '8', 'missionId': '42', 'taskId': '168'}, 'SCCopyFightStart'), restored_start)
            for index in range(5 if continuation else 6):
                player, game = self.profile(login); books = {row['id']: row['num'] for row in game['items']}
                if books.get('item_439', 0) >= 20 and books.get('item_440', 0) >= 15:
                    break
                reuse_original = index == 0 and not continuation
                if reuse_original:
                    start = restored_start
                else:
                    gate = os.environ.get('HYJS_COPY_BOOK_NEW_FIGHTS_GATE')
                    if gate:
                        manifest['status'] = 'settled-checkpoint-awaiting-root-native-ready'
                        atomic_json(manifest_path, manifest)
                        gate_deadline = time.monotonic() + 900
                        while not Path(gate).exists():
                            self.assertLess(time.monotonic(), gate_deadline, 'Root-owned phone automatic AI readiness required before starting a new expiring fight')
                            time.sleep(.2)
                    call('CSCopyEnter', {'chapterId': '8', 'missionId': '42'}, 'SCCopyEnter')
                    # The already declared first3 encounters remain prerequisites
                    # for this boss-only source audit; their victory is not claimed.
                    player, game = self.profile(login); game['run']['passed'] = ['165', '166', '167']
                    self.write_profile(login, player, game)
                    start = call('CSCopyFightStart', {'chapterId': '8', 'missionId': '42', 'taskId': '168'}, 'SCCopyFightStart')
                self.assertEqual(len(start.battleMonsterList), 1)
                wire = message('SCFightPlayback', fightid=start.fightId, randSeed=start.randSeed, battleType=1,
                               attacker=start.battlePlayer, defender2=start.battleMonsterList[0], maxRound=start.maxRound,
                               attackerFactor=1, defenderFactor=1)
                raw = wire.SerializeToString(deterministic=True)
                wire_name = f'{index:03d}.pb'; (folder / wire_name).write_bytes(raw)
                if reuse_original:
                    self.assertEqual(raw, (old / '000.pb').read_bytes())
                    self.assertEqual(hashlib.sha256((old / '000.native.jsonl').read_bytes()).hexdigest(),
                                     '9fe70f2ded52e477429e03bfdd75b36c9ba4513d1e8b5cc3d3ab3d5b77bbaa02')
                    shutil.copy2(old / '000.native.jsonl', folder / '000.native.jsonl')
                case = {'wire': wire_name, 'wireSHA256': hashlib.sha256(raw).hexdigest(), 'fightId': start.fightId,
                        'seed': start.randSeed, 'battleType': 1, 'scope': 'actual-original-mission42-task168-specialty-books',
                        'nativeResult': f'{index:03d}.native.jsonl', 'status': 'awaiting-original-automatic-ai',
                        'genuinePriorOriginalResultReused': reuse_original}
                case['startCheckpoint'] = self.checkpoint(folder, f'{index:03d}-start-paid-state.json', login, start)
                manifest['cases'].append(case); manifest['status'] = case['status']; atomic_json(manifest_path, manifest)
                result_path = folder / case['nativeResult']; deadline = time.monotonic() + 300
                while True:
                    try:
                        events = [json.loads(line) for line in result_path.read_text().splitlines() if line]
                    except (OSError, ValueError):
                        events = []
                    if events and events[-1].get('kind') == 'result':
                        break
                    self.assertLess(time.monotonic(), deadline, 'Original root-owned automatic AI result still required')
                    time.sleep(.2)
                finish = self.original_finish(start, events)
                case['originalResultSHA256'] = hashlib.sha256(result_path.read_bytes()).hexdigest()
                case['originalOperationSHA256'] = hashlib.sha256(finish['optRecord'].SerializeToString(deterministic=True)).hexdigest()
                case['attackerWon'] = bool(finish['result']); case['status'] = 'original-native-outcome-verified'
                atomic_json(manifest_path, manifest)
                before_books = books
                stamp += 1
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                    replies = list(pool.map(lambda _: self.call('CSCopyFightEnd', finish, login, stamp, 'SCCopyFightEnd'), range(8)))
                self.assertTrue(all(reply == replies[0] for reply in replies))
                case['endCheckpoint'] = self.checkpoint(folder, f'{index:03d}-end-paid-state.json', login, start)
                case.update(status='original-native-settled', actualGain=MessageToDict(replies[0].gain))
                player, game = self.profile(login); current = {row['id']: row['num'] for row in game['items']}
                actual = {row.id: row.num for row in replies[0].gain.simps}
                for key in ('item_439', 'item_440'):
                    self.assertEqual(current.get(key, 0) - before_books.get(key, 0), actual.get(key, 0))
                manifest['actualBooks'] = {key: current.get(key, 0) for key in ('item_439', 'item_440')}
                atomic_json(manifest_path, manifest)
                self.stop(); self.start(); login = self.recovered_login()
                self.assertEqual(self.call('CSCopyFightEnd', finish, login, stamp, 'SCCopyFightEnd'), replies[0])
                case['coldIdentical'] = True; atomic_json(manifest_path, manifest)
                self.assertEqual(finish['result'], 1, 'Original source actor lost; no material victory invented')
            player, game = self.profile(login); books = {row['id']: row['num'] for row in game['items']}
            if books.get('item_439', 0) < 20 or books.get('item_440', 0) < 15:
                manifest['status'] = 'bounded-source-batch-insufficient-real-random-books'
                manifest['continuationCheckpoint'] = self.checkpoint(folder, 'continuation-paid-state.json', login)
                manifest['seconds'] = round(time.monotonic() - began, 3); atomic_json(manifest_path, manifest)
                self.fail('Preserved complete actual state after5 new wins; real random materials remain insufficient')
            zero = call('CSCombine', {'targetId': 'hero_002'}, 'SCCombine').get.wars[0]
            player, game = self.profile(login); target = next(row for row in game['warriors'] if row['guid'] == zero.guid)
            self.assertEqual(target.get('skillRefine', []), []); target['level'] = 75
            self.write_profile(login, player, game)
            call('CSFormationOn', {'formatPos': 3, 'id': zero.id, 'guid': zero.guid}, 'SCFormationOn')
            before = call('CSCopyRunMapInfo', {'chapterId': '8', 'missionId': '42'}, 'SCCopyRunMapInfo')
            before_actor = next(actor for actor in before.battlePlayer.formation if actor.guid == zero.guid)
            manifest['beforeSpecialtyCheckpoint'] = self.checkpoint(folder, 'specialty-before-paid-state.json', login)
            fields = {'roleGuid': zero.guid, 'type': 2}; stamp += 1; paid_stamp = stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                replies = list(pool.map(lambda _: self.call('CSUpRoleSkillRefineLevel', fields, login, paid_stamp, 'SCUpRoleSkillRefineLevel'), range(8)))
            self.assertTrue(all(reply == replies[0] for reply in replies))
            self.assertEqual([(row.id, row.num) for row in replies[0].cost], [('item_439', 20), ('item_440', 15)])
            after = call('CSCopyRunMapInfo', {'chapterId': '8', 'missionId': '42'}, 'SCCopyRunMapInfo')
            after_actor = next(actor for actor in after.battlePlayer.formation if actor.guid == zero.guid)
            self.assertEqual(after_actor.prop.mdef - before_actor.prop.mdef, 18)
            player, game = self.profile(login); final_books = {row['id']: row['num'] for row in game['items']}
            self.assertEqual((books['item_439'] - final_books['item_439'], books['item_440'] - final_books['item_440']), (20, 15))
            manifest['afterSpecialtyCheckpoint'] = self.checkpoint(folder, 'specialty-after-paid-state.json', login)
            self.stop(); self.start(); login = self.recovered_login()
            self.assertEqual(self.call('CSUpRoleSkillRefineLevel', fields, login, paid_stamp, 'SCUpRoleSkillRefineLevel'), replies[0])
            manifest.update(status='complete', originalNativeSourceWinsVerified=True,
                            naturalBookConsumption={'item_439': 20, 'item_440': 15, 'mdefAdd': 18,
                                'eightConcurrentAndCold': 'one actual consumption and identical original response'},
                            seconds=round(time.monotonic() - began, 3))
            atomic_json(manifest_path, manifest)
        except Exception as error:
            if manifest['status'] != 'bounded-source-batch-insufficient-real-random-books':
                manifest['status'] = 'incomplete-' + type(error).__name__
            manifest['seconds'] = round(time.monotonic() - began, 3)
            if self.process is not None and self.process.poll() is None:
                try:
                    login = self.recovered_login()
                    manifest['failureCheckpoint'] = self.checkpoint(folder, 'failure-paid-state.json', login)
                except Exception as checkpoint_error:
                    manifest['failureCheckpointUnavailable'] = type(checkpoint_error).__name__
            atomic_json(manifest_path, manifest)
            raise


if __name__ == '__main__':
    unittest.main()
