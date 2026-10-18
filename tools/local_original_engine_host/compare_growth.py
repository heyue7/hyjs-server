"""Exact comparisons for the two immutable highest-awake003 phone samples.

Full schema remains a separate comparison. The historical serializer expanded
skill object graphs; current caller emits a smaller fixed observer contract.
Every projection and unavailable observation is explicitly declared below.
No floating tolerance, actor/order rewrite, expected result or golden mutation.
"""
from pathlib import Path
import argparse
import collections
import copy
import gzip
import hashlib
import json
from itertools import zip_longest
from case_sets import ROOT, pinned, sha
from compare import first_difference

LEGACY_KINDS = {'input', 'state', 'round_begin', 'tick', 'round_end', 'skill_press', 'skill_begin', 'skill_cast', 'result'}
NEW_OBSERVATIONS = {'caller_progress', 'ai_notify', 'original_final_operates'}
BEGIN_FIELDS = {'type', 'id', 'index', 'IsUltimate', 'state', 'stepState', 'cooldownCur', 'target', 'castTargets'}


def opened(path):
    return gzip.open(path, 'rt') if path.suffix == '.gz' else path.open()


def projection(row, legacy, missing):
    result = copy.deepcopy(row)
    kind = result['kind']
    if kind == 'input':
        keys = {'kind', 'scope', 'fightId', 'seed', 'battleType'}
        if not legacy:
            for key in result.keys() - keys:
                missing['new.input.' + key] += 1
        result = {key: result[key] for key in keys}
    elif kind == 'state' and not legacy:
        for camp in ['attacker', 'defender']:
            result['state'][camp].pop('damageFactor')
            missing['new.state.' + camp + '.damageFactor'] += 1
    elif kind in {'round_begin', 'tick', 'round_end'} and not legacy:
        for key in ['round', 'time']:
            result.pop(key)
            missing['new.' + kind + '.' + key] += 1
    elif kind == 'skill_begin' and legacy:
        data = result['data']
        for key in data.keys() - BEGIN_FIELDS:
            missing['old.skill_begin.data.' + key] += 1
        result['data'] = {key: data[key] for key in BEGIN_FIELDS}
    elif kind == 'skill_cast' and legacy:
        data = result['data']['cast']
        for key in data.keys() - {'id', 'skill'}:
            missing['old.skill_cast.data.cast.' + key] += 1
        for key in data['skill'].keys() - {'id'}:
            missing['old.skill_cast.data.cast.skill.' + key] += 1
        result['data']['cast'] = {'id': data['id'], 'skill': {'id': data['skill']['id']}}
    return result


def compare(manifest):
    run = json.loads(manifest.read_bytes())
    if run['caseSet'] != 'growth-awake003' or run['status'] != 'EXECUTED_PENDING_STRICT_COMPARE' or run['nativeCompared'] or run['authoritative']:
        raise ValueError('Exact bounded completed highest003 family required')
    cases = []
    for case in run['caseSets'][0]['cases']:
        old_path = pinned(case['referencePhoneTrace'], case['referencePhoneTraceSHA256'])
        new_path = (manifest.parent / 'runtime' / case['localWire']).with_suffix('.native.jsonl')
        with opened(old_path) as old, new_path.open() as new:
            full_delta = None
            for index, (a, b) in enumerate(zip_longest(old, new)):
                delta = first_difference(json.loads(a) if a is not None else None,
                                         json.loads(b) if b is not None else None, '$[' + str(index) + ']')
                if full_delta is None:
                    full_delta = delta
        missing = collections.Counter()
        extras = collections.Counter()
        old_counts = collections.Counter()
        new_counts = collections.Counter()
        category_delta = {}
        category_count = collections.Counter()
        portable_delta = None
        checksum = hashlib.sha256()
        with (gzip.open(old_path, 'rb') if old_path.suffix == '.gz' else old_path.open('rb')) as old:
            for chunk in iter(lambda: old.read(1024 * 1024), b''):
                checksum.update(chunk)
        if checksum.hexdigest() != case['referencePhoneUncompressedSHA256']:
            raise ValueError('Original phone uncompressed SHA differs')
        def current_rows(stream):
            for line in stream:
                row = json.loads(line)
                new_counts[row['kind']] += 1
                if row['kind'] in NEW_OBSERVATIONS:
                    extras[row['kind']] += 1
                    continue
                if row['kind'] not in LEGACY_KINDS:
                    raise ValueError('Unknown unclassified new event: ' + row['kind'])
                yield row
        with opened(old_path) as old, new_path.open() as new:
            originals = (json.loads(line) for line in old)
            for index, (a, b) in enumerate(zip_longest(originals, current_rows(new))):
                if a is None or b is None:
                    if portable_delta is None:
                        portable_delta = {'path': '$[' + str(index) + ']', 'kind': 'legacy-event-count'}
                    continue
                old_counts[a['kind']] += 1
                left, right = projection(a, True, missing), projection(b, False, missing)
                delta = first_difference(left, right, '$[' + str(index) + ']')
                category = a['kind'] if a['kind'] == b['kind'] else 'event-order'
                category_count[category] += 1
                if delta and category not in category_delta:
                    category_delta[category] = delta
                if portable_delta is None:
                    portable_delta = delta
        cases.append({'case': case['case'], 'wireSHA256': case['wireSHA256'],
                      'phoneCompressedSHA256': sha(old_path), 'phoneUncompressedSHA256': checksum.hexdigest(),
                      'hostTraceSHA256': sha(new_path), 'fullSchemaExactMatch': full_delta is None,
                      'fullSchemaFirstDifference': full_delta,
                      'explicitHistoricalObserverProjectionExactMatch': portable_delta is None,
                      'historicalProjectionFirstDifference': portable_delta,
                      'phoneEventCounts': dict(old_counts), 'hostEventCounts': dict(new_counts),
                      'comparedCategoryCounts': dict(category_count), 'firstCategoryDifferences': category_delta,
                      'explicitUnavailableObservations': dict(missing),
                      'newWholeEventKindsUnavailableInOldPhoneCapture': dict(extras)})
    return {'schema': 'local-original-engine-growth-awake003-comparison-v1',
            'fullSchemaStatus': 'PASS' if all(c['fullSchemaExactMatch'] for c in cases) else 'FAIL',
            'explicitHistoricalObserverProjectionStatus': 'PASS' if all(c['explicitHistoricalObserverProjectionExactMatch'] for c in cases) else 'FAIL',
            'numericTolerance': 0, 'arrayOrderPreserved': True, 'rawSourcesChanged': False,
            'projectionIsNotCompleteHistoricalSchema': True, 'nativeCompared': False, 'authoritative': False,
            'full13FamilyGoMatrixExecuted': False, 'manifestSHA256': sha(manifest), 'cases': cases}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists():
        raise FileExistsError('Preserve first failures and comparisons')
    report = compare(args.manifest.resolve())
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({key: report[key] for key in ['fullSchemaStatus', 'explicitHistoricalObserverProjectionStatus']}, ensure_ascii=False))
    raise SystemExit(0 if report['explicitHistoricalObserverProjectionStatus'] == 'PASS' else 1)
