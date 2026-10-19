"""Extract selected original IL as bounded, readable development evidence."""
import argparse
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--type', action='append', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--assembly',type=Path,default=ROOT / 'analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll')
    args = parser.parse_args()
    output = args.output.resolve()
    if not output.is_relative_to(ROOT / 'analysis'):
        raise RuntimeError('Keep original evidence inside the analysis tree')
    pe = dnfile.dnPE(str(args.assembly))
    methods, fields = {}, {}
    for owner in pe.net.mdtables.TypeDef.rows:
        name = str(owner.TypeNamespace) + '.' + str(owner.TypeName)
        methods.update({m.row_index: name + '::' + str(m.row.Name) for m in owner.MethodList})
        fields.update({f.row_index: name + '::' + str(f.row.Name) for f in owner.FieldList})

    def resolve(operand):
        token = getattr(operand, 'value', None)
        if token is None:
            return str(operand)
        table, index = token >> 24, token & 0xffffff
        if table == 0x70:
            return repr(pe.net.user_strings.get(index).value)
        if table == 6:
            return methods[index]
        if table == 4:
            return fields[index]
        if table == 10:
            row = pe.net.mdtables.MemberRef.rows[index - 1]
            parent = row.Class.row
            return str(getattr(parent, 'TypeName', '')) + '::' + str(row.Name)
        return str(operand)

    lines, found = [], set()
    for owner in pe.net.mdtables.TypeDef.rows:
        full = str(owner.TypeNamespace) + '.' + str(owner.TypeName)
        if full not in args.type and str(owner.TypeName) not in args.type:
            continue
        found.add(full)
        for member in owner.MethodList:
            method = member.row
            if not method.Rva:
                continue
            lines.append('\nMETHOD ' + methods[member.row_index])
            for instruction in read_method_body_from_bytes(pe.get_data(method.Rva, 1048576)).instructions:
                operand = instruction.operand
                opcode = instruction.opcode.name
                # Branch targets are absolute IL offsets. Match the hexadecimal
                # instruction column so a decimal307 cannot be mistaken for a
                # different block than0133 when auditing original control flow.
                if isinstance(operand, int) and opcode.startswith(('br', 'beq', 'bne', 'bge', 'bgt', 'ble', 'blt', 'leave')):
                    rendered = f'0x{operand:04x}'
                elif opcode == 'switch':
                    rendered = '[' + ', '.join(f'0x{target:04x}' for target in operand) + ']'
                else:
                    rendered = resolve(operand)
                lines.append(f'{instruction.offset:04x} {opcode} {rendered}')
    if not found:
        raise RuntimeError('Original selected types were not found')
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text('\n'.join(lines) + '\n')
    print({'types': sorted(found), 'lines': len(lines), 'output': str(output.relative_to(ROOT))})


if __name__ == '__main__':
    main()
