using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using LitJson;
using UI;
using UnityEngine;

namespace HYJS.KingRecruit
{
    // The three shop side entries and the king page share one owner-scoped state.
    // A shop entry owns world selection; the page only reads the accepted world.
    public sealed class ShopWorldMenuControl : MonoBehaviour
    {
        public sealed class EntryWorld
        {
            public string Pool;
            public string Label;
            public int Space;
        }

        static readonly FieldInfo TabBtnsField = typeof(ShopPage).GetField("tabBtns", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);

        ShopPage shop;
        ShopTabButton template;
        readonly List<ShopWorldEntry> entries = new List<ShopWorldEntry>();
        readonly List<EntryWorld> worlds = new List<EntryWorld>();
        long owner;
        string session = "";
        bool loading, catalogLoaded;
        WWW pending;
        float nextCatalog;
        public string Version = "";
        public string SelectedPool = "";
        public int RequestedWorld;
        public string Notice = "";
        public bool Selecting;
        public bool DrawInFlight;
        public float DrawDeadline;

        // UpdateTabBtns always rebuilds its own buttons; re-apply our three entries after it.
        public static void Ensure(ShopPage page)
        {
            if (page == null) return;
            ShopWorldMenuControl control = page.GetComponent<ShopWorldMenuControl>();
            if (control == null) control = page.gameObject.AddComponent<ShopWorldMenuControl>();
            control.Reapply(page);
        }

        public bool CatalogLoaded { get { return catalogLoaded; } }

        void Reapply(ShopPage page)
        {
            if (shop != page)
            {
                StopAllCoroutines();
                DisposePending();
                shop = page;
                template = null;
                entries.Clear();
                catalogLoaded = false;
                worlds.Clear();
                Version = "";
                SelectedPool = "";
                RequestedWorld = 0;
                Notice = "";
            }
            long currentOwner;
            string currentSession;
            if (!KingRecruitSession.Current(out currentOwner, out currentSession))
            {
                ResetOwner();
                return;
            }
            if (currentOwner != owner || currentSession != session)
            {
                // A different role never inherits the previous selection or in-flight state.
                StopAllCoroutines();
                DisposePending();
                loading = false;
                catalogLoaded = false;
                worlds.Clear();
                SelectedPool = "";
                Version = "";
                RequestedWorld = 0;
                Notice = "";
                owner = currentOwner;
                session = currentSession;
            }
            Dictionary<ShopType, GameObject> buttons = TabBtns();
            if (buttons == null || !buttons.ContainsKey(ShopType.KingsRecruit)) return;
            ShopTabButton current = buttons[ShopType.KingsRecruit].GetComponent<ShopTabButton>();
            if (current == null) return;
            if (current != template)
            {
                template = current;
                entries.Clear();
                // The original type-1 dispatch is replaced by the explicit world entry.
                template.m_callback = null;
                ShopWorldEntry entry = template.GetComponent<ShopWorldEntry>();
                if (entry == null) entry = template.gameObject.AddComponent<ShopWorldEntry>();
                entry.Menu = this;
                entry.Space = 1;
                entries.Add(entry);
            }
            if (!catalogLoaded && !loading && Visible()) StartCoroutine(RefreshCatalog());
            Apply();
        }

        void ResetOwner()
        {
            StopAllCoroutines();
            DisposePending();
            owner = 0;
            session = "";
            loading = false;
            catalogLoaded = false;
            Selecting = false;
            DrawInFlight = false;
            worlds.Clear();
            SelectedPool = "";
            Version = "";
            RequestedWorld = 0;
            Notice = "";
        }

        Dictionary<ShopType, GameObject> TabBtns()
        {
            return shop == null || TabBtnsField == null ? null : TabBtnsField.GetValue(shop) as Dictionary<ShopType, GameObject>;
        }

        bool OwnerCurrent()
        {
            long currentOwner;
            string currentSession;
            if (!KingRecruitSession.Current(out currentOwner, out currentSession)) return false;
            return currentOwner == owner && currentSession == session;
        }

        bool Visible()
        {
            return shop != null && shop.gameObject.activeInHierarchy && shop.isOpen && shop.simpleGrid != null;
        }

        void OnEnable()
        {
            if (shop != null) Apply();
        }

        void OnDisable()
        {
            StopAllCoroutines();
            DisposePending();
            loading = false;
            Selecting = false;
            DrawInFlight = false;
            foreach (ShopWorldEntry entry in entries) if (entry != null) entry.gameObject.SetActive(false);
        }

        void OnDestroy()
        {
            StopAllCoroutines();
            DisposePending();
        }

        void LateUpdate()
        {
            if (shop == null || !Visible()) return;
            if (!OwnerCurrent())
            {
                ResetOwner();
                Reapply(shop);
                return;
            }
            // Rebind after the original shop rebuilds its GameObject-valued dictionary.
            Dictionary<ShopType, GameObject> buttons = TabBtns();
            if (buttons == null || !buttons.ContainsKey(ShopType.KingsRecruit)) return;
            if (template == null || template.gameObject != buttons[ShopType.KingsRecruit]) { Reapply(shop); return; }
            if (DrawInFlight && Time.realtimeSinceStartup >= DrawDeadline) { DrawInFlight = false; Notice = "抽卡结果待确认，请重新打开王者招募"; }
            if (!catalogLoaded && !loading && Time.realtimeSinceStartup >= nextCatalog) StartCoroutine(RefreshCatalog());
            if (catalogLoaded) Highlight();
        }

        void DisposePending()
        {
            if (pending != null) { pending.Dispose(); pending = null; }
        }

        public bool HasWorld(int space)
        {
            return WorldOf(space) != null;
        }

        public EntryWorld WorldOf(int space)
        {
            foreach (EntryWorld world in worlds) if (world.Space == space) return world;
            return null;
        }

        public int WorldOfPool(string pool)
        {
            if (String.IsNullOrEmpty(pool)) return 0;
            foreach (EntryWorld world in worlds) if (world.Pool == pool) return world.Space;
            return 0;
        }

        public int ResolvedWorld
        {
            get
            {
                if (RequestedWorld > 0) return RequestedWorld;
                return WorldOfPool(SelectedPool);
            }
        }

        public void OnEntryClick(int space)
        {
            if (!Visible() || !OwnerCurrent()) return;
            if (Selecting) { Notice = "世界切换中，请稍候"; return; }
            if (DrawInFlight) { Notice = "抽卡请求进行中，请稍候"; return; }
            if (WorldOf(space) == null) { Notice = "该世界当前未开放"; return; }
            Notice = "";
            RequestedWorld = space;
            StartCoroutine(Select(space));
        }

        // One retry path serves both a failed catalog and a failed selection.
        public void Retry()
        {
            if (!Visible() || !OwnerCurrent()) return;
            catalogLoaded = false;
            nextCatalog = 0;
            StartCoroutine(RetryFlow());
        }

        IEnumerator RetryFlow()
        {
            yield return StartCoroutine(RefreshCatalog());
            if (catalogLoaded && RequestedWorld > 0 && WorldOf(RequestedWorld) != null && SelectedPool != WorldOf(RequestedWorld).Pool)
                yield return StartCoroutine(Select(RequestedWorld));
        }

        IEnumerator RefreshCatalog()
        {
            if (loading || shop == null) yield break;
            if (!OwnerCurrent()) yield break;
            loading = true;
            nextCatalog = Time.realtimeSinceStartup + 20f;
            string origin = KingRecruitSession.Origin();
            if (origin == null) { loading = false; Notice = "世界加载失败，点击重试"; yield break; }
            string body = "{\"playerId\":" + owner + ",\"sessionHex\":\"" + session + "\"}";
            Dictionary<string, string> headers = new Dictionary<string, string>();
            headers.Add("Content-Type", "application/json");
            WWW request = new WWW(origin + "/king-recruit/worlds", Encoding.UTF8.GetBytes(body), headers);
            pending = request;
            float deadline = Time.realtimeSinceStartup + 15f;
            while (!request.isDone && Time.realtimeSinceStartup < deadline) yield return null;
            bool success = false;
            try
            {
                if (request.isDone && String.IsNullOrEmpty(request.error) && OwnerCurrent() && Visible())
                {
                    JsonData response = JsonMapper.ToObject(request.text);
                    if ((int)response["code"] == 0)
                    {
                        JsonData data = response["data"];
                        Version = (string)data["version"];
                        SelectedPool = (string)data["selectedPoolId"];
                        worlds.Clear();
                        foreach (JsonData row in data["worlds"])
                        {
                            EntryWorld world = new EntryWorld();
                            world.Pool = (string)row["poolId"];
                            world.Label = (string)row["label"];
                            world.Space = (int)row["world"];
                            if (world.Space > 0 && world.Space <= 3) worlds.Add(world);
                        }
                        catalogLoaded = true;
                        if (RequestedWorld > 0 && WorldOf(RequestedWorld) == null) RequestedWorld = WorldOfPool(SelectedPool);
                        success = true;
                    }
                }
            }
            catch (Exception) { }
            request.Dispose();
            if (pending == request) pending = null;
            loading = false;
            if (!OwnerCurrent() || !Visible()) yield break;
            if (!success) { Notice = "世界加载失败，点击重试"; yield break; }
            Notice = "";
            Apply();
        }

        IEnumerator Select(int space)
        {
            Selecting = true;
            nextCatalog = 0;
            EntryWorld world = WorldOf(space);
            string origin = KingRecruitSession.Origin();
            if (world == null || origin == null)
            {
                Selecting = false;
                Notice = "世界选择失败，点击重试";
                yield break;
            }
            string pool = world.Pool;
            string body = "{\"playerId\":" + owner + ",\"sessionHex\":\"" + session + "\",\"poolId\":" + JsonMapper.ToJson(pool) + ",\"version\":" + JsonMapper.ToJson(Version) + "}";
            Dictionary<string, string> headers = new Dictionary<string, string>();
            headers.Add("Content-Type", "application/json");
            WWW request = new WWW(origin + "/king-recruit/select", Encoding.UTF8.GetBytes(body), headers);
            pending = request;
            float deadline = Time.realtimeSinceStartup + 15f;
            while (!request.isDone && Time.realtimeSinceStartup < deadline) yield return null;
            bool success = false;
            try
            {
                if (request.isDone && String.IsNullOrEmpty(request.error) && OwnerCurrent() && Visible())
                {
                    JsonData response = JsonMapper.ToObject(request.text);
                    if ((int)response["code"] == 0)
                    {
                        JsonData data = response["data"];
                        SelectedPool = (string)data["selectedPoolId"];
                        string version = (string)data["version"];
                        if (!String.IsNullOrEmpty(version)) Version = version;
                        success = SelectedPool == pool;
                    }
                }
            }
            catch (Exception) { }
            request.Dispose();
            if (pending == request) pending = null;
            Selecting = false;
            if (!OwnerCurrent() || !Visible()) yield break;
            if (!success) { Notice = "世界选择失败，点击重试"; yield break; }
            RequestedWorld = space;
            Notice = "";
            Apply();
            // Only after the server accepted the world does the original tab reopen with the same king content.
            shop.OnTabBtn(ShopType.KingsRecruit);
        }

        void Apply()
        {
            if (shop == null || shop.simpleGrid == null || template == null) return;
            for (int space = 1; space <= 3; space++)
            {
                ShopWorldEntry entry = Entry(space);
                if (entry == null) continue;
                ShopTabButton button = entry.GetComponent<ShopTabButton>();
                bool open = HasWorld(space);
                entry.gameObject.SetActive(open);
                if (button != null && button.labName != null)
                    button.labName.text = "王者招募·" + (space == 1 ? "SAO" : space == 2 ? "ALO" : "GGO");
                if (button != null && button.chosedBg != null) button.chosedBg.SetActive(false);
                if (button != null && button.objPointer != null) button.objPointer.SetActive(false);
            }
            Order();
            Highlight();
        }

        ShopWorldEntry Entry(int space)
        {
            while (entries.Count < 3) entries.Add(null);
            ShopWorldEntry entry = entries[space - 1];
            if (entry != null) return entry;
            if (space == 1)
            {
                entry = template.GetComponent<ShopWorldEntry>();
                if (entry == null) entry = template.gameObject.AddComponent<ShopWorldEntry>();
                entry.Menu = this;
                entry.Space = 1;
                entries[0] = entry;
                return entry;
            }
            if (shop.tabBtnPrefab == null) return null;
            GameObject clone = (GameObject)UnityEngine.Object.Instantiate((UnityEngine.Object)shop.tabBtnPrefab);
            clone.name = "ShopWorld" + space;
            clone.transform.parent = shop.simpleGrid.transform;
            clone.transform.localScale = Vector3.one;
            clone.transform.localRotation = Quaternion.identity;
            clone.SetActive(true);
            UIDragScrollView drag = clone.GetComponent<UIDragScrollView>();
            if (drag != null) drag.scrollView = shop.scrollView;
            ShopTabButton button = clone.GetComponent<ShopTabButton>();
            if (button == null) { UnityEngine.Object.Destroy(clone); return null; }
            // Same original button template, font and sprites; the original type-1 callback is removed.
            button.SetType(ShopType.KingsRecruit);
            button.m_callback = null;
            entry = clone.AddComponent<ShopWorldEntry>();
            entry.Menu = this;
            entry.Space = space;
            entries[space - 1] = entry;
            return entry;
        }

        // Keep SAO/ALO/GGO contiguous where the original king entry was, independent of curTab.
        void Order()
        {
            if (shop == null || shop.simpleGrid == null || template == null) return;
            List<GameObject> childs = shop.simpleGrid.childs;
            if (childs == null) return;
            int at = childs.IndexOf(template.gameObject);
            if (at < 0) at = childs.Count;
            for (int space = 1; space <= 3; space++)
            {
                ShopWorldEntry entry = entries.Count >= 3 ? entries[space - 1] : null;
                if (entry != null) childs.Remove(entry.gameObject);
            }
            int insert = Math.Min(at, childs.Count);
            for (int space = 1; space <= 3; space++)
            {
                ShopWorldEntry entry = entries.Count >= 3 ? entries[space - 1] : null;
                if (entry == null || !entry.gameObject.activeSelf) continue;
                childs.Insert(insert++, entry.gameObject);
            }
            shop.simpleGrid.Reposition();
        }

        void Highlight()
        {
            int active = ResolvedWorld;
            if (entries.Count < 3) return;
            for (int space = 1; space <= 3; space++)
            {
                ShopWorldEntry entry = entries[space - 1];
                if (entry == null) continue;
                ShopTabButton button = entry.GetComponent<ShopTabButton>();
                if (button == null) continue;
                bool on = space == active;
                if (button.chosedBg != null) button.chosedBg.SetActive(on);
                if (button.objPointer != null) button.objPointer.SetActive(on);
            }
        }
    }

    public sealed class ShopWorldEntry : MonoBehaviour
    {
        public ShopWorldMenuControl Menu;
        public int Space;

        // SimpleGrid dispatches "OnClick" to the child GameObject.
        public void OnClick()
        {
            if (Menu != null) Menu.OnEntryClick(Space);
        }
    }

    internal static class KingRecruitSession
    {
        public static string Origin()
        {
            try { return new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority); }
            catch (Exception) { return null; }
        }

        public static string Hex(byte[] bytes)
        {
            if (bytes == null) return "";
            StringBuilder value = new StringBuilder(bytes.Length * 2);
            foreach (byte b in bytes) value.Append(b.ToString("x2"));
            return value.ToString();
        }

        public static bool Current(out long owner, out string session)
        {
            owner = 0;
            session = "";
            if (StaticData.playerData == null || StaticData.sessionKey == null || StaticData.sessionKey.Length != 16) return false;
            owner = StaticData.playerData.playerId;
            session = Hex(StaticData.sessionKey);
            return true;
        }
    }
}
