using System;
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
        }
        readonly RoomNetField net;
        readonly RoomCharactorManager manager;
        readonly Dictionary<string,Point> points=new Dictionary<string,Point>();
        long revision=-1,line=-1;
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
        { return a.uuid==b.uuid && a.generation==b.generation && a.monId==b.monId && a.heroId==b.heroId && a.position==b.position; }
        static SideMonster Side(Point p,int state)
        {
            return new SideMonster { uuid=p.uuid, monsterPosId=p.id, monId=p.monId, heroId=p.heroId,
                currentPoint=new PointNew { x=p.position.x,y=p.position.y,z=p.position.z }, state=state,curHp=state==3 ? 0:p.hpRatio };
        }
        static int State(Point p) { return p.available ? 1 : p.refreshAtMs>0 ? 3 : 2; }
        public void ApplySnapshot(JsonData snapshot)
        {
            if ((string)snapshot["scene"]!="field_003") throw new InvalidOperationException("Wrong monster scene");
            long nextLine=Integer(snapshot["line"]), nextRevision=Integer(snapshot["revision"]), count=Integer(snapshot["monsterCount"]);
            if (nextLine<1 || nextRevision<0 || count<1 || count>100 || (line!=-1 && line!=nextLine) || nextRevision<revision) throw new InvalidOperationException("Invalid monster revision");
            var next=new Dictionary<string,Point>(); var uuids=new HashSet<string>();
            foreach (JsonData row in snapshot["monsters"])
            {
                Point p=new Point(); p.id=(string)row["pointId"]; p.uuid=(string)row["uuid"]; p.monId=(string)row["monId"]; p.heroId=(string)row["heroId"];
                p.generation=Integer(row["generation"]); p.level=Integer(row["level"]); p.refreshAtMs=Integer(row["refreshAtMs"]); p.hpRatio=Double(row["hpRatio"]); p.position=Vector(row["position"]);
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
                    if (nextRevision==revision && (!SameVisual(old,p)||p.available!=old.available||p.refreshAtMs!=old.refreshAtMs||p.hpRatio!=old.hpRatio)) throw new InvalidOperationException("Monster changed without revision");
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
            points.Clear(); foreach (Point p in next.Values) points.Add(p.id,p);
            line=nextLine; revision=nextRevision;
        }
        public void Clear()
        {
            if (points.Count>0)
            {
                var packet=new SCSideMonster(); foreach (Point p in points.Values) packet.monsters.Add(Side(p,3));
                HYJS.Field63.Runtime.Monsters(net,packet);
            }
            points.Clear(); line=-1; revision=-1;
        }
    }
}
