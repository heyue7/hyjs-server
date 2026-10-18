import http.client
import json
from pathlib import Path
import socket
import struct
import tempfile
import threading
import unittest

from server.app import Service, http_server, tcp_server
from server.codec import (BY_NAME, CLASSES, decode_responses, encode_http_request,
                          encode_response, message, parse_http, parse_tcp)


class PacketTests(unittest.TestCase):
    def test_recovered_descriptor_count_and_login_wire_numbers(self):
        self.assertEqual(len(CLASSES), 1265)
        descriptor = CLASSES["SCUserLogin"].DESCRIPTOR
        self.assertEqual(descriptor.fields_by_name["sessionKey"].number, 1)
        self.assertEqual(descriptor.fields_by_name["clientIp"].number, 8)
        self.assertEqual(BY_NAME["CSCheckVersion"], 20005)
        self.assertEqual(BY_NAME["SCCopyFightStart"], 54016)

    def test_fixed_http_packet(self):
        # Hand-specified layout for player 123, zone 1, timestamp 1700000000000.
        raw = bytes.fromhex(
            "0000004f 00002710 00004e25 000000000000007b 00000001 0000000b"
            "20200000000000000000000000000000"
            "13459aa42cb2d0af804d7a77ecd3cfa0 0000018bcfe56800"
            "0a04746573741203726573")
        request = parse_http(raw)
        self.assertEqual((request.player_id, request.zone_id, request.timestamp), (123, 1, 1700000000000))
        self.assertEqual(request.decode().appVersion, "test")
        self.assertEqual(request.decode().resVersoin, "res")
        self.assertEqual(request.opcode, 20005)

    def test_known_tcp_response_length_excludes_opcode(self):
        raw = encode_response(20032, message("SCSocketLogin", result=100))
        self.assertEqual(raw.hex(), "0000000200004e400864")
        self.assertEqual(decode_responses(raw)[0][1].result, 100)

    def test_invalid_lengths_signature_and_required_fields(self):
        raw = encode_http_request(20029, message("CSPlayerLogin"))
        for bad in (b"", raw[:-1], raw+b"\0", raw[:44]+b"\0"*16+raw[60:]):
            with self.assertRaises(ValueError):
                parse_http(bad)
        for bad in (b"\0", bytes.fromhex("ffffffff00004e40"), bytes.fromhex("0000000200004e4008")):
            with self.assertRaises(ValueError):
                decode_responses(bad)
        with self.assertRaises(ValueError):
            decode_responses(bytes.fromhex("0000000000004e40"))  # required result omitted

    def test_multiple_pushed_frames(self):
        a = encode_response(20012, message("SCGenerateName", playerName="桐人"))
        b = encode_response(10001, message("ErrorMessage", code="LOCAL_TEST", msg="test"))
        frames = decode_responses(a+b)
        self.assertEqual([v[0] for v in frames], [20012, 10001])
        self.assertEqual(frames[0][1].playerName, "桐人")


class LiveServerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.path = Path(self.temp.name)/"local.sqlite3"
        self.start()

    def start(self):
        self.service = Service(self.path)
        self.tcp = tcp_server(self.service, "127.0.0.1", 0)
        self.http = http_server(self.service, "127.0.0.1", 0)
        self.threads = [threading.Thread(target=s.serve_forever, daemon=True) for s in (self.http, self.tcp)]
        for t in self.threads:
            t.start()

    def stop(self):
        for s in (self.http, self.tcp):
            s.shutdown()
            s.server_close()
        for t in self.threads:
            t.join(2)

    def tearDown(self):
        self.stop()
        self.temp.cleanup()

    def send(self, name, fields=None, login=None):
        args = {"zone_id":1, "timestamp":1700000000000}
        if login:
            args.update(player_id=login.playerId, session=bytes(login.sessionKey))
        raw = encode_http_request(BY_NAME[name], message(name, **(fields or {})), **args)
        c = http.client.HTTPConnection("127.0.0.1", self.service.http_port, timeout=3)
        try:
            c.request("POST", "/sao/", body=raw, headers={"Content-Type":"application/octet-stream"})
            response = c.getresponse()
            data = response.read()
            self.assertEqual(response.status, 200, data)
            self.last_frames=decode_responses(data)
            return self.last_frames[0]
        finally:
            c.close()

    def login(self, uid="local-test"):
        opcode, response = self.send("CSUserLogin", {"platformUid":uid, "authCode":"local-test-password", "clientTime":1700000000000})
        self.assertEqual(opcode, BY_NAME["SCUserLogin"])
        return response

    def test_password_required_and_wrong_password_preserves_session(self):
        first = self.login()
        for password in ('', 'wrong-password'):
            opcode, reply = self.send('CSUserLogin', {'platformUid':'local-test', 'authCode':password})
            self.assertEqual(opcode, 10001)
            self.assertEqual(reply.code, '90003')
        self.service.store.authorize(first.playerId, first.sessionKey)
        row = self.service.store.find('local-test')
        self.assertEqual(len(row['password_salt']), 16)
        self.assertEqual(len(row['password_hash']), 32)
        self.assertNotIn(b'local-test-password', self.path.read_bytes())

    def test_bootstrap_persistence_and_session_rotation(self):
        _, version = self.send("CSCheckVersion", {"appVersion":"3.0.0.42620", "resVersoin":"42620"})
        self.assertEqual(version.serverResVersion, "42620")
        _, servers = self.send("CSGetServerList", {"platformUid":"local-test"})
        self.assertEqual(servers.serverList[0].state, "recommend")
        first = self.login()
        self.assertTrue(first.isFirstCreated)
        _, created = self.send("CSPlayerCreate", {"playerName":"归来的桐人", "heroId":"hero_001"}, first)
        self.assertEqual(created.player.name, "归来的桐人")
        self.stop()
        self.start()
        second = self.login()
        self.assertFalse(second.isFirstCreated)
        self.assertEqual(first.playerId, second.playerId)
        self.assertNotEqual(first.sessionKey, second.sessionKey)
        _, player = self.send("CSPlayerLogin", login=second)
        self.assertEqual(player.player.name, "归来的桐人")
        _, old = self.send("CSPlayerLogin", login=first)
        self.assertEqual(old.code, "90002")
        _, recent = self.send("CSGetServerList", {"platformUid":"local-test"})
        self.assertEqual(recent.playerServerList[0].playerName, "归来的桐人")

    def test_unsupported_gameplay_does_not_return_fake_success(self):
        login = self.login()
        opcode, error = self.send("CSCopyFightStart", {"chapterId":"chapter_001"}, login)
        self.assertEqual(opcode, 10001)
        self.assertEqual(error.code, "90001")

    def test_created_hero_formation_and_scene_survive_restart(self):
        login = self.login('hero-test')
        _, created = self.send('CSPlayerCreate', {'playerName':'本地剑士', 'heroId':'hero_043'}, login)
        _, heroes = self.send('CSWarriorList', login=login)
        _, formation = self.send('CSFormationList', login=login)
        self.assertEqual(heroes.warriorList[0].guid, created.player.heroWarId)
        self.assertEqual(formation.formationList[0].things[0].guid, created.player.heroWarId)
        _, scene = self.send('CSLoginScene', login=login)
        self.assertEqual(scene.self.info.warId, 'hero_043')
        self.assertEqual(scene.self.position.playerId, login.playerId)
        self.assertTrue(scene.self.position.scene.startswith('town_main'))
        self.stop()
        self.start()
        login = self.login('hero-test')
        _, persisted = self.send('CSWarriorList', login=login)
        self.assertEqual(persisted, heroes)
        _, duplicate = self.send('CSPlayerCreate', {'playerName':'不同名字', 'heroId':'hero_047'}, login)
        self.assertEqual(duplicate.player.name, '本地剑士')
        _, invalid = self.send('CSPlayerCreate', {'playerName':'无效', 'heroId':'unknown'}, login)
        self.assertEqual(invalid.code, '90003')

    def test_first_mission_state_replay_and_restart(self):
        login=self.login('pve-test')
        self.send('CSPlayerCreate',{'playerName':'冒险者','heroId':'hero_043'},login)
        _, denied=self.send('CSQuestReward',{'questId':'quest_1'},login)
        self.assertEqual(denied.code,'90003')
        _, enter=self.send('CSCopyEnter',{'chapterId':'1','missionId':'1'},login)
        self.assertEqual([x.taskId for x in enter.jobList],['1','4'])
        _, denied=self.send('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'4'},login)
        self.assertEqual(denied.code,'90003')
        for task in ('1','4'):
            args={'chapterId':'1','missionId':'1','taskId':task}
            _, fight=self.send('CSCopyFightStart',args,login)
            _, duplicate=self.send('CSCopyFightStart',args,login)
            self.assertEqual(fight,duplicate)
            finish={'fightId':fight.fightId,'result':1,'battlePlayer':fight.battlePlayer}
            bad=dict(finish,fightId='invented')
            _, denied=self.send('CSCopyFightEnd',bad,login)
            self.assertEqual(denied.code,'90003')
            _, result=self.send('CSCopyFightEnd',finish,login)
            self.assertEqual(self.last_frames[1][0],BY_NAME['SCPlayer'])
            self.assertEqual(self.last_frames[1][1].player.level,2 if task=='4' else 1)
            _, duplicate=self.send('CSCopyFightEnd',finish,login)
            self.assertEqual(result,duplicate)
            self.assertEqual(result.isCopyComplete, int(task=='4'))
            if task=='1':
                _, resume=self.send('CSCopyEnter',{'chapterId':'1','missionId':'1'},login)
                self.assertEqual(resume.taskId,'4')
                self.assertEqual([x.taskId for x in resume.jobList],['4'])
        _, profile=self.send('CSPlayerLogin',login=login)
        self.assertEqual(profile.player.coin,760)
        self.assertEqual((profile.player.level,profile.player.exp,profile.player.stamina),(2,0,getattr(self, "first_mission_stamina", 110)))
        _, reward=self.send('CSQuestReward',{'questId':'quest_1'},login)
        self.assertEqual({x.id:x.num for x in reward.get.simps}, {'item_005':50,'coin':50000,'item_373':5})
        _, denied=self.send('CSQuestReward',{'questId':'quest_1'},login)
        self.assertEqual(denied.code,'90003')
        self.stop();self.start();login=self.login('pve-test')
        _, profile=self.send('CSPlayerLogin',login=login)
        self.assertEqual(profile.player.coin,50760)
        self.assertEqual((profile.player.level,profile.player.exp),(2,0))
        _, progress=self.send('CSCopyPassedInfo',login=login)
        self.assertEqual(progress.passedCopyInfo[0].missionId,'1')
        _, quests=self.send('CSQuestList',login=login)
        self.assertEqual(list(quests.questList),['quest_1'])

    def test_tcp_session_login_with_fragmented_request(self):
        login = self.login()
        payload = message("CSSocketLogin", sessionKey=bytes(login.sessionKey)).SerializeToString()
        raw = struct.pack(">iiiiqi", 24+len(payload), 1, 20031, 1, login.playerId, len(payload))+payload
        self.assertEqual(parse_tcp(raw).decode().sessionKey, login.sessionKey)
        with socket.create_connection(("127.0.0.1", self.service.tcp_port), timeout=3) as sock:
            for part in (raw[:1], raw[1:4], raw[4:17], raw[17:]):
                sock.sendall(part)
            with sock.makefile("rb") as reader:
                header = reader.read(8)
                size, opcode = struct.unpack(">ii", header)
                reply = decode_responses(header+reader.read(size))[0][1]
                self.assertEqual(opcode, 20032)
                self.assertEqual(reply.result, 100)

    def test_http_bad_packet_and_health(self):
        c = http.client.HTTPConnection("127.0.0.1", self.service.http_port, timeout=3)
        c.request("GET", "/health")
        response = c.getresponse()
        self.assertFalse(json.loads(response.read())["playable"])
        c.request("POST", "/sao/", body=b"not a packet")
        response = c.getresponse()
        self.assertEqual(response.status, 400)
        response.read()
        c.close()


if __name__ == "__main__":
    unittest.main()
