"""Offline thirty-case family/provenance boundaries; no native or business proof."""
import copy
import json
from pathlib import Path
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.native_batch import compare_worldboss_party as compare


class WorldBossBoostComparisonPreparationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        compare.configure_family('boost')
        cls.source = compare.preflight()

    @classmethod
    def tearDownClass(cls):
        compare.configure_family('base')

    def test_thirty_exact_unpaid_masks_with_two_seeds_are_not_base_certificate(self):
        self.assertEqual(compare.CASE_COUNT, 30)
        self.assertEqual(compare.BUNDLE_ENV, 'HYJS_P12_WORLDBOSS_BOOST_NATIVE_BUNDLE')
        self.assertEqual(compare.REQUIRED_TEST, 'TestP12WorldBossBoostNativeAllLayers')
        self.assertEqual({(case['boostMask'], case['seed']) for case in self.source['cases']},
                         {(mask, seed) for mask in range(1, 16) for seed in (7, 314159)})
        self.assertFalse(self.source['actualPaid'])
        self.assertFalse(self.source['purchaseLedgerCreated'])
        self.assertFalse(self.source['actualBusinessStart'])

    def test_capture_rejects_missing_subset_and_changed_boost_or_frozen_metadata(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            rows = []
            for index, case in enumerate(self.source['cases']):
                name = f'{index:03d}.native.jsonl'
                events = [{'kind': 'input', 'scope': compare.SCOPE, 'fightId': case['fightId'],
                           'seed': case['seed'], 'battleType': 18,
                           'typedCallbackSHA256': case['typedCallbackSHA256']},
                          {'kind': 'original_final_operates', 'data': {}},
                          {'kind': 'result', 'attackerWon': False}]
                (output / name).write_text(''.join(json.dumps(event) + '\n' for event in events))
                rows.append({**case, 'batchWire': f'{index:03d}.pb', 'nativeResult': name,
                             'nativeSHA256': compare.sha(output / name),
                             'batchTypedCallback': f'{index:03d}.worldboss-start.bin'})
            captured = {'sourceManifest': str(compare.SOURCE), 'sourceManifestSHA256': compare.SOURCE_SHA,
                        'selector': 'highest', 'cases': rows}
            path = output / 'manifest.json'
            path.write_text(json.dumps(captured))
            compare.validate_capture(path, self.source)
            for alter in ('missing-subset', 'wrong-mask', 'wrong-purchased-id', 'wrong-frozen-sha'):
                changed = copy.deepcopy(captured)
                if alter == 'missing-subset':
                    changed['cases'].pop()
                elif alter == 'wrong-mask':
                    changed['cases'][0]['boostMask'] = 15
                elif alter == 'wrong-purchased-id':
                    changed['cases'][0]['diagnosticBoostIDs'] = ['wordboss_4']
                else:
                    changed['cases'][0]['frozenInputSHA256'] = '0' * 64
                path.write_text(json.dumps(changed))
                with self.subTest(alter=alter), self.assertRaises(RuntimeError):
                    compare.validate_capture(path, self.source)


if __name__ == '__main__':
    unittest.main()
