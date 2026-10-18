"""P14真实两服本地海选截止名单；不产生诊断赢家，不连接正式数据库。"""
import json
import secrets
import subprocess
import time
import os
import unittest
import test_p14_cross_mysql as cross


@unittest.skipUnless(os.environ.get('HYJS_P14_SEASON_MYSQL_TEST')=='1','显式启用海选截止独立MySQL')
class P14SeasonMySQLTests(cross.P14CrossMySQLTests):
    test_1_mysql_race_real_snapshot_identity_permissions_replay_and_outbox=None
    test_2_two_isolated_real_servers_original_frames_same_id_signed_route_offline_restart=None
    test_3_mysql_race_all_published_original_calendar_stages=None
    test_4_two_real_servers_original_calendar_frames_history_and_restart=None

    def test_1_mysql_race_roster_cutoff_original_all_stages_and_queued_upload(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP14SeasonActual','-count=1','-timeout=120s'],cwd=cross.SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'));print(result.stdout.strip())

    def test_3_mysql_race_actual_random_matches_original_p6_replay_and_authority_gate(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP14AuditionActual','-count=1','-timeout=180s'],cwd=cross.SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'));print(result.stdout.strip())

    def test_2_two_actual_realms_same_id_local_roster_original_admission_cold_and_history(self):
        result=subprocess.run(['rtk','proxy','go','build','-o',self.binary,'.'],cwd=cross.SOURCE/'hyjs-server',capture_output=True,text=True,timeout=180);self.assertEqual(result.returncode,0,result.stdout+result.stderr)
        self.start(1);self.start(2)
        uids={zone:'p14-audition-'+str(zone)+'-'+secrets.token_hex(4) for zone in [1,2]}
        logins={zone:self.login(zone,uids[zone]) for zone in [1,2]}
        self.assertEqual(logins[1].playerId,logins[2].playerId);pid=logins[1].playerId
        for zone in [1,2]:self.call(zone,'CSPlayerCreate',{'playerName':'真实海选服'+str(zone),'heroId':'hero_047'},logins[zone],2,'SCPlayerCreate')
        key=secrets.token_bytes(32);now=int(time.time()*1000)
        intervals={1:(now-2000,now-1000),2:(now-1000,now+15000),3:(now+15000,now+16000),4:(now+16000,now+120000)}
        for stage in range(5,20):intervals[stage]=(now+120000+(stage-5)*1000,now+121000+(stage-5)*1000)
        values=','.join("(9,"+str(start)+','+str(stage)+','+str(end)+",'真实发布原阶段"+str(stage)+"','隔离具名运营时刻，原日期未知')" for stage,(start,end) in intervals.items())
        for zone in [1,2]:
            self.sql(zone,'UPDATE profiles SET player=JSON_SET(player,"$.level",70) WHERE id='+str(pid)+'; DELETE FROM p14_time_slots; UPDATE p14_season_policies SET enabled=1,season_index=9,register_start='+str(now-1000)+',register_end='+str(now+15000)+'; INSERT INTO p14_time_slots(season_index,start_at,stage,end_at,description,source) VALUES'+values+'; INSERT INTO p14_trusted_peers(server_id,enabled,endpoint,signing_key) VALUES('+str(3-zone)+',1,"http://127.0.0.1:'+str(self.ports[3-zone][0])+'",UNHEX("'+key.hex()+'"));')
            self.call(zone,'CSKFEnterGame',{},logins[zone],10,'SCKFEnterGame')
        for zone in [1,2]:
            self.wait_sql(zone,'SELECT COUNT(*) FROM p14_members WHERE season_index=9;','2')
            self.wait_sql(zone,'SELECT COUNT(*) FROM p14_audition_boundaries WHERE season_index=9 AND server_id='+str(zone)+';','1',timeout=25)
            self.assertEqual(self.sql(zone,'SELECT CONCAT(server_id,",",player_id,",",revision) FROM p14_audition_rosters WHERE season_index=9;').strip(),str(zone)+','+str(pid)+',1')
            self.assertEqual(self.sql(zone,'SELECT count FROM p14_audition_boundaries WHERE season_index=9;').strip(),'1')
            phase=self.call(zone,'CSKFOpenFrightServerInfo',{},logins[zone],11,'SCKFOpenFrightServerInfo')
            self.assertIn(phase.typeStage,[3,4]);self.assertEqual(phase.isEnterCurStage,1)
            op,_=self.send(zone,'CSKFEnterFrightGame',{},logins[zone],12);self.assertEqual(op,10001)
            self.call(zone,'CSKFEnterGame',{},logins[zone],10,'SCKFEnterGame')
        snapshots={zone:self.sql(zone,'SELECT HEX(battle),source_hash FROM p14_audition_rosters WHERE season_index=9;').strip() for zone in [1,2]}
        for zone in [1,2]:self.stop(zone)
        for zone in [1,2]:self.start(zone);logins[zone]=self.login(zone,uids[zone])
        for zone in [1,2]:
            self.assertEqual(self.sql(zone,'SELECT HEX(battle),source_hash FROM p14_audition_rosters WHERE season_index=9;').strip(),snapshots[zone])
            sock=self.tcp(zone,logins[zone]);self.tcp_send(zone,sock,'CSKFOpenFrightServerInfo',{},logins[zone]);op,phase=cross.guild.P13MySQLTests.recv(self,sock);self.assertEqual((op,phase.typeStage,phase.isEnterCurStage),(cross.BY_NAME['SCKFOpenFrightServerInfo'],4,1))
        # 原报名不足autoPass由真实单人本服名册产生；外国同localID不能形成对局或胜场。
        qualifications={}
        for zone in [1,2]:
            self.sql(zone,'UPDATE p14_audition_match_policies SET enabled=1 WHERE id=1;')
            self.wait_sql(zone,'SELECT COUNT(*) FROM p14_audition_qualifications WHERE season_index=9 AND server_id='+str(zone)+';','1')
            qualifications[zone]=self.sql(zone,'SELECT round_count,HEX(players),source_hash FROM p14_audition_qualifications WHERE season_index=9;').strip()
            self.assertEqual(self.sql(zone,'SELECT COUNT(*) FROM p14_audition_matches;').strip(),'0')
            result=self.call(zone,'CSKuaFuMatchResult',{},logins[zone],120,'SCKuaFuMatchResult')
            self.assertEqual((result.result,result.matchResult.typeStage,result.matchResult.winTimes,result.matchResult.failTimes),(1,1,0,0))
            replays=self.call(zone,'CSKFReplayInfo',{'type':1,'subType':1,'pageIndex':0},logins[zone],121,'SCKFReplayInfo')
            self.assertEqual((replays.type,replays.subType,replays.pageIndex,replays.pageNum,len(replays.replays)),(1,1,0,1,0))
            sock=self.tcp(zone,logins[zone]);self.tcp_send(zone,sock,'CSKuaFuMatchResult',{},logins[zone]);op,tcp_result=cross.guild.P13MySQLTests.recv(self,sock)
            self.assertEqual((op,tcp_result),(cross.BY_NAME['SCKuaFuMatchResult'],result))
        for zone in [1,2]:self.stop(zone)
        for zone in [1,2]:
            self.start(zone);logins[zone]=self.login(zone,uids[zone])
            self.assertEqual(self.sql(zone,'SELECT round_count,HEX(players),source_hash FROM p14_audition_qualifications WHERE season_index=9;').strip(),qualifications[zone])
        # 同编号的发布时刻修改不覆盖已封存实例；旧表发布错误仍由未冻结路径拒绝。
        for zone in [1,2]:
            self.sql(zone,'UPDATE p14_time_slots SET end_at=end_at+1000 WHERE season_index=9 AND stage=1;')
            calendar=self.call(zone,'CSKFTimeInfo',{'peakMatchIndex':9},logins[zone],13,'SCKFTimeInfo');self.assertEqual([slot.stage for slot in calendar.timeList],list(range(1,20)));self.assertEqual(calendar.timeList[0].end,now-1000)
            self.sql(zone,'UPDATE p14_season_policies SET season_index=10,register_start=0,register_end=0;')
            current=self.call(zone,'CSKFOpenFrightServerInfo',{},logins[zone],14,'SCKFOpenFrightServerInfo');self.assertEqual((current.typeStage,current.isEnterCurStage),(0,0))
            self.assertEqual(self.sql(zone,'SELECT HEX(battle),source_hash FROM p14_audition_rosters WHERE season_index=9;').strip(),snapshots[zone])
            self.assertEqual(self.call(zone,'CSKFTimeInfo',{'peakMatchIndex':9},logins[zone],15,'SCKFTimeInfo'),calendar)


if __name__=='__main__':unittest.main()
