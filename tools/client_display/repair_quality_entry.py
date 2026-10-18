"""Append display attachment to the proven player-info open/quality refresh path."""
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

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.client_display.build import verify_bridge
from tools.local_asset_ready.remove_gate import exact_body, require

TARGET = "UI.QualityBar::Refresh"
GUARD = "89c7e20b6883489140853f68712a658a7a3f79e83cffe198cdc2061f78231054"
ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def verify(before, after, input_sha):
    require(sha(before) == input_sha, "Exact repair input required")
    left, right = [dnfile.dnPE(data=raw) for raw in (before, after)]
    for table in ("TypeDef", "Field", "MethodDef", "MemberRef", "AssemblyRef"):
        require(len(getattr(left.net.mdtables, table).rows) == len(getattr(right.net.mdtables, table).rows), "Metadata inventory changed: " + table)
    require(not getattr(right.net.mdtables, "FieldPtr", None), "Unexpected FieldPtr")
    unchanged = engine = changed = 0
    for (name, rid, old), (other, oid, new) in zip(methods(left), methods(right)):
        require((name, rid) == (other, oid) and old.Signature.value == new.Signature.value
                and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags,
                "Original method identity/ABI changed: " + name)
        if not old.Rva:
            require(not new.Rva, "Unexpected body"); continue
        old_raw, new_raw = exact_body(left, old), exact_body(right, new)
        if name != TARGET:
            require(old_raw == new_raw, "Unrelated body changed: " + name)
            unchanged += 1; engine += name.startswith("Battle.Engine"); continue
        require(sha(old_raw) == GUARD, "Original Refresh body guard differs")
        ob, oi = decoded(left, old); nb, ni = decoded(right, new)
        require(not ob.exception_handlers and not nb.exception_handlers, "Unexpected Refresh EH")
        require(oi[-1][0] == "ret" and ni[:-3] == oi[:-1], "Original Refresh branches changed")
        require([x[0] for x in ni[-3:]] == ["ldarg.0", "call", "ret"], "Unexpected attach suffix")
        call = nb.instructions[-2].operand
        ref = right.net.mdtables.MemberRef.rows[(call.value & 0xffffff) - 1]
        require(str(ref.Name) == "Attach" and str(ref.Class.row.TypeNamespace) == "HYJS.Display"
                and str(ref.Class.row.TypeName) == "DisplaySettings", "Wrong suffix target")
        changed += 1
    require(changed == 1 and engine == 547, "Repair scope/engine count differs")
    return {"onlyChangedMethod": TARGET, "originalRefreshBranchesPreserved": True,
            "originalMetadataABIAndTokensPreserved": True,
            "unchangedRawMethodBodies": unchanged, "originalEngineMethodsUnchanged": engine}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--input-managed", type=Path, required=True)
    p.add_argument("--input-managed-sha256", required=True)
    p.add_argument("--runtime-apk", type=Path, required=True)
    p.add_argument("--output-directory", type=Path, required=True)
    args = p.parse_args()
    before = args.input_managed.read_bytes()
    require(sha(before) == args.input_managed_sha256, "Input SHA differs")
    out = args.output_directory.resolve(); require(not out.exists(), "Use fresh output directory"); out.mkdir(parents=True)
    dotnet = ROOT / ".tools/native-batch/dotnet/dotnet"
    compiler = next((dotnet.parent / "sdk").glob("*/Roslyn/bincore/csc.dll"))
    dnlib = ROOT / ".tools/native-batch/injector/bin/Debug/net8.0/dnlib.dll"
    injector = Path(__file__).with_name("QualityEntryRepair.cs")
    source = Path(__file__).with_name("DisplaySettings.cs")

    def run(name, command):
        result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=ROOT,
                                capture_output=True, text=True, timeout=90)
        (out / (name + ".log")).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + " failed; inspect saved log")

    with tempfile.TemporaryDirectory(prefix="hyjs-quality-entry-") as temp:
        work = Path(temp); current = work / "Assembly-CSharp.dll"; current.write_bytes(before)
        refs = [current]
        with zipfile.ZipFile(args.runtime_apk) as z:
            for name in ("UnityEngine.dll", "mscorlib.dll", "Assembly-CSharp-firstpass.dll", "System.dll", "System.Core.dll"):
                f = work / name; f.write_bytes(z.read("assets/bin/Data/Managed/" + name)); refs.append(f)
        run("compile-release-bridge", [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+",
                                       "/out:" + str(out / "SoulFlowerDisplay.dll"), *["/reference:" + str(f) for f in refs], source])
        shutil.copy2(injector, work / "Program.cs")
        project = work / "Patch.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run("compile-repair", [dotnet, "build", project, "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        run("repair", [dotnet, work / "bin/Debug/net8.0/Patch.dll", current, out / "Assembly-CSharp.dll"])
    after = (out / "Assembly-CSharp.dll").read_bytes()
    report = {"status": "PASS_STATIC_ENTRY_REPAIR_NOT_PACKAGED", "deviceExecuted": False,
              "inputManagedSHA256": sha(before), "outputManagedSHA256": sha(after),
              "replacementEntries": [ENTRY, "assets/bin/Data/Managed/SoulFlowerDisplay.dll"],
              "evidence": "Phone explicit Attach displayed frame controls; UI.PlayerInfoPage.DoOpen calls QualityBar.Refresh. Start-only entry did not display them.",
              "noDiagnosticAndroidLogs": True,
              "sourcePins": {str(f.relative_to(ROOT)): sha(f.read_bytes()) for f in (Path(__file__).resolve(), injector, source)},
              **verify(before, after, args.input_managed_sha256), **verify_bridge((out / "SoulFlowerDisplay.dll").read_bytes())}
    (out / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    main()
