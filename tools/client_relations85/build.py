"""Apply a pinned managed-only relation-page patch to an explicitly supplied main."""
import argparse
import hashlib
import json
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

sys.dont_write_bytecode = True
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.client_achievements.build import normalize
from tools.local_asset_ready.remove_gate import exact_body, require

HERE = Path(__file__).resolve().parent
PINS = json.loads((HERE / 'target-pins.json').read_text())
PROTECTED = {
    'UI.UILuck::SetCenterHero', 'UI.UILuck::OnCoverPageRemove',
    'UI.UILuckEvent::OpenInBattlePage', 'UI.HeroReplacePage::OnHelpBattle',
    'UI.UILuckEvent::InBattlePressPosOne', 'UI.UILuckEvent::InBattlePressPosTwo',
    'UI.UILuckEvent::InBattlePressPosThree', 'UI.UILuckEvent::InBattlePressPosFour',
    'UI.UILuckEvent::InBattlePressPosFive',
}


def sha(data):
    return hashlib.sha256(data).hexdigest()


def file_sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def assembly_identity(row):
    return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion,
            row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)


def verify(before, after):
    left, right = [dnfile.dnPE(data=x) for x in (before, after)]
    for table in ('TypeDef', 'Field', 'MethodDef', 'Param', 'AssemblyRef', 'TypeRef', 'MemberRef'):
        require(len(getattr(left.net.mdtables, table).rows) == len(getattr(right.net.mdtables, table).rows),
                'metadata count changed: ' + table)
    for table in ('FieldPtr', 'MethodPtr'):
        require(not getattr(right.net.mdtables, table, None), 'unexpected pointer table: ' + table)
    require(left.net.mdtables.Module.rows[0].Mvid.value == right.net.mdtables.Module.rows[0].Mvid.value,
            'module identity changed')
    for table in ('Assembly', 'AssemblyRef'):
        require([assembly_identity(x) for x in getattr(left.net.mdtables, table).rows] ==
                [assembly_identity(x) for x in getattr(right.net.mdtables, table).rows],
                'assembly identity changed: ' + table)
    require(left.net.struct.Flags == right.net.struct.Flags and
            left.net.struct.StrongNameSignatureSize == right.net.struct.StrongNameSignatureSize == 0,
            'CLI identity changed')
    for x, y in zip(left.net.mdtables.Field.rows, right.net.mdtables.Field.rows):
        require((str(x.Name), x.Signature.value, x.struct.Flags) ==
                (str(y.Name), y.Signature.value, y.struct.Flags), 'field ABI changed')
    for x, y in zip(left.net.mdtables.TypeDef.rows, right.net.mdtables.TypeDef.rows):
        require((str(x.TypeName), str(x.TypeNamespace), x.struct.Flags, x.struct.Extends_CodedIndex,
                 x.struct.FieldList_Index, x.struct.MethodList_Index) ==
                (str(y.TypeName), str(y.TypeNamespace), y.struct.Flags, y.struct.Extends_CodedIndex,
                 y.struct.FieldList_Index, y.struct.MethodList_Index), 'type ABI changed')
    for table, attrs in (('TypeRef', ('TypeName', 'TypeNamespace')), ('MemberRef', ('Name', 'Signature'))):
        for x, y in zip(getattr(left.net.mdtables, table).rows, getattr(right.net.mdtables, table).rows):
            for attr in attrs:
                a, b = getattr(x, attr), getattr(y, attr)
                require((a.value if hasattr(a, 'value') else str(a)) ==
                        (b.value if hasattr(b, 'value') else str(b)), 'reference changed: ' + table + '.' + attr)
            a = x.ResolutionScope if table == 'TypeRef' else x.Class
            b = y.ResolutionScope if table == 'TypeRef' else y.Class
            require((a.table.name, a.row_index) == (b.table.name, b.row_index), 'reference scope changed')
    old, new = methods(left), methods(right)
    require(len(old) == len(new), 'method inventory changed')
    changes, protected, unchanged, engine = [], [], 0, 0
    for (name, rid, a), (new_name, new_rid, b) in zip(old, new):
        require((name, rid, a.Signature.value, a.struct.Flags, a.struct.ImplFlags) ==
                (new_name, new_rid, b.Signature.value, b.struct.Flags, b.struct.ImplFlags), 'method ABI changed: ' + name)
        if not a.Rva:
            require(not b.Rva, 'unexpected new body: ' + name)
            continue
        x, y = exact_body(left, a), exact_body(right, b)
        if name in PINS:
            require(sha(x) == PINS[name] and x != y, 'target input differs or target unchanged: ' + name)
            before_body = read_method_body_from_bytes(x)
            after_body = read_method_body_from_bytes(y)
            expected_eh = 0 if name.startswith('<SetInBattleHeroIcon>') else len(before_body.exception_handlers)
            require(len(after_body.exception_handlers) == expected_eh, 'unexpected target exception-region count: ' + name)
            if name == 'UI.UILuckEvent::OpenHelpPage':
                _, old_code = decoded(left, a)
                _, new_code = decoded(right, b)
                prefix = len(new_code) - len(old_code)
                require(prefix > 0 and normalize(new_code, {i: i-prefix for i in range(prefix, len(new_code))}) == old_code,
                        'original help unlock and page-opening flow differs')
            changes.append({'method': name, 'token': hex(0x06000000 | rid),
                            'inputBodySHA256': sha(x), 'outputBodySHA256': sha(y),
                            'inputExceptionRegions': len(before_body.exception_handlers),
                            'outputExceptionRegions': len(after_body.exception_handlers)})
        else:
            require(x == y, 'unrelated raw method changed: ' + name)
            unchanged += 1
            if name in PROTECTED:
                protected.append({'method': name, 'bodySHA256': sha(x)})
        if name.startswith('Battle.Engine'):
            require(x == y, 'battle engine changed: ' + name)
            engine += 1
    require({x['method'] for x in changes} == set(PINS) and {x['method'] for x in protected} == PROTECTED and engine == 547,
            'target, protected method or engine inventory differs')
    return {'allowedChangedMethods': changes, 'allOtherRawBodiesExact': True,
            'unchangedBodyCount': unchanged, 'originalEngineMethodsExact': engine,
            'originalMethodTokensAndDefinitionABIExact': True, 'originalFieldsAndTypeABIExact': True,
            'originalAssemblyAndReferencesExact': True, 'inheritedTypeAndMemberReferencesExact': True,
            'originalHelpEligibilitySuffixExact': True,
            'protectedOriginalMethods': protected, 'newRuntimeAssembly': False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input-managed', required=True, type=Path)
    parser.add_argument('--input-managed-sha256', required=True)
    parser.add_argument('--output-directory', required=True, type=Path)
    parser.add_argument('--tool-root', default='/Users/lirui/LR/hyjs', type=Path)
    args = parser.parse_args()
    source, out = args.input_managed.resolve(), args.output_directory.resolve()
    require(re.fullmatch('[0-9a-fA-F]{64}', args.input_managed_sha256) is not None, 'input SHA must be 64 hex digits')
    before = source.read_bytes()
    require(sha(before) == args.input_managed_sha256.lower(), 'input main SHA differs')
    pe = dnfile.dnPE(data=before)
    found = {name: sha(exact_body(pe, row)) for name, _, row in methods(pe) if name in PINS}
    require(found == PINS, 'target method pins differ or patch already applied')
    require(out.is_relative_to(ROOT / 'analysis/fragment-box85/relations') and not out.exists(),
            'fresh task-owned output required')
    runtime = args.tool_root.resolve() / '.tools/native-batch'
    dotnet, dnlib = runtime / 'dotnet/dotnet', runtime / 'injector/bin/Debug/net8.0/dnlib.dll'
    require(dotnet.is_file() and dnlib.is_file(), 'local SDK/dnlib missing')
    sources = [HERE / name for name in ('build.py', 'Injector.cs', 'target-pins.json')]
    source_pins = {str(p.relative_to(ROOT)): file_sha(p) for p in sources}
    out.mkdir(parents=True)
    with tempfile.TemporaryDirectory(prefix='hyjs-relations85-') as tmp:
        work = Path(tmp)
        shutil.copy2(HERE / 'Injector.cs', work / 'Program.cs')
        project = work / 'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>' +
                           str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        for label, command in (
            ('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false']),
            ('inject', [dotnet, work / 'bin/Debug/net8.0/Patch.dll', source, out / 'Assembly-CSharp.dll'])
        ):
            result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT,
                                    capture_output=True, text=True, timeout=120)
            (out / (label + '.log')).write_text(result.stdout + result.stderr)
            require(result.returncode == 0, label + ' failed; inspect log')
    output = out / 'Assembly-CSharp.dll'
    checks = verify(before, output.read_bytes())
    require(file_sha(source) == sha(before), 'input main changed during build')
    require({str(p.relative_to(ROOT)): file_sha(p) for p in sources} == source_pins,
            'build source changed during build')
    manifest = {'schema': 'hyjs-relations85-v1', 'status': 'STATIC_VERIFIED_NOT_PACKAGED',
                'inputManagedSHA256': sha(before), 'outputManagedSHA256': file_sha(output),
                'payloadFiles': {'assets/bin/Data/Managed/Assembly-CSharp.dll': str(output)},
                'sourcePins': source_pins,
                'deviceTested': False, 'apkBuilt': False, **checks}
    (out / 'manifest.json').write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + '\n')
    print(json.dumps({k: manifest[k] for k in ('status', 'inputManagedSHA256', 'outputManagedSHA256',
                                               'unchangedBodyCount', 'originalEngineMethodsExact')}))


if __name__ == '__main__':
    main()
