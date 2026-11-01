using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using LitJson;
using NetWork.Packets;
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
        sealed class TransferTicket
        {
            public int epoch,reply;
            public bool own;
            public string requestId;
            public object callbackOwner;
            public FieldWire.Session session;
        }
        sealed class TransferPacket
        {
            public int epoch;
            public FieldWire.Session session;
        }
        static readonly object transferGate=new object();
        static readonly Dictionary<Packet,TransferPacket> transferPackets=new Dictionary<Packet,TransferPacket>();
        static readonly List<TransferTicket> transferTickets=new List<TransferTicket>();
        static object dispatchedTransfer,capturedTransfer,capturedOwner;
        static int transferEpoch,dispatchedEpoch,capturedEpoch;
        static bool transferUncertain;
        FieldWire wire;
        FieldWire.Session session;
        LeaveRecord pending;
        RoomBase originRoom;
        object callbackOwner;
        bool busy,awaitingPosition,warned,faulted,saveFailed,callbackFault,receiptVerified,receiptTargetCurrent,nativeCallbackUncertain;
        int epoch,generation;
        long next,positionLease;
        RoomBase syncRoom;
        RoomNetBase syncNet;
        static bool SameActor(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.origin==b.origin; }
        static bool SameSession(FieldWire.Session a,FieldWire.Session b)
        { return SameActor(a,b) && a.sessionHex==b.sessionHex; }
        static bool TransferResponse(object body)
        { return body is SCTransferScene || body is SCTransferLine; }
        static void TransferEpochLocked(int value)
        {
            if (transferEpoch==value) return;
            transferEpoch=value; transferTickets.Clear(); transferUncertain=false;
            dispatchedTransfer=capturedTransfer=capturedOwner=null; dispatchedEpoch=capturedEpoch=0;
        }
        // The original replies have no request ID. Preserve the ordering of
        // actual sends on this connection and only claim our own live ticket.
        static void TrackTransferSend(int opcode,bool own,FieldWire.Session owner,object callback,string requestId)
        {
            int current=PositionSync.ConnectionEpoch;
            lock (transferGate)
            {
                TransferEpochLocked(current);
                if (transferTickets.Count>=64) { transferUncertain=true; return; }
                if (own && transferTickets.Count>0)
                { transferUncertain=true; if (instance!=null) instance.nativeCallbackUncertain=true; }
                transferTickets.Add(new TransferTicket { epoch=current,reply=opcode==30032 ? 30033 : 30062,
                    own=own,session=owner,callbackOwner=callback,requestId=requestId });
            }
        }
        static bool CanSendTransfer(int opcode)
        {
            int current=PositionSync.ConnectionEpoch;
            lock (transferGate)
            {
                TransferEpochLocked(current);
                return !transferUncertain && transferTickets.Count==0;
            }
        }
        // Packet provenance survives room changes; a queued old-connection
        // frame cannot be mistaken for a response on the new connection.
        internal static void TrackPacket(object value)
        {
            Packet packet=value as Packet;
            if (packet==null || (!TransferResponse(packet.kBody) && !(packet.kBody is ErrorMessage))) return;
            FieldWire.Session owner=null;
            try { FieldWire.Current(out owner); } catch (Exception) { }
            lock (transferGate)
            {
                if (transferPackets.Count>=256) { transferUncertain=true; return; }
                transferPackets[packet]=new TransferPacket { epoch=PositionSync.ConnectionEpoch,session=owner };
            }
        }
        internal static bool DropPacket(object value)
        {
            Packet packet=value as Packet;
            if (packet==null || (!TransferResponse(packet.kBody) && !(packet.kBody is ErrorMessage))) return false;
            int current=PositionSync.ConnectionEpoch; TransferPacket queued; bool found;
            FieldWire.Session owner=null;
            try { FieldWire.Current(out owner); } catch (Exception) { }
            lock (transferGate)
            {
                TransferEpochLocked(current);
                found=transferPackets.TryGetValue(packet,out queued); transferPackets.Remove(packet);
                dispatchedTransfer=capturedTransfer=capturedOwner=null; dispatchedEpoch=capturedEpoch=0;
                if (found && (queued.epoch!=current || (queued.session!=null && owner!=null && !SameSession(queued.session,owner)))) return true;
                if (packet.kBody is ErrorMessage)
                {
                    if (transferTickets.Count>0)
                    { transferUncertain=true; if (instance!=null && instance.pending!=null) instance.nativeCallbackUncertain=true; }
                    return false;
                }
                if (found) { dispatchedTransfer=packet.kBody; dispatchedEpoch=current; }
                return false;
            }
        }
        static bool TakeOwnResponse(int opcode,object owner,object response,FieldWire.Session current)
        {
            lock (transferGate)
            {
                int connection=PositionSync.ConnectionEpoch; TransferEpochLocked(connection);
                if (System.Object.ReferenceEquals(capturedTransfer,response) && System.Object.ReferenceEquals(capturedOwner,owner) && capturedEpoch==connection) return true;
                if (transferUncertain || dispatchedEpoch!=connection || !System.Object.ReferenceEquals(dispatchedTransfer,response))
                {
                    if (instance!=null && instance.pending!=null && instance.pending.phase=="sent" &&
                        System.Object.ReferenceEquals(instance.callbackOwner,owner)) instance.nativeCallbackUncertain=true;
                    return false;
                }
                dispatchedTransfer=null; dispatchedEpoch=0;
                int index=transferTickets.FindIndex(delegate(TransferTicket ticket) { return ticket.reply==opcode; });
                if (index<0)
                {
                    if (instance!=null && instance.pending!=null && instance.pending.phase=="sent" &&
                        System.Object.ReferenceEquals(instance.callbackOwner,owner)) instance.nativeCallbackUncertain=true;
                    return false;
                }
                TransferTicket found=transferTickets[index];
                if (found.own && !System.Object.ReferenceEquals(found.callbackOwner,owner)) return false;
                transferTickets.RemoveAt(index);
                if (!found.own)
                {
                    if (transferTickets.Exists(delegate(TransferTicket ticket) { return ticket.own && ticket.reply==opcode; }))
                    { transferUncertain=true; if (instance!=null && instance.pending!=null) instance.nativeCallbackUncertain=true; }
                    return false;
                }
                // Packets queued under an old session were rejected above. If
                // an unprovable ticket still belongs to another role, leave the
                // current role's reply to its original callback.
                if (!SameSession(found.session,current)) return false;
                bool same=true;
                if (same && (instance==null || instance.pending==null || instance.pending.requestId!=found.requestId)) same=false;
                capturedTransfer=response; capturedOwner=owner; capturedEpoch=connection;
                if (same)
                {
                    instance.next=0; instance.busy=instance.awaitingPosition=instance.receiptVerified=false;
                    instance.syncRoom=null; instance.syncNet=null; instance.generation++;
                }
                return true;
            }
        }
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
                if (session==null) epoch=PositionSync.ConnectionEpoch;
                bool same=SameActor(session,now);
                if (!same) { PositionSync.ReleaseIfOwned(positionLease); positionLease=0; }
                session=now; busy=awaitingPosition=receiptVerified=false; syncRoom=null; syncNet=null; next=0; generation++;
                if (!same) { callbackOwner=null; originRoom=null; callbackFault=nativeCallbackUncertain=false; }
                if (!(same && saveFailed && pending!=null))
                {
                    try { pending=Load(now); faulted=saveFailed=false; }
                    catch (Exception) { pending=null; faulted=true; PositionSync.SetLeaveActive(true); Warn("离图记录需要恢复，请勿重复操作"); return false; }
                }
            }
            PositionSync.SetLeaveActive(pending!=null || faulted);
            return !faulted;
        }
        bool Still(FieldWire.Session owner,string id)
        { return SameSession(session,owner) && pending!=null && pending.requestId==id; }
        public static bool Pending { get { return instance!=null && (instance.pending!=null || instance.faulted); } }
        public static bool Send(int opcode,object message)
        {
            PositionSync.TrackSend(opcode);
            if (opcode!=30032 && opcode!=30061) return false;
            if (bypassSend)
            {
                if (instance!=null && instance.pending!=null)
                    TrackTransferSend(opcode,true,instance.session,instance.callbackOwner,instance.pending.requestId);
                return false;
            }
            FieldWire.Session live;
            if (FieldWire.Current(out live))
            {
                try
                {
                    if ((instance!=null || Load(live)!=null) && (!Require().Adopt() || instance.pending!=null))
                    { Notice("上次离图仍在确认"); return true; }
                }
                catch (Exception) { Notice("离图记录需要恢复，请勿重复操作"); return true; }
            }
            else if (instance!=null && (instance.pending!=null || instance.faulted))
            { Notice("离图正在确认"); return true; }
            if (StaticData.fieldId!="field_003" || !(RoomManager.currentRoom is RoomField))
            { TrackTransferSend(opcode,false,live,null,null); return false; }
            LeaveController core=Require();
            if (!core.Adopt()) return true;
            if (core.pending!=null) { Notice("上次离图仍在确认"); return true; }
            // Read the durable entry/extraction record as well as the live
            // controller; the native position frame can arrive before its
            // scene-ready callback has restored the in-memory instance.
            if (BattleRecovery.HasUnsettled() || TravelController.Pending) { Notice("请先完成当前行动"); return true; }
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
                long lease;
                if (!PositionSync.TryLock(Actor(),out lease)) { Notice("位置正在同步，请稍候"); return; }
                core.positionLease=lease;
                core.pending=record; PositionSync.SetLeaveActive(true); core.originRoom=RoomManager.currentRoom;
                core.callbackOwner=opcode==30032 ? (object)ManagerController.Instance : (object)(core.originRoom==null ? null : core.originRoom.netManager as RoomNetField);
                try { Save(owner,record); core.next=0; }
                catch (Exception) { core.saveFailed=true; core.Warn("离图记录待确认"); }
            });
            return true;
        }
        public static bool Response(int opcode,object owner,object response)
        {
            if (bypassReply) return false;
            if (opcode!=30033 && opcode!=30062) return false;
            if (opcode==30033 && !(response is SCTransferScene) || opcode==30062 && !(response is SCTransferLine)) return false;
            FieldWire.Session current; FieldWire.Current(out current);
            return TakeOwnResponse(opcode,owner,response,current);
        }
        void OnDestroy() { if (instance==this) { instance=null; PositionSync.SetLeaveActive(false); } }
        void Update()
        {
            if (!Adopt() || pending==null) return;
            if (saveFailed) { try { Save(session,pending); saveFailed=false; } catch (Exception) { Warn("离图记录待确认"); return; } }
            PositionSync.TickRecovery(); int current=PositionSync.ConnectionEpoch;
            if (current!=epoch)
            {
                epoch=current; generation++; busy=awaitingPosition=receiptVerified=false; syncRoom=null; syncNet=null; next=0;
                // A fresh authenticated native handshake below determines the
                // actual room after a callback fault and reconnect.
                if (callbackFault || nativeCallbackUncertain) { callbackFault=nativeCallbackUncertain=false; originRoom=null; callbackOwner=null; }
            }
            if (syncRoom!=null && (RoomManager.currentRoom!=syncRoom || syncRoom.netManager!=syncNet))
            { generation++; busy=awaitingPosition=receiptVerified=false; syncRoom=null; syncNet=null; next=0; }
            if (busy || awaitingPosition || Stopwatch.GetTimestamp()<next) return;
            if (callbackFault) { Warn("请重新登录以恢复离图"); return; }
            if (pending.phase=="prepare" && StaticData.fieldId=="field_003" && RoomManager.currentRoom is RoomField) Prepare();
            else if (!receiptVerified) Status(); else Synchronize();
        }
        void Later() { busy=false; next=Stopwatch.GetTimestamp()+Stopwatch.Frequency; }
        void Finish(string text)
        {
            try
            {
                Clear(session,pending); pending=null; PositionSync.SetLeaveActive(false); busy=awaitingPosition=receiptVerified=false; callbackOwner=null; originRoom=null; syncRoom=null; syncNet=null;
                PositionSync.ReleaseIfOwned(positionLease); positionLease=0; callbackFault=nativeCallbackUncertain=false; warned=false;
                if (!PositionSync.Locked && (StaticData.fieldId!="field_003" || !(RoomManager.currentRoom is RoomField))) PositionSync.SetFieldActive(false);
                Notice(text); if (StaticData.fieldId=="field_003" && RoomManager.currentRoom is RoomField) FieldRuntime.Require().RequestSnapshot();
            }
            catch (Exception) { Warn("离图结果待确认"); }
        }
        void Prepare()
        {
            FieldWire.Session owner=session; string id=pending.requestId; int op=generation,connection=PositionSync.ConnectionEpoch; busy=true;
            wire.LeavePrepare(owner,id,pending.mode,pending.targetScene,pending.targetLine,delegate(FieldWireResult result)
            {
                if (op!=generation || connection!=PositionSync.ConnectionEpoch || !Still(owner,id)) return; Later();
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
                    if (!CanSendTransfer(pending.opcode)) { Warn("上次转场仍在确认，请重新连接"); return; }
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
            FieldWire.Session owner=session; string id=pending.requestId; int op=generation,connection=PositionSync.ConnectionEpoch; busy=true;
            wire.LeaveStatus(owner,id,delegate(FieldWireResult result)
            {
                if (op!=generation || connection!=PositionSync.ConnectionEpoch || !Still(owner,id)) return; Later();
                if (result.uncertain || result.code!=0 || result.data==null) { Warn("离图结果待确认"); return; }
                try
                {
                    JsonData d=result.data; if ((string)d["requestId"]!=id) throw new InvalidOperationException();
                    string status=(string)d["status"];
                    if ((status=="expired" || status=="superseded") && (pending.phase=="prepare" || pending.phase=="sent"))
                    {
                        if ((string)d["mode"]!=pending.mode || (pending.mode=="scene" && (string)d["targetScene"]!=pending.targetScene) ||
                            (pending.mode=="line" && Integer(d["targetLine"])!=pending.targetLine)) throw new InvalidOperationException("Leave intent identity mismatch");
                        Finish("离图确认已失效，请重试"); return;
                    }
                    if (status=="prepared" || status=="unknown") return;
                    Message(pending);
                    if (status!="completed" || !d["targetCurrent"].IsBoolean || (string)d["mode"]!=pending.mode ||
                        (pending.mode=="scene" && (string)d["targetScene"]!=pending.targetScene) ||
                        (pending.mode=="line" && Integer(d["targetLine"])!=pending.targetLine) ||
                        (string)d["nativeRequestHash"]!=pending.hash || d["nativeResponse"]==null) throw new InvalidOperationException("Leave receipt identity mismatch");
                    bool targetCurrent=(bool)d["targetCurrent"];
                    byte[] bytes=Convert.FromBase64String((string)d["nativeResponse"]);
                    if (bytes.Length>65536) throw new InvalidOperationException("Invalid original response");
                    int responseOpcode=pending.opcode==30032 ? 30033:30062; object native=PBConvert.toObject(responseOpcode,bytes);
                    if (responseOpcode==30033 && (!(native is SCTransferScene) || ((SCTransferScene)native).result!=1) ||
                        responseOpcode==30062 && !(native is SCTransferLine)) throw new InvalidOperationException("Invalid original response");
                    long rev=Integer(d["positionRevision"]); if (rev<1 || (pending.phase=="position" && pending.positionRevision!=rev))
                        throw new InvalidOperationException("Leave receipt revision mismatch");
                    string previousPhase=pending.phase;
                    bool wasSent=previousPhase=="sent";
                    if (previousPhase!="position")
                    {
                        pending.positionRevision=rev; pending.phase="position";
                        try { Save(owner,pending); }
                        catch (Exception) { pending.positionRevision=0; pending.phase=previousPhase; saveFailed=true; throw; }
                    }
                    warned=false;
                    if (wasSent && targetCurrent && !nativeCallbackUncertain && callbackOwner!=null && originRoom!=null && RoomManager.currentRoom==originRoom)
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
                    receiptTargetCurrent=targetCurrent; receiptVerified=true;
                    if (!targetCurrent) { callbackOwner=null; originRoom=null; }
                    next=0;
                }
                catch (Exception) { Warn(callbackFault ? "请重新登录以恢复离图" : "离图回执待确认"); }
            });
        }
        static bool CurrentRoomMatches(RoomBase room,string scene)
        {
            if (room==null || room!=RoomManager.currentRoom || String.IsNullOrEmpty(scene)) return false;
            if (scene.StartsWith("field_",StringComparison.Ordinal)) return room is RoomField && StaticData.fieldId==scene;
            return (scene=="town" || scene=="town_main") && room is RoomTown;
        }
        void Synchronize()
        {
            RoomBase current=RoomManager.currentRoom;
            if (current==null || current.netManager==null || !NetAPI.SocketIsConnected() || !PositionSync.ChannelReady) return;
            if (receiptTargetCurrent && pending.mode=="scene" && originRoom!=null && current==originRoom) return;
            if (!PositionSync.OwnsLock(positionLease))
            {
                if (PositionSync.Locked || !PositionSync.LockAfterTeleport(Actor(),out positionLease)) return;
            }
            FieldWire.Session owner=session; string id=pending.requestId; int connection=PositionSync.ConnectionEpoch,op=generation;
            RoomNetBase net=current.netManager; long lease=positionLease;
            try
            {
                syncRoom=current; syncNet=net; awaitingPosition=true;
                PositionSync.RequestCurrentState(net,delegate(Vector3 observed,string observedScene)
                {
                    if (!Still(owner,id) || connection!=PositionSync.ConnectionEpoch || op!=generation ||
                        current!=RoomManager.currentRoom || current.netManager!=net || !PositionSync.OwnsLock(lease)) return;
                    awaitingPosition=false; busy=true;
                    wire.PositionStatus(owner,delegate(FieldWireResult result)
                    {
                        if (!Still(owner,id) || connection!=PositionSync.ConnectionEpoch || op!=generation ||
                            current!=RoomManager.currentRoom || current.netManager!=net || !PositionSync.OwnsLock(lease)) return;
                        syncRoom=null; syncNet=null; Later();
                        if (result.uncertain || result.code!=0 || result.data==null) { Warn("位置确认中"); return; }
                        try
                        {
                            JsonData d=result.data; string scene=(string)d["scene"], expected=pending.mode=="line" ? "field_003" : pending.targetScene;
                            if (scene!=observedScene || !CurrentRoomMatches(current,scene) || !d["loading"].IsBoolean || (bool)d["loading"]) throw new InvalidOperationException();
                            long rev=Integer(d["positionRevision"]),confirmed=Integer(d["confirmedRevision"]);
                            if (rev<pending.positionRevision || confirmed!=rev || (Vector(d["position"])-observed).sqrMagnitude>.0001f) throw new InvalidOperationException();
                            if (rev==pending.positionRevision && (!receiptTargetCurrent || scene!=expected)) throw new InvalidOperationException();
                            Finish("结算完成，已离开");
                        }
                        catch (Exception) { Warn("位置确认中"); }
                    });
                });
            }
            catch (Exception) { syncRoom=null; syncNet=null; awaitingPosition=false; next=Stopwatch.GetTimestamp()+Stopwatch.Frequency; }
        }
    }
}
