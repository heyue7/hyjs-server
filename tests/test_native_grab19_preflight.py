"""Actual Grab19 prepare/collector preflight; no device, Engine, SQL or fake result."""
import hashlib,importlib.util,json,sys,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
ROOT=next(p for p in Path(__file__).resolve().parents if (p/'hyjs-server/go.mod').is_file() and (p/'server/codec.py').is_file())
NEXT=ROOT/'analysis/p6/native-grab19-next-v183'
sys.path.insert(0,str(ROOT/'tools'))
from server.codec import message

def load(name):
 spec=importlib.util.spec_from_file_location('grab_preflight_'+name,NEXT/'tools/native_batch'/(name+'.py'))
 m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);m.ROOT=ROOT;return m

class GrabRealPreflightTests(unittest.TestCase):
 def setUp(self):
  self.prepare=load('prepare');self.collect=load('collect')
  self.source=ROOT/'analysis/p6/original-grab19-candidate-inputs-v183/manifest.json'
  self.rows=json.loads(self.source.read_bytes())['cases']
 def test_exact_original_callback_and_mode(self):
  for i,row in enumerate(self.rows):
   self.prepare.load_grab_typed_source(row,self.source,'highest')
   wire=message('SCFightPlayback');wire.ParseFromString((self.source.parent/row['wire']).read_bytes())
   staged=dict(row,batchWire='000.pb',batchTypedCallback='000.grab-start.bin')
   self.collect.validate_typed_preflight(staged,wire,'highest','000.pb')
   for changed in [dict(batchTypedCallback='000.arena-result.bin'),dict(batchTypedCallback='000.plunder-start.bin')]:
    with self.assertRaises(RuntimeError):self.collect.validate_typed_preflight(dict(staged,**changed),wire,'highest','000.pb')
   with self.assertRaises(RuntimeError):self.prepare.load_grab_typed_source(row,self.source,'pve-auto')
   with self.assertRaises(RuntimeError):self.prepare.load_grab_typed_source(dict(row,typedCallbackSHA256='0'*64),self.source,'highest')
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
   remote['files/native-batch/highest/status.txt']=b'failed pure-preflight-terminal-witness'
   with patch.object(self.collect,'ROOT',root),patch.object(self.collect,'Device',Device),patch.object(sys,'argv',['collect','--prepared',str(prepared),'--output',str(root/'analysis/p6/output')]),patch('builtins.print'):
    with self.assertRaisesRegex(RuntimeError,'Original batch failed; preserve'):self.collect.main()
