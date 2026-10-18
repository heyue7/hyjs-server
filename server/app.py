"""Early compatibility server: recovered transport and local account bootstrap.

Unsupported gameplay messages explicitly fail. This is not a playable server yet.
"""
import argparse
from dataclasses import replace
import json
import logging
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import socketserver
import struct
import threading
import time

from google.protobuf.message import DecodeError

from .codec import (BY_NAME, MAX_PACKET, OPCODES, encode_response, message,
                    parse_http, parse_tcp)
from .store import Store
from .gameplay import Gameplay

LOG = logging.getLogger("hyjs")


class Service:
    def __init__(self, db, advertise_host="127.0.0.1", http_port=18080, tcp_port=19090):
        self.store = Store(db)
        self.gameplay = Gameplay(self.store)
        self.host, self.http_port, self.tcp_port = advertise_host, http_port, tcp_port

    def dispatch(self, request, peer="127.0.0.1", transport="http"):
        name = OPCODES.get(request.opcode, "unknown")
        LOG.info("%s opcode=%s message=%s player=%s", transport, request.opcode, name, request.player_id)
        try:
            body = request.decode()
            if name == "CSCheckVersion":
                return self.reply("SCCheckVersion", serverAppVersion=body.appVersion,
                                  serverResVersion=body.resVersoin, programMethod=0, resMethod=0,
                                  entry=[{"key":"needActivation", "value":"0"}])
            if name == "CSGetServerList":
                row = self.store.find(body.platformUid)
                recent = []
                if row and row["player"]:
                    player = json.loads(row["player"])
                    recent.append({"serverId":1, "platformUid":body.platformUid, "playerId":row["id"],
                                   "playerName":player["name"], "level":player["level"],
                                   "state":"recommend", "globalServerKey":"local-1"})
                return self.reply("SCGetServerList", serverList=[{
                    "serverId":1, "name":"艾恩葛朗特·本地测试", "serverIp":f"{self.host}:{self.http_port}",
                    "state":"recommend", "globalServerKey":"local-1", "display":"1"}], playerServerList=recent)
            if name == "CSUserLogin":
                row = self.store.login(body.platformUid, body.authCode)
                return self.reply("SCUserLogin", sessionKey=row["session"], playerId=row["id"],
                                  serverTimeNow=int(time.time()*1000), isFirstCreated=row["player"] is None,
                                  ip=self.host, port=self.tcp_port, clientTime=body.clientTime, clientIp=peer)

            session = bytes(body.sessionKey) if name in ("CSSocketLogin", "CSRefreshGatewayIPAndPort") else request.session
            row = self.store.authorize(request.player_id, session)
            if request.zone_id != 1:
                raise ValueError("Only local zone 1 is implemented")
            if name == "CSGenerateName":
                return self.reply("SCGenerateName", playerName=f"归来的剑士{row['id']}")
            if name == "CSPlayerCreate":
                row = self.store.create_player(row["id"], body.playerName, body.heroId or 'hero_043')
                return self.reply("SCPlayerCreate", player=json.loads(row["player"]), createTime=row["created_at"])
            if name == "CSPlayerLogin":
                if row["player"] is None:
                    raise ValueError("Create the local character first")
                return self.reply("SCPlayerLogin", player=json.loads(row["player"]), playerCreateTime=row["created_at"])
            if name == "CSSocketLogin":
                # NetAPI.SCLoginTcpCallback compares SCSocketLogin.result with 100.
                return self.reply("SCSocketLogin", result=100)
            if name == "CSRefreshGatewayIPAndPort":
                return self.reply("SCRefreshGatewayIPAndPort", ip=self.host, port=self.tcp_port)
            if name == 'CSPingTcp':
                return self.reply('SCPingTcp', clientTime=body.clientTime, serverTime=int(time.time()*1000))
            if name == 'CSPingAckTcp':
                return b''
            if name == 'CSSpeedCheck':
                return b''  # Client timing telemetry has no corresponding SC message.
            if row['player'] and row['game']:
                player, game = json.loads(row['player']), json.loads(row['game'])
                if name == 'CSCopyEnter':
                    return self.reply('SCCopyEnter', **self.gameplay.enter(row['id'],body.chapterId,body.missionId))
                if name == 'CSCopyRunMapInfo':
                    run=game.get('run',{})
                    if (run.get('chapter'),run.get('mission'))!=(body.chapterId,body.missionId):
                        raise ValueError('No active mission')
                    return self.reply('SCCopyRunMapInfo', passedTaskIdList=run['passed'],
                                      battlePlayer=self.gameplay.player(row,game))
                if name == 'CSCopyFightStart':
                    return self.reply('SCCopyFightStart', **self.gameplay.begin(row['id'],body.chapterId,body.missionId,body.taskId))
                if name == 'CSCopyFightEnd':
                    result=self.gameplay.settle(row['id'],body)
                    # HTTPManager routes extra SCPlayer frames to RefreshData.
                    # Client AddSimpleItem intentionally ignores experience.
                    updated=self.store.authorize(row['id'],session)
                    return (self.reply('SCCopyFightEnd', **result)
                            +self.reply('SCPlayer',player=json.loads(updated['player'])))
                if name == 'CSCopyLeave':
                    self.gameplay.leave(row['id'])
                    return self.reply('SCCopyLeave')
                if name == 'CSQuestReward':
                    return self.reply('SCQuestReward', **self.gameplay.quest_reward(row['id'],body.questId))
                if name == 'CSCopyCurrentInfo':
                    return self.reply('SCCopyCurrentInfo',currentChapterId=game['copy']['currentChapterId'],
                                      currentMissionId=game['copy']['currentMissionId'])
                if name == 'CSMove':
                    self.store.move(row['id'], {k:getattr(body.pointNew,k) for k in ('x','y','z')})
                    return b''  # Single-player town; no other peers to notify yet.
                if name == 'CSWarriorList':
                    return self.reply('SCWarriorList', warriorList=game['warriors'])
                if name == 'CSFormationList':
                    return self.reply('SCFormationList', formationList=game['formation'])
                if name == 'CSItemList':
                    return self.reply('SCItemList', itemList=game['items'])
                if name == 'CSCopyPassedInfo':
                    return self.reply('SCCopyPassedInfo', **game['copy'])
                if name == 'CSQuestList':
                    return self.reply('SCQuestList', questList=game['quests'])
                if name == 'CSKVInfo':
                    return self.reply('SCKVInfo', entry=game['kv'])
                if name == 'CSGetUnionAddBaseProp':
                    return self.reply('SCGetUnionAddBaseProp', unionAddBaseProp={}, unionPrayProp={})
                if name == 'CSShopProductList':
                    return self.reply('SCShopProductList', shopType=body.shopType)
                if name == 'CSBigClientReward':
                    self.store.confirm_full_client(row['id'])
                    return self.reply('SCBigClientReward')
                if name in ('CSLoginScene', 'CSCurrentPosition'):
                    hero = next(h for h in game['warriors'] if h['guid'] == player['heroWarId'])
                    return self.reply('SC'+name[2:], **{'self':{
                        'info':{'serverId':1, 'playerId':row['id'], 'playerName':player['name'],
                                'level':player['level'], 'warId':hero['id'], 'serverName':'本地测试',
                                'playerSide':{'playerId':row['id'], 'kills':0}},
                        'position':game['position']}, 'limit':100})
                # New local characters own no equipment/pets/titles/friends and have no guild.
                # These are inventory queries only; mutations and unsupported gameplay still fail.
                empty_queries = {'CSEquipmentTreasureList','CSGemAndCookingList','CSFragmentList',
                                 'CSTitleList','CSPetList','CSChangeNameTimes','CSFriendList',
                                 'CSGetUnionInfo','CSCookingLevel','CSRedPoint',
                                 'CSFriendBlackList','CSNavigationItem','CSKFLogin','CSChatCache',
                                 'CSCurrentLuckyBagsInfo','CSLoginRewardInfo','CSNoticeUrl','CSOpenStateCheck',
                                 'CSUFInfoPreView','CSKFOpenFrightServerInfo','CSGetRebels','CSCookingEffectList'}
                if name in empty_queries:
                    return self.reply('SC'+name[2:])
            return self.error("NOT_IMPLEMENTED", f"尚未实现：{name} ({request.opcode})")
        except PermissionError:
            return self.error("LOCAL_SESSION_INVALID", "本地会话无效，请重新登录")
        except (ValueError, DecodeError) as exc:
            return self.error("LOCAL_BAD_REQUEST", str(exc))

    @staticmethod
    def reply(name, **fields):
        return encode_response(BY_NAME[name], message(name, **fields))

    def error(self, code, text):
        # Unity's socket event dispatcher parses this string as Int32.
        codes = {'NOT_IMPLEMENTED':'90001', 'LOCAL_SESSION_INVALID':'90002',
                 'LOCAL_BAD_REQUEST':'90003', 'LOCAL_PACKET_INVALID':'90004'}
        return self.reply("ErrorMessage", code=codes[code], msg=f'{code}: {text}')


def http_server(service, host, port):
    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def setup(self):
            super().setup()
            self.connection.settimeout(15)

        def log_message(self, fmt, *args):
            pass  # Opcode-only service logging avoids retaining credentials or protobuf bodies.

        def send_body(self, status, payload, content_type="application/octet-stream"):
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

        def do_GET(self):
            if self.path == "/health":
                self.send_body(200, b'{"status":"ok","stage":"bootstrap","playable":false}', "application/json")
            else:
                self.send_body(404, b"Not found")

        def do_POST(self):
            if self.path not in ("/sao", "/sao/"):
                self.close_connection = True
                self.send_body(404, b"Not found")
                return
            try:
                size = int(self.headers.get("Content-Length", "0"))
                if self.headers.get("Transfer-Encoding") or not 68 <= size <= MAX_PACKET:
                    raise ValueError("Invalid Content-Length or transfer encoding")
                raw = self.rfile.read(size)
                response = service.dispatch(parse_http(raw), self.client_address[0])
                self.send_body(200, response)
            except (ValueError, DecodeError) as exc:
                self.close_connection = True
                self.send_body(400, service.error("LOCAL_PACKET_INVALID", str(exc)))
            except (TimeoutError, ConnectionError):
                self.close_connection = True

    server = ThreadingHTTPServer((host, port), Handler)
    service.http_port = server.server_address[1]
    return server


def tcp_server(service, host, port):
    class Handler(socketserver.StreamRequestHandler):
        timeout = 30

        def handle(self):
            authenticated = None
            try:
                while True:
                    prefix = self.rfile.read(4)
                    if not prefix:
                        return
                    if len(prefix) != 4:
                        return
                    size = struct.unpack(">i", prefix)[0]
                    if not 24 <= size <= MAX_PACKET-4:
                        return
                    raw = self.rfile.read(size)
                    if len(raw) != size:
                        return
                    request = parse_tcp(prefix+raw)
                    if authenticated and authenticated[0] == request.player_id:
                        request = replace(request, session=authenticated[1])
                    response = service.dispatch(request, self.client_address[0], "tcp")
                    if request.opcode == BY_NAME["CSSocketLogin"] and struct.unpack_from(">i", response, 4)[0] == BY_NAME["SCSocketLogin"]:
                        authenticated = (request.player_id, bytes(request.decode().sessionKey))
                    self.wfile.write(response)
                    self.wfile.flush()
            except (ValueError, TimeoutError, ConnectionError):
                return

    class Server(socketserver.ThreadingTCPServer):
        allow_reuse_address = True
        daemon_threads = True

    server = Server((host, port), Handler)
    service.tcp_port = server.server_address[1]
    return server


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bind", default="127.0.0.1")
    parser.add_argument("--advertise-host", default="127.0.0.1")
    parser.add_argument("--http-port", type=int, default=18080)
    parser.add_argument("--tcp-port", type=int, default=19090)
    parser.add_argument("--db", default="server/data/local.sqlite3")
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(message)s")
    service = Service(args.db, args.advertise_host, args.http_port, args.tcp_port)
    tcp = tcp_server(service, args.bind, args.tcp_port)
    try:
        http = http_server(service, args.bind, args.http_port)
    except OSError:
        tcp.server_close()
        raise
    threading.Thread(target=tcp.serve_forever, daemon=True).start()
    LOG.info("Bootstrap server HTTP=%s:%s TCP=%s:%s; gameplay pending", args.bind, service.http_port, args.bind, service.tcp_port)
    try:
        http.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        http.server_close()
        tcp.shutdown()
        tcp.server_close()


if __name__ == "__main__":
    main()
