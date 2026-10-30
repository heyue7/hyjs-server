"""Inspect selected original IL from a managed DLL or an APK, read-only."""
import argparse
import hashlib
import sys
import zipfile
from pathlib import Path

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods, operand
from tools.local_asset_ready.remove_gate import exact_body

TARGETS = {
    '<SweepNext>c__IteratorF4::MoveNext',
    '<_StartSweep>c__Iterator141::MoveNext',
    'UI.PVPSweepPage::CreateItem', 'UI.UICopySweep::Refresh',
    'UI.PVPSweepPage::SweepNext', 'UI.PVPSweepPage::AskFight',
    'UI.PVPSweepPage::DoOpen', 'UI.PVPSweepPage::DoClose',
    'UI.UICopySweep::OnSweepCopyAuto',
}


def ref(pe, value):
    if not hasattr(value, 'value'):
        return repr(value)
    n = value.value
    tables = {0x01:'TypeRef', 0x02:'TypeDef', 0x04:'Field', 0x06:'MethodDef', 0x0a:'MemberRef', 0x11:'StandAloneSig', 0x1b:'TypeSpec', 0x2b:'MethodSpec'}
    name = tables.get(n >> 24)
    if not name:
        return hex(n)
    table = getattr(pe.net.mdtables, name)
    row = table.rows[(n & 0xffffff)-1]
    label = str(getattr(row, 'Name', getattr(row, 'TypeName', '?')))
    owner = getattr(row, 'Class', None)
    if owner and owner.row:
        ownerrow = owner.row
        label = str(getattr(ownerrow, 'TypeNamespace', '')) + '.' + str(getattr(ownerrow, 'TypeName', getattr(ownerrow, 'Name', '?'))) + '::' + label
    if name in ('Field', 'MethodDef'):
        for typ in pe.net.mdtables.TypeDef.rows:
            entries = typ.FieldList if name == 'Field' else typ.MethodList
            if any(item.row is row for item in entries):
                label = '.'.join(filter(None, [str(typ.TypeNamespace), str(typ.TypeName)])) + '::' + label
                break
    return f'{hex(n)} {label}'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('input', type=Path)
    parser.add_argument('--contains', action='append', default=[])
    args = parser.parse_args()
    if args.input.suffix.lower() == '.apk':
        with zipfile.ZipFile(args.input) as z:
            raw = z.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
    else:
        raw = args.input.read_bytes()
    pe = dnfile.dnPE(data=raw)
    print('SHA256', hashlib.sha256(raw).hexdigest())
    for name, rid, row in methods(pe):
        if not (name in TARGETS or any(x in name for x in args.contains)) or not row.Rva:
            continue
        b = exact_body(pe, row)
        body = read_method_body_from_bytes(b)
        print('\n', name, hex(0x06000000 | rid), 'rawSHA', hashlib.sha256(b).hexdigest(), 'EH', len(body.exception_handlers))
        for i in body.instructions:
            x = i.operand
            if i.opcode.name == 'switch':
                arg = ', '.join(hex(j) for j in x)
            elif i.opcode.flow_control.name in ('Branch', 'Cond_Branch'):
                arg = hex(x)
            elif hasattr(x, 'value'):
                arg = ref(pe, x)
            else:
                arg = repr(operand(x)) if x is not None else ''
            print(f'{i.offset:04x}: {i.opcode.name:14} {arg}')


if __name__ == '__main__':
    main()
