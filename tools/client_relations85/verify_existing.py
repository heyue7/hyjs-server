"""Reverify an existing output against an explicitly pinned DLL or APK entry."""
import argparse
import hashlib
import json
import re
import sys
import zipfile
from pathlib import Path
import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.dont_write_bytecode = True
sys.path.insert(0, str(ROOT))
from tools.client_relations85.build import verify, file_sha

ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'


def verify_metadata(before, after):
    old, new = (dnfile.dnPE(data=raw) for raw in (before, after))
    heaps = {}
    for name in (b'#Strings', b'#US', b'#GUID', b'#Blob'):
        x, y = (pe.net.metadata.streams[name].__data__ for pe in (old, new))
        if not y.startswith(x):
            raise ValueError('original metadata heap bytes changed: ' + name.decode())
        heaps[name.decode()] = {'oldBytes': len(x), 'newBytes': len(y), 'originalPrefixExact': True}
    rows = {}
    for table in ('StandAloneSig', 'TypeSpec', 'MethodSpec'):
        a, b = (getattr(pe.net.mdtables, table).rows for pe in (old, new))
        if len(b) < len(a):
            raise ValueError('metadata rows removed: ' + table)
        for x, y in zip(a, b):
            if table == 'MethodSpec':
                first = (x.Method.table.name, x.Method.row_index, x.Instantiation.value)
                second = (y.Method.table.name, y.Method.row_index, y.Instantiation.value)
            else:
                first, second = x.Signature.value, y.Signature.value
            if first != second:
                raise ValueError('metadata signature changed: ' + table)
        rows[table] = {'oldRows': len(a), 'newRows': len(b), 'originalRowsExact': True}
    return {'metadataHeaps': heaps, 'signatureMetadataRows': rows}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument('--input-managed', type=Path)
    group.add_argument('--input-apk', type=Path)
    parser.add_argument('--input-managed-sha256', required=True)
    parser.add_argument('--output-managed', required=True, type=Path)
    parser.add_argument('--output-managed-sha256', required=True)
    parser.add_argument('--report', required=True, type=Path)
    args = parser.parse_args()
    report, output = args.report.resolve(), args.output_managed.resolve()
    if not report.is_relative_to(ROOT/'analysis/fragment-box85/relations') or report.exists():
        raise ValueError('fresh task-owned report required')
    for value in (args.input_managed_sha256, args.output_managed_sha256):
        if not re.fullmatch('[0-9a-fA-F]{64}', value):
            raise ValueError('full SHA-256 required')
    if args.input_apk:
        with zipfile.ZipFile(args.input_apk) as archive:
            if archive.namelist().count(ENTRY) != 1:
                raise ValueError('ambiguous main entry')
            before = archive.read(ENTRY)
    else:
        before = args.input_managed.read_bytes()
    after = output.read_bytes()
    if hashlib.sha256(before).hexdigest() != args.input_managed_sha256.lower() or \
            hashlib.sha256(after).hexdigest() != args.output_managed_sha256.lower():
        raise ValueError('input or output main SHA differs')
    result = {'status': 'PASS', 'inputManagedSHA256': args.input_managed_sha256.lower(),
              'outputManagedSHA256': args.output_managed_sha256.lower(),
              'verificationSourceSHA256': file_sha(ROOT/'tools/client_relations85/build.py'),
              'additionalAuditSourceSHA256': file_sha(Path(__file__)),
              **verify(before, after), **verify_metadata(before, after)}
    report.write_text(json.dumps(result, indent=2, ensure_ascii=False)+'\n')
    print(json.dumps({k: result[k] for k in ('status', 'inputManagedSHA256', 'outputManagedSHA256',
                                             'unchangedBodyCount', 'originalEngineMethodsExact')}))


if __name__ == '__main__':
    main()
