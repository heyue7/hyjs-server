"""117实际原AskFight捕获/计算提交，非权威拒绝保留；冻结type14原机只读输入导出。"""
import concurrent.futures,hashlib,json,os,subprocess,unittest
from pathlib import Path
import test_p13_union_fight_battle_projection_mysql as projection
import test_p13_union_fight_field_mysql as field

@unittest.skipUnless(os.environ.get('HYJS_P13_BATTLE_AUDIT_MYSQL_TEST')=='1','显式开启117隔离MySQL')
class P13UnionFightBattleAuditMySQLTests(projection.P13UnionFightBattleProjectionMySQLTests):
    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightBattleAuditActual','-test.count=1','-test.timeout=240s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=270)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())

    def publish_item_sources(self,first):
        super().publish_item_sources(first)
        self.sql("INSERT INTO p13_union_fight_battle_audit_sources(day,enabled,seed_salt,attacker_damage_factor,defender_damage_factor,published_at,source) VALUES('2026-10-20',1,42,0,1,"+str(first-60000)+",'原种子/倍率映射未知/localSalt42 explicit0/1，仅实际审计');")

    def check_item_native(self,a,b,*args):
        self.audit_a=a;self.audit_enemy=b;self.audit_done=False
        super().check_item_native(a,b,*args)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_battle_audit_attempts WHERE player_id="+str(a.playerId)+";").strip(),'1')

    def benefit(self,login,at,expected):
        super().benefit(login,at,expected)
        if not self.audit_done and login.playerId==self.audit_a.playerId:
            self.audit_done=True;actor=self.audit_a;enemy=self.audit_enemy
            before=self.sql('SELECT player_id,frame_hash,battle_hash FROM p13_union_fight_field_players WHERE player_id IN ('+str(actor.playerId)+','+str(enemy.playerId)+') ORDER BY player_id;')
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.stamped('CSUFPlayerAskFight',{'targetGuid':enemy.playerId},actor,900)[0],range(8)))
            self.assertEqual(replies,[10001]*8);self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_battle_audit_attempts WHERE player_id='+str(actor.playerId)+';').strip(),'1')
            self.assertEqual(before,self.sql('SELECT player_id,frame_hash,battle_hash FROM p13_union_fight_field_players WHERE player_id IN ('+str(actor.playerId)+','+str(enemy.playerId)+') ORDER BY player_id;'))
            self.assertEqual(self.sql("SELECT r.status,r.authoritative FROM p6_battle_records r JOIN p13_union_fight_battle_audit_attempts a ON a.p6_record_id=r.id WHERE r.source='p13_uf_audit' AND a.player_id="+str(actor.playerId)+";").strip(),'computed\t0')
            self.sql('UPDATE p13_union_fight_battle_audit_sources SET enabled=0,seed_salt=99,attacker_damage_factor=99;')
            export_root=Path(os.environ.get('HYJS_P13_BATTLE_AUDIT_EXPORT_ROOT',str(Path.cwd()/'analysis/p13/actual-battle14-20261014')))
            binary=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE'])/'service-race.test';env=dict(self.env,HYJS_P13_BATTLE_AUDIT_NATIVE_PLAYER=str(actor.playerId),HYJS_P13_BATTLE_AUDIT_NATIVE_STAMP='900',HYJS_P13_BATTLE_AUDIT_EXPORT_ROOT=str(export_root))
            p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightBattleAuditNativeExport$','-test.count=1'],cwd=field.SOURCE/'hyjs-server/service',env=env,capture_output=True,text=True,timeout=60);self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())
        elif self.audit_done and login.playerId==self.audit_a.playerId:
            self.assertEqual(self.stamped('CSUFPlayerAskFight',{'targetGuid':self.audit_enemy.playerId},login,900)[0],10001)
            self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_battle_audit_attempts WHERE player_id='+str(login.playerId)+';').strip(),'1')

if __name__=='__main__':unittest.main()
