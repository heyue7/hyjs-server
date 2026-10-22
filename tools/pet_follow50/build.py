"""Audit every original pet and restore the missing same-model follow clips."""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import zipfile

import UnityPy

from tools.green_hero_compat.build import curves, hierarchy

ROOT = Path(__file__).resolve().parents[2]
BASE = 'fcdce98219b0b51a62b7facecd089170154356a8a264a2914397e12978d052b5'
ENTRY = 'assets/android/model/pet_028_low'
INDEX = 'assets/android/smallassetmanifest'
REQUIRED = {'stand', 'run'}
# Original clips also address stripped helper nodes; these are not skinned bones.
STRIPPED = {
    'pet_011': {'root/Bip001/Bip001 Footsteps'},
    'pet_029': {'root/Bip001/Bip001 Footsteps'},
    'pet_030': {'root/Bip001/Bip001 Pelvis/Bip001 Spine/Bip001 Spine1/Bip001 Neck/Bip001 Head/' + n
                for n in ['Bone001', 'Point002', 'Point003']},
}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def clips(env, archives):
    result = {}
    for obj in env.objects:
        if obj.type.name != 'Animation':
            continue
        for ref in obj.read_typetree()['m_Animations']:
            target = obj.assets_file
            if ref['m_FileID']:
                cab = target.externals[ref['m_FileID'] - 1].path.split('/')[-1].lower()
                assert cab in archives, 'Missing original external animation bundle ' + cab
                target = archives[cab]
            clip = target.objects[ref['m_PathID']]
            assert clip.type.name == 'AnimationClip'
            data = clip.read_typetree()
            result[data['m_Name']] = (clip, data)
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-apk', type=Path, required=True)
    p.add_argument('--output-directory', type=Path, required=True)
    a = p.parse_args()
    out = a.output_directory.resolve()
    assert out.is_relative_to(ROOT / 'build') and not out.exists()
    assert sha(a.base_apk.read_bytes()) == BASE
    out.mkdir(parents=True)
    with zipfile.ZipFile(a.base_apk) as z:
        environments, archives = {}, {}
        # Admit only explicit original pet model/animation bundles, without target execution.
        for name in z.namelist():
            if name.startswith(('assets/android/model/pet_', 'assets/android/models/pet_')):
                env = environments[name] = UnityPy.load(z.read(name))
                for obj in env.objects:
                    archives[obj.assets_file.name.lower()] = obj.assets_file
        low = environments[ENTRY]
        full = environments[ENTRY.removesuffix('_low')]
        assert clips(low, archives) == {}, 'Expected the original empty follow clip list'
        donor = clips(full, archives)
        assert REQUIRED <= donor.keys()
        animation = next(o for o in low.objects if o.type.name == 'Animation')
        tree = animation.read_typetree()
        initial = {o.path_id: sha(o.get_raw_data()) for o in low.objects}
        paths = hierarchy(low)
        added = []
        for name in sorted(REQUIRED):
            source, data = donor[name]
            assert data['m_Legacy'] and {r['path'] for _, r in curves(data)} <= paths.keys()
            target = copy.copy(source)
            target.assets_file = animation.assets_file
            target.path_id = max(target.assets_file.objects) + 1
            target.data = source.get_raw_data()
            if not any(t.class_id == source.serialized_type.class_id for t in target.assets_file.types):
                target.assets_file.types.append(copy.copy(source.serialized_type))
            target.assets_file.objects[target.path_id] = target
            tree['m_Animations'].append({'m_FileID': 0, 'm_PathID': target.path_id})
            if name == 'stand':
                tree['m_Animation'] = {'m_FileID': 0, 'm_PathID': target.path_id}
            added.append({'name': name, 'sourceEntry': ENTRY.removesuffix('_low'),
                          'sourcePathID': source.path_id, 'addedPathID': target.path_id,
                          'clipSHA256': sha(target.data), 'allCurvePathsMatchLowSkeleton': True})
        animation.save_typetree(tree)
        bundle = next(o for o in low.objects if o.type.name == 'AssetBundle')
        bundle_tree = bundle.read_typetree()
        assert len(bundle_tree['m_Container']) == 1
        info = bundle_tree['m_Container'][0][1]
        assert info['preloadIndex'] == 0 and info['preloadSize'] == len(bundle_tree['m_PreloadTable'])
        for item in added:
            bundle_tree['m_PreloadTable'].append({'m_FileID': 0, 'm_PathID': item['addedPathID']})
        info['preloadSize'] += len(added)
        bundle.save_typetree(bundle_tree)
        raw = low.file.save(packer='lz4')
        fixed = UnityPy.load(raw)
        actual = {o.path_id: o for o in fixed.objects}
        assert len(actual) == len(initial) + 2
        for ident, digest in initial.items():
            if ident not in {animation.path_id, bundle.path_id}:
                assert sha(actual[ident].get_raw_data()) == digest, 'Unrelated pet object changed'
        for item in added:
            assert sha(actual[item['addedPathID']].get_raw_data()) == item['clipSHA256']
        environments[ENTRY] = fixed
        text = next(o.read_typetree()['m_Script'] for o in UnityPy.load(z.read('assets/android/tabledata/tablepet')).objects if o.type.name == 'TextAsset')
        lines = text.splitlines()
        pets = [dict(zip(lines[1].split('\t'), line.split('\t'))) for line in lines[5:] if line.startswith('pet_')]
        assert len(pets) == 78
        audit = []
        for pet in pets:
            entry = 'assets/android/model/' + pet['resName'] + '_low'
            env = environments[entry]
            available = clips(env, archives)
            assert REQUIRED <= available.keys(), entry + ' has no follow animation'
            skeleton = hierarchy(env)
            details = []
            for name in sorted(REQUIRED):
                obj, data = available[name]
                bindings = {r['path'] for _, r in curves(data)}
                missing = bindings - skeleton.keys()
                stripped = STRIPPED.get(pet['resName'], set())
                if pet['resName'] == 'pet_001':
                    stripped = {path for path in missing if path.endswith('Nub')}
                assert data['m_Legacy'] and bindings and missing <= stripped, entry + ' incompatible skeleton'
                details.append({'name': name, 'clipSHA256': sha(obj.get_raw_data()),
                                'boundPaths': len(bindings - missing), 'originalStrippedHelperPaths': sorted(missing)})
            audit.append({'petId': pet['id'], 'resName': pet['resName'], 'isFly': pet['isFly'], 'entry': entry, 'clips': details})
        index = z.read(INDEX).decode().splitlines()
        matches = 0
        for i in range(1, len(index)):
            path, digest, size = index[i].split('\t')
            if 'assets/android/' + path == ENTRY:
                index[i] = '\t'.join([path, hashlib.md5(raw).hexdigest(), str(len(raw))])
                matches += 1
        assert matches == 1
        payload = {ENTRY: raw, INDEX: ('\n'.join(index) + '\n').encode()}
        files = []
        for name, value in payload.items():
            dest = out / name
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(value)
            files.append({'entry': name, 'sha256': sha(value), 'beforeSHA256': sha(z.read(name))})
        report = {'schema': 'hyjs-pet-follow50-assets-v1', 'baseAPKSHA256': BASE,
                  'sourcePins': {str(Path(__file__).relative_to(ROOT)): sha(Path(__file__).read_bytes())},
                  'files': files, 'addedClips': added, 'petsAudited': audit,
                  'all78FollowStandRunAndSkeletonsVerified': True,
                  'allOriginalObjectsExceptAnimationAndPreloadExact': True,
                  'samePrefabPreloadAppendedClipCount': 2, 'managedCodeChanged': False,
                  'runtimeVerified': False}
        (out / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps({'pets': len(audit), 'uniqueModels': len({p['resName'] for p in pets}), 'addedClips': added}))


if __name__ == '__main__':
    main()
