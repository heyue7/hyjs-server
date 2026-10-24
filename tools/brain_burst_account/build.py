"""Rebuild the existing account/session helpers and native UI over an explicit current APK.

The original managed game assembly remains byte exact. No APK installation or promotion.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import zipfile

import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.local_asset_ready.remove_gate import require


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def public_api(raw):
    assembly = dnfile.dnPE(data=raw)
    result = {}
    for owner in assembly.net.mdtables.TypeDef:
        if not (owner.Flags.tdPublic or owner.Flags.tdNestedPublic):
            continue
        for member in owner.MethodList:
            method = member.row
            if method.Flags.mdPublic or method.Flags.mdFamily or method.Flags.mdFamORAssem:
                result[(str(owner.TypeNamespace), str(owner.TypeName), str(method.Name), method.Signature.value.hex())] = True
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-sha256', required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    base, out = args.base_apk.resolve(), args.output_directory.resolve()
    require(sha(base.read_bytes()) == args.base_sha256, 'Base APK changed')
    require(not out.exists() and out.is_relative_to(ROOT), 'Use fresh project output')
    subprocess.run(['rtk', 'proxy', sys.executable, str(ROOT / 'tools/login_ui/build.py'),
                    '--base-apk', str(base), '--base-sha256', args.base_sha256,
                    '--dex-only', '--output-directory', str(out)], check=True)
    proof = json.loads((out / 'manifest.json').read_text())
    runtime = ROOT / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    managed = 'assets/bin/Data/Managed/'
    inputs = [Path(__file__).resolve(), ROOT / 'tools/client_native_account/NativeAccount.cs',
              ROOT / 'tools/local_wallet_bridge/LocalWalletSessionBridge.cs',
              ROOT / 'tools/client_native_account/resources/sao-v2/button-atlas.png',
              ROOT / 'tools/local_wallet/assets/icon.png']
    payload = {managed + name: str((out / name).relative_to(ROOT)) for name in
               ['SoulFlowerNativeAccount.dll', 'LocalWalletSession.dll']}
    payload['assets/local-wallet/icon.png'] = 'tools/local_wallet/assets/icon.png'
    with zipfile.ZipFile(base) as source, tempfile.TemporaryDirectory(prefix='hyjs-brain-burst-') as tmp:
        work = Path(tmp)
        refs = []
        for name in ['Assembly-CSharp.dll', 'UnityEngine.dll', 'mscorlib.dll',
                     'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll']:
            path = work / name
            path.write_bytes(source.read(managed + name))
            refs.append('/reference:' + str(path))
        for name, source_file, resources in [
            ('SoulFlowerNativeAccount.dll', inputs[1],
             ['/resource:' + str(inputs[3]) + ',HYJS.AccountButtons.png']),
            ('LocalWalletSession.dll', inputs[2], []),
        ]:
            output = out / name
            process = subprocess.run(['rtk', 'proxy', str(dotnet), str(compiler), '/nologo',
                                      '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+',
                                      '/out:' + str(output), *refs, *resources, str(source_file)],
                                     capture_output=True, text=True, timeout=120)
            (out / (name + '.compile.log')).write_text(process.stdout + process.stderr)
            require(process.returncode == 0, 'Helper compile failed: ' + name)
            require(set(public_api(source.read(managed + name))).issubset(public_api(output.read_bytes())),
                    'Inherited managed helper API removed: ' + name)
    proof.update(schema='brain-burst-account-overlay-v1', payloadFiles=payload,
                 replacementEntries={name: sha((ROOT / rel).read_bytes()) for name, rel in payload.items()},
                 managedAssemblyByteExact=True, apkPackaged=False, phoneVerified=False)
    proof['replacementEntries']['classes2.dex'] = proof['outputDexSHA256']
    proof['sourcePins'].update({str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in inputs})
    (out / 'manifest.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'status': 'QUALIFIED_ACCOUNT_HELPERS_AND_DEX',
                      'managedAssemblyByteExact': True, 'replacementEntries': proof['replacementEntries']}))


if __name__ == '__main__':
    main()
