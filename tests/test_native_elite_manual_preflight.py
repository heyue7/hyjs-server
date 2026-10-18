"""Exercise the actual prepare/collect preflight for no-record manual schedules."""
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[1]
NEXT=ROOT/'analysis/p6/native-elite-manual-next-v165b'
sys.path.insert(0,str(ROOT/'tools'))
from server.codec import message

def load(name):
 spec=importlib.util.spec_from_file_location('elite_preflight_'+name,NEXT/'tools/native_batch'/(name+'.py'))
 module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);module.ROOT=ROOT;return module

class EliteRealPreflightTests(unittest.TestCase):
 def setUp(self):
  self.prepare=load('prepare');self.collect=load('collect')
  self.source=ROOT/'analysis/p6/original-elite-manual-inputs-v165/manifest.json'
  self.rows=json.loads(self.source.read_text())['cases']
 def staged(self,row,index=0):
  row=dict(row,batchWire=f'{index:03}.pb',batchTypedCallback=f'{index:03}.rush-start.bin',batchTypedDriver=f'{index:03}.rush-auto-start.txt',batchTypedDriverPolicy=f'{index:03}.rush-auto-policy.txt')
  row['typedDriverSHA256']=hashlib.sha256(str(row['manualAutoStartRoundTime']).encode('ascii')).hexdigest()
  row['typedDriverPolicySHA256']=hashlib.sha256(row['manualControlSource'].encode('ascii')).hexdigest()
  return row
 def test_real_preflight_only_exact_policy_callback_driver_delay_can_omit_record(self):
  for index,source in enumerate(self.rows):
   row=self.staged(source,index);wire=message('SCFightPlayback');wire.ParseFromString((self.source.parent/source['wire']).read_bytes())
   self.assertTrue(self.collect.validate_typed_preflight(row,wire,'highest',row['batchWire']))
   for fields in [dict(batchTypedCallback=None),dict(batchTypedDriver=None),dict(batchTypedDriverPolicy=None),dict(typedDriverSHA256='0'*64),dict(typedDriverPolicySHA256='0'*64),dict(manualAutoStartRoundTime=30.1),dict(manualControlSource='unknown')]:
    with self.assertRaises(RuntimeError):self.collect.validate_typed_preflight(dict(row,**fields),wire,'highest',row['batchWire'])
   with self.assertRaises(RuntimeError):self.collect.validate_typed_preflight(row,wire,'pve-auto',row['batchWire'])
 def test_observed_record_mode_keeps_its_record_requirement(self):
  path=ROOT/'analysis/p6/actual-phone-rush-manual-auto-input-v161/manifest.json'
  source=json.loads(path.read_text())['cases'][0]
  row=dict(source,batchWire='000.pb',batchTypedCallback='000.rush-start.bin',batchTypedDriver='000.rush-auto-start.txt')
  wire=message('SCFightPlayback');wire.ParseFromString((path.parent/source['wire']).read_bytes())
  self.assertFalse(self.collect.validate_typed_preflight(row,wire,'highest','000.pb'))
  wire.ClearField('optRecord')
  with self.assertRaises(RuntimeError):self.collect.validate_typed_preflight(row,wire,'highest','000.pb')
 def test_actual_prepare_and_collector_main_pass_every_guard_before_result_wait(self):
  with tempfile.TemporaryDirectory() as folder:
   root=Path(folder).resolve();source=root/'analysis/p6/source';source.mkdir(parents=True)
   for row in self.rows:
    for key in ['wire','typedCallback']:(source/row[key]).write_bytes((self.source.parent/row[key]).read_bytes())
   manifest=source/'manifest.json';manifest.write_text(json.dumps({'cases':self.rows}))
   remote={};package=self.prepare.PACKAGE
   class Device:
    def __init__(self,*args):self.serial='pure-mock'
    def run(self,*args,**kwargs):
     if args[0]=='push':
      for item in Path(args[1]).iterdir():remote[args[2]+'/'+item.name]=item.read_bytes()
     elif args[:3]==('shell','run-as',package):
      if args[3]=='cp':remote[args[5]]=remote[args[4]]
      elif args[3]=='rm':
       for name in args[4:]:remote.pop(name,None)
     elif args[:3]==('shell','rm','-rf'):
      for key in list(remote):
       if key.startswith(args[3]+'/'):del remote[key]
    def text(self,*args):
     if args[3]=='ls':
      prefix=args[4]+'/';return '\n'.join(k[len(prefix):] for k in remote if k.startswith(prefix))
     if args[3]=='sha256sum':return hashlib.sha256(remote[args[4]]).hexdigest()+' checked'
     if args[3]=='cat':return remote.get(args[4],b'').decode()
     raise AssertionError(args)
   with patch.object(self.prepare,'ROOT',root),patch.object(self.prepare,'Device',Device),patch.object(sys,'argv',['prepare','--manifest',str(manifest),'--selector','highest']),patch('builtins.print'):self.prepare.main()
   prepared=root/'build/native-batch-input/highest/manifest.json'
   staged=json.loads(prepared.read_text())['cases']
   for row in staged:
    policy=(prepared.parent/row['batchTypedDriverPolicy']).read_bytes()
    self.assertEqual(policy,b'prepublished-diagnostic-schedule-v1')
    self.assertEqual(hashlib.sha256(policy).hexdigest(),row['typedDriverPolicySHA256'])
   remote['files/native-batch/highest/status.txt']=b'failed pure-preflight-terminal-witness'
   with patch.object(self.collect,'ROOT',root),patch.object(self.collect,'Device',Device),patch.object(sys,'argv',['collect','--prepared',str(prepared),'--output',str(root/'analysis/p6/output')]),patch('builtins.print'):
    with self.assertRaisesRegex(RuntimeError,'Original batch failed; preserve'):self.collect.main()
 def test_native_early_gate_is_bound_before_typed_callback_and_policy_execution_sha(self):
  runner=(NEXT/'tools/native_batch/NativeBatchRunner.cs').read_text()
  self.assertLess(runner.index('scheduledManual = true;'),runner.index('SCRushFightStart rush = null;'))
  self.assertIn('wire.optRecord == null && !scheduledManual',runner)
  self.assertIn('typedDriverPolicySHA256',runner)
  row=self.staged(self.rows[0]);first={'typedCallbackSHA256':row['typedCallbackSHA256'],'typedDriverSHA256':row['typedDriverSHA256'],'typedDriverPolicySHA256':row['typedDriverPolicySHA256'],'manualControlSource':row['manualControlSource']}
  self.collect.validate_typed_execution(row,first)
  for field in ['typedCallbackSHA256','typedDriverSHA256','typedDriverPolicySHA256','manualControlSource']:
   missing=dict(first);missing.pop(field)
   with self.assertRaises(ValueError):self.collect.validate_typed_execution(row,missing)
