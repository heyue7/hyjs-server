"""Null-safe pet binding for the viewed player's formation card.

Redirects the single pinned `newobj UI.UIPet::.ctor(PetSimpleInfo)` inside
UI.UIHeroFormation.InitData(FormationItemSimpleInfo) to a static helper that
returns null when petInfo is absent and still builds the original UIPet when it
is present. Everything else in InitData, the combat/attribute calculation, the
own-formation path and the real owned-pet data stay original.

The main DLL is copied from an explicitly SHA-256 pinned APK, one instruction is
rewritten, and a package_native-compatible managed overlay is written. This
never packages, installs, launches the game, edits resources or touches a
device, and it refuses to overwrite an existing overlay directory.
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
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require

SOURCE = Path(__file__).resolve().parent
ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
HELPER_NAME = "SoulFlowerChatProfile"
HELPER_ENTRY = "assets/bin/Data/Managed/" + HELPER_NAME + ".dll"
TARGET = "UI.UIHeroFormation::InitData"
# Instance method with exactly one ProtoBuf.Message.FormationItemSimpleInfo parameter.
TARGET_SIGNATURE = "2001011281c4"
# Raw body of that method, identical in build/hyjs-b31-r5.apk and build/hyjs-b33-local-qa.apk.
TARGET_BODY_SHA256 = "a5a35cadd296a3e41e3ffa274f8b2aa47c7f05af37034599ab2cd0498b379fe4"
ENGINE_METHODS = 547
HELPER_REFERENCES = ["Assembly-CSharp", "mscorlib"]


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def decode(pe, method):
    return list(read_method_body_from_bytes(exact_body(pe, method)).instructions)


def instruction_value(instruction):
    operand = instruction.operand
    if operand is None:
        return None
    value = getattr(operand, "value", operand)
    index = getattr(value, "index", None)
    return ("local", index) if index is not None else value


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
        require(signature[position] == 0x12, "Helper signature must use class types only")
        coded, position = read_compressed(signature, position + 1)
        tag = coded & 0x03
        require(tag in (0x00, 0x01), "Unexpected helper signature type encoding")
        names.append(type_full_name(pe, 0x02 if tag == 0x00 else 0x01, coded >> 2))
    require(position == len(signature), "Trailing helper signature bytes")
    return names[0], names[1:]


def verify_target(left, right, old, new):
    old_raw, new_raw = exact_body(left, old), exact_body(right, new)
    require(sha(old_raw) == TARGET_BODY_SHA256, "Pinned InitData body differs")
    require(len(old_raw) == len(new_raw), "Target body size changed")
    old_body = read_method_body_from_bytes(old_raw)
    new_body = read_method_body_from_bytes(new_raw)
    old_instructions, new_instructions = list(old_body.instructions), list(new_body.instructions)
    require(len(old_instructions) == len(new_instructions), "Target instruction count changed")
    site = None
    for index, (before, after) in enumerate(zip(old_instructions, new_instructions)):
        require(before.offset == after.offset and before.size == after.size,
                "Target instruction offset/size changed")
        if before.opcode.name == after.opcode.name:
            require(instruction_value(before) == instruction_value(after),
                    "Unrelated target operand changed")
            continue
        require(site is None, "More than one target instruction changed")
        require(before.opcode.name == "newobj" and after.opcode.name == "call",
                "Unexpected target instruction rewrite")
        site = index
    require(site is not None, "Target instruction was not rewritten")
    old_site, new_site = old_instructions[site], new_instructions[site]
    old_token = instruction_value(old_site)
    require(method_name(left, old_token) == ".ctor", "Original site is not a constructor")
    require(old_token >> 24 == 0x06, "Original site must reference the original UIPet constructor")
    require(site >= 3 and site + 1 < len(old_instructions), "Target site context missing")
    require(old_instructions[site - 3].opcode.name == "ldarg.0", "Pet binding does not start at ldarg.0")
    require(old_instructions[site - 2].opcode.name == "ldarg.1", "Pet binding argument slot changed")
    require(old_instructions[site - 1].opcode.name == "callvirt"
            and method_name(left, instruction_value(old_instructions[site - 1])) == "get_petInfo",
            "Pet binding getter changed")
    require(old_instructions[site + 1].opcode.name == "stfld"
            and field_name(left, instruction_value(old_instructions[site + 1])) == "pet",
            "Pet binding store changed")

    token = instruction_value(new_site)
    require(isinstance(token, int) and token >> 24 == 0x0A, "Rewritten call is not a MemberRef")
    reference = right.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1]
    require(str(reference.Name) == "Create", "Unexpected helper method name")
    owner = reference.Class.row
    require(str(owner.TypeNamespace) == "HYJS.ChatProfile" and str(owner.TypeName) == "ProfilePet",
            "Unexpected helper type")
    require(str(owner.ResolutionScope.row.Name) == HELPER_NAME, "Unexpected helper assembly")
    returns, parameters = signature_types(right, reference.Signature.value)
    require(returns == "UI.UIPet", "Helper must return the original UIPet")
    require(parameters == ["ProtoBuf.Message.PetSimpleInfo"], "Helper parameter must stay PetSimpleInfo")

    position = old_site.offset
    differences = [index for index, (before, after) in enumerate(zip(old_raw, new_raw)) if before != after]
    require(differences, "Target body bytes are unchanged")
    require(set(differences) <= set(range(position, position + 5)),
            "Target body changed outside the single instruction")
    require(old_raw[position] == 0x73 and new_raw[position] == 0x28,
            "Target opcode is not newobj -> call")
    old_handlers = old_body.exception_handlers
    new_handlers = new_body.exception_handlers
    require(len(old_handlers) == len(new_handlers), "Target exception handler count changed")
    for before, after in zip(old_handlers, new_handlers):
        require((before.try_start, before.try_end, before.handler_start, before.handler_end)
                == (after.try_start, after.try_end, after.handler_start, after.handler_end),
                "Target exception boundary changed")
    return {
        "method": TARGET,
        "siteOffset": old_site.offset,
        "beforeOpcode": "newobj UI.UIPet::.ctor(PetSimpleInfo)",
        "afterOpcode": "call HYJS.ChatProfile.ProfilePet::Create(PetSimpleInfo)",
        "beforeRawSHA256": sha(old_raw),
        "afterRawSHA256": sha(new_raw),
        "changedBodyBytes": len(differences),
        "instructionsBefore": len(old_instructions),
        "instructionsAfter": len(new_instructions),
        "exceptionHandlersPreserved": len(old_handlers),
    }


def verify_helper(raw):
    pe = dnfile.dnPE(data=raw)
    version = bytes(pe.net.metadata.struct.Version).rstrip(b"\x00")
    require(version == b"v2.0.50727", "Helper must target the old Unity runtime")
    assembly = pe.net.mdtables.Assembly.rows
    require(len(assembly) == 1 and str(assembly[0].Name) == HELPER_NAME, "Unexpected helper assembly identity")
    references = sorted(str(row.Name) for row in pe.net.mdtables.AssemblyRef.rows)
    require(references == HELPER_REFERENCES, "Unexpected helper dependency set")
    types = [row for row in pe.net.mdtables.TypeDef.rows if str(row.TypeName) != "<Module>"]
    require(len(types) == 1, "Helper must define exactly one type")
    require(str(types[0].TypeNamespace) == "HYJS.ChatProfile" and str(types[0].TypeName) == "ProfilePet",
            "Unexpected helper type identity")
    definitions = [entry.row for entry in types[0].MethodList]
    require([str(method.Name) for method in definitions] == ["Create"],
            "Helper must define only Create")
    create = definitions[0]
    returns, parameters = signature_types(pe, create.Signature.value)
    require(returns == "UI.UIPet" and parameters == ["ProtoBuf.Message.PetSimpleInfo"],
            "Unexpected helper Create signature")
    require(create.struct.Flags & 0x0010, "Helper Create must be static")
    instructions = decode(pe, create)
    constructors = [instruction for instruction in instructions
                    if instruction.opcode.name == "newobj"
                    or (instruction.opcode.name in ("call", "callvirt")
                        and method_name(pe, instruction_value(instruction)) not in ("", ".ctor"))]
    require([instruction.opcode.name for instruction in constructors] == ["newobj"],
            "Helper must only construct the original UIPet")
    require(method_name(pe, instruction_value(constructors[0])) == ".ctor", "Unexpected helper construction")
    require(any(instruction.opcode.name == "ldnull" for instruction in instructions), "Helper lacks the null result")
    require(any(instruction.opcode.name in ("brtrue", "brfalse", "brtrue.s", "brfalse.s")
                for instruction in instructions), "Helper lacks the null test")
    require(sum(1 for instruction in instructions if instruction.opcode.name == "ret") >= 1, "Helper lacks ret")
    return {
        "assembly": HELPER_NAME,
        "runtime": version.decode(),
        "references": references,
        "definedTypes": [str(types[0].TypeNamespace) + "." + str(types[0].TypeName)],
        "definedMethods": [name for name in [str(method.Name) for method in definitions]],
        "createSignature": create.Signature.value.hex(),
        "createBodySHA256": sha(exact_body(pe, create)),
        "createInstructions": [instruction.opcode.name for instruction in instructions],
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
    for table, addition, attribute in (("TypeRef", "ProfilePet", "TypeName"),
                                       ("MemberRef", "Create", "Name"),
                                       ("AssemblyRef", HELPER_NAME, "Name")):
        expected = len(getattr(left.net.mdtables, table).rows) + 1
        require(len(getattr(right.net.mdtables, table).rows) == expected,
                "Unexpected " + table + " addition")
        require(str(getattr(getattr(right.net.mdtables, table).rows[-1], attribute)) == addition,
                "Unexpected " + table + " entry")
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

    def identity(row):
        return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion,
                row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)

    require([identity(row) for row in left.net.mdtables.Assembly.rows]
            == [identity(row) for row in right.net.mdtables.Assembly.rows], "Assembly identity changed")
    old_references = left.net.mdtables.AssemblyRef.rows
    new_references = right.net.mdtables.AssemblyRef.rows
    require([identity(row) for row in new_references[:len(old_references)]]
            == [identity(row) for row in old_references], "Original assembly references changed")
    require(new_references[-1].struct.Flags == 0 and new_references[-1].PublicKey.value == b"",
            "Helper reference must have an empty PublicKeyToken without the PublicKey flag")

    pinned = [(name, rid, method) for name, rid, method in old_methods
              if method.Rva and sha(exact_body(left, method)) == TARGET_BODY_SHA256]
    require(len(pinned) == 1, "Pinned target body is not uniquely present")
    target_name, target_rid, target_old = pinned[0]
    require(target_name == TARGET, "Pinned body belongs to an unexpected method")
    require(target_old.Signature.value.hex() == TARGET_SIGNATURE,
            "Target ABI is not InitData(FormationItemSimpleInfo)")

    changed = []
    unchanged = engine = 0
    for (name, rid, old), (new_name, new_rid, new) in zip(old_methods, new_methods):
        require((name, rid) == (new_name, new_rid) and old.Signature.value == new.Signature.value
                and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags,
                "Original ABI/token changed: " + name)
        require(bool(old.Rva) == bool(new.Rva), "Original body presence changed: " + name)
        if not old.Rva:
            continue
        old_body, new_body = exact_body(left, old), exact_body(right, new)
        if rid != target_rid:
            require(old_body == new_body, "Unrelated original body changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            continue
        require(old_body != new_body, "Target body did not change")
        changed.append(verify_target(left, right, old, new))
    require(engine == ENGINE_METHODS, "Original engine method count differs")
    require(len(changed) == 1, "Target count differs")
    helper = verify_helper(helper_raw)
    return {
        "changes": changed,
        "originalEngineMethodsUnchanged": engine,
        "unrelatedRawMethodBodiesUnchanged": unchanged,
        "originalMetadataABIAndTokensPreserved": True,
        "noOriginalTypesFieldsMethodsAdded": True,
        "originalAssemblyIdentityPreserved": True,
        "addedAssemblyReference": HELPER_NAME + " (flags 0, empty PublicKeyToken)",
        "addedMetadata": {"TypeRef": 1, "MemberRef": 1, "AssemblyRef": 1},
        "cliFlagsPreserved": True,
        "strongNameSignatureBytes": 0,
        "helper": helper,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True,
                        help="Explicit input APK; never modified")
    parser.add_argument("--base-sha256", required=True,
                        help="Expected SHA-256 of --base-apk")
    parser.add_argument("--output-directory", type=Path, required=True,
                        help="Fresh directory for main DLL, helper DLL and manifest")
    args = parser.parse_args()

    expected = args.base_sha256.strip().lower()
    require(len(expected) == 64 and all(c in "0123456789abcdef" for c in expected),
            "--base-sha256 must be 64 lowercase hex characters")
    base = args.base_apk.resolve()
    require(base.is_file(), "Base APK not found")
    require(sha(base.read_bytes()) == expected, "Base APK SHA-256 differs from --base-sha256; refusing input")
    output = args.output_directory.resolve()
    require(output.is_relative_to(ROOT), "Output must stay inside the project for package_native payload paths")
    require(not output.exists(), "Fresh output directory required; existing overlay is never overwritten")
    output.mkdir(parents=True)

    runtime = ROOT / ".tools/native-batch"
    dotnet = runtime / "dotnet/dotnet"
    compiler = next((runtime / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"
    require(dotnet.is_file() and compiler.is_file() and dnlib.is_file(),
            "Local pinned dotnet/dnlib toolchain missing")
    helper_source = SOURCE / "ProfilePet.cs"
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
        references = [ENTRY, "assets/bin/Data/Managed/mscorlib.dll"]
        with tempfile.TemporaryDirectory(prefix="hyjs-chat-profile-") as temporary:
            work = Path(temporary)
            host = work / "Assembly-CSharp.dll"
            host.write_bytes(before)
            for name in references:
                (work / Path(name).name).write_bytes(archive.read(name))
            helper_path = output / (HELPER_NAME + ".dll")
            run("compile-helper", [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+",
                                   "/langversion:3", "/deterministic+", "/out:" + str(helper_path),
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

    payload_files = {entry: str((output / Path(entry).name).relative_to(ROOT)) for entry in (ENTRY, HELPER_ENTRY)}
    for entry, path in payload_files.items():
        require((ROOT / path).is_file() and sha((ROOT / path).read_bytes())
                == (sha(after) if entry == ENTRY else sha(helper_raw)), "Overlay payload differs")
    sources = [Path(__file__).resolve(), helper_source, injector_source]
    report = {
        "schema": "hyjs-client-chat-profile-pet-null-safe-v1",
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
        "allowedChangedMethods": [TARGET],
        "targetPins": {TARGET: {"signature": TARGET_SIGNATURE, "bodySHA256": TARGET_BODY_SHA256}},
        "sourcePins": {str(path.relative_to(ROOT)): sha(path.read_bytes()) for path in sources},
        "packagingCommand": "tools/client_release/package_native.py --base-apk <this baseAPK> --overlay " + str(output.relative_to(ROOT)),
        "deviceExecuted": False,
        "visualVerified": False,
        "serverMutated": False,
        "resourceFilesModified": False,
        "knownLimits": [
            "petInfo null becomes UIHeroFormation.pet == null; the original UpdatePet and attribute aggregation already branch on a null pet.",
            "Equipped pets still use the original UIPet(PetSimpleInfo) constructor; no pet is created or equipped for a player without one.",
            "Only InitData(FormationItemSimpleInfo) is patched; the other two UIHeroFormation.InitData overloads and every Battle.Engine body stay byte-identical.",
        ],
        "requiredDeviceChecks": [
            "QA player 1 opens QA player 2's formation card without a pet equipped: the card renders instead of throwing NullReferenceException in UIPet..ctor.",
            "A player with a real equipped pet still shows the original pet name, icon and attributes.",
            "The chat panel's six buttons, own formation card and combat/attribute results are unchanged.",
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
