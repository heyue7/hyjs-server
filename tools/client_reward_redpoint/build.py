"""Compose original reward-redpoint queries into a managed overlay without packaging an APK."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.local_asset_ready.remove_gate import exact_body, require

SOURCE = Path(__file__).resolve().parent
TARGETS = json.loads((SOURCE / 'target-pins.json').read_text())


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def verify(before, after):
    a, b = [dnfile.dnPE(data=raw) for raw in (before, after)]
    for table in ['TypeDef', 'Field', 'MethodDef', 'AssemblyRef', 'MemberRef']:
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows), 'Metadata inventory changed: ' + table)
    require(not getattr(b.net.mdtables, 'FieldPtr', None), 'Old Unity must not gain FieldPtr')
    require(a.net.mdtables.Module.rows[0].Mvid.value == b.net.mdtables.Module.rows[0].Mvid.value, 'MVID changed')
    require(a.net.struct.Flags == b.net.struct.Flags and a.net.struct.StrongNameSignatureSize == b.net.struct.StrongNameSignatureSize == 0, 'CLI identity changed')
    for old, new in zip(a.net.mdtables.Field.rows, b.net.mdtables.Field.rows):
        require(str(old.Name) == str(new.Name) and old.Signature.value == new.Signature.value and old.struct.Flags == new.struct.Flags, 'Original field ABI changed')
    left, right = methods(a), methods(b)
    require(len(left) == len(right), 'Method inventory changed')
    names = {rid: name for name, rid, method in right}
    changes = []
    unchanged = engine = 0
    for (name, rid, old), (new_name, new_rid, new) in zip(left, right):
        require((name, rid) == (new_name, new_rid) and old.Signature.value == new.Signature.value and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags, 'Original method ABI/token changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'Bodyless method gained code')
            continue
        x, y = exact_body(a, old), exact_body(b, new)
        if name not in TARGETS:
            require(x == y, 'Unrelated raw method changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
            continue
        require(sha(x) == TARGETS[name], 'Target source pin differs: ' + name)
        ob, oc = decoded(a, old)
        nb, nc = decoded(b, new)
        require(not ob.exception_handlers and not nb.exception_handlers, 'Unexpected target EH')
        require(oc[-1] == ('ret', None) and len(nc) == len(oc) + 3, 'Unexpected query suffix')
        require(nc[-4] == ('ldc.i4', 20117) and nc[-3][0] == 'newobj' and nc[-2][0] == 'call' and nc[-1] == ('ret', None), 'Wrong original socket query')
        require(names[nc[-3][1] & 0xffffff] == 'ProtoBuf.Message.CSRedPoint::.ctor' and names[nc[-2][1] & 0xffffff] == 'NetAPI::SendSocket', 'Wrong query APIs')
        # Retain the original return as failure branch destination; remove only
        # the three added fallthrough instructions before comparing all code.
        indexes = {i: i for i in range(len(oc) - 1)}
        indexes[len(nc) - 1] = len(oc) - 1
        filtered = []
        for i, (op, value) in enumerate(nc):
            if i not in indexes:
                continue
            if op == 'switch':
                value = tuple(indexes[v] for v in value)
            elif op.startswith(('br', 'beq', 'bne', 'bge', 'bgt', 'ble', 'blt', 'leave')) and isinstance(value, int):
                require(value in indexes, 'Original branch enters the new query')
                value = indexes[value]
            filtered.append((op, value))
        require(filtered == oc, 'Original callback behavior changed: ' + name)
        changes.append(dict(method=name, token=hex(0x6000000 | rid), beforeSHA256=sha(x), afterSHA256=sha(y)))
    require(engine == 547 and len(changes) == 2, 'Wrong target or original Engine count')
    return dict(changes=changes, unrelatedRawMethodBodiesUnchanged=unchanged, originalEngineMethodsUnchanged=engine, originalMetadataABIAndTokensPreserved=True, noNewAssemblyReferences=True, rejectedCallbackBranchesSkipQuery=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input-managed', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    before = args.input_managed.read_bytes()
    out = args.output_directory.resolve()
    require(out.is_relative_to(ROOT / 'analysis') and not out.exists(), 'Use a fresh analysis output directory')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'
    with tempfile.TemporaryDirectory(prefix='hyjs-reward-redpoint-') as temporary:
        work = Path(temporary)
        host = work / 'Assembly-CSharp.dll'
        host.write_bytes(before)
        shutil.copy2(SOURCE / 'Injector.cs', work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        for name, command in [
            ('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false']),
            ('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', host, out / 'Assembly-CSharp.dll']),
        ]:
            run = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=90)
            (out / (name + '.log')).write_text(run.stdout + run.stderr)
            require(run.returncode == 0, name + ' failed; inspect saved log')
    after = (out / 'Assembly-CSharp.dll').read_bytes()
    proof = dict(schema='hyjs-client-reward-redpoint-post-callback-v1', status='STATIC_VERIFIED_NOT_PACKAGED', inputManagedSHA256=sha(before), outputManagedSHA256=sha(after), queriesOriginalOpcode=20117, queryAfterOriginalLocalCleanup=True, noNewBusinessMutationRequest=True, noNewHelperAssembly=True, deviceExecuted=False, apkBuilt=False, replacementEntries=['assets/bin/Data/Managed/Assembly-CSharp.dll'], sourcePins={str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [SOURCE / 'build.py', SOURCE / 'Injector.cs', SOURCE / 'target-pins.json']}, **verify(before, after))
    (out / 'manifest.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({k: proof[k] for k in ['status', 'inputManagedSHA256', 'outputManagedSHA256', 'unrelatedRawMethodBodiesUnchanged', 'originalEngineMethodsUnchanged']}))


if __name__ == '__main__':
    main()
