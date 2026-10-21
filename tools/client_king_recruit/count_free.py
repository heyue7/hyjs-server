"""Keep original king result-page daily remaining count consistent after a free draw.

Only replaces the pinned get_free conditional branch with pop/nop. The original
successful result path then counts every awarded draw, matching server quotas.
No method, metadata, price, network, asset or combat calculation is rewritten.
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
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require

ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
TARGET = "UI.TabKingsRecruit::RecruitCallback"
BODY_SHA = "8dd95b82acd7830dac992aee7761fdd3c65dc1b700530199159fd097f05ea9c5"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--base-sha256", required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    require(sha(args.base_apk.read_bytes()) == args.base_sha256, "APK identity differs")
    out = args.output_directory.resolve()
    require(out.is_relative_to(ROOT) and not out.exists(), "Use a fresh project output")
    with zipfile.ZipFile(args.base_apk) as archive:
        raw = archive.read(ENTRY)
        dex_sha = sha(archive.read("classes2.dex"))
    pe = dnfile.dnPE(data=raw)
    targets = [(name, rid, row) for name, rid, row in methods(pe) if name == TARGET]
    require(len(targets) == 1, "Target inventory differs")
    row = targets[0][2]
    body = exact_body(pe, row)
    require(sha(body) == BODY_SHA, "Pinned callback body differs")
    require(row.Signature.value.hex() == "20020112a83c02", "Callback ABI differs")
    instructions = list(read_method_body_from_bytes(body).instructions)
    branch = next(i for i in instructions if i.offset == 0x93)
    require(branch.opcode.name == "brtrue.s" and branch.operand == 0xb7, "Free count branch differs")
    index = instructions.index(branch)
    before = instructions[index-1]
    require(before.opcode.name == "callvirt" and before.offset == 0x8e, "Free getter anchor differs")
    getter = pe.net.mdtables.MethodDef.rows[(before.operand.value & 0xffffff)-1]
    require(str(getter.Name) == "get_free", "Free getter identity differs")
    offset = pe.get_offset_from_rva(row.Rva) + branch.offset
    patched = bytearray(raw)
    require(patched[offset:offset+2] == bytes([0x2d, 0x22]), "Pinned branch bytes differ")
    patched[offset:offset+2] = bytes([0x26, 0x00])
    changed = dnfile.dnPE(data=bytes(patched))
    allowed = []
    engine = 0
    for (name, rid, old), (new_name, new_rid, new) in zip(methods(pe), methods(changed)):
        require((name, rid, old.Signature.value) == (new_name, new_rid, new.Signature.value), "Method ABI differs")
        if not old.Rva:
            continue
        if exact_body(pe, old) != exact_body(changed, new):
            require(name == TARGET, "Unrelated method changed")
            allowed.append(name)
        elif name.startswith("Battle.Engine"):
            engine += 1
    require(allowed == [TARGET] and engine == 547, "Changed method coverage differs")
    require([i for i,(a,b) in enumerate(zip(raw,patched)) if a!=b] == [offset,offset+1], "Non-branch bytes changed")
    out.mkdir(parents=True)
    target = out / "Assembly-CSharp.dll"
    target.write_bytes(patched)
    proof = {"schema": "hyjs-king-free-count-v1", "status": "STATIC_VERIFIED_NOT_PACKAGED",
             "baseAPKSHA256": args.base_sha256, "inputManagedSHA256": sha(raw),
             "inputDexSHA256": dex_sha, "managedOnly": True,
             "inheritedJavaByteExact": True, "inheritedJavaPublicProtectedABIExact": True,
             "payloadFiles": {ENTRY: str(target.relative_to(ROOT))},
             "replacementEntries": {ENTRY: sha(patched)}, "allowedChangedMethods": allowed,
             "sourcePins": {str(Path(__file__).resolve().relative_to(ROOT)): sha(Path(__file__).read_bytes())},
             "originalEngineMethodsExact": engine, "methodBodyBeforeSHA256": BODY_SHA,
             "patch": "0x93 brtrue.s 0xb7 -> pop; nop; successful free awards consume original result-page daily count",
             "phoneVerified": False}
    (out / "manifest.json").write_text(json.dumps(proof, ensure_ascii=False, indent=2)+"\n")
    print(json.dumps({"status": proof["status"], "changedBytes": 2, "engineMethodsExact": engine}))


if __name__ == "__main__":
    main()
