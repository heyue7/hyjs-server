"""Exercise the actual sprout helper with small Unity/network doubles; not a device test."""
import argparse
import hashlib
import shutil
import subprocess
import tempfile
import zipfile
from pathlib import Path

SOURCE = Path(__file__).resolve().parent
ROOT = SOURCE.parents[1]
parser = argparse.ArgumentParser()
parser.add_argument("--tool-root", type=Path, default=Path("/Users/lirui/LR/hyjs"))
parser.add_argument("--base-apk", type=Path, default=ROOT / "build/activity-growth77/hyjs-1.0.77-candidate-r8.apk")
args = parser.parse_args()
dotnet = args.tool_root / ".tools/native-batch/dotnet/dotnet"
with args.base_apk.open("rb") as base:
    if hashlib.file_digest(base, "sha256").hexdigest() != "c970012d7c785c58c3b923ea4a0ea8e650204ed0e50e16e914f47e54ac4aece8":
        raise ValueError("Base APK identity differs")

stubs = r'''
using System;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Text.Json;
namespace UnityEngine {
 public class Object { public static void DontDestroyOnLoad(Object o){} public static void Destroy(Object o){} }
 public class Component:Object {
  public GameObject gameObject; public Transform transform { get { return gameObject.transform; } }
  public string name { get { return gameObject.name; } }
  public T GetComponent<T>() where T:class { return gameObject.GetComponent<T>(); }
 }
 public class MonoBehaviour:Component {
  public object StartCoroutine(IEnumerator e) { Scheduler.running.Add(e); Scheduler.Pump(); return null; }
 }
 public static class Scheduler {
  public static List<IEnumerator> running=new List<IEnumerator>();
  public static void Pump() { foreach(var e in running.ToArray()) if(!e.MoveNext()) running.Remove(e); }
 }
 public class GameObject:Object {
  public static List<GameObject> all=new List<GameObject>();
  public string name; public int layer; public bool activeInHierarchy=true; public Transform transform;
  readonly List<Component> components=new List<Component>();
  public GameObject(string n){name=n;transform=new Transform();transform.gameObject=this;all.Add(this);}
  public T AddComponent<T>() where T:Component,new(){T c=new T();c.gameObject=this;components.Add(c);return c;}
  public T GetComponent<T>() where T:class { return components.OfType<T>().FirstOrDefault(); }
  public void SetActive(bool active){activeInHierarchy=active;}
 }
 public struct Vector2 { public float x,y; public Vector2(float x,float y){this.x=x;this.y=y;} }
 public struct Vector3 {
  public float x,y,z; public Vector3(float x,float y,float z){this.x=x;this.y=y;this.z=z;}
  public static Vector3 zero {get{return new Vector3(0,0,0);}} public static Vector3 one {get{return new Vector3(1,1,1);}}
  public static Vector3 right {get{return new Vector3(1,0,0);}} public static Vector3 up {get{return new Vector3(0,1,0);}}
  public static float Dot(Vector3 a,Vector3 b){return a.x*b.x+a.y*b.y+a.z*b.z;}
  public static Vector3 operator +(Vector3 a,Vector3 b){return new Vector3(a.x+b.x,a.y+b.y,a.z+b.z);}
  public static Vector3 operator -(Vector3 a,Vector3 b){return new Vector3(a.x-b.x,a.y-b.y,a.z-b.z);}
  public static Vector3 operator *(Vector3 a,float b){return new Vector3(a.x*b,a.y*b,a.z*b);}
 }
 public struct Quaternion { public static Quaternion identity {get{return new Quaternion();}} }
 public struct Bounds {
  public Vector3 center,size; public Bounds(Vector3 c,Vector3 s){center=c;size=s;}
  public Vector3 min {get{return center-size*0.5f;}}
 }
 public class Transform:Component {
  public Transform parent; public Vector3 localPosition,localScale; public Quaternion localRotation;
  public Vector3 axisX=Vector3.right,axisY=Vector3.up,axisZ=new Vector3(0,0,1),origin=Vector3.zero;
  public Vector3 TransformVector(Vector3 v){return axisX*v.x+axisY*v.y+axisZ*v.z;}
  public Vector3 TransformPoint(Vector3 p){return origin+TransformVector(p);}
  public Vector3 InverseTransformPoint(Vector3 p){p=p-origin;return new Vector3(Vector3.Dot(p,axisX)/Vector3.Dot(axisX,axisX),Vector3.Dot(p,axisY)/Vector3.Dot(axisY,axisY),Vector3.Dot(p,axisZ)/Vector3.Dot(axisZ,axisZ));}
 }
 public static class Mathf {public static float Abs(float x){return Math.Abs(x);}}
 public struct Color {public static Color white {get{return new Color();}}}
 public class Mesh:Object { public Vector3[] vertices;public Vector2[] uv;public Color[] colors;public int[] triangles;public void RecalculateBounds(){} }
 public class MeshFilter:Component {public Mesh mesh,sharedMesh;}
 public class Material:Object {
  public string shader; public Texture2D mainTexture; public Material(string s){shader=s;} public Material(Material m){shader=m.shader;}
 }
 public class Renderer:Component {public bool enabled=true;public Material sharedMaterial;public Bounds bounds;public int sortingLayerID,sortingOrder;
  public Material material {get{return sharedMaterial;}set{sharedMaterial=value;}}
 }
 public class MeshRenderer:Renderer {}
 public class TextMesh:Component {public string text;}
 public enum TextureFormat {ARGB32}
 public enum FilterMode {Bilinear}
 public class Texture2D:Object {public int width=24,height=24;public FilterMode filterMode;public Texture2D(int w,int h,TextureFormat f,bool mip){}public bool LoadImage(byte[] b){return b.Length>0;}}
 public static class Time {public static float realtimeSinceStartup;}
 public class WWW:IDisposable {
  public class Result {public bool done;public string text,error;}
  public static Queue<Result> replies=new Queue<Result>();public static List<WWW> requests=new List<WWW>();
  public Result reply;public string body;public bool disposed;
  public WWW(string url,byte[] data,Dictionary<string,string> headers){body=Encoding.UTF8.GetString(data);reply=replies.Dequeue();requests.Add(this);}
  public bool isDone {get{return reply.done;}}public string text {get{return reply.text;}}public string error {get{return reply.error;}}
  public void Dispose(){disposed=true;}
 }
}
public class UIName3D:UnityEngine.MonoBehaviour {public UnityEngine.TextMesh text0;public QuadTitle title;}
public class QuadTitle:UnityEngine.Component {}
public class RoomCharactor:UnityEngine.Component {public int type;}
public class RCUIPlayer {public RoomCharactor owner;public UIName3D label;}
public static class StaticData {public static Player playerData=new Player();public static byte[] sessionKey=new byte[16];}
public class Player {public long playerId=1;}
public static class ServerConfig {public static string GetServerUrl(){return "https://example.invalid/sao/";}}
namespace Dict.DataModel {public class TableCopyExp {public int level,exp;}}
'''

tests = r'''
using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using System.Text.Json;
using UnityEngine;
using HYJS.Sprout73;
class Test {
 static int count;static void Assert(bool b,string label){if(!b)throw new Exception(label);count++;}
 static void Invoke(object o,string method){o.GetType().GetMethod(method,BindingFlags.Instance|BindingFlags.NonPublic).Invoke(o,null);}
 static Hub CurrentHub(){return (Hub)typeof(Runtime).GetField("hub",BindingFlags.Static|BindingFlags.NonPublic).GetValue(null);}
 static long Now(){return (DateTime.UtcNow.Ticks-621355968000000000L)/10000;}
 static WWW.Result Good(params long[] ids){long now=Now();return new WWW.Result{done=true,text="{\"code\":0,\"data\":{\"icon\":\"ggo_sprout\",\"serverTime\":"+now+",\"players\":["+String.Join(",",ids.Select(id=>"{\"playerId\":"+id+",\"active\":true,\"expiresAt\":"+(now+60000)+"}"))+ "]}}"};}
 static WWW.Result OwnStatus(bool active,long delta){long now=Now();return new WWW.Result{done=true,text="{\"code\":0,\"data\":{\"icon\":\"ggo_sprout\",\"serverTime\":"+now+",\"players\":[{\"playerId\":1,\"active\":"+(active?"true":"false")+",\"expiresAt\":"+(active?now+delta:0)+"}]}}"};}
 static WWW.Result Denied(){return new WWW.Result{done=true,text="{\"code\":1}"};}
 static List<long> IDs(WWW r){using(var doc=JsonDocument.Parse(r.body))return doc.RootElement.GetProperty("playerIds").EnumerateArray().Select(x=>x.GetInt64()).ToList();}
 static RCUIPlayer Make(long id,int type){
  var actor=new GameObject(id.ToString()).AddComponent<RoomCharactor>();actor.type=type;
  var label=new GameObject("label").AddComponent<UIName3D>();var text=new GameObject("text0");label.text0=text.AddComponent<TextMesh>();label.text0.text="原姓名";
  var tr=text.AddComponent<MeshRenderer>();tr.sharedMaterial=new Material("font");tr.sortingLayerID=2;tr.sortingOrder=7;
  var title=new GameObject("title");label.title=title.AddComponent<QuadTitle>();title.AddComponent<MeshRenderer>().sharedMaterial=new Material("original transparent colored");
  var ui=new RCUIPlayer{owner=actor,label=label};SetBounds(ui,37,21);Runtime.Attach(ui);return ui;
 }
 static void SetBounds(RCUIPlayer ui,double yawDegrees,double pitchDegrees){
  var t=ui.label.text0.transform;double yaw=yawDegrees*Math.PI/180,pitch=pitchDegrees*Math.PI/180;
  t.axisX=new Vector3((float)Math.Cos(yaw)*0.9f,0,(float)-Math.Sin(yaw)*0.9f);
  t.axisY=new Vector3((float)(Math.Sin(yaw)*Math.Sin(pitch))*0.95f,(float)Math.Cos(pitch)*0.95f,(float)(Math.Cos(yaw)*Math.Sin(pitch))*0.95f);
  t.axisZ=new Vector3((float)(Math.Sin(yaw)*Math.Cos(pitch)),(float)-Math.Sin(pitch),(float)(Math.Cos(yaw)*Math.Cos(pitch)));
  Vector3 x=t.axisX,y=t.axisY;Vector3 s=new Vector3(Math.Abs(x.x)*3+Math.Abs(y.x),Math.Abs(x.y)*3+Math.Abs(y.y),Math.Abs(x.z)*3+Math.Abs(y.z));
  ui.label.text0.GetComponent<Renderer>().bounds=new Bounds(t.TransformPoint(Vector3.zero),s);
 }
 static void Tick(Hub h){Time.realtimeSinceStartup+=1.1f;Invoke(h,"Update");}
 static void Main(){
  var self=Make(1,1);var hub=CurrentHub();Assert(self.label.text0.GetComponent<MeshFilter>()==null,"original TextMesh must lack MeshFilter");
  WWW.replies.Enqueue(Good(1));Tick(hub);Assert(IDs(WWW.requests.Last()).SequenceEqual(new long[]{1}),"owner request");
  Runtime render=self.label.GetComponent<Runtime>();Invoke(render,"LateUpdate");
  var icon=GameObject.all.LastOrDefault(x=>x.name=="HYJSSproutNameIcon");Assert(icon!=null&&icon.activeInHierarchy,"active owner produced icon");
  var mesh=icon.GetComponent<MeshFilter>().mesh;Assert(Math.Abs(mesh.vertices[1].x+1.65f)<0.01f&&Math.Abs(mesh.vertices[2].y-0.5f)<0.01f,"position relative to rotated/scaled text bounds");
  var material=icon.GetComponent<Renderer>();Assert(material.sharedMaterial.shader=="original transparent colored"&&material.sortingLayerID==2&&material.sortingOrder==7,"icon preserves color and original sorting");
  Assert(self.label.text0.text=="原姓名"&&self.label.title.GetComponent<Renderer>().sharedMaterial.shader=="original transparent colored","name and title preserved");
  SetBounds(self,-63,12);Invoke(render,"LateUpdate");Assert(Math.Abs(mesh.vertices[1].x+1.65f)<0.02f,"rotating camera retains local alignment");
  Time.realtimeSinceStartup+=31;WWW.replies.Enqueue(OwnStatus(false,0));Tick(hub);Invoke(render,"LateUpdate");Assert(!icon.activeInHierarchy,"disabled status hides icon");
  Time.realtimeSinceStartup+=31;WWW.replies.Enqueue(OwnStatus(true,-1000));Tick(hub);Invoke(render,"LateUpdate");Assert(!icon.activeInHierarchy,"expired status hides icon");
  Time.realtimeSinceStartup+=31;WWW.replies.Enqueue(Good(1));Tick(hub);Invoke(render,"LateUpdate");Assert(icon.activeInHierarchy,"active status restores icon");
  var native=self.label.text0.GetComponent<Renderer>();native.enabled=false;Invoke(render,"LateUpdate");Assert(!icon.activeInHierarchy,"disabled native renderer hides icon");native.enabled=true;Invoke(render,"LateUpdate");
  var peer=Make(2,2);int previousRequests=WWW.requests.Count;WWW.replies.Enqueue(Denied());WWW.replies.Enqueue(Good(1));Tick(hub);
  Assert(IDs(WWW.requests[previousRequests]).SequenceEqual(new long[]{1,2})&&IDs(WWW.requests[previousRequests+1]).SequenceEqual(new long[]{1}),"one owner-only fallback after peer denied");
  Assert(hub.Active(1)&&!hub.Active(2),"denied peer hidden and owner retained");
  Runtime.Detach(peer);WWW.replies.Enqueue(Good(1));Tick(hub);Assert(IDs(WWW.requests.Last()).SequenceEqual(new long[]{1}),"roster change retries before 30 seconds");
  Runtime.Detach(self);var extras=new List<RCUIPlayer>();for(int i=2;i<=102;i++)extras.Add(Make(i,2));Runtime.Attach(self);
  var pending=Denied();pending.done=false;WWW.replies.Enqueue(pending);Tick(hub);var full=IDs(WWW.requests.Last());
  Assert(full.Count==100&&full.Contains(1),"full roster prioritizes owner");
  foreach(var p in extras)Runtime.Detach(p);pending.text=Good(1).text;pending.done=true;Scheduler.Pump();Assert(!hub.Active(1)&&!hub.Active(2),"stale roster response ignored even before next update");
  WWW.replies.Enqueue(Good(1));Tick(hub);Assert(IDs(WWW.requests.Last()).SequenceEqual(new long[]{1}),"changed roster re-queries immediately after pending request");
  Invoke(render,"LateUpdate");icon=GameObject.all.Last(x=>x.name=="HYJSSproutNameIcon");Assert(icon.activeInHierarchy,"pooled label renders after reattach");
  StaticData.sessionKey[0]=1;Tick(hub);Invoke(render,"LateUpdate");Assert(!icon.activeInHierarchy,"session change clears cached status");
  Console.WriteLine("PASS "+count+" actual-source assertions (Unity/network doubles; device and GPU not tested)");
 }
}
'''

with tempfile.TemporaryDirectory(prefix="hyjs-sprout-logic-") as temp:
    work = Path(temp)
    shutil.copy2(SOURCE / "Runtime.cs", work / "Runtime.cs")
    (work / "Stubs.cs").write_text(stubs)
    (work / "Test.cs").write_text(tests)
    with zipfile.ZipFile(args.base_apk) as apk:
        (work / "LitJson.dll").write_bytes(apk.read("assets/bin/Data/Managed/LitJson.dll"))
    image = ROOT / "tools/ggo_items/assets/ggo_sprout.png"
    (work / "Cases.csproj").write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework><GenerateAssemblyInfo>false</GenerateAssemblyInfo><ImplicitUsings>disable</ImplicitUsings><Nullable>disable</Nullable></PropertyGroup><ItemGroup><EmbeddedResource Include="' + str(image) + '" LogicalName="ggo_sprout.png" /><Reference Include="LitJson"><HintPath>' + str(work / "LitJson.dll") + '</HintPath></Reference></ItemGroup></Project>')
    result = subprocess.run(["rtk", "proxy", str(dotnet), "run", "--project", str(work / "Cases.csproj"), "-p:NuGetAudit=false"], capture_output=True, text=True, timeout=180)
    print(result.stdout + result.stderr)
    raise SystemExit(result.returncode)
