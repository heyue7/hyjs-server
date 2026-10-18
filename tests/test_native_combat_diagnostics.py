"""Original APK diagnostics change call operands only, including guarded failures."""
import contextlib
import io
import struct
import unittest
from pathlib import Path

import dnfile
from dncil.cil.body.reader import read_method_body_from_bytes
from tools.native_combat_diagnostics import patch
from tools.patch_client import patch_dll

ROOT = Path(__file__).resolve().parents[1]
EXPECTED = {
    "Battle.Engine.ComputeCore::BattleEnd": 1,
    "Battle.Engine.ComputeCore::RoundBegin": 4,
    "Battle.Engine.LogicModel.FActor::AddHp": 1,
    "Battle.Engine.LogicModel.FActor::DoDead": 1,
    "Battle.Engine.LogicModel.FPlayer::AddAnger": 1,
    "Battle.Engine.LogicModel.FRelease::Do": 6,
    "Battle.Engine.LogicModel.FRelease::ProcMagicStone": 1,
    "Battle.Engine.LogicModel.FSkill::Do": 1,
    "Battle.Engine.util.RandomUtil::SetSeed": 1,
}


class NativeCombatDiagnosticsTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.original = (ROOT / "analysis/extracted/assets/bin/Data/Managed/Assembly-CSharp.dll").read_bytes()
        cls.enabled = patch(cls.original)
        cls.pe = dnfile.dnPE(data=cls.original)
        cls.rows = {}
        for owner in cls.pe.net.mdtables.TypeDef.rows:
            full = str(owner.TypeNamespace) + "." + str(owner.TypeName)
            for method in owner.MethodList:
                cls.rows[full + "::" + str(method.row.Name)] = (method.row_index, method.row)
        cls.source_index, cls.stub = cls.rows["Battle.Engine.Debug::LogFightMsg"]
        cls.source = 0x06000000 | cls.source_index
        cls.target = next(0x0A000000 | index for index, member in enumerate(cls.pe.net.mdtables.MemberRef.rows, 1)
                          if str(member.Name) == "Log" and getattr(member.Class.row, "TypeNamespace", None) is not None
                          and str(member.Class.row.TypeNamespace) == "UnityEngine"
                          and str(member.Class.row.TypeName) == "Debug"
                          and member.Signature.value == bytes.fromhex("0001011c"))
        cls.sites = []
        for name in EXPECTED:
            _, row = cls.rows[name]
            start = cls.pe.get_offset_from_rva(row.Rva)
            body = read_method_body_from_bytes(cls.pe.get_data(row.Rva))
            for instruction in body.instructions:
                if instruction.opcode.name == "call" and getattr(instruction.operand, "value", None) == cls.source:
                    cls.sites.append((name, start + instruction.offset))

    def test_exact_original_17_calls_and_all_other_assembly_bytes_preserved(self):
        self.assertEqual(len(self.original), len(self.enabled))
        counts = {name: 0 for name in EXPECTED}
        allowed = set()
        for name, offset in self.sites:
            counts[name] += 1
            self.assertEqual(self.original[offset:offset + 5], b"\x28" + struct.pack("<I", self.source))
            self.assertEqual(self.enabled[offset:offset + 5], b"\x28" + struct.pack("<I", self.target))
            allowed.update(range(offset + 1, offset + 5))
        self.assertEqual(counts, EXPECTED)
        changed = {index for index, pair in enumerate(zip(self.original, self.enabled)) if pair[0] != pair[1]}
        self.assertEqual(len(changed), 51)
        self.assertTrue(changed <= allowed)
        # Comparing the complete image covers metadata, PE sections, method
        # headers, branch operands, locals and exception tables as well as code.
        restored = bytearray(self.enabled)
        for _, offset in self.sites:
            restored[offset + 1:offset + 5] = struct.pack("<I", self.source)
        self.assertEqual(bytes(restored), self.original)

    def test_matching_static_object_signatures_and_original_stub_untouched(self):
        self.assertEqual(self.stub.Signature.value, bytes.fromhex("0001011c"))
        member = self.pe.net.mdtables.MemberRef.rows[(self.target & 0xFFFFFF) - 1]
        self.assertEqual(member.Signature.value, self.stub.Signature.value)
        start = self.pe.get_offset_from_rva(self.stub.Rva)
        body = read_method_body_from_bytes(self.pe.get_data(self.stub.Rva))
        self.assertEqual([instruction.opcode.name for instruction in body.instructions], ["ret"])
        self.assertEqual(self.original[start:start + body.header_size + body.code_size],
                         self.enabled[start:start + body.header_size + body.code_size])

    def test_existing_optional_client_patch_composes_without_more_changed_regions(self):
        with contextlib.redirect_stdout(io.StringIO()):
            normal = patch_dll(self.original, "http://127.0.0.1:18088/sao/", optional_town_guides=True,
                               repair_copy_navigation=True, demo_replay_controls=True)
        diagnostic = patch(normal)
        self.assertEqual(len(normal), len(diagnostic))
        self.assertEqual(sum(a != b for a, b in zip(normal, diagnostic)), 51)

    def test_enabled_patch_is_idempotent(self):
        self.assertEqual(patch(self.enabled), self.enabled)

    def test_nonstatic_call_rejected_before_mutation(self):
        raw = bytearray(self.original)
        raw[self.sites[0][1]] = 0x6F
        with self.assertRaisesRegex(ValueError, "non-static call"):
            patch(bytes(raw))

    def test_missing_original_site_rejected(self):
        raw = bytearray(self.original)
        index, _ = self.rows["Battle.Engine.Debug::LogFightMsg100"]
        struct.pack_into("<I", raw, self.sites[0][1] + 1, 0x06000000 | index)
        with self.assertRaisesRegex(ValueError, "sites changed or are incomplete"):
            patch(bytes(raw))

    def test_unrelated_caller_rejected(self):
        raw = bytearray(self.original)
        _, row = self.rows["Battle.Engine.Debug::Log"]
        start = self.pe.get_offset_from_rva(row.Rva)
        body = read_method_body_from_bytes(self.pe.get_data(row.Rva))
        code = b"\x02\x28" + struct.pack("<I", self.source) + b"\x2a"
        self.assertGreaterEqual(body.code_size, len(code))
        raw[start + body.header_size:start + body.header_size + body.code_size] = code + b"\0" * (body.code_size - len(code))
        with self.assertRaisesRegex(ValueError, "Unexpected original combat logging caller"):
            patch(bytes(raw))

    def test_incompatible_parameter_signature_rejected(self):
        raw = bytearray(self.original)
        # dnfile's blob item RVA starts at the compressed length byte; changing
        # object to int keeps metadata shape valid while breaking stack safety.
        offset = self.pe.get_offset_from_rva(self.stub.Signature.rva)
        self.assertEqual(raw[offset:offset + 5], b"\x04" + bytes.fromhex("0001011c"))
        raw[offset + 4] = 0x08
        with self.assertRaisesRegex(ValueError, r"static void\(object\)"):
            patch(bytes(raw))


if __name__ == "__main__":
    unittest.main()
