using System;
using System.Collections.Generic;
using System.IO;
using UnityEditor.SceneManagement;
using UnityEngine;

// F01/F02 bounded search in each original water body. Proposals are not release points.
public static class HyjsDesertV4WestFish
{
    const string Scene="Assets/Scenes/map_desertfield_001.unity";
    [Serializable] public class InputPoint { public string id,kind; public float worldX,worldZ; }
    [Serializable] public class Input { public string designSha256,minimapSha256,bundleSha256; public InputPoint[] points; }
    [Serializable] public class Candidate { public Vector3 modelPosition,standPosition; public float standDistance,modelDisplacement; }
    [Serializable] public class SuggestedPoint { public string id,kind; public Candidate[] candidates; }
    [Serializable] public class SelectedPoint { public string id,kind; public Vector3 modelPosition; }
    [Serializable] public class Prior { public string schema,designSha256,minimapSha256,bundleSha256; public SelectedPoint[] points; }
    [Serializable] public class Lake { public string name; public Vector3[] points; public float level; }
    [Serializable] public class Limits { public Vector3[] outer; public Lake[] water; }
    [Serializable] public class Result { public Vector3 modelPosition,standPosition; public float modelDisplacement,standDistance,standHorizontalDistance,pathLength,nearestOtherFish; public bool eligibleForRelease=false; }
    [Serializable] public class Item { public string id,waterName; public int shoreSamples; public Result[] candidates; }
    [Serializable] public class Report { public string schema,unityVersion,scene,designSha256,minimapSha256,bundleSha256; public bool eligibleForRelease=false; public float maxDisplacement=20,preferred3dLimit=3.5f,preferredSpacing=2.5f; public int navVertices,navTriangles; public Item[] points; }
    static bool Inside(Vector3[] poly,Vector3 p)
    {
        bool inside=false;
        for (int i=0,j=poly.Length-1;i<poly.Length;j=i++)
        { Vector3 a=poly[i],b=poly[j]; if ((a.z>p.z)!=(b.z>p.z) && p.x<(b.x-a.x)*(p.z-a.z)/(b.z-a.z)+a.x) inside=!inside; }
        return inside;
    }
    static Lake Water(Limits limits,Vector3 p) { foreach (Lake l in limits.water) if (Inside(l.points,p)) return l; return null; }
    static float Horiz(Vector3 a,Vector3 b) { return Vector2.Distance(new Vector2(a.x,a.z),new Vector2(b.x,b.z)); }
    static float Surface(float x,float z)
    {
        float top=-1000;
        foreach (RaycastHit h in Physics.RaycastAll(new Vector3(x,50,z),Vector3.down,100)) if (h.normal.y>.45f && h.point.y>top) top=h.point.y;
        if (top>-100) return top;
        Terrain t=Terrain.activeTerrain; if (t==null) throw new InvalidOperationException("No ground");
        return t.SampleHeight(new Vector3(x,0,z))+t.transform.position.y;
    }
    static bool Reach(Limits limits,Vector3 spawn,Vector3 probe,out Vector3 stand,out float length)
    {
        stand=Vector3.zero; length=0;
        try
        {
            if (!Inside(limits.outer,probe) || Water(limits,probe)!=null) return false;
            probe.y=Surface(probe.x,probe.z); NavMeshHit h;
            if (!NavMesh.SamplePosition(probe,out h,.75f,-1) || Horiz(probe,h.position)>.75f || Water(limits,h.position)!=null) return false;
            if (Physics.CheckCapsule(h.position+Vector3.up*.35f,h.position+Vector3.up*1.5f,.2f,1<<11)) return false;
            var path=new NavMeshPath();
            if (!NavMesh.CalculatePath(spawn,h.position,-1,path) || path.status!=NavMeshPathStatus.PathComplete) return false;
            for (int i=1;i<path.corners.Length;i++) length+=Vector3.Distance(path.corners[i-1],path.corners[i]);
            stand=h.position; return true;
        }
        catch (Exception) { return false; }
    }
    static bool NearWater(Limits limits,Lake target,Vector3 p)
    {
        for (int i=0;i<16;i++)
        { float a=i*Mathf.PI/8; Lake l=Water(limits,p+new Vector3(Mathf.Cos(a)*3,0,Mathf.Sin(a)*3)); if (l!=null && l.name==target.name) return true; }
        return false;
    }
    static int Compare(Result a,Result b)
    {
        // Prefer modest displacement and separation; report all numbers to the reviewer.
        int ap=a.nearestOtherFish>=2.5f ? 0 : 1, bp=b.nearestOtherFish>=2.5f ? 0 : 1;
        if (ap!=bp) return ap.CompareTo(bp);
        int c=a.modelDisplacement.CompareTo(b.modelDisplacement); return c!=0 ? c : a.standDistance.CompareTo(b.standDistance);
    }
    public static void Run()
    {
        string inputPath=Environment.GetEnvironmentVariable("HYJS_DESERT_AUDIT_INPUT"), priorPath=Environment.GetEnvironmentVariable("HYJS_DESERT_SELECTED_PRIOR"), outputPath=Environment.GetEnvironmentVariable("HYJS_DESERT_WEST_FISH_OUTPUT");
        if (String.IsNullOrEmpty(inputPath)||String.IsNullOrEmpty(priorPath)||String.IsNullOrEmpty(outputPath)||File.Exists(outputPath)) throw new InvalidOperationException("Set fresh input/selected/output paths");
        if (Application.unityVersion!="5.3.3p3") throw new InvalidOperationException("Required Unity 5.3.3p3");
        Input input=JsonUtility.FromJson<Input>(File.ReadAllText(inputPath)); Prior prior=JsonUtility.FromJson<Prior>(File.ReadAllText(priorPath));
        if (input==null || prior==null || input.points==null || prior.points==null || input.points.Length!=59 || prior.points.Length!=59 || prior.schema!="hyjs-desert-v4-selected-audit-input-v1" || input.designSha256!=prior.designSha256 || input.minimapSha256!=prior.minimapSha256 || input.bundleSha256!=prior.bundleSha256) throw new InvalidOperationException("West fish provenance differs");
        var other=new List<Vector3>();
        foreach (SelectedPoint p in prior.points) if (p.kind=="fish" && p.id!="F01" && p.id!="F02") other.Add(p.modelPosition);
        if (other.Count!=10) throw new InvalidOperationException("Expected ten other provisional fish locations");
        EditorSceneManager.OpenScene(Scene); Limits limits=JsonUtility.FromJson<Limits>(File.ReadAllText("Assets/DesertPreview/navigation-input.json"));
        NavMeshTriangulation tri=NavMesh.CalculateTriangulation(); if (tri.vertices.Length!=4947||tri.indices.Length/3!=2337) throw new InvalidOperationException("Unexpected NavMesh");
        NavMeshHit spawn; if (!NavMesh.SamplePosition(new Vector3(-7,.03814265f,-6),out spawn,1,-1)) throw new InvalidOperationException("Spawn unavailable");
        var points=new List<Item>();
        foreach (string id in new string[]{"F01","F02"})
        {
            InputPoint targetPoint=null; foreach (InputPoint p in input.points) if (p.id==id && p.kind=="fish") targetPoint=p;
            if (targetPoint==null) throw new InvalidOperationException("Target fish missing: "+id);
            Vector3 plan=new Vector3(targetPoint.worldX,0,targetPoint.worldZ); Lake target=Water(limits,plan);
            // Use only the same lake that contains or adjoins the original marker.
            for (float r=.25f;target==null && r<=6;r+=.25f)
                for (int i=0;i<64;i++) { float a=i*Mathf.PI/32; target=Water(limits,plan+new Vector3(Mathf.Cos(a)*r,0,Mathf.Sin(a)*r)); if (target!=null) break; }
            if (target==null) throw new InvalidOperationException("Original water body unresolved: "+id);
            var all=new List<Result>(); int sampled=0;
            for (float x=plan.x-23.5f;x<=plan.x+23.5f;x+=.5f)
                for (float z=plan.z-23.5f;z<=plan.z+23.5f;z+=.5f)
                {
                    Vector3 probe=new Vector3(x,0,z);
                    if (Water(limits,probe)!=null || !NearWater(limits,target,probe)) continue;
                    Vector3 stand; float path;
                    if (!Reach(limits,spawn.position,probe,out stand,out path)) continue;
                    sampled++; float dy=Mathf.Abs(stand.y-target.level);
                    if (dy>=3.5f) continue;
                    float max=Mathf.Sqrt(3.5f*3.5f-dy*dy); Result best=null;
                    for (float dx=-max;dx<=max;dx+=.25f)
                        for (float dz=-max;dz<=max;dz+=.25f)
                        {
                            Vector3 water=new Vector3(stand.x+dx,target.level,stand.z+dz);
                            Lake lake=Water(limits,water); float distance=Vector3.Distance(water,stand), shift=Horiz(plan,water);
                            if (lake==null || lake.name!=target.name || distance>=3.5f || shift>20) continue;
                            float nearest=10000; foreach (Vector3 p in other) nearest=Mathf.Min(nearest,Horiz(p,water));
                            Result candidate=new Result { modelPosition=water,standPosition=stand,modelDisplacement=shift,standDistance=distance,standHorizontalDistance=Horiz(water,stand),pathLength=path,nearestOtherFish=nearest };
                            if (best==null || Compare(candidate,best)<0) best=candidate;
                        }
                    if (best!=null) all.Add(best);
                }
            all.Sort(Compare); var selected=new List<Result>();
            foreach (Result c in all)
            {
                bool duplicate=false; foreach (Result old in selected) if (Horiz(old.modelPosition,c.modelPosition)<.75f) { duplicate=true; break; }
                if (!duplicate) selected.Add(c); if (selected.Count>=32) break;
            }
            points.Add(new Item { id=id,waterName=target.name,shoreSamples=sampled,candidates=selected.ToArray() });
        }
        var report=new Report { schema="hyjs-desert-v4-west-fish-v5",unityVersion=Application.unityVersion,scene=Scene,designSha256=input.designSha256,minimapSha256=input.minimapSha256,bundleSha256=input.bundleSha256,navVertices=tri.vertices.Length,navTriangles=tri.indices.Length/3,points=points.ToArray() };
        File.WriteAllText(outputPath,JsonUtility.ToJson(report,true));
        Debug.Log("HYJS_DESERT_V4_WEST_FISH_WRITTEN " + outputPath + "; no point eligible for release");
    }
}
