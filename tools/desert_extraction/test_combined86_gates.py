"""Small synthetic gate tests; none of these fixtures are release evidence."""
import json
import hashlib
from pathlib import Path
import tempfile
import unittest
import zipfile
from types import SimpleNamespace
from unittest import mock

from tools.client_release import package_native
from tools.desert_extraction import build_combined86 as combined, package_resources
from tools.desert_extraction import prepare_release_package as review83
from tools.desert_extraction import prepare_release_package86 as gate


class GateFixtures(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='combined86-gate-synthetic-', dir=combined.ROOT/'build')
        self.dir = Path(self.temp.name)
        self.evidence = self.dir/'synthetic-evidence.txt'
        self.evidence.write_text('synthetic fixture, not acceptance evidence\n')
        ref = {'path': str(self.evidence.relative_to(combined.ROOT)), 'sha256': combined.file_sha(self.evidence)}
        self.review = {'qaAPKSHA256': '1'*64, 'reviewed': True, 'reviewer': 'synthetic-test',
                       'scope': 'Synthetic unit test only; this is not a release acceptance review.',
                       'latencyMs': {'gather': 1000, 'monster': 1000},
                       'cases': {key: [dict(ref)] for key in (*review83.REQUIRED_CASES, *gate.NEW_CASES)}}
        self.review_path = self.dir/'synthetic-review.json'
        self.review_path.write_text(json.dumps(self.review))
        self.breach = self.dir/'breach'; self.breach.mkdir()
        (self.breach/'manifest.json').write_text('{"synthetic": true}')
        (self.breach/'tabletext').write_bytes(b'synthetic text')
        self.static = self.dir/'static'; self.static.mkdir()
        payload = {}
        for name in package_native.MANAGED86:
            (self.static/Path(name).name).write_bytes(('synthetic '+name).encode())
            payload[name] = combined.file_sha(self.static/Path(name).name)
        static_proof = self.static/'build-proof.json'
        static_proof.write_text(json.dumps({'schema': 'hyjs-desert-trial-feedback-fragment86-static-proof-v1',
                                           'status': 'STATIC_PREVIEW_NOT_FOR_RELEASE',
                                           'baseAPKSHA256': combined.BASE_SHA, 'payloadSHA256': payload}))
        self.linkage_path = self.dir/'linkage.json'
        self.linkage_path.write_text('{"synthetic": true}')
        self.linkage_summary = {'synthetic': True}
        proof_name = str(static_proof.relative_to(combined.ROOT))
        linkage_name = str(self.linkage_path.relative_to(combined.ROOT))
        review_name = str(self.review_path.relative_to(combined.ROOT))
        pins = {proof_name: combined.file_sha(static_proof), review_name: combined.file_sha(self.review_path),
                linkage_name: combined.file_sha(self.linkage_path)}
        self.qa = {'schema': combined.SCHEMA_QA, 'qaOnly': True, 'neverPromote': True,
                   'qaOrigin': combined.QA_ORIGIN, 'managedOnly': True,
                   'status': 'STATIC_VERIFIED_QA_NOT_RELEASED', 'baseAPKSHA256': combined.BASE_SHA,
                   'qaOriginProof': {'onlyFourHTTPOriginLiteralsChanged': True},
                   'combinedStaticProof': proof_name, 'sourcePins': pins,
                   'linkageProof': linkage_name, 'linkageProofSummary': self.linkage_summary,
                   'payloadFiles': {name: 'synthetic' for name in payload}, 'replacementEntries': payload,
                   'breach84OverlayManifestSHA256': combined.file_sha(self.breach/'manifest.json'),
                   'breach84ResourceSHA256': combined.file_sha(self.breach/'tabletext')}
        self.formal = dict(self.qa, schema=combined.SCHEMA_FORMAL, qaOnly=False, neverPromote=False,
                           status='READY_FOR_SIGNING_NOT_RELEASED', qaAPKSHA256='1'*64,
                           qaOriginDifferedOnlyInFourVerifiedMethods=True, acceptanceReview=review_name)
        self.formal.pop('qaOrigin')
        self.qa_overlay, self.formal_overlay = self.dir/'qa-overlay', self.dir/'formal-overlay'
        self.qa_overlay.mkdir(); self.formal_overlay.mkdir()
        common_resource = {'schema': package_resources.AWAKE_SCHEMA, 'baseAPKSHA256': combined.BASE_SHA}
        self.qa_resource = dict(common_resource, releaseReady=False, qaOnly=True, neverPromote=True,
                                qaOrigin=combined.QA_ORIGIN)
        self.formal_resource = dict(common_resource, releaseReady=True)
        (self.qa_overlay/'resource-manifest.json').write_text(json.dumps(self.qa_resource))
        (self.formal_overlay/'resource-manifest.json').write_text(json.dumps(self.formal_resource))

    def tearDown(self):
        self.temp.cleanup()

    def review_gate(self):
        self.review_path.write_text(json.dumps(self.review))
        return gate.reviewed_cases(self.review_path, '1'*64)

    def package_gate(self, proof, directory=None, build=combined.BUILD_CODE, version=combined.VERSION,
                     desert=True, repair=True, origin=None, managed=None):
        if managed is None:
            managed = self.qa_overlay if proof.get('schema') == combined.SCHEMA_QA else self.formal_overlay
        resource = managed if desert is True else None if desert is False else desert
        with mock.patch.object(combined, 'verify_linkage_report', return_value=self.linkage_summary):
            return package_native.validate_breach86_request(proof, directory, build, version,
                                                            resource, repair, origin, managed)

    def test_review_requires_all_old_and_new_scopes(self):
        result, pins = self.review_gate()
        self.assertEqual(len(result['cases']), len(review83.REQUIRED_CASES) + len(gate.NEW_CASES))
        self.assertIn(str(self.evidence.relative_to(combined.ROOT)), pins)

    def test_each_new_scope_is_required(self):
        for key in gate.NEW_CASES:
            saved = self.review['cases'].pop(key)
            with self.subTest(key=key), self.assertRaisesRegex(ValueError, 'Missing accepted evidence'):
                self.review_gate()
            self.review['cases'][key] = saved

    def test_original_scope_is_required(self):
        del self.review['cases'][review83.REQUIRED_CASES[0]]
        with self.assertRaisesRegex(ValueError, 'Missing accepted evidence'):
            self.review_gate()

    def test_both_first_frame_thresholds_and_nan(self):
        for key, value in (('gather', 1001), ('monster', 1001), ('monster', float('nan'))):
            original = self.review['latencyMs'][key]
            self.review['latencyMs'][key] = value
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                self.review_gate()
            self.review['latencyMs'][key] = original

    def test_review_identity_and_acceptance_required(self):
        self.review['qaAPKSHA256'] = '2'*64
        with self.assertRaisesRegex(ValueError, 'different QA APK'):
            self.review_gate()
        self.review['qaAPKSHA256'] = '1'*64
        self.review['reviewed'] = False
        with self.assertRaisesRegex(ValueError, 'named review'):
            self.review_gate()

    def test_review_file_sha_and_nonempty_evidence(self):
        self.evidence.write_text('changed')
        with self.assertRaisesRegex(ValueError, 'pin differs'):
            self.review_gate()
        self.evidence.write_bytes(b'')
        with self.assertRaisesRegex(ValueError, 'empty'):
            self.review_gate()

    def test_review_rejects_file_outside_checkout(self):
        self.review['cases'][gate.NEW_CASES[0]][0]['path'] = '/private/tmp/foreign-evidence'
        with self.assertRaisesRegex(ValueError, 'inside project'):
            self.review_gate()

    def test_qa_and_formal_exact_inputs(self):
        self.package_gate(self.qa, self.breach, origin=combined.QA_ORIGIN)
        self.package_gate(self.formal, self.breach)

    def test_86_requires_resource_pair_and_repair(self):
        for directory, desert, repair in ((None, True, True), (self.breach, False, True), (self.breach, True, False)):
            with self.subTest(directory=directory, desert=desert, repair=repair), self.assertRaises(ValueError):
                self.package_gate(self.formal, directory, desert=desert, repair=repair)

    def test_only_86_version_and_schema_allow_breach(self):
        for build, version in ((2026102901, combined.VERSION), (combined.BUILD_CODE, '1.0.83')):
            with self.subTest(build=build, version=version), self.assertRaises(ValueError):
                self.package_gate(self.formal, self.breach, build, version)
        with self.assertRaises(ValueError):
            self.package_gate({'schema': 'other'}, self.breach)
        self.package_gate({'schema': 'other'})

    def test_qa_markers_and_formal_no_qa_promotion(self):
        for marker in ('qaOnly', 'neverPromote'):
            record = dict(self.qa, **{marker: False})
            with self.subTest(marker=marker), self.assertRaises(ValueError):
                self.package_gate(record, self.breach, origin=combined.QA_ORIGIN)
        with self.assertRaises(ValueError):
            self.package_gate(self.qa, self.breach, origin='http://127.0.0.1:9999')
        with self.assertRaises(ValueError):
            self.package_gate(dict(self.formal, qaOnly=True), self.breach)
        with self.assertRaises(ValueError):
            self.package_gate(dict(self.formal, diagnosticOnly=True), self.breach)

    def test_formal_cannot_promote_qa_managed_bytes(self):
        fake = dict(self.formal)
        fake['replacementEntries'] = dict(self.formal['replacementEntries'])
        fake['replacementEntries'][combined.ENTRY] = '0'*64
        with self.assertRaisesRegex(ValueError, 'static payload'):
            self.package_gate(fake, self.breach)
        fake = dict(self.formal, combinedStaticProof='qa-loopback/manifest.json')
        with self.assertRaisesRegex(ValueError, 'static proof pin missing'):
            self.package_gate(fake, self.breach)

    def test_both_new_helpers_are_required_and_bound_to_static(self):
        for name in (combined.DUNGEON, combined.WANTED87):
            with self.subTest(name=name):
                missing = dict(self.formal, payloadFiles=dict(self.formal['payloadFiles']))
                missing['payloadFiles'].pop(name)
                with self.assertRaisesRegex(ValueError, 'managed entry scope'):
                    self.package_gate(missing, self.breach)
                altered = dict(self.formal, replacementEntries=dict(self.formal['replacementEntries']))
                altered['replacementEntries'][name] = '0'*64
                with self.assertRaisesRegex(ValueError, 'static payload'):
                    self.package_gate(altered, self.breach)

    def test_linkage_proof_is_required_and_immutable(self):
        altered = dict(self.formal, linkageProof='missing.json')
        with self.assertRaisesRegex(ValueError, 'linkage proof pin missing'):
            self.package_gate(altered, self.breach)
        self.linkage_path.write_text('{"tampered": true}')
        with self.assertRaisesRegex(ValueError, 'linkage proof differs'):
            self.package_gate(self.formal, self.breach)

    def test_old_single_row_resource_schema_is_not_a_combined85_release(self):
        path = self.formal_overlay/'resource-manifest.json'
        path.write_text(json.dumps(dict(self.formal_resource, schema=package_resources.MAP_SCHEMA)))
        with self.assertRaisesRegex(ValueError, 'resource manifest differs'):
            self.package_gate(self.formal, self.breach)

    def test_desert_and_night_catalog_is_replayed_from_pinned_resources(self):
        from tools.client_awake85_layout import build as awake
        base = self.dir/'resource-base.apk'
        directory = self.dir/'resource-overlay'; directory.mkdir()
        bundle, night = b'synthetic desert', b'synthetic night'
        catalog = b'3000\nmap/example\t0123456789abcdef0123456789abcdef\t10\n'
        with zipfile.ZipFile(base, 'w') as archive:
            archive.writestr(package_resources.CATALOG_ENTRY, catalog)
            archive.writestr(awake.NIGHT, night)
        base_sha = combined.file_sha(base)
        bundle_sha, night_sha = combined.digest(bundle), combined.digest(night)
        with mock.patch.multiple(package_resources, BASE_SHA=base_sha, BUNDLE_SHA=bundle_sha,
                                 BUNDLE_BYTES=len(bundle)), \
             mock.patch.multiple(awake, NIGHT_SHA=night_sha, NIGHT_MD5=hashlib.md5(night).hexdigest(),
                                 NIGHT_SIZE=len(night)):
            final = package_resources.compose_awake_catalog(catalog, bundle, night)
            (directory/'smallassetmanifest').write_bytes(final)
            (directory/'map_desertfield_001').write_bytes(bundle)
            files = {package_resources.CATALOG_ENTRY: 'smallassetmanifest',
                     package_resources.BUNDLE_ENTRY: 'map_desertfield_001'}
            report = {'schema': package_resources.AWAKE_SCHEMA, 'baseAPKSHA256': base_sha,
                      'bundleSHA256': bundle_sha, 'bundleBytes': len(bundle), 'nightBundleSHA256': night_sha,
                      'originalCatalogSHA256': combined.digest(catalog), 'updatedCatalogSHA256': combined.digest(final),
                      'payloadFiles': {k: str((directory/v).relative_to(combined.ROOT)) for k, v in files.items()},
                      'replacementEntries': {k: combined.file_sha(directory/v) for k, v in files.items()},
                      'sourcePins': {str(self.evidence.relative_to(combined.ROOT)): combined.file_sha(self.evidence)}}
            (directory/'resource-manifest.json').write_text(json.dumps(report))
            payload, pins = {}, {}
            proof = package_resources.apply_overlay(base, directory, payload, pins)
            self.assertTrue(proof['onlyDesertAndNightRowsAndMapEntry'])
            self.assertEqual(payload[package_resources.CATALOG_ENTRY], final)
            self.assertTrue(final.startswith(catalog))
            self.assertEqual(final.count(b'map/map_desertfield_001\t'), 1)
            self.assertEqual(final.count(b'map/map_nightforestfield_001\t'), 1)
            with self.assertRaises(ValueError):
                package_resources.compose_awake_catalog(catalog, bundle, b'changed night')
            with self.assertRaises(ValueError):
                package_resources.compose_awake_catalog(catalog+b'map/map_nightforestfield_001\tbad\t1\n', bundle, night)
            (directory/'smallassetmanifest').write_bytes(final+b'other\n')
            with self.assertRaises(ValueError):
                package_resources.apply_overlay(base, directory, {}, {})
            legacy = package_resources.compose_catalog(catalog, bundle)
            (directory/'smallassetmanifest').write_bytes(legacy)
            report.update(schema=package_resources.MAP_SCHEMA, updatedCatalogSHA256=combined.digest(legacy))
            report['replacementEntries'][package_resources.CATALOG_ENTRY] = combined.digest(legacy)
            report.pop('nightBundleSHA256')
            (directory/'resource-manifest.json').write_text(json.dumps(report))
            legacy_payload = {}
            legacy_proof = package_resources.apply_overlay(base, directory, legacy_payload, {})
            self.assertTrue(legacy_proof['onlyExactAppendAndMapEntry'])
            self.assertEqual(legacy_payload[package_resources.CATALOG_ENTRY], legacy)

    def test_scene_audit_binds_main_catalog_helper_and_all_five_chapters(self):
        base, main, helper = (self.dir/name for name in ('base.apk', 'main.dll', 'map.dll'))
        for path in (base, main, helper):
            path.write_bytes(path.name.encode())
        awake, resource = self.dir/'awake', self.dir/'resource'
        awake.mkdir(); resource.mkdir()
        for path in (awake/'HYJS.DungeonContent.dll', awake/'manifest.json', awake/'helper-compatibility.json',
                     resource/'resource-manifest.json', resource/'smallassetmanifest'):
            path.write_bytes(path.name.encode())
        report_path = self.dir/'scene.json'
        report = {'baseAPKSHA256': combined.file_sha(base), 'mainSHA256': combined.file_sha(main),
                  'helperSHA256': combined.file_sha(awake/'HYJS.DungeonContent.dll'),
                  'overlayManifestSHA256': combined.file_sha(awake/'manifest.json'),
                  'helperCompatibilitySHA256': combined.file_sha(awake/'helper-compatibility.json'),
                  'combinedResourceManifestSHA256': combined.file_sha(resource/'resource-manifest.json'),
                  'catalogSHA256': combined.file_sha(resource/'smallassetmanifest'),
                  'desertHelperSHA256': combined.file_sha(helper),
                  'auditSourceSHA256': combined.file_sha(combined.ROOT/'tools/client_awake85_layout/scene_audit.py'),
                  'desertLoaderOverride': True, 'awakeEntryReadyAnyChapter': True,
                  'chapters': [{'chapter': str(n)} for n in range(1, 6)]}
        with mock.patch.object(combined, 'BASE_SHA', combined.file_sha(base)):
            report_path.write_text(json.dumps(report))
            self.assertEqual(combined.verify_awake_scene(report_path, base, main, awake, resource, helper)['chapters'], 5)
            for key, value in (('mainSHA256', '0'*64), ('catalogSHA256', '0'*64),
                               ('desertHelperSHA256', '0'*64), ('chapters', report['chapters'][:-1])):
                report_path.write_text(json.dumps(dict(report, **{key: value})))
                with self.subTest(key=key), self.assertRaises(ValueError):
                    combined.verify_awake_scene(report_path, base, main, awake, resource, helper)

    def test_resource_directory_must_match_its_managed_overlay(self):
        with self.assertRaisesRegex(ValueError, 'managed overlay directory'):
            self.package_gate(self.formal, self.breach, desert=self.qa_overlay)
        with self.assertRaisesRegex(ValueError, 'managed overlay directory'):
            self.package_gate(self.qa, self.breach, desert=self.formal_overlay, origin=combined.QA_ORIGIN)

    def test_managed_payload_path_must_resolve_inside_its_overlay(self):
        name = 'Assembly-CSharp.dll'
        formal_file, qa_file = self.formal_overlay/name, self.qa_overlay/name
        formal_file.write_bytes(b'synthetic identical payload')
        qa_file.write_bytes(formal_file.read_bytes())
        formal_relative = str(formal_file.relative_to(combined.ROOT))
        qa_relative = str(qa_file.relative_to(combined.ROOT))
        for schema in (combined.SCHEMA_QA, combined.SCHEMA_FORMAL):
            with self.subTest(schema=schema):
                self.assertEqual(package_native.resolve_payload_file(self.formal_overlay, formal_relative, schema),
                                 formal_file.resolve())
                with self.assertRaisesRegex(ValueError, 'outside its overlay'):
                    package_native.resolve_payload_file(self.formal_overlay, qa_relative, schema)
        alias = self.formal_overlay/'qa-alias.dll'
        alias.symlink_to(qa_file)
        with self.assertRaisesRegex(ValueError, 'outside its overlay'):
            package_native.resolve_payload_file(self.formal_overlay, str(alias.relative_to(combined.ROOT)), combined.SCHEMA_FORMAL)
        self.assertEqual(package_native.resolve_payload_file(self.formal_overlay, qa_relative, 'legacy-schema'), qa_file.resolve())

    def test_resource_isolation_markers_and_readiness_are_mandatory(self):
        qa_path = self.qa_overlay/'resource-manifest.json'
        for key, value in (('releaseReady', True), ('qaOnly', False), ('neverPromote', False),
                           ('qaOrigin', 'http://127.0.0.1:9999')):
            qa_path.write_text(json.dumps(dict(self.qa_resource, **{key: value})))
            with self.subTest(kind='qa', key=key), self.assertRaisesRegex(ValueError, 'QA resource isolation'):
                self.package_gate(self.qa, self.breach, origin=combined.QA_ORIGIN)
        qa_path.write_text(json.dumps(self.qa_resource))
        formal_path = self.formal_overlay/'resource-manifest.json'
        for key, value in (('releaseReady', False), ('qaOnly', True), ('neverPromote', True),
                           ('qaOrigin', combined.QA_ORIGIN), ('qaOnly', False), ('neverPromote', False),
                           ('qaOrigin', '')):
            formal_path.write_text(json.dumps(dict(self.formal_resource, **{key: value})))
            with self.subTest(kind='formal', key=key), self.assertRaisesRegex(ValueError, 'formal resource'):
                self.package_gate(self.formal, self.breach)

    def test_changed_breach_resource_or_manifest_is_rejected(self):
        (self.breach/'tabletext').write_bytes(b'tampered')
        with self.assertRaises(ValueError):
            self.package_gate(self.formal, self.breach)
        (self.breach/'tabletext').write_bytes(b'synthetic text')
        (self.breach/'manifest.json').write_text('{}')
        with self.assertRaises(ValueError):
            self.package_gate(self.formal, self.breach)

    def test_stage_input_output_and_source_pins_are_exact(self):
        before, after = b'before', b'after'
        record = {'schema': 'synthetic-stage', 'inputManagedSHA256': combined.digest(before),
                  'outputManagedSHA256': combined.digest(after), 'sourcePins': {
                      str(self.evidence.relative_to(combined.ROOT)): combined.file_sha(self.evidence)},
                  'engine': 547}
        combined.assert_stage_manifest(record, 'synthetic-stage', before, after, {'engine': 547})
        for field, value in (('inputManagedSHA256', '0'*64), ('outputManagedSHA256', '0'*64), ('engine', 546),
                             ('qaOnly', True)):
            altered = dict(record, **{field: value})
            with self.subTest(field=field), self.assertRaises(ValueError):
                combined.assert_stage_manifest(altered, 'synthetic-stage', before, after, {'engine': 547})
        self.evidence.write_text('changed')
        with self.assertRaises(ValueError):
            combined.assert_stage_manifest(record, 'synthetic-stage', before, after, {'engine': 547})

    def test_field_manifest_binds_both_payloads_and_map_sources(self):
        field = self.dir/'field'; field.mkdir()
        (field/'Assembly-CSharp.dll').write_bytes(b'synthetic main')
        (field/'SoulFlowerDesertV4.dll').write_bytes(b'synthetic helper')
        main = combined.file_sha(field/'Assembly-CSharp.dll')
        helper = combined.file_sha(field/'SoulFlowerDesertV4.dll')
        sources = {name: combined.file_sha(combined.ROOT/name) for name in combined.required_field_source_pins()}
        manifest = {'schema': 'hyjs-desert-v4-scene-overlay-v2', 'status': 'STATIC_PREVIEW_NOT_FOR_RELEASE',
                    'apkSHA256': combined.BASE_SHA, 'outputManagedSHA256': main,
                    'helperSHA256': helper, 'sourceSHA256': sources}
        target = field/'manifest.json'
        target.write_text(json.dumps(manifest))
        combined.validate_field_manifest(field, main, helper)
        with self.assertRaises(ValueError):
            combined.validate_field_manifest(field, '0'*64, helper)
        with self.assertRaises(ValueError):
            combined.validate_field_manifest(field, main, '0'*64)
        target.write_text(json.dumps(dict(manifest, qaOnly=True)))
        with self.assertRaises(ValueError):
            combined.validate_field_manifest(field, main, helper)
        for name in sorted(sources):
            altered = dict(manifest, sourceSHA256=dict(sources))
            altered['sourceSHA256'].pop(name)
            target.write_text(json.dumps(altered))
            with self.subTest(missing=name), self.assertRaises(ValueError):
                combined.validate_field_manifest(field, main, helper)
            altered = dict(manifest, sourceSHA256=dict(sources))
            altered['sourceSHA256'][name] = '0'*64
            target.write_text(json.dumps(altered))
            with self.subTest(stale=name), self.assertRaises(ValueError):
                combined.validate_field_manifest(field, main, helper)
        altered = dict(manifest, sourceSHA256=dict(sources))
        extra = 'tools/client_remote/build.py'
        altered['sourceSHA256'][extra] = combined.file_sha(combined.ROOT/extra)
        target.write_text(json.dumps(altered))
        with self.assertRaisesRegex(ValueError, 'inventory differs'):
            combined.validate_field_manifest(field, main, helper)

    def test_embedded_evacuation_asset_must_equal_source(self):
        asset = combined.ROOT/'tools/desert_extraction/assets/evacuation-npc.png'
        original = asset.read_bytes()
        row = SimpleNamespace(name='HYJS.DesertV4.Evacuation.png', data=original)
        pe = SimpleNamespace(net=SimpleNamespace(resources=[row]))
        with mock.patch('dnfile.dnPE', return_value=pe):
            self.assertEqual(combined.verify_embedded_evacuation_icon(b'synthetic'), combined.digest(original))
            row.data = b'other'
            with self.assertRaises(ValueError):
                combined.verify_embedded_evacuation_icon(b'synthetic')
            pe.net.resources = [row, row]
            with self.assertRaises(ValueError):
                combined.verify_embedded_evacuation_icon(b'synthetic')

    def test_final_method_union_preserves_each_stage_body(self):
        desert = {'method': 'synthetic.desert', 'token': '0x06000001', 'afterSHA256': 'a'}
        trial = {'method': 'synthetic.trial', 'token': '0x06000002', 'afterSHA256': 'b'}
        wanted = {'method': 'synthetic.wanted', 'token': '0x06000003', 'afterSHA256': 'c'}
        final = [dict(desert), dict(trial), dict(wanted)]
        combined.verify_method_owners([desert, trial], [[wanted]], final)
        with self.assertRaisesRegex(ValueError, 'Overlapping'):
            combined.verify_method_owners([desert], [[dict(desert)]], final)
        with self.assertRaisesRegex(ValueError, 'union'):
            combined.verify_method_owners([desert, trial], [[wanted]], final[:-1])
        final[0]['afterSHA256'] = 'changed-by-later-stage'
        with self.assertRaisesRegex(ValueError, 'later stage'):
            combined.verify_method_owners([desert, trial], [[wanted]], final)


if __name__ == '__main__':
    unittest.main()
