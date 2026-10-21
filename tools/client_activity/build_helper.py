"""Compile only SoulFlowerActivity.dll against an explicitly pinned current APK."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]

def sha(data):
    return hashlib.sha256(data).hexdigest()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    original = args.base_apk.read_bytes()
    if sha(original) != args.base_sha256:
        raise SystemExit('Input APK SHA mismatch')
    out = args.output_directory.resolve()
    if out.exists():
        raise SystemExit('Use a fresh output directory')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    sources = [Path(__file__).with_name(name) for name in ['ActivityGuard.cs', 'InstanceSelector.cs']]
    with tempfile.TemporaryDirectory(prefix='hyjs-personal40-helper-') as temp:
        work = Path(temp)
        references = []
        with zipfile.ZipFile(args.base_apk) as archive:
            for name in ['Assembly-CSharp.dll', 'UnityEngine.dll', 'Assembly-CSharp-firstpass.dll', 'mscorlib.dll', 'System.dll', 'System.Core.dll']:
                path = work / name
                path.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
                references.append(path)
            main_sha = sha(archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll'))
        helper = out / 'SoulFlowerActivity.dll'
        result = subprocess.run(['rtk', 'proxy', str(dotnet), str(compiler), '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(helper), *['/reference:' + str(p) for p in references], *map(str, sources)], cwd=ROOT, capture_output=True, text=True)
        (out / 'compile.log').write_text(result.stdout + result.stderr)
        if result.returncode:
            raise SystemExit('Helper compile failed; inspect compile.log')
    report = {'schema': 'hyjs-personal40-activity-helper-v1', 'status': 'COMPILED_NOT_PACKAGED', 'inputAPKSHA256': args.base_sha256, 'inputManagedSHA256': main_sha, 'originalMainCopiedOrModified': False, 'helperSHA256': sha(helper.read_bytes()), 'replacementEntries': ['assets/bin/Data/Managed/SoulFlowerActivity.dll'], 'sourcePins': {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in sources}, 'phoneVerified': False}
    (out / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report))

if __name__ == '__main__':
    main()
