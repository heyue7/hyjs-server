"""Execute patched native order CIL. No device, APK build or payment confirmation."""
import contextlib
import io
from pathlib import Path
import unittest

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools.patch_client import patch_dll


VOID=object()


class NativePaymentVM:
    def __init__(self,pe,*,page='ChargePage'):
        self.pe=pe
        self.callback={'__type':'PlatformSDKCallBack','orderId':None,'payMentId':None,'tick':5000}
        self.page=None if page is None else {'__type':page,'chargeData':{'__type':'ChargeData'}}
        self.manager={'__type':'PageManager','currentPage':self.page}
        self.config={'__type':'GameConfig'}
        self.now=1800000000000
        self.sent=[];self.scheduled=[];self.popups=[];self.refreshes=0;self.top_refreshes=0
        self.methods={};self.owners={}
        for t in pe.net.mdtables.TypeDef.rows:
            for m in t.MethodList:
                self.methods[(str(t.TypeName),str(m.row.Name))]=m.row
                self.owners[0x06000000|m.row_index]=str(t.TypeName)
        self.steps=0

    def member(self,token):
        table={4:'Field',6:'MethodDef',10:'MemberRef',43:'MethodSpec'}[token>>24]
        return getattr(self.pe.net.mdtables,table).rows[(token&0xffffff)-1]

    def call(self,token,stack):
        if token>>24==43:
            self.assert_component(token)
            return stack.pop()
        row=self.member(token);name=str(row.Name)
        owner=self.owners.get(token,'')
        if token>>24==10:
            owner=str(getattr(row.Class.row,'TypeName',''))
        if name=='get_Instance':
            return {'GameConfig':self.config,'PlatformSDKCallBack':self.callback,'PageManager':self.manager}[owner]
        if owner=='GameConfig' and name=='get_GetChannelId':stack.pop();return 'AND0003'
        if owner=='ServerTimerTool' and name=='CurrentUTCJavaTime':return self.now
        if name=='Parse' and owner=='Double':return float(stack.pop())
        if name=='IsNullOrEmpty':return stack.pop() in (None,'')
        if name=='op_Equality':right=stack.pop();return stack.pop()==right
        if name=='CancelInvoke':stack.pop();self.scheduled.clear();return VOID
        if name=='Invoke':delay=stack.pop();target=stack.pop();stack.pop();self.scheduled.append((target,delay));return VOID
        if name=='SendHttp':handler=stack.pop();request=stack.pop();opcode=stack.pop();self.sent.append((opcode,request,handler));return True
        if name=='GetText' and owner=='TableTextBlo':return stack.pop()
        if name=='CreatePopupTextBox':stack.pop();self.popups.append(stack.pop());return VOID
        if owner=='ChargeData' and name=='InitPayMent':stack.pop();self.refreshes+=1;return VOID
        if owner=='ChargePage' and name=='InitTop':stack.pop();self.top_refreshes+=1;return VOID
        if owner=='PlatformSDKCallBack' and name in ('CheckOrder','TickCheck','TestCharge'):
            self.run(owner,name,[stack.pop()]);return VOID
        if name.startswith('set_'):
            value=stack.pop();stack.pop()[name[4:]]=value;return VOID
        if name.startswith('get_'):
            return stack.pop().get(name[4:])
        raise AssertionError((owner,name,hex(token)))

    def assert_component(self,token):
        assert token in (0x2b00056c,0x2b00056d),hex(token)

    @staticmethod
    def truth(value):
        return value is not None and (bool(value) if isinstance(value,(int,float,bool)) else True)

    def run(self,owner,name,args,*,start=None,stop=None):
        row=self.methods[(owner,name)]
        body=read_method_body_from_bytes(self.pe.get_data(row.Rva))
        instructions={i.offset:i for i in body.instructions}
        pc=body.instructions[0].offset if start is None else start;stack=[];locals_={}
        while True:
            if pc==stop:
                assert not stack,(owner,name,'unbalanced segment',stack)
                return
            self.steps+=1
            assert self.steps<10000,'unbounded native payment CIL'
            ins=instructions[pc];op=ins.opcode.name;operand=ins.operand;next_pc=pc+ins.size
            if op.startswith('ldarg.'):
                stack.append(args[int(op.split('.')[-1])])
            elif op.startswith('ldloc.'):
                stack.append(locals_[int(op.split('.')[-1])])
            elif op.startswith('stloc.'):
                locals_[int(op.split('.')[-1])]=stack.pop()
            elif op=='ldnull':stack.append(None)
            elif op=='ldstr':stack.append(self.pe.net.user_strings.get(operand.value&0xffffff).value)
            elif op.startswith('ldc.i4.'):
                suffix=op.split('.')[-1];stack.append(int(operand) if suffix=='s' else -1 if suffix=='m1' else int(suffix))
            elif op in ('ldc.i4','ldc.r4'):stack.append(operand)
            elif op in ('conv.i8','conv.r8'):stack[-1]=int(stack[-1]) if op=='conv.i8' else float(stack[-1])
            elif op=='dup':stack.append(stack[-1])
            elif op=='pop':stack.pop()
            elif op=='ldfld':stack.append(stack.pop().get(str(self.member(operand.value).Name)))
            elif op=='ldsfld':
                assert str(self.member(operand.value).Name)=='Empty';stack.append('')
            elif op=='stfld':value=stack.pop();stack.pop()[str(self.member(operand.value).Name)]=value
            elif op in ('isinst','castclass'):
                type_name=str(self.pe.net.mdtables.TypeDef.rows[(operand.value&0xffffff)-1].TypeName)
                obj=stack.pop()
                matches=obj is not None and obj.get('__type')==type_name
                if op=='castclass':assert matches or obj is None
                stack.append(obj if matches else None)
            elif op=='ldftn':stack.append(operand.value)
            elif op=='newobj':
                row=self.member(operand.value);owner=self.owners.get(operand.value)
                assert str(row.Name)=='.ctor'
                if owner=='HttpHandler':function=stack.pop();receiver=stack.pop();stack.append({'__type':owner,'function':function,'receiver':receiver})
                else:stack.append({'__type':owner})
            elif op in ('call','callvirt'):
                result=self.call(operand.value,stack)
                if result is not VOID:stack.append(result)
            elif op in ('add','sub','mul','div','rem'):
                right=stack.pop();left=stack.pop()
                stack.append({'add':lambda:left+right,'sub':lambda:left-right,'mul':lambda:left*right,'div':lambda:left//right,'rem':lambda:left%right}[op]())
            elif op in ('br','br.s'):next_pc=operand
            elif op in ('brfalse','brfalse.s','brtrue','brtrue.s'):
                condition=self.truth(stack.pop())
                if condition==op.startswith('brtrue'):next_pc=operand
            elif op in ('ble','bgt'):
                right=stack.pop();left=stack.pop()
                if left<=right if op=='ble' else left>right:next_pc=operand
            elif op=='nop':pass
            elif op=='ret':
                if bytes(row.Signature.value) == b'\x20\x00\x0e':
                    assert len(stack)==1,(owner,name,'invalid string return',stack)
                    return stack.pop()
                assert not stack,(owner,name,'unbalanced return',stack)
                return
            else:raise AssertionError((owner,name,pc,op))
            assert len(stack)<=body.max_stack,(owner,name,'maxstack',len(stack),body.max_stack)
            pc=next_pc

    def begin(self,product='payment_001',price='6'):
        self.run('ChargeData','ChargeBegin',[{'__type':'ChargeData'},'商品',price,product])

    def generated(self,*,success=True,product='payment_001',order='local-order'):
        body={'__type':'SCGenerateOrder','orderId':order,'transactionData':{'__type':'TransactionData','paymentId':product}}
        self.run('PlatformSDKCallBack','PCChargeCallBack',[self.callback,{'kBody':body},success])

    def checked(self,status,*,order='local-order',product='payment_001',success=True):
        body={'__type':'SCCheckOrder','transactionData':{'__type':'TransactionData','orderId':order,'paymentId':product,'payStatus':status}}
        self.run('PlatformSDKCallBack','ChekOrderCallBack',[self.callback,{'kBody':body},success])


class NativePaymentTests(unittest.TestCase):
    def test_native_product_heading_distinguishes_red_bundles_and_keeps_bonus_label(self):
        for name in ('60钻石', '测试红卡礼包', '测试红装备礼包'):
            vm = NativePaymentVM(self.pe)
            label = {}
            vm.run('ChargeItem', 'InitItem', [{'buyNum': label}, {'paymentName': name}], start=0x12f, stop=0x157)
            self.assertEqual(label['text'], name)
        old = dnfile.dnPE(data=self.raw)
        original = NativePaymentVM(old).methods[('ChargeItem', 'InitItem')]
        changed = NativePaymentVM(self.pe).methods[('ChargeItem', 'InitItem')]
        self.assertEqual(original.Rva, changed.Rva)
        before = read_method_body_from_bytes(old.get_data(original.Rva))
        after = read_method_body_from_bytes(self.pe.get_data(changed.Rva))
        self.assertEqual(before.code_size, after.code_size)
        self.assertEqual(old.get_data(original.Rva+0x157,before.code_size+12-0x157),self.pe.get_data(changed.Rva+0x157,after.code_size+12-0x157))

    def test_charge_begin_cross_class_pending_fields_have_clr_access(self):
        original = dnfile.dnPE(data=self.raw)
        for index, (before, after) in enumerate(zip(original.net.mdtables.Field.rows, self.pe.net.mdtables.Field.rows), 1):
            if index in (0x384c, 0x384d):
                self.assertTrue(after.Flags.fdAssembly)
                self.assertFalse(after.Flags.fdPublic)
                self.assertEqual(before.struct.Flags & ~7, after.struct.Flags & ~7)
            else:
                self.assertEqual(before.struct.Flags, after.struct.Flags)
            self.assertEqual(before.Signature.value, after.Signature.value)
        callback = NativePaymentVM(self.pe).methods[('PlatformSDKCallBack', 'PCChargeCallBack')]
        self.assertTrue(callback.Flags.mdAssem)
        self.assertFalse(callback.Flags.mdPublic)

    def test_native_catalogue_shop_fallback_needs_no_retired_java_sdk(self):
        vm = NativePaymentVM(self.pe)
        self.assertEqual(vm.run('GameConfig', 'get_GetShopId', [vm.config]), 'common')

    @classmethod
    def setUpClass(cls):
        cls.raw=Path('analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll').read_bytes()
        with contextlib.redirect_stdout(io.StringIO()):cls.patched=patch_dll(cls.raw,'http://127.0.0.1:18088/sao/')
        cls.pe=dnfile.dnPE(data=cls.patched)

    def test_native_button_creates_correct_order_without_sdk_or_credentials(self):
        vm=NativePaymentVM(self.pe);vm.begin()
        self.assertEqual(len(vm.sent),1)
        opcode,request,handler=vm.sent[0]
        self.assertEqual(opcode,20042)
        self.assertEqual(request,{'__type':'CSGenerateOrder','paymentId':'payment_001','money':6.0,'moneyType':'RMB','channelId':'AND0003','shopId':'common'})
        self.assertEqual(vm.owners[handler['function']],'PlatformSDKCallBack')
        vm.begin();self.assertEqual(len(vm.sent),1,'pending button must not create another order')
        vm.generated();self.assertEqual(vm.sent[-1][0],20019)
        self.assertEqual(vm.sent[-1][1]['orderId'],'local-order')
        self.assertEqual(vm.sent[-1][1]['paymentId'],'payment_001')
        self.assertEqual(vm.callback['tick'],(vm.now+30000)*16+10)

    def test_wait_is_bounded_on_time_and_number_even_if_clock_freezes(self):
        for advance_clock in (True,False):
            vm=NativePaymentVM(self.pe);vm.begin();vm.generated()
            for index in range(11):
                vm.checked('PAY_STATUS_WAIT')
                if not vm.scheduled:break
                target,delay=vm.scheduled.pop();self.assertEqual((target,delay),('CheckOrder',3))
                if advance_clock:vm.now+=int(delay*1000)
                vm.run('PlatformSDKCallBack',target,[vm.callback])
            else:self.fail('native query loop did not stop')
            self.assertEqual(sum(op==20019 for op,_,_ in vm.sent),11)
            self.assertEqual(vm.callback['tick'],0)
            self.assertEqual(vm.popups,['chargeFailed'])
            self.assertEqual(vm.now-1800000000000,30000 if advance_clock else 0)
        vm=NativePaymentVM(self.pe);vm.begin();vm.generated();vm.now+=30001;vm.checked('PAY_STATUS_WAIT')
        self.assertFalse(vm.scheduled);self.assertEqual(vm.callback['tick'],0)

    def test_confirmed_query_refreshes_charge_page_and_safely_handles_other_pages(self):
        for page,manager in [('ChargePage',True),('UIActivity',True),(None,True),(None,False)]:
            vm=NativePaymentVM(self.pe,page=page)
            if not manager:vm.manager=None
            vm.begin();vm.generated();vm.checked('PAY_STATUS_WAIT')
            vm.checked('PAY_STATUS_SUCCESS')
            self.assertEqual(vm.refreshes,1 if page=='ChargePage' else 0)
            self.assertEqual(vm.top_refreshes,1 if page=='ChargePage' else 0)
            self.assertFalse(vm.scheduled)
            self.assertEqual(vm.callback['tick'],0)
            self.assertFalse(vm.popups)
            self.assertEqual({op for op,_,_ in vm.sent},{20042,20019})

    def test_failure_unknown_status_and_bad_generation_stop_without_confirming(self):
        for status in ('PAY_STATUS_FAIL','unexpected'):
            vm=NativePaymentVM(self.pe);vm.begin();vm.generated();vm.checked('PAY_STATUS_WAIT');vm.checked(status)
            self.assertFalse(vm.scheduled);self.assertEqual(vm.callback['orderId'],'');self.assertEqual(vm.popups,['chargeFailed'])
        for success,product in [(False,'payment_001'),(True,'wrong-product')]:
            vm=NativePaymentVM(self.pe);vm.begin();vm.generated(success=success,product=product)
            self.assertEqual([op for op,_,_ in vm.sent],[20042]);self.assertEqual(vm.popups,['chargeFailed'])

    def test_stale_response_cannot_clear_current_order(self):
        vm=NativePaymentVM(self.pe);vm.begin();vm.generated();before=dict(vm.callback)
        vm.checked('PAY_STATUS_SUCCESS',order='older-order')
        vm.checked('PAY_STATUS_FAIL',product='wrong-product')
        self.assertEqual(vm.callback,before);self.assertEqual(vm.refreshes,0);self.assertFalse(vm.popups)

    def test_method_bodies_branches_storage_and_metadata_are_valid(self):
        old=dnfile.dnPE(data=self.raw)
        self.assertEqual(len(self.raw),len(self.patched))
        for name in ('TypeDef','Field','MethodDef','MemberRef','StandAloneSig','MethodSpec'):
            self.assertEqual(len(getattr(old.net.mdtables,name).rows),len(getattr(self.pe.net.mdtables,name).rows))
        vm=NativePaymentVM(self.pe)
        intervals=[]
        for owner,name in [('ChargeData','ChargeBegin'),('PlatformSDKCallBack','PCChargeCallBack'),('PlatformSDKCallBack','ChekOrderCallBack'),('PlatformSDKCallBack','TickCheck'),('PlatformSDKCallBack','TestCharge')]:
            row=vm.methods[(owner,name)];body=read_method_body_from_bytes(self.pe.get_data(row.Rva));self.assertFalse(body.exception_handlers)
            self.assertEqual(body.local_var_sig_tok,None)
            valid={i.offset for i in body.instructions}
            for i in body.instructions:
                if 'BrTarget' in i.opcode.operand_type.name:self.assertIn(i.operand,valid)
            intervals.append((row.Rva,row.Rva+body.header_size+body.code_size))
            calls=[str(vm.member(i.operand.value).Name) for i in body.instructions if i.opcode.name in ('call','callvirt') and i.operand.value>>24 in (6,10)]
            self.assertNotIn('get_PassWord',calls);self.assertNotIn('GenerateOrder',calls);self.assertNotIn('FailOrder',calls)
        for left,right in zip(sorted(intervals),sorted(intervals)[1:]):self.assertLessEqual(left[1],right[0])
        for owner in ('Init','GenerateOrder'):
            row=vm.methods[('PlatformSDK',owner)];body=read_method_body_from_bytes(self.pe.get_data(row.Rva));self.assertEqual(body.code_size,1);self.assertEqual(body.instructions[0].opcode.name,'ret')
