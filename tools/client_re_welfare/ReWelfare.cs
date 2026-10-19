using System;
using System.Globalization;
using UnityEngine;
using UI;
using ProtoBuf.Message;
using NetWork.Packets;

namespace HYJS.ReWelfare
{
    public sealed class CatReceiptMemory : MonoBehaviour
    {
        public UIPlayerData Owner;
        public string LastReply;
    }
    public sealed class ContinuedQuoteState : MonoBehaviour
    {
        public int CurrentStatus;
    }

    public static class ReWelfareBridge
    {
        static bool Cat(UIActivityTreasureCast page)
        { return page != null && !String.IsNullOrEmpty(page.activityId) && page.activityId.StartsWith("fortuneCat_re_", StringComparison.Ordinal); }
        static bool Continued(UIActivityContinueChargeGift page)
        { return page != null && !String.IsNullOrEmpty(page.activityId) && page.activityId.StartsWith("continuedRecharge_re_", StringComparison.Ordinal); }

        public static bool BeforeCatReply(UIActivityTreasureCast page, ActivityFortuneCatData prior, Packet packet, bool success)
        {
            if (!success || !Cat(page)) return true;
            SCFortuneCatReward reply = packet == null ? null : packet.kBody as SCFortuneCatReward;
            if (prior == null || reply == null || reply.info == null || StaticData.playerData == null) return false;
            CatReceiptMemory memory = page.GetComponent<CatReceiptMemory>();
            if (memory == null) memory = page.gameObject.AddComponent<CatReceiptMemory>();
            string identity = reply.info.lastGetTime.ToString(CultureInfo.InvariantCulture) + ":" + reply.info.todayLeft.ToString(CultureInfo.InvariantCulture);
            if (System.Object.ReferenceEquals(memory.Owner, StaticData.playerData) && memory.LastReply == identity) return false;
            // The original callback only adds Get.simps. Apply the accepted cost
            // first, then leave the original full return, popup and animation intact.
            if (prior.coinPool <= 0) return false;
            StaticData.playerData.SubDiamond((long)prior.coinPool);
            memory.Owner = StaticData.playerData;
            memory.LastReply = identity;
            return true;
        }

        static UILabel NativeLabel(GameObject parent, string name, UILabel template)
        {
            if (parent == null || template == null) return null;
            Transform found = parent.transform.Find(name);
            if (found != null) return found.GetComponent<UILabel>();
            GameObject go = new GameObject(name);
            go.layer = parent.layer;
            go.transform.parent = parent.transform;
            go.transform.localPosition = Vector3.zero;
            go.transform.localRotation = Quaternion.identity;
            go.transform.localScale = Vector3.one;
            UILabel label = go.AddComponent<UILabel>();
            // Keep the game's UIFont; assigning trueTypeFont afterwards would clear it.
            if (template.bitmapFont != null) label.bitmapFont = template.bitmapFont;
            else label.trueTypeFont = template.trueTypeFont;
            label.material = template.material;
            label.effectStyle = template.effectStyle;
            label.effectColor = template.effectColor;
            label.effectDistance = template.effectDistance;
            label.fontSize = 20;
            label.depth = template.depth + 5;
            label.pivot = UIWidget.Pivot.Center;
            label.alignment = NGUIText.Alignment.Center;
            label.supportEncoding = false;
            label.width = 260;
            label.height = 40;
            label.overflowMethod = UILabel.Overflow.ShrinkContent;
            label.color = Color.white;
            return label;
        }

        static long ReturnQuote(UIActivityTreasureCast page, ActivityFortuneCatData data)
        {
            string[] parts = page.activityId.Split(new char[] { '~' });
            int used = 5 - data.todayLeft;
            if (parts.Length != 6 || used < 0 || used >= 5) return 0;
            string[] values = parts[used + 1].Split(new char[] { ',' });
            long cost, payout;
            if (values.Length != 2 || !Int64.TryParse(values[0], NumberStyles.None, CultureInfo.InvariantCulture, out cost) ||
                !Int64.TryParse(values[1], NumberStyles.None, CultureInfo.InvariantCulture, out payout) || cost != data.coinPool || payout < cost) return 0;
            return payout;
        }

        public static void BindCat(UIActivityTreasureCast page, ActivityFortuneCatData data)
        {
            if (!Cat(page) || data == null) return;
            // The old once-per-second coroutine otherwise re-enables an exhausted button.
            page.StopCoroutine("UpdateUI");
            if (page.time != null) page.time.SetActive(false);
            if (page.solgon != null) page.solgon.SetActive(false);
            if (page.solgon2 != null) page.solgon2.SetActive(false);
            int left = Math.Max(0, Math.Min(5, data.todayLeft));
            if (page.surplusNumber != null)
            {
                page.surplusNumber.width = 260;
                page.surplusNumber.fontSize = 18;
                page.surplusNumber.text = "今日剩余 " + left.ToString(CultureInfo.InvariantCulture) + " / 5";
            }
            long payout = ReturnQuote(page, data);
            if (page.totlaCoin != null)
            {
                page.totlaCoin.width = 310;
                page.totlaCoin.fontSize = 20;
                page.totlaCoin.text = left == 0 ? "今日招财已完成" : "本次消耗 " + data.coinPool.ToString(CultureInfo.InvariantCulture) + " 钻";
            }
            if (page.coinCount != null)
            {
                page.coinCount.width = 390;
                page.coinCount.fontSize = 18;
                page.coinCount.text = left == 0 ? "次日恢复5次，已得钻石保留" : payout > 0 ? "返还 " + payout.ToString(CultureInfo.InvariantCulture) + " 钻（净增 " + (payout - data.coinPool).ToString(CultureInfo.InvariantCulture) + "）" : "返钻以本期配置为准";
            }
            if (page.btn != null)
            {
                UIImageButton button = page.btn.GetComponent<UIImageButton>();
                if (button != null) button.isEnabled = left > 0;
                UISprite[] sprites = page.btn.GetComponentsInChildren<UISprite>(true);
                for (int i = 0; i < sprites.Length; i++)
                    if (button != null && sprites[i] != button.target) sprites[i].enabled = false;
                UILabel text = NativeLabel(page.btn, "ReCatButton", page.surplusNumber);
                if (text != null) text.text = left > 0 ? "招财一次" : "今日次数已用完";
            }
        }

        public static void BindContinued(UIActivityContinueChargeGift page, Packet packet, bool success)
        {
            if (!success || !Continued(page)) return;
            SCContinuedRechargeInfo info = packet == null ? null : packet.kBody as SCContinuedRechargeInfo;
            if (info == null || info.itemList.Count == 0) return;
            ActivityItem current = null;
            for (int i = 0; i < info.itemList.Count; i++) if (info.itemList[i].index == info.currentIndex) current = info.itemList[i];
            if (current == null) return;
            ContinuedQuoteState state = page.GetComponent<ContinuedQuoteState>();
            if (state == null) state = page.gameObject.AddComponent<ContinuedQuoteState>();
            state.CurrentStatus = current.status;
            if (page.toDayCharedDiamond != null)
            {
                page.toDayCharedDiamond.width = 590;
                page.toDayCharedDiamond.fontSize = 19;
                page.toDayCharedDiamond.text = "今日实际消费 " + current.param + " / " + info.param + " 钻；每个消费日可领1档，共7档";
            }
            if (page.Day != null) page.Day.text = "7";
            if (page.value != null) page.value.text = current.value;
            if (page.wordOnBtn != null) page.wordOnBtn.enabled = false;
            UILabel button = NativeLabel(page.btnCharge, "ReContinuedButton", page.toDayCharedDiamond);
            if (button != null) button.text = current.status == 1 ? "领取" : "去消费";
        }

        public static bool NavigateContinued(UIActivityContinueChargeGift page)
        {
            if (!Continued(page)) return false;
            ContinuedQuoteState state = page.GetComponent<ContinuedQuoteState>();
            if (state == null || state.CurrentStatus != 0) return false;
            PageManager.Instance.OpenPage("ShopPage", String.Empty);
            return true;
        }
    }
}
