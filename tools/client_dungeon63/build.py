"""Build the pinned 62 -> 63 dungeon overlay without packaging, installation or network."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile
import dnfile

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
PREFIX = "assets/bin/Data/Managed/"
APK_SHA = "fe427f13e8683b434bf7d2756b2dbd4a6f42746d0d6c66c86264182252704580"
MAIN_SHA = "29bbef55a2458304b7b86b8a0fc1388b8bf3c6c1185ea47dfb58ce0c6e8abb2e"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--base-apk", type=Path, required=True)
    p.add_argument("--input-managed", type=Path)
    p.add_argument("--output-directory", type=Path, required=True)
    args = p.parse_args()
    assert hashlib.file_digest(args.base_apk.open("rb"), "sha256").hexdigest() == APK_SHA
    out = args.output_directory.resolve()
    assert out.is_relative_to(ROOT) and not out.exists(), "Use a fresh task-owned directory"
    out.mkdir(parents=True)
    spec = importlib.util.spec_from_file_location("proof59", ROOT / "tools/client_one_key59/build.py")
    proof = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(proof)
    targets = json.loads((HERE / "targets.json").read_text())
    runtime = ROOT / ".tools/native-batch"
    dotnet = runtime / "dotnet/dotnet"
    compiler = next((runtime / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    with tempfile.TemporaryDirectory(prefix="hyjs-dungeon63-") as temp:
        temp = Path(temp)
        refs = temp / "refs"
        refs.mkdir()
        with zipfile.ZipFile(args.base_apk) as z:
            assert sha(z.read(PREFIX + "Assembly-CSharp.dll")) == MAIN_SHA
            for n in ["Assembly-CSharp.dll", "mscorlib.dll", "System.dll", "System.Core.dll", "UnityEngine.dll"]:
                (refs / n).write_bytes(z.read(PREFIX + n))
        before = args.input_managed.read_bytes() if args.input_managed else (refs / "Assembly-CSharp.dll").read_bytes()
        (refs / "Assembly-CSharp.dll").write_bytes(before)
        pe = dnfile.dnPE(data=before)
        for target in targets:
            row = pe.net.mdtables.MethodDef.rows[(int(target["token"], 16) & 0xffffff) - 1]
            assert sha(proof.cil(pe, row)) == target["cilSHA256"], target
        helper = out / "HYJS.DungeonContent.dll"
        commands = [
            ("compile-helper", [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+", "/codepage:65001", "/out:" + str(helper), *["/r:" + str(f) for f in refs.glob("*.dll")], ROOT / "tools/client_dungeon_access/Runtime.cs"]),
        ]
        project = temp / "Inject.csproj"
        shutil.copy2(HERE / "Injector.cs", temp / "Program.cs")
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(runtime / "injector/bin/Debug/net8.0/dnlib.dll") + '</HintPath></Reference></ItemGroup></Project>')
        commands += [("compile-injector", [dotnet, "build", project, "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"]), ("inject", [dotnet, temp / "bin/Debug/net8.0/Inject.dll", refs / "Assembly-CSharp.dll", helper, out / "Assembly-CSharp.dll"])]
        for label, cmd in commands:
            r = subprocess.run(["rtk", "proxy", *map(str, cmd)], capture_output=True, text=True, timeout=180)
            (out / (label + ".log")).write_text(r.stdout + r.stderr)
            assert r.returncode == 0, label + " failed; inspect owned log"
        checks = proof.verify(before, (out / "Assembly-CSharp.dll").read_bytes(), targets)
        payload = {PREFIX + n: str((out / n).relative_to(ROOT)) for n in ["Assembly-CSharp.dll", helper.name]}
        report = dict(schema="hyjs-dungeon63-managed-overlay-v1", baseAPKSHA256=APK_SHA, inputManagedSHA256=sha(before), **checks, allowedChangedMethods=[t["type"] + "::" + t["method"] for t in targets], payloadFiles=payload, replacementEntries={n: sha((ROOT / f).read_bytes()) for n, f in payload.items()}, sourcePins={str(f.relative_to(ROOT)): sha(f.read_bytes()) for f in [HERE / "Injector.cs", HERE / "targets.json", HERE / "build.py", ROOT / "tools/client_dungeon_access/Runtime.cs"]}, managedOnly=True, apkBuilt=False, deviceTested=False)
        (out / "manifest.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps({k: report[k] for k in ["changedMethodTokens", "originalEngineMethods", "nonTargetRawCILExact", "deviceTested"]}))


if __name__ == "__main__":
    main()
