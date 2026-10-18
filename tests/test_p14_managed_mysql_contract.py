"""Pure setup/cleanup compatibility checks for the managed multi-realm runners."""
import hashlib
import importlib
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from hyjs_managed_mysql import ManagedConnection


class FakePool:
    def __init__(self):
        self.connections = {}
        self.closed = False

    def allocate(self, label):
        name = 'hyjs_test_' + label + '_mock' + str(len(self.connections))
        connection = ManagedConnection(name, 'hyjs_t_mock', 'mock-secret-only')
        self.connections[name] = connection
        return connection

    def close(self):
        self.closed = True
        self.connections.clear()


class P14ManagedRunnerContractTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='hyjs-managed-p14-mock-')
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        digest = hashlib.sha256(b'mock-only').hexdigest()
        meta = {'files': {}, 'binaries': {}, 'nativeSHA256': digest, 'raceSHA256': digest, 'native_sha256': digest, 'race_sha256': digest}
        self.environ = {}
        folders = {
            'HYJS_P14_QUALIFICATION_FIXED_SOURCE': 'hyjs-p14-season-qualification-contract',
            'HYJS_P14_QUALIFICATION_PREBUILT': 'hyjs-p14-qualification-prebuilt-contract',
            'HYJS_P14_PROMOTION_FIXED_SOURCE': 'hyjs-p14-season-qualification-promotion-contract',
            'HYJS_P14_PROMOTION_PREBUILT': 'hyjs-p14-qualification-prebuilt-promotion',
            'HYJS_P14_ENGINE_UPGRADE_PREBUILT': 'hyjs-p14-upgrade-prebuilt-contract',
            'HYJS_P14_QUALIFICATION_SOURCE': 'hyjs-p14-qualification-domain-contract',
            'HYJS_P14_FORMATION_SOURCE': 'hyjs-p13-natural-missions-contract',
            'HYJS_P14_PRAYER_SOURCE_ROOT': 'hyjs-p13-prayer-contract',
            'HYJS_P10_PROFILE_REALM_SOURCE': 'hyjs-p10-profile-realm-contract',
            'HYJS_P14_PROMOTION_MATCH_SOURCE': 'hyjs-p14-promotion-match-contract',
            'HYJS_P14_PROMOTION_MATCH_PREBUILT': 'hyjs-p14-match-prebuilt-contract',
        }
        for key, folder in folders.items():
            path = self.base / folder
            path.mkdir()
            self.environ[key] = str(path)
            for destination in [path, path / 'prebuilt']:
                destination.mkdir(exist_ok=True)
                (destination / 'manifest.json').write_text(json.dumps(meta))
                for name in ['hyjs-native', 'service-race.test']:
                    (destination / name).write_bytes(b'mock-only')

    def test_all_twelve_class_setups_use_pool_shared_user3306_and_complete_cleanup(self):
        specs = [
            ('test_p13_mysql', 'P13MySQLTests', 1),
            ('test_p14_cross_mysql', 'P14CrossMySQLTests', 3),
            ('test_p14_qualification_mysql', 'P14QualificationMySQLTests', 2),
            ('test_p14_qualification_native_mysql', 'P14QualificationNativeMySQLTests', 4),
            ('test_p14_engine_upgrade_mysql', 'P14EngineUpgradeMySQLTests', 1),
            ('test_p14_promotion_mysql', 'P14PromotionMySQLTests', 2),
            ('test_p14_promotion_formation_mysql', 'P14PromotionFormationMySQLTests', 3),
            ('test_p14_auto_recovery_mysql', 'P14AutoRecoveryMySQLTests', 3),
            ('test_p14_prayer_typed17_source_mysql', 'P14PrayerTyped17SourceMySQLTests', 2),
            ('test_p10_profile_realm_mysql', 'P10ProfileRealmMySQLTests', 2),
            ('test_p13_team_realm_mysql', 'P13TeamRealmMySQLTests', 2),
            ('test_p14_promotion_match_mysql', 'P14PromotionMatchMySQLTests', 2),
        ]
        for module, name, count in specs:
            with self.subTest(runner=module):
                cls = getattr(importlib.import_module(module), name)
                pool = FakePool()
                with patch.dict(os.environ, self.environ), patch('hyjs_managed_mysql.ManagedMySQL', return_value=pool), patch('fcntl.flock'):
                    try:
                        cls.setUpClass()
                        self.assertEqual(len(pool.connections), count)
                        self.assertTrue(all(database.startswith('hyjs_test_p13_') for database in pool.connections))
                        self.assertEqual(cls.env['HYJS_MYSQL_HOST'], '127.0.0.1:3306')
                        self.assertEqual(cls.env['HYJS_P13_MYSQL_USER'], 'hyjs_t_mock')
                        self.assertIn(cls.database, pool.connections)
                        if hasattr(cls, 'realms'):
                            self.assertTrue(set(cls.realms.values()).issubset(pool.connections))
                    finally:
                        cls.doClassCleanups()
                    self.assertTrue(pool.closed)
                    self.assertEqual(pool.connections, {})
                    self.assertEqual(cls.tearDown_exceptions, [])


if __name__ == '__main__':
    unittest.main()
