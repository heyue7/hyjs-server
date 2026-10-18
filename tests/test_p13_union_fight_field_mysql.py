"""P13场景：专属InnoDB、真实原HTTP/TCP及固定测试时钟，不连接正式数据库。"""
import os
from pathlib import Path
import re
import secrets
import selectors
import shutil
import subprocess
import types
import unittest
from datetime import datetime, timedelta
from zoneinfo import ZoneInfo
import test_p13_guild_room_mysql as room
import test_p13_mysql as guild
from server.codec import BY_NAME

ROOT = Path(__file__).resolve().parents[1]
SOURCE = Path(os.environ.get('HYJS_P13_FIELD_SOURCE_ROOT', str(ROOT))).resolve()

@unittest.skipUnless(os.environ.get('HYJS_P13_FIELD_MYSQL_TEST') == '1', '显式启用P13场景专属MySQL')
class P13UnionFightFieldMySQLTests(unittest.TestCase):
    setUp = guild.P13MySQLTests.setUp
    stop = guild.P13MySQLTests.stop
    login = guild.P13MySQLTests.login
    call = guild.P13MySQLTests.call
    stamped = guild.P13MySQLTests.stamped
    profile = guild.P13MySQLTests.profile
    send = guild.P13MySQLTests.send
    recv = guild.P13MySQLTests.recv
    tcp_send = guild.P13MySQLTests.tcp_send
    connect = guild.P13MySQLTests.connect
    drain = room.P13GuildRoomMySQLTests.drain
    call_all = room.P13GuildRoomMySQLTests.call_all
    sql = guild.P13MySQLTests.sql
    root_sql = classmethod(guild.P13MySQLTests.root_sql.__func__)

    @classmethod
    def setUpClass(cls):
        guild.P13MySQLTests.setUpClass.__func__(cls)
        if SOURCE != ROOT and not SOURCE.name.startswith('hyjs-p13-field-'):
            raise AssertionError('P13 field source isolation guard')

    def set_clock(self, at):
        temporary = self.clock.with_suffix('.next')
        temporary.write_text(str(at))
        os.replace(temporary,self.clock)

    def start(self):
        self.process = subprocess.Popen([self.binary, '--http-port', '0', '--tcp-port', '0', '--tables', str(SOURCE / 'analysis/tables'), '--log-file', str(Path(self.temp.name) / 'field.log')], cwd=self.native_root / 'hyjs-server', env=self.env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), '场景原帧进程启动超时')
            line = self.process.stdout.readline()
        ports = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(ports, line.replace(self.test_password, '[redacted]'))
        self.service = types.SimpleNamespace(http_port=int(ports[1]), tcp_port=int(ports[2]))

    def receive(self, sock, name):
        for _ in range(80):
            op, value = self.recv(sock)
            self.assertNotEqual(op, 10001, str(value))
            if op == BY_NAME[name]:
                return value
        self.fail('场景原帧响应队列超限')

    def test_1_mysql_race_actual25_map_legacy_sha_movement_restart_end(self):
        args = ['rtk', 'proxy', 'go', 'test', '-race', './service', '-run', '^TestP13UnionFightFieldActual25', '-count=1', '-timeout=180s']
        result = subprocess.run(args, cwd=SOURCE / 'hyjs-server', env=self.env, capture_output=True, text=True, timeout=210)
        self.assertEqual(result.returncode, 0, (result.stdout + result.stderr).replace(self.test_password, '[redacted]'))
        print(result.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='" + self.database + "' AND TABLE_NAME LIKE 'p13_union_fight_field%';").strip(), 'InnoDB')

    def test_2_native_original_frames_private_instances_relogin_end_restart(self):
        # 同一suite复用专库；清除前一Go批的匹配档案，避免把25人测试的日历带入原帧6会测试。
        tables = self.sql("SHOW TABLES LIKE 'p13_union_fight_%';").splitlines()
        if tables:
            self.sql(' '.join('DELETE FROM `' + table + '`;' for table in tables if table not in ['p13_union_fight_policies', 'p13_union_fight_field_policies']))
            self.sql('UPDATE p13_union_fight_field_policies SET enabled=0; UPDATE p13_union_fight_policies SET enrollment_enabled=0,match_enabled=0;')
        # 测试专属编译副本注入文件时钟并接共享生命周期；原项目main没有时钟文件入口。
        prebuilt = os.environ.get('HYJS_P13_FIELD_PREBUILT_NATIVE')
        if prebuilt:
            # 本域已在锁外按完整源SHA编译；锁内验证源闭包并只运行，不临时混入其他代理工作树。
            import json,hashlib
            prepared=Path(prebuilt)
            self.assertTrue(prepared.name.startswith('hyjs-p13-prebuilt-'),'prebuilt isolation guard')
            manifest=json.loads((prepared/'manifest.json').read_text())
            for relative,digest in manifest['files'].items():
                self.assertEqual(hashlib.sha256((SOURCE/relative).read_bytes()).hexdigest(),digest,'prebuilt source drift '+relative)
            self.native_root=prepared/'native-source';self.binary=str(prepared/'hyjs-native')
            self.assertEqual(hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),manifest['native_sha256'])
        else:
            self.native_root = Path(self.temp.name) / 'native-source'
            shutil.copytree(SOURCE / 'hyjs-server', self.native_root / 'hyjs-server')
            main_path = self.native_root / 'hyjs-server/main.go'
            main = main_path.read_text()
            main = main.replace('"hyjs-server/config"', '"hyjs-server/config"\n"hyjs-server/constUtil"\n"strings"', 1)
            marker = 'defer cancel()\n'
            self.assertEqual(main.count(marker), 1)
            main = main.replace(marker, marker + '''
        // 仅本测试编译副本：受专属库名称保护的服务器内部可控时钟。
        if !strings.HasPrefix(os.Getenv("HYJS_MYSQL_DATABASE"), "hyjs_test_p13_") { return errors.New("native field isolation guard") }
        ctx = context.WithValue(ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time {
            raw, err := os.ReadFile(os.Getenv("HYJS_P13_FIELD_NATIVE_CLOCK"))
            if err != nil { panic(err) }
            at, err := strconv.ParseInt(string(raw), 10, 64)
            if err != nil { panic(err) }
            return time.UnixMilli(at)
        })
    ''', 1)
            main = main.replace('errs := make(chan error, 6)', 'errs := make(chan error, 7)').replace('remaining := 6', 'remaining := 7')
            if 'errs <- s.ServeP13UnionFightField(ctx)' not in main:
                main = main.replace('go func() { errs <- s.ServeP13UnionFight(ctx) }()', 'go func() { errs <- s.ServeP13UnionFight(ctx) }()\ngo func() { errs <- s.ServeP13UnionFightField(ctx) }()', 1)
            main_path.write_text(main)
            scene_path = self.native_root / 'hyjs-server/service/p10SceneService.go'
            scene = scene_path.read_text()
            if 'if handled, err := s.SyncP13UnionFightField(ctx, id, movement);' not in scene:
                scene = scene.replace('if handled, err := s.SyncP13GuildRoom(ctx, id, movement);', 'if handled, err := s.SyncP13UnionFightField(ctx, id, movement); handled || err != nil { return err }\nif handled, err := s.SyncP13GuildRoom(ctx, id, movement);', 1)
            if 'if s.DisconnectP13UnionFightField(ctx, id, session)' not in scene:
                scene = scene.replace('if s.DisconnectP13GuildRoom(ctx, id, session)', 'if s.DisconnectP13UnionFightField(ctx, id, session) { return }\nif s.DisconnectP13GuildRoom(ctx, id, session)', 1)
            scene_path.write_text(scene)
            handler_path = self.native_root / 'hyjs-server/handler/gameHandler.go'
            handler = handler_path.read_text().replace('b.GetTarget() != constUtil.P13GuildRoomTarget {', 'b.GetTarget() != constUtil.P13GuildRoomTarget && b.GetTarget() != constUtil.P13UnionFightFieldTarget {', 1)
            self.assertIn('b.GetTarget() != constUtil.P13UnionFightFieldTarget', handler, '场景原转场白名单未接入')
            handler_path.write_text(handler)
            result = subprocess.run(['rtk', 'proxy', 'gofmt', '-w', str(main_path), str(scene_path), str(handler_path)], capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            result = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', self.binary, '.'], cwd=self.native_root / 'hyjs-server', capture_output=True, text=True, timeout=180)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.clock = Path(self.temp.name) / 'clock'
        monday = datetime(2026, 10, 19, 12, tzinfo=ZoneInfo('Asia/Shanghai'))
        next_day = monday.replace(hour=0) + timedelta(days=1)
        first = int((next_day + timedelta(hours=20)).timestamp() * 1000)
        self.set_clock(int(monday.timestamp() * 1000))
        self.env['HYJS_P13_FIELD_NATIVE_CLOCK'] = str(self.clock)
        self.start()
        chat_test = os.environ.get('HYJS_P13_CHAT_NATIVE_TEST') == '1'
        focus_test = os.environ.get('HYJS_P13_FOCUS_NATIVE_TEST') == '1' or chat_test
        uids = ['p13-field-frame-' + secrets.token_hex(4) for _ in range(12 if focus_test else 6)]
        players = [self.login(uid) for uid in uids]
        for i, login in enumerate(players):
            self.call('CSPlayerCreate', {'playerName': '场景原帧' + str(i), 'heroId': 'hero_047'}, login, 1, 'SCPlayerCreate')
        ids = ','.join(str(x.playerId) for x in players)
        self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",30,"$.diamond",1000) WHERE id IN (' + ids + '); UPDATE p13_guild_policies SET management_enabled=1; UPDATE p13_union_fight_policies SET enrollment_enabled=1,match_enabled=0; UPDATE p10_scene_policies SET enabled=0;')
        guild_ids = []
        for i, login in enumerate(players[:6]):
            self.call('CSUnionCreate', {'name': '场景实会' + str(i)}, login, 2, 'SCUnionCreate')
            gid = self.call('CSGetUnionInfo', {}, login, 3, 'SCGetUnionInfo').unionInfo.unionId
            guild_ids.append(gid)
            self.sql("UPDATE p13_guilds SET level=2 WHERE id='" + gid + "';")
            if focus_test:
                self.call('CSUnionApply', {'unionId': gid}, players[i+6], 2, 'SCUnionApply')
                self.call('CSUnionAgreeApply', {'playerId': players[i+6].playerId}, login, 7, 'SCUnionAgreeApply')
                if chat_test:
                    self.call('CSPromotionViceChairman', {'playerId': players[i+6].playerId}, login, 8, 'SCPromotionViceChairman')
            self.call('CSUFUnionApply', {}, login, 4, 'SCUFUnionApply')
            self.call('CSUFPlayerApply', {}, login, 5, 'SCUFPlayerApply')
            if focus_test:
                self.call('CSUFPlayerApply', {}, players[i+6], 5, 'SCUFPlayerApply')
        self.stop()
        self.sql("INSERT INTO p13_union_fight_schedules(day,first_start_at,second_start_at,duration_millis,source) VALUES('2026-10-20'," + str(first) + ',' + str(first + 600000) + ",300000,'显式隔离场景测试日历'); UPDATE p13_union_fight_policies SET match_enabled=1; UPDATE p13_union_fight_field_policies SET enabled=1,preparation_millis=300000,left_revive_point_id=1,right_revive_point_id=3,initial_guard_hp=1000,initial_gate_hp=10000,move_tolerance=3,source='显式本地测试HP与原复活点，无权威结算';")
        env = dict(self.env, HYJS_P13_FIELD_NATIVE_ADVANCE=str(int(next_day.timestamp() * 1000)))
        if prebuilt:
            race_binary=prepared/'service-race.test'
            self.assertEqual(hashlib.sha256(race_binary.read_bytes()).hexdigest(),manifest['race_sha256'])
            advance_command=['rtk','proxy',str(race_binary),'-test.run','^TestP13UnionFightFieldAdvanceIsolatedNativeClock$','-test.count=1','-test.timeout=120s']
        else:
            advance_command=['rtk','proxy','go','test','./service','-run','^TestP13UnionFightFieldAdvanceIsolatedNativeClock$','-count=1','-timeout=120s']
        result = subprocess.run(advance_command, cwd=SOURCE / ('hyjs-server/service' if prebuilt else 'hyjs-server'), env=env, capture_output=True, text=True, timeout=150)
        self.assertEqual(result.returncode, 0, (result.stdout + result.stderr).replace(self.test_password, '[redacted]'))
        print(result.stdout.strip())
        pair = self.sql('SELECT CONCAT(left_guild_id,",",right_guild_id) FROM p13_union_fight_matches WHERE round=1 ORDER BY id LIMIT 1;').strip().split(',')
        other = self.sql("SELECT left_guild_id FROM p13_union_fight_matches WHERE round=1 AND left_guild_id NOT IN ('" + pair[0] + "','" + pair[1] + "') ORDER BY id LIMIT 1;").strip()
        selected = [guild_ids.index(gid) for gid in pair + [other]]
        self.set_clock(first - 60000)
        item_test = os.environ.get('HYJS_P13_ITEM_NATIVE_TEST') == '1'
        resource_test = os.environ.get('HYJS_P13_RESOURCE_NATIVE_TEST') == '1'
        shop_test = os.environ.get('HYJS_P13_SHOP_NATIVE_TEST') == '1'
        factory_test = os.environ.get('HYJS_P13_FACTORY_NATIVE_TEST') == '1'
        if resource_test:
            self.sql("INSERT INTO p13_union_fight_resource_sources(day,source_key,enabled,resource_id,x,y,z,reward_res,published_at,source) VALUES('2026-10-20','native-left',1,'1',-4.7,-0.12,9.98,2," + str(first-60000) + ",'隔离显式发布：原矿型1，本地位置原NPC2，不声称原spawn'),('2026-10-20','native-right',1,'1',80.22,-0.33,-13.36,2," + str(first-60000) + ",'隔离显式发布：原矿型1，本地位置原NPC5，不声称原spawn');")
        if shop_test:
            from test_p13_union_fight_shop_mysql import publish_shop_sources
            publish_shop_sources(self, first)
        if factory_test:
            from test_p13_union_fight_factory_mysql import publish_factory_sources
            publish_factory_sources(self, first)
        if item_test: self.publish_item_sources(first)
        self.start()
        a, b, c = [self.login(uids[i]) for i in selected]
        sa, sb, sc = [self.connect(x) for x in [a, b, c]]
        active = [a,b,c]
        friend = None
        if focus_test:
            friend = self.login(uids[selected[0]+6])
            friend_sock = self.connect(friend)
            active.append(friend)
        for login in active:
            self.call('CSTransferScene', {'target': 'fight_union'}, login, 10, 'SCTransferScene')
        self.drain(sa); self.drain(sb); self.drain(sc)
        if friend:
            self.drain(friend_sock)
            self.field_friend=friend;self.field_friend_sock=friend_sock
        if os.environ.get('HYJS_P13_POSITION_NATIVE_TEST') == '1':
            old_town = {'pointNew': {'x':-9.357699,'y':10.6129,'z':37.3301}}
            pending = self.profile(a)[1]
            self.assertTrue(pending.get('position_loading',False))
            initial = dict(pending['position']['current'])
            self.tcp_send(sa,'CSMove',old_town,a)
            self.tcp_send(sa,'CSLoginScene',{},a)
            login_frame = self.receive(sa,'SCLoginScene')
            self.assertEqual(login_frame.self.position.scene,'fight_union')
            self.assertEqual(self.profile(a)[1]['position']['current'],initial)
            self.assertTrue(self.profile(a)[1].get('position_loading',False))
            self.tcp_send(sa,'CSMoveConfirm',{},a)
            self.receive(sa,'SCUnionFightTransfer')
            self.assertTrue(self.profile(a)[1].get('position_loading',False))
            self.stop(); self.start()
            a,b,c = [self.login(uids[i]) for i in selected]
            sa,sb,sc = [self.connect(x) for x in [a,b,c]]
            self.assertTrue(self.profile(a)[1].get('position_loading',False))
            self.tcp_send(sa,'CSMove',old_town,a)
            self.tcp_send(sa,'CSLoginScene',{},a)
            self.receive(sa,'SCLoginScene')
            self.assertTrue(self.profile(a)[1].get('position_loading',False))
            self.assertEqual(self.profile(a)[1]['position']['current'],initial)
            self.tcp_send(sa,'CSCurrentPosition',{},a)
            self.receive(sa,'SCCurrentPosition')
            self.receive(sa,'SCUnionFightTransfer')
            self.assertFalse(self.profile(a)[1].get('position_loading',False))
            self.drain(sa);self.drain(sb);self.drain(sc)
        frames = self.call_all('CSCurrentPosition', {}, a, 11)
        position = next(v for op, v in frames if op == BY_NAME['SCCurrentPosition'])
        full = next(v for op, v in frames if op == BY_NAME['SCUnionFightTransfer'])
        self.assertEqual((position.self.position.scene, position.limit, full.state, full.leftTime), ('fight_union', 50, 1, 60000))
        self.assertEqual([len(full.baseCampInfos), len(full.gateInfos), len(full.towerInfos), len(full.factoryInfos), len(full.revivePointInfos)], [2, 8, 6, 4, 7])
        self.tcp_send(sa, 'CSUFInit', {}, a)
        self.assertEqual(self.receive(sa, 'SCUnionFightTransfer'), full)
        self.tcp_send(sa, 'CSUFRefreshElementArray', {'guid': []}, a)
        visible = self.receive(sa, 'SCUFPlayer')
        self.assertEqual({x.guid for x in visible.player}, {x.playerId for x in [a,b]+([friend] if friend else [])})
        self.drain(sa); self.drain(sb); self.drain(sc)
        if item_test:
            return self.check_item_native(a,b,c,sa,sb,sc,uids,selected,first,full)
        if chat_test:
            return self.check_chat_native(a,b,c,friend,sa,sb,sc,friend_sock,uids,selected,first)
        if focus_test:
            import concurrent.futures
            point_request = {'pos': {'x': 200, 'y': 30, 'z': 0}}
            op, _ = self.stamped('CSUFLeaderFocus', point_request, friend, 16)
            self.assertEqual(op,10001)
            focus = self.call('CSUFLeaderFocus', point_request, a, 16, 'SCUFLeaderFocus')
            self.assertEqual((focus.pos[0].x,focus.pos[0].y,focus.lastTime),(200,30,first+60000))
            self.assertEqual(self.receive(friend_sock,'SCUFLeaderFocus'),focus)
            self.assertFalse(any(op==BY_NAME['SCUFLeaderFocus'] for op,_ in self.drain(sb)))
            self.assertFalse(any(op==BY_NAME['SCUFLeaderFocus'] for op,_ in self.drain(sc)))
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                retries=list(pool.map(lambda _:self.call('CSUFLeaderFocus',point_request,a,16,'SCUFLeaderFocus'),range(8)))
            self.assertTrue(all(reply==focus for reply in retries))
            self.assertFalse(any(op==BY_NAME['SCUFLeaderFocus'] for op,_ in self.drain(friend_sock)))
            self.tcp_send(friend_sock,'CSUFLeaderFocusInfo',{},friend)
            self.assertEqual(self.receive(friend_sock,'SCUFLeaderFocus'),focus)
            op,_=self.stamped('CSUFLeaderFocus',{'pos':{'x':201,'y':30,'z':0}},a,17)
            self.assertEqual(op,10001)
        self.set_clock(first - 59000)
        point = position.self.position.current
        target = {'x': point.x + 1, 'y': point.y, 'z': point.z}
        self.tcp_send(sa, 'CSMove', {'pointNew': target, 'currentTime': 1700000000000}, a)
        self.receive(sa, 'SCUFElementMove')
        moved = self.receive(sb, 'SCUFElementMove')
        self.assertEqual(list(moved.guid), [a.playerId])
        self.assertFalse(any(op==BY_NAME['SCUFElementMove'] for op,_ in self.drain(sc)))
        op, _ = self.stamped('CSMove', {'pointNew': {'x': point.x + 40, 'y': point.y, 'z': point.z}}, a, 12)
        self.assertEqual(op, 10001)
        self.call('CSUFOnHook', {'onHook': 1}, a, 13, 'SCUFOnHook')
        self.drain(sa); self.drain(sb)
        self.call('CSUFOnHook', {'onHook': 0}, a, 14, 'SCUFOnHook')
        self.drain(sa); self.drain(sb)
        fresh = self.login(uids[selected[0]])
        new_sa = self.connect(fresh)
        self.drain(new_sa); self.drain(sb); self.drain(sa)
        self.tcp_send(sa, 'CSUFInit', {}, a)
        self.assertEqual(self.recv(sa)[0], 10001)
        self.tcp_send(new_sa, 'CSUFInit', {}, fresh)
        recovered = self.receive(new_sa, 'SCUnionFightTransfer')
        self.assertAlmostEqual(recovered.playerPos.x, target['x'], places=4)
        self.assertEqual(recovered.player.fightInfo, full.player.fightInfo)
        self.stop(); self.start()
        fresh = self.login(uids[selected[0]])
        new_sa = self.connect(fresh)
        self.tcp_send(new_sa, 'CSUFInit', {}, fresh)
        persisted = self.receive(new_sa, 'SCUnionFightTransfer')
        self.assertEqual(persisted.player, recovered.player)
        self.assertEqual(persisted.playerPos, recovered.playerPos)
        if focus_test:
            self.tcp_send(new_sa,'CSUFLeaderFocusInfo',{},fresh)
            self.assertEqual(self.receive(new_sa,'SCUFLeaderFocus'),focus)
            self.set_clock(first+60001)
            self.tcp_send(new_sa,'CSUFFocusPointInfo',{},fresh)
            self.assertEqual(len(self.receive(new_sa,'SCUFFocusPointInfo').pos),0)
        if resource_test:
            from test_p13_union_fight_resource_mysql import native_resource_flow
            fresh,new_sa = native_resource_flow(self,uids[selected[0]],first,fresh,new_sa,persisted)
            if os.environ.get('HYJS_P13_TRANSFER_NATIVE_TEST') == '1':
                from test_p13_union_fight_resource_mysql import native_transfer_flow
                fresh,new_sa = native_transfer_flow(self,uids[selected[0]],first,fresh,new_sa,persisted)
        if shop_test:
            from test_p13_union_fight_shop_mysql import native_shop_flow
            fresh,new_sa = native_shop_flow(self,uids[selected[0]],first,fresh,new_sa,persisted)
        if factory_test:
            from test_p13_union_fight_factory_mysql import native_factory_flow
            fresh,new_sa = native_factory_flow(self,uids[selected[0]],first,fresh,new_sa,persisted)
        # 真正解散对手公会撤销本场资格，当前事务立即回城/消失，保留历史冻结比赛身份。
        fresh_b = self.login(uids[selected[1]])
        new_sb = self.connect(fresh_b)
        self.drain(new_sa); self.drain(new_sb)
        self.call('CSDissolveUnion', {}, fresh_b, 15, 'SCDissolveUnion')
        self.assertEqual(list(self.receive(new_sa, 'SCUFPlayerDisappear').guid), [fresh_b.playerId])
        self.receive(new_sb, 'SCUFKickPlayer')
        self.assertEqual(self.profile(fresh_b)[1]['position']['scene'], 'town_main')
        self.set_clock(first + 301000)
        self.tcp_send(new_sa, 'CSUFInit', {}, fresh)
        self.assertEqual(self.receive(new_sa, 'SCUnionFightTransfer').state, 3)
        self.set_clock(first + 304001)
        self.receive(new_sa, 'SCUFKickPlayer')
        self.assertEqual(self.profile(fresh)[1]['position']['scene'], 'town_main')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_field_players WHERE left_at=0;').strip(), '0')

if __name__ == '__main__':
    unittest.main()
