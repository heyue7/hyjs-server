using System;
using System.Collections.Generic;
using System.Globalization;
using NetWork.Packets;
using ProtoBuf.Message;

namespace HYJS.GameplayFixes
{
    public static class GuildRewardFeedback
    {
        public static void ShowGuildReward(Packet packet, bool ok)
        {
            if (!ok || packet == null) return;
            SCUnionOpenDonateBox reply = packet.kBody as SCUnionOpenDonateBox;
            if (reply == null || reply.get == null || reply.get.simps == null) return;

            var rewards = new List<string>();
            foreach (SimpleItem item in reply.get.simps)
            {
                if (item == null || String.IsNullOrEmpty(item.id) || item.num <= 0) continue;
                string name = item.id == "coin" ? "金币" : TableTextBlo.GetText(CommonMethod.GetObjectNameTextId(item.id));
                if (String.IsNullOrEmpty(name)) name = item.id;
                rewards.Add(name + " × " + item.num.ToString(CultureInfo.InvariantCulture));
            }
            if (rewards.Count > 0)
                BoxManager.CreateMessageBoxWithoutCancelButton(
                    "领取成功：" + String.Join("、", rewards.ToArray()), true,
                    delegate(bool confirmed) { }, "公会宝箱", false);
        }
    }
}
