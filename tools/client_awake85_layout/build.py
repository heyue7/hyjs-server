"""Build a pinned v85 helper and night-forest manifest overlay; never package an APK."""

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
SOURCE = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
from tools.client_arena_state.build import decode, exact_body, instruction_value, member_reference, methods

MANAGED = "assets/bin/Data/Managed/"
MAIN = MANAGED + "Assembly-CSharp.dll"
HELPER = MANAGED + "HYJS.DungeonContent.dll"
SMALL = "assets/android/smallassetmanifest"
NIGHT = "assets/android/map/map_nightforestfield_001"
NIGHT_SHA = "60eafad353de37c0e2a13c2b90b35079d2a48a3b185404d327a63ce2a36a776c"
NIGHT_MD5 = "1fdbc4decf711cab5e8031ecb2033079"
NIGHT_SIZE = 13943479


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def pin_file(path, expected):
    with path.open("rb") as f:
        require(hashlib.file_digest(f, "sha256").hexdigest() == expected, "Input digest differs: " + str(path))


def small_overlay(raw):
    rows = raw.splitlines(keepends=True)
    require(bool(rows) and rows[0].rstrip(b"\r\n") == b"3000", "Small manifest version differs")
    key = b"map/map_nightforestfield_001"
    exact = key + b"\t" + NIGHT_MD5.encode() + b"\t" + str(NIGHT_SIZE).encode()
    found = []
    seen = set()
    for row in rows[1:]:
        data = row.rstrip(b"\r\n")
        require(bool(data), "Unexpected blank small manifest row")
        fields = data.split(b"\t")
        require(len(fields) == 3 and fields[0] not in seen, "Malformed or duplicate small manifest row")
        seen.add(fields[0])
        if fields[0] == key:
            found.append(data)
    require(len(found) <= 1 and (not found or found[0] == exact), "Night forest manifest row conflicts")
    if found:
        return raw, False
    newline = b"\r\n" if rows[0].endswith(b"\r\n") else b"\n"
    separator = b"" if raw.endswith((b"\n", b"\r")) else newline
    output = raw + separator + exact + newline
    require(output[:len(raw)] == raw and output[len(raw):] == separator + exact + newline,
            "Unexpected small manifest mutation")
    return output, True


def engine_and_hooks(original, actual):
    base, target = dnfile.dnPE(data=original), dnfile.dnPE(data=actual)
    base_rows = {(n, rid): m for n, rid, m in methods(base) if n.startswith("Battle.Engine") and m.Rva}
    target_rows = {(n, rid): m for n, rid, m in methods(target) if n.startswith("Battle.Engine") and m.Rva}
    require(len(base_rows) == 547 and base_rows.keys() == target_rows.keys(), "Original Engine inventory differs")
    for key, before in base_rows.items():
        after = target_rows[key]
        require(before.Signature.value == after.Signature.value and exact_body(base, before) == exact_body(target, after),
                "Original Engine body differs: " + key[0])
    specs = json.loads((ROOT / "tools/client_dungeon_access/targets.json").read_text())["targets"]
    base_methods = {(n, rid): m for n, rid, m in methods(base)}
    target_methods = {(n, rid): m for n, rid, m in methods(target)}
    checks = []
    for spec in specs:
        key = (spec["fullName"], int(spec["token"], 16) & 0xffffff)
        method = target_methods.get(key)
        require(method is not None and method.Rva and method.Signature.value.hex() == spec["signatureBlobHex"],
                "Pinned Main hook ABI differs: " + spec["fullName"])
        original_method = base_methods.get(key)
        require(original_method is not None and original_method.Rva and
                original_method.Signature.value == method.Signature.value and
                exact_body(base, original_method) == exact_body(target, method),
                "Pinned Main hook body differs: " + spec["fullName"])
        calls = []
        for ins in decode(target, method):
            token = instruction_value(ins)
            if ins.opcode.name not in ("call", "callvirt") or not isinstance(token, int) or token >> 24 != 10:
                continue
            ref = member_reference(target, token)
            owner = ref.Class.row
            fullname = str(getattr(owner, "TypeNamespace", "")) + "." + str(getattr(owner, "TypeName", ""))
            if fullname == spec["helperType"] and str(ref.Name) == spec["helperMethod"]:
                calls.append(token)
        require(len(calls) == 1, "Pinned Main helper call differs: " + spec["fullName"])
        checks.append({"method": spec["fullName"], "token": spec["token"], "helperMethod": spec["helperMethod"],
                       "methodBodySHA256": sha(exact_body(target, method))})
    return checks


def run(label, command, out, cwd=None):
    result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=cwd, text=True, capture_output=True, timeout=180)
    (out / (label + ".log")).write_text(result.stdout + result.stderr)
    require(result.returncode == 0, label + " failed; inspect task log")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--base-sha256", required=True)
    parser.add_argument("--expected-helper-sha256", required=True)
    parser.add_argument("--input-managed", type=Path)
    parser.add_argument("--input-managed-sha256")
    parser.add_argument("--tool-root", type=Path, required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    base = args.base_apk.resolve()
    out = args.output_directory.resolve()
    allowed = (ROOT / "analysis/fragment-box85/awake-copy-entry/client").resolve()
    require(out.is_relative_to(allowed) and not out.exists(), "Fresh task-owned output directory required")
    require(bool(args.input_managed) == bool(args.input_managed_sha256), "Managed override and SHA must be paired")
    pin_file(base, args.base_sha256)
    if args.input_managed:
        pin_file(args.input_managed.resolve(), args.input_managed_sha256)
    tool_root = args.tool_root.resolve()
    dotnet = tool_root / "dotnet/dotnet"
    compiler = next((tool_root / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    dnlib = tool_root / "injector/bin/Debug/net8.0/dnlib.dll"
    require(dotnet.is_file() and compiler.is_file() and dnlib.is_file(), "Existing .NET toolchain unavailable")
    out.mkdir(parents=True)
    with zipfile.ZipFile(base) as z, tempfile.TemporaryDirectory(prefix="hyjs-awake85-build-") as td:
        names = z.namelist()
        require(len(names) == len(set(names)), "Duplicate APK entries")
        original_helper = z.read(HELPER)
        require(sha(original_helper) == args.expected_helper_sha256, "Input helper digest differs")
        base_main = z.read(MAIN)
        actual_main = args.input_managed.read_bytes() if args.input_managed else base_main
        hook_checks = engine_and_hooks(base_main, actual_main)
        night = z.read(NIGHT)
        require(len(night) == NIGHT_SIZE and sha(night) == NIGHT_SHA and hashlib.md5(night).hexdigest() == NIGHT_MD5,
                "Night forest bundle differs")
        old_small = z.read(SMALL)
        new_small, added = small_overlay(old_small)
        work = Path(td)
        host = work / "Assembly-CSharp.dll"
        host.write_bytes(actual_main)
        old_helper = work / "old-HYJS.DungeonContent.dll"
        old_helper.write_bytes(original_helper)
        refs = [host]
        for name in ["mscorlib.dll", "System.dll", "System.Core.dll", "UnityEngine.dll"]:
            path = work / name
            path.write_bytes(z.read(MANAGED + name))
            refs.append(path)
        helper = out / "HYJS.DungeonContent.dll"
        run("compile-helper", [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3",
             "/deterministic+", "/codepage:65001", "/out:" + str(helper),
             *["/reference:" + str(p) for p in refs], ROOT / "tools/client_dungeon_access/Runtime.cs",
             SOURCE / "Awake85Contract.cs"], out)

        project = work / "verify"
        project.mkdir()
        (project / "Program.cs").write_bytes((SOURCE / "VerifyHelper.cs").read_bytes())
        (project / "NuGet.Config").write_text('<configuration><packageSources><clear /></packageSources></configuration>')
        (project / "Verify.csproj").write_text(
            '<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
            '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
            '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'
            + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run("compile-verifier", [dotnet, "build", project / "Verify.csproj", "--nologo", "--verbosity", "quiet",
             "-p:NuGetAudit=false", "--configfile", project / "NuGet.Config"], out, cwd=project)
        semantic = out / "helper-compatibility.json"
        run("verify-helper", [dotnet, project / "bin/Debug/net8.0/Verify.dll", old_helper, helper, semantic], out)
        (out / "smallassetmanifest").write_bytes(new_small)
        source_files = [ROOT / "tools/client_dungeon_access/Runtime.cs", SOURCE / "Awake85Contract.cs",
                        SOURCE / "VerifyHelper.cs", SOURCE / "build.py"]
        report = {
            "schema": "hyjs-awake85-helper-overlay-v1",
            "status": "STATIC_VERIFIED_NOT_PACKAGED",
            "baseAPK": str(base), "baseAPKSHA256": args.base_sha256,
            "inputManagedSHA256": sha(actual_main), "baseManagedSHA256": sha(base_main),
            "inputHelperSHA256": sha(original_helper), "outputHelperSHA256": sha(helper.read_bytes()),
            "smallManifestInputSHA256": sha(old_small), "smallManifestOutputSHA256": sha(new_small),
            "nightEntryAdded": added, "nightBundleSHA256": NIGHT_SHA,
            "payloadFiles": {HELPER: str(helper.relative_to(ROOT)), SMALL: str((out / "smallassetmanifest").relative_to(ROOT))},
            "replacementEntries": {HELPER: sha(helper.read_bytes()), SMALL: sha(new_small)},
            "sourcePins": {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in source_files},
            "mainBytesModified": False, "originalEngineMethodsCompared": 547,
            "existingMainHooks": hook_checks, "helperCompatibility": "helper-compatibility.json",
            "runtimeOrPhoneVerified": False,
        }
        (out / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({"helperSHA256": report["outputHelperSHA256"], "nightEntryAdded": added,
                          "existingHooks": len(hook_checks), "engineMethods": 547}))


if __name__ == "__main__":
    main()
