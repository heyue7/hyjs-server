"""Add only the VIP7 combined-preview hook to the qualified existing VIP overlay."""
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
SOURCE = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require

TARGET = 'UI.ShopVipItem::OnIconClick'
TARGET_PIN = '6cf335aa3daddb4d73986f80dc8f01cb07b489b4904cd34aa12a5aac1a5190ad'
EMPTY_PIN = 'c45ad88168f7f6056683896a58dba149ddb9008d2ca9e6b4da24d6770741aedc'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def verify(before, after):
    a, b = [dnfile.dnPE(data=x) for x in (before, after)]
    left, right = methods(a), methods(b)
    require(len(left) == len(right), 'Original method count changed')
    for table in ['TypeDef', 'Field', 'MethodDef']:
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows), 'Inventory changed ' + table)
    require(not getattr(b.net.mdtables, 'FieldPtr', None), 'Old Unity FieldPtr forbidden')
    changed, engine = [], 0
    for (name, rid, old), (name2, rid2, new) in zip(left, right):
        require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) ==
                (name2, rid2, new.Signature.value, new.struct.Flags, new.struct.ImplFlags), 'ABI/token changed ' + name)
        if not old.Rva:
            require(not new.Rva, 'New original method body')
            continue
        x, y = exact_body(a, old), exact_body(b, new)
        if name == TARGET:
            require(sha(x) == TARGET_PIN and x != y, 'Preview pin/change differs')
            old_code = read_method_body_from_bytes(x).instructions
            new_code = read_method_body_from_bytes(y).instructions
            require([i.opcode.name for i in new_code[:5]] == ['ldarg.0', 'ldfld', 'call', 'brfalse.s', 'ret'], 'Preview prefix differs')
            require(new_code[3].operand == new_code[5].offset, 'Preview fallback skips original entry')
            canonical = lambda code: [(i.opcode.name, getattr(i.operand, 'value', i.operand)) for i in code]
            require(canonical(old_code) == canonical(new_code[5:]), 'Original preview fallback instructions changed')
            member = b.net.mdtables.MemberRef.rows[(new_code[2].operand.value & 0xffffff) - 1]
            require(str(member.Name) == 'TryOpen' and str(member.Class.row.TypeNamespace) == 'HYJS.VipShop' and
                    str(member.Class.row.TypeName) == 'VipGiftPreview', 'Preview helper target differs')
            changed.append({'method': name, 'beforeSHA256': sha(x), 'afterSHA256': sha(y)})
        else:
            require(x == y, 'Unrelated method changed ' + name)
        if name.startswith('Battle.Engine'):
            engine += 1
            require(x == y, 'Engine changed')
    require(engine == 547 and len(changed) == 1, 'Preview/engine coverage differs')
    for old, new in zip(a.net.mdtables.AssemblyRef, b.net.mdtables.AssemblyRef):
        require((str(old.Name), old.struct.Flags, old.PublicKey.value) ==
                (str(new.Name), new.struct.Flags, new.PublicKey.value), 'Assembly identity changed')
    require(len(a.net.mdtables.AssemblyRef.rows) == len(b.net.mdtables.AssemblyRef.rows), 'Assembly reference count changed')
    return {'changes': changed, 'originalEngineMethodsUnchanged': engine, 'originalMetadataABIAndTokensPreserved': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--runtime-root', type=Path, default=ROOT / '.tools/native-batch')
    args = parser.parse_args()
    require(sha(args.base_apk.read_bytes()) == args.base_sha256, 'APK identity differs')
    with zipfile.ZipFile(args.base_apk) as archive:
        before = archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
        pe = dnfile.dnPE(data=before)
        found = {name: sha(exact_body(pe, method)) for name, _, method in methods(pe)
                 if name in [TARGET, 'UI.TabVipShop::OnMsgCallback']}
        require(found.get(TARGET) == TARGET_PIN, 'Pinned preview input differs; refusing repeated injection')
        require(found.get('UI.TabVipShop::OnMsgCallback') == EMPTY_PIN, 'Qualified existing empty hook differs')
        out = args.output_directory.resolve()
        require(not out.exists(), 'Use fresh output directory')
        out.mkdir(parents=True)
        runtime = args.runtime_root.resolve()
        dotnet = runtime / 'dotnet/dotnet'
        compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
        dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'

        def run(name, cmd):
            result = subprocess.run(['rtk', 'proxy', *map(str, cmd)], cwd=ROOT, capture_output=True, text=True, timeout=120)
            (out / (name + '.log')).write_text(result.stdout + result.stderr)
            require(result.returncode == 0, name + ' failed; see ' + str(out / (name + '.log')))

        with tempfile.TemporaryDirectory(prefix='hyjs-vip-preview-') as temporary:
            work = Path(temporary)
            original = work / 'Assembly-CSharp.dll'
            original.write_bytes(before)
            references = [original]
            for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll']:
                target = work / name
                target.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
                references.append(target)
            helper = out / 'SoulFlowerVipEmpty.dll'
            run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
                                  '/deterministic+', '/out:' + str(helper),
                                  *['/reference:' + str(x) for x in references], SOURCE / 'VipShopEmpty.cs', SOURCE / 'VipGiftPreview.cs'])
            shutil.copy2(SOURCE / 'PreviewInjector.cs', work / 'Program.cs')
            project = work / 'Patch.csproj'
            project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
            run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
            run('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', original, out / 'Assembly-CSharp.dll'])
    result = {'schema': 'hyjs-vip7-combined-preview-v1', 'status': 'STATIC_VERIFIED_NOT_PACKAGED',
              'inputAPK_SHA256': args.base_sha256, 'inputManagedSHA256': sha(before),
              'outputManagedSHA256': sha((out / 'Assembly-CSharp.dll').read_bytes()),
              'helperSHA256': sha(helper.read_bytes()), 'phoneVerified': False,
              'replacementEntries': ['assets/bin/Data/Managed/Assembly-CSharp.dll', 'assets/bin/Data/Managed/SoulFlowerVipEmpty.dll'],
              'sourcePins': {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in
                             [SOURCE / 'VipGiftPreview.cs', SOURCE / 'VipShopEmpty.cs', SOURCE / 'PreviewInjector.cs', Path(__file__).resolve()]},
              **verify(before, (out / 'Assembly-CSharp.dll').read_bytes())}
    (out / 'manifest.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    main()
