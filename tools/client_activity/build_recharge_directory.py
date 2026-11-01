"""Build only the recharge helper against the exact released 1.1.5 APK."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
BASE_COMMIT = '292fe277f67c45413932e0973e15a10c5a53b433'
APK_SHA = 'bd3e4caefe47545de7b2272681a363f79df5d75838b96bc752e882cdbb07137e'
MAIN_SHA = '3d8f50e543ad082ea03bff294f17421a2eed4316d5cda664b03cc0f46cc23e8a'
HELPER_SHA = '17672c2a1e09334a82c37168586720241faec0b26d3bf83e31bc71090740da0b'
ENTRY = 'assets/bin/Data/Managed/'
SOURCES = [ROOT / 'tools/client_activity/ActivityGuard.cs', ROOT / 'tools/client_activity/InstanceSelector.cs']


def sha(data):
    return hashlib.sha256(data).hexdigest()


def command(args, cwd=ROOT):
    result = subprocess.run(['rtk', 'proxy', *map(str, args)], cwd=cwd, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError('Command failed: ' + str(args[0]) + '\n' + result.stdout + result.stderr)
    return result.stdout.strip()


def git_source(path):
    result = subprocess.run(['rtk', 'proxy', 'git', 'show', BASE_COMMIT + ':' + str(path.relative_to(ROOT))],
                            cwd=ROOT, capture_output=True, check=True)
    return result.stdout


def method(pe, typename, name):
    owner = next(t for t in pe.net.mdtables.TypeDef.rows
                 if str(t.TypeNamespace) + '.' + str(t.TypeName) == typename)
    row = next(m.row for m in owner.MethodList if str(m.row.Name) == name)
    return read_method_body_from_bytes(pe.get_data(row.Rva, 1024 * 1024))


def operand_name(pe, instruction):
    token = getattr(instruction.operand, 'value', None)
    if token is None:
        return ''
    table, index = token >> 24, token & 0xffffff
    if table == 4:
        return str(pe.net.mdtables.Field.rows[index - 1].Name)
    if table == 6:
        return str(pe.net.mdtables.MethodDef.rows[index - 1].Name)
    if table == 10:
        return str(pe.net.mdtables.MemberRef.rows[index - 1].Name)
    return ''


def native_call_order(main):
    pe = dnfile.dnPE(data=main)
    setup = method(pe, 'UI.AcRechargeSingleItem', 'SetActivityItem')
    instructions = setup.instructions
    store = next(i for i, ins in enumerate(instructions) if ins.opcode.name == 'stfld' and operand_name(pe, ins) == 'activityId')
    hook = next(i for i, ins in enumerate(instructions) if ins.opcode.name == 'call' and operand_name(pe, ins) == 'Apply')
    assert [ins.opcode.name for ins in instructions[store-2:store]] == ['ldarg.0', 'ldarg.2']
    assert [ins.opcode.name for ins in instructions[hook-2:hook]] == ['ldarg.0', 'ldarg.1']
    assert store < hook
    panel = method(pe, 'UI.UIActivityRechargeSurprise', 'UpdateActivityItems')
    row_call = next(i for i, ins in enumerate(panel.instructions)
                    if ins.opcode.name == 'callvirt' and operand_name(pe, ins) == 'SetActivityItem')
    assert panel.instructions[row_call-1].opcode.name == 'ldfld'
    assert operand_name(pe, panel.instructions[row_call-1]) == 'activityId'
    add = next(i for i, ins in enumerate(panel.instructions)
               if ins.opcode.name == 'callvirt' and operand_name(pe, ins) == 'AddChild')
    assert add < row_call
    callback = method(pe, 'UI.UIActivityRechargeSurprise', 'MsgCallback')
    page_store = next(i for i, ins in enumerate(callback.instructions)
                      if ins.opcode.name == 'stfld' and operand_name(pe, ins) == 'activityId')
    update = next(i for i, ins in enumerate(callback.instructions)
                  if ins.opcode.name == 'call' and operand_name(pe, ins) == 'UpdateActivityItems')
    assert page_store < update
    return {'pageIDStoredIL': callback.instructions[page_store].offset - callback.header_size,
            'rowIDArgumentIL': panel.instructions[row_call-1].offset - panel.header_size,
            'rowAddedBeforeSetupIL': panel.instructions[add].offset - panel.header_size,
            'rowIDStoredIL': instructions[store].offset - setup.header_size,
            'helperCallIL': instructions[hook].offset - setup.header_size}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--tool-root', type=Path, default=Path('/Users/lirui/LR/hyjs'))
    parser.add_argument('--python', type=Path, default=Path('/Users/lirui/LR/hyjs/.analysis-venv/bin/python'))
    args = parser.parse_args()
    with args.base_apk.open('rb') as src:
        assert hashlib.file_digest(src, 'sha256').hexdigest() == APK_SHA, 'input APK SHA mismatch'
    out = args.output_directory.resolve()
    assert not out.exists(), 'use a fresh task-owned output directory'
    runtime = args.tool_root / '.tools/native-batch'
    dotnet = runtime / 'dotnet/dotnet'
    compiler = next((runtime / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    refnet = next((runtime / 'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    dnlib = runtime / 'injector/bin/Debug/net8.0/dnlib.dll'
    checks = json.loads(command([args.python, ROOT / 'tools/client_activity/test_recharge_directory.py']))
    out.mkdir(parents=True)
    try:
        with tempfile.TemporaryDirectory(prefix='hyjs-recharge-build-') as temp:
            work = Path(temp)
            with zipfile.ZipFile(args.base_apk) as apk:
                main_bytes = apk.read(ENTRY + 'Assembly-CSharp.dll')
                old_helper = apk.read(ENTRY + 'SoulFlowerActivity.dll')
                assert sha(main_bytes) == MAIN_SHA and sha(old_helper) == HELPER_SHA
                refs = []
                for name in ['Assembly-CSharp.dll', 'UnityEngine.dll', 'Assembly-CSharp-firstpass.dll',
                             'mscorlib.dll', 'System.dll', 'System.Core.dll']:
                    path = work / name
                    path.write_bytes(apk.read(ENTRY + name))
                    refs.append(path)
            call_order = native_call_order(main_bytes)
            old = work / 'shipped.dll'
            old.write_bytes(old_helper)
            base = work / 'base'
            base.mkdir()
            baseline_sources = []
            for source in SOURCES:
                path = base / source.name
                path.write_bytes(git_source(source))
                baseline_sources.append(path)
            assert SOURCES[0].read_bytes() == baseline_sources[0].read_bytes(), 'baseline ActivityGuard changed'
            baseline = base / 'SoulFlowerActivity.dll'
            def compile_helper(target, sources):
                return command([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
                                '/deterministic+', '/codepage:65001', '/out:' + str(target),
                                *['/reference:' + str(p) for p in refs], *sources])
            compile_helper(baseline, baseline_sources)
            assert sha(baseline.read_bytes()) == HELPER_SHA, 'source baseline differs from shipped helper'
            candidate = out / 'SoulFlowerActivity.dll'
            compile_helper(candidate, SOURCES)
            verifier = work / 'VerifyRechargeHelper.dll'
            command([dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:latest',
                     '/out:' + str(verifier), *['/reference:' + str(p) for p in sorted(refnet.glob('*.dll'))],
                     '/reference:' + str(dnlib), ROOT / 'tools/client_activity/VerifyRechargeHelper.cs'])
            shutil.copy2(dnlib, work / 'dnlib.dll')
            (work / 'VerifyRechargeHelper.runtimeconfig.json').write_text(json.dumps({
                'runtimeOptions': {'tfm': 'net8.0', 'framework': {'name': 'Microsoft.NETCore.App', 'version': '8.0.0'}}
            }))
            abi = command([dotnet, verifier, old, candidate])
            report = {'schema': 'hyjs-recharge-directory-helper-v1', 'status': 'STATIC_VERIFIED_NOT_PACKAGED',
                      'baseCommit': BASE_COMMIT, 'baseAPKSHA256': APK_SHA, 'mainAssemblySHA256': MAIN_SHA,
                      'shippedHelperSHA256': HELPER_SHA, 'reconstructedHelperSHA256': sha(baseline.read_bytes()),
                      'candidateHelperSHA256': sha(candidate.read_bytes()), 'replacementEntry': ENTRY + 'SoulFlowerActivity.dll',
                      'inheritedABIAndBehavior': abi, 'nativeCallOrder': call_order, 'modelAndPrefab': checks,
                      'referenceSHA256': {p.name: sha(p.read_bytes()) for p in refs},
                      'sourceSHA256': {str(p.relative_to(ROOT)): sha(p.read_bytes()) for p in SOURCES + [
                          Path(__file__).resolve(), ROOT / 'tools/client_activity/VerifyRechargeHelper.cs',
                          ROOT / 'tools/client_activity/RechargeDirectoryHarness.cs', ROOT / 'tools/client_activity/test_recharge_directory.py']},
                      'apkBuilt': False, 'deviceVerified': False}
            (out / 'manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
            print(json.dumps({'status': report['status'], 'helper': str(candidate),
                              'sha256': report['candidateHelperSHA256'], 'verification': abi}, ensure_ascii=False))
    except BaseException:
        # This directory was created by this invocation and contains only its outputs.
        shutil.rmtree(out)
        raise


if __name__ == '__main__':
    main()
