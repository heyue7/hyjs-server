"""Build one-key selection59 managed overlay; no APK packaging or installation."""
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


def full_body(pe, method):
    header=pe.get_data(method.Rva,12)
    if header[0]&3==2: length=1+(header[0]>>2)
    else:
        flags=struct.unpack_from("<H",header)[0]
        length=(flags>>12)*4+struct.unpack_from("<I",header,4)[0]
        if flags&8:
            at=(length+3)&~3
            while True:
                section=pe.get_data(method.Rva+at,4)
                size=int.from_bytes(section[1:4],"little") if section[0]&0x40 else section[1]
                assert size>=4
                at+=size
                if not section[0]&0x80:break
                at=(at+3)&~3
            length=at
    return pe.get_data(method.Rva,length)

def verify(before_raw, after_raw, targets):
    a, b = dnfile.dnPE(data=before_raw), dnfile.dnPE(data=after_raw)
    allowed = {int(x["token"], 16) & 0xffffff for x in targets}
    changed, untouched, engine = [], 0, 0
    assert len(a.net.mdtables.MethodDef.rows) == len(b.net.mdtables.MethodDef.rows)
    for owner in a.net.mdtables.TypeDef.rows:
        for method in owner.MethodList:
            x, y = method.row, b.net.mdtables.MethodDef.rows[method.row_index - 1]
            assert str(x.Name) == str(y.Name) and x.Signature.value == y.Signature.value and x.struct.Flags == y.struct.Flags and x.struct.ImplFlags == y.struct.ImplFlags
            if not x.Rva:
                continue
            same = full_body(a, x) == full_body(b, y)
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
    for old, new in zip(a.net.mdtables.TypeRef.rows, b.net.mdtables.TypeRef.rows):
        assert str(old.TypeNamespace)==str(new.TypeNamespace) and str(old.TypeName)==str(new.TypeName)
        if (old.ResolutionScope.table.name,old.ResolutionScope.row_index)!=(new.ResolutionScope.table.name,new.ResolutionScope.row_index):
            assert old.ResolutionScope.table.name=="ModuleRef" and str(old.ResolutionScope.row.Name)=="InstanceProtection.dll"
            assert str(old.TypeNamespace)+"."+str(old.TypeName) in ["UI.UIHero","UI.UIEquipment","UI.UITreasure"]
            assert new.ResolutionScope.table.name=="Module" and new.ResolutionScope.row_index==1
    bad = []
    for row in b.net.mdtables.TypeRef.rows:
        if row.ResolutionScope.table.name == "ModuleRef" and str(row.ResolutionScope.row.Name) == "HYJS.OneKey59.dll":
            bad.append(str(row.TypeNamespace) + "." + str(row.TypeName))
    assert not bad, bad
    for row in b.net.mdtables.TypeRef.rows:
        assert not (row.ResolutionScope.table.name == "ModuleRef" and str(row.ResolutionScope.row.Name) == "InstanceProtection.dll"), "Inherited protection Main type scope unrepaired"
    assert len(b.net.mdtables.Field.rows) == len(a.net.mdtables.Field.rows)
    assert len(b.net.mdtables.TypeDef.rows) == len(a.net.mdtables.TypeDef.rows)
    for name in ["FieldPtr", "MethodPtr"]:
        assert not getattr(a.net.mdtables, name, None), "Input must retain no " + name
        assert not getattr(b.net.mdtables, name, None), "Output must retain no " + name
    for old, new in zip(a.net.mdtables.TypeDef.rows, b.net.mdtables.TypeDef.rows):
        assert [x.row_index for x in old.FieldList] == [x.row_index for x in new.FieldList]
        assert [x.row_index for x in old.MethodList] == [x.row_index for x in new.MethodList]
    return {"nonTargetRawCILExact": untouched, "nonTargetHeadersLocalsAndEHBytesExact": True, "changedMethodTokens": [hex(0x06000000 + x) for x in changed], "originalEngineMethods": engine, "originalEngineCILAndEHExact": True, "originalDefinitionSignaturesAndFieldRIDsExact": True, "helperScopedMainTypeModuleRefs": bad, "addedFieldCount": 0, "fieldPtrAbsent": True, "methodPtrAbsent": True}


def iterator_proof(before, after):
    a, b = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    rid = 0x6904
    bodies = [read_method_body_from_bytes(pe.get_data(pe.net.mdtables.MethodDef.rows[rid-1].Rva)) for pe in [a,b]]
    old, new = bodies
    stripped=[]; mapped={}; index=0; hooks=0
    ins=new.instructions
    while index < len(ins):
        match=False
        if index+3<len(ins) and ins[index].opcode.name=="ldarg.0" and ins[index+1].opcode.name=="ldfld" and ins[index+2].opcode.name=="call" and ins[index+3].opcode.name=="ret":
            token=ins[index+2].operand.value
            row=b.net.mdtables.MemberRef.rows[(token&0xffffff)-1] if token>>24==10 else None
            match=row is not None and str(row.Name)=="Draw" and str(row.Class.row.TypeNamespace)=="HYJS.OneKey59"
        if match:
            for x in ins[index:index+4]:mapped[x.offset]=len(stripped)
            stripped.append(("ret",None));index+=4;hooks+=1
        else:
            mapped[ins[index].offset]=len(stripped);stripped.append((ins[index].opcode.name,ins[index].operand));index+=1
    oldmap={x.offset:i for i,x in enumerate(old.instructions)}
    mapped[new.header_size+new.code_size]=len(stripped);oldmap[old.header_size+old.code_size]=len(old.instructions)
    def value(op, operand, offsets):
        if operand is None:return None
        if hasattr(operand,"value"):return operand.value
        if op=="switch":return [offsets[x] for x in operand]
        if op.startswith(("br","beq","bge","bgt","ble","blt","bne","leave")):return offsets[operand]
        return str(operand)
    expected=[(x.opcode.name,value(x.opcode.name,x.operand,oldmap)) for x in old.instructions]
    actual=[(op,value(op,arg,mapped)) for op,arg in stripped]
    assert expected==actual, "Iterator original evaluation/control sequence differs"
    def regions(body, offsets):
        return [(x.exception_type,offsets[x.try_start+body.header_size],offsets[x.try_end+body.header_size],offsets[x.handler_start+body.header_size],offsets[x.handler_end+body.header_size],getattr(x.catch_type,"value",None)) for x in body.exception_handlers]
    assert regions(old,oldmap)==regions(new,mapped), "Iterator EH ownership differs"
    assert getattr(old.local_var_sig_tok,"value",None)==getattr(new.local_var_sig_tok,"value",None)
    return dict(iteratorOriginalSequenceExactAfterRemovingRetDraw=True,iteratorExceptionRegionsMappedExact=True,iteratorOriginalLocalsExact=True,iteratorBoolReturnPathsPreserved=hooks)

def success_proof(before, after):
    a,b=dnfile.dnPE(data=before),dnfile.dnPE(data=after)
    old,new=[read_method_body_from_bytes(pe.get_data(pe.net.mdtables.MethodDef.rows[0x4c24-1].Rva)) for pe in [a,b]]
    ins=new.instructions
    assert [x.opcode.name for x in ins[:5]]==["ldarg.2","brfalse","ldarg","call","ldnull"]
    assert ins[2].operand.index == 0
    ref=b.net.mdtables.MemberRef.rows[(ins[3].operand.value&0xffffff)-1]
    assert str(ref.Name)=="BeforeSuccessfulRefresh" and str(ref.Class.row.TypeNamespace)=="HYJS.OneKey59"
    kept=ins[:2]+ins[4:]
    oldmap={x.offset:i for i,x in enumerate(old.instructions)};newmap={x.offset:i for i,x in enumerate(kept)}
    oldmap[old.header_size+old.code_size]=len(old.instructions);newmap[new.header_size+new.code_size]=len(kept)
    def val(op,arg,offsets):
        if arg is None:return None
        if hasattr(arg,"value"):return arg.value
        if op=="switch":return [offsets[x] for x in arg]
        if op.startswith(("br","beq","bge","bgt","ble","blt","bne","leave")):return offsets[arg]
        return str(arg)
    normalize=lambda rows,offsets:[(x.opcode.name,val(x.opcode.name,x.operand,offsets)) for x in rows]
    assert normalize(old.instructions,oldmap)==normalize(kept,newmap)
    def regions(body,offsets):
        return [(x.exception_type,offsets[x.try_start+body.header_size],offsets[x.try_end+body.header_size],offsets[x.handler_start+body.header_size],offsets[x.handler_end+body.header_size],getattr(x.catch_type,"value",None)) for x in body.exception_handlers]
    assert regions(old,oldmap)==regions(new,newmap)
    assert getattr(old.local_var_sig_tok,"value",None)==getattr(new.local_var_sig_tok,"value",None)
    assert newmap[ins[1].operand]==oldmap[old.instructions[1].operand]
    return dict(successHookOnlyAfterOriginalBoolBranch=True,failureBranchSkipsHelper=True,originalSuccessBodyAndEHExactAfterRemovingTwoHookInstructions=True,originalSingleRefreshPreserved=True)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-apk", type=Path, default=ROOT / "build/hyjs-feedback55.apk")
    parser.add_argument("--input-managed", type=Path, required=True)
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
    with tempfile.TemporaryDirectory(prefix="hyjs59-onekey-build-") as temp:
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
        helper = output / "HYJS.OneKey59.dll"
        run([dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+", "/out:" + str(helper), *["/r:" + str(x) for x in sorted(refs.glob("*.dll"))], HERE / "Selection.cs"])
        dnlib = work / "injector/bin/Debug/net8.0/dnlib.dll"
        folder = temp / "Inject"; folder.mkdir()
        shutil.copy2(HERE / "Injector.cs", folder / "Program.cs")
        project = folder / "Inject.csproj"
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run([dotnet, "build", project, "--nologo", "--verbosity", "quiet", "-p:NuGetAudit=false"])
        recycle_source = ROOT / "tools/client_recycle_selection/Selection.cs"
        recycle = output / "SoulFlowerRecycle.dll"
        run([dotnet, compiler, "/nologo", "/target:library", "/nostdlib+", "/langversion:3", "/deterministic+", "/out:"+str(recycle), *["/r:"+str(x) for x in sorted(refs.glob("*.dll")) if x.name != "SoulFlowerRecycle.dll"], recycle_source])
        main_output = output / "Assembly-CSharp.dll"
        run([dotnet, temp / "Inject/bin/Debug/net8.0/Inject.dll", refs / "Assembly-CSharp.dll", helper, main_output, HERE / "targets.json"])
        protection = verify(main_input, main_output.read_bytes(), targets)
        protection.update(iterator_proof(main_input, main_output.read_bytes()))
        protection.update(success_proof(main_input, main_output.read_bytes()))
        metadata = (output / "metadata-only-Assembly-CSharp.dll").read_bytes()
        metadata_protection = verify(main_input, metadata, [])
        (refs / "Assembly-CSharp.dll").write_bytes(main_output.read_bytes())
        shutil.copy2(helper, refs / helper.name); shutil.copy2(recycle, refs / recycle.name)
        probe_folder=temp / "Probe"; probe_folder.mkdir()
        shutil.copy2(HERE / "Probe.cs", probe_folder / "Program.cs")
        probe_project=probe_folder / "Probe.csproj"
        probe_project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup></Project>')
        run([dotnet,"build",probe_project,"--nologo","--verbosity","quiet","-p:NuGetAudit=false"])
        run([dotnet,probe_folder / "bin/Debug/net8.0/Probe.dll",refs,output / "selection-policy-probe.json"])
    manifest = dict(schema="hyjs-onekey59-managed-overlay-v1", baseAPKSHA256=FIXED_APK, inputManagedSHA256=sha(main_input), outputManagedSHA256=sha(main_output.read_bytes()), helperSHA256=sha(helper.read_bytes()), firstpassReferenceSHA256=firstpass_sha, metadataOnlyProof=metadata_protection, metadataRepairs=json.loads((output / "metadata-repairs.json").read_text()), **protection, sourceSHA256={x.name: sha(x.read_bytes()) for x in [HERE / "Selection.cs", HERE / "Injector.cs", HERE / "Probe.cs", HERE / "targets.json", Path(__file__).resolve()]}, temporaryBuildDirectoryRemoved=True, apkBuilt=False, deviceTested=False, addedFields=[], recycleHelperSHA256=sha(recycle.read_bytes()), recycleSourceSHA256=sha(recycle_source.read_bytes()), selectionPolicyProbePassed=True, replacementEntries={MANAGED + "Assembly-CSharp.dll": sha(main_output.read_bytes()), MANAGED + helper.name: sha(helper.read_bytes()), MANAGED + recycle.name: sha(recycle.read_bytes())})
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps(manifest, indent=2))


if __name__ == "__main__":
    main()
