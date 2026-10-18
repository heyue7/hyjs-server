"""Execute compiled compatibility-gate IL and reject original field mutations."""
from pathlib import Path
import itertools
import unittest

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

from tools.native_batch.build import compile_implementation

ROOT = Path(__file__).resolve().parents[1]


class LocalPVECompatibilityGateTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dll = compile_implementation()
        cls.pe = dnfile.dnPE(data=cls.dll.read_bytes())
        cls.methods = {(str(t.TypeName), str(m.row.Name)): m.row
                       for t in cls.pe.net.mdtables.TypeDef for m in t.MethodList}

    def gate(self, values):
        row = self.methods['OriginalHeadlessPlaybackCaller', 'PVECompatibilityGate']
        body = read_method_body_from_bytes(self.pe.get_data(row.Rva))
        instructions = {i.offset: i for i in body.instructions}
        cursor, stack, locals_ = body.instructions[0].offset, [], {}
        while True:
            ins = instructions[cursor]
            op, next_cursor = ins.opcode.name, cursor + ins.size
            if op == 'nop':
                pass
            elif op == 'ldarg.s':
                stack.append(int(values[ins.operand.index]))
            elif op.startswith('ldarg.'):
                stack.append(int(values[int(op[-1])]))
            elif op == 'ldc.i4.0':
                stack.append(0)
            elif op == 'and':
                right, left = stack.pop(), stack.pop()
                stack.append(left & right)
            elif op == 'ceq':
                right, left = stack.pop(), stack.pop()
                stack.append(int(left == right))
            elif op == 'brfalse.s':
                if not stack.pop():
                    next_cursor = ins.operand
            elif op == 'br.s':
                next_cursor = ins.operand
            elif op == 'stloc.0':
                locals_[0] = stack.pop()
            elif op == 'ldloc.0':
                stack.append(locals_[0])
            elif op == 'ret':
                result = bool(stack.pop())
                self.assertEqual(stack, [])
                return result
            else:
                self.fail('Unexpected gate opcode: ' + op)
            cursor = next_cursor

    def test_all_512_gate_combinations_authorize_only_one_complete_fact_set(self):
        approved = (True, True, True, False, True, True, True, True, True)
        for values in itertools.product((False, True), repeat=9):
            with self.subTest(facts=values):
                self.assertEqual(self.gate(values), values == approved)

    def test_compiled_caller_contains_no_original_engine_definitions_or_field_writes(self):
        self.assertFalse(any(str(t.TypeNamespace).startswith('Battle.Engine') for t in self.pe.net.mdtables.TypeDef))
        for (owner, name), row in self.methods.items():
            if not row.Rva:
                continue
            body = read_method_body_from_bytes(self.pe.get_data(row.Rva))
            for ins in body.instructions:
                if ins.opcode.name not in ('stfld', 'stsfld') or ins.operand.value >> 24 != 0x0a:
                    continue
                member = self.pe.net.mdtables.MemberRef.rows[(ins.operand.value & 0xffffff)-1]
                declared = member.Class.row
                namespace = str(getattr(declared, 'TypeNamespace', ''))
                self.assertFalse(namespace.startswith('Battle.Engine'), (owner, name, str(member.Name)))

    def test_recovery_uses_original_public_finish_and_keeps_strict_mode_entrypoints(self):
        row = self.methods['Capture', 'TryLocalNoTargetFinish']
        body = read_method_body_from_bytes(self.pe.get_data(row.Rva))
        original_calls = []
        for ins in body.instructions:
            if ins.opcode.name not in ('call', 'callvirt') or ins.operand.value >> 24 != 0x0a:
                continue
            member = self.pe.net.mdtables.MemberRef.rows[(ins.operand.value & 0xffffff)-1]
            declared = member.Class.row
            namespace = str(getattr(declared, 'TypeNamespace', ''))
            if namespace.startswith('Battle.Engine'):
                original_calls.append((str(getattr(declared, 'TypeName', '')), str(member.Name)))
        self.assertEqual(original_calls.count(('EngineDefault', 'FinishRound')), 1)
        self.assertEqual(set(original_calls), {('EngineData', 'GetCurRoundPlayer'), ('EngineDefault', 'FinishRound')})
        for entry in ('RunOne', 'RunOneWithObservers', 'RunAutomatic', 'RunPVECompatibility'):
            self.assertIn(('OriginalHeadlessPlaybackCaller', entry), self.methods)

    def test_animated_recovery_uses_original_view_message_route_and_read_only_engine_access(self):
        row = self.methods['LocalPVEUICompatibility', 'AfterUpdate']
        body = read_method_body_from_bytes(self.pe.get_data(row.Rva))
        calls = []
        for ins in body.instructions:
            if ins.opcode.name not in ('call', 'callvirt') or ins.operand.value >> 24 != 0x0a:
                continue
            member = self.pe.net.mdtables.MemberRef.rows[(ins.operand.value & 0xffffff)-1]
            declared = member.Class.row
            calls.append((str(getattr(declared, 'TypeNamespace', '')),
                          str(getattr(declared, 'TypeName', '')), str(member.Name)))
        self.assertEqual(calls.count(('Battle.View', 'BattleViewCtrl', 'AskRoundOver')), 1)
        engine_calls = {(owner, name) for namespace, owner, name in calls if namespace.startswith('Battle.Engine')}
        self.assertEqual(engine_calls, {('EngineDefault', 'GetEngineData'), ('EngineData', 'GetCurRoundPlayer')})
        self.assertFalse(any(name in ('SetValue', 'SetAuto', 'DoSkill', 'FinishRound') for _, _, name in calls))


if __name__ == '__main__':
    unittest.main()
