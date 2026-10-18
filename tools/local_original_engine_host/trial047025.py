"""Four finite typed9 probes from the real new-player actor snapshot.

No source capture is fabricated: outputs are local CoreCLR observations, and
production/native authority remains false until an independent phone anchor.
"""
import argparse
import copy
import csv
import hashlib
import json
from pathlib import Path
import shutil
import sys

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'analysis/p6/p7-newbie047025-local-inputs-r2-20261017/manifest.json'
PROFILE = ROOT / 'analysis/growth-first-20261016/m1-natural-phone-20261017/53-task7-end-profile.json'
OLD = ROOT / 'analysis/p6/p7-party-rush-pending-20261016/manifest.json'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def generate():
    sys.path.insert(0, str(ROOT))
    from server.codec import message
    from google.protobuf.json_format import ParseDict
    if SOURCE.parent.exists():
        raise FileExistsError('Preserve immutable input source')
    old = json.loads(OLD.read_bytes())
    if sha(OLD) != '59114ed6ef3ec96d53a4a0557719d5876f96489b8f42bb9103896af40d4a2f45':
        raise ValueError('Original NPC reference changed')
    actual = json.loads(PROFILE.read_bytes())['game']['run']['fight']['start']['battlePlayer']
    player = ParseDict(actual, message('BattlePlayer'))
    original_actors = [a.SerializeToString(deterministic=True) for a in player.formation]
    if [a.id for a in player.formation] != ['hero_047', 'hero_025'] or [a.pose for a in player.formation] != [3, 7]:
        raise ValueError('Only actual new-player original roster')
    # Finite admission qualification only; neither actor, inventory nor live
    # account is modified. This is not a natural level32 source proof.
    original_level = player.level
    player.level = 32
    # Main hero's effective level follows Player.level in battleWarrior. Keep
    # the actual static additions (including the observed +10 PDEF) and add
    # only the exact talent0 original growth delta. Companion025 stays level1.
    table = ROOT / 'analysis/tables/TableTalentAdvenced.tsv'
    records = list(csv.reader(table.open(), delimiter='\t'))
    growth = next(d for d in (dict(zip(records[1], row)) for row in records[5:])
                  if d.get('warId') == 'hero_047' and d.get('talentLevel') == '0' and d.get('trunck') == '1')
    main = player.formation[0]
    if main.level != original_level or main.talentList or main.awakeStage:
        raise ValueError('Exact observed main talent0 growth source required')
    before_main = copy.deepcopy(actual['formation'][0])
    for key in ['att', 'hp', 'pdef', 'mdef']:
        setattr(main.prop, key, getattr(main.prop, key) + float(growth[key + 'Grow']) * (32 - main.level))
    main.level = 32
    main.inst.curHp = main.prop.hp
    SOURCE.parent.mkdir(parents=True)
    rows = []
    for index, (npc_index, seed, delay) in enumerate([(0, 7, 0.), (0, 314159, 30.1), (1, 7, 0.), (1, 314159, 30.1)]):
        npc_source = old['cases'][npc_index]
        typed_path = OLD.parent / npc_source['typedCallback']
        if sha(typed_path) != npc_source['typedCallbackSHA256']:
            raise ValueError('Original typed NPC reference changed')
        reference = message('SCRushFightStart')
        reference.ParseFromString(typed_path.read_bytes())
        enemy = reference.battleMonsterList[0]
        fight_id = 'local-newbie047025-trial-' + str(index)
        typed = message('SCRushFightStart', fightId=fight_id, randSeed=seed,
                        battlePlayer=player, battleMonsterList=[enemy], fightScene=reference.fightScene,
                        maxRound=40, winKey=0, winValue=0, rushType=1)
        wire = message('SCFightPlayback', fightid=fight_id, randSeed=seed, battleType=9,
                       attacker=player, defender2=enemy, maxRound=40, attackerFactor=1, defenderFactor=1)
        typed_name, wire_name = f'{index:03d}.SCRushFightStart.pb', f'{index:03d}.pb'
        (SOURCE.parent / typed_name).write_bytes(typed.SerializeToString(deterministic=True))
        (SOURCE.parent / wire_name).write_bytes(wire.SerializeToString(deterministic=True))
        assert original_actors[1] == typed.battlePlayer.formation[1].SerializeToString(deterministic=True)
        rows.append({'case': f'phone8-actual047025/type9/{npc_source["npcPreparation"]}/seed{seed}/auto{delay}',
                     'wire': wire_name, 'wireSHA256': sha(SOURCE.parent / wire_name),
                     'typedCallback': typed_name, 'typedCallbackSHA256': sha(SOURCE.parent / typed_name),
                     'fightId': fight_id, 'seed': seed, 'battleType': 9, 'firstCamp': 0,
                     'nativeScope': 'original-pve-manual-then-auto-ai',
                     'manualControlSource': 'prepublished-diagnostic-schedule-v1',
                     'manualAutoStartRoundTime': delay, 'npcPreparation': npc_source['npcPreparation'],
                     'npcOriginalTypedSource': str(typed_path.relative_to(ROOT)),
                     'npcOriginalTypedSHA256': sha(typed_path),
                     'observedActorSHA256': [hashlib.sha256(x).hexdigest() for x in original_actors],
                     'diagnosticActorSHA256': [hashlib.sha256(a.SerializeToString(deterministic=True)).hexdigest() for a in player.formation],
                     'status': 'AWAITING_LOCAL_EXECUTION_AND_INDEPENDENT_PHONE_ANCHOR'})
    report = {'schema': 'p7-newbie047025-local-diagnostic-inputs-v1',
              'scope': 'Finite local typed9 diagnostic only; actor bytes from real task7, no type9 phone anchor',
              'actorSource': str(PROFILE.relative_to(ROOT)), 'actorSourceSHA256': sha(PROFILE),
              'npcManifestSource': str(OLD.relative_to(ROOT)), 'npcManifestSHA256': sha(OLD),
              'finiteQualification': {'field': 'Player.level plus main BattleHero growth projection', 'original': original_level, 'diagnostic': 32,
                                      'mainBefore': before_main, 'originalTalent0Growth': growth,
                                      'growthTable': str(table.relative_to(ROOT)), 'growthTableSHA256': sha(table),
                                      'projectionSource': 'copyService.battleWarrior effective main level; base+growth*(level-1), actual static bonuses retained',
                                      'productionSourceSHA256': sha(ROOT / 'hyjs-server/service/copyService.go'),
                                      'notObservedLevel32OnPhone': True, 'notNaturalLevelUnlock': True},
              'originalDLLSHA256': 'ee14d5df4077573abf94be309d6ebdbe7f4ba84c31b09743ed9a9008b65d200f',
              'cases': rows, 'nativeCompared': False, 'authoritative': False,
              'producerSHA256': sha(Path(__file__))}
    SOURCE.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'source': str(SOURCE.relative_to(ROOT)), 'sha256': sha(SOURCE), 'cases': 4}))


def prepare(work):
    sys.path.insert(0, str(ROOT))
    from tools.native_batch.prepare import load_rush_typed_source
    source = json.loads(SOURCE.read_bytes())
    if (source['schema'] != 'p7-newbie047025-local-diagnostic-inputs-v1' or len(source['cases']) != 4
            or source['nativeCompared'] or source['authoritative'] or sha(PROFILE) != source['actorSourceSHA256']):
        raise ValueError('Finite diagnostic input identity changed')
    folder = work / 'cases/trial047025/highest'
    folder.mkdir(parents=True)
    cases = []
    for row in source['cases']:
        load_rush_typed_source(row, SOURCE, 'highest')
        wire = SOURCE.parent / row['wire']
        typed = SOURCE.parent / row['typedCallback']
        if sha(wire) != row['wireSHA256']:
            raise ValueError('Wire SHA changed')
        shutil.copy2(wire, folder / wire.name)
        shutil.copy2(typed, folder / (wire.stem + '.rush-start.bin'))
        (folder / (wire.stem + '.rush-auto-start.txt')).write_text(str(row['manualAutoStartRoundTime']))
        (folder / (wire.stem + '.rush-auto-policy.txt')).write_text('prepublished-diagnostic-schedule-v1')
        cases.append({**row, 'localWire': str((folder / wire.name).relative_to(work)),
                      'sourceWire': str(wire.relative_to(ROOT)), 'typedSHA256': sha(typed)})
    return [{'set': 'trial047025', 'caseDirectory': str(folder.relative_to(work)),
             'sourceManifest': str(SOURCE.relative_to(ROOT)), 'sourceManifestSHA256': sha(SOURCE),
             'phoneAnchor': None, 'cases': cases}]


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--generate', action='store_true', required=True)
    parser.parse_args()
    generate()
