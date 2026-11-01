using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using HYJS.CopyBoxSync;
using NetWork.Layer;
using NetWork.Packets;
using ProtoBuf.Message;
using Room;
using UI;

namespace UnityEngine
{
    public class Object { public static void DontDestroyOnLoad(object value) { } }
    public class MonoBehaviour : Object
    {
        public object StartCoroutine(IEnumerator value) { Scheduler.Start(value); return value; }
    }
    public class GameObject
    {
        public bool activeSelf;
        public GameObject(string name) { }
        public void SetActive(bool value) { activeSelf = value; }
        public T AddComponent<T>() where T : new() { return new T(); }
    }
    public static class Time { public static float realtimeSinceStartup; }
    public static class Debug { public static void LogWarning(string value) { } }
}

public static class Scheduler
{
    public static readonly List<IEnumerator> Work = new List<IEnumerator>();
    public static void Start(IEnumerator value) { if (value.MoveNext()) Work.Add(value); }
    public static void Step(float seconds)
    {
        UnityEngine.Time.realtimeSinceStartup += seconds;
        IEnumerator[] old = Work.ToArray(); Work.Clear();
        foreach (IEnumerator value in old) if (value.MoveNext()) Work.Add(value);
    }
}

namespace NetWork.Packets { public class Packet { public int nOpCode; public object kBody; } }
namespace NetWork.Layer
{
    public delegate void HttpHandler(Packet packet, bool success);
    public class HTTPManager
    {
        private readonly Queue<object> sendDatas = new Queue<object>();
        private readonly Queue<object> handlers = new Queue<object>();
        private readonly Queue<object> receivedDatas = new Queue<object>();
        private readonly Queue<object> results = new Queue<object>();
        private readonly Queue<object> externalDatas = new Queue<object>();
        public bool sending;
        public void QueueResult() { results.Enqueue(new object()); }
        public void DrainResult() { results.Dequeue(); }
    }
}
public static class BoxManager
{
    public static UnityEngine.GameObject netMask;
    public static UnityEngine.GameObject netBox;
    public static void Reset() { netMask = null; netBox = null; }
    public static void ClearNetMask() { if (netMask != null) netMask.SetActive(false); netBox = null; }
    public static void CreateNetMask() { ClearNetMask(); if (netMask == null) netMask = new UnityEngine.GameObject("mask"); netMask.SetActive(true); }
}
public static class NetAPI
{
    private static HTTPManager http = new HTTPManager();
    public static HttpHandler Pending;
    public static int Opcode;
    public static int Attempts;
    public static bool RejectNext;
    public static int RejectCount;
    public static Action<int, object> DuringSend;
    public static bool IsSendingHttp() { return http.sending; }
    public static void Reset() { http = new HTTPManager(); Pending = null; Opcode = 0; Attempts = 0; RejectNext = false; RejectCount = 0; DuringSend = null; }
    public static void QueueResult() { http.QueueResult(); }
    public static void DrainResult() { http.DrainResult(); }
    public static bool SendHttp(int opcode, object body, HttpHandler callback)
    {
        Runtime.ObserveSend(opcode, body); Attempts++;
        Action<int, object> nested = DuringSend;
        if (nested != null) { DuringSend = null; nested(opcode, body); }
        if (http.sending) return false;
        BoxManager.CreateNetMask();
        if (RejectNext) { RejectNext = false; return false; }
        if (RejectCount > 0) { RejectCount--; return false; }
        http.sending = true; Pending = callback; Opcode = opcode; return true;
    }
    public static HttpHandler Deliver(object body, bool success)
    {
        HttpHandler callback = Pending; int opcode = Opcode;
        if (callback == null) throw new Exception("No pending request");
        Pending = null; Opcode = 0; http.sending = false; BoxManager.ClearNetMask();
        int response = opcode == 20105 ? 20106 : opcode == 20103 ? 20104 : opcode == 20109 ? 20110 : opcode == 20107 ? 20108 : opcode + 1;
        callback(new Packet { nOpCode = response, kBody = body }, success);
        return callback;
    }
}
namespace Room
{
    public class RoomCopy { }
    public static class RoomManager { public static object currentRoom; }
}
public static class StaticData { public static UIPlayerData playerData; public static byte[] sessionKey; public static string UserName; }
namespace Dict.DataModel
{
    public class TableFrag { public string id; }
    public class TableItem { public string id; }
}
namespace Dict.Blo
{
    public static class DictFragBlo { public static Dict.DataModel.TableFrag GetTableFrag(string id) { return new Dict.DataModel.TableFrag { id = id }; } }
    public static class DictItemBlo { public static Dict.DataModel.TableItem GetTableItem(string id) { return new Dict.DataModel.TableItem { id = id }; } }
}
namespace ProtoBuf.Message
{
    public class BaseResource { public string id, guid; public int num; }
    public class SimpleItem { public string id; public int num; }
    public class Warrior { public string id, guid; }
    public class Equipment { public string equipId, guid; }
    public class SCCopyBox { public List<BaseResource> currentResource = new List<BaseResource>(); }
    public class SCCopyLeave { }
    public class CSWarriorList { }
    public class CSEquipmentTreasureList { }
    public class CSFragmentList { }
    public class CSItemList { }
    public class SCWarriorList { public List<Warrior> warriorList = new List<Warrior>(); }
    public class SCEquipmentTreasureList { public List<Equipment> equipmentList = new List<Equipment>(); }
    public class SCItemList { public List<SimpleItem> itemList = new List<SimpleItem>(); }
    public class SCFragmentList
    {
        public List<SimpleItem> fragWarriorList = new List<SimpleItem>();
        public List<SimpleItem> fragEquipmentList = new List<SimpleItem>();
        public List<SimpleItem> fragPetList = new List<SimpleItem>();
        public List<SimpleItem> fragTreasureList = new List<SimpleItem>();
    }
}
namespace UI
{
    public class UIHero { public string heroId, guid; public object owner; }
    public class UIEquipment { public string equipId, guid; public object owner; }
    public class UIFragItem { public long num; public UIFragItem(Dict.DataModel.TableFrag value) { } }
    public class UISimpleItem { public long num; public UISimpleItem(Dict.DataModel.TableItem value) { } }
    public class UITreasureFrag { public Dictionary<string, UIFragItem> frags = new Dictionary<string, UIFragItem>(); }
    public class UIInstPlayerData
    {
        public Dictionary<string, UIHero> heroes = new Dictionary<string, UIHero>();
        public Dictionary<string, UIEquipment> equipment = new Dictionary<string, UIEquipment>();
        public void AddHero(Warrior item) { heroes[item.guid] = new UIHero { heroId = item.id, guid = item.guid }; }
        public void AddEquipment(Equipment item) { equipment[item.guid] = new UIEquipment { equipId = item.equipId, guid = item.guid }; }
    }
    public class UIPlayerData
    {
        public long playerId;
        public UIInstPlayerData InstPlayerData = new UIInstPlayerData();
        public Dictionary<string, UISimpleItem> SimpleGoods = new Dictionary<string, UISimpleItem>();
        public Dictionary<string, UIFragItem> FragHeros = new Dictionary<string, UIFragItem>();
        public Dictionary<string, UIFragItem> FragEquipments = new Dictionary<string, UIFragItem>();
        public Dictionary<string, UIFragItem> FragPets = new Dictionary<string, UIFragItem>();
        public UITreasureFrag treasureFrag = new UITreasureFrag();
        public Action DuringUpdate;
        public UIHero GetUIHero(string guid) { UIHero item; InstPlayerData.heroes.TryGetValue(guid, out item); return item; }
        public UIEquipment GetUIEquipment(string guid) { UIEquipment item; InstPlayerData.equipment.TryGetValue(guid, out item); return item; }
        public void UpdateSimpleItem(List<SimpleItem> rows)
        {
            foreach (SimpleItem item in rows)
            {
                if (item.id.StartsWith("item_")) SimpleGoods[item.id].num = item.num;
                else
                {
                    Dictionary<string, UIFragItem> list = item.id.StartsWith("frag_hero_") ? FragHeros :
                        item.id.StartsWith("frag_equip_") ? FragEquipments : item.id.StartsWith("frag_pet_") ? FragPets : treasureFrag.frags;
                    if (!list.ContainsKey(item.id)) list[item.id] = new UIFragItem(new Dict.DataModel.TableFrag());
                    list[item.id].num = item.num;
                }
            }
            Action action = DuringUpdate; DuringUpdate = null; if (action != null) action();
        }
    }
}

public static class ModelTests
{
    private static int checks;
    private static int nativeOpen, nativeLeave;
    private static readonly BindingFlags All = BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Static | BindingFlags.Instance;
    private static void Check(bool condition, string reason) { checks++; if (!condition) throw new Exception(reason); }
    private static void Reset()
    {
        typeof(Runtime).GetField("active", All).SetValue(null, null);
        typeof(Runtime).GetField("host", All).SetValue(null, null);
        Scheduler.Work.Clear(); UnityEngine.Time.realtimeSinceStartup = 0;
        NetAPI.Reset(); BoxManager.Reset(); StaticData.playerData = new UIPlayerData { playerId = 7 }; StaticData.UserName = "account-a";
        StaticData.sessionKey = new byte[] { 1, 2, 3, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0 }; RoomManager.currentRoom = new RoomCopy();
        nativeOpen = nativeLeave = 0;
    }
    private static BaseResource Resource(string id, string guid) { return new BaseResource { id = id, guid = guid, num = 1 }; }
    private static void Open(params BaseResource[] items)
    {
        HttpHandler callback = Runtime.WrapOpen(delegate(Packet packet, bool ok) { nativeOpen++; });
        SCCopyBox reply = new SCCopyBox(); reply.currentResource.AddRange(items);
        callback(new Packet { nOpCode = 54020, kBody = reply }, true);
    }
    private static void Leave(bool success)
    {
        HttpHandler callback = Runtime.WrapLeave(delegate(Packet packet, bool ok) { nativeLeave++; }, RoomManager.currentRoom);
        callback(new Packet { nOpCode = 54014, kBody = new SCCopyLeave() }, success);
    }
    private static void Wait(int opcode)
    {
        for (int i = 0; i < 1000 && NetAPI.Pending == null; i++) Scheduler.Step(.1f);
        Check(NetAPI.Pending != null && NetAPI.Opcode == opcode, "expected list " + opcode + " got " + NetAPI.Opcode);
    }
    private static SCWarriorList Warriors(params Warrior[] rows) { SCWarriorList r = new SCWarriorList(); r.warriorList.AddRange(rows); return r; }
    private static SCEquipmentTreasureList Equip(params Equipment[] rows) { SCEquipmentTreasureList r = new SCEquipmentTreasureList(); r.equipmentList.AddRange(rows); return r; }
    private static SCFragmentList Fragments(string id, int count)
    { SCFragmentList r = new SCFragmentList(); r.fragWarriorList.Add(new SimpleItem { id = id, num = count }); return r; }
    private static void SetVersion(ulong value)
    {
        object state = typeof(Runtime).GetField("active", All).GetValue(null);
        state.GetType().GetField("Version", All).SetValue(state, value);
    }
    private static void Progress(float seconds) { for (int i = 0; i < 10; i++) Scheduler.Step(seconds / 10); }

    // Counterexamples, not the shipping algorithm: identity-based receipt
    // dedupe cannot correlate a reward with an unversioned absolute snapshot.
    private sealed class DeltaModel
    {
        internal int Value;
        internal bool Remembered;
        internal void Delta(int amount) { if (!Remembered) { Value += amount; Remembered = true; } }
        internal void Snapshot(int value) { Value = value; }
    }
    private static void DeltaCounterexamples()
    {
        // The snapshot already includes this chest; no receipt has been seen.
        DeltaModel included = new DeltaModel(); included.Snapshot(12); included.Delta(2);
        Check(included.Value == 14, "included snapshot must expose duplicate increment");
        // A still-in-flight earlier snapshot erases the increment.
        DeltaModel old = new DeltaModel { Value = 10 }; old.Delta(2); old.Snapshot(10);
        Check(old.Value == 10, "older absolute snapshot must expose dropped increment");
        // Crash after value update but before a separate dedupe write, then replay.
        DeltaModel crashAfter = new DeltaModel { Value = 10 }; crashAfter.Value += 2;
        crashAfter.Delta(2); Check(crashAfter.Value == 14, "crash after apply must expose duplicate");
        // Crash after dedupe persistence and before cache update, then replay.
        DeltaModel crashBefore = new DeltaModel { Value = 10, Remembered = true }; crashBefore.Delta(2);
        Check(crashBefore.Value == 10, "crash before apply must expose loss");
        // A normal duplicate receipt alone can be deduped; the other examples
        // demonstrate why this is insufficient to correlate the snapshot.
        DeltaModel replay = new DeltaModel { Value = 10 }; replay.Delta(2); replay.Delta(2);
        Check(replay.Value == 12, "receipt dedupe model failed");
    }

    public static int Main()
    {
        Reset();
        object heroOwner = new object(), equipOwner = new object();
        UIHero existingHero = new UIHero { guid = "h-old", heroId = "hero_001", owner = heroOwner };
        UIEquipment existingEquip = new UIEquipment { guid = "e-old", equipId = "equip_001", owner = equipOwner };
        StaticData.playerData.InstPlayerData.heroes.Add("h-old", existingHero);
        StaticData.playerData.InstPlayerData.equipment.Add("e-old", existingEquip);
        StaticData.playerData.FragHeros.Add("frag_hero_001", new UIFragItem(new Dict.DataModel.TableFrag()) { num = 2 });
        Open(Resource("hero_001", "h-old"), Resource("hero_002", "h-new"), Resource("equip_001", "e-old"),
             Resource("equip_002", "e-new"), Resource("frag_hero_001", ""), Resource("item_005", ""), Resource("coin", ""));
        Open(Resource("hero_002", "h-new"));
        Check(nativeOpen == 2 && Scheduler.Work.Count == 0, "chest callback or before-exit behavior");
        object state = typeof(Runtime).GetField("active", All).GetValue(null);
        IList groups = (IList)state.GetType().GetField("Groups", All).GetValue(state);
        object group = groups[0];
        IDictionary fragments = (IDictionary)group.GetType().GetField("Fragments", All).GetValue(group);
        IDictionary items = (IDictionary)group.GetType().GetField("Items", All).GetValue(group);
        Check(fragments.Contains("frag_hero_001") && items.Contains("item_005"), "stack identity was not captured");
        Leave(true); Check(nativeLeave == 1, "native leave callback");
        Wait(20105); HttpHandler old = NetAPI.Deliver(Warriors(new Warrior { id = "hero_001", guid = "h-old" }, new Warrior { id = "hero_002", guid = "h-new" }), true);
        old(new Packet { nOpCode = 20106, kBody = Warriors(new Warrior { id = "hero_002", guid = "h-new" }) }, true);
        Wait(20103); NetAPI.Deliver(Equip(new Equipment { equipId = "equip_001", guid = "e-old" }, new Equipment { equipId = "equip_002", guid = "e-new" }), true);
        Progress(1);
        Check(Object.ReferenceEquals(existingHero, StaticData.playerData.GetUIHero("h-old")) && Object.ReferenceEquals(existingHero.owner, heroOwner), "existing hero reference changed");
        Check(Object.ReferenceEquals(existingEquip, StaticData.playerData.GetUIEquipment("e-old")) && Object.ReferenceEquals(existingEquip.owner, equipOwner), "existing equipment reference changed");
        Check(StaticData.playerData.GetUIHero("h-new") != null && StaticData.playerData.GetUIEquipment("e-new") != null, "current instances not merged");
        Check(StaticData.playerData.FragHeros["frag_hero_001"].num == 2 && !StaticData.playerData.SimpleGoods.ContainsKey("item_005"), "disabled stack path mutated inventory");
        Check(Scheduler.Work.Count == 0 && NetAPI.Pending == null, "finished work did not release");
        StaticData.playerData.InstPlayerData.heroes.Remove("h-new");
        old(new Packet { nOpCode = 20106, kBody = Warriors(new Warrior { id = "hero_002", guid = "h-new" }) }, true);
        Check(StaticData.playerData.GetUIHero("h-new") == null, "duplicate callback revived consumed hero");

        Reset(); Open(Resource("frag_hero_001", ""), Resource("item_005", "")); Leave(true); Progress(10);
        Check(NetAPI.Attempts == 0 && Scheduler.Work.Count == 0, "stack-only reward started unversioned sync");
        DeltaCounterexamples();

        Reset(); Open(Resource("hero_002", "h-new")); Leave(false); Progress(2);
        Check(nativeLeave == 1 && NetAPI.Pending == null, "failed leave started synchronization");
        Leave(true); Wait(20105); NetAPI.Deliver(Warriors(), true); Progress(1);
        Check(StaticData.playerData.GetUIHero("h-new") == null, "missing server instance resurrected");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true); NetAPI.QueueResult(); Progress(1);
        Check(NetAPI.Pending == null, "queued native callback was bypassed"); NetAPI.DrainResult();
        NetAPI.RejectNext = true; Progress(1); Check(NetAPI.Pending == null, "send=false was treated as accepted");
        Check(BoxManager.netMask == null || !BoxManager.netMask.activeSelf, "failed own send left its mask active");
        Wait(20105); NetAPI.Deliver(Warriors(), false); Wait(20105);
        NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true); Progress(1);
        Check(StaticData.playerData.GetUIHero("h-new") != null, "failed requests did not retry");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true);
        BoxManager.netBox = new UnityEngine.GameObject("other timeout");
        Progress(5); Check(NetAPI.Attempts == 0 && BoxManager.netBox != null, "existing native timeout box was replaced");
        BoxManager.netBox = null; BoxManager.netMask = new UnityEngine.GameObject("other wait"); BoxManager.netMask.SetActive(true);
        Progress(5); Check(NetAPI.Attempts == 0 && BoxManager.netMask.activeSelf, "existing native wait mask was replaced");
        BoxManager.netMask.SetActive(false); Wait(20105); NetAPI.Deliver(Warriors(), true); Progress(1);

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true); NetAPI.RejectCount = 10; Progress(40);
        Check(NetAPI.Attempts == 3 && Scheduler.Work.Count == 0 && NetAPI.Pending == null, "failed retries were not bounded");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true); Wait(20105);
        UIHero replacement = new UIHero { guid = "h-new", heroId = "hero_002", owner = heroOwner };
        StaticData.playerData.InstPlayerData.heroes["h-new"] = replacement;
        NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true);
        Check(Object.ReferenceEquals(replacement, StaticData.playerData.GetUIHero("h-new")), "changed local object overwritten");
        Wait(20105); NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true); Progress(1);
        Check(Object.ReferenceEquals(replacement, StaticData.playerData.GetUIHero("h-new")), "changed object not preserved after retry");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true); Wait(20105);
        Check(!NetAPI.SendHttp(9999, new object(), delegate(Packet p, bool ok) { }), "normal busy request changed its result");
        StaticData.playerData.InstPlayerData.heroes["h-new"] = new UIHero { guid = "h-new", heroId = "hero_002" };
        StaticData.playerData.InstPlayerData.heroes.Remove("h-new");
        NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true);
        Check(StaticData.playerData.GetUIHero("h-new") == null, "ABA restored consumed instance");
        Wait(20105); NetAPI.Deliver(Warriors(), true); Progress(1);
        Check(StaticData.playerData.GetUIHero("h-new") == null, "missing instance revived on retry");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true); SetVersion(UInt64.MaxValue); Wait(20105);
        state = typeof(Runtime).GetField("active", All).GetValue(null);
        object priorEpoch = state.GetType().GetField("Epoch", All).GetValue(state);
        Runtime.ObserveSend(9999, new object());
        Check(!Object.ReferenceEquals(priorEpoch, state.GetType().GetField("Epoch", All).GetValue(state)), "generation overflow did not rotate epoch");
        NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true);
        Check(StaticData.playerData.GetUIHero("h-new") == null, "overflow accepted stale snapshot");
        Wait(20105); NetAPI.Deliver(Warriors(), true); Progress(1);

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true);
        NetAPI.DuringSend = delegate(int opcode, object body) { NetAPI.SendHttp(9999, new object(), delegate(Packet p, bool ok) { }); };
        Progress(1); Check(NetAPI.Opcode == 9999, "recursive unrelated send was changed"); NetAPI.Deliver(new object(), true);
        Wait(20105); NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true); Progress(1);
        Check(StaticData.playerData.GetUIHero("h-new") != null, "recursive send blocked retry");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true);
        NetAPI.DuringSend = delegate(int opcode, object body) { NetAPI.SendHttp(opcode, body, delegate(Packet p, bool ok) { }); };
        Progress(1); Check(NetAPI.Opcode == 20105, "recursive own request was intercepted"); NetAPI.Deliver(Warriors(), true);
        Wait(20105); NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true); Progress(1);
        Check(StaticData.playerData.GetUIHero("h-new") != null, "recursive own request prevented retry");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true); Wait(20105); HttpHandler late = NetAPI.Pending;
        Progress(16); NetAPI.Reset(); BoxManager.Reset();
        late(new Packet { nOpCode = 20106, kBody = Warriors(new Warrior { id = "hero_002", guid = "h-new" }) }, true);
        Check(StaticData.playerData.GetUIHero("h-new") == null, "timed-out response applied");
        Wait(20105); NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true); Progress(1);
        Check(StaticData.playerData.GetUIHero("h-new") != null, "timeout retry failed");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true); Wait(20105); UIPlayerData prior = StaticData.playerData;
        StaticData.playerData = new UIPlayerData { playerId = 7 }; StaticData.sessionKey = new byte[] { 4, 3, 2, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0 };
        NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true); Progress(1);
        Check(prior.GetUIHero("h-new") == null && StaticData.playerData.GetUIHero("h-new") == null, "old session callback changed inventory");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true); Wait(20105); prior = StaticData.playerData;
        prior.playerId = 8; NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true); Progress(1);
        Check(prior.GetUIHero("h-new") == null, "changed role callback applied");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true); Wait(20105); prior = StaticData.playerData;
        StaticData.UserName = "account-b"; NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true); Progress(1);
        Check(prior.GetUIHero("h-new") == null, "changed account callback applied");

        Reset(); Open(Resource("hero_002", "h-new")); Leave(true); Wait(20105);
        RoomManager.currentRoom = new RoomCopy(); Open(Resource("equip_002", "e-new"));
        NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true);
        Check(StaticData.playerData.GetUIHero("h-new") == null, "new chest did not invalidate old list");
        Wait(20105); NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true); Progress(1);
        Check(NetAPI.Pending == null && StaticData.playerData.GetUIEquipment("e-new") == null, "new room synchronized before its exit");
        Leave(true); Wait(20103); NetAPI.Deliver(Equip(new Equipment { equipId = "equip_002", guid = "e-new" }), true); Progress(1);
        Check(StaticData.playerData.GetUIEquipment("e-new") != null, "second room did not synchronize");

        Reset(); HttpHandler broken = Runtime.WrapOpen(delegate(Packet p, bool ok) { throw new InvalidOperationException("native callback"); });
        SCCopyBox brokenReply = new SCCopyBox(); brokenReply.currentResource.Add(Resource("hero_002", "h-new"));
        try { broken(new Packet { nOpCode = 54020, kBody = brokenReply }, true); throw new Exception("native callback swallowed"); }
        catch (InvalidOperationException) { }
        Leave(true); Wait(20105); NetAPI.Deliver(Warriors(new Warrior { id = "hero_002", guid = "h-new" }), true); Progress(1);
        Check(StaticData.playerData.GetUIHero("h-new") != null, "native callback exception lost pending synchronization");

        Reset(); Open(Resource("coin", ""), Resource("diamond", "")); Leave(true); Progress(1);
        Check(NetAPI.Pending == null && Scheduler.Work.Count == 0, "currency produced inventory request");
        Console.WriteLine("PASS " + checks + " unique-instance synchronization and disabled-stack counterexample model checks; no Unity or device execution");
        return 0;
    }
}
