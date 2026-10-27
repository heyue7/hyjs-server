"""Own one managed local activity77 phone fixture until quit/EOF/SIGTERM."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import selectors
import signal
import socket
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tests'))
from hyjs_managed_mysql import ManagedMySQL


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', action='store_true')
    parser.add_argument('--feedback78', action='store_true', help='Use the separate feedback78 fixture and artifacts')
    args = parser.parse_args()
    if not args.run:
        parser.print_help()
        return
    for port in (18861, 18862):
        with socket.socket() as probe:
            probe.bind(('127.0.0.1', port))
    private = ROOT / ('.local-env/feedback78-ui' if args.feedback78 else '.local-env/activity77-ui')
    private.mkdir(mode=0o700, parents=True, exist_ok=True)
    credentials = private / 'credentials.json'
    if credentials.exists():
        raise RuntimeError('Existing owned fixture must be reconciled before retry')
    evidence = ROOT / ('analysis/feedback78/qa-runtime' if args.feedback78 else 'analysis/activity-growth77/qa-runtime')
    build = ROOT / ('build/feedback78' if args.feedback78 else 'build/activity-growth77')
    fixture_name = 'TestFeedback78PhoneFixture' if args.feedback78 else 'TestActivity77PhoneFixture'
    evidence.mkdir(parents=True, exist_ok=True)
    pool = ManagedMySQL()
    process = None
    log = None
    try:
        connection = pool.allocate('p1_activity77_phone_f78' if args.feedback78 else 'p1_activity77_phone')
        env = dict(os.environ, **connection.env(), GOMAXPROCS='2', GOMEMLIMIT='768MiB',
                   HYJS_P1_MYSQL_HOST=connection.host, HYJS_P1_MYSQL_USER=connection.user,
                   HYJS_P1_MYSQL_PASSWORD=connection.password, HYJS_P1_MYSQL_DATABASE=connection.database,
                   HYJS_ACTIVITY77_PHONE_FIXTURE_FILE=str(credentials),
                   HYJS_GAME_ACCOUNTS_DATABASE='', HYJS_CONFIG_FILE=str(ROOT / 'hyjs-server/config.example.yaml'),
                   HYJS_LOG_MAX_AGE='1', HYJS_GORM_LOG_LEVEL='silent')
        if args.feedback78:
            env.pop('HYJS_ACTIVITY77_PHONE_FIXTURE_FILE')
            env['HYJS_FEEDBACK78_PHONE_FIXTURE_FILE'] = str(credentials)
        fixture = subprocess.run([str(build / 'service.test'), '-test.run', '^' + fixture_name + '$', '-test.v', '-test.timeout', '3m'],
                                 cwd=ROOT / 'hyjs-server/service', env=env, text=True, capture_output=True, timeout=200)
        (evidence / 'fixture.log').write_text(pool.redact(fixture.stdout + fixture.stderr))
        if fixture.returncode != 0 or not credentials.is_file():
            raise RuntimeError('Isolated phone fixture failed; inspect sanitized fixture.log')
        login = json.loads(credentials.read_text())
        if login['schema'] != connection.database or login['playerId'] <= 1:
            raise RuntimeError('Isolated login identity mismatch')
        binary = build / 'hyjs-server'
        log = (evidence / 'server.stdout.log').open('w')
        process = subprocess.Popen([str(binary), '--env', 'test', '--bind', '127.0.0.1', '--advertise-host', '127.0.0.1',
                                    '--http-port', '18861', '--tcp-port', '18862', '--server-id', '1', '--tables', str(ROOT / 'analysis/tables'),
                                    '--enable-test-payments', '--enable-local-wallet', '--log-file', str(evidence / 'app.log')],
                                   cwd=ROOT / 'hyjs-server', env=env, stdout=log, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 60
        ready = False
        while time.monotonic() < deadline:
            if process.poll() is not None:
                raise RuntimeError('Owned QA server exited during startup')
            try:
                with socket.create_connection(('127.0.0.1', 18861), timeout=.5):
                    ready = True
                    break
            except OSError:
                time.sleep(.2)
        if not ready:
            raise RuntimeError('Owned QA server did not become ready')
        metadata = {'schema': connection.database, 'playerId': login['playerId'], 'accountId': login['accountId'],
                    'httpPort': 18861, 'tcpPort': 18862, 'pid': process.pid, 'binarySHA256': hashlib.sha256(binary.read_bytes()).hexdigest(),
                    'privateLoginFile': str(credentials), 'finiteFixture': login['fixture'], 'productionAccess': False}
        (evidence / 'runtime.json').write_text(json.dumps(metadata, indent=2))
        print(json.dumps({'ready': True, **metadata}), flush=True)
        stopped = False
        def stop(_signum, _frame):
            nonlocal stopped
            stopped = True
        signal.signal(signal.SIGTERM, stop)
        signal.signal(signal.SIGINT, stop)
        deadline = time.monotonic() + 3600
        with selectors.DefaultSelector() as waiting:
            waiting.register(sys.stdin, selectors.EVENT_READ)
            while not stopped and time.monotonic() < deadline:
                if process.poll() is not None:
                    raise RuntimeError('Owned QA server stopped unexpectedly')
                if not waiting.select(1):
                    continue
                command = sys.stdin.readline().strip()
                if command in ('', 'quit', 'exit'):
                    break
                if command == 'status':
                    query = "SELECT source_key,claimed_at>0 FROM p1_reward_deliveries WHERE player_id=" + str(login['playerId']) + " ORDER BY id;"
                    print(json.dumps({'ownQARewardStatus': pool.sql(connection.database, query)}), flush=True)
    finally:
        if process is not None and process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
        if log:
            log.close()
        credentials.unlink(missing_ok=True)
        pool.close()
        (evidence / 'cleanup.json').write_text(json.dumps({'ownedServerStopped': process is None or process.poll() is not None,
                                                         'ownedAllocationsReleased': not bool(pool.connections), 'privateLoginRemoved': not credentials.exists()}, indent=2))
        print('Owned activity77 QA application, database and account released', flush=True)


if __name__ == '__main__':
    main()
