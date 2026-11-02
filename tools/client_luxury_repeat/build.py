"""Build and verify the dragon-offer managed overlay; do not install or publish."""
import argparse
import hashlib
import json
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
SOURCE = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
from tools.client_arena_state.build import decode, exact_body, instruction_value, member_reference, methods

ENTRY = "assets/bin/Data/Managed/Assembly-CSharp.dll"
HELPER = "HYJS.LuxuryDragon.dll"
TARGETS = {
    "UI.UICopy::_CheckLuxuryEquipmentActivity": "6887e40e8d7f22bc6742d95af57bfa710eb470bb7f8676af54b2099d6976d7b1",
    "UI.UICopy::_CheckLevelUp": "5e2455696fc7ef3a245010a747894227ceef69317ead56c523150908690a611c",
    "UI.UICopySweep::DoClose": "14b303bc84050927138e06f39585fb95a7111389a74aaff0dd6c856202f408fa",
}

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def require(condition, message):
    if not condition:
        raise ValueError(message)

def il(pe, method):
    raw = exact_body(pe, method)
    body = read_method_body_from_bytes(raw)
    return raw[body.header_size:body.header_size + body.code_size]

def hook(pe, ins):
    require(ins.opcode.name == "call", "Missing hook call")
    ref = member_reference(pe, instruction_value(ins))
    owner = ref.Class.row
    require(str(owner.TypeNamespace) + "." + str(owner.TypeName) == "HYJS.LuxuryDragon.Runtime" and str(ref.Name) == "Schedule", "Unexpected hook destination")

def verify(before, after):
    left, right = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    original, patched = methods(left), methods(right)
    require(len(original) == len(patched), "Original method count changed")
    changed, unchanged, engine = [], 0, 0
    for (name, rid, a), (other, rid2, b) in zip(original, patched):
        require(name == other and rid == rid2 and a.Signature.value == b.Signature.value, "Original ABI/token changed")
        require(bool(a.Rva) == bool(b.Rva), "Body presence changed")
        if not a.Rva:
            continue
        if name not in TARGETS:
            require(exact_body(left, a) == exact_body(right, b), "Unrelated raw body changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            continue
        x, y = decode(left, a), decode(right, b)
        if name.endswith("::_CheckLuxuryEquipmentActivity"):
            require([v.opcode.name for v in y] == ["call", "ret"], "Story hook body changed")
            hook(right, y[0])
        else:
            require(len(y) == len(x) + 1 and x[-1].opcode.name == "ret" and y[-1].opcode.name == "ret", "Original tail changed: " + name)
            hook(right, y[-2])
            old_handlers = read_method_body_from_bytes(exact_body(left, a)).exception_handlers
            new_handlers = read_method_body_from_bytes(exact_body(right, b)).exception_handlers
            def regions(handlers):
                return [(h.exception_type, h.try_start, h.try_end, h.handler_start, h.handler_end, h.filter_start, str(h.catch_type)) for h in handlers]
            require(regions(old_handlers) == regions(new_handlers), "Exception regions changed: " + name)
            offsets = {v.offset: i for i, v in enumerate(x)}
            for old, new in zip(x[:-1], y[:-2]):
                require(old.opcode.name == new.opcode.name, "Original opcode changed: " + name)
                if isinstance(old.operand, int) and old.opcode.name.startswith(("br", "beq", "bne", "bge", "bgt", "ble", "blt", "leave")):
                    require(new.operand == y[offsets[old.operand]].offset, "Original branch changed: " + name)
                elif old.opcode.name == "switch":
                    require(tuple(new.operand) == tuple(y[offsets[v]].offset for v in old.operand), "Original switch changed: " + name)
                else:
                    require(instruction_value(old) == instruction_value(new), "Original operand changed: " + name)
        changed.append(name)
    require(engine == 547 and set(changed) == set(TARGETS), "Engine/target coverage differs")
    for label in ("TypeDef", "Field"):
        old, new = getattr(left.net.mdtables, label).rows, getattr(right.net.mdtables, label).rows
        require(len(old) == len(new), label + " count changed")
        for a, b in zip(old, new):
            require(str(a.TypeName if label == "TypeDef" else a.Name) == str(b.TypeName if label == "TypeDef" else b.Name), label + " token changed")
            if label == "Field":
                require(a.Signature.value == b.Signature.value, "Field ABI changed")
    return dict(allowedChangedMethods=changed, originalEngineMethodsUnchanged=engine,
                unrelatedRawMethodBodiesUnchanged=unchanged, originalMetadataABIAndTokensPreserved=True)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--expected-sha256", required=True)
    parser.add_argument("--output-directory", type=Path, required=True)
    parser.add_argument("--tool-root", type=Path, required=True)
    args = parser.parse_args()
    base, out = args.base_apk.resolve(), args.output_directory.resolve()
    require(sha(base.read_bytes()) == args.expected_sha256, "Input APK SHA mismatch")
    require(not out.exists() and out.is_relative_to(ROOT), "Fresh project output required")
    out.mkdir(parents=True)
    runtime = args.tool_root.resolve() / ".tools/native-batch"
    dotnet = runtime / "dotnet/dotnet"
    compiler = next((runtime / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    dnlib = runtime / "injector/bin/Debug/net8.0/dnlib.dll"
    def run(name, cmd):
        result = subprocess.run(["rtk", "proxy", *map(str, cmd)], capture_output=True, text=True, timeout=180)
        (out / (name + ".log")).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + " failed; see log")
    with zipfile.ZipFile(base) as archive, tempfile.TemporaryDirectory(prefix="hyjs-luxury-build-") as temp:
        w = Path(temp)
        before = archive.read(ENTRY)
        host = w / "Assembly-CSharp.dll"
        host.write_bytes(before)
        pe = dnfile.dnPE(data=before)
        pins = {name: sha(il(pe, m)) for name, _, m in methods(pe) if name in TARGETS}
        require(pins == TARGETS, "Target original CIL changed")
        refs = [host]
        for name in ("mscorlib.dll", "UnityEngine.dll"):
            ref = w / name
            ref.write_bytes(archive.read("assets/bin/Data/Managed/" + name))
            refs.append(ref)
        helper = out / HELPER
        run("compile-helper", [dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+", "/codepage:65001", "/out:" + str(helper), *["/reference:" + str(p) for p in refs], SOURCE / "Runtime.cs"])
        shutil.copy2(SOURCE / "Injector.cs", w / "Program.cs")
        project = w / "Patch.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run("compile-injector", [dotnet, "build", project, "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        run("inject", [dotnet, w / "bin/Debug/net8.0/Patch.dll", host, helper, out / "Assembly-CSharp.dll"])
        dex_sha = sha(archive.read("classes2.dex"))
    after = (out / "Assembly-CSharp.dll").read_bytes()
    proof = verify(before, after)
    metadata = dnfile.dnPE(data=after)
    require(not any(x.ResolutionScope.table.name == "ModuleRef" and str(x.ResolutionScope.row.Name) == HELPER for x in metadata.net.mdtables.TypeRef.rows), "Invalid helper module scope")
    run("verify-logic", [sys.executable, SOURCE / "verify_logic.py", "--tool-root", args.tool_root.resolve()])
    payload = {ENTRY: str((out / "Assembly-CSharp.dll").relative_to(ROOT)), "assets/bin/Data/Managed/" + HELPER: str(helper.relative_to(ROOT))}
    report = dict(schema="hyjs-client-luxury-dragon-repeat-v1", status="STATIC_VERIFIED_NOT_PACKAGED", managedOnly=True,
                  inputAPKSHA256=args.expected_sha256, baseAPKSHA256=args.expected_sha256, inheritedJavaPublicProtectedABIExact=True,
                  inheritedJavaByteExact=True, inputManagedSHA256=sha(before), inputDexSHA256=dex_sha,
                  replacementEntries={name: sha((ROOT / path).read_bytes()) for name, path in payload.items()},
                  outputManagedSHA256=sha(after), helperSHA256=sha(helper.read_bytes()), targetOriginalCILSHA256=pins,
                  payloadFiles=payload, sourcePins={str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in SOURCE.iterdir() if p.is_file()},
                  deviceExecuted=False, serverMutated=False, actualSourceLogic="see verify-logic.log; net8 doubles, not Unity/device", **proof)
    (out / "manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(proof, ensure_ascii=False))

if __name__ == "__main__":
    main()
