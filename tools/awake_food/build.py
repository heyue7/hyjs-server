"""Patch the original Awake food callback on the exact qualified 1.0.90 APK.

Produces a standalone Assembly-CSharp.dll overlay and an audit manifest. It does
not package an APK or execute the game.
"""
import argparse
import hashlib
import json
from pathlib import Path
import sys
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "tools"))
from local_asset_ready.remove_gate import exact_body  # noqa: E402

ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
APK_SHA256 = "1410ac039dee92ea5341ef5db22d08d1baf1eda563f2fb59e508af980db547e5"
DLL_SHA256 = "16432cee8041735e53af8254d226949cab594bd4be8bde1575d06522159233c8"
TARGET = "UI.ChaCopyItem::<OnClickiItem>m__FA"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def check(value, message):
    if not value:
        raise ValueError(message)


def find_target(pe):
    owner = next(t for t in pe.net.mdtables.TypeDef.rows
                 if str(t.TypeNamespace) == "UI" and str(t.TypeName) == "ChaCopyItem")
    targets = [m for m in owner.MethodList if str(m.row.Name) == "<OnClickiItem>m__FA"]
    check(len(targets) == 1, "Awake food callback identity differs")
    return targets[0]


def build(before):
    check(sha(before) == DLL_SHA256, "Unexpected qualified managed input")
    pe = dnfile.dnPE(data=before)
    method = find_target(pe)
    original = exact_body(pe, method.row)
    body = read_method_body_from_bytes(original)
    check(len(body.exception_handlers) == 2, "Callback exception regions differ")
    code = body.instructions
    matches = []
    for index, ins in enumerate(code):
        token = getattr(ins.operand, "value", None)
        next_token = getattr(code[index + 1].operand, "value", None) if index + 1 < len(code) else None
        if (ins.opcode.name == "callvirt" and token == 0x060009D4
                and next_token == 0x0600040B and code[index - 1].opcode.name == "ldloc.2"):
            matches.append(index)
    check(len(matches) == 1, "Original consumed-count conversion differs")
    source = code[matches[0] - 1].offset
    check(original[source:source + 6] == bytes.fromhex("086fd4090006"), "Original IL bytes differ")
    at = pe.get_offset_from_rva(method.row.Rva) + source
    after = bytearray(before)
    # The response quantity describes the remaining carried stack. Its first
    # use in this callback builds the actual backpack debit, which is one item.
    after[at:at + 6] = bytes.fromhex("170000000000")
    changed = [i for i, (a, b) in enumerate(zip(before, after)) if a != b]
    check(len(after) == len(before) and changed and all(at <= i < at + 6 for i in changed),
          "Unrelated assembly bytes changed")
    checked = dnfile.dnPE(data=bytes(after))
    updated_method = find_target(checked)
    revised = exact_body(checked, updated_method.row)
    check(original[:source] == revised[:source] and original[source + 6:] == revised[source + 6:],
          "Target header, branches or exception regions changed")
    updated = read_method_body_from_bytes(revised)
    check(len(updated.exception_handlers) == 2, "Callback exception regions changed")
    get_items = 0
    set_item = 0
    for ins in updated.instructions:
        token = getattr(ins.operand, "value", None)
        get_items += token == 0x06001EB5
        set_item += token == 0x06004CBC
    check(get_items == 2 and set_item == 1, "Original slot update or response links changed")
    return bytes(after), {
        "target": TARGET,
        "targetToken": hex(0x06000000 | method.row_index),
        "inputMethodSHA256": sha(original),
        "outputMethodSHA256": sha(revised),
        "changedAssemblyByteOffsets": changed,
        "changedInstructionOffset": source,
        "beforeHex": "086fd4090006",
        "afterHex": "170000000000",
        "allOtherAssemblyBytesIdentical": True,
        "exceptionRegionsUnchanged": True,
        "originalSlotAndResponseCallsPreserved": True,
        "additionalRuntimeAssemblies": [],
        "additionalResources": [],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    digest = hashlib.sha256()
    with args.base_apk.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    check(digest.hexdigest() == APK_SHA256, "Unexpected base APK")
    with zipfile.ZipFile(args.base_apk) as apk:
        before = apk.read(ENTRY)
    after, evidence = build(before)
    args.output_directory.mkdir(parents=True, exist_ok=False)
    (args.output_directory / "Assembly-CSharp.dll").write_bytes(after)
    manifest = {
        "schema": "awake-food-count-overlay-v1",
        "baseAPKSHA256": APK_SHA256,
        "inputManagedSHA256": DLL_SHA256,
        "outputManagedSHA256": sha(after),
        "replacementEntries": {ENTRY: sha(after)},
        "packaged": False,
        "phoneExecuted": False,
        **evidence,
    }
    (args.output_directory / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(manifest, ensure_ascii=False))


if __name__ == "__main__":
    main()
