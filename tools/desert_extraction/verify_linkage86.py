"""Audit the fixed 86 static Main and five helpers against the actual v80 APK.

This compiles a read-only dnlib checker in a temporary directory. It never loads
or runs game code and never signs, installs, or turns QA evidence into a review.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
BASE_SHA = '25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
REFERENCE_QA83_SHA = '1d4b6798ffe54f2545a7d14b9109ae9bd84e7a6d66f7a17b253e213b414adaa8'
STATIC_SCHEMA = 'hyjs-desert-trial-feedback-fragment86-static-proof-v1'
PREFIX = 'assets/bin/Data/Managed/'
NAMES = ('Assembly-CSharp.dll', 'SoulFlowerDesert53.dll', 'SoulFlowerDesertV4.dll',
         'SoulFlowerGGOShop.dll', 'HYJS.Feedback65.dll', 'HYJS.FragmentBoxes85.dll',
         'HYJS.DungeonContent.dll', 'HYJS.WantedSkip87.dll')
FRAMEWORK_COMPONENTS = ('UnityEngine.dll', 'LitJson.dll', 'mscorlib.dll', 'System.dll', 'System.Core.dll')
MONO_COMPONENTS = ('lib/armeabi-v7a/libmono.so', 'lib/x86/libmono.so')


def require(condition, message):
    if not condition:
        raise ValueError(message)


def file_sha(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def managed_entries(archive):
    """Require a unique, flat managed DLL inventory before extracting any file."""
    names = archive.namelist()
    require(len(names) == len(set(names)), 'Base APK contains duplicate ZIP entries')
    result, folded = {}, set()
    for name in names:
        if not name.startswith(PREFIX) or not name.lower().endswith('.dll'):
            continue
        tail = name[len(PREFIX):]
        require(bool(tail) and '/' not in tail and '\\' not in tail and
                PurePosixPath(tail).name == tail and tail not in ('.', '..'),
                'Unexpected non-flat managed DLL entry: ' + name)
        require(tail.casefold() not in folded, 'Duplicate managed DLL filename: ' + tail)
        folded.add(tail.casefold())
        result[tail] = name
    require({'Assembly-CSharp.dll', 'SoulFlowerGGOShop.dll', 'HYJS.Feedback65.dll'} <= set(result),
            'Base is missing an inherited 86 assembly')
    return result


def checked_overlay(overlay, base_sha):
    """Bind all six actual DLL hashes to the independent static build proof."""
    overlay = Path(overlay).resolve()
    require(overlay.is_relative_to(ROOT) and overlay.is_dir(), 'Static overlay must be inside the project')
    proof_path = (overlay/'build-proof.json').resolve()
    require(proof_path.parent == overlay and proof_path.is_file(), 'Independent static proof missing')
    proof = json.loads(proof_path.read_text(encoding='utf-8'))
    require(proof.get('schema') == STATIC_SCHEMA and proof.get('status') == 'STATIC_PREVIEW_NOT_FOR_RELEASE'
            and proof.get('baseAPKSHA256') == base_sha
            and not any(proof.get(k) for k in ('qaOnly', 'neverPromote', 'diagnosticOnly', 'qaOrigin')),
            'Overlay is not an independent 86 static candidate')
    source_pins = proof.get('sourcePins')
    require(isinstance(source_pins, dict) and source_pins, 'Static source pins missing')
    for name, expected_sha in source_pins.items():
        require(isinstance(name, str) and isinstance(expected_sha, str)
                and re.fullmatch(r'[0-9a-f]{64}', expected_sha) is not None, 'Invalid static source pin')
        pinned = (ROOT/name).resolve()
        require(pinned.is_relative_to(ROOT) and pinned.is_file() and file_sha(pinned) == expected_sha,
                'Static source pin differs: ' + name)
    expected = proof.get('payloadSHA256')
    require(isinstance(expected, dict) and set(expected) == {PREFIX + name for name in NAMES},
            'Static payload inventory differs from the six 86 assemblies')
    values = {}
    for name in NAMES:
        path = (overlay/name).resolve()
        require(path.parent == overlay and path.is_file(), 'Static DLL missing or outside overlay: ' + name)
        digest = file_sha(path)
        require(re.fullmatch(r'[0-9a-f]{64}', str(expected.get(PREFIX + name, ''))) is not None
                and digest == expected[PREFIX + name], 'Static DLL SHA differs from proof: ' + name)
        values[name] = digest
    return proof_path, values


def version_key(path):
    parts = path.name.split('.')
    require(all(part.isdecimal() for part in parts), 'Unsupported runtime version: ' + path.name)
    return tuple(int(part) for part in parts)


def runtime_files(runtime):
    runtime = Path(runtime).resolve()
    dotnet = runtime/'dotnet/dotnet'
    dnlib = runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    sdk_root = runtime/'dotnet/sdk'
    compilers = sorted((p for p in sdk_root.iterdir() if p.is_dir() and p.name.startswith('8.')
                        and (p/'Roslyn/bincore/csc.dll').is_file()), key=version_key)
    pack = runtime/'dotnet/packs/Microsoft.NETCore.App.Ref'
    references = sorted((p for p in pack.iterdir() if p.is_dir() and p.name.startswith('8.')
                         and (p/'ref/net8.0').is_dir()), key=version_key)
    require(dotnet.is_file() and dnlib.is_file() and compilers and references,
            'Expected local .NET 8 SDK, reference pack, and dnlib runtime')
    reference_paths = sorted((references[-1]/'ref/net8.0').glob('*.dll'))
    require(reference_paths, 'The selected .NET 8 reference pack is empty')
    return dotnet, dnlib, compilers[-1]/'Roslyn/bincore/csc.dll', reference_paths


def run(command, cwd, timeout):
    environment = dict(os.environ, DOTNET_ROOT=str(Path(command[0]).parent), DOTNET_MULTILEVEL_LOOKUP='0',
                       DOTNET_CLI_TELEMETRY_OPTOUT='1', DOTNET_PROCESSOR_COUNT='2')
    return subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=cwd, env=environment,
                          capture_output=True, text=True, timeout=timeout)


def extract_managed(archive, entries, original, final):
    """Store original DLLs once and use owned hard links for unchanged inputs."""
    original.mkdir()
    final.mkdir()
    hashes = {}
    for name, entry in entries.items():
        target = original/name
        digest = hashlib.sha256()
        with archive.open(entry) as source, target.open('xb') as output:
            for block in iter(lambda: source.read(1024*1024), b''):
                output.write(block)
                digest.update(block)
        hashes[name] = digest.hexdigest()
        try:
            os.link(target, final/name)
        except OSError:
            shutil.copyfile(target, final/name)
    return hashes


def compare_original_and_reference_qa(base, reference):
    """Compare actual inherited runtime components; the QA package stays QA-only."""
    require(file_sha(reference) == REFERENCE_QA83_SHA, 'Historical QA APK SHA differs')
    rows = []
    with zipfile.ZipFile(base) as original, zipfile.ZipFile(reference) as qa:
        for entry in (*MONO_COMPONENTS, *(PREFIX + name for name in FRAMEWORK_COMPONENTS)):
            require(original.namelist().count(entry) == qa.namelist().count(entry) == 1,
                    'Missing or duplicate historical runtime component: ' + entry)
            left, right = original.read(entry), qa.read(entry)
            require(left == right, 'Historical QA runtime component differs from v80: ' + entry)
            rows.append({'entry': entry, 'v80SHA256': hashlib.sha256(left).hexdigest(),
                         'qa83SHA256': hashlib.sha256(right).hexdigest(), 'v80AndQA83ByteEqual': True})
    return rows


def check_result(details, expected_hashes):
    """The checker must report the exact inventory that was supplied to it."""
    require(details.get('schema') == 'hyjs-combined86-linkage-result-v1', 'Unexpected verifier result schema')
    inventory = details.get('inventory')
    require(isinstance(inventory, list), 'Verifier inventory missing')
    actual = {}
    for row in inventory:
        require(isinstance(row, dict) and isinstance(row.get('file'), str) and row['file'] not in actual
                and isinstance(row.get('identity'), str) and row['identity'], 'Invalid verifier inventory row')
        actual[row['file']] = row.get('sha256')
    require(actual == expected_hashes, 'Verifier inventory does not match actual APK and static DLLs')
    require(details.get('roots') == [Path(name).stem for name in NAMES], 'Verifier root inventory differs')
    require(isinstance(details.get('problems'), list), 'Verifier problems missing')


def execute(args):
    base, overlay, output = (Path(x).resolve() for x in (args.base_apk, args.overlay, args.output))
    require(base.is_relative_to(ROOT) and base.is_file(), 'Base APK must be inside the project')
    require(output.is_relative_to(ROOT) and output.suffix == '.json' and not output.exists(),
            'Fresh JSON output inside the project required')
    require(file_sha(base) == BASE_SHA, 'Actual base APK SHA differs from pinned v80')
    qa83 = Path(args.reference_qa_apk).resolve()
    require(qa83.is_file(), 'Historical QA APK is unavailable')
    component_comparison = compare_original_and_reference_qa(base, qa83)
    proof_path, payloads = checked_overlay(overlay, BASE_SHA)
    dotnet, dnlib, compiler, references = runtime_files(args.runtime)
    source = HERE/'VerifyLinkage86.cs'
    inputs = [base, qa83, proof_path, source, Path(__file__).resolve(), dotnet, dnlib, compiler, *references]
    inputs.extend(overlay/name for name in NAMES)
    pins = {str(path): file_sha(path) for path in inputs}
    require(pins[str(base)] == BASE_SHA and pins[str(proof_path)] == file_sha(proof_path), 'Input pin differs')
    output.parent.mkdir(parents=True, exist_ok=True)
    report = {'schema': 'hyjs-combined86-linkage-proof-v1', 'status': 'LINKAGE_BLOCKED',
              'baseAPKSHA256': BASE_SHA, 'staticProof': str(proof_path.relative_to(ROOT)),
              'referenceQA83APK': str(qa83), 'referenceQA83APKSHA256': REFERENCE_QA83_SHA,
              'referenceQA83Use': 'read-only historical component comparison; never release input',
              'staticProofSHA256': pins[str(proof_path)], 'overlay': str(overlay.relative_to(ROOT)),
              'payloadSHA256': {PREFIX + name: payloads[name] for name in NAMES},
              'verifierSourceSHA256': pins[str(source)], 'wrapperSourceSHA256': pins[str(Path(__file__).resolve())],
              'toolchainSHA256': {str(path): pins[str(path)] for path in (dotnet, dnlib, compiler, *references)},
              'unityExecuted': False, 'deviceExecuted': False, 'serverExecuted': False,
              'businessReviewRecorded': False,
              'inheritedRuntimeComponentComparison': component_comparison,
              'signed86RuntimeComponentComparison': 'not performed; no signed 86 APK input'}
    try:
        work_parent = ROOT/'build'
        require(work_parent.is_dir(), 'Build temporary parent is missing')
        with tempfile.TemporaryDirectory(prefix='hyjs-linkage86-', dir=work_parent) as temporary:
            work = Path(temporary)
            original, final = work/'original', work/'final'
            with zipfile.ZipFile(base) as archive:
                entries = managed_entries(archive)
                original_hashes = extract_managed(archive, entries, original, final)
            for name in NAMES:
                target = final/name
                if target.exists():
                    target.unlink()  # Keep the original hard link unchanged.
                shutil.copyfile(overlay/name, target)
                require(file_sha(target) == payloads[name], 'Copied static DLL differs: ' + name)
            executable = work/'VerifyLinkage86.dll'
            shutil.copyfile(dnlib, work/'dnlib.dll')
            require(file_sha(work/'dnlib.dll') == pins[str(dnlib)], 'Copied dnlib differs')
            (work/'VerifyLinkage86.runtimeconfig.json').write_text(json.dumps({'runtimeOptions': {
                'tfm': 'net8.0', 'framework': {'name': 'Microsoft.NETCore.App', 'version': '8.0.0'},
                'rollForward': 'LatestPatch'}}), encoding='utf-8')
            result = run([dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:latest',
                          '/deterministic+', '/out:' + str(executable), '/reference:' + str(dnlib),
                          *['/reference:' + str(path) for path in references], source], work, 180)
            if result.returncode != 0:
                raise ValueError('Verifier compilation failed: ' + (result.stdout + result.stderr)[-12000:])
            report['verifierBinarySHA256'] = file_sha(executable)
            fixture = run([dotnet, executable, '--self-test'], work, 30)
            require(fixture.returncode == 0, 'Framework policy fixture failed: ' + (fixture.stdout + fixture.stderr)[-4000:])
            policy = json.loads(fixture.stdout)
            require(policy == {'schema': 'hyjs-linkage86-framework-policy-fixture-v1', 'checks': 35},
                    'Unexpected framework policy fixture results')
            report['frameworkPolicyFixture'] = policy
            result = run([dotnet, executable, original, final], work, 600)
            try:
                details = json.loads(result.stdout)
            except json.JSONDecodeError as error:
                raise ValueError('Verifier did not return a linkage report: ' +
                                 (result.stdout + result.stderr)[-12000:]) from error
            report['linkage'] = details
            report['verifierExitCode'] = result.returncode
            check_result(details, {**original_hashes, **payloads})
            for row in component_comparison:
                if row['entry'].startswith(PREFIX):
                    name = row['entry'][len(PREFIX):]
                    require(original_hashes[name] == row['v80SHA256'] and name not in payloads,
                            '86 composed inherited component differs: ' + name)
                    row['composed86ManagedSHA256'] = original_hashes[name]
                    row['v80AndComposed86ManagedByteEqual'] = True
                else:
                    row['composed86ManagedSHA256'] = None
                    row['signed86RuntimeChecked'] = False
            require(result.returncode == 0 and
                    details.get('status') == 'LINKAGE_STATIC_NO_NEW_ERRORS_WITH_BASELINE_MISMATCHES'
                    and details.get('strictAssemblyIdentityClosure') is False
                    and details.get('runtimeResolutionProven') is False
                    and len(details.get('preservedOriginalFrameworkIdentityMismatches', [])) == 5
                    and not details['problems'],
                    'The final assembly linkage is blocked; see linkage.problems')
            for path, digest in pins.items():
                require(file_sha(path) == digest, 'Input changed during linkage verification: ' + path)
            require(checked_overlay(overlay, BASE_SHA)[1] == payloads, 'Static output changed during verification')
            report['status'] = 'LINKAGE_STATIC_NO_NEW_ERRORS_WITH_BASELINE_MISMATCHES'
    except (ValueError, OSError, KeyError, subprocess.SubprocessError, json.JSONDecodeError) as error:
        report['error'] = str(error)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        raise
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'status': report['status'], 'output': str(output),
                      'baseAPKSHA256': report['baseAPKSHA256'], 'deviceExecuted': False}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-apk', 'overlay', 'runtime', 'output'):
        parser.add_argument('--' + name, required=True, type=Path)
    parser.add_argument('--reference-qa-apk', required=True, type=Path,
                        help='Exact 1.0.83 r3 QA APK for read-only historical component comparison')
    execute(parser.parse_args())


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, KeyError, subprocess.SubprocessError, json.JSONDecodeError) as error:
        print('86 linkage verification blocked: ' + str(error), file=sys.stderr)
        sys.exit(1)
