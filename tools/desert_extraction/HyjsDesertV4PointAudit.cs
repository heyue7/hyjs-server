using System;
using System.Collections.Generic;
using System.IO;
using UnityEditor;
using UnityEditor.SceneManagement;
using UnityEngine;

// Editor-only measurement for the existing rebuilt 5.3 scene. It never modifies
// the scene or exports authoritative placements. Fish interaction range, model
// size, NPC/safe-area geometry and actual device play remain separate gates.
public static class HyjsDesertV4PointAudit
{
    const string ScenePath = "Assets/Scenes/map_desertfield_001.unity";
    [Serializable] public class Point { public string id, kind; public int pixelX, pixelY; public float worldX, worldZ; }
    [Serializable] public class Input { public string schema, designSha256, minimapSha256, bundleSha256, sourceTransform; public Point[] points; }
    [Serializable] public class Lake { public string name; public Vector3[] points; public float level; }
    [Serializable] public class Limits { public Vector3[] outer; public Lake[] water; }
    [Serializable] public class PointResult
    {
        public string id, kind, reason;
        public int pixelX, pixelY;
        public Vector3 plan, projected;
        public float horizontalShift, verticalShift, pathLength;
        public bool hasProjection, withinProjectionLimit, fromSpawnComplete, insideWaterAtPlan, insideWaterAtProjection;
        public bool staticCapsuleClear, automatedChecksPass, eligibleForRelease = false;
    }
    [Serializable] public class Report
    {
        public string schema, unityVersion, scene, inputSchema, designSha256, minimapSha256, bundleSha256, sourceTransform;
        public string[] releaseGates;
        public bool editorSceneIdentityRequiresIndependentVerification = true, allEligibleForRelease = false;
        public int navVertices, navTriangles, pointCount, automaticPassCount;
        public Vector3 spawn;
        public PointResult[] points;
    }

    static bool Inside(Vector3[] polygon, Vector3 p)
    {
        bool inside = false;
        for (int i = 0, j = polygon.Length - 1; i < polygon.Length; j = i++)
        {
            Vector3 a = polygon[i], b = polygon[j];
            if ((a.z > p.z) != (b.z > p.z) && p.x < (b.x-a.x)*(p.z-a.z)/(b.z-a.z)+a.x) inside = !inside;
        }
        return inside;
    }
    static Lake WaterAt(Limits limits, Vector3 p)
    {
        foreach (Lake lake in limits.water) if (Inside(lake.points, p)) return lake;
        return null;
    }
    static float SurfaceY(float x, float z)
    {
        RaycastHit[] hits = Physics.RaycastAll(new Vector3(x, 50, z), Vector3.down, 100);
        float top = -1000;
        foreach (RaycastHit hit in hits) if (hit.normal.y > .45f && hit.point.y > top) top = hit.point.y;
        if (top > -100) return top;
        Terrain t = Terrain.activeTerrain;
        if (t == null) throw new InvalidOperationException("No terrain or upward collision at " + x + "," + z);
        return t.SampleHeight(new Vector3(x,0,z)) + t.transform.position.y;
    }
    static float Horizontal(Vector3 a, Vector3 b)
    {
        return Vector2.Distance(new Vector2(a.x,a.z),new Vector2(b.x,b.z));
    }
    static float Length(Vector3[] points)
    {
        float total = 0;
        for (int i=1; i<points.Length; i++) total += Vector3.Distance(points[i-1],points[i]);
        return total;
    }
    static void AddReason(PointResult result, string text)
    {
        if (!String.IsNullOrEmpty(result.reason)) result.reason += "; ";
        result.reason += text;
    }
    public static void Run()
    {
        string inputPath = Environment.GetEnvironmentVariable("HYJS_DESERT_AUDIT_INPUT");
        string outputPath = Environment.GetEnvironmentVariable("HYJS_DESERT_AUDIT_OUTPUT");
        if (String.IsNullOrEmpty(inputPath) || String.IsNullOrEmpty(outputPath)) throw new InvalidOperationException("Set HYJS_DESERT_AUDIT_INPUT and HYJS_DESERT_AUDIT_OUTPUT");
        if (File.Exists(outputPath)) throw new IOException("Output must not already exist: " + outputPath);
        if (Application.unityVersion != "5.3.3p3") throw new InvalidOperationException("Required Unity 5.3.3p3, found " + Application.unityVersion);
        Input input = JsonUtility.FromJson<Input>(File.ReadAllText(inputPath));
        if (input == null || input.schema != "hyjs-desert-v4-candidate-audit-input-v1" || input.points == null || input.points.Length != 59) throw new InvalidOperationException("Expected exactly 59 V4 candidates");
        EditorSceneManager.OpenScene(ScenePath);
        Limits limits = JsonUtility.FromJson<Limits>(File.ReadAllText("Assets/DesertPreview/navigation-input.json"));
        if (limits == null || limits.outer == null || limits.water == null) throw new InvalidOperationException("Navigation boundary data missing");
        NavMeshTriangulation tri = NavMesh.CalculateTriangulation();
        if (tri.vertices.Length != 4947 || tri.indices.Length/3 != 2337) throw new InvalidOperationException("Unexpected navigation inventory: " + tri.vertices.Length + "/" + tri.indices.Length/3);
        NavMeshHit spawnHit;
        if (!NavMesh.SamplePosition(new Vector3(-7,.03814265f,-6), out spawnHit, 1f, -1)) throw new InvalidOperationException("Verified spawn not on NavMesh");
        var report = new Report();
        report.schema = "hyjs-desert-v4-point-audit-v1"; report.unityVersion=Application.unityVersion; report.scene=ScenePath;
        report.inputSchema=input.schema; report.designSha256=input.designSha256; report.minimapSha256=input.minimapSha256;
        report.bundleSha256=input.bundleSha256; report.sourceTransform=input.sourceTransform;
        report.navVertices=tri.vertices.Length; report.navTriangles=tri.indices.Length/3; report.spawn=spawnHit.position;
        report.releaseGates = new string[] { "verify exact scene and bundle identity", "inspect each projection and collision with original game actors", "fish: verify target water, shore reach and original interaction range", "boss: instantiate actual model and verify bounds and all four bypass paths", "bridge: inspect four individual mobs and walkable bypass", "confirm safe-area and NPC geometry", "verify all points on device and authorize server coordinates" };
        var results = new List<PointResult>();
        foreach (Point point in input.points)
        {
            var result = new PointResult(); result.id=point.id; result.kind=point.kind; result.pixelX=point.pixelX; result.pixelY=point.pixelY;
            try
            {
                Vector3 plan = new Vector3(point.worldX,0,point.worldZ); Lake water = WaterAt(limits,plan);
                result.insideWaterAtPlan=water!=null;
                plan.y = point.kind == "fish" && water != null ? water.level : SurfaceY(plan.x,plan.z); result.plan=plan;
                NavMeshHit hit;
                float sampleRadius = point.kind == "fish" ? 20f : 2f;
                if (!NavMesh.SamplePosition(plan, out hit, sampleRadius, -1)) { AddReason(result,"no NavMesh within sample radius"); results.Add(result); continue; }
                result.hasProjection=true; result.projected=hit.position;
                result.horizontalShift=Horizontal(plan,hit.position); result.verticalShift=Mathf.Abs(plan.y-hit.position.y);
                result.insideWaterAtProjection=WaterAt(limits,hit.position)!=null;
                result.withinProjectionLimit=result.horizontalShift <= (point.kind == "fish" ? 20f : 1.5f);
                var path = new NavMeshPath();
                result.fromSpawnComplete=NavMesh.CalculatePath(spawnHit.position,hit.position,-1,path) && path.status==NavMeshPathStatus.PathComplete;
                if (result.fromSpawnComplete) result.pathLength=Length(path.corners);
                // Checks the map's Scene layer only; this does not substitute for real model bounds.
                result.staticCapsuleClear=!Physics.CheckCapsule(hit.position+Vector3.up*.35f,hit.position+Vector3.up*1.5f,.20f,1<<11);
                if (!result.withinProjectionLimit) AddReason(result,"projection exceeds horizontal limit");
                if (!result.fromSpawnComplete) AddReason(result,"spawn path incomplete");
                if (!result.staticCapsuleClear) AddReason(result,"scene-layer capsule overlaps static collider");
                if (point.kind == "fish")
                {
                    if (!result.insideWaterAtPlan) AddReason(result,"fish marker not inside reconstructed water polygon");
                    if (result.insideWaterAtProjection) AddReason(result,"nearest nav sample still inside water polygon (possibly bridge)");
                    AddReason(result,"original fishing interaction range and actual shore must be verified");
                }
                if (point.kind == "boss") AddReason(result,"actual boss model bounds and four bypass routes required");
                if (point.kind == "safe") AddReason(result,"safe-area boundary and NPC placement require verification");
                if (point.kind == "mob" && (point.id=="M01" || point.id=="M02" || point.id=="M03" || point.id=="M04")) AddReason(result,"bridge clearance with actual mob required");
                result.automatedChecksPass=result.withinProjectionLimit && result.fromSpawnComplete && result.staticCapsuleClear && (point.kind!="fish" || (result.insideWaterAtPlan && !result.insideWaterAtProjection));
                if (result.automatedChecksPass) report.automaticPassCount++;
            }
            catch (Exception error) { AddReason(result,error.GetType().Name + ": " + error.Message); }
            results.Add(result);
        }
        report.points=results.ToArray(); report.pointCount=results.Count;
        File.WriteAllText(outputPath,JsonUtility.ToJson(report,true));
        Debug.Log("HYJS_DESERT_V4_POINT_AUDIT_WRITTEN " + outputPath + " automatic=" + report.automaticPassCount + "/" + report.pointCount + "; no point eligible for release yet");
    }
}
