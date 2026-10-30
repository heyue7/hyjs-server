"""Test the real reward projection using the game's LitJson and minimal protocol objects."""
import argparse
import hashlib
import json
import os
import subprocess
import tempfile
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = Path('/Users/lirui/LR/hyjs')
LIT_SHA = '151e0a2a6d174558ffa9cb681746d092fde6ba36e937b5825c2f06bb6a8d3230'


def run(args):
    env = dict(os.environ, DOTNET_PROCESSOR_COUNT='2', DOTNET_CLI_TELEMETRY_OPTOUT='1')
    result = subprocess.run(['rtk', 'proxy', *map(str, args)], env=env, text=True, capture_output=True, timeout=30)
    if result.returncode:
        raise RuntimeError(result.stderr[-2000:] + result.stdout[-2000:])
    return result.stdout.strip()


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--base-apk', type=Path, default=ROOT / 'build/desert-final86/build/desert-extraction-v4/base-v80.apk')
parser.add_argument('--runtime', type=Path, default=ROOT / '.tools/native-batch')
args = parser.parse_args()
runtime = args.runtime / 'dotnet'
with tempfile.TemporaryDirectory(prefix='hyjs-monster-reward-check-') as path:
    work = Path(path)
    with zipfile.ZipFile(args.base_apk) as archive:
        (work / 'LitJson.dll').write_bytes(archive.read('assets/bin/Data/Managed/LitJson.dll'))
    if hashlib.sha256((work / 'LitJson.dll').read_bytes()).hexdigest() != LIT_SHA:
        raise ValueError('Original game LitJson differs')
    compiler = next((runtime / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    ref = next((runtime / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    refs = ['/reference:' + str(p) for p in sorted(ref.glob('*.dll'))] + ['/reference:' + str(work / 'LitJson.dll')]
    (work / 'Checks.runtimeconfig.json').write_text(json.dumps({'runtimeOptions': {'tfm': 'net8.0', 'framework': {'name': 'Microsoft.NETCore.App', 'version': ref.parents[1].name}}}))
    run([runtime / 'dotnet', compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3', '/out:' + str(work / 'Checks.dll'), *refs, HERE / 'MonsterRewardView.cs', HERE / 'MonsterRewardViewChecks.cs'])
    print(run([runtime / 'dotnet', work / 'Checks.dll']))
