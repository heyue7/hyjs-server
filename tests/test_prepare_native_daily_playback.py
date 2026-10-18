"""Readonly Daily playback projection preserves native operations and true losses."""
import copy
import json
import unittest
from pathlib import Path

from server.codec import BY_NAME, decode_responses, encode_response, message
from tools.prepare_native_daily_playback import CASES, project

ROOT = Path(__file__).resolve().parents[1]


class NativeDailyPlaybackProjectionTests(unittest.TestCase):
    def rows(self):
        for fight_id, (kind, chapter, fixture_name) in CASES.items():
            fixture = json.loads((ROOT / 'analysis/p6' / fixture_name).read_text())
            reply = message('SCDailyCopyFightEnd', isWin=0).SerializeToString().hex()
            yield [fight_id, '4', kind, chapter, '1', fixture['start_hex'], fixture['end_hex'], reply], fixture

    def test_both_original_losses_exact_frozen_sides_seed_operations_and_wire(self):
        for row, fixture in self.rows():
            out, proof = project(row, fixture)
            start = message('SCDailyCopyFightStart'); start.ParseFromString(bytes.fromhex(fixture['start_hex']))
            end = message('CSDailyCopyFightEnd'); end.ParseFromString(bytes.fromhex(fixture['end_hex']))
            self.assertEqual((out.battleType, out.maxRound, out.randSeed), (4, 40, start.randSeed))
            self.assertEqual(out.attacker, start.battlePlayer); self.assertEqual(out.defender2, start.monster)
            self.assertFalse(out.HasField('defender1')); self.assertEqual(out.optRecord, end.optRecord)
            self.assertEqual((out.serverResult, out.clientResult), (1, 1))
            self.assertEqual((out.attackerFactor, out.defenderFactor), (1, 1))
            frame = encode_response(BY_NAME['SCFightPlayback'], out)
            decoded = decode_responses(frame)
            self.assertEqual(decoded, [(60002, out)])
            self.assertEqual(proof['operations'], len(end.optRecord.skillOpts))
            self.assertEqual(fixture['start_hex'], row[5]); self.assertEqual(fixture['end_hex'], row[6])

    def test_foreign_owner_unsettled_and_unknown_record_rejected(self):
        original, fixture = next(self.rows())
        for index, value in [(1, '5'), (4, '0'), (0, 'unreviewed-fight')]:
            row = original.copy(); row[index] = value
            with self.assertRaises(ValueError): project(row, fixture)

    def test_changed_stored_input_and_operations_rejected(self):
        original, fixture = next(self.rows())
        for index in [5, 6]:
            row = original.copy(); row[index] = row[index] + '0800'
            with self.assertRaisesRegex(ValueError, 'bytes differ'): project(row, fixture)

    def test_disagreeing_actual_settlement_result_rejected(self):
        row, fixture = next(self.rows())
        row[7] = message('SCDailyCopyFightEnd', isWin=1).SerializeToString().hex()
        with self.assertRaisesRegex(ValueError, 'ownership/result mismatch'): project(row, fixture)


if __name__ == '__main__':
    unittest.main()
