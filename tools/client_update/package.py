"""Package a qualified update layer without rebuilding inherited game or wallet code."""
import argparse
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import struct
import sys
import tempfile
import zipfile
import zlib

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools import build_wallet_client as common
from tools.client_branding.branding import chunks, elements, make_pool, strings_from_pool
from tools.client_branding.package_release import compressed_hash, copy_original_record, require
from tools.awake_diff_rows.build import methods
from tools.local_asset_ready.remove_gate import exact_body
import dnfile


def patch_manifest(raw, build_code, version_name):
    """Change only the launcher and two Android version attributes."""
    parts = chunks(raw)
    pools = [i for i, item in enumerate(parts) if item[0] == 1]
    require(len(pools) == 1, 'Expected one manifest string pool')
    pool = pools[0]
    original, flags = strings_from_pool(parts[pool][2])
    values = list(original)
    activity = 'com.hyjs.wallet.ClientUpdateActivity'
    for value in (activity, version_name):
        if value not in values:
            values.append(value)
    changed = []
    rebuilt = []
    for index, (kind, header, content) in enumerate(parts):
        if index == pool:
            content = make_pool(values, flags)
        elif kind == 0x102:
            content = bytearray(content)
            element = original[struct.unpack_from('<I', content, 20)[0]]
            start, width, count = struct.unpack_from('<3H', content, 24)
            for number in range(count):
                at = 16 + start + width * number
                name = original[struct.unpack_from('<I', content, at + 4)[0]]
                value = struct.unpack_from('<I', content, at + 16)[0]
                if element == 'manifest' and name == 'versionCode':
                    require(content[at + 15] in (16, 17), 'Unexpected versionCode encoding')
                    require(build_code > value, 'New Android build must increase versionCode')
                    struct.pack_into('<I', content, at + 8, 0xffffffff)
                    struct.pack_into('<HBBI', content, at + 12, 8, 0, 16, build_code)
                    changed.append('versionCode')
                elif element == 'manifest' and name == 'versionName':
                    new_value = values.index(version_name)
                    struct.pack_into('<I', content, at + 8, new_value)
                    struct.pack_into('<HBBI', content, at + 12, 8, 0, 3, new_value)
                    changed.append('versionName')
                elif element == 'activity' and name == 'name' and content[at + 15] == 3 and original[value] in ('com.hyjs.wallet.WalletActivity', activity):
                    new_value = values.index(activity)
                    struct.pack_into('<I', content, at + 8, new_value)
                    struct.pack_into('<I', content, at + 16, new_value)
                    changed.append('launcher')
            content = bytes(content)
        rebuilt.append(content)
    require(sorted(changed) == ['launcher', 'versionCode', 'versionName'], 'Expected exactly three manifest changes')
    result = struct.pack('<HHI', 3, 8, 8 + sum(map(len, rebuilt))) + b''.join(rebuilt)
    expected = []
    for tag, attributes in elements(parts, original):
        revised = []
        for namespace, key, kind, value in attributes:
            if tag == 'manifest' and key == 'versionCode':
                kind, value = 16, build_code
            elif tag == 'manifest' and key == 'versionName':
                kind, value = 3, version_name
            elif tag == 'activity' and key == 'name' and value in ('com.hyjs.wallet.WalletActivity', activity):
                value = activity
            revised.append((namespace, key, kind, value))
        expected.append((tag, revised))
    require(elements(chunks(result), values) == expected, 'Unrelated manifest contract changed')
    return result


def allow_wallet_subclass(raw):
    """Clear one class final flag; preserve every original DEX method byte."""
    require(raw[:8] in (b'dex\n035\0', b'dex\n037\0', b'dex\n038\0', b'dex\n039\0'), 'Unsupported DEX format')
    strings_size, strings_offset, types_size, types_offset = struct.unpack_from('<4I', raw, 56)
    count, offset = struct.unpack_from('<2I', raw, 96)
    target = None
    for index in range(count):
        position = offset + 32 * index
        class_index, access = struct.unpack_from('<2I', raw, position)
        require(class_index < types_size, 'Invalid DEX class type')
        string_index = struct.unpack_from('<I', raw, types_offset + 4 * class_index)[0]
        require(string_index < strings_size, 'Invalid DEX type descriptor')
        cursor = struct.unpack_from('<I', raw, strings_offset + 4 * string_index)[0]
        while raw[cursor] & 128:
            cursor += 1
        cursor += 1
        descriptor = raw[cursor:raw.index(0, cursor)]
        if descriptor == b'Lcom/hyjs/wallet/WalletActivity;':
            require(target is None, 'Expected one inherited WalletActivity')
            target = position + 4
    require(target is not None, 'Inherited WalletActivity is missing')
    result = bytearray(raw)
    access = struct.unpack_from('<I', result, target)[0]
    struct.pack_into('<I', result, target, access & ~16)
    result[12:32] = hashlib.sha1(result[32:]).digest()
    struct.pack_into('<I', result, 8, zlib.adler32(result[12:]) & 0xffffffff)
    require(all(a == b or 8 <= i < 32 or target <= i < target + 4 for i, (a, b) in enumerate(zip(raw, result))), 'DEX changed beyond flag and checksum')
    require(len(result) == len(raw), 'DEX length changed')
    return bytes(result), target


def has_update_class(raw):
    # Existing update layers own a separate DEX. Detect its exact class descriptor.
    return b'Lcom/hyjs/wallet/ClientUpdateActivity;\0' in raw and b'Lcom/hyjs/wallet/ClientUpdate;\0' in raw


def verify_managed(before_raw, after_raw, allowed):
    before, after = [dnfile.dnPE(data=value) for value in (before_raw, after_raw)]
    left, right = methods(before), methods(after)
    require(len(left) == len(right), 'Inherited method inventory differs')
    changed, engine = set(), 0
    for (name, i, old), (new_name, j, new) in zip(left, right):
        require((name, i, old.Signature.value, old.struct.Flags) == (new_name, j, new.Signature.value, new.struct.Flags), 'Managed ABI changed: ' + name)
        if not old.Rva:
            continue
        if exact_body(before, old) != exact_body(after, new):
            require(name in allowed, 'Unapproved method changed: ' + name)
            changed.add(name)
        else:
            engine += name.startswith('Battle.Engine')
    require(changed == set(allowed) and engine == 547, 'Managed target or Engine547 scope differs')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-apk', 'overlay', 'output', 'record'):
        parser.add_argument('--' + name, type=Path, required=True)
    parser.add_argument('--qa-origin', help='Loopback-only update UI QA; original Unity endpoints remain inherited')
    parser.add_argument('--gameplay-overlay', type=Path, help='Qualified gameplay45 input to the update layer')
    parser.add_argument('--pet-equip-overlay', type=Path, help='Qualified pet equipment input to the update layer')
    args = parser.parse_args()
    base, overlay, output, record = [path.resolve() for path in (args.base_apk, args.overlay, args.output, args.record)]
    require(output.is_relative_to(ROOT / 'build') and record.is_relative_to(ROOT / 'analysis'), 'Use owned project output paths')
    require(not output.exists() and not record.exists(), 'Fresh candidate and evidence paths required')
    proof = json.loads((overlay / 'manifest.json').read_text())
    require(common.digest(base) == proof['baseAPKSHA256'], 'Fixed base changed')
    pins = {ROOT / name: value for name, value in proof['sourcePins'].items()}
    require(not (args.gameplay_overlay and args.pet_equip_overlay), 'Choose one qualified managed input')
    gameplay = None
    if args.gameplay_overlay:
        gameplay_dir = args.gameplay_overlay.resolve()
        require(gameplay_dir.is_relative_to(ROOT / 'build'), 'Use owned gameplay overlay')
        gameplay = json.loads((gameplay_dir / 'manifest.json').read_text())
        require(gameplay['schema'] == 'hyjs-gameplay45-v1' and gameplay['baseAPKSHA256'] == proof['baseAPKSHA256'], 'Gameplay base differs')
        pins.update({ROOT / name: value for name, value in gameplay['sourcePins'].items()})
    pet = None
    if args.pet_equip_overlay:
        pet_dir = args.pet_equip_overlay.resolve()
        require(pet_dir.is_relative_to(ROOT / 'build'), 'Use owned pet equipment overlay')
        pet = json.loads((pet_dir / 'manifest.json').read_text())
        require(pet['schema'] == 'hyjs-pet-equip48-overlay-v1' and pet['inputAPK_SHA256'] == proof['baseAPKSHA256'], 'Pet equipment base differs')
        require(pet['originalEngineMethodsUnchanged'] == 547 and pet['originalMetadataABIAndTokensPreserved'] is True, 'Pet equipment qualification missing')
        require({item['method'] for item in pet['changes']} == {'UI.PetEquipAwakeTab::SetCost', 'UI.PetEquipStrengthTab::isCanStrength'}, 'Unexpected pet equipment method scope')
        pins.update({ROOT / name: value for name, value in pet['sourcePins'].items()})
    pins[Path(__file__).resolve()] = common.digest(Path(__file__))
    for path, digest in pins.items():
        require(path.resolve().is_relative_to(ROOT) and common.digest(path) == digest, 'Build source changed')
    sdk = Path.home() / 'Library/Android/sdk/build-tools/33.0.0'
    jdk = Path('/opt/homebrew/opt/openjdk@21')
    env = dict(os.environ, JAVA_HOME=str(jdk), PATH=str(jdk / 'bin') + os.pathsep + os.environ.get('PATH', ''))
    require(shutil.disk_usage(ROOT).free > base.stat().st_size * 2 + 100_000_000, 'Insufficient packaging space')
    certificate = common._certificate(common.run([sdk / 'apksigner', 'verify', '--print-certs', base], env))
    inherited, compressed = {}, {}
    with (ROOT / 'build/.client-build.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        require(not output.exists(), 'Output appeared while waiting for build lease')
        with zipfile.ZipFile(base) as incoming:
            before = incoming.read(common.DLL)
            after = (overlay / 'Assembly-CSharp.dll').read_bytes()
            update_input = before
            if gameplay is not None:
                require(hashlib.sha256(before).hexdigest() == gameplay['inputManagedSHA256'], 'Gameplay managed base differs')
                update_input = (gameplay_dir / 'Assembly-CSharp.dll').read_bytes()
                require(hashlib.sha256(update_input).hexdigest() == gameplay['outputManagedSHA256'], 'Gameplay managed output differs')
                verify_managed(before, update_input, [item['name'] for item in gameplay['allowedChangedMethods']])
            if pet is not None:
                require(hashlib.sha256(before).hexdigest() == pet['inputManagedSHA256'], 'Pet equipment managed base differs')
                update_input = (pet_dir / 'Assembly-CSharp.dll').read_bytes()
                require(hashlib.sha256(update_input).hexdigest() == pet['outputManagedSHA256'], 'Pet equipment managed output differs')
                verify_managed(before, update_input, [item['method'] for item in pet['changes']])
            require(hashlib.sha256(update_input).hexdigest() == proof['inputManagedSHA256'] and hashlib.sha256(after).hexdigest() == proof['outputManagedSHA256'], 'Managed qualification differs')
            verify_managed(update_input, after, proof['allowedChangedMethods'])
            native, flag_offset = allow_wallet_subclass(incoming.read('classes2.dex'))
            dex_names = [name for name in incoming.namelist() if name.startswith('classes') and name.endswith('.dex')]
            update_dex = [name for name in dex_names if has_update_class(incoming.read(name))]
            require(len(update_dex) <= 1, 'Ambiguous inherited update layer')
            dex_entry = update_dex[0] if update_dex else 'classes' + str(len(dex_names) + 1) + '.dex'
            require(bool(update_dex) or dex_entry not in incoming.namelist(), 'New DEX would replace an unrelated entry')
            payload = {
                common.DLL: after,
                'assets/bin/Data/Managed/HYJSClientUpdate.dll': (overlay / 'HYJSClientUpdate.dll').read_bytes(),
                'AndroidManifest.xml': patch_manifest(incoming.read('AndroidManifest.xml'), proof['buildCode'], proof['versionName']),
                'classes2.dex': native,
                dex_entry: (overlay / 'classes.dex').read_bytes()
            }
            if gameplay is not None:
                helper_entry = 'assets/bin/Data/Managed/HYJS.Gameplay.dll'
                helper = ROOT / gameplay['payloadFiles'][helper_entry]
                require(helper.resolve().is_relative_to(gameplay_dir), 'Gameplay helper outside qualified output')
                payload[helper_entry] = helper.read_bytes()
                require(hashlib.sha256(payload[helper_entry]).hexdigest() == gameplay['replacementEntries'][helper_entry], 'Gameplay helper changed')
            if pet is not None:
                helper_entry = 'assets/bin/Data/Managed/SoulFlowerPetEquip48.dll'
                payload[helper_entry] = (pet_dir / 'SoulFlowerPetEquip48.dll').read_bytes()
                require(hashlib.sha256(payload[helper_entry]).hexdigest() == pet['helperSHA256'], 'Pet equipment helper changed')
            if args.qa_origin:
                from urllib.parse import urlsplit
                origin = urlsplit(args.qa_origin)
                require(origin.scheme == 'http' and origin.hostname == '127.0.0.1' and origin.port and origin.path in ('', '/') and not origin.query and not origin.fragment and not origin.username, 'QA origin must be explicit loopback HTTP')
                payload['assets/local-wallet.json'] = json.dumps({'origin': args.qa_origin.rstrip('/')}, separators=(',', ':')).encode()
        with tempfile.TemporaryDirectory(prefix='hyjs-client-update-', dir=ROOT / 'build') as directory:
            temp = Path(directory)
            unsigned, aligned, signed = [temp / name for name in ('unsigned.apk', 'aligned.apk', 'signed.apk')]
            with zipfile.ZipFile(base) as incoming, base.open('rb') as source, zipfile.ZipFile(unsigned, 'w') as target:
                for info in incoming.infolist():
                    if common._signature(info.filename):
                        continue
                    if info.filename in payload:
                        target.writestr(copy.copy(info), payload[info.filename])
                    else:
                        inherited[info.filename] = common._hash_entry(incoming, info.filename)
                        compressed[info.filename] = compressed_hash(source, info)
                        copy_original_record(source, target, info)
                for name in set(payload) - set(incoming.namelist()):
                    target.writestr(name, payload[name], compress_type=zipfile.ZIP_DEFLATED)
            common.run([sdk / 'zipalign', '-f', '-p', '4', unsigned, aligned], env)
            unsigned.unlink()
            common.run([sdk / 'apksigner', 'sign', '--ks', ROOT / '.local-signing/debug.keystore', '--ks-pass', 'pass:android', '--key-pass', 'pass:android', '--out', signed, aligned], env)
            aligned.unlink()
            require(common._certificate(common.run([sdk / 'apksigner', 'verify', '--verbose', '--print-certs', signed], env)) == certificate, 'Signing identity changed')
            common.run([sdk / 'zipalign', '-c', '-p', '4', signed], env)
            hashes = {name: hashlib.sha256(value).hexdigest() for name, value in payload.items()}
            with zipfile.ZipFile(signed) as verified, signed.open('rb') as source:
                names = verified.namelist()
                require(len(names) == len(set(names)) and {n for n in names if not common._signature(n)} == set(inherited) | set(payload), 'APK inventory changed')
                for name, digest in {**inherited, **hashes}.items():
                    require(common._hash_entry(verified, name) == digest, 'APK payload differs: ' + name)
                for name, digest in compressed.items():
                    require(compressed_hash(source, verified.getinfo(name)) == digest, 'Inherited compressed bytes changed: ' + name)
            for path, digest in pins.items():
                require(common.digest(path) == digest, 'Build input changed while packaging')
            require(common.digest(base) == proof['baseAPKSHA256'], 'Protected base changed')
            result = dict(status='SIGNED_ALIGNED_UPDATE_CANDIDATE_NOT_INSTALLED', baseAPKSHA256=proof['baseAPKSHA256'], binarySHA256=common.digest(signed), outputAPK=str(output.relative_to(ROOT)), replacementEntries=hashes, originalEngineMethodsExact=547, inheritedEntryCount=len(inherited), allOtherEntriesAndCompressedPayloadsExact=True, inheritedWalletDexOnlyFinalFlagAndChecksumChanged=True, walletClassAccessFlagOffset=flag_offset, buildCode=proof['buildCode'], versionName=proof['versionName'], signerCertificateSHA256=certificate, signatureVerified=True, zipalignVerified=True, updateUIQAOnly=bool(args.qa_origin), originalGameEndpointsInherited=True, installed=False, promoted=False)
            signed.replace(output)
            if signed.with_suffix('.apk.idsig').exists():
                signed.with_suffix('.apk.idsig').unlink()
    record.parent.mkdir(parents=True, exist_ok=True)
    record.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({key: result[key] for key in ('status', 'outputAPK', 'binarySHA256', 'buildCode', 'versionName')}))


if __name__ == '__main__':
    main()
