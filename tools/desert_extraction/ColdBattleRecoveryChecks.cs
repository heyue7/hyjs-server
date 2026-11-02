// Exercise the real recovery component with deterministic transport callbacks.
using System;
using System.Collections.Generic;
using System.Reflection;
using HYJS.DesertV4;
using LitJson;
using NetWork.Packets;
using Room;

namespace UnityEngine
{
    public class MonoBehaviour { protected static void DontDestroyOnLoad(object value) { } }
    public class GameObject
    {
        public GameObject(string name) { }
        public T AddComponent<T>() where T:new() { return new T(); }
    }
    public static class Time { public static float realtimeSinceStartup; }
}
namespace LitJson
{
    public sealed class JsonData : Dictionary<string,JsonData>
    {
        readonly object scalar;
        public JsonData() { }
        public JsonData(object value) { scalar=value; }
        public bool IsBoolean { get { return scalar is bool; } }
        public static explicit operator bool(JsonData value) { return (bool)value.scalar; }
        public override string ToString() { return scalar==null ? "" : scalar.ToString(); }
    }
}
namespace Room
{
    public class RoomBase { public object netManager=new object(); }
    public sealed class RoomField : RoomBase { }
    public static class RoomManager { public static RoomBase currentRoom; }
}
namespace NetWork.Packets
{
    public sealed class Packet { public object kBody; }
}
namespace ProtoBuf.Message
{
    public sealed class CSPingTcp { public long clientTime; }
    public sealed class SCPingTcp { public long clientTime; }
    public sealed class SCSidePveFightEnd { public int code; }
    public sealed class SCSidePveBossFightEnd { public int code; }
    public sealed class SCSidePveFightStart { public int code; public string fightId; }
    public sealed class SCSidePveBossFightStart { public int code; public string fightId; }
}
public sealed class BattleData { public string fightId; }
public static class StaticData
{
    public static string fieldId,pveBossUUID;
    public static BattleData curBattleData;
}
public static class PBConvert { public static object toObject(int opcode,byte[] bytes) { return null; } }
public static class NetAPI
{
    public sealed class Send { public int opcode; public object body; }
    public static readonly List<Send> sends=new List<Send>();
    public static bool SocketIsConnected() { return true; }
    public static void SendSocket(int opcode,object body) { sends.Add(new Send { opcode=opcode,body=body }); }
    public static void Reconnect() { throw new Exception("Unexpected reconnect"); }
}
public static class BoxManager
{
    public static int prompts,warnings;
    public static void CreatePopupTextBox(string message,float delay) { warnings++; }
    public static void CreateMessageBoxWithoutCancelButton(string text,bool show,Action<bool> done,string label,bool extra) { prompts++; }
}
namespace HYJS.DesertV4
{
    public sealed class FieldWireResult { public bool uncertain; public long code; public JsonData data; }
    public sealed class FieldWire
    {
        public sealed class Session { public long playerId; public string sessionHex,origin; }
        public sealed class Call { public int type; public Action<FieldWireResult> callback; }
        public static Session current;
        public static readonly List<Call> calls=new List<Call>();
        public static bool Current(out Session value) { value=current; return value!=null; }
        public void FightActive(Session owner,int type,Action<FieldWireResult> callback) { calls.Add(new Call { type=type,callback=callback }); }
        public void ResumeFight(Session owner,int type,string fight,string hash,Action<FieldWireResult> callback) { throw new Exception("unexpected resume"); }
    }
    public static class PositionSync { public static int ConnectionEpoch; }
    public static class BattleRecovery
    {
        public static bool journal,live;
        public static bool HasEndJournal() { return journal; }
        public static bool LivePresent { get { return live; } }
        public static void AdmitResume(int type,string fight,string hash) { }
    }
    public sealed class FieldRuntime
    {
        public static bool InteractionReady;
        public static int SceneVersion;
        public static void ClearEncounterWait() { }
        public static FieldRuntime Require() { return new FieldRuntime(); }
        public void RequestSnapshot() { }
        public static bool ResumeEncounter(int type,object native) { return false; }
    }
}
public static class ColdBattleRecoveryChecks
{
    static int checks;
    static readonly FieldInfo Instance=typeof(ColdBattleRecovery).GetField("instance",BindingFlags.Static|BindingFlags.NonPublic);
    static readonly MethodInfo Update=typeof(ColdBattleRecovery).GetMethod("Update",BindingFlags.Instance|BindingFlags.NonPublic);
    static void Check(bool value,string message) { checks++; if (!value) throw new Exception(message); }
    static void Tick() { Update.Invoke(Instance.GetValue(null),null); }
    static void EchoFence(int index)
    {
        Check(EncounterStartRecovery.WaitingForFence,"connection did not wait for an echo");
        Check(index<NetAPI.sends.Count && NetAPI.sends[index].opcode==30020,"connection did not send its native ping");
        ProtoBuf.Message.CSPingTcp ping=NetAPI.sends[index].body as ProtoBuf.Message.CSPingTcp;
        Check(ping!=null,"connection fence body was not CSPingTcp");
        Packet packet=new Packet { kBody=new ProtoBuf.Message.SCPingTcp { clientTime=ping.clientTime } };
        EncounterStartRecovery.TrackPacket(packet);
        Check(!EncounterStartRecovery.DropPacket(packet),"matching native ping echo was dropped");
        Check(!EncounterStartRecovery.WaitingForFence,"matching native ping echo did not release fence");
    }
    static void Reset()
    {
        FieldWire.calls.Clear(); FieldWire.current=new FieldWire.Session { playerId=42,sessionHex="session",origin="origin" };
        RoomManager.currentRoom=new RoomField(); StaticData.fieldId="field_003"; StaticData.curBattleData=null;
        PositionSync.ConnectionEpoch++; EncounterStartRecovery.ConnectionStarting(); NetAPI.sends.Clear();
        FieldRuntime.SceneVersion++; FieldRuntime.InteractionReady=false;
        BattleRecovery.journal=BattleRecovery.live=false; UnityEngine.Time.realtimeSinceStartup=0; BoxManager.prompts=BoxManager.warnings=0;
        ColdBattleRecovery.SceneReady(RoomManager.currentRoom,FieldRuntime.SceneVersion);
        Tick(); Check(FieldWire.calls.Count==0,"scene discovery started before its connection echo");
        EchoFence(0);
    }
    static FieldWireResult Inactive()
    {
        JsonData d=new JsonData(); d["active"]=new JsonData(false); return new FieldWireResult { data=d };
    }
    static FieldWireResult ActiveBoss()
    {
        JsonData d=new JsonData(); d["active"]=new JsonData(true); d["fightId"]=new JsonData(new string('a',24));
        d["inputHash"]=new JsonData(new string('b',64)); d["eventId"]=new JsonData("ggo-event");
        return new FieldWireResult { data=d };
    }
    static void Reply(int index,FieldWireResult value) { FieldWire.calls[index].callback(value); }
    static void ReadWithoutPositionReadiness()
    {
        Reset(); Check(ColdBattleRecovery.Pending,"unknown server state must block exit"); Tick();
        Check(FieldWire.calls.Count==1 && FieldWire.calls[0].type==6,"monster discovery should run before UI is ready");
        Reply(0,Inactive()); Check(FieldWire.calls.Count==2 && FieldWire.calls[1].type==12,"boss discovery not requested");
        Reply(1,Inactive()); Check(!ColdBattleRecovery.Pending,"two authoritative inactive responses did not release exit");
        Check(BoxManager.prompts==0,"an inactive fight produced a prompt");
    }
    static void ActiveFightStillBlocks()
    {
        Reset(); Tick(); Reply(0,Inactive()); Reply(1,ActiveBoss());
        Check(ColdBattleRecovery.Pending,"an active boss fight was released"); Tick();
        Check(BoxManager.prompts==0,"resume was prompted while UI was unavailable");
        FieldRuntime.InteractionReady=true; Tick();
        Check(BoxManager.prompts==1 && ColdBattleRecovery.Pending,"active fight was not preserved for resume");
    }
    static void FailedReadsKeepRetrying()
    {
        Reset();
        for (int i=0;i<4;i++)
        {
            Tick(); Check(FieldWire.calls.Count==i+1,"discovery stopped after a transient error");
            Reply(i,new FieldWireResult { uncertain=true });
            Check(ColdBattleRecovery.Pending,"uncertain server response released exit");
            UnityEngine.Time.realtimeSinceStartup+=31;
        }
        Tick(); Check(FieldWire.calls.Count==5,"discovery did not retry after three failures");
        Reply(4,Inactive()); Reply(5,Inactive());
        Check(!ColdBattleRecovery.Pending && BoxManager.warnings==1,"successful retry did not release the gate");
    }
    static void ReconnectInvalidatesOldCallbacks()
    {
        Reset(); Tick(); int nextPing=NetAPI.sends.Count;
        PositionSync.ConnectionEpoch++; EncounterStartRecovery.ConnectionStarting();
        Check(ColdBattleRecovery.Pending,"reconnect allowed exit before a new discovery");
        Tick(); Check(NetAPI.sends.Count==nextPing+1,"same-room reconnect did not send a new fence");
        Check(FieldWire.calls.Count==1,"same-room reconnect discovered before its echo");
        Reply(0,Inactive()); Check(FieldWire.calls.Count==1,"old connection callback advanced discovery");
        EchoFence(nextPing); Tick();
        Check(FieldWire.calls.Count==2,"same-room reconnect did not restart discovery");
        Reply(0,Inactive()); Check(FieldWire.calls.Count==2,"old connection callback advanced discovery after reconnect");
        Reply(1,Inactive()); Reply(2,Inactive());
        Check(!ColdBattleRecovery.Pending,"new connection did not finish discovery");
    }
    static void JournalRemainsASeparateGuard()
    {
        Reset(); BattleRecovery.journal=true; Tick();
        Check(FieldWire.calls.Count==0 && ColdBattleRecovery.Pending,"pending local end was bypassed");
        BattleRecovery.journal=false; Tick(); Reply(0,Inactive()); Reply(1,Inactive());
        Check(!ColdBattleRecovery.Pending,"discovery did not resume after journal confirmation");
    }
    public static void Main()
    {
        ReadWithoutPositionReadiness(); ActiveFightStillBlocks(); FailedReadsKeepRetrying();
        ReconnectInvalidatesOldCallbacks(); JournalRemainsASeparateGuard();
        Console.WriteLine("ColdBattleRecovery replay passed: "+checks+" assertions");
    }
}
