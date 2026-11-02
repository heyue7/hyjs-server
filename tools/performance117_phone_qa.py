"""Own the isolated performance117 phone fixture until quit, EOF or its TTL.

This runner uses the shared managed MySQL instance and never operates a device.
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

BUILD = ROOT / 'build/round-two-performance-final/server-oom-final'
FIXTURE_BINARY = BUILD / 'service.test'
SERVER_BINARY = BUILD / 'hyjs-server'
PRIVATE_ROOT = ROOT / '.local-env/performance117-ui'
PORTS = (18861, 18862)
FIXTURE_TEST = 'TestGGOIsolatedPerformance117PhoneFixture'
TTL = 3600


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def require_private_directory(path):
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) & 0o077:
        raise RuntimeError('Private QA directory ownership or permissions are unsafe')


def checked_source():
    path = Path(os.environ.get('HYJS_PERFORMANCE117_FIELD_CONFIG_FILE', str(
        PRIVATE_ROOT / 'published-field-configuration-live.json')))
    if not path.is_absolute() or path != Path(os.path.normpath(path)):
        raise RuntimeError('Field configuration must use an absolute private path')
    try:
        path.relative_to(PRIVATE_ROOT)
        path.resolve(strict=True).relative_to(PRIVATE_ROOT.resolve(strict=True))
    except (ValueError, OSError):
        raise RuntimeError('Field configuration is outside the private QA directory') from None
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o600:
        raise RuntimeError('Field configuration must be an owned 0600 regular file')
    value = json.loads(path.read_text())
    field = value.get('ggo') or {}
    runtime = field.get('runtime') or {}
    if value.get('scene') != 'field_003' or runtime.get('enabled') is not True or not runtime.get('points') or not runtime.get('entryPoints'):
        raise RuntimeError('Published field configuration is not enabled or has no points or entries')
    return path, digest(path)


def group_stopped(process):
    if process is None:
        return True
    if process.poll() is None:
        return False
    try:
        os.killpg(process.pid, 0)
    except ProcessLookupError:
        return True
    except PermissionError:
        return False
    return False


def stop_group(process):
    if group_stopped(process):
        return
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    if process.poll() is None:
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait(timeout=5)
    deadline = time.monotonic() + 5
    while not group_stopped(process) and time.monotonic() < deadline:
        time.sleep(.1)
    if not group_stopped(process):
        raise RuntimeError('Owned process group did not stop')


def environment(connection, private_run, fixture, source, analysis_root):
    keep = {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL', 'SSL_CERT_FILE', 'SSL_CERT_DIR'}
    env = {key: value for key, value in os.environ.items() if key in keep}
    values = {'HOST': connection.host, 'USER': connection.user, 'PASSWORD': connection.password,
              'DATABASE': connection.database}
    for prefix in ('HYJS_MYSQL_', 'HYJS_P1_MYSQL_'):
        env.update({prefix + key: value for key, value in values.items()})
    env.update({'TZ': 'Asia/Shanghai', 'GOMAXPROCS': '2', 'GOMEMLIMIT': '768MiB',
                'HYJS_DESERT_QA_FIXTURE': str(fixture),
                'HYJS_PERFORMANCE117_FIELD_CONFIG_FILE': str(source),
                'HYJS_TEST_FIXTURE_ROOT': str(analysis_root),
                'HYJS_GAME_ACCOUNTS_DATABASE': '', 'HYJS_ADMIN_BOOTSTRAP_FILE': '',
                'HYJS_CONFIG_FILE': str(ROOT / 'hyjs-server/config.example.yaml'),
                'HYJS_ADMIN_ASSETS': str(ROOT / 'admin-web/public'),
                'HYJS_LOG_MAX_AGE': '1', 'HYJS_GORM_LOG_LEVEL': 'silent',
                'HYJS_GORM_LOG_FILE': str(private_run / 'gorm.log')})
    return env


def private_secrets(private_run, pool):
    secrets = {'qa-desert-admin', 'qa-desert-placeholder', 'qa-desert-player-1', 'qa-desert-player-2'}
    for name in ('admin.json', 'fixture.json'):
        try:
            value = json.loads((private_run / name).read_text())
        except (OSError, ValueError):
            continue
        records = [value] + (value.get('players') or [])
        for record in records:
            if isinstance(record, dict):
                for key in ('username', 'password', 'sessionHex'):
                    item = record.get(key)
                    if isinstance(item, str) and len(item) >= 3:
                        secrets.add(item)
    for connection in pool.connections.values():
        if connection is not None:
            secrets.update((connection.user, connection.password))
    return secrets


def redacted(text, private_run, pool):
    text = pool.redact(text)
    for secret in private_secrets(private_run, pool):
        text = text.replace(secret, '[redacted]')
    # Guard credential-shaped fields even if a failed fixture left no JSON.
    text = re.sub(r'(?i)(password|sessionhex|session|authorization|token|secret)(["\s:=]+)([^\s",}]+)',
                  r'\1\2[redacted]', text)
    return text


def run_fixture(env, pool, private_run, fixture, evidence, stopping, owned_processes):
    command = ['rtk', 'proxy', str(FIXTURE_BINARY), '-test.v', '-test.run', '^' + FIXTURE_TEST + '$',
               '-test.count=1', '-test.timeout=8m']
    process = subprocess.Popen(command, cwd=ROOT / 'hyjs-server/service', env=env,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT, start_new_session=True)
    owned_processes.append(process)
    timed_out = False
    try:
        deadline = time.monotonic() + 490
        while True:
            try:
                raw, _ = process.communicate(timeout=1)
                break
            except subprocess.TimeoutExpired:
                if stopping() or time.monotonic() >= deadline:
                    stop_group(process)
                    raw, _ = process.communicate()
                    if stopping():
                        raise KeyboardInterrupt()
                    timed_out = True
                    break
    finally:
        stop_group(process)
        owned_processes.remove(process)
    output = redacted(raw.decode(errors='replace'), private_run, pool)
    (evidence / 'fixture.log').write_text(output)
    if timed_out:
        raise RuntimeError('Isolated fixture timed out')
    passed = re.search(r'^--- PASS: ' + re.escape(FIXTURE_TEST) + r'(?:\s|$)', output, re.M) is not None
    if process.returncode != 0 or not passed or '--- SKIP:' in output or not fixture.is_file():
        raise RuntimeError('Isolated fixture failed; inspect sanitized fixture.log')
    if stat.S_IMODE(fixture.stat().st_mode) != 0o600:
        raise RuntimeError('Private fixture output permissions differ')
    return json.loads(fixture.read_text())


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


def start_server(env, private_run, analysis_root, phase):
    stdout_path = private_run / (phase + '.stdout.log')
    log = os.fdopen(os.open(stdout_path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), 'w')
    command = ['rtk', 'proxy', str(SERVER_BINARY), '--env', 'test', '--bind', '127.0.0.1',
               '--advertise-host', '127.0.0.1', '--http-port', str(PORTS[0]), '--tcp-port', str(PORTS[1]),
               '--server-id', '1', '--tables', str(analysis_root / 'tables'), '--enable-ggo-integration',
               '--log-file', str(private_run / (phase + '.app.log'))]
    try:
        process = subprocess.Popen(command, cwd=ROOT / 'hyjs-server', env=env, stdout=log,
                                   stderr=subprocess.STDOUT, start_new_session=True)
    except BaseException:
        log.close()
        raise
    return process, log


def wait_ready(process, stopping):
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        if stopping():
            raise KeyboardInterrupt()
        if process.poll() is not None:
            raise RuntimeError('Owned QA server exited during startup')
        try:
            if health_ready():
                return
        except (OSError, ValueError, json.JSONDecodeError):
            pass
        time.sleep(.2)
    raise RuntimeError('Owned QA server failed readiness')


def main():
    os.umask(0o077)
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
    analysis_root = Path(os.environ.get('HYJS_TEST_FIXTURE_ROOT', '/Users/lirui/LR/hyjs/analysis'))
    if not analysis_root.is_absolute() or not (analysis_root / 'tables').is_dir():
        raise SystemExit('HYJS_TEST_FIXTURE_ROOT must be an absolute original analysis directory')
    local_env = PRIVATE_ROOT.parent.lstat()
    if not stat.S_ISDIR(local_env.st_mode) or local_env.st_uid != os.getuid():
        raise SystemExit('Project local environment directory is unsafe')
    require_private_directory(PRIVATE_ROOT)
    source, source_sha = checked_source()
    evidence = ROOT / 'analysis/performance117' / ('phone-' + args.run_id)
    private_run = PRIVATE_ROOT / args.run_id
    fixture = private_run / 'fixture.json'
    if evidence.exists() or private_run.exists():
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
        evidence.mkdir(parents=True, exist_ok=False)
        try:
            private_run.mkdir(mode=0o700, exist_ok=False)
            require_private_directory(private_run)
        except BaseException:
            try:
                private_run.rmdir()
            except OSError:
                pass
            evidence.rmdir()
            raise
        pool = ManagedMySQL()
        connection = None
        process = None
        owned_processes = []
        logs = []
        ready = False
        stop = False
        outcome = 1
        failure = None
        cleanup_error = None

        def interrupt(_signum, _frame):
            nonlocal stop
            stop = True

        for signum in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT):
            signal.signal(signum, interrupt)
        try:
            connection = pool.allocate('p1_performance117_phone')
            if stop:
                raise KeyboardInterrupt()
            deadline = time.monotonic() + TTL
            env = environment(connection, private_run, fixture, source, analysis_root)
            # The real binary creates the empty schema's structures first. An
            # empty bootstrap path leaves admin_users empty for the fixture.
            process, log = start_server(env, private_run, analysis_root, 'initialize')
            owned_processes.append(process)
            logs.append(log)
            wait_ready(process, lambda: stop)
            stop_group(process)
            if not group_stopped(process):
                raise RuntimeError('Schema initialization server did not stop')
            owned_processes.remove(process)
            log.close()
            process = None
            if stop:
                raise KeyboardInterrupt()
            if digest(source) != source_sha:
                raise RuntimeError('Published field source changed before fixture creation')
            login = run_fixture(env, pool, private_run, fixture, evidence, lambda: stop, owned_processes)
            if stop:
                raise KeyboardInterrupt()
            players = login.get('players')
            if login.get('schema') != connection.database or login.get('sourceHash') != source_sha or not isinstance(players, list) or len(players) != 2:
                raise RuntimeError('Private fixture identity or source mismatch')
            ids = [player.get('playerId') for player in players if isinstance(player, dict)]
            if len(ids) != 2 or len(set(ids)) != 2 or any(not isinstance(value, int) or value <= 1 for value in ids) or any(not player.get('username') or not player.get('password') or not player.get('sessionHex') for player in players):
                raise RuntimeError('Private fixture player identities are invalid')
            if digest(FIXTURE_BINARY) != fixture_sha or digest(SERVER_BINARY) != server_sha or digest(source) != source_sha:
                raise RuntimeError('Fixed input changed during fixture preparation')
            process, log = start_server(env, private_run, analysis_root, 'server')
            owned_processes.append(process)
            logs.append(log)
            wait_ready(process, lambda: stop)
            if time.monotonic() >= deadline:
                raise RuntimeError('Fixture startup exceeded its one-hour lease')
            ready = True
            metadata = {'schema': connection.database, 'playerIds': ids, 'httpPort': PORTS[0],
                        'tcpPort': PORTS[1], 'pid': process.pid, 'binarySHA256': server_sha,
                        'fixtureBinarySHA256': fixture_sha, 'fieldSourceSHA256': source_sha,
                        'privateLoginFile': str(fixture), 'finiteFixture': login['fixture'],
                        'productionAccess': False, 'ttlSeconds': TTL}
            (evidence / 'runtime.json').write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + '\n')
            print(json.dumps({'ready': True, **metadata}, ensure_ascii=False), flush=True)
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
                        print(json.dumps({'ready': True, 'playerIds': ids, 'pid': process.pid}), flush=True)
            outcome = 0
        except KeyboardInterrupt:
            failure = 'interrupted'
            print('Fixture preparation interrupted; releasing owned resources.', flush=True)
        except Exception as error:
            failure = type(error).__name__ + ': ' + str(error)
            print('QA failed: ' + type(error).__name__ + '; inspect sanitized evidence.', flush=True)
        finally:
            for signum in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT):
                signal.signal(signum, signal.SIG_IGN)
            cleanup_errors = []
            for owned in reversed(owned_processes):
                try:
                    stop_group(owned)
                except Exception as error:
                    cleanup_errors.append('process:' + type(error).__name__)
            server_stopped = all(group_stopped(owned) for owned in owned_processes)
            if failure is not None:
                try:
                    failure = redacted(failure, private_run, pool)
                except Exception:
                    failure = 'failure details unavailable'
                    cleanup_errors.append('failure-redaction')
            if server_stopped:
                for log in logs:
                    try:
                        if not log.closed:
                            log.close()
                    except OSError as error:
                        cleanup_errors.append('log-close:' + type(error).__name__)
                # Raw logs and credentials exist only in the owned private run.
                try:
                    for path in private_run.iterdir():
                        if not path.is_symlink() and path.is_file() and (path.name.endswith('.log') or '.log.' in path.name):
                            output = redacted(path.read_text(errors='replace'), private_run, pool)
                            (evidence / path.name).write_text(output)
                    for path in private_run.iterdir():
                        if path.name in ('fixture.json', 'admin.json') or path.name.endswith('.log') or '.log.' in path.name:
                            if not path.is_symlink() and path.is_file():
                                path.unlink()
                    private_run.rmdir()
                except (OSError, ValueError) as error:
                    cleanup_errors.append('private-files:' + type(error).__name__)
            else:
                cleanup_errors.append('server-active-resources-retained')
            database = connection.database if connection is not None else None
            if server_stopped:
                try:
                    pool.close()
                except Exception as error:
                    cleanup_errors.append('database:' + type(error).__name__)
            cleanup_error = ','.join(cleanup_errors) or None
            removed = not pool.connections and (database is None or
                not (SERVICES / 'test-connections' / (database + '.env')).exists() and
                not (SERVICES / 'test-connections' / (database + '.json')).exists())
            cleanup = {'ownedServerStopped': server_stopped,
                       'ownedAllocationsReleased': removed, 'privateRunRemoved': not private_run.exists(),
                       'cleanupErrorType': cleanup_error, 'failure': failure}
            (evidence / 'cleanup.json').write_text(json.dumps(cleanup, indent=2) + '\n')
            if not all(cleanup[key] for key in ('ownedServerStopped', 'ownedAllocationsReleased', 'privateRunRemoved')):
                outcome = 1
            print(json.dumps(cleanup), flush=True)
            fcntl.flock(lease, fcntl.LOCK_UN)
        return outcome


if __name__ == '__main__':
    sys.exit(main())
