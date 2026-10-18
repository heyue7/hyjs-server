"""Prepare a display-only DLL overlay over the qualified remote client."""
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
from tools.awake_diff_rows.build import methods, operand
from tools.local_asset_ready.remove_gate import exact_body, require
from tools.client_branding.branding import chunks, elements, strings_from_pool

BASE = "b9825732e3aa82714483645b0b2d87431bc56a58992d32c1c0ca637e01d941af"
BASE_DLL = "979701a7ee26e016b27ca0fe1970a340270ea0ed833c694285fbb60913f65da7"
ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
UNITY = "assets/bin/Data/Managed/UnityEngine.dll"
TARGETS = {
    "UI.QualityBar::Start": "538b6f46d0e72d0d0c4f89319ee94e4e14f4589ad0526a750ba1f42517008b16",
    "UI.QualityBar::Refresh": "89c7e20b6883489140853f68712a658a7a3f79e83cffe198cdc2061f78231054",
    "UIAutoSize::Awake": "270ab339f575b7304e3dd8ef9ee24f13a657262a15c0f4d1c4af35dfe4cafbe4",
    "FPSFixer::Start": "f87673b0f0c7f2df1af811b3f06b1183eaf4272a534bf929bdf4e15b1b812353",
    "FPSFixer::Update": "1d7e78dd5d71ec5c46ee0bf7cd8909b34c2f965739b9a3d74f6af248f0ff4e15",
    "RenderManager::get_MaxFPS": "62e2878f72f0d44ebf9fecb2c25713cc33b091bd7146a3aacbe65783d465c0d9",
}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def member(pe, token):
    value = operand(token)
    table = {4: "Field", 6: "MethodDef", 10: "MemberRef"}.get(value >> 24)
    if table is None:
        return ("", "")
    row = getattr(pe.net.mdtables, table).rows[(value & 0xffffff) - 1]
    owner = row.Class.row if table == "MemberRef" else None
    return ((str(owner.TypeNamespace) + "." + str(owner.TypeName)) if owner else "", str(row.Name))


def verify_display_call_scopes(pe):
    """Bind helper-call parameters to real main-module types, not helper DLLs."""
    expected = {"get_TargetFPS": None, "StartFPS": "FPSFixer", "Attach": "UI.QualityBar"}
    found = {}

    def compressed(raw):
        if raw[0] < 0x80:
            return raw[0], 1
        if raw[0] < 0xc0:
            return ((raw[0] & 0x3f) << 8) | raw[1], 2
        return ((raw[0] & 0x1f) << 24) | (raw[1] << 16) | (raw[2] << 8) | raw[3], 4

    for ref in pe.net.mdtables.MemberRef:
        owner = ref.Class.row
        if not hasattr(owner, "TypeName") or str(owner.TypeName) != "DisplaySettings" or str(owner.TypeNamespace) != "HYJS.Display":
            continue
        name = str(ref.Name)
        require(name in expected, "Unexpected display bridge call: " + name)
        require(owner.ResolutionScope.table.name == "AssemblyRef" and str(owner.ResolutionScope.row.Name) == "SoulFlowerDisplay",
                "Display helper declaring scope is not its assembly")
        sig = ref.Signature.value
        if expected[name] is None:
            require(sig == bytes.fromhex("000008"), "Wrong saved FPS getter signature")
        else:
            require(sig[:4] == bytes.fromhex("00010112"), "Wrong display initializer signature: " + name)
            token, count = compressed(sig[4:])
            require(len(sig) == count + 4 and token & 3 == 0, "Display parameter must bind directly to main TypeDef: " + name)
            typedef = pe.net.mdtables.TypeDef.rows[(token >> 2) - 1]
            actual = ((str(typedef.TypeNamespace) + ".") if str(typedef.TypeNamespace) else "") + str(typedef.TypeName)
            require(actual == expected[name], "Display parameter bound to wrong main type: " + name)
        found[name] = expected[name] or "System.Int32"
    require(set(found) == set(expected), "Display helper call inventory differs")
    return {"displayHelperCallParametersBoundToMainTypeDefs": found}


def verify(before, after, expected_input_sha=BASE_DLL):
    require(sha(before) == expected_input_sha, "Exact qualified managed input required")
    a, b = [dnfile.dnPE(data=raw) for raw in (before, after)]
    left, right = methods(a), methods(b)
    require(len(left) == len(right), "Method inventory changed")
    for table in ["TypeDef", "Field", "FieldPtr", "MethodPtr"]:
        x, y = getattr(a.net.mdtables, table, None), getattr(b.net.mdtables, table, None)
        require((len(x.rows) if x else 0) == (len(y.rows) if y else 0), "Managed inventory changed: " + table)
    require(not getattr(b.net.mdtables, "FieldPtr", None), "Old Unity must not gain FieldPtr")
    unchanged = engine = 0
    changes = []
    originals, outputs = {}, {}
    for (name, rid, old), (other, oid, new) in zip(left, right):
        require((name, rid) == (other, oid) and old.Signature.value == new.Signature.value
                and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags,
                "Inherited method ABI/token changed: " + name)
        if not old.Rva:
            require(not new.Rva, "Unexpected new body: " + name)
            continue
        x, y = exact_body(a, old), exact_body(b, new)
        if name not in TARGETS:
            require(x == y, "Unrelated method header/IL/EH changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            continue
        require(sha(x) == TARGETS[name], "Display target guard differs: " + name)
        oldbody, newbody = [read_method_body_from_bytes(raw) for raw in (x, y)]
        require(not oldbody.exception_handlers and not newbody.exception_handlers, "Unexpected display EH")
        originals[name], outputs[name] = oldbody, newbody
        if name == "UIAutoSize::Awake":
            calls = [i for i in oldbody.instructions if i.opcode.name == "call"
                     and member(a, i.operand) == ("UnityEngine.Screen", "SetResolution")]
            require(len(calls) == 1 and calls[0].size == 5, "Expected one resolution call")
            at = calls[0].offset
            expected = bytearray(x)
            expected[at:at + 5] = b"\x26\x26\x26\x00\x00"
            require(bytes(expected) == y, "Aspect classification/anchors/resolution data changed")
        elif name in ("FPSFixer::Start", "UI.QualityBar::Start"):
            instructions = newbody.instructions
            require([i.opcode.name for i in instructions] == ["ldarg.0", "call", "ret"], "Unexpected display bridge initializer")
            required_method = "StartFPS" if name == "FPSFixer::Start" else "Attach"
            require(member(b, instructions[1].operand) == ("HYJS.Display.DisplaySettings", required_method), "Display bridge initializer differs")
        elif name == "FPSFixer::Update":
            require([i.opcode.name for i in newbody.instructions] == ["ret"], "Legacy sleep/adaptation remains active")
        elif name == "RenderManager::get_MaxFPS":
            require([i.opcode.name for i in newbody.instructions] == ["call", "ret"]
                    and member(b, newbody.instructions[0].operand) == ("HYJS.Display.DisplaySettings", "get_TargetFPS"), "Saved quality maximum differs")
        elif name == "UI.QualityBar::Refresh":
            def normalized(body):
                indexes = {i.offset: n for n, i in enumerate(body.instructions)}
                values = []
                for instruction in body.instructions:
                    value = operand(instruction.operand)
                    if instruction.opcode.name == "switch":
                        value = tuple(indexes[v] for v in instruction.operand)
                    elif instruction.opcode.flow_control.name in ("Branch", "Cond_Branch"):
                        value = indexes[instruction.operand]
                    values.append((instruction.opcode.name, value))
                return values
            original, updated = normalized(oldbody), normalized(newbody)
            require(original[-1][0] == "ret" and updated[:-3] == original[:-1], "Original quality Refresh branches changed")
            require([i.opcode.name for i in newbody.instructions[-3:]] == ["ldarg.0", "call", "ret"]
                    and member(b, newbody.instructions[-2].operand) == ("HYJS.Display.DisplaySettings", "Attach"), "Missing real page-open display attachment")
        changes.append({"method": name, "beforeRawSHA256": sha(x), "afterRawSHA256": sha(y)})
    require(engine == 547 and len(changes) == 6, "Engine/target inventory differs")
    # The unchanged adaptive Refresh body is private and its only caller was
    # FPSFixer.Update. Prove no residual direct caller can reset the target rate.
    refresh = next((rid, row) for name, rid, row in left if name == "FPSFixer::Refresh")
    require(refresh[1].Flags.mdPrivate, "Legacy adaptive refresh unexpectedly public")
    callers = []
    for name, rid, row in right:
        if not row.Rva:
            continue
        for i in read_method_body_from_bytes(exact_body(b, row)).instructions:
            if i.opcode.name in ("call", "callvirt", "ldftn", "ldvirtftn") and operand(i.operand) == 0x06000000 | refresh[0]:
                callers.append(name)
    require(not callers, "Legacy adaptive refresh still reachable")
    return {"originalEngineMethodsUnchanged": engine, "unchangedInheritedRawMethodBodies": unchanged,
            "allInheritedNonTargetMethodsExact": True, "noManagedTypesMethodsFieldsAdded": True,
            "methodTokensABIAndFlagsPreserved": True, "legacyAdaptiveRefreshCallers": callers,
            "existingNGUIFitBothAndAnchorsUnchanged": True, "defaultRequestedTargetFPS": 60, "selectableRequestedTargetFPS": [60, 120],
            "nativeDisplayResolutionRetained": True, "masterTextureLimit": 0, "maximumLODLevel": 0,
            "userGraphicsQualitySelectionPreserved": True, "antiAliasingModified": False, "changes": changes,
            **verify_display_call_scopes(b)}


def manifest_policy(raw):
    parts = chunks(raw)
    strings, _ = strings_from_pool(next(p[2] for p in parts if p[0] == 1))
    rows = elements(parts, strings)
    sdk = next(attrs for name, attrs in rows if name == "uses-sdk")
    target = next(value for ns, name, kind, value in sdk if name == "targetSdkVersion")
    require(target >= 26, "Review pre-Oreo aspect ratio contract")
    require(not any(key in ("maxAspectRatio", "resizeableActivity", "windowLayoutInDisplayCutoutMode") for _, attrs in rows for _, key, _, _ in attrs), "Review explicit display safety policy")
    require(not any(key == "name" and value == "android.max_aspect" for _, attrs in rows for _, key, _, value in attrs), "Review explicit legacy aspect cap")
    return {"manifestChanged": False, "targetSdkVersion": target, "explicitAspectRatioCap": False,
            "safeRegionPolicy": "Retain Android default display-cutout exclusion and existing native window insets; do not force content under camera/notch"}


def verify_bridge(raw):
    pe = dnfile.dnPE(data=raw)
    require(str(pe.net.mdtables.Assembly.rows[0].Name) == "SoulFlowerDisplay", "Display bridge identity differs")
    references = {str(row.Name) for row in pe.net.mdtables.AssemblyRef}
    require(references == {"mscorlib", "UnityEngine", "Assembly-CSharp"}, "Unexpected bridge runtime/service dependency")
    require(bytes(pe.net.metadata.struct.Version).startswith(b"v2.0.50727"), "Old Unity metadata runtime differs")
    require(not getattr(pe.net.mdtables, "FieldPtr", None), "Bridge must not introduce FieldPtr")
    required = {"get_TargetFPS", "SetTargetFPS", "Apply", "StartFPS", "Attach"}
    owners = [t for t in pe.net.mdtables.TypeDef if str(t.TypeNamespace) == "HYJS.Display" and str(t.TypeName) == "DisplaySettings"]
    require(len(owners) == 1, "Display settings entry missing")
    found = {str(m.row.Name): m.row for m in owners[0].MethodList}
    require(required <= set(found), "Required bridge ABI missing")
    require(all(found[name].Flags.mdStatic and found[name].Flags.mdPublic for name in required), "Display bridge ABI is not public static")
    require(found["get_TargetFPS"].Signature.value.hex() == "000008"
            and found["SetTargetFPS"].Signature.value.hex() == "00010108", "Saved frame option ABI differs")
    for row in pe.net.mdtables.MemberRef:
        owner = row.Class.row
        owner_name = str(getattr(owner, "TypeNamespace", "")) + "." + str(getattr(owner, "TypeName", ""))
        require(not owner_name.startswith(("System.Net.", "System.IO.", "UnityEngine.WWW", "UnityEngine.Networking.")), "Display bridge gained IO/network dependency")
        require(str(row.Name) not in ("set_antiAliasing", "set_allowMSAA", "SetQualityLevel"), "Unexpected anti-aliasing/quality override")
    return {"displayBridgeSHA256": sha(raw), "displayBridgeAssembly": "SoulFlowerDisplay",
            "displayBridgeOriginalRuntimeOnly": True, "displayBridgeReferences": sorted(references),
            "displayBridgePublicEntries": sorted(required), "displayBridgeNoNetworkOrFileIO": True,
            "displayBridgeNoMSAAOrQualityPresetOverride": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, default=ROOT / "apks/build/hyjs-local.apk")
    parser.add_argument("--input-managed", type=Path, help="Apply after a separately qualified overlay; APK remains the pinned source of Unity/runtime/manifest")
    parser.add_argument("--input-managed-sha256", help="Required exact SHA-256 when composing with --input-managed")
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    require(sha(args.base_apk.read_bytes()) == BASE, "Qualified b982 APK required")
    output = args.output_directory.resolve()
    require(not output.exists(), "Use a fresh output directory")
    output.mkdir(parents=True)
    with zipfile.ZipFile(args.base_apk) as apk:
        before, unity, android_manifest = apk.read(ENTRY), apk.read(UNITY), apk.read("AndroidManifest.xml")
    require(sha(before) == BASE_DLL, "Pinned base APK managed input differs")
    expected_input_sha = BASE_DLL
    if args.input_managed:
        require(args.input_managed_sha256 is not None and len(args.input_managed_sha256) == 64,
                "Bind the separately qualified overlay with --input-managed-sha256")
        before = args.input_managed.read_bytes()
        expected_input_sha = args.input_managed_sha256
        require(sha(before) == expected_input_sha, "Composition input SHA differs")
    else:
        require(args.input_managed_sha256 is None, "Input SHA requires --input-managed")
    policy = manifest_policy(android_manifest)
    runtime = ROOT / ".tools/native-batch"
    dotnet = runtime / "dotnet/dotnet"
    dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"
    injector = Path(__file__).with_name("Injector.cs")
    source = Path(__file__).with_name("DisplaySettings.cs")
    compiler = next((runtime / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))

    def run(name, command):
        result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=90)
        (output / (name + ".log")).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + " failed; see saved log")

    with tempfile.TemporaryDirectory(prefix="hyjs-display-overlay-") as temporary:
        folder = Path(temporary)
        current, engine = folder / "Assembly-CSharp.dll", folder / "UnityEngine.dll"
        current.write_bytes(before)
        engine.write_bytes(unity)
        standard = folder / "mscorlib.dll"
        with zipfile.ZipFile(args.base_apk) as apk:
            standard.write_bytes(apk.read("assets/bin/Data/Managed/mscorlib.dll"))
        bridge = output / "SoulFlowerDisplay.dll"
        run("compile-display-bridge", [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+", "/out:" + str(bridge), "/reference:" + str(current), "/reference:" + str(engine), "/reference:" + str(standard), source])
        shutil.copy2(injector, folder / "Program.cs")
        project = folder / "Patch.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run("compile-injector", [dotnet, "build", project, "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        run("patch", [dotnet, folder / "bin/Debug/net8.0/Patch.dll", current, engine, bridge, output / "Assembly-CSharp.dll"])
    after = (output / "Assembly-CSharp.dll").read_bytes()
    checks = verify(before, after, expected_input_sha)
    checks.update(verify_bridge((output / "SoulFlowerDisplay.dll").read_bytes()))
    report = {"schema": "hyjs-native-display-selectable-fps-overlay-v2", "status": "STATIC_VERIFIED_NOT_PACKAGED",
              "baseAPKSHA256": BASE, "qualifiedBaseManagedSHA256": BASE_DLL,
              "inputManagedSHA256": sha(before), "outputManagedSHA256": sha(after),
              "compositionMode": "after-qualified-overlay" if args.input_managed else "pinned-base",
              "sourcePins": {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [Path(__file__).resolve(), injector, source]},
              "replacementEntries": [ENTRY, "assets/bin/Data/Managed/SoulFlowerDisplay.dll"], "phoneExecuted": False,
              "note": "60/120 are requested rendering caps; achieved frame rate requires device measurements and remains limited by display refresh rate, hardware, thermal state and scene load",
              "manifestPolicy": policy, **checks}
    (output / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": report["status"], "outputManagedSHA256": sha(after), "originalEngineMethodsUnchanged": 547}))


if __name__ == "__main__":
    main()
