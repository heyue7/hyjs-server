"""Run original and emitted CIL paths in a bounded Unity/network model."""
import argparse
import hashlib
import json
import re
import shutil
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent


def file_sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--baseline-managed', required=True, type=Path)
    parser.add_argument('--baseline-sha256', required=True)
    parser.add_argument('--managed', required=True, type=Path)
    parser.add_argument('--managed-sha256', required=True)
    parser.add_argument('--report', required=True, type=Path)
    parser.add_argument('--tool-root', default='/Users/lirui/LR/hyjs', type=Path)
    args = parser.parse_args()
    baseline, main, report = args.baseline_managed.resolve(), args.managed.resolve(), args.report.resolve()
    for path, expected in ((baseline, args.baseline_sha256), (main, args.managed_sha256)):
        if not re.fullmatch('[0-9a-fA-F]{64}', expected) or file_sha(path) != expected.lower():
            raise ValueError('managed SHA differs: ' + str(path))
    if not report.is_relative_to(ROOT / 'analysis/fragment-box85/relations') or report.exists():
        raise ValueError('fresh task-owned report required')
    runtime = args.tool_root.resolve() / '.tools/native-batch'
    dotnet, dnlib = runtime / 'dotnet/dotnet', runtime / 'injector/bin/Debug/net8.0/dnlib.dll'
    logs = []
    with tempfile.TemporaryDirectory(prefix='hyjs-relations85-tests-') as tmp:
        work = Path(tmp)
        shutil.copy2(HERE / 'ModelTests.cs', work / 'Program.cs')
        project = work / 'Tests.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '<Nullable>disable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' +
                           str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        for command in ([dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'],
                        [dotnet, work / 'bin/Debug/net8.0/Tests.dll', baseline, main]):
            result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT,
                                    capture_output=True, text=True, timeout=120)
            logs.append(result.stdout + result.stderr)
            if result.returncode:
                raise RuntimeError('model failed:\n' + '\n'.join(logs))
    if file_sha(baseline) != args.baseline_sha256.lower() or file_sha(main) != args.managed_sha256.lower():
        raise ValueError('managed input changed during tests')
    result = {'status': 'PASS', 'baselineManagedSHA256': args.baseline_sha256.lower(),
              'managedSHA256': args.managed_sha256.lower(), 'testSourceSHA256': file_sha(HERE / 'ModelTests.cs'),
              'method': 'dnlib reads emitted method CIL; a bounded interpreter executes selected paths against Unity/network stand-ins',
              'deviceExecuted': False, 'unityExecuted': False, 'networkExecuted': False, 'output': logs[-1]}
    report.write_text(json.dumps(result, indent=2, ensure_ascii=False) + '\n')
    print(logs[-1], end='')


if __name__ == '__main__':
    main()
