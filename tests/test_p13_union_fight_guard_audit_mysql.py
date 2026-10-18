"""118实际原AskFight守卫GUID/Monster/type14审计；具名local模板，战争仍false。"""
import concurrent.futures,hashlib,json,os,subprocess,unittest
from pathlib import Path
import test_p13_union_fight_battle_projection_mysql as projection
import test_p13_union_fight_field_mysql as field

@unittest.skipUnless(os.environ.get('HYJS_P13_GUARD_AUDIT_MYSQL_TEST')=='1','显式开启118独立MySQL')
class P13UnionFightGuardAuditMySQLTests(projection.P13UnionFightBattleProjectionMySQLTests):
    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightGuardAuditActual','-test.count=1','-test.timeout=240s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=270)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())
    def run_helper(self,name,**values):
        env=dict(self.env,**values);binary=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE'])/'service-race.test'
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^'+name+'$','-test.count=1'],cwd=field.SOURCE/'hyjs-server/service',env=env,capture_output=True,text=True,timeout=60)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip());return p.stdout
    def publish_item_sources(self,first):
        super().publish_item_sources(first)
        self.sql("INSERT INTO p13_union_fight_battle_audit_sources(day,enabled,seed_salt,attacker_damage_factor,defender_damage_factor,published_at,source) VALUES('2026-10-20',1,42,0,1,"+str(first-60000)+",'原映射未知/localSalt42 explicit0/1，真实计算审计');")
        self.run_helper('TestP13UnionFightGuardAuditNativePublish',HYJS_P13_GUARD_AUDIT_NATIVE_PUBLISH_AT=str(first-60000))
    def check_item_native(self,a,b,*args):
        self.guard_actor=a;self.guard_done=False;self.guard_uids=args[4];self.guard_selected=args[5];self.guard_first=args[6]
        super().check_item_native(a,b,*args)
        self.assertTrue(self.guard_done)
    def benefit(self,login,at,expected):
        super().benefit(login,at,expected)
        if self.guard_done or login.playerId!=self.guard_actor.playerId or expected!=1.0:return
        self.guard_done=True;first=self.guard_first;guard_id='factory_guard_001' if int(self.sql('SELECT camp FROM p13_union_fight_field_players WHERE player_id='+str(login.playerId)+';').strip())==0 else 'factory_guard_002'
        guard_guid=int(self.sql("SELECT guard_guid FROM p13_union_fight_guard_audit_offers WHERE guard_id='"+guard_id+"' AND match_id=(SELECT match_id FROM p13_union_fight_field_players WHERE player_id="+str(login.playerId)+");").strip())
        point={'x':63,'y':0,'z':-29} if guard_id=='factory_guard_001' else {'x':17,'y':0,'z':25}
        self.set_clock(first+100000);sock=self.connect(login);self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');self.receive(sock,'SCUnionFightTransfer');self.drain(sock)
        self.tcp_send(sock,'CSMove',{'pointNew':point},login);self.receive(sock,'SCUFElementMove')
        before=self.sql('SELECT frame_hash,battle_hash FROM p13_union_fight_field_players WHERE player_id='+str(login.playerId)+';');field_before=self.sql('SELECT snapshot_hash FROM p13_union_fight_fields WHERE match_id=(SELECT match_id FROM p13_union_fight_field_players WHERE player_id='+str(login.playerId)+');')
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.stamped('CSUFPlayerAskFight',{'targetGuid':guard_guid},login,950)[0],range(8)))
        self.assertEqual(replies,[10001]*8);self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_guard_audit_attempts WHERE player_id='+str(login.playerId)+';').strip(),'1')
        self.assertEqual(before,self.sql('SELECT frame_hash,battle_hash FROM p13_union_fight_field_players WHERE player_id='+str(login.playerId)+';'));self.assertEqual(field_before,self.sql('SELECT snapshot_hash FROM p13_union_fight_fields WHERE match_id=(SELECT match_id FROM p13_union_fight_field_players WHERE player_id='+str(login.playerId)+');'))
        self.assertEqual(self.sql("SELECT r.status,r.authoritative FROM p6_battle_records r JOIN p13_union_fight_guard_audit_attempts a ON a.p6_record_id=r.id WHERE a.player_id="+str(login.playerId)+";").strip(),'computed\t0')
        self.sql('UPDATE p13_union_fight_guard_audit_sources SET enabled=0,monster=0xff;')
        export_root=Path(os.environ.get('HYJS_P13_GUARD_AUDIT_EXPORT_ROOT',str(Path.cwd()/'analysis/p13/actual-battle14-20261015')))
        self.run_helper('TestP13UnionFightGuardAuditNativeExport',HYJS_P13_GUARD_AUDIT_NATIVE_PLAYER=str(login.playerId),HYJS_P13_GUARD_AUDIT_NATIVE_STAMP='950',HYJS_P13_GUARD_AUDIT_EXPORT_ROOT=str(export_root))
        self.stop();self.start();fresh=self.login(self.guard_uids[self.guard_selected[0]])
        self.assertEqual(self.stamped('CSUFPlayerAskFight',{'targetGuid':guard_guid},fresh,950)[0],10001);self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_guard_audit_attempts WHERE player_id='+str(fresh.playerId)+';').strip(),'1')

if __name__=='__main__':unittest.main()
