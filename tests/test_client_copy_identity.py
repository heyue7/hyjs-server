"""Regress player/task key collisions using the actual patched copy-room IL."""
import contextlib
import io
from pathlib import Path
import unittest

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

from tools.patch_client import patch_dll


class CopyIdentityTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.original = Path('analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll').read_bytes()
        with contextlib.redirect_stdout(io.StringIO()):
            cls.patched = patch_dll(cls.original, 'http://127.0.0.1:18088/sao/')
        cls.old = dnfile.dnPE(data=cls.original)
        cls.new = dnfile.dnPE(data=cls.patched)

    def method(self, pe, owner, name):
        row = next(m.row for t in pe.net.mdtables.TypeDef if str(t.TypeName) == owner
                   for m in t.MethodList if str(m.row.Name) == name)
        return row, read_method_body_from_bytes(pe.get_data(row.Rva))

    def member(self, pe, token):
        table = {4: 'Field', 6: 'MethodDef', 10: 'MemberRef'}[token.value >> 24]
        return getattr(pe.net.mdtables, table).rows[(token.value & 0xffffff)-1]

    def run_identifiers(self, pe, player_id, pet_id):
        """Execute LoadPlayer's identifier expression and LoadPet's owner/lookup.

        The rest of Unity rendering is intentionally outside this unit test;
        device acceptance checks the real creation/deletion and camera lifecycle.
        """
        _, player = self.method(pe, 'RoomCopy', 'LoadPlayer')
        _, pet = self.method(pe, 'RoomCopy', 'LoadPet')
        stack = []
        room = {'charactorManager': {}}
        locals_ = {0: pet_id, 1: player_id}
        self_key = None
        created_owner = None
        found_pet = None
        for mode, instructions in [('player', player.instructions), ('pet', pet.instructions)]:
            if mode == 'pet':
                # The first CreatePet expression starts with the room receiver.
                stop = next(i for i, ins in enumerate(instructions)
                            if ins.opcode.name == 'callvirt'
                            and str(self.member(pe, ins.operand).Name) == 'CreatePet')
                start = max(i for i, ins in enumerate(instructions[:stop]) if ins.opcode.name == 'ldarg.0')
                instructions = instructions[start:]
            for ins in instructions:
                op = ins.opcode.name
                if op == 'ldstr':
                    stack.append(pe.net.user_strings.get(ins.operand.value & 0xffffff).value)
                elif op == 'ldsfld':
                    name = str(self.member(pe, ins.operand).Name)
                    stack.append({'playerId': player_id} if name == 'playerData' else '')
                elif op in ('ldfld', 'ldflda'):
                    stack.append(stack.pop()[str(self.member(pe, ins.operand).Name)])
                elif op == 'ldarg.0':
                    stack.append(room)
                elif op.startswith('ldloc.'):
                    stack.append(locals_[int(op[-1])])
                elif op.startswith('stloc.'):
                    value = stack.pop()
                    if mode == 'player':
                        self_key = value
                        room['charactorManager'][self_key] = 'self'
                        break
                    locals_[int(op[-1])] = value
                elif op == 'box':
                    pass
                elif op == 'ldc.i4.1':
                    stack.append(True)
                elif op in ('call', 'callvirt'):
                    row = self.member(pe, ins.operand)
                    name = str(row.Name)
                    if name == 'ToString':
                        stack.append(str(stack.pop()))
                    elif name == 'Concat':
                        count = row.Signature.value[1]
                        values = stack[-count:]
                        del stack[-count:]
                        stack.append(''.join(map(str, values)))
                    elif name == 'CreatePet':
                        created_owner, selected_pet, manager = stack.pop(), stack.pop(), stack.pop()
                        self.assertEqual(manager[created_owner], 'self')
                        manager[created_owner + '@' + selected_pet] = 'pet'
                    elif name == 'GetCharactor':
                        key, manager = stack.pop(), stack.pop()
                        found_pet = manager[key]
                        stack.append(found_pet)
                    elif name == 'SetInView':
                        self.assertTrue(stack.pop())
                        self.assertEqual(stack.pop(), 'pet')
                    else:
                        self.fail('Unexpected identifier call: ' + name)
                elif op == 'ret':
                    break
                else:
                    self.fail('Unexpected identifier IL: ' + op)
            self.assertEqual(stack, [])
        return self_key, created_owner, found_pet

    def test_self_survives_monster_creation_and_victory_with_colliding_numeric_ids(self):
        for player_id in (1, 2, 3, 8, 10000, 9223372036854775807):
            with self.subTest(player_id=player_id):
                key, owner, pet = self.run_identifiers(self.new, player_id, 'pet_001')
                self.assertEqual((key, owner, pet), ('self' + str(player_id), 'self' + str(player_id), 'pet'))
                actors = {key: 'self'}
                task_id = str(player_id)
                actors[task_id] = 'monster'
                del actors[task_id]
                self.assertEqual(actors[key], 'self')
                # The original identifier reproduces the collision.
                old_key, _, _ = self.run_identifiers(self.old, player_id, 'pet_001')
                self.assertEqual(old_key, task_id)

    def test_relocation_preserves_locals_branch_targets_and_unrelated_methods(self):
        self.assertEqual(len(self.original), len(self.patched))
        for name in ('LoadPlayer', 'LoadPet'):
            old_row, old = self.method(self.old, 'RoomCopy', name)
            new_row, new = self.method(self.new, 'RoomCopy', name)
            self.assertNotEqual(old_row.Rva, new_row.Rva)
            self.assertEqual(old.local_var_sig_tok.value, new.local_var_sig_tok.value)
            self.assertFalse(new.exception_handlers)
            offsets = {ins.offset: ins for ins in new.instructions}
            old_offsets = {ins.offset: ins for ins in old.instructions}
            old_branches = [i for i in old.instructions if 'BrTarget' in i.opcode.operand_type.name]
            new_branches = [i for i in new.instructions if 'BrTarget' in i.opcode.operand_type.name]
            self.assertEqual(len(old_branches), len(new_branches))
            for before, after in zip(old_branches, new_branches):
                self.assertEqual(before.opcode.name, after.opcode.name)
                self.assertEqual(offsets[after.operand].opcode.name,
                                 old_offsets[before.operand].opcode.name)
        # Allocation is entirely inside the previously disabled platform method.
        sdk_row, sdk_old = self.method(self.old, 'PlatformSDK', 'SDKSubmitData')
        _, sdk_new = self.method(self.new, 'PlatformSDK', 'SDKSubmitData')
        self.assertEqual([i.opcode.name for i in sdk_new.instructions], ['ret'])
        spans = []
        for name in ('LoadPlayer', 'LoadPet'):
            row, body = self.method(self.new, 'RoomCopy', name)
            self.assertGreater(row.Rva, sdk_row.Rva+12)
            self.assertLessEqual(row.Rva+body.size, sdk_row.Rva+sdk_old.size)
            spans.append((row.Rva, row.Rva+body.size))
        self.assertLessEqual(spans[0][1], spans[1][0])
        for owner, name in [('RoomCopy', 'OnLoadComplete'), ('RoomCopy', 'LoadMonsters'),
                            ('RoomCharactorManager', 'CreateCharactor'), ('SceneCamera', 'Tick'),
                            ('RCMoveNavigation', 'ResetPosition')]:
            old_row, old = self.method(self.old, owner, name)
            new_row, new = self.method(self.new, owner, name)
            self.assertEqual(old_row.Rva, new_row.Rva)
            self.assertEqual(self.old.get_data(old_row.Rva, old.size), self.new.get_data(new_row.Rva, new.size))


if __name__ == '__main__':
    unittest.main()
