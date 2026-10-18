"""Short real HTTP proof of the exact Root-owned weekly handler after the long chain."""
import secrets
from datetime import datetime
from pathlib import Path
from zoneinfo import ZoneInfo
from test_p13_union_fight_settlement_mysql import P13UnionFightSettlementMySQLTests

class P13WeeklyRankBoundaryMySQLTests(P13UnionFightSettlementMySQLTests):
    test_1_actual_mysql_race_zero_round_and_refused_combat = None
    test_2_original_tcp_http_signup_mine_result_p1_offline_cold = None

    def test_3_exact_root_handler_original_weekly_http_and_cold(self):
        self.clock = Path(self.temp.name) / 'clock'
        self.set_clock(int(datetime(2026, 10, 12, 12, tzinfo=ZoneInfo('Asia/Shanghai')).timestamp() * 1000))
        self.env['HYJS_P13_FIELD_NATIVE_CLOCK'] = str(self.clock)
        self.start()
        uid = 'p13-week-http-' + secrets.token_hex(4)
        login = self.login(uid)
        self.call('CSPlayerCreate', {'playerName': '周榜原边界', 'heroId': 'hero_047'}, login, 1, 'SCPlayerCreate')
        before = self.profile(login)
        self.sql('UPDATE p13_union_fight_policies SET enrollment_enabled=1;')
        for group in range(1, 5):
            reply = self.call('CSUFRankInfo', {'rankType': 1, 'subType': group}, login, 10 + group, 'SCUFRankInfo')
            self.assertEqual((reply.rankType, reply.subType, len(reply.rankIno.list)), (1, group, 0))
        for kind, group, stamp in [(0, 1, 20), (2, 1, 21), (1, 0, 22), (1, 5, 23), (1, 1, 0)]:
            op, _ = self.stamped('CSUFRankInfo', {'rankType': kind, 'subType': group}, login, stamp)
            self.assertEqual(op, 10001)
        self.assertEqual(self.profile(login), before)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME LIKE 'p13_union_fight_weekly_%';").strip(), '3')
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME LIKE 'p13_union_fight_weekly_%';").strip(), 'InnoDB')
        self.stop(); self.start()
        login = self.login(uid)
        reply = self.call('CSUFRankInfo', {'rankType': 1, 'subType': 2}, login, 24, 'SCUFRankInfo')
        self.assertEqual((reply.subType, len(reply.rankIno.list)), (2, 0))
        self.assertEqual(self.profile(login), before)
        print('exact be287 weekly handler HTTP4/enum+stamp rejection/cold/InnoDB3: passed')
