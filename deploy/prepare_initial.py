"""Stop the verified local writer and take a full project-only migration snapshot."""
from pathlib import Path
import argparse
import gzip
import hashlib
import json
import os
import shutil
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.shared_mysql import settings, sql
from tools.release_go import stop_owned_process, current_advertise_host, current_wallet_enabled, operations

parser = argparse.ArgumentParser()
parser.add_argument('--stop-local', action='store_true', required=True)
args = parser.parse_args()
target = ROOT / '.local-env/deploy'
target.mkdir(mode=0o700, exist_ok=True)
backup = target / 'initial.sql.gz'
assert not backup.exists(), 'Reuse the original frozen dump for a retry; never replace it silently'
state = {'previousHost': current_advertise_host(), 'walletEnabled': current_wallet_enabled(),
         'previousBinarySHA256': operations()['binarySha256']}
stop_owned_process()
state['localWriterStoppedAt'] = time.strftime('%Y-%m-%dT%H:%M:%S%z')
(target / 'local-cutover.json').write_text(json.dumps(state, indent=2))
os.chmod(target / 'local-cutover.json', 0o600)
tables = sql("SELECT table_name FROM information_schema.tables WHERE table_schema='hyjs' ORDER BY table_name;").strip().splitlines()
assert tables and all(name.replace('_', '').isalnum() for name in tables)
count_query = ' UNION ALL '.join("SELECT '" + name + "',COUNT(*) FROM `" + name + '`' for name in tables) + ';'
counts = dict((line.split('\t')[0], int(line.split('\t')[1])) for line in sql(count_query).strip().splitlines())
profile_query = "SELECT id,HEX(uid),HEX(session),COALESCE(SHA2(player,256),'NULL'),COALESCE(SHA2(game,256),'NULL'),created_at,HEX(password_salt),HEX(password_hash) FROM profiles ORDER BY id;"
profile_hash = hashlib.sha256(sql(profile_query).encode()).hexdigest()
credentials = settings()
raw = target / 'initial.sql'
fd = os.open(raw, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'wb') as output:
    result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', 'codex-local-mysql', 'sh', '-c',
        'read -r MYSQL_PWD; export MYSQL_PWD; exec mysqldump -h127.0.0.1 -u hyjs --single-transaction --hex-blob --no-tablespaces --set-gtid-purged=OFF --routines --triggers hyjs'],
        input=(credentials['MYSQL_PASSWORD'] + '\n').encode(), stdout=output, stderr=subprocess.PIPE)
assert result.returncode == 0 and raw.stat().st_size > 0, 'Snapshot failed; local writer remains stopped and data is preserved'
fd = os.open(backup, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'wb') as target_file, gzip.GzipFile(fileobj=target_file, mode='wb') as compressed, raw.open('rb') as source:
    shutil.copyfileobj(source, compressed)
with backup.open('rb') as source:
    digest = hashlib.file_digest(source, 'sha256').hexdigest()
raw.unlink()
expected = {'dumpSHA256': digest, 'counts': counts, 'profileQuery': profile_query, 'profileSHA256': profile_hash}
verification = target / 'initial-verification.json'
verification.write_text(json.dumps(expected, indent=2))
os.chmod(verification, 0o600)
print(json.dumps({'status': 'SNAPSHOT_COMPLETE_LOCAL_WRITER_STOPPED', 'tables': len(tables), 'profiles': counts.get('profiles'), 'gzipBytes': backup.stat().st_size, 'path': str(backup)}))
