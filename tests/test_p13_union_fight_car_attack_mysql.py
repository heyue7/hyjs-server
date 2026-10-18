"""实际120秒首车沿原路到敌门：原动画/600ms释放，真实矿币炸弹可在释放前击毁敌车。"""
import hashlib,json,os,subprocess,time,unittest
from pathlib import Path
import test_p13_union_fight_car_item_mysql as car
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_CAR_ATTACK_MYSQL_TEST')=='1','显式启用战车攻门专属MySQL')
class P13UnionFightCarAttackMySQLTests(car.P13UnionFightCarItemMySQLTests):
    test_2_prebuilt_original_first_cars_actual_bombs_tick_crash_cold=None

    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertTrue(prepared.name.startswith('hyjs-p13-prebuilt-'))
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightCar(Attack|Motion|Item)Actual','-test.count=1','-test.timeout=180s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())

    def test_2_prebuilt_original_actual_car_gate_release_bomb_cancel_cold_crash(self):
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

    def publish_item_sources(self,first):
        super().publish_item_sources(first)
        self.sql("UPDATE p13_union_fight_car_item_sources SET hp_percent=100,source='原比例未知/隔离local100%大炸弹取消待释放' WHERE item_id='ufitem_018';")
        self.sql('INSERT INTO p13_union_fight_car_motion_sources(day,factory_id,enabled,speed,published_at,source) VALUES'+','.join("('2026-10-20',"+str(i)+",1,20,"+str(first-60000)+",'原速度未知/local20米秒')" for i in [1,2])+';')
        self.sql('INSERT INTO p13_union_fight_car_attack_sources(day,factory_id,enabled,hp_percent,radius,published_at,source) VALUES'+','.join("('2026-10-20',"+str(i)+",1,70,3.5,"+str(first-60000)+",'原攻击范围/数额未知/local70%+3.5m；原CD3.5/release0.6')" for i in [1,2])+';')

    def wait_sql(self,query,expected):
        deadline=time.monotonic()+10
        while time.monotonic()<deadline:
            if self.sql(query).strip()==str(expected):return
            time.sleep(.05)
        self.fail('实际worker状态未出现: '+query)

    def check_item_native(self,a,b,c,sa,sb,sc,uids,selected,first,transfer):
        camp=transfer.player.camp;recycler,shop=('2','3') if camp==0 else ('5','6')
        recycle={'x':-4.7,'y':-0.12,'z':9.98} if camp==0 else {'x':80.22,'y':-0.33,'z':-13.36}
        shop_point={'x':-16,'y':-0.12,'z':-9} if camp==0 else {'x':87.8,'y':-0.31,'z':-2.48}
        guid=int(self.sql("SELECT guid FROM p13_union_fight_resource_nodes WHERE match_id='"+transfer.unionFightId+"' AND ABS(x-"+str(recycle['x'])+")<0.001;").strip())
        self.set_clock(first+20000);self.tcp_send(sa,'CSMove',{'pointNew':recycle},a);self.receive(sa,'SCUFElementMove')
        self.tcp_send(sa,'CSUFPlayerCollect',{'resGuid':guid},a);started=self.receive(sa,'SCUFPlayerCollect');self.assertIn(a.playerId,started.playerGuid)
        self.set_clock(first+23000);self.assertEqual(self.receive(sa,'SCUFPlayerCollectEndDetail').addResourceNum,50)
        deposited=self.call('CSUFPlayerUpResource',{'npcId':recycler},a,600,'SCUFPlayerUpResource');self.assertEqual(deposited.player.ownToken,150)
        self.set_clock(first+33000);self.tcp_send(sa,'CSMove',{'pointNew':shop_point},a);self.receive(sa,'SCUFElementMove')
        self.assertEqual(len(self.call('CSUFShopList',{'npcId':shop},a,601,'SCUFShopList').items),3)
        for i,item in enumerate(['ufitem_017','ufitem_018','ufitem_016']):
            for n in range(1 if item=='ufitem_016' else 2):bought=self.call('CSUFPlayerBuyItem',{'shopItemId':item,'npcId':shop},a,700+i*2+n,'SCUFPlayerBuyItemDetail')
        self.assertEqual((bought.player.ownToken,len(bought.player.items)),(100,3))
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=1;')
        self.set_clock(first+120000)
        made={self.receive(sa,'SCUFCariotFactoryBuildComplete').cariot.cariotId for _ in range(2)};self.assertEqual(made,{1,2})
        self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[]},a);cars=self.receive(sa,'SCUFCaropt')
        enemy=next(car for car in cars.cariot if car.camp!=camp);friendly=next(car for car in cars.cariot if car.camp==camp)
        enemy_gate_id=6 if friendly.cariotId==1 else 2;own_gate_id=2 if enemy.cariotId==2 else 6
        own_gate_guid=int(self.sql("SELECT guid FROM p13_union_fight_gate_states WHERE match_id='"+transfer.unionFightId+"' AND gate_id="+str(own_gate_id)+';').strip())
        enemy_gate_guid=int(self.sql("SELECT guid FROM p13_union_fight_gate_states WHERE match_id='"+transfer.unionFightId+"' AND gate_id="+str(enemy_gate_id)+';').strip())
        self.set_clock(first+125000)
        attacks={}
        for _ in range(2):
            attack=self.receive(sa,'SCUFCariotAttackGate');attacks[attack.guid]=attack.gateGuid
        self.assertEqual(attacks,{enemy.guid:own_gate_guid,friendly.guid:enemy_gate_guid})
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_gate_states WHERE match_id='"+transfer.unionFightId+"' AND LENGTH(frame)>0;").strip(),'8')
        self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[enemy.guid]},a);position=self.receive(sa,'SCUFCaropt').local[0]
        self.tcp_send(sa,'CSMove',{'pointNew':{'x':position.x,'y':position.y,'z':position.z}},a);self.receive(sa,'SCUFElementMove')
        self.set_clock(first+125599);time.sleep(.1)
        self.assertEqual(self.sql("SELECT SUM(hits) FROM p13_union_fight_car_attack_states WHERE match_id='"+transfer.unionFightId+"';").strip(),'0')
        cancel=self.call_all('CSUFPlayerUseItem',{'itemId':'ufitem_018'},a,801)
        self.assertEqual(next(v for op,v in cancel if op==BY_NAME['SCUFCariotCrash']).guid,enemy.guid)
        self.assertAlmostEqual(next(v for op,v in cancel if op==BY_NAME['SCUFElementTickExport']).hpDelta[0],-enemy.hp)
        self.sql('UPDATE p13_union_fight_car_attack_sources SET enabled=0,hp_percent=1,radius=9; UPDATE p13_union_fight_car_motion_sources SET enabled=0,speed=99;')
        self.stop();self.start();a=self.login(uids[selected[0]]);b=self.login(uids[selected[1]]);c=self.login(uids[selected[2]]);sa=self.connect(a);sb=self.connect(b);sc=self.connect(c)
        for sock,login in [(sa,a),(sb,b),(sc,c)]:self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');self.receive(sock,'SCUnionFightTransfer')
        cold=self.receive(sa,'SCUFCaropt');self.assertEqual(next(car.hp for car in cold.cariot if car.guid==enemy.guid),0)
        messages=[self.receive(sa,'SCUFCariotCrash'),self.receive(sa,'SCUFCariotAttackGate')] if cold.cariot[0].guid==enemy.guid else [self.receive(sa,'SCUFCariotAttackGate'),self.receive(sa,'SCUFCariotCrash')]
        self.assertTrue(any(getattr(value,'guid',0)==friendly.guid and hasattr(value,'gateGuid') for value in messages))
        self.drain(sa);self.drain(sb);self.drain(sc)
        self.set_clock(first+125600);tick=self.receive(sa,'SCUFElementTickExport')
        self.assertEqual((tick.targetGuid[0],tick.sourceGuid[0],tick.hpDelta[0]),(enemy_gate_guid,friendly.guid,-7000))
        door=self.receive(sa,'SCUFGate').gate[0];self.assertEqual((door.id,door.hp),(enemy_gate_id,3000))
        self.set_clock(first+128499);time.sleep(.2);self.assertEqual(self.sql("SELECT SUM(hits) FROM p13_union_fight_car_attack_states WHERE match_id='"+transfer.unionFightId+"';").strip(),'1')
        self.drain(sa);self.set_clock(first+128500);second=self.receive(sa,'SCUFCariotAttackGate');self.assertEqual((second.guid,second.gateGuid),(friendly.guid,enemy_gate_guid))
        self.set_clock(first+129100);tick=self.receive(sa,'SCUFElementTickExport');self.assertEqual((tick.targetGuid[0],tick.sourceGuid[0],tick.hpDelta[0]),(enemy_gate_guid,friendly.guid,-3000));self.assertEqual(self.receive(sa,'SCUFGate').gate[0].hp,0)
        self.assertEqual(self.receive(sa,'SCUFGateCrash').gateId,enemy_gate_id);self.assertEqual(self.receive(sa,'SCUFCariotComplete').guid,friendly.guid)
        self.assertFalse(any((op==BY_NAME['SCUFElementTickExport'] and enemy_gate_guid in value.targetGuid) or (op==BY_NAME['SCUFCariotComplete'] and value.guid==friendly.guid) for op,value in self.drain(sc)))
        self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');buildings=self.receive(sa,'SCUnionFightTransfer');self.assertEqual(next(door.hp for door in buildings.gateInfos if door.id==own_gate_id),10000);self.assertEqual(next(door.hp for door in buildings.gateInfos if door.id==enemy_gate_id),0)
        self.stop();self.start();a=self.login(uids[selected[0]]);sa=self.connect(a);self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');self.receive(sa,'SCUnionFightTransfer');self.receive(sa,'SCUFCaropt');terminal=self.receive(sa,'SCUFCariotComplete');self.assertEqual(terminal.guid,friendly.guid)
        self.set_clock(first+300000);time.sleep(.2);self.assertEqual(self.sql("SELECT SUM(hits) FROM p13_union_fight_car_attack_states WHERE match_id='"+transfer.unionFightId+"';").strip(),'2')

if __name__=='__main__':unittest.main()
