"""Prepare a SHA-bound local QA overlay; never package, install or promote an APK."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
from xml.sax.saxutils import escape
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_remote.config import validate_origin
from tools.awake_diff_rows.build import methods, operand
from tools.local_asset_ready.remove_gate import exact_body

DLL = "assets/bin/Data/Managed/Assembly-CSharp.dll"
WALLET = "assets/local-wallet.json"
MANIFEST = "AndroidManifest.xml"
TARGET = "http://127.0.0.1:18861"
TARGET_METHODS = (
    "ServerConfig::GetServerUrl",
    "Battle.FieldPVEWorldBossBattleOver::End",
    "Room.RoomNetField::SCCurrentPosition",
    "VoicePlugin::InitGVoice",
)
INJECTOR = ROOT / "tools/client_recycle_selection/QAInjector.cs"
SOURCE_FILES = (
    Path(__file__), Path(__file__).with_name("package.py"), Path(__file__).with_name("manifest.py"), INJECTOR,
    ROOT / "tools/client_remote/config.py", ROOT / "tools/awake_diff_rows/build.py",
    ROOT / "tools/local_asset_ready/remove_gate.py", ROOT / "tools/build_multiclient.py",
    ROOT / "tools/client_branding/package_release.py",
)


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def file_sha(path):
    with Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def qa_origin(value):
    value = validate_origin(value)
    require(value == TARGET, "QA75 permits only the reviewed loopback origin on port18861")
    return value


def read_binding(base, proof, proof_sha):
    base, proof = Path(base), Path(proof)
    require(re.fullmatch(r"[0-9a-f]{64}", proof_sha or ""), "Explicit frozen manifest SHA256 required")
    require(base.is_file() and not base.is_symlink(), "Base APK must be a regular, non-symlink file")
    require(proof.is_file() and not proof.is_symlink(), "Base manifest must be a regular, non-symlink file")
    raw = proof.read_bytes()
    require(sha(raw) == proof_sha, "Frozen base manifest SHA256 mismatch")
    record = json.loads(raw)
    expected = record.get("binarySHA256") if isinstance(record, dict) else None
    require(isinstance(expected, str) and re.fullmatch(r"[0-9a-f]{64}", expected), "Manifest must supply binarySHA256")
    require(file_sha(base) == expected, "Base APK differs from frozen manifest binarySHA256")
    return {"baseAPK": str(base.resolve()), "baseAPKSHA256": expected,
            "baseManifest": str(proof.resolve()), "baseManifestSHA256": proof_sha}


def read_base(base):
    with zipfile.ZipFile(base) as archive:
        names = archive.namelist()
        require(len(names) == len(set(names)), "Duplicate APK entries refused")
        require(all(name in names for name in (DLL, WALLET, MANIFEST)), "Base lacks controlled overlay entries")
        managed, wallet_raw = archive.read(DLL), archive.read(WALLET)
    wallet = json.loads(wallet_raw)
    require(isinstance(wallet, dict) and set(wallet) == {"origin"}, "Unexpected wallet configuration fields")
    old = validate_origin(wallet["origin"])
    require(wallet["origin"] == old and old != TARGET, "Base origin must be canonical and must not already be QA75")
    return managed, wallet_raw, old


def targets_for(old, new):
    return {name: (old + "/sao/", new + "/sao/") if name == TARGET_METHODS[0] else (old, new)
            for name in TARGET_METHODS}


def verify(before, after, old_origin, new_origin):
    """Freshly audit this frozen input without overriding an older SHA gate."""
    import dnfile
    from dncil.cil.body.reader import read_method_body_from_bytes

    require(old_origin == validate_origin(old_origin), "Noncanonical inherited origin")
    new_origin = qa_origin(new_origin)
    targets = targets_for(old_origin, new_origin)
    left_pe, right_pe = [dnfile.dnPE(data=raw) for raw in (before, after)]
    left, right = methods(left_pe), methods(right_pe)
    require(len(left) == len(right), "Managed method inventory changed")
    for name in ("Field", "FieldPtr", "MethodPtr", "TypeDef"):
        a, b = getattr(left_pe.net.mdtables, name, None), getattr(right_pe.net.mdtables, name, None)
        require((len(a.rows) if a else 0) == (len(b.rows) if b else 0), "Metadata inventory changed: " + name)
    require(not getattr(right_pe.net.mdtables, "FieldPtr", None), "Old Unity gained FieldPtr")
    unchanged = engine = 0
    changes = []
    for (name, rid, original), (other, oid, modified) in zip(left, right):
        require((name, rid) == (other, oid) and original.Signature.value == modified.Signature.value
                and original.struct.Flags == modified.struct.Flags and original.struct.ImplFlags == modified.struct.ImplFlags,
                "Inherited managed ABI changed: " + name)
        if not original.Rva:
            require(not modified.Rva, "Unexpected managed method body")
            continue
        x, y = exact_body(left_pe, original), exact_body(right_pe, modified)
        if name not in targets:
            require(x == y, "Unrelated raw managed body changed: " + name)
            unchanged += 1
            engine += name.startswith("Battle.Engine")
            continue
        a, b = [read_method_body_from_bytes(raw) for raw in (x, y)]
        require(len(x) == len(y) and len(a.instructions) == len(b.instructions)
                and getattr(a.local_var_sig_tok, "value", None) == getattr(b.local_var_sig_tok, "value", None)
                and a.max_stack == b.max_stack, "Controlled method contract changed: " + name)
        changed = 0
        for first, second in zip(a.instructions, b.instructions):
            require((first.offset, first.opcode.name) == (second.offset, second.opcode.name), "Opcode/layout changed: " + name)
            if first.opcode.name == "ldstr":
                old = left_pe.net.user_strings.get(first.operand.value & 0xffffff).value
                new = right_pe.net.user_strings.get(second.operand.value & 0xffffff).value
                if old == targets[name][0]:
                    require(new == targets[name][1], "Controlled HTTP origin mismatch")
                    changed += 1
                else:
                    require(old == new and operand(first.operand) == operand(second.operand), "Unrelated string operand changed")
            else:
                require(operand(first.operand) == operand(second.operand), "Unrelated operand changed: " + name)
        require(changed == 1, "Expected one original origin per reviewed method: " + name)
        def regions(body):
            return [(h.exception_type, h.try_start, h.try_end, h.handler_start, h.handler_end,
                     h.filter_start, getattr(h.catch_type, "value", None)) for h in body.exception_handlers]
        require(regions(a) == regions(b), "Controlled exception regions changed")
        changes.append({"method": name, "beforeRawSHA256": sha(x), "afterRawSHA256": sha(y),
                        "onlyOneLdstrOperandChanged": True})
    require(engine == 547 and len(changes) == 4, "Four origins/547 engine method audit failed")
    for _, _, method in right:
        if method.Rva:
            for instruction in read_method_body_from_bytes(exact_body(right_pe, method)).instructions:
                if instruction.opcode.name == "ldstr":
                    value = right_pe.net.user_strings.get(instruction.operand.value & 0xffffff).value
                    require(old_origin not in value, "Inherited remote origin remains executable")
    return {"originalEngineMethodsUnchanged": engine, "unchangedInheritedRawMethodBodies": unchanged,
            "allInheritedNonTargetMethodsExact": True, "onlyFourHTTPOriginLiteralsChanged": True,
            "noManagedTypesMethodsFieldsAdded": True, "noExecutableInheritedOriginRemaining": True,
            "changes": changes}


def source_pins():
    return {str(path.resolve().relative_to(ROOT)): file_sha(path) for path in SOURCE_FILES}


def run(command, cwd, log, env=None):
    result = subprocess.run(["rtk", "proxy", *map(str, command)], cwd=cwd, env=env,
                            capture_output=True, text=True, timeout=120)
    log.write_text(result.stdout + result.stderr)
    require(result.returncode == 0, "QA tool failed; see " + str(log))


def build(base, proof, proof_sha, origin, output, runtime_root):
    origin = qa_origin(origin)
    binding = read_binding(base, proof, proof_sha)
    before, wallet_before, old = read_base(base)
    runtime_root = Path(runtime_root).resolve()
    runtime = runtime_root / ".tools/native-batch"
    dotnet, dnlib = runtime / "dotnet/dotnet", runtime / "injector/bin/Debug/net8.0/dnlib.dll"
    require(dotnet.is_file() and dnlib.is_file(), "Existing .NET8/dnlib runtime missing; pass --runtime-root")
    pins = source_pins()
    output = Path(output).resolve()
    require(output.is_relative_to(ROOT / "analysis/growth75") and not output.exists(), "Fresh output under analysis/growth75 required")
    output.mkdir(parents=True)
    try:
        with tempfile.TemporaryDirectory(prefix=".qa75-injector-", dir=output) as temporary:
            folder = Path(temporary)
            current = folder / "input.dll"
            current.write_bytes(before)
            shutil.copyfile(INJECTOR, folder / "Program.cs")
            project = folder / "Patch.csproj"
            project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                               '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                               '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib">'
                               '<HintPath>' + escape(str(dnlib)) + '</HintPath></Reference></ItemGroup></Project>')
            nuget = folder / "NuGet.Config"
            nuget.write_text('<configuration><packageSources><clear /></packageSources></configuration>')
            env = dict(os.environ, DOTNET_CLI_HOME=str(folder / "dotnet-home"), DOTNET_SKIP_FIRST_TIME_EXPERIENCE="1",
                       DOTNET_CLI_TELEMETRY_OPTOUT="1", DOTNET_CLI_WORKLOAD_UPDATE_NOTIFY_DISABLE="true")
            run([dotnet, "restore", project, "--configfile", nuget, "--ignore-failed-sources", "--disable-parallel",
                 "--verbosity", "quiet", "-p:NuGetAudit=false"], ROOT, output / "restore.log", env)
            run([dotnet, "build", project, "--no-restore", "--nologo", "--verbosity", "quiet",
                 "-p:NuGetAudit=false", "-p:UseSharedCompilation=false"], ROOT, output / "compile.log", env)
            run([dotnet, folder / "bin/Debug/net8.0/Patch.dll", current, old, origin, output / "Assembly-CSharp.dll"],
                ROOT, output / "patch.log", env)
        after = (output / "Assembly-CSharp.dll").read_bytes()
        checks = verify(before, after, old, origin)
        wallet_after = (json.dumps({"origin": origin}, separators=(",", ":")) + "\n").encode()
        (output / "local-wallet.json").write_bytes(wallet_after)
        require(read_binding(base, proof, proof_sha) == binding and source_pins() == pins, "Frozen input or tool changed during overlay build")
        result = {"schema": "hyjs-qa75-loopback-overlay-v1", "status": "STATIC_VERIFIED_NOT_PACKAGED",
                  **binding, "origin": origin, "inheritedOrigin": old, "sourcePins": pins,
                  "inputManagedSHA256": sha(before), "outputManagedSHA256": sha(after),
                  "inputWalletConfigSHA256": sha(wallet_before), "outputWalletConfigSHA256": sha(wallet_after),
                  "runtime": {"dotnetSHA256": file_sha(dotnet), "dnlibSHA256": file_sha(dnlib)},
                  "phoneExecuted": False, "networkServicesContacted": False, "promoted": False,
                  "ownedIntermediatesCleaned": not folder.exists(), **checks}
        (output / "manifest.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
        return result
    except Exception:
        # Preserve diagnostic logs, remove only this fresh output's partial DLL.
        for name in ("Assembly-CSharp.dll", "local-wallet.json", "manifest.json"):
            (output / name).unlink(missing_ok=True)
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, required=True)
    parser.add_argument("--base-manifest", type=Path, required=True)
    parser.add_argument("--base-manifest-sha256", required=True)
    parser.add_argument("--origin", required=True)
    parser.add_argument("--output-directory", type=Path, default=ROOT / "analysis/growth75/qa-local-overlay")
    parser.add_argument("--runtime-root", type=Path, default=ROOT)
    args = parser.parse_args()
    result = build(args.base_apk, args.base_manifest, args.base_manifest_sha256, args.origin, args.output_directory, args.runtime_root)
    print(json.dumps({key: result[key] for key in ("status", "baseAPKSHA256", "outputManagedSHA256", "originalEngineMethodsUnchanged")}))


if __name__ == "__main__":
    main()
