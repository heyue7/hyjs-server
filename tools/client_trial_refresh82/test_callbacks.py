"""Exercise callback ownership using the actual helper source and boundary doubles."""
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DOTNET = Path("/Users/lirui/LR/hyjs/.tools/native-batch/dotnet/dotnet")
with tempfile.TemporaryDirectory(prefix="hyjs-trial82-callback-test-") as temporary:
    path = Path(temporary)
    (path / "Trial.cs").write_text((ROOT / "tools/ggo_items/client/TrialShopRefresh75.cs").read_text())
    (path / "Program.cs").write_text((ROOT / "tools/client_trial_refresh82/CallbackTests.cs").read_text())
    project = path / "Test.csproj"
    project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><LangVersion>3</LangVersion><ImplicitUsings>disable</ImplicitUsings><Nullable>disable</Nullable></PropertyGroup></Project>')
    result = subprocess.run(["rtk", "proxy", str(DOTNET), "run", "--project", str(project), "--verbosity", "quiet", "-p:NuGetAudit=false"],
                            cwd=ROOT, capture_output=True, text=True, timeout=120)
    print(result.stdout, end="")
    if result.returncode:
        print(result.stderr, end="")
        raise SystemExit(result.returncode)
