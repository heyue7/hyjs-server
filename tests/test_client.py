"""Validate the reconstructed client's mission selection against patched Mono IL."""
import contextlib
import io
from pathlib import Path
import unittest
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

from tools.patch_client import DLL, patch_dll

ROOT = Path(__file__).resolve().parents[1]


def copy_sync_method(raw):
    pe = dnfile.dnPE(data=raw)
    for owner in pe.net.mdtables.TypeDef:
        if str(owner.TypeNamespace) == 'UI' and str(owner.TypeName) == 'UICopyData':
            for method in owner.MethodList:
                if str(method.row.Name) == 'SetPassedCopyInfo':
                    start = pe.get_offset_from_rva(method.row.Rva)
                    return pe, start, read_method_body_from_bytes(pe.get_data(method.row.Rva))
    raise AssertionError('Missing UI.UICopyData.SetPassedCopyInfo')


class ClientPatchTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        with zipfile.ZipFile(ROOT / 'apks/original/10056262.apk') as archive:
            cls.original = archive.read(DLL)
        with contextlib.redirect_stdout(io.StringIO()):
            cls.patched = patch_dll(cls.original, 'http://127.0.0.1:18088/sao/')

    def test_default_activity_prefers_wheel_but_preserves_explicit_tab_path(self):
        pe=dnfile.dnPE(data=self.original); patched=dnfile.dnPE(data=self.patched)
        owner=next(t for t in pe.net.mdtables.TypeDef if str(t.TypeName)=='UIActivity')
        method=next(m.row for m in owner.MethodList if str(m.row.Name)=='DoOpen')
        old=read_method_body_from_bytes(pe.get_data(method.Rva))
        new=read_method_body_from_bytes(patched.get_data(method.Rva))
        instructions={i.offset:i for i in new.instructions}
        self.assertEqual((old.code_size,old.header_size),(new.code_size,new.header_size))
        self.assertEqual((instructions[0x1c].opcode.name,instructions[0x1c].operand),('brfalse',0x40))
        self.assertEqual((instructions[0x3c].opcode.name,instructions[0x3c].operand),('br.s',0x48))
        self.assertEqual([instructions[i].opcode.name for i in (0x40,0x41,0x43)],['ldarg.0','ldc.i4.s','stfld'])
        self.assertEqual(instructions[0x41].operand,34)
        start=pe.get_offset_from_rva(method.Rva)
        self.assertEqual(self.original[start+0x21:start+0x3c],self.patched[start+0x21:start+0x3c])
        self.assertEqual(self.original[start+0x48:start+old.size],self.patched[start+0x48:start+old.size])

    def test_single_spin_display_cannot_mutate_wallet_reward_ids_or_amounts(self):
        old_pe, new_pe = dnfile.dnPE(data=self.original), dnfile.dnPE(data=self.patched)
        owner = next(t for t in old_pe.net.mdtables.TypeDef if str(t.TypeName)=='UIActivityBigTurn')
        method = next(m.row for m in owner.MethodList if str(m.row.Name)=='GetResultObject')
        start = old_pe.get_offset_from_rva(method.Rva)
        old = read_method_body_from_bytes(old_pe.get_data(method.Rva))
        new = read_method_body_from_bytes(new_pe.get_data(method.Rva))
        self.assertEqual((old.header_size,old.code_size,old.max_stack),(new.header_size,new.code_size,new.max_stack))
        self.assertEqual([(e.try_start,e.try_end,e.handler_start,e.handler_end) for e in old.exception_handlers],
                         [(e.try_start,e.try_end,e.handler_start,e.handler_end) for e in new.exception_handlers])
        replaced = set()
        for offset in (0x47,0x94,0xc8):
            self.assertEqual(self.patched[start+offset:start+offset+5],b'\x26\x26\x00\x00\x00')
            replaced.update(range(offset,offset+5))
        for i in old.instructions:
            if i.offset not in (0x47,0x94,0xc8):
                self.assertEqual(self.original[start+i.offset:start+i.offset+i.size],
                                 self.patched[start+i.offset:start+i.offset+i.size])
        self.assertFalse(any(i.opcode.name=='callvirt' and
            str(new_pe.net.mdtables.MethodDef.rows[(i.operand.value&0xffffff)-1].Name) in ('set_id','set_num')
            for i in new.instructions if i.opcode.name=='callvirt' and i.operand.value>>24==6))

    def test_copy_sync_preserves_method_structure_and_balanced_branches(self):
        _, old_start, old_body = copy_sync_method(self.original)
        _, new_start, new_body = copy_sync_method(self.patched)
        self.assertEqual(old_start, new_start)
        self.assertEqual(len(self.original), len(self.patched))
        self.assertEqual((old_body.code_size, old_body.header_size, old_body.max_stack),
                         (new_body.code_size, new_body.header_size, new_body.max_stack))
        self.assertEqual(len(old_body.exception_handlers), len(new_body.exception_handlers))
        self.assertEqual([(i.offset, i.opcode.name, i.size) for i in old_body.instructions],
                         [(i.offset, i.opcode.name, i.size) for i in new_body.instructions])
        branch_offsets = (0x124, 0x13e)
        for instruction in new_body.instructions:
            if instruction.offset in branch_offsets:
                self.assertEqual(instruction.opcode.name, 'brfalse')
                self.assertEqual(instruction.operand, instruction.offset + instruction.size)
        end = old_start + old_body.header_size + old_body.code_size
        changed = {offset - old_start for offset in range(old_start, end)
                   if self.original[offset] != self.patched[offset]}
        self.assertEqual(changed, {0x125, 0x13f})

    def test_optional_town_guides_only_disable_three_automatic_triggers(self):
        with contextlib.redirect_stdout(io.StringIO()):
            optional = patch_dll(self.original, 'http://127.0.0.1:18088/sao/',
                                 optional_town_guides=True)
        pe = dnfile.dnPE(data=self.patched)
        optional_pe = dnfile.dnPE(data=optional)
        allowed = set()
        found = set()
        for owner in pe.net.mdtables.TypeDef:
            if str(owner.TypeName) != 'GuideManager':
                continue
            for method in owner.MethodList:
                if str(method.row.Name) not in ('OnLogin', 'OnLevelUp', 'OnQuestComplete'):
                    continue
                found.add(str(method.row.Name))
                start = pe.get_offset_from_rva(method.row.Rva)
                body = read_method_body_from_bytes(pe.get_data(method.row.Rva))
                new_body = read_method_body_from_bytes(optional_pe.get_data(method.row.Rva))
                self.assertEqual(len(new_body.exception_handlers), 0)
                self.assertEqual((new_body.code_size, new_body.header_size, new_body.max_stack),
                                 (body.code_size, body.header_size, body.max_stack))
                self.assertEqual(optional[start], self.patched[start] & ~0x8)
                allowed.add(start)
                start += body.header_size
                end = start + body.code_size
                self.assertEqual(optional[start:end], b'\x2a' + b'\0' * (body.code_size - 1))
                allowed.update(range(start, end))
        self.assertEqual(found, {'OnLogin', 'OnLevelUp', 'OnQuestComplete'})
        self.assertEqual(len(optional), len(self.patched))
        changed = {i for i, (old, new) in enumerate(zip(self.patched, optional)) if old != new}
        self.assertTrue(changed)
        self.assertTrue(changed <= allowed)

    def test_manual_strong_recruit_routes_only_first_cleared_guide_stage(self):
        pe = dnfile.dnPE(data=self.patched)
        owner = next(t for t in pe.net.mdtables.TypeDef if str(t.TypeName) == 'TabRecruit')
        method = next(m.row for m in owner.MethodList if str(m.row.Name) == 'OnHighOnce')
        body = read_method_body_from_bytes(pe.get_data(method.Rva))
        instructions = {i.offset: i for i in body.instructions}
        gate = next(i for i in body.instructions if i.opcode.name == 'brfalse.s')
        for current, expected in [(None, 'normal'), ('guide_start', 'normal'),
                                  ('guide_quest_1', 'guide'), ('guide_recruit', 'normal'),
                                  ('guide_hero_up', 'normal')]:
            stack, pc, calls = [], body.header_size, []
            while pc != gate.operand:
                instruction = instructions[pc]
                name, operand = instruction.opcode.name, instruction.operand
                pc += instruction.size
                if name == 'ldsfld':
                    self.assertEqual(operand.value, 0x040030c0)
                    stack.append({'currentGuideId': current})
                elif name == 'ldfld':
                    field = str(pe.net.mdtables.Field.rows[(operand.value & 0xffffff)-1].Name)
                    stack.append(stack.pop()[field])
                elif name in ('ldc.i4.0', 'ldc.i4.1'):
                    stack.append(int(name[-1]))
                elif name == 'ldarg.0':
                    stack.append('tab')
                elif name in ('call', 'callvirt'):
                    token = operand.value
                    if token == 0x060038c3:
                        field = str(pe.net.mdtables.Field.rows[0x3044-1].Name)
                        stack.append({field: {1: [{'guideId': 'guide_quest_1'}]}})
                    elif token in (0x0a001508, 0x0a001509):
                        key, collection = stack.pop(), stack.pop()
                        stack.append(collection[key])
                    elif token == 0x060053a3:
                        self.assertEqual(stack.pop(), 'tab')
                        calls.append('guide')
                    else:
                        self.assertEqual(str(pe.net.mdtables.MemberRef.rows[(token & 0xffffff)-1].Name), 'op_Equality')
                        right, left = stack.pop(), stack.pop()
                        stack.append(left == right)
                elif name == 'brfalse.s':
                    if not stack.pop():
                        pc = operand
                elif name == 'ret':
                    break
                else:
                    self.fail('Unexpected recruit gate opcode ' + name)
            self.assertEqual(calls, ['guide'] if expected == 'guide' else [])
            self.assertEqual(stack, [])
        for instruction in body.instructions:
            if 'BrTarget' in instruction.opcode.operand_type.name:
                self.assertIn(instruction.operand, instructions)

    def test_manual_guide_free_label_preserves_stored_cooldown_and_other_types(self):
        pe = dnfile.dnPE(data=self.patched)
        owner = next(t for t in pe.net.mdtables.TypeDef if str(t.TypeName) == 'WarriorRecruit')
        method = next(m.row for m in owner.MethodList if str(m.row.Name) == 'get_freeTime')
        body = read_method_body_from_bytes(pe.get_data(method.Rva))
        instructions = {i.offset: i for i in body.instructions}
        cost_field = str(pe.net.mdtables.Field.rows[0x5a9-1].Name)
        time_field = str(pe.net.mdtables.Field.rows[0x5aa-1].Name)
        cases = [(2, 'guide_quest_1', 0), (1, 'guide_quest_1', 1234567890123),
                 (3, 'guide_quest_1', 1234567890123), (2, 'guide_start', 1234567890123),
                 (2, 'guide_recruit', 1234567890123), (2, None, 1234567890123),
                 (2, False, 1234567890123)]
        for kind, guide, expected in cases:
            recruit = {cost_field: kind, time_field: 1234567890123}
            player = None if guide is False else {'currentGuideId': guide}
            stack, pc = [], body.instructions[0].offset
            while True:
                instruction = instructions[pc]
                name, operand = instruction.opcode.name, instruction.operand
                pc += instruction.size
                if name == 'ldarg.0':
                    stack.append(recruit)
                elif name == 'ldsfld':
                    self.assertEqual(operand.value, 0x040030c0)
                    stack.append(player)
                elif name == 'ldfld':
                    field = str(pe.net.mdtables.Field.rows[(operand.value & 0xffffff)-1].Name)
                    stack.append(stack.pop()[field])
                elif name.startswith('ldc.i4.'):
                    stack.append(int(name[-1]))
                elif name == 'conv.i8':
                    stack[-1] = int(stack[-1])
                elif name in ('call', 'callvirt'):
                    token = operand.value
                    if token == 0x06000b13:
                        stack.append(stack.pop()[cost_field])
                    elif token == 0x060038c3:
                        field = str(pe.net.mdtables.Field.rows[0x3044-1].Name)
                        stack.append({field: {1: [{'guideId': 'guide_quest_1'}]}})
                    elif token in (0x0a001508, 0x0a001509):
                        key, collection = stack.pop(), stack.pop()
                        stack.append(collection[key])
                    else:
                        self.assertEqual(str(pe.net.mdtables.MemberRef.rows[(token & 0xffffff)-1].Name), 'op_Equality')
                        right, left = stack.pop(), stack.pop()
                        stack.append(left == right)
                elif name == 'brfalse.s':
                    if not stack.pop():
                        pc = operand
                elif name == 'bne.un.s':
                    right, left = stack.pop(), stack.pop()
                    if left != right:
                        pc = operand
                elif name == 'ret':
                    self.assertEqual(stack, [expected])
                    break
                else:
                    self.fail('Unexpected cooldown getter opcode ' + name)
                self.assertLessEqual(len(stack), body.max_stack)
            self.assertEqual(recruit[time_field], 1234567890123)
        self.assertEqual(body.header_size, 12)
        for instruction in body.instructions:
            if 'BrTarget' in instruction.opcode.operand_type.name:
                self.assertIn(instruction.operand, instructions)

    def test_first_open_replaces_existing_mission_and_chapter_selection(self):
        # Execute the real decoded method tail with stale and empty local selection.
        # This checks both branches consume their condition and persist server values.
        pe, _, body = copy_sync_method(self.patched)
        instructions = {i.offset: i for i in body.instructions}
        for old_chapter, old_mission in (('1', '1'), ('1', '2'), ('', ''), ('1', ''), ('', '1')):
            with self.subTest(chapter=old_chapter, mission=old_mission):
                fields = {'currFightChapterId': old_chapter, 'currFightLevelId': old_mission}
                response = {'currentChapterId': '2', 'currentMissionId': '6'}
                stack = []
                cursor = 0x11a
                while True:
                    instruction = instructions[cursor]
                    opcode = instruction.opcode.name
                    next_cursor = cursor + instruction.size
                    if opcode in ('ldsfld', 'stsfld'):
                        field = pe.net.mdtables.Field.rows[(instruction.operand.value & 0xffffff) - 1]
                        name = str(field.Name)
                        if opcode == 'ldsfld':
                            stack.append(fields[name])
                        else:
                            fields[name] = stack.pop()
                    elif opcode == 'ldarg.1':
                        stack.append(response)
                    elif opcode == 'call':
                        method = pe.net.mdtables.MemberRef.rows[(instruction.operand.value & 0xffffff) - 1]
                        self.assertEqual(str(method.Name), 'IsNullOrEmpty')
                        stack.append(stack.pop() in (None, ''))
                    elif opcode == 'callvirt':
                        method = pe.net.mdtables.MethodDef.rows[(instruction.operand.value & 0xffffff) - 1]
                        self.assertIn(str(method.Name), ('get_currentMissionId', 'get_currentChapterId'))
                        stack.append(stack.pop()[str(method.Name)[4:]])
                    elif opcode == 'brfalse':
                        if not stack.pop():
                            next_cursor = instruction.operand
                    elif opcode == 'ret':
                        break
                    else:
                        self.fail(f'Unexpected selection IL: {opcode}')
                    cursor = next_cursor
                self.assertEqual(fields, {'currFightChapterId': '2', 'currFightLevelId': '6'})
                self.assertEqual(stack, [])


if __name__ == '__main__':
    unittest.main()
