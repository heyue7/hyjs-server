"""Build the fixed 1.1.0 production candidate without changing current APKs."""
from __future__ import annotations

import copy
import fcntl
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import time
import zipfile
from xml.sax.saxutils import escape

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools import build_wallet_client as common
from tools.awake_diff_rows.build import methods, operand
from tools.client_branding.branding import chunks, elements, strings_from_pool
from tools.client_branding.package_release import compressed_hash, copy_original_record
from tools.client_remote.config import validate_origin
from tools.client_update.package import patch_manifest
from tools.local_asset_ready.remove_gate import exact_body

SOURCE = Path('/Users/lirui/LR/hyjs')
BASE = SOURCE / 'build/黑衣剑士灵魂之花v1.0.91.apk'
BASE_SHA = 'b1e62a8ed9c7438391c34783e0bf83aa5c546f0840f843c886df08ba12d84a99'
KEYSTORE = SOURCE / '.local-signing/debug.keystore'
LOCK = SOURCE / 'build/.client-build.lock'
RUNTIME = SOURCE / '.tools/native-batch'
SDK = Path.home() / 'Library/Android/sdk/build-tools/33.0.0'
JAVA = Path('/opt/homebrew/opt/openjdk@21')
OUTPUT = ROOT / 'deliveries/formal110/黑衣剑士灵魂之花v1.1.0.apk'
RECORD = ROOT / 'analysis/formal110/manifest.json'
FAILED = ROOT / 'analysis/formal110/failure.json'
DLL = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
WALLET = 'assets/local-wallet.json'
MANIFEST = 'AndroidManifest.xml'
BUILD_CODE = 2026110101
VERSION_NAME = '1.1.0'
EXPECTED_CERT = 'ada7962ab32b38404b022c3c091ae0311e5741fd020ca0cc653c0aaf716d7c29'
TARGETS = (
    'ServerConfig::GetServerUrl',
    'Battle.FieldPVEWorldBossBattleOver::End',
    'Room.RoomNetField::SCCurrentPosition',
    'VoicePlugin::InitGVoice',
)
SOURCE_FILES = (
    Path(__file__), Path(__file__).with_name('Injector.cs'),
    ROOT / 'tools/client_update/package.py',
    ROOT / 'tools/client_branding/package_release.py',
    ROOT / 'tools/client_branding/branding.py',
    ROOT / 'tools/client_remote/config.py',
    ROOT / 'tools/awake_diff_rows/build.py',
    ROOT / 'tools/local_asset_ready/remove_gate.py',
    ROOT / 'tools/build_wallet_client.py',
)


def require(condition, message):
    if not condition:
        raise ValueError(message)


def file_sha(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def byte_sha(value):
    return hashlib.sha256(value).hexdigest()


def source_pins():
    return {str(path.relative_to(ROOT)): file_sha(path) for path in SOURCE_FILES}


def run(label, command, *, env=None, timeout=240):
    result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, env=env,
                            capture_output=True, text=True, timeout=timeout)
    if result.returncode:
        # Neither command text nor subprocess output is emitted: these may carry
        # local configuration, addresses, or signing-tool diagnostics.
        raise RuntimeError(f'{label} failed (exit {result.returncode}; diagnostic SHA256 '
                           f'{byte_sha((result.stdout + result.stderr).encode())})')
    return result.stdout


def target_origin(env):
    result = run('SSH configuration lookup', ['ssh', '-G', 'ubuntu@heyue-cn-1'], env=env, timeout=15)
    hosts = [line.split(None, 1)[1] for line in result.splitlines()
             if line.lower().startswith('hostname ')]
    require(len(hosts) == 1, 'SSH alias must resolve one hostname')
    try:
        address = ipaddress.ip_address(hosts[0])
    except ValueError as error:
        raise ValueError('SSH alias must resolve a public IP literal for this release') from error
    require(address.version == 4 and address.is_global, 'SSH alias must resolve a public IPv4 address')
    return validate_origin(f'http://{address}:28861')


def build_identity(apk, env):
    output = run('aapt2 identity', [SDK / 'aapt2', 'dump', 'badging', apk], env=env)
    package = next((line for line in output.splitlines() if line.startswith('package:')), '')
    launcher = next((line for line in output.splitlines() if line.startswith('launchable-activity:')), '')
    def field(line, key):
        match = re.search(r"(?:^|\s)" + re.escape(key) + r"='([^']*)'", line)
        require(match is not None, 'APK identity field is missing: ' + key)
        return match.group(1)
    return {'package': field(package, 'name'), 'versionCode': int(field(package, 'versionCode')),
            'versionName': field(package, 'versionName'), 'launcher': field(launcher, 'name')}


def signer(apk, env):
    output = run('APK signature verification',
                 [SDK / 'apksigner', 'verify', '--verbose', '--print-certs', apk], env=env)
    return common._certificate(output)


def manifest_version_only(original):
    revised = patch_manifest(original, BUILD_CODE, VERSION_NAME)
    old_parts, new_parts = chunks(original), chunks(revised)
    require(len(old_parts) == len(new_parts), 'Manifest chunk inventory changed')
    old_pools = [part for part in old_parts if part[0] == 1]
    new_pools = [part for part in new_parts if part[0] == 1]
    require(len(old_pools) == len(new_pools) == 1, 'Expected one manifest string pool')
    old_strings, _ = strings_from_pool(old_pools[0][2])
    new_strings, _ = strings_from_pool(new_pools[0][2])
    require(new_strings[:len(old_strings)] == old_strings, 'Inherited manifest string indices changed')
    changed = []
    for index, (before, after) in enumerate(zip(old_parts, new_parts)):
        require(before[:2] == after[:2], 'Manifest chunk identity changed')
        if before == after:
            continue
        kind, _, raw = before
        tag = old_strings[struct.unpack_from('<I', raw, 20)[0]] if kind == 0x102 else None
        require(kind == 1 or (kind == 0x102 and tag == 'manifest'),
                'Manifest changed outside version attributes/string pool')
        changed.append(index)
    require(len(changed) == 2, 'Expected only manifest and string-pool chunks to change')
    before_elements = elements(old_parts, old_strings)
    expected = []
    count_code = count_name = 0
    for tag, attributes in before_elements:
        row = []
        for namespace, key, kind, value in attributes:
            if tag == 'manifest' and key == 'versionCode':
                kind, value = 16, BUILD_CODE
                count_code += 1
            elif tag == 'manifest' and key == 'versionName':
                kind, value = 3, VERSION_NAME
                count_name += 1
            row.append((namespace, key, kind, value))
        expected.append((tag, row))
    require(count_code == count_name == 1 and elements(new_parts, new_strings) == expected,
            'Manifest fields other than the two version attributes changed')
    return revised


def verify_managed(before, after, inherited, target):
    require(inherited == validate_origin(inherited) and target == validate_origin(target),
            'Origin is not canonical')
    require(inherited != target, 'Target equals inherited origin')
    targets = {name: ((inherited + '/sao/', target + '/sao/') if i == 0 else (inherited, target))
               for i, name in enumerate(TARGETS)}
    left_pe, right_pe = [dnfile.dnPE(data=raw) for raw in (before, after)]
    left, right = methods(left_pe), methods(right_pe)
    require(len(left) == len(right), 'Managed method inventory changed')
    for table in ('Field', 'FieldPtr', 'MethodPtr', 'TypeDef', 'TypeRef', 'MemberRef', 'AssemblyRef'):
        a, b = getattr(left_pe.net.mdtables, table, None), getattr(right_pe.net.mdtables, table, None)
        require((len(a.rows) if a else 0) == (len(b.rows) if b else 0), 'Metadata count changed: ' + table)
    require(not getattr(right_pe.net.mdtables, 'FieldPtr', None)
            and not getattr(right_pe.net.mdtables, 'MethodPtr', None), 'Unity pointer tables are forbidden')
    unchanged = engine = 0
    changes = []
    for (name, rid, old), (other, oid, new) in zip(left, right):
        require((name, rid) == (other, oid) and old.Signature.value == new.Signature.value
                and old.struct.Flags == new.struct.Flags and old.struct.ImplFlags == new.struct.ImplFlags,
                'Managed ABI changed: ' + name)
        if not old.Rva:
            require(not new.Rva, 'Unexpected managed body: ' + name)
            continue
        x, y = exact_body(left_pe, old), exact_body(right_pe, new)
        if name not in targets:
            require(x == y, 'Unrelated managed body changed: ' + name)
            unchanged += 1
            engine += name.startswith('Battle.Engine')
            continue
        a, b = [read_method_body_from_bytes(raw) for raw in (x, y)]
        require(len(x) == len(y) and len(a.instructions) == len(b.instructions)
                and getattr(a.local_var_sig_tok, 'value', None) == getattr(b.local_var_sig_tok, 'value', None)
                and a.max_stack == b.max_stack, 'Controlled method contract changed: ' + name)
        changed = 0
        for first, second in zip(a.instructions, b.instructions):
            require((first.offset, first.opcode.name) == (second.offset, second.opcode.name),
                    'Opcode/layout changed: ' + name)
            if first.opcode.name == 'ldstr':
                old_value = left_pe.net.user_strings.get(first.operand.value & 0xffffff).value
                new_value = right_pe.net.user_strings.get(second.operand.value & 0xffffff).value
                if old_value == targets[name][0]:
                    require(new_value == targets[name][1], 'Controlled origin mismatch')
                    changed += 1
                else:
                    require(old_value == new_value and operand(first.operand) == operand(second.operand),
                            'Unrelated string changed: ' + name)
            else:
                require(operand(first.operand) == operand(second.operand), 'Unrelated operand changed: ' + name)
        require(changed == 1, 'Expected exactly one changed origin literal: ' + name)
        def regions(body):
            return [(h.exception_type, h.try_start, h.try_end, h.handler_start, h.handler_end,
                     h.filter_start, getattr(h.catch_type, 'value', None)) for h in body.exception_handlers]
        require(regions(a) == regions(b), 'Exception regions changed: ' + name)
        changes.append({'method': name, 'beforeSHA256': byte_sha(x), 'afterSHA256': byte_sha(y)})
    require(engine == 547 and len(changes) == 4, 'Four origins or 547-engine guard failed')
    for _, _, method in right:
        if method.Rva:
            for instruction in read_method_body_from_bytes(exact_body(right_pe, method)).instructions:
                if instruction.opcode.name == 'ldstr':
                    value = right_pe.net.user_strings.get(instruction.operand.value & 0xffffff).value
                    require(inherited not in value, 'Inherited origin remains in executable IL')
    return {'originalEngineMethodsUnchanged': engine, 'unchangedInheritedRawMethodBodies': unchanged,
            'allOtherMethodsExact': True, 'abiAndMetadataCountsPreserved': True,
            'oldOriginAbsentFromExecutableIL': True, 'changes': changes}


def entry_digest(archive, info, needles=()):
    digest = hashlib.sha256()
    overlap = max((len(needle) for needle in needles), default=0)
    previous = b''
    found = False
    with archive.open(info) as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            digest.update(block)
            joined = previous + block
            found |= any(needle in joined for needle in needles)
            previous = joined[-overlap:] if overlap else b''
    return digest.hexdigest(), found


def compile_injector(folder, env):
    runtime = RUNTIME / 'dotnet/dotnet'
    dnlib = RUNTIME / 'injector/bin/Debug/net8.0/dnlib.dll'
    require(runtime.is_file() and dnlib.is_file(), 'Existing .NET/dnlib runtime is missing')
    shutil.copy2(Path(__file__).with_name('Injector.cs'), folder / 'Program.cs')
    project = folder / 'Patch.csproj'
    project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                       '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                       '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib">'
                       '<HintPath>' + escape(str(dnlib)) + '</HintPath></Reference></ItemGroup></Project>')
    config = folder / 'NuGet.Config'
    config.write_text('<configuration><packageSources><clear /></packageSources></configuration>')
    run('offline .NET restore', [runtime, 'restore', project, '--configfile', config,
        '--ignore-failed-sources', '--disable-parallel', '--verbosity', 'quiet', '-p:NuGetAudit=false'], env=env)
    run('injector build', [runtime, 'build', project, '--no-restore', '--nologo', '--verbosity', 'quiet',
        '-p:NuGetAudit=false', '-p:UseSharedCompilation=false'], env=env)
    return runtime, folder / 'bin/Debug/net8.0/Patch.dll', dnlib


def protect_current():
    return {str(path): file_sha(path) if path.is_file() else None
            for path in (SOURCE / 'build/hyjs-local.apk', SOURCE / 'build/hyjs-rollback.apk')}


def build():
    phase = 'preflight'
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    RECORD.parent.mkdir(parents=True, exist_ok=True)
    require(not OUTPUT.exists() and not RECORD.exists() and not FAILED.exists(),
            'Formal output/evidence already exists; preserve it and inspect before retry')
    require(BASE.is_file() and not BASE.is_symlink() and file_sha(BASE) == BASE_SHA, 'Base APK differs')
    require(KEYSTORE.is_file() and not KEYSTORE.is_symlink(), 'Protected signing key is missing')
    require(LOCK.is_file(), 'Shared client-build lock is missing')
    for executable in (SDK / 'aapt2', SDK / 'apksigner', SDK / 'zipalign', JAVA / 'bin/java'):
        require(executable.is_file(), 'Required Android/JDK tool is missing: ' + executable.name)
    require(os.environ.get('HYJS_FORMAL110_KEYSTORE_PASSWORD'), 'Signing password environment variable is missing')
    minimum_free = BASE.stat().st_size * 4 + 1_000_000_000
    available = shutil.disk_usage(OUTPUT.parent).free
    require(available > minimum_free, 'Insufficient packaging peak disk space')
    signing_password = os.environ.get('HYJS_FORMAL110_KEYSTORE_PASSWORD')
    env = dict(os.environ, JAVA_HOME=str(JAVA), PATH=str(JAVA / 'bin') + os.pathsep + os.environ.get('PATH', ''))
    env.pop('HYJS_FORMAL110_KEYSTORE_PASSWORD', None)
    sign_env = dict(env, HYJS_FORMAL110_KEYSTORE_PASSWORD=signing_password)
    target = target_origin(env)
    pins = source_pins()
    protected = protect_current()
    require(protected[str(SOURCE / 'build/hyjs-local.apk')] == BASE_SHA, 'Current APK differs from fixed base')
    created_output = False
    started = time.monotonic()
    try:
        with LOCK.open('a') as lock:
            deadline = time.monotonic() + 600
            while True:
                try:
                    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    break
                except BlockingIOError:
                    require(time.monotonic() < deadline, 'Timed out waiting for shared client-build lock')
                    time.sleep(1)
            require(file_sha(BASE) == BASE_SHA and protect_current() == protected, 'Protected APK changed while waiting')
            require(shutil.disk_usage(OUTPUT.parent).free > minimum_free, 'Packaging disk capacity changed')
            require(not OUTPUT.exists() and not RECORD.exists(), 'Output appeared while waiting for lock')
            phase = 'verify-base'
            print('Verifying fixed base APK and signing identity', flush=True)
            base_identity = build_identity(BASE, env)
            require((base_identity['versionCode'], base_identity['versionName']) == (2026103105, '1.0.91'),
                    'Base manifest version differs')
            base_signer = signer(BASE, env)
            require(base_signer == EXPECTED_CERT, 'Base certificate differs from recorded formal identity')
            run('base zip alignment', [SDK / 'zipalign', '-c', '-p', '4', BASE], env=env)
            with tempfile.TemporaryDirectory(prefix='.formal110-', dir=OUTPUT.parent) as temporary:
                folder = Path(temporary)
                process_env = dict(env, DOTNET_CLI_HOME=str(folder / 'dotnet-home'),
                                   DOTNET_SKIP_FIRST_TIME_EXPERIENCE='1', DOTNET_CLI_TELEMETRY_OPTOUT='1',
                                   DOTNET_CLI_WORKLOAD_UPDATE_NOTIFY_DISABLE='true',
                                   NUGET_PACKAGES=str(folder / 'nuget'), TMPDIR=str(folder))
                with zipfile.ZipFile(BASE) as archive:
                    names = archive.namelist()
                    require(len(names) == len(set(names)) and all(n in names for n in (DLL, WALLET, MANIFEST)),
                            'Base APK has duplicate or missing entries')
                    old_main, old_wallet, old_manifest = [archive.read(n) for n in (DLL, WALLET, MANIFEST)]
                wallet = json.loads(old_wallet)
                require(isinstance(wallet, dict) and set(wallet) == {'origin'}, 'Unexpected wallet JSON contract')
                inherited = validate_origin(wallet['origin'])
                require(wallet['origin'] == inherited and inherited != target, 'Base origin is not suitable')
                phase = 'compile-and-verify-origin'
                print('Building the scoped origin overlay and auditing managed methods', flush=True)
                runtime, injector, dnlib = compile_injector(folder, process_env)
                main_input, main_output = folder / 'input.dll', folder / 'output.dll'
                main_input.write_bytes(old_main)
                injector_env = dict(process_env, HYJS_FORMAL110_INHERITED_ORIGIN=inherited,
                                    HYJS_FORMAL110_TARGET_ORIGIN=target)
                run('origin injector', [runtime, injector, main_input, main_output], env=injector_env)
                new_main = main_output.read_bytes()
                managed = verify_managed(old_main, new_main, inherited, target)
                new_wallet = (json.dumps({'origin': target}, separators=(',', ':')) + '\n').encode()
                new_manifest = manifest_version_only(old_manifest)
                replacements = {DLL: new_main, WALLET: new_wallet, MANIFEST: new_manifest}
                replacement_hashes = {name: byte_sha(raw) for name, raw in replacements.items()}
                before_hashes = {DLL: byte_sha(old_main), WALLET: byte_sha(old_wallet), MANIFEST: byte_sha(old_manifest)}
                phase = 'copy-apk'
                print('Preserving and comparing inherited APK entries', flush=True)
                unsigned, aligned, signed = [folder / name for name in ('unsigned.apk', 'aligned.apk', 'signed.apk')]
                inherited_entries = {}
                old_literals = (inherited.encode(), inherited.encode('utf-16le'))
                with zipfile.ZipFile(BASE) as incoming, BASE.open('rb') as source, zipfile.ZipFile(unsigned, 'w') as out:
                    out.comment = incoming.comment
                    for info in incoming.infolist():
                        if common._signature(info.filename):
                            continue
                        if info.filename in replacements:
                            out.writestr(copy.copy(info), replacements[info.filename])
                            continue
                        digest, found = entry_digest(incoming, info, old_literals)
                        require(not found, 'Inherited origin remains in another APK entry: ' + info.filename)
                        inherited_entries[info.filename] = {
                            'sha256': digest, 'compressedSHA256': compressed_hash(source, info),
                            'metadata': (info.compress_type, info.compress_size, info.file_size, info.CRC,
                                         info.flag_bits, info.external_attr, info.internal_attr, info.date_time)}
                        copy_original_record(source, out, info)
                phase = 'align-and-sign'
                print('Aligning and signing with the existing identity', flush=True)
                run('zip alignment', [SDK / 'zipalign', '-f', '-p', '4', unsigned, aligned], env=env)
                unsigned.unlink()
                run('APK signing', [SDK / 'apksigner', 'sign', '--ks', KEYSTORE,
                    '--ks-pass', 'env:HYJS_FORMAL110_KEYSTORE_PASSWORD',
                    '--key-pass', 'env:HYJS_FORMAL110_KEYSTORE_PASSWORD', '--out', signed, aligned], env=sign_env)
                aligned.unlink()
                require(signer(signed, env) == base_signer, 'Candidate signing identity differs from base')
                run('signed APK alignment', [SDK / 'zipalign', '-c', '-p', '4', signed], env=env)
                identity = build_identity(signed, env)
                require(identity == dict(base_identity, versionCode=BUILD_CODE, versionName=VERSION_NAME),
                        'Candidate package, launcher, or version differs')
                phase = 'verify-final-apk'
                print('Verifying every inherited entry and compressed payload', flush=True)
                inventory = hashlib.sha256()
                with zipfile.ZipFile(signed) as actual, signed.open('rb') as stream:
                    names = actual.namelist()
                    require(len(names) == len(set(names)), 'Candidate has duplicate ZIP entries')
                    require({name for name in names if not common._signature(name)} == set(inherited_entries) | set(replacements),
                            'Candidate entry inventory changed')
                    for name, expected in inherited_entries.items():
                        info = actual.getinfo(name)
                        metadata = (info.compress_type, info.compress_size, info.file_size, info.CRC,
                                    info.flag_bits, info.external_attr, info.internal_attr, info.date_time)
                        require(metadata == expected['metadata'], 'Inherited ZIP metadata changed: ' + name)
                        digest, _ = entry_digest(actual, info)
                        require(digest == expected['sha256'] and compressed_hash(stream, info) == expected['compressedSHA256'],
                                'Inherited content or compressed payload changed: ' + name)
                        inventory.update((name + '\0' + digest + '\0' + expected['compressedSHA256'] + '\n').encode())
                    for name, digest in replacement_hashes.items():
                        require(entry_digest(actual, actual.getinfo(name))[0] == digest, 'Replacement changed: ' + name)
                    require(json.loads(actual.read(WALLET)) == {'origin': target}, 'Final wallet origin differs')
                    require(actual.read(DLL) == new_main and actual.read(MANIFEST) == new_manifest,
                            'Final managed assembly or manifest differs')
                require(file_sha(BASE) == BASE_SHA and protect_current() == protected, 'Protected APK changed during build')
                require(source_pins() == pins, 'A pinned release source changed during build')
                candidate_sha = file_sha(signed)
                proof = {'status': 'SIGNED_ALIGNED_FORMAL110_CANDIDATE_NOT_INSTALLED',
                    'baseAPK': str(BASE), 'baseAPKSHA256': BASE_SHA, 'outputAPK': str(OUTPUT),
                    'binarySHA256': candidate_sha, 'identity': identity, 'baseIdentity': base_identity,
                    'signerCertificateSHA256': base_signer, 'signatureVerified': True, 'zipalignVerified': True,
                    'targetOriginSHA256': byte_sha(target.encode()), 'targetResolvedFrom': 'ssh -G ubuntu@heyue-cn-1',
                    'targetHTTPPort': 28861, 'managedAudit': managed, 'beforeSHA256': before_hashes,
                    'replacementEntries': replacement_hashes, 'onlyManifestVersionAttributesChanged': True,
                    'inheritedEntryCount': len(inherited_entries), 'inheritedEntryInventorySHA256': inventory.hexdigest(),
                    'allInheritedEntryBytesAndCompressedPayloadsExact': True,
                    'oldOriginAbsentFromOtherInheritedEntriesASCIIAndUTF16': True,
                    'currentAndRollbackUnchanged': True, 'sourcePins': pins,
                    'runtimeSHA256': {'dotnet': file_sha(runtime), 'dnlib': file_sha(dnlib)},
                    'availableBytesAtPreflight': available, 'minimumRequiredBytes': minimum_free,
                    'elapsedSeconds': round(time.monotonic() - started, 3), 'installed': False, 'promoted': False,
                    'serverContacted': False, 'phoneTested': False}
                phase = 'publish-local-candidate'
                evidence = folder / 'manifest.json'
                evidence.write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
                os.link(signed, OUTPUT)
                created_output = True
                os.link(evidence, RECORD)
                print(json.dumps({'status': proof['status'], 'binarySHA256': candidate_sha,
                                  'outputAPK': str(OUTPUT), 'evidence': str(RECORD)}, ensure_ascii=False), flush=True)
    except Exception as error:
        if created_output:
            OUTPUT.unlink(missing_ok=True)
        if not RECORD.exists() and not FAILED.exists():
            FAILED.write_text(json.dumps({'status': 'FAILED', 'phase': phase,
                'errorType': type(error).__name__, 'message': str(error)}, ensure_ascii=False, indent=2) + '\n')
        raise


if __name__ == '__main__':
    try:
        build()
    except Exception as error:
        if not OUTPUT.exists() and not RECORD.exists() and not FAILED.exists():
            FAILED.parent.mkdir(parents=True, exist_ok=True)
            FAILED.write_text(json.dumps({'status': 'FAILED', 'phase': 'preflight',
                'errorType': type(error).__name__, 'message': str(error)}, ensure_ascii=False, indent=2) + '\n')
        print(f'Formal 1.1.0 build failed: {type(error).__name__}: {error}', file=sys.stderr)
        raise SystemExit(1)
