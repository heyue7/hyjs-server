"""Build independent combined-86 static and loopback QA inputs; never sign or release."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))

BASE_SHA = '25026a1f180d164ba2b191e4cc7e558e20f7a761b740fb7b28d6205d6756112a'
ENTRY = 'assets/bin/Data/Managed/Assembly-CSharp.dll'
MAP = 'assets/bin/Data/Managed/SoulFlowerDesert53.dll'
FIELD = 'assets/bin/Data/Managed/SoulFlowerDesertV4.dll'
SHOP = 'assets/bin/Data/Managed/SoulFlowerGGOShop.dll'
FEEDBACK = 'assets/bin/Data/Managed/HYJS.Feedback65.dll'
FRAGMENT = 'assets/bin/Data/Managed/HYJS.FragmentBoxes85.dll'
DUNGEON = 'assets/bin/Data/Managed/HYJS.DungeonContent.dll'
WANTED87 = 'assets/bin/Data/Managed/HYJS.WantedSkip87.dll'
SCHEMA_QA = 'hyjs-desert-trial-feedback-fragment86-qa-managed-v1'
SCHEMA_FORMAL = 'hyjs-desert-trial-feedback-fragment86-formal-managed-v1'
QA_ORIGIN = 'http://127.0.0.1:28979'
VERSION = '1.0.86'
BUILD_CODE = 2026102904
def require(ok, message):
    if not ok:
        raise ValueError(message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def file_sha(path):
    with Path(path).open('rb') as f:
        return hashlib.file_digest(f, 'sha256').hexdigest()


def local(path):
    p = Path(path).resolve()
    require(p.is_relative_to(ROOT) and p.is_file(), 'Expected a local project file: ' + str(p))
    return p


def add_pin(pins, path, expected=None):
    p = local(path)
    value = file_sha(p)
    require(expected is None or value == expected, 'Pin differs: ' + str(p))
    key = str(p.relative_to(ROOT))
    require(key not in pins or pins[key] == value, 'Conflicting pin: ' + key)
    pins[key] = value


def check_pins(pins):
    require(isinstance(pins, dict) and bool(pins), 'Source pins missing')
    for name, value in pins.items():
        require(isinstance(name, str) and isinstance(value, str)
                and re.fullmatch(r'[0-9a-f]{64}', value) is not None, 'Invalid source pin')
        add_pin({}, ROOT / name, value)


def toolchain_pins(*runtimes):
    pins = {}
    for runtime in runtimes:
        runtime = Path(runtime).resolve()
        compilers = list((runtime/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
        require(bool(compilers), 'Native toolchain compiler missing: ' + str(runtime))
        for path in (runtime/'dotnet/dotnet', runtime/'injector/bin/Debug/net8.0/dnlib.dll', compilers[0]):
            require(path.is_file(), 'Native toolchain file missing: ' + str(path))
            pins[str(path)] = file_sha(path)
    return pins


def check_toolchain_pins(pins):
    require(isinstance(pins, dict) and bool(pins), 'Toolchain pins missing')
    for name, value in pins.items():
        path = Path(name)
        require(path.is_absolute() and path.is_file() and file_sha(path) == value, 'Toolchain pin differs: ' + name)


def read_json(path):
    return json.loads(Path(path).read_text(encoding='utf-8'))


def write_json(path, value):
    Path(path).write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def no_qa(*records):
    for record in records:
        require(not any(record.get(k) for k in ('qaOnly', 'neverPromote', 'diagnosticOnly', 'qaOrigin')),
                'QA or diagnostic input cannot be promoted')


def required_field_source_pins():
    from tools.desert_extraction import build_field_patch
    return {'tools/desert_extraction/' + name
            for name in (*build_field_patch.SOURCES, 'InjectorField.cs', 'build_field_patch.py',
                         'assets/evacuation-npc.png')}


def validate_field_manifest(field_dir, main_sha, helper_sha):
    manifest = read_json(Path(field_dir)/'manifest.json')
    no_qa(manifest); check_pins(manifest.get('sourceSHA256'))
    require(manifest.get('schema') == 'hyjs-desert-v4-scene-overlay-v2'
            and manifest.get('status') == 'STATIC_PREVIEW_NOT_FOR_RELEASE'
            and manifest.get('apkSHA256') == BASE_SHA, 'Field preview manifest is not qualified')
    require(set(manifest['sourceSHA256']) == required_field_source_pins(), 'Field source pin inventory differs')
    for value in (main_sha, helper_sha):
        require(isinstance(value, str) and re.fullmatch(r'[0-9a-f]{64}', value) is not None,
                'Explicit lower-case field SHA required')
    require(file_sha(Path(field_dir)/'Assembly-CSharp.dll') == manifest['outputManagedSHA256'] == main_sha
            and file_sha(Path(field_dir)/'SoulFlowerDesertV4.dll') == manifest['helperSHA256'] == helper_sha,
            'Explicit field SHA, manifest and actual payload differ')
    return manifest


def managed_diff(before, after):
    """Return every changed body, enforcing the original method ABI and 547 engines."""
    import dnfile
    from tools.awake_diff_rows.build import methods
    from tools.local_asset_ready.remove_gate import exact_body
    from tools.desert_extraction import build_v78_adapter
    a, b = dnfile.dnPE(data=before), dnfile.dnPE(data=after)
    for table in ('TypeDef', 'Field', 'MethodDef', 'Param'):
        require(len(getattr(a.net.mdtables, table).rows) == len(getattr(b.net.mdtables, table).rows),
                'Definition count changed: ' + table)
    require(not getattr(b.net.mdtables, 'FieldPtr', None) and not getattr(b.net.mdtables, 'MethodPtr', None),
            'Unity pointer tables forbidden')
    old, new = methods(a), methods(b)
    require(len(old) == len(new), 'Method inventory changed')
    changed, engine, quality = [], 0, False
    for (name, rid, x), (name2, rid2, y) in zip(old, new):
        require((name, rid, x.Signature.value, x.struct.Flags, x.struct.ImplFlags) ==
                (name2, rid2, y.Signature.value, y.struct.Flags, y.struct.ImplFlags), 'Method ABI changed: ' + name)
        if not x.Rva:
            require(not y.Rva, 'Method body added: ' + name)
            continue
        left, right = exact_body(a, x), exact_body(b, y)
        if name == build_v78_adapter.QUALITY_METHOD:
            require(digest(left) == build_v78_adapter.QUALITY_PIN and left == right, 'v80 quality fix changed')
            quality = True
        if name.startswith('Battle.Engine'):
            require(left == right, 'Original engine body changed: ' + name)
            engine += 1
        if left != right:
            changed.append({'method': name, 'token': f'0x{0x06000000 | rid:08x}',
                            'beforeSHA256': digest(left), 'afterSHA256': digest(right)})
    require(engine == 547 and quality, 'Engine inventory or quality fix differs')
    return changed


def verify_embedded_evacuation_icon(helper):
    import dnfile
    pe = dnfile.dnPE(data=helper)
    found = [r for r in pe.net.resources if str(r.name) == 'HYJS.DesertV4.Evacuation.png']
    expected = local(ROOT/'tools/desert_extraction/assets/evacuation-npc.png').read_bytes()
    require(len(found) == 1 and isinstance(found[0].data, bytes) and found[0].data == expected,
            'Embedded evacuation icon differs from pinned source')
    return digest(expected)


def no_qa_origin_in_main(raw):
    import dnfile
    from dncil.cil.body.reader import read_method_body_from_bytes
    from tools.awake_diff_rows.build import methods
    from tools.local_asset_ready.remove_gate import exact_body
    pe = dnfile.dnPE(data=raw)
    for name, _, method in methods(pe):
        if method.Rva:
            for instruction in read_method_body_from_bytes(exact_body(pe, method)).instructions:
                if instruction.opcode.name == 'ldstr':
                    value = pe.net.user_strings.get(instruction.operand.value & 0xffffff).value
                    require('127.0.0.1:28979' not in value, 'QA origin in static Main: ' + name)


def helper_no_absolute_origin(raw, label):
    import dnfile
    from dncil.cil.body.reader import read_method_body_from_bytes
    from tools.awake_diff_rows.build import methods
    from tools.local_asset_ready.remove_gate import exact_body
    pe = dnfile.dnPE(data=raw)
    for name, _, method in methods(pe):
        if method.Rva:
            for instruction in read_method_body_from_bytes(exact_body(pe, method)).instructions:
                if instruction.opcode.name == 'ldstr':
                    value = pe.net.user_strings.get(instruction.operand.value & 0xffffff).value
                    require('://' not in value and '127.0.0.1:28979' not in value,
                            'Absolute or QA URL in ' + label + ': ' + name)


def verify_method_owners(desert_trial, stage_changes, final_changes):
    owners = {}
    for changes in [desert_trial, *stage_changes]:
        for item in changes:
            require(item['token'] not in owners, 'Overlapping stage target: ' + item['method'])
            owners[item['token']] = item
    final_tokens = [item['token'] for item in final_changes]
    require(len(final_tokens) == len(set(final_tokens)) and set(final_tokens) == set(owners),
            'Final target union differs')
    for item in final_changes:
        owner = owners[item['token']]
        require(item['method'] == owner['method'] and item['afterSHA256'] == owner['afterSHA256'],
                'A later stage changed an earlier hook: ' + item['method'])


def assert_stage_manifest(record, schema, before, after, checks):
    no_qa(record)
    require(record.get('schema') == schema and record.get('inputManagedSHA256') == digest(before)
            and record.get('outputManagedSHA256', record.get('replacementEntries', {}).get(ENTRY)) == digest(after),
            'Stage manifest or managed SHA differs: ' + schema)
    for key, value in checks.items():
        require(record.get(key) == value, 'Stage proof differs: ' + schema + ' ' + key)
    check_pins(record.get('sourcePins'))


def verify_awake_overlay(base, main_path, directory):
    """Bind the rebuilt helper to the same Main, base and audited helper ABI."""
    from tools.client_awake85_layout import build as awake
    record = read_json(directory/'manifest.json')
    compat = read_json(directory/'helper-compatibility.json')
    no_qa(record)
    require(record.get('schema') == 'hyjs-awake85-helper-overlay-v1'
            and record.get('status') == 'STATIC_VERIFIED_NOT_PACKAGED'
            and record.get('baseAPKSHA256') == BASE_SHA
            and record.get('inputManagedSHA256') == file_sha(main_path)
            and record.get('mainBytesModified') is False
            and record.get('originalEngineMethodsCompared') == 547,
            'Awake helper manifest differs')
    check_pins(record.get('sourcePins'))
    require(set(record['sourcePins']) == {
        'tools/client_dungeon_access/Runtime.cs', 'tools/client_awake85_layout/Awake85Contract.cs',
        'tools/client_awake85_layout/VerifyHelper.cs', 'tools/client_awake85_layout/build.py'},
        'Awake helper source inventory differs')
    with zipfile.ZipFile(base) as archive:
        original_main, original_helper = archive.read(ENTRY), archive.read(DUNGEON)
        original_catalog, night = archive.read(awake.SMALL), archive.read(awake.NIGHT)
    expected_catalog, added = awake.small_overlay(original_catalog)
    require(added and record.get('nightEntryAdded') is True
            and record.get('nightBundleSHA256') == awake.NIGHT_SHA
            and len(night) == awake.NIGHT_SIZE and digest(night) == awake.NIGHT_SHA
            and hashlib.md5(night).hexdigest() == awake.NIGHT_MD5
            and record.get('inputHelperSHA256') == digest(original_helper)
            and digest(original_helper) == '71d6ee0966e0d656224027a402dc6c21a70cf2bb580a352278020580b59b327b'
            and record.get('smallManifestInputSHA256') == digest(original_catalog)
            and record.get('smallManifestOutputSHA256') == digest(expected_catalog)
            and (directory/'smallassetmanifest').read_bytes() == expected_catalog,
            'Awake base resource or helper pin differs')
    helper = (directory/'HYJS.DungeonContent.dll').read_bytes()
    hooks = awake.engine_and_hooks(original_main, main_path.read_bytes())
    require(len(hooks) == 12 and hooks == record.get('existingMainHooks')
            and digest(helper) == record.get('outputHelperSHA256')
            and len(compat.get('preservedMethodSemantics', [])) == 31
            and len(compat.get('allowedAwakeMethods', [])) == 3
            and all(compat.get(k) == [] for k in ('missing', 'abiDifferences', 'semanticDifferences', 'requiredCallErrors')),
            'Awake helper ABI, existing methods or Main hooks differ')
    helper_no_absolute_origin(helper, DUNGEON)
    return {'manifestSHA256': file_sha(directory/'manifest.json'),
            'compatibilitySHA256': file_sha(directory/'helper-compatibility.json'),
            'mainSHA256': file_sha(main_path), 'helperSHA256': digest(helper),
            'nightBundleSHA256': awake.NIGHT_SHA, 'existingMainHooks': len(hooks),
            'preservedHelperMethods': 31, 'originalEngineMethodsExact': 547}


def verify_awake_contract(directory):
    record = read_json(directory/'tests.json')
    require(record.get('baseAPKSHA256') == BASE_SHA
            and record.get('contractSourceSHA256') == file_sha(ROOT/'tools/client_awake85_layout/Awake85Contract.cs')
            and record.get('testSourceSHA256') == file_sha(ROOT/'tools/client_awake85_layout/ContractTests.cs')
            and record.get('result') == 'PASS 63 actual contract checks'
            and record.get('manifestChecks') == ['insert preserves rows', 'exact row idempotent',
                 'CRLF preserved', 'conflict rejected', 'duplicate rejected']
            and (directory/'contract-tests.log').read_text().strip() == record['result'],
            'Awake contract evidence differs')
    return {'testsSHA256': file_sha(directory/'tests.json'),
            'logSHA256': file_sha(directory/'contract-tests.log'), 'checks': 63}


def verify_awake_scene(report_path, base, main_path, directory, resource_dir, map_helper):
    report = read_json(report_path)
    chapters = report.get('chapters')
    require(report.get('baseAPKSHA256') == file_sha(base) == BASE_SHA
            and report.get('mainSHA256') == file_sha(main_path)
            and report.get('helperSHA256') == file_sha(directory/'HYJS.DungeonContent.dll')
            and report.get('overlayManifestSHA256') == file_sha(directory/'manifest.json')
            and report.get('helperCompatibilitySHA256') == file_sha(directory/'helper-compatibility.json')
            and report.get('combinedResourceManifestSHA256') == file_sha(resource_dir/'resource-manifest.json')
            and report.get('catalogSHA256') == file_sha(resource_dir/'smallassetmanifest')
            and report.get('desertHelperSHA256') == file_sha(map_helper)
            and report.get('auditSourceSHA256') == file_sha(ROOT/'tools/client_awake85_layout/scene_audit.py')
            and report.get('desertLoaderOverride') is True and report.get('awakeEntryReadyAnyChapter') is True
            and isinstance(chapters, list) and {x.get('chapter') for x in chapters} == {'1','2','3','4','5'},
            'Awake scene audit inputs or chapter coverage differ')
    return {'reportSHA256': file_sha(report_path), 'mainSHA256': file_sha(main_path),
            'catalogSHA256': file_sha(resource_dir/'smallassetmanifest'), 'chapters': 5,
            'desertLoaderOverride': True}


def replay_awake_scene(base, main_path, directory, resource_dir, map_helper, report_path):
    home = ROOT/'analysis/fragment-box85/awake-copy-entry/client'
    with tempfile.TemporaryDirectory(prefix='awake85-scene-replay-', dir=home) as temporary:
        work = Path(temporary)
        report = work/'scene-audit.json'
        run('scene', [sys.executable, '-B', ROOT/'tools/client_awake85_layout/scene_audit.py',
            '--base-apk', base, '--base-sha256', BASE_SHA,
            '--input-managed', main_path, '--input-managed-sha256', file_sha(main_path),
            '--overlay-directory', directory, '--desert-resource-overlay', resource_dir,
            '--desert-helper', map_helper, '--expected-desert-helper-sha256', file_sha(map_helper),
            '--output', report], work)
        require(read_json(report) == read_json(report_path), 'Awake scene audit replay differs')


def verify_linkage_report(path, static):
    from tools.desert_extraction import verify_linkage86 as linkage
    report = read_json(path)
    proof_path = static/'build-proof.json'
    proof = read_json(proof_path)
    details = report.get('linkage', {})
    reference = Path(report.get('referenceQA83APK', ''))
    require(report.get('schema') == 'hyjs-combined86-linkage-proof-v1'
            and report.get('status') == 'LINKAGE_STATIC_NO_NEW_ERRORS_WITH_BASELINE_MISMATCHES'
            and report.get('baseAPKSHA256') == BASE_SHA
            and report.get('staticProof') == str(proof_path.relative_to(ROOT))
            and report.get('staticProofSHA256') == file_sha(proof_path)
            and report.get('payloadSHA256') == proof.get('payloadSHA256')
            and report.get('referenceQA83APKSHA256') == linkage.REFERENCE_QA83_SHA
            and reference.is_file() and file_sha(reference) == linkage.REFERENCE_QA83_SHA
            and report.get('verifierSourceSHA256') == file_sha(ROOT/'tools/desert_extraction/VerifyLinkage86.cs')
            and report.get('wrapperSourceSHA256') == file_sha(ROOT/'tools/desert_extraction/verify_linkage86.py')
            and report.get('verifierExitCode') == 0 and report.get('deviceExecuted') is False
            and details.get('roots') == [Path(name).stem for name in linkage.NAMES]
            and details.get('problems') == []
            and len(details.get('preservedOriginalFrameworkIdentityMismatches', [])) == 5
            and details.get('runtimeResolutionProven') is False,
            'Static assembly linkage proof differs')
    check_toolchain_pins(report.get('toolchainSHA256'))
    return {'reportSHA256': file_sha(path), 'staticProofSHA256': file_sha(proof_path),
            'payloadSHA256': report['payloadSHA256'], 'newProblems': 0,
            'preservedFrameworkMismatches': 5}


def replay_awake_overlay(base, main_path, directory, tool_root):
    """Rebuild from the pinned inputs for release review; leave no temporary payload."""
    home = ROOT/'analysis/fragment-box85/awake-copy-entry/client'
    home.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='awake85-replay-', dir=home) as temporary:
        work = Path(temporary)
        out = work/'overlay'
        run('rebuild', [sys.executable, '-B', ROOT/'tools/client_awake85_layout/build.py',
            '--base-apk', base, '--base-sha256', BASE_SHA,
            '--expected-helper-sha256', '71d6ee0966e0d656224027a402dc6c21a70cf2bb580a352278020580b59b327b',
            '--input-managed', main_path, '--input-managed-sha256', file_sha(main_path),
            '--tool-root', tool_root/'.tools/native-batch', '--output-directory', out], work)
        verify_awake_overlay(base, main_path, out)
        require((out/'HYJS.DungeonContent.dll').read_bytes() == (directory/'HYJS.DungeonContent.dll').read_bytes()
                and (out/'smallassetmanifest').read_bytes() == (directory/'smallassetmanifest').read_bytes()
                and read_json(out/'helper-compatibility.json') == read_json(directory/'helper-compatibility.json'),
                'Awake helper replay differs')


def verify_chain(base, map_dir, field_dir, bundle, combined83, stages):
    """Replay the 83 checks and each stage; verify final bodies against their owners."""
    from tools.desert_extraction import build_combined83 as old, build_field_patch, build_v78_adapter, package_resources
    from tools.client_trial_refresh82 import build as trial
    from tools.client_wanted84 import build as wanted
    from tools.client_breach84 import build_main as breach
    from tools.client_sweep84 import build as sweep
    from tools.client_fragment_boxes85 import build as fragment
    from tools.client_relations85 import build as relations
    from tools.client_relations85.verify_existing import verify_metadata
    from tools.client_wanted_skip87 import build as wanted87
    from tools.client_awake85_layout.build import NIGHT
    m, v = read_json(map_dir / 'manifest.json'), read_json(field_dir / 'manifest.json')
    no_qa(m, v)
    require(m.get('schema') == 'hyjs-desert53-runtime-adapter-v2'
            and m.get('status') == 'STATIC_VERIFIED_OVERLAY_NOT_PACKAGED'
            and m.get('apkSHA256') == BASE_SHA and m.get('desertEntrySelectable') is True
            and m.get('desertLoginResume') is True and m.get('walkOnly') is False
            and v.get('schema') == 'hyjs-desert-v4-scene-overlay-v2'
            and v.get('status') == 'STATIC_PREVIEW_NOT_FOR_RELEASE' and v.get('apkSHA256') == BASE_SHA,
            'Map or field manifest is not qualified')
    check_pins(m.get('sourceSHA256')); check_pins(v.get('sourceSHA256'))
    require(set(v['sourceSHA256']) == required_field_source_pins(), 'Field source pin inventory differs')
    native_map = (map_dir / 'Assembly-CSharp.dll').read_bytes()
    field_main = (field_dir / 'Assembly-CSharp.dll').read_bytes()
    map_helper = (map_dir / 'SoulFlowerDesert53.dll').read_bytes()
    field_helper = (field_dir / 'SoulFlowerDesertV4.dll').read_bytes()
    require(digest(native_map) == m['outputManagedSHA256'] == v['inputManagedSHA256']
            and digest(field_main) == v['outputManagedSHA256']
            and digest(map_helper) == m['helperSHA256'] and digest(field_helper) == v['helperSHA256'],
            'Map or field payload does not match its manifest')
    icon_sha = verify_embedded_evacuation_icon(field_helper)
    helper_no_absolute_origin(map_helper, MAP)
    helper_no_absolute_origin(field_helper, FIELD)
    require(file_sha(base) == BASE_SHA, 'Wrong v80 base APK')
    with zipfile.ZipFile(base) as z:
        require(len(z.namelist()) == len(set(z.namelist())), 'Duplicate base APK entries')
        original, original_shop, dex = z.read(ENTRY), z.read(SHOP), z.read('classes2.dex')
        catalog, night = z.read(package_resources.CATALOG_ENTRY), z.read(NIGHT)
        origin = json.loads(z.read('assets/local-wallet.json'))['origin'].rstrip('/')
        inherited = {n: digest(z.read(n)) for n in z.namelist()
                     if n.startswith('lib/') or (n.startswith('classes') and n.endswith('.dex'))}
    require(origin and origin != QA_ORIGIN, 'Unexpected production origin')
    map_catalog = package_resources.compose_catalog(catalog, bundle.read_bytes())
    expected_catalog = package_resources.compose_awake_catalog(catalog, bundle.read_bytes(), night)
    require(digest(original) == m['inputManagedSHA256'] and digest(catalog) == m['originalCatalogSHA256']
            and digest(map_catalog) == m['updatedCatalogSHA256']
            and map_catalog == (map_dir / 'smallassetmanifest').read_bytes(), 'Map resources differ')
    map_check = build_v78_adapter.verify(original, native_map, bundle.stat().st_size, False, True, True)
    field_check = build_field_patch.verify(native_map, field_main)
    proof83 = read_json(combined83 / 'build-proof.json')
    no_qa(proof83); check_pins(proof83['sourcePins'])
    for source in (m['sourceSHA256'], v['sourceSHA256']):
        for name, value in source.items():
            require(proof83['sourcePins'].get(name) == value, '83 dropped map/field source pin: ' + name)
    for path in (*trial.SOURCES, ROOT/'tools/client_trial_refresh82/build.py',
                 ROOT/'tools/client_trial_refresh82/Injector.cs', ROOT/'tools/client_trial_refresh82/Verify.cs'):
        source = local(path)
        name = str(source.relative_to(ROOT))
        require(proof83['sourcePins'].get(name) == file_sha(source), '83 dropped trial source pin: ' + name)
    require(proof83.get('status') == 'STATIC_PREVIEW_NOT_FOR_RELEASE'
            and proof83.get('baseAPKSHA256') == BASE_SHA
            and proof83.get('desertInputManagedSHA256') == digest(field_main), '83 proof differs')
    main83 = (combined83 / 'Assembly-CSharp.dll').read_bytes()
    shop = (combined83 / 'SoulFlowerGGOShop.dll').read_bytes()
    require(digest(main83) == proof83['combinedManagedSHA256'] and digest(shop) == proof83['shopHelperSHA256'],
            '83 static payload differs')
    require(trial.check_main(field_main, main83) == proof83['trialHookCheck']
            and old.composite_check(original, main83, map_check, field_check) == proof83['allChangedMethods']
            and old.audit_shop(shop) == proof83['shopOriginAudit'], '83 hook or shop proof differs')
    import dnfile
    from tools.awake_diff_rows.build import methods
    def icon_signature(raw):
        found = [method for name, _, method in methods(dnfile.dnPE(data=raw))
                 if name == 'HYJS.GGO.GGOShop::RegisterIcons']
        require(len(found) == 1, 'Icon registration method differs')
        method = found[0]
        return method.Signature.value, method.Flags.mdPrivate, method.Flags.mdStatic
    require(icon_signature(original_shop) == icon_signature(shop), 'Private icon registration ABI differs')
    changes83 = managed_diff(original, main83)
    require({x['method'] for x in changes83} == set(proof83['allChangedMethods']), '83 method inventory differs')
    results, stage_changes = [], []
    current = main83
    for name, directory in stages:
        record = read_json(directory / 'manifest.json')
        after = (directory / 'Assembly-CSharp.dll').read_bytes()
        if name == 'wanted84':
            checks = wanted.proof(current, after)
            schema = 'hyjs-wanted84-recovery-overlay-v1'
            expected = {f'0x{0x06000000 | rid:08x}' for rid in wanted.PINS}
        elif name == 'breach84':
            patched, checks = breach.patch(current)
            require(patched == after, 'Breach managed output differs')
            schema = breach.SCHEMA
            expected = {checks['methodToken']}
        elif name == 'sweep84':
            checks = sweep.verify(current, after)
            schema = 'hyjs-sweep84-overlay-v1'
            expected = {f"0x{int(x['token'], 16):08x}" for x in checks['allowedChangedMethods']}
        elif name == 'fragment85':
            checks = fragment.verify(current, after)
            schema = 'hyjs-fragment-choice-85-v1'
            expected = {f"0x{int(x['token'], 16):08x}" for x in checks['allowedChangedMethods']}
        elif name == 'relations85':
            checks = relations.verify(current, after)
            schema = 'hyjs-relations85-v1'
            expected = {f"0x{int(x['token'], 16):08x}" for x in checks['allowedChangedMethods']}
            require(set(record.get('sourcePins', {})) == {
                'tools/client_relations85/build.py', 'tools/client_relations85/Injector.cs',
                'tools/client_relations85/target-pins.json'}, 'Relations 85 source inventory differs')
        elif name == 'wanted87':
            checks = wanted87.verify(current, after)
            schema = wanted87.SCHEMA
            expected = {f"0x{int(x['token'], 16):08x}" for x in checks['changedMethods']}
            require(record.get('status') == 'MODEL_VERIFIED_NOT_DEVICE_TESTED'
                    and digest((directory/'HYJS.WantedSkip87.dll').read_bytes()) == record.get('helperSHA256')
                    and set(record.get('sourcePins', {})) == {
                        'tools/client_wanted_skip87/Runtime.cs', 'tools/client_wanted_skip87/Injector.cs',
                        'tools/client_wanted_skip87/ModelTests.cs', 'tools/client_wanted_skip87/build.py',
                        'tools/client_wanted_skip87/target-pins.json'}
                    and 'PASS 28 Wanted skip boundary checks' in (directory/'run-model.log').read_text(),
                    'Wanted 87 model evidence differs')
        else:
            raise ValueError('Unknown stage: ' + name)
        assert_stage_manifest(record, schema, current, after, checks)
        changed = managed_diff(current, after)
        require({x['token'] for x in changed} == expected, 'Unexpected stage targets: ' + name)
        stage_changes.append(changed)
        result = {'stage': name, 'inputSHA256': digest(current), 'outputSHA256': digest(after),
                  'changes': changed, 'checks': checks}
        if name == 'relations85':
            model = read_json(directory/'model-report.json')
            require(model.get('status') == 'PASS' and model.get('baselineManagedSHA256') == digest(current)
                    and model.get('managedSHA256') == digest(after)
                    and model.get('testSourceSHA256') == file_sha(ROOT/'tools/client_relations85/ModelTests.cs')
                    and 'PASS 37 checks' in model.get('output', '')
                    and model.get('deviceExecuted') is False and model.get('unityExecuted') is False,
                    'Relations 85 model evidence differs')
            result['metadataChecks'] = verify_metadata(current, after)
            result['modelSHA256'] = file_sha(directory/'model-report.json')
        results.append(result)
        current = after
    final_changes = managed_diff(original, current)
    verify_method_owners(changes83, stage_changes, final_changes)
    no_qa_origin_in_main(current)
    return {'main': current, 'map': map_helper, 'field': field_helper, 'shop': shop,
            'original': original, 'originalShop': original_shop, 'dex': dex, 'catalog': catalog,
            'expectedCatalog': expected_catalog, 'origin': origin, 'inherited': inherited,
            'mapCheck': map_check, 'fieldCheck': field_check, 'proof83': proof83,
            'changes83': changes83, 'stages': results, 'finalChanges': final_changes,
            'evacuationIconSHA256': icon_sha}


def run(label, command, logs):
    result = subprocess.run(['rtk', 'proxy', *map(str, command)], cwd=ROOT,
                            capture_output=True, text=True, timeout=900)
    (logs / (label + '.log')).write_text(result.stdout + result.stderr, encoding='utf-8')
    require(result.returncode == 0, label + ' failed; inspect retained log')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ('base-apk', 'map-overlay', 'preview', 'bundle', 'runtime', 'tool-root', 'breach-resource-overlay', 'output'):
        p.add_argument('--' + name, required=True, type=Path)
    p.add_argument('--reference-qa-apk', required=True, type=Path,
                   help='Pinned historical 83 QA APK, used only for read-only component comparison')
    p.add_argument('--field-main-sha256', required=True)
    p.add_argument('--field-helper-sha256', required=True)
    a = p.parse_args()
    base, map_dir, field_dir, bundle, runtime, tool_root, breach_dir, out = (
        x.resolve() for x in (a.base_apk, a.map_overlay, a.preview, a.bundle, a.runtime, a.tool_root,
                              a.breach_resource_overlay, a.output))
    home = ROOT / 'analysis/desert-extraction-v4-client'
    require(out.parent == home and re.fullmatch(r'combined86-[a-zA-Z0-9][a-zA-Z0-9_-]{0,47}', out.name),
            'Use analysis/desert-extraction-v4-client/combined86-<unique-name>')
    tag = out.name
    old_out = home / ('combined83-' + tag)
    wanted_out = ROOT / 'tools/client_wanted84' / tag
    breach_main_out = ROOT / 'build' / (tag + '-breach-main')
    sweep_out = ROOT / 'tools/client_sweep84' / tag
    relations_out = ROOT / 'analysis/fragment-box85/relations' / tag
    awake_out = ROOT / 'analysis/fragment-box85/awake-copy-entry/client' / tag
    awake_contract = ROOT / 'analysis/fragment-box85/awake-copy-entry/client' / (tag + '-contracts')
    for target in (out, old_out, wanted_out, breach_main_out, sweep_out, relations_out, awake_out, awake_contract):
        require(not target.exists(), 'Fresh owned output required: ' + str(target))
    local(base); local(bundle); local(map_dir/'manifest.json'); local(field_dir/'manifest.json')
    require(a.reference_qa_apk.is_file(), 'Historical component comparison APK missing')
    validate_field_manifest(field_dir, a.field_main_sha256, a.field_helper_sha256)
    tool_pins = toolchain_pins(runtime, tool_root/'.tools/native-batch')
    # Run field.verify before invoking the legacy 83 builder with the explicit pins.
    from tools.desert_extraction import build_combined83 as old, build_field_patch, package_resources
    field_check = build_field_patch.verify((map_dir/'Assembly-CSharp.dll').read_bytes(),
                                           (field_dir/'Assembly-CSharp.dll').read_bytes())
    require(field_check.get('originalEngineMethodsUnchanged') == 547 and field_check.get('qualityFilterV80Exact'),
            'Field verification differs')
    previous_pins = old.R8J_MAIN, old.R8J_HELPER
    old.R8J_MAIN, old.R8J_HELPER = a.field_main_sha256, a.field_helper_sha256
    old_argv = sys.argv
    try:
        sys.argv = [str(Path(old.__file__)), '--base-apk', str(base), '--map-overlay', str(map_dir),
                    '--preview', str(field_dir), '--bundle', str(bundle), '--runtime', str(runtime), '--output', str(old_out)]
        old.main()
    finally:
        old.R8J_MAIN, old.R8J_HELPER = previous_pins
        sys.argv = old_argv
    out.mkdir(parents=True)
    stages = [('wanted84', wanted_out), ('breach84', breach_main_out),
              ('sweep84', sweep_out), ('fragment85', out/'fragment85'),
              ('relations85', relations_out), ('wanted87', out/'wanted87')]
    current = old_out/'static/Assembly-CSharp.dll'
    specs = [
        ('wanted84', ROOT/'tools/client_wanted84/build.py', ['--base-apk', base, '--tool-root', tool_root]),
        ('breach84', ROOT/'tools/client_breach84/build_main.py', []),
        ('sweep84', ROOT/'tools/client_sweep84/build.py', ['--base-apk', base, '--base-sha256', BASE_SHA, '--tool-root', tool_root]),
        ('fragment85', ROOT/'tools/client_fragment_boxes85/build.py', ['--base-apk', base, '--tool-root', tool_root]),
        ('relations85', ROOT/'tools/client_relations85/build.py', ['--tool-root', tool_root]),
        ('wanted87', ROOT/'tools/client_wanted_skip87/build.py', ['--base-apk', base, '--tool-root', tool_root]),
    ]
    for (name, directory), (expected, script, extra) in zip(stages, specs):
        require(name == expected, 'Stage order differs')
        run(name, [sys.executable, '-B', script, *extra, '--input-managed', current,
                   '--input-managed-sha256', file_sha(current), '--output-directory', directory], out)
        if name == 'relations85':
            run('relations85-model', [sys.executable, '-B', ROOT/'tools/client_relations85/run_tests.py',
                '--baseline-managed', current, '--baseline-sha256', file_sha(current),
                '--managed', directory/'Assembly-CSharp.dll', '--managed-sha256', file_sha(directory/'Assembly-CSharp.dll'),
                '--report', directory/'model-report.json', '--tool-root', tool_root], out)
        current = directory/'Assembly-CSharp.dll'
    run('awake85-helper', [sys.executable, '-B', ROOT/'tools/client_awake85_layout/build.py',
        '--base-apk', base, '--base-sha256', BASE_SHA,
        '--expected-helper-sha256', '71d6ee0966e0d656224027a402dc6c21a70cf2bb580a352278020580b59b327b',
        '--input-managed', current, '--input-managed-sha256', file_sha(current),
        '--tool-root', tool_root/'.tools/native-batch', '--output-directory', awake_out], out)
    awake_proof = verify_awake_overlay(base, current, awake_out)
    run('awake85-contract', [sys.executable, '-B', ROOT/'tools/client_awake85_layout/test.py',
        '--base-apk', base, '--base-sha256', BASE_SHA, '--tool-root', tool_root/'.tools/native-batch',
        '--output-directory', awake_contract], out)
    contract_proof = verify_awake_contract(awake_contract)
    state = verify_chain(base, map_dir, field_dir, bundle, old_out/'static', stages)
    require(state['fieldCheck'] == field_check and digest(state['main']) == file_sha(current), 'Final field or Main differs')
    feedback = (wanted_out/'HYJS.Feedback65.dll').read_bytes()
    fragment = (out/'fragment85/HYJS.FragmentBoxes85.dll').read_bytes()
    wanted87_helper = (out/'wanted87/HYJS.WantedSkip87.dll').read_bytes()
    dungeon_helper = (awake_out/'HYJS.DungeonContent.dll').read_bytes()
    w, f = read_json(wanted_out/'manifest.json'), read_json(out/'fragment85/manifest.json')
    w87 = read_json(out/'wanted87/manifest.json')
    require(digest(feedback) == w['replacementEntries'][FEEDBACK]
            and w.get('existingHelperPublicABIExact') is True
            and digest(fragment) == f['helperSHA256'] == f['replacementEntries'][FRAGMENT]
            and digest(wanted87_helper) == w87['helperSHA256']
            and digest(dungeon_helper) == awake_proof['helperSHA256'], 'Stage helper differs')
    helper_no_absolute_origin(feedback, FEEDBACK)
    helper_no_absolute_origin(fragment, FRAGMENT)
    helper_no_absolute_origin(wanted87_helper, WANTED87)
    helper_no_absolute_origin(dungeon_helper, DUNGEON)
    pins = dict(state['proof83']['sourcePins'])
    for record in (w, read_json(breach_main_out/'manifest.json'), read_json(sweep_out/'manifest.json'), f,
                   read_json(relations_out/'manifest.json'), w87, read_json(awake_out/'manifest.json')):
        for name, value in record['sourcePins'].items():
            add_pin(pins, ROOT/name, value)
    for file in [Path(__file__), base, bundle, map_dir/'manifest.json', field_dir/'manifest.json',
                 map_dir/'Assembly-CSharp.dll', map_dir/'SoulFlowerDesert53.dll', map_dir/'smallassetmanifest',
                 field_dir/'Assembly-CSharp.dll', field_dir/'SoulFlowerDesertV4.dll',
                 old_out/'static/build-proof.json', old_out/'static/Assembly-CSharp.dll', old_out/'static/SoulFlowerGGOShop.dll',
                 breach_dir/'manifest.json', breach_dir/'tabletext',
                 *[directory/'manifest.json' for _, directory in stages],
                 *[directory/'Assembly-CSharp.dll' for _, directory in stages],
                 wanted_out/'HYJS.Feedback65.dll', out/'fragment85/HYJS.FragmentBoxes85.dll',
                 relations_out/'model-report.json', out/'wanted87/run-model.log', out/'wanted87/HYJS.WantedSkip87.dll',
                 awake_out/'manifest.json', awake_out/'helper-compatibility.json', awake_out/'smallassetmanifest',
                 awake_out/'HYJS.DungeonContent.dll',
                 awake_contract/'tests.json', awake_contract/'contract-tests.log',
                 ROOT/'tools/client_relations85/run_tests.py', ROOT/'tools/client_relations85/ModelTests.cs',
                 ROOT/'tools/client_relations85/verify_existing.py', ROOT/'tools/client_awake85_layout/test.py',
                 ROOT/'tools/client_awake85_layout/ContractTests.cs', ROOT/'tools/client_awake85_layout/scene_audit.py',
                 ROOT/'tools/client_dungeon_access/targets.json', ROOT/'tools/client_arena_state/build.py',
                 ROOT/'tools/awake_diff_rows/build.py', ROOT/'tools/client_activity/build.py',
                 ROOT/'tools/client_achievements/build.py', ROOT/'tools/local_asset_ready/remove_gate.py',
                 ROOT/'analysis/tables/TableAwakeCopyChapter.tsv',
                 ROOT/'tools/desert_extraction/verify_linkage86.py', ROOT/'tools/desert_extraction/VerifyLinkage86.cs',
                 ROOT/'tools/desert_extraction/package_resources.py',
                 ROOT/'tools/client_one_key59/build.py', ROOT/'tools/client_feedback65/build.py',
                 ROOT/'tools/desert_extraction/prepare_release_package.py',
                 ROOT/'tools/desert_extraction/prepare_release_package86.py',
                 ROOT/'tools/client_release/package_native.py']:
        add_pin(pins, file)
    from tools.client_breach84.build import apply_overlay as apply_breach
    resource_payload = {package_resources.CATALOG_ENTRY: state['expectedCatalog']}
    breach_pins = {}
    breach_proof = apply_breach(base, breach_dir, resource_payload, breach_pins)
    require(digest(resource_payload['assets/android/tabledata/tabletext']) == file_sha(breach_dir/'tabletext'),
            'Breach resource differs from pinned preparation')
    for path, value in breach_pins.items():
        add_pin(pins, path, value)
    # Audit the final catalog and desert loader without signing or constructing an APK.
    from tools.client_awake85_layout.build import NIGHT_SHA
    scene_resources = out/'awake-scene-resources'; scene_resources.mkdir()
    (scene_resources/'smallassetmanifest').write_bytes(state['expectedCatalog'])
    (scene_resources/'map_desertfield_001').write_bytes(bundle.read_bytes())
    scene_files = {package_resources.CATALOG_ENTRY: 'smallassetmanifest',
                   package_resources.BUNDLE_ENTRY: 'map_desertfield_001'}
    write_json(scene_resources/'resource-manifest.json', {
        'schema': package_resources.AWAKE_SCHEMA, 'baseAPKSHA256': BASE_SHA,
        'bundleSHA256': package_resources.BUNDLE_SHA, 'bundleBytes': bundle.stat().st_size,
        'nightBundleSHA256': NIGHT_SHA, 'originalCatalogSHA256': digest(state['catalog']),
        'updatedCatalogSHA256': digest(state['expectedCatalog']),
        'payloadFiles': {entry: str((scene_resources/name).relative_to(ROOT)) for entry, name in scene_files.items()},
        'replacementEntries': {entry: file_sha(scene_resources/name) for entry, name in scene_files.items()},
        'sourcePins': dict(pins), 'releaseReady': False})
    scene_report = awake_out/'combined-scene-audit.json'
    run('awake85-scenes', [sys.executable, '-B', ROOT/'tools/client_awake85_layout/scene_audit.py',
        '--base-apk', base, '--base-sha256', BASE_SHA,
        '--input-managed', current, '--input-managed-sha256', file_sha(current),
        '--overlay-directory', awake_out, '--desert-resource-overlay', scene_resources,
        '--desert-helper', map_dir/'SoulFlowerDesert53.dll',
        '--expected-desert-helper-sha256', file_sha(map_dir/'SoulFlowerDesert53.dll'),
        '--output', scene_report], out)
    scene_proof = verify_awake_scene(scene_report, base, current, awake_out, scene_resources,
                                     map_dir/'SoulFlowerDesert53.dll')
    for path in (scene_report, scene_resources/'resource-manifest.json',
                 scene_resources/'smallassetmanifest', scene_resources/'map_desertfield_001'):
        add_pin(pins, path)
    static = out/'static'; static.mkdir()
    files = {ENTRY: state['main'], MAP: state['map'], FIELD: state['field'], SHOP: state['shop'],
             FEEDBACK: feedback, FRAGMENT: fragment, DUNGEON: dungeon_helper, WANTED87: wanted87_helper}
    for entry, raw in files.items():
        target = static/Path(entry).name; target.write_bytes(raw); add_pin(pins, target)
    proof = {'schema': 'hyjs-desert-trial-feedback-fragment86-static-proof-v1',
             'status': 'STATIC_PREVIEW_NOT_FOR_RELEASE', 'baseAPKSHA256': BASE_SHA,
             'fieldMainSHA256': a.field_main_sha256, 'fieldHelperSHA256': a.field_helper_sha256,
             'combined83Static': str((old_out/'static').relative_to(ROOT)),
             'mapOverlay': str(map_dir.relative_to(ROOT)), 'preview': str(field_dir.relative_to(ROOT)),
             'bundle': str(bundle.relative_to(ROOT)), 'breachResourceOverlay': str(breach_dir.relative_to(ROOT)),
             'awakeOverlay': str(awake_out.relative_to(ROOT)), 'awakeToolRoot': str(tool_root), 'awakeProof': awake_proof,
             'awakeContract': str(awake_contract.relative_to(ROOT)), 'awakeContractProof': contract_proof,
             'awakeSceneResources': str(scene_resources.relative_to(ROOT)), 'awakeSceneProof': scene_proof,
             'stages': [{'name': name, 'directory': str(directory.relative_to(ROOT))} for name, directory in stages],
             'stageProofs': state['stages'], 'desertTrialChanges': state['changes83'],
             'finalChanges': state['finalChanges'], 'originalEngineMethodsExact': 547, 'qualityFilterV80Exact': True,
             'evacuationIconSHA256': state['evacuationIconSHA256'],
             'payloadSHA256': {entry: digest(raw) for entry, raw in files.items()},
             'breach84ResourceProof': breach_proof, 'breach84OverlayManifestSHA256': file_sha(breach_dir/'manifest.json'),
             'breach84ResourceSHA256': file_sha(breach_dir/'tabletext'), 'sourcePins': pins,
             'toolchainSHA256': tool_pins,
             'deviceExecuted': False, 'serverExecuted': False}
    write_json(static/'build-proof.json', proof); add_pin(pins, static/'build-proof.json')
    linkage_report = out/'linkage.json'
    run('linkage', [sys.executable, '-B', ROOT/'tools/desert_extraction/verify_linkage86.py',
        '--base-apk', base, '--overlay', static, '--runtime', runtime,
        '--reference-qa-apk', a.reference_qa_apk.resolve(), '--output', linkage_report], out)
    linkage_proof = verify_linkage_report(linkage_report, static)
    add_pin(pins, linkage_report)
    # The QA candidate is explicitly isolated. Formal preparation reads static/, never this directory.
    candidate = out/'qa-production'; candidate.mkdir()
    for entry, raw in files.items():
        (candidate/Path(entry).name).write_bytes(raw)
    (candidate/'smallassetmanifest').write_bytes(state['expectedCatalog'])
    (candidate/'map_desertfield_001').write_bytes(bundle.read_bytes())
    payload = {entry: str((candidate/Path(entry).name).relative_to(ROOT)) for entry in files}
    qa = {'schema': SCHEMA_QA, 'status': 'STATIC_VERIFIED_QA_NOT_RELEASED', 'qaOnly': True, 'neverPromote': True,
          'managedOnly': True, 'baseAPKSHA256': BASE_SHA, 'inputManagedSHA256': digest(state['original']),
          'outputManagedSHA256': digest(state['main']), 'inputDexSHA256': digest(state['dex']),
          'inheritedJavaByteExact': True, 'inheritedJavaPublicProtectedABIExact': True,
          'inheritedJavaEntrySHA256': state['inherited'], 'payloadFiles': payload,
          'replacementEntries': {entry: digest(raw) for entry, raw in files.items()},
          'allowedChangedMethods': sorted(x['method'] for x in state['finalChanges']), 'sourcePins': pins,
          'combinedStaticProof': str((static/'build-proof.json').relative_to(ROOT)),
          'linkageProof': str(linkage_report.relative_to(ROOT)), 'linkageProofSummary': linkage_proof,
          'originalEngineMethodsExact': 547, 'qualityFilterV80Exact': True,
          'breach84OverlayManifestSHA256': proof['breach84OverlayManifestSHA256'],
          'breach84ResourceSHA256': proof['breach84ResourceSHA256'], 'deviceExecuted': False, 'serverExecuted': False}
    write_json(candidate/'manifest.json', qa)
    resources = {package_resources.CATALOG_ENTRY: 'smallassetmanifest', package_resources.BUNDLE_ENTRY: 'map_desertfield_001'}
    write_json(candidate/'resource-manifest.json', {'schema': package_resources.AWAKE_SCHEMA,
        'baseAPKSHA256': BASE_SHA, 'bundleSHA256': package_resources.BUNDLE_SHA, 'bundleBytes': bundle.stat().st_size,
        'nightBundleSHA256': NIGHT_SHA,
        'originalCatalogSHA256': digest(state['catalog']), 'updatedCatalogSHA256': digest(state['expectedCatalog']),
        'payloadFiles': {entry: str((candidate/name).relative_to(ROOT)) for entry, name in resources.items()},
        'replacementEntries': {entry: file_sha(candidate/name) for entry, name in resources.items()},
        'sourcePins': pins, 'releaseReady': False})
    qa_dir = out/'qa-loopback'
    run('prepare-loopback-qa', [sys.executable, '-B', ROOT/'tools/desert_extraction/prepare_qa_package.py',
        '--base-apk', base, '--source', candidate, '--runtime', runtime, '--output', qa_dir], out)
    qa_manifest = read_json(qa_dir/'manifest.json')
    require(qa_manifest.get('qaOnly') is True and qa_manifest.get('neverPromote') is True
            and qa_manifest.get('qaOrigin') == QA_ORIGIN, 'QA isolation markers differ')
    require((qa_dir/Path(SHOP).name).read_bytes() == state['shop'], 'QA shop payload differs')
    qa_manifest['qaOriginProof']['helperNoAbsoluteOrigin']['shop'] = digest(state['shop'])
    qa_manifest['qaOriginProof']['shopActualPayloadAudit'] = old.audit_shop(state['shop'])
    write_json(qa_dir/'manifest.json', qa_manifest)
    qa_resources = read_json(qa_dir/'resource-manifest.json')
    qa_resources.update(releaseReady=False, qaOnly=True, neverPromote=True, qaOrigin=QA_ORIGIN)
    write_json(qa_dir/'resource-manifest.json', qa_resources)
    check_pins(pins)
    check_toolchain_pins(tool_pins)
    print(json.dumps({'status': 'STATIC_ONLY_QA_NOT_SIGNED', 'static': str(static), 'qa': str(qa_dir),
                      'version': VERSION, 'buildCode': BUILD_CODE, 'originalEngineMethodsExact': 547}))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, KeyError, json.JSONDecodeError) as error:
        print('Combined 86 blocked: ' + str(error), file=sys.stderr)
        sys.exit(1)
