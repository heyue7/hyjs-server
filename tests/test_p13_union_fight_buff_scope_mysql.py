"""114原事务退场/快速重入撤销；自然矿币库存+真实协议，恢复仅有限10%injury。"""
import concurrent.futures,hashlib,json,os,subprocess,time,unittest
from pathlib import Path
import test_p13_union_fight_speed_item_mysql as speed
import test_p13_union_fight_heal_aura_mysql as aura
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_BUFF_SCOPE_MYSQL_TEST')=='1','显式开启114隔离原协议')
class P13UnionFightBuffScopeSpeedMySQLTests(speed.P13UnionFightSpeedItemMySQLTests):
    test_2_prebuilt_original_mine_shop_charge_friendly_buff_move_cold_expiry=None

    def test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha(self):
        prepared=Path(os.environ['HYJS_P13_FIELD_PREBUILT_NATIVE']);manifest=json.loads((prepared/'manifest.json').read_text());binary=prepared/'service-race.test'
        self.assertEqual(hashlib.sha256(binary.read_bytes()).hexdigest(),manifest['race_sha256'])
        for path,digest in manifest['files'].items():self.assertEqual(hashlib.sha256((field.SOURCE/path).read_bytes()).hexdigest(),digest)
        p=subprocess.run(['rtk','proxy',str(binary),'-test.run','^TestP13UnionFightBuffScope','-test.count=1','-test.timeout=300s'],cwd=field.SOURCE/'hyjs-server/service',env=self.env,capture_output=True,text=True,timeout=330)
        self.assertEqual(p.returncode,0,(p.stdout+p.stderr).replace(self.test_password,'[redacted]'));print(p.stdout.strip())

    def test_2_original_speed_exit_reentry_peer_off_and_partial_integral(self):
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

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
        ally=self.login(uids[selected[0]+6]);ally_sock=self.connect(ally);self.tcp_send(ally_sock,'CSCurrentPosition',{},ally);self.receive(ally_sock,'SCCurrentPosition');self.receive(ally_sock,'SCUnionFightTransfer')
        self.set_clock(first+35000);self.drain(sa);self.drain(ally_sock)
        left=self.call_all('CSTransferScene',{'target':'town_main'},a,803);off=next(value for op,value in left if op==BY_NAME['SCUFElementBuffOff']);self.assertEqual(set(off.buffGuid),{buff.guid for buff in on.buff});self.assertEqual(set(self.receive(ally_sock,'SCUFElementBuffOff').buffGuid),set(off.buffGuid))
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_speed_item_effects WHERE ended_at="+str(first+35000)+";").strip(),'2')
        self.call('CSTransferScene',{'target':'fight_union'},a,804,'SCTransferScene');self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');self.assertEqual(len(self.receive(sa,'SCUnionFightTransfer').player.buffs),0)
        self.set_clock(first+37000);finish=dict(positions[ally.playerId],x=positions[ally.playerId]['x']+20)
        self.tcp_send(ally_sock,'CSMove',{'pointNew':finish},ally)
        for _ in range(10):
            moved=self.receive(ally_sock,'SCUFElementMove')
            if ally.playerId in moved.guid:break
        else:self.fail('peer actual move GUID missing')
        self.assertEqual(moved.speed[list(moved.guid).index(ally.playerId)],5)
        self.set_clock(first+37200);self.drain(ally_sock);self.tcp_send(ally_sock,'CSMove',{'pointNew':dict(finish,x=finish['x']+10)},ally);self.assertEqual(self.recv(ally_sock)[0],10001)
        self.stop();self.start();a=self.login(uids[selected[0]]);sa=self.connect(a);self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');self.assertEqual(len(self.receive(sa,'SCUnionFightTransfer').player.buffs),0)

@unittest.skipUnless(os.environ.get('HYJS_P13_BUFF_SCOPE_MYSQL_TEST')=='1','显式开启114隔离原协议')
class P13UnionFightBuffScopeAuraMySQLTests(aura.P13UnionFightHealAuraMySQLTests):
    test_1_prebuilt_race_actual_first_cars_mine_shop_bombs_cold_sha=None
    test_2_prebuilt_original_actual_aura_team_ticks_cold_frozen_and_off=None

    def test_2_original_aura_exit_reentry_cold_and_no_postexit_heal(self):
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

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
        self.drain(sa);self.drain(ally_sock)
        left=self.call_all('CSTransferScene',{'target':'town_main'},a,803);self.assertEqual(list(next(value for op,value in left if op==BY_NAME['SCUFElementBuffOff']).buffGuid),[buff_guid]);self.assertEqual(list(self.receive(ally_sock,'SCUFElementBuffOff').buffGuid),[buff_guid])
        self.call('CSTransferScene',{'target':'fight_union'},a,804,'SCTransferScene');self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');self.assertEqual(len(self.receive(sa,'SCUnionFightTransfer').player.buffs),0)
        self.sql('UPDATE p13_union_fight_heal_aura_sources SET enabled=0,hp_percent=99;');self.stop();self.start();ally=self.login(uids[selected[0]+6]);ally_sock=self.connect(ally);self.tcp_send(ally_sock,'CSCurrentPosition',{},ally);self.receive(ally_sock,'SCCurrentPosition');cold=self.receive(ally_sock,'SCUnionFightTransfer').player
        for hp,maxhp in zip(cold.fightInfo.hpPos,cold.fightInfo.maxHpPos):self.assertAlmostEqual(hp,maxhp*.3)
        self.set_clock(first+36000);time.sleep(.3);self.tcp_send(ally_sock,'CSCurrentPosition',{},ally);self.receive(ally_sock,'SCCurrentPosition');after=self.receive(ally_sock,'SCUnionFightTransfer').player
        self.assertEqual(list(after.fightInfo.hpPos),list(cold.fightInfo.hpPos));self.assertEqual(self.sql("SELECT ended_at FROM p13_union_fight_heal_aura_effects;").strip(),str(first+34000))

if __name__=='__main__':unittest.main()
