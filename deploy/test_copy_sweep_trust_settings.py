import ast
import tempfile
import unittest
from pathlib import Path

from settings import OPTIONAL_RUNTIME_KEYS, load


class CopySweepTrustSettingsTest(unittest.TestCase):
    def test_realm_opt_in_is_optional_and_strict(self):
        example = (Path(__file__).parent / '.env.example').read_text()
        key = 'HYJS_COPY_SWEEP_P9_COMPLETE_SERVER'
        self.assertIn(key, OPTIONAL_RUNTIME_KEYS)
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'settings.env'
            for value in (None, '0', '1', '2147483647'):
                with self.subTest(value=value):
                    path.write_text(example + ('' if value is None else '\n' + key + '=' + value + '\n'))
                    values = load(path)
                    self.assertEqual(values.get(key), value)
            for value in ('true', '-1', '01', '2147483648'):
                with self.subTest(value=value):
                    path.write_text(example + '\n' + key + '=' + value + '\n')
                    with self.assertRaises(ValueError):
                        load(path)

    def test_activation_accepts_only_valid_realm_override(self):
        source = (Path(__file__).parent / 'activate.py').read_text()
        tree = ast.parse(source)
        loop = next(node for node in tree.body if isinstance(node, ast.For)
                    and "stage / 'deployment.env'" in ast.get_source_segment(source, node))
        code = compile(ast.Module(body=[loop], type_ignores=[]), 'activate.py', 'exec')
        key = 'HYJS_COPY_SWEEP_P9_COMPLETE_SERVER'
        with tempfile.TemporaryDirectory() as folder:
            stage = Path(folder)
            for value in ('0', '1', '2147483647'):
                (stage / 'deployment.env').write_text(key + '=' + value + '\n')
                namespace = {'stage': stage, 'runtime': {}}
                exec(code, namespace)
                self.assertEqual(namespace['runtime'], {key: value})
            for value in ('true', '-1', '01', '2147483648'):
                (stage / 'deployment.env').write_text(key + '=' + value + '\n')
                with self.assertRaises(AssertionError):
                    exec(code, {'stage': stage, 'runtime': {}})


if __name__ == '__main__':
    unittest.main()
