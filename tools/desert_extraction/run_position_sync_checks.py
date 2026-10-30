"""Replay native position dispatch and fallback using the real PositionSync source."""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile

here = Path(__file__).resolve().parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--runtime', type=Path, required=True, help='native-batch root')
args = parser.parse_args()
runtime = args.runtime.resolve() / 'dotnet'
dotnet = runtime / 'dotnet'
csc = next((runtime / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
refs = next((runtime / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
framework = refs.parents[1].name

with tempfile.TemporaryDirectory(prefix='hyjs-position-sync-check-') as directory:
    work = Path(directory)
    target = work / 'PositionSyncChecks.dll'
    command = ['rtk', 'proxy', str(dotnet), str(csc), '/nologo', '/target:exe', '/nostdlib+',
               '/langversion:3', '/out:' + str(target)]
    command += ['/reference:' + str(path) for path in sorted(refs.glob('*.dll'))]
    command += [str(here / 'PositionSync.cs'), str(here / 'PositionSyncChecks.cs')]
    subprocess.run(command, check=True, timeout=90)
    (work / 'PositionSyncChecks.runtimeconfig.json').write_text(json.dumps({
        'runtimeOptions': {'tfm': 'net8.0', 'framework': {'name': 'Microsoft.NETCore.App', 'version': framework}}
    }))
    subprocess.run(['rtk', 'proxy', str(dotnet), str(target)], check=True, timeout=30)
