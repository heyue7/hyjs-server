"""Pure source admission checks; importing or running these never uses a device."""
import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from server.codec import message
from tools.native_batch.prepare import load_awake_field_typed_source

ROOT = Path(__file__).resolve().parents[1]


class IndependentTypedSources(unittest.TestCase):
    def sources(self):
        for name in ('analysis/p7/awake-playable-20261016/native-inputs/manifest.json',
                     'analysis/p6/field-pvp5-actual-actors-pending-20261016/manifest.json'):
            path = ROOT / name
            for case in json.loads(path.read_text())['cases']:
                yield path, case

    def test_exact_sources_and_scope_separation(self):
        count = 0
        for manifest, case in self.sources():
            self.assertTrue(load_awake_field_typed_source(case, manifest, 'highest'))
            for selector in ('golden', 'pve-auto'):
                with self.assertRaises(RuntimeError):
                    load_awake_field_typed_source(case, manifest, selector)
            wrong = dict(case, nativeScope='original-pvp-automatic-ai')
            with self.assertRaises(RuntimeError):
                load_awake_field_typed_source(wrong, manifest, 'highest')
            count += 1
        self.assertEqual(count, 6)

    def test_rehashed_typed_mutation_does_not_authorize_foreign_actors_or_results(self):
        for manifest, case in self.sources():
            if case['case'] not in ('actual-full', 'diagnostic-awake16-normalparty-0'):
                continue
            with tempfile.TemporaryDirectory(prefix='hyjs-native-source-boundary-') as name:
                folder = Path(name)
                for key in ('wire', 'typedCallback'):
                    (folder / case[key]).write_bytes((manifest.parent / case[key]).read_bytes())
                typed = message('SCAwakeCopyFightBegin' if case['battleType'] == 16 else 'SidePvpFightData')
                typed.ParseFromString((folder / case['typedCallback']).read_bytes())
                if case['battleType'] == 16:
                    typed.battlePlayer.formation[0].inst.curHp -= 1
                else:
                    typed.winPlayerId = typed.attPlayer.playerId
                raw = typed.SerializeToString(deterministic=True)
                (folder / case['typedCallback']).write_bytes(raw)
                changed = dict(case, typedCallbackSHA256=hashlib.sha256(raw).hexdigest())
                with self.assertRaises(RuntimeError):
                    load_awake_field_typed_source(changed, folder / 'manifest.json', 'highest')


if __name__ == '__main__':
    unittest.main()
