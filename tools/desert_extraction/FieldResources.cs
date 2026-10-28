using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using Dict;
using Dict.Blo;
using Dict.DataModel;
using LitJson;
using ProtoBuf.Message;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // This adapter projects only a complete authoritative server snapshot using
    // existing game resources. It never generates a tier, position or reward.
    // A caller owns the room lifecycle, fetch, delta notification and disposal.
    public sealed class FieldResources
    {
        public sealed class Point
        {
            public string id, kind, nativePosId, groupId;
            public long generation, remaining, refreshAtMs;
            public int level;
            public float radius;
            public Vector3 position, standPosition;
        }
        sealed class Rendered { public Point point; public TableFieldResourcePos table; public SideResource native; }
        readonly RoomCharactorManager manager;
        readonly Dictionary<string,Rendered> rendered = new Dictionary<string,Rendered>();
        readonly Dictionary<string,Point> points = new Dictionary<string,Point>();
        long revision = -1, line = -1;
        public FieldResources(RoomCharactorManager value)
        {
            if (value == null) throw new ArgumentNullException("manager");
            manager=value;
        }
        static long Integer(JsonData value) { return Int64.Parse(value.ToString(),CultureInfo.InvariantCulture); }
        static float Number(JsonData value)
        {
            float number=Single.Parse(value.ToString(),CultureInfo.InvariantCulture);
            if (Single.IsNaN(number) || Single.IsInfinity(number)) throw new InvalidOperationException("Non-finite resource coordinate");
            return number;
        }
        static Vector3 Vector(JsonData value)
        { return new Vector3(Number(value["x"]),Number(value["y"]),Number(value["z"])); }
        static string NativeId(string id) { return "ggo_v4_" + id; }
        static string ObjectKey(Point p) { return p.groupId + "@" + p.nativePosId; }
        static bool SameVisual(Point a, Point b)
        { return a.groupId==b.groupId && a.position==b.position && a.standPosition==b.standPosition && a.radius==b.radius; }
        static Point Parse(JsonData value)
        {
            Point p=new Point(); p.id=(string)value["pointId"]; p.kind=(string)value["kind"];
            p.generation=Integer(value["generation"]); p.remaining=Integer(value["remaining"]); p.refreshAtMs=Integer(value["refreshAtMs"]); p.level=checked((int)Integer(value["level"]));
            p.position=Vector(value["position"]); p.standPosition=Vector(value["standPosition"]); p.radius=Number(value["radius"]);
            if (String.IsNullOrEmpty(p.id) || p.id.Length>64 || p.id.Trim()!=p.id ||
                (p.kind!="mine" && p.kind!="fish") || p.generation<1 || p.remaining<0 || p.remaining>100 ||
                p.refreshAtMs<0 || p.level<3 || p.level>6 || p.radius<=0 || p.radius>100)
                throw new InvalidOperationException("Invalid resource snapshot point");
            p.nativePosId=NativeId(p.id);
            p.groupId="group_" + p.kind + "_" + p.level.ToString("D3",CultureInfo.InvariantCulture);
            TableFieldResource original=DictFieldResourceBlo.GetOne(p.groupId);
            if (original==null || original.level!=p.level || String.IsNullOrEmpty(original.model)) throw new InvalidOperationException("Original resource asset mapping missing");
            return p;
        }
        public Point FindByNativePosId(string id)
        {
            if (id==null || !id.StartsWith("ggo_v4_",StringComparison.Ordinal)) return null;
            Point point; return points.TryGetValue(id.Substring(7),out point) ? point : null;
        }
        // Requires a complete snapshot and monotonically increasing line revision.
        // The caller must discard stale owner/scene sessions before invoking it.
        public void ApplySnapshot(JsonData snapshot)
        {
            if ((string)snapshot["scene"]!="field_003") throw new InvalidOperationException("Wrong scene snapshot");
            long nextLine=Integer(snapshot["line"]), nextRevision=Integer(snapshot["revision"]);
            if (nextLine<1 || nextRevision<0 || (line!=-1 && line!=nextLine) || nextRevision<revision) throw new InvalidOperationException("Stale or wrong-line resource snapshot");
            var next=new Dictionary<string,Point>(); int mines=0,fish=0;
            foreach (JsonData item in snapshot["resources"])
            {
                Point p=Parse(item); if (next.ContainsKey(p.id)) throw new InvalidOperationException("Duplicate resource point");
                Point old; if (points.TryGetValue(p.id,out old) && p.generation<old.generation) throw new InvalidOperationException("Resource generation regressed");
                next.Add(p.id,p); if (p.kind=="mine") mines++; else fish++;
            }
            // Counts come from the same published runtime configuration as the
            // snapshot. Exhausted nodes remain in the array, so partial arrays
            // must never replace a complete public scene.
            long expectedMines=Integer(snapshot["resourceMineCount"]), expectedFish=Integer(snapshot["resourceFishCount"]);
            if (expectedMines<1 || expectedMines>50 || expectedFish<1 || expectedFish>50 ||
                mines!=expectedMines || fish!=expectedFish || next.Count!=mines+fish)
                throw new InvalidOperationException("Incomplete authoritative resource snapshot");
            if (revision == nextRevision && points.Count == next.Count)
                foreach (Point p in next.Values)
                {
                    Point old=points[p.id];
                    if (!SameVisual(old,p) || old.standPosition!=p.standPosition || old.generation!=p.generation || old.remaining!=p.remaining || old.refreshAtMs!=p.refreshAtMs)
                        throw new InvalidOperationException("Resource snapshot changed without a revision");
                }
            var cache=DictDataManager.Instance.tableFieldResourcePosDao.CachePrimary;
            if (cache==null || StaticData.fieldRes==null) throw new InvalidOperationException("Native resource caches unavailable");
            foreach (Point p in next.Values)
                if (cache.ContainsKey(p.nativePosId) && !rendered.ContainsKey(p.id)) throw new InvalidOperationException("Resource position ID collision");
            try
            {
                var remove=new List<string>();
                foreach (KeyValuePair<string,Rendered> entry in rendered)
                {
                    Point p=next[entry.Key];
                    if (p.remaining==0 || !SameVisual(entry.Value.point,p)) remove.Add(entry.Key);
                }
                foreach (string id in remove) Remove(id);
                foreach (Point p in next.Values)
                {
                    if (p.remaining==0) continue;
                    Rendered old;
                    if (rendered.TryGetValue(p.id,out old)) { old.point=p; continue; }
                    Add(p);
                }
                points.Clear(); foreach (Point p in next.Values) points.Add(p.id,p);
                line=nextLine; revision=nextRevision;
            }
            catch (Exception)
            {
                Clear(); throw;
            }
        }
        void Add(Point p)
        {
            var table=new TableFieldResourcePos(); table.posId=p.nativePosId;
            // Preserve the server's radius as the native resource threshold.
            // Actor-to-stand and actor-to-model 3D checks remain separate.
            table.r=p.radius; table.angle=0;
            table.pos=p.position.x.ToString("R",CultureInfo.InvariantCulture)+","+p.position.y.ToString("R",CultureInfo.InvariantCulture)+","+p.position.z.ToString("R",CultureInfo.InvariantCulture);
            table.resourceIdProbs=p.groupId+",100";
            var cache=DictDataManager.Instance.tableFieldResourcePosDao.CachePrimary;
            string key=ObjectKey(p);
            if (StaticData.fieldRes.ContainsKey(key) || manager.GetCharactor(key)!=null) throw new InvalidOperationException("Native resource key collision");
            cache.Add(table.posId,table);
            var item=new Rendered(); item.point=p; item.table=table; rendered.Add(p.id,item);
            manager.CreateResource(p.groupId,p.nativePosId);
            if (manager.GetCharactor(key)==null) throw new InvalidOperationException("Native resource was not created");
            item.native=new SideResource { id=p.groupId, posId=p.nativePosId, refreshTime=0 };
            StaticData.fieldRes.Add(key,item.native);
        }
        void Remove(string id)
        {
            Rendered old; if (!rendered.TryGetValue(id,out old)) return;
            string key=ObjectKey(old.point); manager.DestroyCharactor(key);
            SideResource cached;
            if (StaticData.fieldRes!=null && StaticData.fieldRes.TryGetValue(key,out cached) && System.Object.ReferenceEquals(cached,old.native)) StaticData.fieldRes.Remove(key);
            var cache=DictDataManager.Instance.tableFieldResourcePosDao.CachePrimary;
            TableFieldResourcePos row;
            if (cache!=null && cache.TryGetValue(old.table.posId,out row) && System.Object.ReferenceEquals(row,old.table)) cache.Remove(old.table.posId);
            rendered.Remove(id);
        }
        public void Clear()
        {
            foreach (string id in new List<string>(rendered.Keys)) Remove(id);
            points.Clear(); line=-1; revision=-1;
        }
    }
}
