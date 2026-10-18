"""实际首车沿原路线行进，实际矿币购买的炸弹使用同一持久位置。"""
import hashlib,json,os,subprocess,time,unittest
from pathlib import Path
import test_p13_union_fight_car_item_mysql as car
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_CAR_MOTION_MYSQL_TEST')=='1','显式启用原战车行进专属MySQL')
class P13UnionFightCarMotionMySQLTests(car.P13UnionFightCarItemMySQLTests):
    test_2_prebuilt_original_first_cars_actual_bombs_tick_crash_cold=None

    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertTrue(prepared.name.startswith('hyjs-p13-prebuilt-'))
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightCar(Motion|Item)Actual','-test.count=1','-test.timeout=180s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())

    def test_2_prebuilt_original_moving_cars_actual_bombs_cold_and_dead_stop(self):
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

    def publish_item_sources(self,first):
        super().publish_item_sources(first)
        self.sql('INSERT INTO p13_union_fight_car_motion_sources(day,factory_id,enabled,speed,published_at,source) VALUES'+','.join("('2026-10-20',"+str(i)+",1,2,"+str(first-60000)+",'原速度未知/隔离local2米秒/完整原寻路')" for i in [1,2])+';')

    def motion_at(self,match,at):
        self.set_clock(at)
        deadline=time.monotonic()+10
        while time.monotonic()<deadline:
            count=self.sql("SELECT COUNT(*) FROM p13_union_fight_car_motion_states WHERE match_id='"+match+"' AND last_at="+str(at)+";").strip()
            if count=='2':return
            time.sleep(.05)
        self.fail('实际车worker未推进当前时钟')

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
        self.set_clock(first+120000)
        made={self.receive(sa,'SCUFCariotFactoryBuildComplete').cariot.cariotId for _ in range(2)};self.assertEqual(made,{1,2})
        self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[]},a);born=self.receive(sa,'SCUFCaropt')
        index=next(i for i,value in enumerate(born.cariot) if value.camp!=camp);target=born.cariot[index];birth=born.local[index];initial=target.hp
        self.motion_at(transfer.unionFightId,first+130000)
        for _ in range(10):
            actual_move=self.receive(sb,'SCUFElementMove')
            if target.guid in actual_move.guid:break
        else:self.fail('原TCP没有本场目标战车移动GUID')
        car_index=list(actual_move.guid).index(target.guid);self.assertEqual(actual_move.speed[car_index],2)
        self.drain(sa);self.drain(sc)
        self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[target.guid]},a);moving=self.receive(sa,'SCUFCaropt').local[0]
        self.assertEqual(actual_move.pos[car_index],moving)
        self.assertGreater(((moving.x-birth.x)**2+(moving.y-birth.y)**2+(moving.z-birth.z)**2)**.5,15)
        self.tcp_send(sa,'CSMove',{'pointNew':{'x':birth.x,'y':birth.y,'z':birth.z}},a);self.receive(sa,'SCUFElementMove')
        self.assertEqual(self.stamped('CSUFPlayerUseItem',small,a,800)[0],10001)
        self.sql('UPDATE p13_union_fight_car_motion_sources SET enabled=0,speed=99;')
        for offset,item,stamp,expected in [(136000,small,801,.8),(141000,large,802,.2),(146000,large,803,0)]:
            self.motion_at(transfer.unionFightId,first+offset)
            self.drain(sa);self.drain(sb);self.drain(sc)
            self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[target.guid]},a);current=self.receive(sa,'SCUFCaropt').local[0]
            self.tcp_send(sa,'CSMove',{'pointNew':{'x':current.x,'y':current.y,'z':current.z}},a);self.receive(sa,'SCUFElementMove')
            used=self.call_all('CSUFPlayerUseItem',item,a,stamp)
            self.assertAlmostEqual(next(v for op,v in used if op==BY_NAME['SCUFCaropt']).cariot[0].hp,initial*expected)
            tick=self.receive(sb,'SCUFElementTickExport');self.assertEqual(tick.targetGuid[0],target.guid)
            if expected==0:self.assertEqual(next(v for op,v in used if op==BY_NAME['SCUFCariotCrash']).guid,target.guid)
            if offset==136000:
                self.stop();self.start();a=self.login(uids[selected[0]]);b=self.login(uids[selected[1]]);c=self.login(uids[selected[2]])
                sa=self.connect(a);sb=self.connect(b);sc=self.connect(c)
                for sock,login in [(sa,a),(sb,b),(sc,c)]:self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');self.receive(sock,'SCUnionFightTransfer')
                cold=self.receive(sa,'SCUFCaropt');idx=next(i for i,value in enumerate(cold.cariot) if value.guid==target.guid);self.assertAlmostEqual(cold.cariot[idx].hp,initial*.8);self.assertEqual(cold.local[idx],current)
        final_point=current
        self.set_clock(first+153000);time.sleep(.4)
        self.drain(sa);self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[target.guid]},a);dead=self.receive(sa,'SCUFCaropt');self.assertEqual(dead.local[0],final_point);self.assertEqual(dead.cariot[0].hp,0);self.assertEqual(self.receive(sa,'SCUFCariotCrash').guid,target.guid)
        self.assertEqual(self.stamped('CSUFPlayerUseItem',small,a,804)[0],10001)
        self.assertFalse(any(op in [BY_NAME['SCUFElementTickExport'],BY_NAME['SCUFCariotCrash']] for op,_ in self.drain(sc)))
        self.stop();self.start();a=self.login(uids[selected[0]]);sa=self.connect(a);self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');self.receive(sa,'SCUnionFightTransfer');cold=self.receive(sa,'SCUFCaropt');idx=next(i for i,value in enumerate(cold.cariot) if value.guid==target.guid);self.assertEqual(cold.local[idx],final_point);self.assertEqual(self.receive(sa,'SCUFCariotCrash').guid,target.guid)
        self.set_clock(first+300000);self.assertEqual(self.stamped('CSUFPlayerUseItem',small,a,805)[0],10001)

if __name__=='__main__':unittest.main()
