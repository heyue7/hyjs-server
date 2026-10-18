"""Guarded operation for qualified local Awake chapter5/copy501 only.

Default inspection is read-only; --offline never reads runtime or MySQL.
Only explicit --enable/--disable changes policy2+3 and stage501 enabled flags.
Frozen runs, receipts, counters, scores, rewards and ordinary profiles are never
written. A close retains frozen-run fulfilment in the published service.
"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.shared_mysql import sql
from tools.release_go import operations
from tools.p7_rush_continuation_ops import literal, named_const

DEFAULT_CANDIDATE = ROOT / 'build/candidates/awake-playable-r4-20261016'
DEFAULT_BINARY_SHA = '7b84003e9022c4238e1c8b59dd3ef2e14ab2179b662ca8b83256f358c2b49b8e'
CERTIFICATE = 'analysis/p6/awake16-party001003-capability-certificate-20261016.json'
PROOF = 'b6b62036469c1af8eafad7a5f6f210c9a3f83c0b58bc082d361c886721ecbc14'
HISTORICAL_PROOF = {'analysis/p3/awake-field-wanted-r1-awake16-20261016.json',
                    'analysis/p3/awake-field-wanted-r1-awake16-20261016.log'}
SPECS = {
    'profiles': {'id': 'id', 'playerHash': 'SHA2(player,256)', 'gameHash': 'SHA2(game,256)',
                 'hasRun': "IF(JSON_EXTRACT(game,'$.run') IS NULL OR JSON_TYPE(JSON_EXTRACT(game,'$.run'))='NULL',0,1)"},
    'p7_awake_policies': {'id': 'id', 'enabled': 'enabled', 'minLevel': 'min_level', 'sourceHash': 'SHA2(source,256)'},
    'p7_awake_stage_rules': {'id': 'CONVERT(copy_id USING utf8mb4)', 'chapter': 'chapter_id', 'enabled': 'enabled',
                            'restored': 'content_restored', 'power': 'fight_power', 'sourceHash': 'SHA2(source,256)',
                            'content': 'CONVERT(content USING utf8mb4)', 'contentHash': 'SHA2(content,256)'},
    'p7_awake_states': {'id': 'player_id', 'day': 'day', 'used': 'used'},
    'p7_awake_runs': {'id': 'CONVERT(id USING utf8mb4)', 'playerID': 'player_id', 'copyID': 'copy_id',
                      'diagnostic': 'diagnostic', 'startAt': 'start_at', 'endAt': 'end_at',
                      'freezeHash': 'SHA2(freeze,256)', 'actorHash': 'SHA2(actor,256)', 'frameHash': 'SHA2(enter_frame,256)'},
    'p7_awake_explorations': {'id': 'CONVERT(run_id USING utf8mb4)', 'playerID': 'player_id', 'status': 'status',
                              'stateHash': 'SHA2(state,256)', 'enterStamp': 'enter_stamp', 'settleStamp': 'settle_stamp',
                              'settledAt': 'settled_at', 'receiptHash': 'SHA2(receipt,256)'},
    'p7_awake_fights': {'id': 'CONVERT(id USING utf8mb4)', 'playerID': 'player_id', 'status': 'status',
                        'scopeProof': 'scope_proof', 'startHash': 'SHA2(start,256)', 'inputHash': 'input_hash',
                        'terminalHash': 'SHA2(terminal,256)', 'endHash': 'SHA2(end_request,256)', 'replyHash': 'SHA2(reply,256)'},
    'p7_rush_states': {'id': 'player_id', 'activeFight': 'CONVERT(active_fight USING utf8mb4)'},
    'p6_combat_rules': {'id': 'id', 'enabled': 'authoritative_enabled', 'sourceHash': 'SHA2(source,256)'},
}
KEYS = {'p7_awake_stage_rules': 'copy_id', 'p7_awake_states': 'player_id',
        'p7_awake_explorations': 'run_id', 'p7_rush_states': 'player_id'}


def sha(data):
    return hashlib.sha256(data).hexdigest()


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':'))


def qualified_rules(source_root, validation, expected_binary):
    if source_root.parent != (ROOT / 'build/candidates').resolve() or not source_root.is_dir():
        raise RuntimeError('Use a direct frozen build/candidates directory')
    manifest = json.loads((source_root / 'manifest.json').read_text())
    pinned = manifest.get('qualification', {})
    if (ROOT / pinned.get('record', '')).resolve() != validation or sha(validation.read_bytes()) != pinned.get('sha256'):
        raise RuntimeError('Candidate qualification record/hash differs')
    proof = json.loads(validation.read_text())
    if not proof.get('qualified') or proof.get('binarySHA256') != expected_binary or (ROOT / proof.get('candidate', '')).resolve() != source_root:
        raise RuntimeError('Qualified candidate/app SHA binding differs')
    binary = (ROOT / proof.get('binary', '')).resolve()
    if not binary.is_relative_to(ROOT / 'build') or not binary.is_file() or sha(binary.read_bytes()) != expected_binary:
        raise RuntimeError('Qualified application bytes differ')
    pins = {}
    for row in manifest['files']:
        relative = row['path']
        path = (source_root / relative).resolve()
        if relative in pins or not path.is_relative_to(source_root) or not path.is_file() or sha(path.read_bytes()) != row['sha256']:
            raise RuntimeError('Frozen candidate pin changed: ' + relative)
        pins[relative] = row['sha256']
    required = {CERTIFICATE, 'hyjs-server/constUtil/p7Awake.go', 'hyjs-server/constUtil/p7Awake16Scope.go',
                'hyjs-server/service/p7AwakeService.go', 'hyjs-server/service/p7Awake16ScopeService.go',
                'analysis/tables/TableAwakeCopyChapter.tsv', 'analysis/tables/TableGameConfigure.tsv',
                'hyjs-server/db/sql/86_p7_awake_policies.sql', 'hyjs-server/db/sql/86_p7_awake_stage_rules.sql',
                'hyjs-server/db/sql/188_p7_awake_explorations.sql', 'hyjs-server/db/sql/188_p7_awake_fights.sql'}
    if not required <= pins.keys():
        raise RuntimeError('Qualified Awake source/proof/SQL/table pins missing')
    native_proof = named_const(source_root, 'p7Awake16Scope.go', 'P7Awake16NativeProof')
    certificate_path = source_root / CERTIFICATE
    certificate = json.loads(certificate_path.read_text())
    if (native_proof != PROOF or sha(certificate_path.read_bytes()) != PROOF
            or certificate.get('status') != 'THIRTEEN_LAYERS_PASS' or certificate.get('nativeCases') != 2
            or len(certificate.get('layers', [])) != 13
            or certificate.get('scope') != 'original-awake16-auto-integer001003-tal0-fullhp-ap0-copy10-npc2-v1'
            or certificate.get('globalAuthoritative') is not False):
        raise RuntimeError('Independent original type16 proof scope differs')
    external_evidence = []
    for evidence in certificate['evidence']:
        relative = evidence['path']
        path = (source_root / relative).resolve()
        if relative in HISTORICAL_PROOF and relative not in pins:
            path = (ROOT / relative).resolve()
            external_evidence.append(dict(path=relative, sha256=evidence['sha256'], storage='historical workspace evidence; bound by frozen certificate'))
        elif pins.get(relative) != evidence['sha256'] or not path.is_relative_to(source_root):
            raise RuntimeError('Independent native proof candidate pin differs: ' + relative)
        if not path.is_file() or sha(path.read_bytes()) != evidence['sha256']:
            raise RuntimeError('Independent native proof evidence changed: ' + evidence['path'])
    sources = {str(i): named_const(source_root, 'p7Awake.go', name) for i, name in
               [(1, 'P7AwakeSource'), (2, 'P7AwakeExploreSource'), (3, 'P7AwakeBattleSource')]}
    if not sources['2'].startswith('local-awake-forest-explore-v1:chapter5/copy501;') or not sources['3'].startswith('local-awake16-forest-v1:'):
        raise RuntimeError('Different local Awake rule version')
    enemy = bytes.fromhex(named_const(source_root, 'p7Awake.go', 'P7AwakeEnemyHex'))
    if sha(enemy) != named_const(source_root, 'p7Awake.go', 'P7AwakeEnemySHA'):
        raise RuntimeError('Original NPC bytes differ')
    if named_const(source_root, 'p7Awake.go', 'P7AwakeScoreReward') != 'hero_exp_002,1;coin,5000':
        raise RuntimeError('Different actual growth-consumer score award')
    tables = {}
    for name in ['AwakeCopyChapter', 'GameConfigure']:
        lines = (source_root / ('analysis/tables/Table' + name + '.tsv')).read_text().splitlines()
        tables[name] = [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line.strip()]
    chapter = next(row for row in tables['AwakeCopyChapter'] if row['chapterId'] == '5')
    if len(tables['AwakeCopyChapter']) != 5 or any(chapter.get(k) != v for k, v in
            dict(openDay='5,6,7', sceneId='map_forestfield_001', bornPos='0,0,1', bornAngle='0', BattleMap='map_forest_003').items()):
        raise RuntimeError('Original five-chapter/Friday-Sunday/scene contract differs')
    settings = {row['StaticName']: row['NameValue'] for row in tables['GameConfigure']}
    for key, value in dict(awakeCopyPlayTimes='2', awakeCopyTimeLimit='30', awakeCopyReviveCount='3', awakeCopyStaminaCost='0').items():
        if settings.get(key) != value:
            raise RuntimeError('Original Awake setting differs: ' + key)
    positions = ['-1.683333397,-0.078914553,3.266666889', '-2.866666794,-0.060859005,1.400000095',
                 '-0.673333359,-0.096414578,5.273333549', '4.283333619,-0.102525691,3.727777918']
    content = {'boxes': [dict(boxId='box' + str(i), guid='local-forest-box-' + str(i), pos=pos, isOpen=0, angle=0)
                         for i, pos in enumerate(positions, 1)],
               'monsters': [dict(monId='mon_030', guid='local-forest-mon-1', pos='-4.238889058,-0.021970133,-1.994444529', hpPercent=1, angle=0, monsterType=1)],
               'source': sources['2']}
    source_hashes = {key: sha(value.encode()) for key, value in sources.items()}
    return {'sourceRoot': str(source_root), 'qualificationSHA256': sha(validation.read_bytes()),
            'candidatePinsVerified': len(pins), 'nativeProof': native_proof,
            'binary': str(binary), 'externalEvidence': external_evidence,
            'policySourceHashes': source_hashes, 'expectedSourceHash': sha(canonical(source_hashes).encode()),
            'stageContent': content, 'chapter5': chapter,
            'originalSettings': {key: settings[key] for key in settings if key in
                                 ('awakeCopyPlayTimes', 'awakeCopyTimeLimit', 'awakeCopyReviveCount', 'awakeCopyStaminaCost')}}


def snapshot():
    pairs = []
    for table, spec in SPECS.items():
        # These names are fixed code, with ordinary UTF-8 JSON object keys.
        fields = ','.join("'" + key + "'," + expr for key, expr in spec.items())
        pairs.append("'" + table + "',COALESCE((SELECT JSON_ARRAYAGG(JSON_OBJECT(" + fields + ')) FROM ' + table + '),JSON_ARRAY())')
    state = json.loads(sql('SELECT JSON_OBJECT(' + ','.join(pairs) + ');'))
    for rows in state.values():
        rows.sort(key=lambda row: str(row['id']))
    return state


def blockers(state, selected, enabling):
    failures = []
    policies = {row['id']: row for row in state['p7_awake_policies']}
    if set(policies) != {1, 2, 3}:
        failures.append('Require exactly original policy1 plus local policies2/3')
    for ident in [1, 2, 3]:
        row = policies.get(ident, {})
        if row.get('sourceHash') != selected['policySourceHashes'][str(ident)] or row.get('minLevel') != 70 or row.get('enabled') not in (0, 1):
            failures.append('Named policy source/level/state differs: ' + str(ident))
    if policies.get(1, {}).get('enabled') != 0:
        failures.append('Original compatibility policy1 must stay closed')
    stages = {row['id']: row for row in state['p7_awake_stage_rules']}
    stage = stages.get('501', {})
    try:
        content = json.loads(stage.get('content', ''))
    except (ValueError, TypeError):
        content = None
    if (stage.get('chapter') != '5' or stage.get('restored') != 1 or stage.get('power') != 100
            or stage.get('sourceHash') != selected['policySourceHashes']['2'] or stage.get('enabled') not in (0, 1)
            or content != selected['stageContent']):
        failures.append('Local501 exact source/metadata/four-box/one-monster catalogue differs')
    if any(row['id'] != '501' and (row['enabled'] or row['sourceHash'] in selected['policySourceHashes'].values()) for row in stages.values()):
        failures.append('Extra active/named Awake content conflicts with local501 scope')
    profiles = {row['id']: row for row in state['profiles']}
    if not {1, 4} <= profiles.keys() or any(not re.fullmatch('[0-9a-f]{64}', profiles[i].get(key) or '') for i in [1, 4] for key in ['playerHash', 'gameHash']):
        failures.append('Protected account1/account4 fingerprints missing')
    if len(state['p6_combat_rules']) != 1 or state['p6_combat_rules'][0]['enabled'] != 0:
        failures.append('Global P6 authority must remain closed')
    if enabling:
        if any(row['hasRun'] for row in profiles.values()):
            failures.append('An ordinary profile has an active Run')
        if any(row['status'] == 'active' for row in state['p7_awake_explorations']):
            failures.append('An existing Awake exploration is active; preserve it before enabling')
        if any(row['status'] == 'pending' for row in state['p7_awake_fights']) or any(row['activeFight'] for row in state['p7_rush_states']):
            failures.append('An Awake/Rush battle is pending')
    return failures


def guarded_query(before, selected, enable):
    issues = blockers(before, selected, enable)
    if issues:
        raise RuntimeError('Locked operation refused: ' + '; '.join(issues))
    statements = ['SET SESSION innodb_lock_wait_timeout=5', 'SET SESSION lock_wait_timeout=5',
                  'SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ', 'START TRANSACTION', 'SET @ready=0']
    guards = []
    # Profile locks match service lock order. Full indexed scans also guard new rows.
    for table, spec in SPECS.items():
        key = KEYS.get(table, 'id')
        statements.append('SELECT ' + key + ' FROM ' + table + ' ORDER BY ' + key + ' FOR UPDATE')
        guards.append('(SELECT COUNT(*) FROM ' + table + ')=' + str(len(before[table])))
        for row in before[table]:
            terms = [('BINARY (' + expr + ') <=> BINARY ' + literal(row[name])) if isinstance(row[name], str)
                     else '(' + expr + ') <=> ' + literal(row[name]) for name, expr in spec.items()]
            guards.append('(SELECT COUNT(*) FROM ' + table + ' WHERE ' + ' AND '.join('(' + term + ')' for term in terms) + ')=1')
    for ident in [2, 3]:
        guards.append('(SELECT SHA2(source,256) FROM p7_awake_policies WHERE id=' + str(ident) + ')=' + literal(selected['policySourceHashes'][str(ident)]))
    statements.append('SET @ready=COALESCE((' + ' AND '.join(guards) + '),0)')
    statements += ['UPDATE p7_awake_policies SET enabled=' + str(int(enable)) + ' WHERE id IN (2,3) AND @ready=1',
                   'UPDATE p7_awake_stage_rules SET enabled=' + str(int(enable)) + " WHERE copy_id='501' AND @ready=1",
                   'COMMIT', "SELECT CONCAT('HYJS_AWAKE_READY=',COALESCE(@ready,0))"]
    return ';\n'.join(statements) + ';'


def after_check(before, after, enable):
    expected = json.loads(json.dumps(before))
    for row in expected['p7_awake_policies']:
        if row['id'] in (2, 3):
            row['enabled'] = int(enable)
    for row in expected['p7_awake_stage_rules']:
        if row['id'] == '501':
            row['enabled'] = int(enable)
    return expected == after


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument('--enable', action='store_true')
    modes.add_argument('--disable', action='store_true')
    parser.add_argument('--offline', action='store_true', help='Only candidate/proof/table preflight; no runtime/SQL access')
    parser.add_argument('--source-root', type=Path, default=DEFAULT_CANDIDATE)
    parser.add_argument('--validation', type=Path)
    parser.add_argument('--expected-binary-sha', default=DEFAULT_BINARY_SHA)
    parser.add_argument('--expected-source-hash', help='Exact expectedSourceHash emitted by preflight; required for a write')
    parser.add_argument('--record', type=Path)
    args = parser.parse_args()
    writing = args.enable or args.disable
    if args.offline and writing:
        parser.error('--offline cannot write')
    if not re.fullmatch('[0-9a-f]{64}', args.expected_binary_sha):
        parser.error('Require an exact binary SHA256')
    source = args.source_root.resolve()
    manifest = json.loads((source / 'manifest.json').read_text())
    validation = args.validation.resolve() if args.validation else (ROOT / manifest['qualification']['record']).resolve()
    selected = qualified_rules(source, validation, args.expected_binary_sha)
    if args.expected_source_hash and args.expected_source_hash != selected['expectedSourceHash']:
        raise RuntimeError('Operator expectedSourceHash differs from pinned local rules')
    if writing and (args.expected_source_hash != selected['expectedSourceHash'] or not args.record):
        parser.error('Writing requires --expected-source-hash and a new --record path')
    if args.record:
        args.record = args.record.resolve()
        if not args.record.is_relative_to(ROOT / 'analysis') or args.record.exists():
            parser.error('Use a new immutable record under analysis')
    result = {'action': 'enable' if args.enable else 'disable' if args.disable else 'inspect', 'applied': False,
              'expectedBinarySHA256': args.expected_binary_sha, 'namedRules': selected,
              'writes': {'policyIDs': [2, 3], 'stageIDs': ['501'], 'columns': ['enabled']},
              'frozenFulfilmentPreserved': True, 'ordinaryBusinessWrites': False}
    if args.offline:
        result['status'] = 'OFFLINE_SOURCE_PREFLIGHT_PASS_NO_RUNTIME_OR_SQL'
    else:
        live = operations().get('binarySha256')
        if live != args.expected_binary_sha:
            raise RuntimeError('Live binary differs; no SQL read or update submitted')
        names = ','.join(literal(table) for table in SPECS)
        available = dict(line.split('\t') for line in sql('SELECT TABLE_NAME,ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND TABLE_NAME IN (' + names + ');').splitlines())
        if set(available) != set(SPECS) or any(engine != 'InnoDB' for engine in available.values()):
            raise RuntimeError('Required allocated-project InnoDB schema missing; tool never creates/updates schemas')
        before = snapshot()
        issues = blockers(before, selected, not args.disable)
        result.update(before=before, blockers=issues, status='BLOCKED_PREFLIGHT_NO_CHANGE' if issues else 'READY_READ_ONLY')
        if writing and not issues:
            args.record.parent.mkdir(parents=True, exist_ok=True)
            fd = os.open(args.record, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, 'w') as report, open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                result['status'] = 'PENDING_GUARDED_POLICY_UPDATE'
                report.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n'); report.flush(); os.fsync(report.fileno())
                try:
                    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    if operations().get('binarySha256') != live:
                        raise RuntimeError('Live binary changed before policy update')
                    reply = sql(guarded_query(before, selected, args.enable))
                    if 'HYJS_AWAKE_READY=1' not in reply.splitlines():
                        result['status'] = 'LOCKED_PRECONDITIONS_CHANGED_NO_CHANGE'
                    else:
                        result['applied'] = True
                        result['after'] = snapshot()
                        result['onlySelectedFlagsChanged'] = after_check(before, result['after'], args.enable)
                        result['runtimeSHAUnchanged'] = operations().get('binarySha256') == live
                        result['status'] = 'APPLIED' if result['onlySelectedFlagsChanged'] and result['runtimeSHAUnchanged'] else 'APPLIED_CONCURRENT_CHANGE_REQUIRES_REVIEW'
                except BlockingIOError:
                    result['status'] = 'SQL_WINDOW_BUSY_NO_CHANGE'
                except Exception:
                    result['status'] = 'ACTION_FAILED_REVIEW_BEFORE_RETRY'
                    raise
                finally:
                    report.seek(0); report.truncate(); report.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n'); report.flush(); os.fsync(report.fileno())
            args.record = None
    if args.record:
        args.record.parent.mkdir(parents=True, exist_ok=True)
        with args.record.open('x') as report:
            report.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False))
    if result['status'].startswith(('BLOCKED', 'LOCKED', 'SQL_WINDOW', 'APPLIED_CONCURRENT')):
        raise SystemExit(2)


if __name__ == '__main__':
    main()
