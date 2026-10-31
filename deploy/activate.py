"""Install one verified artifact set in place, preserving all persistent data."""
from pathlib import Path
import argparse
import atexit
import fcntl
import gzip
import hashlib
import ipaddress
import json
import os
import re
import shutil
import subprocess
import time
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument('stage')
parser.add_argument('--initial-db', action='store_true')
parser.add_argument('--provision-ingress', action='store_true')
parser.add_argument('--with-backup', action='store_true', help='Create and authenticate a database backup before an existing deployment')
parser.add_argument('--migrate-role-wallets', action='store_true')
parser.add_argument('--expected-live-revision')
parser.add_argument('--apply-growth74', action='store_true')
parser.add_argument('--apply-activity77', action='store_true', help='Apply activity77 under maintenance')
parser.add_argument('--apply-feedback78', action='store_true', help='Apply the reviewed feedback78 configuration under maintenance')
parser.add_argument('--apply-feedback79', action='store_true', help='Apply the reviewed feedback79 configuration under maintenance')
parser.add_argument('--apply-feedback80', action='store_true', help='Apply the reviewed feedback80 configuration under maintenance')
parser.add_argument('--apply-feedback81', action='store_true', help='Apply the reviewed feedback81 configuration under maintenance')
parser.add_argument('--apply-trial82', action='store_true', help='Apply the reviewed trial82 configuration under maintenance')
parser.add_argument('--apply-feedback84', action='store_true', help='Apply the reviewed feedback84 configuration under maintenance')
parser.add_argument('--apply-feedback85', action='store_true', help='Apply reviewed training and awakening configuration under maintenance')
parser.add_argument('--apply-growth75', action='store_true')
parser.add_argument('--maintenance-redeploy', action='store_true')
parser.add_argument('--restore-activity-calendar')
parser.add_argument('--calendar-formal-110', action='store_true')
parser.add_argument('--warm-rush-history-player', action='append', type=int, default=[])
args = parser.parse_args()
if not args.with_backup and any((args.migrate_role_wallets, args.restore_activity_calendar, args.calendar_formal_110,
                                 args.apply_growth74, args.apply_growth75, args.apply_activity77,
                                 args.apply_feedback78, args.apply_feedback79, args.apply_feedback80,
                                 args.apply_feedback81, args.apply_trial82, args.apply_feedback84,
                                 args.apply_feedback85, args.maintenance_redeploy)):
    parser.error('This migration, publication or maintenance redeploy requires explicit --with-backup')
if args.restore_activity_calendar:
    import uuid
    assert str(uuid.UUID(args.restore_activity_calendar)) == args.restore_activity_calendar
    assert args.expected_live_revision and not any((args.initial_db, args.migrate_role_wallets, args.apply_growth74, args.apply_growth75, args.apply_activity77, args.apply_feedback78, args.apply_feedback79, args.apply_feedback80, args.apply_feedback81, args.apply_trial82, args.apply_feedback84, args.apply_feedback85)), 'Calendar restore requires a standalone maintenance cutover'
if args.calendar_formal_110:
    if not args.restore_activity_calendar or not args.expected_live_revision or not re.fullmatch(r'(?:[0-9a-f]{40}|[0-9a-f]{64})', args.expected_live_revision):
        raise ValueError('Formal 110 requires a calendar request and exact live revision')
    if any((args.initial_db, args.provision_ingress, args.migrate_role_wallets, args.warm_rush_history_player,
            args.apply_growth74, args.apply_growth75, args.apply_activity77, args.apply_feedback78,
            args.apply_feedback79, args.apply_feedback80, args.apply_feedback81, args.apply_trial82,
            args.apply_feedback84, args.apply_feedback85, args.maintenance_redeploy)):
        raise ValueError('Formal 110 requires a standalone calendar cutover')
assert sum((args.apply_growth74, args.apply_growth75, args.apply_activity77, args.apply_feedback78, args.apply_feedback79, args.apply_feedback80, args.apply_feedback81, args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85)) <= 1, 'Choose one explicit maintenance deployment version'
if args.apply_activity77:
    assert args.expected_live_revision and not args.initial_db and not args.migrate_role_wallets, 'Activity77 requires a pinned maintenance cutover'
if args.apply_growth75:
    assert args.expected_live_revision and not args.initial_db and not args.migrate_role_wallets, 'Growth75 requires a pinned maintenance cutover'
if args.apply_feedback78:
    assert args.expected_live_revision and re.fullmatch(r'[0-9a-f]{40,64}', args.expected_live_revision), 'Feedback78 requires an exact live revision'
    assert not any((args.initial_db, args.provision_ingress, args.migrate_role_wallets, args.warm_rush_history_player)), 'Feedback78 requires a standalone maintenance cutover'
if args.apply_feedback79:
    assert args.expected_live_revision and re.fullmatch(r'[0-9a-f]{40,64}', args.expected_live_revision), 'Feedback79 requires an exact live revision'
    assert not any((args.initial_db, args.provision_ingress, args.migrate_role_wallets, args.warm_rush_history_player)), 'Feedback79 requires a standalone maintenance cutover'
if args.apply_feedback80:
    assert args.expected_live_revision and re.fullmatch(r'[0-9a-f]{40,64}', args.expected_live_revision), 'Feedback80 requires an exact live revision'
    assert not any((args.initial_db, args.provision_ingress, args.migrate_role_wallets, args.warm_rush_history_player)), 'Feedback80 requires a standalone maintenance cutover'
if args.apply_feedback81:
    assert args.expected_live_revision and re.fullmatch(r'[0-9a-f]{40,64}', args.expected_live_revision), 'Feedback81 requires an exact live revision'
    assert not any((args.initial_db, args.provision_ingress, args.migrate_role_wallets, args.warm_rush_history_player)), 'Feedback81 requires a standalone maintenance cutover'
if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85:
    assert args.expected_live_revision and re.fullmatch(r'(?:[0-9a-f]{40}|[0-9a-f]{64})', args.expected_live_revision), 'Trial82/feedback84 requires an exact live revision'
    assert not any((args.initial_db, args.migrate_role_wallets, args.warm_rush_history_player)), 'Trial82/feedback84 requires an existing maintenance cutover'
if args.maintenance_redeploy:
    assert args.expected_live_revision and re.fullmatch(r'(?:[0-9a-f]{40}|[0-9a-f]{64})', args.expected_live_revision), 'Maintenance redeploy requires an exact live revision'
    assert not any((args.initial_db, args.provision_ingress, args.migrate_role_wallets, args.warm_rush_history_player,
                    args.restore_activity_calendar, args.apply_growth74, args.apply_growth75, args.apply_activity77,
                    args.apply_feedback78, args.apply_feedback79, args.apply_feedback80, args.apply_feedback81,
                    args.apply_trial82, args.apply_feedback84, args.apply_feedback85)), 'Maintenance redeploy must not change configuration or ingress'
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
    backup_pointer.write_backup_pointer(ROOT / 'backup-latest.json', backup_record)
    return backup_record


if getattr(args, 'calendar_formal_110', False):
    # Verify the guard module before importing it, then arm the guard before
    # further artifact checks, image builds, backup, and the calendar operation.
    guard_module = stage / 'calendar_restore_activation.py'
    assert 'calendar_restore_activation.py' in manifest['files'] and sha(guard_module) == manifest['files']['calendar_restore_activation.py'], 'Formal 110 guard artifact differs'
    import calendar_restore_activation
    calendar_restore_activation.validate_formal110_arguments(args, remote=True)
    assert (ROOT / 'migration.json').is_file() and not (ROOT / 'migration.json').is_symlink(), 'Formal 110 requires an existing initialized database'
    live_revision = run('docker', 'inspect', '--format={{index .Config.Labels "org.opencontainers.image.revision"}}', 'hyjs-game').stdout.decode().strip()
    assert live_revision == args.expected_live_revision, 'Formal 110 live container revision differs from deployment record'
    formal110_guard = calendar_restore_activation.Formal110CutoverGuard(
        compose, mysql, run, ROOT / 'backups', manifest['revision'], args.expected_live_revision,
        args.restore_activity_calendar)
    atexit.register(formal110_guard.stop_if_unfinished)
    formal110_guard.install_signal_handlers()
    formal110_guard.prepare()
    formal110_guard.checkpoint('checking-maintenance-and-migration')
    calendar_restore_activation.require_maintenance(mysql)
    formal110_guard.checkpoint('checking-vip10-before-work')
    calendar_restore_activation.require_vip10_approved(mysql)

for name, digest in manifest['files'].items():
    path = stage / name
    assert path.resolve().is_relative_to(stage) and sha(path) == digest, name
assert 'backup_pointer.py' in manifest['files'], 'Backup pointer helper missing from verified artifact'
import backup_pointer
if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    assert 'trial82_activation.py' in manifest['files'], 'Trial82/feedback84 activation missing from verified artifact'
    if args.provision_ingress:
        import trial82_activation
        trial82_activation.require_existing_ingress(Path('/etc/nginx/sites-available/hyjs-test.conf'), Path('/etc/nginx/sites-enabled/hyjs-test.conf'))
        assert 'ingress_activation.py' in manifest['files'], 'Ingress activation missing from verified artifact'
        import ingress_activation
        # Syntax-only isolated preflight; the real full configuration is checked
        # after installation. This runs before arming the maintenance stop guard.
        ingress_desired = ingress_activation.preflight(stage / 'nginx.conf', manifest['files']['nginx.conf'], run)
if not args.provision_ingress:
    assert Path('/etc/nginx/sites-enabled/hyjs-test.conf').read_bytes() == (stage / 'nginx.conf').read_bytes(), 'Ingress differs; explicit provision-ingress required'
if args.with_backup and (ROOT / 'migration.json').exists():
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
if args.maintenance_redeploy:
    live_revision = run('docker', 'inspect', '--format={{index .Config.Labels "org.opencontainers.image.revision"}}', 'hyjs-game').stdout.decode().strip()
    assert live_revision == args.expected_live_revision, 'Live container revision differs from deployment record'
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
if getattr(args, 'calendar_formal_110', False):
    formal110_guard.checkpoint('checking-existing-runtime')
    current_runtime = {}
    for line in private_env.read_text().splitlines():
        key, separator, value = line.partition('=')
        if separator and key in runtime:
            assert key not in current_runtime, 'Formal 110 runtime key is duplicated'
            current_runtime[key] = value
    assert current_runtime == runtime, 'Formal 110 must preserve the existing runtime topology'
    assert (ROOT / 'compose.yaml').is_file() and not (ROOT / 'compose.yaml').is_symlink() and \
        (ROOT / 'compose.yaml').read_bytes() == (stage / 'compose.yaml').read_bytes(), 'Formal 110 must preserve existing Compose configuration'
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
# When requested, verify the encrypted snapshot before Compose can restart MySQL.
wallet_baseline = None
verified_backup = None
if args.restore_activity_calendar:
    if getattr(args, 'calendar_formal_110', False):
        # This mode requires pre-existing maintenance and never toggles it.
        formal110_guard.checkpoint('stopping-application')
        calendar_restore_activation.require_maintenance(mysql)
        compose('stop', '-t', '30', 'app')
        formal110_guard.require_stopped()
        calendar_restore_activation.require_maintenance(mysql)
    else:
        compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--tables', '/app/tables', '--calendar-maintenance', 'begin', '--log-file', '/app/logs/calendar-maintenance.log')
        assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1'
        compose('stop', '-t', '30', 'app')
if args.apply_growth74 or args.apply_growth75 or args.apply_activity77 or args.apply_feedback78 or args.apply_feedback79 or args.apply_feedback80 or args.apply_feedback81 or args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    assert args.expected_live_revision and not args.initial_db
    if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
        assert (ROOT / 'migration.json').is_file() and not (ROOT / 'migration.json').is_symlink(), 'Trial82/feedback84 requires an existing database'
    assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Growth apply requires maintenance'
    if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
        import trial82_activation
        trial82_guard = trial82_activation.Trial82CutoverGuard(
            compose, ROOT / 'backups', manifest['revision'], manifest['files']['app/hyjs-server'], args.expected_live_revision,
            mode='maintenance-redeploy' if args.maintenance_redeploy else 'configuration')
        # Validation and private evidence preparation cannot stop the live app.
        # Arm only after these preconditions succeed, before the first stop.
        trial82_guard.prepare()
        trial82_guard.record['requestedCommands'] = [flag for flag, selected in (('--apply-trial82', args.apply_trial82), ('--apply-feedback84', args.apply_feedback84), ('--apply-feedback85', args.apply_feedback85)) if selected]
        trial82_guard.checkpoint()
        trial82_guard.install_signal_handlers()
        atexit.register(trial82_guard.stop_if_unfinished)
    if args.apply_feedback78:
        assert (ROOT / 'migration.json').is_file() and not (ROOT / 'migration.json').is_symlink(), 'Feedback78 requires an existing database and authenticated backup'
        import feedback78_activation
        feedback78_guard = feedback78_activation.Feedback78CutoverGuard(compose)
        # Cover backup, CLI, startup, health and post-health failures until the release is recorded.
        atexit.register(feedback78_guard.stop_if_unfinished)
        feedback78_guard.install_signal_handlers()
    if args.apply_feedback79:
        import feedback79_activation
        feedback79_guard = feedback79_activation.Feedback79CutoverGuard(compose)
        atexit.register(feedback79_guard.stop_if_unfinished)
        feedback79_guard.install_signal_handlers()
    if args.apply_feedback80:
        import feedback80_activation
        feedback80_guard = feedback80_activation.Feedback80CutoverGuard(compose)
        atexit.register(feedback80_guard.stop_if_unfinished)
        feedback80_guard.install_signal_handlers()
    if args.apply_feedback81:
        import feedback81_activation
        feedback81_guard = feedback81_activation.Feedback81CutoverGuard(compose)
        atexit.register(feedback81_guard.stop_if_unfinished)
        feedback81_guard.install_signal_handlers()
    compose('stop', '-t', '30', 'app')
    if args.maintenance_redeploy:
        trial82_guard.record['state'] = 'checking-maintenance-after-stop'
        trial82_guard.checkpoint()
        assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Maintenance changed after stop'
if args.migrate_role_wallets:
    assert args.expected_live_revision and not args.initial_db and (ROOT / 'migration.json').exists()
    import role_wallet_migration
    role_wallet_migration.before(mysql)
    compose('stop', '-t', '30', 'app')
    wallet_baseline = role_wallet_migration.before(mysql)
    (ROOT / 'role-wallet-cutover-before.json').write_text(json.dumps(wallet_baseline, indent=2))
if args.with_backup and (ROOT / 'migration.json').exists():
    if getattr(args, 'calendar_formal_110', False):
        formal110_guard.checkpoint('backing-up')
    if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
        trial82_guard.record['state'] = 'backing-up'
        trial82_guard.checkpoint()
    verified_backup = backup_existing_database()
    if getattr(args, 'calendar_formal_110', False):
        formal110_guard.record['backupBefore'] = verified_backup
        formal110_guard.checkpoint('backup-created')
    if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
        trial82_guard.record['backupBefore'] = verified_backup
        trial82_guard.record['state'] = 'backup-created'
        trial82_guard.checkpoint()
shutil.copy2(stage / 'compose.yaml', ROOT / 'compose.yaml')
compose('up', '-d', 'mysql')
for _ in range(80):
    try:
        mysql('SELECT 1;')
        break
    except (RuntimeError, subprocess.CalledProcessError):
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

# A pure redeploy verifies the backup and maintenance before any candidate CLI.
if args.maintenance_redeploy:
    try:
        applied = trial82_activation.activate_trial82(mysql, compose, trial82_guard, verified_backup,
                                                      apply_trial82=False, apply_feedback84=False, apply_feedback85=False,
                                                      maintenance_redeploy=True)
        manifest['maintenanceRedeployed'] = applied
    except Exception:
        raise RuntimeError('Maintenance redeploy failed. The stop guard will attempt to keep the app stopped; preserve the backup and subsequent writes for inspection.') from None

# Derived certificates are prepared with the exact candidate before application startup.
# This command opens no listener and cannot alter resources, progress or battle receipts.
if getattr(args, 'calendar_formal_110', False):
    formal110_guard.checkpoint('prewarming-rush-history')
for player_id in args.warm_rush_history_player:
    assert player_id > 0
    compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--tables', '/app/tables',
            '--warm-rush-history-player', str(player_id), '--log-file', '/app/logs/rush-history-warm.log')
manifest['prewarmedRushPlayers'] = sorted(set(args.warm_rush_history_player))
# Validator changes invalidate old derived history. Finish their bounded strict
# revalidation before exposing the candidate to a 15-second game request.
if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    trial82_guard.record['state'] = 'prewarming-rush-history'
    trial82_guard.checkpoint()
compose('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--tables', '/app/tables',
        '--warm-rush-history-all', '--log-file', '/app/logs/rush-history-warm.log')
manifest['staleRushHistoriesVerified'] = True
if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    trial82_guard.record['state'] = 'rush-history-verified'
    trial82_guard.checkpoint()
if args.maintenance_redeploy:
    trial82_guard.record['state'] = 'checking-maintenance-after-warm'
    trial82_guard.checkpoint()
    assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Maintenance changed after history verification'


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

if args.apply_activity77:
    import activity77_activation
    try:
        manifest['activity77Applied'] = activity77_activation.activate_activity77(
            mysql, compose, ROOT / 'backups', manifest['revision'], verified_backup,
            manifest['files']['app/hyjs-server'])
    except Exception:
        compose('stop', '-t', '30', 'app')
        raise RuntimeError('Activity77 failed under maintenance. App remains stopped; preserve the verified backup and private activity77 manifest for inspection; do not automatically restore the database.') from None

if args.apply_feedback78:
    try:
        manifest['feedback78Applied'] = feedback78_activation.activate_feedback78(
            mysql, compose, ROOT / 'backups', manifest['revision'], verified_backup,
            manifest['files']['app/hyjs-server'])
    except Exception:
        raise RuntimeError('Feedback78 failed under maintenance. App remains stopped; preserve the authenticated backup and private feedback78 manifest for inspection; do not automatically restore the database.') from None

if args.apply_feedback79:
    try:
        manifest['feedback79Applied'] = feedback79_activation.activate_feedback79(
            mysql, compose, ROOT / 'backups', manifest['revision'], verified_backup,
            manifest['files']['app/hyjs-server'])
    except Exception:
        raise RuntimeError('Feedback79 failed under maintenance. App remains stopped; preserve the authenticated backup and private feedback79 manifest for inspection; do not automatically restore the database.') from None

if args.apply_feedback80:
    try:
        manifest['feedback80Applied'] = feedback80_activation.activate_feedback80(
            mysql, compose, ROOT / 'backups', manifest['revision'], verified_backup,
            manifest['files']['app/hyjs-server'])
    except Exception:
        raise RuntimeError('Feedback80 failed under maintenance. App remains stopped; preserve the authenticated backup and private feedback80 manifest for inspection; do not automatically restore the database.') from None

if args.apply_feedback81:
    try:
        manifest['feedback81Applied'] = feedback81_activation.activate_feedback81(
            mysql, compose, ROOT / 'backups', manifest['revision'], verified_backup,
            manifest['files']['app/hyjs-server'])
    except Exception:
        raise RuntimeError('Feedback81 failed under maintenance. App remains stopped; preserve the authenticated backup and private feedback81 manifest for inspection; do not automatically restore the database.') from None

if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85:
    try:
        applied = trial82_activation.activate_trial82(mysql, compose, trial82_guard, verified_backup,
                                                      apply_trial82=args.apply_trial82, apply_feedback84=args.apply_feedback84, apply_feedback85=args.apply_feedback85)
        if args.apply_trial82:
            manifest['trial82Applied'] = applied
        if args.apply_feedback84:
            manifest['feedback84Applied'] = applied
        if args.apply_feedback85:
            manifest['feedback85Applied'] = applied
    except Exception:
        raise RuntimeError('Trial82/feedback84 failed under maintenance. The stop guard will attempt to keep the app stopped; preserve the authenticated backup and private trial82 manifest for inspection; do not automatically restore the database.') from None

if args.restore_activity_calendar:
    import calendar_restore_activation
    try:
        if getattr(args, 'calendar_formal_110', False):
            manifest['calendarRestore'] = calendar_restore_activation.activate_calendar_restore(
                mysql, compose, ROOT / 'backups', manifest['revision'], verified_backup, args.restore_activity_calendar,
                formal110=True, expected_live_revision=args.expected_live_revision, guard=formal110_guard)
        else:
            manifest['calendarRestore'] = calendar_restore_activation.activate_calendar_restore(mysql, compose, ROOT / 'backups', manifest['revision'], verified_backup, args.restore_activity_calendar)
    except Exception as error:
        if getattr(args, 'calendar_formal_110', False):
            raise RuntimeError('Formal 110 cutover did not finish. The stop guard will attempt to stop the app and record the observed maintenance state; the calendar transaction may already have committed. Preserve the backup and evidence; do not auto-restore.') from None
        compose('stop', '-t', '30', 'app')
        raise RuntimeError('Calendar restore did not finish: ' + str(error) + '. Application remains stopped under maintenance; preserve the authenticated backup and calendar manifest, inspect the failure and retry the same request. Never auto-restore the database.') from None

if getattr(args, 'calendar_formal_110', False):
    formal110_guard.checkpoint('starting-application')
if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    trial82_guard.record['state'] = 'starting-application'
    trial82_guard.checkpoint()
compose('up', '-d', '--no-deps', '--force-recreate', 'app')
if getattr(args, 'calendar_formal_110', False):
    formal110_guard.checkpoint('checking-health')
if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    trial82_guard.record['state'] = 'checking-health'
    trial82_guard.checkpoint()
for _ in range(90):
    try:
        with urllib.request.urlopen('http://127.0.0.1:' + runtime['HYJS_UPSTREAM_PORT'] + '/health', timeout=2) as response:
            assert json.load(response)['status'] == 'ok'
        break
    except Exception:
        time.sleep(2)
else:
    if args.migrate_role_wallets or args.apply_activity77 or args.apply_feedback78 or args.apply_feedback79 or args.apply_feedback80 or args.apply_feedback81:
        compose('stop', '-t', '30', 'app')
    elif args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
        compose('stop', '-t', '30', 'app')
    elif getattr(args, 'calendar_formal_110', False):
        compose('stop', '-t', '30', 'app')
        raise SystemExit('Formal 110 health check failed after a possible calendar commit; the stop guard will confirm the app state. Preserve the backup and evidence; do not auto-restore.')
    raise SystemExit('Application health failed; data and prior deployed.json preserved. Fix/retry this script, or deploy its recorded Git commit from a clean checkout; do not restore the database automatically.')

if args.apply_activity77 or args.apply_feedback78 or args.apply_feedback79 or args.apply_feedback80 or args.apply_feedback81 or args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    try:
        if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
            trial82_guard.record['state'] = 'checking-maintenance-after-health'
            trial82_guard.checkpoint()
        assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Maintenance changed after health check'
    except Exception:
        compose('stop', '-t', '30', 'app')
        raise

if getattr(args, 'calendar_formal_110', False):
    formal110_guard.checkpoint('checking-maintenance-and-vip10-after-health')
    calendar_restore_activation.require_maintenance(mysql)
    calendar_restore_activation.require_vip10_approved(mysql)

if wallet_baseline is not None:
    try:
        reconciled = role_wallet_migration.after(mysql, wallet_baseline)
        (ROOT / 'role-wallet-cutover-after.json').write_text(json.dumps(reconciled, indent=2))
        manifest['roleWalletCutover'] = reconciled
    except Exception:
        compose('stop', '-t', '30', 'app')
        raise SystemExit('Role wallet reconciliation failed. App stopped; preserve database, backup and evidence. Never start a legacy wallet binary against new role transactions.')

if getattr(args, 'calendar_formal_110', False):
    formal110_guard.checkpoint('validating-ingress')
if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    trial82_guard.record['state'] = 'validating-ingress'
    trial82_guard.checkpoint()
if args.provision_ingress:
    destination = Path('/etc/nginx/sites-available/hyjs-test.conf')
    link = Path('/etc/nginx/sites-enabled/hyjs-test.conf')
    if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85:
        trial82_activation.require_existing_ingress(destination, Path('/etc/nginx/sites-enabled/hyjs-test.conf'))
        ingress_activation.update_existing(destination, link, ingress_desired, stage, run)
    else:
        # Preserve the existing first-provision and ordinary ingress behavior.
        desired = (stage / 'nginx.conf').read_bytes()
        changed = not destination.exists() or destination.read_bytes() != desired
        destination.write_bytes(desired)
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
if getattr(args, 'calendar_formal_110', False):
    formal110_guard.checkpoint('installing-artifacts')
if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    trial82_guard.record['state'] = 'installing-artifacts'
    trial82_guard.checkpoint()
target = ROOT / 'app'
if target.exists():
    assert target.is_dir() and not target.is_symlink()
    shutil.rmtree(target)
os.replace(stage / 'app', target)
installed = run('docker', 'inspect', '--format={{index .Config.Labels "org.opencontainers.image.revision"}}', 'hyjs-game').stdout.decode().strip()
assert installed == manifest['revision']
manifest['deployedAt'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
if getattr(args, 'calendar_formal_110', False):
    formal110_guard.checkpoint('recording-deployment')
    calendar_restore_activation.require_maintenance(mysql)
    manifest['formal110Guard'] = {'privateManifest': str(formal110_guard.manifest)}
if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    trial82_guard.record['state'] = 'recording-deployment'
    trial82_guard.checkpoint()
    if args.maintenance_redeploy:
        assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1', 'Maintenance changed before deployment record'
    applied['manifestSHA256'] = trial82_activation.digest(trial82_guard.manifest)
(ROOT / 'deployed.json').write_text(json.dumps(manifest, indent=2))
shutil.rmtree(stage)
if old_image:
    references = run('docker', 'ps', '-a', '--filter', 'ancestor=' + old_image, '--format={{.ID}}').stdout.strip()
    if not references:
        subprocess.run(['docker', 'image', 'rm', old_image], capture_output=True)
if args.apply_feedback78:
    feedback78_guard.complete = True
if args.apply_feedback79:
    feedback79_guard.complete = True
if args.apply_feedback80:
    feedback80_guard.complete = True
if args.apply_feedback81:
    feedback81_guard.complete = True
if args.apply_trial82 or args.apply_feedback84 or args.apply_feedback85 or args.maintenance_redeploy:
    trial82_guard.complete = True
if getattr(args, 'calendar_formal_110', False):
    formal110_guard.checkpoint('deployed-maintenance')
    formal110_guard.complete = True
deployed_status = {'status': 'DEPLOYED', 'revision': installed, 'binarySHA256': manifest['files']['app/hyjs-server']}
if getattr(args, 'calendar_formal_110', False):
    deployed_status.update(formal110=True, calendarDigest=manifest['calendarRestore']['digest'])
print(json.dumps(deployed_status))
