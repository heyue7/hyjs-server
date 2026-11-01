"""Check the countdown model and compile the helper against fixed APK 1.1.2."""
import argparse
import ast
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

from dncil.cil.body.reader import read_method_body_from_bytes
import dnfile

HERE = Path(__file__).resolve().parent
APK_SHA = '408e9d03291d6f3505fb91590134b25a8b6aa68f3ebde9c2f4ff985299baccb3'
MAIN_SHA = '3d8f50e543ad082ea03bff294f17421a2eed4316d5cda664b03cc0f46cc23e8a'
LIT_SHA = '151e0a2a6d174558ffa9cb681746d092fde6ba36e937b5825c2f06bb6a8d3230'


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(chunk)
    return h.hexdigest()


def run(command, env):
    result = subprocess.run(['rtk', 'proxy', *map(str, command)], env=env,
                            text=True, capture_output=True, timeout=90)
    if result.stdout:
        print(result.stdout, end='')
    if result.stderr:
        print(result.stderr, end='')
    if result.returncode:
        raise RuntimeError('Countdown check failed: %s' % result.returncode)


def calls(path, owner, method):
    pe = dnfile.dnPE(str(path))
    row = next(row for row in pe.net.mdtables.TypeDef.rows
               if str(row.TypeNamespace) + '.' + str(row.TypeName) == owner)
    entries = [entry.row for entry in row.MethodList if str(entry.row.Name) == method]
    if len(entries) != 1 or not entries[0].Rva:
        raise ValueError('Missing compiled method %s.%s' % (owner, method))
    body = read_method_body_from_bytes(pe.get_data(entries[0].Rva, 1024 * 1024))
    result = []
    for item in body.instructions:
        token = getattr(item.operand, 'value', None)
        if not item.opcode.name.startswith('call') or token is None:
            continue
        table, index = token >> 24, token & 0xffffff
        if table == 6:
            result.append(str(pe.net.mdtables.MethodDef.rows[index - 1].Name))
        elif table == 10:
            result.append(str(pe.net.mdtables.MemberRef.rows[index - 1].Name))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apk', type=Path, required=True)
    parser.add_argument('--runtime', type=Path, required=True, help='native-batch root')
    args = parser.parse_args()
    if digest(args.apk) != APK_SHA:
        raise ValueError('Expected fixed APK 1.1.2')
    runtime = args.runtime / 'dotnet'
    dotnet = runtime / 'dotnet'
    compiler = next((runtime / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = next((runtime / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    env = dict(os.environ, DOTNET_PROCESSOR_COUNT='2', DOTNET_CLI_TELEMETRY_OPTOUT='1')
    tree = ast.parse((HERE / 'build_field_patch.py').read_text())
    sources = next(ast.literal_eval(node.value) for node in tree.body
                   if isinstance(node, ast.Assign) and any(
                       isinstance(target, ast.Name) and target.id == 'SOURCES' for target in node.targets))
    if not {'FieldBoss.cs', 'MapViewer86.cs'}.issubset(sources):
        raise ValueError('Production helper source list omits countdown')
    with tempfile.TemporaryDirectory(prefix='hyjs-boss-countdown-check-') as directory:
        temp = Path(directory)
        names = ('Assembly-CSharp.dll', 'Assembly-CSharp-firstpass.dll', 'UnityEngine.dll',
                 'mscorlib.dll', 'System.dll', 'System.Core.dll', 'LitJson.dll',
                 'HYJS.Field63.dll', 'LocalFieldSettlement.dll')
        with zipfile.ZipFile(args.apk) as apk:
            for name in names:
                (temp / name).write_bytes(apk.read('assets/bin/Data/Managed/' + name))
        if digest(temp / 'Assembly-CSharp.dll') != MAIN_SHA or digest(temp / 'LitJson.dll') != LIT_SHA:
            raise ValueError('Fixed assembly identity differs')
        modern = ['/reference:' + str(path) for path in sorted(refs.glob('*.dll'))]
        output = temp / 'FieldBossPositionChecks.dll'
        run([dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
             '/out:' + str(output), *modern, '/reference:' + str(temp / 'LitJson.dll'),
             HERE / 'FieldBoss.cs', HERE / 'FieldBossPositionChecks.cs'], env)
        config = '{"runtimeOptions":{"tfm":"net8.0","framework":{"name":"Microsoft.NETCore.App","version":"%s"}}}' % refs.parents[1].name
        (temp / 'FieldBossPositionChecks.runtimeconfig.json').write_text(config)
        run([dotnet, output], env)
        helper = temp / 'SoulFlowerDesertV4.dll'
        run([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
             '/deterministic+', '/out:' + str(helper), *['/reference:' + str(temp / name) for name in names],
             *[HERE / name for name in sources]], env)
        for method in ('UpdateSmallBoss', 'UpdateLargeMarkers'):
            if 'MapMarkerLabel' not in calls(helper, 'HYJS.DesertV4.MapViewer86', method):
                raise ValueError('Map label misses shared countdown: ' + method)
        if 'UpdateFixedBossTime' not in calls(helper, 'HYJS.DesertV4.MapViewer86', 'UpdateSmallBoss'):
            raise ValueError('Fixed minimap timer is not updated')
        if 'RefreshBossTime' in calls(helper, 'HYJS.DesertV4.FieldBoss', 'Tick'):
            raise ValueError('Countdown writes the shared native timer')
        if digest(temp / 'Assembly-CSharp.dll') != MAIN_SHA:
            raise ValueError('Fixed game assembly was modified')
        print('PASS both map entries share countdown and no native static timer is written')
        print('PASS C# 3 production helper compiled against fixed APK 1.1.2 assemblies')
        print('APK SHA-256:', APK_SHA)
        print('Assembly-CSharp SHA-256:', MAIN_SHA)
        print('Helper SHA-256:', digest(helper))
    print('Temporary compiler and test outputs cleaned; fixed game assembly untouched')


if __name__ == '__main__':
    main()
