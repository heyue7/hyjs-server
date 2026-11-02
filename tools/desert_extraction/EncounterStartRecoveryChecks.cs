// Execute the real encounter and cold-recovery components with deterministic transports.
using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using HYJS.DesertV4;
using LitJson;
using NetWork.Packets;
using ProtoBuf.Message;
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
public static class PBConvert { public static object toObject(int opcode,byte[] value) { return null; } }
public static class BoxManager
{
    public static void CreatePopupTextBox(string message,float delay) { }
    public static void CreateMessageBoxWithoutCancelButton(string text,bool show,Action<bool> done,string label,bool extra) { }
}
public static class NetAPI
{
    public sealed class Send { public int opcode; public object body; }
    public static bool connected;
    public static int reconnects;
    public static readonly List<Send> sends=new List<Send>();
    public static bool SocketIsConnected() { return connected; }
    public static void SendSocket(int opcode,object body) { sends.Add(new Send { opcode=opcode,body=body }); }
    public static void Reconnect() { reconnects++; }
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
        public void FightActive(Session owner,int type,Action<FieldWireResult> callback)
        { calls.Add(new Call { type=type,callback=callback }); }
        public void ResumeFight(Session owner,int type,string fight,string hash,Action<FieldWireResult> callback)
        { throw new Exception("Unexpected resume in discovery test"); }
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
        public static int SceneVersion,clears;
        public static void ClearEncounterWait() { clears++; }
        public static FieldRuntime Require() { return new FieldRuntime(); }
        public void RequestSnapshot() { }
        public static bool ResumeEncounter(int type,object native) { return false; }
    }
}

public static class EncounterStartRecoveryChecks
{
    const BindingFlags HiddenStatic=BindingFlags.Static|BindingFlags.NonPublic;
    const BindingFlags HiddenInstance=BindingFlags.Instance|BindingFlags.NonPublic;
    static readonly FieldInfo Next=typeof(EncounterStartRecovery).GetField("nextBarrier",HiddenStatic);
    static readonly FieldInfo Barrier=typeof(EncounterStartRecovery).GetField("barrier",HiddenStatic);
    static readonly FieldInfo Queued=typeof(EncounterStartRecovery).GetField("queued",HiddenStatic);
    static readonly FieldInfo ColdInstance=typeof(ColdBattleRecovery).GetField("instance",HiddenStatic);
    static readonly FieldInfo ColdGeneration=typeof(ColdBattleRecovery).GetField("generation",HiddenInstance);
    static readonly MethodInfo ColdUpdate=typeof(ColdBattleRecovery).GetMethod("Update",HiddenInstance);
    static int checks;

    static void Check(bool value,string message)
    { checks++; if (!value) throw new Exception(message); }
    static void Run(string name,Action test)
    {
        int before=checks; test();
        Console.WriteLine("CASE\t"+name+"\t"+(checks-before));
    }
    static void Reset()
    {
        Check(Next!=null && Barrier!=null && Queued!=null && ColdInstance!=null && ColdGeneration!=null && ColdUpdate!=null,
              "Required real component fields are unavailable");
        ColdInstance.SetValue(null,null);
        PositionSync.ConnectionEpoch++;
        EncounterStartRecovery.ConnectionStarting();
        ((IDictionary)Queued.GetValue(null)).Clear();
        FieldWire.current=new FieldWire.Session { playerId=42,sessionHex="session-42",origin="origin-1" };
        RoomManager.currentRoom=new RoomField();
        StaticData.fieldId="field_003"; StaticData.pveBossUUID=null; StaticData.curBattleData=null;
        FieldRuntime.SceneVersion++; FieldRuntime.clears=0; FieldRuntime.InteractionReady=false;
        BattleRecovery.journal=BattleRecovery.live=false;
        UnityEngine.Time.realtimeSinceStartup=0;
        FieldWire.calls.Clear(); NetAPI.sends.Clear(); NetAPI.connected=true; NetAPI.reconnects=0;
    }
    static void Sent(int type)
    { EncounterStartRecovery.Sent(type,RoomManager.currentRoom,FieldRuntime.SceneVersion,FieldWire.current); }
    static void ColdTick()
    {
        object instance=ColdInstance.GetValue(null);
        Check(instance!=null,"Cold recovery instance missing");
        ColdUpdate.Invoke(instance,null);
    }
    static FieldWireResult Inactive()
    {
        JsonData data=new JsonData(); data["active"]=new JsonData(false);
        return new FieldWireResult { data=data };
    }
    static long PingAt(int index)
    {
        Check(index<NetAPI.sends.Count,"Ping was not sent");
        Check(NetAPI.sends[index].opcode==30020,"Unexpected socket opcode");
        CSPingTcp ping=NetAPI.sends[index].body as CSPingTcp;
        Check(ping!=null,"Socket body is not CSPingTcp");
        return ping.clientTime;
    }
    static Packet Ping(long value)
    { return new Packet { kBody=new SCPingTcp { clientTime=value } }; }
    static void Echo(long value)
    {
        Packet packet=Ping(value);
        EncounterStartRecovery.TrackPacket(packet);
        Check(!EncounterStartRecovery.DropPacket(packet),"Ping echo was dropped");
    }
    static void AdvanceBarrier()
    { Next.SetValue(null,0L); EncounterStartRecovery.Tick(); }
    static void ColdReadyAndInactive()
    {
        ColdBattleRecovery.SceneReady(RoomManager.currentRoom,FieldRuntime.SceneVersion);
        Echo(PingAt(0));
        ColdTick();
        Check(FieldWire.calls.Count==1 && FieldWire.calls[0].type==6,"Initial monster discovery missing");
        FieldWire.calls[0].callback(Inactive());
        Check(FieldWire.calls.Count==2 && FieldWire.calls[1].type==12,"Initial boss discovery missing");
        FieldWire.calls[1].callback(Inactive());
        Check(ColdBattleRecovery.AllowNewEncounter(),"Inactive baseline did not permit encounter");
        FieldWire.calls.Clear(); NetAPI.sends.Clear(); FieldRuntime.clears=0;
    }
    static object StartBody(int type)
    { return type==6 ? (object)new SCSidePveFightStart() : new SCSidePveBossFightStart(); }
    static object TrackedStart(int type)
    {
        object body=StartBody(type); Packet packet=new Packet { kBody=body };
        EncounterStartRecovery.TrackPacket(packet);
        Check(!EncounterStartRecovery.DropPacket(packet),"Current tracked packet dropped before native callback");
        return body;
    }
    static void TimeoutSendsOnlyPing()
    {
        Reset(); ColdReadyAndInactive(); Sent(6);
        Check(EncounterStartRecovery.Pending && NetAPI.sends.Count==0,"Sent did not establish a quiet pending request");
        EncounterStartRecovery.Timeout();
        Check(EncounterStartRecovery.WaitingForFence,"Timeout did not wait for fence");
        Check(NetAPI.sends.Count==1,"Timeout did not send exactly one fence");
        Check(PingAt(0)==(long)Barrier.GetValue(null),"Fence token differs from sent ping");
        EncounterStartRecovery.Tick();
        Check(NetAPI.sends.Count==1 && NetAPI.reconnects==0,"Immediate tick resent a request");
    }
    static void LateStartAndWrongEcho()
    {
        Reset(); ColdReadyAndInactive(); Sent(6); EncounterStartRecovery.Timeout();
        object body=TrackedStart(6);
        Check(EncounterStartRecovery.DropStart(6,RoomManager.currentRoom.netManager,body),"Late native start was delivered");
        long token=PingAt(0); Echo(token+1);
        Check(EncounterStartRecovery.Pending && EncounterStartRecovery.WaitingForFence,"Wrong ping released pending request");
        ColdTick(); Check(FieldWire.calls.Count==0,"Wrong ping allowed cold discovery");
        Packet untracked=Ping(token);
        Check(!EncounterStartRecovery.DropPacket(untracked),"Untracked ping should pass through");
        Check(EncounterStartRecovery.Pending,"Untracked matching echo released pending request");
    }
    static void MatchingEchoAllowsDiscovery()
    {
        Reset(); ColdReadyAndInactive(); Sent(6); EncounterStartRecovery.Timeout();
        int before=(int)ColdGeneration.GetValue(ColdInstance.GetValue(null));
        ColdTick(); Check(FieldWire.calls.Count==0,"Discovery started before matching echo");
        Echo(PingAt(0));
        Check(!EncounterStartRecovery.Pending && !EncounterStartRecovery.WaitingForFence,"Matching echo did not release fence");
        Check(FieldRuntime.clears==2,"Wait state was not cleared on timeout and echo");
        Check((int)ColdGeneration.GetValue(ColdInstance.GetValue(null))==before+1,"Matching echo did not refresh real cold recovery");
        ColdTick(); Check(FieldWire.calls.Count==1 && FieldWire.calls[0].type==6,"Monster query missing after echo");
        FieldWire.calls[0].callback(Inactive());
        Check(FieldWire.calls.Count==2 && FieldWire.calls[1].type==12,"Boss query missing after monster no-active");
        FieldWire.calls[1].callback(Inactive());
        Check(ColdBattleRecovery.AllowNewEncounter(),"No-active pair did not release cold recovery");
    }
    static void NormalBindingsAndDuplicate()
    {
        int[] types=new int[] { 6,12 };
        foreach (int type in types)
        {
            Reset(); Sent(type); object body=TrackedStart(type); object net=RoomManager.currentRoom.netManager;
            Check(!EncounterStartRecovery.DropStart(type,net,body),"Matched native start was rejected");
            Packet duplicate=new Packet { kBody=body }; EncounterStartRecovery.TrackPacket(duplicate);
            Check(!EncounterStartRecovery.DropPacket(duplicate),"Duplicate packet was not dispatched to guard");
            Check(EncounterStartRecovery.DropStart(type,net,body),"Duplicate native start was accepted");
        }
        Reset(); Sent(6); object wrongType=TrackedStart(12);
        Check(EncounterStartRecovery.DropStart(12,RoomManager.currentRoom.netManager,wrongType),"Wrong type accepted");
        Reset(); Sent(6); object wrongNet=TrackedStart(6);
        Check(EncounterStartRecovery.DropStart(6,new object(),wrongNet),"Wrong native network accepted");
        Reset(); Sent(6); object wrongRoom=TrackedStart(6); object oldNet=RoomManager.currentRoom.netManager;
        RoomManager.currentRoom=new RoomField();
        Check(EncounterStartRecovery.DropStart(6,oldNet,wrongRoom),"Wrong room accepted");
        Reset(); Sent(6); object wrongScene=TrackedStart(6); FieldRuntime.SceneVersion++;
        Check(EncounterStartRecovery.DropStart(6,RoomManager.currentRoom.netManager,wrongScene),"Wrong scene version accepted");
        for (int changed=0;changed<3;changed++)
        {
            Reset(); Sent(6); object wrongSession=TrackedStart(6);
            FieldWire.current=new FieldWire.Session { playerId=changed==0 ? 43 : 42,
                sessionHex=changed==1 ? "other-session" : "session-42",origin=changed==2 ? "other-origin" : "origin-1" };
            Check(EncounterStartRecovery.DropStart(6,RoomManager.currentRoom.netManager,wrongSession),"Changed session identity accepted");
        }
        Reset(); Sent(6); object equalSession=TrackedStart(6);
        FieldWire.current=new FieldWire.Session { playerId=42,sessionHex="session-42",origin="origin-1" };
        Check(!EncounterStartRecovery.DropStart(6,RoomManager.currentRoom.netManager,equalSession),"Equivalent session values rejected");
    }
    static void OldEpochIsDropped()
    {
        Reset(); Sent(6); object body=StartBody(6); Packet queued=new Packet { kBody=body };
        EncounterStartRecovery.TrackPacket(queued); PositionSync.ConnectionEpoch++;
        EncounterStartRecovery.ConnectionStarting();
        Check(EncounterStartRecovery.DropPacket(queued),"Old epoch start packet escaped guard");
        Check(EncounterStartRecovery.DropStart(6,RoomManager.currentRoom.netManager,body),"Old epoch body was delivered");
    }
    static void ExitedSceneIsDropped()
    {
        Reset(); Sent(12); object body=TrackedStart(12); object net=RoomManager.currentRoom.netManager;
        StaticData.fieldId="field_001"; RoomManager.currentRoom=new RoomBase(); FieldRuntime.SceneVersion++;
        Check(EncounterStartRecovery.DropStart(12,net,body),"Start was delivered after scene exit");
    }
    static void ResumeAdmitsOneObjectOnce()
    {
        Reset(); object approved=StartBody(6); object other=StartBody(6);
        EncounterStartRecovery.AdmitResume(approved);
        Check(EncounterStartRecovery.DropStart(6,RoomManager.currentRoom.netManager,other),"Different resume object was accepted");
        Check(!EncounterStartRecovery.DropStart(6,RoomManager.currentRoom.netManager,approved),"Admitted resume object was rejected");
        Check(EncounterStartRecovery.DropStart(6,RoomManager.currentRoom.netManager,approved),"Resume object was accepted twice");
    }
    static void ThreePingsThenBoundedReconnect()
    {
        Reset(); Sent(12); EncounterStartRecovery.Timeout(); long token=PingAt(0);
        AdvanceBarrier(); AdvanceBarrier();
        Check(NetAPI.sends.Count==3 && PingAt(1)==token && PingAt(2)==token,"Fence retries changed token or count");
        Check(NetAPI.reconnects==0,"Reconnect happened before three fence sends");
        AdvanceBarrier();
        Check(NetAPI.reconnects==1 && NetAPI.sends.Count==3,"Fence retry limit did not reconnect exactly once");
        for (int i=0;i<10;i++) EncounterStartRecovery.Tick();
        Check(NetAPI.reconnects==1 && NetAPI.sends.Count==3,"Reconnect or socket requests repeated after limit");
    }
    static void ConnectionFenceBlocksColdDiscovery()
    {
        Reset(); ColdBattleRecovery.SceneReady(RoomManager.currentRoom,FieldRuntime.SceneVersion);
        Check(EncounterStartRecovery.WaitingForFence,"Scene connection did not start a fence");
        long oldToken=PingAt(0); Packet oldEcho=Ping(oldToken); EncounterStartRecovery.TrackPacket(oldEcho);
        ColdTick(); Check(FieldWire.calls.Count==0,"Initial connection discovered before echo");
        PositionSync.ConnectionEpoch++; EncounterStartRecovery.ConnectionStarting();
        ColdTick();
        Check(NetAPI.sends.Count==2 && EncounterStartRecovery.WaitingForFence,"New connection did not fence");
        Check(FieldWire.calls.Count==0,"New connection discovered before echo");
        Check(!EncounterStartRecovery.DropPacket(oldEcho),"Old ping should pass through");
        Check(EncounterStartRecovery.WaitingForFence,"Old connection echo released new fence");
        long newToken=PingAt(1); Echo(newToken+1);
        ColdTick(); Check(FieldWire.calls.Count==0 && EncounterStartRecovery.WaitingForFence,"Wrong new echo allowed discovery");
        Echo(newToken); ColdTick();
        Check(FieldWire.calls.Count==1 && FieldWire.calls[0].type==6,"Matching new echo did not permit discovery");
        FieldWire.calls[0].callback(Inactive()); FieldWire.calls[1].callback(Inactive());
        Check(ColdBattleRecovery.AllowNewEncounter(),"New connection no-active pair did not release gate");
    }
    public static void Main()
    {
        Run("timeout_sends_only_ping",TimeoutSendsOnlyPing);
        Run("late_start_and_wrong_echo",LateStartAndWrongEcho);
        Run("matching_echo_allows_discovery",MatchingEchoAllowsDiscovery);
        Run("normal_bindings_and_duplicate",NormalBindingsAndDuplicate);
        Run("old_epoch_is_dropped",OldEpochIsDropped);
        Run("exited_scene_is_dropped",ExitedSceneIsDropped);
        Run("resume_admits_one_object_once",ResumeAdmitsOneObjectOnce);
        Run("three_pings_then_bounded_reconnect",ThreePingsThenBoundedReconnect);
        Run("connection_fence_blocks_cold_discovery",ConnectionFenceBlocksColdDiscovery);
        Console.WriteLine("PASS\t"+checks);
    }
}
