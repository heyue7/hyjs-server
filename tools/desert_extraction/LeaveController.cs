using System;
using System.Diagnostics;
using System.Globalization;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using LitJson;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    [Serializable] sealed class LeaveRecord
    {
        public int version,opcode,targetLine;
        public long playerId,positionRevision;
        public string origin,requestId,mode,targetScene,payload,hash,phase;
    }
    // Original scene/line packets are sent at most once. An uncertain send is
    // reconciled through the server's committed native receipt, never guessed.
    public sealed class LeaveController : MonoBehaviour
    {
        static LeaveController instance;
        static bool bypassSend,bypassReply;
        FieldWire wire;
        FieldWire.Session session;
        LeaveRecord pending;
        RoomBase originRoom;
        object callbackOwner;
        bool busy,awaitingPosition,warned,faulted,saveFailed,callbackFault;
        int epoch,generation;
        long next;
        static bool SameActor(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.origin==b.origin; }
        static bool SameSession(FieldWire.Session a,FieldWire.Session b)
        { return SameActor(a,b) && a.sessionHex==b.sessionHex; }
        static bool UUID(string value)
        { try { return !String.IsNullOrEmpty(value) && new Guid(value).ToString("D")==value; } catch (Exception) { return false; } }
        static string Key(FieldWire.Session owner)
        { return "hyjs.ggo.leave.v4."+Uri.EscapeDataString(owner.origin)+"."+owner.playerId.ToString(CultureInfo.InvariantCulture); }
        static string Digest(byte[] bytes)
        {
            using (SHA256 sha=SHA256.Create())
            { var result=new StringBuilder(64); foreach (byte b in sha.ComputeHash(bytes)) result.Append(b.ToString("x2")); return result.ToString(); }
        }
        static bool ValidTarget(LeaveRecord p)
        {
            return p.mode=="scene" && p.opcode==30032 && p.targetLine==0 &&
                (p.targetScene=="town" || p.targetScene=="town_main" || p.targetScene=="field_001" || p.targetScene=="field_002") ||
                p.mode=="line" && p.opcode==30061 && String.IsNullOrEmpty(p.targetScene) && p.targetLine>0;
        }
        static object Message(LeaveRecord p)
        {
            byte[] bytes=Convert.FromBase64String(p.payload);
            if (bytes.Length==0 || bytes.Length>65536 || Digest(bytes)!=p.hash) throw new InvalidOperationException("Invalid leave request bytes");
            object msg=PBConvert.toObject(p.opcode,bytes);
            if (p.opcode==30032 && (!(msg is CSTransferScene) || ((CSTransferScene)msg).target!=p.targetScene) ||
                p.opcode==30061 && (!(msg is CSTransferLine) || ((CSTransferLine)msg).line!=p.targetLine))
                throw new InvalidOperationException("Leave request target changed");
            return msg;
        }
        static LeaveRecord Load(FieldWire.Session owner)
        {
            string raw=PlayerPrefs.GetString(Key(owner),""); if (String.IsNullOrEmpty(raw)) return null;
            LeaveRecord p=JsonUtility.FromJson<LeaveRecord>(raw);
            if (p==null || p.version!=1 || p.playerId!=owner.playerId || p.origin!=owner.origin || !UUID(p.requestId) ||
                !ValidTarget(p) || (p.phase!="prepare" && p.phase!="sent" && p.phase!="position") ||
                (p.phase=="position" && p.positionRevision<1)) throw new InvalidOperationException("Unreadable leave record");
            Message(p); return p;
        }
        static void Save(FieldWire.Session owner,LeaveRecord p)
        {
            if (p==null || p.playerId!=owner.playerId || p.origin!=owner.origin || !UUID(p.requestId) || !ValidTarget(p)) throw new InvalidOperationException("Invalid leave owner");
            Message(p); string raw=JsonUtility.ToJson(p),key=Key(owner);
            PlayerPrefs.SetString(key,raw); PlayerPrefs.Save();
            if (PlayerPrefs.GetString(key,"")!=raw) throw new InvalidOperationException("Leave save failed");
        }
        static void Clear(FieldWire.Session owner,LeaveRecord p)
        {
            LeaveRecord old=Load(owner);
            if (old==null || p==null || old.requestId!=p.requestId || old.hash!=p.hash) throw new InvalidOperationException("Leave changed");
            string key=Key(owner),raw=PlayerPrefs.GetString(key,"");
            try { PlayerPrefs.DeleteKey(key); PlayerPrefs.Save(); }
            catch (Exception) { PlayerPrefs.SetString(key,raw); throw; }
        }
        static void Notice(string text) { BoxManager.CreatePopupTextBox(text,0f); }
        void Warn(string text) { if (!warned) { warned=true; Notice(text); } }
        static long Integer(JsonData data) { return Int64.Parse(data.ToString(),CultureInfo.InvariantCulture); }
        static Vector3 Vector(JsonData d)
        {
            float x=(float)Double.Parse(d["x"].ToString(),CultureInfo.InvariantCulture), y=(float)Double.Parse(d["y"].ToString(),CultureInfo.InvariantCulture), z=(float)Double.Parse(d["z"].ToString(),CultureInfo.InvariantCulture);
            if (Single.IsNaN(x)||Single.IsNaN(y)||Single.IsNaN(z)||Single.IsInfinity(x)||Single.IsInfinity(y)||Single.IsInfinity(z)) throw new InvalidOperationException("Invalid position");
            return new Vector3(x,y,z);
        }
        static RoomCharactor Actor()
        { RoomBase room=RoomManager.currentRoom; return room==null || room.charactorManager==null ? null : room.charactorManager.GetSelfCharactor(); }
        static LeaveController Require()
        {
            if (instance!=null) return instance;
            var host=new GameObject("HYJS.DesertV4.Leave"); DontDestroyOnLoad(host);
            instance=host.AddComponent<LeaveController>(); instance.wire=host.AddComponent<FieldWire>(); return instance;
        }
        public static void SceneReady()
        {
            FieldWire.Session owner; if (!FieldWire.Current(out owner)) return;
            try { if (Load(owner)!=null) Require().Adopt(); }
            catch (Exception) { Notice("离图记录需要恢复，请勿重复操作"); }
        }
        bool Adopt()
        {
            FieldWire.Session now; if (!FieldWire.Current(out now)) return false;
            if (!SameSession(session,now))
            {
                bool same=SameActor(session,now);
                if (!same) PositionSync.Release();
                session=now; busy=awaitingPosition=false; next=0; generation++;
                if (!same) { callbackOwner=null; originRoom=null; }
                if (!(same && saveFailed && pending!=null))
                {
                    try { pending=Load(now); faulted=saveFailed=false; }
                    catch (Exception) { pending=null; faulted=true; Warn("离图记录需要恢复，请勿重复操作"); return false; }
                }
            }
            return !faulted;
        }
        bool Still(FieldWire.Session owner,string id)
        { return SameSession(session,owner) && pending!=null && pending.requestId==id; }
        public static bool Pending { get { return instance!=null && (instance.pending!=null || instance.faulted); } }
        public static bool Send(int opcode,object message)
        {
            PositionSync.TrackSend(opcode);
            if (bypassSend || StaticData.fieldId!="field_003" || !(RoomManager.currentRoom is RoomField) || (opcode!=30032 && opcode!=30061)) return false;
            LeaveController core=Require();
            if (!core.Adopt()) return true;
            if (core.pending!=null) { Notice("上次离图仍在确认"); return true; }
            if (BattleRecovery.HasUnsettled() || TravelController.PendingExtraction) { Notice("请先完成当前行动"); return true; }
            FieldWire.Session owner=core.session;
            LeaveRecord record=new LeaveRecord {version=1,opcode=opcode,playerId=owner.playerId,origin=owner.origin,requestId=Guid.NewGuid().ToString("D"),phase="prepare"};
            if (opcode==30032 && message is CSTransferScene) { record.mode="scene"; record.targetScene=((CSTransferScene)message).target; }
            else if (opcode==30061 && message is CSTransferLine) { record.mode="line"; record.targetLine=((CSTransferLine)message).line; }
            if (!ValidTarget(record)) { Notice("当前目标暂不可离开沙漠"); return true; }
            try
            {
                byte[] bytes=PBConvert.toBytes(opcode,message);
                if (bytes==null || bytes.Length==0 || bytes.Length>65536) throw new InvalidOperationException("Invalid native leave");
                record.payload=Convert.ToBase64String(bytes); record.hash=Digest(bytes); Message(record);
            }
            catch (Exception) { Notice("离图请求无法确认"); return true; }
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton("离开前将结算携带芯片，确认继续？",delegate(bool yes)
            {
                if (!yes || !core.Adopt() || !SameSession(owner,core.session) || core.pending!=null || StaticData.fieldId!="field_003") return;
                if (!PositionSync.TryLock(Actor())) { Notice("位置正在同步，请稍候"); return; }
                core.pending=record; core.originRoom=RoomManager.currentRoom;
                core.callbackOwner=opcode==30032 ? (object)ManagerController.Instance : (object)(core.originRoom==null ? null : core.originRoom.netManager as RoomNetField);
                try { Save(owner,record); core.next=0; }
                catch (Exception) { core.saveFailed=true; core.Warn("离图记录待确认"); }
            });
            return true;
        }
        public static bool Response(int opcode,object owner,object response)
        {
            if (bypassReply) return false;
            if ((StaticData.fieldId!="field_003" || !(RoomManager.currentRoom is RoomField)) && !Pending) return false;
            if (opcode!=30033 && opcode!=30062) return false;
            if (instance!=null && instance.pending!=null) { instance.next=0; instance.busy=false; instance.generation++; }
            return true;
        }
        void OnDestroy() { if (instance==this) instance=null; }
        void Update()
        {
            if (!Adopt() || pending==null) return;
            if (saveFailed) { try { Save(session,pending); saveFailed=false; } catch (Exception) { Warn("离图记录待确认"); return; } }
            PositionSync.Tick(); int current=PositionSync.ConnectionEpoch;
            if (current!=epoch) { epoch=current; generation++; busy=awaitingPosition=false; next=0; }
            if (busy || awaitingPosition || Stopwatch.GetTimestamp()<next) return;
            if (pending.phase=="prepare") Prepare(); else if (pending.phase=="sent") Status(); else Synchronize();
        }
        void Later() { busy=false; next=Stopwatch.GetTimestamp()+Stopwatch.Frequency; }
        void Finish(string text)
        {
            try { Clear(session,pending); pending=null; busy=awaitingPosition=false; callbackOwner=null; originRoom=null; PositionSync.Release(); warned=false; Notice(text); if (StaticData.fieldId=="field_003") FieldRuntime.Require().RequestSnapshot(); }
            catch (Exception) { Warn("离图结果待确认"); }
        }
        void Prepare()
        {
            FieldWire.Session owner=session; string id=pending.requestId; int op=generation; busy=true;
            wire.LeavePrepare(owner,id,pending.mode,pending.targetScene,pending.targetLine,delegate(FieldWireResult result)
            {
                if (op!=generation || !Still(owner,id)) return; Later();
                if (result.uncertain || result.code==7002 || result.code==7207) { Warn("离图确认中"); return; }
                if (result.code!=0) { Finish("当前无法离图，请回到安全区重试"); return; }
                try
                {
                    JsonData d=result.data;
                    if (d==null || (string)d["requestId"]!=id || (string)d["status"]!="prepared" || (string)d["mode"]!=pending.mode ||
                        (pending.mode=="scene" && (string)d["targetScene"]!=pending.targetScene) ||
                        (pending.mode=="line" && Integer(d["targetLine"])!=pending.targetLine) || Integer(d["expiresAtMs"])<=0)
                        throw new InvalidOperationException("Invalid prepared leave");
                    if (!NetAPI.SocketIsConnected() || !PositionSync.ChannelReady) return;
                    object native=Message(pending); pending.phase="sent";
                    try { Save(owner,pending); }
                    catch (Exception) { pending.phase="prepare"; saveFailed=true; throw; }
                    bypassSend=true;
                    try { NetAPI.SendSocket(pending.opcode,native); }
                    finally { bypassSend=false; }
                    warned=false; next=0;
                }
                catch (Exception) { Warn("离图确认中"); }
            });
        }
        void Status()
        {
            FieldWire.Session owner=session; string id=pending.requestId; int op=generation; busy=true;
            wire.LeaveStatus(owner,id,delegate(FieldWireResult result)
            {
                if (op!=generation || !Still(owner,id)) return; Later();
                if (result.uncertain || result.code!=0 || result.data==null) { Warn("离图结果待确认"); return; }
                try
                {
                    JsonData d=result.data; if ((string)d["requestId"]!=id) throw new InvalidOperationException();
                    string status=(string)d["status"];
                    if (status=="expired" || status=="superseded") { Finish("离图确认已失效，请重试"); return; }
                    if (status=="prepared" || status=="unknown") return;
                    if (status!="completed" || !d["targetCurrent"].IsBoolean || !(bool)d["targetCurrent"] ||
                        (string)d["nativeRequestHash"]!=pending.hash || d["nativeResponse"]==null) throw new InvalidOperationException("Leave receipt identity mismatch");
                    byte[] bytes=Convert.FromBase64String((string)d["nativeResponse"]);
                    if (bytes.Length>65536) throw new InvalidOperationException("Invalid original response");
                    int responseOpcode=pending.opcode==30032 ? 30033:30062; object native=PBConvert.toObject(responseOpcode,bytes);
                    if (responseOpcode==30033 && (!(native is SCTransferScene) || ((SCTransferScene)native).result!=1) ||
                        responseOpcode==30062 && !(native is SCTransferLine)) throw new InvalidOperationException("Invalid original response");
                    long rev=Integer(d["positionRevision"]); if (rev<1) throw new InvalidOperationException("Missing position revision");
                    pending.positionRevision=rev; pending.phase="position";
                    try { Save(owner,pending); }
                    catch (Exception) { pending.positionRevision=0; pending.phase="sent"; saveFailed=true; throw; }
                    warned=false;
                    if (callbackOwner!=null)
                    {
                        string name=responseOpcode==30033 ? "SceneTransferCallback":"SCTransferLine";
                        if (responseOpcode==30062 && (RoomManager.currentRoom!=originRoom || originRoom==null || originRoom.netManager!=callbackOwner)) throw new InvalidOperationException("Original room changed");
                        MethodInfo method=callbackOwner.GetType().GetMethod(name,BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
                        if (method==null) throw new InvalidOperationException("Original transfer callback unavailable");
                        bypassReply=true;
                        try { method.Invoke(callbackOwner,new object[]{native}); }
                        catch (Exception) { callbackFault=true; throw; }
                        finally { bypassReply=false; }
                    }
                    next=0;
                }
                catch (Exception) { Warn(callbackFault ? "请重新登录以恢复离图" : "离图回执待确认"); }
            });
        }
        void Synchronize()
        {
            if (callbackFault) { Warn("请重新登录以恢复离图"); return; }
            if (!PositionSync.Locked && !PositionSync.LockAfterTeleport(Actor())) return;
            RoomBase current=RoomManager.currentRoom;
            if (current==null || current.netManager==null || !NetAPI.SocketIsConnected() || !PositionSync.ChannelReady) return;
            if (pending.mode=="scene" && originRoom!=null && current==originRoom) return;
            if (pending.mode=="scene" && pending.targetScene.StartsWith("field_",StringComparison.Ordinal) && StaticData.fieldId!=pending.targetScene) return;
            FieldWire.Session owner=session; string id=pending.requestId; int connection=PositionSync.ConnectionEpoch;
            try
            {
                awaitingPosition=true;
                PositionSync.RequestCurrent(current.netManager,delegate(Vector3 observed)
                {
                    if (!Still(owner,id) || connection!=PositionSync.ConnectionEpoch) return;
                    awaitingPosition=false; busy=true;
                    wire.PositionStatus(owner,delegate(FieldWireResult result)
                    {
                        if (!Still(owner,id) || connection!=PositionSync.ConnectionEpoch) return; Later();
                        if (result.uncertain || result.code!=0 || result.data==null) { Warn("位置确认中"); return; }
                        try
                        {
                            JsonData d=result.data; string expected=pending.mode=="line" ? "field_003" : pending.targetScene;
                            if ((string)d["scene"]!=expected || !d["loading"].IsBoolean || (bool)d["loading"]) throw new InvalidOperationException();
                            long rev=Integer(d["positionRevision"]),confirmed=Integer(d["confirmedRevision"]);
                            if (rev<pending.positionRevision || confirmed!=rev || (Vector(d["position"])-observed).sqrMagnitude>.0001f) throw new InvalidOperationException();
                            Finish("结算完成，已离开");
                        }
                        catch (Exception) { Warn("位置确认中"); }
                    });
                });
            }
            catch (Exception) { awaitingPosition=false; next=Stopwatch.GetTimestamp()+Stopwatch.Frequency; }
        }
    }
}
