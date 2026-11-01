using System;
using System.Diagnostics;
using System.Globalization;
using Dict.Blo;
using LitJson;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Single public boss projection from the server event, using the native
    // field boss actor and its existing collision/animation components.
    public sealed class FieldBoss
    {
        sealed class Event
        {
            public string id,point,hero,boss,state;
            public Vector3 position;
            public float radius;
            public long hp,maxHp,start,end,warning,serverTime,nature;
        }
        sealed class PositionConfirmation
        {
            public RoomBase room;
            public RoomCharactor actor;
            public FieldWire wire;
            public FieldWire.Session owner;
            public Action<bool,bool> completed;
            public string eventId;
            public Vector3 position;
            public int epoch;
            public long deadline,nextPoll,lease;
            public bool inFlight;
        }
        readonly RoomCharactorManager manager;
        readonly Action refresh;
        static FieldBoss mapOwner;
        Event current;
        PositionConfirmation confirmation;
        int snapshotEpoch,requestedEpoch;
        long receivedTicks,lastRefresh;
        public FieldBoss(RoomCharactorManager source,Action requestRefresh)
        {
            if (source==null || requestRefresh==null) throw new ArgumentNullException("boss dependencies");
            manager=source; refresh=requestRefresh;
        }
        static long Integer(JsonData d) { return Int64.Parse(d.ToString(),CultureInfo.InvariantCulture); }
        static float Number(JsonData d)
        {
            float n=Single.Parse(d.ToString(),CultureInfo.InvariantCulture);
            if (Single.IsNaN(n)||Single.IsInfinity(n)||Mathf.Abs(n)>100000) throw new InvalidOperationException("Invalid boss coordinate");
            return n;
        }
        static Vector3 Vector(JsonData d) { return new Vector3(Number(d["x"]),Number(d["y"]),Number(d["z"])); }
        static bool SameVisual(Event a,Event b)
        { return a.id==b.id && a.hero==b.hero && a.boss==b.boss && a.point==b.point && a.position==b.position; }
        void Remove(Event e)
        { if (e!=null && !String.IsNullOrEmpty(e.id) && manager.GetCharactor(e.id)!=null) manager.DestroyCharactor(e.id); }
        public void Clear()
        {
            if (System.Object.ReferenceEquals(mapOwner,this)) mapOwner=null;
            if (confirmation!=null) FinishConfirmation(confirmation,false,false,false);
            try { Remove(current); }
            finally { current=null; }
        }
        long ServerNow(long tick)
        {
            if (tick<=receivedTicks) return current.serverTime;
            double elapsed=(tick-receivedTicks)*1000.0/Stopwatch.Frequency;
            if (elapsed>=Int64.MaxValue-current.serverTime) return Int64.MaxValue;
            return current.serverTime+(long)elapsed;
        }
        static string WaitingLabel(long remainingMs,bool killed)
        {
            // Round upward so a positive fraction never appears as 00:00.
            long seconds=remainingMs/1000+(remainingMs%1000==0 ? 0 : 1);
            long hours=seconds/3600, minutes=seconds/60%60, rest=seconds%60;
            string time=hours>0 ? String.Format(CultureInfo.InvariantCulture,"{0}:{1:00}:{2:00}",hours,minutes,rest) :
                String.Format(CultureInfo.InvariantCulture,"{0:00}:{1:00}",minutes,rest);
            return (killed ? "复活 " : "刷新 ")+time;
        }
        // The map reads only a validated event belonging to the current room.
        // Crossing a scheduled boundary needs a fresh server state; the local
        // clock alone must never claim that a boss has appeared or survived.
        public static bool MapMarker(RoomBase room,out string id,out Vector3 position,out string state)
        {
            string label;
            return MapMarkerLabel(room,out id,out position,out state,out label);
        }
        internal static bool MapMarkerLabel(RoomBase room,out string id,out Vector3 position,out string state,out string label)
        {
            id=null; position=Vector3.zero; state=null; label=null;
            FieldBoss owner=mapOwner;
            if (owner==null || owner.current==null || room==null || room!=RoomManager.currentRoom ||
                room.charactorManager!=owner.manager || StaticData.fieldId!="field_003") return false;
            Event e=owner.current;
            long now=owner.ServerNow(Stopwatch.GetTimestamp());
            id=e.id; position=e.position; state=e.state;
            if (owner.snapshotEpoch!=PositionSync.ConnectionEpoch || (state=="waiting" && now>=e.start) ||
                (state=="active" && now>=e.end)) state="syncing";
            if (state=="waiting") label=WaitingLabel(e.start-now,e.hp==0);
            else if (state=="active") label="首领出现";
            else if (state=="defeated") label="已击败";
            else if (state=="expired") label="已消失";
            else label="待确认";
            return true;
        }
        public bool CanInteract(string id,RoomCharactor actor)
        {
            try
            {
                return current!=null && snapshotEpoch==PositionSync.ConnectionEpoch && current.state=="active" && current.hp>0 && current.id==id && actor!=null &&
                    (actor.transform.position-current.position).sqrMagnitude<=current.radius*current.radius;
            }
            catch (Exception) { return false; }
        }
        public bool Confirming { get { return confirmation!=null; } }
        static bool SameSession(FieldWire.Session a,FieldWire.Session b)
        { return a!=null && b!=null && a.playerId==b.playerId && a.sessionHex==b.sessionHex && a.origin==b.origin; }
        static bool ValidPosition(Vector3 p)
        {
            return !Single.IsNaN(p.x) && !Single.IsNaN(p.y) && !Single.IsNaN(p.z) &&
                !Single.IsInfinity(p.x) && !Single.IsInfinity(p.y) && !Single.IsInfinity(p.z) &&
                Mathf.Abs(p.x)<=10000 && Mathf.Abs(p.y)<=10000 && Mathf.Abs(p.z)<=10000;
        }
        bool StillConfirming(PositionConfirmation p)
        {
            try
            {
                FieldWire.Session live;
                return confirmation==p && Stopwatch.GetTimestamp()<p.deadline && p.room!=null && p.room==RoomManager.currentRoom && StaticData.fieldId=="field_003" &&
                    p.room.charactorManager==manager && manager.GetSelfCharactor()==p.actor &&
                    PositionSync.ConnectionEpoch==p.epoch && PositionSync.OwnsLock(p.lease) && FieldWire.Current(out live) && SameSession(live,p.owner) &&
                    !TravelController.Pending && !LeaveController.Pending &&
                    current!=null && current.id==p.eventId && current.state=="active" && current.hp>0 &&
                    manager.GetCharactor(p.eventId)!=null && p.actor!=null &&
                    (p.actor.transform.position-p.position).sqrMagnitude<=.0001f;
            }
            catch (Exception) { return false; }
        }
        // Mirror p8Near: the native PointNew coordinates are floats, while the
        // server accumulates the squares as doubles. Keep its radius check.
        bool ServerNear(Vector3 position)
        {
            double x=(double)(position.x-current.position.x), y=(double)(position.y-current.position.y), z=(double)(position.z-current.position.z);
            return x*x+y*y+z*z<=(double)current.radius*current.radius;
        }
        void FinishConfirmation(PositionConfirmation p,bool accepted,bool cancelled,bool notify)
        {
            if (confirmation!=p) return;
            confirmation=null;
            bool released=PositionSync.ReleaseIfOwned(p.lease);
            if (!released) accepted=false;
            if (notify && p.completed!=null) p.completed(accepted,cancelled);
        }
        // Stop at the actual contact point, send the same original CSMove as
        // the half-second native position sync, and read committed position.
        // The read may race the asynchronous socket write; only a matching
        // server result permits the caller to send the original boss Start.
        public bool ConfirmPosition(RoomBase room,RoomCharactor actor,FieldWire wire,FieldWire.Session owner,Action<bool,bool> completed)
        {
            FieldWire.Session live; long lease=0;
            try
            {
                if (confirmation!=null || room==null || room!=RoomManager.currentRoom || StaticData.fieldId!="field_003" || room.charactorManager!=manager ||
                    actor==null || manager.GetSelfCharactor()!=actor || wire==null || completed==null ||
                    TravelController.Pending || LeaveController.Pending ||
                    !FieldWire.Current(out live) || !SameSession(owner,live) || current==null || !CanInteract(current.id,actor)) return false;
            }
            catch (Exception) { return false; }
            Vector3 position;
            try
            {
                if (!PositionSync.TryLock(actor,out lease)) return false;
                position=actor.transform.position;
                if (!ValidPosition(position) || !CanInteract(current.id,actor) || !ServerNear(position))
                { PositionSync.ReleaseIfOwned(lease); return false; }
            }
            catch (Exception) { PositionSync.ReleaseIfOwned(lease); return false; }
            PositionConfirmation p=new PositionConfirmation { room=room,actor=actor,wire=wire,owner=owner,completed=completed,
                eventId=current.id,position=position,epoch=PositionSync.ConnectionEpoch,lease=lease,
                deadline=Stopwatch.GetTimestamp()+5L*Stopwatch.Frequency };
            confirmation=p;
            try
            {
                var move=new CSMove { pointNew=new PointNew { x=position.x,y=position.y,z=position.z },
                    currentTime=ServerTimerTool.CurrentUTCJavaTime() };
                NetAPI.SendSocket(30026,move);
                PollPosition(p);
            }
            catch (Exception) { FinishConfirmation(p,false,false,true); }
            return true;
        }
        void PollPosition(PositionConfirmation p)
        {
            if (!StillConfirming(p) || p.inFlight) return;
            p.inFlight=true;
            try
            {
                p.wire.PositionStatus(p.owner,delegate(FieldWireResult result)
                {
                    if (confirmation!=p) return;
                    p.inFlight=false;
                    if (!StillConfirming(p)) { FinishConfirmation(p,false,false,true); return; }
                    try
                    {
                        JsonData d=result==null ? null : result.data;
                        if (result!=null && !result.uncertain && result.code==0 && d!=null &&
                            (string)d["scene"]=="field_003" && d["loading"].IsBoolean && !(bool)d["loading"])
                        {
                            Vector3 observed=Vector(d["position"]);
                            if (ValidPosition(observed) && (observed-p.position).sqrMagnitude<=.0001f && ServerNear(observed))
                            { FinishConfirmation(p,true,false,true); return; }
                        }
                    }
                    catch (Exception) { } // Retry a missing or malformed position within the bounded window.
                    p.nextPoll=Stopwatch.GetTimestamp()+Math.Max(1L,Stopwatch.Frequency/4);
                });
            }
            catch (Exception)
            { p.inFlight=false; p.nextPoll=Stopwatch.GetTimestamp()+Math.Max(1L,Stopwatch.Frequency/4); }
        }
        void TickConfirmation(long tick)
        {
            PositionConfirmation p=confirmation;
            if (p==null) return;
            if (!StillConfirming(p)) { FinishConfirmation(p,false,false,true); return; }
            if (Input.GetKeyDown(KeyCode.Escape)) { FinishConfirmation(p,false,true,true); return; }
            if (tick>=p.deadline) { FinishConfirmation(p,false,false,true); return; }
            if (!p.inFlight && tick>=p.nextPoll) PollPosition(p);
        }
        public void ApplySnapshot(JsonData snapshot)
        {
            int epoch=PositionSync.ConnectionEpoch;
            // An old in-flight response may arrive before the frame that sees
            // a reconnect. Discard it and request a snapshot from the new epoch.
            if (current!=null && snapshotEpoch!=epoch && requestedEpoch!=epoch)
            { requestedEpoch=epoch; refresh(); return; }
            JsonData d=snapshot["boss"];
            if (d==null) { Clear(); return; }
            Event e=new Event();
            e.id=(string)d["eventId"]; e.point=(string)d["pointId"]; e.hero=(string)d["heroId"]; e.boss=(string)d["bossId"]; e.state=(string)d["state"];
            e.position=Vector(d["position"]); e.radius=Number(d["radius"]); e.hp=Integer(d["hp"]); e.maxHp=Integer(d["maxHp"]);
            e.start=Integer(d["startsAtMs"]); e.end=Integer(d["endsAtMs"]); e.warning=Integer(d["warningAtMs"]); e.serverTime=Integer(snapshot["serverTimeMs"]);
            long level=Integer(d["level"]); e.nature=Integer(d["nature"]);
            if (String.IsNullOrEmpty(e.id)||e.id.Length>128||String.IsNullOrEmpty(e.point)||String.IsNullOrEmpty(e.hero)||String.IsNullOrEmpty(e.boss)||
                DictHeroBlo.GetTableHero(e.hero)==null||e.radius<=0||e.radius>100||e.maxHp<=0||e.hp<0||e.hp>e.maxHp||level<=0||
                e.start<=0||e.end<=e.start||e.warning>e.start||e.serverTime<=0||e.nature<0||e.nature>Int32.MaxValue||
                (e.state!="waiting" && e.state!="active" && e.state!="defeated" && e.state!="expired") ||
                (e.state=="active" && e.hp<=0)) throw new InvalidOperationException("Invalid authoritative boss");
            if (current!=null && (!SameVisual(current,e) || e.state!="active")) Remove(current);
            if (e.state=="active")
            {
                bool old=current!=null && current.state=="active" && SameVisual(current,e);
                RoomCharactor actor=manager.GetCharactor(e.id);
                if (!old && actor!=null) throw new InvalidOperationException("Boss actor ID collision");
                var side=new SideBoss { uuid=e.id,bossPosId=e.point,bossId=e.boss,heroId=e.hero,
                    currentPoint=new PointNew { x=e.position.x,y=e.position.y,z=e.position.z },
                    state=1,curHp=(double)e.hp/e.maxHp,nature=(int)e.nature };
                if (actor==null)
                {
                    try { manager.CreateFieldBoss(side); }
                    catch (Exception) { Remove(e); throw; }
                    actor=manager.GetCharactor(e.id);
                    if (actor==null || !(actor.ai is RCAIFieldBoss) || actor.ui==null)
                    { Remove(e); throw new InvalidOperationException("Original boss actor unavailable"); }
                    actor.ui.CreateBoxCollider();
                }
                ((RCAIFieldBoss)actor.ai).SetHp((float)side.curHp);
            }
            long received=Stopwatch.GetTimestamp();
            // A repeated snapshot for the same deadline may use an older wall
            // clock. Preserve elapsed monotonic time without carrying it into a
            // new event or a newly established respawn deadline.
            if (current!=null && current.id==e.id && current.state=="waiting" && e.state=="waiting" && current.start==e.start)
                e.serverTime=Math.Max(e.serverTime,ServerNow(received));
            current=e; receivedTicks=received; snapshotEpoch=epoch; requestedEpoch=epoch; mapOwner=this;
            Tick();
        }
        public void Tick()
        {
            long tick=Stopwatch.GetTimestamp();
            TickConfirmation(tick);
            if (current==null) return;
            int epoch=PositionSync.ConnectionEpoch;
            if (snapshotEpoch!=epoch)
            {
                if (requestedEpoch!=epoch) { requestedEpoch=epoch; refresh(); }
                return;
            }
            long now=ServerNow(tick);
            if ((current.state=="waiting" && now>=current.start) ||
                (current.state=="active" && (now>=current.end || tick-lastRefresh>=5L*Stopwatch.Frequency)))
            {
                if (tick-lastRefresh>=2L*Stopwatch.Frequency) { lastRefresh=tick; refresh(); }
            }
            // Map labels derive directly from this room's validated snapshot.
            // The native nature timer is process-static and would leak a desert
            // deadline into another field after a scene transition.
        }
    }
}
