"""Keep the original arena panels' client caches consistent with server replies.

Three narrow hooks, no new target, reward or qualification logic:

1. UI.GrabPVPItem.SetItem(GrabPositionModel) keeps its original rendering, then
   forwards the same server frame to HYJS.ArenaState.ArenaStateSync.SyncGrabCache.
   The helper updates the matching position in the original private
   UI.GrabPVPPanel.grabList and recomputes UI.GrabPVPPanel.isSelfGrabed from that
   cache. The original occupation reply and the original give-up reply both call
   SetItem, so field-tab switches no longer reuse a stale cache.
2. UI.PlunderSelectPage.DoOpen() hides and clears the fixed prefab target slots
   at entry. The original 0.5s delay, CSTreasureBattleList request and callbacks
   stay untouched.
3. UI.PlunderSelectPage.GetRivalCallback(...) adds the empty-result notice on the
   success fall-through only. The failure branch jumps to the original ret.

The main DLL is copied from an explicitly SHA-256 pinned APK, the three pinned
bodies are extended in place, and a package_native-compatible managed overlay is
written. This never packages, installs, launches the game, edits resources, or
touches a device, and it refuses to overwrite an existing overlay directory.
"""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import struct
import subprocess
import sys
import tempfile
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
SOURCE = Path(__file__).resolve().parent
ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
HELPER_NAME = "HYJS.ArenaState"
HELPER_ENTRY = "assets/bin/Data/Managed/" + HELPER_NAME + ".dll"
EXPECTED_MANAGED_SHA256 = "ee14d5df4077573abf94be309d6ebdbe7f4ba84c31b09743ed9a9008b65d200f"
ENGINE_METHODS = 547
HELPER_REFERENCES = ["Assembly-CSharp", "mscorlib", "UnityEngine"]

# Pins bind the published cde88e0 DLL. Its executable instructions match the
# original APK; prior packaging normalized only maxstack headers to 3.
TARGETS = [
    {
        "type": "UI.GrabPVPItem",
        "name": "SetItem",
        "signature": "2001011281d8",
        "body": "99b02b2c05a2f73af100e2e8721628634dc3ca42756f3ee08df03043e71977b6",
        "hook": "tail",
        "inserted": [
            ("ldarg.0", None),
            ("ldfld", "grabPanel"),
            ("ldarg.1", None),
            ("call", ("HYJS.ArenaState.ArenaStateSync", "SyncGrabCache")),
        ],
        "callSignature": ("System.Void", ["UI.GrabPVPPanel", "ProtoBuf.Message.GrabPositionModel"]),
        "reason": "keep the original 18-slot cache current after occupation and give-up replies",
    },
    {
        "type": "UI.PlunderSelectPage",
        "name": "DoOpen",
        "signature": "200001",
        "body": "80dbbf40ec30403d16ccac33d6bafae4b557bccbcc3f4375b9b77c2cdbaeaad5",
        "hook": "entry",
        "inserted": [
            ("ldarg.0", None),
            ("call", ("HYJS.ArenaState.ArenaStateSync", "ClearPlunderTargets")),
        ],
        "callSignature": ("System.Void", ["UI.PlunderSelectPage"]),
        "reason": "hide the four prefab placeholders during the original 0.5s request delay",
    },
    {
        "type": "UI.PlunderSelectPage",
        "name": "GetRivalCallback",
        "signature": "20020112a83c02",
        "body": "6c7b13f15b0adbaccd12d18948d23e2d3646e2a94a78ccf1a904e894b3efb265",
        "hook": "tail",
        "inserted": [
            ("ldarg.0", None),
            ("ldloc.1", None),
            ("call", ("HYJS.ArenaState.ArenaStateSync", "ShowEmptyRivals")),
        ],
        "callSignature": ("System.Void", ["UI.PlunderSelectPage", "ProtoBuf.Message.SCTreasureBattleList"]),
        "reason": "report a successful empty rival list without treating failures as empty",
    },
]


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(ok, message):
    if not ok:
        raise ValueError(message)


def methods(pe):
    return [('.'.join(filter(None, [str(t.TypeNamespace), str(t.TypeName)])) + '::' + str(m.row.Name),
             m.row_index, m.row) for t in pe.net.mdtables.TypeDef for m in t.MethodList]


def exact_body(pe, method):
    header = pe.get_data(method.Rva, 12)
    if header[0] & 3 == 2:
        length = 1 + (header[0] >> 2)
    else:
        flags = struct.unpack_from('<H', header)[0]
        length = ((flags >> 12) * 4) + struct.unpack_from('<I', header, 4)[0]
        if flags & 8:
            at = (length + 3) & ~3
            while True:
                section = pe.get_data(method.Rva + at, 4)
                size = int.from_bytes(section[1:4], 'little') if section[0] & 0x40 else section[1]
                require(size >= 4, 'Malformed method extra section')
                at += size
                if not section[0] & 0x80:
                    break
                at = (at + 3) & ~3
            length = at
    return pe.get_data(method.Rva, length)


def decode(pe, method):
    return list(read_method_body_from_bytes(exact_body(pe, method)).instructions)


def instruction_value(instruction):
    operand = instruction.operand
    if operand is None:
        return None
    value = getattr(operand, "value", operand)
    index = getattr(value, "index", None)
    return ("local", index) if index is not None else value


def read_compressed(raw, position):
    first = raw[position]
    if first & 0x80 == 0:
        return first, position + 1
    if first & 0xC0 == 0x80:
        return ((first & 0x3F) << 8) | raw[position + 1], position + 2
    return (((first & 0x1F) << 24) | (raw[position + 1] << 16)
            | (raw[position + 2] << 8) | raw[position + 3]), position + 4


def type_full_name(pe, table, rid):
    rows = pe.net.mdtables.TypeDef.rows if table == 0x02 else pe.net.mdtables.TypeRef.rows
    row = rows[rid - 1]
    return str(row.TypeNamespace) + "." + str(row.TypeName)


def signature_types(pe, signature, has_this=False):
    """Return (return type, parameter types) for a default-calling-convention signature."""
    require(len(signature) > 1, "Empty helper signature")
    require(signature[0] == (0x20 if has_this else 0x00), "Unexpected helper calling convention")
    count, position = read_compressed(signature, 1)
    names = []
    for _ in range(count + 1):
        if signature[position] == 0x01:
            names.append("System.Void")
            position += 1
            continue
        require(signature[position] == 0x12, "Helper signature must use class types only")
        coded, position = read_compressed(signature, position + 1)
        tag = coded & 0x03
        require(tag in (0x00, 0x01), "Unexpected helper signature type encoding")
        names.append(type_full_name(pe, 0x02 if tag == 0x00 else 0x01, coded >> 2))
    require(position == len(signature), "Trailing helper signature bytes")
    return names[0], names[1:]


def method_name(pe, token):
    if not isinstance(token, int):
        return ""
    table, rid = token >> 24, token & 0xffffff
    if table == 0x06:
        return str(pe.net.mdtables.MethodDef.rows[rid - 1].Name)
    if table == 0x0A:
        return str(pe.net.mdtables.MemberRef.rows[rid - 1].Name)
    return ""


def field_name(pe, token):
    if not isinstance(token, int) or token >> 24 != 0x04:
        return ""
    return str(pe.net.mdtables.Field.rows[(token & 0xffffff) - 1].Name)


def member_reference(pe, token):
    require(isinstance(token, int) and token >> 24 == 0x0A, "Inserted call is not a MemberRef")
    return pe.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1]


def verify_insertion(left, right, old, new, spec):
    old_code, new_code = decode(left, old), decode(right, new)
    inserted = spec["inserted"]
    k = 0 if spec["hook"] == "entry" else len(old_code) - 1
    require(len(new_code) == len(old_code) + len(inserted), "Target instruction count differs")
    old_offsets = {instruction.offset: index for index, instruction in enumerate(old_code)}

    def shifted(index):
        return index + len(inserted) if index >= k else index

    def expected_target(offset):
        require(offset in old_offsets, "Branch target leaves the original body")
        return new_code[shifted(old_offsets[offset])].offset

    for index, before in enumerate(old_code):
        after = new_code[shifted(index)]
        require(before.opcode.name == after.opcode.name, "Original opcode changed: " + spec["name"])
        opcode = before.opcode.name
        if isinstance(before.operand, int) and opcode.startswith(
                ("br", "beq", "bne", "bge", "bgt", "ble", "blt", "leave")):
            require(after.operand == expected_target(before.operand),
                    "Original branch destination changed: " + spec["name"])
        elif opcode == "switch":
            require(tuple(after.operand) == tuple(expected_target(target) for target in before.operand),
                    "Original switch destination changed: " + spec["name"])
        else:
            require(instruction_value(before) == instruction_value(after),
                    "Original operand changed: " + spec["name"])

    block = new_code[k:k + len(inserted)]
    for instruction, expected in zip(block, inserted):
        require(instruction.opcode.name == expected[0], "Inserted opcode differs: " + spec["name"])
        if expected[0] in ("ldarg.0", "ldarg.1", "ldloc.1"):
            require(instruction.operand is None, "Inserted local/arg carries an operand")
        elif expected[0] == "ldfld":
            require(field_name(right, instruction_value(instruction)) == expected[1],
                    "Inserted field differs: " + spec["name"])
        elif expected[0] == "call":
            reference = member_reference(right, instruction_value(instruction))
            owner = reference.Class.row
            require(str(owner.TypeNamespace) + "." + str(owner.TypeName) == expected[1][0],
                    "Inserted call owner differs")
            require(str(reference.Name) == expected[1][1], "Inserted call name differs")
            returns, parameters = signature_types(right, reference.Signature.value)
            require(returns == spec["callSignature"][0], "Inserted call return differs")
            require(parameters == spec["callSignature"][1], "Inserted call parameters differ")
        else:
            raise AssertionError("Unexpected inserted instruction " + expected[0])

    old_body = read_method_body_from_bytes(exact_body(left, old))
    new_body = read_method_body_from_bytes(exact_body(right, new))
    if k == 0:
        site = old_code[0].offset
    else:
        site = old_code[-1].offset
    return {
        "method": spec["type"] + "::" + spec["name"],
        "hook": spec["hook"],
        "siteOffset": site,
        "inserted": [name for name, _ in inserted],
        "reason": spec["reason"],
        "beforeRawSHA256": sha(exact_body(left, old)),
        "afterRawSHA256": sha(exact_body(right, new)),
        "instructionsBefore": len(old_code),
        "instructionsAfter": len(new_code),
        "exceptionHandlersPreserved": len(old_body.exception_handlers) == len(new_body.exception_handlers) == 0,
    }


def verify_helper(raw):
    pe = dnfile.dnPE(data=raw)
    version = bytes(pe.net.metadata.struct.Version).rstrip(b"\x00")
    require(version == b"v2.0.50727", "Helper must target the old Unity runtime")
    assembly = pe.net.mdtables.Assembly.rows
    require(len(assembly) == 1 and str(assembly[0].Name) == HELPER_NAME, "Unexpected helper assembly identity")
    references = sorted(str(row.Name) for row in pe.net.mdtables.AssemblyRef.rows)
    require(references == sorted(HELPER_REFERENCES), "Unexpected helper dependency set: " + repr(references))
    types = [row for row in pe.net.mdtables.TypeDef.rows if str(row.TypeName) != "<Module>"]
    require(len(types) == 1, "Helper must define exactly one type")
    require(str(types[0].TypeNamespace) == "HYJS.ArenaState" and str(types[0].TypeName) == "ArenaStateSync",
            "Unexpected helper type identity")
    definitions = [entry.row for entry in types[0].MethodList]
    expected = {
        "SyncGrabCache": ["UI.GrabPVPPanel", "ProtoBuf.Message.GrabPositionModel"],
        "ClearPlunderTargets": ["UI.PlunderSelectPage"],
        "ShowEmptyRivals": ["UI.PlunderSelectPage", "ProtoBuf.Message.SCTreasureBattleList"],
    }
    require(sorted(str(method.Name) for method in definitions) == sorted(list(expected) + ["GrabList"]),
            "Unexpected helper method set")
    signatures = {}
    for method in definitions:
        name = str(method.Name)
        require(bool(method.struct.Flags & 0x0010), "Helper method must be static: " + name)
        if name == "GrabList":
            # Private static List<GrabPositionModel> GrabList(GrabPVPPanel); the generic
            # return type is intentionally not re-encoded by this narrow verifier.
            require(method.Signature.value[:3] == b"\x00\x01\x15", "Helper cache getter signature differs")
            signatures[name] = {"static": True, "signature": method.Signature.value.hex(),
                                "bodySHA256": sha(exact_body(pe, method))}
            continue
        returns, actual = signature_types(pe, method.Signature.value)
        require(returns == "System.Void", "Helper method must return void: " + name)
        require(actual == expected[name], "Helper parameter types differ: " + name)
        require(method.struct.Flags & 0x0006 == 0x0006, "Helper method must be public: " + name)
        signatures[name] = {
            "static": True,
            "signature": method.Signature.value.hex(),
            "bodySHA256": sha(exact_body(pe, method)),
        }
    fields = [str(entry.row.Name) for entry in types[0].FieldList]
    require(fields == ["grabListField"], "Unexpected helper fields")
    require("grabList".encode("utf-16-le") in raw, "Helper lost the original private cache name")
    require("当前没有可夺取该碎片的对手".encode("utf-16-le") in raw, "Helper lost the empty-rival notice")
    return {
        "assembly": HELPER_NAME,
        "runtime": version.decode(),
        "references": references,
        "definedTypes": [str(types[0].TypeNamespace) + "." + str(types[0].TypeName)],
        "definedMethods": signatures,
    }


def verify(before, after, helper_raw):
    left, right = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    old_methods, new_methods = methods(left), methods(right)
    require(len(old_methods) == len(new_methods), "Method inventory changed")
    for table in ("TypeDef", "Field", "MethodDef", "Param", "Property", "Event",
                  "TypeSpec", "MethodSpec", "ModuleRef"):
        require(len(getattr(left.net.mdtables, table).rows) == len(getattr(right.net.mdtables, table).rows),
                "Original metadata inventory changed: " + table)
    require(not getattr(right.net.mdtables, "FieldPtr", None), "Unexpected old-Unity FieldPtr")
    require(len(right.net.mdtables.TypeRef.rows) == len(left.net.mdtables.TypeRef.rows) + 1,
            "Unexpected TypeRef addition")
    require(len(right.net.mdtables.MemberRef.rows) == len(left.net.mdtables.MemberRef.rows) + 3,
            "Unexpected MemberRef addition")
    require(len(right.net.mdtables.AssemblyRef.rows) == len(left.net.mdtables.AssemblyRef.rows) + 1,
            "Unexpected AssemblyRef addition")
    added = right.net.mdtables.TypeRef.rows[-1]
    require(str(added.TypeNamespace) == "HYJS.ArenaState" and str(added.TypeName) == "ArenaStateSync",
            "Unexpected helper TypeRef")
    added_refs = right.net.mdtables.MemberRef.rows[-3:]
    require([str(row.Name) for row in added_refs]
            == ["SyncGrabCache", "ClearPlunderTargets", "ShowEmptyRivals"],
            "Unexpected helper MemberRef order")
    for row in added_refs:
        require(str(row.Class.row.TypeNamespace) + "." + str(row.Class.row.TypeName)
                == "HYJS.ArenaState.ArenaStateSync", "Unexpected helper MemberRef owner")
    added_assembly = right.net.mdtables.AssemblyRef.rows[-1]
    require(str(added_assembly.Name) == HELPER_NAME, "Unexpected helper AssemblyRef")
    require(added_assembly.struct.Flags == 0 and added_assembly.PublicKey.value == b"",
            "Helper reference must have an empty PublicKeyToken without the PublicKey flag")

    for old_field, new_field in zip(left.net.mdtables.Field.rows, right.net.mdtables.Field.rows):
        require(str(old_field.Name) == str(new_field.Name)
                and old_field.Signature.value == new_field.Signature.value
                and old_field.struct.Flags == new_field.struct.Flags, "Original field ABI changed")
    for old_type, new_type in zip(left.net.mdtables.TypeDef.rows, right.net.mdtables.TypeDef.rows):
        require(str(old_type.TypeName) == str(new_type.TypeName)
                and str(old_type.TypeNamespace) == str(new_type.TypeNamespace)
                and old_type.struct.Flags == new_type.struct.Flags
                and old_type.struct.Extends_CodedIndex == new_type.struct.Extends_CodedIndex
                and old_type.struct.FieldList_Index == new_type.struct.FieldList_Index
                and old_type.struct.MethodList_Index == new_type.struct.MethodList_Index,
                "Original type ABI changed")
    require(left.net.struct.Flags == right.net.struct.Flags, "CLI flags changed")
    require(left.net.struct.StrongNameSignatureSize == right.net.struct.StrongNameSignatureSize == 0,
            "Unexpected strong-name signature")
    require(left.net.mdtables.Module.rows[0].Mvid.value == right.net.mdtables.Module.rows[0].Mvid.value,
            "Module MVID changed")

    def identity(row):
        return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion,
                row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)

    require([identity(row) for row in left.net.mdtables.Assembly.rows]
            == [identity(row) for row in right.net.mdtables.Assembly.rows], "Assembly identity changed")
    old_references = left.net.mdtables.AssemblyRef.rows
    new_references = right.net.mdtables.AssemblyRef.rows
    require([identity(row) for row in new_references[:len(old_references)]]
            == [identity(row) for row in old_references], "Original assembly references changed")

    pinned = {}
    for name, rid, method in old_methods:
        if not method.Rva:
            continue
        digest = sha(exact_body(left, method))
        for spec in TARGETS:
            if digest != spec["body"]:
                continue
            require(name == spec["type"] + "::" + spec["name"], "Pinned body belongs to an unexpected method")
            require(method.Signature.value.hex() == spec["signature"], "Target ABI differs: " + name)
            require(rid not in pinned, "Duplicate target pin: " + name)
            pinned[rid] = spec
    require(len(pinned) == len(TARGETS), "Target pin is not uniquely present")

    changes = []
    unchanged = engine = 0
    for (name, rid, old), (new_name, new_rid, new) in zip(old_methods, new_methods):
        require((name, rid) == (new_name, new_rid) and old.Signature.value == new.Signature.value
                and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags,
                "Original ABI/token changed: " + name)
        require(bool(old.Rva) == bool(new.Rva), "Original body presence changed: " + name)
        if not old.Rva:
            continue
        old_body, new_body = exact_body(left, old), exact_body(right, new)
        if rid not in pinned:
            require(old_body == new_body, "Unrelated original body changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            continue
        require(old_body != new_body, "Target body did not change: " + name)
        changes.append(verify_insertion(left, right, old, new, pinned[rid]))
    require(engine == ENGINE_METHODS, "Original engine method count differs")
    require(len(changes) == len(TARGETS), "Target count differs")
    helper = verify_helper(helper_raw)
    return {
        "changes": changes,
        "originalEngineMethodsUnchanged": engine,
        "unrelatedRawMethodBodiesUnchanged": unchanged,
        "originalMetadataABIAndTokensPreserved": True,
        "noOriginalTypesFieldsMethodsAdded": True,
        "originalAssemblyIdentityPreserved": True,
        "addedAssemblyReference": HELPER_NAME + " (flags 0, empty PublicKeyToken)",
        "addedMetadata": {"TypeRef": 1, "MemberRef": 3, "AssemblyRef": 1},
        "cliFlagsPreserved": True,
        "strongNameSignatureBytes": 0,
        "helper": helper,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True,
                        help="Explicit input APK; never modified")
    parser.add_argument("--expected-sha256", required=True,
                        help="Expected SHA-256 of --base-apk")
    parser.add_argument("--output-directory", type=Path, required=True,
                        help="Fresh directory for main DLL, helper DLL and manifest")
    parser.add_argument("--tool-root", type=Path, default=ROOT,
                        help="Project root holding .tools/native-batch (read-only; defaults to this project)")
    args = parser.parse_args()

    expected = args.expected_sha256.strip().lower()
    require(len(expected) == 64 and all(c in "0123456789abcdef" for c in expected),
            "--expected-sha256 must be 64 lowercase hex characters")
    base = args.base_apk.resolve()
    require(base.is_file(), "Base APK not found")
    require(sha(base.read_bytes()) == expected, "Base APK SHA-256 differs from --expected-sha256; refusing input")
    output = args.output_directory.resolve()
    require(output.is_relative_to(ROOT), "Output must stay inside the project for package_native payload paths")
    require(not output.exists(), "Fresh output directory required; existing overlay is never overwritten")
    output.mkdir(parents=True)

    tool_root = args.tool_root.resolve()
    runtime = tool_root / ".tools/native-batch"
    dotnet = runtime / "dotnet/dotnet"
    compiler = next((runtime / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"), None)
    dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"
    require(dotnet.is_file() and compiler is not None and compiler.is_file() and dnlib.is_file(),
            "Pinned dotnet/dnlib toolchain missing under --tool-root " + str(tool_root))
    helper_source = SOURCE / "ArenaState.cs"
    injector_source = SOURCE / "Injector.cs"

    def run(name, command):
        result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=ROOT,
                                capture_output=True, text=True, timeout=180)
        (output / (name + ".log")).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + " failed; inspect saved log")

    with zipfile.ZipFile(base) as archive:
        require(len(archive.namelist()) == len(set(archive.namelist())), "Duplicate base entries")
        before = archive.read(ENTRY)
        dex = sha(archive.read("classes2.dex"))
        inherited = {name: sha(archive.read(name)) for name in archive.namelist()
                     if name.endswith((".dex", ".class", ".jar"))
                     or (name.startswith("lib/") and name.endswith(".so"))}
        references = [ENTRY, "assets/bin/Data/Managed/mscorlib.dll",
                      "assets/bin/Data/Managed/UnityEngine.dll"]
        with tempfile.TemporaryDirectory(prefix="hyjs-arena-state-") as temporary:
            work = Path(temporary)
            host = work / "Assembly-CSharp.dll"
            host.write_bytes(before)
            for name in references:
                (work / Path(name).name).write_bytes(archive.read(name))
            helper_path = output / (HELPER_NAME + ".dll")
            run("compile-helper", [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+",
                                   "/langversion:3", "/deterministic+", "/codepage:65001",
                                   "/out:" + str(helper_path),
                                   *["/reference:" + str(work / Path(name).name) for name in references],
                                   helper_source])
            shutil.copy2(injector_source, work / "Program.cs")
            project = work / "Patch.csproj"
            project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
            run("compile-injector", [dotnet, "build", project, "--nologo", "--verbosity", "quiet",
                                     "-p:NuGetAudit=false"])
            run("inject", [dotnet, work / "bin/Debug/net8.0/Patch.dll", host, helper_path,
                           output / "Assembly-CSharp.dll"])
    helper_raw = (output / (HELPER_NAME + ".dll")).read_bytes()
    after = (output / "Assembly-CSharp.dll").read_bytes()
    proof = verify(before, after, helper_raw)
    require(sha(base.read_bytes()) == expected, "Base APK changed during the build")

    payload_files = {entry: str((output / Path(entry).name).relative_to(ROOT))
                     for entry in (ENTRY, HELPER_ENTRY)}
    for entry, path in payload_files.items():
        require((ROOT / path).is_file() and sha((ROOT / path).read_bytes())
                == (sha(after) if entry == ENTRY else sha(helper_raw)), "Overlay payload differs")
    sources = [Path(__file__).resolve(), helper_source, injector_source]
    report = {
        "schema": "hyjs-client-arena-state-sync-v1",
        "status": "STATIC_VERIFIED_NOT_PACKAGED",
        "managedOnly": True,
        "baseAPK": str(base.relative_to(ROOT)) if base.is_relative_to(ROOT) else str(base),
        "baseAPKSHA256": expected,
        "inputDexSHA256": dex,
        "inputManagedSHA256": sha(before),
        "analyzedManagedSHA256": EXPECTED_MANAGED_SHA256,
        "outputManagedSHA256": sha(after),
        "helperSHA256": sha(helper_raw),
        "inheritedJavaByteExact": True,
        "inheritedJavaPublicProtectedABIExact": True,
        "inheritedJavaEntrySHA256": inherited,
        "javaProof": "Payload has only two managed DLL entries. Every original Java/Dex/native-library entry is inherited unchanged; package_native copies and re-verifies each inherited entry.",
        "payloadFiles": payload_files,
        "replacementEntries": {ENTRY: sha(after), HELPER_ENTRY: sha(helper_raw)},
        "allowedChangedMethods": [spec["type"] + "::" + spec["name"] for spec in TARGETS],
        "targetPins": {(spec["type"] + "::" + spec["name"]): {"signature": spec["signature"],
                                                              "bodySHA256": spec["body"]}
                       for spec in TARGETS},
        "sourcePins": {str(path.relative_to(ROOT)): sha(path.read_bytes()) for path in sources},
        "packagingCommand": "tools/client_release/package_native.py --base-apk <this baseAPK> --overlay "
                            + str(output.relative_to(ROOT)),
        "deviceExecuted": False,
        "visualVerified": False,
        "serverMutated": False,
        "resourceFilesModified": False,
        "knownLimits": [
            "Only the three pinned bodies change; every other original body, including all 547 Battle.Engine methods, stays byte-identical.",
            "The grab cache is updated from the same server frame SetItem already rendered; no target, position, timer, cost or reward is authored on the client.",
            "A successful empty rival list shows the original text popup; the server still publishes only real eligible holders and never fabricates robots.",
            "DoOpen hides the prefab slots before the original 0.5s request; a real infoList still drives the original display and selection.",
        ],
        "requiredDeviceChecks": [
            "Occupy an empty point in one grab field, switch to another field and back: the occupied slot keeps the player name and timer without a manual refresh.",
            "Give up that point, then switch fields and back: the slot is empty again and the original abandon/grab buttons match the server state.",
            "Open the fragment selection page: no prefab portraits appear during the 0.5s delay; real rivals show as before.",
            "With no eligible rival, the page stays empty and the notice appears; a failed request shows the original error instead of the empty notice.",
            "Close the selection page before the reply: no late notice appears on another page.",
        ],
        **proof,
    }
    (output / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: report[key] for key in ("status", "baseAPKSHA256", "outputManagedSHA256",
                                                   "helperSHA256", "originalEngineMethodsUnchanged",
                                                   "unrelatedRawMethodBodiesUnchanged")},
                     ensure_ascii=False))


if __name__ == "__main__":
    main()
