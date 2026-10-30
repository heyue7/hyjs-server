"""Run PvpBridge recovery checks against the pinned game's actual LitJson."""
import argparse
import ast
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
APK_SHA = '25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
LIT_SHA = '151e0a2a6d174558ffa9cb681746d092fde6ba36e937b5825c2f06bb6a8d3230'
MAP_SHA = 'ba402ea01a9cd4fb3db03e57f6a9335a738a8a48a212cb83cd7935fbce66668f'
BASELINE_HELPER_SHA = '7ebb24ddf0dcca724ca727f2047c0b205fd4102d100a385247eff1ccbe4b97c1'


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def run(command, env):
    result = subprocess.run(['rtk', 'proxy', *map(str, command)], env=env,
                            text=True, capture_output=True, timeout=60)
    if result.stdout:
        print(result.stdout, end='')
    if result.stderr:
        print(result.stderr, end='')
    if result.returncode:
        raise RuntimeError('PvpBridge recovery check failed (exit %s)' % result.returncode)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--base-apk', type=Path, required=True)
    ap.add_argument('--runtime', type=Path, required=True, help='native-batch root')
    ap.add_argument('--map-managed', type=Path,
                    default=ROOT / 'analysis/desert-final86/map-static/Assembly-CSharp.dll')
    ap.add_argument('--baseline-helper', type=Path,
                    default=ROOT / 'analysis/desert-final86/field-preview-team-r12/SoulFlowerDesertV4.dll')
    args = ap.parse_args()
    if digest(args.base_apk) != APK_SHA:
        raise ValueError('Expected pinned v80 base APK')
    if digest(args.map_managed) != MAP_SHA or digest(args.baseline_helper) != BASELINE_HELPER_SHA:
        raise ValueError('Expected pinned game map and r12 baseline helper')
    rt = args.runtime / 'dotnet'
    dotnet = rt / 'dotnet'
    compiler = next((rt / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = next((rt / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    env = dict(os.environ, DOTNET_PROCESSOR_COUNT='2', DOTNET_CLI_TELEMETRY_OPTOUT='1')
    with tempfile.TemporaryDirectory(prefix='hyjs-pvp-recovery-check-') as directory:
        temp = Path(directory)
        names = ('Assembly-CSharp-firstpass.dll', 'UnityEngine.dll', 'mscorlib.dll',
                 'System.dll', 'System.Core.dll', 'LitJson.dll', 'HYJS.Field63.dll',
                 'LocalFieldSettlement.dll')
        with zipfile.ZipFile(args.base_apk) as apk:
            for name in names:
                (temp / name).write_bytes(apk.read('assets/bin/Data/Managed/' + name))
        if digest(temp / 'LitJson.dll') != LIT_SHA:
            raise ValueError('Original game LitJson differs')
        references = ['/reference:' + str(p) for p in sorted(refs.glob('*.dll'))]
        references.append('/reference:' + str(temp / 'LitJson.dll'))
        output = temp / 'PvpBridgeRecoveryChecks.dll'
        run([dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
             '/out:' + str(output), *references, HERE / 'PvpBridge.cs',
             HERE / 'PvpBridgeRecoveryChecks.cs'], env)
        runtime = {'runtimeOptions': {'tfm': 'net8.0', 'framework': {
            'name': 'Microsoft.NETCore.App', 'version': refs.parents[1].name}}}
        (temp / 'PvpBridgeRecoveryChecks.runtimeconfig.json').write_text(json.dumps(runtime))
        run([dotnet, output], env)
        # Use the exact source list and pinned Unity 5.3/game references from the
        # production helper builder, then compare the relevant exported ABI.
        tree = ast.parse((HERE / 'build_field_patch.py').read_text())
        source_list = next(ast.literal_eval(node.value) for node in tree.body
                           if isinstance(node, ast.Assign) and any(
                               isinstance(target, ast.Name) and target.id == 'SOURCES'
                               for target in node.targets))
        if 'PvpBridge.cs' not in source_list:
            raise ValueError('Production helper source list omits PvpBridge')
        candidate = temp / 'SoulFlowerDesertV4.dll'
        game_refs = [args.map_managed, *[temp / name for name in names]]
        run([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+',
             '/langversion:3', '/deterministic+', '/out:' + str(candidate),
             *['/reference:' + str(p.resolve()) for p in game_refs],
             *[HERE / name for name in source_list]], env)
        print('PASS production helper source compiled with C# 3 and pinned Unity/game assemblies')
        run([dotnet, output, '--abi', args.baseline_helper.resolve(), candidate], env)
        print('PvpBridge source SHA-256:', digest(HERE / 'PvpBridge.cs'))
    print('Temporary PvpBridge test directory cleaned')


if __name__ == '__main__':
    main()
