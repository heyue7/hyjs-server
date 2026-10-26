"""Three exact portrait-key loader compatibility over the frozen production-origin 75 APK."""
import argparse, hashlib, json, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path
import dnfile, UnityPy
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require
from tools.ggo_items.build_client_overlay import semantic

BASE = "9313e6d182b65dc52f375375c3baf88b153a799294f72324297b8df1d3087252"
MANAGED = "753e8db67c2d3e6a58ccf5c8e5e1fdb292aba5b13304c24deb121916c665c0e2"
MAIN = "assets/bin/Data/Managed/Assembly-CSharp.dll"
HELPER = "assets/bin/Data/Managed/SoulFlowerGGOShop.dll"
PREFIX = "AssetLoad.AssetBundleManager::"
TARGETS = {PREFIX + "IsAssetExist": 2, PREFIX + "GetFullPath": 7, PREFIX + "LoadAssetBundle": 0}
SOURCE = ROOT / "tools/ggo_items/client"
def sha(value): return hashlib.sha256(value).hexdigest()

def verify(before, after):
    a, b = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    old, new = methods(a), methods(b)
    require(len(old) == len(new), "Original method inventory differs")
    for table in ("TypeDef", "Field", "MethodDef", "Param"):
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows), "Native ABI count changed: " + table)
    for x, y in zip(a.net.mdtables.TypeDef.rows, b.net.mdtables.TypeDef.rows):
        require((str(x.TypeName), str(x.TypeNamespace), x.struct.Flags, x.Extends.row_index) == (str(y.TypeName), str(y.TypeNamespace), y.struct.Flags, y.Extends.row_index), "Native type ABI differs")
        require([f.row_index for f in x.FieldList] == [f.row_index for f in y.FieldList] and [m.row_index for m in x.MethodList] == [m.row_index for m in y.MethodList], "Native type member tokens differ")
    for x, y in zip(a.net.mdtables.Field.rows, b.net.mdtables.Field.rows):
        require((str(x.Name), x.Signature.value, x.struct.Flags) == (str(y.Name), y.Signature.value, y.struct.Flags), "Native field ABI differs")
    changes, engine, unchanged = [], 0, 0
    for (name, rid, x), (name2, rid2, y) in zip(old, new):
        require((name, rid, x.Signature.value, x.struct.Flags, x.struct.ImplFlags) == (name2, rid2, y.Signature.value, y.struct.Flags, y.struct.ImplFlags), "Native method ABI/token differs " + name)
        require(bool(x.Rva) == bool(y.Rva), "Native body presence differs")
        if not x.Rva: continue
        left, right = exact_body(a, x), exact_body(b, y)
        if name not in TARGETS:
            require(left == right, "Unrelated raw method body changed " + name)
            unchanged += 1
        else:
            count = TARGETS[name]
            original, regions = semantic(a, x)
            updated, new_regions = semantic(b, y, count)
            require(regions == new_regions, "Native exception regions differ " + name)
            if count:
                require(original == updated[count:], "Inherited CIL changed " + name)
                expected = ["ldarg.0", "call"] if count == 2 else ["ldarg.1", "ldarg.2", "ldloca", "call", "brfalse", "ldloc", "ret"]
                require([op for op, value in updated[:count]] == expected, "Unexpected prefix " + name)
                calls = [value for op, value in updated[:count] if op == "call"]
                require(len(calls) == 1, "Unexpected prefix calls")
                ref = b.net.mdtables.MemberRef.rows[(calls[0] & 0xffffff) - 1]
                require(str(ref.Name) == ("Ensure" if count == 2 else "TryPath") and str(ref.Class.row.TypeName) == "PetPortraitAssets75", "Unexpected helper call")
            else:
                differences = [(index, l, r) for index, (l, r) in enumerate(zip(original, updated)) if l != r]
                require(len(original) == len(updated) and len(differences) == 1, "Only dependency call may change")
                index, left_call, right_call = differences[0]
                require(left_call[0] == "callvirt" and right_call[0] == "call", "Dependency invocation shape differs")
                old_ref = a.net.mdtables.MemberRef.rows[(left_call[1] & 0xffffff) - 1]
                new_ref = b.net.mdtables.MemberRef.rows[(right_call[1] & 0xffffff) - 1]
                require(str(old_ref.Name) == "GetAllDependencies" and str(new_ref.Name) == "Dependencies" and str(new_ref.Class.row.TypeName) == "PetPortraitAssets75", "Wrong dependency wrapper")
            changes.append(dict(method=name, token=hex(0x6000000 | rid), beforeSHA256=sha(left), afterSHA256=sha(right), inheritedCILPreserved=True))
        if name.startswith("Battle.Engine"):
            require(left == right, "Native battle changed")
            engine += 1
    require(engine == 547 and {c["method"] for c in changes} == set(TARGETS), "Expected three scoped methods and Engine547")
    require(not getattr(b.net.mdtables, "FieldPtr", None), "Native FieldPtr not supported")
    # Preserve all original reference rows; new helper refs append only.
    for table, attrs in (("AssemblyRef", ("Name",)), ("MemberRef", ("Name", "Signature")), ("TypeRef", ("TypeName", "TypeNamespace"))):
        aa, bb = getattr(a.net.mdtables, table).rows, getattr(b.net.mdtables, table).rows
        require(len(bb) >= len(aa), "Original reference rows removed")
        for x, y in zip(aa, bb):
            for attr in attrs:
                xx, yy = getattr(x, attr), getattr(y, attr)
                require((xx.value if hasattr(xx, "value") else str(xx)) == (yy.value if hasattr(yy, "value") else str(yy)), "Original reference changed " + table + "." + attr)
    utc = [(name, sha(exact_body(a, x))) for name, _, x in old if x.Rva and name.endswith("::WebPB2Bytes")]
    require(len(utc) == 1, "Expected unique original HTTP serialization body")
    return dict(allowedChangedMethods=changes, originalEngineMethodsExact=engine, unchangedOriginalRawBodies=unchanged,
                originalMetadataABIAndTokensPreserved=True, allOtherOriginalBodiesByteExact=True,
                existingDungeonManifestCompatibilityExact=True, inheritedHTTPUTCBodyExact=utc[0])

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--base-apk", type=Path, required=True)
    p.add_argument("--output-directory", type=Path, required=True)
    args = p.parse_args()
    out = args.output_directory.resolve()
    require(out.is_relative_to(ROOT / "analysis") and not out.exists(), "Fresh owned analysis output required")
    require(sha(args.base_apk.read_bytes()) == BASE, "Frozen production-origin APK differs")
    out.mkdir(parents=True)
    tool = Path("/Users/lirui/LR/hyjs/.tools/native-batch")
    dotnet = tool / "dotnet/dotnet"
    csc = next((tool / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    dnlib = tool / "injector/bin/Debug/net8.0/dnlib.dll"
    def run(label, command):
        r = subprocess.run(["rtk", "proxy", *map(str, command)], capture_output=True, text=True, timeout=180)
        (out / (label + ".log")).write_text(r.stdout + r.stderr)
        require(r.returncode == 0, label + " failed; see log")
    with tempfile.TemporaryDirectory(prefix="hyjs-pet-cold75-") as temporary, zipfile.ZipFile(args.base_apk) as z:
        work = Path(temporary)
        refs = work / "refs"; refs.mkdir()
        for name in z.namelist():
            if name.startswith("assets/bin/Data/Managed/") and name.endswith(".dll"):
                (refs / Path(name).name).write_bytes(z.read(name))
        before = (refs / "Assembly-CSharp.dll").read_bytes()
        require(sha(before) == MANAGED, "Frozen main (including UTC source patch) differs")
        native_index = UnityPy.load(z.read("assets/android/android"))
        native_tree = next(o.read_typetree() for o in native_index.objects if o.type.name == "AssetBundleManifest")
        native_names, native_infos = dict(native_tree["AssetBundleNames"]), dict(native_tree["AssetBundleInfos"])
        text_index = {line.split("\t")[0]: line.split("\t")[1:] for line in z.read("assets/android/smallassetmanifest").decode().splitlines()[1:] if line}
        portraits = []
        for suffix in ("076", "077", "078"):
            name = "pet/pet_icon_" + suffix
            raw = z.read("assets/android/" + name)
            info = native_infos[next(index for index, key in native_names.items() if key == name)]
            require(info["AssetBundleDependencies"] == [], "Portrait native dependencies are not empty")
            digest = hashlib.md5(raw).hexdigest()
            native_digest = bytes(info["AssetBundleHash"]["bytes[" + str(i) + "]"] for i in range(16)).hex()
            require(text_index[name] == [digest, str(len(raw))] and native_digest == digest, "Original-model portrait hash/size differs")
            portrait_bundle = UnityPy.load(raw)
            require(all(obj.assets_file.externals == [] for obj in portrait_bundle.objects), "Portrait serialized external dependency found")
            require(any(obj.type.name == "Texture2D" and obj.read().m_Name == "pet_icon_" + suffix for obj in portrait_bundle.objects), "Portrait native texture name differs")
            portraits.append(dict(key=name, MD5=digest, size=len(raw), bundleDependencies=[], serializedExternalReferences=[], nativeTexture="pet_icon_" + suffix))
        helper = out / "SoulFlowerGGOShop.dll"
        sources = [SOURCE / name for name in ("GGOShop.cs", "GGOIconManifest.cs", "PetIconManifest75.cs", "TrialShopRefresh75.cs", "PetPortraitAssets75.cs")]
        run("compile-helper", [dotnet, csc, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+", "/out:" + str(helper),
            "/resource:" + str(ROOT / "tools/ggo_items/assets/ggo_credit_hud-source.png") + ",credit_hud.png",
            *["/r:" + str(r) for r in sorted(refs.glob("*.dll")) if r.name != "SoulFlowerGGOShop.dll"], *sources])
        # Compile and execute the exact new helper source against explicit native-boundary doubles.
        tests = work / "tests"; tests.mkdir()
        for name in ("PetPortraitAssets75.cs", "PetPortraitAssets75Tests.cs", "PetIconManifest75.cs"):
            shutil.copy2(SOURCE / name, tests / name)
        (tests / "Tests.csproj").write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>disable</ImplicitUsings><Nullable>disable</Nullable><LangVersion>3</LangVersion></PropertyGroup></Project>')
        run("boundary-tests", [dotnet, "run", "--project", tests / "Tests.csproj", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        patch = work / "patch"; patch.mkdir()
        shutil.copy2(SOURCE / "PetPortraitInjector75.cs", patch / "Program.cs")
        (patch / "Patch.csproj").write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run("compile-injector", [dotnet, "build", patch / "Patch.csproj", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        main_file = out / "Assembly-CSharp.dll"
        run("inject", [dotnet, patch / "bin/Debug/net8.0/Patch.dll", refs / "Assembly-CSharp.dll", main_file])
        after = main_file.read_bytes()
        proof = verify(before, after)
        # Helper inherited public/protected type/member signatures remain callable.
        old_helper, new_helper = dnfile.dnPE(data=z.read(HELPER)), dnfile.dnPE(data=helper.read_bytes())
        old_methods = {(n, bytes(m.Signature.value), m.struct.Flags) for n, _, m in methods(old_helper) if m.struct.Flags & 7 in (4, 5, 6)}
        new_methods = {(n, bytes(m.Signature.value), m.struct.Flags) for n, _, m in methods(new_helper) if m.struct.Flags & 7 in (4, 5, 6)}
        require(old_methods <= new_methods, "Inherited helper public/protected API differs")
        inventory = {name: sha(z.read(name)) for name in z.namelist() if not name.startswith("META-INF/")}
        (out / "inherited-entry-sha256.json").write_text(json.dumps(inventory, indent=2) + "\n")
    pins = sources + [SOURCE / "PetPortraitInjector75.cs", SOURCE / "PetPortraitAssets75Tests.cs", Path(__file__), ROOT / "tools/ggo_items/assets/ggo_credit_hud-source.png"]
    payload = {MAIN: str(main_file.relative_to(ROOT)), HELPER: str(helper.relative_to(ROOT))}
    report = dict(schema="hyjs-growth75-pet-cold-overlay-v1", status="STATIC_VERIFIED_NOT_PACKAGED", baseAPKSHA256=BASE,
        inputManagedSHA256=MANAGED, outputManagedSHA256=sha(after), payloadFiles=payload,
        replacementEntries={key: sha((ROOT / path).read_bytes()) for key, path in payload.items()},
        sourcePins={str(path.relative_to(ROOT)): sha(path.read_bytes()) for path in pins}, inheritedJavaByteExact=True,
        inheritedJavaPublicProtectedABIExact=True, allAssetsByteExact=True, userCacheFilesUntouched=True,
        manifestVersionUnchanged=True, inheritedHelperPublicProtectedAPICompatible=True, exactPortraitBundleProof=portraits, deviceExecuted=False, **proof)
    (out / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(dict(status=report["status"], outputManagedSHA256=sha(after), originalEngineMethodsExact=547, payloadEntries=2)))
if __name__ == "__main__": main()
