"""原013真实团队法阵：自然矿币店购+明确有限10%受伤准备，真实HP伤害producer待P6。"""
import hashlib,json,os,subprocess,time,unittest
from pathlib import Path
import test_p13_union_fight_car_item_mysql as car
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_HEAL_AURA_MYSQL_TEST')=='1','显式启用原法阵专属MySQL')
class P13UnionFightHealAuraMySQLTests(car.P13UnionFightCarItemMySQLTests):
    test_2_prebuilt_original_first_cars_actual_bombs_tick_crash_cold=None

    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertTrue(prepared.name.startswith('hyjs-p13-prebuilt-'));self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightHealAuraActual','-test.count=1','-test.timeout=180s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())

    def test_2_prebuilt_original_actual_aura_team_ticks_cold_frozen_and_off(self):
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

    def publish_item_sources(self,first):
        self.sql("INSERT INTO p13_union_fight_heal_aura_sources(day,item_id,enabled,hp_percent,radius,tick_millis,authority_mask,published_at,source) VALUES('2026-10-20','ufitem_013',1,20,3,1000,4,"+str(first-60000)+",'原数字/radius/tick/UseCondition2未知/local20%/3m/1s/Chair2资格');")
        self.sql("INSERT INTO p13_union_fight_item_sources(day,item_id,enabled,hp_percent,published_at,source) VALUES('2026-10-20','ufitem_011',1,20,"+str(first-60000)+",'本地20%共diamondCD');")
        self.sql('INSERT INTO p13_union_fight_shop_sources(day,shop_item_id,enabled,item_id,price,quantity,buy_count,buy_condition,priority,refresh_millis,published_at,source) VALUES'+','.join("('2026-10-20','"+item+"',1,'"+item+"',10,1,3,0,0,60000,"+str(first-60000)+",'真实矿币local报价10/1')" for item in ['ufitem_013','ufitem_011'])+';')
        self.sql("INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES('2026-10-20','heal-left',1,'1',-4.7,-0.12,9.98,50,"+str(first-60000)+",'原矿1/回收点/local50'),('2026-10-20','heal-right',1,'1',80.22,-0.33,-13.36,50,"+str(first-60000)+",'原矿1/回收点/local50');")

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
        for i,item in enumerate(['ufitem_013','ufitem_011']):
            for n in range(1 if item=='ufitem_011' else 2):bought=self.call('CSUFPlayerBuyItem',{'shopItemId':item,'npcId':shop},a,700+i*2+n,'SCUFPlayerBuyItemDetail')
        self.assertEqual((bought.player.ownToken,len(bought.player.items)),(120,2))
        ally,ally_sock=self.field_friend,self.field_friend_sock
        for sock,login in [(ally_sock,ally),(sb,b),(sc,c)]:self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');self.receive(sock,'SCUnionFightTransfer')
        near=dict(shop_point,x=shop_point['x']+2)
        self.tcp_send(ally_sock,'CSMove',{'pointNew':near},ally);self.receive(ally_sock,'SCUFElementMove')
        self.tcp_send(sb,'CSMove',{'pointNew':shop_point},b);self.receive(sb,'SCUFElementMove')
        # 明确有限10%受伤准备；每个真实角色已有原冻结maxHP，库存/矿币/战争结果不改。
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);binary=prepared/'service-race.test'
        for login in [a,ally,b]:
            env=dict(self.env,HYJS_P13_HEAL_AURA_NATIVE_PLAYER=str(login.playerId));p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightHealAuraNativePrepareHP$','-test.count=1'],cwd=field.SOURCE/'hyjs-server/service',env=env,capture_output=True,text=True,timeout=60);self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'))
        self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');initial=self.receive(sa,'SCUnionFightTransfer').player
        maxima=list(initial.fightInfo.maxHpPos)
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=1;');self.drain(sa);self.drain(ally_sock);self.drain(sb);self.drain(sc)
        used=self.call_all('CSUFPlayerUseItem',{'itemId':'ufitem_013'},a,800);on=next(value for op,value in used if op==BY_NAME['SCUFElementBuffOn']);self.assertEqual(list(on.targetGuid),[a.playerId]);self.assertEqual((on.buff[0].buffId,on.buff[0].mountGuid),('uf_buff_009',a.playerId));buff_guid=on.buff[0].guid
        self.assertEqual(self.stamped('CSUFPlayerUseItem',{'itemId':'ufitem_011'},a,801)[0],10001)
        self.set_clock(first+33999);time.sleep(.2);self.assertEqual(self.sql("SELECT next_tick_at FROM p13_union_fight_heal_aura_effects WHERE match_id='"+transfer.unionFightId+"';").strip(),str(first+34000))
        self.set_clock(first+34000);tick=self.receive(sa,'SCUFElementTickExport');self.assertEqual(set(tick.targetGuid),{a.playerId,ally.playerId});self.assertEqual(set(tick.sourceGuid),{a.playerId});frame=self.receive(sa,'SCUFPlayer');caster=next(player for player in frame.player if player.guid==a.playerId)
        for hp,maxhp in zip(caster.fightInfo.hpPos,maxima):self.assertAlmostEqual(hp,maxhp*.3)
        self.assertFalse(any(op==BY_NAME['SCUFElementTickExport'] for op,_ in self.drain(sc)))
        self.sql('UPDATE p13_union_fight_heal_aura_sources SET enabled=0,hp_percent=1,radius=1,tick_millis=5000,authority_mask=1;')
        self.stop();self.start();a=self.login(uids[selected[0]]);ally=self.login(uids[selected[0]+6]);b=self.login(uids[selected[1]]);sa=self.connect(a);ally_sock=self.connect(ally);sb=self.connect(b)
        for sock,login in [(sa,a),(ally_sock,ally),(sb,b)]:self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');cold=self.receive(sock,'SCUnionFightTransfer');self.assertEqual(len(cold.player.buffs),1 if login.playerId==a.playerId else 0)
        self.set_clock(first+35000);tick=self.receive(sa,'SCUFElementTickExport');self.assertEqual(set(tick.targetGuid),{a.playerId,ally.playerId});frame=self.receive(sa,'SCUFPlayer')
        for player in frame.player:
            for hp,maxhp in zip(player.fightInfo.hpPos,player.fightInfo.maxHpPos):self.assertAlmostEqual(hp,maxhp*.5)
        self.drain(ally_sock);self.tcp_send(ally_sock,'CSMove',{'pointNew':dict(near,x=near['x']+5)},ally);self.receive(ally_sock,'SCUFElementMove')
        for at,percent in [(36000,.7),(37000,.9),(38000,1.0)]:
            self.set_clock(first+at);tick=self.receive(sa,'SCUFElementTickExport');self.assertEqual(list(tick.targetGuid),[a.playerId]);frame=self.receive(sa,'SCUFPlayer').player[0]
            for hp,maxhp in zip(frame.fightInfo.hpPos,maxima):self.assertAlmostEqual(hp,maxhp*percent)
        self.assertEqual(list(self.receive(sa,'SCUFElementBuffOff').buffGuid),[buff_guid])
        for sock,login,expected in [(sa,a,1.0),(ally_sock,ally,.5),(sb,b,.1)]:
            self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');frame=self.receive(sock,'SCUnionFightTransfer').player
            for hp,maxhp in zip(frame.fightInfo.hpPos,frame.fightInfo.maxHpPos):self.assertAlmostEqual(hp,maxhp*expected)
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=0;');self.assertEqual(self.call('CSUFPlayerUseItem',{'itemId':'ufitem_013'},a,800,'SCUFPlayerUseItemDetail'),next(value for op,value in used if op==BY_NAME['SCUFPlayerUseItemDetail']))
        self.set_clock(first+300000);self.assertEqual(self.stamped('CSUFPlayerUseItem',{'itemId':'ufitem_013'},a,800)[0],10001)

if __name__=='__main__':unittest.main()
