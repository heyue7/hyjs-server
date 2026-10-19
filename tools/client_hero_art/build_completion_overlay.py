"""Bounded completion over the current 97775 managed input; never package or install."""
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
from tools.local_asset_ready.remove_gate import exact_body, require

SOURCE = Path(__file__).resolve().parent
MAIN_SHA = '97775db52ca7f8dc445b03b8e2339f5cfc9ceb54690ca1d334181fd76403401e'
PINS = {
    'AssetLoad.AssetBundleManager::GetAssetFromBundle': 'd3661b7d46fd09d9881fc3d519285e623f5710bca98057fc65268b26c32dc278',
    'TableTextBlo::GetText': 'a3368ac001e6e49ce187b120a2762af9463e6e7e44e4352f0efa2ad538be08f5',
    'UI.UIHeroLogic::FillLucksData': 'c7e4ecf751e459beaf0a4b14cfe6e5a434f419f55dc35790bc224efd626ad867',
    'UI.EquipTab::UpdateTab': 'f6dfd33c7ab98a10fbb729137d51c874e7926605ad73f8ecb7309a37f3995045',
    '<UpdateList>c__IteratorB3::MoveNext': 'c9dd748311925e8362b1b37de1e2d4be5364aa1ca36a954621c3ac26bd7f92d4',
    'UI.ActivityAwardItem::InitActivityAward': '11cb1b46f40c82d2c7aa371f6abfacd016cd43b38a7bbd256a91c4e7b7a675a2',
    'HalfPrice::OnReceiveCallBack': 'b52ce3bbd082992c6d7ebe78a6247a6532eb2cffab53107e05da75e98fdf1e00',
    'UI.NPCUIShopItem::SetItem': '65a08b2aad3e4af048edf8c7f42534fdca0793bd35fd9ca5dea48de2d24f17de',
    'UI.NPCUIShopItem::OnBuyClick': '1fba1d9acd04b7e436f67dbd79b49895a509b4394d00e3ce4c79a10e017d1010',
    'Dict.Blo.TableItemShopPriceBlo::GetShopPrice': '296544daf706bd2c21e73f8de04cfff5282efbdffa748f816134f4f4597d4626',
    'UI.BuyItemPage::SetItem': '7bb3b49e39d03d8a9d08a6d0437361d19c309f8080531c42ffe7e698a7e06d06',
    'UI.BuyItemPage::IsHaveEnoughBuyCost': 'c29af0e76a82a13c9a84840c585f79984ac9e0046eedc55d4b05bf1f9ccec687',
    'UI.BuyShopItem::UpdatePriceIcon': '4350483e09cdfa778412e2ee7ea6e577cb4366202797b79f4631353a96498dc4',
    'UI.ShopBatchBuy::UpdateAll': 'a611dff9a1614a7e41686d8fbc1de3cdae00bdd395c8bc21327cc9463d803537',
    'UI.ShopProductItem::SetShopItem': '0b72a93fb009b70143eacd39f1d6788e2dd2f8bb5667e082329b100715b483e1',
    'CommonMethod::GetObjectHaveNum': '47a8ecd2822a9ebcbbd39dc7690c269fedace79b5b1d071217e3424aac5a597c',
    'UI.ShopProductItem::SetActivityItem': '230eb1d2909c62394a8787285e221ffa76670640bb0ebf12c1c3a331ab87f6c7',
    'UI.UIPlayerData::UpdateItemShopProducts': '31cdd46d4d5e3f2aeeaa548027281bf892a1c408655e47a3f0a60d515b866833',
}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def verify(before, after):
    a, b = [dnfile.dnPE(data=raw) for raw in (before, after)]
    left, right = methods(a), methods(b)
    require(len(left) == len(right), 'Method inventory changed')
    for table in ['TypeDef', 'Field', 'MethodDef', 'AssemblyRef']:
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows), 'Original inventory changed: ' + table)
    require(not getattr(b.net.mdtables, 'FieldPtr', None), 'Old Unity FieldPtr is forbidden')
    changed = []
    engine = unchanged = 0
    for (name, rid, old), (name2, rid2, new) in zip(left, right):
        require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) == (name2, rid2, new.Signature.value, new.struct.Flags, new.struct.ImplFlags), 'Original ABI/token changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'New original method body')
            continue
        x, y = exact_body(a, old), exact_body(b, new)
        if name in PINS:
            require(sha(x) == PINS[name] and x != y, 'Exact target pin/change differs: ' + name)
            changed.append({'method': name, 'beforeSHA256': sha(x), 'afterSHA256': sha(y)})
        else:
            require(x == y, 'Unrelated original raw method changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
    require(engine == 547 and len(changed) == len(PINS), 'Expected only pinned UI targets and 547 original engine methods')
    require(a.net.struct.Flags == b.net.struct.Flags, 'CLI flags changed')
    for old, new in zip(a.net.mdtables.AssemblyRef, b.net.mdtables.AssemblyRef):
        require((str(old.Name), old.struct.Flags, old.PublicKey.value) == (str(new.Name), new.struct.Flags, new.PublicKey.value), 'Assembly reference identity changed')
    return {'changes': changed, 'originalEngineMethodsUnchanged': engine, 'unrelatedOriginalRawBodiesExact': unchanged, 'originalMetadataABIAndTokensPreserved': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, default=ROOT / 'apks/build/hyjs-local.apk')
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    require(sha(args.base_apk.read_bytes()) == args.base_sha256, 'Base APK identity differs')
    out = args.output_directory.resolve()
    require(not out.exists(), 'Use a fresh output directory')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'

    def run(name, command):
        process = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, capture_output=True, text=True, timeout=90)
        (out / (name + '.log')).write_text(process.stdout + process.stderr)
        require(process.returncode == 0, name + ' failed; inspect its saved log')

    with zipfile.ZipFile(args.base_apk) as archive, tempfile.TemporaryDirectory(prefix='hyjs-three-hero-completion-') as temporary:
        work = Path(temporary)
        before = archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
        require(sha(before) == MAIN_SHA, 'Current 97775 managed input required')
        original = work / 'Assembly-CSharp.dll'
        original.write_bytes(before)
        references = [original]
        for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll']:
            target = work / name
            target.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
            references.append(target)
        helper = out / 'SoulFlowerHeroArt.dll'
        run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(helper), *['/reference:' + str(p) for p in references], SOURCE / 'HeroArt.cs', SOURCE / 'UFQuotedCost.cs', SOURCE / 'QuotedShopCost.cs'])
        shutil.copy2(SOURCE / 'CompletionInjector.cs', work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        run('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', original, out / 'Assembly-CSharp.dll'])
    result = {'schema': 'hyjs-three-original-character-completion-v2', 'status': 'STATIC_VERIFIED_NOT_PACKAGED', 'inputAPK_SHA256': args.base_sha256, 'inputManagedSHA256': MAIN_SHA, 'outputManagedSHA256': sha((out / 'Assembly-CSharp.dll').read_bytes()), 'bridgeSHA256': sha(helper.read_bytes()), 'replacementEntries': ['assets/bin/Data/Managed/Assembly-CSharp.dll', 'assets/bin/Data/Managed/SoulFlowerHeroArt.dll'], 'phoneVerified': False, 'sourcePins': {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [SOURCE / 'HeroArt.cs', SOURCE / 'UFQuotedCost.cs', SOURCE / 'QuotedShopCost.cs', SOURCE / 'CompletionInjector.cs', Path(__file__).resolve()]}, **verify(before, (out / 'Assembly-CSharp.dll').read_bytes())}
    (out / 'manifest.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    main()
