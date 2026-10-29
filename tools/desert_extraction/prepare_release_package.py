"""Prepare a desert and trial release overlay from v80 after reviewed, pinned QA evidence.

This script does not sign, install, deploy, or promote a QA overlay/APK.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path
import re
import shutil
import sys
import zipfile

ROOT = Path(__file__).resolve().parents[2]
BASE_SHA = '25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
QA_ORIGIN = 'http://127.0.0.1:28979'
BUILD_CODE = 2026102901
VERSION_NAME = '1.0.83'
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
MAP_HELPER = 'assets/bin/Data/Managed/SoulFlowerDesert53.dll'
V4_HELPER = 'assets/bin/Data/Managed/SoulFlowerDesertV4.dll'
SHOP_HELPER = 'assets/bin/Data/Managed/SoulFlowerGGOShop.dll'
REQUIRED_CASES = (
    'entry_position_protection', 'map_points_visibility_navigation',
    'mine_fish_gather_balance_reward', 'gather_first_frame',
    'shared_monster_patrol_battle', 'monster_first_frame',
    'boss_battle_and_settlement', 'six_color_trade', 'pvp_protection_balance',
    'extraction_cancel_complete', 'disconnect_and_process_recovery',
    'server_load_and_integration', 'admin_configuration', 'trial_store_refresh_points',
)


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha_bytes(data):
    return hashlib.sha256(data).hexdigest()


def sha_file(path):
    with Path(path).open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def local_file(value):
    path = (ROOT / value).resolve()
    require(path.is_relative_to(ROOT), 'Evidence file must be inside project: ' + str(value))
    require(path.is_file(), 'Evidence file missing: ' + str(value))
    return path


def add_pin(pins, path, expected=None):
    path = Path(path).resolve()
    require(path.is_relative_to(ROOT) and path.is_file(), 'Pinned file must be inside project: ' + str(path))
    actual = sha_file(path)
    require(expected is None or actual == expected, 'File pin differs: ' + str(path.relative_to(ROOT)))
    key = str(path.relative_to(ROOT))
    require(key not in pins or pins[key] == actual, 'Conflicting file pin: ' + key)
    pins[key] = actual


def read_json(path):
    return json.loads(Path(path).read_text(encoding='utf-8'))


def reject_qa_inputs(*records):
    for record in records:
        require(not record.get('qaOnly') and not record.get('neverPromote')
                and not record.get('diagnosticOnly') and not record.get('qaOrigin'),
                'QA/neverPromote overlays cannot be release inputs')


def review_evidence(path, qa_sha):
    """Check the review's coverage and immutable cited files; humans assess the evidence."""
    path = local_file(path)
    review = read_json(path)
    require(review.get('qaAPKSHA256') == qa_sha, 'Review is for a different QA APK')
    require(review.get('reviewed') is True and isinstance(review.get('reviewer'), str)
            and review['reviewer'].strip(), 'A completed named review is required')
    require(isinstance(review.get('scope'), str) and len(review['scope'].strip()) >= 30,
            'Describe test versions, configurations and evidence reuse boundaries')
    cases = review.get('cases')
    require(isinstance(cases, dict), 'Acceptance cases missing')
    missing = [name for name in REQUIRED_CASES if not cases.get(name)]
    require(not missing, 'Missing accepted evidence: ' + ', '.join(missing))
    pins = {}
    for name in REQUIRED_CASES:
        refs = cases[name]
        require(isinstance(refs, list) and refs, 'Evidence references missing: ' + name)
        for ref in refs:
            require(isinstance(ref, dict) and set(ref) == {'path', 'sha256'}, 'Invalid evidence reference: ' + name)
            require(isinstance(ref['sha256'], str) and re.fullmatch(r'[0-9a-f]{64}', ref['sha256']),
                    'Invalid evidence SHA: ' + name)
            cited = local_file(ref['path'])
            require(cited.stat().st_size > 0, 'Evidence file is empty: ' + name)
            add_pin(pins, cited, ref['sha256'])
    times = review.get('latencyMs')
    require(isinstance(times, dict), 'First-frame timing measurements missing')
    for name in ('gather', 'monster'):
        value = times.get(name)
        require(type(value) in (int, float) and math.isfinite(value) and 0 <= value <= 1000,
                name + ' touch-to-first-frame must be measured at <= 1000 ms')
    add_pin(pins, path)
    return review, pins


def load_inputs(args, qa_sha, review, pins):
    """Validate raw static outputs; a qaOnly/neverPromote overlay cannot be an input."""
    sys.path.insert(0, str(ROOT))
    import dnfile
    from dncil.cil.body.reader import read_method_body_from_bytes
    from tools.awake_diff_rows.build import methods
    from tools.client_remote import build as remote
    from tools.desert_extraction import build_v78_adapter, build_field_patch, build_combined83, package_resources
    from tools.client_trial_refresh82 import build as trial82
    from tools.local_asset_ready.remove_gate import exact_body

    base, map_dir, preview_dir, bundle, qa_apk = (
        path.resolve() for path in (args.base_apk, args.map_overlay, args.preview, args.bundle, args.qa_apk)
    )
    require(package_resources.sha_file(base) == BASE_SHA, 'Wrong v80 base APK')
    qa_apk = local_file(qa_apk)
    qa_manifest_path = local_file(args.qa_manifest)
    qa_record_path = local_file(args.qa_record)
    qa_manifest, qa_record = read_json(qa_manifest_path), read_json(qa_record_path)
    require(qa_manifest.get('qaOnly') is True and qa_manifest.get('neverPromote') is True
            and qa_manifest.get('qaOrigin') == QA_ORIGIN and qa_manifest.get('baseAPKSHA256') == BASE_SHA,
            'Expected a loopback-only QA manifest')
    require(qa_record.get('qaOnly') is True and qa_record.get('neverPromote') is True
            and qa_record.get('binarySHA256') == qa_sha and qa_record.get('baseAPKSHA256') == BASE_SHA
            and qa_record.get('signatureVerified') is True and qa_record.get('zipalignVerified') is True
            and qa_record.get('allOtherEntriesAndCompressedPayloadsExact') is True
            and not qa_record.get('diagnosticOnly'),
            'QA packaging record differs')
    require((qa_record.get('buildCode'), qa_record.get('versionName')) == (BUILD_CODE, VERSION_NAME),
            'QA version differs from tested 1.0.83')
    add_pin(pins, qa_apk, qa_sha)
    for path in (qa_manifest_path, qa_record_path):
        add_pin(pins, path)

    map_manifest_path, field_manifest_path = map_dir / 'manifest.json', preview_dir / 'manifest.json'
    map_manifest_raw, field_manifest_raw = map_manifest_path.read_bytes(), field_manifest_path.read_bytes()
    m, v = json.loads(map_manifest_raw), json.loads(field_manifest_raw)
    reject_qa_inputs(m, v)
    require(m.get('schema') == 'hyjs-desert53-runtime-adapter-v2'
            and m.get('status') == 'STATIC_VERIFIED_OVERLAY_NOT_PACKAGED'
            and m.get('apkSHA256') == BASE_SHA and m.get('desertEntrySelectable') is True
            and m.get('desertLoginResume') is True and m.get('walkOnly') is False,
            'Unqualified v80 map input')
    require(v.get('schema') == 'hyjs-desert-v4-scene-overlay-v2'
            and v.get('status') == 'STATIC_PREVIEW_NOT_FOR_RELEASE' and v.get('apkSHA256') == BASE_SHA,
            'Unqualified v80 field input')
    qa_sources = qa_manifest.get('sourcePins')
    require(isinstance(qa_sources, dict), 'QA source pins missing')
    sources = {}
    for record in (m, v):
        items = record.get('sourceSHA256')
        require(isinstance(items, dict) and items, 'Static input source pins missing')
        for name, digest in items.items():
            require(name not in sources or sources[name] == digest, 'Conflicting static source pin: ' + name)
            require(qa_sources.get(name) == digest, 'Tested QA source differs: ' + name)
            add_pin(pins, local_file(name), digest)
            sources[name] = digest

    native_map = (map_dir / 'Assembly-CSharp.dll').read_bytes()
    native_a = (preview_dir / 'Assembly-CSharp.dll').read_bytes()
    map_helper = (map_dir / 'SoulFlowerDesert53.dll').read_bytes()
    v4_helper = (preview_dir / 'SoulFlowerDesertV4.dll').read_bytes()
    map_raw = bundle.read_bytes()
    require(sha_bytes(native_map) == m['outputManagedSHA256'] == v['inputManagedSHA256']
            and sha_bytes(native_a) == v['outputManagedSHA256']
            and sha_bytes(map_helper) == m['helperSHA256']
            and sha_bytes(v4_helper) == v['helperSHA256'], 'Static output identity differs')
    with zipfile.ZipFile(base) as z:
        names = z.namelist()
        require(len(names) == len(set(names)) and ENTRY in names
                and all(name not in names for name in (package_resources.BUNDLE_ENTRY, MAP_HELPER, V4_HELPER)),
                'Base APK entries differ')
        original, original_shop, dex = z.read(ENTRY), z.read(SHOP_HELPER), z.read('classes2.dex')
        catalog = z.read(package_resources.CATALOG_ENTRY)
        production_origin = json.loads(z.read('assets/local-wallet.json'))['origin'].rstrip('/')
        require(production_origin and production_origin != QA_ORIGIN, 'Invalid base production origin')
        expected_catalog = package_resources.compose_catalog(catalog, map_raw)
        require(sha_bytes(original) == m['inputManagedSHA256']
                and sha_bytes(catalog) == m['originalCatalogSHA256']
                and sha_bytes(expected_catalog) == m['updatedCatalogSHA256']
                and expected_catalog == (map_dir / 'smallassetmanifest').read_bytes(), 'Base or catalog differs')
        inherited = {name: sha_bytes(z.read(name)) for name in names
                     if name.startswith('lib/') or (name.startswith('classes') and name.endswith('.dex'))}
    map_check = build_v78_adapter.verify(original, native_map, len(map_raw), False, True, True)
    field_check = build_field_patch.verify(native_map, native_a)
    require(map_check['originalEngineMethodsUnchanged'] == field_check['originalEngineMethodsUnchanged'] == 547
            and map_check['qualityFilterV80Exact'] and field_check['qualityFilterV80Exact'],
            'Original battle engine or v80 quality fix differs')

    combined_dir = args.combined_static.resolve()
    require(combined_dir.is_relative_to(ROOT / 'analysis/desert-extraction-v4-client')
            and combined_dir.name == 'static' and combined_dir.parent.name.startswith('combined83')
            and not (combined_dir / 'manifest.json').exists()
            and not (combined_dir / 'resource-manifest.json').exists(),
            'Independent combined static input required; QA overlays cannot be promoted')
    combined_proof_path = local_file(combined_dir / 'build-proof.json')
    combined_proof = read_json(combined_proof_path)
    reject_qa_inputs(combined_proof)
    require(combined_proof.get('status') == 'STATIC_PREVIEW_NOT_FOR_RELEASE'
            and combined_proof.get('baseAPKSHA256') == BASE_SHA
            and combined_proof.get('desertInputManagedSHA256') == sha_bytes(native_a),
            'Combined static proof differs')
    combined_sources = combined_proof.get('sourcePins')
    require(isinstance(combined_sources, dict) and combined_sources, 'Combined source pins missing')
    for name, digest in combined_sources.items():
        require(qa_sources.get(name) == digest, 'Tested combined source differs: ' + name)
        add_pin(pins, local_file(name), digest)
    proof_name = str(combined_proof_path.relative_to(ROOT))
    require(qa_sources.get(proof_name) is not None, 'QA did not pin combined build proof')
    add_pin(pins, combined_proof_path, qa_sources[proof_name])
    combined_path = local_file(combined_dir / 'Assembly-CSharp.dll')
    shop_path = local_file(combined_dir / 'SoulFlowerGGOShop.dll')
    native_final, shop_helper = combined_path.read_bytes(), shop_path.read_bytes()
    require(sha_bytes(native_final) == combined_proof.get('combinedManagedSHA256')
            and sha_bytes(shop_helper) == combined_proof.get('shopHelperSHA256'),
            'Combined static payload differs')
    add_pin(pins, combined_path, combined_proof['combinedManagedSHA256'])
    add_pin(pins, shop_path, combined_proof['shopHelperSHA256'])
    trial_check = trial82.check_main(native_a, native_final)
    changed = build_combined83.composite_check(original, native_final, map_check, field_check)
    require(trial_check == combined_proof.get('trialHookCheck')
            and changed == combined_proof.get('allChangedMethods'), 'Combined method proof differs')
    shop_audit = build_combined83.audit_shop(shop_helper)
    require(shop_audit == combined_proof.get('shopOriginAudit'), 'Combined shop audit differs')
    def icon_signature(raw):
        candidates = [method for name, _, method in methods(dnfile.dnPE(data=raw))
                      if name == 'HYJS.GGO.GGOShop::RegisterIcons']
        require(len(candidates) == 1, 'Icon registration method differs')
        method = candidates[0]
        return method.Signature.value, method.Flags.mdPrivate, method.Flags.mdStatic
    require(icon_signature(original_shop) == icon_signature(shop_helper), 'Private icon registration ABI differs')
    pe2 = dnfile.dnPE(data=native_final)
    new = methods(pe2)

    with zipfile.ZipFile(qa_apk) as z:
        names = z.namelist()
        require(len(names) == len(set(names)), 'Duplicate QA APK entries')
        qa_native = z.read(ENTRY)
        qa_replacements = qa_record.get('replacementEntries')
        require(isinstance(qa_replacements, dict) and qa_replacements, 'QA packaged entry pins missing')
        for name, digest in qa_replacements.items():
            require(name in names and sha_bytes(z.read(name)) == digest, 'QA APK entry differs: ' + name)
        for name, raw in ((MAP_HELPER, map_helper), (V4_HELPER, v4_helper), (SHOP_HELPER, shop_helper),
                          (package_resources.BUNDLE_ENTRY, map_raw),
                          (package_resources.CATALOG_ENTRY, expected_catalog)):
            require(z.read(name) == raw, 'QA-tested payload differs: ' + name)
        require(json.loads(z.read('assets/local-wallet.json')).get('origin') == QA_ORIGIN,
                'QA wallet origin differs')
    require(qa_manifest.get('qaOriginProof', {}).get('shopActualPayloadAudit') == shop_audit,
            'QA actual shop helper audit differs')
    for name in (ENTRY, MAP_HELPER, V4_HELPER, SHOP_HELPER):
        require(qa_manifest.get('replacementEntries', {}).get(name) == qa_replacements.get(name),
                'QA source/package mismatch: ' + name)
    targets = {name: (old.replace('http://127.0.0.1:8088', production_origin), new)
               for name, (old, new) in remote.targets_for(QA_ORIGIN).items()}
    original_targets = remote.targets_for
    try:
        remote.targets_for = lambda _origin: targets
        origin_check = remote.verify(native_final, qa_native, QA_ORIGIN)
    finally:
        remote.targets_for = original_targets
    require(origin_check['onlyFourHTTPOriginLiteralsChanged']
            and origin_check['originalEngineMethodsUnchanged'] == 547, 'QA/formal origin boundary differs')
    for name, _, method in new:
        if method.Rva:
            for instruction in read_method_body_from_bytes(exact_body(pe2, method)).instructions:
                if instruction.opcode.name == 'ldstr':
                    value = pe2.net.user_strings.get(instruction.operand.value & 0xffffff).value
                    require('127.0.0.1:28979' not in value, 'QA origin in release managed code: ' + name)
    for label, raw in (('map', map_helper), ('v4', v4_helper)):
        pe = dnfile.dnPE(data=raw)
        for name, _, method in methods(pe):
            if method.Rva:
                for instruction in read_method_body_from_bytes(exact_body(pe, method)).instructions:
                    if instruction.opcode.name == 'ldstr':
                        value = pe.net.user_strings.get(instruction.operand.value & 0xffffff).value
                        require('://' not in value, 'Absolute origin in ' + label + ' helper: ' + name)
    for relative in ('tools/desert_extraction/prepare_release_package.py',
                     'tools/desert_extraction/package_resources.py', 'tools/client_release/package_native.py',
                     'tools/client_remote/build.py', 'tools/client_remote/config.py',
                     'tools/awake_diff_rows/build.py', 'tools/local_asset_ready/remove_gate.py',
                     'tools/client_activity/build.py', 'tools/client_update/package.py',
                     'tools/client_compat63/dex_patch.py'):
        add_pin(pins, local_file(relative))
    data = {'Assembly-CSharp.dll': native_final, 'SoulFlowerDesert53.dll': map_helper,
            'SoulFlowerDesertV4.dll': v4_helper, 'SoulFlowerGGOShop.dll': shop_helper, 'smallassetmanifest': expected_catalog,
            'map_desertfield_001': map_raw}
    return locals()


def prepare(args):
    out = args.output.resolve()
    require(out.is_relative_to(ROOT / 'analysis/desert-extraction-v4-client') and not out.exists(),
            'Use a fresh release output directory in the project analysis directory')
    # Fail closed on missing evidence before importing the packaging dependencies.
    local_file(args.acceptance)
    qa_path = local_file(args.qa_apk)
    qa_sha = sha_file(qa_path)
    review, evidence_pins = review_evidence(args.acceptance, qa_sha)
    state = load_inputs(args, qa_sha, review, evidence_pins)
    pins, data = state['pins'], state['data']
    package_resources = state['package_resources']
    created = False
    try:
        out.mkdir(parents=True)
        created = True
        for name, raw in data.items():
            (out / name).write_bytes(raw)
        # Keep the independently built static proofs for later source-pin rechecks.
        for name, raw in (('map-input-manifest.json', state['map_manifest_raw']),
                          ('field-input-manifest.json', state['field_manifest_raw'])):
            target = out / name
            target.write_bytes(raw)
            add_pin(pins, target)
        files = {ENTRY: 'Assembly-CSharp.dll', MAP_HELPER: 'SoulFlowerDesert53.dll',
                 V4_HELPER: 'SoulFlowerDesertV4.dll', SHOP_HELPER: 'SoulFlowerGGOShop.dll'}
        payload = {entry: str((out / name).relative_to(ROOT)) for entry, name in files.items()}
        proof = {'schema': 'hyjs-desert-trial83-formal-managed-v1', 'status': 'READY_FOR_SIGNING_NOT_RELEASED',
                 'qaOnly': False, 'neverPromote': False, 'managedOnly': True, 'baseAPKSHA256': BASE_SHA,
                 'inputManagedSHA256': sha_bytes(state['original']), 'outputManagedSHA256': sha_bytes(data['Assembly-CSharp.dll']),
                 'inputDexSHA256': sha_bytes(state['dex']), 'inheritedJavaByteExact': True,
                 'inheritedJavaPublicProtectedABIExact': True, 'inheritedJavaEntrySHA256': state['inherited'],
                 'payloadFiles': payload, 'replacementEntries': {entry: sha_bytes(data[name]) for entry, name in files.items()},
                 'allowedChangedMethods': sorted(state['changed']), 'sourcePins': pins,
                 'originalEngineMethodsExact': 547, 'qualityFilterV80Exact': True,
                 'qaAPKSHA256': state['qa_sha'], 'acceptanceReview': str(local_file(args.acceptance).relative_to(ROOT)),
                 'qaOriginDifferedOnlyInFourVerifiedMethods': True,
                 'deviceExecuted': False, 'serverExecuted': False}
        (out / 'manifest.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
        resources = {package_resources.CATALOG_ENTRY: 'smallassetmanifest', package_resources.BUNDLE_ENTRY: 'map_desertfield_001'}
        resource_proof = {'schema': 'hyjs-desert-v4-map-resource-overlay-v1', 'baseAPKSHA256': BASE_SHA,
                          'bundleSHA256': package_resources.BUNDLE_SHA, 'bundleBytes': len(data['map_desertfield_001']),
                          'originalCatalogSHA256': sha_bytes(state['catalog']), 'updatedCatalogSHA256': sha_bytes(data['smallassetmanifest']),
                          'payloadFiles': {entry: str((out / name).relative_to(ROOT)) for entry, name in resources.items()},
                          'replacementEntries': {entry: sha_bytes(data[name]) for entry, name in resources.items()},
                          'sourcePins': pins, 'releaseReady': True,
                          'deviceExecuted': False}
        (out / 'resource-manifest.json').write_text(json.dumps(resource_proof, ensure_ascii=False, indent=2) + '\n')
        # Detect concurrent edits between acceptance and emission.
        for name, digest in pins.items():
            require(sha_file(local_file(name)) == digest, 'Pinned input changed during preparation: ' + name)
        require(package_resources.sha_file(state['base']) == BASE_SHA, 'Base changed during preparation')
    except Exception:
        if created:
            shutil.rmtree(out)
        raise
    print(json.dumps({'output': str(out), 'qaAPKSHA256': state['qa_sha'], 'buildCode': BUILD_CODE,
                      'versionName': VERSION_NAME, 'originalEngineMethodsExact': 547,
                      'qualityFilterV80Exact': True, 'signed': False, 'deviceExecuted': False}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--list-gates', action='store_true', help='List the required reviewed evidence scopes')
    parser.add_argument('--base-apk', type=Path)
    parser.add_argument('--map-overlay', type=Path)
    parser.add_argument('--preview', type=Path)
    parser.add_argument('--combined-static', type=Path)
    parser.add_argument('--bundle', type=Path)
    parser.add_argument('--qa-apk', type=Path)
    parser.add_argument('--qa-manifest', type=Path)
    parser.add_argument('--qa-record', type=Path)
    parser.add_argument('--acceptance', type=Path)
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    if args.list_gates:
        print('\n'.join(REQUIRED_CASES))
        return
    for name in ('base_apk', 'map_overlay', 'preview', 'combined_static', 'bundle', 'qa_apk', 'qa_manifest', 'qa_record', 'acceptance', 'output'):
        require(getattr(args, name) is not None, 'Missing --' + name.replace('_', '-'))
    prepare(args)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, KeyError, json.JSONDecodeError) as error:
        print('Release preparation blocked: ' + str(error), file=sys.stderr)
        sys.exit(1)
