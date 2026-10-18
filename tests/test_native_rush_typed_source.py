"""SHA-bound actual isolated Rush typed source, transport and execution binding."""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import sys
import unittest
from unittest.mock import patch

from server.codec import message

ROOT = Path(__file__).resolve().parents[1]
FIXED = ROOT / 'analysis/p6/v104-delta-source/tools/native_batch'
SOURCE = ROOT / 'analysis/p6/actual-local-p7-rush-inputs-v104'


def module(name):
    sys.path.insert(0, str(ROOT / 'tools'))
    spec = importlib.util.spec_from_file_location('rush_typed_' + name, FIXED / (name + '.py'))
    obj = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(obj)
    obj.ROOT = ROOT
    return obj


class NativeRushTypedSourceTests(unittest.TestCase):
    def setUp(self):
        self.prepare = module('prepare')
        self.collect = module('collect')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.folder = Path(self.temp.name)
        self.rows = json.loads((SOURCE / 'manifest.json').read_text())['cases']
        for row in self.rows:
            for field in ['wire', 'typedCallback']:
                (self.folder / row[field]).write_bytes((SOURCE / row[field]).read_bytes())

    def test_exact_actual_trial_elite_typed_bytes_preserved(self):
        for row in self.rows:
            raw = self.prepare.load_rush_typed_source(row, self.folder / 'manifest.json', 'pve-auto')
            self.assertEqual(raw, (SOURCE / row['typedCallback']).read_bytes())
            self.assertEqual(hashlib.sha256(raw).hexdigest(), row['typedCallbackSHA256'])

    def test_foreign_selector_type_and_changed_original_hash_rejected_before_device_io(self):
        row = self.rows[0]
        for selector in ['highest', 'pve-compat']:
            with self.assertRaises(RuntimeError):
                self.prepare.load_rush_typed_source(row, self.folder / 'manifest.json', selector)
        for change in [{'typedCallbackSHA256': '0' * 64}, {'battleType': 14}, {'typedCallback': '../source.SCRushFightStart.pb'}]:
            with self.assertRaises(RuntimeError):
                self.prepare.load_rush_typed_source(dict(row, **change), self.folder / 'manifest.json', 'pve-auto')

    def test_misprojected_factors_actor_seed_mode_record_or_wave_never_reach_original_caller(self):
        row = self.rows[0]
        raw = (SOURCE / row['wire']).read_bytes()
        mutations = [lambda x: setattr(x, 'attackerFactor', 0), lambda x: setattr(x, 'randSeed', x.randSeed + 1),
                     lambda x: setattr(x, 'battleType', 10), lambda x: setattr(x.attacker.formation[0].inst, 'curHp', 1),
                     lambda x: x.optRecord.SetInParent()]
        for mutate in mutations:
            wire = message('SCFightPlayback'); wire.ParseFromString(raw); mutate(wire)
            (self.folder / row['wire']).write_bytes(wire.SerializeToString())
            with self.assertRaisesRegex(RuntimeError, 'projection differ'):
                self.prepare.load_rush_typed_source(row, self.folder / 'manifest.json', 'pve-auto')
        (self.folder / row['wire']).write_bytes(raw)
        original = message('SCRushFightStart'); original.ParseFromString((SOURCE / row['typedCallback']).read_bytes())
        for kind in ['condition', 'wave']:
            changed = copy.deepcopy(original)
            if kind == 'condition': changed.winKey = 1
            else: changed.battleMonsterList.add().CopyFrom(original.battleMonsterList[0])
            data = changed.SerializeToString()
            (self.folder / row['typedCallback']).write_bytes(data)
            variant = dict(row, typedCallbackSHA256=hashlib.sha256(data).hexdigest())
            with self.assertRaisesRegex(RuntimeError, 'projection differ'):
                self.prepare.load_rush_typed_source(variant, self.folder / 'manifest.json', 'pve-auto')

    def test_prepare_publishes_each_verified_sidecar_with_its_exact_pair(self):
        root = self.folder.resolve() / 'workspace'
        source = root / 'analysis/p6/rush-source'
        source.mkdir(parents=True)
        for row in self.rows:
            for field in ['wire', 'typedCallback']:
                (source / row[field]).write_bytes((SOURCE / row[field]).read_bytes())
        manifest = source / 'manifest.json'
        manifest.write_text(json.dumps({'cases': self.rows}))
        remote = {}

        class Device:
            def __init__(self, *args): pass

            def run(self, *args, **kwargs):
                if args[0] == 'push':
                    for item in Path(args[1]).iterdir():
                        remote[args[2] + '/' + item.name] = item.read_bytes()
                elif args[:3] == ('shell', 'run-as', self_package):
                    if args[3] == 'cp': remote[args[5]] = remote[args[4]]
                    elif args[3] == 'rm': remote.pop(args[4], None)
                elif args[:3] == ('shell', 'rm', '-rf'):
                    for key in list(remote):
                        if key.startswith(args[3] + '/'): del remote[key]

            def text(self, *args):
                if args[3] == 'ls':
                    prefix = args[4] + '/'
                    return '\n'.join(k[len(prefix):] for k in remote if k.startswith(prefix))
                if args[3] == 'sha256sum': return hashlib.sha256(remote[args[4]]).hexdigest() + '  checked'
                if args[3] == 'cat': return remote[args[4]].decode()
                raise AssertionError(args)

        self_package = self.prepare.PACKAGE
        with patch.object(self.prepare, 'ROOT', root), patch.object(self.prepare, 'Device', Device), \
                patch.object(sys, 'argv', ['prepare', '--manifest', str(manifest), '--selector', 'pve-auto']), patch('builtins.print'):
            self.prepare.main()
        staged = json.loads((root / 'build/native-batch-input/pve-auto/manifest.json').read_text())
        self.assertEqual(len(staged['cases']), 2)
        for row in staged['cases']:
            target = 'files/native-batch/pve-auto/' + row['batchTypedCallback']
            self.assertEqual(remote[target], (SOURCE / row['typedCallback']).read_bytes())
            self.assertEqual(row['batchTypedCallback'], Path(row['batchWire']).stem + '.rush-start.bin')
        self.assertFalse(any(p.startswith('/data/local/tmp/') for p in remote))

    def test_original_required_inst_presence_is_not_a_zero_value_default(self):
        row = self.rows[0]
        for target in ['player', 'monster', 'actor', 'prop']:
            typed = message('SCRushFightStart')
            typed.ParseFromString((SOURCE / row['typedCallback']).read_bytes())
            if target == 'player': typed.battlePlayer.ClearField('inst')
            elif target == 'monster': typed.battleMonsterList[0].ClearField('inst')
            elif target == 'actor': typed.battleMonsterList[0].formation[0].ClearField('inst')
            else: typed.battleMonsterList[0].formation[0].ClearField('prop')
            wire = message('SCFightPlayback'); wire.ParseFromString((SOURCE / row['wire']).read_bytes())
            wire.attacker.CopyFrom(typed.battlePlayer)
            wire.defender2.CopyFrom(typed.battleMonsterList[0])
            data = typed.SerializeToString()
            (self.folder / row['typedCallback']).write_bytes(data)
            (self.folder / row['wire']).write_bytes(wire.SerializeToString())
            bound = dict(row, typedCallbackSHA256=hashlib.sha256(data).hexdigest())
            with self.assertRaisesRegex(RuntimeError, 'requires'):
                self.prepare.load_rush_typed_source(bound, self.folder / 'manifest.json', 'pve-auto')
        legacy = ROOT / 'analysis/p6/actual-local-p7-rush-inputs-v97'
        bad = json.loads((legacy / 'manifest.json').read_text())['cases'][0]
        for key in ['wire', 'typedCallback']:
            (self.folder / bad[key]).write_bytes((legacy / bad[key]).read_bytes())
        with self.assertRaisesRegex(RuntimeError, 'requires player and monster Inst'):
            self.prepare.load_rush_typed_source(bad, self.folder / 'manifest.json', 'pve-auto')

    def test_collector_requires_execution_of_exact_typed_artifact(self):
        row = dict(self.rows[0], batchTypedCallback='000.rush-start.bin')
        self.collect.validate_typed_execution(row, {'typedCallbackSHA256': row['typedCallbackSHA256']})
        for first in [{}, {'typedCallbackSHA256': None}, {'typedCallbackSHA256': '0' * 64}]:
            with self.assertRaisesRegex(ValueError, 'execution SHA'):
                self.collect.validate_typed_execution(row, first)
        # Historical inputs have no sidecar; their exact old scope remains valid.
        self.collect.validate_typed_execution({'batchWire': '000.pb'}, {})


if __name__ == '__main__':
    unittest.main()
