"""119原中立复活点3m/0.2实际采集店购同场占领；争夺规则明确local提前发布。"""
import hashlib,json,os,subprocess,unittest
from pathlib import Path
import test_p13_union_fight_battle_projection_mysql as projection
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME
@unittest.skipUnless(os.environ.get('HYJS_P13_REVIVE_POINT_MYSQL_TEST')=='1','显式开启119隔离MySQL')
class P13UnionFightRevivePointMySQLTests(projection.P13UnionFightBattleProjectionMySQLTests):
    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightRevivePointActual','-test.count=1','-test.timeout=240s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=270)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())
    def publish_item_sources(self,first):
        super().publish_item_sources(first)
        self.sql("INSERT INTO p13_union_fight_revive_point_sources(day,point_id,enabled,max_step_millis,max_gap_millis,published_at,source) VALUES('2026-10-20',5,1,1000,2000,"+str(first-60000)+",'原争夺/多人后台未知/localcountDiff先中和后推进/0leftred1rightblue/1秒step2秒gap');")
    def check_item_native(self,a,b,*args):
        self.point_actor=a;self.point_enemy=b;self.point_done=False;self.point_uids=args[4];self.point_selected=args[5];self.point_first=args[6]
        super().check_item_native(a,b,*args);self.assertTrue(self.point_done)
    def receive_update(self,sock):
        value=self.receive(sock,'SCUFRevivePointUpdate');self.assertEqual(value.point.id,5);return value
    def benefit(self,login,at,expected):
        super().benefit(login,at,expected)
        if self.point_done or login.playerId!=self.point_actor.playerId or expected!=1.0:return
        self.point_done=True;first=self.point_first;point={'x':37.99,'y':-3.96,'z':-1.29};self.set_clock(first+100100)
        sock=self.connect(login);self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');transfer=self.receive(sock,'SCUnionFightTransfer');camp=transfer.player.camp;match=transfer.unionFightId
        self.drain(sock);self.tcp_send(sock,'CSMove',{'pointNew':point},login);self.receive(sock,'SCUFElementMove')
        observed=self.receive_update(sock);self.assertEqual((observed.point.camp,observed.point.leftOccupyProgress,observed.point.rightOccupyProgress),(2,0,0));self.assertEqual((observed.redPlayerCount,observed.bluePlayercount),(1,0) if camp==0 else (0,1))
        for sec in range(101,106):
            self.set_clock(first+sec*1000+100);update=self.receive_update(sock);self.assertAlmostEqual(update.point.leftOccupyProgress if camp==0 else update.point.rightOccupyProgress,(sec-100)/5)
        occupied=self.receive(sock,'SCUFRevivePointOccupy');self.assertEqual((occupied.id,occupied.camp),(5,camp))
        self.tcp_send(sock,'CSUFRefreshElementArray',{'guid':[update.point.guid]},login);refresh=self.receive(sock,'SCUFRevivePoint');self.assertEqual(refresh.point[0],update.point)
        enemy=self.login(self.point_uids[self.point_selected[1]]);enemy_sock=self.connect(enemy);self.tcp_send(enemy_sock,'CSCurrentPosition',{},enemy);self.receive(enemy_sock,'SCCurrentPosition');self.receive(enemy_sock,'SCUnionFightTransfer');self.drain(enemy_sock)
        self.set_clock(first+118100);self.tcp_send(enemy_sock,'CSMove',{'pointNew':point},enemy);self.set_clock(first+118200);self.receive(enemy_sock,'SCUFElementMove')
        for sec in [119,120]:
            self.drain(sock);self.set_clock(first+sec*1000+200);contested=self.receive_update(sock);self.assertEqual((contested.redPlayerCount,contested.bluePlayercount),(1,1));self.assertAlmostEqual(contested.point.leftOccupyProgress if camp==0 else contested.point.rightOccupyProgress,1)
        self.set_clock(first+123200);self.tcp_send(sock,'CSMove',{'pointNew':dict(point,x=point['x']+10)},login);self.receive(sock,'SCUFElementMove');self.receive_update(sock)
        for sec in range(124,134):
            self.drain(enemy_sock);self.set_clock(first+sec*1000+200);last=self.receive_update(enemy_sock)
        switched=self.receive(enemy_sock,'SCUFRevivePointOccupy');self.assertEqual((switched.id,switched.camp),(5,1-camp));self.assertEqual(last.point.camp,1-camp)
        before=self.sql("SELECT state_hash FROM p13_union_fight_revive_point_states WHERE match_id='"+match+"';")
        self.sql('UPDATE p13_union_fight_revive_point_sources SET enabled=0,max_step_millis=1,max_gap_millis=60000;');self.set_clock(first+150000);self.stop();self.start();login=self.login(self.point_uids[self.point_selected[0]]);sock=self.connect(login)
        self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');cold=self.receive(sock,'SCUnionFightTransfer');saved=next(x for x in cold.revivePointInfos if x.id==5);self.assertEqual(saved.camp,1-camp)
        self.assertAlmostEqual(saved.rightOccupyProgress if camp==0 else saved.leftOccupyProgress,1)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_revive_point_states WHERE match_id='"+match+"';").strip(),'1')
if __name__=='__main__':unittest.main()
