"""Build four scoped UI/map hooks against an explicitly pinned shipped APK."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require

ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER = 'assets/bin/Data/Managed/HYJS.Gameplay.dll'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--expected-sha256', required=True)
    parser.add_argument('--input-managed', type=Path)
    parser.add_argument('--tool-root', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    out = args.output_directory.resolve()
    require(not out.exists() and out.is_relative_to(ROOT), 'Fresh task output required')
    require(sha(args.base_apk.read_bytes()) == args.expected_sha256, 'Base APK identity changed')
    out.mkdir(parents=True)
    runtime = args.tool_root / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'
    source = Path(__file__).resolve().parent

    def run(name, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT,
                                capture_output=True, text=True, timeout=180)
        (out / (name + '.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + ' failed; inspect task log')

    with zipfile.ZipFile(args.base_apk) as archive, tempfile.TemporaryDirectory(prefix='hyjs-gameplay45-') as temporary:
        work = Path(temporary)
        snow = archive.read('assets/android/map/map_snowforest_001')
        before = args.input_managed.read_bytes() if args.input_managed else archive.read(ENTRY)
        host = work / 'Assembly-CSharp.dll'
        host.write_bytes(before)
        refs = [host]
        for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll']:
            path = work / name
            path.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
            refs.append(path)
        helper = out / 'HYJS.Gameplay.dll'
        run('helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
                       '/deterministic+', '/out:' + str(helper),
                       *['/reference:' + str(path) for path in refs], source / 'GameplayPreferences.cs'])
        shutil.copy2(source / 'Injector.cs', work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib">'
                           '<HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run('injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        run('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', host, out / 'Assembly-CSharp.dll'])
    after = (out / 'Assembly-CSharp.dll').read_bytes()
    left, right = (dnfile.dnPE(data=data) for data in (before, after))
    old_methods, new_methods = list(methods(left)), list(methods(right))
    require(len(old_methods) == len(new_methods), 'Original method count changed')
    changes, engine = [], 0
    for (name, index, old), (new_name, new_index, new) in zip(old_methods, new_methods):
        require((name, index, old.Signature.value, old.struct.Flags) ==
                (new_name, new_index, new.Signature.value, new.struct.Flags), 'Original ABI changed: ' + name)
        if not old.Rva:
            continue
        if exact_body(left, old) == exact_body(right, new):
            engine += name.startswith('Battle.Engine')
            continue
        allowed = name in ('Battle.SpeedBtn::OnClick', 'Battle.View.EntranceState::Leave') or (
            name == 'BattleData::SetData')
        require(allowed, 'Unrelated original method changed: ' + name)
        changes.append({'name': name, 'token': index, 'signatureSHA256': sha(old.Signature.value)})
    require(len(changes) == 4 and engine == 547, 'Expected four hooks with original engine unchanged')
    require(not getattr(right.net.mdtables, 'FieldPtr', None), 'Unsupported metadata indirection')
    manifest = dict(schema='hyjs-gameplay45-v1', status='STATIC_VERIFIED_NOT_PACKAGED',
                    baseAPKSHA256=args.expected_sha256, inputManagedSHA256=sha(before),
                    outputManagedSHA256=sha(after), allowedChangedMethods=changes,
                    originalEngineMethodsUnchanged=engine, snowBattleBundleSHA256=sha(snow),
                    replacementEntries={ENTRY: sha(after), HELPER: sha(helper.read_bytes())},
                    payloadFiles={ENTRY: str((out / 'Assembly-CSharp.dll').relative_to(ROOT)),
                                  HELPER: str(helper.relative_to(ROOT))},
                    sourcePins={str(path.relative_to(ROOT)): sha(path.read_bytes())
                                for path in (source / 'GameplayPreferences.cs', source / 'Injector.cs', Path(__file__).resolve())})
    (out / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'status': manifest['status'], 'changes': changes, 'engine': engine}))


if __name__ == '__main__':
    main()
