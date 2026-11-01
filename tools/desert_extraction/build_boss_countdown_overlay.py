"""Build only a pinned APK 1.1.2 desert helper after proving the baseline."""
import argparse
import ast
from collections import Counter
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

import dnfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
BASE_COMMIT = '9cd746933377b7861e08e8beaf579eb69a70e78a'
APK_SHA = '408e9d03291d6f3505fb91590134b25a8b6aa68f3ebde9c2f4ff985299baccb3'
MAIN_SHA = '3d8f50e543ad082ea03bff294f17421a2eed4316d5cda664b03cc0f46cc23e8a'
HELPER_SHA = 'ff787ae0079907b53055f1e449fc589da674282c4abad221e00c8ecac02363f3'
ICON = 'assets/evacuation-npc.png'


def sha(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def baseline(path):
    result = subprocess.run(['rtk', 'proxy', 'git', 'show', BASE_COMMIT + ':tools/desert_extraction/' + path],
                            cwd=ROOT, capture_output=True, timeout=30)
    if result.returncode:
        raise RuntimeError('Unable to read fixed baseline source: ' + path)
    return result.stdout


def run(command, env):
    result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, env=env,
                            text=True, capture_output=True, timeout=90)
    if result.stdout:
        print(result.stdout, end='')
    if result.stderr:
        print(result.stderr, end='')
    if result.returncode:
        raise RuntimeError('Helper compile failed: %s' % result.returncode)


def source_list(raw):
    tree = ast.parse(raw)
    return next(ast.literal_eval(node.value) for node in tree.body
                if isinstance(node, ast.Assign) and any(
                    isinstance(target, ast.Name) and target.id == 'SOURCES' for target in node.targets))


def public_abi(pe):
    entries = Counter()
    for owner in pe.net.mdtables.TypeDef.rows:
        if not (owner.Flags.tdPublic or owner.Flags.tdNestedPublic):
            continue
        name = str(owner.TypeNamespace) + '.' + str(owner.TypeName)
        entries[('type', name, owner.struct.Flags)] += 1
        for item in owner.MethodList:
            row = item.row
            if row.Flags.mdPublic:
                entries[('method', name, str(row.Name), row.Signature.value.hex(), row.struct.Flags)] += 1
        for item in owner.FieldList:
            row = item.row
            if row.Flags.fdPublic:
                entries[('field', name, str(row.Name), row.Signature.value.hex(), row.struct.Flags)] += 1
    return entries


def assembly_refs(pe):
    return [(str(row.Name), row.MajorVersion, row.MinorVersion, row.BuildNumber, row.RevisionNumber,
             row.PublicKey.value.hex() if row.PublicKey else '') for row in pe.net.mdtables.AssemblyRef.rows]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apk', type=Path, required=True)
    parser.add_argument('--runtime', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if sha(args.apk) != APK_SHA:
        raise ValueError('Expected pinned APK 1.1.2')
    sources = source_list((HERE / 'build_field_patch.py').read_text())
    if sources != source_list(baseline('build_field_patch.py').decode()):
        raise ValueError('Production source list changed from pinned baseline')
    output = args.output.resolve()
    if output.exists():
        raise ValueError('Output must be a fresh path')
    rt = args.runtime / 'dotnet'
    dotnet = rt / 'dotnet'
    compiler = next((rt / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    env = dict(os.environ, DOTNET_PROCESSOR_COUNT='2', DOTNET_CLI_TELEMETRY_OPTOUT='1')
    with tempfile.TemporaryDirectory(prefix='hyjs-desert-boss-overlay-', dir=ROOT / 'build') as directory:
        temp = Path(directory)
        refs = temp / 'refs'
        old = temp / 'baseline'
        candidate = temp / 'candidate'
        refs.mkdir(); old.mkdir(); candidate.mkdir()
        names = ('Assembly-CSharp.dll', 'Assembly-CSharp-firstpass.dll', 'UnityEngine.dll', 'mscorlib.dll',
                 'System.dll', 'System.Core.dll', 'LitJson.dll', 'HYJS.Field63.dll', 'LocalFieldSettlement.dll')
        with zipfile.ZipFile(args.apk) as apk:
            for name in (*names, 'SoulFlowerDesertV4.dll'):
                (refs / name).write_bytes(apk.read('assets/bin/Data/Managed/' + name))
        if sha(refs / 'Assembly-CSharp.dll') != MAIN_SHA or sha(refs / 'SoulFlowerDesertV4.dll') != HELPER_SHA:
            raise ValueError('Pinned APK assembly differs')
        for name in sources:
            (old / name).write_bytes(baseline(name))
        changed = [name for name in sources if (old / name).read_bytes() != (HERE / name).read_bytes()]
        if set(changed) != {'FieldBoss.cs', 'MapViewer86.cs'}:
            raise ValueError('Unexpected production source changes: %s' % changed)
        (old / 'icon.png').write_bytes(baseline(ICON))
        if (HERE / ICON).read_bytes() != (old / 'icon.png').read_bytes():
            raise ValueError('Candidate resource differs from fixed baseline')
        def compile_helper(target, srcdir, icon):
            run([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+',
                 '/out:' + str(target), '/resource:' + str(icon) + ',HYJS.DesertV4.Evacuation.png',
                 *['/reference:' + str(refs / name) for name in names], *[srcdir / name for name in sources]], env)
        baseline_helper = old / 'SoulFlowerDesertV4.dll'
        compile_helper(baseline_helper, old, old / 'icon.png')
        reconstructed = sha(baseline_helper)
        print('Rebuilt baseline SHA-256:', reconstructed)
        if reconstructed != HELPER_SHA:
            raise ValueError('Baseline does not reproduce shipped helper; refusing overlay')
        compiled = candidate / 'SoulFlowerDesertV4.dll'
        compile_helper(compiled, HERE, HERE / ICON)
        left, right = dnfile.dnPE(str(baseline_helper)), dnfile.dnPE(str(compiled))
        before, after = public_abi(left), public_abi(right)
        if before != after:
            raise ValueError('Public ABI changed: removed=%s added=%s' % (list((before-after).elements()), list((after-before).elements())))
        if assembly_refs(left) != assembly_refs(right):
            raise ValueError('Assembly reference identities changed')
        host = dnfile.dnPE(str(refs / 'Assembly-CSharp.dll'))
        engine = sum(1 for typ in host.net.mdtables.TypeDef.rows
                     if (str(typ.TypeNamespace) + '.' + str(typ.TypeName)).startswith('Battle.Engine')
                     for item in typ.MethodList if item.row.Rva)
        if engine != 547:
            raise ValueError('Fixed host has unexpected Battle.Engine method count')
        if sha(refs / 'Assembly-CSharp.dll') != MAIN_SHA:
            raise ValueError('Pinned host assembly was modified')
        output.mkdir(parents=True)
        final = output / 'SoulFlowerDesertV4.dll'
        final.write_bytes(compiled.read_bytes())
        manifest = {'baseCommit': BASE_COMMIT, 'apkSHA256': APK_SHA, 'assemblySHA256': MAIN_SHA,
                    'shippedHelperSHA256': HELPER_SHA, 'reconstructedHelperSHA256': reconstructed,
                    'candidateHelperSHA256': sha(final), 'dotnetSHA256': sha(dotnet), 'compilerSHA256': sha(compiler),
                    'resourceSHA256': sha(HERE / ICON), 'referencesSHA256': {name: sha(refs / name) for name in names},
                    'sourceSHA256': {name: sha(HERE / name) for name in sources},
                    'changedProductionSources': changed, 'publicABIEntriesUnchanged': sum(before.values()),
                    'assemblyReferencesUnchanged': len(assembly_refs(left)), 'hostBattleEngineBodiesUnchanged': engine,
                    'deviceExecuted': False, 'apkBuilt': False, 'hostAssemblyWritten': False}
        (output / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
        print('Candidate helper SHA-256:', manifest['candidateHelperSHA256'])
        print('Manifest:', output / 'manifest.json')


if __name__ == '__main__':
    main()
