"""Own the trial82 local phone fixture until quit, EOF, signal or its TTL.

Only an isolated managed schema is used. This command does not access a device.
"""
import argparse
import fcntl
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import selectors
import signal
import socket
import stat
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tests'))
from hyjs_managed_mysql import ManagedMySQL, SERVICES

BUILD = ROOT / 'build/trial82'
FIXTURE_BINARY = BUILD / 'service.test'
SERVER_BINARY = BUILD / 'hyjs-server'
PORTS = (18871, 18872)


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def stop_group(process):
    if process is None:
        return
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    if process.poll() is not None:
        return
    try:
        process.wait(timeout=15)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.wait(timeout=5)


def environment(connection, credential_file):
    keep = {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL', 'TZ', 'SSL_CERT_FILE', 'SSL_CERT_DIR'}
    env = {key: value for key, value in os.environ.items() if key in keep}
    values = {'HOST': connection.host, 'USER': connection.user, 'PASSWORD': connection.password,
              'DATABASE': connection.database}
    for prefix in ('HYJS_MYSQL_', 'HYJS_P1_MYSQL_'):
        env.update({prefix + key: value for key, value in values.items()})
    env.update({'GOMAXPROCS': '2', 'GOMEMLIMIT': '768MiB',
                'HYJS_TRIAL82_PHONE_FIXTURE_FILE': str(credential_file),
                'HYJS_GAME_ACCOUNTS_DATABASE': '',
                'HYJS_CONFIG_FILE': str(ROOT / 'hyjs-server/config.example.yaml'),
                'HYJS_LOG_MAX_AGE': '1', 'HYJS_GORM_LOG_LEVEL': 'silent',
                'HYJS_TEST_FIXTURE_ROOT': '/Users/lirui/LR/hyjs/analysis'})
    return env


def run_fixture(env, pool, credential_file, evidence):
    command = ['rtk', 'proxy', str(FIXTURE_BINARY), '-test.v', '-test.run', '^TestTrial82PhoneFixture$',
               '-test.count=1', '-test.timeout=8m']
    process = subprocess.Popen(command, cwd=ROOT / 'hyjs-server/service', env=env,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT, start_new_session=True)
    timed_out = False
    try:
        try:
            raw, _ = process.communicate(timeout=490)
        except subprocess.TimeoutExpired:
            stop_group(process)
            raw, _ = process.communicate()
            timed_out = True
    except BaseException:
        stop_group(process)
        raise
    username = 'king-' + hashlib.sha256(b'TestTrial82PhoneFixture').hexdigest()[:24]
    output = pool.redact(raw.decode(errors='replace')).replace('isolated-password', '[redacted]').replace(username, '[test-account]')
    for connection in pool.connections.values():
        if connection is not None:
            output = output.replace(connection.user, '[test-user]')
    (evidence / 'fixture.log').write_text(output)
    if timed_out:
        raise RuntimeError('Isolated fixture timed out')
    passed = re.search(r'^--- PASS: TestTrial82PhoneFixture(?:\s|$)', output, re.M) is not None
    if process.returncode != 0 or not passed or '--- SKIP:' in output or not credential_file.is_file():
        raise RuntimeError('Isolated fixture failed; inspect sanitized fixture.log')
    if stat.S_IMODE(credential_file.stat().st_mode) != 0o600:
        raise RuntimeError('Private fixture output permissions differ')
    return json.loads(credential_file.read_text())


def health_ready():
    for port in PORTS:
        with socket.create_connection(('127.0.0.1', port), timeout=1):
            pass
    connection = http.client.HTTPConnection('127.0.0.1', PORTS[0], timeout=2)
    try:
        connection.request('GET', '/health')
        response = connection.getresponse()
        body = response.read(4096)
        value = json.loads(body)
        return response.status == 200 and value.get('status') == 'ok' and value.get('database') == 'mysql'
    finally:
        connection.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', action='store_true')
    parser.add_argument('--run-id', required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'[a-z0-9][a-z0-9-]{0,63}', args.run_id):
        parser.error('Use a short lowercase run id')
    if not args.run:
        parser.print_help()
        return 0
    if not FIXTURE_BINARY.is_file() or not SERVER_BINARY.is_file():
        raise SystemExit('The fixed fixture and server binaries must already exist')
    fixture_sha, server_sha = digest(FIXTURE_BINARY), digest(SERVER_BINARY)
    evidence = ROOT / 'analysis/trial82' / ('phone-' + args.run_id)
    private_dir = ROOT / '.local-env/trial82-ui'
    credentials = private_dir / (args.run_id + '.json')
    if evidence.exists() or credentials.exists():
        raise SystemExit('Use fresh owned evidence and private credential paths')

    with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lease:
        try:
            fcntl.flock(lease, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print('Shared MySQL lease held elsewhere; no QA resources started.')
            return 75
        for port in PORTS:
            with socket.socket() as probe:
                probe.bind(('127.0.0.1', port))
        private_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
        if private_dir.is_symlink() or private_dir.stat().st_uid != os.getuid() or stat.S_IMODE(private_dir.stat().st_mode) & 0o077:
            raise SystemExit('Trial82 private directory has unsafe ownership or permissions')
        evidence.mkdir(parents=True, exist_ok=False)
        pool = ManagedMySQL()
        connection = None
        process = None
        log = None
        ready = False
        stop = False
        outcome = 1
        cleanup_error = None

        def interrupt(_signum, _frame):
            nonlocal stop
            if not ready:
                raise KeyboardInterrupt()
            stop = True

        for signum in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT):
            signal.signal(signum, interrupt)
        try:
            connection = pool.allocate('p1_activity77_phone_82')
            env = environment(connection, credentials)
            login = run_fixture(env, pool, credentials, evidence)
            if login.get('schema') != connection.database or login.get('playerId') != 2 or login.get('accountId') != 2 or login.get('serverId') != 1 or not login.get('username') or not login.get('password'):
                raise RuntimeError('Private fixture identity mismatch')
            if digest(FIXTURE_BINARY) != fixture_sha or digest(SERVER_BINARY) != server_sha:
                raise RuntimeError('Fixed binary changed during fixture preparation')
            log = (evidence / 'server.stdout.log').open('w')
            command = ['rtk', 'proxy', str(SERVER_BINARY), '--env', 'test', '--bind', '127.0.0.1',
                       '--advertise-host', '127.0.0.1', '--http-port', str(PORTS[0]), '--tcp-port', str(PORTS[1]),
                       '--server-id', '1', '--tables', str(ROOT / 'analysis/tables'), '--enable-test-payments',
                       '--enable-local-wallet', '--log-file', str(evidence / 'app.log')]
            process = subprocess.Popen(command, cwd=ROOT / 'hyjs-server', env=env, stdout=log,
                                       stderr=subprocess.STDOUT, start_new_session=True)
            deadline = time.monotonic() + 90
            while time.monotonic() < deadline:
                if process.poll() is not None:
                    raise RuntimeError('Owned QA server exited during startup')
                try:
                    if health_ready():
                        ready = True
                        break
                except (OSError, ValueError, json.JSONDecodeError):
                    pass
                time.sleep(.2)
            if not ready:
                raise RuntimeError('Owned QA server failed readiness')
            metadata = {'schema': connection.database, 'playerId': login['playerId'],
                        'accountId': login['accountId'], 'httpPort': PORTS[0], 'tcpPort': PORTS[1],
                        'pid': process.pid, 'binarySHA256': server_sha, 'fixtureBinarySHA256': fixture_sha,
                        'privateLoginFile': str(credentials), 'finiteFixture': login['fixture'],
                        'productionAccess': False, 'ttlSeconds': 3600}
            (evidence / 'runtime.json').write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + '\n')
            print(json.dumps({'ready': True, **metadata}, ensure_ascii=False), flush=True)
            deadline = time.monotonic() + 3600
            with selectors.DefaultSelector() as waiting:
                waiting.register(sys.stdin, selectors.EVENT_READ)
                while not stop and time.monotonic() < deadline:
                    if process.poll() is not None:
                        raise RuntimeError('Owned QA server stopped unexpectedly')
                    if not waiting.select(1):
                        continue
                    action = sys.stdin.readline().strip()
                    if action in ('', 'quit', 'exit'):
                        break
                    if action == 'status':
                        print(json.dumps({'ready': True, 'playerId': login['playerId'], 'pid': process.pid}), flush=True)
            outcome = 0
        except KeyboardInterrupt:
            print('Fixture preparation interrupted; releasing owned resources.', flush=True)
        except Exception as error:
            print('QA failed: ' + type(error).__name__ + '; inspect sanitized evidence.', flush=True)
        finally:
            stop_group(process)
            if log:
                log.close()
            credentials.unlink(missing_ok=True)
            database = connection.database if connection is not None else None
            try:
                pool.close()
            except Exception as error:
                cleanup_error = type(error).__name__
            removed = not pool.connections and (database is None or
                not (SERVICES / 'test-connections' / (database + '.env')).exists() and
                not (SERVICES / 'test-connections' / (database + '.json')).exists())
            cleanup = {'ownedServerStopped': process is None or process.poll() is not None,
                       'ownedAllocationsReleased': removed, 'privateLoginRemoved': not credentials.exists(),
                       'cleanupErrorType': cleanup_error}
            (evidence / 'cleanup.json').write_text(json.dumps(cleanup, indent=2) + '\n')
            if not all(cleanup[key] for key in ('ownedServerStopped', 'ownedAllocationsReleased', 'privateLoginRemoved')):
                outcome = 1
            print(json.dumps(cleanup), flush=True)
            fcntl.flock(lease, fcntl.LOCK_UN)
        return outcome


if __name__ == '__main__':
    sys.exit(main())
