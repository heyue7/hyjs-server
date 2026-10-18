"""Build a single-method weapon-remodel UI refresh overlay; never package/install."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require

BASE = "34ce7e0d4eacdcaa7298123215cf02fd21ac347a0601207d0eb2c52b7599c10e"
MAIN = "5b6ba3af357c36f7e81d7ad82f4ba31157eda6d84dc6ec8f18a1314a3604c8f4"
ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
TARGET = "UI.EquipRemodelPage::DoClose"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def verify(before, after):
    left, right = [dnfile.dnPE(data=raw) for raw in (before, after)]
    old, new = methods(left), methods(right)
    require(len(old) == len(new), "Method inventory changed")
    changed, unchanged, engine = [], 0, 0
    for table in ["TypeDef", "Field", "FieldPtr", "MethodPtr"]:
        a, b = getattr(left.net.mdtables, table, None), getattr(right.net.mdtables, table, None)
        require((len(a.rows) if a else 0) == (len(b.rows) if b else 0), "Managed inventory changed: " + table)
    require(not getattr(right.net.mdtables, "FieldPtr", None), "Old Unity must not gain FieldPtr")
    for (name, rid, a), (other, index, b) in zip(old, new):
        require((name, rid) == (other, index) and a.Signature.value == b.Signature.value and a.struct.Flags == b.struct.Flags and a.struct.ImplFlags == b.struct.ImplFlags, "Original ABI changed: " + name)
        if not a.Rva:
            require(not b.Rva, "Unexpected new body")
            continue
        raw, updated = exact_body(left, a), exact_body(right, b)
        if name != TARGET:
            require(raw == updated, "Unrelated method changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            continue
        original, result = [read_method_body_from_bytes(x) for x in (raw, updated)]
        require([x.opcode.name for x in original.instructions] == ["ret"], "Original close body differs")
        require([x.opcode.name for x in result.instructions] == ["call", "ret"], "Unexpected close patch")
        token = result.instructions[0].operand.value
        ref = right.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1]
        owner = ref.Class.row
        require(str(owner.TypeNamespace) == "HYJS.WeaponRefresh" and str(owner.TypeName) == "WeaponRefresh" and str(ref.Name) == "AfterClose" and ref.Signature.value == bytes.fromhex("000001"), "Unexpected helper ABI")
        require(str(owner.ResolutionScope.row.Name) == "SoulFlowerWeaponRefresh", "Unexpected helper assembly")
        changed.append({"method": name, "beforeRawSHA256": sha(raw), "afterRawSHA256": sha(updated)})
    require(engine == 547 and len(changed) == 1, "Changed target/engine inventory differs")
    return {"changes": changed, "unchangedInheritedRawMethodBodies": unchanged, "originalEngineMethodsUnchanged": engine, "allInheritedNonTargetMethodsExact": True, "noManagedTypesMethodsFieldsAdded": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, default=ROOT / "apks/build/hyjs-local.apk")
    parser.add_argument("--input-managed", type=Path)
    parser.add_argument("--input-managed-sha256")
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    digest = hashlib.sha256()
    with args.base_apk.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    require(digest.hexdigest() == BASE, "Pinned current APK required")
    output = args.output_directory.resolve()
    require(not output.exists(), "Fresh overlay directory required")
    output.mkdir(parents=True)
    runtime = ROOT / ".tools/native-batch"
    dotnet = runtime / "dotnet/dotnet"
    compiler = next((runtime / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"
    source, injector = [Path(__file__).with_name(name) for name in ("WeaponRefresh.cs", "Injector.cs")]

    def run(name, command):
        result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=90)
        (output / (name + ".log")).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + " failed; inspect saved log")

    with tempfile.TemporaryDirectory(prefix="hyjs-weapon-refresh-") as temporary:
        folder = Path(temporary)
        with zipfile.ZipFile(args.base_apk) as apk:
            before = apk.read(ENTRY)
            require(sha(before) == MAIN, "Current managed input differs")
            for name in ("Assembly-CSharp", "UnityEngine", "mscorlib"):
                (folder / (name + ".dll")).write_bytes(apk.read("assets/bin/Data/Managed/" + name + ".dll"))
        if args.input_managed:
            require(args.input_managed_sha256 is not None and len(args.input_managed_sha256) == 64, "Composition requires explicit managed SHA")
            before = args.input_managed.read_bytes()
            require(sha(before) == args.input_managed_sha256, "Composition input differs")
            (folder / "Assembly-CSharp.dll").write_bytes(before)
        else:
            require(args.input_managed_sha256 is None, "Input SHA requires managed input")
        bridge = output / "SoulFlowerWeaponRefresh.dll"
        run("compile-bridge", [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+", "/out:" + str(bridge), *["/reference:" + str(folder / (name + ".dll")) for name in ("mscorlib", "UnityEngine", "Assembly-CSharp")], source])
        shutil.copy2(injector, folder / "Program.cs")
        project = folder / "Patch.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run("compile-injector", [dotnet, "build", project, "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        run("patch", [dotnet, folder / "bin/Debug/net8.0/Patch.dll", folder / "Assembly-CSharp.dll", bridge, output / "Assembly-CSharp.dll"])
    after = (output / "Assembly-CSharp.dll").read_bytes()
    proof = verify(before, after)
    bridge_pe = dnfile.dnPE(data=bridge.read_bytes())
    references = sorted(str(x.Name) for x in bridge_pe.net.mdtables.AssemblyRef)
    require(references == ["Assembly-CSharp", "UnityEngine", "mscorlib"], "Unexpected bridge dependency")
    require(bytes(bridge_pe.net.metadata.struct.Version).startswith(b"v2.0.50727"), "Old Unity runtime differs")
    report = {"status": "STATIC_VERIFIED_NOT_PACKAGED", "baseAPKSHA256": BASE, "inputManagedSHA256": sha(before), "outputManagedSHA256": sha(after), "bridgeSHA256": sha(bridge.read_bytes()), "sourcePins": {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in (Path(__file__).resolve(), source, injector)}, "replacementEntries": [ENTRY, "assets/bin/Data/Managed/SoulFlowerWeaponRefresh.dll"], "bridgeReferences": references, "phoneExecuted": False, "serverMutated": False, **proof}
    (output / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": report["status"], "outputManagedSHA256": report["outputManagedSHA256"], "bridgeSHA256": report["bridgeSHA256"]}))


if __name__ == "__main__":
    main()
