"""Build the bounded type12 field-boss settlement recovery overlay.

The original `Battle.FieldPVEBossBattleOver.AskFieldPVEFightResult` registers
opcode 30086 once, sends opcode 30085 once, and then waits for the server
callback forever. This build adds two hooks only:

  * after the original SendSocket(30085, request), `LocalFieldSettlement.Observe`
    freezes that exact request and re-sends it with a bounded wait;
  * at the first instruction of the original success handler,
    `LocalFieldSettlement.Completed` stops the wait.

The original send, registration, handler body, battle algorithm and callback are
preserved. The script reads an explicitly SHA-256 pinned APK, compiles a helper
and a dnlib injector, injects the two hooks, verifies every other method body
byte-for-byte (including 547 Battle.Engine methods), and writes a
package_native-compatible managed overlay. It never packages, installs,
launches the game, edits resources or touches a device.
"""
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
from tools.awake_diff_rows.build import methods  # noqa: E402
from tools.local_asset_ready.remove_gate import exact_body, require, sha  # noqa: E402

SOURCE = Path(__file__).resolve().parent
ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
HELPER_NAME = "LocalFieldSettlement"
HELPER_ENTRY = "assets/bin/Data/Managed/" + HELPER_NAME + ".dll"
TARGET_ASK = "Battle.FieldPVEBossBattleOver::AskFieldPVEFightResult"
TARGET_HANDLER = "Battle.FieldPVEBossBattleOver::FieldPVEBossFightResultHandler"
ALLOWED_TARGETS = (TARGET_ASK, TARGET_HANDLER)
REFERENCE_ENTRIES = [
    ENTRY,
    "assets/bin/Data/Managed/Assembly-CSharp-firstpass.dll",
    "assets/bin/Data/Managed/UnityEngine.dll",
    "assets/bin/Data/Managed/mscorlib.dll",
    "assets/bin/Data/Managed/System.dll",
]
HELPER_REFERENCES = {"mscorlib", "Assembly-CSharp", "Assembly-CSharp-firstpass", "UnityEngine"}


def decode(pe, method):
    return list(read_method_body_from_bytes(exact_body(pe, method)).instructions)


def operand(instruction):
    value = getattr(instruction.operand, "value", instruction.operand)
    index = getattr(value, "index", None)
    return ("local", index) if index is not None else value


def same_instruction(before, after):
    return before.opcode.name == after.opcode.name and operand(before) == operand(after)


def method_name(pe, token):
    if not isinstance(token, int):
        return ""
    table, rid = token >> 24, token & 0xFFFFFF
    if table == 0x06:
        return str(pe.net.mdtables.MethodDef.rows[rid - 1].Name)
    if table == 0x0A:
        return str(pe.net.mdtables.MemberRef.rows[rid - 1].Name)
    return ""


def type_full_name(pe, table, rid):
    rows = pe.net.mdtables.TypeDef.rows if table == 0x02 else pe.net.mdtables.TypeRef.rows
    row = rows[rid - 1]
    namespace = str(row.TypeNamespace)
    name = str(row.TypeName)
    return namespace + "." + name if namespace else name


def member_owner(pe, token):
    reference = pe.net.mdtables.MemberRef.rows[(token & 0xFFFFFF) - 1]
    owner = reference.Class.row
    namespace = str(owner.TypeNamespace)
    name = str(owner.TypeName)
    return namespace + "." + name if namespace else name


def helper_reference(pe, token):
    require(isinstance(token, int) and token >> 24 == 0x0A, "Helper hook is not a MemberRef")
    reference = pe.net.mdtables.MemberRef.rows[(token & 0xFFFFFF) - 1]
    owner = reference.Class.row
    require(str(owner.TypeName) == HELPER_NAME, "Unexpected helper member owner")
    require(str(owner.ResolutionScope.row.Name) == HELPER_NAME, "Unexpected helper member scope")
    return str(reference.Name)


def handlers(body):
    return [(item.try_start, item.try_end, item.handler_start, item.handler_end)
            for item in body.exception_handlers]


def verify_ask(left, right, old, new):
    old_body, new_body = read_method_body_from_bytes(exact_body(left, old)), \
        read_method_body_from_bytes(exact_body(right, new))
    old_instructions, new_instructions = list(old_body.instructions), list(new_body.instructions)
    require(len(new_instructions) == len(old_instructions) + 3, "AskFieldPVEFightResult hook count differs")
    send = [index for index, instruction in enumerate(old_instructions)
            if instruction.opcode.name == "call"
            and method_name(left, operand(instruction)) == "SendSocket"]
    require(len(send) == 1, "Original SendSocket site is not unique")
    send = send[0]
    require(send >= 3 and send + 2 == len(old_instructions), "Original SendSocket is not the final request")
    require(old_instructions[send - 2].opcode.name == "ldc.i4"
            and operand(old_instructions[send - 2]) == 30085, "Original send opcode changed")
    require(old_instructions[send - 1].opcode.name == "ldloc.0", "Original request local changed")
    require(old_instructions[send + 1].opcode.name == "ret", "Original send is not followed by ret")
    for index in range(send + 1):
        require(same_instruction(old_instructions[index], new_instructions[index]),
                "AskFieldPVEFightResult changed before the hook")
    injected = new_instructions[send + 1:send + 4]
    require([item.opcode.name for item in injected] == ["ldarg.0", "ldloc.0", "call"],
            "Unexpected AskFieldPVEFightResult hook shape")
    require(helper_reference(right, operand(injected[2])) == "Observe", "AskFieldPVEFightResult hook is not Observe")
    for before, after in zip(old_instructions[send + 1:], new_instructions[send + 4:]):
        require(same_instruction(before, after), "AskFieldPVEFightResult tail changed")
    require(handlers(old_body) == handlers(new_body), "AskFieldPVEFightResult exception boundaries changed")
    return {
        "method": TARGET_ASK,
        "signatureHex": old.Signature.value.hex(),
        "insertedAfterOffset": old_instructions[send].offset,
        "insertedOpcodes": ["ldarg.0", "ldloc.0", "call LocalFieldSettlement::Observe(object, object)"],
        "originalBodySHA256": sha(exact_body(left, old)),
        "patchedBodySHA256": sha(exact_body(right, new)),
        "instructionsBefore": len(old_instructions),
        "instructionsAfter": len(new_instructions),
        "exceptionHandlersPreserved": len(old_body.exception_handlers),
    }


def verify_handler(left, right, old, new):
    old_body, new_body = read_method_body_from_bytes(exact_body(left, old)), \
        read_method_body_from_bytes(exact_body(right, new))
    old_instructions, new_instructions = list(old_body.instructions), list(new_body.instructions)
    require(len(new_instructions) == len(old_instructions) + 3, "Result handler hook count differs")
    require(old_instructions[0].opcode.name == "ldarg.1"
            and old_instructions[1].opcode.name == "castclass", "Original result handler entry changed")
    injected = new_instructions[:3]
    require([item.opcode.name for item in injected] == ["ldarg.0", "ldarg.1", "call"],
            "Unexpected result handler hook shape")
    require(helper_reference(right, operand(injected[2])) == "Completed", "Result handler hook is not Completed")
    for before, after in zip(old_instructions, new_instructions[3:]):
        require(same_instruction(before, after), "Result handler body changed")
    return {
        "method": TARGET_HANDLER,
        "signatureHex": old.Signature.value.hex(),
        "insertedBeforeOffset": old_instructions[0].offset,
        "insertedOpcodes": ["ldarg.0", "ldarg.1", "call LocalFieldSettlement::Completed(object, object)"],
        "originalBodySHA256": sha(exact_body(left, old)),
        "patchedBodySHA256": sha(exact_body(right, new)),
        "instructionsBefore": len(old_instructions),
        "instructionsAfter": len(new_instructions),
        "exceptionHandlersPreserved": len(old_body.exception_handlers),
    }


def verify_helper(raw):
    pe = dnfile.dnPE(data=raw)
    version = bytes(pe.net.metadata.struct.Version).rstrip(b"\x00")
    require(version == b"v2.0.50727", "Helper must target the old Unity runtime")
    assembly = pe.net.mdtables.Assembly.rows
    require(len(assembly) == 1 and str(assembly[0].Name) == HELPER_NAME, "Unexpected helper assembly identity")
    references = {str(row.Name) for row in pe.net.mdtables.AssemblyRef.rows}
    require(HELPER_REFERENCES <= references, "Helper dependency set changed: " + repr(sorted(references)))
    require(references <= HELPER_REFERENCES | {"System", "System.Core"},
            "Unexpected helper dependency: " + repr(sorted(references - HELPER_REFERENCES)))
    types = [row for row in pe.net.mdtables.TypeDef.rows
             if str(row.TypeName) != "<Module>" and (row.struct.Flags & 7) == 1]
    require(len(types) == 1, "Helper must define exactly one top-level type")
    require(str(types[0].TypeName) == HELPER_NAME and str(types[0].TypeNamespace) == "",
            "Unexpected helper type identity")
    definitions = [entry.row for entry in types[0].MethodList]
    names = sorted(str(method.Name) for method in definitions)
    public_hooks = [method for method in definitions if method.struct.Flags & 7 == 6
                    and method.struct.Flags & 0x0010]
    require(sorted(str(method.Name) for method in public_hooks) == ["Completed", "Observe"],
            "Helper must expose only the two static hooks")
    for method in public_hooks:
        require(method.struct.Flags & 0x0010, "Helper hooks must be static")
        require(method.Signature.value.hex() == "0002011c1c", "Helper hook signature changed")
        instructions = decode(pe, method)
        for instruction in instructions:
            name = method_name(pe, operand(instruction))
            require(name not in ("End", "Invoke", "set_isLastBattleWin", "set_lastBattleEndMsg"),
                    "Helper must not fake a result or invoke the original callback")
        require(any(instruction.opcode.name == "ret" for instruction in instructions), "Helper hook lacks ret")
    return {
        "assembly": HELPER_NAME,
        "runtime": version.decode(),
        "references": sorted(references),
        "definedTypes": [HELPER_NAME],
        "definedMethods": names,
    }


def verify(before, after, helper_raw):
    left, right = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    old_methods, new_methods = methods(left), methods(right)
    require(len(old_methods) == len(new_methods), "Method inventory changed")
    for table in ("TypeDef", "Field", "MethodDef", "Param", "Property", "Event",
                  "TypeSpec", "MethodSpec", "ModuleRef"):
        require(len(getattr(left.net.mdtables, table).rows) == len(getattr(right.net.mdtables, table).rows),
                "Original metadata inventory changed: " + table)
    for old_type, new_type in zip(left.net.mdtables.TypeDef.rows, right.net.mdtables.TypeDef.rows):
        require(str(old_type.TypeName) == str(new_type.TypeName)
                and str(old_type.TypeNamespace) == str(new_type.TypeNamespace)
                and old_type.struct.Flags == new_type.struct.Flags
                and old_type.struct.Extends_CodedIndex == new_type.struct.Extends_CodedIndex
                and old_type.struct.FieldList_Index == new_type.struct.FieldList_Index
                and old_type.struct.MethodList_Index == new_type.struct.MethodList_Index,
                "Original type ABI changed")
    for old_field, new_field in zip(left.net.mdtables.Field.rows, right.net.mdtables.Field.rows):
        require(str(old_field.Name) == str(new_field.Name)
                and old_field.Signature.value == new_field.Signature.value
                and old_field.struct.Flags == new_field.struct.Flags, "Original field ABI changed")
    require(left.net.struct.Flags == right.net.struct.Flags, "CLI flags changed")
    require(left.net.struct.StrongNameSignatureSize == right.net.struct.StrongNameSignatureSize == 0,
            "Unexpected strong-name signature")

    def identity(row):
        return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion,
                row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)

    require([identity(row) for row in left.net.mdtables.Assembly.rows]
            == [identity(row) for row in right.net.mdtables.Assembly.rows], "Assembly identity changed")
    old_references = left.net.mdtables.AssemblyRef.rows
    new_references = right.net.mdtables.AssemblyRef.rows
    require(len(new_references) == len(old_references) + 1, "Assembly reference count changed")
    require([identity(row) for row in new_references[:len(old_references)]]
            == [identity(row) for row in old_references], "Original assembly references changed")
    require(str(new_references[-1].Name) == HELPER_NAME, "Unexpected added assembly reference")
    require(new_references[-1].struct.Flags == 0 and new_references[-1].PublicKey.value == b"",
            "Helper reference must have an empty PublicKeyToken without the PublicKey flag")

    old_type_refs = left.net.mdtables.TypeRef.rows
    new_type_refs = right.net.mdtables.TypeRef.rows
    require(len(new_type_refs) == len(old_type_refs) + 1, "TypeRef count changed")
    for one, two in zip(old_type_refs, new_type_refs[:len(old_type_refs)]):
        require((str(one.TypeNamespace), str(one.TypeName)) == (str(two.TypeNamespace), str(two.TypeName)),
                "Original TypeRef rows changed")
    require(str(new_type_refs[-1].TypeName) == HELPER_NAME, "Unexpected added TypeRef")
    require(str(new_type_refs[-1].ResolutionScope.row.Name) == HELPER_NAME, "Helper TypeRef scope changed")
    old_member_refs = left.net.mdtables.MemberRef.rows
    new_member_refs = right.net.mdtables.MemberRef.rows
    require(len(new_member_refs) == len(old_member_refs) + 2, "MemberRef count changed")
    for one, two in zip(old_member_refs, new_member_refs[:len(old_member_refs)]):
        require(str(one.Name) == str(two.Name) and one.Signature.value == two.Signature.value,
                "Original MemberRef rows changed")
    added = new_member_refs[len(old_member_refs):]
    require({str(row.Name) for row in added} == {"Observe", "Completed"}, "Unexpected helper member names")
    for row in added:
        require(str(row.Class.row.TypeName) == HELPER_NAME, "Helper metadata points at the wrong type")

    changed = []
    unchanged = engine = 0
    expected_engine = sum(1 for name, _, row in old_methods
                          if name.startswith("Battle.Engine") and row.Rva)
    for (name, rid, old), (new_name, new_rid, new) in zip(old_methods, new_methods):
        require((name, rid) == (new_name, new_rid) and old.Signature.value == new.Signature.value
                and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags,
                "Original ABI/token changed: " + name)
        require(bool(old.Rva) == bool(new.Rva), "Original body presence changed: " + name)
        if not old.Rva:
            continue
        old_raw, new_raw = exact_body(left, old), exact_body(right, new)
        if name not in ALLOWED_TARGETS:
            require(old_raw == new_raw, "Unrelated original body changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            continue
        require(old_raw != new_raw, "Target body did not change: " + name)
        if name == TARGET_ASK:
            changed.append(verify_ask(left, right, old, new))
        else:
            changed.append(verify_handler(left, right, old, new))
        engine += name.startswith("Battle.Engine")
    require(engine == expected_engine, "Original engine method count differs")
    require([item["method"] for item in changed] == list(ALLOWED_TARGETS), "Target count differs")
    return {
        "changes": changed,
        "originalEngineMethodsUnchanged": engine,
        "unrelatedRawMethodBodiesUnchanged": unchanged,
        "originalMetadataABIAndTokensPreserved": True,
        "noOriginalTypesFieldsMethodsAdded": True,
        "originalAssemblyIdentityPreserved": True,
        "addedAssemblyReference": HELPER_NAME + " (flags 0, empty PublicKeyToken)",
        "addedMetadata": {"TypeRef": 1, "MemberRef": 2, "AssemblyRef": 1},
        "cliFlagsPreserved": True,
        "strongNameSignatureBytes": 0,
        "helper": verify_helper(helper_raw),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True, help="Explicit input APK; never modified")
    parser.add_argument("--base-sha256", "--expected-sha256", dest="base_sha256", required=True,
                        help="Expected SHA-256 of --base-apk")
    parser.add_argument("--output-directory", type=Path, required=True,
                        help="Fresh directory for main DLL, helper DLL and manifest")
    parser.add_argument("--tool-root", type=Path, default=ROOT / ".tools/native-batch",
                        help="Read-only dotnet/csc/dnlib toolchain root")
    args = parser.parse_args()

    expected = args.base_sha256.strip().lower()
    require(len(expected) == 64 and all(item in "0123456789abcdef" for item in expected),
            "--base-sha256 must be 64 lowercase hex characters")
    base = args.base_apk.resolve()
    require(base.is_file(), "Base APK not found")
    require(sha(base.read_bytes()) == expected, "Base APK SHA-256 differs from --base-sha256; refusing input")
    output = args.output_directory.resolve()
    require(output.is_relative_to(ROOT), "Output must stay inside the project for package_native payload paths")
    require(not output.exists(), "Fresh output directory required; existing overlay is never overwritten")
    output.mkdir(parents=True)

    runtime = args.tool_root.resolve()
    dotnet = runtime / "dotnet/dotnet"
    compiler = next((runtime / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"
    require(dotnet.is_file() and compiler.is_file() and dnlib.is_file(),
            "Local pinned dotnet/dnlib toolchain missing")
    helper_source = SOURCE / "LocalFieldSettlement.cs"
    injector_source = SOURCE / "Injector.cs"

    def run(name, command):
        result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=ROOT,
                                capture_output=True, text=True, timeout=240)
        (output / (name + ".log")).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + " failed; inspect saved log")

    with zipfile.ZipFile(base) as archive:
        require(len(archive.namelist()) == len(set(archive.namelist())), "Duplicate base entries")
        before = archive.read(ENTRY)
        dex = sha(archive.read("classes2.dex"))
        inherited = {name: sha(archive.read(name)) for name in archive.namelist()
                     if name.endswith((".dex", ".class", ".jar"))
                     or (name.startswith("lib/") and name.endswith(".so"))}
        for entry in REFERENCE_ENTRIES:
            require(entry in archive.namelist(), "Missing managed reference in base APK: " + entry)
        with tempfile.TemporaryDirectory(prefix="hyjs-field-settlement-") as temporary:
            temporary = Path(temporary)
            host = temporary / "Assembly-CSharp.dll"
            host.write_bytes(before)
            for entry in REFERENCE_ENTRIES:
                (temporary / Path(entry).name).write_bytes(archive.read(entry))
            helper_path = output / (HELPER_NAME + ".dll")
            run("compile-helper", [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+",
                                   "/langversion:3", "/deterministic+", "/out:" + str(helper_path),
                                   *["/reference:" + str(temporary / Path(entry).name) for entry in REFERENCE_ENTRIES],
                                   helper_source])
            shutil.copy2(injector_source, temporary / "Program.cs")
            project = temporary / "Patch.csproj"
            project.write_text(
                '<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'
                + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
            run("compile-injector", [dotnet, "build", project, "--nologo", "--verbosity", "quiet",
                                     "-p:NuGetAudit=false"])
            run("inject", [dotnet, temporary / "bin/Debug/net8.0/Patch.dll", host, helper_path,
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
    target_pins = {item["method"]: {
        "signature": item["signatureHex"],
        "bodySHA256": item["originalBodySHA256"],
        "patchedBodySHA256": item["patchedBodySHA256"],
    } for item in proof["changes"]}
    report = {
        "schema": "hyjs-client-field-boss-settlement-recovery-v1",
        "status": "STATIC_VERIFIED_NOT_PACKAGED",
        "managedOnly": True,
        "baseAPK": str(base.relative_to(ROOT)) if base.is_relative_to(ROOT) else str(base),
        "baseAPKSHA256": expected,
        "inputDexSHA256": dex,
        "inputManagedSHA256": sha(before),
        "outputManagedSHA256": sha(after),
        "helperSHA256": sha(helper_raw),
        "inheritedJavaByteExact": True,
        "inheritedJavaPublicProtectedABIExact": True,
        "inheritedJavaEntrySHA256": inherited,
        "javaProof": "Payload has only two managed DLL entries. Every original Java/Dex/native-library entry is inherited unchanged; package_native copies and re-verifies each inherited entry.",
        "payloadFiles": payload_files,
        "replacementEntries": {ENTRY: sha(after), HELPER_ENTRY: sha(helper_raw)},
        "allowedChangedMethods": list(ALLOWED_TARGETS),
        "targetPins": target_pins,
        "sourcePins": {str(path.relative_to(ROOT)): sha(path.read_bytes()) for path in sources},
        "packagingCommand": "tools/client_release/package_native.py --base-apk <this baseAPK> --overlay "
                            + str(output.relative_to(ROOT)) + " --output <new apk> --record <json>",
        "deviceExecuted": False,
        "visualVerified": False,
        "serverMutated": False,
        "resourceFilesModified": False,
        "knownLimits": [
            "Only the type12 field-boss settlement path is covered; the separate type18 world-boss bridge is not part of this overlay.",
            "Resend relies on the original 30086 listener staying registered across a socket reconnect. SocketController.ReConnect reuses the same EventManger, but no device run has confirmed recovery against the restarted server.",
            "A server that keeps failing can still leave the battle scene waiting. The dialog offers retry or return to login and never fabricates a win, damage, reward or calls Battle.FieldPVEBossBattleOver.End.",
            "The dialog uses BoxManager.CreateMessageBoxWithCofirmAndCancelButton and prefab UI/MessageBox/MessageBoxWithConfirmAndCancelButton; this wrapper has no other call site in Assembly-CSharp, so its device appearance is covered by the required checks.",
            "No dex, native library or resource entry is changed; only the two managed DLL entries are replaced.",
        ],
        "requiredDeviceChecks": [
            "Interrupt the server during a type12 boss end request: the battle result does not stay permanently stuck, and after the server returns the same request is re-sent and the original result page proceeds exactly once.",
            "Force three failed wait windows: the original-style box shows '首领结算暂未完成'; confirm re-sends the same request and cancel returns to login.",
            "A normal type12 settlement completes without showing the box and still applies the original result, HP and reward data.",
            "Switching account or leaving the battle scene while waiting cancels the retry and destroys only this adapter's box.",
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
