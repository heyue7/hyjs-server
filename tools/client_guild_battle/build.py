"""Build and byte-verify a scoped guild scenery managed overlay; never package or install."""
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
from tools.awake_diff_rows.build import methods, operand
from tools.local_asset_ready.remove_gate import exact_body, require

SOURCE = Path(__file__).resolve().parent
BASE_SHAS = {
    'd60dd8b6a9de9b11d8a673d240f19f3cc16d33956cdede386794cc26e2f71f8d': 69,
    '97a9ebcc82e1127a8c877ea2385da1a715a1b17d1f9a0eeed352debc53c70061': 70,
}
MAIN = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER = 'assets/bin/Data/Managed/SoulFlowerGuildBattle.dll'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def file_sha(path):
    with Path(path).open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def semantic(pe, row, shift=0):
    body = read_method_body_from_bytes(exact_body(pe, row))
    indices = {i.offset: n - shift for n, i in enumerate(body.instructions)}
    indices[body.instructions[-1].offset + body.instructions[-1].size] = len(body.instructions) - shift
    code = []
    for instruction in body.instructions:
        value = instruction.operand
        if instruction.opcode.name == 'switch':
            value = [indices[o] for o in value]
        elif instruction.opcode.flow_control.name in ('Branch', 'Cond_Branch'):
            value = indices[value]
        else:
            value = operand(value)
        code.append((instruction.opcode.name, value))
    header = body.header_size
    regions = [(r.exception_type, indices[r.try_start + header], indices[r.try_end + header],
                indices[r.handler_start + header], indices[r.handler_end + header], operand(r.catch_type),
                indices.get(r.filter_start + header) if r.is_filter() else None) for r in body.exception_handlers]
    return code, regions


def verify(before, after, hooks):
    a, b = [dnfile.dnPE(data=x) for x in (before, after)]
    left, right = methods(a), methods(b)
    allowed = {row['rid']: row for row in hooks}
    require(len(allowed) == 16, 'Expected sixteen exact overload-scoped hooks')
    for table in ['TypeDef', 'Field', 'MethodDef']:
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows), 'ABI inventory changed: ' + table)
    require(not getattr(b.net.mdtables, 'FieldPtr', None), 'Unity FieldPtr forbidden')
    require(len(left) == len(right), 'Method inventory changed')
    changed, engine = [], 0
    for (name, rid, old), (name2, rid2, new) in zip(left, right):
        require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) ==
                (name2, rid2, new.Signature.value, new.struct.Flags, new.struct.ImplFlags), 'ABI/token changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'New inherited body: ' + name)
            continue
        x, y = exact_body(a, old), exact_body(b, new)
        if rid in allowed:
            spec = allowed[rid]
            require(name == spec['method'], 'Hook identity mismatch')
            n = spec['prefixCount']
            old_code, old_regions = semantic(a, old)
            new_code, new_regions = semantic(b, new, n)
            require(old_code == new_code[n:] and old_regions == new_regions, 'Inherited CIL/EH changed: ' + name)
            observer = name in ['Room.RoomNetUnionFight::SCUFPlayerFightBegin', 'Room.RoomNetUnionFight::SCUFPlayerFightEnd']
            if observer:
                require(n == 3 and [op for op, _ in new_code[:n]] == ['ldarg.0', 'ldarg.1', 'call'], 'Observer must fall through to original consumer')
            else:
                require(new_code[n - 1][0] == 'ret', 'Hook must return or fall through')
                require(any(op == 'brfalse' and target == 0 for op, target in new_code[:n]), 'Missing original fallback: ' + name)
            changed.append(dict(spec, signatureHex=bytes(old.Signature.value).hex(), beforeSHA256=sha(x), afterSHA256=sha(y), inheritedCILAndExceptionRegionsExact=True))
        else:
            require(x == y, 'Unrelated body changed: ' + name)
        if name.startswith('Battle.Engine'):
            engine += 1
            require(x == y, 'Battle engine changed')
    require(engine == 547 and len(changed) == 16, 'Engine/hook denominator differs')
    for old, new in zip(a.net.mdtables.AssemblyRef.rows, b.net.mdtables.AssemblyRef.rows):
        require((str(old.Name), old.struct.Flags, old.PublicKey.value) == (str(new.Name), new.struct.Flags, new.PublicKey.value), 'Inherited assembly reference changed')
    native_types = {str(t.TypeNamespace) + '.' + str(t.TypeName) for t in a.net.mdtables.TypeDef.rows}
    for reference in b.net.mdtables.TypeRef.rows[len(a.net.mdtables.TypeRef.rows):]:
        name = str(reference.TypeNamespace) + '.' + str(reference.TypeName)
        scope = reference.ResolutionScope.row
        require(not (name in native_types and ('ModuleRef' in type(scope).__name__ or str(getattr(scope, 'Name', '')) == 'SoulFlowerGuildBattle')), 'Native type imported through helper: ' + name)
    return dict(allowedChangedMethods=sorted({row["method"] for row in changed}), allowedChangedMethodDetails=changed, originalEngineMethodsExact=engine, originalABITokensExact=True,
                allOtherOriginalBodiesByteExact=True, inheritedHelperAPIsPreserved=True, noNewOwnTypesViaHelper=True)



def verify_guard_helper(raw):
    pe = dnfile.dnPE(data=raw)
    names = {index: str(row.Name) for index, row in enumerate(pe.net.mdtables.MemberRef.rows, 1)}
    bodies = {}
    sender_sites = []
    for owner in pe.net.mdtables.TypeDef.rows:
        for member in owner.MethodList:
            row = member.row
            if not row.Rva:
                continue
            body = read_method_body_from_bytes(exact_body(pe, row))
            name = str(row.Name)
            if str(owner.TypeName) == 'GuildBattleScene':
                bodies[name] = body
            for instruction in body.instructions:
                token = getattr(instruction.operand, 'value', 0)
                if token >> 24 == 10 and names.get(token & 0xffffff) == 'SendCSUFPlayerAskFight':
                    sender_sites.append(name)
    require(sender_sites == ['GuardArrived'], 'Guard sender must have one controlled callsite')
    for name in ['ObservePlayerBegin', 'ObservePlayerEnd']:
        body = bodies[name]
        require(bool(body.exception_handlers), 'Observer must catch exceptions')
        require(not any(i.opcode.name in ['throw', 'rethrow'] for i in body.instructions), 'Observer can interrupt original consumer')
    require(not any(str(r.Name) == 'SendSocket' for r in pe.net.mdtables.MemberRef.rows), 'Guard helper bypasses original sender')
    return dict(guardSenderSingleOriginalCallsite=True, guardObserversExceptionContained=True, noDirectSocketSender=True)



def preflight_packager(base, directory):
    """Mirror all mandatory packager manifest reads without packaging, signing or locks."""
    from tools.client_release.package_native import verify_java_class_inventory
    proof = json.loads((directory / 'manifest.json').read_text())
    required = ['inheritedJavaPublicProtectedABIExact', 'baseAPKSHA256', 'sourcePins',
                'inputManagedSHA256', 'inputDexSHA256', 'allowedChangedMethods']
    require(all(key in proof for key in required), 'Required packager key missing')
    require(proof['managedOnly'] and proof['inheritedJavaByteExact'] and proof['inheritedJavaPublicProtectedABIExact'], 'Managed-only Java contract differs')
    require(file_sha(base) == proof['baseAPKSHA256'] and proof['baseAPKSHA256'] in BASE_SHAS, 'Pinned base differs')
    for name, digest in proof['sourcePins'].items():
        file = (ROOT / name).resolve()
        require(file.is_relative_to(ROOT) and file_sha(file) == digest, 'Source pin differs: ' + name)
    payload = {}
    for name, source in proof['payloadFiles'].items():
        file = (ROOT / source).resolve()
        require(name.startswith('assets/bin/Data/Managed/') and name.endswith('.dll'), 'Unapproved payload scope')
        require(file.is_relative_to(ROOT), 'Payload outside worktree')
        payload[name] = file.read_bytes()
    require(set(payload) == set(proof['replacementEntries']), 'Payload/replacement scope differs')
    require(all(sha(raw) == proof['replacementEntries'][name] for name, raw in payload.items()), 'Payload SHA differs')
    require(all(isinstance(name, str) for name in proof['allowedChangedMethods']), 'Packager method whitelist requires strings')
    with zipfile.ZipFile(base) as archive:
        require(sha(archive.read('classes2.dex')) == proof['inputDexSHA256'] == proof['outputDexSHA256'], 'Managed-only DEX pin differs')
        before = archive.read(MAIN)
        require(sha(before) == proof['inputManagedSHA256'], 'Managed input differs')
        java = verify_java_class_inventory(archive, payload)
        a, b = [dnfile.dnPE(data=raw) for raw in (before, payload[MAIN])]
        require(not getattr(b.net.mdtables, 'FieldPtr', None) and not getattr(b.net.mdtables, 'MethodPtr', None), 'Legacy pointer table')
        old, new = methods(a), methods(b)
        require(len(old) == len(new), 'Managed inventory differs')
        changed, engine = set(), 0
        for (name, rid, x), (other, rid2, y) in zip(old, new):
            require((name, rid, x.Signature.value, x.struct.Flags) == (other, rid2, y.Signature.value, y.struct.Flags), 'Packager ABI differs')
            if x.Rva:
                if exact_body(a, x) != exact_body(b, y):
                    require(name in set(proof['allowedChangedMethods']), 'Unapproved method')
                    changed.add(name)
                else:
                    engine += name.startswith('Battle.Engine')
        require(changed == set(proof['allowedChangedMethods']) and engine == 547, 'Packager managed scope differs')
        require(not any(row.struct.Flags & 1 and not row.PublicKey.value for row in b.net.mdtables.AssemblyRef.rows), 'Invalid assembly public key')
        detail = verify(before, payload[MAIN], json.loads((directory / 'hooks.json').read_text()))
        require(len(detail['allowedChangedMethodDetails']) == 16, 'Overload scope differs')
    sdk = Path.home() / 'Library/Android/sdk/build-tools/33.0.0'
    require(all((sdk / name).is_file() for name in ['zipalign', 'apksigner']), 'Packaging tools unavailable')
    require((ROOT / '.local-signing/debug.keystore').is_file(), 'Signing file unavailable')
    require(shutil.disk_usage(ROOT).free > base.stat().st_size * 2 + 150_000_000, 'Insufficient packaging space')
    return dict(status='READ_ONLY_PACKAGER_CONTRACT_PASS_NOT_PACKAGED', manifestSHA256=file_sha(directory / 'manifest.json'),
                payloadUnchangedSHA256={name: sha(raw) for name, raw in payload.items()}, inputDexSHA256=proof['inputDexSHA256'],
                outputDexSHA256=proof['outputDexSHA256'], managedOnly=True, requiredManifestKeysPresent=required,
                uniqueChangedMethodNames=len(changed), changedOverloads=16, originalEngineMethodsExact=engine,
                sourcePinsVerified=True, payloadPinsVerified=True, signingToolsAndDiskAvailable=True,
                javaClassInventory=java, optionalOverlaysNotRequested=True, packaged=False, signerExecuted=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--preflight', action='store_true', help='Read-only exact existing packager contract check')
    parser.add_argument('--preflight-report', type=Path)
    args = parser.parse_args()
    input_apk_hash = file_sha(args.base_apk)
    require(input_apk_hash in BASE_SHAS, 'Require one of the two explicitly pinned formal69/formal70 APKs')
    out = args.output.resolve()
    if args.preflight:
        result = preflight_packager(args.base_apk, out)
        if args.preflight_report:
            args.preflight_report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps(result, ensure_ascii=False))
        return
    require(not out.exists(), 'Use a fresh overlay directory')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'

    def run(name, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=120)
        (out / (name + '.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + ' failed; see ' + str(out / (name + '.log')))

    with tempfile.TemporaryDirectory(prefix='hyjs-guild-overlay-') as temp, zipfile.ZipFile(args.base_apk) as archive:
        work = Path(temp)
        before = archive.read(MAIN)
        inherited_entries = {name: sha(archive.read(name)) for name in archive.namelist() if name.startswith('assets/bin/Data/Managed/') and name != MAIN}
        inherited_dex_sha = sha(archive.read('classes2.dex'))
        original = work / 'Assembly-CSharp.dll'
        original.write_bytes(before)
        abi = dnfile.dnPE(data=before)
        iterator = next(t for t in abi.net.mdtables.TypeDef.rows if str(t.TypeName) == '<_WaitForLoadComplete>c__Iterator98')
        require({'$PC', '$current', '<>f__this'}.issubset({str(f.row.Name) for f in iterator.FieldList}), 'Legacy loader iterator ABI changed')
        require(any(str(m.row.Name) == '.ctor' and bytes(m.row.Signature.value) == bytes.fromhex('200001') for m in iterator.MethodList), 'Legacy loader constructor ABI changed')

        references = [original]
        for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll', 'LitJson.dll']:
            target = work / name
            target.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
            references.append(target)
        helper = out / 'SoulFlowerGuildBattle.dll'
        run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(helper), *['/reference:' + str(x) for x in references], SOURCE / 'GuildBattleScene.cs'])
        shutil.copyfile(SOURCE / 'Injector.cs', work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run('restore-injector', [dotnet, 'restore', project, '--source', '/Users/lirui/.nuget/packages', '--ignore-failed-sources'])
        run('compile-injector', [dotnet, 'build', project, '--no-restore', '--nologo', '--verbosity', 'quiet'])
        run('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', original, helper, out / 'Assembly-CSharp.dll', out / 'hooks.json'])
        after = (out / 'Assembly-CSharp.dll').read_bytes()
        validation = verify(before, after, json.loads((out / 'hooks.json').read_text()))
        helper_pe = dnfile.dnPE(data=helper.read_bytes())
        require(not any(str(r.Name) == 'OfflineBootstrap' for r in helper_pe.net.mdtables.AssemblyRef.rows), 'Re cloud/bootstrap dependency imported')
        pins = {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [SOURCE / 'GuildBattleScene.cs', SOURCE / 'Injector.cs', SOURCE / 'README.md', Path(__file__).resolve()]}
        reference = ROOT / 'analysis/guild-full-qa-20261025/re-reference/csharp/OfflineGuildBattleAssets310.cs'
        reference_pin = dict(path=str(reference), sha256=file_sha(reference))

        manifest = dict(schema='hyjs-guild-scenery-overlay-v1', status='STATIC_VERIFIED_NOT_PACKAGED', baseAPK=str(args.base_apk.resolve()),
                        baseSHA=input_apk_hash, baseAPKSHA256=input_apk_hash, baseVersion=BASE_SHAS[input_apk_hash], inputDexSHA256=inherited_dex_sha, outputDexSHA256=inherited_dex_sha, inputManagedSHA256=sha(before), outputManagedSHA256=sha(after),
                        helperSHA256=file_sha(helper), payloadFiles={MAIN: str(out / 'Assembly-CSharp.dll'), HELPER: str(helper)},
                        replacementEntries={MAIN: sha(after), HELPER: file_sha(helper)}, sourcePins=pins,
                        inheritedManagedEntrySHA256=inherited_entries, inheritedManagedPayloadEntriesUntouched=not bool(set(inherited_entries) & {MAIN, HELPER}), managedOnly=True, inheritedJavaByteExact=True, inheritedJavaPublicProtectedABIExact=True, referenceSourcePin=reference_pin, referenceSource='Re1.1.11 OfflineBootstrap Assets310; SHA b26c6b453e5a534bcdfa7da5d302f9266c9ade88e813129d6ecd321ebe59179b',
                        sceneContract='Authenticated /guild-war/map chooses re-flat-v1; empty projection uses exact original iterator; unknown/error fails closed', phoneVerified=False, **verify_guard_helper(helper.read_bytes()), **validation)
        require(file_sha(args.base_apk) == input_apk_hash, 'Input APK changed during build')
        (out / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps({'status': manifest['status'], 'hooks': len(validation['allowedChangedMethodDetails']), 'engineMethodsExact': validation['originalEngineMethodsExact'], 'manifest': str(out / 'manifest.json')}))


if __name__ == '__main__':
    main()
