using System;
using System.Collections;
using System.Diagnostics;
using System.Collections.Generic;
using System.Globalization;
using Dict.Blo;
using LitJson;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // The existing Field63 bridge supplies original models, collision handlers
    // and ordinary fight UI. All identity, state and position come from the
    // complete authoritative line snapshot, never from legacy private nodes.
    public sealed class FieldMonsters
    {
        sealed class Point
        {
            public string id,uuid,monId,heroId;
            public long generation,level,refreshAtMs;
            public Vector3 position;
            public bool available;
            public double hpRatio;
            public Patrol patrol;
        }
        struct DPoint
        {
            public double x,y,z;
            public DPoint(double x,double y,double z) { this.x=x; this.y=y; this.z=z; }
        }
        sealed class Patrol
        {
            public DPoint[] waypoints;
            public double speed,length;
            public long epochMs,pausedAtMs;
        }
        readonly RoomNetField net;
        readonly RoomCharactorManager manager;
        readonly Dictionary<string,Point> points=new Dictionary<string,Point>();
        long revision=-1,line=-1,clockTicks,clockServerMs,bestRttTicks,sampleTicks;
        public FieldMonsters(RoomNetField field,RoomCharactorManager characters)
        { if (field==null || characters==null) throw new ArgumentNullException("Monster field"); net=field; manager=characters; }
        static long Integer(JsonData d) { return Int64.Parse(d.ToString(),CultureInfo.InvariantCulture); }
        static double Double(JsonData d)
        {
            double n=System.Double.Parse(d.ToString(),CultureInfo.InvariantCulture);
            if (System.Double.IsNaN(n) || System.Double.IsInfinity(n) || Math.Abs(n)>100000) throw new InvalidOperationException("Invalid monster value"); return n;
        }
        static Vector3 Vector(JsonData d) { return new Vector3((float)Double(d["x"]),(float)Double(d["y"]),(float)Double(d["z"])); }
        static bool SameVisual(Point a,Point b)
        { return a.uuid==b.uuid && a.generation==b.generation && a.monId==b.monId && a.heroId==b.heroId; }
        static bool SamePatrol(Patrol a,Patrol b)
        {
            if (a==null || b==null) return a==b;
            if (a.speed!=b.speed || a.epochMs!=b.epochMs || a.pausedAtMs!=b.pausedAtMs || a.waypoints.Length!=b.waypoints.Length) return false;
            for (int i=0;i<a.waypoints.Length;i++)
                if (a.waypoints[i].x!=b.waypoints[i].x || a.waypoints[i].y!=b.waypoints[i].y || a.waypoints[i].z!=b.waypoints[i].z) return false;
            return true;
        }
        static Patrol ParsePatrol(JsonData row)
        {
            if (!((IDictionary)row).Contains("patrol") || row["patrol"]==null) return null;
            JsonData d=row["patrol"], list=d["waypoints"];
            if (!list.IsArray || list.Count<2 || list.Count>32) throw new InvalidOperationException("Invalid public patrol path");
            Patrol p=new Patrol(); p.speed=Double(d["speed"]); p.epochMs=Integer(d["epochMs"]); p.pausedAtMs=Integer(d["pausedAtMs"]);
            if (p.speed<=0 || p.speed>50 || p.epochMs<=0 || p.pausedAtMs<0) throw new InvalidOperationException("Invalid public patrol clock");
            p.waypoints=new DPoint[list.Count];
            for (int i=0;i<list.Count;i++)
            {
                JsonData v=list[i]; p.waypoints[i]=new DPoint(Double(v["x"]),Double(v["y"]),Double(v["z"]));
                if (i>0)
                {
                    double segment=Distance(p.waypoints[i-1],p.waypoints[i]);
                    if (segment<.001) throw new InvalidOperationException("Invalid patrol segment");
                    p.length+=segment;
                }
            }
            if (p.length>10000) throw new InvalidOperationException("Patrol path too long");
            return p;
        }
        static double Distance(DPoint a,DPoint b)
        {
            double x=b.x-a.x,y=b.y-a.y,z=b.z-a.z;
            return Math.Sqrt(x*x+y*y+z*z);
        }
        static Vector3 Position(Patrol p,long nowMs)
        {
            long at=p.pausedAtMs>0 ? p.pausedAtMs : nowMs;
            if (at<p.epochMs) at=p.epochMs;
            double travelMs=p.length/p.speed*1000, waitMs=12000;
            double phase=(double)(at-p.epochMs)%(2*(waitMs+travelMs));
            double progress=phase<waitMs ? 0 : phase<waitMs+travelMs ? (phase-waitMs)/travelMs :
                phase<2*waitMs+travelMs ? 1 : 1-(phase-2*waitMs-travelMs)/travelMs;
            double distance=progress*p.length;
            for (int i=1;i<p.waypoints.Length;i++)
            {
                DPoint a=p.waypoints[i-1],b=p.waypoints[i]; double segment=Distance(a,b);
                if (distance<=segment || i==p.waypoints.Length-1)
                {
                    double ratio=Math.Max(0,Math.Min(1,distance/segment));
                    return new Vector3((float)(a.x+(b.x-a.x)*ratio),(float)(a.y+(b.y-a.y)*ratio),(float)(a.z+(b.z-a.z)*ratio));
                }
                distance-=segment;
            }
            throw new InvalidOperationException("Invalid patrol projection");
        }
        static bool Moving(Patrol p,long nowMs)
        {
            if (p.pausedAtMs>0 || nowMs<p.epochMs) return false;
            double travelMs=p.length/p.speed*1000, waitMs=12000;
            double phase=(double)(nowMs-p.epochMs)%(2*(waitMs+travelMs));
            return (phase>=waitMs && phase<waitMs+travelMs) || phase>=2*waitMs+travelMs;
        }
        static SideMonster Side(Point p,int state)
        {
            return new SideMonster { uuid=p.uuid, monsterPosId=p.id, monId=p.monId, heroId=p.heroId,
                currentPoint=new PointNew { x=p.position.x,y=p.position.y,z=p.position.z }, state=state,curHp=state==3 ? 0:p.hpRatio };
        }
        static int State(Point p) { return p.available ? 1 : p.refreshAtMs>0 ? 3 : 2; }
        public void ApplySnapshot(JsonData snapshot,long sentTicks,long receivedTicks)
        {
            if ((string)snapshot["scene"]!="field_003") throw new InvalidOperationException("Wrong monster scene");
            long nextLine=Integer(snapshot["line"]), nextRevision=Integer(snapshot["revision"]), count=Integer(snapshot["monsterCount"]), nowMs=Integer(snapshot["serverTimeMs"]);
            if (sentTicks<=0 || receivedTicks<sentTicks || nowMs<=0 || nextLine<1 || nextRevision<0 || count<1 || count>100 || (line!=-1 && line!=nextLine) || nextRevision<revision) throw new InvalidOperationException("Invalid monster revision");
            var next=new Dictionary<string,Point>(); var uuids=new HashSet<string>();
            foreach (JsonData row in snapshot["monsters"])
            {
                Point p=new Point(); p.id=(string)row["pointId"]; p.uuid=(string)row["uuid"]; p.monId=(string)row["monId"]; p.heroId=(string)row["heroId"];
                p.generation=Integer(row["generation"]); p.level=Integer(row["level"]); p.refreshAtMs=Integer(row["refreshAtMs"]); p.hpRatio=Double(row["hpRatio"]); p.position=Vector(row["position"]); p.patrol=ParsePatrol(row);
                if (p.patrol!=null && (Position(p.patrol,nowMs)-p.position).sqrMagnitude>.0004f) throw new InvalidOperationException("Public patrol clock/position mismatch");
                if (!row["available"].IsBoolean) throw new InvalidOperationException("Missing monster availability"); p.available=(bool)row["available"];
                if (String.IsNullOrEmpty(p.id)||p.id.Length>64||String.IsNullOrEmpty(p.uuid)||p.uuid.Length>128||String.IsNullOrEmpty(p.monId)||String.IsNullOrEmpty(p.heroId)||
                    p.generation<1||p.level<1||p.level>1000||p.refreshAtMs<0||p.hpRatio<0||p.hpRatio>1||next.ContainsKey(p.id)||!uuids.Add(p.uuid)||DictHeroBlo.GetTableHero(p.heroId)==null)
                    throw new InvalidOperationException("Invalid public monster");
                if ((p.available && p.refreshAtMs!=0) || (p.refreshAtMs==0 && p.hpRatio!=1) || (p.refreshAtMs>0 && p.hpRatio!=0)) throw new InvalidOperationException("Contradictory monster state");
                Point old;
                if (points.TryGetValue(p.id,out old))
                {
                    if (p.generation<old.generation) throw new InvalidOperationException("Monster generation regressed");
                    if (p.generation==old.generation && p.uuid!=old.uuid) throw new InvalidOperationException("Monster identity changed within generation");
                    if (nextRevision==revision && (!SameVisual(old,p)||!SamePatrol(old.patrol,p.patrol)||(p.patrol==null && p.position!=old.position)||p.available!=old.available||p.refreshAtMs!=old.refreshAtMs||p.hpRatio!=old.hpRatio)) throw new InvalidOperationException("Monster changed without revision");
                }
                next.Add(p.id,p);
            }
            if (next.Count!=count) throw new InvalidOperationException("Incomplete public monsters");
            var packet=new SCSideMonster();
            foreach (KeyValuePair<string,Point> old in points)
            {
                Point replacement;
                if (!next.TryGetValue(old.Key,out replacement) || !SameVisual(old.Value,replacement)) packet.monsters.Add(Side(old.Value,3));
            }
            foreach (Point p in next.Values)
            {
                Point old; bool owned=points.TryGetValue(p.id,out old) && old.uuid==p.uuid;
                if (!owned && manager.GetCharactor(p.uuid)!=null) throw new InvalidOperationException("Monster UUID collision");
                packet.monsters.Add(Side(p,State(p)));
            }
            HYJS.Field63.Runtime.Monsters(net,packet);
            foreach (Point p in next.Values)
                if (p.patrol==null && manager.GetCharactor(p.uuid)!=null) manager.SetPosition(p.uuid,p.position);
            points.Clear(); foreach (Point p in next.Values) points.Add(p.id,p);
            line=nextLine; revision=nextRevision;
            long rtt=receivedTicks-sentTicks;
            // The server timestamp was produced during this request. Prefer a
            // recent minimum RTT sample; asymmetric networks still have error.
            if (clockServerMs==0 || receivedTicks-sampleTicks>30L*Stopwatch.Frequency || rtt<bestRttTicks)
            {
                bestRttTicks=rtt; sampleTicks=receivedTicks; clockTicks=receivedTicks;
                clockServerMs=nowMs+(long)((double)rtt*500/Stopwatch.Frequency);
            }
            Tick();
        }
        public void Tick()
        {
            if (clockServerMs<=0) return;
            long elapsed=(long)((double)(Stopwatch.GetTimestamp()-clockTicks)*1000/Stopwatch.Frequency);
            long now=clockServerMs+Math.Max(0,elapsed);
            foreach (Point p in points.Values)
            {
                if (p.patrol==null || p.refreshAtMs>0) continue;
                RoomCharactor actor=manager.GetCharactor(p.uuid);
                if (actor==null) continue;
                Vector3 previous=actor.transform.position,position=Position(p.patrol,now);
                manager.SetPosition(p.uuid,position);
                Vector3 facing=position-previous; facing.y=0;
                if (p.patrol.pausedAtMs==0 && facing.sqrMagnitude>.0000001f) actor.transform.forward=facing.normalized;
            }
        }
        public bool IsPatrolling(RoomCharactor actor)
        {
            if (actor==null || clockServerMs<=0) return false;
            long elapsed=(long)((double)(Stopwatch.GetTimestamp()-clockTicks)*1000/Stopwatch.Frequency);
            long now=clockServerMs+Math.Max(0,elapsed);
            foreach (Point p in points.Values)
                if (p.patrol!=null && p.refreshAtMs==0 && Moving(p.patrol,now) && manager.GetCharactor(p.uuid)==actor) return true;
            return false;
        }
        public void Clear()
        {
            if (points.Count>0)
            {
                var packet=new SCSideMonster(); foreach (Point p in points.Values) packet.monsters.Add(Side(p,3));
                HYJS.Field63.Runtime.Monsters(net,packet);
            }
            points.Clear(); line=-1; revision=-1; clockServerMs=clockTicks=bestRttTicks=sampleTicks=0;
        }
    }
}
