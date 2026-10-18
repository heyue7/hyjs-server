"""Pure class lifecycle checks for the newer signed-rank/original6/final32 runners."""
import hashlib
import importlib
import json
import os
import unittest
from unittest.mock import patch

import test_p14_managed_mysql_contract as managed


class P14RankFinalRunnerContractTests(unittest.TestCase):
    def setUp(self):
        managed.P14ManagedRunnerContractTests.setUp(self)
        digest = hashlib.sha256(b"mock-only").hexdigest()
        for key, folder in [("HYJS_P14_PROMOTION_MATCH_SOURCE", "hyjs-p14-promotion-rank-contract"),
                            ("HYJS_P14_PROMOTION_MATCH_PREBUILT", "hyjs-p14-rank-prebuilt-contract")]:
            directory = self.base / folder
            directory.mkdir()
            self.environ[key] = str(directory)
            (directory / "manifest.json").write_text(json.dumps({"files": {}, "nativeSHA256": digest}))
            (directory / "hyjs-native").write_bytes(b"mock-only")

    def test_new_three_runners_keep_managed_two_realm_lifecycle_and_no_allocation_leaks(self):
        specs = [("test_p14_promotion_rank_mysql", "P14PromotionRankMySQLTests"),
                 ("test_p14_promotion_stage6_mysql", "P14PromotionStageSixMySQLTests"),
                 ("test_p14_final32_mysql", "P14FinalQualificationNativeMySQLTests")]
        for module, name in specs:
            with self.subTest(runner=module):
                cls = getattr(importlib.import_module(module), name)
                pool = managed.FakePool()
                with patch.dict(os.environ, self.environ), patch("hyjs_managed_mysql.ManagedMySQL", return_value=pool), patch("fcntl.flock"):
                    try:
                        cls.setUpClass()
                        self.assertEqual(len(pool.connections), 2)
                        self.assertTrue(all(database.startswith("hyjs_test_p13_") for database in pool.connections))
                        self.assertEqual(cls.env["HYJS_MYSQL_HOST"], "127.0.0.1:3306")
                        self.assertEqual(cls.env["HYJS_P13_MYSQL_USER"], "hyjs_t_mock")
                        self.assertEqual(cls.clock.read_text(), "0")
                    finally:
                        cls.doClassCleanups()
                    self.assertTrue(pool.closed)
                    self.assertEqual(pool.connections, {})
                    self.assertEqual(cls.tearDown_exceptions, [])


if __name__ == "__main__":
    unittest.main()
