"""Compile commerce63 helpers against a SHA-pinned APK; never package or mutate the main DLL."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SOURCES = {
    'SoulFlowerActivity': ['tools/client_activity/ActivityGuard.cs', 'tools/client_activity/InstanceSelector.cs'],
    'SoulFlowerGGOShop': ['tools/ggo_items/client/GGOShop.cs', 'tools/ggo_items/client/GGOIconManifest.cs'],
    'SoulFlowerReWelfare': ['tools/client_re_welfare/ReWelfare.cs'],
}

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--helper', choices=list(SOURCES), help='Compile only one affected helper')
    args = parser.parse_args()
    if sha(args.base_apk.read_bytes()) != args.base_sha256:
        raise ValueError('Pinned APK differs')
    out = args.output_directory.resolve()
    if out.exists():
        raise ValueError('Fresh output directory required')
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    csc = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    report = {'status': 'COMPILED_NOT_PACKAGED', 'inputAPKSHA256': args.base_sha256, 'deviceExecuted': False, 'mainModified': False, 'helpers': {}}
    with tempfile.TemporaryDirectory(prefix='hyjs-commerce63-') as temp, zipfile.ZipFile(args.base_apk) as archive:
        work = Path(temp)
        refs = []
        for name in ['Assembly-CSharp.dll', 'UnityEngine.dll', 'Assembly-CSharp-firstpass.dll', 'mscorlib.dll', 'System.dll', 'System.Core.dll', 'LitJson.dll', 'SoulFlowerHeroArt.dll']:
            path = work / name
            path.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
            refs.append('/reference:' + str(path))
        report['inputManagedSHA256'] = sha((work / 'Assembly-CSharp.dll').read_bytes())
        for name, sources in SOURCES.items():
            if args.helper and name != args.helper:
                continue
            target = out / (name + '.dll')
            run = subprocess.run(['rtk', 'proxy', str(dotnet), str(csc), '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/codepage:65001', '/out:' + str(target), *refs, *[str(ROOT / source) for source in sources]], cwd=ROOT, capture_output=True, text=True)
            (out / (name + '.log')).write_text(run.stdout + run.stderr)
            if run.returncode:
                raise RuntimeError('Helper compile failed: ' + name)
            if name == 'SoulFlowerActivity':
                from verify_activity_arguments import verify
                report['activityReflectionArguments'] = verify(work / 'Assembly-CSharp.dll', target)
                (out / 'activity-arguments-proof.json').write_text(json.dumps(report['activityReflectionArguments'], ensure_ascii=False, indent=2) + '\n')
            report['helpers']['assets/bin/Data/Managed/' + name + '.dll'] = {'path': str(target), 'sha256': sha(target.read_bytes()), 'sourcePins': {source: sha((ROOT / source).read_bytes()) for source in sources}}
    (out / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False))

if __name__ == '__main__':
    main()
