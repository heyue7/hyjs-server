"""Black-box Go migration checks using the independently recovered Python codec.

Build first: cd hyjs-server && go build -o ../build/hyjs-server .
All accounts and database changes stay in a temporary directory.
"""
import concurrent.futures
import json
import os
from pathlib import Path
import re
import selectors
import socket
import struct
import subprocess
import types
import time
import unittest

from test_server import LiveServerTests
from server.codec import BY_NAME, decode_responses, message
from server.store import Store
from server.gameplay import Gameplay

ROOT = Path(__file__).resolve().parents[1]


class GoServerTests(LiveServerTests):
    first_mission_stamina = 100
    def start(self):
        self.process = subprocess.Popen(
            [os.environ.get('HYJS_GO_TEST_BINARY', str(ROOT/'build/hyjs-server')), '--db', str(self.path), '--http-port', '0', '--tcp-port', '0'],
            cwd=ROOT/'hyjs-server', stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        self.addCleanup(self.stop)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            if not ready.select(20):
                self.fail('Go server did not start within 20 seconds')
            line = self.process.stdout.readline()
        match = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(match, line)
        self.service = types.SimpleNamespace(http_port=int(match[1]), tcp_port=int(match[2]), store=Store(self.path))

    def stop(self):
        proc = getattr(self, 'process', None)
        if proc is not None and proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=8)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait()
                self.fail('Go server did not shut down gracefully')
        if proc is not None and proc.stdout:
            proc.stdout.close()

    def test_clock_correction_replies_use_unix_milliseconds_and_cross_server_stays_closed(self):
        # APK ServerTimerTool.CorrectTime is called by login and the main-town cross-server icon.
        # An empty currentTime would reset the clock to 1970 and break all persisted request receipts.
        before = int(time.time()*1000)
        login = self.login('clock-correction')
        self.assertGreaterEqual(login.serverTimeNow, before)
        self.assertLessEqual(login.serverTimeNow, int(time.time()*1000))
        self.assertGreater(login.serverTimeNow, 1000000000000)
        self.send('CSPlayerCreate', {'playerName':'时钟回归','heroId':'hero_047'}, login)
        saved = self.service.store.find('clock-correction')
        before = int(time.time()*1000)
        opcode, info = self.send('CSKFOpenFrightServerInfo', login=login)
        self.assertEqual(opcode, BY_NAME['SCKFOpenFrightServerInfo'], str(info))
        self.assertGreaterEqual(info.currentTime, before)
        self.assertLessEqual(info.currentTime, int(time.time()*1000))
        self.assertGreater(info.currentTime, 1000000000000)
        self.assertEqual((info.typeStage,info.endTime,info.isEnterCurStage,info.isNeedPoped,info.mateTime),(0,0,0,0,0))
        self.assertEqual(self.service.store.find('clock-correction'), saved)
        # Relogin recalibrates the same Unix-ms domain; a closed/unsupported union screen has no success clock payload.
        login = self.login('clock-correction')
        self.assertGreaterEqual(login.serverTimeNow, info.currentTime)
        opcode, error = self.send('CSUFCurrentStageInfo', login=login)
        self.assertEqual((opcode,error.code),(10001,'90001'))

    def test_unsupported_gameplay_does_not_return_fake_success(self):
        login = self.login()
        self.send('CSPlayerCreate', {'playerName':'迁移测试'}, login)
        opcode, error = self.send('CSPetLevelup', login=login)
        self.assertEqual((opcode, error.code), (10001, '90001'))

    def test_python_account_and_active_fight_migrate(self):
        self.stop()
        store = Store(self.path)
        row = store.login('python-migration', 'local-test-password')
        store.create_player(row['id'], '迁移前角色', 'hero_043')
        gameplay = Gameplay(store)
        gameplay.enter(row['id'], '1', '1')
        saved = gameplay.begin(row['id'], '1', '1', '1')
        self.start()
        login = self.login('python-migration')
        self.assertEqual(login.playerId, row['id'])
        _, resumed = self.send('CSCopyFightStart', {'chapterId':'1','missionId':'1','taskId':'1'}, login)
        self.assertEqual(resumed, message('SCCopyFightStart', **saved))
        _, end = self.send('CSCopyFightEnd', {'fightId':resumed.fightId,'result':1,'battlePlayer':resumed.battlePlayer}, login)
        self.assertEqual(end.isCopyComplete, 0)
        self.stop(); self.start()
        login = self.login('python-migration')
        _, entry = self.send('CSCopyEnter', {'chapterId':'1','missionId':'1'}, login)
        self.assertEqual([job.taskId for job in entry.jobList], ['4'])

    def test_parallel_settlement_and_invalid_hp_do_not_duplicate_rewards(self):
        login = self.login('concurrent')
        self.send('CSPlayerCreate', {'playerName':'并发测试'}, login)
        self.send('CSCopyEnter', {'chapterId':'1','missionId':'1'}, login)
        for task in ('1', '4'):
            _, fight = self.send('CSCopyFightStart', {'chapterId':'1','missionId':'1','taskId':task}, login)
            invalid = message('BattlePlayer'); invalid.CopyFrom(fight.battlePlayer)
            invalid.formation[0].inst.curHp = float('nan')
            _, err = self.send('CSCopyFightEnd', {'fightId':fight.fightId,'result':1,'battlePlayer':invalid}, login)
            self.assertEqual(err.code, '90003')
            finish = {'fightId':fight.fightId,'result':1,'battlePlayer':fight.battlePlayer}
            with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
                replies = list(pool.map(lambda _: self.send('CSCopyFightEnd', finish, login), range(6)))
            for opcode, result in replies:
                self.assertEqual(opcode, BY_NAME['SCCopyFightEnd'])
                self.assertEqual(result.isCopyComplete, int(task=='4'))
        _, profile = self.send('CSPlayerLogin', login=login)
        self.assertEqual((profile.player.coin, profile.player.level, profile.player.exp), (760,2,0))

    def test_tcp_relogin_invalidates_existing_connection(self):
        login = self.login('tcp-rotation')
        def packet(name, body):
            payload = message(name, **body).SerializeToString()
            return struct.pack('>iiiiqi', 24+len(payload),1,BY_NAME[name],1,login.playerId,len(payload))+payload
        with socket.create_connection(('127.0.0.1',self.service.tcp_port),timeout=3) as sock:
            with sock.makefile('rb') as reader:
                def receive():
                    header = reader.read(8)
                    size, _ = struct.unpack('>ii',header)
                    return decode_responses(header+reader.read(size))[0]
                sock.sendall(packet('CSSocketLogin', {'sessionKey':login.sessionKey}))
                self.assertEqual(receive()[1].result,100)
                self.login('tcp-rotation')
                sock.sendall(packet('CSPingTcp', {'clientTime':1}))
                self.assertEqual(receive()[1].code,'90002')

    def test_guide_recruit_and_formation_survive_retry_and_restart(self):
        early=self.login('too-early')
        self.send('CSPlayerCreate',{'playerName':'未通关'},early)
        _, denied=self.send('CSWarriorRecruitGuide',login=early)
        self.assertEqual(denied.code,'90003')
        self.test_first_mission_state_replay_and_restart()
        login=self.login('pve-test')
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            replies=list(pool.map(lambda _: self.send('CSWarriorRecruitGuide',login=login),range(4)))
        hero=replies[0][1].get.wars[0]
        self.assertEqual(hero.id,'hero_025')
        for opcode,result in replies:
            self.assertEqual(opcode,BY_NAME['SCWarriorRecruitGuide'])
            self.assertEqual(result.get.wars[0],hero)
        # A manual tutorial claim advances the native button without a forced guide.
        self.send('CSWarriorRecruitGuide', login=login)
        self.assertEqual(self.last_frames[1][0], BY_NAME['SCPlayer'])
        self.assertEqual(self.last_frames[1][1].player.noviceGuideSchedule, 2)
        _, heroes=self.send('CSWarriorList',login=login)
        self.assertEqual(len([w for w in heroes.warriorList if not w.id.startswith("hero_exp_")]),2)
        _, denied=self.send('CSFormationOn',{'formatPos':2,'id':hero.id,'guid':'foreign-hero'},login)
        self.assertEqual(denied.code,'90003')
        fields={'formatPos':2,'id':hero.id,'guid':hero.guid}
        for _ in range(2):
            opcode, reply=self.send('CSFormationOn',fields,login)
            self.assertEqual(opcode,BY_NAME['SCFormationOn'])
            self.assertEqual(reply.formationItem.things[0].guid,hero.guid)
        self.stop();self.start();login=self.login('pve-test')
        _, formation=self.send('CSFormationList',login=login)
        self.assertEqual(len(formation.formationList),2)
        _, profile=self.send('CSPlayerLogin',login=login)
        self.assertEqual(profile.player.noviceGuideSchedule,3)
        self.send('CSCopyEnter',{'chapterId':'1','missionId':'1'},login)
        _, battle=self.send('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'1'},login)
        self.assertEqual(len(battle.battlePlayer.formation),2)

    def test_legacy_experience_after_leaving_mission(self):
        self.test_legacy_missing_experience_is_repaired_once()
        store = Store(self.path)
        with store.connect() as conn:
            row = conn.execute('SELECT * FROM profiles WHERE uid=?',('legacy-exp',)).fetchone()
            p,g=json.loads(row['player']),json.loads(row['game'])
            p.update(level=1,exp=0,stamina=100,energy=100,noviceGuideSchedule=0)
            g.pop('run',None)
            conn.execute('UPDATE profiles SET player=?,game=? WHERE id=?',(json.dumps(p),json.dumps(g),row['id']))
        self.start()
        login=self.login('legacy-exp')
        _, result=self.send('CSPlayerLogin',login=login)
        self.assertEqual((result.player.level,result.player.coin,result.player.noviceGuideSchedule),(2,50760,1))

    def test_all_login_queries_encode(self):
        login = self.login('initialization')
        self.send('CSPlayerCreate', {'playerName':'初始化测试'}, login)
        names = ['CSWarriorList','CSFormationList','CSItemList','CSQuestList','CSKVInfo',
                 'CSCopyPassedInfo','CSCopyCurrentInfo','CSGetUnionAddBaseProp','CSShopProductList',
                 'CSEquipmentTreasureList','CSGemAndCookingList','CSFragmentList','CSTitleList',
                 'CSPetList','CSChangeNameTimes','CSFriendList','CSGetUnionInfo','CSCookingLevel',
                 'CSRedPoint','CSFriendBlackList','CSNavigationItem','CSKFLogin','CSChatCache',
                 'CSCurrentLuckyBagsInfo','CSLoginRewardInfo','CSNoticeUrl','CSOpenStateCheck',
                 'CSUFInfoPreView','CSKFOpenFrightServerInfo','CSGetRebels','CSCookingEffectList',
                 'CSLoginScene','CSCurrentPosition','CSBigClientReward']
        for name in names:
            with self.subTest(message=name):
                opcode, result = self.send(name, login=login)
                self.assertEqual(opcode, BY_NAME['SC'+name[2:]], str(result))

    def test_legacy_missing_experience_is_repaired_once(self):
        self.stop()
        store = Store(self.path)
        row = store.login('legacy-exp', 'local-test-password')
        store.create_player(row['id'], '旧版经验修复')
        gameplay = Gameplay(store)
        gameplay.enter(row['id'], '1', '1')
        for task in ('1','4'):
            fight = gameplay.begin(row['id'],'1','1',task)
            gameplay.settle(row['id'], message('CSCopyFightEnd',fightId=fight['fightId'],result=1,battlePlayer=fight['battlePlayer']))
        gameplay.quest_reward(row['id'],'quest_1')
        # Reproduce the exact early implementation: coin settled, experience absent.
        with store.connect() as conn:
            row = conn.execute('SELECT * FROM profiles WHERE uid=?',('legacy-exp',)).fetchone()
            p,g=json.loads(row['player']),json.loads(row['game'])
            p.update(level=1,exp=0,stamina=100,energy=100)
            g['run']['fight']['result']['gain']['simps']=[{'id':'coin','num':760}]
            conn.execute('UPDATE profiles SET player=?,game=? WHERE id=?',(json.dumps(p),json.dumps(g),row['id']))
        for _ in range(2):
            self.start()
            login=self.login('legacy-exp')
            _, result=self.send('CSPlayerLogin',login=login)
            self.assertEqual((result.player.level,result.player.exp,result.player.coin,result.player.stamina),(2,0,50760,110))
            self.assertEqual(result.player.noviceGuideSchedule,1)
            self.stop()


# Prevent unittest from also collecting the imported Python-server base class.
del LiveServerTests

if __name__ == '__main__':
    unittest.main()
