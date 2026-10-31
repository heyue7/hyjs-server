"""Exercise semantic update policies against an isolated MySQL HTTP/TCP server.

Set HYJS_TEST_BINARY to the compiled candidate. This test allocates and releases
its own schema using the shared local-services manager and never contacts the
production server.
"""

import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import socket
import struct
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import uuid

from hyjs_managed_mysql import ManagedMySQL

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from server.codec import BY_NAME, CLASSES, decode_responses, encode_http_request


def free_port():
    with socket.socket() as probe:
        probe.bind(('127.0.0.1', 0))
        return probe.getsockname()[1]


class Client:
    def __init__(self, http_port, tcp_port):
        self.origin = f'http://127.0.0.1:{http_port}'
        self.tcp_port = tcp_port
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.csrf = ''

    def api(self, path, body=None):
        headers = {'Content-Type': 'application/json', 'Origin': self.origin, 'X-CSRF-Token': self.csrf}
        request = urllib.request.Request(self.origin + path, None if body is None else json.dumps(body).encode(), headers=headers)
        with self.opener.open(request, timeout=20) as response:
            result = json.load(response)
        if result['code'] != 0:
            raise AssertionError(f'{path}: {result.get("msg", result.get("message"))}')
        return result['data']

    def wire(self, name, fields, player=0, session=bytes(16)):
        request = encode_http_request(BY_NAME[name], CLASSES[name](**fields), player_id=player, zone_id=1,
                                      session=session, timestamp=int(time.time() * 1000))
        with urllib.request.urlopen(urllib.request.Request(self.origin + '/sao/', request,
                                    headers={'Content-Type': 'application/octet-stream'}), timeout=20) as response:
            return [body for _, body in decode_responses(response.read())]

    @staticmethod
    def expected(messages, name):
        found = [body for body in messages if body.DESCRIPTOR.name == name]
        assert len(found) == 1, [body.DESCRIPTOR.name for body in messages]
        return found[0]

    @staticmethod
    def exact(connection, length):
        result = b''
        while len(result) < length:
            part = connection.recv(length - len(result))
            assert part, 'TCP closed before the complete response'
            result += part
        return result

    def tcp_login(self, login):
        connection = socket.create_connection(('127.0.0.1', self.tcp_port), timeout=10)
        payload = CLASSES['CSSocketLogin'](sessionKey=login.sessionKey).SerializeToString()
        connection.sendall(struct.pack('>iiiiqi', 24 + len(payload), 1, BY_NAME['CSSocketLogin'], 1,
                                       login.playerId, len(payload)) + payload)
        header = self.exact(connection, 8)
        size, _ = struct.unpack('>ii', header)
        messages = [body for _, body in decode_responses(header + self.exact(connection, size))]
        return connection, messages

    def release(self, version, mode):
        state = self.api('/admin/api/client-updates')
        now = int(time.time() * 1000)
        body = {'id': 0, 'version': state['version'], 'versionPolicy': True, 'versionName': version,
                'buildCode': 0, 'minLoginBuild': 0, 'minRunBuild': 0, 'mode': mode,
                'title': '隔离版本验收', 'content': '仅用于本机测试。',
                'maintenanceStartAt': now + 600000 if mode == 'maintenance' else 0,
                'maintenanceEndAt': now + 1200000 if mode == 'maintenance' else 0,
                'reason': '隔离测试草稿'}
        return self.api('/admin/api/client-updates/draft', body)['items'][0]['id']

    def command(self, release, action):
        state = self.api('/admin/api/client-updates')
        body = {'id': release, 'version': state['version'], 'requestId': str(uuid.uuid4()),
                'action': action, 'reason': '隔离测试操作'}
        result = self.api('/admin/api/client-updates/action', body)
        assert self.api('/admin/api/client-updates/action', body)['version'] == result['version']
        return result

    def status(self, build, startup, version='', login=None):
        body = {'buildCode': build, 'startupId': startup}
        if version:
            body['clientVersion'] = version
        if login:
            body.update(playerId=login.playerId, sessionHex=login.sessionKey.hex())
        return self.api('/client-update/status', body)


def exercise(client, credentials):
    client.csrf = client.api('/admin/api/auth/login', credentials)['csrfToken']
    original = client.api('/admin/api/client-updates')['policy']
    assert original['versionName'] == '1.1.0' and original['latestBuild'] == 2026110101
    startup = uuid.uuid4().hex
    current_build = 2026110101
    first = client.release('1.1.0', 'notice')
    client.command(first, 'publish')
    same = client.status(current_build, startup)
    assert not same['noticeEnabled'] and not same['blocked'], 'installed 1.1.0 was not recognized'

    username, password = 'sv_' + secrets.token_hex(5), secrets.token_urlsafe(24)
    client.api('/game-account/register', {'username': username, 'nickname': '版本验收',
               'password': password, 'confirmPassword': password})
    login_fields = {'platformUid': username, 'authCode': password,
                    'appVersion': f'hyjs/{current_build}/{startup}', 'clientTime': int(time.time() * 1000)}
    old_login = client.expected(client.wire('CSUserLogin', login_fields), 'SCUserLogin')
    client.expected(client.wire('CSPlayerCreate', {'playerName': '版本验收', 'heroId': 'hero_043'},
                                old_login.playerId, old_login.sessionKey), 'SCPlayerCreate')
    connection, messages = client.tcp_login(old_login)
    assert client.expected(messages, 'SCSocketLogin').result == 100
    try:
        client.command(client.release('1.1.1', 'next_login'), 'publish')
        old_status = client.status(current_build, startup, login=old_login)
        assert old_status['noticeEnabled'] and not old_status['blocked']
        assert old_status['latestBuild'] > current_build, 'installed 1.1.0 could not show the reminder'
        assert not any(m.DESCRIPTOR.name == 'SCUserLogin' for m in client.wire('CSUserLogin', login_fields))
        assert not client.status(current_build, startup, login=old_login)['blocked']
        assert not client.status(2026110102, uuid.uuid4().hex, '1.1.1')['noticeEnabled']
        assert not client.status(2026110103, uuid.uuid4().hex, '1.10.0')['noticeEnabled']

        connection.close()
        time.sleep(0.2)
        connection, messages = client.tcp_login(old_login)
        assert client.expected(messages, 'SCSocketLogin').result == 100, 'short reconnect failed'
        connection.close()
        new_startup = uuid.uuid4().hex
        login_fields['appVersion'] = f'hyjs/2026110102/{new_startup}/1.1.1'
        new_login = client.expected(client.wire('CSUserLogin', login_fields), 'SCUserLogin')
        connection, messages = client.tcp_login(new_login)
        assert client.expected(messages, 'SCSocketLogin').result == 100

        maintenance = client.release('1.1.2', 'maintenance')
        before = client.command(maintenance, 'publish')['policy']
        assert client.status(2026110102, new_startup, '1.1.1', new_login)['noticeEnabled']
        assert client.status(2026110104, uuid.uuid4().hex, '1.1.2')['noticeEnabled']
        assert client.status(2026110105, uuid.uuid4().hex, '1.10.0')['noticeEnabled']
        cancelled = client.command(maintenance, 'cancel_maintenance')['policy']
        assert not cancelled['maintenance'] and not cancelled['maintenanceStartAt'] and not cancelled['noticeEnabled']
        for key in ('minLoginVersion', 'minRunVersion', 'minLoginBuild', 'minRunBuild'):
            assert cancelled[key] == before[key], f'cancelling changed {key}'
        assert not client.status(2026110102, new_startup, '1.1.1', new_login)['blocked']
        history = client.api('/admin/api/client-updates/history?page=1&pageSize=20')['items']
        assert any(item['id'] == maintenance and item['state'] == 'cancelled' for item in history)

        # A cancelled plan leaves the same target version available for rescheduling.
        maintenance = client.release('1.1.2', 'maintenance')
        client.command(maintenance, 'publish')
        reopened = client.command(0, 'server_start')['policy']
        assert not reopened['maintenanceStartAt'] and not reopened['noticeEnabled']
        for key in ('minLoginVersion', 'minRunVersion', 'minLoginBuild', 'minRunBuild'):
            assert reopened[key] == before[key], f'opening an already-open server changed {key}'
        history = client.api('/admin/api/client-updates/history?page=1&pageSize=20')['items']
        assert any(item['id'] == maintenance and item['state'] == 'cancelled' for item in history)

        maintenance = client.release('1.1.2', 'maintenance')
        client.command(maintenance, 'publish')
        client.command(maintenance, 'start_maintenance')
        terminal = b''
        while True:
            part = connection.recv(4096)
            if not part:
                break
            terminal += part
            assert len(terminal) < 131072
        assert terminal, 'maintenance did not notify the connected client'
        assert client.status(2026110102, new_startup, '1.1.1', new_login)['blocked']
        assert client.status(2026110104, uuid.uuid4().hex, '1.1.2')['blocked']
        try:
            client.command(maintenance, 'cancel_maintenance')
        except (AssertionError, urllib.error.HTTPError):
            pass
        else:
            raise AssertionError('an active maintenance was cancelled as if it had not begun')
        client.command(maintenance, 'end_maintenance')
        assert client.status(current_build, startup)['blocked']
        assert client.status(2026110102, new_startup, '1.1.1')['blocked']
        assert not client.status(2026110104, uuid.uuid4().hex, '1.1.2')['blocked']

        older = client.release('1.1.1', 'notice')
        try:
            client.command(older, 'publish')
        except (AssertionError, urllib.error.HTTPError):
            pass
        else:
            raise AssertionError('a lower version was published')
        assert client.api('/admin/api/client-updates')['policy']['versionName'] == '1.1.2'
    finally:
        connection.close()


def wait_ready(process, port):
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError('Isolated server exited before readiness')
        try:
            with urllib.request.urlopen(f'http://127.0.0.1:{port}/health', timeout=1):
                return
        except (OSError, urllib.error.URLError):
            time.sleep(0.2)
    raise RuntimeError('Isolated server did not become ready')


def stop(process):
    if process and process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)


def main():
    binary = Path(os.environ['HYJS_TEST_BINARY']).resolve(strict=True)
    pool = ManagedMySQL()
    process = None
    try:
        connection = pool.allocate('semver_http')
        pool.sql(connection.database, (ROOT / 'tests/fixtures/client_update_legacy_110.sql').read_text())
        with tempfile.TemporaryDirectory(prefix='hyjs-semver-http-') as directory:
            temp = Path(directory)
            credentials = {'username': 'versionqa', 'password': secrets.token_urlsafe(28)}
            bootstrap = temp / 'bootstrap.json'
            bootstrap.write_text(json.dumps(credentials))
            bootstrap.chmod(0o600)
            http_port, tcp_port = free_port(), free_port()
            while tcp_port == http_port:
                tcp_port = free_port()
            environment = dict(os.environ, **connection.env())
            environment.update(HYJS_CONFIG_FILE=str(ROOT / 'hyjs-server/config.example.yaml'),
                               HYJS_ADMIN_BOOTSTRAP_FILE=str(bootstrap),
                               HYJS_GAME_TABLES=str(ROOT / 'analysis/tables'),
                               HYJS_LOG_FILE=str(temp / 'server.log'), HYJS_GORM_LOG_FILE=str(temp / 'sql.log'))
            command = [str(binary), '--env', 'test', '--bind', '127.0.0.1', '--advertise-host', '127.0.0.1',
                       '--http-port', str(http_port), '--tcp-port', str(tcp_port)]
            with (temp / 'console.log').open('w') as output:
                process = subprocess.Popen(command, cwd=ROOT / 'hyjs-server', env=environment,
                                           stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.STDOUT)
                try:
                    wait_ready(process, http_port)
                    exercise(Client(http_port, tcp_port), credentials)
                    stop(process)
                    process = subprocess.Popen(command, cwd=ROOT / 'hyjs-server', env=environment,
                                               stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.STDOUT)
                    wait_ready(process, http_port)
                    recovered = Client(http_port, tcp_port)
                    recovered.csrf = recovered.api('/admin/api/auth/login', credentials)['csrfToken']
                    policy = recovered.api('/admin/api/client-updates')['policy']
                    assert policy['versionName'] == '1.1.2' and policy['minLoginVersion'] == '1.1.2'
                    assert recovered.status(2026110102, uuid.uuid4().hex, '1.1.1')['blocked']
                    assert not recovered.status(2026110104, uuid.uuid4().hex, '1.1.2')['blocked']
                    print('PASS: old-schema MySQL migration, admin API, real HTTP/TCP, 1.1.0 compatibility and restart')
                except Exception:
                    output.flush()
                    diagnostic = (temp / 'console.log').read_text(errors='replace')[-5000:]
                    print(pool.redact(diagnostic).replace(credentials['password'], '[redacted]'), file=sys.stderr)
                    raise
                finally:
                    stop(process)
    finally:
        stop(process)
        pool.close()


if __name__ == '__main__':
    main()
