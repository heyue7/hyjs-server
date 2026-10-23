"""Build scoped bulk-purchase managed overlay; no APK packaging or installation."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import struct
import subprocess
import tempfile
import zipfile
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
FIXED_APK = "b7ad3c7c8ba53cc7990f3f3590642aa405e9a554135916a468668f0c7fdbdbbd"
MANAGED = "assets/bin/Data/Managed/"
HERE = Path(__file__).resolve().parent


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def run(args):
    subprocess.run(["rtk", "proxy", *map(str, args)], cwd=ROOT, check=True)


def cil(pe, row):
    header = pe.get_data(row.Rva, 12)
    if header[0] & 3 == 2:
        return pe.get_data(row.Rva + 1, header[0] >> 2)
    size = struct.unpack_from("<I", header, 4)[0]
    offset = (struct.unpack_from("<H", header)[0] >> 12) * 4
    return pe.get_data(row.Rva + offset, size)


def verify(before_raw, after_raw, targets):
    a, b = dnfile.dnPE(data=before_raw), dnfile.dnPE(data=after_raw)
    allowed = {int(x["token"], 16) & 0xffffff for x in targets}
    changed, untouched, engine = [], 0, 0
    assert len(a.net.mdtables.MethodDef.rows) == len(b.net.mdtables.MethodDef.rows)
    for owner in a.net.mdtables.TypeDef.rows:
        for method in owner.MethodList:
            x, y = method.row, b.net.mdtables.MethodDef.rows[method.row_index - 1]
            assert str(x.Name) == str(y.Name) and x.Signature.value == y.Signature.value
            if not x.Rva:
                continue
            same = cil(a, x) == cil(b, y)
            if same:
                untouched += 1
            else:
                changed.append(method.row_index)
            if str(owner.TypeNamespace).startswith("Battle.Engine"):
                engine += 1
                assert same
                old = read_method_body_from_bytes(a.get_data(x.Rva))
                new = read_method_body_from_bytes(b.get_data(y.Rva))
                shape = lambda body: [(h.exception_type, h.try_start, h.try_end, h.handler_start, h.handler_end, h.filter_start, getattr(h.catch_type, "value", None)) for h in body.exception_handlers]
                assert shape(old) == shape(new)
    assert set(changed) == allowed, (changed, allowed)
    assert engine == 547
    # Every pre-existing definition signature/field value/attribute identity remains at its original RID.
    for old, new in zip(a.net.mdtables.TypeDef.rows, b.net.mdtables.TypeDef.rows):
        assert (str(old.TypeName), str(old.TypeNamespace), old.struct.Flags) == (str(new.TypeName), str(new.TypeNamespace), new.struct.Flags)
    for index, old in enumerate(a.net.mdtables.Field.rows):
        new = b.net.mdtables.Field.rows[index]
        assert str(old.Name) == str(new.Name) and old.Signature.value == new.Signature.value and old.struct.Flags == new.struct.Flags
    bad = []
    for row in b.net.mdtables.TypeRef.rows:
        if row.ResolutionScope.table.name == "ModuleRef" and str(row.ResolutionScope.row.Name) == "HYJS.BulkPurchase.dll":
            bad.append(str(row.TypeNamespace) + "." + str(row.TypeName))
    assert not bad, bad
    assert len(b.net.mdtables.Field.rows) == len(a.net.mdtables.Field.rows)
    assert len(b.net.mdtables.TypeDef.rows) == len(a.net.mdtables.TypeDef.rows)
    for name in ["FieldPtr", "MethodPtr"]:
        assert not getattr(a.net.mdtables, name, None), "Input must retain no " + name
        assert not getattr(b.net.mdtables, name, None), "Output must retain no " + name
    for old, new in zip(a.net.mdtables.TypeDef.rows, b.net.mdtables.TypeDef.rows):
        assert [x.row_index for x in old.FieldList] == [x.row_index for x in new.FieldList]
        assert [x.row_index for x in old.MethodList] == [x.row_index for x in new.MethodList]
    return {"nonTargetRawCILExact": untouched, "changedMethodTokens": [hex(0x06000000 + x) for x in changed], "originalEngineMethods": engine, "originalEngineCILAndEHExact": True, "originalDefinitionSignaturesAndFieldRIDsExact": True, "helperScopedMainTypeModuleRefs": bad, "addedFieldCount": 0, "fieldPtrAbsent": True, "methodPtrAbsent": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, default=ROOT / "build/hyjs-feedback55.apk")
    parser.add_argument("--input-managed", type=Path)
    parser.add_argument("--output-directory", type=Path, required=True)
    args = parser.parse_args()
    assert hashlib.file_digest(args.base_apk.open("rb"), "sha256").hexdigest() == FIXED_APK
    assert not args.output_directory.exists(), "Use a new owned output directory"
    targets_raw = (HERE / "targets.json").read_bytes()
    targets = json.loads(targets_raw)["targets"]
    output = args.output_directory.resolve()
    output.mkdir(parents=True)
    work = ROOT / ".tools/native-batch"
    dotnet = work / "dotnet/dotnet"
    compiler = next((work / "dotnet/sdk").glob("*/Roslyn/bincore/csc.dll"))
    with tempfile.TemporaryDirectory(prefix="hyjs58-bulk-build-") as temp:
        temp = Path(temp)
        refs = temp / "refs"
        refs.mkdir()
        with zipfile.ZipFile(args.base_apk) as apk:
            for entry in apk.namelist():
                if entry.startswith(MANAGED) and entry.endswith(".dll"):
                    (refs / Path(entry).name).write_bytes(apk.read(entry))
        firstpass_sha = sha((refs / "Assembly-CSharp-firstpass.dll").read_bytes())
        main_input = args.input_managed.read_bytes() if args.input_managed else (refs / "Assembly-CSharp.dll").read_bytes()
        (refs / "Assembly-CSharp.dll").write_bytes(main_input)
        helper = output / "HYJS.BulkPurchase.dll"
        run([dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+", "/out:" + str(helper), *["/r:" + str(x) for x in sorted(refs.glob("*.dll"))], HERE / "Runtime.cs"])
        dnlib = work / "injector/bin/Debug/net8.0/dnlib.dll"
        for name, source, extra in [("Inject", "Injector.cs", '<ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup>'), ("Probe", "ProtoProbe.cs", "")]:
            folder = temp / name
            folder.mkdir()
            shutil.copy2(HERE / source, folder / "Program.cs")
            project = folder / (name + ".csproj")
            project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup>' + extra + '</Project>')
            run([dotnet, "build", project, "--nologo", "--verbosity", "quiet"])
        # Prove unknown tag transport on original unchanged message classes before injection.
        run([dotnet, temp / "Probe/bin/Debug/net8.0/Probe.dll", refs, output / "original-class-tag100-probe.json"])
        main_output = output / "Assembly-CSharp.dll"
        run([dotnet, temp / "Inject/bin/Debug/net8.0/Inject.dll", refs / "Assembly-CSharp.dll", helper, main_output, HERE / "targets.json"])
        protection = verify(main_input, main_output.read_bytes(), targets)
        # Run the actual shipped protobuf-net implementation against the injected main assembly.
        (refs / "Assembly-CSharp.dll").write_bytes(main_output.read_bytes())
        shutil.copy2(helper, refs / helper.name)
        probe = output / "protobuf-tag100-probe.json"
        run([dotnet, temp / "Probe/bin/Debug/net8.0/Probe.dll", refs, probe])
    manifest = dict(schema="hyjs-bulk58-managed-overlay-v1", baseAPKSHA256=FIXED_APK, inputManagedSHA256=sha(main_input), outputManagedSHA256=sha(main_output.read_bytes()), helperSHA256=sha(helper.read_bytes()), firstpassReferenceSHA256=firstpass_sha, protobufStorage="original IExtensible unknown tag100", **protection, sourceSHA256={x.name: sha(x.read_bytes()) for x in [HERE / "Runtime.cs", HERE / "Injector.cs", HERE / "ProtoProbe.cs", HERE / "targets.json", Path(__file__).resolve()]}, protobufTag100ActualImplementationRoundtrip=True, temporaryBuildDirectoryRemoved=True, apkBuilt=False, deviceTested=False, addedFields=json.loads(targets_raw)["addedFields"], replacementEntries={MANAGED + "Assembly-CSharp.dll": sha(main_output.read_bytes()), MANAGED + helper.name: sha(helper.read_bytes())})
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps(manifest, indent=2))


if __name__ == "__main__":
    main()
