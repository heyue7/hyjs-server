"""Pure contracts for pre-cleanup evidence; no local services are accessed."""
import json
import hashlib
import hmac
from pathlib import Path
import tempfile
import unittest
import p14_final_rounds_diagnostics as diagnostics


class P14FinalRoundsDiagnosticsTests(unittest.TestCase):
    def test_only_owned_read_only_sources_and_exact_hex_are_exported(self):
        class Case:
            realms = {1: "hyjs_test_mock"}
            queries = []
            def sql(self, realm, query):
                self.queries.append(query)
                if "information_schema.tables" in query:
                    return "p14_final_series_slots\np14_trusted_peers\nprofiles\np14_final_bad\np6_battle_records\n"
                if "information_schema.columns" in query:
                    return "id\nsigning_key\n" if "p14_final_bad" in query else "id\ninput\n"
                if "COUNT(*)" in query:
                    return "1\n"
                return '{"id":"31","input":"00FF0A"}\n'
        case = Case()
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / diagnostics.export(case, temporary, "failed actual slot")
            report = json.loads((path / "manifest.json").read_text())
            self.assertEqual(len(report["files"]), 2)
            row = json.loads((path / "realm1-p14_final_series_slots.jsonl").read_text())
            self.assertEqual(bytes.fromhex(row["input"]), b"\x00\xff\n")
            self.assertTrue(report["realms"]["1"]["p6_battle_records"]["complete"])
            self.assertIn("excluded", report["realms"]["1"]["p14_final_bad"])
        self.assertTrue(all(query.startswith("SELECT ") for query in case.queries))
        self.assertTrue(all("HEX(CAST(" in query for query in case.queries if "JSON_OBJECT(" in query))
        self.assertFalse(any("FROM `profiles`" in query or "FROM `p14_trusted_peers`" in query or "FROM `p14_final_bad`" in query for query in case.queries))

    def test_export_failure_is_redacted_and_does_not_raise(self):
        class Case:
            realms = {1: "hyjs_test_mock"}
            def sql(self, realm, query):
                raise RuntimeError("private password must not be retained")
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / diagnostics.export(Case(), temporary, "teardown")
            text = (path / "manifest.json").read_text()
            self.assertNotIn("private password", text)
            self.assertEqual(json.loads(text)["realms"]["1"]["schemaError"], "RuntimeError")

    def test_optional_probe_keeps_exact_original_payload_and_redacts_reply(self):
        payload = b'{"sourceServerId":1,"destinationServerId":2,"deliveryId":"actual-queued-id","snapshot":{"original":true}}'
        key = b"owned-diagnostic-key-not-for-output"
        class Case:
            realms = {1: "hyjs_test_mock"}
            ports = {1: (10001, 20001), 2: (10002, 20002)}
            signing_key = key
            def sql(self, realm, query):
                self.query = query
                return payload.hex()
            def source_constant(self, file, name):
                return "/internal/p14/next-round-formation"
        class Connection:
            closed = False
            def __init__(self, host, port, timeout):
                self.endpoint = (host, port, timeout)
            def request(self, method, path, body, headers):
                self.sent = (method, path, body, headers)
            def getresponse(self):
                class Response:
                    status = 200
                    def read(self, maximum):
                        return b'{"code":0,"data":{"confirmed":true},"msg":"private reply deliberately excluded"}'
                return Response()
            def close(self):
                self.closed = True
        case = Case()
        captured = []
        def factory(*args, **kwargs):
            connection = Connection(*args, **kwargs)
            captured.append(connection)
            return connection
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / diagnostics.probe_signed_transport(case, temporary, factory)
            text = path.read_text()
            report = json.loads(text)
            self.assertTrue(report["attempts"][0]["confirmed"])
            self.assertNotIn("private reply", text)
            self.assertNotIn(key.decode(), text)
            self.assertNotIn(hmac.new(key, payload, hashlib.sha256).hexdigest(), text)
        self.assertEqual(captured[0].sent[2], payload)
        self.assertEqual(captured[0].sent[3]["X-HYJS-P14-Signature"], hmac.new(key, payload, hashlib.sha256).hexdigest())
        self.assertEqual(captured[0].endpoint, ("127.0.0.1", 10002, 30))
        self.assertTrue(captured[0].closed)
        self.assertTrue(case.query.startswith("SELECT HEX(payload)"))
