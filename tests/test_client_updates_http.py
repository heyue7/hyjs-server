"""Exercise the actual local HTTP/TCP login gates and admin policy endpoints."""
import http.cookiejar
import json
from pathlib import Path
import secrets
import socket
import struct
import sys
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from server.codec import BY_NAME, CLASSES, decode_responses, encode_http_request

ORIGIN = 'http://127.0.0.1:28891'
cookies = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookies))
csrf = ''


def api(path, body=None):
    request = urllib.request.Request(ORIGIN + path, None if body is None else json.dumps(body).encode(), headers={'Content-Type': 'application/json', 'Origin': ORIGIN, 'X-CSRF-Token': csrf})
    with opener.open(request, timeout=20) as response:
        result = json.load(response)
    assert result['code'] == 0, result.get('msg')
    return result['data']


def wire(name, fields, player=0, session=bytes(16)):
    raw = encode_http_request(BY_NAME[name], CLASSES[name](**fields), player_id=player, zone_id=1, session=session, timestamp=int(time.time() * 1000))
    with urllib.request.urlopen(urllib.request.Request(ORIGIN + '/sao/', raw, headers={'Content-Type': 'application/octet-stream'}), timeout=20) as response:
        return [body for _, body in decode_responses(response.read())]


def expected(messages, name):
    found = [body for body in messages if body.DESCRIPTOR.name == name]
    assert len(found) == 1, [body.DESCRIPTOR.name for body in messages]
    return found[0]


def exact(connection, length):
    raw = b''
    while len(raw) < length:
        part = connection.recv(length - len(raw))
        assert part, 'TCP closed'
        raw += part
    return raw


def tcp_login(login):
    connection = socket.create_connection(('127.0.0.1', 28893), timeout=10)
    payload = CLASSES['CSSocketLogin'](sessionKey=login.sessionKey).SerializeToString()
    connection.sendall(struct.pack('>iiiiqi', 24 + len(payload), 1, BY_NAME['CSSocketLogin'], 1, login.playerId, len(payload)) + payload)
    header = exact(connection, 8)
    size, _ = struct.unpack('>ii', header)
    response = [body for _, body in decode_responses(header + exact(connection, size))]
    return connection, response


def command(release, action):
    state = api('/admin/api/client-updates')
    body = {'id': release, 'version': state['version'], 'requestId': str(uuid.uuid4()), 'action': action, 'reason': '隔离HTTP/TCP验收'}
    result = api('/admin/api/client-updates/action', body)
    assert api('/admin/api/client-updates/action', body)['version'] == result['version'], 'action retry was not idempotent'
    return result


def release(mode, build, login, run):
    state = api('/admin/api/client-updates')
    now = int(time.time() * 1000)
    result = api('/admin/api/client-updates/draft', {'id': 0, 'version': state['version'], 'buildCode': build, 'versionName': '隔离验收版本', 'title': '版本兼容验收', 'content': '本条仅在本机隔离环境使用。', 'mode': mode, 'minLoginBuild': login, 'minRunBuild': run, 'maintenanceStartAt': now + 600000 if mode == 'maintenance' else 0, 'maintenanceEndAt': now + 1200000 if mode == 'maintenance' else 0, 'reason': '隔离HTTP/TCP验收'})
    return result['items'][0]['id']


def main():
    global csrf
    credentials = json.loads((ROOT / '.local-env/client-update-qa/bootstrap.json').read_text())
    csrf = api('/admin/api/auth/login', credentials)['csrfToken']
    policy = api('/admin/api/client-updates')['policy']
    old_build = max(2026102102, policy['latestBuild'], policy['minLoginBuild'])
    assert old_build < 2099999998
    startup = uuid.uuid4().hex
    username, password = 'cu_' + secrets.token_hex(5), secrets.token_urlsafe(20)
    api('/game-account/register', {'username': username, 'nickname': '更新验收', 'password': password, 'confirmPassword': password})
    fields = {'platformUid': username, 'authCode': password, 'appVersion': f'hyjs/{old_build}/{startup}', 'clientTime': int(time.time() * 1000)}
    login = expected(wire('CSUserLogin', fields), 'SCUserLogin')
    expected(wire('CSPlayerCreate', {'playerName': '更新验收', 'heroId': 'hero_043'}, login.playerId, login.sessionKey), 'SCPlayerCreate')
    connection, messages = tcp_login(login)
    assert expected(messages, 'SCSocketLogin').result == 100
    try:
        published = command(release('next_login', old_build + 1, old_build + 1, policy['minRunBuild']), 'publish')
        state = api('/client-update/status', {'buildCode': old_build, 'startupId': startup, 'playerId': login.playerId, 'sessionHex': login.sessionKey.hex()})
        assert not state['blocked'], 'online compatible session was interrupted'
        assert not any(body.DESCRIPTOR.name == 'SCUserLogin' for body in wire('CSUserLogin', fields)), 'old login bypassed gate'
        # A rejected new-login attempt must not rotate the still-authorized current session.
        assert not api('/client-update/status', {'buildCode': old_build, 'startupId': startup, 'playerId': login.playerId, 'sessionHex': login.sessionKey.hex()})['blocked']
        connection.close()
        time.sleep(0.2)
        connection, recovered = tcp_login(login)
        assert expected(recovered, 'SCSocketLogin').result == 100, 'short reconnect rejected'
        maintenance = release('maintenance', old_build + 2, old_build + 2, old_build + 2)
        command(maintenance, 'publish')
        command(maintenance, 'start_maintenance')
        # Maintenance sends its final explanation and closes the actual old TCP connection.
        terminal = b''
        while True:
            part = connection.recv(4096)
            if not part:
                break
            terminal += part
            assert len(terminal) < 131072
        assert terminal, 'maintenance did not explain the disconnect'
        assert api('/client-update/status', {'buildCode': old_build, 'startupId': startup, 'playerId': login.playerId, 'sessionHex': login.sessionKey.hex()})['blocked']
        command(maintenance, 'end_maintenance')
        assert not any(body.DESCRIPTOR.name == 'SCRefreshGatewayIPAndPort' for body in wire('CSRefreshGatewayIPAndPort', {'sessionKey': login.sessionKey}, login.playerId, login.sessionKey)), 'old token bypassed running gate'
        fields['appVersion'] = f'hyjs/{old_build + 2}/{uuid.uuid4().hex}'
        new_login = expected(wire('CSUserLogin', fields), 'SCUserLogin')
        assert new_login.sessionKey != login.sessionKey
        result = {'status': 'PASS', 'scope': 'actual isolated HTTP/TCP original protocol and authenticated admin APIs', 'nextLoginRetainsOnlineSession': True, 'oldNewLoginRejectedBeforeSessionRotation': True, 'shortTCPReconnect': True, 'maintenanceClosesOldTCPConnection': True, 'maintenanceBlocksAllSessions': True, 'oldTokenAfterMaintenanceRejected': True, 'newBuildLogin': True, 'repeatedAdminRequestIdempotent': True, 'productionDataTouched': False}
        (ROOT / 'analysis/client-update-20261021/http-tcp.json').write_text(json.dumps(result, indent=2) + '\n')
        print(json.dumps(result))
    finally:
        connection.close()


if __name__ == '__main__':
    main()
