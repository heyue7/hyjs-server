"""Only existing immutable inputs; no battle generation or result hints.

Original phone capture metadata is a reference, never relabelled as a local
capture. Schedule bytes are restored only when their original SHA matches.
"""
from pathlib import Path
import hashlib
import json
import shutil
import gzip

ROOT = Path(__file__).resolve().parents[2]
PLAN = 'analysis/growth-first-20261016/p6-local-headless-poc-r1/host-next-cases.md'
PLAN_SHA = '777f3bbdd3d233d58aa92abc27c848b84f276a09d96ba674da357ed95648c756'
SETS = {
    'generic-mechanism-four': {
        'source': 'analysis/p6/generic-four-source-20261017/manifest.json',
        'count': 4, 'battleType': 11, 'phoneAnchor': None,
    },
    'trial047025-numeric': {
        'source': 'analysis/p6/p7-numeric047025-local-inputs-r1-20261017/manifest.json',
        'count': 4, 'battleType': 9, 'phoneAnchor': None,
    },
    'trial047025-natural': {
        'source': 'analysis/p6/p7-natural047025-local-inputs-r1-20261017/manifest.json',
        'count': 4, 'battleType': 9, 'phoneAnchor': None,
    },
    'trial047025': {
        'source': 'analysis/p6/p7-newbie047025-local-inputs-r2-20261017/manifest.json',
        'count': 4, 'battleType': 9, 'phoneAnchor': None,
    },
    'growth-awake003': {
        'source': 'analysis/p6/actual-growth-native-frames-v45/manifest.json',
        'sourceSHA256': '670158965e1f9ff74a99f135ae0a4e48ff89cba35b098c2480c298116179a1e9',
        'phone': 'analysis/p6/native-pvp-auto-priority-v45-20261014/manifest.json',
        'phoneSHA256': '991530f6c1928f3cf730605bbdca6e323896a3c946f6bb72a79e566960f70021',
        'actorSource': 'analysis/p6/p5-awake-highest-actors-20261014.json',
        'actorSourceSHA256': '280e6999c1c07d13c65d6a07e36e8036fb385324ea694f745173e8a69663d430',
        'sourceCount': 182, 'phoneCount': 14, 'count': 2, 'battleType': 2,
        'typedSuffix': None,
        'selectedLabels': ['p5-awake-highest-actors-20261014.json/hero_003/camp0',
                           'p5-awake-highest-actors-20261014.json/hero_003/camp1'],
    },
    'p7-rush': {
        'source': 'analysis/p6/p7-party-rush-pending-20261016/manifest.json',
        'sourceSHA256': '59114ed6ef3ec96d53a4a0557719d5876f96489b8f42bb9103896af40d4a2f45',
        'phone': 'analysis/p6/p7-party-rush-native-20261016/manifest.json',
        'phoneSHA256': '676808e34f645a610beae1e6ace003bef805c6fab71265dd1e73f44f133e1e35',
        'count': 4, 'battleType': 9, 'typedSuffix': '.rush-start.bin',
    },
    'p7-awake': {
        'source': 'analysis/p7/awake-playable-20261016/native-inputs/manifest.json',
        'sourceSHA256': '89f13163e4edb4461c13a3a95dcf3347a67e1e9a5091b9fd303cc7bd04635104',
        'phone': 'analysis/p6/awake16-native-20261016/manifest.json',
        'phoneSHA256': 'f2ac45b97d419f1d7ed33319cf80d3338ad70fb9c1dc959f61ea19e14acfa833',
        'count': 2, 'battleType': 16, 'typedSuffix': '.awake-start.bin',
    },
}


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def pinned(path, expected):
    path = ROOT / path
    if sha(path) != expected:
        raise ValueError('Pinned existing input/capture changed: ' + str(path))
    return path


def prepare(selected, work):
    if selected == 'generic-mechanism-four':
        from generic_mechanism_four import prepare as prepare_generic
        return prepare_generic(work)
    if selected == 'trial047025-numeric':
        from trial047025_numeric import prepare as prepare_numeric
        return prepare_numeric(work)
    if selected == 'trial047025-natural':
        from trial047025_natural import prepare as prepare_natural
        return prepare_natural(work)
    if selected == 'trial047025':
        from trial047025 import prepare as prepare_newbie
        return prepare_newbie(work)
    if selected == 'daily':
        folder = work / 'cases'
        folder.mkdir(exist_ok=True)
        rows = []
        for name in ['000.pb', '001.pb']:
            source = ROOT / 'build/native-batch-input/golden' / name
            shutil.copy2(source, folder / name)
            rows.append({'sourceWire': str(source.relative_to(ROOT)), 'wireSHA256': sha(source), 'localWire': str((folder / name).relative_to(work))})
        return [{'set': 'daily', 'caseDirectory': str(folder.relative_to(work)), 'cases': rows}]
    names = ['p7-rush', 'p7-awake'] if selected == 'p7' else [selected]
    groups = []
    for name in names:
        config = SETS[name]
        if name.startswith('p7-'):
            pinned(PLAN, PLAN_SHA)
        else:
            pinned(config['actorSource'], config['actorSourceSHA256'])
        source = pinned(config['source'], config['sourceSHA256'])
        phone = pinned(config['phone'], config['phoneSHA256'])
        source_data = json.loads(source.read_bytes())
        phone_data = json.loads(phone.read_bytes())
        if len(source_data['cases']) != config.get('sourceCount', config['count']) or len(phone_data['cases']) != config.get('phoneCount', config['count']):
            raise ValueError('Existing case denominator changed')
        captured = {row['wireSHA256']: row for row in phone_data['cases']}
        folder = work / 'cases' / name / 'highest'
        folder.mkdir(parents=True, exist_ok=True)
        rows = []
        source_cases = source_data['cases']
        if config.get('selectedLabels'):
            source_cases = [row for row in source_cases if row['case'] in config['selectedLabels']]
        if len(source_cases) != config['count']:
            raise ValueError('Exact selected existing-case denominator changed')
        for row in source_cases:
            if row['battleType'] != config['battleType'] or Path(row['wire']).name != row['wire']:
                raise ValueError('Only exact named original inputs accepted')
            old = captured[row['wireSHA256']]
            wire = pinned(source.parent / row['wire'], row['wireSHA256'])
            native_name = old.get('nativeResult') or Path(old['batchWire']).stem + '.native.jsonl'
            trace = phone.parent / native_name
            trace_sha = old.get('nativeSHA256') or phone_data['outputs'][native_name]['sha256']
            original_trace_sha = trace_sha
            if trace.is_file():
                pinned(trace, trace_sha)
            else:
                trace = trace.with_suffix(trace.suffix + '.gz')
                checksum = hashlib.sha256()
                with gzip.open(trace, 'rb') as captured_trace:
                    for chunk in iter(lambda: captured_trace.read(1024 * 1024), b''):
                        checksum.update(chunk)
                if checksum.hexdigest() != trace_sha:
                    raise ValueError('Original archived phone trace differs')
                trace_sha = sha(trace)
            with (gzip.open(trace, 'rt') if trace.suffix == '.gz' else trace.open()) as captured_trace:
                input_header = json.loads(next(captured_trace))
            if input_header.get('kind') != 'input' or input_header.get('seed') != row['seed'] or input_header.get('battleType') != row['battleType']:
                raise ValueError('Original phone input header differs')
            shutil.copy2(wire, folder / row['wire'])
            stem = Path(row['wire']).stem
            entry = {'case': row['case'], 'battleType': row['battleType'], 'fightId': row.get('fightId', input_header['fightId']),
                     'sourceWire': str(wire.relative_to(ROOT)), 'wireSHA256': sha(wire),
                     'localWire': str((folder / row['wire']).relative_to(work)),
                     'referencePhoneTrace': str(trace.relative_to(ROOT)), 'referencePhoneTraceSHA256': trace_sha,
                     'referencePhoneUncompressedSHA256': original_trace_sha}
            if config['typedSuffix'] is not None:
                typed = pinned(source.parent / row['typedCallback'], row['typedCallbackSHA256'])
                target = folder / (stem + config['typedSuffix'])
                shutil.copy2(typed, target)
                entry.update({'sourceTyped': str(typed.relative_to(ROOT)), 'typedSHA256': sha(typed),
                              'localTyped': str(target.relative_to(work))})
            else:
                # Existing type2 phone batch used original automatic AI with
                # both players. Its old wire optRecord stays untouched and
                # is not consumed as an automatic AI operation schedule.
                if phone_data.get('scope') != 'original-pvp-automatic-ai':
                    raise ValueError('Highest-awake original driver changed')
                entry['driver'] = 'original-pvp-automatic-ai; RunCase(path,false,false), both EnginePlayerAI'
            if name == 'p7-rush':
                if row.get('manualControlSource') != 'prepublished-diagnostic-schedule-v1':
                    raise ValueError('Unexpected original manual driver')
                for suffix, data, key in [('.rush-auto-start.txt', str(row['manualAutoStartRoundTime']).encode(), 'typedDriverSHA256'),
                                          ('.rush-auto-policy.txt', b'prepublished-diagnostic-schedule-v1', 'typedDriverPolicySHA256')]:
                    if hashlib.sha256(data).hexdigest() != old[key]:
                        raise ValueError('Restored original sidecar SHA differs: ' + key)
                    sidecar = folder / (stem + suffix)
                    sidecar.write_bytes(data)
                    entry[key] = sha(sidecar)
            rows.append(entry)
        groups.append({'set': name, 'caseDirectory': str(folder.relative_to(work)),
                       'sourcePlan': PLAN if name.startswith('p7-') else config['actorSource'],
                       'sourcePlanSHA256': PLAN_SHA if name.startswith('p7-') else config['actorSourceSHA256'],
                       'sourceManifest': config['source'], 'sourceManifestSHA256': config['sourceSHA256'],
                       'referencePhoneCapture': config['phone'], 'referencePhoneCaptureSHA256': config['phoneSHA256'],
                       'executionOrigin': 'local-coreclr-original-engine-not-phone-capture', 'cases': rows})
    return groups
