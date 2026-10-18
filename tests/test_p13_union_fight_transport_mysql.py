"""Actual original TCP0 deposits, unique transport MVP and the original P1 inbox."""
import subprocess

from test_p13_union_fight_settlement_mysql import P13UnionFightSettlementMySQLTests, SOURCE


class P13UnionFightTransportMySQLTests(P13UnionFightSettlementMySQLTests):
    transport_mvp = True

    def test_1_actual_mysql_race_zero_round_and_refused_combat(self):
        result = subprocess.run(['rtk', 'proxy', str(self.prebuilt / 'service-race.test'),
                                 '-test.run', '^TestP13Transport', '-test.count=1', '-test.timeout=180s'],
                                cwd=SOURCE / 'hyjs-server/service', env=self.env,
                                capture_output=True, text=True, timeout=200)
        self.assertEqual(result.returncode, 0, (result.stdout + result.stderr).replace(self.test_password, '[redacted]'))
        print(result.stdout.strip())

    def test_2_original_tcp_http_signup_mine_result_p1_offline_cold(self):
        super().test_2_original_tcp_http_signup_mine_result_p1_offline_cold()
