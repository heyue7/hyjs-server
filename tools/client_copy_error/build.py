"""Build and verify a 1.1.5 copy-start failure overlay without packaging."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import struct
import subprocess
import tempfile
import uuid
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]

ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
BASE_APK = "bd3e4caefe47545de7b2272681a363f79df5d75838b96bc752e882cdbb07137e"
BASE_DLL = "3d8f50e543ad082ea03bff294f17421a2eed4316d5cda664b03cc0f46cc23e8a"
CALLBACK = "<CopyToBattleScene>c__AnonStorey163::<>m__37"
CALLBACK_RAW = "abea16791c9ff4bcb2899afff62d585291eb053ffc5f35a0cef0e8f37ccab50e"
CALLER_RAW = "4acbfac12a7ba990894fd1707ca653acc01228214680236381ae1a5cc6925301"
TICK_RAW = "a76bfe371c29d4217349b2c6bed77e21d62cc45469b135d70c91fdfab79e9d10"
HTTP_ERROR_RAW = "0fe43c30bde5621734102e878c66e7fbecbc939cb7ad595b3f084565a5a35a31"
GUARD_OPS = ["ldarg.1", "brfalse", "ldarg.1", "ldfld", "isinst", "dup", "brfalse", "callvirt",
             "ldloca.s", "call", "brfalse", "ldloc.s", "ldc.i4.s", "beq", "br", "pop"]


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def methods(pe):
    return [('.'.join(filter(None, [str(t.TypeNamespace), str(t.TypeName)])) + '::' + str(m.row.Name),
             m.row_index, m.row) for t in pe.net.mdtables.TypeDef for m in t.MethodList]


def operand(value):
    return value.value if hasattr(value, "value") else value.index if hasattr(value, "index") else value


def exact_body(pe, method):
    header = pe.get_data(method.Rva, 12)
    if header[0] & 3 == 2:
        length = 1 + (header[0] >> 2)
    else:
        flags = struct.unpack_from("<H", header)[0]
        length = ((flags >> 12) * 4) + struct.unpack_from("<I", header, 4)[0]
        if flags & 8:
            at = (length + 3) & ~3
            while True:
                section = pe.get_data(method.Rva + at, 4)
                size = int.from_bytes(section[1:4], "little") if section[0] & 0x40 else section[1]
                require(size >= 4, "malformed method extra section")
                at += size
                if not section[0] & 0x80:
                    break
                at = (at + 3) & ~3
            length = at
    return pe.get_data(method.Rva, length)


def check_source_pins():
    expected = json.loads(Path(__file__).with_name("source-pins.json").read_text())
    require(set(expected) == {"build.py", "Injector.cs"}, "source pin inventory differs")
    for name, digest in expected.items():
        require(sha(Path(__file__).with_name(name).read_bytes()) == digest, "source pin differs: " + name)
    return expected


def preflight(original):
    pe = dnfile.dnPE(data=original)
    require(str(uuid.UUID(bytes_le=pe.net.mdtables.Module.rows[0].Mvid.value)) ==
            "22d2d1ef-5802-443d-9c1e-315513506718", "input MVID differs")
    expected = {CALLBACK: CALLBACK_RAW, "Copy.CopyManager::CopyToBattleScene": CALLER_RAW,
                "NetWork.Layer.HTTPManager::Tick": TICK_RAW, "NetWork.Layer.HTTPManager::HttpError": HTTP_ERROR_RAW}
    found = {}
    for name, rid, row in methods(pe):
        if name in expected:
            require(name not in found and sha(exact_body(pe, row)) == expected[name], "input method differs: " + name)
            found[name] = (rid, row)
    require(set(found) == set(expected) and found[CALLBACK][0] == 0x6c61, "input method inventory differs")
    callback = read_method_body_from_bytes(exact_body(pe, found[CALLBACK][1]))
    instructions = callback.instructions
    at = next(i for i, ins in enumerate(instructions) if ins.opcode.name == "ldc.i4" and ins.operand == 1047)
    require(not callback.exception_handlers and callback.max_stack == 2 and instructions[0].opcode.name == "ldarg.2" and
            instructions[1].opcode.name == "brfalse" and instructions[-1].opcode.name == "ret" and
            instructions[at-1].opcode.name == "callvirt" and operand(instructions[at-1].operand) == 0x06004736 and
            operand(instructions[at+1].operand) == 0x060032b5 and
            operand(instructions[at+2].operand) == 0x06004ad6, "callback cleanup/relogin pattern differs")
    error_type = pe.net.mdtables.TypeDef.rows[0xdd-1]
    field = pe.net.mdtables.Field.rows[0x37ef-1]
    getter = pe.net.mdtables.MethodDef.rows[0x0b82-1]
    parser = pe.net.mdtables.MemberRef.rows[0x07e1-1]
    require((str(error_type.TypeNamespace), str(error_type.TypeName), str(field.Name), str(getter.Name),
             str(parser.Name), str(parser.Class.row.TypeNamespace), str(parser.Class.row.TypeName), parser.Signature.value.hex()) ==
            ("ProtoBuf.Message", "ErrorMessage", "kBody", "get_code", "TryParse", "System", "Int32", "0002020e1008"),
            "packet/error/parser metadata differs")


def int32_try_parse(value):
    if not isinstance(value, str) or not re.fullmatch(r"\s*[+-]?[0-9]+\s*", value):
        return None
    result = int(value)
    return result if -(2**31) <= result < 2**31 else None


def model_guard(instructions, original, original_return, packet):
    """Interpret the emitted guard only; Unity and network are not executed."""
    positions = {instruction.offset: i for i, instruction in enumerate(instructions)}
    stack, local, pc = [], {}, 0
    for _ in range(100):
        if pc == len(instructions):
            require(not stack, "stack not empty before original failure branch")
            return "original-relogin"
        instruction = instructions[pc]
        pc += 1
        op, value = instruction.opcode.name, instruction.operand
        if op == "ldarg.1":
            stack.append(packet)
        elif op == "ldfld":
            stack.append(stack.pop()["body"])
        elif op == "isinst":
            item = stack.pop()
            stack.append(item if isinstance(item, dict) and item.get("kind") == "error" else None)
        elif op == "dup":
            stack.append(stack[-1])
        elif op == "pop":
            stack.pop()
        elif op == "callvirt":
            stack.append(stack.pop().get("code"))
        elif op == "ldloca.s":
            stack.append(operand(value))
        elif op == "call":
            index, text = stack.pop(), stack.pop()
            parsed = int32_try_parse(text)
            local[index] = parsed or 0
            stack.append(parsed is not None)
        elif op == "ldloc.s":
            stack.append(local[operand(value)])
        elif op == "ldc.i4.s":
            stack.append(value)
        elif op in {"brfalse", "beq", "br"}:
            take = op == "br"
            if op == "brfalse":
                take = not stack.pop()
            elif op == "beq":
                right, left = stack.pop(), stack.pop()
                take = left == right
            if take:
                if value == original:
                    require(not stack, "stack not empty on original failure branch")
                    return "original-relogin"
                if value == original_return:
                    require(not stack, "stack not empty on handled business error")
                    return "already-shown"
                pc = positions[value]
        else:
            raise ValueError("unexpected emitted guard instruction: " + op)
    raise ValueError("guard did not terminate")


def verify(before_raw, after_raw):
    require(sha(before_raw) == BASE_DLL, "unexpected input assembly")
    before, after = dnfile.dnPE(data=before_raw), dnfile.dnPE(data=after_raw)
    require(before.net.mdtables.Module.rows[0].Mvid.value == after.net.mdtables.Module.rows[0].Mvid.value,
            "module MVID changed")
    old_types, new_types = before.net.mdtables.TypeDef.rows, after.net.mdtables.TypeDef.rows
    require(len(old_types) == len(new_types), "type inventory changed")
    for old, new in zip(old_types, new_types):
        require((str(old.TypeNamespace), str(old.TypeName), old.struct.Flags, old.struct.Extends_CodedIndex,
                 [m.row_index for m in old.MethodList], [f.row_index for f in old.FieldList]) ==
                (str(new.TypeNamespace), str(new.TypeName), new.struct.Flags, new.struct.Extends_CodedIndex,
                 [m.row_index for m in new.MethodList], [f.row_index for f in new.FieldList]),
                "type identity, inheritance, or members changed")
    old_fields, new_fields = before.net.mdtables.Field.rows, after.net.mdtables.Field.rows
    require(len(old_fields) == len(new_fields), "field inventory changed")
    for old, new in zip(old_fields, new_fields):
        require((str(old.Name), old.Signature.value, old.struct.Flags) ==
                (str(new.Name), new.Signature.value, new.struct.Flags), "field ABI changed")

    old_methods, new_methods = methods(before), methods(after)
    require(len(old_methods) == len(new_methods), "method inventory changed")
    unchanged = engine = 0
    bodies = {}
    target = None
    for (name, rid, old), (new_name, new_rid, new) in zip(old_methods, new_methods):
        require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) ==
                (new_name, new_rid, new.Signature.value, new.struct.Flags, new.struct.ImplFlags),
                "method identity or ABI changed: " + name)
        if not old.Rva:
            require(not new.Rva, "bodyless method gained a body: " + name)
            continue
        old_raw, new_raw = exact_body(before, old), exact_body(after, new)
        if name != CALLBACK:
            require(old_raw == new_raw, "other raw method header/IL/EH changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            bodies[name] = sha(old_raw)
            continue
        require(rid == 0x6c61 and sha(old_raw) == CALLBACK_RAW and target is None, "callback binding changed")
        target = (old_raw, new_raw)
    require(target is not None and engine == 547, "target or original battle engine inventory differs")
    require(bodies.get("Copy.CopyManager::CopyToBattleScene") == CALLER_RAW, "original copy caller changed")
    require(bodies.get("NetWork.Layer.HTTPManager::Tick") == TICK_RAW, "HTTP error/code 21 path changed")
    require(bodies.get("NetWork.Layer.HTTPManager::HttpError") == HTTP_ERROR_RAW, "HTTP transport path changed")

    old_body, new_body = (read_method_body_from_bytes(raw) for raw in target)
    require(not old_body.exception_handlers and not new_body.exception_handlers, "target EH changed")
    require(old_body.max_stack == new_body.max_stack == 2, "target stack contract changed")
    old_sig = before.net.mdtables.StandAloneSig.rows[(old_body.local_var_sig_tok.value & 0xffffff) - 1].Signature.value
    new_sig = after.net.mdtables.StandAloneSig.rows[(new_body.local_var_sig_tok.value & 0xffffff) - 1].Signature.value
    require(old_sig.hex() == "0702128c2c12b774" and new_sig == b"\x07\x03" + old_sig[2:] + b"\x08",
            "existing locals or new int32 local changed")
    old, new = old_body.instructions, new_body.instructions
    at = next(i for i, inst in enumerate(old) if inst.opcode.name == "ldc.i4" and inst.operand == 1047)
    guard = new[at:at + len(GUARD_OPS)]
    require([i.opcode.name for i in guard] == GUARD_OPS, "unexpected inserted guard")
    original_new = new[:at] + new[at + len(GUARD_OPS):]
    require(len(old) == len(original_new), "original instruction count changed")
    old_index = {i.offset: n for n, i in enumerate(old)}
    new_index = {i.offset: n for n, i in enumerate(original_new)}
    for old_i, new_i in zip(old, original_new):
        require(old_i.opcode.name == new_i.opcode.name, "original opcode changed")
        if old_i.opcode.flow_control.name in {"Branch", "Cond_Branch"}:
            require(old_index[old_i.operand] == new_index[new_i.operand], "original branch target changed")
        else:
            require(operand(old_i.operand) == operand(new_i.operand), "original instruction operand changed")
    require(old[0].opcode.name == "ldarg.2" and old[1].opcode.name == "brfalse", "success entry changed")
    require(old[at - 1].opcode.name == "callvirt" and operand(old[at - 1].operand) == 0x06004736,
            "original mask cleanup anchor changed")
    require([operand(guard[i].operand) for i in (3, 4, 7, 9, 12)] ==
            [0x040037ef, 0x020000dd, 0x06000b82, 0x0a0007e1, 21], "guard token binding changed")
    require(operand(guard[8].operand) == operand(guard[11].operand) == 2, "guard local binding changed")
    original, original_return = original_new[at].offset, original_new[-1].offset
    require([guard[i].operand for i in (1, 10, 13)] == [original] * 3 and
            guard[6].operand == guard[15].offset and guard[14].operand == original_return,
            "guard branch target changed")
    cases = [
        ("business-90003", {"body": {"kind": "error", "code": "90003"}}, "already-shown"),
        ("other-business", {"body": {"kind": "error", "code": "90001"}}, "already-shown"),
        ("session-21", {"body": {"kind": "error", "code": "21"}}, "original-relogin"),
        ("session-numeric-equivalent", {"body": {"kind": "error", "code": " +021 "}}, "original-relogin"),
        ("invalid-code", {"body": {"kind": "error", "code": "bad"}}, "original-relogin"),
        ("overflow-code", {"body": {"kind": "error", "code": "2147483648"}}, "original-relogin"),
        ("transport-request", {"body": {"kind": "request"}}, "original-relogin"),
        ("null-body", {"body": None}, "original-relogin"),
        ("null-packet", None, "original-relogin"),
    ]
    for name, packet, expected in cases:
        actual = model_guard(guard, original, original_return, packet)
        require(actual == expected, name + " yielded " + actual)
    return {"changedMethod": CALLBACK, "unchangedRawMethodBodies": unchanged,
            "unchangedBattleEngineMethods": engine, "typeFieldMethodABIAndMVIDPreserved": True,
            "otherMethodHeadersILAndExceptionRegionsByteIdentical": True,
            "originalCallbackInstructionsAndBranchTargetsPreserved": True,
            "callbackExceptionRegions": 0, "originalCleanupAndSuccessPreserved": True,
            "httpTickAndTransportBodiesByteIdentical": True,
            "guardModelCases": [case[0] for case in cases],
            "inputCallbackRawSHA256": sha(target[0]), "outputCallbackRawSHA256": sha(target[1])}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--base-proof", type=Path, required=True)
    parser.add_argument("--toolchain-root", type=Path, required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    source_pins = check_source_pins()
    require(not args.output_directory.exists(), "output directory must not exist")
    with args.base_apk.open("rb") as stream:
        require(hashlib.file_digest(stream, "sha256").hexdigest() == BASE_APK, "unexpected 1.1.5 APK")
    proof_raw = args.base_proof.read_bytes()
    proof = json.loads(proof_raw)
    require(proof.get("binarySHA256") == BASE_APK and proof.get("versionName") == "1.1.5" and
            proof.get("buildCode") == 2026110106 and proof.get("signatureVerified") and proof.get("zipalignVerified"),
            "1.1.5 package proof differs")
    with zipfile.ZipFile(args.base_apk) as apk:
        require(len(apk.namelist()) == len(set(apk.namelist())), "duplicate APK entry")
        original = apk.read(ENTRY)
        require(sha(original) == BASE_DLL, "unexpected input assembly")
        for name, digest in proof["replacementEntries"].items():
            require(sha(apk.read(name)) == digest, "recorded 1.1.5 entry differs: " + name)
    preflight(original)
    dotnet = args.toolchain_root / "dotnet/dotnet"
    dnlib = args.toolchain_root / "injector/bin/Debug/net8.0/dnlib.dll"
    require(dotnet.is_file() and dnlib.is_file(), "explicit dotnet/dnlib toolchain missing")
    with tempfile.TemporaryDirectory(prefix="hyjs-copy-error-") as temporary:
        folder = Path(temporary)
        incoming, outgoing = folder / "input.dll", folder / "output.dll"
        incoming.write_bytes(original)
        shutil.copy2(Path(__file__).with_name("Injector.cs"), folder / "Program.cs")
        project = folder / "Patch.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib">'
                           '<HintPath>' + str(dnlib.resolve()) + '</HintPath></Reference></ItemGroup></Project>')
        def run(command):
            subprocess.run(["rtk", "proxy", *map(str, command)], cwd=ROOT, check=True, timeout=90)
        run([dotnet, "build", project, "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        run([dotnet, folder / "bin/Debug/net8.0/Patch.dll", incoming, outgoing])
        result = outgoing.read_bytes()
        checks = verify(original, result)
    require(check_source_pins() == source_pins, "tool source changed during build")
    with args.base_apk.open("rb") as stream:
        require(hashlib.file_digest(stream, "sha256").hexdigest() == BASE_APK, "input APK changed during build")
    args.output_directory.mkdir(parents=True)
    (args.output_directory / "Assembly-CSharp.dll").write_bytes(result)
    manifest = {"status": "PASS_OFFLINE_NOT_PACKAGED", "baseAPKSHA256": BASE_APK,
                "baseProofSHA256": sha(proof_raw), "inputManagedSHA256": BASE_DLL,
                "outputManagedSHA256": sha(result), "replacementEntries": {ENTRY: sha(result)},
                "sourcePins": {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in
                               [Path(__file__).resolve(), Path(__file__).with_name("Injector.cs").resolve()]},
                "apkBuilt": False, "phoneExecuted": False, "checks": checks}
    (args.output_directory / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(manifest, ensure_ascii=False))


if __name__ == "__main__":
    main()
