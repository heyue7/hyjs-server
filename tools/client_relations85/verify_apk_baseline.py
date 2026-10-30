"""Extract only one pinned main to temporary storage, then build/test the overlay."""
import argparse
import hashlib
import json
import re
import subprocess
import tempfile
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'


def sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apk', required=True, type=Path)
    parser.add_argument('--managed-sha256', required=True)
    parser.add_argument('--label', required=True)
    parser.add_argument('--python', default='/Users/lirui/LR/hyjs/.analysis-venv/bin/python', type=Path)
    parser.add_argument('--tool-root', default='/Users/lirui/LR/hyjs', type=Path)
    args = parser.parse_args()
    if not re.fullmatch('[a-z0-9-]+', args.label) or not re.fullmatch('[0-9a-fA-F]{64}', args.managed_sha256):
        raise ValueError('invalid label or SHA')
    apk = args.apk.resolve()
    parent = ROOT / 'analysis/fragment-box85/relations'
    output = parent / ('overlay-' + args.label)
    report = parent / ('model-' + args.label + '.json')
    if output.exists() or report.exists():
        raise ValueError('fresh named output required')
    with tempfile.TemporaryDirectory(prefix='hyjs-relations85-input-') as tmp:
        managed = Path(tmp) / 'Assembly-CSharp.dll'
        with zipfile.ZipFile(apk) as archive:
            if archive.namelist().count(ENTRY) != 1:
                raise ValueError('ambiguous main entry')
            managed.write_bytes(archive.read(ENTRY))
        if sha(managed) != args.managed_sha256.lower():
            raise ValueError('APK managed SHA differs')
        def run(parts):
            result = subprocess.run(['rtk', 'proxy', str(args.python), *map(str, parts)], cwd=ROOT,
                                    capture_output=True, text=True, timeout=180)
            if result.returncode:
                raise RuntimeError(result.stdout + result.stderr)
            print(result.stdout, end='')
        run([HERE/'build.py', '--input-managed', managed, '--input-managed-sha256', args.managed_sha256,
             '--output-directory', output, '--tool-root', args.tool_root])
        manifest = json.loads((output/'manifest.json').read_text())
        run([HERE/'run_tests.py', '--baseline-managed', managed, '--baseline-sha256', args.managed_sha256,
             '--managed', output/'Assembly-CSharp.dll', '--managed-sha256', manifest['outputManagedSHA256'],
             '--report', report, '--tool-root', args.tool_root])
        print(json.dumps({'apkSHA256': sha(apk), 'inputManagedSHA256': args.managed_sha256.lower(),
                          'outputManagedSHA256': manifest['outputManagedSHA256'], 'modelReport': str(report)}))


if __name__ == '__main__':
    main()
