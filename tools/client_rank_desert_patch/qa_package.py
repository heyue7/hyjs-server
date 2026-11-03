"""Build the isolated multi-package phone QA APK from the pinned 1.1.9 candidate."""
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools import build_wallet_client as common
from tools.awake_diff_rows.build import methods
from tools.client_branding.package_release import compressed_hash, copy_original_record, require
from tools.client_qa75.manifest import patch_package
from tools.client_remote import build as remote
from tools.client_rank_desert_patch.build import (
    BOSS_GETTER, HOOK_TOKEN, MONSTER_GETTER, RESULT_CALL, TARGET, is_call, verify_metadata_tokens,
)
from tools.local_asset_ready.remove_gate import exact_body

BASE = ROOT / 'build/rank-desert-fixes-20261103/黑衣剑士灵魂之花v1.1.9-修复候选.apk'
BASE_SHA = 'cd86472be5572ec63fa22eb76a12ab2589f775dc8483e76350ed01fe7aa4a118'
OUTPUT_DIRECTORY = ROOT / 'build/rank-desert-fixes-20261103/phone-qa'
OUTPUT_NAME = '黑衣剑士灵魂之花v1.1.9-手机验收-multi.apk'
RECORD = ROOT / 'analysis/rank-desert-fixes-20261103/phone/qa-package.json'
ORIGIN = 'http://127.0.0.1:28979'
PACKAGE = 'com.yunchang.djsy.ewan.multi'
DLL = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
HELPER = 'assets/bin/Data/Managed/SoulFlowerDesertV4.dll'
WALLET = 'assets/local-wallet.json'
MANIFEST = 'AndroidManifest.xml'
INJECTOR = ROOT / 'tools/client_recycle_selection/QAInjector.cs'
RUNTIME = Path('/Users/lirui/LR/hyjs/.tools/native-batch')
SDK = Path('/Users/lirui/Library/Android/sdk/build-tools/33.0.0')
JAVA = Path('/opt/homebrew/opt/openjdk@21')


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def unique(items, label):
    found = list(items)
    require(len(found) == 1, 'Expected one local ' + label + '; found ' + str(len(found)))
    return found[0]


def read_unique(archive, name):
    require(archive.namelist().count(name) == 1, 'Missing or duplicate candidate entry: ' + name)
    return archive.read(name)


def object_without_duplicates(pairs):
    result = {}
    for name, value in pairs:
        require(name not in result, 'Duplicate wallet JSON key')
        result[name] = value
    return result


def patch_wallet(raw):
    wallet = json.loads(raw, object_pairs_hook=object_without_duplicates)
    require(isinstance(wallet, dict) and 'origin' in wallet, 'Wallet origin is missing')
    old = remote.validate_origin(wallet['origin'])
    require(wallet['origin'] == old and old != ORIGIN, 'Candidate wallet origin is invalid or already QA')
    before = json.dumps(old, ensure_ascii=False).encode('utf-8')
    after = json.dumps(ORIGIN, ensure_ascii=False).encode('utf-8')
    require(raw.count(before) == 1, 'Wallet origin is not a unique literal')
    patched = raw.replace(before, after, 1)
    expected = dict(wallet)
    expected['origin'] = ORIGIN
    require(json.loads(patched, object_pairs_hook=object_without_duplicates) == expected,
            'Wallet fields other than origin changed')
    return old, patched


def require_old_sites(raw, targets):
    pe = dnfile.dnPE(data=raw)
    found = set()
    for name, _, row in methods(pe):
        if name not in targets:
            continue
        require(name not in found and row.Rva, 'Duplicate or bodyless origin target: ' + name)
        body = read_method_body_from_bytes(exact_body(pe, row))
        literals = [pe.net.user_strings.get(ins.operand.value & 0xFFFFFF).value
                    for ins in body.instructions if ins.opcode.name == 'ldstr']
        require(literals.count(targets[name][0]) == 1, 'Expected candidate origin missing: ' + name)
        found.add(name)
    require(found == set(targets), 'Candidate origin method inventory differs')


def verify_boss_display(before, after):
    left, right = (dnfile.dnPE(data=raw) for raw in (before, after))
    old = [row for name, _, row in methods(left) if name == TARGET]
    new = [row for name, _, row in methods(right) if name == TARGET]
    require(len(old) == len(new) == 1 and exact_body(left, old[0]) == exact_body(right, new[0]),
            'Boss reward display method changed')
    code = read_method_body_from_bytes(exact_body(left, old[0])).instructions
    hooks = [i for i, ins in enumerate(code) if is_call(ins, 'call', HOOK_TOKEN)]
    require(len(hooks) == 2, 'Candidate reward display hook count differs')
    expected = {MONSTER_GETTER, BOSS_GETTER}
    getters = set()
    for i in hooks:
        require(i > 0 and i + 1 < len(code) and code[i - 1].opcode.name == 'callvirt' and
                is_call(code[i + 1], 'call', RESULT_CALL), 'Candidate reward display hook placement differs')
        getters.add(code[i - 1].operand.value)
    require(getters == expected, 'Candidate monster or Boss reward hook is missing')
    member = left.net.mdtables.MemberRef.rows[(HOOK_TOKEN & 0xFFFFFF) - 1]
    typ = member.Class.row
    scope = getattr(getattr(typ, 'ResolutionScope', None), 'row', None)
    require(str(member.Name) == 'MonsterRewardDisplay' and str(typ.TypeNamespace) == 'HYJS.DesertV4' and
            str(typ.TypeName) == 'FieldHooks' and scope is not None and str(scope.Name) == 'SoulFlowerDesertV4',
            'Candidate reward hook reference changed')
    return {'method': TARGET, 'methodBodySHA256': sha(exact_body(left, old[0])),
            'bothMonsterAndBossHooksExact': True, 'existingMemberRefToken': hex(HOOK_TOKEN)}


def source_pins():
    paths = [Path(__file__).resolve(), INJECTOR, ROOT / 'tools/client_remote/build.py',
             ROOT / 'tools/client_remote/config.py', ROOT / 'tools/client_qa75/manifest.py',
             ROOT / 'tools/build_multiclient.py', ROOT / 'tools/build_wallet_client.py',
             ROOT / 'tools/client_branding/package_release.py', ROOT / 'tools/client_rank_desert_patch/build.py',
             ROOT / 'tools/client_relations85/verify_existing.py', ROOT / 'tools/client_relations85/build.py',
             ROOT / 'tools/client_relations85/target-pins.json', ROOT / 'tools/client_activity/build.py',
             ROOT / 'tools/client_achievements/build.py', ROOT / 'tools/awake_diff_rows/build.py',
             ROOT / 'tools/local_asset_ready/remove_gate.py']
    return {str(path.relative_to(ROOT)): common.digest(path) for path in paths}


def main():
    require(BASE.is_file() and common.digest(BASE) == BASE_SHA, 'Pinned 1.1.9 candidate SHA-256 differs')
    require(not OUTPUT_DIRECTORY.exists() and not RECORD.exists(), 'Preserve existing QA output and evidence')
    require(shutil.disk_usage(ROOT).free > BASE.stat().st_size * 3 + 200_000_000, 'Insufficient isolated packaging space')
    require((ROOT / 'build/.client-build.lock').is_file(), 'Existing packaging lock is missing')
    dotnet = RUNTIME / 'dotnet/dotnet'
    compiler = unique((RUNTIME / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'), 'SDK compiler')
    framework = unique((RUNTIME / 'dotnet/packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'), 'net8 reference pack')
    dnlib = RUNTIME / 'injector/bin/Debug/net8.0/dnlib.dll'
    version = framework.parents[1].name
    require(dotnet.is_file() and dnlib.is_file() and (RUNTIME / 'dotnet/shared/Microsoft.NETCore.App' / version).is_dir(),
            'Local runtime or dnlib missing')
    modern_refs = ['/reference:' + str(path) for path in sorted(framework.glob('*.dll'))]
    require(modern_refs, 'Empty net8 reference pack')
    keystore = ROOT / '.local-signing/debug.keystore'
    require(keystore.is_file(), 'Local signing identity unavailable')
    env = dict(os.environ, JAVA_HOME=str(JAVA), PATH=str(JAVA / 'bin') + os.pathsep + os.environ.get('PATH', ''))

    def run(label, command):
        result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT, env=env,
                                capture_output=True, text=True, timeout=240)
        error = (result.stdout + result.stderr)[-4000:].replace('pass:android', '[redacted]')
        require(result.returncode == 0, label + ' failed: ' + error)
        return result.stdout + result.stderr

    pins = source_pins()
    tools = {str(path): common.digest(path) for path in (dotnet, compiler, dnlib, SDK / 'zipalign', SDK / 'apksigner', SDK / 'aapt2')}
    with (ROOT / 'build/.client-build.lock').open('r') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        require(not OUTPUT_DIRECTORY.exists() and not RECORD.exists(), 'QA output or evidence appeared while waiting')
        require(common.digest(BASE) == BASE_SHA and source_pins() == pins, 'Candidate or QA sources changed')
        certificate = common._certificate(run('verify candidate signature', [SDK / 'apksigner', 'verify', '--print-certs', BASE]))
        with zipfile.ZipFile(BASE) as archive:
            names = archive.namelist()
            require(len(names) == len(set(names)), 'Candidate ZIP has duplicate entries')
            before = read_unique(archive, DLL)
            wallet_before = read_unique(archive, WALLET)
            manifest_before = read_unique(archive, MANIFEST)
            inherited_helper = read_unique(archive, HELPER)
        old, wallet_after = patch_wallet(wallet_before)
        manifest_after, manifest_changes, label = patch_package(manifest_before)
        require(manifest_changes == [{'element': 'manifest', 'attribute': 'package',
                                     'before': 'com.yunchang.djsy.ewan.local', 'after': PACKAGE}],
                'Manifest changed outside the reviewed package boundary')
        targets = {name: (old + '/sao/' if name == 'ServerConfig::GetServerUrl' else old,
                          ORIGIN + '/sao/' if name == 'ServerConfig::GetServerUrl' else ORIGIN)
                   for name in remote.targets_for(ORIGIN)}
        require_old_sites(before, targets)
        with tempfile.TemporaryDirectory(prefix='.hyjs-phone-qa-', dir=OUTPUT_DIRECTORY.parent) as temporary:
            work = Path(temporary)
            input_main, output_main = work / 'input.dll', work / 'output.dll'
            input_main.write_bytes(before)
            shutil.copy2(dnlib, work / 'dnlib.dll')
            program = work / 'QAInjector.dll'
            (work / 'QAInjector.runtimeconfig.json').write_text(json.dumps({'runtimeOptions': {'tfm': 'net8.0',
                'framework': {'name': 'Microsoft.NETCore.App', 'version': version}}}) + '\n')
            run('compile QA injector', [dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:latest',
                '/out:' + str(program), *modern_refs, '/reference:' + str(work / 'dnlib.dll'), INJECTOR])
            run('patch managed origins', [dotnet, program, input_main, old, ORIGIN, output_main])
            after = output_main.read_bytes()
            original_targets = remote.targets_for
            try:
                remote.targets_for = lambda _: targets
                checks = remote.verify(before, after, ORIGIN)
            finally:
                remote.targets_for = original_targets
            metadata = verify_metadata_tokens(before, after)
            display = verify_boss_display(before, after)
            pe = dnfile.dnPE(data=after)
            for _, _, row in methods(pe):
                if not row.Rva:
                    continue
                for instruction in read_method_body_from_bytes(exact_body(pe, row)).instructions:
                    if instruction.opcode.name == 'ldstr':
                        value = pe.net.user_strings.get(instruction.operand.value & 0xFFFFFF).value
                        require(old not in value, 'Inherited candidate executable origin remains')
            replacements = {DLL: after, WALLET: wallet_after, MANIFEST: manifest_after}
            inherited, compressed = {}, {}
            unsigned, aligned, signed = [work / name for name in ('unsigned.apk', 'aligned.apk', 'signed.apk')]
            with zipfile.ZipFile(BASE) as source, BASE.open('rb') as stream, zipfile.ZipFile(unsigned, 'w') as output:
                output.comment = source.comment
                for info in source.infolist():
                    if common._signature(info.filename):
                        continue
                    if info.filename in replacements:
                        output.writestr(copy.copy(info), replacements[info.filename])
                    else:
                        inherited[info.filename] = common._hash_entry(source, info.filename)
                        compressed[info.filename] = compressed_hash(stream, info)
                        copy_original_record(stream, output, info)
            run('zipalign', [SDK / 'zipalign', '-f', '-p', '4', unsigned, aligned])
            unsigned.unlink()
            run('sign isolated QA package', [SDK / 'apksigner', 'sign', '--ks', keystore, '--ks-key-alias', 'androiddebugkey',
                '--ks-pass', 'pass:android', '--key-pass', 'pass:android', '--v4-signing-enabled', 'false', '--out', signed, aligned])
            aligned.unlink()
            require(not signed.with_suffix('.apk.idsig').exists(), 'Unexpected detached signing file')
            require(common._certificate(run('verify QA signature', [SDK / 'apksigner', 'verify', '--verbose', '--print-certs', signed])) == certificate,
                    'QA signer differs from candidate')
            run('verify QA alignment', [SDK / 'zipalign', '-c', '-p', '4', signed])
            hashes = {name: sha(raw) for name, raw in replacements.items()}
            with zipfile.ZipFile(signed) as archive, signed.open('rb') as stream:
                names = archive.namelist()
                require(len(names) == len(set(names)) and
                        {n for n in names if not common._signature(n)} == set(inherited) | set(replacements),
                        'QA ZIP entry inventory changed')
                for name, expected in {**inherited, **hashes}.items():
                    require(common._hash_entry(archive, name) == expected, 'QA entry changed: ' + name)
                for name, expected in compressed.items():
                    require(compressed_hash(stream, archive.getinfo(name)) == expected, 'QA compressed payload changed: ' + name)
                require(archive.read(HELPER) == inherited_helper, 'Desert helper differs from candidate')
            badging = run('verify isolated package', [SDK / 'aapt2', 'dump', 'badging', signed])
            require("package: name='" + PACKAGE + "'" in badging and "application-label:'" + label + "'" in badging,
                    'QA package or inherited label differs')
            require(common.digest(BASE) == BASE_SHA and source_pins() == pins, 'Candidate or QA sources changed during packaging')
            for name, expected in tools.items():
                require(common.digest(Path(name)) == expected, 'Local packaging tool changed: ' + name)
            require(not OUTPUT_DIRECTORY.exists() and not RECORD.exists(), 'QA output or evidence appeared during packaging')
            helpers = {name: value for name, value in inherited.items()
                       if name.startswith('assets/bin/Data/Managed/') and name.endswith('.dll')}
            dex = {name: value for name, value in inherited.items() if name.endswith('.dex')}
            require(helpers.get(HELPER) == sha(inherited_helper) and dex, 'Helper or DEX inventory missing')
            output_path = OUTPUT_DIRECTORY / OUTPUT_NAME
            result = {'schema': 'hyjs-rank-desert-phone-qa-v1', 'status': 'PASS_QA_SIGN_ALIGN_STATIC_NOT_INSTALLED',
                      'qaOnly': True, 'neverPromote': True, 'promoted': False, 'installed': False,
                      'phoneExecuted': False, 'networkServicesContacted': False,
                      'baseAPK': str(BASE.relative_to(ROOT)), 'baseSHA256': BASE_SHA,
                      'outputAPK': str(output_path.relative_to(ROOT)), 'binarySHA256': common.digest(signed),
                      'package': PACKAGE, 'label': label, 'inheritedOrigin': old, 'qaOrigin': ORIGIN,
                      'manifestChanges': manifest_changes, 'walletOnlyOriginChanged': True,
                      'replacementEntries': hashes, 'managedHelperSHA256': helpers, 'dexSHA256': dex,
                      'inheritedEntryCount': len(inherited), 'allOtherEntriesAndCompressedPayloadsExact': True,
                      'inheritedCompressedInventorySHA256': sha(json.dumps(compressed, sort_keys=True, separators=(',', ':')).encode()),
                      'allManagedHelpersByteExact': True, 'javaAllDexExact': True,
                      'signatureVerified': True, 'zipalignVerified': True, 'signingCertificateSHA256': certificate,
                      'bossRewardHookProof': display, 'metadataProof': metadata, 'sourcePins': pins,
                      'toolPins': tools, **checks}
            final = work / 'final'
            final.mkdir()
            signed.replace(final / OUTPUT_NAME)
            os.rename(final, OUTPUT_DIRECTORY)
        result['ownedIntermediatesCleaned'] = not work.exists()
        require(result['ownedIntermediatesCleaned'], 'Owned temporary packaging files remain')
        RECORD.parent.mkdir(parents=True, exist_ok=True)
        with RECORD.open('x') as record:
            record.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({key: result[key] for key in ('status', 'outputAPK', 'binarySHA256', 'package',
                                                   'originalEngineMethodsUnchanged', 'ownedIntermediatesCleaned')},
                     ensure_ascii=False))


if __name__ == '__main__':
    main()
