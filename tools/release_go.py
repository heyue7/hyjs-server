"""Publish a tested local binary with backup, process ownership and hash checks."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import signal
import subprocess
import sys
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.shared_mysql import dump as shared_dump, settings as shared_settings, sql as shared_sql


def digest(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def sql(query, database):
    return shared_sql(query, database).strip()


def state(database):
    # Preserve the historical account byte for byte; inspect only bounded demo fields.
    original = sql('SELECT SHA2(player,256),SHA2(game,256) FROM profiles WHERE id=1;', database).split()
    demo = json.loads(sql('SELECT player FROM profiles WHERE id=4;', database))
    game = json.loads(sql('SELECT game FROM profiles WHERE id=4;', database))
    return {'original': original,
            'demo': {key: demo.get(key) for key in ('coin', 'diamond', 'level', 'fightPower')},
            'demoRunPresent': bool(game.get('run'))}


def operations():
    request = urllib.request.Request('http://127.0.0.1:8088/local/operations',
                                    data=b'{"playerId":4}',
                                    headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(request, timeout=5) as response:
        body = json.load(response)
    if body.get('code') != 0:
        raise RuntimeError('Local release version endpoint failed')
    return body['data']


def stop_owned_process():
    path = ROOT / 'server/data/go-server.pid'
    pid = int(path.read_text().strip())
    info = subprocess.run(['rtk', 'proxy', 'ps', '-p', str(pid), '-o', 'command='],
                          text=True, capture_output=True, check=True).stdout.strip()
    if not info.startswith(str(ROOT / 'build/hyjs-server') + ' '):
        raise RuntimeError('Saved PID does not identify the project server')
    os.kill(pid, signal.SIGTERM)
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            return
        time.sleep(.1)
    raise RuntimeError('Local server did not stop within 20 seconds')


def current_advertise_host():
    pid = int((ROOT / 'server/data/go-server.pid').read_text().strip())
    result = subprocess.run(['rtk', 'proxy', 'ps', '-p', str(pid), '-o', 'command='],
                            text=True, capture_output=True, check=True)
    arguments = shlex.split(result.stdout.strip())
    if not arguments or arguments[0] != str(ROOT / 'build/hyjs-server'):
        raise RuntimeError('Saved PID does not identify the project server')
    if '--advertise-host' not in arguments:
        raise RuntimeError('Running address is unknown; supply --host explicitly')
    index = arguments.index('--advertise-host')
    if index + 1 >= len(arguments):
        raise RuntimeError('Running advertised address is missing')
    return arguments[index + 1]


def current_wallet_enabled():
    pid = int((ROOT / 'server/data/go-server.pid').read_text().strip())
    result = subprocess.run(['rtk', 'proxy', 'ps', '-p', str(pid), '-o', 'command='],
                            text=True, capture_output=True, check=True)
    arguments = shlex.split(result.stdout.strip())
    if not arguments or arguments[0] != str(ROOT / 'build/hyjs-server'):
        raise RuntimeError('Saved PID does not identify the project server')
    return '--enable-local-wallet' in arguments or '--enable-local-wallet=true' in arguments


def start(host, wallet=False):
    arguments = ['rtk', 'proxy', 'python3', 'tools/run_go_server.py', '--daemon',
                             '--bind', '0.0.0.0', '--advertise-host', host,
                             '--http-port', '8088', '--enable-test-payments']
    if wallet:
        arguments.append('--enable-local-wallet')
    result = subprocess.run(arguments,
                            cwd=ROOT, capture_output=True, text=True, timeout=100)
    if result.returncode:
        raise RuntimeError('Local server startup failed; inspect its private console')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--candidate', type=Path, required=True)
    parser.add_argument('--label', required=True)
    parser.add_argument('--host', help='Override the running advertised address; omitted preserves it')
    parser.add_argument('--record', type=Path, required=True)
    parser.add_argument('--enable-local-wallet', action=argparse.BooleanOptionalAction, default=None,
                        help='Opt in/out of demo wallet payments; omitted preserves the current startup setting')
    args = parser.parse_args()
    if not re.fullmatch('[a-z0-9-]{1,64}', args.label):
        raise RuntimeError('Invalid release label')
    database = shared_settings()['MYSQL_DATABASE']
    if not re.fullmatch('[A-Za-z0-9_]+', database):
        raise RuntimeError('Invalid configured database identifier')
    current = ROOT / 'build/hyjs-server'
    candidate = args.candidate.resolve()
    if candidate == current or not candidate.is_file():
        raise RuntimeError('Candidate must be a distinct existing binary')
    expected = digest(candidate)
    previous = digest(current)
    host = args.host or current_advertise_host()
    previous_wallet = current_wallet_enabled()
    wallet = previous_wallet if args.enable_local_wallet is None else args.enable_local_wallet
    if operations()['binarySha256'] != previous:
        raise RuntimeError('Running version differs from saved current binary')
    before = state(database)
    if before['demoRunPresent']:
        raise RuntimeError('Demo has an active run; preserve the live battle before publishing')
    timestamp = time.strftime('%Y%m%d-%H%M%S')
    backup = ROOT / 'server/data/backups' / ('pre-' + args.label + '-' + timestamp + '.sql')
    backup.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(backup, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'wb') as output:
        shared_dump(output, database)
    if backup.stat().st_size == 0:
        raise RuntimeError('Local backup failed; server remains running')
    rollback = ROOT / 'build' / ('hyjs-server-pre-' + args.label)
    staging = ROOT / 'build/hyjs-server-promote'
    staging.write_bytes(candidate.read_bytes())
    staging.chmod(0o755)
    stop_owned_process()
    current.replace(rollback)
    staging.replace(current)
    try:
        start(host, wallet)
        live = operations()['binarySha256']
        if live != expected:
            raise RuntimeError('Published server hash mismatch')
    except Exception:
        # Stop only a process created by our launcher, then restore the prior binary.
        try:
            stop_owned_process()
        except (ProcessLookupError, subprocess.CalledProcessError, FileNotFoundError):
            pass
        current.replace(candidate.with_name(candidate.name + '-failed'))
        rollback.replace(current)
        start(host, previous_wallet)
        raise
    after = state(database)
    if before['original'] != after['original'] or before['demo'] != after['demo']:
        raise RuntimeError('Release changed protected account state; inspect backup before further actions')
    record = {'candidate': str(candidate), 'backup': str(backup),
              'advertiseHost': host,
              'localWalletEnabled': wallet,
              'previousLocalWalletEnabled': previous_wallet,
              'serverSha256': expected, 'previousSha256': previous,
              'liveSha256Verified': True, 'before': before, 'after': after}
    args.record.parent.mkdir(parents=True, exist_ok=True)
    args.record.write_text(json.dumps(record, indent=2) + '\n')
    print(json.dumps({'published': expected, 'backup': str(backup), 'record': str(args.record.resolve())}))


if __name__ == '__main__':
    main()
