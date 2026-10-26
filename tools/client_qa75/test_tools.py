"""Offline guard tests. No real APK is patched, signed, installed or launched."""
import json
from pathlib import Path
import struct
import tempfile
import unittest
from unittest.mock import patch
import zipfile

from tools.client_qa75 import build, package
from tools import build_multiclient as multi
from tools.client_qa75.manifest import patch_package


def axml_fixture(alias=False):
    strings = ["manifest", "package", multi.SOURCE_PACKAGE, multi.ANDROID_NS,
               "application", "label", multi.SOURCE_LABEL, "activity", "uses-permission", "name"]
    payload, offsets = bytearray(), []
    for text in strings:
        raw = text.encode()
        offsets.append(len(payload))
        payload.extend(bytes([len(text.encode("utf-16le")) // 2, len(raw)]) + raw + b"\0")
    payload.extend(b"\0" * (-len(payload) % 4))
    pool = struct.pack("<HHIIIIII", 1, 28, 28 + 4 * len(strings) + len(payload), len(strings), 0, 256, 28 + 4 * len(strings), 0)
    pool += struct.pack("<" + "I" * len(offsets), *offsets) + payload

    def node(tag, ns, name, value):
        return (struct.pack("<HHIII", 0x102, 16, 56, 1, 0xffffffff)
                + struct.pack("<IIHHHHHH", 0xffffffff, tag, 20, 20, 1, 0, 0, 0)
                + struct.pack("<IIIHBBI", ns, name, value, 8, 0, 3, value))

    rest = node(0, 0xffffffff, 1, 2) + node(4, 3, 5, 6) + node(7, 3, 5, 6)
    if alias:
        rest += node(8, 3, 9, 2)
    return struct.pack("<HHI", 3, 8, 8 + len(pool) + len(rest)) + pool + rest


class QA75ToolGuards(unittest.TestCase):
    def test_manifest_binds_exact_apk_and_manifest_bytes(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            base, proof = root / "base.apk", root / "base.json"
            base.write_bytes(b"finite fake APK for SHA guard only")
            proof.write_text(json.dumps({"binarySHA256": build.file_sha(base)}))
            digest = build.file_sha(proof)
            self.assertEqual(build.read_binding(base, proof, digest)["baseAPKSHA256"], build.file_sha(base))
            base.write_bytes(b"changed APK")
            with self.assertRaisesRegex(ValueError, "Base APK differs"):
                build.read_binding(base, proof, digest)
            proof.write_text(json.dumps({"binarySHA256": build.file_sha(base)}))
            with self.assertRaisesRegex(ValueError, "manifest SHA256 mismatch"):
                build.read_binding(base, proof, digest)

    def test_missing_binary_hash_and_symlink_refused(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            base, proof = root / "base.apk", root / "base.json"
            base.write_bytes(b"finite")
            proof.write_text("{}")
            with self.assertRaisesRegex(ValueError, "binarySHA256"):
                build.read_binding(base, proof, build.file_sha(proof))
            link = root / "alias.apk"
            link.symlink_to(base)
            with self.assertRaisesRegex(ValueError, "non-symlink"):
                build.read_binding(link, proof, build.file_sha(proof))

    def test_exact_local_origin_only(self):
        self.assertEqual(build.qa_origin("http://127.0.0.1:18861/"), build.TARGET)
        for origin in ("https://127.0.0.1:18861", "http://127.0.0.1:18862", "http://localhost:18861",
                       "http://remote.invalid", "http://user:pass@127.0.0.1:18861", build.TARGET + "/sao/",
                       build.TARGET + "?q=1", build.TARGET + "#x", " " + build.TARGET):
            with self.subTest(origin=origin), self.assertRaises(ValueError):
                build.qa_origin(origin)

    def test_wallet_fields_and_duplicate_zip_guard(self):
        with tempfile.TemporaryDirectory() as temporary:
            base = Path(temporary) / "base.apk"
            def write(wallet):
                with zipfile.ZipFile(base, "w") as archive:
                    archive.writestr(build.DLL, b"finite DLL placeholder")
                    archive.writestr(build.MANIFEST, axml_fixture())
                    archive.writestr(build.WALLET, json.dumps(wallet))
            write({"origin": "https://source.invalid"})
            self.assertEqual(build.read_base(base)[2], "https://source.invalid")
            write({"origin": "https://source.invalid", "extra": True})
            with self.assertRaisesRegex(ValueError, "configuration fields"):
                build.read_base(base)
            write({"origin": build.TARGET})
            with self.assertRaisesRegex(ValueError, "already be QA75"):
                build.read_base(base)
            write({"origin": "https://source.invalid"})
            with zipfile.ZipFile(base, "a") as archive:
                with self.assertWarns(UserWarning):
                    archive.writestr(build.WALLET, "{}")
            with self.assertRaisesRegex(ValueError, "Duplicate APK"):
                build.read_base(base)

    def test_existing_parameterized_axml_patch_and_alias_guards(self):
        original = axml_fixture()
        patched, changes = multi.patch_manifest(original)
        self.assertEqual(len(changes), 3)
        _, _, strings, _ = multi.read_pool(patched)
        self.assertIn(multi.TARGET_PACKAGE, strings)
        self.assertIn(multi.TARGET_LABEL, strings)
        self.assertNotIn(multi.SOURCE_PACKAGE, strings)
        with self.assertRaisesRegex(ValueError, "shared by an unrelated attribute"):
            multi.patch_manifest(axml_fixture(alias=True))
        with self.assertRaises(ValueError):
            multi.patch_manifest(patched)

    def test_dynamic_targets_and_tool_pins(self):
        targets = build.targets_for("https://frozen.invalid", build.TARGET)
        self.assertEqual(set(targets), set(build.TARGET_METHODS))
        self.assertEqual(targets[build.TARGET_METHODS[0]], ("https://frozen.invalid/sao/", build.TARGET + "/sao/"))
        self.assertEqual(len(build.source_pins()), len(build.SOURCE_FILES))

    def test_package_only_keeps_every_inherited_label(self):
        original=axml_fixture()
        changed,edits,label=patch_package(original)
        self.assertEqual(label,multi.SOURCE_LABEL)
        self.assertEqual(len(edits),1)
        _,_,strings,_=multi.read_pool(changed)
        self.assertIn(multi.SOURCE_LABEL,strings)
        self.assertIn(multi.TARGET_PACKAGE,strings)
        with self.assertRaisesRegex(ValueError,"shared by an unrelated attribute"):
            patch_package(axml_fixture(alias=True))
        with self.assertRaisesRegex(ValueError,"reviewed .local package"):
            patch_package(changed)

    def test_bad_frozen_input_never_invokes_compiler_or_signer(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            base, proof, output = root / "base.apk", root / "base.json", root / "unused-output"
            base.write_bytes(b"unreviewed")
            proof.write_text(json.dumps({"binarySHA256": "0" * 64}))
            with patch.object(build, "run", side_effect=AssertionError("compiler invoked")), patch.object(multi, "run", side_effect=AssertionError("signer invoked")):
                with self.assertRaisesRegex(ValueError, "Base APK differs"):
                    build.build(base, proof, build.file_sha(proof), build.TARGET, output, root)
                with self.assertRaisesRegex(ValueError, "Base APK differs"):
                    package.package(base, proof, build.file_sha(proof), output, root / "unused-key")
            self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
