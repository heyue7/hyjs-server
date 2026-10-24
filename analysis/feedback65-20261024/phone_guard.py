from pathlib import Path
import datetime, fcntl, hashlib, json, os, signal, sys, time

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.device_demo import Device

OUT = Path(__file__).parent
PRIVATE = ROOT / '.local-env/feedback65/phone'
PRIVATE.mkdir(parents=True, exist_ok=True, mode=0o700)
PRIVATE.chmod(0o700)
lock = open('/tmp/hyjs-physical-phone.lock', 'a+')
fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
stopping = False
def stop(*_):
    global stopping
    stopping = True
signal.signal(signal.SIGTERM, stop)
signal.signal(signal.SIGINT, stop)
try:
    d = Device('10AF7Q0KSJ003GK')
    status = d.status()
    assert 'com.bbk.launcher2' in str(status['foreground']) or 'com.yunchang.djsy.ewan.local/com.hyjs.wallet.ClientUpdateActivity' in str(status['foreground']), 'Phone is in use outside the owned launcher; do not take over another active app'
    d.run('shell', 'am', 'force-stop', d.package)
    installed = d.text('shell', 'pm', 'path', d.package).removeprefix('package:')
    apk_sha = d.text('shell', 'sha256sum', installed, timeout=60).split()[0]
    assert apk_sha == 'f3ce73cc1f2e41bb45873c64f01e9a6ec81d84d1e41f18482c27d62c42ace3f9'
    names = d.text('shell', 'run-as', d.package, 'ls', 'shared_prefs').splitlines()
    hashes = {n: d.text('shell', 'run-as', d.package, 'sha256sum', 'shared_prefs/' + n).split()[0] for n in names}
    backup = PRIVATE / 'shared-prefs.tar'
    raw = d.run('exec-out', 'run-as', d.package, 'tar', '-cf', '-', '-C', 'shared_prefs', '.')
    fd = os.open(backup, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'wb') as file: file.write(raw)
    caller = d.run('exec-out', 'run-as', d.package, 'cat', 'files/native-runner-current.dll')
    fd = os.open(PRIVATE / 'original-caller.dll', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'wb') as file: file.write(caller)
    lease = dict(status='held', serial=d.serial, package=d.package, owner='feedback65', pid=os.getpid(), originalAPKSHA256=apk_sha,
                 preferenceHashes=hashes, privateBackupSHA256=hashlib.sha256(raw).hexdigest(), originalPreferenceFiles=names,
                 originalCallerSHA256=hashlib.sha256(caller).hexdigest(), originalReverse=status['reverse'],
                 display={n: d.text('shell', 'settings', 'get', domain, n) for domain, n in [('global','stay_on_while_plugged_in'),('system','screen_off_timeout')]},
                 acquiredAt=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat())
    (OUT / 'phone-lease.json').write_text(json.dumps(lease, indent=2)+'\n')
    d.prepare()
    print('Phone65 lease held; formal64 and preferences/caller backed up privately; no game login', flush=True)
    while not stopping and not (PRIVATE / 'finish').exists(): time.sleep(.3)
finally:
    fcntl.flock(lock, fcntl.LOCK_UN)
    lock.close()
