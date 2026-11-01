using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.Reflection;
using LitJson;

namespace UnityEngine
{
    public struct Vector3
    {
        public float x,y,z;
        public Vector3(float x,float y,float z) { this.x=x; this.y=y; this.z=z; }
        public static Vector3 zero { get { return new Vector3(0,0,0); } }
        public float sqrMagnitude { get { return x*x+y*y+z*z; } }
        public static Vector3 operator -(Vector3 a,Vector3 b) { return new Vector3(a.x-b.x,a.y-b.y,a.z-b.z); }
        public static bool operator ==(Vector3 a,Vector3 b) { return a.x==b.x && a.y==b.y && a.z==b.z; }
        public static bool operator !=(Vector3 a,Vector3 b) { return !(a==b); }
        public override bool Equals(object value) { return value is Vector3 && this==(Vector3)value; }
        public override int GetHashCode() { return x.GetHashCode() ^ y.GetHashCode() ^ z.GetHashCode(); }
    }
    public static class Mathf { public static float Abs(float value) { return Math.Abs(value); } }
    public enum KeyCode { Escape }
    public static class Input
    {
        public static bool cancel;
        public static bool GetKeyDown(KeyCode key) { bool result=cancel; cancel=false; return result; }
    }
    public sealed class Transform { public Vector3 position; }
}
namespace ProtoBuf.Message
{
    public sealed class PointNew { public float x,y,z; }
    public sealed class SideBoss
    {
        public string uuid,bossPosId,bossId,heroId;
        public PointNew currentPoint;
        public int state,nature;
        public double curHp;
    }
    public sealed class CSMove { public PointNew pointNew; public long currentTime; }
}
namespace Dict.Blo
{
    public static class DictHeroBlo { public static object GetTableHero(string value) { return String.IsNullOrEmpty(value) ? null : new object(); } }
}
namespace Room
{
    using UnityEngine;
    using ProtoBuf.Message;
    public class RoomBase { public RoomCharactorManager charactorManager; }
    public static class RoomManager { public static RoomBase currentRoom; }
    public sealed class RCAIFieldBoss { public void SetHp(float value) { } }
    public sealed class FakeUI { public void CreateBoxCollider() { } }
    public sealed class RoomCharactor
    {
        public Transform transform=new Transform();
        public object ai;
        public FakeUI ui;
    }
    public sealed class RoomCharactorManager
    {
        readonly Dictionary<string,RoomCharactor> actors=new Dictionary<string,RoomCharactor>();
        public RoomCharactor self;
        public RoomCharactor GetSelfCharactor() { return self; }
        public RoomCharactor GetCharactor(string id) { RoomCharactor actor; return actors.TryGetValue(id,out actor) ? actor : null; }
        public void DestroyCharactor(string id) { actors.Remove(id); }
        public void CreateFieldBoss(SideBoss boss)
        {
            var actor=new RoomCharactor { ai=new RCAIFieldBoss(),ui=new FakeUI() };
            actor.transform.position=new Vector3(boss.currentPoint.x,boss.currentPoint.y,boss.currentPoint.z);
            actors.Add(boss.uuid,actor);
        }
    }
}
namespace UI { public static class UIFieldRunMap { public static int timerCalls; public static void RefreshBossTime(int nature,float seconds) { timerCalls++; } } }
public static class StaticData { public static string fieldId="field_003"; }
public static class ServerTimerTool { public static long CurrentUTCJavaTime() { return 123456; } }
public static class NetAPI
{
    public sealed class Sent { public int opcode; public object body; }
    public static readonly List<Sent> messages=new List<Sent>();
    public static bool fail;
    public static void SendSocket(int opcode,object body)
    {
        if (fail) { fail=false; throw new InvalidOperationException("socket failed"); }
        messages.Add(new Sent { opcode=opcode,body=body });
    }
}
namespace HYJS.DesertV4
{
    using Room;
    public sealed class FieldWireResult { public bool uncertain; public long code; public JsonData data; }
    public static class TravelController { public static bool Pending; }
    public static class LeaveController { public static bool Pending; }
    public sealed class FieldWire
    {
        public sealed class Session { public long playerId; public string sessionHex,origin; }
        public static Session live;
        public readonly List<Action<FieldWireResult>> requests=new List<Action<FieldWireResult>>();
        public static bool Current(out Session value) { value=live; return value!=null; }
        public void PositionStatus(Session owner,Action<FieldWireResult> callback) { requests.Add(callback); }
    }
    public static class PositionSync
    {
        public static int ConnectionEpoch=1;
        public static bool Locked;
        static long generation;
        public static bool TryLock(RoomCharactor actor,out long lease)
        {
            lease=0; if (Locked || actor==null) return false;
            Locked=true; lease=++generation; return true;
        }
        public static bool OwnsLock(long lease) { return Locked && lease>0 && lease==generation; }
        public static bool ReleaseIfOwned(long lease) { if (!OwnsLock(lease)) return false; Locked=false; return true; }
        public static void Release() { Locked=false; }
    }
}

public static class FieldBossPositionChecks
{
    static int checks;
    static void Check(bool value,string message)
    { checks++; if (!value) throw new Exception(message); }
    sealed class Fixture
    {
        public readonly Room.RoomCharactorManager manager=new Room.RoomCharactorManager();
        public readonly Room.RoomBase room=new Room.RoomBase();
        public readonly Room.RoomCharactor self=new Room.RoomCharactor();
        public readonly HYJS.DesertV4.FieldWire wire=new HYJS.DesertV4.FieldWire();
        public readonly HYJS.DesertV4.FieldWire.Session owner=new HYJS.DesertV4.FieldWire.Session { playerId=1,sessionHex="one",origin="test" };
        public readonly HYJS.DesertV4.FieldBoss boss;
        public int completed,starts,refreshes;
        public bool accepted,cancelled;
        public Fixture(float x)
        {
            HYJS.DesertV4.PositionSync.Release(); HYJS.DesertV4.PositionSync.ConnectionEpoch++;
            HYJS.DesertV4.FieldWire.live=owner; StaticData.fieldId="field_003"; NetAPI.messages.Clear(); NetAPI.fail=false;
            HYJS.DesertV4.TravelController.Pending=false; HYJS.DesertV4.LeaveController.Pending=false;
            UnityEngine.Input.cancel=false;
            self.transform.position=new UnityEngine.Vector3(x,0,0); manager.self=self;
            room.charactorManager=manager; Room.RoomManager.currentRoom=room;
            boss=new HYJS.DesertV4.FieldBoss(manager,delegate { refreshes++; });
            boss.ApplySnapshot(Snapshot("ggo-line-one","active",100));
        }
        public bool Begin()
        {
            return boss.ConfirmPosition(room,self,wire,owner,delegate(bool ok,bool cancel)
            {
                completed++; accepted=ok; cancelled=cancel;
                if (ok) { starts++; NetAPI.SendSocket(30083,"original boss start"); }
            });
        }
        public void Respond(int index,float x,string scene,bool loading)
        { wire.requests[index](Status(x,scene,loading)); }
    }
    static JsonData Snapshot(string id,string state,int hp)
    {
        return JsonMapper.ToObject("{\"serverTimeMs\":200000,\"boss\":{\"eventId\":\""+id+
            "\",\"pointId\":\"B1\",\"heroId\":\"hero_017\",\"bossId\":\"boss_003\",\"state\":\""+state+
            "\",\"position\":{\"x\":0,\"y\":0,\"z\":0},\"radius\":3,\"hp\":"+hp+
            ",\"maxHp\":100,\"startsAtMs\":100000,\"endsAtMs\":1000000,\"warningAtMs\":90000,\"level\":60,\"nature\":0}}");
    }
    static JsonData TimedSnapshot(string id,string state,int hp,long now,long start)
    {
        return JsonMapper.ToObject("{\"serverTimeMs\":"+now+",\"boss\":{\"eventId\":\""+id+
            "\",\"pointId\":\"B1\",\"heroId\":\"hero_017\",\"bossId\":\"boss_003\",\"state\":\""+state+
            "\",\"position\":{\"x\":0,\"y\":0,\"z\":0},\"radius\":3,\"hp\":"+hp+
            ",\"maxHp\":100,\"startsAtMs\":"+start+",\"endsAtMs\":"+(start+7200000)+",\"warningAtMs\":"+(start-60000)+",\"level\":60,\"nature\":0}}");
    }
    static void SetReceived(HYJS.DesertV4.FieldBoss boss,long elapsedSeconds)
    { typeof(HYJS.DesertV4.FieldBoss).GetField("receivedTicks",BindingFlags.Instance|BindingFlags.NonPublic).SetValue(boss,Stopwatch.GetTimestamp()-elapsedSeconds*Stopwatch.Frequency); }
    static string Marker(Room.RoomBase room,out string state)
    {
        string id,label; UnityEngine.Vector3 position;
        Check(HYJS.DesertV4.FieldBoss.MapMarkerLabel(room,out id,out position,out state,out label),"current room marker missing");
        return label;
    }
    static void CountdownAndScope()
    {
        var f=new Fixture(2f); string state;
        f.boss.ApplySnapshot(TimedSnapshot("one","waiting",0,200000,3800000));
        Check(Marker(f.room,out state)=="复活 1:00:00" && state=="waiting","death deadline must show immediately before warning window");
        SetReceived(f.boss,61);
        Check(Marker(f.room,out state)=="复活 58:59","countdown must follow monotonic elapsed time");
        f.boss.ApplySnapshot(TimedSnapshot("one","waiting",0,200000,3800000));
        Check(Marker(f.room,out state)=="复活 58:59","older server clock must not increase the same deadline");
        f.boss.ApplySnapshot(TimedSnapshot("two","waiting",0,200000,3800000));
        Check(Marker(f.room,out state)=="复活 1:00:00","new event must reset its clock");
        f.boss.ApplySnapshot(TimedSnapshot("two","waiting",100,200000,560000));
        Check(Marker(f.room,out state)=="刷新 06:00","new deadline and scheduled spawn must replace old timer");
        SetReceived(f.boss,360);
        Check(Marker(f.room,out state)=="待确认" && state=="syncing","deadline must wait for server confirmation");
        typeof(HYJS.DesertV4.FieldBoss).GetField("lastRefresh",BindingFlags.Instance|BindingFlags.NonPublic).SetValue(f.boss,0L);
        int before=f.refreshes; f.boss.Tick(); f.boss.Tick();
        Check(f.refreshes==before+1,"deadline refresh is bounded");
        f.boss.ApplySnapshot(TimedSnapshot("two","waiting",100,200000,560000));
        Check(Marker(f.room,out state)=="待确认","repeated old clock cannot revive an expired countdown");
        f.boss.ApplySnapshot(TimedSnapshot("three","active",100,200000,100000));
        Check(Marker(f.room,out state)=="首领出现" && state=="active","server-confirmed new event must replace waiting state");
        string id,label; UnityEngine.Vector3 position;
        Check(!HYJS.DesertV4.FieldBoss.MapMarkerLabel(new Room.RoomBase(),out id,out position,out state,out label),"other room must not inherit marker");
        StaticData.fieldId="field_002";
        Check(!HYJS.DesertV4.FieldBoss.MapMarkerLabel(f.room,out id,out position,out state,out label),"other map must not inherit marker");
        StaticData.fieldId="field_003"; f.boss.Clear();
        Check(!HYJS.DesertV4.FieldBoss.MapMarkerLabel(f.room,out id,out position,out state,out label),"scene exit must release marker");
        Check(UI.UIFieldRunMap.timerCalls==0,"desert countdown must not write the process-static native field timer");
        var reconnect=new Fixture(2f);
        reconnect.boss.ApplySnapshot(TimedSnapshot("one","waiting",0,300000,3800000));
        Check(Marker(reconnect.room,out state)=="复活 58:20","reconnect must use the fresh authoritative deadline");
        HYJS.DesertV4.PositionSync.ConnectionEpoch++;
        Check(Marker(reconnect.room,out state)=="待确认" && state=="syncing","a disconnected epoch must hide the old timer");
        int refreshes=reconnect.refreshes;
        reconnect.boss.ApplySnapshot(TimedSnapshot("one","waiting",0,300000,3800000));
        Check(reconnect.refreshes==refreshes+1 && Marker(reconnect.room,out state)=="待确认","old in-flight reply must trigger a fresh snapshot");
        reconnect.boss.ApplySnapshot(TimedSnapshot("one","waiting",0,400000,3800000));
        Check(Marker(reconnect.room,out state)=="复活 56:40","new connection snapshot restores the timer");
        string oldId,oldState; UnityEngine.Vector3 oldPosition;
        Check(HYJS.DesertV4.FieldBoss.MapMarker(reconnect.room,out oldId,out oldPosition,out oldState) && oldId=="one" && oldState=="waiting",
            "existing public marker ABI must remain usable");
        reconnect.boss.Clear();
    }
    static HYJS.DesertV4.FieldWireResult Status(float x,string scene,bool loading)
    {
        string text="{\"scene\":\""+scene+"\",\"loading\":"+(loading ? "true" : "false")+
            ",\"position\":{\"x\":"+x.ToString("R",CultureInfo.InvariantCulture)+",\"y\":0,\"z\":0}}";
        return new HYJS.DesertV4.FieldWireResult { code=0,data=JsonMapper.ToObject(text) };
    }
    static object Confirmation(HYJS.DesertV4.FieldBoss boss)
    { return typeof(HYJS.DesertV4.FieldBoss).GetField("confirmation",BindingFlags.Instance|BindingFlags.NonPublic).GetValue(boss); }
    static void SetLong(HYJS.DesertV4.FieldBoss boss,string name,long value)
    {
        object c=Confirmation(boss); if (c==null) throw new Exception("confirmation absent");
        c.GetType().GetField(name).SetValue(c,value);
    }
    static void Poll(Fixture f)
    { SetLong(f.boss,"nextPoll",0); f.boss.Tick(); }
    static void PositionBeforeStart()
    {
        var f=new Fixture(2.9f);
        Check(f.Begin(),"valid collision must begin confirmation");
        Check(HYJS.DesertV4.PositionSync.Locked && f.boss.Confirming,"collision position is locked");
        Check(NetAPI.messages.Count==1 && NetAPI.messages[0].opcode==30026,"native movement precedes boss start");
        var move=NetAPI.messages[0].body as ProtoBuf.Message.CSMove;
        Check(move!=null && move.pointNew.x==2.9f && move.pointNew.y==0 && move.pointNew.z==0 && move.currentTime==123456,"movement contains actual position and native clock");
        Check(f.wire.requests.Count==1 && f.starts==0,"no start while current position is unknown");
        f.Respond(0,3.8f,"field_003",false);
        Check(f.completed==0 && f.starts==0 && HYJS.DesertV4.PositionSync.Locked,"old server position cannot admit a start");
        Poll(f); Check(f.wire.requests.Count==2,"stale position causes a bounded retry");
        f.Respond(1,2.9f,"field_003",false);
        Check(f.completed==1 && f.accepted && f.starts==1 && !HYJS.DesertV4.PositionSync.Locked,"matching committed position admits once and releases");
        Check(NetAPI.messages.Count==2 && NetAPI.messages[1].opcode==30083,"start only follows the confirmed movement");
        f.Respond(0,2.9f,"field_003",false); f.Respond(1,2.9f,"field_003",false);
        Check(f.starts==1 && f.completed==1,"late duplicate status cannot start twice");
    }
    static void RadiusAndStatusGates()
    {
        var f=new Fixture(2.999f); Check(f.Begin(),"inside contact starts confirmation");
        f.Respond(0,3.001f,"field_003",false);
        Check(f.completed==0 && f.starts==0,"tolerance cannot admit a server position outside radius");
        Poll(f); f.Respond(1,2.999f,"other",false); Check(f.starts==0,"other scene rejected");
        Poll(f); f.Respond(2,2.999f,"field_003",true); Check(f.starts==0,"loading position rejected");
        Poll(f); f.Respond(3,2.999f,"field_003",false); Check(f.starts==1,"valid current position admitted");
        var far=new Fixture(3.1f); Check(!far.Begin() && NetAPI.messages.Count==0 && !HYJS.DesertV4.PositionSync.Locked,"outside contact does not send or lock");
    }
    static void InvalidationAndCancellation()
    {
        var f=new Fixture(2f); Check(f.Begin(),"session scenario starts");
        HYJS.DesertV4.FieldWire.live=new HYJS.DesertV4.FieldWire.Session { playerId=1,sessionHex="new",origin="test" };
        f.boss.Tick(); Check(f.completed==1 && !f.accepted && !HYJS.DesertV4.PositionSync.Locked,"session change cancels and unlocks");
        f.Respond(0,2f,"field_003",false); Check(f.starts==0 && f.completed==1,"old session reply ignored");

        f=new Fixture(2f); Check(f.Begin(),"connection scenario starts"); HYJS.DesertV4.PositionSync.ConnectionEpoch++;
        f.boss.Tick(); Check(f.completed==1 && !f.accepted && !HYJS.DesertV4.PositionSync.Locked,"connection change cancels");

        f=new Fixture(2f); Check(f.Begin(),"room scenario starts"); Room.RoomManager.currentRoom=new Room.RoomBase();
        f.boss.Tick(); Check(f.completed==1 && !f.accepted && !HYJS.DesertV4.PositionSync.Locked,"room change cancels");

        f=new Fixture(2f); Check(f.Begin(),"line scenario starts"); f.boss.ApplySnapshot(Snapshot("ggo-line-two","active",100));
        Check(f.completed==1 && f.starts==0 && !HYJS.DesertV4.PositionSync.Locked,"different line event cancels old identity");
        f.Respond(0,2f,"field_003",false); Check(f.starts==0,"old line response cannot start");

        f=new Fixture(2f); Check(f.Begin(),"escape scenario starts"); UnityEngine.Input.cancel=true; f.boss.Tick();
        Check(f.completed==1 && f.cancelled && !f.accepted && !HYJS.DesertV4.PositionSync.Locked,"cancel releases and reports cancellation");
        f.Respond(0,2f,"field_003",false); Check(f.starts==0,"cancelled callback cannot start");

        f=new Fixture(2f); Check(f.Begin(),"timeout scenario starts"); SetLong(f.boss,"deadline",Stopwatch.GetTimestamp()-1); f.boss.Tick();
        Check(f.completed==1 && !f.accepted && !HYJS.DesertV4.PositionSync.Locked,"timeout releases and fails");
        f.Respond(0,2f,"field_003",false); Check(f.starts==0,"timed-out callback cannot start");

        f=new Fixture(2f); Check(f.Begin(),"callback deadline scenario starts"); SetLong(f.boss,"deadline",Stopwatch.GetTimestamp()-1);
        f.Respond(0,2f,"field_003",false);
        Check(f.completed==1 && !f.accepted && f.starts==0 && !HYJS.DesertV4.PositionSync.Locked,"matching reply after deadline cannot start even before the next tick");

        f=new Fixture(2f); Check(f.Begin(),"teardown scenario starts"); f.boss.Clear();
        Check(!f.boss.Confirming && !HYJS.DesertV4.PositionSync.Locked && f.completed==0,"scene teardown releases without a late UI callback");
        f.Respond(0,2f,"field_003",false); Check(f.starts==0,"teardown callback ignored");
    }
    static void LeaseAndFailures()
    {
        var f=new Fixture(2f); Check(f.Begin(),"lease scenario starts");
        HYJS.DesertV4.PositionSync.Release(); long next;
        Check(HYJS.DesertV4.PositionSync.TryLock(f.self,out next),"other operation obtains a new lease");
        f.boss.Tick();
        Check(f.completed==1 && !f.accepted && HYJS.DesertV4.PositionSync.OwnsLock(next),"old callback preserves new owner's lock");
        f.Respond(0,2f,"field_003",false); Check(f.starts==0 && HYJS.DesertV4.PositionSync.OwnsLock(next),"old response cannot release new lease");
        HYJS.DesertV4.PositionSync.ReleaseIfOwned(next);

        f=new Fixture(2f); NetAPI.fail=true; Check(f.Begin(),"send exception is reported through callback");
        Check(f.completed==1 && !f.accepted && !HYJS.DesertV4.PositionSync.Locked && f.wire.requests.Count==0,"send failure releases and cannot start");

        f=new Fixture(2f); Check(f.Begin(),"actor-change scenario starts");
        f.self.transform.position=new UnityEngine.Vector3(2.5f,0,0); f.boss.Tick();
        Check(f.completed==1 && !f.accepted && !HYJS.DesertV4.PositionSync.Locked,"changed contact position invalidates confirmation");

        f=new Fixture(2f); HYJS.DesertV4.TravelController.Pending=true;
        Check(!f.Begin() && NetAPI.messages.Count==0 && !HYJS.DesertV4.PositionSync.Locked,"pending travel cannot acquire a boss lock");
        f=new Fixture(2f); Check(f.Begin(),"travel-race scenario starts"); HYJS.DesertV4.TravelController.Pending=true;
        f.Respond(0,2f,"field_003",false);
        Check(f.completed==1 && f.starts==0 && !HYJS.DesertV4.PositionSync.Locked,"new pending travel cancels before a matching reply can start");
        f=new Fixture(2f); Check(f.Begin(),"leave-race scenario starts"); HYJS.DesertV4.LeaveController.Pending=true;
        f.boss.Tick(); Check(f.completed==1 && f.starts==0 && !HYJS.DesertV4.PositionSync.Locked,"pending leave cancels confirmation");
    }
    public static void Main()
    {
        System.Threading.Thread.CurrentThread.CurrentCulture=CultureInfo.InvariantCulture;
        PositionBeforeStart(); RadiusAndStatusGates(); InvalidationAndCancellation(); LeaseAndFailures(); CountdownAndScope();
        Console.WriteLine("PASS FieldBoss position and countdown: "+checks+" assertions");
    }
}
