"""Local orchestration checks; no SSH, Docker or database is opened."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('growth75_activation', HERE / 'growth75_activation.py')
growth75 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(growth75)


def snapshot():
    form = {'mode': 'limited-shop', 'source': '成长投放V12', 'options': {}}
    row = dict(id='occ-proof', template_id='template', mode='limited-shop', timing='calendar', start_day=1,
               duration_days=14, form_json=json.dumps(form, ensure_ascii=False), enabled=1, archived=0, consumer_key='old-shop')
    return 'admin_activity_occurrences\t' + json.dumps(row, ensure_ascii=False) + '\n'


class Growth75ActivationTests(unittest.TestCase):
    def fixture(self, directory, fail=False):
        events = []
        def mysql(sql):
            if sql == 'SELECT maintenance FROM client_update_policy WHERE id=1;': return '1\n'
            if sql.startswith('SELECT id,maintenance'): return '1\t1\t2026102604\t0\t0\t1.0.74\n'
            if sql.startswith('SELECT id,time'): return '321\t1793026861377\tactivity.schedule.save\t1\trealm=1 revision=10 slots=820\n'
            if sql.startswith('SELECT revision FROM'): return '10\n'
            if sql.startswith('SELECT table_name,engine'): return 'admin_activity_occurrences\tInnoDB\nadmin_audits\tInnoDB\nadmin_activity_schedule_revisions\tInnoDB\n'
            if 'START TRANSACTION;' in sql:
                events.append('cas')
                self.assertTrue(sql.startswith('SET NAMES utf8mb4;'))
                self.assertIn('CHECK(ok=1)', sql)
                self.assertIn('WHERE server_id=1 AND revision=10', sql)
                self.assertLess(sql.index('growth75.schedule.revision'), sql.index('COMMIT;'))
                if fail: raise RuntimeError('stale CAS')
                return '1\t1\n'
            if sql.startswith('SET NAMES utf8mb4;'):
                events.append('export')
                return snapshot()
            if sql.startswith('SELECT maintenance,latest_build'): return '1\t2026102702\t2026102702\t2026102702\t1.0.75\n'
            raise AssertionError(sql)
        def compose(*args):
            events.append('candidate')
            self.assertIn('--apply-growth75', args)
            self.assertNotIn('--apply-growth74', args)
        return events, mysql, compose

    def test_candidate_precedes_fresh_export_and_atomic_cas(self):
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            events, mysql, compose = self.fixture(directory)
            out = growth75.activate_growth75(mysql, compose, HERE / 'growth75_activity_patch.py', directory, 'a'*40, {'file':'verified.aesgcm','sha256':'b'*64})
            self.assertEqual(events, ['candidate','export','cas'])
            manifest = Path(out['privateManifest'])
            self.assertEqual(out['state'], 'applied-maintenance')
            self.assertEqual(json.loads(manifest.read_text())['changeCount'], 1)
            self.assertEqual(manifest.stat().st_mode & 0o777, 0o600)
            self.assertEqual(manifest.parent.stat().st_mode & 0o777, 0o700)
            for path in manifest.parent.iterdir(): self.assertEqual(path.stat().st_mode & 0o777, 0o600)

    def test_stale_cas_preserves_failed_private_manifest_and_does_not_complete(self):
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            events, mysql, compose = self.fixture(directory, fail=True)
            with self.assertRaisesRegex(RuntimeError, 'stale CAS'):
                growth75.activate_growth75(mysql, compose, HERE / 'growth75_activity_patch.py', directory, 'a'*40, {'file':'verified.aesgcm','sha256':'b'*64})
            records = list(directory.glob('growth75-*/manifest.json'))
            self.assertEqual(len(records), 1)
            record = json.loads(records[0].read_text())
            self.assertEqual(record['state'], 'failed-maintenance')
            self.assertEqual(events, ['candidate','export','cas'])
            self.assertIn('Prior candidate CLI', record['recovery'])

    def test_open_game_refuses_before_candidate_or_snapshot(self):
        with tempfile.TemporaryDirectory() as tmp:
            events, _, compose = self.fixture(Path(tmp))
            with self.assertRaises(AssertionError):
                growth75.activate_growth75(lambda query: '0\n', compose, HERE / 'growth75_activity_patch.py', Path(tmp), 'a'*40, {'file':'verified.aesgcm','sha256':'b'*64})
            self.assertEqual(events, [])
            self.assertEqual(list(Path(tmp).iterdir()), [])

    def test_missing_verified_backup_refuses_before_candidate(self):
        with tempfile.TemporaryDirectory() as tmp:
            events, mysql, compose = self.fixture(Path(tmp))
            with self.assertRaises(AssertionError):
                growth75.activate_growth75(mysql, compose, HERE / 'growth75_activity_patch.py', Path(tmp), 'a'*40, None)
            self.assertEqual(events, [])


if __name__ == '__main__': unittest.main()
