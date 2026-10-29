import os
import unittest
import zipfile
from pathlib import Path

import build


class BreachTextTests(unittest.TestCase):
    def test_exact_single_cell(self):
        script = "label\r\nid\ttext\r\ntype\r\nkey\r\nfields\r\nbreak_through_desc\t" + build.BEFORE + "\r\nother\tunchanged\r\n"
        self.assertEqual(build.change_cell(script), script.replace(build.BEFORE, build.AFTER))
        with self.assertRaises(ValueError):
            build.change_cell(script.replace(build.BEFORE, "unexpected"))
        with self.assertRaises(ValueError):
            build.change_cell(script + "break_through_desc\t" + build.BEFORE + "\r\n")

    @unittest.skipUnless(os.getenv("HYJS_BREACH84_BASE_APK"), "reference APK not specified")
    def test_merge_preserves_prior_table_changes(self):
        base = Path(os.environ["HYJS_BREACH84_BASE_APK"])
        out = Path(os.environ["HYJS_BREACH84_REFERENCE_OVERLAY"])
        with zipfile.ZipFile(base) as apk:
            env, _, text = build.asset(apk.read(build.ENTRY))
        original = build.script(text)
        lines = original.splitlines(keepends=True)
        columns = lines[1].rstrip("\r\n").split("\t")
        key_index, value_index = columns.index("id"), columns.index("text")
        for i in range(5, len(lines)):
            body = lines[i].rstrip("\r\n")
            cells = body.split("\t")
            if len(cells) == len(columns) and cells[key_index] != build.TARGET and cells[value_index]:
                cells[value_index] += "_prior_overlay_probe"
                lines[i] = "\t".join(cells) + lines[i][len(body):]
                break
        else:
            self.fail("no unrelated table cell to probe")
        prior = "".join(lines)
        tree = text.read_typetree()
        tree["m_Script"] = prior
        text.save_typetree(tree)
        payload = {build.ENTRY: env.file.save(packer="lz4")}
        pins = {}
        report = build.apply_overlay(base, out, payload, pins)
        _, _, patched = build.asset(payload[build.ENTRY])
        self.assertEqual(build.script(patched), build.change_cell(prior))
        self.assertTrue(report["proof"]["otherRowsAndCellsExact"])
        self.assertIn(out / "manifest.json", pins)


if __name__ == "__main__":
    unittest.main()
