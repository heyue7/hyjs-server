"""Original signed HTTP sweep, startup trust and cold recovery on isolated MySQL."""
import datetime
import http.client
import json
import os
from pathlib import Path
import re
import selectors
import subprocess
import tempfile
import time
import unittest

from server.codec import BY_NAME, decode_responses, encode_http_request, message


ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(
    os.environ.get('MYSQL_DATABASE', '').startswith('hyjs_test_sweep_')
    and os.environ.get('HYJS_GO_TEST_BINARY'),
    'requires an isolated sweep MySQL allocation and a temporary server binary',
)
class CopySweepProcessMySQLTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='hyjs-sweep-process-')
        self.addCleanup(self.temp.cleanup)
        self.process = None
        self.addCleanup(self.stop_server)

    def sql(self, query):
        script = 'IFS= read -r MYSQL_PWD; export MYSQL_PWD; exec mysql --connect-timeout=5 -h127.0.0.1 -u "$1" -NB --binary-mode "$2"'
        result = subprocess.run(
            ['rtk', 'proxy', 'docker', 'exec', '-i', 'codex-local-mysql', 'sh', '-c', script,
             '--', os.environ['MYSQL_USER'], os.environ['MYSQL_DATABASE']],
            input=os.environ['MYSQL_PASSWORD'] + '\n' + query,
            text=True, capture_output=True, timeout=30,
        )
        self.assertEqual(result.returncode, 0, 'isolated MySQL operation failed')
        return result.stdout.strip()

    def start_server(self):
        env = dict(os.environ,
                   HYJS_CONFIG_FILE=str(ROOT / 'hyjs-server/config.example.yaml'),
                   HYJS_MYSQL_HOST='127.0.0.1:3306',
                   HYJS_MYSQL_USER=os.environ['MYSQL_USER'],
                   HYJS_MYSQL_PASSWORD=os.environ['MYSQL_PASSWORD'],
                   HYJS_MYSQL_DATABASE=os.environ['MYSQL_DATABASE'],
                   HYJS_COPY_SWEEP_P9_COMPLETE_SERVER='1',
                   HYJS_LOG_FILE=str(Path(self.temp.name) / 'app.log'),
                   HYJS_GORM_LOG_FILE=str(Path(self.temp.name) / 'sql.log'))
        env.pop('HYJS_GAME_ACCOUNTS_DATABASE', None)
        self.process = subprocess.Popen(
            [os.environ['HYJS_GO_TEST_BINARY'], '--http-port', '0', '--tcp-port', '0',
             '--tables', str(ROOT / 'analysis/tables')],
            cwd=ROOT / 'hyjs-server', env=env, stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT, text=True,
        )
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), 'isolated server did not start')
            line = self.process.stdout.readline()
        match = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(match, 'isolated server did not expose both listeners')
        self.http_port = int(match.group(1))

    def stop_server(self):
        process = self.process
        if process and process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
                self.fail('isolated server did not stop')
        if process and process.stdout:
            process.stdout.close()
        self.process = None

    def request(self, name, fields=None, login=None, stamp=None):
        options = {'zone_id': 1, 'timestamp': int(time.time() * 1000) if stamp is None else stamp}
        if login is not None:
            options.update(player_id=login.playerId, session=bytes(login.sessionKey))
        packet = encode_http_request(BY_NAME[name], message(name, **(fields or {})), **options)
        conn = http.client.HTTPConnection('127.0.0.1', self.http_port, timeout=30)
        try:
            conn.request('POST', '/sao/', body=packet, headers={'Content-Type': 'application/octet-stream'})
            reply = conn.getresponse()
            body = reply.read()
            self.assertEqual(reply.status, 200)
            frames = decode_responses(body)
            self.assertTrue(frames)
            return frames
        finally:
            conn.close()

    def login(self, uid):
        opcode, reply = self.request('CSUserLogin', {
            'platformUid': uid, 'authCode': 'local-test-password', 'clientTime': 1,
        })[0]
        self.assertEqual(opcode, BY_NAME['SCUserLogin'], str(reply))
        return reply

    def register(self, uid):
        body = json.dumps({'nickname': '隔离扫荡', 'username': uid,
                           'password': 'local-test-password', 'confirmPassword': 'local-test-password'})
        conn = http.client.HTTPConnection('127.0.0.1', self.http_port, timeout=20)
        try:
            conn.request('POST', '/game-account/register', body=body, headers={'Content-Type': 'application/json'})
            reply = conn.getresponse()
            data = json.loads(reply.read())
            self.assertEqual(reply.status, 200, str(data))
            self.assertTrue(data.get('data', {}).get('registered'), str(data))
        finally:
            conn.close()

    def profile(self, player_id):
        values = self.sql(f'SELECT HEX(player),HEX(game) FROM profiles WHERE id={int(player_id)};').split('\t')
        self.assertEqual(len(values), 2)
        return tuple(json.loads(bytes.fromhex(value).decode()) for value in values)

    def prepare_chapter_three(self, player_id):
        player, game = self.profile(player_id)
        player.update(level=100, stamina=1000, exp=0)
        progress = game.setdefault('copy', {}).setdefault('passedCopyInfo', [])
        target = next((row for row in progress if row.get('chapterId') == '3' and row.get('missionId') == '12'), None)
        if target is None:
            target = {'chapterId': '3', 'missionId': '12'}
            progress.append(target)
        target.update(star=5, challengeTimes=0, isOpen=1)
        game['copy_challenge_day'] = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).strftime('%Y-%m-%d')
        values = [json.dumps(item, ensure_ascii=False, separators=(',', ':')).encode().hex() for item in (player, game)]
        self.sql(f"UPDATE profiles SET player=CONVERT(UNHEX('{values[0]}') USING utf8mb4), "
                 f"game=CONVERT(UNHEX('{values[1]}') USING utf8mb4) WHERE id={int(player_id)};")

    def assert_state(self, player_id, expected_count, expected_stamp):
        player, game = self.profile(player_id)
        row = next(item for item in game['copy']['passedCopyInfo']
                   if item.get('chapterId') == '3' and item.get('missionId') == '12')
        self.assertEqual(player['stamina'], 1000 - 10 * expected_count)
        self.assertEqual(row['challengeTimes'], expected_count)
        result = self.sql(f"SELECT COUNT(*) FROM p9_pet_copy_grants WHERE player_id={int(player_id)} "
                          "AND source_key LIKE 'sweep:%';")
        self.assertEqual(int(result), expected_count)
        witness = self.sql(f'SELECT last_stamp FROM copy_sweep_witnesses WHERE player_id={int(player_id)};')
        self.assertEqual(int(witness), expected_stamp)

    def emulate_legacy_receipt(self, player_id):
        # The old binary stored only the maximum receipt; its P9 grant was
        # committed in the same transaction. Recreate that pre-upgrade shape.
        _, game = self.profile(player_id)
        self.assertTrue(game.pop('copy_sweep_witness_initialized'))
        encoded = json.dumps(game, ensure_ascii=False, separators=(',', ':')).encode().hex()
        self.sql('START TRANSACTION; '
                 f"UPDATE profiles SET game=CONVERT(UNHEX('{encoded}') USING utf8mb4) WHERE id={int(player_id)}; "
                 f'DELETE FROM copy_sweep_witnesses WHERE player_id={int(player_id)}; COMMIT;')

    def test_chapter_three_clock_rewind_ten_sweeps_and_restart(self):
        self.start_server()
        uid = 'sweep-process-' + str(time.time_ns())[-12:]
        self.register(uid)
        login = self.login(uid)
        opcode, created = self.request('CSPlayerCreate', {'playerName': '隔离扫荡', 'heroId': 'hero_043'}, login)[0]
        self.assertEqual(opcode, BY_NAME['SCPlayerCreate'], str(created))
        self.prepare_chapter_three(login.playerId)
        fields = {'chapterId': '3', 'missionId': '12'}
        future = int(time.time() * 1000) + 3 * 60 * 60 * 1000
        frames = self.request('CSCopySweep', fields, login, future)
        self.assertEqual(frames[0][0], BY_NAME['SCCopySweep'], str(frames[0][1]))
        self.emulate_legacy_receipt(login.playerId)
        base = int(time.time() * 1000)
        elapsed = []
        last = None
        for index in range(10):
            stamp = base + index * 100
            start = time.perf_counter()
            frames = self.request('CSCopySweep', fields, login, stamp)
            elapsed.append(time.perf_counter() - start)
            self.assertEqual(frames[0][0], BY_NAME['SCCopySweep'], str(frames[0][1]))
            last = frames[0][1]
            self.assert_state(login.playerId, index + 2, stamp)
        last_stamp = base + 900
        replay = self.request('CSCopySweep', fields, login, last_stamp)
        self.assertEqual(replay[0][0], BY_NAME['SCCopySweep'])
        self.assertEqual(replay[0][1].SerializeToString(), last.SerializeToString())
        self.assert_state(login.playerId, 11, last_stamp)
        old = self.request('CSCopySweep', fields, login, future)
        self.assertEqual(old[0][0], BY_NAME['ErrorMessage'])
        self.stop_server()
        self.start_server()
        login = self.login(uid)
        replay = self.request('CSCopySweep', fields, login, last_stamp)
        self.assertEqual(replay[0][0], BY_NAME['SCCopySweep'])
        self.assertEqual(replay[0][1].SerializeToString(), last.SerializeToString())
        self.assert_state(login.playerId, 11, last_stamp)
        fresh = self.request('CSCopySweep', fields, login, base + 2000)
        self.assertEqual(fresh[0][0], BY_NAME['SCCopySweep'], str(fresh[0][1]))
        self.assert_state(login.playerId, 12, base + 2000)
        print('isolated chapter 3 signed HTTP sweep seconds:', ', '.join(f'{x:.3f}' for x in elapsed))


if __name__ == '__main__':
    unittest.main()
