"""Read-only managed war evidence and explicit prospective wounded14 inputs.

No DB manager, device, wallet or battle compute API is called here. The caller
provides its already-owned SQL read callback. Actor HP/AP comes only from the
real persisted war frame; original entry BattlePB remains saved unchanged.
"""
import argparse
import base64
import copy
import hashlib
import json
from pathlib import Path
import re
import sys
import zlib

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from server.codec import message

ATTEMPT_FIELDS = [
    'id', 'match_id', 'player_id', 'target_id', 'stamp', 'client_stamp', 'status',
    'fight_id', 'request_hash', 'offer_hash', 'p6_record_id', 'input_hash',
    'start_pb', 'start_hash', 'applied_properties', 'before', 'after',
    'terminal', 'terminal_hash', 'scope_proof', 'dead_player_id',
    'reward_player_id', 'revive_seconds', 'created_at', 'due_at', 'settled_at',
    'source_hash',
]
BLOBS = {'start_pb', 'applied_properties', 'before', 'after', 'terminal'}
MAX_INPUT = 64 * 1024 * 1024
EXPECTED_FULLHP_PROOF = "dd937dec43673be9d883f405491589fd4fcb1f8891ba09861b96bee34519c980"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def dump(value):
    return json.dumps(value, ensure_ascii=False, separators=(',', ':'), allow_nan=False).encode()


def source_snapshot(sql, attempt_id, player_ids):
    """One consistent SELECT excludes profile/session/signing credentials."""
    if not re.fullmatch('[a-f0-9]{64}', attempt_id) or not player_ids or len(set(player_ids)) != len(player_ids) or any(not isinstance(value, int) or value <= 0 for value in player_ids):
        raise ValueError('Use exact real attempt and unique participant IDs')
    fields = ','.join("'" + name + "'," + ('HEX(a.`' + name + '`)' if name in BLOBS else 'a.`' + name + '`') for name in ATTEMPT_FIELDS)
    selected = ','.join(map(str, player_ids))
    query = "SELECT JSON_OBJECT('attempt',JSON_OBJECT(" + fields + "),'p6',JSON_OBJECT('id',r.id,'source',r.source,'sourceFight',r.source_fight,'status',r.status,'seed',r.seed,'input',HEX(r.input),'inputHash',r.input_hash,'operates',HEX(r.operates),'outcome',HEX(r.outcome),'outcomeHash',r.outcome_hash,'replayHash',r.replay_hash,'serverResult',r.server_result,'authoritative',r.authoritative,'engineVersion',r.engine_version,'clientVersion',r.client_version,'configurationHash',r.configuration_hash),'offer',JSON_OBJECT('snapshot',o.snapshot,'sourceHash',o.source_hash,'capturedAt',o.captured_at),'participants',(SELECT JSON_ARRAYAGG(JSON_OBJECT('playerId',f.player_id,'guildId',f.guild_id,'camp',f.camp,'battle',HEX(f.battle),'battleHash',f.battle_hash,'frame',HEX(f.frame),'frameHash',f.frame_hash,'source',f.source,'joinedAt',f.joined_at,'updatedAt',f.updated_at,'leftAt',f.left_at,'attemptCount',(SELECT COUNT(*) FROM p13_union_fight_player_battle_attempts x WHERE x.match_id=f.match_id AND (x.player_id=f.player_id OR x.target_id=f.player_id)))) FROM p13_union_fight_field_players f WHERE f.match_id=a.match_id AND f.player_id IN (" + selected + "))) FROM p13_union_fight_player_battle_attempts a JOIN p6_battle_records r ON r.id=a.p6_record_id JOIN p13_union_fight_player_battle_offers o ON o.match_id=a.match_id WHERE a.id='" + attempt_id + "' AND a.status='settled';"
    raw = sql(query).strip().encode()
    if not raw or len(raw) > MAX_INPUT:
        raise ValueError('Missing or oversized real aftermath capture')
    body = json.loads(raw)
    if {row['playerId'] for row in body['participants']} != set(player_ids):
        raise ValueError('Actual field participant missing from atomic snapshot')
    return body


def unpack_input(stored):
    if len(stored) > MAX_INPUT:
        raise ValueError('Stored P6 input too large')
    if stored[:2] != b'\x1f\x8b':
        return stored
    reader = zlib.decompressobj(31)
    raw = reader.decompress(stored, MAX_INPUT + 1)
    if len(raw) > MAX_INPUT or reader.unconsumed_tail or not reader.eof or reader.unused_data:
        raise ValueError('Require one bounded complete original gzip member')
    return raw


def checked(body):
    """Bind real journal, initial P6, original protobufs and actual aftermath."""
    row, record = body['attempt'], body['p6']
    canonical = {}
    for name in ATTEMPT_FIELDS:
        value = row[name]
        canonical[name] = base64.b64encode(bytes.fromhex(value)).decode() if name in BLOBS and value is not None else value
    canonical['source_hash'] = ''
    if sha(dump(canonical)) != row['source_hash'] or row['status'] != 'settled':
        raise ValueError('Actual journal canonical SHA mismatch')
    raw_input = unpack_input(bytes.fromhex(record['input']))
    if sha(raw_input) != record['inputHash'] or record['inputHash'] != row['input_hash']:
        raise ValueError('P6 canonical input SHA mismatch')
    frozen = json.loads(raw_input)
    if frozen.get('battleType') != 14 or frozen.get('maxRound') != 40 or frozen.get('attackerDamageFactor') != 1 or frozen.get('defenderDamageFactor') != 1 or frozen.get('defenderFirst') or frozen.get('isAttToDef'):
        raise ValueError('Require the independent fullHP ordinary14 source shape')
    terminal_raw = bytes.fromhex(row['terminal'])
    terminal = json.loads(terminal_raw)
    if sha(terminal_raw) != row['terminal_hash'] or sha(bytes.fromhex(row['start_pb'])) != row['start_hash']:
        raise ValueError('Actual terminal or original typed-start SHA mismatch')
    typed = message('SCUFFightRecordReplay')
    typed.ParseFromString(bytes.fromhex(row['start_pb']))
    if not typed.IsInitialized() or typed.HasField('winPlayerId') or typed.fightId != row['fight_id'] or typed.maxRound != 40 or typed.roomId != 'map_forest_002' or len(typed.playerList) != 2 or not typed.HasField('attackerDamageFactor') or not typed.HasField('defenderDamageFactor') or typed.attackerDamageFactor != 1 or typed.defenderDamageFactor != 1 or typed.randomSeed != record['seed'] or frozen['seed'] != record['seed']:
        raise ValueError('Original typed start does not match the real accepted input')
    for player, side, player_id, order in zip(typed.playerList, ('attacker', 'defender'), (row['player_id'], row['target_id']), (1, 0)):
        if player.playerId != player_id or not player.HasField('inst') or player.inst.firstOrder != order or go_json(player) != frozen[side]:
            raise ValueError('Original actor, first order or complete frozen projection changed')
    if record['id'] != row['p6_record_id'] or record['source'] != 'p13-uf-player14-factor1' or record['sourceFight'] != row['fight_id'] or record['status'] != 'computed' or record['authoritative'] != 1:
        raise ValueError('Require a real accepted scoped ordinary14 producer')
    if record['serverResult'] not in (0, 1) or sha(bytes.fromhex(record['operates']) + bytes([record['serverResult']])) != record['replayHash']:
        raise ValueError('P6 complete operation/result hash mismatch')
    outcome_raw = bytes.fromhex(record['outcome'])
    if sha(outcome_raw) != record['outcomeHash'] or json.loads(outcome_raw) != terminal['outcome'] or terminal['outcome']['capabilityHash'] != row['scope_proof'] or row['scope_proof'] != EXPECTED_FULLHP_PROOF:
        raise ValueError('Actual same-terminal capability/outcome mismatch')
    for key in ('engineVersion', 'clientVersion', 'configurationHash'):
        if frozen[key] != record[key] or terminal[key] != record[key]:
            raise ValueError('Frozen source version mismatch: ' + key)
    offer = body['offer']
    if sha(offer['snapshot'].encode()) != offer['sourceHash'] or offer['sourceHash'] != row['offer_hash']:
        raise ValueError('Pre-entry published rule SHA mismatch')
    before, after = json.loads(bytes.fromhex(row['before'])), json.loads(bytes.fromhex(row['after']))
    if {entry['player_id'] for entry in before} != {row['player_id'], row['target_id']} or {entry['guid'] for entry in after} != {row['player_id'], row['target_id']}:
        raise ValueError('Actual battle participant identity mismatch')
    if len({entry['playerId'] for entry in body['participants']}) != len(body['participants']):
        raise ValueError('Duplicate current participant identity')
    for participant in body['participants']:
        for key in ('battle', 'frame'):
            if sha(bytes.fromhex(participant[key])) != participant[key + 'Hash']:
                raise ValueError('Actual field protobuf SHA mismatch')
        battle, frame = message('BattlePlayer'), message('InstUFPlayer')
        battle.ParseFromString(bytes.fromhex(participant['battle']))
        frame.ParseFromString(bytes.fromhex(participant['frame']))
        if battle.playerId != participant['playerId'] or frame.guid != participant['playerId'] or frame.camp != participant['camp']:
            raise ValueError('Actual current frame and original Battle identity mismatch')
    return frozen, terminal, raw_input, before


def export_evidence(body, output, proof_path, observed_frames=()):
    output = Path(output).resolve()
    if not output.is_relative_to(ROOT / 'analysis/p13') or output.exists():
        raise ValueError('Use a new immutable p13 actual-source directory')
    frozen, terminal, raw_input, before = checked(body)
    row, record = body['attempt'], body['p6']
    assets = {'actual-snapshot.json': dump(body) + b'\n', 'actual.frozen-input.json': raw_input,
              'actual.SCUFFightRecordReplay.pb': bytes.fromhex(row['start_pb']),
              'actual.before.json': bytes.fromhex(row['before']), 'actual.aftermath.json': bytes.fromhex(row['after']),
              'actual.terminal.json': bytes.fromhex(row['terminal']), 'actual.outcome.json': bytes.fromhex(record['outcome']),
              'actual.diagnostic-operations.pb': bytes.fromhex(record['operates']),
              'actual.p6-stored-input.bin': bytes.fromhex(record['input']),
              'actual.applied-properties.json': bytes.fromhex(row['applied_properties']),
              'actual.rule.json': body['offer']['snapshot'].encode(), 'fixed-source-proof.json': Path(proof_path).read_bytes()}
    for participant in body['participants']:
        name = 'player-' + str(participant['playerId'])
        assets[name + '.entry-BattlePlayer.pb'] = bytes.fromhex(participant['battle'])
        assets[name + '.current-InstUFPlayer.pb'] = bytes.fromhex(participant['frame'])
    for entry in before:
        assets['player-' + str(entry['player_id']) + '.before-InstUFPlayer.pb'] = base64.b64decode(entry['frame'], validate=True)
    for index, frame in enumerate(observed_frames):
        assets['observed-%02d.%s.pb' % (index, frame.DESCRIPTOR.name)] = frame.SerializeToString(deterministic=True)
    output.mkdir(parents=True)
    for name, raw in assets.items():
        (output / name).write_bytes(raw)
    manifest = {'status': 'ACTUAL_ACCEPTED_172_SOURCE_EXPORT_ONLY', 'scope': 'Managed finite accounts/calendar, true declarations/current movement/originalSocket0. Persisted first terminal is scoped fullHP192 only. Current aftermath is a separate prospective partial source; no partial authorization or second accepted outcome.', 'case': row['id'], 'matchId': row['match_id'], 'p6RecordId': record['id'], 'sourceHash': row['source_hash'], 'sourceProofSHA256': sha(Path(proof_path).read_bytes()), 'engineVersion': frozen['engineVersion'], 'clientVersion': frozen['clientVersion'], 'configurationHash': frozen['configurationHash'], 'files': [{'path': name, 'sha256': sha(raw), 'bytes': len(raw)} for name, raw in sorted(assets.items())], 'protobufWireBoundary': 'DB original-start exact stored bytes; observed Begin/End/Detail/Dead are canonical parsed protobuf bytes, not socket-header wireSHA.', 'partialAuthoritative': False, 'globalAuthoritative': False, 'globalNativeCompared': False}
    (output / 'manifest.json').write_bytes(dump(manifest) + b'\n')
    return manifest


def go_json(msg):
    if not msg.IsInitialized():
        raise ValueError('Original protobuf required fields are missing')
    result = {}
    for field, value in msg.ListFields():
        def scalar(item):
            if field.type == field.TYPE_MESSAGE:
                return go_json(item)
            if field.type == field.TYPE_BYTES:
                return base64.b64encode(item).decode()
            if field.type == field.TYPE_ENUM:
                return int(item)
            return item
        result[field.json_name] = [scalar(item) for item in value] if field.is_repeated else scalar(value)
    return result


def project_current(participant, first):
    battle, frame = message('BattlePlayer'), message('InstUFPlayer')
    battle.ParseFromString(bytes.fromhex(participant['battle']))
    frame.ParseFromString(bytes.fromhex(participant['frame']))
    info = frame.fightInfo
    if battle.playerId != participant['playerId'] or frame.guid != participant['playerId'] or frame.camp != participant['camp']:
        raise ValueError('Original current participant identity mismatch')
    if not battle.IsInitialized() or not frame.IsInitialized() or not battle.formation or not battle.HasField('inst') or not frame.HasField('fightInfo') or frame.state not in (0, 5) or participant['leftAt'] != 0 or len(info.index) != len(battle.formation) or len(info.index) != len(set(info.index)) or len(info.hpPos) != len(info.index) or len(info.maxHpPos) != len(info.index):
        raise ValueError('Prospective partial source requires a real live idle participant')
    slots = {pose: index for index, pose in enumerate(info.index)}
    for hero in battle.formation:
        index = slots.get(hero.pose)
        if index is None or not hero.HasField('prop') or not hero.HasField('inst') or not 0 < info.hpPos[index] <= info.maxHpPos[index] or info.maxHpPos[index] != hero.prop.hp:
            raise ValueError('Do not synthesize missing/zero HP or unknown runtime maxHP')
        hero.inst.curHp = info.hpPos[index]
    battle.inst.anger = info.anger
    battle.inst.firstOrder = first
    return battle


def prepare_partial(source, output):
    """Use a real injured survivor and real never-fought enemy in this war."""
    source, output = Path(source).resolve(), Path(output).resolve()
    if not source.is_relative_to(ROOT / 'analysis/p13') or not output.is_relative_to(ROOT / 'analysis/p6') or output.exists():
        raise ValueError('Use verified p13 source and new p6 diagnostic directory')
    manifest_raw = (source / 'manifest.json').read_bytes()
    manifest = json.loads(manifest_raw)
    for entry in manifest['files']:
        asset = source / entry['path']
        if asset.resolve().parent != source or not asset.is_file():
            raise ValueError('Only same-directory actual-source assets are allowed')
        if sha(asset.read_bytes()) != entry['sha256']:
            raise ValueError('Actual source asset SHA changed')
    body = json.loads((source / 'actual-snapshot.json').read_bytes())
    frozen, terminal, _, _ = checked(body)
    survivor_id = body['attempt']['reward_player_id']
    if survivor_id <= 0:
        raise ValueError('No actual defeated-player survivor available; inspect timeout sources separately')
    survivor = next(row for row in body['participants'] if row['playerId'] == survivor_id)
    current = message('InstUFPlayer'); current.ParseFromString(bytes.fromhex(survivor['frame']))
    if not any(0 < hp < maximum for hp, maximum in zip(current.fightInfo.hpPos, current.fightInfo.maxHpPos)):
        raise ValueError('Actual survivor was not injured; never write HP to create this branch')
    fresh = [row for row in body['participants'] if row['camp'] != survivor['camp'] and row['guildId'] != survivor['guildId'] and row['attemptCount'] == 0]
    if not fresh:
        raise ValueError('No real current-war never-fought enemy; do not restore the dead opponent')
    fresh.sort(key=lambda row: row['playerId'])
    opponent = fresh[0]
    full = message('InstUFPlayer'); full.ParseFromString(bytes.fromhex(opponent['frame']))
    if any(hp != maximum or hp <= 0 for hp, maximum in zip(full.fightInfo.hpPos, full.fightInfo.maxHpPos)):
        raise ValueError('The real fresh enemy is not full HP')
    # Validate both current projections before creating any output.
    projected = [project_current(row, first) for pair in ((survivor, opponent), (opponent, survivor)) for row, first in zip(pair, (1, 0))]
    output.mkdir(parents=True)
    cases = []
    for index, pair in enumerate(((survivor, opponent), (opponent, survivor))):
        players = projected[index * 2:index * 2 + 2]
        fight_id = 'diagnostic-actual-war-wounded14-' + body['attempt']['id'][:20] + '-' + str(index)
        seed = body['p6']['seed']
        typed = message('SCUFFightRecordReplay', fightId=fight_id, roomId='map_forest_002', randomSeed=seed, maxRound=40, attackerDamageFactor=1, defenderDamageFactor=1)
        for player in players:
            typed.playerList.add().CopyFrom(player)
        wire = message('SCFightPlayback', fightid=fight_id, randSeed=seed, battleType=14, maxRound=40, attackerFactor=1, defenderFactor=1)
        wire.attacker.CopyFrom(players[0]); wire.defender1.CopyFrom(players[1])
        prospective = copy.deepcopy(frozen)
        prospective['attacker'], prospective['defender'] = map(go_json, players)
        prospective['defenderFirst'], prospective['isAttToDef'] = False, False
        prospective['appliedProperties'] = {'source': ['Prospective diagnostic ordinary14 current-war actual survivor HP/AP plus never-fought enemy; no second accepted fight; fullHP192 proof not reused.'], 'actualWarSourceManifestSHA256': [sha(manifest_raw)], 'seedSource': ['Prior accepted actual crypto seed reused only for independent diagnostic; not a claimed second public seed.']}
        name = '%03d' % index
        assets = {name + '.pb': wire.SerializeToString(deterministic=True), name + '.SCUFFightRecordReplay.pb': typed.SerializeToString(deterministic=True), name + '.frozen-input.json': dump(prospective) + b'\n'}
        for file, raw in assets.items():
            (output / file).write_bytes(raw)
        cases.append({'case': 'actual-war-injured14-current-live/' + str(index), 'wire': name + '.pb', 'wireSHA256': sha(assets[name + '.pb']), 'battleType': 14, 'fightId': fight_id, 'seed': seed, 'nativeScope': 'original-pvp-automatic-ai', 'originalTypedProjection': name + '.SCUFFightRecordReplay.pb', 'originalTypedProjectionSHA256': sha(assets[name + '.SCUFFightRecordReplay.pb']), 'frozenInput': name + '.frozen-input.json', 'frozenInputSHA256': sha(assets[name + '.frozen-input.json']), 'originalFirstOrder': [1, 0], 'damageFactors': {'attacker': 1, 'defender': 1}, 'diagnosticOnly': True, 'partialAuthoritative': False, 'originalRoomFallback': 'map_forest_002', 'actorSHA256': [sha(player.SerializeToString(deterministic=True)) for player in players], 'sourceCurrentParticipants': [row['playerId'] for row in pair]})
    (output / 'manifest.json').write_bytes(dump({'cases': cases, 'sourceManifest': str(source / 'manifest.json'), 'sourceManifestSHA256': sha(manifest_raw), 'status': 'AWAITING_INDEPENDENT_ORIGINAL_PARTIAL14', 'scope': 'Same actual war/current live injured survivor and real never-fought full enemy; exact HP/AP/poses from accepted aftermath; source identity/skills/panel/GUID retained; explicit local1/0; prior real seed diagnostic reuse. No result hints, no second accepted battle or partial authority.', 'authoritative': False, 'nativeCompared': False}) + b'\n')
    return cases


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    print(json.dumps({'cases': len(prepare_partial(args.source, args.output)), 'deviceOperations': 0, 'SQLWrites': 0}))
