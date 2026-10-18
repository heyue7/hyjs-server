"""Clone a verified local candidate and apply only SHA-pinned agent deliveries."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]
CANDIDATES = ROOT / 'build/candidates'


def sha(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def local_path(name):
    path = (ROOT / name).resolve()
    if not path.is_relative_to(ROOT):
        raise RuntimeError('Delivery source must remain inside the project')
    return path


def delivery(path):
    data = json.loads(path.read_text())
    status = str(data.get('status', '')).upper()
    if data.get('stable') is False or status.startswith(('WIP', 'UNTESTED', 'IN_PROGRESS')):
        raise RuntimeError('An unfinished delivery cannot enter a fixed candidate')
    records = data.get('files', data.get('owned'))
    result = []
    if isinstance(records, dict):
        names = data.get('owned_overlay')
        if names is None or names is True:
            names = list(records)
        if not isinstance(names, list):
            raise RuntimeError('Invalid owned delivery selection')
        for name in names:
            source = local_path(str(Path(data['snapshot']) / name)) if data.get('snapshot') else path.parent / name
            result.append((name, source, records[name]))
    elif isinstance(records, list):
        for row in records + data.get('requiredNewGoldenAssets', []) + data.get('requiredAssets', []):
            snapshot = row.get('snapshot')
            if snapshot is None and data.get('snapshot'):
                snapshot = str(Path(data['snapshot']) / row['path'])
            source = local_path(snapshot or row['path'])
            result.append((row['path'], source, row['sha256']))
    else:
        raise RuntimeError('Unsupported pinned delivery manifest')
    if not result:
        raise RuntimeError('Delivery has no owned files')
    for name, source, digest in result:
        relative = Path(name)
        if relative.is_absolute() or '..' in relative.parts or not source.resolve().is_relative_to(ROOT):
            raise RuntimeError('Invalid pinned delivery path')
        if sha(source) != digest:
            raise RuntimeError('Delivery changed after verification: ' + name)
    return data, result


def validate_overlay_base(metadata, rows, planned):
    """Reject a delivery made against stale shared source before cloning."""
    declarations = metadata.get('files', metadata.get('owned'))
    declarations = declarations if isinstance(declarations, list) else []
    by_name = {row['path']: row for row in declarations}
    for name, source, digest in rows:
        row = by_name.get(name, {})
        field = next((key for key in ('beforeSHA256', 'baseSHA256') if key in row), None)
        actual = planned.get(name)
        # A newly pinned unchanged external tool may quote its own source SHA
        # as the preimage, even when the base did not carry that tool yet.
        unchanged_new_pin = field and actual is None and row[field] == digest
        if field and actual not in (row[field], digest) and not unchanged_new_pin:
            raise RuntimeError('Delivery requires rebasing against current source: ' + name)
        planned[name] = digest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', type=Path, required=True)
    parser.add_argument('--name', required=True)
    parser.add_argument('--overlay', type=Path, action='append', default=[])
    args = parser.parse_args()
    base = args.base.resolve()
    if base.parent != CANDIDATES or not re.fullmatch(r'[a-z0-9-]{1,80}', args.name):
        raise RuntimeError('Use an owned candidate and a bounded local name')
    target = CANDIDATES / args.name
    if target.exists():
        raise RuntimeError('Keep existing fixed candidates immutable')
    baseline = json.loads((base / 'manifest.json').read_text())
    if not baseline.get('stable'):
        raise RuntimeError('Base candidate must have completed verification')
    records = {row['path']: row for row in baseline['files']}
    for name, row in records.items():
        if sha(base / name) != row['sha256']:
            raise RuntimeError('Verified base candidate changed: ' + name)
    overlays = []
    planned = {name: row['sha256'] for name, row in records.items()}
    for path in args.overlay:
        manifest = local_path(path)
        metadata, rows = delivery(manifest)
        validate_overlay_base(metadata, rows, planned)
        overlays.append((manifest, metadata, rows))
    # APFS clone files share unchanged blocks and preserve independent writes.
    # Fail instead of silently duplicating large historical evidence trees.
    subprocess.run(['rtk', 'proxy', 'cp', '-cR', str(base), str(target)], check=True)
    pinned = []
    try:
        for manifest, metadata, rows in overlays:
            for name, source, digest in rows:
                destination = target / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, destination)
                if sha(destination) != digest:
                    raise RuntimeError('Copied delivery failed hash verification')
                records[name] = {'path': name, 'sha256': digest}
            pinned.append({'manifest': str(manifest.relative_to(ROOT)), 'sha256': sha(manifest), 'ownedFiles': len(rows)})
            if metadata.get('engineVersion'):
                baseline['engineVersion'] = metadata['engineVersion']
        # Verification belongs to exact source bytes. A new overlay starts
        # unqualified even when its base candidate has completed all checks.
        for inherited in ('validation', 'qualification', 'criticalSmoke', 'fixtureRepair'):
            baseline.pop(inherited, None)
        baseline.update(stable=False, files=sorted(records.values(), key=lambda row: row['path']),
                        fixedBase=str(base.relative_to(ROOT)), pinnedDeliveries=pinned)
        (target / 'manifest.json').write_text(json.dumps(baseline, indent=2) + '\n')
    except Exception:
        # This directory was created solely by this invocation, has never been
        # published and is unusable. Remove its partial clone immediately.
        shutil.rmtree(target)
        raise
    print(json.dumps({'candidate': str(target), 'files': len(records), 'deliveries': pinned, 'copyMode': 'APFS-clone'}))


if __name__ == '__main__':
    main()
