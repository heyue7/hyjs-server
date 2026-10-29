"""Build a managed-only trial refresh overlay from the exact shipped 80 APK."""
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
from tools.awake_diff_rows.build import methods
from tools.ggo_items.build_client_overlay import semantic
from tools.local_asset_ready.remove_gate import exact_body, require

BASE = "25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a"
MAIN_SHA = "8b732cce869cad9a9a06c75d3ba4891b4084a3ac646848fc99a933148e300437"
MANAGED = "assets/bin/Data/Managed/"
TARGETS = {
    "UI.TabPointShop::UpdateAll": "HandleUpdateAll",
    "UI.TabPointShop::UpdateProducts": "HandleProducts",
}
SOURCES = [ROOT / "tools/ggo_items/client" / name for name in (
    "GGOShop.cs", "GGOIconManifest.cs", "PetIconManifest75.cs", "TrialShopRefresh75.cs", "PetPortraitAssets75.cs")]


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def sha(value):
    return hashlib.sha256(value).hexdigest()


def check_main(before, after):
    old, new = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    left, right = methods(old), methods(new)
    require(len(left) == len(right), "method inventory changed")
    for table in ("TypeDef", "Field", "MethodDef", "Param"):
        require(len(getattr(old.net.mdtables, table).rows) == len(getattr(new.net.mdtables, table).rows), "metadata count changed: " + table)
    require(not getattr(new.net.mdtables, "FieldPtr", None) and not getattr(new.net.mdtables, "MethodPtr", None), "Unity pointer table added")
    changed, engine = [], 0
    for (name, rid, a), (other, rid2, b) in zip(left, right):
        require((name, rid, a.Signature.value, a.struct.Flags, a.struct.ImplFlags) == (other, rid2, b.Signature.value, b.struct.Flags, b.struct.ImplFlags), "method ABI changed: " + name)
        if not a.Rva:
            require(not b.Rva, "body unexpectedly added: " + name)
            continue
        first, second = exact_body(old, a), exact_body(new, b)
        if name in TARGETS:
            original, regions = semantic(old, a)
            updated, updated_regions = semantic(new, b, 4)
            require(original == updated[4:] and regions == updated_regions, "inherited code changed: " + name)
            require([op for op, _ in updated[:4]] == ["ldarg.0", "call", "brfalse", "ret"], "unexpected hook: " + name)
            token = updated[1][1]
            ref = new.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1]
            require(str(ref.Name) == TARGETS[name] and str(ref.Class.row.TypeName) == "TrialShopRefresh75", "wrong hook target")
            changed.append({"method": name, "beforeSHA256": sha(first), "afterSHA256": sha(second), "inheritedCILPreserved": True})
        else:
            require(first == second, "unrelated body changed: " + name)
        if name.startswith("Battle.Engine"):
            require(first == second, "battle body changed")
            engine += 1
    require({x["method"] for x in changed} == set(TARGETS) and engine == 547, "target or battle inventory mismatch")
    for table, attrs in (("AssemblyRef", ("Name", "Flags", "PublicKey")), ("MemberRef", ("Name", "Signature")), ("TypeRef", ("TypeName", "TypeNamespace"))):
        before_rows, after_rows = getattr(old.net.mdtables, table).rows, getattr(new.net.mdtables, table).rows
        require(len(after_rows) >= len(before_rows), "reference rows removed: " + table)
        for a, b in zip(before_rows, after_rows):
            for attr in attrs:
                x, y = getattr(a, attr), getattr(b, attr)
                require((x.value if hasattr(x, "value") else str(x)) == (y.value if hasattr(y, "value") else str(y)), "reference changed: " + table + "." + attr)
    return {"allowedChangedMethods": changed, "originalEngineMethodsExact": engine, "allOtherOriginalBodiesByteExact": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", required=True, type=Path)
    parser.add_argument("--output-directory", required=True, type=Path)
    args = parser.parse_args()
    apk, out = args.base_apk.resolve(), args.output_directory.resolve()
    require(digest(apk) == BASE, "input 80 APK SHA mismatch")
    require(out.is_relative_to(ROOT / "tools/client_trial_refresh82") and not out.exists(), "fresh task-owned output required")
    out.mkdir(parents=True)
    runtime = Path("/Users/lirui/LR/hyjs/.tools/native-batch")
    dot = runtime / "dotnet/dotnet"
    compiler = next((runtime / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"

    def run(label, command):
        result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=180)
        (out / (label + ".log")).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, label + " failed; inspect " + str(out / (label + ".log")))

    with tempfile.TemporaryDirectory(prefix="hyjs-trial82-") as temp, zipfile.ZipFile(apk) as archive:
        work = Path(temp)
        refs = work / "refs"; refs.mkdir()
        for name in archive.namelist():
            if name.startswith(MANAGED) and name.endswith(".dll"):
                (refs / Path(name).name).write_bytes(archive.read(name))
        before = (refs / "Assembly-CSharp.dll").read_bytes()
        require(sha(before) == MAIN_SHA, "input main assembly mismatch")
        helper = out / "SoulFlowerGGOShop.dll"
        run("compile-helper", [dot, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+", "/codepage:65001", "/out:" + str(helper),
            "/resource:" + str(ROOT / "tools/ggo_items/assets/ggo_credit_hud-source.png") + ",credit_hud.png",
            *["/r:" + str(p) for p in sorted(refs.glob("*.dll")) if p.name != helper.name], *SOURCES])
        verify = work / "verify"; verify.mkdir()
        (verify / "Program.cs").write_text((ROOT / "tools/client_trial_refresh82/Verify.cs").read_text())
        (verify / "Verify.csproj").write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run("compile-verifier", [dot, "build", verify / "Verify.csproj", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        run("verify-helper-api", [dot, verify / "bin/Debug/net8.0/Verify.dll", refs / helper.name, helper])
        project = work / "inject"; project.mkdir()
        (project / "Program.cs").write_text((ROOT / "tools/client_trial_refresh82/Injector.cs").read_text())
        (project / "Inject.csproj").write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run("compile-injector", [dot, "build", project / "Inject.csproj", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        main_path = out / "Assembly-CSharp.dll"
        run("inject", [dot, project / "bin/Debug/net8.0/Inject.dll", refs / "Assembly-CSharp.dll", main_path])
        proof = check_main(before, main_path.read_bytes())
        payload = {MANAGED + p.name: str(p.relative_to(ROOT)) for p in (helper, main_path)}
        source_paths = SOURCES + [Path(__file__).resolve(), ROOT / "tools/client_trial_refresh82/Injector.cs", ROOT / "tools/client_trial_refresh82/Verify.cs", ROOT / "tools/ggo_items/assets/ggo_credit_hud-source.png"]
        manifest = {"schema": "hyjs-trial-refresh82-overlay-v1", "status": "STATIC_VERIFIED_NOT_PACKAGED", "baseAPKSHA256": BASE,
            "inputManagedSHA256": MAIN_SHA, "managedOnly": True, "deviceTested": False, "inheritedHelperPublicMethodsCompatible": True,
            "payloadFiles": payload, "replacementEntries": {name: digest(ROOT / path) for name, path in payload.items()},
            "sourcePins": {str(p.relative_to(ROOT)): digest(p) for p in source_paths}, **proof}
        (out / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    require(digest(apk) == BASE, "input APK changed during build")
    print(json.dumps({"status": manifest["status"], "payload": manifest["replacementEntries"], "originalEngineMethodsExact": 547}, ensure_ascii=False))


if __name__ == "__main__":
    main()
