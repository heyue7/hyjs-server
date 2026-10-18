"""One-time, scoped host provisioning. Run through deploy.py --provision only."""
from pathlib import Path
import argparse
import os
import pwd
import secrets
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument('--root', required=True, type=Path)
parser.add_argument('--uploader', required=True)
args = parser.parse_args()
ROOT = args.root
assert ROOT.is_absolute() and len(ROOT.parts) >= 3 and '..' not in ROOT.parts and not ROOT.is_symlink()

def run(*args):
    return subprocess.run(args, check=True, text=True, capture_output=True).stdout

if ROOT.exists() and not (ROOT / '.hyjs-owned').is_file():
    raise SystemExit('Existing project directory has no ownership marker; inspect before provisioning')
ROOT.mkdir(mode=0o755, exist_ok=True)
(ROOT / '.hyjs-owned').touch()
for name in ('data', 'logs', 'backups', '.incoming'):
    (ROOT / name).mkdir(mode=0o700 if name in ('data', 'backups', '.incoming') else 0o755, exist_ok=True)
os.chown(ROOT / 'logs', 65532, 65532)
uploader = pwd.getpwnam(args.uploader)
os.chown(ROOT / '.incoming', uploader.pw_uid, uploader.pw_gid)
env = ROOT / '.env'
if not env.exists():
    fd = os.open(env, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as out:
        out.write('HYJS_MYSQL_PASSWORD=' + secrets.token_hex(32) + '\n')
        out.write('HYJS_MYSQL_ROOT_PASSWORD=' + secrets.token_hex(32) + '\n')
if env.stat().st_mode & 0o077:
    raise SystemExit('Private runtime environment must be 0600')
try:
    run('docker', 'compose', 'version')
except subprocess.CalledProcessError:
    run('apt-get', 'install', '-y', '--no-install-recommends', 'docker-compose-v2')
print('Scoped directory, private credentials and Compose ready')
