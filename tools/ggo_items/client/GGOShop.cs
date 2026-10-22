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
        UILabel balanceLabel, noticeLabel;
        CommonIcon balanceIcon;
        long owner;
        string session = "", origin = "", version = "", currency = "item_9710", currencyName = "回收信用点";
        string pendingId = "", pendingProduct = "", pendingVersion = "", confirmedNotice = "";
        bool selected, busy, enabledShop, restoreTop, pendingUnreadable;
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

        public static void Ensure(ShopPage page)
        {
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
            button.labName.text = "荒野兑换所"; button.labName.fontSize = 22; button.labName.width = 108;
            button.labName.maxLineCount = 1; button.labName.overflowMethod = UILabel.Overflow.ShrinkContent;
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
            restoreTop = shop.shopTopBar.gameObject.activeSelf; shop.shopTopBar.gameObject.SetActive(false);
            selected = true; content.SetActive(true);
            // Reserve the original panel's upper band for a fixed balance and status row.
            UIPanel panel = layout.scrollView.GetComponent<UIPanel>();
            if (panel != null)
            {
                Vector4 clip = panel.baseClipRegion;
                clip.y -= 42f; clip.w -= 84f; panel.baseClipRegion = clip;
            }
            CreateHeader(); SetNotice(String.IsNullOrEmpty(pendingId) ? "正在加载…" : "上次兑换待确认，点击重试");
            StartCoroutine(Fetch(false));
        }
        void CreateHeader()
        {
            ShopScoreItem template = layout.shopItemPrefab.GetComponent<ShopScoreItem>();
            balanceLabel = CloneLabel(template.labName, "GGO Balance", new Vector3(235f, 235f, 0f), 22);
            balanceLabel.text = "荒野兑换所";
            noticeLabel = CloneLabel(template.labName, "GGO Notice", new Vector3(235f, 200f, 0f), 20);
            BoxCollider box = noticeLabel.gameObject.GetComponent<BoxCollider>();
            if (box == null) box = noticeLabel.gameObject.AddComponent<BoxCollider>();
            box.center = new Vector3(-275f, -14f, 0f); box.size = new Vector3(550f, 28f, 1f);
            UIEventListener.Get(noticeLabel.gameObject).onClick = delegate(GameObject ignored) { Retry(); };
            balanceIcon = CommonMethod.CreateCommonIcon(content.transform);
            balanceIcon.transform.localPosition = new Vector3(265f, 220f, 0f); balanceIcon.transform.localScale = Vector3.one * .25f;
            balanceIcon.SetData("item_9710", true, -1, null);
        }
        UILabel CloneLabel(UILabel source, string name, Vector3 position, int fontSize)
        {
            GameObject clone = (GameObject)UnityEngine.Object.Instantiate(source.gameObject);
            clone.name = name; clone.transform.parent = content.transform;
            clone.transform.localScale = Vector3.one; clone.transform.localRotation = Quaternion.identity;
            UILabel label = clone.GetComponent<UILabel>();
            label.pivot = UIWidget.Pivot.TopRight; label.alignment = NGUIText.Alignment.Right;
            label.fontSize = fontSize; label.width = 550; label.height = 28; label.maxLineCount = 1; label.overflowMethod = UILabel.Overflow.ShrinkContent;
            // Setting NGUI pivot moves its transform; apply the fixed content position last.
            clone.transform.localPosition = position; clone.SetActive(true); return label;
        }
        void SetNotice(string text) { if (noticeLabel != null) noticeLabel.text = text; }
        void Hide()
        {
            StopRequests();
            if (selected && shop != null && shop.shopTopBar != null) shop.shopTopBar.gameObject.SetActive(restoreTop);
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
            pendingId = pendingProduct = pendingVersion = ""; pendingUnreadable = false;
            string raw = PlayerPrefs.GetString(PreferenceKey(), "");
            if (String.IsNullOrEmpty(raw)) return;
            try { JsonData value = JsonMapper.ToObject(raw); pendingId = (string)value["id"]; pendingProduct = (string)value["product"]; pendingVersion = (string)value["version"]; if (String.IsNullOrEmpty(pendingId) || String.IsNullOrEmpty(pendingProduct) || String.IsNullOrEmpty(pendingVersion)) throw new Exception("invalid receipt"); new Guid(pendingId); }
            catch { pendingUnreadable = true; }
        }
        void SavePending()
        {
            JsonData value = new JsonData(); value["id"] = pendingId; value["product"] = pendingProduct; value["version"] = pendingVersion;
            PlayerPrefs.SetString(PreferenceKey(), JsonMapper.ToJson(value)); PlayerPrefs.Save();
        }
        public void Buy(string product)
        {
            if (!Visible() || busy) return;
            if (pendingUnreadable) { SetNotice("兑换记录无法读取，请联系管理员核对"); return; }
            if (!String.IsNullOrEmpty(pendingId)) { SetNotice("上次兑换待确认，点击提示重试"); return; }
            if (!enabledShop || String.IsNullOrEmpty(version)) return;
            confirmedNotice = ""; pendingId = Guid.NewGuid().ToString(); pendingProduct = product; pendingVersion = version;
            SavePending(); StartCoroutine(Fetch(true));
        }
        public void Retry() { if (!Visible() || busy) return; if (pendingUnreadable) { SetNotice("兑换记录无法读取，请联系管理员核对"); return; } StartCoroutine(Fetch(!String.IsNullOrEmpty(pendingId))); }
        IEnumerator Fetch(bool buy)
        {
            if (busy || !Visible()) yield break;
            busy = true; RefreshButtons();
            JsonData body = new JsonData(); body["playerId"] = owner; body["sessionHex"] = session;
            if (buy) { body["productId"] = pendingProduct; body["version"] = pendingVersion; body["requestId"] = pendingId; }
            Dictionary<string,string> headers = new Dictionary<string,string>(); headers.Add("Content-Type", "application/json");
            WWW current = new WWW(origin + (buy ? "/ggo-shop/buy" : "/ggo-shop/catalogue"), Encoding.UTF8.GetBytes(JsonMapper.ToJson(body)), headers);
            request = current; float deadline = Time.realtimeSinceStartup + 15f;
            while (!current.isDone && Time.realtimeSinceStartup < deadline) yield return null;
            bool success = false;
            string message = buy ? "兑换结果待确认，点击此处重试" : "加载失败，点击此处重试";
            try
            {
                if (Visible() && current.isDone && String.IsNullOrEmpty(current.error))
                {
                    JsonData response = JsonMapper.ToObject(current.text);
                    if ((int)response["code"] == 0)
                    {
                        JsonData data = response["data"];
                        if (buy)
                        {
                            if ((string)data["requestId"] != pendingId) throw new Exception("receipt differs");
                            string reward = (string)data["rewardId"];
                            string points = data["terminalPointsAdded"].ToString();
                            message = "兑换成功：" + TableTextBlo.GetText(CommonMethod.GetObjectNameTextId(reward)) + " ×" + data["rewardAmount"].ToString();
                            if (points != "0") message += "，" + points + " 超频点数已到账";
                            // A receipt may be replayed long after purchase; its catalogue is historical.
                            // Settle the UUID once, then obtain live balances and limits through catalogue.
                            confirmedNotice = message;
                            PlayerPrefs.DeleteKey(PreferenceKey()); PlayerPrefs.Save(); pendingId = pendingProduct = pendingVersion = "";
                            enabledShop = false; version = "";
                            balanceLabel.text = currencyName + "：余额刷新中…";
                        }
                        else
                        {
                            Render(data);
                            message = !String.IsNullOrEmpty(confirmedNotice) ? confirmedNotice :
                                (String.IsNullOrEmpty(pendingId) ? (enabledShop ? "选择道具查看详情，每次兑换 1 份" : "荒野兑换所暂未开放") : "上次兑换待确认，点击此处重试");
                        }
                        success = true;
                    }
                    else
                    {
                        // A rejected response is definitive: no debit occurred; a new catalogue can be requested.
                        message = ((IDictionary)response).Contains("msg") ? response["msg"].ToString() : "兑换未完成，请刷新后重试";
                        if (buy) { PlayerPrefs.DeleteKey(PreferenceKey()); PlayerPrefs.Save(); pendingId = pendingProduct = pendingVersion = ""; }
                    }
                }
            }
            catch (Exception) { message = buy ? "兑换结果待确认，点击此处重试" : "加载失败，点击此处重试"; }
            current.Dispose(); if (request == current) request = null;
            busy = false;
            if (!Visible()) yield break;
            if (!success && !buy)
            {
                enabledShop = false;
                if (!String.IsNullOrEmpty(confirmedNotice)) message = confirmedNotice + "；余额刷新失败，点击此处重试";
            }
            SetNotice(pendingUnreadable ? "兑换记录无法读取，请联系管理员核对" : message); RefreshButtons();
            if (success && buy) StartCoroutine(Fetch(false));
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
            credit.num = absoluteBalance;
            version = (string)catalogue["version"]; enabledShop = (bool)catalogue["enabled"];
            currency = (string)catalogue["currencyId"]; currencyName = (string)catalogue["currencyName"];
            balanceLabel.text = (string)catalogue["name"] + " · " + currencyName + "：" + catalogue["balance"].ToString();
            balanceIcon.SetData(currency, true, -1, null);
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
        void RefreshButtons() { foreach (GGOProductRow row in rows) if (row != null) row.SetBusy(busy || !enabledShop || pendingUnreadable || !String.IsNullOrEmpty(pendingId)); }

        // Filter only positive terminal tickets at the real inventory merge. The caller's result list remains untouched.
        public static List<SimpleItem> InventoryItems(List<SimpleItem> source)
        {
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
        GGOShop shop; string product; GameObject button, soldLabel; UILabel buttonLabel; bool available;
        public void Bind(GGOShop owner, ShopScoreItem original, JsonData data, string currency, string currencyName)
        {
            shop = owner; product = (string)data["id"]; button = original.btnBuy;
            original.labName.text = (string)data["name"];
            original.discountRoot.SetActive(false); original.twoCostRoot.SetActive(false); original.oneCostRoot.SetActive(true);
            original.labCostName.text = currencyName; original.labPrice.text = data["price"].ToString();
            soldLabel = original.labBuyed;
            UILabel soldText = soldLabel.GetComponent<UILabel>(); RemoveTextKit(soldText); if (soldText != null) soldText.text = "已兑换";
            buttonLabel = button.GetComponentInChildren<UILabel>(); RemoveTextKit(buttonLabel);
            long remaining = Int64.Parse(data["remaining"].ToString()); available = remaining != 0;
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
        public void OnBuyBtn() { if (available && shop != null) shop.Buy(product); }
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
