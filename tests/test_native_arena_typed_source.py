"""Validate optional original arena typed sidecars without device operations."""
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import sys
from unittest.mock import patch
import unittest

from server.codec import message
ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location("arena_prepare_draft",ROOT/"analysis/p6/prepare.typed2-v121.py")
prepare=importlib.util.module_from_spec(spec);spec.loader.exec_module(prepare)
collect_spec=importlib.util.spec_from_file_location("arena_collect_draft",ROOT/"analysis/p6/collect.typed2-v121.py")
collect=importlib.util.module_from_spec(collect_spec);collect_spec.loader.exec_module(collect)

SOURCE=ROOT/"analysis/p6/native-actual-arena047-typed-inputs-v121"

class NativeArenaTypedSourceTests(unittest.TestCase):
 def setUp(self):
  self.prepare=prepare
  self.temp=tempfile.TemporaryDirectory(prefix="hyjs-arena-stage-unit-");self.addCleanup(self.temp.cleanup);self.folder=Path(self.temp.name)
  self.rows=json.loads((SOURCE/"manifest.json").read_text())["cases"]
 def source(self):
  folder=ROOT/"analysis/p6/native-actual-arena047-typed-inputs-v121"
  meta=json.loads((folder/"manifest.json").read_text());case=meta["cases"][0]
  wire=message("SCFightPlayback");wire.ParseFromString((folder/case["wire"]).read_bytes())
  typed=message("SCPvpFightResult");typed.ParseFromString((folder/case["typedCallback"]).read_bytes())
  return wire,typed
 def validate(self,wire,typed,selector="highest",callback="000.SCPvpFightResult.pb"):
  with tempfile.TemporaryDirectory(prefix="hyjs-arena-source-unit-") as tmp:
   folder=Path(tmp);raw=typed.SerializeToString(deterministic=True)
   (folder/"000.pb").write_bytes(wire.SerializeToString(deterministic=True));(folder/callback).write_bytes(raw)
   return prepare.load_arena_typed_source({"wire":"000.pb","battleType":2,"typedCallback":callback,"typedCallbackSHA256":hashlib.sha256(raw).hexdigest()},folder/"manifest.json",selector)
 def test_valid_actual_queries_and_legacy_no_sidecar(self):
  folder=ROOT/"analysis/p6/native-actual-arena047-typed-inputs-v121";meta=json.loads((folder/"manifest.json").read_text())
  for case in meta["cases"]:self.assertEqual(prepare.load_arena_typed_source(case,folder/"manifest.json","highest"),(folder/case["typedCallback"]).read_bytes())
  self.assertIsNone(prepare.load_arena_typed_source({"battleType":2},folder/"manifest.json","highest"))
 def test_wrong_selector(self):
  wire,typed=self.source()
  with self.assertRaises(RuntimeError):self.validate(wire,typed,"pve-auto")
 def test_server_winner_or_reward_hints_rejected(self):
  for name in ["winner","reward"]:
   wire,typed=self.source()
   if name=="winner":typed.winPlayerId=wire.attacker.playerId
   else:typed.result.SetInParent()
   with self.assertRaises(RuntimeError):self.validate(wire,typed)
 def test_source_field_differences_rejected(self):
  for name in ["seed","actor","factor","mode"]:
   wire,typed=self.source()
   if name=="seed":typed.randSeed+=1
   elif name=="actor":typed.playerList[0].formation[0].prop.hp+=1
   elif name=="factor":wire.attackerFactor=0
   else:wire.battleType=11
   with self.assertRaises(RuntimeError):self.validate(wire,typed)
 def test_missing_original_inst_rejected(self):
  for name in ["player","actor"]:
   wire,typed=self.source()
   if name=="player":typed.playerList[1].ClearField("inst")
   else:typed.playerList[1].formation[0].ClearField("inst")
   with self.assertRaises(RuntimeError):self.validate(wire,typed)
 def test_native_must_report_exact_sidecar_sha(self):
  case={"batchTypedCallback":"000.arena-result.bin","typedCallbackSHA256":"a"*64}
  collect.validate_typed_execution(case,{"typedCallbackSHA256":"a"*64})
  for first in [{},{"typedCallbackSHA256":"b"*64}]:
   with self.assertRaises(ValueError):collect.validate_typed_execution(case,first)

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
             patch.object(sys, 'argv', ['prepare', '--manifest', str(manifest), '--selector', 'highest']), patch('builtins.print'):
         self.prepare.main()
     staged = json.loads((root / 'build/native-batch-input/highest/manifest.json').read_text())
     self.assertEqual(len(staged['cases']), 3)
     for row in staged['cases']:
         target = 'files/native-batch/highest/' + row['batchTypedCallback']
         self.assertEqual(remote[target], (SOURCE / row['typedCallback']).read_bytes())
         self.assertEqual(row['batchTypedCallback'], Path(row['batchWire']).stem + '.arena-result.bin')
     self.assertFalse(any(p.startswith('/data/local/tmp/') for p in remote))

 def test_wrong_source_hash_and_callback_name(self):
  case=self.rows[0]
  for changed in [dict(case,typedCallbackSHA256="0"*64),dict(case,typedCallback="../outside.SCPvpFightResult.pb")]:
   with self.assertRaises(RuntimeError):prepare.load_arena_typed_source(changed,SOURCE/"manifest.json","highest")

if __name__=="__main__":unittest.main()
