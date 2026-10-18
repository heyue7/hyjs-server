"""原大小炸弹：真实120秒主厂首车/采矿店购/破车/冷恢复，预编译后二进制锁内运行。"""
import concurrent.futures
import hashlib,json,os,subprocess,unittest
from pathlib import Path
import test_p13_union_fight_field_mysql as field
import test_p13_union_fight_gate_item_mysql as gate
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_CAR_ITEM_MYSQL_TEST')=='1','显式启用原大小炸弹专属MySQL')
class P13UnionFightCarItemMySQLTests(gate.P13UnionFightGateItemMySQLTests):
    test_1_mysql_race_actual_two_camp_mine_shop_gate_items_crash_cold_sha=None
    test_2_native_original_two_actual_camp_gate_item_consume_tick_crash_cold=None

    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE'])
        self.assertTrue(prepared.name.startswith('hyjs-p13-prebuilt-'))
        manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        result=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightCarItemActual','-test.count=1','-test.timeout=180s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'));print(result.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p13_union_fight_car_item%';").strip(),'InnoDB')

    def test_2_prebuilt_original_first_cars_actual_bombs_tick_crash_cold(self):
        if os.environ.get('HYJS_P13_ITEM_NATIVE_TEST')!='1':self.skipTest('显式启用原炸弹HTTP/TCP')
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

    def publish_item_sources(self,first):
        rows=[]
        for item,percent in [('ufitem_017',20),('ufitem_018',60)]:rows.append("('2026-10-20','"+item+"',1,"+str(percent)+","+str(first-60000)+",'隔离明确20/60%本地大小炸弹，原数额未知')")
        self.sql('INSERT INTO p13_union_fight_car_item_sources(day,item_id,enabled,hp_percent,published_at,source) VALUES'+','.join(rows)+';')
        self.sql("INSERT INTO p13_union_fight_item_sources(day,item_id,enabled,hp_percent,published_at,source) VALUES('2026-10-20','ufitem_016',1,20,"+str(first-60000)+",'本地20%普通瓶，共CD验证');")
        self.sql('INSERT INTO p13_union_fight_factory_sources(day,factory_id,enabled,chariot_hp,published_at,source) VALUES'+','.join("('2026-10-20',"+str(i)+",1,"+str(1000+i)+","+str(first-60000)+",'隔离当地HP1000+原厂ID，真实原120s首车timer')" for i in [1,2])+';')
        rows=[]
        for item in ['ufitem_017','ufitem_018','ufitem_016']:rows.append("('2026-10-20','"+item+"',1,'"+item+"',10,1,2,0,0,60000,"+str(first-60000)+",'隔离实际矿币报价10/2库存')")
        self.sql('INSERT INTO p13_union_fight_shop_sources(day,shop_item_id,enabled,item_id,price,quantity,buy_count,buy_condition,priority,refresh_millis,published_at,source) VALUES'+','.join(rows)+';')
        self.sql("INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES('2026-10-20','car-left',1,'1',-4.7,-0.12,9.98,50,"+str(first-60000)+",'原矿1/回收点/本地50'),('2026-10-20','car-right',1,'1',80.22,-0.33,-13.36,50,"+str(first-60000)+",'原矿1/回收点/本地50');")

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
        self.assertEqual(self.stamped('CSUFPlayerUseItem',small,a,800)[0],10001)
        self.set_clock(first+119999);self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_factory_states WHERE match_id='"+transfer.unionFightId+"' AND completed_at>0;").strip(),'0')
        self.set_clock(first+120000)
        made={self.receive(sa,'SCUFCariotFactoryBuildComplete').cariot.cariotId for _ in range(2)}
        self.assertEqual(made,{1,2})
        self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[]},a);all_cars=self.receive(sa,'SCUFCaropt')
        self.assertEqual(len(all_cars.cariot),2)
        index=next(i for i,car in enumerate(all_cars.cariot) if car.camp!=camp);target=all_cars.cariot[index];point=all_cars.local[index]
        initial=target.hp;self.assertEqual(initial,1000+target.cariotId)
        self.assertEqual(self.stamped('CSUFPlayerUseItem',small,a,800)[0],10001)
        self.tcp_send(sa,'CSMove',{'pointNew':{'x':point.x,'y':point.y,'z':point.z}},a);self.receive(sa,'SCUFElementMove')
        self.call('CSUFPlayerUseItem',{'itemId':'ufitem_016'},a,801,'SCUFPlayerUseItemDetail');self.assertEqual(self.stamped('CSUFPlayerUseItem',small,a,800)[0],10001)
        self.set_clock(first+125000);self.drain(sa);self.drain(sb);self.drain(sc)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSUFPlayerUseItem',small,a,800,'SCUFPlayerUseItemDetail'),range(8)))
        first_use=replies[0];self.assertTrue(all(reply==first_use for reply in replies))
        self.assertEqual(self.receive(sb,'SCUFPlayerUseItem').itemId,'ufitem_017')
        tick=self.receive(sb,'SCUFElementTickExport');self.assertEqual((tick.targetGuid[0],tick.sourceGuid[0]),(target.guid,a.playerId));self.assertAlmostEqual(tick.hpDelta[0],-initial*.2)
        damaged=self.receive(sb,'SCUFCaropt').cariot[0];self.assertAlmostEqual(damaged.hp,initial*.8)
        self.assertFalse(any(op in [BY_NAME['SCUFCaropt'],BY_NAME['SCUFElementTickExport'],BY_NAME['SCUFCariotCrash']] for op,_ in self.drain(sc)))
        self.sql('UPDATE p13_union_fight_car_item_sources SET enabled=0,hp_percent=1;')
        self.set_clock(first+130000);second=self.call_all('CSUFPlayerUseItem',large,a,802);self.assertAlmostEqual(next(v for op,v in second if op==BY_NAME['SCUFCaropt']).cariot[0].hp,initial*.2)
        self.stop();self.start();a=self.login(uids[selected[0]]);sa=self.connect(a)
        self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');self.receive(sa,'SCUnionFightTransfer');cold=self.receive(sa,'SCUFCaropt');self.assertAlmostEqual(next(car.hp for car in cold.cariot if car.guid==target.guid),initial*.2)
        self.set_clock(first+135000);final=self.call_all('CSUFPlayerUseItem',large,a,803)
        self.assertEqual(next(v for op,v in final if op==BY_NAME['SCUFCaropt']).cariot[0].hp,0)
        self.assertAlmostEqual(next(v for op,v in final if op==BY_NAME['SCUFElementTickExport']).hpDelta[0],-initial*.2)
        self.assertEqual(next(v for op,v in final if op==BY_NAME['SCUFCariotCrash']).guid,target.guid)
        self.set_clock(first+140000);self.assertEqual(self.stamped('CSUFPlayerUseItem',small,a,804)[0],10001)
        self.stop();self.start();a=self.login(uids[selected[0]]);sa=self.connect(a)
        self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');self.receive(sa,'SCUnionFightTransfer');cold=self.receive(sa,'SCUFCaropt');self.assertEqual(next(car.hp for car in cold.cariot if car.guid==target.guid),0)
        self.assertEqual(self.receive(sa,'SCUFCariotCrash').guid,target.guid)
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=0;');self.assertEqual(self.call('CSUFPlayerUseItem',small,a,800,'SCUFPlayerUseItemDetail'),first_use)
        self.set_clock(first+300000);self.assertEqual(self.stamped('CSUFPlayerUseItem',small,a,800)[0],10001)

if __name__=='__main__':unittest.main()
