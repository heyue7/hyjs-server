"""原019修复/020攻城：两个实际阵营采矿店购到共享门HP/原动画/cold/并发，专属InnoDB。"""
import concurrent.futures
import os
import subprocess
import threading
import unittest
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_GATE_ITEM_MYSQL_TEST')=='1','显式启用原门道具专属MySQL')
class P13UnionFightGateItemMySQLTests(field.P13UnionFightFieldMySQLTests):
    test_1_mysql_race_actual25_map_legacy_sha_movement_restart_end=None
    test_2_native_original_frames_private_instances_relogin_end_restart=None

    def tcp_send(self,sock,name,fields,login):
        lock=self.__dict__.setdefault('_tcp_write_locks',{}).setdefault(sock,threading.Lock())
        with lock:return super().tcp_send(sock,name,fields,login)

    def connect(self,login):
        sock=super().connect(login)
        if '_heartbeat_stop' not in self.__dict__:
            self._heartbeat_stop=threading.Event();self._heartbeat_peers=[];self._heartbeat_lock=threading.Lock()
            def worker():
                while not self._heartbeat_stop.wait(5):
                    with self._heartbeat_lock:peers=list(self._heartbeat_peers)
                    for peer,identity in peers:
                        try:self.tcp_send(peer,'CSPingAckTcp',{},identity)
                        except OSError:
                            with self._heartbeat_lock:
                                if (peer,identity) in self._heartbeat_peers:self._heartbeat_peers.remove((peer,identity))
            self._heartbeat_thread=threading.Thread(target=worker,daemon=True);self._heartbeat_thread.start()
            def finish():self._heartbeat_stop.set();self._heartbeat_thread.join(10)
            self.addCleanup(finish)
        with self._heartbeat_lock:self._heartbeat_peers.append((sock,login))
        return sock

    def test_1_mysql_race_actual_two_camp_mine_shop_gate_items_crash_cold_sha(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP13UnionFightGateItemActual','-count=1','-timeout=180s'],cwd=field.SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'));print(result.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p13_union_fight_gate%';").strip(),'InnoDB')

    def test_2_native_original_two_actual_camp_gate_item_consume_tick_crash_cold(self):
        if os.environ.get('HYJS_P13_ITEM_NATIVE_TEST')!='1':self.skipTest('显式启用原门道具HTTP/TCP')
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

    def publish_item_sources(self,first):
        rows=[]
        for item,percent in [('ufitem_019',25),('ufitem_020',50)]:rows.append("('2026-10-20','"+item+"',1,"+str(percent)+","+str(first-60000)+",'隔离具名本地比例，不声称原修门伤害')")
        self.sql('INSERT INTO p13_union_fight_gate_item_sources(day,item_id,enabled,hp_percent,published_at,source) VALUES'+','.join(rows)+';')
        self.sql("INSERT INTO p13_union_fight_item_sources(day,item_id,enabled,hp_percent,published_at,source) VALUES('2026-10-20','ufitem_016',1,20,"+str(first-60000)+",'隔离当地20%普通瓶；共CD验证');")
        rows=[]
        for item in ['ufitem_019','ufitem_020','ufitem_016']:rows.append("('2026-10-20','"+item+"',1,'"+item+"',10,1,3,0,0,60000,"+str(first-60000)+",'隔离实际矿货币店购，当地价10/3次数')")
        self.sql('INSERT INTO p13_union_fight_shop_sources(day,shop_item_id,enabled,item_id,price,quantity,buy_count,buy_condition,priority,refresh_millis,published_at,source) VALUES'+','.join(rows)+';')
        self.sql("INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES('2026-10-20','gate-left',1,'1',-4.7,-0.12,9.98,50,"+str(first-60000)+",'隔离原矿1/回收点/本地50'),('2026-10-20','gate-right',1,'1',80.22,-0.33,-13.36,50,"+str(first-60000)+",'隔离原矿1/回收点/本地50');")

    def check_item_native(self,a,b,c,sa,sb,sc,uids,selected,first,transfer):
        self.tcp_send(sb,'CSCurrentPosition',{},b);self.receive(sb,'SCCurrentPosition');defender=self.receive(sb,'SCUnionFightTransfer')
        self.assertNotEqual(transfer.player.camp,defender.player.camp)
        for index,(login,sock,full) in enumerate([(a,sa,transfer),(b,sb,defender)]):
            camp=full.player.camp;recycler,shop=('2','3') if camp==0 else ('5','6')
            recycle={'x':-4.7,'y':-0.12,'z':9.98} if camp==0 else {'x':80.22,'y':-0.33,'z':-13.36}
            shop_point={'x':-16,'y':-0.12,'z':-9} if camp==0 else {'x':87.8,'y':-0.31,'z':-2.48}
            guid=int(self.sql("SELECT guid FROM p13_union_fight_resource_nodes WHERE match_id='"+transfer.unionFightId+"' AND ABS(x-"+str(recycle['x'])+")<0.001;").strip())
            self.drain(sock)
            self.set_clock(first+20000+index*60000);self.tcp_send(sock,'CSMove',{'pointNew':recycle},login);self.receive(sock,'SCUFElementMove')
            self.tcp_send(sock,'CSUFPlayerCollect',{'resGuid':guid},login)
            # 同场另一成员采矿也广播这个原opcode；只按本人的guid确认开始，再推进服务器时刻。
            for _ in range(80):
                collected=self.receive(sock,'SCUFPlayerCollect')
                if login.playerId in collected.playerGuid:break
            else:self.fail('本人实际采集开始缺失')
            self.assertEqual(collected.collectTime[list(collected.playerGuid).index(login.playerId)],3)
            self.set_clock(first+23000+index*60000);self.assertEqual(self.receive(sock,'SCUFPlayerCollectEndDetail').addResourceNum,50)
            deposited=self.call('CSUFPlayerUpResource',{'npcId':recycler},login,600,'SCUFPlayerUpResource');self.assertEqual(deposited.player.ownToken,150)
            self.set_clock(first+33000+index*60000);self.tcp_send(sock,'CSMove',{'pointNew':shop_point},login);self.receive(sock,'SCUFElementMove')
            catalogue=self.call('CSUFShopList',{'npcId':shop},login,601,'SCUFShopList');self.assertEqual(len(catalogue.items),3)
            kinds=['ufitem_019','ufitem_020','ufitem_016'] if index==0 else ['ufitem_019']
            for i,item in enumerate(kinds):
                for n in range(1 if item=='ufitem_016' else 3):bought=self.call('CSUFPlayerBuyItem',{'shopItemId':item,'npcId':shop},login,700+i*3+n,'SCUFPlayerBuyItemDetail')
            self.assertEqual(bought.player.ownToken,80 if index==0 else 120)
        target=min((unit for unit in transfer.gateInfos if unit.camp==defender.player.camp),key=lambda unit:unit.id)
        lines=(field.SOURCE/'analysis/tables/TableUnionFightGate.tsv').read_text().splitlines()[5:]
        row=next(line.split('\t') for line in lines if line.strip() and int(line.split('\t')[0])==target.id)
        xyz=[float(v) for v in row[2].split(',')];point=dict(zip(['x','y','z'],xyz))
        hammer={'itemId':'ufitem_020'};repair={'itemId':'ufitem_019'}
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=1;')
        self.assertEqual(self.stamped('CSUFPlayerUseItem',hammer,a,800)[0],10001)
        self.set_clock(first+140000)
        for login,sock in [(a,sa),(b,sb)]:self.tcp_send(sock,'CSMove',{'pointNew':point},login);self.receive(sock,'SCUFElementMove')
        self.assertEqual(self.stamped('CSUFPlayerUseItem',repair,a,801)[0],10001)
        self.call('CSUFPlayerUseItem',{'itemId':'ufitem_016'},a,802,'SCUFPlayerUseItemDetail')
        self.assertEqual(self.stamped('CSUFPlayerUseItem',hammer,a,800)[0],10001)
        self.set_clock(first+145000);self.drain(sa);self.drain(sb);self.drain(sc)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSUFPlayerUseItem',hammer,a,800,'SCUFPlayerUseItemDetail'),range(8)))
        first_use=replies[0];self.assertTrue(all(reply==first_use for reply in replies))
        self.assertEqual(next(item.leftUseNum for item in first_use.player.items if item.id=='ufitem_020'),2)
        self.assertEqual(self.receive(sb,'SCUFPlayerUseItem').itemId,'ufitem_020')
        tick=self.receive(sb,'SCUFElementTickExport');self.assertEqual((tick.targetGuid[0],tick.sourceGuid[0],tick.hpDelta[0]),(target.guid,a.playerId,-5000))
        gate=self.receive(sb,'SCUFGate').gate[0];self.assertEqual((gate.guid,gate.hp),(target.guid,5000))
        self.assertFalse(any(op in [BY_NAME['SCUFGate'],BY_NAME['SCUFElementTickExport'],BY_NAME['SCUFGateCrash']] for op,_ in self.drain(sc)))
        repaired=self.call_all('CSUFPlayerUseItem',repair,b,800)
        self.assertEqual(next(v for op,v in repaired if op==BY_NAME['SCUFGate']).gate[0].hp,7500)
        self.assertEqual(next(v for op,v in repaired if op==BY_NAME['SCUFElementTickExport']).hpDelta[0],2500)
        self.sql('UPDATE p13_union_fight_gate_item_sources SET enabled=0,hp_percent=1;')
        self.set_clock(first+150000)
        second=self.call_all('CSUFPlayerUseItem',hammer,a,803);self.assertEqual(next(v for op,v in second if op==BY_NAME['SCUFGate']).gate[0].hp,2500)
        self.stop();self.start();a,b=[self.login(uids[i]) for i in selected[:2]];sa,sb=[self.connect(login) for login in [a,b]]
        for login,sock in [(a,sa),(b,sb)]:
            self.tcp_send(sock,'CSCurrentPosition',{},login);self.receive(sock,'SCCurrentPosition');cold=self.receive(sock,'SCUnionFightTransfer')
            self.assertEqual(next(gate.hp for gate in cold.gateInfos if gate.guid==target.guid),2500)
            self.assertTrue(all(gate.hp==10000 for gate in cold.gateInfos if gate.guid!=target.guid))
        self.set_clock(first+155000)
        final=self.call_all('CSUFPlayerUseItem',hammer,a,804)
        self.assertEqual(next(v for op,v in final if op==BY_NAME['SCUFElementTickExport']).hpDelta[0],-2500)
        self.assertEqual(next(v for op,v in final if op==BY_NAME['SCUFGate']).gate[0].hp,0)
        crash=next(v for op,v in final if op==BY_NAME['SCUFGateCrash']);self.assertEqual(crash.gateId,target.id);self.assertNotEqual(crash.gateId,target.guid)
        self.assertEqual(self.receive(sb,'SCUFGateCrash'),crash)
        self.assertEqual(self.stamped('CSUFPlayerUseItem',repair,b,801)[0],10001)
        self.tcp_send(sb,'CSUFRefreshElement',{'guid':target.guid},b);self.assertEqual(self.receive(sb,'SCUFGate').gate[0].hp,0)
        self.stop();self.start();a=self.login(uids[selected[0]]);sa=self.connect(a)
        self.tcp_send(sa,'CSCurrentPosition',{},a);self.receive(sa,'SCCurrentPosition');cold=self.receive(sa,'SCUnionFightTransfer');self.assertEqual(next(gate.hp for gate in cold.gateInfos if gate.guid==target.guid),0)
        self.sql('UPDATE p13_union_fight_item_policies SET enabled=0;')
        self.assertEqual(self.call('CSUFPlayerUseItem',hammer,a,800,'SCUFPlayerUseItemDetail'),first_use)
        self.set_clock(first+300000);self.assertEqual(self.stamped('CSUFPlayerUseItem',hammer,a,800)[0],10001)

if __name__=='__main__':unittest.main()
