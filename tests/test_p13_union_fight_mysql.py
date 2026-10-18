"""原公会战报名交易：专属InnoDB、原HTTP/TCP、真实阵容和冻结恢复。"""
import os
from pathlib import Path
import secrets
import re
import selectors
import subprocess
import types
import unittest
import test_p13_guild_room_mysql as room
import test_p13_mysql as guild
from server.codec import BY_NAME
ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P13_UF_MYSQL_TEST')=='1','显式启用P13报名专属MySQL')
class P13UnionFightMySQLTests(room.P13GuildRoomMySQLTests):
    # 原房间方法仅作为协议助手，单独suite不重复上批测试。
    test_1_go_room_current_membership_private_scope_and_restart=None
    test_2_native_original_room_transfer_current_login_building_move_exit_reconnect=None
    test_3_native_original_readonly_uncreated_buildings_and_unprayed_church=None
    test_4_original_building_permission_cost_shared_push_retry_restart=None
    def start(self):
        self.process=subprocess.Popen([self.binary,'--http-port','0','--tcp-port','0','--tables',str(ROOT/'analysis/tables'),'--log-file',str(Path(self.temp.name)/'p13-union-fight.log')],cwd=ROOT/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ);self.assertTrue(ready.select(90),'P13公会战冷启动超时');line=self.process.stdout.readline()
        ports=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line);self.assertIsNotNone(ports,line.replace(self.test_password,'[redacted]'));self.service=types.SimpleNamespace(http_port=int(ports[1]),tcp_port=int(ports[2]))
    def test_1_mysql_race_original_roster_auto_freeze_and_restart(self):
        args=['rtk','proxy','go','test','-race','./service','-run','^TestP13UnionFight','-count=1','-timeout=150s']
        r=subprocess.run(args,cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=180)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p13_union_fight_%';").strip(),'InnoDB')
    def test_3_mysql_race_match_twenty_real_guilds_and_frozen_preview(self):
        args=["rtk","proxy","go","test","-race","./service","-run","^TestP13UnionFightMatch","-count=1","-timeout=150s"]
        r=subprocess.run(args,cwd=ROOT/"hyjs-server",env=self.env,capture_output=True,text=True,timeout=180)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,"[redacted]"));print(r.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA="+chr(39)+self.database+chr(39)+" AND TABLE_NAME LIKE "+chr(39)+"p13_union_fight_%"+chr(39)+";").strip(),"InnoDB")

    def test_4_original_http_tcp_match_frozen_preview_and_restart(self):
        if os.environ.get('HYJS_P13_UF_NATIVE_TEST')!='1':self.skipTest('显式启用原匹配预览HTTP/TCP')
        from datetime import datetime,timedelta
        from zoneinfo import ZoneInfo
        now=datetime.now(ZoneInfo('Asia/Shanghai'))
        if now.weekday() not in [0,2,4]:self.skipTest('原宣战日需一三五；不修改系统时钟')
        r=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(r.returncode,0,r.stdout+r.stderr)
        self.start();uids=['p13-preview-'+secrets.token_hex(4) for _ in range(6)];players=[self.login(uid) for uid in uids]
        for index,login in enumerate(players):self.call('CSPlayerCreate',{'playerName':'原配对帧'+str(index),'heroId':'hero_047'},login,1,'SCPlayerCreate')
        ids=','.join(str(x.playerId) for x in players);self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000) WHERE id IN ('+ids+'); UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=0; UPDATE p13_union_fight_policies SET enrollment_enabled=1,match_enabled=0;')
        guilds=[]
        for index,login in enumerate(players):
            self.call('CSUnionCreate',{'name':'原配对会'+str(index)},login,2,'SCUnionCreate');gid=self.call('CSGetUnionInfo',{},login,3,'SCGetUnionInfo').unionInfo.unionId;guilds.append(gid)
            self.sql("UPDATE p13_guilds SET level=2 WHERE id='"+gid+"';");self.call('CSUFUnionApply',{},login,4,'SCUFUnionApply');self.call('CSUFPlayerApply',{},login,5,'SCUFPlayerApply')
        day=now.replace(hour=0,minute=0,second=0,microsecond=0)+timedelta(days=1);first=int((day+timedelta(hours=20)).timestamp()*1000)
        self.stop()
        self.sql("INSERT INTO p13_union_fight_schedules(day,first_start_at,second_start_at,duration_millis,source) VALUES('"+day.strftime('%Y-%m-%d')+"',"+str(first)+','+str(first+600000)+",300000,'明确隔离测试日历：非原运营时刻'); UPDATE p13_union_fight_policies SET match_enabled=1;")
        # 只推进隔离数据库截止批次；随后真实服务与协议仍使用实际时钟。
        env=dict(self.env,HYJS_P13_MATCH_NATIVE_ADVANCE='1');r=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP13UnionFightMatchAdvanceIsolatedNativeClock$','-count=1','-timeout=120s'],cwd=ROOT/'hyjs-server',env=env,capture_output=True,text=True,timeout=150);self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
        chosen=self.sql('SELECT left_guild_id FROM p13_union_fight_matches ORDER BY id LIMIT 1;').strip();index=guilds.index(chosen)
        self.start();viewer=self.login(uids[index]);preview=self.call('CSUFInfoPreView',{},viewer,6,'SCUFInfoPreView')
        self.assertEqual(preview.isUnionApplied,1);self.assertEqual(preview.firstFightInfo.ourUnion.unionId,chosen);self.assertNotEqual(preview.firstFightInfo.enemyUnion.unionId,chosen);self.assertEqual(preview.firstFightInfo.startFightTime,first);self.assertFalse(preview.firstFightInfo.HasField('fightResult'));self.assertEqual(preview.firstFightInfo.isRoundBye,0);self.assertEqual(preview.secondFightInfo.startFightTime,first+600000);self.assertEqual(list(preview.firstFightInfo.itemIds),['item_302']);self.assertEqual(list(preview.secondFightInfo.itemIds),['item_302'])
        sock=self.connect(viewer);self.tcp_send(sock,'CSUFInfoPreView',{},viewer);self.assertEqual(self.receive(sock,'SCUFInfoPreView'),preview)
        self.tcp_send(sock,'CSUFCurrentStageInfo',{},viewer);stage=self.receive(sock,'SCUFCurrentStageInfo');self.assertEqual(list(stage.fightTimeShow),[first,first+900000]);self.assertEqual(stage.stage,1)
        self.stop();self.start();viewer=self.login(uids[index]);self.assertEqual(self.call('CSUFInfoPreView',{},viewer,7,'SCUFInfoPreView'),preview);self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_matches;').strip(),'4')

    def test_2_original_http_tcp_live_declaration_snapshot_permissions_and_relogin(self):
        if os.environ.get('HYJS_P13_UF_NATIVE_TEST')!='1':self.skipTest('显式启用原报名HTTP/TCP')
        from datetime import datetime
        from zoneinfo import ZoneInfo
        if datetime.now(ZoneInfo('Asia/Shanghai')).weekday() not in [0,2,4]:self.skipTest('原宣战日需一三五；不修改真实时钟')
        r=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(r.returncode,0,r.stdout+r.stderr)
        self.start();uids=['p13-uf-'+secrets.token_hex(4) for _ in range(4)];a,b,c,d=[self.login(uid) for uid in uids]
        for i,login in enumerate([a,b,c,d]):self.call('CSPlayerCreate',{'playerName':'报名原帧'+str(i),'heroId':'hero_047'},login,1,'SCPlayerCreate')
        ids=','.join(str(x.playerId) for x in [a,b,c,d]);self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000) WHERE id IN ('+ids+'); UPDATE p13_guild_policies SET management_enabled=1,economy_enabled=0;')
        self.call('CSUnionCreate',{'name':'报名原甲'},a,10,'SCUnionCreate');ga=self.call('CSGetUnionInfo',{},a,11,'SCGetUnionInfo').unionInfo.unionId
        for login in [b,c]:self.call('CSUnionApply',{'unionId':ga},login,12,'SCUnionApply');self.call('CSUnionAgreeApply',{'playerId':login.playerId},a,13+login.playerId,'SCUnionAgreeApply')
        self.call('CSPromotionViceChairman',{'playerId':b.playerId},a,19,'SCPromotionViceChairman')
        op,_=self.stamped('CSUFUnionApply',{},a,20);self.assertEqual(op,10001)
        self.sql('UPDATE p13_union_fight_policies SET enrollment_enabled=1;')
        op,_=self.stamped('CSUFUnionApply',{},a,20);self.assertEqual(op,10001)
        self.sql("UPDATE p13_guilds SET level=2 WHERE id='"+ga+"';")
        for login in [c,d]:op,_=self.stamped('CSUFUnionApply',{},login,20);self.assertEqual(op,10001)
        for value in [-1,2]:op,_=self.stamped('CSUFAutoApplySet',{'isAuto':value},a,20);self.assertEqual(op,10001)
        before=[self.profile(x)[0] for x in [a,b,c,d]]
        self.call('CSUFAutoApplySet',{'isAuto':1},b,21,'SCUFAutoApplySet');self.call('CSUFUnionApply',{},b,22,'SCUFUnionApply');self.call('CSUFUnionApply',{},a,22,'SCUFUnionApply')
        info=self.call('CSUFCurrentStageInfo',{},c,23,'SCUFCurrentStageInfo');self.assertEqual((info.stage,info.isAutoApply,info.isUnionApplied,info.appliedNum),(1,1,1,0));self.assertGreater(info.nextStageTime,info.serverTimeNow)
        self.assertEqual(self.call('CSUFPlayerApply',{},c,24,'SCUFPlayerApply').isFirst,1);self.call('CSUFPlayerApply',{},c,24,'SCUFPlayerApply');self.call('CSUFPlayerApply',{},a,25,'SCUFPlayerApply')
        self.call('CSUFSetAutoFight',{'isAuto':1},c,26,'SCUFSetAutoFight')
        sa=self.connect(a);self.tcp_send(sa,'CSUFAppliedPlayer',{},a);reply=self.receive(sa,'SCUFAppliedPlayer');self.assertEqual({x.playerId for x in reply.firsts},{a.playerId,c.playerId});self.assertEqual(len(reply.backUps),0)
        self.assertEqual(next(x for x in reply.firsts if x.playerId==c.playerId).isAutoFight,1)
        # 原3次阵容更新字段返回已用次数，精确stamp重试不增加次数。
        for index in [1,2,3]:
            uploaded=self.call('CSUFUpdateBattlePlayer',{},c,40+index,'SCUFUpdateBattlePlayer');self.assertEqual(uploaded.battleArrayAdjustTime,index)
            self.assertEqual(self.call('CSUFUpdateBattlePlayer',{},c,40+index,'SCUFUpdateBattlePlayer'),uploaded)
        op,_=self.stamped('CSUFUpdateBattlePlayer',{},c,44);self.assertEqual(op,10001)
        info=self.call('CSUFCurrentStageInfo',{},c,45,'SCUFCurrentStageInfo');self.assertEqual(info.battleArrayAdjustTime,3);self.assertEqual(info.adjustTimes,10)
        op,_=self.stamped('CSUFAdjustFirstOn',{'optPlayerId':a.playerId,'optType':0},b,46);self.assertEqual(op,10001)
        for name,fields in [('CSUFAdjustFirstOn',{'optPlayerId':c.playerId,'optType':2}),('CSUFAdjustFirstOn',{'optPlayerId':0,'optType':1}),('CSUFSetFightItems',{'fightIndex':3}),('CSUFSetFightItems',{'fightIndex':0,'itemIds':['item_302','item_302']})]:
            op,_=self.stamped(name,fields,a,47);self.assertEqual(op,10001)
        # 明确隔离P1邮件来源进入原库存，再设置公会战类别17道具；选类型保持7件库存。
        self.sql('INSERT INTO p1_reward_deliveries(player_id,source_key,title,content,rewards,created_at,expires_at,claimed_at) VALUES('+str(c.playerId)+',"p13-item-fixture","隔离公会战道具源","配置边界","item_302,7",1,0,0);')
        inbox=self.call('CSQueryUserReward',{},c,48,'SCQueryUserReward');self.call('CSAddRewardsToPlayer',{'rewardIds':[inbox.rewardItemInfoList[0].rewardId]},c,49,'SCAddRewardsToPlayer')
        chosen=self.call('CSUFSetFightItems',{'fightIndex':0,'itemIds':['item_302']},c,50,'SCUFSetFightItems');self.assertEqual(list(chosen.itemIds),['item_302'])
        op,_=self.stamped('CSUFSetFightItems',{'fightIndex':0,'itemIds':['item_302','item_302']},c,52);self.assertEqual(op,10001)
        self.assertEqual(list(self.call('CSUFCurrentStageInfo',{},c,51,'SCUFCurrentStageInfo').itemIds),['item_302'])
        stacks={x['id']:x['num'] for x in self.profile(c)[1]['items']};self.assertEqual(stacks['item_302'],7)
        self.assertEqual([self.profile(x)[0] for x in [a,b,c,d]],before)
        self.assertEqual(self.sql("SELECT COUNT(*) FROM p13_union_fight_entries WHERE guild_id='"+ga+"';").strip(),'1')
        frozen=self.sql("SELECT HEX(battle) FROM p13_union_fight_fighters WHERE guild_id='"+ga+"' AND player_id="+str(c.playerId)+';').strip();self.assertGreater(len(frozen),0)
        self.stop();self.start();a=self.login(uids[0]);c=self.login(uids[2]);info=self.call('CSUFCurrentStageInfo',{},c,27,'SCUFCurrentStageInfo');self.assertEqual((info.isPlayerApplied,info.isIn25,info.appliedNum),(1,1,2));self.assertEqual(info.battleArrayAdjustTime,3);self.assertEqual(list(info.itemIds),['item_302'])
        self.call('CSUFPlayerApply',{},c,24,'SCUFPlayerApply');self.assertEqual(self.sql("SELECT HEX(battle) FROM p13_union_fight_fighters WHERE guild_id='"+ga+"' AND player_id="+str(c.playerId)+';').strip(),frozen)
        self.call('CSQuitUnion',{},c,28,'SCQuitUnion');op,_=self.stamped('CSUFSetAutoFight',{'isAuto':0},c,29);self.assertEqual(op,10001)

if __name__=='__main__':unittest.main()
