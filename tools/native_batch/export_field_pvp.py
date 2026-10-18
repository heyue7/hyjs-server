"""Export independent typed5 probes from preserved actual actor sources.

This reads saved evidence only. It does not start battles, operate a device,
change a profile, or treat earlier type14 authorization as field PK proof.
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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    if not output.is_relative_to(ROOT / 'analysis/p6') or output.exists():
        raise RuntimeError('Use a new owned combat evidence directory')
    full = ROOT / 'analysis/p13/actual-player-battle172-source-5478b0dcd92367fb'
    partial = ROOT / 'analysis/p6/native-actual-war-wounded14-inputs-v196'
    source_manifest = (full / 'manifest.json').read_bytes()
    partial_manifest = (partial / 'manifest.json').read_bytes()
    if (digest(source_manifest) != 'fbf44f3907dc5bd5eb7e836c792601e32148cb3f96418df8ecc6a06a0aff693c'
            or digest(partial_manifest) != '8ee3bd70dd013e277fd879c0a0742a173e1c0a7ed94a784c80f1355140559021'):
        raise RuntimeError('Actual actor provenance changed')
    pins = {str((full / 'manifest.json').relative_to(ROOT)): digest(source_manifest),
            str((partial / 'manifest.json').relative_to(ROOT)): digest(partial_manifest)}
    raw = (full / 'actual.SCUFFightRecordReplay.pb').read_bytes()
    if digest(raw) != 'c9310344e65f3a0775c94320762d703b0255c27df159a588b26330d722e1d076':
        raise RuntimeError('Actual full-HP pair changed')
    original = message('SCUFFightRecordReplay'); original.ParseFromString(raw)
    pins[str((full / 'actual.SCUFFightRecordReplay.pb').relative_to(ROOT))] = digest(raw)
    pairs = [(original.playerList[0], original.playerList[1], 'actual-full'),
             (original.playerList[1], original.playerList[0], 'actual-full-reversed')]
    for row in json.loads(partial_manifest)['cases']:
        path = partial / row['wire']
        raw = path.read_bytes()
        if digest(raw) != row['wireSHA256']:
            raise RuntimeError('Actual injured actor projection changed')
        wire = message('SCFightPlayback'); wire.ParseFromString(raw)
        pairs.append((wire.attacker, wire.defender1, 'actual-wounded-' + str(len(pairs) - 2)))
        pins[str(path.relative_to(ROOT))] = digest(raw)
    output.mkdir(parents=True)
    cases = []
    for index, (attacker, defender, origin) in enumerate(pairs):
        seed = (7, 314159)[index % 2]
        identity = 'diagnostic-field5-actual-actors-' + str(index)
        typed = message('SidePvpFightData')
        typed.id, typed.randSeed, typed.maxRoundCount = identity, seed, 40
        typed.attPlayer.CopyFrom(attacker); typed.defPlayer.CopyFrom(defender)
        wire = message('SCFightPlayback')
        wire.fightid, wire.randSeed, wire.maxRound, wire.battleType = identity, seed, 40, 5
        wire.attackerFactor, wire.defenderFactor = 1, 1
        wire.attacker.CopyFrom(attacker); wire.defender1.CopyFrom(defender)
        if not attacker.HasField('inst') or not defender.HasField('inst'):
            raise RuntimeError('Original fighter instances are required')
        name = '%03d' % index
        typed_raw = typed.SerializeToString(deterministic=True)
        wire_raw = wire.SerializeToString(deterministic=True)
        (output / (name + '.pb')).write_bytes(wire_raw)
        (output / (name + '.SidePvpFightData.pb')).write_bytes(typed_raw)
        cases.append({'case': origin, 'fightId': identity, 'battleType': 5, 'seed': seed,
                      'wire': name + '.pb', 'wireSHA256': digest(wire_raw),
                      'typedCallback': name + '.SidePvpFightData.pb',
                      'typedCallbackSHA256': digest(typed_raw), 'fieldContext': 'field_002',
                      'nativeScope': 'original-field-pvp5-field002-automatic-ai',
                      'actorSHA256': [digest(x.SerializeToString(deterministic=True))
                                      for x in (attacker, defender)], 'diagnosticOnly': True})
    manifest = {'cases': cases, 'sourcePins': pins, 'nativeCompared': False, 'authoritative': False,
                'scope': 'Independent original type5/field002 ice-map automatic driver. Preserved actual '
                         'full/wounded single047 actor bytes, GUID/HP/AP/initiative unchanged; new diagnostic '
                         'seed and identity only. No field battle/result, wallet or type14 authority imported.',
                'originalField001AllSafe': True, 'field001NotOpenedForPK': True,
                'sceneContext': 'Original Field002 allSafe0, BattleMap map_ice_001; diagnostic static restored'}
    (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    print(json.dumps({'cases': len(cases), 'manifest': str((output / 'manifest.json').relative_to(ROOT)),
                      'sha256': digest((output / 'manifest.json').read_bytes())}))


if __name__ == '__main__':
    main()
