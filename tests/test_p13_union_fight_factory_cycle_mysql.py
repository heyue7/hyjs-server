"""原120秒连续车：真实首车与第二代独立GUID/矿币炸弹/冷热/独立运动攻击；defaultoff具名产能。"""
import hashlib,json,os,subprocess,time,unittest
from pathlib import Path
import test_p13_union_fight_car_motion_mysql as motion
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_FACTORY_CYCLE_MYSQL_TEST')=='1','显式启用115专属MySQL')
class P13UnionFightFactoryCycleMySQLTests(motion.P13UnionFightCarMotionMySQLTests):
    test_2_prebuilt_original_moving_cars_actual_bombs_cold_and_dead_stop=None

    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightFactoryCycleActual','-test.count=1','-test.timeout=240s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=270)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())

    def test_2_original_two_generations_actual_mine_bombs_motion_attack_cold_scope(self):
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

    def publish_item_sources(self,first):
        super().publish_item_sources(first)
        self.sql('INSERT INTO p13_union_fight_factory_cycle_sources(day,factory_id,enabled,max_generations,max_alive,published_at,source) VALUES'+','.join("('2026-10-20',"+str(i)+",1,2,2,"+str(first-60000)+",'原产能未知/local2代2存活，120秒原周期')" for i in [1,2])+';')
        self.sql('INSERT INTO p13_union_fight_car_attack_sources(day,factory_id,enabled,hp_percent,radius,published_at,source) VALUES'+','.join("('2026-10-20',"+str(i)+",1,1,3.5,"+str(first-60000)+",'原数额/范围未知/local1%/3.5米')" for i in [1,2])+';')

    def wait_sql(self,query,want):
        deadline=time.monotonic()+12
        while time.monotonic()<deadline:
            if self.sql(query).strip()==str(want):return
            time.sleep(.05)
        self.fail('actual producer did not reach expected isolated state '+str(want))

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
        small={'itemId':'ufitem_017'};large={'itemId':'ufitem_018'}
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=1;')
        self.set_clock(first+120000);made={self.receive(sa,'SCUFCariotFactoryBuildComplete').cariot.cariotId for _ in range(2)};self.assertEqual(made,{1,2})
        self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[]},a);initial=self.receive(sa,'SCUFCaropt');self.assertEqual(len(initial.cariot),2);old_guids={car.guid for car in initial.cariot};old_target=next(car for car in initial.cariot if car.camp!=camp)
        self.motion_at(transfer.unionFightId,first+130000)
        for at,item,stamp,hp in [(130000,small,800,.8),(135000,large,801,.2),(140000,large,802,0)]:
            self.motion_at(transfer.unionFightId,first+at);self.drain(sa);self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[old_target.guid]},a);point=self.receive(sa,'SCUFCaropt').local[0]
            self.tcp_send(sa,'CSMove',{'pointNew':{'x':point.x,'y':point.y,'z':point.z}},a);self.receive(sa,'SCUFElementMove');used=self.call_all('CSUFPlayerUseItem',item,a,stamp);self.assertAlmostEqual(next(value for op,value in used if op==BY_NAME['SCUFCaropt']).cariot[0].hp,old_target.maxHp*hp)
            if hp==0:self.assertEqual(next(value for op,value in used if op==BY_NAME['SCUFCariotCrash']).guid,old_target.guid)
        dead_point=point
        self.sql('UPDATE p13_union_fight_factory_cycle_sources SET enabled=0,max_generations=99,max_alive=1;');self.set_clock(first+239999);time.sleep(.2);self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_car_generations WHERE match_id='"+transfer.unionFightId+"';").strip(),'0')
        self.drain(sa);self.set_clock(first+240000);new=[self.receive(sa,'SCUFCariotFactoryBuildComplete') for _ in range(2)];self.assertEqual({value.cariot.cariotId for value in new},{1,2});self.assertFalse({value.cariot.guid for value in new}&old_guids)
        for value in new:self.assertEqual(value.factoryGuid,value.cariot.factoryGuid)
        self.wait_sql("SELECT COUNT(*) FROM p13_union_fight_car_generations WHERE match_id='"+transfer.unionFightId+"';",2)
        self.drain(sa);self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[]},a);all_cars=self.receive(sa,'SCUFCaropt');self.assertEqual(len(all_cars.cariot),4);self.assertEqual(self.receive(sa,'SCUFCariotCrash').guid,old_target.guid)
        idx=next(i for i,car in enumerate(all_cars.cariot) if car.camp!=camp and car.guid not in old_guids);target=all_cars.cariot[idx];point=all_cars.local[idx]
        self.tcp_send(sa,'CSMove',{'pointNew':{'x':point.x,'y':point.y,'z':point.z}},a);self.receive(sa,'SCUFElementMove');used=self.call_all('CSUFPlayerUseItem',small,a,803);self.assertAlmostEqual(next(value for op,value in used if op==BY_NAME['SCUFCaropt']).cariot[0].hp,target.maxHp*.8)
        self.stop();self.start();a=self.login(uids[selected[0]]);sa=self.connect(a);self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');self.receive(sa,'SCUnionFightTransfer');cold=self.receive(sa,'SCUFCaropt');self.assertEqual(len(cold.cariot),4);self.assertAlmostEqual(next(car for car in cold.cariot if car.guid==target.guid).hp,target.maxHp*.8);self.assertEqual(self.receive(sa,'SCUFCariotCrash').guid,old_target.guid)
        self.set_clock(first+250000);self.wait_sql("SELECT COUNT(*) FROM p13_union_fight_car_motion_states AS s JOIN p13_union_fight_car_generations AS c ON c.match_id=s.match_id AND c.car_guid=s.car_guid WHERE c.match_id='"+transfer.unionFightId+"' AND s.last_at="+str(first+250000)+";",2)
        self.drain(sa);self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[target.guid,old_target.guid]},a);moving=self.receive(sa,'SCUFCaropt');old_idx=next(i for i,car in enumerate(moving.cariot) if car.guid==old_target.guid);new_idx=next(i for i,car in enumerate(moving.cariot) if car.guid==target.guid);self.assertEqual(moving.local[old_idx],dead_point);self.assertGreater(((moving.local[new_idx].x-point.x)**2+(moving.local[new_idx].z-point.z)**2)**.5,15)
        self.drain(sa);self.set_clock(first+290000)
        self.wait_sql("SELECT COUNT(*) FROM p13_union_fight_car_attack_states AS s JOIN p13_union_fight_car_generations AS c ON c.match_id=s.match_id AND c.car_guid=s.car_guid WHERE c.match_id='"+transfer.unionFightId+"' AND s.pending_release_at="+str(first+290600)+";",2)
        self.set_clock(first+290600);self.wait_sql("SELECT COUNT(*) FROM p13_union_fight_car_attack_states AS s JOIN p13_union_fight_car_generations AS c ON c.match_id=s.match_id AND c.car_guid=s.car_guid WHERE c.match_id='"+transfer.unionFightId+"' AND s.hits=1;",2)
        seen=set()
        for _ in range(12):
            tick=self.receive(sa,'SCUFElementTickExport');seen.update(tick.sourceGuid)
            if target.guid in seen:break
        else:self.fail('new actual generation original attack sourceGUID missing')
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_car_generations WHERE match_id='"+transfer.unionFightId+"';").strip(),'2')
        self.set_clock(first+300000);self.assertEqual(self.stamped('CSUFPlayerUseItem',small,a,804)[0],10001)

if __name__=='__main__':unittest.main()
