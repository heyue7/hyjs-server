"""Inspect or atomically activate the qualified named Trial1..6 catalogue.

Default inspect is read-only. Only activate --apply may write, after a pinned
candidate/runtime/source/schema/row preflight. One locked snapshot and one
@ready condition gate every UPDATE. Existing offers, Elite and P6 stay intact.
"""
import argparse
import csv
import fcntl
import hashlib
import io
import json
import os
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.shared_mysql import sql
from tools.release_go import operations

ACTIVATION = 'analysis/p7/rush-continuation-activate-20261016.sql'
CONSTS = {
    'rush': ('p7Rush.go', 'P7RushPolicySource', None),
    'scoped': ('p7ScopedRush.go', 'P7ScopedRushPolicySource', None),
    'manual': ('p6RushManualSettlement.go', 'P7RushManualDriverSource', None),
    'eliteManual': ('p6RushEliteManualSettlement.go', 'P7RushEliteManualDriverSource', None),
    'section': ('p7SectionReward.go', 'P7SectionRewardSource', None),
    'firstStages': ('p7SectionReward.go', 'P7SectionStageSource', None),
    'addition': ('p7RushAddition.go', 'P7RushAdditionSource', 'P7RushAdditionVersion'),
    'continuation': ('p7RushContinuation.go', 'P7RushContinuationSource', 'P7RushContinuationVersion'),
}
# SQL expressions are fixed code, never identifiers supplied by an operator.
SPECS = {
    'profiles': {'id': 'id', 'playerHash': 'SHA2(player,256)', 'gameHash': 'SHA2(game,256)'},
    'p7_rush_policies': {'id': 'id', 'enabled': 'enabled', 'eliteBuyCost': 'elite_buy_cost', 'trialResetCost': 'trial_reset_cost', 'sourceHash': 'SHA2(source,256)'},
    'p7_rush_scoped_policies': {'id': 'id', 'enabled': 'enabled', 'sourceHash': 'SHA2(source,256)'},
    'p7_rush_manual_policies': {'id': 'id', 'enabled': 'enabled', 'sourceHash': 'SHA2(source,256)'},
    'p7_section_reward_policies': {'id': 'id', 'enabled': 'enabled', 'version': 'version', 'sourceHash': 'SHA2(source,256)', 'publishedAt': 'published_at', 'startAt': 'start_at', 'endAt': 'end_at', 'sectionFrom': 'section_from', 'sectionTo': 'section_to', 'threeReward': 'three_reward', 'sixReward': 'six_reward', 'nineReward': 'nine_reward'},
    'p7_rush_addition_policies': {'id': 'id', 'enabled': 'enabled', 'version': 'version', 'sourceHash': 'SHA2(source,256)', 'publishedAt': 'published_at', 'startAt': 'start_at', 'endAt': 'end_at', 'threeOption': 'three_option', 'sixOption': 'six_option', 'nineOption': 'nine_option'},
    'p7_rush_continuation_policies': {'id': 'id', 'enabled': 'enabled', 'version': 'version', 'sourceHash': 'SHA2(source,256)', 'firstFloor': 'first_floor', 'lastFloor': 'last_floor'},
    'p7_rush_stage_rules': {'id': 'CONVERT(id USING utf8mb4)', 'kind': 'kind', 'taskID': 'task_id', 'floor': 'floor', 'star': 'star', 'conditionTaskID': 'condition_task_id', 'minLevel': 'min_level', 'enabled': 'enabled', 'scene': 'CONVERT(scene USING utf8mb4)', 'maxRound': 'max_round', 'winKey': 'win_key', 'winValue': 'win_value', 'enemyHash': 'SHA2(enemies,256)', 'firstReward': 'first_reward', 'reward': 'reward', 'recommendedPower': 'recommended_power', 'sourceHash': 'SHA2(source,256)'},
    'p6_combat_rules': {'id': 'id', 'authoritativeEnabled': 'authoritative_enabled', 'nativeCapabilityHash': 'native_capability_hash', 'clientHash': 'client_hash', 'maxRounds': 'max_rounds', 'timeoutMillis': 'timeout_millis', 'sourceHash': 'SHA2(source,256)'},
}
REQUIRED = set(SPECS) | {'p7_section_reward_offers', 'p7_rush_addition_offers', 'p7_rush_states', 'p7_rush_fights', 'p7_rush_receipts', 'p7_rush_progress'}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def literal(value):
    if value is None:
        return 'NULL'
    if isinstance(value, (bool, int, float)):
        if isinstance(value, float) and not __import__('math').isfinite(value):
            raise RuntimeError('Non-finite snapshot cannot enter an operation')
        return str(int(value)) if isinstance(value, bool) else str(value)
    return '0x' + str(value).encode('utf-8').hex() if str(value) else "''"


def named_const(source_root, filename, name):
    text = (source_root / 'hyjs-server/constUtil' / filename).read_text()
    match = re.search(r'\b' + re.escape(name) + r'\s*=\s*("[^\n]*")', text)
    if not match:
        raise RuntimeError('Pinned named rule is missing: ' + name)
    return json.loads(match.group(1))


def qualified_rules(source_root, validation, expected_sha):
    if source_root.parent != (ROOT / 'build/candidates').resolve() or not source_root.is_dir():
        raise RuntimeError('Use one direct frozen build/candidates directory')
    manifest = json.loads((source_root / 'manifest.json').read_text())
    qualification = manifest.get('qualification', {})
    if (ROOT / qualification.get('record', '')).resolve() != validation or sha(validation.read_bytes()) != qualification.get('sha256'):
        raise RuntimeError('Validation record is not pinned by the candidate')
    proof = json.loads(validation.read_text())
    if not proof.get('qualified') or proof.get('binarySHA256') != expected_sha or (ROOT / proof.get('candidate', '')).resolve() != source_root:
        raise RuntimeError('Qualified candidate and selected running binary differ')
    pins = {row['path']: row['sha256'] for row in manifest['files']}
    paths = {ACTIVATION, 'hyjs-server/db/sql/86_p7_rush_stage_rules.seed.sql',
             'hyjs-server/db/sql/184_p7_rush_continuation_policies.seed.sql',
             'hyjs-server/service/p7RushContinuationService.go', 'hyjs-server/service/p7SectionRewardService.go'}
    paths.update('hyjs-server/constUtil/' + entry[0] for entry in CONSTS.values())
    selected = {'sourceRoot': str(source_root), 'qualificationSHA256': sha(validation.read_bytes()), 'pinnedFiles': {}, 'rules': {}}
    for relative in sorted(paths):
        path = (source_root / relative).resolve()
        if not path.is_relative_to(source_root) or not path.is_file() or sha(path.read_bytes()) != pins.get(relative):
            raise RuntimeError('Frozen Trial source/SQL differs from its manifest: ' + relative)
        selected['pinnedFiles'][relative] = pins[relative]
    for key, (file, source_name, version_name) in CONSTS.items():
        selected['rules'][key] = {'sourceHash': sha(named_const(source_root, file, source_name).encode())}
        if version_name:
            selected['rules'][key]['version'] = named_const(source_root, file, version_name)
    if selected['rules']['addition']['version'] != 'local-trial-section-star-addition-v1' or selected['rules']['continuation']['version'] != 'local-trial-continuation-v1':
        raise RuntimeError('This narrow tool does not operate a different rule version')
    activation = (source_root / ACTIVATION).read_text()
    updates = re.findall(r'\bUPDATE\s+(\w+)\s+SET\b', activation, re.I)
    if updates != ['p7_rush_policies', 'p7_rush_scoped_policies', 'p7_rush_manual_policies', 'p7_rush_stage_rules', 'p7_section_reward_policies', 'p7_rush_addition_policies', 'p7_rush_continuation_policies', 'p7_rush_stage_rules'] or activation.count('7776000000') != 1:
        raise RuntimeError('Frozen activation SQL changed its narrow update/window contract')
    selected['activationSQLSHA256'] = pins[ACTIVATION]
    selected['stages'] = expected_stages(source_root, selected['rules'])
    return selected


def expected_stages(source_root, rules):
    result = {}
    base_hex = named_const(source_root, 'p7SectionReward.go', 'P7SectionEnemyHex')
    base_hash = named_const(source_root, 'p7SectionReward.go', 'P7SectionEnemySHA')
    if sha(bytes.fromhex(base_hex)) != base_hash:
        raise RuntimeError('Pinned original first-section NPC does not match its SHA')
    for floor in range(1, 4):
        for star in range(1, 4):
            row = dict(id=f'local-section:{floor}:{star}', kind=1, taskID=f'local-section-{floor}', floor=floor, star=star,
                       conditionTaskID='', minLevel=22, scene='map_grass_001', maxRound=40, winKey=0, winValue=0,
                       enemyHash=base_hash, firstReward='', reward='point,10', recommendedPower=0, sourceHash=rules['firstStages']['sourceHash'])
            result[row['id']] = row
    text = (source_root / 'hyjs-server/db/sql/86_p7_rush_stage_rules.seed.sql').read_text()
    match = re.search(r'INSERT\s+IGNORE\s+INTO\s+p7_rush_stage_rules\(([^)]+)\)\s+VALUES\s*(.*);\s*$', text, re.I | re.S)
    if not match:
        raise RuntimeError('Pinned continuation seed format is unsupported')
    columns = match.group(1).split(',')
    count = 0
    for line in match.group(2).splitlines():
        line = line.strip().rstrip(',;')
        if not line:
            continue
        if not line.startswith('(') or not line.endswith(')'):
            raise RuntimeError('Pinned continuation seed row is malformed')
        values = next(csv.reader(io.StringIO(line[1:-1]), quotechar="'", doublequote=True))
        if len(values) != len(columns):
            raise RuntimeError('Pinned continuation seed columns differ')
        raw = dict(zip(columns, values))
        row = {'id': raw['id'], 'kind': int(raw['kind']), 'taskID': raw['task_id'], 'floor': int(raw['floor']), 'star': int(raw['star']),
               'conditionTaskID': raw['condition_task_id'], 'minLevel': int(raw['min_level']), 'scene': raw['scene'], 'maxRound': int(raw['max_round']),
               'winKey': int(raw['win_key']), 'winValue': int(raw['win_value']), 'enemyHash': sha(bytes.fromhex(raw['enemies'][2:])),
               'firstReward': raw['first_reward'], 'reward': raw['reward'], 'recommendedPower': float(raw['recommended_power']), 'sourceHash': sha(raw['source'].encode())}
        if row['id'] in result or not 4 <= row['floor'] <= 6 or not 1 <= row['star'] <= 3 or row['sourceHash'] != rules['continuation']['sourceHash']:
            raise RuntimeError('Pinned continuation seed is outside this operation')
        result[row['id']] = row
        count += 1
    if count != 9 or set(result) != {f'local-section:{f}:{s}' for f in range(1, 4) for s in range(1, 4)} | {f'local-continuation:{f}:{s}' for f in range(4, 7) for s in range(1, 4)}:
        raise RuntimeError('Require exactly nine first-section and nine continuation rows')
    return result


def available_tables():
    names = ','.join(literal(name) for name in sorted(REQUIRED))
    rows = sql('SELECT TABLE_NAME,ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND TABLE_NAME IN (' + names + ');').strip().splitlines()
    return dict(row.split('\t', 1) for row in rows)


def snapshot():
    fields = []
    for table, spec in SPECS.items():
        pairs = ','.join("'" + key + "'," + expression for key, expression in spec.items())
        where = ' WHERE id IN (1,4)' if table == 'profiles' else ''
        fields.append("'" + table + "',COALESCE((SELECT JSON_ARRAYAGG(JSON_OBJECT(" + pairs + ')) FROM ' + table + where + '),JSON_ARRAY())')
    result = json.loads(sql('SELECT JSON_OBJECT(' + ','.join(fields) + ');'))
    for rows in result.values():
        rows.sort(key=lambda row: str(row['id']))
    return result


def blockers(state, selected):
    failures = []
    rules = selected['rules']

    def one(table, ident=1):
        rows = [r for r in state.get(table, []) if r.get('id') == ident]
        if len(rows) != 1:
            failures.append(table + ': required row missing/duplicate')
            return {}
        return rows[0]

    for table in SPECS:
        expected = 2 if table in ('profiles', 'p7_rush_manual_policies') else 1
        if table != 'p7_rush_stage_rules' and len(state.get(table, [])) != expected:
            failures.append(table + ': unexpected row count')
    for ident in [1, 4]:
        row = one('profiles', ident)
        if any(not re.fullmatch('[0-9a-f]{64}', row.get(key, '')) for key in ['playerHash', 'gameHash']):
            failures.append('protected profile fingerprint missing: ' + str(ident))
    mappings = [('p7_rush_policies', 'rush'), ('p7_rush_scoped_policies', 'scoped'), ('p7_rush_manual_policies', 'manual'),
                ('p7_section_reward_policies', 'section'), ('p7_rush_addition_policies', 'addition'), ('p7_rush_continuation_policies', 'continuation')]
    for table, key in mappings:
        row = one(table)
        if row.get('sourceHash') != rules[key]['sourceHash'] or row.get('enabled') not in (0, 1):
            failures.append(table + ': wrong named source/enabled state')
        if key in ('addition', 'continuation') and row.get('version') != rules[key]['version']:
            failures.append(table + ': wrong version')
    elite = one('p7_rush_manual_policies', 2)
    if elite.get('sourceHash') != rules['eliteManual']['sourceHash'] or elite.get('enabled') not in (0, 1):
        failures.append('Elite manual source/state is unexpected')
    root = one('p7_rush_policies')
    if root.get('eliteBuyCost') != 50 or root.get('trialResetCost') != 50:
        failures.append('Original finite buy/reset costs changed')
    section = one('p7_section_reward_policies')
    if any(section.get(key) != value for key, value in dict(version='local-first-section-v1', sectionFrom=1, sectionTo=3, threeReward='item_001,1', sixReward='item_001,2', nineReward='item_001,3').items()):
        failures.append('First-section reward contract changed')
    addition = one('p7_rush_addition_policies')
    if any(addition.get(key) != value for key, value in dict(threeOption='peratt,0.03', sixOption='perhp,0.06', nineOption='perpdef,0.09').items()):
        failures.append('Three original-displayed local addition options changed')
    continuation = one('p7_rush_continuation_policies')
    if continuation.get('firstFloor') != 4 or continuation.get('lastFloor') != 6:
        failures.append('Continuation published range differs from4..6')
    for label, row in [('section', section), ('addition', addition)]:
        times = [row.get(key, -1) for key in ['publishedAt', 'startAt', 'endAt']]
        if not (times == [0, 0, 0] or 0 < times[0] <= times[1] < times[2]):
            failures.append(label + ': prior publication window is malformed')
    stage_rows = {row['id']: row for row in state.get('p7_rush_stage_rules', [])}
    for ident, expected in selected['stages'].items():
        actual = stage_rows.get(ident, {})
        if actual.get('enabled') not in (0, 1) or any(actual.get(key) != value for key, value in expected.items()):
            failures.append('Catalogue metadata/NPC/reward mismatch: ' + ident)
    hashes = {rules['firstStages']['sourceHash'], rules['continuation']['sourceHash']}
    for row in stage_rows.values():
        if row['id'] not in selected['stages'] and (row.get('sourceHash') in hashes or row.get('enabled') == 1 and row.get('kind') == 1 and 1 <= row.get('floor', -1) <= 6):
            failures.append('Extra named/active Trial row conflicts with18-row catalogue: ' + str(row['id']))
    scoped = one('p7_rush_scoped_policies')
    if not (root.get('enabled') == 1 and scoped.get('enabled') == 1) and (elite.get('enabled') == 1 or any(r.get('kind') == 2 and r.get('enabled') == 1 for r in stage_rows.values())):
        failures.append('Enabling shared Trial gates would also expand an existing Elite path')
    if one('p6_combat_rules').get('authoritativeEnabled') != 0:
        failures.append('P6 global authority must remain closed')
    return failures


def guarded_activation_query(before, selected):
    failures = blockers(before, selected)
    if failures:
        raise RuntimeError('Preflight refused: ' + '; '.join(failures))
    statements = ['SET SESSION innodb_lock_wait_timeout=5', 'SET SESSION lock_wait_timeout=5', 'START TRANSACTION', 'SET @ready=0']
    guards = []
    for table, spec in SPECS.items():
        where = ' WHERE id IN (1,4)' if table == 'profiles' else ''
        statements.append('SELECT id FROM ' + table + where + ' ORDER BY id FOR UPDATE')
        guards.append('(SELECT COUNT(*) FROM ' + table + where + ')=' + str(len(before[table])))
        for row in before[table]:
            terms = []
            for key, expr in spec.items():
                value = row[key]
                terms.append(('BINARY ' + expr + ' <=> BINARY ' + literal(value)) if isinstance(value, str) else '(' + expr + ') <=> ' + literal(value))
            guards.append('(SELECT COUNT(*) FROM ' + table + ' WHERE ' + ' AND '.join('(' + term + ')' for term in terms) + ')=1')
    # Every locked row must still equal the fully validated read-only snapshot.
    # The one ready value therefore controls the entire catalogue and policies.
    statements += ['SET @published=CAST(UNIX_TIMESTAMP(NOW(3))*1000 AS SIGNED)',
                   'SET @ready=COALESCE((' + ' AND '.join(guards) +
                   ' AND @published>=(SELECT published_at FROM p7_section_reward_policies WHERE id=1)'
                   ' AND @published>=(SELECT published_at FROM p7_rush_addition_policies WHERE id=1)),0)']
    for table in ['p7_rush_policies', 'p7_rush_scoped_policies', 'p7_rush_manual_policies', 'p7_rush_continuation_policies']:
        statements.append('UPDATE ' + table + ' SET enabled=1 WHERE id=1 AND @ready=1')
    for table in ['p7_section_reward_policies', 'p7_rush_addition_policies']:
        statements.append('UPDATE ' + table + ' SET enabled=1,published_at=@published,start_at=@published,end_at=@published+7776000000 WHERE id=1 AND @ready=1')
    ids = ','.join(literal(key) for key in sorted(selected['stages']))
    statements += ['UPDATE p7_rush_stage_rules SET enabled=1 WHERE id IN (' + ids + ') AND @ready=1',
                   'COMMIT', "SELECT CONCAT('HYJS_P7_READY=',COALESCE(@ready,0))"]
    return ';\n'.join(statements) + ';'


def restore_values(before, selected):
    values = {}
    for table, rows in before.items():
        if table in ('profiles', 'p6_combat_rules'):
            continue
        touched = [row for row in rows if row['id'] in selected['stages']] if table == 'p7_rush_stage_rules' else [row for row in rows if row['id'] == 1]
        values[table] = [{key: row[key] for key in ['id', 'enabled', 'publishedAt', 'startAt', 'endAt'] if key in row} for row in touched]
    return values


def already_active(state, selected):
    targets = ['p7_rush_policies', 'p7_rush_scoped_policies', 'p7_rush_manual_policies',
               'p7_section_reward_policies', 'p7_rush_addition_policies', 'p7_rush_continuation_policies']
    if any(next(r for r in state[table] if r['id'] == 1)['enabled'] != 1 for table in targets):
        return False
    stages = {r['id']: r for r in state['p7_rush_stage_rules']}
    if any(stages[key]['enabled'] != 1 for key in selected['stages']):
        return False
    section = state['p7_section_reward_policies'][0]
    addition = state['p7_rush_addition_policies'][0]
    windows = [[row[key] for key in ['publishedAt', 'startAt', 'endAt']] for row in [section, addition]]
    return windows[0] == windows[1] and windows[0][0] == windows[0][1] and windows[0][2] - windows[0][1] == 7776000000


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['inspect', 'activate'], nargs='?', default='inspect')
    parser.add_argument('--source-root', type=Path, default=ROOT / 'build/candidates/playable-continuation-20261016')
    parser.add_argument('--validation', type=Path)
    parser.add_argument('--expected-binary-sha')
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--record', type=Path)
    args = parser.parse_args()
    if args.apply and args.action != 'activate':
        raise RuntimeError('--apply is only valid for activate')
    if args.action == 'activate' and not re.fullmatch('[0-9a-f]{64}', args.expected_binary_sha or ''):
        raise RuntimeError('activate requires an explicit exact --expected-binary-sha')
    source_root = args.source_root.resolve()
    manifest = json.loads((source_root / 'manifest.json').read_text())
    validation = args.validation.resolve() if args.validation else (ROOT / manifest['qualification']['record']).resolve()
    live = operations().get('binarySha256')
    if not re.fullmatch('[0-9a-f]{64}', live or '') or args.expected_binary_sha and live != args.expected_binary_sha:
        raise RuntimeError('Running binary differs from the selected operation')
    selected = qualified_rules(source_root, validation, live)
    result = dict(action=args.action, applied=False, binarySHA256=live, namedRules=selected,
                  activationSQLSHA256=selected['activationSQLSHA256'],
                  protectedProfiles=[1, 4], offersModified=False, eliteModified=False, p6Modified=False)
    available = available_tables()
    missing = sorted(REQUIRED - set(available))
    non_transactional = sorted(table for table, engine in available.items() if engine != 'InnoDB')
    if missing or non_transactional:
        result.update(status='BLOCKED_SCHEMA_NO_CHANGE', missingTables=missing, nonInnoDBTables=non_transactional)
    else:
        before = snapshot()
        issues = blockers(before, selected)
        result.update(before=before, blockers=issues, originalRestoreValues=restore_values(before, selected),
                      status='BLOCKED_PREFLIGHT_NO_CHANGE' if issues else 'READY_INSPECT_ONLY')
        current = not issues and already_active(before, selected)
        if current:
            result['status'] = 'ALREADY_ACTIVE_NO_CHANGE'
        if args.action == 'activate' and args.apply and not issues and not current:
            if not args.record or args.record.exists():
                raise RuntimeError('Applying requires a new immutable --record path')
            args.record.parent.mkdir(parents=True, exist_ok=True)
            # Reserve the independent report before SQL. Existing reports are never reopened.
            fd = os.open(args.record, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, 'w') as report, open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                result['status'] = 'PENDING_GUARDED_ACTIVATION'
                report.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n'); report.flush(); os.fsync(report.fileno())
                try:
                    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    if operations().get('binarySha256') != live:
                        raise RuntimeError('Runtime changed before activation; no SQL update submitted')
                    reply = sql(guarded_activation_query(before, selected))
                    if 'HYJS_P7_READY=1' not in reply.splitlines():
                        result['status'] = 'LOCKED_PRECONDITIONS_CHANGED_NO_CHANGE'
                        result['after'] = snapshot()
                    else:
                        result['applied'] = True
                        result['after'] = snapshot()
                        result['protectedProfilesSame'] = result['after']['profiles'] == before['profiles']
                        result['eliteSame'] = [r for r in result['after']['p7_rush_stage_rules'] if r['kind'] == 2] == [r for r in before['p7_rush_stage_rules'] if r['kind'] == 2] and [r for r in result['after']['p7_rush_manual_policies'] if r['id'] == 2] == [r for r in before['p7_rush_manual_policies'] if r['id'] == 2]
                        result['p6Same'] = result['after']['p6_combat_rules'] == before['p6_combat_rules']
                        result['runtimeHashSame'] = operations().get('binarySha256') == live
                        result['allTargetsEnabled'] = not blockers(result['after'], selected) and already_active(result['after'], selected)
                        result['status'] = 'APPLIED' if all(result[k] for k in ['protectedProfilesSame', 'eliteSame', 'p6Same', 'runtimeHashSame', 'allTargetsEnabled']) else 'APPLIED_CONCURRENT_CHANGE_REQUIRES_REVIEW'
                except BlockingIOError:
                    result.update(status='SQL_WINDOW_BUSY_NO_CHANGE')
                except Exception as error:
                    result.update(status='ACTION_FAILED_REVIEW_REPORT', error=str(error))
                    raise
                finally:
                    report.seek(0); report.truncate(); report.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n'); report.flush(); os.fsync(report.fileno())
            args.record = None
    if args.record:
        args.record.parent.mkdir(parents=True, exist_ok=True)
        with args.record.open('x') as output:
            output.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False))
    if result['status'].startswith(('BLOCKED', 'LOCKED', 'SQL_WINDOW', 'APPLIED_CONCURRENT')):
        raise SystemExit(2)


if __name__ == '__main__':
    main()
