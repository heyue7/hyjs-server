"""Apply the native fragment-choice quantity overlay to an explicitly pinned main."""
import argparse
import csv
import hashlib
import io
import json
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

sys.dont_write_bytecode = True
import dnfile
import UnityPy

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.awake_diff_rows.build import methods
from tools.client_activity.build import decoded
from tools.client_achievements.build import normalize
from tools.local_asset_ready.remove_gate import exact_body, require

HERE = Path(__file__).resolve().parent
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER_ENTRY = 'assets/bin/Data/Managed/HYJS.FragmentBoxes85.dll'
APK_SHA = '25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
MAIN80_SHA = '8b732cce869cad9a9a06c75d3ba4891b4084a3ac646848fc99a933148e300437'
TABLE_SHA = {'Item': '3b426085b94332bb40be7e0921f13efb9da822a063af68aa1ab5da68258b6088',
             'ItemUse': '7e25fe7a4eef44dab17a661a2b936e5b55985428cf8878114bc238d9644e317d'}
PINS = json.loads((HERE/'target-pins.json').read_text())


def sha(data):
    return hashlib.sha256(data).hexdigest()


def file_sha(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def field_token(pe, token, name):
    require(token >> 24 == 4 and str(pe.net.mdtables.Field.rows[(token & 0xffffff)-1].Name) == name,
            'Unexpected field: ' + name)


def bridge_call(pe, token, name):
    require(token >> 24 == 10, 'Expected helper MemberRef: ' + name)
    ref = pe.net.mdtables.MemberRef.rows[(token & 0xffffff)-1]
    owner = ref.Class.row
    require(str(ref.Name) == name and str(owner.TypeNamespace) == 'HYJS.FragmentBoxes85' and
            str(owner.TypeName) == 'FragmentBoxes' and
            owner.ResolutionScope.table.name == 'AssemblyRef' and
            str(owner.ResolutionScope.row.Name) == 'HYJS.FragmentBoxes85', 'Unexpected helper call: ' + name)


def assembly_identity(row):
    return (str(row.Name), str(row.Culture), row.struct.MajorVersion, row.struct.MinorVersion,
            row.struct.BuildNumber, row.struct.RevisionNumber, row.struct.Flags, row.PublicKey.value)


def verify(before, after):
    a, b = [dnfile.dnPE(data=raw) for raw in (before, after)]
    for table in ('TypeDef', 'Field', 'MethodDef', 'Param'):
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows),
                'Definition inventory changed: ' + table)
    for table in ('FieldPtr', 'MethodPtr'):
        require(not getattr(b.net.mdtables, table, None), 'Unexpected Unity pointer table: ' + table)
    require(a.net.mdtables.Module.rows[0].Mvid.value == b.net.mdtables.Module.rows[0].Mvid.value,
            'Module identity changed')
    require([assembly_identity(x) for x in a.net.mdtables.Assembly.rows] ==
            [assembly_identity(x) for x in b.net.mdtables.Assembly.rows], 'Assembly identity changed')
    old_refs, new_refs = a.net.mdtables.AssemblyRef.rows, b.net.mdtables.AssemblyRef.rows
    require([assembly_identity(x) for x in old_refs] == [assembly_identity(x) for x in new_refs[:len(old_refs)]] and
            len(new_refs) == len(old_refs)+1 and str(new_refs[-1].Name) == 'HYJS.FragmentBoxes85' and
            new_refs[-1].struct.Flags == 0 and new_refs[-1].PublicKey.value == b'', 'Assembly references changed')
    require(a.net.struct.Flags == b.net.struct.Flags and
            a.net.struct.StrongNameSignatureSize == b.net.struct.StrongNameSignatureSize == 0, 'CLI identity changed')
    for x, y in zip(a.net.mdtables.Field.rows, b.net.mdtables.Field.rows):
        require((str(x.Name), x.Signature.value, x.struct.Flags) ==
                (str(y.Name), y.Signature.value, y.struct.Flags), 'Field ABI changed')
    for x, y in zip(a.net.mdtables.TypeDef.rows, b.net.mdtables.TypeDef.rows):
        require((str(x.TypeName), str(x.TypeNamespace), x.struct.Flags, x.struct.Extends_CodedIndex,
                 x.struct.FieldList_Index, x.struct.MethodList_Index) ==
                (str(y.TypeName), str(y.TypeNamespace), y.struct.Flags, y.struct.Extends_CodedIndex,
                 y.struct.FieldList_Index, y.struct.MethodList_Index), 'Type ABI changed')
    for table, attrs in (('TypeRef', ('TypeName', 'TypeNamespace')),
                         ('MemberRef', ('Name', 'Signature'))):
        old_table = getattr(a.net.mdtables, table).rows
        new_table = getattr(b.net.mdtables, table).rows
        require(len(new_table) >= len(old_table), 'Inherited references removed: ' + table)
        for x, y in zip(old_table, new_table):
            for attr in attrs:
                first, second = getattr(x, attr), getattr(y, attr)
                require((first.value if hasattr(first, 'value') else str(first)) ==
                        (second.value if hasattr(second, 'value') else str(second)),
                        'Inherited reference differs: ' + table + '.' + attr)
            first = x.ResolutionScope if table == 'TypeRef' else x.Class
            second = y.ResolutionScope if table == 'TypeRef' else y.Class
            require((first.table.name, first.row_index) == (second.table.name, second.row_index),
                    'Inherited reference scope differs: ' + table)
    left, right = methods(a), methods(b)
    require(len(left) == len(right), 'Method inventory changed')
    changed, unchanged, engine = [], 0, 0
    for (name, rid, old), (name2, rid2, new) in zip(left, right):
        require((name, rid, old.Signature.value, old.struct.Flags, old.struct.ImplFlags) ==
                (name2, rid2, new.Signature.value, new.struct.Flags, new.struct.ImplFlags), 'Method ABI changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'New method body: ' + name)
            continue
        x, y = exact_body(a, old), exact_body(b, new)
        if name not in PINS:
            require(x == y, 'Unrelated raw method changed: ' + name)
            unchanged += 1
            if name.startswith('Battle.Engine'):
                engine += 1
            continue
        require(sha(x) == PINS[name], 'Pinned original target changed: ' + name)
        oldbody, oc = decoded(a, old)
        newbody, nc = decoded(b, new)
        require(not oldbody.exception_handlers and not newbody.exception_handlers, 'Target exception regions changed: ' + name)
        if name == 'UI.BagMemberDetail::SetItem':
            require(len(nc) == len(oc)+18 and oc[-1] == ('ret', None), 'SetItem size differs')
            tail = nc[len(oc)-1:]
            require([op for op, value in tail] == [
                'ldarg.1', 'call', 'brfalse', 'ldarg.0', 'ldc.i4.1', 'ldc.i4.0', 'ldc.i4.0',
                'ldc.i4.1', 'ldc.i4.1', 'ldstr', 'ldc.i4.1', 'ldc.i4.0', 'call', 'ldarg.0',
                'ldarg.0', 'ldfld', 'call', 'stfld', 'ret'], 'SetItem patch shape differs')
            bridge_call(b, tail[1][1], 'IsEligible')
            require(tail[2][1] == len(nc)-1 and tail[9][1] == 'UI_beibao_word_shiyong' and
                    tail[12][1] == oc[116][1], 'Original quantity UI call differs')
            field_token(b, tail[15][1], 'totalHaveNum')
            require(tail[15][1] == tail[17][1] == oc[191][1], 'Stock bound field differs')
            bridge_call(b, tail[16][1], 'Limit')
            mapping = {i: i for i in range(len(oc))}
            require(normalize(nc, mapping)[:-1] == oc[:-1], 'SetItem original flow differs')
        elif name == 'UI.BagMemberDetail::OnPublicButtonClick':
            at = 57
            require(len(nc) == len(oc)+7 and oc[at][0] == 'callvirt', 'Open call position differs')
            tail = nc[at:at+7]
            require([op for op, value in tail] == ['ldarg.0', 'ldfld', 'ldarg.0', 'ldfld', 'ldfld', 'ldfld', 'call'],
                    'Options insertion differs')
            require([tail[i][1] for i in (1, 3, 4, 5)] == [oc[i][1] for i in (27, 53, 54, 55)],
                    'Options item or quantity fields differ')
            bridge_call(b, tail[-1][1], 'AppendCount')
            mapping = {i if i < at else i+7: i for i in range(len(oc))}
            require(normalize(nc, mapping) == oc, 'Original open flow differs')
        else:
            at = 16
            require(len(nc) == len(oc)+3 and oc[at] == ('ldc.i4.1', None), 'Confirm count position differs')
            tail = nc[at:at+4]
            require([op for op, value in tail] == ['ldarg.0', 'ldarg.0', 'ldfld', 'call'] and
                    tail[2][1] == oc[13][1], 'Confirm page or item differs')
            bridge_call(b, tail[3][1], 'RequestedCount')
            mapping = {i if i < at else i+3: i for i in range(len(oc)) if i != at}
            require(normalize(nc, mapping) == oc[:at] + oc[at+1:], 'Original validation/send flow differs')
        changed.append({'method': name, 'token': hex(0x06000000 | rid),
                        'inputBodySHA256': sha(x), 'outputBodySHA256': sha(y)})
    require({x['method'] for x in changed} == set(PINS) and engine == 547, 'Target/engine inventory differs')
    return {'allowedChangedMethods': changed, 'allOtherRawBodiesExact': True,
            'unchangedBodyCount': unchanged, 'originalEngineMethodsExact': engine,
            'originalMethodTokensAndDefinitionABIExact': True, 'originalFieldsAndTypeABIExact': True,
            'inheritedTypeAndMemberReferencesExact': True,
            'addedAssemblyReference': 'HYJS.FragmentBoxes85', 'helperHostTypeBindingCheckedByInjector': True}


def table_text(archive, name):
    raw = archive.read('assets/android/tabledata/table' + name.lower())
    entries = [obj.read() for obj in UnityPy.load(raw).objects if obj.type.name == 'TextAsset']
    require(len(entries) == 1, 'Table asset inventory differs: ' + name)
    data = entries[0].m_Script
    data = data.encode('utf-8', 'surrogateescape') if isinstance(data, str) else data
    require(sha(data) == TABLE_SHA[name], 'Reference table differs: ' + name)
    return data


def table_rows(raw):
    rows = list(csv.reader(io.StringIO(raw.decode('utf-8-sig')), delimiter='\t'))
    return [dict(zip(rows[1], row)) for row in rows[5:] if row and row[0]]


def catalogue(item, uses):
    groups = {}
    for row in table_rows(uses):
        groups.setdefault(row['itemId'], []).append(row)
    output = {}
    for row in table_rows(item):
        pool = groups.get(row['itemId'], [])
        def compatible(choice):
            parts = choice['result'].split(',')
            return (len(parts) == 2 and parts[0].startswith('frag_') and len(parts[0]) > 5 and
                    re.fullmatch(r'[0-9]+', parts[1]) is not None and 0 < int(parts[1]) <= 2147483647)
        if row['category'] == '5' and pool and all(compatible(choice) for choice in pool):
            output[row['itemId']] = len(pool)
    require(output == {'item_120': 3, 'item_121': 3, 'item_122': 12, 'item_156': 3, 'item_348': 3},
            'Reference fragment choice catalogue differs')
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', required=True, type=Path)
    parser.add_argument('--input-managed', required=True, type=Path)
    parser.add_argument('--input-managed-sha256', required=True)
    parser.add_argument('--output-directory', required=True, type=Path)
    parser.add_argument('--tool-root', type=Path, help='Workspace containing .tools/native-batch; discovered from parents if omitted')
    args = parser.parse_args()
    apk, input_path, out = args.base_apk.resolve(), args.input_managed.resolve(), args.output_directory.resolve()
    require(file_sha(apk) == APK_SHA, 'Pinned 80 APK differs')
    before = input_path.read_bytes()
    require(re.fullmatch('[0-9a-fA-F]{64}', args.input_managed_sha256) is not None and
            sha(before) == args.input_managed_sha256.lower(), 'Pinned input main differs')
    pe = dnfile.dnPE(data=before)
    found = {name: sha(exact_body(pe, row)) for name, _, row in methods(pe) if name in PINS}
    require(found == PINS, 'Target method pins differ or already patched')
    require(not out.exists(), 'Fresh output directory required')
    out.mkdir(parents=True)
    if args.tool_root:
        tools_root = args.tool_root.resolve()
    else:
        tools_root = next((path for path in (ROOT, *ROOT.parents)
                           if (path/'.tools/native-batch/dotnet/dotnet').is_file()), None)
    require(tools_root is not None, 'Native batch SDK root not found; specify --tool-root')
    runtime = tools_root/'.tools/native-batch'
    dotnet = runtime/'dotnet/dotnet'
    compiler = next((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    dnlib = runtime/'injector/bin/Debug/net8.0/dnlib.dll'

    def run(label, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT,
                                capture_output=True, text=True, timeout=120)
        (out/(label+'.log')).write_text(result.stdout+result.stderr)
        require(result.returncode == 0, label+' failed; inspect log')

    with zipfile.ZipFile(apk) as archive, tempfile.TemporaryDirectory(prefix='hyjs-fragment-box85-') as tmp:
        require(archive.namelist().count(ENTRY) == 1 and sha(archive.read(ENTRY)) == MAIN80_SHA,
                '80 APK main differs')
        item, uses = (table_text(archive, name) for name in ('Item', 'ItemUse'))
        eligible = catalogue(item, uses)
        work = Path(tmp)
        host = work/'Assembly-CSharp.dll'
        host.write_bytes(before)
        references = [host]
        for name in ('UnityEngine.dll', 'mscorlib.dll', 'Assembly-CSharp-firstpass.dll', 'System.dll', 'System.Core.dll'):
            target = work/name
            target.write_bytes(archive.read('assets/bin/Data/Managed/'+name))
            references.append(target)
        helper = out/'HYJS.FragmentBoxes85.dll'
        run('compile-helper', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
                               '/deterministic+', '/out:'+str(helper),
                               *['/reference:'+str(p) for p in references], HERE/'FragmentBoxes.cs'])
        shutil.copy2(HERE/'Injector.cs', work/'Program.cs')
        project = work/'Patch.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'+
                           str(dnlib)+'</HintPath></Reference></ItemGroup></Project>')
        run('compile-injector', [dotnet, 'build', project, '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'])
        output = out/'Assembly-CSharp.dll'
        run('inject', [dotnet, work/'bin/Debug/net8.0/Patch.dll', host, helper, output])
        consumer = work/'Consumer.cs'
        consumer.write_text('public static class Consumer { public static int Probe(UI.Page page) '
                            '{ return HYJS.FragmentBoxes85.FragmentBoxes.RequestedCount(page, "item_120"); } }')
        run('compile-consumer', [dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
                                 '/out:'+str(work/'Consumer.dll'), '/reference:'+str(output), '/reference:'+str(helper),
                                 *['/reference:'+str(p) for p in references[1:]], consumer])
        model = work/'model'
        model.mkdir()
        shutil.copy2(HERE/'ModelTests.cs', model/'Program.cs')
        shutil.copy2(HERE/'FragmentBoxes.cs', model/'FragmentBoxes.cs')
        (model/'items.tsv').write_bytes(item)
        (model/'uses.tsv').write_bytes(uses)
        reference_dirs = sorted((runtime/'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
        require(reference_dirs, 'Missing local .NET 8 reference pack')
        model_exe = model/'Model.dll'
        (model/'Model.runtimeconfig.json').write_text(json.dumps({'runtimeOptions': {'tfm': 'net8.0',
            'framework': {'name': 'Microsoft.NETCore.App', 'version': '8.0.0'}}}))
        run('compile-model', [dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/out:'+str(model_exe),
                              *['/reference:'+str(p) for p in reference_dirs[-1].glob('*.dll')],
                              model/'Program.cs', model/'FragmentBoxes.cs'])
        run('run-model', [dotnet, model_exe, model/'items.tsv', model/'uses.tsv'])
    checks = verify(before, output.read_bytes())
    require(file_sha(input_path) == sha(before) and file_sha(apk) == APK_SHA, 'Input changed during build')
    sources = [HERE/'build.py', HERE/'Injector.cs', HERE/'FragmentBoxes.cs', HERE/'ModelTests.cs', HERE/'target-pins.json']
    report = {'schema': 'hyjs-fragment-choice-85-v1', 'status': 'STATIC_AND_MODEL_VERIFIED_NOT_PACKAGED',
              'baseAPKSHA256': APK_SHA, 'baseMainSHA256': MAIN80_SHA, 'inputManagedSHA256': sha(before),
              'outputManagedSHA256': file_sha(output), 'helperSHA256': file_sha(helper),
              'referenceTableSHA256': TABLE_SHA, 'eligibleReferenceItemsAndOptionCounts': eligible,
              'payloadFiles': {ENTRY: str(output), HELPER_ENTRY: str(helper)},
              'replacementEntries': {ENTRY: file_sha(output), HELPER_ENTRY: file_sha(helper)},
              'sourcePins': {str(p.relative_to(ROOT)): file_sha(p) for p in sources},
              'oldProfileHelperAndConsumerCompiled': True, 'dotnetModelPassed': True,
              'modelIsNotOriginalDeviceOrNetworkExecution': True, 'deviceTested': False, 'apkBuilt': False,
              'nativeResponseAndExactRetryMethodsUnchanged': True, **checks}
    (out/'manifest.json').write_text(json.dumps(report, indent=2, ensure_ascii=False)+'\n')
    print(json.dumps({key: report[key] for key in ('status', 'inputManagedSHA256', 'outputManagedSHA256',
                                                   'helperSHA256', 'unchangedBodyCount', 'originalEngineMethodsExact')}))


if __name__ == '__main__':
    main()
