"""Recompile the notice view while preserving an already-integrated client byte for byte."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import zipfile

import dnfile

ROOT = Path(__file__).resolve().parents[2]
SOURCE = Path(__file__).resolve().parent


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    out = args.output_directory.resolve()
    assert out.is_relative_to(ROOT / 'analysis') and not out.exists()
    out.mkdir(parents=True)
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    with zipfile.ZipFile(args.base_apk) as archive, tempfile.TemporaryDirectory(prefix='hyjs-notice-style-') as temporary:
        work = Path(temporary)
        managed = archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
        old_bridge = archive.read('assets/bin/Data/Managed/SoulFlowerNotices.dll')
        host = dnfile.dnPE(data=managed)
        assert any(str(row.Name) == 'SoulFlowerNotices' for row in host.net.mdtables.AssemblyRef.rows)
        references = []
        for name in ['Assembly-CSharp.dll', 'UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll']:
            reference = work / name
            reference.write_bytes(archive.read('assets/bin/Data/Managed/' + name))
            references.append('/reference:' + str(reference))
        bridge = out / 'SoulFlowerNotices.dll'
        run = subprocess.run(['rtk', 'proxy', str(dotnet), str(compiler), '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(bridge), *references, str(SOURCE / 'OriginalNotices.cs')], capture_output=True, text=True, timeout=90)
        (out / 'compile.log').write_text(run.stdout + run.stderr)
        assert run.returncode == 0, 'Bridge compile failed; inspect compile.log'
        before, after = [dnfile.dnPE(data=raw) for raw in [old_bridge, bridge.read_bytes()]]
        def api(pe):
            return [(str(t.TypeNamespace), str(t.TypeName), str(m.row.Name), m.row.Signature.value.hex(), m.row.struct.Flags) for t in pe.net.mdtables.TypeDef.rows for m in t.MethodList if m.row.Flags.mdPublic]
        assert api(before) == api(after), 'Existing injected helper ABI changed'
        payload = 'assets/bin/Data/Managed/SoulFlowerNotices.dll'
        proof = {
            'schema': 'hyjs-notice-style-helper-only-v1',
            'baseAPKSHA256': digest(args.base_apk.read_bytes()),
            'inputManagedSHA256': digest(managed),
            'inputDexSHA256': digest(archive.read('classes2.dex')),
            'inheritedJavaPublicProtectedABIExact': True,
            'inheritedJavaByteExact': True,
            'managedOnly': True,
            'managedAssemblyByteExact': True,
            'allowedChangedMethods': [],
            'payloadFiles': {payload: str(bridge.relative_to(ROOT))},
            'replacementEntries': {payload: digest(bridge.read_bytes())},
            'oldBridgeSHA256': digest(old_bridge),
            'existingHelperPublicABIExact': True,
            'sourcePins': {str(path.relative_to(ROOT)): digest(path.read_bytes()) for path in [Path(__file__), SOURCE / 'OriginalNotices.cs']},
            'deviceExecuted': False
        }
        (out / 'manifest.json').write_text(json.dumps(proof, indent=2) + '\n')
        print(json.dumps({'helperSHA256': proof['replacementEntries'][payload], 'mainAssemblyUnchanged': True, 'publicABIExact': True}))


if __name__ == '__main__':
    main()
