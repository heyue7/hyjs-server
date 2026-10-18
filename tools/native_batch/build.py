"""Compile a diagnostic runner against the APK's own runtime references.

Only the original PlaybackKit click entry calls the new runner. Reject any
change to original engine instructions, exception regions or member identity
before exposing the result to the APK builder.
"""
from pathlib import Path
import hashlib
import json
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[2]
WORK = ROOT / '.tools/native-batch'
DOTNET = WORK / 'dotnet/dotnet'
REFERENCES = ROOT / 'analysis/extracted/assets/bin/Data/Managed'


def run(*args):
    subprocess.run([str(a) for a in args], cwd=ROOT, check=True)


def compile_implementation():
    compiler = next((WORK / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    sources = [ROOT / 'analysis/p6/OriginalHeadlessPlaybackCaller.cs',
               ROOT / 'tools/native_batch/NativeBatchRunner.cs',
               ROOT / 'tools/native_batch/LocalPVEUICompatibility.cs',
               ROOT / 'tools/native_batch/LocalActivityInformation.cs',
               ROOT / 'tools/native_batch/LocalExpeditionPresentation.cs']
    identity = hashlib.sha256(b''.join(p.read_bytes() for p in sources)).hexdigest()[:16]
    runner = WORK / ('NativeBatchImpl_' + identity + '.dll')
    run(DOTNET, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+',
        '/out:' + str(runner), *['/reference:' + str(p) for p in sorted(REFERENCES.glob('*.dll'))],
        *sources)
    (WORK / 'native-runner-current.dll').write_bytes(runner.read_bytes())
    return runner


def compile_runner():
    compile_implementation()
    compiler = next((WORK / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    runner = WORK / 'NativeBatch.dll'
    run(DOTNET, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+',
        '/out:' + str(runner), *['/reference:' + str(p) for p in sorted(REFERENCES.glob('*.dll'))],
        ROOT / 'tools/native_batch/NativeBatchBridge.cs')
    project = WORK / 'injector/NativeBatchInjector.csproj'
    (project.parent / 'Program.cs').write_bytes((ROOT / 'tools/native_batch/Injector.cs').read_bytes())
    run(DOTNET, 'build', project, '--no-restore', '--nologo', '--verbosity', 'quiet')
    return runner


def verify(original, modified):
    # CLI parsing is needed only when rewriting/verifying APK assemblies.
    # A private caller-only update compiles CSharp and leaves the APK intact.
    import dnfile
    from dncil.cil.body.reader import read_method_body_from_bytes

    before = dnfile.dnPE(data=original)
    after = dnfile.dnPE(data=modified)
    protected = 0
    for index, owner in enumerate(before.net.mdtables.TypeDef.rows):
        revised_owner = after.net.mdtables.TypeDef.rows[index]
        if (str(owner.TypeNamespace), str(owner.TypeName)) != (str(revised_owner.TypeNamespace), str(revised_owner.TypeName)):
            raise ValueError('Original type identity changed')
        for entry in owner.MethodList:
            row = entry.row
            revised = after.net.mdtables.MethodDef.rows[entry.row_index - 1]
            if row.Signature.value != revised.Signature.value or str(row.Name) != str(revised.Name):
                raise ValueError('Original method identity changed')
            if not row.Rva or not str(owner.TypeNamespace).startswith('Battle.Engine'):
                continue
            old = read_method_body_from_bytes(before.get_data(row.Rva))
            new = read_method_body_from_bytes(after.get_data(revised.Rva))
            shape = lambda b: [(e.exception_type, e.try_start, e.try_end, e.handler_start, e.handler_end,
                                 e.filter_start, getattr(e.catch_type, 'value', None)) for e in b.exception_handlers]
            # dncil's helper includes the first alignment byte after CodeSize
            # for some fat bodies. Compare exactly the CLI instruction region;
            # the writer may compact an equivalent exception-table encoding.
            old_code = before.get_data(row.Rva + old.header_size, old.code_size)
            new_code = after.get_data(revised.Rva + new.header_size, new.code_size)
            # A short equivalent body can use the CLI tiny header (implicit
            # stack limit 8). It may enlarge the limit without changing code.
            if old_code != new_code or shape(old) != shape(new) or new.max_stack < old.max_stack:
                raise ValueError('Original engine body changed: ' + str(owner.TypeName) + '::' + str(row.Name))
            protected += 1
    return protected


def patch(raw, *, local_pve_round_compat=False):
    WORK.mkdir(exist_ok=True, parents=True)
    runner = compile_runner()
    source = WORK / 'caller-before.dll'
    output = WORK / 'caller-after.dll'
    source.write_bytes(raw)
    arguments = [DOTNET, WORK / 'injector/bin/Debug/net8.0/NativeBatchInjector.dll', source, runner, output]
    if local_pve_round_compat:
        arguments.append('--local-pve-round-compat')
    run(*arguments)
    revised = output.read_bytes()
    protected = verify(raw, revised)
    proof = {'originalEngineMethodsUnchanged': protected,
             'inputSHA256': hashlib.sha256(raw).hexdigest(),
             'outputSHA256': hashlib.sha256(revised).hexdigest(),
             'runnerSHA256': hashlib.sha256(runner.read_bytes()).hexdigest(),
             'localPVERoundCompatibilityHooks': local_pve_round_compat,
             'nativeExecuted': False}
    (WORK / 'build-validation.json').write_text(json.dumps(proof, indent=2) + '\n')
    print('Original engine methods preserved:', protected)
    return revised, runner.read_bytes()


if __name__ == '__main__':
    source = Path(sys.argv[1])
    result, _ = patch(source.read_bytes())
    Path(sys.argv[2]).write_bytes(result)
