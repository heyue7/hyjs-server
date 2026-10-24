using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using LitJson;
using ProtoBuf.Message;
using UI;
using UnityEngine;

namespace HYJS.GGO
{
    // Requests and pending receipts belong to one role and server origin.
    public sealed class GGOShop : MonoBehaviour
    {
        const int GGOType = 11;
        static readonly BindingFlags Flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        ShopPage shop;
        GameObject entry, content;
        TabScoreShop layout;
        UILabel balanceLabel;
        TopBarItem balanceItem;
        CommonIcon balanceIcon;
        GameObject refreshButton; UILabel refreshInfo; UIButton refreshControl;
        bool refreshEnabled; long freeRefreshLeft, paidRefreshLeft, refreshCost; string refreshCurrency = "";
        const string RefreshProduct = "__refresh__";
        long owner;
        string session = "", origin = "", version = "", currency = "item_9710", currencyName = "信用点";
        string pendingId = "", pendingProduct = "", pendingVersion = "", confirmedNotice = "";
        string inventoryRefreshResource = "";
        bool selected, busy, enabledShop, restoreTop, pendingUnreadable, inventoryRefreshPending;
        int pendingCount = 1;
        long balance, catalogueRevision, inventorySnapshotGeneration;
        WWW request;
        readonly List<GGOProductRow> rows = new List<GGOProductRow>();

        static FieldInfo Field(Type type, string name) { return type.GetField(name, Flags); }
        static bool Current(out long player, out string key)
        {
            player = 0; key = "";
            if (StaticData.playerData == null || StaticData.sessionKey == null || StaticData.sessionKey.Length != 16) return false;
            player = StaticData.playerData.playerId;
            StringBuilder text = new StringBuilder(32);
            foreach (byte b in StaticData.sessionKey) text.Append(b.ToString("x2"));
            key = text.ToString();
            return player > 0;
        }
        static string Origin() { try { return new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority); } catch { return ""; } }
        bool SameOwner() { long id; string key; return Current(out id, out key) && id == owner && key == session && Origin() == origin; }
        bool Visible() { return selected && shop != null && shop.isOpen && gameObject.activeInHierarchy && content != null && content.activeInHierarchy && SameOwner(); }
        string PreferenceKey() { return "hyjs.ggo.pending." + Uri.EscapeDataString(origin) + "." + owner; }

        static bool assetRegistrationErrorLogged;
        // Old user manifests take precedence over APK manifests. Admit only our ten missing
        // bundle keys in memory, through the native parser; never persist or replace entries.
        static void RegisterIcons()
        {
            try
            {
                Type managerType = typeof(StaticData).Assembly.GetType("AssetLoad.AssetBundleManager", true);
                PropertyInfo instance = managerType.GetProperty("Instance", Flags | BindingFlags.Static | BindingFlags.FlattenHierarchy);
                object manager = instance == null ? null : instance.GetValue(null, null);
                if (manager == null) return;
                FieldInfo fullField = Field(managerType, "assetFileTool"), smallField = Field(managerType, "smallAssetFileTool");
                AssetFileTool full = fullField == null ? null : fullField.GetValue(manager) as AssetFileTool;
                AssetFileTool small = smallField == null ? null : smallField.GetValue(manager) as AssetFileTool;
                if (full == null || small == null || full.assetCells == null || small.assetCells == null) return;
                AssetFileTool additions = AssetFileTool.LoadFromString(GGOIconManifest.Text);
                if (additions == null || additions.assetCells.Count != 10) return;
                foreach (KeyValuePair<string, AssetCell> pair in additions.assetCells)
                {
                    if (!full.assetCells.ContainsKey(pair.Key)) full.assetCells.Add(pair.Key, pair.Value);
                    if (!small.assetCells.ContainsKey(pair.Key)) small.assetCells.Add(pair.Key, pair.Value);
                }
            }
            catch (Exception error)
            {
                if (!assetRegistrationErrorLogged) { assetRegistrationErrorLogged = true; Debug.LogWarning("GGO icon registration: " + error.GetType().FullName); }
            }
        }

        public static void Ensure(ShopPage page)
        {
            RegisterIcons();
            if (page == null || page.simpleGrid == null) return;
            GGOShop control = page.GetComponent<GGOShop>();
            if (control == null) control = page.gameObject.AddComponent<GGOShop>();
            control.Hide(); control.shop = page;
            if (control.entry != null) { page.simpleGrid.childs.Remove(control.entry); UnityEngine.Object.Destroy(control.entry); }
            GameObject buttonObject = (GameObject)UnityEngine.Object.Instantiate(page.tabBtnPrefab);
            buttonObject.name = "GGOExchangeEntry";
            ShopTabButton button = buttonObject.GetComponent<ShopTabButton>();
            button.SetType(ShopType.ScoreShop);
            Field(typeof(ShopTabButton), "m_type").SetValue(button, (ShopType)GGOType);
            button.m_callback = page.OnTabBtn;
            button.labName.text = "信用商店";
            UIDragScrollView drag = buttonObject.GetComponent<UIDragScrollView>(); if (drag != null) drag.scrollView = page.scrollView;
            buttonObject.SetActive(true); page.simpleGrid.AddChild(buttonObject, 1f); page.simpleGrid.Reposition();
            Dictionary<ShopType, GameObject> tabs = Field(typeof(ShopPage), "tabBtns").GetValue(page) as Dictionary<ShopType, GameObject>;
            tabs[(ShopType)GGOType] = buttonObject;
            control.entry = buttonObject;
            page.scrollView.enabled = page.simpleGrid.childs.Count > 9;
        }

        // Native dispatch is retained for every original type; our own type is consumed first.
        public static bool TrySelect(ShopPage page, ShopType type)
        {
            GGOShop control = page.GetComponent<GGOShop>();
            if ((int)type != GGOType) { if (ShopPage.canClickTab && control != null) control.Hide(); return false; }
            if (control == null) { Ensure(page); control = page.GetComponent<GGOShop>(); }
            if (!ShopPage.canClickTab) return true;
            control.Select(); return true;
        }
        void Select()
        {
            RegisterIcons();
            Hide();
            if (!Current(out owner, out session) || String.IsNullOrEmpty(origin = Origin())) return;
            LoadPending(); version = ""; enabledShop = false; confirmedNotice = "";
            GameObject old = Field(typeof(ShopPage), "curTab").GetValue(shop) as GameObject;
            if (old != null) { old.SetActive(false); UnityEngine.Object.Destroy(old); }
            // The native prefab has no automatic network lifecycle. Never invoke UpdateAll/SetScoreItem.
            content = (GameObject)typeof(ShopPage).GetMethod("GetRightTab", Flags).Invoke(shop, new object[] { "ShopInnerPage/ScoreShop" });
            layout = content.GetComponent<TabScoreShop>();
            if (layout == null) { UnityEngine.Object.Destroy(content); content = null; return; }
            layout.enabled = false;
            foreach (GameObject child in new List<GameObject>(layout.simpleGrid.childs)) UnityEngine.Object.Destroy(child);
            layout.simpleGrid.childs.Clear(); rows.Clear();
            Field(typeof(ShopPage), "curTab").SetValue(shop, content);
            Field(typeof(ShopPage), "curType").SetValue(shop, (ShopType)GGOType);
            shop.options["type"] = GGOType.ToString();
            shop.btnCharge.SetActive(false); shop.btnRecoverry.SetActive(false);
            restoreTop = shop.shopTopBar.gameObject.activeSelf; shop.shopTopBar.gameObject.SetActive(true);
            selected = true; content.SetActive(true);
            CreateHeader(); CreateRefresh();
            if (!String.IsNullOrEmpty(pendingId)) SetNotice("上次兑换待确认，点击信用点重试");
            StartCoroutine(Fetch(false));
        }
        void CreateHeader()
        {
            // Use the native top bar's item prefab, placement, font and resource icon.
            ShopTopBar bar = shop.shopTopBar;
            foreach (GameObject item in bar.topBarItems) UnityEngine.Object.Destroy(item);
            bar.topBarItems.Clear();
            GameObject go = (GameObject)typeof(ShopTopBar).GetMethod("CreateItem", Flags).Invoke(bar, new object[] { currency });
            go.transform.parent = bar.transform; go.transform.localScale = Vector3.one;
            go.transform.localPosition = new Vector3(-185f, -3f, 0f);
            bar.topBarItems.Add(go);
            balanceItem = go.GetComponent<TopBarItem>();
            balanceLabel = balanceItem.labNum;
            balanceIcon = null;
            BindCreditHeader();
            NGUITools.AddWidgetCollider(go);
            UIEventListener.Get(go).onClick = delegate(GameObject ignored) { Retry(); };
        }
        void CreateRefresh()
        {
            // Copy the original memory shop's refresh control and count font.
            // The recovery-room button uses baked lettering and cannot be relabelled.
            GameObject template = null;
            try
            {
                template = (GameObject)typeof(ShopPage).GetMethod("GetRightTab", Flags).Invoke(shop, new object[] { "ShopInnerPage/MemoryShop" });
                template.SetActive(false);
                TabMemoryShop memory = template.GetComponent<TabMemoryShop>();
                Transform native = null;
                foreach (Transform child in template.GetComponentsInChildren<Transform>(true))
                    if (child.name == "BtnRefresh") { if (native != null) throw new Exception("ambiguous refresh control"); native = child; }
                if (native == null || memory == null || memory.labLeftRefreshTimes == null) throw new Exception("native refresh control unavailable");
                refreshButton = (GameObject)UnityEngine.Object.Instantiate(native.gameObject);
                refreshButton.name = "GGOCreditRefresh";
                refreshButton.transform.parent = shop.btnRecoverry.transform.parent;
                refreshButton.transform.localPosition = shop.btnRecoverry.transform.localPosition;
                refreshButton.transform.localScale = Vector3.one;
                foreach (UIButtonMessage old in refreshButton.GetComponentsInChildren<UIButtonMessage>(true)) UnityEngine.Object.DestroyImmediate(old);
                foreach (UIEventListener old in refreshButton.GetComponentsInChildren<UIEventListener>(true)) UnityEngine.Object.DestroyImmediate(old);
                refreshControl = refreshButton.GetComponent<UIButton>();
                if (refreshControl != null) { refreshControl.onClick.Clear(); EventDelegate.Set(refreshControl.onClick, RefreshStock); }
                else UIEventListener.Get(refreshButton).onClick = delegate(GameObject ignored) { RefreshStock(); };
                GameObject info = (GameObject)UnityEngine.Object.Instantiate(memory.labLeftRefreshTimes.gameObject);
                info.name = "GGOCreditRefreshInfo"; info.transform.parent = refreshButton.transform;
                info.transform.localScale = Vector3.one;
                info.transform.localPosition = new Vector3(-240f, 0f, 0f);
                refreshInfo = info.GetComponent<UILabel>(); refreshInfo.width = 300; refreshInfo.text = "";
                refreshInfo.pivot = UIWidget.Pivot.Center; refreshInfo.alignment = NGUIText.Alignment.Center;
                foreach (Collider collider in info.GetComponentsInChildren<Collider>(true)) UnityEngine.Object.DestroyImmediate(collider);
                refreshButton.SetActive(false);
            }
            catch (Exception)
            {
                if (refreshButton != null) UnityEngine.Object.Destroy(refreshButton);
                refreshButton = null; refreshInfo = null; refreshControl = null;
                SetNotice("刷新按钮暂不可用，请重新打开商店");
            }
            finally { if (template != null) UnityEngine.Object.Destroy(template); }
        }
        public void RefreshStock()
        {
            if (!CanStartPurchase() || !refreshEnabled) return;
            if (freeRefreshLeft < 1 && paidRefreshLeft < 1) { SetNotice("刷新次数已用完"); return; }
            if (freeRefreshLeft < 1 && (refreshCurrency != currency || refreshCost < 1 || balance < refreshCost)) { SetNotice("信用点不足"); return; }
            pendingId = Guid.NewGuid().ToString(); pendingProduct = RefreshProduct; pendingVersion = version; pendingCount = 1;
            confirmedNotice = ""; SavePending(); StartCoroutine(Fetch(true));
        }
        void BindCreditHeader()
        {
            if (balanceItem == null) return;
            // TopBarItem only recognizes original currencies; its prefab retains
            // the previous item caption/icon for this extended currency ID.
            balanceItem.labName.text = String.IsNullOrEmpty(currencyName) ? "信用点" : currencyName;
            UISprite nativeIcon = balanceItem.logoSprite;
            if (nativeIcon == null) return;
            nativeIcon.gameObject.SetActive(true);
            if (balanceIcon == null)
            {
                balanceIcon = CommonMethod.CreateCommonIcon(nativeIcon.transform);
                balanceIcon.gameObject.name = "GGOCreditTopBarIcon";
                balanceIcon.transform.localPosition = Vector3.zero;
                balanceIcon.transform.localScale = Vector3.one * (Math.Max(1, Math.Min(nativeIcon.width, nativeIcon.height)) / 128f);
            }
            // Reuse the same registered GGO resource and original icon renderer
            // as the existing credit quantity selector, within the native slot.
            balanceIcon.SetData(currency, false, -1, null);
            if (balanceIcon.itemCount != null) balanceIcon.itemCount.gameObject.SetActive(false);
            // Keep the parent widget enabled so NGUI does not hide the child icon.
            nativeIcon.spriteName = ""; nativeIcon.enabled = true;
        }
        void SetNotice(string text)
        {
            if (!String.IsNullOrEmpty(text)) BoxManager.CreatePopupTextBox(text, 0f);
        }
        void Hide()
        {
            inventorySnapshotGeneration++;
            StopRequests();
            if (selected && shop != null && shop.shopTopBar != null) shop.shopTopBar.gameObject.SetActive(restoreTop);
            if (refreshButton != null) UnityEngine.Object.Destroy(refreshButton); refreshButton = null; refreshInfo = null; refreshControl = null; refreshEnabled = false;
            selected = false; if (content != null) content.SetActive(false);
        }
        void StopRequests() { StopAllCoroutines(); if (request != null) { request.Dispose(); request = null; } busy = false; }
        void OnDisable() { Hide(); }
        void OnDestroy() { StopRequests(); }
        void Update()
        {
            if (selected && !Visible()) { Hide(); if (content != null) UnityEngine.Object.Destroy(content); content = null; rows.Clear(); }
        }
        void LateUpdate()
        {
            if (!Visible()) return;
            // Original king controls retain their state, while only the active tab is highlighted.
            foreach (ShopTabButton button in shop.simpleGrid.GetComponentsInChildren<ShopTabButton>())
            {
                bool active = button.gameObject == entry;
                if (button.chosedBg != null) button.chosedBg.SetActive(active);
                if (button.objPointer != null) button.objPointer.SetActive(active);
            }
        }
        void LoadPending()
        {
            pendingId = pendingProduct = pendingVersion = ""; pendingCount = 1; pendingUnreadable = false;
            string raw = PlayerPrefs.GetString(PreferenceKey(), "");
            if (String.IsNullOrEmpty(raw)) return;
            try
            {
                JsonData value = JsonMapper.ToObject(raw);
                pendingId = (string)value["id"]; pendingProduct = (string)value["product"]; pendingVersion = (string)value["version"];
                if (String.IsNullOrEmpty(pendingId) || String.IsNullOrEmpty(pendingProduct) || String.IsNullOrEmpty(pendingVersion)) throw new Exception("invalid receipt");
                if (new Guid(pendingId) == Guid.Empty) throw new Exception("invalid UUID");
                if (((IDictionary)value).Contains("count"))
                {
                    JsonData stored = value["count"];
                    if ((!stored.IsInt && !stored.IsLong) || !Int32.TryParse(stored.ToString(), out pendingCount) || pendingCount < 1 || pendingCount > 100)
                        throw new Exception("invalid purchase count");
                }
            }
            catch { pendingUnreadable = true; }
        }
        void SavePending()
        {
            JsonData value = new JsonData(); value["id"] = pendingId; value["product"] = pendingProduct; value["version"] = pendingVersion; value["count"] = pendingCount;
            PlayerPrefs.SetString(PreferenceKey(), JsonMapper.ToJson(value)); PlayerPrefs.Save();
        }
        // Preserve the inherited helper API; existing callers now enter the same scoped quantity selector.
        public void Buy(string product)
        {
            foreach (GGOProductRow row in rows) if (row != null && row.ProductID == product) { row.OnBuyBtn(); return; }
        }
        bool CanStartPurchase()
        {
            if (!Visible() || busy) return false;
            if (inventoryRefreshPending) { SetNotice("背包尚未同步，点击信用点重试"); return false; }
            if (pendingUnreadable) { SetNotice("兑换记录无法读取，请联系管理员核对"); return false; }
            if (!String.IsNullOrEmpty(pendingId)) { SetNotice("上次兑换待确认，点击信用点重试"); return false; }
            return enabledShop && !String.IsNullOrEmpty(version);
        }
        int Maximum(long remaining, long unitCost, int rewardAmount)
        {
            if (unitCost < 1 || rewardAmount < 1 || balance < 0) return 0;
            long max = remaining < 0 ? 100 : Math.Min(100, remaining);
            max = Math.Min(max, balance / unitCost);
            max = Math.Min(max, Int32.MaxValue / unitCost);
            max = Math.Min(max, Int32.MaxValue / rewardAmount);
            return (int)Math.Max(0, max);
        }
        public void ChooseQuantity(Component chooser, string product, string rewardID, int rewardAmount, long price, long remaining)
        {
            if (!CanStartPurchase() || chooser == null) return;
            int max = Maximum(remaining, price, rewardAmount);
            if (max < 1)
            {
                string message = remaining == 0 ? "商品已达到限购次数" : "信用点不足，无法兑换该商品";
                SetNotice(message); BoxManager.CreatePopupTextBox(message, 0f); return;
            }
            long selectedOwner = owner, selectedRevision = catalogueRevision;
            string selectedSession = session, selectedOrigin = origin, selectedVersion = version;
            bool submitted = false;
            Action<int> confirm = delegate(int count)
            {
                if (submitted) return;
                if (chooser == null || !Visible() || owner != selectedOwner || session != selectedSession || origin != selectedOrigin ||
                    version != selectedVersion || catalogueRevision != selectedRevision)
                {
                    if (Visible() && !busy) { SetNotice("目录已更新，正在重新加载…"); StartCoroutine(Fetch(false)); }
                    return;
                }
                if (!CanStartPurchase() || count < 1 || count > Maximum(remaining, price, rewardAmount)) return;
                submitted = true;
                confirmedNotice = ""; pendingId = Guid.NewGuid().ToString(); pendingProduct = product; pendingVersion = selectedVersion; pendingCount = count;
                SavePending(); StartCoroutine(Fetch(true));
            };
            try
            {
                Type type = Type.GetType("HYJS.BulkPurchase.BulkPurchase, HYJS.BulkPurchase", true);
                MethodInfo selector = type.GetMethod("SelectQuantity", BindingFlags.Public | BindingFlags.Static, null,
                    new Type[] { typeof(Component), typeof(string), typeof(int), typeof(string), typeof(long), typeof(int), typeof(Action<int>) }, null);
                if (selector == null || selector.ReturnType != typeof(bool) || !(bool)selector.Invoke(null, new object[] { chooser, rewardID, rewardAmount, currency, price, max, confirm }))
                    throw new Exception("quantity selector unavailable");
            }
            catch (Exception)
            {
                SetNotice("数量选择暂不可用，请重新打开商店");
                BoxManager.CreatePopupTextBox("数量选择暂不可用，请重新打开商店", 0f);
            }
        }
        public void Retry() { if (!Visible() || busy) return; if (pendingUnreadable) { SetNotice("兑换记录无法读取，请联系管理员核对"); return; } StartCoroutine(Fetch(!String.IsNullOrEmpty(pendingId))); }
        IEnumerator Fetch(bool buy)
        {
            if (busy || !Visible()) yield break;
            busy = true; RefreshButtons();
            JsonData body = new JsonData(); body["playerId"] = owner; body["sessionHex"] = session;
            string sentId = pendingId, sentProduct = pendingProduct, sentVersion = pendingVersion;
            int sentCount = pendingCount; bool refresh = buy && sentProduct == RefreshProduct;
            long sentOwner = owner; string sentSession = session, sentOrigin = origin;
            if (buy) { body["version"] = sentVersion; body["requestId"] = sentId; if (!refresh) { body["productId"] = sentProduct; body["count"] = sentCount; } }
            Dictionary<string,string> headers = new Dictionary<string,string>(); headers.Add("Content-Type", "application/json");
            WWW current = new WWW(origin + (buy ? (refresh ? "/ggo-shop/refresh" : "/ggo-shop/buy") : "/ggo-shop/catalogue"), Encoding.UTF8.GetBytes(JsonMapper.ToJson(body)), headers);
            request = current; float deadline = Time.realtimeSinceStartup + 15f;
            while (!current.isDone && Time.realtimeSinceStartup < deadline) yield return null;
            bool success = false;
            string message = buy ? "兑换结果待确认，点击信用点重试" : "加载失败，点击信用点重试";
            try
            {
                if (Visible() && owner == sentOwner && session == sentSession && origin == sentOrigin && current.isDone && String.IsNullOrEmpty(current.error))
                {
                    if (buy && (pendingId != sentId || pendingProduct != sentProduct || pendingVersion != sentVersion || pendingCount != sentCount)) throw new Exception("pending identity changed");
                    JsonData response = JsonMapper.ToObject(current.text);
                    if ((int)response["code"] == 0)
                    {
                        JsonData data = response["data"];
                        if (buy)
                        {
                            if ((string)data["requestId"] != sentId || pendingId != sentId || pendingProduct != sentProduct || pendingVersion != sentVersion || pendingCount != sentCount) throw new Exception("receipt differs");
                            if (refresh) message = "刷新成功";
                            else
                            {
                                string reward = (string)data["rewardId"];
                                string points = data["terminalPointsAdded"].ToString();
                                message = "兑换成功：" + TableTextBlo.GetText(CommonMethod.GetObjectNameTextId(reward)) + " ×" + data["rewardAmount"].ToString();
                                if (points != "0") message += "，" + points + " 超频点数已到账";
                            }
                            // A receipt may be replayed long after purchase; its catalogue is historical.
                            // Settle the UUID once, then obtain live balances and limits through catalogue.
                            confirmedNotice = message;
                            if (!refresh)
                            {
                                inventoryRefreshResource = (string)data["rewardId"];
                                inventoryRefreshPending = inventoryRefreshResource != "item_9711";
                                inventorySnapshotGeneration++;
                            }
                            PlayerPrefs.DeleteKey(PreferenceKey()); PlayerPrefs.Save(); pendingId = pendingProduct = pendingVersion = ""; pendingCount = 1;
                            enabledShop = false; version = "";
                            balanceLabel.text = "…";
                        }
                        else
                        {
                            Render(data);
                            message = String.IsNullOrEmpty(pendingId) ? (enabledShop ? "" : "暂未开放") : "上次兑换待确认，点击信用点重试";
                        }
                        success = true;
                    }
                    else
                    {
                        // A rejected response is definitive: no debit occurred; a new catalogue can be requested.
                        message = ((IDictionary)response).Contains("msg") ? response["msg"].ToString() : "兑换未完成，请刷新后重试";
                        if (buy) { PlayerPrefs.DeleteKey(PreferenceKey()); PlayerPrefs.Save(); pendingId = pendingProduct = pendingVersion = ""; pendingCount = 1; }
                    }
                }
            }
            catch (Exception) { message = buy ? "兑换结果待确认，点击信用点重试" : "加载失败，点击信用点重试"; }
            current.Dispose(); if (request == current) request = null;
            if (success && inventoryRefreshPending) yield return StartCoroutine(RefreshInventory());
            busy = false;
            if (!Visible()) yield break;
            if (!success && !buy)
            {
                enabledShop = false;
                if (!String.IsNullOrEmpty(confirmedNotice)) message = confirmedNotice + "；余额刷新失败，点击信用点重试";
            }
            if (success && inventoryRefreshPending) message += "；背包同步失败，点击信用点重试";
            SetNotice(pendingUnreadable ? "兑换记录无法读取，请联系管理员核对" : message); RefreshButtons();
            if (success && buy) StartCoroutine(Fetch(false));
        }
        IEnumerator RefreshInventory()
        {
            float deadline = Time.realtimeSinceStartup + 15f;
            while (Visible() && NetAPI.IsSendingHttp() && Time.realtimeSinceStartup < deadline) yield return null;
            if (!Visible() || NetAPI.IsSendingHttp()) yield break;
            long player = owner; string key = session, server = origin, resource = inventoryRefreshResource;
            long generation = ++inventorySnapshotGeneration;
            bool finished = false, sent = false;
            try
            {
                NetWork.Layer.HttpHandler completed = delegate(NetWork.Packets.Packet packet, bool ok)
                {
                    finished = true;
                    if (!ok || packet == null || !Visible() || owner != player || session != key || origin != server || inventoryRefreshResource != resource || inventorySnapshotGeneration != generation || Time.realtimeSinceStartup > deadline) return;
                    try
                    {
                        ApplyInventorySnapshot(packet, resource);
                        inventoryRefreshPending = false;
                        if (Visible()) RefreshButtons();
                    }
                    catch (Exception) { Debug.LogWarning("Credit shop inventory snapshot unavailable"); }
                };
                if (resource.StartsWith("hero_exp_", StringComparison.Ordinal)) sent = NetAPI.SendHttp(20105, new CSWarriorList(), completed);
                else if (resource.StartsWith("frag_", StringComparison.Ordinal)) sent = NetAPI.SendHttp(20109, new CSFragmentList(), completed);
                else sent = NetAPI.SendHttp(20107, new CSItemList(), completed);
            }
            catch (Exception) { Debug.LogWarning("Credit shop inventory refresh unavailable"); }
            while (sent && !finished && Visible() && Time.realtimeSinceStartup < deadline) yield return null;
            if (!finished && inventorySnapshotGeneration == generation) inventorySnapshotGeneration++;
        }
        static void ApplyInventorySnapshot(NetWork.Packets.Packet packet, string resource)
        {
            UIPlayerData player = StaticData.playerData;
            if (resource.StartsWith("hero_exp_", StringComparison.Ordinal))
            {
                SCWarriorList reply = packet.kBody as SCWarriorList;
                if (reply == null || reply.warriorList == null) throw new Exception("warrior snapshot missing");
                UIInstPlayerData inst = player.InstPlayerData;
                Dictionary<string, UIHero> next = new Dictionary<string, UIHero>(inst.InstHeros);
                foreach (KeyValuePair<string, UIHero> entry in inst.InstHeros)
                    if (entry.Value != null && entry.Value.heroId == resource)
                    {
                        if (entry.Value.owner != null) throw new Exception("experience material assigned");
                        next.Remove(entry.Key);
                    }
                foreach (Warrior warrior in reply.warriorList)
                    if (warrior != null && warrior.id == resource)
                    {
                        if (String.IsNullOrEmpty(warrior.guid) || next.ContainsKey(warrior.guid)) throw new Exception("invalid material identity");
                        UIHero hero = new UIHero(warrior.id); hero.InitData(warrior);
                        next.Add(warrior.guid, hero);
                    }
                FieldInfo heroes = Field(typeof(UIInstPlayerData), "instHeros");
                if (heroes == null) throw new Exception("material collection unavailable");
                heroes.SetValue(inst, next);
                return;
            }
            List<SimpleItem> items; string fragmentField = null;
            if (resource.StartsWith("frag_", StringComparison.Ordinal))
            {
                SCFragmentList reply = packet.kBody as SCFragmentList;
                if (reply == null) throw new Exception("fragment snapshot missing");
                if (resource.StartsWith("frag_equip", StringComparison.Ordinal)) { items = reply.fragEquipmentList; fragmentField = "fragEquipments"; }
                else if (resource.StartsWith("frag_hero", StringComparison.Ordinal)) { items = reply.fragWarriorList; fragmentField = "fragHeros"; }
                else if (resource.StartsWith("frag_pet", StringComparison.Ordinal)) { items = reply.fragPetList; fragmentField = "fragPets"; }
                else if (resource.StartsWith("frag_trea", StringComparison.Ordinal)) items = reply.fragTreasureList;
                else throw new Exception("unknown fragment category");
            }
            else
            {
                SCItemList reply = packet.kBody as SCItemList;
                if (reply == null) throw new Exception("item snapshot missing");
                items = reply.itemList;
            }
            int amount = 0; bool found = false;
            if (items == null) throw new Exception("inventory list missing");
            foreach (SimpleItem item in items)
                if (item != null && item.id == resource)
                {
                    if (found || item.num < 0) throw new Exception("invalid inventory quantity");
                    found = true; amount = item.num;
                }
            // The original updater assigns absolute quantities. A replayed receipt
            // supplies only the resource identity; its old reward amount is never applied.
            if (fragmentField != null)
            {
                FieldInfo field = Field(typeof(UIPlayerData), fragmentField);
                Dictionary<string, UIFragItem> fragments = field == null ? null : field.GetValue(player) as Dictionary<string, UIFragItem>;
                if (fragments == null) throw new Exception("fragment collection unavailable");
                if (!fragments.ContainsKey(resource))
                {
                    Dict.DataModel.TableFrag table = Dict.Blo.DictFragBlo.GetTableFrag(resource);
                    if (table == null) throw new Exception("fragment table unavailable");
                    fragments.Add(resource, new UIFragItem(table));
                }
            }
            else if (!resource.StartsWith("frag_trea", StringComparison.Ordinal) && !player.SimpleGoods.ContainsKey(resource))
            {
                Dict.DataModel.TableItem table = Dict.Blo.DictItemBlo.GetTableItem(resource);
                if (table == null) throw new Exception("item table unavailable");
                player.SimpleGoods.Add(resource, new UISimpleItem(table));
            }
            player.UpdateSimpleItem(new List<SimpleItem> { new SimpleItem { id = resource, num = amount } });
        }
        void Render(JsonData catalogue)
        {
            if (!Visible()) throw new Exception("owner changed");
            string responseCurrency = (string)catalogue["currencyId"];
            long absoluteBalance = Int64.Parse(catalogue["balance"].ToString());
            if (responseCurrency != "item_9710" || absoluteBalance < 0) throw new Exception("invalid currency balance");
            UISimpleItem credit;
            Dictionary<string, UISimpleItem> inventory = StaticData.playerData.SimpleGoods;
            if (!inventory.TryGetValue("item_9710", out credit) || credit == null)
            {
                Dict.DataModel.TableItem itemData = Dict.Blo.DictItemBlo.GetTableItem("item_9710");
                if (itemData == null) throw new Exception("currency table unavailable");
                credit = new UISimpleItem(itemData); inventory["item_9710"] = credit;
            }
            // Catalogue is authoritative; absolute assignment is safe for repeated receipts.
            credit.num = absoluteBalance; balance = absoluteBalance; catalogueRevision++;
            version = (string)catalogue["version"]; enabledShop = (bool)catalogue["enabled"];
            currency = (string)catalogue["currencyId"]; currencyName = (string)catalogue["currencyName"];
            balanceItem.SetItem(currency);
            BindCreditHeader();
            balanceLabel.text = catalogue["balance"].ToString();
            refreshEnabled = false; freeRefreshLeft = paidRefreshLeft = refreshCost = 0; refreshCurrency = "";
            if (((IDictionary)catalogue).Contains("refresh") && catalogue["refresh"] != null)
            {
                JsonData refresh = catalogue["refresh"];
                refreshEnabled = (bool)refresh["enabled"];
                freeRefreshLeft = Int64.Parse(refresh["freeLeft"].ToString()); paidRefreshLeft = Int64.Parse(refresh["paidLeft"].ToString());
                refreshCost = Int64.Parse(refresh["costAmount"].ToString()); refreshCurrency = (string)refresh["costId"];
                if (freeRefreshLeft < 0 || paidRefreshLeft < 0 || refreshCost < 0) throw new Exception("invalid refresh allowance");
                if (refreshInfo != null) refreshInfo.text = freeRefreshLeft > 0 ? "免费刷新 " + freeRefreshLeft + "次" :
                    (paidRefreshLeft > 0 ? refreshCost + " 信用点  剩余" + paidRefreshLeft + "次" : "刷新次数已用完");
            }
            foreach (GameObject child in new List<GameObject>(layout.simpleGrid.childs)) UnityEngine.Object.Destroy(child);
            layout.simpleGrid.childs.Clear(); rows.Clear();
            foreach (JsonData product in catalogue["items"])
            {
                GameObject clone = (GameObject)UnityEngine.Object.Instantiate(layout.shopItemPrefab);
                clone.name = "GGOProduct-" + (string)product["id"];
                ShopScoreItem original = clone.GetComponent<ShopScoreItem>();
                GGOProductRow row = clone.AddComponent<GGOProductRow>();
                row.Bind(this, original, product, currency, currencyName);
                // Removing the native handler makes legacy score purchase messages impossible on our rows.
                UnityEngine.Object.DestroyImmediate(original);
                clone.SetActive(true); layout.simpleGrid.AddChild(clone, 1f); rows.Add(row);
            }
            layout.simpleGrid.Reposition(); layout.scrollView.ResetPosition(); RefreshButtons();
        }
        void RefreshButtons()
        {
            bool blocked = busy || !enabledShop || pendingUnreadable || inventoryRefreshPending || !String.IsNullOrEmpty(pendingId);
            foreach (GGOProductRow row in rows) if (row != null) row.SetBusy(blocked);
            if (refreshButton != null) refreshButton.SetActive(selected && refreshEnabled);
            if (refreshControl != null) refreshControl.isEnabled = !blocked && (freeRefreshLeft > 0 || paidRefreshLeft > 0);
        }

        // Filter only positive terminal tickets at the real inventory merge. The caller's result list remains untouched.
        public static List<SimpleItem> InventoryItems(List<SimpleItem> source)
        {
            RegisterIcons();
            if (source == null) return source;
            List<SimpleItem> result = null;
            foreach (SimpleItem item in source)
                if (item != null && item.id == "item_9711" && item.num > 0) { result = new List<SimpleItem>(); break; }
            if (result == null) return source;
            foreach (SimpleItem item in source) if (item == null || item.id != "item_9711" || item.num <= 0) result.Add(item);
            return result;
        }
    }

    public sealed class GGOProductRow : MonoBehaviour
    {
        GGOShop shop; string product, rewardID; int rewardAmount; long price, remaining;
        public string ProductID { get { return product; } }
        GameObject button, soldLabel; UILabel buttonLabel; bool available;
        public void Bind(GGOShop owner, ShopScoreItem original, JsonData data, string currency, string currencyName)
        {
            shop = owner; product = (string)data["id"]; button = original.btnBuy;
            original.labName.text = (string)data["name"];
            original.discountRoot.SetActive(false); original.twoCostRoot.SetActive(false); original.oneCostRoot.SetActive(true);
            original.labCostName.text = currencyName; original.labPrice.text = data["price"].ToString();
            soldLabel = original.labBuyed;
            UILabel soldText = soldLabel.GetComponent<UILabel>(); RemoveTextKit(soldText); if (soldText != null) soldText.text = "已兑换";
            buttonLabel = button.GetComponentInChildren<UILabel>(); RemoveTextKit(buttonLabel);
            remaining = Int64.Parse(data["remaining"].ToString()); price = Int64.Parse(data["price"].ToString());
            rewardID = (string)data["resourceId"]; rewardAmount = Int32.Parse(data["amount"].ToString()); available = remaining != 0 && price > 0 && rewardAmount > 0;
            GameObject limitObject = (GameObject)UnityEngine.Object.Instantiate(original.labName.gameObject);
            limitObject.name = "GGORemaining"; limitObject.transform.parent = transform;
            limitObject.transform.localScale = Vector3.one; limitObject.transform.localRotation = Quaternion.identity;
            UILabel limit = limitObject.GetComponent<UILabel>();
            limit.pivot = UIWidget.Pivot.Left; limit.alignment = NGUIText.Alignment.Left;
            limit.fontSize = 18; limit.width = 150; limit.height = 24; limit.maxLineCount = 1; limit.overflowMethod = UILabel.Overflow.ShrinkContent;
            limit.text = remaining < 0 ? "不限购" : "剩余 " + remaining;
            limitObject.transform.localPosition = new Vector3(-142f, -48f, 0f); limitObject.SetActive(true);
            NGUITools.DestroyChildren(original.iconRoot.transform);
            CommonIcon reward = CommonMethod.CreateCommonIcon(original.iconRoot.transform);
            reward.SetData((string)data["resourceId"], true, Int32.Parse(data["amount"].ToString()), null);
            original.costLogo.enabled = false;
            CommonIcon cost = CommonMethod.CreateCommonIcon(original.costLogo.transform);
            cost.transform.localScale = Vector3.one * (Math.Min(original.costLogo.width, original.costLogo.height) / 128f);
            cost.SetData(currency, false, -1, null);
            foreach (UIButtonMessage old in button.GetComponents<UIButtonMessage>()) UnityEngine.Object.DestroyImmediate(old);
            UIEventListener.Get(button).onClick = null;
            UIButton uiButton = button.GetComponent<UIButton>();
            if (uiButton != null) { uiButton.onClick.Clear(); EventDelegate.Set(uiButton.onClick, OnBuyBtn); }
            else UIEventListener.Get(button).onClick = delegate(GameObject ignored) { OnBuyBtn(); };
            SetBusy(false);
        }
        static void RemoveTextKit(UILabel label)
        {
            if (label == null) return;
            // Original prefab localization would replace our owned row text during Start.
            foreach (MonoBehaviour component in label.GetComponents<MonoBehaviour>())
                if (component != null && component.GetType().Name == "TextKit") UnityEngine.Object.DestroyImmediate(component);
        }
        public void OnBuyBtn() { if (available && shop != null) shop.ChooseQuantity(this, product, rewardID, rewardAmount, price, remaining); }
        public void SetBusy(bool value)
        {
            if (button == null) return;
            button.SetActive(true);
            if (buttonLabel != null) { buttonLabel.gameObject.SetActive(available); buttonLabel.text = "兑换"; }
            if (soldLabel != null) soldLabel.SetActive(!available);
            UIButton ui = button.GetComponent<UIButton>(); if (ui != null) ui.isEnabled = available && !value;
        }
    }
}
