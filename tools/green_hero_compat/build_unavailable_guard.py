"""Add precise unavailable-content UI guards to the final qualified managed DLL.

Does not package, sign, install, execute game code or mutate game configuration.
The static verifier preserves every inherited method and all Engine/formation
patch bytes except the five explicitly allowed UI prefixes.
"""
import argparse
import csv
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "tools"))
from awake_diff_rows.build import methods, operand
from local_asset_ready.remove_gate import exact_body, require

TARGETS = {"ManagerController::TrySceneTransfer", "UI.RushPage::OnFightBtnClick",
           "UI.RushFightReadyItem::OnFightBtnClick", "UI.RushEliteBossPage::OnFightBtnClick",
           "UI.NPCUIBuildingPage::SendCSMsg"}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def evaluate_prefix(prefix, suffix, pe, values):
    """Evaluate the emitted CIL prefix only; no Unity/game method executes."""
    positions = {i.offset: k for k, i in enumerate(prefix)}
    stack = []
    index = 0
    notices = 0
    for _ in range(2000):
        if index == len(prefix):
            require(not notices, "Allowed prefix displayed an unavailable notice")
            return True
        instruction = prefix[index]
        name = instruction.opcode.name
        value = instruction.operand
        index += 1
        if name == "ldarg.0":
            stack.append(values)
        elif name == "ldarg.1":
            stack.append(values.get("target"))
        elif name in {"ldfld", "ldsfld"}:
            if name == "ldfld":
                require(stack.pop() is values, "Guard field owner differs")
            field = pe.net.mdtables.Field.rows[(value.value & 0xffffff) - 1]
            stack.append(values[str(field.Name)])
        elif name == "ldstr":
            stack.append(pe.net.user_strings.get(value.value & 0xffffff).value)
        elif name in {"ldc.i4", "ldc.r4"}:
            stack.append(value)
        elif name == "call":
            if value.value >> 24 == 10:
                right, left = stack.pop(), stack.pop()
                stack.append(left == right)
            else:
                delay, text = stack.pop(), stack.pop()
                require(delay == 0 and text.startswith("暂未开放"), "Rejected prefix did not show the notice")
                notices += 1
        elif name in {"br", "brtrue", "ble"}:
            jump = name == "br"
            if name == "brtrue":
                jump = bool(stack.pop())
            if name == "ble":
                right, left = stack.pop(), stack.pop()
                jump = left <= right
            if jump:
                index = len(prefix) if value == suffix[0].offset else positions[value]
        elif name == "ret":
            require(not stack and notices == 1, "Rejected prefix changed state or omitted notice")
            return False
        else:
            raise RuntimeError("Unexpected prefix side effect/opcode: " + name)
    raise RuntimeError("Guard prefix did not terminate")


def freeze_catalogue(path):
    raw = path.read_bytes()
    rows = list(csv.DictReader(raw.decode().splitlines(), delimiter="\t"))
    enabled = [r for r in rows if r["enabled"] == "1"]
    trial = [r for r in enabled if r["kind"] == "1"]
    floors = sorted({int(r["floor"]) for r in trial})
    require(floors and floors == list(range(1, max(floors) + 1)), "Current enabled trial floors are not contiguous")
    original_elite = {f"{i:02}" for i in range(1, 35)}
    elite = {r["task_id"] for r in rows if r["kind"] == "2"}
    require(elite == original_elite, "Current original elite catalogue differs")
    enabled_elite = {r["task_id"] for r in enabled if r["kind"] == "2"}
    # The existing compatible first boss has historical consumers/diagnostics;
    # retain it even when its operational enabled flag is currently closed.
    unavailable = sorted(original_elite - enabled_elite - {"01"})
    return {"availableTrialLastFloor": max(floors), "unavailableEliteIDs": unavailable,
            "unavailableFieldIDs": ["field_003", "field_004", "field_005", "field_006"],
            "liveConfigurationSHA256": sha(raw), "enabledTrialEntries": [r["task_id"] + ":" + r["star"] for r in trial],
            "retainedCompatibleEliteIDs": sorted(enabled_elite | {"01"}),
            "onlyWarBuildingStrengthenBlocked": True}


def verify(before, after, catalogue):
    a, b = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    old, new = methods(a), methods(b)
    require(len(old) == len(new), "Inherited method inventory changed")
    changes = []
    engine = unchanged = evaluated = 0
    for (name, rid, method), (other, oid, modified) in zip(old, new):
        require((name, rid) == (other, oid) and method.Signature.value == modified.Signature.value and
                method.struct.Flags == modified.struct.Flags and method.struct.ImplFlags == modified.struct.ImplFlags,
                "Inherited ABI/token/flags changed: " + name)
        if not method.Rva:
            require(not modified.Rva, "Body added: " + name)
            continue
        x, y = exact_body(a, method), exact_body(b, modified)
        if name not in TARGETS:
            require(x == y, "Non-target raw method changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            continue
        original, patched = read_method_body_from_bytes(x), read_method_body_from_bytes(y)
        require(not original.exception_handlers and not patched.exception_handlers, "Target EH changed")
        count = len(patched.instructions) - len(original.instructions)
        require(count > 0, "Missing guard prefix")
        prefix, suffix = patched.instructions[:count], patched.instructions[count:]
        old_index = {i.offset: k for k, i in enumerate(original.instructions)}
        new_index = {i.offset: k for k, i in enumerate(suffix)}
        for left, right in zip(original.instructions, suffix):
            require(left.opcode.name == right.opcode.name, "Original opcode changed: " + name)
            if left.opcode.flow_control.name in {"Branch", "Cond_Branch"}:
                require(old_index[left.operand] == new_index[right.operand], "Original branch changed")
            else:
                require(operand(left.operand) == operand(right.operand), "Original operand changed")
        literals = [b.net.user_strings.get(i.operand.value & 0xffffff).value for i in prefix if i.opcode.name == "ldstr"]
        require(any(t.startswith("暂未开放") for t in literals), "Guard has no visible unavailable notice")
        # All rejected prefixes finish with a popup and ret. No network or scene
        # method occurs before that return; allowed branches target the old body.
        require([i.opcode.name for i in prefix[-4:]] == ["ldstr", "ldc.r4", "call", "ret"], "Unexpected notice/return sequence")
        calls = [i for i in prefix if i.opcode.name == "call"]
        for i in calls:
            token = i.operand.value
            if token >> 24 == 6:
                called = next(n for n, index, _ in new if index == (token & 0xffffff))
                require(called == "BoxManager::CreatePopupTextBox", "Rejected path invokes scene/network: " + called)
            else:
                reference = b.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1] if token >> 24 == 10 else None
                require(reference is not None and str(reference.Name) == "op_Equality" and
                        str(reference.Class.row.TypeNamespace) == "System" and str(reference.Class.row.TypeName) == "String",
                        "Unexpected guard external call")
        cases = []
        if name.startswith("ManagerController::"):
            cases = [({"target": target}, target not in catalogue["unavailableFieldIDs"]) for target in
                     [None, "", "town_main", "field_001", "field_002", *catalogue["unavailableFieldIDs"], "field_007"]]
        elif name.startswith("UI.RushEliteBossPage::"):
            cases = [({"currentSelectBossId": boss}, boss not in catalogue["unavailableEliteIDs"]) for boss in
                     [None, "", *[f"{i:02}" for i in range(1, 36)]]]
        elif name.startswith("UI.NPCUIBuildingPage::"):
            cases = [({}, False)]
        else:
            key = "currentFloor" if name.startswith("UI.RushPage::") else "rushCurFloorNum"
            cases = [({key: floor}, floor <= catalogue["availableTrialLastFloor"]) for floor in
                     [0, 1, catalogue["availableTrialLastFloor"], catalogue["availableTrialLastFloor"] + 1, 100]]
        for values, allowed in cases:
            require(evaluate_prefix(prefix, suffix, b, values) == allowed, "Actual CIL prefix scope differs: " + name)
            evaluated += 1
        changes.append({"method": name, "beforeRawSHA256": sha(x), "afterRawSHA256": sha(y),
                        "prefixInstructionCount": count, "originalInstructionsAndBranchesPreserved": True,
                        "unavailableNotice": [t for t in literals if t.startswith("暂未开放")],
                        "rejectReturnsBeforeNetworkOrScene": True})
    require(engine == 547 and len(changes) == len(TARGETS), "Target/Engine inventory differs")
    return {"originalEngineMethodsUnchanged": engine, "unchangedRawMethodBodies": unchanged,
            "emittedCILPrefixCasesEvaluated": evaluated, "prefixEvaluationRunsUnityCode": False, "changes": changes}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input-managed", type=Path, required=True)
    parser.add_argument("--input-sha256", required=True)
    parser.add_argument("--live-rush-configuration", type=Path, required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    before = args.input_managed.read_bytes()
    require(sha(before) == args.input_sha256, "Input qualified managed SHA differs")
    out = args.output_directory.resolve()
    require(not out.exists(), "Use a fresh output directory")
    out.mkdir(parents=True)
    catalogue = freeze_catalogue(args.live_rush_configuration)
    (out / "unavailable-catalogue.json").write_text(json.dumps(catalogue, ensure_ascii=False, indent=2) + "\n")
    source = Path(__file__).with_name("UnavailableContentGuardProgram.cs")
    dotnet = ROOT / ".tools/native-batch/dotnet/dotnet"
    dnlib = ROOT / ".tools/native-batch/injector/bin/Debug/net8.0/dnlib.dll"
    with tempfile.TemporaryDirectory(prefix="hyjs-unavailable-ui-") as temporary:
        folder = Path(temporary)
        shutil.copy2(source, folder / "Program.cs")
        (folder / "Patch.csproj").write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        def run(name, argv):
            result = subprocess.run(["rtk", "proxy", *map(str, argv)], cwd=ROOT, capture_output=True, text=True, timeout=90)
            (out / (name + ".log")).write_text(result.stdout + result.stderr)
            require(result.returncode == 0, name + " failed; see saved log")
        run("compile", [dotnet, "build", folder / "Patch.csproj", "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        output = out / "Assembly-CSharp.dll"
        run("patch", [dotnet, folder / "bin/Debug/net8.0/Patch.dll", args.input_managed.resolve(), output, out / "unavailable-catalogue.json"])
        checks = verify(before, output.read_bytes(), catalogue)
    report = {"schema": "hyjs-unavailable-content-view-guard-v1", "status": "STATIC_GUARD_VERIFIED_NOT_PACKAGED",
              "inputManagedSHA256": sha(before), "outputManagedSHA256": sha(output.read_bytes()),
              "allowedChangedMethods": sorted(TARGETS), "sourcePins": {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [Path(__file__).resolve(), source]},
              "configuration": catalogue, "allInheritedNonTargetRawMethodsExact": True,
              "formationAndSixGreenInheritedUnchanged": True, "phoneExecuted": False, "APKPackaged": False, **checks}
    (out / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": report["status"], "outputManagedSHA256": report["outputManagedSHA256"], "engineExact": checks["originalEngineMethodsUnchanged"]}))


if __name__ == "__main__":
    main()
