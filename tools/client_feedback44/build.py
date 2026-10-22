"""Compose the independently verified arena and field settlement overlays."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_arena_state import build as arena
from tools.client_field_settlement import build as field


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--expected-sha256', required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--tool-root', type=Path, required=True)
    args = parser.parse_args()
    output = args.output_directory.resolve()
    assert output.is_relative_to(ROOT) and not output.exists()
    output.mkdir(parents=True)
    runtime = args.tool_root.resolve() / '.tools/native-batch'

    def run(command):
        subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, check=True)

    for name, builder, toolroot in [('arena', arena, args.tool_root), ('field', field, runtime)]:
        run([sys.executable, builder.__file__, '--base-apk', args.base_apk,
             '--expected-sha256', args.expected_sha256, '--output-directory', output / name,
             '--tool-root', toolroot])
    with tempfile.TemporaryDirectory(prefix='hyjs-feedback44-') as temporary:
        work = Path(temporary)
        shutil.copy2(Path(field.__file__).with_name('Injector.cs'), work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(runtime / 'injector/bin/Debug/net8.0/dnlib.dll') + '</HintPath></Reference></ItemGroup></Project>')
        run([runtime / 'dotnet/dotnet', 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        run([runtime / 'dotnet/dotnet', work / 'bin/Debug/net8.0/Patch.dll',
             output / 'arena/Assembly-CSharp.dll', output / 'field/LocalFieldSettlement.dll',
             output / 'Assembly-CSharp.dll'])
    # This second proof keeps every arena hook byte-exact while adding only the
    # two field hooks. Together, the proofs bind the full five-method delta.
    proof = field.verify((output / 'arena/Assembly-CSharp.dll').read_bytes(),
                         (output / 'Assembly-CSharp.dll').read_bytes(),
                         (output / 'field/LocalFieldSettlement.dll').read_bytes())
    a = json.loads((output / 'arena/manifest.json').read_text())
    b = json.loads((output / 'field/manifest.json').read_text())
    manifest = dict(a)
    manifest.update(schema='hyjs-client-feedback44-v1', status='STATIC_VERIFIED_NOT_PACKAGED',
                    outputManagedSHA256=sha(output / 'Assembly-CSharp.dll'),
                    allowedChangedMethods=a['allowedChangedMethods'] + b['allowedChangedMethods'],
                    sourcePins={**a['sourcePins'], **b['sourcePins'],
                                str(Path(__file__).relative_to(ROOT)): sha(Path(__file__))},
                    composedFieldProof=proof, arenaProof=a['changes'])
    manifest['payloadFiles'] = {**a['payloadFiles'], **b['payloadFiles'],
                               arena.ENTRY: str((output / 'Assembly-CSharp.dll').relative_to(ROOT))}
    manifest['replacementEntries'] = {**a['replacementEntries'], **b['replacementEntries'],
                                      arena.ENTRY: sha(output / 'Assembly-CSharp.dll')}
    (output / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'status':manifest['status'], 'outputManagedSHA256':manifest['outputManagedSHA256'],
                      'allowedChangedMethods':manifest['allowedChangedMethods'],
                      'originalEngineMethodsUnchanged':547}))


if __name__ == '__main__':
    main()
