"""Playback completes its synchronous scene return without touching cleared actors."""
import contextlib
import io
from pathlib import Path
import unittest

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools.patch_client import patch_dll


class NativePlaybackResultTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.original = Path('analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll').read_bytes()
        with contextlib.redirect_stdout(io.StringIO()):
            cls.normal = patch_dll(cls.original, 'http://127.0.0.1:18088/sao/')
            cls.enabled = patch_dll(cls.original, 'http://127.0.0.1:18088/sao/', repair_copy_navigation=True)

    def method(self, raw):
        pe = dnfile.dnPE(data=raw)
        owner = next(t for t in pe.net.mdtables.TypeDef if str(t.TypeName) == 'ResultState')
        row = next(m.row for m in owner.MethodList if str(m.row.Name) == 'Enter')
        start = pe.get_offset_from_rva(row.Rva)
        return pe, start, read_method_body_from_bytes(pe.get_data(row.Rva))

    def transition(self, raw, owner):
        pe = dnfile.dnPE(data=raw)
        kind = next(t for t in pe.net.mdtables.TypeDef if str(t.TypeName) == owner)
        method = next(m.row for m in kind.MethodList if str(m.row.Name) == 'MoveNext')
        return pe, read_method_body_from_bytes(pe.get_data(method.Rva))

    def test_playback_preserves_source_scene_and_restores_original_room_before_ui(self):
        for owner, offset, field, effect in (
            ('<_RoomToBattleRecruit>c__Iterator5E', 0x59, 'curSceneType', 'stsfld'),
            ('<_BattleRecruitToRoom>c__Iterator5F', 0x66, 'awakeCopyPlaybackFromScene', 'call'),
        ):
            pe, body = self.transition(self.enabled, owner)
            items = {i.offset: i for i in body.instructions}
            self.assertEqual(items[offset].opcode.name, 'ldsfld')
            flag = pe.net.mdtables.Field.rows[(items[offset+5].operand.value & 0xffffff)-1]
            self.assertEqual(str(flag.Name), 'isPlayBack')
            guard = items[offset+10]
            self.assertEqual(guard.opcode.name, 'brfalse.s')
            source = items[offset+12]
            self.assertEqual(source.opcode.name, 'ldsfld')
            self.assertEqual(str(pe.net.mdtables.Field.rows[(source.operand.value & 0xffffff)-1].Name), field)
            original = items[guard.operand]
            self.assertEqual(original.opcode.name, 'ldc.i4.1' if effect == 'stsfld' else 'ldarg.0')
            if effect == 'call':
                restore = items[offset+23]
                self.assertEqual(str(pe.net.mdtables.MethodDef.rows[(restore.operand.value & 0xffffff)-1].Name), 'Switch')
                calls = [i for i in body.instructions if i.opcode.name == 'callvirt' and i.operand.value >> 24 == 6]
                ui = next(i for i in calls if str(pe.net.mdtables.MethodDef.rows[(i.operand.value & 0xffffff)-1].Name) == 'ChangeUIBack')
                self.assertLess(restore.offset, ui.offset)
            boundaries = set(items)
            for ins in body.instructions:
                if 'BrTarget' in ins.opcode.operand_type.name:
                    self.assertIn(ins.operand, boundaries)
                elif ins.opcode.name == 'switch':
                    self.assertTrue(set(ins.operand).issubset(boundaries))

    def test_default_recruit_room_transitions_unchanged(self):
        for owner in ('<_RoomToBattleRecruit>c__Iterator5E', '<_BattleRecruitToRoom>c__Iterator5F'):
            _, before = self.transition(self.original, owner)
            _, after = self.transition(self.normal, owner)
            self.assertEqual(before.get_instruction_bytes(), after.get_instruction_bytes())

    def test_playback_calls_original_result_then_returns_before_old_actor_loops(self):
        pe, start, body = self.method(self.enabled)
        items = {i.offset: i for i in body.instructions}
        self.assertEqual(items[0x0c].opcode.name, 'ldsfld')
        self.assertEqual(items[0x11].opcode.name, 'ldfld')
        flag = pe.net.mdtables.Field.rows[(items[0x11].operand.value & 0xffffff)-1]
        self.assertEqual(str(flag.Name), 'isPlayBack')
        self.assertEqual((items[0x16].opcode.name, items[0x16].operand), ('brfalse.s', 0x2b))
        self.assertEqual(items[0x27].opcode.name, 'ret')
        self.assertEqual(self.enabled[start+0x18:start+0x27], self.original[start+0x1d5:start+0x1e4])

    def test_nonplayback_logic_and_exception_regions_remain_at_original_offsets(self):
        _, before_start, before = self.method(self.original)
        _, after_start, after = self.method(self.enabled)
        self.assertEqual((before_start, before.header_size, before.code_size, before.max_stack),
                         (after_start, after.header_size, after.code_size, after.max_stack))
        end = before.header_size + before.code_size
        self.assertEqual(self.original[before_start+0x2b:before_start+end], self.enabled[after_start+0x2b:after_start+end])
        shape = lambda b: [(e.exception_type, e.try_start, e.try_end, e.handler_start, e.handler_end, e.filter_start, e.catch_type) for e in b.exception_handlers]
        self.assertEqual(shape(before), shape(after))

    def test_default_result_and_all_native_simulation_methods_unchanged(self):
        _, old_start, old = self.method(self.original)
        _, start, body = self.method(self.normal)
        self.assertEqual(self.original[old_start:old_start+old.size], self.normal[start:start+body.size])
        source = dnfile.dnPE(data=self.original)
        patched = dnfile.dnPE(data=self.enabled)
        for owner in source.net.mdtables.TypeDef:
            if not str(owner.TypeNamespace).startswith('Battle.Engine'):
                continue
            for entry in owner.MethodList:
                row = entry.row
                if not row.Rva:
                    continue
                original_body = read_method_body_from_bytes(source.get_data(row.Rva))
                revised = read_method_body_from_bytes(patched.get_data(row.Rva))
                self.assertEqual(original_body.get_instruction_bytes(), revised.get_instruction_bytes(), str(owner.TypeName)+'::'+str(row.Name))


if __name__ == '__main__':
    unittest.main()
