"""Run the actual copy-exit helper against bounded native transport/UI doubles."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apk', required=True, type=Path)
    parser.add_argument('--runtime', required=True, type=Path)
    args = parser.parse_args()
    with args.apk.open('rb') as source:
        if hashlib.file_digest(source, 'sha256').hexdigest() != 'cac89c00b7ad79e0a098a3108229cee94fd693bbf155fc64076c83ae25c36a47':
            raise ValueError('Expected pinned 1.1.6 APK')
    runtime = args.runtime / 'dotnet'
    dotnet = runtime / 'dotnet'
    csc = next((runtime / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = next((runtime / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    with tempfile.TemporaryDirectory(prefix='hyjs-copy-exit-check-') as directory, zipfile.ZipFile(args.apk) as apk:
        work = Path(directory)
        lit = work / 'LitJson.dll'
        lit.write_bytes(apk.read('assets/bin/Data/Managed/LitJson.dll'))
        if hashlib.sha256(lit.read_bytes()).hexdigest() != '151e0a2a6d174558ffa9cb681746d092fde6ba36e937b5825c2f06bb6a8d3230':
            raise ValueError('Original LitJson differs')
        output = work / 'CopyExitRecoveryChecks.dll'
        cmd = [dotnet, csc, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3', '/out:' + str(output)]
        cmd += ['/reference:' + str(p) for p in sorted(refs.glob('*.dll'))]
        cmd += ['/reference:' + str(lit), HERE / 'CopyExitRecovery.cs', HERE / 'CopyExitRecoveryChecks.cs']
        subprocess.run(['rtk', 'proxy', *map(str, cmd)], check=True, timeout=90)
        output.with_suffix('.runtimeconfig.json').write_text(json.dumps({'runtimeOptions': {'tfm': 'net8.0', 'framework': {'name': 'Microsoft.NETCore.App', 'version': refs.parents[1].name}}}))
        subprocess.run(['rtk', 'proxy', str(dotnet), str(output)], check=True, timeout=30)

if __name__ == '__main__':
    main()
