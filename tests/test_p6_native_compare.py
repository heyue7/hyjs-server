"""Exercise original-log comparison boundaries using one real captured battle."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class NativeCompareBoundaryTests(unittest.TestCase):
    def test_matching_operation_prefix_reports_missing_tail(self):
        source = ROOT / "analysis/p6/native-pvp-auto-priority-v45-20261014"
        expected = ROOT / "analysis/p6/native-old14-expected-v52-tool-regression"
        native_manifest = json.loads((source / "manifest.json").read_text())
        case = native_manifest["cases"][0]
        proof = next(row for row in json.loads((expected / "manifest.json").read_text())["cases"] if row["case"] == case["case"])
        native_file = Path(case["batchWire"]).with_suffix(".native.jsonl").name
        with tempfile.TemporaryDirectory(prefix="hyjs-p6-native-compare-") as directory:
            target = Path(directory)
            os.link(source / native_file, target / native_file)
            (target / "manifest.json").write_text(json.dumps({**native_manifest, "cases": [case]}))
            go = json.loads((expected / proof["goTimeline"]).read_text())
            self.assertGreater(len(go["operates"]["skillOpts"]), 1)
            go["operates"]["skillOpts"].pop()
            altered = json.dumps(go).encode()
            expected_target = target / "expected"
            expected_target.mkdir()
            (expected_target / proof["goTimeline"]).write_bytes(altered)
            os.link(expected / proof["goTicks"], expected_target / proof["goTicks"])
            altered_proof = {**proof, "goTimelineSHA256": hashlib.sha256(altered).hexdigest()}
            (expected_target / "manifest.json").write_text(json.dumps({"cases": [altered_proof]}))
            completed = subprocess.run(["rtk", "proxy", "python3", str(ROOT / "analysis/p6/compare_native_pvp_auto.py"), str(target), "--expected", str(expected_target)], cwd=ROOT, text=True, capture_output=True)
            self.assertEqual(completed.returncode, 0, completed.stderr)
            report = json.loads((target / "comparison.json").read_text())[0]
            self.assertIsNone(report["firstOperationDrift"])
            self.assertEqual(report["sequenceCountDrifts"], {"operations": {"native": report["nativeOperations"], "go": report["nativeOperations"] - 1}})
            self.assertIsNone(report["firstCompletedTickClockDrift"])


if __name__ == "__main__":
    unittest.main()
