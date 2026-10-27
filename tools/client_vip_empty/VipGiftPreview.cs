using System;
using System.Text;
using UI;
using ProtoBuf.Message;
using Dict.Blo;
using Dict.DataModel;

namespace HYJS.VipShop
{
    public static class VipGiftPreview
    {
        const string OriginalVIP7Contents = "item_050,2;item_009,50;item_012,50;item_001,10;coin,1000000";

        // Only the reviewed VIP7 parcel plus its directly granted gem book needs
        // a combined preview. Returning false keeps the original icon handler.
        public static bool TryOpen(ShopItem item)
        {
            try
            {
                if (item == null || item.item == null || item.item.Count < 2 ||
                    item.item[0] == null || item.item[0].id != "item_041" || item.item[0].num <= 0)
                    return false;
                bool hasBook = false;
                for (int i = 1; i < item.item.Count; i++)
                {
                    BaseResource resource = item.item[i];
                    if (resource == null || resource.num <= 0 || !SafeID(resource.id)) return false;
                    if (resource.id == "item_063") hasBook = true;
                }
                if (!hasBook) return false;
                TableItemUse box = DictItemBlo.GetTableItemUse("item_041");
                if (box == null || box.result != OriginalVIP7Contents) return false;
                StringBuilder contents = new StringBuilder();
                foreach (string entry in OriginalVIP7Contents.Split(';'))
                {
                    string[] pair = entry.Split(',');
                    int quantity = checked(Int32.Parse(pair[1]) * item.item[0].num);
                    Append(contents, pair[0], quantity);
                }
                for (int i = 1; i < item.item.Count; i++)
                    Append(contents, item.item[i].id, item.item[i].num);
                PageManager.Instance.OpenPage("ShopGiftPreview", "type=3&activityGetGiftId=" + contents.ToString());
                return true;
            }
            catch (Exception) { return false; }
        }

        static bool SafeID(string id)
        {
            if (String.IsNullOrEmpty(id)) return false;
            foreach (char c in id)
                if (!(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') &&
                    !(c >= '0' && c <= '9') && c != '_') return false;
            return true;
        }

        static void Append(StringBuilder contents, string id, int quantity)
        {
            if (contents.Length > 0) contents.Append(';');
            contents.Append(id).Append(',').Append(quantity.ToString(System.Globalization.CultureInfo.InvariantCulture));
        }
    }
}
