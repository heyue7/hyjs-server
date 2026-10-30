using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using Dict.Blo;
using LitJson;
using Room;
using UnityEngine;

namespace HYJS.DesertV4
{
    // NPC identity, model, positions and interaction radius are taken only from
    // the same authoritative snapshot as the public field. Native NPC actors,
    // click ranges, labels and OnClick remain those shipped in the game.
    public sealed class FieldNpcs
    {
        public const string PageMarker="HYJS_GGO_DESERT_V4";
        public sealed class Point
        {
            public string id,name,heroId,role,pointId,nativeId;
            public Vector3 position,interactionPosition;
            public float radius;
        }
        readonly RoomCharactorManager manager;
        readonly Dictionary<string,Point> points=new Dictionary<string,Point>();
        readonly Dictionary<RoomCharactor,Point> owners=new Dictionary<RoomCharactor,Point>();
        readonly Dictionary<string,RoomCharactor> actors=new Dictionary<string,RoomCharactor>();
        public FieldNpcs(RoomCharactorManager source) { if (source==null) throw new ArgumentNullException("NPC manager"); manager=source; }
        static string Optional(JsonData data,string name)
        { var dict=data as IDictionary; return dict!=null && dict.Contains(name) && data[name]!=null ? data[name].ToString() : ""; }
        static float Number(JsonData value)
        {
            float n=Single.Parse(value.ToString(),CultureInfo.InvariantCulture);
            if (Single.IsNaN(n) || Single.IsInfinity(n)) throw new InvalidOperationException("Invalid NPC number"); return n;
        }
        static Vector3 Vector(JsonData value) { return new Vector3(Number(value["x"]),Number(value["y"]),Number(value["z"])); }
        static bool Same(Point a,Point b)
        { return a.id==b.id && a.name==b.name && a.heroId==b.heroId && a.role==b.role && a.pointId==b.pointId && a.position==b.position && a.interactionPosition==b.interactionPosition && a.radius==b.radius; }
        public Point Find(RoomCharactor owner) { Point p; return owner!=null && owners.TryGetValue(owner,out p) ? p : null; }
        // Roles come from the accepted snapshot; positions come from the actual
        // spawned actors, using the same local coordinates as the native map.
        public Dictionary<string,Vector3> ExtractionMapPositions()
        {
            var result=new Dictionary<string,Vector3>();
            foreach (Point point in points.Values)
            {
                RoomCharactor actor;
                if (point.role=="extraction" && actors.TryGetValue(point.id,out actor) && actor!=null)
                    result.Add(point.id,actor.transform.localPosition);
            }
            return result;
        }
        public void ApplySnapshot(JsonData snapshot)
        {
            JsonData area=snapshot["safeArea"];
            float minX=Number(area["minX"]),maxX=Number(area["maxX"]),minZ=Number(area["minZ"]),maxZ=Number(area["maxZ"]);
            if (minX>=maxX || minZ>=maxZ) throw new InvalidOperationException("Invalid safe area");
            var exits=new Dictionary<string,Vector3>();
            foreach (JsonData item in snapshot["exits"])
            {
                string id=(string)item["pointId"]; if (String.IsNullOrEmpty(id) || exits.ContainsKey(id)) throw new InvalidOperationException("Invalid exit ID");
                exits.Add(id,Vector(item["position"]));
            }
            if (exits.Count!=3) throw new InvalidOperationException("Expected three authoritative exits");
            var next=new Dictionary<string,Point>(); var exitRoles=new HashSet<string>(); int guide=0,settlement=0;
            foreach (JsonData item in snapshot["npcs"])
            {
                Point p=new Point(); p.id=(string)item["id"]; p.name=(string)item["name"]; p.heroId=(string)item["heroId"]; p.role=(string)item["role"];
                p.pointId=Optional(item,"pointId"); p.position=Vector(item["position"]); p.interactionPosition=Vector(item["interactionPosition"]); p.radius=Number(item["radius"]);
                if (String.IsNullOrEmpty(p.id) || p.id.Length>64 || p.id.Trim()!=p.id || String.IsNullOrEmpty(p.name) || p.name.Length>64 || String.IsNullOrEmpty(p.heroId) ||
                    (p.role!="teleport" && p.role!="settlement" && p.role!="extraction") || p.radius<=0 || p.radius>100 || next.ContainsKey(p.id) || DictHeroBlo.GetTableHero(p.heroId)==null)
                    throw new InvalidOperationException("Invalid NPC snapshot");
                if (p.role=="teleport" || p.role=="settlement")
                {
                    if (p.interactionPosition.x<minX || p.interactionPosition.x>maxX || p.interactionPosition.z<minZ || p.interactionPosition.z>maxZ) throw new InvalidOperationException("Safe NPC outside safe area");
                    if (p.role=="teleport") guide++; else settlement++;
                }
                else
                {
                    Vector3 exit;
                    if (String.IsNullOrEmpty(p.pointId) || !exitRoles.Add(p.pointId) || !exits.TryGetValue(p.pointId,out exit) || (exit-p.interactionPosition).sqrMagnitude>.0001f)
                        throw new InvalidOperationException("Extraction NPC/exit mismatch");
                }
                p.nativeId="ggo_v4_npc_"+p.id;
                next.Add(p.id,p);
            }
            if (guide!=1 || settlement!=1 || exitRoles.Count!=3) throw new InvalidOperationException("Incomplete NPC snapshot");
            try
            {
                var remove=new List<string>();
                foreach (KeyValuePair<string,Point> pair in points)
                { Point p; if (!next.TryGetValue(pair.Key,out p) || !Same(pair.Value,p)) remove.Add(pair.Key); }
                foreach (string id in remove) Remove(id);
                foreach (Point p in next.Values)
                {
                    if (points.ContainsKey(p.id)) continue;
                    if (manager.GetCharactor(p.nativeId)!=null) throw new InvalidOperationException("NPC identifier collision");
                    try { manager.CreateNormalNPC(p.nativeId,p.position.x,p.position.y,p.position.z,0f,p.name,p.heroId,"","stand",PageMarker,"",0,"",""); }
                    catch (Exception) { if (manager.GetCharactor(p.nativeId)!=null) manager.DestroyCharactor(p.nativeId); throw; }
                    RoomCharactor actor=manager.GetCharactor(p.nativeId);
                    RCUINormalNPC ui=actor==null ? null : actor.ui as RCUINormalNPC;
                    if (ui==null) { if (actor!=null) manager.DestroyCharactor(p.nativeId); throw new InvalidOperationException("Native NPC creation failed"); }
                    float dx=p.position.x-p.interactionPosition.x,dz=p.position.z-p.interactionPosition.z;
                    ui.ClickRange=p.radius+Mathf.Sqrt(dx*dx+dz*dz);
                    points.Add(p.id,p); actors.Add(p.id,actor); owners.Add(actor,p);
                }
            }
            catch (Exception) { Clear(); throw; }
        }
        void Remove(string id)
        {
            Point point; RoomCharactor actor;
            if (!points.TryGetValue(id,out point)) return;
            if (actors.TryGetValue(id,out actor)) { owners.Remove(actor); actors.Remove(id); }
            manager.DestroyCharactor(point.nativeId); points.Remove(id);
        }
        public void Clear() { foreach (string id in new List<string>(points.Keys)) Remove(id); }
    }
}
