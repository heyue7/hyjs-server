"""Prepare diagnostic type3 original callbacks from immutable actual actor sources.

This performs no DB/device IO and supplies no winner or resources. Public
plunder still requires current level/membership/fragment/avoid-war verification
and an independently certified type3 computation in its own transaction.
"""
import argparse
import hashlib
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from server.codec import message


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def export(output):
    output = Path(output).resolve()
    if not output.is_relative_to(ROOT/'analysis/p11') or output.exists():
        raise ValueError('Use a new immutable analysis/p11 directory')
    sources = [
        (ROOT/'analysis/p6/native-actual-arena047-typed-inputs-v121', None),
        (ROOT/'analysis/p6/actual-full-party-arena2-inputs-v151',
         'c60ffe40b136342ecf406c749b85dea9d5c4776ef4cbe0d635c0a61bf1b3f244'),
    ]
    rows, prepared, bindings = [], [], []
    for source, expected in sources:
        manifest_raw = (source/'manifest.json').read_bytes()
        if expected is not None and digest(manifest_raw) != expected:
            raise ValueError('Original diagnostic source manifest changed')
        original = json.loads(manifest_raw)
        bindings.append({'path': str((source/'manifest.json').relative_to(ROOT)),
                         'sha256': digest(manifest_raw)})
        for source_index, case in enumerate(original['cases']):
            wire_raw = (source/case['wire']).read_bytes()
            frozen_raw = (source/case['frozenInput']).read_bytes()
            if digest(wire_raw) != case['wireSHA256'] or digest(frozen_raw) != case['frozenInputSHA256']:
                raise ValueError('Original actor source SHA changed')
            wire = message('SCFightPlayback')
            wire.ParseFromString(wire_raw)
            if wire.battleType != 2 or wire.HasField('optRecord'):
                raise ValueError('Expected original full-skill automatic source')
            # Preserve all actor/Inst/seed bytes, including full original skills,
            # actual GUIDs, panels and gear. Only the new diagnostic mode/round
            # contract and local diagnostic identity change.
            actors = [v.SerializeToString(deterministic=True) for v in [wire.attacker, wire.defender1]]
            if any(not player.HasField('inst') or not player.formation
                   or any(not actor.HasField('prop') or not actor.HasField('inst') for actor in player.formation)
                   for player in [wire.attacker, wire.defender1]):
                raise ValueError('Original caller requires complete actor and team Inst')
            pair = (wire.attacker.inst.firstOrder, wire.defender1.inst.firstOrder)
            if pair not in [(1, 0), (0, 1)]:
                raise ValueError('Preserve an actual legal original first-order pair')
            frozen = json.loads(frozen_raw)
            index = len(rows)
            wire.battleType, wire.maxRound = 3, 20
            wire.fightid = 'diagnostic-original-plunder3-'+str(index)
            callback = message('SCFightTreasureStartResult', fightId=wire.fightid,
                               roomId='map_arena_001', randSeed=wire.randSeed,
                               maxRoundNum=20)
            callback.playerList.add().CopyFrom(wire.attacker)
            callback.playerList.add().CopyFrom(wire.defender1)
            if actors != [v.SerializeToString(deterministic=True) for v in callback.playerList]:
                raise ValueError('Actor projection changed during diagnostic transport')
            frozen['battleType'], frozen['maxRound'] = 3, 20
            frozen.setdefault('appliedProperties', {}).setdefault('source', []).append(
                'Explicit diagnostic type3 bridge from immutable original actor sources; '
                'original SCFightTreasureStartResult SetData fixes Max20/map_arena_001. '
                'No current plunder query, target possession, payment, reward, transfer or server result.')
            frozen['appliedProperties']['plunderDiagnosticTarget'] = ['frag_trea_001_1']
            frozen['appliedProperties']['plunderActorSourceSHA256'] = [digest(wire_raw), digest(frozen_raw)]
            name = f'{index:03d}'
            data = {
                name+'.pb': wire.SerializeToString(deterministic=True),
                name+'.SCFightTreasureStartResult.pb': callback.SerializeToString(deterministic=True),
                # Compact exact JSON avoids duplicating formatted multi-MB table
                # snapshots; every original table value remains present.
                name+'.frozen-input.json': (json.dumps(frozen, ensure_ascii=False, separators=(',', ':'))+'\n').encode(),
            }
            prepared.append(data)
            levels = [wire.attacker.level, wire.defender1.level]
            distinct = wire.attacker.playerId != wire.defender1.playerId
            rows.append(dict(case='diagnostic-original-plunder3/'+case['case'], battleType=3,
                             fightId=wire.fightid, seed=wire.randSeed, firstCamp=0 if pair==(1, 0) else 1,
                             maxRound=20, targetFragId='frag_trea_001_1', nativeScope='original-plunder-automatic-ai',
                             wire=name+'.pb', wireSHA256=digest(data[name+'.pb']),
                             typedCallback=name+'.SCFightTreasureStartResult.pb',
                             typedCallbackSHA256=digest(data[name+'.SCFightTreasureStartResult.pb']),
                             frozenInput=name+'.frozen-input.json', frozenInputSHA256=digest(data[name+'.frozen-input.json']),
                             sourceManifest=bindings[-1], sourceCase=source_index,
                             originalWireSHA256=digest(wire_raw), originalFrozenInputSHA256=digest(frozen_raw),
                             actorSHA256=[digest(v) for v in actors], actorPlayerLevels=levels,
                             distinctPlayers=distinct, capturedLevel15Qualification=all(v>=15 for v in levels),
                             currentPlunderQualificationVerified=False, targetFragmentPossessionVerified=False,
                             status='awaiting-original-type3-callback'))
    output.mkdir(parents=True)
    for data in prepared:
        for name, raw in data.items():
            (output/name).write_bytes(raw)
    result = dict(scope='Diagnostic only: exact captured actors/whole skills/GUID/Inst/seed preserved; '
                        'type3 Max20 original callback, no winner/get/cost/reward/isGetFrag. '
                        'Old public Arena/diagnostic sources do not prove current plunder eligibility or fragment possession. '
                        'Some full-party sources contain a level6 peer or same-account diagnostic mirror; those are engine samples only. '
                        'Original type3 certificate and actual current protected/fragment/profile checks remain mandatory.',
                  sourceManifests=bindings, cases=rows, nativeCompared=False, authoritative=False,
                  dbOrDeviceAccess=False)
    (output/'manifest.json').write_text(json.dumps(result, ensure_ascii=False, indent=2)+'\n')
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = export(args.output)
    print(args.output/'manifest.json', digest((args.output/'manifest.json').read_bytes()), 'cases', len(result['cases']))
