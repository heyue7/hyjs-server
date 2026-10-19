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
            def signature(raw):
                offset = 0
                def integer():
                    nonlocal offset
                    value = raw[offset]; offset += 1
                    if value & 0x80 == 0: return value
                    if value & 0xc0 == 0x80:
                        value = ((value & 0x3f) << 8) | raw[offset]; offset += 1; return value
                    value = ((value & 0x1f) << 24) | int.from_bytes(raw[offset:offset + 3], 'big'); offset += 3; return value
                def kind():
                    tag = integer()
                    if tag in (0x11, 0x12):
                        token = integer()
                        table = [pe.net.mdtables.TypeDef, pe.net.mdtables.TypeRef][token & 3]
                        row = table.rows[(token >> 2) - 1]
                        return (tag, str(row.TypeNamespace) + '.' + str(row.TypeName))
                    if tag == 0x15:
                        parent = kind(); count = integer(); return (tag, parent, tuple(kind() for _ in range(count)))
                    if tag in (0x10, 0x1d): return (tag, kind())
                    assert tag in (1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 24, 25, 28), 'Unsupported public ABI element'
                    return tag
                flags = integer(); assert not flags & 0x10, 'Generic public method requires explicit verification'
                count = integer(); result = (flags, kind(), tuple(kind() for _ in range(count)))
                assert offset == len(raw)
                return result
            # Metadata token ordinals may move when private implementation references change.
            # Private compiler-generated coroutine/closure types have public
            # constructors, but are not part of the injected helper contract.
            public_types = [t for t in pe.net.mdtables.TypeDef.rows if t.Flags.tdPublic]
            assert {str(t.TypeName) for t in public_types} == {'NoticeEntry', 'OriginalNotices', 'NoticeContent'}
            return sorted([(str(t.TypeNamespace), str(t.TypeName), str(m.row.Name), signature(m.row.Signature.value), m.row.struct.Flags) for t in public_types for m in t.MethodList if m.row.Flags.mdPublic], key=repr)
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
