"""Rebuild the existing small sprout helper against the pinned 77 candidate."""
import argparse
import hashlib
import json
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

import dnfile
from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_arena_state.build import decode, instruction_value, methods

BASE_SHA256 = "c970012d7c785c58c3b923ea4a0ea8e650204ed0e50e16e914f47e54ac4aece8"
MANAGED = "assets/bin/Data/Managed/"
HELPER = "HYJS.Sprout73.dll"
IMAGE = ROOT / "tools/ggo_items/assets/ggo_sprout.png"
SOURCE = Path(__file__).with_name("Runtime.cs")


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def public_api(pe):
    result = []
    for t in pe.net.mdtables.TypeDef:
        if not t.Flags.tdPublic:
            continue
        name = str(t.TypeNamespace) + "." + str(t.TypeName)
        result.append(("type", name))
        result.extend(("method", name, str(m.row.Name), m.row.Signature.value.hex())
                      for m in t.MethodList if m.row.Flags.mdPublic)
        result.extend(("field", name, str(f.row.Name), f.row.Signature.value.hex())
                      for f in t.FieldList if f.row.Flags.fdPublic)
    return sorted(result)


def assembly_identity(pe):
    assembly = pe.net.mdtables.Assembly.rows[0]
    return (str(assembly.Name), assembly.MajorVersion, assembly.MinorVersion,
            assembly.BuildNumber, assembly.RevisionNumber, str(assembly.Culture),
            assembly.PublicKey.value)


def check_hooks(pe):
    expected = {
        "Room.RCUIPlayer::DoStart": ("Attach", "tail", "ldarg.0"),
        "Room.RCUIPlayer::DoStop": ("Detach", "head", "ldarg.0"),
        "Dict.Dao.TableCopyExpDao::ParseRowData": ("Experience", "tail", "dup"),
    }
    found = set()
    for name, _, method in methods(pe):
        if name not in expected:
            continue
        helper, placement, load = expected[name]
        code = decode(pe, method)
        selected = code[:2] if placement == "head" else code[-3:-1]
        if len(selected) != 2 or selected[0].opcode.name != load or selected[1].opcode.name != "call":
            raise ValueError("Existing hook shape differs: " + name)
        token = instruction_value(selected[1])
        if not isinstance(token, int) or token >> 24 != 10:
            raise ValueError("Existing hook is not a member reference: " + name)
        ref = pe.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1]
        owner = ref.Class.row
        assembly = owner.ResolutionScope.row
        if (str(owner.TypeNamespace), str(owner.TypeName), str(ref.Name), str(assembly.Name)) != (
            "HYJS.Sprout73", "Runtime", helper, "HYJS.Sprout73"
        ):
            raise ValueError("Existing hook target differs: " + name)
        found.add(name)
    if found != set(expected):
        raise ValueError("Existing hook inventory differs")
    return sorted(found)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    parser.add_argument("--tool-root", type=Path, default=Path("/Users/lirui/LR/hyjs"))
    args = parser.parse_args()
    base = args.base_apk.resolve()
    out = args.output_directory.resolve()
    with base.open("rb") as f:
        if hashlib.file_digest(f, "sha256").hexdigest() != BASE_SHA256:
            raise ValueError("Base APK identity differs")
    if not out.is_relative_to(ROOT) or out.exists():
        raise ValueError("Fresh project output required")
    out.mkdir(parents=True)
    runtime = args.tool_root.resolve() / ".tools/native-batch"
    dotnet = runtime / "dotnet/dotnet"
    compiler = next((runtime / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    with tempfile.TemporaryDirectory(prefix="hyjs-sprout78-") as temp, zipfile.ZipFile(base) as apk:
        work = Path(temp)
        refs = work / "refs"
        refs.mkdir()
        for name in apk.namelist():
            if name.startswith(MANAGED) and name.endswith(".dll") and Path(name).name != HELPER:
                (refs / Path(name).name).write_bytes(apk.read(name))
        main_data = apk.read(MANAGED + "Assembly-CSharp.dll")
        original_helper = apk.read(MANAGED + HELPER)
        main_pe = dnfile.dnPE(data=main_data)
        hooks = check_hooks(main_pe)
        engine = sum(1 for name, _, method in methods(main_pe) if name.startswith("Battle.Engine") and method.Rva)
        if engine != 547:
            raise ValueError("Original engine inventory differs")
        image = Image.open(IMAGE).convert("RGBA")
        image = image.crop(image.getchannel("A").getbbox())
        image.save(work / "ggo_sprout.png")
        helper = out / HELPER
        command = [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3",
                   "/deterministic+", "/out:" + str(helper),
                   "/resource:" + str(work / "ggo_sprout.png") + ",ggo_sprout.png"]
        command += ["/r:" + str(p) for p in sorted(refs.glob("*.dll"))]
        command.append(SOURCE)
        result = subprocess.run(["rtk", "proxy", *map(str, command)], capture_output=True, text=True, timeout=180)
        (out / "compile.log").write_text(result.stdout + result.stderr)
        if result.returncode:
            raise ValueError("Helper compilation failed; inspect compile.log")
        rebuilt = helper.read_bytes()
        before, after = dnfile.dnPE(data=original_helper), dnfile.dnPE(data=rebuilt)
        if assembly_identity(before) != assembly_identity(after):
            raise ValueError("Helper assembly identity differs")
        if public_api(before) != public_api(after):
            raise ValueError("Helper public API differs")
        if [str(r.Name) for r in after.net.mdtables.ManifestResource] != ["ggo_sprout.png"]:
            raise ValueError("Helper icon resource differs")
        entry = MANAGED + HELPER
        report = {
            "schema": "hyjs-sprout78-helper-refresh-v1",
            "status": "STATIC_VERIFIED_NOT_PACKAGED",
            "baseAPKSHA256": BASE_SHA256,
            "inputManagedSHA256": sha(main_data),
            "previousHelperSHA256": sha(original_helper),
            "existingHooksVerified": hooks,
            "helperPublicAPIExact": True,
            "originalEngineMethodsExact": engine,
            "originalMetadataABIAndTokensPreserved": True,
            "payloadFiles": {entry: str(helper.relative_to(ROOT))},
            "replacementEntries": {entry: sha(rebuilt)},
            "sourcePins": {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in (Path(__file__), SOURCE, IMAGE)},
            "deviceTested": False,
            "visualVerified": False,
        }
        (out / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({"status": report["status"], "existingHooksVerified": hooks,
                          "replacementEntries": report["replacementEntries"]}))


if __name__ == "__main__":
    main()
