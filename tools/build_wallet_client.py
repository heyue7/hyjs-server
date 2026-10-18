"""Compose the native wallet or reviewed managed overlays on a qualified APK.

Never rerun original Mono/voice/navigation flags. The inherited APK is the
source of every existing entry. Only its wallet session hooks, launcher name,
new wallet dex/assets and fresh signing metadata may differ in wallet mode.
Managed overlays keep all other entries exact, including wallet and IME bytes.
"""
import argparse
import copy
import fcntl
import hashlib
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

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.local_wallet.verify import verify as verify_wallet_ui

DLL = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
BRIDGE = 'assets/bin/Data/Managed/LocalWalletSession.dll'
OLD_ACTIVITY = 'com.unity3d.player.UnityPlayerActivity'
NEW_ACTIVITY = 'com.hyjs.wallet.WalletActivity'


def digest(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def _length8(raw, at):
    value = raw[at]
    return (((value & 0x7f) << 8) | raw[at+1], at+2) if value & 0x80 else (value, at+1)


def _length16(raw, at):
    value = struct.unpack_from('<H', raw, at)[0]
    return (((value & 0x7fff) << 16) | struct.unpack_from('<H', raw, at+2)[0], at+4) if value & 0x8000 else (value, at+2)


def _encode8(value):
    if value > 0x7fff:
        raise ValueError('Manifest UTF8 length is unsupported')
    return bytes([0x80 | (value >> 8), value & 0xff]) if value > 0x7f else bytes([value])


def patch_manifest_launcher(raw):
    """Replace one launcher string in-place, preserving all offsets/attributes."""
    if len(raw) < 8 or struct.unpack_from('<HHI', raw) != (3, 8, len(raw)):
        raise ValueError('Expected complete Android binary XML')
    chunks = []
    strings, slots = [], []
    at = 8
    pools = 0
    utf8 = False
    while at < len(raw):
        if at+8 > len(raw):
            raise ValueError('Truncated manifest chunk')
        kind, header, size = struct.unpack_from('<HHI', raw, at)
        if header < 8 or size < header or at+size > len(raw):
            raise ValueError('Invalid manifest chunk bounds')
        chunks.append((at, kind, header, size))
        if kind == 1:
            pools += 1
            if pools != 1 or header < 28:
                raise ValueError('Expected one normal manifest string pool')
            count, styles, flags, start, style_start = struct.unpack_from('<5I', raw, at+8)
            if header+4*(count+styles) > start or start >= size:
                raise ValueError('Invalid manifest string offsets')
            utf8 = bool(flags & 0x100)
            end = at+(style_start if style_start else size)
            for index in range(count):
                offset = struct.unpack_from('<I', raw, at+header+4*index)[0]
                begin = at+start+offset
                if begin >= end:
                    raise ValueError('Manifest string outside pool')
                if utf8:
                    units, cursor = _length8(raw, begin)
                    length, cursor = _length8(raw, cursor)
                    stop = cursor+length+1
                    if stop > end or raw[stop-1] != 0:
                        raise ValueError('Invalid manifest UTF8 terminator')
                    value = raw[cursor:cursor+length].decode('utf-8')
                    if len(value.encode('utf-16le'))//2 != units:
                        raise ValueError('Manifest UTF8 unit count mismatch')
                else:
                    units, cursor = _length16(raw, begin)
                    stop = cursor+units*2+2
                    if stop > end or raw[stop-2:stop] != b'\0\0':
                        raise ValueError('Invalid manifest UTF16 terminator')
                    value = raw[cursor:stop-2].decode('utf-16le')
                strings.append(value)
                slots.append((begin, stop))
        at += size
    if at != len(raw) or pools != 1 or strings.count(OLD_ACTIVITY) != 1 or NEW_ACTIVITY in strings:
        raise ValueError('Expected one unmodified Unity launcher class')
    old_index = strings.index(OLD_ACTIVITY)
    # Confirm this exact string is referenced by one activity/name attribute,
    # whose actual child intent-filter contains MAIN and LAUNCHER.
    references = []
    stack = []
    target = None
    has_main = has_launcher = False
    for pos, kind, header, size in chunks:
        if kind == 0x102:
            if header != 16 or size < 36:
                raise ValueError('Unexpected manifest XML start-element shape')
            element = strings[struct.unpack_from('<I', raw, pos+20)[0]]
            attr_start, attr_size, count = struct.unpack_from('<3H', raw, pos+24)
            if attr_size < 20 or 16+attr_start+count*attr_size > size:
                raise ValueError('Invalid manifest attribute array')
            attrs = {}
            for index in range(count):
                entry = pos+16+attr_start+index*attr_size
                namespace, name, raw_value = struct.unpack_from('<3I', raw, entry)
                value_type = raw[entry+15]
                value_data = struct.unpack_from('<I', raw, entry+16)[0]
                attr_name = strings[name]
                value = strings[value_data] if value_type == 3 else None
                attrs[attr_name] = value
                if raw_value == old_index or (value_type == 3 and value_data == old_index):
                    references.append((element, attr_name, entry))
                    if element != 'activity' or attr_name != 'name' or value_type != 3 or value_data != old_index or raw_value not in (0xffffffff,old_index) or namespace == 0xffffffff or strings[namespace] != 'http://schemas.android.com/apk/res/android':
                        raise ValueError('Unity class string has a non-launcher reference')
                    target = len(stack)
            stack.append(element)
            if target is not None and len(stack)-1 > target:
                if element == 'action' and attrs.get('name') == 'android.intent.action.MAIN':
                    has_main = True
                if element == 'category' and attrs.get('name') == 'android.intent.category.LAUNCHER':
                    has_launcher = True
        elif kind == 0x103:
            if not stack:
                raise ValueError('Manifest element stack underflow')
            stack.pop()
            if target is not None and len(stack) <= target:
                target = None
    if stack or len(references) != 1 or not has_main or not has_launcher:
        raise ValueError('Expected sole actual Unity MAIN/LAUNCHER activity')
    begin, stop = slots[old_index]
    encoded = NEW_ACTIVITY.encode('utf-8') if utf8 else NEW_ACTIVITY.encode('utf-16le')
    units = len(NEW_ACTIVITY.encode('utf-16le'))//2
    replacement = (_encode8(units)+_encode8(len(encoded))+encoded+b'\0') if utf8 else struct.pack('<H', units)+encoded+b'\0\0'
    if len(replacement) > stop-begin:
        raise ValueError('Wallet launcher exceeds original string capacity')
    revised = bytearray(raw)
    revised[begin:stop] = replacement+b'\0'*(stop-begin-len(replacement))
    if len(revised) != len(raw) or revised[:begin] != raw[:begin] or revised[stop:] != raw[stop:]:
        raise ValueError('Manifest bytes changed outside the one class-name slot')
    return bytes(revised), {'onlyLauncherStringChanged': True, 'binaryXMLLengthUnchanged': len(raw),
                            'allChunkOffsetsAndAttributesPreserved': True, 'stringEncoding': 'utf8' if utf8 else 'utf16le',
                            'stringIndex': old_index, 'byteStart': begin, 'byteEnd': stop,
                            'oldActivity': OLD_ACTIVITY, 'newActivity': NEW_ACTIVITY,
                            'beforeSHA256': hashlib.sha256(raw).hexdigest(), 'afterSHA256': hashlib.sha256(revised).hexdigest()}


def _signature(name):
    return bool(re.fullmatch(r'META-INF/(?:MANIFEST\.MF|[^/]+\.(?:SF|RSA|DSA|EC))', name, re.IGNORECASE))


def run(arguments, env=None, timeout=240):
    result = subprocess.run(['rtk', 'proxy', *map(str, arguments)], cwd=ROOT, env=env,
                            capture_output=True, text=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError('Wallet APK command failed: '+str(arguments[0])+'\n'+(result.stdout+result.stderr)[-5000:])
    return result.stdout


def _certificate(output):
    matches = re.findall(r'Signer #\d+ certificate SHA-256 digest:\s*([0-9a-fA-F]{64})', output)
    if len(matches) != 1:
        raise ValueError('Expected one existing debug signing identity')
    return matches[0].lower()


def _hash_entry(archive, name):
    with archive.open(name) as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def verify_wallet_upgrade_base(archive, proof, expected_apk_sha, promotion=None):
    """Bind the already installed wallet's exact bytes to its qualified proof."""
    promoted = proof.get('status') == 'PASS_BUILD_VERIFY_PROMOTED_NOT_INSTALLED'
    # Independent candidate packaging records promotion separately. Validate
    # both facts without rewriting either historical status.
    if proof.get('status') == 'PASS_BUILD_VERIFY_CANDIDATE_NOT_INSTALLED' and promotion:
        promoted = (promotion.get('currentSHA256') == expected_apk_sha
                    and promotion.get('current') == 'build/hyjs-local.apk'
                    and promotion.get('sameAsVerifiedInstalledAPK') is True)
    if not promoted or proof.get('binarySHA256') != expected_apk_sha or not proof.get('signatureVerified') or not proof.get('zipalignVerified'):
        raise ValueError('Base wallet proof does not qualify the exact input APK')
    engine = proof.get('bridgeProof', {})
    if engine.get('originalEngineMethods') != 547 or not engine.get('originalEngineInstructionBytesAndExceptionRegionsUnchanged'):
        raise ValueError('Base wallet has no inherited qualified engine547 protection')
    entries = proof.get('qualifiedWalletEntries', proof.get('replacementEntries', {}))
    required = {DLL, BRIDGE, 'AndroidManifest.xml', 'classes2.dex', 'assets/local-wallet.json', 'assets/local-wallet/icon.png'}
    if not required.issubset(entries):
        raise ValueError('Base wallet proof is missing its managed/UI/manifest entries')
    for name, expected in entries.items():
        if name not in archive.namelist() or _hash_entry(archive, name) != expected:
            raise ValueError('Qualified base wallet entry differs: ' + name)
    if entries[DLL] != engine.get('outputSHA256') or entries[BRIDGE] != engine.get('bridgeSHA256'):
        raise ValueError('Base wallet game/bridge proof identities disagree')
    return entries


def wallet_base_promotion(args):
    path = getattr(args, 'base_promotion_proof', None)
    if path is None:
        return None
    path = path.resolve()
    if not path.is_relative_to(ROOT/'analysis') or not path.is_file():
        raise ValueError('Use a saved local base APK promotion record')
    return json.loads(path.read_text())


def prepare_modern_ime(archive, base, directory, temp, env):
    """Reproduce the adapter patch, then require its full parser/EH/dexdump proof."""
    directory = Path(directory).resolve()
    if not directory.is_relative_to(ROOT/'build') or 'classes3.dex' in archive.namelist():
        raise ValueError('Use a fixed new IME adapter; already installed split dex is rejected')
    proof = json.loads((directory/'manifest.json').read_text())
    from tools.modern_ime import patch as adapter
    expected_removed = {'Lcom/unity3d/player/s;', 'Lcom/unity3d/player/s$1;', 'Lcom/unity3d/player/s$2;', 'Lcom/unity3d/player/s$3;'}
    if set(adapter.REMOVED) != expected_removed or set(proof.get('removedClassDefinitions', [])) != expected_removed or proof.get('removedCount') != 4:
        raise ValueError('IME must replace exactly the approved four original class definitions')
    raw = archive.read('classes.dex')
    if proof.get('originalDexSHA256') != hashlib.sha256(raw).hexdigest() or proof.get('inputAPKSHA256') != digest(base):
        raise ValueError('IME proof input differs from the exact wallet base APK')
    expected, reproduced = adapter.patch(raw)
    if expected != (directory/'classes.dex').read_bytes() or reproduced['patchedDexSHA256'] != proof['patchedDexSHA256']:
        raise ValueError('IME original dex patch differs from the reproduced exact metadata adapter')
    if not reproduced.get('allOriginalCodeItemsAndExceptionHandlersByteIdentical') or not reproduced.get('allOriginalStringTypeProtoFieldMethodIdsByteIdentical') or not reproduced.get('allRetainedClassDataRecordsByteIdentical') or reproduced.get('removedClassDataCount') != 4:
        raise ValueError('IME must preserve original code/EH/IDs and exact retained class-data records')
    new = (directory/'classes3.dex').read_bytes()
    allowed_new = expected_removed | {'Lcom/unity3d/player/s$InputState;', 'Lcom/unity3d/player/s$NativeEditor;', 'Lcom/unity3d/player/s$NativeEditor$1;'}
    definitions = {name for name, _ in adapter.definitions(new)}
    if definitions != allowed_new or definitions != set(proof.get('replacementClasses', [])) or hashlib.sha256(new).hexdigest() != proof.get('replacementDexSHA256'):
        raise ValueError('IME replacement class whitelist or exact compiled source binding differs')
    if definitions.intersection(name for name, _ in adapter.definitions(expected)):
        raise ValueError('IME split dex collides with retained original class definitions')
    for source in proof['sources']:
        if digest(ROOT/source['path']) != source['sha256']:
            raise ValueError('IME source changed after its fixed build')
    if proof.get('managedAssemblySHA256') != _hash_entry(archive, DLL):
        raise ValueError('IME proof targets a different inherited managed assembly')
    verification = temp/'ime-verification.json'
    verifier_sha = digest(ROOT/'tools/modern_ime/verify.py')
    # Run the adapter's actual verifier, including Android's dexdump. A failed
    # code/EH/layout validation aborts before any APK is signed or promoted.
    run([sys.executable, ROOT/'tools/modern_ime/verify.py', '--input-apk', base,
         '--output', directory, '--proof', verification], env, timeout=240)
    parsed = json.loads(verification.read_text())
    if digest(ROOT/'tools/modern_ime/verify.py') != verifier_sha:
        raise ValueError('IME verifier changed during build')
    if parsed.get('status') != 'PASS_OFFLINE_ONLY' or not parsed.get('androidDexChecksumsValid') or not parsed.get('allBytesOutsideMetadataWhitelistUnchanged') or not parsed.get('originalEngineInstructionsAndEHUnchanged'):
        raise ValueError('IME parser/code/EH/Android verifier did not pass')
    return {'classes.dex': expected, 'classes3.dex': new}, {'buildManifestSHA256': digest(directory/'manifest.json'),
                                                               'verifierSourceSHA256': verifier_sha,
                                                               'reproducedPatchProof': reproduced, 'fullVerification': parsed}


def build(args):
    build_dir = ROOT/'build'
    current, rollback = build_dir/'hyjs-local.apk', build_dir/'hyjs-rollback.apk'
    base = args.base_apk.resolve()
    ui = args.ui.resolve()
    record = args.record.resolve()
    if base != current.resolve() or not re.fullmatch('[0-9a-f]{64}', args.base_sha256):
        raise ValueError('Use the exact current APK and its explicitly confirmed SHA256')
    if not ui.is_relative_to(build_dir) or not record.is_relative_to(ROOT/'analysis') or record.exists():
        raise ValueError('Use fixed project UI build and a fresh analysis record')
    sdk = Path(os.environ.get('ANDROID_SDK_ROOT', Path.home()/'Library/Android/sdk'))
    tools = sdk/'build-tools/33.0.0'
    java_home = Path.home()/'Library/Java/JavaVirtualMachines/openjdk-18.0.1.1/Contents/Home'
    key = ROOT/'.local-signing/debug.keystore'
    for required in (tools/'zipalign', tools/'apksigner', java_home/'bin/java', key):
        if not required.is_file():
            raise ValueError('Existing local signing/Android toolchain is required: '+str(required))
    env = dict(os.environ, JAVA_HOME=str(java_home), PATH=str(java_home/'bin')+os.pathsep+os.environ.get('PATH',''))
    report = {'status': 'PENDING', 'baseAPK': str(base.relative_to(ROOT)), 'baseSHA256': args.base_sha256,
              'ui': str(ui.relative_to(ROOT)), 'phoneUI': False, 'installed': False,
              'originalAPKRepatchedFromFlags': False, 'ownedTemporaryFilesCleaned': False}
    report['mode'] = 'wallet-upgrade' if args.base_wallet_proof else 'first-install'
    began = time.monotonic()
    owned_temporary = None
    with (build_dir/'.client-build.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        try:
            if digest(base) != args.base_sha256:
                raise ValueError('Current APK SHA changed; refuse stale-base replacement')
            if shutil.disk_usage(build_dir).free < base.stat().st_size*3+200*1024*1024:
                raise ValueError('Insufficient free disk for owned unsigned/aligned/signed intermediates')
            ui_proof = json.loads((ui/'manifest.json').read_text())
            verify_wallet_ui(ui/'classes2.dex', ui/'AndroidManifest.wallet.xml')
            if digest(ui/'classes2.dex') != ui_proof['dexSHA256'] or not ui_proof['iconIncluded'] or digest(ui/'assets/local-wallet/icon.png') != ui_proof['iconSHA256']:
                raise ValueError('Fixed native wallet dex/icon proof mismatch')
            for source in ui_proof['sources']:
                if digest(ROOT/source['path']) != source['sha256']:
                    raise ValueError('Wallet UI sources changed after fixed Java build')
            report['uiProofSHA256'] = digest(ui/'manifest.json')
            base_cert = _certificate(run([tools/'apksigner','verify','--print-certs',base],env))
            bridge_sources = [ROOT/'tools/local_wallet_bridge'/name for name in ('build.py','Injector.cs','LocalWalletSessionBridge.cs')]
            source_hashes = {str(path.relative_to(ROOT)):digest(path) for path in bridge_sources}
            with tempfile.TemporaryDirectory(prefix='hyjs-wallet-apk-', dir=build_dir) as temporary:
                temp = Path(temporary)
                owned_temporary = temp
                unsigned, aligned, signed = temp/'unsigned.apk', temp/'aligned.apk', temp/'signed.apk'
                with zipfile.ZipFile(base) as incoming:
                    names = incoming.namelist()
                    if len(names) != len(set(names)):
                        raise ValueError('Base APK contains duplicate entries')
                    base_wallet = None
                    if args.base_wallet_proof:
                        proof_path = args.base_wallet_proof.resolve()
                        if not proof_path.is_relative_to(ROOT/'analysis'):
                            raise ValueError('Use the reviewed prior local wallet qualification proof')
                        base_wallet = json.loads(proof_path.read_text())
                        verify_wallet_upgrade_base(incoming, base_wallet, args.base_sha256, wallet_base_promotion(args))
                        report['baseWalletProofSHA256'] = digest(proof_path)
                        report['baseWalletProof'] = str(proof_path.relative_to(ROOT))
                        if base_wallet['signerCertificateSHA256'] != base_cert:
                            raise ValueError('Base wallet signing proof differs from its actual certificate')
                    elif any(name in names for name in ('classes2.dex',BRIDGE,'assets/local-wallet.json','assets/local-wallet/icon.png')):
                        raise ValueError('Wallet entries already exist; provide its qualified base-wallet-proof to upgrade')
                    for required in (DLL,'AndroidManifest.xml','classes.dex'):
                        if required not in names:
                            raise ValueError('Required inherited APK entry absent')
                    incoming_classes_dex = incoming.read('classes.dex')
                    original_dll = incoming.read(DLL)
                    input_dll = temp/'base-Assembly-CSharp.dll'
                    input_dll.write_bytes(original_dll)
                    bridge_output = temp/'bridge'
                    command = [sys.executable,ROOT/'tools/local_wallet_bridge/build.py','--input',input_dll,'--output-directory',bridge_output]
                    if base_wallet is not None: command.append('--bridge-only')
                    run(command,timeout=300)
                    compiled_bridge = json.loads((bridge_output/'manifest.json').read_text())
                    if compiled_bridge['inputSHA256'] != hashlib.sha256(original_dll).hexdigest() or digest(bridge_output/'LocalWalletSession.dll') != compiled_bridge['bridgeSHA256'] or compiled_bridge.get('assemblyName') != 'LocalWalletSession':
                        raise ValueError('Recompiled wallet bridge input/output/assembly ABI proof mismatch')
                    if base_wallet is not None:
                        if compiled_bridge.get('mode') != 'bridge-only' or not compiled_bridge.get('managedAssemblyUntouched') or compiled_bridge.get('freshEngineComparison') is not False or compiled_bridge['outputSHA256'] != compiled_bridge['inputSHA256'] or (bridge_output/'Assembly-CSharp.dll').exists():
                            raise ValueError('Upgrade must never re-inject or write the qualified game DLL')
                        bridge_proof = {'mode': 'wallet-upgrade', 'originalEngineMethods': 547,
                                        'originalEngineInstructionBytesAndExceptionRegionsUnchanged': True,
                                        'engineProtectionInheritedFromBaseProof': report['baseWalletProofSHA256'],
                                        'freshEngineComparison': False, 'managedAssemblyByteIdenticalToBase': True,
                                        'onlyChangedEntries': [], 'inputSHA256': compiled_bridge['inputSHA256'],
                                        'outputSHA256': compiled_bridge['inputSHA256'], 'bridgeSHA256': compiled_bridge['bridgeSHA256']}
                        manifest, manifest_proof = incoming.read('AndroidManifest.xml'), dict(base_wallet['manifestProof'])
                        manifest_proof['unchangedThisUpgrade'] = True
                        report['bridgeOnlyBuildProof'] = compiled_bridge
                    else:
                        bridge_proof = compiled_bridge
                        if not bridge_proof['originalEngineInstructionBytesAndExceptionRegionsUnchanged'] or bridge_proof['originalEngineMethods'] != 547 or set(bridge_proof['onlyChangedEntries']) != {'GameConfig::Awake','LoginUserLogin::UserLogin','LoginUI::OnSwitchAccountButtonClick'} or digest(bridge_output/'Assembly-CSharp.dll') != bridge_proof['outputSHA256']:
                            raise ValueError('First wallet install did not preserve the exact game engine/hooks')
                        manifest, manifest_proof = patch_manifest_launcher(incoming.read('AndroidManifest.xml'))
                    if source_hashes != {str(path.relative_to(ROOT)):digest(path) for path in bridge_sources}:
                        raise ValueError('Bridge sources changed during build')
                    replacements = {BRIDGE:(bridge_output/'LocalWalletSession.dll').read_bytes(), 'classes2.dex':(ui/'classes2.dex').read_bytes(),
                                    'assets/local-wallet.json':(ui/'assets/local-wallet.json').read_bytes(), 'assets/local-wallet/icon.png':(ui/'assets/local-wallet/icon.png').read_bytes()}
                    if base_wallet is None: replacements.update({DLL:(bridge_output/'Assembly-CSharp.dll').read_bytes(), 'AndroidManifest.xml':manifest})
                    if args.modern_ime:
                        ime_entries, ime_proof = prepare_modern_ime(incoming, base, args.modern_ime, temp, env)
                        replacements.update(ime_entries)
                        report['modernIME'] = ime_proof
                    elif base_wallet is not None and base_wallet.get('modernIME'):
                        # A UI-only upgrade inherits the already verified IME
                        # DEX bytes and keeps that proof available to later upgrades.
                        report['modernIME'] = base_wallet['modernIME']
                        report['modernIMEInheritedFromBase'] = True
                    qualified_wallet_entries = {name:hashlib.sha256(replacements.get(name,incoming.read(name) if name in names else b'')).hexdigest() for name in (DLL,'AndroidManifest.xml',BRIDGE,'classes2.dex','assets/local-wallet.json','assets/local-wallet/icon.png')}
                    if args.modern_ime:
                        qualified_wallet_entries.update({name:hashlib.sha256(replacements[name]).hexdigest() for name in ('classes.dex','classes3.dex')})
                    elif base_wallet is not None and base_wallet.get('modernIME'):
                        qualified_wallet_entries.update({name:_hash_entry(incoming,name) for name in ('classes.dex','classes3.dex')})
                    inherited = {}
                    with zipfile.ZipFile(unsigned,'w') as outgoing:
                        outgoing.comment = incoming.comment
                        for info in incoming.infolist():
                            if _signature(info.filename):
                                continue
                            if info.filename in replacements:
                                outgoing.writestr(copy.copy(info),replacements[info.filename])
                            else:
                                hash_value = hashlib.sha256()
                                with incoming.open(info) as source, outgoing.open(copy.copy(info),'w') as target:
                                    while True:
                                        block=source.read(1024*1024)
                                        if not block:break
                                        hash_value.update(block);target.write(block)
                                inherited[info.filename]=hash_value.hexdigest()
                        for name in sorted(set(replacements)-set(names)):
                            info=zipfile.ZipInfo(name,date_time=(1980,1,1,0,0,0));info.compress_type=zipfile.ZIP_DEFLATED
                            outgoing.writestr(info,replacements[name])
                run([tools/'zipalign','-f','-p','4',unsigned,aligned],env)
                run([tools/'apksigner','sign','--ks',key,'--ks-pass','pass:android','--key-pass','pass:android','--out',signed,aligned],env)
                signature = run([tools/'apksigner','verify','--verbose','--print-certs',signed],env)
                if _certificate(signature) != base_cert:
                    raise ValueError('New APK signing identity differs from installed-base identity')
                run([tools/'zipalign','-c','-p','4',signed],env)
                with zipfile.ZipFile(signed) as verified:
                    signed_names = verified.namelist()
                    expected_names = (set(names)-{name for name in names if _signature(name)})|set(replacements)
                    actual_names = {name for name in signed_names if not _signature(name)}
                    if len(signed_names)!=len(set(signed_names)) or actual_names!=expected_names:
                        raise ValueError('Unexpected inherited/new APK entry set')
                    for name, expected in inherited.items():
                        if _hash_entry(verified,name)!=expected:
                            raise ValueError('Inherited APK entry changed: '+name)
                    for name, expected in replacements.items():
                        if _hash_entry(verified,name)!=hashlib.sha256(expected).hexdigest():
                            raise ValueError('Wallet APK replacement changed after signing: '+name)
                if digest(current)!=args.base_sha256:
                    raise ValueError('Current APK changed before verified promotion')
                report.update(bridgeProof=bridge_proof,bridgeSourceSHA256=source_hashes,manifestProof=manifest_proof,
                              originalClassesDexSHA256=base_wallet.get('originalClassesDexSHA256') if base_wallet else hashlib.sha256(incoming_classes_dex).hexdigest(),allOtherInheritedEntriesExact=True,
                              baseClassesDexSHA256=hashlib.sha256(incoming_classes_dex).hexdigest(),
                              classesDexSHA256=hashlib.sha256(replacements.get('classes.dex',incoming_classes_dex)).hexdigest(),
                              originalClassesDexByteIdentical=hashlib.sha256(replacements.get('classes.dex',incoming_classes_dex)).hexdigest()==(base_wallet.get('originalClassesDexSHA256') if base_wallet else hashlib.sha256(incoming_classes_dex).hexdigest()),
                              baseClassesDexByteIdentical=not bool(args.modern_ime),
                              managedAssemblyByteIdenticalToBase=base_wallet is not None,
                              qualifiedWalletEntries=qualified_wallet_entries,
                              inheritedEntryCount=len(inherited),inheritedEntrySHA256=hashlib.sha256(json.dumps(inherited,sort_keys=True,separators=(',',':')).encode()).hexdigest(),
                              replacementEntries={name:hashlib.sha256(value).hexdigest() for name,value in replacements.items()},
                              signerCertificateSHA256=base_cert,binarySHA256=digest(signed),outputAPK=str(current.relative_to(ROOT)),
                              zipalignVerified=True,signatureVerified=True)
                current.replace(rollback)
                try:
                    signed.replace(current)
                except BaseException:
                    if not current.exists():rollback.replace(current)
                    raise
                idsig=signed.with_suffix('.apk.idsig')
                if idsig.exists():idsig.replace(current.with_suffix('.apk.idsig'))
                else:current.with_suffix('.apk.idsig').unlink(missing_ok=True)
                rollback.with_suffix('.apk.idsig').unlink(missing_ok=True)
                report.update(status='PASS_BUILD_VERIFY_PROMOTED_NOT_INSTALLED',rollbackAPK=str(rollback.relative_to(ROOT)),rollbackSHA256=digest(rollback))
        except BaseException as error:
            report.update(status='FAILED_NOT_QUALIFIED',errorType=type(error).__name__,error=str(error)[-6000:])
            raise
        finally:
            report['ownedTemporaryFilesCleaned']=owned_temporary is None or not owned_temporary.exists()
            if not report['ownedTemporaryFilesCleaned']:
                report['ownedTemporaryDirectoryRemaining']=str(owned_temporary)
            report['seconds']=round(time.monotonic()-began,3)
            record.parent.mkdir(parents=True,exist_ok=True)
            record.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({key:report[key] for key in ('status','binarySHA256','rollbackSHA256','inheritedEntryCount')},indent=2))


def build_managed_overlay(args):
    """Compose one reviewed, bounded managed change on the qualified APK."""
    current, rollback = ROOT/'build/hyjs-local.apk', ROOT/'build/hyjs-rollback.apk'
    base, record = args.base_apk.resolve(), args.record.resolve()
    overlay = args.managed_overlay.resolve()
    proof_path = args.base_wallet_proof.resolve()
    if (base not in (current, rollback) or not re.fullmatch('[0-9a-f]{64}', args.base_sha256)
            or not overlay.is_relative_to(ROOT/'analysis')
            or not proof_path.is_relative_to(ROOT/'analysis')
            or not record.is_relative_to(ROOT/'analysis') or record.exists()):
        raise ValueError('Use the exact current/rollback APK, reviewed local overlay and new record')
    sdk = Path(os.environ.get('ANDROID_SDK_ROOT', Path.home()/'Library/Android/sdk'))
    android = sdk/'build-tools/33.0.0'
    java_home = Path.home()/'Library/Java/JavaVirtualMachines/openjdk-18.0.1.1/Contents/Home'
    key = ROOT/'.local-signing/debug.keystore'
    env = dict(os.environ, JAVA_HOME=str(java_home), PATH=str(java_home/'bin')+os.pathsep+os.environ.get('PATH',''))
    report = dict(status='PENDING', mode='managed-resource-overlay', baseAPK=str(base.relative_to(ROOT)),
                  baseSHA256=args.base_sha256, phoneUI=False, installed=False,
                  originalAPKRepatchedFromFlags=False, ownedTemporaryFilesCleaned=False)
    if getattr(args, 'base_promotion_proof', None):
        promotion_path = args.base_promotion_proof.resolve()
        wallet_base_promotion(args)
        report.update(basePromotionProof=str(promotion_path.relative_to(ROOT)),
                      basePromotionProofSHA256=digest(promotion_path))
    began = time.monotonic()
    temporary_path = None
    with (ROOT/'build/.client-build.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        try:
            current_before = digest(current)
            if digest(base) != args.base_sha256:
                raise ValueError('Current APK changed before overlay composition')
            if shutil.disk_usage(current.parent).free < base.stat().st_size*3+200*1024*1024:
                raise ValueError('Insufficient free disk for owned APK intermediates')
            proof, module = json.loads(proof_path.read_text()), json.loads((overlay/'manifest.json').read_text())
            recovery = module.get('schema') == 'local-awake-end-recovery-overlay-v1'
            title = module.get('schema') == 'local-activity-title-overlay-v1'
            gate_removal = module.get('schema') == 'local-awake-resource-gate-removal-overlay-v1'
            bootstrap = module.get('schema') == 'local-formation-bootstrap-overlay-v1'
            formation = bootstrap or module.get('schema') == 'local-formation-ui-overlay-v1'
            resource = not title and not formation and not gate_removal and not recovery
            allowed = {DLL} if title or gate_removal or recovery else {DLL, 'assets/bin/Data/Managed/LocalFormationUI.dll' if formation else 'assets/bin/Data/Managed/LocalAssetReady.dll'}
            changed_methods = ({'Battle.AwakeCopyBattleOver::OnAskAwakeCopyFightEndCallback'} if recovery else
                               {'UIActivity::InitTabs', 'CopyBenefit::PrepareData'} if title else
                               {'Room.RCUINormalNPC::OpenPage', 'AwakeDiffItem::OnChallengeClick'} if gate_removal else
                               {'UI.ArrayPage::DoOpen'} if formation else
                               {'AwakeDiffItem::OnChallengeClick', 'RCUINormalNPC::OpenPage'})
            if (module.get('schema') not in ('local-awake501-resource-overlay-v1', 'local-activity-title-overlay-v1', 'local-formation-ui-overlay-v1', 'local-formation-bootstrap-overlay-v1', 'local-awake-resource-gate-removal-overlay-v1', 'local-awake-end-recovery-overlay-v1')
                    or module.get('baseAPKSHA256') != args.base_sha256
                    or module.get('baseProofSHA256') != digest(proof_path)
                    or not module.get('originalEngineInstructionBytesAndExceptionRegionsUnchanged')
                    or module.get('originalEngineMethods') != 547
                    or set(module.get('onlyChangedEntries', [])) != changed_methods
                    or set(module.get('replacementEntries', {})) != allowed
                    or module['replacementEntries'].get(DLL) != module.get('outputManagedSHA256')
                    or (resource and module['replacementEntries'].get('assets/bin/Data/Managed/LocalAssetReady.dll') != module.get('bridgeSHA256'))
                    or (resource and module.get('globalFullFlagChanged') is not False)
                    or (gate_removal and (module.get('allOtherInputManagedBytesIdentical') is not True
                                          or module.get('allOtherOriginalRawMethodHeadersILAndEHByteIdentical') is not True
                                          or module.get('noNewRuntimeAssembly') is not True
                                          or module.get('runtimeResourceBridgeCalledByPatchedGates') is not False
                                          or module.get('globalFullGetterAndPersistentFlagsUnchanged') is not True
                                          or module.get('introducedQualityPartyResourceOrCopyIDChecks') is not False
                                          or module.get('originalPostGateLevelEligibilitySelectionUnchanged') is not True
                                          or set(module.get('sourcePins', {})) != {
                                              'tools/local_asset_ready/remove_gate.py',
                                              'tools/local_asset_ready/RemoveGateInjector.cs'}))
                    or (recovery and (module.get('allOtherOriginalRawMethodHeadersILAndEHByteIdentical') is not True
                                      or module.get('noNewRuntimeAssembly') is not True
                                      or module.get('failureOnlyReturnToOriginalRoom') is not True
                                      or module.get('noSynthesizedEndOrResultCallback') is not True))
                    or (formation and (module.get('nakedNonMainOnly') is not True
                                       or module.get('fetchOriginal20111AfterSuccess') is not True
                                       or module['replacementEntries'].get('assets/bin/Data/Managed/LocalFormationUI.dll') != module.get('bridgeSHA256')))
                    or (bootstrap and (module.get('originalArrayPageAllMethodsRestored') is not True
                                       or module.get('bootstrapUsesReflection') is not True))
                    or (title and (module.get('allOtherOriginalRawMethodHeadersILAndEHByteIdentical') is not True
                                   or module.get('originalActivityHeaderLocalsBranchesAndRawEHByteIdentical') is not True
                                   or module.get('noResourceEntriesChanged') is not True
                                   or module.get('noNewRuntimeAssembly') is not True
                                   or module.get('titleLiteral') != '首充双倍'))
                    or module.get('playerStateOrBattleResultChanged') is not False):
                raise ValueError('Overlay does not have the bounded resource/engine contract')
            for name, expected in module['sourcePins'].items():
                source = (ROOT/name).resolve()
                source_root = ROOT/('tools/awake_battle_recovery' if recovery else 'tools/local_activity_title' if title else 'tools/local_formation_ui' if formation else 'tools/local_asset_ready')
                if not source.is_relative_to(source_root) or digest(source) != expected:
                    raise ValueError('Resource adapter source changed: '+name)
            replacements = {}
            for name, expected in module['replacementEntries'].items():
                path = overlay/Path(name).name
                if digest(path) != expected:
                    raise ValueError('Fixed resource DLL changed: '+name)
                replacements[name] = path.read_bytes()
            if args.ui:
                if not formation:
                    raise ValueError('Combine the wallet catalogue only with the reviewed formation batch')
                ui = args.ui.resolve()
                if not ui.is_relative_to(ROOT/'build'):
                    raise ValueError('Use a fixed owned Java build')
                ui_proof = json.loads((ui/'manifest.json').read_text())
                verify_wallet_ui(ui/'classes2.dex', ui/'AndroidManifest.wallet.xml')
                if (digest(ui/'classes2.dex') != ui_proof.get('dexSHA256')
                        or not ui_proof.get('iconIncluded')
                        or digest(ui/'assets/local-wallet/icon.png') != ui_proof.get('iconSHA256')):
                    raise ValueError('Wallet catalogue dex/icon proof mismatch')
                for entry in ('assets/local-wallet.json', 'assets/local-wallet/icon.png'):
                    if digest(ui/entry) != proof['qualifiedWalletEntries'].get(entry):
                        raise ValueError('Catalogue upgrade must preserve wallet configuration/icon')
                for source in ui_proof['sources']:
                    path = (ROOT/source['path']).resolve()
                    if not path.is_relative_to(ROOT/'tools/local_wallet') or digest(path) != source['sha256']:
                        raise ValueError('Fixed catalogue Java sources changed')
                replacements['classes2.dex'] = (ui/'classes2.dex').read_bytes()
                allowed.add('classes2.dex')
                report.update(ui=str(ui.relative_to(ROOT)), uiProofSHA256=digest(ui/'manifest.json'),
                              walletCatalogueUpgraded=True, walletConfigurationAndIconUnchanged=True)
            if bool(args.wallet_bootstrap_overlay) != bootstrap:
                raise ValueError('The isolated formation bootstrap requires its reviewed wallet bridge')
            wallet_bootstrap = None
            if bootstrap:
                extra = args.wallet_bootstrap_overlay.resolve()
                if not extra.is_relative_to(ROOT/'analysis'):
                    raise ValueError('Use a fixed reviewed wallet bootstrap module')
                wallet_bootstrap = json.loads((extra/'manifest.json').read_text())
                if (wallet_bootstrap.get('mode') != 'bridge-only'
                        or wallet_bootstrap.get('managedAssemblyUntouched') is not True
                        or wallet_bootstrap.get('credentialsOnDisk') is not False
                        or wallet_bootstrap.get('bootstrapUsesReflection') is not True
                        or wallet_bootstrap.get('bootstrapFailuresIsolated') is not True
                        or wallet_bootstrap.get('baseAPKSHA256') != args.base_sha256
                        or wallet_bootstrap.get('baseProofSHA256') != digest(proof_path)
                        or wallet_bootstrap.get('inputSHA256') != module['inputManagedSHA256']
                        or wallet_bootstrap.get('outputSHA256') != module['inputManagedSHA256']
                        or wallet_bootstrap.get('existingPublicABIUnchanged') is not True):
                    raise ValueError('Wallet bootstrap must preserve the session ABI and isolate errors')
                for name, expected in wallet_bootstrap['sourcePins'].items():
                    path = (ROOT/name).resolve()
                    if not path.is_relative_to(ROOT/'tools/local_wallet_bridge') or digest(path) != expected:
                        raise ValueError('Wallet bootstrap source changed')
                path = extra/'LocalWalletSession.dll'
                if digest(path) != wallet_bootstrap['bridgeSHA256']:
                    raise ValueError('Fixed wallet bootstrap DLL changed')
                from tools.local_wallet_bridge.build import verify_bridge_abi
                abi = verify_bridge_abi(path)
                if abi['requiredMethodSignatures'] != wallet_bootstrap.get('requiredMethodSignatures'):
                    raise ValueError('Wallet bootstrap callable signatures differ')
                replacements[BRIDGE] = path.read_bytes()
                allowed.add(BRIDGE)
                report.update(walletBootstrapProof=wallet_bootstrap,
                              walletBootstrapManifestSHA256=digest(extra/'manifest.json'),
                              originalArrayPageMethodsRestored=True)
            diagnostic = None
            if args.resource_diagnostics_overlay:
                if not formation:
                    raise ValueError('Combine resource diagnostics only with the reviewed formation UI batch')
                extra = args.resource_diagnostics_overlay.resolve()
                if not extra.is_relative_to(ROOT/'analysis'):
                    raise ValueError('Use a fixed local resource diagnostics module')
                diagnostic = json.loads((extra/'manifest.json').read_text())
                bridge_name = 'assets/bin/Data/Managed/LocalAssetReady.dll'
                if (diagnostic.get('schema') != 'local-awake501-bridge-diagnostics-v1'
                        or diagnostic.get('baseAPKSHA256') != args.base_sha256
                        or diagnostic.get('baseProofSHA256') != digest(proof_path)
                        or diagnostic.get('inputManagedSHA256') != module['inputManagedSHA256']
                        or diagnostic.get('outputManagedSHA256') != module['inputManagedSHA256']
                        or diagnostic.get('managedAssemblyByteIdenticalToBase') is not True
                        or diagnostic.get('predicateChanged') is not False
                        or diagnostic.get('globalFullFlagChanged') is not False
                        or diagnostic.get('originalResourceLoaderChanged') is not False
                        or diagnostic.get('playerStateOrBattleResultChanged') is not False
                        or diagnostic.get('allExistingPublicABISignaturesUnchanged') is not True
                        or diagnostic.get('logsContainCredentials') is not False
                        or not 1 <= diagnostic.get('diagnosticBudget', 0) <= 64
                        or diagnostic.get('replacementEntries') != {bridge_name: diagnostic.get('bridgeSHA256')}):
                    raise ValueError('Resource diagnostics must preserve the exact bridge contract and scope')
                for name, expected in diagnostic['sourcePins'].items():
                    source = (ROOT/name).resolve()
                    if not source.is_relative_to(ROOT/'tools/local_asset_ready') or digest(source) != expected:
                        raise ValueError('Resource diagnostics source changed: '+name)
                path = extra/'LocalAssetReady.dll'
                if digest(path) != diagnostic['bridgeSHA256']:
                    raise ValueError('Fixed resource diagnostics DLL changed')
                replacements[bridge_name] = path.read_bytes()
                allowed.add(bridge_name)
                report['resourceDiagnosticsProof'] = diagnostic
                report['resourceDiagnosticsManifestSHA256'] = digest(extra/'manifest.json')
            certificate = _certificate(run([android/'apksigner','verify','--print-certs',base], env))
            if certificate != proof['signerCertificateSHA256']:
                raise ValueError('Base APK signing identity differs from proof')
            with tempfile.TemporaryDirectory(prefix='hyjs-resource-apk-', dir=ROOT/'build') as directory:
                temp = temporary_path = Path(directory)
                unsigned, aligned, signed = (temp/name for name in ('unsigned.apk','aligned.apk','signed.apk'))
                inherited = {}
                with zipfile.ZipFile(base) as incoming:
                    qualified = verify_wallet_upgrade_base(incoming, proof, args.base_sha256, wallet_base_promotion(args))
                    names = incoming.namelist()
                    if len(names) != len(set(names)) or _hash_entry(incoming, DLL) != module['inputManagedSHA256']:
                        raise ValueError('Base APK entries/managed assembly differ from module input')
                    if gate_removal:
                        from tools.local_asset_ready.remove_gate import verify as verify_gate_removal
                        checks = verify_gate_removal(incoming.read(DLL), replacements[DLL])
                        if any(module.get(name) != value for name, value in checks.items()):
                            raise ValueError('Gate removal proof differs from exact input/output comparison')
                    if recovery:
                        from tools.awake_battle_recovery.build import verify as verify_awake_recovery
                        checks = verify_awake_recovery(incoming.read(DLL), replacements[DLL])
                        if any(module.get(name) != value for name, value in checks.items()):
                            raise ValueError('Awake recovery proof differs from exact input/output comparison')
                    if resource and 'assets/bin/Data/Managed/LocalAssetReady.dll' in names:
                        raise ValueError('Resource module is already present')
                    if diagnostic and _hash_entry(incoming, 'assets/bin/Data/Managed/LocalAssetReady.dll') != diagnostic['oldBridgeSHA256']:
                        raise ValueError('Resource diagnostics input bridge differs from the qualified base')
                    if wallet_bootstrap and _hash_entry(incoming, BRIDGE) != wallet_bootstrap.get('oldBridgeSHA256'):
                        raise ValueError('Wallet bootstrap input bridge differs from the qualified base')
                    with zipfile.ZipFile(unsigned,'w') as outgoing:
                        outgoing.comment = incoming.comment
                        for info in incoming.infolist():
                            if _signature(info.filename):
                                continue
                            if info.filename in replacements:
                                outgoing.writestr(copy.copy(info), replacements[info.filename])
                                continue
                            sha = hashlib.sha256()
                            with incoming.open(info) as source, outgoing.open(copy.copy(info),'w') as destination:
                                while block := source.read(1024*1024):
                                    sha.update(block)
                                    destination.write(block)
                            inherited[info.filename] = sha.hexdigest()
                        for name in sorted(allowed-set(names)):
                            info = zipfile.ZipInfo(name, date_time=(1980,1,1,0,0,0))
                            info.compress_type = zipfile.ZIP_DEFLATED
                            outgoing.writestr(info, replacements[name])
                run([android/'zipalign','-f','-p','4',unsigned,aligned], env)
                run([android/'apksigner','sign','--ks',key,'--ks-pass','pass:android','--key-pass','pass:android','--out',signed,aligned], env)
                if _certificate(run([android/'apksigner','verify','--verbose','--print-certs',signed], env)) != certificate:
                    raise ValueError('New signing identity differs from installed base')
                run([android/'zipalign','-c','-p','4',signed], env)
                with zipfile.ZipFile(signed) as verified:
                    entries = verified.namelist()
                    if (len(entries) != len(set(entries))
                            or {n for n in entries if not _signature(n)} != set(inherited)|allowed):
                        raise ValueError('Unexpected signed APK entry set')
                    for name, expected in dict(inherited, **{n: hashlib.sha256(v).hexdigest() for n, v in replacements.items()}).items():
                        if _hash_entry(verified,name) != expected:
                            raise ValueError('Signed inherited/overlay entry differs: '+name)
                if digest(current) != current_before or digest(base) != args.base_sha256:
                    raise ValueError('Current/base APK changed before promotion')
                qualified = dict(qualified)
                replacement_hashes = {n: hashlib.sha256(v).hexdigest() for n, v in replacements.items()}
                qualified.update(replacement_hashes)
                engine = dict(proof['bridgeProof'])
                engine.update(mode='managed-awake-end-recovery' if recovery else 'managed-awake-resource-gate-removal' if gate_removal else 'managed-formation-ui-overlay' if formation else 'managed-activity-title-overlay' if title else 'managed-resource-overlay', inputSHA256=module['inputManagedSHA256'],
                              outputSHA256=module['outputManagedSHA256'], freshEngineComparison=True,
                              managedAssemblyByteIdenticalToBase=False,
                              onlyChangedEntries=module['onlyChangedEntries'])
                if wallet_bootstrap:
                    engine['bridgeSHA256'] = wallet_bootstrap['bridgeSHA256']
                report.update(baseWalletProof=str(proof_path.relative_to(ROOT)), baseWalletProofSHA256=digest(proof_path),
                              managedOverlay=str(overlay.relative_to(ROOT)), managedOverlaySHA256=digest(overlay/'manifest.json'),
                              bridgeProof=engine, manifestProof=proof['manifestProof'],
                              modernIME=proof.get('modernIME'), modernIMEInheritedFromBase=True,
                              qualifiedWalletEntries=qualified, replacementEntries=replacement_hashes,
                              managedAssemblyByteIdenticalToBase=False, allOtherInheritedEntriesExact=True,
                              inheritedEntryCount=len(inherited),
                              inheritedEntrySHA256=hashlib.sha256(json.dumps(inherited,sort_keys=True,separators=(',',':')).encode()).hexdigest(),
                              signerCertificateSHA256=certificate, binarySHA256=digest(signed),
                              outputAPK=str(current.relative_to(ROOT)), signatureVerified=True, zipalignVerified=True,
                              originalClassesDexSHA256=proof.get('originalClassesDexSHA256'),
                              classesDexSHA256=qualified.get('classes.dex'), baseClassesDexByteIdentical=True)
                if recovery:
                    report.update(mode='managed-awake-end-recovery', awakeEndRecoveryOverlayProof=module,
                                  activityTitleOverlayProof=proof.get('activityTitleOverlayProof'),
                                  formationOverlayProof=proof.get('formationOverlayProof'),
                                  resourceOverlayProof=proof.get('resourceOverlayProof'))
                elif gate_removal:
                    report.update(mode='managed-awake-resource-gate-removal', awakeGateRemovalProof=module,
                                  activityTitleOverlayProof=proof.get('activityTitleOverlayProof'),
                                  formationOverlayProof=proof.get('formationOverlayProof'),
                                  resourceOverlayProof=proof.get('resourceOverlayProof'))
                elif title:
                    report.update(mode='managed-activity-title-overlay', activityTitleOverlayProof=module,
                                  resourceOverlayProof=proof.get('resourceOverlayProof'))
                elif formation:
                    report.update(mode='managed-formation-ui-overlay', formationOverlayProof=module,
                                  activityTitleOverlayProof=proof.get('activityTitleOverlayProof'),
                                  resourceOverlayProof=proof.get('resourceOverlayProof'))
                else:
                    report.update(resourceOverlayProof=module, resourceOverlay=str(overlay.relative_to(ROOT)),
                                  resourceOverlaySHA256=digest(overlay/'manifest.json'))
                current.replace(rollback)
                try:
                    signed.replace(current)
                except BaseException:
                    if not current.exists():
                        rollback.replace(current)
                    raise
                idsig = signed.with_suffix('.apk.idsig')
                if idsig.exists():
                    idsig.replace(current.with_suffix('.apk.idsig'))
                else:
                    current.with_suffix('.apk.idsig').unlink(missing_ok=True)
                rollback.with_suffix('.apk.idsig').unlink(missing_ok=True)
                report.update(status='PASS_BUILD_VERIFY_PROMOTED_NOT_INSTALLED',
                              rollbackAPK=str(rollback.relative_to(ROOT)), rollbackSHA256=digest(rollback))
        except BaseException as error:
            report.update(status='FAILED_NOT_QUALIFIED', errorType=type(error).__name__, error=str(error)[-6000:])
            raise
        finally:
            report.update(ownedTemporaryFilesCleaned=temporary_path is None or not temporary_path.exists(),
                          seconds=round(time.monotonic()-began,3))
            record.parent.mkdir(parents=True,exist_ok=True)
            record.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({k:report[k] for k in ('status','binarySHA256','rollbackSHA256','inheritedEntryCount')}))


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk',type=Path,required=True)
    parser.add_argument('--base-sha256',required=True)
    parser.add_argument('--ui',type=Path)
    parser.add_argument('--record',type=Path,required=True)
    parser.add_argument('--base-wallet-proof',type=Path,
                        help='Upgrade an exact qualified wallet APK without re-injecting its game DLL')
    parser.add_argument('--base-promotion-proof',type=Path,
                        help='Separate verified promotion record for an independently packaged wallet candidate')
    parser.add_argument('--modern-ime',type=Path,
                        help='Fixed keyboard adapter directory; exact reproduced patch/full Android verifier required')
    parser.add_argument('--managed-overlay',type=Path,
                        help='Reviewed Awake501 or activity-title overlay; keep wallet/IME entries exact')
    parser.add_argument('--resource-diagnostics-overlay',type=Path,
                        help='Combine the bounded501 bridge diagnostics with the formation UI overlay')
    parser.add_argument('--wallet-bootstrap-overlay',type=Path,
                        help='Isolated reflection bootstrap; preserve the existing wallet public ABI')
    parser.add_argument('--green-hero-overlay',type=Path,help='Fixed six-green resource overlay')
    parser.add_argument('--formation-guard',type=Path,help='Fixed first-round-only managed guard')
    args=parser.parse_args()
    if args.green_hero_overlay:
        if not args.base_wallet_proof or not args.ui or not args.formation_guard:
            parser.error('Growth overlay requires exact base proof, rebuilt UI and formation guard')
        from tools.green_hero_compat.package_growth import build_growth_overlay
        build_growth_overlay(args)
        return
    if args.managed_overlay:
        if not args.base_wallet_proof or args.modern_ime:
            parser.error('Managed overlay needs its base proof and inherits the existing IME')
        build_managed_overlay(args)
    else:
        if args.resource_diagnostics_overlay or args.wallet_bootstrap_overlay:
            parser.error('Bridge overlays require a reviewed managed overlay batch')
        if not args.ui:
            parser.error('Wallet build requires --ui')
        build(args)


if __name__=='__main__':
    main()
