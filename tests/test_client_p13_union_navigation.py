"""Verify the scoped Union actor rebind proposal against original CIL; no APK/phone operations."""
import contextlib
import io
from pathlib import Path
import types
import unittest
import zipfile
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT=Path(__file__).resolve().parents[1]

class P13UnionNavigationCILTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        with zipfile.ZipFile(ROOT/'apks/original/10056262.apk') as archive:
            cls.original=archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
        source=(ROOT/'tools/patch_client.py').read_text()
        # Exercise the reviewable proposal until root integrates this shared file; afterwards use it directly.
        if "union_rebind = " not in source:
            patch=(ROOT/'analysis/p13/client-guild-room-navigation-integration.patch').read_text().splitlines()
            inserted='\n'.join(line[1:] for line in patch if line.startswith('+') and not line.startswith('+++'))+'\n'
            marker='        # The retained Transform adds one stack slot during SamplePosition.\n'
            if source.count(marker)!=1:raise AssertionError('Original navigation insertion boundary moved')
            source=source.replace(marker,inserted+marker)
        module=types.ModuleType('p13_navigation_proposal')
        module.__file__=str(ROOT/'tools/patch_client.py')
        exec(compile(source,module.__file__,'exec'),module.__dict__)
        with contextlib.redirect_stdout(io.StringIO()):
            cls.enabled=module.patch_dll(cls.original,'http://127.0.0.1:18088/sao/',repair_copy_navigation=True)
            cls.disabled=module.patch_dll(cls.original,'http://127.0.0.1:18088/sao/',repair_copy_navigation=False)

    def method(self,raw,owner,name):
        pe=dnfile.dnPE(data=raw)
        row=next(m.row for t in pe.net.mdtables.TypeDef if str(t.TypeName)==owner for m in t.MethodList if str(m.row.Name)==name)
        body=read_method_body_from_bytes(pe.get_data(row.Rva))
        method_names={m.row_index:str(t.TypeName)+'::'+str(m.row.Name) for t in pe.net.mdtables.TypeDef for m in t.MethodList}
        return pe,body,method_names

    def test_original_callback_then_guarded_rebind_before_kills_and_minimap(self):
        _,body,names=self.method(self.enabled,'RoomNetUnion','SCCurrentPosition')
        calls=[names.get(i.operand.value&0xffffff) for i in body.instructions if i.opcode.name in ['call','callvirt'] and i.operand.value>>24==6]
        self.assertEqual(calls.count('RoomCharactor::ResetPosition'),1)
        self.assertLess(calls.index('RoomNetBase::SCCurrentPosition'),calls.index('RoomCharactor::ResetPosition'))
        self.assertLess(calls.index('RoomCharactor::ResetPosition'),calls.index('RoomCharactor::SetKillNum'))
        reset=next(i for i in body.instructions if i.opcode.name=='callvirt' and names.get(i.operand.value&0xffffff)=='RoomCharactor::ResetPosition')
        before=[i for i in body.instructions if i.offset<reset.offset][-3:]
        self.assertEqual([i.opcode.name for i in before],['ldloc.0','brfalse.s','ldloc.0'])
        self.assertEqual(before[1].operand,reset.offset+reset.size)
        self.assertFalse(body.exception_handlers)

    def test_disabled_navigation_flag_leaves_original_union_body(self):
        _,original,_=self.method(self.original,'RoomNetUnion','SCCurrentPosition')
        _,disabled,_=self.method(self.disabled,'RoomNetUnion','SCCurrentPosition')
        self.assertEqual(original.get_instruction_bytes()[:original.code_size],disabled.get_instruction_bytes()[:disabled.code_size])

    def test_generic_room_callback_and_original_position_assignment_unchanged(self):
        _,original,_=self.method(self.original,'RoomNetBase','SCCurrentPosition')
        _,enabled,_=self.method(self.enabled,'RoomNetBase','SCCurrentPosition')
        self.assertEqual(original.get_instruction_bytes()[:original.code_size],enabled.get_instruction_bytes()[:enabled.code_size])

if __name__=='__main__':unittest.main()
