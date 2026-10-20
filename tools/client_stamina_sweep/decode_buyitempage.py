"""Decode the exact BuyItemPage prefab layout (read-only, static evidence)."""
import json
import struct
import sys
import zipfile
from pathlib import Path

import UnityPy

ROOT = Path(__file__).resolve().parents[2]
APK = ROOT / 'build/hyjs-local.apk'
PIVOT = ['TopLeft', 'Top', 'TopRight', 'Left', 'Center', 'Right', 'BottomLeft', 'Bottom', 'BottomRight']


def string_at(raw, at):
    size = struct.unpack_from('<i', raw, at)[0]
    if not 0 <= size <= len(raw) - at - 4:
        return None, at
    return raw[at + 4:at + 4 + size].decode('utf-8', 'replace'), (at + 4 + size + 3) & ~3


def header(raw):
    go = struct.unpack_from('<iq', raw, 0)
    script = struct.unpack_from('<iq', raw, 16)
    name, at = string_at(raw, 28)
    return go, script, name, at


def widget(raw):
    go, script, name, at = header(raw)
    result = {'name': name}
    # 4 AnchorPoint(target PPtr12 + relative float + absolute int) + updateAnchors bool, aligned
    anchors = []
    for index in range(4):
        base = at + 20 * index
        target = struct.unpack_from('<iq', raw, base)
        relative, absolute = struct.unpack_from('<fi', raw, base + 12)
        anchors.append({'target': target[1], 'relative': round(relative, 3), 'absolute': absolute})
    result['anchors'] = anchors
    result['updateAnchors'] = raw[at + 80]
    base = at + 84
    color = struct.unpack_from('<4f', raw, base)
    pivot, width, height, depth = struct.unpack_from('<4i', raw, base + 16)
    result.update(color=[round(v, 3) for v in color], pivot=PIVOT[pivot] if 0 <= pivot < 9 else pivot,
                  width=width, height=height, depth=depth)
    tail = base + 32
    result['tail'] = list(raw[tail:tail + 48])
    return result


def main():
    with zipfile.ZipFile(APK) as apk:
        manager = next(o for o in UnityPy.load(apk.read('assets/bin/Data/globalgamemanagers')).objects
                       if o.type.name == 'ResourceManager')
        ref = dict(manager.read_typetree()['m_Container'])['pages/buyitempage']
        entry = 'assets/bin/Data/' + manager.assets_file.externals[ref['m_FileID'] - 1].path.rsplit('/', 1)[-1]
        env = UnityPy.load(apk.read(entry))
        scripts = {o.path_id: (o.read_typetree().get('m_Namespace', '') + '.' + o.read_typetree().get('m_ClassName', ''))
                   for o in UnityPy.load(apk.read('assets/bin/Data/globalgamemanagers.assets')).objects
                   if o.type.name == 'MonoScript'}
        objects = {o.path_id: o for o in env.objects}
        components = {}
        for o in env.objects:
            if o.type.name == 'MonoBehaviour':
                go, script, name, at = header(o.get_raw_data())
                components.setdefault(go[1], []).append((o, scripts.get(script[1], '?'), at))
        out = []

        def walk(go_id, depth):
            go = objects[go_id]
            data = go.read_typetree()
            transform = None
            rows = []
            for component in data['m_Component']:
                reference = component[-1] if isinstance(component, tuple) else component['component']
                sub = objects.get(reference['m_PathID'])
                if sub is None:
                    continue
                if sub.type.name == 'Transform':
                    transform = sub.read_typetree()
                elif sub.type.name == 'MonoBehaviour':
                    row = {'class': None}
                    for comp, cls, _ in components.get(go_id, []):
                        if comp.path_id == sub.path_id:
                            row['class'] = cls
                    raw = sub.get_raw_data()
                    row['pathid'] = sub.path_id
                    row['raw'] = len(raw)
                    try:
                        row.update(widget(raw))
                    except Exception as exc:
                        row['error'] = str(exc)
                    if row['class'] and 'UILabel' in row['class']:
                        text, after = string_at(raw, 192)
                        row['text'] = text
                        if after + 4 <= len(raw):
                            row['fontSize'] = struct.unpack_from('<i', raw, after)[0]
                    if row['class'] and 'UISprite' in row['class']:
                        # UISprite fields follow UIWidget: mAtlas PPtr(12), mSpriteName string
                        base = 84 + 40
                        atlas = struct.unpack_from('<iq', raw, base)
                        sprite, _ = string_at(raw, base + 12)
                        row['atlas'] = atlas[1]
                        row['sprite'] = sprite
                    rows.append(row)
                else:
                    rows.append({'class': sub.type.name, 'pathid': sub.path_id})
            pos = transform.get('m_LocalPosition', {}) if transform else {}
            scale = transform.get('m_LocalScale', {}) if transform else {}
            out.append({'depth': depth, 'object': data['m_Name'], 'pathid': go_id,
                        'active': data['m_IsActive'], 'position': [pos.get('x'), pos.get('y'), pos.get('z')],
                        'scale': [scale.get('x'), scale.get('y'), scale.get('z')], 'components': rows})
            for child in (transform or {}).get('m_Children', []):
                child_transform = objects.get(child['m_PathID'])
                if child_transform is not None:
                    child_go = child_transform.read_typetree()['m_GameObject']['m_PathID']
                    walk(child_go, depth + 1)

        walk(ref['m_PathID'], 0)
        print(json.dumps({'entry': entry, 'objects': out}, ensure_ascii=False, indent=1))


if __name__ == '__main__':
    main()
