"""Build the pinned 1.0.80 Wanted recovery overlay; never package or use a device."""
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
HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
from tools.client_feedback65.build import normalized, parsed
from tools.client_one_key59.build import verify as verify_inventory
from tools.local_asset_ready.remove_gate import exact_body, require

BASE = "25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a"
MAIN_SHA = "8b732cce869cad9a9a06c75d3ba4891b4084a3ac646848fc99a933148e300437"
HELPER_SHA = "9a5bdef854697868c49e1cfb28cd81038b8c8b6ce3d48ba988b079385e6a9036"
UNCHANGED_SELECTION_SOURCE = "f273754f400d620b9586828c4fcff9e269a9c5d9d7177185f1386a9f379faaf6"
PINS = {0x2B6B: "3342c34c9bae5cced1305fe32cd96a6ab76810fc970bc4fd800b46a453619e4e",
        0x5043: "3ac5e074a911ac2c77da715b699a3f265d4cfd4b1d86ad9dc039ed39ed2eb0a5",
        0x5044: "00127b1c2a0bcfa337362bea71458f753f450d5ded38795f835cd14f62cd5c44",
        0x504E: "d40aef1d6316eb46ab4436120ddfdf19ccbd37d7ea0cb7595cfe64c9c8b4fb87"}
MANAGED = "assets/bin/Data/Managed/"
MAIN = MANAGED + "Assembly-CSharp.dll"
HELPER = MANAGED + "HYJS.Feedback65.dll"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def file_sha(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def ref(pe, instruction):
    if instruction.opcode.name != "call" or instruction.operand.value >> 24 != 0x0A:
        return ""
    row = pe.net.mdtables.MemberRef.rows[(instruction.operand.value & 0xFFFFFF) - 1]
    if str(getattr(row.Class.row, "TypeNamespace", "")) != "HYJS.Feedback65":
        return ""
    return str(row.Name)


def proof(before, after):
    a, b = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    for rid, expected in PINS.items():
        require(sha(exact_body(a, a.net.mdtables.MethodDef.rows[rid - 1])) == expected, "input target drift")
    result = verify_inventory(before, after, [{"token": hex(0x06000000 + rid)} for rid in PINS])
    for rid in PINS:
        old, new = parsed(a, rid), parsed(b, rid)
        require(not old.exception_handlers and not new.exception_handlers, "unexpected target exception handling")
        require(getattr(old.local_var_sig_tok, "value", None) == getattr(new.local_var_sig_tok, "value", None), "target locals changed")
        rows = list(new.instructions)
        if rid == 0x2B6B:
            at = next(i for i, item in enumerate(rows) if item.opcode.name == "ldc.i4" and item.operand == 55011)
            capture = rows[at - 3:at]
            require([i.opcode.name for i in capture] == ["ldarg.0", "ldloc", "call"] and
                    ref(b, capture[-1]) == "WantedCaptureEnd", "wrong capture hook")
            rows = rows[:at - 3] + rows[at:]
            calls = [item for item in rows if ref(b, item) in ("WantedWrapEnd", "WantedEndSent")]
            require([ref(b, item) for item in calls] == ["WantedWrapEnd", "WantedEndSent"], "wrong send hooks")
            rows = [item for item in rows if item not in calls]
        elif rid == 0x504E:
            at = next(i for i, item in enumerate(rows) if item.opcode.name == "ldc.i4" and item.operand == 55003)
            guard = rows[at - 5:at]
            require([i.opcode.name for i in guard] == ["ldarg.0", "ldloc", "call", "brtrue", "ret"] and
                    ref(b, guard[2]) == "WantedPrepareStart" and guard[3].operand == rows[at].offset, "wrong paid Start guard")
            rows = rows[:at - 5] + rows[at:]
            sent = [item for item in rows if ref(b, item) in ("WantedWrapStart", "WantedStartSent")]
            require([ref(b, item) for item in sent] == ["WantedWrapStart", "WantedStartSent"], "wrong Start send hooks")
            rows = [item for item in rows if item not in sent]
        elif rid == 0x5043:
            require([i.opcode.name for i in rows[-3:]] == ["ldarg.0", "call", "ret"] and
                    ref(b, rows[-2]) == "WantedPageShown", "wrong page hook")
            rows = rows[:-3] + rows[-1:]
        else:
            prefix, rows = rows[:4], rows[4:]
            require([i.opcode.name for i in prefix] == ["ldarg.0", "ldarg.1", "ldarg.2", "call"] and
                    ref(b, prefix[-1]) == "WantedListReceived", "wrong list callback hook")
        require(normalized(old, old.instructions) == normalized(new, rows), "original target control flow changed")
    # Callback hook signatures must refer to the native Main TypeDef, never a
    # circular helper/module type reference, as required by the old Unity runtime.
    own = {(str(t.TypeNamespace), str(t.TypeName)) for t in a.net.mdtables.TypeDef.rows}
    for row in b.net.mdtables.TypeRef.rows[len(a.net.mdtables.TypeRef.rows):]:
        require((str(row.TypeNamespace), str(row.TypeName)) not in own or row.ResolutionScope.table.name == "Module",
                "new native type reference has external scope")
    added = [row for row in b.net.mdtables.MemberRef.rows[len(a.net.mdtables.MemberRef.rows):]
             if str(getattr(row.Class.row, "TypeNamespace", "")) == "HYJS.Feedback65"]
    require({str(row.Name) for row in added} == {"WantedCaptureEnd", "WantedWrapEnd", "WantedEndSent", "WantedPrepareStart", "WantedWrapStart", "WantedStartSent", "WantedPageShown", "WantedListReceived"},
            "unexpected Wanted hook inventory")
    handler_type = next((i for i, row in enumerate(b.net.mdtables.TypeDef.rows, 1)
                         if str(row.TypeNamespace) == "NetWork.Layer" and str(row.TypeName) == "HttpHandler"), None)
    require(handler_type is not None, "native callback type missing")
    wrap = next(row for row in added if str(row.Name) == "WantedWrapEnd")
    # TypeDefOrRef encoded index for a TypeDef has tag 0.
    native_index = handler_type << 2
    if native_index < 0x80:
        encoded = bytes([native_index])
    elif native_index < 0x4000:
        encoded = bytes([(native_index >> 8) | 0x80, native_index & 0xFF])
    else:
        encoded = bytes([(native_index >> 24) | 0xC0, (native_index >> 16) & 0xFF, (native_index >> 8) & 0xFF, native_index & 0xFF])
    require(wrap.Signature.value == b"\x00\x01\x12" + encoded + b"\x12" + encoded, "callback hook is not bound to Main TypeDef")
    start_wrap = next(row for row in added if str(row.Name) == "WantedWrapStart")
    require(start_wrap.Signature.value == wrap.Signature.value, "Start callback hook is not bound to Main TypeDef")
    result.update(originalTargetsControlFlowAndLocalsExact=True, nativeCallbackTypeDefBound=True,
                  originalEngineMethodsExact=547)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", required=True, type=Path)
    parser.add_argument("--output-directory", required=True, type=Path)
    parser.add_argument("--input-managed", type=Path, help="Exact Main from the current composed overlay")
    parser.add_argument("--input-managed-sha256", help="SHA256 of --input-managed")
    parser.add_argument("--tool-root", type=Path, default=Path("/Users/lirui/LR/hyjs"))
    args = parser.parse_args()
    out, apk = args.output_directory.resolve(), args.base_apk.resolve()
    require(file_sha(apk) == BASE, "formal 80 APK SHA mismatch")
    require((args.input_managed is None) == (args.input_managed_sha256 is None), "managed input and SHA must be supplied together")
    require(out.is_relative_to(HERE) and not out.exists(), "fresh task-owned output required")
    source = ROOT / "tools/client_feedback65/Runtime.cs"
    source_bytes = source.read_bytes()
    source_paths = [source, HERE / "Injector.cs", HERE / "build.py"]
    snapshots = {path: path.read_bytes() for path in source_paths}
    suffix = source_bytes[source_bytes.index(b"        static FieldInfo Field("):]
    require(sha(suffix) == UNCHANGED_SELECTION_SOURCE, "non-Wanted helper source changed")
    out.mkdir(parents=True)
    runtime = args.tool_root.resolve() / ".tools/native-batch"
    dotnet = runtime / "dotnet/dotnet"
    compiler = next((runtime / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"

    def run(label, command):
        result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=180)
        (out / (label + ".log")).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, label + " failed; inspect retained log")

    with tempfile.TemporaryDirectory(prefix="hyjs-wanted84-") as temporary, zipfile.ZipFile(apk) as archive:
        work = Path(temporary)
        refs = work / "refs"
        refs.mkdir()
        base_main, previous_helper = archive.read(MAIN), archive.read(HELPER)
        require(sha(base_main) == MAIN_SHA and sha(previous_helper) == HELPER_SHA, "formal 80 managed SHA mismatch")
        before = args.input_managed.resolve().read_bytes() if args.input_managed else base_main
        input_sha = args.input_managed_sha256.lower() if args.input_managed else MAIN_SHA
        require(len(input_sha) == 64 and sha(before) == input_sha, "composed managed input SHA mismatch")
        require(len(archive.namelist()) == len(set(archive.namelist())), "duplicate APK entries")
        for name in archive.namelist():
            if name.startswith(MANAGED) and name.endswith(".dll"):
                (refs / Path(name).name).write_bytes(archive.read(name))
        (refs / "Assembly-CSharp.dll").write_bytes(before)
        compiled = out / "HYJS.Feedback65.dll"
        stable_source = work / "Runtime.cs"
        stable_source.write_bytes(snapshots[source])
        run("compile-helper", [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3",
                               "/deterministic+", "/codepage:65001", "/out:" + str(compiled),
                               *["/r:" + str(path) for path in sorted(refs.glob("*.dll")) if path.name != compiled.name], stable_source])
        old_helper, new_helper = dnfile.dnPE(data=previous_helper), dnfile.dnPE(str(compiled))
        def public_methods(pe):
            owner = next(t for t in pe.net.mdtables.TypeDef.rows if str(t.TypeNamespace) == "HYJS.Feedback65" and str(t.TypeName) == "Runtime")
            return {str(x.row.Name): x.row.Signature.value for x in owner.MethodList if x.row.struct.Flags & 0x7 == 0x6}
        old_public, new_public = public_methods(old_helper), public_methods(new_helper)
        require(all(new_public.get(name) == signature for name, signature in old_public.items()), "existing helper public ABI changed")
        (work / "Program.cs").write_bytes(snapshots[HERE / "Injector.cs"])
        project = work / "Inject.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><EnableDefaultCompileItems>false</EnableDefaultCompileItems></PropertyGroup><ItemGroup><Compile Include="Program.cs" /><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run("compile-injector", [dotnet, "build", project, "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        patched = out / "Assembly-CSharp.dll"
        run("inject", [dotnet, work / "bin/Debug/net8.0/Inject.dll", refs / "Assembly-CSharp.dll", compiled, patched, input_sha])
        result = proof(before, patched.read_bytes())
        require(all(path.read_bytes() == value for path, value in snapshots.items()), "source changed during overlay build")
        payload = {MAIN: str(patched.relative_to(ROOT)), HELPER: str(compiled.relative_to(ROOT))}
        report = {"schema": "hyjs-wanted84-recovery-overlay-v1", "baseAPKSHA256": BASE,
                  "inputManagedSHA256": input_sha, "inputHelperSHA256": HELPER_SHA,
                  "payloadFiles": payload, "replacementEntries": {name: file_sha(ROOT / path) for name, path in payload.items()},
                  "sourcePins": {str(path.relative_to(ROOT)): sha(value) for path, value in snapshots.items()},
                  "nonWantedSelectionSourceSHA256": sha(suffix), "existingHelperPublicABIExact": True,
                  "managedOnly": True, "inheritedJavaByteExact": True, "inheritedJavaPublicProtectedABIExact": True,
                  "apkBuilt": False, "deviceTested": False, **result}
        (out / "manifest.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(result))


if __name__ == "__main__":
    main()
