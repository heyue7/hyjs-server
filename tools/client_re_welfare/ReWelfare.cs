using System;
using System.Globalization;
using System.Collections.Generic;
using UnityEngine;
using UI;
using ProtoBuf.Message;
using ProtoBuf;
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
            long payout = 0;
            foreach (List<long> group in Extensible.GetValues<List<long>>(data, 100))
            foreach (long quote in group)
            {
                if (payout != 0 || quote < 1) return 0;
                payout = quote;
            }
            return payout;
        }

        public static void BindCat(UIActivityTreasureCast page, ActivityFortuneCatData data)
        {
            if (!Cat(page) || data == null) return;
            // Keep the original treasure artwork and 探索 caption. Daily eligibility still comes from the server.
            page.StopCoroutine("UpdateUI");
            if (page.time != null) page.time.SetActive(false);
            if (page.solgon != null) page.solgon.SetActive(true);
            if (page.solgon2 != null) page.solgon2.SetActive(false);
            int left = Math.Max(0, Math.Min(5, data.todayLeft));
            long payout = ReturnQuote(page, data);
            if (page.surplusNumber != null) page.surplusNumber.text = left.ToString(CultureInfo.InvariantCulture) + "/5";
            if (page.totlaCoin != null) page.totlaCoin.text = payout.ToString(CultureInfo.InvariantCulture);
            GameObject root = page.btn == null || page.btn.transform.parent == null ? null : page.btn.transform.parent.gameObject;
            UILabel cost = NativeLabel(root, "CatCostQuote", page.surplusNumber);
            if (cost != null)
            {
                cost.transform.localPosition = new Vector3(-130f, -116f, 0f);
                cost.width = 350; cost.height = 40; cost.fontSize = 20;
                cost.text = left > 0 && payout < 1 ? "请重新打开藏宝箱刷新奖励" : left > 0 ? "消耗 " + data.coinPool.ToString(CultureInfo.InvariantCulture) + " 钻石" : "今日探索次数已用完";
            }
            if (page.btn != null)
            {
                UIImageButton button = page.btn.GetComponent<UIImageButton>();
                if (button != null) button.isEnabled = left > 0 && payout > 0;
                UISprite[] words = page.btn.GetComponentsInChildren<UISprite>(true);
                for (int i = 0; i < words.Length; i++)
                    if (words[i].spriteName.StartsWith("UI_huodong_cangbaoxiang_word_", StringComparison.Ordinal))
                        words[i].spriteName = "UI_huodong_cangbaoxiang_word_tansuo";
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
                // Restore the original 306x40 under-title boundary from the shipped prefab.
                page.toDayCharedDiamond.width = 306;
                page.toDayCharedDiamond.height = 40;
                page.toDayCharedDiamond.fontSize = 20;
                page.toDayCharedDiamond.overflowMethod = UILabel.Overflow.ShrinkContent;
                page.toDayCharedDiamond.text = "今日消费：" + current.param + " / " + info.param + " 钻";
            }
            UILabel[] labels=page.GetComponentsInChildren<UILabel>(true);
            for(int i=0;i<labels.Length;i++)
                if(labels[i].text.IndexOf("充值天数达到",StringComparison.Ordinal)>=0)
                    labels[i].text=labels[i].text.Replace("充值天数达到","消费达标累计");
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
