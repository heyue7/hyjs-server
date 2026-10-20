"""Read-only prefab hierarchy inspector rooted at the ResourceManager container."""
import json
import sys
import zipfile
from pathlib import Path

import UnityPy

ROOT = Path(__file__).resolve().parents[2]
APK = ROOT / 'build/hyjs-local.apk'


def main():
    needle = sys.argv[1].lower()
    with zipfile.ZipFile(APK) as apk:
        manager = next(o for o in UnityPy.load(apk.read('assets/bin/Data/globalgamemanagers')).objects
                       if o.type.name == 'ResourceManager')
        container = manager.read_typetree()['m_Container']
        matches = [(name, ref) for name, ref in container if needle in name.lower()]
        for name, ref in matches:
            print('RESOURCE', name, ref)
        if not matches:
            return
        name, ref = matches[0]
        scripts = {}
        for obj in UnityPy.load(apk.read('assets/bin/Data/globalgamemanagers.assets')).objects:
            if obj.type.name == 'MonoScript':
                t = obj.read_typetree()
                scripts[obj.path_id] = t.get('m_Namespace', '') + '.' + t.get('m_ClassName', '')
        loaded = {}

        def entry_of(obj, reference):
            if not reference['m_FileID']:
                for key, env in loaded.items():
                    if obj.assets_file in env.files.values():
                        return key
                raise KeyError('current file unknown')
            path = obj.assets_file.externals[reference['m_FileID'] - 1].path
            if path.startswith('archive:/'):
                path = path.rsplit('/', 1)[-1]
            return 'assets/bin/Data/' + path

        def load(entry):
            if entry not in loaded:
                loaded[entry] = UnityPy.load(apk.read(entry))
            return loaded[entry]

        env = load(entry_of(manager, ref))
        root = next(o for o in env.objects if o.path_id == ref['m_PathID'])
        print('ROOT', root.type.name, root.path_id)

        def deref(obj, reference):
            if not reference or not reference.get('m_PathID'):
                return None
            other = load(entry_of(obj, reference))
            return next((o for o in other.objects if o.path_id == reference['m_PathID']), None)

        def tree(obj):
            try:
                return obj.read_typetree()
            except Exception:
                return {}

        def dump(go, depth=0, seen=None):
            seen = seen or set()
            if go is None or go.path_id in seen:
                return
            seen.add(go.path_id)
            data = tree(go)
            print('  ' * depth + f'[GO] {data.get("m_Name", "?")} path={go.path_id} active={data.get("m_IsActive")}')
            transform = None
            for component in data.get('m_Component', []):
                reference = component.get('component') if isinstance(component, dict) else component[-1]
                sub = deref(go, reference)
                if sub is None:
                    continue
                st = tree(sub)
                extra = ''
                if sub.type.name == 'MonoBehaviour':
                    script = scripts.get((st.get('m_Script') or {}).get('m_PathID'), '?')
                    extra = ' script=' + script
                    keys = {}
                    for key, value in st.items():
                        if key in ('m_GameObject', 'm_Script'):
                            continue
                        keys[key] = value
                    try:
                        rendered = json.dumps(keys, ensure_ascii=False, default=str)
                    except Exception:
                        rendered = str(keys)
                    print('  ' * (depth + 1) + f'{sub.type.name}{extra} raw={len(sub.get_raw_data())} {rendered[:1500]}')
                else:
                    print('  ' * (depth + 1) + f'{sub.type.name}{extra} {json.dumps(st, ensure_ascii=False, default=str)[:500]}')
                if sub.type.name == 'Transform':
                    transform = st
            if transform:
                for child in transform.get('m_Children', []):
                    sub = deref(go, child)
                    if sub is None:
                        continue
                    child_go = deref(sub, tree(sub).get('m_GameObject'))
                    dump(child_go, depth + 1, seen)

        dump(root)
    print('done')


if __name__ == '__main__':
    main()
