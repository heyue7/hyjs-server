"""Original HTTP/TCP movement and heartbeat against an isolated MySQL server."""
import concurrent.futures
import http.client
import json
import os
from pathlib import Path
import re
import selectors
import socket
import struct
import subprocess
import tempfile
import time
import unittest

from server.codec import BY_NAME, decode_responses, encode_http_request, message


ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(
    os.environ.get("HYJS_MYSQL_TEST_DATABASE", "").startswith("hyjs_test_")
    and os.environ.get("HYJS_GO_TEST_BINARY"),
    "requires an isolated MySQL database and a temporary server binary",
)
class CpuOptimizationMySQLWireTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="hyjs-cpuopt-wire-")
        self.addCleanup(self.temp.cleanup)

    def start_server(self):
        env = dict(os.environ, HYJS_MYSQL_DATABASE=os.environ["HYJS_MYSQL_TEST_DATABASE"])
        env.setdefault("HYJS_CONFIG_FILE", str(ROOT / "hyjs-server/config.example.yaml"))
        env.setdefault("HYJS_LOG_FILE", str(Path(self.temp.name) / "app.log"))
        env.setdefault("HYJS_GORM_LOG_FILE", str(Path(self.temp.name) / "sql.log"))
        self.process = subprocess.Popen(
            [os.environ["HYJS_GO_TEST_BINARY"], "--http-port", "0", "--tcp-port", "0",
             "--tables", str(ROOT / "analysis/tables")],
            cwd=ROOT / "hyjs-server", env=env, stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT, text=True,
        )
        self.addCleanup(self.stop_server)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), "isolated server did not start")
            line = self.process.stdout.readline()
        match = re.search(r"HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)", line)
        self.assertIsNotNone(match, line)
        self.http_port, self.tcp_port = map(int, match.groups())

    def stop_server(self):
        process = getattr(self, "process", None)
        if process and process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
                self.fail("isolated server failed to stop")
        if process and process.stdout:
            process.stdout.close()

    def request(self, name, fields=None, login=None):
        options = {"zone_id": 1, "timestamp": int(time.time() * 1000)}
        if login is not None:
            options.update(player_id=login.playerId, session=bytes(login.sessionKey))
        packet = encode_http_request(BY_NAME[name], message(name, **(fields or {})), **options)
        conn = http.client.HTTPConnection("127.0.0.1", self.http_port, timeout=15)
        try:
            conn.request("POST", "/sao/", body=packet, headers={"Content-Type": "application/octet-stream"})
            reply = conn.getresponse()
            body = reply.read()
            self.assertEqual(reply.status, 200)
            frames = decode_responses(body)
            self.assertTrue(frames)
            return frames[0]
        finally:
            conn.close()

    def login(self, uid):
        op, response = self.request("CSUserLogin", {"platformUid": uid, "authCode": "local-test-password", "clientTime": 1})
        self.assertEqual(op, BY_NAME["SCUserLogin"], str(response))
        return response

    def register(self, uid, index):
        body = json.dumps({"nickname": "隔离账号" + str(index), "username": uid,
                           "password": "local-test-password", "confirmPassword": "local-test-password"})
        conn = http.client.HTTPConnection("127.0.0.1", self.http_port, timeout=15)
        try:
            conn.request("POST", "/game-account/register", body=body, headers={"Content-Type": "application/json"})
            reply = conn.getresponse()
            result = json.loads(reply.read())
            self.assertEqual(reply.status, 200, str(result))
            self.assertTrue(result.get("data", {}).get("registered"), str(result))
        finally:
            conn.close()

    def tcp_send(self, sock, login, name, fields):
        payload = message(name, **fields).SerializeToString()
        sock.sendall(struct.pack(">iiiiqi", 24 + len(payload), 1, BY_NAME[name], 1, login.playerId, len(payload)) + payload)

    def tcp_receive(self, reader, expected):
        for _ in range(32):
            header = reader.read(8)
            self.assertEqual(len(header), 8, "TCP session closed")
            size, _ = struct.unpack(">ii", header)
            opcode, response = decode_responses(header + reader.read(size))[0]
            if opcode == BY_NAME[expected] or opcode == BY_NAME["ErrorMessage"]:
                self.assertEqual(opcode, BY_NAME[expected], str(response))
                return response
        self.fail("expected TCP frame was not received")

    def test_parallel_heartbeat_move_and_cold_recovery(self):
        self.start_server()
        users = []
        suffix = str(time.time_ns())
        for index in range(2):
            uid = "cpuopt-" + suffix[-12:] + "-" + str(index)
            self.register(uid, index)
            login = self.login(uid)
            op, created = self.request("CSPlayerCreate", {"playerName": "隔离并发" + str(index), "heroId": "hero_043"}, login)
            self.assertEqual(op, BY_NAME["SCPlayerCreate"], str(created))
            users.append((uid, login))

        def move_and_ping(item):
            index, (_, login) = item
            with socket.create_connection(("127.0.0.1", self.tcp_port), timeout=15) as sock:
                with sock.makefile("rb") as reader:
                    self.tcp_send(sock, login, "CSSocketLogin", {"sessionKey": login.sessionKey})
                    self.assertEqual(self.tcp_receive(reader, "SCSocketLogin").result, 100)
                    self.tcp_send(sock, login, "CSLoginScene", {})
                    self.tcp_receive(reader, "SCLoginScene")
                    self.tcp_send(sock, login, "CSCurrentPosition", {})
                    self.tcp_receive(reader, "SCCurrentPosition")
                    for step in range(3):
                        self.tcp_send(sock, login, "CSMove", {
                            "pointNew": {"x": float(10 + index * 10 + step), "y": 0, "z": float(index)},
                            "currentTime": int(time.time() * 1000),
                        })
                        self.tcp_send(sock, login, "CSPingTcp", {"clientTime": step + 1})
                        self.assertEqual(self.tcp_receive(reader, "SCPingTcp").clientTime, step + 1)

        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            list(pool.map(move_and_ping, enumerate(users)))
        for index, (_, login) in enumerate(users):
            op, current = self.request("CSCurrentPosition", login=login)
            self.assertEqual(op, BY_NAME["SCCurrentPosition"], str(current))
            self.assertAlmostEqual(current.self.position.current.x, float(12 + index * 10))

        self.stop_server()
        self.start_server()
        for index, (uid, _) in enumerate(users):
            login = self.login(uid)
            op, current = self.request("CSCurrentPosition", login=login)
            self.assertEqual(op, BY_NAME["SCCurrentPosition"], str(current))
            self.assertAlmostEqual(current.self.position.current.x, float(12 + index * 10))
