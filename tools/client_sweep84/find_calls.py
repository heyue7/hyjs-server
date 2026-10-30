"""Enumerate original method callers of the two sweep coroutine factories."""
import sys
import zipfile
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body

with zipfile.ZipFile(sys.argv[1]) as archive:
    pe = dnfile.dnPE(data=archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll'))
for name, rid, method in methods(pe):
    if not method.Rva:
        continue
    for ins in read_method_body_from_bytes(exact_body(pe, method)).instructions:
        token = getattr(ins.operand, 'value', None)
        if ins.opcode.name in ('call', 'callvirt') and token in (0x06005132, 0x06005ba1):
            print(f'{name} token={0x06000000 | rid:#010x} IL_{ins.offset:04x} -> {token:#010x}')
