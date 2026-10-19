using System;
using UnityEngine;
using ProtoBuf.Message;

namespace HYJS.Shop
{
    // The original UF wire message has an integer price and a string SKU. A
    // quoted SKU adds resource identity without changing protobuf field IDs.
    public sealed class UFQuotedCost : MonoBehaviour
    {
        public string QuotedSKU;
        string resource;
        UILabel balanceLabel;
        float nextRefresh;

        static int Parse(string sku, out string cost)
        {
            cost = null;
            if (sku == null || !sku.StartsWith("ufq:", StringComparison.Ordinal)) return 0;
            string[] parts = sku.Split(new char[] { ':' }, 4);
            if (parts.Length != 4 || parts[1].Length == 0 || parts[2].Length != 16 || parts[3].Length == 0) return -1;
            foreach (char c in parts[2]) if (!(c >= '0' && c <= '9' || c >= 'a' && c <= 'f')) return -1;
            foreach (char c in parts[1]) if (!(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_')) return -1;
            cost = parts[1];
            return 1;
        }

        static string CostName(string cost)
        {
            return QuotedShopCost.CostName(cost);
        }

        static long Balance(string cost)
        {
            return CommonMethod.GetObjectHaveNum(cost);
        }

        public static void Render(UI.NPCUIShopItem view, InstUFShopItem item)
        {
            string cost;
            int status = Parse(item.shopItemId, out cost);
            UFQuotedCost state = view.GetComponent<UFQuotedCost>();
            Transform priceGroup = view.price.transform.parent;
            Transform oldLogo = priceGroup.Find("logo");
            if (status == 0 || status == 1 && cost == "unionFightToken")
            {
                if (oldLogo != null) oldLogo.gameObject.SetActive(true);
                if (oldLogo != null) QuotedShopCost.RestoreIcon(oldLogo.GetComponent<UISprite>(), true);
                if (state != null) state.Deactivate();
                return;
            }
            // Never leave the original token image beside a material price.
            if (oldLogo != null) oldLogo.gameObject.SetActive(false);
            if (state == null) state = view.gameObject.AddComponent<UFQuotedCost>();
            state.QuotedSKU = item.shopItemId;
            state.resource = status == 1 ? cost : null;
            if (status != 1)
            {
                state.Deactivate();
                view.price.text = "报价无效";
                return;
            }
            if (oldLogo != null)
            {
                oldLogo.gameObject.SetActive(true);
                QuotedShopCost.UFIcon(oldLogo.GetComponent<UISprite>(), cost);
            }
            if (state.balanceLabel == null)
            {
                GameObject label = (GameObject)UnityEngine.Object.Instantiate(view.price.gameObject);
                label.name = "SoulFlowerCostBalance";
                label.transform.parent = priceGroup;
                label.transform.localPosition = new Vector3(0, -22, 0);
                label.transform.localScale = Vector3.one;
                state.balanceLabel = label.GetComponent<UILabel>();
                state.balanceLabel.pivot = UIWidget.Pivot.Center;
                state.balanceLabel.width = 260;
                state.balanceLabel.height = 20;
                state.balanceLabel.fontSize = 14;
            }
            state.balanceLabel.gameObject.SetActive(true);
            state.RefreshBalance();
        }

        void Deactivate()
        {
            resource = null;
            if (balanceLabel != null) balanceLabel.gameObject.SetActive(false);
        }

        void Update()
        {
            if (resource != null && Time.unscaledTime >= nextRefresh) RefreshBalance();
        }

        void RefreshBalance()
        {
            nextRefresh = Time.unscaledTime + .5f;
            if (resource != null && balanceLabel != null)
                balanceLabel.text = CostName(resource) + " · 持有 " + Balance(resource);
        }

        static bool Eligible(InstUFShopItem item, int identity, int left, string cost)
        {
            var room = UI.UIUnionFightRunMap.GetNetData();
            if (room == null || room.netActor == null || room.netActor.players == null || StaticData.playerData == null || !room.netActor.players.ContainsKey(StaticData.playerData.playerId))
            {
                BoxManager.CreatePopupTextBox("当前商店已关闭，请重新进入", 0);
                return false;
            }
            var player = room.netActor.players[StaticData.playerData.playerId];
            if (identity == 1 && player.unionAuthority == 0)
            {
                BoxManager.CreatePopupTextBox(TableTextBlo.GetText("union_fight_buy_item_limit0"), 0);
                return false;
            }
            if (identity == 2 && player.unionAuthority != 2)
            {
                BoxManager.CreatePopupTextBox(TableTextBlo.GetText("union_fight_buy_item_limit1"), 0);
                return false;
            }
            if (Balance(cost) < item.price)
            {
                BoxManager.CreatePopupTextBox(CostName(cost) + "不足", 0);
                return false;
            }
            if (left <= 0)
            {
                BoxManager.CreatePopupTextBox(TableTextBlo.GetText("union_fight_no_buy_times"), 0);
                return false;
            }
            return true;
        }

        public static bool TryBuy(UI.NPCUIShopItem view, InstUFShopItem item, UI.NPCUIShopPage page, int identity, int left)
        {
            string cost;
            int status = Parse(item.shopItemId, out cost);
            if (status == 0 || status == 1 && cost == "unionFightToken") return false;
            if (status != 1 || item.price < 0)
            {
                BoxManager.CreatePopupTextBox("商品报价无效，请重新打开商店", 0);
                return true;
            }
            if (!Eligible(item, identity, left, cost)) return true;
            string sku = item.shopItemId;
            string npc = page.dictData.UFNId + String.Empty;
            string product = TableTextBlo.GetText(Dict.Blo.DictUnionFightItemBlo.GetOne(item.itemId).ItemName);
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton(
                "消耗 " + CostName(cost) + " ×" + item.price + " 购买 " + product + " ×" + item.num + "？\n当前持有：" + Balance(cost),
                delegate(bool confirmed)
                {
                    if (!confirmed || view == null || !view.gameObject.activeInHierarchy) return;
                    UFQuotedCost current = view.GetComponent<UFQuotedCost>();
                    if (current == null || current.QuotedSKU != sku)
                    {
                        BoxManager.CreatePopupTextBox("商品报价已更新，请重新选择", 0);
                        return;
                    }
                    if (!Eligible(item, identity, left, cost)) return;
                    UI.UIUnionFightRunMap.GetNetData().SendCSUFPlayerBuyItem(sku, npc);
                });
            return true;
        }
    }
}
