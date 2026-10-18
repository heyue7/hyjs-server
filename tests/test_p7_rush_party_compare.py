"""Offline four-case source/record retention boundaries; no native parity claim."""
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
from tools.native_batch import compare_rush_party as compare
from server.codec import message


class RushPartyComparisonPreparationTests(unittest.TestCase):
    def test_real_input_preflight_is_four_unrecorded_typed9_cases(self):
        source = compare.preflight()
        self.assertEqual(len(source['cases']), 4)
        for case in source['cases']:
            wire = message('SCFightPlayback')
            wire.ParseFromString((compare.SOURCE.parent / case['wire']).read_bytes())
            self.assertFalse(wire.HasField('optRecord'))
            self.assertEqual(wire.battleType, 9)
            self.assertEqual([hero.id for hero in wire.attacker.formation], ['hero_001', 'hero_003'])

    def make_capture(self, output):
        source = compare.preflight()
        rows = []
        for index, case in enumerate(source['cases']):
            stem = f'{index:03d}'
            driver_sha = hashlib.sha256(str(case['manualAutoStartRoundTime']).encode()).hexdigest()
            policy_sha = hashlib.sha256(case['manualControlSource'].encode()).hexdigest()
            first = {'kind': 'input', 'scope': compare.SCOPE, 'fightId': case['fightId'],
                     'seed': case['seed'], 'battleType': 9,
                     'typedCallbackSHA256': case['typedCallbackSHA256'], 'typedDriverSHA256': driver_sha,
                     'typedDriverPolicySHA256': policy_sha, 'manualControlSource': case['manualControlSource']}
            # Synthetic boundary event, never used by comparison or labelled native proof.
            events = [first, {'kind': 'original_final_operates', 'data': {}},
                      {'kind': 'result', 'attackerWon': True}]
            native = output / (stem + '.native.jsonl')
            native.write_text(''.join(json.dumps(event) + '\n' for event in events))
            rows.append({**case, 'batchWire': stem + '.pb', 'nativeResult': native.name,
                         'nativeSHA256': compare.sha(native), 'batchTypedCallback': stem + '.rush-start.bin',
                         'batchTypedDriver': stem + '.rush-auto-start.txt', 'typedDriverSHA256': driver_sha,
                         'batchTypedDriverPolicy': stem + '.rush-auto-policy.txt',
                         'typedDriverPolicySHA256': policy_sha})
        capture = {'sourceManifest': str(compare.SOURCE), 'sourceManifestSHA256': compare.SOURCE_SHA,
                   'selector': 'highest', 'cases': rows}
        path = output / 'manifest.json'
        path.write_text(json.dumps(capture))
        return source, capture, path

    def test_capture_checks_exact_denominator_scope_and_driver(self):
        with tempfile.TemporaryDirectory() as directory:
            source, captured, path = self.make_capture(Path(directory))
            compare.validate_capture(path, source)
            for alter in ('missing-case', 'changed-clock', 'changed-native-sha'):
                modified = copy.deepcopy(captured)
                if alter == 'missing-case':
                    modified['cases'].pop()
                elif alter == 'changed-clock':
                    modified['cases'][0]['manualAutoStartRoundTime'] = 1.0
                else:
                    modified['cases'][0]['nativeSHA256'] = 'a' * 64
                path.write_text(json.dumps(modified))
                with self.subTest(alter=alter), self.assertRaises(RuntimeError):
                    compare.validate_capture(path, source)

    def test_truncated_raw_capture_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            source, captured, path = self.make_capture(output)
            native = output / captured['cases'][0]['nativeResult']
            native.write_text(native.read_text().splitlines()[0] + '\n')
            captured['cases'][0]['nativeSHA256'] = compare.sha(native)
            path.write_text(json.dumps(captured))
            with self.assertRaises(RuntimeError):
                compare.validate_capture(path, source)

    def test_original_record_bytes_preserved_and_end_labelled_derived(self):
        source = compare.preflight()
        original = message('BattleOperate', finalRoundNum=2, attackerTotalLeftHp=700,
                           defenderTotalLeftHp=0).SerializeToString(deterministic=True)
        rows = [{'wireSHA256': case['wireSHA256'], 'attackerWon': True,
                 'originalFinalRecord': {'protobufBase64': base64.b64encode(original).decode(),
                                         'protobufSHA256': hashlib.sha256(original).hexdigest()}}
                for case in source['cases']]
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            path = output / 'records.json'
            path.write_text(json.dumps({'cases': rows}))
            artifacts = compare.preserve_records(path, output, source)
            for artifact, case in zip(artifacts, source['cases']):
                self.assertEqual((output / artifact['originalRecord']).read_bytes(), original)
                self.assertIn('NOT_NETWORK_CAPTURE', artifact['typedEndProvenance'])
                end = message('CSRushFightEnd')
                end.ParseFromString((output / artifact['derivedTypedEnd']).read_bytes())
                self.assertEqual(end.fightId, case['fightId'])
                self.assertEqual(end.optRecord.SerializeToString(deterministic=True), original)


if __name__ == '__main__':
    unittest.main()
