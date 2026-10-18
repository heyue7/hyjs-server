"""Schema4 complete host-to-phone comparison: no projections or tolerances."""
import argparse
import collections
import hashlib
import json
from itertools import zip_longest
from pathlib import Path
from case_sets import ROOT, SETS, pinned, sha
from compare import first_difference


def compare(run_manifest):
    run = json.loads(run_manifest.read_bytes())
    if (run['executionOrigin'] != 'local-coreclr-original-engine-not-phone-capture'
            or run['status'] not in {'EXECUTED_PENDING_STRICT_COMPARE', 'BLOCKED'}
            or run.get('nativeCompared') or run.get('authoritative')):
        raise ValueError('Not a completed independent local host run')
    rows = []
    for group in run['caseSets']:
        config = SETS[group['set']]
        pinned(config['source'], config['sourceSHA256'])
        pinned(config['phone'], config['phoneSHA256'])
        if len(group['cases']) != config['count']:
            raise ValueError('Case count differs from fixed source')
        for case in group['cases']:
            phone = pinned(case['referencePhoneTrace'], case['referencePhoneTraceSHA256'])
            host = (run_manifest.parent / 'runtime' / case['localWire']).with_suffix('.native.jsonl')
            delta = None
            phone_rows = host_rows = 0
            counts = collections.Counter()
            missing_host = not host.is_file()
            if missing_host:
                delta = {'path': '$', 'kind': 'host-not-executed', 'phone': str(phone), 'host': str(host)}
            import contextlib
            with phone.open() as original, (host.open() if not missing_host else contextlib.nullcontext([])) as local:
                for index, (old_line, new_line) in enumerate(zip_longest(original, local)):
                    expected = json.loads(old_line) if old_line is not None else None
                    try:
                        actual = json.loads(new_line) if new_line is not None else None
                    except json.JSONDecodeError:
                        actual = None
                        if delta is None:
                            delta = {'path': '$[' + str(index) + ']', 'kind': 'truncated-host-JSON'}
                    phone_rows += expected is not None
                    host_rows += actual is not None
                    if expected is not None:
                        counts[expected['kind']] += 1
                    if index == 0 and (expected.get('schemaRevision') != 4 or (actual is not None and actual.get('schemaRevision') != 4)):
                        raise ValueError('Only original schema4 full comparison accepted')
                    if delta is None:
                        delta = first_difference(expected, actual, '$[' + str(index) + ']')
            rows.append({'set': group['set'], 'case': case['case'], 'battleType': case['battleType'],
                         'phoneTrace': str(phone.relative_to(ROOT)), 'phoneSHA256': sha(phone),
                         'hostTrace': str(host.relative_to(ROOT)), 'hostSHA256': sha(host) if not missing_host else None,
                         'rawBytesIdentical': not missing_host and sha(phone) == sha(host),
                         'phoneRows': phone_rows, 'hostRows': host_rows,
                         'counts': dict(counts),
                         'firstDifference': delta, 'fullSchemaExactMatch': delta is None})
    if len(rows) != (6 if run['caseSet'] == 'p7' else SETS[run['caseSet']]['count']):
        raise ValueError('Completed case denominator changed')
    return {'schema': 'local-original-engine-host-p7-schema4-exact-v1',
            'status': 'PASS' if all(row['fullSchemaExactMatch'] for row in rows) else 'FAIL',
            'executionOrigin': 'local-coreclr-original-engine-not-phone-capture',
            'runManifestSHA256': sha(run_manifest), 'numericTolerance': 0,
            'fieldsDropped': [], 'arraysSorted': False,
            'full13FamilyGoMatrixExecuted': False, 'nativeCompared': False, 'authoritative': False,
            'cases': rows}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run-manifest', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists():
        raise FileExistsError('Preserve prior comparison')
    result = compare(args.run_manifest.resolve())
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False))
    raise SystemExit(0 if result['status'] == 'PASS' else 1)
