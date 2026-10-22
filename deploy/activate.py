"""Install one verified artifact set in place, preserving all persistent data."""
from pathlib import Path
import argparse
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
args = parser.parse_args()
os.umask(0o077)
stage = Path(args.stage).resolve()
ROOT = stage.parent.parent
assert stage.parent == ROOT / '.incoming' and (ROOT / '.hyjs-owned').is_file()
manifest = json.loads((stage / 'artifact.json').read_text())

def sha(path):
    digest = hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            digest.update(block)
    return digest.hexdigest()

def run(*cmd, **kwargs):
    return subprocess.run(cmd, check=True, capture_output=True, **kwargs)

def compose(*cmd):
    return run('docker', 'compose', '--project-directory', str(ROOT), '--env-file', str(ROOT / '.env'), '-f', str(ROOT / 'compose.yaml'), *cmd)

def mysql(query):
    return run('docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
               'MYSQL_PWD="$MYSQL_PASSWORD" exec mysql -uhyjs --batch --raw --skip-column-names hyjs',
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
    # Authenticate the encrypted snapshot before deleting this task's temporary plaintext.
    verified = backup_dir / (name + '.verified')
    run(str(stage / 'app/hyjs-backupseal'), '--decrypt', '--input', str(encrypted), '--output', str(verified), '--key-file', str(key_file))
    assert sha(verified) == sha(plain), 'Encrypted backup verification failed'
    plain.unlink()
    verified.unlink()
    (ROOT / 'backup-latest.json').write_text(json.dumps({'file': encrypted.name, 'sha256': sha(encrypted), 'format': 'HYJSBK01-AES256GCM', 'revisionBefore': 'preserved running image'}, indent=2))


for name, digest in manifest['files'].items():
    path = stage / name
    assert path.resolve().is_relative_to(stage) and sha(path) == digest, name
# Deployment topology may change; preserve all existing database credentials.
runtime = {}
for line in (stage / 'deployment.env').read_text().splitlines():
    key, separator, value = line.partition('=')
    assert separator and key in ('HYJS_PUBLIC_HOST', 'HYJS_HTTP_PORT', 'HYJS_ADMIN_PORT', 'HYJS_TCP_PORT', 'HYJS_UPSTREAM_PORT', 'HYJS_API_ORIGIN', 'HYJS_ADMIN_ORIGIN') and value
    runtime[key] = value
private_env = ROOT / '.env'
assert private_env.is_file() and not private_env.stat().st_mode & 0o077
kept = [line for line in private_env.read_text().splitlines() if line.partition('=')[0] not in runtime]
temporary_env = ROOT / '.env.next'
fd = os.open(temporary_env, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
with os.fdopen(fd, 'w') as output:
    output.write('\n'.join(kept) + '\n' + ''.join(key + '=' + value + '\n' for key, value in runtime.items()))
os.chmod(temporary_env, 0o600)
os.replace(temporary_env, private_env)
# Verify the encrypted snapshot before Compose can restart MySQL for new limits.
if (ROOT / 'migration.json').exists():
    backup_existing_database()
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

shutil.copy2('/etc/ssl/certs/ca-certificates.crt', stage / 'ca-certificates.crt')
old_image = subprocess.run(['docker', 'inspect', '--format={{.Image}}', 'hyjs-game'], capture_output=True, text=True).stdout.strip()
run('docker', 'build', '--build-arg', 'REVISION=' + manifest['revision'], '-t', 'hyjs-game:current', str(stage))
compose('up', '-d', '--no-deps', '--force-recreate', 'app')
for _ in range(90):
    try:
        with urllib.request.urlopen('http://127.0.0.1:' + runtime['HYJS_UPSTREAM_PORT'] + '/health', timeout=2) as response:
            assert json.load(response)['status'] == 'ok'
        break
    except Exception:
        time.sleep(2)
else:
    raise SystemExit('Application health failed; data and prior deployed.json preserved. Fix/retry this script, or deploy its recorded Git commit from a clean checkout; do not restore the database automatically.')

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
