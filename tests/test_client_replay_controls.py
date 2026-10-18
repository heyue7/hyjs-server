"""Verify the optional original native replay entry without changing its request path."""
import contextlib
import io
from pathlib import Path
import unittest

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools.patch_client import patch_dll


class NativeReplayEntryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.original = Path('analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll').read_bytes()
        with contextlib.redirect_stdout(io.StringIO()):
            cls.normal = patch_dll(cls.original, 'http://127.0.0.1:18088/sao/')
            cls.enabled = patch_dll(cls.original, 'http://127.0.0.1:18088/sao/', demo_replay_controls=True)

    def activation(self, raw):
        pe = dnfile.dnPE(data=raw)
        owner = next(t for t in pe.net.mdtables.TypeDef.rows if str(t.TypeName) == 'PlaybackKit')
        row = next(m.row for m in owner.MethodList if str(m.row.Name) == 'Start')
        body = read_method_body_from_bytes(pe.get_data(row.Rva))
        stack = []
        active = None
        for instruction in body.instructions:
            name = instruction.opcode.name
            if name == 'ldarg.0':
                stack.append('component')
            elif name == 'call':
                self.assertEqual(stack.pop(), 'component')
                stack.append('gameObject')
            elif name in ('ldc.i4.0', 'ldc.i4.1'):
                stack.append(name == 'ldc.i4.1')
            elif name == 'callvirt':
                active = stack.pop()
                self.assertEqual(stack.pop(), 'gameObject')
            elif name == 'ret':
                self.assertEqual(stack, [])
            else:
                self.fail('Unexpected original replay activation instruction')
        return active

    def test_default_hidden_and_optional_visible_native_entry(self):
        self.assertFalse(self.activation(self.original))
        self.assertFalse(self.activation(self.normal))
        self.assertTrue(self.activation(self.enabled))

    def test_only_visibility_byte_changes_and_native_protocol_remains(self):
        self.assertEqual(len(self.normal), len(self.enabled))
        diff = [i for i, (a, b) in enumerate(zip(self.normal, self.enabled)) if a != b]
        self.assertEqual(len(diff), 1)
        self.assertEqual((self.normal[diff[0]], self.enabled[diff[0]]), (0x16, 0x17))

    def test_daily_copy_entry_always_refreshes_original_state_without_changing_handlers(self):
        def read(raw):
            pe = dnfile.dnPE(data=raw)
            owner = next(t for t in pe.net.mdtables.TypeDef.rows if str(t.TypeName) == 'ActivityCopyPage')
            row = next(m.row for m in owner.MethodList if str(m.row.Name) == 'DoOpen')
            start = pe.get_offset_from_rva(row.Rva)
            return start, read_method_body_from_bytes(pe.get_data(row.Rva))
        old_start, old = read(self.original)
        new_start, new = read(self.normal)
        old_bytes = self.original[old_start:old_start + old.header_size + old.code_size]
        new_bytes = self.normal[new_start:new_start + new.header_size + new.code_size]
        self.assertEqual(old_bytes[:0x32], new_bytes[:0x32])
        self.assertEqual(old_bytes[0x38:], new_bytes[0x38:])
        self.assertEqual(new_bytes[0x32:0x38], b'\x16' + b'\0' * 5)
        instructions = {i.offset: i for i in new.instructions}
        self.assertEqual(instructions[0x32].opcode.name, 'ldc.i4.0')
        self.assertEqual(instructions[0x38].opcode.name, 'brfalse')
        self.assertEqual(instructions[0x38].operand, 0x3e)
        self.assertGreater(len(old.exception_handlers), 0)
        self.assertEqual(len(old.exception_handlers), len(new.exception_handlers))


if __name__ == '__main__':
    unittest.main()
