"""Check composed Main overlays and execute their comparison opcodes on .NET."""

import argparse
import json
import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path

import build_main


def run(command, cwd):
    result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=cwd, text=True,
                            capture_output=True, timeout=90,
                            env={**os.environ, "DOTNET_CLI_TELEMETRY_OPTOUT": "1",
                                 "DOTNET_SKIP_FIRST_TIME_EXPERIENCE": "1"})
    if result.returncode:
        raise RuntimeError("command failed: " + result.stdout + result.stderr)
    return result.stdout.strip()


def validate_case(source, out):
    report = json.loads((out / "manifest.json").read_text())
    before = source.read_bytes()
    after = (out / "Assembly-CSharp.dll").read_bytes()
    if report["schema"] != build_main.SCHEMA or build_main.sha(before) != report["inputManagedSHA256"] or build_main.sha(after) != report["outputManagedSHA256"]:
        raise ValueError("case manifest or digest changed")
    rebuilt, proof = build_main.patch(before)
    if rebuilt != after or any(report[k] != v for k, v in proof.items()):
        raise ValueError("overlay differs from deterministic in-memory patch")
    try:
        build_main.apply_overlay({build_main.ENTRY: before}, "0" * 64)
    except ValueError:
        pass
    else:
        raise ValueError("wrong composed-input SHA was accepted")
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--case", action="append", nargs=2, required=True,
                        metavar=("INPUT_MANAGED", "OVERLAY_DIRECTORY"))
    parser.add_argument("--dotnet", type=Path,
                        default=Path("/Users/lirui/LR/hyjs/.tools/native-batch/dotnet/dotnet"))
    args = parser.parse_args()
    cases = [(Path(source).resolve(), Path(out).resolve()) for source, out in args.case]
    reports = [validate_case(source, out) for source, out in cases]
    with tempfile.TemporaryDirectory(prefix="hyjs-breach84-clr-") as temp:
        temp = Path(temp)
        shutil.copyfile(Path(__file__).with_name("LabelBranchProbe.cs"), temp / "LabelBranchProbe.cs")
        (temp / "Probe.csproj").write_text(
            '<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
            '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>disable</ImplicitUsings>'
            '<Nullable>disable</Nullable></PropertyGroup></Project>')
        config = temp / "NuGet.Config"
        config.write_text('<configuration><packageSources><clear /></packageSources></configuration>')
        run([args.dotnet, "restore", temp / "Probe.csproj", "--configfile", config,
             "--nologo", "-p:NuGetAudit=false"], temp)
        run([args.dotnet, "build", temp / "Probe.csproj", "--no-restore", "--nologo",
             "--configuration", "Release", "--output", temp / "out"], temp)
        runtime_version = run([args.dotnet, "--version"], temp)
        for (source, out), report in zip(cases, reports):
            offsets = [str(p["fileOffset"]) for p in report["patches"]]
            message = run([args.dotnet, temp / "out/Probe.dll", source, out / "Assembly-CSharp.dll",
                           *offsets, build_main.ROOT / "analysis/tables/TableHeroBreached.tsv"], temp)
            match = re.fullmatch(r"CLR_OK stages=(\d+) expStates=(\d+) changedBoundaryCases=(\d+)", message)
            if match is None:
                raise ValueError("unexpected CLR probe result: " + message)
            table = build_main.ROOT / "analysis/tables/TableHeroBreached.tsv"
            evidence = {
                "schema": "hyjs-breach84-clr-label-comparison-v1",
                "scope": "isolated CLR comparison using the two target method opcode bytes; Unity UI not executed",
                "inputManagedSHA256": report["inputManagedSHA256"],
                "outputManagedSHA256": report["outputManagedSHA256"],
                "sourceTableSHA256": build_main.sha(table.read_bytes()),
                "probeSourceSHA256": build_main.sha(Path(__file__).with_name("LabelBranchProbe.cs").read_bytes()),
                "verificationSourceSHA256": build_main.sha(Path(__file__).read_bytes()),
                "dotnetVersion": runtime_version,
                "stages": int(match.group(1)),
                "validExperienceStates": int(match.group(2)),
                "changedBoundaryCases": int(match.group(3)),
                "deviceTested": False,
            }
            proof_file = out / "clr-proof.json"
            encoded = json.dumps(evidence, ensure_ascii=False, indent=2) + "\n"
            if proof_file.exists() and proof_file.read_text() != encoded:
                raise ValueError("existing CLR proof differs; prepare a fresh output directory")
            proof_file.write_text(encoded)
            print(source, message)


if __name__ == "__main__":
    main()
