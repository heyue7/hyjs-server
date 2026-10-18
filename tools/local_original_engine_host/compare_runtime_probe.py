"""Keep complete-schema FAIL separate from exact non-platform battle payload.

The sole separately classified field is original ai_notify.data.stack. It is
never removed from raw output or from the complete-schema comparison.
"""
from pathlib import Path
from itertools import zip_longest
import argparse
import collections
import copy
import json
from case_sets import ROOT, pinned, sha
from compare import first_difference


def compare(manifest):
    run = json.loads(manifest.read_bytes())
    if run['schema'] != 'local-original-engine-runtime-probe-v1' or run['status'] != 'EXECUTED_PENDING_STRICT_COMPARE':
        raise ValueError('Only completed bounded runtime probe accepted')
    cases = []
    for row in run['caseSets'][0]['cases']:
        phone = pinned(row['referencePhoneTrace'], row['referencePhoneTraceSHA256'])
        host = (manifest.parent / 'runtime' / row['localWire']).with_suffix('.native.jsonl')
        full_delta = payload_delta = None
        counts = collections.Counter()
        platform_differences = []
        phone_rows = host_rows = 0
        with phone.open() as old, host.open() as new:
            for index, (old_line, new_line) in enumerate(zip_longest(old, new)):
                a = json.loads(old_line) if old_line is not None else None
                b = json.loads(new_line) if new_line is not None else None
                phone_rows += a is not None
                host_rows += b is not None
                if a is not None:
                    counts[a['kind']] += 1
                path = '$[' + str(index) + ']'
                if full_delta is None:
                    full_delta = first_difference(a, b, path)
                if a is not None and b is not None and a.get('kind') == b.get('kind') == 'ai_notify':
                    left, right = a['data'].get('stack'), b['data'].get('stack')
                    if left != right:
                        platform_differences.append({'event': index, 'round': a['round'], 'time': a['time'],
                                                     'phoneStack': left, 'hostStack': right})
                    # Diagnostic classification only. Raw rows and the strict
                    # full-schema comparison above keep this exact field.
                    a = copy.deepcopy(a)
                    b = copy.deepcopy(b)
                    a['data'].pop('stack', None)
                    b['data'].pop('stack', None)
                if payload_delta is None:
                    payload_delta = first_difference(a, b, path)
        cases.append({'case': row['case'], 'phoneTraceSHA256': sha(phone), 'hostTraceSHA256': sha(host),
                      'phoneRows': phone_rows, 'hostRows': host_rows, 'eventCounts': dict(counts),
                      'fullSchemaExactMatch': full_delta is None, 'fullSchemaFirstDifference': full_delta,
                      'payloadExactExceptExplicitPlatformStack': payload_delta is None,
                      'payloadFirstDifference': payload_delta,
                      'explicitlyClassifiedPlatformField': 'ai_notify.data.stack',
                      'platformIdentityDifferences': platform_differences})
    return {'schema': 'local-original-engine-runtime-separated-comparison-v1',
            'fullSchemaStatus': 'PASS' if all(r['fullSchemaExactMatch'] for r in cases) else 'FAIL',
            'battlePayloadStatusExceptExplicitPlatformStack': 'PASS' if all(r['payloadExactExceptExplicitPlatformStack'] for r in cases) else 'FAIL',
            'numericTolerance': 0, 'listOrderPreserved': True, 'rawCapturesUnchanged': True,
            'platformIdentityIsNotBattleAuthority': True, 'nativeCompared': False, 'authoritative': False,
            'full13FamilyGoMatrixExecuted': False, 'runtimeManifestSHA256': sha(manifest), 'cases': cases}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists():
        raise FileExistsError('Preserve previous comparison')
    report = compare(args.manifest.resolve())
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({k: report[k] for k in ['fullSchemaStatus', 'battlePayloadStatusExceptExplicitPlatformStack']}, ensure_ascii=False))
    raise SystemExit(0 if report['battlePayloadStatusExceptExplicitPlatformStack'] == 'PASS' else 1)
