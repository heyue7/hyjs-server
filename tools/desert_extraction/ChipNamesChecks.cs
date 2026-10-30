using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using LitJson;
using Room;
using UnityEngine;

// Small Unity doubles exercise the actual source; they do not render glyphs.
namespace UnityEngine
{
    public class Object
    {
        public bool destroyed;
        public static void DontDestroyOnLoad(Object value) { }
        public static void Destroy(Object value)
        {
            if (ReferenceEquals(value,null) || value.destroyed) return;
            GameObject go=value as GameObject;
            if (go!=null)
            {
                foreach (GameObject child in GameObject.all.ToArray())
                    if (!child.destroyed && child.transform.parent==go.transform) Destroy(child);
                foreach (Component component in go.components) component.destroyed=true;
            }
            value.destroyed=true;
        }
        public static bool operator ==(Object a,Object b)
        {
            bool an=ReferenceEquals(a,null) || a.destroyed, bn=ReferenceEquals(b,null) || b.destroyed;
            return an || bn ? an && bn : ReferenceEquals(a,b);
        }
        public static bool operator !=(Object a,Object b) { return !(a==b); }
        public override bool Equals(object other) { return this==(other as Object); }
        public override int GetHashCode() { return base.GetHashCode(); }
    }
    public class Component : Object
    {
        public GameObject gameObject;
        public Transform transform { get { return gameObject.transform; } }
        public T GetComponent<T>() where T : class { return gameObject.GetComponent<T>(); }
    }
    public class MonoBehaviour : Component { }
    public sealed class GameObject : Object
    {
        public static readonly List<GameObject> all=new List<GameObject>();
        public readonly List<Component> components=new List<Component>();
        public string name; public int layer; public bool activeSelf=true;
        public Transform transform;
        public GameObject(string name)
        {
            this.name=name; transform=new Transform(); transform.gameObject=this;
            components.Add(transform); all.Add(this);
        }
        public bool activeInHierarchy
        { get { return !destroyed && activeSelf && (transform.parent==null || transform.parent.gameObject.activeInHierarchy); } }
        public void SetActive(bool value) { activeSelf=value; }
        public T AddComponent<T>() where T : Component,new()
        {
            T value=new T(); value.gameObject=this; components.Add(value);
            if (value is TextMesh && GetComponent<Renderer>()==null) AddComponent<MeshRenderer>();
            return value;
        }
        public T GetComponent<T>() where T : class
        {
            foreach (Component value in components) if (!value.destroyed && value is T) return value as T;
            return null;
        }
    }
    public struct Vector3
    {
        public float x,y,z;
        public Vector3(float x,float y,float z) { this.x=x; this.y=y; this.z=z; }
        public static Vector3 zero { get { return new Vector3(0,0,0); } }
        public static Vector3 one { get { return new Vector3(1,1,1); } }
        public static Vector3 right { get { return new Vector3(1,0,0); } }
        public static Vector3 up { get { return new Vector3(0,1,0); } }
        public static float Dot(Vector3 a,Vector3 b) { return a.x*b.x+a.y*b.y+a.z*b.z; }
        public static Vector3 operator +(Vector3 a,Vector3 b) { return new Vector3(a.x+b.x,a.y+b.y,a.z+b.z); }
        public static Vector3 operator -(Vector3 a,Vector3 b) { return new Vector3(a.x-b.x,a.y-b.y,a.z-b.z); }
        public static Vector3 operator *(Vector3 a,float b) { return new Vector3(a.x*b,a.y*b,a.z*b); }
    }
    public struct Quaternion { public static Quaternion identity { get { return new Quaternion(); } } }
    public struct Bounds
    {
        public Vector3 center,size;
        public Bounds(Vector3 center,Vector3 size) { this.center=center; this.size=size; }
        public Vector3 max { get { return center+size*0.5f; } }
    }
    public sealed class Transform : Component
    {
        public Transform parent;
        public Vector3 localPosition,localScale,origin,axisX=Vector3.right,axisY=Vector3.up,axisZ=new Vector3(0,0,1);
        public Quaternion localRotation;
        public Vector3 TransformVector(Vector3 v) { return axisX*v.x+axisY*v.y+axisZ*v.z; }
        public Vector3 TransformPoint(Vector3 v) { return origin+TransformVector(v); }
        public Vector3 InverseTransformPoint(Vector3 v)
        {
            v=v-origin;
            return new Vector3(Vector3.Dot(v,axisX)/Vector3.Dot(axisX,axisX),
                Vector3.Dot(v,axisY)/Vector3.Dot(axisY,axisY),Vector3.Dot(v,axisZ)/Vector3.Dot(axisZ,axisZ));
        }
    }
    public static class Mathf { public static float Abs(float value) { return Math.Abs(value); } }
    public struct Color
    {
        public int value;
        public Color(int value) { this.value=value; }
        public static bool operator ==(Color a,Color b) { return a.value==b.value; }
        public static bool operator !=(Color a,Color b) { return !(a==b); }
        public override bool Equals(object value) { return value is Color && this==(Color)value; }
        public override int GetHashCode() { return value; }
    }
    public sealed class Font : Object { }
    public sealed class Material : Object { }
    public enum FontStyle { Normal,Bold,Italic,BoldAndItalic }
    public enum TextAnchor { UpperLeft,UpperCenter,UpperRight,MiddleLeft,MiddleCenter,MiddleRight,LowerLeft,LowerCenter,LowerRight }
    public enum TextAlignment { Left,Center,Right }
    public sealed class TextMesh : Component
    {
        public string text=""; public Font font; public int fontSize; public FontStyle fontStyle;
        public float characterSize=1,lineSpacing=1,tabSize=4,offsetZ; public bool richText;
        public Color color; public TextAnchor anchor; public TextAlignment alignment;
        public float Width()
        {
            float width=0;
            foreach (char c in text) width+=c==' ' ? 0.4f : c<128 ? 0.6f : 1f;
            return width*characterSize;
        }
    }
    public class Renderer : Component
    {
        public bool enabled=true; public Material sharedMaterial; public int sortingLayerID,sortingOrder;
        public Bounds bounds
        {
            get
            {
                TextMesh text=GetComponent<TextMesh>(); float w=text.Width(), h=text.characterSize;
                Vector3 x=transform.axisX, y=transform.axisY;
                Vector3 size=new Vector3(Math.Abs(x.x)*w+Math.Abs(y.x)*h,
                    Math.Abs(x.y)*w+Math.Abs(y.y)*h,Math.Abs(x.z)*w+Math.Abs(y.z)*h);
                return new Bounds(transform.TransformPoint(new Vector3(0,h*0.5f,0)),size);
            }
        }
    }
    public sealed class MeshRenderer : Renderer { }
}
namespace LitJson
{
    public sealed class JsonData : IEnumerable<JsonData>
    {
        readonly object value;
        public JsonData(object value) { this.value=value; }
        public JsonData this[string key] { get { return ((Dictionary<string,JsonData>)value)[key]; } }
        public static explicit operator string(JsonData value) { return (string)value.value; }
        public override string ToString() { return value.ToString(); }
        public IEnumerator<JsonData> GetEnumerator() { return ((List<JsonData>)value).GetEnumerator(); }
        IEnumerator IEnumerable.GetEnumerator() { return GetEnumerator(); }
    }
}
public class UIName3D : MonoBehaviour
{
    public TextMesh text0,text1,text2,text3,text4;
    public string text
    {
        get { return text0.text; }
        set { text0.text=value; text1.text=value; text2.text=value; text3.text=value; text4.text=value; }
    }
}
public static class StaticData { public static string fieldId="field_003"; }
public static class NetAPI { public static bool connected=true; public static bool SocketIsConnected() { return connected; } }
namespace Room
{
    public sealed class RCUIPlayer { public UIName3D label; }
    public sealed class RoomCharactor { public object ui; }
    public sealed class Characters
    {
        public readonly Dictionary<string,RoomCharactor> actors=new Dictionary<string,RoomCharactor>();
        public RoomCharactor GetSelfCharactor() { return GetCharactor("1"); }
        public RoomCharactor GetCharactor(string id) { RoomCharactor actor; return actors.TryGetValue(id,out actor) ? actor : null; }
    }
    public sealed class RoomBase { public Characters charactorManager=new Characters(); }
    public static class RoomManager { public static RoomBase currentRoom; }
}
namespace HYJS.DesertV4
{
    public sealed class FieldWire
    {
        public sealed class Session { public long playerId; public string sessionHex,origin; }
        public static Session current=new Session { playerId=1,sessionHex="session-a",origin="local" };
        public static bool Current(out Session value)
        {
            value=current==null ? null : new Session { playerId=current.playerId,sessionHex=current.sessionHex,origin=current.origin };
            return value!=null;
        }
    }
    public static class FieldRuntime { public static int SceneVersion=1; }
    public static class PositionSync { public static int ConnectionEpoch=1; }
}
public static class ChipNamesChecks
{
    static int assertions;
    static JsonData Item(long id,long count)
    {
        return new JsonData(new Dictionary<string,JsonData> {
            {"playerId",new JsonData(id)}, {"count",new JsonData(count)} });
    }
    static JsonData Snapshot(long line,params JsonData[] entries)
    {
        return new JsonData(new Dictionary<string,JsonData> {
            {"scene",new JsonData("field_003")}, {"line",new JsonData(line)},
            {"playerChipCounts",new JsonData(new List<JsonData>(entries))} });
    }
    static object Instance()
    { return typeof(HYJS.DesertV4.ChipNames).GetField("instance",BindingFlags.Static|BindingFlags.NonPublic).GetValue(null); }
    static void Invoke(string method)
    { typeof(HYJS.DesertV4.ChipNames).GetMethod(method,BindingFlags.Instance|BindingFlags.NonPublic).Invoke(Instance(),null); }
    static void Tick() { Invoke("LateUpdate"); }
    static void Check(bool value,string message) { assertions++; if (!value) throw new Exception(message); }
    static void Near(float a,float b,string message) { Check(Math.Abs(a-b)<0.002f,message+": "+a+" / "+b); }
    static TextMesh[] Native(UIName3D label)
    { return new TextMesh[] { label.text0,label.text1,label.text2,label.text3,label.text4 }; }
    static UIName3D Make(string name)
    {
        UIName3D label=new GameObject("native-label").AddComponent<UIName3D>();
        TextMesh[] text=new TextMesh[5]; Font font=new Font(); Material material=new Material();
        for (int i=0;i<5;i++)
        {
            GameObject go=new GameObject("native-"+i); go.transform.parent=label.transform; go.layer=7;
            text[i]=go.AddComponent<TextMesh>(); text[i].font=font; text[i].fontSize=18;
            text[i].fontStyle=FontStyle.Bold; text[i].anchor=TextAnchor.LowerCenter;
            text[i].alignment=TextAlignment.Left; text[i].offsetZ=i==0 ? -0.01f : 0;
            text[i].color=new Color(i==0 ? 0x123456 : 0x654321);
            Renderer renderer=go.GetComponent<Renderer>(); renderer.sharedMaterial=material;
            renderer.sortingLayerID=2; renderer.sortingOrder=3+i;
        }
        label.text0=text[0]; label.text1=text[1]; label.text2=text[2]; label.text3=text[3]; label.text4=text[4];
        label.text=name; SetView(label,37,21,0.9f,0.95f,new Vector3(2,3,4)); return label;
    }
    static void SetView(UIName3D label,double yawDegrees,double pitchDegrees,float sx,float sy,Vector3 origin)
    {
        double yaw=yawDegrees*Math.PI/180,pitch=pitchDegrees*Math.PI/180;
        foreach (TextMesh text in Native(label))
        {
            Transform t=text.transform;
            t.axisX=new Vector3((float)Math.Cos(yaw)*sx,0,(float)-Math.Sin(yaw)*sx);
            t.axisY=new Vector3((float)(Math.Sin(yaw)*Math.Sin(pitch))*sy,(float)Math.Cos(pitch)*sy,
                (float)(Math.Cos(yaw)*Math.Sin(pitch))*sy);
            t.axisZ=new Vector3((float)(Math.Sin(yaw)*Math.Cos(pitch)),(float)-Math.Sin(pitch),
                (float)(Math.Cos(yaw)*Math.Cos(pitch)));
            t.origin=origin;
        }
    }
    static void AddActor(long id,UIName3D label)
    { RoomManager.currentRoom.charactorManager.actors[id.ToString()]=new RoomCharactor { ui=new RCUIPlayer { label=label } }; }
    static GameObject Output(TextMesh source,int index)
    {
        GameObject found=null;
        foreach (GameObject go in GameObject.all)
            if (!go.destroyed && go.name=="HYJSDesertChipName"+index && go.transform.parent==source.transform)
            { Check(found==null,"duplicate chip layer"); found=go; }
        return found;
    }
    static int LiveOutputs()
    {
        int count=0;
        foreach (GameObject go in GameObject.all)
            if (!go.destroyed && go.name.StartsWith("HYJSDesertChipName") && go.name!="HYJSDesertChipNames") count++;
        return count;
    }
    static void ExpectLayers(UIName3D label,string text)
    {
        TextMesh[] native=Native(label);
        for (int i=0;i<native.Length;i++)
        {
            GameObject go=Output(native[i],i); Check(go!=null && go.activeInHierarchy,"visible layer "+i);
            TextMesh copy=go.GetComponent<TextMesh>(); Renderer r=go.GetComponent<Renderer>(), n=native[i].GetComponent<Renderer>();
            Check(copy.text==text && copy.font==native[i].font && copy.fontSize==native[i].fontSize &&
                copy.fontStyle==native[i].fontStyle && copy.characterSize==native[i].characterSize &&
                copy.color==native[i].color && copy.offsetZ==native[i].offsetZ,"native font/style layer "+i);
            Check(copy.anchor==TextAnchor.LowerLeft && copy.alignment==TextAlignment.Left,"independent left anchor "+i);
            Check(r.sharedMaterial==n.sharedMaterial && r.sortingLayerID==n.sortingLayerID &&
                r.sortingOrder==n.sortingOrder && go.layer==native[i].gameObject.layer,"native renderer layer "+i);
            Near(go.transform.localPosition.x,label.text0.Width()*0.5f+label.text0.characterSize*0.2f,"name right edge "+i);
            Near(go.transform.localPosition.y,0,"native baseline "+i);
        }
    }
    static void CheckAbi()
    {
        Type type=typeof(HYJS.DesertV4.ChipNames);
        MethodInfo[] methods=type.GetMethods(BindingFlags.Public|BindingFlags.Static|BindingFlags.DeclaredOnly);
        Check(type.IsSealed && type.BaseType==typeof(MonoBehaviour) && methods.Length==2,"public helper ABI");
        MethodInfo apply=type.GetMethod("ApplySnapshot"), release=type.GetMethod("ReleaseScene");
        Check(apply.ReturnType==typeof(void) && apply.GetParameters().Length==1 &&
            apply.GetParameters()[0].ParameterType==typeof(JsonData) && release.ReturnType==typeof(void) &&
            release.GetParameters().Length==0,"public method signatures");
    }
    public static void Main()
    {
        CheckAbi(); RoomManager.currentRoom=new RoomBase();
        UIName3D self=Make("自己"), peer=Make("另一位"); AddActor(1,self); AddActor(2,peer);
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(1,Item(1,10),Item(2,7))); Tick(); Tick();
        Check(self.text=="自己" && peer.text=="另一位" && self.text0.anchor==TextAnchor.LowerCenter,
            "self and peer names remain centred and unchanged");
        ExpectLayers(self,"芯片 10"); ExpectLayers(peer,"芯片 7"); Check(LiveOutputs()==10,"five layers per actor");
        float initialX=Output(self.text0,0).transform.localPosition.x;
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(1,Item(1,123456),Item(2,0))); Tick();
        ExpectLayers(self,"芯片 123456"); ExpectLayers(peer,"芯片 0");
        Near(Output(self.text0,0).transform.localPosition.x,initialX,"count width cannot shift name or anchor");
        peer.text="改了一个更长的名字"; Tick(); Check(peer.text=="改了一个更长的名字","rename remains unchanged");
        ExpectLayers(peer,"芯片 0");
        SetView(peer,-63,12,1.7f,0.6f,new Vector3(-13,27,50)); Tick();
        ExpectLayers(peer,"芯片 0"); Check(Output(peer.text0,0).transform.parent==peer.text0.transform,"moving billboard retains parent");
        peer.text0.color=new Color(99); peer.text0.fontSize=23; peer.text0.characterSize=1.4f;
        peer.text0.GetComponent<Renderer>().sortingOrder=11; Tick();
        Check(Output(peer.text0,0).GetComponent<TextMesh>().color==peer.text0.color &&
            Output(peer.text0,0).GetComponent<TextMesh>().fontSize==23 &&
            Output(peer.text0,0).GetComponent<Renderer>().sortingOrder==11,"dynamic native style copied");
        Near(Output(peer.text0,0).transform.localPosition.x,peer.text0.Width()*0.5f+0.28f,"name size changes position");
        peer.gameObject.SetActive(false); Tick(); Check(!Output(peer.text0,0).activeSelf,"hidden label hides chips");
        peer.gameObject.SetActive(true); Tick(); Check(Output(peer.text0,0).activeInHierarchy,"visible label restores chips");
        peer.text0.gameObject.SetActive(false); Tick(); Check(!Output(peer.text1,1).activeSelf,"hidden base text hides all layers");
        peer.text0.gameObject.SetActive(true); Tick(); Check(Output(peer.text0,0).activeInHierarchy,"visible base text restores chips");
        peer.text0.GetComponent<Renderer>().enabled=false; Tick(); Check(!Output(peer.text0,0).activeSelf,"hidden native renderer hides chips");
        peer.text0.GetComponent<Renderer>().enabled=true; Tick(); Check(Output(peer.text0,0).activeInHierarchy,"native renderer restores chips");
        peer.text=""; Tick(); Check(!Output(peer.text0,0).activeSelf,"empty native name hides chips");
        peer.text="归来"; Tick(); Check(Output(peer.text0,0).activeInHierarchy && peer.text=="归来","name return restores chips");
        peer.text0.transform.axisY=peer.text0.transform.axisX; Tick();
        Check(!Output(peer.text0,0).activeSelf,"degenerate projection hides chips");
        SetView(peer,28,19,1.4f,0.7f,new Vector3(5,6,7)); Tick();
        Check(Output(peer.text0,0).activeInHierarchy,"valid projection restores chips");
        UnityEngine.Object.Destroy(Output(peer.text2,2)); Tick();
        Check(LiveOutputs()==10 && Output(peer.text2,2)!=null && Output(peer.text1,1)!=null,
            "partial layer destruction rebuilds one complete set");
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(1,Item(1,8))); Tick();
        Check(LiveOutputs()==5 && Output(peer.text0,0)==null,"snapshot removes departed peer");
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(1,Item(1,8),Item(2,3))); Tick();
        Check(LiveOutputs()==10 && Output(peer.text0,0)!=null,"peer rejoins");
        RoomManager.currentRoom.charactorManager.actors.Remove("2"); Tick();
        Check(LiveOutputs()==5 && Output(peer.text0,0)==null,"actor disappears before snapshot");
        UIName3D replacement=Make("新角色"); AddActor(2,replacement); Tick(); ExpectLayers(replacement,"芯片 3");
        UIName3D newer=Make("更换名牌"); AddActor(2,newer); Tick();
        Check(Output(replacement.text0,0)==null && LiveOutputs()==10,"label replacement cleans old objects");
        ExpectLayers(newer,"芯片 3");
        TextMesh previous=newer.text1; newer.text1=Make("临时").text1; newer.text1.text=newer.text;
        Tick(); Check(Output(previous,1)==null && Output(newer.text1,1)!=null && LiveOutputs()==10,"native layer replacement");
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,8))); Tick();
        Check(LiveOutputs()==5 && Output(newer.text0,0)==null,"line switch releases old line");
        NetAPI.connected=false; Tick(); Check(LiveOutputs()==0 && self.text=="自己","disconnect releases layers");
        NetAPI.connected=true; HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,8))); Tick();
        HYJS.DesertV4.PositionSync.ConnectionEpoch++; Tick(); Check(LiveOutputs()==0,"socket epoch releases layers");
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,8))); Tick();
        HYJS.DesertV4.FieldRuntime.SceneVersion++; Tick(); Check(LiveOutputs()==0,"scene version releases layers");
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,8))); Tick();
        HYJS.DesertV4.FieldWire.current.sessionHex="changed"; Tick(); Check(LiveOutputs()==0,"session change releases layers");
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,8))); Tick();
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,1),Item(1,2))); Check(LiveOutputs()==0,"invalid snapshot releases layers");
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,-1))); Check(LiveOutputs()==0,"negative count rejected");
        for (int i=0;i<20;i++)
        {
            HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,i))); Tick();
            Check(LiveOutputs()==5,"single live set after re-entry");
            HYJS.DesertV4.ChipNames.ReleaseScene(); Check(LiveOutputs()==0 && self.text=="自己","leave destroys chip layers");
        }
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,9))); Tick(); Invoke("OnDestroy");
        Check(LiveOutputs()==0 && Instance()==null,"component destruction releases scene");
        Console.WriteLine("PASS "+assertions+" actual-source assertions: centred names, native layers, placement, style and lifecycle (Unity doubles)");
    }
}
