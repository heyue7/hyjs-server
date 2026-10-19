using System;
using System.Globalization;
using UnityEngine;
using ProtoBuf.Message;

namespace HYJS.Achievements
{
    // These types extend only display/navigation. All progress and reward eligibility
    // remain the authenticated server's original AchievementInfo fields.
    public static class AchievementsBridge
    {
        public static string Page(int type)
        {
            if (type == 37) return "UIActivity";
            if (type == 38) return "AllHerosPage";
            if (type == 40) return "ArrayPage";
            return "UICopy";
        }

        static bool Positive(string value, out int number)
        {
            return Int32.TryParse(value, NumberStyles.None, CultureInfo.InvariantCulture, out number) && number > 0;
        }

        static string GrowthTitle(string id)
        {
            string[] parts = (id ?? "").Split(new char[] { '~' });
            int days, level, stages;
            if (parts.Length != 4 || String.IsNullOrEmpty(parts[0]) ||
                !Positive(parts[1], out days) || !Positive(parts[2], out level) || !Positive(parts[3], out stages))
                return "OS成长：登录、等级与主线目标";
            return String.Format(CultureInfo.InvariantCulture, "OS成长：登录{0}天 · 等级{1}级 · 主线{2}关", days, level, stages);
        }

        public static void Bind(AchievementItem item, AchievementInfo info)
        {
            if (item == null || info == null || info.type < 37 || info.type > 41) return;
            int total = Math.Max(0, info.totalNum);
            int current = Math.Min(Math.Max(0, info.currentNum), total);
            string text;
            switch (info.type)
            {
                case 37: text = "累计登录 " + total.ToString(CultureInfo.InvariantCulture) + " 天"; break;
                case 38: text = "收集 " + total.ToString(CultureInfo.InvariantCulture) + " 种角色"; break;
                case 39: text = "通关 " + total.ToString(CultureInfo.InvariantCulture) + " 个不同主线关卡"; break;
                case 40: text = "同时上阵 " + total.ToString(CultureInfo.InvariantCulture) + " 名角色"; break;
                default: text = GrowthTitle(info.id); break;
            }
            if (item.title != null) item.title.text = text;
            if (item.curScore != null)
                item.curScore.text = current.ToString(CultureInfo.InvariantCulture) + " / " + total.ToString(CultureInfo.InvariantCulture);
            // Keep state0=claim, state1=go, state2=claimed. The original prefab's
            // finishBtn/receiveBtn field names differ from their visible labels.
            // The full quoted ID remains in the original rewardID and is sent unchanged.
        }
    }
}
