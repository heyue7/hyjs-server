"""Original limited-sign UI requests -> unchanged P4 welfare / once / cold.

The runtime change only removes two incorrect P11 dispatch cases. This suite
uses no finite resource/progression preparation and does not alter welfare rules.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import tempfile
import time
import unittest

from hyjs_managed_mysql import ManagedMySQL
from test_p11_arena_prize_mysql import P11ArenaPrizeMySQLTests as Transport

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P4_LIMIT_SIGN_ROUTING_MYSQL_TEST') == '1',
                     'explicit root-coordinated managed SQL window')
class P4LimitedSignRoutingMySQLTests(unittest.TestCase):
    send = Transport.send
    login = Transport.login
    stamped = Transport.stamped
    call = Transport.call
    profile = Transport.profile
    start = Transport.start
    stop = Transport.stop
    setUp = Transport.setUp
    sql = Transport.sql

    def test_original_list_claim_eight_once_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source = Path(os.environ['HYJS_P4_LIMIT_SIGN_ROUTING_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent, ROOT/'build/candidates')
        manifest = json.loads((ROOT/'analysis/p4/limited-sign-routing-code-manifest.json').read_text())
        hashes = {}
        for row in manifest['runtimeFiles']:
            value = hashlib.sha256((self.source/row['path']).read_bytes()).hexdigest()
            self.assertEqual(value, row['sha256'], row['path'])
            hashes[row['path']] = value
        proof = json.loads((ROOT/'analysis/p4/limited-sign-routing-code-validation.json').read_text())
        self.assertEqual(proof['status'], 'PASS')
        prebuilt = Path(os.environ['HYJS_P4_LIMIT_SIGN_ROUTING_PREBUILT_BINARY']).resolve()
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(), proof['binarySHA256'])
        helper = hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper, '483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        report = dict(status='PENDING', runtimeSHA256=hashes, managedHelperSHA256=helper,
                      productionModified=False, phoneUI=False)
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-limited-sign-native-') as temporary:
            self.binary = Path(temporary)/'limited-sign-native'
            shutil.copy2(prebuilt, self.binary)
            self.managed_pool = ManagedMySQL()
            connection = None
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                try:
                    connection = self.managed_pool.allocate('p4_limit_sign_routing')
                    self.database, self.test_user = connection.database, connection.user
                    self.test_password = connection.password
                    self.env = dict(os.environ, **connection.env(), GOMEMLIMIT='768MiB')
                    self.start()
                    uid = 'original-limited-sign-'+secrets.token_hex(5)
                    actor = self.login(uid)
                    self.call('CSPlayerCreate', {'playerName': '原限时签到', 'heroId': 'hero_043'}, actor, 1, 'SCPlayerCreate')
                    initial = self.profile(actor)
                    self.assertEqual(initial[0].get('coin', 0), 0)
                    listing = self.call('CSRewardLimitSignList', {}, actor, 2, 'SCRewardLimitSignList')
                    data = listing.limitDaySignDatas
                    self.assertEqual((data.signNum, data.allNum, len(data.list)), (0, 7, 7))
                    self.assertEqual((data.list[0].getState, data.list[0].rewardId), (1, 'coin,1000'))
                    self.assertTrue(all(v.getState == 0 for v in data.list[1:]))
                    rule_before = self.sql("SELECT id,HEX(rules),HEX(frozen),source,enabled FROM p4_welfare_rules WHERE kind='limit';")
                    self.assertTrue(rule_before.strip(), 'the original existing local welfare row must be active')
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                        claims = list(pool.map(lambda _: self.call('CSReceiveLimitMonthSign', {}, actor,
                                                                  3, 'SCReceiveLimitMonthSign'), range(8)))
                    self.assertTrue(all(v == claims[0] for v in claims))
                    self.assertEqual([(v.id, v.num) for v in claims[0].get.simps], [('coin', 1000)])
                    after = self.profile(actor)
                    self.assertEqual(after[0].get('coin', 0), 1000)
                    self.assertEqual(after[0]['heroWarId'], initial[0]['heroWarId'])
                    zero = self.call('CSReceiveLimitMonthSign', {}, actor, 4, 'SCReceiveLimitMonthSign')
                    self.assertEqual(list(zero.get.simps), [])
                    current = self.call('CSRewardLimitSignList', {}, actor, 5, 'SCRewardLimitSignList').limitDaySignDatas
                    self.assertEqual((current.signNum, current.allNum), (1, 7))
                    self.assertEqual(current.list[0].getState, 2)
                    self.assertTrue(all(v.getState == 0 for v in current.list[1:]))
                    self.assertEqual(self.sql("SELECT id,HEX(rules),HEX(frozen),source,enabled FROM p4_welfare_rules WHERE kind='limit';"), rule_before)
                    stable = self.profile(actor)
                    self.stop()
                    self.start()
                    actor = self.login(uid)
                    self.assertEqual(self.profile(actor), stable)
                    self.assertEqual(self.call('CSReceiveLimitMonthSign', {}, actor, 3, 'SCReceiveLimitMonthSign'), claims[0])
                    self.assertEqual(self.profile(actor), stable)
                    self.assertEqual(self.call('CSRewardLimitSignList', {}, actor, 6, 'SCRewardLimitSignList').limitDaySignDatas, current)
                    report.update(status='PASS_ORIGINAL_LIMITED_SIGN_LIST_EXISTING_REWARD_ONCE_COLD',
                                  actualReward={'id': 'coin', 'num': 1000}, sameStampWorkers=8,
                                  newStampSameDayZero=True, coldRestarts=1, existingDays=7,
                                  welfareRulesAndProducerUnchanged=True, finitePreparation='none',
                                  noWalletOrItemOrProgressionInjection=True)
                except BaseException as failure:
                    report.update(status='FAIL', failure=self.managed_pool.redact(str(failure)))
                    raise
                finally:
                    try:
                        self.stop()
                    finally:
                        try:
                            if connection is not None:
                                self.managed_pool.release(connection.database)
                        finally:
                            self.managed_pool.close()
                    report['managedAllocationsRemoved'] = True
                    report['seconds'] = round(time.monotonic()-began, 3)
                    (ROOT/'analysis/p4/limited-sign-routing-native-checkpoint.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
        (ROOT/'analysis/p4/limited-sign-routing-mysql-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')


if __name__ == '__main__':
    unittest.main()
