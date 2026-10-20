"""Compile the corrected hero-art/text helper and verify the three in-game call chains."""
import argparse
import hashlib
import json
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body, require
from tools.client_hero_art.gen_hero_copy import main as generate_copy

SOURCE = Path(__file__).resolve().parent
HELPER = 'assets/bin/Data/Managed/SoulFlowerHeroArt.dll'
SOURCES = [SOURCE / 'HeroArt.cs', SOURCE / 'UFQuotedCost.cs', SOURCE / 'QuotedShopCost.cs', SOURCE / 'HeroCopyTable.g.cs']
HELPER_TYPES = ('HYJS.HeroArt.HeroArtFallback', 'HYJS.HeroArt.OriginalHeroCrop', 'HYJS.Shop.QuotedShopCost', 'HYJS.Shop.UFQuotedCost', 'HYJS.Shop.QuotedCostIcon')


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def strings(pe, row):
    values = []
    for instruction in read_method_body_from_bytes(exact_body(pe, row)).instructions:
        if instruction.opcode.name == 'ldstr':
            values.append(str(pe.net.user_strings.get(instruction.operand.value & 0xFFFFFF).value))
    return values


def calls(pe, row, owners=None):
    result = []
    for instruction in read_method_body_from_bytes(exact_body(pe, row)).instructions:
        if instruction.opcode.name not in ('call', 'callvirt'):
            continue
        token = getattr(instruction.operand, 'value', None)
        if not isinstance(token, int):
            continue
        if token >> 24 == 0x0A:
            reference = pe.net.mdtables.MemberRef.rows[(token & 0xFFFFFF) - 1]
            try:
                owner = '.'.join(filter(None, [str(reference.Class.row.TypeNamespace), str(reference.Class.row.TypeName)]))
            except Exception:
                owner = ''
            result.append((owner, str(reference.Name)))
        elif token >> 24 == 0x06:
            method = pe.net.mdtables.MethodDef.rows[(token & 0xFFFFFF) - 1]
            result.append(((owners or {}).get((token & 0xFFFFFF) - 1, ''), str(method.Name)))
    return result


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


def verify_chains(main):
    pe = dnfile.dnPE(data=main)
    by_name = {name: row for name, _, row in methods(pe)}
    owners = {}
    for table in pe.net.mdtables.TypeDef.rows:
        owner = '.'.join(filter(None, [str(table.TypeNamespace), str(table.TypeName)]))
        for method in table.MethodList:
            owners[method.row_index] = owner
    checks = []

    def need(name, string=None, call=None):
        require(name in by_name, 'Missing original method ' + name)
        row = by_name[name]
        if string is not None:
            require(string in strings(pe, row), 'Missing request prefix %s in %s' % (string, name))
        if call is not None:
            require(call in calls(pe, row, owners), 'Missing call %s in %s' % (call, name))
        checks.append({'method': name, 'string': string, 'call': ('%s::%s' % call) if call else None})

    need('UI.UIHeroLogic::FillTopData', 'hero2d/', ('AssetLoadHelper', 'SetTextureAsync'))
    need('UI.AllHerosPage::SetHeroList', None, ('HYJS.HeroArt.HeroArtFallback', 'AtlasHidden'))
    need('UI.HeroInfoPage::FillTopData', 'heroPainter/', None)
    for name in ['UI.ActivityRecruitPreview::OnHeroClick', 'UI.NormalRecruitPreview::OnHeroClick', 'UI.RecruitResultCardItem::OnClickHero']:
        need(name, 'UIHeroPage', ('UI.PageManager', 'OpenPage'))
    need('UI.RecruitResultCardItem::SetHeroBuyId', None, ('HYJS.KingRecruit.KingRecruitWorldUI', 'OriginalCardPath'))
    need('<SetPath>c__AnonStorey186::<>m__D8', None, ('HYJS.HeroArt.HeroArtFallback', 'AssignCard'))
    need('AloneTextureKit::SetPath', None, ('HYJS.HeroArt.HeroArtFallback', 'PrepareCard'))
    need('<_LoadTexture>c__IteratorCD::MoveNext', None, ('HYJS.HeroArt.HeroArtFallback', 'AssignHead'))
    need('Battle.BattleHeroItem::Init', None, ('HYJS.HeroArt.HeroArtFallback', 'AssignHead'))
    need('TableTextBlo::GetText', None, ('HYJS.HeroArt.HeroArtFallback', 'MissingOriginalText'))
    need('<_SetTextureAsync>c__Iterator2A::MoveNext', None, ('HYJS.HeroArt.HeroArtFallback', 'AssignRequested'))
    need('<_SetTextureAsync>c__Iterator2A::MoveNext', None, ('HYJS.HeroArt.HeroArtFallback', 'MakeRequestedPixelPerfect'))
    return checks


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--input-managed', type=Path, required=True)
    parser.add_argument('--input-managed-sha256', required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    require(sha(args.base_apk.read_bytes()) == args.base_sha256, 'APK identity differs')
    before = args.input_managed.read_bytes()
    require(sha(before) == args.input_managed_sha256, 'Input main identity differs')
    generate_copy()
    require(SOURCE.joinpath('HeroCopyTable.g.cs').exists(), 'Copy table generation failed')
    out = args.output_directory.resolve()
    require(not out.exists(), 'Use fresh output directory')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))

    def run(name, cmd):
        result = subprocess.run(['rtk', 'proxy', *map(str, cmd)], cwd=ROOT, capture_output=True, text=True, timeout=120)
        (out / (name + '.log')).write_text(result.stdout + result.stderr)
        require(result.returncode == 0, name + ' failed; inspect saved log')

    with zipfile.ZipFile(args.base_apk) as archive, tempfile.TemporaryDirectory(prefix='hyjs-hero-copy-') as temporary:
        work = Path(temporary)
        host = work / 'Assembly-CSharp.dll'
        host.write_bytes(before)
        references = [host]
        for name in ['UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll']:
            target = work / name
            target.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
            references.append(target)
        helper = out / 'SoulFlowerHeroArt.dll'
        run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(helper), *['/reference:' + str(p) for p in references], *SOURCES])
        previous = archive.read(HELPER)
        previous_types = {'.'.join(filter(None, [str(t.TypeNamespace), str(t.TypeName)])) for t in dnfile.dnPE(data=previous).net.mdtables.TypeDef.rows}
        current_types = {'.'.join(filter(None, [str(t.TypeNamespace), str(t.TypeName)])) for t in dnfile.dnPE(data=helper.read_bytes()).net.mdtables.TypeDef.rows}
        require(previous_types <= current_types, 'Helper type removed: ' + str(sorted(previous_types - current_types)))
        provided = helper_api(helper.read_bytes())
        needed = required_api(before)
        missing = sorted({(t, n) for t, n, a, s in needed if not any(x[0] == t and x[1] == n and x[2] == a and x[3] == s for x in provided)})
        require(not missing, 'Helper API missing for existing main calls: ' + str(missing))
        chains = verify_chains(before)
    result = {'schema': 'hyjs-three-hero-copy-and-single-crop-v1', 'status': 'COMPILED_NOT_PACKAGED', 'inputAPK_SHA256': args.base_sha256, 'inputManagedSHA256': sha(before), 'originalMainCopiedOrModified': False, 'helperSHA256': sha(helper.read_bytes()), 'replacementEntries': [HELPER], 'helperAPICompatibleWithInputMain': True, 'existingMainCallsCovered': len(needed), 'threeCallChains': chains, 'singleCropBoundary': {'assetBoundary': 'alias only, no derived texture', 'widgetSetters': ['AssignHead', 'AssignCard', 'AssignRequested']}, 'phoneVerified': False, 'sourcePins': {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in SOURCES + [SOURCE / 'hero_copy.json', SOURCE / 'gen_hero_copy.py', Path(__file__).resolve()]}}
    (out / 'manifest.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({k: result[k] for k in ['status', 'helperSHA256', 'existingMainCallsCovered', 'threeCallChains']}, ensure_ascii=False))


if __name__ == '__main__':
    main()
