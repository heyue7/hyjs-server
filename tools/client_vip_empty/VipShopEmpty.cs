using System;
using UnityEngine;
using UI;
using NetWork.Packets;
using ProtoBuf.Message;

namespace HYJS.VipShop
{
    public static class VipShopEmpty
    {
        // A successful empty native catalogue has an explicit player-facing state.
        public static void Received(TabVipShop tab, Packet packet, bool success)
        {
            try
            {
                if (tab == null) return;
                SCShopProductList response = packet == null ? null : packet.kBody as SCShopProductList;
                if (success && response != null && response.shopType == 2)
                    RefreshPrices(tab, response);
                bool empty = success && response != null && response.shopType == 2 && response.shopItemList.Count == 0;
                VipShopEmptyControl control = tab.GetComponent<VipShopEmptyControl>();
                if (control == null && empty) control = tab.gameObject.AddComponent<VipShopEmptyControl>();
                if (control != null) control.Display(tab, empty);
            }
            catch (Exception) { Debug.LogWarning("Member gift empty state could not render"); }
        }

        // The native callback sorts the response and creates grid children in that order.
        // Keep the actual (Discount) price and the native purchase handlers intact.
        static void RefreshPrices(TabVipShop tab, SCShopProductList response)
        {
            if (tab.simpleGrid == null || tab.simpleGrid.childs.Count != response.shopItemList.Count) return;
            for (int i = 0; i < response.shopItemList.Count; i++)
            {
                GameObject child = tab.simpleGrid.childs[i];
                ShopVipItem card = child == null ? null : child.GetComponent<ShopVipItem>();
                ShopItem item = response.shopItemList[i];
                if (card == null || card.labPriceBefore == null || card.labPriceAfter == null || item == null) continue;
                Transform row = card.labPriceBefore.transform.parent;
                // The original prefab places the label and strike sprite in PriceBefore.
                // Refuse an unexpected hierarchy rather than hide a whole product card.
                if (row == null || row.name != "PriceBefore" || row.parent != card.transform ||
                    card.labPriceAfter.transform.parent == row) continue;
                bool same = item.price.Count == 1 && item.discount.Count == 1 &&
                    item.price[0] != null && item.discount[0] != null &&
                    item.price[0].id == item.discount[0].id &&
                    item.price[0].num == item.discount[0].num;
                row.gameObject.SetActive(!same);
            }
        }
    }

    public sealed class VipShopEmptyControl : MonoBehaviour
    {
        GameObject message;
        public void Display(TabVipShop tab, bool visible)
        {
            if (message == null && visible)
            {
                UILabel theme = null;
                if (tab.shopItemPrefab != null)
                    foreach (UILabel label in tab.shopItemPrefab.GetComponentsInChildren<UILabel>(true))
                        if (label.bitmapFont != null || label.trueTypeFont != null) { theme = label; break; }
                if (theme == null)
                    foreach (UILabel label in tab.GetComponentsInChildren<UILabel>(true))
                        if (label.bitmapFont != null || label.trueTypeFont != null) { theme = label; break; }
                if (theme == null) return;
                Transform parent = tab.scrollView == null ? tab.transform : tab.scrollView.transform;
                UIPanel panel = parent.GetComponent<UIPanel>();
                Vector4 region = panel == null ? new Vector4(0f, 0f, 600f, 400f) : panel.baseClipRegion;
                message = new GameObject("MemberGiftEmptyState");
                message.layer = theme.gameObject.layer;
                message.transform.parent = parent;
                message.transform.localScale = Vector3.one;
                message.transform.localPosition = new Vector3(region.x, region.y, 0f);
                UILabel caption = message.AddComponent<UILabel>();
                caption.bitmapFont = theme.bitmapFont; caption.trueTypeFont = theme.trueTypeFont;
                caption.fontStyle = theme.fontStyle; caption.fontSize = 28;
                caption.width = region.z > 80f ? Math.Min(580, Math.Max(300, (int)region.z - 32)) : 480;
                caption.height = 76; caption.pivot = UIWidget.Pivot.Center;
                caption.alignment = NGUIText.Alignment.Center; caption.color = new Color(.86f, .86f, .86f, 1f);
                caption.effectStyle = UILabel.Effect.Shadow; caption.effectColor = Color.black;
                int depth = theme.depth;
                foreach (UIWidget widget in parent.GetComponentsInChildren<UIWidget>(true)) depth = Math.Max(depth, widget.depth);
                caption.depth = depth + 10; caption.text = "暂无开放的会员礼包";
            }
            if (message != null) message.SetActive(visible);
        }
        void OnDestroy() { if (message != null) UnityEngine.Object.Destroy(message); }
    }
}
