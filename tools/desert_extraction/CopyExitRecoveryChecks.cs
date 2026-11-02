// Actual recovery source with native transport/UI doubles; no device or DB.
using System;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using HYJS.DesertV4;
using LitJson;
using ProtoBuf.Message;
using Room;

namespace UnityEngine
{
    public class MonoBehaviour { protected static void DontDestroyOnLoad(object value) { } }
    public sealed class GameObject
    {
        public GameObject(string name) { }
        public T AddComponent<T>() where T:new() { return new T(); }
    }
}
namespace Room { public class RoomBase { } public sealed class RoomTown:RoomBase { } public static class RoomManager { public static RoomBase currentRoom; } }
namespace ProtoBuf.Message { public sealed class CSTransferScene { public string target; } }
public static class PBConvert
{
    public static byte[] toBytes(int opcode,object message) { return Encoding.UTF8.GetBytes(((CSTransferScene)message).target); }
    public static object toObject(int opcode,byte[] raw) { return new CSTransferScene { target=Encoding.UTF8.GetString(raw) }; }
}
public static class NetAPI
{
    public static bool connected=true;
    public static readonly List<object> sent=new List<object>();
    public static bool SocketIsConnected() { return connected; }
    public static void SendSocket(int opcode,object value) { if (!CopyExitRecovery.Send(opcode,value)) sent.Add(value); }
}
public static class BoxManager
{
    public static Action<bool> confirm;
    public static readonly List<string> notices=new List<string>();
    public static void CreatePopupTextBox(string value,float ignored) { notices.Add(value); }
    public static void CreateMessageBoxWithCofirmAndCancelButton(string text,Action<bool> action) { confirm=action; }
}
namespace HYJS.DesertV4
{
    public static class PositionSync { public static int ConnectionEpoch; }
    public sealed class FieldWireResult { public bool uncertain; public long code; public JsonData data; }
    public sealed class FieldWire
    {
        public sealed class Session { public long playerId; public string sessionHex,origin; }
        public static Session live;
        public static bool fail;
        public static string token;
        public static readonly Queue<Action<FieldWireResult>> statuses=new Queue<Action<FieldWireResult>>(), leaves=new Queue<Action<FieldWireResult>>();
        public static bool Current(out Session value) { value=live; return value!=null; }
        public void CopyRunStatus(Session owner,Action<FieldWireResult> callback) { if (fail) throw new Exception(); statuses.Enqueue(callback); }
        public void CopyRunLeave(Session owner,string value,Action<FieldWireResult> callback) { if (fail) throw new Exception(); token=value; leaves.Enqueue(callback); }
    }
}
static class CopyExitRecoveryChecks
{
    static int count;
    static readonly BindingFlags Private=BindingFlags.Static|BindingFlags.Instance|BindingFlags.NonPublic;
    static readonly string Token=new string('a',64);
    static void Check(bool ok,string label) { count++; if (!ok) throw new Exception(label); }
    static void Reset()
    {
        typeof(CopyExitRecovery).GetField("instance",Private).SetValue(null,null);
        typeof(CopyExitRecovery).GetField("bypass",Private).SetValue(null,false);
        FieldWire.live=new FieldWire.Session { playerId=67,sessionHex="one",origin="https://example.test" };
        FieldWire.fail=false; FieldWire.token=null; FieldWire.statuses.Clear(); FieldWire.leaves.Clear();
        PositionSync.ConnectionEpoch=1; RoomManager.currentRoom=new RoomTown();
        NetAPI.connected=true; NetAPI.sent.Clear(); BoxManager.confirm=null; BoxManager.notices.Clear();
    }
    static FieldWireResult Result(bool active)
    {
        JsonData data=new JsonData(); data["active"]=active; data["runToken"]=Token; data["chapterId"]="14"; data["missionId"]="81";
        return new FieldWireResult { data=data };
    }
    static void Start() { Check(CopyExitRecovery.Send(30032,new CSTransferScene { target="field_001" }),"town transfer was not held"); Check(NetAPI.sent.Count==0,"unconfirmed transfer sent"); }
    static void HealthyAndFrozen()
    {
        Reset(); var target=new CSTransferScene { target="field_001" }; Check(CopyExitRecovery.Send(30032,target),"query guard"); target.target="field_002";
        FieldWire.statuses.Dequeue()(Result(false)); Check(NetAPI.sent.Count==1 && ((CSTransferScene)NetAPI.sent[0]).target=="field_001","original destination was not frozen");
        Reset(); Start(); FieldWire.statuses.Dequeue()(Result(true)); Check(BoxManager.confirm!=null && FieldWire.leaves.Count==0,"active run needs explicit confirmation");
        BoxManager.confirm(false); Check(FieldWire.leaves.Count==0 && NetAPI.sent.Count==0,"decline changed run or transferred");
        Start(); FieldWire.statuses.Dequeue()(Result(true)); BoxManager.confirm(true);
        Check(FieldWire.token==Token && NetAPI.sent.Count==0,"wrong observed run token or early transfer");
        var late=FieldWire.leaves.Dequeue(); late(Result(false)); late(Result(false)); Check(NetAPI.sent.Count==1,"duplicate result transferred twice");
    }
    static void Failures()
    {
        for (int kind=0;kind<4;kind++)
        {
            Reset(); Start(); FieldWireResult result=kind==0 ? null:Result(true);
            if (kind==1) result.uncertain=true; if (kind==2) result.code=7002; if (kind==3) result.data["runToken"]="bad";
            FieldWire.statuses.Dequeue()(result); Check(NetAPI.sent.Count==0 && FieldWire.leaves.Count==0 && BoxManager.confirm==null,"invalid status permitted exit");
            Start(); Check(FieldWire.statuses.Count==1,"failed query could not retry");
        }
        Reset(); Start(); FieldWire.statuses.Dequeue()(Result(true)); BoxManager.confirm(true); FieldWire.leaves.Dequeue()(new FieldWireResult { uncertain=true });
        Check(NetAPI.sent.Count==0,"uncertain exit transferred"); Start(); FieldWire.statuses.Dequeue()(Result(false)); Check(NetAPI.sent.Count==1,"confirmed absent run did not recover uncertain exit");
        Reset(); FieldWire.fail=true; Start(); Check(BoxManager.notices.Count>0 && NetAPI.sent.Count==0,"transport exception retained busy operation"); FieldWire.fail=false; Start();
        Reset(); Start(); FieldWire.statuses.Dequeue()(Result(true)); FieldWire.fail=true; BoxManager.confirm(true); Check(NetAPI.sent.Count==0,"exit exception transferred");
    }
    static void Scope()
    {
        for (int kind=0;kind<5;kind++)
        {
            Reset(); Start(); var callback=FieldWire.statuses.Dequeue();
            if (kind==0) RoomManager.currentRoom=new RoomTown();
            if (kind==1) FieldWire.live=new FieldWire.Session { playerId=67,sessionHex="two",origin="https://example.test" };
            if (kind==2) FieldWire.live=new FieldWire.Session { playerId=68,sessionHex="one",origin="https://example.test" };
            if (kind==3) PositionSync.ConnectionEpoch++; if (kind==4) NetAPI.connected=false;
            callback(Result(true)); Check(BoxManager.confirm==null && NetAPI.sent.Count==0,"stale context reached confirmation");
        }
        Reset(); Start(); Check(CopyExitRecovery.Send(30032,new CSTransferScene { target="field_002" }) && FieldWire.statuses.Count==1,"duplicate click started competing query");
        FieldWire.statuses.Dequeue()(Result(true)); var confirm=BoxManager.confirm; PositionSync.ConnectionEpoch++; confirm(true);
        Check(FieldWire.leaves.Count==0 && NetAPI.sent.Count==0,"old confirmation changed new session");
        Reset(); RoomManager.currentRoom=new RoomBase(); Check(!CopyExitRecovery.Send(30032,new CSTransferScene { target="field_001" }),"field transfer intercepted");
        Reset(); Check(!CopyExitRecovery.Send(30061,new CSTransferScene { target="field_001" }),"line transfer intercepted");
        Check(!CopyExitRecovery.Send(30032,new object()),"unexpected body intercepted");
    }
    static void Main() { HealthyAndFrozen(); Failures(); Scope(); Console.WriteLine("PASS copy exit source model: "+count+" assertions"); }
}
