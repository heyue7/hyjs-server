"""Optional original APK combat logs, enabled solely by retargeting existing calls.

The original calls already format native round, skill, HP and actual AP details.
The two targets both have the exact static void(object) signature. This module
changes four operand bytes per existing call, leaving arithmetic, method bodies,
metadata, branches, exception regions and the disabled original stub intact.
"""
import struct

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

_SIGNATURE = bytes.fromhex("0001011c")
# APK10056262 has exactly these 17 existing logging sites. Reject drift instead
# of turning on unrelated UI/network logging or modifying an unknown assembly.
_CALLERS = {
    "Battle.Engine.ComputeCore::BattleEnd": 1,
    "Battle.Engine.ComputeCore::RoundBegin": 4,
    "Battle.Engine.LogicModel.FActor::AddHp": 1,
    "Battle.Engine.LogicModel.FActor::DoDead": 1,
    "Battle.Engine.LogicModel.FPlayer::AddAnger": 1,
    "Battle.Engine.LogicModel.FRelease::Do": 6,
    "Battle.Engine.LogicModel.FRelease::ProcMagicStone": 1,
    "Battle.Engine.LogicModel.FSkill::Do": 1,
    "Battle.Engine.util.RandomUtil::SetSeed": 1,
}


def _code(raw, offset):
    """Read only each method's code region before spending time on IL decoding."""
    if offset < 0 or offset >= len(raw):
        raise ValueError("Combat diagnostic method offset is outside the assembly")
    flags = raw[offset] & 3
    if flags == 2:
        header, length = 1, raw[offset] >> 2
    elif flags == 3 and offset + 12 <= len(raw):
        header = (struct.unpack_from("<H", raw, offset)[0] >> 12) * 4
        length = struct.unpack_from("<I", raw, offset + 4)[0]
        if header < 12:
            raise ValueError("Combat diagnostic fat method header is invalid")
    else:
        raise ValueError("Combat diagnostic method header is unsupported")
    if offset + header + length > len(raw):
        raise ValueError("Combat diagnostic method code is outside the assembly")
    return raw[offset + header:offset + header + length]


def patch(raw: bytes) -> bytes:
    """Enable 17 native combat logging calls; return unchanged bytes on repetition.

    Validation completes before any operand is changed. Incompatible signatures,
    unexpected callers/opcodes, removed sites and altered original logging stubs
    raise ValueError, so a diagnostic build cannot silently weaken its boundary.
    """
    pe = dnfile.dnPE(data=raw)
    if not pe.net or not pe.net.mdtables:
        raise ValueError("Combat diagnostic input has no managed metadata")
    owners = {}
    sources = []
    for owner in pe.net.mdtables.TypeDef.rows:
        name = str(owner.TypeNamespace) + "." + str(owner.TypeName)
        for method in owner.MethodList:
            full = name + "::" + str(method.row.Name)
            owners[method.row_index] = full
            if full == "Battle.Engine.Debug::LogFightMsg":
                sources.append((0x06000000 | method.row_index, method.row))
    if len(sources) != 1 or sources[0][1].Signature.value != _SIGNATURE:
        raise ValueError("Original combat logging target must be unique static void(object)")
    source, stub = sources[0]
    if not stub.Rva:
        raise ValueError("Original combat logging stub has no IL body")
    stub_body = read_method_body_from_bytes(pe.get_data(stub.Rva))
    if len(stub_body.instructions) != 1 or stub_body.instructions[0].opcode.name != "ret" or stub_body.exception_handlers:
        raise ValueError("Original combat logging stub must remain its original single ret")
    targets = []
    for index, member in enumerate(pe.net.mdtables.MemberRef.rows, 1):
        owner = member.Class.row
        if (str(member.Name) == "Log" and getattr(owner, "TypeNamespace", None) is not None
                and str(owner.TypeNamespace) == "UnityEngine" and str(owner.TypeName) == "Debug"
                and member.Signature.value == _SIGNATURE):
            targets.append(0x0A000000 | index)
    if len(targets) != 1:
        raise ValueError("UnityEngine.Debug.Log target must be unique static void(object)")
    target = targets[0]
    source_bytes, target_bytes = struct.pack("<I", source), struct.pack("<I", target)
    found = {name: 0 for name in _CALLERS}
    operands = []
    for index, method in enumerate(pe.net.mdtables.MethodDef.rows, 1):
        if not method.Rva:
            continue
        name = owners.get(index, "")
        start = pe.get_offset_from_rva(method.Rva)
        code = _code(raw, start)
        # Target bytes alone are a fast candidate filter; actual instruction
        # decoding below prevents changing byte patterns inside data or operands.
        if source_bytes not in code and (name not in _CALLERS or target_bytes not in code):
            continue
        body = read_method_body_from_bytes(pe.get_data(method.Rva))
        for instruction in body.instructions:
            token = getattr(instruction.operand, "value", None)
            if token != source and not (name in _CALLERS and token == target):
                continue
            if name not in _CALLERS or instruction.opcode.name != "call" or instruction.size != 5:
                raise ValueError("Unexpected original combat logging caller or non-static call: " + name)
            found[name] += 1
            if token == source:
                operands.append(start + instruction.offset + 1)
    if found != _CALLERS:
        raise ValueError("Original 17 combat logging sites changed or are incomplete")
    result = bytearray(raw)
    for offset in operands:
        if raw[offset:offset + 4] != source_bytes:
            raise ValueError("Combat diagnostic operand offset disagrees with original metadata")
        result[offset:offset + 4] = target_bytes
    return bytes(result)
