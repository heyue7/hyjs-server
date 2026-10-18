"""Export original completed Daily records as native playback protocol, read-only.

This does not serve HTTP, alter a Profile/P6 record, recompute a battle or issue
rewards. Root integrates an authenticated diagnostic-only lookup or relay before
using the unchanged original PlaybackKit buttons on the one device.
"""
import argparse
import hashlib
import json
import math
import re
import subprocess
from pathlib import Path

from server.codec import BY_NAME, encode_response, message

ROOT = Path(__file__).resolve().parents[1]
CASES = {
    "95efecfe-ac5a-44c5-bf8c-69f7e859bb31": ("TreasureExp", "5", "daily-id4-native-20261013.json"),
    "42ac0bba-d5fd-472e-a54a-83fa53002dcd": ("PetFood_dark", "28", "daily-fooddark-id4-native-20261013.json"),
}


def project(row, fixture):
    """Return only the frozen original players, seed, operations and actual result."""
    fight_id, player_id, kind, chapter, settled, start_hex, end_hex, response_hex = row
    if fight_id not in CASES or str(player_id) != "4" or str(settled) != "1":
        raise ValueError("Diagnostic playback must be an existing completed owned Daily")
    expected_type, expected_chapter, _ = CASES[fight_id]
    if kind != expected_type or chapter != expected_chapter or fixture['fight_id'] != fight_id or fixture['player_id'] != "4":
        raise ValueError("Original Daily identity/catalog changed")
    start_raw, end_raw, response_raw = (bytes.fromhex(value) for value in [start_hex, end_hex, response_hex])
    if start_raw != bytes.fromhex(fixture['start_hex']) or end_raw != bytes.fromhex(fixture['end_hex']):
        raise ValueError("Stored native start/end bytes differ from the reviewed original fixture")
    start = message('SCDailyCopyFightStart'); start.ParseFromString(start_raw)
    end = message('CSDailyCopyFightEnd'); end.ParseFromString(end_raw)
    response = message('SCDailyCopyFightEnd'); response.ParseFromString(response_raw)
    if (start.fightId != fight_id or end.fightId != fight_id or end.ChapterId != chapter
            or start.battlePlayer.playerId != 4 or start.battlePlayer.serverId != 1
            or start.monster.taskId != chapter or end.isWin not in (0, 1)
            or response.isWin != end.isWin or not end.HasField('optRecord')):
        raise ValueError("Original Daily protocol ownership/result mismatch")
    # Validate references without interpreting AP or manufacturing a result.
    sides = [{actor.guid for actor in start.battlePlayer.formation}, {actor.guid for actor in start.monster.formation}]
    if (end.optRecord.finalRoundNum < 1 or end.optRecord.finalRoundNum > start.maxRoundNum
            or any(not math.isfinite(hp) or hp < 0 for hp in [end.optRecord.attackerTotalLeftHp, end.optRecord.defenderTotalLeftHp])):
        raise ValueError("Original completed Daily summary is invalid")
    for operation in end.optRecord.skillOpts:
        if (operation.camp not in (0, 1) or operation.actorUid not in sides[operation.camp]
                or operation.roundNum < 0 or operation.roundNum > end.optRecord.finalRoundNum
                or not math.isfinite(operation.roundTime) or operation.roundTime < 0):
            raise ValueError("Original Daily operation does not belong to its frozen sides")
    # Original playback uses0=attacker win/1=attacker loss. Daily isWin has the
    # opposite bool meaning; never pass its raw0 as an invented playback win.
    actual_result = 0 if end.isWin == 1 else 1
    out = message('SCFightPlayback', fightid=fight_id, randSeed=start.randSeed, battleType=4,
                  serverResult=actual_result, clientResult=actual_result, attackerFactor=1,
                  defenderFactor=1, maxRound=start.maxRoundNum)
    out.attacker.CopyFrom(start.battlePlayer); out.defender2.CopyFrom(start.monster)
    out.optRecord.CopyFrom(end.optRecord)
    proof = {'fightId': fight_id, 'playerId': 4, 'serverId': 1, 'type': kind, 'chapter': chapter,
             'seed': start.randSeed, 'nativeIsWin': end.isWin, 'playbackResult': actual_result,
             'rounds': end.optRecord.finalRoundNum, 'operations': len(end.optRecord.skillOpts),
             'startSha256': hashlib.sha256(start_raw).hexdigest(), 'endSha256': hashlib.sha256(end_raw).hexdigest(),
             'settlementResponseSha256': hashlib.sha256(response_raw).hexdigest()}
    return out, proof


def read_rows(container, database):
    if not re.fullmatch(r"[a-zA-Z0-9_.-]+", container) or not re.fullmatch(r"[a-zA-Z0-9_]+", database):
        raise ValueError("Invalid Docker/MySQL identifier")
    ids = ','.join("'" + value + "'" for value in CASES)
    sql = ("START TRANSACTION READ ONLY; SELECT fight_id,player_id,type,chapter_id,settled,"
           "HEX(start_snapshot),HEX(end_request),HEX(response) FROM daily_copy_fights "
           "WHERE player_id=4 AND fight_id IN (" + ids + ") ORDER BY fight_id; COMMIT;")
    command = ['rtk', 'proxy', 'docker', 'exec', '-i', container, 'sh', '-c',
               'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --database="$1" --batch --raw --skip-column-names',
               'native-daily-readonly', database]
    # Capture credentials only inside Docker's existing environment; do not
    # return raw stderr or session/profile data from the query to the caller.
    result = subprocess.run(command, input=sql, text=True, capture_output=True, timeout=30)
    if result.returncode:
        raise RuntimeError("Read-only Daily query failed; check the selected local MySQL container")
    rows = [line.split('\t') for line in result.stdout.splitlines() if line]
    if len(rows) != len(CASES) or any(len(row) != 8 for row in rows):
        raise ValueError("The two original completed Daily records are not both present")
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--container', default='hyjs-mysql')
    parser.add_argument('--database', default='hyjs')
    parser.add_argument('--output', type=Path, default=ROOT / 'analysis/p6/native-daily-playback-20261014')
    args = parser.parse_args()
    exports = []
    for row in read_rows(args.container, args.database):
        fixture = json.loads((ROOT / 'analysis/p6' / CASES[row[0]][2]).read_text())
        body, proof = project(row, fixture)
        exports.append((body, proof))
    args.output.mkdir(parents=True, exist_ok=True)
    for body, proof in exports:
        raw = body.SerializeToString(); frame = encode_response(BY_NAME['SCFightPlayback'], body)
        request = message('CSFightPlayback', fightId=proof['fightId'], serverId=1).SerializeToString()
        for suffix, value in [('SCFightPlayback.pb', raw), ('SCFightPlayback.frame', frame), ('CSFightPlayback.pb', request)]:
            (args.output / (proof['fightId'] + '.' + suffix)).write_bytes(value)
        proof['responseSha256'] = hashlib.sha256(raw).hexdigest()
        proof['requestOpcode'] = BY_NAME['CSFightPlayback']; proof['responseOpcode'] = BY_NAME['SCFightPlayback']
    manifest = {'source': 'actual completed owned daily_copy_fights SELECT in read-only transaction',
                'records': [proof for _, proof in exports], 'resourcesModified': False, 'recordsModified': False,
                'serveStatus': 'Protocol exported; authenticated diagnostic lookup/relay and original device playback remain root integration',
                'nativeCompared': False, 'authoritative': False}
    (args.output / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    print('Read-only original Daily playback protocol exported: two original completed losses; no rows/profile/session changed')


if __name__ == '__main__':
    main()
