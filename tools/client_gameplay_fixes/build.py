"""Build the 1.1.9 gameplay managed overlay; no APK packaging or device work."""
import hashlib
import json
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

sys.dont_write_bytecode = True
import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.local_asset_ready.remove_gate import exact_body, require

APK = Path("/Users/lirui/LR/hyjs/deliveries/client-update-1.1.9/黑衣剑士灵魂之花v1.1.9.apk")
APK_SHA = "cd86472be5572ec63fa22eb76a12ab2589f775dc8483e76350ed01fe7aa4a118"
MAIN_SHA = "e399d027eecbcebd67914c8da2df5ac6eedca15445dd0803715ffb253de8c451"
MANAGED = "assets/bin/Data/Managed/"
REFS = ROOT / "build/gameplay-five-fixes-20261103/client/refs"
OUT = ROOT / "build/gameplay-five-fixes-20261103/overlay"
RUNTIME = Path("/Users/lirui/LR/hyjs/.tools/native-batch")
HERE = Path(__file__).resolve().parent
GGO = ROOT / "tools/ggo_items/client"
GGO_SOURCES = [GGO / name for name in (
    "GGOShop.cs", "GGOIconManifest.cs", "PetIconManifest75.cs",
    "TrialShopRefresh75.cs", "PetPortraitAssets75.cs")]
TRIAL = ROOT / "tools/client_feedback66/TrialRuntime.cs"
VERIFY = ROOT / "tools/client_trial_refresh82/Verify.cs"
RESOURCE = ROOT / "tools/ggo_items/assets/ggo_credit_hud-source.png"
SOURCES = [Path(__file__).resolve(), HERE / "Injector.cs", HERE / "GuildRewardFeedback.cs",
           HERE / "TransportBusy.cs", TRIAL, VERIFY, RESOURCE] + GGO_SOURCES
TARGETS = {
    0x3dd4: "NetWork.Layer.HTTPManager::Send",
    0x3df6: "NetAPI::IsSendingHttp",
    0x561c: "UI.UnionWarehouseTabBox::OpenBoxCallHandle",
}

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()

def identity(row):
    return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion,
            row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)

def reference(pe, token, owner, name, signature):
    require(token >> 24 == 10, "Expected helper MemberRef")
    ref = pe.net.mdtables.MemberRef.rows[(token & 0xffffff)-1]
    typ = ref.Class.row
    require(str(ref.Name) == name and str(typ.TypeNamespace) == "HYJS.GameplayFixes" and
            str(typ.TypeName) == owner and typ.ResolutionScope.table.name == "AssemblyRef" and
            str(typ.ResolutionScope.row.Name) == "HYJS.GameplayFixes" and
            bytes(ref.Signature.value) == signature, "Helper reference scope/signature mismatch: " + name)

def compare_original(old, new, mapping, extras=None, replaced=None):
    extras = extras or {}
    replaced = replaced or set()
    back = {v: k for k, v in mapping.items()}
    back.update(extras)
    require(len(mapping) == len(old), "Missing inherited instruction")
    for oi, ni in mapping.items():
        if oi in replaced:
            continue
        op, arg = old[oi]
        nop, narg = new[ni]
        if op == "switch":
            narg = tuple(back[v] for v in narg)
        elif op.startswith(("br", "beq", "bne", "bge", "bgt", "ble", "blt", "leave")):
            require(narg in back, "Original branch enters unknown inserted code")
            narg = back[narg]
        require((op, arg) == (nop, narg), "Original instruction or branch changed at " + str(oi))

def check_main(before, after):
    a, b = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    for name in ("TypeDef", "Field", "MethodDef", "Param"):
        require(len(getattr(a.net.mdtables, name).rows) == len(getattr(b.net.mdtables, name).rows),
                "Definition count changed: " + name)
    require(not getattr(b.net.mdtables, "FieldPtr", None) and not getattr(b.net.mdtables, "MethodPtr", None),
            "Unexpected Unity pointer table")
    require(a.net.mdtables.Module.rows[0].Mvid.value == b.net.mdtables.Module.rows[0].Mvid.value,
            "Module MVID changed")
    require([identity(r) for r in a.net.mdtables.Assembly.rows] ==
            [identity(r) for r in b.net.mdtables.Assembly.rows], "Assembly identity changed")
    ar, br = a.net.mdtables.AssemblyRef.rows, b.net.mdtables.AssemblyRef.rows
    require([identity(r) for r in ar] == [identity(r) for r in br[:len(ar)]] and
            len(br) == len(ar) + 1 and str(br[-1].Name) == "HYJS.GameplayFixes" and
            br[-1].struct.Flags == 0 and br[-1].PublicKey.value == b"", "AssemblyRef drift")
    require(a.net.struct.Flags == b.net.struct.Flags and
            a.net.struct.StrongNameSignatureSize == b.net.struct.StrongNameSignatureSize == 0,
            "CLI identity changed")
    for x, y in zip(a.net.mdtables.Field.rows, b.net.mdtables.Field.rows):
        require((str(x.Name), x.Signature.value, x.struct.Flags) ==
                (str(y.Name), y.Signature.value, y.struct.Flags), "Field ABI changed")
    for x, y in zip(a.net.mdtables.TypeDef.rows, b.net.mdtables.TypeDef.rows):
        require((str(x.TypeName), str(x.TypeNamespace), x.struct.Flags, x.struct.Extends_CodedIndex,
                 x.struct.FieldList_Index, x.struct.MethodList_Index) ==
                (str(y.TypeName), str(y.TypeNamespace), y.struct.Flags, y.struct.Extends_CodedIndex,
                 y.struct.FieldList_Index, y.struct.MethodList_Index), "Type ABI changed")
    for table, attrs, scope in (("TypeRef", ("TypeName", "TypeNamespace"), "ResolutionScope"),
                                ("MemberRef", ("Name", "Signature"), "Class")):
        old, new = getattr(a.net.mdtables, table).rows, getattr(b.net.mdtables, table).rows
        require(len(new) >= len(old), "Original references removed: " + table)
        for x, y in zip(old, new):
            for attr in attrs:
                v, w = getattr(x, attr), getattr(y, attr)
                require((v.value if hasattr(v, "value") else str(v)) ==
                        (w.value if hasattr(w, "value") else str(w)), "Original reference changed: " + table)
            v, w = getattr(x, scope), getattr(y, scope)
            require((v.table.name, v.row_index) == (w.table.name, w.row_index),
                    "Original reference scope changed: " + table)

    changed, unchanged, engine = [], 0, 0
    left, right = methods(a), methods(b)
    require(len(left) == len(right), "Method inventory changed")
    for (name, rid, old), (newname, newrid, new) in zip(left, right):
        require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) ==
                (newname, newrid, new.Signature.value, new.struct.Flags, new.struct.ImplFlags),
                "Method ABI/token changed: " + name)
        if not old.Rva:
            require(not new.Rva, "Original method gained body: " + name)
            continue
        x, y = exact_body(a, old), exact_body(b, new)
        if rid not in TARGETS:
            require(x == y, "Unrelated original body changed: " + name)
            unchanged += 1
            if name.startswith("Battle.Engine"):
                engine += 1
            continue
        require(name == TARGETS[rid] and x != y, "Wrong target method: " + name)
        ob, oc = decoded(a, old)
        nb, nc = decoded(b, new)
        require(not ob.exception_handlers and not nb.exception_handlers, "Target exception region changed: " + name)
        if rid == 0x3dd4:
            require(len(nc) == len(oc) + 1 and oc[0][0] == "ldarg.0" and
                    oc[1] == ("ldfld", 0x040037d6) and nc[1][0] == "call",
                    "HTTP admission drift")
            reference(b, nc[1][1], "TransportBusy", "IsSending", bytes.fromhex("0001021c"))
            failure = len(oc) - 2
            require(oc[failure:] == [("ldc.i4.0", None), ("ret", None)] and
                    nc[failure][0] == "call" and nc[failure+1:] == oc[failure:],
                    "HTTP post-mask failure cleanup drift")
            require(nc[failure][1] == 0x06003e04, "Wrong network cleanup target")
            mapping = {i: i if i < failure else i+1 for i in range(len(oc))}
            compare_original(oc, nc, mapping, {failure: failure}, {1})
        elif rid == 0x3df6:
            require(len(nc) == len(oc) == 3 and oc[0][0] == "ldsfld" and
                    oc[1] == ("callvirt", 0x06003ddd) and nc[1][0] == "call",
                    "NetAPI status gate drift")
            reference(b, nc[1][1], "TransportBusy", "IsSending", bytes.fromhex("0001021c"))
            compare_original(oc, nc, {i: i for i in range(len(oc))}, replaced={1})
        else:
            calls = [i for i, row in enumerate(oc) if row == ("callvirt", 0x06003afd)]
            require(len(calls) == 1 and len(nc) == len(oc)+3, "Guild callback insertion drift")
            at = calls[0]+1
            require([op for op, _ in nc[at:at+3]] == ["ldarg.1", "ldarg.2", "call"],
                    "Guild feedback instructions drift")
            reference(b, nc[at+2][1], "GuildRewardFeedback", "ShowGuildReward",
                      bytes([0]) + bytes(old.Signature.value)[1:])
            mapping = {i: i if i < at else i+3 for i in range(len(oc))}
            compare_original(oc, nc, mapping)
        changed.append({"method": name, "token": hex(0x06000000 | rid),
                        "inputBodySHA256": sha(x), "outputBodySHA256": sha(y)})
    require({r["method"] for r in changed} == set(TARGETS.values()) and engine == 547,
            "Target or engine inventory mismatch")
    return {"allowedChangedMethods": [item["method"] for item in changed], "methodChanges": changed,
            "originalEngineMethodsExact": engine,
            "allOtherOriginalBodiesByteExact": True, "unchangedOriginalBodies": unchanged,
            "originalDefinitionsTokensAndReferencesPreserved": True, "helperMemberRefBindingsVerified": True}

def main():
    require(digest(APK) == APK_SHA, "Input APK SHA differs")
    require(REFS.is_dir() and not OUT.exists(), "Existing refs and fresh output required")
    pins = {str(p.relative_to(ROOT)): digest(p) for p in SOURCES}
    with zipfile.ZipFile(APK) as archive:
        names = [name for name in archive.namelist() if name.startswith(MANAGED) and name.endswith(".dll")]
        require({Path(n).name for n in names} == {p.name for p in REFS.glob("*.dll")}, "Managed ref inventory differs")
        for name in names:
            require(sha(archive.read(name)) == digest(REFS / Path(name).name), "Ref differs: " + name)
        before = archive.read(MANAGED + "Assembly-CSharp.dll")
        require(sha(before) == MAIN_SHA, "Input main SHA differs")
        dex_sha = sha(archive.read("classes2.dex"))
    OUT.mkdir(parents=True)
    dot = RUNTIME / "dotnet/dotnet"
    compiler = next((RUNTIME / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    dnlib = RUNTIME / "injector/bin/Debug/net8.0/dnlib.dll"

    def run(label, command):
        result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=ROOT,
                                capture_output=True, text=True, timeout=180)
        (OUT / (label + ".log")).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, label + " failed; inspect " + str(OUT / (label + ".log")))

    with tempfile.TemporaryDirectory(prefix="hyjs-five-managed-") as temp:
        work = Path(temp)
        for output, sources in (
            ("HYJS.GameplayFixes.dll", [HERE / "GuildRewardFeedback.cs", HERE / "TransportBusy.cs"]),
            ("HYJS.Trial66.dll", [TRIAL]),
            ("SoulFlowerGGOShop.dll", GGO_SOURCES),
        ):
            args = [dot, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3",
                    "/deterministic+", "/codepage:65001", "/out:" + str(OUT / output)]
            if output == "SoulFlowerGGOShop.dll":
                args.append("/resource:" + str(RESOURCE) + ",credit_hud.png")
            args.extend("/r:" + str(p) for p in sorted(REFS.glob("*.dll")) if p.name != output)
            run("compile-" + output.removesuffix(".dll"), args + sources)

        def project(name, source):
            folder = work / name
            folder.mkdir()
            (folder / "Program.cs").write_text(source.read_text())
            path = folder / (name + ".csproj")
            path.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                            '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                            '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib">'
                            '<HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
            run("compile-" + name, [dot, "build", path, "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
            return folder / ("bin/Debug/net8.0/" + name + ".dll")

        verifier = project("Verify", VERIFY)
        for name in ("HYJS.Trial66.dll", "SoulFlowerGGOShop.dll"):
            run("verify-api-" + name.removesuffix(".dll"), [dot, verifier, REFS / name, OUT / name])
        injector = project("Inject", HERE / "Injector.cs")
        run("inject", [dot, injector, REFS / "Assembly-CSharp.dll", OUT / "HYJS.GameplayFixes.dll",
                       OUT / "Assembly-CSharp.dll"])

    proof = check_main(before, (OUT / "Assembly-CSharp.dll").read_bytes())
    require(pins == {str(p.relative_to(ROOT)): digest(p) for p in SOURCES}, "Source changed during build")
    require(digest(APK) == APK_SHA, "Input APK changed during build")
    payload = {MANAGED + name: str((OUT / name).relative_to(ROOT)) for name in (
        "Assembly-CSharp.dll", "HYJS.GameplayFixes.dll", "HYJS.Trial66.dll", "SoulFlowerGGOShop.dll")}
    report = {"schema": "hyjs-gameplay-five-fixes-managed-v1", "status": "STATIC_VERIFIED_NOT_PACKAGED",
              "baseAPKSHA256": APK_SHA, "inputManagedSHA256": MAIN_SHA, "inputDexSHA256": dex_sha,
              "managedOnly": True, "inheritedJavaByteExact": True, "inheritedJavaPublicProtectedABIExact": True,
              "deviceTested": False, "inheritedHelperPublicProtectedAPICompatible": True,
              "payloadFiles": payload, "replacementEntries": {name: digest(ROOT / p) for name, p in payload.items()},
              "sourcePins": pins, **proof}
    (OUT / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": report["status"], "payload": report["replacementEntries"],
                      "originalEngineMethodsExact": proof["originalEngineMethodsExact"]}, ensure_ascii=False))

if __name__ == "__main__":
    main()
