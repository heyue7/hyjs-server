"""Read P7 phone acceptance facts without issuing any game action.

Offline profile inspection is the default. --live explicitly reads the shared
project MySQL allocation; every database statement in this tool is a SELECT.
"""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from server.codec import message


def profile_facts(profile):
    player, game = profile['player'], profile['game']
    slots = game.get('formation', [])
    used = {thing['guid'] for slot in slots for thing in slot.get('things', [])}
    pets = []
    for row in game.get('kv', []):
        if not row['key'].startswith('p9.pet:') or row['key'][7:] not in used:
            continue
        raw = base64.b64decode(row['value'], validate=True)
        pet = message('Pet')
        pet.ParseFromString(raw)
        pets.append({'id': pet.id, 'guid': pet.guid, 'level': pet.level,
                     'breachedLevel': pet.breached.level, 'intimacyLevel': pet.eat.level,
                     'intimacyExp': pet.eat.exp, 'protobufSHA256': hashlib.sha256(raw).hexdigest()})
    facts = {'player': {key: player.get(key, 0) for key in
                       ('playerId', 'level', 'heroWarId', 'coin', 'diamond', 'point', 'energy')},
             'formation': slots,
             'equippedWarriors': [row for row in game.get('warriors', []) if row['guid'] in used],
             'equippedEquipment': [row for row in game.get('equipment', []) if row['guid'] in used],
             'guardPets': pets, 'items': game.get('items', []),
             'activeRun': game.get('run'), 'field': game.get('field'),
             'tacticalSkillId': game.get('tacticalSkillId'),
             'tacticalFormations': game.get('tacticalFormations')}
    facts['profileSHA256'] = hashlib.sha256(json.dumps(profile, sort_keys=True,
        separators=(',', ':'), ensure_ascii=False).encode()).hexdigest()
    return facts


def live_facts(player_id):
    from tools.shared_mysql import sql
    raw = sql("SELECT JSON_OBJECT('player',CAST(player AS JSON),'game',CAST(game AS JSON)) "
              f'FROM profiles WHERE id={player_id};').strip()
    if not raw:
        raise RuntimeError('Requested existing player is missing')
    result = profile_facts(json.loads(raw))
    queries = {
        'state': 'SELECT JSON_OBJECT("runId",run_id,"floor",floor,"totalStars",total_stars,"highFloor",high_floor,"activeFight",CONVERT(active_fight USING utf8mb4)) FROM p7_rush_states',
        'addition': 'SELECT JSON_OBJECT("runId",run_id,"selected",selected,"spentStars",spent_stars,"firstFightId",CONVERT(first_fight_id USING utf8mb4),"selectedAt",selected_at,"policyHash",policy_hash) FROM p7_rush_addition_offers',
        'section': 'SELECT JSON_OBJECT("runId",run_id,"claimedStars",claimed_stars,"claimedAt",claimed_at,"acceptanceHash",acceptance_hash) FROM p7_section_reward_offers',
        'fights': 'SELECT JSON_OBJECT("fightId",CONVERT(id USING utf8mb4),"runId",run_id,"taskId",task_id,"kind",kind,"status",CONVERT(status USING utf8mb4),"diagnostic",diagnostic,"startHash",start_hash,"ruleHash",rule_hash,"endRequestSHA",SHA2(end_request,256)) FROM p7_rush_fights',
        'manual': 'SELECT JSON_OBJECT("fightId",CONVERT(fight_id USING utf8mb4),"status",status,"scopeProof",scope_proof,"driverPolicy",driver_policy,"seriesHash",series_hash,"startHash",start_hash,"terminalHash",terminal_hash,"endRequestHash",end_request_hash,"acceptedAt",accepted_at) FROM p7_rush_manual_calculations',
    }
    for name, query in queries.items():
        result[name] = [json.loads(line) for line in sql(query + f' WHERE player_id={player_id};').splitlines()]
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument('--profile-file', type=Path)
    source.add_argument('--live', action='store_true')
    parser.add_argument('--player-id', type=int, default=4)
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    if args.player_id != 4:
        parser.error('This acceptance is reserved for the existing normal account4')
    facts = live_facts(args.player_id) if args.live else profile_facts(json.loads(args.profile_file.read_bytes()))
    result = {'observedAtUnixMs': int(time.time() * 1000), 'source': 'live-read-only' if args.live else str(args.profile_file),
              'gameActions': 0, 'sqlWrites': False, 'facts': facts}
    raw = json.dumps(result, ensure_ascii=False, indent=2) + '\n'
    if args.output:
        output = args.output.resolve()
        if not output.is_relative_to(ROOT / 'analysis/p7'):
            parser.error('Use an owned P7 evidence path')
        output.parent.mkdir(parents=True, exist_ok=True)
        with output.open('x') as file:
            file.write(raw)
        print(json.dumps({'path': str(output), 'sha256': hashlib.sha256(raw.encode()).hexdigest()}))
    else:
        print(raw, end='')


if __name__ == '__main__':
    main()
