"""Freeze two result-free type16 original-wrapper probes; no device or DB IO."""
import argparse
import hashlib
import json
from pathlib import Path
import sys
ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from server.codec import message
ACTOR = 'analysis/p7/normal-party-phone-20261016/wire/start-CSRushFightStart-1792135594857-SCRushFightStart.pb'
NPC = 'analysis/p6/native-actual-paid-copy-auto-input-v87/000.pb'
PINS = {ACTOR: '2ff3a485782333378b2abf9084b53b0a91d6641df1862eeeea39d70108d6f1a4', NPC: 'ca7954f335c3613a864080ba1d2ce26974220942d60fb11067321fa200624937'}
DLL = 'analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll'
DLL_SHA = 'ee14d5df4077573abf94be309d6ebdbe7f4ba84c31b09743ed9a9008b65d200f'
def sha(raw):
    return hashlib.sha256(raw).hexdigest()
def prepare(output):
    if output.exists():
        raise ValueError('Immutable output already exists')
    sources = {}
    for name, expected in {**PINS, DLL: DLL_SHA}.items():
        raw = (ROOT / name).read_bytes()
        if sha(raw) != expected:
            raise ValueError('Pinned source changed: ' + name)
        sources[name] = raw
    original = message('SCRushFightStart'); original.ParseFromString(sources[ACTOR])
    npc = message('SCFightPlayback'); npc.ParseFromString(sources[NPC])
    if [v.id for v in original.battlePlayer.formation] != ['hero_001', 'hero_003']:
        raise ValueError('Actual normal party source differs')
    if [v.id for v in npc.defender2.formation] != ['monster_033', 'monster_033']:
        raise ValueError('Actual original NPC source differs')
    cases = []
    assets = {}
    for index, seed in enumerate((7, 314159)):
        name = f'diagnostic-awake16-normalparty-{index}'
        typed = message('SCAwakeCopyFightBegin', fightId=name, randomSeed=seed,
                        maxRoundNum=40, roomId='map_forest_003')
        typed.battlePlayer.CopyFrom(original.battlePlayer)
        typed.battleMonser.CopyFrom(npc.defender2)
        wire = message('SCFightPlayback', fightid=name, randSeed=seed, battleType=16,
                       maxRound=40, attackerFactor=1, defenderFactor=1)
        wire.attacker.CopyFrom(typed.battlePlayer); wire.defender2.CopyFrom(typed.battleMonser)
        if wire.HasField('optRecord') or wire.winConditions or wire.HasField('defender1'):
            raise ValueError('Probe contains result or unrelated defender metadata')
        wire_name = f'{index:03}.pb'; typed_name = f'{index:03}.SCAwakeCopyFightBegin.pb'
        assets[wire_name] = wire.SerializeToString(deterministic=True)
        assets[typed_name] = typed.SerializeToString(deterministic=True)
        cases.append({'case': name, 'battleType': 16, 'fightId': name, 'seed': seed,
                      'nativeScope': 'original-awake16-automatic-ai', 'diagnosticOnly': True,
                      'wire': wire_name, 'wireSHA256': sha(assets[wire_name]),
                      'typedCallback': typed_name, 'typedCallbackSHA256': sha(assets[typed_name]),
                      'originalSceneBranch': 'nonempty roomId -> original roomId',
                      'sourceResultMetadataIgnored': True})
    manifest = {'cases': cases, 'authoritative': False, 'nativeCompared': False,
                'sourcePins': {**PINS, DLL: DLL_SHA},
                'actorSource': 'Actual account4 original Rush Start actor bytes only; no type9 authority borrowed',
                'enemySource': 'Actual original Copy task10 two monster033 actor bytes; Awake private formation unknown',
                'localComposition': 'local-awake-forest-explore-v1: outside mon030 model monster033; original NPC pair frozen unchanged',
                'driver': {'typed': 'BattleData.SetData(SCAwakeCopyFightBegin)',
                           'isPveAutoCache': True, 'attackFirst': True, 'factors': [1, 1],
                           'maxRound': 40, 'roomId': 'map_forest_003',
                           'controls': 'Original automatic AI from initialization; no supplied record, winner, manual input or patched engine'},
                'requiredEvidence': ['raw original End and complete optRecord', 'all export/snapshot/event families', 'independent terminal actor HP', 'source and collector/caller/APK SHA']}
    output.mkdir(parents=True)
    for name, raw in assets.items():
        (output / name).write_bytes(raw)
    raw = (json.dumps(manifest, ensure_ascii=False, indent=2) + '\n').encode()
    (output / 'manifest.json').write_bytes(raw)
    return {'manifest': str(output / 'manifest.json'), 'sha256': sha(raw), 'cases': len(cases), 'authority': False}
def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args=parser.parse_args()
    print(json.dumps(prepare(args.output.resolve()), ensure_ascii=False))
if __name__ == '__main__':
    main()
