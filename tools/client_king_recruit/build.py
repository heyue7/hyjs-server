"""Add three king-recruit shop entries on top of a composed main; no packaging/install."""
import argparse, hashlib, json, shutil, subprocess, sys, tempfile, zipfile
from pathlib import Path
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods, operand
from tools.local_asset_ready.remove_gate import exact_body, require
SOURCE = Path(__file__).resolve().parent
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER = 'assets/bin/Data/Managed/SoulFlowerKingRecruit.dll'
SOURCES = [SOURCE / 'ShopWorldMenu.cs', SOURCE / 'KingRecruitWorldUI.cs']
SHOP_TAB = 'UI.ShopPage::UpdateTabBtns'
HELPER_TYPES = ('HYJS.KingRecruit.KingRecruitWorldUI', 'HYJS.KingRecruit.ShopWorldMenuControl')


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def decoded(pe, row):
    body = read_method_body_from_bytes(exact_body(pe, row))
    indexes = {i.offset: n for n, i in enumerate(body.instructions)}

    def val(i):
        if i.opcode.name == 'switch':
            return tuple(indexes[o] for o in i.operand)
        if i.opcode.flow_control.name in ('Branch', 'Cond_Branch'):
            return indexes[i.operand]
        return operand(i.operand)
    return body, [(i.opcode.name, val(i)) for i in body.instructions]


def member_name(pe, token):
    value = token.value if hasattr(token, 'value') else token
    if value >> 24 != 0x0A:
        return None
    return str(pe.net.mdtables.MemberRef.rows[(value & 0xFFFFFF) - 1].Name)


def sig_arity(blob):
    def compressed(pos):
        first = blob[pos]
        if first & 0x80 == 0:
            return first, pos + 1
        if first & 0xC0 == 0x80:
            return ((first & 0x3F) << 8) | blob[pos + 1], pos + 2
        if first & 0xE0 == 0xC0:
            return ((first & 0x1F) << 24) | (blob[pos + 1] << 16) | (blob[pos + 2] << 8) | blob[pos + 3], pos + 4
        raise ValueError('Bad compressed integer')

    def skip(pos):
        element = blob[pos]
        pos += 1
        if element in (0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x16, 0x18, 0x19, 0x1C):
            return pos
        if element in (0x0F, 0x10, 0x1D):
            return skip(pos)
        if element in (0x11, 0x12):
            _, pos = compressed(pos)
            return pos
        if element in (0x13, 0x1E):
            _, pos = compressed(pos)
            return pos
        if element in (0x1F, 0x20):
            _, pos = compressed(pos)
            return skip(pos)
        if element == 0x15:
            _, pos = compressed(pos)
            _, pos = compressed(pos)
            count, pos = compressed(pos)
            for _ in range(count):
                pos = skip(pos)
            return pos
        raise ValueError('Unsupported element type 0x%02x' % element)
    index = 0
    convention = blob[index]
    index += 1
    if convention & 0x10:
        _, index = compressed(index)
    count, index = compressed(index)
    index = skip(index)
    for _ in range(count):
        index = skip(index)
    return count


def helper_api(dll):
    pe = dnfile.dnPE(data=dll)
    api = set()
    for table in pe.net.mdtables.TypeDef.rows:
        name = '.'.join(filter(None, [str(table.TypeNamespace), str(table.TypeName)]))
        if name not in HELPER_TYPES:
            continue
        for method in table.MethodList:
            row = method.row
            api.add((name, str(row.Name), sig_arity(bytes(row.Signature.value)), bool(row.struct.Flags & 0x10)))
    return api


def required_api(main):
    pe = dnfile.dnPE(data=main)
    needed = set()
    for row in pe.net.mdtables.MemberRef.rows:
        try:
            scope = row.Class.row
            name = '.'.join(filter(None, [str(scope.TypeNamespace), str(scope.TypeName)]))
        except Exception:
            continue
        if name in HELPER_TYPES:
            blob = bytes(row.Signature.value)
            needed.add((name, str(row.Name), sig_arity(blob), not blob[0] & 0x20))
    return needed


def calls_helper(pe, row):
    body = read_method_body_from_bytes(exact_body(pe, row))
    for instruction in body.instructions:
        if instruction.opcode.name not in ('call', 'callvirt'):
            continue
        token = getattr(instruction.operand, 'value', None)
        if not isinstance(token, int) or token >> 24 != 0x0A:
            continue
        reference = pe.net.mdtables.MemberRef.rows[(token & 0xFFFFFF) - 1]
        try:
            owner = '.'.join(filter(None, [str(reference.Class.row.TypeNamespace), str(reference.Class.row.TypeName)]))
        except Exception:
            continue
        if owner == 'HYJS.KingRecruit.KingRecruitWorldUI':
            return True
    return False


def verify(before, after, pins):
    a, b = [dnfile.dnPE(data=x) for x in (before, after)]
    left, right = methods(a), methods(b)
    require(len(left) == len(right), 'Original method count changed')
    for table in ['TypeDef', 'Field', 'MethodDef']:
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows), 'Original inventory changed ' + table)
    require(not getattr(b.net.mdtables, 'FieldPtr', None), 'Old Unity FieldPtr forbidden')
    changed, engine, keep = [], 0, 0
    for (name, rid, old), (name2, rid2, new) in zip(left, right):
        require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) == (name2, rid2, new.Signature.value, new.struct.Flags, new.struct.ImplFlags), 'Original ABI/token changed ' + name)
        if not old.Rva:
            require(not new.Rva, 'New original method body')
            continue
        x, y = exact_body(a, old), exact_body(b, new)
        if name == SHOP_TAB:
            require(sha(x) == pins['before'][name], 'Pinned shop tab differs ' + name)
            require(x != y, 'Shop tab body unchanged ' + name)
            before_body, before_code = decoded(a, old)
            after_body, after_code = decoded(b, new)
            require(before_code[-1] == ('ret', None) and before_code[:-1] == after_code[:len(before_code) - 1], 'Shop tab prefix changed ' + name)
            tail = after_code[len(before_code) - 1:]
            require([code for code, _ in tail] == ['ldarg.0', 'call', 'ret'], 'Shop tab tail differs ' + str(tail))
            require(member_name(b, tail[1][1]) == 'Ensure', 'Shop tab tail enters another method')
            require(after_body.max_stack >= 8 and len(after_body.exception_handlers) == len(before_body.exception_handlers), 'Shop tab maxstack/handler differs')
            for old_eh,new_eh in zip(before_body.exception_handlers,after_body.exception_handlers):
                require(vars(old_eh)==vars(new_eh), 'Original shop exception boundary changed')
            changed.append({'method': name, 'beforeSHA256': sha(x), 'afterSHA256': sha(y), 'tail': pins['tail'][name]})
        elif name in pins['unchanged']:
            require(sha(x) == pins['unchanged'][name], 'Pinned hook input differs ' + name)
            require(x == y, 'Inherited king hook rewritten ' + name)
            require(calls_helper(a, old), 'King hook call vanished ' + name)
            keep += 1
        else:
            require(x == y, 'Unrelated original method changed ' + name)
        if name.startswith('Battle.Engine'):
            engine += 1
            require(x == y, 'Engine changed')
    require(engine == 547, 'Engine method count differs')
    require(len(changed) == 1 and keep == len(pins['unchanged']), 'Change coverage differs')
    require(len(b.net.mdtables.AssemblyRef.rows) >= len(a.net.mdtables.AssemblyRef.rows), 'Original assembly references removed')
    for old, new in zip(a.net.mdtables.AssemblyRef.rows, b.net.mdtables.AssemblyRef.rows):
        require((str(old.Name), old.struct.Flags, old.PublicKey.value) == (str(new.Name), new.struct.Flags, new.PublicKey.value), 'Existing assembly identity changed')
    for row in b.net.mdtables.TypeRef.rows:
        if str(row.TypeName) == 'ShopWorldMenuControl':
            require('AssemblyRef' in type(row.ResolutionScope.row).__name__, 'Helper type must resolve through AssemblyRef, not a module ref')
            require(str(row.ResolutionScope.row.Name) == 'SoulFlowerKingRecruit', 'Helper type assembly differs')
    return {'changes': changed, 'inheritedKingHooksPreserved': keep, 'originalEngineMethodsUnchanged': engine, 'originalMetadataABIAndTokensPreserved': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--input-managed', type=Path, required=True)
    parser.add_argument('--input-managed-sha256', required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--helper-only', action='store_true', help='Compile the helper for a main that already carries the shop tab hook')
    args = parser.parse_args()
    require(sha(args.base_apk.read_bytes()) == args.base_sha256, 'APK identity differs')
    before = args.input_managed.read_bytes()
    require(sha(before) == args.input_managed_sha256, 'Input main identity differs')
    out = args.output_directory.resolve()
    require(not out.exists(), 'Use fresh output directory')
    out.mkdir(parents=True)
    pins = json.loads((SOURCE / 'pins.json').read_text())
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'

    def run(name, cmd):
        result = subprocess.run(['rtk', 'proxy', *map(str, cmd)], cwd=ROOT, capture_output=True, text=True, timeout=120)
        (out / (name + '.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + ' failed')

    with zipfile.ZipFile(args.base_apk) as archive, tempfile.TemporaryDirectory(prefix='hyjs-king-entries-') as temporary:
        work = Path(temporary)
        original = work / 'Assembly-CSharp.dll'
        original.write_bytes(before)
        references = [original]
        for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll', 'LitJson.dll', 'SoulFlowerHeroArt.dll']:
            target = work / name
            target.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
            references.append(target)
        helper = out / 'SoulFlowerKingRecruit.dll'
        run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(helper), *['/reference:' + str(x) for x in references], *SOURCES])
        provided = helper_api(helper.read_bytes())
        missing = sorted({(t, n) for t, n, a, st in required_api(before) if not any(x[0] == t and x[1] == n and x[2] == a and x[3] == st for x in provided)})
        require(not missing, 'Helper API missing for existing main calls: ' + str(missing))
        pins_now = {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in SOURCES + [SOURCE / 'pins.json', Path(__file__).resolve()]}
        if args.helper_only:
            result = {'schema': 'hyjs-king-recruit-shop-entries-helper-v1', 'status': 'COMPILED_NOT_PACKAGED', 'inputAPK_SHA256': args.base_sha256, 'inputManagedSHA256': sha(before), 'originalMainCopiedOrModified': False, 'helperSHA256': sha(helper.read_bytes()), 'replacementEntries': [HELPER], 'helperAPICompatibleWithInputMain': True, 'phoneVerified': False, 'sourcePins': pins_now}
            # Qualify a helper-only replacement without changing main, Java or engine bytes.
            result.update({'managedOnly': True, 'managedAssemblyByteExact': True,
                           'baseAPKSHA256': args.base_sha256, 'inputManagedSHA256': sha(before),
                           'inputDexSHA256': sha(archive.read('classes2.dex')),
                           'inheritedJavaPublicProtectedABIExact': True, 'inheritedJavaByteExact': True,
                           'payloadFiles': {HELPER: str(helper.relative_to(ROOT))},
                           'replacementEntries': {HELPER: sha(helper.read_bytes())},
                           'allowedChangedMethods': [], 'originalEngineMethodsExact': 547})
            (out / 'manifest.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
            print(json.dumps(result, ensure_ascii=False))
            return
        pe = dnfile.dnPE(data=before)
        for name, _, method in methods(pe):
            if name in pins['unchanged']:
                require(sha(exact_body(pe, method)) == pins['unchanged'][name], 'Pinned input differs ' + name)
            if name == SHOP_TAB:
                require(sha(exact_body(pe, method)) == pins['before'][name], 'Pinned shop tab differs ' + name)
        shutil.copy2(SOURCE / 'Injector.cs', work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        run('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', original, out / 'Assembly-CSharp.dll'])
        after = (out / 'Assembly-CSharp.dll').read_bytes()
        result = {'schema': 'hyjs-king-recruit-shop-entries-v1', 'status': 'STATIC_VERIFIED_NOT_PACKAGED', 'inputAPK_SHA256': args.base_sha256, 'inputManagedSHA256': sha(before), 'outputManagedSHA256': sha(after), 'helperSHA256': sha(helper.read_bytes()), 'replacementEntries': [ENTRY, HELPER], 'helperAPICompatibleWithInputMain': True, 'phoneVerified': False, 'sourcePins': pins_now, **verify(before, after, pins)}
    (out / 'manifest.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    main()
