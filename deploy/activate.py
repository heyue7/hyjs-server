"""Install one verified artifact set in place, preserving all persistent data."""
from pathlib import Path
import argparse
import fcntl
import gzip
import hashlib
import ipaddress
import json
import os
import shutil
import subprocess
import time
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument('stage')
parser.add_argument('--initial-db', action='store_true')
parser.add_argument('--provision-ingress', action='store_true')
parser.add_argument('--migrate-role-wallets', action='store_true')
parser.add_argument('--expected-live-revision')
parser.add_argument('--apply-growth74', action='store_true')
parser.add_argument('--apply-growth75', action='store_true')
parser.add_argument('--restore-activity-calendar')
parser.add_argument('--warm-rush-history-player', action='append', type=int, default=[])
args = parser.parse_args()
if args.restore_activity_calendar:
    import uuid
    assert str(uuid.UUID(args.restore_activity_calendar)) == args.restore_activity_calendar
    assert args.expected_live_revision and not any((args.initial_db, args.migrate_role_wallets, args.apply_growth74, args.apply_growth75)), 'Calendar restore requires a standalone maintenance cutover'
assert not (args.apply_growth74 and args.apply_growth75), 'Choose one explicit growth deployment version'
if args.apply_growth75:
    assert args.expected_live_revision and not args.initial_db and not args.migrate_role_wallets, 'Growth75 requires a pinned maintenance cutover'
os.umask(0o077)
stage = Path(args.stage).resolve()
ROOT = stage.parent.parent
assert stage.parent == ROOT / '.incoming' and (ROOT / '.hyjs-owned').is_file()
manifest = json.loads((stage / 'artifact.json').read_text())
deployment_lock = (ROOT / '.deploy.lock').open('a')
fcntl.flock(deployment_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
if args.expected_live_revision:
    assert json.loads((ROOT / 'deployed.json').read_text())['revision'] == args.expected_live_revision, 'Live revision changed; rebase and verify first'

def sha(path):
    digest = hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            digest.update(block)
    return digest.hexdigest()

def run(*cmd, **kwargs):
    try:
        return subprocess.run(cmd, check=True, capture_output=True, **kwargs)
    except subprocess.CalledProcessError as error:
        detail = ((error.stdout or b'') + (error.stderr or b'')).decode('utf-8', errors='replace')
        secret_file = ROOT / '.env'
        if secret_file.is_file():
            for line in secret_file.read_text().splitlines():
                key, sep, value = line.partition('=')
                if sep and value and any(word in key for word in ('PASSWORD', 'SECRET', 'KEY', 'TOKEN')):
                    detail = detail.replace(value, '[REDACTED]')
        raise RuntimeError('Deployment command failed: ' + detail[-5000:]) from None

def compose(*cmd):
    return run('docker', 'compose', '--project-directory', str(ROOT), '--env-file', str(ROOT / '.env'), '-f', str(ROOT / 'compose.yaml'), *cmd)

def mysql(query):
    return run('docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
               'MYSQL_PWD="$MYSQL_PASSWORD" exec mysql -uhyjs --default-character-set=utf8mb4 --binary-mode --batch --raw --skip-column-names hyjs',
               input=query.encode()).stdout.decode()


def backup_existing_database():
    # Keep historical snapshots untouched; new snapshots are authenticated-encrypted.
    backup_dir = ROOT / 'backups'
    assert backup_dir.is_dir() and not backup_dir.is_symlink()
    os.chmod(backup_dir, 0o700)
    key_file = ROOT / '.backup-encryption.key'
    if not key_file.exists():
        fd = os.open(key_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'wb') as target:
            target.write(os.urandom(32))
    assert not key_file.is_symlink() and key_file.stat().st_mode & 0o777 == 0o600 and key_file.stat().st_size == 32
    name = 'pre-deploy-' + manifest['revision'][:12] + '-' + str(time.time_ns()) + '.sql.gz'
    plain = backup_dir / (name + '.pending')
    encrypted = backup_dir / (name + '.aesgcm')
    process = subprocess.Popen(['docker', 'exec', 'hyjs-mysql', 'sh', '-c',
                                'MYSQL_PWD="$MYSQL_PASSWORD" exec mysqldump -uhyjs --single-transaction --hex-blob --no-tablespaces --set-gtid-purged=OFF hyjs'], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    with gzip.open(plain, 'xb') as target:
        shutil.copyfileobj(process.stdout, target)
    assert process.wait() == 0, 'Backup failed; pending snapshot retained privately'
    run(str(stage / 'app/hyjs-backupseal'), '--input', str(plain), '--output', str(encrypted), '--key-file', str(key_file))
    # Authenticate the complete encrypted stream and original digest without a third disk copy.
    run(str(stage / 'app/hyjs-backupseal'), '--input', str(encrypted), '--key-file', str(key_file), '--verify-sha256', sha(plain))
    plain.unlink()
    backup_record = {'file': encrypted.name, 'sha256': sha(encrypted), 'format': 'HYJSBK02-AES256GCM-CHUNKED', 'revisionBefore': args.expected_live_revision or 'preserved running image'}
    (ROOT / 'backup-latest.json').write_text(json.dumps(backup_record, indent=2))
    return backup_record


for name, digest in manifest['files'].items():
    path = stage / name
    assert path.resolve().is_relative_to(stage) and sha(path) == digest, name
if not args.provision_ingress:
    assert Path('/etc/nginx/sites-enabled/hyjs-test.conf').read_bytes() == (stage / 'nginx.conf').read_bytes(), 'Ingress differs; explicit provision-ingress required'
if (ROOT / 'migration.json').exists():
    latest_backup = ROOT / 'backup-latest.json'
    previous_backup = (ROOT / 'backups' / json.loads(latest_backup.read_text())['file']
                       if latest_backup.exists() else ROOT / 'backups/initial-migration.sql.gz')
    if previous_backup.is_file():
        required_backup_space = previous_backup.stat().st_size * 2
    else:
        # A previous snapshot may have been moved off-host. Use a conservative
        # estimate for the new plaintext and encrypted snapshots, including hex dumps.
        database_bytes = int(mysql("SELECT COALESCE(SUM(data_length + index_length), 0) FROM information_schema.tables WHERE table_schema=DATABASE();").strip())
        assert database_bytes > 0, 'Cannot estimate database size for backup'
        required_backup_space = database_bytes * 4
    assert shutil.disk_usage(ROOT).free > required_backup_space + 400 * 1024 * 1024, 'Insufficient peak space for verified encrypted cutover backup'
# Build before the maintenance window; MySQL and the old application stay live.
shutil.copy2('/etc/ssl/certs/ca-certificates.crt', stage / 'ca-certificates.crt')
old_image = subprocess.run(['docker', 'inspect', '--format={{.Image}}', 'hyjs-game'], capture_output=True, text=True).stdout.strip()
run('docker', 'build', '--build-arg', 'REVISION=' + manifest['revision'], '-t', 'hyjs-game:current', str(stage))
# Deployment topology may change; preserve all existing database credentials.
runtime = {}
for line in (stage / 'deployment.env').read_text().splitlines():
    key, separator, value = line.partition('=')
    assert separator and key in ('HYJS_PUBLIC_HOST', 'HYJS_HTTP_PORT', 'HYJS_ADMIN_PORT', 'HYJS_TCP_PORT', 'HYJS_UPSTREAM_PORT', 'HYJS_API_ORIGIN', 'HYJS_ADMIN_ORIGIN') and value
    runtime[key] = value
private_env = ROOT / '.env'
assert private_env.is_file() and not private_env.stat().st_mode & 0o077
kept = [line for line in private_env.read_text().splitlines() if line.partition('=')[0] not in runtime]
# Persist this application-only signing secret once; never emit or put it in the image.
certificate_keys = [line.partition('=')[2] for line in kept if line.startswith('HYJS_RUSH_HISTORY_CERTIFICATE_KEY=')]
assert len(certificate_keys) <= 1
if not certificate_keys:
    kept.append('HYJS_RUSH_HISTORY_CERTIFICATE_KEY=' + os.urandom(32).hex())
else:
    assert len(certificate_keys[0]) >= 32, 'Trial history certificate key is too short'
temporary_env = ROOT / '.env.next'
fd = os.open(temporary_env, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
with os.fdopen(fd, 'w') as output:
    output.write('\n'.join(kept) + '\n' + ''.join(key + '=' + value + '\n' for key, value in runtime.items()))
os.chmod(temporary_env, 0o600)
os.replace(temporary_env, private_env)
# Verify the encrypted snapshot before Compose can restart MySQL for new limits.
wallet_baseline = None
verified_backup = None
if args.restore_activity_calendar:
    compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--tables', '/app/tables', '--calendar-maintenance', 'begin', '--log-file', '/app/logs/calendar-maintenance.log')
    assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1'
    compose('stop', '-t', '30', 'app')
if args.apply_growth74 or args.apply_growth75:
    assert args.expected_live_revision and not args.initial_db
    assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Growth apply requires maintenance'
    compose('stop', '-t', '30', 'app')
if args.migrate_role_wallets:
    assert args.expected_live_revision and not args.initial_db and (ROOT / 'migration.json').exists()
    import role_wallet_migration
    role_wallet_migration.before(mysql)
    compose('stop', '-t', '30', 'app')
    wallet_baseline = role_wallet_migration.before(mysql)
    (ROOT / 'role-wallet-cutover-before.json').write_text(json.dumps(wallet_baseline, indent=2))
if (ROOT / 'migration.json').exists():
    verified_backup = backup_existing_database()
shutil.copy2(stage / 'compose.yaml', ROOT / 'compose.yaml')
compose('up', '-d', 'mysql')
for _ in range(80):
    try:
        mysql('SELECT 1;')
        break
    except subprocess.CalledProcessError:
        time.sleep(2)
else:
    raise SystemExit('HYJS MySQL did not become ready')

# Trust only the actual host bridge gateway used by this owned deployment.
network_info = json.loads(run('docker', 'inspect', 'hyjs-mysql').stdout)[0]['NetworkSettings']['Networks']
gateways = sorted({str(ipaddress.ip_address(item['Gateway'])) for item in network_info.values() if item.get('Gateway')})
assert gateways, 'Cannot identify the owned reverse-proxy gateway'
current_lines = [line for line in private_env.read_text().splitlines() if not line.startswith('HYJS_TRUSTED_PROXIES=')]
current_lines.append('HYJS_TRUSTED_PROXIES=' + ','.join(gateways))
with temporary_env.open('w') as output:
    output.write('\n'.join(current_lines) + '\n')
os.chmod(temporary_env, 0o600)
os.replace(temporary_env, private_env)

already_imported = (ROOT / 'migration.json').exists()
if args.initial_db and already_imported:
    previous = json.loads((ROOT / 'migration.json').read_text())
    expected = json.loads((ROOT / '.incoming/initial-verification.json').read_text())
    assert previous['sourceSHA256'] == expected['dumpSHA256'], 'Refuse a different initial import'
    assert sha(ROOT / '.incoming/initial.sql.gz') == previous['sourceSHA256']
    (ROOT / '.incoming/initial.sql.gz').unlink()
    (ROOT / '.incoming/initial-verification.json').unlink()
if args.initial_db and not already_imported:
    count = mysql('SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE();').strip()
    assert count == '0', 'Refuse initial import over an existing database'
    incoming = ROOT / '.incoming/initial.sql.gz'
    expected = json.loads((ROOT / '.incoming/initial-verification.json').read_text())
    assert sha(incoming) == expected['dumpSHA256']
    with gzip.open(incoming, 'rb') as source:
        process = subprocess.Popen(['docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                                    'MYSQL_PWD="$MYSQL_PASSWORD" exec mysql -uhyjs --binary-mode hyjs'], stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        shutil.copyfileobj(source, process.stdin)
        process.stdin.close()
        process.stderr.read()
    if process.wait():
        raise SystemExit('Initial restore failed; preserve imported database for inspection')
    for table, wanted in expected['counts'].items():
        assert table.replace('_', '').isalnum()
    counts_query = ' UNION ALL '.join("SELECT '" + table + "', COUNT(*) FROM `" + table + '`' for table in expected['counts']) + ';'
    counts = dict((line.split('\t')[0], int(line.split('\t')[1])) for line in mysql(counts_query).strip().splitlines())
    assert counts == expected['counts'], 'Restored table counts differ'
    actual = mysql(expected['profileQuery'])
    assert hashlib.sha256(actual.encode()).hexdigest() == expected['profileSHA256'], 'Profile bytes changed'
    os.replace(incoming, ROOT / 'backups/initial-migration.sql.gz')
    os.chmod(ROOT / 'backups/initial-migration.sql.gz', 0o600)
    (ROOT / 'migration.json').write_text(json.dumps({'sourceSHA256': expected['dumpSHA256'], 'tables': len(expected['counts']), 'profilesExact': True}, indent=2))
    (ROOT / '.incoming/initial-verification.json').unlink()
elif not (ROOT / 'migration.json').exists():
    raise SystemExit('Initial data must be explicitly imported before application start')

# Derived certificates are prepared with the exact candidate while the old app remains live.
# This command opens no listener and cannot alter resources, progress or battle receipts.
for player_id in args.warm_rush_history_player:
    assert player_id > 0
    compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--tables', '/app/tables',
            '--warm-rush-history-player', str(player_id), '--log-file', '/app/logs/rush-history-warm.log')
manifest['prewarmedRushPlayers'] = sorted(set(args.warm_rush_history_player))

if args.apply_growth74:
    compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--tables', '/app/tables',
            '--enable-test-payments', '--enable-local-wallet', '--apply-growth74', '--log-file', '/app/logs/growth74-apply.log')
    assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1'
    manifest['growth74Applied'] = True

if args.apply_growth75:
    import growth75_activation
    try:
        manifest['growth75Applied'] = growth75_activation.activate_growth75(
            mysql, compose, stage / 'growth75_activity_patch.py', ROOT / 'backups', manifest['revision'],
            verified_backup)
    except Exception:
        compose('stop', '-t', '30', 'app')
        raise RuntimeError('Growth75 failed under maintenance. App remains stopped; inspect private backups/growth75-*/manifest.json. Any failed uncommitted CAS transaction was rolled back; prior candidate CLI steps remain audited. Do not reopen or auto-restore the full database.') from None

if args.restore_activity_calendar:
    import calendar_restore_activation
    try:
        manifest['calendarRestore'] = calendar_restore_activation.activate_calendar_restore(mysql, compose, ROOT / 'backups', manifest['revision'], verified_backup, args.restore_activity_calendar)
    except Exception as error:
        compose('stop', '-t', '30', 'app')
        raise RuntimeError('Calendar restore did not finish: ' + str(error) + '. Application remains stopped under maintenance; preserve the authenticated backup and calendar manifest, inspect the failure and retry the same request. Never auto-restore the database.') from None

compose('up', '-d', '--no-deps', '--force-recreate', 'app')
for _ in range(90):
    try:
        with urllib.request.urlopen('http://127.0.0.1:' + runtime['HYJS_UPSTREAM_PORT'] + '/health', timeout=2) as response:
            assert json.load(response)['status'] == 'ok'
        break
    except Exception:
        time.sleep(2)
else:
    if args.migrate_role_wallets:
        compose('stop', '-t', '30', 'app')
    raise SystemExit('Application health failed; data and prior deployed.json preserved. Fix/retry this script, or deploy its recorded Git commit from a clean checkout; do not restore the database automatically.')

if wallet_baseline is not None:
    try:
        reconciled = role_wallet_migration.after(mysql, wallet_baseline)
        (ROOT / 'role-wallet-cutover-after.json').write_text(json.dumps(reconciled, indent=2))
        manifest['roleWalletCutover'] = reconciled
    except Exception:
        compose('stop', '-t', '30', 'app')
        raise SystemExit('Role wallet reconciliation failed. App stopped; preserve database, backup and evidence. Never start a legacy wallet binary against new role transactions.')

if args.provision_ingress:
    destination = Path('/etc/nginx/sites-available/hyjs-test.conf')
    desired = (stage / 'nginx.conf').read_bytes()
    changed = not destination.exists() or destination.read_bytes() != desired
    destination.write_bytes(desired)
    link = Path('/etc/nginx/sites-enabled/hyjs-test.conf')
    if not link.exists():
        link.symlink_to(destination)
        changed = True
    assert link.resolve() == destination
    run('nginx', '-t')
    if changed:
        run('systemctl', 'reload', 'nginx')
else:
    assert Path('/etc/nginx/sites-enabled/hyjs-test.conf').is_file()
    assert Path('/etc/nginx/sites-enabled/hyjs-test.conf').read_bytes() == (stage / 'nginx.conf').read_bytes(), 'Ingress differs; explicit provision-ingress required'

# One application artifact directory; no timestamped release copies.
target = ROOT / 'app'
if target.exists():
    assert target.is_dir() and not target.is_symlink()
    shutil.rmtree(target)
os.replace(stage / 'app', target)
installed = run('docker', 'inspect', '--format={{index .Config.Labels "org.opencontainers.image.revision"}}', 'hyjs-game').stdout.decode().strip()
assert installed == manifest['revision']
manifest['deployedAt'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
(ROOT / 'deployed.json').write_text(json.dumps(manifest, indent=2))
shutil.rmtree(stage)
if old_image:
    references = run('docker', 'ps', '-a', '--filter', 'ancestor=' + old_image, '--format={{.ID}}').stdout.strip()
    if not references:
        subprocess.run(['docker', 'image', 'rm', old_image], capture_output=True)
print(json.dumps({'status': 'DEPLOYED', 'revision': installed, 'binarySHA256': manifest['files']['app/hyjs-server']}))
