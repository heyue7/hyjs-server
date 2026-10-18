"""Small three-character artwork/atlas overlay. Does not package or install an APK."""
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
INPUT_SHA = '5b6ba3af357c36f7e81d7ad82f4ba31157eda6d84dc6ec8f18a1314a3604c8f4'
TARGETS = json.loads((SOURCE / 'target-pins.json').read_text())
ALIASES = {'AssetLoad.AssetBundleManager::' + name for name in ['LoadAssetBundle', 'LoadAssetBundleCoroutine', 'UnloadAssetBundle', 'IsLoadedAssetBundle', 'IsAssetExist']}
PREFIX = {name: 3 for name in ALIASES}
PREFIX.update({'AssetLoad.AssetBundleManager::GetAssetFromBundle': 7, 'AloneTextureKit::SetPath': 3})
SUBSTITUTIONS = {
    'Battle.BattleHeroItem::Init': ('callvirt', 'AssignHead'),
    '<_LoadTexture>c__IteratorCD::MoveNext': ('callvirt', 'AssignHead'),
    '<SetPath>c__AnonStorey186::<>m__D8': ('callvirt', 'AssignCard'),
    'UI.AllHerosPage::SetHeroList': ('ldfld', 'AtlasHidden'),
}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def member(pe, token):
    require(token >> 24 == 10, 'Expected bridge member reference')
    row = pe.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1]
    require(str(row.Class.row.TypeNamespace) == 'HYJS.HeroArt' and str(row.Class.row.TypeName) == 'HeroArtFallback', 'Wrong helper type')
    return str(row.Name)


def verify(before, after, input_sha):
    require(sha(before) == input_sha, 'Input SHA differs')
    left, right = [dnfile.dnPE(data=raw) for raw in [before, after]]
    a, b = methods(left), methods(right)
    require(len(a) == len(b), 'Method inventory changed')
    for name in ['TypeDef', 'Field', 'MethodDef']:
        require(len(getattr(left.net.mdtables, name).rows) == len(getattr(right.net.mdtables, name).rows), 'Original inventory changed: ' + name)
    require(not getattr(right.net.mdtables, 'FieldPtr', None), 'Unexpected old-Unity FieldPtr')
    def assembly_identity(row):
        return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion,
                row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)
    require([assembly_identity(row) for row in left.net.mdtables.Assembly.rows] == [assembly_identity(row) for row in right.net.mdtables.Assembly.rows], 'Original assembly identity/key/flags changed')
    old_refs = left.net.mdtables.AssemblyRef.rows
    new_refs = right.net.mdtables.AssemblyRef.rows
    require([assembly_identity(row) for row in new_refs[:len(old_refs)]] == [assembly_identity(row) for row in old_refs], 'Original assembly references changed')
    require(len(new_refs) == len(old_refs) + 1 and str(new_refs[-1].Name) == 'SoulFlowerHeroArt', 'Unexpected assembly reference additions')
    for row in new_refs:
        require(not (row.struct.Flags & 1) or bool(row.PublicKey.value), 'AssemblyRef has empty full PublicKey: ' + str(row.Name))
    require(new_refs[-1].struct.Flags == 0 and new_refs[-1].PublicKey.value == b'', 'Helper must have empty PublicKeyToken without PublicKey flag')
    require(left.net.struct.Flags == right.net.struct.Flags, 'CLI flags changed')
    require(left.net.struct.StrongNameSignatureSize == right.net.struct.StrongNameSignatureSize == 0, 'Unexpected strong-name signature directory')
    changes = []
    engine = unchanged = 0
    for (name, rid, old), (newname, newrid, new) in zip(a, b):
        require((name, rid) == (newname, newrid) and old.Signature.value == new.Signature.value and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags, 'Original ABI/token changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'New original body: ' + name)
            continue
        x, y = exact_body(left, old), exact_body(right, new)
        if name not in TARGETS:
            require(x == y, 'Unrelated original raw method changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
            continue
        require(sha(x) == TARGETS[name], 'Original target pin differs: ' + name)
        ob, oc = decoded(left, old)
        nb, nc = decoded(right, new)
        prefix = PREFIX.get(name, 0)
        require(len(nc) == len(oc) + prefix, 'Target instruction count differs: ' + name)
        suffix = []
        for op, value in nc[prefix:]:
            if op == 'switch':
                value = tuple(v - prefix for v in value)
            elif op.startswith(('br', 'beq', 'bne', 'bge', 'bgt', 'ble', 'blt', 'leave')) and isinstance(value, int):
                value -= prefix
            suffix.append((op, value))
        if name in SUBSTITUTIONS:
            differences = [i for i in range(len(oc)) if oc[i] != suffix[i]]
            require(len(differences) == 1, 'Expected one bounded instruction substitution: ' + name)
            index = differences[0]
            require(oc[index][0] == SUBSTITUTIONS[name][0] and suffix[index][0] == 'call', 'Unexpected substitution opcode')
            require(member(right, suffix[index][1]) == SUBSTITUTIONS[name][1], 'Wrong crop/atlas helper')
        else:
            require(oc == suffix, 'Original target suffix changed: ' + name)
            calls = [member(right, value) for op, value in nc[:prefix] if op == 'call']
            expect = ['MapAsset', 'MapBundle'] if name.endswith('::GetAssetFromBundle') else ['PrepareCard'] if name == 'AloneTextureKit::SetPath' else ['MapBundle']
            require(calls == expect, 'Unexpected boundary helper')
        require(len(ob.exception_handlers) == len(nb.exception_handlers), 'Exception handler count changed')
        # All target EH bounds must retain exactly the same instruction boundaries.
        oi = {v.offset: i for i, v in enumerate(ob.instructions)}
        ni = {v.offset: i for i, v in enumerate(nb.instructions)}
        oi[ob.instructions[-1].offset + ob.instructions[-1].size] = len(oc)
        ni[nb.instructions[-1].offset + nb.instructions[-1].size] = len(nc)
        for oe, ne in zip(ob.exception_handlers, nb.exception_handlers):
            for key in ['try_start', 'try_end', 'handler_start', 'handler_end']:
                require(ni[getattr(ne, key)] - prefix == oi[getattr(oe, key)], 'EH scope changed: ' + name)
        changes.append(dict(method=name, beforeSHA256=sha(x), afterSHA256=sha(y), prefixInstructions=prefix))
    require(engine == 547 and len(changes) == len(TARGETS), 'Engine or target count differs')
    return dict(changes=changes, originalEngineMethodsUnchanged=engine, unrelatedRawMethodBodiesUnchanged=unchanged, originalMetadataABIAndTokensPreserved=True,
                originalAssemblyIdentityAndReferencesPreserved=True, addedAssemblyReferencePublicKeyFlags=0, cliFlagsPreserved=True, strongNameSignatureBytes=0)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, default=ROOT / 'apks/build/hyjs-local.apk')
    parser.add_argument('--input-managed', type=Path, required=True)
    parser.add_argument('--input-managed-sha256', default=INPUT_SHA)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    before = args.input_managed.read_bytes()
    require(sha(before) == args.input_managed_sha256, 'Exact input required')
    out = args.output_directory.resolve()
    require(not out.exists(), 'Use fresh output directory')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'

    def run(name, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=90)
        (out / (name + '.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + ' failed; inspect saved log')

    with tempfile.TemporaryDirectory(prefix='hyjs-hero-art-') as temporary:
        work = Path(temporary)
        host = work / 'Assembly-CSharp.dll'
        host.write_bytes(before)
        references = [host]
        with zipfile.ZipFile(args.base_apk) as archive:
            for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll']:
                target = work / name
                target.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
                references.append(target)
        bridge = out / 'SoulFlowerHeroArt.dll'
        run('compile-bridge', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(bridge), *['/reference:' + str(p) for p in references], SOURCE / 'HeroArt.cs'])
        shutil.copy2(SOURCE / 'Injector.cs', work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        run('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', host, bridge, out / 'Assembly-CSharp.dll'])
        consumer = work / 'MetadataConsumer.cs'
        consumer.write_text('public static class MetadataConsumer { public static UI.UIHero Keep(UI.UIHero hero) { return hero; } }')
        run('compile-metadata-consumer', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/out:' + str(work / 'MetadataConsumer.dll'),
             '/reference:' + str(out / 'Assembly-CSharp.dll'), *['/reference:' + str(p) for p in references[1:]], consumer])
    after = (out / 'Assembly-CSharp.dll').read_bytes()
    checks = verify(before, after, args.input_managed_sha256)
    report = dict(schema='hyjs-three-original-hero-art-v1', status='STATIC_VERIFIED_NOT_PACKAGED', inputManagedSHA256=sha(before), outputManagedSHA256=sha(after), bridgeSHA256=sha(bridge.read_bytes()),
                  deviceExecuted=False, threeDimensionalModelRootCauseProven=False, resourceFilesModified=False, oldProfileCompilerAcceptedOutputAsReference=True,
                  replacementEntries=['assets/bin/Data/Managed/Assembly-CSharp.dll', 'assets/bin/Data/Managed/SoulFlowerHeroArt.dll'],
                  sourcePins={str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [SOURCE / 'build.py', SOURCE / 'HeroArt.cs', SOURCE / 'Injector.cs', SOURCE / 'target-pins.json']}, **checks)
    (out / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({k: report[k] for k in ['status', 'outputManagedSHA256', 'bridgeSHA256', 'originalEngineMethodsUnchanged', 'unrelatedRawMethodBodiesUnchanged']}))


if __name__ == '__main__':
    main()
