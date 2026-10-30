"""Compile the actual Wanted helper with bounded callback test doubles."""
import argparse
import subprocess
import tempfile
from pathlib import Path
from xml.sax.saxutils import escape

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tool-root", type=Path, default=Path("/Users/lirui/LR/hyjs"))
    args = parser.parse_args()
    dotnet = args.tool_root.resolve() / ".tools/native-batch/dotnet/dotnet"
    sources = [ROOT / "tools/client_feedback65/Runtime.cs", HERE / "TestStubs.cs", HERE / "ClientTests.cs"]
    snapshots = {source: source.read_bytes() for source in sources}
    with tempfile.TemporaryDirectory(prefix="hyjs-wanted84-tests-") as temporary:
        work = Path(temporary)
        project = work / "WantedTests.csproj"
        stable_sources = []
        for index, source in enumerate(sources):
            stable = work / (str(index) + "-" + source.name)
            stable.write_bytes(snapshots[source])
            stable_sources.append(stable)
        links = "".join('<Compile Include="' + escape(str(path)) + '" />' for path in stable_sources)
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><EnableDefaultCompileItems>false</EnableDefaultCompileItems><ImplicitUsings>disable</ImplicitUsings><Nullable>disable</Nullable></PropertyGroup><ItemGroup>' + links + '</ItemGroup></Project>')
        subprocess.run(["rtk", "proxy", str(dotnet), "build", str(project), "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"], cwd=ROOT, check=True, timeout=180)
        subprocess.run(["rtk", "proxy", str(dotnet), str(work / "bin/Debug/net8.0/WantedTests.dll")], cwd=ROOT, check=True, timeout=30)
        if any(source.read_bytes() != snapshot for source, snapshot in snapshots.items()):
            raise RuntimeError("test source changed during execution")


if __name__ == "__main__":
    main()
