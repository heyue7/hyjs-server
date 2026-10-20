"""Read-only IL scanner for the current bag/assembly. Not part of the overlay."""
import sys

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

PE = dnfile.dnPE(sys.argv[1])
types = PE.net.mdtables.TypeDef.rows
methods = {}
for owner in types:
    full = str(owner.TypeNamespace) + '.' + str(owner.TypeName)
    for m in owner.MethodList:
        methods[m.row_index] = full + '::' + str(m.row.Name)
fields = {}
for owner in types:
    full = str(owner.TypeNamespace) + '.' + str(owner.TypeName)
    for f in owner.FieldList:
        fields[f.row_index] = (full, str(f.row.Name))

want = sys.argv[2] if len(sys.argv) > 2 else None
if want == 'types':
    for owner in types:
        if str(owner.TypeName) in sys.argv[3:]:
            full = str(owner.TypeNamespace) + '.' + str(owner.TypeName)
            print('TYPE', full)
            for f in owner.FieldList:
                print('  FIELD', str(f.row.Name), f.row.struct.Signature.value.hex())
            for m in owner.MethodList:
                print('  METHOD', str(m.row.Name), 'rva', m.row.Rva)
    sys.exit()

needles = sys.argv[3:]
for owner in types:
    full = str(owner.TypeNamespace) + '.' + str(owner.TypeName)
    for m in owner.MethodList:
        method = m.row
        if not method.Rva:
            continue
        try:
            body = read_method_body_from_bytes(PE.get_data(method.Rva, 1048576))
        except Exception:
            continue
        for ins in body.instructions:
            tok = getattr(ins.operand, 'value', None)
            text = None
            if isinstance(tok, int):
                table, index = tok >> 24, tok & 0xffffff
                if table == 0x70:
                    try:
                        text = PE.net.user_strings.get(index).value
                    except Exception:
                        pass
                elif table == 4 and index in fields:
                    text = fields[index][0] + '::' + fields[index][1]
                elif table == 6 and index in methods:
                    text = methods[index]
                elif table == 10:
                    try:
                        row = PE.net.mdtables.MemberRef.rows[index - 1]
                        parent = row.Class.row
                        text = str(getattr(parent, 'TypeName', '')) + '::' + str(row.Name)
                    except Exception:
                        pass
            if text and any(n in text for n in needles):
                print(f'{methods[m.row_index]} | {ins.offset:04x} {ins.opcode.name} {text!r}')
    for f in owner.FieldList:
        if any(n in str(f.row.Name) for n in needles):
            print('FIELD', full + '::' + str(f.row.Name))
