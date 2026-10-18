"""Scale original character/equipment VIP bag preview deltas tenfold."""
import argparse
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
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require

TARGET = "UI.BagPage::OnCapacityClick"
TARGETS = {
    TARGET: "a3341c9897b0b965f4ffdf7d71e7cafe64c65b890d8107b83e950db86f4aaa67",
    "VIPData::InitVipDes": "3b12e6062eb2d222c1e3ee5172b94c2878c0ff9e07bfec542c822306ff57a533",
}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def identity(pe):
    def row_value(row):
        extra = getattr(row, "HashValue", None)
        return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion,
                row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value,
                getattr(row.struct, "HashAlgId", None), extra.value if extra is not None else None)
    for row in [*pe.net.mdtables.Assembly.rows, *pe.net.mdtables.AssemblyRef.rows]:
        require(not (row.struct.Flags & 1) or len(row.PublicKey.value) >= 16, "Assembly metadata declares an invalid full PublicKey: " + str(row.Name))
    require(pe.net.struct.StrongNameSignatureSize == 0, "Review signed original input")
    return ([row_value(x) for x in pe.net.mdtables.Assembly.rows],
            [row_value(x) for x in pe.net.mdtables.AssemblyRef.rows],
            pe.net.struct.Flags, pe.net.struct.StrongNameSignatureRva, pe.net.struct.StrongNameSignatureSize)


def verify(before, after):
    left, right = [dnfile.dnPE(data=raw) for raw in (before, after)]
    old, new = methods(left), methods(right)
    require(len(old) == len(new), "Method inventory changed")
    require(identity(left) == identity(right), "Original Assembly/AssemblyRef identity, key flags or CLI metadata changed")
    changed, unchanged, engine = [], 0, 0
    require(not getattr(right.net.mdtables, "FieldPtr", None), "Old Unity must not gain FieldPtr")
    for table in ["TypeDef", "Field", "FieldPtr", "MethodPtr"]:
        a, b = getattr(left.net.mdtables, table, None), getattr(right.net.mdtables, table, None)
        require((len(a.rows) if a else 0) == (len(b.rows) if b else 0), "Managed inventory changed: " + table)
    for (name, rid, a), (other, index, b) in zip(old, new):
        require((name, rid) == (other, index) and a.Signature.value == b.Signature.value and a.struct.Flags == b.struct.Flags and a.struct.ImplFlags == b.struct.ImplFlags, "Original ABI changed: " + name)
        if not a.Rva:
            require(not b.Rva, "Unexpected new body")
            continue
        raw, updated = exact_body(left, a), exact_body(right, b)
        if name not in TARGETS:
            require(raw == updated, "Unrelated method changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            continue
        original, result = [read_method_body_from_bytes(x) for x in (raw, updated)]
        require(sha(raw) == TARGETS[name], "Unmodified original preview/description required; do not compose on superseded scaled output")
        require(not original.exception_handlers and not result.exception_handlers, "Unexpected preview EH")
        def instructions(body):
            indexes = {x.offset: n for n, x in enumerate(body.instructions)}
            normalized = []
            for instruction in body.instructions:
                operand = instruction.operand
                if instruction.opcode.name == "switch":
                    operand = tuple(indexes[x] for x in operand)
                elif instruction.opcode.flow_control.name in ("Branch", "Cond_Branch"):
                    operand = indexes[operand]
                elif hasattr(operand, "value"):
                    operand = operand.value
                else:
                    operand = str(operand)
                normalized.append((instruction.opcode.name, operand))
            return normalized
        normalized = instructions(original)
        insertions = []
        fields = []
        for at, instruction in enumerate(original.instructions):
            if name == TARGET:
                if instruction.opcode.name != "sub" or at == 0:
                    continue
                load = original.instructions[at - 1]
                if load.opcode.name != "ldfld":
                    continue
            else:
                if instruction.opcode.name != "ldfld" or original.instructions[at + 1].opcode.name != "box":
                    continue
                load = instruction
            field = left.net.mdtables.Field.rows[(load.operand.value & 0xffffff) - 1]
            if str(field.Name) in ("numPerk3", "numPerk4"):
                insertions.append(at + 1)
                fields.append(str(field.Name))
        per_field = 2 if name == TARGET else 1
        require(len(insertions) == per_field * 2 and fields.count("numPerk3") == per_field and fields.count("numPerk4") == per_field, "Unexpected original character/equipment bag value sites")
        def shifted(index):
            return index + sum(2 for at in insertions if at <= index)
        expected = []
        for at, (opcode, operand) in enumerate(normalized):
            original_instruction = original.instructions[at]
            if opcode == "switch":
                operand = tuple(shifted(x) for x in operand)
            elif original_instruction.opcode.flow_control.name in ("Branch", "Cond_Branch"):
                operand = shifted(operand)
            expected.append((opcode, operand))
            if at + 1 in insertions:
                expected.extend([("ldc.i4.s", "10"), ("mul", "None")])
        require(instructions(result) == expected, "Preview patch changed more than character/equipment bag deltas")
        changed.append({"method": name, "beforeRawSHA256": sha(raw), "afterRawSHA256": sha(updated), "bagCapacityValueMultiplier": 10, "valueSites": len(insertions), "originalPerkFields": fields})
    require(engine == 547 and len(changed) == 2, "Changed target/engine inventory differs")
    return {"changes": changed, "unchangedInheritedRawMethodBodies": unchanged, "originalEngineMethodsUnchanged": engine, "allInheritedNonTargetMethodsExact": True, "VIPCostAndLevelsAndOtherBenefitsUnchanged": True, "originalVIPPerkTableUnchanged": True, "petBagVIPPreviewPresent": False, "originalAssemblyIdentityAndAllReferencesPreserved": True, "cliFlagsPreserved": True, "strongNameSignatureBytes": 0}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input-managed", type=Path, required=True)
    parser.add_argument("--input-managed-sha256", required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    before = args.input_managed.read_bytes()
    require(sha(before) == args.input_managed_sha256, "Pinned composition input differs")
    input_pe = dnfile.dnPE(data=before)
    identity(input_pe)
    for target, pinned in TARGETS.items():
        rows = [row for name, _, row in methods(input_pe) if name == target]
        require(len(rows) == 1 and sha(exact_body(input_pe, rows[0])) == pinned, "Unmodified original preview/description required; superseded fivefold/tenfold outputs cannot be stacked")
    output = args.output_directory.resolve()
    require(not output.exists(), "Fresh overlay directory required")
    output.mkdir(parents=True)
    runtime = ROOT / ".tools/native-batch"
    dotnet = runtime / "dotnet/dotnet"
    dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"
    injector = Path(__file__).with_name("Injector.cs")
    def run(name, command):
        result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=90)
        (output / (name + ".log")).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + " failed; inspect saved log")
    with tempfile.TemporaryDirectory(prefix="hyjs-role-bag-preview-") as temporary:
        folder = Path(temporary)
        shutil.copy2(injector, folder / "Program.cs")
        project = folder / "Patch.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run("compile-injector", [dotnet, "build", project, "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        run("patch", [dotnet, folder / "bin/Debug/net8.0/Patch.dll", args.input_managed, output / "Assembly-CSharp.dll"])
    after = (output / "Assembly-CSharp.dll").read_bytes()
    report = {"status": "STATIC_VERIFIED_NOT_PACKAGED", "inputManagedSHA256": sha(before), "outputManagedSHA256": sha(after), "sourcePins": {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in (Path(__file__).resolve(), injector)}, "replacementEntries": ["assets/bin/Data/Managed/Assembly-CSharp.dll"], "phoneExecuted": False, **verify(before, after)}
    (output / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"status": report["status"], "outputManagedSHA256": report["outputManagedSHA256"]}))


if __name__ == "__main__":
    main()
