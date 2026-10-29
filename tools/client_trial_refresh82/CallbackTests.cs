// Boundary doubles exercise the actual TrialShopRefresh75 callbacks. They do
// not claim to reproduce Unity/NGUI rendering or a running game client.
using System;
using System.Collections.Generic;
using System.Reflection;
using HYJS.GGO;
using NetWork.Layer;
using NetWork.Packets;
using ProtoBuf.Message;
using UI;
using UnityEngine;

namespace UnityEngine
{
    public class Object
    {
        public string name;
        public static Object Instantiate(Object o) { return o; }
        public static void Destroy(Object o) { }
        public static void DestroyImmediate(Object o) { }
    }
    public class Component : Object
    {
        public GameObject gameObject;
        public Transform transform { get { return gameObject.transform; } }
        public T GetComponent<T>() where T : Component { return gameObject.GetComponent<T>(); }
        public T[] GetComponents<T>() where T : Component { return gameObject.GetComponents<T>(); }
    }
    public class MonoBehaviour : Component { }
    public class Collider : Component { public bool enabled; }
    public class Transform : Component
    {
        public Transform parent;
        public Vector3 localPosition, localScale, position, lossyScale;
        public Quaternion localRotation, rotation;
        public Transform Find(string path) { return null; }
        public Vector3 InverseTransformPoint(Vector3 p) { return p; }
    }
    public struct Vector3
    {
        public float x, y, z;
        public Vector3(float a, float b, float c) { x = a; y = b; z = c; }
        public static Vector3 operator -(Vector3 a, Vector3 b) { return new Vector3(a.x-b.x, a.y-b.y, a.z-b.z); }
    }
    public struct Quaternion
    {
        public static Quaternion Inverse(Quaternion q) { return q; }
        public static Quaternion operator *(Quaternion a, Quaternion b) { return a; }
    }
    public class GameObject : Object
    {
        readonly Dictionary<Type, Component> components = new Dictionary<Type, Component>();
        public bool activeInHierarchy = true;
        public Transform transform;
        public GameObject() { transform = new Transform(); transform.gameObject = this; }
        public T AddComponent<T>() where T : Component, new() { T c = new T(); c.gameObject = this; components[typeof(T)] = c; return c; }
        public T GetComponent<T>() where T : Component { Component c; return components.TryGetValue(typeof(T), out c) ? (T)c : null; }
        public T[] GetComponents<T>() where T : Component { T c = GetComponent<T>(); return c == null ? new T[0] : new T[] { c }; }
        public T[] GetComponentsInChildren<T>(bool includeInactive) where T : Component { return GetComponents<T>(); }
        public void SetActive(bool value) { activeInHierarchy = value; }
    }
    public static class Time { public static float realtimeSinceStartup = 10; }
    public static class Debug { public static void LogWarning(object o) { } }
}
public class UILabel : MonoBehaviour { public string text; }
public class UISprite : MonoBehaviour { public string spriteName; }
public class UIButton : MonoBehaviour { public List<object> onClick = new List<object>(); public bool isEnabled; }
public class UIButtonMessage : MonoBehaviour { }
public class UIEventListener : MonoBehaviour
{
    public Action<GameObject> onClick;
    public static UIEventListener Get(GameObject go) { return go.AddComponent<UIEventListener>(); }
}
public static class EventDelegate { public static void Set(List<object> list, Action action) { } }
public class UIScrollView : MonoBehaviour { public void ResetPosition() { } }
public class SimpleGrid : MonoBehaviour
{
    public int arrangement, maxPerLine;
    public float cellWidth, cellHeight, paddingX, paddingY;
    public List<GameObject> childs = new List<GameObject>();
    public void AddChild(GameObject child, float scale) { childs.Add(child); }
    public void Reposition() { }
}
public static class TableTextBlo { public static string GetText(string key) { return key; } }
public static class CommonMethod { public static string GetObjectNameTextId(string id) { return id; } }
public static class BoxManager { public static void CreatePopupTextBox(string value, float seconds) { } }
public static class ServerTimerTool { public static long CurrentUTCJavaTime() { return 100000; } }
public static class StaticData { public static UIPlayerData playerData; public static byte[] sessionKey = new byte[16]; }
public static class ServerConfig { public static string GetServerUrl() { return "http://local.invalid/game"; } }
namespace AssetLoad
{
    public class ResourceManager { public static ResourceManager Instance = new ResourceManager(); public object LoadResource(string name) { return null; } }
}
namespace NetWork.Packets { public class Packet { public object kBody; public Packet(object body) { kBody = body; } } }
namespace NetWork.Layer { public delegate void HttpHandler(Packet packet, bool ok); }
public static class NetAPI
{
    public static int Opcode, Sends;
    public static object Request;
    public static HttpHandler Handler;
    public static bool SendHttp(int code, object body, HttpHandler handler) { Opcode = code; Request = body; Handler = handler; Sends++; return true; }
    public static void Reset() { Opcode = Sends = 0; Request = null; Handler = null; }
    public static void Reply(object body) { Handler(new Packet(body), true); }
}
namespace ProtoBuf.Message
{
    public class BaseResource { public string id; public int num; }
    public class ShopCondition { public string type; public long value; }
    public class ShopItem
    {
        public string id;
        public int priority, canBuy;
        public List<BaseResource> item = new List<BaseResource>(), exchangeList = new List<BaseResource>();
        public ShopCondition condition;
    }
    public class ResultObject { }
    public class CSShopBuy { public int shopType, buyCount; public string shopItemId; }
    public class SCShopBuy { public int shopType; public List<ShopItem> shopItemList = new List<ShopItem>(); public List<BaseResource> cost = new List<BaseResource>(); public ResultObject get; }
    public class CSRefreshShopQuery { public int shopType; }
    public class SCRefreshShopQuery { public int shopType, freeLeftTimes, freeMaxTimes, resLeftTimes, resMaxTimes; public long nextUpdateTime; public List<ShopItem> shopItemList = new List<ShopItem>(); }
    public class CSRefreshShopRefresh { public int shopType; }
    public class SCRefreshShopRefresh { public int shopType, freeLeftTimes, freeMaxTimes, resLeftTimes, resMaxTimes; public long nextUpdateTime; public List<ShopItem> shopItemList = new List<ShopItem>(); public List<BaseResource> cost = new List<BaseResource>(); }
}
namespace UI
{
    public class UIPlayerData
    {
        public long playerId, diamond;
        public int strategyScore, Deletes, Adds;
        public long GetSimpleItemCount(string id) { return 0; }
        public void Delete(List<BaseResource> resources) { Deletes++; }
        public void AddResultObj(ResultObject result) { Adds++; }
    }
    public class ShopPage : MonoBehaviour
    {
        GameObject curTab;
        public void Select(GameObject tab) { curTab = tab; }
        GameObject GetRightTab(string path) { return null; }
    }
    public class TabMemoryShop : MonoBehaviour
    {
        public SimpleGrid simpleGrid;
        public GameObject shopItemPrefab, freeRefreshCDRoot;
        public UILabel labLeftRefreshTimes, labRefreshCost, labCountDown;
    }
    public class ShopPointItem : MonoBehaviour { public UILabel conditionLabel; }
    public class ShopMemoryItem : MonoBehaviour
    {
        public GameObject btnBuy;
        public UILabel labPriceName;
        public UISprite costLogo;
        public void SetShopMemoryItem(ShopItem item) { }
    }
    public class TabPointShop : MonoBehaviour
    {
        public ShopPage shopPage;
        public SimpleGrid simpleGrid;
        public GameObject shopItemPrefab;
        public UIScrollView scrollView;
        List<ShopItem> shopItems;
        int curTabIndex;
        public int NativeUpdates, NativePaints;
        public List<ShopItem> VisibleItems { get { return shopItems; } }
        public void Group(int value) { curTabIndex = value; }
        public void UpdateAll() { if (!TrialShopRefresh75.HandleUpdateAll(this)) NativeUpdates++; }
        void UpdateProducts() { if (!TrialShopRefresh75.HandleProducts(this)) NativePaints++; }
    }
}

public static class CallbackTests
{
    static void Check(bool ok, string message) { if (!ok) throw new Exception(message); }
    static void Set(object owner, string name, object value) { owner.GetType().GetField(name, BindingFlags.Instance | BindingFlags.NonPublic).SetValue(owner, value); }
    static void Invoke(object owner, string name) { owner.GetType().GetMethod(name, BindingFlags.Instance | BindingFlags.NonPublic).Invoke(owner, null); }
    static ShopItem Slot()
    {
        ShopItem item = new ShopItem(); item.id = "slot-g1-s1-source"; item.priority = 1001; item.canBuy = 1;
        item.item.Add(new BaseResource { id = "item_005", num = 1 }); item.exchangeList.Add(new BaseResource { id = "point", num = 20 }); return item;
    }
    static SCRefreshShopQuery Query()
    {
        SCRefreshShopQuery q = new SCRefreshShopQuery(); q.shopType = 5; q.freeLeftTimes = 1; q.freeMaxTimes = 10; q.resLeftTimes = q.resMaxTimes = 15; q.nextUpdateTime = 200000;
        q.shopItemList.Add(Slot()); q.shopItemList.Add(new ShopItem { id = "purple", priority = 2001 }); return q;
    }
    static TabPointShop Create(long id)
    {
        StaticData.playerData = new UIPlayerData { playerId = id, diamond = 50, strategyScore = 100 };
        GameObject shopObject = new GameObject(); ShopPage shop = shopObject.AddComponent<ShopPage>();
        GameObject tabObject = new GameObject(); TabPointShop tab = tabObject.AddComponent<TabPointShop>();
        tab.shopPage = shop; tab.simpleGrid = new GameObject().AddComponent<SimpleGrid>(); tab.scrollView = new GameObject().AddComponent<UIScrollView>();
        tab.Group(1); shop.Select(tabObject); return tab;
    }
    static TrialShopRefresh75 Control(TabPointShop tab) { return tab.shopPage.GetComponent<TrialShopRefresh75>(); }
    static void Load(TabPointShop tab)
    {
        Check(TrialShopRefresh75.HandleUpdateAll(tab), "query hook");
        Check(NetAPI.Opcode == 24503 && ((CSRefreshShopQuery)NetAPI.Request).shopType == 5, "query protocol");
        NetAPI.Reply(Query());
        Check(tab.NativePaints == 1 && tab.VisibleItems.Count == 2, "fixed tab receives complete list");
    }
    public static void Main()
    {
        NetAPI.Reset(); TabPointShop legacy = Create(1);
        Check(TrialShopRefresh75.HandleUpdateAll(legacy), "legacy query hook");
        NetAPI.Reply(new SCRefreshShopQuery { shopType = 3 });
        Check(legacy.NativeUpdates == 1, "legacy server falls back to native listing");

        NetAPI.Reset(); TabPointShop tab = Create(2); Load(tab); TrialShopRefresh75 control = Control(tab);
        tab.Group(0); Set(control, "group", 0); ShopItem slot = tab.VisibleItems[0];
        control.Buy(slot);
        Check(NetAPI.Opcode == 23007 && ((CSShopBuy)NetAPI.Request).shopItemId == slot.id && ((CSShopBuy)NetAPI.Request).shopType == 5, "slot buy identity");
        HttpHandler buy = NetAPI.Handler; tab.Group(2);
        SCShopBuy bought = new SCShopBuy { shopType = 5, get = new ResultObject(), shopItemList = Query().shopItemList };
        buy(new Packet(bought), true); buy(new Packet(bought), true);
        Check(StaticData.playerData.Deletes == 1 && StaticData.playerData.Adds == 1, "duplicate callback applied once");
        Check(tab.NativePaints == 2 && tab.VisibleItems.Count == 2, "buy reply respects selected fixed tab");

        tab.Group(0); Set(control, "group", 0); control.Buy(tab.VisibleItems[0]);
        HttpHandler late = NetAPI.Handler; UIPlayerData oldPlayer = StaticData.playerData;
        tab.shopPage.Select(new GameObject());
        int paints = tab.NativePaints;
        late(new Packet(bought), true);
        Check(oldPlayer.Deletes == 2 && oldPlayer.Adds == 2 && tab.NativePaints == paints, "closed shop accepts balance but does not repaint");

        NetAPI.Reset(); tab = Create(3); Load(tab); control = Control(tab); tab.Group(0); Set(control, "group", 0);
        control.Buy(tab.VisibleItems[0]); late = NetAPI.Handler; oldPlayer = StaticData.playerData;
        StaticData.playerData = new UIPlayerData { playerId = 4 };
        late(new Packet(bought), true);
        Check(oldPlayer.Deletes == 0 && StaticData.playerData.Deletes == 0, "previous owner reply ignored");

        NetAPI.Reset(); tab = Create(5); Load(tab); control = Control(tab); tab.Group(0); Set(control, "group", 0);
        control.Buy(tab.VisibleItems[0]); late = NetAPI.Handler; oldPlayer = StaticData.playerData;
        GameObject nextObject = new GameObject(); TabPointShop nextTab = nextObject.AddComponent<TabPointShop>();
        nextTab.shopPage = tab.shopPage; nextTab.simpleGrid = new GameObject().AddComponent<SimpleGrid>();
        nextTab.scrollView = new GameObject().AddComponent<UIScrollView>(); nextTab.Group(1); tab.shopPage.Select(nextObject);
        int sends = NetAPI.Sends; Check(TrialShopRefresh75.HandleUpdateAll(nextTab) && NetAPI.Sends == sends, "reopen waits for pending transaction");
        late(new Packet(bought), true);
        Check(oldPlayer.Deletes == 1 && oldPlayer.Adds == 1 && nextTab.NativePaints == 0, "old response does not repaint new page");
        Invoke(Control(nextTab), "Update");
        Check(NetAPI.Sends == sends + 1 && NetAPI.Opcode == 24503, "fresh query follows old transaction");
        NetAPI.Reply(Query());
        Check(nextTab.NativePaints == 1 && nextTab.VisibleItems.Count == 2, "new page receives fresh data");

        nextTab.Group(0); control = Control(nextTab); Set(control, "group", 0); Invoke(control, "Refresh");
        Check(NetAPI.Opcode == 24501 && ((CSRefreshShopRefresh)NetAPI.Request).shopType == 5, "refresh protocol");
        HttpHandler refresh = NetAPI.Handler;
        SCRefreshShopRefresh refreshed = new SCRefreshShopRefresh { shopType = 5, freeMaxTimes = 10, freeLeftTimes = 0, resMaxTimes = 15, resLeftTimes = 15, shopItemList = Query().shopItemList };
        refresh(new Packet(refreshed), true); refresh(new Packet(refreshed), true);
        Check(oldPlayer.Deletes == 2, "duplicate refresh cost applied once");
        Console.WriteLine("PASS query fallback, native grouping, opaque slot ID, tab changes, duplicate callbacks, closed page, account change, reopen serialization and refresh");
    }
}
