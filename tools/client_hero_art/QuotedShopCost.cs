using System;
using System.Globalization;
using System.Collections.Generic;
using UnityEngine;
using Dict.DataModel;

namespace HYJS.Shop
{
    public sealed class QuotedCostIcon : MonoBehaviour
    {
        public UI.CommonIcon Icon;
        public UIAtlas OriginalAtlas;
        public string OriginalSprite;
        public int OriginalWidth;
        public int OriginalHeight;
        public UIWidget.Pivot OriginalPivot;
    }

    public static class QuotedShopCost
    {
        static bool Quote(string id, out string stable, out bool tier)
        {
            stable = null;
            tier = false;
            if (id == null) return false;
            if (!(id.StartsWith("p4q:", StringComparison.Ordinal) || id.StartsWith("p4qt:", StringComparison.Ordinal) || id.StartsWith("s1q:", StringComparison.Ordinal))) return false;
            string[] parts = id.Split(new char[] { ':' }, 3);
            if (parts.Length != 3 || parts[1].Length != 16 || parts[2].Length == 0) return false;
            foreach (char c in parts[1]) if (!(c >= '0' && c <= '9' || c >= 'a' && c <= 'f')) return false;
            stable = parts[2];
            tier = parts[0] == "p4qt";
            return true;
        }

        public static bool IsQuoted(string id)
        {
            string stable;
            bool tier;
            return Quote(id, out stable, out tier);
        }

        public static void RemoveSuperseded(Dictionary<string, UI.UIShopProduct> cache, List<ProtoBuf.Message.ShopItem> incoming)
        {
            if (cache == null || incoming == null) return;
            HashSet<string> stableIDs = new HashSet<string>();
            HashSet<string> currentQuotes = new HashSet<string>();
            foreach (var item in incoming)
            {
                string stable;
                bool tier;
                if (Quote(item.id, out stable, out tier))
                {
                    stableIDs.Add(stable);
                    currentQuotes.Add(item.id);
                }
            }
            if (stableIDs.Count == 0) return;
            // Copy keys after the entire native merge. No native enumerator is
            // invalidated; unrelated products and every newly-listed quote stay.
            foreach (string key in new List<string>(cache.Keys))
            {
                if (currentQuotes.Contains(key)) continue;
                string stable;
                bool tier;
                if (stableIDs.Contains(key) || Quote(key, out stable, out tier) && stableIDs.Contains(stable)) cache.Remove(key);
            }
        }

        public static string CostName(string id)
        {
            if (id == "stamina") return "体力";
            string value = TableTextBlo.GetText(CommonMethod.GetObjectNameTextId(id));
            return String.IsNullOrEmpty(value) ? "消耗材料" : value;
        }

        // Only missing native categories are intercepted. All original handled
        // inventories keep their exact readers and have no reconstructed count.
        public static long SupplementBalance(string id)
        {
            if (StaticData.playerData == null || id == null) return -1;
            if (id == "energy") return StaticData.playerData.Energy;
            if (id == "stamina") return StaticData.playerData.Stamina;
            if (id == "contribution") return StaticData.playerData.contribution;
            if (id.StartsWith("frag_pet_", StringComparison.Ordinal))
            {
                UI.UIFragItem frag;
                var frags = StaticData.playerData.FragPets;
                return frags != null && frags.TryGetValue(id, out frag) && frag != null ? frag.num : 0;
            }
            if (id.StartsWith("frag_trea_", StringComparison.Ordinal)) return StaticData.playerData.treasureFrag == null ? 0 : StaticData.playerData.treasureFrag.GetFragNumber(id);
            return -1;
        }

        public static TableItemShopPrice Price(string quoted, int ordinal)
        {
            string stable;
            bool tier;
            if (!Quote(quoted, out stable, out tier)) return null;
            UI.UIShopProduct product = StaticData.playerData.GetItemShopProduct(quoted);
            if (product == null || product.price == null || product.price.Count != 1) return null;
            var wire = product.price[0];
            TableItemShopPrice result = new TableItemShopPrice();
            result.shopItemId = quoted;
            result.count = ordinal.ToString(CultureInfo.InvariantCulture);
            result.price = wire.id + "," + wire.num.ToString(CultureInfo.InvariantCulture);
            if (!tier) return result;
            // A sold-out quote is the server's final purchasable unit for display.
            // Inventory use can redraw this page; there is no next sale to validate.
            if (product.dayLeftTimes == 0 && product.dayTotalTimes > 0 && product.canBuy == 0 &&
                (long)ordinal == (long)product.dayTotalTimes + 1)
                return result;

            // The server emits p4qt only for an unchanged original tier curve.
            // Confirm the visible next unit before using that original curve
            // for the batch's later ordinals. Never infer tiers from a name.
            int next = product.dayTotalTimes - product.dayLeftTimes + 1;
            var nativeNext = Dict.Blo.TableItemShopPriceBlo.GetShopPrice(stable, next);
            if (nativeNext == null || nativeNext.price != result.price)
            {
                product.canBuy = 0;
                product.dayLeftTimes = 0;
                BoxManager.CreatePopupTextBox("商品阶梯报价已变化，请刷新商店", 0);
                return result;
            }
            return Dict.Blo.TableItemShopPriceBlo.GetShopPrice(stable, ordinal);
        }

        public static void CostIcon(UISprite original, string id, bool nativeSeven)
        {
            DrawCostIcon(original, id, nativeSeven, true);
        }

        public static void UFIcon(UISprite original, string id)
        {
            // The UF prefab's fixed token glyph is never a coin/diamond glyph.
            DrawCostIcon(original, id, false, false);
        }

        static void DrawCostIcon(UISprite original, string id, bool nativeSeven, bool nativeBasic)
        {
            if (original == null) return;
            QuotedCostIcon state = original.GetComponent<QuotedCostIcon>();
            bool native = nativeBasic && (id == "coin" || id == "diamond") || nativeSeven && (id == "popularity" || id == "score" || id == "point" || id == "item_020" || id == "item_021");
            if (native)
            {
                RestoreIcon(original, false);
                return;
            }
            if (state == null)
            {
                state = original.gameObject.AddComponent<QuotedCostIcon>();
                state.OriginalAtlas = original.atlas;
                state.OriginalSprite = original.spriteName;
                state.OriginalWidth = original.width;
                state.OriginalHeight = original.height;
                state.OriginalPivot = original.pivot;
            }
            // A reused row can switch between a thin resource bar and a square
            // item image. Restore its native box before drawing either kind.
            RestoreGeometry(original, state);
            if (id == "energy" || id == "stamina")
            {
                // Exact source widgets from the original TownPage prefab.
                // Loading its prefab as fallback has no scene/page side effect.
                var town = UI.PageManager.Instance.GetDictPage("TownPage");
                Transform root = town == null ? null : town.transform;
                if (root == null)
                {
                    GameObject prefab = Resources.Load("Pages/TownPage", typeof(GameObject)) as GameObject;
                    if (prefab != null) root = prefab.transform;
                }
                Transform sample = root == null ? null : root.Find("LeftUp/UserHead/" + id);
                UISprite source = sample == null ? null : sample.GetComponent<UISprite>();
                if (source != null)
                {
                    if (state.Icon != null) state.Icon.gameObject.SetActive(false);
                    original.atlas = source.atlas;
                    original.spriteName = id == "stamina" ? "UI_zhujiemian_touxiang_item_01" : "UI_zhujiemian_touxiang_item_02";
                    // Both original atlas rectangles are196x15 resource bars,
                    // so fit their ratio inside the native cost-icon width.
                    original.pivot = UIWidget.Pivot.Center;
                    original.width = state.OriginalWidth;
                    original.height = Math.Max(1, Mathf.RoundToInt(state.OriginalWidth * (15f / 196f)));
                    original.enabled = true;
                    return;
                }
                original.enabled = false;
                if (state.Icon != null) state.Icon.gameObject.SetActive(false);
                Debug.LogWarning("SoulFlower original stamina/energy cost icon is unavailable");
                return;
            }
            if (state.Icon == null)
            {
                state.Icon = CommonMethod.CreateCommonIcon(original.transform);
                state.Icon.gameObject.name = "SoulFlowerQuotedCost";
                state.Icon.transform.localScale = Vector3.one * (Math.Min(original.width, original.height) / 128f);
            }
            original.enabled = false;
            state.Icon.gameObject.SetActive(true);
            state.Icon.SetData(id, false, -1, null);
            if (state.Icon.itemCount != null) state.Icon.itemCount.gameObject.SetActive(false);
        }

        public static void RestoreIcon(UISprite original, bool restoreSprite)
        {
            if (original == null) return;
            original.enabled = true;
            QuotedCostIcon state = original.GetComponent<QuotedCostIcon>();
            if (state != null)
            {
                original.atlas = state.OriginalAtlas;
                RestoreGeometry(original, state);
                if (restoreSprite) original.spriteName = state.OriginalSprite;
                if (state.Icon != null) state.Icon.gameObject.SetActive(false);
            }
        }

        static void RestoreGeometry(UISprite original, QuotedCostIcon state)
        {
            original.width = state.OriginalWidth;
            original.height = state.OriginalHeight;
            original.pivot = state.OriginalPivot;
        }

        public static void List(UI.ShopProductItem view, UI.UIShopProduct product)
        {
            if (!IsQuoted(product.productId) || product.price == null || product.price.Count != 1) { RestoreIcon(view.priceIcon, false); return; }
            string id = product.price[0].id;
            view.labPriceName.text = CostName(id);
            CostIcon(view.priceIcon, id, false);
        }

        public static void Batch(UI.ShopBatchBuy view, UI.UIShopProduct product)
        {
            if (!IsQuoted(product.productId) || product.price == null || product.price.Count != 1) { RestoreIcon(view.priceIcon, false); return; }
            CostIcon(view.priceIcon, product.price[0].id, false);
        }

        public static void Activity(UI.ShopProductItem view, ProtoBuf.Message.ActivityShopItem item)
        {
            if (item.costResource == null || item.costResource.Count != 1) { RestoreIcon(view.priceIcon, false); return; }
            string id = item.costResource[0].id;
            view.labPriceName.text = CostName(id);
            CostIcon(view.priceIcon, id, false);
        }

        public static void BuyPage(UI.BuyItemPage view)
        {
            if (view.curProduct == null || !IsQuoted(view.curProduct.productId) || view.curProduct.price == null || view.curProduct.price.Count != 1) return;
            string id = view.curProduct.price[0].id;
            view.currentCostTokenText = CostName(id);
            view.tokenLabel.text = view.currentCostTokenText + "（持有 " + CommonMethod.GetObjectHaveNum(id) + "）";
        }

        public static bool CanAfford(UI.UIShopProduct product)
        {
            if (product == null || product.price == null || product.price.Count == 0) return false;
            foreach (var cost in product.price)
                if (cost.num < 0 || CommonMethod.GetObjectHaveNum(cost.id) < cost.num) return false;
            return true;
        }
    }
}
