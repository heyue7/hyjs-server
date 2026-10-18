"""公会战集结：真实12角色/6会、同会授权120秒原帧/并发回执/持久恢复。"""
import os
import subprocess
import unittest
import test_p13_union_fight_field_mysql as field

@unittest.skipUnless(os.environ.get('HYJS_P13_FOCUS_MYSQL_TEST')=='1','显式启用集结专属MySQL')
class P13UnionFightFocusMySQLTests(field.P13UnionFightFieldMySQLTests):
    test_1_mysql_race_actual25_map_legacy_sha_movement_restart_end=None
    test_2_native_original_frames_private_instances_relogin_end_restart=None

    def test_1_mysql_race_actual_chairman_vice_frozen_scope_cooldown_and_recovery(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP13UnionFightFocusActual','-count=1','-timeout=180s'],cwd=field.SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'))
        print(result.stdout.strip())
        self.assertEqual(self.sql("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME='p13_union_fight_focus';").strip(),'InnoDB')

    def test_2_native_original_private_command_exact_deadline_8concurrent_restart_expiry(self):
        if os.environ.get('HYJS_P13_FOCUS_NATIVE_TEST')!='1':self.skipTest('显式启用集结原HTTP/TCP')
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

if __name__=='__main__':unittest.main()
