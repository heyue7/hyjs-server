using System;
using System.Globalization;
using LitJson;
using ProtoBuf.Message;

namespace HYJS.DesertV4
{
    // The server receipt owns the temporary terminal. This projection is used
    // only by the original battle reward renderer; native inventory keeps the
    // unchanged result object returned by the game protocol.
    internal sealed class MonsterRewardView
    {
        readonly string fightId;
        readonly long[] chips;
        MonsterRewardView(string fight,long[] values) { fightId=fight; chips=values; }

        internal static MonsterRewardView FromReceipt(string fight,JsonData receipt)
        {
            if (String.IsNullOrEmpty(fight) || receipt==null || receipt["chips"]==null ||
                !receipt["chips"].IsArray || receipt["chips"].Count!=6)
                throw new InvalidOperationException("Invalid monster chip receipt");
            string delivery=(string)receipt["deliveryStatus"];
            if (delivery!="delivered" && delivery!="pending_delivery" && delivery!="lost")
                throw new InvalidOperationException("Invalid monster delivery status");
            var values=new long[6]; long total=0;
            for (int i=0;i<values.Length;i++)
            {
                long amount=Int64.Parse(receipt["chips"][i].ToString(),CultureInfo.InvariantCulture);
                if (amount<0 || amount>1 || total>1-amount) throw new InvalidOperationException("Invalid monster chips");
                values[i]=amount; total+=amount;
            }
            // A pending ordinary reward may still report chips already committed
            // to the terminal by the same monster settlement transaction.
            if (delivery=="lost" && total!=0) throw new InvalidOperationException("Lost monster chips");
            return new MonsterRewardView(fight,values);
        }

        internal ResultObject Project(string currentFight,ResultObject original)
        {
            if (currentFight!=fightId || original==null) return original;
            bool any=false; foreach (long amount in chips) if (amount>0) any=true;
            if (!any) return original;
            var view=new ResultObject();
            view.simps.AddRange(original.simps); view.equips.AddRange(original.equips);
            view.treas.AddRange(original.treas); view.wars.AddRange(original.wars); view.pets.AddRange(original.pets);
            for (int i=0;i<chips.Length;i++)
                if (chips[i]>0) view.simps.Add(new SimpleItem { id="item_"+(9701+i).ToString(CultureInfo.InvariantCulture),num=(int)chips[i] });
            return view;
        }
    }
}
