using System;
using System.Collections.Generic;
using System.Reflection;
using System.Globalization;
using UnityEngine;
using UI;
using ProtoBuf.Message;
using ProtoBuf;
using NetWork.Layer;
using NetWork.Packets;

namespace HYJS.BulkPurchase
{
    public sealed class QuantitySession : MonoBehaviour
    {
        public List<BaseResource> Costs;
        public List<BaseResource> Gains;
        public List<BaseResource> Curve;
        public int Maximum;
        public bool Busy;
        public Action<int, HttpHandler> Send;
        public Action<Packet, bool> Completed;
        public Action<int> Confirm;
        public Component Owner;
        public object OwnerData;
        public long OwnerPlayer;
        public object SourceQuote;
        public string QuoteField;
        public bool DestroyOnHide;
        public string PendingKey;
        void OnDisable() { if (DestroyOnHide) UnityEngine.Object.Destroy(gameObject); }
    }

    public sealed class UFPurchaseScope : MonoBehaviour
    {
        void OnDisable() { BulkPurchase.UnionFightClosed(GetComponent<NPCUIShopPage>()); }
    }

    public static class BulkPurchase
    {
        const BindingFlags Flags = BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic;
        static readonly Dictionary<int, bool> BusyPages = new Dictionary<int, bool>();
        static GameObject NativeQuantityPrefab;
        sealed class UFPending
        {
            public NPCUIShopPage Page;
            public object Player;
            public long PlayerId;
            public byte[] Session;
            public object Room;
            public float Deadline;
            public float RefreshAfter;
        }
        static readonly Dictionary<string, UFPending> PendingUF = new Dictionary<string, UFPending>();
        static byte[] SessionBytes()
        {
            FieldInfo f = typeof(StaticData).GetField("sessionKey", BindingFlags.Static | BindingFlags.Public | BindingFlags.NonPublic);
            return f == null ? null : f.GetValue(null) as byte[];
        }
        static bool SameUF(UFPending p, NPCUIShopPage page)
        {
            byte[] now = SessionBytes();
            if (p.Page == null || p.Page != page || !p.Page.gameObject.activeInHierarchy || p.Player != StaticData.playerData ||
                p.PlayerId != StaticData.playerData.playerId || !System.Object.ReferenceEquals(p.Room, UIUnionFightRunMap.GetNetData()) ||
                (now == null) != (p.Session == null)) return false;
            if (now != null) { if (now.Length != p.Session.Length) return false; for (int i = 0; i < now.Length; i++) if (now[i] != p.Session[i]) return false; }
            return true;
        }
        static void SweepUF()
        {
            var dead = new List<string>();
            foreach (var kv in PendingUF) if (!SameUF(kv.Value, kv.Value.Page)) dead.Add(kv.Key);
            foreach (string key in dead) PendingUF.Remove(key);
        }
        public static void UnionFightClosed(NPCUIShopPage page)
        {
            var dead = new List<string>();
            foreach (var kv in PendingUF) if (kv.Value.Page == page) dead.Add(kv.Key);
            foreach (string key in dead) PendingUF.Remove(key);
        }
        sealed class HttpPurchase
        {
            public object Player; public long PlayerId; public byte[] Session;
            public object Quote; public Component Owner; public HttpHandler Handler;
            public float RefreshAfter;
        }
        static readonly Dictionary<string, HttpPurchase> PendingHttp = new Dictionary<string, HttpPurchase>();
        static bool SameHttpScope(HttpPurchase p)
        {
            byte[] now = SessionBytes();
            if (p.Player != StaticData.playerData || p.PlayerId != StaticData.playerData.playerId || (now == null) != (p.Session == null)) return false;
            if (now != null) { if (now.Length != p.Session.Length) return false; for (int i = 0; i < now.Length; i++) if (now[i] != p.Session[i]) return false; }
            return true;
        }
        static object NativeHttpHandler() { return Value(HTTPManager.Instance, "m_dLastSendHandler"); }
        static string PurchaseKey(Component owner, object quote)
        {
            object id = Value(quote, "id") ?? Value(quote, "productId") ?? Value(quote, "index") ?? Value(quote, "itemId") ?? Value(Value(owner, "halfPriceActivity"), "id");
            return owner.GetType().FullName + ":" + (Value(owner, "activityId") ?? String.Empty) + ":" + (id == null ? owner.GetInstanceID().ToString() : Convert.ToString(id));
        }
        static bool HttpBlocked(Component owner, object quote, string key, bool refresh)
        {
            var dead = new List<string>(); foreach (var kv in PendingHttp) if (!SameHttpScope(kv.Value)) dead.Add(kv.Key);
            foreach (string d in dead) PendingHttp.Remove(d);
            HttpPurchase p; if (!PendingHttp.TryGetValue(key, out p)) return false;
            Delegate last = NativeHttpHandler() as Delegate;
            bool ownsLast = p.Handler != null ? System.Object.ReferenceEquals(last, p.Handler) : last != null && System.Object.ReferenceEquals(last.Target, p.Owner);
            if (!System.Object.ReferenceEquals(p.Quote, quote) && !NetAPI.IsSendingHttp() && !ownsLast)
            { PendingHttp.Remove(key); return false; }
            if (refresh && !NetAPI.IsSendingHttp())
            {
                Tip("购买结果尚未确认，请刷新商店核对");
                if (Time.realtimeSinceStartup >= p.RefreshAfter)
                {
                    p.RefreshAfter = Time.realtimeSinceStartup + 5f;
                    if (owner is UnionShopTabItem) Invoke(Value(owner, "unionShopTab"), "Init");
                    else if (owner is ActivityAwardItem || owner is HalfPrice)
                    { var seven = PageManager.Instance.GetDictPage("SevenDayPage"); if (seven != null) Invoke(seven, "AskServerActivity"); }
                    else if (owner is BuyShopItem || owner is ShopBatchBuy || owner is ShopPetItem)
                    { var shop = PageManager.Instance.GetDictPage("ShopPage") as ShopPage; if (shop != null && shop.gameObject.activeInHierarchy) Invoke(shop, "UpdateCurTab"); }
                    // No guessed refresh API for activity panels: original timeout dialog retains its exact packet retry.
                }
            }
            return true;
        }
        static HttpPurchase BeginHttp(Component owner, object quote, string key)
        {
            byte[] session = SessionBytes();
            var p = new HttpPurchase { Owner = owner, Quote = quote, Player = StaticData.playerData, PlayerId = StaticData.playerData.playerId, Session = session == null ? null : (byte[])session.Clone() };
            PendingHttp[key] = p; return p;
        }
        static void EndHttp(string key, HttpPurchase p)
        { HttpPurchase current; if (key != null && PendingHttp.TryGetValue(key, out current) && System.Object.ReferenceEquals(current, p)) PendingHttp.Remove(key); }
        static object Value(object owner, string name)
        {
            if (owner == null) return null;
            for (Type t = owner.GetType(); t != null; t = t.BaseType)
            {
                FieldInfo f = t.GetField(name, Flags | BindingFlags.DeclaredOnly);
                if (f != null) return f.GetValue(owner);
                PropertyInfo p = t.GetProperty(name, Flags | BindingFlags.DeclaredOnly);
                if (p != null) return p.GetValue(owner, null);
            }
            return null;
        }
        static T Get<T>(object owner, string name) { object v = Value(owner, name); return v == null ? default(T) : (T)v; }
        static void Put(object owner, string name, object value)
        {
            for (Type t = owner.GetType(); t != null; t = t.BaseType)
            {
                FieldInfo f = t.GetField(name, Flags | BindingFlags.DeclaredOnly);
                if (f != null) { f.SetValue(owner, value); return; }
                PropertyInfo p = t.GetProperty(name, Flags | BindingFlags.DeclaredOnly);
                if (p != null) { p.SetValue(owner, value, null); return; }
            }
            throw new InvalidOperationException("Missing original member " + name);
        }
        static void Invoke(object owner, string name, params object[] args)
        {
            if (owner == null || owner is UnityEngine.Object && (UnityEngine.Object)owner == null) return;
            MethodInfo method = owner.GetType().GetMethod(name, Flags);
            if (method == null) throw new InvalidOperationException("Missing original callback " + name);
            method.Invoke(owner, args);
        }
        static void Tip(string text) { BoxManager.CreatePopupTextBox(text, 0); }
        static string Name(string id)
        {
            if (id == "unionFightToken") return "战争贡献";
            if (id == "item_9710") return "信用点";
            string name = TableTextBlo.GetText(CommonMethod.GetObjectNameTextId(id));
            return String.IsNullOrEmpty(name) ? id : name;
        }
        static long Balance(string id)
        {
            if (id == "unionFightToken")
            {
                var net = UIUnionFightRunMap.GetNetData();
                if (net == null || net.netActor == null || net.netActor.players == null || !net.netActor.players.ContainsKey(StaticData.playerData.playerId)) return 0;
                double n = net.netActor.players[StaticData.playerData.playerId].ownToken;
                return Double.IsNaN(n) || Double.IsInfinity(n) || n < 0 || n > Int64.MaxValue ? 0 : (long)n;
            }
            return CommonMethod.GetObjectHaveNum(id);
        }
        static List<BaseResource> Copy(IList<BaseResource> source, int count)
        {
            var result = new List<BaseResource>();
            if (source != null) foreach (var r in source)
            {
                if (r == null || r.num < 0 || String.IsNullOrEmpty(r.id)) throw new InvalidOperationException("Invalid purchase quote");
                long n = checked((long)r.num * count);
                if (n > Int32.MaxValue) throw new InvalidOperationException("购买数量超过协议上限");
                result.Add(new BaseResource { id = r.id, num = (int)n });
            }
            return result;
        }
        static List<BaseResource> Costs(QuantitySession s, int count)
        {
            if (s.Curve != null && s.Curve.Count > 0)
            {
                if (count < 1 || count > s.Curve.Count) throw new InvalidOperationException("报价已变化，请刷新");
                return Copy(new BaseResource[] { s.Curve[count - 1] }, 1);
            }
            return Copy(s.Costs, count);
        }
        static bool Affordable(IList<BaseResource> costs)
        {
            var totals = new Dictionary<string, long>(StringComparer.Ordinal);
            foreach (var r in costs) { long old; totals.TryGetValue(r.id, out old); totals[r.id] = checked(old + r.num); }
            foreach (var r in totals) if (Balance(r.Key) < r.Value) return false;
            return true;
        }
        static int Maximum(QuantitySession s)
        {
            int cap = Math.Max(0, Math.Min(100, s.Maximum));
            if (s.SourceQuote is ShopItem)
                cap = Math.Min(cap, ShopMaximum((ShopItem)s.SourceQuote, s.Owner is UnionShopTabItem ? 8 : 10));
            if (s.SourceQuote is ActivityShopItem) cap = Math.Min(cap, Math.Max(0, ((ActivityShopItem)s.SourceQuote).left));
            if (s.Curve != null && s.Curve.Count > 0) cap = Math.Min(cap, s.Curve.Count);
            int result = 0;
            for (int n = 1; n <= cap; n++)
            {
                try { Copy(s.Gains, n); if (Affordable(Costs(s, n))) result = n; else break; }
                catch (OverflowException) { break; }
                catch (InvalidOperationException) { break; }
            }
            return result;
        }
        static int ShopMaximum(ShopItem item, int type)
        {
            if (item == null || item.canBuy < 1) return 0;
            int cap = 100;
            if (item.dayTotalTimes > 0) cap = Math.Min(cap, Math.Max(0, item.dayLeftTimes));
            if (item.totalTimes > 0) cap = Math.Min(cap, Math.Max(0, item.totalLeftTimes));
            var curve = Curve(item);
            if (curve != null && curve.Count > 0) cap = Math.Min(cap, curve.Count);
            else if (!String.IsNullOrEmpty(item.perDiscount) && item.perDiscount != "100") cap = Math.Min(cap, 1);
            if (type == 2 || type == 3 || item.condition != null && item.condition.type == "rank") cap = Math.Min(cap, 1);
            return cap;
        }
        static QuantitySession State(BuyShopItem view) { return view.GetComponent<QuantitySession>(); }
        static string ResourceLines(IList<BaseResource> resources)
        {
            var lines = new List<string>();
            if (resources != null) foreach (var r in resources) lines.Add(Name(r.id) + " ×" + r.num);
            return String.Join("\n", lines.ToArray());
        }
        static void Buttons(Component view, int current, int max, bool busy)
        {
            foreach (var button in view.GetComponentsInChildren<UIButton>(true))
            {
                string n = button.gameObject.name;
                if (n == "Add1" || n == "Add10") button.isEnabled = !busy && current < max;
                else if (n == "Minu1" || n == "Minu10") button.isEnabled = !busy && current > 1;
                else if (n == "okButton") button.isEnabled = !busy && current > 0 && current <= max;
            }
        }
        static void CostIcon(BuyShopItem view, UISprite target, string id)
        {
            Transform existing = target.transform.Find("BulkResourceIcon");
            if (existing != null) existing.gameObject.SetActive(false);
            Transform legacyCredit = target.transform.Find("GGOCreditCostIcon");
            if (legacyCredit != null) legacyCredit.gameObject.SetActive(false);
            Transform credit = target.transform.parent == null ? null : target.transform.parent.Find("GGOCreditPopupIcon");
            if (credit != null) credit.gameObject.SetActive(id == "item_9710");
            if (id == "item_9710")
            {
                // Original Show has already created a higher-depth quoted icon
                // for this item. Hide it while the HUD credit glyph is in use.
                Transform quoted = target.transform.Find("SoulFlowerQuotedCost");
                if (quoted != null) quoted.gameObject.SetActive(false);
                Type shop = Type.GetType("HYJS.GGO.GGOShop, SoulFlowerGGOShop", false);
                MethodInfo bind = shop == null ? null : shop.GetMethod("BindCreditPopupIcon", BindingFlags.Public | BindingFlags.Static);
                if (bind != null) { try { bind.Invoke(null, new object[] { target }); return; } catch (Exception error) { Debug.LogWarning("Credit icon fallback: " + error.GetType().Name); } }
                CommonIcon icon = existing == null ? CommonMethod.CreateCommonIcon(target.transform) : existing.GetComponent<CommonIcon>();
                icon.gameObject.name = "BulkResourceIcon"; icon.gameObject.SetActive(true);
                icon.transform.localPosition = Vector3.zero; icon.transform.localScale = Vector3.one * (Math.Max(1, Math.Min(target.width, target.height)) / 128f);
                icon.SetData(id, false, -1, null); target.enabled = false; return;
            }
            target.enabled = true;
            if (id != "unionFightToken") { Invoke(view, "UpdatePriceIcon", target, id); return; }
            var state = State(view);
            var source = state == null ? null : state.Owner as NPCUIShopItem;
            Transform logo = source == null ? null : source.price.transform.parent.Find("logo");
            var sprite = logo == null ? null : logo.GetComponent<UISprite>();
            if (sprite != null) { target.atlas = sprite.atlas; target.spriteName = sprite.spriteName; target.enabled = true; }
        }
        static void Description(UILabel label, UILabel bodyStyle, UILabel priceLabel, IList<BaseResource> gains, IList<BaseResource> prices, int count)
        {
            // Keep the original body label between the item header and native total-price bar.
            // A clipped native NGUI scroll region preserves every long reward/cost without covering controls.
            Transform viewport = label.transform.parent;
            if (viewport.gameObject.name != "BulkDescriptionViewport")
            {
                Transform parent = label.transform.parent;
                Vector3 top = label.transform.localPosition;
                float priceY = parent.InverseTransformPoint(priceLabel.transform.position).y;
                int height = Math.Max(40, Math.Min(76, (int)(top.y - priceY - 18f)));
                UIPanel inherited = NGUITools.FindInParents<UIPanel>(label.gameObject);
                GameObject root = new GameObject("BulkDescriptionViewport");
                root.layer = label.gameObject.layer; root.transform.parent = parent;
                root.transform.localPosition = top; root.transform.localRotation = Quaternion.identity; root.transform.localScale = Vector3.one;
                UIPanel panel = root.AddComponent<UIPanel>();
                panel.depth = inherited == null ? 1 : inherited.depth + 1;
                panel.clipping = UIDrawCall.Clipping.SoftClip;
                int width = Math.Max(220, Math.Min(480, label.width));
                label.width = width;
                panel.baseClipRegion = new Vector4(width * .5f, -height * .5f, width, height);
                panel.clipSoftness = Vector2.zero;
                UIScrollView scroll = root.AddComponent<UIScrollView>();
                scroll.movement = UIScrollView.Movement.Vertical; scroll.contentPivot = UIWidget.Pivot.TopLeft; scroll.disableDragIfFits = true;
                label.transform.parent = root.transform; label.transform.localPosition = Vector3.zero;
                UIDragScrollView drag = label.gameObject.GetComponent<UIDragScrollView>();
                if (drag == null) drag = label.gameObject.AddComponent<UIDragScrollView>();
                drag.scrollView = scroll;
                viewport = root.transform;
            }
            label.color = bodyStyle.color;
            label.fontSize = Math.Max(18, Math.Min(20, bodyStyle.fontSize));
            label.spacingY = 2; label.maxLineCount = 0;
            label.pivot = UIWidget.Pivot.TopLeft; label.alignment = NGUIText.Alignment.Left;
            label.overflowMethod = UILabel.Overflow.ResizeHeight;
            label.transform.localPosition = Vector3.zero;
            string rewards = ResourceLines(gains).Replace("\n", "、");
            string costs = ResourceLines(prices).Replace("\n", "、");
            label.text = "每份：" + rewards + "\n本次 " + count + " 份，总消耗：" + costs;
            NGUITools.AddWidgetCollider(label.gameObject);
            UIScrollView scrolling = viewport.GetComponent<UIScrollView>();
            scrolling.ResetPosition();
        }
        public static bool QuantityPrice(BuyShopItem view)
        {
            var s = State(view);
            ShopItem item = Get<ShopItem>(view, "m_shopItem");
            if (item == null) return false;
            if (s == null) s = view.gameObject.AddComponent<QuantitySession>();
            if (s.Costs == null)
            {
                s.Costs = Copy(item.exchangeList, 1);
                s.Gains = Copy(item.item, 1);
                s.Curve = Curve(item);
                s.Maximum = ShopMaximum(item, Get<int>(view, "m_shopType"));
            }
            int max = Maximum(s);
            int count = Math.Max(0, Math.Min(Get<int>(view, "curSelectNum"), max));
            Put(view, "totalCanBuyNum", max); Put(view, "curSelectNum", count);
            view.labCurCount.text = count.ToString();
            var prices = count == 0 ? Copy(s.Costs, 0) : Costs(s, count);
            view.priceRoot1.SetActive(prices.Count > 0); view.priceRoot2.SetActive(prices.Count > 1);
            if (prices.Count > 0) { view.labTotalPrice1.text = prices[0].num.ToString(); CostIcon(view, view.priceIcon1, prices[0].id); }
            if (prices.Count > 1) { view.labTotalPrice2.text = prices[1].num.ToString(); CostIcon(view, view.priceIcon2, prices[1].id); }
            // Every cost/reward remains visible in the original wrapping description label.
            if (s.Send != null || s.Confirm != null) Description(view.labDesc, view.labHaveNum, view.labTotalPrice1, s.Gains, prices, count);
            Buttons(view, count, max, s.Busy);
            return true;
        }
        public static void Shown(BuyShopItem view)
        {
            var s = State(view);
            if (s != null && s.Send == null && s.Confirm == null) { s.Costs = null; s.Gains = null; s.Busy = false; }
            s = State(view);
            if (s == null) s = view.gameObject.AddComponent<QuantitySession>();
            s.OwnerData = StaticData.playerData; s.OwnerPlayer = StaticData.playerData.playerId;
            QuantityPrice(view);
            object quote = s.Send == null && s.Confirm == null ? Value(view, "m_shopItem") : s.SourceQuote;
            Component owner = s.Owner == null ? view : s.Owner;
            s.PendingKey = PurchaseKey(owner, quote);
            s.Busy = HttpBlocked(owner, quote, s.PendingKey, true);
            QuantityPrice(view);
        }
        public static bool QuantitySubmit(BuyShopItem view)
        {
            var s = State(view);
            QuantityPrice(view); s = State(view);
            int n = Get<int>(view, "curSelectNum");
            if (s == null) return true;
            Component purchaseOwner = s.Owner == null ? view : s.Owner;
            object purchaseQuote = s.Send == null && s.Confirm == null ? Value(view, "m_shopItem") : s.SourceQuote;
            string purchaseKey = PurchaseKey(purchaseOwner, purchaseQuote);
            if (s.Confirm == null && HttpBlocked(purchaseOwner, purchaseQuote, purchaseKey, false)) { s.Busy = true; return true; }
            if (s.Busy || n < 1 || n > Maximum(s) || NetAPI.IsSendingHttp()) return true;
            if (s.OwnerData != StaticData.playerData || s.OwnerPlayer != StaticData.playerData.playerId ||
                s.Owner != null && (!s.Owner.gameObject.activeInHierarchy || s.QuoteField != null && !System.Object.ReferenceEquals(Value(s.Owner, s.QuoteField), s.SourceQuote))) { Tip("当前商品已关闭，请重新选择"); return true; }
            s.Busy = true; Buttons(view, n, Maximum(s), true);
            if (s.Confirm != null) { Action<int> confirmed = s.Confirm; s.Confirm = null; view.Hide(); confirmed(n); return true; }
            HttpPurchase pending = BeginHttp(purchaseOwner, purchaseQuote, purchaseKey); s.PendingKey = purchaseKey;
            if (s.Send == null) return false;
            try
            {
                pending.Handler = delegate(Packet packet, bool ok)
                {
                    EndHttp(purchaseKey, pending);
                    s.Busy = false;
                    if (s.OwnerData != StaticData.playerData || s.OwnerPlayer != StaticData.playerData.playerId) return;
                    if (s.Completed != null) s.Completed(packet, ok);
                    if (view != null) { if (ok) view.Hide(); else QuantityPrice(view); }
                };
                s.Send(n, pending.Handler);
                if (!System.Object.ReferenceEquals(NativeHttpHandler(), pending.Handler)) { EndHttp(purchaseKey, pending); s.Busy = false; QuantityPrice(view); }
            }
            catch (Exception e) { EndHttp(purchaseKey, pending); s.Busy = false; QuantityPrice(view); Tip(e.Message); }
            return true;
        }
        public static void QuantityDone(BuyShopItem view, Packet packet, bool ok)
        {
            var s = State(view); if (s != null) { s.Busy = false; HttpPurchase p; if (s.PendingKey != null && PendingHttp.TryGetValue(s.PendingKey, out p) && System.Object.ReferenceEquals(p.Owner, view)) EndHttp(s.PendingKey, p); }
            if (!ok && view != null) QuantityPrice(view);
            if (ok && s != null && s.OwnerData == StaticData.playerData && s.OwnerPlayer == StaticData.playerData.playerId)
            {
                var shop = PageManager.Instance.GetDictPage("ShopPage") as ShopPage;
                if (shop != null && shop.gameObject.activeInHierarchy) Invoke(shop, "UpdateCurTab");
            }
        }
        static bool Open(Component owner, List<BaseResource> gains, List<BaseResource> costs, int maximum, List<BaseResource> curve, Action<int, HttpHandler> send, Action<Packet, bool> done, Action<int> confirm)
        {
            if (owner == null || gains == null || gains.Count == 0 || costs == null || costs.Count == 0 || maximum < 1) { Tip("当前商品不可购买"); return true; }
            foreach (var r in costs) if (r.num <= 0) { Tip("商品报价无效，请刷新"); return true; }
            object currentQuote = owner is HalfPrice ? Value(owner, "datas") : null;
            foreach (string field in new string[] { "shopItemInfo", "item", "rewardInfo", "activityItem" })
            { object q = Value(owner, field); if (q is ShopItem || q is ActivityShopItem || q is InstUFShopItem || q is OpenServer7DayItem) { currentQuote = q; break; } }
            if (send != null && HttpBlocked(owner, currentQuote, PurchaseKey(owner, currentQuote), true)) return true;
            var source = PageManager.Instance.GetDictPage("ShopPage") as ShopPage;
            GameObject prefab = source == null ? null : Get<GameObject>(source, "batchBuyPref");
            if (prefab == null)
            {
                if (NativeQuantityPrefab == null)
                {
                    // This is the same Resources path used by original PageManager.GetPage, without opening the ShopPage.
                    var native = AssetLoad.ResourceManager.Instance.LoadResource("Pages/ShopPage") as GameObject;
                    var template = native == null ? null : native.GetComponent<ShopPage>();
                    NativeQuantityPrefab = template == null ? null : Get<GameObject>(template, "batchBuyPref");
                }
                prefab = NativeQuantityPrefab;
            }
            if (prefab == null) { Tip("原数量组件暂不可用"); return false; }
            Page quantityPage = PageManager.Instance.currentPage;
            if (owner is ActivityAwardItem && quantityPage is SelectReward)
            {
                // The native picker closes immediately after invoking Receive; keep its quantity shell on the owning activity page.
                Page activityPage = null;
                for (Transform parent = owner.transform; parent != null; parent = parent.parent)
                {
                    activityPage = parent.GetComponent<Page>();
                    if (activityPage != null) break;
                }
                if (!owner.gameObject.activeInHierarchy || activityPage == null || activityPage is SelectReward || !activityPage.gameObject.activeInHierarchy)
                { Tip("当前活动页面已关闭，请重新打开后选择"); return true; }
                quantityPage = activityPage;
            }
            var root = (GameObject)UnityEngine.Object.Instantiate(prefab);
            root.name = "HYJSBulkQuantity";
            root.transform.parent = quantityPage.transform;
            root.transform.localPosition = new Vector3(0, 0, -500); root.transform.localScale = Vector3.one;
            var view = root.GetComponent<BuyShopItem>();
            var s = root.AddComponent<QuantitySession>();
            s.Owner = owner; s.OwnerData = StaticData.playerData; s.OwnerPlayer = StaticData.playerData.playerId;
            foreach (string field in new string[] { "shopItemInfo", "item", "rewardInfo", "activityItem" })
            {
                object quote = Value(owner, field);
                if (quote is ShopItem || quote is ActivityShopItem || quote is InstUFShopItem || quote is OpenServer7DayItem)
                { s.SourceQuote = quote; s.QuoteField = field; break; }
            }
            if (owner is HalfPrice) { s.SourceQuote = Value(owner, "datas"); s.QuoteField = "datas"; }
            s.Costs = costs; s.Gains = gains; s.Maximum = maximum; s.Curve = curve; s.Send = send; s.Completed = done; s.Confirm = confirm; s.DestroyOnHide = true;
            var item = new ShopItem { id = "bulk-view", canBuy = 1 };
            item.item.AddRange(Copy(gains, 1)); item.exchangeList.AddRange(Copy(costs.GetRange(0, Math.Min(2, costs.Count)), 1));
            view.Show(item, 1);
            int max = Maximum(s); Put(view, "curSelectNum", max > 0 ? 1 : 0); QuantityPrice(view);
            return true;
        }
        public static bool SelectQuantity(Component owner, string rewardId, int rewardAmount, string costId, long unitCost, int max, Action<int> confirm)
        {
            if (unitCost < 1 || unitCost > Int32.MaxValue || rewardAmount < 1 || confirm == null) return false;
            return Open(owner, new List<BaseResource> { new BaseResource { id = rewardId, num = rewardAmount } }, new List<BaseResource> { new BaseResource { id = costId, num = (int)unitCost } }, max, null, null, null, confirm);
        }
        public static void OrdinaryClamp(ShopBatchBuy view)
        {
            var product = Get<UIShopProduct>(view, "productInfo"); if (product == null) return;
            int cap = Math.Max(0, Math.Min(100, product.dayLeftTimes));
            int affordable = 0;
            long total = 0;
            for (int n = 1; n <= cap; n++)
            {
                var price = Dict.Blo.TableItemShopPriceBlo.GetShopPrice(product.productId, product.dayTotalTimes - product.dayLeftTimes + n);
                if (price == null) break;
                string[] pair = price.price.Split(','); int value;
                if (pair.Length != 2 || !Int32.TryParse(pair[1], out value) || value < 1) break;
                total += value;
                if (total > Int32.MaxValue || Balance(pair[0]) < total) break;
                affordable = n;
            }
            int count = Math.Max(0, Math.Min(Get<int>(view, "curSelectNum"), affordable));
            bool changed = count != Get<int>(view, "curSelectNum");
            Put(view, "totalCanBuyNum", affordable); Put(view, "curSelectNum", count); view.labCurCount.text = count.ToString();
            if (changed) Invoke(view, "UpdatePrice");
            bool busy; BusyPages.TryGetValue(view.GetInstanceID(), out busy); Buttons(view, count, affordable, busy);
        }
        public static bool OrdinarySubmit(ShopBatchBuy view)
        {
            OrdinaryClamp(view); bool busy; BusyPages.TryGetValue(view.GetInstanceID(), out busy);
            int n = Get<int>(view, "curSelectNum"); if (busy || n < 1 || n > Get<int>(view, "totalCanBuyNum")) return true;
            object quote = Value(view, "productInfo"); string key = PurchaseKey(view, quote);
            if (HttpBlocked(view, quote, key, false) || NetAPI.IsSendingHttp()) return true;
            BeginHttp(view, quote, key); BusyPages[view.GetInstanceID()] = true; return false;
        }
        public static void OrdinaryOpened(ShopBatchBuy view)
        {
            object quote = Value(view, "productInfo");
            bool blocked = HttpBlocked(view, quote, PurchaseKey(view, quote), true);
            if (!blocked) BusyPages.Remove(view.GetInstanceID()); else BusyPages[view.GetInstanceID()] = true;
            OrdinaryClamp(view);
        }
        public static void OrdinaryDone(ShopBatchBuy view)
        {
            BusyPages.Remove(view.GetInstanceID());
            string key = PurchaseKey(view, Value(view, "productInfo")); HttpPurchase p;
            if (PendingHttp.TryGetValue(key, out p) && System.Object.ReferenceEquals(p.Owner, view)) EndHttp(key, p);
        }
        public static bool Point(ShopPointItem view) { return StandardRow(view, Get<ShopItem>(view, "shopItemInfo"), 5); }
        public static bool Popularity(ShopPopularityItem view) { return StandardRow(view, Get<ShopItem>(view, "shopItemInfo"), 4); }
        static bool StandardRow(Component row, ShopItem item, int type)
        {
            if (item == null || item.condition != null && item.condition.type == "rank") return false;
            var shop = Get<ShopPage>(row, "shopPage"); if (shop == null) return false;
            shop.OpenBatchBuyUI(item, type); return true;
        }
        public static bool Pet(ShopPetItem row)
        {
            var item = Get<ShopItem>(row, "shopItemInfo"); if (item == null) return false;
            return Open(row, Copy(item.item, 1), Copy(item.exchangeList, 1), ShopMaximum(item, 10), null,
                delegate(int n, HttpHandler cb) { NetAPI.SendHttp(21521, new CSPetShopBuy { shopItemId = item.id, buyCount = n }, cb); },
                delegate(Packet packet, bool ok) { Invoke(row, "MsgCallback", packet, ok); }, null);
        }
        public static bool Guild(UnionShopTabItem row)
        {
            var item = Get<ShopItem>(row, "item"); if (item == null || item.canBuy != 1) return false;
            // Native list canBuy carries all guild membership/level qualification; server rechecks it atomically.
            return Open(row, Copy(item.item, 1), Copy(item.exchangeList, 1), ShopMaximum(item, 8), null,
                delegate(int n, HttpHandler cb) { NetAPI.SendHttp(23007, new CSShopBuy { shopItemId = item.id, shopType = 8, buyCount = n }, cb); },
                delegate(Packet packet, bool ok) { Invoke(row, "BuyCallback", packet, ok); }, null);
        }
        static void AppendBatch(IExtensible request, int count)
        {
            if (request == null || count < 1 || count > 100) throw new InvalidOperationException("购买数量超过协议上限");
            // This shipped protobuf-net merges scalar GetValues<T>; a List<T> preserves each wire occurrence.
            foreach (List<int> existing in Extensible.GetValues<List<int>>(request, 100))
                if (existing != null && existing.Count != 0) throw new InvalidOperationException("购买请求数量已设置");
            Extensible.AppendValue<int>(request, 100, count);
        }
        static List<BaseResource> Curve(IExtensible item)
        {
            var result = new List<BaseResource>();
            foreach (List<BaseResource> group in Extensible.GetValues<List<BaseResource>>(item, 100))
            {
                if (group == null) throw new InvalidOperationException("商品报价无效，请刷新");
                foreach (BaseResource value in group)
                {
                    if (result.Count >= 100 || value == null || String.IsNullOrEmpty(value.id) || value.num <= 0)
                        throw new InvalidOperationException("商品报价无效，请刷新");
                    result.Add(new BaseResource { id = value.id, num = value.num });
                }
            }
            return result.Count == 0 ? null : result;
        }
        static int ActivityMaximum(ActivityShopItem item, List<BaseResource> curve)
        {
            int cap = item.status == 1 ? Math.Min(100, Math.Max(0, item.left)) : 0;
            if (curve != null && curve.Count > 0) return Math.Min(cap, curve.Count);
            double discount;
            if (!Double.TryParse(item.discount, NumberStyles.Float, CultureInfo.InvariantCulture, out discount) || discount != 1.0) return Math.Min(cap, 1);
            return cap;
        }
        static QuantitySession ActivityState(ActivityItemBatchBuyPage view)
        {
            var item = Get<ActivityShopItem>(view, "productInfo"); if (item == null) return null;
            var s = view.GetComponent<QuantitySession>(); if (s == null) s = view.gameObject.AddComponent<QuantitySession>();
            if (!System.Object.ReferenceEquals(s.SourceQuote, item))
            {
                s.SourceQuote = item; s.OwnerData = StaticData.playerData; s.OwnerPlayer = StaticData.playerData.playerId; s.Busy = false;
            }
            s.PendingKey = PurchaseKey(view, item); s.Busy = HttpBlocked(view, item, s.PendingKey, false);
            s.Costs = Copy(item.costResource, 1); s.Gains = Copy(item.getResource, 1); s.Curve = Curve(item); s.Maximum = ActivityMaximum(item, s.Curve); return s;
        }
        public static void ActivityPrice(ActivityItemBatchBuyPage view, int selected)
        {
            var s = ActivityState(view); if (s == null) return;
            int max = Maximum(s); int n = Math.Max(0, Math.Min(selected, max));
            Put(view, "curSelectNum", n); Put(view, "totalCanBuyNum", max);
            var prices = n == 0 ? Copy(s.Costs, 0) : Costs(s, n);
            long total = 0; foreach (var r in prices) total += r.num;
            Put(view, "totalPriceNum", total <= Int32.MaxValue ? (int)total : 0);
            view.labCurCount.text = n.ToString(); view.labTotalPrice.text = prices.Count == 1 ? prices[0].num.ToString() : ResourceLines(prices);
            Description(view.labDesc, view.labHaveNum, view.labTotalPrice, s.Gains, prices, n);
            Buttons(view, n, max, s.Busy);
        }
        public static bool ActivitySubmit(ActivityItemBatchBuyPage view)
        {
            ActivityPrice(view, Get<int>(view, "curSelectNum")); var s = ActivityState(view); int n = Get<int>(view, "curSelectNum");
            if (s == null) return true;
            Component purchaseOwner = s.Owner == null ? view : s.Owner;
            object purchaseQuote = s.Send == null && s.Confirm == null ? Value(view, "m_shopItem") : s.SourceQuote;
            string purchaseKey = PurchaseKey(purchaseOwner, purchaseQuote);
            if (s.Confirm == null && HttpBlocked(purchaseOwner, purchaseQuote, purchaseKey, false)) { s.Busy = true; return true; }
            if (s.Busy || n < 1 || n > Maximum(s) || NetAPI.IsSendingHttp()) return true;
            if (s.OwnerData != StaticData.playerData || s.OwnerPlayer != StaticData.playerData.playerId) { Tip("角色已变化，请重新打开活动"); return true; }
            var item = Get<ActivityShopItem>(view, "productInfo");
            var request = new CSBuyActivityShop { activityId = Get<string>(view, "activityId"), index = item.index, num = n };
            request.costResources.AddRange(Costs(s, n)); request.getResources.AddRange(Copy(s.Gains, n));
            s.Busy = true; Buttons(view, n, Maximum(s), true);
            if (NetAPI.IsSendingHttp()) { s.Busy = false; return true; }
            string pendingKey = s.PendingKey; HttpPurchase pending = BeginHttp(view, item, pendingKey);
            pending.Handler = delegate(Packet packet, bool ok) { EndHttp(pendingKey, pending); s.Busy = false; if (s.OwnerData != StaticData.playerData || s.OwnerPlayer != StaticData.playerData.playerId) return; Invoke(view, "BuyCallback", packet, ok); if (!ok && view != null) ActivityPrice(view, n); }; NetAPI.SendHttp(82026, request, pending.Handler);
            return true;
        }
        static bool ActivityOpen(OnSailItem row, BaseResource selected)
        {
            var item = Get<ActivityShopItem>(row, "rewardInfo"); if (item == null) return false;
            if (selected == null && item.pickOneOrGetAll != 1) return false;
            var gains = selected == null ? Copy(item.getResource, 1) : Copy(new BaseResource[] { selected }, 1);
            var curve = Curve(item); int cap = ActivityMaximum(item, curve);
            string activity = Get<string>(row, "activityId");
            // The legacy red exchange has a one-purchase proof, even if the catalogue advertises several slots.
            if (activity != null && (activity.StartsWith("red", StringComparison.Ordinal) || activity.StartsWith("p2", StringComparison.Ordinal) || activity.StartsWith("limitExchange_p4_resources_", StringComparison.Ordinal) && (curve == null || curve.Count == 0))) return false;
            return Open(row, gains, Copy(item.costResource, 1), cap, curve,
                delegate(int n, HttpHandler cb)
                {
                    var req = new CSBuyActivityShop { activityId = activity, index = item.index, num = n };
                    req.costResources.AddRange(curve != null && curve.Count >= n ? Copy(new BaseResource[] { curve[n - 1] }, 1) : Copy(item.costResource, n)); req.getResources.AddRange(Copy(gains, n)); NetAPI.SendHttp(82026, req, cb);
                }, delegate(Packet packet, bool ok) { Invoke(row, "OnGetRewardRecv", packet, ok); }, null);
        }
        public static bool OnSail(OnSailItem row) { return ActivityOpen(row, null); }
        public static bool OnSailChoice(OnSailItem row, BaseResource selected) { return ActivityOpen(row, selected); }
        static List<BaseResource> Parse(string text, string chosen)
        {
            var list = new List<BaseResource>();
            foreach (string part in (text ?? "").Split(';'))
            {
                string[] pair = part.Split(','); int amount;
                if (pair.Length == 2 && Int32.TryParse(pair[1], out amount) && amount > 0 && (String.IsNullOrEmpty(chosen) || pair[0] == chosen)) list.Add(new BaseResource { id = pair[0], num = amount });
            }
            return list;
        }
        static bool OpenServer(Component row, Dict.DataModel.TableActivityNewOpen7Day cfg, int bought, string selected, string callback)
        {
            if (cfg == null || (cfg.taskType != 107 && cfg.taskType != 108)) return false;
            if (cfg.taskType == 107 && String.IsNullOrEmpty(selected)) return false;
            int totalLimit;
            if (!Int32.TryParse(cfg.num, out totalLimit)) return false;
            int cap = Math.Max(0, Math.Min(100, totalLimit - bought));
            if (cfg.newPrice < 1 || cfg.newPrice > Int32.MaxValue) return false;
            return Open(row, Parse(cfg.reward, selected), new List<BaseResource> { new BaseResource { id = "diamond", num = (int)cfg.newPrice } }, cap, null,
                delegate(int n, HttpHandler cb) { var req = new CSGetOpenServerReward { itemId = cfg.id, heroId = selected ?? String.Empty }; AppendBatch(req, n); NetAPI.SendHttp(57019, req, cb); },
                delegate(Packet packet, bool ok)
                {
                    Invoke(row, callback, packet, ok);
                    if (ok)
                    {
                        // Refresh the original tab after its native callback; its single-step presentation must not subtract only one from a batch.
                        var seven = PageManager.Instance.GetDictPage("SevenDayPage");
                        if (seven != null && seven.gameObject.activeInHierarchy) Invoke(seven, "AskServerActivity");
                    }
                }, null);
        }
        public static bool OpenAward(ActivityAwardItem row, string chosen)
        {
            var data = Get<OpenServer7DayItem>(row, "activityItem");
            if (data == null || data.state != 1) return false;
            return OpenServer(row, Get<Dict.DataModel.TableActivityNewOpen7Day>(row, "activity"), data.currentNum, chosen, "OnReceiveCallBack");
        }
        public static bool OpenHalf(HalfPrice row)
        {
            if (Get<bool>(row, "activityIsEnd")) return false;
            return OpenServer(row, Get<Dict.DataModel.TableActivityNewOpen7Day>(row, "halfPriceActivity"), Get<int>(row, "currentNum"), null, "OnReceiveCallBack");
        }
        public static bool UnionFight(NPCUIShopItem row)
        {
            var item = Get<InstUFShopItem>(row, "item"); var page = Get<NPCUIShopPage>(row, "page"); var net = UIUnionFightRunMap.GetNetData();
            if (item == null || page == null || net == null || net.netActor == null || Room.RoomUnionFight.IsOnHook) return false;
            SweepUF();
            UFPending pending;
            if (PendingUF.TryGetValue(item.shopItemId, out pending))
            {
                if (Time.realtimeSinceStartup < pending.Deadline) Tip("购买请求处理中");
                else
                {
                    Tip("结果尚未确认，请刷新商店核对");
                    if (Time.realtimeSinceStartup >= pending.RefreshAfter)
                    {
                        pending.RefreshAfter = Time.realtimeSinceStartup + 5f;
                        Invoke(page, "SendCSMsg", (object)null);
                    }
                }
                return true;
            }
            if (page.GetComponent<UFPurchaseScope>() == null) page.gameObject.AddComponent<UFPurchaseScope>();
            var actor = net.netActor.players[StaticData.playerData.playerId];
            if (item.buyCondition == 1 && actor.unionAuthority == 0 || item.buyCondition == 2 && actor.unionAuthority != 2) return false;
            string cost = "unionFightToken";
            if (item.shopItemId.StartsWith("ufq:", StringComparison.Ordinal)) { string[] q = item.shopItemId.Split(':'); if (q.Length != 4) return false; cost = q[1]; }
            var definition = Dict.Blo.DictUnionFightItemBlo.GetOne(item.itemId); int carry;
            if (definition == null || !Int32.TryParse(Convert.ToString(definition.ItemCatchMaxNum), out carry) || carry < 1 || item.num < 1) return false;
            int owned = 0; foreach (var r in actor.items) if (r.id == item.itemId) owned += r.leftUseNum;
            int cap = Math.Min(Math.Min(item.leftBuyCount, Get<int>(row, "restTimes")), Math.Max(0, (carry - owned) / item.num));
            string npc = page.dictData.UFNId + String.Empty;
            return Open(row, new List<BaseResource> { new BaseResource { id = item.itemId, num = item.num } }, new List<BaseResource> { new BaseResource { id = cost, num = item.price } }, cap, null,
                null, null, delegate(int n) { var req = new CSUFPlayerBuyItem { shopItemId = item.shopItemId, npcId = npc }; AppendBatch(req, n); SweepUF(); if (PendingUF.ContainsKey(item.shopItemId)) { Tip("购买请求处理中"); return; }
                    byte[] session = SessionBytes();
                    PendingUF[item.shopItemId] = new UFPending { Page = page, Player = StaticData.playerData, PlayerId = StaticData.playerData.playerId,
                        Session = session == null ? null : (byte[])session.Clone(), Room = net, Deadline = Time.realtimeSinceStartup + 20f };
                    NetAPI.SendSocket(80050, req); });
        }
        public static void UnionFightDone(object value)
        {
            SweepUF();
            var message = value as SCUFPlayerBuyItem;
            if (message != null && message.playerGuid == StaticData.playerData.playerId) PendingUF.Remove(message.shopItemId);
        }
        public static void UnionFightListed(NPCUIShopItem row)
        {
            SweepUF();
            var item = Get<InstUFShopItem>(row, "item");
            if (item != null) PendingUF.Remove(item.shopItemId);
        }
    }
}
