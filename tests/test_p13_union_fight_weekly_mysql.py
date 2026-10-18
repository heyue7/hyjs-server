"""Two actual timed rounds, original weekly ranks, and Sunday offline rewards."""
import subprocess
from test_p13_union_fight_settlement_mysql import P13UnionFightSettlementMySQLTests, SOURCE

class P13UnionFightWeeklyMySQLTests(P13UnionFightSettlementMySQLTests):
    transport_mvp = True
    natural_missions = True
    weekly_rewards = True

    def test_1_actual_mysql_race_zero_round_and_refused_combat(self):
        result = subprocess.run(['rtk', 'proxy', str(self.prebuilt / 'service-race.test'),
                                 '-test.v', '-test.run', '^TestP13WeeklyActual(TwoTimed|RefusedCombat)', '-test.count=1', '-test.timeout=180s'],
                                cwd=SOURCE / 'hyjs-server/service', env=self.env,
                                capture_output=True, text=True, timeout=200)
        self.assertEqual(result.returncode, 0, (result.stdout + result.stderr).replace(self.test_password, '[redacted]'))
        self.assertIn('--- PASS: TestP13WeeklyActualTwoTimedRoundsSundayOfflineCold', result.stdout)
        self.assertIn('--- PASS: TestP13WeeklyActualRefusedCombatKeepsWeekUnsettled', result.stdout)
        self.assertNotIn('no tests to run', result.stdout + result.stderr)
        print(result.stdout.strip())

    def test_2_original_tcp_http_signup_mine_result_p1_offline_cold(self):
        super().test_2_original_tcp_http_signup_mine_result_p1_offline_cold()
