"""Check ownership and exception boundaries for native VIP product cards."""
import contextlib
import io
from pathlib import Path
import struct
import unittest

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools.patch_client import patch_dll


class VIPShopClientTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        raw = (Path(__file__).resolve().parents[1] /
               'analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll').read_bytes()
        cls.original = dnfile.dnPE(data=raw)
        with contextlib.redirect_stdout(io.StringIO()):
            patched = patch_dll(raw, 'http://192.168.31.253:8088/sao/')
        cls.patched = dnfile.dnPE(data=patched)

    def callback(self, pe):
        owner = next(t for t in pe.net.mdtables.TypeDef.rows if str(t.TypeName) == 'TabVipShop')
        row = next(m.row for m in owner.MethodList if str(m.row.Name) == 'OnMsgCallback')
        return read_method_body_from_bytes(pe.get_data(row.Rva))

    def method_name(self, pe, token):
        table = {6: 'MethodDef', 10: 'MemberRef', 43: 'MethodSpec'}[token >> 24]
        row = getattr(pe.net.mdtables, table).rows[(token & 0xffffff) - 1]
        if table == 'MethodSpec':
            row = row.Method.row
        return str(row.Name)

    def test_card_has_shop_owner_before_visibility_or_initialization(self):
        # Walk actual CIL calls. A malformed response may fail initialization;
        # the card must already belong to the closable shop grid at that point.
        body = self.callback(self.patched)
        calls = [(i.offset, self.method_name(self.patched, i.operand.value))
                 for i in body.instructions if i.opcode.name in ('call', 'callvirt')]
        add = next(offset for offset, name in calls if name == 'AddChild')
        visible = next(offset for offset, name in calls if name == 'SetActive')
        fill = next(offset for offset, name in calls if name == 'SetShopVipItem')
        self.assertLess(add, visible)
        self.assertLess(add, fill)

    def test_original_branches_cleanup_and_purchase_callbacks_survive(self):
        old, new = self.callback(self.original), self.callback(self.patched)
        self.assertEqual(old.code_size, new.code_size)
        self.assertEqual([(h.try_start, h.try_end, h.handler_start, h.handler_end)
                          for h in old.exception_handlers],
                         [(h.try_start, h.try_end, h.handler_start, h.handler_end)
                          for h in new.exception_handlers])
        branches = lambda body: [(i.offset, i.opcode.name, i.operand)
                                 for i in body.instructions
                                 if 'BrTarget' in i.opcode.operand_type.name]
        self.assertEqual(branches(old), branches(new))
        # No network call or replacement purchase callback is introduced.
        calls = lambda pe, body: sorted(self.method_name(pe, i.operand.value)
                                       for i in body.instructions
                                       if i.opcode.name in ('call', 'callvirt', 'ldftn'))
        self.assertEqual(calls(self.original, old), calls(self.patched, new))

    def warehouse(self, boxes, score=0):
        """Run actual guarded CIL, including the original nonempty fill branches."""
        owner = next(t for t in self.patched.net.mdtables.TypeDef.rows
                     if str(t.TypeName) == 'UnionWarehouseTabBox')
        row = next(m.row for m in owner.MethodList if str(m.row.Name) == 'Init')
        body = read_method_body_from_bytes(self.patched.get_data(row.Rva))
        receiver = {'gameObject': {'active': False}, 'cuurentValue': {},
                    'currentPeople': {}, 'currentGet': {}, 'boxCalls': []}
        args = [receiver, boxes, score, 1, 35]
        stack, locals_, cursor = [], {}, 0
        instructions = body.instructions
        positions = {i.offset: n for n, i in enumerate(instructions)}
        for _ in range(500):
            ins = instructions[cursor]
            cursor += 1
            op = ins.opcode.name
            if op == 'ret':
                self.assertEqual(stack, [])
                return receiver
            if op == 'ldarg.s':
                stack.append(args[ins.operand.index])
            elif op.startswith('ldarg.'):
                stack.append(args[int(op.rsplit('.', 1)[1])])
            elif op.startswith('ldloc.'):
                stack.append(locals_[int(op.rsplit('.', 1)[1])])
            elif op.startswith('stloc.'):
                locals_[int(op.rsplit('.', 1)[1])] = stack.pop()
            elif op in ('ldc.r4', 'ldc.i4'):
                stack.append(ins.operand)
            elif op.startswith('ldc.i4.'):
                stack.append(int(op.rsplit('.', 1)[1]))
            elif op == 'ldstr':
                stack.append(self.patched.net.user_strings.get(ins.operand.value & 0xffffff).value)
            elif op == 'ldfld':
                field = self.patched.net.mdtables.Field.rows[(ins.operand.value & 0xffffff) - 1]
                stack.append(stack.pop()[str(field.Name)])
            elif op == 'conv.r4':
                stack.append(float(stack.pop()))
            elif op == 'box':
                pass
            elif op in ('mul', 'sub', 'add'):
                right, left = stack.pop(), stack.pop()
                stack.append(left * right if op == 'mul' else left - right if op == 'sub' else left + right)
            elif 'BrTarget' in ins.opcode.operand_type.name:
                branch = op[:-2] if op.endswith('.s') else op
                take = True
                if branch in ('brtrue', 'brfalse'):
                    take = bool(stack.pop()) == (branch == 'brtrue')
                elif branch != 'br':
                    right, left = stack.pop(), stack.pop()
                    take = left >= right if branch == 'bge.un' else left != right if branch == 'bne.un' else None
                    self.assertIsNotNone(take, branch)
                if take:
                    cursor = positions[ins.operand]
            elif op in ('call', 'callvirt'):
                name = self.method_name(self.patched, ins.operand.value)
                if name == 'get_gameObject':
                    stack.append(stack.pop()['gameObject'])
                elif name == 'SetActive':
                    value = stack.pop()
                    stack.pop()['active'] = bool(value)
                elif name == 'get_Count':
                    stack.append(len(stack.pop()))
                elif name == 'GetText':
                    stack.pop()
                    stack.append('{0}')
                elif name == 'Format':
                    value, template = stack.pop(), stack.pop()
                    stack.append(template.format(value))
                elif name == 'Concat':
                    values = [stack.pop() for _ in range(3)][::-1]
                    stack.append(''.join(str(v) for v in values))
                elif name == 'set_text':
                    value = stack.pop()
                    stack.pop()['text'] = value
                elif name == 'get_Item':
                    index, source = stack.pop(), stack.pop()
                    stack.append(source[index])
                elif name == 'get_score':
                    stack.append(stack.pop()['score'])
                elif name == 'set_fillAmount':
                    value = stack.pop()
                    stack.pop()['fill'] = value
                elif name == 'InitBox':
                    value = stack.pop()
                    stack.pop()['boxCalls'].append(value)
                else:
                    self.fail('Unexpected original warehouse call ' + name)
            else:
                self.fail('Unexpected original warehouse instruction ' + op)
        self.fail('Warehouse CIL did not terminate')

    def test_empty_unrestored_box_directory_does_not_abort_asset_callback(self):
        result = self.warehouse([])
        self.assertFalse(result['gameObject']['active'])
        self.assertEqual(result['cuurentValue']['text'], '0')
        self.assertEqual(result['currentPeople']['text'], '1/35')
        self.assertEqual(result['boxCalls'], [])

    def test_original_nonempty_box_progress_and_visibility_survive(self):
        boxes = [{'score': 250}, {'score': 400}, {'score': 500}, {'score': 600}]
        for score, expected in [(0, 0), (250, .06), (400, .37), (500, .677), (600, 1), (300, .1633333), (450, .5275)]:
            with self.subTest(score=score):
                result = self.warehouse(boxes, score)
                self.assertTrue(result['gameObject']['active'])
                self.assertEqual(result['boxCalls'], [boxes])
                self.assertAlmostEqual(result['currentGet']['fill'], expected, places=5)


if __name__ == '__main__':
    unittest.main()
