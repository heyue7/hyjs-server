"""Size-preserving navigation repair preserves NavMesh sample failure and native APIs."""
import contextlib
import io
from pathlib import Path
import struct
import unittest

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools.patch_client import patch_dll


class ClientNavigationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.raw = (Path(__file__).resolve().parents[1] /
                   'analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll').read_bytes()
        with contextlib.redirect_stdout(io.StringIO()):
            cls.patched = patch_dll(cls.raw, 'http://127.0.0.1:18088/sao/', repair_copy_navigation=True)
            cls.baseline = patch_dll(cls.raw, 'http://127.0.0.1:18088/sao/')

    def method(self, raw, owner, name):
        pe = dnfile.dnPE(data=raw)
        target = next(m.row for t in pe.net.mdtables.TypeDef
                      if str(t.TypeName) == owner for m in t.MethodList if str(m.row.Name) == name)
        offset = pe.get_offset_from_rva(target.Rva)
        return pe, offset, read_method_body_from_bytes(pe.get_data(target.Rva))

    def resolve(self, pe, instruction):
        token = instruction.operand.value
        if token >> 24 == 6:
            return str(pe.net.mdtables.MethodDef.rows[(token & 0xffffff)-1].Name)
        return str(pe.net.mdtables.MemberRef.rows[(token & 0xffffff)-1].Name)

    def test_size_metadata_exception_and_native_call_sequence(self):
        self.assertEqual(len(self.raw), len(self.patched))
        _, original_offset, original = self.method(self.raw, 'RCMoveNavigation', 'ResetPosition')
        pe, offset, repaired = self.method(self.patched, 'RCMoveNavigation', 'ResetPosition')
        self.assertEqual((offset, repaired.code_size, repaired.header_size),
                         (original_offset, original.code_size, original.header_size))
        self.assertFalse(original.exception_handlers)
        self.assertFalse(repaired.exception_handlers)
        self.assertGreaterEqual(struct.unpack_from('<H', self.patched, offset+2)[0], 5)
        calls = [self.resolve(pe, i) for i in repaired.instructions if i.opcode.name in ('call', 'callvirt')]
        self.assertEqual(calls, ['get_transform', 'get_position', 'SamplePosition',
                                'get_position', 'set_position', 'GetCurrentSpeed', 'SetSpeed'])
        speed_pe, _, speed = self.method(self.patched, 'RCMoveNavigation', 'SetSpeed')
        enabled = [i for i in speed.instructions if i.opcode.name == 'callvirt'
                   and self.resolve(speed_pe, i) == 'set_enabled']
        self.assertEqual(len(enabled), 2)

    def test_rebind_restores_original_native_enabled_state(self):
        pe, offset, speed = self.method(self.patched, 'RCMoveNavigation', 'SetSpeed')
        _, _, original = self.method(self.raw, 'RCMoveNavigation', 'SetSpeed')
        self.assertEqual(speed.code_size, original.code_size)
        self.assertFalse(speed.exception_handlers)
        self.assertGreaterEqual(struct.unpack_from('<H', self.patched, offset+2)[0], 4)
        calls = [self.resolve(pe, i) for i in speed.instructions if i.opcode.name in ('call', 'callvirt')]
        self.assertEqual(calls, ['get_agent', 'set_speed', 'get_enabled', 'get_agent',
                                'set_enabled', 'set_enabled'])
        opcodes = [i.opcode.name for i in speed.instructions]
        # Original enabled value remains on the stack until the final setter;
        # no hard-coded true value can enable a previously disabled agent.
        self.assertNotIn('ldc.i4.1', opcodes)
        self.assertEqual(opcodes.count('ldc.i4.0'), 1)
        self.assertEqual(opcodes.count('dup'), 2)

    def test_false_sample_branch_only_discards_transform_and_returns(self):
        _, _, repaired = self.method(self.patched, 'RCMoveNavigation', 'ResetPosition')
        instructions = {i.offset: i for i in repaired.instructions}
        branch = next(i for i in repaired.instructions if i.opcode.name == 'brfalse.s')
        self.assertEqual(instructions[branch.operand].opcode.name, 'pop')
        self.assertEqual(instructions[branch.operand+1].opcode.name, 'ret')
        self.assertTrue(any(i.opcode.name == 'ldc.i4.s' and i.operand == 10 for i in repaired.instructions))
        self.assertTrue(any(i.opcode.name == 'conv.r4' for i in repaired.instructions))
        self.assertTrue(any(i.opcode.name == 'ldc.i4.m1' for i in repaired.instructions))

    def test_default_build_retains_original_navigation_body(self):
        _, offset, original = self.method(self.raw, 'RCMoveNavigation', 'ResetPosition')
        length = original.header_size + original.code_size
        self.assertEqual(self.raw[offset:offset+length], self.baseline[offset:offset+length])
        _, speed_offset, speed = self.method(self.raw, 'RCMoveNavigation', 'SetSpeed')
        length = speed.header_size + speed.code_size
        self.assertEqual(self.raw[speed_offset:speed_offset+length],
                         self.baseline[speed_offset:speed_offset+length])


if __name__ == '__main__':
    unittest.main()
