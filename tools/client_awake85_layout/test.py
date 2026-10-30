"""Run scoped v85 identity, slot, restore and manifest checks without Unity or a device."""

import argparse
import hashlib
import json
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SOURCE = Path(__file__).resolve().parent
sys.path.insert(0, str(SOURCE))
from build import NIGHT_MD5, NIGHT_SIZE, small_overlay


def require(ok, message):
    if not ok:
        raise ValueError(message)


def fail(function):
    try:
        function()
    except ValueError:
        return
    raise AssertionError("Expected manifest rejection")


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base-apk", type=Path, required=True)
    ap.add_argument("--base-sha256", required=True)
    ap.add_argument("--tool-root", type=Path, required=True)
    ap.add_argument("--output-directory", type=Path, required=True)
    args = ap.parse_args()
    out = args.output_directory.resolve()
    allowed = (ROOT / "analysis/fragment-box85/awake-copy-entry/client").resolve()
    require(out.is_relative_to(allowed) and not out.exists(), "Fresh task-owned test directory required")
    with args.base_apk.open("rb") as f:
        require(hashlib.file_digest(f, "sha256").hexdigest() == args.base_sha256, "Base APK differs")
    out.mkdir(parents=True)
    base = b"3000\nmap/example\t0123456789abcdef0123456789abcdef\t10\n"
    added, changed = small_overlay(base)
    require(changed and added.startswith(base), "Existing small manifest rows changed")
    identical, changed = small_overlay(added)
    require(not changed and identical == added, "Existing exact row is not idempotent")
    line = b"map/map_nightforestfield_001\t" + NIGHT_MD5.encode() + b"\t" + str(NIGHT_SIZE).encode()
    require(added.count(line) == 1, "Expected exactly one inserted row")
    crlf, changed = small_overlay(base.replace(b"\n", b"\r\n"))
    require(changed and crlf.endswith(line + b"\r\n"), "Small manifest newline drift")
    fail(lambda: small_overlay(base + b"map/map_nightforestfield_001\tbad\t1\n"))
    fail(lambda: small_overlay(added + line + b"\n"))
    tool = args.tool_root.resolve()
    dotnet = tool / "dotnet/dotnet"
    compiler = next((tool / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    with zipfile.ZipFile(args.base_apk) as z, tempfile.TemporaryDirectory(prefix="hyjs-awake85-tests-") as td:
        work = Path(td)
        refs = []
        for name in ["mscorlib.dll", "System.dll", "System.Core.dll"]:
            p = work / name
            p.write_bytes(z.read("assets/bin/Data/Managed/" + name))
            refs.append(p)
        program = work / "Awake85ContractTests.exe"
        cmd = ["rtk", "proxy", str(dotnet), str(compiler), "/nologo", "/target:exe", "/nostdlib+",
               "/langversion:3", "/deterministic+", "/codepage:65001", "/out:" + str(program),
               *["/reference:" + str(p) for p in refs], str(SOURCE / "Awake85Contract.cs"),
               str(SOURCE / "ContractTests.cs")]
        compiled = subprocess.run(cmd, capture_output=True, text=True, timeout=180)
        (out / "compile.log").write_text(compiled.stdout + compiled.stderr)
        require(compiled.returncode == 0, "Contract test compilation failed")
        (work / "Awake85ContractTests.runtimeconfig.json").write_text(json.dumps({
            "runtimeOptions": {"tfm": "net8.0", "framework": {"name": "Microsoft.NETCore.App", "version": "8.0.0"}}
        }))
        executed = subprocess.run(["rtk", "proxy", str(dotnet), str(program)], capture_output=True, text=True, timeout=60)
        (out / "contract-tests.log").write_text(executed.stdout + executed.stderr)
        require(executed.returncode == 0 and "PASS " in executed.stdout, "Contract tests failed")
        result = {
            "scope": "pure compiled C#3 contract and small-manifest inputs; no Unity NavMesh, device, or service",
            "contractSourceSHA256": hashlib.sha256((SOURCE / "Awake85Contract.cs").read_bytes()).hexdigest(),
            "testSourceSHA256": hashlib.sha256((SOURCE / "ContractTests.cs").read_bytes()).hexdigest(),
            "testProgramSHA256": hashlib.sha256(program.read_bytes()).hexdigest(),
            "result": executed.stdout.strip(),
            "manifestChecks": ["insert preserves rows", "exact row idempotent", "CRLF preserved", "conflict rejected", "duplicate rejected"],
            "baseAPKSHA256": args.base_sha256,
        }
        (out / "tests.json").write_text(json.dumps(result, indent=2) + "\n")
        print(result["result"] + "; small manifest checks PASS")


if __name__ == "__main__":
    main()
