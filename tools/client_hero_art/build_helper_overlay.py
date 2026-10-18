"""Compose a helper update and one async texture hook over the fixed current APK."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.local_asset_ready.remove_gate import exact_body, require

SOURCE = Path(__file__).resolve().parent
BASE_SHA = '0a5e72067c621b54e96524e61b29ebcd50d77e088d86834af5691bb2fb16d6e8'
MAIN_SHA = 'd00e52f6d301fa1752023a791cd974de4ec5051ceca03cedd6a58ca613301886'
TARGET = '<_SetTextureAsync>c__Iterator2A::MoveNext'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def verify(before, after):
    left, right = [dnfile.dnPE(data=raw) for raw in [before, after]]
    a, b = methods(left), methods(right)
    require(len(a) == len(b), 'Method inventory differs')
    for name in ['TypeDef', 'Field', 'MethodDef', 'AssemblyRef']:
        require(len(getattr(left.net.mdtables, name).rows) == len(getattr(right.net.mdtables, name).rows), 'Original inventory differs: ' + name)
    unchanged = engine = 0
    changes = []
    for (name, rid, old), (newname, newrid, new) in zip(a, b):
        require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) == (newname, newrid, new.Signature.value, new.struct.Flags, new.struct.ImplFlags), 'Original ABI differs: ' + name)
        if not old.Rva:
            require(not new.Rva, 'Unexpected new body')
            continue
        x, y = exact_body(left, old), exact_body(right, new)
        if name != TARGET:
            require(x == y, 'Unrelated original body changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
            continue
        ob, oc = decoded(left, old)
        nb, nc = decoded(right, new)
        require(len(nc) == len(oc) + 2, 'Expected two path-argument instructions')
        differences = []
        # Normalize new instruction indexes after the two path loads, preserving
        # exact original branch destinations and exception boundaries.
        index = next(i for i, (op, operand) in enumerate(nc) if op == 'call' and operand >> 24 == 10 and str(right.net.mdtables.MemberRef.rows[(operand & 0xffffff) - 1].Name) == 'AssignRequested')
        require(nc[index - 2][0] == 'ldarg.0' and nc[index - 1][0] == 'ldfld', 'Missing original bundleName argument')
        bundle = right.net.mdtables.Field.rows[(nc[index - 1][1] & 0xffffff) - 1]
        require(str(bundle.Name) == 'bundleName', 'Unexpected request field')
        def original_index(i):
            require(i not in [index - 2, index - 1], 'Original branch targets inserted helper argument')
            return i - 2 if i >= index else i
        reduced = []
        for i, (op, value) in enumerate(nc):
            if i in [index - 2, index - 1]:
                continue
            if op == 'switch':
                value = tuple(original_index(v) for v in value)
            elif op.startswith(('br', 'beq', 'bne', 'bge', 'bgt', 'ble', 'blt', 'leave')) and isinstance(value, int):
                value = original_index(value)
            reduced.append((op.replace('.s', '') if op.startswith(('br', 'beq', 'bne', 'bge', 'bgt', 'ble', 'blt', 'leave')) else op, value))
        normalized_old = [(op.replace('.s', '') if op.startswith(('br', 'beq', 'bne', 'bge', 'bgt', 'ble', 'blt', 'leave')) else op, val) for op, val in oc]
        differences = [i for i in range(len(oc)) if normalized_old[i] != reduced[i]]
        require(len(differences) == 2, 'Expected only assignment and pixel-perfect substitutions')
        called = []
        for i in differences:
            require(oc[i][0] == 'callvirt' and reduced[i][0] == 'call', 'Unexpected replacement opcode')
            ref = right.net.mdtables.MemberRef.rows[(reduced[i][1] & 0xffffff) - 1]
            require(str(ref.Class.row.TypeNamespace) == 'HYJS.HeroArt' and str(ref.Class.row.TypeName) == 'HeroArtFallback', 'Unexpected helper class')
            called.append(str(ref.Name))
        require(called == ['AssignRequested', 'MakeRequestedPixelPerfect'], 'Unexpected helper calls')
        require(not ob.exception_handlers and not nb.exception_handlers, 'Unexpected async exception regions')
        changes.append(dict(method=name, beforeSHA256=sha(x), afterSHA256=sha(y), insertedInstructions=2, substitutedCalls=called))
    require(engine == 547 and len(changes) == 1, 'Expected all 547 engine methods and one original change')
    require(left.net.struct.Flags == right.net.struct.Flags, 'CLI flags differ')
    for row in right.net.mdtables.AssemblyRef.rows:
        require(not (row.struct.Flags & 1) or bool(row.PublicKey.value), 'Empty public-key reference')
    return dict(changes=changes, originalEngineMethodsUnchanged=engine, unrelatedOriginalRawBodiesUnchanged=unchanged, originalMetadataABIAndTokensPreserved=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, default=ROOT / 'apks/build/hyjs-local.apk')
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    require(hashlib.file_digest(args.base_apk.open('rb'), 'sha256').hexdigest() == BASE_SHA, 'Fixed current APK identity differs')
    out = args.output_directory.resolve()
    require(not out.exists(), 'Use a fresh output directory')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'
    def run(name, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=90)
        (out / (name + '.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + ' failed; inspect saved log')
    with zipfile.ZipFile(args.base_apk) as archive, tempfile.TemporaryDirectory(prefix='hyjs-hero-size-') as temporary:
        work = Path(temporary)
        before = archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
        require(sha(before) == MAIN_SHA, 'Fixed original managed identity differs')
        host = work / 'Assembly-CSharp.dll'
        host.write_bytes(before)
        references = [host]
        for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll']:
            target = work / name
            target.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
            references.append(target)
        helper = out / 'SoulFlowerHeroArt.dll'
        run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(helper), *['/reference:' + str(p) for p in references], SOURCE / 'HeroArt.cs'])
        shutil.copy2(SOURCE / 'AsyncInjector.cs', work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        run('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', host, out / 'Assembly-CSharp.dll'])
    after = (out / 'Assembly-CSharp.dll').read_bytes()
    result = dict(schema='hyjs-original-hero-widget-size-v1', status='STATIC_VERIFIED_NOT_PACKAGED', inputAPK_SHA256=BASE_SHA, inputManagedSHA256=MAIN_SHA, outputManagedSHA256=sha(after), bridgeSHA256=sha(helper.read_bytes()), deviceExecuted=False, sourcePins={str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [SOURCE / 'HeroArt.cs', SOURCE / 'AsyncInjector.cs', SOURCE / 'build_helper_overlay.py']}, replacementEntries=['assets/bin/Data/Managed/Assembly-CSharp.dll', 'assets/bin/Data/Managed/SoulFlowerHeroArt.dll'], **verify(before, after))
    (out / 'manifest.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
