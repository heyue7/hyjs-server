"""Pure pre-device validation of new independently named original drivers."""
import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'tools'))
from server.codec import message
from tools.native_batch.prepare import load_plunder_typed_source,load_rush_typed_source
from tools.native_batch.collect import validate_typed_execution
ROOT=Path(__file__).resolve().parents[1]

class NativeRecordedRushAndPlunderTests(unittest.TestCase):
    def test_actual_rush_recorded_source_exact_and_never_automatic(self):
        manifest=ROOT/'analysis/p6/actual-phone-rush-recorded-input-v157/manifest.json'
        row=json.loads(manifest.read_text())['cases'][0]
        self.assertEqual(load_rush_typed_source(row,manifest,'highest'),(manifest.parent/row['typedCallback']).read_bytes())
        for selector in ['pve-auto','pve-compat']:
            with self.assertRaises(RuntimeError):load_rush_typed_source(row,manifest,selector)

    def test_all_eleven_original_plunder_sources_bind_max20_full_actors(self):
        manifest=ROOT/'analysis/p11/actual-original-plunder3-inputs-v156/manifest.json'
        for row in json.loads(manifest.read_text())['cases']:
            self.assertEqual(load_plunder_typed_source(row,manifest,'highest'),(manifest.parent/row['typedCallback']).read_bytes())
            with self.assertRaises(RuntimeError):load_plunder_typed_source(row,manifest,'pve-auto')

    def test_changed_original_rush_end_clock_never_reaches_device(self):
        manifest=ROOT/'analysis/p6/actual-phone-rush-recorded-input-v157/manifest.json';row=json.loads(manifest.read_text())['cases'][0]
        with tempfile.TemporaryDirectory()as folder:
            path=Path(folder)
            for name in ['wire','typedCallback','originalEnd']:(path/row[name]).write_bytes((manifest.parent/row[name]).read_bytes())
            end=message('CSRushFightEnd');end.ParseFromString((path/row['originalEnd']).read_bytes())
            end.optRecord.skillOpts[0].roundTime+=0.1;raw=end.SerializeToString();(path/row['originalEnd']).write_bytes(raw)
            changed=dict(row,originalEndSHA256=hashlib.sha256(raw).hexdigest())
            with self.assertRaisesRegex(RuntimeError,'record identity'):load_rush_typed_source(changed,path/'manifest.json','highest')

    def test_plunder_preset_result_or_wrong_max_round_rejected(self):
        manifest=ROOT/'analysis/p11/actual-original-plunder3-inputs-v156/manifest.json';row=json.loads(manifest.read_text())['cases'][0]
        with tempfile.TemporaryDirectory()as folder:
            path=Path(folder)
            for name in ['wire','typedCallback']:(path/row[name]).write_bytes((manifest.parent/row[name]).read_bytes())
            callback=message('SCFightTreasureStartResult');callback.ParseFromString((path/row['typedCallback']).read_bytes());callback.isWin=1
            raw=callback.SerializeToString();(path/row['typedCallback']).write_bytes(raw)
            changed=dict(row,typedCallbackSHA256=hashlib.sha256(raw).hexdigest())
            with self.assertRaisesRegex(RuntimeError,'result hints'):load_plunder_typed_source(changed,path/'manifest.json','highest')
            (path/row['typedCallback']).write_bytes((manifest.parent/row['typedCallback']).read_bytes())
            wire=message('SCFightPlayback');wire.ParseFromString((path/row['wire']).read_bytes());wire.maxRound=40;(path/row['wire']).write_bytes(wire.SerializeToString())
            with self.assertRaisesRegex(RuntimeError,'Max20 projection'):load_plunder_typed_source(row,path/'manifest.json','highest')

    def test_target_execution_sha_is_required_independently_of_callback(self):
        row=dict(batchTypedCallback='000.plunder-start.bin',typedCallbackSHA256='a'*64,batchTypedTarget='000.plunder-target.txt',typedTargetSHA256='b'*64)
        validate_typed_execution(row,dict(typedCallbackSHA256='a'*64,typedTargetSHA256='b'*64))
        for value in [None,'c'*64]:
            with self.assertRaisesRegex(ValueError,'target execution'):validate_typed_execution(row,dict(typedCallbackSHA256='a'*64,typedTargetSHA256=value))
