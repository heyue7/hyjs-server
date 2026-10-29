using System;
using System.Collections;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using LitJson;
using NetWork.Packets.Factory;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // The original 30045 body and a positive HTTP stamp are durable before the
    // first send. Authentication is always taken from the current live session.
    [Serializable] sealed class PvpPending
    {
        public int version;
        public long playerId,opponentId,stamp;
        public string origin,body,bodyHash;
    }
    public sealed class PvpBridge : MonoBehaviour
    {
        const int MaxBody=1024;
        static PvpBridge instance;
        FieldWire wire;
        FieldWire.Session session;
        PvpPending pending;
        bool faulted,saveFailed,busy,loginReady,delivering,warned;
        int epoch;
        long next;
        static bool Target() { return StaticData.fieldId=="field_003"; }
        static bool SameActor(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.origin==b.origin; }
        static bool SameSession(FieldWire.Session a,FieldWire.Session b)
        { return SameActor(a,b) && a.sessionHex==b.sessionHex; }
        static void Notice(string text) { BoxManager.CreatePopupTextBox(text,0f); }
        void Warn(string text) { if (!warned) { warned=true; Notice(text); } }
        static string Key(FieldWire.Session owner,string suffix)
        {
            if (owner==null || owner.playerId<=0 || String.IsNullOrEmpty(owner.origin)) throw new InvalidOperationException("Missing PK owner");
            return "hyjs.ggo.pvp.v4."+Uri.EscapeDataString(owner.origin)+"."+owner.playerId.ToString(CultureInfo.InvariantCulture)+"."+suffix;
        }
        static string Digest(byte[] data)
        {
            using (SHA256 sha=SHA256.Create())
            {
                var result=new StringBuilder(64);
                foreach (byte b in sha.ComputeHash(data)) result.Append(b.ToString("x2",CultureInfo.InvariantCulture));
                return result.ToString();
            }
        }
        static bool Hex(string text,int length)
        {
            if (text==null || text.Length!=length) return false;
            foreach (char c in text) if (!((c>='0'&&c<='9')||(c>='a'&&c<='f'))) return false;
            return true;
        }
        static bool FightId(string value)
        {
            try { return !String.IsNullOrEmpty(value) && new Guid(value).ToString("D")==value; }
            catch (Exception) { return false; }
        }
        static byte[] Bytes(PvpPending record)
        {
            byte[] data=Convert.FromBase64String(record.body);
            if (data.Length==0 || data.Length>MaxBody || !Hex(record.bodyHash,64) || Digest(data)!=record.bodyHash)
                throw new InvalidOperationException("Invalid PK request bytes");
            var message=PBConvert.toObject(30045,data) as CSSidePvpFightResult;
            if (message==null || message.playerId!=record.opponentId || message.fightType<0 || message.fightType>1)
                throw new InvalidOperationException("PK target differs from request");
            return data;
        }
        static void Validate(FieldWire.Session owner,PvpPending record)
        {
            if (record==null || record.version!=1 || record.playerId!=owner.playerId || record.origin!=owner.origin ||
                record.opponentId<=0 || record.opponentId==owner.playerId || record.stamp<=0 || String.IsNullOrEmpty(record.body))
                throw new InvalidOperationException("Invalid pending PK owner");
            Bytes(record);
        }
        static PvpPending Load(FieldWire.Session owner)
        {
            string raw=PlayerPrefs.GetString(Key(owner,"pending"),""); if (String.IsNullOrEmpty(raw)) return null;
            PvpPending value=JsonUtility.FromJson<PvpPending>(raw); Validate(owner,value); return value;
        }
        static void Save(FieldWire.Session owner,PvpPending value)
        {
            Validate(owner,value);
            string key=Key(owner,"pending"),lastKey=Key(owner,"last"),raw=JsonUtility.ToJson(value);
            string lastRaw=PlayerPrefs.GetString(lastKey,""); long last=0;
            if (!String.IsNullOrEmpty(lastRaw) && (!Int64.TryParse(lastRaw,NumberStyles.None,CultureInfo.InvariantCulture,out last) || last<=0))
                throw new InvalidOperationException("Invalid prior PK stamp");
            if (last>value.stamp) throw new InvalidOperationException("PK stamp regressed");
            PlayerPrefs.SetString(lastKey,value.stamp.ToString(CultureInfo.InvariantCulture));
            PlayerPrefs.SetString(key,raw); PlayerPrefs.Save();
            if (PlayerPrefs.GetString(key,"")!=raw || PlayerPrefs.GetString(lastKey,"")!=value.stamp.ToString(CultureInfo.InvariantCulture))
                throw new InvalidOperationException("PK journal save failed");
        }
        static long NextStamp(FieldWire.Session owner)
        {
            long last=0; string raw=PlayerPrefs.GetString(Key(owner,"last"),"");
            if (!String.IsNullOrEmpty(raw) && (!Int64.TryParse(raw,NumberStyles.None,CultureInfo.InvariantCulture,out last) || last<=0))
                throw new InvalidOperationException("Invalid previous PK stamp");
            // 63 bits of system entropy make independent-device collisions
            // negligible; the persisted high-water mark also prevents reuse on
            // this install even when clocks change or a request is rejected.
            if (last==Int64.MaxValue) throw new InvalidOperationException("PK stamp exhausted");
            byte[] random=new byte[8]; RandomNumberGenerator.Create().GetBytes(random); random[0]=(byte)(random[0]&127);
            long proposal=0; foreach (byte value in random) proposal=(proposal<<8)|value;
            if (proposal==Int64.MaxValue) proposal--;
            return Math.Max(Math.Max(1,proposal),last+1);
        }
        static void Clear(FieldWire.Session owner,PvpPending expected)
        {
            PvpPending current=Load(owner);
            if (current==null || expected==null || current.stamp!=expected.stamp || current.opponentId!=expected.opponentId ||
                current.body!=expected.body || current.bodyHash!=expected.bodyHash) throw new InvalidOperationException("PK journal changed");
            string key=Key(owner,"pending"),raw=PlayerPrefs.GetString(key,"");
            try { PlayerPrefs.DeleteKey(key); PlayerPrefs.Save(); if (PlayerPrefs.HasKey(key)) throw new InvalidOperationException("PK journal remains"); }
            catch (Exception) { PlayerPrefs.SetString(key,raw); throw; }
        }
        static bool MarkShown(FieldWire.Session owner,string fightId)
        {
            if (!FightId(fightId)) throw new InvalidOperationException("Invalid PK fight ID");
            string key=Key(owner,"shown"),raw=PlayerPrefs.GetString(key,"");
            var ids=new List<string>();
            if (!String.IsNullOrEmpty(raw)) foreach (string id in raw.Split(','))
            { if (!FightId(id) || ids.Contains(id)) throw new InvalidOperationException("Invalid shown PK journal"); ids.Add(id); }
            if (ids.Contains(fightId)) return false;
            ids.Add(fightId); if (ids.Count>64) ids.RemoveAt(0);
            string next=String.Join(",",ids.ToArray()); PlayerPrefs.SetString(key,next); PlayerPrefs.Save();
            if (PlayerPrefs.GetString(key,"")!=next) throw new InvalidOperationException("PK display journal failed");
            return true;
        }
        static PvpBridge Require()
        {
            if (instance!=null) return instance;
            GameObject host=new GameObject("HYJS.DesertV4.PvpBridge"); DontDestroyOnLoad(host);
            instance=host.AddComponent<PvpBridge>(); instance.wire=host.AddComponent<FieldWire>(); return instance;
        }
        void OnDestroy() { if (instance==this) instance=null; }
        bool Adopt()
        {
            FieldWire.Session now; if (!FieldWire.Current(out now)) return false;
            if (!SameSession(session,now))
            {
                bool same=SameActor(session,now); session=now; busy=false; epoch++; next=0;
                if (!same) loginReady=false;
                if (!(same && saveFailed && pending!=null))
                {
                    try { pending=Load(now); faulted=saveFailed=false; }
                    catch (Exception) { pending=null; faulted=true; Warn("PK记录需要恢复，请勿重复挑战"); return false; }
                }
            }
            return !faulted;
        }
        public static bool HasPending()
        {
            if (!Target()) return false;
            FieldWire.Session owner; if (!FieldWire.Current(out owner)) return true;
            if (instance!=null && SameActor(instance.session,owner) && (instance.pending!=null || instance.faulted)) return true;
            try { return Load(owner)!=null; } catch (Exception) { return true; }
        }
        public static void SceneReady()
        {
            if (!Target() || !NetAPI.SocketIsConnected()) return;
            FieldWire.Session owner; if (!FieldWire.Current(out owner)) return;
            try { if (Load(owner)==null) return; } catch (Exception) { Notice("PK记录需要恢复，请勿重复挑战"); return; }
            PvpBridge core=Require(); if (core.Adopt()) { core.loginReady=true; core.next=0; }
        }
        public static void ConnectionStarting()
        { if (instance!=null) { instance.epoch++; instance.loginReady=false; instance.busy=false; } }
        public static void LoginResponse(object value)
        {
            SCSocketLogin login=value as SCSocketLogin; if (login==null || login.result!=100) return;
            FieldWire.Session owner; if (!FieldWire.Current(out owner)) return;
            if (instance==null)
            {
                try { if (Load(owner)==null) return; } catch (Exception) { Notice("PK记录需要恢复，请勿重复挑战"); return; }
                Require();
            }
            if (instance.Adopt()) { instance.loginReady=true; instance.busy=false; instance.next=0; }
        }
        // Called in place of native TCP send exclusively for desert opcode 30045.
        public static bool Capture(int opcode,object value)
        {
            if (!Target() || opcode!=30045) return false;
            if (!FieldRuntime.Require().CanAttackPvp()) return true;
            PvpBridge core=Require(); if (!core.Adopt()) return true;
            if (core.pending!=null) { Notice("上次挑战仍在确认"); return true; }
            try
            {
                var message=value as CSSidePvpFightResult;
                if (message==null || message.playerId<=0 || message.playerId==core.session.playerId || message.fightType<0 || message.fightType>1)
                    throw new InvalidOperationException("Invalid original PK request");
                byte[] bytes=PBConvert.toBytes(30045,message);
                if (bytes==null || bytes.Length==0 || bytes.Length>MaxBody) throw new InvalidOperationException("Missing original PK request");
                core.pending=new PvpPending { version=1,playerId=core.session.playerId,opponentId=message.playerId,origin=core.session.origin,
                    stamp=NextStamp(core.session),body=Convert.ToBase64String(bytes),bodyHash=Digest(bytes) };
                core.saveFailed=true; Save(core.session,core.pending); core.saveFailed=false;
                core.loginReady=NetAPI.SocketIsConnected(); core.next=0; core.warned=false;
            }
            catch (Exception) { core.Warn("PK请求保存失败，请保持当前界面"); }
            return true;
        }
        void Update()
        {
            if (!Adopt() || pending==null) return;
            if (saveFailed)
            {
                try { Save(session,pending); saveFailed=false; }
                catch (Exception) { Warn("PK请求保存失败，请保持当前界面"); return; }
            }
            if (!NetAPI.SocketIsConnected()) loginReady=false;
            if (!loginReady || busy || Stopwatch.GetTimestamp()<next) return;
            Query();
        }
        bool Still(FieldWire.Session owner,PvpPending expected,int generation)
        { return generation==epoch && SameSession(owner,session) && pending!=null && expected!=null && pending.stamp==expected.stamp && pending.bodyHash==expected.bodyHash; }
        void Later() { busy=false; next=Stopwatch.GetTimestamp()+2L*Stopwatch.Frequency; }
        void Query()
        {
            FieldWire.Session owner=session; PvpPending record=pending; int gen=epoch; long sent=Stopwatch.GetTimestamp(); busy=true;
            try
            {
                wire.PvpStatus(owner,record.stamp,record.body,delegate(FieldWireResult result)
                {
                    if (!Still(owner,record,gen)) return;
                    Later();
                    if (result.uncertain || result.code!=0 || result.data==null) { Warn("PK结果待确认，请检查连接"); return; }
                    try
                    {
                        JsonData d=result.data;
                        if (Integer(d["stamp"])!=record.stamp || Integer(d["opponentId"])!=record.opponentId || !Hex((string)d["requestHash"],64))
                            throw new InvalidOperationException("PK status identity changed");
                        string status=(string)d["status"];
                        if (status=="unknown")
                        {
                            if (Target() && RoomManager.currentRoom is RoomField) { busy=true; StartCoroutine(Transmit(owner,record,gen)); }
                            return;
                        }
                        byte[] raw=Convert.FromBase64String((string)d["nativeResponse"]);
                        var response=PBConvert.toObject(30046,raw) as SCSidePvpFightResult;
                        if (response==null) throw new InvalidOperationException("Missing original PK response");
                        if (status=="rejected")
                        {
                            if (response.code!=2001 && response.code!=2002 && response.code!=2007) throw new InvalidOperationException("Invalid PK rejection");
                            Clear(owner,record); pending=null; warned=false;
                            if (Target()) { FieldRuntime.Require().RequestSnapshot(); if (!FieldRuntime.Require().PvpRejected(response.code)) Deliver(response); }
                            return;
                        }
                        if (status!="settled" || response.code!=1 || response.attPlayerId!=owner.playerId || response.defPlayerId!=record.opponentId ||
                            response.winPlayerId!=owner.playerId && response.winPlayerId!=record.opponentId || !FightId(response.fightId) ||
                            (string)d["fightId"]!=response.fightId || Integer(d["winnerId"])!=response.winPlayerId || !d["presentationCurrent"].IsBoolean)
                            throw new InvalidOperationException("Invalid settled PK receipt");
                        bool current=(bool)d["presentationCurrent"];
                        long serverTime=Integer(d["serverTimeMs"]),until=Integer(d["presentationUntilMs"]);
                        if (serverTime<=0 || until<=0) throw new InvalidOperationException("Invalid PK presentation lease");
                        if (current)
                        {
                            long elapsed=(Stopwatch.GetTimestamp()-sent)*1000/Stopwatch.Frequency;
                            if (until-serverTime<=elapsed) { next=0; return; }
                            if (!Target() || !(RoomManager.currentRoom is RoomField) || UI.PageManager.Instance==null ||
                                !(UI.PageManager.Instance.currentPage is UI.UIFieldRunMap)) return;
                            byte[] stateBytes=Convert.FromBase64String((string)d["nativeState"]);
                            var state=PBConvert.toObject(30042,stateBytes) as SCFieldPlayerState;
                            if (state==null || state.player==null || state.player.Count!=2) throw new InvalidOperationException("Invalid native PK state");
                            long a=state.player[0].playerId,b=state.player[1].playerId;
                            if (!((a==owner.playerId && b==record.opponentId) || (b==owner.playerId && a==record.opponentId)))
                                throw new InvalidOperationException("Native PK participants differ");
                            ApplyState(state);
                        }
                        bool display=current && MarkShown(owner,response.fightId);
                        Clear(owner,record); pending=null; warned=false;
                        if (Target()) FieldRuntime.Require().RequestSnapshot();
                        if (display) Deliver(response);
                        else Notice("PK结果已同步");
                    }
                    catch (Exception) { Warn("PK结果需要确认，请勿重复挑战"); }
                });
            }
            catch (Exception) { Later(); Warn("PK结果待确认，请检查连接"); }
        }
        static long Integer(JsonData data) { return Int64.Parse(data.ToString(),CultureInfo.InvariantCulture); }
        static void ApplyState(SCFieldPlayerState state)
        {
            RoomField room=RoomManager.currentRoom as RoomField; if (room==null || room.netManager==null) throw new InvalidOperationException("PK room unavailable");
            MethodInfo method=room.netManager.GetType().GetMethod("SCFieldPlayerState",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
            if (method==null) throw new InvalidOperationException("Original PK state handler missing");
            method.Invoke(room.netManager,new object[]{state});
        }
        static void Deliver(SCSidePvpFightResult response)
        {
            RoomField room=RoomManager.currentRoom as RoomField; if (room==null || room.netManager==null) return;
            PvpBridge core=Require(); MethodInfo method=room.netManager.GetType().GetMethod("SCSidePvpFightResult",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
            if (method==null) throw new InvalidOperationException("Original PK callback missing");
            try { core.delivering=true; method.Invoke(room.netManager,new object[]{response}); }
            finally { core.delivering=false; }
        }
        // Native socket replies in desert are accepted only for the defender;
        // initiator replies are tied to the journal and status above.
        public static bool Intercept(object value)
        {
            if (!Target() || instance!=null && instance.delivering) return false;
            var response=value as SCSidePvpFightResult; FieldWire.Session owner;
            if (response==null || response.code!=1 || !FieldWire.Current(out owner) || response.defPlayerId!=owner.playerId ||
                response.attPlayerId<=0 || response.attPlayerId==owner.playerId ||
                response.winPlayerId!=owner.playerId && response.winPlayerId!=response.attPlayerId || !FightId(response.fightId)) return true;
            try { return !MarkShown(owner,response.fightId); }
            catch (Exception) { Notice("PK记录需要确认"); return true; }
        }
        static int ReadInt(byte[] bytes,int offset)
        { return (bytes[offset]<<24)|(bytes[offset+1]<<16)|(bytes[offset+2]<<8)|bytes[offset+3]; }
        static long ReadLong(byte[] bytes,int offset)
        {
            long value=0; for (int i=0;i<8;i++) value=(value<<8)|bytes[offset+i]; return value;
        }
        static byte[] Frame(FieldWire.Session owner,PvpPending record)
        {
            FieldWire.Session live;
            if (!FieldWire.Current(out live) || !SameSession(owner,live)) throw new InvalidOperationException("PK session changed");
            byte[] body=Bytes(record); object message=PBConvert.toObject(30045,body); byte[] packet;
            if (!PacketFactory.WebPB2Bytes(30045,message,out packet) || packet==null || packet.Length!=68+body.Length)
                throw new InvalidOperationException("Original HTTP packet unavailable");
            if (StaticData.playerData==null || ReadInt(packet,0)!=packet.Length || ReadInt(packet,4)!=10000 ||
                ReadInt(packet,8)!=30045 || ReadLong(packet,12)!=owner.playerId || ReadInt(packet,20)!=StaticData.playerData.ZoneId ||
                ReadInt(packet,24)!=body.Length) throw new InvalidOperationException("Original PK header differs");
            for (int i=0;i<16;i++)
                if (packet[28+i]!=Byte.Parse(owner.sessionHex.Substring(i*2,2),NumberStyles.HexNumber,CultureInfo.InvariantCulture))
                    throw new InvalidOperationException("Original PK session differs");
            for (int i=0;i<body.Length;i++) if (packet[68+i]!=body[i]) throw new InvalidOperationException("Original PK body changed");
            MethodInfo signer=typeof(PacketFactory).GetMethod("GetSig",BindingFlags.Static|BindingFlags.NonPublic);
            if (signer==null) throw new InvalidOperationException("Original packet signer unavailable");
            byte[] signature=signer.Invoke(null,new object[]{owner.playerId,StaticData.playerData.ZoneId,body,record.stamp}) as byte[];
            if (signature==null || signature.Length!=16) throw new InvalidOperationException("Invalid original packet signature");
            System.Buffer.BlockCopy(signature,0,packet,44,16); long stamp=record.stamp;
            for (int i=67;i>=60;i--) { packet[i]=(byte)(stamp&255); stamp>>=8; }
            return packet;
        }
        IEnumerator Transmit(FieldWire.Session owner,PvpPending record,int gen)
        {
            WWW request=null;
            try
            {
                byte[] packet=Frame(owner,record);
                var headers=new Dictionary<string,string>(); headers.Add("Content-Type","application/octet-stream");
                request=new WWW(owner.origin+"/sao/",packet,headers);
            }
            catch (Exception) { if (Still(owner,record,gen)) { Later(); Warn("PK请求待确认，请检查连接"); } yield break; }
            long deadline=Stopwatch.GetTimestamp()+15L*Stopwatch.Frequency;
            while (!request.isDone && Stopwatch.GetTimestamp()<deadline) yield return null;
            request.Dispose();
            if (Still(owner,record,gen)) { Later(); next=0; }
        }
    }
}
