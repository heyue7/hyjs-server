using System;
using System.Collections.Generic;
using System.IO;
using UnityEditor.SceneManagement;
using UnityEngine;

// Editor-only candidate generator. It uses bounded local searches around failed
// V4 markers; every result remains subject to visual/device/model validation.
public static class HyjsDesertV4FishOptions
{
    const string ScenePath="Assets/Scenes/map_desertfield_001.unity";
    [Serializable] public class InputPoint { public string id,kind; public int pixelX,pixelY; public float worldX,worldZ; }
    [Serializable] public class Input { public string designSha256,minimapSha256,bundleSha256; public InputPoint[] points; }
    [Serializable] public class PriorPoint { public string id; public bool automatedChecksPass; }
    [Serializable] public class Prior { public string schema,designSha256,minimapSha256,bundleSha256; public PriorPoint[] points; }
    [Serializable] public class Lake { public string name; public Vector3[] points; public float level; }
    [Serializable] public class Limits { public Vector3[] outer; public Lake[] water; }
    [Serializable] public class Suggestion { public Vector3 modelPosition,standPosition; public float modelDisplacement,standDistance,standHorizontalDistance,pathLength; public bool eligibleForRelease=false; }
    [Serializable] public class Item { public string id,kind,reason; public Vector3 originalPlan; public Suggestion[] candidates; }
    [Serializable] public class Report
    {
        public string schema,unityVersion,scene,designSha256,minimapSha256,bundleSha256;
        public string[] manualGates; public bool eligibleForRelease=false;
        public int navVertices,navTriangles,failedPointCount; public Item[] points;
    }
    static bool Inside(Vector3[] polygon,Vector3 p)
    {
        bool inside=false;
        for (int i=0,j=polygon.Length-1;i<polygon.Length;j=i++)
        {
            Vector3 a=polygon[i],b=polygon[j];
            if ((a.z>p.z)!=(b.z>p.z) && p.x<(b.x-a.x)*(p.z-a.z)/(b.z-a.z)+a.x) inside=!inside;
        }
        return inside;
    }
    static Lake Water(Limits limits,Vector3 p)
    { foreach (Lake l in limits.water) if (Inside(l.points,p)) return l; return null; }
    static float Horizontal(Vector3 a,Vector3 b)
    { return Vector2.Distance(new Vector2(a.x,a.z),new Vector2(b.x,b.z)); }
    static float Surface(float x,float z)
    {
        RaycastHit[] hits=Physics.RaycastAll(new Vector3(x,50,z),Vector3.down,100);
        float top=-1000;
        foreach (RaycastHit hit in hits) if (hit.normal.y>.45f && hit.point.y>top) top=hit.point.y;
        if (top>-100) return top;
        Terrain t=Terrain.activeTerrain;
        if (t==null) throw new InvalidOperationException("No ground");
        return t.SampleHeight(new Vector3(x,0,z))+t.transform.position.y;
    }
    static bool Reach(Limits limits,Vector3 source,Vector3 probe,out Vector3 result,out float pathLength)
    {
        result=Vector3.zero; pathLength=0;
        try
        {
            if (!Inside(limits.outer,probe) || Water(limits,probe)!=null) return false;
            probe.y=Surface(probe.x,probe.z); NavMeshHit hit;
            if (!NavMesh.SamplePosition(probe,out hit,.75f,-1) || Horizontal(probe,hit.position)>.75f || Water(limits,hit.position)!=null) return false;
            if (Physics.CheckCapsule(hit.position+Vector3.up*.35f,hit.position+Vector3.up*1.5f,.20f,1<<11)) return false;
            var path=new NavMeshPath();
            if (!NavMesh.CalculatePath(source,hit.position,-1,path) || path.status!=NavMeshPathStatus.PathComplete) return false;
            for (int i=1;i<path.corners.Length;i++) pathLength+=Vector3.Distance(path.corners[i-1],path.corners[i]);
            result=hit.position; return true;
        }
        catch (Exception) { return false; }
    }
    static IEnumerable<Vector3> Ring(Vector3 center,float radius,float step)
    {
        if (radius==0) { yield return center; yield break; }
        int count=Mathf.Max(16,Mathf.CeilToInt(2*Mathf.PI*radius/step));
        for (int i=0;i<count;i++)
        { float a=2*Mathf.PI*i/count; yield return new Vector3(center.x+Mathf.Cos(a)*radius,center.y,center.z+Mathf.Sin(a)*radius); }
    }
    static bool Distinct(List<Suggestion> items,Vector3 model,Vector3 stand)
    {
        foreach (Suggestion item in items) if (Horizontal(item.modelPosition,model)<.5f && Horizontal(item.standPosition,stand)<.5f) return false;
        return true;
    }
    static List<Suggestion> Ground(Limits limits,Vector3 spawn,Vector3 plan)
    {
        var result=new List<Suggestion>();
        for (float radius=0;radius<=3.001f && result.Count<5;radius+=.25f)
            foreach (Vector3 probe in Ring(plan,radius,.35f))
            {
                Vector3 hit; float length;
                if (!Reach(limits,spawn,probe,out hit,out length) || Horizontal(plan,hit)>3 || !Distinct(result,hit,hit)) continue;
                result.Add(new Suggestion { modelPosition=hit,standPosition=hit,modelDisplacement=Horizontal(plan,hit),standDistance=0,pathLength=length });
                if (result.Count>=5) break;
            }
        return result;
    }
    sealed class Shore { public Vector3 position; public float pathLength; }
    static List<Suggestion> Fish(Limits limits,Vector3 spawn,Vector3 plan)
    {
        var shores=new List<Shore>(); var all=new List<Suggestion>();
        for (float radius=0;radius<=10.001f;radius+=.5f)
            foreach (Vector3 probe in Ring(plan,radius,.65f))
            {
                Vector3 stand; float length;
                if (!Reach(limits,spawn,probe,out stand,out length)) continue;
                bool duplicate=false;
                foreach (Shore old in shores) if (Horizontal(old.position,stand)<.4f) { duplicate=true; break; }
                if (!duplicate) shores.Add(new Shore { position=stand,pathLength=length });
            }
        Lake target=Water(limits,plan);
        for (float radius=0;target==null && radius<=6.001f;radius+=.25f)
            foreach (Vector3 probe in Ring(plan,radius,.5f))
            { target=Water(limits,probe); if (target!=null) break; }
        if (target==null) return all;
        // Every model candidate has its closest reachable bank, independently of
        // the search order. Retain candidates on all azimuths for joint spacing.
        for (float radius=0;radius<=6.001f;radius+=.25f)
            foreach (Vector3 probe in Ring(plan,radius,.45f))
            {
                Lake lake=Water(limits,probe); if (lake==null || lake.name!=target.name) continue;
                Vector3 water=new Vector3(probe.x,lake.level,probe.z); Shore best=null; float distance=4;
                foreach (Shore shore in shores)
                {
                    float d=Vector3.Distance(water,shore.position);
                    if (d<distance) { best=shore; distance=d; }
                }
                if (best!=null) all.Add(new Suggestion { modelPosition=water,standPosition=best.position,modelDisplacement=Horizontal(plan,water),standDistance=distance,standHorizontalDistance=Horizontal(water,best.position),pathLength=best.pathLength });
            }
        all.Sort(delegate(Suggestion a,Suggestion b)
        {
            int margin=(a.standDistance<3.5f ? 0 : 1).CompareTo(b.standDistance<3.5f ? 0 : 1);
            if (margin!=0) return margin;
            int offset=a.modelDisplacement.CompareTo(b.modelDisplacement); if (offset!=0) return offset;
            return a.standDistance.CompareTo(b.standDistance);
        });
        var result=new List<Suggestion>();
        // Limit same-point crowding so the output offers meaningful alternative
        // model sites, rather than repeated shore choices for one location.
        foreach (Suggestion item in all)
        {
            bool duplicate=false;
            foreach (Suggestion selected in result) if (Horizontal(selected.modelPosition,item.modelPosition)<.75f) { duplicate=true; break; }
            if (!duplicate) result.Add(item);
            if (result.Count>=32) break;
        }
        return result;
    }
    public static void Run()
    {
        string inputPath=Environment.GetEnvironmentVariable("HYJS_DESERT_AUDIT_INPUT");
        string priorPath=Environment.GetEnvironmentVariable("HYJS_DESERT_AUDIT_PRIOR");
        string outputPath=Environment.GetEnvironmentVariable("HYJS_DESERT_SUGGEST_OUTPUT");
        if (String.IsNullOrEmpty(inputPath) || String.IsNullOrEmpty(priorPath) || String.IsNullOrEmpty(outputPath)) throw new InvalidOperationException("Set input/prior/suggestion environment paths");
        if (File.Exists(outputPath)) throw new IOException("Suggestion output must be new");
        if (Application.unityVersion!="5.3.3p3") throw new InvalidOperationException("Required Unity 5.3.3p3");
        Input input=JsonUtility.FromJson<Input>(File.ReadAllText(inputPath)); Prior prior=JsonUtility.FromJson<Prior>(File.ReadAllText(priorPath));
        if (input==null || prior==null || input.points==null || prior.points==null || input.points.Length!=59 || prior.points.Length!=59 || prior.schema!="hyjs-desert-v4-point-audit-v1" || input.designSha256!=prior.designSha256 || input.minimapSha256!=prior.minimapSha256 || input.bundleSha256!=prior.bundleSha256) throw new InvalidOperationException("Audit provenance or point count mismatch");
        EditorSceneManager.OpenScene(ScenePath);
        Limits limits=JsonUtility.FromJson<Limits>(File.ReadAllText("Assets/DesertPreview/navigation-input.json"));
        NavMeshTriangulation tri=NavMesh.CalculateTriangulation();
        if (tri.vertices.Length!=4947 || tri.indices.Length/3!=2337) throw new InvalidOperationException("Unexpected NavMesh");
        NavMeshHit start;
        if (!NavMesh.SamplePosition(new Vector3(-7,.03814265f,-6),out start,1,-1)) throw new InvalidOperationException("Spawn unavailable");
        var failed=new Dictionary<string,bool>(); foreach (PriorPoint point in prior.points) if (!point.automatedChecksPass) failed.Add(point.id,true);
        var items=new List<Item>();
        foreach (InputPoint point in input.points)
        {
            // The first pass used horizontal distance for fish. All twelve need
            // fresh 3D checks even if their general navigation result was true.
            if (point.kind!="fish" || point.id=="F09") continue;
            Vector3 plan=new Vector3(point.worldX,0,point.worldZ);
            List<Suggestion> candidates=point.kind=="fish" ? Fish(limits,start.position,plan) : Ground(limits,start.position,plan);
            items.Add(new Item { id=point.id,kind=point.kind,originalPlan=plan,candidates=candidates.ToArray(),reason=candidates.Count==0 ? "No bounded reachable candidate; inspect area manually" : "Candidates require scene, interaction and model verification" });
        }
        var report=new Report { schema="hyjs-desert-v4-fish-options-v4",unityVersion=Application.unityVersion,scene=ScenePath,designSha256=input.designSha256,minimapSha256=input.minimapSha256,bundleSha256=input.bundleSha256,navVertices=tri.vertices.Length,navTriangles=tri.indices.Length/3,failedPointCount=failed.Count,points=items.ToArray(),manualGates=new string[] { "visual inspection of exact area", "prefer 3D under 3.5, always under 4, and verify original actor pivot", "actual resource/exit model bounds and player navigation", "same scene and bundle identity", "device verification", "server point-version approval" } };
        File.WriteAllText(outputPath,JsonUtility.ToJson(report,true));
        Debug.Log("HYJS_DESERT_V4_FISH_OPTIONS_WRITTEN " + outputPath + " points="+items.Count+"; no point eligible for release");
    }
}
