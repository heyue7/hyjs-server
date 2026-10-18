"""Small integrity checks for ordered fixed-source deliveries."""
from pathlib import Path
import tempfile
import json
import hashlib
import unittest

from tools.freeze_candidate import ROOT, delivery, validate_overlay_base


class OverlayBaseTests(unittest.TestCase):
    def test_stale_shared_source_is_rejected(self):
        metadata = {'files': [{'path': 'shared.go', 'beforeSHA256': 'old'}]}
        with self.assertRaisesRegex(RuntimeError, 'requires rebasing'):
            validate_overlay_base(metadata, [('shared.go', None, 'new')], {'shared.go': 'current'})

    def test_ordered_successor_and_idempotent_delivery(self):
        planned = {'shared.go': 'old'}
        for before, after in [('old', 'first'), ('first', 'second'), ('first', 'second')]:
            validate_overlay_base({'files': [{'path': 'shared.go', 'baseSHA256': before}]},
                                  [('shared.go', None, after)], planned)
        self.assertEqual(planned, {'shared.go': 'second'})

    def test_new_file_cannot_replace_existing_unrelated_source(self):
        with self.assertRaises(RuntimeError):
            validate_overlay_base({'files': [{'path': 'new.go', 'beforeSHA256': None}]},
                                  [('new.go', None, 'new')], {'new.go': 'unrelated'})

    def test_unchanged_external_tool_can_be_pinned_for_first_time(self):
        planned = {}
        validate_overlay_base({'files': [{'path': 'tool.py', 'beforeSHA256': 'same'}]},
                              [('tool.py', None, 'same')], planned)
        self.assertEqual(planned, {'tool.py': 'same'})

    def test_required_proof_assets_are_hash_checked_and_included(self):
        with tempfile.TemporaryDirectory(prefix='freeze-proof-', dir=ROOT / 'build') as temporary:
            folder = Path(temporary)
            for name in ('source', 'proof'):
                (folder / name).write_bytes(name.encode())
            def row(name):
                return {'path': name, 'snapshot': str(folder / name),
                        'sha256': hashlib.sha256(name.encode()).hexdigest()}
            manifest = folder / 'manifest.json'
            manifest.write_text(json.dumps({'files': [row('source')], 'requiredAssets': [row('proof')]}))
            _, rows = delivery(manifest)
            self.assertEqual([entry[0] for entry in rows], ['source', 'proof'])
            (folder / 'proof').write_bytes(b'changed')
            with self.assertRaisesRegex(RuntimeError, 'changed after verification'):
                delivery(manifest)


if __name__ == '__main__':
    unittest.main()
