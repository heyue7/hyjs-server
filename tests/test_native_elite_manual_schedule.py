"""Explicit diagnostic control stays separate from actual observed End evidence."""
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tools'))
NEXT=ROOT/'analysis/p6/native-elite-manual-next-v165'

class EliteManualScheduleTests(unittest.TestCase):
 def setUp(self):
  spec=importlib.util.spec_from_file_location('elite_manual_prepare',NEXT/'tools/native_batch/prepare.py')
  self.prepare=importlib.util.module_from_spec(spec);spec.loader.exec_module(self.prepare);self.prepare.ROOT=ROOT
  self.manifest=ROOT/'analysis/p6/original-elite-manual-inputs-v165/manifest.json'
  self.cases=json.loads(self.manifest.read_text())['cases']
 def test_exact_original_typed_and_actor_sources_are_unchanged(self):
  original=ROOT/'analysis/p6/actual-local-p7-rush-terminal-inputs-v127'
  for index,row in enumerate(self.cases):
   self.prepare.load_rush_typed_source(row,self.manifest,'highest')
   self.assertEqual((self.manifest.parent/row['wire']).read_bytes(),(original/f'{index+2:03}.pb').read_bytes())
   self.assertEqual((self.manifest.parent/row['typedCallback']).read_bytes(),(original/f'{index+2:03}.SCRushFightStart.pb').read_bytes())
   self.assertNotIn('originalEnd',row)
 def test_observed_manual_playback_and_original_auto_remain_strict(self):
  for folder,selector in [('actual-phone-rush-manual-auto-input-v161','highest'),('actual-phone-rush-recorded-input-v157','highest'),('actual-local-p7-rush-inputs-v104','pve-auto')]:
   path=ROOT/'analysis/p6'/folder/'manifest.json'
   for row in json.loads(path.read_text())['cases']:self.prepare.load_rush_typed_source(row,path,selector)
  path=ROOT/'analysis/p6/actual-phone-rush-manual-auto-input-v161/manifest.json'
  row=json.loads(path.read_text())['cases'][0]
  with self.assertRaises(RuntimeError):self.prepare.load_rush_typed_source(dict(row,manualAutoStartRoundTime=30.1),path,'highest')
 def test_schedule_cannot_carry_or_relabel_observed_end(self):
  row=self.cases[0]
  for fields in [dict(originalEnd='000.CSRushFightEnd.pb'),dict(originalRecordSHA256='a'*64),dict(manualAutoStartRoundTime=-1),dict(manualAutoStartRoundTime=111),dict(manualAutoStartRoundTime=float('nan')),dict(manualControlSource='unknown')]:
   with self.assertRaises(RuntimeError):self.prepare.load_rush_typed_source(dict(row,**fields),self.manifest,'highest')
  with tempfile.TemporaryDirectory() as temporary:
   target=Path(temporary)
   for key in ['wire','typedCallback']:(target/row[key]).write_bytes((self.manifest.parent/row[key]).read_bytes())
   from server.codec import message
   wire=message('SCFightPlayback');wire.ParseFromString((target/row['wire']).read_bytes());wire.optRecord.finalRoundNum=1
   (target/row['wire']).write_bytes(wire.SerializeToString())
   with self.assertRaises(RuntimeError):self.prepare.load_rush_typed_source(row,target/'manifest.json','highest')
 def test_live_caller_never_consumes_supplied_operation_record(self):
  source=(NEXT/'analysis/p6/OriginalHeadlessPlaybackCaller.cs').read_text()
  method=source.split('public static bool RunRushManualAutoStart',1)[1].split('private static string OriginalMessageBytes',1)[0]
  self.assertNotIn('identity.optRecord == null',method)
  self.assertNotIn('SetOperateRecord',method)
  self.assertIn('data.SetData(start, identity.battleType == 9)',method)
  self.assertIn('RunWithManualAutoDelay',method)

 def test_native_reports_diagnostic_schedule_provenance(self):
  spec=importlib.util.spec_from_file_location('elite_manual_collect',NEXT/'tools/native_batch/collect.py')
  collect=importlib.util.module_from_spec(spec);spec.loader.exec_module(collect)
  row={'manualControlSource':'prepublished-diagnostic-schedule-v1'}
  collect.validate_typed_execution(row,dict(row))
  for source in [None,'actual-original-End-evidence']:
   with self.assertRaises(ValueError):collect.validate_typed_execution(row,{'manualControlSource':source})
  runner=(NEXT/'tools/native_batch/NativeBatchRunner.cs').read_text()
  self.assertIn('manualControlSource',runner)
  self.assertNotIn('rush == null || forceAutomatic || localCompatibility || wire.optRecord == null',runner)
