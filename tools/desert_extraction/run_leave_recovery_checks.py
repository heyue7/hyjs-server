"""Run the leave recovery model against LitJson from the fixed 1.1.2 APK."""
import argparse
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
APK_SHA = '408e9d03291d6f3505fb91590134b25a8b6aa68f3ebde9c2f4ff985299baccb3'
LIT_SHA = '151e0a2a6d174558ffa9cb681746d092fde6ba36e937b5825c2f06bb6a8d3230'


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def run(args, env):
    result = subprocess.run(['rtk', 'proxy', *map(str, args)], env=env,
                            text=True, capture_output=True, timeout=90)
    if result.stdout:
        print(result.stdout, end='')
    if result.stderr:
        print(result.stderr, end='')
    if result.returncode:
        raise RuntimeError('Leave recovery check failed: %s' % result.returncode)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apk', type=Path, required=True)
    parser.add_argument('--runtime', type=Path, required=True)
    args = parser.parse_args()
    if digest(args.apk) != APK_SHA:
        raise ValueError('Expected fixed APK 1.1.2')
    runtime = args.runtime / 'dotnet'
    dotnet = runtime / 'dotnet'
    compiler = next((runtime / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = next((runtime / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    env = dict(os.environ, DOTNET_PROCESSOR_COUNT='2', DOTNET_CLI_TELEMETRY_OPTOUT='1')
    with tempfile.TemporaryDirectory(prefix='hyjs-leave-recovery-check-') as directory:
        temp = Path(directory)
        with zipfile.ZipFile(args.apk) as apk:
            (temp / 'LitJson.dll').write_bytes(apk.read('assets/bin/Data/Managed/LitJson.dll'))
        if digest(temp / 'LitJson.dll') != LIT_SHA:
            raise ValueError('Fixed LitJson differs')
        stub = (HERE / 'PositionSyncChecks.cs').read_text().split('public static class PositionSyncChecks')[0]
        (temp / 'PositionStubs.cs').write_text(stub)
        output = temp / 'LeaveRecoveryChecks.dll'
        modern = ['/reference:' + str(path) for path in sorted(refs.glob('*.dll'))]
        run([dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
             '/out:' + str(output), *modern, '/reference:' + str(temp / 'LitJson.dll'),
             temp / 'PositionStubs.cs', HERE / 'PositionSync.cs', HERE / 'TravelController.cs', HERE / 'LeaveController.cs', HERE / 'LeaveRecoveryChecks.cs'], env)
        config = '{"runtimeOptions":{"tfm":"net8.0","framework":{"name":"Microsoft.NETCore.App","version":"%s"}}}' % refs.parents[1].name
        (temp / 'LeaveRecoveryChecks.runtimeconfig.json').write_text(config)
        run([dotnet, output], env)
    print('Temporary leave model outputs cleaned')


if __name__ == '__main__':
    main()
