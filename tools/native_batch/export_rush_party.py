"""Prepare bounded original type9 party evidence; never reads DB or controls a device."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import re
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
        raise RuntimeError('Choose a new task-owned directory under analysis/p6')
    original = ROOT / 'analysis/p6/actual-demo4-vs-demo3-friend11-input-v128b'
    source_path = original / '000.frozen-input.json'
    source_raw = source_path.read_bytes()
    expected = 'd0e70186dc665f5e894eaae0a30915cb8032beb2ab86e0950ebd3e0ada41fa94'
    if digest(source_raw) != expected:
        raise RuntimeError('Historical actual P5 source changed')
    source = json.loads(source_raw)
    player = message('BattlePlayer', **source['attacker'])
    # Rush's existing battlePlayer producer supplies Inst anger0 with no PvP
    # firstOrder flag. Historical actor bytes remain completely unchanged.
    player.inst.ClearField('firstOrder')
    if [hero.id for hero in player.formation] != ['hero_001', 'hero_003']:
        raise RuntimeError('Actual source is not the expected complete original party')
    actor_digest = [digest(hero.SerializeToString(deterministic=True)) for hero in player.formation]
    constants = (ROOT / 'hyjs-server/constUtil/p7SectionReward.go').read_text()
    enemy_hex = re.search(r'P7SectionEnemyHex\s*=\s*"([^"]+)"', constants).group(1)
    template = bytes.fromhex(enemy_hex)
    if digest(template) != '97ec2ffad1cf9bc3946ecde31274df10b95d030dbfb40d98c9859951189c12f2':
        raise RuntimeError('Pinned original NPC source changed')
    output.mkdir()
    cases = []
    # First two are existing local gameplay shapes. The long pair changes only
    # explicit diagnostic NPC HP to exercise real ultimates and expiry paths;
    # it has no expected native winner or business eligibility.
    for i, (shape, seed, delay) in enumerate([
        ('original-npc', 7, 0.0), ('local-floor4', 314159, 30.1),
        ('explicit-long-hp-probe', 7, 0.0), ('explicit-long-hp-probe', 314159, 30.1),
    ]):
        enemy = message('BattleMonster')
        enemy.ParseFromString(template)
        if shape == 'local-floor4':
            for key in ['att', 'hp', 'pdef', 'mdef']:
                setattr(enemy.formation[0].prop, key, math.floor(getattr(enemy.formation[0].prop, key)*1.5))
            enemy.formation[0].inst.curHp = enemy.formation[0].prop.hp
        if shape == 'explicit-long-hp-probe':
            enemy.formation[0].prop.hp = 100000
            enemy.formation[0].inst.curHp = 100000
        fight_id = 'diagnostic-party-rush-' + str(i)
        typed = message('SCRushFightStart', fightId=fight_id, randSeed=seed, battlePlayer=player,
                        battleMonsterList=[enemy], fightScene='map_grass_001', maxRound=40, winKey=0, winValue=0, rushType=1)
        wire = message('SCFightPlayback', fightid=fight_id, randSeed=seed, battleType=9,
                       attacker=player, defender2=enemy, maxRound=40, attackerFactor=1, defenderFactor=1)
        typed_raw = typed.SerializeToString(deterministic=True)
        wire_raw = wire.SerializeToString(deterministic=True)
        typed_name = f'{i:03d}.SCRushFightStart.pb'
        wire_name = f'{i:03d}.pb'
        (output / typed_name).write_bytes(typed_raw)
        (output / wire_name).write_bytes(wire_raw)
        cases.append(dict(case=f'actual-historical001003/type9/{shape}/seed{seed}/auto{delay}',
                          battleType=9, fightId=fight_id, seed=seed, firstCamp=0,
                          typedCallback=typed_name, typedCallbackSHA256=digest(typed_raw),
                          wire=wire_name, wireSHA256=digest(wire_raw),
                          nativeScope='original-pve-manual-then-auto-ai',
                          manualControlSource='prepublished-diagnostic-schedule-v1', manualAutoStartRoundTime=delay,
                          actualActorSHA256=actor_digest, npcPreparation=shape,
                          status='awaiting-original-automatic-ai', evidenceStatus='AWAITING_ORIGINAL_TYPED9_FULL_PARTY_EVIDENCE'))
    manifest = dict(scope='Four bounded diagnostic typed9 cases; unchanged historical actual P5 complete001003 actors. No business qualification, live account read, preset result or global authority.',
                    originalDLLSHA256='ee14d5df4077573abf94be309d6ebdbe7f4ba84c31b09743ed9a9008b65d200f',
                    actorSource=str(source_path.relative_to(ROOT)), actorSourceSHA256=expected,
                    sourceActorEngineVersion=source['engineVersion'], configurationHash=source['configurationHash'],
                    originalNPCSHA256=digest(template), originalTotalFloorsKnown=False,
                    cases=cases, nativeCompared=False, authoritative=False)
    (output / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps({'manifest': str(output / 'manifest.json'), 'cases': len(cases), 'nativeCompared': False}))


if __name__ == '__main__':
    main()
