"""Validate isolated real guard producer exports and stage unchanged UF14 inputs.

Reads evidence only. Does not connect to DB/device, run Go, modify actors, apply
aftermath or grant authority. Run the explicitly enabled Go export test first.
"""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from google.protobuf import json_format
from server.codec import message


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def pb(kind, raw):
    value = message(kind)
    value.ParseFromString(raw)
    return value


def require(condition, explanation):
    if not condition:
        raise RuntimeError(explanation)


def actor_summary(fighter):
    return {'anger': fighter.inst.anger,
            'actors': [{'guid': a.guid, 'heroID': a.id, 'pose': a.pose,
                        'bloodNumber': a.bloodNumber, 'hp': a.inst.curHp,
                        'maxHP': a.prop.hp,
                        'sha256': sha(a.SerializeToString(deterministic=True))}
                       for a in fighter.formation]}


def verify_case(row, source):
    prefix = row['prefix']
    files = row['files']
    data = {}
    for name, digest in files.items():
        path = source.parent / name
        require(path.parent == source.parent and path.name == name,
                'Evidence filenames must remain within the source directory')
        data[name] = path.read_bytes()
        require(sha(data[name]) == digest, 'Source evidence SHA mismatch: ' + name)
    read = lambda suffix: data[prefix + '.' + suffix]
    frozen_raw = read('frozen-input.json')
    frozen = json.loads(frozen_raw)
    require(sha(frozen_raw) == row['p6InputHash'] == row['attempt']['input_hash'],
            'Canonical P6 input SHA differs from the actual audit attempt')
    typed = pb('SCUFPveFightRecordReplay', read('SCUFPveFightRecordReplay.pb'))
    wire = pb('SCFightPlayback', read('SCFightPlayback.pb'))
    guard = pb('InstUFGuard', read('guard-frame.pb'))
    player = pb('InstUFPlayer', read('player-frame.pb'))
    attacker = json_format.ParseDict(frozen['attacker'], message('BattlePlayer'))
    monster = json_format.ParseDict(frozen['monster'], message('BattleMonster'))
    require(not frozen.get('defender') and frozen['battleType'] == 14
            and not frozen.get('diagnosticAIDriverPolicy')
            and not frozen.get('pveCompatibilityRule')
            and not frozen.get('diagnosticManualAutoStartRoundTime')
            and not frozen.get('isAttToDef') and not frozen['defenderFirst']
            and frozen['maxRound'] == 40,
            'Only actual original automatic Monster UF14 inputs are eligible')
    require(typed.player == attacker == wire.attacker
            and typed.monster == monster == wire.defender2
            and typed.fightId == wire.fightid == str(row['attempt']['guid'])
            and typed.randomSeed == wire.randSeed == frozen['seed']
            and typed.maxRound == wire.maxRound == frozen['maxRound']
            and typed.HasField('attackerDamageFactor')
            and typed.HasField('defenderDamageFactor')
            and typed.attackerDamageFactor == wire.attackerFactor == frozen['attackerDamageFactor'] == 1
            and typed.defenderDamageFactor == wire.defenderFactor == frozen['defenderDamageFactor'] == 1
            and wire.battleType == 14 and not wire.HasField('defender1')
            and not typed.HasField('isPlayerWin') and not wire.HasField('optRecord')
            and not wire.winConditions,
            'Typed/wire/frozen actor, seed, factor or result-hint mismatch')
    require(guard.guid == monster.monsterId == row['guardGUID']
            and guard.UFGuardId == row['guardID']
            and guard.bindGuid == row['quote']['bind_guid']
            and guard.camp == row['quote']['camp']
            and player.guid == attacker.playerId == row['playerID']
            and len(monster.formation) == 1
            and monster.formation[0].id == row['heroID'] == row['quote']['hero_id']
            and row['quote']['source_hash'] == row['attempt']['offer_hash']
            and row['commonQuote']['source_hash'] == row['quote']['battle_offer_hash'],
            'Actual producer identity/template/offer links differ')
    require(sha(read('field-snapshot.json')) == row['fieldSnapshotHash']
            and sha(read('player-frame.pb')) == row['playerFrameHash'],
            'Actual frozen field/player frame identity differs')
    require(list(guard.fightInfo.index) == [0]
            and list(guard.fightInfo.hpPos) == [monster.formation[0].inst.curHp]
            and list(guard.fightInfo.maxHpPos) == [monster.formation[0].prop.hp]
            and guard.fightInfo.anger == monster.inst.anger,
            'Guard field-index0 -> template pose HP/AP projection differs')
    positions = {pose: index for index, pose in enumerate(player.fightInfo.index)}
    require(len(positions) == len(attacker.formation)
            and player.fightInfo.anger == attacker.inst.anger,
            'Actual player pose/AP source differs')
    for actor in attacker.formation:
        index = positions.get(actor.pose)
        require(index is not None and player.fightInfo.hpPos[index] == actor.inst.curHp
                and player.fightInfo.maxHpPos[index] == actor.prop.hp,
                'Actual player pose HP projection differs')
    require(row['authoritative'] is False and row['warApplied'] is False,
            'Diagnostic producer export cannot import authority')
    return wire, data


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True, help='Go source-export.json')
    parser.add_argument('--output', type=Path, required=True, help='new analysis/p6/guild-guard-inputs-* directory')
    parser.add_argument('--source-snapshot', action='store_true', help='verify the exact producer source-snapshot beside this export')
    args = parser.parse_args()
    source, output = args.source.resolve(), args.output.resolve()
    require(source.parent.parent == ROOT / 'analysis/p6'
            and source.name == 'source-export.json', 'Use an isolated in-repository producer export')
    require(output.parent == ROOT / 'analysis/p6'
            and output.name.startswith('guild-guard-inputs-') and not output.exists(),
            'Choose a new owned analysis/p6/guild-guard-inputs-* output')
    raw = source.read_bytes()
    exported = json.loads(raw)
    require(exported['schema'] == 'guild-guard-producer-export-v1'
            and exported['authoritative'] is False and exported['warApplied'] is False,
            'Explicit non-authoritative producer export required')
    source_root = source.parent / 'source-snapshot' if args.source_snapshot else ROOT
    for relative, digest in exported['sourcePins'].items():
        path = (source_root / relative).resolve()
        require(path.is_relative_to(source_root) and sha(path.read_bytes()) == digest,
                'Producer source pin drift: ' + relative)
    require(len(set(exported['allOriginalGuardIDs'])) == len(exported['allOriginalGuardIDs'])
            and {r['guardID'] for r in exported['cases']} == set(exported['allOriginalGuardIDs']),
            'Per-ID original guard coverage ledger is incomplete')
    verified, blocked = [], []
    for row in exported['cases']:
        if row['status'] == 'producer-blocked':
            blocked.append(row)
            continue
        wire, data = verify_case(row, source)
        verified.append((row, wire, data))
    require(verified, 'All current producer cases are blocked; preserve source-export.json and diagnose the first error')
    output.mkdir(mode=0o700)
    shutil.copyfile(source, output / source.name)
    cases = []
    for index, (row, wire, data) in enumerate(verified):
        for name, content in data.items():
            (output / name).write_bytes(content)
        numbered = '%03d.pb' % index
        wire_raw = data[row['prefix'] + '.SCFightPlayback.pb']
        (output / numbered).write_bytes(wire_raw)
        # prepare.py's existing typedCallback fallback routes type14 to the
        # Rush loader. Retain the verified UF callback as a separate sidecar;
        # its existing pve-auto wire route reconstructs the UF14 caller shape.
        callback = row['prefix'] + '.SCUFPveFightRecordReplay.pb'
        cases.append({'case': row['guardID'], 'guardHeroID': row['heroID'],
                      'fightId': wire.fightid, 'battleType': 14, 'seed': wire.randSeed,
                      'wire': numbered, 'wireSHA256': sha(wire_raw),
                      'originalCallback': 'SCUFPveFightRecordReplay',
                      'originalCallbackFile': callback, 'originalCallbackSHA256': sha(data[callback]),
                      'frozenInput': row['prefix'] + '.frozen-input.json',
                      'p6InputHash': row['p6InputHash'],
                      'guardFrameSHA256': sha(data[row['prefix'] + '.guard-frame.pb']),
                      'fieldSnapshotHash': row['fieldSnapshotHash'],
                      'playerFrameSHA256': row['playerFrameHash'],
                      'offerHash': row['quote']['source_hash'],
                      'commonOfferHash': row['commonQuote']['source_hash'],
                      'attacker': actor_summary(wire.attacker), 'monster': actor_summary(wire.defender2),
                      'nativeScope': 'original-pve-automatic-ai',
                      'originalWrapperScope': 'original-uf14-playerai-monster',
                      'status': 'awaiting-original-automatic-ai',
                      'authoritative': False, 'warApplied': False, 'diagnosticOnly': True})
    manifest = {'schema': 'guild-guard-native-input-v1', 'cases': cases,
                'blockedProducerCases': blocked, 'allOriginalGuardIDs': exported['allOriginalGuardIDs'],
                'sourceExportSHA256': sha(raw), 'sourcePins': exported['sourcePins'],
                'exporterSHA256': sha(Path(__file__).read_bytes()),
                'nativeCompared': False, 'authoritative': False, 'warApplied': False,
                'sourceFixture': exported['fixture'], 'sourceStoreDialect': exported['storeDialect'],
                'scope': 'Actual current isolated guard producer factors1/1; unchanged seed/HP/AP/skills/GUIDs. '
                         'Local prepublished guard template, not original private-server scaling. '
                         'No synthetic injury, supplied winner or optRecord. Original 547 UF14 PlayerAI required.',
                'prospectiveAftermath': 'Go diagnostic terminal sidecars only; no continuation is claimed as actual war damage.'}
    encoded = (json.dumps(manifest, indent=2, ensure_ascii=False) + '\n').encode()
    (output / 'manifest.json').write_bytes(encoded)
    print(json.dumps({'manifest': str(output / 'manifest.json'), 'sha256': sha(encoded),
                      'cases': len(cases), 'blocked': len(blocked)}))


if __name__ == '__main__':
    main()
