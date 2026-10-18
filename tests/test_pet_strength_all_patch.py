"""Execute the original and patched StrengthAll callback IL, including finally."""
from pathlib import Path
import unittest

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

from tools.patch_pet_strength_all import patch_pet_strength_all
from tools.native_batch.build import verify

ROOT = Path(__file__).resolve().parents[1]
OWNER = '<WaitSendMsg>c__Iterator121'
METHOD = '<>m__1A4'


class StrengthAllIL:
    def __init__(self, pe, raw):
        self.pe, self.raw = pe, raw
        self.methods = {(str(t.TypeName), str(m.row.Name)): m.row
                        for t in pe.net.mdtables.TypeDef for m in t.MethodList}

    def body(self, owner=OWNER, method=METHOD):
        return read_method_body_from_bytes(self.pe.get_data(self.methods[owner, method].Rva))

    def member(self, token):
        table = {0x06: self.pe.net.mdtables.MethodDef, 0x0a: self.pe.net.mdtables.MemberRef,
                 0x04: self.pe.net.mdtables.Field}[token.value >> 24]
        return str(table.rows[(token.value & 0xffffff)-1].Name)

    def run(self, count, success=True, throw_update=None):
        equips = [{'guid': 'equip-' + str(i)} for i in range(count)]
        wallet = {'coin': 720}
        env = {'playerData': wallet, 'deletes': [], 'updates': [], 'refresh': 0,
               'popups': [], 'disposed': 0, 'getEnumerator': 0}
        response = {'petEquip': equips, 'cost': [{'id': 'coin', 'num': 120}]}
        args = [{'<>f__this': {'petData': object()}}, {'kBody': response}, success]
        body = self.body()
        instructions = {i.offset: i for i in body.instructions}
        stack, locals_ = [], {}
        cursor = body.instructions[0].offset
        pending_leave, pending_exception = None, None
        steps = 0
        handler = body.exception_handlers[0]
        try_start, try_end = handler.try_start + body.header_size, handler.try_end + body.header_size
        while True:
            steps += 1
            if steps > 1000:
                raise AssertionError('Callback loop did not finish')
            ins = instructions[cursor]
            op, next_cursor = ins.opcode.name, cursor + ins.size
            if op.startswith('ldarg.'):
                stack.append(args[int(op[-1])])
            elif op.startswith('ldloc.'):
                stack.append(locals_[int(op[-1])])
            elif op.startswith('stloc.'):
                locals_[int(op[-1])] = stack.pop()
            elif op == 'ldloca.s':
                stack.append(locals_[ins.operand.index])
            elif op == 'ldfld':
                stack.append(stack.pop()[self.member(ins.operand)])
            elif op == 'ldsfld':
                stack.append(env[self.member(ins.operand)])
            elif op == 'isinst':
                type_name = str(self.pe.net.mdtables.TypeDef.rows[(ins.operand.value & 0xffffff)-1].TypeName)
                if type_name != 'SCPetEquipStrengthAll' or stack[-1] is not response:
                    raise AssertionError('Unexpected callback response type')
            elif op == 'ldstr':
                stack.append(self.pe.net.user_strings.get(ins.operand.value & 0xffffff).value)
            elif op == 'ldc.r4':
                stack.append(ins.operand)
            elif op == 'box':
                pass
            elif op in ('call', 'callvirt'):
                name = self.member(ins.operand)
                if name in ('get_petEquip', 'get_cost'):
                    stack.append(stack.pop()[name[4:]])
                elif name == 'GetEnumerator':
                    stack.append({'values': stack.pop(), 'index': -1})
                    env['getEnumerator'] += 1
                elif name == 'get_Current':
                    enum = stack.pop()
                    stack.append(enum['values'][enum['index']])
                elif name == 'MoveNext':
                    enum = stack.pop()
                    enum['index'] += 1
                    stack.append(enum['index'] < len(enum['values']))
                elif name == 'Dispose':
                    stack.pop()
                    env['disposed'] += 1
                elif name == 'UpdateEquip':
                    equip, _pet = stack.pop(), stack.pop()
                    if len(env['updates']) == throw_update:
                        if not try_start <= cursor < try_end:
                            raise AssertionError('UpdateEquip left the original finally region')
                        pending_exception = RuntimeError('UpdateEquip probe')
                        stack.clear()
                        next_cursor = handler.handler_start + body.header_size
                    else:
                        env['updates'].append(equip['guid'])
                elif name == 'Refresh':
                    stack.pop()
                    env['refresh'] += 1
                elif name == 'Delete':
                    cost, player = stack.pop(), stack.pop()
                    env['deletes'].append(cost)
                    for item in cost:
                        player[item['id']] -= item['num']
                elif name == 'GetText':
                    stack.append(stack.pop())
                elif name == 'CreatePopupTextBox':
                    delay, message = stack.pop(), stack.pop()
                    env['popups'].append((message, delay))
                else:
                    raise AssertionError(('Unmodeled actual callback call', name))
            elif op in ('br', 'br.s'):
                next_cursor = ins.operand
            elif op in ('brtrue', 'brfalse'):
                if bool(stack.pop()) == (op == 'brtrue'):
                    next_cursor = ins.operand
            elif op == 'leave':
                self.assert_empty(stack)
                pending_leave = ins.operand
                next_cursor = handler.handler_start + body.header_size
            elif op == 'endfinally':
                self.assert_empty(stack)
                if pending_exception is not None:
                    env['exception'] = str(pending_exception)
                    return env
                if pending_leave is None:
                    raise AssertionError('Finally was entered without leave/exception')
                next_cursor = pending_leave
                pending_leave = None
            elif op == 'ret':
                self.assert_empty(stack)
                return env
            else:
                raise AssertionError(('Unmodeled actual callback opcode', op))
            if len(stack) > body.max_stack:
                raise AssertionError('Callback exceeds declared stack bound')
            cursor = next_cursor

    @staticmethod
    def assert_empty(stack):
        if stack:
            raise AssertionError('Callback evaluation stack is not balanced')


class PetStrengthAllPatchTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.raw = (ROOT / 'analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll').read_bytes()
        original_pe = dnfile.dnPE(data=cls.raw)
        data = bytearray(cls.raw)
        cls.proof = patch_pet_strength_all(original_pe, cls.raw, data)
        cls.patched = bytes(data)
        cls.before = StrengthAllIL(original_pe, cls.raw)
        cls.after = StrengthAllIL(dnfile.dnPE(data=cls.patched), cls.patched)

    def test_success_zero_one_two_three_six_equipment_deducts_once(self):
        for count in (0, 1, 2, 3, 6):
            with self.subTest(count=count):
                before, after = self.before.run(count), self.after.run(count)
                self.assertEqual(len(before['deletes']), count)
                self.assertEqual(before['playerData']['coin'], 720 - count * 120)
                self.assertEqual(len(after['deletes']), 1)
                self.assertEqual(after['playerData']['coin'], 600)
                for key in ('updates', 'refresh', 'popups', 'disposed', 'getEnumerator'):
                    self.assertEqual(before[key], after[key], key)
                self.assertEqual(after['updates'], ['equip-' + str(i) for i in range(count)])
                self.assertEqual(after['refresh'], count)
                self.assertEqual(after['popups'], [('strSuccess', 0.0)] * count)
                self.assertEqual(after['disposed'], 1)

    def test_false_reply_leaves_wallet_and_equipment_unchanged(self):
        for count in (0, 1, 2, 3, 6):
            with self.subTest(count=count):
                before, after = self.before.run(count, False), self.after.run(count, False)
                self.assertEqual(before, after)
                self.assertEqual(after['playerData']['coin'], 720)
                self.assertEqual(after['deletes'], [])
                self.assertEqual(after['updates'], [])
                self.assertEqual(after['refresh'], 0)
                self.assertEqual(after['popups'], [])
                self.assertEqual(after['disposed'], 0)

    def test_update_exception_still_disposes_original_enumerator(self):
        for fail_at in (0, 1, 2):
            with self.subTest(fail_at=fail_at):
                before = self.before.run(3, throw_update=fail_at)
                after = self.after.run(3, throw_update=fail_at)
                self.assertEqual(after['disposed'], 1)
                self.assertEqual(after['exception'], 'UpdateEquip probe')
                self.assertEqual(len(after['deletes']), 1)
                for key in ('updates', 'refresh', 'popups', 'disposed', 'exception'):
                    self.assertEqual(before[key], after[key], key)

    def test_only_callback_rotation_branches_and_try_start_change(self):
        before, after = self.before.body(), self.after.body()
        self.assertEqual((before.header_size, before.code_size, before.max_stack, before.local_var_sig_tok.value),
                         (after.header_size, after.code_size, after.max_stack, after.local_var_sig_tok.value))
        self.assertEqual(len(self.raw), len(self.patched))
        row = self.before.methods[OWNER, METHOD]
        start = self.before.pe.get_offset_from_rva(row.Rva)
        section = (start + before.header_size + before.code_size + 3) & ~3
        allowed = set(range(start + 0x1e, start + 0x63)) | set(range(start + 0x7f, start + 0x83)) | set(range(section + 8, section + 16))
        changed = {i for i, (a, b) in enumerate(zip(self.raw, self.patched)) if a != b}
        self.assertTrue(changed)
        self.assertFalse(changed - allowed)
        old_handler, new_handler = before.exception_handlers[0], after.exception_handlers[0]
        self.assertEqual((old_handler.try_start, new_handler.try_start), (30, 46))
        self.assertEqual((new_handler.try_end, new_handler.handler_start, new_handler.handler_end), (124, 124, 136))
        self.assertEqual(old_handler.exception_type, new_handler.exception_type)
        self.assertEqual(self.raw[start + 0x88:start + 0x94], self.patched[start + 0x88:start + 0x94])
        self.assertEqual(self.proof['costCallsPerSuccess'], 1)

    def test_original_coroutine_animation_and_all_547_engine_bodies_unchanged(self):
        for owner, method in ((OWNER, 'MoveNext'), ('PetInfoEquipTab', 'ShowEffect')):
            with self.subTest(method=method):
                old_row, new_row = self.before.methods[owner, method], self.after.methods[owner, method]
                old_body, new_body = self.before.body(owner, method), self.after.body(owner, method)
                self.assertEqual(self.before.pe.get_data(old_row.Rva, old_body.size),
                                 self.after.pe.get_data(new_row.Rva, new_body.size))
        self.assertEqual(verify(self.raw, self.patched), 547)


if __name__ == '__main__':
    unittest.main()
