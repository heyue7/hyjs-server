"""Real tutorial-talented roster, finite Trial22/32 projection, no device IO."""
import argparse
import copy
import csv
import hashlib
import json
from pathlib import Path
import shutil
import sys
from trial047025 import ROOT, sha

SOURCE = ROOT / 'analysis/p6/p7-numeric047025-local-inputs-r1-20261017/manifest.json'
PROFILE = ROOT / 'analysis/growth-first-20261016/m1-natural-phone-20261017/102-first-real-strengthen-profile.json'
OBSERVED = ROOT / 'analysis/growth-first-20261016/m1-natural-phone-20261017/106-equipped-copy1-run-profile.json'
OLD = ROOT / 'analysis/p6/p7-party-rush-pending-20261016/manifest.json'


def table(name):
    rows = list(csv.reader((ROOT / 'analysis/tables' / ('Table' + name + '.tsv')).open(), delimiter='\t'))
    return [dict(zip(rows[1], r)) for r in rows[5:]]


def generate():
    sys.path.insert(0, str(ROOT))
    from server.codec import message
    from google.protobuf.json_format import ParseDict
    if SOURCE.parent.exists():
        raise FileExistsError('Preserve immutable source')
    actual = json.loads(PROFILE.read_bytes())
    observed = json.loads(OBSERVED.read_bytes())
    if actual['player']['level'] != 4:
        raise ValueError('Exact actual player4 source required')
    for field in ['warriors', 'formation', 'tactical_formations', 'tactical_skill_id', 'pets', 'kv']:
        if actual['game'].get(field) != observed['game'].get(field):
            raise ValueError('Real combat state changed from the observed original Start: ' + field)
    if actual['player']['currentWorldSeedId'] != observed['player']['currentWorldSeedId']:
        raise ValueError('Worldseed changed')
    # Real level2 gear stays worn. Its actual +34.5 ATT is retained from the
    # observed original Start, never truncated or synthesized from a GUI label.
    worn = {x.get('guid') for f in actual['game']['formation'] for x in f.get('things', [])}
    gear = actual['game'].get('equipment', [])
    if len(gear) != 1 or gear[0]['equipId'] != 'equip_001' or gear[0]['level'] != 2 or gear[0]['guid'] not in worn:
        raise ValueError('Exact real strengthened worn001 source required')
    battle = observed['game']['run']['fight']['start']['battlePlayer']
    if [a['id'] for a in battle['formation']] != ['hero_047', 'hero_025']:
        raise ValueError('Exact actual party required')
    if battle['formation'][0]['talentList'] != ['tal_1'] or battle['formation'][1]['level'] != 3:
        raise ValueError('Mandatory tutorial talent/companion level lost')
    growth = next(r for r in table('TalentAdvenced') if r['warId'] == 'hero_047' and r['trunck'] == '1' and r['talentLevel'] == '1')
    gate = next(r for r in table('GameConfigure') if r['StaticName'] == 'RushLimitLevel')
    if gate['NameValue'] != '22':
        raise ValueError('Actual Trial gate changed')
    npc_source = json.loads(OLD.read_bytes())['cases'][0]
    npc_path = OLD.parent / npc_source['typedCallback']
    if sha(npc_path) != npc_source['typedCallbackSHA256']:
        raise ValueError('Existing original NPC reference changed')
    reference = message('SCRushFightStart')
    reference.ParseFromString(npc_path.read_bytes())
    SOURCE.parent.mkdir(parents=True)
    cases = []
    for index, (level, seed, at) in enumerate([(22, 7, 0.), (22, 314159, 30.1), (32, 7, 0.), (32, 314159, 30.1)]):
        player = ParseDict(copy.deepcopy(battle), message('BattlePlayer'))
        main = player.formation[0]
        # Original frozen talent1 panel includes its constant additions and
        # original buff_1 ATT50. Only effective-main level growth changes.
        for key in ['att', 'hp', 'pdef', 'mdef']:
            setattr(main.prop, key, getattr(main.prop, key) + float(growth[key + 'Grow']) * (level - main.level))
        main.level = level
        main.inst.curHp = main.prop.hp
        player.level = level
        fight = 'local-natural047025-trial-' + str(index)
        typed = message('SCRushFightStart', fightId=fight, randSeed=seed, battlePlayer=player,
                        battleMonsterList=[reference.battleMonsterList[0]], fightScene=reference.fightScene,
                        maxRound=40, winKey=0, winValue=0, rushType=1)
        wire = message('SCFightPlayback', fightid=fight, randSeed=seed, battleType=9, attacker=player,
                       defender2=reference.battleMonsterList[0], maxRound=40, attackerFactor=1, defenderFactor=1)
        typed_name, wire_name = f'{index:03d}.SCRushFightStart.pb', f'{index:03d}.pb'
        (SOURCE.parent / typed_name).write_bytes(typed.SerializeToString(deterministic=True))
        (SOURCE.parent / wire_name).write_bytes(wire.SerializeToString(deterministic=True))
        cases.append({'case': f'actual-equip001lv2-047tal1-025lv3/type9/finite-player{level}/seed{seed}/auto{at}',
                      'wire': wire_name, 'wireSHA256': sha(SOURCE.parent / wire_name),
                      'typedCallback': typed_name, 'typedCallbackSHA256': sha(SOURCE.parent / typed_name),
                      'fightId': fight, 'seed': seed, 'battleType': 9, 'firstCamp': 0,
                      'finitePlayerLevel': level, 'nativeScope': 'original-pve-manual-then-auto-ai',
                      'manualControlSource': 'prepublished-diagnostic-schedule-v1', 'manualAutoStartRoundTime': at,
                      'originalNPCSHA256': hashlib.sha256(reference.battleMonsterList[0].SerializeToString(deterministic=True)).hexdigest(),
                      'diagnosticActorSHA256': [hashlib.sha256(a.SerializeToString(deterministic=True)).hexdigest() for a in player.formation],
                      'status': 'LOCAL_EXECUTION_AND_INDEPENDENT_PHONE_ANCHOR_PENDING'})
    report = {'schema': 'p7-numeric047025-local-inputs-v1',
              'actualProfile': str(PROFILE.relative_to(ROOT)), 'actualProfileSHA256': sha(PROFILE),
              'originalObservedStartProfile': str(OBSERVED.relative_to(ROOT)), 'originalObservedStartProfileSHA256': sha(OBSERVED),
              'actualPlayerLevel': 4, 'actualTrialLimitLevel': 22, 'actualWornEquipment': gear,
              'finiteQualification': 'Only Player level22/32 and corresponding effective main growth projection; actual tutorial talent1 and companion3 retained; not natural level unlocking proof',
              'projection': {'originalMainLevel': battle['formation'][0]['level'], 'originalTalent1Growth': growth,
                             'productionSourceSHA256': sha(ROOT / 'hyjs-server/service/copyService.go'),
                             'talentTableSHA256': sha(ROOT / 'analysis/tables/TableTalentAdvenced.tsv'),
                             'gameConfigureSHA256': sha(ROOT / 'analysis/tables/TableGameConfigure.tsv')},
              'cases': cases, 'nativeCompared': False, 'authoritative': False,
              'producerSHA256': sha(Path(__file__))}
    SOURCE.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'source': str(SOURCE.relative_to(ROOT)), 'sha256': sha(SOURCE), 'cases': 4}))


def prepare(work):
    sys.path.insert(0, str(ROOT))
    from tools.native_batch.prepare import load_rush_typed_source
    source = json.loads(SOURCE.read_bytes())
    if len(source['cases']) != 4 or source['nativeCompared'] or source['authoritative']:
        raise ValueError('Explicit four natural-roster diagnostic vectors only')
    if sha(PROFILE) != source['actualProfileSHA256'] or sha(OBSERVED) != source['originalObservedStartProfileSHA256']:
        raise ValueError('Actual source evidence changed')
    folder = work / 'cases/trial047025-numeric/highest'
    folder.mkdir(parents=True)
    rows = []
    for row in source['cases']:
        load_rush_typed_source(row, SOURCE, 'highest')
        wire = SOURCE.parent / row['wire']
        if sha(wire) != row['wireSHA256']:
            raise ValueError('Wire changed')
        shutil.copy2(wire, folder / wire.name)
        shutil.copy2(SOURCE.parent / row['typedCallback'], folder / (wire.stem + '.rush-start.bin'))
        (folder / (wire.stem + '.rush-auto-start.txt')).write_text(str(row['manualAutoStartRoundTime']))
        (folder / (wire.stem + '.rush-auto-policy.txt')).write_text(row['manualControlSource'])
        rows.append({**row, 'localWire': str((folder / wire.name).relative_to(work)),
                     'sourceWire': str(wire.relative_to(ROOT)), 'typedSHA256': row['typedCallbackSHA256']})
    return [{'set': 'trial047025-numeric', 'caseDirectory': str(folder.relative_to(work)),
             'sourceManifest': str(SOURCE.relative_to(ROOT)), 'sourceManifestSHA256': sha(SOURCE),
             'phoneAnchor': None, 'cases': rows}]


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--generate', action='store_true', required=True)
    parser.parse_args()
    generate()
