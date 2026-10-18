"""既有Daily原录像只读恢复/跨账号/重启，独占InnoDB测试库结束清理。"""
import concurrent.futures
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import unittest
from server.codec import BY_NAME
import test_p10_mysql as transport
ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_DPLAY_MYSQL_TEST')=='1','显式启用Daily播放专属MySQL')
class DailyPlaybackMySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    stamped=transport.P10MySQLTests.stamped
    call=transport.P10MySQLTests.call
    profile=transport.P10MySQLTests.profile
    recv=transport.P10MySQLTests.recv
    tcp_send=transport.P10MySQLTests.tcp_send
    connect=transport.P10MySQLTests.connect
    start=transport.P10MySQLTests.start
    stop=transport.P10MySQLTests.stop
    setUp=transport.P10MySQLTests.setUp
    @classmethod
    def setUpClass(cls):
        cls.database='hyjs_test_dplay_'+secrets.token_hex(5);cls.test_user='dplay_'+secrets.token_hex(5);cls.test_password=secrets.token_hex(24)
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-dplay-isolated-');cls.addClassCleanup(cls.temp.cleanup);cls.binary=str(Path(cls.temp.name)/'hyjs-dplay')
        cls.root_sql("CREATE DATABASE `"+cls.database+"` CHARACTER SET utf8mb4; CREATE USER '"+cls.test_user+"'@'%' IDENTIFIED BY '"+cls.test_password+"'; GRANT ALL ON `"+cls.database+"`.* TO '"+cls.test_user+"'@'%';")
        cls.addClassCleanup(lambda:cls.root_sql("DROP DATABASE IF EXISTS `"+cls.database+"`; DROP USER IF EXISTS '"+cls.test_user+"'@'%';"))
        cls.env=dict(os.environ,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER=cls.test_user,HYJS_MYSQL_PASSWORD=cls.test_password,HYJS_MYSQL_DATABASE=cls.database,HYJS_DPLAY_MYSQL_HOST='127.0.0.1:33068',HYJS_DPLAY_MYSQL_USER=cls.test_user,HYJS_DPLAY_MYSQL_PASSWORD=cls.test_password,HYJS_DPLAY_MYSQL_DATABASE=cls.database)
        if 'return s.dailyPlayback(ctx, id, request.GetFightId())' not in (ROOT/'hyjs-server/service/battleReplayService.go').read_text():raise AssertionError('Daily播放共享fallback尚未集成')
        r=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,text=True,timeout=180)
        if r.returncode:raise AssertionError(r.stdout+r.stderr)
    @classmethod
    def root_sql(cls,text):
        if not cls.database.startswith('hyjs_test_dplay_') or not cls.test_user.startswith('dplay_'):raise AssertionError('Daily playback isolation guard')
        r=subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],input=text,text=True,capture_output=True,timeout=30)
        if r.returncode:raise AssertionError(r.stderr.replace(cls.test_password,'[redacted]'))
        return r.stdout
    def sql(self,text):return self.root_sql('USE `'+self.database+'`; '+text)
    def test_1_original_go_mysql_readonly_native_two_losses_conflicts_and_recovery(self):
        args=['rtk','proxy','go','test','./service','-run','^TestDailyPlayback','-count=1','-timeout=120s']
        if os.environ.get('HYJS_DPLAY_GO_RACE')=='1':args.insert(4,'-race')
        r=subprocess.run(args,cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
    def test_2_original_http_tcp_playback_two_real_saved_loss_fixtures_restart_and_all_storage_unchanged(self):
        from server.codec import message
        self.start();uids=['dplay-original-'+secrets.token_hex(4) for _ in range(2)];players=[self.login(uid) for uid in uids]
        for player,name in zip(players,['原录像恢复甲','原录像恢复乙']):self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},player,1,'SCPlayerCreate')
        owner=players[0].playerId
        self.sql('UPDATE p10_scene_policies SET enabled=0;')
        expected={}
        for index,file in enumerate(['daily-id4-native-20261013.json','daily-fooddark-id4-native-20261013.json']):
            fixture=json.loads((ROOT/'analysis/p6'/file).read_text());start=message('SCDailyCopyFightStart');start.ParseFromString(bytes.fromhex(fixture['start_hex']));end=message('CSDailyCopyFightEnd');end.ParseFromString(bytes.fromhex(fixture['end_hex']))
            start.battlePlayer.playerId=owner
            kind,stamina=('TreasureExp',10) if end.ChapterId=='5' else ('PetFood_dark',30)
            settled=message('SCDailyCopyFightEnd',isWin=end.isWin,lastTime=0,starNum=0);getattr(settled,'get').SetInParent();settled.cost.add(id='stamina',num=stamina)
            stamp=100+index;fid=start.fightId
            self.sql("DELETE FROM daily_copy_start_receipts WHERE fight_id='"+fid+"'; DELETE FROM daily_copy_fights WHERE fight_id='"+fid+"';")
            self.sql("INSERT INTO daily_copy_fights(fight_id,player_id,start_stamp,day,type,chapter_id,stamina,win_stars,reward,start_snapshot,settled,end_request,response) VALUES ('"+fid+"',"+str(owner)+","+str(stamp)+",'2026-10-13','"+kind+"','"+end.ChapterId+"',"+str(stamina)+",3,'coin,1',X'"+start.SerializeToString().hex()+"',1,X'"+fixture['end_hex']+"',X'"+settled.SerializeToString().hex()+"'); INSERT INTO daily_copy_start_receipts(player_id,stamp,chapter_id,fight_id) VALUES ("+str(owner)+","+str(stamp)+",'"+end.ChapterId+"','"+fid+"');")
            expected[fid]=(start,end)
        before=[self.profile(x) for x in players]
        snapshot=self.sql("SELECT CONCAT(fight_id,'|',HEX(start_snapshot),'|',HEX(end_request),'|',HEX(response),'|',settled) FROM daily_copy_fights WHERE player_id="+str(owner)+" ORDER BY fight_id;")
        p6before=self.sql('SELECT COUNT(*) FROM p6_battle_records; SELECT id,authoritative_enabled,native_capability_hash,client_hash,max_rounds,timeout_millis,source FROM p6_combat_rules ORDER BY id;')
        for fid,(start,end) in expected.items():
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda i:self.call('CSFightPlayback',{'fightId':fid,'serverId':1},players[0],200+i,'SCFightPlayback'),range(8)))
            response=replies[0];self.assertTrue(all(response==r for r in replies));self.assertEqual((response.battleType,response.serverResult,response.clientResult,response.maxRound),(4,1,1,40));self.assertEqual(response.randSeed,start.randSeed);self.assertEqual(response.attacker,start.battlePlayer);self.assertEqual(response.defender2,start.monster);self.assertEqual(response.optRecord,end.optRecord);self.assertFalse(response.HasField('defender1'))
            op,r=self.stamped('CSFightPlayback',{'fightId':fid,'serverId':1},players[1],300);self.assertEqual(op,10001,str(r))
            op,r=self.stamped('CSFightPlayback',{'fightId':fid,'serverId':2},players[0],301);self.assertEqual(op,10001,str(r))
            self.sql("UPDATE daily_copy_fights SET settled=0 WHERE fight_id='"+fid+"';");op,r=self.stamped('CSFightPlayback',{'fightId':fid,'serverId':1},players[0],302);self.assertEqual(op,10001,str(r));self.sql("UPDATE daily_copy_fights SET settled=1 WHERE fight_id='"+fid+"';")
            sock=self.connect(players[0]);self.tcp_send(sock,'CSFightPlayback',{'fightId':fid,'serverId':1},players[0]);op,tcp=self.recv(sock);self.assertEqual(op,BY_NAME['SCFightPlayback']);self.assertEqual(tcp,response);sock.close()
        self.stop();self.start();players=[self.login(uid) for uid in uids]
        for fid,(start,end) in expected.items():self.assertEqual(self.call('CSFightPlayback',{'fightId':fid,'serverId':1},players[0],401,'SCFightPlayback').optRecord,end.optRecord)
        self.assertEqual([self.profile(x) for x in players],before)
        self.assertEqual(self.sql("SELECT CONCAT(fight_id,'|',HEX(start_snapshot),'|',HEX(end_request),'|',HEX(response),'|',settled) FROM daily_copy_fights WHERE player_id="+str(owner)+" ORDER BY fight_id;"),snapshot)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p6_battle_records; SELECT id,authoritative_enabled,native_capability_hash,client_hash,max_rounds,timeout_millis,source FROM p6_combat_rules ORDER BY id;'),p6before)
        print('Actual source original60001/60002 HTTP/TCP; two native historical loss inputs+ops unchanged; readonly replay storage/authority/profile stable across restart.')
