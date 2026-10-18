"""Execute the native first-pet branch and refresh CIL without issuing real requests."""
import contextlib
import io
from pathlib import Path
import unittest

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools.patch_client import patch_dll


VOID = object()


class PetVM:
    def __init__(self, pe, reward='', num=0, next_free=0):
        self.pe = pe
        self.methods, self.owners = {}, {}
        for row in pe.net.mdtables.TypeDef.rows:
            for method in row.MethodList:
                self.methods[str(row.TypeName), str(method.row.Name)] = method.row
                self.owners[0x06000000 | method.row_index] = str(row.TypeName)
        self.recruit = dict(reward=reward, num=num, nextFree=next_free, mustTimes=0)
        self.config = {'tableGameConfigureDao': {'currentData': {
            'PetFirstRecruitGet': 'pet_002,1', 'PetRecruitCostOnceCoin': 1000,
            'PetRecruitCostOnce': 'diamond,100', 'PetRecruitCostFive': 'diamond,450'}}}
        self.tab = {'recruitInfo': {'recruit': self.recruit}}
        self.tab['itemsList'] = ['stale-normal-recruit-cache']
        for field in ('extraGetLab', 'fiveConstLab', 'mustTimes', 'mustLabel',
                      'remainLab', 'NoFreeRoot', 'oneConstLab', 'FreeThisTimeLab'):
            self.tab[field] = {'name': field}
        self.static = {'Empty': '', 'canClickTab': True, 'playerData': {'diamond': 1000}, 'redTips': {}}
        self.sent, self.timers, self.stops, self.refreshes = [], [], [], []

    def member(self, token):
        table = {4: 'Field', 6: 'MethodDef', 10: 'MemberRef'}[token >> 24]
        return getattr(self.pe.net.mdtables, table).rows[(token & 0xffffff)-1]

    def call(self, token, stack):
        row = self.member(token)
        name = str(row.Name)
        owner = self.owners.get(token, str(getattr(getattr(row, 'Class', None), 'row', '')))
        if name == 'get_Instance' and owner == 'DictDataManager':
            return self.config
        if name == 'op_Equality':
            right = stack.pop()
            return stack.pop() == right
        if owner == 'TabPetRecruit' and name == 'OnRecruitGuide':
            self.run('OnRecruitGuide', [stack.pop()])
            return VOID
        if owner == 'TabPetRecruit' and name == 'UpdateAll':
            arg = stack.pop()
            stack.pop()
            self.refreshes.append(arg)
            return VOID
        if name == 'SendHttp':
            handler, request, opcode = stack.pop(), stack.pop(), stack.pop()
            self.sent.append((opcode, request, handler))
            return True
        if name == 'GetText':
            return stack.pop()
        if name == 'Format':
            value, template = stack.pop(), stack.pop()
            return f'{template}:{value}'
        if name == 'Split':
            stack.pop()
            return stack.pop().split(',')
        if name == 'SetPointState':
            stack.pop(); stack.pop(); stack.pop()
            return VOID
        if name == 'get_gameObject':
            return stack.pop()
        if name == 'get_Count':
            return len(stack.pop())
        if name == 'Clear':
            stack.pop().clear()
            return VOID
        if name == 'SetActive':
            active = stack.pop()
            stack.pop()['active'] = bool(active)
            return VOID
        if name == 'StopCoroutine':
            self.stops.append(stack.pop())
            stack.pop()
            return VOID
        if name == 'UpdateOpenCountDown':
            stack.pop()
            self.timers.append('UpdateOpenCountDown')
            return {'timer': True}
        if name == 'StartCoroutine':
            stack.pop(); stack.pop()
            return {'coroutine': True}
        if name.startswith('get_'):
            return stack.pop().get(name[4:])
        if name.startswith('set_'):
            value = stack.pop()
            stack.pop()[name[4:]] = value
            return VOID
        raise AssertionError((owner, name))

    def run(self, name, args, start=None, stop=None, owner='TabPetRecruit'):
        row = self.methods[owner, name]
        body = read_method_body_from_bytes(self.pe.get_data(row.Rva))
        ins = {i.offset: i for i in body.instructions}
        pc, stack, locals_ = start or body.instructions[0].offset, [], {}
        for _ in range(1000):
            if pc == stop:
                assert not stack, stack
                return
            i = ins[pc]
            op, operand, next_pc = i.opcode.name, i.operand, pc+i.size
            if op.startswith('ldarg.'):
                stack.append(args[int(op.rsplit('.', 1)[1])])
            elif op.startswith('ldloc.'):
                stack.append(locals_[int(op.rsplit('.', 1)[1])])
            elif op.startswith('stloc.'):
                locals_[int(op.rsplit('.', 1)[1])] = stack.pop()
            elif op.startswith('ldc.i4.'):
                n = op.rsplit('.', 1)[1]
                stack.append(int(operand) if n == 's' else int(n))
            elif op == 'ldc.i4': stack.append(operand)
            elif op == 'conv.i8': stack[-1] = int(stack[-1])
            elif op == 'ldstr': stack.append(self.pe.net.user_strings.get(operand.value & 0xffffff).value)
            elif op == 'ldsfld': stack.append(self.static[str(self.member(operand.value).Name)])
            elif op == 'ldfld': stack.append(stack.pop().get(str(self.member(operand.value).Name)))
            elif op == 'ldftn': stack.append(operand.value)
            elif op == 'newobj':
                owner = self.owners[operand.value]
                if owner == 'HttpHandler':
                    function, receiver = stack.pop(), stack.pop()
                    stack.append(dict(type=owner, function=function, receiver=receiver))
                else: stack.append(dict(type=owner))
            elif op in ('call', 'callvirt'):
                value = self.call(operand.value, stack)
                if value is not VOID: stack.append(value)
            elif op == 'isinst': pass
            elif op == 'newarr': stack.append([None]*stack.pop())
            elif op == 'dup': stack.append(stack[-1])
            elif op == 'stelem.i2':
                value, index, array = stack.pop(), stack.pop(), stack.pop()
                array[index] = value
            elif op == 'ldelem.ref':
                index = stack.pop()
                stack.append(stack.pop()[index])
            elif op == 'box': pass
            elif op == 'pop': stack.pop()
            elif op in ('cgt', 'or'):
                right, left = stack.pop(), stack.pop()
                stack.append(left > right if op == 'cgt' else bool(left) or bool(right))
            elif op in ('br', 'br.s'): next_pc = operand
            elif op in ('brtrue', 'brtrue.s', 'brfalse', 'brfalse.s'):
                if bool(stack.pop()) == op.startswith('brtrue'): next_pc = operand
            elif op in ('ble', 'bgt'):
                right, left = stack.pop(), stack.pop()
                if (left <= right if op == 'ble' else left > right): next_pc = operand
            elif op == 'nop': pass
            elif op == 'ret':
                assert not stack, (name, pc, stack)
                return
            else: raise AssertionError((name, pc, op))
            assert len(stack) <= body.max_stack, (name, pc, stack)
            pc = next_pc
        raise AssertionError('unbounded pet CIL')


class NativeFirstPetTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.raw = (Path(__file__).resolve().parents[1] / 'analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll').read_bytes()
        with contextlib.redirect_stdout(io.StringIO()):
            cls.patched = patch_dll(cls.raw, 'http://127.0.0.1:18088/sao/', optional_town_guides=True)
        cls.pe = dnfile.dnPE(data=cls.patched)

    def test_actual_pending_preview_sends_original_first_opcode_and_callback(self):
        vm = PetVM(self.pe, 'pet_002,1')
        vm.run('OnRecruitOnce', [vm.tab])
        self.assertEqual(len(vm.sent), 1)
        opcode, request, callback = vm.sent[0]
        self.assertEqual((opcode, request['type']), (21509, 'CSPetFirstRecruit'))
        self.assertEqual(vm.member(callback['function']).Name, 'RecruitGuideCallback')
        self.assertEqual(vm.recruit['num'], 0)

    def test_claimed_or_unrelated_preview_does_not_enter_first_path(self):
        probe = PetVM(self.pe)
        body = read_method_body_from_bytes(self.pe.get_data(probe.methods['TabPetRecruit', 'OnRecruitOnce'].Rva))
        ordinary = next(i.offset for i in body.instructions if i.opcode.name == 'newobj')
        for reward in ('', None, 'pet_003,1', 'pet_002', 'pet_002,2'):
            vm = PetVM(self.pe, reward)
            vm.run('OnRecruitOnce', [vm.tab], stop=ordinary)
            self.assertEqual(vm.sent, [])

    def test_free_label_and_zero_cooldown_never_schedule_unknown_pool(self):
        for reward, num, next_free, free in (('pet_002,1', 0, 0, True), ('', 0, 0, False), ('', 2, 0, True)):
            vm = PetVM(self.pe, reward, num, next_free)
            vm.run('OnMsgCallback', [vm.tab, {'kBody': {'recruit': vm.recruit}}, True])
            self.assertEqual(vm.tab['FreeThisTimeLab']['active'], free)
            self.assertEqual(vm.tab['NoFreeRoot']['active'], not free)
            self.assertEqual(vm.timers, [])
            self.assertFalse(vm.tab['remainLab'].get('active', False))
            self.assertEqual(vm.recruit['num'], num)
        vm = PetVM(self.pe, '', 0, 1000)
        vm.run('OnMsgCallback', [vm.tab, {'kBody': {'recruit': vm.recruit}}, True])
        self.assertEqual(vm.timers, ['UpdateOpenCountDown'])

    def test_success_refresh_preserves_count_and_exception_layout(self):
        vm = PetVM(self.pe, 'pet_002,1')
        vm.run('RecruitGuideCallback', [vm.tab], start=0x90, stop=0xb7)
        self.assertEqual((vm.recruit['reward'], vm.recruit['num'], vm.refreshes), ('', 0, [False]))
        self.assertEqual(vm.tab['itemsList'], [])
        original = dnfile.dnPE(data=self.raw)
        original_vm = PetVM(original)
        row = original_vm.methods['TabPetRecruit', 'RecruitGuideCallback']
        offset = original.get_offset_from_rva(row.Rva)
        before = read_method_body_from_bytes(original.get_data(row.Rva))
        after = read_method_body_from_bytes(self.pe.get_data(vm.methods['TabPetRecruit', 'RecruitGuideCallback'].Rva))
        self.assertEqual(len(self.raw), len(self.patched))
        self.assertEqual((before.header_size, before.code_size, [vars(e) for e in before.exception_handlers]),
                         (after.header_size, after.code_size, [vars(e) for e in after.exception_handlers]))
        self.assertEqual(self.raw[offset:offset+0x90], self.patched[offset:offset+0x90])
        self.assertEqual(self.raw[offset+0xb7:offset+before.header_size+before.code_size],
                         self.patched[offset+0xb7:offset+before.header_size+before.code_size])
        for name in ('OnRecruitOnce', 'OnMsgCallback'):
            body = read_method_body_from_bytes(self.pe.get_data(vm.methods['TabPetRecruit', name].Rva))
            offsets = {i.offset for i in body.instructions}
            for i in body.instructions:
                if 'BrTarget' in i.opcode.operand_type.name:
                    self.assertIn(i.operand, offsets)

    def test_first_result_hides_purchase_title_and_normal_result_retains_it(self):
        vm = PetVM(self.pe)
        row = vm.methods['PetRecruitResultPage', 'DoOpen']
        body = read_method_body_from_bytes(self.pe.get_data(row.Rva))
        start = next(i.offset for i in body.instructions if i.opcode.name == 'ldfld'
                     and str(vm.member(i.operand.value).Name) == 'extraGetLab') - 1
        stop = next(i.offset for i in body.instructions if i.offset > start
                    and i.opcode.name == 'call' and vm.owners.get(i.operand.value) == 'DictDataManager')
        for items in ([], [{'id': 'pet_002', 'num': 1}]):
            page = dict(itemsList=items, extraGetLab={})
            vm.run('DoOpen', [page], start=start, stop=stop, owner='PetRecruitResultPage')
            self.assertEqual(page['extraGetLab']['active'], bool(items))


if __name__ == '__main__':
    unittest.main()
