"""Read selected prefab component references and bytes; no asset mutation."""
import struct
import sys
import zipfile
import json
import UnityPy

with zipfile.ZipFile(sys.argv[1]) as z:
    manager = next(x for x in UnityPy.load(z.read('assets/bin/Data/globalgamemanagers')).objects if x.type.name == 'ResourceManager')
    ref = dict(manager.read_typetree()['m_Container'])['pages/uicopysweep']
    entry = 'assets/bin/Data/' + manager.assets_file.externals[ref['m_FileID']-1].path.rsplit('/', 1)[-1]
    env = UnityPy.load(z.read(entry))
    scripts = {}
    for s in UnityPy.load(z.read('assets/bin/Data/globalgamemanagers.assets')).objects:
        if s.type.name == 'MonoScript':
            tree = s.read_typetree()
            scripts[s.path_id] = '.'.join(filter(None, (tree.get('m_Namespace'), tree.get('m_ClassName'))))
    print('entry', entry)
    objects = {o.path_id: o for o in env.objects}
    for obj in env.objects:
        if obj.type.name != 'MonoBehaviour':
            continue
        raw = obj.get_raw_data()
        go = struct.unpack_from('<iq', raw, 0)
        script = struct.unpack_from('<iq', raw, 16)
        name = scripts.get(script[1], '?')
        if name in ('UI.UICopySweep', 'TweenAlpha', 'TweenScale'):
            print('component', obj.path_id, 'gameobject', go[1], 'script', name, 'size', len(raw))
            for offset in range(0, len(raw), 4):
                value = struct.unpack_from('<I', raw, offset)[0]
                f = struct.unpack_from('<f', raw, offset)[0]
                print(f'  {offset:03x} {value:08x} {f:.6g}')
    # Header and field offsets belong to this pinned Unity 5.3 prefab. The
    # UITweener serialization has two curve keys before delay/duration.
    root = objects[145].get_raw_data()
    title_ref = struct.unpack_from('<iq', root, 0xd8)
    body_ref = struct.unpack_from('<iq', root, 0xe4)
    assert title_ref == (0, 178) and body_ref == (0, 146)
    rows = {}
    for label, path, expected in (('titleAlpha', title_ref[1], 'TweenAlpha'), ('bodyScale', body_ref[1], 'TweenScale')):
        raw = objects[path].get_raw_data()
        script = struct.unpack_from('<iq', raw, 16)
        assert scripts[script[1]] == expected and struct.unpack_from('<i', raw, 0x28)[0] == 2
        delay, duration = struct.unpack_from('<ff', raw, 0x5c)
        rows[label] = {'componentPathID':path, 'type':expected, 'delay':delay, 'duration':duration, 'durationOffset':96}
    assert abs(rows['titleAlpha']['duration'] - .2) < 1e-6 and abs(rows['bodyScale']['duration'] - .25) < 1e-6
    print('WAIT_SUMMARY', json.dumps({'entry':entry, 'rootPathID':32, 'pageComponentPathID':145,
                                     'serializedValues':rows, 'waitSeconds':rows['titleAlpha']['duration'] + rows['bodyScale']['duration']}))
