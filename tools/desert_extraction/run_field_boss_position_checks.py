"""Exercise boss position confirmation and compile the actual C# 3 helper."""
import argparse
import ast
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

HERE = Path(__file__).resolve().parent
APK_SHA = '25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
MAP_SHA = 'ba402ea01a9cd4fb3db03e57f6a9335a738a8a48a212cb83cd7935fbce66668f'
LIT_SHA = '151e0a2a6d174558ffa9cb681746d092fde6ba36e937b5825c2f06bb6a8d3230'


def digest(path):
    sha = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            sha.update(chunk)
    return sha.hexdigest()


def run(args, env):
    result = subprocess.run(['rtk', 'proxy', *map(str, args)], env=env,
                            text=True, capture_output=True, timeout=90)
    if result.stdout:
        print(result.stdout, end='')
    if result.stderr:
        print(result.stderr, end='')
    if result.returncode:
        raise RuntimeError('Boss confirmation check failed: %s' % result.returncode)


def check_compiled_entry(helper):
    pe = dnfile.dnPE(str(helper))
    owners = {str(row.TypeNamespace) + '.' + str(row.TypeName): row
              for row in pe.net.mdtables.TypeDef.rows}

    def calls(owner, name):
        rows = [entry.row for entry in owners[owner].MethodList if str(entry.row.Name) == name]
        if len(rows) != 1 or not rows[0].Rva:
            raise ValueError('Compiled entry missing: %s.%s' % (owner, name))
        body = read_method_body_from_bytes(pe.get_data(rows[0].Rva, 1024 * 1024))
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

    entry = calls('HYJS.DesertV4.FieldRuntime', 'StartEncounter')
    if entry.count('ShowLoadingPre') != 1 or entry.count('ConfirmPosition') != 1 or entry.index('ShowLoadingPre') > entry.index('ConfirmPosition'):
        raise ValueError('Original loader is not started before boss confirmation')
    if 'StartEncounter' not in calls('HYJS.DesertV4.FieldHooks', 'BeforeEncounter'):
        raise ValueError('Original encounter hook does not reach the checked entry')
    print('PASS compiled encounter hook reaches original loader before boss confirmation')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--map-managed', type=Path, required=True)
    parser.add_argument('--runtime', type=Path, required=True)
    args = parser.parse_args()
    if digest(args.base_apk) != APK_SHA or digest(args.map_managed) != MAP_SHA:
        raise ValueError('Expected pinned v80 APK and its desert map assembly')
    runtime = args.runtime / 'dotnet'
    dotnet = runtime / 'dotnet'
    compiler = next((runtime / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = next((runtime / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    env = dict(os.environ, DOTNET_PROCESSOR_COUNT='2', DOTNET_CLI_TELEMETRY_OPTOUT='1')
    source_tree = ast.parse((HERE / 'build_field_patch.py').read_text())
    source_list = next(ast.literal_eval(node.value) for node in source_tree.body
                       if isinstance(node, ast.Assign) and any(
                           isinstance(target, ast.Name) and target.id == 'SOURCES'
                           for target in node.targets))
    if not {'FieldBoss.cs', 'FieldRuntime.cs', 'PositionSync.cs'}.issubset(source_list):
        raise ValueError('Production source list omits a boss dependency')
    with tempfile.TemporaryDirectory(prefix='hyjs-boss-position-check-') as directory:
        tmp = Path(directory)
        names = ('Assembly-CSharp-firstpass.dll', 'UnityEngine.dll', 'mscorlib.dll',
                 'System.dll', 'System.Core.dll', 'LitJson.dll', 'HYJS.Field63.dll',
                 'LocalFieldSettlement.dll')
        with zipfile.ZipFile(args.base_apk) as apk:
            for name in names:
                (tmp / name).write_bytes(apk.read('assets/bin/Data/Managed/' + name))
        if digest(tmp / 'LitJson.dll') != LIT_SHA:
            raise ValueError('Original game LitJson differs')
        output = tmp / 'FieldBossPositionChecks.dll'
        modern = ['/reference:' + str(path) for path in sorted(refs.glob('*.dll'))]
        run([dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
             '/out:' + str(output), *modern, '/reference:' + str(tmp / 'LitJson.dll'),
             HERE / 'FieldBoss.cs', HERE / 'FieldBossPositionChecks.cs'], env)
        config = '{"runtimeOptions":{"tfm":"net8.0","framework":{"name":"Microsoft.NETCore.App","version":"%s"}}}' % refs.parents[1].name
        (tmp / 'FieldBossPositionChecks.runtimeconfig.json').write_text(config)
        run([dotnet, output], env)
        game_refs = [args.map_managed.resolve(), *[tmp / name for name in names]]
        helper = tmp / 'SoulFlowerDesertV4.dll'
        run([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+',
             '/langversion:3', '/deterministic+', '/out:' + str(helper),
             *['/reference:' + str(path) for path in game_refs],
             *[HERE / name for name in source_list]], env)
        check_compiled_entry(helper)
        print('PASS complete helper C# 3 compile against pinned game and Unity assemblies')
        print('FieldBoss.cs SHA-256:', digest(HERE / 'FieldBoss.cs'))
        print('FieldRuntime.cs SHA-256:', digest(HERE / 'FieldRuntime.cs'))
        print('PositionSync.cs SHA-256:', digest(HERE / 'PositionSync.cs'))
    print('Temporary compiler and test outputs cleaned')


if __name__ == '__main__':
    main()
