"""Align the original breach label's two threshold branches with the server."""

import argparse
import hashlib
import json
import struct
from pathlib import Path

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
TYPE = "UI.BreachPage"
METHOD = "UpdateAll"
ORIGINAL_CIL_SHA256 = "82b8b3fbc02936c120b9df636c571b39af66b6e94bbcdaf34346acdd96aeb5c0"
EXPECTED_ENGINE_METHODS = 547
BRANCHES = ((0x3B4, "Critical1", 0x3D9), (0x3EB, "Critical2", 0x410))
SCHEMA = "hyjs-breach84-probability-label-main-v1"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


def inventory(pe):
    methods, fields = [], {}
    for owner in pe.net.mdtables.TypeDef.rows:
        name = ".".join(filter(None, [str(owner.TypeNamespace), str(owner.TypeName)]))
        for field in owner.FieldList:
            fields[0x04000000 | field.row_index] = name + "::" + str(field.row.Name)
        methods.extend((name + "::" + str(method.row.Name), method.row_index, method.row)
                       for method in owner.MethodList)
    return methods, fields


def body_extent(pe, method):
    """Include original headers, locals token, aligned extra and exception sections."""
    header = pe.get_data(method.Rva, 12)
    require(len(header) == 12, "truncated method header")
    kind = header[0] & 3
    if kind == 2:
        length = 1 + (header[0] >> 2)
    elif kind == 3:
        flags = struct.unpack_from("<H", header)[0]
        size = (flags >> 12) * 4
        require(size >= 12, "invalid fat method header")
        length = size + struct.unpack_from("<I", header, 4)[0]
        if flags & 8:
            at = (length + 3) & ~3
            while True:
                section = pe.get_data(method.Rva + at, 4)
                require(len(section) == 4, "truncated method section")
                size = int.from_bytes(section[1:4], "little") if section[0] & 0x40 else section[1]
                require(size >= 4, "invalid method section")
                at += size
                if not section[0] & 0x80:
                    break
                at = (at + 3) & ~3
            length = at
    else:
        raise ValueError("unknown CIL method header")
    offset = pe.get_offset_from_rva(method.Rva)
    require(len(pe.get_data(method.Rva, length)) == length, "truncated method body")
    return offset, length


def patch(before):
    pe = dnfile.dnPE(data=before)
    require(pe.net is not None and pe.net.mdtables is not None, "input is not a managed assembly")
    methods, fields = inventory(pe)
    targets = [(rid, row) for name, rid, row in methods if name == TYPE + "::" + METHOD]
    require(len(targets) == 1, "breach method missing or ambiguous")
    rid, method = targets[0]
    require(method.Rva, "breach method has no CIL")
    start, extent = body_extent(pe, method)
    body = read_method_body_from_bytes(before[start:start + extent])
    require(not body.exception_handlers, "unexpected breach method exception region")
    code_start = start + body.header_size
    code_end = code_start + body.code_size
    require(sha(before[code_start:code_end]) == ORIGINAL_CIL_SHA256, "breach method preimage changed")

    instructions = list(body.instructions)
    offsets = {item.offset - body.header_size: index for index, item in enumerate(instructions)}
    accessed = {fields.get(getattr(item.operand, "value", 0)) for item in instructions
                if item.opcode.name in ("ldfld", "ldflda", "stfld")}
    require("Dict.DataModel.TableHeroBreached::Critical3" not in accessed,
            "the label method gained a third comparison")
    changed = []
    for il_offset, field, destination in BRANCHES:
        require(il_offset in offsets, "label branch offset missing")
        index = offsets[il_offset]
        branch, previous = instructions[index], instructions[index - 1]
        require(branch.opcode.name == "bgt.un" and branch.operand == destination + body.header_size,
                "label branch or destination changed")
        require(previous.opcode.name == "ldfld" and
                fields.get(getattr(previous.operand, "value", 0)) == "Dict.DataModel.TableHeroBreached::" + field,
                "label threshold field changed")
        require(index >= 4 and instructions[index - 4].opcode.name == "ldfld" and
                fields.get(getattr(instructions[index - 4].operand, "value", 0)) == "UI.UIHero::breachExp" and
                instructions[index - 3].opcode.name == "conv.r8" and instructions[index - 2].opcode.name == "ldloc.0",
                "label experience comparison changed")
        file_offset = code_start + il_offset
        require(before[file_offset:file_offset + 5] == b"\x42\x20\x00\x00\x00", "branch bytes changed")
        changed.append({"threshold": field, "cilOffset": il_offset, "fileOffset": file_offset,
                        "before": "42", "after": "41", "destinationCilOffset": destination,
                        "relativeBranchBytes": "20000000"})

    after = bytearray(before)
    for entry in changed:
        after[entry["fileOffset"]] = 0x41
    after = bytes(after)
    differences = [i for i, (left, right) in enumerate(zip(before, after)) if left != right]
    require(len(before) == len(after) and differences == [x["fileOffset"] for x in changed],
            "bytes outside the two branch opcodes changed")

    engine = 0
    unchanged = 0
    checked = 0
    for name, method_rid, row in methods:
        if not row.Rva:
            continue
        offset, length = body_extent(pe, row)
        checked += 1
        if name.startswith("Battle.Engine"):
            engine += 1
            require(before[offset:offset + length] == after[offset:offset + length], "battle engine changed")
        if method_rid != rid:
            require(before[offset:offset + length] == after[offset:offset + length],
                    "another managed method overlaps or changed")
            unchanged += 1
    require(engine == EXPECTED_ENGINE_METHODS, "battle engine inventory differs")
    return after, {
        "targetType": TYPE,
        "targetMethod": METHOD,
        "methodToken": f"0x{0x06000000 | rid:08x}",
        "beforeCilSHA256": sha(before[code_start:code_end]),
        "afterCilSHA256": sha(after[code_start:code_end]),
        "patches": changed,
        "critical3UsedForLabel": False,
        "allOtherFileBytesExact": True,
        "otherManagedBodiesExact": unchanged,
        "managedBodiesChecked": checked,
        "originalEngineMethodsExact": engine,
        "branchTargetsUnchanged": True,
    }


def apply_overlay(payload, expected_input_sha256):
    """Apply to an already-composed Main instead of replacing another overlay."""
    if ENTRY not in payload:
        raise ValueError("composed Main entry is missing")
    before = payload[ENTRY]
    if sha(before) != expected_input_sha256.lower():
        raise ValueError("composed Main SHA-256 mismatch")
    after, proof = patch(before)
    payload[ENTRY] = after
    return proof


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input-managed", required=True, type=Path)
    parser.add_argument("--input-managed-sha256", required=True)
    parser.add_argument("--output-directory", required=True, type=Path)
    args = parser.parse_args()
    source, out = args.input_managed.resolve(), args.output_directory.resolve()
    expected = args.input_managed_sha256.lower()
    before = source.read_bytes()
    require(len(expected) == 64 and sha(before) == expected, "input Main SHA-256 mismatch")
    require(out.is_relative_to(ROOT / "build") and not out.exists(), "fresh build output directory required")
    after, proof = patch(before)
    out.mkdir(parents=True)
    result = out / "Assembly-CSharp.dll"
    result.write_bytes(after)
    report = {
        "schema": SCHEMA,
        "status": "STATIC_VERIFIED_NOT_PACKAGED",
        "inputManagedSHA256": sha(before),
        "outputManagedSHA256": sha(after),
        "payloadFiles": {ENTRY: str(result.relative_to(ROOT))},
        "replacementEntries": {ENTRY: sha(after)},
        "sourcePins": {str(Path(__file__).resolve().relative_to(ROOT)): sha(Path(__file__).read_bytes())},
        "deviceTested": False,
        **proof,
    }
    (out / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": report["status"], "outputSHA256": report["outputManagedSHA256"],
                      "patches": proof["patches"], "engineMethods": proof["originalEngineMethodsExact"]}))


if __name__ == "__main__":
    main()
