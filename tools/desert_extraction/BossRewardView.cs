using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using LitJson;
using ProtoBuf.Message;

namespace HYJS.DesertV4
{
    // A receipt is authoritative for delivery. This projection is consumed only
    // by CopyBalance.Refresh; the native result and inventory are left intact.
    internal sealed class BossRewardView
    {
        sealed class Reward { internal string id; internal int amount; }
        readonly string fightId;
        readonly List<Reward> rewards;
        BossRewardView(string fight,List<Reward> values) { fightId=fight; rewards=values; }
        internal bool Matches(string fight) { return fight==fightId; }

        static JsonData Field(JsonData data,string key)
        {
            var obj=data as IDictionary;
            return obj!=null && obj.Contains(key) ? data[key] : null;
        }
        static bool FightId(string value)
        {
            if (value==null || value.Length!=24) return false;
            foreach (char c in value)
                if (!((c>='0' && c<='9') || (c>='a' && c<='f'))) return false;
            return true;
        }
        static int Quantity(JsonData value,bool positive)
        {
            int amount;
            if (value==null || (!value.IsInt && !value.IsLong) ||
                !Int32.TryParse(value.ToString(),NumberStyles.None,CultureInfo.InvariantCulture,out amount) ||
                (positive && amount==0)) throw new InvalidOperationException("Invalid boss reward quantity");
            return amount;
        }
        static bool ResourceId(string value)
        {
            if (String.IsNullOrEmpty(value) || value.Length>128 || value.Trim()!=value) return false;
            foreach (char c in value) if (Char.IsControl(c) || c==',' || c==';') return false;
            return true;
        }
        static bool ChipId(string id)
        {
            for (int i=1;i<=6;i++) if (id=="item_970"+i.ToString(CultureInfo.InvariantCulture)) return true;
            return false;
        }
        static void Add(List<Reward> rewards,Dictionary<string,Reward> byId,string id,int amount)
        {
            if (amount==0) return;
            Reward found;
            if (byId.TryGetValue(id,out found)) found.amount=checked(found.amount+amount);
            else { found=new Reward { id=id,amount=amount }; byId.Add(id,found); rewards.Add(found); }
        }
        internal static BossRewardView FromReceipt(string fight,JsonData receipt)
        {
            JsonData id=Field(receipt,"fightId"), state=Field(receipt,"rewardStatus");
            if (!FightId(fight) || receipt==null || !receipt.IsObject || id==null || !id.IsString ||
                (string)id!=fight || state==null || !state.IsString)
                throw new InvalidOperationException("Invalid boss reward identity");
            string status=(string)state;
            bool resourcesDelivered=status=="delivered" || status=="pending_chip_delivery";
            if (!resourcesDelivered && status!="pending_boss_defeat" && status!="pending_delivery" &&
                status!="daily_limit" && status!="no_contribution")
                throw new InvalidOperationException("Invalid boss reward status");

            var rewards=new List<Reward>(); var byId=new Dictionary<string,Reward>(StringComparer.Ordinal);
            JsonData resources=Field(receipt,"rewardResources");
            if (resources!=null)
            {
                if (!resources.IsArray || resources.Count>128) throw new InvalidOperationException("Invalid boss resources");
                for (int i=0;i<resources.Count;i++)
                {
                    JsonData row=resources[i], rid=Field(row,"id"), quantity=Field(row,"amount");
                    if (row==null || !row.IsObject || rid==null || !rid.IsString) throw new InvalidOperationException("Invalid boss resource");
                    string key=(string)rid; int amount=Quantity(quantity,true);
                    if (!resourcesDelivered || !ResourceId(key) || ChipId(key) || byId.ContainsKey(key))
                        throw new InvalidOperationException("Invalid boss resource identity");
                    Add(rewards,byId,key,amount);
                }
            }
            JsonData chips=Field(receipt,"rewardChips"), credit=Field(receipt,"rewardCredit");
            if (chips!=null)
            {
                if (status!="delivered" || !chips.IsArray || chips.Count!=6) throw new InvalidOperationException("Invalid boss chips");
                for (int i=0;i<6;i++) Add(rewards,byId,"item_"+(9701+i).ToString(CultureInfo.InvariantCulture),Quantity(chips[i],false));
            }
            if (credit!=null)
            {
                int amount=Quantity(credit,false);
                if (status!="delivered" || (chips!=null && amount!=0)) throw new InvalidOperationException("Invalid boss credit");
                Add(rewards,byId,"item_9710",amount);
            }
            return new BossRewardView(fight,rewards);
        }

        internal ResultObject Project(string currentFight,ResultObject original)
        {
            if (currentFight!=fightId || original==null || rewards.Count==0) return original;
            var present=new HashSet<string>(StringComparer.Ordinal);
            foreach (SimpleItem item in original.simps) if (item!=null && item.id!=null) present.Add(item.id);
            bool missing=false;
            foreach (Reward reward in rewards) if (!present.Contains(reward.id)) { missing=true; break; }
            if (!missing) return original;
            var view=new ResultObject();
            foreach (SimpleItem item in original.simps)
                view.simps.Add(item==null ? null : new SimpleItem { id=item.id,num=item.num });
            view.equips.AddRange(original.equips); view.treas.AddRange(original.treas);
            view.wars.AddRange(original.wars); view.pets.AddRange(original.pets);
            foreach (Reward reward in rewards)
                if (present.Add(reward.id)) view.simps.Add(new SimpleItem { id=reward.id,num=reward.amount });
            return view;
        }
    }
}
