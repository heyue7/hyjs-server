// In-process test doubles for Runtime.cs. These checks model ownership and
// scheduling; they do not exercise Unity networking or the real game UI.
using System;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using HYJS.Ranks;
using NetWork.Layer;
using NetWork.Packets;
using UnityEngine;

namespace UnityEngine
{
    public class Object
    {
        public bool Destroyed;
        public static void Destroy(Object value) { value.Destroyed = true; }
    }
    public struct Vector2 { public float x, y; public Vector2(float a, float b) { x = a; y = b; } }
    public struct Vector3 { public float x, y, z; public Vector3(float a, float b, float c) { x = a; y = b; z = c; } }
    public class Component : Object
    {
        internal GameObject host;
        public GameObject gameObject { get { return host; } }
        public Transform transform { get { return host.transform; } }
        public T GetComponentInParent<T>() where T : Component
        {
            for (GameObject go = host; go != null; go = go.parent)
            {
                T[] matches = go.GetComponents<T>();
                if (matches.Length != 0) return matches[0];
            }
            return null;
        }
        public T[] GetComponentsInChildren<T>(bool includeInactive) where T : Component
        { return host.GetComponentsInChildren<T>(); }
    }
    public class Behaviour : Component { public bool enabled = true; }
    public class MonoBehaviour : Behaviour { }
    public class Transform : Component
    {
        public Vector3 localPosition;
        public Transform parent
        {
            get { return host.parent == null ? null : host.parent.transform; }
            set
            {
                if (host.parent != null) host.parent.children.Remove(host);
                host.parent = value == null ? null : value.gameObject;
                if (host.parent != null) host.parent.children.Add(host);
            }
        }
    }
    public class GameObject : Object
    {
        internal readonly List<Component> components = new List<Component>();
        internal readonly List<GameObject> children = new List<GameObject>();
        public GameObject parent;
        public bool active = true;
        public bool activeInHierarchy { get { return active && (parent == null || parent.activeInHierarchy); } }
        public Transform transform;
        public GameObject() { transform = AddComponent<Transform>(); }
        public void AddChild(GameObject child) { child.transform.parent = transform; }
        public void SetActive(bool value) { active = value; }
        public T AddComponent<T>() where T : Component, new()
        { T result = new T(); result.host = this; components.Add(result); return result; }
        public T[] GetComponents<T>() where T : Component
        {
            List<T> results = new List<T>();
            foreach (Component component in components) if (component is T) results.Add((T)component);
            return results.ToArray();
        }
        public T[] GetComponentsInChildren<T>() where T : Component
        {
            List<T> results = new List<T>(GetComponents<T>());
            foreach (GameObject child in children) results.AddRange(child.GetComponentsInChildren<T>());
            return results.ToArray();
        }
    }
    public static class Time { public static float realtimeSinceStartup; }
    public sealed class WWW : IDisposable
    {
        public static readonly List<WWW> Requests = new List<WWW>();
        public string Url;
        public byte[] Request;
        public bool isDone;
        public string error;
        public byte[] bytes;
        public bool Disposed;
        public WWW(string url, byte[] request) { Url = url; Request = request; Requests.Add(this); }
        public void Dispose() { Disposed = true; }
    }
}

public class UIPanel : MonoBehaviour { public Vector2 clipOffset; }
public class UIScrollView : MonoBehaviour { public UIPanel panel; }
public static class ServerConfig { public static string Url = "http://test.invalid/sao/"; public static string GetServerUrl() { return Url; } }
public static class StaticData
{
    public static UI.UIPlayerData playerData;
    public static byte[] sessionKey;
    public static string UserName;
}
namespace UI
{
    public class UIPlayerData { public long playerId; public int ZoneId; }
    public class Page : MonoBehaviour { }
    public class PageManager { public static PageManager Instance = new PageManager(); public Page currentPage; }
    public class RankPage : Page
    {
        private int rankPageType = 1;
        private readonly List<GameObject> itemList = new List<GameObject>();
        public int Calls;
        public bool LastSuccess;
        public Packet LastPacket;
        public UIScrollView Scroll;
        public int Rows { get { return itemList.Count; } }
        public GameObject AddRow() { GameObject row = new GameObject(); gameObject.AddChild(row); itemList.Add(row); return row; }
        public void Select(int type) { rankPageType = type; }
        public void Receive(Packet packet, bool success)
        {
            Calls++; LastPacket = packet; LastSuccess = success;
            if (success && Scroll != null)
            {
                Scroll.transform.localPosition = new Vector3(0, 0, 0);
                Scroll.panel.clipOffset = new Vector2(0, 0);
            }
        }
    }
    public class UnionRankPage : Page
    {
        private int curPage = 1;
        public int Calls;
        public string Displayed;
        public int CurrentPage { get { return curPage; } }
        public void SetPage(int page) { curPage = page; }
        public void Receive(Packet packet, bool success)
        {
            Calls++;
            ProtoBuf.Message.SCUnionSearch body = packet.kBody as ProtoBuf.Message.SCUnionSearch;
            if (success && body != null && body.unions.Count == 0 && curPage > 1) { curPage--; return; }
            if (success && body != null) Displayed = body.Value;
        }
    }
    public class PowerPage : MonoBehaviour
    {
        private readonly List<long> playerFlagList = new List<long>();
        public readonly List<string> topNames = new List<string>();
        public int CleanCalls;
        public int Calls;
        private void clean() { CleanCalls++; topNames.Clear(); }
        public int Flags { get { return playerFlagList.Count; } }
        public void Receive(Packet packet, bool success)
        {
            Calls++;
            if (success) { playerFlagList.Add(1); topNames.Add(((ProtoBuf.Message.TestBody)packet.kBody).Value); }
        }
    }
}
namespace ProtoBuf.Message
{
    public class CSRank { public int type; }
    public class CSFamous { public int type; }
    public class CSPvpRank { }
    public class CSLimitPvpRank { }
    public class CSUnionSearch { public int page; public string name = ""; }
    public class SCUnionSearch
    {
        public string Value;
        public List<string> unions = new List<string>();
        public SCUnionSearch(string value) { Value = value; if (value != "empty") unions.Add(value); }
    }
    public class SCGood { }
    public class CSUFRankInfo { }
    public class CSKFRankInfo { }
    public class CSWorldBossRank { }
    public class CSBigTurnRank { }
    public class TestBody { public string Value; public TestBody(string value) { Value = value; } }
    public static class PBConvert
    {
        public static byte[] toBytes(int opcode, object data)
        {
            string value;
            if (data is CSRank) value = ((CSRank)data).type.ToString();
            else if (data is CSFamous) value = ((CSFamous)data).type.ToString();
            else if (data is CSUnionSearch) value = ((CSUnionSearch)data).name + "/" + ((CSUnionSearch)data).page.ToString();
            else if (data is SCUnionSearch) value = ((SCUnionSearch)data).Value;
            else if (data is TestBody) value = ((TestBody)data).Value;
            else value = data.GetType().FullName;
            return Encoding.UTF8.GetBytes(value);
        }
    }
}
namespace NetWork.Packets
{
    public class Packet
    {
        public int nOpCode;
        public object kBody;
        public Packet(int opcode, object body) { nOpCode = opcode; kBody = body; }
    }
}
namespace NetWork.Layer { public delegate void HttpHandler(Packet packet, bool success); }
namespace NetWork.Packets.Factory
{
    public static class PacketFactory
    {
        public static int Frames;
        public static bool ThrowDecode;
        public static bool WebPB2Bytes(int opcode, object body, out byte[] frame)
        { Frames++; frame = Encoding.UTF8.GetBytes(opcode.ToString() + ":" + Frames.ToString() + ":" + Encoding.UTF8.GetString(ProtoBuf.Message.PBConvert.toBytes(opcode, body))); return true; }
        public static bool WebBytes2PB(byte[] bytes, out int opcode, out object body, out List<Packet> extras)
        {
            if (ThrowDecode) { ThrowDecode = false; throw new InvalidOperationException("model decode"); }
            string[] parts = Encoding.UTF8.GetString(bytes).Split('|');
            opcode = Int32.Parse(parts[0]);
            body = opcode == 57072 ? (object)new ProtoBuf.Message.SCUnionSearch(parts[1]) : new ProtoBuf.Message.TestBody(parts[1]);
            extras = new List<Packet>(); extras.Add(new Packet(20102, new object()));
            return true;
        }
    }
}
public static class NetAPI
{
    public static readonly List<HttpHandler> Pending = new List<HttpHandler>();
    public static bool Accepted = true;
    public static bool SendHttp(int opcode, object request, HttpHandler handler)
    { if (!Accepted) return false; Pending.Add(handler); return true; }
    public static void Reply(int index, int opcode, string value, bool ok)
    {
        object body = opcode == 57072 ? (object)new ProtoBuf.Message.SCUnionSearch(value) : new ProtoBuf.Message.TestBody(value);
        Pending[index](new Packet(opcode, body), ok);
    }
}

public static class ModelTests
{
    private static int assertions;
    private static int staticCalls;
    private static void StaticHandler(Packet packet, bool success) { staticCalls++; }
    private static void Check(bool value, string message)
    { assertions++; if (!value) throw new Exception(message); }
    private static void SetTime(float t) { Time.realtimeSinceStartup = t; }
    private static void Tick(RankRefreshRunner runner)
    { typeof(RankRefreshRunner).GetMethod("Update", BindingFlags.Instance | BindingFlags.NonPublic).Invoke(runner, null); }
    private static void Disable(RankRefreshRunner runner)
    { typeof(RankRefreshRunner).GetMethod("OnDisable", BindingFlags.Instance | BindingFlags.NonPublic).Invoke(runner, null); }
    private static void Complete(WWW request, int opcode, string value)
    { request.bytes = Encoding.UTF8.GetBytes(opcode.ToString() + "|" + value); request.isDone = true; }
    private static RankRefreshRunner Runner(MonoBehaviour owner)
    {
        RankRefreshRunner[] runners = owner.gameObject.GetComponents<RankRefreshRunner>();
        return runners.Length == 0 ? null : runners[0];
    }
    private static void Reset()
    {
        NetAPI.Pending.Clear(); NetAPI.Accepted = true; WWW.Requests.Clear(); SetTime(0);
        StaticData.playerData = new UI.UIPlayerData(); StaticData.playerData.playerId = 5; StaticData.playerData.ZoneId = 7;
        StaticData.sessionKey = new byte[16]; StaticData.sessionKey[0] = 8; StaticData.UserName = "account";
        ServerConfig.Url = "http://test.invalid/sao/"; UI.PageManager.Instance.currentPage = null;
        NetWork.Packets.Factory.PacketFactory.Frames = 0;
    }
    private static UI.RankPage MakePage()
    {
        GameObject go = new GameObject(); UI.RankPage page = go.AddComponent<UI.RankPage>();
        UI.PageManager.Instance.currentPage = page;
        GameObject child = new GameObject(); go.AddChild(child);
        page.Scroll = child.AddComponent<UIScrollView>(); page.Scroll.panel = child.AddComponent<UIPanel>();
        return page;
    }
    private static void TimingResponsesAndScroll()
    {
        Reset(); UI.RankPage page = MakePage(); ProtoBuf.Message.CSRank request = new ProtoBuf.Message.CSRank();
        Check(Refresh.Send(21057, request, page.Receive), "initial send accepted");
        Check(Runner(page) == null, "no runner before initial result");
        SetTime(50); Check(WWW.Requests.Count == 0, "no background send while initial pending");
        NetAPI.Reply(0, 21058, "A", true); RankRefreshRunner runner = Runner(page);
        Check(runner != null && page.Calls == 1 && page.LastSuccess && page.LastPacket.nOpCode == 21058, "initial callback and bind");
        SetTime(64.99f); Tick(runner); Check(WWW.Requests.Count == 0, "15 second interval");
        SetTime(65); Tick(runner); Check(WWW.Requests.Count == 1 && WWW.Requests[0].Url == "http://test.invalid/sao/", "fresh frame and full URL");
        Complete(WWW.Requests[0], 21058, "A"); SetTime(66); Tick(runner);
        Check(page.Calls == 1 && WWW.Requests[0].Disposed, "same primary does not redraw");
        page.Scroll.transform.localPosition = new Vector3(1, 72, 3); page.Scroll.panel.clipOffset = new Vector2(4, 51);
        SetTime(80); Tick(runner); Check(WWW.Requests.Count == 2, "next period sends");
        SetTime(81); Tick(runner); Check(WWW.Requests.Count == 2, "at most one in flight");
        Complete(WWW.Requests[1], 21058, "B"); SetTime(82); Tick(runner);
        Check(page.Calls == 2 && page.LastPacket.nOpCode == 21058, "changed primary redraws once");
        Check(page.Scroll.transform.localPosition.y == 72 && page.Scroll.panel.clipOffset.y == 51, "scroll position and clipping restored");
        Check(NetWork.Packets.Factory.PacketFactory.Frames == 2, "one newly encoded frame per attempt");
        SetTime(95); Tick(runner); SetTime(100); Tick(runner);
        Check(WWW.Requests[2].Disposed && page.Calls == 2, "timeout disposes without redraw");
        SetTime(109); Tick(runner); Check(WWW.Requests.Count == 3, "timeout waits for next period");
        SetTime(110); Tick(runner); Complete(WWW.Requests[3], 10001, "error"); SetTime(111); Tick(runner);
        Check(page.Calls == 2, "server error opcode does not redraw");
        SetTime(125); Tick(runner); Complete(WWW.Requests[4], 21058, "C");
        NetWork.Packets.Factory.PacketFactory.ThrowDecode = true; SetTime(126); Tick(runner);
        Check(page.Calls == 2 && WWW.Requests[4].Disposed, "decode exception keeps last table");
        SetTime(140); Tick(runner); Check(WWW.Requests.Count == 6, "decode failure retries next period");
    }
    private static void GenerationAndOwner()
    {
        Reset(); UI.RankPage page = MakePage();
        Refresh.Send(21057, new ProtoBuf.Message.CSRank(), page.Receive); NetAPI.Reply(0, 21058, "A", true);
        RankRefreshRunner runner = Runner(page); SetTime(15); Tick(runner); WWW old = WWW.Requests[0];
        page.Select(2); Refresh.Send(23009, new ProtoBuf.Message.CSPvpRank(), page.Receive);
        Check(old.Disposed, "manual selection cancels old request");
        Complete(old, 21058, "old"); NetAPI.Reply(1, 23010, "new", true);
        SetTime(30); Tick(runner); Check(WWW.Requests.Count == 2, "new selection becomes current");
        Complete(WWW.Requests[1], 23010, "newer"); SetTime(31); Tick(runner);
        Check(page.Calls == 3 && page.LastPacket.nOpCode == 23010, "only new selection background result applied");
        SetTime(45); Tick(runner); WWW changedSession = WWW.Requests[2]; StaticData.sessionKey[0] = 9; SetTime(46); Tick(runner);
        Check(changedSession.Disposed && page.Calls == 3, "session change cancels in-flight request");
        SetTime(90); Tick(runner); Check(WWW.Requests.Count == 3, "old session remains stopped");

        Reset(); page = MakePage();
        Refresh.Send(21057, new ProtoBuf.Message.CSRank(), page.Receive);
        page.Select(2); Refresh.Send(23009, new ProtoBuf.Message.CSPvpRank(), page.Receive);
        NetAPI.Reply(0, 21058, "old initial", true);
        Check(page.Calls == 1 && Runner(page) == null, "old initial retains native callback but cannot register");
        NetAPI.Reply(1, 23010, "new initial", true); runner = Runner(page);
        Check(runner != null, "latest initial can register");
        SetTime(15); Tick(runner); WWW oldPage = WWW.Requests[0];
        page.enabled = false; SetTime(16); Tick(runner); Check(oldPage.Disposed, "disabled owner disposes request");
    }
    private static void InputAndLifetimes()
    {
        Reset(); UI.RankPage page = MakePage();
        Refresh.Send(99999, new ProtoBuf.Message.CSRank(), page.Receive);
        NetAPI.Reply(0, 100000, "x", true);
        Check(page.Calls == 1 && Runner(page) == null, "unknown opcode passes through without registration");
        Refresh.Send(21057, new ProtoBuf.Message.CSPvpRank(), page.Receive); NetAPI.Reply(1, 21058, "x", true);
        Check(Runner(page) == null, "wrong DTO passes through");
        staticCalls = 0;
        Refresh.Send(21057, new ProtoBuf.Message.CSRank(), StaticHandler); NetAPI.Reply(2, 21058, "x", true);
        Check(staticCalls == 1 && Runner(page) == null, "non-component handler passes through");
        Refresh.Send(21057, new ProtoBuf.Message.CSRank(), page.Receive); NetAPI.Reply(3, 21059, "wrong", true);
        Check(Runner(page) == null && page.LastSuccess, "wrong initial opcode preserves callback without registration");
        Refresh.Send(21057, new ProtoBuf.Message.CSRank(), page.Receive); NetAPI.Reply(4, 10001, "error", false);
        Check(Runner(page) == null && !page.LastSuccess, "failed first request preserves callback and does not bind");
        Refresh.Send(21057, new ProtoBuf.Message.CSRank(), page.Receive); NetAPI.Reply(5, 21058, "ok", true);
        RankRefreshRunner runner = Runner(page); SetTime(15); Tick(runner); WWW flight = WWW.Requests[0];
        Disable(runner); Check(flight.Disposed, "OnDisable disposes own WWW");

        Reset(); page = MakePage(); ProtoBuf.Message.CSRank mutable = new ProtoBuf.Message.CSRank();
        Refresh.Send(21057, mutable, page.Receive); NetAPI.Reply(0, 21058, "ok", true); runner = Runner(page);
        mutable.type = 5; SetTime(15); Tick(runner); Check(WWW.Requests.Count == 0, "mutated request is not resent");

        Reset(); page = MakePage(); Refresh.Send(21057, new ProtoBuf.Message.CSRank(), page.Receive);
        NetAPI.Reply(0, 21058, "ok", true); runner = Runner(page);
        GameObject coveredObject = new GameObject(); UI.Page covered = coveredObject.AddComponent<UI.Page>();
        UI.PageManager.Instance.currentPage = covered; SetTime(15); Tick(runner);
        Check(WWW.Requests.Count == 0, "covered page does not refresh");
        UI.PageManager.Instance.currentPage = page; Tick(runner); Check(WWW.Requests.Count == 1, "previously displayed page resumes when uncovered");
        ServerConfig.Url = "http://changed.invalid/sao/"; SetTime(16); Tick(runner);
        Check(WWW.Requests[0].Disposed, "server change cancels request");

        Reset(); page = MakePage(); Refresh.Send(21057, new ProtoBuf.Message.CSRank(), page.Receive);
        NetAPI.Reply(0, 21058, "ok", true); runner = Runner(page); SetTime(15); Tick(runner);
        StaticData.playerData.ZoneId++; SetTime(16); Tick(runner);
        Check(WWW.Requests[0].Disposed, "zone change cancels request");

        Reset(); page = MakePage(); Refresh.Send(21057, new ProtoBuf.Message.CSRank(), page.Receive);
        NetAPI.Reply(0, 21058, "ok", true); runner = Runner(page); SetTime(15); Tick(runner);
        StaticData.UserName = "other"; SetTime(16); Tick(runner);
        Check(WWW.Requests[0].Disposed, "account change cancels request");
    }
    private static void UnionAndFamous()
    {
        Reset(); GameObject go = new GameObject(); UI.UnionRankPage page = go.AddComponent<UI.UnionRankPage>();
        UI.PageManager.Instance.currentPage = page; page.SetPage(2);
        ProtoBuf.Message.CSUnionSearch request = new ProtoBuf.Message.CSUnionSearch(); request.page = 2;
        Refresh.Send(57071, request, page.Receive); NetAPI.Reply(0, 57072, "empty", true);
        Check(page.Calls == 1 && Runner(page) == null, "last empty page fallback must not register wrong page");

        Reset(); GameObject root = new GameObject(); UI.Page host = root.AddComponent<UI.Page>(); UI.PageManager.Instance.currentPage = host;
        GameObject child = new GameObject(); root.AddChild(child); UI.PowerPage famous = child.AddComponent<UI.PowerPage>();
        Refresh.Send(23023, new ProtoBuf.Message.CSFamous(), famous.Receive); NetAPI.Reply(0, 23024, "old", true);
        RankRefreshRunner runner = Runner(famous); Check(famous.Flags == 1 && famous.topNames.Count == 1, "famous initial data");
        SetTime(15); Tick(runner); Complete(WWW.Requests[0], 23024, "new"); SetTime(16); Tick(runner);
        Check(famous.Calls == 2 && famous.CleanCalls == 1 && famous.Flags == 1 && famous.topNames.Count == 1 && famous.topNames[0] == "new", "famous cleanup before changed result");
    }
    private static void ReplaceRankRowsAndWorship()
    {
        Reset(); UI.RankPage page = MakePage();
        Refresh.Send(21057, new ProtoBuf.Message.CSRank(), page.Receive); NetAPI.Reply(0, 21058, "old", true);
        GameObject old1 = page.AddRow(); GameObject old2 = page.AddRow();
        GameObject header = new GameObject(); page.gameObject.AddChild(header);
        RankRefreshRunner runner = Runner(page); SetTime(15); Tick(runner);
        Complete(WWW.Requests[0], 21058, "changed"); SetTime(16); Tick(runner);
        Check(old1.Destroyed && old2.Destroyed && !old1.active && !old2.active, "old owned rows destroyed and deactivated");
        Check(old1.transform.parent == null && old2.transform.parent == null && page.Rows == 0, "old rows detached and item list cleared");
        Check(!header.Destroyed && header.active && header.transform.parent == page.transform, "non-row header is preserved");
        Check(page.Calls == 2, "rank callback follows row cleanup");

        Reset(); GameObject root = new GameObject(); UI.Page host = root.AddComponent<UI.Page>(); UI.PageManager.Instance.currentPage = host;
        GameObject child = new GameObject(); root.AddChild(child); UI.PowerPage famous = child.AddComponent<UI.PowerPage>();
        Refresh.Send(23023, new ProtoBuf.Message.CSFamous(), famous.Receive); NetAPI.Reply(0, 23024, "old", true);
        runner = Runner(famous); SetTime(15); Tick(runner); WWW beforeWorship = WWW.Requests[0];
        Refresh.WorshipCompleted(famous, new Packet(23022, new ProtoBuf.Message.SCGood()), false);
        Refresh.WorshipCompleted(famous, new Packet(23022, new ProtoBuf.Message.TestBody("wrong")), true);
        Check(!beforeWorship.Disposed, "failed or wrong-type worship does not change runner");
        SetTime(16); Refresh.WorshipCompleted(famous, new Packet(23022, new ProtoBuf.Message.SCGood()), true);
        Check(beforeWorship.Disposed && famous.Calls == 1 && famous.Flags == 1, "successful worship cancels old rank without clearing displayed view");
        Complete(beforeWorship, 23024, "stale"); SetTime(30.9f); Tick(runner);
        Check(WWW.Requests.Count == 1 && famous.Calls == 1, "stale worship-era response cannot apply and next read waits");
        SetTime(31); Tick(runner); Check(WWW.Requests.Count == 2, "worship schedules next read after 15 seconds");
        Complete(WWW.Requests[1], 23024, "fresh"); SetTime(32); Tick(runner);
        Check(famous.Calls == 2 && famous.Flags == 1 && famous.topNames[0] == "fresh", "new rank snapshot remains active after worship");
    }
    private static void UnionFirstPageRecovery()
    {
        Reset(); GameObject go = new GameObject(); UI.UnionRankPage page = go.AddComponent<UI.UnionRankPage>();
        UI.PageManager.Instance.currentPage = page; page.SetPage(2);
        ProtoBuf.Message.CSUnionSearch request = new ProtoBuf.Message.CSUnionSearch(); request.page = 2; request.name = "";
        Refresh.Send(57071, request, page.Receive); NetAPI.Reply(0, 57072, "old-page-2", true);
        RankRefreshRunner runner = Runner(page); SetTime(15); Tick(runner);
        Complete(WWW.Requests[0], 57072, "empty"); SetTime(16); Tick(runner);
        Check(WWW.Requests.Count == 2 && page.CurrentPage == 2 && page.Displayed == "old-page-2" && request.page == 2, "empty last page starts one bounded page-1 read without changing the view");
        Check(Encoding.UTF8.GetString(WWW.Requests[1].Request).EndsWith("/1"), "fallback request asks for page one");
        WWW.Requests[1].error = "network"; WWW.Requests[1].isDone = true; SetTime(17); Tick(runner);
        Check(page.CurrentPage == 2 && page.Displayed == "old-page-2" && request.page == 2, "fallback failure preserves prior page");
        SetTime(29); Tick(runner); Check(WWW.Requests.Count == 2, "fallback failure waits until next normal period");
        SetTime(30); Tick(runner); Complete(WWW.Requests[2], 57072, "empty"); SetTime(31); Tick(runner);
        Check(WWW.Requests.Count == 4 && page.CurrentPage == 2, "next cycle gets one new fallback");
        Complete(WWW.Requests[3], 57072, "first-page"); SetTime(32); Tick(runner);
        Check(page.CurrentPage == 1 && request.page == 1 && page.Displayed == "first-page" && page.Calls == 2, "successful fallback commits page and original callback");
        SetTime(45); Tick(runner); Check(Encoding.UTF8.GetString(WWW.Requests[4].Request).EndsWith("/1"), "following periods refresh page one");

        Reset(); go = new GameObject(); page = go.AddComponent<UI.UnionRankPage>(); UI.PageManager.Instance.currentPage = page; page.SetPage(2);
        request = new ProtoBuf.Message.CSUnionSearch(); request.page = 2;
        Refresh.Send(57071, request, page.Receive); NetAPI.Reply(0, 57072, "old", true);
        runner = Runner(page); SetTime(15); Tick(runner); Complete(WWW.Requests[0], 57072, "empty"); SetTime(16); Tick(runner);
        Complete(WWW.Requests[1], 57072, "empty"); SetTime(17); Tick(runner);
        Check(page.CurrentPage == 1 && page.Displayed == "empty" && page.Calls == 2 && WWW.Requests.Count == 2, "empty first page displays empty without recursion");

        Reset(); go = new GameObject(); page = go.AddComponent<UI.UnionRankPage>(); UI.PageManager.Instance.currentPage = page; page.SetPage(2);
        request = new ProtoBuf.Message.CSUnionSearch(); request.page = 2;
        Refresh.Send(57071, request, page.Receive); NetAPI.Reply(0, 57072, "old", true);
        runner = Runner(page); SetTime(15); Tick(runner); Complete(WWW.Requests[0], 57072, "empty"); SetTime(16); Tick(runner);
        WWW stale = WWW.Requests[1]; page.SetPage(3); ProtoBuf.Message.CSUnionSearch newRequest = new ProtoBuf.Message.CSUnionSearch(); newRequest.page = 3;
        Refresh.Send(57071, newRequest, page.Receive); Check(stale.Disposed, "manual page change disposes first-page recovery");
        Complete(stale, 57072, "stale-first"); NetAPI.Reply(1, 57072, "page-three", true); SetTime(17); Tick(runner);
        Check(page.CurrentPage == 3 && page.Displayed == "page-three" && request.page == 2, "old fallback cannot overwrite new selection");

        Reset(); go = new GameObject(); page = go.AddComponent<UI.UnionRankPage>(); UI.PageManager.Instance.currentPage = page; page.SetPage(2);
        request = new ProtoBuf.Message.CSUnionSearch(); request.page = 2;
        Refresh.Send(57071, request, page.Receive); NetAPI.Reply(0, 57072, "old", true);
        runner = Runner(page); SetTime(15); Tick(runner); Complete(WWW.Requests[0], 57072, "empty"); SetTime(16); Tick(runner);
        stale = WWW.Requests[1]; StaticData.sessionKey[0]++; SetTime(17); Tick(runner);
        Check(stale.Disposed && page.CurrentPage == 2 && request.page == 2, "session change cancels fallback without changing page");
    }
    public static void Main()
    {
        TimingResponsesAndScroll(); GenerationAndOwner(); InputAndLifetimes(); UnionAndFamous();
        ReplaceRankRowsAndWorship(); UnionFirstPageRecovery();
        Console.WriteLine("Rank refresh model passed " + assertions + " assertions.");
    }
}
