"""Update only the developer diagnostic caller while the original APK stays open."""
import argparse
import hashlib
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
sys.path.insert(0, str(ROOT))
from device_demo import Device, PACKAGE
from tools.native_batch.build import compile_implementation


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--serial')
    args = parser.parse_args()
    device = Device(args.serial)
    device.require_game_surface_accessible()
    for selector in ('golden', 'highest', 'pve-auto', 'pve-compat'):
        try:
            status = device.text('shell', 'run-as', PACKAGE, 'cat',
                                 'files/native-batch/' + selector + '/status.txt')
        except RuntimeError:
            continue
        if status.startswith('running'):
            raise RuntimeError('Original engine batch is running; retain its caller until completion')
    artifact = compile_implementation()
    digest = hashlib.sha256(artifact.read_bytes()).hexdigest()
    staged = '/data/local/tmp/hyjs-native-caller-' + digest[:16] + '.dll'
    device.run('push', artifact, staged, timeout=30)
    device.run('shell', 'run-as', PACKAGE, 'cp', staged, 'files/native-runner-pending.dll')
    actual = device.text('shell', 'run-as', PACKAGE, 'sha256sum', 'files/native-runner-pending.dll').split()[0]
    if actual != digest:
        raise RuntimeError('Private diagnostic caller differs from the compiled artifact')
    device.run('shell', 'run-as', PACKAGE, 'mv', 'files/native-runner-pending.dll', 'files/native-runner-current.dll')
    device.run('shell', 'rm', '-f', staged)
    print('Private diagnostic caller updated and hash verified:', digest)


if __name__ == '__main__':
    main()
