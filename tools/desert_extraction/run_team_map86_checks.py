#!/usr/bin/env python3
"""Run bounded, isolated TeamMap86 behavior checks against the real source."""
import argparse
from pathlib import Path
import subprocess
import tempfile

here = Path(__file__).resolve().parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--runtime', type=Path, required=True)
args = parser.parse_args()
dotnet = args.runtime.resolve() / 'dotnet' / 'dotnet'
with tempfile.TemporaryDirectory(prefix='hyjs-teammap86-check-') as directory:
    root = Path(directory)
    project = root / 'Checks.csproj'
    project.write_text('''<Project Sdk="Microsoft.NET.Sdk">
<PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework>
<LangVersion>3</LangVersion><ImplicitUsings>disable</ImplicitUsings><Nullable>disable</Nullable>
<EnableDefaultCompileItems>false</EnableDefaultCompileItems><GenerateAssemblyInfo>false</GenerateAssemblyInfo>
<GenerateTargetFrameworkAttribute>false</GenerateTargetFrameworkAttribute></PropertyGroup><ItemGroup>
<Compile Include="%s"/><Compile Include="%s"/>
</ItemGroup></Project>''' % (here / 'TeamMap86.cs', here / 'TeamMap86Checks.cs'))
    subprocess.run(['rtk', 'run', str(dotnet), 'build', str(project), '--nologo', '--verbosity', 'quiet',
                    '-p:NuGetAudit=false'], check=True, timeout=120)
    subprocess.run(['rtk', 'run', str(dotnet), str(root / 'bin/Debug/net8.0/Checks.dll')], check=True, timeout=30)
