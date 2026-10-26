"""Build a clean Git revision and deploy it through the project's Compose contract."""
from pathlib import Path
import argparse
import hashlib
import json
import os
import shlex
import shutil
import subprocess
import tarfile
import time
from settings import load, render, RUNTIME_KEYS

ROOT = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--revision', default='master', help='Exact clean checked-out revision; defaults to master')
parser.add_argument('--provision', action='store_true', help='Explicit first-time infrastructure and ingress setup')
parser.add_argument('--update-ingress', action='store_true', help='Apply the reviewed project Nginx template without first-time infrastructure setup')
parser.add_argument('--initial-dump', type=Path, help='One-time private SQL gzip; sibling initial-verification.json required')
parser.add_argument('--env-file', type=Path, default=ROOT / 'deploy/.env', help='Ignored private deployment settings')
parser.add_argument('--check-config', action='store_true', help='Validate and render templates without build, SSH or Docker')
parser.add_argument('--migrate-role-wallets', action='store_true', help='Quiesce legacy writers, back up and reconcile the one-time role-wallet cutover')
parser.add_argument('--warm-rush-history-player', action='append', type=int, default=[], help='Prevalidate this existing owner with the exact candidate before application cutover')
parser.add_argument('--apply-growth74', action='store_true', help='Apply reviewed growth configuration while game is in maintenance')
parser.add_argument('--apply-growth75', action='store_true', help='Apply audited 75 publication, calendar, economy and fresh activity CAS while keeping maintenance')
parser.add_argument('--expected-live-revision', help='Refuse activation if a different live release appeared')
args = parser.parse_args()
assert not (args.apply_growth74 and args.apply_growth75), 'Choose one explicit growth deployment version'
if args.apply_growth75:
    assert args.expected_live_revision and not args.provision and not args.initial_dump and not args.migrate_role_wallets, 'Growth75 requires a pinned existing release and its own maintenance window'
settings = load(args.env_file)
HOST = settings['HYJS_DEPLOY_SSH']
REMOTE = settings['HYJS_DEPLOY_ROOT']
if args.check_config:
    for name in ('config.test.yaml', 'nginx.conf'):
        render((ROOT / 'deploy' / name).read_text(), settings)
    print('Deployment settings and templates valid; no external action performed')
    raise SystemExit(0)
record_dir = ROOT / 'analysis/deploy-20261018'
record_dir.mkdir(parents=True, exist_ok=True)
log = record_dir / 'deployment.log'

def run(*cmd, cwd=ROOT, env=None):
    result = subprocess.run(['rtk', 'proxy', *cmd], cwd=cwd, env=env, capture_output=True, text=True)
    with log.open('a') as output:
        output.write(result.stdout + result.stderr)
    if result.returncode:
        raise RuntimeError('Command failed: ' + cmd[0] + '; inspect ' + str(log))
    return result.stdout.strip()

def remote(command):
    return run('ssh', '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=15', HOST, command)

def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()

revision = run('git', 'rev-parse', args.revision + '^{commit}')
assert run('git', 'rev-parse', 'HEAD') == revision, 'Checkout the requested revision first'
assert not run('git', 'status', '--porcelain'), 'Refuse a dirty/untracked source mixture'
assert run('git', 'branch', '--show-current') in ('master', 'test', 'dev', ''), 'Unexpected deployment branch'
remote('test "$(uname -m)" = x86_64 && sudo -n nginx -t')
if args.provision:
    setup = '/tmp/hyjs-setup-' + revision[:12] + '.py'
    run('scp', '-q', str(ROOT / 'deploy/setup_remote.py'), HOST + ':' + setup)
    remote('sudo -n python3 ' + shlex.quote(setup) + ' --root ' + shlex.quote(REMOTE) + ' --uploader ' + shlex.quote(settings['HYJS_DEPLOY_UPLOADER']) + ' && rm ' + shlex.quote(setup))
else:
    remote('test -f ' + shlex.quote(REMOTE + '/.hyjs-owned') + ' && sudo -n docker compose version && test -f /etc/nginx/sites-enabled/hyjs-test.conf')

stage = ROOT / 'build/deploy-stage'
if stage.exists():
    assert (stage / '.hyjs-stage').is_file()
    shutil.rmtree(stage)
stage.mkdir(parents=True)
(stage / '.hyjs-stage').touch()
app = stage / 'app'
app.mkdir()
environment = dict(os.environ, GOOS='linux', GOARCH='amd64', CGO_ENABLED='0', GOTOOLCHAIN='auto')
print('Building clean Linux revision ' + revision, flush=True)
run('go', 'test', './middleware', '-run', '^TestCORSExactOriginsAndNativeCompatibility$', '-count=1', cwd=ROOT / 'hyjs-server')
run('go', 'vet', './...', cwd=ROOT / 'hyjs-server')
run('go', 'build', '-trimpath', '-buildvcs=false', '-o', str(app / 'hyjs-server'), '.', cwd=ROOT / 'hyjs-server', env=environment)
run('go', 'build', '-trimpath', '-buildvcs=false', '-o', str(app / 'hyjs-backupseal'), './cmd/backupseal', cwd=ROOT / 'hyjs-server', env=environment)
# 覆盖发布只包含当前业务程序；历史版本需要时从Git重新构建。
if args.migrate_role_wallets:
    assert args.expected_live_revision and not args.provision and not args.initial_dump, 'Wallet cutover requires an existing pinned release'
    assert (ROOT / 'hyjs-server/db/roleWalletMigration.go').is_file()
run('pnpm', 'typecheck', cwd=ROOT / 'admin-web')
run('pnpm', 'build', cwd=ROOT / 'admin-web')
assert not run('git', 'status', '--porcelain'), 'Build unexpectedly modified tracked source'
shutil.copytree(ROOT / 'admin-web/dist', app / 'admin-web')
shutil.copytree(ROOT / 'analysis/tables', app / 'tables', ignore=shutil.ignore_patterns('*.json', '*.md', '*.png'))
(app / 'config.test.yaml').write_text(render((ROOT / 'deploy/config.test.yaml').read_text(), settings))
(stage / 'nginx.conf').write_text(render((ROOT / 'deploy/nginx.conf').read_text(), settings))
(stage / 'deployment.env').write_text(''.join(key + '=' + settings[key] + '\n' for key in RUNTIME_KEYS))
for name in ('Dockerfile', 'compose.yaml', 'activate.py', 'role_wallet_migration.py', 'growth75_activation.py', 'growth75_activity_patch.py'):
    shutil.copy2(ROOT / 'deploy' / name, stage / name)
files = {str(p.relative_to(stage)): sha(p) for p in sorted(stage.rglob('*')) if p.is_file() and p.name != '.hyjs-stage'}
manifest = {'revision': revision, 'architecture': 'linux/amd64', 'database': 'MySQL8.4', 'files': files,
            'frontend': settings['HYJS_ADMIN_ORIGIN'] + '/admin/', 'api': settings['HYJS_API_ORIGIN'], 'tcpPort': int(settings['HYJS_TCP_PORT'])}
(stage / 'artifact.json').write_text(json.dumps(manifest, indent=2))
archive = ROOT / 'build/deploy-current.tar.gz'
with tarfile.open(archive, 'w:gz') as output:
    output.add(stage, arcname='.')
remote_stage = REMOTE + '/.incoming/' + revision[:12]
# This exact incoming directory holds only reproducible application artifacts.
# The initial database and verification file live outside it and are preserved.
prepare = """from pathlib import Path
import json, shutil
p=Path(%r)
root=Path(%r)
assert (root/'.hyjs-owned').is_file()
assert p.parent == root/'.incoming' and not p.is_symlink()
if p.exists():
    manifest=p/'artifact.json'
    if manifest.exists(): assert json.loads(manifest.read_text())['revision'] == %r
    shutil.rmtree(p)
p.mkdir(mode=0o700)
""" % (remote_stage, REMOTE, revision)
remote('python3 -c ' + shlex.quote(prepare))
run('scp', '-q', str(archive), HOST + ':' + remote_stage + '/bundle.tar.gz')
remote('cd ' + shlex.quote(remote_stage) + ' && tar -xzf bundle.tar.gz && rm bundle.tar.gz')
if args.initial_dump:
    backup = args.initial_dump.resolve()
    verification = backup.with_name('initial-verification.json')
    assert backup.is_file() and verification.is_file() and not backup.stat().st_mode & 0o077
    run('scp', '-q', str(backup), HOST + ':' + REMOTE + '/.incoming/initial.sql.gz')
    run('scp', '-q', str(verification), HOST + ':' + REMOTE + '/.incoming/initial-verification.json')
    remote('chmod 600 ' + shlex.quote(REMOTE + '/.incoming/initial.sql.gz') + ' ' + shlex.quote(REMOTE + '/.incoming/initial-verification.json'))
command = 'sudo -n python3 ' + shlex.quote(remote_stage + '/activate.py') + ' ' + shlex.quote(remote_stage)
for player_id in args.warm_rush_history_player:
    assert player_id > 0
    command += ' --warm-rush-history-player ' + str(player_id)
if args.expected_live_revision:
    command += ' --expected-live-revision ' + shlex.quote(args.expected_live_revision)
if args.apply_growth74:
    command += ' --apply-growth74'
if args.apply_growth75:
    command += ' --apply-growth75'
if args.migrate_role_wallets:
    command += ' --migrate-role-wallets'
if args.provision or args.update_ingress:
    command += ' --provision-ingress'
if args.initial_dump:
    command += ' --initial-db'
print('Activating verified artifacts and preserving persistent data', flush=True)
print(remote(command), flush=True)
deployed = json.loads(remote('cat ' + shlex.quote(REMOTE + '/deployed.json')))
assert deployed['revision'] == revision and deployed['files'] == manifest['files']
(record_dir / 'deployed.json').write_text(json.dumps(deployed, indent=2))
shutil.rmtree(stage)
archive.unlink()
print('Deployment recorded in ' + str(record_dir / 'deployed.json'), flush=True)
