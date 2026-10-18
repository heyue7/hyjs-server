"""Actual current-day PetEquip49/50/51 source on the genuine paid party.

The original date/three shared attempts remain intact. Original automatic-AI
device outcomes settle their frozen real rewards; target379..390 are never
prepared. A finite non-source pet/374 prerequisite is declared separately.
"""
import concurrent.futures
import datetime
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
import unittest

from google.protobuf.json_format import MessageToDict
from server.codec import message
from test_copy_book_recovery_native_mysql import CopyBookRecoveryNativeMySQLTests, atomic_json
import test_p5_talent_scope_mysql as shared


@unittest.skipUnless(os.environ.get('HYJS_COPY_PET_DAILY_NATIVE_TEST') == '1', 'Explicit actual day/original device/isolated daily material source')
class CopyPetDailyNativeMySQLTests(CopyBookRecoveryNativeMySQLTests):
    __unittest_skip__ = False
    __unittest_skip_why__ = ''
    test_original_paid_recovery_short_source_chain_to_specialty = None

    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        for relative in ('service/dailyCopyService.go', 'service/dailyCopyActorIdentity_test.go', 'service/copyDailyPetPrerequisite_test.go'):
            shutil.copy2(shared.ROOT / 'hyjs-server' / relative, cls.frozen_source / relative)
            cls.source_shas[relative] = hashlib.sha256((cls.frozen_source / relative).read_bytes()).hexdigest()
        rebuilt = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', cls.binary, '.'], cwd=cls.frozen_source,
                                 capture_output=True, timeout=180)
        if rebuilt.returncode:
            raise AssertionError('Fixed natural daily candidate build failed')

    def daily_checkpoint(self, folder, filename, login):
        saved = self.checkpoint(folder, filename, login)
        path = folder / filename; value = json.loads(path.read_text())
        layout = {
            'daily_copy_states': ['player_id', 'day', 'type', 'used', 'limit'],
            'daily_copy_fights': ['fight_id', 'player_id', 'start_stamp', 'day', 'type', 'chapter_id', 'stamina', 'win_stars', 'reward', 'start_snapshot', 'settled', 'end_request', 'response'],
            'daily_copy_start_receipts': ['player_id', 'stamp', 'chapter_id', 'fight_id'],
        }
        frozen = {}
        for table, columns in layout.items():
            fields = ','.join("'" + key + "'," + ('HEX(' + key + ')' if key in ('start_snapshot', 'end_request', 'response') else '`' + key + '`') for key in columns)
            raw = self.sql('SELECT JSON_OBJECT(' + fields + ') FROM ' + table + ' WHERE player_id=1;')
            frozen[table] = [json.loads(line) for line in raw.splitlines() if line]
        value['dailyMetadata'] = frozen; value['dailyQuotaNotReset'] = True
        atomic_json(path, value); saved['sha256'] = hashlib.sha256(path.read_bytes()).hexdigest()
        return saved

    def test_current_day_original_daily_material_source_and_equipment_consumption(self):
        folder = Path(os.environ.get('HYJS_COPY_PET_DAILY_NATIVE_FOLDER', shared.ROOT / 'analysis/p6/copy-pet-daily-native-20261014')).resolve()
        self.assertIn((shared.ROOT / 'analysis/p6').resolve(), folder.parents, 'Keep source evidence inside the isolated analysis directory')
        folder.mkdir(parents=True, exist_ok=True)
        manifest_path = folder / 'manifest.json'; self.assertFalse(manifest_path.exists(), 'Keep original source evidence immutable')
        prior = shared.ROOT / 'analysis/p6/copy-book-recovery-native-20261014'
        prior_manifest = json.loads((prior / 'manifest.json').read_text())
        checkpoint = prior_manifest['afterSpecialtyCheckpoint']
        source = prior / checkpoint['file']
        continuation = os.environ.get('HYJS_COPY_PET_DAILY_CONTINUATION_FILE')
        prior_pending = os.environ.get('HYJS_COPY_PET_DAILY_PENDING_FOLDER')
        if continuation:
            source = Path(continuation)
            checkpoint = {'file': str(source), 'sha256': os.environ['HYJS_COPY_PET_DAILY_CONTINUATION_SHA256']}
        # Derive the continuation from actual stored quota and pending rows.
        # Completed original outcomes stay committed; a pending start is reused.
        frozen = json.loads(source.read_text())
        self.assertEqual(hashlib.sha256(source.read_bytes()).hexdigest(), checkpoint['sha256'])
        daily = frozen.get('dailyMetadata', {})
        quotas = [row for row in daily.get('daily_copy_states', []) if row['type'] == 'PetEquip']
        pending = [row for row in daily.get('daily_copy_fights', []) if row['type'] == 'PetEquip' and not row['settled']]
        self.assertLessEqual(len(pending), 1)
        used = quotas[0]['used'] if quotas else 0
        self.assertEqual(len(quotas), 1 if continuation else 0)
        self.assertEqual(used, len([row for row in daily.get('daily_copy_fights', []) if row['type'] == 'PetEquip']))
        first_index = used - 1 if pending else used
        self.assertGreaterEqual(first_index, 0); self.assertLess(first_index, 3)
        native_wait = int(os.environ.get('HYJS_COPY_PET_DAILY_NATIVE_WAIT_SECONDS', '600'))
        self.assertGreaterEqual(native_wait, 600); self.assertLessEqual(native_wait, 1800)
        local = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)))
        manifest = {'date': local.isoformat(), 'scope': 'original-pve-automatic-ai', 'cases': [], 'status': 'recovering-actual-paid-party',
                    'productionModified': False, 'sourceSha256': self.source_shas, 'baseline': self.baseline_source,
                    'originalPaidCheckpoint': {'file': str(source.relative_to(shared.ROOT)), 'sha256': checkpoint['sha256']},
                    'originalDay': local.date().isoformat(), 'originalWeekday': local.isoweekday(), 'originalDailyPetEquipCount': 3,
                    'targetMaterialsPrepared': False, 'nativeCompared': False, 'authoritative': False,
                    'boundary': 'original actual current date/quota; genuine paid party reused; native automatic winner/HP/operations; weighted original drops frozen at start; no whole native timeline differential',
                    'nonSourcePrerequisiteBoundary': 'only after source battles, finite original pet047 factory instance, actual level10/strength10 via paid original APIs and item3748; these prerequisites do not certify their natural sources'}
        if continuation and first_index:
            self.assertTrue(prior_pending, 'Actual earlier source evidence required for committed outcomes')
            prior_folder = Path(prior_pending).resolve()
            self.assertIn((shared.ROOT / 'analysis/p6').resolve(), prior_folder.parents)
            old = json.loads((prior_folder / 'manifest.json').read_text())
            for index in range(first_index):
                case = old['cases'][index]
                self.assertEqual(case['status'], 'original-native-settled')
                self.assertTrue(case['coldIdentical'])
                stored = next(row for row in daily['daily_copy_fights'] if row['fight_id'] == case['fightId'])
                self.assertTrue(stored['settled'])
                for filename, digest in ((case['wire'], case['wireSHA256']), (case['nativeResult'], case['originalResultSHA256']),
                                         (case['startCheckpoint']['file'], case['startCheckpoint']['sha256']),
                                         (case['endCheckpoint']['file'], case['endCheckpoint']['sha256'])):
                    original = prior_folder / filename
                    self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(), digest)
                    shutil.copy2(original, folder / filename)
                manifest['cases'].append(dict(case, actualPreviouslyCommittedPreserved=True))
        manifest['nativeWaitSeconds'] = native_wait
        atomic_json(manifest_path, manifest); began = time.monotonic()
        try:
            env = dict(self.env, HYJS_COPY_BOOK_RECOVERY_CAPTURE_MYSQL='1',
                       HYJS_COPY_BOOK_CONTINUATION_FILE=str(source), HYJS_COPY_BOOK_CONTINUATION_SHA256=checkpoint['sha256'])
            prepared = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestCopyOriginalPaidStateRecovery$',
                                      '-count=1', '-v', '-timeout=90s'], cwd=self.frozen_source, env=env, text=True, capture_output=True, timeout=120)
            (folder / 'strict-paid-recovery.log').write_text(prepared.stdout + prepared.stderr)
            self.assertEqual(prepared.returncode, 0, 'Exact genuine paid party/source metadata recovery failed')
            self.start(); login = self.recovered_login()
            stamp = max([1800002500000] + [row['stamp'] for row in daily.get('daily_copy_start_receipts', [])])
            def call(name, fields, response):
                nonlocal stamp
                stamp += 1
                return self.call(name, fields, login, stamp, response)
            current = call('CSDailyCopyState', {}, 'SCDailyCopyState')
            pet_type = next(item for item in current.items if item.type == 'PetEquip')
            self.assertEqual(pet_type.lastTime, 3 - used)
            if pet_type.isOpen != 1:
                manifest['status'] = 'original-current-day-closed-no-consumption'; atomic_json(manifest_path, manifest)
                self.skipTest('Original PetEquip weekday gate remains closed today')
            self.assertEqual([item.ChapterId for item in pet_type.dailyCopyState], ['49', '50', '51'])
            if pending:
                # The original single pending fight exposes only its chapter.
                # Other difficulties reopen after the actual pending outcome.
                self.assertEqual([item.ChapterId for item in pet_type.dailyCopyState if item.isOpen == 1], [pending[0]['chapter_id']])
            else:
                self.assertTrue(all(item.isOpen == 1 for item in pet_type.dailyCopyState))
            player, game = self.profile(login)
            expected_items = {row['id']: row['num'] for row in frozen['game']['items'] if row['id'] in {f'item_{i}' for i in range(379, 391)}}
            self.assertEqual({row['id']: row['num'] for row in game['items'] if row['id'] in {f'item_{i}' for i in range(379, 391)}}, expected_items)
            if not continuation:
                self.assertFalse(any(expected_items.values()))
            manifest['actualSourceItems'] = expected_items
            gate = os.environ.get('HYJS_COPY_PET_DAILY_NATIVE_GATE')
            if gate:
                manifest['status'] = 'actual-paid-party-awaiting-root-native-ready'
                manifest['initialPaidCheckpoint'] = self.daily_checkpoint(folder, 'initial-paid-state.json', login)
                atomic_json(manifest_path, manifest); deadline = time.monotonic() + 900
                while not Path(gate).exists():
                    self.assertLess(time.monotonic(), deadline, 'Root original automatic PVE selector readiness required before first expiring daily battle')
                    time.sleep(.2)
            for index in range(first_index, 3):
                chapter = '49' if index == 0 else '50' if index == 1 else '51' if manifest['cases'][1]['attackerWon'] else '49'
                if index == first_index and pending:
                    self.assertEqual(chapter, pending[0]['chapter_id'])
                if index == 2 and chapter == '49':
                    manifest['chapter51Boundary'] = 'actual original50 loss retained; original final shared quota uses49, high51 remains pending stronger actual cultivation'
                player, game = self.profile(login); before_items = {row['id']: row['num'] for row in game['items']}
                start = call('CSDailyCopyFightStart', {'ChapterId': chapter}, 'SCDailyCopyFightStart')
                wire = message('SCFightPlayback', fightid=start.fightId, randSeed=start.randSeed, battleType=4,
                               attacker=start.battlePlayer, defender2=start.monster, maxRound=start.maxRoundNum,
                               attackerFactor=1, defenderFactor=1)
                raw = wire.SerializeToString(deterministic=True); filename = f'{index:03d}.pb'; (folder / filename).write_bytes(raw)
                if index == first_index and pending:
                    self.assertTrue(prior_pending, 'Exact saved pending wire folder required')
                    previous = message('SCFightPlayback'); previous.ParseFromString((Path(prior_pending) / filename).read_bytes())
                    # Correct only the erroneous exported playback type. The
                    # actual business start/seed/party/monster/quota stay exact.
                    previous.battleType = 4
                    self.assertEqual(wire, previous, 'Pending daily identity/seed/every original field changed')
                    self.assertEqual(self.sql('SELECT used FROM daily_copy_states WHERE player_id=1 AND type="PetEquip" AND day="' + manifest['originalDay'] + '";').strip(), str(used))
                case = {'wire': filename, 'wireSHA256': hashlib.sha256(raw).hexdigest(), 'fightId': start.fightId,
                        'seed': start.randSeed, 'battleType': 4, 'scope': 'original-pve-automatic-ai',
                        'chapterId': chapter, 'nativeResult': f'{index:03d}.native.jsonl', 'status': 'awaiting-original-automatic-ai'}
                if index == first_index and pending:
                    case['actualPriorPendingReusedWithoutExtraQuota'] = True
                case['startCheckpoint'] = self.daily_checkpoint(folder, f'{index:03d}-start-paid-state.json', login)
                manifest['cases'].append(case); manifest['status'] = case['status']; atomic_json(manifest_path, manifest)
                result_path = folder / case['nativeResult']; deadline = time.monotonic() + native_wait
                while True:
                    try: events = [json.loads(line) for line in result_path.read_text().splitlines() if line]
                    except (OSError, ValueError): events = []
                    if events and events[-1].get('kind') == 'result': break
                    self.assertLess(time.monotonic(), deadline, 'Root original PVE automatic AI result still required')
                    time.sleep(.2)
                original = self.original_finish(start, events, battle_type=4)
                finish = {'ChapterId': chapter, 'isWin': original['result'], 'fightId': start.fightId, 'optRecord': original['optRecord']}
                case['attackerWon'] = bool(original['result']); case['originalResultSHA256'] = hashlib.sha256(result_path.read_bytes()).hexdigest()
                case['originalOperationSHA256'] = hashlib.sha256(original['optRecord'].SerializeToString(deterministic=True)).hexdigest()
                case['status'] = 'original-native-outcome-verified'; atomic_json(manifest_path, manifest)
                stamp += 1
                with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                    replies = list(pool.map(lambda _: self.call('CSDailyCopyFightEnd', finish, login, stamp, 'SCDailyCopyFightEnd'), range(8)))
                self.assertTrue(all(reply == replies[0] for reply in replies)); self.assertEqual(replies[0].lastTime, 2 - index)
                player, game = self.profile(login); items = {row['id']: row['num'] for row in game['items']}
                actual = {row.id: row.num for row in replies[0].get.simps}
                for key in (f'item_{i}' for i in range(379, 391)):
                    self.assertEqual(items.get(key, 0) - before_items.get(key, 0), actual.get(key, 0))
                case.update(status='original-native-settled', actualGain=MessageToDict(replies[0].get))
                case['endCheckpoint'] = self.daily_checkpoint(folder, f'{index:03d}-end-paid-state.json', login)
                manifest['actualSourceItems'] = {key: value for key, value in items.items() if key in {f'item_{i}' for i in range(379, 391)}}
                atomic_json(manifest_path, manifest)
                self.stop(); self.start(); login = self.recovered_login()
                self.assertEqual(self.call('CSDailyCopyFightEnd', finish, login, stamp, 'SCDailyCopyFightEnd'), replies[0])
                case['coldIdentical'] = True; atomic_json(manifest_path, manifest)
                if not original['result']:
                    self.assertFalse(replies[0].get.ByteSize(), 'Original loss must not award natural source items')
            before = self.profile(login); stamp += 1
            self.assertEqual(self.stamped('CSDailyCopyFightStart', {'ChapterId': '49'}, login, stamp)[0], 10001)
            self.assertEqual(self.profile(login), before)
            manifest['sharedQuotaExhaustedOriginalErrorNoMutation'] = True
            player, game = before; items = {row['id']: row['num'] for row in game['items']}
            if items.get('item_379', 0) < 12 or items.get('item_384', 0) < 5:
                manifest['status'] = 'real-original-source-three-attempts-quota-exhausted-consumption-material-insufficient'
                manifest['actualSourceContinuationCheckpoint'] = self.daily_checkpoint(folder, 'actual-quota-exhausted-paid-state.json', login)
                manifest['seconds'] = round(time.monotonic() - began, 3); atomic_json(manifest_path, manifest)
                return
            self.stop()
            prep = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestCopyDailyPetPrerequisite$', '-count=1', '-v', '-timeout=90s'],
                                  cwd=self.frozen_source, env=dict(self.env, HYJS_COPY_DAILY_PET_PREREQUISITE='1'), text=True, capture_output=True, timeout=120)
            (folder / 'finite-nonsource-pet-prerequisite.log').write_text(prep.stdout + prep.stderr)
            self.assertEqual(prep.returncode, 0, 'Finite original non-source pet prerequisite failed')
            self.start(); login = self.recovered_login()
            pets = call('CSPetList', {}, 'SCPetList').petList
            self.assertEqual(len(pets), 1); pet = pets[0]; self.assertEqual(pet.id, 'pet_047')
            call('CSFormationOn', {'id': pet.id, 'guid': pet.guid, 'formatPos': 1}, 'SCFormationOn')
            # A completed mainline map provides a current real guard projection.
            before_actor = call('CSCopyRunMapInfo', {'chapterId': '8', 'missionId': '42'}, 'SCCopyRunMapInfo').battlePlayer.formation[0]
            fields = {'petGuid': pet.guid, 'equipId': 'petequip_002'}; stamp += 1; paid_stamp = stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                advanced = list(pool.map(lambda _: self.call('CSPetEquipAdvance', fields, login, paid_stamp, 'SCPetEquipAdvance'), range(8)))
            self.assertTrue(all(reply == advanced[0] for reply in advanced)); self.assertEqual(advanced[0].petEquip.advanceLevel, 1)
            after_actor = call('CSCopyRunMapInfo', {'chapterId': '8', 'missionId': '42'}, 'SCCopyRunMapInfo').battlePlayer.formation[0]
            self.assertEqual(after_actor.prop.pdef - before_actor.prop.pdef, 33); self.assertEqual(after_actor.prop.mdef - before_actor.prop.mdef, 33)
            player, game = self.profile(login); final = {row['id']: row['num'] for row in game['items']}
            self.assertEqual((items['item_379'] - final['item_379'], items['item_384'] - final['item_384']), (12, 5))
            self.stop(); self.start(); login = self.recovered_login()
            self.assertEqual(self.call('CSPetEquipAdvance', fields, login, paid_stamp, 'SCPetEquipAdvance'), advanced[0])
            manifest.update(status='complete-source-and-actual-consumption', naturalSourceCost={'item_379': 12, 'item_384': 5},
                            finiteNonSourceCost={'item_374': 8, 'coin': 50000}, actualGuardEffect={'pdef': 33, 'mdef': 33},
                            eightConcurrentAndCold='one actual advance and identical original response', seconds=round(time.monotonic() - began, 3))
            atomic_json(manifest_path, manifest)
        except Exception as error:
            manifest['status'] = 'incomplete-' + type(error).__name__; manifest['seconds'] = round(time.monotonic() - began, 3)
            if self.process is not None and self.process.poll() is None:
                try: manifest['failureCheckpoint'] = self.daily_checkpoint(folder, 'failure-paid-state.json', self.recovered_login())
                except Exception as failure: manifest['failureCheckpointUnavailable'] = type(failure).__name__
            atomic_json(manifest_path, manifest)
            raise


if __name__ == '__main__': unittest.main()
