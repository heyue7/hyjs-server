"""Actual180 checks after two adjacent, truly completed177 parents exist.

This plugin allocates nothing and inserts no qualification/source/result rows.
The owner must publish its local policy/calendar before the original signup and
hold the managed SQL lease. An8qualifier group is needed for its first stage12
series; the prior4qualifier prefix correctly cannot create one.
"""
import hashlib
import json
from pathlib import Path
import re
import time
from concurrent.futures import ThreadPoolExecutor

from server.codec import CLASSES


def _scope(series_id, index, realm=None, pid=None):
    if not re.fullmatch(r"[0-9a-f]{64}", series_id) or index not in range(1, 6):
        raise AssertionError("actual180 series and current game required")
    query = " WHERE series_id='" + series_id + "' AND game_index=" + str(index)
    if realm is not None:
        if realm not in (1, 2) or pid <= 0:
            raise AssertionError("actual allocated joint identity required")
        query += " AND server_id=" + str(realm) + " AND player_id=" + str(pid)
    return query


def read_actual(runner, observer, realm, owner, series_id, index):
    where = _scope(series_id, index, realm, owner.playerId)
    fields = ("'seriesId',series_id,'gameIndex',game_index,'serverId',server_id,'playerId',player_id,"
              "'revision',revision,'capturedAt',captured_at,'offerHash',offer_hash,'sourceHash',source_hash,"
              "'snapshotHash',snapshot_hash,'fightPower',fight_power,'configurationHash',configuration_hash,"
              "'engineVersion',engine_version,'clientVersion',client_version,'battleHEX',HEX(battle),'frameHEX',HEX(snapshot)")
    row = json.loads(runner.sql(observer, "SELECT JSON_OBJECT(" + fields + ") FROM p14_final_round_formations" + where + ";").strip())
    battle_raw, frame_raw = bytes.fromhex(row.pop("battleHEX")), bytes.fromhex(row.pop("frameHEX"))
    battle, frame = CLASSES["BattlePlayer"](), CLASSES["FormationSimpleInfo"]()
    battle.ParseFromString(battle_raw); frame.ParseFromString(frame_raw)
    runner.assertEqual((battle.serverId, battle.playerId), (realm, owner.playerId))
    runner.assertEqual(row["fightPower"], frame.fightPower)
    runner.assertEqual((row["seriesId"], row["gameIndex"]), (series_id, index))
    row.update(battleSHA256=hashlib.sha256(battle_raw).hexdigest(), formationSHA256=hashlib.sha256(frame_raw).hexdigest())
    return row, frame, battle_raw


def capture_actual(runner, realm, owner, series_id, index, stamp):
    where = _scope(series_id, index, realm, owner.playerId)
    previous = int(runner.sql(realm, "SELECT COALESCE(MAX(revision),0) FROM p14_final_round_formations" + where + ";").strip())
    with ThreadPoolExecutor(max_workers=8) as pool:
        replies = list(pool.map(lambda _: runner.call(realm, "CSKFEnterFrightGame", {}, owner, stamp, "SCKFEnterFrightGame"), range(8)))
    runner.assertTrue(all(reply == replies[0] for reply in replies))
    row, frame, battle = read_actual(runner, realm, realm, owner, series_id, index)
    runner.assertEqual(row["revision"], previous + 1)
    runner.assertEqual(replies[0].lastFightPower, row["fightPower"])
    peer = 3 - realm
    runner.wait_sql(peer, "SELECT source_hash FROM p14_final_round_formations" + where + ";", row["sourceHash"])
    imported, remote_frame, remote_battle = read_actual(runner, peer, realm, owner, series_id, index)
    runner.assertEqual((imported, remote_frame, remote_battle), (row, frame, battle))
    ack = ("SELECT COUNT(*) FROM p14_final_round_deliveries WHERE inbound=0 AND confirmed=1 AND destination_server_id=" + str(peer)
           + " AND JSON_UNQUOTE(JSON_EXTRACT(CONVERT(payload USING utf8mb4),'$.snapshot.formation.sourceHash'))='" + row["sourceHash"] + "';")
    runner.wait_sql(realm, ack, "1")
    current = runner.call(peer, "CSKFPlayerInfo", {"type": 3, "serverId": realm, "playerId": owner.playerId, "replayFlag": 0}, runner.current_round_viewers[peer], stamp + 1, "SCKFPlayerInfo")
    runner.assertEqual(current.formationInfo, frame)
    return row


def run_actual(runner, players, identities, series_id, evidence_dir):
    """Consume one actual remaining-round series with new captures each game."""
    _scope(series_id, 1)
    common = " WHERE id='" + series_id + "'"
    raw_manifest = bytes.fromhex(runner.sql(1, "SELECT HEX(manifest) FROM p14_final_round_series" + common + ";").strip())
    runner.assertEqual(runner.sql(2, "SELECT HEX(manifest) FROM p14_final_round_series" + common + ";").strip(), raw_manifest.hex().upper())
    manifest = json.loads(raw_manifest)
    offer = json.loads(bytes(manifest["offer"]["content"]) if isinstance(manifest["offer"]["content"], list) else __import__("base64").b64decode(manifest["offer"]["content"]))
    pair, scheduled = manifest["pair"], manifest["slots"]
    runner.assertIn(offer["roundStage"], (12, 14, 16, 18))
    runner.assertEqual(len(offer["parents"]), 2)
    runner.assertTrue(all(len(parent["reference"]["lastCanonicalResultHash"]) == 64 for parent in offer["parents"]))
    runner.assertEqual([(entry["serverId"], entry["playerId"]) for entry in offer["entrants"]],
                       [(pair["leftServerId"], pair["leftPlayerId"]), (pair["rightServerId"], pair["rightPlayerId"])])
    canonical = pair["leftServerId"]
    origins = sorted({canonical, pair["rightServerId"]})
    runner.current_round_viewers = {zone: players[zone][0] for zone in (1, 2)}
    destination = Path(evidence_dir); destination.mkdir(parents=True, exist_ok=False)
    history, sources, seen_inputs = [], [], set()
    for index in range(1, 6):
        state = int(runner.sql(1, "SELECT state FROM p14_final_round_series" + common + ";").strip())
        if state == 3:
            break
        opening = offer["openAt"] if index == 1 else scheduled[index - 2]
        runner.clock.write_text(str(opening + 1000))
        for realm, pid in ((pair["leftServerId"], pair["leftPlayerId"]), (pair["rightServerId"], pair["rightPlayerId"])):
            sources.append(capture_actual(runner, realm, identities[(realm, pid)], series_id, index, 10000 + index * 1000 + realm * 100 + pid))
        runner.clock.write_text(str(scheduled[index - 1] - 300000))
        for realm in (1, 2):
            runner.wait_sql(realm, "SELECT COUNT(*) FROM p14_final_round_seals" + _scope(series_id, index) + ";", str(len(origins)))
        runner.clock.write_text(str(scheduled[index - 1] + 1))
        for realm in (1, 2):
            runner.wait_sql(realm, "SELECT left_wins+right_wins FROM p14_final_round_series" + common + ";", str(index), timeout=45)
        raw = bytes.fromhex(runner.sql(1, "SELECT HEX(input) FROM p14_final_round_slots" + _scope(series_id, index) + ";").strip())
        runner.assertEqual(runner.sql(2, "SELECT HEX(input) FROM p14_final_round_slots" + _scope(series_id, index) + ";").strip(), raw.hex().upper())
        replay = CLASSES["KFReplayData"](); replay.ParseFromString(raw)
        runner.assertNotIn(replay.warId, seen_inputs); seen_inputs.add(replay.warId)
        runner.assertEqual([(p.serverId, p.playerId) for p in replay.playerList], [(pair["leftServerId"], pair["leftPlayerId"]), (pair["rightServerId"], pair["rightPlayerId"])])
        runner.assertFalse(replay.HasField("winPlayerId")); runner.assertFalse(replay.HasField("score")); runner.assertFalse(replay.HasField("roomId"))
        for fighter in replay.playerList:
            actual, _, battle = read_actual(runner, 1, fighter.serverId, identities[(fighter.serverId, fighter.playerId)], series_id, index)
            saved = CLASSES["BattlePlayer"](); saved.ParseFromString(battle); runner.assertEqual(fighter, saved)
        (destination / ("game" + str(index) + ".KFReplayData.pb")).write_bytes(raw)
        hashes = [runner.sql(realm, "SELECT certificate_hash FROM p14_final_round_slots" + _scope(series_id, index) + ";").strip() for realm in (1, 2)]
        runner.assertEqual(hashes[0], hashes[1])
        history.append({"gameIndex": index, "inputSHA256": hashlib.sha256(raw).hexdigest(), "warId": replay.warId, "certificateHash": hashes[0]})
    fields = "'state',state,'leftWins',left_wins,'rightWins',right_wins,'winnerServerId',winner_server_id,'winnerPlayerId',winner_player_id,'currentGame',current_game"
    query = "SELECT JSON_OBJECT(" + fields + ") FROM p14_final_round_series" + common + ";"
    finals = [json.loads(runner.sql(realm, query).strip()) for realm in (1, 2)]
    runner.assertEqual(finals[0], finals[1]); final = finals[0]
    runner.assertEqual(final["state"], 3); runner.assertIn(3, (final["leftWins"], final["rightWins"]))
    runner.assertEqual(final["leftWins"] + final["rightWins"], len(history))
    for realm in (1, 2):
        owned = "SELECT COUNT(*) FROM p6_battle_records WHERE source='p14-rounds180-kf' AND source_fight IN (SELECT input_id FROM p14_final_round_slots WHERE series_id='" + series_id + "');"
        runner.assertEqual(int(runner.sql(realm, owned).strip()), len(history) if realm == canonical else 0)
        runner.assertEqual(runner.sql(realm, "SELECT COUNT(*) FROM p14_final_round_slots WHERE series_id='" + series_id + "' AND game_index>" + str(len(history)) + " AND (state<>4 OR LENGTH(input)>0 OR LENGTH(certificate)>0);").strip(), "0")
        fixed = runner.ports[realm]; runner.stop(realm); runner.start(realm, fixed)
        runner.assertEqual(json.loads(runner.sql(realm, query).strip()), final)
    report = {"status": "PASS_ACTUAL_REMAINING_ROUND_SERIES_COLD", "seriesId": series_id,
              "stage": offer["roundStage"], "pair": pair, "actualGameHistory": history, "actualFreshSources": sources,
              "final": final, "scope": "actual full parent/source prefix required; no semifinal pairing/bye/champion/prize inference"}
    (destination / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    return report
