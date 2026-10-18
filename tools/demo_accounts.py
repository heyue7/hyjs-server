"""Prepare finite, separate P0 demo accounts through the real protocol and explicit SQL fixtures."""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from server.codec import BY_NAME, decode_responses, encode_http_request, message
from tools.shared_mysql import sql as shared_sql


def sql(query):
    return shared_sql(query)


class Client:
    def __init__(self, host, port):
        self.host = host
        self.port = port
        self.login = None
        self.stamp = 0

    def send(self, name, fields=None):
        self.stamp = max(self.stamp + 1, int(time.time() * 1000))
        args = {'zone_id': 1, 'timestamp': self.stamp}
        if self.login:
            args.update(player_id=self.login.playerId, session=bytes(self.login.sessionKey))
        payload = encode_http_request(BY_NAME[name], message(name, **(fields or {})), **args)
        conn = http.client.HTTPConnection(self.host, self.port, timeout=10)
        try:
            conn.request('POST', '/sao/', payload, {'Content-Type': 'application/octet-stream'})
            response = conn.getresponse()
            raw = response.read()
            if response.status != 200:
                raise RuntimeError(f'{name} HTTP status {response.status}')
            opcode, result = decode_responses(raw)[0]
            if opcode == 10001:
                raise RuntimeError(f'{name} rejected with code {result.code}')
            return result
        finally:
            conn.close()


def profile(player_id):
    rows = sql(f'SELECT uid,player,game FROM profiles WHERE id={int(player_id)};').splitlines()
    if len(rows) != 1:
        raise RuntimeError('Missing demo profile')
    uid, player, game = rows[0].split('\t')
    if not uid.startswith('demo-p0-') or player_id == 1:
        raise RuntimeError('Refusing fixture changes to a non-demo profile')
    return uid, json.loads(player), json.loads(game)


def prepare(role, host, port):
    path = ROOT / '.local-env' / f'demo-p0-{role}.json'
    if not path.exists():
        data = {'uid': f'demo-p0-{role}-{uuid.uuid4().hex[:8]}',
                'password': secrets.token_hex(12), 'role': role, 'initialized': False}
        path.parent.mkdir(exist_ok=True)
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'w') as file:
            json.dump(data, file)
    data = json.loads(path.read_text())
    if not data['uid'].startswith('demo-p0-' + role + '-'):
        raise RuntimeError('Demo credential file has unexpected account scope')
    client = Client(host, port)
    client.login = client.send('CSUserLogin', {'platformUid': data['uid'],
                                             'authCode': data['password'],
                                             'clientTime': int(time.time() * 1000)})
    player_id = client.login.playerId
    client.send('CSPlayerCreate', {'playerName': 'DemoStarter' if role == 'starter' else 'DemoSystems',
                                  'heroId': 'hero_047' if role == 'starter' else 'hero_001'})
    if not data.get('initialized'):
        uid, player, game = profile(player_id)
        if role == 'systems':
            # Only this isolated account receives a documented, finite high-level fixture.
            player.update(level=75, coin=1000000, diamond=5000, stamina=200, energy=100,
                          noviceGuideSchedule=3)
            for warrior in game['warriors']:
                warrior['level'] = 75
            game['items'] = [{'id': f'item_{number:03d}', 'num': 200}
                             for number in [1, 2, 6, 7, *range(439, 455)]]
            game['equipment'] = [{'guid': uuid.uuid4().hex, 'equipId': 'equip_001',
                                  'level': 1, 'refineLv': 0, 'refineExp': 0, 'star': 0,
                                  'starExp': 0, 'lastHeroId': '', 'coinPrice': 1000,
                                  'forgeLevel': 0}]
            player_hex = json.dumps(player, ensure_ascii=False).encode().hex()
            game_hex = json.dumps(game, ensure_ascii=False).encode().hex()
            uid_hex = uid.encode().hex()
            sql(f"UPDATE profiles SET player=CONVERT(0x{player_hex} USING utf8mb4), "
                f"game=CONVERT(0x{game_hex} USING utf8mb4) WHERE id={player_id} "
                f"AND uid=CONVERT(0x{uid_hex} USING utf8mb4);")
        uid, player, game = profile(player_id)
        evidence = {'role': role, 'player_id': player_id,
                    'initialization': 'normal protocol creation' if role == 'starter' else
                                      'explicit finite isolated SQL fixture',
                    'level': player['level'], 'coin': player['coin'],
                    'diamond': player['diamond'], 'stamina': player['stamina'],
                    'energy': player['energy'], 'items': game.get('items', []),
                    'equipment': game.get('equipment', []),
                    'player_sha256': hashlib.sha256(json.dumps(player, sort_keys=True).encode()).hexdigest(),
                    'game_sha256': hashlib.sha256(json.dumps(game, sort_keys=True).encode()).hexdigest()}
        folder = ROOT / 'analysis/p0'
        folder.mkdir(exist_ok=True)
        (folder / f'{role}-initialization.json').write_text(json.dumps(evidence, ensure_ascii=False,
                                                                     indent=2) + '\n')
        data.update(player_id=player_id, initialized=True)
        path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')
        path.chmod(0o600)
    uid, player, game = profile(player_id)
    return {'role': role, 'player_id': player_id, 'level': player['level'],
            'coin': player['coin'], 'diamond': player['diamond'],
            'credential_file': str(path), 'initialized': data.get('initialized', False)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--host', default='127.0.0.1')
    parser.add_argument('--port', type=int, default=8088)
    parser.add_argument('--role', choices=['starter', 'systems', 'both'], default='both')
    args = parser.parse_args()
    roles = ['starter', 'systems'] if args.role == 'both' else [args.role]
    print(json.dumps([prepare(role, args.host, args.port) for role in roles], ensure_ascii=False))


if __name__ == '__main__':
    main()
