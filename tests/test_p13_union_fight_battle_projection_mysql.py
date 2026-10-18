"""116真实矿币图腾/冷重启场景完整HP输入审计；10%injury显式有限准备，P6authority仍false。"""
import hashlib,json,os,subprocess,unittest
from pathlib import Path
import test_p13_union_fight_war_drum_mysql as drum
import test_p13_union_fight_field_mysql as field

@unittest.skipUnless(os.environ.get('HYJS_P13_BATTLE_PROJECTION_MYSQL_TEST')=='1','显式开启116独立MySQL')
class P13UnionFightBattleProjectionMySQLTests(drum.P13UnionFightWarDrumMySQLTests):
    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightBattleProjectionActual','-test.count=1','-test.timeout=240s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=270)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())

    def check_item_native(self,*args):
        self.projection_prepared=set()
        return super().check_item_native(*args)

    def benefit(self,login,at,expected):
        binary=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE'])/'service-race.test'
        if login.playerId not in self.projection_prepared:
            env=dict(self.env,HYJS_P13_HEAL_AURA_NATIVE_PLAYER=str(login.playerId));p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightHealAuraNativePrepareHP$','-test.count=1'],cwd=field.SOURCE/'hyjs-server/service',env=env,capture_output=True,text=True,timeout=60);self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));self.projection_prepared.add(login.playerId)
        super().benefit(login,at,expected)
        env=dict(self.env,HYJS_P13_PROJECTION_NATIVE_PLAYER=str(login.playerId),HYJS_P13_PROJECTION_NATIVE_AT=str(at))
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightBattleProjectionNativeRead$','-test.count=1'],cwd=field.SOURCE/'hyjs-server/service',env=env,capture_output=True,text=True,timeout=60);self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'))
        line=next(value for value in p.stdout.splitlines() if value.startswith('GUILD_PROJECTION_NATIVE_INPUT '));value=json.loads(line.split(' ',1)[1]);self.assertTrue(value['inputOnly']);self.assertEqual(value['playerId'],login.playerId);self.assertTrue(all(abs(hp-.1)<1e-8 for hp in value['hpFraction']));self.assertTrue(all(abs(mult-expected)<1e-8 for mult in value['multiplier']));self.assertGreater(value['applied'],0)

if __name__=='__main__':unittest.main()
