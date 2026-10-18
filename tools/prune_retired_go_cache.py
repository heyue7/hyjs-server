"""Preview or remove large Go objects whose owned source directories are gone.

Shared toolchain/dependency objects and existing candidate directories are kept.
Run after a development batch; --apply rechecks every retired source and object.
"""
import argparse
import json
import mmap
import math
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[1]
MIN_BYTES = 8 * 1024 * 1024


def owned_patterns():
    # Each component has a bounded alphabet. Do not let a match consume file
    # suffixes or concatenate multiple source paths embedded in Go archives.
    return (
        re.compile(rb'/(?:private/)?var/folders/[A-Za-z0-9_-]+/'
                   rb'[A-Za-z0-9_-]+/T/hyjs-[A-Za-z0-9_-]+'),
        re.compile(re.escape(str(ROOT / 'build/candidates').encode())
                   + rb'/[A-Za-z0-9_-]+'),
        # These diagnostic clones are owned by the root batch and removed
        # only after the bounded Go child exits. Existing clones stay hot.
        re.compile(re.escape(str(ROOT / 'build').encode())
                   + rb'/hyjs-(?:phone13|natural-family)-[A-Za-z0-9_-]+'),
    )


def inspect(cache, min_bytes=MIN_BYTES):
    patterns = owned_patterns()
    rows = []
    for path in cache.glob('*/*-d'):
        if path.is_symlink() or not path.is_file():
            continue
        before = path.stat()
        if before.st_size < min_bytes:
            continue
        with path.open('rb') as file:
            with mmap.mmap(file.fileno(), 0, access=mmap.ACCESS_READ) as data:
                sources = sorted({match.group().decode() for pattern in patterns
                                  for match in pattern.finditer(data)})
        if sources and all(not Path(source).exists() for source in sources):
            rows.append({'path': str(path), 'bytes': before.st_size,
                         'mtimeNS': before.st_mtime_ns, 'inode': before.st_ino,
                         'retiredSourceDirectories': sources})
    return rows


def apply(cache, rows):
    removed, skipped = [], []
    for row in rows:
        path = Path(row['path'])
        if (path.parent.parent != cache or path.is_symlink()
                or not re.fullmatch(r'[0-9a-f]{64}-d', path.name)):
            raise RuntimeError('Unexpected Go object path')
        try:
            current = path.stat()
        except FileNotFoundError:
            skipped.append(row['path'])
            continue
        if (current.st_size != row['bytes'] or current.st_mtime_ns != row['mtimeNS']
                or current.st_ino != row['inode']
                or any(Path(source).exists() for source in row['retiredSourceDirectories'])):
            skipped.append(row['path'])
            continue
        path.unlink()
        removed.append(row)
    return removed, skipped


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--min-mib', type=float, default=8,
                        help='Minimum object size; ownership and retired-source checks always apply')
    args = parser.parse_args()
    if not math.isfinite(args.min_mib) or not 1 <= args.min_mib <= 512:
        parser.error('--min-mib must be between 1 and 512')
    cache = Path(subprocess.check_output(
        ['rtk', 'proxy', 'go', 'env', 'GOCACHE'], text=True).strip()).resolve()
    if not cache.is_dir():
        raise RuntimeError('GOCACHE must be an existing directory')
    rows = inspect(cache, int(args.min_mib * 1024 * 1024))
    removed, skipped = apply(cache, rows) if args.apply else ([], [])
    print(json.dumps({'cache': str(cache), 'apply': args.apply,
                      'minimumObjectMiB': args.min_mib,
                      'retiredObjects': len(rows),
                      'reclaimableBytes': sum(row['bytes'] for row in rows),
                      'removedObjects': len(removed),
                      'reclaimedBytes': sum(row['bytes'] for row in removed),
                      'skippedObjects': len(skipped),
                      'retiredSources': sorted({source for row in rows
                                               for source in row['retiredSourceDirectories']}),
                      'retained': 'SDK/shared dependencies/existing owned sources'}))


if __name__ == '__main__':
    main()
