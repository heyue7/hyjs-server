"""Execute native snapshot IL through level, experience and wallet synchronization."""
import contextlib
import copy
import io
from pathlib import Path
import unittest
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

from tools.patch_client import DLL, patch_dll

ROOT = Path(__file__).resolve().parents[1]


class MissingChannelSDK(RuntimeError):
    pass


class PlayerSnapshotIL:
    def __init__(self, raw):
        self.pe = dnfile.dnPE(data=raw)
        self.methods = {}
        self.fields = {}
        for owner in self.pe.net.mdtables.TypeDef:
            for method in owner.MethodList:
                self.methods[method.row_index] = (str(owner.TypeName), str(method.row.Name), method.row)
            for field in owner.FieldList:
                self.fields[field.row_index] = str(field.row.Name)

    def body(self, name):
        row = next(row for owner, method, row in self.methods.values()
                   if (owner, method) == ('UIPlayerData', name))
        return read_method_body_from_bytes(self.pe.get_data(row.Rva))

    def callee(self, token):
        if token.value >> 24 == 6:
            return self.methods[token.value & 0xffffff][:2]
        row = self.pe.net.mdtables.MemberRef.rows[(token.value & 0xffffff) - 1]
        return str(getattr(row.Class.row, 'TypeName', 'external')), str(row.Name)

    def execute(self, name, args, state):
        state['current_player'] = args[0]
        body = self.body(name)
        instructions = {ins.offset: ins for ins in body.instructions}
        cursor = body.instructions[0].offset
        stack, locals_ = [], {}
        for _ in range(400):
            # The unchanged remainder handles VIP, guides, clocks and union data.
            # Execute all original branches through both wallet assignments.
            if name == 'ResetPlayer' and cursor == 0x10a:
                if stack:
                    raise AssertionError('Unbalanced snapshot stack')
                return
            ins = instructions[cursor]
            op, arg = ins.opcode.name, ins.operand
            next_cursor = cursor + ins.size
            if op.startswith('ldarg.'):
                stack.append(args[int(op.rsplit('.', 1)[1])])
            elif op.startswith('ldc.i4.'):
                suffix = op.rsplit('.', 1)[1]
                stack.append(arg if suffix == 's' else -1 if suffix == 'm1' else int(suffix))
            elif op.startswith('ldloc.'):
                stack.append(locals_[int(op.rsplit('.', 1)[1])])
            elif op.startswith('stloc.'):
                locals_[int(op.rsplit('.', 1)[1])] = stack.pop()
            elif op == 'ldfld':
                stack.append(stack.pop()[self.fields[arg.value & 0xffffff]])
            elif op == 'stfld':
                value, target = stack.pop(), stack.pop()
                target[self.fields[arg.value & 0xffffff]] = value
            elif op == 'ldstr':
                stack.append(self.pe.net.user_strings.get(arg.value & 0xffffff).value)
            elif op == 'conv.i8':
                stack.append(int(stack.pop()))
            elif op in ('call', 'callvirt'):
                owner, method = self.callee(arg)
                if owner == 'UIPlayerData' and method in ('get_level', 'set_level'):
                    call_args = [stack.pop(), stack.pop()][::-1] if method == 'set_level' else [stack.pop()]
                    value = self.execute(method, call_args, state)
                    if method == 'get_level':
                        stack.append(value)
                elif owner == 'UIPlayerData' and method == 'get_isLevelMax':
                    stack.pop()
                    stack.append(state.get('level_max', False))
                elif owner == 'Player' and method.startswith('get_'):
                    stack.append(stack.pop()[method[4:]])
                elif method == 'GetUIHero':
                    stack.pop()  # hero GUID
                    instance = stack.pop()
                    stack.append(instance.get('hero') if instance.get('initialized', True) else None)
                elif method == 'ResetAttribute':
                    stack.pop()
                    state['hero_resets'] += 1
                    state.setdefault('hero_reset_levels', []).append(state['current_player']['_level'])
                elif owner == 'UIInstPlayerData' and method == 'ResetAllFormationAttribute':
                    instance = stack.pop()
                    if not instance.get('initialized', True) or not instance.get('hero'):
                        raise AssertionError('Formation refresh before hero initialization')
                    state['hero_resets'] += 1
                    level = state['current_player']['_level']
                    state.setdefault('hero_reset_levels', []).append(level)
                    state['formation_refreshes'] = state.get('formation_refreshes', 0) + 1
                    # The real routine refreshes heroes first and then each camp's
                    # cached attributes/power. Model that external cache boundary.
                    state['cached_formation_level'] = level
                elif owner == 'GameConfig' and method == 'get_Instance':
                    stack.append({'platform': 2})
                elif method == 'isSupported':
                    stack.pop()
                    state['sdk_checks'] += 1
                    if state['missing_sdk']:
                        raise MissingChannelSDK('Retired Java SDK is unavailable')
                    stack.append(True)
                elif owner == 'PlatformSDK' and method == 'GetInstance':
                    stack.append({})
                elif owner == 'PlatformSDK' and method == 'SDKSubmitData':
                    stack.pop()
                    stack.pop()
                    state['sdk_reports'] += 1
                elif method == 'GetUserExpInfoByLevel':
                    stack.pop()
                    stack.append({'exp': 10000})
                elif method == 'Log':
                    stack.pop()
                else:
                    raise AssertionError((owner, method))
            elif op in ('br', 'br.s'):
                next_cursor = arg
            elif op in ('brtrue', 'brfalse', 'brtrue.s', 'brfalse.s'):
                if bool(stack.pop()) == op.startswith('brtrue'):
                    next_cursor = arg
            elif op in ('bge', 'beq', 'beq.s', 'bne.un'):
                right, left = stack.pop(), stack.pop()
                if {'bge': left >= right, 'beq': left == right, 'beq.s': left == right,
                    'bne.un': left != right}[op]:
                    next_cursor = arg
            elif op == 'dup':
                stack.append(stack[-1])
            elif op == 'pop':
                stack.pop()
            elif op == 'nop':
                pass
            elif op == 'ret':
                if name == 'set_level' and stack:
                    raise AssertionError('Unbalanced setter return stack')
                return stack.pop() if stack else None
            else:
                raise AssertionError(op)
            if len(stack) > body.max_stack:
                raise AssertionError('Snapshot stack exceeds declared max_stack')
            cursor = next_cursor
        raise AssertionError('Snapshot IL did not terminate')


class PlayerSnapshotPatchTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        with zipfile.ZipFile(ROOT / 'apks/original/10056262.apk') as apk:
            cls.original = apk.read(DLL)
        with contextlib.redirect_stdout(io.StringIO()):
            cls.patched = patch_dll(cls.original, 'http://192.168.31.252:8088/sao/')
        cls.before = PlayerSnapshotIL(cls.original)
        cls.after = PlayerSnapshotIL(cls.patched)

    def fixture(self, current=3, incoming=4, missing=True):
        player = {'instPlayerData': {'hero': {'id': 'main'}}, 'heroWarUID': 'main',
                  'playerId': 3, 'name': 'old', '_level': current,
                  'coin': 197630, 'diamond': 20, 'exp': 70, 'maxExp': 10000}
        snapshot = {'playerId': 3, 'name': 'demo', 'level': incoming,
                    'coin': 198610, 'diamond': 25, 'exp': 10}
        state = {'missing_sdk': missing, 'sdk_checks': 0, 'sdk_reports': 0, 'hero_resets': 0}
        return player, snapshot, state

    def test_missing_sdk_reproduces_half_updated_level_and_wallet(self):
        player, snapshot, state = self.fixture()
        with self.assertRaises(MissingChannelSDK):
            self.before.execute('ResetPlayer', [player, snapshot], state)
        self.assertEqual((player['_level'], player['coin']), (4, 197630))
        self.assertEqual(state['sdk_checks'], 1)
        player, snapshot, state = self.fixture()
        self.after.execute('ResetPlayer', [player, snapshot], state)
        self.assertEqual((player['_level'], player['coin'], player['exp'], player['diamond']),
                         (4, 198610, 10, 25))
        self.assertEqual((state['sdk_checks'], state['sdk_reports']), (0, 0))

    def test_upgrade_same_level_downgrade_and_first_initialization_copy_snapshot(self):
        for current, incoming in [(1, 2), (3, 4), (4, 4), (4, 3), (0, 1)]:
            with self.subTest(current=current, incoming=incoming):
                player, snapshot, state = self.fixture(current, incoming)
                self.after.execute('ResetPlayer', [player, snapshot], state)
                self.assertEqual((player['_level'], player['coin'], player['exp'], player['diamond']),
                                 (incoming, 198610, 10, 25))
                self.assertEqual(state['sdk_checks'], 0)

    def test_existing_sdk_keeps_native_level_and_wallet_behavior(self):
        player, snapshot, state = self.fixture(missing=False)
        original_player = copy.deepcopy(player)
        original_state = copy.deepcopy(state)
        self.before.execute('ResetPlayer', [original_player, snapshot], original_state)
        self.after.execute('ResetPlayer', [player, snapshot], state)
        self.assertEqual(player, original_player)
        self.assertEqual(state['hero_resets'], original_state['hero_resets'])
        self.assertEqual(original_state['sdk_reports'], 1)
        self.assertEqual(state['sdk_reports'], 0)

    def test_uninitialized_inventory_guard_remains_unchanged(self):
        player, snapshot, state = self.fixture()
        player['instPlayerData'] = None
        expected = copy.deepcopy(player)
        self.after.execute('ResetPlayer', [player, snapshot], state)
        self.assertEqual(player, expected)

    def test_only_reporting_block_changes_and_branch_labels_remain(self):
        original = self.before.body('ResetPlayer')
        patched = self.after.body('ResetPlayer')
        self.assertEqual(original.code_size, patched.code_size)
        self.assertEqual(original.max_stack, patched.max_stack)
        before = {ins.offset: ins for ins in original.instructions}
        after = {ins.offset: ins for ins in patched.instructions}
        for offset, ins in before.items():
            if 0x62 <= offset < 0x96:
                continue
            self.assertEqual(after[offset].opcode.name, ins.opcode.name)
            self.assertEqual(str(after[offset].operand), str(ins.operand))
        self.assertTrue(all(ins.opcode.name == 'nop' for ins in patched.instructions
                            if 0x62 <= ins.offset < 0x96))

    def test_main_hero_attribute_refresh_observes_new_player_level(self):
        player, _, state = self.fixture(current=4, incoming=5)
        self.before.execute('set_level', [player, 5], state)
        self.assertEqual((player['_level'], state['hero_reset_levels']), (5, [4]))
        player, _, state = self.fixture(current=4, incoming=5)
        self.after.execute('set_level', [player, 5], state)
        self.assertEqual((player['_level'], state['hero_reset_levels']), (5, [5]))
        self.assertEqual((state['formation_refreshes'], state['cached_formation_level']), (1, 5))

    def test_setter_same_level_null_hero_initialization_and_downgrade(self):
        for current, incoming, has_hero, expected_resets in [
                (5, 5, True, []), (4, 5, False, []),
                (0, 1, True, [1]), (5, 4, True, [4])]:
            with self.subTest(current=current, incoming=incoming, has_hero=has_hero):
                player, _, state = self.fixture(current, incoming)
                if not has_hero:
                    player['instPlayerData']['hero'] = None
                self.after.execute('set_level', [player, incoming], state)
                self.assertEqual(player['_level'], incoming)
                self.assertEqual(state.get('hero_reset_levels', []), expected_resets)
                self.assertEqual(state.get('formation_refreshes', 0), bool(expected_resets))
                if expected_resets:
                    self.assertEqual(state['cached_formation_level'], incoming)

    def test_empty_instance_and_uninitialized_hero_dictionary_skip_refresh(self):
        for instance in (None, {'initialized': False, 'hero': None}):
            with self.subTest(instance=instance):
                player, _, state = self.fixture(current=4, incoming=5)
                player['instPlayerData'] = instance
                self.after.execute('set_level', [player, 5], state)
                self.assertEqual(player['_level'], 5)
                self.assertEqual(state['hero_resets'], 0)
                self.assertEqual(state.get('formation_refreshes', 0), 0)

    def test_setter_size_stack_locals_and_effective_branches_remain_valid(self):
        original = self.before.body('set_level')
        patched = self.after.body('set_level')
        self.assertEqual(original.code_size, patched.code_size)
        self.assertEqual(patched.max_stack, max(original.max_stack, 3))
        self.assertEqual(original.local_var_sig_tok.value, patched.local_var_sig_tok.value)
        instructions = {ins.offset: ins for ins in patched.instructions}
        self.assertEqual(instructions[0x13].operand, 0x3a)
        self.assertEqual(instructions[0x23].operand, 0x39)
        self.assertEqual(instructions[0x31].operand, 0x39)
        self.assertEqual(instructions[0x17].opcode.name, 'stfld')
        self.assertEqual(self.after.callee(instructions[0x33].operand),
                         ('UIInstPlayerData', 'ResetAllFormationAttribute'))
        self.assertEqual(instructions[0x38].opcode.name, 'ret')
        self.assertEqual(instructions[0x39].opcode.name, 'pop')
        self.assertEqual(instructions[0x3a].opcode.name, 'ret')


if __name__ == '__main__':
    unittest.main()
