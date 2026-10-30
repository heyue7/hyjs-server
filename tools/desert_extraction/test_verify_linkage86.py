"""Synthetic input-boundary tests only; no real game or acceptance evidence."""
import json
from pathlib import Path
import tempfile
import unittest
import warnings
import zipfile

from tools.desert_extraction import verify_linkage86 as linkage


class LinkageInputFixtures(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='linkage86-input-fixture-', dir=linkage.ROOT/'build')
        self.directory = Path(self.temp.name)
        self.overlay = self.directory/'static'
        self.overlay.mkdir()
        hashes = {}
        for name in linkage.NAMES:
            path = self.overlay/name
            path.write_bytes(('synthetic-' + name).encode())
            hashes[linkage.PREFIX + name] = linkage.file_sha(path)
        source_fixture = self.directory/'source.txt'
        source_fixture.write_text('synthetic source', encoding='utf-8')
        self.proof = {'schema': linkage.STATIC_SCHEMA, 'status': 'STATIC_PREVIEW_NOT_FOR_RELEASE',
                      'baseAPKSHA256': linkage.BASE_SHA, 'payloadSHA256': hashes,
                      'sourcePins': {str(source_fixture.relative_to(linkage.ROOT)): linkage.file_sha(source_fixture)}}
        self.proof_path = self.overlay/'build-proof.json'
        self.write_proof()

    def tearDown(self):
        self.temp.cleanup()

    def write_proof(self):
        self.proof_path.write_text(json.dumps(self.proof), encoding='utf-8')

    def base_zip(self, extra=()):
        path = self.directory/'base.zip'
        with warnings.catch_warnings():
            warnings.simplefilter('ignore', UserWarning)
            with zipfile.ZipFile(path, 'w') as archive:
                for name in ('Assembly-CSharp.dll', 'SoulFlowerGGOShop.dll', 'HYJS.Feedback65.dll', *extra):
                    archive.writestr(linkage.PREFIX + name, b'synthetic')
        return path

    def test_every_static_dll_is_pinned_to_its_actual_bytes(self):
        proof, hashes = linkage.checked_overlay(self.overlay, linkage.BASE_SHA)
        self.assertEqual(proof, self.proof_path.resolve())
        self.assertEqual(set(hashes), set(linkage.NAMES))
        (self.overlay/linkage.NAMES[-1]).write_bytes(b'changed')
        with self.assertRaisesRegex(ValueError, 'SHA differs'):
            linkage.checked_overlay(self.overlay, linkage.BASE_SHA)

    def test_static_inventory_and_qa_isolation_are_required(self):
        removed = linkage.PREFIX + linkage.NAMES[-1]
        digest = self.proof['payloadSHA256'].pop(removed)
        self.write_proof()
        with self.assertRaisesRegex(ValueError, 'payload inventory'):
            linkage.checked_overlay(self.overlay, linkage.BASE_SHA)
        self.proof['payloadSHA256'][removed] = digest
        self.proof['qaOnly'] = True
        self.write_proof()
        with self.assertRaisesRegex(ValueError, 'independent 86 static'):
            linkage.checked_overlay(self.overlay, linkage.BASE_SHA)

    def test_static_source_pin_must_match_and_stay_in_checkout(self):
        source_name = next(iter(self.proof['sourcePins']))
        (linkage.ROOT/source_name).write_text('changed', encoding='utf-8')
        with self.assertRaisesRegex(ValueError, 'source pin differs'):
            linkage.checked_overlay(self.overlay, linkage.BASE_SHA)
        self.proof['sourcePins'] = {'/private/tmp/not-in-checkout': '0'*64}
        self.write_proof()
        with self.assertRaisesRegex(ValueError, 'source pin differs'):
            linkage.checked_overlay(self.overlay, linkage.BASE_SHA)

    def test_managed_inventory_rejects_duplicate_and_nested_entries(self):
        for names in (('Assembly-CSharp.dll',), ('Nested/Other.dll',), ('assembly-csharp.dll',)):
            path = self.base_zip(names)
            with zipfile.ZipFile(path) as archive:
                with self.subTest(names=names), self.assertRaises(ValueError):
                    linkage.managed_entries(archive)

    def test_managed_inventory_requires_original_feedback_and_shop(self):
        path = self.directory/'missing.zip'
        with zipfile.ZipFile(path, 'w') as archive:
            archive.writestr(linkage.PREFIX + 'Assembly-CSharp.dll', b'synthetic')
        with zipfile.ZipFile(path) as archive:
            with self.assertRaisesRegex(ValueError, 'missing an inherited'):
                linkage.managed_entries(archive)

    def test_checker_inventory_cannot_omit_or_change_a_real_input(self):
        inventory = [{'file': name, 'identity': Path(name).stem, 'sha256': str(i)}
                     for i, name in enumerate(linkage.NAMES)]
        expected = {row['file']: row['sha256'] for row in inventory}
        result = {'schema': 'hyjs-combined86-linkage-result-v1', 'inventory': inventory,
                  'roots': [Path(name).stem for name in linkage.NAMES], 'problems': []}
        linkage.check_result(result, expected)
        inventory[-1]['sha256'] = 'changed'
        with self.assertRaisesRegex(ValueError, 'inventory does not match'):
            linkage.check_result(result, expected)
        inventory.pop()
        with self.assertRaisesRegex(ValueError, 'inventory does not match'):
            linkage.check_result(result, expected)


if __name__ == '__main__':
    unittest.main()
