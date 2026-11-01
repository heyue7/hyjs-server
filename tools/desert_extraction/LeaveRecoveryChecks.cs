// Isolated behavioral replay of the actual leave and position-sync sources.
// The host supplies native UI/network stubs; this is not a device test.
using System;
using System.Collections.Generic;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using HYJS.DesertV4;
using LitJson;
using NetWork.Packets;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace UnityEngine
{
    public partial struct Vector3
    {
        public static Vector3 operator -(Vector3 a,Vector3 b) { return new Vector3(a.x-b.x,a.y-b.y,a.z-b.z); }
        public float sqrMagnitude { get { return x*x+y*y+z*z; } }
    }
    public class Object { protected static void DontDestroyOnLoad(Object value) { } protected static void Destroy(Object value) { } }
    public class MonoBehaviour : Object { }
    public sealed class Transform { public Vector3 localPosition; }
    public sealed class GameObject : Object
    {
        public GameObject(string name) { }
        public T AddComponent<T>() where T : new() { return new T(); }
        public T GetComponent<T>() where T : new() { return new T(); }
        public readonly Transform transform=new Transform();
        public void SetActive(bool active) { }
    }
    public static class PlayerPrefs
    {
        public static readonly Dictionary<string,string> items=new Dictionary<string,string>();
        public static bool failDelete;
        public static string GetString(string key,string fallback) { string value; return items.TryGetValue(key,out value) ? value : fallback; }
        public static void SetString(string key,string value) { items[key]=value; }
        public static void DeleteKey(string key) { if (failDelete) throw new InvalidOperationException("isolated save failure"); items.Remove(key); }
        public static void Save() { }
    }
    public static class JsonUtility
    {
        static readonly Dictionary<string,object> snapshots=new Dictionary<string,object>();
        static int serial;
        static object Copy(object value)
        {
            object copy=Activator.CreateInstance(value.GetType(),true);
            foreach (FieldInfo field in value.GetType().GetFields(BindingFlags.Instance|BindingFlags.Public)) field.SetValue(copy,field.GetValue(value));
            return copy;
        }
        public static string ToJson(object value) { string key="json-"+(++serial); snapshots[key]=Copy(value); return key; }
        public static T FromJson<T>(string value) { return (T)Copy(snapshots[value]); }
    }
}
namespace ProtoBuf.Message
{
    public sealed class CSTransferScene { public string target; }
    public sealed class CSTransferLine { public int line; }
    public sealed class SCTransferScene { public int result; }
    public sealed class SCTransferLine { }
    public sealed class ErrorMessage { }
}
namespace Room
{
    public sealed class RoomField : RoomBase { }
    public sealed class RoomTown : RoomBase { }
    public sealed class ObservedNet : RoomNetBase
    {
        public int requests;
        public override void SendCSCurrentPosition(object ignored) { requests++; base.SendCSCurrentPosition(ignored); }
    }
}
public static class StaticData { public static string fieldId; }
public static class BoxManager
{
    public static readonly List<string> notices=new List<string>();
    public static Action<bool> confirmation;
    public static GameObject m_kBox;
    public static void CreatePopupTextBox(string text,float delay) { notices.Add(text); }
    public static void CreateMessageBoxWithCofirmAndCancelButton(string text,Action<bool> action) { confirmation=action; }
    public static void CreateCountdownBoxTwoButton(string text,bool mode,float seconds,Action<bool> action,string value) { }
}
public sealed class MessageBoxCountdown { public GameObject m_kOKBtn=new GameObject("ok"),m_kCancelBtn=new GameObject("cancel"); }
public sealed class ManagerController
{
    public static readonly ManagerController Instance=new ManagerController();
    public int calls;
    public bool fail;
    public void SceneTransferCallback(object value) { calls++; if (fail) throw new InvalidOperationException("isolated callback failure"); }
}
public static class PBConvert
{
    public static byte[] toBytes(int opcode,object value)
    {
        if (opcode==30032) return Encoding.UTF8.GetBytes("scene:"+((CSTransferScene)value).target);
        if (opcode==30061) return Encoding.UTF8.GetBytes("line:"+((CSTransferLine)value).line);
        throw new InvalidOperationException("unexpected request opcode");
    }
    public static object toObject(int opcode,byte[] bytes)
    {
        string text=Encoding.UTF8.GetString(bytes);
        if (opcode==30032 && text.StartsWith("scene:")) return new CSTransferScene { target=text.Substring(6) };
        if (opcode==30061 && text.StartsWith("line:")) return new CSTransferLine { line=Int32.Parse(text.Substring(5)) };
        if (opcode==30033 && text=="ok") return new SCTransferScene { result=1 };
        if (opcode==30062 && bytes.Length==0) return new SCTransferLine();
        throw new InvalidOperationException("invalid native bytes");
    }
}
public static partial class NetAPI
{
    public static int nativeTransfers;
    public static void SendSocket(int opcode,object value)
    { if (!LeaveController.Send(opcode,value)) nativeTransfers++; }
}
namespace HYJS.DesertV4
{
    public sealed class FieldWireResult { public bool uncertain; public long code; public JsonData data; }
    public sealed partial class FieldWire
    {
        public static readonly Queue<Action<FieldWireResult>> statuses=new Queue<Action<FieldWireResult>>();
        public static readonly Queue<Action<FieldWireResult>> positions=new Queue<Action<FieldWireResult>>();
        public static int prepares;
        public void LeavePrepare(Session owner,string id,string mode,string scene,int line,Action<FieldWireResult> action) { prepares++; }
        public void LeaveStatus(Session owner,string id,Action<FieldWireResult> action) { statuses.Enqueue(action); }
        public void PositionStatus(Session owner,Action<FieldWireResult> action) { positions.Enqueue(action); }
        public void Entry(Session owner,string id,Action<FieldWireResult> action) { }
        public void ExtractionStart(Session owner,string id,string point,Action<FieldWireResult> action) { }
        public void ExtractionCancel(Session owner,string cancel,string id,Action<FieldWireResult> action) { }
        public void ExtractionStatus(Session owner,string id,Action<FieldWireResult> action) { }
    }
    public sealed class FieldRuntimeStub { public void RequestSnapshot() { } }
    public static partial class FieldRuntime { public static FieldRuntimeStub Require() { return new FieldRuntimeStub(); } }
    public static class BattleRecovery { public static bool HasUnsettled() { return false; } }
}

public static class LeaveRecoveryChecks
{
    const string Request="b7c33870-2204-49ae-b9dd-f1bf7143d8da";
    static readonly BindingFlags Private=BindingFlags.Static|BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic;
    static int checks;
    static FieldWire.Session session;
    static RoomTown town;
    static RoomCharactor actor;
    static ObservedNet net;
    static LeaveRecord original;
    static string key;
    sealed class SocketBox { public object socket; }
    sealed class SocketCallback : IAsyncResult
    {
        public object AsyncState { get; set; }
        public System.Threading.WaitHandle AsyncWaitHandle { get { return null; } }
        public bool CompletedSynchronously { get { return false; } }
        public bool IsCompleted { get { return true; } }
    }
    static void Check(bool condition,string message) { checks++; if (!condition) throw new Exception(message); }
    static object Core() { return typeof(LeaveController).GetField("instance",Private).GetValue(null); }
    static void Set(object value,string name,object data) { value.GetType().GetField(name,Private).SetValue(value,data); }
    static object Get(object value,string name) { return value.GetType().GetField(name,Private).GetValue(value); }
    static void Tick() { typeof(LeaveController).GetMethod("Update",Private).Invoke(Core(),null); }
    static void Now() { Set(Core(),"next",0L); }
    static string Hash(byte[] bytes)
    {
        using (SHA256 hash=SHA256.Create())
        { StringBuilder s=new StringBuilder(); foreach (byte b in hash.ComputeHash(bytes)) s.Append(b.ToString("x2")); return s.ToString(); }
    }
    static void Reset(string phase)
    {
        PositionSync.Release(); PositionSync.SetFieldActive(false); PositionSync.ConnectionStarting();
        typeof(LeaveController).GetField("instance",Private).SetValue(null,null);
        typeof(TravelController).GetField("instance",Private).SetValue(null,null);
        FieldWire.statuses.Clear(); FieldWire.positions.Clear(); FieldWire.prepares=0;
        PlayerPrefs.items.Clear(); PlayerPrefs.failDelete=false; BoxManager.notices.Clear(); BoxManager.confirmation=null;
        ManagerController.Instance.calls=0; ManagerController.Instance.fail=false; NetAPI.connected=true; NetAPI.nativeTransfers=0;
        session=new FieldWire.Session { playerId=69,sessionHex="session-a",origin="https://example.test" }; FieldWire.session=session;
        actor=new RoomCharactor(); net=new ObservedNet(); town=new RoomTown { charactorManager=new RoomCharactorManager { self=actor },netManager=net };
        RoomManager.currentRoom=town; StaticData.fieldId="field_001"; EasyTouchTool.Instance.Enable=true;
        PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        byte[] raw=PBConvert.toBytes(30032,new CSTransferScene { target="field_001" });
        original=new LeaveRecord { version=1,opcode=30032,playerId=69,origin=session.origin,requestId=Request,mode="scene",targetScene="field_001",
            payload=Convert.ToBase64String(raw),hash=Hash(raw),phase=phase,positionRevision=phase=="position" ? 1331 : 0 };
        key="hyjs.ggo.leave.v4."+Uri.EscapeDataString(session.origin)+".69";
        PlayerPrefs.SetString(key,JsonUtility.ToJson(original)); LeaveController.SceneReady();
        Check(Core()!=null && LeaveController.Pending,"saved leave was not adopted");
    }
    static JsonData Receipt(bool current,long version)
    {
        JsonData d=new JsonData(); d["requestId"]=Request; d["status"]="completed"; d["mode"]="scene";
        d["targetScene"]="field_001"; d["nativeRequestHash"]=original.hash; d["nativeResponse"]=Convert.ToBase64String(Encoding.UTF8.GetBytes("ok"));
        d["positionRevision"]=version; d["targetCurrent"]=current; return d;
    }
    static JsonData PositionStatus(string scene,long revision,long confirmed,bool loading,float x)
    {
        JsonData d=new JsonData(),p=new JsonData(); p["x"]=x; p["y"]=0; p["z"]=0;
        d["scene"]=scene; d["position"]=p; d["positionRevision"]=revision; d["confirmedRevision"]=confirmed; d["loading"]=loading; return d;
    }
    static void ReplyStatus(JsonData data)
    { Check(FieldWire.statuses.Count==1,"one receipt read expected"); FieldWire.statuses.Dequeue()(new FieldWireResult { data=data }); }
    static void ReplyPosition(JsonData data)
    { Check(FieldWire.positions.Count==1,"one position read expected"); FieldWire.positions.Dequeue()(new FieldWireResult { data=data }); }
    static void NativePosition(string scene,float x,long player)
    {
        var body=new SCCurrentPosition { self=new MovePlayer { info=new MovePlayerInfo { playerId=player },
            position=new Position { scene=scene,current=new PointNew { x=x,y=0,z=0 } } } };
        Packet packet=new Packet { kBody=body }; PositionSync.TrackPacket(packet);
        Check(!PositionSync.DropPacket(packet),"fresh native position was dropped");
        PositionSync.TrackBaseResponse(net,body);
    }
    static void ReceiptThenBaseline()
    {
        Tick(); Check(net.requests==1 && FieldWire.statuses.Count==1,"town must request a real baseline with fieldActive false");
        ReplyStatus(Receipt(false,1331));
        Check(LeaveController.Pending && !PositionSync.Locked && ManagerController.Instance.calls==0,"old receipt replayed or unlocked before current frame");
        NativePosition("town_main",12,69);
        Check(PositionSync.ChannelReady,"actual town baseline was not accepted");
        Tick(); Check(PositionSync.Locked && net.requests==2 && LeaveController.Pending,"current position must be verified under an owned lock");
        NativePosition("town_main",12,69);
        Check(FieldWire.positions.Count==1,"native 30024 did not trigger authenticated status read");
    }
    static void SupersededPosition()
    {
        foreach (string phase in new string[] { "prepare","sent","position" })
        {
            Reset(phase); ReceiptThenBaseline();
            ReplyPosition(PositionStatus("town_main",1357,1357,false,12));
            Check(!LeaveController.Pending && !PositionSync.Locked && EasyTouchTool.Instance.Enable,"newer verified town position did not clear and unlock");
            Check(!PlayerPrefs.items.ContainsKey(key) && ManagerController.Instance.calls==0 && FieldWire.prepares==0 && NetAPI.nativeTransfers==0,
                "superseded leave was replayed or settled again");
        }
    }
    static void InvalidProofsRetain()
    {
        string[] cases={"not-confirmed","same-revision","loading","wrong-scene","wrong-position"};
        foreach (string name in cases)
        {
            Reset("position"); ReceiptThenBaseline();
            JsonData d=PositionStatus(name=="wrong-scene" ? "field_001" : "town_main",name=="same-revision" ? 1331 : 1357,
                name=="not-confirmed" ? 0 : name=="same-revision" ? 1331 : 1357,name=="loading",name=="wrong-position" ? 13 : 12);
            ReplyPosition(d);
            Check(LeaveController.Pending && PositionSync.Locked && PlayerPrefs.items.ContainsKey(key),"invalid position proof cleared "+name);
            Check(ManagerController.Instance.calls==0 && NetAPI.nativeTransfers==0,"invalid proof replayed transfer "+name);
        }
        foreach (string name in new string[] { "request","target","hash","response","revision" })
        {
            Reset("position"); Tick(); JsonData d=Receipt(false,1331);
            if (name=="request") d["requestId"]="another";
            if (name=="target") d["targetScene"]="field_002";
            if (name=="hash") d["nativeRequestHash"]="different";
            if (name=="response") d["nativeResponse"]=Convert.ToBase64String(Encoding.UTF8.GetBytes("bad"));
            if (name=="revision") d["positionRevision"]=1332;
            ReplyStatus(d);
            Check(LeaveController.Pending && PlayerPrefs.items.ContainsKey(key) && !(bool)Get(Core(),"receiptVerified"),"invalid receipt accepted: "+name);
        }
    }
    static void SaveFailureAndOwnership()
    {
        Reset("position"); ReceiptThenBaseline(); PlayerPrefs.failDelete=true;
        ReplyPosition(PositionStatus("town_main",1357,1357,false,12));
        Check(LeaveController.Pending && PositionSync.Locked && PlayerPrefs.items.ContainsKey(key),"failed delete released owned lock or record");
        PlayerPrefs.failDelete=false; Now(); Tick(); NativePosition("town_main",12,69);
        ReplyPosition(PositionStatus("town_main",1357,1357,false,12));
        Check(!LeaveController.Pending && !PositionSync.Locked,"safe retry after delete failure did not complete");

        Reset("position"); ReceiptThenBaseline();
        Action<FieldWireResult> late=FieldWire.positions.Dequeue();
        FieldWire.session=new FieldWire.Session { playerId=70,sessionHex="session-b",origin=session.origin };
        Check(!LeaveController.Send(30032,new CSTransferScene { target="field_002" }),"another role was blocked by old owner's leave");
        late(new FieldWireResult { data=PositionStatus("town_main",1357,1357,false,12) });
        Check(PlayerPrefs.items.ContainsKey(key) && !PositionSync.Locked,"old callback modified a new role or deleted the old record");
    }
    static void ReconnectAndOldFrames()
    {
        Reset("position");
        Set(Core(),"callbackFault",true); Set(Core(),"epoch",PositionSync.ConnectionEpoch);
        Tick(); Check(FieldWire.statuses.Count==0,"callback fault retried on the same connection");
        PositionSync.ConnectionStarting(); PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        Tick(); Check(!(bool)Get(Core(),"callbackFault") && FieldWire.statuses.Count==1,"new connection did not restart receipt recovery");
        Action<FieldWireResult> stale=FieldWire.statuses.Dequeue();
        PositionSync.ConnectionStarting(); PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        stale(new FieldWireResult { data=Receipt(false,1331) });
        Check(!(bool)Get(Core(),"receiptVerified") && PlayerPrefs.items.ContainsKey(key),"old connection receipt was accepted");
        Tick(); ReplyStatus(Receipt(false,1331)); NativePosition("town_main",12,69); Tick(); NativePosition("town_main",12,69);
        ReplyPosition(PositionStatus("town_main",1357,1357,false,12));
        Check(!LeaveController.Pending && !PositionSync.Locked,"same-process reconnect did not recover callback fault");
    }
    static bool Incoming(int opcode,object owner,object body,out bool dropped)
    {
        Packet packet=new Packet { kBody=body }; LeaveController.TrackPacket(packet); dropped=LeaveController.DropPacket(packet);
        return !dropped && LeaveController.Response(opcode,owner,body);
    }
    static void Ticket(bool own,int opcode,FieldWire.Session owner,object callback,string id)
    { typeof(LeaveController).GetMethod("TrackTransferSend",Private).Invoke(null,new object[] { opcode,own,owner,callback,id }); }
    static void OriginalReplyScope()
    {
        Reset("sent"); object callback=ManagerController.Instance; bool dropped;
        Check(LeaveController.Send(30032,new CSTransferScene { target="town_main" }),"pending leave allowed another scene request");
        Check(LeaveController.Send(30061,new CSTransferLine { line=2 }),"pending leave allowed another line request");
        Ticket(true,30032,session,callback,Request);
        var own=new SCTransferScene { result=1 };
        Check(Incoming(30033,callback,own,out dropped) && !dropped,"own sent reply was not intercepted");
        Check(LeaveController.Response(30033,callback,own),"duplicate callback for the same body escaped");
        Check(!Incoming(30033,callback,new SCTransferScene { result=1 },out dropped),"unsolicited response was swallowed");
        Ticket(true,30032,session,callback,Request);
        Packet stale=new Packet { kBody=new SCTransferScene { result=1 } }; LeaveController.TrackPacket(stale);
        PositionSync.ConnectionStarting(); Check(LeaveController.DropPacket(stale),"queued old-connection transfer was delivered to a new session");
        Check(!Incoming(30033,callback,new SCTransferScene { result=1 },out dropped),"new connection swallowed an unrelated response");
        Ticket(true,30032,session,callback,Request);
        Check(!Incoming(30033,new object(),new SCTransferScene { result=1 },out dropped),"different original callback was swallowed");
        Check(Incoming(30033,callback,new SCTransferScene { result=1 },out dropped),"different callback consumed the own ticket");

        Reset("sent"); Ticket(true,30032,session,callback,Request);
        Packet prior=new Packet { kBody=new SCTransferScene { result=1 } }; LeaveController.TrackPacket(prior);
        FieldWire.session=new FieldWire.Session { playerId=70,sessionHex="other",origin=session.origin };
        Check(LeaveController.DropPacket(prior),"previous role's queued transfer reached a new role");
        Check(!LeaveController.Send(30032,new CSTransferScene { target="town_main" }),"new role's ordinary transfer was blocked");
        Check(!Incoming(30033,callback,new SCTransferScene { result=1 },out dropped),"new role's reply was swallowed by an old ticket");
    }
    static void MissingEarlierTicketAndLateStatus()
    {
        Reset("sent"); object callback=ManagerController.Instance; bool dropped;
        Ticket(false,30032,session,callback,null);
        bool ready=(bool)typeof(LeaveController).GetMethod("CanSendTransfer",Private).Invoke(null,new object[] { 30032 });
        Check(!ready,"missing earlier same-type reply did not block the new native send");
        Check(!(bool)typeof(LeaveController).GetMethod("CanSendTransfer",Private).Invoke(null,new object[] { 30061 }),"an earlier scene request allowed a competing line change");
        PositionSync.ConnectionStarting();
        Check((bool)typeof(LeaveController).GetMethod("CanSendTransfer",Private).Invoke(null,new object[] { 30032 }),"new connection did not clear old send ordering");
        Ticket(false,30061,session,callback,null);
        Check(!(bool)typeof(LeaveController).GetMethod("CanSendTransfer",Private).Invoke(null,new object[] { 30032 }),"an earlier line request allowed a competing scene change");

        Reset("sent"); Set(Core(),"callbackOwner",callback); Set(Core(),"originRoom",town);
        Ticket(false,30032,session,callback,null); Ticket(true,30032,session,callback,Request);
        var originalReply=new SCTransferScene { result=1 };
        Check(!Incoming(30033,callback,originalReply,out dropped),"ambiguous same-type reply was swallowed");
        ManagerController.Instance.SceneTransferCallback(originalReply);
        Check(!Incoming(30033,callback,new SCTransferScene { result=1 },out dropped),"ambiguous later response was swallowed");
        Tick(); ReplyStatus(Receipt(true,1331));
        Check(ManagerController.Instance.calls==1 && LeaveController.Pending,"late HTTP replayed an original callback");
        NativePosition("town_main",12,69);
        RoomManager.currentRoom=new RoomField { charactorManager=new RoomCharactorManager { self=actor },netManager=net };
        StaticData.fieldId="field_001"; Now(); Tick(); NativePosition("field_001",13,69);
        ReplyPosition(PositionStatus("field_001",1331,1331,false,13));
        Check(!LeaveController.Pending && !PositionSync.Locked && ManagerController.Instance.calls==1,"handled original reply left the player locked");

        Reset("sent"); Set(Core(),"callbackOwner",callback); Set(Core(),"originRoom",town);
        var untracked=new SCTransferScene { result=1 };
        Check(!LeaveController.Response(30033,callback,untracked),"untracked native callback was swallowed");
        ManagerController.Instance.SceneTransferCallback(untracked);
        Tick(); ReplyStatus(Receipt(true,1331));
        Check(ManagerController.Instance.calls==1 && (bool)Get(Core(),"nativeCallbackUncertain"),"untracked original callback was replayed");
    }
    static void OrdinaryTransferAndLateOwnReply()
    {
        Reset("sent"); PlayerPrefs.items.Clear(); typeof(LeaveController).GetField("instance",Private).SetValue(null,null);
        bool dropped; object callback=ManagerController.Instance;
        Check(!LeaveController.Send(30032,new CSTransferScene { target="field_001" }),"ordinary non-desert scene request was blocked");
        Packet error=new Packet { kBody=new ErrorMessage() }; LeaveController.TrackPacket(error);
        Check(!LeaveController.DropPacket(error),"ordinary error frame was swallowed");
        Check(!LeaveController.Send(30061,new CSTransferLine { line=2 }),"ordinary non-desert line request was blocked after error");
        Check(!Incoming(30033,callback,new SCTransferScene { result=1 },out dropped),"ordinary response was swallowed after an error");

        Reset("position"); ReceiptThenBaseline();
        Action<FieldWireResult> late=FieldWire.positions.Dequeue();
        Set(Core(),"callbackOwner",callback); Ticket(true,30032,session,callback,Request);
        Check(Incoming(30033,callback,new SCTransferScene { result=1 },out dropped),"late own reply was not recognized");
        late(new FieldWireResult { data=PositionStatus("town_main",1357,1357,false,12) });
        Check(LeaveController.Pending && !(bool)Get(Core(),"awaitingPosition") && PlayerPrefs.items.ContainsKey(key),"late position callback cleared state or left wait stuck");
        Now(); Tick(); ReplyStatus(Receipt(false,1331)); Tick(); NativePosition("town_main",12,69);
        ReplyPosition(PositionStatus("town_main",1357,1357,false,12));
        Check(!LeaveController.Pending && !PositionSync.Locked,"late own reply prevented recovery");
    }
    static void PreparedOutsideDesertDoesNotSend()
    {
        Reset("prepare"); Tick();
        JsonData prepared=new JsonData(); prepared["requestId"]=Request; prepared["status"]="prepared";
        ReplyStatus(prepared); Now(); Tick();
        Check(FieldWire.prepares==0 && NetAPI.nativeTransfers==0 && LeaveController.Pending,
            "an old prepare in town resent a desert transition");
    }
    static void OldSocketBeforeEnqueue()
    {
        Reset("position");
        object oldSocket=new object(),newSocket=new object();
        var tool=new SocketBox { socket=newSocket };
        var oldCallback=new SocketCallback { AsyncState=new SocketBox { socket=oldSocket } };
        var freshCallback=new SocketCallback { AsyncState=new SocketBox { socket=newSocket } };
        PositionSync.ConnectionStarting();
        Check(PositionSync.DropOldSocket(tool,oldCallback),"old socket reached the new epoch while town leave was pending");
        Check(!PositionSync.DropOldSocket(tool,freshCallback),"current socket was rejected during town leave recovery");
        Check(PositionSync.DropOldSocket(tool,null),"unknown socket callback passed during leave recovery");
        PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        Tick(); ReplyStatus(Receipt(false,1331)); NativePosition("town_main",12,69); Tick(); NativePosition("town_main",12,69);
        ReplyPosition(PositionStatus("town_main",1357,1357,false,12));
        Check(!LeaveController.Pending && !PositionSync.DropOldSocket(tool,oldCallback),"ordinary town socket behavior was not restored after leave completion");
    }
    static void PrepareDesert()
    {
        Reset("position");
        PlayerPrefs.items.Clear(); typeof(LeaveController).GetField("instance",Private).SetValue(null,null); PositionSync.SetLeaveActive(false);
        PositionSync.Release(); PositionSync.SetFieldActive(true); PositionSync.ConnectionStarting();
        PositionSync.LoginResponse(new SCSocketLogin { result=100 });
        RoomManager.currentRoom=new RoomField { charactorManager=new RoomCharactorManager { self=actor },netManager=net };
        StaticData.fieldId="field_003"; net.SendCSCurrentPosition(null); NativePosition("field_003",12,69);
        Check(PositionSync.ChannelReady,"desert baseline unavailable");
    }
    static void PersistTravel(string kind,bool corrupt)
    {
        string travelKey="hyjs.ggo.travel.v4."+Uri.EscapeDataString(session.origin)+".69";
        var record=new TravelRecord { version=1,playerId=69,origin=session.origin,requestId=Request,kind=kind,phase="start",pointId=kind=="extraction" ? "exit-1" : null };
        PlayerPrefs.SetString(travelKey,corrupt ? "invalid-json" : JsonUtility.ToJson(record));
    }
    static void TravelPersistenceGate()
    {
        foreach (string kind in new string[] { "entry","extraction" })
        {
            PrepareDesert(); PersistTravel(kind,false);
            Check(TravelController.Pending && !TravelController.PendingExtraction,"fixture must precede travel instance restoration");
            Check(LeaveController.Send(30032,new CSTransferScene { target="field_001" }),"persisted "+kind+" did not block leaving");
            Check(BoxManager.confirmation==null && !LeaveController.Pending && !PositionSync.Locked && NetAPI.nativeTransfers==0,
                "persisted "+kind+" was changed or a new leave was started");
        }
        PrepareDesert(); PersistTravel("entry",true);
        Check(TravelController.Pending,"unreadable travel record was not treated as pending");
        Check(LeaveController.Send(30032,new CSTransferScene { target="field_001" }) && BoxManager.confirmation==null,
            "corrupt travel record allowed a new leave");
        PrepareDesert();
        Check(!TravelController.Pending,"empty travel store looked pending");
        Check(LeaveController.Send(30032,new CSTransferScene { target="field_001" }) && BoxManager.confirmation!=null,
            "normal desert leave did not present its confirmation");
        BoxManager.confirmation(true);
        Check(LeaveController.Pending && PositionSync.Locked,"normal desert leave did not acquire its owned lock");

        PrepareDesert(); TravelController.Entry(); Check(BoxManager.confirmation!=null,"entry confirmation missing");
        BoxManager.confirmation(true); Check(TravelController.Pending && PositionSync.Locked,"entry did not establish its real travel lock");
        BoxManager.confirmation=null;
        Check(LeaveController.Send(30032,new CSTransferScene { target="field_001" }) && BoxManager.confirmation==null && !LeaveController.Pending,
            "active entry permitted a competing leave");
        RoomManager.currentRoom=town; StaticData.fieldId="field_001";
        typeof(TravelController).GetMethod("Update",Private).Invoke(typeof(TravelController).GetField("instance",Private).GetValue(null),null);
        Check(PositionSync.Locked && TravelController.Pending,"forced outside-scene entry did not reproduce retained travel lock");

        PrepareDesert(); TravelController.Extraction("exit-1"); Check(BoxManager.confirmation!=null,"extraction confirmation missing");
        BoxManager.confirmation(true); BoxManager.confirmation=null;
        Check(TravelController.PendingExtraction && LeaveController.Send(30032,new CSTransferScene { target="field_001" }) && BoxManager.confirmation==null,
            "loaded extraction permitted a competing leave");
    }
    public static void Main()
    {
        SupersededPosition(); InvalidProofsRetain(); SaveFailureAndOwnership(); ReconnectAndOldFrames(); OriginalReplyScope();
        MissingEarlierTicketAndLateStatus(); OrdinaryTransferAndLateOwnReply();
        PreparedOutsideDesertDoesNotSend(); OldSocketBeforeEnqueue(); TravelPersistenceGate();
        Console.WriteLine("PASS leave recovery: "+checks+" assertions");
    }
}
