"""Pure isolated next caller binding; no phone, SQL or original source writes."""
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[1]
NEXT=(ROOT if 'RunRushManualAutoStart' in (ROOT/'tools/native_batch/NativeBatchRunner.cs').read_text() else ROOT/'analysis/p6/native-manual-auto-driver-next-v161')
sys.path.insert(0,str(ROOT/'tools'))

def load(name):
    spec=importlib.util.spec_from_file_location('manual_auto_next_'+name,NEXT/'tools/native_batch'/ (name+'.py'))
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);module.ROOT=ROOT;return module

class NativeManualAutoDriverTests(unittest.TestCase):
    def setUp(self):
        self.prepare=load('prepare');self.collect=load('collect')
        self.manifest=ROOT/'analysis/p6/actual-phone-rush-manual-auto-input-v161/manifest.json'
        self.row=json.loads(self.manifest.read_text())['cases'][0]

    def test_original_first_success_clock_is_required_without_rewriting(self):
        self.prepare.load_rush_typed_source(self.row,self.manifest,'highest')
        for delay in [0,30.1,30.2,float('nan'),111]:
            with self.assertRaises(RuntimeError):self.prepare.load_rush_typed_source(dict(self.row,manualAutoStartRoundTime=delay),self.manifest,'highest')

    def test_recorded_and_automatic_source_modes_are_unchanged(self):
        for folder,selector in [('actual-phone-rush-recorded-input-v157','highest'),('actual-local-p7-rush-inputs-v104','pve-auto')]:
            manifest=ROOT/'analysis/p6'/folder/'manifest.json'
            for row in json.loads(manifest.read_text())['cases']:self.prepare.load_rush_typed_source(row,manifest,selector)

    def test_manual_control_execution_requires_its_independent_sha(self):
        row=dict(batchTypedCallback='000.rush-start.bin',typedCallbackSHA256='a'*64,batchTypedDriver='000.rush-auto-start.txt',typedDriverSHA256='b'*64)
        self.collect.validate_typed_execution(row,dict(typedCallbackSHA256='a'*64,typedDriverSHA256='b'*64))
        with self.assertRaisesRegex(ValueError,'manual control execution'):self.collect.validate_typed_execution(row,dict(typedCallbackSHA256='a'*64))

    def test_prepare_stages_exact_observed_control_and_callback(self):
        with tempfile.TemporaryDirectory()as folder:
            root=Path(folder).resolve();source=root/'analysis/p6/source';source.mkdir(parents=True)
            for key in ['wire','typedCallback','frozenInput','originalEnd']:(source/self.row[key]).write_bytes((self.manifest.parent/self.row[key]).read_bytes())
            manifest=source/'manifest.json';manifest.write_text(json.dumps(dict(cases=[self.row])))
            remote={};package=self.prepare.PACKAGE
            class Device:
                def __init__(self,*args):pass
                def run(self,*args,**kwargs):
                    if args[0]=='push':
                        for item in Path(args[1]).iterdir():remote[args[2]+'/'+item.name]=item.read_bytes()
                    elif args[:3]==('shell','run-as',package):
                        if args[3]=='cp':remote[args[5]]=remote[args[4]]
                        elif args[3]=='rm':remote.pop(args[4],None)
                    elif args[:3]==('shell','rm','-rf'):
                        for key in list(remote):
                            if key.startswith(args[3]+'/'):del remote[key]
                def text(self,*args):
                    if args[3]=='ls':
                        prefix=args[4]+'/';return '\n'.join(k[len(prefix):]for k in remote if k.startswith(prefix))
                    if args[3]=='sha256sum':return hashlib.sha256(remote[args[4]]).hexdigest()+' checked'
                    if args[3]=='cat':return remote[args[4]].decode()
                    raise AssertionError(args)
            with patch.object(self.prepare,'ROOT',root),patch.object(self.prepare,'Device',Device),patch.object(sys,'argv',['prepare','--manifest',str(manifest),'--selector','highest']),patch('builtins.print'):self.prepare.main()
            staged=json.loads((root/'build/native-batch-input/highest/manifest.json').read_text())['cases'][0]
            raw=remote['files/native-batch/highest/'+staged['batchTypedDriver']]
            self.assertEqual(raw,b'30.100000000000158');self.assertEqual(hashlib.sha256(raw).hexdigest(),staged['typedDriverSHA256'])
            self.assertEqual(remote['files/native-batch/highest/'+staged['batchTypedCallback']],(self.manifest.parent/self.row['typedCallback']).read_bytes())
