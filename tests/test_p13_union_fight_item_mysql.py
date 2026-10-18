"""原三个生命道具：真实采矿→店购→消费；专属MySQL/HTTP/TCP，伤亡仅显式有限准备。"""
import concurrent.futures
import os
from pathlib import Path
import subprocess
import unittest
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_ITEM_MYSQL_TEST')=='1','显式启用原生命物品专属MySQL')
class P13UnionFightItemMySQLTests(field.P13UnionFightFieldMySQLTests):
    test_1_mysql_race_actual25_map_legacy_sha_movement_restart_end=None
    test_2_native_original_frames_private_instances_relogin_end_restart=None

    def test_1_mysql_race_actual_mine_shop_item_same_and_different_frames_cold(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP13UnionFightItemActual','-count=1','-timeout=180s'],cwd=field.SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'))
        print(result.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p13_union_fight_item%';").strip(),'InnoDB')

    def test_2_native_original_mining_shop_life_cooldown_tcp_cold(self):
        if os.environ.get('HYJS_P13_ITEM_NATIVE_TEST')!='1':self.skipTest('显式启用原生命物品HTTP/TCP')
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

    def publish_item_sources(self,first):
        rows=[];quotes=[]
        for item,percent in [('ufitem_011',50),('ufitem_012',100),('ufitem_016',20)]:
            rows.append("('2026-10-20','"+item+"',1,"+str(percent)+","+str(first-60000)+",'隔离具名当地比例，原数額未知')")
            quotes.append("('2026-10-20','"+item+"',1,'"+item+"',10,1,2,0,0,60000,"+str(first-60000)+",'隔离实际资源店购，当地价10，每种2次')")
        self.sql('INSERT INTO p13_union_fight_item_sources(day,item_id,enabled,hp_percent,published_at,source) VALUES'+','.join(rows)+'; INSERT INTO p13_union_fight_shop_sources(day,shop_item_id,enabled,item_id,price,quantity,buy_count,buy_condition,priority,refresh_millis,published_at,source) VALUES'+','.join(quotes)+';')
        self.sql("INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES('2026-10-20','life-left',1,'1',-4.7,-0.12,9.98,50,"+str(first-60000)+",'隔离原矿型1/回收点/当地50产量'),('2026-10-20','life-right',1,'1',80.22,-0.33,-13.36,50,"+str(first-60000)+",'隔离原矿型1/回收点/当地50产量');")

    def check_item_native(self,a,b,c,sa,sb,sc,uids,selected,first,transfer):
        camp=transfer.player.camp
        recycler,shop=('2','3') if camp==0 else ('5','6')
        recycle={'x':-4.7,'y':-0.12,'z':9.98} if camp==0 else {'x':80.22,'y':-0.33,'z':-13.36}
        shop_point={'x':-16,'y':-0.12,'z':-9} if camp==0 else {'x':87.8,'y':-0.31,'z':-2.48}
        guid=int(self.sql("SELECT guid FROM p13_union_fight_resource_nodes WHERE match_id='"+transfer.unionFightId+"' AND ABS(x-"+str(recycle['x'])+")<0.001;").strip())
        self.set_clock(first+20000)
        self.tcp_send(sa,'CSMove',{'pointNew':recycle},a);self.receive(sa,'SCUFElementMove')
        self.tcp_send(sa,'CSUFPlayerCollect',{'resGuid':guid},a);self.assertEqual(self.receive(sa,'SCUFPlayerCollect').collectTime[0],3)
        self.set_clock(first+23000);mined=self.receive(sa,'SCUFPlayerCollectEndDetail');self.assertEqual(mined.addResourceNum,50)
        deposited=self.call('CSUFPlayerUpResource',{'npcId':recycler},a,600,'SCUFPlayerUpResource');self.assertEqual(deposited.player.ownToken,150)
        self.set_clock(first+33000);self.tcp_send(sa,'CSMove',{'pointNew':shop_point},a);self.receive(sa,'SCUFElementMove')
        catalogue=self.call('CSUFShopList',{'npcId':shop},a,601,'SCUFShopList');self.assertEqual(len(catalogue.items),3)
        for i,item in enumerate(['ufitem_011','ufitem_012','ufitem_016']):
            for n in range(2):bought=self.call('CSUFPlayerBuyItem',{'shopItemId':item,'npcId':shop},a,700+i*2+n,'SCUFPlayerBuyItemDetail')
        self.assertEqual((bought.player.ownToken,len(bought.player.items)),(90,3))
        self.assertTrue(all(item.leftUseNum==2 for item in bought.player.items))
        def prepare(state):
            env=dict(self.env,HYJS_P13_ITEM_NATIVE_PLAYER=str(a.playerId),HYJS_P13_ITEM_NATIVE_STATE=str(state))
            result=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP13UnionFightItemNativePrepareHP$','-count=1'],cwd=field.SOURCE/'hyjs-server',env=env,capture_output=True,text=True,timeout=150)
            self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'))
        prepare(0)
        req={'itemId':'ufitem_011'}
        self.assertEqual(self.stamped('CSUFPlayerUseItem',req,a,800)[0],10001)
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=1;')
        for body,stamp in [({'itemId':' '},890),({'itemId':'bad\x00'},891),({'itemId':'x'*65},892),(req,-1)]:
            self.assertEqual(self.stamped('CSUFPlayerUseItem',body,a,stamp)[0],10001)

        self.drain(sa);self.drain(sb);self.drain(sc)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies=list(pool.map(lambda _:self.call('CSUFPlayerUseItem',req,a,800,'SCUFPlayerUseItemDetail'),range(8)))
        first_use=replies[0];self.assertTrue(all(reply==first_use for reply in replies))
        self.assertEqual((first_use.itemId,first_use.player.items[0].leftUseNum,first_use.player.useDiamondItemCD),('ufitem_011',1,5))
        self.assertEqual(self.stamped('CSUFPlayerUseItem',{'itemId':'  ufitem_011  '},a,800)[0],10001)
        for hp,maximum in zip(first_use.player.fightInfo.hpPos,first_use.player.fightInfo.maxHpPos):self.assertAlmostEqual(hp,maximum*.6)
        message=self.receive(sb,'SCUFPlayerUseItem');self.assertEqual((message.guid,message.itemId),(a.playerId,'ufitem_011'))
        effect=self.receive(sb,'SCUFPlayer');self.assertEqual(effect.player[0].fightInfo,first_use.player.fightInfo)
        self.assertFalse(any(op in [BY_NAME['SCUFPlayerUseItem'],BY_NAME['SCUFPlayer']] for op,_ in self.drain(sc)))
        self.assertFalse(any(op==BY_NAME['SCUFPlayerUseItem'] for op,_ in self.drain(sb)))
        self.assertEqual(self.stamped('CSUFPlayerUseItem',req,a,801)[0],10001)
        self.assertEqual(self.stamped('CSUFPlayerUseItem',{'itemId':'ufitem_012'},a,802)[0],10001)
        self.tcp_send(sa,'CSUFPlayerUseItem',{'itemId':'ufitem_016'},a)
        normal=self.receive(sa,'SCUFPlayerUseItemDetail');self.assertEqual((normal.player.items[2].leftUseNum,normal.player.useNormalItemCD),(1,5))
        self.receive(sa,'SCUFPlayer')
        for hp,maximum in zip(normal.player.fightInfo.hpPos,normal.player.fightInfo.maxHpPos):self.assertAlmostEqual(hp,maximum*.8)
        self.set_clock(first+35000);self.stop();self.start();a=self.login(uids[selected[0]]);sa=self.connect(a)
        self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');cold=self.receive(sa,'SCUnionFightTransfer')
        self.assertEqual((cold.player.useNormalItemCD,cold.player.useDiamondItemCD),(3,3))
        self.assertEqual(cold.player.fightInfo,normal.player.fightInfo)
        self.sql("UPDATE p13_union_fight_item_sources SET enabled=0,hp_percent=1;")
        self.set_clock(first+38000)
        self.assertEqual(self.stamped('CSUFPlayerUseItem',{'itemId':'ufitem_012'},a,803)[0],10001)
        prepare(3)
        revived=self.call('CSUFPlayerUseItem',{'itemId':'ufitem_012'},a,803,'SCUFPlayerUseItemDetail')
        self.assertEqual((revived.player.state,revived.player.items[1].leftUseNum),(0,1))
        self.assertEqual(revived.player.fightInfo.hpPos,revived.player.fightInfo.maxHpPos)
        self.call('CSUFOnHook',{'onHook':1},a,804,'SCUFOnHook')
        self.assertEqual(self.stamped('CSUFPlayerUseItem',req,a,800)[0],10001)
        self.call('CSUFOnHook',{'onHook':0},a,805,'SCUFOnHook')
        self.set_clock(first+43000)
        last=self.call('CSUFPlayerUseItem',req,a,806,'SCUFPlayerUseItemDetail');self.assertEqual(last.player.items[0].leftUseNum,0)
        self.set_clock(first+48000);self.assertEqual(self.stamped('CSUFPlayerUseItem',req,a,807)[0],10001)
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=0;')
        self.assertEqual(self.call('CSUFPlayerUseItem',req,a,800,'SCUFPlayerUseItemDetail'),first_use)
        self.assertEqual(self.stamped('CSUFPlayerUseItem',{'itemId':'ufitem_017'},a,808)[0],10001)
        self.set_clock(first+300000);self.assertEqual(self.stamped('CSUFPlayerUseItem',req,a,800)[0],10001)

if __name__=='__main__':unittest.main()
