"""Deduct the original StrengthAll aggregate cost once, preserving updates and finally.

This changes only the UI reply consumer. Server allocation policy, original
coroutine timing, equipment updates and all battle-engine methods remain intact.
"""
import struct
from dncil.cil.body.reader import read_method_body_from_bytes


def patch_pet_strength_all(pe, raw, data, methods=None):
    owner, name = "<WaitSendMsg>c__Iterator121", "<>m__1A4"
    if methods is None:
        methods = {(str(t.TypeName), str(m.row.Name)): m.row
                   for t in pe.net.mdtables.TypeDef for m in t.MethodList}
    row = methods[owner, name]
    body = read_method_body_from_bytes(pe.get_data(row.Rva))
    start = pe.get_offset_from_rva(row.Rva)
    assert body.header_size == 12 and body.code_size == 137 and body.max_stack == 20
    assert body.local_var_sig_tok.value == 0x11000F58
    by_offset = {instruction.offset: instruction for instruction in body.instructions}
    assert [by_offset[offset].opcode.name for offset in (0x53, 0x58, 0x59, 0x5e)] == ["ldsfld", "ldloc.0", "callvirt", "callvirt"]
    assert str(pe.net.mdtables.Field.rows[(by_offset[0x53].operand.value & 0xffffff)-1].Name) == "playerData"
    assert str(pe.net.mdtables.MethodDef.rows[(by_offset[0x59].operand.value & 0xffffff)-1].Name) == "get_cost"
    assert str(pe.net.mdtables.MethodDef.rows[(by_offset[0x5e].operand.value & 0xffffff)-1].Name) == "Delete"
    assert [(by_offset[offset].opcode.name, by_offset[offset].operand) for offset in (0x0d, 0x2a, 0x7e, 0x83)] == [("brfalse", 0x94), ("br", 0x77), ("brtrue", 0x2f), ("leave", 0x94)]
    assert len(body.exception_handlers) == 1
    handler = body.exception_handlers[0]
    assert (handler.exception_type, handler.try_start, handler.try_end,
            handler.handler_start, handler.handler_end) == (2, 30, 124, 124, 136)
    # Rotate the intact 16-byte cost expression before GetEnumerator. Equipment
    # UpdateEquip/Refresh and each original popup keep their relative order.
    section = (start + body.header_size + body.code_size + 3) & ~3
    assert raw[section:section+4] == bytes.fromhex("411c0000")
    assert struct.unpack_from("<IIIIII", raw, section+4) == (2, 30, 94, 124, 12, 0)
    assert data[start:start+12+body.code_size] == raw[start:start+12+body.code_size]
    assert data[section:section+28] == raw[section:section+28]
    data[start+0x1e:start+0x63] = raw[start+0x53:start+0x63] + raw[start+0x1e:start+0x53]
    # The initializer/loop entry moves 16 bytes. The back edge must point to the
    # relocated get_Current, while the leave target and finally stay unchanged.
    struct.pack_into("<i", data, start+0x3a+1, 0x77-(0x3a+5))
    struct.pack_into("<i", data, start+0x7e+1, 0x3f-(0x7e+5))
    # CLI EH offsets exclude the 12-byte method header. Move only the try start;
    # its end, original Dispose handler, locals and stack bound remain identical.
    struct.pack_into("<I", data, section+8, 46)
    struct.pack_into("<I", data, section+12, 78)
    assert len(data) == len(raw)
    return {"owner": owner, "method": name, "codeBytes": 137,
            "costCallsPerSuccess": 1, "originalFinallyPreserved": True}
