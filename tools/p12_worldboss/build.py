"""Build the opt-in type18 finish bridge without packaging or installing an APK.

Only type18 End and the final original field position callback get adapters.
The existing p10 verifier checks all other methods and exact engine bytecode.
"""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[2]
WORK = ROOT / '.tools/native-batch'


def run(arguments):
    result = subprocess.run(['rtk', 'proxy', *[str(x) for x in arguments]],
                            capture_output=True, text=True)
    if result.stdout:
        print(result.stdout.strip())
    if result.returncode:
        if result.stderr:
            print(result.stderr.strip())
        raise RuntimeError('world boss bridge build command failed')


def verify(original, modified):
    spec = importlib.util.spec_from_file_location('hyjs_voice_build_verify', ROOT / 'tools/p10_voice/build.py')
    verifier = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(verifier)
    result = verifier.verify(original, modified, {('FieldPVEWorldBossBattleOver', 'End'), ('RoomNetField', 'SCCurrentPosition')})
    import dnfile
    from dncil.cil.body.reader import read_method_body_from_bytes
    before, after = dnfile.dnPE(data=original), dnfile.dnPE(data=modified)
    owner = next(t for t in before.net.mdtables.TypeDef if str(t.TypeNamespace) == 'Battle' and str(t.TypeName) == 'FieldPVEWorldBossBattleOver')
    entry = next(x for x in owner.MethodList if str(x.row.Name) == 'End')
    old_body = read_method_body_from_bytes(before.get_data(entry.row.Rva))
    new_body = read_method_body_from_bytes(after.get_data(after.net.mdtables.MethodDef.rows[entry.row_index - 1].Rva))
    prefix = new_body.instructions[:7]
    assert [x.opcode.name for x in prefix] == ['ldstr', prefix[1].opcode.name,
                                               'call', 'ldarg.0', 'call', 'brfalse', 'ret']
    assert prefix[1].opcode.name in ('ldc.i4.0', 'ldc.i4.1')
    assert prefix[5].operand == new_body.instructions[7].offset
    assert len(new_body.instructions) == len(old_body.instructions) + 7
    # The original callback body remains the complete fallback. Compare token
    # operands and branch targets after accounting for its prefix relocation.
    delta = new_body.instructions[7].offset - old_body.instructions[0].offset
    for old, new in zip(old_body.instructions, new_body.instructions[7:]):
        assert old.opcode.name == new.opcode.name
        if 'BrTarget' in old.opcode.operand_type.name:
            assert old.operand + delta == new.operand
        elif hasattr(old.operand, 'value'):
            assert old.operand.value == new.operand.value
        else:
            assert str(old.operand) == str(new.operand)
    result['originalEndCallbackFallbackPreserved'] = True
    result['finishGateOnlyAtOriginalType18End'] = True
    field = next(t for t in before.net.mdtables.TypeDef if str(t.TypeNamespace) == 'Room' and str(t.TypeName) == 'RoomNetField')
    entry = next(x for x in field.MethodList if str(x.row.Name) == 'SCCurrentPosition')
    old = read_method_body_from_bytes(before.get_data(entry.row.Rva))
    new = read_method_body_from_bytes(after.get_data(after.net.mdtables.MethodDef.rows[entry.row_index - 1].Rva))
    hook = new.instructions[-7:-1]
    assert [x.opcode.name for x in hook] == ['ldstr', hook[1].opcode.name, 'call', 'ldarg.0', 'ldarg.1', 'call']
    assert hook[1].opcode.name in ('ldc.i4.0', 'ldc.i4.1')
    assert new.instructions[-1].opcode.name == 'ret' and old.instructions[-1].opcode.name == 'ret'
    assert len(new.instructions) == len(old.instructions) + 6
    assert not old.exception_handlers and not new.exception_handlers
    old_positions = {i.offset: n for n, i in enumerate(old.instructions)}
    new_positions = {i.offset: n for n, i in enumerate(new.instructions[:-7])}
    new_positions[hook[0].offset] = len(old.instructions) - 1
    for left, right in zip(old.instructions[:-1], new.instructions[:-7]):
        assert left.opcode.name == right.opcode.name
        if 'BrTarget' in left.opcode.operand_type.name:
            assert old_positions[left.operand] == new_positions[right.operand]
        elif hasattr(left.operand, 'value'):
            assert left.operand.value == right.operand.value
        else:
            assert str(left.operand) == str(right.operand)
    result['originalFieldPositionHandlerPreservedBeforeRecoveryObserver'] = True
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--input', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--origin', required=True)
    parser.add_argument('--enable-local-world-boss-settlement', action='store_true')
    args = parser.parse_args()
    origin = urlsplit(args.origin)
    if (origin.scheme not in ('http', 'https') or not origin.hostname or origin.username or origin.password or
            origin.query or origin.fragment or origin.path not in ('', '/')):
        raise ValueError('explicit local compatibility HTTP origin required')
    if args.output_directory.exists() and any(args.output_directory.iterdir()):
        raise ValueError('output directory must be empty')
    args.output_directory.mkdir(parents=True, exist_ok=True)
    dotnet = WORK / 'dotnet/dotnet'
    compiler = next((WORK / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = sorted((ROOT / 'analysis/extracted/assets/bin/Data/Managed').glob('*.dll'))
    bridge_sources = [ROOT / 'tools/p12_worldboss/LocalWorldBossSettlement.cs',
                      ROOT / 'tools/p12_worldboss/LocalWorldBossRecoveryPolicy.cs']
    source_sha = hashlib.sha256(b''.join(p.read_bytes() for p in bridge_sources)).hexdigest()
    bridge = args.output_directory / 'LocalWorldBossSettlement.dll'
    run([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+',
         '/out:' + str(bridge), *['/reference:' + str(x) for x in refs],
         *bridge_sources])
    if hashlib.sha256(b''.join(p.read_bytes() for p in bridge_sources)).hexdigest() != source_sha:
        raise RuntimeError('bridge source changed during compilation')
    with tempfile.TemporaryDirectory(prefix='hyjs-worldboss-injector-') as directory:
        temp = Path(directory)
        shutil.copy2(ROOT / 'tools/p12_worldboss/Injector.cs', temp / 'Program.cs')
        dnlib = WORK / 'injector/bin/Debug/net8.0/dnlib.dll'
        (temp / 'WorldBossInjector.csproj').write_text(
            '<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
            '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
            '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib"><HintPath>'
            + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run([dotnet, 'build', temp / 'WorldBossInjector.csproj', '--nologo', '--verbosity', 'quiet'])
        output = args.output_directory / 'Assembly-CSharp.dll'
        run([dotnet, temp / 'bin/Debug/net8.0/WorldBossInjector.dll', args.input, bridge, output,
             args.origin, 'true' if args.enable_local_world_boss_settlement else 'false'])
    proof = verify(args.input.read_bytes(), output.read_bytes())
    proof.update(adapter='local-world-boss-end-commit-recovery-v2', enabled=args.enable_local_world_boss_settlement,
                 originalServerRuleRestored=False, deviceExecuted=False,
                 inputSHA256=hashlib.sha256(args.input.read_bytes()).hexdigest(),
                 outputSHA256=hashlib.sha256(output.read_bytes()).hexdigest(),
                 bridgeSourceSHA256=source_sha,
                 bridgeSources=[{'path': str(p.relative_to(ROOT)), 'sha256': hashlib.sha256(p.read_bytes()).hexdigest()} for p in bridge_sources],
                 bridgeSHA256=hashlib.sha256(bridge.read_bytes()).hexdigest())
    (args.output_directory / 'manifest.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps(proof))


if __name__ == '__main__':
    main()
