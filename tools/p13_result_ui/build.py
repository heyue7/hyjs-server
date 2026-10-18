"""Remove original guild result prefab placeholders; no APK packaging or install."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.p10_voice.build import verify

ENTRIES = {('UIUnionFightBattleResult', 'SetPerson'), ('UIUnionFightBattleResult', 'SetAward'),
           ('UIUnionFightPlayerItem', 'SetPlayerItem'), ('UIUnionFightAwardItem', 'SetItem')}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', required=True, type=Path)
    parser.add_argument('--output-directory', required=True, type=Path)
    args = parser.parse_args()
    if args.output_directory.exists() and any(args.output_directory.iterdir()):
        raise ValueError('Output directory must be empty')
    args.output_directory.mkdir(parents=True, exist_ok=True)
    work = ROOT / '.tools/native-batch'
    dotnet = work / 'dotnet/dotnet'
    dnlib = work / 'injector/bin/Debug/net8.0/dnlib.dll'
    output = args.output_directory / 'Assembly-CSharp.dll'
    # The compiler and generated project stay private and are removed after verification.
    with tempfile.TemporaryDirectory(prefix='hyjs-guild-result-ui-') as temporary:
        temporary = Path(temporary)
        shutil.copy2(Path(__file__).with_name('ResultInjector.cs'), temporary / 'Program.cs')
        project = temporary / 'ResultInjector.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '</PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) +
                           '</HintPath></Reference></ItemGroup></Project>')
        for command in ([dotnet, 'build', project, '--nologo', '--verbosity', 'quiet'],
                        [dotnet, temporary / 'bin/Debug/net8.0/ResultInjector.dll', args.input.resolve(), output.resolve()]):
            subprocess.run(['rtk', 'proxy', *map(str, command)], check=True)
    proof = verify(args.input.read_bytes(), output.read_bytes(), ENTRIES)
    proof.update(inputSHA256=hashlib.sha256(args.input.read_bytes()).hexdigest(),
                 outputSHA256=hashlib.sha256(output.read_bytes()).hexdigest(),
                 scope='hide unreturned prefab rows; activate only original server-bound rows',
                 phoneVerified=False)
    (args.output_directory / 'manifest.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps(proof))


if __name__ == '__main__':
    main()
