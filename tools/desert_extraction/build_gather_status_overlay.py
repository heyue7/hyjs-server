"""Rebuild the shipped 1.1.4 helper, then compile the gather recovery overlay.

This produces a helper and proof manifest only. It does not build an APK or
contact a device. Both the input APK and all production sources are pinned.
"""
import argparse
from collections import Counter
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

import dnfile

from build_boss_countdown_overlay import assembly_refs, public_abi, run, sha, source_list

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
BASE_COMMIT = 'e24e48f492933ca1f2e755882ecb61797d782cf2'
CANDIDATE_COMMIT = 'dc030d6c4de1b7c06815b1090f267a4642d604bd'
APK_SHA = '5b8d368fad415204a4b9f67d512c36fe6614e4fe1055d1d362a93ec9dc26b695'
MAIN_SHA = '3d8f50e543ad082ea03bff294f17421a2eed4316d5cda664b03cc0f46cc23e8a'
HELPER_SHA = '3080d87917cf578f073f303e9c3bb33fbfb99bae8cb964b344890bf37c77b68b'
DEX_SHA = '131bacb3847eda65274c4111fbbd7cf02d5e437b5363f414210828f6d03fa143'
HELPER_ENTRY = 'assets/bin/Data/Managed/SoulFlowerDesertV4.dll'
ICON = 'assets/evacuation-npc.png'
CHANGED_SOURCES = {'GatherAttempt.cs', 'GatherCoordinator.cs'}

# Additions are intentional; all shipped public ABI entries must remain exact.
ABI_ADDITIONS = Counter({
    ('field', 'HYJS.DesertV4.GatherPhase', 'CheckingClaim', 'valuetype:def:HYJS.DesertV4.GatherPhase', 32854): 1,
    ('method', 'HYJS.DesertV4.GatherAttempt', 'CheckClaimStatus', '32/0:void()', 134, 0): 1,
    ('method', 'HYJS.DesertV4.GatherAttempt', 'ResumeCheckedClaim', '32/0:void(string,string)', 134, 0): 1,
    ('method', 'HYJS.DesertV4.GatherAttempt', 'StatusCancellation', '32/0:void(string,string)', 134, 0): 1,
})
OLD_PHASES = {'Starting': 0, 'Active': 1, 'Cancelling': 2, 'Claiming': 3,
              'RecoverReceipt': 4, 'Finished': 5, 'PendingDelivery': 6}


def committed(commit, path):
    result = subprocess.run(['rtk', 'proxy', 'git', 'show', commit + ':tools/desert_extraction/' + path],
                            cwd=ROOT, capture_output=True, timeout=30)
    if result.returncode:
        raise RuntimeError('Unable to read pinned source %s:%s' % (commit, path))
    return result.stdout


def phase_values(pe):
    owner = next(row for row in pe.net.mdtables.TypeDef.rows
                 if str(row.TypeNamespace) == 'HYJS.DesertV4' and str(row.TypeName) == 'GatherPhase')
    fields = [field.row for field in owner.FieldList]
    return {str(item.Parent.row.Name): int.from_bytes(item.Value.value, 'little', signed=True)
            for item in pe.net.mdtables.Constant.rows if item.Parent.row in fields}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apk', type=Path, required=True, help='pinned installed 1.1.4 APK')
    parser.add_argument('--runtime', type=Path, required=True, help='native-batch root')
    parser.add_argument('--output', type=Path, required=True, help='fresh helper output directory')
    args = parser.parse_args()
    if sha(args.apk) != APK_SHA:
        raise ValueError('Expected the exact pinned 1.1.4 APK')
    output = args.output.resolve()
    if output.exists() or not output.is_relative_to(ROOT):
        raise ValueError('Output must be a fresh path inside this worktree')
    output.parent.mkdir(parents=True, exist_ok=True)
    if (HERE / 'build_boss_countdown_overlay.py').read_bytes() != committed(BASE_COMMIT, 'build_boss_countdown_overlay.py'):
        raise ValueError('ABI checker differs from installed build baseline')
    source_script = (HERE / 'build_field_patch.py').read_bytes()
    if source_script != committed(BASE_COMMIT, 'build_field_patch.py'):
        raise ValueError('Production source list script differs from installed baseline')
    sources = source_list(source_script.decode())
    if sources != source_list(committed(CANDIDATE_COMMIT, 'build_field_patch.py').decode()):
        raise ValueError('Candidate source list differs from installed baseline')
    for name in (*sources, ICON):
        if (HERE / name).read_bytes() != committed(CANDIDATE_COMMIT, name):
            raise ValueError('Working source differs from pinned candidate: ' + name)
    changed = [name for name in sources if committed(BASE_COMMIT, name) != (HERE / name).read_bytes()]
    if set(changed) != CHANGED_SOURCES:
        raise ValueError('Unexpected source changes from 1.1.4: %s' % changed)
    if committed(BASE_COMMIT, ICON) != (HERE / ICON).read_bytes():
        raise ValueError('Embedded resource differs from 1.1.4')

    runtime = args.runtime / 'dotnet'
    dotnet = runtime / 'dotnet'
    compiler = next((runtime / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    env = dict(os.environ, DOTNET_PROCESSOR_COUNT='2', DOTNET_CLI_TELEMETRY_OPTOUT='1')
    names = ('Assembly-CSharp.dll', 'Assembly-CSharp-firstpass.dll', 'UnityEngine.dll', 'mscorlib.dll',
             'System.dll', 'System.Core.dll', 'LitJson.dll', 'HYJS.Field63.dll', 'LocalFieldSettlement.dll')
    with tempfile.TemporaryDirectory(prefix='hyjs-gather-overlay-', dir=output.parent) as directory:
        temp = Path(directory)
        refs, old, candidate = temp / 'refs', temp / 'baseline', temp / 'candidate'
        refs.mkdir(); old.mkdir(); candidate.mkdir()
        with zipfile.ZipFile(args.apk) as apk:
            for name in (*names, 'SoulFlowerDesertV4.dll'):
                (refs / name).write_bytes(apk.read('assets/bin/Data/Managed/' + name))
            dex_hash = hashlib.sha256()
            with apk.open('classes2.dex') as stream:
                for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                    dex_hash.update(chunk)
            if dex_hash.hexdigest() != DEX_SHA:
                raise ValueError('Pinned APK classes2.dex differs')
        if sha(refs / 'Assembly-CSharp.dll') != MAIN_SHA or sha(refs / 'SoulFlowerDesertV4.dll') != HELPER_SHA:
            raise ValueError('Pinned APK assemblies differ')
        for name in sources:
            (old / name).write_bytes(committed(BASE_COMMIT, name))
        (old / 'icon.png').write_bytes(committed(BASE_COMMIT, ICON))

        def compile_helper(target, source_dir, icon):
            run([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+',
                 '/out:' + str(target), '/resource:' + str(icon) + ',HYJS.DesertV4.Evacuation.png',
                 *['/reference:' + str(refs / name) for name in names], *[source_dir / name for name in sources]], env)

        baseline = old / 'SoulFlowerDesertV4.dll'
        compile_helper(baseline, old, old / 'icon.png')
        if sha(baseline) != HELPER_SHA:
            raise ValueError('1.1.4 baseline reconstruction is not byte-exact')
        print('PASS 1.1.4 helper reconstructed byte-for-byte:', HELPER_SHA)
        compiled = candidate / 'SoulFlowerDesertV4.dll'
        compile_helper(compiled, HERE, HERE / ICON)
        left, right = dnfile.dnPE(str(baseline)), dnfile.dnPE(str(compiled))
        before, after = public_abi(left), public_abi(right)
        removed, added = before - after, after - before
        if removed or added != ABI_ADDITIONS:
            raise ValueError('Unexpected public ABI: removed=%s added=%s' % (list(removed.elements()), list(added.elements())))
        if assembly_refs(left) != assembly_refs(right):
            raise ValueError('Assembly reference identities changed')
        if phase_values(left) != OLD_PHASES or phase_values(right) != dict(OLD_PHASES, CheckingClaim=7):
            raise ValueError('Persisted gather phase values changed')
        if sha(refs / 'Assembly-CSharp.dll') != MAIN_SHA:
            raise ValueError('Pinned host assembly was modified')
        print('PASS existing public ABI, assembly references and persisted phase values')

        output.mkdir()
        final = output / 'SoulFlowerDesertV4.dll'
        final.write_bytes(compiled.read_bytes())
        pins = {str((HERE / name).relative_to(ROOT)): sha(HERE / name) for name in sources}
        for name in (ICON, 'build_field_patch.py', 'build_boss_countdown_overlay.py', 'build_gather_status_overlay.py'):
            pins[str((HERE / name).relative_to(ROOT))] = sha(HERE / name)
        manifest = {
            'schema': 'hyjs-gather-status-helper-114-v1',
            'status': 'STATIC_VERIFIED_NOT_INSTALLED_NOT_DEVICE_TESTED',
            'baseAPKSHA256': APK_SHA, 'inputManagedSHA256': MAIN_SHA, 'inputDexSHA256': DEX_SHA,
            'managedOnly': True, 'managedAssemblyByteExact': True,
            'inheritedJavaByteExact': True, 'inheritedJavaPublicProtectedABIExact': True,
            'payloadFiles': {HELPER_ENTRY: str(final.relative_to(ROOT))},
            'replacementEntries': {HELPER_ENTRY: sha(final)}, 'sourcePins': pins,
            'baseCommit': BASE_COMMIT, 'candidateCommit': CANDIDATE_COMMIT,
            'apkSHA256': APK_SHA, 'assemblySHA256': MAIN_SHA,
            'shippedHelperSHA256': HELPER_SHA, 'reconstructedHelperSHA256': sha(baseline),
            'candidateHelperSHA256': sha(final), 'dotnetSHA256': sha(dotnet), 'compilerSHA256': sha(compiler),
            'resourceSHA256': sha(HERE / ICON), 'referencesSHA256': {name: sha(refs / name) for name in names},
            'sourceSHA256': {name: sha(HERE / name) for name in sources}, 'changedProductionSources': changed,
            'retainedPublicABIEntries': sum(before.values()),
            'allowedPublicABIAdditions': [list(entry) for entry in sorted(added.elements())],
            'gatherPhaseValues': phase_values(right), 'assemblyReferencesUnchanged': len(assembly_refs(left)),
            'deviceExecuted': False, 'apkBuilt': False, 'hostAssemblyWritten': False,
        }
        (output / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
        print('Candidate helper SHA-256:', manifest['candidateHelperSHA256'])
        print('Manifest:', output / 'manifest.json')
    print('Temporary build outputs cleaned')


if __name__ == '__main__':
    main()
