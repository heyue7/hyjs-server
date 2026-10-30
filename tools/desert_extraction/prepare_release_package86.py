"""Prepare an independent combined-86 formal overlay with recorded acceptance status.

This script does not sign, install, publish, or promote a QA payload.
"""
import argparse
import json
from pathlib import Path
import re
import shutil
import sys
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.desert_extraction import build_combined86 as combined
from tools.desert_extraction import prepare_release_package as review83

NEW_CASES = ('map_zoom_and_drag', 'map_evacuation_points', 'wanted84_recovery',
             'breach84_text_and_probability', 'sweep84', 'fragment85_quantity',
             'plunder84', 'boss_lines85', 'team_map_lifecycle', 'training85_gradient',
             'relations85_sparse_formation', 'awake85_chapters_and_night_cache',
             'awake85_first_75_crash', 'wanted87_skip_settlement_and_recovery')


def reviewed_cases(path, qa_sha):
    review, pins = review83.review_evidence(path, qa_sha)
    cases = review['cases']
    for name in NEW_CASES:
        refs = cases.get(name)
        combined.require(isinstance(refs, list) and bool(refs), 'Missing accepted evidence: ' + name)
        for ref in refs:
            combined.require(isinstance(ref, dict) and set(ref) == {'path', 'sha256'}
                             and re.fullmatch(r'[0-9a-f]{64}', ref.get('sha256', '')),
                             'Invalid evidence reference: ' + name)
            evidence = review83.local_file(ref['path'])
            combined.require(evidence.stat().st_size > 0, 'Evidence file is empty: ' + name)
            review83.add_pin(pins, evidence, ref['sha256'])
    return review, pins


def user_waiver(path, qa_sha):
    """Record the user's decision without claiming review or device acceptance."""
    path = combined.local(path)
    waiver = combined.read_json(path)
    combined.require(waiver.get('schema') == 'hyjs-desert86-user-acceptance-waiver-v1'
                     and waiver.get('qaAPKSHA256') == qa_sha
                     and waiver.get('userRequestedReleaseWithoutFurtherReview') is True
                     and waiver.get('userWillTest') is True
                     and waiver.get('reviewCompleted') is False
                     and waiver.get('deviceAcceptanceCompleted') is False
                     and isinstance(waiver.get('userInstruction'), str)
                     and len(waiver['userInstruction'].strip()) >= 10,
                     'Invalid user acceptance waiver')
    pins = {}
    combined.add_pin(pins, path)
    return waiver, pins


def verify_origin(static_main, qa_main, production_origin):
    from tools.client_remote import build as remote
    targets = {name: (old.replace('http://127.0.0.1:8088', production_origin), new)
               for name, (old, new) in remote.targets_for(combined.QA_ORIGIN).items()}
    original_targets = remote.targets_for
    try:
        remote.targets_for = lambda _origin: targets
        result = remote.verify(static_main, qa_main, combined.QA_ORIGIN)
    finally:
        remote.targets_for = original_targets
    combined.require(result['onlyFourHTTPOriginLiteralsChanged']
                     and result['originalEngineMethodsUnchanged'] == 547, 'QA origin boundary differs')
    return result


def prepare(args):
    from tools.desert_extraction import build_combined83, package_resources
    from tools.client_breach84.build import apply_overlay as apply_breach

    out, static = args.output.resolve(), args.combined_static.resolve()
    home = ROOT/'analysis/desert-extraction-v4-client'
    combined.require(out.parent == home and not out.exists(), 'Use a fresh release output in the analysis directory')
    combined.require(static.parent.parent == home and static.parent.name.startswith('combined86-')
                     and static.name == 'static' and not (static/'manifest.json').exists(),
                     'Independent combined 86 static input required')
    base = combined.local(args.base_apk)
    qa_path, qa_manifest_path, qa_record_path = map(combined.local, (args.qa_apk, args.qa_manifest, args.qa_record))
    combined.require(qa_manifest_path == static.parent/'qa-loopback/manifest.json', 'Wrong combined QA manifest')
    qa_resource_path = combined.local(qa_manifest_path.parent/'resource-manifest.json')
    qa_sha = combined.file_sha(qa_path)
    acceptance_path = args.acceptance or args.user_acceptance_waiver
    if args.user_acceptance_waiver:
        review, evidence_pins = user_waiver(args.user_acceptance_waiver, qa_sha)
        acceptance_status = 'USER_WAIVED_FURTHER_REVIEW_AND_DEVICE_ACCEPTANCE'
    else:
        review, evidence_pins = reviewed_cases(args.acceptance, qa_sha)
        acceptance_status = 'REVIEWED'
    proof_path = combined.local(static/'build-proof.json')
    proof = combined.read_json(proof_path)
    combined.no_qa(proof)
    combined.require(proof.get('schema') == 'hyjs-desert-trial-feedback-fragment86-static-proof-v1'
                     and proof.get('status') == 'STATIC_PREVIEW_NOT_FOR_RELEASE'
                     and proof.get('baseAPKSHA256') == combined.BASE_SHA, 'Wrong 86 static proof')
    combined.check_pins(proof['sourcePins'])
    combined.check_toolchain_pins(proof.get('toolchainSHA256'))
    qa_manifest, qa_record = combined.read_json(qa_manifest_path), combined.read_json(qa_record_path)
    qa_resources = combined.read_json(qa_resource_path)
    combined.require(qa_resources.get('schema') == package_resources.AWAKE_SCHEMA
                     and qa_resources.get('baseAPKSHA256') == combined.BASE_SHA
                     and qa_resources.get('releaseReady') is False and qa_resources.get('qaOnly') is True
                     and qa_resources.get('neverPromote') is True and qa_resources.get('qaOrigin') == combined.QA_ORIGIN
                     and not qa_resources.get('diagnosticOnly'), 'QA resource isolation differs')
    combined.require(qa_manifest.get('schema') == combined.SCHEMA_QA and qa_manifest.get('qaOnly') is True
                     and qa_manifest.get('neverPromote') is True and qa_manifest.get('qaOrigin') == combined.QA_ORIGIN
                     and qa_manifest.get('baseAPKSHA256') == combined.BASE_SHA
                     and qa_manifest.get('combinedStaticProof') == str(proof_path.relative_to(ROOT)),
                     'Expected isolated 86 QA manifest')
    combined.require(qa_record.get('qaOnly') is True and qa_record.get('neverPromote') is True
                     and not qa_record.get('diagnosticOnly') and qa_record.get('binarySHA256') == qa_sha
                     and qa_record.get('baseAPKSHA256') == combined.BASE_SHA
                     and qa_record.get('signatureVerified') is True and qa_record.get('zipalignVerified') is True
                     and qa_record.get('allOtherEntriesAndCompressedPayloadsExact') is True
                     and (qa_record.get('buildCode'), qa_record.get('versionName')) == (combined.BUILD_CODE, combined.VERSION),
                     'Wrong signed 86 QA record')
    qa_sources = qa_manifest.get('sourcePins')
    combined.check_pins(qa_sources)
    pins = dict(proof['sourcePins'])
    for name, value in pins.items():
        combined.require(qa_sources.get(name) == value, 'QA did not test the current source: ' + name)
    combined.add_pin(pins, proof_path)
    combined.require(qa_sources.get(str(proof_path.relative_to(ROOT))) == combined.file_sha(proof_path),
                     'QA did not pin the independent static proof')
    linkage_path = combined.local(static.parent/'linkage.json')
    linkage_name = str(linkage_path.relative_to(ROOT))
    combined.require(qa_manifest.get('linkageProof') == linkage_name
                     and qa_sources.get(linkage_name) == combined.file_sha(linkage_path)
                     and combined.verify_linkage_report(linkage_path, static) == qa_manifest.get('linkageProofSummary'),
                     'QA linkage proof differs')
    for name, value in qa_sources.items():
        combined.add_pin(pins, ROOT/name, value)
    for path in (qa_path, qa_manifest_path, qa_resource_path, qa_record_path, Path(__file__), ROOT/'tools/client_release/package_native.py',
                 ROOT/'tools/desert_extraction/package_resources.py', ROOT/'tools/client_breach84/build.py'):
        combined.add_pin(pins, path)
    for name, value in evidence_pins.items():
        combined.add_pin(pins, ROOT/name, value)
    map_dir, field_dir = (ROOT/proof['mapOverlay']).resolve(), (ROOT/proof['preview']).resolve()
    bundle = combined.local(ROOT/proof['bundle'])
    combined83 = (ROOT/proof['combined83Static']).resolve()
    stages = [(row['name'], (ROOT/row['directory']).resolve()) for row in proof['stages']]
    combined.require([name for name, _ in stages] == ['wanted84', 'breach84', 'sweep84', 'fragment85',
                                                       'relations85', 'wanted87'],
                     'Stage order differs')
    awake_dir = (ROOT/proof['awakeOverlay']).resolve()
    awake_contract = (ROOT/proof['awakeContract']).resolve()
    scene_resources = (ROOT/proof['awakeSceneResources']).resolve()
    tool_root = Path(proof['awakeToolRoot']).resolve()
    native = tool_root/'.tools/native-batch'
    tool_files = [native/'dotnet/dotnet', native/'injector/bin/Debug/net8.0/dnlib.dll',
                  next((native/'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))]
    for path in tool_files:
        combined.require(proof['toolchainSHA256'].get(str(path)) == combined.file_sha(path),
                         'Awake toolchain not bound to the static proof')
    expected_inputs = [base, bundle, map_dir/'manifest.json', map_dir/'Assembly-CSharp.dll',
                       map_dir/'SoulFlowerDesert53.dll', map_dir/'smallassetmanifest', field_dir/'manifest.json',
                       field_dir/'Assembly-CSharp.dll', field_dir/'SoulFlowerDesertV4.dll',
                       combined83/'build-proof.json', combined83/'Assembly-CSharp.dll', combined83/'SoulFlowerGGOShop.dll',
                       *[directory/'manifest.json' for _, directory in stages],
                       *[directory/'Assembly-CSharp.dll' for _, directory in stages],
                       stages[0][1]/'HYJS.Feedback65.dll', stages[3][1]/'HYJS.FragmentBoxes85.dll',
                       stages[4][1]/'model-report.json', stages[5][1]/'run-model.log',
                       stages[5][1]/'HYJS.WantedSkip87.dll', awake_dir/'manifest.json',
                       awake_dir/'helper-compatibility.json', awake_dir/'smallassetmanifest',
                       awake_dir/'HYJS.DungeonContent.dll', awake_contract/'tests.json',
                       awake_contract/'contract-tests.log', awake_dir/'combined-scene-audit.json',
                       scene_resources/'resource-manifest.json', scene_resources/'smallassetmanifest',
                       scene_resources/'map_desertfield_001']
    for path in expected_inputs:
        path = combined.local(path)
        name = str(path.relative_to(ROOT))
        combined.require(proof['sourcePins'].get(name) == combined.file_sha(path), 'Static input is not pinned: ' + name)
    state = combined.verify_chain(base, map_dir, field_dir, bundle, combined83, stages)
    final_main = stages[-1][1]/'Assembly-CSharp.dll'
    combined.require(combined.verify_awake_overlay(base, final_main, awake_dir) == proof['awakeProof'],
                     'Awake helper proof differs')
    combined.require(combined.verify_awake_contract(awake_contract) == proof['awakeContractProof'],
                     'Awake contract proof differs')
    combined.replay_awake_overlay(base, final_main, awake_dir, tool_root)
    scene_report = awake_dir/'combined-scene-audit.json'
    map_helper = map_dir/'SoulFlowerDesert53.dll'
    combined.require(combined.verify_awake_scene(scene_report, base, final_main, awake_dir, scene_resources,
                                                  map_helper) == proof['awakeSceneProof'],
                     'Awake scene proof differs')
    combined.replay_awake_scene(base, final_main, awake_dir, scene_resources, map_helper, scene_report)
    combined.require(state['changes83'] == proof['desertTrialChanges'] and state['stages'] == proof['stageProofs']
                     and state['finalChanges'] == proof['finalChanges']
                     and state['evacuationIconSHA256'] == proof['evacuationIconSHA256']
                     and combined.file_sha(field_dir/'Assembly-CSharp.dll') == proof['fieldMainSHA256']
                     and combined.file_sha(field_dir/'SoulFlowerDesertV4.dll') == proof['fieldHelperSHA256'],
                     'Replayed chain differs from independent build proof')
    helpers = {combined.ENTRY: state['main'], combined.MAP: state['map'], combined.FIELD: state['field'],
               combined.SHOP: state['shop'],
               combined.FEEDBACK: (stages[0][1]/'HYJS.Feedback65.dll').read_bytes(),
               combined.FRAGMENT: (stages[3][1]/'HYJS.FragmentBoxes85.dll').read_bytes(),
               combined.DUNGEON: (awake_dir/'HYJS.DungeonContent.dll').read_bytes(),
               combined.WANTED87: (stages[5][1]/'HYJS.WantedSkip87.dll').read_bytes()}
    for entry, raw in helpers.items():
        combined.require(combined.digest(raw) == proof['payloadSHA256'][entry]
                         and (static/Path(entry).name).read_bytes() == raw, 'Static payload differs: ' + entry)
        combined.add_pin(pins, static/Path(entry).name)
        if entry != combined.ENTRY:
            combined.helper_no_absolute_origin(raw, entry)
    breach_dir = (ROOT/proof['breachResourceOverlay']).resolve()
    combined.require(breach_dir.is_relative_to(ROOT)
                     and combined.file_sha(breach_dir/'manifest.json') == proof['breach84OverlayManifestSHA256']
                     and combined.file_sha(breach_dir/'tabletext') == proof['breach84ResourceSHA256']
                     and qa_manifest.get('breach84OverlayManifestSHA256') == proof['breach84OverlayManifestSHA256']
                     and qa_manifest.get('breach84ResourceSHA256') == proof['breach84ResourceSHA256'],
                     'QA and formal breach inputs differ')
    resources = {package_resources.CATALOG_ENTRY: state['expectedCatalog'],
                 package_resources.BUNDLE_ENTRY: bundle.read_bytes()}
    temporary_pins = {}
    breach_proof = apply_breach(base, breach_dir, resources, temporary_pins)
    combined.require(breach_proof == proof['breach84ResourceProof']
                     and qa_record.get('breach84ResourceProof') == breach_proof, 'Breach resource proof differs')
    for path, value in temporary_pins.items():
        combined.add_pin(pins, path, value)
    replacements = qa_record.get('replacementEntries')
    combined.require(isinstance(replacements, dict) and replacements, 'QA packaged entry pins missing')
    with zipfile.ZipFile(qa_path) as archive:
        names = archive.namelist()
        combined.require(len(names) == len(set(names)), 'Duplicate QA APK entries')
        for entry, value in replacements.items():
            combined.require(entry in names and combined.digest(archive.read(entry)) == value, 'QA APK entry differs: ' + entry)
        for entry, raw in helpers.items():
            combined.require(qa_manifest.get('replacementEntries', {}).get(entry) == replacements.get(entry),
                             'QA managed manifest/package mismatch: ' + entry)
            if entry != combined.ENTRY:
                combined.require(archive.read(entry) == raw, 'QA helper differs: ' + entry)
        for entry, raw in resources.items():
            combined.require(entry in replacements and archive.read(entry) == raw, 'QA resource differs: ' + entry)
        combined.require(json.loads(archive.read('assets/local-wallet.json')).get('origin') == combined.QA_ORIGIN,
                         'QA wallet origin differs')
        origin_check = verify_origin(state['main'], archive.read(combined.ENTRY), state['origin'])
    combined.require(qa_manifest.get('qaOriginProof', {}).get('shopActualPayloadAudit') == build_combined83.audit_shop(state['shop'])
                     and qa_manifest.get('qaOriginProof', {}).get('onlyFourHTTPOriginLiteralsChanged') is True,
                     'QA origin or actual shop audit differs')
    combined.require(qa_record.get('desertResourceProof', {}).get('onlyDesertAndNightRowsAndMapEntry') is True
                     and qa_record.get('desertResourceProof', {}).get('nightBundleSHA256') == proof['awakeProof']['nightBundleSHA256']
                     and qa_record.get('desertResourceProof', {}).get('resourceManifestSHA256') == combined.file_sha(qa_resource_path)
                     and qa_record.get('manifestPinRepair', {}).get('changed') is True,
                     'QA resource and six-pin repair proof missing')
    combined.check_pins(pins)
    created = False
    try:
        out.mkdir(parents=True); created = True
        for entry, raw in helpers.items():
            (out/Path(entry).name).write_bytes(raw)
        (out/'smallassetmanifest').write_bytes(state['expectedCatalog'])
        (out/'map_desertfield_001').write_bytes(bundle.read_bytes())
        files = {entry: str((out/Path(entry).name).relative_to(ROOT)) for entry in helpers}
        formal = {'schema': combined.SCHEMA_FORMAL, 'status': 'READY_FOR_SIGNING_NOT_RELEASED',
                  'qaOnly': False, 'neverPromote': False, 'managedOnly': True, 'baseAPKSHA256': combined.BASE_SHA,
                  'inputManagedSHA256': combined.digest(state['original']), 'outputManagedSHA256': combined.digest(state['main']),
                  'inputDexSHA256': combined.digest(state['dex']), 'inheritedJavaByteExact': True,
                  'inheritedJavaPublicProtectedABIExact': True, 'inheritedJavaEntrySHA256': state['inherited'],
                  'payloadFiles': files, 'replacementEntries': {entry: combined.digest(raw) for entry, raw in helpers.items()},
                  'allowedChangedMethods': sorted(x['method'] for x in state['finalChanges']), 'sourcePins': pins,
                  'combinedStaticProof': str(proof_path.relative_to(ROOT)),
                  'linkageProof': linkage_name, 'linkageProofSummary': qa_manifest['linkageProofSummary'],
                  'originalEngineMethodsExact': 547, 'qualityFilterV80Exact': True,
                  'breach84OverlayManifestSHA256': proof['breach84OverlayManifestSHA256'],
                  'breach84ResourceSHA256': proof['breach84ResourceSHA256'],
                  'qaAPKSHA256': qa_sha, 'acceptanceReview': str(combined.local(acceptance_path).relative_to(ROOT)),
                  'acceptanceStatus': acceptance_status,
                  'qaOriginDifferedOnlyInFourVerifiedMethods': origin_check['onlyFourHTTPOriginLiteralsChanged'],
                  'deviceExecuted': False, 'serverExecuted': False}
        combined.write_json(out/'manifest.json', formal)
        resource_names = {package_resources.CATALOG_ENTRY: 'smallassetmanifest', package_resources.BUNDLE_ENTRY: 'map_desertfield_001'}
        combined.write_json(out/'resource-manifest.json', {'schema': package_resources.AWAKE_SCHEMA,
            'baseAPKSHA256': combined.BASE_SHA, 'bundleSHA256': package_resources.BUNDLE_SHA,
            'nightBundleSHA256': proof['awakeProof']['nightBundleSHA256'],
            'bundleBytes': bundle.stat().st_size, 'originalCatalogSHA256': combined.digest(state['catalog']),
            'updatedCatalogSHA256': combined.digest(state['expectedCatalog']),
            'payloadFiles': {entry: str((out/name).relative_to(ROOT)) for entry, name in resource_names.items()},
            'replacementEntries': {entry: combined.file_sha(out/name) for entry, name in resource_names.items()},
            'sourcePins': pins, 'releaseReady': True, 'deviceExecuted': False})
        combined.check_pins(pins)
    except Exception:
        if created:
            shutil.rmtree(out)
        raise
    print(json.dumps({'status': 'READY_FOR_SIGNING_NOT_RELEASED', 'output': str(out),
                      'qaAPKSHA256': qa_sha, 'buildCode': combined.BUILD_CODE, 'versionName': combined.VERSION,
                      'originalEngineMethodsExact': 547, 'signed': False, 'deviceExecuted': False}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--list-gates', action='store_true')
    for name in ('base-apk', 'combined-static', 'qa-apk', 'qa-manifest', 'qa-record', 'output'):
        parser.add_argument('--' + name, type=Path)
    acceptance = parser.add_mutually_exclusive_group()
    acceptance.add_argument('--acceptance', type=Path)
    acceptance.add_argument('--user-acceptance-waiver', type=Path)
    args = parser.parse_args()
    if args.list_gates:
        print('\n'.join((*review83.REQUIRED_CASES, *NEW_CASES)))
        return
    for name in ('base_apk', 'combined_static', 'qa_apk', 'qa_manifest', 'qa_record', 'output'):
        combined.require(getattr(args, name) is not None, 'Missing --' + name.replace('_', '-'))
    combined.require(args.acceptance is not None or args.user_acceptance_waiver is not None,
                     'Missing --acceptance or --user-acceptance-waiver')
    prepare(args)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, KeyError, json.JSONDecodeError) as error:
        print('Release 86 preparation blocked: ' + str(error), file=sys.stderr)
        sys.exit(1)
