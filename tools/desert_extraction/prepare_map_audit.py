"""Make candidate input for a Unity 5.3 navigation audit; never emits release points."""
import argparse
import hashlib
import json
from pathlib import Path


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--design', type=Path, required=True)
    parser.add_argument('--minimap', type=Path, required=True)
    parser.add_argument('--bundle', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    design = json.loads(args.design.read_text())
    markers = design['mapMarkers']
    expected = {'safe': 1, 'mob': 24, 'mine': 18, 'fish': 12, 'boss': 1, 'exit': 3}
    counts = {kind: sum(m['kind'] == kind for m in markers) for kind in expected}
    if design.get('revision') != 4 or len(markers) != 59 or counts != expected:
        raise ValueError('Unexpected V4 marker inventory: ' + repr(counts))
    ids = [m['id'] for m in markers]
    if len(ids) != len(set(ids)):
        raise ValueError('Duplicate point ID')
    output = {'schema': 'hyjs-desert-v4-candidate-audit-input-v1',
              'designSha256': digest(args.design), 'minimapSha256': digest(args.minimap),
              'bundleSha256': digest(args.bundle),
              'sourceTransform': 'build_layout.py: worldX=(pixelX-256)/4.096+30.4; worldZ=(256-pixelY)/4.096+35.5',
              'points': []}
    for m in markers:
        px, py = m['x'], m['y']
        if not (0 <= px <= 512 and 0 <= py <= 512):
            raise ValueError('Pixel outside 512 map: ' + m['id'])
        output['points'].append({'id': m['id'], 'kind': m['kind'], 'pixelX': px, 'pixelY': py,
                                 'worldX': (px-256)/4.096+30.4, 'worldZ': (256-py)/4.096+35.5})
    if args.output.exists():
        raise FileExistsError('Output must be new')
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(output, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'output': str(args.output), 'sha256': digest(args.output), 'counts': counts}))


if __name__ == '__main__':
    main()
