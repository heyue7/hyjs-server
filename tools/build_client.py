"""Build and verify the parallel-install local APK using the installed Android SDK."""
from pathlib import Path
import hashlib
import fcntl
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
if any(arg in ('-h', '--help') for arg in sys.argv[1:]):
    # Help exits before touching signing keys or any build/rollback artifact.
    raise SystemExit(subprocess.run([sys.executable, str(root/'tools/patch_client.py'), '--help'], cwd=root).returncode)
sdk = Path.home()/'Library/Android/sdk'
build_tools = sdk/'build-tools/33.0.0'
java = Path.home()/'Library/Java/JavaVirtualMachines/openjdk-18.0.1.1/Contents/Home/bin'
keys = root/'.local-signing'
keys.mkdir(exist_ok=True, mode=0o700)
key = keys/'debug.keystore'


def run(*args):
    subprocess.run([str(x) for x in args], cwd=root, check=True)


if not key.exists():
    run(java/'keytool', '-genkeypair', '-keystore', key, '-storepass', 'android',
        '-keypass', 'android', '-alias', 'androiddebugkey', '-dname',
        'CN=HYJS Local Development', '-keyalg', 'RSA', '-keysize', '2048', '-validity', '10000')
    key.chmod(0o600)
build = root/'build'
build.mkdir(exist_ok=True)
current = build/'hyjs-local.apk'
rollback = build/'hyjs-rollback.apk'
candidate = build/'hyjs-candidate.apk'


def digest(path):
    with path.open('rb') as file:
        return hashlib.file_digest(file, 'sha256').digest()


# Serialize builds so cleanup cannot remove another invocation's intermediate APK.
with (build/'.client-build.lock').open('a') as lock:
    fcntl.flock(lock, fcntl.LOCK_EX)
    try:
        run(sys.executable, root/'tools/patch_client.py', *sys.argv[1:])
        run(build_tools/'zipalign', '-f', '-p', '4', build/'hyjs-local-unsigned.apk', build/'hyjs-local-aligned.apk')
        run(build_tools/'apksigner', 'sign', '--ks', key, '--ks-pass', 'pass:android', '--key-pass',
            'pass:android', '--out', candidate, build/'hyjs-local-aligned.apk')
        run(build_tools/'apksigner', 'verify', candidate)
        # Promote only a verified candidate; retain one previous signed APK for rollback.
        if current.exists() and digest(current) != digest(candidate):
            current.replace(rollback)
        candidate.replace(current)
        signature = candidate.with_suffix('.apk.idsig')
        if signature.exists():
            signature.replace(current.with_suffix('.apk.idsig'))
        else:
            current.with_suffix('.apk.idsig').unlink(missing_ok=True)
        print('Verified APK:', current)
    finally:
        # Keep the deliverable and at most one rollback; the original input APK is outside build/.
        keep = {current, rollback}
        for path in build.glob('hyjs-*.apk'):
            if path not in keep:
                path.unlink(missing_ok=True)
        (build/'manifest.apk').unlink(missing_ok=True)
        for path in build.glob('hyjs-*.apk.idsig'):
            if path != current.with_suffix('.apk.idsig'):
                path.unlink(missing_ok=True)
