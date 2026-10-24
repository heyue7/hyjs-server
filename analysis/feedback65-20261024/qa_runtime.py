"""Task-owned isolated shared-MySQL runtime; private finish marker releases every allocation."""
import fcntl, hashlib, http.cookiejar, json, os, secrets, signal, socket, subprocess, sys, time, urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = Path(__file__).parent
PRIVATE = ROOT / '.local-env/feedback65'
PRIVATE.mkdir(parents=True, exist_ok=True, mode=0o700)
PRIVATE.chmod(0o700)
sys.path.insert(0, str(ROOT / 'tests'))
from hyjs_managed_mysql import ManagedMySQL

def private(name, value):
    p = PRIVATE / name
    fd = os.open(p, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w') as f: json.dump(value, f, indent=2)
    return p

pool = ManagedMySQL()
app = None
stopping = False
def stop(*_):
    global stopping
    stopping = True
signal.signal(signal.SIGTERM, stop)
signal.signal(signal.SIGINT, stop)
lock = open('/tmp/hyjs-isolated-mysql.lock', 'a+')
fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
try:
    assert not (PRIVATE / 'finish').exists(), 'Do not revive a released runtime'
    for port in [28979,28980]:
        with socket.socket() as sock:
            sock.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
            sock.bind(('127.0.0.1',port))
    conn = pool.allocate('feedback65')
    bootstrap = {'username':'qa63','password':secrets.token_hex(20)}
    bp = private('bootstrap.json',bootstrap)
    config = PRIVATE / 'config.yaml'
    config.write_text('port: 28979\nlogConfig:\n  logLevel: warn\n  maxSize: 4\n  maxBackups: 1\n  maxAge: 1\ngame:\n  serverId: 1\n  driver: mysql\n  bind: 127.0.0.1\n  advertiseHost: 127.0.0.1\n  httpPort: 28979\n  tcpPort: 28980\n')
    config.chmod(0o600)
    env = dict(os.environ, **conn.env(), HYJS_CONFIG_FILE=str(config), HYJS_ADMIN_BOOTSTRAP_FILE=str(bp),
               HYJS_ADMIN_ASSETS=str(ROOT/'admin-web/dist'), HYJS_GAME_ACCOUNTS_DATABASE='', GOMAXPROCS='2', GOMEMLIMIT='768MiB')
    binary = ROOT/'build/hyjs-feedback65-current'
    def launch():
        log = (PRIVATE/'app.log').open('ab')
        (PRIVATE/'app.log').chmod(0o600)
        p = subprocess.Popen([str(binary),'--env','test','--bind','127.0.0.1','--advertise-host','127.0.0.1','--http-port','28979','--tcp-port','28980','--tables',str(ROOT/'analysis/tables'),'--log-file',str(PRIVATE/'game.log'),'--enable-test-payments','--enable-local-wallet'],cwd=ROOT/'hyjs-server',env=env,stdout=log,stderr=log)
        log.close()
        return p
    app=launch()
    private('processes.json',{'wrapperPid':os.getpid(),'appPid':app.pid,'database':conn.database})
    origin='http://127.0.0.1:28979'
    for _ in range(240):
        assert app.poll() is None,'Owned QA app exited; inspect private log'
        try:
            with urllib.request.urlopen(origin+'/health',timeout=1) as response:
                if json.load(response)['status']=='ok':break
        except Exception:pass
        time.sleep(.25)
    else:raise RuntimeError('Owned QA startup timeout')
    jar=http.cookiejar.MozillaCookieJar(str(PRIVATE/'admin-cookies.netscape'))
    opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    req=urllib.request.Request(origin+'/admin/api/auth/login',data=json.dumps(bootstrap).encode(),headers={'Content-Type':'application/json','Origin':origin})
    with opener.open(req,timeout=30) as response:auth=json.load(response)
    assert auth['code']==0,'QA admin bootstrap refused'
    jar.save(ignore_discard=True,ignore_expires=True)
    (PRIVATE/'admin-cookies.netscape').chmod(0o600)
    private('admin-auth.json',{'origin':origin,'csrfToken':auth['data']['csrfToken']})
    state={'origin':origin,'database':conn.database,'pid':app.pid,'wrapperPid':os.getpid(),'binarySHA256':hashlib.file_digest(binary.open('rb'),'sha256').hexdigest()}
    private('runtime.json',state)
    (OUT/'qa-ready.json').write_text(json.dumps(state,indent=2)+'\n')
    print('QA64 ready: isolated MySQL and app on28979/28980; credentials private',flush=True)
    while not stopping and not (PRIVATE/'finish').exists():
        assert app.poll() is None,'Owned QA app stopped'
        if (PRIVATE/'restart').exists():
            (PRIVATE/'restart').unlink();app.terminate();app.wait(timeout=20);app=launch()
            private('processes.json',{'wrapperPid':os.getpid(),'appPid':app.pid,'database':conn.database})
            print('QA64 restarted against same owned schema',flush=True)
        time.sleep(.3)
finally:
    if app is not None and app.poll() is None:
        app.terminate()
        try:app.wait(timeout=20)
        except subprocess.TimeoutExpired:app.kill();app.wait(timeout=5)
    pool.close()
    fcntl.flock(lock,fcntl.LOCK_UN);lock.close()
    (OUT/'qa-cleanup.json').write_text(json.dumps({'appStopped':True,'ownedMySQLReleased':not pool.connections})+'\n')
    print('QA64 app and owned schema/account released',flush=True)
