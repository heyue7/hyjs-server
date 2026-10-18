"""120真实原AskFight PVE同TX冻结报名combat tables/精确引擎与client，不产生新金样本。"""
import hashlib,json,os,subprocess,unittest
from pathlib import Path
import test_p13_union_fight_guard_audit_mysql as guard
import test_p13_union_fight_field_mysql as field
@unittest.skipUnless(os.environ.get('HYJS_P13_BATTLE_FROZEN_MYSQL_TEST')=='1','显式开启120隔离MySQL')
class P13UnionFightBattleFrozenMySQLTests(guard.P13UnionFightGuardAuditMySQLTests):
    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFight(BattleFrozenOriginal|BattleAuditActual|GuardAuditActual)','-test.count=1','-test.timeout=300s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=330)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())
    def run_helper(self,name,**values):
        if name=='TestP13UnionFightGuardAuditNativeExport':
            return super().run_helper('TestP13UnionFightBattleFrozenNativeRead',HYJS_P13_FROZEN_NATIVE_PLAYER=values['HYJS_P13_GUARD_AUDIT_NATIVE_PLAYER'])
        return super().run_helper(name,**values)
if __name__=='__main__':unittest.main()
