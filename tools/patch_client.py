"""Reproducible, size-preserving Mono patches and a parallel-install manifest.

Always starts from the original APK. Does not remove or overwrite the installed original.
"""
import argparse
import hashlib
import json
from pathlib import Path
import struct
import shutil
import subprocess
import sys
import tempfile
import zipfile
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
try:
    from tools.patch_pet_strength_all import patch_pet_strength_all
except ModuleNotFoundError:
    from patch_pet_strength_all import patch_pet_strength_all

ORIGINAL_SHA = 'b1d99496c7ffe17c45ab7646c72023be406924997d471e3f18db169fb0dfd641'
DLL = 'assets/bin/Data/Managed/Assembly-CSharp.dll'


def patch_native_test_payments(pe, raw, data, methods):
    """Use original order messages, with bounded Unity-thread polling.

    The client can only create/query. A separate trusted local adapter confirms
    payment facts. Reuse retired SDK method/string storage; no new metadata.
    """
    def method_token(owner, name):
        row = methods[owner, name]
        return 0x06000000 | next(i for i, r in enumerate(pe.net.mdtables.MethodDef.rows, 1) if r is row)

    def type_token(owner):
        return 0x02000000 | next(i for i, r in enumerate(pe.net.mdtables.TypeDef.rows, 1) if str(r.TypeName) == owner)

    def original_body(owner, name):
        row = methods[owner, name]
        return pe.get_offset_from_rva(row.Rva), read_method_body_from_bytes(pe.get_data(row.Rva))

    def encoded(opcode, token):
        return bytes([opcode]) + struct.pack('<I', token)

    class Program:
        def __init__(self):
            self.code, self.labels, self.jumps = bytearray(), {}, []
        def emit(self, value):
            self.code.extend(value)
        def label(self, name):
            assert name not in self.labels
            self.labels[name] = len(self.code)
        def jump(self, opcode, target):
            self.emit(bytes([opcode]))
            size = 1 if 0x2b <= opcode <= 0x37 else 4
            self.jumps.append((len(self.code), target, size))
            self.emit(b'\0' * size)
        def finish(self):
            for position, target, size in self.jumps:
                struct.pack_into('<b' if size == 1 else '<i', self.code, position, self.labels[target]-position-size)
            return bytes(self.code)

    sdk_start, sdk_body = original_body('PlatformSDK', 'GenerateOrder')
    assert sdk_body.header_size == 12 and not sdk_body.exception_handlers
    strings = {pe.net.user_strings.get(i.operand.value & 0xffffff).value: i.operand.value
               for i in sdk_body.instructions if i.opcode.name == 'ldstr'}
    test_start, test_body = original_body('PlatformSDKCallBack', 'TestCharge')
    test_literal = next(i.operand.value for i in test_body.instructions if i.opcode.name == 'ldstr')
    assert pe.net.user_strings.get(test_literal & 0xffffff).value == 'TestCharge'
    # These three diagnostic strings belonged solely to the retired SDK path.
    # Their #US offsets and all other metadata remain unchanged.
    def reuse_string(token, value):
        index = token & 0xffffff
        old = pe.net.user_strings.get(index)
        content = value.encode('utf-16le') + b'\0'
        assert len(content) < 128 and len(content) <= old.item_size
        offset = pe.net.user_strings.file_offset + index
        data[offset:offset+len(content)+1] = bytes([len(content)]) + content
        return encoded(0x72, token)
    currency = reuse_string(strings['generateOrder begin'], 'RMB')
    failed = reuse_string(strings['gamePlayerUid'], 'chargeFailed')
    check_name = reuse_string(test_literal, 'CheckOrder')
    data[test_start+test_body.header_size] = 0x2a
    # Shrink the retired method to ret; remaining bytes become explicit storage.
    data[sdk_start+12:sdk_start+12+sdk_body.code_size] = b'\0' * sdk_body.code_size
    data[sdk_start+12] = 0x2a
    struct.pack_into('<I', data, sdk_start+4, 1)
    cursor = (sdk_start+13+3) & ~3
    storage_end = sdk_start+12+sdk_body.code_size
    def rewrite(owner, name, code, stack, *, relocate=False):
        nonlocal cursor
        row = methods[owner, name]
        start, original = original_body(owner, name)
        assert original.header_size == 12 and not original.exception_handlers
        header = struct.pack('<HHII', 0x3003, stack, len(code), 0)
        if relocate:
            assert cursor+12+len(code) <= storage_end, (owner,name,len(code),storage_end-cursor-12)
            data[cursor:cursor+12+len(code)] = header+code
            struct.pack_into('<I', data, row.struct.get_file_offset(), pe.get_rva_from_offset(cursor))
            cursor = (cursor+12+len(code)+3) & ~3
        else:
            assert len(code) <= original.code_size
            data[start:start+12+original.code_size] = header+code+b'\0'*(original.code_size-len(code))
        print('Patched native test payment', owner, name, 'code_bytes',len(code))

    order, product, tick = (0x0400384c, 0x0400384d, 0x0400384e)
    # ChargeData shares this assembly with the callback. Permit only those two
    # pending markers to be read/written across the classes; retain field
    # tokens, signatures, layout and all unrelated visibility flags.
    for token, expected in ((order, 'orderId'), (product, 'payMentId')):
        field = pe.net.mdtables.Field.rows[(token & 0xffffff) - 1]
        assert str(field.Name) == expected and field.Flags.fdPrivate
        location = field.struct.get_file_offset()
        flags = struct.unpack_from('<H', data, location)[0]
        struct.pack_into('<H', data, location, (flags & ~7) | 3)
    # The original SDK callback was private to its class. The native order
    # sender now binds this existing Packet,bool method from ChargeData.
    callback_method = methods['PlatformSDKCallBack', 'PCChargeCallBack']
    assert callback_method.Flags.mdPrivate
    callback_flags = callback_method.struct.get_field_absolute_offset('Flags')
    flags = struct.unpack_from('<H', data, callback_flags)[0]
    struct.pack_into('<H', data, callback_flags, (flags & ~7) | 3)
    packet_body = encoded(0x7b, 0x040037ef)
    empty = encoded(0x7e, 0x0a00005f)
    equality = encoded(0x28, 0x0a000047)
    is_empty = encoded(0x28, 0x0a000577)
    now = encoded(0x28, method_token('ServerTimerTool', 'CurrentUTCJavaTime'))
    callback_instance = encoded(0x28, method_token('PlatformSDKCallBack', 'get_Instance'))
    popup = failed+encoded(0x28, method_token('TableTextBlo', 'GetText'))+b'\x22'+struct.pack('<f', 0)+encoded(0x28, method_token('BoxManager', 'CreatePopupTextBox'))
    stop = (b'\x02'+encoded(0x28, 0x0a000941)  # CancelInvoke()
            + b'\x02\x16\x6a'+encoded(0x7d, tick)
            + b'\x02'+empty+encoded(0x7d, order)
            + b'\x02'+empty+encoded(0x7d, product))
    # Reuse the old void TestCharge entry as a bounded failure finalizer.
    # Its body lives in the already-disabled SDK Init area, away from all earlier
    # room/wallet/recruit relocations and from the two order callbacks below.
    init_start, init_body = original_body('PlatformSDK','Init')
    assert init_body.header_size==12 and not init_body.exception_handlers
    assert data[init_start+12]==0x2a
    struct.pack_into('<I',data,init_start+4,1)
    failure_code=stop+popup+b'\x2a'
    failure_start=(init_start+13+3)&~3
    assert failure_start+12+len(failure_code)<=init_start+12+init_body.code_size
    data[failure_start:failure_start+12+len(failure_code)]=struct.pack('<HHII',0x3003,3,len(failure_code),0)+failure_code
    struct.pack_into('<I',data,methods['PlatformSDKCallBack','TestCharge'].struct.get_file_offset(),pe.get_rva_from_offset(failure_start))
    stop_failed=b'\x02'+encoded(0x28,method_token('PlatformSDKCallBack','TestCharge'))+b'\x2a'
    # No dictionary, password, SDK or speculative success is sent to the server.
    p = Program()
    p.emit(callback_instance+encoded(0x7b, order)+is_empty)
    p.jump(0x39, 'return')
    p.emit(callback_instance+b'\x05'+encoded(0x7d, order))  # pending marker
    p.emit(callback_instance+b'\x05'+encoded(0x7d, product))
    p.emit(b'\x20'+struct.pack('<i', 20042)+encoded(0x73, method_token('CSGenerateOrder', '.ctor')))
    for value, setter in ((b'\x05','set_paymentId'), (b'\x04'+encoded(0x28,0x0a000b49),'set_money'),
                          (currency,'set_moneyType'),
                          (encoded(0x28,method_token('GameConfig','get_Instance'))+encoded(0x6f,method_token('GameConfig','get_GetChannelId')),'set_channelId'),
                          (encoded(0x72,0x7000d455),'set_shopId')):
        p.emit(b'\x25'+value+encoded(0x6f,method_token('CSGenerateOrder',setter)))
    p.emit(callback_instance+b'\xfe\x06'+struct.pack('<I',method_token('PlatformSDKCallBack','PCChargeCallBack'))
           +encoded(0x73,0x06006372)+encoded(0x28,0x06003df4)+b'\x26')
    p.label('return');p.emit(b'\x2a')
    rewrite('ChargeData','ChargeBegin',p.finish(),6)

    # A legal Packet,bool callback publishes the server order and binds its product.
    generated = b'\x03'+packet_body+encoded(0x74,type_token('SCGenerateOrder'))
    generated_data = generated+encoded(0x6f,method_token('SCGenerateOrder','get_transactionData'))
    p = Program();p.emit(b'\x04');p.jump(0x39,'fail')
    p.emit(b'\x03'+packet_body+encoded(0x75,type_token('SCGenerateOrder')));p.jump(0x39,'fail')
    p.emit(generated_data);p.jump(0x39,'fail')
    p.emit(generated_data+encoded(0x6f,method_token('TransactionData','get_paymentId'))+b'\x02'+encoded(0x7b,product)+equality);p.jump(0x39,'fail')
    p.emit(generated+encoded(0x6f,method_token('SCGenerateOrder','get_orderId'))+is_empty);p.jump(0x3a,'fail')
    p.emit(b'\x02'+generated+encoded(0x6f,method_token('SCGenerateOrder','get_orderId'))+encoded(0x7d,order))
    # Pack a server-clock deadline and ten remaining scheduled probes in Int64.
    p.emit(b'\x02'+now+b'\x20'+struct.pack('<i',30000)+b'\x6a\x58\x1f\x10\x6a\x5a\x1f\x0a\x6a\x58'+encoded(0x7d,tick))
    p.emit(b'\x02'+encoded(0x28,method_token('PlatformSDKCallBack','CheckOrder'))+b'\x2a')
    p.label('fail');p.emit(stop_failed)
    rewrite('PlatformSDKCallBack','PCChargeCallBack',p.finish(),4,relocate=True)

    # Unknown/failing queries stop; stale replies cannot clear a later order.
    checked = b'\x03'+packet_body+encoded(0x74,type_token('SCCheckOrder'))
    transaction = checked+encoded(0x6f,method_token('SCCheckOrder','get_transactionData'))
    p = Program();p.emit(b'\x04');p.jump(0x39,'fail')
    p.emit(b'\x03'+packet_body+encoded(0x75,type_token('SCCheckOrder')));p.jump(0x39,'fail')
    p.emit(transaction);p.jump(0x39,'fail')
    p.emit(transaction+encoded(0x6f,method_token('TransactionData','get_orderId'))+b'\x02'+encoded(0x7b,order)+equality);p.jump(0x39,'return')
    p.emit(transaction+encoded(0x6f,method_token('TransactionData','get_paymentId'))+b'\x02'+encoded(0x7b,product)+equality);p.jump(0x39,'return')
    p.emit(transaction+encoded(0x6f,method_token('TransactionData','get_payStatus'))+encoded(0x72,0x7001087d)+equality);p.jump(0x3a,'success')
    # The server returns WAIT until its trusted adapter has actually confirmed.
    # Use the exact WAIT enum in a retired SDK diagnostic string slot.
    wait = reuse_string(strings['generateOrder begin2'], 'PAY_STATUS_WAIT')
    p.emit(transaction+encoded(0x6f,method_token('TransactionData','get_payStatus'))+wait+equality);p.jump(0x39,'fail')
    p.emit(b'\x02'+encoded(0x28,method_token('PlatformSDKCallBack','TickCheck'))+b'\x2a')
    p.label('success');p.emit(stop)
    p.emit(encoded(0x28,method_token('PageManager','get_Instance'))+b'\x25');p.jump(0x2c,'no_charge_page')
    p.emit(encoded(0x7b,0x040050a8)+encoded(0x75,type_token('ChargePage'))+b'\x25');p.jump(0x2c,'no_charge_page')
    p.emit(b'\x25'+encoded(0x6f,method_token('ChargePage','InitTop')))
    old_start,_=original_body('PlatformSDKCallBack','ChekOrderCallBack')
    p.emit(raw[old_start+0x4f:old_start+0x63]+b'\x2a')
    p.label('no_charge_page');p.emit(b'\x26');p.label('return');p.emit(b'\x2a')
    p.label('fail');p.emit(stop_failed)
    rewrite('PlatformSDKCallBack','ChekOrderCallBack',p.finish(),3,relocate=True)

    # Invoke runs on Unity's thread; no Timer worker, recursive spin or 1h backoff.
    p=Program();p.emit(b'\x02'+encoded(0x7b,tick)+b'\x1f\x10\x6a\x5d\x16\x6a');p.jump(0x3e,'stop')  # rem; ble
    p.emit(b'\x02'+encoded(0x7b,tick)+b'\x1f\x10\x6a\x5b'+now);p.jump(0x3e,'stop')  # div; ble deadline
    p.emit(b'\x02\x02'+encoded(0x7b,tick)+b'\x17\x6a\x59'+encoded(0x7d,tick))
    p.emit(b'\x02'+check_name+b'\x22'+struct.pack('<f',3)+encoded(0x28,0x0a00093e)+b'\x2a')
    p.label('stop');p.emit(stop_failed)
    rewrite('PlatformSDKCallBack','TickCheck',p.finish(),3)


def patch_dll(raw, url, *, optional_town_guides=False, repair_copy_navigation=False, demo_replay_controls=False):
    pe = dnfile.dnPE(data=raw)
    data = bytearray(raw)
    methods = {}
    for t in pe.net.mdtables.TypeDef:
        for m in t.MethodList:
            methods[(str(t.TypeName), str(m.row.Name))] = m.row

    def body(owner, name):
        r = methods[owner, name]
        b = read_method_body_from_bytes(pe.get_data(r.Rva))
        return pe.get_offset_from_rva(r.Rva), b

    def replace(owner, name, code, *, drop_exception_handlers=False):
        start, b = body(owner, name)
        assert len(code) <= b.code_size
        if drop_exception_handlers:
            assert owner == 'GuideManager' and name in ('OnLogin', 'OnLevelUp', 'OnQuestComplete')
            assert code == b'\x2a' and b.header_size == 12
            # These void methods contain foreach/finally. Their bodies become empty;
            # clear only MoreSects so Mono ignores the now-unused exception table.
            flags = struct.unpack_from('<H', raw, start)[0]
            assert flags & 3 == 3
            struct.pack_into('<H', data, start, flags & ~0x8)
        else:
            assert not b.exception_handlers
        start += b.header_size
        data[start:start+b.code_size] = code + b'\0' * (b.code_size-len(code))
        print('Patched', owner, name)

    def slice_code(owner, name, lo, hi):
        start, _ = body(owner, name)
        return raw[start+lo:start+hi]

    # 原会员卡先显示/填充、后加入网格。填充失败会留下无所属页面的卡片。
    # 提前执行原 AddChild，使失败卡片也随商店关闭释放；购买与报价路径保持原代码。
    vip_start, vip_body = body('TabVipShop', 'OnMsgCallback')
    vip_offsets = {i.offset: i for i in vip_body.instructions}
    assert vip_body.header_size == 12
    assert vip_offsets[0xae].opcode.name == 'ldloc.s'
    assert vip_offsets[0x108].opcode.name == 'ldarg.0'
    assert vip_offsets[0x115].opcode.name == 'callvirt'
    assert vip_offsets[0x11a].opcode.name == 'ldloc.2'
    # 区间内没有分支，移动不改变其外的分支目标及原 foreach/finally 边界。
    assert not any('BrTarget' in i.opcode.operand_type.name
                   for i in vip_body.instructions if 0xae <= i.offset < 0x11a)
    data[vip_start+0xae:vip_start+0x11a] = (
        slice_code('TabVipShop', 'OnMsgCallback', 0x108, 0x11a)
        + slice_code('TabVipShop', 'OnMsgCallback', 0xae, 0x108))
    print('Patched TabVipShop ownership before native card initialization')

    # 原仓库先刷新宝箱栏再刷新资产，空的未恢复目录会在 boxes[0] 中断整个回调。
    # 使用原 ICollection.Count 检查空目录，仅隐藏该栏；资产/捐献回调继续原路径。
    # 有真实目录时重新显示并执行原全部分支。缩短可达短跳转，保持程序集大小。
    def compact_guarded_method(owner, name, inserts):
        start, original = body(owner, name)
        assert original.header_size == 12 and not original.exception_handlers
        short = set()
        instructions = original.instructions
        def positions():
            mapping, length = {}, 0
            for ins in instructions:
                mapping[ins.offset] = length
                length += len(inserts.get(ins.offset, b'')) + (2 if ins.offset in short else ins.size)
            return mapping
        while True:
            mapping = positions()
            changed = False
            for ins in instructions:
                if 'BrTarget' not in ins.opcode.operand_type.name or ins.offset in short:
                    continue
                assert ins.size == 5 and 0x38 <= raw[start+ins.offset] <= 0x44
                distance = mapping[ins.operand] - mapping[ins.offset] - len(inserts.get(ins.offset, b'')) - 2
                if -128 <= distance <= 127:
                    short.add(ins.offset)
                    changed = True
            if not changed:
                break
        mapping = positions()
        code = bytearray()
        for ins in instructions:
            code.extend(inserts.get(ins.offset, b''))
            encoded = raw[start+ins.offset:start+ins.offset+ins.size]
            if 'BrTarget' in ins.opcode.operand_type.name:
                if ins.offset in short:
                    encoded = bytes([encoded[0]-0x0d]) + struct.pack('<b', mapping[ins.operand]-len(code)-2)
                else:
                    encoded = encoded[:1] + struct.pack('<i', mapping[ins.operand]-len(code)-5)
            code.extend(encoded)
        assert len(code) <= original.code_size
        replace(owner, name, bytes(code))
        struct.pack_into('<I', data, start+4, len(code))

    collection_count = pe.net.mdtables.MemberRef.rows[0x97d-1]
    assert str(collection_count.Name) == 'get_Count' and str(collection_count.Class.row.TypeName) == 'ICollection'
    assert str(pe.net.mdtables.MemberRef.rows[0x544-1].Name) == 'get_gameObject'
    assert str(pe.net.mdtables.MemberRef.rows[0x542-1].Name) == 'SetActive'
    show_box = b'\x02\x6f'+struct.pack('<I', 0x0a000544)+b'\x17\x6f'+struct.pack('<I', 0x0a000542)
    hide_box = b'\x02\x6f'+struct.pack('<I', 0x0a000544)+b'\x16\x6f'+struct.pack('<I', 0x0a000542)+b'\x2a'
    guard_box = b'\x03\x6f'+struct.pack('<I', 0x0a00097d)+b'\x2d'+bytes([len(hide_box)])+hide_box
    compact_guarded_method('UnionWarehouseTabBox', 'Init', {0x0c: show_box, 0x5d: guard_box})

    if demo_replay_controls:
        # 原调试录像面板保留fightId/serverId输入和原协议，只在验收构建中显示。
        replay_start, replay_body = body('PlaybackKit', 'Start')
        active = [i for i in replay_body.instructions if i.opcode.name == 'ldc.i4.0']
        assert len(active) == 1 and not replay_body.exception_handlers
        assert raw[replay_start + active[0].offset] == 0x16
        data[replay_start + active[0].offset] = 0x17
        print('Enabled original PlaybackKit controls for demo acceptance')

    # 部分原入口保留isBtn=true，使第一次DoOpen在状态为空时提前返回，留下预制页的占位次数。
    # 每次打开都查询原状态，保留原循环、finally、响应回调及真实挑战按钮的条件校验。
    daily_page_start, daily_page_body = body('ActivityCopyPage', 'DoOpen')
    daily_ins = {i.offset: i for i in daily_page_body.instructions}
    assert daily_ins[0x32].opcode.name == 'ldarg.0' and daily_ins[0x33].opcode.name == 'ldfld'
    assert str(pe.net.mdtables.Field.rows[(daily_ins[0x33].operand.value & 0xffffff)-1].Name) == 'isBtn'
    assert daily_ins[0x38].opcode.name == 'brfalse' and daily_ins[0x38].operand == 0x3e
    data[daily_page_start+0x32:daily_page_start+0x38] = b'\x16' + b'\0' * 5
    print('Patched native resource-copy state refresh on every open')

    # Reuse only the production URL's #US slot; no other string offsets move.
    _, b = body('ServerConfig', 'GetServerUrl')
    ins = next(i for i in b.instructions if i.opcode.name == 'ldstr'
               and pe.net.user_strings.get(i.operand.value & 0xffffff).value.startswith('http://djsy.'))
    idx = ins.operand.value & 0xffffff
    old = pe.net.user_strings.get(idx)
    content = url.encode('utf-16le') + b'\0'
    assert len(content) < 128 and len(content) <= old.item_size, 'URL too long for original slot'
    offset = pe.net.user_strings.file_offset + idx
    data[offset:offset+len(content)+1] = bytes([len(content)]) + content
    replace('ServerConfig', 'GetServerUrl', b'\x72' + struct.pack('<I', ins.operand.value) + b'\x2a')
    # Use the existing username/password UI and its original transition to server selection.
    replace('LoginAuth', 'Auth', slice_code('LoginAuth', 'Auth', 0x1c, 0x32)
            + slice_code('LoginAuth', 'Auth', 0x50, 0x65) + b'\x2a')
    replace('LoginUI', 'OnSwitchAccountButtonClick',
            slice_code('LoginUI', 'OnSwitchAccountButtonClick', 0x0c, 0x17)
            + slice_code('LoginUI', 'OnSwitchAccountButtonClick', 0x27, 0x3e) + b'\x2a')
    for name, value in [('get_GetChannelId', 'AND0003'), ('get_GetChannelName', 'AND_TEST')]:
        _, b = body('GameConfig', name)
        i = next(i for i in b.instructions if i.opcode.name == 'ldstr'
                 and pe.net.user_strings.get(i.operand.value & 0xffffff).value == value)
        replace('GameConfig', name, b'\x72'+struct.pack('<I', i.operand.value)+b'\x2a')
    # The original Android SHOP_ID getter calls the retired Java channel SDK
    # before its common fallback. Freeze the local catalogue to that original
    # fallback so opening the native charge page sends CSGetPaymentInfo.
    _, shop_body = body('GameConfig', 'get_GetShopId')
    shop_string = next(i for i in shop_body.instructions if i.opcode.name == 'ldstr'
                       and pe.net.user_strings.get(i.operand.value & 0xffffff).value == 'common')
    replace('GameConfig', 'get_GetShopId', b'\x72' + struct.pack('<I', shop_string.operand.value) + b'\x2a')
    # The original month-card page bypasses GameConfig with literal placeholders.
    # Reuse the exact local channel/common tokens already selected above, keeping
    # its request/callback/payment list and all instruction offsets unchanged.
    _, channel_body = body('GameConfig', 'get_GetChannelId')
    channel_string = next(i for i in channel_body.instructions if i.opcode.name == 'ldstr'
                          and pe.net.user_strings.get(i.operand.value & 0xffffff).value == 'AND0003')
    month_start, month_body = body('UIActivityMonthCard', 'AskChargeList')
    assert not month_body.exception_handlers
    month_literals = {pe.net.user_strings.get(i.operand.value & 0xffffff).value: i
                      for i in month_body.instructions if i.opcode.name == 'ldstr'}
    assert set(month_literals) == {'channelId', 'shopId'}
    for literal, token in [('channelId', channel_string.operand.value), ('shopId', shop_string.operand.value)]:
        position = month_start + month_literals[literal].offset + 1
        struct.pack_into('<I', data, position, token)
    print('Patched original month-card catalogue literal channel/shop tokens')
    # The original charge cell used only the diamond quantity as its heading.
    # Display the server-frozen product name so red-card and equipment test
    # bundles are distinguishable; retain the separate bonus and price labels.
    item_start, item_body = body('ChargeItem', 'InitItem')
    item_offsets = {i.offset: i for i in item_body.instructions}
    assert item_offsets[0x135].opcode.name == 'ldstr'
    assert item_offsets[0x152].opcode.name == 'callvirt'
    name_getter = next(i for i in item_body.instructions if i.offset == 0x2c8)
    assert name_getter.opcode.name == 'callvirt'
    label_code = b'\x03\x6f' + struct.pack('<I', name_getter.operand.value)
    data[item_start+0x135:item_start+0x152] = label_code + b'\0' * (0x152-0x135-len(label_code))
    for owner, name in [('PlatformSDKCallBack','Update'), ('PlatformSDK','Splash'), ('PlatformSDK','Init'),
                        ('PlatformSDK','InitJpush'), ('PlatformSDK','SDKSubmitData')]:
        replace(owner, name, b'\x2a')
    # A level-changing SCPlayer writes level before checking the retired channel
    # SDK, and only then copies coin and the remaining snapshot fields. Skip
    # that complete, stack-balanced reporting block so a missing Java SDK cannot
    # leave the native player half updated. Preserve the existing level setter,
    # common snapshot path, method size, branch targets and original log strings.
    start, reset = body('UIPlayerData', 'ResetPlayer')
    reset_offsets = {ins.offset: ins for ins in reset.instructions}
    assert not reset.exception_handlers
    assert reset_offsets[0x5d].opcode.name == 'call'
    assert reset_offsets[0x62].opcode.name == 'call'
    assert reset_offsets[0x77].opcode.name == 'call'
    assert reset_offsets[0x96].opcode.name == 'br' and reset_offsets[0x96].operand == 0xa7
    assert reset_offsets[0xf4].opcode.name == 'callvirt'
    assert reset_offsets[0xf9].opcode.name == 'stfld'
    data[start+0x62:start+0x96] = b'\0' * (0x96-0x62)
    print('Patched UIPlayerData ResetPlayer retired SDK reporting')
    # Main-hero growth reads the player's level synchronously; formation power
    # is a separate cached value. Publish the level, then invoke the original
    # two-pass hero/formation refresh. Keep equality and null-hero guards, also
    # stop safely before looking up a hero on an uninitialized player instance.
    start, setter = body('UIPlayerData', 'set_level')
    setter_offsets = {ins.offset: ins for ins in setter.instructions}
    assert not setter.exception_handlers and setter.header_size == 12
    assert setter_offsets[0x13].opcode.name == 'beq' and setter_offsets[0x13].operand == 0x36
    assert setter_offsets[0x2b].opcode.name == 'brfalse' and setter_offsets[0x2b].operand == 0x36
    assert setter_offsets[0x31].opcode.name == 'callvirt'
    assert setter_offsets[0x38].opcode.name == 'stfld'
    assert setter_offsets[0x3d].opcode.name == 'ret'
    formation_refresh = methods['UIInstPlayerData', 'ResetAllFormationAttribute']
    assert formation_refresh.Signature.value == b'\x20\x00\x01'  # instance void()
    formation_token = 0x06000000 | next(index for index, row in
        enumerate(pe.net.mdtables.MethodDef.rows, 1) if row is formation_refresh)
    setter_code = bytearray(raw[start+0x0c:start+0x13] + b'\x2e\x00'
                            + raw[start+0x36:start+0x3d]
                            + raw[start+0x18:start+0x1e] + b'\x25\x2c\x00\x25'
                            + raw[start+0x1e:start+0x29] + b'\x2c\x00'
                            + b'\x6f' + struct.pack('<I', formation_token) + b'\x2a')
    null_pop = len(setter_code)
    setter_code.extend(b'\x26\x2a')
    return_offset = len(setter_code) - 1
    for operand_offset, target in ((8, return_offset), (24, null_pop), (38, null_pop)):
        displacement = target - operand_offset - 1
        assert -128 <= displacement <= 127
        setter_code[operand_offset] = displacement & 0xff
    assert len(setter_code) == 47 and len(setter_code) <= setter.code_size
    data[start+setter.header_size:start+setter.header_size+setter.code_size] = (
        setter_code + b'\0' * (setter.code_size-len(setter_code)))
    struct.pack_into('<H', data, start+2, max(setter.max_stack, 3))
    print('Patched UIPlayerData set_level before hero and formation refresh')
    replace('PlatformSDKCallBack', 'Start',
            slice_code('PlatformSDKCallBack', 'Start', 0x26, 0x31) + b'\x2a')
    # The original Android login logs the password. Remove just that balanced block,
    # keeping every branch offset and the surrounding request construction intact.
    start, _ = body('LoginUserLogin', 'UserLogin')
    data[start+0x91:start+0xb1] = b'\0' * (0xb1-0x91)
    # Existing-character SwitchState immediately sends CSPlayerLogin. Save the new
    # gateway/session before that transition; both relocated blocks retain their
    # internal relative branch distances, and total method length is unchanged.
    start, _ = body('LoginUserLogin', 'UserLoginCallBack')
    data[start+0x78:start+0xc3] = (raw[start+0xa2:start+0xc3]
                                + raw[start+0x78:start+0xa2])
    # A first-open GetPassedCopy response must replace the previous mission selection.
    # Consume the original IsNullOrEmpty result while making both branch outcomes
    # continue to the server's mission/chapter assignment. Method size and stack stay intact.
    start, b = body('UICopyData', 'SetPassedCopyInfo')
    for offset in (0x124, 0x13e):
        branch = next(i for i in b.instructions if i.offset == offset)
        assert branch.opcode.name == 'brfalse' and branch.size == 5
        assert raw[start+offset:start+offset+5] == b'\x39\x0b\0\0\0'
        data[start+offset+1:start+offset+5] = b'\0' * 4
    print('Patched UICopyData SetPassedCopyInfo current mission selection')
    # Modern wide displays exceed UIAutoSize's last supported aspect ratio.
    # Let the existing NGUI FitBoth path preserve the whole design rectangle,
    # rather than scaling to width and clipping the top/bottom of the 640-high UI.
    start, b = body('UIRoot', 'get_constraint')
    constraint = next(i for i in b.instructions if i.offset == 0x24)
    assert not b.exception_handlers and constraint.opcode.name == 'ldc.i4.2'
    assert raw[start+0x24] == 0x18
    data[start+0x24] = 0x16  # ldc.i4.0: FitBoth
    print('Patched UIRoot width-only constraint to preserve full viewport')
    # The single-spin display mutates the same reward object later applied to the
    # wallet. Keep coin/diamond increments intact, using ordinary reward icons.
    # Replace only setters with equal-size stack-balanced pops; EH and branches stay.
    start, b = body('UIActivityBigTurn', 'GetResultObject')
    for offset, setter in ((0x47, 'set_id'), (0x94, 'set_num'), (0xc8, 'set_num')):
        ins = next(i for i in b.instructions if i.offset == offset)
        assert ins.opcode.name == 'callvirt' and ins.size == 5
        assert ins.operand.value >> 24 == 6
        method = pe.net.mdtables.MethodDef.rows[(ins.operand.value & 0xffffff)-1]
        assert str(method.Name) == setter
        data[start+offset:start+offset+5] = b'\x26\x26\x00\x00\x00'
    print('Patched single-spin display to preserve wallet rewards')
    # Prefer the restored wheel on a default activity open. Init still checks that
    # the server advertised tab 34 and honors any explicit tab option.
    # Shorten the preceding unconditional branch to make one byte for ldc.i4.s.
    start, b = body('UIActivity', 'DoOpen')
    ins_by_offset = {i.offset:i for i in b.instructions}
    assert ins_by_offset[0x1c].opcode.name == 'brfalse' and ins_by_offset[0x1c].operand == 0x41
    assert ins_by_offset[0x3c].opcode.name == 'br' and ins_by_offset[0x3c].operand == 0x48
    assert ins_by_offset[0x43].opcode.name == 'stfld'
    data[start+0x1d:start+0x21] = struct.pack('<i', 0x40-0x21)
    data[start+0x3c:start+0x48] = (b'\x2b\x0a\x00\x00\x02\x1f\x22'
                                   + raw[start+0x43:start+0x48])
    print('Patched default activity selection to the advertised wheel tab')
    # Core systems can be tested without the tutorial forcing another story mission.
    # Preserve character creation, combat tutorials and all saved quest/guide progress.
    # Rebuild without the explicit flag to restore the original town guide triggers.
    if optional_town_guides:
        for name in ('OnLogin', 'OnLevelUp', 'OnQuestComplete'):
            replace('GuideManager', name, b'\x2a', drop_exception_handlers=True)
    # Copy monsters use the unqualified TaskId as their RoomCharactor key.
    # A local player ID such as 1 collides with task 1: CreateCharactor destroys
    # the player and replaces it with the monster; victory then removes it.
    # Namespace only the copy-room self key, including its pet owner/lookup.
    # Protocol player IDs, task IDs, world rooms and save data remain original.
    prefix = b'\x72' + struct.pack('<I', 0x7001348f)
    concat = b'\x28' + struct.pack('<I', 0x0a000009)
    assert pe.net.user_strings.get(0x1348f).value == 'self'
    concat_ref = pe.net.mdtables.MemberRef.rows[8]
    assert str(concat_ref.Name) == 'Concat' and concat_ref.Signature.value == bytes.fromhex('00020e0e0e')
    # The disabled channel SDK payload owns sufficient unused space. Shorten
    # its already-empty body before storing two complete managed method bodies
    # there, preserving assembly length, sections and every metadata token.
    storage, storage_body = body('PlatformSDK', 'SDKSubmitData')
    assert storage_body.header_size == 12 and not storage_body.exception_handlers
    assert data[storage+12] == 0x2a
    struct.pack_into('<I', data, storage+4, 1)
    cursor = (storage+13+3) & ~3
    storage_end = storage + storage_body.header_size + storage_body.code_size

    def relocate_method(owner, name, inserts, *, replacements=None, extra_stack=1):
        nonlocal cursor
        replacements = replacements or {}
        start, original = body(owner, name)
        assert original.header_size in (1, 12) and not original.exception_handlers
        offsets = {i.offset for i in original.instructions}
        assert set(inserts) <= offsets and set(replacements) <= offsets
        mapping = {}
        length = 0
        for ins in original.instructions:
            mapping[ins.offset] = length
            length += len(inserts.get(ins.offset, b'')) + len(replacements.get(ins.offset,
                           raw[start+ins.offset:start+ins.offset+ins.size]))
        code = bytearray()
        for ins in original.instructions:
            code.extend(inserts.get(ins.offset, b''))
            encoded = replacements.get(ins.offset, raw[start+ins.offset:start+ins.offset+ins.size])
            if 'BrTarget' in ins.opcode.operand_type.name:
                assert ins.offset not in replacements
                assert ins.size == 5
                encoded = encoded[:1] + struct.pack('<i', mapping[ins.operand] - len(code) - 5)
            elif ins.opcode.name == 'switch':
                assert ins.offset not in replacements
                assert ins.size == 5 + 4*len(ins.operand)
                # Switch targets are relative to the end of the complete jump
                # table. Preserve each coroutine state after inserted guards.
                encoded = encoded[:5] + b''.join(
                    struct.pack('<i', mapping[target] - len(code) - ins.size)
                    for target in ins.operand)
            code.extend(encoded)
        if original.header_size == 12:
            header = bytearray(raw[start:start+12])
        else:
            # Tiny getters have no locals. Emit a normal three-word fat header
            # when a guarded UI projection needs more than the tiny body space.
            assert original.local_var_sig_tok is None
            header = bytearray(struct.pack('<HHII', 0x3003, 8, 0, 0))
        struct.pack_into('<H', header, 2, original.max_stack+extra_stack)
        struct.pack_into('<I', header, 4, len(code))
        assert cursor + 12 + len(code) <= storage_end
        data[cursor:cursor+12+len(code)] = header + code
        row = methods[owner, name]
        struct.pack_into('<I', data, row.struct.get_file_offset(), pe.get_rva_from_offset(cursor))
        cursor = (cursor + 12 + len(code) + 3) & ~3
        print('Patched', owner, name)

    relocate_method('RoomCopy', 'LoadPlayer', {0x0c: prefix, 0x1b: concat})
    relocate_method('RoomCopy', 'LoadPet', {0x68: prefix, 0x78: concat, 0x83: prefix, 0x94: concat})
    saved_storage = (cursor, storage_end)
    # 本地支付仅使用原订单查询回调；退役渠道的字符串支付回调不再确认任何订单。
    # 收回该独立方法的代码空间，保留PE节大小、元数据及所有实际订单路径。
    pet_storage, pet_storage_body = body('PlatformSDKCallBack', 'PayCallBackFun')
    assert methods['PlatformSDKCallBack', 'PayCallBackFun'].Signature.value == b'\x20\x01\x01\x0e'
    assert pet_storage_body.header_size == 12
    flags = struct.unpack_from('<H', raw, pet_storage)[0]
    struct.pack_into('<H', data, pet_storage, flags & ~0x8)
    struct.pack_into('<I', data, pet_storage+4, 1)
    data[pet_storage+12:pet_storage+12+pet_storage_body.code_size] = b'\x2a' + b'\0'*(pet_storage_body.code_size-1)
    cursor = (pet_storage+13+3) & ~3
    storage_end = pet_storage+12+pet_storage_body.code_size
    # 自由测试保留原首次精灵入口：服务端只在真实首领奖励尚未领取时返回原配置预览。
    # 原单抽按钮调用原OnRecruitGuide；已领取后继续原普通单抽，奖池门禁由服务端保留。
    def own_method(owner, name):
        row = methods[owner, name]
        index = next(i for i, candidate in enumerate(pe.net.mdtables.MethodDef.rows, 1)
                     if candidate is row)
        return struct.pack('<I', 0x06000000 | index)

    recruit_info = b'\x02\x28' + own_method('TabPetRecruit', 'get_recruitInfo')
    recruit_value = b'\x6f' + own_method('SCPetRecruitGet', 'get_recruit')
    reward_value = b'\x6f' + own_method('PetRecruit', 'get_reward')
    once_start, once_body = body('TabPetRecruit', 'OnRecruitOnce')
    assert not once_body.exception_handlers
    # 从原单抽读取相同的Dict实例、配置DAO和currentData，末端仅换成原首领奖励字段。
    config_chain = raw[once_start+0x1d:once_start+0x2c]
    first_field = pe.net.mdtables.Field.rows[0x295e-1]
    assert str(first_field.Name) == 'PetFirstRecruitGet' and first_field.Signature.value == b'\x06\x0e'
    pending_first = (recruit_info + recruit_value + reward_value + config_chain
                     + b'\x7b' + struct.pack('<I', 0x0400295e)
                     + b'\x28' + struct.pack('<I', 0x0a000047))
    guard_first = (pending_first + b'\x2c\x07\x02\x28'
                   + own_method('TabPetRecruit', 'OnRecruitGuide') + b'\x2a')
    relocate_method('TabPetRecruit', 'OnRecruitOnce', {0x17: guard_first})
    # 保留原免费文案/根节点，仅由真实首领奖励补充免费显示条件，不修改免费次数。
    # 零nextFree表示普通免费规则未开放，不启动反复查询状态的倒计时。
    pet_msg_start, _ = body('TabPetRecruit', 'OnMsgCallback')
    # 零时间还要隐藏预制页的倒计时占位文字，避免展示不存在的免费招募时间。
    hide_zero_timer = (raw[pet_msg_start+0x100:pet_msg_start+0x10b]
                       + b'\x16' + raw[pet_msg_start+0x10c:pet_msg_start+0x111] + b'\x2a')
    stop_zero_timer = (recruit_info + recruit_value
                       + b'\x6f' + own_method('PetRecruit', 'get_nextFree')
                       + b'\x16\x6a\xfe\x02\x2d' + bytes([len(hide_zero_timer)]) + hide_zero_timer)
    relocate_method('TabPetRecruit', 'OnMsgCallback',
                    {0xfa: b'\x16\xfe\x02' + pending_first + b'\x60',
                     0x177: stop_zero_timer})
    # 原首领回调错误地从普通免费次数扣一次。原响应添加完整实例后刷新真实状态，
    # 保留foreach/finally、动画、返回点和原存档教程进度。
    guide_start, guide_body = body('TabPetRecruit', 'RecruitGuideCallback')
    guide_ins = {i.offset: i for i in guide_body.instructions}
    assert guide_ins[0x90].opcode.name == 'ldarg.0'
    assert guide_ins[0xb2].opcode.name == 'callvirt'
    assert str(pe.net.mdtables.MethodDef.rows[(guide_ins[0xb2].operand.value & 0xffffff)-1].Name) == 'set_num'
    # 原构造已初始化这个列表；首抽清空普通购买结果缓存，确保两种结果文案能准确区分。
    clear_first_items = b'\x02\x7b' + struct.pack('<I', 0x04004d10) + b'\x6f' + struct.pack('<I', 0x0a000ab3)
    refresh_first = (clear_first_items + recruit_info + recruit_value + b'\x7e' + struct.pack('<I', 0x0a00005f)
                     + b'\x6f' + own_method('PetRecruit', 'set_reward')
                     + b'\x02\x16\x28' + own_method('TabPetRecruit', 'UpdateAll'))
    assert len(refresh_first) <= 0xb7-0x90
    data[guide_start+0x90:guide_start+0xb7] = refresh_first + b'\0'*(0xb7-0x90-len(refresh_first))
    # 首抽结果没有普通购买资源清单；保持其原卡片动画，同时隐藏通用购买金币标题。
    # 普通招募仍显示原计算文案，避免首次领取被误写成金币购买。
    result_storage, result_storage_body = body('PlatformSDK', 'SDKSubmitPayData')
    assert methods['PlatformSDK', 'SDKSubmitPayData'].Signature.value[2] == 1
    assert result_storage_body.header_size == 12
    struct.pack_into('<H', data, result_storage, struct.unpack_from('<H', raw, result_storage)[0] & ~0x8)
    struct.pack_into('<I', data, result_storage+4, 1)
    data[result_storage+12:result_storage+12+result_storage_body.code_size] = b'\x2a' + b'\0'*(result_storage_body.code_size-1)
    cursor = (result_storage+13+3) & ~3
    storage_end = result_storage+12+result_storage_body.code_size
    for token, expected in ((0x04004bd8, 'extraGetLab'), (0x04004bda, 'itemsList'), (0x04004d10, 'itemsList')):
        assert str(pe.net.mdtables.Field.rows[(token & 0xffffff)-1].Name) == expected
    result_title = (b'\x02\x7b' + struct.pack('<I', 0x04004bd8)
                    + b'\x6f' + struct.pack('<I', 0x0a000544)
                    + b'\x02\x7b' + struct.pack('<I', 0x04004bda)
                    + b'\x6f' + struct.pack('<I', 0x0a000ab1)
                    + b'\x16\xfe\x02\x6f' + struct.pack('<I', 0x0a000542))
    relocate_method('PetRecruitResultPage', 'DoOpen', {0x73: result_title})
    print('Restored native first-pet button and actual free-state refresh')
    cursor, storage_end = saved_storage
    # Main and daily copy replies supply SCPlayer before their reward panels close.
    # Preserve that authoritative coin value across the later AddResultObj call,
    # while retaining normal item/hero additions and the unchanged reward display.
    # The coin field is public Int64; no locals, strings or metadata entries are added.
    player_type = next(t for t in pe.net.mdtables.TypeDef if str(t.TypeName) == 'UIPlayerData')
    coin_field = next(f for f in player_type.FieldList if str(f.row.Name) == 'coin')
    assert coin_field.row.Flags.fdPublic and coin_field.row.Signature.value == b'\x06\x0a'
    coin_token = struct.pack('<I', 0x04000000 | coin_field.row_index)
    balance_start, balance_body = body('CopyBalance', 'AddBalanceReward')
    balance_ins = {i.offset: i for i in balance_body.instructions}
    assert balance_ins[0x33].opcode.name == 'ldsfld'
    assert balance_ins[0x3e].opcode.name == 'callvirt'
    assert str(pe.net.mdtables.MethodDef.rows[
        (balance_ins[0x3e].operand.value & 0xffffff)-1].Name) == 'AddResultObj'
    assert balance_ins[0x6f].opcode.name == 'ldsfld'
    assert balance_ins[0x7a].opcode.name == 'callvirt'
    assert str(pe.net.mdtables.MethodDef.rows[
        (balance_ins[0x7a].operand.value & 0xffffff)-1].Name) == 'AddResultObj'
    snapshot_coin = (raw[balance_start+0x33:balance_start+0x38]
                     + b'\x25\x7b' + coin_token)
    add_and_restore = (raw[balance_start+0x3e:balance_start+0x43]
                       + b'\x7d' + coin_token)
    daily_snapshot = (raw[balance_start+0x6f:balance_start+0x74]
                      + b'\x25\x7b' + coin_token)
    daily_restore = (raw[balance_start+0x7a:balance_start+0x7f]
                     + b'\x7d' + coin_token)
    # Restoring inside the call replacement keeps the null-gain branch stack balanced.
    relocate_method('CopyBalance', 'AddBalanceReward',
                    {0x33: snapshot_coin, 0x6f: daily_snapshot},
                    replacements={0x3e: add_and_restore, 0x7a: daily_restore}, extra_stack=2)
    # Route the existing strong-recruit button to its original one-time tutorial
    # request while the player remains at the first cleared quest's guide stage.
    # The server validates quest_1 and returns SCPlayer with the advanced stage;
    # subsequent button presses retain the original paid/free recruitment path.
    # Compare the player's guide ID with the original DAO row for schedule 1,
    # reusing ResetPlayer's lookup tokens without adding strings or metadata.
    reset_start, reset_body = body('UIPlayerData', 'ResetPlayer')
    reset_ins = {i.offset: i for i in reset_body.instructions}
    assert reset_ins[0x1e4].opcode.name == 'call'
    assert reset_ins[0x204].operand.value == 0x040032af
    equality = next(i for i, row in enumerate(pe.net.mdtables.MemberRef.rows, 1)
                    if str(row.Name) == 'op_Equality'
                    and row.Signature.value == bytes.fromhex('0002020e0e'))
    recruit_guide = b'\x02\x28' + struct.pack('<I', 0x060053a3) + b'\x2a'
    assert str(pe.net.mdtables.MethodDef.rows[0x53a3-1].Name) == 'OnGuideOne'
    guide_check = (raw[reset_start+0x1e4:reset_start+0x1ee] + b'\x17'
                   + raw[reset_start+0x1f4:reset_start+0x204]
                   + raw[balance_start+0x33:balance_start+0x38]
                   + b'\x7b' + struct.pack('<I', 0x040032af)
                   + b'\x28' + struct.pack('<I', 0x0a000000 | equality))
    guide_route = guide_check + b'\x2c' + bytes([len(recruit_guide)]) + recruit_guide
    # AddBalanceReward now points to its relocated SDK storage. Reclaim its
    # unreferenced original body for this method, keeping every section size.
    moved_row = methods['CopyBalance', 'AddBalanceReward']
    assert struct.unpack_from('<I', data, moved_row.struct.get_file_offset())[0] != pe.get_rva_from_offset(balance_start)
    cursor = balance_start
    storage_end = balance_start + balance_body.header_size + balance_body.code_size
    relocate_method('TabRecruit', 'OnHighOnce', {0x0c: guide_route}, extra_stack=2)
    # Present the original free label/red point for the one-time guide claim.
    # Preserve the message's stored cooldown and all normal/ten-draw rules.
    # Before UI tables/player state exist, the getter retains its original value.
    free_start, free_body = body('WarriorRecruit', 'get_freeTime')
    cost_row = methods['WarriorRecruit', 'get_costType']
    cost_index = next(i for i, row in enumerate(pe.net.mdtables.MethodDef.rows, 1)
                      if row is cost_row)
    free_projection = bytearray(b'\x02\x6f' + struct.pack('<I', 0x06000000 | cost_index) + b'\x18')
    fallback_jumps = []
    def fallback_jump(opcode):
        free_projection.append(opcode)
        fallback_jumps.append(len(free_projection))
        free_projection.append(0)
    fallback_jump(0x33)  # bne.un.s: normal recruit keeps the original cooldown.
    player_load = raw[balance_start+0x33:balance_start+0x38]
    free_projection.extend(player_load)
    fallback_jump(0x2c)
    free_projection.extend(player_load + b'\x7b' + struct.pack('<I', 0x040032af))
    fallback_jump(0x2c)
    free_projection.extend(guide_check)
    fallback_jump(0x2c)
    free_projection.extend(b'\x16\x6a\x2a')  # ldc.i4.0; conv.i8; ret
    for position in fallback_jumps:
        distance = len(free_projection) - position - 1
        assert 0 <= distance <= 127
        free_projection[position] = distance
    old_high_start, old_high_body = body('TabRecruit', 'OnHighOnce')
    high_row = methods['TabRecruit', 'OnHighOnce']
    assert struct.unpack_from('<I', data, high_row.struct.get_file_offset())[0] != high_row.Rva
    cursor = old_high_start
    storage_end = old_high_start + old_high_body.header_size + old_high_body.code_size
    relocate_method('WarriorRecruit', 'get_freeTime',
                    {free_body.instructions[0].offset: bytes(free_projection)}, extra_stack=2)
    if repair_copy_navigation:
        # 原 PlaybackOver 会同步切回场景并清理角色；ResultState 随后
        # 继续访问已清理的 BattleCharacter。录像先走原 ShowResult 后返回，
        # 普通战斗仍从原类型/镜头/角色动画分支继续。模拟与回包保持原值。
        result_start, result_body = body('ResultState', 'Enter')
        result_by_offset = {i.offset: i for i in result_body.instructions}
        assert result_body.header_size == 12
        assert result_by_offset[0x0c].opcode.name == 'ldstr'
        assert result_by_offset[0x2b].opcode.name == 'ldsfld'
        assert result_by_offset[0x1d5].opcode.name == 'call'
        assert result_by_offset[0x1df].opcode.name == 'callvirt'
        battle_type = next(t for t in pe.net.mdtables.TypeDef if str(t.TypeName) == 'BattleData')
        playback_flag = next(0x04000000 | f.row_index for f in battle_type.FieldList
                             if str(f.row.Name) == 'isPlayBack')
        playback_result = (slice_code('ResultState', 'Enter', 0x2b, 0x30)
                           + b'\x7b' + struct.pack('<I', playback_flag)
                           + b'\x2c\x13'
                           + slice_code('ResultState', 'Enter', 0x1d5, 0x1e4)
                           + b'\x2a')
        assert len(playback_result) == 28
        # 此区间原为纯日志、没有分支目标或异常区域边界；保留后续全部偏移。
        assert not any('BrTarget' in i.opcode.operand_type.name and 0x0c < i.operand < 0x2b
                       for i in result_body.instructions)
        data[result_start+0x0c:result_start+0x2b] = playback_result + b'\0'*3
        # The native diagnostic playback uses the recruit transition. Preserve
        # its source room and re-enter it before the original UI return, so the
        # loaded battle map does not remain behind the town/union interface.
        static_type = next(t for t in pe.net.mdtables.TypeDef if str(t.TypeName) == 'StaticData')
        room_manager = next(t for t in pe.net.mdtables.TypeDef if str(t.TypeName) == 'RoomManager')
        source_room = next(0x04000000 | f.row_index for f in static_type.FieldList
                           if str(f.row.Name) == 'awakeCopyPlaybackFromScene')
        current_room = next(0x04000000 | f.row_index for f in room_manager.FieldList
                            if str(f.row.Name) == 'curSceneType')
        entry_owner = '<_RoomToBattleRecruit>c__Iterator5E'
        exit_owner = '<_BattleRecruitToRoom>c__Iterator5F'
        _, entry_body = body(entry_owner, 'MoveNext')
        _, exit_body = body(exit_owner, 'MoveNext')
        assert next(i for i in entry_body.instructions if i.offset == 0x59).opcode.name == 'ldc.i4.1'
        assert next(i for i in exit_body.instructions if i.offset == 0x66).opcode.name == 'ldarg.0'
        # Reuse the original enum field that already stores awake playback's
        # source scene. The calculator input and all engine methods stay intact.
        playback_guard = slice_code('ResultState', 'Enter', 0x2b, 0x30) + b'\x7b' + struct.pack('<I', playback_flag)
        save_source = (playback_guard + b'\x2c\x0a' + b'\x7e' + struct.pack('<I', current_room)
                       + b'\x80' + struct.pack('<I', source_room))
        restore_args = b'\x7e' + struct.pack('<I', source_room) + slice_code(entry_owner, 'MoveNext', 0x5a, 0x65)
        assert len(restore_args) == 16
        restore_source = playback_guard + b'\x2c\x10' + restore_args
        # Local password login bypasses the retired channel user callback.
        # Reserve its unused body for the two guarded coroutine relocations.
        replay_storage, replay_storage_body = body('PlatformSDKCallBack', 'UserCallBackFun')
        assert replay_storage_body.header_size == 12 and not replay_storage_body.exception_handlers
        struct.pack_into('<I', data, replay_storage+4, 1)
        data[replay_storage+12:replay_storage+12+replay_storage_body.code_size] = (
            b'\x2a' + b'\0'*(replay_storage_body.code_size-1))
        cursor = (replay_storage+13+3) & ~3
        storage_end = replay_storage+12+replay_storage_body.code_size
        relocate_method(entry_owner, 'MoveNext', {0x59: save_source}, extra_stack=0)
        # ChangeUIBack invokes TownPage.CoverdBackFromBattle for Daily type 11.
        # Its room-dependent controls must observe the restored room, matching
        # the original BattleToCopy/BattleToAwakeCopy switch-before-UI order.
        relocate_method(exit_owner, 'MoveNext', {0x66: restore_source}, extra_stack=0)
        # Rebind the native agent after the original successful NavMesh sample.
        # SetSpeed already implements the client's disable/enable sequence. Keep
        # its current speed and retain the original failed-sample no-op behavior.
        start, b = body('RCMoveNavigation', 'ResetPosition')
        assert b.header_size == 12 and not b.exception_handlers and b.code_size == 58
        by_offset = {i.offset: i for i in b.instructions}
        assert by_offset[0x24].opcode.name == 'call'
        assert by_offset[0x29].opcode.name == 'brfalse'
        assert by_offset[0x40].opcode.name == 'callvirt'
        assert by_offset[0x45].opcode.name == 'ret'
        current_speed = next(i for i in pe.net.mdtables.TypeDef
                             if str(i.TypeName) == 'RCMoveNavigation')
        refs = {str(m.row.Name): 0x06000000 | m.row_index for m in current_speed.MethodList}
        code = (slice_code('RCMoveNavigation', 'ResetPosition', 0x0c, 0x17)
                + b'\x25'  # retain Transform for the sampled-position assignment
                + slice_code('RCMoveNavigation', 'ResetPosition', 0x17, 0x1e)
                + b'\x1f\x0a\x6b'  # float32(10), original NavMesh search radius
                + slice_code('RCMoveNavigation', 'ResetPosition', 0x23, 0x29)
                + b'\x2c\x19'  # sample false -> pop retained Transform, return
                + slice_code('RCMoveNavigation', 'ResetPosition', 0x39, 0x45)
                + b'\x02\x02\x28' + struct.pack('<I', refs['GetCurrentSpeed'])
                + b'\x28' + struct.pack('<I', refs['SetSpeed'])
                + b'\x2a\x26\x2a')
        assert len(code) == 57
        replace('RCMoveNavigation', 'ResetPosition', code)
        # Union starts its actor before applying the network position. Rebind
        # after the original base callback, then keep kill/map processing.
        union_start, union_body = body('RoomNetUnion', 'SCCurrentPosition')
        union_by_offset = {i.offset: i for i in union_body.instructions}
        assert union_by_offset[0x23].opcode.name == 'stloc.0'
        assert union_by_offset[0x24].opcode.name == 'ldarg.1'
        assert not union_body.exception_handlers
        character_type = next(t for t in pe.net.mdtables.TypeDef if str(t.TypeName) == 'RoomCharactor')
        reset_position = next(0x06000000 | m.row_index for m in character_type.MethodList
                              if str(m.row.Name) == 'ResetPosition')
        # A missing self skips rebind; the original network callback stays intact.
        union_rebind = bytes([0x06, 0x2c, 0x06, 0x06, 0x6f]) + struct.pack('<I', reset_position)
        relocate_method('RoomNetUnion', 'SCCurrentPosition', {0x24: union_rebind}, extra_stack=0)
        # The retained Transform adds one stack slot during SamplePosition.
        original_stack = struct.unpack_from('<H', raw, start+2)[0]
        assert original_stack >= 4
        struct.pack_into('<H', data, start+2, max(original_stack, 5))
        # The original SetSpeed ends with enabled=true. Preserve the previous
        # native enabled state so paused or usingNavAgent=false characters stay
        # disabled when RoomCopy restores every character's sampled position.
        speed_start, speed_body = body('RCMoveNavigation', 'SetSpeed')
        assert speed_body.header_size == 12 and speed_body.code_size == 44
        assert not speed_body.exception_handlers
        enabled_getters = [0x0a000000 | (i+1) for i, r in enumerate(pe.net.mdtables.MemberRef.rows)
                           if str(r.Name) == 'get_enabled'
                           and getattr(r.Class.row, 'TypeName', None) is not None
                           and str(r.Class.row.TypeName) == 'Behaviour']
        assert len(enabled_getters) == 1
        speed_code = (slice_code('RCMoveNavigation', 'SetSpeed', 0x0c, 0x12)
                      + b'\x25'
                      + slice_code('RCMoveNavigation', 'SetSpeed', 0x12, 0x18)
                      + b'\x25\x6f' + struct.pack('<I', enabled_getters[0])
                      + slice_code('RCMoveNavigation', 'SetSpeed', 0x18, 0x1f)
                      + slice_code('RCMoveNavigation', 'SetSpeed', 0x1f, 0x2b)
                      + slice_code('RCMoveNavigation', 'SetSpeed', 0x32, 0x38))
        assert len(speed_code) == 44
        replace('RCMoveNavigation', 'SetSpeed', speed_code)
        # Stack retains agent+enabled beneath the temporary disable receiver.
        speed_stack = struct.unpack_from('<H', raw, speed_start+2)[0]
        struct.pack_into('<H', data, speed_start+2, max(speed_stack, 4))
    check = dnfile.dnPE(data=bytes(data))
    assert check.net.user_strings.get(idx).value == url
    patch_native_test_payments(pe, raw, data, methods)
    patch_pet_strength_all(pe, raw, data, methods)
    return bytes(data)


def patch_manifest(raw, target):
    # Both package names have equal UTF-16 length. Class names use different namespaces.
    old, new = 'com.yunchang.djsy.ewan.qiguo', 'com.yunchang.djsy.ewan.local'
    data = bytearray(raw.replace(old.encode('utf-16le'), new.encode('utf-16le')))
    assert data != raw
    # Decode the string pool, then change only targetSdk's integer attribute.
    strings = []
    pos = 8
    while pos < len(data):
        typ, header, size = struct.unpack_from('<HHI', data, pos)
        if typ == 1:
            count, styles, flags, start = struct.unpack_from('<IIII', data, pos+8)
            assert not flags & 0x100 and styles == 0
            for j in range(count):
                off = pos+start+struct.unpack_from('<I', data, pos+header+4*j)[0]
                n = struct.unpack_from('<H', data, off)[0]
                assert n < 0x8000
                strings.append(data[off+2:off+2+n*2].decode('utf-16le'))
        elif typ == 0x102:
            attrstart, attrsize, count = struct.unpack_from('<HHH', data, pos+24)
            for j in range(count):
                off = pos+16+attrstart+attrsize*j
                name = strings[struct.unpack_from('<I', data, off+4)[0]]
                if name == 'targetSdkVersion':
                    struct.pack_into('<I', data, off+16, target)
                elif name == 'debuggable':
                    struct.pack_into('<I', data, off+16, 0xffffffff)
        pos += size
    return bytes(data)


def apply_planar_movement(raw, output_directory, *, archive_directory=None):
    """Apply the explicitly selected one-byte correction after other DLL stages.

    Keep a content-addressed proof beside the APK and in the P15 evidence
    archive. The patch itself verifies every other byte and all 547 original
    combat engine methods against this exact incoming assembly.
    """
    try:
        from tools.p15_navigation.patch_planar_gate import patch
    except ModuleNotFoundError:
        from p15_navigation.patch_planar_gate import patch
    revised, proof = patch(raw)
    proof['buildFlag'] = '--repair-planar-movement'
    proof['assemblyPathInsideAPK'] = DLL
    source = Path(__file__).resolve().parent/'p15_navigation/patch_planar_gate.py'
    proof['patchSourceSHA256'] = hashlib.sha256(source.read_bytes()).hexdigest()
    contents = (json.dumps(proof, indent=2) + '\n').encode()
    name = 'apk-planar-movement-build-proof-' + hashlib.sha256(contents).hexdigest() + '.json'
    for directory in (Path(output_directory), Path(archive_directory) if archive_directory is not None else Path('analysis/p15')):
        directory.mkdir(parents=True, exist_ok=True)
        target = directory/name
        if target.exists():
            assert target.read_bytes() == contents
        else:
            target.write_bytes(contents)
    print('Planar movement build proof:', Path(output_directory)/name)
    return revised


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', default='http://127.0.0.1:18088/sao/')
    parser.add_argument('--target-sdk', type=int, default=27)
    parser.add_argument('--legacy-manifest', action='store_true')
    parser.add_argument('--optional-town-guides', action='store_true',
                        help='Disable automatic forced town tutorials; retain saved quest progress')
    parser.add_argument('--repair-copy-navigation', action='store_true',
                        help='Rebind original NavMesh agent after sampled copy-position restoration')
    parser.add_argument('--repair-planar-movement', action='store_true',
                        help='Opt in to the strictly matched one-byte original world x/z joystick gate correction')
    parser.add_argument('--demo-replay-controls', action='store_true',
                        help='Show the original fightId/serverId playback panel in this acceptance build')
    parser.add_argument('--native-combat-diagnostics', action='store_true',
                        help='Enable original engine logs for native replay comparison; default off')
    parser.add_argument('--native-batch-diagnostics', action='store_true',
                        help='Add an isolated original-engine runner to PlaybackKit server 60006; default off')
    parser.add_argument('--local-pve-round-compat', action='store_true',
                        help='Add named opt-in PVE UI round recovery observers; private policy file required')
    parser.add_argument('--local-voice-origin',
                        help='Opt in to the original chat PCM adapter at an explicit server origin')
    parser.add_argument('--local-worldboss-origin',
                        help='Commit the precomputed local world boss result at the original battle-end callback')
    parser.add_argument('--repair-guild-result-visibility', action='store_true',
                        help='Hide guild result prefab rows until original server data binds them')
    parser.add_argument('--repair-promotion-rank-self-zone', action='store_true',
                        help='Match original promotion self labels by player and realm IDs')
    parser.add_argument('--repair-finals-empty-slots-and-self-zone', action='store_true',
                        help='Allow genuine empty Finals slots and match self by player and realm')
    parser.add_argument('--repair-kf-replay-joint-viewer', action='store_true',
                        help='Use realm and player identity for the original duplicate-PID KF wrapper')
    args = parser.parse_args()
    if args.local_pve_round_compat and not args.native_batch_diagnostics:
        parser.error('--local-pve-round-compat requires the native batch bridge')
    if args.local_voice_origin and args.legacy_manifest:
        parser.error('--local-voice-origin requires the controlled Android manifest')
    source = Path('apks/original/10056262.apk')
    assert hashlib.file_digest(source.open('rb'), 'sha256').hexdigest() == ORIGINAL_SHA
    out = Path('build'); out.mkdir(exist_ok=True)
    if not args.legacy_manifest:
        sdk = Path.home()/'Library/Android/sdk'
        with tempfile.TemporaryDirectory(prefix='hyjs-client-manifest-') as temporary:
            manifest_source = Path('tools/AndroidManifest.xml')
            if args.local_voice_origin:
                text = manifest_source.read_text()
                permission = '    <uses-permission android:name="android.permission.RECORD_AUDIO" />\n'
                assert 'android.permission.RECORD_AUDIO' not in text
                temporary_manifest = Path(temporary)/'AndroidManifest.xml'
                temporary_manifest.write_text(text.replace('    <uses-sdk', permission+'    <uses-sdk', 1))
                manifest_source = temporary_manifest
            subprocess.run([str(sdk/'build-tools/33.0.0/aapt2'), 'link',
                            '-I', str(sdk/'platforms/android-33/android.jar'),
                            '--manifest', str(manifest_source), '-o', str(out/'manifest.apk')], check=True)
        with zipfile.ZipFile(out/'manifest.apk') as manifest_apk:
            manifest = manifest_apk.read('AndroidManifest.xml')
    with zipfile.ZipFile(source) as apk:
        replacements = {DLL: patch_dll(apk.read(DLL), args.url, optional_town_guides=args.optional_town_guides,
                                      repair_copy_navigation=args.repair_copy_navigation,
                                      demo_replay_controls=args.demo_replay_controls),
                        'AndroidManifest.xml': patch_manifest(apk.read('AndroidManifest.xml'), args.target_sdk)
                        if args.legacy_manifest else manifest}
        if args.native_combat_diagnostics:
            from native_combat_diagnostics import patch
            replacements[DLL] = patch(replacements[DLL])
        if args.native_batch_diagnostics:
            from native_batch.build import patch
            replacements[DLL], runner = patch(replacements[DLL], local_pve_round_compat=args.local_pve_round_compat)
            replacements['assets/bin/Data/Managed/NativeBatch.dll'] = runner
        if args.local_voice_origin:
            with tempfile.TemporaryDirectory(prefix='hyjs-client-voice-') as temporary:
                temporary = Path(temporary)
                input_dll = temporary/'input.dll'
                input_dll.write_bytes(replacements[DLL])
                voice_output = temporary/'output'
                subprocess.run(['rtk', 'proxy', sys.executable, 'tools/p10_voice/build.py',
                                '--input', str(input_dll), '--output-directory', str(voice_output),
                                '--origin', args.local_voice_origin, '--enable-local-voice'], check=True)
                replacements[DLL] = (voice_output/'Assembly-CSharp.dll').read_bytes()
                replacements['assets/bin/Data/Managed/LocalVoicePCM.dll'] = (voice_output/'LocalVoicePCM.dll').read_bytes()
                proof = voice_output/'manifest.json'
                digest = hashlib.sha256(proof.read_bytes()).hexdigest()
                proof_target = Path('analysis/p10')/('apk-voice-build-proof-'+digest+'.json')
                proof_target.parent.mkdir(parents=True, exist_ok=True)
                if proof_target.exists():
                    assert proof_target.read_bytes() == proof.read_bytes()
                else:
                    shutil.copy2(proof, proof_target)
        if args.local_worldboss_origin:
            with tempfile.TemporaryDirectory(prefix='hyjs-client-worldboss-') as temporary:
                temporary = Path(temporary)
                input_dll = temporary/'input.dll'
                input_dll.write_bytes(replacements[DLL])
                boss_output = temporary/'output'
                subprocess.run(['rtk', 'proxy', sys.executable, 'tools/p12_worldboss/build.py',
                                '--input', str(input_dll), '--output-directory', str(boss_output),
                                '--origin', args.local_worldboss_origin,
                                '--enable-local-world-boss-settlement'], check=True)
                replacements[DLL] = (boss_output/'Assembly-CSharp.dll').read_bytes()
                replacements['assets/bin/Data/Managed/LocalWorldBossSettlement.dll'] = (boss_output/'LocalWorldBossSettlement.dll').read_bytes()
                proof = boss_output/'manifest.json'
                proof_target = Path('analysis/p12')/('apk-worldboss-build-proof-'+hashlib.sha256(proof.read_bytes()).hexdigest()+'.json')
                proof_target.parent.mkdir(parents=True, exist_ok=True)
                if proof_target.exists():
                    assert proof_target.read_bytes() == proof.read_bytes()
                else:
                    shutil.copy2(proof, proof_target)
        if args.repair_guild_result_visibility:
            with tempfile.TemporaryDirectory(prefix='hyjs-client-guild-ui-') as temporary:
                temporary = Path(temporary)
                input_dll = temporary/'input.dll'
                input_dll.write_bytes(replacements[DLL])
                ui_output = temporary/'output'
                subprocess.run(['rtk', 'proxy', sys.executable, 'tools/p13_result_ui/build.py',
                                '--input', str(input_dll), '--output-directory', str(ui_output)], check=True)
                replacements[DLL] = (ui_output/'Assembly-CSharp.dll').read_bytes()
                proof = ui_output/'manifest.json'
                proof_target = Path('analysis/p13')/('apk-result-ui-build-proof-'+hashlib.sha256(proof.read_bytes()).hexdigest()+'.json')
                proof_target.parent.mkdir(parents=True, exist_ok=True)
                if proof_target.exists():
                    assert proof_target.read_bytes() == proof.read_bytes()
                else:
                    shutil.copy2(proof, proof_target)
        if args.repair_promotion_rank_self_zone:
            with tempfile.TemporaryDirectory(prefix='hyjs-client-promotion-rank-ui-') as temporary:
                temporary = Path(temporary)
                input_dll = temporary/'input.dll'
                input_dll.write_bytes(replacements[DLL])
                ui_output = temporary/'output'
                subprocess.run(['rtk', 'proxy', sys.executable, 'tools/p14_rank_ui/build.py',
                                '--input', str(input_dll), '--output-directory', str(ui_output)], check=True)
                replacements[DLL] = (ui_output/'Assembly-CSharp.dll').read_bytes()
                proof = ui_output/'manifest.json'
                proof_target = Path('analysis/p14')/('apk-rank-self-zone-build-proof-'+hashlib.sha256(proof.read_bytes()).hexdigest()+'.json')
                proof_target.parent.mkdir(parents=True, exist_ok=True)
                if proof_target.exists():
                    assert proof_target.read_bytes() == proof.read_bytes()
                else:
                    shutil.copy2(proof, proof_target)
        if args.repair_finals_empty_slots_and_self_zone:
            with tempfile.TemporaryDirectory(prefix='hyjs-client-finals-ui-') as temporary:
                temporary = Path(temporary)
                input_dll = temporary/'input.dll'
                input_dll.write_bytes(replacements[DLL])
                ui_output = temporary/'output'
                subprocess.run(['rtk', 'proxy', sys.executable, 'tools/p14_finals_ui/build.py',
                                '--input', str(input_dll), '--input-sha256', hashlib.sha256(replacements[DLL]).hexdigest(),
                                '--output-directory', str(ui_output)], check=True)
                replacements[DLL] = (ui_output/'Assembly-CSharp.dll').read_bytes()
                proof = ui_output/'manifest.json'
                proof_target = Path('analysis/p14')/('apk-finals-ui-build-proof-'+hashlib.sha256(proof.read_bytes()).hexdigest()+'.json')
                proof_target.parent.mkdir(parents=True, exist_ok=True)
                if proof_target.exists():
                    assert proof_target.read_bytes() == proof.read_bytes()
                else:
                    shutil.copy2(proof, proof_target)
        if args.repair_kf_replay_joint_viewer:
            with tempfile.TemporaryDirectory(prefix='hyjs-client-kf-viewer-') as temporary:
                temporary = Path(temporary)
                input_dll = temporary/'input.dll'
                input_dll.write_bytes(replacements[DLL])
                ui_output = temporary/'output'
                subprocess.run(['rtk', 'proxy', sys.executable, 'tools/p6_kf_viewer/build.py',
                                '--input', str(input_dll), '--input-sha256', hashlib.sha256(replacements[DLL]).hexdigest(),
                                '--output-directory', str(ui_output)], check=True)
                replacements[DLL] = (ui_output/'Assembly-CSharp.dll').read_bytes()
                proof = ui_output/'manifest.json'
                proof_target = Path('analysis/p6')/('apk-kf-viewer-build-proof-'+hashlib.sha256(proof.read_bytes()).hexdigest()+'.json')
                proof_target.parent.mkdir(parents=True, exist_ok=True)
                if proof_target.exists():
                    assert proof_target.read_bytes() == proof.read_bytes()
                else:
                    shutil.copy2(proof, proof_target)
        # Apply last so all existing Mono/voice/Boss/UI stages stay intact and
        # the one-byte proof binds the exact final assembly before packaging.
        if args.repair_planar_movement:
            replacements[DLL] = apply_planar_movement(replacements[DLL], out)
    # Preserve compressed asset bytes; rewrite the central directory without obsolete signatures
    # or replaced records, then append exactly one active record for each replacement.
    shutil.copyfile(source, out/'hyjs-local-unsigned.apk')
    with zipfile.ZipFile(out/'hyjs-local-unsigned.apk', 'a') as result:
        result.filelist = [i for i in result.filelist if not i.filename.startswith('META-INF/')
                           and i.filename not in replacements]
        result.NameToInfo = {i.filename:i for i in result.filelist}
        for name, content in replacements.items():
            result.writestr(name, content, compress_type=zipfile.ZIP_DEFLATED, compresslevel=1)
    print('Created build/hyjs-local-unsigned.apk')


if __name__ == '__main__':
    main()
