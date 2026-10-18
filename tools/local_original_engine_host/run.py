"""Bounded desktop-original-Engine POC. No Go, device or installed game changes.

Uses only the already installed private .NET SDK, original managed references,
original caller sources and two existing Daily inputs. Outputs never grant P6
authority. No NuGet restore/download or application assembly rewriting.
"""
from pathlib import Path
import hashlib
import json
import shutil
import subprocess
import sys
import struct
import zipfile
import UnityPy
import argparse
import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from case_sets import prepare
import time
import os

ROOT = Path(__file__).resolve().parents[2]
DOTNET = ROOT / '.tools/native-batch/dotnet/dotnet'
REFS = ROOT / 'analysis/extracted/assets/bin/Data/Managed'
OUTPUT = ROOT / 'analysis/growth-first-20261016/p6-local-headless-poc-r1/attempt-4-reader-jit-boundary'
ORIGINAL = 'ee14d5df4077573abf94be309d6ebdbe7f4ba84c31b09743ed9a9008b65d200f'
STABLE_HOST_ENV = {'COMPlus_TieredCompilation': '0', 'COMPlus_JitNoInline': '1', 'COMPlus_JITMinOpts': '1'}


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(label, args, timeout=90, host_environment=None):
    started = time.monotonic()
    try:
        result = subprocess.run([str(a) for a in args], cwd=ROOT, capture_output=True, text=True, timeout=timeout,
                                env=None if host_environment is None else {**os.environ, **host_environment})
    except subprocess.TimeoutExpired as error:
        decode = lambda value: value.decode(errors='replace') if isinstance(value, bytes) else value or ''
        (OUTPUT / (label + '.log')).write_text(decode(error.stdout) + decode(error.stderr) + '\nHOST_TIMEOUT_SECONDS=' + str(timeout) + '\n')
        raise
    (OUTPUT / (label + '.log')).write_text(result.stdout + result.stderr)
    print(label, result.returncode, flush=True)
    if result.returncode:
        raise RuntimeError(label + ' failed; see preserved log')
    return round(time.monotonic() - started, 6)


def main():
    global OUTPUT
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output-directory', type=Path, default=OUTPUT)
    parser.add_argument('--case-set', choices=['daily', 'p7', 'p7-rush', 'p7-awake', 'growth-awake003', 'trial047025'], default='daily')
    args = parser.parse_args()
    OUTPUT = args.output_directory.resolve()
    if OUTPUT.exists() and (OUTPUT / 'manifest.json').exists():
        raise ValueError('Refuse to replace prior POC evidence')
    OUTPUT.mkdir(parents=True, exist_ok=True)
    work = OUTPUT / 'runtime'
    work.mkdir(exist_ok=True)
    compiler = next((DOTNET.parent / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    netrefs = sorted((DOTNET.parent / 'packs/Microsoft.NETCore.App.Ref/8.0.32/ref/net8.0').glob('*.dll'))
    sources = [ROOT / 'analysis/p6/OriginalHeadlessPlaybackCaller.cs'] + [ROOT / 'tools/native_batch' / f for f in ['NativeBatchRunner.cs', 'LocalPVEUICompatibility.cs', 'LocalActivityInformation.cs', 'LocalExpeditionPresentation.cs']]
    pins = {str(p.relative_to(ROOT)): sha(p) for p in sources + list(Path(__file__).parent.glob('*.*')) if p.is_file()}
    assert sha(REFS / 'Assembly-CSharp.dll') == ORIGINAL
    config = {'runtimeOptions': {'tfm': 'net8.0', 'framework': {'name': 'Microsoft.NETCore.App', 'version': '8.0.32'}}}
    config['runtimeOptions']['configProperties'] = {'System.Runtime.TieredCompilation': False}
    manifest = {'schema': 'local-original-engine-host-poc-v2', 'caseSet': args.case_set, 'executionOrigin': 'local-coreclr-original-engine-not-phone-capture', 'sourcePins': pins, 'originalAssemblySHA256': ORIGINAL, 'nativeCompared': False, 'authoritative': False, 'originalEngineMethodsChanged': False, 'goldensChanged': False, 'unityAdapterWhitelist': ['UnityEngine.Application::get_isPlaying', 'UnityEngine.AssetBundle::Unload', 'UnityEngine.TextAsset::get_bytes'], 'unityLogging': 'original managed Debug.logger.logEnabled=false; no method rewrite', 'status': 'STARTED'}
    manifest['hostRuntimeEnvironment'] = dict(STABLE_HOST_ENV)
    manifest['hostRuntimeConfigProperties'] = dict(config['runtimeOptions']['configProperties'])
    manifest['runtimeProfileRationale'] = 'Retain original caller StackTrace AI.Update origin across long runs; host-only, compiler processes unchanged'
    snapshots = OUTPUT / 'execution-source'
    snapshots.mkdir(exist_ok=True)
    for path in Path(__file__).parent.glob('*.*'):
        if path.is_file():
            shutil.copy2(path, snapshots / path.name)
    try:
        for file in ['Assembly-CSharp.dll', 'Assembly-CSharp-firstpass.dll', 'LitJson.dll']:
            shutil.copy2(REFS / file, work / file)
        for name in ['LocalOriginalEngineHost']:
            (work / (name + '.runtimeconfig.json')).write_text(json.dumps(config))
        # Preserve every existing Unity method body and metadata token verbatim.
        # Reuse original bodies for three explicit noncombat Unity adapters;
        # only their RVA and InternalCall metadata flag bytes may change.
        unity_bytes = bytearray((REFS / 'UnityEngine.dll').read_bytes())
        unity = dnfile.dnPE(data=bytes(unity_bytes))
        getter = unity.net.mdtables.MethodDef.rows[0x09cb - 1]
        assert str(getter.Name) == 'get_isPlaying' and getter.Rva == 0 and getter.struct.ImplFlags == 0x1000
        zero_body = None
        for method in unity.net.mdtables.MethodDef.rows:
            if not method.Rva:
                continue
            body = read_method_body_from_bytes(unity.get_data(method.Rva))
            if unity.get_data(method.Rva + body.header_size, body.code_size) == b'\x16\x2a' and not body.exception_handlers:
                zero_body = method.Rva
                break
        assert zero_body is not None
        offset = getter.struct.get_file_offset()
        struct.pack_into('<IH', unity_bytes, offset, zero_body, 0)
        allowed_offsets = set(range(offset, offset + 6))
        # The not-taken bundle path is still JIT-validated by CoreCLR. These
        # two native APIs fail closed if reached; they never supply game data.
        thrower = unity.net.mdtables.MethodDef.rows[9348 - 1]
        throw_body = read_method_body_from_bytes(unity.get_data(thrower.Rva))
        assert [i.opcode.name for i in throw_body.instructions] == ['newobj', 'throw']
        for rid, expected_name in [(0x2f, 'Unload'), (0x83b, 'get_bytes')]:
            native = unity.net.mdtables.MethodDef.rows[rid - 1]
            assert str(native.Name) == expected_name and native.Rva == 0 and native.struct.ImplFlags == 0x1000
            native_offset = native.struct.get_file_offset()
            struct.pack_into('<IH', unity_bytes, native_offset, thrower.Rva, 0)
            allowed_offsets.update(range(native_offset, native_offset + 6))
        assert all(a == b for i, (a, b) in enumerate(zip((REFS / 'UnityEngine.dll').read_bytes(), unity_bytes)) if i not in allowed_offsets)
        (work / 'UnityEngine.dll').write_bytes(unity_bytes)
        manifest['unityGetterMetadataByteWhitelist'] = {'offset': offset, 'length': 6, 'sharedFalseBodyRVA': zero_body}
        manifest['unityMetadataByteWhitelist'] = sorted(allowed_offsets)
        before, after = dnfile.dnPE(str(REFS / 'UnityEngine.dll')), dnfile.dnPE(str(work / 'UnityEngine.dll'))
        unchanged = 0
        for index, old in enumerate(before.net.mdtables.MethodDef.rows):
            new = after.net.mdtables.MethodDef.rows[index]
            assert str(old.Name) == str(new.Name) and old.Signature.value == new.Signature.value
            if not old.Rva:
                if new.Rva:
                    assert index + 1 in {0x09cb, 0x2f, 0x83b}
                continue
            a, b = read_method_body_from_bytes(before.get_data(old.Rva)), read_method_body_from_bytes(after.get_data(new.Rva))
            assert before.get_data(old.Rva + a.header_size, a.code_size) == after.get_data(new.Rva + b.header_size, b.code_size)
            eh = lambda z: [(x.exception_type,x.try_start,x.try_end,x.handler_start,x.handler_end,x.filter_start,getattr(x.catch_type,'value',None)) for x in z.exception_handlers]
            assert eh(a) == eh(b)
            unchanged += 1
        manifest['unchangedOriginalUnityMethodBodies'] = unchanged
        run('compile-original-caller', [DOTNET, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+', '/out:' + str(work / 'LocalOriginalCaller.dll'), *['/reference:' + str(p) for p in sorted(REFS.glob('*.dll'))], *sources])
        run('compile-host', [DOTNET, compiler, '/nologo', '/target:exe', '/out:' + str(work / 'LocalOriginalEngineHost.dll'), *['/reference:' + str(p) for p in netrefs], Path(__file__).parent / 'Program.cs'])
        game = dnfile.dnPE(str(REFS / 'Assembly-CSharp.dll'))
        tables = work / 'Assets/OutputBundle/tables'
        tables.mkdir(parents=True, exist_ok=True)
        table_pins = {}
        apk = zipfile.ZipFile(ROOT / 'apks/original/10056262.apk')
        for owner in game.net.mdtables.TypeDef.rows:
            if not str(owner.TypeNamespace).startswith('Dict.Dao'):
                continue
            for entry in owner.MethodList:
                if str(entry.row.Name) != 'GetFileName' or not entry.row.Rva:
                    continue
                for instruction in read_method_body_from_bytes(game.get_data(entry.row.Rva)).instructions:
                    if instruction.opcode.name != 'ldstr':
                        continue
                    name = game.net.user_strings.get(instruction.operand.value & 0xffffff).value
                    source = ROOT / 'analysis/tables' / name.removesuffix('.txt').lower()
                    if not source.is_file():
                        source = ROOT / 'analysis/tables' / name.replace('.txt', '.tsv')
                    bundle_name = 'assets/android/tabledata/' + name.removesuffix('.txt').lower()
                    bundle_bytes = apk.read(bundle_name)
                    assets = [o.read() for o in UnityPy.load(bundle_bytes).objects if o.type.name == 'TextAsset']
                    assert len(assets) == 1, bundle_name
                    raw = assets[0].m_Script
                    if isinstance(raw, str):
                        raw = raw.encode('utf-8', errors='surrogateescape')
                    assert source.read_bytes() == raw, 'Extracted original table differs: ' + str(source)
                    (tables / name).write_bytes(raw)
                    table_pins[str(source.relative_to(ROOT))] = {'sha256': sha(source), 'originalAPKBundle': bundle_name, 'bundleSHA256': hashlib.sha256(bundle_bytes).hexdigest()}
        apk.close()
        manifest['originalRawTablePins'] = table_pins
        manifest['caseSets'] = prepare(args.case_set, work)
        manifest['executionSeconds'] = {}
        for group in manifest['caseSets']:
            manifest['executionSeconds'][group['set']] = run('execute-original-engine-' + group['set'], [DOTNET, work / 'LocalOriginalEngineHost.dll', work, work / group['caseDirectory']], timeout=120, host_environment=STABLE_HOST_ENV)
        manifest['status'] = 'EXECUTED_PENDING_STRICT_COMPARE'
    except Exception as error:
        manifest['status'] = 'BLOCKED'
        manifest['error'] = type(error).__name__ + ': ' + str(error)
        print(error, flush=True)
    finally:
        assert sha(work / 'Assembly-CSharp.dll') == ORIGINAL
        manifest['outputPins'] = {str(p.relative_to(OUTPUT)): sha(p) for p in OUTPUT.glob('*.log')}
        (OUTPUT / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    return 0 if manifest['status'].startswith('EXECUTED') else 1


if __name__ == '__main__':
    sys.exit(main())
