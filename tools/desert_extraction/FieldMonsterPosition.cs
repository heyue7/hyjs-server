using System;
using System.Diagnostics;
using System.Globalization;
using LitJson;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Synchronize the actual collision position before issuing native type-6
    // start. Socket writes are asynchronous, so a committed HTTP observation
    // is required before the second native message is sent.
    internal sealed class FieldMonsterPosition
    {
        sealed class Attempt
        {
            internal RoomBase room;
            internal RoomCharactor actor;
            internal FieldWire wire;
            internal FieldWire.Session owner;
            internal string uuid;
            internal Vector3 position;
            internal int epoch;
            internal long lease,deadline,nextPoll;
            internal bool inFlight;
            internal Action<bool,bool> completed;
        }
        readonly RoomCharactorManager manager;
        readonly FieldMonsters monsters;
        Attempt active;
        internal FieldMonsterPosition(RoomCharactorManager manager,FieldMonsters monsters)
        { this.manager=manager; this.monsters=monsters; }
        internal bool Confirming { get { return active!=null; } }
        static bool Same(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.sessionHex==b.sessionHex && a.origin==b.origin; }
        static bool Valid(Vector3 p)
        {
            return !Single.IsNaN(p.x) && !Single.IsNaN(p.y) && !Single.IsNaN(p.z) &&
                !Single.IsInfinity(p.x) && !Single.IsInfinity(p.y) && !Single.IsInfinity(p.z) &&
                Math.Abs(p.x)<=10000 && Math.Abs(p.y)<=10000 && Math.Abs(p.z)<=10000;
        }
        static Vector3 Vector(JsonData d)
        {
            return new Vector3(Single.Parse(d["x"].ToString(),CultureInfo.InvariantCulture),
                Single.Parse(d["y"].ToString(),CultureInfo.InvariantCulture),Single.Parse(d["z"].ToString(),CultureInfo.InvariantCulture));
        }
        bool Current(Attempt a)
        {
            try
            {
                FieldWire.Session live;
                return active==a && Stopwatch.GetTimestamp()<a.deadline && a.room!=null && a.room==RoomManager.currentRoom &&
                    StaticData.fieldId=="field_003" && a.room.charactorManager==manager && manager.GetSelfCharactor()==a.actor &&
                    PositionSync.ConnectionEpoch==a.epoch && PositionSync.OwnsLock(a.lease) &&
                    FieldWire.Current(out live) && Same(a.owner,live) && !TravelController.Pending && !LeaveController.Pending &&
                    monsters.HasAvailable(a.uuid) && a.actor!=null && (a.actor.transform.position-a.position).sqrMagnitude<=.0001f;
            }
            catch (Exception) { return false; }
        }
        void Finish(Attempt a,bool confirmed,bool cancelled,bool notify)
        {
            if (active!=a) return;
            active=null;
            if (!PositionSync.ReleaseIfOwned(a.lease)) confirmed=false;
            if (notify && a.completed!=null) a.completed(confirmed,cancelled);
        }
        internal bool Begin(RoomBase room,RoomCharactor actor,FieldWire wire,FieldWire.Session owner,string uuid,Action<bool,bool> completed)
        {
            long lease=0; FieldWire.Session live;
            try
            {
                if (active!=null || room==null || room!=RoomManager.currentRoom || StaticData.fieldId!="field_003" ||
                    room.charactorManager!=manager || actor==null || manager.GetSelfCharactor()!=actor || wire==null || completed==null ||
                    TravelController.Pending || LeaveController.Pending || !FieldWire.Current(out live) || !Same(owner,live) || !monsters.HasAvailable(uuid)) return false;
                if (!PositionSync.TryLock(actor,out lease)) return false;
                Vector3 position=actor.transform.position;
                if (!Valid(position) || !monsters.HasAvailable(uuid)) { PositionSync.ReleaseIfOwned(lease); return false; }
                Attempt a=new Attempt { room=room,actor=actor,wire=wire,owner=owner,uuid=uuid,completed=completed,position=position,
                    epoch=PositionSync.ConnectionEpoch,lease=lease,deadline=Stopwatch.GetTimestamp()+5L*Stopwatch.Frequency };
                active=a;
                NetAPI.SendSocket(30026,new CSMove { pointNew=new PointNew { x=position.x,y=position.y,z=position.z },currentTime=ServerTimerTool.CurrentUTCJavaTime() });
                Poll(a);
                return true;
            }
            catch (Exception)
            {
                if (active!=null && active.lease==lease) Finish(active,false,false,true);
                else PositionSync.ReleaseIfOwned(lease);
                return false;
            }
        }
        void Poll(Attempt a)
        {
            if (!Current(a) || a.inFlight) return;
            a.inFlight=true;
            try
            {
                a.wire.PositionStatus(a.owner,delegate(FieldWireResult result)
                {
                    if (active!=a) return;
                    a.inFlight=false;
                    if (!Current(a)) { Finish(a,false,false,true); return; }
                    try
                    {
                        JsonData d=result==null ? null : result.data;
                        if (result!=null && !result.uncertain && result.code==0 && d!=null && (string)d["scene"]=="field_003" &&
                            d["loading"].IsBoolean && !(bool)d["loading"])
                        {
                            Vector3 observed=Vector(d["position"]);
                            if (Valid(observed) && (observed-a.position).sqrMagnitude<=.0001f)
                            { Finish(a,true,false,true); return; }
                        }
                    }
                    catch (Exception) { }
                    a.nextPoll=Stopwatch.GetTimestamp()+Math.Max(1L,Stopwatch.Frequency/4);
                });
            }
            catch (Exception) { a.inFlight=false; a.nextPoll=Stopwatch.GetTimestamp()+Math.Max(1L,Stopwatch.Frequency/4); }
        }
        internal void Tick()
        {
            Attempt a=active;
            if (a==null) return;
            if (!Current(a)) { Finish(a,false,false,true); return; }
            if (Input.GetKeyDown(KeyCode.Escape)) { Finish(a,false,true,true); return; }
            if (!a.inFlight && Stopwatch.GetTimestamp()>=a.nextPoll) Poll(a);
        }
        internal void Clear() { if (active!=null) Finish(active,false,false,false); }
    }
}
