"""原四车厂首车：真实400采矿资源/原HP明确本地发布/原帧/冷恢复，隔离InnoDB。"""
import concurrent.futures
import os
import subprocess
import unittest
import test_p13_union_fight_field_mysql as field


@unittest.skipUnless(os.environ.get('HYJS_P13_FACTORY_MYSQL_TEST')=='1','显式启用原车厂专属MySQL')
class P13UnionFightFactoryMySQLTests(field.P13UnionFightFieldMySQLTests):
    test_1_mysql_race_actual25_map_legacy_sha_movement_restart_end=None
    test_2_native_original_frames_private_instances_relogin_end_restart=None
    def test_1_mysql_race_original_all4_factory_and_round2_loading(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP13UnionFightFactoryActual|^TestP13UnionFightPositionLoadingSameSceneNextRound','-count=1','-timeout=180s'],cwd=field.SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'));print(result.stdout.strip())
    def test_2_native_original_resource400_factory_timing_restart(self):
        if os.environ.get('HYJS_P13_FACTORY_NATIVE_TEST')!='1':self.skipTest('显式启用车厂原HTTP/TCP')
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)


def publish_factory_sources(t,first):
    t.sql("INSERT INTO p13_union_fight_factory_sources(day,factory_id,enabled,chariot_hp,published_at,source) VALUES"+','.join("('2026-10-20',"+str(index)+",1,"+str(1000+index)+","+str(first-60000)+",'隔离具名本地HP1000+原厂ID；不声称原服务端公式')" for index in range(1,5))+';')
    rows=[]
    for camp,point in [(0,(-4.7,-0.12,9.98)),(1,(80.22,-0.33,-13.36))]:
        for index in range(2):
            rows.append("('2026-10-20','factory-native-"+str(camp)+'-'+str(index)+"',1,'1',"+','.join(str(n) for n in point)+",50,"+str(first-60000)+",'隔离原型1/原回收NPC位置/本地50产量和双点；非原spawn')")
    t.sql('INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES'+','.join(rows)+';')


def native_factory_flow(t,uid,first,login,sock,transfer):
    camp=transfer.player.camp
    recycler,npc,factory_id=('2','1',3) if camp==0 else ('5','4',4)
    recycle_point={'x':-4.7,'y':-0.12,'z':9.98} if camp==0 else {'x':80.22,'y':-0.33,'z':-13.36}
    factory_point={'x':15,'y':-4.02,'z':26} if camp==0 else {'x':60,'y':-3.58,'z':-30}
    guid=next(unit.guid for unit in transfer.factoryInfos if unit.factoryId==factory_id)
    nodes=[int(row) for row in t.sql("SELECT guid FROM p13_union_fight_resource_nodes WHERE match_id='"+transfer.unionFightId+"' AND ABS(x-"+str(recycle_point['x'])+")<0.001 ORDER BY guid;").splitlines()]
    t.assertEqual(len(nodes),2)
    t.set_clock(first+20000)
    t.tcp_send(sock,'CSMove',{'pointNew':recycle_point},login);t.receive(sock,'SCUFElementMove')
    for index in range(8):
        node=nodes[0] if index<5 else nodes[1]
        t.tcp_send(sock,'CSUFPlayerCollect',{'resGuid':node},login);t.assertEqual(t.receive(sock,'SCUFPlayerCollect').collectTime[0],3)
        t.set_clock(first+23000+index*3000);t.assertEqual(t.receive(sock,'SCUFPlayerCollectEndDetail').addResourceNum,50)
        deposited=t.call('CSUFPlayerUpResource',{'npcId':recycler},login,600+index,'SCUFPlayerUpResource');t.assertEqual(deposited.player.ownToken,(index+1)*150)
    request={'factoryGuid':guid,'npcId':npc}
    op,_=t.stamped('CSUFFieldCariotFactoryOpen',request,login,800);t.assertEqual(op,10001)
    t.set_clock(first+64000)
    t.tcp_send(sock,'CSMove',{'pointNew':factory_point},login);t.receive(sock,'SCUFElementMove')
    quote=t.call('CSUFOpenFactoryCost',{'factoryId':guid},login,700,'SCUFOpenFactoryCost');t.assertEqual(quote.cost.resourceNum,400)
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
        replies=list(pool.map(lambda _:t.call('CSUFFieldCariotFactoryOpen',request,login,800,'SCUFCariotFactoryOpen'),range(8)))
    t.assertTrue(all(reply==replies[0] for reply in replies));opened=replies[0]
    t.assertEqual((opened.factory.isOpen,opened.factory.state),(1,0))
    t.tcp_send(sock,'CSUFInit',{},login);after=t.receive(sock,'SCUnionFightTransfer')
    t.assertEqual(after.player.ownToken,1200);t.assertEqual(next(unit.unionResource.resourceNum for unit in after.unionInfos if unit.camp==camp),0)
    t.set_clock(first+175329)
    t.tcp_send(sock,'CSUFInit',{},login);before=t.receive(sock,'SCUnionFightTransfer')
    t.assertEqual(next(unit.state for unit in before.factoryInfos if unit.guid==guid),0)
    t.set_clock(first+175330);t.assertEqual(t.receive(sock,'SCUFFactoryBuildPlay').guid,guid)
    t.set_clock(first+183999);t.stop();t.start();login=t.login(uid);sock=t.connect(login)
    t.tcp_send(sock,'CSCurrentPosition',{},login);t.receive(sock,'SCCurrentPosition');cold=t.receive(sock,'SCUnionFightTransfer')
    t.assertEqual(next(unit.state for unit in cold.factoryInfos if unit.guid==guid),1)
    t.set_clock(first+184000);produced=t.receive(sock,'SCUFCariotFactoryBuildComplete')
    t.assertEqual((produced.factoryGuid,produced.cariot.cariotId,produced.cariot.factoryGuid,produced.cariot.hp,produced.cariot.maxHp),(guid,factory_id,guid,1000+factory_id,1000+factory_id))
    t.tcp_send(sock,'CSUFRefreshElement',{'guid':produced.cariot.guid},login);car=t.receive(sock,'SCUFCaropt')
    t.assertEqual(list(car.cariot),[produced.cariot]);t.assertEqual(list(car.local),[produced.pos])
    t.assertEqual(t.call('CSUFFieldCariotFactoryOpen',request,login,800,'SCUFCariotFactoryOpen'),opened)
    t.stop();t.start();login=t.login(uid);sock=t.connect(login)
    t.tcp_send(sock,'CSUFInit',{},login);cold=t.receive(sock,'SCUnionFightTransfer');all_cars=t.receive(sock,'SCUFCaropt')
    t.assertEqual(next(unit.cariotGuid for unit in cold.factoryInfos if unit.guid==guid),produced.cariot.guid)
    t.assertEqual(sum(unit.guid==produced.cariot.guid for unit in all_cars.cariot),1)
    return login,sock


if __name__=='__main__':unittest.main()
