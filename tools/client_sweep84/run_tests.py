"""Run .NET iterator-IL and existing sweep-guard source tests with stubs."""
import argparse
import hashlib
import json
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def extract_class(source, marker):
    start = source.index(marker)
    opening = source.index('{', start)
    depth = 0
    for i in range(opening, len(source)):
        if source[i] == '{': depth += 1
        if source[i] == '}':
            depth -= 1
            if depth == 0:
                return source[start:i+1]
    raise ValueError('unterminated source class')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--managed', required=True, type=Path)
    parser.add_argument('--expected-managed-sha256', required=True)
    parser.add_argument('--report', required=True, type=Path)
    parser.add_argument('--tool-root', default='/Users/lirui/LR/hyjs', type=Path)
    args = parser.parse_args()
    report = args.report.resolve()
    if not report.is_relative_to(ROOT/'analysis/feedback84/sweep-client') or report.exists():
        raise ValueError('fresh task-owned report required')
    if sha(args.managed) != args.expected_managed_sha256.lower():
        raise ValueError('managed SHA differs')
    dot = args.tool_root/'.tools/native-batch/dotnet/dotnet'
    dnlib = args.tool_root/'.tools/native-batch/injector/bin/Debug/net8.0/dnlib.dll'
    source = ROOT/'tools/client_stamina_sweep/Stamina.cs'
    source_text = source.read_text()
    files = [Path(__file__).with_name('BehaviorTests.cs'), Path(__file__).with_name('TestStubs.cs')]
    with tempfile.TemporaryDirectory(prefix='hyjs-sweep84-tests-') as directory:
        work = Path(directory)
        for file in files:
            (work/file.name).write_text(file.read_text())
        extracted = 'using System; using System.Collections.Generic; using System.Runtime.CompilerServices;\nnamespace HYJS.Stamina {\n'
        extracted += extract_class(source_text, 'internal sealed class ReferenceComparer<T>') + '\n'
        extracted += extract_class(source_text, 'public static class StaminaSweep') + '\n}\n'
        (work/'Guard.cs').write_text(extracted)
        project = work/'Tests.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        outputs = []
        for cmd in ([dot, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'],
                    [dot, work/'bin/Debug/net8.0/Tests.dll', args.managed.resolve()]):
            result = subprocess.run(['rtk', 'proxy', *map(str, cmd)], cwd=ROOT, text=True, capture_output=True, timeout=120)
            outputs.append(result.stdout+result.stderr)
            if result.returncode:
                raise RuntimeError('test failed:\n' + '\n'.join(outputs))
    if sha(args.managed) != args.expected_managed_sha256.lower():
        raise ValueError('managed changed during tests')
    result = {'status':'PASS', 'managedSHA256':args.expected_managed_sha256.lower(),
              'method':'dnlib reads actual emitted coroutine IL; a .NET interpreter executes it against modeled Unity calls. Existing StaminaSweep source is compiled against stubs.',
              'guardSourceSHA256':sha(source), 'testSources':{str(p.relative_to(ROOT)):sha(p) for p in files},
              'unityExecuted':False, 'phoneExecuted':False, 'networkExecuted':False, 'output':outputs[-1]}
    report.write_text(json.dumps(result, indent=2, ensure_ascii=False)+'\n')
    print(outputs[-1], end='')


if __name__ == '__main__':
    main()
