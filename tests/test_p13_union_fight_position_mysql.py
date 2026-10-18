"""实际公会战换场握手：未Current旧Town移动静默忽略、冷库保留、新Room Current后正常移动。"""
import os
import subprocess
import unittest
import test_p13_union_fight_field_mysql as field

@unittest.skipUnless(os.environ.get('HYJS_P13_POSITION_MYSQL_TEST')=='1','显式启用换场专属MySQL')
class P13UnionFightPositionMySQLTests(field.P13UnionFightFieldMySQLTests):
    test_1_mysql_race_actual25_map_legacy_sha_movement_restart_end=None
    test_2_native_original_frames_private_instances_relogin_end_restart=None

    def test_1_mysql_race_old_town_login_confirm_cold_current_then_valid_move(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP13UnionFightPositionLoading','-count=1','-timeout=180s'],cwd=field.SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'))
        print(result.stdout.strip())

    def test_2_native_original_old_tcp_moves_before_current_cold_then_new_moves(self):
        if os.environ.get('HYJS_P13_POSITION_NATIVE_TEST')!='1':self.skipTest('显式启用换场原HTTP/TCP')
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

if __name__=='__main__':unittest.main()
