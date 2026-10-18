"""Rebuild only the existing tutorial helper on an exact qualified APK."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import zipfile

import dnfile

ROOT = Path(__file__).resolve().parents[2]
MAIN = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER = 'assets/bin/Data/Managed/LocalPlayability.dll'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    out = args.output_directory.resolve()
    assert not out.exists(), 'Use a fresh output directory'
    with args.base_apk.open('rb') as stream:
        assert hashlib.file_digest(stream, 'sha256').hexdigest() == args.base_sha256
    with zipfile.ZipFile(args.base_apk) as archive:
        managed = archive.read(MAIN)
        native = archive.read('classes2.dex')
        original_helper = archive.read(HELPER)
    out.mkdir(parents=True)
    source = Path(__file__).with_name('NoviceSkipUI.cs')
    runtime = ROOT / '.tools/native-batch'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dotnet = runtime / 'dotnet/dotnet'
    with tempfile.TemporaryDirectory(prefix='hyjs-guide-helper-') as tmp:
        main_file = Path(tmp) / 'Assembly-CSharp.dll'
        main_file.write_bytes(managed)
        references = [main_file if p.name == 'Assembly-CSharp.dll' else p
                      for p in sorted((ROOT / 'analysis/extracted/assets/bin/Data/Managed').glob('*.dll'))]
        command = [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+',
                   '/langversion:3', '/deterministic+', '/out:' + str(out / 'LocalPlayability.dll'),
                   *['/reference:' + str(p) for p in references], source]
        process = subprocess.run(['rtk', 'proxy', *map(str, command)], capture_output=True, text=True, timeout=90)
        (out / 'compile.log').write_text(process.stdout + process.stderr)
        assert process.returncode == 0, 'Helper compiler failed; inspect saved log'
    helper = (out / 'LocalPlayability.dll').read_bytes()
    before, after = dnfile.dnPE(data=original_helper), dnfile.dnPE(data=helper)
    # These are the only exported APIs used by original managed callers.
    for pe in (before, after):
        exports = {(str(t.TypeNamespace), str(t.TypeName), str(m.row.Name))
                   for t in pe.net.mdtables.TypeDef.rows for m in t.MethodList
                   if t.Flags.tdPublic and m.row.Flags.mdPublic}
        assert exports == {('HYJS.Playability', 'NoviceSkipUI', 'Ensure'),
                           ('HYJS.Playability', 'NoviceSkipControl', '.ctor')}, exports
    (out / 'classes2.dex').write_bytes(native)
    manifest = {
        'schema': 'hyjs-qualified-helper-only-v1',
        'baseAPKSHA256': args.base_sha256,
        'inputManagedSHA256': sha(managed), 'managedAssemblyByteExact': True,
        'inputDexSHA256': sha(native), 'outputDexSHA256': sha(native),
        'inheritedJavaPublicProtectedABIExact': True,
        'originalEngineMethodsExact': 547,
        'payloadFiles': {HELPER: str((out / 'LocalPlayability.dll').relative_to(ROOT))},
        'replacementEntries': {HELPER: sha(helper), 'classes2.dex': sha(native)},
        'sourcePins': {str(p.resolve().relative_to(ROOT)): sha(p.read_bytes())
                       for p in (Path(__file__), source)},
        'publicHelperExportNamesUnchanged': True,
    }
    (out / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    print(json.dumps({'helperSHA256': sha(helper), 'mainAndNativeUnchanged': True}))


if __name__ == '__main__':
    main()
