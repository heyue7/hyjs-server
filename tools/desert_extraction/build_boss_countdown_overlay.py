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


class CliSignatures:
    """Normalize build-local TypeDefOrRef tokens to named managed types."""

    SIMPLE = {1: 'void', 2: 'bool', 3: 'char', 4: 'i1', 5: 'u1', 6: 'i2', 7: 'u2',
              8: 'i4', 9: 'u4', 10: 'i8', 11: 'u8', 12: 'r4', 13: 'r8', 14: 'string',
              22: 'typedbyref', 24: 'nativeint', 25: 'nativeuint', 28: 'object'}

    def __init__(self, pe):
        self.pe = pe
        self.parents = {row.NestedClass.row_index: row.EnclosingClass.row_index
                        for row in (pe.net.mdtables.NestedClass.rows if pe.net.mdtables.NestedClass else [])}

    @staticmethod
    def uint(raw, pos):
        first = raw[pos]
        if first < 0x80:
            return first, pos + 1
        if first < 0xc0:
            return ((first & 0x3f) << 8) | raw[pos + 1], pos + 2
        if first < 0xe0:
            return ((first & 0x1f) << 24) | (raw[pos + 1] << 16) | (raw[pos + 2] << 8) | raw[pos + 3], pos + 4
        raise ValueError('Invalid compressed signature integer')

    def definition(self, rid):
        row = self.pe.net.mdtables.TypeDef.rows[rid - 1]
        if rid in self.parents:
            return self.definition(self.parents[rid]) + '+' + str(row.TypeName)
        return '.'.join(filter(None, [str(row.TypeNamespace), str(row.TypeName)]))

    def reference(self, rid):
        row = self.pe.net.mdtables.TypeRef.rows[rid - 1]
        scope = row.ResolutionScope.row
        if scope is not None and hasattr(scope, 'TypeName'):
            return self.reference(row.ResolutionScope.row_index) + '+' + str(row.TypeName)
        assembly = str(scope.Name) if scope is not None and hasattr(scope, 'Name') else ''
        return '[' + assembly + ']' + '.'.join(filter(None, [str(row.TypeNamespace), str(row.TypeName)]))

    def coded(self, value):
        kind, rid = value & 3, value >> 2
        if not rid:
            raise ValueError('Null TypeDefOrRef in public ABI')
        if kind == 0:
            return 'def:' + self.definition(rid)
        if kind == 1:
            return 'ref:' + self.reference(rid)
        if kind == 2:
            raw = self.pe.net.mdtables.TypeSpec.rows[rid - 1].Signature.value
            value, end = self.typ(raw, 0)
            if end != len(raw):
                raise ValueError('Trailing TypeSpec signature')
            return 'spec:' + value
        raise ValueError('Invalid TypeDefOrRef tag')

    def typ(self, raw, pos):
        kind = raw[pos]
        pos += 1
        if kind in self.SIMPLE:
            return self.SIMPLE[kind], pos
        if kind in (0x0f, 0x10, 0x1d, 0x45):
            nested, pos = self.typ(raw, pos)
            return {0x0f: 'ptr', 0x10: 'byref', 0x1d: 'array', 0x45: 'pinned'}[kind] + '(' + nested + ')', pos
        if kind in (0x11, 0x12):
            value, pos = self.uint(raw, pos)
            return ('valuetype:' if kind == 0x11 else 'class:') + self.coded(value), pos
        if kind in (0x13, 0x1e):
            value, pos = self.uint(raw, pos)
            return ('var:' if kind == 0x13 else 'mvar:') + str(value), pos
        if kind in (0x1f, 0x20):
            value, pos = self.uint(raw, pos)
            nested, pos = self.typ(raw, pos)
            return ('modreq:' if kind == 0x1f else 'modopt:') + self.coded(value) + '(' + nested + ')', pos
        if kind == 0x15:
            base, pos = self.typ(raw, pos)
            count, pos = self.uint(raw, pos)
            args = []
            for _ in range(count):
                item, pos = self.typ(raw, pos)
                args.append(item)
            return 'generic:' + base + '<' + ','.join(args) + '>', pos
        if kind == 0x14:
            nested, pos = self.typ(raw, pos)
            rank, pos = self.uint(raw, pos)
            sizes_count, pos = self.uint(raw, pos)
            sizes = []
            for _ in range(sizes_count):
                value, pos = self.uint(raw, pos)
                sizes.append(value)
            bounds_count, pos = self.uint(raw, pos)
            bounds = []
            for _ in range(bounds_count):
                value, pos = self.uint(raw, pos)
                bounds.append(value)
            return 'mdarray:%s:%s:%s:%s' % (nested, rank, sizes, bounds), pos
        if kind == 0x1b:
            nested, pos = self.method(raw, pos)
            return 'fnptr:' + nested, pos
        raise ValueError('Unsupported public ABI element 0x%02x' % kind)

    def method(self, raw, pos=0):
        flags = raw[pos]
        pos += 1
        generic = 0
        if flags & 0x10:
            generic, pos = self.uint(raw, pos)
        count, pos = self.uint(raw, pos)
        result, pos = self.typ(raw, pos)
        params = []
        while len([p for p in params if p != 'sentinel']) < count:
            if raw[pos] == 0x41:
                params.append('sentinel')
                pos += 1
            else:
                value, pos = self.typ(raw, pos)
                params.append(value)
        return '%d/%d:%s(%s)' % (flags, generic, result, ','.join(params)), pos

    def signature(self, raw, field=False):
        if field:
            if raw[0] != 0x06:
                raise ValueError('Invalid field signature')
            result, end = self.typ(raw, 1)
        else:
            result, end = self.method(raw)
        if end != len(raw):
            raise ValueError('Trailing public ABI signature')
        return result


def public_abi(pe):
    entries = Counter()
    signatures = CliSignatures(pe)
    for rid, owner in enumerate(pe.net.mdtables.TypeDef.rows, 1):
        if not (owner.Flags.tdPublic or owner.Flags.tdNestedPublic):
            continue
        name = signatures.definition(rid)
        entries[('type', name, owner.struct.Flags)] += 1
        for item in owner.MethodList:
            row = item.row
            if row.Flags.mdPublic:
                entries[('method', name, str(row.Name), signatures.signature(row.Signature.value), row.struct.Flags, row.struct.ImplFlags)] += 1
        for item in owner.FieldList:
            row = item.row
            if row.Flags.fdPublic:
                entries[('field', name, str(row.Name), signatures.signature(row.Signature.value, field=True), row.struct.Flags)] += 1
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
        if set(changed) != {'FieldBoss.cs', 'MapViewer86.cs', 'FieldRuntime.cs', 'PositionSync.cs', 'LeaveController.cs'}:
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
