"""原015实际冲锋：自然矿币店购，近友3米/敌拒、5秒BuffOn/Off、跨过期积分与cold。"""
import concurrent.futures,hashlib,json,os,subprocess,time,unittest
from pathlib import Path
import test_p13_union_fight_car_item_mysql as car
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_SPEED_ITEM_MYSQL_TEST')=='1','显式启用原015冲锋专属MySQL')
class P13UnionFightSpeedItemMySQLTests(car.P13UnionFightCarItemMySQLTests):
    test_2_prebuilt_original_first_cars_actual_bombs_tick_crash_cold=None

    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertTrue(prepared.name.startswith('hyjs-p13-prebuilt-'))
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightSpeedItemActual','-test.count=1','-test.timeout=180s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())

    def test_2_prebuilt_original_mine_shop_charge_friendly_buff_move_cold_expiry(self):
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

    def publish_item_sources(self,first):
        self.sql("INSERT INTO p13_union_fight_speed_item_sources(day,item_id,enabled,multiplier,authority_mask,published_at,source) VALUES('2026-10-20','ufitem_015',1,2,4,"+str(first-60000)+",'原倍率/UseCondition1未知/local2倍和frozen-currentChair2资格');")
        self.sql("INSERT INTO p13_union_fight_item_sources(day,item_id,enabled,hp_percent,published_at,source) VALUES('2026-10-20','ufitem_011',1,20,"+str(first-60000)+",'本地20%钻石类瓶共CD');")
        self.sql('INSERT INTO p13_union_fight_shop_sources(day,shop_item_id,enabled,item_id,price,quantity,buy_count,buy_condition,priority,refresh_millis,published_at,source) VALUES'+','.join("('2026-10-20','"+item+"',1,'"+item+"',10,1,3,0,0,60000,"+str(first-60000)+",'真实矿币当地报价10/1库存')" for item in ['ufitem_015','ufitem_011'])+';')
        self.sql("INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES('2026-10-20','speed-left',1,'1',-4.7,-0.12,9.98,50,"+str(first-60000)+",'原矿1/回收点/local50'),('2026-10-20','speed-right',1,'1',80.22,-0.33,-13.36,50,"+str(first-60000)+",'原矿1/回收点/local50');")

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
        self.assertEqual(len(self.call('CSUFShopList',{'npcId':shop},a,601,'SCUFShopList').items),2)
        for i,item in enumerate(['ufitem_015','ufitem_011']):
            for n in range(1 if item=='ufitem_011' else 2):bought=self.call('CSUFPlayerBuyItem',{'shopItemId':item,'npcId':shop},a,700+i*2+n,'SCUFPlayerBuyItemDetail')
        self.assertEqual((bought.player.ownToken,len(bought.player.items)),(120,2))
        ally,ally_sock=self.field_friend,self.field_friend_sock
        for sock,login in [(ally_sock,ally),(sb,b),(sc,c)]:self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');self.receive(sock,'SCUnionFightTransfer')
        near=dict(shop_point,x=shop_point['x']+2)
        self.tcp_send(ally_sock,'CSMove',{'pointNew':near},ally);self.receive(ally_sock,'SCUFElementMove')
        self.tcp_send(sb,'CSMove',{'pointNew':shop_point},b);self.receive(sb,'SCUFElementMove')
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=1;')
        self.drain(sa);self.drain(ally_sock);self.drain(sb);self.drain(sc)
        request={'itemId':'ufitem_015'}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSUFPlayerUseItem',request,a,800,'SCUFPlayerUseItemDetail'),range(8)))
        first_use=replies[0];self.assertTrue(all(value==first_use for value in replies));self.assertEqual(first_use.player.items[0].leftUseNum,1)
        on=self.receive(ally_sock,'SCUFElementBuffOn');self.assertEqual(set(on.targetGuid),{a.playerId,ally.playerId});self.assertEqual({buff.buffId for buff in on.buff},{'uf_buff_008'});self.assertEqual({buff.mountGuid for buff in on.buff},{a.playerId,ally.playerId})
        self.assertFalse(any(op==BY_NAME['SCUFElementBuffOn'] for op,_ in self.drain(sc)))
        self.assertEqual(self.stamped('CSUFPlayerUseItem',{'itemId':'ufitem_011'},a,801)[0],10001)
        self.set_clock(first+34000);self.drain(sa);self.drain(ally_sock);self.drain(sb)
        positions={a.playerId:dict(shop_point,x=shop_point['x']+10),ally.playerId:dict(near,x=near['x']+10)}
        for login,sock in [(a,sa),(ally,ally_sock)]:
            self.tcp_send(sock,'CSMove',{'pointNew':positions[login.playerId]},login)
            for _ in range(10):
                moved=self.receive(sock,'SCUFElementMove')
                if login.playerId in moved.guid:break
            else:self.fail('原移动没有当前玩家GUID')
            self.assertEqual(moved.speed[list(moved.guid).index(login.playerId)],10)
        # 未选中的敌方不能从相同3米位置借到倍速，即使也收到了本场视觉广播。
        self.drain(sb)
        self.tcp_send(sb,'CSMove',{'pointNew':positions[a.playerId]},b);self.assertEqual(self.recv(sb)[0],10001)
        self.sql('UPDATE p13_union_fight_speed_item_sources SET enabled=0,multiplier=9,authority_mask=1;')
        self.stop();self.start();a=self.login(uids[selected[0]]);sa=self.connect(a);self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');cold=self.receive(sa,'SCUnionFightTransfer');self.assertEqual(len(cold.player.buffs),1)
        self.drain(sa);self.set_clock(first+38800)
        off_guids={self.receive(sa,'SCUFElementBuffOff').buffGuid[0] for _ in range(2)}
        self.assertEqual(off_guids,{buff.guid for buff in on.buff})
        finish=dict(positions[a.playerId],x=positions[a.playerId]['x']+44)
        self.tcp_send(sa,'CSMove',{'pointNew':finish},a);moved=self.receive(sa,'SCUFElementMove');self.assertEqual(moved.speed[0],5)
        # 4秒实际加速+0.8秒普通速度允许44米；Source热改9倍不能扩大下一段许可。
        self.set_clock(first+39000);self.tcp_send(sa,'CSMove',{'pointNew':dict(finish,x=finish['x']+10)},a);self.assertEqual(self.recv(sa)[0],10001)
        self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');expired=self.receive(sa,'SCUnionFightTransfer');self.assertEqual(len(expired.player.buffs),0)
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=0;');self.assertEqual(self.call('CSUFPlayerUseItem',request,a,800,'SCUFPlayerUseItemDetail'),first_use)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_speed_item_effects WHERE match_id='"+transfer.unionFightId+"';").strip(),'2')
        self.set_clock(first+300000);self.assertEqual(self.stamped('CSUFPlayerUseItem',request,a,800)[0],10001)

if __name__=='__main__':unittest.main()
