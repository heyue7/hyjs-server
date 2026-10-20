using System;
using System.Collections;
using System.Collections.Generic;
using System.Runtime.CompilerServices;
using UnityEngine;
using Dict.DataModel;

namespace HYJS.Stamina
{
    internal sealed class ReferenceComparer<T> : IEqualityComparer<T> where T : class
    {
        public static readonly ReferenceComparer<T> Instance = new ReferenceComparer<T>();
        public bool Equals(T x, T y) { return object.ReferenceEquals(x, y); }
        public int GetHashCode(T value) { return RuntimeHelpers.GetHashCode(value); }
    }

    /// <summary>
    /// Re-lays the original BuyItemPage for the stamina potion only. Existing
    /// labels, font, colors and buttons are reused; every value comes from the
    /// original item table or the SCShopProductList product.
    /// </summary>
    public static class StaminaShop
    {
        sealed class View
        {
            public UILabel StockCaption;
            public UILabel PriceCaption;
            public UILabel DayCaption;
            public UILabel Status;
            public GameObject UseButton;
            public GameObject BuyButton;
            public string OriginalStockCaption;
            public string OriginalPriceCaption;
            public string OriginalDayCaption;
        }

        static readonly Dictionary<UI.BuyItemPage, View> views =
            new Dictionary<UI.BuyItemPage, View>(ReferenceComparer<UI.BuyItemPage>.Instance);

        public static bool IsStamina(UI.BuyItemPage page)
        {
            if (page == null || String.IsNullOrEmpty(page.currentItemId)) return false;
            TableItem item = Dict.Blo.DictItemBlo.GetTableItem(page.currentItemId);
            return item != null && item.stamina > 0;
        }

        public static void Render(UI.BuyItemPage page)
        {
            if (!IsStamina(page))
            {
                Restore(page);
                return;
            }
            TableItem item = Dict.Blo.DictItemBlo.GetTableItem(page.currentItemId);
            View view;
            if (!views.TryGetValue(page, out view))
            {
                view = Create(page);
                views[page] = view;
            }

            long stock = 0;
            if (StaticData.playerData != null) stock = StaticData.playerData.GetSimpleItemCount(page.currentItemId);

            if (page.titleLabel != null) page.titleLabel.text = "体力药剂";
            if (page.itemNameLabel != null) page.itemNameLabel.text = "体力药剂";
            if (page.itemDescLabel != null) page.itemDescLabel.text = "每瓶恢复" + item.stamina + "点体力，本次1瓶";
            if (page.ownNumLabel != null) page.ownNumLabel.text = stock.ToString() + " 瓶";
            if (page.tokenLabel != null && page.currentCostTokenText != null) page.tokenLabel.text = page.currentCostTokenText;
            if (view.Status != null) view.Status.gameObject.SetActive(true);
            if (view.StockCaption != null) view.StockCaption.text = "库存";
            if (view.PriceCaption != null) view.PriceCaption.text = "单价";
            if (view.DayCaption != null) view.DayCaption.text = "今日剩余";

            bool hasStock = stock > 0;
            if (view.UseButton != null) view.UseButton.SetActive(hasStock);
            if (view.BuyButton != null) view.BuyButton.SetActive(!hasStock);
            if (view.Status != null)
            {
                if (hasStock) view.Status.text = "点击「使用」消耗1瓶，可连续使用";
                else if (page.curProduct != null && page.curProduct.dayLeftTimes <= 0) view.Status.text = "今日购买次数已用完";
                else view.Status.text = "背包中暂无体力药剂，点击「购买」";
            }
        }

        static void Restore(UI.BuyItemPage page)
        {
            if (page == null) return;
            View view;
            if (!views.TryGetValue(page, out view)) return;
            if (view.Status != null) view.Status.gameObject.SetActive(false);
            if (view.UseButton != null) view.UseButton.SetActive(true);
            if (view.BuyButton != null) view.BuyButton.SetActive(true);
            if (view.StockCaption != null && view.OriginalStockCaption != null) view.StockCaption.text = view.OriginalStockCaption;
            if (view.PriceCaption != null && view.OriginalPriceCaption != null) view.PriceCaption.text = view.OriginalPriceCaption;
            if (view.DayCaption != null && view.OriginalDayCaption != null) view.DayCaption.text = view.OriginalDayCaption;
        }

        static View Create(UI.BuyItemPage page)
        {
            View view = new View();
            UILabel[] labels = page.GetComponentsInChildren<UILabel>(true);
            foreach (UILabel label in labels)
            {
                // Captions are fixed prefab text. Match once and retain the
                // original UILabel instead of appending into a narrow cell.
                if (label.text == "拥有") view.StockCaption = label;
                else if (label.text == "购买价格") view.PriceCaption = label;
                else if (label.text == "今日剩余购买次数") view.DayCaption = label;
            }
            if (view.StockCaption != null) view.OriginalStockCaption = view.StockCaption.text;
            if (view.PriceCaption != null) view.OriginalPriceCaption = view.PriceCaption.text;
            if (view.DayCaption != null) view.OriginalDayCaption = view.DayCaption.text;
            Transform di = page.transform.Find("Di");
            if (di != null)
            {
                Transform use = di.Find("UseButtn");
                if (use != null) view.UseButton = use.gameObject;
                Transform buy = di.Find("BuyButton");
                if (buy != null) view.BuyButton = buy.gameObject;
            }
            if (page.itemDescLabel != null)
            {
                Transform parent = page.transform.Find("Di/GameObject");
                if (parent == null) parent = page.transform;
                GameObject clone = UnityEngine.Object.Instantiate(page.itemDescLabel.gameObject) as GameObject;
                clone.name = "StaminaStatusLabel";
                clone.transform.SetParent(parent);
                clone.transform.localPosition = new Vector3(-229f, -57f, 0f);
                clone.transform.localScale = Vector3.one;
                view.Status = clone.GetComponent<UILabel>();
                if (view.Status != null)
                {
                    view.Status.text = String.Empty;
                    view.Status.width = 460;
                    view.Status.height = 18;
                    view.Status.pivot = UIWidget.Pivot.Left;
                    view.Status.fontSize = 16;
                    view.Status.depth = 4;
                    view.Status.leftAnchor.target = null;
                    view.Status.rightAnchor.target = null;
                    view.Status.bottomAnchor.target = null;
                    view.Status.topAnchor.target = null;
                }
            }
            // Original UILocalize.Start runs after the first SetItem; refresh
            // once on the next frame so the first opening retains short captions.
            page.StartCoroutine(RefreshAfterLocalization(page));
            return view;
        }

        static IEnumerator RefreshAfterLocalization(UI.BuyItemPage page)
        {
            yield return null;
            if (page != null && page.gameObject.activeInHierarchy) Render(page);
        }
    }

    /// <summary>
    /// Stop/generation bookkeeping for the original auto sweep. Stop keeps the
    /// in-flight response valid so its rewards are applied, but blocks every
    /// further send. Closing invalidates older responses for a reused page.
    /// </summary>
    public static class StaminaSweep
    {
        sealed class State
        {
            public long Generation;
            public bool Stopped;
        }
        sealed class Request
        {
            public UI.UICopySweep Page;
            public long Generation;
            public string LevelId;
        }
        static long generation;
        static readonly Dictionary<UI.UICopySweep, State> states =
            new Dictionary<UI.UICopySweep, State>(ReferenceComparer<UI.UICopySweep>.Instance);
        static readonly Dictionary<object, Request> requests =
            new Dictionary<object, Request>(ReferenceComparer<object>.Instance);
        // Never evict a submitted request: full capacity blocks further sends.
        const int Capacity = 128;

        public static void Opened(UI.UICopySweep page)
        {
            if (page != null) states[page] = new State { Generation = ++generation };
        }
        public static void Closed(UI.UICopySweep page)
        {
            if (page != null) states.Remove(page);
        }
        public static void Stop(UI.UICopySweep page)
        {
            if (page == null) return;
            State state;
            if (states.TryGetValue(page, out state)) state.Stopped = true;
            page.StopAllCoroutines();
            ShowDone(page);
        }
        static void ShowDone(UI.UICopySweep page)
        {
            if (page.stopBtn != null) page.stopBtn.SetActive(false);
            if (page.submitBtn != null) page.submitBtn.SetActive(true);
        }
        public static bool CanContinue(UI.UICopySweep page)
        {
            State state;
            return page != null && states.TryGetValue(page, out state) && !state.Stopped && !page.needStop;
        }
        public static bool BeginSend(UI.UICopySweep page)
        {
            if (!CanContinue(page)) return false;
            if (requests.Count < Capacity) return true;
            Stop(page);
            return false;
        }
        public static void Capture(object token, UI.UICopySweep page)
        {
            State state;
            if (token == null || !states.TryGetValue(page, out state)) throw new InvalidOperationException("Sweep capture outside open page");
            requests.Add(token, new Request { Page = page, Generation = state.Generation, LevelId = page.levelId });
        }
        // Consume the closure identity once. Old pages still account for actual
        // successful server replies, without touching a reused page or iterator.
        public static bool Accept(object token, UI.UICopySweep page, NetWork.Packets.Packet packet, bool success)
        {
            Request request;
            if (!requests.TryGetValue(token, out request)) return false;
            requests.Remove(token);
            State state;
            if (object.ReferenceEquals(request.Page, page) && states.TryGetValue(page, out state) && state.Generation == request.Generation)
                return true;
            if (success)
            {
                ProtoBuf.Message.SCCopySweep result = (ProtoBuf.Message.SCCopySweep)packet.kBody;
                StaticData.playerData.AddResultObj(result.gain);
                UI.UILevelData level = UI.UICopyData.GetUILevelData(request.LevelId);
                StaticData.playerData.Stamina -= level.level_power_cost;
                UI.UICopyData.AddCurrentLevelChallengeCount(request.LevelId, 1);
                StaticData.playerData.AddPetAddIntimacy(result.petAdds);
                if (!String.IsNullOrEmpty(result.wantedId)) BoxManager.CreateMeetBossBox(result.wantedId);
            }
            return false;
        }
        public static bool CanStartNext(UI.UICopySweep page) { return CanContinue(page); }
    }
}
