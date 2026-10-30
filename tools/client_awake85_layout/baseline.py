"""Rebuild the pinned pre-v85 helper source and compare it with an APK helper."""

import argparse
import hashlib
import json
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_arena_state.build import exact_body, methods

ENTRY = "assets/bin/Data/Managed/"
BASELINE_COMMIT = "b0e684edde8f847c7371aaed093cbd2d47f5d9d8"
BASELINE_SHA = "f7df21a3f52f73dcd9c8899fd0d33ac18f085f8be37770d26eb5683ccf99bb49"
TOOL_ROOT = Path("/Users/lirui/LR/hyjs/.tools/native-batch")


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--base-apk", type=Path, required=True)
    ap.add_argument("--base-sha256", required=True)
    ap.add_argument("--helper-sha256", required=True)
    ap.add_argument("--output-directory", type=Path, required=True)
    args = ap.parse_args()
    base = args.base_apk.resolve()
    out = args.output_directory.resolve()
    allowed = ROOT / "analysis/fragment-box85/awake-copy-entry/client"
    if not out.is_relative_to(allowed) or out.exists():
        raise ValueError("Fresh task-owned output directory required")
    with base.open("rb") as f:
        if hashlib.file_digest(f, "sha256").hexdigest() != args.base_sha256:
            raise ValueError("Base APK digest differs")
    original_source = subprocess.run(
        ["rtk", "proxy", "git", "show", BASELINE_COMMIT + ":tools/client_dungeon_access/Runtime.cs"],
        cwd=ROOT, capture_output=True, check=True, timeout=60).stdout
    if digest(original_source) != BASELINE_SHA:
        raise ValueError("Pinned baseline source differs")
    out.mkdir(parents=True)
    compiler = next((TOOL_ROOT / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    dotnet = TOOL_ROOT / "dotnet/dotnet"
    with zipfile.ZipFile(base) as z, tempfile.TemporaryDirectory(prefix="hyjs-awake85-probe-") as td:
        names = z.namelist()
        if len(names) != len(set(names)):
            raise ValueError("Duplicate APK entries")
        original = z.read(ENTRY + "HYJS.DungeonContent.dll")
        if digest(original) != args.helper_sha256:
            raise ValueError("Helper digest differs")
        work = Path(td)
        runtime = work / "Runtime.cs"
        runtime.write_bytes(original_source)
        refs = []
        for name in ["Assembly-CSharp.dll", "mscorlib.dll", "System.dll", "System.Core.dll", "UnityEngine.dll"]:
            path = work / name
            path.write_bytes(z.read(ENTRY + name))
            refs.append(path)
        built = out / "HYJS.DungeonContent.dll"
        cmd = ["rtk", "proxy", str(dotnet), str(compiler), "/nologo", "/target:library", "/nostdlib+",
               "/langversion:3", "/deterministic+", "/codepage:65001", "/out:" + str(built),
               *["/reference:" + str(p) for p in refs], str(runtime)]
        run = subprocess.run(cmd, text=True, capture_output=True, timeout=180)
        (out / "compile.log").write_text(run.stdout + run.stderr)
        if run.returncode:
            raise RuntimeError("Helper compilation failed; inspect compile.log")
        a, b = dnfile.dnPE(data=original), dnfile.dnPE(data=built.read_bytes())
        left = {name: m for name, _, m in methods(a)}
        right = {name: m for name, _, m in methods(b)}
        common = sorted(left.keys() & right.keys())
        body_same = []
        body_different = []
        signature_different = []
        for name in common:
            x, y = left[name], right[name]
            if x.Signature.value != y.Signature.value or x.struct.Flags != y.struct.Flags:
                signature_different.append(name)
            if bool(x.Rva) != bool(y.Rva) or (x.Rva and exact_body(a, x) != exact_body(b, y)):
                body_different.append(name)
            else:
                body_same.append(name)
        report = {
            "base_sha256": args.base_sha256,
            "main_sha256": digest(z.read(ENTRY + "Assembly-CSharp.dll")),
            "original_helper_sha256": digest(original),
            "source_commit": BASELINE_COMMIT,
            "source_sha256": digest(original_source),
            "compiled_helper_sha256": digest(built.read_bytes()),
            "original_methods": len(left), "compiled_methods": len(right),
            "original_only": sorted(left.keys() - right.keys()),
            "compiled_only": sorted(right.keys() - left.keys()),
            "signature_or_flags_changed": signature_different,
            "raw_body_equal": body_same,
            "raw_body_changed": body_different,
            "scope": "raw bodies include metadata token differences; semantic equivalence not inferred",
        }
        (out / "baseline.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps({k: report[k] for k in ["original_methods", "compiled_methods", "original_only", "compiled_only", "signature_or_flags_changed", "raw_body_changed"]}))


if __name__ == "__main__":
    main()
