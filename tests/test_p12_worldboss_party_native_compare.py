"""Offline provenance/completion boundaries; synthetic events never grant native proof."""
import base64
import copy
import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.native_batch import compare_worldboss_party as compare
from server.codec import message


class WorldBossPartyComparisonPreparationTests(unittest.TestCase):
    def test_pinned_source_is_two_unrecorded_full_four_body_typed18_diagnostics(self):
        source = compare.preflight()
        self.assertEqual(len(source['cases']), 2)
        self.assertFalse(source['actualBusinessStart'])
        for case in source['cases']:
            wire = message('SCFightPlayback')
            wire.ParseFromString((compare.SOURCE.parent / case['wire']).read_bytes())
            self.assertFalse(wire.HasField('optRecord'))
            self.assertEqual(wire.battleType, 18)
            self.assertEqual(len(wire.defender2.formation), 4)
            self.assertEqual([actor.bloodNumber for actor in wire.defender2.formation], [2, 1, 1, 1])

    def make_capture(self, output):
        source = compare.preflight()
        rows = []
        for index, case in enumerate(source['cases']):
            stem = f'{index:03d}'
            first = {'kind': 'input', 'scope': compare.SCOPE, 'fightId': case['fightId'],
                     'seed': case['seed'], 'battleType': 18,
                     'typedCallbackSHA256': case['typedCallbackSHA256']}
            # Boundary examples only: these events never enter a comparison fixture.
            events = [first, {'kind': 'original_final_operates', 'data': {}},
                      {'kind': 'result', 'attackerWon': True}]
            native = output / (stem + '.native.jsonl')
            native.write_text(''.join(json.dumps(event) + '\n' for event in events))
            rows.append({**case, 'batchWire': stem + '.pb', 'nativeResult': native.name,
                         'nativeSHA256': compare.sha(native),
                         'batchTypedCallback': stem + '.worldboss-start.bin'})
        capture = {'sourceManifest': str(compare.SOURCE), 'sourceManifestSHA256': compare.SOURCE_SHA,
                   'selector': 'highest', 'cases': rows}
        path = output / 'manifest.json'
        path.write_text(json.dumps(capture))
        return source, capture, path

    def test_capture_rejects_missing_case_changed_typed_seed_scope_and_hash(self):
        with tempfile.TemporaryDirectory() as directory:
            source, captured, path = self.make_capture(Path(directory))
            compare.validate_capture(path, source)
            for alter in ('missing-case', 'changed-typed', 'changed-seed', 'changed-scope', 'changed-native-sha'):
                modified = copy.deepcopy(captured)
                if alter == 'missing-case':
                    modified['cases'].pop()
                elif alter == 'changed-typed':
                    modified['cases'][0]['typedCallbackSHA256'] = 'a' * 64
                elif alter == 'changed-seed':
                    modified['cases'][0]['seed'] += 1
                elif alter == 'changed-scope':
                    modified['cases'][0]['nativeScope'] = 'original-pvp-automatic-ai'
                else:
                    modified['cases'][0]['nativeSHA256'] = 'a' * 64
                path.write_text(json.dumps(modified))
                with self.subTest(alter=alter), self.assertRaises(RuntimeError):
                    compare.validate_capture(path, source)

    def test_truncated_capture_rejected_even_with_updated_raw_hash(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            source, captured, path = self.make_capture(output)
            native = output / captured['cases'][0]['nativeResult']
            native.write_text(native.read_text().splitlines()[0] + '\n')
            captured['cases'][0]['nativeSHA256'] = compare.sha(native)
            path.write_text(json.dumps(captured))
            with self.assertRaises(RuntimeError):
                compare.validate_capture(path, source)

    def record_rows(self):
        source = compare.preflight()
        original = message('BattleOperate', finalRoundNum=2, attackerTotalLeftHp=700,
                           defenderTotalLeftHp=0).SerializeToString(deterministic=True)
        rows = []
        for case in source['cases']:
            typed = message('SCWorldBossFight')
            typed.ParseFromString((compare.SOURCE.parent / case['typedCallback']).read_bytes())
            actors = [(0, actor) for actor in typed.battlePlayer.formation]
            actors += [(1, actor) for actor in typed.battleMonsterList[0].formation]
            terminal = [{'camp': camp, 'heroId': actor.id, 'uid': str(index + 1), 'state': 0,
                         'maxHp': actor.prop.hp, 'currentHp': actor.inst.curHp - (0.4 if camp else 0)}
                        for index, (camp, actor) in enumerate(actors)]
            rows.append({'wireSHA256': case['wireSHA256'], 'attackerWon': False,
                         'originalFinalActorHP': terminal,
                         'originalFinalRecord': {'protobufBase64': base64.b64encode(original).decode(),
                                                 'protobufSHA256': hashlib.sha256(original).hexdigest()}})
        return source, original, rows

    def test_original_bytes_preserved_no_business_end_and_floor_once_without_blood(self):
        source, original, rows = self.record_rows()
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            path = output / 'records.json'
            path.write_text(json.dumps({'cases': rows}))
            artifacts = compare.preserve_records(path, output, source)
            for artifact in artifacts:
                self.assertEqual((output / artifact['originalRecord']).read_bytes(), original)
                self.assertEqual(artifact['localPolicyProjectedDamage'], 1)
                self.assertFalse(artifact['typedBusinessFinishCaptured'])
                self.assertNotIn('derivedTypedEnd', artifact)

    def test_missing_body_terminal_cannot_be_reported_complete(self):
        source, _, rows = self.record_rows()
        rows[0]['originalFinalActorHP'].pop()
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            path = output / 'records.json'
            path.write_text(json.dumps({'cases': rows}))
            with self.assertRaises(RuntimeError):
                compare.preserve_records(path, output, source)


if __name__ == '__main__':
    unittest.main()
