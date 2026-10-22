"""Compile the GGO NGUI helper and compose three scoped CIL hooks; never package an APK."""
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
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods, operand
from tools.local_asset_ready.remove_gate import exact_body, require

SOURCE = Path(__file__).resolve().parent / 'client'
MAIN = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER = 'assets/bin/Data/Managed/SoulFlowerGGOShop.dll'
ALLOWED = {'UI.ShopPage::UpdateTabBtns': ('tail', 2), 'UI.ShopPage::OnTabBtn': ('prefix', 5), 'UI.UIPlayerData::AddSimpleItem': ('prefix', 3)}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def semantic(pe, row, shift=0):
    body = read_method_body_from_bytes(exact_body(pe, row))
    indexes = {i.offset: n - shift for n, i in enumerate(body.instructions)}
    indexes[body.instructions[-1].offset + body.instructions[-1].size] = len(body.instructions) - shift
    code = []
    for instruction in body.instructions:
        value = instruction.operand
        if instruction.opcode.name == 'switch':
            value = [indexes[o] for o in value]
        elif instruction.opcode.flow_control.name in ('Branch', 'Cond_Branch'):
            value = indexes[value]
        else:
            value = operand(value)
        code.append((instruction.opcode.name, value))
    # dncil EH offsets are relative to the code, instruction offsets include the method header.
    header = body.header_size
    regions = []
    for region in body.exception_handlers:
        regions.append((region.exception_type, indexes[region.try_start + header], indexes[region.try_end + header],
                        indexes[region.handler_start + header], indexes[region.handler_end + header],
                        operand(region.catch_type), indexes.get(region.filter_start + header) if region.is_filter() else None))
    return code, regions


def verify(before, after):
    a, b = [dnfile.dnPE(data=x) for x in (before, after)]
    left, right = methods(a), methods(b)
    require(len(left) == len(right), 'Original method count changed')
    for table in ['TypeDef', 'Field', 'MethodDef']:
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows), 'ABI inventory changed ' + table)
    require(not getattr(b.net.mdtables, 'FieldPtr', None), 'Old Unity FieldPtr forbidden')
    original_types = {str(t.TypeNamespace) + '.' + str(t.TypeName) for t in a.net.mdtables.TypeDef.rows}
    before_type_refs = len(a.net.mdtables.TypeRef.rows)
    for reference in b.net.mdtables.TypeRef.rows[before_type_refs:]:
        name = str(reference.TypeNamespace) + '.' + str(reference.TypeName)
        scope = reference.ResolutionScope.row
        require(not (name in original_types and ('ModuleRef' in type(scope).__name__ or str(getattr(scope, 'Name', '')) == 'SoulFlowerGGOShop')),
                'Own type incorrectly imported through helper/module: ' + name)
    native = next(row for name, _, row in left if name == 'UI.UIPlayerData::AddSimpleItem')
    native_signature = bytes(native.Signature.value)
    require(native_signature[:3] == bytes([0x20, 1, 1]), 'Native inventory signature changed')
    native_list = native_signature[3:]
    imported = [r for r in b.net.mdtables.MemberRef.rows if str(r.Name) == 'InventoryItems']
    require(len(imported) == 1, 'InventoryItems import count differs')
    require(bytes(imported[0].Signature.value) == bytes([0, 1]) + native_list + native_list,
            'InventoryItems must reuse native List<SimpleItem> TypeDef signature')
    helper_scope = imported[0].Class.row.ResolutionScope.row
    require('AssemblyRef' in type(helper_scope).__name__ and str(helper_scope.Name) == 'SoulFlowerGGOShop',
            'InventoryItems owner must resolve through helper AssemblyRef')
    changed, engine = [], 0
    for (name, rid, old), (name2, rid2, new) in zip(left, right):
        require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) == (name2, rid2, new.Signature.value, new.struct.Flags, new.struct.ImplFlags), 'Original ABI/token changed ' + name)
        if not old.Rva:
            require(not new.Rva, 'New original body ' + name)
            continue
        x, y = exact_body(a, old), exact_body(b, new)
        if name in ALLOWED:
            kind, count = ALLOWED[name]
            original, eh = semantic(a, old)
            updated, new_eh = semantic(b, new, count if kind == 'prefix' else 0)
            require(eh == new_eh, 'Original exception region changed ' + name)
            if kind == 'prefix':
                require(original == updated[count:], 'Inherited CIL changed ' + name)
                expected = ['ldarg.0', 'ldarg.1', 'call', 'brfalse', 'ret'] if count == 5 else ['ldarg.1', 'call', 'starg']
                require([op for op, _ in updated[:count]] == expected, 'Unexpected hook prefix ' + name)
            else:
                require(original[:-1] == updated[:-3] and [op for op, _ in updated[-3:]] == ['ldarg.0', 'call', 'ret'], 'Inherited update CIL changed')
            changed.append({'method': name, 'beforeSHA256': sha(x), 'afterSHA256': sha(y), 'exceptionRegionsPreserved': True, 'inheritedCILPreserved': True})
        else:
            require(x == y, 'Unrelated original body changed ' + name)
        if name.startswith('Battle.Engine'):
            engine += 1
            require(x == y, 'Original battle engine changed')
    require(engine == 547, 'Original battle method count differs')
    require(len(changed) == 3, 'Expected exactly three hooks')
    for old, new in zip(a.net.mdtables.AssemblyRef.rows, b.net.mdtables.AssemblyRef.rows):
        require((str(old.Name), old.struct.Flags, old.PublicKey.value) == (str(new.Name), new.struct.Flags, new.PublicKey.value), 'Inherited assembly reference changed')
    return {'allowedChangedMethods': changed, 'originalEngineMethodsExact': engine, 'originalABITokensExact': True, 'allOtherOriginalBodiesByteExact': True, 'noNewOwnTypesViaModuleOrHelper': True, 'inventoryItemsNativeTypeDefSignatureExact': True, 'inventoryItemsSignatureHex': bytes(imported[0].Signature.value).hex()}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True, help='Fresh managed overlay directory')
    args = parser.parse_args()
    input_apk_hash = sha(args.base_apk.read_bytes())
    out = args.output.resolve()
    require(not out.exists(), 'Use a fresh output directory')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'

    def run(name, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=120)
        (out / (name + '.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + ' failed; see ' + str(out / (name + '.log')))

    with zipfile.ZipFile(args.base_apk) as archive, tempfile.TemporaryDirectory(prefix='hyjs-ggo-client-') as temp:
        work = Path(temp)
        before = archive.read(MAIN)
        enum_pe = dnfile.dnPE(data=before)
        shop_enum = next(t for t in enum_pe.net.mdtables.TypeDef.rows if str(t.TypeNamespace) == 'UI' and str(t.TypeName) == 'ShopType')
        shop_fields = [f.row for f in shop_enum.FieldList]
        for constant in enum_pe.net.mdtables.Constant.rows:
            if constant.Parent.row in shop_fields and str(constant.Parent.row.Name) != 'Count':
                require(int.from_bytes(constant.Value.value, 'little', signed=True) != 11, 'Shop type 11 is occupied')
        original = work / 'Assembly-CSharp.dll'
        original.write_bytes(before)
        references = [original]
        for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll', 'LitJson.dll', 'SoulFlowerHeroArt.dll']:
            target = work / name
            target.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
            references.append(target)
        helper = out / 'SoulFlowerGGOShop.dll'
        run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(helper), *['/reference:' + str(x) for x in references], SOURCE / 'GGOShop.cs'])
        shutil.copy2(SOURCE / 'Injector.cs', work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        run('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', original, helper, out / 'Assembly-CSharp.dll'])
        king_out = out / 'king-helper'
        run('compile-king-helper', [sys.executable, ROOT / 'tools/client_king_recruit/build.py',
            '--base-apk', args.base_apk.resolve(), '--base-sha256', input_apk_hash,
            '--input-managed', original, '--input-managed-sha256', sha(before),
            '--output-directory', king_out, '--helper-only'])
        king_manifest = json.loads((king_out / 'manifest.json').read_text())
        king_helper = king_out / 'SoulFlowerKingRecruit.dll'
        require(king_manifest['helperAPICompatibleWithInputMain'], 'King helper inherited API differs')
        after = (out / 'Assembly-CSharp.dll').read_bytes()
        manifest = {'schema': 'hyjs-ggo-shop-overlay-v1', 'status': 'STATIC_VERIFIED_NOT_PACKAGED',
                    'baseAPK': str(args.base_apk.resolve()), 'baseAPKSHA256': input_apk_hash,
                    'inputManagedSHA256': sha(before), 'outputManagedSHA256': sha(after), 'helperSHA256': sha(helper.read_bytes()),
                    'replacementEntries': {MAIN: sha(after), HELPER: sha(helper.read_bytes()), 'assets/bin/Data/Managed/SoulFlowerKingRecruit.dll': sha(king_helper.read_bytes())},
                    'payloadFiles': {MAIN: str(out / 'Assembly-CSharp.dll'), HELPER: str(helper), 'assets/bin/Data/Managed/SoulFlowerKingRecruit.dll': str(king_helper)},
                    'inheritedKingAPICompatible': True, 'kingHelperSourcePins': king_manifest['sourcePins'],
                    'phoneVerified': False, 'sourcePins': {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [SOURCE / 'GGOShop.cs', SOURCE / 'Injector.cs', Path(__file__).resolve()]}, **verify(before, after)}
        manifest['sourcePins'].update(king_manifest['sourcePins'])
        require(sha(args.base_apk.read_bytes()) == input_apk_hash, 'Input APK changed during compilation')
        (out / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps(manifest, ensure_ascii=False))


if __name__ == '__main__':
    main()
