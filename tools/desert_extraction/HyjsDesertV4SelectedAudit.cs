using System;
using System.Collections.Generic;
using System.IO;
using UnityEditor.SceneManagement;
using UnityEngine;

// Read-only editor audit of candidate placements. Passing checks does not authorize release.
public static class HyjsDesertV4SelectedAudit
{
    const string Scene = "Assets/Scenes/map_desertfield_001.unity";
    [Serializable] public class Point
    {
        public string id,kind,source;
        public int pixelX,pixelY;
        public Vector3 modelPosition,standPosition;
        public bool eligibleForRelease;
    }
    [Serializable] public class Input
    {
        public string schema,designSha256,minimapSha256,bundleSha256;
        public bool eligibleForRelease;
        public Point[] points;
    }
    [Serializable] public class Lake { public string name; public Vector3[] points; public float level; }
    [Serializable] public class Limits { public Vector3[] outer; public Lake[] water; }
    [Serializable] public class Result
    {
        public string id,kind,source,originalWater,modelWater,reason;
        public int pixelX,pixelY;
        public Vector3 originalPlan,modelPosition,standPosition,navPosition;
        public float modelToStand3d,modelShiftHorizontal,standToNav3d,pathLength,nearestSameKindHorizontal;
        public bool insideMap,insideWaterAtModel,insideWaterAtStand,sameWater,modelAtWaterLevel;
        public bool standOnNav,fromSpawnComplete,staticCapsuleClear,rangeUnderFour,automatedChecksPass,eligibleForRelease=false;
    }
    [Serializable] public class Report
    {
        public string schema,unityVersion,scene,inputSchema,designSha256,minimapSha256,bundleSha256,bossRouteEvidenceSha256;
        public string[] releaseGates;
        public bool allEligibleForRelease=false,editorSceneIdentityRequiresIndependentVerification=true;
        public int navVertices,navTriangles,pointCount,automaticPassCount;
        public Vector3 spawn;
        public Result[] points;
    }
    static bool Inside(Vector3[] poly,Vector3 p)
    {
        bool inside=false;
        for (int i=0,j=poly.Length-1;i<poly.Length;j=i++)
        { Vector3 a=poly[i],b=poly[j]; if ((a.z>p.z)!=(b.z>p.z) && p.x<(b.x-a.x)*(p.z-a.z)/(b.z-a.z)+a.x) inside=!inside; }
        return inside;
    }
    static Lake Water(Limits limits,Vector3 p)
    { foreach (Lake lake in limits.water) if (Inside(lake.points,p)) return lake; return null; }
    static Lake NearbyOriginalWater(Limits limits,Vector3 p)
    {
        Lake lake=Water(limits,p);
        for (float r=.25f;lake==null && r<=6f;r+=.25f)
            for (int i=0;i<64;i++)
            {
                float a=i*Mathf.PI/32f;
                lake=Water(limits,p+new Vector3(Mathf.Cos(a)*r,0,Mathf.Sin(a)*r));
                if (lake!=null) break;
            }
        return lake;
    }
    static bool Finite(float v) { return !Single.IsNaN(v) && !Single.IsInfinity(v) && Mathf.Abs(v)<100000f; }
    static bool Finite(Vector3 p) { return Finite(p.x)&&Finite(p.y)&&Finite(p.z); }
    static float Horizontal(Vector3 a,Vector3 b) { return Vector2.Distance(new Vector2(a.x,a.z),new Vector2(b.x,b.z)); }
    static float PathLength(Vector3[] p)
    { float n=0; for (int i=1;i<p.Length;i++) n+=Vector3.Distance(p[i-1],p[i]); return n; }
    static void Why(Result r,string s) { if (!String.IsNullOrEmpty(r.reason)) r.reason += "; "; r.reason+=s; }
    public static void Run()
    {
        string inputPath=Environment.GetEnvironmentVariable("HYJS_DESERT_SELECTED_INPUT");
        string outputPath=Environment.GetEnvironmentVariable("HYJS_DESERT_SELECTED_OUTPUT");
        if (String.IsNullOrEmpty(inputPath)||String.IsNullOrEmpty(outputPath)||File.Exists(outputPath)) throw new InvalidOperationException("Set selected input and fresh output paths");
        if (Application.unityVersion!="5.3.3p3") throw new InvalidOperationException("Required Unity 5.3.3p3");
        Input input=JsonUtility.FromJson<Input>(File.ReadAllText(inputPath));
        if (input==null || input.schema!="hyjs-desert-v4-selected-audit-input-v1" || input.eligibleForRelease || input.points==null || input.points.Length!=59) throw new InvalidOperationException("Expected 59 non-release selected points");
        var ids=new HashSet<string>();
        foreach (Point p in input.points)
            if (p==null || String.IsNullOrEmpty(p.id) || !ids.Add(p.id) || p.eligibleForRelease || !Finite(p.modelPosition) || !Finite(p.standPosition)) throw new InvalidOperationException("Invalid point");
        EditorSceneManager.OpenScene(Scene);
        Limits limits=JsonUtility.FromJson<Limits>(File.ReadAllText("Assets/DesertPreview/navigation-input.json"));
        if (limits==null || limits.outer==null || limits.water==null) throw new InvalidOperationException("Missing navigation boundaries");
        NavMeshTriangulation tri=NavMesh.CalculateTriangulation();
        if (tri.vertices.Length!=4947 || tri.indices.Length/3!=2337) throw new InvalidOperationException("Unexpected NavMesh inventory");
        NavMeshHit spawn;
        if (!NavMesh.SamplePosition(new Vector3(-7,.03814265f,-6),out spawn,1f,-1)) throw new InvalidOperationException("Spawn not on NavMesh");
        var report=new Report { schema="hyjs-desert-v4-selected-audit-report-v1",unityVersion=Application.unityVersion,scene=Scene,inputSchema=input.schema,
            designSha256=input.designSha256,minimapSha256=input.minimapSha256,bundleSha256=input.bundleSha256,
            bossRouteEvidenceSha256="9324cd12fc03e4107255ea823371425c5d3f8c1a6a74b56d0f7dbe3a703fac5d",
            navVertices=tri.vertices.Length,navTriangles=tri.indices.Length/3,spawn=spawn.position,
            releaseGates=new string[]{"independently verify scene/input/bundle hashes", "visually verify same water body and shore", "verify original fish actor pivot, native range and animation on device",
            "verify bridge four moving monsters and real player collision", "verify boss four routes against referenced independent report and original device",
            "verify NPC positions/safe area", "authorize all server world coordinates"} };
        var results=new List<Result>();
        foreach (Point p in input.points)
        {
            var r=new Result { id=p.id,kind=p.kind,source=p.source,pixelX=p.pixelX,pixelY=p.pixelY,modelPosition=p.modelPosition,standPosition=p.standPosition };
            try
            {
                // Pixel coordinates are provenance only; this transform does not authorize placement.
                r.originalPlan=new Vector3((p.pixelX-256)/4.096f+30.4f,0,(256-p.pixelY)/4.096f+35.5f);
                r.modelShiftHorizontal=Horizontal(r.originalPlan,p.modelPosition);
                r.modelToStand3d=Vector3.Distance(p.modelPosition,p.standPosition);
                r.rangeUnderFour=r.modelToStand3d<4f;
                r.insideMap=Inside(limits.outer,p.modelPosition) && Inside(limits.outer,p.standPosition);
                Lake modelWater=Water(limits,p.modelPosition), standWater=Water(limits,p.standPosition), originalWater=NearbyOriginalWater(limits,r.originalPlan);
                r.originalWater=originalWater==null ? "" : originalWater.name; r.modelWater=modelWater==null ? "" : modelWater.name;
                r.insideWaterAtModel=modelWater!=null; r.insideWaterAtStand=standWater!=null;
                r.sameWater=originalWater!=null && modelWater!=null && originalWater.name==modelWater.name;
                r.modelAtWaterLevel=modelWater!=null && Mathf.Abs(p.modelPosition.y-modelWater.level)<.05f;
                NavMeshHit h;
                r.standOnNav=NavMesh.SamplePosition(p.standPosition,out h,.2f,-1);
                if (r.standOnNav)
                {
                    r.navPosition=h.position; r.standToNav3d=Vector3.Distance(h.position,p.standPosition); r.standOnNav=r.standToNav3d<=.2f;
                    var path=new NavMeshPath(); r.fromSpawnComplete=NavMesh.CalculatePath(spawn.position,h.position,-1,path) && path.status==NavMeshPathStatus.PathComplete;
                    if (r.fromSpawnComplete) r.pathLength=PathLength(path.corners);
                    r.staticCapsuleClear=!Physics.CheckCapsule(h.position+Vector3.up*.35f,h.position+Vector3.up*1.5f,.2f,1<<11);
                }
                r.nearestSameKindHorizontal=100000f;
                foreach (Point q in input.points) if (q.id!=p.id && q.kind==p.kind) r.nearestSameKindHorizontal=Mathf.Min(r.nearestSameKindHorizontal,Horizontal(q.modelPosition,p.modelPosition));
                bool fish=p.kind=="fish";
                if (!r.insideMap) Why(r,"outside reconstructed map boundary");
                if (!r.standOnNav) Why(r,"stand position not on NavMesh within 0.2");
                if (!r.fromSpawnComplete) Why(r,"spawn path incomplete");
                if (!r.staticCapsuleClear) Why(r,"scene-layer static capsule overlaps");
                if (fish)
                {
                    if (!r.sameWater) Why(r,"model differs from original nearby water body or body unresolved");
                    if (!r.modelAtWaterLevel) Why(r,"fish model is not at target water level");
                    if (r.insideWaterAtStand) Why(r,"fish stand lies within water polygon");
                    if (!r.rangeUnderFour) Why(r,"model to stand exceeds original 4-unit 3D limit");
                    if (r.nearestSameKindHorizontal<2.5f) Why(r,"fish spacing below 2.5");
                }
                if (p.kind=="boss") Why(r,"independent four-direction path evidence and original device required");
                if (p.kind=="safe") Why(r,"safe area and NPC geometry require review");
                if (p.kind=="mob" && (p.id=="M01"||p.id=="M02"||p.id=="M03"||p.id=="M04")) Why(r,"bridge patrol and actor clearance require review");
                r.automatedChecksPass=r.insideMap && r.standOnNav && r.fromSpawnComplete && r.staticCapsuleClear &&
                    (!fish || (r.sameWater && r.modelAtWaterLevel && !r.insideWaterAtStand && r.rangeUnderFour && r.nearestSameKindHorizontal>=2.5f));
                if (r.automatedChecksPass) report.automaticPassCount++;
            }
            catch (Exception e) { Why(r,e.GetType().Name+": "+e.Message); }
            results.Add(r);
        }
        report.points=results.ToArray(); report.pointCount=results.Count;
        File.WriteAllText(outputPath,JsonUtility.ToJson(report,true));
        Debug.Log("HYJS_DESERT_V4_SELECTED_AUDIT_WRITTEN "+outputPath+" automatic="+report.automaticPassCount+"/"+report.pointCount+"; no point approved");
    }
}
