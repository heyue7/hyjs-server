"""P13真实采集/上交：专属InnoDB、原HTTP/TCP和持久计时；不连接正式库。"""
import os
import subprocess
import unittest
import concurrent.futures
import test_p13_union_fight_field_mysql as field

@unittest.skipUnless(os.environ.get('HYJS_P13_RESOURCE_MYSQL_TEST')=='1','显式启用资源专属MySQL')
class P13UnionFightResourceMySQLTests(field.P13UnionFightFieldMySQLTests):
    test_1_mysql_race_actual25_map_legacy_sha_movement_restart_end=None
    test_2_native_original_frames_private_instances_relogin_end_restart=None
    def test_1_mysql_race_actual_original_sources_mining_capacity_deposit_cold(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP13UnionFightResourceActual|^TestP13UnionFightTransferAll18|^TestP13UnionFightOriginalCache','-count=1','-timeout=180s'],cwd=field.SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'))
        print(result.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p13_union_fight_resource%';").strip(),'InnoDB')
    def test_2_native_actual_original_mining_three_seconds_exhaustion_deposit_restart(self):
        if os.environ.get('HYJS_P13_RESOURCE_NATIVE_TEST')!='1':self.skipTest('显式启用资源原HTTP/TCP')
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

def native_resource_flow(t,uid,first,login,sock,transfer):
    camp=transfer.player.camp
    npc='2' if camp==0 else '5'
    point={'x':-4.7,'y':-0.12,'z':9.98} if camp==0 else {'x':80.22,'y':-0.33,'z':-13.36}
    row=t.sql("SELECT guid FROM p13_union_fight_resource_nodes WHERE match_id='"+transfer.unionFightId+"' AND ABS(x-"+str(point['x'])+")<0.001;").strip()
    guid=int(row)
    t.set_clock(first+20000)
    t.tcp_send(sock,'CSMove',{'pointNew':point},login)
    t.receive(sock,'SCUFElementMove')
    t.tcp_send(sock,'CSUFPlayerCollect',{'resGuid':guid},login)
    started=t.receive(sock,'SCUFPlayerCollect')
    t.assertEqual((list(started.playerGuid),list(started.resGuid),list(started.collectTime)),([login.playerId],[guid],[3]))
    t.set_clock(first+22999)
    before=t.call('CSUFInit',{},login,110,'SCUnionFightTransfer')
    t.assertEqual(before.player.ownRes.resourceNum,0)
    t.stop();t.start()
    login=t.login(uid);sock=t.connect(login)
    t.tcp_send(sock,'CSCurrentPosition',{},login)
    t.receive(sock,'SCCurrentPosition');t.receive(sock,'SCUnionFightTransfer');t.receive(sock,'SCUFResourcePoint')
    recovering=t.receive(sock,'SCUFPlayerCollect');t.assertAlmostEqual(recovering.collectTime[0],0.001,places=6)
    t.set_clock(first+23000)
    finished=t.receive(sock,'SCUFPlayerCollectEndDetail')
    t.assertEqual((finished.resGuid,finished.addResourceNum,finished.player.ownRes.resourceNum),(guid,2,2))
    for count in range(1,5):
        t.tcp_send(sock,'CSUFPlayerCollect',{'resGuid':guid},login);t.receive(sock,'SCUFPlayerCollect')
        t.set_clock(first+23000+count*3000)
        finished=t.receive(sock,'SCUFPlayerCollectEndDetail')
        t.assertEqual((finished.addResourceNum,finished.player.ownRes.resourceNum,finished.isResExhausted),(2,2*(count+1),int(count==4)))
    op,_=t.stamped('CSUFPlayerCollect',{'resGuid':guid},login,120);t.assertEqual(op,10001)
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
        replies=list(pool.map(lambda _:t.call('CSUFPlayerUpResource',{'npcId':npc},login,130,'SCUFPlayerUpResource'),range(8)))
    t.assertTrue(all(x==replies[0] for x in replies))
    t.assertEqual((replies[0].resourceNum,replies[0].playerReward,replies[0].player.ownRes.resourceNum,replies[0].player.ownToken),(10,30,0,30))
    amount=t.sql("SELECT amount FROM p13_union_fight_camp_resources WHERE match_id='"+transfer.unionFightId+"' AND guild_id=(SELECT guild_id FROM p13_guild_members WHERE player_id="+str(login.playerId)+");").strip();t.assertEqual(amount,'10')
    t.stop();t.start();login=t.login(uid);sock=t.connect(login)
    t.tcp_send(sock,'CSUFInit',{},login);restored=t.receive(sock,'SCUnionFightTransfer')
    t.assertEqual((restored.player.ownRes.resourceNum,restored.player.ownToken),(0,30))
    t.assertEqual(next(u.unionResource.resourceNum for u in restored.unionInfos if u.camp==camp),10)
    return login,sock

if __name__=='__main__':unittest.main()

def native_transfer_flow(t,uid,first,login,sock,transfer):
    camp=transfer.player.camp
    fixed='transfer_1' if camp==0 else 'transfer_9'
    start={'x':-3.5,'y':0,'z':-21.5} if camp==0 else {'x':78.5,'y':0,'z':18}
    target={'x':0,'y':0,'z':-24} if camp==0 else {'x':75.5,'y':0,'z':22.5}
    t.set_clock(first+55000)
    t.tcp_send(sock,'CSMove',{'pointNew':start},login);t.receive(sock,'SCUFElementMove')
    t.tcp_send(sock,'CSUFBeginTransfer',{'transferId':fixed,'target':target},login)
    begin=t.receive(sock,'SCUFBeginTransfer');t.assertEqual(begin.costTime,1)
    op,_=t.stamped('CSUFEndTransfer',{'target':target},login,141);t.assertEqual(op,10001)
    t.set_clock(first+55999)
    t.stop();t.start();login=t.login(uid);sock=t.connect(login)
    t.tcp_send(sock,'CSCurrentPosition',{},login);t.receive(sock,'SCCurrentPosition');t.receive(sock,'SCUnionFightTransfer');t.receive(sock,'SCUFResourcePoint')
    pending=t.receive(sock,'SCUFBeginTransfer');t.assertAlmostEqual(pending.costTime,0.001,places=6)
    t.set_clock(first+56000)
    end=t.receive(sock,'SCUFEndTransfer');t.assertEqual(end.costTime,1)
    t.assertEqual((end.target.x,end.target.y,end.target.z),(target['x'],target['y'],target['z']))
    again=t.call('CSUFEndTransfer',{'target':target},login,142,'SCUFEndTransfer');t.assertEqual(again,end)
    state=t.profile(login)[1]['position']['current'];t.assertAlmostEqual(state.get('x',0),target['x'],places=4);t.assertAlmostEqual(state.get('z',0),target['z'],places=4)
    back='transfer_17' if camp==0 else 'transfer_18'
    back_target={'x':-12,'y':0,'z':-2} if camp==0 else {'x':86,'y':0,'z':0}
    t.tcp_send(sock,'CSUFBeginTransfer',{'transferId':back,'target':back_target},login);t.assertEqual(t.receive(sock,'SCUFBeginTransfer').costTime,5)
    t.set_clock(first+61000);t.assertEqual(t.receive(sock,'SCUFEndTransfer').costTime,5)
    return login,sock
