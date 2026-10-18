"""Exercise the recovered reward IL against authoritative wallet snapshots and inventory deltas."""
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


class RewardIL:
    def __init__(self, raw):
        self.pe = dnfile.dnPE(data=raw)
        self.methods = {}
        self.fields = {}
        for owner in self.pe.net.mdtables.TypeDef:
            for method in owner.MethodList:
                self.methods[method.row_index] = (str(owner.TypeName), str(method.row.Name), method.row)
            for field in owner.FieldList:
                self.fields[field.row_index] = str(field.row.Name)

    def run(self, owner, name, args, environment):
        row = next(row for kind, method, row in self.methods.values()
                   if (kind, method) == (owner, name))
        body = read_method_body_from_bytes(self.pe.get_data(row.Rva))
        instructions = {i.offset: i for i in body.instructions}
        stack, locals_ = [], {}
        cursor = body.instructions[0].offset
        max_stack = 0
        while True:
            ins = instructions[cursor]
            opcode = ins.opcode.name
            next_cursor = cursor + ins.size
            if opcode.startswith('ldarg.'):
                stack.append(args[int(opcode.rsplit('.', 1)[1])])
            elif opcode.startswith('ldloc.') and opcode != 'ldloc.s':
                stack.append(locals_[int(opcode.rsplit('.', 1)[1])])
            elif opcode.startswith('stloc.') and opcode != 'stloc.s':
                locals_[int(opcode.rsplit('.', 1)[1])] = stack.pop()
            elif opcode.startswith('ldc.i4.') and opcode != 'ldc.i4.s':
                stack.append(int(opcode.rsplit('.', 1)[1]))
            elif opcode == 'ldc.i4.s':
                stack.append(ins.operand)
            elif opcode == 'ldsfld':
                stack.append(environment[self.fields[ins.operand.value & 0xffffff]])
            elif opcode == 'ldfld':
                stack.append(stack.pop()[self.fields[ins.operand.value & 0xffffff]])
            elif opcode == 'stfld':
                value, target = stack.pop(), stack.pop()
                target[self.fields[ins.operand.value & 0xffffff]] = value
            elif opcode == 'dup':
                stack.append(stack[-1])
            elif opcode == 'isinst':
                # The selected response branch is known from the independently supplied battle type.
                self.assert_response_type(stack[-1], ins.operand.value)
            elif opcode in ('call', 'callvirt'):
                kind, method, _ = self.methods[ins.operand.value & 0xffffff]
                if method == 'GetResultMsg':
                    stack.append(stack.pop()['response'])
                elif kind == 'UIPlayerData' and method == 'AddResultObj':
                    result, player = stack.pop(), stack.pop()
                    environment['add_calls'] += 1
                    self.run(kind, method, [player, result], environment)
                elif kind == 'UIPlayerData' and method.startswith('Add'):
                    values, player = stack.pop(), stack.pop()
                    if method == 'AddSimpleItem':
                        for resource in values:
                            if resource['id'] == 'coin':
                                # The original unchecked Int64 add may wrap before the snapshot is restored.
                                player['coin'] = ((player['coin'] + resource['num'] + 2**63) % 2**64) - 2**63
                            elif resource['id'].startswith('item_'):
                                stock = player['items']
                                stock[resource['id']] = stock.get(resource['id'], 0) + resource['num']
                            elif resource['id'] != 'exp':
                                raise AssertionError('Unmodeled resource in reward fixture')
                    else:
                        player[method].extend(values)
                elif method.startswith('get_'):
                    stack.append(stack.pop()[method[4:]])
                else:
                    raise AssertionError((kind, method))
            elif opcode in ('br', 'br.s'):
                next_cursor = ins.operand
            elif opcode in ('brfalse', 'brtrue'):
                truth = bool(stack.pop())
                if truth == (opcode == 'brtrue'):
                    next_cursor = ins.operand
            elif opcode in ('bne.un', 'beq'):
                right, left = stack.pop(), stack.pop()
                if (left == right) == (opcode == 'beq'):
                    next_cursor = ins.operand
            elif opcode == 'ret':
                if stack:
                    raise AssertionError('Unbalanced reward evaluation stack')
                return
            else:
                raise AssertionError(opcode)
            max_stack = max(max_stack, len(stack))
            if max_stack > body.max_stack:
                raise AssertionError('Patched stack exceeds declared max_stack')
            cursor = next_cursor

    def assert_response_type(self, value, token):
        row = self.pe.net.mdtables.TypeDef.rows[(token & 0xffffff)-1]
        if value['type'] != str(row.TypeName):
            raise AssertionError('Unexpected response branch')


class CopyWalletPatchTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        with zipfile.ZipFile(ROOT / 'apks/original/10056262.apk') as archive:
            cls.raw = archive.read(DLL)
        with contextlib.redirect_stdout(io.StringIO()):
            cls.patched = patch_dll(cls.raw, 'http://192.168.31.252:8088/sao/')
        cls.before, cls.after = RewardIL(cls.raw), RewardIL(cls.patched)

    def fixture(self, coin=50760, gain=True, kind=1):
        reward = {'simps': [{'id': 'coin', 'num': 760}, {'id': 'exp', 'num': 10},
                            {'id': 'item_124', 'num': 2}],
                  'wars': [{'id': 'hero_exp_006', 'guid': 'material-from-copy'}],
                  'equips': [], 'treas': [], 'pets': []}
        response = {'type': {1: 'SCCopyFightEnd', 4: 'SCDailyCopyFightEnd', 9: 'SCRushFightEnd'}[kind],
                    'gain': reward if gain else None, 'get': reward if gain else None,
                    'resultObj': reward if gain else None}
        player = {'coin': coin, 'items': {}, 'AddHero': [], 'AddEquipment': [],
                  'AddTreasure': [], 'AddPet': []}
        environment = {'playerData': player, 'curBattleData': {'battleType': kind}, 'add_calls': 0}
        return {'response': response}, environment

    def test_reproduces_observed_double_coin_and_preserves_snapshot_after_patch(self):
        args, before = self.fixture()
        old_reward = copy.deepcopy(args)
        self.before.run('CopyBalance', 'AddBalanceReward', [args], before)
        self.assertEqual(before['playerData']['coin'], 51520)
        args, after = self.fixture()
        self.after.run('CopyBalance', 'AddBalanceReward', [args], after)
        self.assertEqual(after['playerData']['coin'], 50760)
        self.assertEqual(after['playerData']['items'], {'item_124': 2})
        self.assertEqual(after['playerData']['AddHero'], [{'id': 'hero_exp_006', 'guid': 'material-from-copy'}])
        self.assertEqual(args, old_reward)
        self.assertEqual(after['add_calls'], 1)

    def test_zero_and_large_wallet_snapshots_restore_exactly(self):
        for coin in (0, 2**63 - 1):
            with self.subTest(coin=coin):
                args, after = self.fixture(coin=coin)
                self.after.run('CopyBalance', 'AddBalanceReward', [args], after)
                self.assertEqual(after['playerData']['coin'], coin)
                self.assertEqual(after['playerData']['items']['item_124'], 2)

    def test_null_copy_gain_skips_snapshot_and_restore_with_balanced_stack(self):
        args, after = self.fixture(gain=False)
        self.after.run('CopyBalance', 'AddBalanceReward', [args], after)
        self.assertEqual(after['playerData']['coin'], 50760)
        self.assertEqual(after['add_calls'], 0)

    def test_other_battle_reward_branch_keeps_original_behavior(self):
        args, after = self.fixture(kind=9)
        self.after.run('CopyBalance', 'AddBalanceReward', [args], after)
        self.assertEqual(after['playerData']['coin'], 51520)
        self.assertEqual(after['playerData']['items']['item_124'], 2)

    def test_daily_copy_preserves_authoritative_wallet_and_inventory(self):
        args, before = self.fixture(kind=4)
        self.before.run('CopyBalance', 'AddBalanceReward', [args], before)
        self.assertEqual(before['playerData']['coin'], 51520)
        for coin in (0, 50760, 2**63 - 1):
            with self.subTest(coin=coin):
                args, after = self.fixture(kind=4, coin=coin)
                original_reward = copy.deepcopy(args)
                self.after.run('CopyBalance', 'AddBalanceReward', [args], after)
                self.assertEqual(after['playerData']['coin'], coin)
                self.assertEqual(after['playerData']['items'], {'item_124': 2})
                self.assertEqual(len(after['playerData']['AddHero']), 1)
                self.assertEqual(after['add_calls'], 1)
                self.assertEqual(args, original_reward)

    def test_null_daily_reward_leaves_stack_and_inventory_untouched(self):
        args, after = self.fixture(kind=4, gain=False)
        self.after.run('CopyBalance', 'AddBalanceReward', [args], after)
        self.assertEqual(after['playerData']['coin'], 50760)
        self.assertEqual(after['add_calls'], 0)

    def test_assembly_size_and_unchanged_original_reward_helper(self):
        self.assertEqual(len(self.raw), len(self.patched))
        old = next(row for owner, name, row in self.before.methods.values()
                   if (owner, name) == ('UIPlayerData', 'AddResultObj'))
        new = next(row for owner, name, row in self.after.methods.values()
                   if (owner, name) == ('UIPlayerData', 'AddResultObj'))
        before = read_method_body_from_bytes(self.before.pe.get_data(old.Rva))
        after = read_method_body_from_bytes(self.after.pe.get_data(new.Rva))
        self.assertEqual(self.before.pe.get_data(old.Rva, before.size),
                         self.after.pe.get_data(new.Rva, after.size))


if __name__ == '__main__':
    unittest.main()
