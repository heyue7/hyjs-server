"""原014实际图腾：矿币店购/原CreateDestroy/cold寿命/真实空间受益，数值及模型明确local。"""
import concurrent.futures,hashlib,json,os,subprocess,time,unittest
from pathlib import Path
import test_p13_union_fight_car_item_mysql as car
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_WAR_DRUM_MYSQL_TEST')=='1','显式启用原战鼓专属MySQL')
class P13UnionFightWarDrumMySQLTests(car.P13UnionFightCarItemMySQLTests):
    test_2_prebuilt_original_first_cars_actual_bombs_tick_crash_cold=None

    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertTrue(prepared.name.startswith('hyjs-p13-prebuilt-'));self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightWarDrumActual','-test.count=1','-test.timeout=180s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())

    def test_2_prebuilt_original_mine_shop_totem_benefit_cold_expiry_exit(self):
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

    def publish_item_sources(self,first):
        self.sql("INSERT INTO p13_union_fight_war_drum_sources(day,item_id,enabled,attack_percent,radius,duration_millis,authority_mask,model_name,model_scale,published_at,source) VALUES('2026-10-20','ufitem_014',1,20,3,10000,4,'particles/03_buff/buff_jingshendun',1,"+str(first-60000)+",'原totem1模型/数字/UseCondition2未知/local已核shield-prefab/Atk20%/3m/10s/Chair2');")
        self.sql("INSERT INTO p13_union_fight_item_sources(day,item_id,enabled,hp_percent,published_at,source) VALUES('2026-10-20','ufitem_011',1,20,"+str(first-60000)+",'local20共diamondCD');")
        self.sql('INSERT INTO p13_union_fight_shop_sources(day,shop_item_id,enabled,item_id,price,quantity,buy_count,buy_condition,priority,refresh_millis,published_at,source) VALUES'+','.join("('2026-10-20','"+item+"',1,'"+item+"',10,1,3,0,0,60000,"+str(first-60000)+",'自然矿币local10买1')" for item in ['ufitem_014','ufitem_011'])+';')
        self.sql("INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES('2026-10-20','drum-left',1,'1',-4.7,-0.12,9.98,50,"+str(first-60000)+",'原矿1/回收点/local50'),('2026-10-20','drum-right',1,'1',80.22,-0.33,-13.36,50,"+str(first-60000)+",'原矿1/回收点/local50');")

    def benefit(self,login,at,expected):
        binary=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE'])/'service-race.test';env=dict(self.env,HYJS_P13_WAR_DRUM_NATIVE_PLAYER=str(login.playerId),HYJS_P13_WAR_DRUM_NATIVE_AT=str(at))
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightWarDrumNativeBenefit$','-test.count=1'],cwd=field.SOURCE/'hyjs-server/service',env=env,capture_output=True,text=True,timeout=60);self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'))
        line=next(value for value in p.stdout.splitlines() if value.startswith('WAR_DRUM_NATIVE_BENEFIT '));value=json.loads(line.split(' ',1)[1]);self.assertEqual(value['playerId'],login.playerId);self.assertTrue(value['currentHPUnchanged']);self.assertTrue(all(abs(amount-expected)<1e-8 for amount in value['multiplier']));self.assertEqual(value['applied']>0,expected>1)

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
        for i,item in enumerate(['ufitem_014','ufitem_011']):
            for n in range(1 if item=='ufitem_011' else 2):bought=self.call('CSUFPlayerBuyItem',{'shopItemId':item,'npcId':shop},a,700+i*2+n,'SCUFPlayerBuyItemDetail')
        self.assertEqual((bought.player.ownToken,len(bought.player.items)),(120,2))
        ally,ally_sock=self.field_friend,self.field_friend_sock
        for sock,login in [(ally_sock,ally),(sb,b),(sc,c)]:self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');self.receive(sock,'SCUnionFightTransfer')
        near=dict(shop_point,x=shop_point['x']+2)
        self.tcp_send(ally_sock,'CSMove',{'pointNew':near},ally);self.receive(ally_sock,'SCUFElementMove');self.tcp_send(sb,'CSMove',{'pointNew':shop_point},b);self.receive(sb,'SCUFElementMove')
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=1;');self.drain(sa);self.drain(ally_sock);self.drain(sb);self.drain(sc)
        request={'itemId':'ufitem_014'}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSUFPlayerUseItem',request,a,800,'SCUFPlayerUseItemDetail'),range(8)))
        first_use=replies[0];self.assertTrue(all(value==first_use for value in replies))
        created=self.receive(ally_sock,'SCUFTotemCreate');self.assertEqual((created.totem.totemId,created.totem.modelName,created.totem.modelScale,created.totem.lastTime,created.totem.sourcePlayerGuid),('1','particles/03_buff/buff_jingshendun',1,10,a.playerId));[self.assertAlmostEqual(value,shop_point[key],places=5) for key,value in zip(['x','y','z'],[created.pos.x,created.pos.y,created.pos.z])]
        self.assertFalse(any(op==BY_NAME['SCUFTotemCreate'] for op,_ in self.drain(sc)))
        for login,multiplier in [(a,1.2),(ally,1.2),(b,1.0)]:self.benefit(login,first+33000,multiplier)
        self.assertEqual(self.stamped('CSUFPlayerUseItem',{'itemId':'ufitem_011'},a,801)[0],10001)
        self.sql('UPDATE p13_union_fight_war_drum_sources SET enabled=0,attack_percent=1,radius=1,duration_millis=1000,model_scale=2,authority_mask=1;')
        self.set_clock(first+36000);self.stop();self.start();a=self.login(uids[selected[0]]);ally=self.login(uids[selected[0]+6]);b=self.login(uids[selected[1]]);sa=self.connect(a);ally_sock=self.connect(ally);sb=self.connect(b)
        for sock,login in [(sa,a),(ally_sock,ally),(sb,b)]:self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');self.receive(sock,'SCUnionFightTransfer')
        cold=self.receive(sa,'SCUFTotemCreate');self.assertEqual((cold.totem.guid,cold.totem.lastTime,cold.totem.modelScale),(created.totem.guid,7,1));self.assertEqual(cold.pos,created.pos)
        self.benefit(a,first+36000,1.2)
        self.set_clock(first+38000);self.drain(sa);second=self.call_all('CSUFPlayerUseItem',request,a,802);new=next(value for op,value in second if op==BY_NAME['SCUFTotemCreate']);self.assertNotEqual(new.totem.guid,created.totem.guid);self.benefit(a,first+38000,1.2)
        self.drain(ally_sock);self.tcp_send(ally_sock,'CSMove',{'pointNew':dict(near,x=near['x']+10)},ally);self.receive(ally_sock,'SCUFElementMove');self.benefit(ally,first+38000,1.0)
        self.drain(sa);self.set_clock(first+43000);self.assertEqual(self.receive(sa,'SCUFTotemDestroy').guid,created.totem.guid)
        self.tcp_send(sa,'CSUFRefreshElementArray',{'guid':[new.totem.guid]},a);remaining=self.receive(sa,'SCUFTotemCreate');self.assertEqual(remaining.totem.lastTime,5)
        left=self.call_all('CSTransferScene',{'target':'town_main'},a,803);self.assertEqual(next(value for op,value in left if op==BY_NAME['SCUFTotemDestroy']).guid,new.totem.guid)
        self.call('CSTransferScene',{'target':'fight_union'},a,804,'SCTransferScene');self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');self.receive(sa,'SCUnionFightTransfer');self.benefit(a,first+43000,1.0)
        self.set_clock(first+300000);self.assertEqual(self.stamped('CSUFPlayerUseItem',request,a,800)[0],10001)

if __name__=='__main__':unittest.main()
