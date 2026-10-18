"""Strict exact-value comparison to the two immutable original phone Daily logs.

Only explicitly enumerated new diagnostic fields are separated from
the historical schema. All nested state/export fields and list orders must
match, without epsilon, sorting, rounding, RNG rewriting or golden changes.
"""
import argparse
import collections
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
GOLDEN = ROOT / 'analysis/p6/native-batch-golden-v4'
KINDS = {'input', 'state', 'tick', 'round_begin', 'round_end', 'result'}
EXTRA_INPUT = {'schemaRevision', 'scope', 'manualControlSource', 'typedCallbackSHA256',
               'typedTargetSHA256', 'typedDriverPolicySHA256', 'typedDriverSHA256',
               'attackerFactor', 'defenderFactor'}


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def first_difference(left, right, path='$'):
    if isinstance(left, dict) and isinstance(right, dict):
        if left.keys() != right.keys():
            return {'path': path, 'kind': 'field-set', 'phone': sorted(left), 'host': sorted(right)}
        for key in left:
            delta = first_difference(left[key], right[key], path + '.' + key)
            if delta:
                return delta
    elif isinstance(left, list) and isinstance(right, list):
        if len(left) != len(right):
            return {'path': path, 'kind': 'count', 'phone': len(left), 'host': len(right)}
        for index, (a, b) in enumerate(zip(left, right)):
            delta = first_difference(a, b, path + '[' + str(index) + ']')
            if delta:
                return delta
    elif left != right or (isinstance(left, bool) != isinstance(right, bool)):
        return {'path': path, 'kind': 'value', 'phone': left, 'host': right}
    return None


def compare(folder):
    cases = []
    for name in ['000', '001']:
        old = GOLDEN / (name + '.native.jsonl')
        new = folder / (name + '.native.jsonl')
        expected = [json.loads(line) for line in old.read_text().splitlines()]
        observed = [json.loads(line) for line in new.read_text().splitlines()]
        projected = []
        additions = collections.Counter()
        for row in observed:
            if row['kind'] not in KINDS:
                additions[row['kind']] += 1
                continue
            row = dict(row)
            extra = EXTRA_INPUT if row['kind'] == 'input' else ({'round', 'time'} if row['kind'] in {'tick', 'round_begin', 'round_end'} else set())
            for key in extra:
                row.pop(key, None)
            if row['kind'] == 'state':
                # This field was added to the phone runner after v4. Keep its
                # exact path explicit; old phone logs cannot certify it.
                row['state'] = dict(row['state'])
                for camp in ['attacker', 'defender']:
                    row['state'][camp] = dict(row['state'][camp])
                    if 'damageFactor' in row['state'][camp]:
                        row['state'][camp].pop('damageFactor')
                        additions['state.' + camp + '.damageFactor'] += 1
            projected.append(row)
        delta = first_difference(expected, projected)
        cases.append({'case': name, 'phoneSHA256': digest(old), 'hostSHA256': digest(new),
                      'comparedRows': len(expected), 'counts': dict(collections.Counter(row['kind'] for row in expected)),
                      'firstDifference': delta, 'exactHistoricalSchemaMatch': delta is None,
                      'additionalCurrentCallerObservationsWithoutOldPhoneCounterpart': dict(additions)})
    return {'schema': 'local-original-engine-host-exact-daily-comparison-v1',
            'status': 'PASS' if all(row['exactHistoricalSchemaMatch'] for row in cases) else 'FAIL',
            'numericTolerance': 0, 'listOrderPreserved': True, 'full13FamilyGoMatrixExecuted': False,
            'nativeCompared': False, 'authoritative': False, 'cases': cases}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--cases', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists():
        raise FileExistsError('Preserve previous comparison evidence')
    report = compare(args.cases)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False))
    raise SystemExit(0 if report['status'] == 'PASS' else 1)
