using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using LitJson;
using Room;

namespace UnityEngine
{
    public class Object { public static void DontDestroyOnLoad(Object value) { } }
    public class MonoBehaviour : Object { }
    public sealed class GameObject : Object
    {
        public GameObject(string name) { }
        public T AddComponent<T>() where T : new() { return new T(); }
    }
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
public class UIName3D : UnityEngine.Object { public string text; }
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
        public static bool Current(out Session value) { value=current; return value!=null; }
    }
    public static class FieldRuntime { public static int SceneVersion=1; }
    public static class PositionSync { public static int ConnectionEpoch=1; }
}
public static class ChipNamesChecks
{
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
    static void Tick()
    {
        Type t=typeof(HYJS.DesertV4.ChipNames);
        object value=t.GetField("instance",BindingFlags.Static|BindingFlags.NonPublic).GetValue(null);
        t.GetMethod("LateUpdate",BindingFlags.Instance|BindingFlags.NonPublic).Invoke(value,null);
    }
    static void Check(bool value,string message) { if (!value) throw new Exception(message); }
    public static void Main()
    {
        RoomManager.currentRoom=new RoomBase();
        UIName3D self=new UIName3D { text="自己" }, peer=new UIName3D { text="另一位" };
        RoomManager.currentRoom.charactorManager.actors["1"]=new RoomCharactor { ui=new RCUIPlayer { label=self } };
        RoomManager.currentRoom.charactorManager.actors["2"]=new RoomCharactor { ui=new RCUIPlayer { label=peer } };
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(1,Item(1,10),Item(2,7))); Tick(); Tick();
        Check(self.text=="自己  芯片 10" && peer.text=="另一位  芯片 7","self/peer native labels or duplicate suffix");
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(1,Item(1,8),Item(2,9))); Tick();
        Check(self.text=="自己  芯片 8" && peer.text=="另一位  芯片 9","committed count update");
        peer.text="改名"; Tick(); Check(peer.text=="改名  芯片 9","native name update");
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,8))); Tick();
        Check(self.text=="自己  芯片 8" && peer.text=="改名","line change removes old peer");
        NetAPI.connected=false; Tick(); Check(self.text=="自己" && peer.text=="改名","disconnect restores labels");
        NetAPI.connected=true; HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,8))); Tick();
        HYJS.DesertV4.PositionSync.ConnectionEpoch++; Tick(); Check(self.text=="自己","new socket clears previous state");
        HYJS.DesertV4.ChipNames.ApplySnapshot(Snapshot(2,Item(1,3))); Tick();
        HYJS.DesertV4.ChipNames.ReleaseScene(); Check(self.text=="自己","leave restores original name");
        Console.WriteLine("ChipNames: same-line self/peer, updates, native rename, line, disconnect, socket and leave passed");
    }
}
