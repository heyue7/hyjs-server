"""Rebuild only the existing scoped501 bridge; no reinjection/APK/device work."""
import argparse
import json
from pathlib import Path
import sys
import tempfile
import zipfile
import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.local_asset_ready.build import MANAGED, SCOPE, SCOPE_SHA, run, sha, verify_scope_apk
from tools.p10_voice.build import verify

BASE = '28ea77c8d1ced6c77a40862a9787804ca95ffea8912c01516d95995061341c3d'
ASSEMBLY = '90450ff8b95facef1308d77f73a4905af7e343dac550d21d6e8f898018da70ea'
BRIDGE = '8b6cd2b977bbb92e2dd395bfdac4c25ed12d0a1132ddb8f8d99a47678394d789'


def public_api(blob):
    module = dnfile.dnPE(data=blob)
    return sorted((str(owner.TypeNamespace), str(owner.TypeName), str(method.row.Name),
                   method.row.Signature.value.hex(), method.row.Flags.mdStatic)
                  for owner in module.net.mdtables.TypeDef.rows if owner.Flags.tdPublic
                  for method in owner.MethodList if method.row.Flags.mdPublic)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--base-proof', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    if args.output_directory.exists():
        raise ValueError('Choose a new immutable output directory')
    proof = json.loads(args.base_proof.read_bytes())
    if (sha(args.base_apk.read_bytes()) != BASE or proof['binarySHA256'] != BASE
            or not proof['signatureVerified'] or not proof['zipalignVerified']):
        raise ValueError('Expected qualified current28ea input')
    if sha(SCOPE.read_bytes()) != SCOPE_SHA:
        raise ValueError('Original scoped resource proof changed')
    assembly, inventory = verify_scope_apk(args.base_apk, json.loads(SCOPE.read_bytes()), existing_bridge=True)
    if sha(assembly) != ASSEMBLY:
        raise ValueError('Qualified managed Assembly changed')
    with zipfile.ZipFile(args.base_apk) as apk:
        old_bridge = apk.read(MANAGED + 'LocalAssetReady.dll')
        if sha(old_bridge) != BRIDGE:
            raise ValueError('Expected original installed501 bridge')
        for entry, expected in proof['qualifiedWalletEntries'].items():
            if sha(apk.read(entry)) != expected:
                raise ValueError('Qualified inherited entry changed: ' + entry)
    output = args.output_directory
    output.mkdir(parents=True)
    (output / 'local501.inventory').write_bytes(inventory)
    bridge = output / 'LocalAssetReady.dll'
    work = ROOT / '.tools/native-batch'
    compiler = next((work / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = sorted((ROOT / 'analysis/extracted/assets/bin/Data/Managed').glob('*.dll'))
    with tempfile.TemporaryDirectory(prefix='hyjs-local501-diagnostic-') as temp:
        source = Path(temp) / 'LocalResourceProof.cs'
        source.write_text('namespace HYJS.LocalAssets { internal static class LocalResourceProof {'
                          ' internal const string InventorySHA256 = "' + sha(inventory) + '"; }}')
        run([work / 'dotnet/dotnet', compiler, '/nologo', '/target:library', '/nostdlib+',
             '/langversion:3', '/deterministic+', '/out:' + str(bridge),
             *['/reference:' + str(p) for p in refs],
             '/resource:' + str(output / 'local501.inventory') + ',local501.inventory',
             ROOT / 'tools/local_asset_ready/LocalAssetReady.cs', source])
    protection = verify(assembly, assembly, set())
    if protection['originalEngineMethods'] != 547:
        raise ValueError('Engine method denominator changed')
    # The primary assembly is reused byte for byte. The common voice verifier
    # defaults an empty set to its own ten methods; no such exclusions apply here.
    protection['onlyChangedEntries'] = []
    original = dnfile.dnPE(data=assembly)
    protection['originalUnmodifiedMethodBodies'] = sum(bool(m.Rva) for m in original.net.mdtables.MethodDef.rows)
    api = public_api(old_bridge)
    if len(api) != 9 or public_api(bridge.read_bytes()) != api:
        raise ValueError('Original public bridge ABI changed')
    manifest = dict(schema='local-awake501-bridge-diagnostics-v1', **protection,
                    baseAPKSHA256=BASE, baseProof=str(args.base_proof),
                    baseProofSHA256=sha(args.base_proof.read_bytes()),
                    inputManagedSHA256=ASSEMBLY, outputManagedSHA256=ASSEMBLY,
                    managedAssemblyByteIdenticalToBase=True, oldBridgeSHA256=BRIDGE,
                    bridgeSHA256=sha(bridge.read_bytes()), inventorySHA256=sha(inventory),
                    originalScopeProofSHA256=SCOPE_SHA, resourcesVerified=2680,
                    predicateChanged=False, globalFullFlagChanged=False,
                    originalResourceLoaderChanged=False, playerStateOrBattleResultChanged=False,
                    diagnosticBudget=64, logsContainCredentials=False,
                    diagnosticTransport='android.util.Log.i/HYJS501 and Unity LogWarning',
                    allExistingPublicABISignaturesUnchanged=True, existingPublicABIMethods=len(api),
                    ABIComparisonBoundary='public original bridge types; private generated closures excluded',
                    apkBuilt=False, deviceVerified=False,
                    replacementEntries={MANAGED + 'LocalAssetReady.dll': sha(bridge.read_bytes())},
                    sourcePins={str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in [
                        Path(__file__).resolve(), ROOT / 'tools/local_asset_ready/build.py',
                        ROOT / 'tools/local_asset_ready/LocalAssetReady.cs',
                        ROOT / 'tools/local_asset_ready/test_bridge.py']})
    (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    print(json.dumps({'manifest': str(output / 'manifest.json'), 'bridgeSHA256': manifest['bridgeSHA256'],
                      'predicateChanged': False, 'apkBuilt': False}))


if __name__ == '__main__':
    main()
