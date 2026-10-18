"""Execute the patched NGUI scaling IL against real and legacy screen sizes."""
import contextlib
import io
from pathlib import Path
import unittest
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

from tools.patch_client import DLL, patch_dll

ROOT = Path(__file__).resolve().parents[1]


class RootScalingIL:
    """Run the decoded UIRoot methods with only screen input and rounding stubbed."""

    def __init__(self, raw):
        self.pe = dnfile.dnPE(data=raw)
        self.methods = {}
        for owner in self.pe.net.mdtables.TypeDef:
            if str(owner.TypeName) == 'UIRoot':
                for method in owner.MethodList:
                    self.methods[str(method.row.Name)] = method.row

    def body(self, name):
        return read_method_body_from_bytes(self.pe.get_data(self.methods[name].Rva))

    def member(self, token):
        table = {4: 'Field', 6: 'MethodDef', 10: 'MemberRef'}[token.value >> 24]
        return getattr(self.pe.net.mdtables, table).rows[(token.value & 0xffffff) - 1]

    def execute(self, name, root, screen):
        # Follow the original branches and arithmetic so changing the wrong
        # constraint or height formula fails without copying the NGUI formula.
        body = self.body(name)
        instructions = {i.offset: i for i in body.instructions}
        cursor = body.instructions[0].offset
        stack, local = [], {}
        for _ in range(200):
            instruction = instructions[cursor]
            opcode, operand = instruction.opcode.name, instruction.operand
            next_cursor = cursor + instruction.size
            if opcode == 'ldarg.0':
                stack.append(root)
            elif opcode.startswith('ldc.i4.'):
                suffix = opcode[7:]
                stack.append(operand if suffix == 's' else -1 if suffix == 'm1' else int(suffix))
            elif opcode in ('ldc.i4', 'ldc.r4'):
                stack.append(operand)
            elif opcode.startswith(('ldloc.', 'stloc.', 'ldloca.')):
                index = operand.index if opcode.endswith('.s') else int(opcode.split('.')[1])
                if opcode.startswith('stloc.'):
                    local[index] = stack.pop()
                else:
                    stack.append(local[index])
            elif opcode == 'ldfld':
                stack.append(stack.pop()[str(self.member(operand).Name)])
            elif opcode in ('conv.r4', 'conv.i4'):
                stack.append(float(stack.pop()) if opcode == 'conv.r4' else int(stack.pop()))
            elif opcode in ('div', 'mul', 'sub', 'add'):
                right, left = stack.pop(), stack.pop()
                stack.append({'div': lambda: left / right, 'mul': lambda: left * right,
                              'sub': lambda: left - right, 'add': lambda: left + right}[opcode]())
            elif opcode in ('call', 'callvirt'):
                method = self.member(operand)
                callee = str(method.Name)
                if operand.value >> 24 == 6 and callee in self.methods:
                    stack.append(self.execute(callee, stack.pop(), screen))
                elif callee == 'get_screenSize':
                    stack.append({'x': float(screen[0]), 'y': float(screen[1])})
                elif callee == 'RoundToInt':
                    stack.append(round(stack.pop()))
                else:
                    raise AssertionError(f'Unexpected scaling call: {callee}')
            elif opcode in ('br', 'br.s'):
                next_cursor = operand
            elif opcode in ('brtrue', 'brfalse'):
                condition = bool(stack.pop())
                if condition == (opcode == 'brtrue'):
                    next_cursor = operand
            elif opcode in ('beq', 'bne.un', 'bge.un', 'ble.un'):
                right, left = stack.pop(), stack.pop()
                matches = {'beq': left == right, 'bne.un': left != right,
                           'bge.un': left >= right, 'ble.un': left <= right}
                if matches[opcode]:
                    next_cursor = operand
            elif opcode == 'switch':
                choice = stack.pop()
                if 0 <= choice < len(operand):
                    next_cursor = operand[choice]
            elif opcode == 'ret':
                result = stack.pop()
                if stack:
                    raise AssertionError(f'Unbalanced scaling stack: {stack}')
                return result
            else:
                raise AssertionError(f'Unexpected scaling IL: {opcode}')
            cursor = next_cursor
        raise AssertionError('Scaling IL did not terminate')


class ClientViewportTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        with zipfile.ZipFile(ROOT / 'apks/original/10056262.apk') as archive:
            cls.original = archive.read(DLL)
        with contextlib.redirect_stdout(io.StringIO()):
            cls.patched = patch_dll(cls.original, 'http://127.0.0.1:18088/sao/')
        cls.old_il = RootScalingIL(cls.original)
        cls.new_il = RootScalingIL(cls.patched)

    @staticmethod
    def root(width=1280, fit_width=True, fit_height=False):
        # Match level0's serialized constrained root and UIAutoSize's width.
        return {'scalingStyle': 1, 'manualWidth': width, 'manualHeight': 640,
                'fitWidth': fit_width, 'fitHeight': fit_height}

    def test_constraint_patch_changes_one_instruction_without_relocating_methods(self):
        old, new = self.old_il.body('get_constraint'), self.new_il.body('get_constraint')
        old_method = self.old_il.methods['get_constraint']
        new_method = self.new_il.methods['get_constraint']
        self.assertEqual(old_method.Rva, new_method.Rva)
        self.assertEqual(len(self.original), len(self.patched))
        start = self.old_il.pe.get_offset_from_rva(old_method.Rva)
        end = start + old.header_size + old.code_size
        changes = [i - start for i in range(start, end)
                   if self.original[i] != self.patched[i]]
        self.assertEqual(changes, [0x24])
        self.assertEqual((self.original[start + 0x24], self.patched[start + 0x24]), (0x18, 0x16))
        self.assertEqual(self.original[start:start + old.header_size],
                         self.patched[start:start + new.header_size])
        self.assertEqual((old.code_size, old.max_stack, getattr(old.local_var_sig_tok, 'value', None)),
                         (new.code_size, new.max_stack, getattr(new.local_var_sig_tok, 'value', None)))
        self.assertFalse(new.exception_handlers)
        # Preserve the existing scaling arithmetic and anchor notification path.
        for name in ('get_activeScaling', 'get_activeHeight', 'UpdateScale'):
            row = self.old_il.methods[name]
            body = self.old_il.body(name)
            offset = self.old_il.pe.get_offset_from_rva(row.Rva)
            length = body.header_size + body.code_size
            self.assertEqual(self.original[offset:offset + length],
                             self.patched[offset:offset + length])

    def test_other_constraint_combinations_keep_their_original_behavior(self):
        for fit_width, fit_height, expected in ((True, True, 0), (False, True, 3),
                                                (False, False, 1)):
            with self.subTest(fit_width=fit_width, fit_height=fit_height):
                root = self.root(fit_width=fit_width, fit_height=fit_height)
                self.assertEqual(self.old_il.execute('get_constraint', root, (2696, 1260)), expected)
                self.assertEqual(self.new_il.execute('get_constraint', root, (2696, 1260)), expected)
        self.assertEqual(self.old_il.execute('get_constraint', self.root(), (2696, 1260)), 2)
        self.assertEqual(self.new_il.execute('get_constraint', self.root(), (2696, 1260)), 0)

    def test_real_long_screens_restore_full_design_height(self):
        for screen, cropped in (((2696, 1260), 598), ((2800, 1260), 576)):
            with self.subTest(screen=screen):
                root = self.root()
                self.assertEqual(self.old_il.execute('get_activeHeight', root, screen), cropped)
                self.assertEqual(self.new_il.execute('get_activeHeight', root, screen), 640)

    def test_full_design_width_and_height_fit_long_legacy_and_narrow_screens(self):
        # Widths follow the APK's closest-ratio categories: S8, 16:9, 16:10, 4:3.
        for screen, width in (((2696, 1260), 1280), ((2800, 1260), 1280),
                              ((1920, 1080), 1136), ((1280, 720), 1136),
                              ((1920, 1200), 1080), ((1024, 768), 1080)):
            with self.subTest(screen=screen, design_width=width):
                root = self.root(width=width)
                height = self.new_il.execute('get_activeHeight', root, screen)
                aspect = screen[0] / screen[1]
                self.assertGreaterEqual(height, root['manualHeight'])
                self.assertGreaterEqual(height * aspect, root['manualWidth'])
                self.assertGreaterEqual(height, self.old_il.execute('get_activeHeight', root, screen))


if __name__ == '__main__':
    unittest.main()
