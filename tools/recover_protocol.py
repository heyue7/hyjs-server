"""Recover protobuf-net contracts and PBConvert dispatch from the supplied client.

Static metadata/IL inspection only: no target assembly is loaded or executed.
"""
from pathlib import Path
import collections
import hashlib
import json
import struct
import sys

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[1]
DLL = ROOT / "analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll"
OUT = ROOT / "server/protocol"


class Reader:
    def __init__(self, data):
        self.data, self.pos = data, 0

    def read(self, n):
        result = self.data[self.pos:self.pos+n]
        if len(result) != n:
            raise ValueError("Truncated metadata")
        self.pos += n
        return result

    def byte(self):
        return self.read(1)[0]

    def compressed(self):
        b = self.byte()
        if b < 128:
            return b
        if b < 192:
            return ((b & 63) << 8) | self.byte()
        return ((b & 31) << 24) | int.from_bytes(self.read(3), "big")

    def string(self):
        if self.data[self.pos] == 255:
            self.pos += 1
            return None
        return self.read(self.compressed()).decode("utf-8")


def recover():
    pe = dnfile.dnPE(str(DLL))
    md = pe.net.mdtables

    def typename(coded):
        table = [md.TypeDef, md.TypeRef, md.TypeSpec][coded & 3]
        row = table.rows[(coded >> 2)-1]
        return str(row.TypeNamespace) + "." + str(row.TypeName)

    primitives = {2:"bool", 5:"byte", 8:"int32", 9:"uint32", 10:"int64", 11:"uint64", 12:"float", 13:"double", 14:"string"}

    def signature(r):
        code = r.byte()
        if code in primitives:
            return {"type":primitives[code]}
        if code in (0x11, 0x12):
            return {"type":typename(r.compressed()).split(".")[-1]}
        if code == 0x1d:
            t = signature(r)
            if t["type"] == "byte":
                return {"type":"bytes"}
            return {**t, "repeated":True}
        if code == 0x15:
            assert r.byte() in (0x11, 0x12)
            name = typename(r.compressed())
            assert name == "System.Collections.Generic.List`1", name
            assert r.compressed() == 1
            return {**signature(r), "repeated":True}
        raise ValueError(f"Unsupported signature element {code:#x}")

    def attr_name(ca):
        ctor = ca.Type.row
        if hasattr(ctor, "Class"):
            return str(getattr(ctor.Class.row, "TypeName", ""))
        return ""

    def proto_member(blob):
        r = Reader(blob)
        assert r.read(2) == b"\x01\x00"
        attrs = {"number":struct.unpack("<i", r.read(4))[0]}
        count = struct.unpack("<H", r.read(2))[0]
        for _ in range(count):
            assert r.byte() in (0x53, 0x54)
            typ = r.byte()
            if typ == 0x55:
                enum_type = r.string()
                assert enum_type.startswith("ProtoBuf.DataFormat,"), enum_type
            key = r.string()
            if typ == 2:
                value = bool(r.byte())
            elif typ == 14:
                value = r.string()
            elif typ in (8, 0x55):
                value = struct.unpack("<i", r.read(4))[0]
            else:
                raise ValueError(f"Unsupported attribute type: {typ}")
            attrs[key] = value
        assert r.pos == len(blob)
        return attrs

    attrs = collections.defaultdict(list)
    contracts = set()
    for ca in md.CustomAttribute.rows:
        name = attr_name(ca)
        if name == "ProtoContractAttribute" and ca.Parent.table.name == "TypeDef":
            contracts.add(ca.Parent.row_index)
        if name == "ProtoMemberAttribute":
            assert ca.Parent.table.name == "Property"
            attrs[ca.Parent.row_index].append(proto_member(ca.Value.value))

    props = {pm.Parent.row_index:pm.PropertyList for pm in md.PropertyMap.rows}
    messages = {}
    for index, t in enumerate(md.TypeDef.rows, 1):
        if index not in contracts or str(t.TypeNamespace) != "ProtoBuf.Message":
            continue
        fields = []
        for prop in props.get(index, []):
            if prop.row_index not in attrs:
                continue
            assert len(attrs[prop.row_index]) == 1
            a = attrs[prop.row_index][0]
            r = Reader(prop.row.Type.value)
            assert r.byte() == 8 and r.compressed() == 0
            spec = signature(r)
            assert r.pos == len(r.data)
            fmt = a.get("DataFormat", 0)
            typ = spec["type"]
            if fmt == 1 and typ in ("int32", "int64"):
                spec["type"] = "s" + typ
            elif fmt == 3 and typ in ("int32", "int64", "uint32", "uint64"):
                spec["type"] = ("sfixed" if typ.startswith("int") else "fixed") + typ[-2:]
            else:
                assert fmt in (0, 2) or (fmt == 3 and typ in ("float", "double")), (str(t.TypeName), str(prop.row.Name), typ, fmt)
            fields.append({"name":a.get("Name", str(prop.row.Name)), **spec,
                           "number":a["number"], "required":a.get("IsRequired", False),
                           "packed":a.get("IsPacked", False), "property_row":prop.row_index})
        fields.sort(key=lambda f:f["number"])
        assert len({f["number"] for f in fields}) == len(fields)
        messages[str(t.TypeName)] = {"type_row":index, "fields":fields}

    # Resolve concrete generic Deserialize<T> calls in each PBConvert switch arm.
    convert = next(t for t in md.TypeDef.rows if str(t.TypeName) == "PBConvert")
    method = next(m.row for m in convert.MethodList if str(m.row.Name) == "toObject" and len(m.row.Signature.value) > 6)
    body = read_method_body_from_bytes(pe.get_data(method.Rva))
    ins = body.instructions
    by_offset = {i.offset:k for k,i in enumerate(ins)}

    def case_type(offset):
        k = by_offset[offset]
        # Real arms are ldloc.1; call Deserialize<T>; stloc.0; br end.
        for i in ins[k:k+4]:
            if i.opcode.name == "call" and i.operand.value >> 24 == 0x2b:
                row = md.MethodSpec.rows[(i.operand.value & 0xffffff)-1]
                target = row.Method.row
                if str(target.Name) != "Deserialize":
                    return None
                r = Reader(row.Instantiation.value)
                assert r.byte() == 0x0a and r.compressed() == 1
                result = signature(r)["type"]
                assert result in messages, result
                return result
        return None

    opcodes = {}
    for k,i in enumerate(ins):
        if i.opcode.name == "switch":
            assert ins[k-1].opcode.name == "sub"
            assert ins[k-2].opcode.name == "ldc.i4"
            base = ins[k-2].operand
            for j,offset in enumerate(i.operand):
                typ = case_type(offset)
                if typ:
                    opcodes[base+j] = typ
        elif i.opcode.name in ("beq", "beq.s") and ins[k-1].opcode.name == "ldc.i4":
            typ = case_type(i.operand)
            if typ:
                opcodes[ins[k-1].operand] = typ
    assert opcodes[20005] == "CSCheckVersion"
    assert opcodes[20029] == "CSPlayerLogin"
    deserialize_arms = sum(
        i.opcode.name == "call" and i.operand.value >> 24 == 0x2b
        and str(md.MethodSpec.rows[(i.operand.value & 0xffffff)-1].Method.row.Name) == "Deserialize"
        for i in ins
    )
    assert len(opcodes) == deserialize_arms, "Incomplete opcode recovery"

    scalar = set(primitives.values()) | {"bytes", "sint32", "sint64", "fixed32", "fixed64", "sfixed32", "sfixed64"}
    for name,msg in messages.items():
        for f in msg["fields"]:
            assert f["type"] in scalar or f["type"] in messages, (name, f)
    result = {"assembly_sha256":hashlib.sha256(DLL.read_bytes()).hexdigest(),
              "messages":messages, "opcodes":dict(sorted(opcodes.items()))}
    OUT.mkdir(parents=True, exist_ok=True)
    (OUT/"schema.json").write_text(json.dumps(result, ensure_ascii=False, indent=2)+"\n")
    lines = ['// Recovered from client metadata. Original server behavior is not included.',
             'syntax = "proto2";', 'package hyjs;', '']
    for name,msg in messages.items():
        lines.append(f"message {name} {{")
        for f in msg["fields"]:
            label = "repeated" if f.get("repeated") else "required" if f["required"] else "optional"
            option = " [packed = true]" if f["packed"] else ""
            lines.append(f'  {label} {f["type"]} {f["name"]} = {f["number"]}{option};')
        lines.append("}\n")
    (OUT/"game.proto").write_text("\n".join(lines))
    print(json.dumps({"messages":len(messages), "fields":sum(len(v["fields"]) for v in messages.values()),
                      "opcode_mappings":len(opcodes), "output":str(OUT)}, ensure_ascii=False))


if __name__ == "__main__":
    recover()
