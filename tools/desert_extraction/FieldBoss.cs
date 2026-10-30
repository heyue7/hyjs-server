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
        readonly RoomCharactorManager manager;
        readonly Action refresh;
        static FieldBoss mapOwner;
        Event current;
        string shownWarning="";
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
            try { Remove(current); }
            finally { current=null; shownWarning=""; }
        }
        // The map reads only a validated event belonging to the current room.
        // Crossing a scheduled boundary needs a fresh server state; the local
        // clock alone must never claim that a boss has appeared or survived.
        public static bool MapMarker(RoomBase room,out string id,out Vector3 position,out string state)
        {
            id=null; position=Vector3.zero; state=null;
            FieldBoss owner=mapOwner;
            if (owner==null || owner.current==null || room==null || room!=RoomManager.currentRoom ||
                room.charactorManager!=owner.manager || StaticData.fieldId!="field_003") return false;
            Event e=owner.current;
            long elapsed=(Stopwatch.GetTimestamp()-owner.receivedTicks)*1000/Stopwatch.Frequency;
            long now=e.serverTime+Math.Max(0,elapsed);
            id=e.id; position=e.position; state=e.state;
            if ((state=="waiting" && now>=e.start) || (state=="active" && now>=e.end)) state="syncing";
            return true;
        }
        public bool CanInteract(string id,RoomCharactor actor)
        {
            return current!=null && current.state=="active" && current.hp>0 && current.id==id && actor!=null &&
                (actor.transform.position-current.position).sqrMagnitude<=current.radius*current.radius;
        }
        public void ApplySnapshot(JsonData snapshot)
        {
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
            if (current==null || current.id!=e.id) shownWarning="";
            current=e; receivedTicks=Stopwatch.GetTimestamp(); mapOwner=this;
            Tick();
        }
        public void Tick()
        {
            if (current==null) return;
            long tick=Stopwatch.GetTimestamp();
            long elapsed=(tick-receivedTicks)*1000/Stopwatch.Frequency;
            long now=current.serverTime+Math.Max(0,elapsed);
            if ((current.state=="waiting" && now>=current.start) ||
                (current.state=="active" && (now>=current.end || tick-lastRefresh>=5L*Stopwatch.Frequency)))
            {
                if (tick-lastRefresh>=2L*Stopwatch.Frequency) { lastRefresh=tick; refresh(); }
            }
            if (current.state!="waiting" || shownWarning==current.id || now<current.warning || now>=current.start) return;
            // Nature is the server's native SideBoss value; the original map
            // timer accepts seconds and updates its own NGUI countdown.
            float seconds=(float)((current.start-now)/1000.0);
            UI.UIFieldRunMap.RefreshBossTime((int)current.nature,seconds);
            shownWarning=current.id;
        }
    }
}
