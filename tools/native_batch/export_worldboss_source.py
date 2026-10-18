"""Offline type18 sources for the existing private NativeBatchRunner.

Actual mode preserves archived SCWorldBossFight bytes and optional matching
server FrozenBattleInput. Diagnostic mode uses the existing actual paid P5
actor export unchanged plus the four original WorldBoss table rows. Neither
mode opens a database/device, runs a battle, selects a winner, or grants proof.
"""
import argparse
import csv
import hashlib
import json
import math
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from server.codec import message

SCOPE = 'original-worldboss-automatic-ai'
STATS = ('Attack', 'PhyDefend', 'MagicDefend', 'Hp', 'hit', 'dodge', 'cri',
         'antiCri', 'criFactor', 'damageAdd', 'damageReduce')
PROP_KEYS = {'Attack': 'att', 'PhyDefend': 'pdef', 'MagicDefend': 'mdef', 'Hp': 'hp'}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def rows(root, name):
    path = root / 'analysis/tables' / ('Table' + name + '.tsv')
    with path.open(newline='') as stream:
        data = list(csv.reader(stream, delimiter='\t'))
    return [dict(zip(data[1], row)) for row in data[5:] if row and row[0]]


def proto_json(packet):
    # Standard ProtoJSON quotes int64; Go encoding/json FrozenBattleInput uses
    # numeric int64. Preserve original field names and presence without that conversion.
    result = {}
    for field, value in packet.ListFields():
        convert = proto_json if field.message_type else lambda item: item
        result[field.name] = [convert(item) for item in value] if field.is_repeated else convert(value)
    return result


def original_boss(root, level):
    definitions = rows(root, 'WorldBoss')
    factors = rows(root, 'WorldBossFactor')
    if len(definitions) != 4 or {row['Location'] for row in definitions} != {'13', '7', '9', '3'}:
        raise ValueError('Original complete four-member world boss changed')
    monster = message('BattleMonster', inst={'anger': 0})
    for source in definitions:
        matches = [row for row in factors if row['BossId'] == source['BossId']
                   and row['Level'] == str(level) and row['Location'] == source['Location']]
        if len(matches) != 1:
            raise ValueError('Original level/location factor must be unique')
        factor = matches[0]
        prop = {PROP_KEYS.get(key, key): float(source[key]) * float(factor[key]) for key in STATS}
        if any(not math.isfinite(value) or value < 0 for value in prop.values()):
            raise ValueError('Original panel/factor nonfinite or negative')
        actor = message('BattleHero', id=source['HeroId'], guid='worldboss-' + source['Id'],
                        pose=int(source['Location']), level=level, prop=prop,
                        inst={'curHp': prop['hp']}, bloodNumber=int(source['bloodNumber'] or 1),
                        skillOrder=','.join(source.get(key, '') for key in ('SkillId1', 'SkillId2', 'UltSkillId')),
                        SkillCastChance=float(source['SkillCastChance']) if source['SkillCastChance'] else 100,
                        talentList=[source['Talent' + str(index)] for index in range(6)
                                    if source.get('Talent' + str(index))])
        monster.formation.add().CopyFrom(actor)
    return monster


def validate_full_boss(root, typed, level=1):
    if (not typed.fightId or not typed.HasField('randSeed') or not typed.HasField('battlePlayer')
            or not typed.battlePlayer.HasField('inst') or not typed.battlePlayer.formation
            or len(typed.battleMonsterList) != 1 or not typed.battleMonsterList[0].HasField('inst')
            or not typed.fightScene or typed.maxround != 40):
        raise ValueError('Require exact current P12 type18 full first-group input and nonempty scene')
    expected = original_boss(root, level)
    if typed.battleMonsterList[0] != expected:
        raise ValueError('Four original members, HP, talents, skills, poses or visual blood count differ')
    for actor in typed.battlePlayer.formation:
        if not actor.HasField('prop') or not actor.HasField('inst') or actor.inst.curHp < 0:
            raise ValueError('Complete original player instances required')
    return expected


def validate_frozen(typed, path):
    data = json.loads(path.read_text())
    if (data.get('battleType') != 18 or data.get('maxRound') != 40 or data.get('seed') != typed.randSeed
            or data.get('defender') or data.get('defenderFirst') or data.get('isAttToDef')
            or data.get('attackerDamageFactor') != 1 or data.get('defenderDamageFactor') != 1
            or data.get('pveCompatibilityRule') or data.get('diagnosticAIDriverPolicy')
            or data.get('diagnosticManualAutoStartRoundTime') is not None
            or message('BattlePlayer', **data['attacker']) != typed.battlePlayer
            or message('BattleMonster', **data['monster']) != typed.battleMonsterList[0]
            or not data.get('configurationHash') or not data.get('tables')):
        raise ValueError('Archived server frozen input differs from exact original typed source')
    return data


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--callback', action='append', type=Path)
    mode.add_argument('--diagnostic-p5', action='store_true')
    mode.add_argument('--diagnostic-party', action='store_true',
                      help='Unchanged historical actual001003/equip037/Pet002 source already used by P7')
    parser.add_argument('--provenance', type=Path)
    parser.add_argument('--frozen-input', action='append', type=Path)
    parser.add_argument('--source-root', type=Path, default=ROOT)
    parser.add_argument('--seed', action='append', type=int)
    parser.add_argument('--level', type=int, default=1)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    source_root, output = args.source_root.resolve(), args.output.resolve()
    if not output.is_relative_to(ROOT / 'analysis/p6') or output.exists():
        raise ValueError('Select a new owned analysis/p6 output directory; preserve existing evidence')
    if args.level < 1 or args.level > 50:
        raise ValueError('Original boss level must be 1..50')
    metadata = {'scope': SCOPE, 'diagnosticOnly': True, 'nativeCompared': False,
                'authoritative': False, 'engineExecuted': False,
                'originalDLLSHA256': sha((source_root / 'analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll').read_bytes()),
                'originalTableSHA256': {name: sha((source_root / 'analysis/tables' / ('Table' + name + '.tsv')).read_bytes())
                                        for name in ('WorldBoss', 'WorldBossFactor', 'WorldBossPos', 'Skill', 'SkillCast', 'Talent', 'Buff')}}
    sources = []
    if args.diagnostic_p5:
        if args.provenance or args.frozen_input:
            raise ValueError('Finite P5 diagnostic cannot claim a server receipt/provenance')
        fixture_path = source_root / 'analysis/p6/p5-combined-all76-highest-actors-20261014.json'
        fixture = json.loads(fixture_path.read_text())
        if len(fixture['characterPaths']) != 76 or fixture['productionModified'] or fixture['dynamicCombatCertified']:
            raise ValueError('Existing actual P5 export changed its scope')
        for name, digest in fixture['originalTableSha256'].items():
            if sha((source_root / 'analysis/tables' / ('Table' + name + '.tsv')).read_bytes()) != digest:
                raise ValueError('Original P5 table source changed: ' + name)
        actor_source = next(row for row in fixture['characterPaths'] if row['heroId'] == 'hero_047')
        if not actor_source['actualContinuouslyCombinedPaidGrowth']:
            raise ValueError('Use the existing actual continuously paid P5 actor, not a synthesized panel')
        actor = message('BattleHero', **actor_source['battleHero'])
        player = message('BattlePlayer', playerId=10001, serverId=1, level=actor.level,
                         nickName='离线原成长诊断', formation=[actor], inst={'anger': 0})
        monster = original_boss(source_root, args.level)
        position = next(row for row in rows(source_root, 'WorldBossPos') if row['id'] == '1')
        seeds = args.seed if args.seed is not None else [7, 314159]
        if len(set(seeds)) != len(seeds) or not 1 <= len(seeds) <= 4 or any(seed < 0 or seed > 2147483647 for seed in seeds):
            raise ValueError('Choose one to four independent prepublished original seeds, without result search')
        for index, seed in enumerate(seeds):
            typed = message('SCWorldBossFight', fightId='diagnostic-worldboss18-p5actual-' + str(index),
                            randSeed=seed, battlePlayer=player, battleMonsterList=[monster],
                            fightScene=position['scene'], maxround=40)
            sources.append((typed.SerializeToString(deterministic=True), typed, None))
        metadata.update(sourceKind='finite-actual-p5-original-table-diagnostic', actualBusinessStart=False,
                        actorSource=str(fixture_path), actorSourceSHA256=sha(fixture_path.read_bytes()),
                        exactActualP5ActorSHA256=sha(actor.SerializeToString(deterministic=True)),
                        finitePrerequisites='Unchanged hero_047 BattleHero from actual paid P5/cold export; '
                        'original four-member table and level factors; wrapper playerId10001/name/anger0 and '
                        'prepublished seeds are explicit diagnostic context. No actual account, purchase, '
                        'shared HP, damage, result, or reward is constructed or claimed.')
    elif args.diagnostic_party:
        if args.provenance or args.frozen_input:
            raise ValueError('Historical actual-party diagnostic cannot claim a current business receipt')
        if args.level != 1:
            raise ValueError('This first actual-party type18 batch retains the original Lv1 four-body source')
        fixture_path = source_root / 'analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b/000.frozen-input.json'
        fixture_raw = fixture_path.read_bytes()
        if sha(fixture_raw) != 'd0e70186dc665f5e894eaae0a30915cb8032beb2ab86e0950ebd3e0ada41fa94':
            raise ValueError('Historical actual P5 party source changed')
        fixture = json.loads(fixture_raw)
        profile_path = fixture_path.parent / '000.actual-profile-source.json'
        if sha(profile_path.read_bytes()) != '89c060a63da435f39a4d15137a0435efb77b68ab786747440f1b081805c0a88b':
            raise ValueError('Historical read-only actual-profile source changed')
        profiles = json.loads(profile_path.read_bytes())
        game = next(row['game'] for row in profiles if row['playerId'] == 4)
        if (not any(row.get('equipId') == 'equip_037' and row.get('level') == 7 for row in game['equipment']) or
                not any(thing.get('id') == 'pet_002' for slot in game['formation'] for thing in slot['things'])):
            raise ValueError('Historical equip037/Pet002 loadout no longer matches actual source')
        player = message('BattlePlayer', **fixture['attacker'])
        actor_hashes = [sha(actor.SerializeToString(deterministic=True)) for actor in player.formation]
        expected = ['bf72e8364dd340aeea87a59f3001f046679c2a51833c526e17b9bc4b10368ef7',
                    '01325a107d0e67d2bb97261af19c3e697ac13dacc1dd4edfabb4f1f7b28413be']
        if [actor.id for actor in player.formation] != ['hero_001', 'hero_003'] or actor_hashes != expected:
            raise ValueError('Original complete actual party actor bytes changed')
        # The original type18 battlePlayer producer has anger0 without the PvP
        # firstOrder field. Retain every actor and all other player source bytes.
        player.inst.ClearField('firstOrder')
        if player.inst.anger != 0:
            raise ValueError('Historical party is not the preserved AP0 source')
        monster = original_boss(source_root, args.level)
        position = next(row for row in rows(source_root, 'WorldBossPos') if row['id'] == '1')
        seeds = args.seed if args.seed is not None else [7, 314159]
        if seeds != [7, 314159]:
            raise ValueError('This bounded actual-party batch keeps the prepublished two original seeds')
        for index, seed in enumerate(seeds):
            typed = message('SCWorldBossFight', fightId='diagnostic-worldboss18-actualparty-' + str(index),
                            randSeed=seed, battlePlayer=player, battleMonsterList=[monster],
                            fightScene=position['scene'], maxround=40)
            sources.append((typed.SerializeToString(deterministic=True), typed, None))
        metadata.update(sourceKind='finite-historical-actual-p5-party001003-original-table-diagnostic',
                        actualBusinessStart=False, actorSource=str(fixture_path.relative_to(source_root)),
                        actorSourceSHA256=sha(fixture_raw), profileSource=str(profile_path.relative_to(source_root)),
                        profileSourceSHA256=sha(profile_path.read_bytes()),
                        sourceActorEngineVersion=fixture['engineVersion'],
                        sourceActorConfigurationHash=fixture['configurationHash'],
                        exactActualP5ActorSHA256List=actor_hashes,
                        finitePrerequisites='Unchanged historical read-only Demo4 full001003 actors, same d0e70186 '
                        'source used by P7; actual equip037 Lv7 and formation Pet002 are provenance for the '
                        'already projected flat P5 panels, not new acquisition or dynamic-pet certification. '
                        'Only wrapper firstOrder removal, fightId, original scene and prepublished seeds7/314159 '
                        'are new context. Full original Lv1 four-body boss, max40, factors1/1, both automatic. '
                        'No actor panel/skill/talent changes, no real Start, purchase, result or shared HP claim.')
    else:
        if args.seed or not args.provenance:
            raise ValueError('Actual callback mode requires provenance and never overrides its original seed')
        provenance = args.provenance.resolve()
        if not provenance.is_relative_to(ROOT / 'analysis') or not provenance.is_file():
            raise ValueError('Use archived producer provenance under project analysis')
        frozen = args.frozen_input or []
        if frozen and len(frozen) != len(args.callback):
            raise ValueError('Supply one exact frozen input per preserved callback, in the same order')
        for index, path in enumerate(args.callback):
            path = path.resolve()
            if not path.is_relative_to(ROOT / 'analysis') or not path.is_file():
                raise ValueError('Use a preserved actual source callback under analysis')
            raw = path.read_bytes()
            typed = message('SCWorldBossFight')
            typed.ParseFromString(raw)
            if frozen:
                validate_frozen(typed, frozen[index].resolve())
            sources.append((raw, typed, frozen[index].resolve() if frozen else None))
        metadata.update(sourceKind='archived-actual-business-start', actualBusinessStart=True,
                        provenance=str(provenance), provenanceSHA256=sha(provenance.read_bytes()),
                        sourceQualification='Producer provenance is retained; exporter only verifies bytes and projection')
    for _, typed, _ in sources:
        validate_full_boss(source_root, typed, args.level)
    output.mkdir(parents=True)
    cases = []
    for index, (raw, typed, frozen) in enumerate(sources):
        stem = '%03d' % index
        typed_name, wire_name = stem + '.SCWorldBossFight.pb', stem + '.pb'
        wire = message('SCFightPlayback', fightid=typed.fightId, randSeed=typed.randSeed,
                       battleType=18, maxRound=40, attackerFactor=1, defenderFactor=1,
                       attacker=typed.battlePlayer, defender2=typed.battleMonsterList[0])
        wire_raw = wire.SerializeToString(deterministic=True)
        (output / typed_name).write_bytes(raw)
        (output / wire_name).write_bytes(wire_raw)
        projection = {'attacker': proto_json(typed.battlePlayer),
                      'monster': proto_json(typed.battleMonsterList[0]),
                      'seed': typed.randSeed, 'maxRound': 40, 'battleType': 18,
                      'attackerDamageFactor': 1, 'defenderDamageFactor': 1,
                      'defenderFirst': False, 'isAttToDef': False}
        projection_raw = (json.dumps(projection, ensure_ascii=False, indent=2) + '\n').encode()
        projection_name = stem + '.input-projection.json'
        (output / projection_name).write_bytes(projection_raw)
        case = {'case': typed.fightId, 'battleType': 18, 'fightId': typed.fightId, 'seed': typed.randSeed,
                'nativeScope': SCOPE, 'diagnosticOnly': True, 'status': 'awaiting-original-automatic-ai',
                'wire': wire_name, 'wireSHA256': sha(wire_raw),
                'typedCallback': typed_name, 'typedCallbackSHA256': sha(raw),
                'inputProjection': projection_name, 'inputProjectionSHA256': sha(projection_raw),
                'originalSceneBranch': 'nonempty fightScene -> original map_arena_001',
                'sourceResultMetadataIgnored': True, 'fullBossMembers': 4, 'bossLevel': args.level,
                'damageFactors': {'attacker': 1, 'defender': 1},
                'sourceActorSHA256': [sha(actor.SerializeToString(deterministic=True))
                                      for actor in typed.battlePlayer.formation],
                'sourceBossMemberSHA256': [sha(actor.SerializeToString(deterministic=True))
                                           for actor in typed.battleMonsterList[0].formation]}
        if frozen:
            frozen_name = stem + '.frozen-input.json'
            frozen_raw = frozen.read_bytes()
            (output / frozen_name).write_bytes(frozen_raw)
            case.update(frozenInput=frozen_name, frozenInputSHA256=sha(frozen_raw))
        cases.append(case)
    metadata['cases'] = cases
    manifest = output / 'manifest.json'
    manifest.write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'manifest': str(manifest), 'sha256': sha(manifest.read_bytes()),
                      'sourceKind': metadata['sourceKind'], 'cases': len(cases), 'engineExecuted': False}))


if __name__ == '__main__':
    main()
