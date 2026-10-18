"""真实P7小时风险→好友原帮助→5钻及实际消费；专属MySQL/原HTTP与内部有界时钟。"""
import concurrent.futures
from datetime import datetime,timedelta
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile
import unittest
from zoneinfo import ZoneInfo
from server.codec import BY_NAME
import test_p10_mysql as transport

ROOT=Path(__file__).resolve().parents[1]
SOURCE=Path(os.environ.get('HYJS_P10_HELP_SOURCE_ROOT',str(ROOT))).resolve()
if SOURCE!=ROOT and not SOURCE.name.startswith('hyjs-p10-exp-help-'):
    raise AssertionError('P10 help source isolation guard')

@unittest.skipUnless(os.environ.get('HYJS_P10_HELP_MYSQL_TEST')=='1','显式启用好友远征专属MySQL')
class P10ExpeditionHelpMySQLTests(transport.P10MySQLTests):
    test_1_innodb_real_relationship_concurrency_private_permissions=None
    test_2_two_accounts_original_http_tcp_offline_and_session_rotation=None
    test_3_native_friend_energy_real_give_claim_flags_and_restart=None
    test_4_original_full_actual_formation_profile_permissions_and_restart=None
    test_5_three_real_tcp_scene_move_line_visibility_session_and_restart=None
    test_6_initial_original_login_queries_work_with_default_closed_policy=None
    test_7_original_world_field_text_visibility_black_retry_history_and_restart=None
    test_8_original_fireworks_real20_bags_shared_throw_claim_race_and_restart=None

    @classmethod
    def setUpClass(cls):
        game=(SOURCE/'hyjs-server/service/gameService.go').read_text()
        if game.index('DispatchP10ExpeditionHelp(ctx')>game.index('DispatchP7Expedition(ctx'):
            raise AssertionError('actual consumer registered after old P7 fallback')
        clock_source=tempfile.TemporaryDirectory(prefix='hyjs-p10-exp-help-native-')
        cls.addClassCleanup(clock_source.cleanup)
        cls.native_root=Path(clock_source.name)
        shutil.copytree(SOURCE/'hyjs-server',cls.native_root/'hyjs-server')
        shutil.copytree(SOURCE/'analysis/tables',cls.native_root/'analysis/tables')
        path=cls.native_root/'hyjs-server/main.go';main=path.read_text()
        if '"hyjs-server/constUtil"' not in main:main=main.replace('"hyjs-server/config"','"hyjs-server/config"\n"hyjs-server/constUtil"',1)
        if '"strings"' not in main:main=main.replace('"strconv"','"strconv"\n"strings"',1)
        marker='defer cancel()\n'
        if main.count(marker)!=1:raise AssertionError('clock injection point')
        main=main.replace(marker,marker+'''
    // 仅专属原帧测试：两个域同一过去时刻；进程重启推进，不接受客户端时间。
    if !strings.HasPrefix(os.Getenv("HYJS_MYSQL_DATABASE"), "hyjs_test_p10_") { return errors.New("helper clock database isolation guard") }
    raw, err := os.ReadFile(os.Getenv("HYJS_P10_HELP_NATIVE_CLOCK"))
    if err != nil { return err }
    at, err := strconv.ParseInt(string(raw), 10, 64)
    if err != nil { return err }
    ctx = context.WithValue(ctx, service.P7ExpeditionClockContextKey{}, time.UnixMilli(at))
    ctx = context.WithValue(ctx, constUtil.P10ExpeditionHelpClockContextKey{}, func() time.Time { return time.UnixMilli(at) })
''',1)
        path.write_text(main)
        result=subprocess.run(['rtk','proxy','gofmt','-w',str(path)],capture_output=True,text=True,timeout=30)
        if result.returncode:raise AssertionError(result.stdout+result.stderr)
        transport.ROOT=cls.native_root
        super().setUpClass()
        if any(getattr(cls,name,False) for name in ['used_pending_patch','used_energy_patch','used_profile_patch','used_scene_patch','used_channel_patch','used_lucky_patch']):
            raise AssertionError('help forbids missing feature integration patches')
        cls.clock=Path(cls.temp.name)/'clock'
        cls.env['HYJS_P10_HELP_NATIVE_CLOCK']=str(cls.clock)

    def set_clock(self,at):self.clock.write_text(str(at))

    def test_1_mysql_race_actual_hourly_source_vip_ten_eight_once_cold(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP10ExpeditionHelpActual','-count=1','-timeout=180s'],cwd=SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'))
        print(result.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p10_expedition_help%';").strip(),'InnoDB')

    def test_2_original_real_accept_hourly_risk_help_diamond_source_spend_cold(self):
        base=datetime.now(ZoneInfo('Asia/Shanghai')).replace(hour=0,minute=0,second=0,microsecond=0)-timedelta(days=1)
        at=int(base.timestamp()*1000);self.set_clock(at);self.start()
        uids=['exp-help-native-'+secrets.token_hex(4) for _ in range(7)]
        players=[self.login(uid) for uid in uids]
        for i,login in enumerate(players):self.call('CSPlayerCreate',{'playerName':'远征实助'+str(i),'heroId':'hero_047'},login,1,'SCPlayerCreate')
        helper=players[0];owners=players[1:6];competitor=players[6]
        self.assertEqual(self.profile(helper)[0]['diamond'],0)
        self.stop()
        prepared=dict(self.env,HYJS_P10_HELP_NATIVE_IDS=','.join(str(login.playerId) for login in players))
        result=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP10ExpeditionHelpNativePartnerPreparation$','-count=1'],cwd=SOURCE/'hyjs-server',env=prepared,capture_output=True,text=True,timeout=150)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'))
        self.sql("UPDATE p10_social_policies SET enabled=1; UPDATE p7_expedition_policies SET enabled=1; UPDATE p7_expedition_event_policies SET enabled=1,risk_percent=100,help_reward='energy,2',source='隔离真实小时风险100与6剧情等权；energy2当地冻结，非原概率'; UPDATE p10_expedition_help_policies SET enabled=0;")
        self.start();players=[self.login(uid) for uid in uids];helper=players[0];owners=players[1:6];competitor=players[6]
        for owner in owners:
            self.call('CSApplyFriend',{'friendId':owner.playerId,'word':'一起救援'},helper,100+owner.playerId,'SCApplyFriend')
            self.call('CSAgreeApply',{'friendId':helper.playerId},owner,200+owner.playerId,'SCAgreeApply')
        self.call('CSApplyFriend',{'friendId':owners[0].playerId,'word':'同时帮助'},competitor,800,'SCApplyFriend')
        self.call('CSAgreeApply',{'friendId':competitor.playerId},owners[0],801,'SCAgreeApply')
        accepted={}
        for owner in owners:
            offers=self.call('CSGetRefreshedMissions',{},owner,300,'SCGetRefreshedMissions').refreshedMissions
            offer=min(offers,key=lambda value:value.endTime-value.startTime) if offers[0].endTime else offers[0]
            idle=self.call('CSGetExpeditionIdleHeros',{'missionSerial':offer.missionSerial},owner,301,'SCGetExpeditionIdleHeros')
            self.assertTrue(idle.canSendGuids)
            self.call('CSAcceptMission',{'missionSerial':offer.missionSerial,'heroGuids':[idle.canSendGuids[0]]},owner,302,'SCAcceptMission')
            record=self.call('CSGetAcceptedMissions',{},owner,303,'SCGetAcceptedMissions').acceptedMissions[0]
            self.assertEqual(record.startTime,at);self.assertIn((record.endTime-at)//3600000,[4,8,12]);accepted[owner.playerId]=record
        self.assertEqual(len(self.call('CSFriendBadEvents',{},helper,900,'SCFriendBadEvents').eventList),0)
        self.stop();self.set_clock(at+3600000);self.start();players=[self.login(uid) for uid in uids];helper=players[0];owners=players[1:6];competitor=players[6]
        pending=self.call('CSFriendBadEvents',{},helper,901,'SCFriendBadEvents');self.assertEqual((pending.todayHelpTime,len(pending.eventList)),(0,5))
        first=next(event for event in pending.eventList if event.friendsId==owners[0].playerId)
        req={'friendId':first.friendsId,'missionSerial':first.missionSerial,'eventSerial':first.eventSerial}
        for invalid,stamp in [({'friendId':0,'missionSerial':first.missionSerial,'eventSerial':first.eventSerial},910),({'friendId':helper.playerId,'missionSerial':first.missionSerial,'eventSerial':first.eventSerial},911),({'friendId':first.friendsId,'missionSerial':' ','eventSerial':first.eventSerial},912),({'friendId':first.friendsId,'missionSerial':first.missionSerial,'eventSerial':'bad\x00'},913)]:
            self.assertEqual(self.stamped('CSHelpFriendEvent',invalid,helper,stamp)[0],10001)
        self.assertEqual(self.stamped('CSHelpFriendEvent',req,helper,0)[0],10001)
        self.assertEqual(self.stamped('CSHelpFriendEvent',req,helper,1000)[0],10001)
        self.sql('UPDATE p10_expedition_help_policies SET enabled=1;')
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies=list(pool.map(lambda _:self.call('CSHelpFriendEvent',req,helper,1000,'SCHelpFriendEvent'),range(8)))
        self.assertTrue(all(reply==replies[0] for reply in replies));self.assertEqual(self.profile(helper)[0]['diamond'],5)
        self.assertEqual(self.stamped('CSHelpFriendEvent',req,helper,1001)[0],10001)
        self.assertEqual(self.stamped('CSHelpFriendEvent',req,competitor,1000)[0],10001)
        detail=self.call('CSExpeditionMissionDetail',{'missionSerial':first.missionSerial},owners[0],1002,'SCExpeditionMissionDetail')
        self.assertTrue(any(event.eventType==4 and event.storyId=='stroy_104' and event.reward=='energy,2' for event in detail.events))
        used=1
        self.sql("UPDATE p7_expedition_event_policies SET enabled=0,risk_percent=0,help_reward='energy,99';")
        for hour in [1,2,3]:
            if hour>1:
                self.stop();self.set_clock(at+hour*3600000);self.start();players=[self.login(uid) for uid in uids];helper=players[0];owners=players[1:6]
            for event in self.call('CSFriendBadEvents',{},helper,1100+hour,'SCFriendBadEvents').eventList:
                if used==10:break
                self.call('CSHelpFriendEvent',{'friendId':event.friendsId,'missionSerial':event.missionSerial,'eventSerial':event.eventSerial},helper,1200+used,'SCHelpFriendEvent');used+=1
        self.assertEqual(used,10);pending=self.call('CSFriendBadEvents',{},helper,1300,'SCFriendBadEvents');self.assertEqual(pending.todayHelpTime,10);self.assertTrue(pending.eventList)
        event=pending.eventList[0]
        self.assertEqual(self.stamped('CSHelpFriendEvent',{'friendId':event.friendsId,'missionSerial':event.missionSerial,'eventSerial':event.eventSerial},helper,1301)[0],10001)
        self.assertEqual(self.profile(helper)[0]['diamond'],50)
        self.call('CSRefreshMissionList',{},helper,1400,'SCRefreshMissionList');self.call('CSRefreshMissionList',{},helper,1401,'SCRefreshMissionList')
        spent=self.call('CSRefreshMissionList',{},helper,1402,'SCRefreshMissionList');self.assertEqual((spent.cost.id,spent.cost.num),('diamond',20));self.assertEqual(self.profile(helper)[0]['diamond'],30)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_expedition_help_actions WHERE helper_id='+str(helper.playerId)+';').strip(),'10')
        self.assertEqual(self.sql("SELECT SUM(amount) FROM p1_event_facts WHERE player_id="+str(helper.playerId)+" AND kind='expedition_help_diamond_received';").strip(),'50')
        self.stop();self.sql('UPDATE p10_expedition_help_policies SET enabled=0;');self.start();helper=self.login(uids[0])
        self.call('CSHelpFriendEvent',req,helper,1000,'SCHelpFriendEvent');self.assertEqual(self.profile(helper)[0]['diamond'],30)
        self.assertEqual(self.call('CSFriendBadEvents',{},helper,1500,'SCFriendBadEvents').todayHelpTime,10)
        sock=self.connect(helper)
        self.tcp_send(sock,'CSFriendBadEvents',{},helper)
        op,tcp_history=self.recv(sock);self.assertEqual(op,BY_NAME['SCFriendBadEvents']);self.assertEqual(tcp_history.todayHelpTime,10)
        self.tcp_send(sock,'CSPlayer',{},helper)
        op,tcp_wallet=self.recv(sock);self.assertEqual(op,BY_NAME['SCPlayer']);self.assertEqual(tcp_wallet.player.diamond,30)
        self.stop();self.set_clock(at+24*3600000);self.start();helper=self.login(uids[0])
        day=self.call('CSFriendBadEvents',{},helper,1501,'SCFriendBadEvents');self.assertEqual(day.todayHelpTime,0);self.assertEqual(len(day.eventList),0)
        self.call('CSHelpFriendEvent',req,helper,1000,'SCHelpFriendEvent');self.assertEqual(self.profile(helper)[0]['diamond'],30)

if __name__=='__main__':unittest.main()
