"""Preserve owned series evidence before managed allocation cleanup.

Only read-only, schema-local test SQL is used. Credentials, sessions, profiles
and peer keys are outside the table allowlist. Every cell is hex encoded so
binary protobufs and exact signed JSON survive the mysql text transport.
"""
import hashlib
import hmac
import http.client
import json
from pathlib import Path
import re
import time


def probe_signed_transport(case, root, connection_factory=None):
    """Retry at most one genuine queued formation per realm with a longer read.

    This optional diagnostic can persist the same authenticated source. It does
    not forge a new source or acknowledge a timeout as success. Call only after
    preserving the untouched failed-state export, while both owned apps exist.
    """
    connection_factory = connection_factory or http.client.HTTPConnection
    report = {"scope": "diagnostic exact queued source retry, may persist genuine original import; not production3s transport proof", "attempts": []}
    for realm in sorted(getattr(case, "realms", {})):
        attempt = {"sourceRealm": realm}
        try:
            raw = case.sql(realm, "SELECT HEX(payload) FROM p14_next_round_deliveries WHERE confirmed=0 ORDER BY created_at,id LIMIT 1;").strip()
            if not raw:
                continue
            payload = bytes.fromhex(raw)
            envelope = json.loads(payload)
            destination = envelope["destinationServerId"]
            if envelope["sourceServerId"] != realm or destination == realm or destination not in case.ports:
                raise ValueError("queued destination outside owned apps")
            path = case.source_constant("p14NextRoundFormation.go", "P14NextRoundFormationImportPath")
            signature = hmac.new(case.signing_key, payload, hashlib.sha256).hexdigest()
            attempt.update(destinationRealm=destination, deliveryID=envelope["deliveryId"], payloadSHA256=hashlib.sha256(payload).hexdigest())
            connection = connection_factory("127.0.0.1", case.ports[destination][0], timeout=30)
            began = time.monotonic()
            try:
                connection.request("POST", path, body=payload, headers={"Content-Type": "application/json", "X-HYJS-P14-Signature": signature})
                response = connection.getresponse()
                body = response.read(1048577)
                attempt.update(httpStatus=response.status, seconds=round(time.monotonic()-began, 3), responseBytes=len(body))
                decoded = json.loads(body)
                attempt["bizCode"] = decoded.get("code")
                attempt["confirmed"] = bool(decoded.get("data", {}).get("confirmed")) if isinstance(decoded.get("data"), dict) else False
                # Keep no message/body/header that could expose diagnostics or keys.
            finally:
                connection.close()
        except Exception as error:
            attempt["probeError"] = type(error).__name__
        report["attempts"].append(attempt)
    path = Path(root) / "analysis/p14" / ("final-rounds-transport-probe-" + str(time.time_ns()) + ".json")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    return str(path.relative_to(Path(root)))


def export(case, root, label):
    """Capture current genuine rows without repairing or advancing a series."""
    evidence = Path(root) / "analysis/p14" / ("final-rounds-state-" + str(time.time_ns()))
    evidence.mkdir(parents=True, exist_ok=False)
    report = {"label": label, "readOnly": True, "snapshot": "live sequential reads, not an atomic cross-table snapshot", "pendingRead": getattr(case, "pending_read", None), "encoding": "each non-null cell is HEX(CAST(column AS BINARY)); numeric text and binary bytes are exact; null remains null", "realms": {}, "files": {}, "stateSummary": {}}
    for realm in sorted(getattr(case, "realms", {})):
        exported = report["realms"][str(realm)] = {}
        summary = report["stateSummary"][str(realm)] = {}
        try:
            tables = case.sql(realm, "SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() ORDER BY table_name;").splitlines()
        except Exception as error:
            exported["schemaError"] = type(error).__name__
            continue
        for table in tables:
            # These families contain frozen game sources and result journals.
            # Keep authentication/profile/peer-key tables strictly excluded.
            if not re.fullmatch(r"(?:p14_[a-z0-9_]*|p6_battle_records)", table) or table == "p14_trusted_peers":
                continue
            try:
                columns = case.sql(realm, "SELECT column_name FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='" + table + "' ORDER BY ordinal_position;").splitlines()
                if not columns or any(not re.fullmatch(r"[a-z][a-z0-9_]*", column) or re.search(r"password|session|signing_key|auth_code|token", column) for column in columns):
                    exported[table] = {"excluded": "unexpected or credential-bearing schema"}
                    continue
                fields = ",".join("'" + column + "',HEX(CAST(`" + column + "` AS BINARY))" for column in columns)
                raw = case.sql(realm, "SELECT JSON_OBJECT(" + fields + ") FROM `" + table + "` LIMIT 10000;")
                # Parse before writing to reject mysql warnings or truncated rows.
                rows = [json.loads(line) for line in raw.splitlines() if line]
                count = int(case.sql(realm, "SELECT COUNT(*) FROM `" + table + "`;").strip())
                name = "realm" + str(realm) + "-" + table + ".jsonl"
                data = ("\n".join(json.dumps(row, ensure_ascii=False, sort_keys=True) for row in rows) + ("\n" if rows else "")).encode()
                (evidence / name).write_bytes(data)
                report["files"][name] = hashlib.sha256(data).hexdigest()
                exported[table] = {"file": name, "rows": len(rows), "observedCount": count, "complete": count == len(rows)}
                if table.endswith(("_series", "_slots", "_results", "_receipts", "_deliveries")):
                    # This compact view identifies a producer/peer discrepancy;
                    # the full hex rows remain the exact authoritative evidence.
                    keys = ("id", "series_id", "game_index", "current_game", "state", "left_wins", "right_wins", "winner_server_id", "winner_player_id", "input_id", "input_hash", "source_result_hash", "scope_proof", "p6_record_id", "destination_server_id", "confirmed", "attempt_count", "last_attempt_at")
                    summary[table] = [{key: bytes.fromhex(row[key]).decode("utf-8") if row[key] is not None else None for key in keys if key in row} for row in rows]
            except Exception as error:
                # Do not retain exception text which may echo SQL or data.
                exported[table] = {"exportError": type(error).__name__}
    (evidence / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    return str(evidence.relative_to(Path(root)))
