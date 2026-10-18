"""Execute the production recovery-scope policy without Unity, network or DB."""
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[3]
DOTNET = ROOT / '.tools/native-batch/dotnet/dotnet'


def main():
    with tempfile.TemporaryDirectory(prefix='hyjs-worldboss-policy-') as directory:
        temp = Path(directory)
        shutil.copy2(ROOT / 'tools/p12_worldboss/LocalWorldBossRecoveryPolicy.cs', temp / 'Policy.cs')
        shutil.copy2(Path(__file__).with_name('PolicyChecks.cs'), temp / 'Program.cs')
        project = temp / 'PolicyChecks.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework></PropertyGroup></Project>')
        for args in ([DOTNET, 'build', project, '--nologo', '--verbosity', 'quiet'],
                     [DOTNET, temp / 'bin/Debug/net8.0/PolicyChecks.dll']):
            result = subprocess.run(['rtk', 'proxy', *map(str, args)], capture_output=True, text=True)
            if result.stdout:
                print(result.stdout.strip())
            if result.returncode:
                print(result.stderr.strip())
                raise RuntimeError('world boss recovery scope checks failed')


if __name__ == '__main__':
    main()
