using System;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using NetWork.Layer;
using NetWork.Packets;
using ProtoBuf.Message;
using UI;
using UnityEngine;

namespace HYJS.GGO
{
    // The trial shop keeps its original four tabs. Only the goods tab uses
    // the native MemoryShop cards and refresh controls.
    public sealed class TrialShopRefresh75 : MonoBehaviour
    {
        const int TrialType = 5;
        const float RefreshOffset = 64f;
        static readonly BindingFlags Flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        static readonly FieldInfo CurrentTab = typeof(ShopPage).GetField("curTab", Flags);
        static readonly FieldInfo CurrentGroup = typeof(TabPointShop).GetField("curTabIndex", Flags);
        static readonly FieldInfo Items = typeof(TabPointShop).GetField("shopItems", Flags);
        static readonly MethodInfo UpdateProducts = typeof(TabPointShop).GetMethod("UpdateProducts", Flags);
        static readonly MethodInfo GetRightTab = typeof(ShopPage).GetMethod("GetRightTab", Flags);
        static readonly string[] LayoutFields = { "arrangement", "maxPerLine", "cellWidth", "cellHeight", "paddingX", "paddingY" };
        static Pending pending;

        sealed class Pending
        {
            public UIPlayerData player;
            public long playerId;
            public string session, origin;
            public int generation;
            public bool finished;
            public HttpHandler handler;
        }

        ShopPage shop;
        TabPointShop tab;
        UIPlayerData player;
        long playerId;
        string session = "", origin = "";
        int generation, mode, group = -1;
        bool queryWanted, layoutSaved;
        float nextQueryAt, nextClockAt;
        int freeLeft, freeMax, resLeft, resMax;
        long nextUpdate;
        object[] savedLayout;
        Vector3 savedPosition;
        TabMemoryShop memoryPrefab;
        TabPopularityShop dailyPrefab;
        GameObject refreshRoot, refreshButton, countdownRoot;
        UILabel refreshInfo, refreshCost, countdown;
        UIButton refreshControl;
        readonly List<TrialRefreshItem82> rows = new List<TrialRefreshItem82>();
        readonly List<TrialDailyItem> dailyRows = new List<TrialDailyItem>();

        public static void Ensure(ShopPage page)
        {
            if (page == null) return;
            TrialShopRefresh75 control = page.GetComponent<TrialShopRefresh75>();
            if (control == null) control = page.gameObject.AddComponent<TrialShopRefresh75>();
            control.shop = page;
        }

        static bool Current(out UIPlayerData data, out long id, out string key, out string server)
        {
            data = StaticData.playerData; id = data == null ? 0 : data.playerId;
            key = server = "";
            if (id < 1 || StaticData.sessionKey == null || StaticData.sessionKey.Length != 16) return false;
            StringBuilder bytes = new StringBuilder(32);
            foreach (byte b in StaticData.sessionKey) bytes.Append(b.ToString("x2"));
            key = bytes.ToString();
            try { server = new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority); }
            catch (Exception) { return false; }
            return server.Length > 0;
        }

        static bool SamePlayer(Pending request)
        {
            UIPlayerData data; long id; string key, server;
            return request != null && Current(out data, out id, out key, out server) &&
                System.Object.ReferenceEquals(data, request.player) && id == request.playerId &&
                key == request.session && server == request.origin;
        }

        static bool Busy()
        {
            if (pending != null && !SamePlayer(pending)) pending = null;
            return pending != null;
        }

        bool SamePage()
        {
            UIPlayerData data; long id; string key, server;
            return shop != null && tab != null && tab.gameObject.activeInHierarchy && shop.gameObject.activeInHierarchy &&
                CurrentTab != null && (CurrentTab.GetValue(shop) as GameObject) == tab.gameObject &&
                Current(out data, out id, out key, out server) && System.Object.ReferenceEquals(data, player) &&
                id == playerId && key == session && server == origin;
        }

        void Attach(TabPointShop page)
        {
            UIPlayerData data; long id; string key, server;
            if (!Current(out data, out id, out key, out server)) return;
            if (tab == page && System.Object.ReferenceEquals(player, data) && playerId == id && session == key && origin == server) return;
            Detach();
            tab = page; shop = page.shopPage; player = data; playerId = id; session = key; origin = server;
            mode = 0; group = -1; nextQueryAt = 0f;
        }

        void Detach()
        {
            generation++;
            RestoreLayout();
            if (refreshRoot != null) UnityEngine.Object.Destroy(refreshRoot);
            refreshRoot = refreshButton = countdownRoot = null;
            refreshInfo = refreshCost = countdown = null; refreshControl = null;
            memoryPrefab = null; dailyPrefab = null; tab = null; player = null; playerId = 0; session = origin = "";
            mode = 0; group = -1; queryWanted = false; nextUpdate = 0; rows.Clear(); dailyRows.Clear();
        }

        void OnDisable() { Detach(); }
        void OnDestroy() { Detach(); }

        // Entry hooks in the frozen 80 Assembly-CSharp call these two methods.
        public static bool HandleUpdateAll(TabPointShop page)
        {
            if (page == null || page.shopPage == null) return false;
            Ensure(page.shopPage);
            TrialShopRefresh75 control = page.shopPage.GetComponent<TrialShopRefresh75>();
            control.Attach(page);
            if (control.tab != page || control.mode == 1) return false;
            control.queryWanted = true;
            control.TryQuery();
            return true;
        }

        public static bool HandleProducts(TabPointShop page)
        {
            if (page == null || page.shopPage == null) return false;
            Ensure(page.shopPage);
            TrialShopRefresh75 control = page.shopPage.GetComponent<TrialShopRefresh75>();
            control.Attach(page);
            if (control.tab != page || control.mode == 1) return false;
            int selected = CurrentGroup == null ? -1 : Convert.ToInt32(CurrentGroup.GetValue(page));
            int previous = control.group; control.group = selected;
            if (selected != 0)
            {
                control.RestoreLayout();
                if (control.refreshRoot != null) control.refreshRoot.SetActive(false);
                if (Items == null || Items.GetValue(page) == null) { control.ClearGrid(); return true; }
                return false;
            }
            if (control.mode != 2 && control.mode != 3)
            {
                control.ClearGrid(); control.queryWanted = true; control.TryQuery(); return true;
            }
            if (previous > 0 && !Busy()) control.queryWanted = true;
            bool rendered = control.Render();
            control.TryQuery();
            return rendered;
        }

        void Update()
        {
            if (tab == null) return;
            if (!SamePage()) { Detach(); return; }
            if ((mode == 2 || mode == 3) && group == 0 && Time.realtimeSinceStartup >= nextClockAt)
            {
                nextClockAt = Time.realtimeSinceStartup + 1f;
                PaintRefresh();
                if ((mode == 3 || freeLeft < freeMax) && nextUpdate > 0 && ServerTimerTool.CurrentUTCJavaTime() >= nextUpdate)
                    queryWanted = true;
            }
            TryQuery();
            PaintButtons();
        }

        Pending Begin()
        {
            if (!SamePage() || Busy()) return null;
            Pending p = new Pending(); p.player = player; p.playerId = playerId; p.session = session; p.origin = origin; p.generation = generation;
            pending = p; return p;
        }

        static void Finish(Pending p)
        {
            if (p == null || p.finished) return;
            p.finished = true;
            if (System.Object.ReferenceEquals(pending, p)) pending = null;
        }

        bool Owns(Pending p) { return p != null && p.generation == generation && SamePlayer(p) && SamePage(); }

        void TryQuery()
        {
            if (!queryWanted || !SamePage() || mode == 1 || Busy() || Time.realtimeSinceStartup < nextQueryAt) return;
            Pending p = Begin(); if (p == null) return;
            p.handler = delegate(Packet packet, bool ok)
            {
                if (p.finished) return;
                bool owned = Owns(p); Finish(p);
                if (!owned) return;
                SCRefreshShopQuery reply = ok && packet != null ? packet.kBody as SCRefreshShopQuery : null;
                if (reply == null) return;
                bool daily = reply.freeMaxTimes == 0 && reply.resMaxTimes == 0 && reply.nextUpdateTime > 0;
                if (reply.shopType != TrialType || reply.freeMaxTimes == 0 && reply.resMaxTimes == 0 && !daily)
                {
                    mode = 1; queryWanted = false; RestoreLayout();
                    if (refreshRoot != null) refreshRoot.SetActive(false);
                    tab.UpdateAll(); return;
                }
                if (reply.freeLeftTimes < 0 || reply.freeMaxTimes < reply.freeLeftTimes ||
                    reply.resLeftTimes < 0 || reply.resMaxTimes < reply.resLeftTimes || reply.shopItemList == null) return;
                int nextMode = daily ? 3 : 2;
                if (mode != nextMode) RestoreLayout();
                mode = nextMode; freeLeft = reply.freeLeftTimes; freeMax = reply.freeMaxTimes;
                resLeft = reply.resLeftTimes; resMax = reply.resMaxTimes; nextUpdate = reply.nextUpdateTime;
                ApplyItems(reply.shopItemList);
            };
            queryWanted = false; nextQueryAt = Time.realtimeSinceStartup + 1f;
            bool sent = false;
            try { sent = NetAPI.SendHttp(24503, new CSRefreshShopQuery { shopType = TrialType }, p.handler); }
            catch (Exception) { }
            if (!sent) { Finish(p); queryWanted = true; }
        }

        void ApplyItems(List<ShopItem> items)
        {
            if (!SamePage() || Items == null || UpdateProducts == null || items == null) return;
            Items.SetValue(tab, items);
            UpdateProducts.Invoke(tab, null);
            PaintRefresh(); PaintButtons();
        }

        static string RelativePath(Transform root, Transform child)
        {
            string path = "";
            while (child != root)
            {
                if (child == null) throw new InvalidOperationException("native refresh hierarchy changed");
                path = child.name + (path.Length == 0 ? "" : "/" + path); child = child.parent;
            }
            return path;
        }

        static void RemoveLocalization(UILabel label)
        {
            if (label == null) return;
            foreach (MonoBehaviour component in label.GetComponents<MonoBehaviour>())
                if (component != null && component.GetType().Name == "TextKit") UnityEngine.Object.DestroyImmediate(component);
        }

        bool PrepareVisual()
        {
            if (memoryPrefab != null && refreshRoot != null) return true;
            GameObject temporary = null;
            try
            {
                GameObject asset = AssetLoad.ResourceManager.Instance.LoadResource("ShopInnerPage/MemoryShop") as GameObject;
                memoryPrefab = asset == null ? null : asset.GetComponent<TabMemoryShop>();
                if (memoryPrefab == null || memoryPrefab.simpleGrid == null || memoryPrefab.shopItemPrefab == null || GetRightTab == null) return false;
                temporary = (GameObject)GetRightTab.Invoke(shop, new object[] { "ShopInnerPage/MemoryShop" });
                temporary.SetActive(false);
                TabMemoryShop native = temporary.GetComponent<TabMemoryShop>();
                Transform button = null;
                foreach (Transform value in temporary.GetComponentsInChildren<Transform>(true))
                    if (value.name == "BtnRefresh") { if (button != null) throw new InvalidOperationException("ambiguous native refresh"); button = value; }
                if (native == null || button == null || native.labLeftRefreshTimes == null || native.labRefreshCost == null ||
                    native.freeRefreshCDRoot == null || native.labCountDown == null) throw new InvalidOperationException("native refresh unavailable");
                Transform source = button.parent;
                string buttonPath = RelativePath(source, button);
                string infoPath = RelativePath(source, native.labLeftRefreshTimes.transform);
                string costPath = RelativePath(source, native.labRefreshCost.transform);
                string cdRootPath = RelativePath(source, native.freeRefreshCDRoot.transform);
                string countPath = RelativePath(source, native.labCountDown.transform);
                refreshRoot = (GameObject)UnityEngine.Object.Instantiate(source.gameObject);
                refreshRoot.name = "TrialNativeRefresh";
                refreshRoot.transform.parent = tab.transform;
                refreshRoot.transform.localPosition = temporary.transform.InverseTransformPoint(source.position) - new Vector3(0f, RefreshOffset, 0f);
                refreshRoot.transform.localRotation = Quaternion.Inverse(temporary.transform.rotation) * source.rotation;
                Vector3 scale = source.lossyScale, parentScale = temporary.transform.lossyScale;
                refreshRoot.transform.localScale = new Vector3(scale.x / parentScale.x, scale.y / parentScale.y, scale.z / parentScale.z);
                refreshButton = refreshRoot.transform.Find(buttonPath).gameObject;
                refreshInfo = refreshRoot.transform.Find(infoPath).GetComponent<UILabel>();
                refreshCost = refreshRoot.transform.Find(costPath).GetComponent<UILabel>();
                countdownRoot = refreshRoot.transform.Find(cdRootPath).gameObject;
                countdown = refreshRoot.transform.Find(countPath).GetComponent<UILabel>();
                RemoveLocalization(refreshInfo); RemoveLocalization(refreshCost); RemoveLocalization(countdown);
                foreach (UIButtonMessage old in refreshRoot.GetComponentsInChildren<UIButtonMessage>(true)) UnityEngine.Object.DestroyImmediate(old);
                foreach (UIEventListener old in refreshRoot.GetComponentsInChildren<UIEventListener>(true)) UnityEngine.Object.DestroyImmediate(old);
                refreshControl = refreshButton.GetComponent<UIButton>();
                if (refreshControl != null) { refreshControl.onClick.Clear(); EventDelegate.Set(refreshControl.onClick, Refresh); }
                else UIEventListener.Get(refreshButton).onClick = delegate(GameObject ignored) { Refresh(); };
                refreshRoot.SetActive(false); return true;
            }
            catch (Exception error)
            {
                Debug.LogWarning("Trial refresh prefab: " + error.GetType().Name);
                if (refreshRoot != null) UnityEngine.Object.Destroy(refreshRoot);
                refreshRoot = refreshButton = countdownRoot = null; memoryPrefab = null;
                return false;
            }
            finally { if (temporary != null) UnityEngine.Object.Destroy(temporary); }
        }

        void ApplyLayout()
        {
            SimpleGrid source = mode == 3 ? (dailyPrefab == null ? null : dailyPrefab.simpleGrid) : (memoryPrefab == null ? null : memoryPrefab.simpleGrid);
            if (layoutSaved || tab == null || tab.simpleGrid == null || source == null) return;
            savedLayout = new object[LayoutFields.Length];
            for (int i = 0; i < LayoutFields.Length; i++)
            {
                FieldInfo field = typeof(SimpleGrid).GetField(LayoutFields[i], Flags);
                if (field == null) throw new InvalidOperationException("native grid field missing");
                savedLayout[i] = field.GetValue(tab.simpleGrid); field.SetValue(tab.simpleGrid, field.GetValue(source));
            }
            savedPosition = tab.simpleGrid.transform.localPosition;
            tab.simpleGrid.transform.localPosition = mode == 3 ? savedPosition : savedPosition - new Vector3(0f, RefreshOffset, 0f);
            layoutSaved = true;
        }

        void RestoreLayout()
        {
            if (!layoutSaved) return;
            if (tab != null && tab.simpleGrid != null)
            {
                for (int i = 0; i < LayoutFields.Length; i++)
                {
                    FieldInfo field = typeof(SimpleGrid).GetField(LayoutFields[i], Flags);
                    if (field != null) field.SetValue(tab.simpleGrid, savedLayout[i]);
                }
                tab.simpleGrid.transform.localPosition = savedPosition;
            }
            layoutSaved = false; savedLayout = null;
        }

        void ClearGrid()
        {
            rows.Clear(); dailyRows.Clear();
            if (tab == null || tab.simpleGrid == null) return;
            foreach (GameObject child in new List<GameObject>(tab.simpleGrid.childs)) if (child != null) UnityEngine.Object.Destroy(child);
            tab.simpleGrid.childs.Clear();
        }

        bool Render()
        {
            if (mode == 3) return RenderDaily();
            if (!SamePage() || Items == null || !PrepareVisual()) return false;
            List<ShopItem> all = Items.GetValue(tab) as List<ShopItem>;
            if (all == null) { ClearGrid(); return true; }
            List<ShopItem> goods = new List<ShopItem>();
            foreach (ShopItem item in all) if (item != null && item.priority >= 1000 && item.priority < 2000) goods.Add(item);
            goods.Sort(delegate(ShopItem a, ShopItem b) { return a.priority.CompareTo(b.priority); });
            ClearGrid(); ApplyLayout();
            foreach (ShopItem item in goods)
            {
                if (item.item == null || item.item.Count == 0 || item.exchangeList == null || item.exchangeList.Count == 0) continue;
                GameObject clone = (GameObject)UnityEngine.Object.Instantiate(memoryPrefab.shopItemPrefab);
                ShopMemoryItem native = clone.GetComponent<ShopMemoryItem>();
                if (native == null) { UnityEngine.Object.Destroy(clone); continue; }
                native.SetShopMemoryItem(item);
                native.labPriceName.text = TableTextBlo.GetText(CommonMethod.GetObjectNameTextId(item.exchangeList[0].id));
                native.costLogo.spriteName = item.exchangeList[0].id == "point" ? "UI_item_08" : native.costLogo.spriteName;
                RemoveLocalization(native.labPriceName);
                TrialRefreshItem82 row = clone.AddComponent<TrialRefreshItem82>();
                row.Bind(this, native, item, tab.shopItemPrefab);
                clone.SetActive(true); tab.simpleGrid.AddChild(clone, 1f); rows.Add(row);
            }
            tab.simpleGrid.Reposition();
            if (tab.scrollView != null) tab.scrollView.ResetPosition();
            refreshRoot.SetActive(true); PaintRefresh(); PaintButtons(); return true;
        }

        bool RenderDaily()
        {
            if (!SamePage() || Items == null) return false;
            if (dailyPrefab == null)
            {
                GameObject asset = AssetLoad.ResourceManager.Instance.LoadResource("ShopInnerPage/PopularityShop") as GameObject;
                dailyPrefab = asset == null ? null : asset.GetComponent<TabPopularityShop>();
                if (dailyPrefab == null || dailyPrefab.shopItemPrefab == null || dailyPrefab.simpleGrid == null)
                    throw new InvalidOperationException("native popularity shop unavailable");
            }
            List<ShopItem> all = Items.GetValue(tab) as List<ShopItem>;
            List<ShopItem> goods = new List<ShopItem>();
            if (all != null) foreach (ShopItem item in all)
                if (item != null && item.priority >= 1000 && item.priority < 2000) goods.Add(item);
            goods.Sort(delegate(ShopItem a, ShopItem b) { return a.priority.CompareTo(b.priority); });
            ClearGrid(); ApplyLayout();
            if (refreshRoot != null) refreshRoot.SetActive(false);
            foreach (ShopItem item in goods)
            {
                GameObject clone = (GameObject)UnityEngine.Object.Instantiate(dailyPrefab.shopItemPrefab);
                ShopPopularityItem native = clone.GetComponent<ShopPopularityItem>();
                if (native == null) throw new InvalidOperationException("native popularity item unavailable");
                native.SetPopularityItem(item, shop, false);
                UILabel currency = clone.transform.Find("Price/popularity").GetComponent<UILabel>();
                UISprite logo = clone.transform.Find("Price/logo").GetComponent<UISprite>();
                UILabel caption = clone.transform.Find("Price/WhiteBlock/Label").GetComponent<UILabel>();
                RemoveLocalization(currency); RemoveLocalization(caption);
                currency.text = TableTextBlo.GetText(CommonMethod.GetObjectNameTextId("point"));
                currency.width = 74; currency.overflowMethod = UILabel.Overflow.ShrinkContent;
                currency.transform.localPosition = new Vector3(-46f, 0f, 0f);
                logo.spriteName = "UI_item_08";
                logo.transform.localPosition = new Vector3(-60f, 0f, 0f);
                caption.text = "消耗";
                if (native.labCondition != null && item.condition != null && item.condition.type == "star")
                {
                    UILabel terms = native.labCondition;
                    RemoveLocalization(terms);
                    Transform quota = clone.transform.Find("DailyQuota66");
                    if (quota != null) quota.gameObject.SetActive(false);
                    terms.text = "需" + item.condition.value + "星 今日剩余" +
                        Math.Max(0, item.dayLeftTimes) + "/" + Math.Max(0, item.dayTotalTimes);
                    terms.width = 178; terms.height = 30; terms.fontSize = 17; terms.maxLineCount = 1;
                    terms.overflowMethod = UILabel.Overflow.ShrinkContent;
                    terms.color = native.labName.color;
                    terms.transform.localPosition = new Vector3(-146f, -44f, 0f);
                    terms.gameObject.SetActive(true);
                }
                TrialDailyItem row = clone.AddComponent<TrialDailyItem>();
                row.Bind(this, native, item);
                clone.SetActive(true); tab.simpleGrid.AddChild(clone, 1f); dailyRows.Add(row);
            }
            tab.simpleGrid.Reposition();
            if (tab.scrollView != null) tab.scrollView.ResetPosition();
            PaintButtons(); return true;
        }

        void PaintRefresh()
        {
            if (refreshRoot == null || mode != 2 || group != 0) return;
            refreshInfo.text = resLeft + "/" + resMax;
            if (freeLeft > 0) refreshCost.text = TableTextBlo.GetText("for_free") + "(" + freeLeft + "/" + freeMax + ")";
            else if (player.GetSimpleItemCount("item_013") > 0) refreshCost.text = TableTextBlo.GetText("name_item_013") + "x1";
            else refreshCost.text = TableTextBlo.GetText("diamond") + "x20";
            long seconds = (nextUpdate - ServerTimerTool.CurrentUTCJavaTime()) / 1000L;
            bool show = freeLeft < freeMax && seconds > 0;
            countdownRoot.SetActive(show);
            if (show) countdown.text = (seconds / 3600L).ToString("D2") + ":" + ((seconds / 60L) % 60L).ToString("D2") + ":" + (seconds % 60L).ToString("D2");
        }

        void PaintButtons()
        {
            bool enabled = mode == 2 && group == 0 && SamePage() && !Busy();
            if (refreshControl != null) refreshControl.isEnabled = enabled;
            foreach (TrialRefreshItem82 row in rows) if (row != null) row.SetEnabled(enabled);
            bool dailyEnabled = mode == 3 && group == 0 && SamePage() && !Busy();
            foreach (TrialDailyItem row in dailyRows) if (row != null) row.SetEnabled(dailyEnabled);
        }

        void Refresh()
        {
            if (!SamePage() || mode != 2 || group != 0 || Busy()) return;
            if (freeLeft <= 0 && resLeft <= 0) { BoxManager.CreatePopupTextBox(TableTextBlo.GetText("res_refresh_times_useoff"), 0f); return; }
            if (freeLeft <= 0 && player.GetSimpleItemCount("item_013") < 1 && player.diamond < 20)
            { BoxManager.CreatePopupTextBox(TableTextBlo.GetText("diamond_lack"), 0f); return; }
            Pending p = Begin(); if (p == null) return;
            p.handler = delegate(Packet packet, bool ok)
            {
                if (p.finished) return;
                bool samePlayer = SamePlayer(p), owned = Owns(p); Finish(p);
                SCRefreshShopRefresh reply = ok && packet != null ? packet.kBody as SCRefreshShopRefresh : null;
                if (!samePlayer || reply == null || reply.shopType != TrialType) return;
                if (reply.cost != null) p.player.Delete(reply.cost);
                if (!owned) return;
                freeLeft = reply.freeLeftTimes; freeMax = reply.freeMaxTimes;
                resLeft = reply.resLeftTimes; resMax = reply.resMaxTimes; nextUpdate = reply.nextUpdateTime;
                ApplyItems(reply.shopItemList);
            };
            bool sent = false;
            try { sent = NetAPI.SendHttp(24501, new CSRefreshShopRefresh { shopType = TrialType }, p.handler); }
            catch (Exception) { }
            if (!sent) Finish(p);
            PaintButtons();
        }

        internal void Buy(ShopItem item)
        {
            if (!SamePage() || mode != 2 && mode != 3 || group != 0 || Busy() || item == null || item.canBuy != 1) return;
            List<ShopItem> current = Items.GetValue(tab) as List<ShopItem>;
            if (current == null || !current.Contains(item) || item.exchangeList == null || item.exchangeList.Count != 1) return;
            BaseResource price = item.exchangeList[0];
            if (price.id == "point" && player.strategyScore < price.num)
            { BoxManager.CreatePopupTextBox(TableTextBlo.GetText("no_enough_point"), 0f); return; }
            Pending p = Begin(); if (p == null) return;
            p.handler = delegate(Packet packet, bool ok)
            {
                if (p.finished) return;
                bool samePlayer = SamePlayer(p), owned = Owns(p); Finish(p);
                SCShopBuy reply = ok && packet != null ? packet.kBody as SCShopBuy : null;
                if (!samePlayer || reply == null || reply.shopType != TrialType) return;
                if (reply.cost != null) p.player.Delete(reply.cost);
                if (reply.get != null) p.player.AddResultObj(reply.get);
                if (!owned) return;
                BoxManager.CreatePopupTextBox(TableTextBlo.GetText("buy_success"), 0f);
                ApplyItems(reply.shopItemList);
            };
            bool sent = false;
            try { sent = NetAPI.SendHttp(23007, new CSShopBuy { shopType = TrialType, shopItemId = item.id, buyCount = 1 }, p.handler); }
            catch (Exception) { }
            if (!sent) Finish(p);
            PaintButtons();
        }
    }

    public sealed class TrialDailyItem : MonoBehaviour
    {
        TrialShopRefresh75 owner;
        ShopItem item;
        UIButton control;
        Collider collider;

        public void Bind(TrialShopRefresh75 target, ShopPopularityItem native, ShopItem product)
        {
            owner = target; item = product;
            GameObject button = native.btnBuy;
            foreach (UIButtonMessage old in button.GetComponents<UIButtonMessage>()) UnityEngine.Object.DestroyImmediate(old);
            foreach (UIEventListener old in button.GetComponents<UIEventListener>()) UnityEngine.Object.DestroyImmediate(old);
            UnityEngine.Object.DestroyImmediate(native);
            control = button.GetComponent<UIButton>(); collider = button.GetComponent<Collider>();
            if (control != null) { control.onClick.Clear(); EventDelegate.Set(control.onClick, OnBuy); }
            else UIEventListener.Get(button).onClick = delegate(GameObject ignored) { OnBuy(); };
        }

        void OnBuy() { if (owner != null) owner.Buy(item); }
        public void SetEnabled(bool enabled)
        {
            enabled = enabled && item != null && item.canBuy == 1;
            if (control != null) control.isEnabled = enabled;
            if (collider != null) collider.enabled = enabled;
        }
    }

    public sealed class TrialRefreshItem82 : MonoBehaviour
    {
        TrialShopRefresh75 owner;
        ShopItem item;
        GameObject button;
        UIButton control;
        Collider collider;

        public void Bind(TrialShopRefresh75 target, ShopMemoryItem native, ShopItem product, GameObject pointPrefab)
        {
            owner = target; item = product; button = native.btnBuy;
            if (product.condition != null && product.condition.type == "star" && pointPrefab != null)
            {
                ShopPointItem point = pointPrefab.GetComponent<ShopPointItem>();
                if (point != null && point.conditionLabel != null)
                {
                    GameObject labelObject = (GameObject)UnityEngine.Object.Instantiate(point.conditionLabel.gameObject);
                    labelObject.name = "Condition"; labelObject.transform.parent = transform;
                    labelObject.transform.localPosition = point.conditionLabel.transform.localPosition;
                    labelObject.transform.localRotation = point.conditionLabel.transform.localRotation;
                    labelObject.transform.localScale = point.conditionLabel.transform.localScale;
                    UILabel label = labelObject.GetComponent<UILabel>();
                    foreach (MonoBehaviour component in label.GetComponents<MonoBehaviour>())
                        if (component != null && component.GetType().Name == "TextKit") UnityEngine.Object.DestroyImmediate(component);
                    label.text = String.Format(TableTextBlo.GetText("point_shop_item_condition"), product.condition.value);
                    labelObject.SetActive(true);
                }
            }
            foreach (UIButtonMessage old in button.GetComponents<UIButtonMessage>()) UnityEngine.Object.DestroyImmediate(old);
            foreach (UIEventListener old in button.GetComponents<UIEventListener>()) UnityEngine.Object.DestroyImmediate(old);
            UnityEngine.Object.DestroyImmediate(native);
            control = button.GetComponent<UIButton>(); collider = button.GetComponent<Collider>();
            if (control != null) { control.onClick.Clear(); EventDelegate.Set(control.onClick, OnBuy); }
            else UIEventListener.Get(button).onClick = delegate(GameObject ignored) { OnBuy(); };
        }

        void OnBuy() { if (owner != null) owner.Buy(item); }
        public void SetEnabled(bool enabled)
        {
            if (control != null) control.isEnabled = enabled;
            if (collider != null) collider.enabled = enabled;
        }
    }
}
