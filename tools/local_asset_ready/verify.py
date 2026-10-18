"""Verify the owned original Android resource inventory, without build/device use."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import zipfile

ROOT = Path(__file__).resolve().parents[2]
ORIGINAL_APK_SHA = 'b1d99496c7ffe17c45ab7646c72023be406924997d471e3f18db169fb0dfd641'
MANIFEST = 'assets/android/smallassetmanifest'
EXPECTED_COUNT = 7818
MANIFEST_SHA = 'c2c63f6ceeb212585b8ad26368f9f68d3918b8f97abda937f4034f7f5fe17c5e'


def parse_manifest(raw):
    lines = raw.decode('utf-8', errors='strict').splitlines()
    if not lines or lines[0] != '3000':
        raise ValueError('Original resource manifest version differs')
    rows, seen = [], set()
    for line in lines[1:]:
        fields = line.split('\t')
        if len(fields) != 3:
            raise ValueError('Resource manifest row shape differs')
        name, digest, size = fields
        if (not re.fullmatch(r'[a-z0-9_./@()&-]+', name) or name.startswith('/')
                or any(p in ('', '.', '..') for p in name.split('/')) or name in seen
                or not re.fullmatch(r'[0-9a-f]{32}', digest) or not size.isdigit() or int(size) <= 0):
            raise ValueError('Unsafe/duplicate/malformed resource manifest row')
        seen.add(name)
        rows.append(dict(path=name, md5=digest, size=int(size)))
    return rows


def verify_apk(path, *, expected_manifest_sha=None, expected_count=EXPECTED_COUNT):
    with zipfile.ZipFile(path) as apk:
        entries = apk.infolist()
        names = [entry.filename for entry in entries]
        if len(names) != len(set(names)):
            raise ValueError('Duplicate ZIP entry could hide an unverified resource')
        raw = apk.read(MANIFEST)
        manifest_sha = hashlib.sha256(raw).hexdigest()
        if expected_manifest_sha and manifest_sha != expected_manifest_sha:
            raise ValueError('Pinned original manifest changed')
        rows = parse_manifest(raw)
        if len(rows) != expected_count:
            raise ValueError('Original complete inventory count differs')
        actual = {n for n in names if n.startswith('assets/android/') and not n.endswith('/')}
        expected = {MANIFEST} | {'assets/android/' + row['path'] for row in rows}
        if actual != expected:
            raise ValueError('APK resources have missing or undeclared entries')
        verified = []
        for row in rows:
            name = 'assets/android/' + row['path']
            entry = apk.getinfo(name)
            if entry.flag_bits & 1 or entry.compress_type not in (0, 8) or entry.file_size != row['size']:
                raise ValueError('Resource length/compression differs: ' + row['path'])
            md5, sha, count = hashlib.md5(), hashlib.sha256(), 0
            with apk.open(entry) as source:
                while block := source.read(1024 * 1024):
                    count += len(block)
                    md5.update(block)
                    sha.update(block)
            if count != row['size'] or md5.hexdigest() != row['md5']:
                raise ValueError('Original resource bytes differ: ' + row['path'])
            verified.append(dict(row, sha256=sha.hexdigest()))
    inventory = json.dumps(verified, sort_keys=True, separators=(',', ':')).encode()
    return dict(schema='local-owned-original-asset-inventory-v1', manifest=MANIFEST,
                manifestSHA256=manifest_sha, manifestVersion=3000, resourceCount=len(rows),
                resourceBytes=sum(row['size'] for row in rows), inventorySHA256=hashlib.sha256(inventory).hexdigest(),
                resources=verified, allDeclaredOriginalResourcesVerified=True,
                runtimeOrDeviceExecuted=False, originalServerUpdateServiceRestored=False)


def awake_scope(path, inventory):
    """Original bundle graph plus original normal-party dynamic resource rows.

    The small manifest is a package inventory, not the full bundle graph.
    Other chapters and low-quality replacement families remain outside scope.
    """
    import UnityPy
    def table(name):
        lines = (ROOT / ('analysis/tables/Table' + name + '.tsv')).read_text().splitlines()
        return [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line.strip()]
    with zipfile.ZipFile(path) as apk:
        tree = next(obj.read_typetree() for obj in UnityPy.load(apk.read('assets/android/android')).objects
                    if obj.type.name == 'AssetBundleManifest')
        names, infos = dict(tree['AssetBundleNames']), dict(tree['AssetBundleInfos'])
        by_name = {value: key for key, value in names.items()}
        available = {row['path']: row for row in inventory['resources']}
        roots = {'map/map_forestfield_001', 'map/map_forest_003', 'model/box',
                 'particles/01_general/monster_die', 'particles/ui_dxp/changjingzhandou_guang',
                 'particles/03_buff/buff_fuhuowudi', 'minimap/map_forestfield_001'}
        families = ('kirito01', 'kirito_body', 'asuna02', 'monster_033', 'pet_020', 'awakecopy')
        roots.update(name for name in names.values() if any(f in name for f in families)
                     and 'low_' not in name and not name.endswith('_low'))
        skills = {'001_1', '001_2', '001_3', '003_1', '003_2', '003_3', '1033_1'}
        particle_ids = set()
        def particle(value):
            if value:
                particle_ids.add(value)
        def audio(value):
            for part in value.split('#'):
                if part:
                    roots.add('audio/' + part.split('=')[0].lower())
        for row in table('Skill'):
            if row['SkillId'] in skills:
                for i in range(5):
                    particle(row.get('Effect' + str(i), ''))
                audio(row.get('SkillAudio', ''))
        for row in table('SkillCast'):
            if row['SkillId'] in skills:
                for key in ['BehitEffectEnemy', 'BehitEffSelf', 'GroundEffect']:
                    particle(row.get(key, ''))
                audio(row.get('BehitAudio', ''))
        for row in table('UltimateSkill'):
            if row['SkillId'] in skills:
                roots.add('ultimate/' + row['EffectResourceName'].lower())
                if row.get('PaintResUrl'):
                    roots.add(row['PaintResUrl'].lower())
                for key in ['FinalBeHitEffect', 'FinalBeHitOwnEffect']:
                    particle(row.get(key, ''))
                audio(row.get('UltiAudio', '')); audio(row.get('UltiHitAudio', ''))
        particles = {row['ParticleId']: row['ResourcePath'] for row in table('Particle')}
        for ident in particle_ids:
            if ident not in particles:
                raise ValueError('Original particle reference unknown: ' + ident)
            roots.add('particles/' + particles[ident].lower())
        # Ordinary default weapon paths, plus the existing normal red-equipment
        # visual rows for the scoped pair. No special weapon mechanics admitted.
        for row in table('WeaponRes'):
            if row['heroId'] in ('hero_001', 'hero_003'):
                for key in ['modelRes0', 'modelRes1']:
                    if row.get(key):
                        roots.add('weaponprefab/' + row[key].lower())
        for row in table('MonsterWeaponRes'):
            if row.get('heroId', row.get('id')) == 'monster_033':
                for key, value in row.items():
                    if key not in ('heroId', 'id') and value:
                        roots.add('weaponprefab/' + value.lower())
        undeclared = sorted(roots - by_name.keys())
        if undeclared:
            raise ValueError('Scoped original root absent from bundle graph: ' + ','.join(undeclared))
        pending, seen = [by_name[name] for name in roots], set()
        while pending:
            index = pending.pop()
            if index in seen:
                continue
            seen.add(index)
            pending.extend(infos[index]['AssetBundleDependencies'])
        closure = {names[index] for index in seen}
        missing = sorted(closure - available.keys())
        if missing:
            raise ValueError('Scoped original dependencies unavailable: ' + ','.join(missing))
        selected = [dict(available[name], entry='assets/android/' + name)
                    for name in sorted(closure | {'android'})]
        selected.append(dict(path='smallassetmanifest', entry=MANIFEST, size=len(apk.read(MANIFEST)),
                             md5=hashlib.md5(apk.read(MANIFEST)).hexdigest(), sha256=MANIFEST_SHA))
        # Native Resources.Load UI prefabs/atlases also come from unchanged
        # built-in Unity data; they must survive APK composition byte for byte.
        builtins = [entry for entry in apk.infolist() if entry.filename.startswith('assets/bin/Data/')
                    and not entry.filename.startswith('assets/bin/Data/Managed/') and not entry.is_dir()]
        for entry in builtins:
            raw = apk.read(entry)
            selected.append(dict(path=entry.filename, entry=entry.filename, size=len(raw),
                                 md5=hashlib.md5(raw).hexdigest(), sha256=hashlib.sha256(raw).hexdigest()))
    return dict(schema='local-awake501-original-resource-closure-v1', globalFullAssetReady=False,
                allowedCopyID='501', allowedChapterID='5', allowedRenderQuality=2,
                manifestSHA256=MANIFEST_SHA, rootBundleSHA256=available['android']['sha256'],
                originalGraphBundles=len(names), originalGraphMissingFromAPK=sorted(set(names.values()) - available.keys()),
                roots=sorted(roots), dependencyBundles=len(closure), builtInFiles=len(builtins),
                resources=selected, otherChaptersRemainOriginalGate=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apk', type=Path, default=ROOT / 'apks/original/10056262.apk')
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--awake-scope', action='store_true')
    args = parser.parse_args()
    if args.output.exists():
        parser.error('Use a new evidence file')
    digest = hashlib.file_digest(args.apk.open('rb'), 'sha256').hexdigest()
    if digest != ORIGINAL_APK_SHA:
        parser.error('Initial inventory must use the exact owned original APK')
    inventory = verify_apk(args.apk, expected_manifest_sha=MANIFEST_SHA)
    result = awake_scope(args.apk, inventory) if args.awake_scope else inventory
    result['originalAPKSHA256'] = digest
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps({key: value for key, value in result.items() if key != 'resources'}))


if __name__ == '__main__':
    main()
