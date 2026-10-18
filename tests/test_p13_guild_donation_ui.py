"""Offline UI batch control checks; no device, HTTP, or database connection."""
import copy
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

from PIL import Image

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools import p13_guild_donation_ui as ui


def facts(coin=2000000, badge=30, score=0, sources=0):
    return {'playerQualification': {'playerId': 4, 'coin': coin, 'diamond': 100,
                                   'heldBadge089': badge, 'hasGuild': True, 'role': 1,
                                   'lastDonateDay': '2026-10-16',
                                   'lastDayDonatedCoin': sources * 100000,
                                   'lastDayDonatedBadge': 30 - badge},
            'guildGates': {'economy': True}, 'policies': {'badges': {'enabled': True}},
            'boxScore': score, 'boxClaims': 0, 'paidBadgeSources': sources,
            'paidBadgeDeliveriesClaimed': 0}


class FakeDevice:
    def __init__(self):
        self.taps = []
        self.captures = []

    def require_game_surface_accessible(self):
        pass

    def game_is_foreground(self):
        return True

    def capture(self, path):
        self.captures.append(path)
        Image.new('RGB', (20, 20)).save(path)
        return {'evidence': str(path)}

    def run(self, *args):
        self.taps.append(args)


class DonationUIBatchTests(unittest.TestCase):
    def run_batch(self, action, count, observations, binary_shas=None, expect_failure=False):
        device = FakeDevice()
        with tempfile.TemporaryDirectory() as folder:
            record = Path(folder) / 'batch.json'
            argv = ['donation-ui', action, '--serial', 'offline-only', '--x', '10', '--y', '10',
                    '--expected-binary-sha', 'a' * 64, '--record', str(record)]
            if count is not None:
                argv += ['--count', str(count)]
            shas = binary_shas or ['a' * 64] * 32
            with patch.object(sys, 'argv', argv), patch.object(ui, 'Device', return_value=device), \
                    patch.object(ui, 'snapshot', side_effect=copy.deepcopy(observations)), \
                    patch.object(ui, 'operations', side_effect=[{'binarySha256': sha} for sha in shas]), \
                    patch('builtins.print'):
                if expect_failure:
                    with self.assertRaises(RuntimeError):
                        ui.main()
                else:
                    ui.main()
            result = json.loads(record.read_text())
            self.assertEqual(len(device.captures), 2)
            self.assertEqual(len(result['runnerSHA256']), 64)
            for step in result['steps']:
                self.assertEqual(len(step['before']['snapshotSHA256']), 64)
                if 'after' in step:
                    self.assertEqual(len(step['after']['snapshotSHA256']), 64)
            return result, device

    def test_default_single_step_keeps_status(self):
        result, device = self.run_batch('coin', None,
                                       [facts(), facts(1900000, score=1, sources=1)])
        self.assertEqual(result['status'], 'PASS_ONE_ORIGINAL_UI_STEP')
        self.assertEqual(result['requestedCount'], 1)
        self.assertEqual(result['completedCount'], 1)
        self.assertEqual(len(device.taps), 1)

    def test_coin_batch_uses_each_verified_read_as_next_baseline(self):
        values = [facts(2000000 - i * 100000, score=i, sources=i) for i in range(4)]
        result, device = self.run_batch('coin', 3, values)
        self.assertEqual(result['status'], 'PASS_ORIGINAL_UI_BATCH')
        self.assertEqual(result['completedCount'], 3)
        self.assertEqual(len(device.taps), 3)
        for before, after in zip(result['steps'], result['steps'][1:]):
            self.assertEqual(before['after'], after['before'])

    def test_thirty_badges_have_thirty_exact_consumptions(self):
        values = [facts(badge=30 - i, score=8 * i) for i in range(31)]
        result, device = self.run_batch('badge', 30, values)
        self.assertEqual(result['completedCount'], 30)
        self.assertEqual(len(device.taps), 30)
        self.assertEqual(result['after']['boxScore'], 240)

    def test_double_change_stops_before_next_tap(self):
        result, device = self.run_batch('coin', 3,
                                       [facts(), facts(1800000, score=2, sources=2)], expect_failure=True)
        self.assertEqual(result['completedCount'], 0)
        self.assertEqual(len(device.taps), 1)
        self.assertEqual(result['steps'][0]['status'], 'FAIL_STOP_WITHOUT_RETRY')
        self.assertIn('after', result['steps'][0])

    def test_no_change_times_out_without_retry(self):
        # Expire the poll immediately, so the offline case has no real sleep.
        with patch.object(ui.time, 'monotonic', side_effect=[0, 9]):
            result, device = self.run_batch('badge', 2, [facts(), facts()], expect_failure=True)
        self.assertEqual(len(device.taps), 1)
        self.assertEqual(result['completedCount'], 0)

    def test_runtime_change_preserves_completed_step_and_stops(self):
        result, device = self.run_batch('coin', 3, [facts(), facts(1900000, score=1, sources=1)],
                                       ['a' * 64, 'a' * 64, 'b' * 64], expect_failure=True)
        self.assertEqual(result['completedCount'], 1)
        self.assertEqual(len(device.taps), 1)
        self.assertFalse(result['steps'][1]['tapSent'])

    def test_bad_counts_rejected_before_resources(self):
        for action, count in [('coin', 0), ('badge', 31), ('mail', 2), ('relogin', 2)]:
            with self.subTest(action=action, count=count), self.assertRaises(RuntimeError):
                ui.check_count(action, count)
        ui.check_count('coin', 30)
        ui.check_count('observe', 1)


if __name__ == '__main__':
    unittest.main()
