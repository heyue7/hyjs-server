"""Build a fixed59 -> recycle10061 managed overlay; no APK packaging/install."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
FIXED_APK = "957ca6dd731e5d87d1d103f6fe1c76081f8491dc331581437eaffcdf3cdd265b"
FIXED_MAIN = "d4fefebc5239c16421b7bd57418e2106f4f28ca6905bff83400fd338c1dfb6b2"
MANAGED = "assets/bin/Data/Managed/"


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def run(args):
    subprocess.run(["rtk", "proxy", *map(str, args)], cwd=ROOT, check=True)


def load_proof():
    # Reuse the accepted full raw-method-body verifier without changing its source.
    path = ROOT / "tools/client_one_key59/build.py"
    spec = importlib.util.spec_from_file_location("recycle61_proof59", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def targeted_body_proof(before, after, target):
    """Remove only the declared hooks, then compare every original operation/branch."""
    rid = int(target["token"], 16) & 0xffffff
    old, new = [read_method_body_from_bytes(pe.get_data(pe.net.mdtables.MethodDef.rows[rid - 1].Rva)) for pe in [before, after]]
    mode = target["mode"]
    ins = new.instructions

    def called(row, name):
        if row.opcode.name != "call" or row.operand.value >> 24 != 10:
            return False
        ref = after.net.mdtables.MemberRef.rows[(row.operand.value & 0xffffff) - 1]
        return str(ref.Name) == name and str(ref.Class.row.TypeNamespace) == "HYJS.Recycle61" and str(ref.Class.row.TypeName) == "Flow"

    def arguments(rows):
        expected = target.get("args", [])
        assert len(rows) == len(expected)
        for row, index in zip(rows, expected):
            assert row.opcode.name == "ldarg" and row.operand.index == index

    if mode == "replace":
        count = len(target.get("args", []))
        arguments(ins[:count])
        if "kind" in target:
            assert ins[count].opcode.name == "ldc.i4" and ins[count].operand == target["kind"]
            count += 1
        assert called(ins[count], target["helper"]) and ins[count + 1].opcode.name == "ret" and len(ins) == count + 2
        return dict(token=target["token"], exactDeclaredReplacement=True)
    gate = target.get("gate") or (target.get("helper") if mode in ["gate", "quality100"] else None)
    offset = 0
    if gate:
        count = len(target.get("args", []))
        arguments(ins[:count])
        assert called(ins[count], gate) and ins[count + 1].opcode.name == "brtrue" and ins[count + 2].opcode.name == "ret"
        offset = count + 3
        assert ins[count + 1].operand == ins[offset].offset
    kept = []
    mapped = {}
    index = offset
    while index < len(ins):
        if mode == "postfix" and index + 2 < len(ins) and called(ins[index + 1], target["helper"]):
            arguments(ins[index:index + 1]); assert ins[index + 2].opcode.name == "ret"
            for row in ins[index:index + 3]:
                mapped[row.offset] = len(kept)
            kept.append(("ret", None)); index += 3
        else:
            mapped[ins[index].offset] = len(kept)
            kept.append((ins[index].opcode.name, ins[index].operand)); index += 1
    oldmap = {row.offset: index for index, row in enumerate(old.instructions)}
    oldmap[old.header_size + old.code_size] = len(old.instructions)
    mapped[new.header_size + new.code_size] = len(kept)

    def normalized(op, operand, offsets):
        if op.startswith(("br", "beq", "bge", "bgt", "ble", "blt", "bne", "leave")):
            return op.removesuffix(".s"), offsets[operand]
        if op == "switch":
            return op, [offsets[value] for value in operand]
        if op == "ldc.i4":
            return op, operand
        if op.startswith("ldc.i4."):
            if op == "ldc.i4.s":
                return "ldc.i4", operand
            return "ldc.i4", -1 if op.endswith("m1") else int(op[-1])
        return op, getattr(operand, "value", str(operand) if operand is not None else None)

    expected = []
    for index, row in enumerate(old.instructions):
        operation = normalized(row.opcode.name, row.operand, oldmap)
        if mode == "loop100" and (operation == ("ldsfld", 0x0400310b) or operation == ("ldc.i4", 40)):
            operation = "ldc.i4", 100
        if mode == "quality100" and operation == ("ldc.i4", 4):
            operation = "ldc.i4", 100
        if mode == "capacity" and index + 2 < len(old.instructions) and old.instructions[index + 2].opcode.name == "stsfld" and getattr(old.instructions[index + 2].operand, "value", None) == 0x0400310d:
            assert operation == ("ldc.i4", 40)
            operation = "ldc.i4", 100
        expected.append(operation)
    actual = [normalized(op, operand, mapped) for op, operand in kept]
    assert expected == actual, (target["token"], "Target original operation/branch sequence differs")
    return dict(token=target["token"], originalOperationAndBranchSequenceExactExceptDeclaredLimitsAndHooks=True)


def verify(before_raw, after_raw, targets):
    prior = load_proof()
    proof = prior.verify(before_raw, after_raw, targets)
    a, b = dnfile.dnPE(data=before_raw), dnfile.dnPE(data=after_raw)
    local_proofs = []
    for target in targets:
        rid = int(target["token"], 16) & 0xffffff
        old, new = [read_method_body_from_bytes(p.get_data(p.net.mdtables.MethodDef.rows[rid - 1].Rva)) for p in [a, b]]
        assert getattr(old.local_var_sig_tok, "value", None) == getattr(new.local_var_sig_tok, "value", None)
        assert not old.exception_handlers and not new.exception_handlers
        local_proofs.append(dict(token=target["token"], originalLocalSignatureToken=getattr(old.local_var_sig_tok, "value", None), targetEHAbsent=True))
    target_operations = [targeted_body_proof(a, b, target) for target in targets]
    assert [row.Signature.value for row in a.net.mdtables.StandAloneSig.rows] == [row.Signature.value for row in b.net.mdtables.StandAloneSig.rows[:len(a.net.mdtables.StandAloneSig.rows)]]
    ctor = read_method_body_from_bytes(b.get_data(b.net.mdtables.MethodDef.rows[0x38cf - 1].Rva)).instructions
    initial = {}
    for index, ins in enumerate(ctor):
        if ins.opcode.name == "stsfld" and getattr(ins.operand, "value", None) in [0x0400310b, 0x0400310d]:
            initial[ins.operand.value] = ctor[index - (2 if ins.operand.value == 0x0400310d else 1)].operand
    assert initial == {0x0400310b: 40, 0x0400310d: 100}, initial
    # Four native previews remain byte-for-byte the same after removing the sole appended DrawMain call.
    original = read_method_body_from_bytes(a.get_data(a.net.mdtables.MethodDef.rows[0x5736 - 1].Rva))
    updated = read_method_body_from_bytes(b.get_data(b.net.mdtables.MethodDef.rows[0x5736 - 1].Rva))
    assert [i.opcode.name for i in updated.instructions[-3:]] == ["ldarg", "call", "ret"]
    assert sum(i.opcode.name == "ldc.i4.4" for i in original.instructions) == 3
    assert sum(i.opcode.name == "ldc.i4.4" for i in updated.instructions) == 3
    # Original successful cost/reward callbacks and original animation body are exact non-target bodies.
    for rid in [0x5741, 0x5742, 0x5743, 0x6b0a, 0x3dd6, 0x3dd8, 0x3ddc, 0x57a4]:
        assert prior.full_body(a, a.net.mdtables.MethodDef.rows[rid - 1]) == prior.full_body(b, b.net.mdtables.MethodDef.rows[rid - 1])
    assert not any(row.ResolutionScope.table.name == "ModuleRef" and str(row.ResolutionScope.row.Name) == "SoulFlowerRecycle.dll" for row in b.net.mdtables.TypeRef.rows)
    proof.update(globalRecycleMaxUnchanged40=True, dedicatedArrayCapacity100=True, originalFourPreviewLoopsPreserved=True, originalCallbacksAnimationTransportRetryAndRewardPageRawBodiesExact=True, targetLocalSignaturesAndEH=local_proofs, originalStandAloneSignatureBlobsExact=True, targetDeclaredOperations=target_operations)
    return proof


def package_contract(base_apk, before_raw, main_output, helper, targets):
    """Export package_native's actual method-name whitelist from the verified binaries."""
    prior = load_proof()
    before, after = dnfile.dnPE(data=before_raw), dnfile.dnPE(data=main_output.read_bytes())
    allowed = []
    changed_tokens = set()
    for owner in before.net.mdtables.TypeDef.rows:
        namespace = str(owner.TypeNamespace)
        owner_name = ".".join(filter(None, [namespace, str(owner.TypeName)]))
        for member in owner.MethodList:
            old, new = member.row, after.net.mdtables.MethodDef.rows[member.row_index - 1]
            if old.Rva and prior.full_body(before, old) != prior.full_body(after, new):
                allowed.append(owner_name + "::" + str(old.Name))
                changed_tokens.add(0x06000000 | member.row_index)
    assert changed_tokens == {int(target["token"], 16) for target in targets}
    assert len(allowed) == len(set(allowed)) == 33
    payload = {MANAGED + path.name: str(path.relative_to(ROOT)) for path in [main_output, helper]}
    assert all((ROOT / file).resolve().is_relative_to(ROOT) for file in payload.values())
    assert set(payload) == {MANAGED + "Assembly-CSharp.dll", MANAGED + "SoulFlowerRecycle.dll"}
    with zipfile.ZipFile(base_apk) as archive:
        dex_sha = sha(archive.read("classes2.dex"))
        java_entries = {name: sha(archive.read(name)) for name in archive.namelist() if name.startswith("classes") and name.endswith(".dex")}
    # This overlay declares only the two managed DLLs. All Java entries are inherited by the packager.
    return dict(managedOnly=True, inheritedJavaPublicProtectedABIExact=True, inheritedJavaByteExact=True,
                inputDexSHA256=dex_sha, inheritedJavaEntrySHA256=java_entries, payloadFiles=payload,
                allowedChangedMethods=sorted(allowed), originalEngineMethodsExact=547,
                replacementEntries={entry: sha((ROOT / file).read_bytes()) for entry, file in payload.items()})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, default=ROOT / "build/hyjs-feedback59-r4.apk")
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    assert hashlib.file_digest(args.base_apk.open("rb"), "sha256").hexdigest() == FIXED_APK
    assert not args.output_directory.exists(), "Use a new owned output directory"
    targets = json.loads((HERE / "targets.json").read_text())["targets"]
    sources = [*sorted(HERE.glob("*.cs")), HERE / "README.md", HERE / "targets.json", HERE / "layout-evidence.json", Path(__file__).resolve(), ROOT / "tools/client_recycle_selection/Selection.cs", ROOT / "tools/client_one_key59/build.py"]
    source_pins = {str(path.relative_to(ROOT)): sha(path.read_bytes()) for path in sources}
    output = args.output_directory.resolve()
    output.mkdir(parents=True)
    work = ROOT / ".tools/native-batch"
    dotnet = work / "dotnet/dotnet"
    compiler = next((work / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    with tempfile.TemporaryDirectory(prefix="hyjs61-recycle-build-") as directory:
        temp = Path(directory)
        refs = temp / "refs"; refs.mkdir()
        with zipfile.ZipFile(args.base_apk) as apk:
            for entry in apk.namelist():
                if entry.startswith(MANAGED) and entry.endswith(".dll"):
                    (refs / Path(entry).name).write_bytes(apk.read(entry))
        main_input = (refs / "Assembly-CSharp.dll").read_bytes()
        assert sha(main_input) == FIXED_MAIN
        reference_pins = {path.name: sha(path.read_bytes()) for path in refs.glob("*.dll")}
        helper = output / "SoulFlowerRecycle.dll"
        run([dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+", "/out:" + str(helper), *["/r:" + str(path) for path in sorted(refs.glob("*.dll")) if path.name != helper.name], ROOT / "tools/client_recycle_selection/Selection.cs", HERE / "Flow.cs"])
        for name in ["Inject", "Probe"]:
            folder = temp / name; folder.mkdir()
            shutil.copy2(HERE / ("Injector.cs" if name == "Inject" else "Probe.cs"), folder / "Program.cs")
            dependency = '<ItemGroup><Reference Include="dnlib"><HintPath>' + str(work / "injector/bin/Debug/net8.0/dnlib.dll") + '</HintPath></Reference></ItemGroup>' if name == "Inject" else ""
            (folder / (name + ".csproj")).write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup>' + dependency + '</Project>')
            run([dotnet, "build", folder / (name + ".csproj"), "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        main_output = output / "Assembly-CSharp.dll"
        run([dotnet, temp / "Inject/bin/Debug/net8.0/Inject.dll", refs / "Assembly-CSharp.dll", helper, main_output, HERE / "targets.json"])
        proof = verify(main_input, main_output.read_bytes(), targets)
        (refs / "Assembly-CSharp.dll").write_bytes(main_output.read_bytes())
        shutil.copy2(helper, refs / helper.name)
        run([dotnet, temp / "Probe/bin/Debug/net8.0/Probe.dll", refs, output / "selection-policy-probe.json"])
    assert source_pins == {str(path.relative_to(ROOT)): sha(path.read_bytes()) for path in sources}, "Source changed during build"
    packaging = package_contract(args.base_apk, main_input, main_output, helper, targets)
    manifest = dict(schema="hyjs-recycle10061-managed-overlay-v1", baseAPKSHA256=FIXED_APK, inputManagedSHA256=FIXED_MAIN, outputManagedSHA256=sha(main_output.read_bytes()), helperSHA256=sha(helper.read_bytes()), sourcePins=source_pins, referencePins=reference_pins, **proof, **packaging, originalRefsCSharp3Compiled=True, selectionPolicyProbePassed=True, temporaryBuildDirectoryRemoved=True, apkBuilt=False, deviceTested=False, dbTested=False)
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps({key: value for key, value in manifest.items() if key not in ["sourcePins", "referencePins", "targetLocalSignaturesAndEH", "targetDeclaredOperations"]}, indent=2))


if __name__ == "__main__":
    main()
