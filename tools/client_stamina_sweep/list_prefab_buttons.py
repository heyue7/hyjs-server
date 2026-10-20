"""List Button click handlers and nearby names from one original UI prefab."""
import struct
import sys
import zipfile

import UnityPy

ROOT = __import__('pathlib').Path(__file__).resolve().parents[2]
APK = ROOT / 'build/hyjs-local.apk'


def strings(raw, start=0):
    out = []
    for at in range(start, len(raw) - 4):
        size = struct.unpack_from('<i', raw, at)[0]
        if 0 < size < 120 and at + 4 + size <= len(raw):
            try:
                value = raw[at + 4:at + 4 + size].decode('utf-8')
            except Exception:
                continue
            if value and all(ch.isprintable() for ch in value):
                out.append((at, value))
    return out


def main():
    resource = sys.argv[1].lower()
    with zipfile.ZipFile(APK) as apk:
        manager = next(o for o in UnityPy.load(apk.read('assets/bin/Data/globalgamemanagers')).objects
                       if o.type.name == 'ResourceManager')
        found = [(name, ref) for name, ref in manager.read_typetree()['m_Container'] if resource in name.lower()]
        print('RESOURCES', [name for name, _ in found])
        if not found:
            return
        ref = found[0][1]
        entry = 'assets/bin/Data/' + manager.assets_file.externals[ref['m_FileID'] - 1].path.rsplit('/', 1)[-1]
        env = UnityPy.load(apk.read(entry))
        scripts = {}
        for obj in UnityPy.load(apk.read('assets/bin/Data/globalgamemanagers.assets')).objects:
            if obj.type.name == 'MonoScript':
                tree = obj.read_typetree()
                scripts[obj.path_id] = tree.get('m_Namespace', '') + '.' + tree.get('m_ClassName', '')
        objects = {o.path_id: o for o in env.objects}
        names = {}
        for o in env.objects:
            if o.type.name == 'GameObject':
                names[o.path_id] = o.read_typetree()['m_Name']
        for o in env.objects:
            if o.type.name != 'MonoBehaviour':
                continue
            raw = o.get_raw_data()
            script = struct.unpack_from('<iq', raw, 16)
            cls = scripts.get(script[1])
            if cls != '.UIButton':
                continue
            text = strings(raw, 100)
            go = struct.unpack_from('<iq', raw, 0)[1]
            print(o.path_id, names.get(go), text)


if __name__ == '__main__':
    main()
