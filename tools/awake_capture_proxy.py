"""Bounded local HTTP forwarding: save one original account4 Awake End body only.

No authentication header, request frame, unrelated body, secret or login is written.
The phone owner coordinates its existing reverse mapping and restores it before
stopping this proxy. This tool never operates ADB, changes game state or synthesizes
an End. It forwards original bytes to the unchanged running service.
"""
import argparse
import hashlib
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import signal
import sys
import threading
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from server.codec import message, parse_http, decode_responses


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', action='store_true')
    parser.add_argument('--start-pb', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--ttl-seconds', type=int, default=180)
    args = parser.parse_args()
    if not args.run:
        parser.print_help()
        return
    if not 30 <= args.ttl_seconds <= 300:
        parser.error('Use a bounded 30..300 second window')
    start = message('SCAwakeCopyFightBegin')
    start.ParseFromString(args.start_pb.read_bytes())
    if not start.fightId or start.battlePlayer.playerId != 4:
        parser.error('An original account4 Awake Begin is required')
    output = args.output_directory.resolve()
    if not output.is_relative_to(ROOT / 'analysis') or output.exists():
        parser.error('Use a fresh analysis directory')
    output.mkdir(mode=0o700, parents=True)
    state = {'captured': False, 'forwarded': 0}
    lock = threading.Lock()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            try:
                size = int(self.headers.get('Content-Length', '0'))
                if self.path != '/sao/' or self.headers.get('Transfer-Encoding') or not 68 <= size <= 8 * 1024 * 1024:
                    self.send_error(400)
                    return
                raw = self.rfile.read(size)
                if len(raw) != size:
                    self.send_error(400)
                    return
                request = parse_http(raw)
                candidate = request.opcode == 81011 and request.player_id == 4
                end = None
                if candidate:
                    end = request.decode()
                    candidate = end.fightId == start.fightId
                connection = http.client.HTTPConnection('127.0.0.1', 8088, timeout=20)
                try:
                    connection.request('POST', '/sao/', raw, {'Content-Type': 'application/octet-stream'})
                    response = connection.getresponse()
                    result = response.read()
                    status = response.status
                finally:
                    connection.close()
                with lock:
                    state['forwarded'] += 1
                    if candidate and not state['captured']:
                        # Store only the already decoded protobuf business body.
                        # Never retain the signed envelope or session bytes.
                        body = request.payload
                        fd = os.open(output / 'original-End.pb', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                        with os.fdopen(fd, 'wb') as file:
                            file.write(body)
                        decoded = decode_responses(result)
                        callback = decoded[0][1] if decoded else None
                        record = {'playerId': 4, 'opcode': 81011,
                                  'originalEndSHA256': hashlib.sha256(body).hexdigest(),
                                  'startSHA256': hashlib.sha256(args.start_pb.read_bytes()).hexdigest(),
                                  'receivedAtUnix': time.time(), 'responseStatus': status,
                                  'responseOpcode': decoded[0][0] if decoded else None,
                                  'responseCode': getattr(callback, 'code', None),
                                  'responseMessage': getattr(callback, 'msg', None),
                                  'credentialEnvelopeSaved': False,
                                  'forwardedOriginalBytesUnchanged': True}
                        (output / 'record.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
                        state['captured'] = True
                        print(json.dumps({'capturedOriginalEnd': True, 'outputDirectory': str(output)}), flush=True)
                self.send_response(status)
                self.send_header('Content-Type', 'application/octet-stream')
                self.send_header('Content-Length', str(len(result)))
                self.end_headers()
                self.wfile.write(result)
            except Exception:
                # Exceptions can contain network arguments: never echo them.
                self.send_error(502, 'Original local forwarding failed')

    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    server.timeout = .5
    server.daemon_threads = True
    stopped = False

    def stop(signum, frame):
        nonlocal stopped
        stopped = True
    signal.signal(signal.SIGINT, stop)
    signal.signal(signal.SIGTERM, stop)
    print(json.dumps({'ready': True, 'port': server.server_port,
                      'ttlSeconds': args.ttl_seconds, 'targetPlayerId': 4,
                      'forwardTarget': '127.0.0.1:8088', 'outputDirectory': str(output)}), flush=True)
    deadline = time.monotonic() + args.ttl_seconds
    try:
        while not stopped and time.monotonic() < deadline:
            server.handle_request()
    finally:
        server.server_close()
        print(json.dumps({'closed': True, **state, 'credentialEnvelopeSaved': False}), flush=True)


if __name__ == '__main__':
    main()
