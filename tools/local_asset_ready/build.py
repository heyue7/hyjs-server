"""Build only the scoped Awake501 readiness DLL overlay. Never packages/installs an APK."""
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
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.p10_voice.build import verify

MANAGED = 'assets/bin/Data/Managed/'
SCOPE_SHA = 'c10add31ab1ba521150b9072366bf68ef3e03728d213bcddf5e10115d3b5dc7c'
SCOPE = ROOT / 'analysis/p7/awake-assets-ready-20261016/local501-scope.json'
ORIGINAL = 'b1d99496c7ffe17c45ab7646c72023be406924997d471e3f18db169fb0dfd641'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def run(args):
    subprocess.run(['rtk', 'proxy', *map(str, args)], cwd=ROOT, check=True)


def verify_gate_instructions(before_raw, after_raw):
    """Remove only the two new calls and compare all original control flow/EH."""
    before, after = dnfile.dnPE(data=before_raw), dnfile.dnPE(data=after_raw)
    checks = []
    for owner, name, helper in [('Room.RCUINormalNPC', 'OpenPage', 'CatalogueReady'),
                                ('AwakeDiffItem', 'OnChallengeClick', 'EntryReady')]:
        def method(pe):
            return next(m.row for t in pe.net.mdtables.TypeDef
                        if '.'.join(filter(None, [str(t.TypeNamespace), str(t.TypeName)])) == owner
                        for m in t.MethodList if str(m.row.Name) == name)
        old, new = method(before), method(after)
        a, b = (read_method_body_from_bytes(pe.get_data(m.Rva)) for pe, m in [(before, old), (after, new)])
        additions = []
        for i, inst in enumerate(b.instructions):
            token = getattr(inst.operand, 'value', 0)
            if inst.opcode.name != 'call' or token >> 24 != 0x0a:
                continue
            ref = after.net.mdtables.MemberRef.rows[(token & 0xffffff) - 1]
            if str(ref.Name) == helper and str(ref.Class.row.TypeName) == 'LocalAssetReady':
                if (i < 2 or b.instructions[i - 1].opcode.name != 'ldarg.0'
                        or b.instructions[i - 2].opcode.name != 'callvirt'):
                    raise ValueError('Adapter must follow original full getter')
                getter = after.net.mdtables.MethodDef.rows[(b.instructions[i-2].operand.value & 0xffffff)-1]
                if str(getter.Name) != 'get_IsAssetFull':
                    raise ValueError('Adapter replaced a different result')
                additions.append((i-1, i))
        if len(additions) != 1:
            raise ValueError('Expected one new adapter call per original gate')
        removed = set(additions[0])
        def shape(body, omit):
            kept = [x for i, x in enumerate(body.instructions) if i not in omit]
            offsets = {x.offset: i for i, x in enumerate(kept)}
            offsets[body.header_size + body.code_size] = len(kept)
            result = []
            for x in kept:
                op, value = x.opcode.name, x.operand
                if op.startswith(('br', 'beq', 'bne', 'bge', 'bgt', 'ble', 'blt', 'leave')):
                    value = offsets[value]
                elif op == 'switch':
                    value = tuple(offsets[v] for v in value)
                elif hasattr(value, 'value'):
                    value = value.value
                else:
                    value = str(value)
                result.append((op, value))
            handlers = [(h.exception_type, *[offsets[p + body.header_size] for p in
                         [h.try_start, h.try_end, h.handler_start, h.handler_end]],
                         h.filter_start if h.filter_start < 0 else offsets[h.filter_start + body.header_size],
                         getattr(h.catch_type, 'value', None)) for h in body.exception_handlers]
            return result, handlers
        if old.Signature.value != new.Signature.value or shape(a, set()) != shape(b, removed):
            raise ValueError('Original gate instructions/control flow/EH changed: ' + owner + '::' + name)
        checks.append(owner + '::' + name)
    return checks


def verify_scope_apk(apk_path, scope, existing_bridge=False):
    resources = scope['resources']
    if (scope['schema'] != 'local-awake501-original-resource-closure-v1'
            or scope['originalAPKSHA256'] != ORIGINAL or scope['allowedCopyID'] != '501'
            or scope['allowedChapterID'] != '5' or scope['allowedRenderQuality'] != 2
            or scope['globalFullAssetReady'] is not False or len(resources) != 2680
            or scope['dependencyBundles'] != 755 or scope['builtInFiles'] != 1923):
        raise ValueError('Frozen original scope differs')
    names = [r['entry'] for r in resources]
    if len(set(names)) != len(names):
        raise ValueError('Duplicate proof resource')
    with zipfile.ZipFile(apk_path) as apk:
        if len(apk.namelist()) != len(set(apk.namelist())):
            raise ValueError('Duplicate APK entries')
        for row in resources:
            name = row['entry']
            if not (name.startswith('assets/android/') or
                    name.startswith('assets/bin/Data/') and not name.startswith(MANAGED)):
                raise ValueError('Scope includes non-resource data')
            raw = apk.read(name)
            if len(raw) != row['size'] or sha(raw) != row['sha256'] or hashlib.md5(raw).hexdigest() != row['md5']:
                raise ValueError('Original owned resource differs: ' + name)
        assembly = apk.read(MANAGED + 'Assembly-CSharp.dll')
        if not existing_bridge and MANAGED + 'LocalAssetReady.dll' in apk.namelist():
            raise ValueError('This mode requires a base without LocalAssetReady')
    text = scope['schema'] + '\n' + ''.join(
        f"{r['entry']}\t{r['size']}\t{r['sha256']}\t{r['md5']}\n"
        for r in sorted(resources, key=lambda r: r['entry']))
    return assembly, text.encode()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--base-proof', type=Path, required=True,
                        help='Existing qualified wallet/IME APK composition proof')
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    if args.output_directory.exists():
        raise ValueError('Use a new overlay output directory')
    base_sha = hashlib.file_digest(args.base_apk.open('rb'), 'sha256').hexdigest()
    base_proof = json.loads(args.base_proof.read_text())
    if base_sha != args.base_sha256 or base_proof['binarySHA256'] != base_sha:
        raise ValueError('Base APK/proof SHA differs')
    if not base_proof.get('signatureVerified') or not base_proof.get('zipalignVerified'):
        raise ValueError('Qualified base signature/zipalign proof required')
    scope_raw = SCOPE.read_bytes()
    if sha(scope_raw) != SCOPE_SHA:
        raise ValueError('Frozen original closure proof changed')
    scope = json.loads(scope_raw)
    assembly, inventory = verify_scope_apk(args.base_apk, scope)
    if base_proof['qualifiedWalletEntries'][MANAGED + 'Assembly-CSharp.dll'] != sha(assembly):
        raise ValueError('Qualified base managed DLL differs')
    with zipfile.ZipFile(args.base_apk) as apk:
        for entry, expected in base_proof['qualifiedWalletEntries'].items():
            if sha(apk.read(entry)) != expected:
                raise ValueError('Qualified wallet/IME entry differs: ' + entry)
    output = args.output_directory
    output.mkdir(parents=True)
    (output / 'local501.inventory').write_bytes(inventory)
    work = ROOT / '.tools/native-batch'
    dotnet = work / 'dotnet/dotnet'
    compiler = next((work / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = sorted((ROOT / 'analysis/extracted/assets/bin/Data/Managed').glob('*.dll'))
    bridge = output / 'LocalAssetReady.dll'
    with tempfile.TemporaryDirectory(prefix='hyjs-local501-build-') as temporary:
        temporary = Path(temporary)
        original = temporary / 'Assembly-CSharp.dll'
        original.write_bytes(assembly)
        proof_source = temporary / 'LocalResourceProof.cs'
        proof_source.write_text('namespace HYJS.LocalAssets { internal static class LocalResourceProof {'
                                f' internal const string InventorySHA256 = "{sha(inventory)}"; }} }}\n')
        run([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+',
             '/out:' + str(bridge), *['/reference:' + str(p) for p in refs],
             '/resource:' + str(output / 'local501.inventory') + ',local501.inventory',
             ROOT / 'tools/local_asset_ready/LocalAssetReady.cs', proof_source])
        shutil.copy2(ROOT / 'tools/local_asset_ready/Injector.cs', temporary / 'Program.cs')
        dnlib = work / 'injector/bin/Debug/net8.0/dnlib.dll'
        project = temporary / 'Local501Injector.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib">'
                           '<HintPath>' + str(dnlib) + '</HintPath></Reference>'
                           '<Compile Remove="LocalResourceProof.cs" /></ItemGroup></Project>')
        run([dotnet, 'build', project, '--nologo', '--verbosity', 'quiet'])
        result = output / 'Assembly-CSharp.dll'
        run([dotnet, temporary / 'bin/Debug/net8.0/Local501Injector.dll', original, bridge, result])
        protection = verify(assembly, result.read_bytes(), {
            ('RCUINormalNPC', 'OpenPage'), ('AwakeDiffItem', 'OnChallengeClick')})
        checked_gates = verify_gate_instructions(assembly, result.read_bytes())
    if protection['originalEngineMethods'] != 547:
        raise ValueError('Expected all 547 protected original Engine methods')
    manifest = dict(schema='local-awake501-resource-overlay-v1', **protection,
                    baseAPKSHA256=base_sha, baseProof=str(args.base_proof),
                    baseProofSHA256=sha(args.base_proof.read_bytes()),
                    inputManagedSHA256=sha(assembly), outputManagedSHA256=sha(result.read_bytes()),
                    bridgeSHA256=sha(bridge.read_bytes()), inventorySHA256=sha(inventory),
                    originalScopeProofSHA256=SCOPE_SHA,
                    originalGateInstructionsAndExceptionHandlersPreserved=checked_gates,
                    resourcesVerified=len(scope['resources']), missingScopedBundles=[],
                    globalFullFlagChanged=False, originalResourceLoaderChanged=False,
                    allowedCopy='501', allowedChapter='5', allowedRenderQuality=2,
                    allowedHeroIDs=['hero_001', 'hero_003'], allowedPetIDs=['pet_002'],
                    otherContentRetainsOriginalFullGate=True,
                    originalFullTrueRetainsOriginalBehaviour=True,
                    playerStateOrBattleResultChanged=False, apkBuilt=False, deviceVerified=False,
                    replacementEntries={MANAGED + 'Assembly-CSharp.dll': sha(result.read_bytes()),
                                        MANAGED + 'LocalAssetReady.dll': sha(bridge.read_bytes())},
                    sourcePins={str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [
                        ROOT / 'tools/local_asset_ready/LocalAssetReady.cs',
                        ROOT / 'tools/local_asset_ready/Injector.cs', Path(__file__).resolve()]})
    (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    print(json.dumps(manifest, indent=2))


if __name__ == '__main__':
    main()
