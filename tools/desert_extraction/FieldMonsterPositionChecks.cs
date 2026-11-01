using System;
using System.Diagnostics;
using System.Globalization;
using System.Reflection;
using LitJson;
using HYJS.DesertV4;

namespace HYJS.DesertV4
{
    internal sealed class FieldMonsters
    {
        internal bool available=true;
        internal bool HasAvailable(string uuid) { return available && uuid=="monster"; }
    }
}
static class FieldMonsterPositionChecks
{
    static int count;
    static void Check(bool yes,string name) { count++; if (!yes) throw new Exception(name); }
    sealed class Fixture
    {
        internal Room.RoomCharactorManager manager=new Room.RoomCharactorManager();
        internal Room.RoomBase room=new Room.RoomBase();
        internal Room.RoomCharactor actor=new Room.RoomCharactor();
        internal FieldWire wire=new FieldWire();
        internal FieldWire.Session owner=new FieldWire.Session { playerId=1,sessionHex="one",origin="test" };
        internal FieldMonsters monsters=new FieldMonsters();
        internal FieldMonsterPosition position;
        internal int done,starts;
        internal bool accepted,cancelled;
        internal Fixture()
        {
            PositionSync.Release(); PositionSync.ConnectionEpoch++; FieldWire.live=owner;
            StaticData.fieldId="field_003"; NetAPI.messages.Clear(); NetAPI.fail=false;
            TravelController.Pending=false; LeaveController.Pending=false; UnityEngine.Input.cancel=false;
            actor.transform.position=new UnityEngine.Vector3(2,0,0); manager.self=actor;
            room.charactorManager=manager; Room.RoomManager.currentRoom=room;
            position=new FieldMonsterPosition(manager,monsters);
        }
        internal bool Begin()
        {
            return position.Begin(room,actor,wire,owner,"monster",delegate(bool ok,bool cancel)
            { done++; accepted=ok; cancelled=cancel; if (ok) { starts++; NetAPI.SendSocket(30047,"native start"); } });
        }
        internal void Reply(int index,float x,string scene,bool loading)
        {
            string json="{\"scene\":\""+scene+"\",\"loading\":"+(loading ? "true" : "false")+
                ",\"position\":{\"x\":"+x.ToString("R",CultureInfo.InvariantCulture)+",\"y\":0,\"z\":0}}";
            wire.requests[index](new FieldWireResult { code=0,data=JsonMapper.ToObject(json) });
        }
        internal void SetLong(string name,long value)
        {
            object a=typeof(FieldMonsterPosition).GetField("active",BindingFlags.Instance|BindingFlags.NonPublic).GetValue(position);
            a.GetType().GetField(name,BindingFlags.Instance|BindingFlags.NonPublic|BindingFlags.Public).SetValue(a,value);
        }
        internal void Poll() { SetLong("nextPoll",0); position.Tick(); }
    }
    public static void Main()
    {
        var f=new Fixture();
        Check(f.Begin(),"valid collision starts");
        Check(PositionSync.Locked && NetAPI.messages.Count==1 && NetAPI.messages[0].opcode==30026,"real movement sent first");
        Check(f.wire.requests.Count==1 && f.starts==0,"no battle before commit");
        f.Reply(0,1,"field_003",false); Check(f.done==0,"stale position rejected");
        f.Poll(); f.Reply(1,2,"wrong",false); Check(f.done==0,"wrong scene rejected");
        f.Poll(); f.Reply(2,2,"field_003",true); Check(f.done==0,"loading rejected");
        f.Poll(); f.Reply(3,2,"field_003",false);
        Check(f.accepted && f.starts==1 && f.done==1 && !PositionSync.Locked,"matching committed position starts once");
        f.Reply(3,2,"field_003",false); Check(f.starts==1,"duplicate ignored");
        f=new Fixture(); Check(f.Begin(),"session case"); FieldWire.live=new FieldWire.Session(); f.position.Tick();
        Check(f.done==1 && !f.accepted && !PositionSync.Locked,"session change releases"); f.Reply(0,2,"field_003",false); Check(f.starts==0,"old reply ignored");
        f=new Fixture(); Check(f.Begin(),"target case"); f.monsters.available=false; f.position.Tick();
        Check(f.done==1 && !f.accepted && !PositionSync.Locked,"vanished monster releases");
        f=new Fixture(); Check(f.Begin(),"deadline case"); f.SetLong("deadline",Stopwatch.GetTimestamp()-1); f.Reply(0,2,"field_003",false);
        Check(f.done==1 && !f.accepted && !PositionSync.Locked,"late reply rejected");
        f=new Fixture(); Check(f.Begin(),"cancel case"); UnityEngine.Input.cancel=true; f.position.Tick();
        Check(f.cancelled && !PositionSync.Locked,"cancel releases");
        f=new Fixture(); Check(f.Begin(),"teardown case"); f.position.Clear(); f.Reply(0,2,"field_003",false);
        Check(f.done==0 && f.starts==0 && !PositionSync.Locked,"teardown ignores reply");
        f=new Fixture(); Check(f.Begin(),"lease case"); PositionSync.Release(); long lease; Check(PositionSync.TryLock(f.actor,out lease),"new lease");
        f.position.Tick(); Check(!f.accepted && PositionSync.OwnsLock(lease),"old attempt cannot release new lease"); PositionSync.ReleaseIfOwned(lease);
        f=new Fixture(); NetAPI.fail=true; Check(!f.Begin() && f.done==1 && !PositionSync.Locked && f.starts==0,"failed send releases");
        Console.WriteLine("PASS monster contact position: "+count+" assertions");
    }
}
